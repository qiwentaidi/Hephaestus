'use client'

import { type ReactNode, useState, useEffect, useRef, useCallback } from 'react'
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  Download,
  FileCode,
  HardDriveDownload,
  Package,
  RefreshCw,
} from 'lucide-react'
import DOMPurify from 'dompurify'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { fetchWithAuth, getApiBaseURL, getToken } from '@/lib/api'
import { isDesktopRuntime } from '@/lib/desktop'
import {
  downloadDockerUpdate,
  fetchRunningUpdateTasks,
  formatRunningUpdateTasks,
  installDesktopUpdatePackage,
  persistStagedUpdateAction,
  persistStagedUpdateVersion,
  readStagedUpdateAction,
  readStagedUpdateVersion,
  reloadAfterServiceRestart,
  requestSystemUpdateRestart,
  subscribeStagedUpdateVersion,
  syncStagedDockerUpdate,
  SYSTEM_UPDATE_SNAPSHOT_KEY,
  type RunningUpdateTaskSummary,
} from '@/lib/system-update'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from 'react-hot-toast'

interface SystemInfo {
  version: string
  deployment?: string
  selfUpdateDisabled?: boolean
}

interface UpdateInfo {
  delivery?: 'docker'
  version: string
  releaseDate: string
  changelog: string
  downloadUrl: string
  mirrorUrl: string
}

interface SystemUpdateSnapshot {
  currentVersion?: string
  hasUpdate: boolean
  noUpdate?: boolean
  updateInfo?: UpdateInfo | null
  checkedAt?: number
}

interface UpdateHistoryItem {
  version: string
  releaseDate: string
  changelog: string
}

type UpdateStatus =
  | 'idle'
  | 'checking'
  | 'available'
  | 'downloading'
  | 'installing'
  | 'up-to-date'
  | 'error'

const getCurrentTimestamp = () => Date.now()

interface UpdateCardProps {
  id: 'client' | 'config'
  docker?: boolean
  name: string
  currentVersion: string
  latestVersion: string
  releaseDate: string
  changelog: string[]
  changelogMarkdown: string
  hasUpdate: boolean
  isUpToDate: boolean
  actionLabel: string
  actionIcon: ReactNode
  icon: ReactNode
  iconWrap: string
  extra: ReactNode
  statusLabel: string
  actionDisabled: boolean
  isExpanded: boolean
  newTag: string
  upToDateLabel: string
  changelogTitle: string
  markdownClassName: string
  onAction: () => void | Promise<void>
  onToggle: (id: 'client' | 'config') => void
  renderMarkdownHTML: (markdown: string) => string
}

function UpdateCard({
  id,
  docker,
  name,
  currentVersion,
  latestVersion,
  releaseDate,
  changelog,
  changelogMarkdown,
  hasUpdate,
  isUpToDate,
  actionLabel,
  actionIcon,
  icon,
  iconWrap,
  extra,
  statusLabel,
  actionDisabled,
  isExpanded,
  newTag,
  upToDateLabel,
  changelogTitle,
  markdownClassName,
  onAction,
  onToggle,
  renderMarkdownHTML,
}: UpdateCardProps) {
  return (
    <div
      className={cn(
        'overflow-hidden rounded-xl border border-border bg-card transition-all',
        hasUpdate && 'ring-1 ring-primary/20'
      )}
    >
      <div className="p-4">
        <div className="flex flex-wrap items-start gap-3 sm:flex-nowrap sm:gap-4">
          <div
            className={cn(
              'flex size-10 shrink-0 items-center justify-center rounded-lg',
              iconWrap
            )}
          >
            {icon}
          </div>

          <div className="min-w-0 flex-1">
            <div className="mb-1 flex flex-wrap items-center gap-2">
              <h3 className="font-medium text-foreground">{name}</h3>
              {hasUpdate && (
                <span className="inline-flex items-center gap-1 rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-primary">
                  {newTag}
                </span>
              )}
              {isUpToDate && <CheckCircle2 className="size-4 text-primary" />}
            </div>

            {hasUpdate ? (
              <div className="flex flex-wrap items-center gap-2 text-sm">
                {docker && <Badge variant="secondary">Docker</Badge>}
                <span className="font-mono text-muted-foreground">{currentVersion}</span>
                <span className="text-muted-foreground">→</span>
                <span className="font-mono text-primary">{latestVersion}</span>
                {releaseDate && (
                  <>
                    <span className="text-muted-foreground">·</span>
                    <span className="text-muted-foreground">{releaseDate}</span>
                  </>
                )}
                {id === 'config' && !releaseDate && (
                  <>
                    <span className="text-muted-foreground">·</span>
                    <span className="text-muted-foreground">~/config</span>
                  </>
                )}
              </div>
            ) : (
              <div className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                {docker && <Badge variant="secondary">Docker</Badge>}
                <span>{currentVersion} · {isUpToDate ? upToDateLabel : statusLabel}</span>
              </div>
            )}

            {extra}
          </div>

          <div className="flex w-full shrink-0 flex-wrap items-center justify-end gap-2 sm:w-auto">
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                void onAction()
              }}
              disabled={actionDisabled}
              className="gap-1.5 border-border bg-background hover:bg-muted"
            >
              {actionIcon}
              {actionLabel}
            </Button>
            {(hasUpdate || changelog.length > 0) && (
              <Button
                size="sm"
                variant="ghost"
                onClick={() => onToggle(id)}
                className="size-8 p-0"
              >
                {isExpanded ? (
                  <ChevronUp className="size-4 text-muted-foreground" />
                ) : (
                  <ChevronDown className="size-4 text-muted-foreground" />
                )}
              </Button>
            )}
          </div>
        </div>
      </div>

      {isExpanded && changelogMarkdown && (
        <div className="px-4 pb-4 pt-1">
          <div className="ml-14 pt-2">
            <p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
              {changelogTitle}
            </p>
            <div
              className={cn(markdownClassName, 'text-sm text-muted-foreground')}
              dangerouslySetInnerHTML={{
                __html: renderMarkdownHTML(changelogMarkdown),
              }}
            />
          </div>
        </div>
      )}
    </div>
  )
}

