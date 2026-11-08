import { fetchWithAuth } from '@/lib/api'
import { parseFilenameFromContentDisposition } from '@/lib/download'

export type UnifiedAssetStatus = 'active' | 'stale' | 'retired'

export interface UnifiedAsset {
  id: string
  scopeKey: string
  type: string
  canonicalValue: string
  displayValue: string
  name?: string
  notes?: string
  tags?: string[]
  groupKey?: string
  verification?: 'declared' | 'observed' | 'verified' | string
  status: UnifiedAssetStatus | string
  confidence: string
  archivedAt?: string | null
  metadata?: Record<string, unknown>
  firstSeenAt: string
  lastSeenAt: string
  createdAt: string
  updatedAt: string
}

export interface UnifiedAssetObservation {
  id: string
  assetId: string
  scopeKey: string
  type: string
  canonicalValue: string
  source: string
  sourceId: string
  sourceVersion: number
  taskId?: string
  taskVersion?: number
  importBatchId?: string
  origin?: string
  observationKey: string
  confidence: string
  observedAt: string
  expiresAt?: string | null
  evidenceRef?: string
  attributes?: Record<string, unknown>
  createdAt: string
}

export interface UnifiedAssetRelation {
  id: string
  scopeKey: string
  fromAssetId: string
  toAssetId: string
  relationType: string
  source: string
  sourceId: string
  sourceVersion: number
  active: boolean
  firstSeenAt: string
  lastSeenAt: string
  metadata?: Record<string, unknown>
}

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetchWithAuth(`/api/v1/assets/catalog${path}`, { signal })
  const body = await response.json().catch(() => null)
  if (!response.ok || !body?.success) {
    throw new Error(body?.error || `Asset catalog request failed (HTTP ${response.status})`)
  }
  return body.data as T
}

export interface UnifiedAssetSourceTask {
  id: string
  version: number
  name: string
  scanType: string
  status: string
  missing?: boolean
}

export interface AssetLedgerItem extends UnifiedAsset {
  sources: string[]
  observationCount: number
  taskCount: number
  lastTaskId?: string
  lastTaskName?: string
  lastObservedAt?: string
  archived: boolean
}

export interface AssetLedgerSummary {
  total: number
  domains: number
  ips: number
  urls: number
  declared: number
  observed: number
  verified: number
  manual: number
  imported: number
  scanned: number
  archived: number
}

export interface AssetLedgerResponse {
  items: AssetLedgerItem[]
  total: number
  page: number
  pageSize: number
  summary: AssetLedgerSummary
}

export type AssetLedgerEntityCategory = 'root' | 'ip' | 'subdomain' | 'port' | 'website'

export interface AssetLedgerEntity {
  id: string
  category: string
  value: string
  displayValue: string
  name?: string
  rootDomain?: string
  host?: string
  ips?: string[]
  domains?: string[]
  port?: number
  protocol?: string
  transport?: string
  service?: string
  title?: string
  httpStatus?: number
  fingerprints?: string[]
  highRiskFingerprints?: string[]
  subdomainCount?: number
  ipCount?: number
  portCount?: number
  websiteCount?: number
  assetId?: string
  sources: string[]
  sourceTaskIds: string[]
  lastTaskId?: string
  lastTaskName?: string
  firstSeenAt: string
  lastSeenAt: string
  status: string
  verification: string
  archived?: boolean
}

export interface AssetLedgerEntityCounts {
  root: number
  ip: number
  subdomain: number
  port: number
  website: number
}

export interface AssetLedgerEntityResponse {
  items: AssetLedgerEntity[]
  total: number
  page: number
  pageSize: number
  counts: AssetLedgerEntityCounts
  /** Row counts per category after the active filters are applied. */
  filteredCounts?: AssetLedgerEntityCounts
}

export interface AssetImportError {
  row: number
  value?: string
  reason: string
}

export interface AssetImportPreview {
  total: number
  valid: number
  invalid: number
  duplicates: number
  byType: Record<string, number>
  errors: AssetImportError[]
  preview: Array<{ type: string; value: string; original: string }>
  batchId?: string
  created?: number
  updated?: number
}

export interface AssetImportRequest {
  content?: string
  file?: File
  groupKey?: string
  dryRun?: boolean
}

export function getUnifiedAssetDetail(id: string, signal?: AbortSignal) {
  return get<{
    asset: UnifiedAsset
    observations: UnifiedAssetObservation[]
    sourceTasks?: UnifiedAssetSourceTask[]
    sources?: string[]
  }>(`/${encodeURIComponent(id)}`, signal)
}

