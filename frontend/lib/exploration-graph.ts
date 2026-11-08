import type { AIWorkbenchFindingLineage, AIWorkbenchFindingLineageEdge, AIWorkbenchFindingLineageNode, AIWorkbenchPenTestSessionBlackboard as Blackboard, AIWorkbenchExplorationExecution } from './api/ai-workbench'

export type ExplorationKind = 'start' | 'goal' | 'intent' | 'fact' | 'finding' | 'blocker' | 'missing' | 'digest'
export type ExplorationNode = {
  id: string; kind: ExplorationKind; title: string; status?: string; priority?: number; body?: string
  factKey?: string; findingId?: string; conversationId?: string; executionId?: string; executionIds?: string[]
  executions?: AIWorkbenchExplorationExecution[]
  members?: string[]; origin?: string; reconstructionExecutionId?: string; external?: boolean; unlinked?: boolean; missingIntent?: boolean; sourceTitle?: string
  /** The owning conversation is still running, so this node can represent live work. */
  live?: boolean
  /** Session target, kept on the start card when no explicit goal was recorded. */
  target?: string
  /** The implicit goal was folded into this start card instead of a duplicate node. */
  mergedGoal?: boolean
  /** Last durable update of the recorded intent; orders the fallback "recent" direction. */
  updatedAt?: string
  /** Recorded severity of a finding, rendered as a risk badge on its card. */
  severity?: string
  severityLabel?: string
  /** Target recorded on a finding; shown as read-only context in the detail panel. */
  url?: string
}
export type ExplorationEdge = { id: string; source: string; target: string; relation: string; provenance?: boolean; tentative?: boolean }
export type ExplorationGraph = { nodes: ExplorationNode[]; edges: ExplorationEdge[] }
export const explorationHref = (session: string) => `/ai/workbench/exploration?session=${encodeURIComponent(session)}`

export function buildFindingLineageGraph(lineage: AIWorkbenchFindingLineage): ExplorationGraph {
  // Facts and tool executions are expandable evidence in the detail panel.
  // Keep the causal graph focused on intent and finding ownership, even if an
  // older API payload still includes those nodes.
  const nodes = lineage.nodes.filter(node => node.kind !== 'execution' && node.kind !== 'fact')
  const nodeIDs = new Set(nodes.map(node => node.id))
  const hiddenIDs = new Set(lineage.nodes.filter(node => !nodeIDs.has(node.id)).map(node => node.id))
  const edges = new Map<string, AIWorkbenchFindingLineage['edges'][number]>()
  const add = (edge: AIWorkbenchFindingLineage['edges'][number]) => edges.set(`${edge.source}|${edge.target}|${edge.relation}`, edge)
  for (const source of nodes) {
    const queue = lineage.edges.filter(edge => edge.source === source.id).map(edge => ({ id: edge.target, persisted: edge.persisted, relation: edge.relation }))
    const seen = new Set<string>()
    while (queue.length) {
      const current = queue.shift()!
      if (seen.has(`${current.id}|${current.relation}`)) continue
      seen.add(`${current.id}|${current.relation}`)
      if (nodeIDs.has(current.id)) {
        add({ id: `lineage:${source.id}:${current.id}:${current.relation}`, source: source.id, target: current.id, relation: current.relation, persisted: current.persisted })
        continue
      }
      if (!hiddenIDs.has(current.id)) continue
      for (const edge of lineage.edges.filter(candidate => candidate.source === current.id)) queue.push({ id: edge.target, persisted: current.persisted && edge.persisted, relation: edge.relation })
    }
  }
  return {
    nodes: nodes.map(node => ({
      id: node.id,
      kind: node.kind as ExplorationKind,
      title: node.title,
      status: node.status,
      priority: node.priority,
      body: node.body || node.summary,
      factKey: node.factKey,
      findingId: node.findingId,
      conversationId: node.conversationId,
      origin: node.origin,
    })),
    edges: [...edges.values()].map(edge => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      relation: edge.relation,
      provenance: !edge.persisted,
    })),
  }
}
const factID = (key: string) => `fact:${key}`

function lineageNodeToExploration(node: AIWorkbenchFindingLineageNode): ExplorationNode {
  return {
    id: node.id,
    kind: node.kind as ExplorationKind,
    title: node.title,
    status: node.status,
    priority: node.priority,
    body: node.body || node.summary,
    factKey: node.factKey,
    findingId: node.findingId,
    conversationId: node.conversationId,
    origin: node.origin,
  }
}