export function SystemUpdateTab() {
  const { t } = useI18n()
  const i18n = t.settings.systemUpdate
  const CONFIG_UPDATE_SNAPSHOT_KEY = 'hephaestus:config-update-snapshot'
  const [systemInfo, setSystemInfo] = useState<SystemInfo | null>(null)
  const [updateInfo, setUpdateInfo] = useState<UpdateInfo | null>(null)
  const [hasUpdate, setHasUpdate] = useState(false)
  const [noUpdate, setNoUpdate] = useState(false)
  const [checking, setChecking] = useState(false)
  const [downloading, setDownloading] = useState(false)
  const [downloadProgress, setDownloadProgress] = useState(0)
  const [downloadedSize, setDownloadedSize] = useState(0)
  const [totalSize, setTotalSize] = useState(0)
  const [status, setStatus] = useState<UpdateStatus>('idle')
  const [restartDialogOpen, setRestartDialogOpen] = useState(false)
  const [dockerRestarting, setDockerRestarting] = useState(false)
  const [pendingRestartAction, setPendingRestartAction] = useState<'restart' | 'install'>(
    () => readStagedUpdateAction() ?? 'restart'
  )
  const [clientPackageReadyVersion, setClientPackageReadyVersion] = useState<string | null>(
    () => readStagedUpdateVersion()
  )
  const [runningTasks, setRunningTasks] = useState<RunningUpdateTaskSummary | null>(null)
  const downloadStartTimeRef = useRef(0)
  const [estimatedTimeLeft, setEstimatedTimeLeft] = useState(0)
  const [configInfo, setConfigInfo] = useState<SystemInfo | null>(null)
  const [configUpdateInfo, setConfigUpdateInfo] = useState<UpdateInfo | null>(null)
  const [configHasUpdate, setConfigHasUpdate] = useState(false)
  const [configNoUpdate, setConfigNoUpdate] = useState(false)
  const [configChecking, setConfigChecking] = useState(false)
  const [configDownloading, setConfigDownloading] = useState(false)
  const [configDownloadProgress, setConfigDownloadProgress] = useState(0)
  const [configDownloadedSize, setConfigDownloadedSize] = useState(0)
  const [configTotalSize, setConfigTotalSize] = useState(0)
  const [configStatus, setConfigStatus] = useState<UpdateStatus>('idle')
  const configDownloadStartTimeRef = useRef(0)
  const [configEstimatedTimeLeft, setConfigEstimatedTimeLeft] = useState(0)
  const [expandedItems, setExpandedItems] = useState<string[]>(['client', 'config'])
  const [clientUpdateHistory, setClientUpdateHistory] = useState<UpdateHistoryItem[]>([])

  const ensureVersionPrefix = (version: string) => {
    return version.startsWith('v') ? version : `v${version}`
  }

  const isDMGDownloadURL = (value: string) => {
    const url = value.trim()
    if (!url) return false

    try {
      return new URL(url).pathname.toLowerCase().endsWith('.dmg')
    } catch {
      return /\.dmg(?:$|[?#])/i.test(url)
    }
  }

  const applySnapshot = useCallback((snapshot: SystemUpdateSnapshot) => {
    const { currentVersion } = snapshot
    if (currentVersion) {
      setSystemInfo((previous) => ({ ...previous, version: currentVersion }))
    }

    setHasUpdate(Boolean(snapshot.hasUpdate))
    setNoUpdate(Boolean(snapshot.noUpdate))
    setUpdateInfo(snapshot.updateInfo || null)

    if (snapshot.hasUpdate && snapshot.updateInfo) {
      setStatus((prev) =>
        prev === 'downloading' || prev === 'installing' ? prev : 'available'
      )
    } else if (snapshot.noUpdate) {
      setStatus((prev) =>
        prev === 'downloading' || prev === 'installing' ? prev : 'up-to-date'
      )
    }
  }, [])

  const loadCachedUpdateSnapshot = useCallback(() => {
    if (typeof window === 'undefined') return

    try {
      const raw = window.localStorage.getItem(SYSTEM_UPDATE_SNAPSHOT_KEY)
      if (!raw) return
      const snapshot = JSON.parse(raw) as SystemUpdateSnapshot
      applySnapshot(snapshot)
    } catch (error) {
      console.error('读取更新快照失败:', error)
    }
  }, [applySnapshot])

  const applyConfigSnapshot = useCallback((snapshot: SystemUpdateSnapshot) => {
    setConfigInfo({ version: snapshot.currentVersion || 'v0.0.0' })
    setConfigHasUpdate(Boolean(snapshot.hasUpdate))
    setConfigNoUpdate(Boolean(snapshot.noUpdate))
    setConfigUpdateInfo(snapshot.updateInfo || null)

    if (snapshot.hasUpdate && snapshot.updateInfo) {
      setConfigStatus((prev) =>
        prev === 'downloading' || prev === 'installing' ? prev : 'available'
      )
    } else if (snapshot.noUpdate) {
      setConfigStatus((prev) =>
        prev === 'downloading' || prev === 'installing' ? prev : 'up-to-date'
      )
    }
  }, [])

  const loadCachedConfigUpdateSnapshot = useCallback(() => {
    if (typeof window === 'undefined') return

    try {
      const raw = window.localStorage.getItem(CONFIG_UPDATE_SNAPSHOT_KEY)
      if (!raw) return
      const snapshot = JSON.parse(raw) as SystemUpdateSnapshot
      applyConfigSnapshot(snapshot)
    } catch (error) {
      console.error('读取配置更新快照失败:', error)
    }
  }, [applyConfigSnapshot])

  const persistConfigUpdateSnapshot = (snapshot: SystemUpdateSnapshot) => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(CONFIG_UPDATE_SNAPSHOT_KEY, JSON.stringify(snapshot))
    window.dispatchEvent(
      new CustomEvent<SystemUpdateSnapshot>('hephaestus:config-update-changed', {
        detail: snapshot,
      })
    )
  }

  const persistUpdateSnapshot = (snapshot: SystemUpdateSnapshot) => {
    if (typeof window === 'undefined') return
    window.localStorage.setItem(SYSTEM_UPDATE_SNAPSHOT_KEY, JSON.stringify(snapshot))
    window.dispatchEvent(
      new CustomEvent<SystemUpdateSnapshot>('hephaestus:system-update-changed', {
        detail: snapshot,
      })
    )
  }

  const fetchSystemInfo = useCallback(async () => {
    try {
      const target = (await isDesktopRuntime()) ? 'desktop' : 'client'
      const res = await fetchWithAuth(`/api/v1/system/update/info?target=${target}`)
      const data = await res.json()
      if (data.success) {
        setSystemInfo(data.data)
      }
    } catch (error) {
      console.error(i18n.messages.fetchSystemFailed + ':', error)
    }
  }, [i18n])

  useEffect(() => {
    if (updateInfo?.delivery !== 'docker' || updateInfo.version !== systemInfo?.version) return
    setHasUpdate(false)
    setUpdateInfo(null)
    setNoUpdate(true)
    setStatus('up-to-date')
  }, [systemInfo?.version, updateInfo])

  const fetchConfigInfo = useCallback(async () => {
    try {
      const res = await fetchWithAuth('/api/v1/system/update/config/info')
      const data = await res.json()
      if (data.success) {
        setConfigInfo({ version: data.data?.version || 'v0.0.0' })
      }
    } catch (error) {
      console.error(i18n.messages.fetchConfigFailed + ':', error)
      setConfigInfo({ version: 'v0.0.0' })
    }
  }, [i18n])

  const fetchClientUpdateHistory = useCallback(async () => {
    try {
      const target = (await isDesktopRuntime()) ? 'desktop' : 'client'
      const res = await fetchWithAuth(`/api/v1/system/update/history?target=${target}`)
      const data = await res.json()
      if (data.success && Array.isArray(data.data)) {
        setClientUpdateHistory(data.data.slice(0, 10))
      }
    } catch (error) {
      console.error(i18n.messages.fetchHistoryFailed + ':', error)
    }
  }, [i18n])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void fetchSystemInfo()
      void fetchConfigInfo()
      void fetchClientUpdateHistory()
      loadCachedUpdateSnapshot()
      loadCachedConfigUpdateSnapshot()
    }, 0)

    const handleSnapshotChanged = (event: Event) => {
      const detail = (event as CustomEvent<SystemUpdateSnapshot>).detail
      if (detail) {
        applySnapshot(detail)
      } else {
        loadCachedUpdateSnapshot()
      }
    }

    const handleConfigSnapshotChanged = (event: Event) => {
      const detail = (event as CustomEvent<SystemUpdateSnapshot>).detail
      if (detail) {
        applyConfigSnapshot(detail)
      } else {
        loadCachedConfigUpdateSnapshot()
      }
    }

    window.addEventListener('hephaestus:system-update-changed', handleSnapshotChanged)
    window.addEventListener('hephaestus:config-update-changed', handleConfigSnapshotChanged)
    return () => {
      window.clearTimeout(timer)
      window.removeEventListener('hephaestus:system-update-changed', handleSnapshotChanged)
      window.removeEventListener('hephaestus:config-update-changed', handleConfigSnapshotChanged)
    }
  }, [
    applyConfigSnapshot,
    applySnapshot,
    fetchClientUpdateHistory,
    fetchConfigInfo,
    fetchSystemInfo,
    loadCachedConfigUpdateSnapshot,
    loadCachedUpdateSnapshot,
  ])

  useEffect(
    () =>
      subscribeStagedUpdateVersion((version) => {
        setClientPackageReadyVersion(version)
        setPendingRestartAction(readStagedUpdateAction() ?? 'restart')
      }),
    []
  )

  const handleCheckUpdate = async () => {
    if (systemInfo?.selfUpdateDisabled) {
      toast.error(i18n.messages.containerUpdateDisabled)
      return
    }

    setChecking(true)
    setStatus('checking')
    setNoUpdate(false)
    setHasUpdate(false)
    setUpdateInfo(null)

    try {
      const target = (await isDesktopRuntime()) ? 'desktop' : 'client'
      const res = await fetchWithAuth(`/api/v1/system/update/check?target=${target}`)
      const data = await res.json()

      if (data.success) {
        if (data.hasUpdate) {
          // 暂存的更新包只对同一版本有效。
          if (data.updateInfo.delivery === 'docker') {
            syncStagedDockerUpdate(data.updateInfo.version, data.stagedVersion)
          } else if (readStagedUpdateVersion() !== data.updateInfo.version) {
            persistStagedUpdateVersion(null)
          }
          setHasUpdate(true)
          setUpdateInfo(data.updateInfo)
          setStatus('available')
          persistUpdateSnapshot({
            currentVersion: systemInfo?.version,
            hasUpdate: true,
            noUpdate: false,
            updateInfo: data.updateInfo,
            checkedAt: getCurrentTimestamp(),
          })
          toast.success(
            i18n.messages.foundVersion.replace('{version}', ensureVersionPrefix(data.updateInfo.version))
          )
          fetchClientUpdateHistory()
        } else {
          persistStagedUpdateVersion(null)
          setNoUpdate(true)
          setStatus('up-to-date')
          persistUpdateSnapshot({
            currentVersion: systemInfo?.version,
            hasUpdate: false,
            noUpdate: true,
            updateInfo: null,
            checkedAt: getCurrentTimestamp(),
          })
          toast.success(i18n.messages.currentLatest)
        }
      } else {
        setStatus('error')
        toast.error(data.error || i18n.messages.checkFailed)
      }
    } catch (error: any) {
      setStatus('error')
      toast.error(`${i18n.messages.checkFailed}: ${error.message || i18n.unknown}`)
    } finally {
      setChecking(false)
    }
  }

  const handleDownloadUpdate = async () => {
    if (systemInfo?.selfUpdateDisabled) {
      toast.error(i18n.messages.containerUpdateDisabled)
      return
    }

    if (!updateInfo) return
    // 更新包已经下载完成时不允许重复下载。
    if (clientPackageReadyVersion === updateInfo.version) return

    persistStagedUpdateVersion(null)
    setDownloading(true)
    setStatus('downloading')
    setDownloadProgress(0)
    setDownloadedSize(0)
    setTotalSize(0)
    downloadStartTimeRef.current = getCurrentTimestamp()
    setEstimatedTimeLeft(0)
    const toastId = toast.loading(i18n.messages.downloadPackage)

    if (updateInfo.delivery === 'docker') {
      try {
        await downloadDockerUpdate(updateInfo.version, i18n.messages.downloadFailed)
        toast.dismiss(toastId)
        setDownloading(false)
        await finishClientUpdate('restart')
      } catch (error) {
        setStatus('error')
        toast.error(error instanceof Error ? error.message : i18n.messages.downloadFailed, { id: toastId })
      } finally {
        setDownloading(false)
      }
      return
    }

    const downloadSources = [
      { name: i18n.sourceGitee, url: updateInfo.mirrorUrl },
      { name: i18n.sourceGithub, url: updateInfo.downloadUrl },
    ]
    const clientDownloadTarget = (await isDesktopRuntime()) ? 'desktop' : 'client'

    let downloadSuccess = false

    for (const source of downloadSources) {
      if (!source.url) continue

      try {
        toast.loading(i18n.messages.downloadFrom.replace('{source}', source.name), { id: toastId })

        const apiBaseURL = getApiBaseURL()
        const token = getToken()
        const shouldAutoInstallDesktopDMG =
          clientDownloadTarget === 'desktop' && isDMGDownloadURL(source.url)
        const eventSource = new EventSource(
          `${apiBaseURL}/api/v1/system/update/download-stream?url=${encodeURIComponent(source.url)}&target=${clientDownloadTarget}&version=${encodeURIComponent(updateInfo.version)}&token=${token}`
        )

        await new Promise<void>((resolve, reject) => {
          eventSource.addEventListener('progress', (e) => {
            const data = JSON.parse(e.data)
            setDownloadProgress(data.progress)
            setDownloadedSize(data.downloaded)
            setTotalSize(data.total)

            const elapsed = (getCurrentTimestamp() - downloadStartTimeRef.current) / 1000
            if (data.downloaded > 0 && elapsed > 0) {
              const speed = data.downloaded / elapsed
              const remaining = data.total - data.downloaded
              const timeLeft = Math.ceil(remaining / speed)
              setEstimatedTimeLeft(timeLeft)
            }
          })

          eventSource.addEventListener('installing', () => {
            setStatus('installing')
          })

          eventSource.addEventListener('complete', () => {
            eventSource.close()
            downloadSuccess = true
            setStatus('installing')
            resolve()
          })

          eventSource.addEventListener('error', (e: any) => {
            eventSource.close()
            const errorData = e.data ? JSON.parse(e.data) : {}
            console.error(`从 ${source.name} 下载失败:`, errorData.message)
            reject(new Error(errorData.message || '下载失败'))
          })

          eventSource.onerror = () => {
            eventSource.close()
            reject(new Error('连接中断'))
          }
        })

        toast.success(i18n.messages.downloadComplete.replace('{source}', source.name), { id: toastId })
        setDownloading(false)
        await finishClientUpdate(shouldAutoInstallDesktopDMG ? 'install' : 'restart')
        return
      } catch (error: any) {
        console.error(`从 ${source.name} 下载失败:`, error)
        if (source === downloadSources[downloadSources.length - 1]) {
          toast.error(i18n.messages.allSourcesFailed.replace('{message}', error.message), { id: toastId })
        }
      }
    }

    if (!downloadSuccess) {
      setStatus('error')
    }

    setDownloading(false)
  }

  const handleCheckConfigUpdate = async () => {
    setConfigChecking(true)
    setConfigStatus('checking')
    setConfigNoUpdate(false)
    setConfigHasUpdate(false)
    setConfigUpdateInfo(null)

    try {
      const res = await fetchWithAuth('/api/v1/system/update/config/check')
      const data = await res.json()

      if (data.success) {
        if (data.hasUpdate) {
          setConfigHasUpdate(true)
          setConfigUpdateInfo(data.updateInfo)
          setConfigStatus('available')
          setConfigInfo({ version: data.currentVersion || configInfo?.version || 'v0.0.0' })
          persistConfigUpdateSnapshot({
            currentVersion: data.currentVersion || configInfo?.version || 'v0.0.0',
            hasUpdate: true,
            noUpdate: false,
            updateInfo: data.updateInfo,
            checkedAt: getCurrentTimestamp(),
          })
          toast.success(
            i18n.messages.foundConfigVersion.replace('{version}', ensureVersionPrefix(data.updateInfo.version))
          )
        } else {
          setConfigNoUpdate(true)
          setConfigStatus('up-to-date')
          setConfigInfo({ version: data.currentVersion || configInfo?.version || 'v0.0.0' })
          persistConfigUpdateSnapshot({
            currentVersion: data.currentVersion || configInfo?.version || 'v0.0.0',
            hasUpdate: false,
            noUpdate: true,
            updateInfo: null,
            checkedAt: getCurrentTimestamp(),
          })
          toast.success(i18n.messages.configLatest)
        }
      } else {
        setConfigStatus('error')
        toast.error(data.error || i18n.messages.checkConfigFailed)
      }
    } catch (error: any) {
      setConfigStatus('error')
      toast.error(`${i18n.messages.checkConfigFailed}: ${error.message || i18n.unknown}`)
    } finally {
      setConfigChecking(false)
    }
  }

  const handleDownloadConfigUpdate = async () => {
    if (!configUpdateInfo) return

    setConfigDownloading(true)
    setConfigStatus('downloading')
    setConfigDownloadProgress(0)
    setConfigDownloadedSize(0)
    setConfigTotalSize(0)
    configDownloadStartTimeRef.current = getCurrentTimestamp()
    setConfigEstimatedTimeLeft(0)
    const toastId = toast.loading(i18n.messages.downloadConfigPackage)

    const downloadSources = Array.from(
      new Set([configUpdateInfo.mirrorUrl, configUpdateInfo.downloadUrl].filter(Boolean))
    ).map((url) => ({ name: i18n.sourceGitee, url }))

    let downloadSuccess = false

    for (const source of downloadSources) {
      if (!source.url) continue

      try {
        toast.loading(i18n.messages.downloadConfigFrom.replace('{source}', source.name), { id: toastId })

        const apiBaseURL = getApiBaseURL()
        const token = getToken()
        const eventSource = new EventSource(
          `${apiBaseURL}/api/v1/system/update/config/download-stream?url=${encodeURIComponent(source.url)}&version=${encodeURIComponent(configUpdateInfo.version)}&token=${token}`
        )

        await new Promise<void>((resolve, reject) => {
          eventSource.addEventListener('progress', (e) => {
            const data = JSON.parse(e.data)
            setConfigDownloadProgress(data.progress)
            setConfigDownloadedSize(data.downloaded)
            setConfigTotalSize(data.total)

            const elapsed = (getCurrentTimestamp() - configDownloadStartTimeRef.current) / 1000
            if (data.downloaded > 0 && elapsed > 0) {
              const speed = data.downloaded / elapsed
              const remaining = data.total - data.downloaded
              const timeLeft = Math.ceil(remaining / speed)
              setConfigEstimatedTimeLeft(timeLeft)
            }
          })

          eventSource.addEventListener('installing', () => {
            setConfigStatus('installing')
          })

          eventSource.addEventListener('complete', () => {
            eventSource.close()
            toast.success(i18n.messages.downloadConfigComplete.replace('{source}', source.name), { id: toastId })
            downloadSuccess = true
            setConfigStatus('up-to-date')
            setConfigHasUpdate(false)
            setConfigNoUpdate(true)
            setConfigInfo({ version: configUpdateInfo.version })
            persistConfigUpdateSnapshot({
              currentVersion: configUpdateInfo.version,
              hasUpdate: false,
              noUpdate: true,
              updateInfo: null,
              checkedAt: getCurrentTimestamp(),
            })
            resolve()
          })

          eventSource.addEventListener('error', (e: any) => {
            eventSource.close()
            const errorData = e.data ? JSON.parse(e.data) : {}
            console.error(`从 ${source.name} 下载配置失败:`, errorData.message)
            reject(new Error(errorData.message || '下载失败'))
          })

          eventSource.onerror = () => {
            eventSource.close()
            reject(new Error('连接中断'))
          }
        })

        if (downloadSuccess) {
          break
        }
      } catch (error: any) {
        console.error(`从 ${source.name} 下载配置失败:`, error)
        if (source === downloadSources[downloadSources.length - 1]) {
          toast.error(i18n.messages.allConfigSourcesFailed.replace('{message}', error.message), { id: toastId })
        }
      }
    }

    if (!downloadSuccess) {
      setConfigStatus('error')
    }

    setConfigDownloading(false)
  }

  // 重启后等新进程接管服务再整页刷新；前端资源随后端二进制一起替换，
  // 提前刷新只会重新加载旧版本页面。
  const reloadAfterRestart = async (toastId: string, expectedVersion?: string | null) => {
    await reloadAfterServiceRestart({
      expectedVersion: expectedVersion ?? undefined,
      waitingMessage: i18n.waitingRestart,
      readyMessage: i18n.messages.reloadReady,
      timeoutMessage: i18n.messages.restartTimeout,
      toastId,
    })
  }

  const restartDocker = async (version: string) => {
    if (dockerRestarting) return
    setDockerRestarting(true)
    const toastId = toast.loading(i18n.autoRestarting)
    try {
      await requestSystemUpdateRestart(version)
      setRestartDialogOpen(false)
      if (await reloadAfterServiceRestart({
        expectedVersion: version,
        waitingMessage: i18n.waitingRestart,
        readyMessage: i18n.messages.reloadReady,
        timeoutMessage: i18n.messages.restartTimeout,
        timeoutMs: 360_000,
        docker: true,
        toastId,
      })) return
    } catch (error) {
      toast.error(`${i18n.messages.restartFailed}: ${error instanceof Error ? error.message : i18n.unknown}`, { id: toastId })
    } finally {
      setDockerRestarting(false)
    }
    persistStagedUpdateVersion(null)
    setStatus('error')
  }

  const handleRestart = async () => {
    if (updateInfo?.delivery === 'docker') {
      await restartDocker(updateInfo.version)
      return
    }
    const toastId = toast.loading(i18n.autoRestarting)
    const action = pendingRestartAction
    try {
      if (action === 'install') {
        await installDesktopUpdatePackage()
      } else {
        await requestSystemUpdateRestart()
      }
      setRestartDialogOpen(false)
      persistStagedUpdateVersion(null)
      setClientPackageReadyVersion(null)
      setStatus('up-to-date')
      setHasUpdate(false)
      setNoUpdate(true)

      if (action === 'install') {
        toast.success(i18n.messages.openInstallerSuccess, { id: toastId })
        return
      }

      await reloadAfterRestart(toastId, updateInfo?.version)
    } catch (error: any) {
      persistStagedUpdateVersion(null)
      persistStagedUpdateAction(null)
      setClientPackageReadyVersion(null)
      toast.error(`${i18n.messages.restartFailed}: ${error.message || i18n.unknown}`, { id: toastId })
    }
  }

  // 下载完成后：没有运行中任务就直接自动重启；有任务或状态未知则弹窗确认。
  const finishClientUpdate = async (action: 'restart' | 'install') => {
    setPendingRestartAction(action)
    setStatus('installing')
    setClientPackageReadyVersion(updateInfo?.version ?? null)
    // 先记录完成方式，再记录版本，订阅方收到版本事件时就能读到正确的动作。
    persistStagedUpdateAction(action)
    persistStagedUpdateVersion(updateInfo?.version ?? null)

    const summary = await fetchRunningUpdateTasks()
    setRunningTasks(summary)

    if (!summary.known || summary.total > 0) {
      setRestartDialogOpen(true)
      return
    }

    if (updateInfo?.delivery === 'docker') {
      await restartDocker(updateInfo.version)
      return
    }

    const toastId = toast.loading(i18n.autoRestarting)
    try {
      if (action === 'install') {
        await installDesktopUpdatePackage()
      } else {
        await requestSystemUpdateRestart()
      }
      persistStagedUpdateVersion(null)
      setClientPackageReadyVersion(null)
      setStatus('up-to-date')
      setHasUpdate(false)
      setNoUpdate(true)

      if (action === 'install') {
        toast.success(i18n.messages.openInstallerSuccess, { id: toastId })
        return
      }

      await reloadAfterRestart(toastId, updateInfo?.version)
    } catch (error: any) {
      persistStagedUpdateVersion(null)
      persistStagedUpdateAction(null)
      setClientPackageReadyVersion(null)
      toast.error(`${i18n.messages.restartFailed}: ${error.message || i18n.unknown}`, { id: toastId })
      setStatus('error')
      setRestartDialogOpen(true)
    }
  }

  const openRestartDialog = async () => {
    const summary = await fetchRunningUpdateTasks()
    setRunningTasks(summary)
    setRestartDialogOpen(true)
  }

  const formatFileSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(2) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(2) + ' MB'
  }

  const formatTime = (seconds: number) => {
    if (seconds < 60) return `${seconds}${i18n.progress.second}`
    const minutes = Math.floor(seconds / 60)
    const secs = seconds % 60
    return `${minutes}${i18n.progress.minute}${secs}${i18n.progress.second}`
  }

  const getChangelogLines = (changelog?: string) => {
    if (!changelog) return []
    return changelog
      .split('\n')
      .map((line) => getPreviewLine(line.trim()))
      .filter(Boolean)
      .slice(0, 8)
  }

  const getPreviewLine = (line: string) => {
    return line
      .replace(/^[-*•#]+\s*/, '')
      .replace(/!\[[^\]]*\]\([^)]+\)/g, i18n.imageFallback)
      .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')
      .replace(/[*_`>#~]/g, '')
      .trim()
  }

  const escapeHTML = (value: string) =>
    value
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;')

  const renderInlineMarkdown = (value: string) => {
    let html = escapeHTML(value)
    html = html.replace(
      /!\[([^\]]*)\]\((https?:\/\/[^)\s]+)\)/g,
      '<img src="$2" alt="$1" />'
    )
    html = html.replace(
      /\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g,
      '<a href="$2" target="_blank" rel="noreferrer">$1</a>'
    )
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>')
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    html = html.replace(/__([^_]+)__/g, '<strong>$1</strong>')
    html = html.replace(/\*([^*]+)\*/g, '<em>$1</em>')
    html = html.replace(/_([^_]+)_/g, '<em>$1</em>')
    return html
  }

  const renderMarkdownHTML = (markdown?: string) => {
    if (!markdown) return ''

    const lines = markdown.replace(/\r\n/g, '\n').split('\n')
    const html: string[] = []
    let paragraph: string[] = []
    let listType: 'ul' | 'ol' | null = null
    let codeBlock: string[] = []
    let inCodeBlock = false
    let inBlockquote = false
    let blockquoteLines: string[] = []

    const flushParagraph = () => {
      if (!paragraph.length) return
      html.push(`<p>${renderInlineMarkdown(paragraph.join('<br />'))}</p>`)
      paragraph = []
    }

    const flushList = () => {
      if (!listType) return
      html.push(`</${listType}>`)
      listType = null
    }

    const flushCodeBlock = () => {
      if (!inCodeBlock) return
      html.push(`<pre><code>${escapeHTML(codeBlock.join('\n'))}</code></pre>`)
      codeBlock = []
      inCodeBlock = false
    }

    const flushBlockquote = () => {
      if (!inBlockquote) return
      html.push(`<blockquote>${blockquoteLines.join('<br />')}</blockquote>`)
      blockquoteLines = []
      inBlockquote = false
    }

    for (const rawLine of lines) {
      const line = rawLine.trimEnd()
      const trimmed = line.trim()

      if (trimmed.startsWith('```')) {
        flushParagraph()
        flushList()
        flushBlockquote()
        if (inCodeBlock) {
          flushCodeBlock()
        } else {
          inCodeBlock = true
          codeBlock = []
        }
        continue
      }

      if (inCodeBlock) {
        codeBlock.push(rawLine)
        continue
      }

      if (!trimmed) {
        flushParagraph()
        flushList()
        flushBlockquote()
        continue
      }

      if (trimmed.startsWith('> ')) {
        flushParagraph()
        flushList()
        inBlockquote = true
        blockquoteLines.push(renderInlineMarkdown(trimmed.slice(2)))
        continue
      }

      flushBlockquote()

      const heading = trimmed.match(/^(#{1,3})\s+(.*)$/)
      if (heading) {
        flushParagraph()
        flushList()
        const level = heading[1].length
        html.push(`<h${level}>${renderInlineMarkdown(heading[2])}</h${level}>`)
        continue
      }

      if (/^---+$/.test(trimmed) || /^\*\*\*+$/.test(trimmed)) {
        flushParagraph()
        flushList()
        html.push('<hr />')
        continue
      }

      const unordered = trimmed.match(/^[-*]\s+(.*)$/)
      if (unordered) {
        flushParagraph()
        if (listType !== 'ul') {
          flushList()
          listType = 'ul'
          html.push('<ul>')
        }
        html.push(`<li>${renderInlineMarkdown(unordered[1])}</li>`)
        continue
      }

      const ordered = trimmed.match(/^\d+\.\s+(.*)$/)
      if (ordered) {
        flushParagraph()
        if (listType !== 'ol') {
          flushList()
          listType = 'ol'
          html.push('<ol>')
        }
        html.push(`<li>${renderInlineMarkdown(ordered[1])}</li>`)
        continue
      }

      flushList()
      paragraph.push(trimmed)
    }

    flushParagraph()
    flushList()
    flushBlockquote()
    flushCodeBlock()

    return DOMPurify.sanitize(html.join(''), {
      USE_PROFILES: { html: true },
      ADD_ATTR: ['target', 'rel'],
    })
  }

  const markdownClassName =
    '[&_h1]:mb-3 [&_h1]:text-2xl [&_h1]:font-semibold [&_h1]:text-foreground ' +
    '[&_h2]:mb-2 [&_h2]:mt-5 [&_h2]:text-lg [&_h2]:font-semibold [&_h2]:text-foreground first:[&_h2]:mt-0 ' +
    '[&_h3]:mb-2 [&_h3]:mt-4 [&_h3]:text-base [&_h3]:font-semibold [&_h3]:text-foreground first:[&_h3]:mt-0 ' +
    '[&_p]:mb-3 [&_p]:leading-6 last:[&_p]:mb-0 ' +
    '[&_ul]:mb-3 [&_ul]:list-disc [&_ul]:space-y-1.5 [&_ul]:pl-5 last:[&_ul]:mb-0 ' +
    '[&_ol]:mb-3 [&_ol]:list-decimal [&_ol]:space-y-1.5 [&_ol]:pl-5 last:[&_ol]:mb-0 ' +
    '[&_li]:leading-6 [&_strong]:font-semibold [&_strong]:text-foreground ' +
    '[&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.9em] [&_code]:text-foreground ' +
    '[&_pre]:mb-3 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-border [&_pre]:bg-muted/40 [&_pre]:p-3 [&_pre]:text-sm [&_pre]:text-foreground ' +
    '[&_pre_code]:bg-transparent [&_pre_code]:p-0 ' +
    '[&_a]:text-primary [&_a]:underline [&_a]:underline-offset-4 ' +
    '[&_img]:my-3 [&_img]:max-h-[420px] [&_img]:w-full [&_img]:rounded-lg [&_img]:border [&_img]:border-border [&_img]:object-contain ' +
    '[&_hr]:my-4 [&_hr]:border-border ' +
    '[&_blockquote]:mb-3 [&_blockquote]:border-l-2 [&_blockquote]:border-primary/30 [&_blockquote]:pl-4 [&_blockquote]:italic'

  const currentVersion = systemInfo?.version
    ? ensureVersionPrefix(systemInfo.version)
    : '...'
  const latestVersion = updateInfo?.version
    ? ensureVersionPrefix(updateInfo.version)
    : i18n.none
  const currentConfigVersion = configInfo?.version
    ? ensureVersionPrefix(configInfo.version)
    : '...'
  const latestConfigVersion = configUpdateInfo?.version
    ? ensureVersionPrefix(configUpdateInfo.version)
    : i18n.none
  const displayedHistory = clientUpdateHistory.slice(0, 10)
  const latestHistoryVersion = displayedHistory[0]?.version
    ? ensureVersionPrefix(displayedHistory[0].version)
    : i18n.noHistory

  const toggleExpand = (id: string) => {
    setExpandedItems((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id]
    )
  }

  const statusMeta: Record<UpdateStatus, { label: string }> = {
    idle: { label: i18n.pending },
    checking: { label: i18n.checking },
    available: { label: i18n.available },
    downloading: { label: i18n.downloading },
    installing: { label: i18n.installing },
    'up-to-date': { label: i18n.upToDate },
    error: { label: i18n.actionFailed },
  }

  const clientPackageReady = Boolean(
    updateInfo && clientPackageReadyVersion === updateInfo.version
  )

  return (
    <div className="space-y-4">
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div className="space-y-3">
          <UpdateCard
            id="client"
            docker={systemInfo?.deployment === 'docker'}
            name={i18n.clientName}
            currentVersion={currentVersion}
            latestVersion={hasUpdate ? latestVersion : currentVersion}
            releaseDate={updateInfo?.releaseDate ?? ''}
            changelog={updateInfo ? getChangelogLines(updateInfo.changelog) : []}
            changelogMarkdown={updateInfo?.changelog ?? ''}
            hasUpdate={hasUpdate}
            isUpToDate={noUpdate}
            actionLabel={
              clientPackageReady
                ? i18n.restartDialog.now
                : hasUpdate && updateInfo
                  ? downloading
                    ? i18n.actions.downloading
                    : i18n.actions.update
                  : checking
                    ? i18n.actions.checking
                    : i18n.actions.check
            }
            actionIcon={
              clientPackageReady ? (
                <RefreshCw className="size-3.5 text-primary" />
              ) : hasUpdate && updateInfo ? (
                <Download className="size-3.5 text-primary" />
              ) : (
                <RefreshCw className={cn('size-3.5 text-primary', checking && 'animate-spin')} />
              )
            }
            icon={<Package className={cn('size-5', hasUpdate ? 'text-primary' : 'text-muted-foreground')} />}
            iconWrap={hasUpdate ? 'bg-primary/10' : 'bg-muted'}
            extra={
              updateInfo?.delivery === 'docker' && downloading ? (
                <div className="mt-3 space-y-2 text-xs text-muted-foreground">
                  <span>{i18n.progress.downloading}</span>
                  <div className="h-1 animate-pulse rounded-full bg-primary/40" />
                </div>
              ) : updateInfo?.delivery !== 'docker' && (status === 'downloading' || status === 'installing') ? (
                <div className="mt-3 space-y-2">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">
                      {status === 'downloading' ? i18n.progress.downloading : i18n.progress.installing}
                    </span>
                    <span className="font-mono text-foreground">{Math.round(downloadProgress)}%</span>
                  </div>
                  <Progress value={status === 'downloading' ? downloadProgress : 100} className="h-1" />
                  <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                    <span>{i18n.progress.downloaded} {formatFileSize(downloadedSize)}</span>
                    <span>{i18n.progress.totalSize} {totalSize > 0 ? formatFileSize(totalSize) : '--'}</span>
                    <span>{i18n.progress.remaining} {estimatedTimeLeft > 0 ? formatTime(estimatedTimeLeft) : '--'}</span>
                  </div>
                </div>
              ) : null
            }
            statusLabel={clientPackageReady ? i18n.downloaded : statusMeta[status].label}
            actionDisabled={checking || downloading || dockerRestarting}
            isExpanded={expandedItems.includes('client')}
            newTag={i18n.newTag}
            upToDateLabel={i18n.upToDate}
            changelogTitle={i18n.changelogTitle}
            markdownClassName={markdownClassName}
            onAction={
              clientPackageReady
                ? openRestartDialog
                : hasUpdate && updateInfo
                  ? handleDownloadUpdate
                  : handleCheckUpdate
            }
            onToggle={toggleExpand}
            renderMarkdownHTML={renderMarkdownHTML}
          />
          <UpdateCard
            id="config"
            name={i18n.configName}
            currentVersion={currentConfigVersion}
            latestVersion={configHasUpdate ? latestConfigVersion : currentConfigVersion}
            releaseDate={configUpdateInfo?.releaseDate ?? ''}
            changelog={configUpdateInfo ? getChangelogLines(configUpdateInfo.changelog) : []}
            changelogMarkdown={configUpdateInfo?.changelog ?? ''}
            hasUpdate={configHasUpdate}
            isUpToDate={configNoUpdate}
            actionLabel={
              configHasUpdate && configUpdateInfo
                ? configDownloading
                  ? i18n.actions.downloading
                  : i18n.actions.update
                : configChecking
                  ? i18n.actions.checking
                  : i18n.actions.check
            }
            actionIcon={
              configHasUpdate && configUpdateInfo ? (
                <Download className="size-3.5 text-primary" />
              ) : (
                <RefreshCw className={cn('size-3.5 text-primary', configChecking && 'animate-spin')} />
              )
            }
            icon={<FileCode className={cn('size-5', configHasUpdate ? 'text-primary' : 'text-muted-foreground')} />}
            iconWrap={configHasUpdate ? 'bg-primary/10' : 'bg-muted'}
            extra={
              configStatus === 'downloading' || configStatus === 'installing' ? (
                <div className="mt-3 space-y-2">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">
                      {configStatus === 'downloading' ? i18n.progress.downloading : i18n.progress.applying}
                    </span>
                    <span className="font-mono text-foreground">
                      {Math.round(configDownloadProgress)}%
                    </span>
                  </div>
                  <Progress
                    value={configStatus === 'downloading' ? configDownloadProgress : 100}
                    className="h-1"
                  />
                  <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                    <span>{i18n.progress.downloaded} {formatFileSize(configDownloadedSize)}</span>
                    <span>{i18n.progress.totalSize} {configTotalSize > 0 ? formatFileSize(configTotalSize) : '--'}</span>
                    <span>{i18n.progress.remaining} {configEstimatedTimeLeft > 0 ? formatTime(configEstimatedTimeLeft) : '--'}</span>
                  </div>
                </div>
              ) : null
            }
            statusLabel={statusMeta[configStatus].label}
            actionDisabled={configChecking || configDownloading}
            isExpanded={expandedItems.includes('config')}
            newTag={i18n.newTag}
            upToDateLabel={i18n.upToDate}
            changelogTitle={i18n.changelogTitle}
            markdownClassName={markdownClassName}
            onAction={configHasUpdate && configUpdateInfo ? handleDownloadConfigUpdate : handleCheckConfigUpdate}
            onToggle={toggleExpand}
            renderMarkdownHTML={renderMarkdownHTML}
          />

        </div>

        <div className="overflow-hidden rounded-xl border border-border bg-card">
          <div className="p-4">
            <div className="mb-3 flex items-center gap-3">
              <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10">
                <HardDriveDownload className="size-5 text-primary" />
              </div>
              <div>
                <h3 className="font-medium text-foreground">{i18n.historyTitle}</h3>
                <p className="text-sm text-muted-foreground">
                  {latestHistoryVersion}
                </p>
              </div>
            </div>

            <div className="space-y-3">
              <div className="max-h-[420px] space-y-2 overflow-y-auto pr-1">
                {displayedHistory.length > 0 ? (
                  displayedHistory.map((item, index) => {
                    const itemVersion = ensureVersionPrefix(item.version)
                    return (
                      <div key={`${item.version}-${index}`} className="rounded-lg bg-muted/20 p-3">
                        <div className="mb-2 text-sm text-muted-foreground">
                          <span className="font-mono text-primary">{itemVersion}</span>
                          {item.releaseDate && (
                            <>
                              <span className="mx-2 text-muted-foreground/60">·</span>
                              {item.releaseDate}
                            </>
                          )}
                        </div>
                        {item.changelog ? (
                          <div
                            className={cn(markdownClassName, 'text-sm text-muted-foreground')}
                            dangerouslySetInnerHTML={{
                              __html: renderMarkdownHTML(item.changelog),
                            }}
                          />
                        ) : (
                          <div className="text-sm text-muted-foreground">{i18n.noHistoryContent}</div>
                        )}
                      </div>
                    )
                  })
                ) : (
                  <div className="rounded-lg bg-muted/30 px-3 py-4 text-sm text-muted-foreground">
                    {i18n.noHistoryContent}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>

      <AlertDialog open={restartDialogOpen} onOpenChange={setRestartDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{i18n.restartDialog.title}</AlertDialogTitle>
            <AlertDialogDescription className="space-y-2">
              <p>
                {runningTasks && !runningTasks.known
                  ? i18n.restartBlockedUnknown
                  : runningTasks && runningTasks.total > 0
                    ? i18n.restartBlocked.replace('{count}', String(runningTasks.total))
                    : i18n.restartDialog.description}
              </p>
              {runningTasks?.known && runningTasks.groups.length > 0 && (
                <p className="text-xs">
                  {formatRunningUpdateTasks(
                    runningTasks.groups,
                    i18n.taskGroups as Record<string, string>
                  )}
                </p>
              )}
              <p className="text-amber-500">{i18n.restartDialog.warning}</p>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{i18n.restartDialog.later}</AlertDialogCancel>
            <AlertDialogAction onClick={handleRestart} disabled={dockerRestarting}>
              {i18n.restartDialog.now}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
