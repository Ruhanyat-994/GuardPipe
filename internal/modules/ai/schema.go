package ai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrSchemaViolation is returned when a response is not valid JSON, is
// missing a required field, or violates a length/enum constraint
// (documentation/10-ai-integration.md §5, layer 4-5 "schema-constrained
// output" / "post-validation"). Service.Run treats this as retryable once
// (§5's repair retry), then gives up cleanly.
var ErrSchemaViolation = errors.New("ai: response did not match the expected schema")

// ErrInjectionSuspected is returned when a response — even one that is
// otherwise valid JSON matching the schema — appears to have echoed or
// obeyed injected instructions from the untrusted content it was analysing.
// Service.Run discards the response entirely and raises a
// prompt_injection_attempt finding instead of retrying
// (documentation/10-ai-integration.md §5).
var ErrInjectionSuspected = errors.New("ai: response suspected of prompt injection, discarded")

// maxResponseBytes is the "exceedsExpectedLength" sanity check
// (documentation/10-ai-integration.md §5, layer 5) — every schema here caps
// individual string fields well under this, so a response many times larger
// than any valid one indicates the model is echoing untrusted content back
// wholesale rather than producing the bounded structured output it was
// asked for.
const maxResponseBytes = 32 * 1024

// instructionEchoPhrases are a blunt heuristic for a response having quoted
// or obeyed injected instructions instead of treating them as data
// (documentation/10-ai-integration.md §5, layer 5 — "these layers reduce
// risk substantially; they do not eliminate it"). This is the last of five
// layers, not the only one.
var instructionEchoPhrases = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"ignore the above",
	"disregard previous instructions",
	"disregard the above instructions",
	"new instructions:",
	"you are now",
	"reveal this prompt",
	"reveal your instructions",
	"reveal your system prompt",
}

func containsInstructionEcho(raw []byte) bool {
	lower := strings.ToLower(string(raw))
	for _, phrase := range instructionEchoPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// RemediationStep is one step of a remediate_finding plan.
type RemediationStep struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code,omitempty"`
}

// RemediateFindingResponse is the decoded, validated shape of a
// remediate_finding response (the finding assistant's "Remediation").
type RemediateFindingResponse struct {
	Summary      string            `json:"summary"`
	Steps        []RemediationStep `json:"steps"`
	Verification string            `json:"verification"`
	Confidence   string            `json:"confidence"`
}

func (r RemediateFindingResponse) validate() error {
	if err := requireNonEmpty("summary", r.Summary, 400); err != nil {
		return err
	}
	if len(r.Steps) == 0 || len(r.Steps) > 8 {
		return fmt.Errorf("%w: steps must have 1-8 items, got %d", ErrSchemaViolation, len(r.Steps))
	}
	for i, st := range r.Steps {
		if err := requireNonEmpty(fmt.Sprintf("steps[%d].title", i), st.Title, 120); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("steps[%d].detail", i), st.Detail, 600); err != nil {
			return err
		}
		if len(st.Code) > 2000 {
			return fmt.Errorf("%w: steps[%d].code too long", ErrSchemaViolation, i)
		}
	}
	if err := requireNonEmpty("verification", r.Verification, 400); err != nil {
		return err
	}
	return requireEnum("confidence", r.Confidence, "high", "medium", "low")
}

// ExplainFindingResponse is the decoded, validated shape of an
// explain_finding response (documentation/10-ai-integration.md §6.1).
type ExplainFindingResponse struct {
	What         string `json:"what"`
	WhyItMatters string `json:"why_it_matters"`
	HowExploited string `json:"how_exploited"`
	Confidence   string `json:"confidence"`
}

func (r ExplainFindingResponse) validate() error {
	if err := requireNonEmpty("what", r.What, 400); err != nil {
		return err
	}
	if err := requireNonEmpty("why_it_matters", r.WhyItMatters, 400); err != nil {
		return err
	}
	if err := requireNonEmpty("how_exploited", r.HowExploited, 600); err != nil {
		return err
	}
	return requireEnum("confidence", r.Confidence, "high", "medium", "low")
}

