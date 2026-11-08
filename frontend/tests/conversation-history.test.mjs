import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'
const source = readFileSync(new URL('../app/(main)/ai/workbench/conversation-history.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
const exports = {}
new Function('exports', compiled)(exports)
const { mergeConversationTurns, readingTurnId } = exports
const index = Array.from({ length: 236 }, (_, i) => ({ id: `round-${i}` }))

test('prepend, overlapping refresh, and rail jump retain ordered unique rounds', () => {
  const old = index.slice(180, 228).map(t => ({ ...t, body: 'old' }))
  const snapshot = index.slice(224).map(t => ({ ...t, body: 'updated' }))
  const merged = mergeConversationTurns(old, snapshot, index)
  assert.deepEqual(merged.map(t => t.id), index.slice(180).map(t => t.id))
  assert.equal(merged.find(t => t.id === 'round-224').body, 'updated')
  assert.equal(merged.find(t => t.id === 'round-180').body, 'old')
  assert.deepEqual(mergeConversationTurns(index.slice(120, 181), merged, index).map(t => t.id), index.slice(120).map(t => t.id))
})

test('reading highlight follows offset inside tall rounds, gaps and virtual range changes', () => {
  const rows = index.map(t => ({ key: t.id, kind: 'turn' }))
  const items = [{ index: 189, start: 0 }, { index: 190, start: 4000 }, { index: 191, start: 4200 }]
  assert.equal(readingTurnId(items, rows, 3999, false), 'round-189')
  assert.equal(readingTurnId(items, rows, 4000, false), 'round-190')
  assert.equal(readingTurnId(items.slice(1), rows, 4300, false), 'round-191')
  assert.equal(readingTurnId(items, rows, 0, true), 'round-191')
  const withFooter = [...rows, { key: 'footer', kind: 'footer' }]
  assert.equal(readingTurnId([...items, { index: 236, start: 5000 }], withFooter, 5500, true), 'round-191')
})

test('legacy conversations retain their ordering without a planner index', () => {
  assert.deepEqual(mergeConversationTurns([{ id: 'a' }], [{ id: 'a', body: 'new' }, { id: 'b' }], []), [{ id: 'a', body: 'new' }, { id: 'b' }])
})
