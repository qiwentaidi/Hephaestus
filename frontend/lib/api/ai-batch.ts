import { fetchWithAuth } from '@/lib/api'

export type BatchStatus = 'pending' | 'running' | 'pausing' | 'paused' | 'completed' | 'completed_with_errors' | 'failed'
export interface AIBatchItem {
  id: string; queueId: string; position: number; instruction: string; conversationId: string
  previousConversations?: string[]; generation?: number
  status: BatchStatus; attempts: number; error: string; result: string; startedAt?: string; completedAt?: string
}
export interface AIBatchQueue {
  id: string; projectId: string; name: string; providerId: string; model: string; concurrency: number
  status: BatchStatus; pauseRequested: boolean; error: string; createdAt: string; updatedAt?: string; items: AIBatchItem[]
}
export interface CreateAIBatchQueue {
  name: string; projectId?: string; providerId: string; model: string; concurrency: number; instructions: string[]; executeNow: boolean
}
async function request<T>(path = '', method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetchWithAuth(`/api/v1/ai/batch-queues${path}`, {
    method, signal, cache: 'no-store', ...(body === undefined ? {} : { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }),
  })
  const payload = await response.json().catch(() => null)
  if (!response.ok || !payload?.success) throw new Error(payload?.error || `Task queue API: HTTP ${response.status}`)
  return payload.data as T
}
export const listAIBatchQueues = (signal?: AbortSignal) => request<AIBatchQueue[]>('', 'GET', undefined, signal)
export const getAIBatchQueue = (id: string) => request<AIBatchQueue>(`/${encodeURIComponent(id)}`)
export const createAIBatchQueue = (body: CreateAIBatchQueue) => request<AIBatchQueue>('', 'POST', body)
export const actionAIBatchQueues = (ids: string[], action: 'pause' | 'resume' | 'retry_failed') => request<Record<string, string>>('/actions', 'POST', { ids, action })
export const retryAIBatchItem = (id: string, itemId: string) => request<boolean>(`/${encodeURIComponent(id)}/items/${encodeURIComponent(itemId)}/retry`, 'POST')
export const deleteAIBatchQueue = (id: string) => request<boolean>(`/${encodeURIComponent(id)}`, 'DELETE')
