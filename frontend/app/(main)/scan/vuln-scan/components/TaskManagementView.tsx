'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useIsMobile } from '@/hooks/use-mobile'
import { MobileListToolbar } from '@/components/ui/mobile-list-toolbar'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogTitle, AlertDialogDescription, AlertDialogFooter, AlertDialogCancel, AlertDialogAction } from '@/components/ui/alert-dialog'
import { useRouter } from 'next/navigation'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { getStatusBadge } from '@/lib/status-badge'
import { getRiskLevelBadgeClassName, type RiskLevel } from '@/lib/risk-level-badge'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Plus, Trash2, Globe, Clock, Search, Target, Play, Pause, Square, Eye, RotateCcw, AlertTriangle, Server, FolderTree, Code2, FolderSearch, FileCode, Layers, Loader2, Captions, Bot, Pencil, MoreHorizontal } from 'lucide-react'
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from '@/components/ui/select'
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
import { Progress } from '@/components/ui/progress'
import type { WebScanTaskSummary } from '../types'
import { ScanType } from '../types'
import { formatSchedule, formatScheduleTimestamp } from '../utils/task-schedule'
import { useI18n } from '@/lib/i18n'
import { CreateTaskDialog } from './CreateTaskDialog'
import { ScanStageTooltipContent, scanStageLabel } from './ScanStageTooltipContent'
import { cn } from '@/lib/utils'

interface TaskManagementViewProps {
	tasks?: WebScanTaskSummary[]
	total?: number  // 后端返回的总数
	currentPage?: number  // 当前页码
	pageSize?: number  // 每页数量
	onDeleteTask?: (taskId: string) => void
	onBatchDeleteTasks?: (taskIds: string[]) => void
	onCreateTask?: (scanMode: ScanType) => void // Updated to support extended task types
	onOpenCombinationTask?: () => void
	onViewTask?: (taskId: string) => void
	onStartTask?: (taskId: string) => void
	onPauseTask?: (taskId: string) => void
	onResumeTask?: (taskId: string) => void
	onStopTask?: (taskId: string) => void
	onUpdateTaskName?: (taskId: string, taskName: string) => Promise<boolean>
	onPageChange?: (page: number, pageSize: number) => void  // 分页变化回调
	onFilterChange?: (filters: { search?: string; status?: string; scanMode?: string }) => void  // 筛选变化回调
}

