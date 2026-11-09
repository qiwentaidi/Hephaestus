'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import DOMPurify from 'dompurify'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Badge } from '@/components/ui/badge'
import { AlertTriangle, Calendar, Check, Download, FileCode, Loader2, Package, Sparkles } from 'lucide-react'
import { getApiBaseURL, getToken } from '@/lib/api'
import { isDesktopRuntime } from '@/lib/desktop'
import {
  downloadDockerUpdate,
  fetchRunningUpdateTasks,
  formatRunningUpdateTasks,
  installDesktopUpdatePackage,
  persistStagedUpdateVersion,
  persistStagedUpdateAction,
  readStagedUpdateAction,
  readStagedUpdateVersion,
  reloadAfterServiceRestart,
  requestSystemUpdateRestart,
  subscribeStagedUpdateVersion,
  type RunningUpdateTaskSummary,
  type StagedUpdateAction,
} from '@/lib/system-update'
import { useI18n } from '@/lib/i18n'
import toast from 'react-hot-toast'

interface UpdateInfo {
  delivery?: 'docker'
  version: string
  releaseDate: string
  changelog: string
  downloadUrl: string
  mirrorUrl: string
}

type UpdateTarget = 'client' | 'config'

interface UpdateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  clientCurrentVersion?: string
  clientUpdateInfo?: UpdateInfo | null
  configCurrentVersion?: string
  configUpdateInfo?: UpdateInfo | null
  onIgnoreVersion: (version: string) => void
  onUpdateComplete?: (target: UpdateTarget, version: string) => void
}

type UpdateSection = {
  target: UpdateTarget
  title: string
  currentVersion: string
  updateInfo: UpdateInfo
}

