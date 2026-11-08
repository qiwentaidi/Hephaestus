'use client'

import { useCallback, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Fingerprint, Shield, Network, Eye, ArrowLeft, Globe, Server, TrendingUp, Layers, FolderTree, GitBranch, FileCode2, type LucideIcon } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import { cn } from '@/lib/utils'
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip as RechartsTooltip,
  Legend,
  ResponsiveContainer,
} from 'recharts'
import type { Props as LegendContentProps } from 'recharts/types/component/DefaultLegendContent'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { WebScanTask } from '../types'
import { ScanType } from '../types'
import { useI18n } from '@/lib/i18n'
import { getStatusBadge } from '@/lib/status-badge'

interface ScanVersion {
  version: number
  status: string
  progress: number
  createdAt: string
  completedAt?: string
  fingerprintCount: number
  portCount: number
  dirResultCount?: number
  subdomainCount?: number
  vulnCount: number
  webVulnCount?: number
  jsVulnCount?: number
  criticalCount: number
  highCount: number
  mediumCount: number
  lowCount: number
  infoCount: number
  resultCount?: number
}

interface VersionHistoryResponse {
  taskId: string
  taskName: string
  scanType: string
  versions: ScanVersion[]
}

interface HistoryMetric {
  key: string
  label: string
  value: number
  icon: LucideIcon
  stroke: string
  fill: string
}

const metricMeta: Record<string, Omit<HistoryMetric, 'value'>> = {
  fingerprint: {
    key: 'fingerprint',
    label: '指纹',
    icon: Fingerprint,
    stroke: '#3b82f6',
    fill: 'url(#historyTrendBlue)',
  },
  port: {
    key: 'port',
    label: '端口',
    icon: Network,
    stroke: '#22c55e',
    fill: 'url(#historyTrendGreen)',
  },
  directory: {
    key: 'directory',
    label: '目录',
    icon: FolderTree,
    stroke: '#06b6d4',
    fill: 'url(#historyTrendCyan)',
  },
  subdomain: {
    key: 'subdomain',
    label: '子域名',
    icon: GitBranch,
    stroke: '#14b8a6',
    fill: 'url(#historyTrendTeal)',
  },
  webVulnerability: {
    key: 'webVulnerability',
    label: 'Web 漏洞',
    icon: Shield,
    stroke: '#f97316',
    fill: 'url(#historyTrendOrange)',
  },
  jsVulnerability: {
    key: 'jsVulnerability',
    label: 'JS 漏洞',
    icon: FileCode2,
    stroke: '#f97316',
    fill: 'url(#historyTrendOrange)',
  },
  vulnerability: {
    key: 'vulnerability',
    label: '漏洞',
    icon: Shield,
    stroke: '#f97316',
    fill: 'url(#historyTrendOrange)',
  },
}

const getMetricMeta = (key: string, label?: string): Omit<HistoryMetric, 'value'> => {
  return metricMeta[key] || {
    key,
    label: label || key,
    icon: Layers,
    stroke: '#8b5cf6',
    fill: 'url(#historyTrendPurple)',
  }
}

