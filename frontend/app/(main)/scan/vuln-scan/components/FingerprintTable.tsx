'use client'

import { useState, useMemo, useEffect, useCallback } from 'react'
import Image from 'next/image'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { StatusBadge } from '@/lib/status-badge'
import { Button } from '@/components/ui/button'
import { FingerprintToolbar } from './FingerprintToolbar'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { ArrowDown, ArrowUp, ArrowUpDown, ExternalLink, ImageOff, ListChecks } from 'lucide-react'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { getApiBaseURL } from '@/lib/env'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import { fetchWithAuth } from '@/lib/api'
import type { FingerprintInfo } from '../types'
import toast from 'react-hot-toast'
import { ScreenshotPreviewDialog } from './ScreenshotPreviewDialog'
import { AssetTagBadge, normalizeAssetTag } from './AssetTagBadge'
import { buildDisplayFingerprints } from './fingerprint-display'
import { matchesFingerprintStatus } from './fingerprint-status'

interface FingerprintTableProps {
  data?: FingerprintInfo[]
  taskId?: string
  version?: number
  onViewDetail?: (item: FingerprintInfo) => void
  onLayoutChange?: (layout: 'table' | 'card') => void
  currentLayout?: 'table' | 'card'
  onTotalChange?: (total: number) => void
}

const EMPTY_PLACEHOLDER = '-'
type FingerprintVerificationStatus = 'pending' | 'verified' | 'false_positive' | 'ignored'
const FINGERPRINT_VERIFICATION_STATUS_OPTIONS: FingerprintVerificationStatus[] = ['pending', 'verified', 'false_positive', 'ignored']
type FingerprintVerificationState = {
  key: string
  statusOverrides: Record<string, FingerprintVerificationStatus>
  updatingRows: Record<string, boolean>
}
const EMPTY_VERIFICATION_STATE: FingerprintVerificationState = {
  key: '',
  statusOverrides: {},
  updatingRows: {},
}

