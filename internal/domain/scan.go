package domain

import (
	"time"

	"github.com/google/uuid"
)

// ScanType matches the `scan_type` Postgres enum
// (documentation/06-database-design.md §3).
type ScanType string

const (
	ScanTypeFullSupplyChain ScanType = "full_supply_chain"
	ScanTypePartial         ScanType = "partial"
	ScanTypePentestOnly     ScanType = "pentest_only"
)

func (t ScanType) Valid() bool {
	switch t {
	case ScanTypeFullSupplyChain, ScanTypePartial, ScanTypePentestOnly:
		return true
	default:
		return false
	}
}

// ScanStatus matches the `scan_status` Postgres enum
// (documentation/06-database-design.md §3).
type ScanStatus string

const (
	ScanStatusQueued    ScanStatus = "queued"
	ScanStatusRunning   ScanStatus = "running"
	ScanStatusCompleted ScanStatus = "completed"
	ScanStatusFailed    ScanStatus = "failed"
	ScanStatusCancelled ScanStatus = "cancelled"
)

func (s ScanStatus) Valid() bool {
	switch s {
	case ScanStatusQueued, ScanStatusRunning, ScanStatusCompleted, ScanStatusFailed, ScanStatusCancelled:
		return true
	default:
		return false
	}
}

// Scan is one run of one or more engines against one project. Field shapes
// mirror the `scans` table (documentation/06-database-design.md §4.9).
type Scan struct {
	ID               uuid.UUID
	ProjectID        uuid.UUID
	TriggeredBy      *uuid.UUID // nil if the triggering user was later deleted
	Type             ScanType
	Status           ScanStatus
	RequestedEngines []EngineID
	CommitSHA        *string
	Branch           *string
	CancelRequested  bool
	ErrorReason      *string // only set on whole-scan failure
	QueuedAt         time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
	// FindingCounts is the denormalised per-severity count written once, in
	// the same transaction that finalises the scan, so the scan list can
	// render severity badges without an N+1 aggregate query.
	FindingCounts map[Severity]int
}