export type FindingChainView = {
  graph: ExplorationGraph
  /** Visible intents/goals on the chosen causal path, including the finding. */
  chainLength: number
  /** Visible lineage nodes outside the path; expand to inspect them. */
  relatedVisible: number
  /** Hidden evidence nodes (facts/executions) outside the path. */
  relatedEvidence: number
  /** True when no causal path exists and the caller should show the full lineage. */
  fallback: boolean
}

/**
 * Evidence chain view: pick one causal path from a root direction to the
 * finding, preferring the path that carries the most recorded evidence
 * (facts and executions) and persisted edges. Everything else stays folded so
 * the chain reads top-to-bottom instead of becoming a hairball.
 */
export function buildFindingChainView(lineage: AIWorkbenchFindingLineage, findingId: string): FindingChainView {
  const visible = lineage.nodes.filter(node => node.kind !== 'execution' && node.kind !== 'fact')
  const visibleIDs = new Set(visible.map(node => node.id))
  const byID = new Map(lineage.nodes.map(node => [node.id, node]))
  const target = lineage.nodes.find(node => node.findingId === findingId)?.id
    ?? lineage.nodes.find(node => node.id === `finding:${findingId}`)?.id
    ?? lineage.nodes.find(node => node.kind === 'finding')?.id
  const fallback = (): FindingChainView => ({ graph: buildFindingLineageGraph(lineage), chainLength: 0, relatedVisible: 0, relatedEvidence: 0, fallback: true })
  if (!target) return fallback()

  const incoming = new Map<string, AIWorkbenchFindingLineageEdge[]>()
  for (const edge of lineage.edges) {
    const list = incoming.get(edge.target)
    if (list) list.push(edge)
    else incoming.set(edge.target, [edge])
  }
  const nodeWeight = (id: string) => {
    switch (byID.get(id)?.kind) {
      case 'fact': return 3
      case 'execution': return 5
      case 'intent': return 1
      default: return 0
    }
  }
  let best: { score: number; nodes: string[]; edges: AIWorkbenchFindingLineageEdge[] } | null = null
  let expansions = 0
  const visit = (id: string, nodes: string[], edges: AIWorkbenchFindingLineageEdge[], seen: Set<string>) => {
    if (++expansions > 4000) return
    const parents = (incoming.get(id) || []).filter(edge => !seen.has(edge.source))
    if (!parents.length) {
      if (!edges.length) return
      const score = nodes.reduce((sum, nodeID) => sum + nodeWeight(nodeID), 0) + edges.filter(edge => edge.persisted).length
      if (!best || score > best.score || (score === best.score && nodes.length > best.nodes.length)) best = { score, nodes, edges }
      return
    }
    for (const edge of parents) visit(edge.source, [edge.source, ...nodes], [edge, ...edges], new Set([...seen, edge.source]))
  }
  visit(target, [target], [], new Set([target]))
  if (!best) return fallback()

  const chosen = best as { score: number; nodes: string[]; edges: AIWorkbenchFindingLineageEdge[] }
  const chain = chosen.nodes
    .map((id, index) => ({ id, index }))
    .filter(item => visibleIDs.has(item.id))
  const chainIDs = new Set(chain.map(item => item.id))
  const graphEdges = new Map<string, ExplorationEdge>()
  for (let index = 0; index + 1 < chain.length; index++) {
    const from = chain[index], to = chain[index + 1]
    const key = `${from.id}|${to.id}`
    if (graphEdges.has(key)) continue
    // The hop may pass through hidden fact/execution nodes; the first edge keeps
    // the recorded relation (yields, supports, spawns, ...) and provenance.
    const hop = chosen.edges[from.index]
    graphEdges.set(key, { id: `chain:${from.id}:${to.id}`, source: from.id, target: to.id, relation: hop?.relation || 'supports', provenance: hop ? !hop.persisted : false })
  }
  const graph: ExplorationGraph = {
    nodes: chain.map(item => lineageNodeToExploration(byID.get(item.id)!)),
    edges: [...graphEdges.values()],
  }
  if (chain.length < 2 || graphEdges.size === 0) return fallback()
  const relatedVisible = visible.filter(node => !chainIDs.has(node.id)).length
  const relatedEvidence = lineage.nodes.filter(node => (node.kind === 'fact' || node.kind === 'execution') && !chosen.nodes.includes(node.id)).length
  return { graph, chainLength: chain.length, relatedVisible, relatedEvidence, fallback: false }
}