// GeneratePatchResponse is the decoded, validated shape of a generate_patch
// response (documentation/10-ai-integration.md §6.2). Verified/unverified
// (via "git apply --check") is computed by the caller, not this package —
// that needs a sandboxed checkout this package has no access to.
type GeneratePatchResponse struct {
	Patch       string   `json:"patch"`
	Explanation string   `json:"explanation"`
	Confidence  string   `json:"confidence"`
	Caveats     []string `json:"caveats"`
}

func (r GeneratePatchResponse) validate() error {
	if strings.TrimSpace(r.Patch) == "" {
		return fmt.Errorf("%w: patch is empty", ErrSchemaViolation)
	}
	if err := requireNonEmpty("explanation", r.Explanation, 300); err != nil {
		return err
	}
	if r.Caveats == nil {
		return fmt.Errorf("%w: caveats field is required (may be an empty array)", ErrSchemaViolation)
	}
	return requireEnum("confidence", r.Confidence, "high", "medium", "low")
}

// DocumentReviewFinding is one element of a review_document response
// (documentation/10-ai-integration.md §6.3).
type DocumentReviewFinding struct {
	RuleID       string `json:"rule_id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Severity     string `json:"severity"`
	Excerpt      string `json:"excerpt"`
	Suggestion   string `json:"suggestion"`
	LocationHint string `json:"location_hint"`
}

// DocumentReviewResponse is the decoded, validated shape of a
// review_document response: an array of findings, possibly empty — an empty
// array is a valid, common result (§6.3, "return empty rather than
// manufacturing findings").
//
// Deliberately not enforced here: "rule_id must come from the supplied
// enum" (§6.3). The concrete docreview category list is owned by the
// docreview engine (documentation/05-module-specifications.md §11), which
// doesn't exist yet — it's built in Phase 11. Until then this validates
// shape only (every field present and non-empty, within its length cap);
// Phase 11 is expected to add the enum check once the category list is a
// real, importable thing rather than a set this package would have to
// duplicate and let drift.
type DocumentReviewResponse []DocumentReviewFinding

func (r DocumentReviewResponse) validate() error {
	for i, f := range r {
		if err := requireNonEmpty(fmt.Sprintf("[%d].rule_id", i), f.RuleID, 200); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].title", i), f.Title, 200); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].description", i), f.Description, 800); err != nil {
			return err
		}
		if err := requireEnum(fmt.Sprintf("[%d].severity", i), f.Severity, "critical", "high", "medium", "low", "informational"); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].excerpt", i), f.Excerpt, 400); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].suggestion", i), f.Suggestion, 400); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].location_hint", i), f.LocationHint, 200); err != nil {
			return err
		}
	}
	return nil
}

// WorkflowReviewFinding is one element of a review_workflow response
// (documentation/10-ai-integration.md §6.4). Filtering findings that
// overlap an already-fired rule by line proximity is the calling engine's
// job (cicdscan, Phase 10) — "the prompt instruction is a hint, the code is
// the guarantee" only becomes buildable once that engine's line-number
// bookkeeping exists; this package only validates shape.
type WorkflowReviewFinding struct {
	RuleID       string `json:"rule_id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	Severity     string `json:"severity"`
	Excerpt      string `json:"excerpt"`
	LocationHint string `json:"location_hint"`
}

type WorkflowReviewResponse []WorkflowReviewFinding

func (r WorkflowReviewResponse) validate() error {
	for i, f := range r {
		if err := requireNonEmpty(fmt.Sprintf("[%d].rule_id", i), f.RuleID, 200); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].title", i), f.Title, 200); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].description", i), f.Description, 800); err != nil {
			return err
		}
		if err := requireEnum(fmt.Sprintf("[%d].severity", i), f.Severity, "critical", "high", "medium", "low", "informational"); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].excerpt", i), f.Excerpt, 400); err != nil {
			return err
		}
		if err := requireNonEmpty(fmt.Sprintf("[%d].location_hint", i), f.LocationHint, 200); err != nil {
			return err
		}
	}
	return nil
}

// ScanSummaryResponse is the decoded, validated shape of a summarise_scan
// response (documentation/10-ai-integration.md §6.5).
type ScanSummaryResponse struct {
	Summary       string   `json:"summary"`
	TopPriorities []string `json:"top_priorities"`
}

