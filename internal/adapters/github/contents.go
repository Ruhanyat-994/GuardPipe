package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// maxFileBytes caps GetFileContent — enough for any source file worth
// excerpting, small enough that a huge generated file can't be pulled into
// memory (or a prompt) by accident.
const maxFileBytes = 512 * 1024

// GetFileContent fetches one file's raw content at ref (a commit SHA or a
// branch) through the contents API. token may be empty for a public
// repository. Used by the finding assistant to give the patch/remediation
// prompts the real code around a finding, since scan workspaces are
// deleted once a scan finishes.
func (c *Client) GetFileContent(ctx context.Context, owner, name, path, ref, token string) (string, error) {
	escaped := make([]string, 0, 8)
	for _, seg := range strings.Split(strings.Trim(path, "/"), "/") {
		escaped = append(escaped, url.PathEscape(seg))
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", c.baseURL, url.PathEscape(owner), url.PathEscape(name), strings.Join(escaped, "/"))
	if ref != "" {
		u += "?ref=" + url.QueryEscape(ref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", apperrors.Internal(fmt.Errorf("github: build request: %w", err))
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", apperrors.External("github.unreachable", "could not reach the GitHub API", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes+1))
		if err != nil {
			return "", apperrors.External("github.invalid_response", "GitHub returned an unreadable response", err)
		}
		if len(body) > maxFileBytes {
			return "", apperrors.Unprocessable("github.file_too_large", "the file is too large to excerpt")
		}
		return string(body), nil
	case resp.StatusCode == http.StatusNotFound:
		return "", apperrors.NotFound("github.file_not_found", "file not found at that ref, or the repository is inaccessible")
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") != "0":
		return "", apperrors.Unauthorized("github.credential_invalid", "access to this repository was denied")
	case resp.StatusCode == http.StatusForbidden:
		return "", apperrors.RateLimited("github.rate_limited", "GitHub API rate limit exceeded", retryAfterSeconds(resp))
	default:
		return "", apperrors.External("github.request_failed", fmt.Sprintf("GitHub API returned unexpected status %d", resp.StatusCode), errors.New(resp.Status))
	}
}
