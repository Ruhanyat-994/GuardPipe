// Package gemini is the sole real implementation of ai.LLMProvider
// (documentation/10-ai-integration.md §2, ADR-0004) — nothing outside this
// package knows Gemini exists (FR-AI-001). It is a minimal REST client
// against the generateContent endpoint, not a general Gemini SDK, matching
// adapters/github's precedent of a small hand-rolled client over an
// unneeded third-party dependency.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// defaultBaseURL is Gemini's public API base. defaultTimeout matches the
// 30s timeout documented in documentation/10-ai-integration.md §8's retry
// policy table.
const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"
	defaultTimeout = 30 * time.Second
	defaultTopP    = 0.95 // documentation/10-ai-integration.md §3
)

// Client implements ai.LLMProvider. baseURL/httpClient are overridable so
// tests point it at an httptest server instead of the real API
// (documentation/15-testing-strategy.md: "Tests never call Gemini ... live").
type Client struct {
	baseURL    string
	httpClient *http.Client
	keys       *KeyPool
}

// NewClient builds a Client. keys is the resolved rotation pool
// (config.AI.KeyPool()) — at least one key is required; NewClient itself
// doesn't reach into platform/config so it stays testable without a real
// Config value.
func NewClient(baseURL string, httpClient *http.Client, keys []string) (*Client, error) {
	if len(keys) == 0 {
		return nil, errors.New("gemini: at least one API key is required")
	}
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient, keys: NewKeyPool(keys)}, nil
}

func (c *Client) Name() string { return "gemini" }

// Complete sends one request, rotating through the key pool on a
// quota/rate-limit response (BUILD_GUIDE.md Phase 4): on 429/RESOURCE_EXHAUSTED
// it advances to the next key and retries the same request once, continuing
// until either a key succeeds or the whole pool has been tried. Any other
// error (invalid request, bad credential, upstream 5xx) returns immediately
// without rotating or retrying — a bad API key or a malformed request will
// not fix itself by trying a different key.
func (c *Client) Complete(ctx context.Context, req ai.LLMRequest) (ai.LLMResponse, error) {
	payload, err := json.Marshal(buildRequestBody(req))
	if err != nil {
		return ai.LLMResponse{}, apperrors.Internal(fmt.Errorf("gemini: encode request: %w", err))
	}

	var lastErr error
	for attempt := 0; attempt < c.keys.Len(); attempt++ {
		resp, err := c.doRequest(ctx, req.Model, c.keys.Current(), payload)
		if err == nil {
			return resp, nil
		}
		if !isQuotaError(err) {
			return ai.LLMResponse{}, err
		}
		lastErr = err
		c.keys.Advance()
	}

	return ai.LLMResponse{}, apperrors.External(
		"gemini.unavailable",
		"Gemini is unavailable: every configured API key is rate-limited or out of quota",
		lastErr,
	)
}

