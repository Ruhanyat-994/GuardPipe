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
import type { Project } from './projectsApi'

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
    // A short, deliberately generic present-tense phrase for "what this
    // engine is doing right now" — used by SupplyChainPipeline's live node
    // detail and EngineRunDetail (documentation/09-ui-ux-design-system.md
    // §4.8). Names the category of check only (already public via this same
    // label/icon and the Rules catalogue page), never a specific rule ID,
    // pattern, or detection technique — the whole point is a scanner can
    // show it's genuinely working without narrating its own rule logic.
    activity: string
    // Whether this engine structurally needs a repository checkout to have
    // anything to look at (backend: internal/engines/*/engine.go's
    // Applicable() walks WorkspaceDir) or an attested pentest target
    // (pentest's Applicable() checks in.Target != nil). A project missing
    // the thing an engine requires never gets that engine's job created at
    // all (internal/modules/orchestrator/service.go's resolveEngines) — this
    // is the same true/false shape mirrored client-side so the picker can
    // explain *why* a box is unavailable instead of just greying it out.
    // docreview is neither: it degrades to "no repo documents" cleanly and
    // reads uploaded documents regardless (internal/engines/docreview).
    requiresRepo: boolean
    requiresTarget: boolean
  }
> = {
  // docreview (Phase 11) is the one engine where AI review is the entire
  // output, not an enrichment layered on top of a rule pass — hasGeminiMark
  // is the most load-bearing brand mark in the product for exactly that
  // reason (documentation/09-ui-ux-design-system.md §4.5).
  docreview: {
    label: 'Docs',
    icon: FileText,
    hasGeminiMark: true,
    activity: 'Reviewing design and documentation content',
    requiresRepo: false,
    requiresTarget: false,
  },
  // codescan wraps a self-hosted SonarQube instance (Phase 7, ADR-0011)
  // rather than running its own SAST — hasSonarQubeMark reverses the
  // earlier "no external brand mark" note now that one applies.
  codescan: {
    label: 'Code',
    icon: Code2,
    hasSonarQubeMark: true,
    activity: 'Scanning source code for security issues',
    requiresRepo: true,
    requiresTarget: false,
  },
  depscan: {
    label: 'Deps',
    icon: Package,
    hasOsvMark: true,
    activity: 'Checking dependencies against known advisories',
    requiresRepo: true,
    requiresTarget: false,
  },
  containerscan: {
    label: 'Containers',
    icon: ContainerIcon,
    activity: 'Analyzing the Dockerfile and container image',
    requiresRepo: true,
    requiresTarget: false,
  },
  // k8sscan (Phase 9) is GuardPipe's own rule engine, not a wrapped tool
  // (ADR-0010 unreversed) — hasKubernetesMark names the standards its rules
  // are grounded in, the same attribution role hasOsvMark plays for depscan.
  k8sscan: {
    label: 'K8s',
    icon: Boxes,
    hasKubernetesMark: true,
    activity: 'Evaluating Kubernetes manifests and Helm charts',
    requiresRepo: true,
    requiresTarget: false,
  },
  // cicdscan (Phase 10) analyses GitHub Actions workflows specifically —
  // GitHub Actions is part of the GitHub product, so this reuses the
  // already-built GitHubMark rather than a new CI/CD-specific icon
  // (documentation/09-ui-ux-design-system.md's own reasoning, BUILD_GUIDE.md
  // Phase 10's frontend checklist).
  cicdscan: {
    label: 'CI/CD',
    icon: Workflow,
    hasGitHubMark: true,
    activity: 'Auditing CI/CD workflow configuration',
    requiresRepo: true,
    requiresTarget: false,
  },
  pentest: {
    label: 'Pentest',
    icon: ShieldAlert,
    activity: 'Probing the attested target',
    requiresRepo: false,
    requiresTarget: true,
  },
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
  'pentest',
]

export function isEngineEnabled(engine: Engine): boolean {
  return ENABLED_ENGINES.includes(engine)
}

/** The subset of a `Project` this module actually needs — narrower than the
 * full type so a caller building one from a scan's own `project_id` lookup
 * doesn't have to have every field on hand. */
type ProjectShape = Pick<Project, 'repository' | 'has_pentest_target'>

/**
 * Whether `engine` can structurally run against `project` today — combines
 * `isEngineEnabled` (is it registered on the backend at all) with the same
 * repo/target precondition `resolveEngines` enforces server-side
 * (internal/modules/orchestrator/service.go): a repo-based engine needs
 * `project.repository`, pentest needs an attested target. A project with
 * only a pentest target (no repository attached) therefore never shows a
 * runnable checkbox for depscan/codescan/containerscan/k8sscan/cicdscan —
 * only pentest (and docreview, which tolerates having no repo). Attaching a
 * repository later flips this back to true the moment the project is
 * re-fetched, no other state change needed.
 *
 * Deliberately still just a boolean gate, not a reason to hide the engine
 * entirely — this codebase's own stated design principle (SupplyChainPipeline
 * and this module's own ENABLED_ENGINES doc comment) is "never hide the
 * pipeline's true shape," so callers should keep rendering every engine and
 * use `engineRequirementReason` below to explain *why* one is disabled,
 * the same treatment already used for "not built yet."
 */
export function isEngineRunnable(engine: Engine, project: ProjectShape): boolean {
  if (!isEngineEnabled(engine)) return false
  const meta = ENGINE_META[engine]
  if (meta.requiresRepo && !project.repository) return false
  if (meta.requiresTarget && !project.has_pentest_target) return false
  return true
}

/** A short, plain-language reason `engine` can't run against `project` right
 * now, or `null` if it can (or if the reason is simply "not built yet" —
 * callers already have their own copy for that case via `isEngineEnabled`). */
export function engineRequirementReason(engine: Engine, project: ProjectShape): string | null {
  if (!isEngineEnabled(engine)) return null
  const meta = ENGINE_META[engine]
  if (meta.requiresRepo && !project.repository) {
    return 'Attach a GitHub repository to run this'
  }
  if (meta.requiresTarget && !project.has_pentest_target) {
    return 'Register a pentest target to run this'
  }
  return null
}
