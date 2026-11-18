import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../app/(main)/scan/vuln-scan/components/combinationFlowEdges.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
const { buildCombinationFlowEdges, buildCombinationFlowPaths, combinationFlowEdgeKey } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const stage = (id, modules) => ({ id, nodes: modules.map((moduleId, index) => ({ id: `${id}-${moduleId}-${index}`, moduleId })) })
const keys = (edges) => edges.map((edge) => combinationFlowEdgeKey(edge.from, edge.to))
const nodeId = (stages, moduleId) => stages.flatMap((item) => item.nodes).find((node) => node.moduleId === moduleId).id

test('full scan preset links JS analysis to the web scan, not to the credential attack', () => {
  const stages = [stage('s1', ['subdomain']), stage('s2', ['portscan']), stage('s3', ['crack', 'webscan']), stage('s4', ['jsscan'])]
  const edges = buildCombinationFlowEdges(stages)
  const jsscan = nodeId(stages, 'jsscan')
  const webscan = nodeId(stages, 'webscan')
  const crack = nodeId(stages, 'crack')
  assert.deepEqual(keys(edges).sort(), [
    combinationFlowEdgeKey(nodeId(stages, 'subdomain'), nodeId(stages, 'portscan')),
    combinationFlowEdgeKey(nodeId(stages, 'portscan'), crack),
    combinationFlowEdgeKey(nodeId(stages, 'portscan'), webscan),
    combinationFlowEdgeKey(webscan, jsscan),
  ].sort())
  assert.ok(!edges.some((edge) => edge.to === jsscan && edge.from === crack), 'the credential attack must not feed the JS scan')
})

test('the edge follows the module that precedes the stage, not the card order inside it', () => {
  const stages = [stage('s1', ['subdomain']), stage('s2', ['portscan']), stage('s3', ['webscan', 'crack']), stage('s4', ['jsscan'])]
  const edges = buildCombinationFlowEdges(stages)
  const jsscan = nodeId(stages, 'jsscan')
  assert.deepEqual(edges.filter((edge) => edge.to === jsscan).map((edge) => edge.from), [nodeId(stages, 'webscan')])
})

test('a stage without a declared parent links to every node of the closest non-empty stage', () => {
  const stages = [stage('s1', ['portscan']), stage('s2', []), stage('s3', ['jsscan'])]
  const edges = buildCombinationFlowEdges(stages)
  assert.deepEqual(edges.map((edge) => [edge.from, edge.to]), [
    [nodeId(stages, 'portscan'), nodeId(stages, 'jsscan')],
  ])
})

test('edges never skip a stage, so curves stay inside the gutter', () => {
  const stages = [stage('s1', ['webscan']), stage('s2', ['portscan']), stage('s3', ['jsscan'])]
  const edges = buildCombinationFlowEdges(stages)
  assert.ok(edges.some((edge) => edge.from === nodeId(stages, 'portscan') && edge.to === nodeId(stages, 'jsscan')))
  assert.ok(!edges.some((edge) => edge.from === nodeId(stages, 'webscan') && edge.to === nodeId(stages, 'jsscan')))
})

test('directory scanning hangs off the web scan, and a lone first stage has no inbound edge', () => {
  const stages = [stage('s1', ['webscan']), stage('s2', ['dirscan'])]
  const edges = buildCombinationFlowEdges(stages)
  assert.deepEqual(edges.map((edge) => [edge.from, edge.to]), [[nodeId(stages, 'webscan'), nodeId(stages, 'dirscan')]])
})

test('flow paths leave the source card edge, end on the target edge and carry an arrow head', () => {
  const points = {
    a: { left: 0, right: 280, cy: 50 },
    b: { left: 344, right: 624, cy: 50 },
  }
  const paths = buildCombinationFlowPaths(points, [{ from: 'a', to: 'b' }])
  assert.equal(paths.length, 2)
  const curve = paths.find((path) => path.kind === 'curve')
  const arrow = paths.find((path) => path.kind === 'arrow')
  assert.equal(curve.edgeKey, 'a->b')
  assert.match(curve.d, /^M 280 50 C /)
  assert.match(curve.d, / 344 50$/)
  assert.match(arrow.d, /L 344 50 /)
})

test('flow paths skip edges that reference a node the canvas did not render', () => {
  const paths = buildCombinationFlowPaths({ a: { left: 0, right: 10, cy: 5 } }, [{ from: 'a', to: 'missing' }])
  assert.deepEqual(paths, [])
})
