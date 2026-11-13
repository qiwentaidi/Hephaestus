'use client'

import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useEffect, useId, useRef, useState } from 'react'
import { ArrowUpRight, ChevronRight, GitBranch, Pause, Play, Search, UserRoundCog } from 'lucide-react'

import { PlanStatusBadge } from '@/components/plan-status-badge'
import { Accordion, AccordionContent, AccordionItem, AccordionTrigger } from '@/components/ui/accordion'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Empty, EmptyDescription } from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Spinner } from '@/components/ui/spinner'
import { ensureAIPlanRunController, getAIPlanItem, listAIPlanItemRows, type AIPlanItem, type AIPlanItemRow, type AIPlanItemRows, type AIPlanRunSummary } from '@/lib/api/ai-plan'
import { useI18n } from '@/lib/i18n'
import toast from 'react-hot-toast'
import { canPausePlanRun, canResumePlanRun, planItemInstruction, planItemTarget, planItemTitle } from '@/lib/ai-plan-view'
import { cn } from '@/lib/utils'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { ReportContent } from '../../../vulnerabilities/detail/ReportContent'

const detailTriggerClass = 'cursor-pointer flex-row-reverse justify-end gap-1.5 rounded-sm py-0 hover:no-underline focus-visible:border-transparent focus-visible:ring-0 focus-visible:outline-2 focus-visible:outline-foreground [&>svg]:translate-y-0 [&>svg]:-rotate-90 [&[data-state=open]>svg]:rotate-0'

