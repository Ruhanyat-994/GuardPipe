package repo

import (
	"context"
	"fmt"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
)

// AISuggestionRepo implements ai.SuggestionRepository against the
// `ai_suggestions` table (documentation/06-database-design.md §4.13) — one
// row per finding (`finding_id UNIQUE`), so a re-scored/re-enriched finding
// (e.g. a redelivered job re-triggering enrichment, same reasoning
// RiskAssessmentRepo's own doc comment gives) upserts rather than
// duplicates.
type AISuggestionRepo struct {
	db Querier
}

func NewAISuggestionRepo(db Querier) *AISuggestionRepo {
	return &AISuggestionRepo{db: db}
}

var _ ai.SuggestionRepository = (*AISuggestionRepo)(nil)

func (r *AISuggestionRepo) Upsert(ctx context.Context, s ai.Suggestion) error {
	const q = `
		INSERT INTO ai_suggestions (finding_id, explanation, patch_diff, patch_status, model, prompt_version, input_hash, tokens_in, tokens_out, generated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (finding_id) DO UPDATE SET
			explanation = EXCLUDED.explanation, patch_diff = EXCLUDED.patch_diff, patch_status = EXCLUDED.patch_status,
			model = EXCLUDED.model, prompt_version = EXCLUDED.prompt_version, input_hash = EXCLUDED.input_hash,
			tokens_in = EXCLUDED.tokens_in, tokens_out = EXCLUDED.tokens_out, generated_at = EXCLUDED.generated_at`
	_, err := r.db.Exec(ctx, q,
		s.FindingID, nullIfEmpty(s.Explanation), nullIfEmpty(s.PatchDiff), s.PatchStatus,
		s.Model, s.PromptVersion, s.InputHash, nullIfZero(s.TokensIn), nullIfZero(s.TokensOut), s.GeneratedAt,
	)
	if err != nil {
		return fmt.Errorf("repo: upsert ai suggestion: %w", err)
	}
	return nil
}
