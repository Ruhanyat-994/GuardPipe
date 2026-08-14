// Package trivy is the sole client for the Trivy CLI, invoked as a
// short-lived sibling container (documentation/05-module-specifications.md
// §8, BUILD_GUIDE.md Phase 8, ADR-0012). engines/containerscan is the only
// caller — nothing outside this package and engines/containerscan knows
// Trivy exists.
//
// Unlike adapters/sonarqube, there is no persistent server and no REST API
// to poll: Trivy runs synchronously, writes one JSON report to stdout, and
// exits. So the split here is report.go (parsing that JSON into Go types)
// and scanner.go (launching the container and capturing its stdout) rather
// than sonarqube's "REST client" / "scanner" split.
package trivy

import "encoding/json"

// Report is the subset of Trivy's JSON report format
// (https://trivy.dev, `--format json`) engines/containerscan needs, shared
// by both `trivy config` and `trivy image` output — they use the same
// top-level Results shape, distinguished by which fields are populated per
// result.
type Report struct {
	Results []Result `json:"Results"`
}

// Result is one scan target within a report — one Dockerfile for a config
// scan, or one of (OS packages / language packages / secrets) for an image
// scan. Class distinguishes which: "config", "os-pkgs", "lang-pkgs",
// "secret".
type Result struct {
	Target            string          `json:"Target"`
	Class             string          `json:"Class"`
	Type              string          `json:"Type"`
	Misconfigurations []Misconfig     `json:"Misconfigurations"`
	Vulnerabilities   []Vulnerability `json:"Vulnerabilities"`
	Secrets           []Secret        `json:"Secrets"`
}

// Misconfig is one Dockerfile/IaC misconfiguration finding
// (`trivy config`'s output, or the "config" class within an image scan).
type Misconfig struct {
	ID            string `json:"ID"`
	Title         string `json:"Title"`
	Message       string `json:"Message"`
	Resolution    string `json:"Resolution"`
	Severity      string `json:"Severity"` // CRITICAL | HIGH | MEDIUM | LOW
	CauseMetadata struct {
		StartLine int `json:"StartLine"`
		EndLine   int `json:"EndLine"`
	} `json:"CauseMetadata"`
}

// Vulnerability is one CVE match against an OS or language package
// (`trivy image --scanners vuln`'s output).
type Vulnerability struct {
	VulnerabilityID  string   `json:"VulnerabilityID"`
	PkgName          string   `json:"PkgName"`
	InstalledVersion string   `json:"InstalledVersion"`
	FixedVersion     string   `json:"FixedVersion"`
	Severity         string   `json:"Severity"`
	Title            string   `json:"Title"`
	Description      string   `json:"Description"`
	CweIDs           []string `json:"CweIDs"`
	PrimaryURL       string   `json:"PrimaryURL"`
	Layer            Layer    `json:"Layer"`
}

// Layer identifies which image layer a vulnerability or secret was found
// in — populated for image scans, empty for config scans.
type Layer struct {
	Digest string `json:"Digest"`
}

// Secret is one secret match inside an image layer — a file's contents or a
// layer's build-command history (`trivy image --scanners secret`'s output).
type Secret struct {
	RuleID    string `json:"RuleID"`
	Category  string `json:"Category"`
	Title     string `json:"Title"`
	Severity  string `json:"Severity"`
	StartLine int    `json:"StartLine"`
	EndLine   int    `json:"EndLine"`
	Layer     Layer  `json:"Layer"`
}

// ParseReport unmarshals raw Trivy JSON output. A malformed/empty report
// (Trivy printed nothing to stdout, e.g. because it errored before scanning
// began) is the caller's concern via the returned error, not silently
// treated as "zero findings."
func ParseReport(raw []byte) (Report, error) {
	var r Report
	if err := json.Unmarshal(raw, &r); err != nil {
		return Report{}, err
	}
	return r, nil
}
