'use client'

import { useEffect, useState } from 'react'
import { useI18n } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { listReferenceAssets, listReferenceTasks, type AssetReferenceSelection, type AssetReferenceSnapshot, type ReferenceAsset, type ReferenceTask } from '@/lib/api/ai-asset-references'

const formatReferenceDate = (value: string, language: 'zh' | 'en') => new Intl.DateTimeFormat(language === 'zh' ? 'zh-CN' : 'en-US', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value))

const copy = {
  zh: { title: '引用攻击面资产', search: '搜索任务名称、目标或 ID', assets: '搜索资产', all: '全部类型', back: '返回任务', previous: '上一页', next: '下一页', loading: '正在读取…', empty: '暂无匹配结果', selectTask: '引用整个任务', filtered: '引用全部筛选结果', selected: '引用已勾选资产', hint: '整个任务按任务和版本分页读取；筛选或勾选资产保存固定快照。不会自动重新扫描。', collecting: '采集中：整个任务会读到后续新增结果，筛选和勾选范围在发送时固定。', remove: '移除引用', task: '来源任务', version: '扫描版本', captured: '引用时间', count: '资产数', selection: '选择范围', selectedMode: '已勾选资产', filteredMode: '全部筛选结果', allMode: '整个任务', failed: '读取失败', retry: '重试', unsupported: '此任务暂时没有可引用的资产。可选择已有资产的子任务。', close: '关闭', source: '资产来源', pending: '发送时固定快照', taskPending: '发送时固定任务版本', taskCount: '引用时资产数', readMode: '读取方式', live: '按任务版本分页读取，运行中结果可能增加', snapshot: '固定资产快照', kinds: { web: '网站', port: '端口', directory: '目录', subdomain: '子域名', operation: '接口模板', api_record: '请求记录', js_resource: 'JS 文件', js_asset: 'JS 提取资产' } },
  en: { title: 'Reference attack surface assets', search: 'Search tasks, targets or IDs', assets: 'Search assets', all: 'All types', back: 'Back to tasks', previous: 'Previous', next: 'Next', loading: 'Loading…', empty: 'No matching results', selectTask: 'Reference entire task', filtered: 'Reference all matching assets', selected: 'Reference checked assets', hint: 'Entire tasks are read by task/version in pages; filtered or checked assets are saved as fixed snapshots. Scans are not restarted.', collecting: 'Collecting: entire-task references include later results; filtered and checked selections are fixed when sent.', remove: 'Remove reference', task: 'Source task', version: 'Scan version', captured: 'Captured at', count: 'Assets', selection: 'Selection', selectedMode: 'Checked assets', filteredMode: 'All matching assets', allMode: 'Entire task', failed: 'Unable to load', retry: 'Retry', unsupported: 'This task has no referenceable assets yet. You can select a child task with assets.', close: 'Close', source: 'Asset source', pending: 'Snapshot captured when sent', taskPending: 'Task version fixed when sent', taskCount: 'Assets at reference time', readMode: 'Reading mode', live: 'Read by task/version in pages; running results may grow', snapshot: 'Fixed asset snapshot', kinds: { web: 'Websites', port: 'Ports', directory: 'Directories', subdomain: 'Subdomains', operation: 'API templates', api_record: 'Requests', js_resource: 'JS files', js_asset: 'Extracted assets' } },
}

