'use client'

import { useCallback, useEffect, useState } from 'react'
import { getAIWorkbenchPenTestExecutionEvidence, type AIWorkbenchExecutionEvidence, type AIWorkbenchExplorationExecution } from '@/lib/api/ai-workbench'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import { workbenchErrorMessage } from '@/lib/ai-workbench-i18n'
import toast from 'react-hot-toast'

type ExecutionEvidenceProps = {
  conversationId: string
  executionId: string
  language: 'zh' | 'en'
  autoLoad?: boolean
  initialData?: AIWorkbenchExecutionEvidence
  onLoaded?: (data: AIWorkbenchExecutionEvidence) => void
  tabbed?: boolean
}

const statusNames: Record<string, [string, string]> = {
  pending: ['待执行', 'Pending'], running: ['执行中', 'Running'], completed: ['已完成', 'Completed'],
  failed: ['失败', 'Failed'], cancelled: ['已取消', 'Cancelled'], canceled: ['已取消', 'Cancelled'],
  interrupted: ['已中断', 'Interrupted'], timeout: ['超时', 'Timed out'],
}
const statusName = (status: string, language: 'zh' | 'en') => statusNames[status]?.[language === 'zh' ? 0 : 1] || status
const formatExecutionTime = (value: string | null | undefined, language: 'zh' | 'en') => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(language === 'zh' ? 'zh-CN' : 'en-GB', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date)
}
const serializeEvidence = (value: unknown) => typeof value === 'string' ? value : JSON.stringify(value, null, 2) || ''

// Reset the reader on identity changes, including callers outside the exploration graph.
export function ExecutionEvidence(props: ExecutionEvidenceProps) {
  return <ExecutionEvidenceRecord key={`${props.conversationId}:${props.executionId}`} {...props} />
}

function EvidenceBlock({ title, raw, language }: { title: string; raw: string; language: 'zh' | 'en' }) {
  const zh = language === 'zh'
  return <figure className="min-w-0 overflow-hidden rounded-lg border">
    <figcaption className="flex items-center justify-between border-b bg-muted/40 px-3 py-1 text-xs">
      <span>{title}</span>
      <Button variant="ghost" size="sm" className="h-7 px-2 text-xs" aria-label={`${zh ? '复制' : 'Copy'}${title}`} onClick={async () => {
        if (await copyToClipboard(raw)) toast.success(zh ? '已复制' : 'Copied')
        else toast.error(zh ? '复制失败' : 'Copy failed')
      }}>{zh ? '复制' : 'Copy'}</Button>
    </figcaption>
    <pre tabIndex={0} aria-label={title} className="max-h-[420px] overflow-auto bg-muted/20 p-3 font-mono text-xs leading-6"><code>{raw}</code></pre>
  </figure>
}