function formatDate(value: string | undefined | null, locale: string) {
  if (!value || value.startsWith('0001-')) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

interface PlanTaskDetailProps {
  item: AIPlanRunSummary
  rows?: AIPlanItemRows
  error?: string
  refreshing: boolean
  onRefresh: () => void
  projectName: string
  onPause: () => void
  onResume: () => void
  busy: boolean
  action?: 'pause' | 'resume'
}

export function PlanTaskDetail({ item, rows, error, refreshing, onRefresh, projectName, onPause, onResume, busy, action }: PlanTaskDetailProps) {
  const { t, language } = useI18n()
  const text = t.aiPlan
  const router = useRouter()
  const [openingController, setOpeningController] = useState(false)
  const [query, setQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [selectedId, setSelectedId] = useState<string>()
  const [searchResult, setSearchResult] = useState<{ query: string; items?: AIPlanItemRow[]; error?: string }>()
  const [searchAttempt, setSearchAttempt] = useState(0)
  const searchRequest = useRef<AbortController | null>(null)
  const [itemDetail, setItemDetail] = useState<{ id: string; data?: AIPlanItem; error?: string }>()
  const [detailAttempt, setDetailAttempt] = useState(0)
  const listRef = useRef<HTMLDivElement>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)
  const triggerRef = useRef<HTMLButtonElement | null>(null)
  const sheetId = useId()
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'
  const { run } = item
  const items = rows?.items || []
  const needle = query.trim()
  const result = searchResult?.query === needle ? searchResult : undefined
  const filtered = (needle ? result?.items || [] : items).filter(row => statusFilter.length === 0 || statusFilter.includes(row.status))
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filtered.length / pageSize)))
  const pageItems = filtered.slice((currentPage - 1) * pageSize, currentPage * pageSize)
  const selectedRow = items.find(value => value.id === selectedId)
  const selectedItem = selectedId === itemDetail?.id ? itemDetail?.data : undefined
  const itemError = selectedId === itemDetail?.id ? itemDetail?.error : undefined
  const selectedStatus = selectedRow?.status
  const selectedCompletedAt = selectedRow?.completedAt
  useEffect(() => () => {
    searchRequest.current?.abort()
    searchRequest.current = null
  }, [needle, run.id, searchAttempt])
  useEffect(() => {
    if (!needle || !rows) return
    const timer = setTimeout(() => {
      if (searchRequest.current) return
      const controller = new AbortController()
      searchRequest.current = controller
      void listAIPlanItemRows(run.id, needle, controller.signal).then(data => {
        if (!controller.signal.aborted) setSearchResult({ query: needle, items: data.items })
      }).catch(reason => {
        if (!controller.signal.aborted) setSearchResult({ query: needle, error: reason instanceof Error ? reason.message : String(reason) })
      }).finally(() => {
        if (searchRequest.current === controller) searchRequest.current = null
      })
    }, 250)
    return () => clearTimeout(timer)
  }, [needle, rows, run.id, searchAttempt])

  useEffect(() => {
    if (!selectedId || !selectedStatus) return
    const controller = new AbortController()
    void getAIPlanItem(run.id, selectedId, controller.signal).then(data => {
      if (!controller.signal.aborted) setItemDetail({ id: selectedId, data })
    }).catch(reason => {
      if (!controller.signal.aborted) setItemDetail({ id: selectedId, error: reason instanceof Error ? reason.message : String(reason) })
    })
    return () => controller.abort()
  }, [run.id, selectedId, selectedStatus, selectedCompletedAt, detailAttempt])
  const statusCounts = items.reduce<Record<string, number>>((counts, value) => {
    counts[value.status] = (counts[value.status] || 0) + 1
    return counts
  }, {})
  const filters = [...new Set(['running', 'pending', 'paused', 'failed', 'completed', ...Object.keys(statusCounts)])]
  const statusLabel = (value: string) => text.statuses[value as keyof typeof text.statuses] || value
  const resetPage = () => { setPage(1); listRef.current?.scrollTo({ top: 0 }) }
  const toggleStatusFilter = (value: string) => {
    setStatusFilter(prev => value === 'all'
      ? []
      : prev.includes(value)
        ? prev.filter(item => item !== value)
        : [...prev, value])
    resetPage()
  }
  const name = item.project?.name || projectName
  const active = run.status === 'planning' || run.status === 'waiting'
  const openController = async () => {
    setOpeningController(true)
    try {
      let controllerId = run.controllerConversationId || ''
      if (!controllerId) {
        const detail = await ensureAIPlanRunController(run.id)
        controllerId = detail.controllerConversation?.id || detail.run.controllerConversationId || ''
      }
      if (!controllerId) throw new Error(text.loadFailed)
      router.push(`/ai/workbench?session=${encodeURIComponent(controllerId)}`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : text.loadFailed)
    } finally {
      setOpeningController(false)
    }
  }
  if (!rows) return <div className="flex h-full flex-col items-center justify-center gap-3 p-4 text-center">
    {error && !refreshing ? <>
      <p role="alert" className="text-sm text-destructive">{error}</p>
      <Button variant="outline" size="sm" onClick={onRefresh}>{t.aiBatch.retryLoad}</Button>
    </> : <span role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><Spinner aria-hidden="true" className="motion-reduce:animate-none" />{text.loading}</span>}
  </div>
  return <div className="flex h-full min-h-0 min-w-0 flex-col p-4 md:p-5">
    <header className="shrink-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h2 ref={titleRef} tabIndex={-1} className="min-w-0 text-xl font-semibold tracking-tight outline-none"><Link href={`/ai/workbench/project?project=${encodeURIComponent(run.projectId)}&tab=plan`} className="line-clamp-2 break-words hover:underline">{name}</Link></h2>
            <PlanStatusBadge status={run.status} />
            {run.wakePending && active ? <Badge variant="outline" className="border-amber-500/30 text-amber-700 dark:text-amber-300">wake pending</Badge> : null}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" disabled={openingController} onClick={() => void openController()}><UserRoundCog className="size-3.5" />{openingController ? text.openingController : text.openController}</Button>
          <Button variant="outline" size="sm" asChild><Link href={`/ai/workbench/project?project=${encodeURIComponent(run.projectId)}&tab=plan`}>{text.openProject}<ArrowUpRight className="size-3.5" /></Link></Button>
          <Button variant="outline" size="sm" asChild><Link href={`/ai/workbench?session=${encodeURIComponent(run.plannerConversationId)}`}>{text.openPlanner}<ArrowUpRight className="size-3.5" /></Link></Button>
          {canPausePlanRun(run) && <Button variant="outline" size="sm" disabled={busy} onClick={onPause}><Pause className="size-3.5" />{action === 'pause' ? text.statuses.pausing : text.pause}</Button>}
          {canResumePlanRun(run) && <Button variant="outline" size="sm" disabled={busy} onClick={onResume}><Play className="size-3.5" />{action === 'resume' ? text.resuming : text.resume}</Button>}
          {run.status === 'pausing' && <Button variant="outline" size="sm" disabled><Pause className="size-3.5" />{text.statuses.pausing}</Button>}
        </div>
      </div>
      {run.objective && <p className="mt-2 max-h-24 overflow-y-auto whitespace-pre-wrap break-words text-sm text-muted-foreground">{run.objective}</p>}
      {rows.summary && <details className="mt-3 rounded-lg border p-2 text-sm text-muted-foreground"><summary className="cursor-pointer">{text.summary}</summary><p className="mt-2 max-h-32 overflow-auto whitespace-pre-wrap break-words">{rows.summary}</p></details>}
      {rows.error && <p role={run.status === 'failed' ? 'alert' : 'status'} className={cn('mt-2 max-h-32 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border p-3 text-sm', run.status === 'failed' ? 'border-destructive/30 text-destructive' : 'border-amber-500/30 text-amber-700 dark:text-amber-300')}>{rows.error}</p>}
      {error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}
    </header>
    <div className="mt-4 flex shrink-0 flex-col gap-3">
      <div role="group" aria-label={text.statusFilter} className="flex min-w-0 max-w-full gap-3 overflow-x-auto border-b">
        {['all', ...filters].map(value => {
          const isActive = value === 'all' ? statusFilter.length === 0 : statusFilter.includes(value)
          return <button key={value} type="button" aria-pressed={isActive} className={cn('shrink-0 border-b-2 px-1 py-2 text-xs outline-offset-2 focus-visible:outline-2 focus-visible:outline-foreground', isActive ? 'border-foreground font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')} onClick={() => toggleStatusFilter(value)}>{value === 'all' ? t.aiBatch.all : statusLabel(value)} <span className="tabular-nums">{value === 'all' ? items.length : statusCounts[value] || 0}</span></button>
        })}
      </div>
      <div className="relative"><Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" aria-hidden /><Input className="h-9 pl-9" aria-label={text.searchItems} placeholder={text.searchItems} value={query} onChange={event => { setQuery(event.target.value); if (event.target.value.trim() !== needle) setSearchResult(undefined); resetPage() }} /></div>
    </div>
    <div ref={listRef} className="mt-3 min-h-0 flex-1 overflow-auto rounded-lg border" data-testid="plan-items-scroll">
      <table className="w-full table-fixed text-sm" aria-label={text.items}>
        <thead className="sticky top-0 z-10 bg-card text-xs text-muted-foreground"><tr className="border-b">
          <th className="w-10 px-2 py-3 text-left font-normal">#</th><th className="px-2 py-3 text-left font-normal">{text.planAndTarget}</th><th className="w-20 px-2 py-3 text-left font-normal">{text.status}</th><th className="w-16 px-2 py-3 text-right font-normal">{t.common.action}</th>
        </tr></thead>
        <tbody>{pageItems.map(planItem => <tr key={planItem.id} data-state={selectedId === planItem.id ? 'selected' : undefined} className="group cursor-pointer border-b last:border-b-0 hover:bg-muted/30 data-[state=selected]:bg-muted/60" onClick={event => {
          if ((event.target as HTMLElement).closest('button')) return
          const trigger = event.currentTarget.querySelector('button')
          triggerRef.current = trigger
          setSelectedId(planItem.id)
        }}>
          <td className={cn('border-l-2 px-2 py-3 text-xs tabular-nums text-muted-foreground', selectedId === planItem.id ? 'border-l-foreground' : 'border-l-transparent')}>{planItem.seq}</td>
          <td className="min-w-0 px-2 py-3"><p className="truncate font-medium" title={planItem.title || planItem.instruction}>{planItemTitle(planItem) || `${text.items} #${planItem.seq}`}</p><p className="mt-1 truncate font-mono text-xs text-muted-foreground" title={planItemTarget(planItem)}>{planItemTarget(planItem) || '—'}</p></td>
          <td className="px-2 py-3"><PlanStatusBadge status={planItem.status} /></td>
          <td className="px-2 py-3 text-right"><button type="button" className="inline-flex items-center gap-0.5 rounded px-1 py-1 text-xs text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-foreground" aria-label={`${text.viewItem} #${planItem.seq}`} aria-haspopup="dialog" aria-expanded={selectedId === planItem.id} aria-controls={selectedId === planItem.id ? sheetId : undefined} onClick={event => { triggerRef.current = event.currentTarget; setSelectedId(planItem.id) }}>{text.view}<ChevronRight className="size-3" /></button></td>
        </tr>)}</tbody>
      </table>
      {!pageItems.length && <div className="p-6 text-center text-sm text-muted-foreground">
        {needle && !result ? <span role="status" className="inline-flex items-center gap-2"><Spinner aria-hidden="true" className="motion-reduce:animate-none" />{t.aiBatch.loading}</span>
          : result?.error ? <>
            <p role="alert" className="text-destructive">{result.error}</p>
            <Button variant="outline" size="sm" className="mt-3" onClick={() => { setSearchResult(undefined); setSearchAttempt(value => value + 1) }}>{t.aiBatch.retryLoad}</Button>
          </> : <>
            <p role="status">{items.length ? text.noMatchingItems : text.noItems}</p>
            {items.length > 0 && <Button variant="outline" size="sm" className="mt-3" onClick={() => { setQuery(''); setSearchResult(undefined); setStatusFilter([]); resetPage() }}>{t.aiBatch.clearFilters}</Button>}
          </>}
      </div>}
    </div>
    <footer className="shrink-0 pt-3"><PaginationPro current={currentPage} total={filtered.length} pageSize={pageSize} pageSizeOptions={[20, 50, 100]} middlePageCount={3} compact onChange={(nextPage, nextSize) => { setPage(nextPage); setPageSize(nextSize); listRef.current?.scrollTo({ top: 0 }) }} className="gap-2 [&_[aria-current=page]]:bg-foreground [&_[aria-current=page]]:text-background" /></footer>
    <Sheet open={Boolean(selectedRow)} onOpenChange={open => { if (!open) { setSelectedId(undefined); setItemDetail(undefined) } }}>
      <SheetContent id={sheetId} side="right" className="w-full gap-0 p-0 sm:max-w-3xl" onCloseAutoFocus={event => { event.preventDefault(); (triggerRef.current?.isConnected ? triggerRef.current : titleRef.current)?.focus() }}>
        {selectedRow && <>
          <SheetHeader className="shrink-0 border-b px-5 py-4 pr-12">
            <SheetTitle className="flex flex-wrap items-center gap-2">{text.itemDetail} #{selectedRow.seq} <PlanStatusBadge status={selectedRow.status} /></SheetTitle>
            <SheetDescription className="break-words">{planItemTitle(selectedItem || selectedRow) || `${text.items} #${selectedRow.seq}`}</SheetDescription>
          </SheetHeader>
          {!selectedItem && <div className="p-6 text-center text-sm text-muted-foreground">
            {itemError ? <>
              <p role="alert" className="text-destructive">{itemError}</p>
              <Button variant="outline" size="sm" className="mt-3" onClick={() => { setItemDetail(undefined); setDetailAttempt(value => value + 1) }}>{t.aiBatch.retryLoad}</Button>
            </> : <span role="status" className="inline-flex items-center gap-2"><Spinner aria-hidden="true" className="motion-reduce:animate-none" />{t.aiBatch.loading}</span>}
          </div>}
          {selectedItem && <>
            <div key={selectedItem.id} className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
              <section><h3 className="mb-2 text-sm font-medium">{text.target}</h3><p className="break-all font-mono text-xs leading-5 text-muted-foreground">{planItemTarget(selectedItem) || '—'}</p></section>
              <Accordion type="multiple" defaultValue={['summary']} className="min-w-0 space-y-4 motion-reduce:[&_[data-slot=accordion-content]]:animate-none motion-reduce:[&_[data-slot=accordion-trigger]>svg]:transition-none">
                <AccordionItem value="instruction" className="min-w-0 border-0">
                  <AccordionTrigger className={detailTriggerClass}>{text.fullInstruction}</AccordionTrigger>
                  <AccordionContent className="pt-3 pb-0">
                    <div className="min-w-0 overflow-hidden rounded-md bg-muted/20 p-4">
                      <p className="whitespace-pre-wrap break-words text-sm leading-6">{planItemInstruction(selectedItem) || '—'}</p>
                    </div>
                  </AccordionContent>
                </AccordionItem>
                {selectedItem.instruction && selectedItem.instruction !== planItemInstruction(selectedItem) && <AccordionItem value="raw-instruction" className="min-w-0 border-0">
                  <AccordionTrigger className={detailTriggerClass}>{text.rawInstruction}</AccordionTrigger>
                  <AccordionContent className="pt-3 pb-0"><pre className="min-w-0 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted/20 p-4 text-xs leading-5">{selectedItem.instruction}</pre></AccordionContent>
                </AccordionItem>}
                <AccordionItem value="summary" className="min-w-0 border-0">
                  <AccordionTrigger className={detailTriggerClass}>{text.summary}</AccordionTrigger>
                  <AccordionContent className="pt-3 pb-0">
                    <div className="min-w-0 overflow-hidden rounded-md bg-muted/20 p-4">
                      {selectedItem.resultSummary?.trim() ? <ReportContent value={selectedItem.resultSummary} language={language} /> : <p className="text-sm text-muted-foreground">{text.noSummary}</p>}
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
              {selectedItem.error && <section className={cn('rounded-lg border p-3', selectedItem.status === 'failed' ? 'border-destructive/30 text-destructive' : 'border-amber-500/30 text-amber-700 dark:text-amber-300')}><h3 className="mb-2 text-sm font-medium">{selectedItem.status === 'blocked' ? text.blockedReason : text.error}</h3><p className="whitespace-pre-wrap break-words text-sm leading-6">{selectedItem.error}</p></section>}
              <dl className="space-y-2 text-xs text-muted-foreground"><div className="flex justify-between gap-3"><dt>{t.aiBatch.started}</dt><dd>{formatDate(selectedItem.startedAt, locale)}</dd></div><div className="flex justify-between gap-3"><dt>{t.aiBatch.finished}</dt><dd>{formatDate(selectedItem.completedAt, locale)}</dd></div></dl>
              <section><h3 className="mb-2 text-sm font-medium">{text.workerSession}</h3><p className="break-all text-xs text-muted-foreground">{selectedItem.childConversationId || text.noWorker}</p></section>
            </div>
            {selectedItem.childConversationId && <SheetFooter className="shrink-0 flex-row flex-wrap border-t px-5 py-4"><Button size="sm" asChild><Link href={`/ai/workbench?session=${encodeURIComponent(selectedItem.childConversationId)}`}>{text.openWorker}<ArrowUpRight className="size-3.5" /></Link></Button><Button size="sm" variant="outline" asChild><Link href={`/ai/workbench/exploration?session=${encodeURIComponent(selectedItem.childConversationId)}`}><GitBranch className="size-3.5" />{text.exploration}</Link></Button></SheetFooter>}
          </>}
        </>}
      </SheetContent>
    </Sheet>
  </div>
}

export function EmptyPlanDetail({ hasVisible }: { hasVisible: boolean }) {
  const { t } = useI18n()
  return <Empty className="min-h-64"><EmptyDescription>{hasVisible ? t.aiPlan.selectRun : t.aiPlan.empty}</EmptyDescription></Empty>
}
