// Package osv is the sole client for OSV.dev (documentation/05-module-specifications.md's
// "Advisory lookup" sequence diagram, BUILD_GUIDE.md Phase 5) — a minimal
// hand-rolled REST client against the two endpoints depscan's advisory
// lookup actually needs, matching adapters/gemini's precedent of a small
// client over an unneeded SDK. Nothing outside this package (and its caller,
// modules/advisory) knows OSV.dev exists.
package osv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// defaultBaseURL is OSV.dev's public API base
// (documentation/13-devops-and-environments.md §5.7's GUARDPIPE_OSV_API_URL
// default). defaultTimeout matches adapters/gemini's own default.
const (
	defaultBaseURL = "https://api.osv.dev"
	defaultTimeout = 30 * time.Second

	// MaxBatchSize is OSV.dev's documented cap on queries per /v1/querybatch
	// call (documentation/05-module-specifications.md's sequence diagram:
	// "POST /v1/querybatch (misses, ≤1000/batch)"). Chunking a larger
	// inventory into batches is the caller's job (modules/advisory) — this
	// client stays a dumb, correct transport.
	MaxBatchSize = 1000
)

// Client implements the two OSV.dev calls modules/advisory needs.
// baseURL/httpClient are overridable so tests point it at an httptest
// server instead of the real API (documentation/15-testing-strategy.md:
// "Tests never call ... OSV ... live").
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client. An empty baseURL/nil httpClient fall back to
// OSV.dev's real API and a default-timeout client respectively.
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// PackageQuery identifies one dependency version to check for advisories.
type PackageQuery struct {
	Ecosystem string // e.g. "npm", "PyPI", "Go", "Maven", "Packagist"
	Name      string
	Version   string
}

// QueryBatch looks up advisories for every query in one request and returns
// the matching vulnerability IDs in the same order as queries — result[i]
// is the ID list for queries[i]. OSV.dev's batch endpoint intentionally
// returns only IDs (plus a modified timestamp this client doesn't need);
// GetVulnerability fetches the full record for IDs the cache doesn't
// already have.
func (c *Client) QueryBatch(ctx context.Context, queries []PackageQuery) ([][]string, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if len(queries) > MaxBatchSize {
		return nil, apperrors.Internal(fmt.Errorf("osv: batch of %d exceeds the %d-query limit", len(queries), MaxBatchSize))
	}

	reqBody := queryBatchRequest{Queries: make([]queryBatchQuery, len(queries))}
	for i, q := range queries {
		reqBody.Queries[i] = queryBatchQuery{
			Package: queryPackage{Name: q.Name, Ecosystem: q.Ecosystem},
			Version: q.Version,
		}
	}

	var parsed queryBatchResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/querybatch", reqBody, &parsed); err != nil {
		return nil, err
	}

	out := make([][]string, len(queries))
	for i, r := range parsed.Results {
		ids := make([]string, len(r.Vulns))
		for j, v := range r.Vulns {
			ids[j] = v.ID
		}
		out[i] = ids
	}
	return out, nil
}

// GetVulnerability fetches the full advisory record for one OSV ID.
func (c *Client) GetVulnerability(ctx context.Context, id string) (Vulnerability, error) {
	var v Vulnerability
	if err := c.doJSON(ctx, http.MethodGet, "/v1/vulns/"+id, nil, &v); err != nil {
		return Vulnerability{}, err
	}
	return v, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return apperrors.Internal(fmt.Errorf("osv: encode request: %w", err))
		}
		bodyReader = bytes.NewReader(payload)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("osv: build request: %w", err))
	}
	if reqBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return apperrors.External("osv.unreachable", "could not reach OSV.dev", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return apperrors.External("osv.invalid_response", "OSV.dev returned an unreadable response", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return classifyError(httpResp.StatusCode, bodyBytes)
	}

	if respBody == nil {
		return nil
	}
	if err := json.Unmarshal(bodyBytes, respBody); err != nil {
		return apperrors.External("osv.invalid_response", "OSV.dev returned an unparsable response", err)
	}
	return nil
}

// classifyError maps a non-200 OSV.dev response to platform/errors. OSV.dev
// has no API key and no documented per-key rate-limit rotation (unlike
// Gemini) — every failure here is either "not found" or "upstream trouble,"
// both of which modules/advisory treats identically: keep the dependency
// inventory, mark advisories unavailable, and let the job succeed
// (documentation/05-module-specifications.md's depscan "Failure modes"
// table, FR-DEP-011).
func classifyError(statusCode int, body []byte) error {
	switch {
	case statusCode == http.StatusNotFound:
		return apperrors.NotFound("osv.not_found", "OSV.dev has no record for this ID")
	case statusCode == http.StatusTooManyRequests:
		return apperrors.RateLimited("osv.rate_limited", "OSV.dev rate-limited this request", 0)
	case statusCode >= 500:
		return apperrors.External("osv.upstream_error", "OSV.dev returned a server error", fmt.Errorf("status %d: %s", statusCode, string(body)))
	default:
		return apperrors.External("osv.request_failed", fmt.Sprintf("OSV.dev returned unexpected status %d", statusCode), fmt.Errorf("%s", string(body)))
	}
}

// --- OSV.dev wire format ---

type queryPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type queryBatchQuery struct {
	Package queryPackage `json:"package"`
	Version string       `json:"version"`
}

type queryBatchRequest struct {
	Queries []queryBatchQuery `json:"queries"`
}

type queryBatchResult struct {
	Vulns []struct {
		ID string `json:"id"`
	} `json:"vulns"`
}

type queryBatchResponse struct {
	Results []queryBatchResult `json:"results"`
}

// Vulnerability is the subset of OSV.dev's full vulnerability schema
// (https://ossf.github.io/osv-schema/) that modules/advisory needs to build
// a Finding: CVE aliasing, human summary, severity, and affected ranges for
// the "no fix available" check.
type Vulnerability struct {
	ID        string           `json:"id"`
	Summary   string           `json:"summary"`
	Details   string           `json:"details"`
	Aliases   []string         `json:"aliases"` // includes CVE-YYYY-NNNNN when known
	Severity  []VulnSeverity   `json:"severity"`
	Affected  []VulnAffected   `json:"affected"`
	References []VulnReference `json:"references"`
}

type VulnSeverity struct {
	Type  string `json:"type"`  // e.g. "CVSS_V3"
	Score string `json:"score"` // CVSS vector string
}

type VulnAffected struct {
	Package VulnAffectedPackage `json:"package"`
	Ranges  []VulnRange         `json:"ranges"`
	// Versions lists every individually-affected version OSV.dev enumerates
	// directly, alongside (not instead of) Ranges.
	Versions []string `json:"versions"`
}

type VulnAffectedPackage struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

type VulnRange struct {
	Type   string          `json:"type"` // "SEMVER" | "ECOSYSTEM" | "GIT"
	Events []VulnRangeEvent `json:"events"`
}

// VulnRangeEvent is one boundary in a range: exactly one of Introduced or
// Fixed is set. No Fixed event anywhere in Affected means no patched
// version exists yet — depscan.vuln.no-fix-available's exact signal.
type VulnRangeEvent struct {
	Introduced string `json:"introduced,omitempty"`
	Fixed      string `json:"fixed,omitempty"`
}

type VulnReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}
