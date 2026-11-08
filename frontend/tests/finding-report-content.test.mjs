import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../lib/finding-report-content.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { findingReportBlocks } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

test('fenced code preserves indentation, masking and HTML literally', () => {
  const code = '  print("<script>not HTML</script>")\n  # **masked** user_id=1\n'
  const blocks = findingReportBlocks(`说明\n\n\`\`\`\`python\n${code}\`\`\`\n\`\`\`\`\n后续说明`)
  assert.equal(blocks[1].content, code + '```')
  assert.equal(blocks[1].language, 'python')
  assert.equal(blocks[2].content, '后续说明')
})

test('recorded request and incomplete JSON remain verbatim', () => {
  const request = 'GET /fixture?id=1 (token: recorded) →'
  const json = '{"phone":"130****9906","user_id":6,...}'
  const blocks = findingReportBlocks(`1) 核对响应：${request}\n${json}\n结论仅基于上述记录。`)
  assert.equal(blocks[0].content, '### 1. 核对响应')
  assert.equal(blocks[1].content, request)
  assert.equal(blocks[2].content, json)
  assert.equal(blocks[3].kind, 'markdown')
})

test('prose and references are not guessed as executable code', () => {
  const source = '说明：未取得响应，不能确认。\n文件：evidence/order_1.json\n1) 修复鉴权；2) 增加日志。'
  const blocks = findingReportBlocks(source)
  assert.ok(blocks.every(block => block.kind === 'markdown'))
  assert.ok(!blocks.some(block => block.content.includes('###')))
})

test('labeled JSON and tilde fenced content keep original field values', () => {
  const blocks = findingReportBlocks('用户信息：{"user_id":1,"masked":"**"}\n~~~json\n{"field":"value"}\n~~~')
  assert.equal(blocks[1].content, '{"user_id":1,"masked":"**"}')
  assert.equal(blocks[2].content, '{"field":"value"}')
  assert.equal(blocks[2].recorded, false)
})

test('Markdown links and bracketed prose stay prose', () => {
  const blocks = findingReportBlocks('[参考](https://example.invalid/report)\n[待验证] 需要更多证据。')
  assert.equal(blocks.length, 1)
  assert.equal(blocks[0].kind, 'markdown')
})
