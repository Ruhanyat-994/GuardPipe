package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// ScanScheduleRepo implements orchestrator.ScanScheduleRepository against
// `scan_schedules` (migration 00021, BUILD_GUIDE.md Phase 15).
type ScanScheduleRepo struct {
	db Querier
}

func NewScanScheduleRepo(db Querier) *ScanScheduleRepo {
	return &ScanScheduleRepo{db: db}
}

var _ orchestrator.ScanScheduleRepository = (*ScanScheduleRepo)(nil)

func (r *ScanScheduleRepo) Create(ctx context.Context, s *orchestrator.ScanSchedule) error {
	profileJSON, err := json.Marshal(scanProfileDTO(s.Profile))
	if err != nil {
		return fmt.Errorf("repo: marshal scan profile: %w", err)
	}
	const q = `
		INSERT INTO scan_schedules (id, project_id, cron_expression, scan_profile, assigned_to, created_by, enabled, next_run_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`
	sID := s.ID
	if sID == uuid.Nil {
		sID = id.New()
	}
	err = r.db.QueryRow(ctx, q, sID, s.ProjectID, s.CronExpression, profileJSON, s.AssignedTo, s.CreatedBy, s.Enabled, s.NextRunAt).
		Scan(&s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("repo: insert scan schedule: %w", err)
	}
	s.ID = sID
	return nil
}

const scanScheduleColumns = `
	SELECT id, project_id, cron_expression, scan_profile, assigned_to, created_by, enabled,
	       next_run_at, last_run_at, last_run_status, created_at, updated_at`

func (r *ScanScheduleRepo) GetByID(ctx context.Context, id uuid.UUID) (*orchestrator.ScanSchedule, error) {
	const q = scanScheduleColumns + ` FROM scan_schedules WHERE id = $1`
	s, err := scanScanSchedule(r.db.QueryRow(ctx, q, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("schedule.not_found", "schedule not found")
		}
		return nil, fmt.Errorf("repo: get scan schedule: %w", err)
	}
	return s, nil
}

func (r *ScanScheduleRepo) ListByProject(ctx context.Context, projectID uuid.UUID) ([]orchestrator.ScanSchedule, error) {
	const q = scanScheduleColumns + ` FROM scan_schedules WHERE project_id = $1 ORDER BY created_at`
	return r.list(ctx, q, projectID)
}

// ListDue satisfies the scheduler ticker's one query — every enabled
// schedule whose next_run_at has arrived, oldest-due first so a backlog
// after downtime drains in a stable order.
func (r *ScanScheduleRepo) ListDue(ctx context.Context, at time.Time) ([]orchestrator.ScanSchedule, error) {
	const q = scanScheduleColumns + ` FROM scan_schedules WHERE enabled AND next_run_at <= $1 ORDER BY next_run_at`
	return r.list(ctx, q, at)
}

func (r *ScanScheduleRepo) list(ctx context.Context, q string, arg any) ([]orchestrator.ScanSchedule, error) {
	rows, err := r.db.Query(ctx, q, arg)
	if err != nil {
		return nil, fmt.Errorf("repo: list scan schedules: %w", err)
	}
	defer rows.Close()

	var out []orchestrator.ScanSchedule
	for rows.Next() {
		s, err := scanScanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("repo: scan scan schedule: %w", err)
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate scan schedules: %w", err)
	}
	return out, nil
}

func (r *ScanScheduleRepo) Update(ctx context.Context, s *orchestrator.ScanSchedule) error {
	profileJSON, err := json.Marshal(scanProfileDTO(s.Profile))
	if err != nil {
		return fmt.Errorf("repo: marshal scan profile: %w", err)
	}
	const q = `
		UPDATE scan_schedules
		SET cron_expression = $2, scan_profile = $3, assigned_to = $4, enabled = $5, next_run_at = $6, updated_at = now()
		WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, s.ID, s.CronExpression, profileJSON, s.AssignedTo, s.Enabled, s.NextRunAt)
	if err != nil {
		return fmt.Errorf("repo: update scan schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("schedule.not_found", "schedule not found")
	}
	return nil
}

func (r *ScanScheduleRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM scan_schedules WHERE id = $1`
	tag, err := r.db.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("repo: delete scan schedule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("schedule.not_found", "schedule not found")
	}
	return nil
}

func (r *ScanScheduleRepo) RecordRun(ctx context.Context, id uuid.UUID, ranAt time.Time, status string, nextRunAt time.Time) error {
	const q = `
		UPDATE scan_schedules
		SET last_run_at = $2, last_run_status = $3, next_run_at = $4, updated_at = now()
		WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, ranAt, status, nextRunAt); err != nil {
		return fmt.Errorf("repo: record scan schedule run: %w", err)
	}
	return nil
}

// scanProfileDTO/its json tags are this repo's own on-disk shape for
// orchestrator.ScanProfile — kept local rather than adding json tags to the
// domain-adjacent orchestrator type itself (store/repo owns serialisation
// concerns, the same split scan_repo.go's marshalPentestConfig already
// keeps for domain.Scan.PentestConfig).
type scanProfileDTOType struct {
	Type          domain.ScanType           `json:"type"`
	Engines       []domain.EngineID         `json:"engines,omitempty"`
	Branch        string                    `json:"branch,omitempty"`
	PentestConfig *domain.PentestScanConfig `json:"pentest_config,omitempty"`
}

func scanProfileDTO(p orchestrator.ScanProfile) scanProfileDTOType {
	return scanProfileDTOType{Type: p.Type, Engines: p.Engines, Branch: p.Branch, PentestConfig: p.PentestConfig}
}

func scanScanSchedule(row rowScanner) (*orchestrator.ScanSchedule, error) {
	var s orchestrator.ScanSchedule
	var profileJSON []byte
	var lastRunStatus *string
	if err := row.Scan(
		&s.ID, &s.ProjectID, &s.CronExpression, &profileJSON, &s.AssignedTo, &s.CreatedBy, &s.Enabled,
		&s.NextRunAt, &s.LastRunAt, &lastRunStatus, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	var dto scanProfileDTOType
	if err := json.Unmarshal(profileJSON, &dto); err != nil {
		return nil, fmt.Errorf("unmarshal scan profile: %w", err)
	}
	s.Profile = orchestrator.ScanProfile{Type: dto.Type, Engines: dto.Engines, Branch: dto.Branch, PentestConfig: dto.PentestConfig}
	if lastRunStatus != nil {
		s.LastRunStatus = *lastRunStatus
	}
	return &s, nil
}
