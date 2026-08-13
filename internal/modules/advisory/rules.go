package advisory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// Rule is one row of the `rules` catalogue
// (documentation/06-database-design.md §4.15) — what documentation/07-api-specification.md
// §8's endpoints read and (for Enabled) write.
type Rule struct {
	ID              string
	Engine          domain.EngineID
	Category        string
	Title           string
	Description     string
	Remediation     string
	DefaultSeverity domain.Severity
	CWE             []string
	OWASP           []string
	References      []string
	Tier            domain.Tier
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// RuleFilter narrows GET /rules (documentation/07-api-specification.md §8:
// "filterable by engine/tier/severity"). A nil field means unfiltered on
// that dimension.
type RuleFilter struct {
	Engine   *domain.EngineID
	Tier     *domain.Tier
	Severity *domain.Severity
}

// RulePage is the same page/page-size shape project.Page uses, kept local
// to this package rather than shared, since the two have no reason to
// change together.
type RulePage struct {
	Page     int
	PageSize int
}

// RuleRepository is defined by this package
// (documentation/04-backend-architecture.md §5.1); implementation lives in
// internal/store/repo.
type RuleRepository interface {
	// Upsert inserts a new rule, or updates every column except Enabled on
	// an existing one — SyncRules calls this every startup, and a re-sync
	// must never silently re-enable a rule an operator disabled
	// (documentation/06-database-design.md §4.15: "lets an operator disable
	// a noisy rule without a deploy").
	Upsert(ctx context.Context, r Rule) error
	List(ctx context.Context, filter RuleFilter, page RulePage) ([]Rule, int, error)
	GetByID(ctx context.Context, id string) (*Rule, error)
	SetEnabled(ctx context.Context, id string, enabled bool) (*Rule, error)
}

// RuleRegistry collects every engine's domain.RuleMeta so SyncRules can
// upsert them into the database at startup
// (documentation/06-database-design.md §11: "rules — all rules, synced from
// the code registry at every startup"). At Phase 5 no engine exists yet
// (codescan/depscan land in Phase 6+), so a fresh RuleRegistry is
// legitimately empty — SyncRules upserting zero rows is the honest state of
// the system today, not a bug, the same acknowledgment Phase 4's ai module
// made about nothing calling it yet.
type RuleRegistry struct {
	rules []domain.RuleMeta
}

func NewRuleRegistry() *RuleRegistry {
	return &RuleRegistry{}
}

// Register adds rules to the registry. Called once per engine package at
// wiring time (cmd/guardpipe/main.go), not per-request.
func (r *RuleRegistry) Register(rules ...domain.RuleMeta) {
	r.rules = append(r.rules, rules...)
}

// All returns every registered rule.
func (r *RuleRegistry) All() []domain.RuleMeta {
	return r.rules
}

// ParseRuleID splits a permanent rule ID into its engine and category
// segments, per CLAUDE.md's documented format: `<engine>.<category>.<rule>`
// (e.g. "codescan.injection.sql-string-concat"). This is how Rule.Engine
// and Rule.Category are derived without adding fields to domain.RuleMeta
// that every existing rule declaration would need to grow.
func ParseRuleID(id string) (engine domain.EngineID, category string, err error) {
	parts := strings.SplitN(id, ".", 3)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("advisory: rule ID %q is not in <engine>.<category>.<rule> form", id)
	}
	engine = domain.EngineID(parts[0])
	if !engine.Valid() {
		return "", "", fmt.Errorf("advisory: rule ID %q has an unrecognised engine %q", id, parts[0])
	}
	return engine, parts[1], nil
}

func (s *service) SyncRules(ctx context.Context) error {
	for _, rm := range s.registry.All() {
		engine, category, err := ParseRuleID(rm.ID)
		if err != nil {
			return apperrors.Internal(fmt.Errorf("sync rules: %w", err))
		}
		if !rm.Tier.Valid() {
			return apperrors.Internal(fmt.Errorf("sync rules: rule %q has an invalid tier %q", rm.ID, rm.Tier))
		}
		r := Rule{
			ID:              rm.ID,
			Engine:          engine,
			Category:        category,
			Title:           rm.Title,
			Description:     rm.Description,
			Remediation:     rm.Remediation,
			DefaultSeverity: rm.Severity,
			CWE:             rm.CWE,
			OWASP:           rm.OWASP,
			References:      rm.References,
			Tier:            rm.Tier,
		}
		if err := s.rules.Upsert(ctx, r); err != nil {
			return apperrors.Internal(fmt.Errorf("sync rule %q: %w", rm.ID, err))
		}
	}
	return nil
}

func (s *service) ListRules(ctx context.Context, filter RuleFilter, page RulePage) ([]Rule, int, error) {
	if page.Page < 1 {
		page.Page = 1
	}
	if page.PageSize < 1 || page.PageSize > 200 {
		page.PageSize = 50
	}
	rules, total, err := s.rules.List(ctx, filter, page)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list rules: %w", err))
	}
	return rules, total, nil
}

func (s *service) GetRule(ctx context.Context, id string) (*Rule, error) {
	r, err := s.rules.GetByID(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("rule.not_found", "rule not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get rule: %w", err))
	}
	return r, nil
}

func (s *service) SetRuleEnabled(ctx context.Context, id string, enabled bool) (*Rule, error) {
	r, err := s.rules.SetEnabled(ctx, id, enabled)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("rule.not_found", "rule not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("set rule enabled: %w", err))
	}
	return r, nil
}

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
