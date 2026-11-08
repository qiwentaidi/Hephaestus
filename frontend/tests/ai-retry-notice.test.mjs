import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const code = readFileSync(new URL('../lib/ai-retry-notice.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(code, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { foldModelRetryActivities, modelRetryNotice } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)

test('structured attempt counters win over the detail text', () => {
  assert.deepEqual(modelRetryNotice('模型调用失败，9s 后第 2/5 次重试', 3, 5), { attempt: 3, max: 5 })
})

test('older retry notices parse the attempt from persisted detail', () => {
  assert.deepEqual(modelRetryNotice('模型调用失败，9.607986834s 后第 5/5 次重试'), { attempt: 5, max: 5 })
  assert.equal(modelRetryNotice('模型调用失败'), null)
  assert.equal(modelRetryNotice('', 6, 5), null)
})

test('consecutive retry notices collapse to one row with the latest count', () => {
  const items = Array.from({ length: 5 }, (_, index) => ({
    id: `retry-${index + 1}`,
    eventType: 'model_retry',
    title: '模型调用遇到临时问题，正在重试',
    detail: `模型调用失败，${index + 1}s 后第 ${index + 1}/5 次重试`,
    status: 'running',
    retry: { attempt: index + 1, max: 5 },
  }))
  const folded = foldModelRetryActivities(items)
  assert.equal(folded.length, 1)
  assert.equal(folded[0].id, 'retry-5')
  assert.deepEqual(folded[0].retry, { attempt: 5, max: 5 })
})

test('retry attempts without structured data fall back to the detail text', () => {
  const folded = foldModelRetryActivities([
    { id: 'a', eventType: 'model_retry', title: 'retry', detail: '模型调用失败，1s 后第 1/5 次重试' },
    { id: 'b', eventType: 'model_retry', title: 'retry', detail: '模型调用失败，4s 后第 4/5 次重试' },
  ])
  assert.equal(folded.length, 1)
  assert.deepEqual(folded[0].retry, { attempt: 4, max: 5 })
})

test('non retry activities break the retry sequence', () => {
  const folded = foldModelRetryActivities([
    { id: 'a', eventType: 'model_retry', title: 'retry', retry: { attempt: 1, max: 5 } },
    { id: 'b', eventType: 'tool_done', title: 'tool' },
    { id: 'c', eventType: 'model_retry', title: 'retry', retry: { attempt: 1, max: 5 } },
  ])
  assert.deepEqual(folded.map(activity => activity.id), ['a', 'b', 'c'])
})

test('retries without counters stay one row without a fabricated count', () => {
  const folded = foldModelRetryActivities([
    { id: 'a', eventType: 'model_retry', title: 'retry' },
    { id: 'b', eventType: 'model_retry', title: 'retry' },
  ])
  assert.equal(folded.length, 1)
  assert.equal(folded[0].retry, undefined)
})
