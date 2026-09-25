package billing

import (
	"context"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
)

// OrchestratorCharger adapts Service to orchestrator.TokenCharger (and so
// also orchestrator.TokenRefunder). orchestrator defines that interface
// with its own request types so it never imports billing; this is the one
// place the two meet.
type OrchestratorCharger struct{ Service *Service }

var _ orchestrator.TokenCharger = OrchestratorCharger{}

func (a OrchestratorCharger) Charge(ctx context.Context, orgID uuid.UUID, req orchestrator.ChargeRequest) error {
	lines := make([]ChargeLine, len(req.Lines))
	for i, l := range req.Lines {
		lines[i] = ChargeLine{JobID: l.JobID, Engine: l.Engine}
	}
	return a.Service.Charge(ctx, orgID, ChargeRequest{
		ScanID: req.ScanID, Lines: lines, Preset: req.Preset, Trigger: req.Trigger, ActorID: req.ActorID,
	})
}

func (a OrchestratorCharger) RefundJob(ctx context.Context, jobID uuid.UUID, reason string) error {
	return a.Service.RefundJob(ctx, jobID, reason)
}

func (a OrchestratorCharger) AllowedEngines(ctx context.Context, orgID uuid.UUID, engines []domain.EngineID) ([]domain.EngineID, error) {
	return a.Service.AllowedEngines(ctx, orgID, engines)
}

func (a OrchestratorCharger) RequireFeature(ctx context.Context, orgID uuid.UUID, feature string) error {
	return a.Service.RequireFeature(ctx, orgID, feature)
}
