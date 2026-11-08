'use client'

import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type PointerEvent, type WheelEvent } from 'react'
import { useRouter } from 'next/navigation'
import {
  CircleDot,
  Crown,
  Download,
  ExternalLink,
  Globe2,
  Link2,
  Maximize2,
  Minimize2,
  Monitor,
  Search,
  Server,
  X,
  ZoomIn,
  ZoomOut,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Spinner } from '@/components/ui/spinner'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { fetchWithAuth } from '@/lib/api'
import { getDesktopWindowState, toggleDesktopWindowFullscreen } from '@/lib/desktop'
import { saveBlobAsFile } from '@/lib/download'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from '@/lib/toast'

type EntryAssetType = 'root' | 'domain' | 'ip'
type AssetType = EntryAssetType | 'endpoint' | 'technology'
type GraphFullscreenMode = 'browser' | 'desktop' | null
interface AssetCenterItem {
  type: AssetType
  value: string
  ips: string[]
  operator?: string
  location?: string
  asn?: string
  endpointHost?: string
  port?: number
  protocol?: string
  service?: string
  serviceLabels?: string[]
  taskCount: number
  sourceTaskIds: string[]
  firstSeenAt?: string
  updatedAt?: string
}

interface TopologyNode {
  id: string
  type: AssetType
  label: string
  asset?: AssetCenterItem
  service?: string
}

interface TopologyEdge {
  source: string
  target: string
  kind: 'contains' | 'resolves' | 'serves' | 'identifies'
}

interface AssetTopology {
  root: AssetCenterItem
  nodes: TopologyNode[]
  edges: TopologyEdge[]
}

interface TopologyColumnData {
  type: AssetType
  nodes: TopologyNode[]
}

interface TechnologyGroup {
  id: string
  nodes: TopologyNode[]
}

interface TopologyRoute {
  id: string
  path: string
  active: boolean
}

interface ChildDisplayControl {
  total: number
  visible: number
  targetType: AssetType
  parentLabel: string
}

interface AssetsResponse {
  success: boolean
  data?: { items: AssetCenterItem[]; total: number }
  error?: string
}

interface TopologyResponse {
  success: boolean
  data?: AssetTopology
  error?: string
}

async function readAssetResponse<T extends { success: boolean; error?: string }>(response: Response): Promise<T> {
  const body = await response.text()
  try {
    return JSON.parse(body) as T
  } catch {
    return {
      success: false,
      error: response.ok ? undefined : `HTTP ${response.status}`,
    } as T
  }
}

function TopologyTypeIcon({ type, className }: { type: AssetType; className?: string }) {
  switch (type) {
    case 'root':
      return <Crown className={className} />
    case 'domain':
      return <Globe2 className={className} />
    case 'ip':
      return <Monitor className={className} />
    case 'endpoint':
      return <Server className={className} />
    default:
      return <CircleDot className={className} />
  }
}

const topologyLabel = (type: AssetType, isEnglish: boolean) => {
  const labels = isEnglish
    ? { root: 'Root domain', domain: 'Subdomain', ip: 'IP', endpoint: 'Port service', technology: 'Technology' }
    : { root: '根域名', domain: '子域名', ip: 'IP', endpoint: '端口服务', technology: '技术识别' }
  return labels[type]
}

const topologyTypeOrder: AssetType[] = ['root', 'domain', 'ip', 'endpoint', 'technology']

function formatAssetTime(value: string | undefined, isEnglish: boolean) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleDateString(isEnglish ? 'en-US' : 'zh-CN', isEnglish
    ? { year: 'numeric', month: 'short', day: 'numeric' }
    : { year: 'numeric', month: 'numeric', day: 'numeric' })
}

function getEndpointDisplay(node: TopologyNode) {
  const [labelPort, labelProtocol] = node.label.split(/\s*\/\s*/, 2)
  return {
    port: node.asset?.port ? String(node.asset.port) : labelPort,
    protocol: (node.asset?.protocol || labelProtocol || '').toUpperCase(),
  }
}

function getEndpointPortService(node: TopologyNode) {
  return node.asset?.service?.trim() || ''
}

function getEndpointVendor(node: TopologyNode) {
  const service = getEndpointPortService(node)
  if (!service) return ''
  return service.replace(/\s+v?\d+(?:\.\d+)+(?:[-+][\w.-]+)?\s*$/i, '').trim() || service
}

function TopologyNodeCard({
  node,
  active,
  onClick,
}: {
  node: TopologyNode
  active: boolean
  onClick: () => void
}) {
  if (node.type === 'endpoint') {
    const endpoint = getEndpointDisplay(node)
    const portService = getEndpointPortService(node)
    return (
      <button
        type="button"
        onClick={onClick}
        className={cn(
          'group relative flex w-full items-center gap-2 rounded-lg border bg-card/90 px-3 py-2.5 text-left shadow-sm transition-colors hover:border-primary/60 hover:bg-muted/65',
          active ? 'border-primary bg-muted/75 ring-1 ring-primary/20' : 'border-border',
        )}
      >
        <span className={cn(
          'flex size-10 shrink-0 items-center justify-center rounded-full border font-mono text-[10px] font-semibold',
          active ? 'border-primary/60 text-primary' : 'border-border text-foreground',
        )}>
          {endpoint.port || '-'}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium text-foreground">{endpoint.protocol || node.label}</span>
          {portService && <span className="mt-0.5 block truncate text-xs text-muted-foreground">{portService}</span>}
        </span>
      </button>
    )
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'group relative flex w-full items-center gap-2 rounded-lg border bg-card/90 px-3 py-2.5 text-left shadow-sm transition-colors hover:border-primary/60 hover:bg-muted/65',
        active ? 'border-primary bg-muted/75 ring-1 ring-primary/20' : 'border-border',
      )}
    >
      <span className={cn(
        'flex size-8 shrink-0 items-center justify-center rounded-full border',
        active ? 'border-primary/60 bg-muted/75 text-primary' : 'border-border bg-muted/40 text-muted-foreground',
      )}>
        <TopologyTypeIcon type={node.type} className="size-4" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium text-foreground">{node.label}</span>
        <span className="mt-0.5 block truncate text-xs text-muted-foreground">{topologyLabel(node.type, false)}</span>
      </span>
    </button>
  )
}

