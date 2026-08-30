package gemini

import "sync"

// KeyPool rotates through a fixed set of Gemini API keys
// (documentation/13-devops-and-environments.md §5.6, GUARDPIPE_GEMINI_API_KEYS,
// BUILD_GUIDE.md Phase 4). It is thread-safe: every Client.Complete call
// shares one pool and one "current" index, so a key one call finds
// exhausted is skipped by the next call too, instead of every goroutine
// separately rediscovering the same dead key.
//
// Free-tier Gemini quota is scoped to the Google Cloud project a key
// belongs to, not the key string itself — a pool of same-project keys
// shares one quota bucket and rotating between them buys nothing. This
// package has no way to detect that; it's a deployment-time concern (see
// BUILD_GUIDE.md's note on Phase 4, "keys should come from separate Google
// Cloud projects to actually add quota").
type KeyPool struct {
	mu      sync.Mutex
	keys    []string
	current int
}

// NewKeyPool builds a KeyPool starting at the first key. keys must be
// non-empty — callers (NewClient) are expected to have already validated
// that via platform/config's fail-fast Load.
func NewKeyPool(keys []string) *KeyPool {
	cp := make([]string, len(keys))
	copy(cp, keys)
	return &KeyPool{keys: cp}
}

// Len is how many keys are in the pool — the upper bound on how many times
// Complete will try before giving up.
func (p *KeyPool) Len() int {
	return len(p.keys)
}

// Current returns the key currently in rotation.
func (p *KeyPool) Current() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.keys[p.current]
}

// Advance moves rotation to the next key, wrapping back to the first once
// the last one is passed.
func (p *KeyPool) Advance() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = (p.current + 1) % len(p.keys)
}

// CurrentIndex reports which key is in rotation without ever exposing the
// key value itself — used for `GET /admin/system-health`
// (BUILD_GUIDE.md Phase 14). The raw key from Current() is a secret and
// must never be surfaced there.
func (p *KeyPool) CurrentIndex() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.current
}
