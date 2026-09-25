// Package assist is the finding assistant: the side panel that opens on one
// finding and answers a fixed set of commands about it — never free-form
// chat. Three commands call the model and are paid in tokens (explain,
// remediate, fix); "where is it?" is answered by the frontend from the
// finding's own location and never reaches this package.
//
// Rules this package keeps:
//   - Authorization is the finding's scan's project's org, the same 404-not-
//     403 rule every scan read uses, plus the project-scoped collaborator
//     restriction.
//   - The model call happens before the charge, and only a usable answer is
//     charged — a failed, schema-invalid or injection-discarded call costs
//     nothing. Each command is paid once per finding; asking again is free
//     (and normally a cache hit anyway).
//   - Repository content (evidence, a fetched source excerpt) only ever
//     reaches the model as untrusted, boundary-delimited content (modules/ai).
//   - A secret finding's file is never fetched or sent: only the scanner's
//     already-redacted evidence is.
package assist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// Action is one assistant command.
type Action = billing.AIAction

// FindingContext is a finding plus what's needed to authorize and enrich it.
type FindingContext struct {
	Finding     domain.Finding
	OrgID       uuid.UUID
	ProjectID   uuid.UUID
	ProjectName string
	CommitSHA   *string
	Branch      *string
}

// Repository loads a finding with its scan/project context.
type Repository interface {
	GetFindingContext(ctx context.Context, findingID uuid.UUID) (*FindingContext, error)
}

// Tokens is the billing surface this package needs. nil = billing off
// (every command is free).
type Tokens interface {
	AIQuote(ctx context.Context, orgID, findingID uuid.UUID, action billing.AIAction) (price int64, paid bool, balance int64, err error)
	ChargeAI(ctx context.Context, orgID, actorID, scanID, findingID uuid.UUID, action billing.AIAction) (int64, error)
}

// CloneInfo gives the project's repository URL and decrypted token
// (project.Service.GetCloneInfo).
type CloneInfo interface {
	GetCloneInfo(ctx context.Context, projectID uuid.UUID) (repoURL, branch, token string, err error)
}

// SourceFetcher reads one file at a ref from the repository host.
type SourceFetcher interface {
	FetchFile(ctx context.Context, repoURL, ref, filePath, token string) (string, error)
}

// Reply is one command's answer. Exactly one of Explain/Remediate/Fix is set.
type Reply struct {
	Action        Action
	TokensCharged int64
	// AlreadyPaid: this org paid for this command on this finding before,
	// so this answer was free.
	AlreadyPaid bool
	// SourceUsed: the answer was grounded in the real file, not only the
	// scanner's evidence snippet.
	SourceUsed bool
	Explain    *ai.ExplainFindingResponse
	Remediate  *ai.RemediateFindingResponse
	Fix        *ai.GeneratePatchResponse
}

// Service runs assistant commands.
type Service struct {
	repo   Repository
	ai     ai.Service // nil = AI disabled
	tokens Tokens     // nil = billing off
	clone  CloneInfo
	source SourceFetcher
	log    *slog.Logger
}

// NewService wires the assistant. aiSvc nil makes every command answer
// "AI is not configured"; tokens nil makes every command free; clone or
// source nil means answers use the scanner's evidence only.
func NewService(repo Repository, aiSvc ai.Service, tokens Tokens, clone CloneInfo, source SourceFetcher, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, ai: aiSvc, tokens: tokens, clone: clone, source: source, log: log}
}

// Enabled reports whether AI commands can run at all.
func (s *Service) Enabled() bool { return s.ai != nil }

