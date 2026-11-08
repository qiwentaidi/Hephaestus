'use client'

import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectTrigger, SelectContent, SelectItem, SelectValue } from '@/components/ui/select'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { getAIWorkbenchPenTestProjectBlackboard, type AIWorkbenchPenTestSessionBlackboard as Blackboard } from '@/lib/api/ai-workbench'
import { buildExplorationGraph, explorationHref, type ExplorationNode, type ExplorationGraph } from '@/lib/exploration-graph'
import { projectExplorationHref, scopeProjectBoard } from '@/lib/project-exploration'
import { findingDetailHref } from '@/lib/finding-detail'
import { useI18n } from '@/lib/i18n'
import { workbenchErrorMessage } from '@/lib/ai-workbench-i18n'
import { ExplorationGraphView, kinds, stateNames, relationNames } from '../exploration/ExplorationClient'
import { ExecutionEvidence } from '../components/ExecutionEvidence'
import { PlanWorkspace } from './components/PlanWorkspace'
import { ReportContent } from '../../../vulnerabilities/detail/ReportContent'
import graphStyles from '../exploration/exploration.module.css'
import styles from './project.module.css'

const tabNames = { overview: ['概览', 'Overview'], graph: ['探索链路', 'Exploration'], plan: ['编排', 'Plan'], evidence: ['证据', 'Evidence'] }
type SourceConversation = NonNullable<Blackboard['sourceConversations']>[number]
/** Labels are capped so one long session title cannot stretch the filter. */
const SOURCE_LABEL_MAX = 40
function fullSourceLabel(source: SourceConversation): string {
  const name = source.title || source.target || source.id
  return source.agentRole === 'worker' ? `${name} · Worker` : name
}
function clipSourceLabel(label: string): string {
  return label.length > SOURCE_LABEL_MAX ? `${label.slice(0, SOURCE_LABEL_MAX - 1).trimEnd()}…` : label
}
export default function ProjectClient() {
  const params = useSearchParams()
  const id = params.get('project') || ''
  return <ProjectPage key={id} id={id} />
}