const liveConversationStatuses = new Set(['queued', 'cancelling', 'recovering', 'pending', 'running', 'draining', 'waiting', 'waiting_tool', 'waiting_approval', 'active'])

export function buildExplorationGraph(board: Blackboard): ExplorationGraph {
  const nodes = new Map<string, ExplorationNode>()
  const edges = new Map<string, ExplorationEdge>()
  const session = board.conversation
  const root = 'session'
  const liveConversations = new Set<string>()
  if (session && liveConversationStatuses.has((session.status || '').trim().toLowerCase())) liveConversations.add(session.id)
  for (const source of board.sourceConversations || []) if (source.status && liveConversationStatuses.has(source.status.trim().toLowerCase())) liveConversations.add(source.id)
  const add = (node: ExplorationNode) => nodes.set(node.id, node)
  const link = (source: string, target: string, relation: string, provenance = false, tentative = false) => {
    if (source === target) return
    const id = JSON.stringify([source, target, relation])
    edges.set(id, { id, source, target, relation, provenance, tentative })
  }
  if (session) {
    // The conversation target is the implicit goal. Keep it on the start card
    // instead of emitting a second node that duplicates the same target.
    add({ id: root, kind: 'start', title: session.title || session.target, body: session.target, status: session.status, conversationId: session.id, target: session.target, mergedGoal: !!session.target && !board.exploration?.goals?.length })
  }
  const projectScope = !session && board.scope === 'project'
  const sources = board.sourceConversations || []
  // In an orchestration project, a Worker conversation executes an exploration
  // intent owned by the Planner, while the controller is only the human-facing
  // shell. Only the Planner and ordinary sessions own a root; drawing every
  // Worker as a start produced one disconnected root per Worker because
  // projected intents are stored under the Planner conversation.
  const preferredRoots = sources.filter(source => source.agentRole !== 'worker' && source.agentRole !== 'controller')
  const nonWorkerRoots = sources.filter(source => source.agentRole !== 'worker')
  const rootSources = projectScope ? (preferredRoots.length ? preferredRoots : nonWorkerRoots.length ? nonWorkerRoots : sources) : []
  const sourceByID = new Map(sources.map(source => [source.id, source]))
  const rootsByConversation = new Map<string, string>()
  if (projectScope) {
    for (const source of rootSources) {
      const sourceID = `session:${source.id}`
      const hasGoal = board.exploration?.goals?.some(g => g.conversationId === source.id)
      add({ id: sourceID, kind: 'start', title: source.title || source.target, body: source.target, conversationId: source.id, status: source.status, target: source.target, mergedGoal: !!source.target && !hasGoal })
      rootsByConversation.set(source.id, sourceID)
    }
  }
  /** Resolve a node's owning conversation to the nearest graph root. Worker
   * conversations deliberately have no root of their own; their evidence is
   * scoped to the Planner conversation that owns the intent. */
  const sourceRoot = (conversationID: string): string | undefined => {
    if (session) return root
    let current = conversationID
    const seen = new Set<string>()
    while (current && !seen.has(current)) {
      seen.add(current)
      const direct = rootsByConversation.get(current)
      if (direct) return direct
      current = sourceByID.get(current)?.parentConversationId || ''
    }
    return rootsByConversation.values().next().value
  }
  for (const intent of board.exploration?.intents || []) {
    const id = `intent:${intent.id}`
    const executions = board.exploration?.executions?.filter(e => e.intentId === intent.id)
    add({ id, kind: 'intent', title: intent.title, status: intent.status, priority: intent.priority, body: intent.reason, conversationId: intent.conversationId, origin: intent.origin, reconstructionExecutionId: intent.reconstructionExecutionId, executionIds: executions?.map(e => e.id), executions, live: liveConversations.has(intent.conversationId), updatedAt: intent.updatedAt })
    // Only initial directions need a source-membership edge. Evidence-derived
    // directions connect to their actual parents, avoiding a repeated goal fan-out.
    if (!(board.exploration?.relations || []).some(e => e.target === id && e.relation === 'derived_from')) {
      const from = sourceRoot(intent.conversationId)
      if (from) link(from, id, 'spawns')
    }
  }
  for (const goal of board.exploration?.goals || []) {
    const id = `goal:${goal.id}`
    add({ id, kind: 'goal', title: goal.title, status: goal.status, body: goal.reason, conversationId: goal.conversationId })
    if (!(board.exploration?.relations || []).some(e => e.target === id && e.relation === 'proves')) {
      const from = sourceRoot(goal.conversationId)
      if (from) link(from, id, 'spawns', true)
    }
  }
  const own = new Set(board.facts.map(f => f.factKey))
  const allFacts = new Map([...(board.contextFacts || []), ...board.facts].map(f => [f.factKey, f]))
  // Raw extraction is evidence, not an exploration conclusion. Explicit relationships
  // and finding references can promote any observation into the graph.
  const automatic = (key: string) => /^auto\.(?:clue|tool)\./.test(key)
  const needed = new Set(board.facts.filter(f => !automatic(f.factKey) || f.relatedFindingId).map(f => f.factKey))
  for (const edge of board.edges) { needed.add(edge.sourceFactKey); needed.add(edge.targetFactKey) }
  for (const finding of board.findings) { if (finding.sourceFactKey) needed.add(finding.sourceFactKey) }
  for (const edge of board.exploration?.relations || []) {
    for (const ref of [edge.source, edge.target]) if (ref.startsWith('fact:')) needed.add(ref.slice(5))
  }
  const addFact = (key: string) => {
    const f = allFacts.get(key)
    if (!f) { add({ id: factID(key), kind: 'missing', title: key, factKey: key }); return }
    // Only explicit category/key semantics classify blockers, never raw evidence text.
    const blocked = f.category === 'blocker' || /^(?:blocker|blocked)[/.]/.test(f.factKey)
    add({ id: factID(key), kind: blocked ? 'blocker' : 'fact', title: f.summary, body: f.body, status: f.confidence, factKey: key, conversationId: f.sourceConversationId, external: !own.has(key), executionId: automatic(key) && f.sourceConversationId ? f.sourceMessageId : undefined })
  }
  needed.forEach(addFact)
  for (const edge of board.edges) {
    link(factID(edge.sourceFactKey), factID(edge.targetFactKey), edge.edgeType, false, edge.confidence !== 'confirmed')
  }
  const referencedFindingIDs = new Set((board.exploration?.relations || []).flatMap(e => [e.source, e.target]).filter(id => id.startsWith('finding:')).map(id => id.slice(8)))
  const allFindings = new Map([...(board.contextFindings || []).filter(f => !!f.id && referencedFindingIDs.has(f.id)), ...board.findings].map(f => [f.id, f]))
  for (const f of allFindings.values()) {
    if (!f.id) continue
    const id = `finding:${f.id}`
    add({ id, kind: 'finding', title: f.name, status: f.status, findingId: f.id, body: f.impact, conversationId: f.taskId, severity: f.severity, severityLabel: f.severityLabel, url: f.rawTarget || f.url })
    if (f.sourceFactKey) { if (!nodes.has(factID(f.sourceFactKey))) addFact(f.sourceFactKey); link(factID(f.sourceFactKey), id, 'supports') }
    for (const fact of board.facts) { if (fact.relatedFindingId === f.id) link(factID(fact.factKey), id, 'supports') }
  }
  for (const edge of board.exploration?.relations || []) {
    for (const ref of [edge.source, edge.target]) if (!nodes.has(ref)) add({ id: ref, kind: 'missing', title: ref })
    link(edge.source, edge.target, edge.relation)
  }
  // Provenance edges carry scope, never imply that a goal/intent caused a finding.
  for (const node of nodes.values()) {
    if (node.factKey && own.has(node.factKey) && (node.conversationId === session?.id || board.scope === 'project') && ![...edges.values()].some(e => e.target === node.id)) {
      const from = sourceRoot(node.conversationId || '')
      if (from) {
        node.unlinked = true
        link(from, node.id, 'recorded', true)
      }
    }
    if (node.kind === 'finding' && ![...edges.values()].some(e => e.target === node.id)) node.unlinked = true
  }
  for (const node of nodes.values()) {
    if ((node.kind === 'fact' || node.kind === 'finding') && board.exploration?.intents.some(i => i.conversationId === node.conversationId) && ![...edges.values()].some(e => e.target === node.id && e.relation === 'yields')) node.missingIntent = true
    if (board.scope === 'project' && node.conversationId) node.sourceTitle = board.sourceConversations?.find(s => s.id === node.conversationId)?.title
  }
  for (const digest of board.exploration?.digests || []) add({id: `digest:${digest.id}`, kind: 'digest', title: digest.title, body: digest.body, members: digest.members, conversationId: digest.conversationId})
  return { nodes: [...nodes.values()], edges: [...edges.values()].filter(e => nodes.has(e.source) && nodes.has(e.target)) }
}