export function TaskManagementView({
	tasks = [],
	total = 0,
	currentPage: propCurrentPage = 1,
	pageSize: propPageSize = 20,
	onDeleteTask,
	onBatchDeleteTasks,
	onCreateTask,
	onOpenCombinationTask,
	onViewTask,
	onStartTask,
	onPauseTask,
	onResumeTask,
	onStopTask,
	onUpdateTaskName,
	onPageChange,
	onFilterChange,
}: TaskManagementViewProps) {
	const { t } = useI18n()
	const isMobile = useIsMobile()
	const [mobileSelecting, setMobileSelecting] = useState(false)
	const taskStatusLabels = {
		pending: t.vulnScan.statuses.pending,
		running: t.vulnScan.statuses.running,
		pausing: t.vulnScan.statuses.pausing,
		paused: t.vulnScan.statuses.paused,
		recovering: t.vulnScan.statuses.recovering,
		deleting: t.vulnScan.statuses.deleting,
		completed: t.vulnScan.statuses.completed,
		failed: t.vulnScan.statuses.failed,
		cancelled: t.vulnScan.statuses.cancelled,
	}
	const router = useRouter()
	const [searchQuery, setSearchQuery] = useState('')
	const [statusFilter, setStatusFilter] = useState<string>('all')
	const [typeFilter, setTypeFilter] = useState<string>('all')
	const [selectedRows, setSelectedRows] = useState<Set<string>>(new Set())
	const [createDialogOpen, setCreateDialogOpen] = useState(false)
	const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
	const [confirmDeleteTaskId, setConfirmDeleteTaskId] = useState<string | null>(null)
	const [batchDeletePopoverOpen, setBatchDeletePopoverOpen] = useState(false)
	const [editingTask, setEditingTask] = useState<WebScanTaskSummary | null>(null)
	const [editedTaskName, setEditedTaskName] = useState('')
	const [isUpdatingTaskName, setIsUpdatingTaskName] = useState(false)
	const taskNameInputRef = useRef<HTMLInputElement | null>(null)
	const taskNameUpdatePendingRef = useRef(false)
	const taskNameEditCancelledRef = useRef(false)
	const tableHostRef = useRef<HTMLDivElement | null>(null)
	const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)

	// 使用后端分页时直接使用 props 中的任务列表
	const displayTasks = tasks

	const formatDate = (dateString?: string) => {
		if (!dateString) return '-'
		return new Date(dateString).toLocaleString('zh-CN')
	}

	const openTaskNameEditor = (task: WebScanTaskSummary) => {
		if (!onUpdateTaskName || taskNameUpdatePendingRef.current) return
		taskNameEditCancelledRef.current = false
		setEditingTask(task)
		setEditedTaskName(task.taskName || '')
	}

	const closeTaskNameEditor = () => {
		if (taskNameUpdatePendingRef.current) return
		setEditingTask(null)
		setEditedTaskName('')
	}

	const handleUpdateTaskName = async () => {
		if (taskNameEditCancelledRef.current || taskNameUpdatePendingRef.current || !editingTask || !onUpdateTaskName) return
		const taskName = editedTaskName.trim()
		if (!taskName || taskName === editingTask.taskName) {
			closeTaskNameEditor()
			return
		}

		taskNameUpdatePendingRef.current = true
		setIsUpdatingTaskName(true)
		let updated = false
		try {
			updated = await onUpdateTaskName(editingTask.id, taskName)
		} finally {
			taskNameUpdatePendingRef.current = false
			setIsUpdatingTaskName(false)
			if (updated) {
				closeTaskNameEditor()
			} else {
				requestAnimationFrame(() => taskNameInputRef.current?.focus())
			}
		}
	}

	const formatTaskSchedule = (task: WebScanTaskSummary) => {
		if (!task.scheduleSpec || task.scheduleEnabled === false) {
			return task.scanType === ScanType.Combination ? t.vulnScan.newScan.scheduleModes.manual : null
		}
		return formatSchedule(task.scheduleSpec, t)
	}

	// 处理筛选变化
	const handleFilterChange = (newFilters: { search?: string; status?: string; scanMode?: string }) => {
		onFilterChange?.(newFilters)
	}

	useEffect(() => () => {
		if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current)
	}, [])

	// 处理搜索输入（带防抖）
	const handleSearchChange = (value: string) => {
		setSearchQuery(value)
		if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current)
		searchTimeoutRef.current = setTimeout(() => {
			handleFilterChange({
				search: value || undefined,
				status: statusFilter !== 'all' ? statusFilter : undefined,
				scanMode: typeFilter !== 'all' ? typeFilter : undefined,
			})
		}, 300)
	}

	// 处理状态筛选变化
	const handleStatusFilterChange = (value: string) => {
		if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current)
		setStatusFilter(value)
		handleFilterChange({
			search: searchQuery || undefined,
			status: value !== 'all' ? value : undefined,
			scanMode: typeFilter !== 'all' ? typeFilter : undefined,
		})
	}

	// 处理类型筛选变化
	const handleTypeFilterChange = (value: string) => {
		if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current)
		setTypeFilter(value)
		handleFilterChange({
			search: searchQuery || undefined,
			status: statusFilter !== 'all' ? statusFilter : undefined,
			scanMode: value !== 'all' ? value : undefined,
		})
	}

	const handleViewScanHistory = (taskId: string) => {
		// 导航到扫描历史页面
		const params = new URLSearchParams()
		params.set('taskId', taskId)
		router.push(`/scan/vuln-scan/history?${params.toString()}`)
	}

	const handleSelectRow = (taskId: string, checked: boolean) => {
		const newSelected = new Set(selectedRows)
		if (checked) {
			newSelected.add(taskId)
		} else {
			newSelected.delete(taskId)
		}
		setSelectedRows(newSelected)
	}

	const isTaskOperationLocked = (task: WebScanTaskSummary) =>
		task.status === 'running' || task.status === 'recovering' || task.status === 'deleting'

	const handleSelectAll = (checked: boolean) => {
		if (checked) {
			// 只选择当前页且不在运行/恢复/删除中的任务
			const selectableTaskIds = displayTasks
				.filter(task => !isTaskOperationLocked(task))
				.map(task => task.id)
			setSelectedRows(new Set(selectableTaskIds))
		} else {
			setSelectedRows(new Set())
		}
	}

	const handleBatchDelete = () => {
		if (selectedRows.size === 0) return
		onBatchDeleteTasks?.(Array.from(selectedRows))
		setSelectedRows(new Set())
		setBatchDeletePopoverOpen(false)
	}

	const handleConfirmDeleteTask = (taskId: string) => {
		onDeleteTask?.(taskId)
		setConfirmDeleteTaskId(null)
	}

	// 计算全选状态
	const selectableTasksOnPage = displayTasks.filter(task => !isTaskOperationLocked(task))
	const allSelected = selectableTasksOnPage.length > 0 && selectableTasksOnPage.every(task => selectedRows.has(task.id))
	const someSelected = selectableTasksOnPage.some(task => selectedRows.has(task.id))

	// 计算风险等级
	const getRiskLevel = (task: WebScanTaskSummary): { level: RiskLevel, label: string, color: string } => {
		const critical = task.criticalCount || 0
		const high = task.highCount || 0
		const medium = task.mediumCount || 0

		if (critical > 0) {
			return { level: 'critical', label: t.vulnScan.severity.critical, color: getRiskLevelBadgeClassName('critical') }
		}
		if (high > 0) {
			return { level: 'high', label: t.vulnScan.severity.high, color: getRiskLevelBadgeClassName('high') }
		}
		if (medium > 0) {
			return { level: 'medium', label: t.vulnScan.severity.medium, color: getRiskLevelBadgeClassName('medium') }
		}
		if ((task.lowCount || 0) > 0 || (task.infoCount || 0) > 0) {
			return { level: 'low', label: t.vulnScan.severity.low, color: getRiskLevelBadgeClassName('low') }
		}
		return { level: 'none', label: t.vulnScan.taskManagement.noRisk, color: getRiskLevelBadgeClassName('none') }
	}

	// 格式化漏洞统计
	const formatVulnStats = (task: WebScanTaskSummary) => {
		if (task.status !== 'completed' || !task.vulnCount) {
			return '-'
		}
		const parts: string[] = []
		if (task.criticalCount) parts.push(`${t.vulnScan.severity.critical}:${task.criticalCount}`)
		if (task.highCount) parts.push(`${t.vulnScan.severity.high}:${task.highCount}`)
		if (task.mediumCount) parts.push(`${t.vulnScan.severity.medium}:${task.mediumCount}`)
		if (task.lowCount) parts.push(`${t.vulnScan.severity.low}:${task.lowCount}`)
		if (task.infoCount) parts.push(`${t.vulnScan.severity.info}:${task.infoCount}`)
		return parts.length > 0 ? parts.join(' ') : '0'
	}


	const updateRightFixedShadow = useCallback(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) {
			setShowRightFixedShadow(false)
			return
		}

		const maxScrollLeft = el.scrollWidth - el.clientWidth
		setShowRightFixedShadow(maxScrollLeft > 1 && el.scrollLeft < maxScrollLeft - 1)
	}, [])

	useEffect(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) return

		updateRightFixedShadow()
		el.addEventListener('scroll', updateRightFixedShadow, { passive: true })
		window.addEventListener('resize', updateRightFixedShadow)

		return () => {
			el.removeEventListener('scroll', updateRightFixedShadow)
			window.removeEventListener('resize', updateRightFixedShadow)
		}
	}, [displayTasks.length, isMobile, updateRightFixedShadow])

	const getDisplayTaskStatus = (task: WebScanTaskSummary) => {
		if (task.pauseRequested && (task.status === 'running' || task.status === 'recovering')) {
			return 'pausing'
		}
		return task.status
	}

	const isPauseInFlight = (task: WebScanTaskSummary) =>
		Boolean(task.pauseRequested && (task.status === 'running' || task.status === 'recovering'))

	const isTaskInProgress = (task: WebScanTaskSummary) => {
		const status = getDisplayTaskStatus(task)
		return status === 'running' || status === 'recovering' || status === 'pausing'
	}

  const activeFilterCount = Number(Boolean(searchQuery.trim())) + Number(statusFilter !== 'all') + Number(typeFilter !== 'all')
  const filterControls = <>
    <Select
      value={statusFilter}
      onValueChange={handleStatusFilterChange}
    >
      <SelectTrigger className={cn(isMobile ? "h-11 w-full" : "w-[140px]")}>
        <SelectValue placeholder={t.vulnScan.taskManagement.allStatus} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t.vulnScan.taskManagement.allStatus}</SelectItem>
        <SelectItem value="pending">{t.vulnScan.statuses.pending}</SelectItem>
        <SelectItem value="running">{t.vulnScan.statuses.running}</SelectItem>
        <SelectItem value="paused">{t.vulnScan.statuses.paused}</SelectItem>
        <SelectItem value="recovering">{t.vulnScan.statuses.recovering}</SelectItem>
        <SelectItem value="deleting">{t.vulnScan.statuses.deleting}</SelectItem>
        <SelectItem value="completed">{t.vulnScan.statuses.completed}</SelectItem>
        <SelectItem value="failed">{t.vulnScan.statuses.failed}</SelectItem>
        <SelectItem value="cancelled">{t.vulnScan.statuses.cancelled}</SelectItem>
      </SelectContent>
    </Select>
    <Select
      value={typeFilter}
      onValueChange={handleTypeFilterChange}
    >
      <SelectTrigger className={cn(isMobile ? "h-11 w-full" : "w-[140px]")}>
        <SelectValue placeholder={t.vulnScan.taskManagement.allTypes} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{t.vulnScan.taskManagement.allTypes}</SelectItem>
        <SelectItem value={ScanType.Subdomain}>{t.vulnScan.taskManagement.subdomain}</SelectItem>
        <SelectItem value={ScanType.WebScan}>{t.vulnScan.taskManagement.webscan}</SelectItem>
        <SelectItem value={ScanType.PortScan}>{t.vulnScan.taskManagement.portscan}</SelectItem>
        <SelectItem value={ScanType.Crack}>{t.vulnScan.taskManagement.crack}</SelectItem>
        <SelectItem value={ScanType.DirScan}>{t.vulnScan.taskManagement.dirscan}</SelectItem>
        <SelectItem value={ScanType.JsScan}>{t.vulnScan.taskManagement.jsscan}</SelectItem>
        <SelectItem value={ScanType.Combination}>{t.vulnScan.taskManagement.combination}</SelectItem>
      </SelectContent>
    </Select></>
  const renderTaskName = (task: WebScanTaskSummary) => <><div className={cn('flex min-w-0 items-center gap-1', !isMobile && 'max-w-[320px] justify-center mx-auto', editingTask?.id === task.id && 'w-full')}>
      {task.createdByAI && editingTask?.id !== task.id && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span
              role="img"
              aria-label={t.vulnScan.taskManagement.aiCreated}
              tabIndex={0}
              className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground/70 hover:text-muted-foreground focus-visible:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <Bot className="h-3.5 w-3.5" aria-hidden="true" />
            </span>
          </TooltipTrigger>
          <TooltipContent>{t.vulnScan.taskManagement.aiCreated}</TooltipContent>
        </Tooltip>
      )}
      {editingTask?.id === task.id ? (
        <Input
          ref={taskNameInputRef}
          value={editedTaskName}
          onChange={(event) => setEditedTaskName(event.target.value)}
          onBlur={() => void handleUpdateTaskName()}
          onKeyDown={(event) => {
            if (event.nativeEvent.isComposing) return
            if (event.key === 'Enter') {
              event.preventDefault()
              void handleUpdateTaskName()
            } else if (event.key === 'Escape') {
              event.preventDefault()
              if (taskNameUpdatePendingRef.current) return
              taskNameEditCancelledRef.current = true
              closeTaskNameEditor()
            }
          }}
          aria-label={t.vulnScan.taskManagement.editTaskName}
          autoFocus
          readOnly={isUpdatingTaskName}
          className="h-11 md:h-8 min-w-0 flex-1 px-2 text-base md:text-sm"
        />
      ) : (
        <>
          <span
            className={cn("min-w-0 text-sm font-medium", isMobile ? "break-all" : "truncate")}
            title={task.taskName || `${t.dashboard.recentTasks.task} ${task.id}`}
          >
            {task.taskName || task.id}
          </span>
          {onUpdateTaskName && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={t.vulnScan.taskManagement.editTaskName}
                  onClick={() => openTaskNameEditor(task)}
                  className="inline-flex h-11 w-11 md:h-5 md:w-5 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-opacity hover:text-foreground focus-visible:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:opacity-0 sm:group-hover:opacity-100 sm:group-hover:text-foreground sm:group-focus-within:opacity-100"
                >
                  <Pencil className="h-3.5 w-3.5" aria-hidden="true" />
                </button>
              </TooltipTrigger>
              <TooltipContent>{t.vulnScan.taskManagement.editTaskName}</TooltipContent>
            </Tooltip>
          )}
        </>
      )}
    </div>
    {formatTaskSchedule(task) && (
      <div className="mt-1 text-xs text-muted-foreground truncate max-w-[320px] mx-auto" title={formatTaskSchedule(task) || undefined}>
        {t.vulnScan.taskManagement.schedule}: {formatTaskSchedule(task)}
      </div>
    )}
    {task.nextRunAt && (
      <div className="mt-0.5 text-[11px] text-muted-foreground truncate max-w-[320px] mx-auto" title={formatScheduleTimestamp(task.nextRunAt)}>
        {t.vulnScan.taskManagement.nextRunAt}: {formatScheduleTimestamp(task.nextRunAt)}
      </div>
    )}</>
  const taskTypes = {
    [ScanType.Subdomain]: { icon: Captions, label: t.vulnScan.taskManagement.subdomain },
    [ScanType.WebScan]: { icon: Globe, label: t.vulnScan.taskManagement.webscan },
    [ScanType.PortScan]: { icon: Server, label: t.vulnScan.taskManagement.portscan },
    [ScanType.Crack]: { icon: Target, label: t.vulnScan.taskManagement.crack },
    [ScanType.DirScan]: { icon: FolderSearch, label: t.vulnScan.taskManagement.dirscan },
    [ScanType.JsScan]: { icon: FileCode, label: t.vulnScan.taskManagement.jsscan },
    [ScanType.Combination]: { icon: Layers, label: t.vulnScan.taskManagement.combination },
  }
  const renderTaskType = (task: WebScanTaskSummary) => {
    const { icon: Icon, label } = taskTypes[task.scanType] || taskTypes[ScanType.WebScan]
    return <div className="flex justify-center"><Badge variant="outline" className="flex w-fit items-center gap-1"><Icon className="h-3 w-3 shrink-0" />{label}</Badge></div>
  }
  const mobileDeleteTask = displayTasks.find(task => task.id === confirmDeleteTaskId)
  const mobileTaskCards = isMobile ? <div className="space-y-3" data-slot="mobile-task-list">
    {displayTasks.map(task => <article key={task.id} aria-label={task.taskName || task.id} className={cn('min-w-0 space-y-3 rounded-xl border bg-card p-4', selectedRows.has(task.id) ? 'border-primary/50 bg-muted/40' : 'border-border')}>
      <div className="flex items-start gap-2">
        {mobileSelecting && <label className="flex h-11 w-8 shrink-0 items-center justify-start"><span className="sr-only">{t.common.select} {task.taskName || task.id}</span><Checkbox aria-label={`${t.common.select} ${task.taskName || task.id}`} checked={selectedRows.has(task.id)} disabled={isTaskOperationLocked(task)} onCheckedChange={checked => handleSelectRow(task.id, checked === true)} /></label>}
        <div className="min-w-0 flex-1">{renderTaskName(task)}</div>
      </div>
      <div className="flex flex-wrap items-center gap-2">{renderTaskType(task)}{getStatusBadge(getDisplayTaskStatus(task), taskStatusLabels)}{task.status === 'completed' && Boolean(task.vulnCount) ? <Badge variant="outline" className={getRiskLevel(task).color}>{getRiskLevel(task).label} · {task.vulnCount}</Badge> : null}</div>
      <div className="flex items-center gap-3"><Progress value={task.progress} className="h-2 flex-1" aria-label={t.vulnScan.progress} /><span className="text-xs tabular-nums text-muted-foreground">{task.progress}%</span></div>
      {task.stageEvents?.slice(-1).map((event) => (
        <p key={event.stage} className="wrap-anywhere text-xs text-muted-foreground">
          {scanStageLabel(event, t.vulnScan.scanStages)}
        </p>
      ))}
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <dt>{t.vulnScan.createTime}</dt><dd className="text-right">{formatDate(task.createdAt)}</dd>
        {task.completedAt ? <><dt>{t.vulnScan.completedTime}</dt><dd className="text-right">{formatDate(task.completedAt)}</dd></> : null}
      </dl>
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-2">
        <Button variant="ghost" className="h-11 px-0 text-xs text-muted-foreground" onClick={() => handleViewScanHistory(task.id)} aria-label={t.vulnScan.taskManagement.historyResults}>{t.vulnScan.taskManagement.scanCountFormat.replace('{n}', String(task.version || 1))}</Button>
        <div className="flex items-center gap-1">
          <Button variant="outline" className="h-11 gap-2" disabled={task.status === 'deleting'} onClick={() => onViewTask?.(task.id)}><Eye className="h-4 w-4" />{t.vulnScan.taskManagement.viewDetail}</Button>
          <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon" className="h-11 w-11" aria-label={t.vulnScan.taskManagement.moreActions}><MoreHorizontal className="h-5 w-5" /></Button></DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-48 [&_[role=menuitem]]:min-h-11">
              {task.status === 'pending' ? <DropdownMenuItem onSelect={() => onStartTask?.(task.id)}><Play />{t.vulnScan.taskManagement.startTask}</DropdownMenuItem> : null}
              {task.status === 'running' && !isPauseInFlight(task) ? <DropdownMenuItem onSelect={() => onPauseTask?.(task.id)}><Pause />{t.vulnScan.taskManagement.pauseTask}</DropdownMenuItem> : null}
              {isPauseInFlight(task) ? <DropdownMenuItem disabled><Loader2 className="animate-spin" />{t.vulnScan.statuses.pausing}</DropdownMenuItem> : null}
              {(task.status === 'paused' || task.status === 'recovering') && !isPauseInFlight(task) ? <DropdownMenuItem onSelect={() => onResumeTask?.(task.id)}><Play />{t.vulnScan.taskManagement.resumeTask}</DropdownMenuItem> : null}
              {['running', 'recovering', 'paused'].includes(task.status) ? <DropdownMenuItem onSelect={() => onStopTask?.(task.id)}><Square />{t.vulnScan.taskManagement.stopTask}</DropdownMenuItem> : null}
              {['failed', 'cancelled', 'completed'].includes(task.status) ? <DropdownMenuItem onSelect={() => onStartTask?.(task.id)}><RotateCcw />{t.vulnScan.taskManagement.restartTask}</DropdownMenuItem> : null}
              <DropdownMenuSeparator />
              <DropdownMenuItem disabled={isTaskOperationLocked(task)} className="text-destructive focus:text-destructive" onSelect={() => setConfirmDeleteTaskId(task.id)}><Trash2 />{t.vulnScan.taskManagement.deleteTask}</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </article>)}
  </div> : null

	return (
		<TooltipProvider delayDuration={200}>
			<div className="min-w-0 space-y-4">
				{/* 操作栏 */}
        {isMobile && <MobileListToolbar
          search={searchQuery}
          searchPlaceholder={t.vulnScan.taskManagement.searchPlaceholder}
          onSearchChange={handleSearchChange}
          filters={filterControls}
          activeFilterCount={activeFilterCount}
          onResetFilters={() => {
            if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current)
            setSearchQuery(''); setStatusFilter('all'); setTypeFilter('all')
            handleFilterChange({ search: undefined, status: undefined, scanMode: undefined })
          }}
          createLabel={t.vulnScan.newTask}
          onCreate={() => setCreateDialogOpen(true)}
          selecting={mobileSelecting}
          onSelectingChange={value => { setMobileSelecting(value); setSelectedRows(new Set()) }}
          selectedCount={selectedRows.size}
          allSelected={allSelected}
          someSelected={someSelected}
          canSelect={selectableTasksOnPage.length > 0}
          onSelectAll={handleSelectAll}
          onDelete={() => setBatchDeletePopoverOpen(true)}
        />}
				{!isMobile && <div className="flex flex-wrap items-center justify-between gap-3">
					<div className="flex min-w-0 basis-full items-center gap-2 md:basis-auto md:gap-4 md:flex-1">
						<div className="relative min-w-0 flex-1 md:max-w-md">
							<Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-muted-foreground" />
							<Input
								placeholder={t.vulnScan.taskManagement.searchPlaceholder}
								value={searchQuery}
								onChange={(e) => handleSearchChange(e.target.value)}
								className="h-11 pl-9 text-base md:h-9 md:text-sm"
							/>
						</div>
{filterControls}
					</div>
					<div className="flex w-full shrink-0 flex-wrap items-center justify-end gap-2 md:ml-2 md:w-auto">
						{selectedRows.size > 0 && (
							<Popover open={batchDeletePopoverOpen} onOpenChange={setBatchDeletePopoverOpen}>
								<PopoverTrigger asChild>
									<Button
										variant="outline"
										className="border-destructive/30 bg-destructive/10 text-destructive hover:bg-destructive/15 hover:text-destructive"
									>
										<Trash2 className="w-4 h-4 mr-2" />
										{t.common.delete} ({selectedRows.size})
									</Button>
								</PopoverTrigger>
								<PopoverContent
									align="end"
									side="bottom"
									sideOffset={10}
									className="w-80 rounded-2xl border-destructive/20 bg-background/95 p-0 shadow-2xl shadow-black/20 backdrop-blur-xl"
								>
									<div className="flex items-start gap-3 px-4 py-3">
										<div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-destructive/20 bg-destructive/10 text-destructive">
											<AlertTriangle className="h-4 w-4" />
										</div>
										<div className="min-w-0 flex-1">
											<p className="text-sm font-semibold text-foreground">确认批量删除？</p>
											<p className="mt-1 text-xs leading-5 text-muted-foreground">
												将删除 {selectedRows.size} 个任务，删除后不可恢复。
											</p>
										</div>
									</div>
									<div className="flex items-center justify-end gap-2 border-t border-border/60 px-4 py-2.5">
										<Button
											variant="outline"
											size="sm"
											onClick={() => setBatchDeletePopoverOpen(false)}
										>
											{t.common.cancel}
										</Button>
										<Button
											variant="outline"
											size="sm"
											className="border-destructive/35 bg-destructive/10 text-destructive hover:bg-destructive/15 hover:text-destructive"
											onClick={handleBatchDelete}
										>
											删除选中
										</Button>
									</div>
								</PopoverContent>
							</Popover>
						)}
							<Button className="h-11 md:h-9" onClick={() => setCreateDialogOpen(true)}>
								<Plus className="w-4 h-4 mr-2" />
								{t.vulnScan.newTask}
							</Button>
					</div>
				</div>}

				{/* 任务列表 */}
				{displayTasks.length === 0 ? (
					<Card className="p-12 text-center">
						<Clock className="w-12 h-12 mx-auto mb-4 text-muted-foreground opacity-50" />
						<p className="text-muted-foreground">{t.vulnScan.taskManagement.noTasks}</p>
					</Card>
				) : isMobile ? mobileTaskCards : (
					<div ref={tableHostRef} className="border rounded-lg overflow-x-auto">
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead className="w-12 pl-6">
										<Checkbox
											checked={allSelected ? true : someSelected ? 'indeterminate' : false}
											onCheckedChange={handleSelectAll}
										/>
									</TableHead>
									<TableHead className="w-[300px] text-center">{t.vulnScan.taskName}</TableHead>
									<TableHead className="w-[100px] text-center">{t.vulnScan.taskManagement.type}</TableHead>
									<TableHead className="w-[100px] text-center">{t.vulnScan.taskManagement.riskLevel}</TableHead>
									<TableHead className="w-[100px] text-center">{t.vulnScan.status}</TableHead>
									<TableHead className="w-[120px] text-center">{t.vulnScan.progress}</TableHead>
									<TableHead className="w-[180px] text-center">{t.vulnScan.taskManagement.scanCount}</TableHead>
										<TableHead className="w-[120px] text-center">{t.vulnScan.createTime}</TableHead>
										<TableHead className="w-[120px] text-center">{t.vulnScan.completedTime}</TableHead>
										<TableHead className={cn('w-[180px] text-center sticky right-0 z-20 border-l border-border bg-background', showRightFixedShadow && 'table-fixed-column-shadow-left')}>
										{t.vulnScan.action}
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{displayTasks.map((task) => (
									<TableRow
										key={task.id}
										className={cn(
											'group transition-colors duration-200 hover:bg-muted',
											isTaskInProgress(task) && 'bg-primary/[0.025]',
										)}
									>
										<TableCell className="pl-6">
											<Checkbox
												checked={selectedRows.has(task.id)}
												disabled={isTaskOperationLocked(task)}
												onCheckedChange={(checked) => handleSelectRow(task.id, checked === true)}
											/>
										</TableCell>
										<TableCell className="text-center">
											{renderTaskName(task)}
										</TableCell>
										<TableCell className="text-center">
											{renderTaskType(task)}
										</TableCell>
										<TableCell className="text-center">
											{task.status === 'completed' ? (
												<div className="flex justify-center">
													{(() => {
														// PortScan 不显示风险等级
														if (task.scanType === ScanType.Subdomain || task.scanType === ScanType.PortScan || task.scanType === ScanType.DirScan || task.scanType === ScanType.Combination) {
															return <span className="text-sm text-muted-foreground">-</span>
														}
														// WebScan、Crack、JsScan、DirScan 都显示风险等级
														const risk = getRiskLevel(task)
														return (
															<Badge variant="outline" className={`${risk.color} border`}>
																<AlertTriangle className="w-3 h-3 mr-1" />
																{risk.label}
															</Badge>
														)
													})()}
												</div>
											) : (
												<span className="text-sm text-muted-foreground">-</span>
											)}
										</TableCell>
										<TableCell className="text-center">
											<div className={cn('flex justify-center', isTaskInProgress(task) && 'scan-status-active')}>
												{getStatusBadge(getDisplayTaskStatus(task), taskStatusLabels)}
											</div>
										</TableCell>
										<TableCell className="text-center">
										<Tooltip>
											<TooltipTrigger asChild>
												<div className="flex items-center gap-2 justify-center cursor-default">
													<div className="w-[80px]">
														<Progress
															value={task.progress}
															className={cn('h-2 w-full', isTaskInProgress(task) && 'scan-progress-active')}
														/>
													</div>
													<div className="text-xs text-muted-foreground whitespace-nowrap w-[35px] text-left">
														{task.progress}%
													</div>
												</div>
											</TooltipTrigger>
											<ScanStageTooltipContent events={task.stageEvents} />
										</Tooltip>
									</TableCell>
										<TableCell className="text-center">
											<Tooltip>
												<TooltipTrigger asChild>
													<button
														type="button"
														className="inline-flex h-7 items-center rounded-md border border-border/70 bg-muted/20 px-2.5 text-xs font-medium text-foreground/80 transition-colors hover:border-primary/35 hover:bg-muted/40 hover:text-foreground"
														onClick={() => handleViewScanHistory(task.id)}
														aria-label={t.vulnScan.taskManagement.historyResults}
													>
														{t.vulnScan.taskManagement.scanCountFormat.replace('{n}', String(task.version || 1))}
													</button>
												</TooltipTrigger>
												<TooltipContent>{t.vulnScan.taskManagement.historyHint}</TooltipContent>
											</Tooltip>
										</TableCell>
										<TableCell className="text-sm text-muted-foreground text-center">
											{formatDate(task.createdAt)}
										</TableCell>
										<TableCell className="text-sm text-muted-foreground text-center">
											{formatDate(task.completedAt)}
										</TableCell>
										<TableCell className={cn('text-center sticky right-0 z-10 border-l border-border bg-background group-hover:bg-muted group-data-[state=selected]:bg-muted', showRightFixedShadow && 'table-fixed-column-shadow-left')}>
											<div className="flex items-center justify-center gap-2">
												{/* 任务控制按钮（仅图标，文字通过 Tooltip 显示） */}
												{task.status === 'pending' && (
													<Tooltip>
														<TooltipTrigger asChild>
															<Button
																variant="ghost"
																size="icon"
																onClick={() => onStartTask?.(task.id)}
																aria-label={t.vulnScan.taskManagement.startTask}
															>
																<Play className="w-4 h-4" />
															</Button>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.taskManagement.startTask}</TooltipContent>
													</Tooltip>
												)}
												{task.status === 'running' && !isPauseInFlight(task) && (
													<Tooltip>
														<TooltipTrigger asChild>
															<Button
																variant="ghost"
																size="icon"
																onClick={() => onPauseTask?.(task.id)}
																aria-label={t.vulnScan.taskManagement.pauseTask}
															>
																<Pause className="w-4 h-4" />
															</Button>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.taskManagement.pauseTask}</TooltipContent>
													</Tooltip>
												)}
												{isPauseInFlight(task) && (
													<Tooltip>
														<TooltipTrigger asChild>
															<Button
																variant="ghost"
																size="icon"
																disabled
																aria-label={t.vulnScan.statuses.pausing}
															>
																<Loader2 className="w-4 h-4 animate-spin" />
															</Button>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.statuses.pausing}</TooltipContent>
													</Tooltip>
												)}
												{(task.status === 'paused' || task.status === 'recovering') && !isPauseInFlight(task) && (
													<Tooltip>
														<TooltipTrigger asChild>
															<Button
																variant="ghost"
																size="icon"
																onClick={() => onResumeTask?.(task.id)}
																aria-label={t.vulnScan.taskManagement.resumeTask}
															>
																<Play className="w-4 h-4" />
															</Button>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.taskManagement.resumeTask}</TooltipContent>
													</Tooltip>
												)}
												{(task.status === 'running' || task.status === 'recovering' || task.status === 'paused') && (
													<Tooltip>
														<TooltipTrigger asChild>
															<Button
																variant="ghost"
																size="icon"
																onClick={() => onStopTask?.(task.id)}
																aria-label={t.vulnScan.taskManagement.stopTask}
															>
																<Square className="w-4 h-4" />
															</Button>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.taskManagement.stopTask}</TooltipContent>
													</Tooltip>
												)}
												{(task.status === 'failed' || task.status === 'cancelled' || task.status === 'completed') && (
													<Tooltip>
														<TooltipTrigger asChild>
															<Button
																variant="ghost"
																size="icon"
																onClick={() => onStartTask?.(task.id)}
																aria-label={t.vulnScan.taskManagement.restartTask}
															>
																<RotateCcw className="w-4 h-4" />
															</Button>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.taskManagement.restartTask}</TooltipContent>
													</Tooltip>
												)}

												<Tooltip>
													<TooltipTrigger asChild>
														<Button
															variant="ghost"
															size="icon"
															onClick={() => {
																onViewTask?.(task.id)
															}}
															disabled={task.status === 'deleting'}
															aria-label={t.vulnScan.taskManagement.viewDetail}
														>
															<Eye className="w-4 h-4" />
														</Button>
													</TooltipTrigger>
													<TooltipContent>{t.vulnScan.taskManagement.viewDetail}</TooltipContent>
												</Tooltip>
												<Popover
													open={confirmDeleteTaskId === task.id}
													onOpenChange={(open) => setConfirmDeleteTaskId(open ? task.id : null)}
												>
													<Tooltip>
														<TooltipTrigger asChild>
															<PopoverTrigger asChild>
																<Button
																	variant="ghost"
																	size="icon"
																	onClick={() => setConfirmDeleteTaskId(task.id)}
																	disabled={isTaskOperationLocked(task)}
																	aria-label={t.vulnScan.taskManagement.deleteTask}
																>
																	<Trash2 className="w-4 h-4" />
																</Button>
															</PopoverTrigger>
														</TooltipTrigger>
														<TooltipContent>{t.vulnScan.taskManagement.deleteTask}</TooltipContent>
													</Tooltip>
													<PopoverContent
														align="end"
														side="top"
														sideOffset={8}
														className="w-80 rounded-2xl border-destructive/15 bg-background/95 p-0 shadow-2xl shadow-black/20 backdrop-blur-xl"
													>
														<div>
															<div className="flex items-start gap-3 px-4 py-3">
																<div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-destructive/20 bg-destructive/10 text-destructive">
																	<AlertTriangle className="h-4 w-4" />
																</div>
																<div className="min-w-0 flex-1">
																	<p className="text-sm font-semibold text-foreground">确认删除任务？</p>
																	<p className="mt-1 text-xs leading-5 text-muted-foreground break-all">
																	删除后不可恢复：{task.taskName || task.id}
																</p>
																</div>
															</div>
															<div className="flex items-center justify-end gap-2 border-t border-border/60 px-4 py-2.5">
																<Button
																	variant="outline"
																	size="sm"
																	onClick={() => setConfirmDeleteTaskId(null)}
																>
																	{t.common.cancel}
																</Button>
																<Button
																	variant="outline"
																	size="sm"
																	className="border-destructive/35 bg-destructive/10 text-destructive hover:bg-destructive/15 hover:text-destructive"
																	onClick={() => handleConfirmDeleteTask(task.id)}
																>
																	删除
																</Button>
															</div>
														</div>
													</PopoverContent>
												</Popover>
											</div>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
							</Table>
					</div>
				)}

				{/* 分页 */}
				{total > 0 && (
					<PaginationPro
						current={propCurrentPage}
						total={total}
						pageSize={propPageSize}
						onChange={(page, size) => {
							onPageChange?.(page, size)
						}}
						onPageSizeChange={(size) => {
							onPageChange?.(1, size)
						}}
						pageSizeOptions={[10, 20, 50, 100]}
						showSizeChanger
						middlePageCount={isMobile ? 1 : 5}
						showQuickJumper={!isMobile}
						showTotal
					/>
				)}
        {isMobile && <AlertDialog open={batchDeletePopoverOpen} onOpenChange={setBatchDeletePopoverOpen}>
          <AlertDialogContent>
            <AlertDialogHeader><AlertDialogTitle>{t.mobileList.deleteTitle}</AlertDialogTitle><AlertDialogDescription>{t.mobileList.deleteDescription.replace('{count}', String(selectedRows.size))}</AlertDialogDescription></AlertDialogHeader>
            <AlertDialogFooter><AlertDialogCancel>{t.common.cancel}</AlertDialogCancel><AlertDialogAction disabled={!selectedRows.size} className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={handleBatchDelete}>{t.common.delete}</AlertDialogAction></AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>}
        {isMobile ? <AlertDialog open={Boolean(mobileDeleteTask)} onOpenChange={open => { if (!open) setConfirmDeleteTaskId(null) }}>
          <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t.vulnScan.taskManagement.deleteTask}</AlertDialogTitle><AlertDialogDescription className="break-all">{t.vulnScan.taskManagement.deleteConfirmation} {mobileDeleteTask?.taskName || mobileDeleteTask?.id}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{t.common.cancel}</AlertDialogCancel><AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={() => mobileDeleteTask && handleConfirmDeleteTask(mobileDeleteTask.id)}>{t.common.delete}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
        </AlertDialog> : null}
				<CreateTaskDialog
					open={createDialogOpen}
					onOpenChange={setCreateDialogOpen}
					onSelectType={(type) => {
						onCreateTask?.(type)
					}}
					onSelectCombination={onOpenCombinationTask}
				/>
			</div>
		</TooltipProvider>
	)
}
