'use client'

import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowUpRight, ChevronDown, GitBranch, RefreshCw, Search, UserRoundCog } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { PlanStatusBadge } from '@/components/plan-status-badge'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import {
  cancelAIPlanRun,
  ensureAIPlanRunController,
  listAIPlanItemRows,
  listAIPlanRunSummaries,
  isOrchestrationRunId,
  type AIPlanItemRow,
  type AIPlanRunSummary,
} from '@/lib/api/ai-plan'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import styles from './plan.module.css'

function formatDate(value: string | undefined | null, locale: string) {
  if (!value || value.startsWith('0001-')) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale, { hour12: false })
}

/** Status buckets, in the order an operator needs to see them. */
const GROUPS = [
  { key: 'running', statuses: ['running'], labels: ['进行中', 'Running'], tone: 'amber' },
  { key: 'blocked', statuses: ['blocked'], labels: ['受阻', 'Blocked'], tone: 'orange' },
  { key: 'failed', statuses: ['failed'], labels: ['失败', 'Failed'], tone: 'red' },
  { key: 'paused', statuses: ['paused'], labels: ['已暂停', 'Paused'], tone: 'muted' },
  { key: 'pending', statuses: ['pending'], labels: ['待执行', 'Pending'], tone: 'muted' },
  { key: 'completed', statuses: ['completed'], labels: ['已完成', 'Completed'], tone: 'green' },
  { key: 'cancelled', statuses: ['cancelled'], labels: ['已取消', 'Cancelled'], tone: 'muted' },
] as const

const FILTERS = [
  { key: 'all', labels: ['全部', 'All'] },
  { key: 'running', labels: ['进行中', 'Running'] },
  { key: 'blocked', labels: ['受阻', 'Blocked'] },
  { key: 'failed', labels: ['失败', 'Failed'] },
  { key: 'completed', labels: ['已完成', 'Completed'] },
] as const

type FilterKey = typeof FILTERS[number]['key']

/** Rows rendered per group before "show more": a 150-item group must not paint 150 cards. */
const GROUP_PAGE = 30
const EMPTY_COUNTS: Record<string, number> = {}
const EMPTY_ROWS: AIPlanItemRow[] = []

/** Rows belong to a run+search key; a stale key renders as loading, never as another run's data. */
type RowState = { key: string; items: AIPlanItemRow[]; loading: boolean; error: string }

