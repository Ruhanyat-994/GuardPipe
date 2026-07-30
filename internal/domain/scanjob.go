package domain

import (
	"time"

	"github.com/google/uuid"
)

// JobStatus matches the `job_status` Postgres enum
// (documentation/06-database-design.md §3).
type JobStatus string

const (
	JobStatusQueued    JobStatus = "queued"
	JobStatusRunning   JobStatus = "running"
	JobStatusSucceeded JobStatus = "succeeded"
	JobStatusFailed    JobStatus = "failed"
	JobStatusSkipped   JobStatus = "skipped"
	JobStatusCancelled JobStatus = "cancelled"
)

func (s JobStatus) Valid() bool {
	switch s {
	case JobStatusQueued, JobStatusRunning, JobStatusSucceeded, JobStatusFailed, JobStatusSkipped, JobStatusCancelled:
		return true
	default:
		return false
	}
}

// ScanJob is one engine's execution within a Scan — the unit the worker pool
// claims and the reaper sweeps. Field shapes mirror the `scan_jobs` table
// (documentation/06-database-design.md §4.10).
type ScanJob struct {
	ID          uuid.UUID
	ScanID      uuid.UUID
	Engine      EngineID
	Status      JobStatus
	Attempt     int
	ClaimedAt   *time.Time // compared against by the reaper
	StartedAt   *time.Time
	FinishedAt  *time.Time
	ErrorReason *string // "timeout", "panic", "ai_unavailable", ...
	SkipReason  *string // "no_target_artifacts", "docker_unavailable", ...
	Stats       map[string]any
}
