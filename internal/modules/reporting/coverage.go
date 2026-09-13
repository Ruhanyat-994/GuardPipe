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

// pentestAsset mirrors internal/engines/pentest/pipeline.Asset's exported
// field shape after its own JSON round-trip through Stats — decoded here
// rather than importing that package directly, same dependency-rule reason
// PentestCoverage above gives.
type pentestAsset struct {
	Value     string `json:"Value"`
	Type      string `json:"Type"`
	Validated bool   `json:"Validated"`
}

// decodeValidatedSubdomains reads Stats["assets"] (every asset this run
// discovered, of every type) and returns just the validated subdomain
// names — the "assets" key already exists in a pentest job's Stats for
// Pentest v2's own correlation ingest (modules/pentest.ScanResultInputFromStats),
// this is a second, independent reader of the same field.
func decodeValidatedSubdomains(stats map[string]any) []string {
	raw, ok := stats["assets"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var assets []pentestAsset
	if err := json.Unmarshal(b, &assets); err != nil {
		return nil
	}
	var names []string
	for _, a := range assets {
		if a.Type == "subdomain" && a.Validated {
			names = append(names, a.Value)
		}
	}
	return names
}

// pentestValidatedDirectory mirrors
// internal/engines/pentest/pipeline.ValidatedDirectory's exported field
// shape, same JSON-round-trip decoding approach as pentestAsset above.
type pentestValidatedDirectory struct {
	URL string `json:"URL"`
}

// decodeValidatedDirectories reads Stats["validated_directories"] (every
// real, served path a disclosure/admin-path ffuf run found) and returns
// just the URLs, in discovery order.
func decodeValidatedDirectories(stats map[string]any) []string {
	raw, ok := stats["validated_directories"]
	if !ok {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var dirs []pentestValidatedDirectory
	if err := json.Unmarshal(b, &dirs); err != nil {
		return nil
	}
	urls := make([]string, 0, len(dirs))
	for _, d := range dirs {
		urls = append(urls, d.URL)
	}
	return urls
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