export function AssetTopologyView() {
  const router = useRouter()
  const { language } = useI18n()
  const isEnglish = language === 'en'
  const copy = isEnglish
    ? {
        title: 'Asset Topology', observedAssets: 'Observed assets', all: 'All', roots: 'Root domains', ip: 'IP assets', domains: 'Subdomains',
        search: 'Search assets', export: 'Export', type: 'Type', updated: 'Last updated',
        relatedAssets: 'Related assets', assetType: 'Asset type', relatedIPs: 'Related IPs',
        sourceTasks: 'Source tasks', firstSeen: 'First seen', lastUpdated: 'Last updated', portServices: 'Ports & services', noAssets: 'No domain or IP assets observed yet',
        noTopology: 'No linked assets were observed for this entry.', contains: 'Contains', resolves: 'Resolves to', serves: 'Provides service', fullscreen: 'Enter fullscreen', exitFullscreen: 'Exit fullscreen', zoomOut: 'Zoom out', zoomIn: 'Zoom in', overview: 'Overview',
        sourceTask: 'Source task', openTask: 'Open task detail', asset: 'Asset', technologies: 'Technologies', loadMore: 'Load 10 more', showAll: 'Show all', closeDetails: 'Close details',
      }
    : {
        title: '资产拓扑', observedAssets: '观测资产', all: '全部', roots: '根域名', ip: 'IP 资产', domains: '子域名',
        search: '搜索资产', export: '导出', type: '类型', updated: '最近更新',
        relatedAssets: '已关联资产', assetType: '资产类型', relatedIPs: '关联 IP',
        sourceTasks: '来源任务', firstSeen: '首次发现', lastUpdated: '最近更新', portServices: '端口与服务', noAssets: '暂无可用于拓扑的域名或 IP 资产',
        noTopology: '该入口暂无已观测到的关联资产。', contains: '包含', resolves: '解析到', serves: '提供服务', fullscreen: '进入全屏', exitFullscreen: '退出全屏', zoomOut: '缩小', zoomIn: '放大', overview: '概览',
        sourceTask: '来源任务', openTask: '查看任务详情', asset: '资产', technologies: '技术识别', loadMore: '加载 10 条', showAll: '全部展示', closeDetails: '关闭详情',
      }

  const [assets, setAssets] = useState<AssetCenterItem[]>([])
  const [keyword, setKeyword] = useState('')
  const [entryPage, setEntryPage] = useState(1)
  const [entryTotal, setEntryTotal] = useState(0)
  const entryPageSize = 50
  const [selectedEntry, setSelectedEntry] = useState<{ type: EntryAssetType; value: string } | null>(null)
  const [topology, setTopology] = useState<AssetTopology | null>(null)
  const [inspectedNodeId, setInspectedNodeId] = useState('')
  const [isDetailPanelOpen, setIsDetailPanelOpen] = useState(true)
  const [loadingAssets, setLoadingAssets] = useState(true)
  const [loadingTopology, setLoadingTopology] = useState(false)
  const [error, setError] = useState('')
  const [zoom, setZoom] = useState(100)
  const [topologyTypeVisibility, setTopologyTypeVisibility] = useState<Record<AssetType, boolean>>({
    root: true,
    domain: true,
    ip: true,
    endpoint: true,
    technology: true,
  })
  const [childDisplayLimits, setChildDisplayLimits] = useState<Record<string, number>>({})
  const graphViewportRef = useRef<HTMLDivElement | null>(null)
  const graphContentRef = useRef<HTMLDivElement | null>(null)
  const graphDragRef = useRef<{ pointerId: number; startX: number; startY: number; scrollLeft: number; scrollTop: number } | null>(null)
  const [graphCanvasSize, setGraphCanvasSize] = useState({ width: 0, height: 0 })
  const [isGraphDragging, setIsGraphDragging] = useState(false)
  const graphPanelRef = useRef<HTMLElement | null>(null)
  const assetSearchRef = useRef<HTMLInputElement | null>(null)
  const topologyNodeRefs = useRef(new Map<string, HTMLDivElement>())
  const [topologyRoutes, setTopologyRoutes] = useState<TopologyRoute[]>([])
  const [graphFullscreenMode, setGraphFullscreenMode] = useState<GraphFullscreenMode>(null)
  const isGraphFullscreen = graphFullscreenMode !== null

  const loadAssets = useCallback(async () => {
    setLoadingAssets(true)
    setError('')
    try {
      const params = new URLSearchParams({
        type: 'all',
        page: String(entryPage),
        pageSize: String(entryPageSize),
      })
      if (keyword.trim()) params.set('keyword', keyword.trim())
      const response = await fetchWithAuth(`/api/v1/assets/topology/entries?${params.toString()}`)
      const payload = await readAssetResponse<AssetsResponse>(response)
      if (!response.ok || !payload.success || !payload.data) {
        throw new Error(payload.error || (isEnglish ? 'Failed to load assets' : '加载资产失败'))
      }
      const entries = payload.data.items.filter((item): item is AssetCenterItem & { type: EntryAssetType } => item.type === 'root' || item.type === 'domain' || item.type === 'ip')
      setAssets(entries)
      setEntryTotal(payload.data.total || 0)
      setSelectedEntry((current) => {
        if (current) return current
        const first = entries[0]
        return first ? { type: first.type, value: first.value } : null
      })
    } catch (requestError) {
      setAssets([])
      setEntryTotal(0)
      setSelectedEntry(null)
      setTopology(null)
      setError(requestError instanceof Error ? requestError.message : (isEnglish ? 'Failed to load assets' : '加载资产失败'))
    } finally {
      setLoadingAssets(false)
    }
  }, [entryPage, isEnglish, keyword])

  const loadTopology = useCallback(async () => {
    if (!selectedEntry) return
    setLoadingTopology(true)
    setError('')
    try {
      const query = new URLSearchParams(selectedEntry)
      const response = await fetchWithAuth(`/api/v1/assets/topology?${query.toString()}`)
      const payload = await readAssetResponse<TopologyResponse>(response)
      if (!response.ok || !payload.success || !payload.data) {
        throw new Error(payload.error || (isEnglish ? 'Failed to load topology' : '加载资产拓扑失败'))
      }
      setTopology(payload.data)
      const rootNode = payload.data.nodes.find((node) => node.asset?.type === selectedEntry.type && node.asset.value === selectedEntry.value)
      setInspectedNodeId(rootNode?.id || '')
      setIsDetailPanelOpen(true)
      setZoom(100)
    } catch (requestError) {
      setTopology(null)
      setError(requestError instanceof Error ? requestError.message : (isEnglish ? 'Failed to load topology' : '加载资产拓扑失败'))
    } finally {
      setLoadingTopology(false)
    }
  }, [isEnglish, selectedEntry])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadAssets()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadAssets])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadTopology()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadTopology])

  useEffect(() => {
    const handleSearchShortcut = (event: KeyboardEvent) => {
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target?.isContentEditable) return
      event.preventDefault()
      assetSearchRef.current?.focus()
    }
    window.addEventListener('keydown', handleSearchShortcut)
    return () => window.removeEventListener('keydown', handleSearchShortcut)
  }, [])

  useEffect(() => {
    const handleFullscreenChange = () => {
      if (document.fullscreenElement === graphPanelRef.current) {
        setGraphFullscreenMode('browser')
        return
      }
      setGraphFullscreenMode((current) => current === 'browser' ? null : current)
    }
    document.addEventListener('fullscreenchange', handleFullscreenChange)
    return () => document.removeEventListener('fullscreenchange', handleFullscreenChange)
  }, [])

  const toggleGraphFullscreen = useCallback(async () => {
    try {
      if (graphFullscreenMode === 'browser') {
        if (document.fullscreenElement) {
          await document.exitFullscreen()
        } else {
          setGraphFullscreenMode(null)
        }
        return
      }

      if (graphFullscreenMode === 'desktop') {
        const state = await toggleDesktopWindowFullscreen()
        if (state) {
          setGraphFullscreenMode(null)
          return
        }
        toast.error(isEnglish ? 'Unable to exit fullscreen' : '无法退出全屏')
        return
      }

      const desktopState = await toggleDesktopWindowFullscreen()
      if (desktopState) {
        setGraphFullscreenMode('desktop')
        return
      }

      await graphPanelRef.current?.requestFullscreen()
    } catch {
      setGraphFullscreenMode(null)
      toast.error(isEnglish ? 'Unable to enter fullscreen' : '无法进入全屏')
    }
  }, [graphFullscreenMode, isEnglish])

  useEffect(() => {
    if (graphFullscreenMode !== 'desktop') return

    let syncTimeout: ReturnType<typeof setTimeout> | null = null
    const syncDesktopFullscreenState = () => {
      if (syncTimeout) {
        clearTimeout(syncTimeout)
      }
      syncTimeout = setTimeout(() => {
        void getDesktopWindowState().then((state) => {
          if (state && !state.isFullscreen) {
            setGraphFullscreenMode((current) => current === 'desktop' ? null : current)
          }
        })
      }, 600)
    }
    const handleDesktopFullscreenKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        void toggleGraphFullscreen()
      }
    }

    window.addEventListener('resize', syncDesktopFullscreenState)
    window.addEventListener('focus', syncDesktopFullscreenState)
    window.addEventListener('keydown', handleDesktopFullscreenKey)
    document.addEventListener('visibilitychange', syncDesktopFullscreenState)

    return () => {
      if (syncTimeout) {
        clearTimeout(syncTimeout)
      }
      window.removeEventListener('resize', syncDesktopFullscreenState)
      window.removeEventListener('focus', syncDesktopFullscreenState)
      window.removeEventListener('keydown', handleDesktopFullscreenKey)
      document.removeEventListener('visibilitychange', syncDesktopFullscreenState)
    }
  }, [graphFullscreenMode, toggleGraphFullscreen])

  const handleGraphWheel = useCallback((event: WheelEvent<HTMLDivElement>) => {
    if (!isGraphFullscreen || (!event.ctrlKey && !event.metaKey)) return
    event.preventDefault()
    if (event.deltaY === 0) return
    setZoom((value) => Math.min(140, Math.max(70, value + (event.deltaY < 0 ? 10 : -10))))
  }, [isGraphFullscreen])

  const stopGraphDrag = useCallback((event?: PointerEvent<HTMLDivElement>) => {
    const viewport = graphViewportRef.current
    const pointerId = graphDragRef.current?.pointerId
    const wasDragging = graphDragRef.current !== null
    if (viewport && pointerId !== undefined && viewport.hasPointerCapture(pointerId)) {
      viewport.releasePointerCapture(pointerId)
    }
    graphDragRef.current = null
    setIsGraphDragging(false)
    if (wasDragging) event?.preventDefault()
  }, [])

  const handleGraphPointerDown = useCallback((event: PointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return
    const target = event.target as HTMLElement | null
    if (target?.closest('button,a,input,textarea,select,[role="button"]')) return

    const viewport = graphViewportRef.current
    if (!viewport) return
    const canPan = viewport.scrollWidth > viewport.clientWidth || viewport.scrollHeight > viewport.clientHeight
    if (!canPan) return

    graphDragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      scrollLeft: viewport.scrollLeft,
      scrollTop: viewport.scrollTop,
    }
    viewport.setPointerCapture(event.pointerId)
    setIsGraphDragging(true)
    event.preventDefault()
  }, [])

  const handleGraphPointerMove = useCallback((event: PointerEvent<HTMLDivElement>) => {
    const dragState = graphDragRef.current
    const viewport = graphViewportRef.current
    if (!dragState || !viewport || dragState.pointerId !== event.pointerId) return

    viewport.scrollLeft = dragState.scrollLeft - (event.clientX - dragState.startX)
    viewport.scrollTop = dragState.scrollTop - (event.clientY - dragState.startY)
    event.preventDefault()
  }, [])

  const nodeMap = useMemo(() => new Map(topology?.nodes.map((node) => [node.id, node]) || []), [topology])
  const isTopologyTypeVisible = useCallback((type: AssetType) => {
    const typeIndex = topologyTypeOrder.indexOf(type)
    return topologyTypeOrder.slice(0, typeIndex + 1).every((itemType) => topologyTypeVisibility[itemType])
  }, [topologyTypeVisibility])
  const inspectedNode = nodeMap.get(inspectedNodeId) || topology?.nodes[0]
  const inspectedAsset = inspectedNode?.asset || topology?.root
  const rootNodes = useMemo(() => {
    const nodes = topology?.nodes.filter((node) => node.type === 'root') || []
    if (nodes.length || topology?.root.type !== 'root') return nodes
    return [{ id: `root:${topology.root.value}`, type: 'root' as const, label: topology.root.value, asset: topology.root }]
  }, [topology])
  const domainNodes = useMemo(() => topology?.nodes.filter((node) => node.type === 'domain') || [], [topology])
  const ipNodes = useMemo(() => topology?.nodes.filter((node) => node.type === 'ip') || [], [topology])
  const endpointNodes = useMemo(() => topology?.nodes.filter((node) => node.type === 'endpoint') || [], [topology])
  const technologyNodes = useMemo(() => topology?.nodes.filter((node) => node.type === 'technology') || [], [topology])
  const { visibleNodeIDs, childDisplayControls } = useMemo(() => {
    const nodesByID = new Map(topology?.nodes.map((node) => [node.id, node]) || [])
    const childrenByParent = new Map<string, string[]>()
    topology?.edges.forEach((edge) => {
      if (!nodesByID.has(edge.source) || !nodesByID.has(edge.target)) return
      const children = childrenByParent.get(edge.source) || []
      if (!children.includes(edge.target)) children.push(edge.target)
      childrenByParent.set(edge.source, children)
    })

    const entryNodes = topology?.nodes.filter((node) => (
      node.asset?.type === topology.root.type && node.asset.value === topology.root.value
    )) || []
    const entryIDs = entryNodes.length
      ? entryNodes.map((node) => node.id)
      : rootNodes.map((node) => node.id)
    const visibleIDs = new Set(entryIDs)
    const controls = new Map<string, ChildDisplayControl>()
    const queue = [...entryIDs]

    while (queue.length) {
      const parentID = queue.shift()
      if (!parentID) continue
      const children = childrenByParent.get(parentID) || []
      const limit = childDisplayLimits[parentID] || 10
      const visibleChildren = children.slice(0, limit)
      const firstVisibleChild = nodesByID.get(visibleChildren[0])
      if (children.length > visibleChildren.length && firstVisibleChild) {
        controls.set(parentID, {
          total: children.length,
          visible: visibleChildren.length,
          targetType: firstVisibleChild.type,
          parentLabel: nodesByID.get(parentID)?.label || parentID,
        })
      }
      visibleChildren.forEach((childID) => {
        if (visibleIDs.has(childID)) return
        visibleIDs.add(childID)
        queue.push(childID)
      })
    }

    return { visibleNodeIDs: visibleIDs, childDisplayControls: controls }
  }, [childDisplayLimits, rootNodes, topology])
  const visibleRootNodes = useMemo(() => rootNodes.filter((node) => visibleNodeIDs.has(node.id)), [rootNodes, visibleNodeIDs])
  const visibleDomainNodes = useMemo(() => domainNodes.filter((node) => visibleNodeIDs.has(node.id)), [domainNodes, visibleNodeIDs])
  const visibleIPNodes = useMemo(() => ipNodes.filter((node) => visibleNodeIDs.has(node.id)), [ipNodes, visibleNodeIDs])
  const visibleEndpointNodes = useMemo(() => endpointNodes.filter((node) => visibleNodeIDs.has(node.id)), [endpointNodes, visibleNodeIDs])
  const visibleTechnologyNodes = useMemo(() => technologyNodes.filter((node) => visibleNodeIDs.has(node.id)), [technologyNodes, visibleNodeIDs])
  const { technologyGroups, technologyGroupByNodeID } = useMemo(() => {
    if (!isTopologyTypeVisible('technology')) {
      return { technologyGroups: [], technologyGroupByNodeID: new Map<string, string>() }
    }
    const technologyByID = new Map(visibleTechnologyNodes.map((node) => [node.id, node]))
    const endpointsByID = new Map(endpointNodes.map((node) => [node.id, node]))
    const groupsBySource = new Map<string, TechnologyGroup>()
    const groupByNodeID = new Map<string, string>()

    topology?.edges.forEach((edge) => {
      if (!visibleNodeIDs.has(edge.source)) return
      const technology = technologyByID.get(edge.target)
      if (!technology) return
      const endpoint = endpointsByID.get(edge.source)
      if (endpoint && getEndpointDisplay(endpoint).protocol.toLowerCase() === technology.label.trim().toLowerCase()) return
      const groupID = `technology-group:${edge.source}`
      let group = groupsBySource.get(edge.source)
      if (!group) {
        group = { id: groupID, nodes: [] }
        groupsBySource.set(edge.source, group)
      }
      if (!group.nodes.some((node) => node.id === technology.id)) group.nodes.push(technology)
      groupByNodeID.set(technology.id, groupID)
    })

    return { technologyGroups: Array.from(groupsBySource.values()), technologyGroupByNodeID: groupByNodeID }
  }, [endpointNodes, isTopologyTypeVisible, topology, visibleNodeIDs, visibleTechnologyNodes])
  const topologyColumns = useMemo<TopologyColumnData[]>(() => {
    const columns: TopologyColumnData[] = [
      { type: 'root', nodes: visibleRootNodes },
      { type: 'domain', nodes: visibleDomainNodes },
      { type: 'ip', nodes: visibleIPNodes },
      { type: 'endpoint', nodes: visibleEndpointNodes },
    ]
    return columns.filter((column) => column.nodes.length > 0 && isTopologyTypeVisible(column.type))
  }, [isTopologyTypeVisible, visibleDomainNodes, visibleEndpointNodes, visibleIPNodes, visibleRootNodes])

  const loadMoreChildren = useCallback((nodeID: string) => {
    setChildDisplayLimits((current) => ({ ...current, [nodeID]: (current[nodeID] || 10) + 10 }))
  }, [])

  const showAllChildren = useCallback((nodeID: string) => {
    setChildDisplayLimits((current) => ({ ...current, [nodeID]: Number.MAX_SAFE_INTEGER }))
  }, [])

  const registerTopologyNode = useCallback((id: string, element: HTMLDivElement | null) => {
    if (element) topologyNodeRefs.current.set(id, element)
    else topologyNodeRefs.current.delete(id)
  }, [])

  const updateGraphCanvasSize = useCallback(() => {
    const viewport = graphViewportRef.current
    const content = graphContentRef.current
    if (!viewport || !content) return

    const scale = zoom / 100
    const nextSize = {
      width: Math.max(viewport.clientWidth, Math.ceil(content.offsetWidth * scale)),
      height: Math.max(viewport.clientHeight, Math.ceil(content.offsetHeight * scale)),
    }

    setGraphCanvasSize((current) => (
      current.width === nextSize.width && current.height === nextSize.height ? current : nextSize
    ))
  }, [zoom])

  const updateTopologyRoutes = useCallback(() => {
    const content = graphContentRef.current
    if (!content || !topology) {
      setTopologyRoutes([])
      return
    }

    const contentRect = content.getBoundingClientRect()
    const scale = zoom / 100
    const nodePoints = new Map<string, { source: { x: number; y: number }; target: { x: number; y: number } }>()
    topologyNodeRefs.current.forEach((element, id) => {
      const rect = element.getBoundingClientRect()
      nodePoints.set(id, {
        source: { x: (rect.right - contentRect.left) / scale, y: (rect.top + rect.height / 2 - contentRect.top) / scale },
        target: { x: (rect.left - contentRect.left) / scale, y: (rect.top + rect.height / 2 - contentRect.top) / scale },
      })
    })

    const normalizedEdges = new Map<string, TopologyEdge>()
    topology.edges.forEach((edge) => {
      const target = technologyGroupByNodeID.get(edge.target) || edge.target
      const normalizedEdge = target === edge.target ? edge : { ...edge, target }
      const key = `${normalizedEdge.source}:${normalizedEdge.target}:${normalizedEdge.kind}`
      if (nodePoints.has(normalizedEdge.source) && nodePoints.has(normalizedEdge.target)) normalizedEdges.set(key, normalizedEdge)
    })
    const edges = Array.from(normalizedEdges.values())
    const outbound = new Map<string, TopologyEdge[]>()
    const inbound = new Map<string, TopologyEdge[]>()
    edges.forEach((edge) => {
      outbound.set(edge.source, [...(outbound.get(edge.source) || []), edge])
      inbound.set(edge.target, [...(inbound.get(edge.target) || []), edge])
    })

    const routes: TopologyRoute[] = []
    const routedEdges = new Set<string>()
    const edgeKey = (edge: TopologyEdge) => `${edge.source}:${edge.target}:${edge.kind}`
    // Highlight the complete lineage back to the root, not just the edge adjacent
    // to the selected node. Technology nodes are rendered inside a grouped target,
    // so start their lineage from that rendered group instead.
    const inspectedRouteNodeID = inspectedNode?.type === 'technology'
      ? technologyGroupByNodeID.get(inspectedNode.id) || inspectedNode.id
      : inspectedNode?.id
    const activeEdgeKeys = new Set<string>()
    const visitedNodeIDs = new Set(inspectedRouteNodeID ? [inspectedRouteNodeID] : [])
    const lineageQueue = inspectedRouteNodeID ? [inspectedRouteNodeID] : []
    while (lineageQueue.length) {
      const targetID = lineageQueue.shift()
      if (!targetID) continue
      const incomingEdges = inbound.get(targetID) || []
      incomingEdges.forEach((edge) => {
        activeEdgeKeys.add(edgeKey(edge))
        if (!visitedNodeIDs.has(edge.source)) {
          visitedNodeIDs.add(edge.source)
          lineageQueue.push(edge.source)
        }
      })
    }
    // A selected subdomain should show its immediate context on both sides:
    // the lineage back to the root and the IPs it resolves to. Do not continue
    // beyond those IPs into services, which would make the selection ambiguous.
    if (inspectedNode?.type === 'domain' && inspectedRouteNodeID) {
      const childEdges = outbound.get(inspectedRouteNodeID) || []
      childEdges.forEach((edge) => activeEdgeKeys.add(edgeKey(edge)))
    }
    const isActive = (edge: TopologyEdge) => activeEdgeKeys.has(edgeKey(edge))
    const curve = (from: { x: number; y: number }, to: { x: number; y: number }) => {
      const controlOffset = Math.max(24, Math.abs(to.x - from.x) * 0.42)
      return `M ${from.x} ${from.y} C ${from.x + controlOffset} ${from.y}, ${to.x - controlOffset} ${to.y}, ${to.x} ${to.y}`
    }

    outbound.forEach((sourceEdges, sourceID) => {
      if (sourceEdges.length < 2) return
      const source = nodePoints.get(sourceID)?.source
      const targets = sourceEdges.map((edge) => nodePoints.get(edge.target)?.target).filter((point): point is { x: number; y: number } => Boolean(point))
      if (!source || !targets.length) return
      const railX = Math.min(...targets.map((point) => point.x)) - Math.max(28, (Math.min(...targets.map((point) => point.x)) - source.x) * 0.58)
      const minY = Math.min(source.y, ...targets.map((point) => point.y))
      const maxY = Math.max(source.y, ...targets.map((point) => point.y))
      const activeSourceEdges = sourceEdges.filter(isActive)
      // Keep the shared rail muted, then overlay only the section traversed by
      // the selected lineage. Highlighting the whole rail would spill into
      // unrelated sibling branches.
      routes.push({ id: `source-rail:${sourceID}`, path: `${curve(source, { x: railX, y: source.y })} M ${railX} ${minY} L ${railX} ${maxY}`, active: false })
      if (activeSourceEdges.length) {
        routes.push({ id: `source-trunk:${sourceID}`, path: curve(source, { x: railX, y: source.y }), active: true })
        activeSourceEdges.forEach((edge) => {
          const target = nodePoints.get(edge.target)?.target
          if (!target) return
          routes.push({ id: `source-lineage:${edgeKey(edge)}`, path: `M ${railX} ${source.y} L ${railX} ${target.y}`, active: true })
        })
      }
      sourceEdges.forEach((edge) => {
        const target = nodePoints.get(edge.target)?.target
        if (!target) return
        routes.push({ id: `source-branch:${edgeKey(edge)}`, path: curve({ x: railX, y: target.y }, target), active: isActive(edge) })
        routedEdges.add(edgeKey(edge))
      })
    })

    inbound.forEach((targetEdges, targetID) => {
      const pendingEdges = targetEdges.filter((edge) => !routedEdges.has(edgeKey(edge)))
      if (pendingEdges.length < 2) return
      const target = nodePoints.get(targetID)?.target
      const sources = pendingEdges.map((edge) => nodePoints.get(edge.source)?.source).filter((point): point is { x: number; y: number } => Boolean(point))
      if (!target || !sources.length) return
      const railX = target.x - Math.max(28, (target.x - Math.max(...sources.map((point) => point.x))) * 0.3)
      const minY = Math.min(target.y, ...sources.map((point) => point.y))
      const maxY = Math.max(target.y, ...sources.map((point) => point.y))
      const activeTargetEdges = pendingEdges.filter(isActive)
      pendingEdges.forEach((edge) => {
        const source = nodePoints.get(edge.source)?.source
        if (!source) return
        routes.push({ id: `target-branch:${edgeKey(edge)}`, path: curve(source, { x: railX, y: source.y }), active: isActive(edge) })
        routedEdges.add(edgeKey(edge))
      })
      routes.push({ id: `target-rail:${targetID}`, path: `M ${railX} ${minY} L ${railX} ${maxY} ${curve({ x: railX, y: target.y }, target)}`, active: false })
      if (activeTargetEdges.length) {
        routes.push({ id: `target-trunk:${targetID}`, path: curve({ x: railX, y: target.y }, target), active: true })
        activeTargetEdges.forEach((edge) => {
          const source = nodePoints.get(edge.source)?.source
          if (!source) return
          routes.push({ id: `target-lineage:${edgeKey(edge)}`, path: `M ${railX} ${source.y} L ${railX} ${target.y}`, active: true })
        })
      }
    })

    edges.forEach((edge) => {
      if (routedEdges.has(edgeKey(edge))) return
      const source = nodePoints.get(edge.source)?.source
      const target = nodePoints.get(edge.target)?.target
      if (!source || !target) return
      routes.push({ id: `direct:${edgeKey(edge)}`, path: curve(source, target), active: isActive(edge) })
    })

    setTopologyRoutes(routes)
  }, [inspectedNode, technologyGroupByNodeID, topology, zoom])

  useEffect(() => {
    const refreshGraphLayout = () => {
      updateGraphCanvasSize()
      updateTopologyRoutes()
    }
    const frame = window.requestAnimationFrame(refreshGraphLayout)
    const viewport = graphViewportRef.current
    const content = graphContentRef.current
    const resizeObserver = typeof ResizeObserver === 'undefined' ? null : new ResizeObserver(refreshGraphLayout)
    if (viewport) {
      viewport.addEventListener('scroll', updateTopologyRoutes, { passive: true })
      resizeObserver?.observe(viewport)
    }
    if (content) resizeObserver?.observe(content)
    return () => {
      window.cancelAnimationFrame(frame)
      viewport?.removeEventListener('scroll', updateTopologyRoutes)
      resizeObserver?.disconnect()
    }
  }, [topologyColumns, updateGraphCanvasSize, updateTopologyRoutes])

  const selectNode = (node: TopologyNode) => {
    setInspectedNodeId(node.id)
    setIsDetailPanelOpen(true)
    // Child domains and IPs belong to the currently displayed topology. Selecting
    // either retains that graph so the route renderer can emphasize its connected
    // edges instead of reloading a new topology rooted at the selected node.
    if (node.type === 'root') {
      setSelectedEntry({ type: node.type, value: node.asset?.value || node.label })
    }
  }

  const exportTopology = async () => {
    if (!topology) return
    try {
      const blob = new Blob([JSON.stringify(topology, null, 2)], { type: 'application/json;charset=utf-8' })
      const result = await saveBlobAsFile(blob, `asset-topology-${topology.root.value.replaceAll(/[^a-zA-Z0-9.-]/g, '_')}.json`)
      if (result === 'saved') toast.success(isEnglish ? 'Topology exported' : '拓扑已导出')
    } catch (exportError) {
      toast.error(exportError instanceof Error ? exportError.message : (isEnglish ? 'Export failed' : '导出失败'))
    }
  }

  const sourceTaskIds = inspectedAsset?.sourceTaskIds || []
  const connectedIPs = inspectedAsset?.type === 'domain' ? inspectedAsset.ips || [] : []
  const relatedEndpoints = endpointNodes.filter((node) => {
    if (!inspectedAsset?.value) return false
    return node.asset?.endpointHost === inspectedAsset.value
  })
  const displayedEndpoints = relatedEndpoints.length ? relatedEndpoints : endpointNodes
  const detailRows: Array<[string, string]> = inspectedAsset
    ? [
        [copy.assetType, topologyLabel(inspectedAsset.type, isEnglish)],
        ...(inspectedAsset.type === 'ip'
          ? [
              [isEnglish ? 'Operator' : '运营商', inspectedAsset.operator || '-'] as [string, string],
              [isEnglish ? 'Location' : '地理位置', inspectedAsset.location || '-'] as [string, string],
              ['ASN', inspectedAsset.asn || '-'] as [string, string],
            ]
          : []),
        ...(connectedIPs.length ? [[copy.relatedIPs, `${connectedIPs.length}`] as [string, string]] : []),
        [copy.firstSeen, formatAssetTime(inspectedAsset.firstSeenAt, isEnglish)],
        [copy.lastUpdated, formatAssetTime(inspectedAsset.updatedAt, isEnglish)],
      ]
    : []

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-background xl:overflow-hidden" aria-busy={loadingAssets || loadingTopology}>
      <header className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border px-3 py-3 md:px-6 md:py-4">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{copy.title}</h1>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input ref={assetSearchRef} value={keyword} onChange={(event) => { setKeyword(event.target.value); setEntryPage(1) }} className="pl-9 pr-10" placeholder={copy.search} />
            <kbd className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 rounded border border-border bg-muted px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground">/</kbd>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button size="icon-sm" variant="outline" aria-label={isGraphFullscreen ? copy.exitFullscreen : copy.fullscreen} onClick={() => void toggleGraphFullscreen()} disabled={!topology}>
                {isGraphFullscreen ? <Minimize2 /> : <Maximize2 />}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{isGraphFullscreen ? copy.exitFullscreen : copy.fullscreen}</TooltipContent>
          </Tooltip>
          <div className="flex items-center rounded-md border border-border bg-muted/30 p-0.5">
            <Button size="icon-sm" variant="ghost" aria-label={copy.zoomOut} onClick={() => setZoom((value) => Math.max(70, value - 10))} disabled={!topology}><ZoomOut /></Button>
            <span className="min-w-12 text-center text-xs text-muted-foreground">{zoom}%</span>
            <Button size="icon-sm" variant="ghost" aria-label={copy.zoomIn} onClick={() => setZoom((value) => Math.min(140, value + 10))} disabled={!topology}><ZoomIn /></Button>
          </div>
          <Button size="sm" onClick={() => void exportTopology()} disabled={!topology}>
            <Download />{copy.export}
          </Button>
        </div>
      </header>

      <div className={cn(
        'grid min-h-0 flex-none grid-cols-1 xl:flex-1',
        isDetailPanelOpen ? 'xl:grid-cols-[20rem_minmax(0,1fr)_19rem]' : 'xl:grid-cols-[20rem_minmax(0,1fr)]',
      )}>
        <aside className="h-80 min-h-0 xl:h-auto border-b border-border bg-card/20 xl:border-b-0 xl:border-r">
          <div className="flex h-full min-h-0 flex-col p-4">
            <div className="flex items-center justify-between gap-2">
              <h2 className="font-medium">{copy.observedAssets}</h2>
              <Badge variant="outline">{entryTotal}</Badge>
            </div>
            <div className="mt-3 grid grid-cols-[minmax(0,1fr)_4.5rem] gap-2 border-b border-border px-2 pb-2 text-[11px] text-muted-foreground">
              <span>{copy.asset}</span><span className="text-right">{copy.type}</span>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto">
              {loadingAssets ? <div className="flex h-44 items-center justify-center"><Spinner /></div> : assets.map((asset) => {
                const active = selectedEntry?.type === asset.type && selectedEntry.value === asset.value
                return (
                  <button key={`${asset.type}:${asset.value}`} type="button" onClick={() => setSelectedEntry({ type: asset.type as EntryAssetType, value: asset.value })}
                    className={cn('grid w-full grid-cols-[minmax(0,1fr)_4.5rem] items-center gap-2 border-b border-border/70 px-2 py-2.5 text-left transition-colors hover:bg-muted/40', active && 'border-l-2 border-l-primary bg-muted/55')}>
                    <span className="flex min-w-0 items-center gap-2"><TopologyTypeIcon type={asset.type} className={cn('size-3.5 shrink-0', active ? 'text-primary' : 'text-muted-foreground')} /><span className="truncate font-mono text-xs">{asset.value}</span></span>
                    <span className="text-right text-[11px] text-muted-foreground">{topologyLabel(asset.type, isEnglish)}</span>
                  </button>
                )
              })}
              {!loadingAssets && assets.length === 0 && <p className="px-2 py-8 text-center text-sm text-muted-foreground">{copy.noAssets}</p>}
            </div>
            <PaginationPro current={entryPage} total={entryTotal} pageSize={entryPageSize} onChange={(nextPage) => setEntryPage(nextPage)} showSizeChanger={false} showTotal={false} middlePageCount={3} compact disabled={loadingAssets} className="mt-3 border-t border-border pt-3" />
          </div>
        </aside>

        <main className={cn(
          'relative h-[36rem] min-h-[36rem] min-w-0 overflow-hidden xl:h-auto border-b border-border bg-background xl:border-b-0',
          isDetailPanelOpen && 'xl:border-r',
        )}>
          {error ? <div className="m-6 rounded-lg border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">{error}</div> : loadingTopology ? (
            <div className="flex h-full items-center justify-center"><Spinner /></div>
          ) : !topology ? (
            <div className="flex h-full items-center justify-center px-6 text-center text-sm text-muted-foreground">{copy.noTopology}</div>
          ) : (
            <section ref={graphPanelRef} className={cn('relative flex h-full min-h-0 flex-col p-4', isGraphFullscreen && 'fixed inset-0 z-50 h-screen w-screen bg-background p-5')}>
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="flex items-center gap-2"><Link2 className="size-4 text-primary" /><h2 className="truncate text-sm font-medium">{topology.root.value}</h2><Badge variant="outline">{topologyLabel(topology.root.type, isEnglish)}</Badge></div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline">{topology.nodes.length} {copy.relatedAssets}</Badge>
                  {isGraphFullscreen && <Tooltip>
                    <TooltipTrigger asChild>
                      <Button size="icon-sm" variant="outline" aria-label={copy.exitFullscreen} onClick={() => void toggleGraphFullscreen()}><Minimize2 /></Button>
                    </TooltipTrigger>
                    <TooltipContent>{copy.exitFullscreen}</TooltipContent>
                  </Tooltip>}
                </div>
              </div>
              <div className="relative mt-4 min-h-0 flex-1">
                <div
                  ref={graphViewportRef}
                  onWheel={handleGraphWheel}
                  onPointerDown={handleGraphPointerDown}
                  onPointerMove={handleGraphPointerMove}
                  onPointerUp={stopGraphDrag}
                  onPointerCancel={stopGraphDrag}
                  onLostPointerCapture={stopGraphDrag}
                  className={cn(
                    'relative h-full min-h-0 overflow-auto rounded-lg border border-border bg-muted/20',
                    isGraphDragging ? 'cursor-grabbing select-none' : 'cursor-grab',
                  )}
                >
                  <div
                    className="relative min-h-full min-w-full"
                    style={{ width: graphCanvasSize.width || undefined, height: graphCanvasSize.height || undefined }}
                  >
                    <div ref={graphContentRef} className="absolute left-0 top-0 w-max p-7" style={{ transform: `scale(${zoom / 100})`, transformOrigin: 'top left' }}>
                    <svg className="pointer-events-none absolute inset-0 size-full overflow-visible text-muted-foreground/70" aria-hidden="true">
                      {topologyRoutes.map((route) => <g key={route.id} className={cn(route.active ? 'text-primary opacity-80' : 'opacity-25')}>
                        <path d={route.path} fill="none" stroke="currentColor" strokeWidth={route.active ? '1.6' : '1.2'} />
                      </g>)}
                    </svg>
                    <div className="relative z-10 flex min-h-[28rem] w-max items-start gap-5 md:items-center">
                      {topologyColumns.map((column, index) => <Fragment key={column.type}>
                        <TopologyColumn
                          type={column.type}
                          nodes={column.nodes}
                          activeId={inspectedNode?.id}
                          onSelect={selectNode}
                          onNodeElement={registerTopologyNode}
                          childDisplayControls={childDisplayControls}
                          onLoadMore={loadMoreChildren}
                          onShowAll={showAllChildren}
                          labels={{ loadMore: copy.loadMore, showAll: copy.showAll, showing: isEnglish ? (visible, total) => `${visible} of ${total}` : (visible, total) => `已显示 ${visible}/${total}` }}
                        />
                        {index < topologyColumns.length - 1 && <RelationRail />}
                      </Fragment>)}
                      {technologyGroups.length > 0 && <>
                        <RelationRail />
                        <TechnologyResultsColumn
                          groups={technologyGroups}
                          activeId={inspectedNode?.id}
                          onSelect={selectNode}
                          onGroupElement={registerTopologyNode}
                        />
                      </>}
                    </div>
                    </div>
                  </div>
                </div>
                {isGraphFullscreen && <div className="absolute bottom-3 right-3 z-20 flex items-center rounded-md border border-border bg-background/90 p-0.5 shadow-md backdrop-blur">
                  <Button size="icon-sm" variant="ghost" aria-label={copy.zoomOut} onClick={() => setZoom((value) => Math.max(70, value - 10))}><ZoomOut /></Button>
                  <span className="min-w-12 text-center text-xs text-muted-foreground">{zoom}%</span>
                  <Button size="icon-sm" variant="ghost" aria-label={copy.zoomIn} onClick={() => setZoom((value) => Math.min(140, value + 10))}><ZoomIn /></Button>
                </div>}
              </div>
              <div className="mt-3 flex flex-wrap items-center justify-center gap-x-5 gap-y-2 rounded-lg border border-border bg-card/40 px-4 py-2 text-xs text-muted-foreground">
                {(['root', 'domain', 'ip', 'endpoint', 'technology'] as AssetType[]).map((type) => {
                  const isVisible = isTopologyTypeVisible(type)
                  return <button
                    key={type}
                    type="button"
                    onClick={() => setTopologyTypeVisibility((current) => ({ ...current, [type]: !current[type] }))}
                    className={cn('flex items-center gap-1.5 rounded-md px-1.5 py-1 transition-colors hover:bg-muted/70', !isVisible && 'opacity-45')}
                    aria-pressed={topologyTypeVisibility[type]}
                  >
                    <TopologyTypeIcon type={type} className={cn('size-3.5', type === 'technology' ? 'text-success' : 'text-primary')} />
                    {topologyLabel(type, isEnglish)}
                  </button>
                })}
              </div>
            </section>
          )}
        </main>

        {isDetailPanelOpen && <aside className="min-h-0 bg-card/20">
          <div className="h-full space-y-3 overflow-y-auto p-4">
            <div className="flex items-start gap-3 px-1 py-1">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-full border border-primary/30 bg-muted text-primary">
                {inspectedNode && <TopologyTypeIcon type={inspectedNode.type} className="size-5" />}
              </span>
              <div className="min-w-0 flex-1">
                <h2 className="truncate font-mono text-sm font-semibold">{inspectedNode?.label || '-'}</h2>
                {inspectedNode && <Badge variant="outline" className="mt-2">{topologyLabel(inspectedNode.type, isEnglish)}</Badge>}
              </div>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button size="icon-sm" variant="ghost" aria-label={copy.closeDetails} onClick={() => setIsDetailPanelOpen(false)}>
                    <X />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{copy.closeDetails}</TooltipContent>
              </Tooltip>
            </div>
            {inspectedAsset ? <>
              <DetailSection title={copy.overview}><DetailRows rows={detailRows} /></DetailSection>
              {displayedEndpoints.length > 0 && <DetailSection title={`${copy.portServices} (${displayedEndpoints.length})`}>
                {displayedEndpoints.slice(0, 8).map((node) => {
                  const endpoint = getEndpointDisplay(node)
                  return <button key={node.id} type="button" onClick={() => setInspectedNodeId(node.id)} className="flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted/60">
                    <span className="flex min-w-0 items-center gap-2"><Badge variant="outline" className="h-5 rounded-md px-1.5 font-mono text-[10px]">{endpoint.port}</Badge><span className="truncate font-medium">{endpoint.protocol || '-'}</span></span>
                    <span className="flex min-w-0 items-center gap-2"><span className="truncate text-muted-foreground">{getEndpointVendor(node) || endpoint.protocol || '-'}</span><span className="size-1.5 shrink-0 rounded-full bg-success" title={isEnglish ? 'Open' : '开放'} /></span>
                  </button>
                })}
              </DetailSection>}
              {sourceTaskIds.length > 0 && <DetailSection title={`${copy.sourceTasks} (${sourceTaskIds.length})`}>
                {sourceTaskIds.map((taskID) => <button key={taskID} type="button" onClick={() => router.push(`/scan/vuln-scan/detail?taskId=${encodeURIComponent(taskID)}&tab=info`)} className="flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-xs text-primary hover:bg-muted/65"><span className="truncate font-mono">{taskID}</span><ExternalLink className="size-3.5 shrink-0" /></button>)}
              </DetailSection>}
            </> : <p className="py-10 text-center text-sm text-muted-foreground">{copy.noTopology}</p>}
          </div>
        </aside>}
      </div>
    </div>
  )
}

