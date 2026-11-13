import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../app/(main)/ai/workbench/session-title.ts', import.meta.url), 'utf8')
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
const exports = {}
new Function('exports', 'require', code)(exports, (name) => {
  throw new Error(`unexpected import ${name}`)
})
const { sessionTitle } = exports

const row = (task, latestMessage = null) => ({ task, latestMessage })

test('orchestration Controller and Planner rows are named after the run objective', () => {
  const objective = '请对下面URL目标进行渗透测试漏洞挖掘'
  for (const [role, prefix] of [['controller', '主控'], ['planner', 'Planner']]) {
    const name = `${prefix} · ${objective}`
    const summary = row({ name, target: 'https://cn.unimtx.com/sms/us' })
    assert.equal(sessionTitle(summary, '未命名目标'), name)
    assert.ok(!sessionTitle(summary, '未命名目标').includes('https://'), `${role} row still shows the first target`)
  }
})

test('a solo session keeps its own title', () => {
  assert.equal(sessionTitle(row({ name: 'https://example.test/login', target: 'https://example.test/login' }), '未命名目标'), 'https://example.test/login')
})

test('a session without a title falls back to its target, then the unnamed label', () => {
  assert.equal(sessionTitle(row({ name: '', target: 'https://example.test/a' }), '未命名目标'), 'https://example.test/a')
  assert.equal(sessionTitle(row({ name: '', target: '' }, { content: '继续测试' }), '未命名目标'), '继续测试')
  assert.equal(sessionTitle(null, '未命名目标'), '未命名目标')
})

test('long titles are compacted to a single truncated line', () => {
  const title = sessionTitle(row({ name: `目标要求\n${'A'.repeat(80)}`, target: 'https://example.test' }), '未命名目标')
  assert.ok(!title.includes('\n'))
  assert.ok(title.endsWith('…'))
  assert.equal([...title].length, 42)
})
