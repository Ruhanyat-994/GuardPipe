package advisory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/osv"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// fakeOSVClient never makes a real HTTP call
// (documentation/15-testing-strategy.md: "Tests never call ... OSV ...
// live").
type fakeOSVClient struct {
	batchResults map[string][]string // "eco|name|ver" -> vuln IDs
	batchErr     error
	vulns        map[string]osv.Vulnerability
	vulnErr      map[string]error
	batchCalls   int
	detailCalls  []string
}

func (f *fakeOSVClient) QueryBatch(_ context.Context, queries []osv.PackageQuery) ([][]string, error) {
	f.batchCalls++
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	out := make([][]string, len(queries))
	for i, q := range queries {
		out[i] = f.batchResults[q.Ecosystem+"|"+q.Name+"|"+q.Version]
	}
	return out, nil
}

func (f *fakeOSVClient) GetVulnerability(_ context.Context, id string) (osv.Vulnerability, error) {
	f.detailCalls = append(f.detailCalls, id)
	if err, ok := f.vulnErr[id]; ok {
		return osv.Vulnerability{}, err
	}
	return f.vulns[id], nil
}

// fakeCache is a hand-written in-memory Cache fake — no mocking framework
// per the project's testing philosophy.
type fakeCache struct {
	mu      sync.Mutex
	entries map[string][]advisory.Advisory
	getErr  error
}

func newFakeCache() *fakeCache {
	return &fakeCache{entries: make(map[string][]advisory.Advisory)}
}

func (f *fakeCache) Get(_ context.Context, key string) ([]advisory.Advisory, bool, error) {
	if f.getErr != nil {
		return nil, false, f.getErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.entries[key]
	return v, ok, nil
}

func (f *fakeCache) Set(_ context.Context, key string, advisories []advisory.Advisory, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries[key] = advisories
	return nil
}

func vulnerableDep() advisory.Dependency {
	return advisory.Dependency{Ecosystem: "npm", Name: "left-pad", Version: "1.0.0"}
}

func TestLookup_CacheHit_NeverCallsOSV(t *testing.T) {
	cache := newFakeCache()
	dep := vulnerableDep()
	cache.entries["gp:cache:osv:npm:left-pad:1.0.0"] = []advisory.Advisory{{ID: "GHSA-cached"}}

	osvClient := &fakeOSVClient{}
	svc := advisory.NewService(osvClient, cache, time.Hour, nil, nil, nil)

	results, err := svc.Lookup(context.Background(), []advisory.Dependency{dep})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "GHSA-cached", results[0].Advisories[0].ID)
	require.Equal(t, 0, osvClient.batchCalls)
}

func TestLookup_CacheMiss_FetchesAndCaches(t *testing.T) {
	cache := newFakeCache()
	dep := vulnerableDep()
	osvClient := &fakeOSVClient{
		batchResults: map[string][]string{"npm|left-pad|1.0.0": {"GHSA-1234"}},
		vulns: map[string]osv.Vulnerability{
			"GHSA-1234": {
				ID:      "GHSA-1234",
				Summary: "left-pad has a bug",
				Aliases: []string{"CVE-2024-9999"},
				Affected: []osv.VulnAffected{
					{Ranges: []osv.VulnRange{{Events: []osv.VulnRangeEvent{{Introduced: "0"}, {Fixed: "1.0.1"}}}}},
				},
			},
		},
	}
	svc := advisory.NewService(osvClient, cache, time.Hour, nil, nil, nil)

	results, err := svc.Lookup(context.Background(), []advisory.Dependency{dep})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.False(t, results[0].Unavailable)
	require.Len(t, results[0].Advisories, 1)
	require.Equal(t, "CVE-2024-9999", results[0].Advisories[0].CVE)
	require.True(t, results[0].Advisories[0].HasFix)
	require.Equal(t, "1.0.1", results[0].Advisories[0].FixedVersion)

	// Now cached — a second Lookup must not re-query OSV.
	_, err = svc.Lookup(context.Background(), []advisory.Dependency{dep})
	require.NoError(t, err)
	require.Equal(t, 1, osvClient.batchCalls)
}

func TestLookup_CleanDependency_CachesEmptyResult(t *testing.T) {
	cache := newFakeCache()
	dep := advisory.Dependency{Ecosystem: "npm", Name: "clean-pkg", Version: "2.0.0"}
	osvClient := &fakeOSVClient{batchResults: map[string][]string{}}
	svc := advisory.NewService(osvClient, cache, time.Hour, nil, nil, nil)

	results, err := svc.Lookup(context.Background(), []advisory.Dependency{dep})
	require.NoError(t, err)
	require.Empty(t, results[0].Advisories)
	require.False(t, results[0].Unavailable)

	cached, hit, err := cache.Get(context.Background(), "gp:cache:osv:npm:clean-pkg:2.0.0")
	require.NoError(t, err)
	require.True(t, hit)
	require.Empty(t, cached)
}

func TestLookup_OSVUnreachable_MarksUnavailableInsteadOfFailing(t *testing.T) {
	cache := newFakeCache()
	dep := vulnerableDep()
	osvClient := &fakeOSVClient{batchErr: apperrors.External("osv.unreachable", "could not reach OSV.dev", nil)}
	svc := advisory.NewService(osvClient, cache, time.Hour, nil, nil, nil)

	results, err := svc.Lookup(context.Background(), []advisory.Dependency{dep})
	require.NoError(t, err, "FR-DEP-011: OSV failure must not fail the whole lookup")
	require.Len(t, results, 1)
	require.True(t, results[0].Unavailable)
	require.Equal(t, dep, results[0].Dependency)
}

func TestLookup_DetailFetchFailure_MarksThatDependencyUnavailable(t *testing.T) {
	cache := newFakeCache()
	dep := vulnerableDep()
	osvClient := &fakeOSVClient{
		batchResults: map[string][]string{"npm|left-pad|1.0.0": {"GHSA-broken"}},
		vulnErr:      map[string]error{"GHSA-broken": apperrors.External("osv.upstream_error", "boom", nil)},
	}
	svc := advisory.NewService(osvClient, cache, time.Hour, nil, nil, nil)

	results, err := svc.Lookup(context.Background(), []advisory.Dependency{dep})
	require.NoError(t, err)
	require.True(t, results[0].Unavailable)
}

func TestLookup_DedupsDetailFetchesAcrossDependencies(t *testing.T) {
	cache := newFakeCache()
	depA := advisory.Dependency{Ecosystem: "npm", Name: "pkg-a", Version: "1.0.0"}
	depB := advisory.Dependency{Ecosystem: "npm", Name: "pkg-b", Version: "1.0.0"}
	osvClient := &fakeOSVClient{
		batchResults: map[string][]string{
			"npm|pkg-a|1.0.0": {"GHSA-shared"},
			"npm|pkg-b|1.0.0": {"GHSA-shared"},
		},
		vulns: map[string]osv.Vulnerability{"GHSA-shared": {ID: "GHSA-shared"}},
	}
	svc := advisory.NewService(osvClient, cache, time.Hour, nil, nil, nil)

	results, err := svc.Lookup(context.Background(), []advisory.Dependency{depA, depB})
	require.NoError(t, err)
	require.Len(t, results, 2)
	require.Len(t, osvClient.detailCalls, 1, "GHSA-shared should be fetched once, not per dependency")
}

func TestLookup_EmptyInventory(t *testing.T) {
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, nil, nil, nil)
	results, err := svc.Lookup(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, results)
}
