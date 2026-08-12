package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// repairInstruction is sent back to the model, once, when its response
// fails schema validation (documentation/10-ai-integration.md §5).
const repairInstruction = "\n\nYour previous response did not match the schema. Return only valid JSON matching it."

// evidenceExcerptLimit bounds how much of a discarded response is kept as
// Finding evidence — enough to show the user what triggered the detection,
// not the full (potentially large) response.
const evidenceExcerptLimit = 500

// RunInput is one call through the AI adapter.
type RunInput struct {
	PromptID  PromptID
	Vars      map[string]string // trusted metadata interpolated into the prompt — never raw repository content
	Untrusted []UntrustedBlock  // repository content — always boundary-delimited, never interpolated

	// ScanID and Engine identify what triggered this call, used only to
	// build a prompt_injection_attempt finding's ScanID/Engine/RuleID if the
	// response is discarded. Engine.Valid() need not hold for callers that
	// aren't a real engine (e.g. a scan-summary call from modules/scoring).
	ScanID uuid.UUID
	Engine domain.EngineID
	// Source is where the analysed content lives, used as the discarded
	// finding's Location. Left at its zero value, it still marshals to a
	// valid (if unhelpful) Location.
	Source domain.Location
}

// RunResult is the outcome of one successful Service.Run call. Discarded is
// true when the response was suspected of prompt injection and thrown away
// (Value/Raw are zero in that case) — callers must check it before using
// Value, exactly the same way they'd handle a provider error, per
// documentation/10-ai-integration.md §9's per-caller fallback table.
type RunResult struct {
	Value     any // one of the typed *Response structs in schema.go, matching RunInput.PromptID
	Raw       json.RawMessage
	FromCache bool
	Discarded bool
	TokensIn  int
	TokensOut int
}

// Service is the one entry point callers use — an engine, or later
// modules/scoring for a scan summary. It owns prompt rendering, injection
// defence, schema validation with one repair retry, and caching; callers
// never see a Prompt or an LLMProvider directly.
type Service interface {
	Run(ctx context.Context, in RunInput, emit func(domain.Finding)) (*RunResult, error)
}

type service struct {
	provider LLMProvider
	cache    Cache
	cacheTTL time.Duration
	modelFor func(ModelTier) string
}

// NewService wires the AI module. modelFast/modelSmart come from
// platform/config (GUARDPIPE_GEMINI_MODEL_FAST/_SMART) — never hardcoded
// (documentation/10-ai-integration.md §3). cache may be nil to disable
// caching entirely (every call is a live provider call); cacheTTL is
// GUARDPIPE_AI_CACHE_TTL.
func NewService(provider LLMProvider, cache Cache, cacheTTL time.Duration, modelFast, modelSmart string) Service {
	return &service{
		provider: provider,
		cache:    cache,
		cacheTTL: cacheTTL,
		modelFor: func(tier ModelTier) string {
			if tier == ModelTierSmart {
				return modelSmart
			}
			return modelFast
		},
	}
}

func (s *service) Run(ctx context.Context, in RunInput, emit func(domain.Finding)) (*RunResult, error) {
	prompt, ok := lookupPrompt(in.PromptID)
	if !ok {
		return nil, fmt.Errorf("ai: unknown prompt id %q", in.PromptID)
	}
	model := s.modelFor(prompt.Model)

	key, err := CacheKey(prompt.ID, prompt.Version, model, in.Vars, in.Untrusted)
	if err != nil {
		return nil, fmt.Errorf("ai: compute cache key: %w", err)
	}

	if s.cache != nil {
		if cached, hit, cerr := s.cache.Get(ctx, key); cerr == nil && hit {
			return s.finish(prompt, cached, in, emit)
		}
	}

	boundary, err := newBoundary()
	if err != nil {
		return nil, fmt.Errorf("ai: %w", err)
	}
	system, user, err := renderPrompt(prompt, in.Vars, in.Untrusted, boundary)
	if err != nil {
		return nil, fmt.Errorf("ai: %w", err)
	}
	req := LLMRequest{
		PromptID:    prompt.ID,
		System:      system,
		User:        user,
		Schema:      prompt.Schema,
		Model:       model,
		MaxTokens:   prompt.MaxTokens,
		Temperature: prompt.Temperature,
	}

	resp, result, err := s.attempt(ctx, prompt, req, in, emit)
	if err != nil && errors.Is(err, ErrSchemaViolation) {
		// One repair retry (documentation/10-ai-integration.md §5), then
		// give up cleanly rather than looping.
		req.User = user + repairInstruction
		resp, result, err = s.attempt(ctx, prompt, req, in, emit)
	}
	if err != nil {
		return nil, err
	}

	if s.cache != nil && !result.Discarded {
		// Best-effort: a cache write failure must never fail a call that
		// otherwise succeeded.
		_ = s.cache.Set(ctx, key, resp, s.cacheTTL)
	}
	return result, nil
}

