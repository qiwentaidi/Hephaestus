'use client'

import { Goal } from 'lucide-react'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import { useI18n } from '@/lib/i18n'
import type { GoalRun } from '@/lib/ai-goal-mode'

export function GoalModeCard({ goal, variant = 'card' }: { goal?: GoalRun | null; variant?: 'card' | 'compose' }) {
  const { language } = useI18n()
  if (!goal || goal.mode !== 'goal') return null
  const zh = language === 'zh'
  const compose = variant === 'compose'
  const labels: Record<string, string> = zh ? { active: '进行中', completed: '目标已达成', blocked: '需要补充条件', stopping: '等待停止确认', stopped: '已停止', failed: '执行失败', budget_exhausted: '预算已用完', pending: '待执行', in_progress: '执行中', skipped: '已调整' } : { active: 'In progress', completed: 'Goal achieved', blocked: 'Blocked', stopping: 'Stopping', stopped: 'Stopped', failed: 'Failed', budget_exhausted: 'Budget exhausted', pending: 'Pending', in_progress: 'In progress', skipped: 'Adjusted' }
  const statusLabel = labels[goal.status] || goal.status
  const statusClass = goal.status === 'active' || goal.status === 'in_progress'
    ? 'text-primary'
    : goal.status === 'blocked' || goal.status === 'stopping'
      ? 'text-tone-amber'
      : goal.status === 'failed'
        ? 'text-destructive'
        : 'text-muted-foreground'
  const roundLabel = `${goal.iterations} ${zh ? '轮' : 'rounds'} · ${goal.maxIterations ? `${zh ? '上限' : 'Limit'} ${goal.maxIterations}` : zh ? '不限轮数' : 'No round limit'}`
  if (compose) return <div
    data-slot="ai-goal-progress"
    data-variant={variant}
    title={zh ? '目标模式' : 'Goal mode'}
    className="text-sm"
  >
    <div className="flex items-start gap-3 px-3 py-2.5 md:px-4">
      <Goal aria-hidden="true" className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground/70" />
      <div className="min-w-0 flex-1">
        <p className="line-clamp-2 whitespace-pre-wrap break-words text-sm leading-5 text-foreground">{goal.currentStep || goal.objective}</p>
        <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] leading-4 text-muted-foreground"><span className={statusClass}>{statusLabel}</span><span aria-hidden="true">·</span><span>{roundLabel}</span></div>
      </div>
    </div>
  </div>
  return <Collapsible className="mb-2 rounded-xl border border-border bg-muted/30 px-3 py-2 text-sm" data-slot="ai-goal-progress" data-variant={variant}>
    <CollapsibleTrigger asChild><Button variant="ghost" className="block h-auto w-full space-y-1 whitespace-normal p-0 text-left hover:bg-transparent"><div className="flex flex-wrap items-center gap-2"><span className="font-medium">{zh ? '目标模式' : 'Goal mode'}</span><span className="rounded-md border border-border px-1.5 py-0.5 text-xs">{statusLabel}</span><span className="ml-auto text-xs text-muted-foreground">{roundLabel}</span></div><p className="truncate text-xs text-muted-foreground">{goal.currentStep || goal.objective}</p></Button></CollapsibleTrigger>
    <CollapsibleContent className="mt-2.5 max-h-56 space-y-3 overflow-y-auto border-t border-border pt-3">
      <p className="whitespace-pre-wrap break-words">{goal.objective}</p>
      {goal.successCriteria ? <p className="whitespace-pre-wrap text-xs text-muted-foreground">{zh ? '完成标准：' : 'Success criteria: '}{goal.successCriteria}</p> : null}
      {goal.plan?.length ? <ol className="space-y-2">{goal.plan.map((step) => <li key={step.id} className="rounded-lg bg-background/40 px-2 py-1.5"><span className="mr-2 text-xs text-muted-foreground">{step.status === 'completed' ? (zh ? '已完成' : 'Completed') : labels[step.status] || step.status}</span>{step.title}{step.result ? <p className="mt-1 whitespace-pre-wrap text-xs text-muted-foreground">{step.result}</p> : null}</li>)}</ol> : null}
      {goal.reason ? <p className="whitespace-pre-wrap break-words text-xs">{goal.reason}</p> : null}
    </CollapsibleContent>
  </Collapsible>
}
