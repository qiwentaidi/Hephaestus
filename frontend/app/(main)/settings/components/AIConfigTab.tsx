'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  Bot,
  CheckCircle2,
  Loader2,
  MoreVertical,
  Plus,
  Power,
  RadioTower,
  Save,
  Settings2,
  Star,
  Trash2,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PasswordInput } from '@/components/ui/password-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  getAIConfig,
  saveAIConfig,
  testAIConfig,
  type AIConfig,
  type AIProviderConfig,
  type AIProviderHeader,
  type AIProviderProtocol,
} from '@/lib/api/ai-config'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'
import { cn } from '@/lib/utils'
import { AIModelSelect } from './AIModelSelect'

const DEFAULT_PROVIDER: AIProviderConfig = {
  requestTimeoutMinutes: 30,
  workbenchRequestsPerMinute: 0,
  workbenchRequestRateUnit: 'second',
  id: 'dashscope-default',
  name: 'DashScope Compatible',
  protocol: 'openai-compatible',
  enabled: false,
  api: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
  key: '',
  model: 'qwen-plus',
  headers: [],
  supportsTools: true,
  supportsStreaming: true,
}

const ROUTE_DEFAULT_VALUE = '__default__'
function requestRateUnit(provider: AIProviderConfig): 'second' | 'minute' {
  const rpm = provider.workbenchRequestsPerMinute || 0
  if (provider.workbenchRequestRateUnit === 'minute') return 'minute'
  if (provider.workbenchRequestRateUnit === 'second' && rpm % 60 === 0) return 'second'
  return rpm > 0 ? 'minute' : 'second'
}

function normalizeConfig(config?: AIConfig | null): AIConfig {
  const providers =
    config?.providers && config.providers.length > 0
      ? config.providers
      : [
          {
            ...DEFAULT_PROVIDER,
            requestTimeoutMinutes: config?.requestTimeoutMinutes || 30,
            enabled: config?.enabled ?? false,
            protocol: (config?.protocol as AIProviderProtocol) || DEFAULT_PROVIDER.protocol,
            api: config?.api || DEFAULT_PROVIDER.api,
            key: config?.key || '',
            model: config?.model || DEFAULT_PROVIDER.model,
            headers: config?.headers || [],
          },
        ]

  const usedProviderIDs = new Set<string>()
  const normalizedProviders = providers.map((provider, index) => {
    const providerID =
      provider.id && !usedProviderIDs.has(provider.id) ? provider.id : uniqueProviderID(usedProviderIDs)
    usedProviderIDs.add(providerID)
    return {
      ...DEFAULT_PROVIDER,
      ...provider,
      requestTimeoutMinutes: provider.requestTimeoutMinutes ?? 30,
      workbenchRequestsPerMinute: Math.max(0, provider.workbenchRequestsPerMinute || 0),
      workbenchRequestRateUnit: requestRateUnit(provider),
      id: providerID,
      name: provider.name ?? `AI Provider ${index + 1}`,
      protocol: provider.protocol || 'openai-compatible',
      api: provider.api ?? '',
      model: provider.model ?? '',
      headers: provider.headers || [],
    }
  })

  const defaultProviderId =
    config?.defaultProviderId && normalizedProviders.some((provider) => provider.id === config.defaultProviderId)
      ? config.defaultProviderId
      : normalizedProviders[0]?.id

  const defaultProvider = normalizedProviders.find((provider) => provider.id === defaultProviderId) || normalizedProviders[0]
  const rawRouting = config?.routing || {}
  const workbenchProviderId = defaultProviderId || ''
  const workbenchFallback =
    rawRouting.workbenchFallback &&
    rawRouting.workbenchFallback !== workbenchProviderId &&
    normalizedProviders.some((provider) => provider.id === rawRouting.workbenchFallback)
      ? rawRouting.workbenchFallback
      : ''

  return {
    // Global AI is always on; providers keep their individual enable switches.
    enabled: true,
    requestTimeoutMinutes: defaultProvider?.requestTimeoutMinutes ?? 30,
    protocol: defaultProvider?.protocol || 'openai-compatible',
    api: defaultProvider?.api || DEFAULT_PROVIDER.api,
    key: defaultProvider?.key || '',
    model: defaultProvider?.model || DEFAULT_PROVIDER.model,
    headers: defaultProvider?.headers || [],
    defaultProviderId,
    providers: normalizedProviders,
    routing: {
      workbenchFallback,
      workbenchFallbackEnabled: Boolean(rawRouting.workbenchFallbackEnabled),
    },
    workbenchRemoteDispatch: config?.workbenchRemoteDispatch ?? false,
  }
}

let providerIDSequence = 0

