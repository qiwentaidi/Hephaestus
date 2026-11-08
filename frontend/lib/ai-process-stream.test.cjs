const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const ts = require('typescript')
const source = fs.readFileSync(__dirname + '/ai-process-stream.ts', 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
const loaded = { exports: {} }
new Function('exports', 'require', 'module', compiled)(loaded.exports, require, loaded)
const { AIProcessMerger, readAIProcessFrames } = loaded.exports
const item = (id, sequence, text) => ({ id, type: 'marker', status: 'running', description: text, metadata: { sequence }, createdAt: '', updatedAt: '' })
const snapshot = (items = []) => ({ task: { id: 'a', status: 'running' }, chatItems: items, turns: [{ id: 'turn', chatItems: items }], timeline: {} })
const update = (sequence, text) => ({ sessionId: 'a', blockId: 'block', runId: 'run', sequence, item: item('block', sequence, text) })

test('live blocks survive stale snapshots, deduplicate and reconcile terminal state', () => {
 const merger = new AIProcessMerger(); merger.reset('a')
 assert.equal(merger.accept(update(2, '更新后的分析')), true)
 assert.equal(merger.accept(update(2, '重复')), false)
 assert.equal(merger.accept(update(1, '旧内容')), false)
 let next = merger.merge(snapshot([item('block', 1, '旧快照')]))
 assert.equal(next.chatItems[0].description, '更新后的分析')
 assert.equal(next.turns[0].chatItems.length, 1)
 assert.equal(merger.merge(snapshot([item('block', 3, '更新快照')])).chatItems[0].description, '更新快照')
 assert.equal(merger.merge(snapshot([item('block', 1, '迟到的另一条连接')])).chatItems[0].description, '更新快照')
 merger.accept({ ...update(4, ''), item: null })
 assert.equal(merger.merge(next).chatItems.length, 0)
 const terminal = { ...snapshot(), task: { id: 'a', status: 'cancelled' } }
 assert.equal(merger.merge(terminal), terminal)
 assert.equal(merger.merge(snapshot()).chatItems.length, 0)
})

test('blocks stay in their original turn and cannot leak across sessions', () => {
 const merger = new AIProcessMerger(); merger.reset('a'); merger.accept(update(2, '分析'))
 const old = item('block', 1, '旧')
 const state = { ...snapshot([old]), turns: [{ id: 'old', chatItems: [old] }, { id: 'new', chatItems: [] }] }
 const next = merger.merge(state)
 assert.equal(next.turns[0].chatItems[0].description, '分析')
 assert.equal(next.turns[1].chatItems.length, 0)
 merger.reset('b')
 assert.equal(merger.accept(update(3, '错误会话')), false)
 assert.equal(merger.merge(snapshot()).chatItems.length, 0)
})

test('SSE handles split UTF-8/CRLF and yields during event bursts', async () => {
 const enc = new TextEncoder()
 const text = Array.from({ length: 100 }, (_, i) => `event: process_update\r\ndata: ${JSON.stringify({ text: '中文分片', i })}\r\n\r\n`).join('')
 const bytes = enc.encode(text)
 const body = new ReadableStream({ start(c) { c.enqueue(bytes.slice(0, 63)); c.enqueue(bytes.slice(63)); c.close() } })
 let painted = false; setTimeout(() => { painted = true }, 0)
 const received = []
 await readAIProcessFrames(new Response(body), (type, data) => { assert.equal(type, 'process_update'); received.push(data); if (data.i > 32) assert.equal(painted, true) })
 assert.equal(received.length, 100); assert.equal(received[99].text, '中文分片')
})

test('paint fallback flushes once when animation frames are suspended', async () => {
 const originalRAF = global.requestAnimationFrame, originalCancel = global.cancelAnimationFrame
 let onFrame, cancelled = 0, flushed = 0
 global.requestAnimationFrame = callback => { onFrame = callback; return 7 }
 global.cancelAnimationFrame = id => { assert.equal(id, 7); cancelled++ }
 try {
  const cancel = loaded.exports.scheduleProcessFrame(() => { flushed++ })
  await new Promise(resolve => setTimeout(resolve, 80))
  assert.equal(flushed, 1)
  onFrame(); cancel()
  assert.equal(flushed, 1); assert.ok(cancelled >= 1)
  const cancelPending = loaded.exports.scheduleProcessFrame(() => { flushed++ })
  cancelPending()
  await new Promise(resolve => setTimeout(resolve, 80))
  assert.equal(flushed, 1)
 } finally { global.requestAnimationFrame = originalRAF; global.cancelAnimationFrame = originalCancel }
})
