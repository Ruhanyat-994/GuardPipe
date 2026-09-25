// Package ai is the shared LLM adapter (documentation/10-ai-integration.md,
// documentation/05-module-specifications.md). It owns the LLMProvider port,
// the prompt registry, prompt-injection defence, response schema validation,
// and content-hash caching — everything that sits between a caller (an
// engine, later scoring/reporting) and whichever provider actually talks to
// an LLM. Nothing outside internal/adapters/gemini knows Gemini exists
// (FR-AI-001) — this package only ever talks to the LLMProvider interface.
//
// The governing rule (documentation/10-ai-integration.md §1): every
// deterministic capability of GuardPipe stays deterministic. AI output can
// never raise or lower a severity, compute the risk score, or gate a
// release — it is enrichment layered on top of a system that already works
// without it. If every provider call fails, callers degrade to their
// documented fallback (§9); nothing in this package panics or blocks a scan.
package ai

import (
	"context"
	"encoding/json"
	"time"
)

// PromptID is the registry key for a versioned prompt
// (documentation/10-ai-integration.md §4).
type PromptID string

const (
	PromptExplainFinding PromptID = "explain_finding"
	PromptGeneratePatch  PromptID = "generate_patch"
	PromptReviewDocument PromptID = "review_document"
	PromptReviewWorkflow PromptID = "review_workflow"
	PromptSummariseScan  PromptID = "summarise_scan"
	// PromptRemediateFinding is the finding assistant's "Remediation"
	// command (modules/assist): a step-by-step fix plan for one finding.
	PromptRemediateFinding PromptID = "remediate_finding"
)

// ModelTier is a prompt's declared model class, resolved to a concrete model
// ID at call time via platform/config's GUARDPIPE_GEMINI_MODEL_FAST/_SMART
// (documentation/10-ai-integration.md §3) — never hardcoded, so a model
// rename doesn't need a release.
type ModelTier string

const (
	ModelTierFast  ModelTier = "fast"
	ModelTierSmart ModelTier = "smart"
)

// UntrustedBlock is one piece of content that came from a repository under
// analysis — a file, a workflow, a document chunk. It is never interpolated
// into the trusted instruction section; Service.Run wraps it in a random
// per-request boundary before it ever reaches a provider
// (documentation/10-ai-integration.md §5, layers 1-3).
type UntrustedBlock struct {
	Label   string // e.g. a file path — shown in the delimiter for the model's benefit, not trusted either
	Content string
}

// LLMRequest is what this package sends to whichever LLMProvider is wired
// in. System and User are already fully rendered — prompt-registry lookup
// and boundary delimiting happen in this package (Service.Run), never in an
// adapter. This is a deliberate simplification of
// documentation/10-ai-integration.md §2's illustrative LLMRequest shape
// (which carries PromptID/Vars/Untrusted separately, implying the adapter
// renders them): keeping prompt construction in exactly one place is what
// makes the injection-defence boundary token (§5) exist in exactly one
// place too, instead of every current and future adapter needing to
// reimplement it correctly.
type LLMRequest struct {
	PromptID    PromptID // for provider-side logging/metrics only — never branched on
	System      string
	User        string
	Schema      json.RawMessage
	Model       string
	MaxTokens   int
	Temperature float32
}

// LLMResponse is what a provider returns for one completion.
type LLMResponse struct {
	Raw       json.RawMessage
	TokensIn  int
	TokensOut int
	Model     string
	FromCache bool
	Latency   time.Duration
}

// LLMProvider is the port every LLM implementation satisfies
// (documentation/10-ai-integration.md §2, ADR-0004). Gemini
// (internal/adapters/gemini) is the only real implementation; tests use a
// hand-written fake (StubProvider in this package's own tests) — no test
// anywhere calls a real provider (documentation/15-testing-strategy.md).
type LLMProvider interface {
	Complete(ctx context.Context, req LLMRequest) (LLMResponse, error)
	Name() string
}
