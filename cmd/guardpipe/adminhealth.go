package main

import (
	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/gemini"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

// geminiPoolReader and aiCacheReader adapt adapters/gemini and modules/ai
// to admin.GeminiPoolReader/admin.AICacheReader (BUILD_GUIDE.md Phase 14).
// The adaptation lives here, in main's own wiring code, rather than inside
// either package, so neither `adapters/gemini` nor `modules/ai` — both
// lower-level, product-critical packages — needs to import the
// platform-admin module just to satisfy its System Health screen.
type geminiPoolReader struct{ client *gemini.Client }

func (r geminiPoolReader) PoolStatus() admin.GeminiPoolStatus {
	size, current := r.client.PoolStatus()
	return admin.GeminiPoolStatus{Available: true, PoolSize: size, CurrentIndex: current}
}

type aiCacheReader struct{ cache *ai.MemoryCache }

func (r aiCacheReader) CacheStats() admin.AICacheStatus {
	hits, misses := r.cache.Stats()
	return admin.AICacheStatus{Available: true, Hits: hits, Misses: misses}
}
