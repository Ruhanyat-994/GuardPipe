package ai_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

func TestCacheKey_SameInputsProduceSameKey(t *testing.T) {
	vars := map[string]string{"rule_id": "x", "severity": "high"}
	untrusted := []ai.UntrustedBlock{{Label: "f.py", Content: "print(1)"}}

	k1, err := ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", vars, untrusted)
	require.NoError(t, err)
	k2, err := ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", vars, untrusted)
	require.NoError(t, err)
	require.Equal(t, k1, k2)
}

// TestCacheKey_MapIterationOrderDoesNotAffectTheKey is the canonicalisation
// guarantee documentation/10-ai-integration.md §7 exists for: two Vars maps
// built independently, with the same content, must hash identically
// regardless of Go's randomised map iteration order.
func TestCacheKey_MapIterationOrderDoesNotAffectTheKey(t *testing.T) {
	varsA := map[string]string{"a": "1", "b": "2", "c": "3"}
	varsB := map[string]string{"c": "3", "a": "1", "b": "2"}

	kA, err := ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", varsA, nil)
	require.NoError(t, err)
	kB, err := ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", varsB, nil)
	require.NoError(t, err)
	require.Equal(t, kA, kB)
}

// TestCacheKey_AnyInputChangeChangesTheKey is the near-miss half — every
// field that's part of the documented key formula must actually matter, or
// two different prompts/versions/models/inputs would collide on the same
// cached response.
func TestCacheKey_AnyInputChangeChangesTheKey(t *testing.T) {
	base, err := ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", map[string]string{"a": "1"}, nil)
	require.NoError(t, err)

	variants := map[string]string{}
	k, _ := ai.CacheKey(ai.PromptGeneratePatch, "v1", "gemini-2.5-flash", map[string]string{"a": "1"}, nil)
	variants["prompt_id"] = k
	k, _ = ai.CacheKey(ai.PromptExplainFinding, "v2", "gemini-2.5-flash", map[string]string{"a": "1"}, nil)
	variants["prompt_version"] = k
	k, _ = ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-pro", map[string]string{"a": "1"}, nil)
	variants["model"] = k
	k, _ = ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", map[string]string{"a": "2"}, nil)
	variants["vars"] = k
	k, _ = ai.CacheKey(ai.PromptExplainFinding, "v1", "gemini-2.5-flash", map[string]string{"a": "1"}, []ai.UntrustedBlock{{Label: "f", Content: "x"}})
	variants["untrusted"] = k

	for field, variant := range variants {
		require.NotEqual(t, base, variant, "changing %s must change the cache key", field)
	}
}

func TestMemoryCache_SetThenGetRoundTrips(t *testing.T) {
	c := ai.NewMemoryCache()
	ctx := context.Background()

	resp := ai.LLMResponse{Raw: []byte(`{"ok":true}`), TokensIn: 5, TokensOut: 7}
	require.NoError(t, c.Set(ctx, "k1", resp, time.Hour))

	got, hit, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	require.True(t, hit)
	require.True(t, got.FromCache, "a cache hit must be marked FromCache even though the stored value wasn't")
	require.Equal(t, resp.Raw, got.Raw)
	require.Equal(t, resp.TokensIn, got.TokensIn)
}

func TestMemoryCache_MissOnUnknownKey(t *testing.T) {
	c := ai.NewMemoryCache()
	_, hit, err := c.Get(context.Background(), "does-not-exist")
	require.NoError(t, err)
	require.False(t, hit)
}

// TestMemoryCache_ExpiredEntryIsTreatedAsMiss is the near-miss half of the
// round-trip test above — a TTL that has actually elapsed must not still
// serve stale content.
func TestMemoryCache_ExpiredEntryIsTreatedAsMiss(t *testing.T) {
	c := ai.NewMemoryCache()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "k1", ai.LLMResponse{Raw: []byte(`{}`)}, -time.Second)) // already expired

	_, hit, err := c.Get(ctx, "k1")
	require.NoError(t, err)
	require.False(t, hit)
}
