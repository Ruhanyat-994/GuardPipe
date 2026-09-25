package billing

import (
	"context"
	"log/slog"
	"time"
)

// Ticker runs monthly grants, period ends and top-up expiry on a fixed
// interval. Same shape and role split as orchestrator.Scheduler: it runs
// only where GUARDPIPE_ROLE is worker or all. Every step is idempotent, so
// two workers ticking at once can't double-grant.
type Ticker struct {
	Service  *Service
	Interval time.Duration
	Log      *slog.Logger
}

func (t *Ticker) Start(ctx context.Context) {
	interval := t.Interval
	if interval <= 0 {
		interval = time.Minute
	}
	log := t.Log
	if log == nil {
		log = slog.Default()
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if n, err := t.Service.RenewDue(ctx); err != nil {
			log.Error("billing: renewal tick failed", "error", err)
		} else if n > 0 {
			log.Info("billing: renewals processed", "orgs", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
