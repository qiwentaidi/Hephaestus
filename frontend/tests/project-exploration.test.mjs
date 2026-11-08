import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const code = readFileSync(new URL('../lib/project-exploration.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(code, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { scopeProjectBoard, projectExplorationHref } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)

test('source filter retains referenced cross-session evidence without importing unrelated records', () => {
  const input = { facts: [{ factKey: 'own', sourceConversationId: 'a' }, { factKey: 'support', sourceConversationId: 'b' }, { factKey: 'unrelated', sourceConversationId: 'b' }], edges: [{ sourceFactKey: 'support', targetFactKey: 'own' }], findings: [{ id: 'f', taskId: 'a', sourceFactKey: 'support' }, { id: 'other', taskId: 'b' }], sourceConversations: [{ id: 'a' }, { id: 'b' }], todoPlans: [{ conversationId: 'a' }, { conversationId: 'b' }] }
  const result = scopeProjectBoard(input, 'a')
  assert.deepEqual(result.facts.map(f => f.factKey), ['own'])
  assert.deepEqual(result.contextFacts.map(f => f.factKey), ['support'])
  assert.deepEqual(result.findings.map(f => f.id), ['f'])
  assert.deepEqual(result.todoPlans, [{ conversationId: 'a' }])
  assert.equal(scopeProjectBoard(input, 'all'), input)
})
test('project links preserve tab, node and source without query injection', () => {
  const url = new URL(projectExplorationHref('project', 'evidence', 'fact:a&b', 'session'), 'http://localhost')
  assert.equal(url.searchParams.get('node'), 'fact:a&b')
  assert.equal(url.searchParams.get('source'), 'session')
  assert.equal(url.searchParams.get('tab'), 'evidence')
})

test('source filtering preserves explicit fact and finding parents of persistent intents', () => {
  const input = {
    scope: 'project', facts: [{ factKey: 'shared', sourceConversationId: 'a' }, { factKey: 'unrelated', sourceConversationId: 'a' }], findings: [{ id: 'f', taskId: 'a', sourceFactKey: 'shared' }], edges: [],
    exploration: { intents: [{ id: 'a', conversationId: 'a' }, { id: 'b', conversationId: 'b' }], relations: [{ source: 'fact:shared', target: 'intent:b', relation: 'derived_from', conversationId: 'b' }, { source: 'finding:f', target: 'intent:b', relation: 'derived_from', conversationId: 'b' }], executions: [{ id: 'exec', intentId: 'b', conversationId: 'b' }] },
    sourceConversations: [{ id: 'a' }, { id: 'b' }], todoPlans: [],
  }
  const result = scopeProjectBoard(input, 'b')
  assert.deepEqual(result.exploration.intents.map(i => i.id), ['b'])
  assert.equal(result.exploration.relations.length, 2)
  assert.deepEqual(result.contextFacts.map(f => f.factKey), ['shared'])
  assert.deepEqual(result.contextFindings.map(f => f.id), ['f'])
})

test('a finding parent also retains its supporting fact when no fact parent was supplied', () => {
  const input = { facts: [{ factKey: 'evidence', sourceConversationId: 'a' }], findings: [{ id: 'f', taskId: 'a', sourceFactKey: 'evidence' }], edges: [], exploration: { intents: [{ id: 'i', conversationId: 'b' }], relations: [{ source: 'finding:f', target: 'intent:i', relation: 'derived_from', conversationId: 'b' }] } }
  assert.deepEqual(scopeProjectBoard(input, 'b').contextFacts.map(f => f.factKey), ['evidence'])
})

test('source filtering keeps its goal proof and supporting evidence', () => {
  const input = { facts: [{ factKey: 'evidence', sourceConversationId: 'a' }], findings: [], edges: [], exploration: { goals: [{ id: 'g', conversationId: 'b' }, { id: 'other', conversationId: 'a' }], intents: [], relations: [{ source: 'fact:evidence', target: 'goal:g', relation: 'proves', conversationId: 'b' }] } }
  const result = scopeProjectBoard(input, 'b')
  assert.deepEqual(result.exploration.goals.map(g => g.id), ['g'])
  assert.deepEqual(result.contextFacts.map(f => f.factKey), ['evidence'])
})
