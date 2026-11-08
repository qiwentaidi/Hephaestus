'use client'

import { useId } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useI18n } from '@/lib/i18n'
import { DEFAULT_TCP_HEURISTIC_CONFIG, type TCPHeuristicConfig, validateTCPHeuristicConfig } from '@/lib/tcp-heuristic'

export function HostDiscoverySettings({ aliveDetection, onAliveDetectionChange, value, onChange, portRange }: {
  aliveDetection: boolean
  onAliveDetectionChange: (enabled: boolean) => void
  value: TCPHeuristicConfig
  onChange: (value: Partial<TCPHeuristicConfig>) => void
  portRange: string
}) {
  const id = useId()
  const { t } = useI18n()
  const discovery = t.vulnScan.newScan.hostDiscovery
  const subnet = t.vulnScan.newScan.subnetDiscovery
  const text = t.vulnScan.newScan.tcpHeuristic
  const error = validateTCPHeuristicConfig(value, portRange)

  return (
    <section aria-labelledby={`${id}-title`} className="rounded-lg border border-border p-4 sm:p-5">
      <h3 id={`${id}-title`} className="text-base font-medium">{discovery.title}</h3>
      <div className="space-y-4 border-b border-border py-4">
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`${id}-subnet`} className="text-sm font-medium">{subnet.title}</Label>
            <p id={`${id}-subnet-help`} className="text-xs leading-5 text-muted-foreground">{subnet.description}</p>
          </div>
          <Switch id={`${id}-subnet`} className="shrink-0" aria-describedby={`${id}-subnet-help`} aria-controls={`${id}-subnet-config`} checked={value.subnetDiscoveryEnabled} onCheckedChange={(subnetDiscoveryEnabled) => onChange({ subnetDiscoveryEnabled, ...(subnetDiscoveryEnabled ? { tcpHeuristicEnabled: true } : {}) })} />
        </div>
        {value.subnetDiscoveryEnabled && (
          <div id={`${id}-subnet-config`} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor={`${id}-subnet-mode`}>{subnet.mode}</Label>
              <Select value={value.subnetDiscoveryMode} onValueChange={(subnetDiscoveryMode: 'fast' | 'full') => onChange({ subnetDiscoveryMode })}>
                <SelectTrigger id={`${id}-subnet-mode`} aria-describedby={`${id}-subnet-mode-help`} className="w-full"><SelectValue /></SelectTrigger>
                <SelectContent><SelectItem value="fast">{subnet.fastRecommended}</SelectItem><SelectItem value="full">{subnet.full}</SelectItem></SelectContent>
              </Select>
              <p id={`${id}-subnet-mode-help`} className="text-xs leading-5 text-muted-foreground">{value.subnetDiscoveryMode === 'fast' ? subnet.fastHelp : subnet.fullHelp}</p>
              <details className="rounded-md border border-border p-3 text-xs leading-5">
                <summary className="cursor-pointer rounded-sm font-medium text-foreground focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-primary">{subnet.exampleTitle}</summary>
                <div className="mt-3 space-y-2 text-muted-foreground">
                  <p>{subnet.exampleIntro}</p>
                  <ul className="list-disc space-y-2 pl-4">
                    <li><strong className="font-medium text-foreground">{subnet.fast}</strong>{subnet.exampleFast}</li>
                    <li><strong className="font-medium text-foreground">{subnet.full}</strong>{subnet.exampleFull}</li>
                  </ul>
                  <p>{subnet.exampleMiss}</p>
                  <p>{subnet.exampleScope}</p>
                </div>
              </details>
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor={`${id}-suffixes`}>{subnet.suffixes}</Label>
                <Input id={`${id}-suffixes`} className="font-mono" value={value.subnetDiscoveryHostSuffixes} placeholder="1,2,254" onChange={(event) => onChange({ subnetDiscoveryHostSuffixes: event.target.value })} aria-invalid={error === 'subnetSuffixError'} />
              </div>
              <div className="space-y-2">
                <Label htmlFor={`${id}-samples`}>{subnet.randomSamples}</Label>
                <Input id={`${id}-samples`} type="number" min={0} max={32} step={1} value={value.subnetDiscoveryRandomSamples} onChange={(event) => onChange({ subnetDiscoveryRandomSamples: Number(event.target.value) })} aria-invalid={error === 'subnetRandomError'} />
              </div>
            </div>
            <p className="text-xs leading-5 text-muted-foreground">{subnet.shared}</p>
          </div>
        )}
      </div>
      <div className="flex items-center justify-between gap-4 py-4">
        <div className="min-w-0 space-y-1">
          <Label htmlFor={`${id}-ping`} className="text-sm font-medium">{discovery.pingTitle}</Label>
          <p id={`${id}-ping-help`} className="text-xs leading-5 text-muted-foreground">{discovery.pingDescription}</p>
        </div>
        <Switch id={`${id}-ping`} className="shrink-0" aria-describedby={`${id}-ping-help`} checked={aliveDetection} onCheckedChange={onAliveDetectionChange} />
      </div>
      <div className="space-y-4 border-t border-border pt-4">
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0 space-y-1">
            <Label htmlFor={`${id}-enabled`} className="text-sm font-medium">{text.title}</Label>
            <p id={`${id}-help`} className="text-xs leading-5 text-muted-foreground">{text.description}</p>
          </div>
          <Switch id={`${id}-enabled`} className="shrink-0 disabled:opacity-100" aria-describedby={`${id}-help`} aria-controls={`${id}-config`} disabled={value.subnetDiscoveryEnabled} checked={value.tcpHeuristicEnabled || value.subnetDiscoveryEnabled} onCheckedChange={(tcpHeuristicEnabled) => onChange({ tcpHeuristicEnabled })} />
        </div>
        {(value.tcpHeuristicEnabled || value.subnetDiscoveryEnabled) && (
          <div id={`${id}-config`} className="space-y-3">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
              <div className="space-y-2">
                <Label htmlFor={`${id}-ports`}>{text.ports}</Label>
                <Input id={`${id}-ports`} className="font-mono" value={value.tcpHeuristicPorts} placeholder={DEFAULT_TCP_HEURISTIC_CONFIG.tcpHeuristicPorts} onChange={(event) => onChange({ tcpHeuristicPorts: event.target.value })} aria-invalid={error === 'portsError'} />
              </div>
              <div className="space-y-2">
                <Label htmlFor={`${id}-timeout`}>{text.timeout}</Label>
                <Input id={`${id}-timeout`} type="number" min={0.1} max={30} step={0.1} value={value.tcpHeuristicTimeoutMs ? value.tcpHeuristicTimeoutMs / 1000 : ''} onChange={(event) => onChange({ tcpHeuristicTimeoutMs: Math.round(Number(event.target.value) * 1000) })} aria-invalid={error === 'timeoutError'} />
              </div>
            </div>
            {error && <p role="alert" className="text-sm text-destructive">{text[error]}</p>}
          </div>
        )}
      </div>
    </section>
  )
}
