const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

function load(file) {
  const code = ts.transpileModule(fs.readFileSync(path.join(__dirname, file), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const module = { exports: {} }
  new Function('exports', 'module', code)(module.exports, module)
  return module.exports
}
const { scanFindingDetailHref, parseScanFindingReference } = load('scan-finding-detail.ts')
const { findingReturnHref } = load('finding-detail.ts')

test('Web detail links preserve complete identity, historical version and return filters', () => {
  const finding = { source: 'web', taskId: 'scan/a', version: 3, templateId: 'rule&a', protocol: 'http', url: 'https://example.test/a?x=1&y=2', name: '漏洞 / 标题', taskName: '历史扫描', createdAt: '2026-07-03T15:31:28+08:00' }
  const from = '/vulnerabilities?source=web&severity=high&page=2'
  const url = new URL(scanFindingDetailHref(finding, from), 'http://localhost')
  const parsed = parseScanFindingReference(url.searchParams)
  for (const [key, value] of Object.entries(finding)) assert.equal(parsed[key], value)
  assert.equal(findingReturnHref(url.searchParams.get('from')), from)
})

test('JS detail links preserve native ID and exact scan version without display metadata', () => {
  const url = new URL(scanFindingDetailHref({ source: 'js', taskId: 'task', version: 2, nativeId: 'id/a?b' }), 'http://localhost')
  const parsed = parseScanFindingReference(url.searchParams)
  assert.equal(parsed.nativeId, 'id/a?b')
  assert.equal(parsed.version, 2)
})

test('incomplete and invalid scan identities are rejected instead of loading latest', () => {
  for (const query of [
    'source=web&taskId=t&url=https://example.test',
    'source=web&taskId=t&version=1',
    'source=js&taskId=t&version=1',
    'source=ai&taskId=t&version=1&nativeId=id',
    ...['0', '-1', '1.5', 'NaN', 'Infinity', '9007199254740992'].map(version => `source=js&taskId=t&nativeId=id&version=${version}`),
  ]) assert.equal(parseScanFindingReference(new URLSearchParams(query)), null, query)
})

test('return links allow native scan detail filters but reject external destinations', () => {
  for (const route of ['/scan/vuln-scan/detail?taskId=a&version=2&tab=vulnerability', '/scan/js-scan/detail?taskId=b&version=3', '/vulnerabilities?source=js']) assert.equal(findingReturnHref(route), route)
  for (const route of ['https://example.test', '//example.test', '/scan/vuln-scan/detail/../../other', '/vulnerabilities-evil', null]) assert.equal(findingReturnHref(route), '/vulnerabilities')
})

test('Web permalink can use its persistent ID without mutable descriptive fields', () => {
  const url = new URL(scanFindingDetailHref({ source: 'web', taskId: 'task', version: 3, nativeId: 'persisted-web-uuid' }), 'http://localhost')
  const parsed = parseScanFindingReference(url.searchParams)
  assert.equal(parsed.nativeId, 'persisted-web-uuid')
  assert.equal(parsed.version, 3)
})
