'use client'

import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { ArrowDown, ArrowLeft, ArrowUp, ArrowUpDown, FileJson2, Loader2, RefreshCw, Send } from 'lucide-react'
import toast from 'react-hot-toast'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { fetchWithAuth } from '@/lib/api'
import { StatusBadge, getStatusBadge } from '@/lib/status-badge'
import { cn } from '@/lib/utils'

type APIDocTask = {
  id: string
  taskName: string
  status: string
  progress: number
  sourceUrl: string
  baseUrl?: string
  documentType: string
  endpointCount: number
  testedCount: number
  errorMsg?: string
  createdAt: string
  updatedAt: string
}

type APIDocResultRow = {
  id: string
  taskId: string
  url: string
  method: string
  summary?: string
  statusCode?: number
  statusText?: string
  responseLength?: number
  durationMs?: number
  testTags?: string[]
  rawRequest?: string
  requestHeaders?: Record<string, string>
  requestBody?: string
  responseBody?: string
  responseTruncated?: boolean
  errorMsg?: string
}

type TaskDetailResponse = {
  task: APIDocTask
  results: APIDocResultRow[]
}

const activeStatuses = new Set(['pending', 'running'])

const taskStatusLabels = {
  pending: '等待中',
  running: '运行中',
  completed: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

type LengthSortDirection = 'none' | 'asc' | 'desc'

const methodClassName = (method: string) => {
  switch (method.toUpperCase()) {
    case 'GET':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'POST':
      return 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300'
    case 'DELETE':
      return 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300'
    default:
      return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
  }
}

const formatBody = (body?: string) => {
  if (!body) return ''
  const trimmed = body.trim()
  if (!trimmed.startsWith('{') && !trimmed.startsWith('[')) return body
  try {
    return JSON.stringify(JSON.parse(body), null, 2)
  } catch {
    return body
  }
}

const formatTime = (value?: string | null) => {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

const buildRawRepeaterRequest = (row: APIDocResultRow) => {
  const parsed = new URL(row.url)
  const target = `${parsed.pathname || '/'}${parsed.search}`
  const headers = { ...(row.requestHeaders || {}) }
  const headerLines = [`Host: ${parsed.host}`]
  Object.entries(headers).forEach(([key, value]) => {
    if (/^host$/i.test(key) || /^content-length$/i.test(key)) return
    headerLines.push(`${key}: ${value}`)
  })
  return `${row.method || 'GET'} ${target} HTTP/1.1\n${headerLines.join('\n')}\n\n${row.requestBody || ''}`
}

const formatRawRequest = (row: APIDocResultRow) => {
  if (row.rawRequest) return row.rawRequest
  try {
    return buildRawRepeaterRequest(row)
  } catch {
    return `${row.method || 'GET'} ${row.url || '/'} HTTP/1.1\n\n${row.requestBody || ''}`
  }
}

export default function ApiDocTestDetailPage() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const taskId = searchParams.get('id') || ''
  const [task, setTask] = useState<APIDocTask | null>(null)
  const [results, setResults] = useState<APIDocResultRow[]>([])
  const [expandedRowId, setExpandedRowId] = useState('')
  const [loading, setLoading] = useState(false)
  const [lengthSort, setLengthSort] = useState<LengthSortDirection>('none')

  const taskIsActive = useMemo(() => activeStatuses.has(task?.status || ''), [task?.status])

  const sortedResults = useMemo(() => {
    if (lengthSort === 'none') return results
    return [...results].sort((left, right) => {
      const leftLength = left.responseLength ?? 0
      const rightLength = right.responseLength ?? 0
      return lengthSort === 'asc' ? leftLength - rightLength : rightLength - leftLength
    })
  }, [lengthSort, results])

  const toggleLengthSort = () => {
    setLengthSort((current) => {
      if (current === 'none') return 'desc'
      if (current === 'desc') return 'asc'
      return 'none'
    })
  }

  const LengthSortIcon = lengthSort === 'desc' ? ArrowDown : lengthSort === 'asc' ? ArrowUp : ArrowUpDown

  const forwardToRepeater = (row: APIDocResultRow) => {
    try {
      const rawRequest = formatRawRequest(row)
      const payload = {
        raw: rawRequest,
        method: row.method || 'GET',
        url: row.url,
        forceHttps: row.url.toLowerCase().startsWith('https://'),
        name: row.summary || row.url,
      }
      window.localStorage.setItem('repeater_pending_request', JSON.stringify(payload))
      router.push('/scan/repeater')
      toast.success('已转发到请求转发')
    } catch (error) {
      toast.error(error instanceof Error ? error.message : '转发到请求转发失败')
    }
  }

  const loadTaskDetail = useCallback(async (silent = false) => {
    if (!taskId) {
      setTask(null)
      setResults([])
      return
    }
    if (!silent) setLoading(true)
    try {
      const response = await fetchWithAuth(`/api/v1/tools/api-doc-test/tasks/${encodeURIComponent(taskId)}`)
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || '获取任务详情失败')
      }
      const payload = data.data as TaskDetailResponse
      setTask(payload.task)
      setResults(payload.results || [])
    } catch (error) {
      if (!silent) toast.error(error instanceof Error ? error.message : '获取任务详情失败')
    } finally {
      if (!silent) setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    loadTaskDetail()
  }, [loadTaskDetail])

  useEffect(() => {
    if (!taskIsActive) return
    const timer = window.setInterval(() => loadTaskDetail(true), 3000)
    return () => window.clearInterval(timer)
  }, [loadTaskDetail, taskIsActive])

  return (
    <div className="flex-1 overflow-y-auto scan-panel-enter">
      <div className="flex items-center justify-between gap-3 px-3 py-3 md:gap-4 md:px-6 md:py-4">
        <div className="flex min-w-0 items-center gap-3">
          <Button variant="outline" size="icon" onClick={() => router.push('/tools/api-doc-test')} aria-label="返回任务列表">
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <FileJson2 className="h-5 w-5 shrink-0 text-primary" />
              <h1 className="truncate text-lg font-semibold">{task?.taskName || '接口文档测试详情'}</h1>
              {task ? getStatusBadge(task.status, taskStatusLabels) : null}
            </div>
            <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{task?.sourceUrl || taskId}</div>
          </div>
        </div>
        <Button variant="outline" onClick={() => loadTaskDetail()} disabled={loading} className="h-10 shrink-0 px-3" aria-label="刷新任务详情">
          {loading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
          <span className="hidden sm:inline">刷新</span>
        </Button>
      </div>

      <div className="space-y-3 px-3 pb-4 md:space-y-4 md:px-6 md:pb-6">
        {task ? (
          <div className="grid grid-cols-2 gap-2 md:grid-cols-4 md:gap-3">
            <Card className="rounded-lg py-0">
              <CardContent className="p-3 md:p-4">
                <div className="text-xs text-muted-foreground">文档类型</div>
                <div className="mt-1 font-medium">{task.documentType || 'auto'}</div>
              </CardContent>
            </Card>
            <Card className="rounded-lg py-0">
              <CardContent className="p-3 md:p-4">
                <div className="text-xs text-muted-foreground">接口数</div>
                <div className="mt-1 font-mono text-lg font-semibold">{task.endpointCount || 0}</div>
              </CardContent>
            </Card>
            <Card className="rounded-lg py-0">
              <CardContent className="p-3 md:p-4">
                <div className="text-xs text-muted-foreground">已测试</div>
                <div className="mt-1 font-mono text-lg font-semibold">{task.testedCount || 0}</div>
              </CardContent>
            </Card>
            <Card className="rounded-lg py-0">
              <CardContent className="p-3 md:p-4">
                <div className="text-xs text-muted-foreground">更新时间</div>
                <div className="mt-1 truncate text-sm" title={formatTime(task.updatedAt)}>{formatTime(task.updatedAt)}</div>
              </CardContent>
            </Card>
          </div>
        ) : null}

        {task?.errorMsg ? (
          <div className="rounded-md border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-700 dark:text-red-300">
            {task.errorMsg}
          </div>
        ) : null}

        <div className="grid gap-3 md:hidden">
          {sortedResults.map((row) => {
            const expanded = expandedRowId === row.id
            return (
              <div key={row.id} className="overflow-hidden rounded-lg border bg-card">
                <button
                  type="button"
                  className="w-full space-y-3 p-3 text-left"
                  onClick={() => setExpandedRowId(expanded ? '' : row.id)}
                >
                  <div className="flex items-center justify-between gap-2">
                    <Badge variant="outline" className={cn('font-mono', methodClassName(row.method))}>
                      {row.method}
                    </Badge>
                    {row.statusCode ? <StatusBadge status={row.statusCode} /> : <Badge variant="outline">ERR</Badge>}
                  </div>
                  <div className="break-all font-mono text-xs leading-5" title={row.url}>{row.url}</div>
                  <div className="grid grid-cols-2 gap-3 text-xs">
                    <div className="min-w-0">
                      <div className="text-muted-foreground">描述</div>
                      <div className="mt-0.5 truncate text-sm" title={row.summary || '-'}>{row.summary || '-'}</div>
                    </div>
                    <div>
                      <div className="text-muted-foreground">长度</div>
                      <div className="mt-0.5 font-mono">{row.responseLength ?? 0}B</div>
                    </div>
                  </div>
                  {(row.testTags || []).length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {(row.testTags || []).map((tag) => <Badge key={tag} variant="secondary" className="text-[11px]">{tag}</Badge>)}
                    </div>
                  )}
                </button>
                {expanded ? (
                  <div className="border-t border-border bg-muted/30">
                    <div className="flex justify-end border-b border-border px-3 py-2">
                      <Button size="sm" variant="outline" onClick={() => forwardToRepeater(row)}>
                        <Send className="mr-2 h-4 w-4" />
                        转发到请求转发
                      </Button>
                    </div>
                    <div className="space-y-3 p-3">
                      <div className="min-w-0">
                        <div className="mb-2 text-sm font-medium">Raw Request</div>
                        <pre className="max-h-[280px] max-w-full overflow-auto rounded-md border bg-background p-3 text-xs leading-relaxed">
                          {formatRawRequest(row)}
                        </pre>
                      </div>
                      <div className="min-w-0">
                        <div className="mb-2 text-sm font-medium">响应体</div>
                        <pre className="max-h-[280px] max-w-full overflow-auto rounded-md border bg-background p-3 text-xs leading-relaxed">
                          {formatBody(row.responseBody) || row.errorMsg || '无响应体'}
                        </pre>
                        {row.responseTruncated ? <div className="mt-2 text-xs text-amber-600 dark:text-amber-300">响应体已截断。</div> : null}
                      </div>
                    </div>
                  </div>
                ) : null}
              </div>
            )
          })}
          {!sortedResults.length ? (
            <div className="rounded-lg border bg-card p-10 text-center text-sm text-muted-foreground">
              {loading ? '正在加载...' : '暂无测试结果'}
            </div>
          ) : null}
        </div>

        <div className="hidden overflow-hidden rounded-lg border md:block">
          <Table className="w-full table-fixed">
            <colgroup>
              <col className="w-[44%]" />
              <col className="w-[90px]" />
              <col className="w-[18%]" />
              <col className="w-[90px]" />
              <col className="w-[90px]" />
              <col className="w-[160px]" />
            </colgroup>
            <TableHeader>
              <TableRow>
                <TableHead>URL</TableHead>
                <TableHead>请求方式</TableHead>
                <TableHead>描述</TableHead>
                <TableHead>状态码</TableHead>
                <TableHead>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="-ml-3 h-8 gap-1 px-2"
                    onClick={toggleLengthSort}
                  >
                    长度
                    <LengthSortIcon className="h-3.5 w-3.5" />
                  </Button>
                </TableHead>
                <TableHead>测试标签</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedResults.map((row) => {
                const expanded = expandedRowId === row.id
                return (
                  <Fragment key={row.id}>
                    <TableRow
                      data-state={expanded ? 'selected' : undefined}
                      className="cursor-pointer"
                      onClick={() => setExpandedRowId(expanded ? '' : row.id)}
                    >
                      <TableCell className="min-w-0">
                        <div className="truncate font-mono text-xs" title={row.url}>{row.url}</div>
                        {row.errorMsg ? <div className="mt-1 truncate text-xs text-red-600 dark:text-red-300" title={row.errorMsg}>{row.errorMsg}</div> : null}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline" className={cn('font-mono', methodClassName(row.method))}>
                          {row.method}
                        </Badge>
                      </TableCell>
                      <TableCell className="min-w-0">
                        <div className="truncate text-sm" title={row.summary || '-'}>
                          {row.summary || '-'}
                        </div>
                      </TableCell>
                      <TableCell>
                        {row.statusCode ? <StatusBadge status={row.statusCode} /> : <Badge variant="outline">ERR</Badge>}
                      </TableCell>
                      <TableCell className="font-mono text-xs">{row.responseLength ?? 0}B</TableCell>
                      <TableCell className="min-w-0">
                        <div className="flex min-w-0 flex-wrap gap-1 overflow-hidden">
                          {(row.testTags || []).map((tag) => (
                            <Badge key={tag} variant="secondary" className="text-[11px]">
                              {tag}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                    </TableRow>
                    {expanded ? (
                      <TableRow>
                        <TableCell colSpan={6} className="bg-muted/30 p-0">
                          <div className="flex items-center justify-end border-b border-border px-4 py-3">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => forwardToRepeater(row)}
                            >
                              <Send className="mr-2 h-4 w-4" />
                              转发到请求转发
                            </Button>
                          </div>
                          <div className="grid min-w-0 gap-0 md:grid-cols-2">
                            <div className="min-w-0 border-b border-border p-4 md:border-b-0 md:border-r">
                              <div className="mb-2 text-sm font-medium">Raw Request</div>
                              <pre className="max-h-[360px] max-w-full overflow-auto rounded-md border bg-background p-3 text-xs leading-relaxed">
                                {formatRawRequest(row)}
                              </pre>
                            </div>
                            <div className="min-w-0 p-4">
                              <div className="mb-2 text-sm font-medium">响应体</div>
                              <pre className="max-h-[360px] max-w-full overflow-auto rounded-md border bg-background p-3 text-xs leading-relaxed">
                                {formatBody(row.responseBody) || row.errorMsg || '无响应体'}
                              </pre>
                              {row.responseTruncated ? (
                                <div className="mt-2 text-xs text-amber-600 dark:text-amber-300">响应体已截断。</div>
                              ) : null}
                            </div>
                          </div>
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </Fragment>
                )
              })}
              {!results.length ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-40 text-center text-muted-foreground">
                    {loading ? '正在加载...' : '暂无测试结果'}
                  </TableCell>
                </TableRow>
              ) : null}
            </TableBody>
          </Table>
        </div>
      </div>
    </div>
  )
}