// Run answers one command about one finding for actor.
func (s *Service) Run(ctx context.Context, actor domain.Actor, findingID uuid.UUID, action Action) (*Reply, error) {
	promptID, ok := map[Action]ai.PromptID{
		billing.AIExplain:   ai.PromptExplainFinding,
		billing.AIRemediate: ai.PromptRemediateFinding,
		billing.AIFix:       ai.PromptGeneratePatch,
	}[action]
	if !ok {
		return nil, apperrors.Validation("assist.unknown_action", "unknown command", nil)
	}
	if s.ai == nil {
		return nil, apperrors.Unprocessable("assist.ai_disabled", "AI isn't configured on this server.")
	}

	fc, err := s.repo.GetFindingContext(ctx, findingID)
	if err != nil {
		var appErr *apperrors.Error
		if errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound {
			return nil, apperrors.NotFound("finding.not_found", "finding not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("load finding: %w", err))
	}
	if fc.OrgID != actor.OrgID || (actor.ProjectID != nil && *actor.ProjectID != fc.ProjectID) {
		return nil, apperrors.NotFound("finding.not_found", "finding not found")
	}
	f := fc.Finding

	// A patch needs a line of the repository to change. Dependency, image
	// and network findings are fixed by upgrading or reconfiguring, which
	// Remediation covers — refuse before any model call or charge.
	if action == billing.AIFix && filePathOf(f.Location) == "" {
		return nil, apperrors.Unprocessable("assist.not_patchable",
			"There's no source line to patch for this kind of finding — use Remediation for the exact upgrade or configuration steps.")
	}

	// Pre-flight the balance so a user who can't pay doesn't get a free
	// answer; the real charge comes after a usable answer.
	var alreadyPaid bool
	if s.tokens != nil {
		price, paid, balance, err := s.tokens.AIQuote(ctx, fc.OrgID, f.ID, action)
		if err != nil {
			return nil, err
		}
		alreadyPaid = paid
		if !paid && balance < price {
			return nil, apperrors.PaymentRequired("billing.insufficient_tokens",
				fmt.Sprintf("this needs %d tokens and %d are left", price, balance)).
				WithExtensions(map[string]any{"required": price, "available": balance, "shortfall": price - balance})
		}
	}

	in := ai.RunInput{PromptID: promptID, Vars: s.vars(fc), ScanID: f.ScanID, Engine: f.Engine, Source: f.Location}
	sourceUsed := false
	if action != billing.AIExplain {
		if excerpt, ok := s.sourceExcerpt(ctx, fc); ok {
			in.Untrusted = append(in.Untrusted, ai.UntrustedBlock{Label: filePathOf(f.Location), Content: excerpt})
			sourceUsed = true
		}
	}
	if !sourceUsed || action == billing.AIExplain {
		in.Untrusted = append(in.Untrusted, evidenceBlocks(f)...)
	}

	res, err := s.ai.Run(ctx, in, nil)
	if err != nil {
		s.log.Warn("assist: model call failed", "finding_id", f.ID, "action", action, "error", err)
		var appErr *apperrors.Error
		if errors.As(err, &appErr) && appErr.Code == "gemini.unavailable" {
			return nil, apperrors.RateLimited("assist.ai_quota",
				"GuardPipe AI has hit its usage limit for now — try again in a few minutes. Nothing was charged.", 60)
		}
		return nil, apperrors.External("assist.ai_failed", "The AI couldn't answer right now — nothing was charged. Try again in a moment.", err)
	}
	if res.Discarded {
		return nil, apperrors.Unprocessable("assist.discarded",
			"The AI's answer was withheld because this finding's content looked like an attempt to manipulate it. Nothing was charged.")
	}

	reply := &Reply{Action: action, AlreadyPaid: alreadyPaid, SourceUsed: sourceUsed}
	switch v := res.Value.(type) {
	case ai.ExplainFindingResponse:
		reply.Explain = &v
	case ai.RemediateFindingResponse:
		reply.Remediate = &v
	case ai.GeneratePatchResponse:
		reply.Fix = &v
	default:
		return nil, apperrors.Internal(fmt.Errorf("assist: unexpected AI result %T", res.Value))
	}

	if s.tokens != nil && !alreadyPaid {
		charged, err := s.tokens.ChargeAI(ctx, fc.OrgID, actor.UserID, f.ScanID, f.ID, action)
		if err != nil {
			return nil, err
		}
		reply.TokensCharged = charged
	}
	return reply, nil
}

// vars is the trusted metadata interpolated into every assistant prompt —
// scanner-authored fields only, never repository content.
func (s *Service) vars(fc *FindingContext) map[string]string {
	f := fc.Finding
	lines := ""
	if f.Location.LineStart > 0 {
		lines = fmt.Sprint(f.Location.LineStart)
		if f.Location.LineEnd > f.Location.LineStart {
			lines += fmt.Sprintf("-%d", f.Location.LineEnd)
		}
	}
	return map[string]string{
		"rule_id":     f.RuleID,
		"title":       f.Title,
		"severity":    string(f.Severity),
		"cwe":         orNone(strings.Join(f.CWE, ", ")),
		"cve":         orNone(strings.Join(f.CVE, ", ")),
		"location":    describeLocation(f.Location),
		"description": orNone(f.Description),
		"remediation": orNone(f.Remediation),
		"file_path":   orNone(filePathOf(f.Location)),
		"lines":       orNone(lines),
		"language":    languageFor(filePathOf(f.Location)),
	}
}

// excerptRadius is how many lines around the finding the excerpt carries.
const excerptRadius = 40

// sourceExcerpt fetches the finding's file at the scanned commit and returns
// a numbered excerpt around it — or ok=false (then evidence is used). Never
// for secret findings: their files hold the secret itself.
func (s *Service) sourceExcerpt(ctx context.Context, fc *FindingContext) (string, bool) {
	f := fc.Finding
	filePath := filePathOf(f.Location)
	if s.clone == nil || s.source == nil || filePath == "" || isSecretFinding(f) {
		return "", false
	}
	repoURL, branch, token, err := s.clone.GetCloneInfo(ctx, fc.ProjectID)
	if err != nil || repoURL == "" {
		return "", false
	}
	ref := branch
	if fc.CommitSHA != nil && *fc.CommitSHA != "" {
		ref = *fc.CommitSHA
	} else if fc.Branch != nil && *fc.Branch != "" {
		ref = *fc.Branch
	}
	content, err := s.source.FetchFile(ctx, repoURL, ref, filePath, token)
	if err != nil {
		s.log.Info("assist: source excerpt unavailable, using evidence", "finding_id", f.ID, "error", err)
		return "", false
	}
	return numberedExcerpt(content, f.Location.LineStart, f.Location.LineEnd, excerptRadius), true
}

// numberedExcerpt returns lines [start-radius, end+radius] prefixed "N | ".
// With no line info it returns the first 2*radius lines.
func numberedExcerpt(content string, start, end, radius int) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if end < start {
		end = start
	}
	from, to := 1, min(len(lines), 2*radius)
	if start > 0 {
		from = max(1, start-radius)
		to = min(len(lines), end+radius)
	}
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "%d | %s\n", i, lines[i-1])
	}
	return b.String()
}