func (c *Client) doRequest(ctx context.Context, model, apiKey string, payload []byte) (ai.LLMResponse, error) {
	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, model, apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return ai.LLMResponse{}, apperrors.Internal(fmt.Errorf("gemini: build request: %w", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ai.LLMResponse{}, apperrors.External("gemini.unreachable", "could not reach the Gemini API", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return ai.LLMResponse{}, apperrors.External("gemini.invalid_response", "Gemini returned an unreadable response", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return ai.LLMResponse{}, classifyError(httpResp.StatusCode, bodyBytes)
	}

	var body generateContentResponse
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return ai.LLMResponse{}, apperrors.External("gemini.invalid_response", "Gemini returned an unparsable response", err)
	}
	text, err := extractText(body)
	if err != nil {
		return ai.LLMResponse{}, apperrors.External("gemini.invalid_response", err.Error(), err)
	}

	return ai.LLMResponse{
		Raw:       json.RawMessage(text),
		TokensIn:  body.UsageMetadata.PromptTokenCount,
		TokensOut: body.UsageMetadata.CandidatesTokenCount,
		Model:     model,
		Latency:   time.Since(start),
	}, nil
}

// isQuotaError reports whether err is the specific "this key is exhausted"
// case that Complete rotates the key pool for — everything else (a bad
// request, an invalid credential, an upstream 5xx) is not retried.
func isQuotaError(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindRateLimited
}

// --- Gemini wire format (generateContent) ---

type generateContentRequest struct {
	SystemInstruction *systemInstruction `json:"system_instruction,omitempty"`
	Contents          []requestContent   `json:"contents"`
	GenerationConfig  generationConfig   `json:"generationConfig"`
}

type systemInstruction struct {
	Parts []requestPart `json:"parts"`
}

type requestContent struct {
	Role  string        `json:"role"`
	Parts []requestPart `json:"parts"`
}

type requestPart struct {
	Text string `json:"text"`
}

type generationConfig struct {
	Temperature      float32         `json:"temperature"`
	TopP             float32         `json:"topP"`
	MaxOutputTokens  int             `json:"maxOutputTokens,omitempty"`
	ResponseMimeType string          `json:"responseMimeType,omitempty"`
	ResponseSchema   json.RawMessage `json:"responseSchema,omitempty"`
	ThinkingConfig   *thinkingConfig `json:"thinkingConfig,omitempty"`
}

// thinkingConfig disables Gemini 2.5's "thinking" tokens. Reproduced against
// the live API: thinking tokens count against generationConfig's own
// maxOutputTokens, and by default can consume the entire budget before the
// model ever writes its schema-constrained answer — the response then comes
// back truncated mid-object with finishReason "MAX_TOKENS", which fails
// schema validation (modules/ai/schema.go's Validate) as invalid JSON. None
// of GuardPipe's prompts need chain-of-thought for a structured
// classification/generation task, so this is set unconditionally rather than
// left at the provider default.
type thinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

func buildRequestBody(req ai.LLMRequest) generateContentRequest {
	body := generateContentRequest{
		Contents: []requestContent{{Role: "user", Parts: []requestPart{{Text: req.User}}}},
		GenerationConfig: generationConfig{
			Temperature:      req.Temperature,
			TopP:             defaultTopP,
			MaxOutputTokens:  req.MaxTokens,
			ResponseMimeType: "application/json",
			ThinkingConfig:   &thinkingConfig{ThinkingBudget: 0},
		},
	}
	if req.System != "" {
		body.SystemInstruction = &systemInstruction{Parts: []requestPart{{Text: req.System}}}
	}
	if len(req.Schema) > 0 {
		body.GenerationConfig.ResponseSchema = req.Schema
	}
	return body
}

type generateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func extractText(body generateContentResponse) (string, error) {
	if len(body.Candidates) == 0 || len(body.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("gemini: response had no candidates (likely blocked by a safety filter)")
	}
	return body.Candidates[0].Content.Parts[0].Text, nil
}

type geminiErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// classifyError maps a non-200 Gemini response to platform/errors, per
// documentation/10-ai-integration.md §8's retry-policy table: 429/
// RESOURCE_EXHAUSTED is rate-limited (Complete rotates keys for this one),
// 401/403 is an invalid credential (no retry — "a bad API key will not fix
// itself"), 400 is a bad request (no retry), 5xx is an upstream failure.
func classifyError(statusCode int, body []byte) error {
	var parsed geminiErrorBody
	_ = json.Unmarshal(body, &parsed) // best-effort; fall back to the status code alone if unparsable

	switch {
	case statusCode == http.StatusTooManyRequests || parsed.Error.Status == "RESOURCE_EXHAUSTED":
		return apperrors.RateLimited("gemini.quota_exhausted", "Gemini API quota exceeded for this key", 0)
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return apperrors.Unauthorized("gemini.credential_invalid", "the provided Gemini API key was rejected")
	case statusCode == http.StatusBadRequest:
		return apperrors.External("gemini.bad_request", fmt.Sprintf("Gemini rejected the request: %s", firstNonEmpty(parsed.Error.Message, string(body))), errors.New(parsed.Error.Status))
	case statusCode >= 500:
		return apperrors.External("gemini.upstream_error", "Gemini returned a server error", errors.New(parsed.Error.Status))
	default:
		return apperrors.External("gemini.request_failed", fmt.Sprintf("Gemini API returned unexpected status %d", statusCode), errors.New(parsed.Error.Status))
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
