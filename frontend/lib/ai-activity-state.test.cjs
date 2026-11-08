const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const ts = require('typescript')
const source = fs.readFileSync(__dirname + '/ai-activity-state.ts', 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
const loaded = { exports: {} }
new Function('exports', 'require', 'module', compiled)(loaded.exports, require, loaded)
const { activityAnimationStates } = loaded.exports
const phase = (eventType, status = 'running') => ({ type: 'marker', eventType, status, title: eventType })

test('planning and retries stop animating when the next model step arrives', () => {
  const items = [phase('iteration')]
  assert.deepEqual(activityAnimationStates(items, true), [true])
  items.push(phase('reasoning', 'cancelled'), phase('model_retry'))
  assert.deepEqual(activityAnimationStates(items, true), [false, false, true])
  items.push(phase('reasoning'))
  assert.deepEqual(activityAnimationStates(items, true), [false, false, false, true])
  items[3] = phase('reasoning', 'completed')
  items.push(phase('iteration'))
  assert.deepEqual(activityAnimationStates(items, true), [false, false, false, false, true])
  assert.equal(items[0].status, 'running', 'presentation must not rewrite recorded events')
})

test('successive retries and fallback leave only the current phase animated', () => {
  const items = [phase('iteration'), phase('model_retry'), phase('model_retry'), phase('model_fallback')]
  assert.deepEqual(activityAnimationStates(items, true), [false, false, false, true])
  items.push(phase('model_retry_exhausted', 'failed'))
  assert.deepEqual(activityAnimationStates(items, true), [false, false, false, false, false])
})

test('parallel tools and subtasks keep their own animation lifecycle', () => {
  const items = [phase('iteration'), { type: 'tool', status: 'running', title: 'tool A' }, { type: 'tool', status: 'running', title: 'tool B' }, phase('subtask'), phase('reasoning')]
  assert.deepEqual(activityAnimationStates(items, true), [false, true, true, true, true])
  items[1].status = 'completed'
  assert.deepEqual(activityAnimationStates(items, true), [false, false, true, true, true])
  assert.deepEqual(activityAnimationStates(items, false), [false, false, false, false, false])
})

test('old history without event types also stops stale phase animations', () => {
  const items = [
    { type: 'marker', title: '第 2 轮：请求模型规划下一步', status: 'running' },
    { type: 'marker', title: '模型调用遇到临时问题，正在重试', status: 'running' },
    phase('reasoning'),
  ]
  assert.deepEqual(activityAnimationStates(items, true), [false, false, true])
})

test('terminal states stay static and missing status is active only at the tail', () => {
  assert.deepEqual(activityAnimationStates([phase('iteration', 'cancelled')], true), [false])
  assert.deepEqual(activityAnimationStates([{ title: 'old' }, { title: 'current' }], true), [false, true])
  assert.deepEqual(activityAnimationStates([phase('synthesis'), phase('assistant_commentary')], true), [false, true])
})
