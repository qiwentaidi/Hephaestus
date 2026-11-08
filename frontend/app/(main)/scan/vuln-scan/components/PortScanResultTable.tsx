'use client'
import { useIsMobile } from '@/hooks/use-mobile'
import { DetailResultToolbar } from './DetailResultToolbar'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Label } from '@/components/ui/label'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { DataTableWithColumns, ColumnDef, RowSelection } from '@/components/ui/data-table-with-columns'
import {
  Eye,
  Search,
  Network,
  Download,
  Lock,
  Copy,
  ChevronsLeft,
  ChevronsRight,
} from 'lucide-react'
import toast from 'react-hot-toast'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { copyToClipboard } from '@/lib/clipboard'
import type { PortInfo } from '../types'
import { ScanType } from '../types'

interface PortScanResultTableProps {
  data?: PortInfo[]
  taskId?: string
  version?: number
  loading?: boolean
  onExport?: () => void
  onTotalChange?: (total: number) => void
  pollWhileRunning?: boolean
}

interface PortStatItem {
  label: string
  count: number
}

interface PortNumberStatItem {
  port: number
  count: number
}

interface PortStats {
  total: number
  protocolTop5: PortStatItem[]
  portTop5: PortNumberStatItem[]
  hostOpenPortTop5: PortStatItem[]
}

const EMPTY_PORT_STATS: PortStats = {
  total: 0,
  protocolTop5: [],
  portTop5: [],
  hostOpenPortTop5: [],
}

const normalizePortStats = (stats: Partial<PortStats> | null | undefined, total: number): PortStats => ({
  total: Number.isFinite(Number(stats?.total)) ? Number(stats?.total) : total,
  protocolTop5: Array.isArray(stats?.protocolTop5) ? stats.protocolTop5 : [],
  portTop5: Array.isArray(stats?.portTop5) ? stats.portTop5 : [],
  hostOpenPortTop5: Array.isArray(stats?.hostOpenPortTop5) ? stats.hostOpenPortTop5 : [],
})

const buildTopStats = <T,>(
  items: T[],
  getKey: (item: T) => string,
  mapLabel: (key: string) => string = (key) => key,
): PortStatItem[] => {
  const counts = new Map<string, number>()
  items.forEach((item) => {
    const key = getKey(item).trim() || 'unknown'
    counts.set(key, (counts.get(key) || 0) + 1)
  })
  return Array.from(counts.entries())
    .map(([key, count]) => ({ label: mapLabel(key), count }))
    .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
    .slice(0, 5)
}

const formatProtocolLabel = (transport?: string, protocol?: string) => {
  const normalizedTransport = transport?.trim()
  const normalizedProtocol = protocol?.trim()

  if (!normalizedTransport || normalizedTransport.toLowerCase() === 'tcp') {
    return normalizedProtocol || '-'
  }

  if (!normalizedProtocol) {
    return normalizedTransport.toUpperCase()
  }

  return `${normalizedTransport.toUpperCase()} / ${normalizedProtocol}`
}

const formatBannerSize = (value?: string) => {
  if (!value) {
    return '-'
  }

  const bytes = new TextEncoder().encode(value).length
  if (bytes < 1024) {
    return `${bytes} B`
  }
  if (bytes < 1024 * 1024) {
    return `${(bytes / 1024).toFixed(2)} KB`
  }
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
}

