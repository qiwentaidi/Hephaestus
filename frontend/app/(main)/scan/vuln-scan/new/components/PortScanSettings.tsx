import { useCallback, useEffect, useMemo, useState } from 'react'
import { HostDiscoverySettings } from './HostDiscoverySettings'
import type { TCPHeuristicConfig } from '@/lib/tcp-heuristic'
import { fetchWithAuth } from '@/lib/api'
import toast from '@/lib/toast'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  builtInPortGroupOptions,
  findPortGroupByText,
  findPortGroupByValue,
  mergePortGroupOptions,
  type PortGroupOption,
} from "../../utils/portGroup"
import type { SYNCapabilityStatus, TCPDiscoveryMode } from '../../types'
import { Cable, Download, Plus, RefreshCw, ShieldAlert, ShieldCheck, Trash2, X, Zap } from 'lucide-react'
import { useI18n } from '@/lib/i18n'

const NPCAP_DOWNLOAD_URL = 'https://npcap.com/dist/npcap-1.60.exe'

interface PortScanSettingsProps {
  portGroup: string
  setPortGroup: (val: string) => void
  portRange: string
  setPortRange: (val: string) => void
  threads: number
  setThreads: (val: number) => void
  portTimeout: number
  setPortTimeout: (val: number) => void
  tcpHeuristic: TCPHeuristicConfig
  setTcpHeuristic: (value: Partial<TCPHeuristicConfig>) => void
  aliveDetection: boolean
  setAliveDetection: (val: boolean) => void
  tcpDiscoveryMode: TCPDiscoveryMode
  setTcpDiscoveryMode: (val: TCPDiscoveryMode) => void
  synRate: number
  setSynRate: (val: number) => void
  synCapability: SYNCapabilityStatus | null
  loadingSynCapability: boolean
  authorizingSyn: boolean
  desktopRuntime: boolean
  runMode: 'auto' | 'local' | 'agent'
  onRefreshSynCapability: () => void | Promise<void>
  onAuthorizeSYN: () => void | Promise<void>
  // 模板相关
  scanTemplates?: any[]
  selectedTemplateId?: string
  setSelectedTemplateId?: (val: string) => void
  onApplyTemplate?: (id: string) => void
  onClearTemplate?: () => void
}

