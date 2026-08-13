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
export const ENGINE_META: Record<Engine, { label: string; icon: LucideIcon; hasOsvMark?: boolean }> = {
  docreview: { label: 'Docs', icon: FileText },
  codescan: { label: 'Code', icon: Code2 },
  depscan: { label: 'Deps', icon: Package, hasOsvMark: true },
  containerscan: { label: 'Containers', icon: ContainerIcon },
  k8sscan: { label: 'K8s', icon: Boxes },
  cicdscan: { label: 'CI/CD', icon: Workflow },
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
 * "list registered engines" endpoint yet, and `internal/modules/orchestrator/registry.go`
 * only registers `depscan` as of this phase (BUILD_GUIDE.md). Update this
 * list in lockstep with `cmd/guardpipe/main.go`'s engine registration as
 * each new engine lands; a partial scan requesting an engine not in both
 * places gets rejected by the backend with `scan.engine_unavailable` (422)
 * regardless, so this is a UI convenience (disable what can't run), not the
 * source of truth.
 */
export const ENABLED_ENGINES: Engine[] = ['depscan']

export function isEngineEnabled(engine: Engine): boolean {
  return ENABLED_ENGINES.includes(engine)
}
