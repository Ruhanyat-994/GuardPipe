package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

// GetByFindingID returns findingID's suggestion, or nil if enrichment
// never produced one for it (a low/informational finding, one whose
// budget ran out, or one from before AI enrichment existed) — a normal
// case, not an error.
func (r *AISuggestionRepo) GetByFindingID(ctx context.Context, findingID uuid.UUID) (*ai.Suggestion, error) {
	const q = `
		SELECT finding_id, explanation, patch_diff, patch_status, model, prompt_version, input_hash, tokens_in, tokens_out, generated_at
		FROM ai_suggestions WHERE finding_id = $1`

	var s ai.Suggestion
	var explanation, patchDiff *string
	var tokensIn, tokensOut *int
	var generatedAt time.Time
	err := r.db.QueryRow(ctx, q, findingID).Scan(
		&s.FindingID, &explanation, &patchDiff, &s.PatchStatus, &s.Model, &s.PromptVersion, &s.InputHash, &tokensIn, &tokensOut, &generatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("repo: get ai suggestion: %w", err)
	}
	if explanation != nil {
		s.Explanation = *explanation
	}
	if patchDiff != nil {
		s.PatchDiff = *patchDiff
	}
	if tokensIn != nil {
		s.TokensIn = *tokensIn
	}
	if tokensOut != nil {
		s.TokensOut = *tokensOut
	}
	s.GeneratedAt = generatedAt
	return &s, nil
}
