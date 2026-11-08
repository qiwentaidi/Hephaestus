'use client'

import { Badge } from '@/components/ui/badge'
import { useI18n } from '@/lib/i18n'

export function PlanStatusBadge({ status }: { status: string }) {
  const { t } = useI18n()
  const label = t.aiPlan.statuses[status as keyof typeof t.aiPlan.statuses] || status
  const classes: Record<string, string> = {
    planning: 'border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300',
    waiting: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    completed: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
    failed: 'border-destructive/30 bg-destructive/10 text-destructive',
    cancelled: 'border-muted-foreground/30 bg-muted text-muted-foreground',
    paused: 'border-muted-foreground/30 bg-muted text-muted-foreground',
    pausing: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    cancelling: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    pending: 'border-border bg-muted/40 text-muted-foreground',
    running: 'border-tone-amber-border bg-tone-amber-soft text-tone-amber',
    blocked: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
  }
  return <Badge variant="outline" className={classes[status] || classes.pending}>{label}</Badge>
}
