package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
)

// EnableLiveScanRequest matches `PUT /projects/{id}/live-scanning`.
// "pentest" passes this validation on purpose: the service then rejects it
// with its own specific "penetration tests never run automatically" error
// instead of a generic invalid-field one.
type EnableLiveScanRequest struct {
	Engines         []string `json:"engines" validate:"required,min=1,dive,oneof=docreview codescan depscan containerscan k8sscan cicdscan pentest"`
	WatchedBranches []string `json:"watched_branches" validate:"omitempty,max=20,dive,max=255"`
	// Confirmed is the confirmation checkbox — the user authorising
	// automatic scanning of this repository under their name.
	Confirmed bool `json:"confirmed"`
	// MinBalancePercent: stop live scans below this % of the monthly token
	// grant. Omitted keeps the current value.
	MinBalancePercent *int `json:"min_balance_percent" validate:"omitempty,min=0,max=90"`
}

func (r EnableLiveScanRequest) ToInput() livescan.EnableInput {
	engines := make([]domain.EngineID, len(r.Engines))
	for i, e := range r.Engines {
		engines[i] = domain.EngineID(e)
	}
	return livescan.EnableInput{Engines: engines, WatchedBranches: r.WatchedBranches, Confirmed: r.Confirmed, MinBalancePercent: r.MinBalancePercent}
}

// LiveScanResponse matches `GET/PUT /projects/{id}/live-scanning`. The
// webhook secret is never part of any response.
type LiveScanResponse struct {
	Enabled bool `json:"enabled"`
	// AllowedEngines is what the settings form offers — never pentest.
	AllowedEngines     []string   `json:"allowed_engines"`
	Engines            []string   `json:"engines"`
	WatchedBranches    []string   `json:"watched_branches"`
	EnabledBy          *string    `json:"enabled_by"`
	EnabledByName      *string    `json:"enabled_by_name"`
	AttestedAt         *time.Time `json:"attested_at"`
	LastDeliveryAt     *time.Time `json:"last_delivery_at"`
	LastDeliveryStatus *string    `json:"last_delivery_status"`
	PausedReason       *string    `json:"paused_reason"`
	PausedAt           *time.Time `json:"paused_at"`
	MinBalancePercent  int        `json:"min_balance_percent"`
}

// FromLiveScan builds the response; w nil means live scanning is off.
// enabledByName may be empty when the user can't be looked up.
func FromLiveScan(w *livescan.Webhook, enabledByName string) LiveScanResponse {
	allowed := make([]string, len(livescan.AllowedEngines))
	for i, e := range livescan.AllowedEngines {
		allowed[i] = string(e)
	}
	resp := LiveScanResponse{AllowedEngines: allowed, Engines: []string{}, WatchedBranches: []string{}}
	if w == nil {
		return resp
	}
	resp.Enabled = true
	resp.Engines = make([]string, len(w.Engines))
	for i, e := range w.Engines {
		resp.Engines[i] = string(e)
	}
	resp.WatchedBranches = w.WatchedBranches
	if w.EnabledBy != nil {
		v := w.EnabledBy.String()
		resp.EnabledBy = &v
	}
	if enabledByName != "" {
		resp.EnabledByName = &enabledByName
	}
	attested := w.AttestedAt
	resp.AttestedAt = &attested
	resp.LastDeliveryAt = w.LastDeliveryAt
	if w.LastDeliveryStatus != "" {
		v := w.LastDeliveryStatus
		resp.LastDeliveryStatus = &v
	}
	resp.PausedReason, resp.PausedAt = w.PausedReason, w.PausedAt
	resp.MinBalancePercent = w.MinBalancePercent
	return resp
}

// DisableLiveScanResponse matches `DELETE /projects/{id}/live-scanning`.
type DisableLiveScanResponse struct {
	// GitHubHookRemoved false means live scanning is off in GuardPipe but the
	// hook is still on GitHub (the token couldn't delete it) — the user
	// should remove it under the repository's Settings → Webhooks.
	GitHubHookRemoved bool `json:"github_hook_removed"`
}