async function mutate<T>(path: string, method: 'POST' | 'PATCH', body: unknown): Promise<T> {
  const response = await fetchWithAuth(`/api/v1/assets${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const payload = await response.json().catch(() => null)
  if (!response.ok || !payload?.success) {
    throw new Error(payload?.error || `Asset request failed (HTTP ${response.status})`)
  }
  return payload.data as T
}

export function listAssetLedger(params: URLSearchParams, signal?: AbortSignal) {
  const response = fetchWithAuth(`/api/v1/assets/ledger?${params.toString()}`, { signal })
  return response.then(async (result) => {
    const body = await result.json().catch(() => null)
    if (!result.ok || !body?.success) {
      throw new Error(body?.error || `Asset ledger request failed (HTTP ${result.status})`)
    }
    return body.data as AssetLedgerResponse
  })
}

export function listAssetLedgerEntities(params: URLSearchParams, signal?: AbortSignal) {
  const response = fetchWithAuth(`/api/v1/assets/ledger/entities?${params.toString()}`, { signal })
  return response.then(async (result) => {
    const body = await result.json().catch(() => null)
    if (!result.ok || !body?.success) {
      throw new Error(body?.error || `Asset ledger request failed (HTTP ${result.status})`)
    }
    return body.data as AssetLedgerEntityResponse
  })
}

export function createAssetLedger(payload: {
  type?: string
  value: string
  name?: string
  notes?: string
  tags?: string[]
  groupKey?: string
}) {
  return mutate<UnifiedAsset>('/ledger/assets', 'POST', payload)
}

export function updateAssetLedger(id: string, payload: {
  website?: boolean
  name?: string
  notes?: string
  tags?: string[]
  groupKey?: string
  archived?: boolean
}) {
  return mutate<UnifiedAsset>(`/ledger/assets/${encodeURIComponent(id)}`, 'PATCH', payload)
}

export async function importAssetLedger(payload: AssetImportRequest): Promise<AssetImportPreview> {
  let response: Response
  if (payload.file) {
    const form = new FormData()
    form.append('file', payload.file)
    if (payload.groupKey) form.append('groupKey', payload.groupKey)
    form.append('dryRun', payload.dryRun ? 'true' : 'false')
    response = await fetchWithAuth('/api/v1/assets/ledger/import', { method: 'POST', body: form })
  } else {
    response = await fetchWithAuth('/api/v1/assets/ledger/import', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ content: payload.content || '', groupKey: payload.groupKey || '', dryRun: Boolean(payload.dryRun) }),
    })
  }
  const body = await response.json().catch(() => null)
  if (!response.ok || !body?.success) {
    throw new Error(body?.error || `Asset import failed (HTTP ${response.status})`)
  }
  return body.data as AssetImportPreview
}

export function listUnifiedAssetRelations(id: string, signal?: AbortSignal) {
  return get<{ items: UnifiedAssetRelation[]; total: number }>(`/${encodeURIComponent(id)}/relations`, signal)
}

export interface AssetLedgerEntityRef {
  id: string
  category: string
  assetId?: string
  value?: string
}

export interface AssetLedgerBatchArchiveResult {
  action: string
  updated: number
  skipped: number
  failed: Array<{ id: string; error: string }>
}

export function batchArchiveAssetLedger(payload: {
  action: 'archive' | 'restore'
  items: AssetLedgerEntityRef[]
}) {
  return mutate<AssetLedgerBatchArchiveResult>('/ledger/entities/batch-archive', 'POST', payload)
}

export type AssetLedgerExportRequest =
  | { params: URLSearchParams; lang: string }
  | { items: AssetLedgerEntityRef[]; lang: string }

export async function exportAssetLedgerEntities(
  payload: AssetLedgerExportRequest,
): Promise<{ blob: Blob; filename: string }> {
  let response: Response
  if ('params' in payload) {
    const params = new URLSearchParams(payload.params)
    params.set('lang', payload.lang)
    response = await fetchWithAuth(`/api/v1/assets/ledger/entities/export?${params.toString()}`)
  } else {
    response = await fetchWithAuth('/api/v1/assets/ledger/entities/export', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ lang: payload.lang, items: payload.items }),
    })
  }
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.error || `Asset export failed (HTTP ${response.status})`)
  }
  const blob = await response.blob()
  const filename = parseFilenameFromContentDisposition(response.headers.get('Content-Disposition'))
    || `asset-ledger-${Date.now()}.csv`
  return { blob, filename }
}

