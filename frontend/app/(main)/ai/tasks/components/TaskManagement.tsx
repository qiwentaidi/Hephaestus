'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { usePathname, useRouter, useSearchParams } from 'next/navigation'
import { Copy, FileText, MessageSquare, MoreHorizontal, Pause, Play, Plus, RefreshCw, Search, Trash2, X } from 'lucide-react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { MultiSelectFilter } from '@/components/ui/multi-select-filter'
import { Badge } from '@/components/ui/badge'
import { Spinner } from '@/components/ui/spinner'
import { Empty, EmptyDescription } from '@/components/ui/empty'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { getStatusBadge } from '@/lib/status-badge'
import { PlanStatusBadge } from '@/components/plan-status-badge'
import { useI18n } from '@/lib/i18n'
import { listAllAIWorkbenchPenTestProjects, type AIWorkbenchProject } from '@/lib/api/ai-workbench'
import { actionAIBatchQueues, deleteAIBatchQueue, listAIBatchQueues, retryAIBatchItem, type AIBatchQueue, type BatchStatus } from '@/lib/api/ai-batch'
import { deleteAIPlanRun, pauseAIPlanRun, resumeAIPlanRun, listAIPlanRunSummaries, listAIPlanItemRows, type AIPlanItemRows, type AIPlanRunSummary, type AIPlanRunListItem } from '@/lib/api/ai-plan'
import { batchDetailHref, canPauseQueue, canResumeQueue, canRetryQueue, deleteQueueSelection, isActiveQueue, summarizeBatch } from '@/lib/ai-batch-view'
import { canDeletePlanRun, canPausePlanRun, canResumePlanRun } from '@/lib/ai-plan-view'
import { cn } from '@/lib/utils'
import { BatchTaskDetail } from './BatchTaskDetail'
import { TaskCreateDialog } from './TaskCreateDialog'
import { PlanTaskDetail, EmptyPlanDetail } from './PlanTaskDetail'
import { ProjectStorageDialog } from './ProjectStorageDialog'

type UnifiedTask = { kind: 'batch' | 'plan'; key: string; id: string; name: string; projectId: string; status: string; updatedAt: string; queue?: AIBatchQueue; plan?: AIPlanRunSummary }
type DeleteTarget = { key: string; kind: 'batch' | 'plan'; id: string; name: string; status: string; active: boolean }
type PlanDeleteResult = { key: string; name: string; error?: string }

const taskRefreshIntervalMs = 5000

function taskStatusGroup(kind: 'batch' | 'plan', status: string) {
  if (kind === 'plan') return ['planning', 'waiting', 'pausing'].includes(status) ? 'active' : status
  if (status === 'pending' || status === 'running' || status === 'pausing') return 'active'
  if (status === 'completed' || status === 'completed_with_errors') return 'completed'
  return status
}

