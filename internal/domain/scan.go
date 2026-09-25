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

// TriggerSource records what started a scan — the `scans.trigger_source`
// column (migration 00027). Empty means "unknown", i.e. a scan created
// before that column existed.
type TriggerSource string

const (
	TriggerManual             TriggerSource = "manual"
	TriggerScheduled          TriggerSource = "scheduled"
	TriggerWebhookPush        TriggerSource = "webhook_push"
	TriggerWebhookPullRequest TriggerSource = "webhook_pull_request"
	// TriggerCLIWatch is reserved for the CLI git hook (not built yet).
	TriggerCLIWatch TriggerSource = "cli_watch"
)

// Scan is one run of one or more engines against one project. Field shapes
// mirror the `scans` table (documentation/06-database-design.md §4.9).
type Scan struct {
	ID          uuid.UUID
	ProjectID   uuid.UUID
	TriggeredBy *uuid.UUID // nil if the triggering user was later deleted
	// RequestedFromIP is the requesting client's IP at the moment this scan
	// was created (captured from the HTTP request, see
	// transport/http/handler/scan_handler.go's Create) — empty when unknown
	// (e.g. a pre-migration scan). Same accountability shape
	// target_attestations.source_ip already gives the one-time target
	// attestation, applied per scan-execution instead: reporting.Assembler
	// surfaces this as the exported report's "requested by / from IP"
	// section, GuardPipe's answer to "who actually ran this scan" the way
	// Nessus/Qualys-class tools stamp their own reports.
	RequestedFromIP  string
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
	// ScanNumber is this scan's 1-based ordinal among its project's own
	// scans (oldest = 1), computed at read time by ScanRepo rather than
	// stored — a human-readable "Scan #3" in place of a meaningless UUID
	// prefix in the UI.
	ScanNumber int
	// PentestConfig is nil for a scan with no pentest job; once one exists,
	// this is always populated (at minimum DefaultPentestScanConfig, never
	// left nil-meaning-Stealth) — see BUILD_GUIDE.md Phase 12's "default-to-
	// Stealth is literal, not just a UI default" rule.
	PentestConfig *PentestScanConfig
	// TriggerSource/TriggerRef/TriggerActor say where this scan came from:
	// TriggerRef is the branch or refs/pull/{n}/head a webhook named, and
	// TriggerActor the GitHub login that pushed. TriggeredBy (above) is still
	// the GuardPipe user accountable for the scan — for a webhook scan, the
	// person who turned live scanning on.
	TriggerSource TriggerSource
	TriggerRef    *string
	TriggerActor  *string
}
