import { fetchWithAuth } from '@/lib/api'

export type AIWebSearchBackend = 'ddgs' | 'brave-free' | 'tavily' | 'deepseek'

export interface AIWebSearchSettings {
  enabled: boolean
  backend: AIWebSearchBackend
  proxy: string
  braveKeySet: boolean
  tavilyKeySet: boolean
}

export interface AIWebSearchResult {
  title: string
  url: string
  description: string
  position: number
}

export interface AIWebSearchUpdate {
  enabled: boolean
  backend: AIWebSearchBackend
  proxy: string
  braveApiKey?: string
  tavilyApiKey?: string
}

export interface AIWebSearchTest {
  backend: AIWebSearchBackend
  proxy: string
  braveApiKey?: string
  tavilyApiKey?: string
}

async function readPayload<T>(response: Response): Promise<T> {
  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    throw new Error(result.error || result.message || `HTTP ${response.status}`)
  }
  return result.data as T
}

export async function getAIWebSearchSettings(): Promise<AIWebSearchSettings> {
  return readPayload<AIWebSearchSettings>(await fetchWithAuth('/api/v1/config/ai/web-search'))
}

export async function saveAIWebSearchSettings(input: AIWebSearchUpdate): Promise<AIWebSearchSettings> {
  return readPayload<AIWebSearchSettings>(await fetchWithAuth('/api/v1/config/ai/web-search', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  }))
}

export async function testAIWebSearch(input: AIWebSearchTest): Promise<AIWebSearchResult[]> {
  return readPayload<AIWebSearchResult[]>(await fetchWithAuth('/api/v1/config/ai/web-search/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  }))
}
