'use client'

import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ReportContent } from './ReportContent'
import styles from './report.module.css'
import ScanFindingDetailClient from './ScanFindingDetailClient'
import { ArrowLeft, ArrowUpRight, Bug, Check, Copy, FileText, Link2, Loader2, RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { copyToClipboard } from '@/lib/clipboard'
import { formatDateTime } from '@/lib/date'
import { findingSeverityBadgeClassName, findingStatusBadgeClassName } from '@/lib/finding-badge'
import { findingDetailHref, findingReturnHref } from '@/lib/finding-detail'
import { findingRetestHref } from '@/lib/finding-retest'
import { useI18n } from '@/lib/i18n'
import { workbenchErrorMessage } from '@/lib/ai-workbench-i18n'
import { getFindingLabels } from '@/lib/finding-i18n'
import { cn } from '@/lib/utils'
import { buildFindingChainView, buildFindingLineageGraph } from '@/lib/exploration-graph'
import { getAIWorkbenchFinding, getAIWorkbenchFindingLineage, updateAIWorkbenchFindingStatus, type AIWorkbenchFindingLineage, type AIWorkbenchFindingReport, type AIWorkbenchFindingStatus } from '@/lib/api/ai-workbench'
import { ExplorationGraphView } from '../../ai/workbench/exploration/ExplorationClient'

const copy = {
  zh: {
    back: '返回', title: '漏洞详情', overview: '概览', chain: '证据链', share: '复制链接', refresh: '刷新', independentRetest: '独立复测', retestSession: '查看复测会话',
    summary: '摘要与影响', preconditions: '前置条件', steps: '复现步骤', evidence: '证据 / PoC', remediation: '修复建议', retest: '复测记录', aiRetest: 'AI 复测', retestResult: '复测结论', stillVulnerable: '仍存在风险', fixed: '已修复', inconclusive: '无法确认', noRetestHistory: '尚未记录 AI 复测结果。',
    noRetest: '尚未记录复测结论。', close: '关闭', empty: '尚未记录此项内容。', metadata: '漏洞信息', id: '漏洞 ID', severity: '严重等级', status: '处理状态', type: '漏洞类型', target: '涉及目标', project: '所属项目', conversation: '来源会话', discovered: '发现时间', updated: '更新时间', contents: '本页目录',
    copied: '已复制', copy: '复制', copyFailed: '复制失败', loading: '正在加载漏洞详情', failed: '无法加载漏洞详情', missingId: '链接中缺少漏洞 ID。', retry: '重试', statusSaved: '状态已更新',
    chainHint: '默认展示证据最完整的一条因果主链；其余相关意图与事实已折叠，需要时展开全部关联。原始工具执行保留在下方。', chainSteps: '主链 {count} 步', chainExpand: '展开全部关联（{count}）', chainCollapse: '只看主链', lineage: '探索链路', noLineage: '尚未记录结构化探索链路，可先查看概览中的原始证据。', legacyLineage: '当前仅有历史记录中的显式关联，未根据时间或标题推断缺失链路。', lineageError: '无法加载探索链路', lineagePartial: '部分上游节点不可用。', facts: '关联事实', executions: '原始工具执行', relationships: '事实关系', noFacts: '尚未关联结构化事实，可先查看概览中的原始证据。', noExecutions: '尚未记录可追溯的执行引用。', source: '来源', open: '查看原始记录', arguments: '调用参数', result: '执行结果', error: '执行错误', unavailable: '部分引用无法读取', limited: '关联链较长，当前展示部分上游事实。', linkHint: '正文中记录的原始内容，按需展开查看。', unknown: '未记录', noImpact: '尚未记录影响说明，请查阅下方复现步骤与证据。',
  },
  en: {
    back: 'Back', title: 'Finding details', overview: 'Overview', chain: 'Evidence chain', share: 'Copy link', refresh: 'Refresh', independentRetest: 'Independent retest', retestSession: 'View retest conversation',
    summary: 'Summary & impact', preconditions: 'Preconditions', steps: 'Reproduction', evidence: 'Evidence / PoC', remediation: 'Remediation', retest: 'Retest notes', aiRetest: 'AI retest', retestResult: 'Retest result', stillVulnerable: 'Still vulnerable', fixed: 'Fixed', inconclusive: 'Inconclusive', noRetestHistory: 'No AI retest result recorded.',
    noRetest: 'No retest conclusion recorded.', close: 'Close', empty: 'No content recorded.', metadata: 'Finding information', id: 'Finding ID', severity: 'Severity', status: 'Status', type: 'Vulnerability type', target: 'Target', project: 'Project', conversation: 'Source conversation', discovered: 'Discovered', updated: 'Updated', contents: 'On this page',
    copied: 'Copied', copy: 'Copy', copyFailed: 'Copy failed', loading: 'Loading finding details', failed: 'Unable to load finding', missingId: 'The link has no finding ID.', retry: 'Retry', statusSaved: 'Status updated',
    chainHint: 'The default view is the single causal chain with the richest recorded evidence; other related intents and facts stay folded. Original tool executions remain below.', chainSteps: 'Chain · {count} steps', chainExpand: 'Show related records ({count})', chainCollapse: 'Back to chain', lineage: 'Exploration lineage', noLineage: 'No structured exploration lineage is recorded. See the original evidence in Overview.', legacyLineage: 'Only explicit legacy references are shown; missing links are never inferred from time or titles.', lineageError: 'Unable to load exploration lineage', lineagePartial: 'Some upstream nodes are unavailable.', facts: 'Linked facts', executions: 'Original tool executions', relationships: 'Fact relationships', noFacts: 'No structured facts linked. See the original evidence in Overview.', noExecutions: 'No traceable execution references recorded.', source: 'Source', open: 'View original record', arguments: 'Arguments', result: 'Result', error: 'Execution error', unavailable: 'Some references are unavailable', limited: 'This is a partial view of a long upstream chain.', linkHint: 'Original recorded content. Expand to inspect.', unknown: 'Not recorded', noImpact: 'No impact statement recorded. See reproduction and evidence below.',
  },
}
type Labels = typeof copy.zh
const dispositionStatuses: AIWorkbenchFindingStatus[] = ['open', 'in_progress', 'fixed', 'false_positive', 'ignored']
function normalizedStatus(value: string): AIWorkbenchFindingStatus { return (value === 'confirmed' ? 'in_progress' : value === 'accepted_risk' ? 'ignored' : value) as AIWorkbenchFindingStatus }
function CopyButton({ value, label, l }: { value: string; label?: string; l: Labels }) {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])
  return <Button variant="ghost" size="sm" className="h-8 shrink-0 gap-1.5 text-xs text-muted-foreground" aria-label={label || l.copy} onClick={async () => {
    if (!await copyToClipboard(value)) { toast.error(l.copyFailed); return }
    setCopied(true); if (timer.current) clearTimeout(timer.current); timer.current = setTimeout(() => setCopied(false), 1800)
  }}>{copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}{copied ? l.copied : label || l.copy}</Button>
}

