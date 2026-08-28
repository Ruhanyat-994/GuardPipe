import { useCallback, useMemo, useState } from 'react'
import {
  applyNodeChanges,
  Background,
  BackgroundVariant,
  ConnectionMode,
  Controls,
  MarkerType,
  ReactFlow,
  type Edge,
  type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import {
  CheckCircle2,
  Circle,
  FileText,
  GitBranch,
  Loader2,
  MinusCircle,
  Package,
  ShieldAlert,
  XCircle,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { engineRequirementReason, ENGINE_META, isEngineRunnable } from '../../lib/engines'
import type { Project } from '../../lib/projectsApi'
import type { Engine } from '../../lib/rulesApi'
import type { Job, JobStatus, Progress, Scan } from '../../lib/scansApi'
import { EngineRunDetail } from './EngineRunDetail'
import { FloatingEdge } from './FloatingEdge'
import { GroupLabelNode, PipelineNode } from './PipelineNode'
import { ScanProgressBar } from './ScanProgressBar'
import {
  STATUS_COLOR,
  STATUS_LABEL,
  type GroupLabelFlowNode,
  type NodeStatus,
  type PipelineFlowNode,
  type PipelineNodeData,
} from '../../lib/pipelineFlowTypes'

/**
 * The live scan execution graph (documentation/09-ui-ux-design-system.md
 * §4.8) — the orchestrator's own execution DAG (workspace prep fans out to
 * six parallel engines; pentest branches independently straight off scan
 * start since it needs no workspace; all seven converge into ai enrichment
 * -> scoring), rendered as a draggable, pannable, zoomable canvas
 * (@xyflow/react — already the project's chosen graph library for this,
 * nothing else in `package.json` overlaps it) instead of a fixed flexbox
 * row. Every node keeps the exact same status/click wiring it always had —
 * only *where it sits* is now something the user controls; which nodes
 * connect to which never changes, only where the edge is drawn (task's own
 * framing). Driven entirely by `GET /scans/{id}/progress`, polled every 2s
 * (FR-UI-002) — no new backend data needed.
 *
 * Every node in the fixed shape renders even when its engine hasn't run —
 * Phase 6 only registers depscan, so every other engine node legitimately
 * shows `not_run` this phase. That's the true state (they didn't run),
 * not a placeholder standing in for missing backend work — exactly the
 * distinction this design system's "never fake it" rule cares about
 * (GlobalSearch/NotificationPanel, documentation/09 §4.4). For the same
 * reason, the header only surfaces fields the scan actually has (id, type,
 * branch, queued time, status) — no fabricated "started by"/commit/CI-run
 * chrome the backend doesn't produce.
 */

const STATUS_ICON: Record<NodeStatus, LucideIcon> = {
  not_run: Circle,
  running: Loader2,
  succeeded: CheckCircle2,
  failed: XCircle,
  skipped: MinusCircle,
}

const PARALLEL_ENGINES: Engine[] = [
  'docreview',
  'codescan',
  'depscan',
  'containerscan',
  'k8sscan',
  'cicdscan',
]

function jobStatusToNodeStatus(status: JobStatus): NodeStatus {
  switch (status) {
    case 'queued':
      return 'not_run'
    case 'running':
      return 'running'
    case 'succeeded':
      return 'succeeded'
    case 'failed':
      return 'failed'
    case 'skipped':
    case 'cancelled':
      return 'skipped'
  }
}

interface EngineNodeState {
  status: NodeStatus
  findingCount: number
  reason: string | null
}

function resolveEngineState(
  engine: Engine,
  progress: Progress | null,
  jobs: Job[],
): EngineNodeState {
  const live = progress?.engines.find((e) => e.engine === engine)
  if (live) {
    return {
      status: jobStatusToNodeStatus(live.status),
      findingCount: live.finding_count,
      reason: null,
    }
  }
  const job = jobs.find((j) => j.engine === engine)
  if (job) {
    return {
      status: jobStatusToNodeStatus(job.status),
      findingCount: job.finding_count,
      reason: job.error_reason ?? job.skip_reason,
    }
  }
  return { status: 'not_run', findingCount: 0, reason: null }
}

function StatusBadge({ status, size = 'md' }: { status: NodeStatus; size?: 'sm' | 'md' }) {
  const color = STATUS_COLOR[status]
  const Icon = STATUS_ICON[status]
  return (
    <span
      className={
        'inline-flex items-center gap-1.5 rounded-full font-semibold capitalize ' +
        (size === 'md' ? 'px-3 py-1 text-body-sm' : 'text-caption')
      }
      style={{
        color,
        backgroundColor:
          size === 'md' ? `color-mix(in srgb, ${color} 12%, transparent)` : undefined,
      }}
    >
      <Icon
        className={'h-3.5 w-3.5' + (status === 'running' ? ' animate-spin' : '')}
        aria-hidden="true"
      />
      {STATUS_LABEL[status]}
    </span>
  )
}

function overallStatus(scanStarted: boolean, progress: Progress | null, jobs: Job[]): NodeStatus {
  if (!scanStarted) return 'not_run'
  if (progress) {
    switch (progress.status) {
      case 'queued':
        return 'not_run'
      case 'running':
        return 'running'
      case 'completed':
        return 'succeeded'
      case 'failed':
        return 'failed'
      case 'cancelled':
        return 'skipped'
    }
  }
  if (jobs.some((j) => j.status === 'failed')) return 'failed'
  if (jobs.every((j) => j.status !== 'queued' && j.status !== 'running')) return 'succeeded'
  return 'running'
}

const LEGEND: { status: NodeStatus; label: string }[] = [
  { status: 'succeeded', label: 'Succeeded' },
  { status: 'not_run', label: 'Not run' },
  { status: 'running', label: 'In progress' },
  { status: 'failed', label: 'Failed' },
  { status: 'skipped', label: 'Skipped' },
]

/* ---- Canvas layout (initial positions only — the user is free to drag
   every node anywhere afterward; nothing here is re-applied once mounted).
   Roughly mirrors the original flexbox shape: scan start -> workspace prep
   -> a 2-column scanner grid -> ai enrichment -> scoring, left to right,
   with pentest branching independently below scan start. */

const NODE_W = 116
const NODE_H = 132
const GAP_X = 90
const GAP_Y = 24
const GROUP_PAD = 20
const GROUP_GAP = 16

const GROUP_POS = { x: NODE_W * 2 + GAP_X * 2, y: 0 }
const GROUP_SIZE = {
  width: GROUP_PAD * 2 + NODE_W * 2 + GROUP_GAP,
  height: GROUP_PAD * 2 + NODE_H * 3 + GAP_Y * 2,
}
const STAGE_CENTER_Y = GROUP_SIZE.height / 2 - NODE_H / 2

const STAGE_POSITIONS: Record<
  'scanStart' | 'workspacePrep' | 'aiEnrichment' | 'scoring',
  {
    x: number
    y: number
  }
> = {
  scanStart: { x: 0, y: STAGE_CENTER_Y },
  workspacePrep: { x: NODE_W + GAP_X, y: STAGE_CENTER_Y },
  aiEnrichment: { x: GROUP_POS.x + GROUP_SIZE.width + GAP_X, y: STAGE_CENTER_Y },
  scoring: {
    x: GROUP_POS.x + GROUP_SIZE.width + GAP_X * 2 + NODE_W,
    y: STAGE_CENTER_Y,
  },
}

const PENTEST_POS = { x: 0, y: STAGE_CENTER_Y + NODE_H + 60 }

const ENGINE_POSITIONS: Record<string, { x: number; y: number }> = {}
PARALLEL_ENGINES.forEach((engine, i) => {
  const col = i % 2
  const row = Math.floor(i / 2)
  ENGINE_POSITIONS[engine] = {
    x: GROUP_POS.x + GROUP_PAD + col * (NODE_W + GROUP_GAP),
    y: GROUP_POS.y + GROUP_PAD + row * (NODE_H + GAP_Y),
  }
})

type FlowNode = PipelineFlowNode | GroupLabelFlowNode

function buildInitialNodes(data: Record<string, PipelineNodeData>): FlowNode[] {
  const group: GroupLabelFlowNode = {
    id: 'scanner-group',
    type: 'groupLabel',
    position: GROUP_POS,
    data: { label: 'Scanners', width: GROUP_SIZE.width, height: GROUP_SIZE.height },
    draggable: false,
    selectable: false,
    zIndex: 0,
  }

  const stageNode = (id: string, position: { x: number; y: number }): PipelineFlowNode => ({
    id,
    type: 'pipelineNode',
    position,
    data: data[id],
    zIndex: 1,
  })

  const nodes: FlowNode[] = [
    group,
    stageNode('scan-start', STAGE_POSITIONS.scanStart),
    stageNode('workspace-prep', STAGE_POSITIONS.workspacePrep),
    ...PARALLEL_ENGINES.map((engine) => stageNode(engine, ENGINE_POSITIONS[engine])),
    stageNode('ai-enrichment', STAGE_POSITIONS.aiEnrichment),
    stageNode('scoring', STAGE_POSITIONS.scoring),
    stageNode('pentest', PENTEST_POS),
  ]
  return nodes
}

const nodeTypes = { pipelineNode: PipelineNode, groupLabel: GroupLabelNode }
const edgeTypes = { floating: FloatingEdge }

const arrow = { type: MarkerType.ArrowClosed, color: 'var(--border-strong)', width: 14, height: 14 }

const EDGES: Edge[] = [
  { id: 'e-start-workspace', source: 'scan-start', target: 'workspace-prep' },
  ...PARALLEL_ENGINES.flatMap((engine) => [
    { id: `e-workspace-${engine}`, source: 'workspace-prep', target: engine },
    { id: `e-${engine}-ai`, source: engine, target: 'ai-enrichment' },
  ]),
  { id: 'e-start-pentest', source: 'scan-start', target: 'pentest', data: { dashed: true } },
  { id: 'e-pentest-ai', source: 'pentest', target: 'ai-enrichment', data: { dashed: true } },
  { id: 'e-ai-scoring', source: 'ai-enrichment', target: 'scoring' },
].map((e) => ({ ...e, type: 'floating', markerEnd: arrow }))

export function SupplyChainPipeline({
  progress,
  jobs,
  scan,
  project,
}: {
  progress: Progress | null
  jobs: Job[]
  scan?: Pick<Scan, 'id' | 'type' | 'branch' | 'queued_at'>
  // Optional — a caller without the project handy yet (e.g. mid-fetch) just
  // gets no repo/target-based disabling this render; every node still shows
  // its real job status either way.
  project?: Pick<Project, 'repository' | 'has_pentest_target'> | null
}) {
  const scanStarted = progress !== null || jobs.length > 0
  const anyRunningOrDone =
    jobs.some((j) => j.status !== 'queued') || (progress?.progress_pct ?? 0) > 0
  const workspaceStatus: NodeStatus = !scanStarted
    ? 'not_run'
    : anyRunningOrDone
      ? 'succeeded'
      : 'running'

  const pentestState = resolveEngineState('pentest', progress, jobs)
  const status = overallStatus(scanStarted, progress, jobs)

  // Only nodes with a real per-engine job to drill into are clickable —
  // "Scan start"/"Workspace prep"/"AI enrichment"/"Scoring" have no job of
  // their own (workspace prep is shared, the other two don't exist as
  // engines yet), so making them clickable would mean either faking data
  // for them or opening a panel that just says "nothing here," neither of
  // which earns a click. Same `isEngineEnabled` gate ScanLauncher already
  // uses for "can this engine even run today."
  const [selectedEngine, setSelectedEngine] = useState<Engine | null>(null)
  const toggleEngine = useCallback((engine: Engine) => {
    setSelectedEngine((current) => (current === engine ? null : engine))
  }, [])

  // Recomputed on every status tick, but never touches node *position* —
  // dragging a node is state this component owns independently (`nodes`
  // below), so a 2s progress poll can't ever snap a moved card back.
  const computedData = useMemo(() => {
    const data: Record<string, PipelineNodeData> = {}
    data['scan-start'] = {
      label: 'Scan start',
      icon: ShieldAlert,
      status: scanStarted ? 'succeeded' : 'not_run',
      statusIcon: STATUS_ICON[scanStarted ? 'succeeded' : 'not_run'],
    }
    data['workspace-prep'] = {
      label: 'Workspace prep',
      icon: Package,
      status: workspaceStatus,
      statusIcon: STATUS_ICON[workspaceStatus],
    }
    for (const engine of PARALLEL_ENGINES) {
      const meta = ENGINE_META[engine]
      const state = resolveEngineState(engine, progress, jobs)
      const runnable = project ? isEngineRunnable(engine, project) : true
      const clickable = scanStarted && runnable
      const requirementReason = project ? engineRequirementReason(engine, project) : null
      data[engine] = {
        label: meta.label,
        icon: meta.icon,
        status: state.status,
        statusIcon: STATUS_ICON[state.status],
        osvMark: meta.hasOsvMark,
        sonarQubeMark: meta.hasSonarQubeMark,
        kubernetesMark: meta.hasKubernetesMark,
        githubMark: meta.hasGitHubMark,
        geminiMark: meta.hasGeminiMark,
        tooltip: state.reason ?? requirementReason,
        onClick: clickable ? () => toggleEngine(engine) : undefined,
        selected: selectedEngine === engine,
      }
    }
    data['ai-enrichment'] = {
      label: 'AI enrichment',
      icon: FileText,
      status: 'not_run',
      statusIcon: STATUS_ICON.not_run,
      tooltip: 'Lands in Phase 10/11',
    }
    data['scoring'] = {
      label: 'Scoring',
      icon: ShieldAlert,
      status: 'not_run',
      statusIcon: STATUS_ICON.not_run,
      tooltip: 'Lands in Phase 13',
    }
    data['pentest'] = {
      label: 'Pentest',
      icon: ShieldAlert,
      status: pentestState.status,
      statusIcon: STATUS_ICON[pentestState.status],
      tooltip:
        pentestState.reason ?? (project ? engineRequirementReason('pentest', project) : null),
      onClick:
        scanStarted && (project ? isEngineRunnable('pentest', project) : true)
          ? () => toggleEngine('pentest')
          : undefined,
      selected: selectedEngine === 'pentest',
    }
    return data
  }, [
    scanStarted,
    workspaceStatus,
    progress,
    jobs,
    project,
    selectedEngine,
    pentestState.status,
    pentestState.reason,
    toggleEngine,
  ])

  // `rawNodes` owns position (and drag/selection state) exclusively — the
  // only thing that ever changes it is `onNodesChange`, i.e. the user
  // dragging a card. Status/click `data` is merged in below as a pure
  // derivation on every render instead of being synced via an effect, so a
  // 2s progress poll can update a card's status without ever touching
  // where the user put it.
  const [rawNodes, setRawNodes] = useState<FlowNode[]>(() => buildInitialNodes(computedData))

  const onNodesChange = useCallback((changes: NodeChange<FlowNode>[]) => {
    setRawNodes((nds) => applyNodeChanges(changes, nds) as FlowNode[])
  }, [])

  const nodes = useMemo(
    () =>
      rawNodes.map((n) => {
        if (n.type !== 'pipelineNode') return n
        const data = computedData[n.id]
        return data ? { ...n, data } : n
      }),
    [rawNodes, computedData],
  )

  return (
    <div className="overflow-hidden rounded-lg border border-border-default bg-bg-surface">
      {/* header bar */}
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border-default px-5 py-3.5">
        <div className="flex items-center gap-2 rounded-md border border-border-default bg-bg-subtle px-3 py-1.5 text-body-sm text-text-secondary">
          <GitBranch className="h-3.5 w-3.5" aria-hidden="true" />
          <span className="font-medium text-text-primary">Security Pipeline</span>
        </div>
        <StatusBadge status={status} />
      </div>

      {/* Real, live overall progress — the real fraction of this scan's jobs
          that have reached a terminal state (orchestrator's own GetProgress),
          not a fabricated animation. Shown for every scan, not just pentest,
          from the moment it starts until it's fully done. */}
      {scanStarted && (
        <div className="border-b border-border-default px-5 py-3">
          <ScanProgressBar pct={progress?.progress_pct ?? 0} />
        </div>
      )}

      {/* graph — a draggable, pannable, zoomable canvas (task: "turn the
          static workflow into a smooth, interactive, draggable canvas").
          Edges are computed from live node geometry (FloatingEdge), so
          dragging any card — including the six scanner nodes out of their
          initial grouping — never leaves a disconnected-looking line. */}
      <div className="sc-canvas border-b border-border-default" style={{ height: 560 }}>
        <ReactFlow
          nodes={nodes}
          edges={EDGES}
          onNodesChange={onNodesChange}
          nodeTypes={nodeTypes}
          edgeTypes={edgeTypes}
          connectionMode={ConnectionMode.Loose}
          nodesConnectable={false}
          deleteKeyCode={null}
          minZoom={0.35}
          maxZoom={1.5}
          fitView
          fitViewOptions={{ padding: 0.25 }}
          proOptions={{ hideAttribution: true }}
        >
          <Background
            variant={BackgroundVariant.Dots}
            gap={18}
            size={1}
            color="var(--border-default)"
          />
          <Controls showInteractive={false} position="bottom-left" />
        </ReactFlow>
      </div>

      {selectedEngine && (
        <div className="px-6 py-6">
          <EngineRunDetail
            engine={selectedEngine}
            job={jobs.find((j) => j.engine === selectedEngine)}
            liveProgress={progress?.engines.find((e) => e.engine === selectedEngine)}
            onClose={() => setSelectedEngine(null)}
          />
        </div>
      )}

      {/* footer: legend + real scan info, no fabricated fields */}
      <div className="flex flex-wrap items-start justify-between gap-6 border-t border-border-default bg-bg-subtle px-5 py-4">
        <div className="flex flex-col gap-1.5">
          <span className="text-caption font-semibold uppercase tracking-wide text-text-tertiary">
            Legend
          </span>
          <div className="flex flex-wrap gap-x-4 gap-y-1.5">
            {LEGEND.map(({ status: s, label }) => {
              const Icon = STATUS_ICON[s]
              return (
                <span
                  key={s}
                  className="flex items-center gap-1.5 text-caption text-text-secondary"
                >
                  <Icon className="h-3 w-3" style={{ color: STATUS_COLOR[s] }} aria-hidden="true" />
                  {label}
                </span>
              )
            })}
            <span className="flex items-center gap-1.5 text-caption text-text-secondary">
              <span
                className="inline-block h-px w-4 border-t border-dashed border-border-strong"
                aria-hidden="true"
              />
              Runs independently
            </span>
          </div>
        </div>

        {scan && (
          <div className="flex flex-col gap-1 text-caption">
            <span className="font-semibold uppercase tracking-wide text-text-tertiary">Scan</span>
            <div className="grid grid-cols-[auto_auto] gap-x-3 gap-y-0.5 text-text-secondary">
              <span>ID</span>
              <span className="font-mono text-text-primary">{scan.id.slice(0, 8)}</span>
              <span>Type</span>
              <span className="text-text-primary">{scan.type.replace(/_/g, ' ')}</span>
              {scan.branch && (
                <>
                  <span>Branch</span>
                  <span className="text-text-primary">{scan.branch}</span>
                </>
              )}
              <span>Queued</span>
              <span className="text-text-primary">{new Date(scan.queued_at).toLocaleString()}</span>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
