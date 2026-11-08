import toast from 'react-hot-toast'
import { fetchWithAuth } from '@/lib/api'

export interface RunningUpdateTaskGroup {
  key: string
  count: number
}

export interface RunningUpdateTaskSummary {
  total: number
  groups: RunningUpdateTaskGroup[]
  /** 是否成功从后端确认过任务状态；false 表示无法判断，重启前应要求人工确认。 */
  known: boolean
}

export const EMPTY_RUNNING_UPDATE_TASKS: RunningUpdateTaskSummary = {
  total: 0,
  groups: [],
  known: false,
}

/**
 * 查询重启会中断的运行中任务。查询失败时返回 known=false，
 * 调用方应视为“无法确认”并要求人工确认，而不是直接自动重启。
 */
export async function fetchRunningUpdateTasks(): Promise<RunningUpdateTaskSummary> {
  try {
    const response = await fetchWithAuth('/api/v1/system/update/running-tasks')
    if (!response.ok) {
      return { total: 0, groups: [], known: false }
    }
    const payload = await response.json()
    if (!payload?.success) {
      return { total: 0, groups: [], known: false }
    }

    const rawGroups = Array.isArray(payload.data?.groups) ? payload.data.groups : []
    const groups: RunningUpdateTaskGroup[] = rawGroups
      .map((item: any) => ({ key: String(item?.key ?? ''), count: Number(item?.count) || 0 }))
      .filter((item: RunningUpdateTaskGroup) => item.key !== '' && item.count > 0)

    const total = Number(payload.data?.total) || groups.reduce((sum, item) => sum + item.count, 0)
    return { total, groups, known: true }
  } catch {
    return { total: 0, groups: [], known: false }
  }
}

/** 调用后端重启接口，由后端完成二进制替换并拉起新进程。 */
export async function requestSystemUpdateRestart(): Promise<void> {
  const response = await fetchWithAuth('/api/v1/system/update/restart', { method: 'POST' })
  const payload = await response.json()
  if (!payload?.success) {
    throw new Error(payload?.error || '重启失败')
  }
}

/** 桌面端 macOS 安装已下载的 dmg 并重新拉起应用。 */
export async function installDesktopUpdatePackage(): Promise<void> {
  const response = await fetchWithAuth('/api/v1/desktop/update/install-latest', { method: 'POST' })
  const payload = await response.json()
  if (!payload?.success) {
    throw new Error(payload?.error || '打开安装器失败')
  }
}

/** 把运行中任务概况格式化为“Web 扫描 ×2、目录扫描 ×1”。 */
export function formatRunningUpdateTasks(
  groups: RunningUpdateTaskGroup[],
  labels: Record<string, string>,
): string {
  return groups
    .map((group) => `${labels[group.key] ?? group.key} ×${group.count}`)
    .join('、')
}

const STAGED_UPDATE_KEY = 'hephaestus:system-update-staged'
const STAGED_UPDATE_ACTION_KEY = 'hephaestus:system-update-staged-action'
const STAGED_UPDATE_EVENT = 'hephaestus:update-staged-changed'

export type StagedUpdateAction = 'restart' | 'install'

/** 读取已经下载并暂存的客户端更新版本，用于跨页面共享“已下载完成”状态。 */
export function readStagedUpdateVersion(): string | null {
  if (typeof window === 'undefined') return null
  try {
    return window.localStorage.getItem(STAGED_UPDATE_KEY)
  } catch {
    return null
  }
}

/** 读取暂存更新包对应的完成方式：dmg 安装或原地重启。 */
export function readStagedUpdateAction(): StagedUpdateAction | null {
  if (typeof window === 'undefined') return null
  try {
    const action = window.localStorage.getItem(STAGED_UPDATE_ACTION_KEY)
    return action === 'install' || action === 'restart' ? action : null
  } catch {
    return null
  }
}

/** 记录或清除已暂存的客户端更新版本，并通知同一页面的其他更新入口。 */
export function persistStagedUpdateVersion(version: string | null): void {
  if (typeof window === 'undefined') return
  try {
    if (version) {
      window.localStorage.setItem(STAGED_UPDATE_KEY, version)
    } else {
      window.localStorage.removeItem(STAGED_UPDATE_KEY)
      window.localStorage.removeItem(STAGED_UPDATE_ACTION_KEY)
    }
  } catch {
    // ignore storage failures; in-memory state still drives the current view
  }
  window.dispatchEvent(new CustomEvent(STAGED_UPDATE_EVENT, { detail: { version } }))
}

