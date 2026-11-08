import { fetchWithAuth } from '@/lib/api'

export interface AssetReferenceSelection {
  taskId: string
  version: number
  selection: 'all' | 'filtered' | 'selected'
  kind?: string
  query?: string
  assetIds?: string[]
  name: string
  count: number
}
export interface AssetReferenceSnapshot {
  readMode?: 'task' | 'snapshot'
  sources?: Array<{ taskId: string; version: number; scanType: string }>
  id: string
  taskId: string
  version: number
  name: string
  scanType: string
  status: string
  selection: string
  kind: string
  query: string
  count: number
  capturedAt: string
}
export interface ReferenceTask {
  id: string
  version: number
  name: string
  scanType: string
  targets: string
  status: string
  updatedAt: string
}
export interface ReferenceAsset {
  id: string
  taskId: string
  version: number
  kind: string
  sourceId: string
  value: string
  detail: string
}
async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetchWithAuth(`/api/v1/ai/${path}`, { signal })
  const body = await response.json().catch(() => { throw new Error('资产服务暂不可用，请稍后重试 / Asset service is unavailable') })
  if (!response.ok || !body.success) throw new Error(body.error || 'Unable to load assets')
  return body.data as T
}
export function listReferenceTasks(query: string, page: number, signal?: AbortSignal) {
  return get<{ tasks: ReferenceTask[]; total: number }>(`asset-reference-tasks?${new URLSearchParams({ query, page: String(page) })}`, signal)
}
export function listReferenceAssets(task: ReferenceTask, query: string, kind: string, page: number, signal?: AbortSignal) {
  return get<{ task: ReferenceTask; assets: ReferenceAsset[]; total: number }>(`asset-reference-tasks/${encodeURIComponent(task.id)}/assets?${new URLSearchParams({ version: String(task.version), query, kind, page: String(page) })}`, signal)
}
