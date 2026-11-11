const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')
const compiled = ts.transpileModule(fs.readFileSync(path.join(__dirname, 'finding-retest.ts'), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText
const loaded = { exports: {} }
new Function('exports', 'require', 'module', compiled)(loaded.exports, require, loaded)
const { findingRetestHref, findingRetestDestination, buildFindingRetestPrompt } = loaded.exports

const finding = {
  view: { id: 'canonical-finding', name: 'Observed issue', rawTarget: 'https://example.test', evidence: 'original response', reproductionSteps: 'replay request' },
  project: { id: 'project' },
  conversation: { id: 'worker', agentRole: 'worker', planRunId: 'orchestration:1' },
  retestTarget: { mode: 'orchestration', conversationId: 'main', planRunId: 'orchestration:1' },
}

test('entry URLs carry finding intent without selecting its source worker', () => {
  const url = new URL(findingRetestHref('id /?中文'), 'https://app.test')
  assert.equal(url.searchParams.get('findingId'), 'id /?中文')
  assert.equal(url.searchParams.get('retest'), '1')
  assert.equal(url.searchParams.has('session'), false)
  assert.equal(new URL(findingRetestHref('id', true), url).searchParams.get('retestMode'), 'independent')
})

test('default retest uses the resolved main while explicit independent mode clears the session', () => {
  assert.deepEqual(findingRetestDestination(finding, false), finding.retestTarget)
  assert.deepEqual(findingRetestDestination(finding, true), { mode: 'independent' })
  assert.deepEqual(findingRetestDestination({ ...finding, retestTarget: undefined }, false), { mode: 'independent' })
  assert.deepEqual(findingRetestDestination({ ...finding, retestTarget: { mode: 'independent' } }, false), { mode: 'independent' })
})

test('retest request carries canonical provenance and delegates exactly the targeted finding', () => {
  const prompt = buildFindingRetestPrompt(finding, 'orchestration', 'zh')
  const context = JSON.parse(prompt.slice(prompt.indexOf('{')))
  assert.equal(context.findingId, finding.view.id)
  assert.equal(context.sourceConversationId, 'worker')
  assert.equal(context.target, finding.view.rawTarget)
  assert.equal(context.originalEvidence, 'original response')
  assert.equal(context.reproductionSteps, 'replay request')
  assert.match(prompt, /add_intent/)
  assert.match(prompt, /record_vulnerability_retest/)
  assert.match(prompt, /不新增全局目标/)
  assert.match(prompt, /不覆盖原始证据/)
})

test('independent requests read full evidence without inheriting the orchestration command', () => {
  const prompt = buildFindingRetestPrompt({ ...finding, view: { ...finding.view, evidence: 'e'.repeat(12000) } }, 'independent', 'en')
  const context = JSON.parse(prompt.slice(prompt.indexOf('{')))
  assert.equal(context.originalEvidence.length, 4000)
  assert.doesNotMatch(prompt, /add_intent/)
  assert.match(prompt, /retest_vulnerability/)
  assert.match(prompt, /same findingId/)
  assert.match(prompt, /may be truncated/)
})
