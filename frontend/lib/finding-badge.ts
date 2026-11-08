export function findingSeverityBadgeClassName(severity: string): string {
  switch (severity) {
    case 'critical':
      return 'border-red-500/35 bg-red-500/10 text-red-700 dark:text-red-300'
    case 'high':
      return 'border-orange-500/35 bg-orange-500/10 text-orange-700 dark:text-orange-300'
    case 'medium':
      return 'border-amber-500/35 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'low':
      return 'border-lime-500/35 bg-lime-500/10 text-lime-700 dark:text-lime-300'
    default:
      return 'border-border bg-muted text-muted-foreground'
  }
}

export function findingStatusBadgeClassName(status: string): string {
  switch (status) {
    case 'open':
      return 'border-amber-500/35 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'confirmed':
    case 'in_progress':
      return 'border-blue-500/30 bg-blue-500/10 text-blue-700 dark:text-blue-300'
    case 'fixed':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'false_positive':
    case 'ignored':
    case 'accepted_risk':
      return 'border-border bg-muted/70 text-muted-foreground'
    default:
      return 'border-border bg-muted/50 text-foreground'
  }
}
