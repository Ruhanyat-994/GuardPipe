package docreview

import (
	"context"
	"errors"
	"fmt"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

// errAIUnavailable is what Run wraps its returned error with when aiSvc is
// nil — documentation/05-module-specifications.md §11's own failure-mode
// table: "Gemini unavailable: Job failed with ai_unavailable." Unlike
// cicdscan's supplementary AI pass (which degrades gracefully — 16
// deterministic rules still run with no AI configured), docreview has no
// deterministic fallback at all: AI review is this engine's entire output,
// so a missing AI service is a job failure, not a quieter partial result.
// The worker's current per-job error-reason mapping (internal/modules/orchestrator/worker.go)
// only distinguishes "engine_error"/"timeout" from a returned error today —
// giving every engine a way to name its own specific ErrorReason would mean
// changing the shared domain.Engine contract for all seven engines, out of
// scope here — so this surfaces as the existing generic "engine_error"
// bucket rather than the spec's illustrative "ai_unavailable" string. The
// job still fails, honestly, which is the behaviour that actually matters.
var errAIUnavailable = errors.New("AI service unavailable — docreview has no deterministic fallback")

// Engine implements domain.Engine — docreview's AI review of design/
// requirements documents (documentation/05-module-specifications.md §11).
// Every finding is AI-authored (aipass.go); there are no deterministic rule
// evaluators of its own the way every other engine has.
type Engine struct {
	aiSvc ai.Service
}

func New(aiSvc ai.Service) *Engine {
	return &Engine{aiSvc: aiSvc}
}

var _ domain.Engine = (*Engine)(nil)

func (e *Engine) ID() domain.EngineID {
	return domain.EngineDocReview
}

// Applicable is a cheap existence check: does this scan have anything for
// docreview to look at — an uploaded document, or a matching file inside
// the checkout (§11's own discovery locations). Deliberately independent of
// aiSvc being non-nil — "no documents" and "AI unavailable" are two
// different failure modes in §11's own table, and Applicable only speaks to
// the first; Run reports the second.
func (e *Engine) Applicable(ctx context.Context, in domain.ScanInput) (bool, string) {
	if len(in.Documents) > 0 {
		return true, ""
	}
	repoDocuments, err := discoverRepoDocuments(in.WorkspaceDir)
	if err != nil {
		return false, "could not read the repository for documents"
	}
	if len(repoDocuments) > 0 {
		return true, ""
	}
	return false, "no uploaded documents and no matching files found in the repository"
}

// Run has no deterministic fallback (see errAIUnavailable's own doc
// comment): a nil aiSvc fails the job outright rather than silently
// succeeding with zero findings, which would look identical to a
// genuinely clean document set.
func (e *Engine) Run(ctx context.Context, in domain.ScanInput, emit func(domain.Finding)) (domain.EngineResult, error) {
	if e.aiSvc == nil {
		return domain.EngineResult{}, fmt.Errorf("docreview: %w", errAIUnavailable)
	}

	repoDocuments, err := discoverRepoDocuments(in.WorkspaceDir)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("docreview: discover repository documents: %w", err)
	}

	filesReviewed, err := runAIPass(ctx, e.aiSvc, in, repoDocuments, emit)
	if err != nil {
		return domain.EngineResult{}, fmt.Errorf("docreview: AI review: %w", err)
	}

	return domain.EngineResult{
		RulesEvaluated: len(Rules),
		FilesScanned:   filesReviewed,
		Stats: map[string]any{
			"uploaded_documents":        len(in.Documents),
			"repo_discovered_documents": len(repoDocuments),
		},
	}, nil
}