function RelationRail() {
  return <div className="w-12 shrink-0" aria-hidden="true" />
}

function TopologyColumn({ type, nodes, activeId, onSelect, onNodeElement, childDisplayControls, onLoadMore, onShowAll, labels }: {
  type: AssetType
  nodes: TopologyNode[]
  activeId?: string
  onSelect: (node: TopologyNode) => void
  onNodeElement: (id: string, element: HTMLDivElement | null) => void
  childDisplayControls: Map<string, ChildDisplayControl>
  onLoadMore: (nodeID: string) => void
  onShowAll: (nodeID: string) => void
  labels: { loadMore: string; showAll: string; showing: (visible: number, total: number) => string }
}) {
  const controls = Array.from(childDisplayControls.entries()).filter(([, control]) => control.targetType === type)
  return <div className="flex w-44 shrink-0 flex-col justify-center gap-3">{nodes.map((node) => {
    return <div key={node.id}>
      <div ref={(element) => onNodeElement(node.id, element)}><TopologyNodeCard node={node} active={node.id === activeId} onClick={() => onSelect(node)} /></div>
    </div>
  })}
    {controls.length > 0 && <div className="space-y-2 border-t border-border/70 pt-2 text-[11px] text-muted-foreground">{controls.map(([parentID, control]) => <div key={parentID} className="space-y-1">
      <div className="truncate" title={control.parentLabel}>{control.parentLabel} · {labels.showing(control.visible, control.total)}</div>
      <div className="flex flex-wrap gap-1">
          <button type="button" className="rounded px-1 py-0.5 hover:bg-muted hover:text-foreground" onClick={() => onLoadMore(parentID)}>{labels.loadMore}</button>
          <button type="button" className="rounded px-1 py-0.5 hover:bg-muted hover:text-foreground" onClick={() => onShowAll(parentID)}>{labels.showAll}</button>
      </div>
    </div>)}</div>}
  </div>
}