export default function TaskManagement() {
  const { t, language } = useI18n(); const l = t.aiBatch; const p = t.aiPlan
  const pathname = usePathname(); const params = useSearchParams()
  const [projects, setProjects] = useState<AIWorkbenchProject[]>([])
  const [queues, setQueues] = useState<AIBatchQueue[]>([])
  const [plans, setPlans] = useState<AIPlanRunSummary[]>([])
  const [selectedTaskKey, setSelectedTaskKey] = useState(() => params.get('run') ? `plan:${params.get('run')}` : params.get('queue') ? `batch:${params.get('queue')}` : '')
  // Workbench guard deep link: /ai/tasks?create=plan&source=<native task id>
  // opens the orchestration form with that run preselected as the read-only
  // source. null means the dialog was not opened by the deep link.
  const [deepLinkPlanSource, setDeepLinkPlanSource] = useState<string[] | null>(() => {
    if (params.get('create') !== 'plan') return null
    const source = (params.get('source') || '').trim()
    return source ? [source] : []
  })
  const [planAction, setPlanAction] = useState<{ id: string; action: 'pause' | 'resume' } | null>(null)
  const planActionLock = useRef(false)
  const [selected, setSelected] = useState<string[]>([])
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [kindFilter, setKindFilter] = useState('all')
  const [creating, setCreating] = useState(false)
  const [copySource, setCopySource] = useState<AIBatchQueue>()
  const [copyPlanSource, setCopyPlanSource] = useState<AIPlanRunListItem>()
  const [busy, setBusy] = useState(false)
  const [listsLoaded, setListsLoaded] = useState(false)
  const [layoutReady, setLayoutReady] = useState(false)
  const [planRowsState, setPlanRowsState] = useState<{
    plan: AIPlanRunSummary
    result?: { id: string; data: AIPlanItemRows }
    error?: string
  }>()
  const [refreshing, setRefreshing] = useState(false)
  const [loadError, setLoadError] = useState('')
  const [error, setError] = useState('')
  const [deleteKeys, setDeleteKeys] = useState<string[]>([])
  const [actionErrors, setActionErrors] = useState<string[]>([])
  const [storageProject, setStorageProject] = useState<{ id: string; name: string } | null>(null)
  const mounted = useRef(true)
  const listRequest = useRef<Promise<void> | null>(null)
  const listAbort = useRef<AbortController | null>(null)
  const rowsAbort = useRef<AbortController | null>(null)
  const lastRefreshAt = useRef(0)
  const searchRef = useRef('')
  const [loadedSearch, setLoadedSearch] = useState('')
  const detailScroll = useRef<HTMLDivElement>(null)
  const router = useRouter()
  const closeCreateDialog = useCallback(() => {
    setCreating(false)
    setCopySource(undefined)
    setCopyPlanSource(undefined)
    if (deepLinkPlanSource) {
      setDeepLinkPlanSource(null)
      router.replace(pathname)
    }
  }, [deepLinkPlanSource, pathname, router])
  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      listAbort.current?.abort()
    }
  }, [])
  const refresh = useCallback(async (force = false) => {
    const pending = listRequest.current
    if (pending) {
      if (!force) return
      await pending
      const replacement = listRequest.current
      if (replacement) return replacement
    }
    if (!mounted.current) return
    const controller = new AbortController()
    listAbort.current = controller
    const { signal } = controller
    const search = searchRef.current
    setRefreshing(true)
    const request = Promise.allSettled([listAIBatchQueues(signal), listAIPlanRunSummaries(search, signal)]).then(([batch, plan]) => {
      if (signal.aborted) return
      if (batch.status === 'fulfilled') {
        setQueues(batch.value)
      }
      if (plan.status === 'fulfilled') {
        setPlans(plan.value)
        setLoadedSearch(search)
      }
      if (batch.status === 'fulfilled' && plan.status === 'fulfilled') {
        setListsLoaded(true)
        setLoadError('')
      } else {
        const reason = batch.status === 'rejected' ? batch.reason : plan.status === 'rejected' ? plan.reason : ''
        setLoadError(reason instanceof Error ? reason.message : String(reason))
      }
    }).finally(() => {
      listRequest.current = null
      if (!signal.aborted) {
        lastRefreshAt.current = Date.now()
        setRefreshing(false)
      }
    })
    listRequest.current = request
    await request
  }, [])
  const refreshProjects = useCallback(async () => {
    try {
      const data = await listAllAIWorkbenchPenTestProjects()
      if (mounted.current) setProjects(data)
    } catch (e) { if (mounted.current) toast.error(e instanceof Error ? e.message : String(e)) }
  }, [])
  useEffect(() => {
    const initial = setTimeout(() => { void refresh(); void refreshProjects() }, 0)
    const onReturn = () => {
      if (!document.hidden && !rowsAbort.current && Date.now() - lastRefreshAt.current >= taskRefreshIntervalMs) {
        void refresh()
      }
    }
    window.addEventListener('focus', onReturn)
    document.addEventListener('visibilitychange', onReturn)
    return () => {
      clearTimeout(initial)
      window.removeEventListener('focus', onReturn)
      document.removeEventListener('visibilitychange', onReturn)
    }
  }, [refresh, refreshProjects])
  const search = query.trim()
  useEffect(() => {
    if (searchRef.current === search) return
    searchRef.current = search
    const timer = setTimeout(() => void refresh(true), 250)
    return () => clearTimeout(timer)
  }, [refresh, search])

  const projectName = (projectId: string) => projects.find(p => p.id === projectId)?.name || l.unknownProject
  const needle = loadedSearch.toLocaleLowerCase()
  const unifiedTasks: UnifiedTask[] = [
    ...queues.map(queue => ({
      kind: 'batch' as const, key: `batch:${queue.id}`, id: queue.id, name: queue.name,
      projectId: queue.projectId, status: queue.status, updatedAt: queue.updatedAt || queue.createdAt, queue,
    })),
    ...plans.map(plan => ({
      kind: 'plan' as const, key: `plan:${plan.run.id}`, id: plan.run.id,
      name: plan.project?.name || projectName(plan.run.projectId), projectId: plan.run.projectId,
      status: plan.run.status, updatedAt: plan.run.updatedAt || plan.run.createdAt, plan,
    })),
  ]
  const taskText = (task: UnifiedTask) => task.kind === 'batch'
    ? [task.name, projectName(task.projectId), ...(task.queue?.items || []).map(item => item.instruction)]
    : [task.name, task.plan?.run.objective, projectName(task.projectId)]
  const visibleTasks = unifiedTasks.filter(task => {
    if (kindFilter !== 'all' && task.kind !== kindFilter) return false
    if (statusFilter.length > 0 && !statusFilter.includes(taskStatusGroup(task.kind, task.status))) return false
    return !needle || task.plan?.planItemMatch || taskText(task).filter(Boolean).join(' ').toLocaleLowerCase().includes(needle)
  })
  const selectedTask = visibleTasks.find(task => task.key === selectedTaskKey) || visibleTasks[0]
  const detail = selectedTask?.queue
  const selectedPlan = selectedTask?.plan
  const planRows = planRowsState?.result?.id === selectedPlan?.run.id ? planRowsState?.result?.data : undefined
  const planRowsError = planRowsState?.plan === selectedPlan ? planRowsState?.error : undefined
  const rowsLoading = Boolean(listsLoaded && selectedPlan && planRowsState?.plan !== selectedPlan)
  useEffect(() => {
    if (!listsLoaded || !selectedPlan) return
    const id = selectedPlan.run.id
    const controller = new AbortController()
    rowsAbort.current = controller
    void listAIPlanItemRows(id, '', controller.signal).then(data => {
      if (controller.signal.aborted) return
      setPlanRowsState({ plan: selectedPlan, result: { id, data } })
    }).catch(reason => {
      if (!controller.signal.aborted) setPlanRowsState(previous => ({
        plan: selectedPlan,
        result: previous?.result?.id === id ? previous.result : undefined,
        error: reason instanceof Error ? reason.message : String(reason),
      }))
    }).finally(() => {
      if (controller.signal.aborted) return
      rowsAbort.current = null
      lastRefreshAt.current = Date.now()
    })
    return () => {
      controller.abort()
      rowsAbort.current = null
    }
  }, [listsLoaded, selectedPlan])
  const hasActiveTasks = queues.some(isActiveQueue) || plans.some(({ run }) => taskStatusGroup('plan', run.status) === 'active')
  useEffect(() => {
    if (!listsLoaded || !hasActiveTasks || refreshing || rowsLoading) return
    const timer = setTimeout(() => { if (!document.hidden) void refresh() }, taskRefreshIntervalMs)
    return () => clearTimeout(timer)
  }, [hasActiveTasks, listsLoaded, plans, refresh, refreshing, rowsLoading])
  const firstViewReady = listsLoaded && (!selectedPlan || Boolean(planRows))
  if (firstViewReady && !layoutReady) setLayoutReady(true)
  const showLayout = layoutReady || firstViewReady
  const firstLoadError = !showLayout && (loadError || planRowsError)
  const validSelection = selected.filter(key => key.startsWith('batch:')
    ? queues.some(queue => `batch:${queue.id}` === key)
    : plans.some(plan => `plan:${plan.run.id}` === key))
  const selectedKeySet = new Set(validSelection)
  const selectedQueues = queues.filter(q => selectedKeySet.has(`batch:${q.id}`))
  const selectedPlans = plans.filter(p => selectedKeySet.has(`plan:${p.run.id}`))
  const deletableQueues = selectedQueues.filter(q => !isActiveQueue(q))
  const deletablePlans = selectedPlans.filter(p => canDeletePlanRun(p.run))
  const deletableKeys = [
    ...deletableQueues.map(q => `batch:${q.id}`),
    ...deletablePlans.map(p => `plan:${p.run.id}`),
  ]
  const deleteTargets: DeleteTarget[] = []
  for (const key of deleteKeys) {
    if (key.startsWith('batch:')) {
      const queue = queues.find(q => `batch:${q.id}` === key)
      if (!queue) continue
      deleteTargets.push({ key, kind: 'batch', id: queue.id, name: queue.name, status: queue.status, active: isActiveQueue(queue) })
    } else if (key.startsWith('plan:')) {
      const plan = plans.find(p => `plan:${p.run.id}` === key)
      if (!plan) continue
      deleteTargets.push({ key, kind: 'plan', id: plan.run.id, name: plan.project?.name || projectName(plan.run.projectId), status: plan.run.status, active: !canDeletePlanRun(plan.run) })
    }
  }
  const activeDeleteTargets = deleteTargets.some(target => target.active)
  const visibleTaskKeys = visibleTasks.map(task => task.key)
  const allVisibleSelected = visibleTaskKeys.length > 0 && visibleTaskKeys.every(key => validSelection.includes(key))
  const toggleVisibleSelection = (checked: boolean | 'indeterminate') => {
    setSelected(previous => {
      if (checked === true) return [...new Set([...previous, ...visibleTaskKeys])]
      return previous.filter(key => !visibleTaskKeys.includes(key))
    })
  }
  const date = (value?: string) => value ? new Date(value).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—'
  const countLabel = (label: string, count: number) => label.replace('{count}', String(count))
  const navigate = (queueId: string) => {
    const url = new URLSearchParams(params.toString())
    url.delete('run'); url.delete('view')
    window.history.replaceState(null, '', batchDetailHref(pathname, url.toString(), queueId))
  }
  const chooseQueue = (queue: AIBatchQueue) => {
    setSelectedTaskKey(`batch:${queue.id}`)
    navigate(queue.id)
    detailScroll.current?.scrollTo({ top: 0 })
  }
  const choosePlan = (plan: AIPlanRunListItem) => {
    setSelectedTaskKey(`plan:${plan.run.id}`)
    const url = new URLSearchParams(params.toString())
    url.delete('queue'); url.delete('item'); url.delete('items'); url.delete('view')
    url.set('run', plan.run.id)
    window.history.replaceState(null, '', `${pathname}?${url}`)
    detailScroll.current?.scrollTo({ top: 0 })
  }
  const actOnPlan = async (id: string, action: 'pause' | 'resume') => {
    if (planActionLock.current) return
    planActionLock.current = true
    setPlanAction({ id, action }); setError('')
    try {
      if (action === 'pause') {
        const next = await pauseAIPlanRun(id)
        toast.success(next.run.status === 'paused' ? p.pauseDone : p.pauseSubmitted)
      } else {
        await resumeAIPlanRun(id)
        toast.success(p.resumeSubmitted)
      }
      await refresh(true)
    } catch (e) { if (mounted.current) setError(e instanceof Error ? e.message : String(e)) }
    finally { planActionLock.current = false; if (mounted.current) setPlanAction(null) }
  }
  const badge = (status: BatchStatus) => getStatusBadge(status === 'completed_with_errors' ? 'completed' : status, { ...l.status, cancelled: l.cancel })
  const act = async (ids: string[], action: 'pause' | 'resume' | 'retry_failed') => {
    if (!ids.length || busy) return
    setBusy(true); setActionErrors([])
    try {
      const results = await actionAIBatchQueues(ids, action)
      const failures = Object.entries(results).filter(([, message]) => message).map(([key, message]) => `${queues.find(q => q.id === key)?.name || key}: ${message}`)
      setActionErrors(failures)
      if (!failures.length) toast.success(action === 'retry_failed' ? l.retryFailedStarted : action === 'pause' ? l.pauseSubmitted : l.operationDone)
      await refresh(true)
    } catch (e) { setActionErrors([e instanceof Error ? e.message : l.error]) }
    finally { setBusy(false) }
  }
  const remove = async () => {
    if (busy || !deleteKeys.length || activeDeleteTargets || !deleteTargets.length) return
    setBusy(true)
    const currentSelectedKey = selectedTask?.key
    const batchTargets = deleteTargets.filter(target => target.kind === 'batch')
    const planTargets = deleteTargets.filter(target => target.kind === 'plan')
    const batchResults = await deleteQueueSelection(
      queues,
      batchTargets.map(target => target.id),
      deleteAIBatchQueue,
      l.deleteStopped,
    )
    const planResults = await Promise.all(planTargets.map(async (target): Promise<PlanDeleteResult> => {
      try {
        await deleteAIPlanRun(target.id)
        return { key: target.key, name: target.name }
      } catch (error) {
        return { key: target.key, name: target.name, error: error instanceof Error ? error.message : String(error) }
      }
    }))
    const removedKeys = [
      ...batchResults.filter(result => !result.error).map(result => `batch:${result.id}`),
      ...planResults.filter(result => !result.error).map(result => result.key),
    ]
    const failures = [
      ...batchResults.filter(result => result.error).map(result => ({ name: result.name, error: result.error || '' })),
      ...planResults.filter(result => result.error).map(result => ({ name: result.name, error: result.error || '' })),
    ]
    const summary = [
      removedKeys.length > 0 && countLabel(l.deleteSuccessCount, removedKeys.length),
      failures.length > 0 && countLabel(l.deleteFailureCount, failures.length),
    ].filter(Boolean).join(' · ')
    if (failures.length) {
      const details = failures.slice(0, 3).map(result => `${result.name}: ${result.error}`).join('\n')
      toast.error(`${summary}\n${details}`, { duration: 6000 })
    } else {
      toast.success(summary)
    }
    setDeleteKeys([])
    setQueues(prev => prev.filter(queue => !removedKeys.includes(`batch:${queue.id}`)))
    setPlans(prev => prev.filter(plan => !removedKeys.includes(`plan:${plan.run.id}`)))
    setSelected(prev => prev.filter(key => !removedKeys.includes(key)))
    if (currentSelectedKey && removedKeys.includes(currentSelectedKey)) {
      const next = new URLSearchParams(params.toString())
      next.delete('run'); next.delete('queue'); next.delete('item'); next.delete('items'); next.delete('view')
      window.history.replaceState(null, '', `${pathname}${next.size ? `?${next}` : ''}`)
    }
    await refresh(true)
    setBusy(false)
  }

  const retryItem = async (queueId: string, itemId: string) => {
    if (busy) return
    setBusy(true)
    try { await retryAIBatchItem(queueId, itemId); toast.success(l.retryHint); await refresh(true) }
    catch (e) { toast.error(e instanceof Error ? e.message : l.error) }
    finally { setBusy(false) }
  }
  const openCreate = (source?: AIBatchQueue) => { setCopySource(source); setCopyPlanSource(undefined); setCreating(true) }
  const copyPlan = (plan: AIPlanRunListItem) => { setCopySource(undefined); setCopyPlanSource(plan); setCreating(true) }
  const queueMenu = (queue: AIBatchQueue) => <DropdownMenu modal={false}>
    <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="size-8 shrink-0" disabled={busy} aria-label={`${l.more}: ${queue.name}`}><MoreHorizontal className="size-4" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem onSelect={() => openCreate(queue)}><Copy />{l.copyTask}</DropdownMenuItem>
      <DropdownMenuItem disabled={!queue.projectId} onSelect={() => setStorageProject({ id: queue.projectId, name: projectName(queue.projectId) })}><FileText />{l.projectStorage}</DropdownMenuItem>
      {canPauseQueue(queue) && <DropdownMenuItem onSelect={() => void act([queue.id], 'pause')}><Pause />{l.pause}</DropdownMenuItem>}
      {canResumeQueue(queue) && <DropdownMenuItem onSelect={() => void act([queue.id], 'resume')}><Play />{queue.status === 'pending' ? l.start : l.resume}</DropdownMenuItem>}
      {canRetryQueue(queue) && <DropdownMenuItem onSelect={() => void act([queue.id], 'retry_failed')}><RefreshCw />{l.retryFailed}</DropdownMenuItem>}
      <DropdownMenuSeparator />
      <DropdownMenuItem variant="destructive" disabled={isActiveQueue(queue)} onSelect={() => setDeleteKeys([`batch:${queue.id}`])}><Trash2 />{l.delete}</DropdownMenuItem>
      {isActiveQueue(queue) && <p className="max-w-60 px-2 py-1.5 text-xs leading-5 text-muted-foreground">{l.deleteStopped}</p>}
    </DropdownMenuContent>
  </DropdownMenu>
  const planMenu = (plan: AIPlanRunListItem) => <DropdownMenu modal={false}>
    <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="size-8 shrink-0" aria-label={`${l.more}: ${plan.project?.name || plan.run.id}`}><MoreHorizontal className="size-4" /></Button></DropdownMenuTrigger>
    <DropdownMenuContent align="end">
      <DropdownMenuItem onSelect={() => copyPlan(plan)}><Copy />{l.copyTask}</DropdownMenuItem>
      <DropdownMenuItem asChild><Link href={`/ai/workbench/project?project=${encodeURIComponent(plan.run.projectId)}&tab=plan`}><FileText />{p.openProject}</Link></DropdownMenuItem>
      <DropdownMenuItem asChild><Link href={`/ai/workbench?session=${encodeURIComponent(plan.run.plannerConversationId)}`}><MessageSquare />{p.openPlanner}</Link></DropdownMenuItem>
      <DropdownMenuItem disabled={!plan.run.projectId} onSelect={() => setStorageProject({ id: plan.run.projectId, name: plan.project?.name || projectName(plan.run.projectId) })}><FileText />{l.projectStorage}</DropdownMenuItem>
      {canPausePlanRun(plan.run) && <><DropdownMenuSeparator /><DropdownMenuItem onSelect={() => void actOnPlan(plan.run.id, 'pause')} disabled={Boolean(planAction)}><Pause />{planAction?.id === plan.run.id ? p.statuses.pausing : p.pause}</DropdownMenuItem></>}
      {canResumePlanRun(plan.run) && <><DropdownMenuSeparator /><DropdownMenuItem onSelect={() => void actOnPlan(plan.run.id, 'resume')} disabled={Boolean(planAction)}><Play />{planAction?.id === plan.run.id ? p.resuming : p.resume}</DropdownMenuItem></>}
      {plan.run.status === 'pausing' && <DropdownMenuItem disabled><Pause />{p.statuses.pausing}</DropdownMenuItem>}
      <DropdownMenuSeparator />
      <DropdownMenuItem variant="destructive" disabled={busy || !canDeletePlanRun(plan.run)} onSelect={() => setDeleteKeys([`plan:${plan.run.id}`])}><Trash2 />{p.delete}</DropdownMenuItem>
      {!canDeletePlanRun(plan.run) && <p className="max-w-60 px-2 py-1.5 text-xs leading-5 text-muted-foreground">{p.deleteActive}</p>}
    </DropdownMenuContent>
  </DropdownMenu>

  return <div className="flex h-full min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-hidden p-4 md:p-5" data-testid="task-management">
    <header className="flex shrink-0 flex-wrap items-center gap-2">
      <div className="mr-auto flex min-w-0 items-baseline gap-2">
        <h1 className="text-lg font-semibold tracking-tight">{l.title}</h1>
      </div>
      <div className="flex min-w-0 flex-1 flex-wrap items-center justify-end gap-2">
        <div className="relative w-full min-w-0 sm:w-56 xl:w-60"><Search className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" aria-hidden /><Input aria-label={l.search} placeholder={l.search} value={query} onChange={e => setQuery(e.target.value)} className="pl-9" /></div>
        <MultiSelectFilter
          values={statusFilter}
          options={Object.entries(l.taskStatus).map(([value, label]) => ({ value, label }))}
          allLabel={l.allStates}
          countLabel={t.common.selectedCount}
          removeLabel={t.common.remove}
          clearLabel={t.common.clear}
          ariaLabel={l.allStates}
          onChange={setStatusFilter}
          className="w-32"
        />
        <Button variant="ghost" size="icon" aria-label={l.refresh} title={l.refresh} disabled={refreshing || rowsLoading} onClick={() => void refresh(true)}><RefreshCw className={cn('size-4', (refreshing || rowsLoading) && 'motion-safe:animate-spin')} /></Button>
        <Button disabled={busy} onClick={() => openCreate()}><Plus className="size-4" />{l.newQueue}</Button>
      </div>
    </header>
    {showLayout && loadError && <Alert variant="destructive"><AlertDescription>{loadError}</AlertDescription></Alert>}
    {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
    {actionErrors.length > 0 && <Alert variant="destructive"><div className="flex items-start justify-between gap-2"><AlertDescription><strong>{l.operationResults}</strong>{actionErrors.map((message, index) => <p key={index} className="break-words">{message}</p>)}</AlertDescription><Button variant="ghost" size="icon" aria-label={l.dismiss} onClick={() => setActionErrors([])}><X className="size-4" /></Button></div></Alert>}
    {!showLayout ? <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 p-4 text-center">
      {firstLoadError && !refreshing && !rowsLoading ? <>
        <p role="alert" className="text-sm text-destructive">{firstLoadError}</p>
        <Button variant="outline" size="sm" onClick={() => void refresh(true)}>{t.aiBatch.retryLoad}</Button>
      </> : <span role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><Spinner aria-hidden="true" className="motion-reduce:animate-none" />{l.loadingTasks}</span>}
    </div> : <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto xl:grid xl:grid-cols-[minmax(260px,300px)_minmax(0,1fr)] xl:overflow-hidden">
      <section aria-label={l.listTitle} className="flex min-h-0 shrink-0 flex-col rounded-xl border bg-card/30 xl:overflow-hidden">
        <div className="shrink-0 border-b p-3">
          <div className="flex min-h-8 flex-wrap items-center gap-2">
            {validSelection.length > 0 ? (
              <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2" data-testid="task-bulk-actions">
                <Checkbox checked={allVisibleSelected} onCheckedChange={toggleVisibleSelection} aria-label={l.selectVisible} />
                <span className="text-xs font-medium text-foreground">{countLabel(l.selected, validSelection.length)}</span>
                <Button size="icon-sm" variant="outline" className="text-destructive" disabled={busy || !deletableKeys.length} aria-label={countLabel(l.deleteSelected, deletableKeys.length)} title={countLabel(l.deleteSelected, deletableKeys.length)} onClick={() => setDeleteKeys(deletableKeys)}><Trash2 className="size-4" /></Button>
                {deletableKeys.length < validSelection.length && <span className="min-w-0 text-[10px] leading-4 text-muted-foreground">{countLabel(l.deleteSkipped, validSelection.length - deletableKeys.length)}</span>}
                <Button size="icon-sm" variant="ghost" className="ml-auto" aria-label={l.clearSelection} title={l.clearSelection} disabled={busy} onClick={() => setSelected([])}><X className="size-4" /></Button>
              </div>
            ) : <>
              <h2 className="flex items-center gap-2 text-sm font-semibold">{l.listTitle}<Badge variant="secondary" className="tabular-nums">{visibleTasks.length}</Badge></h2>
              <Checkbox className="ml-auto" checked={allVisibleSelected} onCheckedChange={toggleVisibleSelection} aria-label={l.selectVisible} />
            </>}
          </div>
          <div role="group" aria-label={l.listTitle} className="mt-3 flex rounded-md border p-0.5">
            {(['all', 'batch', 'plan'] as const).map(kind => <button key={kind} type="button" aria-pressed={kindFilter === kind} className={cn('min-w-0 flex-1 rounded px-2 py-1.5 text-xs focus-visible:outline-2 focus-visible:outline-foreground', kindFilter === kind ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground')} onClick={() => setKindFilter(kind)}>{kind === 'all' ? l.all : kind === 'batch' ? l.batchTab : p.tab}</button>)}
          </div>
        </div>
        <div className="max-h-[28dvh] min-h-0 flex-1 overflow-y-auto xl:max-h-none">
          {visibleTasks.map(task => {
            const isBatch = task.kind === 'batch'
            const summary = task.queue ? summarizeBatch(task.queue) : null
            const planCompleted = task.plan?.itemCounts.completed || 0
            const active = selectedTask?.key === task.key
            return <div key={task.key} className={cn('group relative flex items-start gap-2 border-b border-l-2 border-l-transparent px-3 py-3 last:border-b-0 hover:bg-muted/30', active && 'border-l-foreground bg-muted/50', selected.includes(task.key) && 'bg-muted/30')}>
              <Checkbox className="mt-1" disabled={busy} aria-label={`${l.selectQueue}: ${task.name}`} checked={selected.includes(task.key)} onCheckedChange={checked => setSelected(prev => checked === true ? [...new Set([...prev, task.key])] : prev.filter(value => value !== task.key))} />
              <button type="button" aria-current={active ? 'true' : undefined} className="min-w-0 flex-1 rounded-sm text-left focus-visible:outline-2 focus-visible:outline-foreground" onClick={() => task.queue ? chooseQueue(task.queue) : task.plan ? choosePlan(task.plan) : undefined}>
                <span className="flex min-w-0 items-center gap-2 pr-8"><Badge variant="outline" className="h-5 shrink-0 rounded px-1 text-[10px]">{isBatch ? l.batchTab : p.tab}</Badge><span className="min-w-0 flex-1 truncate text-sm font-semibold" title={task.name}>{task.name}</span></span>
                <span className="mt-2 flex flex-wrap items-center gap-2 pr-8">{isBatch ? badge(task.status as BatchStatus) : <PlanStatusBadge status={task.status} />}<span className="text-xs tabular-nums text-muted-foreground">{isBatch ? `${l.processed} ${summary?.processed || 0}/${summary?.total || 0}` : `${p.total.replace('{count}', String(task.plan?.itemTotal || 0))} · ${p.completed.replace('{count}', String(planCompleted))}`}</span></span>
                <span className="mt-1.5 flex justify-between gap-2 text-[11px] leading-5 text-muted-foreground">{task.queue && projectName(task.projectId) !== task.name && <span className="min-w-0 truncate">{projectName(task.projectId)}</span>}<time className="ml-auto shrink-0">{date(task.updatedAt)}</time></span>
              </button>
              <div className="absolute right-3 top-3">{task.queue ? queueMenu(task.queue) : task.plan ? planMenu(task.plan) : null}</div>
            </div>
          })}
          {!visibleTasks.length && <Empty className="min-h-48"><EmptyDescription>{unifiedTasks.length ? l.noMatches : l.empty}</EmptyDescription>{unifiedTasks.length > 0 && <Button variant="outline" onClick={() => { setQuery(''); setStatusFilter([]); setKindFilter('all') }}>{l.clearFilters}</Button>}</Empty>}
        </div>
      </section>
      <section aria-label={l.detail} ref={detailScroll} className="h-[max(28rem,calc(100dvh-17rem))] min-h-0 min-w-0 shrink-0 overflow-hidden rounded-xl border bg-card/30 md:h-[max(32rem,calc(100dvh-12rem))] xl:h-auto">
        {selectedPlan ? <PlanTaskDetail key={selectedPlan.run.id} item={selectedPlan} rows={planRows} error={planRowsError} refreshing={refreshing || rowsLoading} onRefresh={() => void refresh(true)} projectName={projectName(selectedPlan.run.projectId)} onPause={() => void actOnPlan(selectedPlan.run.id, 'pause')} onResume={() => void actOnPlan(selectedPlan.run.id, 'resume')} busy={Boolean(planAction)} action={planAction?.id === selectedPlan.run.id ? planAction.action : undefined} onOpenSource={sourceRunId => { const source = plans.find(plan => plan.run.id === sourceRunId); if (source) { choosePlan(source); return } window.location.assign(new URL(`/ai/tasks?run=${encodeURIComponent(sourceRunId)}`, window.location.origin).toString()) }} /> : detail ? <BatchTaskDetail key={detail.id} queue={detail} projectName={projectName(detail.projectId)} busy={busy} onRetryFailed={() => void act([detail.id], 'retry_failed')} onRetryItem={itemId => void retryItem(detail.id, itemId)} actions={<>
          {canResumeQueue(detail) && <Button size="sm" disabled={busy} onClick={() => void act([detail.id], 'resume')}><Play className="size-3.5" />{detail.status === 'pending' ? l.start : l.resume}</Button>}
          {canPauseQueue(detail) && detail.status !== 'pending' && <Button variant="outline" size="sm" disabled={busy} onClick={() => void act([detail.id], 'pause')}><Pause className="size-3.5" />{l.pause}</Button>}
          {queueMenu(detail)}
        </>} /> : <EmptyPlanDetail hasVisible={visibleTasks.length > 0} />}
      </section>
    </div>}
    {storageProject && <ProjectStorageDialog key={storageProject.id} project={storageProject} onClose={() => setStorageProject(null)} />}
    {(creating || deepLinkPlanSource !== null) ? <TaskCreateDialog key={deepLinkPlanSource ? 'deep-link' : 'manual'} projects={projects} projectId={detail?.projectId} source={copySource} planSource={copyPlanSource} initialMode={deepLinkPlanSource ? 'plan' : undefined} initialPlanSourceTaskIds={deepLinkPlanSource || undefined} onClose={closeCreateDialog} onCreated={queue => { closeCreateDialog(); setQueues(prev => [queue, ...prev]); setQuery(''); setStatusFilter([]); setKindFilter('all'); chooseQueue(queue); toast.success(l.created); void refresh(true); void refreshProjects() }} onPlanCreated={plan => { closeCreateDialog(); setSelectedTaskKey(`plan:${plan.run.id}`); setQuery(''); setStatusFilter([]); setKindFilter('all'); toast.success(p.createCreated); void refresh(true); void refreshProjects() }} /> : null}
    <AlertDialog open={deleteTargets.length > 0} onOpenChange={open => { if (!open && !busy) setDeleteKeys([]) }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{countLabel(l.deleteConfirmMixed, deleteTargets.length)}</AlertDialogTitle><AlertDialogDescription>{l.deleteScopeMixed}</AlertDialogDescription></AlertDialogHeader>
        <ul className="max-h-48 space-y-2 overflow-y-auto rounded-md border p-3 text-sm">{deleteTargets.map(target => <li key={target.key} className="flex items-center justify-between gap-3"><span className="flex min-w-0 items-center gap-2"><Badge variant="outline" className="h-5 shrink-0 rounded px-1 text-[10px]">{target.kind === 'batch' ? l.batchTab : p.tab}</Badge><span className="break-words">{target.name}</span></span>{target.kind === 'batch' ? badge(target.status as BatchStatus) : <PlanStatusBadge status={target.status} />}</li>)}</ul>
        {activeDeleteTargets && <p role="alert" className="text-sm text-tone-amber">{l.deleteStopped}</p>}
        <AlertDialogFooter><AlertDialogCancel disabled={busy}>{l.cancel}</AlertDialogCancel><AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" disabled={busy || activeDeleteTargets || !deleteTargets.length} onClick={event => { event.preventDefault(); void remove() }}>{busy ? l.loading : countLabel(l.deleteSelected, deleteTargets.length)}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
}
