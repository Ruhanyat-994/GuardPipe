package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

// ActorRefResponse is the {id, display_name} shape
// documentation/07-api-specification.md uses for "who did this" fields.
type ActorRefResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

// FindingDetailResponse matches `GET /findings/{id}`. Embeds
// FindingListItemResponse — the list row is already self-sufficient
// (evidence/location/metadata all present, per that type's own doc
// comment) — and adds the two fields only a detail view needs.
type FindingDetailResponse struct {
	FindingListItemResponse
	StatusReason    string            `json:"status_reason,omitempty"`
	StatusChangedBy *ActorRefResponse `json:"status_changed_by"`
	StatusChangedAt *time.Time        `json:"status_changed_at"`
	// AISuggestion is nil until AI enrichment exists (BUILD_GUIDE.md Phase
	// 13's "wire AI enrichment onto findings" item, not yet built) — present
	// as null rather than omitted, documentation/07-api-specification.md
	// §1's standing convention.
	AISuggestion any `json:"ai_suggestion"`
	// History is nil until cross-scan correlation exists
	// (first_seen_scan_id/age_days/occurrence_count — a different, not-yet-
	// built piece from the status-change audit trail
	// FindingStatusHistoryResponse below already provides for real via
	// GET /findings/{id}/history).
	History any `json:"history"`
}

func FromFindingDetail(f domain.Finding) FindingDetailResponse {
	var changedBy *ActorRefResponse
	if f.StatusChangedBy != nil {
		changedBy = &ActorRefResponse{ID: f.StatusChangedBy.String()}
	}
	var reason string
	if f.StatusReason != nil {
		reason = *f.StatusReason
	}
	return FindingDetailResponse{
		FindingListItemResponse: FromFinding(f),
		StatusReason:            reason,
		StatusChangedBy:         changedBy,
		StatusChangedAt:         f.StatusChangedAt,
	}
}

// UpdateFindingStatusRequest matches `PATCH /findings/{id}/status`. Reason
// is validated server-side (reporting.ValidateTransition), not here —
// whether it's required at all depends on the target status (only
// suppressed needs it), which isn't expressible as a static validator tag.
type UpdateFindingStatusRequest struct {
	Status string `json:"status" validate:"required,oneof=open acknowledged suppressed false_positive fixed"`
	Reason string `json:"reason"`
}

// UpdateFindingStatusResponse matches `PATCH /findings/{id}/status`'s 200
// body.
type UpdateFindingStatusResponse struct {
	ID              string            `json:"id"`
	Status          string            `json:"status"`
	StatusReason    string            `json:"status_reason,omitempty"`
	StatusChangedBy *ActorRefResponse `json:"status_changed_by"`
	StatusChangedAt *time.Time        `json:"status_changed_at"`
}

// FromUpdatedFinding builds the response from the finding
// Service.UpdateFindingStatus just returned, plus its own separately
// resolved changedByName (the service already had a UserReader lookup to
// do for it — see that method's own doc comment).
func FromUpdatedFinding(f *domain.Finding, changedByName string) UpdateFindingStatusResponse {
	var changedBy *ActorRefResponse
	if f.StatusChangedBy != nil {
		changedBy = &ActorRefResponse{ID: f.StatusChangedBy.String(), DisplayName: changedByName}
	}
	var reason string
	if f.StatusReason != nil {
		reason = *f.StatusReason
	}
	return UpdateFindingStatusResponse{
		ID: f.ID.String(), Status: string(f.Status), StatusReason: reason,
		StatusChangedBy: changedBy, StatusChangedAt: f.StatusChangedAt,
	}
}

// FindingStatusHistoryEntryResponse is one entry in
// `GET /findings/{id}/history`.
type FindingStatusHistoryEntryResponse struct {
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	Reason     string    `json:"reason,omitempty"`
	ChangedBy  *string   `json:"changed_by"`
	ChangedAt  time.Time `json:"changed_at"`
}

// FindingStatusHistoryResponse matches `GET /findings/{id}/history`.
type FindingStatusHistoryResponse struct {
	Data []FindingStatusHistoryEntryResponse `json:"data"`
}

func FromStatusHistory(entries []reporting.StatusHistoryEntry) FindingStatusHistoryResponse {
	out := make([]FindingStatusHistoryEntryResponse, len(entries))
	for i, e := range entries {
		var changedBy *string
		if e.ChangedBy != nil {
			s := e.ChangedBy.String()
			changedBy = &s
		}
		out[i] = FindingStatusHistoryEntryResponse{
			FromStatus: string(e.FromStatus), ToStatus: string(e.ToStatus),
			Reason: e.Reason, ChangedBy: changedBy, ChangedAt: e.ChangedAt,
		}
	}
	return FindingStatusHistoryResponse{Data: out}
}
