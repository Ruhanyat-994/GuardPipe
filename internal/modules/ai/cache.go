package ai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

// Cache is the content-hash response cache
// (documentation/10-ai-integration.md §7) — "the single most important cost
// control. Without it, the free tier does not survive development, let
// alone a demo." Defined by this package (the consumer), per
// documentation/04-backend-architecture.md §5.1.
type Cache interface {
	Get(ctx context.Context, key string) (LLMResponse, bool, error)
	Set(ctx context.Context, key string, resp LLMResponse, ttl time.Duration) error
}

// CacheKey computes cache_key = SHA256(prompt_id ‖ prompt_version ‖ model ‖
// canonical_json(inputs)) exactly as documented (§7). Canonicalisation sorts
// map keys and marshals with no caller-controlled formatting, so
// semantically identical inputs hash identically regardless of Go map
// iteration order.
func CacheKey(promptID PromptID, promptVersion, model string, vars map[string]string, untrusted []UntrustedBlock) (string, error) {
	canonical, err := canonicalInputs(vars, untrusted)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(promptID))
	h.Write([]byte{0})
	h.Write([]byte(promptVersion))
	h.Write([]byte{0})
	h.Write([]byte(model))
	h.Write([]byte{0})
	h.Write(canonical)
	return "gp:cache:ai:" + hex.EncodeToString(h.Sum(nil)), nil
}

// canonicalOrderedVar is one Vars entry in a fixed key order, so JSON
// marshalling never depends on Go's randomised map iteration.
type canonicalOrderedVar struct {
	Key   string `json:"k"`
	Value string `json:"v"`
}

type canonicalInputsShape struct {
	Vars      []canonicalOrderedVar `json:"vars"`
	Untrusted []UntrustedBlock      `json:"untrusted"`
}

func canonicalInputs(vars map[string]string, untrusted []UntrustedBlock) ([]byte, error) {
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make([]canonicalOrderedVar, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, canonicalOrderedVar{Key: k, Value: vars[k]})
	}
	if untrusted == nil {
		untrusted = []UntrustedBlock{}
	}

	return json.Marshal(canonicalInputsShape{Vars: ordered, Untrusted: untrusted})
}

// MemoryCache is an in-process Cache implementation. Documented deviation:
// documentation/10-ai-integration.md §7 specifies a two-tier Redis+Postgres
// cache, but no Redis client exists in the codebase yet — adapters/queue
// (the Redis adapter) lands in Phase 6, the same reasoning Phase 2 used for
// its in-memory rate limiter (see PROGRESS-LOG.md's Phase 2 deviations
// table). This satisfies the Cache interface today and is swappable for a
// Redis-backed implementation without any caller change once that adapter
// exists. Not suitable for a multi-replica deployment (each process has its
// own cache) — same caveat as the Phase 2 rate limiter, revisit together.
type MemoryCache struct {
	mu      sync.Mutex
	entries map[string]memoryCacheEntry
	hits    int
	misses  int
}

type memoryCacheEntry struct {
	resp    LLMResponse
	expires time.Time
}

// NewMemoryCache builds an empty MemoryCache.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{entries: make(map[string]memoryCacheEntry)}
}

func (c *MemoryCache) Get(_ context.Context, key string) (LLMResponse, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return LLMResponse{}, false, nil
	}
	if time.Now().After(entry.expires) {
		delete(c.entries, key)
		c.misses++
		return LLMResponse{}, false, nil
	}

	c.hits++
	resp := entry.resp
	resp.FromCache = true
	return resp, true, nil
}

// Stats reports cumulative hit/miss counts since process start — used by
// `GET /admin/system-health` (BUILD_GUIDE.md Phase 14) via a small wrapper
// in cmd/guardpipe (this package doesn't import modules/admin itself; see
// that wrapper's own doc comment).
func (c *MemoryCache) Stats() (hits, misses int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

func (c *MemoryCache) Set(_ context.Context, key string, resp LLMResponse, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = memoryCacheEntry{resp: resp, expires: time.Now().Add(ttl)}
	return nil
}
