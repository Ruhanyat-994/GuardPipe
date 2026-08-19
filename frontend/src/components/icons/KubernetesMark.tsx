import { Ship } from 'lucide-react'
import { cn } from '../../lib/cn'

/**
 * Kubernetes attribution mark (documentation/09-ui-ux-design-system.md
 * §4.5, BUILD_GUIDE.md Phase 9) — a labelled badge rather than a traced
 * brand logomark, same reasoning as OsvMark: this session has no verified
 * source artwork for Kubernetes' actual helm-wheel logo to trace
 * accurately, and a guessed-at SVG claiming to be a real project's mark
 * would be worse than a plain, honestly-labelled badge. Unlike
 * SonarQubeMark/TrivyMark, k8sscan doesn't wrap an external tool (ADR-0010
 * is unreversed for this engine) — this mark just names the domain the
 * engine's own rules are grounded in (Pod Security Standards, the CIS
 * Kubernetes Benchmark, NSA/CISA hardening guidance), the same way OsvMark
 * names where depscan's advisory data came from.
 */
export function KubernetesMark({ className }: { className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full bg-bg-subtle px-1.5 py-0.5 text-caption font-semibold text-text-secondary',
        className,
      )}
      title="Kubernetes manifest policy — Pod Security Standards, CIS Benchmark, NSA/CISA guidance"
    >
      <Ship className="h-3 w-3" aria-hidden="true" />
      K8s
    </span>
  )
}
