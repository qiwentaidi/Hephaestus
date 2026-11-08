'use client'

import { useId, useRef, useState, type ReactNode } from 'react'
import Link from 'next/link'
import { usePathname, useSearchParams } from 'next/navigation'
import { ChevronRight, FileText, MessageSquare, Search } from 'lucide-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { batchDetailHref, canRetryQueue, filterBatchItems, isActiveQueue, summarizeBatch, type ItemFilter } from '@/lib/ai-batch-view'
import type { AIBatchQueue, BatchStatus } from '@/lib/api/ai-batch'
import { useI18n } from '@/lib/i18n'
import { getStatusBadge } from '@/lib/status-badge'
import { cn } from '@/lib/utils'
import { ReportContent } from '../../../vulnerabilities/detail/ReportContent'

export function BatchTaskDetail({ queue, projectName, actions, busy, onRetryFailed, onRetryItem }: {
  queue: AIBatchQueue
  projectName: string
  actions: ReactNode
  busy: boolean
  onRetryFailed: () => void
  onRetryItem: (itemId: string) => void
}) {
  const { t, language } = useI18n()
  const l = t.aiBatch
  const pathname = usePathname()
  const params = useSearchParams()
  const requestedFilter = params.get('items')
  const filter: ItemFilter = ['all', 'unfinished', 'failed', 'completed'].includes(requestedFilter || '') ? requestedFilter as ItemFilter : 'all'
  const [query, setQuery] = useState('')
  const [pageSize, setPageSize] = useState(20)
  const [page, setPage] = useState(() => {
    const index = filterBatchItems(queue.items, filter).findIndex(item => item.id === params.get('item'))
    return Math.max(1, Math.floor(index / 20) + 1)
  })
  const listRef = useRef<HTMLDivElement>(null)
  const triggerRef = useRef<HTMLButtonElement | null>(null)
  const titleRef = useRef<HTMLHeadingElement>(null)
  const sheetId = useId()
  const counts = summarizeBatch(queue)
  const items = filterBatchItems(queue.items, filter, query)
  const currentPage = Math.min(page, Math.max(1, Math.ceil(items.length / pageSize)))
  const pageItems = items.slice((currentPage - 1) * pageSize, currentPage * pageSize)
  const selectedItem = queue.items.find(item => item.id === params.get('item'))
  const badge = (status: BatchStatus) => getStatusBadge(status === 'completed_with_errors' ? 'completed' : status, { ...l.status, cancelled: l.cancel })
  const date = (value?: string) => value ? new Date(value).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '—'
  const selectItem = (id?: string, nextFilter = filter) => {
    const url = new URLSearchParams(window.location.search)
    url.delete('run'); url.delete('view')
    window.history.replaceState(null, '', batchDetailHref(pathname, url.toString(), queue.id, id, nextFilter))
  }
  const resetPage = () => { setPage(1); listRef.current?.scrollTo({ top: 0 }) }
  const openItem = (id: string, trigger: HTMLButtonElement | null) => { triggerRef.current = trigger; selectItem(id) }

  return <div className="flex h-full min-h-0 min-w-0 flex-col p-4 md:p-5">
    <header className="shrink-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          <h2 ref={titleRef} tabIndex={-1} className="min-w-0 break-words text-xl font-semibold tracking-tight outline-none">{queue.name}</h2>
          {badge(queue.status)}
        </div>
        <div className="flex flex-wrap items-center gap-2">{actions}</div>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <Link href={`/ai/workbench/project?project=${encodeURIComponent(queue.projectId)}`} className="min-w-0 truncate hover:text-foreground hover:underline" title={projectName}>{projectName}</Link>
        <span aria-hidden>·</span><span>{l.processed} {counts.processed}/{counts.total}</span>
        <span aria-hidden>·</span><span>{l.concurrency} {queue.concurrency}</span>
      </div>
      {queue.error && <Alert className="mt-3 max-h-24 overflow-auto border-tone-amber-border"><AlertDescription className="break-words">{queue.error}</AlertDescription></Alert>}
    </header>
    <div className="mt-4 flex shrink-0 flex-wrap items-center justify-between gap-x-3 gap-y-2 border-b">
      <div role="group" aria-label={l.itemList} className="flex min-w-0 max-w-full gap-3 overflow-x-auto">
        {(['all', 'unfinished', 'failed', 'completed'] as const).map(value => <button key={value} type="button" aria-pressed={filter === value} className={cn('shrink-0 border-b-2 px-1 py-2 text-xs outline-offset-2 focus-visible:outline-2 focus-visible:outline-foreground', filter === value ? 'border-foreground font-medium text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')} onClick={() => { selectItem(undefined, value); resetPage() }}>{value === 'all' ? l.all : value === 'unfinished' ? l.unfinished : value === 'failed' ? l.failedCount : l.done} <span className="tabular-nums">{value === 'all' ? counts.total : value === 'unfinished' ? counts.unfinished : value === 'failed' ? counts.failed : counts.completed}</span></button>)}
      </div>
      {canRetryQueue(queue) && <Button variant="outline" size="sm" className="mb-2" disabled={busy} onClick={onRetryFailed}>{l.retryFailed} ({counts.failed})</Button>}
    </div>
    <div className="relative my-3 shrink-0"><Search className="pointer-events-none absolute left-3 top-2.5 size-4 text-muted-foreground" aria-hidden /><Input className="h-9 pl-9" aria-label={l.searchItems} placeholder={l.searchItems} value={query} onChange={event => { setQuery(event.target.value); resetPage() }} /></div>
    <div ref={listRef} className="min-h-0 flex-1 overflow-auto rounded-lg border" data-testid="batch-items-scroll">
      <table className="w-full table-fixed text-sm" aria-label={l.itemList}>
        <thead className="sticky top-0 z-10 bg-card text-xs text-muted-foreground"><tr className="border-b">
          <th className="w-10 px-2 py-3 text-left font-normal">#</th>
          <th className="px-2 py-3 text-left font-normal">{l.instruction}</th>
          <th className="w-24 px-2 py-3 text-left font-normal">{l.statusColumn}</th>
          <th className="hidden w-28 px-2 py-3 text-left font-normal md:table-cell">{l.lastActivity}</th>
          <th className="w-16 px-2 py-3 text-right font-normal">{t.common.action}</th>
        </tr></thead>
        <tbody>{pageItems.map(item => <tr key={item.id} data-state={selectedItem?.id === item.id ? 'selected' : undefined} className="group cursor-pointer border-b last:border-b-0 hover:bg-muted/30 data-[state=selected]:bg-muted/60" onClick={event => {
          if ((event.target as HTMLElement).closest('button')) return
          openItem(item.id, event.currentTarget.querySelector('button'))
        }}>
          <td className={cn('border-l-2 px-2 py-3 text-xs tabular-nums text-muted-foreground', selectedItem?.id === item.id ? 'border-l-foreground' : 'border-l-transparent')}>{item.position + 1}</td>
          <td className="min-w-0 px-2 py-3"><p className="line-clamp-2 break-words" title={item.instruction}>{item.instruction}</p></td>
          <td className="px-2 py-3">{badge(item.status)}</td>
          <td className="hidden px-2 py-3 text-xs text-muted-foreground md:table-cell"><time>{date(item.completedAt || item.startedAt)}</time></td>
          <td className="px-2 py-3 text-right"><button type="button" className="inline-flex items-center gap-0.5 rounded px-1 py-1 text-xs text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-foreground" aria-label={`${l.record} #${item.position + 1}`} aria-haspopup="dialog" aria-expanded={selectedItem?.id === item.id} aria-controls={selectedItem?.id === item.id ? sheetId : undefined} onClick={event => openItem(item.id, event.currentTarget)}>{t.aiPlan.view}<ChevronRight className="size-3" /></button></td>
        </tr>)}</tbody>
      </table>
      {!pageItems.length && <p role="status" className="p-6 text-center text-sm text-muted-foreground">{l.emptyItems}</p>}
    </div>
    <footer className="shrink-0 pt-3"><PaginationPro current={currentPage} total={items.length} pageSize={pageSize} pageSizeOptions={[20, 50, 100]} middlePageCount={3} compact onChange={(nextPage, nextSize) => { setPage(nextPage); setPageSize(nextSize); listRef.current?.scrollTo({ top: 0 }) }} className="gap-2 [&_[aria-current=page]]:bg-foreground [&_[aria-current=page]]:text-background" /></footer>
    <Sheet open={Boolean(selectedItem)} onOpenChange={open => { if (!open) selectItem() }}>
      <SheetContent id={sheetId} side="right" className="w-full gap-0 p-0 sm:max-w-3xl" onCloseAutoFocus={event => { event.preventDefault(); (triggerRef.current?.isConnected ? triggerRef.current : titleRef.current)?.focus() }}>
        {selectedItem && <>
          <SheetHeader className="shrink-0 border-b px-5 py-4 pr-12">
            <SheetTitle className="flex flex-wrap items-center gap-2">{l.record} #{selectedItem.position + 1} {badge(selectedItem.status)}</SheetTitle>
            <SheetDescription className="break-words">{queue.name}</SheetDescription>
          </SheetHeader>
          <div className="min-h-0 flex-1 space-y-4 overflow-y-auto p-5">
            <p className="whitespace-pre-wrap break-words text-sm leading-6">{selectedItem.instruction}</p>
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs leading-5 text-muted-foreground"><span>{l.attempts}: {selectedItem.attempts}</span>{selectedItem.startedAt && <span>{l.started}: {date(selectedItem.startedAt)}</span>}{selectedItem.completedAt && <span>{l.finished}: {date(selectedItem.completedAt)}</span>}</div>
            {selectedItem.error && <Alert variant="destructive"><AlertDescription className="whitespace-pre-wrap break-words">{selectedItem.error}</AlertDescription></Alert>}
            <div className="min-w-0 overflow-hidden rounded-md bg-muted/20 p-4">
              {selectedItem.result ? <ReportContent value={selectedItem.result} language={language} /> : <p className="py-5 text-sm leading-6 text-muted-foreground">{selectedItem.status === 'paused' ? l.pausedResult : selectedItem.status === 'pausing' ? l.pausingResult : selectedItem.status === 'pending' ? l.pendingResult : selectedItem.status === 'running' ? l.runningResult : l.noResult}</p>}
            </div>
          </div>
          <SheetFooter className="shrink-0 flex-row flex-wrap border-t px-5 py-4">
            {selectedItem.conversationId && <Button size="sm" variant="outline" asChild><Link href={`/ai/workbench?session=${encodeURIComponent(selectedItem.conversationId)}`}><MessageSquare className="size-4" />{l.conversation}</Link></Button>}
            <Button size="sm" variant="outline" asChild><Link href={`/ai/workbench/project?project=${encodeURIComponent(queue.projectId)}&tab=evidence${selectedItem.conversationId ? `&source=${encodeURIComponent(selectedItem.conversationId)}` : ''}`}><FileText className="size-4" />{l.projectEvidence}</Link></Button>
            {selectedItem.status === 'failed' && <Button size="sm" variant="outline" disabled={busy || isActiveQueue(queue)} onClick={() => onRetryItem(selectedItem.id)}>{l.retry}</Button>}
          </SheetFooter>
        </>}
      </SheetContent>
    </Sheet>
  </div>
}
