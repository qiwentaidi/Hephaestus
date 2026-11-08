import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../lib/ai-plan-view.ts', import.meta.url), 'utf8')
const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { planItemTitle, planItemTarget, planItemInstruction, filterPlanItems } = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`)
const item = (title, extra = {}) => ({ id: '1', seq: 1, title, instruction: title, target: '', status: 'completed', ...extra })

test('legacy instruction titles retain the action after target and technology notes', () => {
  const value = item('对 https://example.com 做首页指纹识别与被动侦察：检查响应头和安全配置')
  assert.equal(planItemTitle(value), '首页指纹识别与被动侦察')
  assert.equal(planItemTitle(item('针对 dev.example.com（Java/Spring 开发环境，测试配置）实测跨域带凭据读取：验证接口')), '实测跨域带凭据读取')
  assert.equal(planItemTitle(item('对 example.com（PHP8.2.29，含 /article.html）测试 SQL 注入，检查错误响应')), '测试 SQL 注入')
  assert.equal(planItemTitle(item('对 example.com 做验证：确认认证是否生效')), '验证：确认认证是否生效')
  assert.equal(value.instruction, value.title)
})

test('only explicit targets are recovered; versions, tools and file names are excluded', () => {
  assert.equal(planItemTarget(item('针对 a.example.com / b.example.com 做子域枚举（使用 crt.sh）')), 'a.example.com · b.example.com')
  assert.equal(planItemTarget(item('对 example.com（nginx1.21.6 + PHP8.2.29）测试 /article.html 和 main.js')), 'example.com')
  assert.equal(planItemTarget(item('使用 crt.sh 检查 example.com')), '')
  assert.equal(planItemTarget(item('对 192.168.2.101 做端口探测')), '192.168.2.101')
  assert.equal(planItemTarget(item('对 example.com 做检查', { target: 'https://explicit.example:8443/path' })), 'https://explicit.example:8443/path')
})

test('known instruction envelopes render their full summary without losing unknown payloads', () => {
  const instruction = JSON.stringify({ asset_ids: [1, 2], summary: '第一行\n第二行' })
  const value = item('任务', { instruction })
  assert.equal(planItemInstruction(value), '第一行\n第二行')
  assert.equal(value.instruction, instruction)
  const unknown = JSON.stringify({ summary: '摘要', constraints: '必须保留的范围' })
  assert.equal(planItemInstruction(item('任务', { instruction: unknown })), unknown)
  assert.equal(planItemInstruction(item('空指令回退', { instruction: '' })), '空指令回退')
})

test('search includes full instructions and exact states, including cancelled and future states', () => {
  const items = [item('A', { status: 'cancelled', instruction: 'Search Hidden.EXAMPLE /admin' }), item('B', { status: 'blocked', error: 'timeout' }), item('C', { status: 'skipped', resultSummary: 'scope excluded' })]
  assert.deepEqual(filterPlanItems(items, 'cancelled', ' hidden.example '), [items[0]])
  assert.deepEqual(filterPlanItems(items, 'failed', ''), [])
  assert.deepEqual(filterPlanItems(items, 'all', 'timeout'), [items[1]])
  assert.deepEqual(filterPlanItems(items, 'skipped', 'scope'), [items[2]])
  assert.equal(filterPlanItems(items, 'all', '').length, 3)
})
