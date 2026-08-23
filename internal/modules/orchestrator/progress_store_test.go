package orchestrator

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestLiveProgress_SetGetClear(t *testing.T) {
	p := NewLiveProgress()
	jobID := uuid.New()

	if _, ok := p.Get(jobID); ok {
		t.Fatal("Get() on an unset job returned ok=true")
	}

	p.Set(jobID, 40, "Crawling the site for endpoints")
	got, ok := p.Get(jobID)
	if !ok {
		t.Fatal("Get() after Set() returned ok=false")
	}
	if got.Pct != 40 || got.Activity != "Crawling the site for endpoints" {
		t.Errorf("got %+v, want pct=40 activity=%q", got, "Crawling the site for endpoints")
	}

	p.Clear(jobID)
	if _, ok := p.Get(jobID); ok {
		t.Error("Get() after Clear() still returned ok=true")
	}
}

func TestLiveProgress_Set_ClampsToNeverReportTheTerminal100(t *testing.T) {
	// Only the orchestrator (a job actually reaching a terminal status) may
	// report 100% — an engine calling Set(jobID, 100, …) itself must not be
	// able to claim completion prematurely.
	p := NewLiveProgress()
	jobID := uuid.New()

	p.Set(jobID, 100, "done?")
	got, _ := p.Get(jobID)
	if got.Pct >= 100 {
		t.Errorf("Pct = %d, want clamped below 100", got.Pct)
	}

	p.Set(jobID, -5, "negative")
	got, _ = p.Get(jobID)
	if got.Pct < 0 {
		t.Errorf("Pct = %d, want clamped to >= 0", got.Pct)
	}
}

func TestElapsedFallbackPct_RealAndMonotonic(t *testing.T) {
	timeout := 100 * time.Millisecond

	early := elapsedFallbackPct(time.Now(), timeout)
	if early < 1 || early > 20 {
		t.Errorf("pct just after start = %d, want a small value near 0", early)
	}

	time.Sleep(60 * time.Millisecond)
	later := elapsedFallbackPct(time.Now().Add(-60*time.Millisecond), timeout)
	if later <= early {
		t.Errorf("pct did not advance with real elapsed time: early=%d later=%d", early, later)
	}
}

func TestElapsedFallbackPct_NearMiss_NeverReachesTerminal100(t *testing.T) {
	// A job that's run well past its own timeout (worker.go will have
	// already cancelled its context and marked it failed by then, but this
	// function must still never itself claim "done").
	longAgo := time.Now().Add(-10 * time.Hour)
	pct := elapsedFallbackPct(longAgo, time.Minute)
	if pct != 95 {
		t.Errorf("pct = %d, want capped at 95 for a wildly-overrun job", pct)
	}
}
