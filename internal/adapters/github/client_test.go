package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

func TestClient_GetRepository_Success(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		require.Equal(t, "/repos/acme/payments-api", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"default_branch": "main",
			"private":        true,
			"size":           18432,
		})
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	meta, err := client.GetRepository(context.Background(), "acme", "payments-api", "ghp_secrettoken")
	require.NoError(t, err)
	require.Equal(t, "main", meta.DefaultBranch)
	require.True(t, meta.IsPrivate)
	require.EqualValues(t, 18432, meta.SizeKB)
	require.Equal(t, "Bearer ghp_secrettoken", gotAuth)
}

func TestClient_GetRepository_PublicNoToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"default_branch": "main", "private": false, "size": 100})
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	meta, err := client.GetRepository(context.Background(), "acme", "public-repo", "")
	require.NoError(t, err)
	require.False(t, meta.IsPrivate)
	// Near-miss: an empty token must not send a bogus Authorization header.
	require.Empty(t, gotAuth)
}

func TestClient_GetRepository_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	_, err := client.GetRepository(context.Background(), "acme", "missing", "")
	requireAppErrorKind(t, err, apperrors.KindNotFound, "github.repository_not_found")
}

func TestClient_GetRepository_InvalidCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	_, err := client.GetRepository(context.Background(), "acme", "payments-api", "bad-token")
	requireAppErrorKind(t, err, apperrors.KindUnauthorized, "github.credential_invalid")
}

func TestClient_GetRepository_RateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "9999999999")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	_, err := client.GetRepository(context.Background(), "acme", "payments-api", "")
	requireAppErrorKind(t, err, apperrors.KindRateLimited, "github.rate_limited")
}

func TestClient_GetRepository_ForbiddenWithoutRateLimit(t *testing.T) {
	// Near-miss: a plain 403 (no X-RateLimit-Remaining: 0) is a permission
	// problem, not a rate limit — must not be misclassified.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	_, err := client.GetRepository(context.Background(), "acme", "payments-api", "token")
	requireAppErrorKind(t, err, apperrors.KindUnauthorized, "github.credential_invalid")
}

func TestClient_GetRepository_UpstreamServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := github.NewClient(srv.URL, nil)
	_, err := client.GetRepository(context.Background(), "acme", "payments-api", "")
	requireAppErrorKind(t, err, apperrors.KindExternal, "github.request_failed")
}

func requireAppErrorKind(t *testing.T, err error, wantKind apperrors.Kind, wantCode string) {
	t.Helper()
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, wantKind, appErr.Kind)
	require.Equal(t, wantCode, appErr.Code)
}
