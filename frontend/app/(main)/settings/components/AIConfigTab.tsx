'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  Bot,
  CheckCircle2,
  Info,
  Loader2,
  MoreVertical,
  Pencil,
  Plus,
  Power,
  RadioTower,
  Save,
  Settings2,
  Star,
  Trash2,
  X,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
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
import { MultiSelectFilter } from '@/components/ui/multi-select-filter'
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetTitle } from '@/components/ui/sheet'
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
import { AIWebSearchCard } from './AIWebSearchCard'

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
  const workbenchProviderId = rawRouting.workbench || defaultProviderId || ''
  const configuredFallbacks =
    Array.isArray(rawRouting.workbenchFallbacks) && rawRouting.workbenchFallbacks.length > 0
      ? rawRouting.workbenchFallbacks
      : rawRouting.workbenchFallback
        ? [rawRouting.workbenchFallback]
        : []
  const workbenchFallbacks: string[] = []
  for (const candidate of configuredFallbacks) {
    const id = typeof candidate === 'string' ? candidate.trim() : ''
    if (!id || id === workbenchProviderId || workbenchFallbacks.includes(id)) continue
    if (!normalizedProviders.some((provider) => provider.id === id)) continue
    workbenchFallbacks.push(id)
  }
  const workbenchFallback = workbenchFallbacks[0] || ''

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
      workbenchFallbacks,
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

