package gemini_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/gemini"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

func successBody(text string) map[string]any {
	return map[string]any{
		"candidates": []map[string]any{
			{
				"content": map[string]any{
					"parts": []map[string]any{{"text": text}},
				},
				"finishReason": "STOP",
			},
		},
		"usageMetadata": map[string]any{
			"promptTokenCount":     42,
			"candidatesTokenCount": 17,
		},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(body))
}

func TestClient_Complete_Success(t *testing.T) {
	var gotKey, gotModel, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		require.Equal(t, "/models/gemini-2.5-flash:generateContent", r.URL.Path)
		gotModel = "gemini-2.5-flash"
		buf, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		gotBody = string(buf)
		writeJSON(t, w, http.StatusOK, successBody(`{"what":"x"}`))
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"only-key"})
	require.NoError(t, err)

	resp, err := client.Complete(context.Background(), ai.LLMRequest{
		System: "system text", User: "user text", Model: "gemini-2.5-flash", MaxTokens: 100, Temperature: 0.1,
	})
	require.NoError(t, err)
	require.Equal(t, `{"what":"x"}`, string(resp.Raw))
	require.Equal(t, 42, resp.TokensIn)
	require.Equal(t, 17, resp.TokensOut)
	require.Equal(t, "gemini-2.5-flash", resp.Model)
	require.Equal(t, "only-key", gotKey)
	require.Equal(t, "gemini-2.5-flash", gotModel)
	require.Contains(t, gotBody, "system text")
	require.Contains(t, gotBody, "user text")
}

func TestClient_Name(t *testing.T) {
	client, err := gemini.NewClient("http://example.invalid", nil, []string{"k"})
	require.NoError(t, err)
	require.Equal(t, "gemini", client.Name())
}

func TestNewClient_RequiresAtLeastOneKey(t *testing.T) {
	_, err := gemini.NewClient("http://example.invalid", nil, nil)
	require.Error(t, err)
}

// TestClient_Complete_RotatesToNextKeyOn429 is the Phase 4 multi-key
// rotation pool's core guarantee (BUILD_GUIDE.md Phase 4: "fake a 429 from
// key 1, assert it retries with key 2"): the first key hits quota, the
// adapter advances and retries the SAME request with the second key,
// in-process, without the caller seeing an error.
func TestClient_Complete_RotatesToNextKeyOn429(t *testing.T) {
	var keysSeen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		keysSeen = append(keysSeen, key)
		if key == "key-1" {
			writeJSON(t, w, http.StatusTooManyRequests, map[string]any{
				"error": map[string]any{"code": 429, "status": "RESOURCE_EXHAUSTED", "message": "quota exceeded"},
			})
			return
		}
		writeJSON(t, w, http.StatusOK, successBody(`{"ok":true}`))
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"key-1", "key-2"})
	require.NoError(t, err)

	resp, err := client.Complete(context.Background(), ai.LLMRequest{User: "u", Model: "gemini-2.5-flash"})
	require.NoError(t, err)
	require.Equal(t, `{"ok":true}`, string(resp.Raw))
	require.Equal(t, []string{"key-1", "key-2"}, keysSeen, "must try key-1, hit quota, then retry the same request with key-2")
}

// TestClient_Complete_AllKeysExhaustedReturnsUnavailable is the near-miss
// half — once every key in the pool has hit quota, Complete must fall
// through to a clean external error rather than retrying forever.
func TestClient_Complete_AllKeysExhaustedReturnsUnavailable(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeJSON(t, w, http.StatusTooManyRequests, map[string]any{
			"error": map[string]any{"code": 429, "status": "RESOURCE_EXHAUSTED", "message": "quota exceeded"},
		})
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"key-1", "key-2", "key-3"})
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), ai.LLMRequest{User: "u", Model: "gemini-2.5-flash"})
	require.Error(t, err)

	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, "gemini.unavailable", appErr.Code)
	require.Equal(t, 3, callCount, "must try every key in the pool exactly once, no more")
}

// TestClient_Complete_InvalidCredentialDoesNotRotate is the other
// near-miss: a 401/403 is not a quota problem, so rotating to a different
// key would just burn that key's quota on a request that was never going
// to succeed for a credential reason. Only one key is configured here
// specifically so a second call would be observable as a test failure via
// an out-of-range key index, but the call count is the direct assertion.
func TestClient_Complete_InvalidCredentialDoesNotRotate(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeJSON(t, w, http.StatusUnauthorized, map[string]any{
			"error": map[string]any{"code": 401, "status": "UNAUTHENTICATED", "message": "API key not valid"},
		})
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"key-1", "key-2"})
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), ai.LLMRequest{User: "u", Model: "gemini-2.5-flash"})
	require.Error(t, err)

	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindUnauthorized, appErr.Kind)
	require.Equal(t, "gemini.credential_invalid", appErr.Code)
	require.Equal(t, 1, callCount, "an invalid credential must not trigger key rotation")
}

func TestClient_Complete_SchemaAndResponseMimeTypeAreSentWhenSet(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		writeJSON(t, w, http.StatusOK, successBody(`{}`))
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"k"})
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), ai.LLMRequest{
		User:   "u",
		Model:  "gemini-2.5-flash",
		Schema: json.RawMessage(`{"type":"object"}`),
	})
	require.NoError(t, err)

	genConfig, ok := gotBody["generationConfig"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "application/json", genConfig["responseMimeType"])
	require.NotNil(t, genConfig["responseSchema"])
}

func TestClient_Complete_NoCandidatesIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"candidates": []any{}})
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"k"})
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), ai.LLMRequest{User: "u", Model: "gemini-2.5-flash"})
	require.Error(t, err)
}

func TestClient_Complete_ServerErrorIsExternalNotRotated(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client, err := gemini.NewClient(srv.URL, nil, []string{"key-1", "key-2"})
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), ai.LLMRequest{User: "u", Model: "gemini-2.5-flash"})
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindExternal, appErr.Kind)
	require.Equal(t, 1, callCount, "a 5xx is not a quota error and must not trigger rotation")
}