function createProviderID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `provider-${crypto.randomUUID()}`
  }
  providerIDSequence += 1
  return `provider-${Date.now().toString(36)}-${providerIDSequence}-${Math.random().toString(36).slice(2, 8)}`
}

function uniqueProviderID(used: Set<string>): string {
  let id = createProviderID()
  while (used.has(id)) {
    id = createProviderID()
  }
  return id
}

function createEmptyProvider(used: Set<string>): AIProviderConfig {
  return {
    requestTimeoutMinutes: 30,
    workbenchRequestsPerMinute: 0,
    workbenchRequestRateUnit: 'second',
    id: uniqueProviderID(used),
    name: '',
    protocol: 'openai-compatible',
    enabled: false,
    api: '',
    key: '',
    model: '',
    headers: [],
    supportsTools: true,
    supportsStreaming: true,
  }
}

function syncTopLevelConfig(config: AIConfig): AIConfig {
  const normalized = normalizeConfig(config)
  const provider =
    normalized.providers?.find((item) => item.id === normalized.defaultProviderId) ||
    normalized.providers?.[0] ||
    DEFAULT_PROVIDER

  return {
    ...normalized,
    protocol: provider.protocol,
    api: provider.api,
    key: provider.key,
    model: provider.model,
    headers: provider.headers || [],
  }
}

function withProviderNameFallbacks(config: AIConfig): AIConfig {
  const normalized = syncTopLevelConfig(config)
  return syncTopLevelConfig({
    ...normalized,
    providers: (normalized.providers || []).map((provider, index) => ({
      ...provider,
      name: provider.name.trim() || `AI Provider ${index + 1}`,
    })),
  })
}

function providerStatus(provider: AIProviderConfig) {
  return Boolean(provider.enabled && provider.api.trim() && provider.key.trim() && provider.model.trim())
}

function providerAPIHost(api: string): string {
  try {
    return new URL(api.trim()).hostname.toLowerCase()
  } catch {
    return ''
  }
}

function isOpenCodeProvider(api: string) {
  const host = providerAPIHost(api)
  return host === 'opencode.ai' || host.endsWith('.opencode.ai')
}

