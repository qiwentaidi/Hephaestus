import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const code = readFileSync(new URL('../lib/api/ai-mcp-tools.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(code, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText
const exports = {}
const calls = []
const fetchWithAuth = async (url, options) => {
  calls.push({ url, options })
  return new Response(JSON.stringify({ success: true, data: tools }))
}
new Function('require', 'exports', compiled)(() => ({ fetchWithAuth }), exports)
const { messageWithMCPTools, mcpToolTitle, mcpToolSummary, listCallableMCPTools } = exports
const tools = [
  { name: 'attack_surface_search_jsscan_vulnerabilities', displayName: 'attack_surface_search_jsscan_vulnerabilities', description: 'Find saved vulnerabilities', source: 'platform' },
  { name: 'external_mcp_search_service_a', displayName: 'search', description: 'Search A', source: 'external', sourceName: 'Service A' },
  { name: 'external_mcp_search_service_b', displayName: 'search', description: 'Search B', source: 'external', sourceName: 'Service B' },
]

test('selected tools reach the message with exact identities and preserve user requirements', () => {
  const content = '查询任务 task-1 的高危漏洞，固定版本 3。'
  assert.equal(messageWithMCPTools(content, [], 'zh'), content)
  const message = messageWithMCPTools(content, tools, 'zh')
  assert.ok(message.startsWith(content))
  for (const tool of tools) assert.ok(message.includes(tool.name))
  assert.ok(message.includes('必要信息不足时先向我询问'))
  assert.ok(messageWithMCPTools('', [tools[0]], 'en').includes('ask me first'))
})

test('external tools sharing display names retain distinct invocation identities', () => {
  const labels = { search: { title: 'Internal search', summary: 'Internal description' }, [tools[0].name]: { title: 'JS / API 漏洞检索', summary: '漏洞摘要' } }
  assert.equal(mcpToolTitle(tools[0], labels), 'JS / API 漏洞检索')
  assert.equal(mcpToolSummary(tools[0], labels), '漏洞摘要')
  for (const tool of tools.slice(1)) {
    assert.equal(mcpToolTitle(tool, labels), 'search')
    assert.equal(mcpToolSummary(tool, labels), tool.description)
  }
  assert.notEqual(tools[1].name, tools[2].name)
})

test('catalog requests use the authenticated API and support cancellation', async () => {
  const controller = new AbortController()
  assert.deepEqual(await listCallableMCPTools(controller.signal), tools)
  assert.equal(calls.at(-1).url, '/api/v1/mcp/callable-tools')
  assert.equal(calls.at(-1).options.signal, controller.signal)
})
