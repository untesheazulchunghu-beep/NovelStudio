export type WorkflowEdgeEndpoints = { source: string; target: string }

export type WorkflowEdgeDecision = { ok: true } | { ok: false; reason: 'missing' | 'self' | 'duplicate' }

export function acceptWorkflowEdge(
  edges: WorkflowEdgeEndpoints[],
  source: string,
  target: string,
): WorkflowEdgeDecision {
  if (!source || !target) return { ok: false, reason: 'missing' }
  if (source === target) return { ok: false, reason: 'self' }
  if (edges.some((edge) => edge.source === source && edge.target === target)) return { ok: false, reason: 'duplicate' }
  return { ok: true }
}

// Empty when either endpoint is missing so a stale edge does not draw a wire.
export function workflowEdgePath(
  source: { x: number; y: number } | undefined,
  target: { x: number; y: number } | undefined,
): string {
  if (!source || !target) return ''
  const x1 = source.x + 190
  const y1 = source.y + 62
  const x2 = target.x + 12
  const y2 = target.y + 62
  return `M ${x1} ${y1} C ${x1 + 56} ${y1}, ${x2 - 56} ${y2}, ${x2} ${y2}`
}
