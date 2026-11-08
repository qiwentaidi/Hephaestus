'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { useRouter } from 'next/navigation'
import {
  CheckCircle2,
  Copy,
  FileArchive,
  FolderOpen,
  Package,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Settings2,
  Trash2,
} from 'lucide-react'

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '@/components/ui/resizable'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  decompileMiniProgram,
  getMiniProgramDecompileSettings,
  getMiniProgramMatches,
  listMiniPrograms,
  saveMiniProgramDecompileSettings,
  type MiniProgramDecompileResult,
  type MiniProgramItem,
  type MiniProgramMatchesResult,
  type MiniProgramRuleGroup,
  type MiniProgramVersion,
} from '@/lib/api/mini-program-decompile'
import { openLauncherFolder, selectLauncherPath } from '@/lib/api/launcher'
import { copyToClipboard } from '@/lib/clipboard'
import { isDesktopRuntime } from '@/lib/desktop'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

type SelectedVersion = {
  appId: string
  version: string
  outputDir?: string
}

const versionKey = (appId: string, version: string) => `${appId}:${version}`

const emptyMatchesResult = (): MiniProgramMatchesResult => ({ groups: [] })

const cloneRuleGroups = (groups: MiniProgramRuleGroup[]) => (
  groups.map((group) => ({
    ...group,
    rules: group.rules.map((rule) => ({ ...rule })),
  }))
)

const normalizeRuleGroupsForSave = (
  groups: MiniProgramRuleGroup[],
  getFallbackName: (index: number) => string,
) => (
  groups.map((group, groupIndex) => ({
    ...group,
    name: group.name.trim() || getFallbackName(groupIndex + 1),
    rules: group.rules
      .map((rule) => ({ ...rule, pattern: rule.pattern.trim() }))
      .filter((rule) => rule.pattern.length > 0),
  }))
)