/** Rank strongly connected components so cycles cannot push nodes off canvas. */
export function layoutExplorationGraph(graph: ExplorationGraph): Map<string, { x: number; y: number }> {
  const outgoing = new Map(graph.nodes.map(n => [n.id, [] as string[]]))
  for (const e of graph.edges) outgoing.get(e.source)?.push(e.target)
  let cursor = 0
  const indices = new Map<string, number>(), low = new Map<string, number>(), stack: string[] = [], active = new Set<string>(), components: string[][] = []
  const visit = (id: string) => {
    indices.set(id, cursor); low.set(id, cursor++); stack.push(id); active.add(id)
    for (const next of outgoing.get(id) || []) {
      if (!indices.has(next)) { visit(next); low.set(id, Math.min(low.get(id)!, low.get(next)!)) }
      else if (active.has(next)) low.set(id, Math.min(low.get(id)!, indices.get(next)!))
    }
    if (low.get(id) === indices.get(id)) {
      const group: string[] = []; let item: string
      do { item = stack.pop()!; active.delete(item); group.push(item) } while (item !== id)
      components.push(group)
    }
  }
  graph.nodes.forEach(n => { if (!indices.has(n.id)) visit(n.id) })
  const component = new Map<string, number>(); components.forEach((group, i) => group.forEach(id => component.set(id, i)))
  const ranks = components.map(() => 0), indegree = components.map(() => 0), arcs = components.map(() => new Set<number>())
  for (const edge of graph.edges) {
    const a = component.get(edge.source)!, b = component.get(edge.target)!
    if (a !== b && !arcs[a].has(b)) { arcs[a].add(b); indegree[b]++ }
  }
  const queue = indegree.flatMap((d, i) => d === 0 ? [i] : [])
  for (let i = 0; i < queue.length; i++) for (const next of arcs[queue[i]]) { ranks[next] = Math.max(ranks[next], ranks[queue[i]] + 1); if (--indegree[next] === 0) queue.push(next) }
  const columns = new Map<number, ExplorationNode[]>()
  for (const node of graph.nodes) { const rank = ranks[component.get(node.id)!]; columns.set(rank, [...(columns.get(rank) || []), node]) }
  // Keep parallel directions in one stage. Neighbor ordering reduces crossings
  // without suggesting a later stage just because a column contains many nodes.
  const layers = [...columns.entries()].sort(([a], [b]) => a - b).map(([, nodes]) => nodes)
  const neighbors = (id: string, forward: boolean) => graph.edges.flatMap(e => forward ? (e.source === id ? [e.target] : []) : (e.target === id ? [e.source] : []))
  const order = new Map<string, number>()
  const indexLayers = () => layers.forEach(layer => layer.forEach((n, i) => order.set(n.id, i - (layer.length - 1) / 2)))
  indexLayers()
  for (let sweep = 0; sweep < 4; sweep++) for (const forward of [false, true]) {
    const sequence = forward ? [...layers].reverse() : layers
    for (const layer of sequence) {
      const score = (n: ExplorationNode) => {
        const linked = neighbors(n.id, forward).filter(id => component.get(id) !== component.get(n.id))
        return linked.length ? linked.reduce((sum, id) => sum + (order.get(id) || 0), 0) / linked.length : order.get(n.id) || 0
      }
      const scores = new Map(layer.map(n => [n.id, score(n)]))
      layer.sort((a, b) => scores.get(a.id)! - scores.get(b.id)!)
      indexLayers()
    }
  }
  const multiConversation = new Set(graph.nodes.map(n => n.conversationId).filter(Boolean)).size > 1
  const mergedTarget = graph.nodes.some(n => n.mergedGoal && n.target && n.target !== n.title)
  // The merged start card spends one extra line on the target; project cards also
  // show the source title, so keep enough vertical room to avoid overlaps.
  const rowHeight = (multiConversation ? 174 : mergedTarget ? 174 : 154) + (multiConversation && mergedTarget ? 20 : 0)
  const maxRows = Math.max(1, ...layers.map(layer => layer.length))
  const positions = new Map<string, { x: number; y: number }>()
  layers.forEach((layer, rank) => layer.forEach((node, i) => positions.set(node.id, { x: rank * 304, y: (maxRows - layer.length) * rowHeight / 2 + i * rowHeight })))
  return positions
}

