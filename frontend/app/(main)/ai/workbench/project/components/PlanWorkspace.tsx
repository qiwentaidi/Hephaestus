'use client'

import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { useCallback, useEffect, useState } from 'react'
import { ArrowUpRight, GitBranch, ListTodo, RefreshCw, UserRoundCog } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { PlanStatusBadge } from '@/components/plan-status-badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  cancelAIPlanRun,
  ensureAIPlanRunController,
  listAIPlanRuns,
  type AIPlanItem,
  type AIPlanRunListItem,
  isOrchestrationRunId,
} from '@/lib/api/ai-plan'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

function formatDate(value: string | undefined | null, locale: string) {
  if (!value || value.startsWith('0001-')) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale, { hour12: false })
}

function itemSummary(item: AIPlanItem, empty: string) {
  if (item.error) return <span className="text-destructive">{item.error}</span>
  if (item.resultSummary) return <span className="line-clamp-2 text-muted-foreground">{item.resultSummary}</span>
  return <span className="text-muted-foreground">{empty}</span>
}

export function PlanWorkspace({ projectId }: { projectId: string }) {
  const { t, language } = useI18n()
  const text = t.aiPlan
  const router = useRouter()
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'
  const [runs, setRuns] = useState<AIPlanRunListItem[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [openingControllerId, setOpeningControllerId] = useState('')
  const [error, setError] = useState('')

  const openController = useCallback(async (run: AIPlanRunListItem['run']) => {
    setOpeningControllerId(run.id)
    try {
      let controllerId = run.controllerConversationId || ''
      if (!controllerId) {
        const detail = await ensureAIPlanRunController(run.id)
        controllerId = detail.controllerConversation?.id || detail.run.controllerConversationId || ''
      }
      if (!controllerId) throw new Error(text.loadFailed)
      router.push(`/ai/workbench?session=${encodeURIComponent(controllerId)}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      setOpeningControllerId('')
    }
  }, [router, text.loadFailed])

  const load = useCallback(async (quiet = false) => {
    if (!projectId) return
    if (quiet) setRefreshing(true)
    else setLoading(true)
    try {
      const data = await listAIPlanRuns({ projectId })
      setRuns(data.items || [])
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [projectId, text.loadFailed])

  useEffect(() => {
    const initial = window.setTimeout(() => { void load() }, 0)
    const timer = window.setInterval(() => { void load(true) }, 5000)
    return () => {
      window.clearTimeout(initial)
      window.clearInterval(timer)
    }
  }, [load])

  const active = runs.some((item) => item.run.status === 'planning' || item.run.status === 'waiting')

  if (loading && !runs.length) {
    return <div className="flex items-center justify-center gap-2 p-12"><Spinner /><span className="text-sm text-muted-foreground">{text.loading}</span></div>
  }

  return (
    <div className="min-h-0 overflow-y-auto p-4 md:p-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="flex items-center gap-2 text-sm font-semibold"><ListTodo className="size-4" />{text.title}</h2>
          <p className="mt-1 text-xs text-muted-foreground">{text.description}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load(true)} disabled={refreshing}>
          <RefreshCw className={cn('size-4', refreshing && 'animate-spin')} />
          {text.refresh}
        </Button>
      </div>
      {error ? <p role="alert" className="mb-3 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p> : null}
      {runs.length === 0 ? (
        <div className="rounded-lg border border-dashed p-10 text-center text-sm text-muted-foreground">{text.empty}</div>
      ) : (
        <div className="space-y-4">
          {runs.map(({ run, items = [] }) => {
            const counts = {
              total: items.length,
              completed: items.filter((item) => item.status === 'completed').length,
              running: items.filter((item) => item.status === 'running').length,
              failed: items.filter((item) => item.status === 'failed' || item.status === 'blocked').length,
            }
            const terminal = ['completed', 'failed', 'cancelled'].includes(run.status)
            return (
              <section key={run.id} className="rounded-lg border bg-card">
                <header className="flex flex-wrap items-start justify-between gap-3 border-b px-4 py-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <PlanStatusBadge status={run.status} />
                      <span className="text-xs text-muted-foreground">{formatDate(run.createdAt, locale)}</span>
                      {run.wakePending && !terminal ? <Badge variant="outline" className="border-amber-500/30 text-amber-700 dark:text-amber-300">wake pending</Badge> : null}
                    </div>
                    <p className="mt-2 max-w-3xl break-words text-sm">{run.objective}</p>
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>{text.total.replace('{count}', String(counts.total))}</span>
                      <span>·</span>
                      <span>{text.completed.replace('{count}', String(counts.completed))}</span>
                      <span>·</span>
                      <span>{text.running.replace('{count}', String(counts.running))}</span>
                      {counts.failed ? <><span>·</span><span className="text-destructive">{text.failed.replace('{count}', String(counts.failed))}</span></> : null}
                    </div>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Button variant="default" size="sm" onClick={() => void openController(run)} disabled={openingControllerId === run.id || ((run.readOnly || !isOrchestrationRunId(run.id)) && !run.controllerConversationId)}>
                      <UserRoundCog className="size-3.5" />
                      {openingControllerId === run.id ? text.openingController : run.controllerConversationId ? text.openController : text.enableController}
                    </Button>
                    <Button variant="outline" size="sm" asChild>
                      <Link href={`/ai/workbench?session=${encodeURIComponent(run.plannerConversationId)}`}>
                        {text.openPlanner}
                        <ArrowUpRight className="size-3.5" />
                      </Link>
                    </Button>
                    {!terminal && !run.readOnly && isOrchestrationRunId(run.id) ? (
                      <Button variant="outline" size="sm" onClick={async () => {
                        try {
                          await cancelAIPlanRun(run.id)
                          await load(true)
                        } catch (err) {
                          setError(err instanceof Error ? err.message : text.loadFailed)
                        }
                      }}>{text.cancel}</Button>
                    ) : null}
                  </div>
                </header>
                {run.summary ? <p className="whitespace-pre-wrap break-words border-b bg-muted/20 px-4 py-2 text-xs text-muted-foreground">{run.summary}</p> : null}
                {items.length === 0 ? (
                  <p className="px-4 py-6 text-sm text-muted-foreground">{text.noItems}</p>
                ) : (
                  <div className="divide-y">
                    {items.map((item) => (
                      <div key={item.id} className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start">
                        <div className="flex min-w-0 flex-1 gap-3">
                          <span className="w-6 shrink-0 pt-0.5 text-right text-xs tabular-nums text-muted-foreground">{item.seq}</span>
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-2">
                              <p className="text-sm font-medium">{item.title}</p>
                              <PlanStatusBadge status={item.status} />
                            </div>
                            <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{item.target}</p>
                            <div className="mt-1 text-xs">{itemSummary(item, text.noSummary)}</div>
                          </div>
                        </div>
                        <div className="flex shrink-0 flex-wrap items-center gap-2 pl-9 sm:pl-0">
                          {item.childConversationId ? (
                            <>
                              <Button variant="outline" size="sm" asChild>
                                <Link href={`/ai/workbench?session=${encodeURIComponent(item.childConversationId)}`}>{text.openWorker}<ArrowUpRight className="size-3.5" /></Link>
                              </Button>
                              <Button variant="ghost" size="sm" asChild>
                                <Link href={`/ai/workbench/exploration?session=${encodeURIComponent(item.childConversationId)}`}><GitBranch className="size-3.5" />{text.exploration}</Link>
                              </Button>
                            </>
                          ) : null}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </section>
            )
          })}
        </div>
      )}
      {active ? <p className="mt-3 text-xs text-muted-foreground">{text.liveHint}</p> : null}
    </div>
  )
}
