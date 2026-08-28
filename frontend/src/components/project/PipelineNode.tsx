import { Handle, Position, type NodeProps } from '@xyflow/react'
import { cn } from '../../lib/cn'
import {
  STATUS_COLOR,
  STATUS_LABEL,
  type GroupLabelFlowNode,
  type PipelineFlowNode,
} from '../../lib/pipelineFlowTypes'
import { GeminiMark } from '../icons/GeminiMark'
import { GitHubMark } from '../icons/GitHubMark'
import { KubernetesMark } from '../icons/KubernetesMark'
import { OsvMark } from '../icons/OsvMark'
import { SonarQubeMark } from '../icons/SonarQubeMark'

/** Invisible connection anchors on every side — `nodesConnectable={false}`
 * on the canvas means these are never draggable-from by the user, they only
 * give React Flow's floating-edge math a real DOM handle to satisfy edge
 * validity. Actual attach points are computed from node geometry
 * (FloatingEdge), not from these fixed positions. */
function InvisibleHandles() {
  return (
    <>
      <Handle type="source" position={Position.Top} style={{ opacity: 0 }} />
      <Handle type="target" position={Position.Bottom} style={{ opacity: 0 }} />
    </>
  )
}

/** The same card visual SupplyChainPipeline's old `Node()` rendered — kept
 * pixel-for-pixel (icon circle, label + brand marks, status line) per the
 * task's "keep the existing nodes and their current visual design as much
 * as possible." Only the outer wiring changed: this is now a React Flow
 * node type, dragged by the canvas rather than laid out by flexbox. */
export function PipelineNode({ data, dragging }: NodeProps<PipelineFlowNode>) {
  const { label, icon: Icon, status, statusIcon: StatusIcon, tooltip, onClick, selected } = data
  const color = STATUS_COLOR[status]
  const Tag = onClick ? 'button' : 'div'

  return (
    <>
      <InvisibleHandles />
      <Tag
        type={onClick ? 'button' : undefined}
        onClick={onClick}
        aria-pressed={onClick ? selected : undefined}
        title={tooltip ?? undefined}
        className={cn(
          'sc-node-card relative z-[1] flex w-[116px] shrink-0 flex-col items-center gap-2 rounded-xl border bg-bg-surface px-3 py-3.5 text-center shadow-sm transition-[border-color,box-shadow,transform] duration-150',
          onClick && 'cursor-pointer hover:border-accent/50 hover:shadow-md',
          dragging && 'shadow-lg',
        )}
        style={{
          borderColor: selected
            ? 'var(--accent)'
            : status === 'not_run'
              ? 'var(--border-default)'
              : `color-mix(in srgb, ${color} 45%, transparent)`,
          boxShadow: selected
            ? '0 0 0 2px color-mix(in srgb, var(--accent) 30%, transparent)'
            : undefined,
          transform: dragging ? 'scale(1.04)' : undefined,
        }}
      >
        <div
          className="flex h-10 w-10 items-center justify-center rounded-full"
          style={{ backgroundColor: `color-mix(in srgb, ${color} 15%, transparent)`, color }}
        >
          <Icon className="h-5 w-5" aria-hidden="true" />
        </div>
        <div className="flex items-center gap-1">
          <span className="text-body-sm font-semibold text-text-primary">{label}</span>
          {data.osvMark && <OsvMark />}
          {data.sonarQubeMark && <SonarQubeMark />}
          {data.kubernetesMark && <KubernetesMark />}
          {data.githubMark && <GitHubMark className="h-3 w-3" />}
          {data.geminiMark && <GeminiMark />}
        </div>
        <span
          className="flex items-center gap-1 text-caption font-medium capitalize"
          style={{ color }}
        >
          <StatusIcon
            className={cn('h-3 w-3', status === 'running' && 'animate-spin')}
            aria-hidden="true"
          />
          {STATUS_LABEL[status]}
        </span>
      </Tag>
    </>
  )
}

/** Purely decorative echo of the scanner cards' original bordered container
 * (task §7/§6: preserve that grouping on initial load). Not draggable, not
 * selectable, no handles — it doesn't participate in the connection graph
 * at all, it just sits behind the six scanner nodes' starting position. */
export function GroupLabelNode({ data }: NodeProps<GroupLabelFlowNode>) {
  return (
    <div
      className="rounded-xl border border-border-default bg-bg-subtle/60"
      style={{ width: data.width, height: data.height }}
    >
      <span className="ml-3 mt-2 inline-block text-caption font-semibold uppercase tracking-wide text-text-tertiary">
        {data.label}
      </span>
    </div>
  )
}
