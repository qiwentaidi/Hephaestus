import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const source = readFileSync(new URL('../lib/exploration-graph.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
const { buildFindingLineageGraph } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

test('finding lineage keeps facts in the drawer and collapses their explicit path in the graph', () => {
  const lineage = {
    nodes: [
      { id: 'intent:i', kind: 'intent', title: 'Verify access' },
      { id: 'fact:f', kind: 'fact', title: 'Observed access' },
      { id: 'finding:v', kind: 'finding', title: 'Missing authorization' },
      ...Array.from({ length: 100 }, (_, i) => ({ id: `execution:${i}`, kind: 'execution', title: 'http_request' })),
    ],
    edges: [
      { id: 'produces', source: 'intent:i', target: 'fact:f', relation: 'yields', persisted: true },
      { id: 'supports', source: 'fact:f', target: 'finding:v', relation: 'supports', persisted: true },
      ...Array.from({ length: 100 }, (_, i) => ({ id: `e${i}`, source: 'intent:i', target: `execution:${i}`, relation: 'evidence', persisted: true })),
    ],
  }
  const graph = buildFindingLineageGraph(lineage)
  assert.deepEqual(graph.nodes.map(n => n.id), ['intent:i', 'finding:v'])
  assert.equal(graph.edges.length, 1)
  assert.equal(graph.edges[0].source, 'intent:i')
  assert.equal(graph.edges[0].target, 'finding:v')
  assert.equal(graph.edges[0].relation, 'supports')
})
