import { fetchWithAuth } from '@/lib/api'

export type AgentPromptRole = 'pentest' | 'planner' | 'worker' | 'controller'

export interface AgentPrompt {
  role: AgentPromptRole
  prompt: string
  defaultPrompt: string
  version: number
  customized: boolean
  updatedAt: string
}

export async function listAgentPrompts(): Promise<AgentPrompt[]> {
  const response = await fetchWithAuth('/api/v1/ai/agent-prompts')
  const result = await response.json()
  if (!response.ok || !result.success || !Array.isArray(result.data)) throw new Error('load_failed')
  return result.data
}

export async function saveAgentPrompt(item: AgentPrompt, prompt: string, reset = false): Promise<AgentPrompt> {
  const response = await fetchWithAuth(`/api/v1/ai/agent-prompts/${item.role}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt, version: item.version, reset }),
  })
  const result = await response.json()
  if (!response.ok || !result.success) throw new Error(result.code || 'save_failed')
  return result.data
}
