'use client'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { ArrowRight, FileJson2, Loader2, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-react'
import toast from 'react-hot-toast'

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
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { fetchWithAuth } from '@/lib/api'
import { getStatusBadge } from '@/lib/status-badge'
import { cn } from '@/lib/utils'

type APIDocTask = {
  id: string
  taskName: string
  status: string
  progress: number
  sourceUrl: string
  baseUrl?: string
  documentType: string
  headers?: Record<string, string>
  endpointCount: number
  testedCount: number
  errorMsg?: string
  createdAt: string
  updatedAt: string
}

type TaskListResponse = {
  tasks: APIDocTask[]
  total: number
  limit: number
  offset: number
}

const activeStatuses = new Set(['pending', 'running'])

const taskStatusLabels = {
  pending: '等待中',
  running: '运行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

const parseHeaders = (input: string) => {
  const headers: Record<string, string> = {}
  input
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .forEach((line) => {
      const index = line.indexOf(':')
      if (index <= 0) return
      const key = line.slice(0, index).trim()
      const value = line.slice(index + 1).trim()
      if (key) headers[key] = value
    })
  return headers
}

const formatHeaders = (value?: Record<string, string>) => {
  return Object.entries(value || {})
    .map(([key, headerValue]) => `${key}: ${headerValue}`)
    .join('\n')
}

const formatTime = (value?: string | null) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

export default function ApiDocTestPage() {
  const router = useRouter()
  const tableHostRef = useRef<HTMLDivElement | null>(null)
  const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)
  const [tasks, setTasks] = useState<APIDocTask[]>([])
  const [searchQuery, setSearchQuery] = useState('')
  const [loadingTasks, setLoadingTasks] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [creating, setCreating] = useState(false)
  const [editingTask, setEditingTask] = useState<APIDocTask | null>(null)
  const [retestingTaskId, setRetestingTaskId] = useState('')
  const [deletingTask, setDeletingTask] = useState<APIDocTask | null>(null)
  const [deletingTaskId, setDeletingTaskId] = useState('')
  const [sourceUrl, setSourceUrl] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [documentType, setDocumentType] = useState('auto')
  const [headers, setHeaders] = useState('')

  const hasActiveTask = useMemo(() => tasks.some((task) => activeStatuses.has(task.status)), [tasks])

  const filteredTasks = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    if (!query) return tasks
    return tasks.filter((task) => (
      task.taskName.toLowerCase().includes(query) ||
      task.sourceUrl.toLowerCase().includes(query) ||
      task.documentType.toLowerCase().includes(query)
    ))
  }, [searchQuery, tasks])

  const loadTasks = useCallback(async () => {
    setLoadingTasks(true)
    try {
      const response = await fetchWithAuth('/api/v1/tools/api-doc-test/tasks?limit=100')
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || '获取任务列表失败')
      }
      const payload = data.data as TaskListResponse
      setTasks(payload.tasks || [])
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '获取任务列表失败')
    } finally {
      setLoadingTasks(false)
    }
  }, [])

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
    loadTasks()
  }, [loadTasks])

  useEffect(() => {
    if (!hasActiveTask) return
    const timer = window.setInterval(loadTasks, 3000)
    return () => window.clearInterval(timer)
  }, [hasActiveTask, loadTasks])

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
  }, [filteredTasks.length, updateRightFixedShadow])

  const resetDialogForm = useCallback(() => {
    setEditingTask(null)
    setSourceUrl('')
    setBaseUrl('')
    setDocumentType('auto')
    setHeaders('')
  }, [])

  const openCreateDialog = () => {
    resetDialogForm()
    setCreateOpen(true)
  }

  const openEditDialog = (task: APIDocTask) => {
    setEditingTask(task)
    setSourceUrl(task.sourceUrl || '')
    setBaseUrl(task.baseUrl || '')
    setDocumentType(task.documentType || 'auto')
    setHeaders(formatHeaders(task.headers))
    setCreateOpen(true)
  }

  const handleDialogOpenChange = (open: boolean) => {
    setCreateOpen(open)
    if (!open && !creating) {
      resetDialogForm()
    }
  }

  const handleSaveTask = async () => {
    if (!sourceUrl.trim()) {
      toast.error('请输入文档 URL')
      return
    }
    setCreating(true)
    try {
      const isEditing = Boolean(editingTask)
      const response = await fetchWithAuth(
        isEditing
          ? `/api/v1/tools/api-doc-test/tasks/${encodeURIComponent(editingTask!.id)}`
          : '/api/v1/tools/api-doc-test/tasks',
        {
          method: isEditing ? 'PUT' : 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sourceUrl: sourceUrl.trim(),
            baseUrl: baseUrl.trim(),
            documentType,
            headers: parseHeaders(headers),
            timeoutSeconds: 15,
          }),
        },
      )
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || (isEditing ? '更新任务失败' : '创建任务失败'))
      }
      const task = data.data as APIDocTask
      toast.success(isEditing ? '任务已更新' : '任务已创建')
      setCreateOpen(false)
      resetDialogForm()
      await loadTasks()
      if (!isEditing) {
        router.push(`/tools/api-doc-test/detail?id=${encodeURIComponent(task.id)}`)
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : editingTask ? '更新任务失败' : '创建任务失败')
    } finally {
      setCreating(false)
    }
  }

  const handleRetestTask = async (task: APIDocTask) => {
    if (activeStatuses.has(task.status)) return
    setRetestingTaskId(task.id)
    try {
      const response = await fetchWithAuth(`/api/v1/tools/api-doc-test/tasks/${encodeURIComponent(task.id)}/retest`, {
        headers: { 'Content-Type': 'application/json' },
        method: 'POST',
      })
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || '重新测试失败')
      }
      toast.success('已开始重新测试')
      await loadTasks()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '重新测试失败')
    } finally {
      setRetestingTaskId('')
    }
  }

  const handleDeleteTask = async () => {
    if (!deletingTask) return
    setDeletingTaskId(deletingTask.id)
    try {
      const response = await fetchWithAuth(`/api/v1/tools/api-doc-test/tasks/${encodeURIComponent(deletingTask.id)}`, {
        method: 'DELETE',
      })
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || '删除任务失败')
      }
      toast.success('任务已删除')
      setDeletingTask(null)
      await loadTasks()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '删除任务失败')
    } finally {
      setDeletingTaskId('')
    }
  }

  return (
    <div className="flex-1 overflow-y-auto scan-panel-enter">
      <div className="flex flex-col gap-2 px-3 py-3 md:flex-row md:items-center md:justify-between md:gap-3 md:px-6 md:py-4">
        <div className="min-w-0 w-full flex-1 md:max-w-md">
          <div className="relative">
            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="搜索任务名称、文档 URL 或类型..."
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              className="pl-9"
            />
          </div>
        </div>
        <div className="flex items-center justify-end gap-2">
          <Button variant="outline" size="icon" className="h-10 w-10" onClick={loadTasks} disabled={loadingTasks} aria-label="刷新任务">
            <RefreshCw className={cn('h-4 w-4', loadingTasks && 'animate-spin')} />
          </Button>
          <Button onClick={openCreateDialog} className="h-10 flex-1 md:flex-none">
            <Plus className="mr-2 h-4 w-4" />
            新建任务
          </Button>
        </div>
      </div>

      <div className="px-3 md:px-6 pb-6">
        {filteredTasks.length === 0 ? (
          <Card className="rounded-lg p-12 text-center">
            <FileJson2 className="mx-auto mb-4 h-12 w-12 text-muted-foreground opacity-50" />
            <p className="text-muted-foreground">{tasks.length ? '没有匹配的任务' : '暂无接口文档测试任务'}</p>
          </Card>
        ) : (
          <>
            <div className="grid gap-3 md:hidden">
            {filteredTasks.map((task) => {
              const isTaskActive = activeStatuses.has(task.status)
              const isRetesting = retestingTaskId === task.id
              const isDeleting = deletingTaskId === task.id
              return (
                <Card key={task.id} className="overflow-hidden rounded-lg p-3">
                  <div className="flex min-w-0 items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="truncate font-medium" title={task.taskName}>{task.taskName}</div>
                      {task.errorMsg ? <div className="mt-1 break-words text-xs text-red-600 dark:text-red-300">{task.errorMsg}</div> : null}
                    </div>
                    <div className="shrink-0">{getStatusBadge(task.status, taskStatusLabels)}</div>
                  </div>
                  <div className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-border pt-3 text-sm">
                    <div className="col-span-2 min-w-0">
                      <div className="text-xs text-muted-foreground">文档地址</div>
                      <div className="mt-0.5 break-all font-mono text-xs text-muted-foreground">{task.sourceUrl}</div>
                    </div>
                    <div>
                      <div className="text-xs text-muted-foreground">文档类型</div>
                      <div className="mt-0.5"><Badge variant="outline">{task.documentType || 'auto'}</Badge></div>
                    </div>
                    <div>
                      <div className="text-xs text-muted-foreground">更新时间</div>
                      <div className="mt-0.5 text-xs text-muted-foreground">{formatTime(task.updatedAt)}</div>
                    </div>
                    <div>
                      <div className="text-xs text-muted-foreground">接口数</div>
                      <div className="mt-0.5 font-mono text-xs">{task.endpointCount || '-'}</div>
                    </div>
                    <div>
                      <div className="text-xs text-muted-foreground">已测试</div>
                      <div className="mt-0.5 font-mono text-xs">{task.endpointCount > 0 ? `${task.testedCount}/${task.endpointCount}` : '-'}</div>
                    </div>
                  </div>
                  <div className="mt-3 flex items-center justify-end gap-1 border-t border-border pt-2">
                    <Button size="icon" variant="ghost" className="h-10 w-10" onClick={() => router.push(`/tools/api-doc-test/detail?id=${encodeURIComponent(task.id)}`)} aria-label="进入详情">
                      <ArrowRight className="h-4 w-4" />
                    </Button>
                    <Button size="icon" variant="ghost" className="h-10 w-10" onClick={() => openEditDialog(task)} disabled={isTaskActive} aria-label="编辑任务">
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button size="icon" variant="ghost" className="h-10 w-10" onClick={() => handleRetestTask(task)} disabled={isTaskActive || isRetesting || isDeleting} aria-label="重新测试">
                      <RefreshCw className={cn('h-4 w-4', isRetesting && 'animate-spin')} />
                    </Button>
                    <Button size="icon" variant="ghost" className="h-10 w-10 text-destructive hover:text-destructive" onClick={() => setDeletingTask(task)} disabled={isTaskActive || isDeleting} aria-label="删除任务">
                      {isDeleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                    </Button>
                  </div>
                </Card>
              )
            })}
            </div>

            <div ref={tableHostRef} className="hidden overflow-hidden rounded-lg border md:block">
            <Table className="min-w-[1180px]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[220px]">任务名称</TableHead>
                  <TableHead>文档 URL</TableHead>
                  <TableHead className="w-[120px]">文档类型</TableHead>
                  <TableHead className="w-[110px]">状态</TableHead>
                  <TableHead className="w-[100px]">接口数</TableHead>
                  <TableHead className="w-[100px]">已测试</TableHead>
                  <TableHead className="w-[180px]">更新时间</TableHead>
                  <TableHead
                    className={cn(
                      'sticky right-0 z-20 w-[190px] border-l border-border bg-background text-center',
                      showRightFixedShadow && 'table-fixed-column-shadow-left',
                    )}
                  >
                    操作
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredTasks.map((task) => {
                  const isTaskActive = activeStatuses.has(task.status)
                  const isRetesting = retestingTaskId === task.id
                  const isDeleting = deletingTaskId === task.id
                  return (
                    <TableRow key={task.id} className="group hover:bg-muted">
                      <TableCell data-label="任务名称" className="font-medium">
                        <div className="truncate" title={task.taskName}>{task.taskName}</div>
                        {task.errorMsg ? <div className="mt-1 truncate text-xs text-red-600 dark:text-red-300">{task.errorMsg}</div> : null}
                      </TableCell>
                      <TableCell data-label="文档地址" className="max-w-[420px] truncate font-mono text-xs text-muted-foreground" title={task.sourceUrl}>
                        {task.sourceUrl}
                      </TableCell>
                      <TableCell data-label="文档类型">
                        <Badge variant="outline">{task.documentType || 'auto'}</Badge>
                      </TableCell>
                      <TableCell data-label="状态">{getStatusBadge(task.status, taskStatusLabels)}</TableCell>
                      <TableCell data-label="接口数" className="font-mono text-xs">{task.endpointCount || '-'}</TableCell>
                      <TableCell data-label="已测试" className="font-mono text-xs">
                        {task.endpointCount > 0 ? `${task.testedCount}/${task.endpointCount}` : '-'}
                      </TableCell>
                      <TableCell data-label="更新时间" className="text-muted-foreground">{formatTime(task.updatedAt)}</TableCell>
                      <TableCell data-label="操作"
                        className={cn(
                          'sticky right-0 z-10 border-l border-border bg-background text-center group-hover:bg-muted',
                          showRightFixedShadow && 'table-fixed-column-shadow-left',
                        )}
                      >
                        <div className="flex items-center justify-center gap-1">
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => router.push(`/tools/api-doc-test/detail?id=${encodeURIComponent(task.id)}`)}
                            aria-label="进入详情"
                          >
                            <ArrowRight className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => openEditDialog(task)}
                            disabled={isTaskActive}
                            aria-label="编辑任务"
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => handleRetestTask(task)}
                            disabled={isTaskActive || isRetesting || isDeleting}
                            aria-label="重新测试"
                          >
                            <RefreshCw className={cn('h-4 w-4', isRetesting && 'animate-spin')} />
                          </Button>
                          <Button
                            size="icon"
                            variant="ghost"
                            onClick={() => setDeletingTask(task)}
                            disabled={isTaskActive || isDeleting}
                            aria-label="删除任务"
                            className="text-destructive hover:text-destructive"
                          >
                            {isDeleting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
            </div>
          </>
        )}
      </div>

      <Dialog open={createOpen} onOpenChange={handleDialogOpenChange}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{editingTask ? '编辑接口文档测试任务' : '新建接口文档测试任务'}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4">
            <div className="space-y-2">
              <Label htmlFor="api-doc-source-url">文档 URL</Label>
              <Input
                id="api-doc-source-url"
                value={sourceUrl}
                onChange={(event) => setSourceUrl(event.target.value)}
                placeholder="https://example.com/v3/api-docs"
              />
            </div>
            <div className="space-y-2">
              <Label>文档类型</Label>
              <Select value={documentType} onValueChange={setDocumentType}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">自动识别</SelectItem>
                  <SelectItem value="openapi">OpenAPI</SelectItem>
                  <SelectItem value="swagger">Swagger</SelectItem>
                  <SelectItem value="wsdl">WSDL</SelectItem>
                  <SelectItem value="graphql">GraphQL</SelectItem>
                  <SelectItem value="text">普通文本</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="api-doc-base-url">Base URL</Label>
              <Input
                id="api-doc-base-url"
                value={baseUrl}
                onChange={(event) => setBaseUrl(event.target.value)}
                placeholder="留空自动推断"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="api-doc-headers">请求头</Label>
              <Textarea
                id="api-doc-headers"
                value={headers}
                onChange={(event) => setHeaders(event.target.value)}
                placeholder={'Authorization: Bearer ...\nCookie: session=...'}
                className="min-h-[120px] font-mono text-xs"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => handleDialogOpenChange(false)} disabled={creating}>
              取消
            </Button>
            <Button onClick={handleSaveTask} disabled={creating}>
              {creating ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Plus className="mr-2 h-4 w-4" />}
              {editingTask ? '保存' : '创建任务'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <AlertDialog open={Boolean(deletingTask)} onOpenChange={(open) => !open && setDeletingTask(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除接口文档测试任务</AlertDialogTitle>
            <AlertDialogDescription>
              将删除任务「{deletingTask?.taskName || deletingTask?.sourceUrl || '-'}」以及它的全部测试结果，此操作无法恢复。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={Boolean(deletingTaskId)}>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              disabled={Boolean(deletingTaskId)}
              onClick={(event) => {
                event.preventDefault()
                handleDeleteTask()
              }}
            >
              {deletingTaskId ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Trash2 className="mr-2 h-4 w-4" />}
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