export function PortScanSettings({
  portGroup,
  setPortGroup,
  portRange,
  setPortRange,
  threads,
  setThreads,
  portTimeout,
  setPortTimeout,
  tcpHeuristic,
  setTcpHeuristic,
  aliveDetection,
  setAliveDetection,
  tcpDiscoveryMode,
  setTcpDiscoveryMode,
  synRate,
  setSynRate,
  synCapability,
  loadingSynCapability,
  authorizingSyn,
  desktopRuntime,
  runMode,
  onRefreshSynCapability,
  onAuthorizeSYN,
  scanTemplates = [],
  selectedTemplateId = '',
  setSelectedTemplateId,
  onApplyTemplate,
  onClearTemplate,
}: PortScanSettingsProps) {
  const { t } = useI18n()
  const [customPortGroups, setCustomPortGroups] = useState<PortGroupOption[]>([])
  const [loadingPortGroups, setLoadingPortGroups] = useState(false)
  const [showPortGroupDialog, setShowPortGroupDialog] = useState(false)
  const [newPortGroupName, setNewPortGroupName] = useState('')
  const [newPortGroupPorts, setNewPortGroupPorts] = useState('')
  const [savingPortGroup, setSavingPortGroup] = useState(false)
  const [deletingPortGroupId, setDeletingPortGroupId] = useState<string | null>(null)
  const currentCustomGroup = useMemo(
    () => customPortGroups.find((group) => group.text === portGroup) || null,
    [customPortGroups, portGroup]
  )

  const portGroupOptions = useMemo(
    () => mergePortGroupOptions(customPortGroups),
    [customPortGroups]
  )

  const synAuthorizationNotice = useMemo(() => {
    if (tcpDiscoveryMode !== 'syn') {
      return null
    }

    if (loadingSynCapability || (!synCapability && runMode !== 'agent')) {
      return {
        icon: RefreshCw,
        title: t.vulnScan.newScan.synScan.checkingTitle,
        description: t.vulnScan.newScan.synScan.checkingDesc,
        className: 'border-border bg-muted/30 text-muted-foreground',
        canAuthorize: false,
      }
    }

    if (synCapability?.authorized) {
      return null
    }

    if (!desktopRuntime && runMode === 'agent') {
      return {
        icon: ShieldAlert,
        title: t.vulnScan.newScan.synScan.agentTitle,
        description: t.vulnScan.newScan.synScan.agentDesc,
        className: 'border-amber-500/30 bg-amber-500/10 text-amber-900 dark:text-amber-100',
        canAuthorize: false,
      }
    }

    if (synCapability && !synCapability.supported) {
      return {
        icon: ShieldAlert,
        title: t.vulnScan.newScan.synScan.unsupportedTitle,
        description: synCapability.message || t.vulnScan.newScan.synScan.unsupportedDesc,
        className: 'border-border bg-muted/30 text-muted-foreground',
        canAuthorize: false,
      }
    }

    if (!desktopRuntime) {
      return {
        icon: ShieldAlert,
        title: t.vulnScan.newScan.synScan.serverTitle,
        description: t.vulnScan.newScan.synScan.serverDesc,
        className: 'border-amber-500/30 bg-amber-500/10 text-amber-900 dark:text-amber-100',
        canAuthorize: false,
      }
    }

    return {
      icon: ShieldAlert,
      title: t.vulnScan.newScan.synScan.fallbackTitle,
      description: t.vulnScan.newScan.synScan.fallbackDesc,
      className: 'border-amber-500/30 bg-amber-500/10 text-amber-900 dark:text-amber-100',
      canAuthorize: true,
    }
  }, [desktopRuntime, loadingSynCapability, runMode, synCapability, t, tcpDiscoveryMode])

  const SynAuthorizationIcon = synAuthorizationNotice?.icon
  const canDownloadNpcap = Boolean(
    synCapability?.platform === 'windows' && synCapability && !synCapability.supported
  )

  const fetchCustomPortGroups = useCallback(async () => {
    const response = await fetchWithAuth('/api/v1/config/port-groups', { cache: 'no-store' })
    const data = await response.json()
    if (!response.ok || !data.success) {
      throw new Error(data.error || t.vulnScan.newScan.portGroups.loadFailed)
    }

    return Array.isArray(data.data)
      ? data.data.map((item: any) => ({
          id: String(item.id || ''),
          text: String(item.name || ''),
          value: String(item.ports || ''),
        })) as PortGroupOption[]
      : []
  }, [t.vulnScan.newScan.portGroups.loadFailed])

  const loadCustomPortGroups = useCallback(async () => {
    try {
      setLoadingPortGroups(true)
      setCustomPortGroups(await fetchCustomPortGroups())
    } catch (error: any) {
      toast.error(error.message || t.vulnScan.newScan.portGroups.loadFailed)
    } finally {
      setLoadingPortGroups(false)
    }
  }, [fetchCustomPortGroups, t.vulnScan.newScan.portGroups.loadFailed])

  useEffect(() => {
    let cancelled = false

    const loadInitialPortGroups = async () => {
      try {
        setLoadingPortGroups(true)
        const nextGroups = await fetchCustomPortGroups()
        if (!cancelled) {
          setCustomPortGroups(nextGroups)
        }
      } catch (error: any) {
        if (!cancelled) {
          toast.error(error.message || t.vulnScan.newScan.portGroups.loadFailed)
        }
      } finally {
        if (!cancelled) {
          setLoadingPortGroups(false)
        }
      }
    }

    void loadInitialPortGroups()

    return () => {
      cancelled = true
    }
  }, [fetchCustomPortGroups, t.vulnScan.newScan.portGroups.loadFailed])

  const handleSaveCustomPortGroup = async () => {
    if (!newPortGroupName.trim()) {
      toast.error(t.vulnScan.newScan.portGroups.nameRequired)
      return
    }
    if (!newPortGroupPorts.trim()) {
      toast.error(t.vulnScan.newScan.portGroups.portsRequired)
      return
    }

    try {
      setSavingPortGroup(true)
      const response = await fetchWithAuth('/api/v1/config/port-groups', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          name: newPortGroupName.trim(),
          ports: newPortGroupPorts.trim(),
        }),
      })
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || t.vulnScan.newScan.portGroups.saveFailed)
      }

      const savedGroup: PortGroupOption = {
        id: String(data.data?.id || ''),
        text: String(data.data?.name || ''),
        value: String(data.data?.ports || ''),
      }

      setCustomPortGroups((prev) => {
        const next = prev.filter((item) => item.id !== savedGroup.id)
        next.push(savedGroup)
        return next.sort((a, b) => a.text.localeCompare(b.text, 'zh-CN'))
      })
      setPortGroup(savedGroup.text)
      setPortRange(savedGroup.value)
      setNewPortGroupName('')
      setNewPortGroupPorts('')
      setShowPortGroupDialog(false)
      toast.success(t.vulnScan.newScan.portGroups.saveSuccess)
    } catch (error: any) {
      toast.error(error.message || t.vulnScan.newScan.portGroups.saveFailed)
    } finally {
      setSavingPortGroup(false)
    }
  }

  const handleDeleteCustomPortGroup = async (group: PortGroupOption) => {
    try {
      setDeletingPortGroupId(group.id)
      const response = await fetchWithAuth(`/api/v1/config/port-groups/${group.id}`, {
        method: 'DELETE',
      })
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || t.vulnScan.newScan.portGroups.deleteFailed)
      }

      setCustomPortGroups((prev) => prev.filter((item) => item.id !== group.id))
      if (portGroup === group.text) {
        const fallback = builtInPortGroupOptions[0]
        setPortGroup(fallback.text)
        setPortRange(fallback.value)
      }
      toast.success(t.vulnScan.newScan.portGroups.deleteSuccess)
    } catch (error: any) {
      toast.error(error.message || t.vulnScan.newScan.portGroups.deleteFailed)
    } finally {
      setDeletingPortGroupId(null)
    }
  }

  return (
    <div className="space-y-6 pt-4 border-t">
      {/* 模板选择 */}
      {scanTemplates.length > 0 && (
        <div className="space-y-2">
          <Label>{t.vulnScan.newScan.applyTemplate}</Label>
          <div className="flex items-center gap-2">
            <Select
              value={selectedTemplateId}
              onValueChange={(value) => {
                setSelectedTemplateId?.(value)
                if (value && onApplyTemplate) {
                  onApplyTemplate(value)
                }
              }}
            >
              <SelectTrigger className="flex-1">
                <SelectValue placeholder={t.vulnScan.newScan.selectTemplate} />
              </SelectTrigger>
              <SelectContent>
                {scanTemplates.map((template) => (
                  <SelectItem key={template.id} value={template.id}>
                    {template.name}
                    {template.description && (
                      <span className="text-muted-foreground ml-2">
                        - {template.description}
                      </span>
                    )}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {selectedTemplateId && (
              <Button
                type="button"
                variant="outline"
                size="icon"
                onClick={onClearTemplate}
                title={t.vulnScan.newScan.clearTemplate}
              >
                <X className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      )}

      {/* 端口范围 */}
      <div className="space-y-2">
        <Label>{t.vulnScan.newScan.portRange}</Label>
        <div className="space-y-3">
          <Select
            value={portGroup}
            onValueChange={(value) => {
              if (value === '__create_custom_group__') {
                setNewPortGroupName(currentCustomGroup?.text || '')
                setNewPortGroupPorts(portRange)
                setShowPortGroupDialog(true)
                return
              }
              setPortGroup(value)
              const option = findPortGroupByText(portGroupOptions, value)
              if (option) {
                setPortRange(option.value)
              }
            }}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder={t.vulnScan.newScan.selectPortGroup} />
            </SelectTrigger>
            <SelectContent>
              {portGroupOptions.map((option) => (
                <SelectItem key={option.text} value={option.text}>
                  <div className="flex w-full items-center justify-between gap-2">
                    <span>{option.text}</span>
                    {!option.builtIn && (
                      <Badge variant="secondary" className="h-5 px-1.5 text-[10px] bg-orange-500/10 text-orange-600 border border-orange-500/20">
                        {t.vulnScan.newScan.portGroups.custom}
                      </Badge>
                    )}
                  </div>
                </SelectItem>
              ))}
              <SelectItem value="__create_custom_group__">
                <div className="flex items-center gap-2 text-primary">
                  <Plus className="h-4 w-4" />
                  <span>{t.vulnScan.newScan.portGroups.add}</span>
                </div>
              </SelectItem>
            </SelectContent>
          </Select>
          <Textarea
            value={portRange}
            onChange={(e) => {
              const nextValue = e.target.value
              setPortRange(nextValue)
              // 检查是否匹配某个预设组
              const matchedOption = findPortGroupByValue(portGroupOptions, nextValue)
              const selectedOption = findPortGroupByText(portGroupOptions, portGroup)
              if (matchedOption) {
                setPortGroup(matchedOption.text)
              } else if (selectedOption?.builtIn && selectedOption.value === '') {
                setPortGroup(selectedOption.text)
              } else if (currentCustomGroup) {
                setPortGroup(currentCustomGroup.text)
              } else if (portGroup) {
                setPortGroup('')
              }
            }}
            placeholder={t.vulnScan.newScan.portRangePlaceholder}
            className="min-h-[92px] max-h-[180px] overflow-y-auto resize-y font-mono"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {t.vulnScan.newScan.portRangeHelp}
        </p>
        {currentCustomGroup && (
          <div className="flex items-center justify-between gap-3 rounded-lg border border-orange-500/20 bg-orange-500/5 px-3 py-2">
            <div className="flex min-w-0 items-center gap-2">
              <span className="truncate text-sm font-medium">{currentCustomGroup.text}</span>
              <Badge variant="secondary" className="h-5 px-1.5 text-[10px] bg-orange-500/10 text-orange-600 border border-orange-500/20">
                {t.vulnScan.newScan.portGroups.custom}
              </Badge>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-8 px-2 text-destructive"
              disabled={deletingPortGroupId === currentCustomGroup.id}
              onClick={() => void handleDeleteCustomPortGroup(currentCustomGroup)}
            >
              <Trash2 className="h-4 w-4 mr-1" />
              {t.vulnScan.newScan.portGroups.delete}
            </Button>
          </div>
        )}
        {loadingPortGroups && (
          <p className="text-xs text-muted-foreground">{t.vulnScan.newScan.portGroups.loading}</p>
        )}
      </div>

      <div className="space-y-4 rounded-lg border p-4">
        <div className="space-y-1">
          <Label className="text-base">{t.vulnScan.newScan.synScan.discoveryMode}</Label>
        </div>

        <div className="grid gap-2 sm:grid-cols-2">
          <Button
            type="button"
            variant={tcpDiscoveryMode === 'syn' ? 'default' : 'outline'}
            className="h-auto justify-start gap-3 px-3 py-3 text-left"
            onClick={() => setTcpDiscoveryMode('syn')}
          >
            <Zap className="h-4 w-4 shrink-0" />
            <span className="flex min-w-0 flex-col items-start">
              <span className="text-sm font-medium leading-5">{t.vulnScan.newScan.synScan.synMode}</span>
              <span className="text-xs font-normal opacity-80">{t.vulnScan.newScan.synScan.synModeDesc}</span>
            </span>
          </Button>
          <Button
            type="button"
            variant={tcpDiscoveryMode === 'connect' ? 'default' : 'outline'}
            className="h-auto justify-start gap-3 px-3 py-3 text-left"
            onClick={() => setTcpDiscoveryMode('connect')}
          >
            <Cable className="h-4 w-4 shrink-0" />
            <span className="flex min-w-0 flex-col items-start">
              <span className="text-sm font-medium leading-5">{t.vulnScan.newScan.synScan.connectMode}</span>
              <span className="text-xs font-normal opacity-80">{t.vulnScan.newScan.synScan.connectModeDesc}</span>
            </span>
          </Button>
        </div>

        {synAuthorizationNotice && SynAuthorizationIcon && (
          <div className={cn('flex flex-col gap-3 rounded-md border px-3 py-3 sm:flex-row sm:items-start', synAuthorizationNotice.className)}>
            <SynAuthorizationIcon className={cn('mt-0.5 h-4 w-4 shrink-0', loadingSynCapability && 'animate-spin')} />
            <div className="min-w-0 flex-1 space-y-1">
              <div className="text-sm font-medium">{synAuthorizationNotice.title}</div>
              <div className="text-xs leading-5 opacity-85">{synAuthorizationNotice.description}</div>
            </div>
            <div className="flex shrink-0 flex-wrap gap-2">
              {desktopRuntime && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void onRefreshSynCapability()}
                  disabled={loadingSynCapability || authorizingSyn}
                >
                  <RefreshCw className={cn('mr-1 h-3.5 w-3.5', loadingSynCapability && 'animate-spin')} />
                  {t.vulnScan.newScan.synScan.refresh}
                </Button>
              )}
              {canDownloadNpcap && (
                <Button asChild size="sm">
                  <a href={NPCAP_DOWNLOAD_URL} target="_blank" rel="noopener noreferrer">
                    <Download className="mr-1 h-3.5 w-3.5" />
                    {t.vulnScan.newScan.synScan.downloadNpcap}
                  </a>
                </Button>
              )}
              {synAuthorizationNotice.canAuthorize && (
                <Button
                  type="button"
                  size="sm"
                  onClick={() => void onAuthorizeSYN()}
                  disabled={authorizingSyn || loadingSynCapability}
                >
                  <ShieldCheck className="mr-1 h-3.5 w-3.5" />
                  {authorizingSyn ? t.vulnScan.newScan.synScan.authorizing : t.vulnScan.newScan.synScan.authorize}
                </Button>
              )}
            </div>
          </div>
        )}

      </div>

      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-2">
          <Label htmlFor="portScanRate">
            {tcpDiscoveryMode === 'syn'
              ? t.vulnScan.newScan.synScan.rate
              : t.vulnScan.newScan.synScan.connectRate}
          </Label>
          <Input
            id="portScanRate"
            type="number"
            min="1"
            max={tcpDiscoveryMode === 'syn' ? '100000' : '10000'}
            value={tcpDiscoveryMode === 'syn' ? synRate : threads}
            onChange={(e) => {
              const value = parseInt(e.target.value, 10)
              if (isNaN(value) || value < 1) {
                return
              }
              if (tcpDiscoveryMode === 'syn' && value <= 100000) {
                setSynRate(value)
              } else if (tcpDiscoveryMode === 'connect' && value <= 10000) {
                setThreads(value)
              }
            }}
            placeholder={tcpDiscoveryMode === 'syn' ? '3000' : '1-10000'}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="portTimeout">{t.vulnScan.newScan.timeout}</Label>
          <Input
            id="portTimeout"
            type="number"
            min="1"
            max="300"
            value={portTimeout}
            onChange={(e) => {
              const value = parseInt(e.target.value, 10)
              if (!isNaN(value) && value >= 1 && value <= 300) {
                setPortTimeout(value)
              }
            }}
            placeholder="1-300"
          />
        </div>
      </div>

      <HostDiscoverySettings
        aliveDetection={aliveDetection}
        onAliveDetectionChange={setAliveDetection}
        value={tcpHeuristic}
        onChange={setTcpHeuristic}
        portRange={portRange}
      />

      <Dialog open={showPortGroupDialog} onOpenChange={setShowPortGroupDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t.vulnScan.newScan.portGroups.dialogTitle}</DialogTitle>
            <DialogDescription>
              {t.vulnScan.newScan.portGroups.dialogDesc}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="custom-port-group-name">{t.vulnScan.newScan.portGroups.nameLabel}</Label>
              <Input
                id="custom-port-group-name"
                value={newPortGroupName}
                onChange={(e) => setNewPortGroupName(e.target.value)}
                placeholder={t.vulnScan.newScan.portGroups.namePlaceholder}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="custom-port-group-ports">{t.vulnScan.newScan.portGroups.portsLabel}</Label>
              <Textarea
                id="custom-port-group-ports"
                value={newPortGroupPorts}
                onChange={(e) => setNewPortGroupPorts(e.target.value)}
                placeholder={t.vulnScan.newScan.portGroups.portsPlaceholder}
                className="min-h-[92px] max-h-[180px] overflow-y-auto resize-y font-mono break-all whitespace-pre-wrap [overflow-wrap:anywhere]"
              />
            </div>
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowPortGroupDialog(false)}
                disabled={savingPortGroup}
              >
                {t.vulnScan.newScan.cancel}
              </Button>
              <Button
                type="button"
                onClick={() => void handleSaveCustomPortGroup()}
                disabled={savingPortGroup}
              >
                {savingPortGroup ? t.vulnScan.newScan.portGroups.saving : t.vulnScan.newScan.portGroups.save}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