function ExecutionEvidenceRecord({ conversationId, executionId, language, autoLoad = false, initialData, onLoaded, tabbed = false }: ExecutionEvidenceProps) {
  const { t } = useI18n()
  const zh = language === 'zh'
  const [data, setData] = useState(initialData)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(autoLoad && !initialData)
  const [attempt, setAttempt] = useState(autoLoad ? 0 : -1)
  useEffect(() => {
    if (initialData || attempt < 0) return
    const controller = new AbortController()
    getAIWorkbenchPenTestExecutionEvidence(conversationId, executionId, controller.signal)
      .then(next => {
        if (controller.signal.aborted) return
        setData(next)
        setLoading(false)
        onLoaded?.(next)
      })
      .catch((e: unknown) => {
        if (!controller.signal.aborted) setError(workbenchErrorMessage(e, t.aiWorkbench, t.aiWorkbench.loadWorkbenchFailed))
      })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [attempt, conversationId, executionId, initialData, onLoaded, t.aiWorkbench])
  const panes = data ? [
    { id: 'arguments', label: zh ? '调用参数' : 'Arguments', value: serializeEvidence(data.arguments) },
    { id: 'result', label: zh ? '执行结果' : 'Result', value: serializeEvidence(data.result) },
    ...(data.error ? [{ id: 'error', label: zh ? '执行错误' : 'Error', value: data.error }] : []),
  ] : []
  return <section className={tabbed ? 'min-w-0 space-y-3' : 'my-5 min-w-0 space-y-4'} aria-busy={loading}>
    {!data && <Button variant="outline" size="sm" disabled={loading} onClick={() => { setLoading(true); setError(''); setAttempt(previous => previous + 1) }}>{loading ? (zh ? '读取中…' : 'Loading…') : error ? (zh ? '重新读取' : 'Retry') : (zh ? '读取原始执行证据' : 'Load original execution')}</Button>}
    {error && <p role="alert" className="text-sm text-amber-600">{error}</p>}
    {data && <>
      <div className="space-y-1">
        <div className="flex items-start justify-between gap-2 text-sm font-medium">
          <span className="min-w-0 break-all">{data.toolName}</span>
          <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[11px] font-normal text-muted-foreground">{statusName(data.status, language)}</span>
        </div>
        <p className="text-[11px] text-muted-foreground">{formatExecutionTime(data.startedAt, language)}{data.completedAt && ` → ${formatExecutionTime(data.completedAt, language)}`}</p>
      </div>
      {tabbed ? <Tabs defaultValue="arguments" className="min-w-0 gap-3">
        <TabsList aria-label={zh ? '执行记录内容' : 'Execution content'} className="h-9 w-full">
          {panes.map(item => <TabsTrigger key={item.id} value={item.id} className="px-2 py-1 text-xs focus-visible:ring-muted-foreground">{item.label}</TabsTrigger>)}
        </TabsList>
        {panes.map(item => <TabsContent key={item.id} value={item.id} className="min-w-0"><EvidenceBlock title={item.label} raw={item.value} language={language} /></TabsContent>)}
      </Tabs> : panes.map(item => <EvidenceBlock key={item.id} title={item.label} raw={item.value} language={language} />)}
    </>}
  </section>
}

export function ExecutionEvidenceList({ conversationId, executions, language }: { conversationId: string; executions: AIWorkbenchExplorationExecution[]; language: 'zh' | 'en' }) {
  const zh = language === 'zh'
  const [selection, setSelection] = useState(executions[0]?.id || '')
  const [records, setRecords] = useState<Record<string, AIWorkbenchExecutionEvidence>>({})
  const [visibleCount, setVisibleCount] = useState(20)
  const selected = executions.find(item => item.id === selection) || executions[0]
  const selectedIndex = selected ? executions.indexOf(selected) : -1
  // Completed evidence is cached; running records are reread when revisited.
  // Metadata changes from graph refreshes invalidate a cached snapshot.
  const cached = selected ? records[selected.id] : undefined
  const selectedData = cached?.completedAt && (!selected.status || selected.status === cached.status)
    && (!selected.completedAt || selected.completedAt === cached.completedAt) ? cached : undefined
  const remember = useCallback((data: AIWorkbenchExecutionEvidence) => {
    setRecords(previous => ({ ...previous, [data.id]: data }))
  }, [])
  return <section className="my-4 min-w-0 space-y-3" aria-label={zh ? '工具执行记录' : 'Tool executions'}>
    <div className="flex items-center justify-between gap-2"><h3 className="text-sm font-semibold">{zh ? '工具执行记录' : 'Tool executions'}</h3><span className="text-xs text-muted-foreground">{executions.length} {zh ? '条' : 'records'}</span></div>
    <div className="max-h-52 overflow-y-auto rounded-lg border" role="group" aria-label={zh ? '选择执行记录' : 'Select execution'}>
      {executions.slice(0, visibleCount).map((execution, index) => {
        const record = records[execution.id]
        const active = execution.id === selected?.id
        const name = execution.toolName || record?.toolName || (zh ? '执行记录' : 'Execution')
        const status = execution.status || record?.status
        return <button type="button" key={execution.id} title={`${name} · ${execution.id}`} className={`flex w-full items-center gap-2 border-b px-3 py-2.5 text-left transition-colors last:border-b-0 focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-muted-foreground ${active ? 'bg-muted' : 'hover:bg-muted/60'}`} aria-pressed={active} onClick={() => setSelection(execution.id)}>
          <span className="w-6 shrink-0 text-[11px] tabular-nums text-muted-foreground">{String(index + 1).padStart(2, '0')}</span>
          <span className="min-w-0 flex-1"><span className="block truncate text-xs font-medium">{name}</span><span className="block truncate font-mono text-[10px] text-muted-foreground">{formatExecutionTime(execution.startedAt || record?.startedAt, language) || execution.id.slice(0, 12)}</span></span>
          {status && <span className="shrink-0 text-[11px] text-muted-foreground">{statusName(status, language)}</span>}
        </button>
      })}
    </div>
    {executions.length > visibleCount && <Button variant="ghost" size="sm" className="w-full text-xs" onClick={() => setVisibleCount(previous => previous + 20)}>{zh ? `显示更多（已显示 ${visibleCount} / ${executions.length}）` : `Show more (${visibleCount} / ${executions.length})`}</Button>}
    {selected && <div className="min-w-0 space-y-2 border-t pt-3">
      <p className="text-[11px] text-muted-foreground">{zh ? `第 ${selectedIndex + 1} 条记录` : `Record ${selectedIndex + 1}`}</p>
      <ExecutionEvidence key={`${selected.id}:${selected.status || ''}:${selected.completedAt || ''}`} conversationId={conversationId} executionId={selected.id} language={language} autoLoad tabbed initialData={selectedData} onLoaded={remember} />
    </div>}
  </section>
}