const normalizeAssetFilename = (value: string) => {
  const normalized = value
    .replace(/\\/g, '/')
    .replace(/^data\/screenshot\//, '')
    .replace(/^data\/favicon\//, '')
    .replace(/^screenshot\//, '')
    .replace(/^favicon\//, '')
    .replace(/^\/+/, '')
  const segments = normalized.split('/').filter(Boolean)
  return segments.length > 0 ? segments[segments.length - 1] : ''
}

export function FingerprintTable({
  data = [],
  taskId,
  version,
  onViewDetail,
  onLayoutChange,
  currentLayout,
  onTotalChange,
}: FingerprintTableProps) {
  const { t } = useI18n()
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [highRiskFilter, setHighRiskFilter] = useState<string>('all')
  const [lengthSortOrder, setLengthSortOrder] = useState<'asc' | 'desc' | null>(null)
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [previewImage, setPreviewImage] = useState<string | null>(null)
  const [imageErrors, setImageErrors] = useState<Set<string>>(new Set())
  const [faviconErrors, setFaviconErrors] = useState<Set<string>>(new Set())
  const [serverData, setServerData] = useState<FingerprintInfo[]>([])
  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(false)
  const [verificationState, setVerificationState] = useState<FingerprintVerificationState>(EMPTY_VERIFICATION_STATE)
  const verificationStateKey = `${taskId || 'client'}:${version ?? 'latest'}`
  const activeVerificationState = verificationState.key === verificationStateKey ? verificationState : EMPTY_VERIFICATION_STATE
  const verificationStatusOverrides = activeVerificationState.statusOverrides
  const updatingVerificationRows = activeVerificationState.updatingRows

  // Determine if using server-side pagination
  const useServerPagination = !!taskId

  // Fetch data from server when using server-side pagination
  const fetchServerData = useCallback(async () => {
    if (!taskId) return

    setLoading(true)
    try {
      const versionParam = version ? `&version=${version}` : ''
      const searchParam = searchQuery ? `&search=${encodeURIComponent(searchQuery)}` : ''
      const statusParam = statusFilter.length > 0 ? `&status=${statusFilter.join(',')}` : ''
      const highRiskParam = highRiskFilter !== 'all' ? `&highRisk=${highRiskFilter}` : ''
      const sortParam = lengthSortOrder ? `&sortBy=length&sortOrder=${lengthSortOrder}` : ''
      const response = await fetchWithAuth(
        `/api/v1/webscan/task/${taskId}/fingerprints?page=${currentPage}&pageSize=${pageSize}${versionParam}${searchParam}${statusParam}${highRiskParam}${sortParam}`
      )
      if (response.ok) {
        const result = await response.json()
        if (result.success && result.data) {
          setServerData(result.data.fingerprints || [])
          setTotalCount(result.data.total || 0)
          onTotalChange?.(result.data.total || 0)
        }
      }
    } catch (error) {
      console.error('Failed to fetch fingerprints:', error)
    } finally {
      setLoading(false)
    }
  }, [
    currentPage,
    highRiskFilter,
    lengthSortOrder,
    onTotalChange,
    pageSize,
    searchQuery,
    statusFilter,
    taskId,
    version,
  ])

  // Fetch data when page changes (server-side pagination only)
  useEffect(() => {
    if (useServerPagination) {
      // 使用防抖，避免频繁请求
      const timer = setTimeout(() => {
        fetchServerData()
      }, 300)
      return () => clearTimeout(timer)
    }
  }, [fetchServerData, useServerPagination])

  // 获取截图 URL
  const getScreenshotUrl = (screenshot?: string) => {
    if (!screenshot) return null
    if (screenshot.startsWith('data:image')) return screenshot
    if (screenshot.startsWith('http://') || screenshot.startsWith('https://')) return screenshot
    // 使用接口方式访问截图（支持后续扩展对象存储）
    const backend = getApiBaseURL()
    const filename = normalizeAssetFilename(screenshot)
    if (!filename) return null
    return `${backend}/api/v1/webscan/screenshot/${filename}`
  }

  // 获取favicon URL
  const getFaviconUrl = (favicon?: string) => {
    if (!favicon) return null
    if (favicon.startsWith('data:image')) return favicon
    if (favicon.startsWith('http://') || favicon.startsWith('https://')) return favicon
    // 使用接口方式访问favicon
    const backend = getApiBaseURL()
    const filename = normalizeAssetFilename(favicon)
    if (!filename) return null
    return `${backend}/api/v1/webscan/favicon/${filename}`
  }

  // Use server data if available, otherwise use client data
  const displayData = useServerPagination ? serverData : data

  const hasHighRiskFingerprints = (item: FingerprintInfo) => {
    const value = item.highRiskFingerprints?.trim()
    return Boolean(value && value !== '[]' && value.toLowerCase() !== 'null')
  }

  // 过滤和搜索 (only for client-side pagination)
  const filteredData = useMemo(() => {
    if (useServerPagination) {
      return displayData
    }

    let filtered = displayData

    // 搜索过滤
    if (searchQuery) {
      const query = searchQuery.toLowerCase()
      filtered = filtered.filter(
        (item) =>
          item.url.toLowerCase().includes(query) ||
          item.title?.toLowerCase().includes(query) ||
          item.host.toLowerCase().includes(query) ||
          item.fingerprints?.toLowerCase().includes(query)
      )
    }

    // 状态过滤
    if (statusFilter.length > 0) {
      filtered = filtered.filter((item) => statusFilter.some((filter) => matchesFingerprintStatus(item.status, filter)))
    }

    if (highRiskFilter === 'with_high_risk') {
      filtered = filtered.filter((item) => hasHighRiskFingerprints(item))
    } else if (highRiskFilter === 'without_high_risk') {
      filtered = filtered.filter((item) => !hasHighRiskFingerprints(item))
    }

    if (lengthSortOrder) {
      return [...filtered].sort((a, b) => (
        lengthSortOrder === 'asc' ? a.length - b.length : b.length - a.length
      ))
    }

    return filtered
  }, [displayData, searchQuery, statusFilter, highRiskFilter, lengthSortOrder, useServerPagination])

  // 分页 (only for client-side pagination)
  const paginatedData = useMemo(() => {
    if (useServerPagination) {
      return filteredData
    }
    const start = (currentPage - 1) * pageSize
    return filteredData.slice(start, start + pageSize)
  }, [filteredData, currentPage, pageSize, useServerPagination])

  const totalItems = useServerPagination ? totalCount : filteredData.length

  useEffect(() => {
    if (!useServerPagination) {
      onTotalChange?.(filteredData.length)
    }
  }, [filteredData.length, onTotalChange, useServerPagination])

  const getStatusBadge = (status: number) => <StatusBadge status={status} />

  const handleLengthSort = () => {
    setLengthSortOrder((current) => current === null ? 'asc' : current === 'asc' ? 'desc' : null)
    setCurrentPage(1)
  }

  const getLengthSortIcon = () => {
    if (lengthSortOrder === 'asc') {
      return <ArrowUp className="ml-1 h-3 w-3" />
    }
    if (lengthSortOrder === 'desc') {
      return <ArrowDown className="ml-1 h-3 w-3" />
    }
    return <ArrowUpDown className="ml-1 h-3 w-3 opacity-40" />
  }

  const getFingerprintBadgeClassName = (isHighRisk: boolean) =>
    cn(
      'border px-2 py-0.5 text-[11px] font-normal rounded-md transition-colors',
      isHighRisk
        ? 'border-amber-500/35 bg-amber-500/12 text-amber-700 hover:bg-amber-500/18 dark:border-amber-500/40 dark:text-amber-300'
        : 'border-slate-500/25 bg-slate-500/8 text-slate-700 hover:bg-slate-500/12 dark:border-slate-500/35 dark:bg-slate-500/10 dark:text-slate-200 dark:hover:bg-slate-500/16'
    )

  const getRowKey = (item: FingerprintInfo, index: number) => (
    item.id || `${item.taskId}-${item.version ?? version ?? 0}-${item.url}-${item.port}-${index}`
  )

  const getVerificationStatusLabel = (status: FingerprintVerificationStatus) => {
    switch (status) {
      case 'verified':
        return t.vulnScan.fingerprint.statusVerified
      case 'false_positive':
        return t.vulnScan.fingerprint.statusFalsePositive
      case 'ignored':
        return t.vulnScan.fingerprint.statusIgnored
      default:
        return t.vulnScan.fingerprint.statusPending
    }
  }

  const getVerificationStatusBadge = (status: FingerprintVerificationStatus) => {
    switch (status) {
      case 'verified':
        return <Badge className="bg-emerald-500/15 text-emerald-400 border-emerald-500/30">{getVerificationStatusLabel(status)}</Badge>
      case 'false_positive':
        return <Badge className="bg-amber-500/15 text-amber-400 border-amber-500/30">{getVerificationStatusLabel(status)}</Badge>
      case 'ignored':
        return <Badge className="bg-zinc-500/20 text-zinc-300 border-zinc-500/40">{getVerificationStatusLabel(status)}</Badge>
      default:
        return <Badge variant="outline">{getVerificationStatusLabel(status)}</Badge>
    }
  }

  const getCurrentVerificationStatus = (item: FingerprintInfo, rowKey: string): FingerprintVerificationStatus => {
    return verificationStatusOverrides[rowKey] || item.verificationStatus || 'pending'
  }

  const updateVerificationStatus = async (item: FingerprintInfo, index: number, nextStatus: FingerprintVerificationStatus) => {
    const rowKey = getRowKey(item, index)
    const currentStatus = getCurrentVerificationStatus(item, rowKey)
    if (currentStatus === nextStatus) {
      return
    }

    setVerificationState((prev) => {
      const base = prev.key === verificationStateKey
        ? prev
        : { key: verificationStateKey, statusOverrides: {}, updatingRows: {} }
      return {
        ...base,
        updatingRows: { ...base.updatingRows, [rowKey]: true },
      }
    })
    try {
      const response = await fetchWithAuth('/api/v1/webscan/fingerprint/verification-status', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          taskId: item.taskId,
          version: item.version ?? version,
          url: item.url,
          verificationStatus: nextStatus,
        }),
      })

      const result = await response.json().catch(() => ({}))
      if (!response.ok || !result.success) {
        throw new Error(result.error || `HTTP ${response.status}`)
      }

      setVerificationState((prev) => {
        if (prev.key !== verificationStateKey) {
          return prev
        }
        return {
          ...prev,
          statusOverrides: { ...prev.statusOverrides, [rowKey]: nextStatus },
        }
      })
      setServerData((prev) =>
        prev.map((fingerprint, i) => (
          getRowKey(fingerprint, i) === rowKey
            ? { ...fingerprint, verificationStatus: nextStatus }
            : fingerprint
        ))
      )
      toast.success(t.vulnScan.messages.fingerprintStatusUpdated)
    } catch (error: any) {
      console.error('Update fingerprint verification status failed:', error)
      toast.error(error?.message || t.vulnScan.messages.updateFingerprintStatusFailed)
    } finally {
      setVerificationState((prev) => {
        if (prev.key !== verificationStateKey) {
          return prev
        }
        const next = { ...prev.updatingRows }
        delete next[rowKey]
        return { ...prev, updatingRows: next }
      })
    }
  }

  return (
    <div className="space-y-4">
      <FingerprintToolbar
        searchQuery={searchQuery}
        onSearchChange={(value) => {
          setSearchQuery(value)
          setCurrentPage(1)
        }}
        statusFilter={statusFilter}
        onStatusFilterChange={(value) => {
          setStatusFilter(value)
          setCurrentPage(1)
        }}
        highRiskFilter={highRiskFilter}
        onHighRiskFilterChange={(value) => {
          setHighRiskFilter(value)
          setCurrentPage(1)
        }}
        taskId={taskId}
        version={version}
        currentLayout={currentLayout || 'table'}
        onLayoutChange={onLayoutChange}
      />

      {/* 表格 */}
      <div className="border rounded-lg overflow-hidden">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[300px]">{t.vulnScan.fingerprint.url}</TableHead>
              <TableHead className="w-[100px]">{t.vulnScan.fingerprint.status}</TableHead>
              <TableHead className="w-[200px]">{t.vulnScan.fingerprint.title}</TableHead>
              <TableHead className="w-[200px]">{t.vulnScan.fingerprint.fingerprints}</TableHead>
              <TableHead className="w-[100px]">
                <Button
                  variant="ghost"
                  className="-ml-3 h-8 px-3 font-medium hover:bg-muted/60"
                  onClick={handleLengthSort}
                  aria-label={`${t.vulnScan.fingerprint.length}${lengthSortOrder === 'asc' ? '，升序' : lengthSortOrder === 'desc' ? '，降序' : '，未排序'}`}
                >
                  {t.vulnScan.fingerprint.length}
                  {getLengthSortIcon()}
                </Button>
              </TableHead>
              <TableHead className="w-[150px]">{t.vulnScan.fingerprint.assetTags}</TableHead>
              <TableHead className="w-[120px] text-center">{t.vulnScan.fingerprint.verificationStatus}</TableHead>
              <TableHead className="w-[120px]">{t.vulnScan.fingerprint.screenshot}</TableHead>
              <TableHead className="w-[96px] text-center">{t.vulnScan.vulnTable.action}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {paginatedData.length === 0 ? (
              <TableRow>
                <TableCell colSpan={9} className="text-center py-8 text-muted-foreground">
                  {loading ? t.common.loading : t.vulnScan.fingerprint.noData}
                </TableCell>
              </TableRow>
            ) : (
              paginatedData.map((item, index) => (
                <TableRow key={getRowKey(item, index)}>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      {/* Favicon 图标 - 仅在有图标时显示 */}
                      {item.favicon && !faviconErrors.has(item.favicon) && (
                        <Image
                          src={getFaviconUrl(item.favicon) || ''}
                          alt=""
                          width={16}
                          height={16}
                          unoptimized
                          className="w-4 h-4 flex-shrink-0 rounded-sm object-contain"
                          onError={() => {
                            if (item.favicon) {
                              setFaviconErrors((prev) => new Set(prev).add(item.favicon || ''))
                            }
                          }}
                        />
                      )}
                      {item.url ? (
                        <>
                          <a
                            href={item.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-primary hover:underline truncate max-w-[280px]"
                          >
                            {item.url}
                          </a>
                          <ExternalLink className="w-3 h-3 text-muted-foreground flex-shrink-0" />
                        </>
                      ) : (
                        <span className="text-muted-foreground">{EMPTY_PLACEHOLDER}</span>
                      )}
                    </div>
                  </TableCell>
                  <TableCell>{getStatusBadge(item.status)}</TableCell>
                  <TableCell className="truncate max-w-[200px]" title={item.title}>
                    {item.title || EMPTY_PLACEHOLDER}
                  </TableCell>
                  <TableCell>
                    {(() => {
                      const allFingerprints = buildDisplayFingerprints(item.fingerprints, item.highRiskFingerprints)

                      if (allFingerprints.length === 0) {
                        return <span className="text-muted-foreground">{EMPTY_PLACEHOLDER}</span>
                      }

                      return (
                        <div className="flex flex-wrap gap-1">
                          {allFingerprints.map(({ fp, isHighRisk }, idx) => (
                            <Badge
                              key={`${item.id || item.taskId}-fp-${idx}-${fp}`}
                              variant="outline"
                              className={getFingerprintBadgeClassName(isHighRisk)}
                            >
                              {fp}
                            </Badge>
                          ))}
                        </div>
                      )
                    })()}
                  </TableCell>
                  <TableCell>{item.length ?? EMPTY_PLACEHOLDER}</TableCell>
                  <TableCell>
                    {(() => {
                      const normalized = normalizeAssetTag(item.assetTags)
                      if (!normalized) {
                        return <span className="text-muted-foreground">{EMPTY_PLACEHOLDER}</span>
                      }

                      try {
                        const typeLabel = normalized.assetType || t.vulnScan.fingerprint.tagLabel
                        const nameLabel = normalized.productName || t.vulnScan.fingerprint.unknown
                        const sourceLabel = normalized.source || EMPTY_PLACEHOLDER

                        return (
                          <TooltipProvider delayDuration={0}>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <AssetTagBadge label={typeLabel} assetType={normalized.assetType} />
                              </TooltipTrigger>
                              <TooltipContent>
                                <div className="text-xs">
                                  {t.vulnScan.fingerprint.name}: {nameLabel}｜{t.vulnScan.fingerprint.source}: {sourceLabel}
                                </div>
                              </TooltipContent>
                            </Tooltip>
                          </TooltipProvider>
                        )
                      } catch {
                        return <span className="text-muted-foreground text-xs">{item.assetTags}</span>
                      }
                    })()}
                  </TableCell>
                  <TableCell className="text-center">
                    {getVerificationStatusBadge(getCurrentVerificationStatus(item, getRowKey(item, index)))}
                  </TableCell>
                  <TableCell className="align-middle">
                    <div className="flex items-center justify-center h-12 w-16">
                      {item.screenshot && !imageErrors.has(item.screenshot || '') ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          className="h-12 w-16 p-0 hover:bg-muted/50"
                          onClick={() => {
                            const screenshotUrl = getScreenshotUrl(item.screenshot)
                            if (screenshotUrl) {
                              setPreviewImage(screenshotUrl)
                            }
                          }}
                        >
                          <Image
                            src={getScreenshotUrl(item.screenshot) || ''}
                            alt={t.vulnScan.fingerprint.screenshot}
                            width={64}
                            height={48}
                            unoptimized
                            className="h-12 w-16 object-cover rounded cursor-pointer hover:opacity-80 transition-opacity border border-border"
                            onError={() => {
                              if (item.screenshot) {
                                setImageErrors((prev) => new Set(prev).add(item.screenshot || ''))
                              }
                            }}
                          />
                        </Button>
                      ) : (
                        <div className="flex items-center justify-center h-12 w-16 bg-muted rounded border border-border">
                          <ImageOff className="w-6 h-6 text-muted-foreground" />
                        </div>
                      )}
                    </div>
                  </TableCell>
                  <TableCell className="text-center align-middle">
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          disabled={updatingVerificationRows[getRowKey(item, index)]}
                        >
                          <ListChecks className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-40">
                        {FINGERPRINT_VERIFICATION_STATUS_OPTIONS.map((status) => (
                          <DropdownMenuItem
                            key={status}
                            onClick={() => updateVerificationStatus(item, index, status)}
                          >
                            {getVerificationStatusLabel(status)}
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <ScreenshotPreviewDialog image={previewImage} onOpenChange={(open) => !open && setPreviewImage(null)} />

      {/* 分页 */}
      {totalItems > 0 && (
        <div className="pt-4 border-t border-border">
          <PaginationPro
            current={currentPage}
            total={totalItems}
            pageSize={pageSize}
            onChange={(page, size) => {
              setCurrentPage(page)
              if (size !== pageSize) {
                setPageSize(size)
              }
            }}
            onPageSizeChange={(size) => {
              setPageSize(size)
              setCurrentPage(1)
            }}
            pageSizeOptions={[10, 20, 50, 100]}
            showSizeChanger
            showQuickJumper
            showTotal
          />
        </div>
      )}
    </div>
  )
}