/** 记录暂存更新包的完成方式，重载后仍可正确执行 dmg 安装或重启。 */
export function persistStagedUpdateAction(action: StagedUpdateAction | null): void {
  if (typeof window === 'undefined') return
  try {
    if (action) {
      window.localStorage.setItem(STAGED_UPDATE_ACTION_KEY, action)
    } else {
      window.localStorage.removeItem(STAGED_UPDATE_ACTION_KEY)
    }
  } catch {
    // ignore storage failures
  }
}

/** 订阅暂存版本变化，返回取消订阅函数。 */
export function subscribeStagedUpdateVersion(
  listener: (version: string | null) => void,
): () => void {
  if (typeof window === 'undefined') return () => {}
  const handler = (event: Event) => {
    const detail = (event as CustomEvent<{ version?: string | null }>).detail
    listener(detail?.version ?? readStagedUpdateVersion())
  }
  window.addEventListener(STAGED_UPDATE_EVENT, handler)
  return () => window.removeEventListener(STAGED_UPDATE_EVENT, handler)
}

const normalizeVersion = (version: string) => {
  const trimmed = version.trim()
  return /^[vV]/.test(trimmed) ? trimmed.slice(1) : trimmed
}

/**
 * 读取后端当前运行的版本号；服务重启期间请求会失败，此时返回 null。
 * 前端产物随后端二进制一起分发，因此版本号也是“页面资源是否过期”的判断依据。
 */
export async function fetchServiceVersion(target: 'client' | 'desktop' = 'client'): Promise<string | null> {
  try {
    const response = await fetchWithAuth(`/api/v1/system/update/info?target=${target}`, {
      cache: 'no-store',
    })
    if (!response.ok) return null
    const payload = await response.json().catch(() => null)
    if (!payload?.success) return null
    const version = payload?.data?.version
    return typeof version === 'string' && version.trim() !== '' ? version.trim() : null
  } catch {
    return null
  }
}

export type ServiceRestartWaitResult = 'ready' | 'timeout' | 'aborted'

/**
 * 等待重启后的新进程接管服务。
 *
 * 重启请求返回时旧进程仍在对外服务，必须等到接口重新可达且版本已是目标版本，
 * 否则提前刷新只会重新加载旧二进制里的旧前端资源。
 * 目标版本未知时退化为“先观察到断连、再恢复正常”，避免把旧进程的响应当成重启完成。
 */
export async function waitForRestartedService(options: {
  expectedVersion?: string
  timeoutMs?: number
  intervalMs?: number
  signal?: AbortSignal
} = {}): Promise<ServiceRestartWaitResult> {
  const { expectedVersion, timeoutMs = 120_000, intervalMs = 1500, signal } = options
  const expected = expectedVersion ? normalizeVersion(expectedVersion) : ''
  const deadline = Date.now() + timeoutMs
  let sawDowntime = false

  while (Date.now() < deadline) {
    if (signal?.aborted) return 'aborted'
    const version = await fetchServiceVersion()
    if (!version) {
      sawDowntime = true
    } else if (expected ? normalizeVersion(version) === expected : sawDowntime) {
      return 'ready'
    }
    await new Promise((resolve) => setTimeout(resolve, intervalMs))
  }

  return 'timeout'
}

/**
 * 重启后等待新版本就绪，然后整页刷新以加载新二进制内置的前端资源。
 * 返回 false 表示超时未就绪，调用方需要提示用户手动刷新。
 */
export async function reloadAfterServiceRestart(options: {
  expectedVersion?: string
  waitingMessage: string
  readyMessage: string
  timeoutMessage: string
  toastId?: string
  timeoutMs?: number
}): Promise<boolean> {
  const id = options.toastId ?? toast.loading(options.waitingMessage)
  toast.loading(options.waitingMessage, { id })

  const result = await waitForRestartedService({
    expectedVersion: options.expectedVersion,
    timeoutMs: options.timeoutMs,
  })

  if (result !== 'ready') {
    toast.error(options.timeoutMessage, { id })
    return false
  }

  toast.success(options.readyMessage, { id })
  // 新版本已在运行，暂存的更新包标记不再有意义。
  persistStagedUpdateVersion(null)
  // 留出一点时间让提示渲染完，再整页刷新。
  await new Promise((resolve) => setTimeout(resolve, 800))
  window.location.reload()
  return true
}

