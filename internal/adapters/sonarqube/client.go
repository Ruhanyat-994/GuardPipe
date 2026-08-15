// Package sonarqube is the sole client for a self-hosted SonarQube
// Community Edition instance (documentation/05-module-specifications.md §6,
// BUILD_GUIDE.md Phase 7, ADR-0011). engines/codescan is the only caller —
// nothing outside this package and engines/codescan knows SonarQube exists.
//
// Two concerns live here, split across two files: client.go is a pure Web
// API REST client (mirrors adapters/osv's shape — overridable
// baseURL/httpClient so tests point it at an httptest server, never live
// SonarQube in CI); scanner.go launches the sonar-scanner-cli container that
// actually performs analysis, since that needs Docker, not just HTTP.
package sonarqube

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

const (
	defaultTimeout = 30 * time.Second

	// pageSize is the page size used for every paginated search — SonarQube's
	// own documented maximum per page.
	pageSize = 500
)

// Client implements the handful of SonarQube Web API calls engines/codescan
// needs: poll a background analysis task, read issues/hotspots for a
// project, and read a rule's own description text (used as Finding.Remediation
// — see documentation/05-module-specifications.md §6).
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient builds a Client against a self-hosted SonarQube instance at
// baseURL (GUARDPIPE_SONARQUBE_API_URL) authenticating with token
// (GUARDPIPE_SONARQUBE_TOKEN, a user token generated once in SonarQube's own
// UI — BUILD_GUIDE.md Phase 7). A nil httpClient falls back to a
// default-timeout client.
func NewClient(baseURL, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, token: token, httpClient: httpClient}
}

// TaskStatus is one of SonarQube's background-task states
// (GET /api/ce/task).
type TaskStatus string

const (
	TaskPending    TaskStatus = "PENDING"
	TaskInProgress TaskStatus = "IN_PROGRESS"
	TaskSuccess    TaskStatus = "SUCCESS"
	TaskFailed     TaskStatus = "FAILED"
	TaskCanceled   TaskStatus = "CANCELED"
)

// Task is the subset of GET /api/ce/task's response engines/codescan needs:
// whether the background analysis finished, and if so, which analysis to
// read issues/hotspots for.
type Task struct {
	Status     TaskStatus
	AnalysisID string
	ErrorMsg   string
}

