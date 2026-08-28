package dto

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
)

func TestFromFindingDetail_OpenFinding_NoStatusChangeFields(t *testing.T) {
	f := domain.Finding{ID: uuid.New(), Title: "t", Status: domain.StatusOpen}
	got := FromFindingDetail(f)
	if got.StatusChangedBy != nil {
		t.Errorf("StatusChangedBy = %+v, want nil for a never-triaged finding", got.StatusChangedBy)
	}
	if got.StatusChangedAt != nil {
		t.Errorf("StatusChangedAt = %v, want nil", got.StatusChangedAt)
	}
	if got.AISuggestion != nil {
		t.Errorf("AISuggestion = %v, want nil (not built yet)", got.AISuggestion)
	}
	if got.History != nil {
		t.Errorf("History = %v, want nil (cross-scan correlation not built yet)", got.History)
	}
}

func TestFromFindingDetail_TriagedFinding_CarriesStatusChangeFields(t *testing.T) {
	changedBy := uuid.New()
	changedAt := time.Now()
	reason := "false positive per code review"
	f := domain.Finding{
		ID: uuid.New(), Title: "t", Status: domain.StatusFalsePositive,
		StatusReason: &reason, StatusChangedBy: &changedBy, StatusChangedAt: &changedAt,
	}
	got := FromFindingDetail(f)
	if got.StatusReason != reason {
		t.Errorf("StatusReason = %q, want %q", got.StatusReason, reason)
	}
	if got.StatusChangedBy == nil || got.StatusChangedBy.ID != changedBy.String() {
		t.Errorf("StatusChangedBy = %+v, want ID %s", got.StatusChangedBy, changedBy)
	}
	if got.StatusChangedAt == nil || !got.StatusChangedAt.Equal(changedAt) {
		t.Errorf("StatusChangedAt = %v, want %v", got.StatusChangedAt, changedAt)
	}
}

func TestFromUpdatedFinding_ResolvesDisplayName(t *testing.T) {
	changedBy := uuid.New()
	changedAt := time.Now()
	f := &domain.Finding{
		ID: uuid.New(), Status: domain.StatusAcknowledged,
		StatusChangedBy: &changedBy, StatusChangedAt: &changedAt,
	}
	got := FromUpdatedFinding(f, "Ada Lovelace")
	if got.StatusChangedBy == nil {
		t.Fatal("StatusChangedBy is nil")
	}
	if got.StatusChangedBy.DisplayName != "Ada Lovelace" {
		t.Errorf("DisplayName = %q, want Ada Lovelace", got.StatusChangedBy.DisplayName)
	}
	if got.Status != "acknowledged" {
		t.Errorf("Status = %q, want acknowledged", got.Status)
	}
}

func TestFromStatusHistory_MapsEveryEntryNewestFirst(t *testing.T) {
	userA := uuid.New()
	entries := []reporting.StatusHistoryEntry{
		{FromStatus: domain.StatusAcknowledged, ToStatus: domain.StatusSuppressed, Reason: "accepted risk, 30+ chars long here", ChangedBy: &userA, ChangedAt: time.Now()},
		{FromStatus: domain.StatusOpen, ToStatus: domain.StatusAcknowledged, ChangedAt: time.Now().Add(-time.Hour)},
	}
	got := FromStatusHistory(entries)
	if len(got.Data) != 2 {
		t.Fatalf("len(Data) = %d, want 2", len(got.Data))
	}
	if got.Data[0].FromStatus != "acknowledged" || got.Data[0].ToStatus != "suppressed" {
		t.Errorf("Data[0] = %+v", got.Data[0])
	}
	if got.Data[0].ChangedBy == nil || *got.Data[0].ChangedBy != userA.String() {
		t.Errorf("Data[0].ChangedBy = %v, want %s", got.Data[0].ChangedBy, userA)
	}
	if got.Data[1].ChangedBy != nil {
		t.Errorf("Data[1].ChangedBy = %v, want nil (no actor recorded)", got.Data[1].ChangedBy)
	}
}

func TestFromStatusHistory_Empty_ReturnsEmptySliceNotNil(t *testing.T) {
	got := FromStatusHistory(nil)
	if got.Data == nil {
		t.Error("Data is nil, want an empty (but non-nil) slice so the JSON body is [] not null")
	}
}
