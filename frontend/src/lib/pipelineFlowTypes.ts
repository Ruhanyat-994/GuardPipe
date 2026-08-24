import type { LucideIcon } from 'lucide-react'
import type { Node } from '@xyflow/react'

/** Shared node-shape types/constants for SupplyChainPipeline's draggable
 * canvas — split out from `components/project/PipelineNode.tsx` so that
 * file exports components only (react-refresh/only-export-components). */

export type NodeStatus = 'not_run' | 'running' | 'succeeded' | 'failed' | 'skipped'

export const STATUS_COLOR: Record<NodeStatus, string> = {
  not_run: 'var(--text-tertiary)',
  running: 'var(--accent)',
  succeeded: 'var(--success)',
  failed: 'var(--danger)',
  skipped: 'var(--text-tertiary)',
}

export const STATUS_LABEL: Record<NodeStatus, string> = {
  not_run: 'Not run',
  running: 'In progress',
  succeeded: 'Succeeded',
  failed: 'Failed',
  skipped: 'Skipped',
}

export interface PipelineNodeData extends Record<string, unknown> {
  label: string
  icon: LucideIcon
  status: NodeStatus
  statusIcon: LucideIcon
  osvMark?: boolean
  sonarQubeMark?: boolean
  kubernetesMark?: boolean
  githubMark?: boolean
  geminiMark?: boolean
  tooltip?: string | null
  onClick?: () => void
  selected?: boolean
}

export type PipelineFlowNode = Node<PipelineNodeData, 'pipelineNode'>

export interface GroupLabelData extends Record<string, unknown> {
  label: string
  width: number
  height: number
}

export type GroupLabelFlowNode = Node<GroupLabelData, 'groupLabel'>
