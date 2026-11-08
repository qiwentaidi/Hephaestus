import { fetchWithAuth } from '@/lib/api'

export type AgentToolRole = 'planner' | 'worker' | 'controller' | 'pentest'
export type AgentToolSource = 'platform' | 'system' | 'mcp'

export interface AgentTool {
  name: string
  title: string
  description: string
  source: AgentToolSource
  enabled: boolean
  serverId?: string
  serverName?: string
}

export interface AgentToolVisibilityConfig {
  roles: AgentToolRole[]
  tools: AgentTool[]
  /** Disabled tool names per role. Missing names are visible. */
  visibility: Record<AgentToolRole, string[]>
}

export async function listAgentTools(): Promise<AgentToolVisibilityConfig> {
  const response = await fetchWithAuth('/api/v1/mcp/agent-tool-visibility')
  const result = await response.json()
  if (!response.ok || !result.success || !result.data) throw new Error('load_failed')
  return result.data as AgentToolVisibilityConfig
}

export async function saveAgentTools(visibility: Record<AgentToolRole, string[]>): Promise<void> {
  const response = await fetchWithAuth('/api/v1/mcp/agent-tool-visibility', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ visibility }),
  })
  const result = await response.json()
  if (!response.ok || !result.success) throw new Error(result.error || 'save_failed')
}
