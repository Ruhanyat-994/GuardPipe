package advisory

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/osv"
)

// OSVClient is the subset of adapters/osv.Client this package needs.
// Defined here (the consumer) per documentation/04-backend-architecture.md
// §5.1, so tests substitute a fake instead of making a real HTTP call
// (documentation/15-testing-strategy.md: "Tests never call ... OSV ...
// live").
type OSVClient interface {
	QueryBatch(ctx context.Context, queries []osv.PackageQuery) ([][]string, error)
	GetVulnerability(ctx context.Context, id string) (osv.Vulnerability, error)
}

// Cache is the Redis-backed advisory cache
// (documentation/06-database-design.md §11: 24h TTL, FR-DEP-005). Defined by
// this package per the same consumer-owns-the-port rule modules/ai's Cache
// interface follows.
type Cache interface {
	Get(ctx context.Context, key string) ([]Advisory, bool, error)
	Set(ctx context.Context, key string, advisories []Advisory, ttl time.Duration) error
}

// Service looks up advisories for a dependency inventory and serves the
// rules catalogue (documentation/07-api-specification.md §8) — both are
// "advisory data" per BUILD_GUIDE.md Phase 5's own naming, see this
// package's doc comment.
type Service interface {
	// Lookup returns one Result per entry in deps, in the same order.
	// Cache hits never touch OSV.dev; on a cache miss it batches queries
	// (osv.MaxBatchSize per call) and only reaches OSV.dev's /v1/vulns/{id}
	// once per distinct advisory ID across the whole batch, not once per
	// dependency.
	Lookup(ctx context.Context, deps []Dependency) ([]Result, error)

	// SyncRules upserts every rule in the RuleRegistry into the database —
	// called once at startup (documentation/06-database-design.md §11).
	SyncRules(ctx context.Context) error
	ListRules(ctx context.Context, filter RuleFilter, page RulePage) ([]Rule, int, error)
	GetRule(ctx context.Context, id string) (*Rule, error)
	SetRuleEnabled(ctx context.Context, id string, enabled bool) (*Rule, error)
}

type service struct {
	osv      OSVClient
	cache    Cache
	cacheTTL time.Duration
	log      *slog.Logger

	rules    RuleRepository
	registry *RuleRegistry
}

// NewService wires the advisory module. cacheTTL is
// GUARDPIPE_OSV_CACHE_TTL (documentation/13-devops-and-environments.md
// §5.7), matching the 24h default the Redis key table documents. registry
// is legitimately empty until Phase 6+ engines register their rules — see
// RuleRegistry's doc comment.
func NewService(osvClient OSVClient, cache Cache, cacheTTL time.Duration, rules RuleRepository, registry *RuleRegistry, log *slog.Logger) Service {
	if log == nil {
		log = slog.Default()
	}
	return &service{osv: osvClient, cache: cache, cacheTTL: cacheTTL, rules: rules, registry: registry, log: log}
}

func (s *service) Lookup(ctx context.Context, deps []Dependency) ([]Result, error) {
	results := make([]Result, len(deps))
	var missIdx []int

	for i, d := range deps {
		advisories, hit, err := s.cache.Get(ctx, cacheKey(d))
		if err != nil {
			// A cache read failure degrades to a live lookup rather than
			// failing the whole scan — the cache is an optimisation, not a
			// dependency of correctness.
			s.log.Warn("advisory: cache read failed, falling back to live lookup", "error", err)
			missIdx = append(missIdx, i)
			continue
		}
		if hit {
			results[i] = Result{Dependency: d, Advisories: advisories}
			continue
		}
		missIdx = append(missIdx, i)
	}

	if len(missIdx) == 0 {
		return results, nil
	}

	detailCache := make(map[string]Advisory)
	for _, chunk := range chunkIndices(missIdx, osv.MaxBatchSize) {
		s.resolveChunk(ctx, deps, results, chunk, detailCache)
	}

	return results, nil
}

// resolveChunk queries OSV.dev for one batch of dependencies and fills
// results for each index in chunk. A batch-level failure (OSV.dev
// unreachable) marks every dependency in the chunk Unavailable rather than
// failing Lookup entirely — the caller's job still succeeds with a partial
// inventory (FR-DEP-011).
func (s *service) resolveChunk(ctx context.Context, deps []Dependency, results []Result, chunk []int, detailCache map[string]Advisory) {
	queries := make([]osv.PackageQuery, len(chunk))
	for j, idx := range chunk {
		d := deps[idx]
		queries[j] = osv.PackageQuery{Ecosystem: d.Ecosystem, Name: d.Name, Version: d.Version}
	}

	idLists, err := s.osv.QueryBatch(ctx, queries)
	if err != nil {
		s.log.Warn("advisory: OSV.dev batch lookup failed, marking dependencies unavailable", "error", err, "count", len(chunk))
		for _, idx := range chunk {
			results[idx] = Result{Dependency: deps[idx], Unavailable: true}
		}
		return
	}

	for j, idx := range chunk {
		d := deps[idx]
		ids := idLists[j]
		advisories := make([]Advisory, 0, len(ids))
		unavailable := false

		for _, id := range ids {
			adv, ok := detailCache[id]
			if !ok {
				fetched, err := s.osv.GetVulnerability(ctx, id)
				if err != nil {
					s.log.Warn("advisory: OSV.dev detail lookup failed", "id", id, "error", err)
					unavailable = true
					break
				}
				adv = translateVulnerability(fetched)
				detailCache[id] = adv
			}
			advisories = append(advisories, adv)
		}

		if unavailable {
			results[idx] = Result{Dependency: d, Unavailable: true}
			continue
		}

		results[idx] = Result{Dependency: d, Advisories: advisories}
		if err := s.cache.Set(ctx, cacheKey(d), advisories, s.cacheTTL); err != nil {
			s.log.Warn("advisory: cache write failed", "error", err)
		}
	}
}

// translateVulnerability maps OSV.dev's wire schema onto Advisory —
// depscan's rules (documentation/05-module-specifications.md §"Core
// rules": known-cve, no-fix-available) only need a CVE ID, a fix
// boolean/version, and a severity string, not OSV's full affected-ranges
// shape.
func translateVulnerability(v osv.Vulnerability) Advisory {
	adv := Advisory{ID: v.ID, Summary: v.Summary}

	for _, alias := range v.Aliases {
		if strings.HasPrefix(alias, "CVE-") {
			adv.CVE = alias
			break
		}
	}
	if len(v.Severity) > 0 {
		adv.CVSSVector = v.Severity[0].Score
	}
	for _, ref := range v.References {
		adv.References = append(adv.References, ref.URL)
	}

	for _, affected := range v.Affected {
		for _, r := range affected.Ranges {
			for _, event := range r.Events {
				if event.Fixed != "" {
					adv.HasFix = true
					if adv.FixedVersion == "" {
						adv.FixedVersion = event.Fixed
					}
				}
			}
		}
	}

	return adv
}

// chunkIndices splits idx into slices of at most size elements, preserving
// order — how Lookup keeps OSV.dev batches under osv.MaxBatchSize.
func chunkIndices(idx []int, size int) [][]int {
	if size <= 0 {
		size = len(idx)
	}
	var chunks [][]int
	for size < len(idx) {
		idx, chunks = idx[size:], append(chunks, idx[:size:size])
	}
	if len(idx) > 0 {
		chunks = append(chunks, idx)
	}
	return chunks
}
