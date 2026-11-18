import type { ModuleId } from "./CombinationTaskBuilder"

export interface CombinationFlowNode {
  id: string
  moduleId: ModuleId | string
}

export interface CombinationFlowStage {
  id: string
  nodes: CombinationFlowNode[]
}

export interface CombinationFlowEdge {
  from: string
  to: string
}

export interface CombinationFlowPoint {
  left: number
  right: number
  cy: number
}

export interface CombinationFlowPath {
  /** `curve:` or `arrow:` plus the edge key; stable React keys. */
  id: string
  edgeKey: string
  from: string
  to: string
  kind: "curve" | "arrow"
  d: string
}

/**
 * Which module a module's work is derived from.
 *
 * The scheduler only knows stages: every node of a stage runs in parallel and
 * the next stage waits for all of them (`combination_queue.go`). This table
 * therefore describes the work flow the canvas draws, not a per-module
 * dependency: the canvas still has to show that a stage is a barrier.
 */
export const COMBINATION_FLOW_PARENTS: Record<string, string[]> = {
  subdomain: [],
  portscan: ["subdomain"],
  webscan: ["portscan"],
  crack: ["portscan"],
  dirscan: ["webscan"],
  jsscan: ["webscan"],
}

const edgeKey = (from: string, to: string) => `${from}->${to}`

const parentsOf = (moduleId: string) => COMBINATION_FLOW_PARENTS[moduleId] || []

const precedingNodes = (stages: CombinationFlowStage[], stageIndex: number): CombinationFlowNode[] => {
  for (let index = stageIndex - 1; index >= 0; index -= 1) {
    if (stages[index].nodes.length > 0) return stages[index].nodes
  }
  return []
}

/**
 * Sources for one node:
 *  1. The stage directly before it, when that stage holds a declared parent
 *     (this is what draws 网站扫描 -> JS 漏洞检测 instead of 密码暴破 -> JS).
 *  2. Otherwise every node of the closest non-empty preceding stage, so a node
 *     is never left unconnected. Edges never span more than one stage, because
 *     a longer edge would have to cross the cards in between.
 */
const sourceNodesFor = (
  stages: CombinationFlowStage[],
  stageIndex: number,
  moduleId: string,
): CombinationFlowNode[] => {
  const parents = parentsOf(moduleId)
  if (parents.length > 0) {
    const adjacent = stages[stageIndex - 1].nodes.filter((node) => parents.includes(String(node.moduleId)))
    if (adjacent.length > 0) return adjacent
  }
  return precedingNodes(stages, stageIndex)
}

/**
 * Node-level edges for the canvas. Edges only ever connect adjacent stages so
 * the curves stay inside the gutter between two columns.
 */
export function buildCombinationFlowEdges(stages: CombinationFlowStage[]): CombinationFlowEdge[] {
  const edges: CombinationFlowEdge[] = []
  const seen = new Set<string>()
  const push = (from: string, to: string) => {
    if (!from || !to || from === to) return
    const key = edgeKey(from, to)
    if (seen.has(key)) return
    seen.add(key)
    edges.push({ from, to })
  }

  stages.forEach((stage, stageIndex) => {
    if (stageIndex === 0) return
    for (const node of stage.nodes) {
      for (const source of sourceNodesFor(stages, stageIndex, String(node.moduleId))) push(source.id, node.id)
    }
  })
  return edges
}

/** Exposed so tests can assert the same identity the canvas uses. */
export const combinationFlowEdgeKey = edgeKey

const cubicCurve = (from: { x: number; y: number }, to: { x: number; y: number }) => {
  // A horizontal control point on both ends keeps the curve flat where it
  // leaves and enters a card, which reads as a React Flow style link.
  const offset = Math.max(18, Math.abs(to.x - from.x) * 0.42)
  return `M ${from.x} ${from.y} C ${from.x + offset} ${from.y}, ${to.x - offset} ${to.y}, ${to.x} ${to.y}`
}

const arrowHead = (tip: { x: number; y: number }) => {
  const halfHeight = 3.4
  return `M ${tip.x - 5.4} ${tip.y - halfHeight} L ${tip.x} ${tip.y} L ${tip.x - 5.4} ${tip.y + halfHeight}`
}

/**
 * SVG geometry for the measured node positions. Kept free of DOM access so the
 * curve math stays testable.
 */
export function buildCombinationFlowPaths(
  points: Record<string, CombinationFlowPoint>,
  edges: CombinationFlowEdge[],
): CombinationFlowPath[] {
  const paths: CombinationFlowPath[] = []
  for (const edge of edges) {
    const from = points[edge.from]
    const to = points[edge.to]
    if (!from || !to) continue
    const key = edgeKey(edge.from, edge.to)
    const start = { x: from.right, y: from.cy }
    const end = { x: to.left, y: to.cy }
    paths.push({ id: `curve:${key}`, edgeKey: key, from: edge.from, to: edge.to, kind: "curve", d: cubicCurve(start, end) })
    paths.push({ id: `arrow:${key}`, edgeKey: key, from: edge.from, to: edge.to, kind: "arrow", d: arrowHead(end) })
  }
  return paths
}
