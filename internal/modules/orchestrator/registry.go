package orchestrator

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// Registry is the engine registry documentation/03-architecture-overview.md
// §6.3 promises: "Adding an engine means implementing this interface and
// registering it — no orchestrator changes." Phase 6 registers depscan
// only; Phase 7+ registers the rest, one Register call each, no other
// change to this package.
type Registry struct {
	engines map[domain.EngineID]domain.Engine
}

func NewRegistry() *Registry {
	return &Registry{engines: make(map[domain.EngineID]domain.Engine)}
}

func (r *Registry) Register(e domain.Engine) {
	r.engines[e.ID()] = e
}

func (r *Registry) Get(id domain.EngineID) (domain.Engine, bool) {
	e, ok := r.engines[id]
	return e, ok
}

func (r *Registry) Has(id domain.EngineID) bool {
	_, ok := r.engines[id]
	return ok
}

// IDs returns every registered engine ID — what a full_supply_chain scan
// requests by default.
func (r *Registry) IDs() []domain.EngineID {
	ids := make([]domain.EngineID, 0, len(r.engines))
	for id := range r.engines {
		ids = append(ids, id)
	}
	return ids
}
