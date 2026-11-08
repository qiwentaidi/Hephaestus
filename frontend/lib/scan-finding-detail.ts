import type { CenterFinding } from './api/vulnerability-center'

export type ScanFindingReference = Pick<CenterFinding, 'source' | 'taskId' | 'version'> & Partial<Pick<CenterFinding, 'nativeId' | 'templateId' | 'protocol' | 'url' | 'name' | 'taskName' | 'createdAt'>>

export function scanFindingDetailHref(finding: ScanFindingReference, from = '/vulnerabilities') {
  const query = new URLSearchParams({ source: finding.source, taskId: finding.taskId, version: String(finding.version), from })
  for (const key of ['nativeId', 'templateId', 'protocol', 'url', 'name', 'taskName', 'createdAt'] as const) {
    if (finding[key]) query.set(key, finding[key]!)
  }
  return `/vulnerabilities/detail?${query}`
}

// Missing scan versions must never silently fall back to the latest run.
export function parseScanFindingReference(params: Pick<URLSearchParams, 'get'>): CenterFinding | null {
  const source = params.get('source')
  const taskId = params.get('taskId') || ''
  const rawVersion = params.get('version') || ''
  const version = Number(rawVersion)
  if ((source !== 'web' && source !== 'js') || !taskId || !/^\d+$/.test(rawVersion) || !Number.isSafeInteger(version) || version < 1) return null
  const nativeId = params.get('nativeId') || ''
  if (source === 'js' && !nativeId) return null
  if (source === 'web' && !nativeId && !params.get('url')) return null
  return {
    source, taskId, version, nativeId, id: `${source}:${nativeId}`, name: params.get('name') || '',
    url: params.get('url') || '', templateId: params.get('templateId') || '', protocol: params.get('protocol') || '',
    taskName: params.get('taskName') || taskId, createdAt: params.get('createdAt') || '',
    severity: 'info', status: 'open', vulnerabilityType: '', projectId: '', projectName: '', verificationStatus: '', needsReview: false,
  }
}