export function AssetReferenceChips({ references, onRemove, align = 'start', compact = false }: { align?: 'start' | 'end'; references: (AssetReferenceSelection | AssetReferenceSnapshot)[]; onRemove?: (index: number) => void; compact?: boolean }) {
  const { language } = useI18n(); const l = copy[language]
  const [active, setActive] = useState<number | null>(null)
  const ref = active === null ? undefined : references[active]
  const isTaskReference = (item: AssetReferenceSelection | AssetReferenceSnapshot) => 'capturedAt' in item ? item.readMode === 'task' : item.selection === 'all'
  return <>
    <div className={`flex flex-wrap ${compact ? 'gap-1' : 'gap-2'} ${align === 'end' ? 'justify-end' : ''}`}>{references.map((item, index) => <span key={'id' in item ? item.id : `${item.taskId}:${index}`} className={`inline-flex max-w-full items-center border border-border bg-muted/50 text-foreground ${compact ? 'rounded-md text-[11px]' : 'rounded-lg text-xs'}`}>
      <Button type="button" variant="ghost" onClick={() => setActive(index)} className={`h-auto min-w-0 truncate text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${compact ? 'rounded-md px-1.5 py-0.5' : 'rounded-lg px-2.5 py-1.5'}`}>@ {item.name} · {isTaskReference(item) ? l.allMode : `${item.count} ${l.count}`}</Button>
      {onRemove ? <Button type="button" variant="ghost" onClick={() => onRemove(index)} aria-label={`${l.remove}: ${item.name}`} className={`h-auto hover:bg-muted ${compact ? 'rounded-md px-1.5 py-0.5' : 'rounded-lg px-2 py-1.5'}`}>×</Button> : null}
    </span>)}</div>
    <Dialog open={Boolean(ref)} onOpenChange={(open) => { if (!open) setActive(null) }}><DialogContent><DialogHeader><DialogTitle>{ref?.name}</DialogTitle><DialogDescription>{l.source}</DialogDescription></DialogHeader>{ref ? <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 break-all text-sm">
      <dt className="text-muted-foreground">{l.task}</dt><dd>{ref.taskId}</dd>
      <dt className="text-muted-foreground">{l.version}</dt><dd>{ref.version || '—'}</dd>
      <dt className="text-muted-foreground">{isTaskReference(ref) ? l.taskCount : l.count}</dt><dd>{ref.count}</dd>
      <dt className="text-muted-foreground">{l.readMode}</dt><dd>{isTaskReference(ref) ? l.live : l.snapshot}</dd>
      <dt className="text-muted-foreground">{l.selection}</dt><dd>{ref.selection === 'all' ? l.allMode : ref.selection === 'selected' ? l.selectedMode : l.filteredMode}{ref.query ? ` · ${ref.query}` : ''}</dd>
      <dt className="text-muted-foreground">{l.captured}</dt><dd>{'capturedAt' in ref ? formatReferenceDate(ref.capturedAt, language) : isTaskReference(ref) ? l.taskPending : l.pending}</dd>
    </dl> : null}</DialogContent></Dialog>
  </>
}

