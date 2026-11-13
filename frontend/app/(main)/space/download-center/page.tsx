'use client'

import mobileTable from '@/components/ui/mobile-record-table.module.css'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { CircleAlert, Download, MoreHorizontal, RefreshCw, Trash2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { ReportFormatSelector } from '@/app/(main)/space/download-center/components/ReportFormatSelector'
import { fetchWithAuth } from '@/lib/api'
import { exportAIWorkbenchProjectReports, listAIWorkbenchPenTestProjects, type AIWorkbenchProject } from '@/lib/api/ai-workbench'
import { ensureFileExtension, parseFilenameFromContentDisposition, saveBlobAsFile } from '@/lib/download'
import { useI18n } from '@/lib/i18n'
import { saveReportBlob, type ReportExportFormat } from '@/lib/report-export'
import { getRiskLevelBadgeClassName, type RiskLevel } from '@/lib/risk-level-badge'
import { getStatusBadge as renderTaskStatusBadge, type TaskStatusLabels } from '@/lib/status-badge'

interface ExportTask {
  id: string
  engine: string
  taskName: string
  status: 'pending' | 'running' | 'paused' | 'success' | 'failed' | 'cancelled' | 'canceled'
  progress: number
  totalCount: number
  currentCount: number
  fileName: string
  fileSize: number
  errorMsg: string
  createdAt: string
  completedAt?: string
}

interface TaskListResponse {
  success: boolean
  data?: {
    list: ExportTask[]
    total: number
    page: number
    pageSize: number
  }
  error?: string
}

interface WebScanTask {
  id: string
  taskName?: string
  status: string
  progress: number
  resultCount: number
  version: number
  scanType: string
  criticalCount?: number
  highCount?: number
  mediumCount?: number
  lowCount?: number
  infoCount?: number
  vulnCount?: number
  errorMsg?: string
  createdAt: string
  completedAt?: string
}

interface WebScanTaskListResponse {
  success: boolean
  data?: {
    tasks: WebScanTask[]
    total: number
    page: number
    pageSize: number
    totalPages: number
  }
  error?: string
}

interface CompanyInfoTask {
  id: string
  taskName: string
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'cancelled' | 'canceled'
  progress: number
  totalCount: number
  currentCount: number
  errorMsg?: string
  createdAt: string
  completedAt?: string
}

interface CompanyInfoTaskListResponse {
  success: boolean
  data?: CompanyInfoTask[]
  error?: string
}

const webscanTaskKey = (task: Pick<WebScanTask, 'id' | 'version'>) => `${task.id}::${task.version}`
type WebscanExportFormat = 'word' | 'xlsx' | 'html'

const reportFormatOptions: Array<{ value: WebscanExportFormat; label: string; icon: string }> = [
  { value: 'word', label: 'Word (.docx)', icon: '/icon/word.svg' },
  { value: 'xlsx', label: 'Excel (.xlsx)', icon: '/icon/excel.svg' },
  { value: 'html', label: 'HTML', icon: '/icon/html.svg' },
]

const aiReportFormatOptions: Array<{ value: ReportExportFormat; label: string; icon: string }> = [
  { value: 'markdown', label: 'Markdown (.md)', icon: '/icon/markdown.svg' },
  { value: 'html', label: 'HTML', icon: '/icon/html.svg' },
]

const exportableReportStatuses = new Set(['completed', 'success', 'paused', 'cancelled', 'canceled'])

function canExportReportStatus(status: string | undefined) {
  return exportableReportStatuses.has((status || '').toLowerCase())
}

function formatFileSize(size: number) {
  if (!size || size <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = size
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(value >= 100 ? 0 : value >= 10 ? 1 : 2)} ${units[unitIndex]}`
}

function formatDateTime(value: string | undefined, locale: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString(locale)
}

export default function ReportManagementPage() {
  const { t, language } = useI18n()
  const locale = language === 'en' ? 'en-US' : 'zh-CN'

  // 扫描类型展示映射：列表与已选任务摘要统一显示可读类型名称
  const scanTypeLabels: Record<string, string> = {
    subdomain: t.vulnScan.taskManagement.subdomain,
    webscan: t.vulnScan.taskManagement.webscan,
    portscan: t.vulnScan.taskManagement.portscan,
    crack: t.vulnScan.taskManagement.crack,
    dirscan: t.vulnScan.taskManagement.dirscan,
    jsscan: t.vulnScan.taskManagement.jsscan,
    combination: t.vulnScan.taskManagement.combination,
  }
  const getScanTypeLabel = (scanType?: string | null) => {
    if (!scanType) return '-'
    const key = scanType.toLowerCase().trim()
    return scanTypeLabels[key] || scanType
  }
  const [activeTab, setActiveTab] = useState<'space-export' | 'vuln-scan' | 'company-info' | 'ai-workbench'>('vuln-scan')

  const [spaceTasks, setSpaceTasks] = useState<ExportTask[]>([])
  const [spaceLoading, setSpaceLoading] = useState(false)
  const [spacePage, setSpacePage] = useState(1)
  const [spacePageSize, setSpacePageSize] = useState(20)
  const [spaceTotal, setSpaceTotal] = useState(0)

  const [webscanTasks, setWebscanTasks] = useState<WebScanTask[]>([])
  const [webscanLoading, setWebscanLoading] = useState(false)
  const [webscanPage, setWebscanPage] = useState(1)
  const [webscanPageSize, setWebscanPageSize] = useState(20)
  const [webscanTotal, setWebscanTotal] = useState(0)
  const [webscanTypeFilter, setWebscanTypeFilter] = useState<
    'all' | 'webscan' | 'portscan' | 'dirscan' | 'jsscan' | 'crack' | 'combination'
  >('all')
  const [selectedWebscanTaskMap, setSelectedWebscanTaskMap] = useState<Record<string, WebScanTask>>({})
  const [webscanExportFormats, setWebscanExportFormats] = useState<WebscanExportFormat[]>(['word'])
  const [webscanExportFileName, setWebscanExportFileName] = useState('')

  const [companyTasks, setCompanyTasks] = useState<CompanyInfoTask[]>([])
  const [companyLoading, setCompanyLoading] = useState(false)
  const [selectedCompanyTaskMap, setSelectedCompanyTaskMap] = useState<Record<string, CompanyInfoTask>>({})
  const [companyExportFormats, setCompanyExportFormats] = useState<WebscanExportFormat[]>(['xlsx'])
  const [companyExportFileName, setCompanyExportFileName] = useState('')

  const [aiProjects, setAiProjects] = useState<AIWorkbenchProject[]>([])
  const [aiLoading, setAiLoading] = useState(false)
  const [aiExporting, setAiExporting] = useState(false)
  const [selectedAIProjectMap, setSelectedAIProjectMap] = useState<Record<string, AIWorkbenchProject>>({})
  const [aiReportFormat, setAIReportFormat] = useState<ReportExportFormat>('markdown')
  const [aiExportFileName, setAIExportFileName] = useState('')

  const hasRunningSpaceTask = useMemo(
    () => spaceTasks.some((task) => task.status === 'pending' || task.status === 'running'),
    [spaceTasks],
  )

  const hasRunningWebscanTask = useMemo(
    () => webscanTasks.some((task) => task.status === 'pending' || task.status === 'running'),
    [webscanTasks],
  )

  const hasRunningCompanyTask = useMemo(
    () => companyTasks.some((task) => task.status === 'pending' || task.status === 'running'),
    [companyTasks],
  )

  const webscanTaskStatusLabels = useMemo<TaskStatusLabels>(
    () => ({
      pending: t.vulnScan.statuses.pending,
      running: t.vulnScan.statuses.running,
      pausing: t.vulnScan.statuses.pausing,
      paused: t.vulnScan.statuses.paused,
      recovering: t.vulnScan.statuses.recovering,
      deleting: t.vulnScan.statuses.deleting,
      completed: t.vulnScan.statuses.completed,
      failed: t.vulnScan.statuses.failed,
      cancelled: t.vulnScan.statuses.cancelled,
    }),
    [t],
  )

  const getTaskStatusLabel = useCallback(
    (status: string) => {
      switch (status) {
        case 'pending':
          return webscanTaskStatusLabels.pending
        case 'running':
          return webscanTaskStatusLabels.running
        case 'pausing':
          return webscanTaskStatusLabels.pausing
        case 'paused':
          return webscanTaskStatusLabels.paused
        case 'recovering':
          return webscanTaskStatusLabels.recovering
        case 'completed':
        case 'success':
          return webscanTaskStatusLabels.completed
        case 'failed':
          return webscanTaskStatusLabels.failed
        case 'canceled':
        case 'cancelled':
          return webscanTaskStatusLabels.cancelled
        default:
          return status || '-'
      }
    },
    [webscanTaskStatusLabels],
  )

  const exportTaskStatusBadge = useCallback(
    (status: ExportTask['status']) => {
      switch (status) {
        case 'pending':
          return <Badge variant="secondary">{t.downloadCenter.status.pending}</Badge>
        case 'running':
          return <Badge className="bg-blue-500/15 text-blue-500 hover:bg-blue-500/20">{t.downloadCenter.status.running}</Badge>
        case 'paused':
          return <Badge variant="outline">{t.vulnScan.statuses.paused}</Badge>
        case 'success':
          return <Badge className="bg-emerald-500/15 text-emerald-500 hover:bg-emerald-500/20">{t.downloadCenter.status.success}</Badge>
        case 'failed':
          return <Badge className="bg-red-500/15 text-red-400 border-red-500/30 hover:bg-red-500/20">{t.downloadCenter.status.failed}</Badge>
        case 'canceled':
        case 'cancelled':
          return <Badge variant="outline">{t.downloadCenter.status.cancelled}</Badge>
        default:
          return <Badge variant="outline">{status}</Badge>
      }
    },
    [t],
  )

  const webscanTaskStatusBadge = useCallback(
    (status: string) => {
      const normalizedStatus = status === 'success' ? 'completed' : status
      return renderTaskStatusBadge(normalizedStatus, webscanTaskStatusLabels)
    },
    [webscanTaskStatusLabels],
  )

  const getRiskLevel = useCallback(
    (task: WebScanTask): { label: string; className: string } => {
      const critical = task.criticalCount || 0
      const high = task.highCount || 0
      const medium = task.mediumCount || 0
      const low = (task.lowCount || 0) + (task.infoCount || 0)

      if (critical > 0) return { label: t.vulnScan.severity.critical, className: getRiskLevelBadgeClassName('critical') }
      if (high > 0) return { label: t.vulnScan.severity.high, className: getRiskLevelBadgeClassName('high') }
      if (medium > 0) return { label: t.vulnScan.severity.medium, className: getRiskLevelBadgeClassName('medium') }
      if (low > 0) return { label: t.vulnScan.severity.low, className: getRiskLevelBadgeClassName('low') }
      return { label: t.common.none, className: getRiskLevelBadgeClassName('none' as RiskLevel) }
    },
    [t],
  )

  const loadSpaceTasks = useCallback(
    async (silent = false) => {
      if (!silent) setSpaceLoading(true)
      try {
        const response = await fetchWithAuth(`/api/v1/space/export/tasks?page=${spacePage}&pageSize=${spacePageSize}`, {
          method: 'GET',
        })
        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}))
          throw new Error(errorData.error || t.downloadCenter.spaceExport.loadTasksFailed)
        }
        const data = (await response.json()) as TaskListResponse
        if (!data.success) {
          throw new Error(data.error || t.downloadCenter.spaceExport.loadTasksFailed)
        }
        setSpaceTasks(data.data?.list || [])
        setSpaceTotal(data.data?.total || 0)
      } catch (error: any) {
        toast.error(error?.message || t.downloadCenter.spaceExport.loadTasksFailed)
      } finally {
        if (!silent) setSpaceLoading(false)
      }
    },
    [spacePage, spacePageSize, t],
  )

  const loadWebscanTasks = useCallback(
    async (silent = false) => {
      if (!silent) setWebscanLoading(true)
      try {
        const queryParams = new URLSearchParams()
        queryParams.set('page', String(webscanPage))
        queryParams.set('pageSize', String(webscanPageSize))
        if (webscanTypeFilter !== 'all') queryParams.set('scanMode', webscanTypeFilter)

        const response = await fetchWithAuth(`/api/v1/webscan/tasks?${queryParams.toString()}`, {
          method: 'GET',
        })
        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}))
          throw new Error(errorData.error || t.downloadCenter.vulnScan.loadTasksFailed)
        }
        const data = (await response.json()) as WebScanTaskListResponse
        if (!data.success) {
          throw new Error(data.error || t.downloadCenter.vulnScan.loadTasksFailed)
        }
        setWebscanTasks(data.data?.tasks || [])
        setWebscanTotal(data.data?.total || 0)
      } catch (error: any) {
        toast.error(error?.message || t.downloadCenter.vulnScan.loadTasksFailed)
      } finally {
        if (!silent) setWebscanLoading(false)
      }
    },
    [webscanPage, webscanPageSize, webscanTypeFilter, t],
  )

  const loadCompanyTasks = useCallback(
    async (silent = false) => {
      if (!silent) setCompanyLoading(true)
      try {
        const response = await fetchWithAuth('/api/v1/company/tasks', { method: 'GET' })
        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}))
          throw new Error(errorData.error || t.downloadCenter.companyInfo.loadTasksFailed)
        }
        const data = (await response.json()) as CompanyInfoTaskListResponse
        if (!data.success) {
          throw new Error(data.error || t.downloadCenter.companyInfo.loadTasksFailed)
        }
        setCompanyTasks(data.data || [])
      } catch (error: any) {
        toast.error(error?.message || t.downloadCenter.companyInfo.loadTasksFailed)
      } finally {
        if (!silent) setCompanyLoading(false)
      }
    },
    [t],
  )

  const loadAIProjects = useCallback(
    async (silent = false) => {
      if (!silent) setAiLoading(true)
      try {
        const data = await listAIWorkbenchPenTestProjects({ pageSize: 10000 })
        setAiProjects(data.projects || [])
      } catch (error: any) {
        toast.error(error?.message || t.downloadCenter.aiWorkbench.loadProjectsFailed)
      } finally {
        if (!silent) setAiLoading(false)
      }
    },
    [t],
  )

  useEffect(() => {
    if (activeTab !== 'space-export') return
    const timer = window.setTimeout(() => {
      void loadSpaceTasks()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [activeTab, loadSpaceTasks])

  useEffect(() => {
    if (activeTab !== 'vuln-scan') return
    const timer = window.setTimeout(() => {
      void loadWebscanTasks()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [activeTab, webscanTypeFilter, webscanPage, webscanPageSize, loadWebscanTasks])

  useEffect(() => {
    if (activeTab !== 'company-info') return
    const timer = window.setTimeout(() => {
      void loadCompanyTasks()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [activeTab, loadCompanyTasks])

  useEffect(() => {
    if (activeTab !== 'ai-workbench') return
    const timer = window.setTimeout(() => {
      void loadAIProjects()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [activeTab, loadAIProjects])

  const selectedWebscanTaskList = useMemo(
    () => Object.values(selectedWebscanTaskMap),
    [selectedWebscanTaskMap],
  )

  const selectedWebscanTask = useMemo(
    () => (selectedWebscanTaskList.length === 1 ? selectedWebscanTaskList[0] : null),
    [selectedWebscanTaskList],
  )

  const selectedCompanyTaskList = useMemo(
    () => Object.values(selectedCompanyTaskMap),
    [selectedCompanyTaskMap],
  )

  const selectedAIProjectList = useMemo(
    () => Object.values(selectedAIProjectMap),
    [selectedAIProjectMap],
  )

  const allAIProjectsSelected = aiProjects.length > 0 && aiProjects.every((project) => Boolean(selectedAIProjectMap[project.id]))
  const aiSelectionKey = selectedAIProjectList.map((project) => project.id).sort().join('|')
  const aiSelectionKeyRef = useRef('')

  const canExportSelectedCompanyTask = useMemo(() => {
    if (selectedCompanyTaskList.length === 0) return false
    return selectedCompanyTaskList.every((task) => canExportReportStatus(task.status))
  }, [selectedCompanyTaskList])

  const canExportSelectedTask = useMemo(() => {
    if (selectedWebscanTaskList.length === 0) return false
    return selectedWebscanTaskList.every((task) => canExportReportStatus(task.status))
  }, [selectedWebscanTaskList])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (selectedWebscanTaskList.length === 0) {
        setWebscanExportFileName('')
        return
      }
      if (selectedWebscanTaskList.length === 1) {
        const task = selectedWebscanTaskList[0]
        const baseName = (task.taskName || '').trim()
        const safeBase = baseName
          ? baseName.replaceAll('/', '_').replaceAll('\\', '_')
          : `${t.downloadCenter.vulnScan.defaultTaskName}_${task.id.slice(0, 8)}`
        setWebscanExportFileName(`${safeBase}_v${task.version}`)
        return
      }
      const date = new Date().toLocaleDateString(locale).replaceAll('/', '').replaceAll('.', '')
      setWebscanExportFileName(
        `${t.downloadCenter.vulnScan.mergedDefaultName}_${selectedWebscanTaskList.length}${t.downloadCenter.vulnScan.itemUnit}_${date}`,
      )
    }, 0)

    return () => window.clearTimeout(timer)
  }, [locale, selectedWebscanTaskList, t])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      // 仅在选择变化时重置默认文件名，刷新项目列表不覆盖用户输入。
      if (aiSelectionKeyRef.current === aiSelectionKey) return
      aiSelectionKeyRef.current = aiSelectionKey
      if (selectedAIProjectList.length === 0) {
        setAIExportFileName('')
        return
      }
      if (selectedAIProjectList.length === 1) {
        const project = selectedAIProjectList[0]
        const baseName = (project.name || '').trim()
        const safeBase = baseName
          ? baseName.replaceAll('/', '_').replaceAll('\\', '_')
          : `${t.downloadCenter.aiWorkbench.defaultProjectName}_${project.id.slice(0, 8)}`
        setAIExportFileName(safeBase)
        return
      }
      const date = new Date().toLocaleDateString(locale).replaceAll('/', '').replaceAll('.', '')
      setAIExportFileName(
        `${t.downloadCenter.aiWorkbench.mergedDefaultName}_${selectedAIProjectList.length}${t.downloadCenter.aiWorkbench.projectUnit}_${date}`,
      )
    }, 0)

    return () => window.clearTimeout(timer)
  }, [aiSelectionKey, locale, selectedAIProjectList, t])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      // 刷新时同步已选项目，并丢弃已经不存在的项目
      setSelectedAIProjectMap((prev) => {
        if (Object.keys(prev).length === 0) return prev
        let changed = false
        const next: Record<string, AIWorkbenchProject> = {}
        aiProjects.forEach((project) => {
          if (prev[project.id]) {
            next[project.id] = project
            changed = true
          }
        })
        if (Object.keys(next).length !== Object.keys(prev).length) changed = true
        return changed ? next : prev
      })
    }, 0)

    return () => window.clearTimeout(timer)
  }, [aiProjects])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      // 刷新时同步已选任务状态（保持跨页多选）
      setSelectedWebscanTaskMap((prev) => {
        if (Object.keys(prev).length === 0) return prev
        let changed = false
        const next = { ...prev }
        webscanTasks.forEach((task) => {
          const key = webscanTaskKey(task)
          if (next[key]) {
            next[key] = task
            changed = true
          }
        })
        return changed ? next : prev
      })
    }, 0)

    return () => window.clearTimeout(timer)
  }, [webscanTasks])

  useEffect(() => {
    if (activeTab !== 'space-export') return
    if (!hasRunningSpaceTask) return
    const timer = setInterval(() => {
      void loadSpaceTasks(true)
    }, 5000)
    return () => clearInterval(timer)
  }, [activeTab, hasRunningSpaceTask, loadSpaceTasks])

  useEffect(() => {
    if (activeTab !== 'vuln-scan') return
    if (!hasRunningWebscanTask) return
    const timer = setInterval(() => {
      void loadWebscanTasks(true)
    }, 5000)
    return () => clearInterval(timer)
  }, [activeTab, hasRunningWebscanTask, loadWebscanTasks])

  useEffect(() => {
    if (activeTab !== 'company-info') return
    if (!hasRunningCompanyTask) return
    const timer = setInterval(() => {
      void loadCompanyTasks(true)
    }, 5000)
    return () => clearInterval(timer)
  }, [activeTab, hasRunningCompanyTask, loadCompanyTasks])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (selectedCompanyTaskList.length === 0) {
        setCompanyExportFileName('')
        return
      }
      if (selectedCompanyTaskList.length === 1) {
        const task = selectedCompanyTaskList[0]
        const baseName = (task.taskName || '').trim()
        const safeBase = baseName
          ? baseName.replaceAll('/', '_').replaceAll('\\', '_')
          : `企业信息_${task.id.slice(0, 8)}`
        setCompanyExportFileName(safeBase)
        return
      }
      const date = new Date().toLocaleDateString(locale).replaceAll('/', '').replaceAll('.', '')
      setCompanyExportFileName(`企业信息合并报告_${selectedCompanyTaskList.length}${t.downloadCenter.vulnScan.itemUnit}_${date}`)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [locale, selectedCompanyTaskList, t])

  const downloadSpaceExportTask = async (task: ExportTask) => {
    try {
      const response = await fetchWithAuth(`/api/v1/space/export/tasks/${task.id}/download`, {
        method: 'GET',
      })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.downloadCenter.spaceExport.downloadFailed)
      }
      const blob = await response.blob()
      const filenameFromHeader = parseFilenameFromContentDisposition(response.headers.get('content-disposition'))
      const result = await saveBlobAsFile(blob, ensureFileExtension(filenameFromHeader || task.fileName || task.id, 'csv'))
      if (result === 'saved') {
        toast.success(t.downloadCenter.spaceExport.downloadStarted)
      }
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.spaceExport.downloadFailed)
    }
  }

  const deleteSpaceExportTask = async (task: ExportTask) => {
    try {
      const response = await fetchWithAuth(`/api/v1/space/export/tasks/${task.id}`, {
        method: 'DELETE',
      })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.downloadCenter.spaceExport.deleteFailed)
      }
      toast.success(t.downloadCenter.spaceExport.deleteSuccess)
      await loadSpaceTasks(true)
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.spaceExport.deleteFailed)
    }
  }

  const downloadWebscanReport = async (task: WebScanTask, opts?: { formats?: WebscanExportFormat[]; fileName?: string }) => {
    try {
      const formats = opts?.formats?.length ? opts.formats : ['word']
      const fileName = (opts?.fileName || '').trim()
      const query = new URLSearchParams()
      query.set('formats', formats.join(','))
      if (fileName) query.set('fileName', fileName)
      const urlPath = `/api/v1/webscan/task/${task.id}/version/${task.version}/report?${query.toString()}`

      const response = await fetchWithAuth(urlPath, {
        method: 'GET',
      })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.downloadCenter.vulnScan.exportReportFailed)
      }
      const blob = await response.blob()
      const filenameFromHeader = parseFilenameFromContentDisposition(response.headers.get('content-disposition'))
      const primaryFormat = formats[0]
      const ext = formats.length > 1 ? 'zip' : primaryFormat === 'xlsx' ? 'xlsx' : primaryFormat === 'html' ? 'html' : 'docx'
      const result = await saveBlobAsFile(
        blob,
        filenameFromHeader || `${t.downloadCenter.vulnScan.scanReportDefaultName}_${task.id}_v${task.version}.${ext}`,
      )
      if (result === 'saved') {
        toast.success(t.downloadCenter.vulnScan.exportStarted)
      }
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.vulnScan.exportReportFailed)
    }
  }

  const downloadMergedWebscanReport = async (
    tasks: WebScanTask[],
    opts?: { formats?: WebscanExportFormat[]; fileName?: string },
  ) => {
    try {
      if (!tasks.length) return
      const formats = opts?.formats?.length ? opts.formats : ['word']
      const fileName = (opts?.fileName || '').trim()

      const response = await fetchWithAuth('/api/v1/webscan/reports/merge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tasks: tasks.map((task) => ({ id: task.id, version: task.version })),
          formats,
          fileName,
        }),
      })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.downloadCenter.vulnScan.exportMergedFailed)
      }

      const blob = await response.blob()
      const filenameFromHeader = parseFilenameFromContentDisposition(response.headers.get('content-disposition'))
      const primaryFormat = formats[0]
      const ext = formats.length > 1 ? 'zip' : primaryFormat === 'xlsx' ? 'xlsx' : primaryFormat === 'html' ? 'html' : 'docx'
      const result = await saveBlobAsFile(
        blob,
        filenameFromHeader ||
          `${t.downloadCenter.vulnScan.mergedExportFileName}_${tasks.length}${t.downloadCenter.vulnScan.itemUnit}.${ext}`,
      )
      if (result === 'saved') {
        toast.success(t.downloadCenter.vulnScan.exportMergedStarted)
      }
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.vulnScan.exportMergedFailed)
    }
  }

  const downloadCompanyInfoReport = async (task: CompanyInfoTask, opts?: { formats?: WebscanExportFormat[]; fileName?: string }) => {
    try {
      const formats = opts?.formats?.length ? opts.formats : ['xlsx']
      const fileName = (opts?.fileName || '').trim()
      const query = new URLSearchParams()
      query.set('formats', formats.join(','))
      if (fileName) query.set('fileName', fileName)
      const response = await fetchWithAuth(`/api/v1/company/task/${task.id}/export?${query.toString()}`, { method: 'GET' })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.downloadCenter.companyInfo.exportFailed)
      }
      const blob = await response.blob()
      const filenameFromHeader = parseFilenameFromContentDisposition(response.headers.get('content-disposition'))
      const primaryFormat = formats[0]
      const ext = formats.length > 1 ? 'zip' : primaryFormat === 'html' ? 'html' : primaryFormat === 'word' ? 'docx' : 'xlsx'
      const result = await saveBlobAsFile(
        blob,
        filenameFromHeader || `${(fileName || task.taskName || 'company_info').trim() || 'company_info'}.${ext}`,
      )
      if (result === 'saved') {
        toast.success(t.downloadCenter.companyInfo.exportStarted)
      }
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.companyInfo.exportFailed)
    }
  }

  const downloadMergedCompanyInfoReport = async (tasks: CompanyInfoTask[], opts?: { formats?: WebscanExportFormat[]; fileName?: string }) => {
    try {
      const formats = opts?.formats?.length ? opts.formats : ['xlsx']
      const fileName = (opts?.fileName || '').trim()
      const response = await fetchWithAuth('/api/v1/company/reports/merge', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tasks: tasks.map((task) => ({ id: task.id })),
          formats,
          fileName,
        }),
      })
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.downloadCenter.companyInfo.exportMergedFailed)
      }
      const blob = await response.blob()
      const filenameFromHeader = parseFilenameFromContentDisposition(response.headers.get('content-disposition'))
      const primaryFormat = formats[0]
      const ext = formats.length > 1 ? 'zip' : primaryFormat === 'html' ? 'html' : primaryFormat === 'word' ? 'docx' : 'xlsx'
      const result = await saveBlobAsFile(
        blob,
        filenameFromHeader || `${((fileName || 'company_info_merged').trim() || 'company_info_merged')}.${ext}`,
      )
      if (result === 'saved') {
        toast.success(t.downloadCenter.companyInfo.exportMergedStarted)
      }
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.companyInfo.exportMergedFailed)
    }
  }

  const exportSelectedAIProjects = async () => {
    if (selectedAIProjectList.length === 0 || aiExporting) return
    setAiExporting(true)
    try {
      const report = await exportAIWorkbenchProjectReports(
        selectedAIProjectList.map((project) => project.id),
        aiExportFileName.trim(),
        aiReportFormat,
      )
      const result = await saveReportBlob(report.blob, report.filename, aiReportFormat)
      if (result === 'saved') {
        toast.success(t.downloadCenter.aiWorkbench.exportStarted)
      }
    } catch (error: any) {
      toast.error(error?.message || t.downloadCenter.aiWorkbench.exportFailed)
    } finally {
      setAiExporting(false)
    }
  }

  return (
    <div className="p-3 md:p-6 space-y-6">
      <Card className="p-3 md:p-6">
        <Tabs
          value={activeTab}
          onValueChange={(v) => {
            if (v === 'space-export' || v === 'vuln-scan' || v === 'company-info' || v === 'ai-workbench') setActiveTab(v)
          }}
        >
          <TabsList className="h-auto max-w-full flex-wrap justify-start">
            <TabsTrigger value="vuln-scan">{t.downloadCenter.tabs.vulnScan}</TabsTrigger>
            <TabsTrigger value="company-info">{t.downloadCenter.tabs.companyInfo}</TabsTrigger>
            <TabsTrigger value="ai-workbench">{t.downloadCenter.tabs.aiWorkbench}</TabsTrigger>
            <TabsTrigger value="space-export">{t.downloadCenter.tabs.spaceExport}</TabsTrigger>
          </TabsList>

          <TabsContent value="space-export" className="mt-4">
            <div className="flex flex-col items-stretch justify-between gap-3 mb-4 md:flex-row md:items-center">
              <div>
                <p className="text-sm text-muted-foreground">{t.downloadCenter.spaceExport.description}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void loadSpaceTasks()} disabled={spaceLoading}>
                <RefreshCw className="w-4 h-4 mr-2" />
                {t.common.refresh}
              </Button>
            </div>

            <div className="border rounded-lg overflow-hidden">
              <Table className={mobileTable.table}>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t.downloadCenter.spaceExport.taskName}</TableHead>
                    <TableHead>{t.downloadCenter.spaceExport.engine}</TableHead>
                    <TableHead>{t.downloadCenter.spaceExport.status}</TableHead>
                    <TableHead>{t.downloadCenter.spaceExport.progress}</TableHead>
                    <TableHead>{t.downloadCenter.spaceExport.resultCount}</TableHead>
                    <TableHead>{t.downloadCenter.spaceExport.fileSize}</TableHead>
                    <TableHead>{t.downloadCenter.spaceExport.createdAt}</TableHead>
                    <TableHead className="text-center">
                      <span className="inline-flex items-center justify-center w-full">
                        <MoreHorizontal className="w-4 h-4 text-muted-foreground" aria-hidden="true" />
                        <span className="sr-only">{t.common.action}</span>
                      </span>
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {spaceLoading ? (
                    <TableRow>
                      <TableCell colSpan={8} className="text-center py-10 text-muted-foreground">
                        {t.common.loading}
                      </TableCell>
                    </TableRow>
                  ) : spaceTasks.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="text-center py-10 text-muted-foreground">
                        {t.downloadCenter.spaceExport.noTasks}
                      </TableCell>
                    </TableRow>
                  ) : (
                    spaceTasks.map((task) => (
                      <TableRow key={task.id}>
                        <TableCell data-label={t.downloadCenter.spaceExport.taskName} className="max-w-[260px] truncate" title={task.taskName}>
                          {task.taskName || '-'}
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.engine} className="uppercase">{task.engine}</TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.status}>{exportTaskStatusBadge(task.status)}</TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.progress}>{task.progress}%</TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.resultCount}>
                          {task.currentCount}/{task.totalCount}
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.fileSize}>{formatFileSize(task.fileSize)}</TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.createdAt}>{formatDateTime(task.createdAt, locale)}</TableCell>
                        <TableCell data-label={t.common.action} className="text-center">
                          <TooltipProvider>
                            <div className="flex items-center justify-center gap-1">
                              {task.status === 'success' && (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="h-8 w-8"
                                      onClick={() => void downloadSpaceExportTask(task)}
                                      aria-label={t.common.download}
                                    >
                                      <Download className="w-4 h-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>{t.common.download}</TooltipContent>
                                </Tooltip>
                              )}

                              {task.status === 'failed' && (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="h-8 w-8 text-destructive hover:text-destructive"
                                      aria-label={t.downloadCenter.spaceExport.exportFailed}
                                    >
                                      <CircleAlert className="w-4 h-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent className="max-w-[360px] break-all">
                                    {task.errorMsg || t.downloadCenter.spaceExport.exportFailed}
                                  </TooltipContent>
                                </Tooltip>
                              )}

                              {task.status === 'pending' || task.status === 'running' ? (
                                <span className="text-xs text-muted-foreground">{t.downloadCenter.spaceExport.processing}</span>
                              ) : null}

                              {task.status !== 'pending' && task.status !== 'running' && (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <Button
                                      variant="ghost"
                                      size="icon"
                                      className="h-8 w-8"
                                      onClick={() => void deleteSpaceExportTask(task)}
                                      aria-label={t.common.delete}
                                    >
                                      <Trash2 className="w-4 h-4" />
                                    </Button>
                                  </TooltipTrigger>
                                  <TooltipContent>{t.common.delete}</TooltipContent>
                                </Tooltip>
                              )}
                            </div>
                          </TooltipProvider>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            <div className="mt-4">
              <PaginationPro
                current={spacePage}
                total={spaceTotal}
                pageSize={spacePageSize}
                onChange={(nextPage) => setSpacePage(nextPage)}
                onPageSizeChange={(size) => {
                  setSpacePageSize(size)
                  setSpacePage(1)
                }}
                pageSizeOptions={[10, 20, 50, 100]}
                showSizeChanger
                showQuickJumper
                showTotal
              />
            </div>
          </TabsContent>

          <TabsContent value="vuln-scan" className="mt-4">
            <div className="flex flex-col items-stretch justify-between gap-3 mb-4 md:flex-row md:items-center">
              <div>
                <p className="text-sm text-muted-foreground">
                  {t.downloadCenter.vulnScan.description}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <Select
                  value={webscanTypeFilter}
                  onValueChange={(v) => {
                    if (
                      v === 'all' ||
                      v === 'webscan' ||
                      v === 'portscan' ||
                      v === 'dirscan' ||
                      v === 'jsscan' ||
                      v === 'crack' ||
                      v === 'combination'
                    ) {
                      setWebscanTypeFilter(v)
                      setWebscanPage(1)
                    }
                  }}
                >
                  <SelectTrigger className="h-8 w-[160px]">
                    <SelectValue placeholder={t.downloadCenter.vulnScan.filterTaskType} />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t.downloadCenter.vulnScan.allTypes}</SelectItem>
                    <SelectItem value="webscan">{t.vulnScan.taskManagement.webscan}</SelectItem>
                    <SelectItem value="portscan">{t.vulnScan.taskManagement.portscan}</SelectItem>
                    <SelectItem value="dirscan">{t.vulnScan.taskManagement.dirscan}</SelectItem>
                    <SelectItem value="jsscan">{t.vulnScan.taskManagement.jsscan}</SelectItem>
                    <SelectItem value="crack">{t.vulnScan.taskManagement.crack}</SelectItem>
                    <SelectItem value="combination">{t.vulnScan.taskManagement.combination}</SelectItem>
                  </SelectContent>
                </Select>

                <Button variant="outline" size="sm" onClick={() => void loadWebscanTasks()} disabled={webscanLoading}>
                  <RefreshCw className="w-4 h-4 mr-2" />
                  {t.common.refresh}
                </Button>
              </div>
            </div>

            <div className="border rounded-lg p-4 bg-muted/20 mb-4 space-y-3">
              <div className="flex items-center justify-between gap-3 flex-wrap">
                <div className="text-sm">
                  <div className="font-medium">
                    {t.downloadCenter.vulnScan.selectedTasks}:{' '}
                    {selectedWebscanTaskList.length > 0
                      ? `${selectedWebscanTaskList.length} ${t.downloadCenter.vulnScan.itemUnit}`
                      : t.downloadCenter.vulnScan.notSelected}
                  </div>
                  {selectedWebscanTask ? (
                    <div className="text-muted-foreground mt-1 flex items-center gap-2 flex-wrap">
                      <Badge variant="outline" className={`${getRiskLevel(selectedWebscanTask).className} border`}>
                        {t.downloadCenter.vulnScan.risk}: {getRiskLevel(selectedWebscanTask).label}
                      </Badge>
                      <span>{t.downloadCenter.vulnScan.type}: {getScanTypeLabel(selectedWebscanTask.scanType)}</span>
                      <span>{t.downloadCenter.vulnScan.version}: v{selectedWebscanTask.version}</span>
                      <span>{t.downloadCenter.vulnScan.status}: {getTaskStatusLabel(selectedWebscanTask.status)}</span>
                    </div>
                  ) : selectedWebscanTaskList.length > 1 ? (
                    <div className="text-muted-foreground mt-1 flex items-center gap-2 flex-wrap">
                      <Badge variant="outline">{t.downloadCenter.vulnScan.mergeExport}</Badge>
                      <span>{t.downloadCenter.vulnScan.mergeExportHint}</span>
                    </div>
                  ) : null}
                </div>

                <Button
                  onClick={() => {
                    if (selectedWebscanTaskList.length === 0) return
                    if (selectedWebscanTaskList.length === 1) {
                      void downloadWebscanReport(selectedWebscanTaskList[0], {
                        formats: webscanExportFormats,
                        fileName: webscanExportFileName,
                      })
                      return
                    }
                    void downloadMergedWebscanReport(selectedWebscanTaskList, {
                      formats: webscanExportFormats,
                      fileName: webscanExportFileName,
                    })
                  }}
                  disabled={!canExportSelectedTask || webscanExportFormats.length === 0}
                >
                  <Download className="w-4 h-4 mr-2" />
                  {selectedWebscanTaskList.length > 1 ? t.downloadCenter.vulnScan.mergeExport : t.downloadCenter.vulnScan.export}
                </Button>
              </div>

              <div className="flex items-end gap-3 flex-wrap">
                <div className="min-w-0 w-full sm:w-auto sm:min-w-[280px]">
                  <div className="text-xs text-muted-foreground mb-1">{t.downloadCenter.vulnScan.exportType}</div>
                  <ReportFormatSelector
                    options={reportFormatOptions}
                    selected={webscanExportFormats}
                    onToggle={(value) =>
                      setWebscanExportFormats((prev) =>
                        prev.includes(value) ? prev.filter((item) => item !== value) : [...prev, value],
                      )
                    }
                    ariaLabel={t.downloadCenter.vulnScan.exportType}
                  />
                </div>

                <div className="flex-1 min-w-0 basis-full sm:basis-auto sm:min-w-[240px]">
                  <div className="text-xs text-muted-foreground mb-1">{t.downloadCenter.vulnScan.exportName}</div>
                  <Input
                    value={webscanExportFileName}
                    onChange={(e) => setWebscanExportFileName(e.target.value)}
                    placeholder={t.downloadCenter.vulnScan.exportNamePlaceholder}
                    className="h-9"
                  />
                </div>
              </div>

              {selectedWebscanTaskList.length > 0 && !canExportSelectedTask ? (
                <div className="text-xs text-muted-foreground">
                  {t.downloadCenter.vulnScan.pendingHint}
                </div>
              ) : null}
            </div>

            <div className="border rounded-lg overflow-hidden">
              <Table className={mobileTable.table}>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[56px] text-center">{t.downloadCenter.vulnScan.select}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.taskName}</TableHead>
                    <TableHead className="w-[120px]">{t.downloadCenter.vulnScan.riskLevel}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.type}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.status}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.version}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.resultCount}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.createdAt}</TableHead>
                    <TableHead>{t.downloadCenter.vulnScan.completedAt}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {webscanLoading ? (
                    <TableRow>
                      <TableCell colSpan={9} className="text-center py-10 text-muted-foreground">
                        {t.common.loading}
                      </TableCell>
                    </TableRow>
                  ) : webscanTasks.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={9} className="text-center py-10 text-muted-foreground">
                        {t.downloadCenter.vulnScan.noTasks}
                      </TableCell>
                    </TableRow>
                  ) : (
                    webscanTasks.map((task) => (
                      <TableRow key={`${task.id}-${task.version}`}>
                        <TableCell data-label={t.downloadCenter.vulnScan.select} className="text-center">
                          <Checkbox
                            checked={Boolean(selectedWebscanTaskMap[webscanTaskKey(task)])}
                            onCheckedChange={(checked) => {
                              const key = webscanTaskKey(task)
                              setSelectedWebscanTaskMap((prev) => {
                                if (checked) {
                                  return { ...prev, [key]: task }
                                }
                                if (!prev[key]) return prev
                                const next = { ...prev }
                                delete next[key]
                                return next
                              })
                            }}
                            aria-label={t.downloadCenter.vulnScan.selectTask}
                          />
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.taskName} className="max-w-[260px] truncate" title={task.taskName}>
                          {task.taskName || '-'}
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.riskLevel}>
                          <Badge variant="outline" className={`${getRiskLevel(task).className} border`}>
                            {getRiskLevel(task).label}
                          </Badge>
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.type} className="uppercase" title={task.scanType || undefined}>{getScanTypeLabel(task.scanType)}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.status}>{webscanTaskStatusBadge(task.status)}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.version}>v{task.version}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.resultCount}>{task.resultCount ?? '-'}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.createdAt}>{formatDateTime(task.createdAt, locale)}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.completedAt}>{formatDateTime(task.completedAt, locale)}</TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>

            <div className="mt-4">
              <PaginationPro
                current={webscanPage}
                total={webscanTotal}
                pageSize={webscanPageSize}
                onChange={(nextPage) => setWebscanPage(nextPage)}
                onPageSizeChange={(size) => {
                  setWebscanPageSize(size)
                  setWebscanPage(1)
                }}
                pageSizeOptions={[10, 20, 50, 100]}
                showSizeChanger
                showQuickJumper
                showTotal
              />
            </div>
          </TabsContent>

          <TabsContent value="company-info" className="mt-4">
            <div className="flex flex-col items-stretch justify-between gap-3 mb-4 md:flex-row md:items-center">
              <div>
                <p className="text-sm text-muted-foreground">
                  {t.downloadCenter.companyInfo.description}
                </p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void loadCompanyTasks()} disabled={companyLoading}>
                <RefreshCw className="w-4 h-4 mr-2" />
                {t.common.refresh}
              </Button>
            </div>

            <div className="border rounded-lg p-4 bg-muted/20 mb-4 space-y-3">
              <div className="flex items-center justify-between gap-3 flex-wrap">
                <div className="text-sm">
                  <div className="font-medium">
                    {t.downloadCenter.vulnScan.selectedTasks}: {selectedCompanyTaskList.length > 0 ? `${selectedCompanyTaskList.length} ${t.downloadCenter.vulnScan.itemUnit}` : t.downloadCenter.vulnScan.notSelected}
                  </div>
                  {selectedCompanyTaskList.length > 1 ? (
                    <div className="text-muted-foreground mt-1 flex items-center gap-2 flex-wrap">
                      <Badge variant="outline">{t.downloadCenter.companyInfo.mergeExport}</Badge>
                      <span>{t.downloadCenter.companyInfo.mergeExportHint}</span>
                    </div>
                  ) : null}
                </div>

                <Button
                  onClick={() => {
                    if (selectedCompanyTaskList.length === 0) return
                    if (selectedCompanyTaskList.length === 1) {
                      void downloadCompanyInfoReport(selectedCompanyTaskList[0], {
                        formats: companyExportFormats,
                        fileName: companyExportFileName,
                      })
                      return
                    }
                    void downloadMergedCompanyInfoReport(selectedCompanyTaskList, {
                      formats: companyExportFormats,
                      fileName: companyExportFileName,
                    })
                  }}
                  disabled={!canExportSelectedCompanyTask || companyExportFormats.length === 0}
                >
                  <Download className="w-4 h-4 mr-2" />
                  {selectedCompanyTaskList.length > 1 ? t.downloadCenter.companyInfo.mergeExport : t.downloadCenter.vulnScan.export}
                </Button>
              </div>

              <div className="flex items-end gap-3 flex-wrap">
                <div className="min-w-0 w-full sm:w-auto sm:min-w-[280px]">
                  <div className="text-xs text-muted-foreground mb-1">{t.downloadCenter.companyInfo.exportType}</div>
                  <ReportFormatSelector
                    options={reportFormatOptions}
                    selected={companyExportFormats}
                    onToggle={(value) =>
                      setCompanyExportFormats((prev) =>
                        prev.includes(value) ? prev.filter((item) => item !== value) : [...prev, value],
                      )
                    }
                    ariaLabel={t.downloadCenter.companyInfo.exportType}
                  />
                </div>

                <div className="flex-1 min-w-0 basis-full sm:basis-auto sm:min-w-[240px]">
                  <div className="text-xs text-muted-foreground mb-1">{t.downloadCenter.companyInfo.exportName}</div>
                  <Input
                    value={companyExportFileName}
                    onChange={(e) => setCompanyExportFileName(e.target.value)}
                    placeholder={t.downloadCenter.companyInfo.exportNamePlaceholder}
                    className="h-9"
                  />
                </div>
              </div>

              {selectedCompanyTaskList.length > 0 && !canExportSelectedCompanyTask ? (
                <div className="text-xs text-muted-foreground">
                  {t.downloadCenter.companyInfo.pendingHint}
                </div>
              ) : null}
            </div>

            <div className="border rounded-lg overflow-hidden">
              <Table className={mobileTable.table}>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[56px] text-center">{t.downloadCenter.vulnScan.select}</TableHead>
                    <TableHead>任务名</TableHead>
                    <TableHead>状态</TableHead>
                    <TableHead>进度</TableHead>
                    <TableHead>{t.downloadCenter.companyInfo.resultCount}</TableHead>
                    <TableHead>创建时间</TableHead>
                    <TableHead>完成时间</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {companyLoading ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-center py-10 text-muted-foreground">
                        {t.common.loading}
                      </TableCell>
                    </TableRow>
                  ) : companyTasks.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7} className="text-center py-10 text-muted-foreground">
                        {t.downloadCenter.companyInfo.noTasks}
                      </TableCell>
                    </TableRow>
                  ) : (
                    companyTasks.map((task) => (
                      <TableRow key={task.id}>
                        <TableCell data-label={t.downloadCenter.vulnScan.select} className="text-center">
                          <Checkbox
                            checked={Boolean(selectedCompanyTaskMap[task.id])}
                            onCheckedChange={(checked) => {
                              setSelectedCompanyTaskMap((prev) => {
                                if (checked) return { ...prev, [task.id]: task }
                                if (!prev[task.id]) return prev
                                const next = { ...prev }
                                delete next[task.id]
                                return next
                              })
                            }}
                            aria-label="选择企业信息任务"
                          />
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.taskName} className="max-w-[260px] truncate" title={task.taskName}>
                          {task.taskName || '-'}
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.status}>{webscanTaskStatusBadge(task.status)}</TableCell>
                        <TableCell data-label={t.downloadCenter.spaceExport.progress}>{task.progress}%</TableCell>
                        <TableCell data-label={t.downloadCenter.companyInfo.resultCount}>{task.currentCount}/{task.totalCount}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.createdAt}>{formatDateTime(task.createdAt, locale)}</TableCell>
                        <TableCell data-label={t.downloadCenter.vulnScan.completedAt}>{formatDateTime(task.completedAt, locale)}</TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </TabsContent>

          <TabsContent value="ai-workbench" className="mt-4">
            <div className="flex flex-col items-stretch justify-between gap-3 mb-4 md:flex-row md:items-center">
              <div>
                <p className="text-sm text-muted-foreground">{t.downloadCenter.aiWorkbench.description}</p>
              </div>
              <Button variant="outline" size="sm" onClick={() => void loadAIProjects()} disabled={aiLoading || aiExporting}>
                <RefreshCw className="w-4 h-4 mr-2" />
                {t.common.refresh}
              </Button>
            </div>

            <div className="border rounded-lg p-4 bg-muted/20 mb-4 space-y-3">
              <div className="flex items-center justify-between gap-3 flex-wrap">
                <div className="text-sm">
                  <div className="font-medium">
                    {t.downloadCenter.aiWorkbench.selectedProjects}:{' '}
                    {selectedAIProjectList.length > 0
                      ? `${selectedAIProjectList.length} ${t.downloadCenter.aiWorkbench.projectUnit}`
                      : t.downloadCenter.aiWorkbench.notSelected}
                  </div>
                  {selectedAIProjectList.length === 1 ? (
                    <div className="text-muted-foreground mt-1 flex items-center gap-2 flex-wrap">
                      <span>{t.downloadCenter.aiWorkbench.project}: {selectedAIProjectList[0].name || '-'}</span>
                      <span>{t.downloadCenter.aiWorkbench.updatedAt}: {formatDateTime(selectedAIProjectList[0].updatedAt, locale)}</span>
                    </div>
                  ) : selectedAIProjectList.length > 1 ? (
                    <div className="text-muted-foreground mt-1 flex items-center gap-2 flex-wrap">
                      <Badge variant="outline">{t.downloadCenter.aiWorkbench.mergeExport}</Badge>
                      <span>{t.downloadCenter.aiWorkbench.mergeExportHint}</span>
                    </div>
                  ) : null}
                </div>

                <Button onClick={() => void exportSelectedAIProjects()} disabled={selectedAIProjectList.length === 0 || aiExporting}>
                  {aiExporting ? <RefreshCw className="w-4 h-4 mr-2 animate-spin" /> : <Download className="w-4 h-4 mr-2" />}
                  {selectedAIProjectList.length > 1 ? t.downloadCenter.aiWorkbench.mergeExport : t.downloadCenter.aiWorkbench.export}
                </Button>
              </div>

              <div className="flex items-end gap-3 flex-wrap">
                <div className="min-w-0 w-full sm:w-auto sm:min-w-[280px]">
                  <div className="text-xs text-muted-foreground mb-1">{t.downloadCenter.vulnScan.exportType}</div>
                  <ReportFormatSelector
                    options={aiReportFormatOptions}
                    selected={[aiReportFormat]}
                    onToggle={(value) => setAIReportFormat(value)}
                    ariaLabel={t.downloadCenter.vulnScan.exportType}
                    multiple={false}
                    className="sm:grid-cols-2"
                  />
                </div>

                <div className="flex-1 min-w-0 basis-full sm:basis-auto sm:min-w-[240px]">
                  <div className="text-xs text-muted-foreground mb-1">{t.downloadCenter.aiWorkbench.exportName}</div>
                  <Input
                    value={aiExportFileName}
                    onChange={(e) => setAIExportFileName(e.target.value)}
                    placeholder={t.downloadCenter.aiWorkbench.exportNamePlaceholder}
                    className="h-9"
                  />
                </div>
              </div>
            </div>

            <div className="border rounded-lg overflow-hidden">
              <Table className={mobileTable.table}>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[56px] text-center">
                      <Checkbox
                        checked={allAIProjectsSelected ? true : selectedAIProjectList.length ? 'indeterminate' : false}
                        onCheckedChange={(checked) => {
                          setSelectedAIProjectMap(
                            checked === true
                              ? Object.fromEntries(aiProjects.map((project) => [project.id, project]))
                              : {},
                          )
                        }}
                        aria-label={t.downloadCenter.aiWorkbench.selectAll}
                      />
                    </TableHead>
                    <TableHead>{t.downloadCenter.aiWorkbench.project}</TableHead>
                    <TableHead>{t.downloadCenter.aiWorkbench.projectDescription}</TableHead>
                    <TableHead>{t.downloadCenter.aiWorkbench.status}</TableHead>
                    <TableHead>{t.downloadCenter.aiWorkbench.updatedAt}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {aiLoading ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-center py-10 text-muted-foreground">
                        {t.common.loading}
                      </TableCell>
                    </TableRow>
                  ) : aiProjects.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={5} className="text-center py-10 text-muted-foreground">
                        {t.downloadCenter.aiWorkbench.noProjects}
                      </TableCell>
                    </TableRow>
                  ) : (
                    aiProjects.map((project) => (
                      <TableRow key={project.id}>
                        <TableCell data-label={t.downloadCenter.aiWorkbench.select} className="text-center">
                          <Checkbox
                            checked={Boolean(selectedAIProjectMap[project.id])}
                            onCheckedChange={(checked) => {
                              setSelectedAIProjectMap((prev) => {
                                if (checked) return { ...prev, [project.id]: project }
                                if (!prev[project.id]) return prev
                                const next = { ...prev }
                                delete next[project.id]
                                return next
                              })
                            }}
                            aria-label={`${t.downloadCenter.aiWorkbench.selectProject} ${project.name || project.id}`}
                          />
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.aiWorkbench.project} className="max-w-[260px] truncate" title={project.name}>
                          {project.name || '-'}
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.aiWorkbench.projectDescription} className="max-w-[360px]">
                          <div className="line-clamp-2 text-muted-foreground" title={project.description}>{project.description || '-'}</div>
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.aiWorkbench.status}>
                          <Badge variant="outline">{project.status || '-'}</Badge>
                        </TableCell>
                        <TableCell data-label={t.downloadCenter.aiWorkbench.updatedAt}>{formatDateTime(project.updatedAt, locale)}</TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
          </TabsContent>
        </Tabs>
      </Card>
    </div>
  )
}
