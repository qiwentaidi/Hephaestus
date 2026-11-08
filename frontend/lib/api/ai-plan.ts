import { fetchWithAuth } from '@/lib/api'

export const ORCHESTRATION_RUN_PREFIX = 'orchestration:'
export const LEGACY_ORCHESTRATION_RUN_PREFIX = 'artex:'

export function isOrchestrationRunId(id?: string | null): boolean {
  return Boolean(id && (id.startsWith(ORCHESTRATION_RUN_PREFIX) || id.startsWith(LEGACY_ORCHESTRATION_RUN_PREFIX)))
}

export interface AIPlanRun {
  engine?: 'legacy' | 'orchestration'
  readOnly?: boolean
  id: string
  projectId: string
  plannerConversationId: string
  /** Human-facing 主控 conversation for this run; empty for runs created before Phase 2. */
  controllerConversationId?: string
  objective: string
  scopeJson?: string
  status: 'planning' | 'waiting' | 'pausing' | 'paused' | 'blocked' | 'completed' | 'failed' | 'cancelled' | string
  plannerProviderId?: string
  plannerModel?: string
  workerProviderId?: string
  workerModel?: string
  maxWorkers: number
  /** Optional hard cap; 0 means the planner may delegate unlimited workers. */
  maxTotalWorkers: number
  /** Consecutive idle planner rounds; exceeding the reminder budget blocks the run. */
  idleRounds?: number
  wakePending?: boolean
  summary?: string
  error?: string
  createdAt: string
  updatedAt: string
  completedAt?: string | null
}

export interface AIPlanItem {
  id: string
  runId: string
  seq: number
  batchNo: number
  title: string
  instruction: string
  target: string
  scopeJson?: string
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled' | 'blocked' | string
  childConversationId?: string
  resultSummary?: string
  error?: string
  createdAt: string
  startedAt?: string | null
  completedAt?: string | null
}

export interface AIPlanAgentRole {
  conversationId: string
  role: 'controller' | 'planner' | 'worker' | string
  parentConversationId?: string
  planRunId?: string
}

export interface AIPlanConversation {
  id: string
  projectId: string
  title: string
  target: string
  mode: string
  status: string
}

export interface AIPlanDirective {
  id: string
  runId: string
  kind: 'hint' | 'goal' | 'constraint' | string
  content: string
  status: string
  createdBy: string
  createdAt: string
}

export interface AIPlanRunDetail {
  run: AIPlanRun
  items: AIPlanItem[]
  plannerConversation: AIPlanConversation
  controllerConversation?: AIPlanConversation
  directives?: AIPlanDirective[]
  project: { id: string; name: string; description?: string; status?: string }
  agentRoles: AIPlanAgentRole[]
}

export interface AIPlanRunListItem {
  run: AIPlanRun
  project: { id: string; name: string; description?: string; status?: string }
  items?: AIPlanItem[]
}

export interface AIPlanRunSummary extends AIPlanRunListItem {
  itemCounts: Record<string, number>
  itemTotal: number
  planItemMatch: boolean
}

export type AIPlanItemRow = Pick<AIPlanItem, 'id' | 'seq' | 'title' | 'target' | 'instruction' | 'status' | 'completedAt'>

export interface AIPlanItemRows {
  items: AIPlanItemRow[]
  summary?: string
  error?: string
}

export interface CreateAIPlanRunRequest {
  projectId?: string
  objective: string
  targets: string[]
  projectName?: string
  scope?: string[]
  plannerProviderId?: string
  plannerModel?: string
  workerProviderId?: string
  workerModel?: string
  maxWorkers?: number
  maxTotalWorkers?: number
}

async function parse<T>(response: Response): Promise<T> {
  const body = await response.json().catch(() => null)
  if (!response.ok || !body?.success) {
    throw new Error(body?.error || `AI plan request failed (HTTP ${response.status})`)
  }
  return body.data as T
}

export async function createAIPlanRun(payload: CreateAIPlanRunRequest): Promise<AIPlanRunDetail> {
  const response = await fetchWithAuth('/api/v1/ai/plan-runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parse<AIPlanRunDetail>(response)
}

export async function listAIPlanRuns(params: { projectId?: string } = {}): Promise<{ items: AIPlanRunListItem[] }> {
  const query = params.projectId ? `?projectId=${encodeURIComponent(params.projectId)}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs${query}`, { cache: 'no-store' })
  return parse<{ items: AIPlanRunListItem[] }>(response)
}

export async function listAIPlanRunSummaries(search = '', signal?: AbortSignal): Promise<AIPlanRunSummary[]> {
  const params = new URLSearchParams({ view: 'summary' })
  if (search) params.set('search', search)
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs?${params}`, { cache: 'no-store', signal })
  const data = await parse<{ items: (Omit<AIPlanRunSummary, 'project'> & { projectName: string })[] }>(response)
  return data.items.map(({ projectName, ...item }) => ({
    ...item, project: { id: item.run.projectId, name: projectName },
  }))
}

export async function listAIPlanItemRows(runId: string, search: string, signal: AbortSignal): Promise<AIPlanItemRows> {
  const query = search ? `?search=${encodeURIComponent(search)}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(runId)}/items${query}`, { cache: 'no-store', signal })
  return parse<AIPlanItemRows>(response)
}

export async function getAIPlanItem(runId: string, itemId: string, signal: AbortSignal): Promise<AIPlanItem> {
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(runId)}/items/${encodeURIComponent(itemId)}`, { cache: 'no-store', signal })
  return parse<AIPlanItem>(response)
}

export async function getAIPlanRun(id: string): Promise<AIPlanRunDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(id)}`, { cache: 'no-store' })
  return parse<AIPlanRunDetail>(response)
}

/**
 * Attaches the per-run 主控 conversation if it does not exist yet (runs created
 * before Phase 2) and returns the run snapshot.
 */
export async function ensureAIPlanRunController(id: string): Promise<AIPlanRunDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(id)}/controller`, {
    method: 'POST',
  })
  return parse<AIPlanRunDetail>(response)
}

export async function cancelAIPlanRun(id: string): Promise<AIPlanRunDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(id)}/cancel`, {
    method: 'POST',
  })
  return parse<AIPlanRunDetail>(response)
}

export async function pauseAIPlanRun(id: string): Promise<AIPlanRunDetail & { stopPending?: boolean }> {
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(id)}/pause`, { method: 'POST' })
  return parse<AIPlanRunDetail & { stopPending?: boolean }>(response)
}

export async function resumeAIPlanRun(id: string): Promise<{ id: string; status: string; projectionPending?: boolean }> {
  const response = await fetchWithAuth(`/api/v1/ai/plan-runs/${encodeURIComponent(id)}/resume`, { method: 'POST' })
  return parse<{ id: string; status: string; projectionPending?: boolean }>(response)
}
