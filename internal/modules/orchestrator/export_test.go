package orchestrator

import "time"

// SetCancelPollInterval shortens how often a running job checks its scan's
// cancel flag, so tests don't wait the real 3s. Returns a restore func.
func SetCancelPollInterval(d time.Duration) func() {
	old := cancelPollInterval
	cancelPollInterval = d
	return func() { cancelPollInterval = old }
}
