package repo

import (
	"context"
	"encoding/json"
	"fmt"

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