export function PlanWorkspace({ projectId }: { projectId: string }) {
  const { t, language } = useI18n()
  const text = t.aiPlan
  const zh = language === 'zh'
  const router = useRouter()
  const locale = zh ? 'zh-CN' : 'en-US'

  // Runs come from the summary projection (metadata + status counts, no items),
  // and rows from the lightweight item projection. Neither endpoint ships a
  // multi-kilobyte report per row, so polling stays cheap.
  const [runs, setRuns] = useState<AIPlanRunSummary[]>([])
  const [rowState, setRowState] = useState<RowState>({ key: '', items: [], loading: true, error: '' })
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [openingControllerId, setOpeningControllerId] = useState('')
  const [error, setError] = useState('')
  const [selectedId, setSelectedId] = useState('')
  const [filter, setFilter] = useState<FilterKey>('all')
  const [query, setQuery] = useState('')
  const [search, setSearch] = useState('')
  // The limit is keyed by run+search so switching scope resets it without a
  // state-writing effect.
  const [limit, setLimit] = useState<{ key: string; value: number }>({ key: '', value: 0 })
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({ completed: true, cancelled: true })
  const runsGeneration = useRef(0)
  const rowsGeneration = useRef(0)
  const firstLoad = useRef(true)

  const openController = useCallback(async (run: AIPlanRunSummary['run']) => {
    setOpeningControllerId(run.id)
    try {
      let controllerId = run.controllerConversationId || ''
      if (!controllerId) {
        const detail = await ensureAIPlanRunController(run.id)
        controllerId = detail.controllerConversation?.id || detail.run.controllerConversationId || ''
      }
      if (!controllerId) throw new Error(text.loadFailed)
      router.push(`/ai/workbench?session=${encodeURIComponent(controllerId)}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      setOpeningControllerId('')
    }
  }, [router, text.loadFailed])

  const loadRuns = useCallback(async (quiet = false) => {
    if (!projectId) return
    const generation = ++runsGeneration.current
    if (quiet) setRefreshing(true)
    else setLoading(true)
    try {
      const controller = new AbortController()
      const summaries = await listAIPlanRunSummaries('', controller.signal, projectId)
      if (generation !== runsGeneration.current) return
      setRuns(summaries)
      setError('')
    } catch (err) {
      if (generation === runsGeneration.current) setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      if (generation === runsGeneration.current) {
        setLoading(false)
        setRefreshing(false)
      }
    }
  }, [projectId, text.loadFailed])

  const selected = runs.find(item => item.run.id === selectedId) || runs[0]
  const runId = selected?.run.id || ''
  const terminal = selected ? ['completed', 'failed', 'cancelled'].includes(selected.run.status) : true

  useEffect(() => {
    void loadRuns(!firstLoad.current)
    firstLoad.current = false
  }, [loadRuns])

  // Debounce typing so server-side search does not fire per keystroke.
  useEffect(() => {
    const timer = window.setTimeout(() => setSearch(query.trim()), 250)
    return () => window.clearTimeout(timer)
  }, [query])

  const loadRows = useCallback(async () => {
    if (!runId) return
    const key = `${runId}|${search}`
    const generation = ++rowsGeneration.current
    try {
      const controller = new AbortController()
      const data = await listAIPlanItemRows(runId, search, controller.signal)
      if (generation !== rowsGeneration.current) return
      setRowState({ key, items: data.items, loading: false, error: '' })
    } catch (err) {
      if (generation !== rowsGeneration.current) return
      setRowState({ key, items: [], loading: false, error: err instanceof Error ? err.message : text.loadFailed })
    }
  }, [runId, search, text.loadFailed])

  useEffect(() => {
    // The fetch resolves asynchronously; the compiler cannot see through the
    // callback and flags the call itself.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadRows()
  }, [loadRows])

  // The run list is tiny, so it polls quickly; the row projection is heavier and
  // only refreshes while the selected run can still change.
  useEffect(() => {
    const timer = window.setInterval(() => {
      if (!document.hidden) void loadRuns(true)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [loadRuns])

  useEffect(() => {
    if (terminal) return
    const timer = window.setInterval(() => {
      if (!document.hidden) void loadRows()
    }, 15000)
    return () => window.clearInterval(timer)
  }, [loadRows, terminal])

  const rowsKey = `${runId}|${search}`
  const rows = rowState.key === rowsKey ? rowState.items : EMPTY_ROWS
  const rowsLoading = rowState.key !== rowsKey || rowState.loading
  const rowsError = rowState.key === rowsKey ? rowState.error : ''
  const itemTotal = selected?.itemTotal ?? 0
  const counts = selected?.itemCounts ?? EMPTY_COUNTS
  const doneCount = counts.completed || 0
  const runningCount = counts.running || 0
  const blockedCount = counts.blocked || 0
  const failedCount = counts.failed || 0
  const percent = itemTotal ? Math.round((doneCount / itemTotal) * 100) : 0
  const searching = search !== ''

  const groups = useMemo(() => GROUPS.map(group => {
    const groupRows = rows.filter(item => (group.statuses as readonly string[]).includes(item.status))
    return { ...group, rows: groupRows }
  }).filter(group => group.rows.length > 0 || (filter === 'all' && !searching && (counts[group.key] || 0) > 0)), [rows, searching, counts, filter])

  const visibleGroups = filter === 'all' ? groups : groups.filter(group => group.key === filter)
  const chipValue = (key: FilterKey) => {
    if (searching) {
      if (key === 'all') return rows.length
      return rows.filter(item => item.status === key).length
    }
    if (key === 'all') return itemTotal
    return counts[key] || 0
  }

  if (loading && !runs.length) {
    return <div className="flex items-center justify-center gap-2 p-12"><Spinner /><span className="text-sm text-muted-foreground">{text.loading}</span></div>
  }

  return (
    <div className={styles.page}>
      {error ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p> : null}
      {!selected ? (
        <div className={styles.empty}>{text.empty}</div>
      ) : (
        <>
          <section className={styles.runbar}>
            <div className={styles.r1}>
              <div className={styles.who}>
                <div className={styles.line1}>
                  <PlanStatusBadge status={selected.run.status} />
                  {runs.length > 1 ? (
                    <Select value={selected.run.id} onValueChange={value => setSelectedId(value)}>
                      <SelectTrigger className={styles.selector} aria-label={zh ? '选择编排运行' : 'Select run'}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {runs.map((item, index) => (
                          <SelectItem key={item.run.id} value={item.run.id}>
                            {formatDate(item.run.createdAt, locale)}{index === 0 ? (zh ? ' · 最新' : ' · latest') : ''}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <span className={styles.name}>{formatDate(selected.run.createdAt, locale)}</span>
                  )}
                  {selected.run.wakePending && !terminal ? <Badge variant="outline">{zh ? '等待唤醒' : 'wake pending'}</Badge> : null}
                  <Button variant="ghost" size="sm" onClick={() => { void loadRuns(true); void loadRows() }} disabled={refreshing}>
                    <RefreshCw className={cn('size-3.5', refreshing && 'animate-spin')} />
                  </Button>
                </div>
                <div className={styles.when}>
                  {zh ? 'Planner' : 'Planner'} {selected.run.plannerModel || '—'} · {zh ? 'Worker' : 'Worker'} {selected.run.workerModel || '—'}
                  {' · '}{zh ? `并发上限 ${selected.run.maxWorkers}` : `${selected.run.maxWorkers} workers`}
                </div>
                <div className={styles.objective} title={selected.run.objective}>{selected.run.objective}</div>
              </div>
              <div className={styles.actions}>
                <Button variant="default" size="sm" onClick={() => void openController(selected.run)} disabled={openingControllerId === selected.run.id || ((selected.run.readOnly || !isOrchestrationRunId(selected.run.id)) && !selected.run.controllerConversationId)}>
                  <UserRoundCog className="size-3.5" />
                  {openingControllerId === selected.run.id ? text.openingController : selected.run.controllerConversationId ? text.openController : text.enableController}
                </Button>
                <Button variant="outline" size="sm" asChild>
                  <Link href={`/ai/workbench?session=${encodeURIComponent(selected.run.plannerConversationId)}`}>
                    {text.openPlanner}
                    <ArrowUpRight className="size-3.5" />
                  </Link>
                </Button>
                {!terminal && !selected.run.readOnly && isOrchestrationRunId(selected.run.id) ? (
                  <Button variant="outline" size="sm" onClick={async () => {
                    try {
                      await cancelAIPlanRun(selected.run.id)
                      void loadRuns(true)
                    } catch (err) {
                      setError(err instanceof Error ? err.message : text.loadFailed)
                    }
                  }}>{text.cancel}</Button>
                ) : null}
              </div>
            </div>
            <div className={styles.r2}>
              <div className={styles.progress}>
                <div className={styles.progressTop}>
                  <strong>{doneCount} / {itemTotal}</strong>
                  <span>{zh ? `计划项完成 ${percent}%` : `${percent}% complete`}</span>
                  <span className={styles.progressRight}>
                    {zh ? `运行中 ${runningCount} · 受阻 ${blockedCount} · 失败 ${failedCount}` : `${runningCount} running · ${blockedCount} blocked · ${failedCount} failed`}
                  </span>
                </div>
                <div className={styles.seg}>
                  {doneCount ? <i className={styles.segDone} style={{ width: `${(doneCount / Math.max(1, itemTotal)) * 100}%` }} /> : null}
                  {runningCount ? <i className={styles.segRun} style={{ width: `${(runningCount / Math.max(1, itemTotal)) * 100}%` }} /> : null}
                  {blockedCount ? <i className={styles.segBlock} style={{ width: `${(blockedCount / Math.max(1, itemTotal)) * 100}%` }} /> : null}
                  {failedCount ? <i className={styles.segFail} style={{ width: `${(failedCount / Math.max(1, itemTotal)) * 100}%` }} /> : null}
                </div>
                <div className={styles.legend}>
                  <span className={styles.legendItem}><span className={styles.dot} data-tone="green" />{zh ? '已完成' : 'Completed'} {doneCount}</span>
                  <span className={styles.legendItem}><span className={styles.dot} data-tone="amber" />{zh ? '进行中' : 'Running'} {runningCount}</span>
                  <span className={styles.legendItem}><span className={styles.dot} data-tone="orange" />{zh ? '受阻' : 'Blocked'} {blockedCount}</span>
                  <span className={styles.legendItem}><span className={styles.dot} data-tone="red" />{zh ? '失败' : 'Failed'} {failedCount}</span>
                </div>
              </div>
            </div>
          </section>

          <div className={styles.toolbar}>
            <div className={styles.chips} role="group" aria-label={text.statusFilter}>
              {FILTERS.map(option => (
                <button key={option.key} type="button" className={styles.chip} data-active={filter === option.key} onClick={() => setFilter(option.key)}>
                  {option.labels[zh ? 0 : 1]}
                  <span className={styles.chipCount}>{chipValue(option.key)}</span>
                </button>
              ))}
            </div>
            <label className={styles.search}>
              <Search />
              <input value={query} onChange={event => setQuery(event.target.value)} placeholder={text.searchItems} aria-label={text.searchItems} />
            </label>
          </div>

          {rowsError ? <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{rowsError}</p> : null}

          <section className={styles.list} aria-label={text.title}>
            {visibleGroups.length === 0 ? (
              <p className={styles.empty}>{rowsLoading ? text.loading : query || filter !== 'all' ? text.noMatchingItems : text.noItems}</p>
            ) : visibleGroups.map(group => {
              const open = filter !== 'all' || !collapsed[group.key]
              const limitKey = `${runId}|${search}|${group.key}`
              const groupLimit = limit.key === limitKey ? limit.value : GROUP_PAGE
              const shown = group.rows.slice(0, groupLimit)
              const remaining = group.rows.length - shown.length
              return (
                <div key={group.key} className={styles.group}>
                  <button type="button" className={styles.groupHead} onClick={() => setCollapsed(prev => ({ ...prev, [group.key]: !prev[group.key] }))} aria-expanded={open}>
                    <ChevronDown className={styles.chevron} data-open={open} />
                    <span className={styles.dot} data-tone={group.tone} />
                    {group.labels[zh ? 0 : 1]}
                    <span className={styles.groupCount}>{searching ? group.rows.length : (counts[group.key] || 0)}</span>
                    <span className={styles.groupHint}>
                      {group.key === 'blocked' ? (zh ? '需要处理' : 'Needs attention')
                        : group.key === 'failed' ? (zh ? '执行出错' : 'Execution errors')
                        : group.key === 'completed' ? (zh ? '默认折叠' : 'Collapsed by default')
                        : group.key === 'running' ? (zh ? '正在执行' : 'In progress')
                        : ''}
                    </span>
                  </button>
                  {open ? (
                    <>
                      <div className={styles.rows}>{shown.map(item => (
                        <PlanRow key={item.id} item={item} zh={zh} text={text} />
                      ))}</div>
                      {remaining > 0 ? (
                        <button type="button" className={styles.more} onClick={() => setLimit({ key: limitKey, value: groupLimit + 50 })}>
                          {zh ? `显示其余 ${remaining} 项` : `Show ${remaining} more`}
                        </button>
                      ) : null}
                    </>
                  ) : null}
                </div>
              )
            })}
          </section>

          <div className={styles.footer}>
            <span>{zh ? `计划项 ${itemTotal} · 运行 ${runs.length}` : `${itemTotal} items · ${runs.length} runs`}</span>
            {!terminal ? <span className={styles.liveHint}>{text.liveHint}</span> : null}
          </div>
        </>
      )}
    </div>
  )
}

function PlanRow({ item, zh, text }: { item: AIPlanItemRow; zh: boolean; text: ReturnType<typeof useI18n>['t']['aiPlan'] }) {
  const group = GROUPS.find(entry => (entry.statuses as readonly string[]).includes(item.status))
  const tone = group?.tone || 'muted'
  const statusLabel = text.statuses[item.status as keyof typeof text.statuses] || item.status
  return (
    <div className={styles.item}>
      <div className={styles.seq}>{item.seq}</div>
      <div className={styles.main}>
        <div className={styles.ttl}>
          <span className={styles.dot} data-tone={tone} />
          <span className={styles.title} title={item.title || item.instruction}>{item.title || item.instruction}</span>
          {item.status !== 'completed' ? <Badge variant="outline" className="shrink-0 text-[10.5px]">{statusLabel}</Badge> : null}
        </div>
        {item.target ? <div className={styles.target}>{item.target}</div> : null}
        {item.error ? <div className={`${styles.summary} ${styles.summaryError}`}>{item.error}</div>
          : item.summary ? <div className={styles.summary}>{item.summary}</div> : null}
      </div>
      <div className={styles.acts}>
        {item.childConversationId ? (
          <>
            <Button variant="outline" size="sm" asChild>
              <Link href={`/ai/workbench?session=${encodeURIComponent(item.childConversationId)}`}>{text.openWorker}<ArrowUpRight className="size-3.5" /></Link>
            </Button>
            <Button variant="ghost" size="sm" asChild>
              <Link href={`/ai/workbench/exploration?session=${encodeURIComponent(item.childConversationId)}`} aria-label={text.exploration}><GitBranch className="size-3.5" /></Link>
            </Button>
          </>
        ) : null}
      </div>
    </div>
  )
}
