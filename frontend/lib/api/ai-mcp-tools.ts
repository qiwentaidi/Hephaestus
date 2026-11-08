import { fetchWithAuth } from '@/lib/api'

export type CallableMCPTool = {
  name: string
  displayName: string
  description: string
  source: 'platform' | 'system' | 'external'
  sourceName?: string
}

export async function listCallableMCPTools(signal?: AbortSignal): Promise<CallableMCPTool[]> {
  const response = await fetchWithAuth('/api/v1/mcp/callable-tools', { signal })
  const payload = await response.json()
  if (!response.ok || !payload.success) throw new Error(payload.error || `HTTP ${response.status}`)
  return payload.data
}

export type MCPToolLabels = Record<string, { title: string; summary: string }>

export function mcpToolTitle(tool: CallableMCPTool, labels: MCPToolLabels) {
  return tool.source === 'external' ? tool.displayName : labels[tool.name]?.title || tool.displayName
}

export function mcpToolSummary(tool: CallableMCPTool, labels: MCPToolLabels) {
  return tool.source === 'external' ? tool.description : labels[tool.name]?.summary || tool.description
}

export function messageWithMCPTools(content: string, tools: CallableMCPTool[], language: string) {
  if (!tools.length) return content
  const names = tools.map(tool => tool.name).join(', ')
  const request = language === 'zh'
    ? `请调用以下工具完成本次需求：${names}。根据本次需求与会话上下文填写参数；必要信息不足时先向我询问。`
    : `Use these tools for this request: ${names}. Fill parameters from this request and the conversation; ask me first if required information is missing.`
  return content ? `${content}\n\n${request}` : request
}
