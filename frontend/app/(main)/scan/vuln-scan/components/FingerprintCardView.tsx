'use client'

import { useMemo, useState, useEffect, useCallback } from 'react'
import Image from 'next/image'
import { Badge } from '@/components/ui/badge'
import { PaginationPro } from '@/components/ui/pagination-pro'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { getApiBaseURL } from '@/lib/env'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import { fetchWithAuth } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import type { FingerprintInfo } from '../types'
import { ExternalLink, ImageOff, Copy, Check, ListChecks } from 'lucide-react'
import { FingerprintToolbar } from './FingerprintToolbar'
import { ScreenshotPreviewDialog } from './ScreenshotPreviewDialog'
import toast from 'react-hot-toast'
import { AssetTagBadge, normalizeAssetTag } from './AssetTagBadge'
import { buildDisplayFingerprints } from './fingerprint-display'
import { matchesFingerprintStatus } from './fingerprint-status'

interface FingerprintCardViewProps {
  data?: FingerprintInfo[]
  taskId?: string
  version?: number
  currentLayout?: 'table' | 'card'
  onLayoutChange?: (layout: 'table' | 'card') => void
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

export function FingerprintCardView({
  data = [],
  taskId,
  version,
  currentLayout = 'card',
  onLayoutChange,
  onTotalChange,
}: FingerprintCardViewProps) {
  const { t } = useI18n()
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [highRiskFilter, setHighRiskFilter] = useState<string>('all')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)
  const [previewImage, setPreviewImage] = useState<string | null>(null)
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null)
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
      const statusParam = statusFilter !== 'all' ? `&status=${statusFilter}` : ''
      const highRiskParam = highRiskFilter !== 'all' ? `&highRisk=${highRiskFilter}` : ''
      const response = await fetchWithAuth(
        `/api/v1/webscan/task/${taskId}/fingerprints?page=${currentPage}&pageSize=${pageSize}${versionParam}${searchParam}${statusParam}${highRiskParam}`
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

  // Use server data if available, otherwise use client data
  const displayData = useServerPagination ? serverData : data

  const hasHighRiskFingerprints = (item: FingerprintInfo) => {
    const value = item.highRiskFingerprints?.trim()
    return Boolean(value && value !== '[]' && value.toLowerCase() !== 'null')
  }

  const handleCopy = (text: string) => {
    copyToClipboard(text)
    setCopiedUrl(text)
    setTimeout(() => {
      setCopiedUrl((current) => (current === text ? null : current))
    }, 2000)
  }

  // 过滤和搜索 (only for client-side pagination)
  const filteredData = useMemo(() => {
    if (useServerPagination) {
      return displayData
    }

    let filtered = displayData

    if (searchQuery) {
      const q = searchQuery.toLowerCase()
      filtered = filtered.filter((item) =>
        item.url.toLowerCase().includes(q) ||
        item.title?.toLowerCase().includes(q) ||
        item.host.toLowerCase().includes(q) ||
        item.fingerprints?.toLowerCase().includes(q),
      )
    }

    if (statusFilter !== 'all') {
      filtered = filtered.filter((item) => matchesFingerprintStatus(item.status, statusFilter))
    }

    if (highRiskFilter === 'with_high_risk') {
      filtered = filtered.filter((item) => hasHighRiskFingerprints(item))
    } else if (highRiskFilter === 'without_high_risk') {
      filtered = filtered.filter((item) => !hasHighRiskFingerprints(item))
    }

    return filtered
  }, [displayData, searchQuery, statusFilter, highRiskFilter, useServerPagination])

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

  const getFingerprintBadgeClassName = (isHighRisk: boolean) =>
    cn(
      'h-5 px-2 text-[11px] font-normal border rounded-md transition-colors',
      isHighRisk
        ? 'border-amber-500/35 bg-amber-500/12 text-amber-700 hover:bg-amber-500/18 dark:border-amber-500/40 dark:text-amber-300'
        : 'border-slate-500/25 bg-slate-500/8 text-slate-700 hover:bg-slate-500/12 dark:border-slate-500/35 dark:bg-slate-500/10 dark:text-slate-200 dark:hover:bg-slate-500/16'
    )

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

  const getStatusBadge = (status: number) => {
    // 只显示数字状态码，不带文本，使用不同颜色的 outline 样式
    if (status >= 200 && status < 300) {
      // 成功：绿色描边
      return (
        <Badge className="border-green-500 text-green-400 bg-transparent px-2 py-0.5 rounded-md font-medium">
          {status}
        </Badge>
      )
    }
    if (status >= 300 && status < 400) {
      // 重定向：蓝色描边
      return (
        <Badge className="border-sky-500 text-sky-400 bg-transparent px-2 py-0.5 rounded-md font-medium">
          {status}
        </Badge>
      )
    }
    if (status >= 400 && status < 500) {
      // 客户端错误：橙色描边
      return (
        <Badge className="border-amber-500 text-amber-400 bg-transparent px-2 py-0.5 rounded-md font-medium">
          {status}
        </Badge>
      )
    }
    if (status >= 500) {
      // 服务端错误：红色描边
      return (
        <Badge className="border-red-500 text-red-400 bg-transparent px-2 py-0.5 rounded-md font-medium">
          {status}
        </Badge>
      )
    }
    // 其它：灰色描边
    return (
      <Badge className="border-slate-500 text-slate-400 bg-transparent px-2 py-0.5 rounded-md font-medium">
        {status}
      </Badge>
    )
  }

  // 解析资产标签
  const parseAssetTags = (tagsStr?: string) => {
    return normalizeAssetTag(tagsStr)
  }

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

  return (
    <div className="min-w-0 max-w-full space-y-4">
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
        currentLayout={currentLayout}
        onLayoutChange={onLayoutChange}
      />

      {/* 卡片网格 */}
      {paginatedData.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-12 text-muted-foreground">
          <ImageOff className="mb-3 h-10 w-10 opacity-40" />
          <p className="text-sm opacity-80">{loading ? t.common.loading : t.vulnScan.fingerprint.noData}</p>
        </div>
      ) : (
        <div className="grid min-w-0 auto-rows-fr grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5">
          {paginatedData.map((item, idx) => {
            const screenshotUrl = getScreenshotUrl(item.screenshot)
            const allFingerprints = buildDisplayFingerprints(item.fingerprints, item.highRiskFingerprints)
            const assetTags = parseAssetTags(item.assetTags)

            return (
              <div
                key={item.id || `${item.taskId}-${item.url}-${item.port}-${idx}`}
                className="group flex h-full min-w-0 max-w-full flex-col overflow-hidden rounded-xl border bg-card/70 text-card-foreground shadow-sm hover:shadow-md py-0"
              >
                {/* 截图区域 */}
                <div
                  className={cn(
                    'relative w-full h-[200px] bg-muted/40 overflow-hidden',
                    screenshotUrl && 'cursor-zoom-in'
                  )}
                  onClick={() => screenshotUrl && setPreviewImage(screenshotUrl)}
                >
                  {screenshotUrl ? (
                    <>
                      <Image
                        src={screenshotUrl}
                        alt={item.title || item.url}
                        fill
                        sizes="(min-width: 1536px) 20vw, (min-width: 1280px) 25vw, (min-width: 1024px) 33vw, (min-width: 768px) 50vw, 100vw"
                        unoptimized
                        className="h-full w-full object-cover transition-transform group-hover:scale-[1.02]"
                        onError={(e) => {
                          // 图片加载失败时隐藏图片，显示占位图标
                          e.currentTarget.style.display = 'none'
                          const fallback = e.currentTarget.nextElementSibling as HTMLElement | null
                          if (fallback) {
                            fallback.style.display = 'flex'
                          }
                        }}
                      />
                      {/* 加载失败时显示占位图标 */}
                      <div className="absolute inset-0 hidden flex-col items-center justify-center bg-muted/40 text-muted-foreground">
                        <ImageOff className="h-7 w-7 opacity-60" />
                      </div>
                    </>
                  ) : (
                    <div className="screenshot-placeholder flex h-full w-full flex-col items-center justify-center bg-muted/40 text-muted-foreground">
                      <ImageOff className="h-7 w-7 opacity-60" />
                    </div>
                  )}
                </div>

                {/* 信息区域 */}
                <div className="flex min-w-0 flex-1 flex-col justify-between px-3 py-4">
                  <div className="min-w-0 space-y-2">
                    {/* 标题 */}
                    <h3
                      className={cn(
                        'min-w-0 break-all text-sm font-semibold leading-snug line-clamp-1',
                        item.title ? 'text-foreground' : 'text-muted-foreground'
                      )}
                      title={item.title || EMPTY_PLACEHOLDER}
                    >
                      {item.title || EMPTY_PLACEHOLDER}
                    </h3>

                    {/* URL + 复制按钮 */}
                    <div className="flex min-w-0 items-center gap-2 group/url">
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
                      <code className="min-w-0 text-[11px] text-muted-foreground flex-1 whitespace-normal break-all md:truncate font-mono bg-muted/50 px-2 py-1 rounded">
                        {item.url || EMPTY_PLACEHOLDER}
                      </code>
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-11 w-11 shrink-0 md:h-7 md:w-7 text-muted-foreground hover:text-foreground"
                              onClick={(e) => {
                                e.stopPropagation()
                                if (!item.url) return
                                handleCopy(item.url)
                              }}
                              disabled={!item.url}
                            >
                              {copiedUrl === item.url ? (
                                <Check className="h-3.5 w-3.5 text-emerald-500" />
                              ) : (
                                <Copy className="h-3.5 w-3.5" />
                              )}
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>
                            <p>{copiedUrl === item.url ? t.vulnScan.fingerprint.copied : t.vulnScan.fingerprint.copyLink}</p>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </div>

                  {/* 标签区域：指纹 + 资产标签 */}
                  <div className="mt-3 flex flex-wrap items-center gap-1.5">
                    {(() => {
                      // 展示2个指纹，其余用 +x 标签汇总，悬停展示完整列表
                      // 显示前2个指纹
                      const displayFingerprints = allFingerprints.slice(0, 2)
                      // 剩余的指纹
                      const restFingerprints = allFingerprints.slice(2)

                      return (
                        <>
                          {displayFingerprints.map((item, i) => (
                            <Badge
                              key={`fp-display-${idx}-${i}`}
                              variant="outline"
                              className={getFingerprintBadgeClassName(item.isHighRisk)}
                            >
                              {item.fp.length > 14 ? `${item.fp.slice(0, 14)}...` : item.fp}
                            </Badge>
                          ))}

                          {restFingerprints.length > 0 && (
                            <Popover>
                              <PopoverTrigger asChild>
                                <Badge
                                  variant="outline"
                                  className="inline-flex h-5 items-center px-2 text-[11px] font-normal cursor-pointer hover:bg-muted"
                                  onClick={(e) => {
                                    // 阻止触发卡片点击（截图预览）
                                    e.stopPropagation()
                                  }}
                                >
                                  +{restFingerprints.length}
                                </Badge>
                              </PopoverTrigger>
                              <PopoverContent
                                className="w-auto max-w-72 max-h-64 overflow-y-auto p-3"
                                side="top"
                                align="start"
                                onOpenAutoFocus={(e) => e.preventDefault()}
                              >
                                <div className="flex flex-wrap gap-1.5">
                                  {restFingerprints.map((item, i) => (
                                    <Badge
                                      key={`fp-more-${idx}-${i}`}
                                      variant="outline"
                                      className={cn(
                                        'text-xs',
                                        getFingerprintBadgeClassName(item.isHighRisk)
                                      )}
                                      title={item.fp}
                                    >
                                      {item.fp.length > 20 ? `${item.fp.slice(0, 20)}...` : item.fp}
                                    </Badge>
                                  ))}
                                </div>
                              </PopoverContent>
                            </Popover>
                          )}
                        </>
                      )
                    })()}

                    {assetTags && (
                      <TooltipProvider delayDuration={0}>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <AssetTagBadge
                              label={
                                (assetTags.assetType || t.vulnScan.fingerprint.tagLabel).length > 12
                                  ? `${(assetTags.assetType || t.vulnScan.fingerprint.tagLabel).slice(0, 12)}...`
                                  : assetTags.assetType || t.vulnScan.fingerprint.tagLabel
                              }
                              assetType={assetTags.assetType}
                              className="h-5 px-2 text-[11px] font-normal"
                            />
                          </TooltipTrigger>
                          <TooltipContent>
                            <div className="text-xs">
                              {t.vulnScan.fingerprint.name}: {assetTags.productName || t.vulnScan.fingerprint.unknown}｜{t.vulnScan.fingerprint.source}: {assetTags.source || EMPTY_PLACEHOLDER}
                            </div>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    )}
                  </div>

                  {/* 底部：状态 + 操作 */}
                  <div className="mt-3 flex flex-wrap items-center justify-between gap-3 pt-3 border-t border-border">
                    <div className="flex flex-wrap items-center gap-2">
                      {getStatusBadge(item.status)}
                      {getVerificationStatusBadge(getCurrentVerificationStatus(item, getRowKey(item, idx)))}
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-11 w-11 shrink-0 md:h-8 md:w-8"
                            disabled={updatingVerificationRows[getRowKey(item, idx)]}
                            onClick={(e) => e.stopPropagation()}
                          >
                            <ListChecks className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-40" onClick={(e) => e.stopPropagation()}>
                          {FINGERPRINT_VERIFICATION_STATUS_OPTIONS.map((status) => (
                            <DropdownMenuItem
                              key={status}
                              onClick={() => updateVerificationStatus(item, idx, status)}
                            >
                              {getVerificationStatusLabel(status)}
                            </DropdownMenuItem>
                          ))}
                        </DropdownMenuContent>
                      </DropdownMenu>
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-11 w-11 shrink-0 md:h-8 md:w-8"
                              onClick={(e) => {
                                e.stopPropagation()
                                if (!item.url) return
                                window.open(item.url, '_blank', 'noopener,noreferrer')
                              }}
                              disabled={!item.url}
                            >
                              <ExternalLink className="h-4 w-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>
                            <p>{t.vulnScan.fingerprint.visitWebsite}</p>
                          </TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </div>
                </div>
              </div>
            )
          })}
        </div>
      )}

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

      <ScreenshotPreviewDialog image={previewImage} onOpenChange={(open) => !open && setPreviewImage(null)} />
    </div>
  )
}