export default function ScanHistoryClient() {
  const { t } = useI18n()
  const router = useRouter()
  const searchParams = useSearchParams()
  const taskId = searchParams?.get('taskId') || ''

  const [versions, setVersions] = useState<ScanVersion[]>([])
  const [loading, setLoading] = useState(false)
  const [taskName, setTaskName] = useState<string>('')
  const [tasks, setTasks] = useState<WebScanTask[]>([])
  const [currentTask, setCurrentTask] = useState<WebScanTask | null>(null)
  const [hiddenTrendMetricKeys, setHiddenTrendMetricKeys] = useState<Set<string>>(() => new Set())
  const taskStatusLabels = {
    pending: t.vulnScan.statuses.pending,
    running: t.vulnScan.statuses.running,
    paused: t.vulnScan.statuses.paused,
    recovering: t.vulnScan.statuses.recovering,
    completed: t.vulnScan.statuses.completed,
    failed: t.vulnScan.statuses.failed,
    cancelled: t.vulnScan.statuses.cancelled,
  }

  // 加载任务列表（用于任务选择器）
  useEffect(() => {
    const loadTasks = async () => {
      try {
        const response = await fetchWithAuth(`/api/v1/webscan/tasks?limit=50`)
        if (response.ok) {
          const data = await response.json()
          if (data.success && data.data) {
            setTasks(data.data.tasks || [])
          }
        }
      } catch (error) {
        console.error('Failed to load tasks:', error)
      }
    }

    loadTasks()
  }, [])

  const fetchScanHistory = useCallback(async () => {
    if (!taskId) return
    // 使用新的一次性获取所有版本历史的接口
    const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/versions/history`)
    if (!response.ok) throw new Error('获取任务版本历史失败')
    const result = await response.json()
    if (!result.success || !result.data) return null
    return result.data as VersionHistoryResponse
  }, [taskId])

  const applyScanHistory = useCallback((data: VersionHistoryResponse) => {
    setTaskName(data.taskName || data.taskId)
    setVersions(data.versions || [])

    // 设置当前任务信息
    setCurrentTask({
      id: data.taskId,
      taskName: data.taskName,
      scanType: data.scanType as any,
      status: data.versions.length > 0 ? data.versions[data.versions.length - 1].status : 'unknown',
      version: data.versions.length,
    } as WebScanTask)
  }, [])

  const loadScanHistory = useCallback(async () => {
    if (!taskId) return
    setLoading(true)
    try {
      const data = await fetchScanHistory()
      if (data) {
        applyScanHistory(data)
      }
    } catch (error) {
      console.error('加载扫描历史失败:', error)
    } finally {
      setLoading(false)
    }
  }, [applyScanHistory, fetchScanHistory, taskId])

  useEffect(() => {
    if (!taskId) {
      return
    }

    let cancelled = false
    const loadInitialScanHistory = async () => {
      setLoading(true)
      try {
        const data = await fetchScanHistory()
        if (cancelled || !data) return
        applyScanHistory(data)
      } catch (error) {
        if (!cancelled) {
          console.error('加载扫描历史失败:', error)
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    void loadInitialScanHistory()

    return () => {
      cancelled = true
    }
  }, [applyScanHistory, fetchScanHistory, taskId])

  const handleViewVersionDetails = (version: number) => {
    // 直接跳转到任务详情页面，带上version参数
    const params = new URLSearchParams()
    params.set('taskId', taskId)
    params.set('version', String(version))
    router.push(`/scan/vuln-scan/detail?${params.toString()}`)
  }

  const handleTaskChange = (newTaskId: string) => {
    // 切换任务时跳转到新任务的历史页面
    const params = new URLSearchParams()
    params.set('taskId', newTaskId)
    router.push(`/scan/vuln-scan/history?${params.toString()}`)
  }

  const toggleTrendMetric = useCallback((metricKey: string) => {
    setHiddenTrendMetricKeys((current) => {
      const next = new Set(current)
      if (next.has(metricKey)) {
        next.delete(metricKey)
      } else {
        next.add(metricKey)
      }
      return next
    })
  }, [])

  const renderTrendLegend = useCallback(({ payload }: LegendContentProps) => {
    if (!payload?.length) return null

    return (
      <div className="flex items-center justify-center gap-4 pt-3 text-xs">
        {payload.map((item) => {
          const metricKey = typeof item.dataKey === 'string' ? item.dataKey : ''
          const isHidden = metricKey ? hiddenTrendMetricKeys.has(metricKey) : false
          const label = item.value || metricKey

          return (
            <button
              key={String(item.dataKey || item.value)}
              type="button"
              className={cn(
                'inline-flex h-6 items-center gap-1.5 rounded-sm px-1.5 transition-opacity hover:opacity-80 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                isHidden && 'opacity-45',
              )}
              aria-pressed={!isHidden}
              aria-label={`${label}${isHidden ? '，当前已隐藏，点击显示' : '，当前已显示，点击隐藏'}`}
              onClick={() => {
                if (metricKey) toggleTrendMetric(metricKey)
              }}
            >
              <span
                className={cn('h-0 w-3 shrink-0 rounded-full border-t-2', isHidden && 'border-dashed')}
                style={{ borderColor: item.color }}
              />
              <span className={cn(isHidden && 'line-through')}>{label}</span>
            </button>
          )
        })}
      </div>
    )
  }, [hiddenTrendMetricKeys, toggleTrendMetric])

  // 获取可切换的任务列表（按创建时间倒序）
  const availableTasks = tasks
    .filter((t) => t.id !== taskId)
    .sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime())

  const getStatusText = (status: string) => {
    switch (status) {
      case 'completed':
        return t.vulnScan.statuses.completed
      case 'failed':
        return t.vulnScan.statuses.failed
      case 'running':
        return t.vulnScan.statuses.running
      case 'paused':
        return t.vulnScan.statuses.paused
      case 'recovering':
        return t.vulnScan.statuses.recovering
      case 'cancelled':
        return t.vulnScan.statuses.cancelled
      case 'pending':
        return t.vulnScan.statuses.pending
      default:
        return status
    }
  }

  // 格式化日期函数
  const formatDate = (dateString: string) => {
    if (!dateString) return '-'
    try {
      const date = new Date(dateString)
      return date.toLocaleString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
      })
    } catch {
      return dateString
    }
  }

  // 判断任务类型，用于决定显示哪些趋势指标
  const scanType = currentTask?.scanType
  const isCrackTask = scanType === ScanType.Crack
  const isPortScanTask = scanType === ScanType.PortScan
  const isWebScanTask = scanType === ScanType.WebScan
  const isJsScanTask = scanType === ScanType.JsScan
  const isDirScanTask = scanType === ScanType.DirScan
  const isSubdomainTask = scanType === ScanType.Subdomain
  const isCombinationTask = scanType === ScanType.Combination

  const getMetricValue = (version: ScanVersion, key: string) => {
    switch (key) {
      case 'fingerprint':
        return version.fingerprintCount || 0
      case 'port':
        return version.portCount || 0
      case 'directory':
        return version.dirResultCount || 0
      case 'subdomain':
        return version.subdomainCount || 0
      case 'webVulnerability':
        return version.webVulnCount ?? version.vulnCount ?? 0
      case 'jsVulnerability':
        return version.jsVulnCount || 0
      case 'vulnerability':
        return version.vulnCount || 0
      default:
        return 0
    }
  }

  const getVersionMetrics = (version: ScanVersion): HistoryMetric[] => {
    const metricKeys: string[] = []
    if (isCombinationTask) {
      metricKeys.push('fingerprint', 'port', 'directory', 'subdomain', 'webVulnerability', 'jsVulnerability')
    } else {
      if (isWebScanTask) metricKeys.push('fingerprint', 'webVulnerability')
      if (isPortScanTask) metricKeys.push('port')
      if (isCrackTask) metricKeys.push('port', 'webVulnerability')
      if (isDirScanTask) metricKeys.push('directory')
      if (isSubdomainTask) metricKeys.push('subdomain')
      if (isJsScanTask) metricKeys.push('jsVulnerability')
    }
    if (metricKeys.length === 0) metricKeys.push('fingerprint', 'port', 'vulnerability')

    return metricKeys.map((key) => ({
      ...getMetricMeta(key),
      value: getMetricValue(version, key),
    }))
  }

  const sortedVersions = [...versions].sort((a, b) => a.version - b.version)
  const latestVersion = sortedVersions[sortedVersions.length - 1]
  const trendMetrics = latestVersion ? getVersionMetrics(latestVersion) : []
  const trendData = sortedVersions.map((version) => {
    const row: Record<string, string | number> = {
      version: t.vulnScan.history.scanNumber.replace('{n}', String(version.version)),
    }
    trendMetrics.forEach((metric) => {
      row[metric.key] = getMetricValue(version, metric.key)
    })
    return row
  })

  if (!taskId) {
    return <div className="p-6">{t.vulnScan.taskDetail.invalidTaskId}</div>
  }

  return (
    <div className="flex h-full min-w-0 flex-col">
      {/* 顶部栏 - 页头操作区 */}
      <div className="border-b border-border bg-card p-4">
        <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex items-center gap-4">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => router.push('/scan/vuln-scan')}
            >
              <ArrowLeft className="w-4 h-4 mr-2" />
              {t.vulnScan.backToList}
            </Button>
          </div>
          <div className="flex min-w-0 items-center gap-3">
            {/* 任务选择器 - 始终显示 */}
            <Select
              value={taskId || ''}
              onValueChange={(value) => {
                handleTaskChange(value)
              }}
            >
              <SelectTrigger className="w-full min-w-0 sm:w-[300px] [&>span]:min-w-0 [&>span]:truncate">
                <SelectValue>
                  {currentTask?.taskName || taskName || taskId}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={taskId || ''} disabled>
                  {t.vulnScan.taskDetail.currentTask}: {currentTask?.taskName || taskName || taskId}
                </SelectItem>
                {availableTasks.map((task) => (
                  <SelectItem key={task.id} value={task.id}>
                    <div className="flex items-center gap-2">
                      {task.scanType === ScanType.Combination ? (
                        <Layers className="w-3 h-3" />
                      ) : task.scanType === ScanType.WebScan ? (
                        <Globe className="w-3 h-3" />
                      ) : (
                        <Server className="w-3 h-3" />
                      )}
                      <span>
                        {task.taskName || task.id} ({getStatusText(task.status)})
                      </span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>

      {/* 内容区域 */}
      <div className="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6">
        <div className="flex min-w-0 flex-col gap-4 sm:gap-6">
          {/* 趋势图 */}
          {versions.length > 1 && (
            <Card className="min-w-0 flex-shrink-0 p-4 sm:p-6">
              <div className="flex items-center gap-2 mb-4">
                <TrendingUp className="w-5 h-5 text-primary" />
                <h2 className="text-lg font-semibold">{t.vulnScan.history.trendTitle}</h2>
              </div>
              <div className="h-[200px]">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={trendData} margin={{ top: 10, right: 10, left: -20, bottom: 0 }}>
                    <defs>
                      <linearGradient id="historyTrendBlue" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="#3b82f6" stopOpacity={0.45} />
                        <stop offset="100%" stopColor="#3b82f6" stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="historyTrendGreen" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="#22c55e" stopOpacity={0.45} />
                        <stop offset="100%" stopColor="#22c55e" stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="historyTrendOrange" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="#f97316" stopOpacity={0.45} />
                        <stop offset="100%" stopColor="#f97316" stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="historyTrendCyan" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="#06b6d4" stopOpacity={0.45} />
                        <stop offset="100%" stopColor="#06b6d4" stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="historyTrendTeal" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="#14b8a6" stopOpacity={0.45} />
                        <stop offset="100%" stopColor="#14b8a6" stopOpacity={0} />
                      </linearGradient>
                      <linearGradient id="historyTrendPurple" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor="#8b5cf6" stopOpacity={0.45} />
                        <stop offset="100%" stopColor="#8b5cf6" stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" opacity={0.2} vertical={false} />
                    <XAxis
                      dataKey="version"
                      tick={{ fontSize: 11 }}
                      axisLine={false}
                      tickLine={false}
                    />
                    <YAxis
                      tick={{ fontSize: 11 }}
                      axisLine={false}
                      tickLine={false}
                      allowDecimals={false}
                    />
                    <RechartsTooltip
                      contentStyle={{
                        backgroundColor: 'hsl(var(--popover))',
                        border: '1px solid hsl(var(--border))',
                        borderRadius: '8px',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.3)',
                      }}
                      itemStyle={{ color: 'hsl(var(--foreground))', fontSize: 12 }}
                      labelStyle={{ color: 'hsl(var(--foreground))', fontWeight: 600, marginBottom: 4 }}
                    />
                    <Legend wrapperStyle={{ fontSize: 12 }} content={renderTrendLegend} />
                    {trendMetrics.map((metric) => (
                      <Area
                        key={metric.key}
                        type="monotone"
                        dataKey={metric.key}
                        name={metric.label}
                        stroke={metric.stroke}
                        strokeWidth={2}
                        fill={metric.fill}
                        hide={hiddenTrendMetricKeys.has(metric.key)}
                      />
                    ))}
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </Card>
          )}

          {/* 扫描历史列表 */}
          <div className="space-y-4">
            {loading && versions.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">{t.vulnScan.history.loading}</div>
            ) : versions.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">{t.vulnScan.history.noHistory}</div>
            ) : (
              [...versions].sort((a, b) => b.version - a.version).map((version) => {
                const versionMetrics = getVersionMetrics(version)
                return (
                  <Card key={version.version} className="p-4">
                    <div className="flex min-w-0 flex-col gap-4 sm:flex-row sm:items-start sm:justify-between sm:gap-0">
                      <div className="w-full min-w-0 flex-1">
                        <div className="flex items-center gap-3 mb-2">
                          <div className="flex w-full flex-wrap items-center justify-between gap-2 sm:w-auto sm:justify-start">
                            <span className="font-semibold whitespace-nowrap">{t.vulnScan.history.scanNumber.replace('{n}', String(version.version))}</span>
                            <div className="shrink-0 sm:ml-2">
                              {getStatusBadge(version.status, taskStatusLabels)}
                            </div>
                          </div>
                        </div>

                        <div className="mt-3 grid grid-cols-2 gap-x-3 gap-y-3 md:grid-cols-4 md:gap-4">
                          {versionMetrics.map((metric) => {
                            const Icon = metric.icon
                            return (
                              <div key={metric.key} className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                                <span className="inline-flex shrink-0 items-center gap-2 whitespace-nowrap text-sm text-muted-foreground">
                                  <Icon className="h-4 w-4 shrink-0" />
                                  {metric.label}:
                                </span>
                                <span className="min-w-0 break-all text-sm font-medium tabular-nums">{metric.value}</span>
                              </div>
                            )
                          })}
                          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                            <span className="whitespace-nowrap text-sm text-muted-foreground">{t.vulnScan.history.progress}:</span>
                            <span className="text-sm font-medium tabular-nums">{version.progress}%</span>
                          </div>
                        </div>

                        {version.vulnCount > 0 && (
                          <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1">
                            {version.criticalCount > 0 && (
                              <span className="text-xs text-red-400">{t.vulnScan.severity.critical}: {version.criticalCount}</span>
                            )}
                            {version.highCount > 0 && (
                              <span className="text-xs text-orange-400">{t.vulnScan.severity.high}: {version.highCount}</span>
                            )}
                            {version.mediumCount > 0 && (
                              <span className="text-xs text-yellow-400">{t.vulnScan.severity.medium}: {version.mediumCount}</span>
                            )}
                          </div>
                        )}

                        <div className="mt-3 flex flex-col gap-1.5 text-xs text-muted-foreground sm:mt-2 sm:flex-row sm:flex-wrap sm:items-center sm:gap-x-4 sm:gap-y-1">
                          <span>{t.vulnScan.history.created}: {formatDate(version.createdAt)}</span>
                          {version.completedAt && (
                            <span>{t.vulnScan.history.completed}: {formatDate(version.completedAt)}</span>
                          )}
                        </div>
                      </div>

                      <Button
                        variant="default"
                        size="sm"
                        onClick={() => handleViewVersionDetails(version.version)}
                        className="h-11 w-full shrink-0 sm:ml-4 sm:h-8 sm:w-auto"
                      >
                        <Eye className="w-4 h-4 mr-1" />
                        {t.vulnScan.history.viewDetail}
                      </Button>
                    </div>
                  </Card>
                )
              })
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
