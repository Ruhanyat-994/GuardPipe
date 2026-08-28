import {
  BaseEdge,
  getBezierPath,
  Position,
  useInternalNode,
  type EdgeProps,
  type InternalNode,
} from '@xyflow/react'

/**
 * Connections computed from actual node geometry (task: "The connections
 * must be calculated from the actual node positions", not static divs/CSS)
 * rather than fixed handle positions — every workflow node can be dragged
 * anywhere, including above/below/diagonal from its neighbours, so a fixed
 * "always leaves from the right side" handle would draw lines through node
 * bodies the moment a node crosses its neighbour vertically. This is
 * xyflow's own documented "floating edge" recipe: find where the straight
 * line between the two nodes' centers crosses each node's border, and draw
 * the curve between those two border points instead of fixed handle dots.
 */

function getNodeIntersection(intersectionNode: InternalNode, targetNode: InternalNode) {
  const width = intersectionNode.measured.width ?? 0
  const height = intersectionNode.measured.height ?? 0
  const intersectionNodePosition = intersectionNode.internals.positionAbsolute
  const targetPosition = targetNode.internals.positionAbsolute

  const w = width / 2
  const h = height / 2

  const x2 = intersectionNodePosition.x + w
  const y2 = intersectionNodePosition.y + h
  const x1 = targetPosition.x + (targetNode.measured.width ?? 0) / 2
  const y1 = targetPosition.y + (targetNode.measured.height ?? 0) / 2

  const xx1 = (x1 - x2) / (2 * w) - (y1 - y2) / (2 * h)
  const yy1 = (x1 - x2) / (2 * w) + (y1 - y2) / (2 * h)
  const a = 1 / (Math.abs(xx1) + Math.abs(yy1) || 1)
  const xx3 = a * xx1
  const yy3 = a * yy1
  const x = w * (xx3 + yy3) + x2
  const y = h * (-xx3 + yy3) + y2

  return { x, y }
}

function getEdgePosition(node: InternalNode, intersectionPoint: { x: number; y: number }) {
  const n = node.internals.positionAbsolute
  const width = node.measured.width ?? 0
  const height = node.measured.height ?? 0
  const nx = Math.round(n.x)
  const ny = Math.round(n.y)
  const px = Math.round(intersectionPoint.x)
  const py = Math.round(intersectionPoint.y)

  if (px <= nx + 1) return Position.Left
  if (px >= nx + width - 1) return Position.Right
  if (py <= ny + 1) return Position.Top
  if (py >= ny + height - 1) return Position.Bottom
  return Position.Top
}

function getEdgeParams(source: InternalNode, target: InternalNode) {
  const sourceIntersectionPoint = getNodeIntersection(source, target)
  const targetIntersectionPoint = getNodeIntersection(target, source)
  return {
    sx: sourceIntersectionPoint.x,
    sy: sourceIntersectionPoint.y,
    tx: targetIntersectionPoint.x,
    ty: targetIntersectionPoint.y,
    sourcePos: getEdgePosition(source, sourceIntersectionPoint),
    targetPos: getEdgePosition(target, targetIntersectionPoint),
  }
}

/** dashed=true renders the "runs independently" branch (scan start →
 * pentest) — the same semantic the old dashed HConnector caption gave it,
 * now carried on the edge itself since that node can move anywhere. */
export function FloatingEdge({ id, source, target, style, markerEnd, data }: EdgeProps) {
  const sourceNode = useInternalNode(source)
  const targetNode = useInternalNode(target)

  if (!sourceNode?.measured.width || !targetNode?.measured.width) {
    return null
  }

  const { sx, sy, tx, ty, sourcePos, targetPos } = getEdgeParams(sourceNode, targetNode)
  const [path] = getBezierPath({
    sourceX: sx,
    sourceY: sy,
    sourcePosition: sourcePos,
    targetX: tx,
    targetY: ty,
    targetPosition: targetPos,
  })

  const dashed = (data as { dashed?: boolean } | undefined)?.dashed

  return (
    <BaseEdge
      id={id}
      path={path}
      markerEnd={markerEnd}
      style={{ ...style, strokeDasharray: dashed ? '5 4' : undefined }}
    />
  )
}