/** ARTEX task graph vocabulary. Supporting fact networks stay in evidence
 * details; never replace a direct intent→finding output with a fact triangle. */
export function mainExplorationGraph(original: ExplorationGraph, expanded = new Set<string>()): ExplorationGraph {
  const graph = collapseExplorationDigests(original, expanded)
  const relations = new Set(['spawns', 'derived_from', 'yields', 'proves'])
  // Keep provenance for facts that have no recorded exploration parent. The
  // dashed source edge makes their scope visible without implying causality.
  const provenance = graph.edges.filter(e => {
    if (e.relation !== 'recorded') return false
    const target = graph.nodes.find(n => n.id === e.target)
    return !!target?.unlinked && ['fact', 'blocker', 'missing'].includes(target.kind)
  })
  const edges = [...graph.edges.filter(e => relations.has(e.relation)), ...provenance]
  const referenced = new Set(edges.flatMap(e => [e.source, e.target]))
  return {
    nodes: graph.nodes.filter(n => referenced.has(n.id) || ['start', 'goal', 'intent', 'finding', 'digest'].includes(n.kind) || ((n.kind === 'fact' || n.kind === 'blocker') && n.status !== 'deprecated')),
    edges,
  }
}

function upstreamExplorationGraph(graph: ExplorationGraph, seed: Set<string>): ExplorationGraph {
  if (!seed.size) return { nodes: [], edges: [] }
  const keep = new Set(seed)
  for (let changed = true; changed;) {
    changed = false
    for (const e of graph.edges) if (keep.has(e.target) && !keep.has(e.source)) { keep.add(e.source); changed = true }
  }
  return { nodes: graph.nodes.filter(n => keep.has(n.id)), edges: graph.edges.filter(e => keep.has(e.source) && keep.has(e.target)) }
}