const createDraftId = (prefix: string) => (
  `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
)

const getProgramName = (program: MiniProgramItem) => (
  program.info?.nickname?.trim() || program.info?.username?.trim() || program.appId
)

const getProgramAvatarFallback = (program: MiniProgramItem) => {
  const name = getProgramName(program)
  if (/^[a-zA-Z0-9]/.test(name)) return 'WX'
  return Array.from(name).slice(0, 2).join('') || 'WX'
}

const getProgramSearchText = (program: MiniProgramItem) => [
  program.appId,
  getProgramName(program),
  program.info?.username,
  program.info?.description,
  ...program.versions.map((version) => version.number),
]
  .filter(Boolean)
  .join('\n')
  .toLowerCase()

export default function MiniProgramDecompilePage() {
	const { t } = useI18n()
	const copy = t.tools.miniProgramDecompile
	const router = useRouter()

  const [appletPath, setAppletPath] = useState('')
  const [ruleGroups, setRuleGroups] = useState<MiniProgramRuleGroup[]>([])
  const [defaultRuleGroups, setDefaultRuleGroups] = useState<MiniProgramRuleGroup[]>([])
  const [ruleGroupsDraft, setRuleGroupsDraft] = useState<MiniProgramRuleGroup[]>([])
  const [outputRoot, setOutputRoot] = useState('')
  const [programs, setPrograms] = useState<MiniProgramItem[]>([])
  const [selected, setSelected] = useState<SelectedVersion | null>(null)
  const [matchesResult, setMatchesResult] = useState<MiniProgramMatchesResult>(emptyMatchesResult)
  const [lastResult, setLastResult] = useState<MiniProgramDecompileResult | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [rulesOpen, setRulesOpen] = useState(false)
  const [runningKey, setRunningKey] = useState('')
  const [autoDecompile, setAutoDecompile] = useState(false)
  const [programSearch, setProgramSearch] = useState('')
	const [activeResultTab, setActiveResultTab] = useState('')
	const [desktopRuntimeReady, setDesktopRuntimeReady] = useState(false)
	const [desktopRuntime, setDesktopRuntime] = useState(false)
  const selectedKeyRef = useRef('')
  const selectionVersionRef = useRef(0)

  const stats = useMemo(() => {
    const versions = programs.reduce((total, item) => total + item.versions.length, 0)
    const decompiled = programs.reduce(
      (total, item) => total + item.versions.filter((version) => version.decompiled).length,
      0,
    )
    return { programs: programs.length, versions, decompiled }
  }, [programs])

  const filteredPrograms = useMemo(() => {
    const keyword = programSearch.trim().toLowerCase()
    if (!keyword) return programs
    return programs.filter((program) => getProgramSearchText(program).includes(keyword))
  }, [programSearch, programs])

  const effectiveActiveResultTab = matchesResult.groups.some((group) => group.id === activeResultTab)
    ? activeResultTab
    : matchesResult.groups[0]?.id || ''

  const activeResultGroup = useMemo(() => (
    matchesResult.groups.find((group) => group.id === effectiveActiveResultTab) || null
  ), [effectiveActiveResultTab, matchesResult.groups])

  const activeResultItems = useMemo(() => (
    (activeResultGroup?.matches || []).map((match) => ({ value: match.value }))
  ), [activeResultGroup])

  const loadSettings = useCallback(async () => {
    const settings = await getMiniProgramDecompileSettings()
    const nextRuleGroups = cloneRuleGroups(settings.ruleGroups || [])
    setAppletPath(settings.appletPath || '')
    setRuleGroups(nextRuleGroups)
    setRuleGroupsDraft(cloneRuleGroups(nextRuleGroups))
    setDefaultRuleGroups(cloneRuleGroups(settings.defaultRuleGroups || []))
    setOutputRoot(settings.outputRoot || '')
    return settings
  }, [])

  const refreshPrograms = useCallback(async (path = appletPath) => {
    setLoading(true)
    try {
      const result = await listMiniPrograms(path)
      setPrograms(result.items || [])
      setOutputRoot(result.outputRoot || '')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : copy.toasts.loadFailed)
    } finally {
      setLoading(false)
    }
  }, [appletPath, copy.toasts.loadFailed])

  const refreshMatches = useCallback(async (target = selected) => {
    if (!target) return
    const targetKey = versionKey(target.appId, target.version)
    try {
      const result = await getMiniProgramMatches(target.appId, target.version)
      if (selectedKeyRef.current !== targetKey) return
      setMatchesResult(result)
    } catch (error) {
      if (selectedKeyRef.current === targetKey) {
        toast.error(error instanceof Error ? error.message : copy.toasts.loadMatchesFailed)
      }
    }
  }, [copy.toasts.loadMatchesFailed, selected])

	useEffect(() => {
		let cancelled = false

		void isDesktopRuntime().then((isDesktop) => {
			if (cancelled) return
			setDesktopRuntime(isDesktop)
			setDesktopRuntimeReady(true)
			if (!isDesktop) router.replace('/dashboard')
		})

		return () => {
			cancelled = true
		}
	}, [router])

	useEffect(() => {
		if (!desktopRuntime) return
		let cancelled = false

    async function load() {
      setLoading(true)
      try {
        const settings = await loadSettings()
        if (cancelled) return
        const result = await listMiniPrograms(settings.appletPath)
        if (cancelled) return
        setPrograms(result.items || [])
        setOutputRoot(result.outputRoot || settings.outputRoot || '')
      } catch (error) {
        if (!cancelled) {
          toast.error(error instanceof Error ? error.message : copy.toasts.loadFailed)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => {
      cancelled = true
    }
	}, [copy.toasts.loadFailed, desktopRuntime, loadSettings])

  const handleSaveSettings = useCallback(async (nextRuleGroups = ruleGroups) => {
    setSaving(true)
    try {
      await saveMiniProgramDecompileSettings({ appletPath, ruleGroups: nextRuleGroups })
      setRuleGroups(cloneRuleGroups(nextRuleGroups))
      setRuleGroupsDraft(cloneRuleGroups(nextRuleGroups))
      toast.success(copy.toasts.saveSuccess)
      return true
    } catch (error) {
      toast.error(error instanceof Error ? error.message : copy.toasts.saveFailed)
      return false
    } finally {
      setSaving(false)
    }
  }, [appletPath, copy.toasts.saveFailed, copy.toasts.saveSuccess, ruleGroups])

  const handleSelectPath = useCallback(async () => {
    try {
      const path = await selectLauncherPath({ directory: true })
      if (path) setAppletPath(path)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : copy.toasts.selectPathFailed)
    }
  }, [copy.toasts.selectPathFailed])

  const handleCopy = useCallback(async (value: string, successMessage = copy.toasts.copySuccess) => {
    if (!value.trim()) {
      toast(copy.toasts.emptyCopy)
      return
    }
    const ok = await copyToClipboard(value)
    if (ok) {
      toast.success(successMessage)
    } else {
      toast.error(copy.toasts.copyFailed)
    }
  }, [copy.toasts.copyFailed, copy.toasts.copySuccess, copy.toasts.emptyCopy])

  const handleDecompile = useCallback(async (appId: string, version: string, silent = false) => {
    const key = versionKey(appId, version)
    const selectionVersionAtStart = selectionVersionRef.current
    let shouldRefreshPrograms = false
    setRunningKey(key)
    try {
      const result = await decompileMiniProgram({
        appletPath,
        appId,
        version,
        ruleGroups,
      })
      const shouldApplyResult = (
        selectionVersionRef.current === selectionVersionAtStart
        || selectedKeyRef.current === key
      )
      if (shouldApplyResult) {
        selectedKeyRef.current = key
        setLastResult(result)
        setSelected({ appId, version, outputDir: result.outputDir })
        setActiveResultTab('')
        setMatchesResult({
          groups: result.matchGroups || [],
        })
      }
      shouldRefreshPrograms = true
      try {
        const freshMatches = await getMiniProgramMatches(appId, version)
        if (selectedKeyRef.current === key) {
          setMatchesResult(freshMatches)
        }
      } catch (error) {
        if (selectedKeyRef.current === key) {
          toast.error(error instanceof Error ? error.message : copy.toasts.loadMatchesFailed)
        }
      }
      if (!silent) {
        toast.success(copy.toasts.decompileSuccess)
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : copy.toasts.decompileFailed)
    } finally {
      setRunningKey('')
    }
    if (shouldRefreshPrograms) {
      await refreshPrograms(appletPath)
    }
  }, [
    appletPath,
    copy.toasts.decompileFailed,
    copy.toasts.decompileSuccess,
    copy.toasts.loadMatchesFailed,
    refreshPrograms,
    ruleGroups,
  ])

  useEffect(() => {
    if (!autoDecompile || runningKey || loading) return

    let timer: ReturnType<typeof setTimeout> | null = null
    for (const program of programs) {
      const nextVersion = program.versions.find((version) => !version.decompiled)
      if (nextVersion) {
        timer = setTimeout(() => {
          void handleDecompile(program.appId, nextVersion.number, true)
        }, 0)
        return () => {
          if (timer) clearTimeout(timer)
        }
      }
    }

    if (programs.length > 0) {
      timer = setTimeout(() => {
        toast.success(copy.toasts.autoComplete)
        setAutoDecompile(false)
      }, 0)
      return () => {
        if (timer) clearTimeout(timer)
      }
    }
  }, [autoDecompile, copy.toasts.autoComplete, handleDecompile, loading, programs, runningKey])

  const handleSelectVersion = useCallback(async (
    appId: string,
    version: MiniProgramVersion,
  ) => {
    selectedKeyRef.current = versionKey(appId, version.number)
    selectionVersionRef.current += 1
    setSelected({ appId, version: version.number, outputDir: version.outputDir })
    setLastResult(null)
    setActiveResultTab('')
    await refreshMatches({ appId, version: version.number, outputDir: version.outputDir })
  }, [refreshMatches])

  const handleOpenFolder = useCallback(async (path?: string) => {
    const target = path || outputRoot
    if (!target) return
    try {
      await openLauncherFolder(target)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : copy.toasts.openFolderFailed)
    }
  }, [copy.toasts.openFolderFailed, outputRoot])

  const saveRules = useCallback(async () => {
    const nextRuleGroups = normalizeRuleGroupsForSave(
      ruleGroupsDraft.length ? ruleGroupsDraft : defaultRuleGroups,
      (index) => copy.rules.newGroupName.replace('{index}', String(index)),
    )
    const saved = await handleSaveSettings(nextRuleGroups)
    if (saved) setRulesOpen(false)
  }, [copy.rules.newGroupName, defaultRuleGroups, handleSaveSettings, ruleGroupsDraft])

  const openRules = useCallback(() => {
    setRuleGroupsDraft(cloneRuleGroups(ruleGroups))
    setRulesOpen(true)
  }, [ruleGroups])

  const updateRuleGroup = useCallback((groupId: string, updates: Partial<MiniProgramRuleGroup>) => {
    setRuleGroupsDraft((current) => current.map((group) => (
      group.id === groupId ? { ...group, ...updates } : group
    )))
  }, [])

  const addRuleGroup = useCallback(() => {
    setRuleGroupsDraft((current) => [
      ...current,
      {
        id: createDraftId('group'),
        name: copy.rules.newGroupName.replace('{index}', String(current.length + 1)),
        enabled: true,
        rules: [],
      },
    ])
  }, [copy.rules.newGroupName])

  const removeRuleGroup = useCallback((groupId: string) => {
    setRuleGroupsDraft((current) => current.filter((group) => group.id !== groupId))
  }, [])

  const addRule = useCallback((groupId: string) => {
    setRuleGroupsDraft((current) => current.map((group) => (
      group.id === groupId
        ? {
            ...group,
            rules: [
              ...group.rules,
              { id: createDraftId('rule'), pattern: '', enabled: true },
            ],
          }
        : group
    )))
  }, [])

  const updateRule = useCallback((
    groupId: string,
    ruleId: string,
    updates: Partial<MiniProgramRuleGroup['rules'][number]>,
  ) => {
    setRuleGroupsDraft((current) => current.map((group) => (
      group.id === groupId
        ? {
            ...group,
            rules: group.rules.map((rule) => (
              rule.id === ruleId ? { ...rule, ...updates } : rule
            )),
          }
        : group
    )))
  }, [])

  const removeRule = useCallback((groupId: string, ruleId: string) => {
    setRuleGroupsDraft((current) => current.map((group) => (
      group.id === groupId
        ? { ...group, rules: group.rules.filter((rule) => rule.id !== ruleId) }
        : group
    )))
  }, [])

	if (!desktopRuntimeReady || !desktopRuntime) return null

	return (
    <TooltipProvider delayDuration={200}>
      <div className="flex h-[calc(100vh-4rem)] min-h-0 flex-col gap-4 p-4">
        <div className="flex flex-col gap-3 rounded-md border bg-card p-4">
          <div className="flex flex-wrap items-end gap-3">
            <Field className="min-w-[280px] flex-1 gap-2">
              <FieldLabel htmlFor="mini-program-applet-path">{copy.controls.appletPath}</FieldLabel>
              <div className="flex gap-2">
                <Input
                  id="mini-program-applet-path"
                  value={appletPath}
                  onChange={(event) => setAppletPath(event.target.value)}
                  placeholder={copy.controls.appletPathPlaceholder}
                />
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button variant="outline" size="icon" onClick={handleSelectPath} aria-label={copy.controls.selectPath}>
                      <FolderOpen />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{copy.controls.selectPath}</TooltipContent>
                </Tooltip>
              </div>
            </Field>

            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" onClick={() => void handleSaveSettings()} disabled={saving}>
                {saving ? <Spinner data-icon="inline-start" /> : <CheckCircle2 data-icon="inline-start" />}
                {copy.controls.save}
              </Button>
              <Button variant="outline" onClick={() => void refreshPrograms()} disabled={loading}>
                {loading ? <Spinner data-icon="inline-start" /> : <RefreshCw data-icon="inline-start" />}
                {copy.controls.refresh}
              </Button>
              <Button variant="outline" onClick={openRules}>
                <Settings2 data-icon="inline-start" />
                {copy.controls.rules}
              </Button>
              <Button variant="outline" onClick={() => void handleOpenFolder(outputRoot)} disabled={!outputRoot}>
                <FolderOpen data-icon="inline-start" />
                {copy.controls.output}
              </Button>
            </div>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap gap-2">
              <Badge variant="secondary">{copy.stats.programs.replace('{count}', String(stats.programs))}</Badge>
              <Badge variant="secondary">{copy.stats.versions.replace('{count}', String(stats.versions))}</Badge>
              <Badge variant="secondary">{copy.stats.decompiled.replace('{count}', String(stats.decompiled))}</Badge>
            </div>
            <div className="flex items-center gap-2">
              <Switch
                id="mini-program-auto-decompile"
                checked={autoDecompile}
                onCheckedChange={setAutoDecompile}
                disabled={loading || !programs.length}
              />
              <label htmlFor="mini-program-auto-decompile" className="text-sm text-muted-foreground">
                {copy.controls.autoDecompile}
              </label>
            </div>
          </div>
          {autoDecompile && (
            <Progress value={stats.versions ? Math.round((stats.decompiled / stats.versions) * 100) : 0} />
          )}
        </div>

        <ResizablePanelGroup direction="horizontal" className="min-h-0 flex-1 rounded-md border">
          <ResizablePanel defaultSize={38} minSize={28}>
            <div className="flex h-full min-h-0 flex-col">
              <div className="flex h-14 shrink-0 flex-nowrap items-center justify-between gap-3 border-b bg-muted/40 px-3 py-2">
                <div className="flex h-9 shrink-0 items-center gap-2 rounded-md bg-primary/10 px-3 font-medium text-primary">
                  <FileArchive className="size-4 shrink-0" />
                  <span>{copy.programs.title}</span>
                  <Badge variant="secondary" className="bg-background/80 text-foreground">
                    {copy.programs.listCount.replace('{count}', String(filteredPrograms.length))}
                  </Badge>
                  {loading && <Spinner />}
                </div>
                <div className="relative min-w-0 flex-1 sm:max-w-[340px]">
                  <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={programSearch}
                    onChange={(event) => setProgramSearch(event.target.value)}
                    placeholder={copy.programs.searchPlaceholder}
                    className="h-9 bg-background pl-9"
                  />
                </div>
              </div>
              <ScrollArea className="min-h-0 flex-1">
                {filteredPrograms.length === 0 && !loading ? (
                  <Empty className="h-full min-h-[360px] border-0">
                    <EmptyHeader>
                      <EmptyMedia variant="icon">
                        <FileArchive />
                      </EmptyMedia>
                      <EmptyTitle>
                        {programs.length ? copy.programs.searchEmptyTitle : copy.programs.emptyTitle}
                      </EmptyTitle>
                      <EmptyDescription>
                        {programs.length ? copy.programs.searchEmptyDescription : copy.programs.emptyDescription}
                      </EmptyDescription>
                    </EmptyHeader>
                  </Empty>
                ) : (
                  <div className="overflow-hidden border bg-card">
                    {filteredPrograms.flatMap((program) => {
                      const appName = getProgramName(program)
                      const info = program.info
                      return program.versions.map((version) => {
                        const key = versionKey(program.appId, version.number)
                        const isRunning = runningKey === key
                        const isSelected = selected?.appId === program.appId
                          && selected.version === version.number
                        const decompileLabel = version.decompiled
                          ? copy.actions.recompile
                          : copy.actions.startDecompile

                        return (
                          <div
                            key={key}
                            role="button"
                            tabIndex={0}
                            aria-pressed={isSelected}
                            aria-label={copy.programs.selectProgram.replace('{name}', appName)}
                            className={cn(
                              'group @container/mini-program-row flex min-w-0 items-center gap-3 border-b px-3 py-3 last:border-b-0 hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset',
                              isSelected && 'bg-primary/10 hover:bg-primary/15',
                            )}
                            onClick={() => void handleSelectVersion(program.appId, version)}
                            onKeyDown={(event) => {
                              if (event.key === 'Enter' || event.key === ' ') {
                                event.preventDefault()
                                void handleSelectVersion(program.appId, version)
                              }
                            }}
                          >
                            <Avatar className="size-10 shrink-0 rounded-lg">
                              {info?.avatar && <AvatarImage src={info.avatar} alt={appName} />}
                              <AvatarFallback className="rounded-lg bg-muted text-xs font-medium">
                                {getProgramAvatarFallback(program)}
                              </AvatarFallback>
                            </Avatar>

                            <div className="min-w-0 flex-1">
                              <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-1">
                                <span className="min-w-0 shrink truncate text-sm font-semibold leading-5">{appName}</span>
                                <div className="flex shrink-0 items-center gap-1 @max-md/mini-program-row:order-3 @max-md/mini-program-row:basis-full">
                                  <Badge
                                    variant="outline"
                                    title={copy.programs.latestVersion.replace('{version}', version.number)}
                                    className="h-5 gap-1 px-1.5 text-[10px] font-medium leading-none tabular-nums"
                                  >
                                    <Package className="size-3" />
                                    v{version.number}
                                  </Badge>
                                  <Badge
                                    variant="secondary"
                                    title={copy.programs.packageCount.replace('{count}', String(version.wxapkgCount))}
                                    className="h-5 gap-1 px-1.5 text-[10px] font-medium leading-none tabular-nums"
                                  >
                                    <FileArchive className="size-3" />
                                    {version.wxapkgCount}
                                  </Badge>
                                </div>
                                {info?.username && (
                                  <span className="shrink-0 truncate text-xs text-muted-foreground">
                                    @{info.username}
                                  </span>
                                )}
                              </div>
                              <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                                {program.appId}
                              </p>
                              <div className="mt-1 flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
                                <span className="truncate">{version.updateDate}</span>
                                <span aria-hidden="true">·</span>
                                <span className="shrink-0">
                                  {version.decompiled ? copy.status.decompiled : copy.status.waiting}
                                </span>
                              </div>
                            </div>

                            <div className="flex shrink-0 items-center gap-1.5">
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    type="button"
                                    size="icon-sm"
                                    disabled={Boolean(runningKey)}
                                    aria-label={decompileLabel}
                                    onClick={(event) => {
                                      event.stopPropagation()
                                      void handleDecompile(program.appId, version.number)
                                    }}
                                  >
                                    {isRunning ? <Spinner /> : version.decompiled ? <RotateCcw /> : <Play />}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>{decompileLabel}</TooltipContent>
                              </Tooltip>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    type="button"
                                    variant="outline"
                                    size="icon-sm"
                                    disabled={!version.outputDir}
                                    aria-label={copy.actions.openFolder}
                                    onClick={(event) => {
                                      event.stopPropagation()
                                      void handleOpenFolder(version.outputDir)
                                    }}
                                  >
                                    <FolderOpen />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>{copy.actions.openFolder}</TooltipContent>
                              </Tooltip>
                            </div>
                          </div>
                        )
                      })
                    })}
                  </div>
                )}
              </ScrollArea>
            </div>
          </ResizablePanel>

          <ResizableHandle withHandle />

          <ResizablePanel defaultSize={62} minSize={36}>
            <div className="flex h-full min-h-0 flex-col">
              <div className="flex h-12 shrink-0 items-center justify-between gap-3 border-b bg-muted/40 px-4">
                <div className="flex min-w-0 items-center gap-2 font-medium">
                  <span className="truncate">{copy.results.title}</span>
                  {lastResult && (
                    <Badge variant="outline" className="shrink-0 font-normal text-muted-foreground">
                      {copy.results.duration.replace('{duration}', String(lastResult.durationMs))}
                    </Badge>
                  )}
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={activeResultItems.length === 0}
                  onClick={() => void handleCopy(activeResultItems.map((item) => item.value).join('\n'))}
                >
                  <Copy data-icon="inline-start" />
                  {copy.actions.copy}
                </Button>
              </div>

              {selected && matchesResult.groups.length ? (
                <Tabs
                  value={effectiveActiveResultTab}
                  onValueChange={setActiveResultTab}
                  className="min-h-0 flex-1 gap-0"
                >
                  <div className="shrink-0 border-b px-4 py-2">
                    <div className="min-w-0 flex-1 overflow-x-auto">
                      <TabsList className="h-9 w-max min-w-full justify-start">
                        {matchesResult.groups.map((group) => (
                          <TabsTrigger key={group.id} value={group.id} className="flex-none gap-1.5 px-3">
                            {group.id === 'unclassified' ? copy.results.unclassified : group.name}
                            <Badge
                              variant="secondary"
                              className="h-5 min-w-5 justify-center rounded-full px-1.5 text-[11px] font-medium tabular-nums"
                            >
                              {group.matches.length}
                            </Badge>
                          </TabsTrigger>
                        ))}
                      </TabsList>
                    </div>
                  </div>

                  <TabsContent value={effectiveActiveResultTab} className="mt-0 flex min-h-0 flex-1 flex-col">
                    <ScrollArea className="min-h-0 flex-1">
                      {activeResultItems.length ? (
                        <div className="divide-y">
                          {activeResultItems.map((item) => (
                            <div key={item.value} className="flex flex-col gap-2 px-4 py-3 hover:bg-muted/30">
                              <code className="whitespace-pre-wrap break-all font-mono text-sm leading-6">
                                {item.value}
                              </code>
                            </div>
                          ))}
                        </div>
                      ) : (
                        <Empty className="min-h-[320px] border-0">
                          <EmptyHeader>
                            <EmptyMedia variant="icon">
                              <Search />
                            </EmptyMedia>
                            <EmptyTitle>{copy.results.emptyMatchesTitle}</EmptyTitle>
                            <EmptyDescription>
                              {activeResultGroup
                                ? copy.results.emptyGroupDescription.replace(
                                    '{group}',
                                    activeResultGroup.id === 'unclassified'
                                      ? copy.results.unclassified
                                      : activeResultGroup.name,
                                  )
                                : copy.results.emptyMatchesDescription}
                            </EmptyDescription>
                          </EmptyHeader>
                        </Empty>
                      )}
                    </ScrollArea>
                  </TabsContent>
                </Tabs>
              ) : selected ? (
                <Empty className="h-full border-0">
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <Search />
                    </EmptyMedia>
                    <EmptyTitle>{copy.results.emptyMatchesTitle}</EmptyTitle>
                    <EmptyDescription>{copy.results.emptyMatchesDescription}</EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                <Empty className="h-full border-0">
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <FileArchive />
                    </EmptyMedia>
                    <EmptyTitle>{copy.results.placeholderTitle}</EmptyTitle>
                    <EmptyDescription>{copy.results.placeholderDescription}</EmptyDescription>
                  </EmptyHeader>
                  <EmptyContent>
                    <Button onClick={() => void refreshPrograms()} disabled={loading}>
                      {loading ? <Spinner data-icon="inline-start" /> : <RefreshCw data-icon="inline-start" />}
                      {copy.controls.refresh}
                    </Button>
                  </EmptyContent>
                </Empty>
              )}
            </div>
          </ResizablePanel>
        </ResizablePanelGroup>

        <Dialog open={rulesOpen} onOpenChange={setRulesOpen}>
          <DialogContent className="flex h-[min(88vh,900px)] flex-col gap-0 overflow-hidden p-0 sm:max-w-5xl">
            <DialogHeader className="shrink-0 px-6 pb-4 pt-6">
              <DialogTitle>{copy.rules.title}</DialogTitle>
            </DialogHeader>

            <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-y bg-muted/20 px-6 py-3">
              <FieldDescription>
                {copy.rules.summary
                  .replace('{groups}', String(ruleGroupsDraft.length))
                  .replace(
                    '{rules}',
                    String(ruleGroupsDraft.reduce((total, group) => total + group.rules.length, 0)),
                  )}
              </FieldDescription>
              <Button type="button" size="sm" onClick={addRuleGroup}>
                <Plus data-icon="inline-start" />
                {copy.rules.addGroup}
              </Button>
            </div>

            <ScrollArea className="min-h-0 flex-1">
              <div className="flex flex-col gap-3 p-6">
                {ruleGroupsDraft.map((group) => (
                  <section key={group.id} className="overflow-hidden rounded-lg border bg-card">
                    <div className="flex flex-wrap items-center gap-3 border-b bg-muted/20 p-3">
                      <Switch
                        checked={group.enabled}
                        onCheckedChange={(enabled) => updateRuleGroup(group.id, { enabled })}
                        aria-label={copy.rules.toggleGroup.replace('{group}', group.name)}
                      />
                      <Input
                        value={group.name}
                        onChange={(event) => updateRuleGroup(group.id, { name: event.target.value })}
                        className="h-9 min-w-[220px] flex-1 font-medium"
                        aria-label={copy.rules.groupName}
                      />
                      <Badge variant={group.enabled ? 'secondary' : 'outline'}>
                        {group.enabled ? copy.rules.enabled : copy.rules.disabled}
                      </Badge>
                      <Badge variant="outline">
                        {copy.rules.count.replace('{count}', String(group.rules.length))}
                      </Badge>
                      <Button type="button" variant="outline" size="sm" onClick={() => addRule(group.id)}>
                        <Plus data-icon="inline-start" />
                        {copy.rules.addRule}
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => removeRuleGroup(group.id)}
                        aria-label={copy.rules.deleteGroup.replace('{group}', group.name)}
                        className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                      >
                        <Trash2 />
                      </Button>
                    </div>

                    {group.rules.length ? (
                      <div className="divide-y">
                        {group.rules.map((rule, ruleIndex) => (
                          <div key={rule.id} className="flex items-start gap-3 p-3">
                            <Switch
                              checked={rule.enabled}
                              onCheckedChange={(enabled) => updateRule(group.id, rule.id, { enabled })}
                              aria-label={copy.rules.toggleRule.replace('{index}', String(ruleIndex + 1))}
                              className="mt-2"
                            />
                            <Field className="min-w-0 flex-1 gap-1.5">
                              <FieldLabel className="text-xs text-muted-foreground">
                                {copy.rules.ruleLabel.replace('{index}', String(ruleIndex + 1))}
                              </FieldLabel>
                              <Textarea
                                value={rule.pattern}
                                onChange={(event) => updateRule(group.id, rule.id, { pattern: event.target.value })}
                                className="min-h-20 resize-y font-mono text-xs leading-5"
                                placeholder={copy.rules.placeholder}
                              />
                            </Field>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => removeRule(group.id, rule.id)}
                              aria-label={copy.rules.deleteRule.replace('{index}', String(ruleIndex + 1))}
                              className="mt-6 text-destructive hover:bg-destructive/10 hover:text-destructive"
                            >
                              <Trash2 />
                            </Button>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="flex min-h-24 flex-col items-center justify-center gap-2 p-4 text-center text-sm text-muted-foreground">
                        <span>{copy.rules.emptyGroup}</span>
                        <Button type="button" variant="outline" size="sm" onClick={() => addRule(group.id)}>
                          <Plus data-icon="inline-start" />
                          {copy.rules.addRule}
                        </Button>
                      </div>
                    )}
                  </section>
                ))}

                {ruleGroupsDraft.length === 0 && (
                  <div className="flex min-h-56 flex-col items-center justify-center gap-3 rounded-lg border border-dashed text-center text-sm text-muted-foreground">
                    <span>{copy.rules.emptyGroups}</span>
                    <Button type="button" variant="outline" onClick={addRuleGroup}>
                      <Plus data-icon="inline-start" />
                      {copy.rules.addGroup}
                    </Button>
                  </div>
                )}
              </div>
            </ScrollArea>

            <DialogFooter className="shrink-0 border-t bg-background px-6 py-4 sm:justify-between">
              <Button
                type="button"
                variant="outline"
                onClick={() => setRuleGroupsDraft(cloneRuleGroups(defaultRuleGroups))}
                disabled={defaultRuleGroups.length === 0}
              >
                <RotateCcw data-icon="inline-start" />
                {copy.rules.restoreDefaults}
              </Button>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setRulesOpen(false)}>
                  {copy.controls.cancel}
                </Button>
                <Button onClick={() => void saveRules()} disabled={saving}>
                  {saving ? <Spinner data-icon="inline-start" /> : <CheckCircle2 data-icon="inline-start" />}
                  {copy.controls.save}
                </Button>
              </div>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </TooltipProvider>
  )
}
