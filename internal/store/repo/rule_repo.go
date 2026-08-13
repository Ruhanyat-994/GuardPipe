package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// RuleRepo implements advisory.RuleRepository against the `rules` table
// (documentation/06-database-design.md §4.15).
type RuleRepo struct {
	db Querier
}

func NewRuleRepo(db Querier) *RuleRepo {
	return &RuleRepo{db: db}
}

var _ advisory.RuleRepository = (*RuleRepo)(nil)

// Upsert inserts a new rule, or refreshes every column except `enabled` on
// an existing one — a re-sync from the code registry must never silently
// re-enable a rule an operator disabled (documentation/06-database-design.md
// §4.15).
func (r *RuleRepo) Upsert(ctx context.Context, rule advisory.Rule) error {
	const q = `
		INSERT INTO rules (id, engine, category, title, description, remediation,
			default_severity, cwe, owasp, "references", tier, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, true)
		ON CONFLICT (id) DO UPDATE SET
			engine = EXCLUDED.engine,
			category = EXCLUDED.category,
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			remediation = EXCLUDED.remediation,
			default_severity = EXCLUDED.default_severity,
			cwe = EXCLUDED.cwe,
			owasp = EXCLUDED.owasp,
			"references" = EXCLUDED."references",
			tier = EXCLUDED.tier,
			updated_at = now()`
	_, err := r.db.Exec(ctx, q,
		rule.ID, string(rule.Engine), rule.Category, rule.Title, rule.Description, rule.Remediation,
		string(rule.DefaultSeverity), nonNilStrings(rule.CWE), nonNilStrings(rule.OWASP), nonNilStrings(rule.References), string(rule.Tier),
	)
	if err != nil {
		return fmt.Errorf("repo: upsert rule: %w", err)
	}
	return nil
}

func (r *RuleRepo) List(ctx context.Context, filter advisory.RuleFilter, page advisory.RulePage) ([]advisory.Rule, int, error) {
	where, args := buildRuleFilter(filter)

	countQ := "SELECT count(*) FROM rules" + where
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count rules: %w", err)
	}

	listQ := ruleSelectColumns + " FROM rules" + where +
		fmt.Sprintf(" ORDER BY id LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	offset := (page.Page - 1) * page.PageSize
	rows, err := r.db.Query(ctx, listQ, append(args, page.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list rules: %w", err)
	}
	defer rows.Close()

	var out []advisory.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("repo: scan rule: %w", err)
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate rules: %w", err)
	}
	return out, total, nil
}

func (r *RuleRepo) GetByID(ctx context.Context, id string) (*advisory.Rule, error) {
	const q = ruleSelectColumns + ` FROM rules WHERE id = $1`
	rule, err := scanRule(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("rule.not_found", "rule not found")
		}
		return nil, fmt.Errorf("repo: get rule: %w", err)
	}
	return &rule, nil
}

func (r *RuleRepo) SetEnabled(ctx context.Context, id string, enabled bool) (*advisory.Rule, error) {
	const q = `UPDATE rules SET enabled = $2, updated_at = now() WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id, enabled)
	if err != nil {
		return nil, fmt.Errorf("repo: set rule enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, apperrors.NotFound("rule.not_found", "rule not found")
	}
	return r.GetByID(ctx, id)
}

// buildRuleFilter turns a RuleFilter into a WHERE clause and its args,
// matching project_repo.go's pattern of hand-written SQL for filters that
// don't fit sqlc's static-query model.
func buildRuleFilter(filter advisory.RuleFilter) (string, []any) {
	var clauses []string
	var args []any

	if filter.Engine != nil {
		args = append(args, string(*filter.Engine))
		clauses = append(clauses, fmt.Sprintf("engine = $%d", len(args)))
	}
	if filter.Tier != nil {
		args = append(args, string(*filter.Tier))
		clauses = append(clauses, fmt.Sprintf("tier = $%d", len(args)))
	}
	if filter.Severity != nil {
		args = append(args, string(*filter.Severity))
		clauses = append(clauses, fmt.Sprintf("default_severity = $%d", len(args)))
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

const ruleSelectColumns = `
	SELECT id, engine, category, title, description, remediation,
		default_severity, cwe, owasp, "references", tier, enabled, created_at, updated_at`

func scanRule(row rowScanner) (advisory.Rule, error) {
	var rule advisory.Rule
	var engine, severity, tier string
	err := row.Scan(
		&rule.ID, &engine, &rule.Category, &rule.Title, &rule.Description, &rule.Remediation,
		&severity, &rule.CWE, &rule.OWASP, &rule.References, &tier, &rule.Enabled,
		&rule.CreatedAt, &rule.UpdatedAt,
	)
	if err != nil {
		return advisory.Rule{}, err
	}
	rule.Engine = domain.EngineID(engine)
	rule.DefaultSeverity = domain.Severity(severity)
	rule.Tier = domain.Tier(tier)
	return rule, nil
}