// GetTask reads the current status of one background analysis task —
// engines/codescan polls this in a bounded loop
// (GUARDPIPE_SONARQUBE_ANALYSIS_TIMEOUT) until it leaves PENDING/IN_PROGRESS.
func (c *Client) GetTask(ctx context.Context, taskID string) (Task, error) {
	var parsed struct {
		Task struct {
			Status       string `json:"status"`
			AnalysisID   string `json:"analysisId"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"task"`
	}
	q := url.Values{"id": {taskID}}
	if err := c.doJSON(ctx, http.MethodGet, "/api/ce/task", q, &parsed); err != nil {
		return Task{}, err
	}
	return Task{
		Status:     TaskStatus(parsed.Task.Status),
		AnalysisID: parsed.Task.AnalysisID,
		ErrorMsg:   parsed.Task.ErrorMessage,
	}, nil
}

// QualityGateStatus reads whether analysisId's project passed its quality
// gate — engines/codescan uses this only as a "the analysis actually landed
// and SonarQube considers it complete" confirmation, not as a pass/fail gate
// of its own (that's GuardPipe's own scoring module's job, not SonarQube's).
func (c *Client) QualityGateStatus(ctx context.Context, analysisID string) (string, error) {
	var parsed struct {
		ProjectStatus struct {
			Status string `json:"status"`
		} `json:"projectStatus"`
	}
	q := url.Values{"analysisId": {analysisID}}
	if err := c.doJSON(ctx, http.MethodGet, "/api/qualitygates/project_status", q, &parsed); err != nil {
		return "", err
	}
	return parsed.ProjectStatus.Status, nil
}

// Issue is one row of GET /api/issues/search — a rule violation SonarQube
// classified as type=VULNERABILITY (BUG and CODE_SMELL are also possible
// types on the raw endpoint; engines/codescan always queries with
// types=VULNERABILITY, so an Issue here is already a vulnerability, never a
// code smell — see the "Security-relevance filter" in
// documentation/05-module-specifications.md §6).
type Issue struct {
	Key       string
	RuleKey   string
	Severity  string // BLOCKER | CRITICAL | MAJOR | MINOR | INFO
	Component string // "<projectKey>:<path>"
	Message   string
	Line      int
	LineEnd   int
}

// SearchIssues returns every VULNERABILITY-type issue for projectKey,
// paginating through SonarQube's own page size automatically.
func (c *Client) SearchIssues(ctx context.Context, projectKey string) ([]Issue, error) {
	var out []Issue
	page := 1
	for {
		var parsed struct {
			Issues []struct {
				Key       string `json:"key"`
				Rule      string `json:"rule"`
				Severity  string `json:"severity"`
				Component string `json:"component"`
				Message   string `json:"message"`
				Line      int    `json:"line"`
				TextRange struct {
					StartLine int `json:"startLine"`
					EndLine   int `json:"endLine"`
				} `json:"textRange"`
			} `json:"issues"`
			Paging struct {
				PageIndex int `json:"pageIndex"`
				PageSize  int `json:"pageSize"`
				Total     int `json:"total"`
			} `json:"paging"`
		}
		q := url.Values{
			"componentKeys": {projectKey},
			"types":         {"VULNERABILITY"},
			// resolved=false is load-bearing: SonarQube's own incremental
			// analysis correctly closes an issue once its next scan no
			// longer finds it (resolution=FIXED, status=CLOSED) — but
			// /api/issues/search returns issues regardless of resolution
			// unless told otherwise, so without this a fixed vulnerability
			// keeps reappearing as an "open" Finding on every subsequent
			// scan forever, even though SonarQube itself already knows it's
			// gone. Verified directly against a real fixed-and-rescanned
			// issue before this fix landed.
			"resolved": {"false"},
			"ps":       {strconv.Itoa(pageSize)},
			"p":        {strconv.Itoa(page)},
		}
		if err := c.doJSON(ctx, http.MethodGet, "/api/issues/search", q, &parsed); err != nil {
			return nil, err
		}
		for _, i := range parsed.Issues {
			startLine, endLine := i.Line, i.Line
			if i.TextRange.StartLine != 0 {
				startLine, endLine = i.TextRange.StartLine, i.TextRange.EndLine
			}
			out = append(out, Issue{
				Key: i.Key, RuleKey: i.Rule, Severity: i.Severity, Component: i.Component,
				Message: i.Message, Line: startLine, LineEnd: endLine,
			})
		}
		if page*parsed.Paging.PageSize >= parsed.Paging.Total || len(parsed.Issues) == 0 {
			break
		}
		page++
	}
	return out, nil
}

// Hotspot is one row of GET /api/hotspots/search — code SonarQube flags as
// "needs manual security review," not a confirmed vulnerability
// (engines/codescan's Confidence mapping reflects that — see
// documentation/05-module-specifications.md §6's normalisation table).
type Hotspot struct {
	Key                      string
	RuleKey                  string
	Component                string
	Message                  string
	Line                     int
	VulnerabilityProbability string // HIGH | MEDIUM | LOW
}

// SearchHotspots returns every security hotspot for projectKey, paginating
// automatically.
func (c *Client) SearchHotspots(ctx context.Context, projectKey string) ([]Hotspot, error) {
	var out []Hotspot
	page := 1
	for {
		var parsed struct {
			Hotspots []struct {
				Key                      string `json:"key"`
				RuleKey                  string `json:"ruleKey"`
				Component                string `json:"component"`
				Message                  string `json:"message"`
				Line                     int    `json:"line"`
				VulnerabilityProbability string `json:"vulnerabilityProbability"`
			} `json:"hotspots"`
			Paging struct {
				PageIndex int `json:"pageIndex"`
				PageSize  int `json:"pageSize"`
				Total     int `json:"total"`
			} `json:"paging"`
		}
		q := url.Values{
			"projectKey": {projectKey},
			// status=TO_REVIEW excludes hotspots already REVIEWED (marked
			// either FIXED or SAFE) — same reasoning as SearchIssues'
			// resolved=false: a reviewed hotspot must not keep reappearing
			// as a Finding on every later scan.
			"status": {"TO_REVIEW"},
			"ps":     {strconv.Itoa(pageSize)},
			"p":      {strconv.Itoa(page)},
		}
		if err := c.doJSON(ctx, http.MethodGet, "/api/hotspots/search", q, &parsed); err != nil {
			return nil, err
		}
		for _, h := range parsed.Hotspots {
			out = append(out, Hotspot{
				Key: h.Key, RuleKey: h.RuleKey, Component: h.Component,
				Message: h.Message, Line: h.Line, VulnerabilityProbability: h.VulnerabilityProbability,
			})
		}
		if page*parsed.Paging.PageSize >= parsed.Paging.Total || len(parsed.Hotspots) == 0 {
			break
		}
		page++
	}
	return out, nil
}

// RuleInfo is the subset of GET /api/rules/show engines/codescan needs to
// build a Finding's remediation text and CWE list
// (documentation/05-module-specifications.md §6: "Remediation... SonarQube's
// own rule description, not hand-written GuardPipe copy").
type RuleInfo struct {
	Key             string
	Name            string
	RemediationHTML string // rule.htmlDesc's "How to fix it" section, SonarQube-authored
	CWE             []string
}

// GetRule reads one rule's own metadata by its key (e.g. "java:S2076").
//
// SonarQube versions differ on how a rule's description arrives: older ones
// return one flat rule.htmlDesc string; current ones (verified against this
// project's own sonarqube:community, 26.8.0) split it into
// rule.descriptionSections instead — htmlDesc is simply absent — with the
// actual "how to fix it" guidance under the section keyed "how_to_fix". Both
// shapes are handled here so RemediationHTML is never silently empty just
// because the server happens to be on the newer shape.
func (c *Client) GetRule(ctx context.Context, ruleKey string) (RuleInfo, error) {
	var parsed struct {
		Rule struct {
			Key                 string   `json:"key"`
			Name                string   `json:"name"`
			HTMLDesc            string   `json:"htmlDesc"`
			CWE                 []string `json:"cwe"`
			DescriptionSections []struct {
				Key     string `json:"key"`
				Content string `json:"content"`
			} `json:"descriptionSections"`
		} `json:"rule"`
	}
	q := url.Values{"key": {ruleKey}}
	if err := c.doJSON(ctx, http.MethodGet, "/api/rules/show", q, &parsed); err != nil {
		return RuleInfo{}, err
	}
	return RuleInfo{
		Key: parsed.Rule.Key, Name: parsed.Rule.Name,
		RemediationHTML: firstNonEmptyHTML(parsed.Rule.HTMLDesc, parsed.Rule.DescriptionSections),
		CWE:             parsed.Rule.CWE,
	}, nil
}

// firstNonEmptyHTML prefers the legacy flat htmlDesc when a server still
// sends it; otherwise it's the "how_to_fix" section's own content, and
// failing that, whatever the "introduction" section says — better than no
// remediation text at all.
func firstNonEmptyHTML(htmlDesc string, sections []struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}) string {
	if htmlDesc != "" {
		return htmlDesc
	}
	var introduction string
	for _, s := range sections {
		if s.Key == "how_to_fix" && s.Content != "" {
			return s.Content
		}
		if s.Key == "introduction" {
			introduction = s.Content
		}
	}
	return introduction
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, respBody any) error {
	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("sonarqube: build request: %w", err))
	}
	// SonarQube user tokens authenticate as HTTP Basic with the token as the
	// username and an empty password — stable across CE versions, unlike
	// Bearer support which varies by version.
	httpReq.SetBasicAuth(c.token, "")

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return apperrors.External("sonarqube.unreachable", "could not reach SonarQube", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return apperrors.External("sonarqube.invalid_response", "SonarQube returned an unreadable response", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return classifyError(httpResp.StatusCode, bodyBytes)
	}

	if respBody == nil {
		return nil
	}
	if err := json.Unmarshal(bodyBytes, respBody); err != nil {
		return apperrors.External("sonarqube.invalid_response", "SonarQube returned an unparsable response", err)
	}
	return nil
}

// classifyError maps a non-200 SonarQube response to platform/errors.
// engines/codescan treats every one of these identically: fail the
// codescan job only, never the whole scan
// (documentation/05-module-specifications.md §6's Failure modes table,
// FR-ORC-006/NFR-REL-001).
func classifyError(statusCode int, body []byte) error {
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return apperrors.External("sonarqube.unauthorized", "SonarQube rejected the configured token", fmt.Errorf("status %d", statusCode))
	case statusCode == http.StatusNotFound:
		return apperrors.NotFound("sonarqube.not_found", "SonarQube has no record for this key")
	case statusCode == http.StatusTooManyRequests:
		return apperrors.RateLimited("sonarqube.rate_limited", "SonarQube rate-limited this request", 0)
	case statusCode >= 500:
		return apperrors.External("sonarqube.upstream_error", "SonarQube returned a server error", fmt.Errorf("status %d: %s", statusCode, string(body)))
	default:
		return apperrors.External("sonarqube.request_failed", fmt.Sprintf("SonarQube returned unexpected status %d", statusCode), fmt.Errorf("%s", string(body)))
	}
}
