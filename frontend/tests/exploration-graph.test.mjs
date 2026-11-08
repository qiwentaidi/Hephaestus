import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const source = readFileSync(new URL('../lib/exploration-graph.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
const { buildExplorationGraph, layoutExplorationGraph, mainExplorationGraph, ancestorExplorationGraph, activeExplorationGraph, explorationDirection } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const fact = (key, extra = {}) => ({ id: key, factKey: key, summary: key, body: '', confidence: 'confirmed', sourceConversationId: 'session', category: 'note', ...extra })
const board = (extra = {}) => ({ conversation: { id: 'session', title: 'fixture', target: 'https://example.invalid' }, facts: [], edges: [], findings: [], ...extra })

test('preserves recorded direction, missing context and deduplicates shared finding', () => {
  const graph = buildExplorationGraph(board({ facts: [fact('a'), fact('b', { relatedFindingId: 'f' })], edges: [{ sourceFactKey: 'missing', targetFactKey: 'a', edgeType: 'supports', confidence: 'confirmed' }], findings: [{ id: 'f', name: 'finding', sourceFactKey: 'b' }] }))
  assert.equal(graph.nodes.filter(n => n.id === 'finding:f').length, 1)
  assert.equal(graph.nodes.find(n => n.id === 'fact:missing').kind, 'missing')
  assert.ok(graph.edges.some(e => e.source === 'fact:missing' && e.target === 'fact:a'))
  assert.ok(!graph.edges.some(e => e.source === 'fact:a' && e.target === 'fact:b'))
})
test('raw observations stay out of the graph while referenced evidence and negative conclusions remain', () => {
  const data = board({
    facts: [
      ...Array.from({ length: 1000 }, (_, i) => fact(`auto.clue.endpoint.${i}`, { sourceMessageId: `exec${i}` })),
      fact('auto.tool.unreferenced', { sourceMessageId: 'raw-execution' }),
      fact('auth/negative', { summary: 'No authorization bypass was reproduced' }),
      fact('blocker/admin', { summary: 'Administrator credentials unavailable' }),
    ],
    edges: [{ sourceFactKey: 'auto.clue.endpoint.1', targetFactKey: 'auth/negative', edgeType: 'supports', confidence: 'confirmed' }],
    findings: [{ id: 'f', name: 'finding', sourceFactKey: 'auto.clue.endpoint.0' }],
  })
  const before = JSON.stringify(data)
  const graph = buildExplorationGraph(data)
  assert.equal(graph.nodes.filter(n => n.factKey).length, 4)
  assert.equal(graph.nodes.find(n => n.factKey === 'auto.clue.endpoint.0').executionId, 'exec0')
  assert.equal(graph.nodes.find(n => n.factKey === 'auto.clue.endpoint.1').executionId, 'exec1')
  assert.ok(graph.nodes.some(n => n.factKey === 'auth/negative'))
  assert.equal(graph.nodes.find(n => n.factKey === 'blocker/admin').kind, 'blocker')
  assert.ok(!graph.nodes.some(n => n.kind === 'execution' || n.kind === 'group'))
  assert.ok(!graph.nodes.some(n => n.factKey === 'auto.tool.unreferenced'))
  assert.equal(JSON.stringify(data), before, 'graph selection must not delete stored evidence')
  for (const edge of graph.edges) assert.ok(graph.nodes.some(n => n.id === edge.source) && graph.nodes.some(n => n.id === edge.target))
})
test('plan progress never creates a causal edge to facts or findings', () => {
  const graph = buildExplorationGraph(board({ todoPlan: { goal: 'goal', items: [{ id: 'p', title: 'Check', status: 'completed' }] }, facts: [fact('fact')], findings: [{ id: 'f', name: 'finding' }] }))
  assert.ok(!graph.nodes.some(n => n.id === 'intent:p'), 'Todo is not an intent even without exploration data')
  assert.ok(!graph.edges.some(e => e.source === 'intent:p'))
  assert.ok(graph.nodes.find(n => n.kind === 'finding').unlinked)
})
test('cyclic recorded relations produce finite, unique positions', () => {
  const graph = buildExplorationGraph(board({ facts: [fact('a'), fact('b'), fact('c')], edges: [{ sourceFactKey: 'a', targetFactKey: 'b', edgeType: 'supports' }, { sourceFactKey: 'b', targetFactKey: 'a', edgeType: 'leads_to' }, { sourceFactKey: 'b', targetFactKey: 'c', edgeType: 'supports' }] }))
  const positions = layoutExplorationGraph(graph)
  assert.equal(positions.size, graph.nodes.length)
  assert.equal(new Set([...positions.values()].map(p => `${p.x},${p.y}`)).size, graph.nodes.length)
  assert.ok([...positions.values()].every(p => Number.isFinite(p.x) && Number.isFinite(p.y)))
  assert.ok(positions.get('fact:c').x > positions.get('fact:b').x)
})

test('project graph ignores Todo plans and preserves recorded cross-session evidence', () => {
  const graph = buildExplorationGraph(board({ scope: 'project', conversation: null, sourceConversations: [{ id: 's1', title: 'First', target: 'target' }, { id: 's2', title: 'Second', target: 'target' }], todoPlans: ['s1', 's2'].map(conversationId => ({ conversationId, goal: 'goal', items: [{ id: 'same', title: 'Check', status: 'completed' }] })), facts: [fact('a', { sourceConversationId: 's1' }), fact('b', { sourceConversationId: 's2' })], edges: [{ sourceFactKey: 'a', targetFactKey: 'b', edgeType: 'supports', confidence: 'confirmed' }] }))
  assert.equal(graph.nodes.filter(n => n.kind === 'intent').length, 0)
  assert.equal(new Set(graph.nodes.map(n => n.id)).size, graph.nodes.length)
  assert.ok(graph.edges.some(e => e.source === 'fact:a' && e.target === 'fact:b' && !e.provenance))
  assert.ok(!graph.edges.some(e => e.source.startsWith('intent:') && e.target.startsWith('fact:')))
  assert.equal(graph.nodes.find(n => n.id === 'fact:b').sourceTitle, 'Second')
  // The target is folded into the start card; there is no duplicate implicit goal.
  assert.equal(graph.nodes.filter(n => n.kind === 'goal').length, 0)
  assert.equal(graph.nodes.find(n => n.id === 'session:s1').target, 'target')
  assert.equal(graph.nodes.find(n => n.id === 'session:s1').mergedGoal, true)
})

test('a session target is merged into the start card instead of a duplicate goal column', () => {
  const graph = buildExplorationGraph(board({
    exploration: { goals: [], intents: [{ id: 'i', conversationId: 'session', title: 'Question', status: 'pending' }], relations: [] },
  }))
  assert.equal(graph.nodes.filter(n => n.kind === 'goal').length, 0)
  const start = graph.nodes.find(n => n.kind === 'start')
  assert.equal(start.mergedGoal, true)
  assert.equal(start.target, 'https://example.invalid')
  const positions = layoutExplorationGraph(mainExplorationGraph(graph))
  assert.ok(positions.get('intent:i').x > positions.get('session').x)
})

test('durable intents retain exact outputs and multiple parents when removed from the current Todo plan', () => {
  const data = board({
    todoPlan: { goal: 'goal', items: [{ id: 'new', title: 'Deepen', status: 'in_progress' }] },
    exploration: {
      intents: [{ id: 'i1', todoId: 'removed', title: 'Read files', status: 'completed', conversationId: 'session' }, { id: 'i2', todoId: 'new', title: 'Deepen', status: 'in_progress', conversationId: 'session' }],
      relations: [
        { source: 'intent:i1', target: 'fact:read', relation: 'yields' },
        { source: 'intent:i1', target: 'finding:f', relation: 'yields' },
        { source: 'fact:read', target: 'intent:i2', relation: 'derived_from' },
        { source: 'finding:f', target: 'intent:i2', relation: 'derived_from' },
        { source: 'intent:i2', target: 'fact:config', relation: 'yields' },
      ],
      executions: [{ id: 'exec', intentId: 'i1', conversationId: 'session' }],
    },
    facts: [fact('read'), fact('config')], findings: [{ id: 'f', name: 'Read', sourceFactKey: 'read' }],
  })
  const graph = buildExplorationGraph(data)
  assert.deepEqual(graph.nodes.filter(n => n.kind === 'intent').map(n => n.id), ['intent:i1', 'intent:i2'])
  assert.deepEqual(graph.nodes.find(n => n.id === 'intent:i1').executionIds, ['exec'])
  assert.equal(graph.edges.filter(e => e.relation === 'yields').length, 3)
  assert.equal(graph.edges.filter(e => e.target === 'intent:i2' && !e.provenance).length, 2)
  assert.equal(graph.nodes.find(n => n.id === 'fact:config').unlinked, undefined)
  assert.equal(graph.edges.find(e => e.source === 'intent:i1' && e.target === 'finding:f').provenance, false)
  const positions = layoutExplorationGraph(graph)
  assert.ok(positions.get('fact:config').x > positions.get('intent:i1').x)
})

test('a referenced finding from another session stays available as exploration evidence', () => {
  const graph = buildExplorationGraph(board({
    exploration: { intents: [{ id: 'i', conversationId: 'session', title: 'Follow up', status: 'pending' }], relations: [{ source: 'finding:foreign', target: 'intent:i', relation: 'derived_from' }] },
    contextFindings: [{ id: 'foreign', name: 'Earlier finding', sourceFactKey: 'support', taskId: 'other' }], contextFacts: [fact('support', { sourceConversationId: 'other' })],
  }))
  assert.equal(graph.nodes.find(n => n.id === 'finding:foreign').kind, 'finding')
  assert.equal(graph.nodes.find(n => n.id === 'fact:support').kind, 'fact')
  assert.ok(graph.edges.some(e => e.source === 'fact:support' && e.target === 'finding:foreign'))
})

test('explicit goal proofs terminate the chain without repeated goal-to-intent fan-out', () => {
  const graph = buildExplorationGraph(board({
    exploration: { goals: [{ id: 'g', conversationId: 'session', title: 'Verify read', status: 'met', reason: 'Read independently verified', evidence: ['finding:f'] }], intents: [{ id: 'i', conversationId: 'session', title: 'Read', status: 'completed', priority: 8 }], relations: [{ source: 'intent:i', target: 'fact:read', relation: 'yields' }, { source: 'intent:i', target: 'finding:f', relation: 'yields' }, { source: 'finding:f', target: 'goal:g', relation: 'proves' }] },
    facts: [fact('read')], findings: [{ id: 'f', taskId: 'session', name: 'Read', status: 'confirmed', sourceFactKey: 'read' }],
  }))
  assert.equal(graph.nodes.filter(n => n.kind === 'goal').length, 1)
  assert.equal(graph.nodes.find(n => n.id === 'goal:g').status, 'met')
  assert.equal(graph.nodes.find(n => n.kind === 'start').mergedGoal, false)
  assert.equal(graph.nodes.find(n => n.id === 'intent:i').priority, 8)
  assert.ok(!graph.edges.some(e => e.source.startsWith('goal:') && e.target.startsWith('intent:')))
  const positions = layoutExplorationGraph(graph)
  assert.ok(positions.get('goal:g').x > positions.get('finding:f').x)
})
test('parallel directions remain in one column even with ten branches', () => {
  const graph = { nodes: [{ id: 'root', kind: 'start' }, ...Array.from({ length: 10 }, (_, i) => ({ id: `i${i}`, kind: 'intent' }))], edges: Array.from({ length: 10 }, (_, i) => ({ source: 'root', target: `i${i}`, relation: 'plan', provenance: true })) }
  const positions = layoutExplorationGraph(graph)
  assert.equal(new Set([...positions].filter(([id]) => id !== 'root').map(([, p]) => p.x)).size, 1)
  assert.equal(new Set([...positions.values()].map(p => `${p.x},${p.y}`)).size, 11)
})


test('main view retains direct outputs and proof and moves supporting networks to evidence', () => {
  const graph = buildExplorationGraph(board({ exploration: { goals: [{ id: 'g', conversationId: 'session', title: 'Goal', status: 'met' }], intents: [{ id: 'i', conversationId: 'session', title: 'Read', status: 'completed' }], relations: [{ source: 'intent:i', target: 'fact:read', relation: 'yields' }, { source: 'intent:i', target: 'finding:f', relation: 'yields' }, { source: 'fact:read', target: 'goal:g', relation: 'proves' }, { source: 'finding:f', target: 'goal:g', relation: 'proves' }] }, facts: [fact('read')], findings: [{ id: 'f', name: 'Read', sourceFactKey: 'read' }] }))
  const before = JSON.stringify(graph)
  const main = mainExplorationGraph(graph)
  assert.ok(main.edges.some(e => e.source === 'intent:i' && e.target === 'finding:f'))
  assert.ok(main.edges.some(e => e.source === 'fact:read' && e.target === 'goal:g'))
  assert.ok(!main.edges.some(e => e.relation === 'supports'))
  const ancestors = ancestorExplorationGraph(main, 'finding:f')
  assert.deepEqual(ancestors.nodes.map(n => n.id).sort(), ['session', 'intent:i', 'finding:f'].sort())
  assert.equal(JSON.stringify(graph), before)
})

test('main view exposes provenance for unlinked facts and places them after the source', () => {
  const graph = buildExplorationGraph(board({ facts: [fact('unlinked')] }))
  const main = mainExplorationGraph(graph)
  const edge = main.edges.find(e => e.source === 'session' && e.target === 'fact:unlinked')
  assert.equal(edge?.relation, 'recorded')
  assert.equal(edge?.provenance, true)
  const positions = layoutExplorationGraph(main)
  assert.ok(positions.get('fact:unlinked').x > positions.get('session').x)
})

test('main view preserves an entry into cyclic facts', () => {
  const graph = { nodes: ['i', 'fact:a', 'fact:b'].map(id => ({ id, kind: 'fact' })), edges: [
    { id: 'ia', source: 'i', target: 'fact:a', relation: 'yields' }, { id: 'ib', source: 'i', target: 'fact:b', relation: 'yields' }, { id: 'ab', source: 'fact:a', target: 'fact:b', relation: 'supports' }, { id: 'ba', source: 'fact:b', target: 'fact:a', relation: 'supports' },
  ] }
  const main = mainExplorationGraph(graph)
  assert.equal(main.edges.filter(e => e.relation === 'yields').length, 2)
  assert.ok(!main.edges.some(e => e.relation === 'supports'))
})

test('main view never substitutes a different intent for the recorded producer', () => {
 const graph = { nodes: ['a', 'b', 'fact:source', 'finding:f'].map(id => ({ id, kind: 'fact' })), edges: [
  { id: 'af', source: 'a', target: 'finding:f', relation: 'yields' }, { id: 'ae', source: 'a', target: 'fact:source', relation: 'yields' }, { id: 'eb', source: 'fact:source', target: 'b', relation: 'derived_from' }, { id: 'bf', source: 'b', target: 'finding:f', relation: 'yields' },
 ] }
 assert.ok(mainExplorationGraph(graph).edges.some(e => e.id === 'af'))
})

test('main view omits withdrawn notes but retains a deprecated source still cited by an intent', () => {
 const graph = { nodes: [{ id: 'fact:old', kind: 'fact', status: 'deprecated' }, { id: 'fact:live', kind: 'fact' }, { id: 'intent:i', kind: 'intent' }], edges: [{ id: 'old-live', source: 'fact:old', target: 'fact:live', relation: 'supports' }] }
 assert.ok(!mainExplorationGraph(graph).nodes.some(n => n.id === 'fact:old'))
 graph.edges.push({ id: 'old-intent', source: 'fact:old', target: 'intent:i', relation: 'derived_from' })
 assert.ok(mainExplorationGraph(graph).nodes.some(n => n.id === 'fact:old'))
})

test('digest preserves findings and boundary relations and expands original branch', () => {
 const graph = buildExplorationGraph(board({
  exploration: {intents: [{id: 'i1', conversationId: 'session', title: 'Read', status: 'completed'}, {id: 'i2', conversationId: 'session', title: 'Verify', status: 'completed'}], relations: [{source: 'intent:i1', target: 'fact:a', relation: 'yields'}, {source: 'fact:a', target: 'intent:i2', relation: 'derived_from'}, {source: 'intent:i2', target: 'finding:f', relation: 'yields'}], digests: [{id: 'd', conversationId: 'session', title: 'Settled branch', body: 'Read independently verified; limits preserved.', members: ['intent:i1', 'fact:a', 'intent:i2']}]},
  facts: [fact('a')], findings: [{id: 'f', name: 'Finding', status: 'fixed'}]
 }))
 const before = JSON.stringify(graph)
 const compact = mainExplorationGraph(graph)
 assert.ok(compact.nodes.some(n => n.id === 'finding:f'), 'findings never fold')
 assert.ok(compact.nodes.some(n => n.id === 'digest:d'))
 assert.ok(!compact.nodes.some(n => n.id === 'fact:a'))
 assert.ok(compact.edges.some(e => e.source === 'digest:d' && e.target === 'finding:f' && e.relation === 'yields'))
 assert.ok(!compact.edges.some(e => e.source === e.target))
 const expanded = mainExplorationGraph(graph, new Set(['digest:d']))
 assert.ok(expanded.nodes.some(n => n.id === 'fact:a'))
 assert.ok(expanded.edges.some(e => e.source === 'intent:i2' && e.target === 'finding:f'))
 assert.ok(!expanded.nodes.some(n => n.id === 'digest:d'))
 assert.equal(JSON.stringify(graph), before, 'view transformations preserve original records')
})

test('intent execution indexes retain all ordered metadata without mixing intent records', () => {
  const executions = Array.from({ length: 25 }, (_, i) => ({
    id: `exec-${i}`, intentId: 'i1', conversationId: 'session', toolName: `tool-${i}`,
    status: i === 24 ? 'running' : 'completed', startedAt: '2026-10-13T01:02:03Z',
    ...(i < 24 ? { completedAt: '2026-10-13T01:02:04Z' } : {}),
  }))
  const graph = buildExplorationGraph(board({ exploration: {
    intents: [{ id: 'i1', title: 'Inspect', conversationId: 'session' }, { id: 'i2', title: 'Other', conversationId: 'session' }],
    relations: [], executions: [...executions, { id: 'other', intentId: 'i2', conversationId: 'session' }],
  } }))
  assert.deepEqual(graph.nodes.find(n => n.id === 'intent:i1').executions, executions)
  assert.deepEqual(graph.nodes.find(n => n.id === 'intent:i2').executionIds, ['other'])
})

test('only the recorded chain into an in-progress direction flows', () => {
 const graph = {
  nodes: [
   { id: 'session', kind: 'start', title: 'session' },
   { id: 'intent:done', kind: 'intent', title: 'settled', status: 'completed' },
   { id: 'intent:active', kind: 'intent', title: 'current', status: 'in_progress', live: true },
   { id: 'intent:pending', kind: 'intent', title: 'queued', status: 'pending' },
   { id: 'fact:parent', kind: 'fact', title: 'evidence' },
   { id: 'fact:unrelated', kind: 'fact', title: 'other branch' },
  ],
  edges: [
   { id: 'e1', source: 'session', target: 'intent:done', relation: 'spawns' },
   { id: 'e2', source: 'intent:done', target: 'intent:active', relation: 'derived_from' },
   { id: 'e3', source: 'session', target: 'intent:pending', relation: 'spawns' },
   { id: 'e4', source: 'intent:active', target: 'fact:unrelated', relation: 'yields' },
   { id: 'e5', source: 'fact:parent', target: 'intent:active', relation: 'derived_from' },
  ],
 }
 const active = activeExplorationGraph(graph)
 assert.deepEqual(active.nodes.map(n => n.id).sort(), ['fact:parent', 'intent:active', 'intent:done', 'session'])
 assert.deepEqual(active.edges.map(e => e.id).sort(), ['e1', 'e2', 'e5'])
})

test('settled or pending-only explorations do not flow', () => {
 const nodes = [
  { id: 'intent:done', kind: 'intent', title: 'settled', status: 'completed' },
  { id: 'intent:pending', kind: 'intent', title: 'queued', status: 'pending', live: true },
 ]
 const edges = [{ id: 'e1', source: 'intent:done', target: 'intent:pending', relation: 'spawns' }]
 assert.deepEqual(activeExplorationGraph({ nodes, edges }), { nodes: [], edges: [] })
 const legacyNodes = [...nodes, { id: 'intent:legacy', kind: 'intent', title: 'legacy running', status: 'running', live: true }]
 const legacy = activeExplorationGraph({ nodes: legacyNodes, edges: [{ id: 'e2', source: 'intent:pending', target: 'intent:legacy', relation: 'spawns' }] })
 assert.deepEqual(legacy.nodes.map(n => n.id).sort(), ['intent:legacy', 'intent:pending'])
 assert.deepEqual(legacy.edges.map(e => e.id), ['e2'])
})

test('an in-progress intent only flows while its conversation is live', () => {
 const exploration = { intents: [{ id: 'i1', conversationId: 'session', title: 'current', status: 'in_progress' }], relations: [] }
 const running = buildExplorationGraph(board({ conversation: { id: 'session', title: 'fixture', target: 'https://example.invalid', status: 'running' }, exploration }))
 assert.equal(running.nodes.find(n => n.id === 'intent:i1').live, true)
 assert.deepEqual(activeExplorationGraph(running).edges.map(e => e.relation), ['spawns'])
 const completed = buildExplorationGraph(board({ conversation: { id: 'session', title: 'fixture', target: 'https://example.invalid', status: 'completed' }, exploration }))
 assert.equal(completed.nodes.find(n => n.id === 'intent:i1').live, false)
 assert.deepEqual(activeExplorationGraph(completed), { nodes: [], edges: [] })
})

test('direction prefers the live intent and keeps the latest unsettled intent after a run stops', () => {
 const live = {
  nodes: [
   { id: 'session', kind: 'start' },
   { id: 'intent:old', kind: 'intent', title: 'old', status: 'in_progress', updatedAt: '2026-10-01T00:00:00Z' },
   { id: 'intent:new', kind: 'intent', title: 'new', status: 'in_progress', live: true, updatedAt: '2026-10-02T00:00:00Z' },
   { id: 'intent:done', kind: 'intent', title: 'settled', status: 'completed', updatedAt: '2026-10-03T00:00:00Z' },
  ],
  edges: [
   { id: 'e0', source: 'session', target: 'intent:old', relation: 'spawns' },
   { id: 'e1', source: 'session', target: 'intent:new', relation: 'spawns' },
  ],
 }
 const current = explorationDirection(live)
 assert.equal(current.intent.id, 'intent:new')
 assert.equal(current.live, true)
 assert.deepEqual(current.edges.map(e => e.id), ['e1'])
 const stopped = explorationDirection({ nodes: live.nodes.map(n => ({ ...n, live: false })), edges: live.edges })
 assert.equal(stopped.intent.id, 'intent:new')
 assert.equal(stopped.live, false)
 assert.deepEqual(stopped.edges.map(e => e.id), ['e1'])
 const completedOnly = explorationDirection({
  nodes: [{ id: 'session', kind: 'start' }, { id: 'intent:done', kind: 'intent', status: 'completed', updatedAt: '2026-10-03T00:00:00Z' }],
  edges: [{ id: 'e0', source: 'session', target: 'intent:done', relation: 'spawns' }],
 })
 assert.equal(completedOnly.intent.id, 'intent:done')
 assert.equal(completedOnly.live, false)
 assert.deepEqual(completedOnly.edges.map(e => e.id), ['e0'])
 assert.equal(explorationDirection({ nodes: [{ id: 'session', kind: 'start' }], edges: [] }), null)
})

test('finding nodes keep recorded severity and target for the risk card', () => {
  const graph = buildExplorationGraph(board({ findings: [{ id: 'f', name: 'SQL injection', status: 'open', severity: 'high', severityLabel: 'High', rawTarget: 'https://target.example/login', url: 'https://target.example/login?q=1' }] }))
  const finding = graph.nodes.find(n => n.id === 'finding:f')
  assert.equal(finding.kind, 'finding')
  assert.equal(finding.severity, 'high')
  assert.equal(finding.severityLabel, 'High')
  assert.equal(finding.url, 'https://target.example/login')
  const fallback = buildExplorationGraph(board({ findings: [{ id: 'f2', name: 'XSS', url: 'https://target.example/search' }] }))
  assert.equal(fallback.nodes.find(n => n.id === 'finding:f2').url, 'https://target.example/search')
})

test('orchestration project roots belong to the Planner, not every Worker conversation', () => {
  const graph = buildExplorationGraph(board({
    scope: 'project', conversation: null,
    sourceConversations: [
      { id: 'controller', title: 'Controller', target: 'target', agentRole: 'controller' },
      { id: 'planner', title: 'Planner', target: 'target', agentRole: 'planner', status: 'running' },
      { id: 'worker', title: 'Worker A', target: 'target', agentRole: 'worker', parentConversationId: 'planner', planRunId: 'run' },
      { id: 'standalone', title: 'Standalone', target: 'target' },
    ],
    exploration: {
      intents: [{ id: 'i1', conversationId: 'planner', title: 'Test SSRF', status: 'stopped' }],
      relations: [], executions: [],
    },
    facts: [fact('worker-evidence', { sourceConversationId: 'worker' })],
  }))
  assert.deepEqual(graph.nodes.filter(n => n.kind === 'start').map(n => n.id).sort(), ['session:planner', 'session:standalone'])
  assert.ok(!graph.nodes.some(n => n.id === 'session:worker' || n.id === 'session:controller'))
  assert.equal(graph.nodes.find(n => n.id === 'intent:i1').status, 'stopped')
  assert.ok(graph.edges.some(e => e.source === 'session:planner' && e.target === 'intent:i1' && e.relation === 'spawns'))
  // A Worker's isolated evidence is scoped to the Planner root instead of
  // reviving the removed Worker root as another disconnected start.
  assert.ok(graph.edges.some(e => e.source === 'session:planner' && e.target === 'fact:worker-evidence' && e.relation === 'recorded' && e.provenance))
})

test('a worker-only filtered project view still gets a local context root', () => {
  const graph = buildExplorationGraph(board({
    scope: 'project', conversation: null,
    sourceConversations: [{ id: 'worker', title: 'Worker A', target: 'target', agentRole: 'worker', status: 'cancelled' }],
    facts: [fact('worker-evidence', { sourceConversationId: 'worker', confidence: 'tentative' })],
  }))
  assert.ok(graph.nodes.some(n => n.id === 'session:worker' && n.status === 'cancelled'))
  assert.ok(graph.edges.some(e => e.source === 'session:worker' && e.target === 'fact:worker-evidence' && e.relation === 'recorded'))
  assert.equal(graph.nodes.find(n => n.id === 'fact:worker-evidence').status, 'tentative')
})

test('tentative fact relations stay marked for the dashed tentative style', () => {
  const graph = buildExplorationGraph(board({
    facts: [fact('a'), fact('b')],
    edges: [{ sourceFactKey: 'a', targetFactKey: 'b', edgeType: 'supports', confidence: 'tentative' }],
  }))
  assert.equal(graph.edges.find(e => e.source === 'fact:a' && e.target === 'fact:b').tentative, true)
})
