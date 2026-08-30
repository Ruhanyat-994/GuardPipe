package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
)

// AuditRepo implements audit.Repository against the `audit_log` table
// (documentation/06-database-design.md §4.19). Insert-only, matching
// DR-007 — this type exposes no update or delete method.
type AuditRepo struct {
	db Querier
}

func NewAuditRepo(db Querier) *AuditRepo {
	return &AuditRepo{db: db}
}

var _ audit.Repository = (*AuditRepo)(nil)

func (r *AuditRepo) Insert(ctx context.Context, e audit.Entry) error {
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("repo: encode audit detail: %w", err)
	}

	const q = `
		INSERT INTO audit_log (org_id, actor_id, action, resource_type, resource_id, detail, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err = r.db.Exec(ctx, q, e.OrgID, e.ActorID, e.Action, e.ResourceType, e.ResourceID, detailJSON, e.IP)
	if err != nil {
		return fmt.Errorf("repo: insert audit entry: %w", err)
	}
	return nil
}

// List backs `GET /admin/audit-log` (BUILD_GUIDE.md Phase 14) — the one
// screen where a nil filter.OrgID deliberately means "every organisation,"
// not "none," since lifting the normal per-org scoping is the whole point
// of that screen.
func (r *AuditRepo) List(ctx context.Context, filter audit.ListFilter, page audit.Page) ([]audit.Entry, int, error) {
	where, args := buildAuditFilter(filter)

	countQ := "SELECT count(*) FROM audit_log" + where
	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("repo: count audit log: %w", err)
	}

	const cols = `SELECT id, org_id, actor_id, action, resource_type, resource_id, detail, ip, created_at FROM audit_log`
	listQ := cols + where + fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	offset := (page.Page - 1) * page.PageSize
	rows, err := r.db.Query(ctx, listQ, append(args, page.PageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list audit log: %w", err)
	}
	defer rows.Close()

	var out []audit.Entry
	for rows.Next() {
		e, err := scanAuditEntry(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("repo: scan audit entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate audit log: %w", err)
	}
	return out, total, nil
}

func scanAuditEntry(row rowScanner) (audit.Entry, error) {
	var e audit.Entry
	var detailJSON []byte
	var ip *netip.Addr
	if err := row.Scan(&e.ID, &e.OrgID, &e.ActorID, &e.Action, &e.ResourceType, &e.ResourceID, &detailJSON, &ip, &e.CreatedAt); err != nil {
		return audit.Entry{}, err
	}
	if len(detailJSON) > 0 {
		if err := json.Unmarshal(detailJSON, &e.Detail); err != nil {
			return audit.Entry{}, fmt.Errorf("decode audit detail: %w", err)
		}
	}
	e.IP = ip
	return e, nil
}

// buildAuditFilter turns a ListFilter into a WHERE clause and its args,
// matching rule_repo.go's buildRuleFilter pattern for hand-written dynamic
// filters.
func buildAuditFilter(filter audit.ListFilter) (string, []any) {
	var clauses []string
	var args []any

	if filter.OrgID != nil {
		args = append(args, *filter.OrgID)
		clauses = append(clauses, fmt.Sprintf("org_id = $%d", len(args)))
	}
	if filter.ActorID != nil {
		args = append(args, *filter.ActorID)
		clauses = append(clauses, fmt.Sprintf("actor_id = $%d", len(args)))
	}
	if filter.Action != nil {
		args = append(args, *filter.Action)
		clauses = append(clauses, fmt.Sprintf("action = $%d", len(args)))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		clauses = append(clauses, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		clauses = append(clauses, fmt.Sprintf("created_at <= $%d", len(args)))
	}

	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
