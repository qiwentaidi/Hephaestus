import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const code = readFileSync(new URL('../lib/ai-batch-view.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(code, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { parseBatchInstructions, summarizeBatch, filterBatchItems, deleteQueueSelection, batchDetailHref, canResumeQueue } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)
const queue = (id, status, items = []) => ({ id, name: id, status, items })
test('multiline instructions remain one task unless batch splitting is selected', () => {
  const input = '目标：检查登录\n范围：授权环境\n\n保留证据'
  assert.deepEqual(parseBatchInstructions(input, 'single'), [input])
  assert.deepEqual(parseBatchInstructions(input, 'batch'), ['目标：检查登录', '范围：授权环境', '保留证据'])
  assert.deepEqual(parseBatchInstructions(' \n ', 'single'), [])
})
test('processed includes failures while unfinished preserves failure recovery', () => {
  const items = [{ status: 'completed', instruction: 'a' }, { status: 'failed', instruction: 'b', error: 'timeout' }, { status: 'paused', instruction: 'c' }]
  assert.deepEqual(summarizeBatch(queue('q', 'paused', items)), { completed: 1, failed: 1, processed: 2, total: 3, unfinished: 2 })
  assert.equal(filterBatchItems(items, 'unfinished').length, 2)
  assert.deepEqual(filterBatchItems(items, 'failed', 'TIMEOUT'), [items[1]])
  assert.equal(canResumeQueue(queue('q', 'pausing', items)), false)
})
test('bulk deletion never calls active queues and preserves per-item failures', async () => {
  const calls = []
  const queues = [queue('done', 'completed'), queue('active', 'running'), queue('stopping', 'pausing'), queue('race', 'paused'), queue('last', 'pending')]
  const results = await deleteQueueSelection(queues, ['done', 'done', 'active', 'stopping', 'missing', 'race', 'last'], async id => {
    calls.push(id)
    if (id === 'race') throw new Error('queue is still stopping')
  }, 'stop first')
  assert.deepEqual(calls, ['done', 'race', 'last'])
  assert.deepEqual(results.filter(r => !r.error).map(r => r.id), ['done', 'last'])
  assert.deepEqual(results.filter(r => r.skipped).map(r => r.id), ['active', 'stopping', 'missing'])
  assert.equal(results.find(r => r.id === 'race').error, 'queue is still stopping')
})
test('detail URLs preserve unrelated parameters and clear the previous selection', () => {
  const url = new URL(batchDetailHref('/ai/tasks', 'queue=old&item=old&items=failed&keep=1', 'a&b', 'item/1', 'unfinished'), 'http://localhost')
  assert.equal(url.searchParams.get('queue'), 'a&b')
  assert.equal(url.searchParams.get('item'), 'item/1')
  assert.equal(url.searchParams.get('keep'), '1')
  const next = new URL(batchDetailHref('/ai/tasks', url.search, 'new'), url)
  assert.equal(next.searchParams.has('item'), false)
  assert.equal(next.searchParams.has('items'), false)
})