/** Follow saved parents only. No text, time or completion-based inference. */
export function ancestorExplorationGraph(graph: ExplorationGraph, nodeId: string): ExplorationGraph {
  return upstreamExplorationGraph(graph, new Set([nodeId]))
}

const unsettledIntent = (node: ExplorationNode) => node.kind === 'intent' && (node.status === 'in_progress' || node.status === 'running')

/** The recorded chain into the live direction. Only in-progress intents from a
 * still-running conversation qualify; settled work and pending siblings stay static. */
export function activeExplorationGraph(graph: ExplorationGraph): ExplorationGraph {
  return upstreamExplorationGraph(graph, new Set(graph.nodes.filter(node => unsettledIntent(node) && node.live).map(node => node.id)))
}

export type ExplorationDirection = { intent: ExplorationNode; live: boolean; edges: ExplorationEdge[] }

/** The direction worth calling out. A running conversation wins; otherwise keep
 * the latest unsettled intent, and finally the latest settled one, as a static
 * "recent" direction, so stopping a run does not erase where it last worked. */
export function explorationDirection(graph: ExplorationGraph): ExplorationDirection | null {
  const intents = graph.nodes.filter(node => node.kind === 'intent')
  if (!intents.length) return null
  const unsettled = intents.filter(unsettledIntent)
  const live = unsettled.filter(node => node.live)
  const pool = live.length ? live : unsettled.length ? unsettled : intents
  const intent = pool.reduce((latest, node) => (node.updatedAt || '') > (latest.updatedAt || '') ? node : latest)
  const chain = live.length ? activeExplorationGraph(graph) : ancestorExplorationGraph(graph, intent.id)
  return { intent, live: live.length > 0, edges: chain.edges }
}

/** ARTEX collapseDigestGraph: retain source nodes for detail/expansion, remap
 * boundary edges, remove internal loops and deduplicate remapped relations. */
export function collapseExplorationDigests(graph: ExplorationGraph, expanded = new Set<string>()): ExplorationGraph {
 const owner = new Map<string, string>()
 for (const node of graph.nodes) if (node.kind === 'digest' && !expanded.has(node.id)) for (const id of node.members || []) owner.set(id, node.id)
 if (!owner.size) return {nodes: graph.nodes.filter(n => n.kind !== 'digest' || !expanded.has(n.id)), edges: graph.edges}
 const edges = new Map<string, ExplorationEdge>()
 for (const edge of graph.edges) {
  const source = owner.get(edge.source) || edge.source, target = owner.get(edge.target) || edge.target
  if (source === target) continue
  const id = JSON.stringify([source, target, edge.relation])
  edges.set(id, {...edge, id: source === edge.source && target === edge.target ? edge.id : id, source, target})
 }
 return {nodes: graph.nodes.filter(n => n.kind === 'digest' ? !expanded.has(n.id) : !owner.has(n.id)), edges: [...edges.values()]}
}
