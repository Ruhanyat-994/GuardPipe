import { Sparkles } from 'lucide-react'
import { cn } from '../../lib/cn'
import { PatchDiff } from './PatchDiff'
import type { AISuggestion } from '../../lib/scansApi'

/**
 * `AiPanel` (documentation/09-ui-ux-design-system.md §4.2): loading ·
 * content · unavailable · budget-exhausted — purple left border,
 * "AI-generated" chip. Only three states are actually driveable today:
 * `GET /findings/{id}`'s `ai_suggestion` field is nil for every reason a
 * finding might have no explanation yet (never enriched because it's low/
 * informational severity, the per-scan budget ran out, or the provider
 * call genuinely failed) — the API doesn't distinguish "budget-exhausted"
 * from any other "unavailable" cause, so this component doesn't invent a
 * fourth state it can't actually tell apart from the third.
 */
export type AiPanelState = 'loading' | 'content' | 'unavailable'

export function AiPanel({
  state,
  suggestion,
  className,
}: {
  state: AiPanelState
  suggestion?: AISuggestion | null
  className?: string
}) {
  return (
    <div className={cn('rounded-md border-l-4 border-ai bg-ai/5 p-4', className)}>
      <span className="inline-flex items-center gap-1.5 rounded-full bg-ai/10 px-2 py-0.5 text-caption font-semibold text-ai">
        <Sparkles className="h-3 w-3" aria-hidden="true" />
        AI-generated
      </span>

      {state === 'loading' && (
        <div className="mt-3 flex flex-col gap-2" aria-busy="true">
          <div className="h-3 w-full animate-pulse rounded bg-bg-subtle" />
          <div className="h-3 w-5/6 animate-pulse rounded bg-bg-subtle" />
          <div className="h-3 w-2/3 animate-pulse rounded bg-bg-subtle" />
        </div>
      )}

      {state === 'unavailable' && (
        <p className="mt-2 text-body-sm text-text-tertiary">
          AI explanation unavailable for this finding. The remediation guidance above is
          deterministic and stands on its own.
        </p>
      )}

      {state === 'content' && suggestion && (
        <div className="mt-2 flex flex-col gap-3">
          {suggestion.explanation && (
            <p className="text-body-sm whitespace-pre-wrap text-text-primary">
              {suggestion.explanation}
            </p>
          )}
          {suggestion.patch_diff && (
            <PatchDiff diff={suggestion.patch_diff} patchStatus={suggestion.patch_status} />
          )}
        </div>
      )}
    </div>
  )
}
