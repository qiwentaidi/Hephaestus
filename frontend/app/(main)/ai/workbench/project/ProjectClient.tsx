'use client'

import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectTrigger, SelectContent, SelectItem, SelectValue } from '@/components/ui/select'
import {
  getAIWorkbenchPenTestProjectBlackboard,
  getAIWorkbenchProjectStats,
  listAIWorkbenchProjectConversationOptions,
  type AIWorkbenchConversationOption,
  type AIWorkbenchPenTestSessionBlackboard as Blackboard,
  type AIWorkbenchProjectStats,
} from '@/lib/api/ai-workbench'
import { buildExplorationGraph } from '@/lib/exploration-graph'
import { projectExplorationHref, scopeProjectBoard } from '@/lib/project-exploration'
import { useI18n } from '@/lib/i18n'
import { workbenchErrorMessage } from '@/lib/ai-workbench-i18n'
import { ExplorationGraphView } from '../exploration/ExplorationClient'
import { PlanWorkspace } from './components/PlanWorkspace'
import { EvidenceWorkspace } from './components/EvidenceWorkspace'
import { OverviewWorkspace } from './components/OverviewWorkspace'
import graphStyles from '../exploration/exploration.module.css'
import styles from './project.module.css'

const tabNames = { overview: ['概览', 'Overview'], graph: ['探索链路', 'Exploration'], plan: ['编排', 'Plan'], evidence: ['证据', 'Evidence'] }
const STATUS_LABELS: Record<string, [string, string]> = {
  active: ['进行中', 'Active'],
  planning: ['规划中', 'Planning'],
  completed: ['已完成', 'Completed'],
  archived: ['已归档', 'Archived'],
}
/** Labels are capped so one long session name cannot stretch the filter. */
const SOURCE_LABEL_MAX = 40
function clipSourceLabel(label: string): string {
  return label.length > SOURCE_LABEL_MAX ? `${label.slice(0, SOURCE_LABEL_MAX - 1).trimEnd()}…` : label
}
function activityLabel(value: string | undefined, zh: boolean): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getTime() <= 0) return ''
  const now = new Date()
  return date.toDateString() === now.toDateString()
    ? date.toLocaleTimeString(zh ? 'zh-CN' : 'en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
    : date.toLocaleDateString(zh ? 'zh-CN' : 'en-US', { month: 'numeric', day: 'numeric' })
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
  const rawFilter = params.get('filter') || ''
  const filter = ['finding', 'fact', 'blocker'].includes(rawFilter) ? rawFilter : ''
  const source = params.get('source') || 'all', selected = params.get('node')

  // The header and overview only need aggregates; the heavy blackboard snapshot
  // is fetched lazily, and only for the tabs that actually render the graph.
  const [stats, setStats] = useState<AIWorkbenchProjectStats | null>(null)
  const [board, setBoard] = useState<Blackboard | null>(null)
  const [sources, setSources] = useState<AIWorkbenchConversationOption[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  // Bumped whenever the project snapshot reloads; the evidence index follows it.
  const [revision, setRevision] = useState(0)
  const statsRequest = useRef(0), boardRequest = useRef(0), boardSignature = useRef('')

  const loadStats = useCallback((signal?: AbortSignal) => {
    const generation = ++statsRequest.current
    const pending = id ? getAIWorkbenchProjectStats(id, { source, signal }) : Promise.reject(new Error(zh ? '链接缺少项目 ID' : 'Missing project ID'))
    return pending.then(next => {
      if (signal?.aborted || generation !== statsRequest.current) return
      setStats(next)
      setError('')
    }).catch((e: unknown) => {
      if (!signal?.aborted && generation === statsRequest.current) setError(workbenchErrorMessage(e, t.aiWorkbench, t.aiWorkbench.loadWorkbenchFailed))
    }).finally(() => {
      if (!signal?.aborted && generation === statsRequest.current) setLoading(false)
    })
  }, [id, source, zh, t.aiWorkbench])

  const loadBoard = useCallback((signal?: AbortSignal) => {
    const generation = ++boardRequest.current
    const pending = id ? getAIWorkbenchPenTestProjectBlackboard(id) : Promise.reject(new Error(zh ? '链接缺少项目 ID' : 'Missing project ID'))
    return pending.then(next => {
      if (signal?.aborted || generation !== boardRequest.current) return
      const sig = JSON.stringify({ ...next, updatedAt: '' })
      if (sig !== boardSignature.current) {
        boardSignature.current = sig
        setBoard(next)
        setRevision(value => value + 1)
      }
      setError('')
    }).catch((e: unknown) => {
      if (!signal?.aborted && generation === boardRequest.current) setError(workbenchErrorMessage(e, t.aiWorkbench, t.aiWorkbench.loadWorkbenchFailed))
    })
  }, [id, zh, t.aiWorkbench])

  const needsBoard = tab === 'graph' || tab === 'evidence'

  useEffect(() => {
    const controller = new AbortController()
    void loadStats(controller.signal)
    const timer = setInterval(() => { if (!document.hidden) void loadStats(controller.signal) }, 20000)
    return () => { clearInterval(timer); controller.abort() }
  }, [loadStats])

  useEffect(() => {
    if (!needsBoard) return
    const controller = new AbortController()
    void loadBoard(controller.signal)
    const timer = setInterval(() => { if (!document.hidden) void loadBoard(controller.signal) }, 20000)
    return () => { clearInterval(timer); controller.abort() }
  }, [needsBoard, loadBoard])

  useEffect(() => {
    if (!id) return
    const controller = new AbortController()
    void listAIWorkbenchProjectConversationOptions(id, 1, 100).then(data => {
      if (!controller.signal.aborted) setSources(data.conversations || [])
    }).catch(() => {
      if (!controller.signal.aborted) setSources([])
    })
    return () => controller.abort()
  }, [id])

  const scoped = useMemo(() => board ? scopeProjectBoard(board, source) : null, [board, source])
  const graph = useMemo(() => scoped ? buildExplorationGraph(scoped) : { nodes: [], edges: [] }, [scoped])
  const jump = (nextTab: string, node: string | null = selected, nextSource = source, nextFilter = filter) => router.push(projectExplorationHref(id, nextTab, node || undefined, nextSource, nextFilter), { scroll: false })
  const href = projectExplorationHref(id, tab, selected || undefined, source, filter)
  const selectedSource = source === 'all' ? undefined : sources.find(item => item.id === source)
  const evidenceNodes = graph.nodes.filter(n => n.factKey || n.findingId)
  const evidenceTotal = stats ? stats.counts.facts + stats.counts.findings : undefined
  const project = stats?.project
  const scopeTargets = project?.scope || []
  const statusLabels = project?.status ? STATUS_LABELS[project.status.toLowerCase()] : undefined
  const lastActivity = stats?.updatedAt || project?.updatedAt
  const boardPending = needsBoard && !scoped
  return <div className={`${styles.page} ${graphStyles.page}`}>
    <header className={styles.header}>
      <div className="flex min-w-0 items-start gap-4">
        <Button variant="ghost" size="sm" asChild><Link href="/ai/workbench">← {zh ? '返回对话' : 'Conversations'}</Link></Button>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h1 className="truncate text-lg font-semibold">{project?.name || (zh ? '项目探索' : 'Project exploration')}</h1>
            {statusLabels ? <span className={styles.pill} data-live={project?.status === 'active'}><span className={styles.pillDot} />{statusLabels[zh ? 0 : 1]}</span> : null}
            {scopeTargets.length ? <span className={styles.pill}>{zh ? `${scopeTargets.length} 个资产` : `${scopeTargets.length} assets`}</span> : null}
          </div>
          <p className={styles.sub}>
            {zh ? '项目探索 · 结论与证据' : 'Project exploration · Conclusions and evidence'}
            {lastActivity ? ` · ${zh ? '最近活动' : 'last activity'} ${activityLabel(lastActivity, zh)}` : ''}
          </p>
          {scopeTargets.length ? (
            <div className={styles.scope} title={scopeTargets.join(', ')}>
              {scopeTargets.slice(0, 5).map(target => <span key={target} className={styles.tag}>{target}</span>)}
              {scopeTargets.length > 5 ? <span className={styles.tag}>+{scopeTargets.length - 5}</span> : null}
            </div>
          ) : null}
        </div>
      </div>
      <div className="flex min-w-0 shrink-0 items-center gap-2">
        {tab !== 'plan' ? <Select value={source} onValueChange={value => jump(tab, null, value)}><SelectTrigger aria-label={zh ? '来源会话' : 'Source conversation'} className="w-[220px] max-w-[60vw]" title={selectedSource?.name}><SelectValue /></SelectTrigger><SelectContent style={{ maxWidth: 'min(360px, calc(100vw - 24px))' }}><SelectItem value="all">{zh ? '全部会话' : 'All conversations'}</SelectItem>{sources.map(s => { const full = s.name || s.id; return <SelectItem key={s.id} value={s.id} title={full}><span className="block min-w-0 max-w-[300px] truncate">{clipSourceLabel(full)}</span></SelectItem> })}</SelectContent></Select> : null}
        <Button variant="outline" size="sm" disabled={loading} onClick={() => { setLoading(true); void loadStats(); if (needsBoard) void loadBoard() }}>{zh ? '刷新' : 'Refresh'}</Button>
      </div>
    </header>
    <Tabs value={tab} onValueChange={value => jump(value, value === 'evidence' && !evidenceNodes.some(n => n.id === selected) ? null : selected)} className={styles.navigation}><TabsList className="grid h-auto w-full grid-cols-4 sm:flex sm:w-fit">{Object.entries(tabNames).map(([value, labels]) => <TabsTrigger key={value} value={value} className="px-2 text-xs sm:px-3 sm:text-sm">{labels[zh ? 0 : 1]}{value === 'evidence' && evidenceTotal ? <span className={styles.tabCount}>{evidenceTotal}</span> : null}</TabsTrigger>)}</TabsList></Tabs>
    {error && <p role="alert" className="m-4 rounded border border-amber-500/30 p-3 text-sm">{error}</p>}
    {tab === 'overview' && loading && !stats ? <p role="status" className="p-8 text-sm text-muted-foreground">{zh ? '加载项目统计…' : 'Loading project stats…'}</p> : <>
      {tab === 'overview' && <OverviewWorkspace projectId={id} source={source} stats={stats} activity={stats?.updatedAt} onOpenEvidence={kind => jump('evidence', null, source, kind || '')} onOpenPlan={() => jump('plan')} />}
      {boardPending ? <p role="status" className="p-8 text-sm text-muted-foreground">{zh ? '加载项目记录…' : 'Loading project records…'}</p> : null}
      {tab === 'graph' && scoped && <><ExplorationGraphView key={source} graph={graph} selected={selected} onSelect={node => jump('graph', node)} returnHref={href} onEvidence={node => jump('evidence', node)} /><p className="px-6 py-3 text-xs text-muted-foreground">{zh ? '跨会话连线使用已记录关系；运行中的方向以流动虚线标出，会话未运行时保留最近方向；计划为当前快照，完成不代表漏洞已证实。' : 'Cross-session edges use recorded links. The running direction flows; the latest direction stays when no conversation is running. Plans are current snapshots; completion does not prove a finding.'}</p></>}
      {tab === 'plan' && <PlanWorkspace projectId={id} />}
      {tab === 'evidence' && scoped && <EvidenceWorkspace key={source} projectId={id} source={source} kind={filter} onKindChange={next => jump('evidence', null, source, next)} revision={String(revision)} selected={selected} onSelect={node => jump('evidence', node, source, filter)} onLocate={node => jump('graph', node)} board={scoped} graph={graph} returnHref={href} />}
    </>}
    {(tab === 'overview' || tab === 'graph') && board && (board.exploration?.truncated || (board.facts.length) >= 1000 || (board.findings.length) >= 1000 || (board.sourceConversations?.length || 0) >= 1000) && <p className="px-6 py-2 text-xs text-amber-600">{zh ? '已达到接口返回上限：图谱与概览仅展示最近 1000 条记录；完整记录请在「证据」中翻页查看。' : 'API limit reached: the graph and overview show the latest 1000 records; page through every record in Evidence.'}</p>}
  </div>
}