export function PortScanResultTable({
  data = [],
  taskId,
  version,
  loading = false,
  onExport,
  onTotalChange,
  pollWhileRunning = false,
}: PortScanResultTableProps) {
  const { t } = useI18n()
  const isMobile = useIsMobile()
  const [searchKeyword, setSearchKeyword] = useState('')
  const [selectedRowKeys, setSelectedRowKeys] = useState<(string | number)[]>([])
  const [selectedRows, setSelectedRows] = useState<PortInfo[]>([])
  const [selectedBanner, setSelectedBanner] = useState<PortInfo | null>(null)
  const [bannerDialogOpen, setBannerDialogOpen] = useState(false)

  // 分页状态
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20)
  const [serverData, setServerData] = useState<PortInfo[]>([])
  const [totalCount, setTotalCount] = useState(0)
  const [serverLoading, setServerLoading] = useState(false)
  const [serverStatsState, setServerStatsState] = useState<{ key: string; stats: PortStats | null }>({
    key: '',
    stats: null,
  })
  const [statsCollapsed, setStatsCollapsed] = useState(false)

  const useServerPagination = !!taskId
  const paginationKey = `${taskId || 'client'}:${version ?? 'latest'}`
  const statsRequestKey = `${paginationKey}:${searchKeyword.trim()}`
  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1
  const serverStats = serverStatsState.key === statsRequestKey ? serverStatsState.stats : null

  const fetchServerData = useCallback(async () => {
    if (!taskId) return

    setServerLoading(true)
    try {
      const queryParams = new URLSearchParams({
        page: String(currentPage),
        pageSize: String(pageSize),
      })
      if (version) queryParams.set('version', String(version))
      if (searchKeyword.trim()) queryParams.set('search', searchKeyword.trim())

      const response = await fetchWithAuth(`/api/v1/portscan/task/${taskId}/results?${queryParams.toString()}`)
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }

      const result = await response.json()
      if (result.success && result.data) {
        const mappedPorts = (result.data.ports || []).map((port: any, index: number) => ({
          id: `${port.host}-${port.port}-${port.transport || 'unknown'}-${index}`,
          taskId,
          host: port.host,
          assets: Array.isArray(port.assets) ? port.assets : [],
          subdomains: Array.isArray(port.subdomains) ? port.subdomains : [],
          port: port.port,
          transport: port.transport,
          protocol: port.protocol || 'tcp',
          discoveryStatus: port.discoveryStatus === 'pending' ? 'pending' : 'verified',
          product: port.product,
          service: port.service,
          version: port.version,
          state: 'open' as const,
          banner: port.rawResponse || port.banner,
          rawResponse: port.rawResponse,
          createdAt: port.createdAt || new Date().toISOString(),
        }))
        const total = result.data.pagination?.total || 0
        setServerData(mappedPorts)
        setTotalCount(total)
        setServerStatsState({
          key: statsRequestKey,
          stats: normalizePortStats(result.data.stats, total),
        })
        onTotalChange?.(total)
      }
    } catch (error) {
      console.error('Failed to fetch port scan results:', error)
      setServerData([])
      setTotalCount(0)
      setServerStatsState({ key: statsRequestKey, stats: null })
      onTotalChange?.(0)
    } finally {
      setServerLoading(false)
    }
  }, [currentPage, onTotalChange, pageSize, searchKeyword, statsRequestKey, taskId, version])

  useEffect(() => {
    if (!useServerPagination) return
    const timer = setTimeout(() => {
      fetchServerData()
    }, 300)
    return () => clearTimeout(timer)
  }, [fetchServerData, useServerPagination])

  useEffect(() => {
    if (!useServerPagination || !pollWhileRunning) return
    const timer = window.setInterval(() => { void fetchServerData() }, 2000)
    return () => window.clearInterval(timer)
  }, [fetchServerData, pollWhileRunning, useServerPagination])

  const displayData = useServerPagination ? serverData : data

  // 前端搜索过滤
  const filteredResults = useMemo(() => {
    if (useServerPagination) {
      return displayData
    }
    if (!searchKeyword.trim()) {
      return displayData
    }
    const keyword = searchKeyword.toLowerCase()
    return displayData.filter(item =>
      item.host.toLowerCase().includes(keyword) ||
      item.subdomains?.some((subdomain) => subdomain.toLowerCase().includes(keyword)) ||
      item.assets?.some((asset) =>
        asset.subdomain?.toLowerCase().includes(keyword) ||
        asset.domain?.toLowerCase().includes(keyword) ||
        asset.assetType?.toLowerCase().includes(keyword) ||
        asset.assetName?.toLowerCase().includes(keyword)
      ) ||
      item.port.toString().includes(keyword) ||
      item.transport?.toLowerCase().includes(keyword) ||
      item.protocol?.toLowerCase().includes(keyword) ||
      item.product?.toLowerCase().includes(keyword) ||
      item.version?.toLowerCase().includes(keyword) ||
      item.banner?.toLowerCase().includes(keyword) ||
      item.service?.toLowerCase().includes(keyword)
    )
  }, [displayData, searchKeyword, useServerPagination])

  // 分页数据
  const paginatedResults = useMemo(() => {
    if (useServerPagination) {
      return filteredResults
    }
    const start = (currentPage - 1) * pageSize
    return filteredResults.slice(start, start + pageSize)
  }, [filteredResults, currentPage, pageSize, useServerPagination])

  const totalItems = useServerPagination ? totalCount : filteredResults.length
  const tableLoading = loading || serverLoading

  const stats = useMemo<PortStats>(() => {
    if (useServerPagination) {
      return serverStats || { ...EMPTY_PORT_STATS, total: totalCount }
    }

    const protocolTop5 = buildTopStats(
      filteredResults,
      (item) => formatProtocolLabel(item.transport, item.protocol),
    )
    const portTop5 = buildTopStats(filteredResults, (item) => String(item.port))
      .map((item) => ({ port: Number(item.label), count: item.count }))
      .filter((item) => Number.isFinite(item.port))
    const hostOpenPortTop5 = buildTopStats(filteredResults, (item) => item.host)

    return {
      total: filteredResults.length,
      protocolTop5,
      portTop5,
      hostOpenPortTop5,
    }
  }, [filteredResults, serverStats, totalCount, useServerPagination])

  const hasResolvedStats = !useServerPagination || serverStatsState.key === statsRequestKey
  const showStatsOverview = !hasResolvedStats || tableLoading || stats.total > 0

  useEffect(() => {
    if (!useServerPagination) {
      onTotalChange?.(filteredResults.length)
    }
  }, [filteredResults.length, onTotalChange, useServerPagination])

  const getRowKey = (result: PortInfo, index: number): string => {
    return `${result.host}-${result.port}-${result.transport || 'unknown'}-${index}`
  }

  const handleViewBanner = (result: PortInfo) => {
    setSelectedBanner(result)
    setBannerDialogOpen(true)
  }

  // 获取实际要操作的目标列表（如果有选中项则用选中项，否则用右键点击的项）
  const getTargetsForAction = (contextItem: PortInfo | null): PortInfo[] => {
    if (selectedRows.length > 0) {
      return selectedRows
    }
    return contextItem ? [contextItem] : []
  }

  const handleCopy = (contextItem: PortInfo | null) => {
    const targets = getTargetsForAction(contextItem)
    if (targets.length === 0) {
      toast.error(t.portScan.results.messages.selectTargets)
      return
    }

    // 构建复制内容（格式：host:port protocol）
    const copyText = targets.map(r => {
      const protocol = formatProtocolLabel(r.transport, r.protocol)
      const aliases = r.subdomains && r.subdomains.length > 0 ? ` [${r.subdomains.join(', ')}]` : ''
      return `${r.host}:${r.port} ${protocol}${aliases}`
    }).join('\n')

    void copyToClipboard(copyText).then((success) => {
      if (success) toast.success(t.portScan.results.messages.copied)
      else toast.error(t.common.error)
    })
  }

  const handleSendToWebscan = async (contextItem: PortInfo | null) => {
    const targets = getTargetsForAction(contextItem)
    if (targets.length === 0) {
      toast.error(t.portScan.results.messages.selectTargets)
      return
    }

    // 构建 URL 列表
    const urls = targets.flatMap(r => {
      const protocol = r.protocol?.toLowerCase() === 'https' || r.port === 443 ? 'https' : 'http'
      const hosts = r.subdomains && r.subdomains.length > 0 ? r.subdomains : [r.host]
      return hosts.map((host) => `${protocol}://${host}:${r.port}`)
    })

    // 生成任务名称
    const taskName = `webscan - ${urls[0]} - ${new Date().toLocaleString('zh-CN')}`

    try {
      const requestBody = {
        scanType: ScanType.WebScan,
        taskName,
        config: {
          targets: urls,
          threads: 30,
          webscanOption: 3,
          rootPathScan: true,
          skipNucleiWithoutTags: false,
          skipNucleiWithWAF: true,
          screenshot: true,
          activeTimeoutLimit: 5,
        },
      }

      // 直接调用 API 创建任务
      const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody),
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || '创建任务失败')
      }

      const data = await response.json()
      if (!data.success || !data.taskId) {
        throw new Error(data.error || t.portScan.results.messages.createTaskFailed)
      }

      toast.success(t.portScan.results.messages.createWebscanSuccess.replace('{count}', targets.length.toString()))

      // 清空选择
      setSelectedRowKeys([])
      setSelectedRows([])
    } catch (error: any) {
      toast.error(t.portScan.results.messages.createTaskFailed + ': ' + (error.message || t.portScan.results.messages.unknownError))
    }
  }

  const handleSendToCrack = async (contextItem: PortInfo | null) => {
    const targets = getTargetsForAction(contextItem)
    if (targets.length === 0) {
      toast.error(t.portScan.results.messages.selectTargets)
      return
    }

    // 构建地址列表（格式：协议://ip:port）
    const addresses = targets.map(r => {
      const protocol = r.protocol || 'tcp'
      return `${protocol}://${r.host}:${r.port}`
    })

    // 生成任务名称
    const taskName = `crack - ${addresses[0]} - ${new Date().toLocaleString('zh-CN')}`

    try {
      const requestBody = {
        scanType: ScanType.Crack,
        taskName,
        config: {
          targets: addresses,
          threads: 10,
          username: '',
          password: '',
          defaultDict: true,
        },
      }

      // 直接调用 API 创建任务
      const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody),
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.portScan.results.messages.createTaskFailed)
      }

      const data = await response.json()
      if (!data.success || !data.taskId) {
        throw new Error(data.error || t.portScan.results.messages.createTaskFailed)
      }

      toast.success(t.portScan.results.messages.createCrackSuccess.replace('{count}', targets.length.toString()))

      // 清空选择
      setSelectedRowKeys([])
      setSelectedRows([])
    } catch (error: any) {
      console.error('创建暴破任务失败:', error)
      toast.error(t.portScan.results.messages.createTaskFailed + ': ' + (error.message || t.portScan.results.messages.unknownError))
    }
  }

  const getProtocolBadge = (transport?: string, protocol?: string) => {
    return (
      <Badge variant="outline">
        {formatProtocolLabel(transport, protocol)}
      </Badge>
    )
  }

  const renderStatList = (items?: Array<{ label: string; count: number }> | null) => {
    const safeItems = Array.isArray(items) ? items : []
    if (safeItems.length === 0) {
      return (
        <div className="rounded-md border border-dashed border-border bg-muted/20 px-3 py-3 text-center text-xs text-muted-foreground">
          {t.portScan.results.stats.noData}
        </div>
      )
    }

    return (
      <div className="space-y-2">
        {safeItems.map((item) => (
          <div key={item.label} className="flex items-center justify-between gap-3 rounded-md border border-border bg-muted/30 px-3 py-2 text-foreground">
            <span className="min-w-0 truncate text-sm" title={item.label}>{item.label}</span>
            <span className="shrink-0 text-base font-semibold">{item.count}</span>
          </div>
        ))}
      </div>
    )
  }

  // 行选择配置
  const rowSelection: RowSelection<PortInfo> = {
    selectedRowKeys,
    onChange: (keys, rows) => {
      setSelectedRowKeys(keys)
      setSelectedRows(rows)
    },
  }

  // 定义表格列配置
  const tableColumns = useMemo<ColumnDef<PortInfo>[]>(() => [
    {
      id: 'host',
      header: t.portScan.results.table.host,
      cell: (item) => <span className="font-mono">{item.host}</span>,
      headerClassName: 'min-w-[180px]',
      className: 'min-w-[180px]',
      alwaysVisible: true,
    },
    {
      id: 'port',
      header: t.portScan.results.table.port,
      cell: (item) => <span className="font-mono">{item.port}</span>,
      headerClassName: 'w-[100px] min-w-[100px]',
      className: 'min-w-[100px]',
    },
    {
      id: 'protocol',
      header: t.portScan.results.table.protocol,
      cell: (item) => item.discoveryStatus === 'pending'
        ? <Badge variant="outline" className="border-amber-500/40 text-amber-600 dark:text-amber-400">{t.portScan.results.table.identifying}</Badge>
        : getProtocolBadge(item.transport, item.protocol),
      headerClassName: 'w-[128px] min-w-[128px]',
      className: 'min-w-[128px]',
    },
    {
      id: 'product',
      header: t.portScan.results.table.product,
      cell: (item) => (
        <div className="max-w-[220px] truncate" title={item.product}>
          {item.product || '-'}
        </div>
      ),
      headerClassName: 'w-[180px] min-w-[180px]',
      className: 'min-w-[180px]',
    },
    {
      id: 'version',
      header: t.portScan.results.table.version,
      cell: (item) => (
        <div className="max-w-[160px] truncate font-mono" title={item.version}>
          {item.version || '-'}
        </div>
      ),
      headerClassName: 'w-[152px] min-w-[152px]',
      className: 'min-w-[152px]',
    },
    {
      id: 'banner',
      header: t.portScan.results.table.banner,
      cell: (item) => (
        item.banner ? (
          <button
            type="button"
            className="truncate text-sm font-medium text-foreground/80 underline-offset-4 transition-colors hover:text-foreground hover:underline"
            title={item.banner}
            onClick={() => handleViewBanner(item)}
          >
            {formatBannerSize(item.banner)}
          </button>
        ) : (
          <span>-</span>
        )
      ),
      headerClassName: 'w-[120px] min-w-[120px]',
      className: 'min-w-[120px]',
      defaultVisible: true,
    },
    {
      id: 'actions',
      header: t.portScan.results.table.actions,
      cell: (item) => (
        <div className="flex items-center justify-center">
          <Button
            variant="ghost"
            size="icon"
            className="h-8 w-8"
            onClick={() => handleViewBanner(item)}
          >
            <Eye className="w-4 h-4" />
          </Button>
        </div>
      ),
      headerClassName: 'w-[88px] min-w-[88px] text-center sticky right-0 z-20 border-l border-border bg-card',
      className: 'min-w-[88px] sticky right-0 z-10 border-l border-border bg-card text-center group-hover:bg-muted group-data-[state=selected]:bg-muted',
      alwaysVisible: true,
    },
  ], [t])

  // 自定义行渲染，添加右键菜单
  const renderRow = (item: PortInfo, index: number, children: React.ReactNode) => {
    return (
      <ContextMenu key={getRowKey(item, index)}>
        <ContextMenuTrigger asChild>
          {children}
        </ContextMenuTrigger>
        <ContextMenuContent className="w-56">
          {selectedRows.length > 0 ? (
            <>
              <div className="px-2 py-1.5 text-sm font-semibold">
                {t.portScan.results.contextMenu.selectedTargets.replace('{count}', selectedRows.length.toString())}
              </div>
              <ContextMenuSeparator />
              <ContextMenuItem onClick={() => handleSendToWebscan(item)}>
                <Network className="w-4 h-4 mr-2" />
                {t.portScan.results.contextMenu.sendToWebscan}
              </ContextMenuItem>
              <ContextMenuItem onClick={() => handleSendToCrack(item)}>
                <Lock className="w-4 h-4 mr-2" />
                {t.portScan.results.contextMenu.sendToCrack}
              </ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuItem onClick={() => handleCopy(item)}>
                <Copy className="w-4 h-4 mr-2" />
                {t.portScan.results.contextMenu.copy}
              </ContextMenuItem>
            </>
          ) : (
            <>
              <ContextMenuItem onClick={() => handleSendToWebscan(item)}>
                <Network className="w-4 h-4 mr-2" />
                {t.portScan.results.contextMenu.sendToWebscan}
              </ContextMenuItem>
              <ContextMenuItem onClick={() => handleSendToCrack(item)}>
                <Lock className="w-4 h-4 mr-2" />
                {t.portScan.results.contextMenu.sendToCrack}
              </ContextMenuItem>
              <ContextMenuSeparator />
              <ContextMenuItem onClick={() => handleCopy(item)}>
                <Copy className="w-4 h-4 mr-2" />
                {t.portScan.results.contextMenu.copy}
              </ContextMenuItem>
            </>
          )}
        </ContextMenuContent>
      </ContextMenu>
    )
  }

  return (
    <div className={`grid gap-4 ${showStatsOverview ? (statsCollapsed ? 'lg:grid-cols-[56px_minmax(0,1fr)]' : 'lg:grid-cols-[260px_minmax(0,1fr)]') : 'lg:grid-cols-1'}`}>
      {showStatsOverview && (
        <div className="hidden space-y-4 md:block">
          <div className="rounded-lg border bg-card/60 p-4">
            <div className="flex items-center justify-between gap-2">
              {!statsCollapsed ? (
                <div className="min-w-0 flex-1">
                  <div className="text-sm font-medium">{t.portScan.results.stats.title}</div>
                </div>
              ) : (
                <div className="flex flex-1 flex-col items-center justify-center gap-2">
                  <span className="text-xs font-medium text-muted-foreground [writing-mode:vertical-rl]">{t.portScan.results.stats.title}</span>
                </div>
              )}
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="h-8 w-8 shrink-0 text-muted-foreground hover:text-foreground"
                onClick={() => setStatsCollapsed((prev) => !prev)}
              >
                {statsCollapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
              </Button>
            </div>

            {!statsCollapsed && (
              <div className="mt-4 space-y-4">
                <div>
                  <div className="mb-2 text-xs font-medium text-muted-foreground">{t.portScan.results.stats.protocolTop5}</div>
                  {renderStatList(stats.protocolTop5)}
                </div>
                <div>
                  <div className="mb-2 text-xs font-medium text-muted-foreground">{t.portScan.results.stats.portTop5}</div>
                  {renderStatList(stats.portTop5.map((item) => ({ label: String(item.port), count: item.count })))}
                </div>
                <div>
                  <div className="mb-2 text-xs font-medium text-muted-foreground">{t.portScan.results.stats.hostOpenPortTop5}</div>
                  {renderStatList(stats.hostOpenPortTop5)}
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      <div className="min-w-0 space-y-4">
        <DataTableWithColumns
          mobileCards
          columns={tableColumns}
          data={paginatedResults}
          loading={tableLoading}
          emptyMessage={searchKeyword ? t.portScan.results.table.noResults : t.portScan.results.table.noData}
          getRowKey={getRowKey}
          rowSelection={rowSelection}
          renderRow={renderRow}
          toolbarLeft={<DetailResultToolbar className="flex w-full items-center gap-4" activeCount={Number(Boolean(searchKeyword.trim()))} onReset={() => { setSearchKeyword(''); setPaginationState({ key: paginationKey, currentPage: 1 }) }} actions={isMobile ? (totalItems > 0 && onExport ? (
              <Button
                variant="outline"
                size="sm"
                onClick={onExport}
              >
                <Download className="w-4 h-4 mr-2" />
                {t.portScan.results.table.exportResults}
              </Button>
            ) : null) : null}>
<div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-muted-foreground" />
              <Input
                placeholder={t.portScan.results.table.searchPlaceholder}
                value={searchKeyword}
                onChange={(e) => {
                  setSearchKeyword(e.target.value)
                  setPaginationState({ key: paginationKey, currentPage: 1 })
                }}
                className="pl-9"
              />
            </div>
</DetailResultToolbar>}
          toolbarRight={!isMobile ? (totalItems > 0 && onExport ? (
              <Button
                variant="outline"
                size="sm"
                onClick={onExport}
              >
                <Download className="w-4 h-4 mr-2" />
                {t.portScan.results.table.exportResults}
              </Button>
            ) : null) : null}
        />

        {/* 分页组件 */}
        <div className="pt-4">
          <PaginationPro
            current={currentPage}
            total={totalItems}
            pageSize={pageSize}
            onChange={(page) => setPaginationState({ key: paginationKey, currentPage: page })}
            onPageSizeChange={(size) => {
              setPageSize(size)
              setPaginationState({ key: paginationKey, currentPage: 1 })
            }}
            showSizeChanger
            showTotal
            pageSizeOptions={[10, 20, 50, 100]}
          />
        </div>
      </div>

      {/* 端口详情弹窗 */}
      <Dialog open={bannerDialogOpen} onOpenChange={setBannerDialogOpen}>
        <DialogContent className="max-w-4xl max-h-[80vh]">
          <DialogHeader>
            <DialogTitle>{t.portScan.results.dialog.bannerTitle}</DialogTitle>
            <DialogDescription>
              {selectedBanner ? `${selectedBanner.host}:${selectedBanner.port}` : ''}
            </DialogDescription>
          </DialogHeader>
          {selectedBanner && (
            <ScrollArea className="max-h-[60vh] pr-4">
              <div className="space-y-4">
                <div>
                  <Label className="text-sm font-semibold">{t.portScan.results.dialog.host}</Label>
                  <p className="mt-1 text-sm font-mono">{selectedBanner.host}</p>
                </div>
                <div>
                  <Label className="text-sm font-semibold">{t.portScan.results.dialog.port}</Label>
                  <p className="mt-1 text-sm">{selectedBanner.port}</p>
                </div>
                <div>
                  <Label className="text-sm font-semibold">{t.portScan.results.dialog.protocol}</Label>
                  <div className="mt-1">
                    {getProtocolBadge(selectedBanner.transport, selectedBanner.protocol)}
                  </div>
                </div>
                <div>
                  <Label className="text-sm font-semibold">{t.portScan.results.dialog.product}</Label>
                  <p className="mt-1 text-sm break-words">{selectedBanner.product || '—'}</p>
                </div>
                <div>
                  <Label className="text-sm font-semibold">{t.portScan.results.dialog.version}</Label>
                  <p className="mt-1 text-sm font-mono break-words">{selectedBanner.version || '—'}</p>
                </div>
                <div>
                  <Label className="text-sm font-semibold">{t.portScan.results.dialog.banner}</Label>
                  <div className="mt-1 h-48 w-full rounded-md border overflow-hidden">
                    <ScrollArea className="h-full w-full p-4">
                      <pre className="text-xs font-mono whitespace-pre-wrap break-all max-w-full">
                        {selectedBanner.banner || selectedBanner.rawResponse || '-'}
                      </pre>
                    </ScrollArea>
                  </div>
                </div>
                {selectedBanner.rawResponse && selectedBanner.rawResponse !== selectedBanner.banner && (
                  <div>
                    <Label className="text-sm font-semibold">{t.portScan.results.dialog.rawResponse}</Label>
                    <div className="mt-1 h-48 w-full rounded-md border overflow-hidden">
                      <ScrollArea className="h-full w-full p-4">
                        <pre className="text-xs font-mono whitespace-pre-wrap break-all max-w-full">
                          {selectedBanner.rawResponse}
                        </pre>
                      </ScrollArea>
                    </div>
                  </div>
                )}
              </div>
            </ScrollArea>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