export function AIConfigTab() {
  const { t } = useI18n()
  const [config, setConfig] = useState<AIConfig>(() => normalizeConfig())
  const [selectedProviderId, setSelectedProviderId] = useState(DEFAULT_PROVIDER.id)
  const [saving, setSaving] = useState(false)
  const [testingProviderId, setTestingProviderId] = useState<string | null>(null)
  const [draggedProviderId, setDraggedProviderId] = useState<string | null>(null)
  const [dragOverProviderId, setDragOverProviderId] = useState<string | null>(null)

  const providers = useMemo(() => config.providers || [], [config.providers])
  const selectedProvider = providers.find((provider) => provider.id === selectedProviderId) || providers[0]
  const rateUnit = selectedProvider ? requestRateUnit(selectedProvider) : 'second'
  const rateValue = (selectedProvider?.workbenchRequestsPerMinute || 0) / (rateUnit === 'second' ? 60 : 1)
  const providerHeaders = selectedProvider?.headers || []
  const selectedIsDefault = Boolean(selectedProvider && selectedProvider.id === config.defaultProviderId)
  const enabledProviders = providers.filter(providerStatus)

  useEffect(() => {
    let cancelled = false

    const load = async () => {
      try {
        const loaded = normalizeConfig(await getAIConfig())
        if (cancelled) return
        setConfig(loaded)
        setSelectedProviderId(loaded.defaultProviderId || loaded.providers?.[0]?.id || DEFAULT_PROVIDER.id)
      } catch (error) {
        console.error('Failed to load AI config:', error)
        toast.error(t.aiConfig.loadFailed)
      }
    }

    void load()

    return () => {
      cancelled = true
    }
  }, [t.aiConfig.loadFailed])

  const updateConfig = (next: Partial<AIConfig>) => {
    setConfig((current) => syncTopLevelConfig({ ...current, ...next }))
  }

  const updateSelectedProvider = (next: Partial<AIProviderConfig>) => {
    if (!selectedProvider) return
    setConfig((current) => {
      const updatedProviders = (current.providers || []).map((provider) =>
        provider.id === selectedProvider.id ? { ...provider, ...next } : provider,
      )
      return syncTopLevelConfig({ ...current, providers: updatedProviders })
    })
  }

  const updateProvider = (providerId: string, next: Partial<AIProviderConfig>) => {
    setConfig((current) =>
      syncTopLevelConfig({
        ...current,
        providers: (current.providers || []).map((provider) =>
          provider.id === providerId ? { ...provider, ...next } : provider,
        ),
      }),
    )
  }

  const reorderProviders = (fromId: string, toId: string) => {
    if (!fromId || !toId || fromId === toId) return
    setConfig((current) => {
      const items = [...(current.providers || [])]
      const fromIndex = items.findIndex((provider) => provider.id === fromId)
      const toIndex = items.findIndex((provider) => provider.id === toId)
      if (fromIndex < 0 || toIndex < 0) return current
      const [moved] = items.splice(fromIndex, 1)
      items.splice(toIndex, 0, moved)
      return syncTopLevelConfig({ ...current, providers: items })
    })
  }

  const moveProvider = (providerId: string, offset: number) => {
    setConfig((current) => {
      const items = [...(current.providers || [])]
      const index = items.findIndex((provider) => provider.id === providerId)
      const target = index + offset
      if (index < 0 || target < 0 || target >= items.length) return current
      const [moved] = items.splice(index, 1)
      items.splice(target, 0, moved)
      return syncTopLevelConfig({ ...current, providers: items })
    })
  }

  const addProviderHeader = () => {
    if (!selectedProvider) return
    updateSelectedProvider({ headers: [...(selectedProvider.headers || []), { name: '', value: '' }] })
  }

  const updateProviderHeader = (index: number, next: Partial<AIProviderHeader>) => {
    if (!selectedProvider) return
    updateSelectedProvider({
      headers: (selectedProvider.headers || []).map((header, headerIndex) =>
        headerIndex === index ? { ...header, ...next } : header,
      ),
    })
  }

  const removeProviderHeader = (index: number) => {
    if (!selectedProvider) return
    updateSelectedProvider({
      headers: (selectedProvider.headers || []).filter((_, headerIndex) => headerIndex !== index),
    })
  }

  const addProvider = () => {
    setConfig((current) => {
      const existingProviders = current.providers || []
      const nextProvider = createEmptyProvider(new Set(existingProviders.map((provider) => provider.id)))
      const next = syncTopLevelConfig({
        ...current,
        providers: [...existingProviders, nextProvider],
        defaultProviderId: current.defaultProviderId || existingProviders[0]?.id || nextProvider.id,
      })
      setSelectedProviderId(nextProvider.id)
      return next
    })
  }

  const removeProvider = (providerId: string) => {
    if (providers.length <= 1) return
    const provider = providers.find((item) => item.id === providerId)
    if (!provider) return

    setConfig((current) => {
      const remaining = (current.providers || []).filter((item) => item.id !== providerId)
      const defaultProviderId =
        current.defaultProviderId === providerId ? remaining[0]?.id : current.defaultProviderId
      const routing = { ...current.routing }
      if (routing.workbenchFallback === providerId) {
        routing.workbenchFallback = ''
        routing.workbenchFallbackEnabled = false
      }
      if (selectedProviderId === providerId) {
        setSelectedProviderId(defaultProviderId || remaining[0]?.id || DEFAULT_PROVIDER.id)
      }
      return syncTopLevelConfig({ ...current, providers: remaining, defaultProviderId, routing })
    })
  }

  const removeSelectedProvider = () => {
    if (selectedProvider) removeProvider(selectedProvider.id)
  }

  const validateTimeouts = (nextConfig: AIConfig) => {
    for (const provider of nextConfig.providers || []) {
      const minutes = provider.requestTimeoutMinutes ?? 30
      if (!Number.isInteger(minutes) || minutes < 1 || minutes > 120) {
        setSelectedProviderId(provider.id)
        throw new Error(t.aiConfig.requestTimeoutInvalid)
      }
    }
  }

  const prepareHeaders = (nextConfig: AIConfig): AIConfig => {
    const providers = (nextConfig.providers || []).map((provider) => ({
      ...provider,
      headers: (provider.headers || [])
        .map((header) => ({ name: header.name.trim(), value: header.value.trim() }))
        .filter((header) => header.name || header.value),
    }))
    return syncTopLevelConfig({ ...nextConfig, providers })
  }

  const validateHeaders = (nextConfig: AIConfig) => {
    for (const provider of nextConfig.providers || []) {
      const seen = new Set<string>()
      for (const header of provider.headers || []) {
        if (!header.name) {
          if (!header.value) continue
          setSelectedProviderId(provider.id)
          throw new Error(t.aiConfig.headersNameRequired)
        }
        const key = header.name.toLowerCase()
        if (seen.has(key)) {
          setSelectedProviderId(provider.id)
          throw new Error(t.aiConfig.headersDuplicate.replace('{name}', header.name))
        }
        seen.add(key)
      }
    }
  }

  const persistConfig = async (nextConfig: AIConfig) => {
    const prepared = prepareHeaders(nextConfig)
    validateTimeouts(prepared)
    validateHeaders(prepared)
    const normalized = withProviderNameFallbacks(prepared)
    await saveAIConfig(normalized)
    setConfig(normalized)
    return normalized
  }

  const selectDefaultProvider = async (providerId: string) => {
    const provider = providers.find((item) => item.id === providerId)
    if (!provider || !providerStatus(provider)) {
      toast.error(t.aiConfig.fillProviderFirst)
      return
    }

    const nextConfig = withProviderNameFallbacks(syncTopLevelConfig({ ...config, defaultProviderId: providerId }))
    setSelectedProviderId(providerId)
    setSaving(true)
    try {
      await persistConfig(nextConfig)
      toast.success(t.common.configSaved)
    } catch (error: any) {
      toast.error(`${t.common.saveFailed}: ${error.message || ''}`)
    } finally {
      setSaving(false)
    }
  }

  const saveConfig = async () => {
    setSaving(true)
    try {
      await persistConfig(config)
      toast.success(t.common.configSaved)
    } catch (error: any) {
      toast.error(`${t.common.saveFailed}: ${error.message || ''}`)
    } finally {
      setSaving(false)
    }
  }

  const testProvider = async (provider: AIProviderConfig) => {
    if (!providerStatus(provider)) {
      toast.error(t.aiConfig.fillProviderFirst)
      return
    }

    setTestingProviderId(provider.id)
    try {
      await testAIConfig({
        enabled: true,
        protocol: provider.protocol,
        api: provider.api,
        key: provider.key,
        model: provider.model,
        defaultProviderId: provider.id,
        providers: [{ ...provider, enabled: true }],
        workbenchRemoteDispatch: config.workbenchRemoteDispatch,
      })
      toast.success(t.aiConfig.testSuccess)
    } catch (error: any) {
      toast.error(`${t.aiConfig.testFailed}: ${error.message || ''}`)
    } finally {
      setTestingProviderId(null)
    }
  }

  const workbenchProviderId = config.defaultProviderId || ''
  const fallbackProviderOptions = providers.map((provider, index) => ({
    value: provider.id,
    label: provider.name.trim() || `${t.aiConfig.unnamedProvider} ${index + 1}`,
    disabled:
      provider.id === workbenchProviderId ||
      !providerStatus(provider) ||
      provider.protocol !== 'openai-compatible',
  }))
  const fallbackEnabled = Boolean(config.routing?.workbenchFallbackEnabled)
  const fallbackProviderId = config.routing?.workbenchFallback || ROUTE_DEFAULT_VALUE

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-300">
      <div className="flex flex-col items-start justify-between gap-3 border-b py-3 sm:flex-row sm:items-center">
        <div>
          <h2 className="text-lg font-semibold tracking-tight">{t.aiConfig.title}</h2>
          <p className="text-xs text-muted-foreground">{t.aiConfig.description}</p>
        </div>
        <Button onClick={saveConfig} size="sm" className="h-8 shadow-sm" disabled={saving}>
          {saving ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <Save className="mr-1.5 h-3.5 w-3.5" />}
          {t.common.saveConfig}
        </Button>
      </div>

      <div className="grid gap-6 xl:grid-cols-[360px_minmax(0,1fr)]">
        <section className="min-w-0 space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <RadioTower className="h-4 w-4 text-primary/70" />
              <h3 className="text-sm font-semibold">{t.aiConfig.providers}</h3>
              <Badge variant="secondary" className="h-5">
                {enabledProviders.length}/{providers.length} {t.aiConfig.available}
              </Badge>
            </div>
            <Button size="sm" variant="outline" className="h-8" onClick={addProvider}>
              <Plus className="mr-1.5 h-3.5 w-3.5" />
              {t.aiConfig.addProvider}
            </Button>
          </div>

          <div className="grid gap-2">
            {providers.map((provider, index) => {
              const active = provider.id === selectedProvider?.id
              const complete = providerStatus(provider)
              const isDefault = provider.id === config.defaultProviderId
              const displayName = provider.name.trim() || `${t.aiConfig.unnamedProvider} ${index + 1}`
              const starTitle = isDefault
                ? t.aiConfig.currentDefault
                : complete
                  ? t.aiConfig.setAsDefault
                  : t.aiConfig.fillProviderFirst
              return (
                <div
                  key={provider.id}
                  className={cn('relative', draggedProviderId === provider.id && 'opacity-50')}
                  onDragOver={(event) => {
                    if (!draggedProviderId || draggedProviderId === provider.id) return
                    event.preventDefault()
                    event.dataTransfer.dropEffect = 'move'
                    setDragOverProviderId(provider.id)
                  }}
                  onDragLeave={(event) => {
                    const related = event.relatedTarget as Node | null
                    if (!related || !event.currentTarget.contains(related)) {
                      setDragOverProviderId((current) => (current === provider.id ? null : current))
                    }
                  }}
                  onDrop={(event) => {
                    event.preventDefault()
                    const fromId = draggedProviderId || event.dataTransfer.getData('text/plain')
                    reorderProviders(fromId, provider.id)
                    setDraggedProviderId(null)
                    setDragOverProviderId(null)
                  }}
                >
                  <button
                    type="button"
                    draggable
                    onDragStart={(event) => {
                      setDraggedProviderId(provider.id)
                      setDragOverProviderId(null)
                      event.dataTransfer.effectAllowed = 'move'
                      event.dataTransfer.setData('text/plain', provider.id)
                    }}
                    onDragEnd={() => {
                      setDraggedProviderId(null)
                      setDragOverProviderId(null)
                    }}
                    onKeyDown={(event) => {
                      if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
                      event.preventDefault()
                      moveProvider(provider.id, event.key === 'ArrowUp' ? -1 : 1)
                    }}
                    className={cn(
                      'min-h-[68px] w-full rounded-md border bg-card p-3 pr-10 text-left shadow-sm transition hover:border-primary/30 hover:shadow-md',
                      active && 'border-primary/60 ring-1 ring-primary/20',
                      dragOverProviderId === provider.id && 'border-primary/60 ring-2 ring-primary/20',
                    )}
                    onClick={() => setSelectedProviderId(provider.id)}
                  >
                    <div className="flex min-w-0 items-start gap-3">
                      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded bg-muted/60 text-sm font-semibold text-primary">
                        {displayName.slice(0, 1).toUpperCase()}
                      </div>
                      <div className="min-w-0">
                        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                          <span className="truncate text-sm font-medium">{displayName}</span>
                          {isDefault ? (
                            <Badge
                              variant="secondary"
                              className="h-5 gap-1 border-amber-200 bg-amber-50 px-1.5 text-[10px] text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
                            >
                              <Star className="h-3 w-3 fill-amber-400 text-amber-500" />
                              {t.aiConfig.defaultProvider}
                            </Badge>
                          ) : null}
                        </div>
                        <p className="mt-0.5 flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground">
                          <span className="truncate">{provider.model || t.common.none}</span>
                          {complete ? (
                            <CheckCircle2 className="h-3 w-3 shrink-0 text-green-500" aria-hidden="true" />
                          ) : (
                            <span aria-hidden="true" className="h-1.5 w-1.5 shrink-0 rounded-full bg-muted-foreground/40" />
                          )}
                        </p>
                      </div>
                    </div>
                  </button>
                  <button
                    type="button"
                    className={cn(
                      'absolute right-2 top-2 inline-flex h-7 w-7 items-center justify-center rounded-md transition',
                      isDefault
                        ? 'text-amber-400'
                        : complete
                          ? 'text-muted-foreground/60 hover:bg-muted/60 hover:text-amber-400'
                          : 'cursor-not-allowed text-muted-foreground/30',
                    )}
                    title={starTitle}
                    aria-label={starTitle}
                    disabled={!complete || isDefault}
                    onClick={(event) => {
                      event.stopPropagation()
                      if (provider.id !== config.defaultProviderId) {
                        void selectDefaultProvider(provider.id)
                      }
                    }}
                  >
                    <Star className={cn('h-4 w-4', isDefault && 'fill-amber-400 text-amber-500')} />
                  </button>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button
                        type="button"
                        aria-label={t.aiConfig.providerActions}
                        title={t.aiConfig.providerActions}
                        className="absolute right-2 top-9 inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground/60 transition hover:bg-muted/60 hover:text-foreground"
                        onClick={(event) => event.stopPropagation()}
                      >
                        <MoreVertical className="h-4 w-4" />
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-40">
                      <DropdownMenuItem
                        onSelect={() => updateProvider(provider.id, { enabled: !provider.enabled })}
                      >
                        <Power className="h-4 w-4" />
                        {provider.enabled ? t.aiConfig.disableProvider : t.aiConfig.enableProviderShort}
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => void testProvider(provider)}>
                        <RadioTower className="h-4 w-4" />
                        {t.aiConfig.testProvider}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        disabled={providers.length <= 1}
                        onSelect={() => removeProvider(provider.id)}
                      >
                        <Trash2 className="h-4 w-4" />
                        {t.aiConfig.deleteProvider}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              )
            })}
          </div>
        </section>

        <section className="min-w-0 space-y-4">
          {selectedProvider ? (
            <div className="rounded-md border bg-card p-4 shadow-sm">
              <div className="flex flex-wrap items-center justify-between gap-3 border-b pb-3 sm:flex-nowrap">
                <div className="flex min-w-0 items-center gap-2">
                  <Bot className="h-4 w-4 shrink-0 text-primary/70" />
                  <h3 className="truncate text-sm font-semibold">{t.aiConfig.providerDetail}</h3>
                </div>
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <div
                    className="flex items-center gap-1.5 rounded-md border border-border/70 px-2 py-1"
                    title={t.aiConfig.enableProviderHint}
                  >
                    <Switch
                      id="ai-provider-enabled"
                      checked={selectedProvider.enabled}
                      onCheckedChange={(enabled) => updateSelectedProvider({ enabled })}
                      aria-label={t.aiConfig.enableProvider}
                    />
                    <Label htmlFor="ai-provider-enabled" className="text-xs">
                      {t.aiConfig.enableProviderShort}
                    </Label>
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant={selectedIsDefault ? 'secondary' : 'outline'}
                    className={cn(
                      'h-8',
                      selectedIsDefault && 'border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300',
                    )}
                    title={!providerStatus(selectedProvider) ? t.aiConfig.fillProviderFirst : undefined}
                    onClick={() => {
                      if (!selectedIsDefault) {
                        void selectDefaultProvider(selectedProvider.id)
                      }
                    }}
                    disabled={saving || !providerStatus(selectedProvider)}
                  >
                    <Star
                      className={cn(
                        'mr-1.5 h-3.5 w-3.5',
                        selectedIsDefault && 'fill-amber-400 text-amber-500',
                      )}
                    />
                    {selectedIsDefault ? t.aiConfig.currentDefault : t.aiConfig.setAsDefault}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="h-8"
                    onClick={() => testProvider(selectedProvider)}
                    disabled={testingProviderId === selectedProvider.id}
                  >
                    {testingProviderId === selectedProvider.id ? (
                      <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                    ) : null}
                    {t.aiConfig.testProvider}
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="outline"
                    className="h-8 w-8"
                    onClick={removeSelectedProvider}
                    disabled={providers.length <= 1}
                    title={t.aiConfig.deleteProvider}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>

              <div className="mt-4 grid gap-4 md:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="ai-provider-name" className="text-xs">
                    {t.aiConfig.providerName}
                  </Label>
                  <Input
                    id="ai-provider-name"
                    className="h-9"
                    value={selectedProvider.name}
                    onChange={(event) => updateSelectedProvider({ name: event.target.value })}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs">{t.aiConfig.protocol}</Label>
                  <Select
                    value={selectedProvider.protocol}
                    onValueChange={(protocol) => {
                      updateSelectedProvider({
                        protocol: protocol as AIProviderProtocol,
                        supportsStreaming: protocol !== 'anthropic',
                      })
                    }}
                  >
                    <SelectTrigger className="h-9">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="openai-compatible">{t.aiConfig.openAICompatible}</SelectItem>
                      <SelectItem value="anthropic">{t.aiConfig.anthropic}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5 md:col-span-2">
                  <Label htmlFor="ai-provider-api" className="text-xs">
                    {t.apiKeys.baseUrl}
                  </Label>
                  <Input
                    id="ai-provider-api"
                    className="h-9 font-mono text-xs"
                    value={selectedProvider.api}
                    onChange={(event) => updateSelectedProvider({ api: event.target.value })}
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="ai-provider-key" className="text-xs">
                    {t.apiKeys.apiKey}
                  </Label>
                  <PasswordInput
                    id="ai-provider-key"
                    className="h-9"
                    value={selectedProvider.key}
                    onChange={(event) => updateSelectedProvider({ key: event.target.value })}
                  />
                </div>
                <AIModelSelect
                  key={selectedProvider.id}
                  provider={selectedProvider}
                  onChange={(model) => updateSelectedProvider({ model })}
                />
                <div className="space-y-1.5 md:col-span-2">
                  <Label htmlFor="ai-provider-request-rate" className="text-xs">
                    {t.aiConfig.requestRate}
                  </Label>
                  <div className="flex items-center gap-2">
                    <div className="min-w-0 flex-1 sm:w-[180px] sm:flex-none">
                      <Input
                        id="ai-provider-request-rate"
                        type="number"
                        min={0}
                        step={1}
                        className="h-9"
                        aria-describedby="ai-provider-request-rate-help"
                        value={rateValue}
                        onChange={(event) => {
                          const count = Math.max(0, Math.trunc(Number(event.target.value)))
                          if (Number.isSafeInteger(count * 60)) {
                            updateSelectedProvider({
                              workbenchRequestsPerMinute: count * (rateUnit === 'second' ? 60 : 1),
                              workbenchRequestRateUnit: rateUnit,
                            })
                          }
                        }}
                      />
                    </div>
                    <span className="shrink-0 text-sm text-muted-foreground">{t.aiConfig.requestsPer}</span>
                    <Select
                      value={rateUnit}
                      onValueChange={(unit: 'second' | 'minute') => updateSelectedProvider({
                        workbenchRequestRateUnit: unit,
                        workbenchRequestsPerMinute: rateValue * (unit === 'second' ? 60 : 1),
                      })}
                    >
                      <SelectTrigger id="ai-provider-request-rate-unit" className="h-9 w-[100px] shrink-0" aria-label={t.aiConfig.requestRateUnit}>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="second">{t.aiConfig.perSecond}</SelectItem>
                        <SelectItem value="minute">{t.aiConfig.perMinute}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <p id="ai-provider-request-rate-help" className="text-xs leading-relaxed text-muted-foreground">
                    {t.aiConfig.requestRateHelp}
                  </p>
                </div>
                <div className="space-y-1.5 md:col-span-2">
                  <Label className="text-xs">{t.aiConfig.tokenQuota}</Label>
                  <div className="grid gap-4 md:grid-cols-2">
                    <div className="space-y-1.5">
                      <Label htmlFor="ai-provider-max-total-tokens" className="text-xs text-muted-foreground">
                        {t.aiConfig.maxTotalTokens}
                      </Label>
                      <Input
                        id="ai-provider-max-total-tokens"
                        type="number"
                        min={0}
                        step={1000}
                        className="h-9"
                        placeholder="120000"
                        aria-describedby="ai-provider-max-total-tokens-help"
                        value={selectedProvider.maxTotalTokens || ''}
                        onChange={(event) => {
                          const raw = event.target.value.trim()
                          const parsed = raw === '' ? 0 : Math.max(0, Math.trunc(Number(raw)))
                          if (raw === '' || Number.isSafeInteger(parsed)) {
                            updateSelectedProvider({ maxTotalTokens: parsed })
                          }
                        }}
                      />
                      <p id="ai-provider-max-total-tokens-help" className="text-xs leading-relaxed text-muted-foreground">
                        {t.aiConfig.maxTotalTokensHelp}
                      </p>
                    </div>
                    <div className="space-y-1.5">
                      <Label htmlFor="ai-provider-max-output-tokens" className="text-xs text-muted-foreground">
                        {t.aiConfig.maxOutputTokens}
                      </Label>
                      <Input
                        id="ai-provider-max-output-tokens"
                        type="number"
                        min={0}
                        step={1024}
                        className="h-9"
                        placeholder="16384"
                        aria-describedby="ai-provider-max-output-tokens-help"
                        value={selectedProvider.maxOutputTokens || ''}
                        onChange={(event) => {
                          const raw = event.target.value.trim()
                          const parsed = raw === '' ? 0 : Math.max(0, Math.trunc(Number(raw)))
                          if (raw === '' || Number.isSafeInteger(parsed)) {
                            updateSelectedProvider({ maxOutputTokens: parsed })
                          }
                        }}
                      />
                      <p id="ai-provider-max-output-tokens-help" className="text-xs leading-relaxed text-muted-foreground">
                        {t.aiConfig.maxOutputTokensHelp}
                      </p>
                    </div>
                  </div>
                  <p className="text-xs leading-relaxed text-muted-foreground">{t.aiConfig.tokenQuotaHelp}</p>
                </div>
                <details className="rounded-md border p-3 md:col-span-2">
                  <summary className="flex cursor-pointer items-center gap-2 text-xs font-medium">
                    <span>{t.aiConfig.advancedSettings}</span>
                    {providerHeaders.length > 0 ? (
                      <Badge variant="outline" className="h-5 px-1.5 text-[10px] font-normal">
                        {t.aiConfig.headersCount.replace('{count}', String(providerHeaders.length))}
                      </Badge>
                    ) : null}
                  </summary>
                  <div className="mt-3 space-y-4">
                    <div className="space-y-1.5">
                      <Label htmlFor="ai-provider-request-timeout" className="text-xs">
                        {t.aiConfig.requestTimeout}
                      </Label>
                      <div className="flex items-center gap-2">
                        <div className="w-[140px]">
                          <Input
                            id="ai-provider-request-timeout"
                            type="number"
                            min={1}
                            max={120}
                            step={1}
                            className="h-9"
                            aria-describedby="ai-provider-request-timeout-help"
                            aria-invalid={!Number.isInteger(selectedProvider.requestTimeoutMinutes) || (selectedProvider.requestTimeoutMinutes ?? 30) < 1 || (selectedProvider.requestTimeoutMinutes ?? 30) > 120}
                            value={selectedProvider.requestTimeoutMinutes === 0 ? '' : selectedProvider.requestTimeoutMinutes ?? 30}
                            onChange={(event) => updateSelectedProvider({ requestTimeoutMinutes: Number(event.target.value) })}
                          />
                        </div>
                        <span className="text-xs text-muted-foreground">{t.aiConfig.perMinute}</span>
                      </div>
                      <p id="ai-provider-request-timeout-help" className="text-xs leading-relaxed text-muted-foreground">
                        {t.aiConfig.requestTimeoutHelp}
                      </p>
                    </div>

                    <div className="space-y-2 border-t pt-3">
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div>
                          <Label className="text-xs">{t.aiConfig.headersTitle}</Label>
                          <p className="mt-1 text-[11px] leading-relaxed text-muted-foreground">
                            {t.aiConfig.headersHelp}
                          </p>
                        </div>
                        <Button type="button" size="sm" variant="outline" className="h-8" onClick={addProviderHeader}>
                          <Plus className="mr-1.5 h-3.5 w-3.5" />
                          {t.aiConfig.addHeader}
                        </Button>
                      </div>

                      {providerHeaders.length === 0 ? (
                        <div className="rounded-md border border-dashed px-3 py-2 text-[11px] text-muted-foreground">
                          {t.aiConfig.headersEmpty}
                        </div>
                      ) : (
                        <div className="space-y-2">
                          {providerHeaders.map((header, index) => (
                            <div key={index} className="flex items-start gap-2">
                              <Input
                                aria-label={t.aiConfig.headersName}
                                className="h-9 min-w-0 flex-1 font-mono text-xs"
                                placeholder={t.aiConfig.headersNamePlaceholder}
                                value={header.name}
                                onChange={(event) => updateProviderHeader(index, { name: event.target.value })}
                              />
                              <Input
                                aria-label={t.aiConfig.headersValue}
                                className="h-9 min-w-0 flex-1 font-mono text-xs"
                                placeholder={t.aiConfig.headersValuePlaceholder}
                                value={header.value}
                                onChange={(event) => updateProviderHeader(index, { value: event.target.value })}
                              />
                              <Button
                                type="button"
                                size="icon"
                                variant="ghost"
                                className="h-9 w-9 shrink-0 text-muted-foreground"
                                title={t.aiConfig.removeHeader}
                                aria-label={t.aiConfig.removeHeader}
                                onClick={() => removeProviderHeader(index)}
                              >
                                <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                            </div>
                          ))}
                        </div>
                      )}

                      {isOpenCodeProvider(selectedProvider.api) ? (
                        <div className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-[11px] leading-relaxed text-amber-700 dark:text-amber-300">
                          <span aria-hidden="true" className="mt-0.5">◆</span>
                          <span>{t.aiConfig.headersOpenCodeHint}</span>
                        </div>
                      ) : null}
                    </div>
                  </div>
                </details>
              </div>

            </div>
          ) : null}

          <div className="rounded-md border bg-card p-4 shadow-sm">
            <div className="mb-4 flex items-center justify-between gap-3 border-b pb-3">
              <div className="flex min-w-0 items-center gap-2">
                <Settings2 className="h-4 w-4 shrink-0 text-primary/70" />
                <div className="min-w-0">
                  <h3 className="text-sm font-semibold">{t.aiConfig.failoverTitle}</h3>
                  <p className="text-xs text-muted-foreground">{t.aiConfig.failoverDescription}</p>
                </div>
              </div>
              <Switch
                id="ai-workbench-fallback-enabled"
                aria-label={t.aiConfig.workbenchFallbackEnabled}
                checked={fallbackEnabled}
                onCheckedChange={(enabled) =>
                  updateConfig({
                    routing: {
                      ...config.routing,
                      workbenchFallbackEnabled: enabled,
                    },
                  })
                }
              />
            </div>
            <div className="grid gap-1.5">
              <Label className="text-xs">{t.aiConfig.workbenchFallbackProvider}</Label>
              <Select
                value={fallbackProviderId}
                disabled={!fallbackEnabled}
                onValueChange={(value) =>
                  updateConfig({
                    routing: {
                      ...config.routing,
                      workbenchFallback: value === ROUTE_DEFAULT_VALUE ? '' : value,
                    },
                  })
                }
              >
                <SelectTrigger className="h-9">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ROUTE_DEFAULT_VALUE}>
                    {t.aiConfig.selectWorkbenchFallbackProvider}
                  </SelectItem>
                  {fallbackProviderOptions.map((option) => (
                    <SelectItem key={option.value} value={option.value} disabled={option.disabled}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-[11px] text-muted-foreground">
                {t.aiConfig.workbenchFallbackProviderHint}
              </p>
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