function ProjectPage({ id }: { id: string }) {
  const router = useRouter(), params = useSearchParams()
  const { language, t } = useI18n(), zh = language === 'zh'
  const rawTab = params.get('tab') || 'overview'
  const tab = Object.prototype.hasOwnProperty.call(tabNames, rawTab) ? rawTab : 'overview'
  const source = params.get('source') || 'all', selected = params.get('node')
  const [board, setBoard] = useState<Blackboard | null>(null), [error, setError] = useState(''), [loading, setLoading] = useState(true)
  const request = useRef(0), signature = useRef('')
  const load = useCallback((signal?: AbortSignal) => {
    const generation = ++request.current
    const pending = id ? getAIWorkbenchPenTestProjectBlackboard(id) : Promise.reject(new Error(zh ? '链接缺少项目 ID' : 'Missing project ID'))
    return pending.then(next => {
      if (signal?.aborted || generation !== request.current) return
      const sig = JSON.stringify({ ...next, updatedAt: '' })
      if (sig !== signature.current) { signature.current = sig; setBoard(next) }
      setError('')
    }).catch((e: unknown) => {
      if (!signal?.aborted && generation === request.current) setError(workbenchErrorMessage(e, t.aiWorkbench, t.aiWorkbench.loadWorkbenchFailed))
    }).finally(() => {
      if (!signal?.aborted && generation === request.current) setLoading(false)
    })
  }, [id, zh, t.aiWorkbench])
  useEffect(() => {
    const controller = new AbortController()
    void load(controller.signal)
    const timer = setInterval(() => { if (!document.hidden) void load(controller.signal) }, 20000)
    return () => { clearInterval(timer); controller.abort() }
  }, [load])
  const scoped = useMemo(() => board ? scopeProjectBoard(board, source) : null, [board, source])
  const graph = useMemo(() => scoped ? buildExplorationGraph(scoped) : { nodes: [], edges: [] }, [scoped])
  const jump = (nextTab: string, node: string | null = selected, nextSource = source) => router.push(projectExplorationHref(id, nextTab, node || undefined, nextSource), { scroll: false })
  const href = projectExplorationHref(id, tab, selected || undefined, source)
  const selectedSource = source === 'all' ? undefined : board?.sourceConversations?.find(item => item.id === source)
  const facts = graph.nodes.filter(n => n.factKey && n.kind !== 'missing')
  const blockers = graph.nodes.filter(n => n.kind === 'blocker')
  const findings = graph.nodes.filter(n => n.kind === 'finding')
  const confirmed = findings.filter(n => n.status === 'confirmed')
  const candidates = findings.filter(n => !['confirmed', 'fixed', 'false_positive', 'accepted_risk'].includes(n.status || ''))
  const evidenceNodes = graph.nodes.filter(n => n.factKey || n.findingId)
  const label = (node: ExplorationNode) => stateNames[node.status || '']?.[zh ? 0 : 1] || node.status
  const row = (node: ExplorationNode, nextTab = 'evidence') => <button key={node.id} className={styles.row} onClick={() => jump(nextTab, node.id)}><span className="flex items-start justify-between gap-3"><span className="text-sm font-medium leading-6">{node.title}</span>{node.status && <Badge variant="outline" className="shrink-0 bg-muted/40 text-muted-foreground">{label(node)}</Badge>}</span>{node.sourceTitle && <span className="mt-2 block truncate text-xs text-muted-foreground">{node.sourceTitle}</span>}</button>
  const empty = <p className={styles.empty}>{zh ? '暂无记录' : 'No records'}</p>
  return <div className={`${styles.page} ${graphStyles.page}`}>
    <header className={styles.header}><div className="flex min-w-0 items-center gap-4"><Button variant="ghost" size="sm" asChild><Link href="/ai/workbench">← {zh ? '返回对话' : 'Conversations'}</Link></Button><div className="min-w-0"><h1 className="truncate text-lg font-semibold">{board?.project.name || (zh ? '项目探索' : 'Project exploration')}</h1><p className="mt-1 text-xs text-muted-foreground">{zh ? '项目探索 · 结论与证据' : 'Project exploration · Conclusions and evidence'}</p></div></div><div className="flex min-w-0 items-center gap-2">{tab !== 'plan' ? <Select value={source} onValueChange={value => jump(tab, null, value)}><SelectTrigger aria-label={zh ? '来源会话' : 'Source conversation'} className="w-[220px] max-w-[60vw]" title={selectedSource ? fullSourceLabel(selectedSource) : undefined}><SelectValue /></SelectTrigger><SelectContent style={{ maxWidth: 'min(360px, calc(100vw - 24px))' }}><SelectItem value="all">{zh ? '全部会话' : 'All conversations'}</SelectItem>{board?.sourceConversations?.map(s => { const full = fullSourceLabel(s); return <SelectItem key={s.id} value={s.id} title={full}><span className="block min-w-0 max-w-[300px] truncate">{clipSourceLabel(full)}</span></SelectItem> })}</SelectContent></Select> : null}<Button variant="outline" size="sm" disabled={loading} onClick={() => { setLoading(true); void load() }}>{zh ? '刷新' : 'Refresh'}</Button></div></header>
    <Tabs value={tab} onValueChange={value => jump(value, value === 'evidence' && !evidenceNodes.some(n => n.id === selected) ? null : selected)} className={styles.navigation}><TabsList className="grid h-auto w-full grid-cols-4 sm:flex sm:w-fit">{Object.entries(tabNames).map(([value, labels]) => <TabsTrigger key={value} value={value} className="px-2 text-xs sm:px-3 sm:text-sm">{labels[zh ? 0 : 1]}</TabsTrigger>)}</TabsList></Tabs>
    {error && <p role="alert" className="m-4 rounded border border-amber-500/30 p-3 text-sm">{error}</p>}
    {board && (board.exploration?.truncated || board.facts.length >= 1000 || board.findings.length >= 1000 || (board.sourceConversations?.length || 0) >= 1000) && <p className="px-6 py-2 text-xs text-amber-600">{zh ? '已达到接口返回上限，当前展示部分项目记录。' : 'API limit reached; showing partial project records.'}</p>}
    {loading && !board ? <p role="status" className="p-8 text-sm text-muted-foreground">{zh ? '加载项目记录…' : 'Loading project records…'}</p> : scoped && <>
      {tab === 'overview' && <div className={styles.content}><div className={styles.stats}>{[[zh ? '已确认漏洞' : 'Confirmed findings', confirmed.length], [zh ? '待验证发现' : 'Candidate findings', candidates.length], [zh ? '关键事实' : 'Recorded facts', facts.length], [zh ? '阻塞原因' : 'Blockers', blockers.length]].map(([title, count]) => <div key={String(title)} className={styles.stat}><span className="text-xs text-muted-foreground">{title}</span><strong>{count}</strong></div>)}</div><div className={styles.columns}><div><section className={styles.section}><h2>{zh ? '已确认漏洞' : 'Confirmed findings'}</h2>{confirmed.length ? confirmed.map(n => row(n)) : empty}</section><section className={styles.section}><h2>{zh ? '待验证发现' : 'Candidate findings'}</h2>{candidates.length ? candidates.map(n => row(n)) : empty}</section></div><div><section className={styles.section}><h2>{zh ? '关键事实' : 'Recorded facts'}</h2>{facts.length ? facts.slice(0, 6).map(n => row(n)) : empty}{facts.length > 6 && <Button variant="ghost" size="sm" onClick={() => jump('evidence', null)}>{zh ? '查看全部事实' : 'View all facts'}</Button>}</section><section className={styles.section}><h2>{zh ? '当前阻塞' : 'Current blockers'}</h2>{blockers.length ? blockers.map(n => row(n, 'graph')) : empty}</section></div></div></div>}
      {tab === 'graph' && <><ExplorationGraphView key={source} graph={graph} selected={selected} onSelect={node => jump('graph', node)} returnHref={href} onEvidence={node => jump('evidence', node)} /><p className="px-6 py-3 text-xs text-muted-foreground">{zh ? '跨会话连线使用已记录关系；运行中的方向以流动虚线标出，会话未运行时保留最近方向；计划为当前快照，完成不代表漏洞已证实。' : 'Cross-session edges use recorded links. The running direction flows; the latest direction stays when no conversation is running. Plans are current snapshots; completion does not prove a finding.'}</p></>}
      {tab === 'plan' && <PlanWorkspace projectId={id} />}
      {tab === 'evidence' && <EvidenceWorkspace key={source} nodes={evidenceNodes} selected={selected} onSelect={node => jump('evidence', node)} onLocate={node => jump('graph', node)} board={scoped} graph={graph} returnHref={href} />}
    </>}
  </div>
}

