'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useIsMobile } from '@/hooks/use-mobile'
import { MobileListToolbar } from '@/components/ui/mobile-list-toolbar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { getStatusBadge as renderStatusBadge } from '@/lib/status-badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { MultiSelectFilter } from '@/components/ui/multi-select-filter'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Progress } from '@/components/ui/progress'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { useI18n } from '@/lib/i18n'
import {
  Search,
  Clock,
  Trash2,
  Eye,
  X,
  Loader2,
  Plus,
  BarChart3,
} from 'lucide-react'

// 任务状态类型
export interface CompanyInfoTask {
  id: string
  taskName: string
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  progress: number
  totalCount: number
  currentCount: number
  config?: string // JSON 格式的任务配置
  errorMsg?: string
  startedAt?: string
  completedAt?: string
  createdAt: string
  updatedAt: string
}

interface TaskManagementViewProps {
  tasks: CompanyInfoTask[]
  loading?: boolean
  onDeleteTask?: (taskId: string) => void
  onBatchDeleteTasks?: (taskIds: string[]) => void
  onViewTask?: (taskId: string) => void
  onCancelTask?: (taskId: string) => void
  onLoadTestData?: () => void
  onCreateTask?: () => void
}

export function TaskManagementView({
  tasks = [],
  loading = false,
  onDeleteTask,
  onBatchDeleteTasks,
  onViewTask,
  onCancelTask,
  onLoadTestData,
  onCreateTask,
}: TaskManagementViewProps) {
  const { t } = useI18n()
  const isMobile = useIsMobile()
  const [mobileSelecting, setMobileSelecting] = useState(false)
  const taskStatusLabels = {
    pending: t.companyInfo.taskManagement.pending,
    running: t.companyInfo.taskManagement.running,
    completed: t.companyInfo.taskManagement.completed,
    failed: t.companyInfo.taskManagement.failed,
    cancelled: t.companyInfo.taskManagement.cancelled,
  }
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<string[]>([])
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [selectedTaskIds, setSelectedTaskIds] = useState<Set<string>>(new Set())
  const [batchDeleteDialogOpen, setBatchDeleteDialogOpen] = useState(false)
  const tableHostRef = useRef<HTMLDivElement | null>(null)
  const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)

  // 过滤任务
  const filteredTasks = tasks.filter((task) => {
    if (searchQuery) {
      const query = searchQuery.toLowerCase()
      if (
        !task.id.toLowerCase().includes(query) &&
        !task.taskName.toLowerCase().includes(query)
      ) {
        return false
      }
    }
    if (statusFilter.length > 0 && !statusFilter.includes(task.status)) {
      return false
    }
    return true
  })

  // 分页
  const paginatedTasks = filteredTasks.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize
  )

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
  }, [paginatedTasks.length, updateRightFixedShadow])

  // 格式化时间
  const formatDate = (dateString?: string) => {
    if (!dateString) return '-'
    return new Date(dateString).toLocaleString('zh-CN')
  }

  // 切换任务选择
  const toggleTaskSelection = (taskId: string) => {
    setSelectedTaskIds(prev => {
      const newSet = new Set(prev)
      if (newSet.has(taskId)) {
        newSet.delete(taskId)
      } else {
        newSet.add(taskId)
      }
      return newSet
    })
  }

  // 全选/取消全选
  const toggleSelectAll = () => {
    const selectableTasks = paginatedTasks.filter(t => t.status !== 'running')
    if (selectedTaskIds.size === selectableTasks.length && selectableTasks.length > 0) {
      setSelectedTaskIds(new Set())
    } else {
      setSelectedTaskIds(new Set(selectableTasks.map(t => t.id)))
    }
  }

  // 批量删除
  const handleBatchDelete = () => {
    if (onBatchDeleteTasks && selectedTaskIds.size > 0) {
      onBatchDeleteTasks(Array.from(selectedTaskIds))
      setSelectedTaskIds(new Set())
      setBatchDeleteDialogOpen(false)
    }
  }

  const selectableTasksOnPage = paginatedTasks.filter(task => task.status !== 'running')
  const statusFilterControl = (
          <MultiSelectFilter
            values={statusFilter}
            options={[
              { value: 'pending', label: t.companyInfo.taskManagement.pending },
              { value: 'running', label: t.companyInfo.taskManagement.running },
              { value: 'completed', label: t.companyInfo.taskManagement.completed },
              { value: 'failed', label: t.companyInfo.taskManagement.failed },
              { value: 'cancelled', label: t.companyInfo.taskManagement.cancelled },
            ]}
            allLabel={t.companyInfo.taskManagement.allStatus}
            countLabel={t.common.selectedCount}
            removeLabel={t.common.remove}
            clearLabel={t.common.clear}
            ariaLabel={t.companyInfo.taskManagement.allStatus}
            onChange={(values) => {
              setStatusFilter(values)
              setCurrentPage(1)
            }}
            className={isMobile ? 'h-11 w-full' : 'w-[140px]'}
          />
  )

  return (
    <TooltipProvider delayDuration={200}>
      <div className="space-y-4">
        {/* 操作栏 - 搜索、筛选、按钮在同一行 */}
        {isMobile && <MobileListToolbar
          search={searchQuery}
          searchPlaceholder={t.companyInfo.taskManagement.searchPlaceholder}
          onSearchChange={value => { setSearchQuery(value); setCurrentPage(1) }}
          filters={statusFilterControl}
          activeFilterCount={Number(Boolean(searchQuery.trim())) + Number(statusFilter.length > 0)}
          onResetFilters={() => { setSearchQuery(''); setStatusFilter([]); setCurrentPage(1) }}
          createLabel={t.companyInfo.taskManagement.newTask}
          onCreate={onCreateTask}
          selecting={mobileSelecting}
          onSelectingChange={value => { setMobileSelecting(value); setSelectedTaskIds(new Set()) }}
          selectedCount={selectedTaskIds.size}
          allSelected={selectableTasksOnPage.length > 0 && selectableTasksOnPage.every(task => selectedTaskIds.has(task.id))}
          someSelected={selectableTasksOnPage.some(task => selectedTaskIds.has(task.id))}
          canSelect={!loading && selectableTasksOnPage.length > 0}
          onSelectAll={checked => setSelectedTaskIds(new Set(checked ? selectableTasksOnPage.map(task => task.id) : []))}
          onDelete={() => setBatchDeleteDialogOpen(true)}
        />}
        {!isMobile && <div className="flex flex-wrap items-center gap-3">
          <div className="relative min-w-0 basis-full md:basis-auto flex-1 md:max-w-md">
            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-muted-foreground" />
            <Input
              placeholder={t.companyInfo.taskManagement.searchPlaceholder}
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value)
                setCurrentPage(1)
              }}
              className="pl-9"
            />
          </div>
          {statusFilterControl}
          {selectedTaskIds.size > 0 && (
            <Button
              variant="outline"
              onClick={() => setBatchDeleteDialogOpen(true)}
              className="bg-destructive/10 text-destructive border-destructive/30 hover:bg-destructive/20"
            >
              <Trash2 className="w-4 h-4 mr-2" />
              删除 ({selectedTaskIds.size})
            </Button>
          )}
          <div className="flex items-center gap-2 ml-auto">
            {onLoadTestData && (
              <Button
                variant="outline"
                onClick={onLoadTestData}
              >
                <BarChart3 className="w-4 h-4 mr-2" />
                {t.companyInfo.taskManagement.loadTestData}
              </Button>
            )}
            {onCreateTask && (
              <Button onClick={onCreateTask}>
                <Plus className="w-4 h-4 mr-2" />
                {t.companyInfo.taskManagement.newTask}
              </Button>
            )}
          </div>
        </div>}

        {/* 任务列表 */}
        {loading ? (
          <Card className="p-12 text-center">
            <Loader2 className="w-8 h-8 mx-auto mb-4 animate-spin text-muted-foreground" />
            <p className="text-muted-foreground">{t.companyInfo.taskManagement.loading}</p>
          </Card>
        ) : filteredTasks.length === 0 ? (
          <Card className="p-12 text-center">
            <Clock className="w-12 h-12 mx-auto mb-4 text-muted-foreground opacity-50" />
            <p className="text-muted-foreground">{t.companyInfo.taskManagement.empty}</p>
          </Card>
        ) : (
          <>
          <div className="space-y-3 md:hidden">
            {paginatedTasks.map(task => (
              <article key={task.id} className={cn('space-y-3 rounded-lg border bg-card p-4', selectedTaskIds.has(task.id) && 'border-primary/40 bg-muted/40')}>
                <div className="flex items-start gap-3">{mobileSelecting && <label className="flex min-h-11 items-center"><Checkbox aria-label={task.taskName || task.id} disabled={task.status === 'running'} checked={selectedTaskIds.has(task.id)} onCheckedChange={() => toggleTaskSelection(task.id)} /></label>}<h2 className="min-w-0 flex-1 break-words py-2 text-sm font-medium">{task.taskName || task.id}</h2>{renderStatusBadge(task.status, taskStatusLabels)}</div>
                {task.errorMsg && <p className="break-words text-xs text-destructive">{task.errorMsg}</p>}
                <div className="flex items-center gap-3"><Progress value={task.progress} className="h-2 flex-1" /><span className="text-xs tabular-nums">{task.progress}%</span></div>
                <div className="space-y-1 text-xs text-muted-foreground"><p>{t.companyInfo.taskManagement.count}: {task.currentCount} / {task.totalCount}</p><p>{t.companyInfo.taskManagement.createdAt}: {formatDate(task.createdAt)}</p><p>{t.companyInfo.taskManagement.completedAt}: {formatDate(task.completedAt)}</p></div>
                <div className="flex flex-wrap justify-end gap-2 border-t pt-2">
                  {task.status === 'running' && <Button variant="outline" className="h-11" onClick={() => onCancelTask?.(task.id)}><X className="h-4 w-4" />{t.companyInfo.taskManagement.cancelTask}</Button>}
                  {task.status === 'completed' && <Button variant="outline" className="h-11" onClick={() => onViewTask?.(task.id)}><Eye className="h-4 w-4" />{t.companyInfo.taskManagement.viewResult}</Button>}
                  <Button variant="ghost" className="h-11 text-destructive" disabled={task.status === 'running'} onClick={() => onDeleteTask?.(task.id)}><Trash2 className="h-4 w-4" />{t.companyInfo.taskManagement.deleteTask}</Button>
                </div>
              </article>
            ))}
          </div>
          <div ref={tableHostRef} className="hidden border rounded-lg overflow-hidden md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[50px] text-center">
                    <Checkbox
                      checked={selectedTaskIds.size > 0 && selectedTaskIds.size === paginatedTasks.filter(t => t.status !== 'running').length}
                      onCheckedChange={toggleSelectAll}
                    />
                  </TableHead>
                  <TableHead className="w-[300px] text-center">{t.companyInfo.taskManagement.taskName}</TableHead>
                  <TableHead className="w-[100px] text-center">{t.companyInfo.taskManagement.status}</TableHead>
                  <TableHead className="w-[120px] text-center">{t.companyInfo.taskManagement.progress}</TableHead>
                  <TableHead className="w-[120px] text-center">{t.companyInfo.taskManagement.count}</TableHead>
                  <TableHead className="w-[180px] text-center">{t.companyInfo.taskManagement.createdAt}</TableHead>
                  <TableHead className="w-[180px] text-center">{t.companyInfo.taskManagement.completedAt}</TableHead>
                  <TableHead className={cn('w-[96px] text-center sticky right-0 z-20 border-l border-border bg-background', showRightFixedShadow && 'table-fixed-column-shadow-left')}>{t.companyInfo.taskManagement.actions}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {paginatedTasks.map((task) => (
                  <TableRow key={task.id} className="group hover:bg-muted">
                    <TableCell className="text-center">
                      <Checkbox
                        checked={selectedTaskIds.has(task.id)}
                        onCheckedChange={() => toggleTaskSelection(task.id)}
                        disabled={task.status === 'running'}
                      />
                    </TableCell>
                    <TableCell className="text-center">
                      <div className="space-y-1">
                        <div className="text-sm font-medium truncate max-w-[320px] mx-auto" title={task.taskName || task.id}>
                          {task.taskName || task.id}
                        </div>
                        {task.errorMsg && (
                          <div className="text-xs text-destructive truncate max-w-[300px] mx-auto" title={task.errorMsg}>
                            {t.companyInfo.taskManagement.errorPrefix}: {task.errorMsg}
                          </div>
                        )}
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <div className="flex justify-center">
                        {renderStatusBadge(task.status, taskStatusLabels)}
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <div className="flex items-center gap-2 justify-center">
                        <div className="w-[80px]">
                          <Progress value={task.progress} className="h-2 w-full" />
                        </div>
                        <div className="text-xs text-muted-foreground whitespace-nowrap w-[35px] text-left">
                          {task.progress}%
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <div className="text-sm text-muted-foreground">
                        {task.currentCount} / {task.totalCount}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground text-center">
                      {formatDate(task.createdAt)}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground text-center">
                      {formatDate(task.completedAt)}
                    </TableCell>
                    <TableCell className={cn('w-[96px] text-center sticky right-0 z-10 border-l border-border bg-background group-hover:bg-muted group-data-[state=selected]:bg-muted', showRightFixedShadow && 'table-fixed-column-shadow-left')}>
                      <div className="flex items-center justify-center gap-2">
                        {/* 任务控制按钮 */}
                        {task.status === 'running' && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => onCancelTask?.(task.id)}
                                aria-label={t.companyInfo.taskManagement.cancelTask}
                              >
                                <X className="w-4 h-4" />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>{t.companyInfo.taskManagement.cancelTask}</TooltipContent>
                          </Tooltip>
                        )}
                        {/* 查看 / 删除 */}
                        {task.status === 'completed' && (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant="ghost"
                                size="icon"
                                onClick={() => onViewTask?.(task.id)}
                                aria-label={t.companyInfo.taskManagement.viewResult}
                              >
                                <Eye className="w-4 h-4" />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>{t.companyInfo.taskManagement.viewResult}</TooltipContent>
                          </Tooltip>
                        )}
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => onDeleteTask?.(task.id)}
                              disabled={task.status === 'running'}
                              aria-label={t.companyInfo.taskManagement.deleteTask}
                            >
                              <Trash2 className="w-4 h-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t.companyInfo.taskManagement.deleteTask}</TooltipContent>
                        </Tooltip>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          </>
        )}

        {/* 分页 */}
        {filteredTasks.length > 0 && (
          <PaginationPro
            current={currentPage}
            total={filteredTasks.length}
            pageSize={pageSize}
            onChange={(page, size) => {
              setCurrentPage(page)
              setPageSize(size)
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
        )}
      </div>

      {/* 批量删除确认对话框 */}
      <AlertDialog open={batchDeleteDialogOpen} onOpenChange={setBatchDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.companyInfo.taskManagement.batchDeleteTitle}</AlertDialogTitle>
            <AlertDialogDescription>
              {t.companyInfo.taskManagement.batchDeleteDescription.replace('{count}', String(selectedTaskIds.size))}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t.companyInfo.taskManagement.cancel}</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleBatchDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t.companyInfo.taskManagement.batchDelete}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </TooltipProvider>
  )
}
