package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/assist"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// AssistRepo implements assist.Repository: one finding (with evidence) plus
// its scan's project/org context. Read-only.
type AssistRepo struct {
	db Querier
}

func NewAssistRepo(db Querier) *AssistRepo {
	return &AssistRepo{db: db}
}

var _ assist.Repository = (*AssistRepo)(nil)

func (r *AssistRepo) GetFindingContext(ctx context.Context, findingID uuid.UUID) (*assist.FindingContext, error) {
	f, err := findingRowScan(r.db.QueryRow(ctx, findingSelectColumns+` WHERE id = $1`, findingID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("finding.not_found", "finding not found")
	}
	if err != nil {
		return nil, fmt.Errorf("repo: get finding: %w", err)
	}
	withEvidence := []domain.Finding{f}
	if err := attachEvidence(ctx, r.db, withEvidence); err != nil {
		return nil, err
	}

	fc := &assist.FindingContext{Finding: withEvidence[0]}
	err = r.db.QueryRow(ctx, `
		SELECT s.project_id, p.org_id, p.name, s.commit_sha, s.branch
		FROM scans s JOIN projects p ON p.id = s.project_id
		WHERE s.id = $1`, f.ScanID,
	).Scan(&fc.ProjectID, &fc.OrgID, &fc.ProjectName, &fc.CommitSHA, &fc.Branch)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("finding.not_found", "finding not found")
	}
	if err != nil {
		return nil, fmt.Errorf("repo: get finding scan context: %w", err)
	}
	return fc, nil
}