function TechnologyResultsColumn({ groups, activeId, onSelect, onGroupElement }: {
  groups: TechnologyGroup[]
  activeId?: string
  onSelect: (node: TopologyNode) => void
  onGroupElement: (id: string, element: HTMLDivElement | null) => void
}) {
  return <div className="flex w-fit max-w-64 shrink-0 flex-col justify-center gap-3">{groups.map((group) => (
    <div key={group.id} ref={(element) => onGroupElement(group.id, element)} className="w-fit max-w-full rounded-lg border border-dashed border-border bg-card/35 p-2.5">
      <div className="space-y-2">{group.nodes.map((node) => (
        <button
          key={node.id}
          type="button"
          onClick={() => onSelect(node)}
          className={cn(
            'w-fit max-w-full whitespace-nowrap rounded-md border border-border bg-card px-2.5 py-2 text-left text-sm shadow-sm transition-colors hover:border-primary/60 hover:bg-muted/65',
            node.id === activeId && 'border-primary bg-muted/75 ring-1 ring-primary/20',
          )}
        >
          <span className="block truncate font-medium text-foreground">{node.label}</span>
        </button>
      ))}</div>
    </div>
  ))}</div>
}

function DetailRows({ rows }: { rows: Array<[string, string]> }) {
  return <dl className="space-y-3 text-sm">{rows.map(([label, value]) => <div key={label} className="flex items-start justify-between gap-4"><dt className="text-muted-foreground">{label}</dt><dd className="max-w-[11rem] break-all text-right text-foreground">{value}</dd></div>)}</dl>
}

function DetailSection({ title, children }: { title: string; children: React.ReactNode }) {
  return <section className="rounded-lg border border-border bg-card/60 p-3 shadow-sm"><h3 className="mb-3 text-sm font-medium">{title}</h3><div className="space-y-1">{children}</div></section>
}
