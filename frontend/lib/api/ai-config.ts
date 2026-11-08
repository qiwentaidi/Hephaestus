import { fetchWithAuth } from '@/lib/api'

export interface AIConfig {
  requestTimeoutMinutes?: number
  enabled: boolean
  protocol?: AIProviderProtocol
  api: string
  key: string
  model: string
  defaultProviderId?: string
  providers?: AIProviderConfig[]
  routing?: AIRoutingConfig
  workbenchRemoteDispatch?: boolean
}

export type AIProviderProtocol = 'openai-compatible' | 'anthropic'

export interface AIProviderConfig {
  /** Complete model request timeout in minutes; missing/0 defaults to 30. */
  requestTimeoutMinutes?: number
  workbenchMaxConcurrent?: number
  workbenchRequestsPerMinute?: number
  workbenchRequestRateUnit?: 'second' | 'minute'
  /** 单次模型调用上下文窗口软预算（token），0/undefined 使用默认 120000 */
  maxTotalTokens?: number
  /** 单次模型回复输出上限（token），0/undefined 使用运行时默认 */
  maxOutputTokens?: number
  id: string
  name: string
  protocol: AIProviderProtocol
  enabled: boolean
  api: string
  key: string
  model: string
  supportsTools?: boolean
  supportsStreaming?: boolean
}

export interface AIRoutingConfig {
  workbench?: string
  workbenchFallbackEnabled?: boolean
  workbenchFallback?: string
  jsscan?: string
  repeaterPolish?: string
  subdomainAssist?: string
}

export interface AIModelOption {
  id: string
  name?: string
}

export async function listAIModels(
  provider: Pick<AIProviderConfig, 'protocol' | 'api' | 'key'>,
  signal?: AbortSignal,
): Promise<AIModelOption[]> {
  const response = await fetchWithAuth('/api/v1/config/ai/models', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(provider),
    signal,
  })
  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success || !Array.isArray(result.data)) {
    throw new Error(result.code || 'request_failed')
  }
  return result.data
}

export async function getAIConfig(): Promise<AIConfig> {
  const response = await fetchWithAuth('/api/v1/config/ai')
  const result = await response.json()
  return result.data
}

export async function saveAIConfig(config: AIConfig): Promise<void> {
  const response = await fetchWithAuth('/api/v1/config/ai', {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(config),
  })

  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    throw new Error(result.error || result.message || `HTTP ${response.status}`)
  }
}

export async function testAIConfig(config: AIConfig): Promise<{ message?: string; error?: string }> {
  const response = await fetchWithAuth('/api/v1/config/ai/test', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(config),
  })

  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    throw new Error(result.error || result.message || `HTTP ${response.status}`)
  }

  return result
}
