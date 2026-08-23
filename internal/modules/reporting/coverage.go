package reporting

import "encoding/json"

// decodeCoverage re-marshals the free-form "coverage" entry a pentest job's
// Stats map carries (persisted as JSONB, read back as map[string]any — see
// internal/store/repo/scan_job_repo.go) into a typed PentestCoverage. This
// round-trip is the accepted pattern for the rest of this codebase's
// free-form Stats fields; it also decouples reporting from importing
// engines/pentest just to reuse one struct (documentation/03-architecture-overview.md
// §6.2's dependency rule). Returns ok=false for any other engine's job,
// whose Stats never has a "coverage" key.
func decodeCoverage(stats map[string]any) (*PentestCoverage, bool) {
	raw, ok := stats["coverage"]
	if !ok {
		return nil, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var cov PentestCoverage
	if err := json.Unmarshal(b, &cov); err != nil {
		return nil, false
	}
	return &cov, true
}

func stringStat(stats map[string]any, key string) string {
	v, ok := stats[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
