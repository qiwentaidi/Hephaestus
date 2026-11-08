'use client'

import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import type { AIWorkbenchTokenUsage } from '@/lib/api/ai-workbench'

function format(value: number | undefined, language: string) {
  return typeof value === 'number' ? value.toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US') : '—'
}

export function TokenUsage({ total, compact = false, scope = 'session' }: { total?: number; compact?: boolean; scope?: 'session' | 'turn' }) {
  const { t, language } = useI18n()
  const value = format(total, language)
  const separator = language === 'zh' ? '：' : ': '
  return <span data-slot="session-token-usage" title={t.tokenUsage.recordedOnly} className={cn('block text-xs font-normal tabular-nums text-muted-foreground', compact ? 'mt-0.5 truncate' : 'shrink-0 whitespace-nowrap')}>{compact ? `${value} Tokens` : `${scope === 'turn' ? t.tokenUsage.turnTotal : t.tokenUsage.sessionTotal}${separator}${value}`}</span>
}

export function TokenUsageButton({ usage, scope = 'session' }: { usage?: AIWorkbenchTokenUsage; scope?: 'session' | 'project' }) {
  const { t, language } = useI18n()
  const separator = language === 'zh' ? '：' : ': '
  const label = scope === 'project' ? t.tokenUsage.projectTotal : t.tokenUsage.currentSession
  const shortLabel = scope === 'project' ? t.tokenUsage.projectShort : t.tokenUsage.sessionShort
  const total = usage?.totalTokens
  return <Tooltip>
    <TooltipTrigger asChild>
      <button type="button" data-slot={`${scope}-token-usage-button`} className="shrink-0 whitespace-nowrap rounded-md px-2 py-1 text-xs tabular-nums text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" aria-label={`${label}: ${format(total, language)}`}>
        <span className="hidden sm:inline">{label} </span><span className="sm:hidden">{shortLabel} </span>{typeof total === 'number' ? formatTokenMillions(total) : '—'}
      </button>
    </TooltipTrigger>
    <TooltipContent side="bottom" align="end" className="space-y-1 text-xs">
      <div className="font-medium">{label}</div>
      <div>{t.tokenUsage.input}{separator}{format(usage?.inputTokens, language)}</div>
      <div>{t.tokenUsage.output}{separator}{format(usage?.outputTokens, language)}</div>
      <div>{t.tokenUsage.cacheRead}{separator}{format(usage?.cacheReadTokens, language)}</div>
      <div className="border-t border-border/60 pt-1 font-medium">{t.tokenUsage.total}{separator}{format(total, language)}</div>
      {scope === 'project' ? <div className="max-w-64 border-t border-border/60 pt-1 text-muted-foreground">{t.tokenUsage.projectRecordedOnly}</div> : null}
    </TooltipContent>
  </Tooltip>
}

function formatTokenMillions(value: number) {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1).replace(/\.0$/, '')}K`
  return String(value)
}
