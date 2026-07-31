package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// defaultTimeout bounds a single GitHub API call — this is a metadata
// lookup on the request path (project creation), not a long-running clone.
const defaultTimeout = 10 * time.Second

// RepoMetadata is what GetRepository returns, matching the fields
// `repositories` needs (documentation/06-database-design.md §4.5):
// default branch, visibility, and size for the pre-clone size guard.
type RepoMetadata struct {
	DefaultBranch string
	IsPrivate     bool
	SizeKB        int64
}

// Client is a minimal GitHub REST client — just enough to validate a
// repository and read the metadata needed to store it
// (documentation/05-module-specifications.md §4). It is not a general
// GitHub SDK.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a Client. baseURL is GUARDPIPE_GITHUB_API_URL
// (documentation/13-devops-and-environments.md §5.7), normally
// "https://api.github.com" — configurable so tests can point it at an
// httptest server instead of the real GitHub API (documentation's testing
// philosophy: "Tests never call Gemini, OSV, or GitHub live").
func NewClient(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

// apiRepo mirrors the subset of GitHub's repository JSON shape this client
// reads (https://docs.github.com/en/rest/repos/repos#get-a-repository).
type apiRepo struct {
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	SizeKB        int64  `json:"size"`
}

// GetRepository fetches owner/name's metadata. token is the caller's
// decrypted PAT and may be empty for a public repository — a request is
// sent unauthenticated in that case, at GitHub's lower unauthenticated rate
// limit (documentation/19-client-journey-and-pricing-model.md §6.2).
func (c *Client) GetRepository(ctx context.Context, owner, name, token string) (*RepoMetadata, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("github: build request: %w", err))
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, apperrors.External("github.unreachable", "could not reach the GitHub API", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		var body apiRepo
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, apperrors.External("github.invalid_response", "GitHub returned an unreadable response", err)
		}
		return &RepoMetadata{DefaultBranch: body.DefaultBranch, IsPrivate: body.Private, SizeKB: body.SizeKB}, nil

	case resp.StatusCode == http.StatusNotFound:
		// A private repo GuardPipe has no valid credential for also
		// surfaces as 404, by GitHub's own design (it never confirms a
		// private repo's existence to an unauthorized caller) — from the
		// caller's side these two cases are indistinguishable and both
		// mean "attach a valid credential."
		return nil, apperrors.NotFound("github.repository_not_found", "repository not found, or private and inaccessible with the given credential")

	case resp.StatusCode == http.StatusUnauthorized:
		return nil, apperrors.Unauthorized("github.credential_invalid", "the provided GitHub credential was rejected")

	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return nil, apperrors.RateLimited("github.rate_limited", "GitHub API rate limit exceeded", retryAfterSeconds(resp))

	case resp.StatusCode == http.StatusForbidden:
		return nil, apperrors.Unauthorized("github.credential_invalid", "access to this repository was denied")

	default:
		return nil, apperrors.External("github.request_failed", fmt.Sprintf("GitHub API returned unexpected status %d", resp.StatusCode), errors.New(resp.Status))
	}
}

func retryAfterSeconds(resp *http.Response) int {
	reset := resp.Header.Get("X-RateLimit-Reset")
	if reset == "" {
		return 60
	}
	var epoch int64
	if _, err := fmt.Sscanf(reset, "%d", &epoch); err != nil {
		return 60
	}
	secs := int(time.Until(time.Unix(epoch, 0)).Seconds())
	if secs < 1 {
		return 1
	}
	return secs
}
