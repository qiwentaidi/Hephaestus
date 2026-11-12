import { fetchWithAuth } from '@/lib/api'

export type DataPurgeErrorCode =
  | 'invalid_confirmation'
  | 'purge_in_progress'
  | 'active_tasks'
  | 'purge_failed'
  | 'request_failed'

export interface DataPurgeActiveTask {
  kind: string
  count: number
}

export interface DataPurgeResult {
  clearedTables: number
  clearedOrchestrationTables: number
  deletedDirectories: number
  logsCleared: boolean
  warnings: string[]
}

export class DataPurgeError extends Error {
  readonly code: DataPurgeErrorCode
  readonly activeTasks: DataPurgeActiveTask[]

  constructor(code: DataPurgeErrorCode, message: string, activeTasks: DataPurgeActiveTask[] = []) {
    super(message)
    this.name = 'DataPurgeError'
    this.code = code
    this.activeTasks = activeTasks
  }
}

/**
 * Clears every business-data row and generated artifact. The confirmation
 * phrase is validated by the backend as well, so the API cannot be used as a
 * one-click bypass.
 */
export async function purgeData(confirm: string, includeLogs: boolean): Promise<DataPurgeResult> {
  let response: Response
  try {
    response = await fetchWithAuth('/api/v1/system/data-purge', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm, includeLogs }),
    })
  } catch (error) {
    throw new DataPurgeError(
      'request_failed',
      error instanceof Error ? error.message : 'request_failed',
    )
  }

  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    const code = typeof result.code === 'string' ? result.code : 'purge_failed'
    const activeTasks = Array.isArray(result?.data?.activeTasks)
      ? (result.data.activeTasks as DataPurgeActiveTask[])
      : []
    throw new DataPurgeError(
      code as DataPurgeErrorCode,
      typeof result.error === 'string' ? result.error : 'purge_failed',
      activeTasks,
    )
  }
  const data = (result.data || {}) as Partial<DataPurgeResult>
  return {
    clearedTables: Number(data.clearedTables) || 0,
    clearedOrchestrationTables: Number(data.clearedOrchestrationTables) || 0,
    deletedDirectories: Number(data.deletedDirectories) || 0,
    logsCleared: data.logsCleared === true,
    warnings: Array.isArray(data.warnings) ? data.warnings : [],
  }
}
