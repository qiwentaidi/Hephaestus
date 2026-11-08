'use client'

import { useState, useEffect, useCallback, useRef } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { VideoOff } from 'lucide-react'
import { TaskManagementView } from './components/TaskManagementView'
import type {
  WebScanTask,
  WebScanTaskSummary,
  Activity,
  ScanType,
} from './types'
import { fetchWithAuth } from '@/lib/api'
import toast from 'react-hot-toast'
import { useI18n } from '@/lib/i18n'

// 分页和筛选参数类型
interface TaskQueryParams {
  page: number
  pageSize: number
  search?: string
  status?: string
  scanMode?: string
}

export default function WebScanPage() {
  const { t } = useI18n()
  const router = useRouter()
  const [isRunning, setIsRunning] = useState(false)
  const [currentTask, setCurrentTask] = useState<WebScanTask | null>(null)
  const [tasks, setTasks] = useState<WebScanTaskSummary[]>([])
  const [activities, setActivities] = useState<Activity[]>([])
  const [pollingTimer, setPollingTimer] = useState<NodeJS.Timeout | null>(null)
  
  // 分页状态
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  
  // 筛选状态
  const [filters, setFilters] = useState<{ search?: string; status?: string; scanMode?: string }>({})
  const queryStateRef = useRef({
    currentPage: 1,
    pageSize: 20,
    filters: {} as { search?: string; status?: string; scanMode?: string },
  })

  useEffect(() => {
    queryStateRef.current = { currentPage, pageSize, filters }
  }, [currentPage, filters, pageSize])

  const fetchTasksPage = useCallback(async (params?: Partial<TaskQueryParams>) => {
    const queryState = queryStateRef.current
    const queryParams = new URLSearchParams()
    const page = params?.page ?? queryState.currentPage
    const size = params?.pageSize ?? queryState.pageSize
    // An explicitly cleared filter must not fall back to the previous query.
    const search = params && 'search' in params ? params.search : queryState.filters.search
    const status = params && 'status' in params ? params.status : queryState.filters.status
    const scanMode = params && 'scanMode' in params ? params.scanMode : queryState.filters.scanMode

    queryParams.set('page', String(page))
    queryParams.set('pageSize', String(size))
    if (search) queryParams.set('search', search)
    if (status) queryParams.set('status', status)
    if (scanMode) queryParams.set('scanMode', scanMode)

    const response = await fetchWithAuth(`/api/v1/webscan/tasks?${queryParams.toString()}`)
    if (!response.ok) {
      return null
    }

    const data = await response.json()
    if (!data.success || !data.data) {
      return null
    }

    return {
      tasks: data.data.tasks || [],
      total: data.data.total || 0,
      page: data.data.page || 1,
      pageSize: data.data.pageSize || size,
    }
  }, [])

  // 加载任务列表
  const loadTasks = useCallback(async (params?: Partial<TaskQueryParams>) => {
    try {
      const result = await fetchTasksPage(params)
      if (!result) return
      setTasks(result.tasks)
      setTotal(result.total)
      setCurrentPage(result.page)
      setPageSize(result.pageSize)
    } catch (error) {
      console.error('Failed to load tasks:', error)
    }
  }, [fetchTasksPage])

  // 初始加载任务列表
  useEffect(() => {
    let cancelled = false

    const loadInitialTasks = async () => {
      try {
        const result = await fetchTasksPage()
        if (cancelled || !result) return
        setTasks(result.tasks)
        setTotal(result.total)
        setCurrentPage(result.page)
        setPageSize(result.pageSize)
      } catch (error) {
        if (!cancelled) {
          console.error('Failed to load tasks:', error)
        }
      }
    }

    void loadInitialTasks()

    return () => {
      cancelled = true
    }
  }, [fetchTasksPage])

  // 轮询正在运行的任务状态
  useEffect(() => {
    const hasRunningTasks = tasks.some(
      t => t.status === 'running' || t.status === 'pending' || t.status === 'recovering' || t.status === 'deleting'
    )
    if (!hasRunningTasks) return

    const timer = setInterval(() => loadTasks(), 2000)
    return () => clearInterval(timer)
  }, [tasks, loadTasks])

  // 处理分页变化
  const handlePageChange = (page: number, size: number) => {
    setCurrentPage(page)
    setPageSize(size)
    loadTasks({ page, pageSize: size })
  }

  // 处理筛选变化
  const handleFilterChange = (newFilters: { search?: string; status?: string; scanMode?: string }) => {
    setFilters(newFilters)
    setCurrentPage(1) // 筛选变化时重置到第一页
    loadTasks({ page: 1, ...newFilters })
  }

  // 停止扫描
  const handleStopScan = async () => {
    if (!currentTask) return

    try {
      await fetchWithAuth(`/api/v1/webscan/task/${currentTask.id}/cancel`, {
        method: 'POST',
      })

      setIsRunning(false)
      setActivities((prev) => [
        {
          content: t.vulnScan.messages.userCancelled,
          timestamp: new Date().toLocaleTimeString(),
          type: 'danger',
        },
        ...prev,
      ])

      if (pollingTimer) {
        clearInterval(pollingTimer)
        setPollingTimer(null)
      }
    } catch (error) {
      console.error('Stop scan error:', error)
    }
  }

  // 删除任务
  const handleDeleteTask = async (taskId: string) => {
    const previousTasks = tasks
    setTasks((prev) =>
      prev.map((task) =>
        task.id === taskId ? { ...task, status: 'deleting', pauseRequested: false } : task
      )
    )

    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}`, {
        method: 'DELETE',
      })

      if (!response.ok) {
        const data = await response.json().catch(() => ({}))
        throw new Error(data.error || t.vulnScan.messages.deleteTaskFailed)
      }

      if (response.ok) {
        // 重新加载任务列表以获取正确的分页数据
        loadTasks()
        // 如果删除的是当前查看的任务，返回列表视图
        // 注意：如果当前在详情页，需要检查路由
        const currentUrl = new URL(window.location.href)
        if (
          currentUrl.pathname === '/scan/vuln-scan/detail' &&
          currentUrl.searchParams.get('taskId') === taskId
        ) {
          router.push('/scan/vuln-scan')
        }
        toast.success(t.vulnScan.messages.taskDeleteQueued || t.vulnScan.messages.taskDeleted)
      }
    } catch (error) {
      setTasks(previousTasks)
      console.error('Delete task error:', error)
      toast.error(t.vulnScan.messages.deleteTaskFailed)
    }
  }

  // 批量删除任务
  const handleBatchDeleteTasks = async (taskIds: string[]) => {
    if (taskIds.length === 0) return

    const previousTasks = tasks
    const deletingTaskIds = new Set(taskIds)
    setTasks((prev) =>
      prev.map((task) =>
        deletingTaskIds.has(task.id) ? { ...task, status: 'deleting', pauseRequested: false } : task
      )
    )

    try {
      // 并行删除所有任务
      const deletePromises = taskIds.map(taskId =>
        fetchWithAuth(`/api/v1/webscan/task/${taskId}`, {
          method: 'DELETE',
        })
      )

      const results = await Promise.allSettled(deletePromises)

      // 统计成功和失败的数量
      const successCount = results.filter(r => r.status === 'fulfilled' && r.value.ok).length
      const failCount = taskIds.length - successCount
      const failedTaskIds = new Set(
        results
          .map((result, index) => (result.status === 'fulfilled' && result.value.ok ? null : taskIds[index]))
          .filter((id): id is string => Boolean(id))
      )

      if (failedTaskIds.size > 0) {
        const previousTaskMap = new Map(previousTasks.map((task) => [task.id, task]))
        setTasks((prev) =>
          prev.map((task) =>
            failedTaskIds.has(task.id) ? previousTaskMap.get(task.id) || task : task
          )
        )
      }

      // 重新加载任务列表以获取正确的分页数据
      if (successCount > 0) {
        loadTasks()
      }

      // 显示结果
      if (failCount === 0) {
        toast.success(`已提交 ${successCount} 个任务的删除请求`)
      } else if (successCount === 0) {
        toast.error(`删除失败，共 ${failCount} 个任务`)
      } else {
        toast.success(`已提交 ${successCount} 个任务的删除请求，失败 ${failCount} 个`)
      }
    } catch (error) {
      setTasks(previousTasks)
      console.error('Batch delete tasks error:', error)
      toast.error('批量删除任务失败')
    }
  }

  // 创建任务 - 导航到创建页面
  const handleCreateTask = (scanMode: ScanType) => {
    router.push(`/scan/vuln-scan/new?type=${scanMode}`)
  }

  const handleOpenCombinationTask = () => {
    router.push('/scan/vuln-scan/new?type=combination')
  }

  // 查看任务详情 - 使用路由导航
  const handleViewTask = (taskId: string) => {
    const params = new URLSearchParams()
    params.set('taskId', taskId)
    params.set('tab', 'info')
    router.push(`/scan/vuln-scan/detail?${params.toString()}`)
  }

  const handleUpdateTaskName = async (taskId: string, taskName: string) => {
    const loadingToast = toast.loading(t.common.updating)

    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/name`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ taskName }),
      })

      if (!response.ok) {
        const data = await response.json().catch(() => ({}))
        throw new Error(data.error || t.common.updateFailed)
      }

      setTasks((previousTasks) => previousTasks.map((task) => (
        task.id === taskId ? { ...task, taskName } : task
      )))
      toast.success(t.common.updateSuccess, { id: loadingToast })
      return true
    } catch (error: any) {
      toast.error(error.message || t.common.updateFailed, { id: loadingToast })
      return false
    }
  }

  // 开始任务
  const handleStartTask = async (taskId: string) => {
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/start`, {
        method: 'POST',
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.messages.startTaskFailed)
      }

      toast.success(t.vulnScan.messages.taskStarted)
      // 重新加载任务列表
      loadTasks()
    } catch (error: any) {
      console.error('Start task error:', error)
      toast.error(t.vulnScan.messages.startTaskFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    }
  }

  const handlePauseTask = async (taskId: string) => {
    try {
      setTasks((prev) =>
        prev.map((task) =>
          task.id === taskId && (task.status === 'running' || task.status === 'recovering')
            ? { ...task, pauseRequested: true }
            : task
        )
      )

      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/pause`, {
        method: 'POST',
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.messages.pauseTaskFailed)
      }

      toast.success(t.vulnScan.messages.taskPauseRequested || t.vulnScan.messages.taskPaused)
      loadTasks()
    } catch (error: any) {
      setTasks((prev) =>
        prev.map((task) =>
          task.id === taskId ? { ...task, pauseRequested: false } : task
        )
      )
      console.error('Pause task error:', error)
      toast.error(t.vulnScan.messages.pauseTaskFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    }
  }

  const handleResumeTask = async (taskId: string) => {
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/resume`, {
        method: 'POST',
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.messages.resumeTaskFailed)
      }

      toast.success(t.vulnScan.messages.taskResumed)
      loadTasks()
    } catch (error: any) {
      console.error('Resume task error:', error)
      toast.error(t.vulnScan.messages.resumeTaskFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    }
  }

  // 停止任务
  const handleStopTask = async (taskId: string) => {
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/cancel`, {
        method: 'POST',
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.messages.stopTaskFailed)
      }

      toast.success(t.vulnScan.messages.taskStopped)
      // 重新加载任务列表
      loadTasks()
    } catch (error: any) {
      toast.error(t.vulnScan.messages.stopTaskFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    }
  }



  return (
    <div className="flex-1 overflow-y-auto">
      {/* 顶部栏 */}
      {isRunning && (
        <div className="border-b border-border bg-card">
          <div className="flex items-center justify-end p-6">
            <Button variant="destructive" onClick={handleStopScan}>
              <VideoOff className="w-4 h-4 mr-2" />
              {t.vulnScan.taskManagement.stopTask}
            </Button>
          </div>
        </div>
      )}

      <div className="min-w-0 p-3 md:p-6">
        {/* 任务管理界面 */}
        <TaskManagementView
          tasks={tasks}
          total={total}
          currentPage={currentPage}
          pageSize={pageSize}
          onDeleteTask={handleDeleteTask}
          onBatchDeleteTasks={handleBatchDeleteTasks}
          onCreateTask={handleCreateTask}
          onOpenCombinationTask={handleOpenCombinationTask}
          onViewTask={handleViewTask}
		  onUpdateTaskName={handleUpdateTaskName}
          onStartTask={handleStartTask}
          onPauseTask={handlePauseTask}
          onResumeTask={handleResumeTask}
          onStopTask={handleStopTask}
          onPageChange={handlePageChange}
          onFilterChange={handleFilterChange}
        />
      </div>
    </div>
  )
}