func (r ScanSummaryResponse) validate() error {
	if err := requireNonEmpty("summary", r.Summary, 1200); err != nil {
		return err
	}
	if len(r.TopPriorities) != 3 {
		return fmt.Errorf("%w: top_priorities must have exactly 3 items, got %d", ErrSchemaViolation, len(r.TopPriorities))
	}
	for i, p := range r.TopPriorities {
		if strings.TrimSpace(p) == "" {
			return fmt.Errorf("%w: top_priorities[%d] is empty", ErrSchemaViolation, i)
		}
	}
	return nil
}

// Validate decodes raw against Prompt p's expected response shape and
// applies documentation/10-ai-integration.md §5's layers 4-5: valid JSON,
// every documented field present within its constraints, the
// injection-echo heuristic, and an overall size sanity check. It returns
// the decoded, typed value on success.
func Validate(p Prompt, raw json.RawMessage) (any, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, fmt.Errorf("%w: response is not valid JSON", ErrSchemaViolation)
	}

	value, err := decodeAndValidate(p, raw)
	if err != nil {
		return nil, err
	}

	if containsInstructionEcho(raw) {
		return nil, ErrInjectionSuspected
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("%w: response is %d bytes, larger than any valid response for %s", ErrSchemaViolation, len(raw), p.ID)
	}

	return value, nil
}

func decodeAndValidate(p Prompt, raw json.RawMessage) (any, error) {
	dec := func(v any) error {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(v); err != nil {
			return fmt.Errorf("%w: %v", ErrSchemaViolation, err)
		}
		return nil
	}

	switch p.ID {
	case PromptExplainFinding:
		var v ExplainFindingResponse
		if err := dec(&v); err != nil {
			return nil, err
		}
		v.Confidence = normalizeEnum(v.Confidence)
		if err := v.validate(); err != nil {
			return nil, err
		}
		return v, nil

	case PromptGeneratePatch:
		var v GeneratePatchResponse
		if err := dec(&v); err != nil {
			return nil, err
		}
		v.Confidence = normalizeEnum(v.Confidence)
		if err := v.validate(); err != nil {
			return nil, err
		}
		return v, nil

	case PromptReviewDocument:
		var v DocumentReviewResponse
		if err := dec(&v); err != nil {
			return nil, err
		}
		for i := range v {
			v[i].Severity = normalizeEnum(v[i].Severity)
		}
		if err := v.validate(); err != nil {
			return nil, err
		}
		return v, nil

	case PromptReviewWorkflow:
		var v WorkflowReviewResponse
		if err := dec(&v); err != nil {
			return nil, err
		}
		for i := range v {
			v[i].Severity = normalizeEnum(v[i].Severity)
		}
		if err := v.validate(); err != nil {
			return nil, err
		}
		return v, nil

	case PromptRemediateFinding:
		var v RemediateFindingResponse
		if err := dec(&v); err != nil {
			return nil, err
		}
		v.Confidence = normalizeEnum(v.Confidence)
		if err := v.validate(); err != nil {
			return nil, err
		}
		return v, nil

	case PromptSummariseScan:
		var v ScanSummaryResponse
		if err := dec(&v); err != nil {
			return nil, err
		}
		if err := v.validate(); err != nil {
			return nil, err
		}
		return v, nil

	default:
		return nil, fmt.Errorf("%w: unknown prompt id %q", ErrSchemaViolation, p.ID)
	}
}

func requireNonEmpty(field, value string, maxLen int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%w: %s is empty", ErrSchemaViolation, field)
	}
	if len(value) > maxLen {
		return fmt.Errorf("%w: %s is %d characters, longer than the %d-character limit", ErrSchemaViolation, field, len(value), maxLen)
	}
	return nil
}

// normalizeEnum canonicalizes an enum-constrained field before it's checked
// against requireEnum's allowed list — reproduced live against the real
// Gemini API: generationConfig.responseSchema's enum constraint is a hint,
// not a hard guarantee, and the model has been observed returning "High"
// where the schema lists only "high". The allowed lists themselves
// (requireEnum's call sites) are always lowercase, so this is the one place
// case/whitespace gets normalized rather than every call site needing to
// know that.
func normalizeEnum(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func requireEnum(field, value string, allowed ...string) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	return fmt.Errorf("%w: %s is %q, must be one of %v", ErrSchemaViolation, field, value, allowed)
}