// filePathOf is the repository file a finding points into: Path for file
// findings, File for Kubernetes manifests; "" for anything else (a
// dependency CVE, an image layer, a network endpoint).
func filePathOf(l domain.Location) string {
	switch l.Type {
	case domain.LocationTypeFile:
		return l.Path
	case domain.LocationTypeK8s:
		return l.File
	}
	return ""
}

func isSecretFinding(f domain.Finding) bool {
	if strings.Contains(f.RuleID, ".secret") {
		return true
	}
	for _, ev := range f.Evidence {
		if ev.Redacted {
			return true
		}
	}
	return false
}

func evidenceBlocks(f domain.Finding) []ai.UntrustedBlock {
	var out []ai.UntrustedBlock
	for i, ev := range f.Evidence {
		if strings.TrimSpace(ev.Value) == "" || i >= 3 {
			continue
		}
		label := "evidence"
		if f.Location.Path != "" {
			label = f.Location.Path
		}
		if ev.LineStart > 0 {
			label += fmt.Sprintf(" (line %d)", ev.LineStart)
		}
		out = append(out, ai.UntrustedBlock{Label: label, Content: ev.Value})
	}
	return out
}

func describeLocation(l domain.Location) string {
	switch {
	case l.Path != "" && l.LineStart > 0:
		return fmt.Sprintf("%s line %d", l.Path, l.LineStart)
	case l.Path != "":
		return l.Path
	case l.File != "":
		return l.File
	case l.Package != "":
		return strings.TrimSpace(fmt.Sprintf("%s %s@%s", l.Ecosystem, l.Package, l.Version))
	case l.URL != "":
		return l.URL
	case l.Host != "":
		return fmt.Sprintf("%s:%d", l.Host, l.Port)
	case l.Image != "":
		return l.Image
	}
	return "unknown"
}

func languageFor(p string) string {
	switch strings.ToLower(path.Ext(p)) {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".java":
		return "Java"
	case ".php":
		return "PHP"
	case ".rb":
		return "Ruby"
	case ".cs":
		return "C#"
	case ".yml", ".yaml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".tf":
		return "Terraform"
	case ".md":
		return "Markdown"
	}
	if strings.EqualFold(path.Base(p), "Dockerfile") {
		return "Dockerfile"
	}
	return "unknown"
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "none"
	}
	return s
}
