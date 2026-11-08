import { TooltipContent } from '@/components/ui/tooltip'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import type { StageEvent } from '../types'

type StageLabels = ReturnType<typeof useI18n>['t']['vulnScan']['scanStages']

export function scanStageLabel(event: StageEvent, labels: StageLabels) {
  switch (event.stage) {
    case 'portscan:subnets':
      return labels.subnetDiscovery
    case 'portscan:alive':
      return event.label.startsWith('网段预探测') ? labels.subnetDiscovery : labels.aliveDetection
    case 'portscan:scan': {
      const mode = event.label.match(/\b(SYN|CONNECT)$/i)?.[1]
      return mode ? `${labels.portScan} · ${mode.toUpperCase()}` : labels.portScan
    }
    case 'portscan:complete':
      return event.label.includes('未确认可扫描主机') ? labels.noHosts : labels.finished
    default:
      return event.label
  }
}

export function ScanStageTooltipContent({ events }: { events?: StageEvent[] }) {
  const { t } = useI18n()
  const labels = t.vulnScan.scanStages
  const visibleEvents = events?.slice(-8) ?? []

  return (
    <TooltipContent side="top" align="center" className="w-[240px] max-w-[calc(100vw-1rem)] p-3 text-left">
      <div className="mb-1.5 font-medium">{labels.title}</div>
      {visibleEvents.length > 0 ? (
        <ol className="space-y-0.5 text-[11px] leading-5">
          {visibleEvents.map((event, index) => {
            const date = new Date(event.time)
            const time = Number.isNaN(date.getTime()) ? '—' : date.toLocaleTimeString('zh-CN', { hour12: false })

            return (
              <li
                key={`${event.stage}-${index}`}
                className={cn(
                  'grid grid-cols-[auto_minmax(0,1fr)] gap-x-2',
                  index === visibleEvents.length - 1 ? 'font-medium text-foreground' : 'text-muted-foreground',
                )}
              >
                <span className="font-mono tabular-nums">{time}</span>
                <span className="whitespace-normal wrap-anywhere">{scanStageLabel(event, labels)}</span>
              </li>
            )
          })}
        </ol>
      ) : (
        <div className="text-muted-foreground">{labels.empty}</div>
      )}
    </TooltipContent>
  )
}