function EvidenceWorkspace({ nodes, selected, onSelect, onLocate, board, graph, returnHref }: { nodes: ExplorationNode[]; selected: string | null; onSelect: (id: string) => void; onLocate: (id: string) => void; board: Blackboard; graph: ExplorationGraph; returnHref: string }) {
  const { language } = useI18n(), zh = language === 'zh'
  const [query, setQuery] = useState(''), [page, setPage] = useState(1)
  const filtered = nodes.filter(n => `${n.title} ${n.body || ''}`.toLowerCase().includes(query.toLowerCase()))
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filtered.length / 20)))
  const active = selected ? nodes.find(n => n.id === selected) : filtered[0]
  const indexRef = useRef<HTMLElement>(null)
  const reportRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const index = indexRef.current
    const item = index?.querySelector('[data-selected="true"]')
    if (index && item) index.scrollTo({ top: index.scrollTop + item.getBoundingClientRect().top - index.getBoundingClientRect().top - 80 })
  }, [active?.id])
  useEffect(() => { reportRef.current?.scrollTo({ top: 0 }) }, [active?.id])
  const finding = board.findings.find(f => f.id === active?.findingId)
  const relations = graph.edges.filter(e => active && !e.provenance && (e.source === active.id || e.target === active.id))
  const source = board.sourceConversations?.find(s => s.id === active?.conversationId)
  const section = (title: string, content?: string) => <section><h3>{title}</h3>{content ? <ReportContent value={content} language={language} /> : <p className="text-sm text-muted-foreground">{zh ? '尚未记录' : 'Not recorded'}</p>}</section>
  return <div className={styles.evidence}><aside ref={indexRef} className={styles.index} aria-label={zh ? '证据目录' : 'Evidence index'}><Input aria-label={zh ? '搜索证据' : 'Search evidence'} placeholder={zh ? '搜索结论与证据…' : 'Search evidence…'} value={query} onChange={e => { setQuery(e.target.value); setPage(1) }} className="mb-4" />{filtered.slice((currentPage - 1) * 20, currentPage * 20).map(n => <button key={n.id} className={styles.row} data-selected={active?.id === n.id} onClick={() => onSelect(n.id)}><span className="mb-1 block text-xs text-muted-foreground">{kinds[n.kind][language]}</span><span className="line-clamp-3 text-sm leading-6">{n.title}</span></button>)}{!filtered.length && <p className={styles.empty}>{zh ? '没有匹配记录' : 'No matches'}</p>}{filtered.length > 20 && <PaginationPro current={currentPage} total={filtered.length} pageSize={20} onChange={setPage} showSizeChanger={false} />}</aside><div ref={reportRef} className={styles.report}>{active ? <article key={active.id}><div className="flex flex-wrap items-center gap-2"><Badge variant="outline">{kinds[active.kind][language]}</Badge>{active.status && <Badge variant="outline" className="bg-muted/40 text-muted-foreground">{stateNames[active.status]?.[zh ? 0 : 1] || active.status}</Badge>}<Button size="sm" variant="outline" onClick={() => onLocate(active.id)}>{zh ? '图中定位' : 'Locate in graph'}</Button>{finding?.id && <Button variant="ghost" size="sm" asChild><Link href={findingDetailHref(finding.id, returnHref)}>{zh ? '完整漏洞报告 ↗' : 'Full report ↗'}</Link></Button>}</div><h2>{active.title}</h2><div className="flex flex-wrap items-center gap-3 border-y py-3 text-xs text-muted-foreground"><span>{zh ? '来源：' : 'Source: '}{source?.title || active.sourceTitle || (zh ? '未记录来源会话' : 'No source conversation')}</span>{active.conversationId && <><Link className="underline underline-offset-4" href={`/ai/workbench?session=${encodeURIComponent(active.conversationId)}`}>{zh ? '打开会话' : 'Conversation'}</Link><Link className="underline underline-offset-4" href={explorationHref(active.conversationId)}>{zh ? '会话探索链路' : 'Session exploration'}</Link></>}</div>{finding ? <>{section(zh ? '影响与结论' : 'Impact and conclusion', finding.impact)}{section(zh ? '验证步骤' : 'Reproduction', finding.reproductionSteps)}{section(zh ? '原始证据' : 'Evidence', finding.evidence)}</> : section(zh ? '事实与证据' : 'Fact and evidence', active.body)}{active.executionId && active.conversationId && <section><h3>{zh ? '原始工具执行' : 'Original execution'}</h3><ExecutionEvidence conversationId={active.conversationId} executionId={active.executionId} language={language} /></section>}<section><h3>{zh ? '关联依据与结论' : 'Related evidence and conclusions'}</h3>{relations.length ? relations.map(e => { const other = graph.nodes.find(n => n.id === (e.source === active.id ? e.target : e.source)); return other ? <button className={styles.row} key={e.id} onClick={() => nodes.some(n => n.id === other.id) ? onSelect(other.id) : onLocate(other.id)}><span className="mb-1 block text-xs text-muted-foreground">{e.source === active.id ? '→ ' : '← '}{relationNames[e.relation]?.[zh ? 0 : 1] || e.relation}</span><span className="text-sm">{other.title}</span></button> : null }) : <p className="text-sm text-muted-foreground">{zh ? '尚未记录关联关系' : 'No relationships recorded'}</p>}</section></article> : <p className={styles.empty}>{selected ? (zh ? '该记录不在当前范围，请切换来源会话或重新选择。' : 'This record is outside the current scope.') : (zh ? '暂无可展示的证据' : 'No evidence recorded')}</p>}</div></div>
}
