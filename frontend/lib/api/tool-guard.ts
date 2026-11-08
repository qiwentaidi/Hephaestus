import { fetchWithAuth } from '@/lib/api'

export interface ToolGuardRule {
  id?: string
  name: string
  enabled: boolean
  origin?: string
  target?: 'tool_input' | 'executable_input' | 'http_method'
  priority?: number
  tools?: string[]
  pattern: string
  message: string
}

export interface ToolGuardConfig {
  enabled: boolean
  rules: ToolGuardRule[]
}

export interface ToolGuardMatch {
  ruleId: string
  ruleName: string
  matchedText: string
  message: string
}

export interface ToolGuardTestResult {
  blocked: boolean
  match?: ToolGuardMatch | null
}

async function parseResponse<T>(response: Response): Promise<T> {
  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    throw new Error(result.error || result.message || `HTTP ${response.status}`)
  }
  return result.data as T
}

export async function getToolGuardConfig(): Promise<ToolGuardConfig> {
  return parseResponse<ToolGuardConfig>(await fetchWithAuth('/api/v1/config/tool-guard'))
}

export async function saveToolGuardConfig(config: ToolGuardConfig): Promise<ToolGuardConfig> {
  return parseResponse<ToolGuardConfig>(await fetchWithAuth('/api/v1/config/tool-guard', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  }))
}

export async function testToolGuardConfig(
  config: ToolGuardConfig,
  toolName: string,
  args: unknown,
): Promise<ToolGuardTestResult> {
  return parseResponse<ToolGuardTestResult>(await fetchWithAuth('/api/v1/config/tool-guard/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ config, toolName, arguments: args }),
  }))
}
