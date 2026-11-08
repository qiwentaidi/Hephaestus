import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../lib/tcp-heuristic.ts', import.meta.url), 'utf8')
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
const api = {}
new Function('exports', code)(api)
const { normalizeTCPHeuristicConfig: normalize, validateTCPHeuristicConfig: validate } = api

test('old templates remain opt-out and explicit configuration survives normalization', () => {
  assert.deepEqual(normalize({}), { ...api.DEFAULT_TCP_HEURISTIC_CONFIG })
  assert.equal(normalize({}).subnetDiscoveryMode, 'fast')
  assert.equal(normalize({ subnetDiscoveryEnabled: true, subnetDiscoveryMode: 'full' }).subnetDiscoveryMode, 'full')
  assert.equal(normalize({ subnetDiscoveryEnabled: true, subnetDiscoveryMode: 'fast' }).subnetDiscoveryMode, 'fast')
  const custom = { tcpHeuristicEnabled: true, tcpHeuristicPorts: '8080,8443', tcpHeuristicTimeoutMs: 1500 }
  assert.deepEqual(normalize(custom), { ...api.DEFAULT_TCP_HEURISTIC_CONFIG, ...custom })
  assert.equal(validate(normalize({}), '80,U:53'), null)
})

test('enabled discovery rejects invalid ports, timeout and UDP before submission', () => {
  const defaults = normalize({ tcpHeuristicEnabled: true })
  assert.equal(validate(defaults, '1-65535'), null)
  assert.equal(validate({ ...defaults, tcpHeuristicPorts: '' }, '80'), null)
  for (const ports of ['0', '65536', '80-443', '80,,443', 'abc', Array.from({ length: 65 }, (_, i) => i + 1).join(',')]) {
    assert.equal(validate({ ...defaults, tcpHeuristicPorts: ports }, '80'), 'portsError')
  }
  for (const timeout of [0, 99, 30001, 1000.5]) {
    assert.equal(validate({ ...defaults, tcpHeuristicTimeoutMs: timeout }, '80'), 'timeoutError')
  }
  assert.equal(validate(defaults, '80,U:53'), 'udpError')
})


test('subnet discovery stays opt-in, requires TCP settings, and preserves explicit zero', () => {
  assert.equal(normalize({}).subnetDiscoveryEnabled, false)
  const config = normalize({ subnetDiscoveryEnabled: true, subnetDiscoveryRandomSamples: 0 })
  assert.equal(config.tcpHeuristicEnabled, true)
  assert.equal(config.subnetDiscoveryMode, 'full')
  assert.equal(config.subnetDiscoveryRandomSamples, 0)
  assert.equal(validate(config, '80'), null)
  assert.equal(validate(config, 'U:53'), 'udpError')
  assert.equal(validate({ ...config, subnetDiscoveryMode: 'bad' }, '80'), 'subnetModeError')
  for (const suffixes of ['0', '255', '1,,2', '1-254']) assert.equal(validate({ ...config, subnetDiscoveryHostSuffixes: suffixes }, '80'), 'subnetSuffixError')
  for (const samples of [-1, 33, 1.5]) assert.equal(validate({ ...config, subnetDiscoveryRandomSamples: samples }, '80'), 'subnetRandomError')
})
