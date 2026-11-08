import { useMemo, useState } from 'react'
import { useRouter } from 'next/navigation'
import { AlertTriangle, CheckCircle2, Plus, RotateCcw, Settings, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { useI18n } from '@/lib/i18n'
import type { SpaceEngineConfig } from '@/lib/config'
import {
  cloneDefaultDNSServers,
  isSubdomainEngineConfigured,
  SPACE_ENGINES,
  SPACE_ENGINE_NAMES,
  type SpaceEngineName,
  type SubdomainConfigValue,
} from './subdomain-config'

interface SubdomainConfigPanelProps {
  value: SubdomainConfigValue
  onChange: (value: SubdomainConfigValue) => void
  spaceEngineConfig: SpaceEngineConfig | null
  isSavingDnsServers?: boolean
  onConfigureApi?: () => void
  description?: string
}

export function SubdomainConfigPanel({
  value,
  onChange,
  spaceEngineConfig,
  isSavingDnsServers = false,
  onConfigureApi,
  description,
}: SubdomainConfigPanelProps) {
  const { t } = useI18n()
  const router = useRouter()
  const [addDnsServerOpen, setAddDnsServerOpen] = useState(false)
  const [newDnsServer, setNewDnsServer] = useState({ address: '', name: '' })

  const selectedEngines = value.appendEngines as SpaceEngineName[]
  const isAllSelected = selectedEngines.length === SPACE_ENGINE_NAMES.length
  const isPartiallySelected = selectedEngines.length > 0 && !isAllSelected
  const missingEngines = useMemo(
    () => selectedEngines.filter((engineName) => {
      const engine = SPACE_ENGINES.find((item) => item.name === engineName)
      return Boolean(engine?.requiresApi) && !isSubdomainEngineConfigured(spaceEngineConfig, engineName)
    }),
    [selectedEngines, spaceEngineConfig],
  )

  const update = (patch: Partial<SubdomainConfigValue>) => onChange({ ...value, ...patch })

  const handleEngineToggle = (engineName: SpaceEngineName, checked: boolean) => {
    update({
      appendEngines: checked
        ? Array.from(new Set([...selectedEngines, engineName]))
        : selectedEngines.filter((item) => item !== engineName),
    })
  }

  const handleDnsServerToggle = (index: number, enabled: boolean) => {
    update({
      dnsServers: value.dnsServers.map((server, currentIndex) => (
        currentIndex === index ? { ...server, enabled } : server
      )),
    })
  }

  const resetDnsServers = () => update({ dnsServers: cloneDefaultDNSServers() })

  const confirmAddDnsServer = () => {
    const address = newDnsServer.address.trim()
    if (!address) {
      return
    }
    update({
      dnsServers: [
        ...value.dnsServers,
        {
          address,
          name: newDnsServer.name.trim() || t.subdomain.configSheet.customDns,
          enabled: true,
        },
      ],
    })
    setNewDnsServer({ address: '', name: '' })
    setAddDnsServerOpen(false)
  }

  const removeDnsServer = (index: number) => {
    if (value.dnsServers.length <= 1) return
    update({ dnsServers: value.dnsServers.filter((_, currentIndex) => currentIndex !== index) })
  }

  return (
    <div className="rounded-lg border border-border/70 bg-muted/20 p-4 space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">{t.vulnScan.taskManagement.subdomain}</h3>
          <p className="mt-1 text-xs text-muted-foreground">
            {description || t.vulnScan.createTaskDialog.taskTypes.subdomainDesc}
          </p>
        </div>
        {onConfigureApi && (
          <Button type="button" variant="outline" size="sm" onClick={() => onConfigureApi ? onConfigureApi() : router.push('/settings')}>
            <Settings className="mr-2 h-4 w-4" />
            {t.subdomain.configSheet.configApi}
          </Button>
        )}
      </div>

      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Label>{t.subdomain.configSheet.selectEngines}</Label>
          <div className="flex items-center gap-2">
            <Checkbox
              checked={isPartiallySelected ? 'indeterminate' : isAllSelected}
              onCheckedChange={(checked) => update({ appendEngines: checked === true ? [...SPACE_ENGINE_NAMES] : [] })}
            />
            <Label className="text-sm font-normal">
              {t.subdomain.configSheet.selectAll}
              {isPartiallySelected && <span className="ml-1 text-muted-foreground">({t.subdomain.configSheet.partiallySelected})</span>}
            </Label>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
          {SPACE_ENGINES.map((engine) => {
            const engineName = engine.name
            const checked = selectedEngines.includes(engineName)
            const hasConfig = !engine.requiresApi || isSubdomainEngineConfigured(spaceEngineConfig, engineName)
            return (
              <div key={engineName} className="flex min-w-0 items-center gap-2 rounded-lg border bg-background/60 p-2 hover:bg-muted/50">
                <Checkbox className="shrink-0" checked={checked} onCheckedChange={(checkedValue) => handleEngineToggle(engineName, checkedValue === true)} />
                <div className="flex min-w-0 flex-1 items-center gap-2">
                  <Label className="flex min-w-0 cursor-pointer items-center gap-1 text-sm font-normal">
                    <span className="min-w-0 truncate" title={engineName}>{engineName}</span>
                    {engine.recommended && <Badge variant="default" className="h-4 shrink-0 px-1 py-0 text-xs">{t.common.recommended}</Badge>}
                    {!engine.requiresApi && <Badge variant="secondary" className="h-4 shrink-0 px-1 py-0 text-xs">{t.subdomain.configSheet.free}</Badge>}
                  </Label>
                  {checked && engine.requiresApi && !hasConfig && (
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild><AlertTriangle className="h-4 w-4 shrink-0 text-destructive" /></TooltipTrigger>
                        <TooltipContent><p>{String(t.subdomain.configSheet.apiNotConfigured).replace('{engine}', engineName)}</p></TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                  {engine.requiresApi && hasConfig && (
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild><CheckCircle2 className="h-4 w-4 shrink-0 text-green-500" /></TooltipTrigger>
                        <TooltipContent><p>{String(t.subdomain.configSheet.apiConfigured).replace('{engine}', engineName)}</p></TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  )}
                </div>
              </div>
            )
          })}
        </div>

        {missingEngines.length > 0 && (
          <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-3">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 text-destructive" />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium text-destructive">
                  {String(t.subdomain.configSheet.missingEngines).replace('{engines}', missingEngines.join(', '))}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">{t.subdomain.configSheet.missingEnginesHint}</p>
              </div>
            </div>
          </div>
        )}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="space-y-2">
          <Label>{t.subdomain.configSheet.threadCount}</Label>
          <Input type="number" min={1} value={value.thread} onChange={(event) => update({ thread: Number(event.target.value) || 1 })} />
        </div>
        <div className="space-y-2">
          <Label>{t.subdomain.configSheet.resolveTimeout}</Label>
          <Input type="number" min={1} value={value.timeout} onChange={(event) => update({ timeout: Number(event.target.value) || 1 })} />
        </div>
      </div>

      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Label>{t.subdomain.configSheet.dnsServers}</Label>
          <div className="flex flex-wrap items-center gap-2">
            <Button type="button" variant="outline" size="sm" onClick={resetDnsServers} disabled={isSavingDnsServers}>
              <RotateCcw className="mr-2 h-4 w-4" />
              {t.subdomain.configSheet.resetDefaultDnsServers}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setAddDnsServerOpen(true)} disabled={isSavingDnsServers}>
              <Plus className="mr-2 h-4 w-4" />
              {t.subdomain.configSheet.addDnsServer}
            </Button>
          </div>
        </div>
        <div className="space-y-2">
          {value.dnsServers.map((server, index) => (
            <div key={`${server.address || 'dns'}-${index}`} className="flex min-h-8 min-w-0 items-center gap-2">
              <Checkbox checked={server.enabled} disabled={isSavingDnsServers} onCheckedChange={(checked) => handleDnsServerToggle(index, checked === true)} />
              <span className="min-w-0 truncate text-base">{server.address}{server.name ? ` (${server.name})` : ''}</span>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button type="button" variant="ghost" size="icon" className="h-8 w-8 shrink-0 text-muted-foreground hover:text-destructive" onClick={() => removeDnsServer(index)} disabled={isSavingDnsServers || value.dnsServers.length <= 1} aria-label={t.subdomain.configSheet.deleteDnsServer}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent><p>{t.subdomain.configSheet.deleteDnsServer}</p></TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          ))}
        </div>
      </div>

      <Dialog open={addDnsServerOpen} onOpenChange={setAddDnsServerOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t.subdomain.configSheet.addDnsServer}</DialogTitle>
            <DialogDescription>{t.subdomain.configSheet.dnsServers}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label>{t.subdomain.configSheet.dnsServerAddress}</Label>
              <Input value={newDnsServer.address} onChange={(event) => setNewDnsServer((prev) => ({ ...prev, address: event.target.value }))} placeholder="223.6.6.6:53" autoFocus />
            </div>
            <div className="space-y-2">
              <Label>{t.subdomain.configSheet.dnsServerName}</Label>
              <Input value={newDnsServer.name} onChange={(event) => setNewDnsServer((prev) => ({ ...prev, name: event.target.value }))} placeholder={t.subdomain.configSheet.customDns} />
            </div>
            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => setAddDnsServerOpen(false)}>{t.common.cancel}</Button>
              <Button type="button" onClick={confirmAddDnsServer}>{t.common.confirm}</Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
