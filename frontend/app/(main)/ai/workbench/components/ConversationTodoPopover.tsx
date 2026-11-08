'use client'

import { useI18n } from '@/lib/i18n'
import { formatWorkbenchText } from '@/lib/ai-workbench-i18n'

import { AlertCircle, Check, Circle, ListTodo, Loader2 } from 'lucide-react'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import type { AIWorkbenchTodoPlan } from '@/lib/api/ai-workbench'

export function ConversationTodoPopover({ plan, working }: { plan?: AIWorkbenchTodoPlan | null; working: boolean }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const labels = { pending: l.planPending, in_progress: l.planInProgress, completed: l.planCompleted, paused: l.paused, blocked: l.planBlocked, exhausted: l.planExhausted, stopped: l.planStopped }
  const items = plan?.items || []
  const completed = items.filter(item => item.status === 'completed').length

  const trigger = (className: string) => (
    <button type="button" aria-label={l.viewPlan} className={`${className} h-9 items-center gap-1.5 rounded-lg px-2 text-xs text-muted-foreground hover:bg-[var(--input-panel-action-bg-hover)] hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`}>
        <ListTodo className="h-4 w-4 shrink-0" />
        <span className="shrink-0">{l.plan}</span>
        <span className="shrink-0 tabular-nums">{completed}/{items.length}</span>
    </button>
  )

  const details = <>
      <div className="border-b border-border/70 px-4 py-3">
        <div className="flex items-center justify-between gap-3"><span className="text-sm font-medium">{l.currentPlan}</span>{items.length ? <span className="text-xs tabular-nums text-muted-foreground">{formatWorkbenchText(l.planProgress, { completed, total: items.length })}</span> : null}</div>
        {plan?.goal ? <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground" title={plan.goal}>{plan.goal}</p> : null}
      </div>
      {items.length ? <div className="max-h-80 overflow-y-auto px-2 py-2"><ol className="space-y-0.5">{items.map(item => {
        const icon = item.status === 'completed' ? <Check className="h-4 w-4 text-emerald-600 dark:text-emerald-400" /> : item.status === 'in_progress' ? <Loader2 className="h-4 w-4 animate-spin text-primary" /> : item.status === 'blocked' ? <AlertCircle className="h-4 w-4 text-amber-600 dark:text-amber-400" /> : <Circle className="h-4 w-4 text-muted-foreground/60" />
        return <li key={item.id} className="flex gap-2 rounded-md px-2 py-2 text-xs leading-5">
          <span className="mt-0.5 shrink-0">{icon}</span>
          <div className="min-w-0"><p className={item.status === 'completed' ? 'text-muted-foreground line-through' : 'text-foreground'}>{item.title}</p><p className="text-[10px] text-muted-foreground">{labels[item.status]}{item.priority !== undefined ? ` · P${item.priority}` : ''}</p>{item.reason ? <p className="mt-0.5 text-amber-700 dark:text-amber-400">{item.reason}</p> : null}</div>
        </li>
      })}</ol></div> : <p className="px-4 py-5 text-xs text-muted-foreground">{working ? l.planPreparing : l.planEmpty}</p>}
  </>

  return <>
    <HoverCard openDelay={150} closeDelay={100}>
      <HoverCardTrigger asChild>{trigger('hidden md:flex')}</HoverCardTrigger>
      <HoverCardContent side="top" align="start" sideOffset={10} className="w-[min(380px,calc(100vw-32px))] p-0 shadow-xl">{details}</HoverCardContent>
    </HoverCard>
    <Popover>
      <PopoverTrigger asChild>{trigger('flex md:hidden')}</PopoverTrigger>
      <PopoverContent side="top" align="start" sideOffset={10} className="w-[min(380px,calc(100vw-32px))] p-0 shadow-xl">{details}</PopoverContent>
    </Popover>
  </>
}
