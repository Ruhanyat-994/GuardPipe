package reporting

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// ErrStatusConflict is returned by FindingStatusRepository.UpdateStatus
// when the finding's stored status no longer matches the `from` this call
// expected — a concurrent transition already happened between this
// service's read and write. Service maps it to a 409, not a 500: the
// client's fix is "reload and retry," not "something broke."
var ErrStatusConflict = errors.New("reporting: finding status changed concurrently")

// FindingRepository is the read path for a single finding — defined here
// (the consumer), implemented by store/repo.FindingRepo. Deliberately
// narrower than orchestrator.FindingRepository (no scan-scoped listing):
// this package only ever looks up one finding by its own ID.
type FindingRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Finding, error)
}

// FindingStatusRepository is the triage write path — separate from
// FindingRepository (which stays read-only) because it's a genuinely
// different write path than finding creation: a finding is created once,
// in bulk, by orchestrator's JobResultRepository (see that type's own doc
// comment); triage is a later, independent, one-row-at-a-time mutation of
// the same DR-003 mutable field group.
type FindingStatusRepository interface {
	// UpdateStatus writes the new status onto the finding and appends one
	// finding_status_history row, atomically. Returns ErrStatusConflict if
	// the finding's current status no longer matches from.
	UpdateStatus(ctx context.Context, findingID uuid.UUID, from, to domain.Status, reason string, changedBy uuid.UUID) error
	ListHistory(ctx context.Context, findingID uuid.UUID) ([]StatusHistoryEntry, error)
}

// ScanRepository is the narrow read this package needs purely to resolve a
// finding's project (for org-scoped authorization) — the same shape
// orchestrator.ScanRepository exposes, defined separately here rather than
// imported from that package, since Assembler (this same package) already
// depends on orchestrator.Service one direction and importing it back would
// cycle.
type ScanRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Scan, error)
}

// Service is finding triage: read one finding, transition its status, read
// its status-change history (documentation/05-module-specifications.md
// §15's triage slice — query/filter and cross-scan correlation are
// separate, not-yet-built pieces of that same section; export already
// exists via Assembler).
type Service interface {
	GetFinding(ctx context.Context, actor domain.Actor, findingID uuid.UUID) (*domain.Finding, error)
	// UpdateFindingStatus also returns the resolved display name of the
	// actor who made the change (via UserReader — same tolerant-of-missing-
	// user convention Assembler.attachRequestedBy already uses), so the
	// caller doesn't need a second lookup to answer the API's own
	// status_changed_by: {id, display_name} shape.
	UpdateFindingStatus(ctx context.Context, actor domain.Actor, findingID uuid.UUID, newStatus domain.Status, reason string) (finding *domain.Finding, changedByName string, err error)
	GetFindingHistory(ctx context.Context, actor domain.Actor, findingID uuid.UUID) ([]StatusHistoryEntry, error)
}

type triageService struct {
	findings FindingRepository
	status   FindingStatusRepository
	scans    ScanRepository
	projects ProjectReader
	users    UserReader
}

// NewService wires the triage Service. users may be nil (a caller that
// doesn't care about the display-name resolution) — UpdateFindingStatus
// then simply returns an empty changedByName, the same tolerant fallback
// Assembler.attachRequestedBy already uses for a missing UserReader.
func NewService(findings FindingRepository, status FindingStatusRepository, scans ScanRepository, projects ProjectReader, users UserReader) Service {
	return &triageService{findings: findings, status: status, scans: scans, projects: projects, users: users}
}

func (s *triageService) GetFinding(ctx context.Context, actor domain.Actor, findingID uuid.UUID) (*domain.Finding, error) {
	return s.getOwnedFinding(ctx, actor, findingID)
}

func (s *triageService) UpdateFindingStatus(ctx context.Context, actor domain.Actor, findingID uuid.UUID, newStatus domain.Status, reason string) (*domain.Finding, string, error) {
	finding, err := s.getOwnedFinding(ctx, actor, findingID)
	if err != nil {
		return nil, "", err
	}
	if !newStatus.Valid() {
		return nil, "", apperrors.Validation("finding.invalid_status", "status is not a recognised value", nil)
	}

	if err := ValidateTransition(finding.Status, newStatus, reason); err != nil {
		switch {
		case errors.Is(err, ErrReasonTooShort):
			return nil, "", apperrors.Validation("finding.reason_required", "suppression requires at least 20 characters of justification", nil)
		case errors.Is(err, ErrInvalidTransition):
			return nil, "", apperrors.Unprocessable("finding.invalid_transition", fmt.Sprintf("cannot move a finding from %s to %s", finding.Status, newStatus))
		default:
			return nil, "", apperrors.Internal(err)
		}
	}

	if err := s.status.UpdateStatus(ctx, findingID, finding.Status, newStatus, reason, actor.UserID); err != nil {
		if errors.Is(err, ErrStatusConflict) {
			return nil, "", apperrors.Conflict("finding.status_conflict", "this finding's status changed since it was last read — reload and try again")
		}
		return nil, "", apperrors.Internal(fmt.Errorf("update finding status: %w", err))
	}

	finding.Status = newStatus
	if reason != "" {
		finding.StatusReason = &reason
	}
	finding.StatusChangedBy = &actor.UserID

	return finding, s.resolveDisplayName(ctx, actor.UserID), nil
}

func (s *triageService) GetFindingHistory(ctx context.Context, actor domain.Actor, findingID uuid.UUID) ([]StatusHistoryEntry, error) {
	if _, err := s.getOwnedFinding(ctx, actor, findingID); err != nil {
		return nil, err
	}
	entries, err := s.status.ListHistory(ctx, findingID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list finding status history: %w", err))
	}
	return entries, nil
}

// getOwnedFinding is this package's own 404-not-403 authorization check,
// mirroring orchestrator.service's getOwnedScan exactly (a cross-org
// finding ID must look identical to a nonexistent one): resolve the
// finding, resolve its scan to get the owning project, then let
// ProjectReader.Get's own org-scoping decide.
func (s *triageService) getOwnedFinding(ctx context.Context, actor domain.Actor, findingID uuid.UUID) (*domain.Finding, error) {
	finding, err := s.findings.GetByID(ctx, findingID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("finding.not_found", "finding not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get finding: %w", err))
	}
	scan, err := s.scans.GetByID(ctx, finding.ScanID)
	if err != nil {
		return nil, apperrors.NotFound("finding.not_found", "finding not found")
	}
	if _, err := s.projects.Get(ctx, actor, scan.ProjectID); err != nil {
		return nil, apperrors.NotFound("finding.not_found", "finding not found")
	}
	return finding, nil
}

// resolveDisplayName is the same tolerant-of-failure lookup
// Assembler.attachRequestedBy already uses — a missing UserReader or a
// since-deleted user just means an empty name, never an error that fails
// the whole triage action.
func (s *triageService) resolveDisplayName(ctx context.Context, userID uuid.UUID) string {
	if s.users == nil {
		return ""
	}
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return ""
	}
	return u.DisplayName
}

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
