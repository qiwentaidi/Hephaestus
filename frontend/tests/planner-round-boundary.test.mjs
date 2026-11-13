import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../lib/ai-workbench-rounds.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
const exports = {}
new Function('exports', compiled)(exports)
const { isPlannerRoundBoundary } = exports

test('planner round boundaries render as dividers', () => {
  assert.equal(isPlannerRoundBoundary({ roundBoundary: true }), true)
  assert.equal(isPlannerRoundBoundary({ title: '第 23 轮规划', eventType: 'assistant_commentary' }), true)
  assert.equal(isPlannerRoundBoundary({ title: '第330轮规划', eventType: 'assistant_commentary' }), true)
  assert.equal(isPlannerRoundBoundary({ title: '第 0 轮目标拆解', eventType: 'assistant_commentary' }), true)
})

test('ordinary commentary and other activity types stay inline', () => {
  assert.equal(isPlannerRoundBoundary({ title: '本轮不新增意图，理由如下', eventType: 'assistant_commentary' }), false)
  assert.equal(isPlannerRoundBoundary({ title: '推理过程', eventType: 'reasoning' }), false)
  assert.equal(isPlannerRoundBoundary({ title: '工具调用完成：graph_overview', eventType: 'tool_done' }), false)
  assert.equal(isPlannerRoundBoundary({ title: '第 9 轮规划', eventType: 'reasoning' }), false)
})
