import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const code = readFileSync(new URL('../lib/ai-goal-mode.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(code, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { goalTurnMessages } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)
const stages = [{ id: 'stage-1', content: 'Still working' }, { id: 'stage-2', content: 'Evidence collected' }]
const final = { id: 'final', content: 'Verified conclusion\nEvidence and limits' }
const goal = { id: 'old-goal', mode: 'goal', status: 'completed', reason: final.content }

test('completed goal exposes its final conclusion and retains earlier replies as details', () => {
  const messages = [...stages, final]
  const presentation = goalTurnMessages(messages, goal)
  assert.equal(presentation.completed, true)
  assert.deepEqual(presentation.conclusions, [final])
  assert.deepEqual(presentation.details, stages)
  assert.deepEqual(messages, [...stages, final])
})

test('completion update arriving before final message does not hide the last stage reply', () => {
  assert.deepEqual(goalTurnMessages(stages, goal).conclusions, stages)
  assert.equal(goalTurnMessages(stages, goal).completed, false)
  assert.equal(goalTurnMessages([], goal).completed, false)
})

test('ordinary follow-ups and unsuccessful goals retain all their replies', () => {
  const messages = [...stages, final]
  for (const status of ['active', 'blocked', 'failed', 'stopped', 'budget_exhausted']) {
    assert.deepEqual(goalTurnMessages(messages, { ...goal, status }).conclusions, messages)
  }
  assert.deepEqual(goalTurnMessages(stages).conclusions, stages)
  assert.deepEqual(goalTurnMessages([...stages, { content: 'Ordinary follow-up' }], goal).conclusions, [...stages, { content: 'Ordinary follow-up' }])
  assert.equal(goalTurnMessages([{ content: '' }], { ...goal, reason: '' }).completed, false)
})