// 默认渠道必须始终指向可用渠道：关闭当前默认渠道时，自动切到下一个「已启用
// 且配置完整」的渠道。否则 defaultProviderId 会停在已关闭的渠道上，后端解析
// 请求时把 enabled && provider.enabled 求值为 false，整个 AI 会被判定为不可用。
function ensureAvailableDefaultProvider(config: AIConfig): AIConfig {
  const providers = config.providers || []
  const current = providers.find((provider) => provider.id === config.defaultProviderId)
  if (!current || current.enabled) return config
  const next = providers.find((provider) => provider.id !== current.id && providerStatus(provider))
  if (!next) return config
  return syncTopLevelConfig({ ...config, defaultProviderId: next.id })
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
  const [detailOpen, setDetailOpen] = useState(false)
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

  const updateSelectedProvider = (next: Partial<AIProviderConfig>) => {
    if (!selectedProvider) return
    setConfig((current) => {
      const updatedProviders = (current.providers || []).map((provider) =>
        provider.id === selectedProvider.id ? { ...provider, ...next } : provider,
      )
      return syncTopLevelConfig({ ...current, providers: updatedProviders })
    })
  }

  // 更多操作菜单里的启用/关闭直接落盘，与排序、设默认、删除保持一致；
  // 详情面板里的字段仍由“保存配置”统一提交。
  const toggleProviderEnabled = (providerId: string) => {
    const updatedProviders = (config.providers || []).map((provider) =>
      provider.id === providerId ? { ...provider, enabled: !provider.enabled } : provider,
    )
    autoSaveConfig(
      ensureAvailableDefaultProvider(syncTopLevelConfig({ ...config, providers: updatedProviders })),
    )
  }

  const reorderProviders = (fromId: string, toId: string) => {
    if (!fromId || !toId || fromId === toId) return
    const items = [...(config.providers || [])]
    const fromIndex = items.findIndex((provider) => provider.id === fromId)
    const toIndex = items.findIndex((provider) => provider.id === toId)
    if (fromIndex < 0 || toIndex < 0) return
    const [moved] = items.splice(fromIndex, 1)
    items.splice(toIndex, 0, moved)
    autoSaveConfig(syncTopLevelConfig({ ...config, providers: items }))
  }

  const moveProvider = (providerId: string, offset: number) => {
    const items = [...(config.providers || [])]
    const index = items.findIndex((provider) => provider.id === providerId)
    const target = index + offset
    if (index < 0 || target < 0 || target >= items.length) return
    const [moved] = items.splice(index, 1)
    items.splice(target, 0, moved)
    autoSaveConfig(syncTopLevelConfig({ ...config, providers: items }))
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
    const existingProviders = config.providers || []
    const nextProvider = createEmptyProvider(new Set(existingProviders.map((provider) => provider.id)))
    const next = syncTopLevelConfig({
      ...config,
      providers: [...existingProviders, nextProvider],
      defaultProviderId: config.defaultProviderId || existingProviders[0]?.id || nextProvider.id,
    })
    setSelectedProviderId(nextProvider.id)
    setDetailOpen(true)
    autoSaveConfig(next)
  }

  const removeProvider = (providerId: string) => {
    if (providers.length <= 1) return
    const provider = providers.find((item) => item.id === providerId)
    if (!provider) return

    const remaining = (config.providers || []).filter((item) => item.id !== providerId)
    const defaultProviderId =
      config.defaultProviderId === providerId ? remaining[0]?.id : config.defaultProviderId
    const routing = { ...config.routing }
    const remainingFallbacks = (routing.workbenchFallbacks || []).filter((id) => id !== providerId)
    routing.workbenchFallbacks = remainingFallbacks
    routing.workbenchFallback = remainingFallbacks[0] || ''
    if (remainingFallbacks.length === 0) {
      routing.workbenchFallbackEnabled = false
    }
    if (selectedProviderId === providerId) {
      setSelectedProviderId(defaultProviderId || remaining[0]?.id || DEFAULT_PROVIDER.id)
    }
    autoSaveConfig(syncTopLevelConfig({ ...config, providers: remaining, defaultProviderId, routing }))
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

  const persistConfig = async (
    nextConfig: AIConfig,
    options: { notifyDefaultSwitch?: boolean } = {},
  ) => {
    const prepared = ensureAvailableDefaultProvider(prepareHeaders(nextConfig))
    validateTimeouts(prepared)
    validateHeaders(prepared)
    const normalized = withProviderNameFallbacks(prepared)
    await saveAIConfig(normalized)
    if (options.notifyDefaultSwitch && normalized.defaultProviderId !== config.defaultProviderId) {
      const previousDefault = normalized.providers?.find(
        (provider) => provider.id === config.defaultProviderId,
      )
      const nextDefault = normalized.providers?.find(
        (provider) => provider.id === normalized.defaultProviderId,
      )
      if (previousDefault && !previousDefault.enabled && nextDefault) {
        toast.success(t.aiConfig.defaultProviderAutoSwitched.replace('{name}', nextDefault.name))
      }
    }
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
      await persistConfig(config, { notifyDefaultSwitch: true })
      toast.success(t.common.configSaved)
    } catch (error: any) {
      toast.error(`${t.common.saveFailed}: ${error.message || ''}`)
    } finally {
      setSaving(false)
    }
  }

  // 渠道列表与主备路由没有单独的保存按钮：改动后直接落盘。
  const autoSaveConfig = (next: AIConfig) => {
    setConfig(next)
    setSaving(true)
    void (async () => {
      try {
        await persistConfig(next, { notifyDefaultSwitch: true })
      } catch (error: any) {
        toast.error(`${t.common.saveFailed}: ${error.message || ''}`)
      } finally {
        setSaving(false)
      }
    })()
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

  const fallbackEnabled = Boolean(config.routing?.workbenchFallbackEnabled)
  const fallbackProviderIds = config.routing?.workbenchFallbacks || []
  const fallbackPrimaryId = config.routing?.workbench || config.defaultProviderId || ''
  const fallbackOptions = providers
    .filter(
      (provider) =>
        provider.id !== fallbackPrimaryId &&
        providerStatus(provider) &&
        provider.protocol === 'openai-compatible',
    )
    .map((provider, index) => ({
      value: provider.id,
      label: provider.name.trim() || `${t.aiConfig.unnamedProvider} ${index + 1}`,
    }))

  return (
    <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-300">
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
                    onClick={() => {
                      setSelectedProviderId(provider.id)
                      setDetailOpen(true)
                    }}
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
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <button
                        type="button"
                        aria-label={t.aiConfig.providerActions}
                        title={t.aiConfig.providerActions}
                        className="absolute right-2 top-2 inline-flex h-7 w-7 items-center justify-center rounded-md text-muted-foreground/60 transition hover:bg-muted/60 hover:text-foreground"
                        onClick={(event) => event.stopPropagation()}
                      >
                        <MoreVertical className="h-4 w-4" />
                      </button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-40">
                      <DropdownMenuItem
                        onSelect={() => {
                          setSelectedProviderId(provider.id)
                          setDetailOpen(true)
                        }}
                      >
                        <Pencil className="h-4 w-4" />
                        {t.common.edit}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        onSelect={() => {
                          if (!complete || isDefault) return
                          void selectDefaultProvider(provider.id)
                        }}
                        disabled={!complete || isDefault}
                      >
                        <Star className={cn('h-4 w-4', isDefault && 'fill-amber-400 text-amber-500')} />
                        {starTitle}
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem onSelect={() => toggleProviderEnabled(provider.id)}>
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
          <AIWebSearchCard />

          <div className="rounded-md border bg-card p-4 shadow-sm">
            <div className="mb-3 flex items-center justify-between gap-3 border-b pb-3">
              <div className="flex min-w-0 items-center gap-2">
                <Settings2 className="h-4 w-4 shrink-0 text-primary/70" />
                <h3 className="truncate text-sm font-semibold">{t.aiConfig.failoverTitle}</h3>
              </div>
              <Switch
                id="ai-workbench-fallback-enabled"
                aria-label={t.aiConfig.workbenchFallbackEnabled}
                checked={fallbackEnabled}
                onCheckedChange={(enabled) =>
                  autoSaveConfig(
                    syncTopLevelConfig({
                      ...config,
                      routing: {
                        ...config.routing,
                        workbenchFallbackEnabled: enabled,
                      },
                    }),
                  )
                }
              />
            </div>
            <Alert className="mb-4 border-border/70 bg-muted/30 py-2.5">
              <Info className="h-4 w-4 text-primary/70" />
              <AlertDescription className="text-xs leading-relaxed">
                <p>{t.aiConfig.failoverDescription}</p>
                <p>{t.aiConfig.workbenchFallbackProviderHint}</p>
              </AlertDescription>
            </Alert>
            <div className="grid gap-1.5">
              <Label className="text-xs">{t.aiConfig.workbenchFallbackProvider}</Label>
              <MultiSelectFilter
                className="w-full"
                options={fallbackOptions}
                values={fallbackProviderIds}
                allLabel={t.aiConfig.workbenchFallbackClear}
                placeholderLabel={t.aiConfig.selectWorkbenchFallbackProvider}
                countLabel={t.common.selectedCount}
                removeLabel={t.common.remove}
                clearLabel={t.common.clear}
                ariaLabel={t.aiConfig.workbenchFallbackProvider}
                disabled={!fallbackEnabled}
                contentClassName="w-72"
                onChange={(next) => {
                  const ids = next.filter((id) => fallbackOptions.some((option) => option.value === id))
                  autoSaveConfig(
                    syncTopLevelConfig({
                      ...config,
                      routing: {
                        ...config.routing,
                        workbenchFallbacks: ids,
                        workbenchFallback: ids[0] || '',
                      },
                    }),
                  )
                }}
              />
            </div>
          </div>
        </section>
      </div>

      <Sheet open={detailOpen} onOpenChange={setDetailOpen}>
        <SheetContent side="right" showCloseButton={false} className="flex w-full flex-col gap-0 p-0 sm:max-w-xl">
          {selectedProvider ? (
            <>
              <div className="flex items-center justify-between gap-3 border-b px-5 py-3.5">
                <div className="min-w-0">
                  <SheetTitle className="flex items-center gap-2 text-sm font-semibold">
                    <Bot className="h-4 w-4 shrink-0 text-primary/70" />
                    {t.aiConfig.providerDetail}
                  </SheetTitle>
                  <SheetDescription className="mt-0.5 truncate text-xs text-muted-foreground">
                    {selectedProvider.name.trim() || t.aiConfig.unnamedProvider}
                  </SheetDescription>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <div
                    className="flex items-center justify-center"
                    title={t.aiConfig.enableProvider}
                  >
                    <Switch
                      id="ai-provider-enabled"
                      checked={selectedProvider.enabled}
                      onCheckedChange={(enabled) => updateSelectedProvider({ enabled })}
                      aria-label={t.aiConfig.enableProvider}
                    />
                  </div>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className={cn(
                      'h-8 w-8',
                      selectedIsDefault && 'bg-amber-500/10 text-amber-700 hover:bg-amber-500/15 dark:text-amber-300',
                    )}
                    title={selectedIsDefault ? t.aiConfig.currentDefault : t.aiConfig.setAsDefault}
                    aria-label={selectedIsDefault ? t.aiConfig.currentDefault : t.aiConfig.setAsDefault}
                    onClick={() => {
                      if (!selectedIsDefault) {
                        void selectDefaultProvider(selectedProvider.id)
                      }
                    }}
                    disabled={saving || !providerStatus(selectedProvider)}
                  >
                    <Star
                      className={cn(
                        'h-3.5 w-3.5',
                        selectedIsDefault && 'fill-amber-400 text-amber-500',
                      )}
                    />
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    title={t.aiConfig.testProvider}
                    aria-label={t.aiConfig.testProvider}
                    onClick={() => testProvider(selectedProvider)}
                    disabled={testingProviderId === selectedProvider.id}
                  >
                    {testingProviderId === selectedProvider.id ? (
                      <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                      <RadioTower className="h-3.5 w-3.5" />
                    )}
                  </Button>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    className="h-8 w-8"
                    onClick={() => {
                      removeSelectedProvider()
                      setDetailOpen(false)
                    }}
                    disabled={providers.length <= 1}
                    title={t.aiConfig.deleteProvider}
                    aria-label={t.aiConfig.deleteProvider}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                  <div className="mx-1 h-5 w-px shrink-0 bg-border" aria-hidden="true" />
                  <SheetClose asChild>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="h-8 w-8 text-muted-foreground"
                      aria-label={t.common.close}
                      title={t.common.close}
                    >
                      <X className="h-4 w-4" />
                    </Button>
                  </SheetClose>
                </div>
              </div>

              <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
                <div className="grid gap-4 md:grid-cols-2">
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
                  <div className="space-y-4 border-t pt-4 md:col-span-2">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-xs font-medium">{t.aiConfig.advancedSettings}</span>
                      {providerHeaders.length > 0 ? (
                        <Badge variant="outline" className="h-5 px-1.5 text-[10px] font-normal">
                          {t.aiConfig.headersCount.replace('{count}', String(providerHeaders.length))}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="space-y-4">
                      <div className="space-y-2">
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
                    <div className="space-y-1.5">
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
                    <div className="space-y-1.5">
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
                            value={selectedProvider.maxTotalTokens || ''}
                            onChange={(event) => {
                              const raw = event.target.value.trim()
                              const parsed = raw === '' ? 0 : Math.max(0, Math.trunc(Number(raw)))
                              if (raw === '' || Number.isSafeInteger(parsed)) {
                                updateSelectedProvider({ maxTotalTokens: parsed })
                              }
                            }}
                          />
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
                            value={selectedProvider.maxOutputTokens || ''}
                            onChange={(event) => {
                              const raw = event.target.value.trim()
                              const parsed = raw === '' ? 0 : Math.max(0, Math.trunc(Number(raw)))
                              if (raw === '' || Number.isSafeInteger(parsed)) {
                                updateSelectedProvider({ maxOutputTokens: parsed })
                              }
                            }}
                          />
                        </div>
                      </div>
                    </div>
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


                    
                  
                    </div>
                  </div>

                </div>
              </div>

              <div className="flex flex-wrap justify-end gap-2 border-t px-5 py-4">
                <Button variant="outline" onClick={() => setDetailOpen(false)}>
                  {t.common.close}
                </Button>
                <Button onClick={saveConfig} disabled={saving}>
                  {saving ? (
                    <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Save className="mr-1.5 h-3.5 w-3.5" />
                  )}
                  {t.common.saveConfig}
                </Button>
              </div>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
    </div>
  )
}
