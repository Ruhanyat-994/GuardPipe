package github_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

func TestVerifySignature(t *testing.T) {
	secret := []byte("s3cret")
	body := []byte(`{"ref":"refs/heads/main"}`)
	valid := github.Sign(secret, body)

	cases := []struct {
		name   string
		secret []byte
		body   []byte
		header string
		want   bool
	}{
		{"valid signature", secret, body, valid, true},
		// Near-misses: every one of these must be rejected.
		{"missing header", secret, body, "", false},
		{"wrong secret", []byte("other"), body, valid, false},
		{"tampered body", secret, []byte(`{"ref":"refs/heads/evil"}`), valid, false},
		{"sha1 scheme instead of sha256", secret, body, "sha1=" + valid[len("sha256="):], false},
		{"not hex", secret, body, "sha256=zzzz", false},
		{"truncated digest", secret, body, valid[:len(valid)-2], false},
		{"empty secret never verifies", nil, body, github.Sign(nil, body), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, github.VerifySignature(tc.secret, tc.body, tc.header))
		})
	}
}

func TestParsePushEvent(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/feature/login","after":"abc123","deleted":false,
		"repository":{"full_name":"acme/payments-api"},"sender":{"login":"octocat"}}`)
	e, err := github.ParsePushEvent(body)
	require.NoError(t, err)
	require.Equal(t, "feature/login", e.Branch())
	require.Equal(t, "abc123", e.After)
	require.Equal(t, "acme/payments-api", e.Repository.FullName)
	require.Equal(t, "octocat", e.Sender.Login)
}

func TestPushEvent_TagPushHasNoBranch(t *testing.T) {
	e := github.PushEvent{Ref: "refs/tags/v1.0.0"}
	require.Equal(t, "", e.Branch())
}

func TestParsePullRequestEvent_ForkDetection(t *testing.T) {
	sameRepo := []byte(`{"action":"opened","number":7,
		"pull_request":{"head":{"ref":"fix-bug","sha":"def456","repo":{"full_name":"acme/payments-api"}},"base":{"ref":"main"}},
		"repository":{"full_name":"acme/payments-api"},"sender":{"login":"octocat"}}`)
	e, err := github.ParsePullRequestEvent(sameRepo)
	require.NoError(t, err)
	require.False(t, e.FromFork())
	require.Equal(t, "fix-bug", e.PullRequest.Head.Ref)
	require.Equal(t, "main", e.PullRequest.Base.Ref)
	require.Equal(t, 7, e.Number)

	fork := []byte(`{"action":"opened","number":8,
		"pull_request":{"head":{"ref":"main","sha":"aaa","repo":{"full_name":"stranger/payments-api"}},"base":{"ref":"main"}},
		"repository":{"full_name":"acme/payments-api"}}`)
	e, err = github.ParsePullRequestEvent(fork)
	require.NoError(t, err)
	require.True(t, e.FromFork())
}

func TestClient_CreateHook_Success(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/repos/acme/payments-api/hooks", r.URL.Path)
		require.Equal(t, "Bearer ghp_token", r.Header.Get("Authorization"))
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &got))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":4242}`))
	}))
	defer srv.Close()

	id, err := github.NewClient(srv.URL, nil).CreateHook(context.Background(), "acme", "payments-api", "ghp_token",
		"https://guardpipe.example/api/v1/webhooks/github/abc", "hooksecret", []string{"push", "pull_request"})
	require.NoError(t, err)
	require.EqualValues(t, 4242, id)

	cfg := got["config"].(map[string]any)
	require.Equal(t, "https://guardpipe.example/api/v1/webhooks/github/abc", cfg["url"])
	require.Equal(t, "json", cfg["content_type"])
	require.Equal(t, "hooksecret", cfg["secret"])
	require.Equal(t, "0", cfg["insecure_ssl"])
	require.Equal(t, []any{"push", "pull_request"}, got["events"])
}

func TestClient_CreateHook_MissingScopeIsAClearError(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := github.NewClient(srv.URL, nil).CreateHook(context.Background(), "acme", "api", "ghp_ro", "https://x", "s", []string{"push"})
		srv.Close()

		var appErr *apperrors.Error
		require.ErrorAs(t, err, &appErr)
		require.Equal(t, "github.hook_permission_denied", appErr.Code, "status %d", status)
	}
}

func TestClient_DeleteHook_AlreadyGoneIsNotAnError(t *testing.T) {
	for _, status := range []int{http.StatusNoContent, http.StatusNotFound} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, http.MethodDelete, r.Method)
			require.Equal(t, "/repos/acme/api/hooks/99", r.URL.Path)
			w.WriteHeader(status)
		}))
		err := github.NewClient(srv.URL, nil).DeleteHook(context.Background(), "acme", "api", "ghp_token", 99)
		srv.Close()
		require.NoError(t, err, "status %d", status)
	}
}