export function UpdateDialog({
  open,
  onOpenChange,
  clientCurrentVersion,
  clientUpdateInfo,
  configCurrentVersion,
  configUpdateInfo,
  onIgnoreVersion,
  onUpdateComplete,
}: UpdateDialogProps) {
  const { t } = useI18n()
  const i18n = t.settings.systemUpdate
  const [activeTarget, setActiveTarget] = useState<UpdateTarget | null>(null)
  const [downloadedVersions, setDownloadedVersions] = useState<Partial<Record<UpdateTarget, string>>>({})
  const [downloadProgress, setDownloadProgress] = useState(0)
  const [downloadedSize, setDownloadedSize] = useState(0)
  const [totalSize, setTotalSize] = useState(0)
  const [estimatedTimeLeft, setEstimatedTimeLeft] = useState(0)
  const [runningTasks, setRunningTasks] = useState<RunningUpdateTaskSummary | null>(null)
  const [restartAction, setRestartAction] = useState<StagedUpdateAction | null>(
    () => readStagedUpdateAction()
  )
  const [restarting, setRestarting] = useState(false)
  const downloadStartTimeRef = useRef(0)
  const activeTargetRef = useRef<UpdateTarget | null>(null)
  const downloadedVersionsRef = useRef<Partial<Record<UpdateTarget, string>>>({})

  const sections = useMemo<UpdateSection[]>(() => {
    const next: UpdateSection[] = []
    if (clientUpdateInfo) {
      next.push({
        target: 'client',
        title: i18n.clientName,
        currentVersion: clientCurrentVersion || '',
        updateInfo: clientUpdateInfo,
      })
    }
    if (configUpdateInfo) {
      next.push({
        target: 'config',
        title: i18n.configName,
        currentVersion: configCurrentVersion || 'v0.0.0',
        updateInfo: configUpdateInfo,
      })
    }
    return next
  }, [clientCurrentVersion, clientUpdateInfo, configCurrentVersion, configUpdateInfo, i18n.clientName, i18n.configName])

  const clientDownloadReady = Boolean(
    clientUpdateInfo && downloadedVersions.client === clientUpdateInfo.version
  )
  const preventDismiss = Boolean(activeTarget || clientDownloadReady)

  useEffect(() => {
    downloadedVersionsRef.current = downloadedVersions
  }, [downloadedVersions])

  // 与设置页共用“已下载完成”状态：任一处下载完成后，另一处也能直接重启。
  useEffect(() => {
    const applyStagedVersion = (staged: string | null) => {
      setRestartAction(staged ? readStagedUpdateAction() ?? 'restart' : null)
      setDownloadedVersions((current) => {
        if (!staged && !current.client) return current
        const next = { ...current }
        if (staged) {
          next.client = staged
        } else {
          delete next.client
        }
        downloadedVersionsRef.current = next
        return next
      })
    }

    applyStagedVersion(readStagedUpdateVersion())
    return subscribeStagedUpdateVersion(applyStagedVersion)
  }, [])

  // 打开弹窗时刷新运行中任务，避免使用上次下载时的旧快照。
  useEffect(() => {
    if (!open || !clientDownloadReady) return
    let cancelled = false
    void fetchRunningUpdateTasks().then((summary) => {
      if (!cancelled) setRunningTasks(summary)
    })
    return () => {
      cancelled = true
    }
  }, [open, clientDownloadReady])

  const ensureVersionPrefix = (version: string) => {
    if (!version) return i18n.none
    return version.startsWith('v') ? version : `v${version}`
  }

  const formatFileSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
  }

  const formatTime = (seconds: number) => {
    if (seconds < 60) return `${seconds}${i18n.progress.second}`
    const minutes = Math.floor(seconds / 60)
    const secs = seconds % 60
    return `${minutes}${i18n.progress.minute}${secs}${i18n.progress.second}`
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

    for (const rawLine of lines) {
      const line = rawLine.trimEnd()
      const trimmed = line.trim()

      if (trimmed.startsWith('```')) {
        flushParagraph()
        flushList()
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
        continue
      }

      const heading = trimmed.match(/^(#{1,3})\s+(.*)$/)
      if (heading) {
        flushParagraph()
        flushList()
        const level = heading[1].length
        html.push(`<h${level}>${renderInlineMarkdown(heading[2])}</h${level}>`)
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
    flushCodeBlock()

    return DOMPurify.sanitize(html.join(''), {
      USE_PROFILES: { html: true },
      ADD_ATTR: ['target', 'rel'],
    })
  }

  const markdownClassName =
    '[&_h1]:mb-3 [&_h1]:text-xl [&_h1]:font-semibold [&_h1]:text-foreground ' +
    '[&_h2]:mb-2 [&_h2]:mt-4 [&_h2]:text-base [&_h2]:font-semibold [&_h2]:text-foreground first:[&_h2]:mt-0 ' +
    '[&_h3]:mb-2 [&_h3]:mt-3 [&_h3]:text-sm [&_h3]:font-semibold [&_h3]:text-foreground first:[&_h3]:mt-0 ' +
    '[&_p]:mb-2 [&_p]:leading-6 last:[&_p]:mb-0 ' +
    '[&_ul]:mb-2 [&_ul]:list-disc [&_ul]:space-y-1 [&_ul]:pl-5 last:[&_ul]:mb-0 ' +
    '[&_ol]:mb-2 [&_ol]:list-decimal [&_ol]:space-y-1 [&_ol]:pl-5 last:[&_ol]:mb-0 ' +
    '[&_li]:leading-6 [&_strong]:font-semibold [&_strong]:text-foreground ' +
    '[&_code]:rounded [&_code]:bg-muted [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.9em] [&_code]:text-foreground ' +
    '[&_pre]:mb-2 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-border [&_pre]:bg-muted/40 [&_pre]:p-3 [&_pre]:text-sm [&_pre]:text-foreground ' +
    '[&_pre_code]:bg-transparent [&_pre_code]:p-0 ' +
    '[&_a]:text-primary [&_a]:underline [&_a]:underline-offset-4 '

  const resetProgress = () => {
    setDownloadProgress(0)
    setDownloadedSize(0)
    setTotalSize(0)
    setEstimatedTimeLeft(0)
  }

  const isDownloaded = (target: UpdateTarget, version: string) =>
    downloadedVersionsRef.current[target] === version

  const isDismissLocked = () =>
    Boolean(
      activeTargetRef.current ||
        (clientUpdateInfo && downloadedVersionsRef.current.client === clientUpdateInfo.version)
    )

  const executeRestart = async (action: StagedUpdateAction) => {
    if (restarting) return

    setRestarting(true)
    const toastId = toast.loading(i18n.autoRestarting)
    try {
      if (action === 'install') {
        await installDesktopUpdatePackage()
        toast.success(i18n.messages.openInstallerSuccess, { id: toastId })
        onOpenChange(false)
        return
      }

      const expectedVersion = downloadedVersionsRef.current.client ?? clientUpdateInfo?.version
      const docker = clientUpdateInfo?.delivery === 'docker'
      await requestSystemUpdateRestart(docker ? expectedVersion : undefined)
      if (!docker) onOpenChange(false)
      // 前端产物随后端二进制一起替换：等新进程真正接管服务后再整页刷新，
      // 否则刷到的仍是旧进程里的旧页面。
      await reloadAfterServiceRestart({
        expectedVersion,
        docker,
        timeoutMs: docker ? 360_000 : undefined,
        waitingMessage: i18n.waitingRestart,
        readyMessage: i18n.messages.reloadReady,
        timeoutMessage: i18n.messages.restartTimeout,
        toastId,
      })
    } catch (error: any) {
      persistStagedUpdateVersion(null)
      persistStagedUpdateAction(null)
      setDownloadedVersions((current) => {
        const next = { ...current }
        delete next.client
        downloadedVersionsRef.current = next
        return next
      })
      toast.error(`${i18n.messages.restartFailed}: ${error.message || i18n.unknown}`, { id: toastId })
    } finally {
      setRestarting(false)
    }
  }

  // 下载完成后：没有运行中任务就直接自动重启；有任务或状态未知则等待用户确认。
  const finishClientUpdate = async (action: StagedUpdateAction, version: string) => {
    setRestartAction(action)
    // 先记录完成方式，再记录版本，订阅方收到版本事件时就能读到正确的动作。
    persistStagedUpdateAction(action)
    persistStagedUpdateVersion(version)
    const summary = await fetchRunningUpdateTasks()
    setRunningTasks(summary)

    if (!summary.known) {
      toast(i18n.restartBlockedUnknown, { icon: '⚠️' })
      return
    }
    if (summary.total > 0) {
      toast(i18n.restartBlocked.replace('{count}', String(summary.total)), { icon: '⚠️' })
      return
    }

    await executeRestart(action)
  }

  const handleUpdate = async (section: UpdateSection) => {
    // 已经下载完成的更新包不能再次触发下载，否则会覆盖已下载并安装好的更新状态。
    if (activeTargetRef.current || isDownloaded(section.target, section.updateInfo.version)) return

    activeTargetRef.current = section.target
    setActiveTarget(section.target)
    if (section.target === 'client') {
      persistStagedUpdateVersion(null)
    }
    setDownloadedVersions((current) => {
      const next = { ...current }
      delete next[section.target]
      downloadedVersionsRef.current = next
      return next
    })
    downloadStartTimeRef.current = Date.now()
    resetProgress()

    if (section.target === 'client' && section.updateInfo.delivery === 'docker') {
      try {
        await downloadDockerUpdate(section.updateInfo.version, i18n.messages.downloadFailed)
      } catch (error) {
        toast.error(error instanceof Error ? error.message : i18n.messages.downloadFailed)
        return
      } finally {
        activeTargetRef.current = null
        setActiveTarget(null)
      }
      await finishClientUpdate('restart', section.updateInfo.version)
      return
    }

    const clientDownloadTarget =
      section.target === 'client' && (await isDesktopRuntime()) ? 'desktop' : 'client'

    const downloadSources = [
      { name: i18n.sourceGitee, url: section.updateInfo.mirrorUrl },
      { name: i18n.sourceGithub, url: section.updateInfo.downloadUrl },
    ]

    for (const source of downloadSources) {
      if (!source.url) continue

      try {
        const apiBaseURL = getApiBaseURL()
        const token = getToken()
        const streamURL =
          section.target === 'config'
            ? `${apiBaseURL}/api/v1/system/update/config/download-stream?url=${encodeURIComponent(source.url)}&version=${encodeURIComponent(section.updateInfo.version)}&token=${token}`
            : `${apiBaseURL}/api/v1/system/update/download-stream?url=${encodeURIComponent(source.url)}&target=${clientDownloadTarget}&token=${token}`

        const eventSource = new EventSource(streamURL)

        await new Promise<void>((resolve, reject) => {
          eventSource.addEventListener('progress', (e) => {
            const data = JSON.parse(e.data)
            setDownloadProgress(data.progress)
            setDownloadedSize(data.downloaded)
            setTotalSize(data.total)

            const elapsed = (Date.now() - downloadStartTimeRef.current) / 1000
            if (data.downloaded > 0 && elapsed > 0) {
              const speed = data.downloaded / elapsed
              const remaining = data.total - data.downloaded
              setEstimatedTimeLeft(Math.ceil(remaining / speed))
            }
          })

          eventSource.addEventListener('installing', () => {
            setDownloadProgress(100)
          })

          eventSource.addEventListener('complete', () => {
            eventSource.close()
            resolve()
          })

          eventSource.addEventListener('error', (e: any) => {
            eventSource.close()
            const errorData = e.data ? JSON.parse(e.data) : {}
            reject(new Error(errorData.message || i18n.messages.downloadFailed))
          })

          eventSource.onerror = () => {
            eventSource.close()
            reject(new Error(i18n.messages.connectionInterrupted))
          }
        })

        const shouldAutoInstallDesktopDMG =
          section.target === 'client' && clientDownloadTarget === 'desktop' && isDMGDownloadURL(source.url)

        setDownloadedVersions((current) => {
          const next = { ...current, [section.target]: section.updateInfo.version }
          downloadedVersionsRef.current = next
          return next
        })
        if (section.target === 'config') {
          toast.success(i18n.messages.downloadConfigComplete.replace('{source}', source.name))
          activeTargetRef.current = null
          setActiveTarget(null)
          onUpdateComplete?.(section.target, section.updateInfo.version)
          return
        }

        toast.success(i18n.messages.downloadComplete.replace('{source}', source.name))
        activeTargetRef.current = null
        setActiveTarget(null)
        onUpdateComplete?.(section.target, section.updateInfo.version)

        await finishClientUpdate(
          shouldAutoInstallDesktopDMG ? 'install' : 'restart',
          section.updateInfo.version
        )
        return
      } catch (error: any) {
        if (source === downloadSources[downloadSources.length - 1]) {
          if (section.target === 'config') {
            toast.error(i18n.messages.allConfigSourcesFailed.replace('{message}', error.message))
          } else {
            toast.error(i18n.messages.allSourcesFailed.replace('{message}', error.message))
          }
        }
      }
    }

    activeTargetRef.current = null
    setActiveTarget(null)
    resetProgress()
  }

  const handlePostponeRestart = () => {
    if (restarting) return
    toast.success(i18n.restartPostponed)
    onOpenChange(false)
  }

  const handleIgnore = (version: string) => {
    onIgnoreVersion(version)
    toast.success(i18n.messages.ignoreSuccess)
  }

  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen && isDismissLocked()) {
      return
    }
    onOpenChange(nextOpen)
  }

  const handleClose = () => {
    if (isDismissLocked()) {
      return
    }
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent
        className="max-w-3xl"
        showCloseButton={!preventDismiss}
        onPointerDownOutside={(event) => event.preventDefault()}
        onInteractOutside={(event) => event.preventDefault()}
        onEscapeKeyDown={(event) => {
          if (isDismissLocked()) {
            event.preventDefault()
          }
        }}
      >
        <DialogHeader>
          <div className="flex items-center gap-2">
            <Sparkles className="size-5 text-primary" />
            <DialogTitle>{clientDownloadReady ? i18n.restartDialog.title : i18n.dialogTitle}</DialogTitle>
          </div>
          <DialogDescription>
            {clientDownloadReady
              ? restarting
                ? i18n.autoRestarting
                : runningTasks && !runningTasks.known
                  ? i18n.restartBlockedUnknown
                  : runningTasks && runningTasks.total > 0
                    ? i18n.restartBlocked.replace('{count}', String(runningTasks.total))
                    : i18n.restartDialog.description
              : sections.length > 1
                ? i18n.dialogDescriptionMultiple
                : i18n.dialogDescriptionSingle}
          </DialogDescription>
        </DialogHeader>

        {clientDownloadReady && runningTasks && (!runningTasks.known || runningTasks.total > 0) && (
          <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-600">
            <div className="flex items-center gap-2 font-medium">
              <AlertTriangle className="size-3.5" />
              <span>{i18n.restartDialog.warning}</span>
            </div>
            {runningTasks.known && runningTasks.groups.length > 0 && (
              <p className="mt-1 pl-5">
                {formatRunningUpdateTasks(
                  runningTasks.groups,
                  i18n.taskGroups as Record<string, string>
                )}
              </p>
            )}
          </div>
        )}

        <div className="space-y-4">
          {sections.map((section) => {
            const isActive = activeTarget === section.target
            const isDone = downloadedVersions[section.target] === section.updateInfo.version
            const icon = section.target === 'client'
              ? <Package className="size-4 text-primary" />
              : <FileCode className="size-4 text-primary" />

            return (
              <div key={section.target} className="rounded-xl border border-border bg-card p-4">
                <div className="mb-3 flex items-start justify-between gap-3">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      {icon}
                      <div className="font-medium text-foreground">{section.title}</div>
                      {isDone && <Badge variant="outline">{i18n.downloaded}</Badge>}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      {section.updateInfo.delivery === 'docker' && (
                        <Badge variant="secondary" className="mr-2">Docker</Badge>
                      )}
                      {ensureVersionPrefix(section.currentVersion)} →{' '}
                      <span className="font-medium text-primary">{ensureVersionPrefix(section.updateInfo.version)}</span>
                    </div>
                    {section.updateInfo.releaseDate && (
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <Calendar className="size-3.5" />
                        <span>{section.updateInfo.releaseDate}</span>
                      </div>
                    )}
                  </div>
                  <div className="flex items-center gap-2">
                    {section.target === 'client' && !isDone && (
                      <Button variant="ghost" onClick={() => handleIgnore(section.updateInfo.version)} disabled={isActive}>
                        {i18n.ignoreVersion}
                      </Button>
                    )}
                    <Button
                      variant={isDone ? 'secondary' : 'default'}
                      onClick={() => {
                        if (isDone) return
                        void handleUpdate(section)
                      }}
                      disabled={Boolean(activeTarget) || isDone}
                    >
                      {isDone ? (
                        <Check className="mr-2 size-4" />
                      ) : (
                        <Download className="mr-2 size-4" />
                      )}
                      {isActive ? i18n.actions.downloading : isDone ? i18n.downloaded : i18n.actions.update}
                    </Button>
                  </div>
                </div>

                {(section.updateInfo.delivery !== 'docker' || section.updateInfo.changelog) && (
                  <ScrollArea className="h-[160px] rounded-md border p-3">
                    <div
                      className={`${markdownClassName} text-sm text-muted-foreground`}
                      dangerouslySetInnerHTML={{
                        __html: renderMarkdownHTML(section.updateInfo.changelog),
                      }}
                    />
                  </ScrollArea>
                )}

                {isActive && (
                  <div className="mt-3 space-y-2 border-t pt-3">
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">
                        {section.target === 'config' ? i18n.progress.applying : i18n.progress.downloading}
                      </span>
                      {section.updateInfo.delivery !== 'docker' && (
                        <span className="font-semibold text-primary">{downloadProgress}%</span>
                      )}
                    </div>
                    {totalSize > 0 && (
                      <div className="text-xs text-muted-foreground space-y-1">
                        <div>{i18n.progress.sizeLabel}: {formatFileSize(downloadedSize)} / {formatFileSize(totalSize)}</div>
                        {estimatedTimeLeft > 0 && <div>{i18n.progress.remainingLabel}: {formatTime(estimatedTimeLeft)}</div>}
                      </div>
                    )}
                    <div className="h-2 w-full rounded-full bg-secondary">
                      <div
                        className={section.updateInfo.delivery === 'docker'
                          ? 'h-2 w-full animate-pulse rounded-full bg-primary/40'
                          : 'h-2 rounded-full bg-primary transition-all duration-300'}
                        style={section.updateInfo.delivery === 'docker' ? undefined : { width: `${downloadProgress}%` }}
                      />
                    </div>
                  </div>
                )}
              </div>
            )
          })}
        </div>

        <DialogFooter className="flex-row justify-end gap-2">
          <Button variant="outline" onClick={handleClose} disabled={preventDismiss}>
            {t.common.close}
          </Button>
          {clientDownloadReady && (
            <Button variant="outline" onClick={handlePostponeRestart} disabled={restarting}>
              {i18n.restartDialog.later}
            </Button>
          )}
          {clientDownloadReady && (
            <Button
              onClick={() => void executeRestart(restartAction ?? 'restart')}
              disabled={restarting}
            >
              {restarting && <Loader2 className="mr-2 size-4 animate-spin" />}
              {restarting ? i18n.restarting : i18n.restartDialog.now}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