// attempt makes one provider call and validates it, returning the raw
// response alongside the built RunResult so the caller can decide whether
// to cache it.
func (s *service) attempt(ctx context.Context, prompt Prompt, req LLMRequest, in RunInput, emit func(domain.Finding)) (LLMResponse, *RunResult, error) {
	resp, err := s.provider.Complete(ctx, req)
	if err != nil {
		return LLMResponse{}, nil, err
	}
	result, err := s.finish(prompt, resp, in, emit)
	return resp, result, err
}

// finish validates a (possibly cached) response. On suspected injection it
// discards the content and emits a prompt_injection_attempt finding instead
// of returning it (documentation/10-ai-integration.md §5) — an attempt to
// manipulate the analyser is itself a security-relevant, deterministic fact
// about the repository, independent of whatever the AI said.
func (s *service) finish(prompt Prompt, resp LLMResponse, in RunInput, emit func(domain.Finding)) (*RunResult, error) {
	value, err := Validate(prompt, resp.Raw)
	if err != nil {
		if errors.Is(err, ErrInjectionSuspected) {
			if emit != nil {
				emit(injectionFinding(in, resp.Raw))
			}
			return &RunResult{Discarded: true, FromCache: resp.FromCache}, nil
		}
		return nil, err
	}
	return &RunResult{
		Value:     value,
		Raw:       resp.Raw,
		FromCache: resp.FromCache,
		TokensIn:  resp.TokensIn,
		TokensOut: resp.TokensOut,
	}, nil
}

// injectionFinding builds the deterministic finding documented in
// documentation/10-ai-integration.md §5 and named in
// documentation/03-architecture-overview.md QS-7 / CLAUDE.md's security
// posture section. RuleID follows the project-wide "<engine>.<category>.<rule>"
// convention (documentation/03-architecture-overview.md §7.1) using
// whichever engine's call triggered it — the doc's own worked example is
// literally "docreview.security.prompt-injection-attempt".
func injectionFinding(in RunInput, raw json.RawMessage) domain.Finding {
	evidence := string(raw)
	if len(evidence) > evidenceExcerptLimit {
		evidence = evidence[:evidenceExcerptLimit] + "…"
	}

	engine := in.Engine
	if engine == "" {
		engine = domain.EngineID("ai")
	}
	ruleID := fmt.Sprintf("%s.security.prompt-injection-attempt", engine)

	locationBytes, _ := json.Marshal(in.Source) // Location always marshals cleanly; see domain.Location's json tags
	fingerprint := id.Fingerprint(ruleID, string(locationBytes), evidence)

	return domain.Finding{
		ID:          id.New(),
		ScanID:      in.ScanID,
		Engine:      in.Engine,
		RuleID:      ruleID,
		Fingerprint: fingerprint,
		Title:       "Prompt injection attempt detected",
		Description: "Content sent to the AI analysis service appears to contain an attempt to override its instructions (e.g. \"ignore previous instructions\"). The AI's response was discarded rather than trusted; this finding records the attempt itself as a deterministic fact about the repository, not an AI judgement.",
		Severity:    domain.SeverityMedium,
		Confidence:  domain.ConfidenceMedium,
		Location:    in.Source,
		Evidence: []domain.Evidence{{
			Kind:  domain.EvidenceKindCommandOutput,
			Value: evidence,
		}},
		Remediation: "Review the flagged content and remove any text designed to manipulate automated analysis tools. This finding does not by itself indicate a vulnerability in the scanned code or documents.",
		Status:      domain.StatusOpen,
	}
}
