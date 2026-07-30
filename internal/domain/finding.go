package domain

import "github.com/google/uuid"

// Finding is the one type every engine, however different its input,
// produces. It is the heart of the system — the normalisation that makes a
// single dashboard and a single risk score possible across seven
// unrelated analysis techniques (documentation/03-architecture-overview.md
// §7.1).
//
// Engines emit Findings; they never persist them. The orchestrator persists
// them, inside one transaction per job (documentation/03-architecture-overview.md
// §6.3) — so Finding carries no job/project bookkeeping fields of its own.
type Finding struct {
	ID          uuid.UUID
	ScanID      uuid.UUID
	Engine      EngineID
	RuleID      string // "codescan.injection.sql-string-concat" — permanent, see RuleMeta
	Fingerprint string // SHA-256 hex, stable across scans — see platform/id.Fingerprint

	Title       string // one line, human-first
	Description string // what and why, plain language before jargon
	Severity    Severity
	Confidence  Confidence

	CWE        []string // ["CWE-89"]
	CVE        []string // ["CVE-2024-1234"]
	CVSSScore  *float64
	CVSSVector *string
	OWASP      []string // ["A03:2021"]

	Location    Location
	Evidence    []Evidence
	Remediation string // deterministic guidance — must stand alone without AI

	Status   Status
	Metadata map[string]any
}
