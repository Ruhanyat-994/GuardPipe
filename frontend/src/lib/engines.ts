import {
  Boxes,
  Code2,
  Container as ContainerIcon,
  FileText,
  Package,
  ShieldAlert,
  Workflow,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { Engine } from './rulesApi'

/**
 * The one place every one of the seven engines' label/icon is defined —
 * previously duplicated across RulesPage and SupplyChainPipeline, which had
 * drifted apart on the `hasOsvMark` flag. Both now import from here.
 */
export const ENGINE_META: Record<
  Engine,
  {
    label: string
    icon: LucideIcon
    hasOsvMark?: boolean
    hasSonarQubeMark?: boolean
    hasKubernetesMark?: boolean
    hasGitHubMark?: boolean
    hasGeminiMark?: boolean
  }
> = {
  // docreview (Phase 11) is the one engine where AI review is the entire
  // output, not an enrichment layered on top of a rule pass — hasGeminiMark
  // is the most load-bearing brand mark in the product for exactly that
  // reason (documentation/09-ui-ux-design-system.md §4.5).
  docreview: { label: 'Docs', icon: FileText, hasGeminiMark: true },
  // codescan wraps a self-hosted SonarQube instance (Phase 7, ADR-0011)
  // rather than running its own SAST — hasSonarQubeMark reverses the
  // earlier "no external brand mark" note now that one applies.
  codescan: { label: 'Code', icon: Code2, hasSonarQubeMark: true },
  depscan: { label: 'Deps', icon: Package, hasOsvMark: true },
  containerscan: { label: 'Containers', icon: ContainerIcon },
  // k8sscan (Phase 9) is GuardPipe's own rule engine, not a wrapped tool
  // (ADR-0010 unreversed) — hasKubernetesMark names the standards its rules
  // are grounded in, the same attribution role hasOsvMark plays for depscan.
  k8sscan: { label: 'K8s', icon: Boxes, hasKubernetesMark: true },
  // cicdscan (Phase 10) analyses GitHub Actions workflows specifically —
  // GitHub Actions is part of the GitHub product, so this reuses the
  // already-built GitHubMark rather than a new CI/CD-specific icon
  // (documentation/09-ui-ux-design-system.md's own reasoning, BUILD_GUIDE.md
  // Phase 10's frontend checklist).
  cicdscan: { label: 'CI/CD', icon: Workflow, hasGitHubMark: true },
  pentest: { label: 'Pentest', icon: ShieldAlert },
}

/** Every engine the product will eventually have, in execution-graph order
 * — shown everywhere so the full seven-stage pipeline is always visible,
 * even for the six that aren't runnable yet (never fake it: they're shown
 * as not-yet-available, not hidden). */
export const ALL_ENGINES: Engine[] = [
  'docreview',
  'codescan',
  'depscan',
  'containerscan',
  'k8sscan',
  'cicdscan',
  'pentest',
]

/**
 * Which engines are actually registered on the backend and can be run
 * today. Hardcoded here rather than queried — the orchestrator has no
 * "list registered engines" endpoint yet, and `cmd/guardpipe/main.go`
 * registers `depscan`, `codescan`, and `containerscan` as of this phase
 * (BUILD_GUIDE.md Phase 8). Update this list in lockstep with that
 * registration as each new engine lands — missing this once already caused
 * a real, confusing bug: "Run all scans" ran `containerscan` correctly
 * (that path just asks the backend for everything registered), but its
 * individual checkbox stayed greyed out here until this list caught up,
 * making it look broken when it wasn't. A partial scan requesting an engine
 * not in both places gets rejected by the backend with
 * `scan.engine_unavailable` (422) regardless, so this is a UI convenience
 * (disable what can't run), not the source of truth.
 */
export const ENABLED_ENGINES: Engine[] = [
  'depscan',
  'codescan',
  'containerscan',
  'k8sscan',
  'cicdscan',
  'docreview',
]

export function isEngineEnabled(engine: Engine): boolean {
  return ENABLED_ENGINES.includes(engine)
}
