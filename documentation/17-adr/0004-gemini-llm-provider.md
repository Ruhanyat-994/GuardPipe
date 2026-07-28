# ADR-0004 — Gemini as the LLM, behind a provider port

| Status | Accepted |
|---|---|
| Date | 2026-07-29 |
| Deciders | M4, M1 |
| Supersedes | — |

## Context

Four features need a language model: document review, finding explanation, patch generation, and CI/CD semantic analysis.

Constraints:
- **Zero budget.** A student project with no funding.
- Quota must survive three weeks of repeated development scans plus a live demo.
- Structured JSON output is mandatory — we parse every response against a schema.
- Large context is useful for document review.
- The features must degrade cleanly if the provider is unavailable.

## Options considered

### Option A — Google Gemini
Generous free tier. Native structured output via `responseSchema`. Very large context window.

### Option B — Anthropic Claude
Strongest reasoning on security analysis in our informal comparison. Paid — no free tier adequate for development.

### Option C — OpenAI GPT
Broad ecosystem. Paid, with a small trial credit.

### Option D — Local model (Ollama + Llama/Mistral)
Free, private, offline. Requires meaningful local hardware; quality on structured security analysis is markedly weaker.

### Option E — No AI at all
Rule-based only.

## Decision

**Option A — Google Gemini**, accessed exclusively through an internal `LLMProvider` interface. `gemini-2.5-flash` for high-volume tasks, `gemini-2.5-pro` for patch generation.

## Rationale

The budget constraint decides this. Claude produces better security reasoning in our testing; that is not in dispute. But a paid API that runs out mid-sprint is worse than a free one that works, and Gemini's free tier (~1,500 flash requests/day) comfortably covers development and demo needs once caching is in place ([10 §10](../10-ai-integration.md#10-cost-model)).

Gemini's native `responseSchema` support is a concrete technical advantage rather than a tiebreaker: it enforces output shape server-side, which is the first layer of both our schema-validation requirement (FR-AI-007) and our prompt-injection defence ([10 §5](../10-ai-integration.md#5-prompt-injection-defence-fr-ai-010)). Providers where JSON conformance is a prompt instruction rather than an API guarantee require more repair logic.

Local models were rejected on hardware — not every team member has a machine that runs a useful model at acceptable speed, and "the AI features only work on one person's laptop" is not a shippable state.

Option E deserves more credit than it usually gets. The rule-based engines produce the majority of the value, and AI adds explanation and patches. We rejected it because the AI-generated patch is a genuine product differentiator and the explanation materially improves usability for the Developer persona. But the architecture treats AI as though Option E were still live: **every deterministic capability stays deterministic**, and the system is fully functional with `GUARDPIPE_AI_ENABLED=false`.

**The provider port** costs roughly 40 lines and buys disproportionate value: a deterministic offline stub for every test, a single choke point for caching, budget enforcement, retries, and injection defence, and a genuine escape hatch if the free tier changes. With a hard deadline, an escape hatch on an external dependency we do not control is cheap insurance.

## Consequences

### Positive
- Zero cost.
- Native structured output reduces parsing and repair logic.
- Large context window suits document review.
- The port gives every test a free, deterministic, offline stub.
- Caching, budget, retry, and injection defence are implemented once.
- Swapping providers is one adapter.

### Negative
- **Free-tier rate limits are real** — ~15 RPM on flash, ~2 RPM on pro. Mitigated by content-hash caching, per-scan token budgets, on-demand-only patch generation, and pre-warming the cache before the demo.
- **Quality is below Claude on security reasoning.** Accepted — mitigated by AI being advisory only and never affecting severity, score, or verdict.
- **User source code is sent to Google.** This is a real privacy cost, disclosed in-product at project creation, with a documented disable switch ([10 §13](../10-ai-integration.md#13-ethics-and-honesty-rules)).
- Free-tier terms may change without notice.
- Model IDs change; kept in configuration rather than code.

### Neutral
- The port is mild over-engineering for a single implementation. We consider it justified by the test benefit alone.

## Revisit when

- The free tier is withdrawn or materially reduced.
- Funding becomes available → Claude for patch generation, where quality matters most.
- Local models become good enough on commodity hardware → a privacy-preserving default.
