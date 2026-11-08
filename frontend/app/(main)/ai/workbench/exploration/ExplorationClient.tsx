'use client'

import Link from 'next/link'
import { useSearchParams } from 'next/navigation'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ReactFlow, Background, BackgroundVariant, MiniMap, Handle, Position, MarkerType, useNodesState, useNodesInitialized, useReactFlow, useStore, type Node, type NodeProps } from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { ArrowLeft, ArrowUpRight, Bug, Compass, Flag, FlaskConical, Hand, Loader2, Maximize2, Minus, MousePointer2, Plus, RefreshCw, Search, ShieldAlert, Target, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { getAIWorkbenchPenTestSessionBlackboard, type AIWorkbenchPenTestSessionBlackboard as Blackboard } from '@/lib/api/ai-workbench'
import { buildExplorationGraph, layoutExplorationGraph, mainExplorationGraph, ancestorExplorationGraph, explorationDirection, type ExplorationNode, type ExplorationKind, type ExplorationGraph, explorationHref } from '@/lib/exploration-graph'
import { findingDetailHref } from '@/lib/finding-detail'
import { findingSeverityBadgeClassName, findingStatusBadgeClassName } from '@/lib/finding-badge'
import { getFindingLabels } from '@/lib/finding-i18n'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import { workbenchErrorMessage } from '@/lib/ai-workbench-i18n'
import { ExecutionEvidence, ExecutionEvidenceList } from '../components/ExecutionEvidence'
import { ReportContent } from '../../../vulnerabilities/detail/ReportContent'
import styles from './exploration.module.css'

export const kinds = {
  digest: { zh: '探索摘要', en: 'Exploration digest', icon: Compass, color: '#64748b' },
  start: { zh: '起点', en: 'Start', icon: Flag, color: '#64748b' },
  goal: { zh: '目标', en: 'Goal', icon: Target, color: '#10b981' },
  intent: { zh: '探索意图', en: 'Intent', icon: Compass, color: '#3b82f6' },
  fact: { zh: '事实', en: 'Fact', icon: FlaskConical, color: '#d97706' },
  finding: { zh: '漏洞', en: 'Finding', icon: Bug, color: 'var(--destructive)' },
  blocker: { zh: '阻塞', en: 'Blocked', icon: ShieldAlert, color: '#b28b32' },
  missing: { zh: '缺失引用', en: 'Missing reference', icon: ShieldAlert, color: '#94a3b8' },
} satisfies Record<ExplorationKind, { zh: string; en: string; icon: typeof Flag; color: string }>
export const relationNames: Record<string, [string, string]> = {
  spawns: ['派生', 'Spawns'], proves: ['目标证明', 'Proves'], goal: ['目标', 'Goal'], plan: ['计划包含', 'Plan item'], recorded: ['来源归属', 'Recorded in'], derived_from: ['意图链', 'Derived from'], yields: ['产出', 'Output'], finding: ['证据支撑', 'Supports'],
  depends_on: ['依赖', 'Depends on'], leads_to: ['引出', 'Leads to'], enables: ['使能', 'Enables'], exploits: ['利用', 'Exploits'], discovered_on: ['发现于', 'Discovered on'], contains: ['包含', 'Contains'], part_of: ['组成', 'Part of'], supports: ['证据支撑', 'Supports'],
}
export const stateNames: Record<string, [string, string]> = { confirmed: ['已确认', 'Confirmed'], tentative: ['待验证', 'Tentative'], deprecated: ['已废弃', 'Deprecated'], open: ['待确认', 'Open'], fixed: ['已修复', 'Fixed'], false_positive: ['误报', 'False positive'], accepted_risk: ['接受风险', 'Accepted risk'], superseded: ['已移出计划', 'Removed from plan'], pending: ['待执行', 'Pending'], in_progress: ['进行中', 'In progress'], running: ['进行中', 'Running'], draining: ['等待已创建任务收敛', 'Draining'], completed: ['已完成', 'Completed'], blocked: ['受阻', 'Blocked'], met: ['已达成', 'Met'], abandoned: ['已放弃', 'Abandoned'], exhausted: ['无可行升级', 'Exhausted'], stopped: ['已停止', 'Stopped'], paused: ['已暂停', 'Paused'], failed: ['失败', 'Failed'], cancelled: ['已取消', 'Cancelled'], cancelling: ['取消中', 'Cancelling'], not_executed: ['未执行', 'Not executed'] }
const relationColors: Record<string, string> = { derived_from: '#6366f1', yields: '#d97706', supports: '#0d9488', finding: '#0d9488', proves: '#10b981', spawns: '#10b981', recorded: '#94a3b8' }
const nodeStateName = (node: ExplorationNode, zh: boolean) => node.kind === 'goal' && node.status === 'open' ? (zh ? '未达成' : 'Open') : stateNames[node.status || '']?.[zh ? 0 : 1] || node.status
type ViewNode = Node<{ record: ExplorationNode; language: 'zh' | 'en'; showSource: boolean; direction?: 'live' | 'recent'; dimmed?: boolean }, 'exploration'>
function GraphNode({ data, selected }: NodeProps<ViewNode>) {
  const n = data.record, zh = data.language === 'zh', meta = kinds[n.kind], Icon = meta.icon
  const { t } = useI18n()
  const findingLabels = useMemo(() => getFindingLabels(t.vulnerabilityManagement), [t.vulnerabilityManagement])
  const mergedGoal = n.kind === 'start' && n.mergedGoal
  const showTarget = mergedGoal && !!n.target && n.target !== n.title
  // Findings are conclusions, so they carry their recorded risk on the card.
  // Facts and intents keep the neutral kind-only header.
  const severity = n.kind === 'finding' && n.severity ? findingLabels.severity[n.severity] || n.severityLabel || n.severity : ''
  const findingStatus = n.kind === 'finding' && n.status ? findingLabels.status[n.status] || stateNames[n.status]?.[zh ? 0 : 1] || n.status : ''
  return <div className={styles.node} data-kind={n.kind} data-severity={n.severity} data-selected={selected} data-dimmed={data.dimmed} data-missing={n.kind === 'missing'} data-unlinked={n.unlinked} data-direction={data.direction} data-status={n.kind === 'start' || n.kind === 'goal' || n.kind === 'intent' ? n.status : undefined}>
    <Handle type="target" position={Position.Left} className={styles.handle} />
    <div className={styles.nodeHeader}><span className={styles.nodeIcon} style={{ background: meta.color }}><Icon size={13} /></span><strong>{mergedGoal ? `${meta[data.language]} · ${zh ? '目标' : 'Goal'}` : meta[data.language]}</strong>{severity && n.severity && <span className={cn(styles.severity, findingSeverityBadgeClassName(n.severity))} title={`${zh ? '严重度' : 'Severity'}：${severity}`}>{severity}</span>}{n.priority !== undefined && <span className={styles.priority}>P{n.priority}</span>}{n.external && <span>{zh ? '跨会话' : 'Context'}</span>}{data.direction && <span className={styles.directionDot} data-live={data.direction === 'live'} role="img" aria-label={data.direction === 'live' ? (zh ? '当前探索方向' : 'Current direction') : (zh ? '最近探索方向' : 'Recent direction')} title={data.direction === 'live' ? (zh ? '当前探索方向' : 'Current direction') : (zh ? '最近探索方向（会话已结束）' : 'Recent direction (conversation ended)')} />}</div>
    <p className={styles.nodeTitle}>{n.title}</p>
    {showTarget && <p className={styles.nodeTarget} title={n.target}>{n.target}</p>}
    <div className={styles.nodeFooter}>{findingStatus ? <span className={styles.findingStatus} data-finding-status={n.status}>{findingStatus}</span> : n.status && (n.kind === 'start' || n.kind === 'goal' || n.kind === 'intent') ? <span>{nodeStateName(n, zh)}</span> : <span>{n.origin === 'reconstructed' ? (zh ? '历史整理' : 'Reconstructed') : n.executionId ? (zh ? '附原始证据' : 'Evidence attached') : n.id.replace(/^[^:]+:/, '').slice(0, 8)}</span>}{n.unlinked ? <span>{zh ? '未关联事实' : 'Unlinked fact'}</span> : n.missingIntent && <span>{zh ? '未记录产出意图' : 'Producer not recorded'}</span>}</div>
    {data.showSource && n.sourceTitle && <p className="mt-2 truncate text-[10px] text-muted-foreground" title={n.sourceTitle}>{n.sourceTitle}</p>}
    <Handle type="source" position={Position.Right} className={styles.handle} />
  </div>
}
const nodeTypes = { exploration: GraphNode }
const fitOptions = { padding: { top: '110px', bottom: '45px', left: '35px', right: '35px' }, maxZoom: 1 } as const
type CameraTarget = { id: string; nonce: number }
function FitVisible({ filterKey, ready, camera }: { filterKey: string; ready: boolean; camera?: CameraTarget | null }) {
  const initialized = useNodesInitialized()
  const { fitView } = useReactFlow()
  const width = useStore(state => state.width)
  const height = useStore(state => state.height)
  // The camera only moves when the filter/search/graph really changes, or on
  // the first non-empty render. Background refreshes, node selection and
  // pointer movement never re-fit, so the viewport stays where the user left it.
  const fitted = useRef('')
  useEffect(() => {
    if (!ready) { fitted.current = ''; return }
    if (!initialized || !width || !height) return
    if (fitted.current === filterKey) return
    fitted.current = filterKey
    void fitView({ ...fitOptions, duration: 200 })
  }, [ready, initialized, filterKey, fitView, width, height])
  // Explicit "locate this node" requests (recent direction, etc.) zoom in on the
  // node once it is actually part of the rendered graph.
  const cameraVisible = useStore(state => camera ? state.nodeLookup.has(camera.id) : false)
  useEffect(() => {
    if (!camera || !initialized || !cameraVisible) return
    void fitView({ ...fitOptions, nodes: [{ id: camera.id }], duration: 260, maxZoom: 1.25 })
  }, [camera, initialized, cameraVisible, fitView])
  return null
}


export default function ExplorationClient() {
  const id = useSearchParams().get('session') || ''
  return <ExplorationPage key={id} id={id} />
}

function ExplorationPage({ id }: { id: string }) {
  const { language, t } = useI18n(); const zh = language === 'zh'
  const [board, setBoard] = useState<Blackboard | null>(null)
  const [error, setError] = useState(''), [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<string | null>(null)
  const request = useRef(0), signature = useRef('')
  const load = useCallback((signal?: AbortSignal) => {
    const generation = ++request.current
    const fetchBoard = id ? getAIWorkbenchPenTestSessionBlackboard(id) : Promise.reject(new Error(zh ? '链接中缺少会话 ID。' : 'The URL has no session ID.'))
    return fetchBoard.then(next => {
      if (signal?.aborted || generation !== request.current) return
      const sig = JSON.stringify({ ...next, updatedAt: '' })
      if (sig !== signature.current) { signature.current = sig; setBoard(next) }
      setError('')
    }).catch((e: unknown) => {
      if (!signal?.aborted && generation === request.current) setError(workbenchErrorMessage(e, t.aiWorkbench, t.aiWorkbench.loadSessionFailed))
    }).finally(() => { if (!signal?.aborted && generation === request.current) setLoading(false) })
  }, [id, zh, t.aiWorkbench])
  // Keep the poll independent of render-time identities: the interval restarts
  // only when the session changes, so no re-render can trigger an extra fetch.
  const loadRef = useRef(load)
  useEffect(() => { loadRef.current = load }, [load])
  useEffect(() => {
    const controller = new AbortController()
    void loadRef.current(controller.signal)
    const timer = setInterval(() => { if (!document.hidden) void loadRef.current(controller.signal) }, 20000)
    return () => { clearInterval(timer); controller.abort() }
  }, [id])
  const graph = useMemo(() => board ? buildExplorationGraph(board) : { nodes: [], edges: [] }, [board])
  return <div className={styles.page}>
    <header className={styles.header}>
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <Button asChild variant="ghost" size="sm" className="shrink-0"><Link href={`/ai/workbench${id ? `?session=${encodeURIComponent(id)}` : ''}`} aria-label={zh ? '返回会话' : 'Conversation'}><ArrowLeft className="h-4 w-4 sm:mr-1" /><span className="hidden sm:inline">{zh ? '返回会话' : 'Conversation'}</span></Link></Button>
        <div className="flex min-w-0 flex-1 items-baseline gap-2">
          <h1 className="shrink-0 text-lg font-semibold">{zh ? '探索链路' : 'Exploration'}</h1>
          <span aria-hidden="true" className="shrink-0 text-muted-foreground/40">·</span>
          <p className="min-w-0 flex-1 truncate text-sm text-muted-foreground" title={board?.conversation?.title || board?.project.name || '—'}>{board?.conversation?.title || board?.project.name || '—'}</p>
        </div>
      </div>
      <div className="flex shrink-0 items-center gap-2">{board?.conversation?.status && <Badge variant="outline" className="bg-muted/40 text-muted-foreground">{stateNames[board.conversation.status]?.[zh ? 0 : 1] || board.conversation.status}</Badge>}<Button variant="ghost" size="icon" aria-label={zh ? '刷新链路' : 'Refresh graph'} onClick={() => { setLoading(true); void load() }} disabled={loading}><RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} /></Button></div>
    </header>
    {error && <p role="alert" className="mx-4 rounded border border-amber-500/30 bg-amber-500/5 p-3 text-sm">{error}</p>}
    <ExplorationGraphView graph={graph} loading={loading && !board} selected={selected} onSelect={setSelected} returnHref={explorationHref(id)} />
    <footer className={styles.footer}>{zh ? '沿线索形成问题，验证得到产出，再沿产出深入。会话运行中时，流动虚线标出当前探索方向；会话未运行时，最近一次探索方向保留为静态加粗虚线。来源归属使用灰色虚线，待验证关系使用琥珀色虚线；取消或未执行只体现在节点状态上，不改变关系本身。起点、目标和意图显示过程状态；漏洞人工处置在漏洞管理中进行。' : 'Follow evidence, outputs and proofs. While a conversation is running, animated dashes mark its current direction; when none is running, the latest explored direction stays as a static, thicker dashed path. Source ownership uses a gray dashed line and tentative relations use an amber dashed line; cancellation or non-execution is a node state, not a different relation. Starts, goals and intents carry process state; findings are dispositioned in vulnerability management.'}{board && (board.exploration?.truncated || board.facts.length >= 1000 || (board.contextFacts?.length || 0) >= 1000) && <strong> {zh ? '已达到接口返回上限，当前为部分记录。' : 'API limit reached; this is a partial graph.'}</strong>}</footer>
  </div>
}


type PanMode = 'pointer' | 'hand'
function GraphControls({ mode, onModeChange, zh }: { mode: PanMode; onModeChange: (mode: PanMode) => void; zh: boolean }) {
  const { zoomIn, zoomOut, fitView } = useReactFlow()
  const zoom = useStore(state => state.transform[2])
  const button = (active: boolean) => `${styles.controlButton}${active ? ' ' + styles.controlActive : ''}`
  return <div className={styles.controls} role="toolbar" aria-label={zh ? '画布控制' : 'Canvas controls'}>
    <button type="button" className={button(mode === 'pointer')} aria-pressed={mode === 'pointer'} title={zh ? '指针：可拖动节点' : 'Pointer: drag nodes'} onClick={() => onModeChange('pointer')}><MousePointer2 size={14} /></button>
    <button type="button" className={button(mode === 'hand')} aria-pressed={mode === 'hand'} title={zh ? '手：平移画布，节点锁定' : 'Hand: pan canvas, nodes locked'} onClick={() => onModeChange('hand')}><Hand size={14} /></button>
    <span className={styles.controlDivider} aria-hidden="true" />
    <button type="button" className={styles.controlButton} title={zh ? '缩小' : 'Zoom out'} aria-label={zh ? '缩小' : 'Zoom out'} onClick={() => void zoomOut({ duration: 150 })}><Minus size={14} /></button>
    <span className={styles.zoomValue} aria-live="polite">{Math.round(zoom * 100)}%</span>
    <button type="button" className={styles.controlButton} title={zh ? '放大' : 'Zoom in'} aria-label={zh ? '放大' : 'Zoom in'} onClick={() => void zoomIn({ duration: 150 })}><Plus size={14} /></button>
    <span className={styles.controlDivider} aria-hidden="true" />
    <button type="button" className={styles.controlButton} title={zh ? '适配视图' : 'Fit view'} aria-label={zh ? '适配视图' : 'Fit view'} onClick={() => void fitView({ ...fitOptions, duration: 200 })}><Maximize2 size={14} /></button>
  </div>
}
export function ExplorationGraphView({ graph, loading = false, selected, onSelect, returnHref, onEvidence, showAllRelations = false, showRelationToggle = true, hideDetails = false, showFactLegend = true, fitKey = '' }: {
  graph: ExplorationGraph; loading?: boolean; selected: string | null; onSelect: (id: string | null) => void; returnHref: string; onEvidence?: (id: string) => void; showAllRelations?: boolean; showRelationToggle?: boolean; hideDetails?: boolean; showFactLegend?: boolean; fitKey?: string
}) {
  const { language } = useI18n(); const zh = language === 'zh'
  const [query, setQuery] = useState(''), [kind, setKind] = useState('all')
  const [expandedDigests, setExpandedDigests] = useState<Set<string>>(new Set())
  const [fullRelations, setFullRelations] = useState(showAllRelations), [focus, setFocus] = useState<string | null>(null)
  // Chain views hand in an already filtered graph; there the relation toggle has
  // nothing to switch, so all recorded relations stay visible.
  const effectiveFull = fullRelations || !showRelationToggle
  const shown = useMemo(() => { const view = effectiveFull ? {nodes: graph.nodes.filter(n => n.kind !== 'digest'), edges: graph.edges} : mainExplorationGraph(graph, expandedDigests); return focus ? ancestorExplorationGraph(view, focus) : view }, [graph, effectiveFull, focus, expandedDigests])
  const direction = useMemo(() => explorationDirection(shown), [shown])
  const setSelected = onSelect
  const showSources = new Set(graph.nodes.map(n => n.conversationId).filter(Boolean)).size > 1
  const positions = useMemo(() => layoutExplorationGraph(shown), [shown])
  const visible = useMemo(() => {
    const matches = new Set(shown.nodes.filter(n => (kind === 'all' || n.kind === kind) && (!query.trim() || `${n.title} ${n.factKey || ''} ${n.body || ''}`.toLowerCase().includes(query.trim().toLowerCase()))).map(n => n.id))
    const included = new Set(matches)
    if (kind !== 'all' || query.trim()) for (const e of shown.edges) if (matches.has(e.source) || matches.has(e.target)) { included.add(e.source); included.add(e.target) }
    return { nodes: shown.nodes.filter(n => included.has(n.id)), edges: shown.edges.filter(e => included.has(e.source) && included.has(e.target)) }
  }, [shown, kind, query])
  const [nodes, setNodes, applyNodeChanges] = useNodesState<ViewNode>([])
  const [hovered, setHovered] = useState<string | null>(null)
  const [panMode, setPanMode] = useState<PanMode>('pointer')
  const [camera, setCamera] = useState<CameraTarget | null>(null)
  // Edges follow hover for quick inspection, but nodes only react to a real
  // selection: rebuilding every node on pointer move looked like a reload.
  const edgeFocus = hovered || selected
  const adjacency = useMemo(() => {
    const map = new Map<string, Set<string>>()
    const link = (a: string, b: string) => { const set = map.get(a); if (set) set.add(b); else map.set(a, new Set([b])) }
    for (const edge of visible.edges) { link(edge.source, edge.target); link(edge.target, edge.source) }
    return map
  }, [visible.edges])
  const focusNeighbors = useMemo(() => selected ? adjacency.get(selected) || null : null, [selected, adjacency])
  const dragged = useRef(new Set<string>())
  const onNodesChange = useCallback((changes: Parameters<typeof applyNodeChanges>[0]) => {
    for (const change of changes) if (change.type === 'position' && change.dragging) dragged.current.add(change.id)
    applyNodeChanges(changes)
  }, [applyNodeChanges])
  const filterKey = `${query}:${kind}:${fullRelations}:${focus || ''}:${[...expandedDigests].sort().join(',')}:${fitKey}`
  const previousFilter = useRef(filterKey)
  const nodeDirections = useMemo(() => {
    const marked = new Map<string, 'live' | 'recent'>()
    for (const n of visible.nodes) {
      if (n.kind !== 'intent' || (n.status !== 'in_progress' && n.status !== 'running')) continue
      if (n.live) marked.set(n.id, 'live')
      else if (!direction?.live && direction?.intent.id === n.id) marked.set(n.id, 'recent')
    }
    return marked
  }, [visible.nodes, direction])
  useEffect(() => {
    const relayout = previousFilter.current !== filterKey
    previousFilter.current = filterKey
    if (relayout) dragged.current.clear()
    // Synchronize an external graph snapshot, retaining dragged node positions on refresh.
    setNodes(previous => visible.nodes.map(n => {
      const existing = previous.find(p => p.id === n.id)?.position
      // Keep the on-screen position across refreshes; only a filter change or an
      // explicit relayout recomputes it.
      const position = (!relayout && existing) || (dragged.current.has(n.id) && existing) || positions.get(n.id)!
      return { id: n.id, type: 'exploration', data: { record: n, language, showSource: showSources, direction: nodeDirections.get(n.id), dimmed: !!focusNeighbors && n.id !== selected && !focusNeighbors.has(n.id) }, position, ariaLabel: `${kinds[n.kind][language]}: ${n.title}`, selected: n.id === selected }
    }))
  }, [visible, positions, filterKey, language, selected, direction, nodeDirections, setNodes, showSources, focusNeighbors])
  const flowing = useMemo(() => new Set(direction?.live ? direction.edges.map(e => e.id) : []), [direction])
  const recent = useMemo(() => new Set(direction && !direction.live ? direction.edges.map(e => e.id) : []), [direction])
  // Parallel relations between the same two nodes are drawn once; the label
  // lists every recorded relation so nothing is silently hidden.
  const edges = useMemo(() => {
    const groups = new Map<string, { source: string; target: string; ids: string[]; relations: string[]; provenance: boolean; tentative: boolean }>()
    for (const edge of visible.edges) {
      const key = `${edge.source}|${edge.target}`
      const group = groups.get(key)
      if (!group) { groups.set(key, { source: edge.source, target: edge.target, ids: [edge.id], relations: [edge.relation], provenance: !!edge.provenance, tentative: !!edge.tentative }); continue }
      group.ids.push(edge.id)
      if (!group.relations.includes(edge.relation)) group.relations.push(edge.relation)
      group.provenance = group.provenance && !!edge.provenance
      group.tentative = group.tentative || !!edge.tentative
    }
    const byID = new Map(visible.nodes.map(node => [node.id, node]))
    const order = ['proves', 'yields', 'spawns', 'derived_from', 'supports', 'depends_on', 'part_of', 'discovered_on', 'leads_to']
    return [...groups.values()].map(group => {
      const relation = [...group.relations].sort((a, b) => order.indexOf(a) - order.indexOf(b))[0]
      // A tentative fact makes every edge that carries it tentative, including
      // the intent -> output edge, which has no confidence of its own.
      const tentative = group.tentative || byID.get(group.source)?.status === 'tentative' || byID.get(group.target)?.status === 'tentative'
      const color = group.provenance ? '#94a3b8' : tentative ? '#b7791f' : relationColors[relation] || '#94a3b8'
      const connected = edgeFocus === group.source || edgeFocus === group.target
      const muted = !!selected && !connected
      const isFlowing = group.ids.some(id => flowing.has(id))
      const isRecent = !isFlowing && group.ids.some(id => recent.has(id))
      const label = connected ? group.relations.map(item => relationNames[item]?.[zh ? 0 : 1] || item).join(' / ') : undefined
      return {
        id: `edge:${group.source}:${group.target}`,
        source: group.source,
        target: group.target,
        label,
        type: 'default',
        className: isFlowing ? styles.flowing : undefined,
        markerEnd: { type: MarkerType.ArrowClosed, color },
        style: {
          stroke: color,
          strokeWidth: connected || isFlowing || isRecent ? 2.4 : 1.2,
          opacity: connected ? .95 : muted ? .07 : isFlowing ? .85 : isRecent ? .8 : .26,
          strokeDasharray: isFlowing ? '7 5' : isRecent ? '6 4' : group.provenance || tentative ? '5 4' : undefined,
        },
        labelStyle: { fill: 'var(--muted-foreground)', fontSize: 10 },
        labelBgStyle: { fill: 'var(--card)' },
      }
    })
  }, [visible, edgeFocus, selected, zh, flowing, recent])
  const active = graph.nodes.find(n => n.id === selected)
  return <div className={styles.graphView}>
    <div className={styles.toolbar}>
      <div className={styles.search}><Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" /><Input aria-label={zh ? '搜索节点' : 'Search nodes'} placeholder={zh ? '搜索目标、问题或产出…' : 'Search goals, questions or outputs…'} value={query} onChange={e => setQuery(e.target.value)} className="h-9 pl-9" /></div>
      <Select value={kind} onValueChange={setKind}>
        <SelectTrigger aria-label={zh ? '节点类型' : 'Node type'} className="w-[140px]">
          <SelectValue placeholder={zh ? '全部类型' : 'All types'} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{zh ? '全部类型' : 'All types'}</SelectItem>
          {Object.entries(kinds).map(([value, meta]) => <SelectItem key={value} value={value}>{meta[language]}</SelectItem>)}
        </SelectContent>
      </Select>
      {showRelationToggle && <div className="inline-flex rounded-md border border-border p-0.5"><Button variant="ghost" size="sm" aria-pressed={!fullRelations} className={!fullRelations ? 'bg-muted' : ''} onClick={() => setFullRelations(false)}>{zh ? '主线' : 'Main path'}</Button><Button variant="ghost" size="sm" aria-pressed={fullRelations} className={fullRelations ? 'bg-muted' : ''} onClick={() => setFullRelations(true)}>{zh ? '证据关系' : 'Evidence relations'}</Button></div>}
      {selected && !focus && <Button variant="outline" size="sm" onClick={() => { setFocus(selected); setQuery(''); setKind('all') }}>{zh ? '聚焦上游链路' : 'Focus upstream'}</Button>}
      {focus && <Button variant="outline" size="sm" onClick={() => setFocus(null)}>{zh ? '返回完整探索' : 'Show whole exploration'}</Button>}
      {direction && <Button variant="outline" size="sm" className="max-w-[min(340px,100%)] gap-2" onClick={() => { setFocus(null); setQuery(''); setKind('all'); setSelected(direction.intent.id); setCamera({ id: direction.intent.id, nonce: Date.now() }) }} title={direction.live ? (zh ? '定位当前探索方向' : 'Locate the current direction') : (zh ? '会话未在运行，保留最近一次探索方向；点击定位' : 'No conversation is running; the latest explored direction is kept. Click to locate.')}>
        <span className={styles.directionDot} data-live={direction.live} aria-hidden="true" />
        <span className="truncate">{direction.live ? (zh ? '当前方向' : 'Current') : (zh ? '最近方向' : 'Recent')}：{direction.intent.title}</span>
      </Button>}
      <span className="text-xs text-muted-foreground">{visible.nodes.length} {zh ? '节点' : 'nodes'} · {edges.length} {zh ? '连线' : 'links'}{visible.edges.length > edges.length ? ` · ${visible.edges.length} ${zh ? '关系' : 'relations'}` : ''}{!effectiveFull && graph.edges.length > shown.edges.length && !focus ? ` · ${zh ? '完整记录' : 'Recorded'} ${graph.edges.length}` : ''}</span>
    </div>
    <div className={styles.workspace}>
      <div className={styles.canvas} data-pan={panMode}>
        {loading ? <div role="status" className={styles.empty}><Loader2 className="h-5 w-5 animate-spin" />{zh ? '加载探索记录…' : 'Loading exploration…'}</div> : <ReactFlow<ViewNode> nodes={nodes} edges={edges} nodeTypes={nodeTypes} onNodesChange={onNodesChange} onNodeClick={(_, n) => setSelected(n.id)} onNodeMouseEnter={(_, n) => setHovered(n.id)} onNodeMouseLeave={() => setHovered(null)} onPaneClick={() => { setSelected(null); setHovered(null) }} nodesConnectable={false} nodesDraggable={panMode === 'pointer'} edgesReconnectable={false} deleteKeyCode={null} minZoom={.1} maxZoom={1.6} fitView fitViewOptions={fitOptions} proOptions={{ hideAttribution: true }} ariaLabelConfig={zh ? { 'controls.zoomIn.ariaLabel': '放大', 'controls.zoomOut.ariaLabel': '缩小', 'controls.fitView.ariaLabel': '适配视图', 'minimap.ariaLabel': '缩略导航图' } : undefined}>
          <FitVisible filterKey={filterKey} ready={visible.nodes.length > 0} camera={camera} /><Background variant={BackgroundVariant.Dots} gap={18} size={1} color="var(--border)" /><GraphControls mode={panMode} onModeChange={setPanMode} zh={zh} /><MiniMap pannable zoomable nodeColor={n => kinds[(n.data as ViewNode['data']).record.kind].color} maskColor="color-mix(in srgb, var(--background) 65%, transparent)" />
        </ReactFlow>}
        <div className={styles.legend}><div className="flex flex-wrap gap-x-3 gap-y-2">{(['start', 'goal', ...(showFactLegend ? ['fact' as const] : []), 'intent', 'finding', 'digest'] as const).map(k => { const Icon = kinds[k].icon; return <span key={k} className="inline-flex items-center gap-1.5"><Icon size={13} style={{ color: kinds[k].color }} />{kinds[k][language]}</span> })}</div><div className="mt-2 flex flex-wrap gap-4 border-t border-border/60 pt-2 text-muted-foreground"><span className={styles.flowLegend}><svg className={styles.flowLegendLine} viewBox="0 0 18 4" aria-hidden="true"><line x1="0" y1="2" x2="18" y2="2" /></svg>{zh ? '当前探索方向' : 'Current direction'}</span>{(['spawns', 'derived_from', 'yields', 'proves'] as const).map(relation => <span key={relation}><span style={{ color: relationColors[relation] }}>── </span>{relationNames[relation][zh ? 0 : 1]}</span>)}<span><span style={{ color: '#94a3b8' }}>┄┄ </span>{zh ? '来源归属' : 'Source'}</span><span><span style={{ color: '#b7791f' }}>┄┄ </span>{zh ? '待验证' : 'Tentative'}</span></div></div>
        {!loading && visible.nodes.length === 0 && <p className={styles.empty}>{zh ? '没有匹配的节点' : 'No matching nodes'}</p>}
      </div>
      {active && !hideDetails && <NodeDetail key={active.id} node={active} direction={nodeDirections.get(active.id)} language={language} returnHref={returnHref} onExpand={active.kind === 'digest' ? () => {setExpandedDigests(previous => new Set([...previous, active.id])); setFocus(null); setKind('all'); setQuery(''); setSelected(null)} : undefined} onEvidence={onEvidence ? () => onEvidence(active.id) : undefined} onClose={() => setSelected(null)} />}
    </div>
  </div>
}

function NodeDetail({ node, direction, language, returnHref, onClose, onEvidence, onExpand }: { node: ExplorationNode; direction?: 'live' | 'recent'; language: 'zh' | 'en'; returnHref: string; onClose: () => void; onEvidence?: () => void; onExpand?: () => void }) {
  const zh = language === 'zh'
  const { t } = useI18n()
  const findingLabels = useMemo(() => getFindingLabels(t.vulnerabilityManagement), [t.vulnerabilityManagement])
  const finding = node.kind === 'finding'
  return <aside className={styles.details} aria-label={zh ? '节点详情' : 'Node details'}>
    <div className="mb-5 flex items-center justify-between"><span className="text-sm font-semibold">{kinds[node.kind][language]}</span><Button variant="ghost" size="icon" aria-label={zh ? '关闭节点详情' : 'Close node details'} onClick={onClose}><X className="h-4 w-4" /></Button></div>
    <h2 className="mb-3 break-words text-base font-semibold leading-7">{node.title}</h2>
    {node.kind === 'start' && node.mergedGoal && node.target && <p className="mb-3 break-all text-xs text-muted-foreground">{zh ? '会话目标：' : 'Session goal: '}{node.target}</p>}
    <div className="mb-4 flex flex-wrap items-center gap-2">
      {finding && node.severity && <Badge variant="outline" className={findingSeverityBadgeClassName(node.severity)}>{findingLabels.severity[node.severity] || node.severityLabel || node.severity}</Badge>}
      {finding && node.status && <Badge variant="outline" className={findingStatusBadgeClassName(node.status)}>{findingLabels.status[node.status] || stateNames[node.status]?.[zh ? 0 : 1] || node.status}</Badge>}
      {!finding && node.status && (node.kind === 'start' || node.kind === 'goal' || node.kind === 'intent') && <Badge variant="outline" className="bg-muted/40 text-muted-foreground">{nodeStateName(node, zh)}</Badge>}
      {direction && <Badge variant="outline" className={direction === 'live' ? 'border-blue-500/30 bg-blue-500/10 text-blue-600 dark:text-blue-300' : 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'}>{direction === 'live' ? (zh ? '当前探索方向' : 'Current direction') : (zh ? '最近探索方向' : 'Recent direction')}</Badge>}
    </div>
    {finding && node.url && <div className="mb-4"><p className="mb-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">{zh ? '目标' : 'Target'}</p><p className="break-all font-mono text-xs text-foreground/80">{node.url}</p></div>}
    {node.origin === 'reconstructed' && <p className="mb-3 text-xs text-muted-foreground">{zh ? '历史整理：问题依据原始成功记录重建，未还原未记录的模型决策或验证顺序。' : 'Reconstructed from successful recording calls; unrecorded model decisions and execution order are not inferred.'}</p>}
    {node.priority !== undefined && <p className="mb-3 text-xs text-muted-foreground">{zh ? '探索优先级' : 'Priority'}：P{node.priority} / 10</p>}
    {node.unlinked && <p className="mb-3 text-xs text-muted-foreground">{zh ? '尚未记录此节点的探索来源关系。' : 'No exploration relationship was recorded for this node.'}</p>}
    {node.missingIntent && <p className="mb-3 text-xs text-muted-foreground">{zh ? '未记录产生此结果的探索意图；已有支撑证据仍然保留。历史记录不会按标题或执行时间自动补连。' : 'The producing intent was not recorded; supporting evidence remains. Historical lineage is not inferred from titles or timing.'}</p>}
    {node.kind === 'missing' && <p className="mb-3 text-sm text-muted-foreground">{zh ? '关联事实未包含在当前数据中，无法展示其内容。' : 'The referenced fact is absent from the current data.'}</p>}
    {node.factKey && <p className="mb-4 break-all font-mono text-xs text-muted-foreground">{node.factKey}</p>}
    {node.kind === 'digest' && <div className="mb-4"><p className="mb-3 text-xs text-muted-foreground">{zh ? `收起 ${node.members?.length || 0} 项已冷却的问题与事实；原始记录完整保留，漏洞与目标始终可见。` : `${node.members?.length || 0} settled questions and facts; findings and goals remain visible.`}</p><Button variant="outline" size="sm" onClick={onExpand}>{zh ? '展开原始分支' : 'Expand original branch'}</Button></div>}
    {node.body && <ReportContent value={node.body} language={language} />}
    {(node.conversationId || node.findingId) && <div className="my-4 flex flex-wrap items-center gap-2">
      {node.conversationId && <Button asChild variant="outline" size="sm" className="my-0"><Link href={`/ai/workbench?session=${encodeURIComponent(node.conversationId)}`}>{zh ? '来源会话' : 'Source conversation'}<ArrowUpRight className="ml-1 h-3.5 w-3.5" /></Link></Button>}
      {node.findingId && <Button asChild variant="outline" size="sm" className="my-0"><Link href={findingDetailHref(node.findingId, returnHref)}>{zh ? '查看漏洞完整报告' : 'Open finding report'}<ArrowUpRight className="ml-1 h-3.5 w-3.5" /></Link></Button>}
    </div>}
    {onEvidence && (node.factKey || node.findingId) && <Button variant="outline" size="sm" className="my-3" onClick={onEvidence}>{zh ? '查看证据' : 'View evidence'}</Button>}
    {!!node.executions?.length && node.conversationId && <ExecutionEvidenceList key={node.conversationId} conversationId={node.conversationId} executions={node.executions} language={language} />}
    {node.executionId && node.conversationId && <ExecutionEvidence conversationId={node.conversationId} executionId={node.executionId} language={language} />}

  </aside>
}
