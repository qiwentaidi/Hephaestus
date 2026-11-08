import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

async function loadModule(relative) {
  const source = readFileSync(new URL(relative, import.meta.url), 'utf8')
  const js = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText
  return import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)
}

const scan = await loadModule('../lib/asset-scan.ts')
const presets = await loadModule('../lib/combination-presets.ts')

const entity = (overrides) => ({
  id: overrides.id || 'asset-id',
  category: 'root',
  value: 'example.com',
  displayValue: 'example.com',
  sources: [],
  sourceTaskIds: [],
  firstSeenAt: '',
  lastSeenAt: '',
  status: 'active',
  verification: 'observed',
  ...overrides,
})

test('combination targets are derived from the template first-stage input type', () => {
  const entities = [
    entity({ id: 'root', category: 'root', value: 'Example.COM.', host: 'Example.COM' }),
    entity({ id: 'sub', category: 'subdomain', value: 'api.example.com', host: 'api.example.com' }),
    entity({ id: 'ip', category: 'ip', value: '1.2.3.4', host: '1.2.3.4' }),
  ]

  const domain = scan.deriveCombinationTargets(entities, 'domain')
  assert.deepEqual(domain.targets, ['example.com', 'api.example.com'])
  assert.equal(domain.skipped.length, 1)
  assert.equal(domain.skipped[0].reason, 'needs-domain')

  const network = scan.deriveCombinationTargets(entities, 'network')
  assert.deepEqual(network.targets, ['example.com', 'api.example.com', '1.2.3.4'])
  assert.equal(network.skipped.length, 0)
})

test('web targets only include HTTP(S) URLs and infer schemes for web ports', () => {
  const entities = [
    entity({ id: 'website', category: 'website', value: 'https://example.com/path#fragment', host: 'example.com' }),
    entity({ id: 'web-port', category: 'port', value: 'example.com:8080', host: 'example.com', port: 8080, protocol: 'tcp' }),
    entity({ id: 'ssh-port', category: 'port', value: 'example.com:22', host: 'example.com', port: 22, protocol: 'ssh / tcp' }),
    entity({ id: 'root', category: 'root', value: 'example.com', host: 'example.com' }),
    entity({ id: 'ip', category: 'ip', value: '1.2.3.4', host: '1.2.3.4' }),
  ]

  const derived = scan.deriveCombinationTargets(entities, 'web')
  assert.deepEqual(derived.targets, [
    'https://example.com/path',
    'http://example.com:8080',
    'https://example.com',
    'http://example.com',
  ])
  assert.deepEqual(
    derived.skipped.map((item) => item.reason).sort(),
    ['needs-web', 'non-web-port'],
  )
})

test('service targets require a known crack protocol and reuse port hosts', () => {
  const entities = [
    entity({ id: 'ssh', category: 'port', value: '10.0.0.5:22', host: '10.0.0.5', port: 22, protocol: 'ssh / tcp' }),
    entity({ id: 'http', category: 'port', value: '10.0.0.5:80', host: '10.0.0.5', port: 80, protocol: 'http / tcp' }),
    entity({ id: 'site', category: 'website', value: 'https://app.example.com', host: 'app.example.com' }),
  ]

  const derived = scan.deriveCombinationTargets(entities, 'service')
  assert.deepEqual(derived.targets, ['ssh://10.0.0.5:22'])
  assert.equal(derived.skipped.length, 2)
  assert.ok(derived.skipped.every((item) => item.reason === 'needs-service'))
})

test('AI scan targets keep one target per selected asset and add split instructions', () => {
  const entities = [
    entity({ id: 'root', category: 'root', value: 'example.com', host: 'example.com' }),
    entity({ id: 'site', category: 'website', value: 'https://example.com/login', host: 'example.com' }),
    entity({ id: 'port', category: 'port', value: 'example.com:22', host: 'example.com', port: 22, protocol: 'ssh / tcp' }),
    entity({ id: 'ip', category: 'ip', value: '10.0.0.8', host: '10.0.0.8' }),
    entity({ id: 'duplicate', category: 'ip', value: '10.0.0.8', host: '10.0.0.8' }),
  ]

  const targets = scan.deriveAITargets(entities)
  assert.deepEqual(targets.map((item) => item.target), [
    'example.com',
    'https://example.com/login',
    'example.com:22',
    '10.0.0.8',
  ])
  assert.deepEqual(targets.map((item) => item.category), ['root', 'website', 'port', 'ip'])

  const builtIn = scan.buildAIInstructions(targets.slice(0, 1), undefined, 'en')
  assert.equal(builtIn.length, 1)
  assert.match(builtIn[0], /domain example\.com/)

  const templated = scan.buildAIInstructions(targets.slice(0, 1), 'Assess {{.goal}} with evidence.', 'en')
  assert.equal(templated[0], 'Assess domain example.com with evidence.')

  const appended = scan.buildAIInstructions(targets.slice(0, 1), 'Assess the target.', 'en')
  assert.match(appended[0], /Assess the target\./)
  assert.match(appended[0], /Target: domain example\.com/)
})

test('preset drafts keep the first stage exclusive and valid for the backend', () => {
  const host = presets.buildPresetCombinationDraft('host-discovery', ['1.2.3.4'], 'host scan')
  assert.ok(host)
  assert.equal(host.nodes.length, 2)
  assert.deepEqual(host.nodes.map((node) => [node.moduleId, node.stageIndex]), [
    ['portscan', 0],
    ['webscan', 1],
  ])
  assert.equal(presets.combinationTemplateInputTypeFromNodes(host.nodes), 'network')
  assert.equal(host.schedule.mode, 'manual')
  assert.equal(host.scanMode, 'auto')

  const full = presets.buildPresetCombinationDraft('full-vuln-scan', ['example.com'], 'full')
  assert.ok(full)
  assert.deepEqual(full.nodes.filter((node) => node.stageIndex === 2).map((node) => node.moduleId), ['crack', 'webscan'])
})

test('saved templates reuse their workflow but reject mixed first-stage types', () => {
  const saved = {
    name: 'saved',
    targets: ['old.example.com'],
    nodes: [
      { id: 'a', moduleId: 'subdomain', stageIndex: 0, config: { thread: 100 } },
      { id: 'b', moduleId: 'portscan', stageIndex: 1, config: { threads: 500 } },
    ],
  }
  const draft = presets.buildTemplateCombinationDraft(saved, ['example.com'], 'from assets')
  assert.ok(draft)
  assert.deepEqual(draft.targets, ['example.com'])
  assert.equal(draft.nodes[0].config.thread, 100)
  assert.equal(presets.combinationTemplateInputTypeFromNodes(draft.nodes), 'domain')

  const mixed = presets.buildTemplateCombinationDraft({
    nodes: [
      { moduleId: 'subdomain', stageIndex: 0 },
      { moduleId: 'webscan', stageIndex: 0 },
    ],
  }, ['example.com'], 'mixed')
  assert.equal(presets.combinationTemplateInputTypeFromNodes(mixed.nodes), null)
})

test('skipped reasons are grouped for the dialog summary', () => {
  const summary = scan.summarizeSkippedReasons([
    { entity: entity({ id: '1' }), reason: 'needs-web' },
    { entity: entity({ id: '2' }), reason: 'needs-web' },
    { entity: entity({ id: '3' }), reason: 'non-web-port' },
  ])
  assert.deepEqual(summary, [
    { reason: 'needs-web', count: 2 },
    { reason: 'non-web-port', count: 1 },
  ])
})