export function AssetReferencePicker({ open, onOpenChange, onSelect, initialTask, inline = false }: { open: boolean; onOpenChange: (open: boolean) => void; onSelect: (reference: AssetReferenceSelection) => void; initialTask?: ReferenceTask; inline?: boolean }) {
  const { language } = useI18n(); const l = copy[language]
  const [task, setTask] = useState<ReferenceTask | null>(initialTask || null)
  const [search, setSearch] = useState(''); const [kind, setKind] = useState(''); const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [retry, setRetry] = useState(0)
  const requestKey = JSON.stringify([task?.id, task?.version, search, kind, page, retry])
  const [result, setResult] = useState<{ key: string; tasks: ReferenceTask[]; assets: ReferenceAsset[]; total: number; error: string }>({ key: '', tasks: [], assets: [], total: 0, error: '' })
  const loading = result.key !== requestKey
  const { tasks, assets, total, error } = loading ? { tasks: [], assets: [], total: 0, error: '' } : result
  useEffect(() => {
    if (!open) return
    const controller = new AbortController()
    const timer = setTimeout(() => {
      const request = task ? listReferenceAssets(task, search, kind, page, controller.signal).then((data) => { if (!controller.signal.aborted) setResult({ key: requestKey, tasks: [], assets: data.assets, total: data.total, error: '' }) }) : listReferenceTasks(search, page, controller.signal).then((data) => { if (!controller.signal.aborted) setResult({ key: requestKey, tasks: data.tasks, assets: [], total: data.total, error: '' }) })
      void request.catch((e: Error) => { if (!controller.signal.aborted) setResult({ key: requestKey, tasks: [], assets: [], total: 0, error: e.message }) })
    }, 180)
    return () => { clearTimeout(timer); controller.abort() }
  }, [open, task, search, kind, page, requestKey])
  function choose(selection: AssetReferenceSelection['selection']) {
    if (!task) return
    onSelect({ taskId: task.id, version: task.version, name: task.name || task.id, selection, kind, query: search, assetIds: selection === 'selected' ? [...selected] : undefined, count: selection === 'selected' ? selected.size : total })
    onOpenChange(false)
  }
  const pageSize = task ? 50 : 20
  const content = <>
    <div className="flex items-center justify-between gap-3"><div><h3 className="text-sm font-medium">{l.title}</h3><p className="mt-1 text-xs text-muted-foreground">{l.hint}</p></div>{inline ? <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)} aria-label={l.close}>×</Button> : null}</div>
    {task ? <div className="space-y-1"><Button variant="ghost" size="sm" onClick={() => { setTask(null); setSearch(''); setKind(''); setPage(1); setSelected(new Set()) }}>{l.back}</Button><div className="break-all text-sm font-medium">{task.name || task.id} · v{task.version || '—'}</div>{['running', 'pending', 'recovering'].includes(task.status) ? <p className="text-xs text-muted-foreground">{l.collecting}</p> : null}</div> : null}
    <div className="flex gap-2"><Input autoFocus value={search} onChange={(event) => { setSearch(event.target.value); setPage(1) }} aria-label={task ? l.assets : l.search} placeholder={task ? l.assets : l.search} />{task ? <Select value={kind || 'all'} onValueChange={(value) => { setKind(value === 'all' ? '' : value); setPage(1) }}><SelectTrigger aria-label={l.all} className="w-40 shrink-0"><SelectValue /></SelectTrigger><SelectContent data-composer-command-portal><SelectItem value="all">{l.all}</SelectItem>{Object.entries(l.kinds).map(([key, label]) => <SelectItem key={key} value={key}>{label}</SelectItem>)}</SelectContent></Select> : null}</div>
    <div className="h-48 min-h-20 shrink overflow-y-auto rounded-lg border border-border" aria-busy={loading}>
      {loading ? <p className="p-4 text-sm text-muted-foreground" role="status">{l.loading}</p> : error ? <div className="p-4 text-sm" role="alert">{l.failed}: {error}<Button variant="outline" size="sm" onClick={() => setRetry((n) => n + 1)}>{l.retry}</Button></div> : total === 0 ? <p className="p-4 text-sm text-muted-foreground">{task ? l.unsupported : l.empty}</p> : task ? assets.map((asset) => <label key={asset.id} className="flex cursor-pointer items-start gap-3 border-b border-border p-3 text-sm last:border-0 hover:bg-muted/50"><Checkbox aria-label={asset.value} className="mt-1" checked={selected.has(asset.id)} onCheckedChange={(checked) => setSelected((old) => { const next = new Set(old); if (checked === true) next.add(asset.id); else next.delete(asset.id); return next })} /><span className="min-w-0"><span className="block break-all">{asset.value}</span><span className="block break-all text-xs text-muted-foreground">{l.kinds[asset.kind as keyof typeof l.kinds] || asset.kind} · {asset.detail} · v{asset.version}</span></span></label>) : tasks.map((item) => <Button key={`${item.id}:${item.version}`} type="button" variant="ghost" className="block h-auto w-full whitespace-normal border-b border-border p-3 text-left text-sm last:border-0 hover:bg-muted/50" onClick={() => { setTask(item); setSearch(''); setKind(''); setPage(1); setSelected(new Set()) }}><span className="block font-medium">{item.name || item.id}</span><span className="block truncate text-xs text-muted-foreground">{item.scanType} · {item.status} · {item.targets}</span><span className="block text-xs text-muted-foreground">{item.id} · v{item.version || '—'} · {formatReferenceDate(item.updatedAt, language)}</span></Button>)}
    </div>
    <div className="flex items-center justify-between text-xs text-muted-foreground"><span>{total} · {page} / {Math.max(1, Math.ceil(total / pageSize))}{task ? ` · ${l.selectedMode}: ${selected.size}` : ''}</span><div className="flex gap-2"><Button variant="ghost" size="sm" disabled={loading || page <= 1} onClick={() => setPage(page - 1)}>{l.previous}</Button><Button variant="ghost" size="sm" disabled={loading || page * pageSize >= total} onClick={() => setPage(page + 1)}>{l.next}</Button></div></div>
    {task ? <DialogFooter><Button variant="outline" disabled={loading || Boolean(error) || total === 0} onClick={() => choose(search || kind ? 'filtered' : 'all')}>{search || kind ? l.filtered : l.selectTask}</Button><Button disabled={loading || Boolean(error) || selected.size === 0} onClick={() => choose('selected')}>{l.selected} ({selected.size})</Button></DialogFooter> : null}
  </>
  if (inline) return <section role="region" aria-label={l.title} className="flex max-h-[min(32rem,60vh)] flex-col gap-3 overflow-y-auto rounded-2xl border border-border bg-popover p-4 text-popover-foreground shadow-2xl" onKeyDown={(event) => { if (event.key === 'Escape') { event.preventDefault(); onOpenChange(false) } }}>{content}</section>
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="max-w-3xl"><DialogHeader className="sr-only"><DialogTitle>{l.title}</DialogTitle><DialogDescription>{l.hint}</DialogDescription></DialogHeader>{content}</DialogContent></Dialog>
}
