'use client'

import { useI18n } from '@/lib/i18n'

export default function ToolCallCount({ count }: { count?: number }) {
  const { t, language } = useI18n()
  const value = count === undefined ? '—' : count.toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US')
  return <span className="inline-flex shrink-0 whitespace-nowrap rounded-full bg-muted/60 px-2 py-0.5 text-[11px] font-normal tabular-nums text-muted-foreground">
    {t.mcpConnections.callCount.replace('{count}', value)}
  </span>
}
