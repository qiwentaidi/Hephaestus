import type { AIWorkbenchPenTestSessionBlackboard as Blackboard } from './api/ai-workbench'

export function projectExplorationHref(project: string, tab = 'overview', node?: string, source = 'all') {
  const params = new URLSearchParams({ project, tab })
  if (node) params.set('node', node)
  if (source !== 'all') params.set('source', source)
  return `/ai/workbench/project?${params}`
}

/** Keep only real cross-session context adjacent to the selected session. */
export function scopeProjectBoard(board: Blackboard, source: string): Blackboard {
  if (source === 'all') return board
  const facts = board.facts.filter(f => f.sourceConversationId === source)
  const own = new Set(facts.map(f => f.factKey))
  const findings = board.findings.filter(f => f.taskId === source || own.has(f.sourceFactKey || '') || facts.some(fact => fact.relatedFindingId === f.id))
  const referenced = new Set(findings.flatMap(f => f.sourceFactKey ? [f.sourceFactKey] : []))
  const edges = board.edges.filter(e => e.sourceConversationId === source || own.has(e.sourceFactKey) || own.has(e.targetFactKey) || referenced.has(e.sourceFactKey) || referenced.has(e.targetFactKey))
  const relations = board.exploration?.relations.filter(e => e.conversationId === source) || []
  for (const e of relations) for (const ref of [e.source, e.target]) if (ref.startsWith('fact:')) referenced.add(ref.slice(5))
  const findingRefs = new Set(relations.flatMap(e => [e.source, e.target]).filter(id => id.startsWith('finding:')).map(id => id.slice(8)))
  const contextFindings = [...(board.contextFindings || []), ...board.findings].filter(f => !!f.id && findingRefs.has(f.id))
  for (const f of contextFindings) if (f.sourceFactKey) referenced.add(f.sourceFactKey)
  for (const e of edges) { referenced.add(e.sourceFactKey); referenced.add(e.targetFactKey) }
  return { ...board, facts, findings, edges,
    contextFindings,
    exploration: board.exploration ? { ...board.exploration, digests: board.exploration.digests?.filter(d => d.conversationId === source), goals: board.exploration.goals?.filter(g => g.conversationId === source), intents: board.exploration.intents.filter(i => i.conversationId === source), relations, executions: board.exploration.executions?.filter(e => e.conversationId === source) } : undefined, contextFacts: [...(board.contextFacts || []), ...board.facts].filter(f => !own.has(f.factKey) && referenced.has(f.factKey)), sourceConversations: board.sourceConversations?.filter(s => s.id === source), todoPlans: board.todoPlans?.filter(p => p.conversationId === source) }
}
