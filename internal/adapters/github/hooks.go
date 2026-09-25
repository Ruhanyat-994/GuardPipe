package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// hookPermissionHint is what a client sees when their PAT can read the
// repository but can't manage its webhooks — the most likely failure when
// turning live scanning on, since Phase 3 only ever needed read access.
const hookPermissionHint = "the attached GitHub token can't manage this repository's webhooks — it needs the admin:repo_hook scope (classic PAT) or Webhooks: read & write (fine-grained PAT)"

type createHookRequest struct {
	Name   string           `json:"name"`
	Active bool             `json:"active"`
	Events []string         `json:"events"`
	Config createHookConfig `json:"config"`
}

type createHookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret"`
	InsecureSSL string `json:"insecure_ssl"`
}

// CreateHook registers a repository webhook that delivers events to url,
// signed with secret, and returns GitHub's hook ID (needed to delete it
// later). https://docs.github.com/en/rest/repos/webhooks#create-a-repository-webhook
func (c *Client) CreateHook(ctx context.Context, owner, name, token, url, secret string, events []string) (int64, error) {
	payload, err := json.Marshal(createHookRequest{
		Name: "web", Active: true, Events: events,
		Config: createHookConfig{URL: url, ContentType: "json", Secret: secret, InsecureSSL: "0"},
	})
	if err != nil {
		return 0, apperrors.Internal(fmt.Errorf("github: encode hook request: %w", err))
	}

	resp, err := c.do(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/%s/hooks", c.baseURL, owner, name), token, payload)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		var body struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.ID == 0 {
			return 0, apperrors.External("github.invalid_response", "GitHub returned an unreadable response", err)
		}
		return body.ID, nil
	case http.StatusUnprocessableEntity:
		// Most commonly "Hook already exists on this repository" — someone
		// registered the same URL by hand, or a previous disable failed
		// halfway.
		return 0, apperrors.Conflict("github.hook_rejected", "GitHub rejected the webhook (a webhook with this URL may already exist on the repository)")
	default:
		return 0, hookError(resp)
	}
}

// DeleteHook removes a repository webhook. A hook that's already gone
// (deleted by hand on GitHub) is not an error — the caller wanted it gone.
// https://docs.github.com/en/rest/repos/webhooks#delete-a-repository-webhook
func (c *Client) DeleteHook(ctx context.Context, owner, name, token string, hookID int64) error {
	resp, err := c.do(ctx, http.MethodDelete, fmt.Sprintf("%s/repos/%s/%s/hooks/%d", c.baseURL, owner, name, hookID), token, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return hookError(resp)
	}
}

func (c *Client) do(ctx context.Context, method, url, token string, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("github: build request: %w", err))
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, apperrors.External("github.unreachable", "could not reach the GitHub API", err)
	}
	return resp, nil
}

// hookError maps a failed hook-management response. GitHub answers 404
// (not 403) when a token lacks the webhook scope on a private repository,
// so both mean "your token can't do this" from the caller's side.
func hookError(resp *http.Response) error {
	switch {
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return apperrors.RateLimited("github.rate_limited", "GitHub API rate limit exceeded", retryAfterSeconds(resp))
	case resp.StatusCode == http.StatusUnauthorized:
		return apperrors.Unauthorized("github.credential_invalid", "the attached GitHub credential was rejected")
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusNotFound:
		return apperrors.Unprocessable("github.hook_permission_denied", hookPermissionHint)
	default:
		return apperrors.External("github.request_failed", fmt.Sprintf("GitHub API returned unexpected status %d", resp.StatusCode), errors.New(resp.Status))
	}
}