function Markdown({ value, l }: { value: string; l: Labels }) {
  return <ReportContent value={value} language={l === copy.zh ? 'zh' : 'en'} />
}

function Section({ id, title, children }: { id: string; title: string; children: ReactNode }) {
  return <section id={id} className="scroll-mt-6 rounded-2xl border border-border bg-card p-5 sm:p-6"><h2 className="mb-4 text-[15px] font-semibold">{title}</h2>{children}</section>
}

function FindingFactsDrawer({ facts, l, onClose }: { facts: AIWorkbenchFindingReport['facts']; l: Labels; onClose: () => void }) {
  return <aside aria-label={l.facts} className="absolute inset-y-0 right-0 z-20 w-[min(560px,88%)] overflow-y-auto border-l border-border bg-card p-5 shadow-2xl xl:p-7">
    <div className="mb-5 flex items-center justify-between"><h2 className="text-base font-semibold">{l.facts}</h2><Button variant="ghost" size="icon" aria-label={l.close} onClick={onClose}>×</Button></div>
    {facts.length ? <ol className="space-y-5">{facts.map((fact, index) => <li key={fact.factKey} className="rounded-xl border border-border bg-muted/10 p-4"><div className="flex items-start gap-3"><span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-border text-xs text-muted-foreground">{index + 1}</span><div className="min-w-0"><h3 className="text-sm font-semibold leading-6">{fact.summary}</h3><div className="my-2 flex flex-wrap gap-2 text-xs text-muted-foreground"><Badge variant="outline" className="font-normal">{fact.confidence}</Badge><span>{formatDateTime(fact.createdAt)}</span></div><p className="break-all font-mono text-xs text-muted-foreground">{fact.factKey}</p>{fact.sourceConversationId && <Link href={`/ai/workbench?session=${encodeURIComponent(fact.sourceConversationId)}`} className="mt-2 inline-flex items-center gap-1 text-xs underline underline-offset-4">{l.conversation}<ArrowUpRight className="h-3 w-3" /></Link>}</div></div>{fact.body && <details className="mt-4 rounded-lg border p-3"><summary className="cursor-pointer text-sm">{l.open}</summary><div className="mt-3"><Markdown value={fact.body} l={l} /></div></details>}</li>)}</ol> : <p className="text-sm text-muted-foreground">{l.noFacts}</p>}
  </aside>
}

export default function FindingDetailClient() {
  const params = useSearchParams()
  const id = params.get('id') || ''
  if (params.get('source') === 'web' || params.get('source') === 'js') return <ScanFindingDetailClient key={params.toString()} />
  return <FindingDetailContent key={id} id={id} back={findingReturnHref(params.get('from'))} />
}

function FindingDetailContent({ id, back }: { id: string; back: string }) {
  const { language, t } = useI18n()
  const router = useRouter()
  const labels = getFindingLabels(t.vulnerabilityManagement)
  const l = copy[language]
  const [data, setData] = useState<AIWorkbenchFindingReport | null>(null)
  const canonicalID = data?.view.id || id
  const [loading, setLoading] = useState(Boolean(id))
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const [tab, setTab] = useState('overview')
  const [lineage, setLineage] = useState<AIWorkbenchFindingLineage | null>(null)
  const [lineageLoading, setLineageLoading] = useState(false)
  const [lineageError, setLineageError] = useState('')
  const [lineageSelected, setLineageSelected] = useState<string | null>(null)
  const [lineageReload, setLineageReload] = useState(0)
  const [chainExpanded, setChainExpanded] = useState(false)
  // Evidence chain: one causal path with the richest evidence, related records folded.
  const chainView = useMemo(() => lineage ? buildFindingChainView(lineage, canonicalID) : null, [lineage, canonicalID])
  const fullLineageGraph = useMemo(() => lineage ? buildFindingLineageGraph(lineage) : { nodes: [], edges: [] }, [lineage])
  const showFullLineage = chainExpanded || !chainView || chainView.fallback
  const relatedCount = chainView ? chainView.relatedVisible + chainView.relatedEvidence : 0
  const [activeSection, setActiveSection] = useState('summary')
  const pageRef = useRef<HTMLDivElement>(null)
  const generation = useRef(0)
  const load = useCallback((signal?: AbortSignal) => {
    const request = ++generation.current
    if (!id) return Promise.resolve()
    return getAIWorkbenchFinding(id)
      .then(next => { if (!signal?.aborted && request === generation.current) setData(next) })
      .catch((e: unknown) => { if (!signal?.aborted && request === generation.current) { setData(null); setError(workbenchErrorMessage(e, t.aiWorkbench, l.failed)) } })
      .finally(() => { if (!signal?.aborted && request === generation.current) setLoading(false) })
  }, [id, l, t.aiWorkbench])
  useEffect(() => { const controller = new AbortController(); void load(controller.signal); return () => controller.abort() }, [load])
  useEffect(() => {
    if (tab !== 'chain' || !id) return
    const controller = new AbortController()
    setLineageLoading(true)
    setLineageError('')
    void getAIWorkbenchFindingLineage(id)
      .then(next => { if (!controller.signal.aborted) setLineage(next) })
      .catch((e: unknown) => { if (!controller.signal.aborted) { setLineage(null); setLineageError(workbenchErrorMessage(e, t.aiWorkbench, l.lineageError)) } })
      .finally(() => { if (!controller.signal.aborted) setLineageLoading(false) })
    return () => controller.abort()
  }, [id, l.lineageError, tab, t.aiWorkbench, lineageReload])
  const reload = () => { setLoading(true); setError(''); setLineage(null); setLineageReload(value => value + 1); void load() }
  const displayError = id ? error : l.missingId
  const changeStatus = async (status: string) => {
    setSaving(true)
    try { const next = await updateAIWorkbenchFindingStatus(id, status as AIWorkbenchFindingStatus); setData(previous => previous ? { ...previous, view: { ...previous.view, ...next } } : previous); toast.success(l.statusSaved) }
    catch (e) { toast.error(workbenchErrorMessage(e, t.aiWorkbench, l.failed)) } finally { setSaving(false) }
  }
  useEffect(() => {
    if (!data || !pageRef.current) return
    const ids = tab === 'overview'
      ? ['summary', 'preconditions', 'steps', 'evidence', 'remediation', 'retest']
      : ['lineage']
    let scrollHost = pageRef.current.parentElement
    while (scrollHost && !/(auto|scroll)/.test(getComputedStyle(scrollHost).overflowY)) scrollHost = scrollHost.parentElement
    const host = scrollHost || document.documentElement
    const scrollTarget = scrollHost || window
    let frame = 0
    const update = () => {
      const top = (scrollHost ? scrollHost.getBoundingClientRect().top : 0) + 28
      let current = ids[0]
      for (const section of ids) {
        if ((document.getElementById(`finding-${section}`)?.getBoundingClientRect().top ?? Infinity) <= top) current = section
      }
      if (host.scrollTop > 0 && host.scrollHeight - host.clientHeight - host.scrollTop <= 2) current = ids[ids.length - 1]
      setActiveSection(current)
    }
    const schedule = () => { cancelAnimationFrame(frame); frame = requestAnimationFrame(update) }
    scrollTarget.addEventListener('scroll', schedule, { passive: true })
    window.addEventListener('resize', schedule)
    const observer = new ResizeObserver(schedule)
    observer.observe(pageRef.current)
    schedule()
    return () => { cancelAnimationFrame(frame); observer.disconnect(); scrollTarget.removeEventListener('scroll', schedule); window.removeEventListener('resize', schedule) }
  }, [data, tab])
  const f = data?.view
  const status = f ? normalizedStatus(f.status) : 'open'
  const sections = ['summary', 'preconditions', 'steps', 'evidence', 'remediation', 'retest'] as const
  return <div ref={pageRef} className="min-h-full bg-background text-foreground">
    <header className="border-b border-border bg-background px-5 py-5 lg:px-8">
      <div className="flex flex-wrap items-center gap-3">
        <Button asChild variant="ghost" size="sm" className="-ml-2 gap-1.5"><Link href={back}><ArrowLeft className="h-4 w-4" />{l.back}</Link></Button>
        <span className="hidden h-5 w-px bg-border sm:block" /><Bug className="hidden h-5 w-5 shrink-0 text-muted-foreground sm:block" />
        <h1 className="order-3 w-full min-w-0 line-clamp-2 break-words text-lg font-semibold leading-7 sm:order-none sm:w-auto sm:flex-1 sm:text-xl">{f?.name || l.title}</h1>
        <div className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-1"><Button variant="outline" size="sm" className="gap-1.5" disabled={!f} onClick={() => router.push(findingRetestHref(canonicalID))}><RefreshCw className="h-4 w-4" />{l.aiRetest}</Button><Button variant="ghost" size="sm" disabled={!f} onClick={() => router.push(findingRetestHref(canonicalID, true))}>{l.independentRetest}</Button><Button variant="ghost" size="sm" className="gap-1.5 text-muted-foreground" disabled={!f} onClick={async () => { const url = new URL(window.location.href); url.searchParams.delete('from'); url.searchParams.set('id', canonicalID); if (await copyToClipboard(url.toString())) toast.success(l.copied); else toast.error(l.copyFailed) }}><Link2 className="h-4 w-4" />{l.share}</Button><Button variant="ghost" size="icon" disabled={loading} aria-label={l.refresh} onClick={reload}><RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} /></Button></div>
      </div>
      {f && <div className="mt-3 flex flex-wrap items-center gap-2"><Badge variant="outline" className={findingSeverityBadgeClassName(f.severity)}>{labels.severity[f.severity] || f.severity}</Badge><Badge variant="outline" className={cn(findingStatusBadgeClassName(status), 'font-normal')}>{labels.status[status] || f.status}</Badge><span className="break-all font-mono text-xs text-muted-foreground">#{canonicalID}</span></div>}
    </header>
    {loading && !data ? <div role="status" className="flex items-center justify-center gap-3 p-16 text-sm text-muted-foreground"><Loader2 className="h-5 w-5 animate-spin" />{l.loading}</div> : displayError ? <div role="alert" className="m-6 rounded-xl border bg-card p-8 text-center"><h2 className="font-semibold">{l.failed}</h2><p className="my-3 text-sm text-muted-foreground">{displayError}</p>{id && <Button variant="outline" onClick={reload}>{l.retry}</Button>}</div> : f && data && <Tabs value={tab} onValueChange={setTab} className="gap-0">
      <div className="border-b bg-background px-5 pb-3 pt-3 lg:px-8"><TabsList className="h-9 bg-muted/50"><TabsTrigger value="overview" className="gap-2"><FileText className="h-3.5 w-3.5" />{l.overview}</TabsTrigger><TabsTrigger value="chain" className="gap-2"><Link2 className="h-3.5 w-3.5" />{l.chain}</TabsTrigger></TabsList></div>
      <div className={cn('mx-auto grid w-full max-w-[1600px] items-start gap-6 p-4 md:p-6', tab === 'overview' ? 'xl:grid-cols-[minmax(0,1fr)_256px] 2xl:gap-9 2xl:grid-cols-[minmax(0,1fr)_280px]' : 'grid-cols-1')}>
        <main className="min-w-0">
          <TabsContent value="overview" className={cn("mt-0", styles.report)}>
            {sections.map(section => {
              const value = section === 'summary' ? f.impact : section === 'steps' ? f.reproductionSteps : section === 'remediation' ? f.remediation : section === 'retest' ? f.retestNotes : f[section]
              return <section key={section} id={`finding-${section}`} className={cn(styles.section, section === 'summary' && styles.summary)}>
                <div className={styles.sectionHeader}><h2>{l[section]}</h2>{value?.trim() && <CopyButton value={value} label={language === 'zh' ? '复制本节' : 'Copy section'} l={l} />}</div>
                {value?.trim() ? <Markdown value={value} l={l} /> : <p className="text-sm leading-7 text-muted-foreground">{section === 'retest' ? l.noRetest : section === 'summary' ? l.noImpact : l.empty}</p>}
                {section === 'retest' && <div className="mt-5 space-y-3 border-t border-border/70 pt-5"><h3 className="text-sm font-semibold">{l.retestResult}</h3>{data.retests?.length ? data.retests.map((item) => {
                  const resultLabel = item.result === 'still_vulnerable' ? l.stillVulnerable : item.result === 'fixed' ? l.fixed : l.inconclusive
                  const resultClass = item.result === 'fixed' ? 'border-green-500/30 bg-green-500/10 text-green-300' : item.result === 'still_vulnerable' ? 'border-red-500/30 bg-red-500/10 text-red-300' : 'border-amber-500/30 bg-amber-500/10 text-amber-200'
                  return <article key={item.id} className="rounded-xl border border-border bg-muted/10 p-4"><div className="flex flex-wrap items-center justify-between gap-2"><Badge variant="outline" className={resultClass}>{resultLabel}</Badge><time className="text-xs text-muted-foreground">{formatDateTime(item.createdAt)}</time></div><p className="mt-3 whitespace-pre-wrap break-words text-sm leading-6">{item.summary}</p>{item.conversationId && <Link href={`/ai/workbench?session=${encodeURIComponent(item.conversationId)}`} className="mt-2 inline-flex items-center gap-1 text-xs text-muted-foreground underline underline-offset-4">{l.retestSession}<ArrowUpRight className="h-3 w-3" /></Link>}{item.evidence && <details className="mt-3 rounded-lg border border-border/70 p-3"><summary className="cursor-pointer text-xs text-muted-foreground">{l.evidence}</summary><div className="mt-3"><Markdown value={item.evidence} l={l} /></div></details>}</article>
                }) : <p className="text-sm text-muted-foreground">{l.noRetestHistory}</p>}</div>}
              </section>
            })}
            {!!data.dispositions?.length && <section className={styles.section}>
              <div className={styles.sectionHeader}><h2>{language === 'zh' ? '处置记录' : 'Disposition history'}</h2></div>
              <ol className="space-y-4">{data.dispositions.map(item => <li key={item.id} className="rounded-lg border border-border bg-muted/20 p-4">
                <p className="text-sm font-medium">{labels.status[normalizedStatus(item.previousStatus)] || item.previousStatus} → {labels.status[normalizedStatus(item.status)] || item.status}</p>
                <p className="mt-1 text-xs text-muted-foreground">{formatDateTime(item.createdAt)} · {item.actor || '—'}</p>
                {item.note && <p className="mt-3 whitespace-pre-wrap break-words text-sm">{item.note}</p>}
              </li>)}</ol>
            </section>}
          </TabsContent>
          <TabsContent value="chain" className="mt-0 space-y-5">
            <p className="px-1 text-sm leading-6 text-muted-foreground">{l.chainHint}</p>
            {lineageError && <div role="alert" className="rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm"><p>{lineageError}</p><Button variant="outline" size="sm" className="mt-3" onClick={() => setLineageReload(value => value + 1)}>{l.retry}</Button></div>}
            {lineageLoading && !lineage && <div role="status" className="flex min-h-[420px] items-center justify-center rounded-2xl border border-border bg-card text-sm text-muted-foreground"><Loader2 className="mr-2 h-4 w-4 animate-spin" />{l.loading}</div>}
            {!lineageLoading && lineage && lineage.nodes.length > 0 && <>
              {lineage.mode === 'explicit_legacy' && <div className="rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm text-muted-foreground">{l.legacyLineage}</div>}
              {((lineage.missingRefs?.length ?? 0) > 0 || lineage.truncated) && <div className="rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm"><p>{lineage.truncated ? l.limited : l.lineagePartial}</p>{(lineage.missingRefs ?? []).map(ref => <p key={ref} className="mt-2 break-all font-mono text-xs text-muted-foreground">{ref}</p>)}</div>}
              <section id="finding-lineage" className="overflow-hidden rounded-2xl border border-border bg-card">
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-5 py-4">
                  <div className="flex flex-wrap items-baseline gap-3"><h2 className="text-[15px] font-semibold">{l.lineage}</h2>{chainView && !chainView.fallback && <span className="text-xs text-muted-foreground">{l.chainSteps.replace('{count}', String(chainView.chainLength))}</span>}</div>
                  {chainView && !chainView.fallback && <Button variant="outline" size="sm" onClick={() => setChainExpanded(value => !value)}>{showFullLineage ? l.chainCollapse : l.chainExpand.replace('{count}', String(relatedCount))}</Button>}
                </div>
                <div className="relative min-h-[560px]"><ExplorationGraphView graph={showFullLineage ? fullLineageGraph : chainView!.graph} selected={lineageSelected} onSelect={setLineageSelected} returnHref={findingDetailHref(canonicalID, back)} showAllRelations showRelationToggle={chainExpanded} hideDetails showFactLegend={false} fitKey={showFullLineage ? 'full' : 'chain'} />{lineageSelected === `finding:${canonicalID}` && <FindingFactsDrawer facts={data.facts} l={l} onClose={() => setLineageSelected(null)} />}</div>
              </section>
            </>}
            {!lineageLoading && lineage && lineage.nodes.length === 1 && lineage.mode === 'empty' && <p className="px-1 text-sm text-muted-foreground">{l.noLineage}</p>}
            {(data.missingEvidenceRefs.length > 0 || data.chainTruncated) && <div className="rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm"><p>{data.chainTruncated ? l.limited : l.unavailable}</p>{data.missingEvidenceRefs.map(ref => <p key={ref} className="mt-2 break-all font-mono text-xs text-muted-foreground">{ref}</p>)}</div>}
          </TabsContent>
        </main>
        {tab === 'overview' && <aside className="flex min-h-0 flex-col gap-5 xl:sticky xl:top-6 xl:max-h-[calc(100dvh-128px)]">
          <nav aria-label={l.contents} className="hidden shrink-0 border-b border-border/60 px-2 pb-5 xl:block"><h2 className="mb-3 text-xs font-semibold tracking-wide text-muted-foreground">{l.contents}</h2><ul className="space-y-1">{(tab === 'overview' ? sections : ['lineage', 'facts', ...(data.edges.length ? ['relationships'] : []), 'executions'] as const).map(section => <li key={section}><button aria-current={activeSection === section ? 'location' : undefined} className="w-full rounded-lg px-3 py-2 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground aria-[current=location]:bg-muted aria-[current=location]:font-medium aria-[current=location]:text-foreground" onClick={() => document.getElementById(`finding-${section}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })}>{l[section as keyof Labels]}</button></li>)}</ul></nav>
          <section className="min-h-0 px-2 py-2 xl:overflow-y-auto xl:overscroll-contain"><h2 className="mb-3 text-xs font-semibold tracking-wide text-muted-foreground">{l.metadata}</h2><dl className="divide-y divide-border text-sm">
            <Meta label={l.id}><span className="break-all font-mono text-xs">{canonicalID}</span></Meta>
            <Meta label={l.status}><Select value={status} disabled={saving} onValueChange={changeStatus}><SelectTrigger aria-label={l.status} className="h-8 w-full border-0 bg-muted/40 text-xs shadow-none"><SelectValue /></SelectTrigger><SelectContent>{dispositionStatuses.map(value => <SelectItem key={value} value={value}>{labels.status[value]}</SelectItem>)}</SelectContent></Select></Meta>
            <Meta label={l.type}>{f.vulnerabilityType || l.unknown}</Meta>
            <Meta label={l.target}><span className="whitespace-pre-wrap break-all font-mono text-xs">{f.rawTarget || f.url || l.unknown}</span></Meta>
            <Meta label={l.project}>{data.project.name}</Meta>
            <Meta label={l.conversation}>{data.conversation ? <Link className="underline underline-offset-4" href={`/ai/workbench?session=${encodeURIComponent(data.conversation.id)}`}>{data.conversation.title || data.conversation.id}<ArrowUpRight className="ml-1 inline h-3 w-3" /></Link> : l.unknown}</Meta>
            <Meta label={l.discovered}>{formatDateTime(f.createdAt || '')}</Meta><Meta label={l.updated}>{formatDateTime(f.updatedAt || '')}</Meta>
          </dl></section>
        </aside>}
      </div>
    </Tabs>}
  </div>
}

function Meta({ label, children }: { label: string; children: ReactNode }) {
  return <div className="space-y-1.5 py-3"><dt className="text-xs leading-6 text-muted-foreground">{label}</dt><dd className="min-w-0 break-words text-xs leading-6">{children}</dd></div>
}
