'use client'

import { useEffect, useMemo, useState } from 'react'
import {
  Bot,
  BrainCircuit,
  CheckCircle2,
  Loader2,
  Plus,
  RadioTower,
  Save,
  Settings2,
  Star,
  Trash2,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  supportsTools: true,
  supportsStreaming: true,
}

const PROVIDER_PRESETS: Array<{
  label: string
  protocol: AIProviderProtocol
  api: string
  model: string
}> = [
  {
    label: 'OpenAI Compatible',
    protocol: 'openai-compatible',
    api: 'https://api.openai.com/v1',
    model: 'gpt-4o-mini',
  },
  {
    label: 'DashScope Compatible',
    protocol: 'openai-compatible',
    api: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    model: 'qwen-plus',
  },
  {
    label: 'DeepSeek Compatible',
    protocol: 'openai-compatible',
    api: 'https://api.deepseek.com/v1',
    model: 'deepseek-chat',
  },
  {
    label: 'Claude (Anthropic)',
    protocol: 'anthropic',
    api: 'https://api.anthropic.com/v1',
    model: 'claude-3-5-sonnet-latest',
  },
]

const ROUTE_DEFAULT_VALUE = '__default__'
const ROUTE_KEYS = ['workbench', 'jsscan', 'repeaterPolish', 'subdomainAssist'] as const
type AIRouteKey = (typeof ROUTE_KEYS)[number]

function effectiveWorkbenchProviderId(config: AIConfig) {
  return config.routing?.workbench || config.defaultProviderId || ''
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
          },
        ]

  const normalizedProviders = providers.map((provider, index) => ({
    ...DEFAULT_PROVIDER,
    ...provider,
    requestTimeoutMinutes: provider.requestTimeoutMinutes ?? 30,
    workbenchRequestsPerMinute: Math.max(0, provider.workbenchRequestsPerMinute || 0),
    workbenchRequestRateUnit: requestRateUnit(provider),
    id: provider.id || `provider-${index + 1}`,
    name: provider.name ?? `AI Provider ${index + 1}`,
    protocol: provider.protocol || 'openai-compatible',
    api:
      provider.api ||
      (provider.protocol === 'anthropic'
        ? 'https://api.anthropic.com/v1'
        : DEFAULT_PROVIDER.api),
    model:
      provider.model ||
      (provider.protocol === 'anthropic' ? 'claude-3-5-sonnet-latest' : DEFAULT_PROVIDER.model),
  }))

  const defaultProviderId =
    config?.defaultProviderId && normalizedProviders.some((provider) => provider.id === config.defaultProviderId)
      ? config.defaultProviderId
      : normalizedProviders[0]?.id

  const defaultProvider = normalizedProviders.find((provider) => provider.id === defaultProviderId) || normalizedProviders[0]
  const rawRouting = config?.routing || {}
  const workbenchProviderId = rawRouting.workbench || defaultProviderId || ''
  const workbenchFallback =
    rawRouting.workbenchFallback &&
    rawRouting.workbenchFallback !== workbenchProviderId &&
    normalizedProviders.some((provider) => provider.id === rawRouting.workbenchFallback)
      ? rawRouting.workbenchFallback
      : ''

  return {
    enabled: config?.enabled ?? false,
    requestTimeoutMinutes: defaultProvider?.requestTimeoutMinutes ?? 30,
    protocol: defaultProvider?.protocol || 'openai-compatible',
    api: defaultProvider?.api || DEFAULT_PROVIDER.api,
    key: defaultProvider?.key || '',
    model: defaultProvider?.model || DEFAULT_PROVIDER.model,
    defaultProviderId,
    providers: normalizedProviders,
    routing: {
      ...rawRouting,
      workbenchFallback,
      workbenchFallbackEnabled: Boolean(rawRouting.workbenchFallbackEnabled),
    },
    workbenchRemoteDispatch: config?.workbenchRemoteDispatch ?? false,
  }
}

function buildProviderFromPreset(index: number): AIProviderConfig {
  const preset = PROVIDER_PRESETS[index % PROVIDER_PRESETS.length]
  const suffix = Date.now().toString(36)
  return {
    requestTimeoutMinutes: 30,
    workbenchRequestsPerMinute: 0,
    workbenchRequestRateUnit: 'second',
    id: `provider-${suffix}`,
    name: preset.label,
    protocol: preset.protocol,
    enabled: true,
    api: preset.api,
    key: '',
    model: preset.model,
    supportsTools: true,
    supportsStreaming: preset.protocol !== 'anthropic',
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

export function AIConfigTab() {
  const { t } = useI18n()
  const [config, setConfig] = useState<AIConfig>(() => normalizeConfig())
  const [selectedProviderId, setSelectedProviderId] = useState(DEFAULT_PROVIDER.id)
  const [saving, setSaving] = useState(false)
  const [testingProviderId, setTestingProviderId] = useState<string | null>(null)

  const providers = useMemo(() => config.providers || [], [config.providers])
  const selectedProvider = providers.find((provider) => provider.id === selectedProviderId) || providers[0]
  const rateUnit = selectedProvider ? requestRateUnit(selectedProvider) : 'second'
  const rateValue = (selectedProvider?.workbenchRequestsPerMinute || 0) / (rateUnit === 'second' ? 60 : 1)
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

  const providerOptions = useMemo(
    () =>
      providers.map((provider, index) => ({
        value: provider.id,
        label: provider.name.trim() || `${t.aiConfig.unnamedProvider} ${index + 1}`,
        disabled: !providerStatus(provider),
      })),
    [providers, t.aiConfig.unnamedProvider],
  )

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

  const addProvider = () => {
    setConfig((current) => {
      const nextProvider = buildProviderFromPreset(current.providers?.length || 0)
      const next = syncTopLevelConfig({
        ...current,
        providers: [...(current.providers || []), nextProvider],
        defaultProviderId: current.defaultProviderId || nextProvider.id,
      })
      setSelectedProviderId(nextProvider.id)
      return next
    })
  }

  const removeSelectedProvider = () => {
    if (!selectedProvider || providers.length <= 1) return

    setConfig((current) => {
      const remaining = (current.providers || []).filter((provider) => provider.id !== selectedProvider.id)
      const defaultProviderId =
        current.defaultProviderId === selectedProvider.id ? remaining[0]?.id : current.defaultProviderId
      const routing = { ...current.routing }
      for (const key of ROUTE_KEYS) {
        if (routing[key] === selectedProvider.id) {
          routing[key] = ''
        }
      }
      if (routing.workbenchFallback === selectedProvider.id) {
        routing.workbenchFallback = ''
        routing.workbenchFallbackEnabled = false
      }
      setSelectedProviderId(defaultProviderId || remaining[0]?.id || DEFAULT_PROVIDER.id)
      return syncTopLevelConfig({ ...current, providers: remaining, defaultProviderId, routing })
    })
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

  const persistConfig = async (nextConfig: AIConfig) => {
    validateTimeouts(nextConfig)
    const normalized = withProviderNameFallbacks(nextConfig)
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
    setConfig(nextConfig)
    setSelectedProviderId(providerId)
    setSaving(true)
    try {
      validateTimeouts(nextConfig)
      await saveAIConfig(nextConfig)
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

  const updateWorkbenchRoute = (value: string) => {
    const providerId = value === ROUTE_DEFAULT_VALUE ? '' : value
    const nextWorkbenchProviderId = providerId || config.defaultProviderId || ''
    const nextRouting = {
      ...config.routing,
      workbench: providerId,
    }
    if (nextRouting.workbenchFallback === nextWorkbenchProviderId) {
      nextRouting.workbenchFallback = ''
      nextRouting.workbenchFallbackEnabled = false
    }
    updateConfig({ routing: nextRouting })
  }

  const routeSelect = (key: AIRouteKey, label: string, description: string) => (
    <div className="grid gap-1.5">
      <Label className="text-xs">{label}</Label>
      <Select
        value={config.routing?.[key] || ROUTE_DEFAULT_VALUE}
        onValueChange={(value) =>
          key === 'workbench'
            ? updateWorkbenchRoute(value)
            : updateConfig({
                routing: {
                  ...config.routing,
                  [key]: value === ROUTE_DEFAULT_VALUE ? '' : value,
                },
              })
        }
      >
        <SelectTrigger className="h-9">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ROUTE_DEFAULT_VALUE}>{t.aiConfig.useDefaultProvider}</SelectItem>
          {providerOptions.map((option) => (
            <SelectItem key={option.value} value={option.value} disabled={option.disabled}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-[11px] text-muted-foreground">{description}</p>
    </div>
  )

  const workbenchProviderId = effectiveWorkbenchProviderId(config)
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

      <section className="grid gap-4 rounded-md border bg-card p-4 shadow-sm lg:grid-cols-[1fr_260px]">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded bg-primary/10 text-primary">
            <BrainCircuit className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-semibold">{t.aiConfig.globalTitle}</h3>
              <Badge variant={config.enabled ? 'default' : 'outline'} className="h-5">
                {config.enabled ? t.common.enabled : t.common.disabled}
              </Badge>
              <Badge variant="secondary" className="h-5">
                {enabledProviders.length}/{providers.length} {t.aiConfig.available}
              </Badge>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">{t.aiConfig.globalDescription}</p>
          </div>
        </div>
        <div className="flex items-center justify-between gap-3 lg:justify-end">
          <Label htmlFor="ai-global-enabled" className="text-xs text-muted-foreground">
            {t.aiConfig.enableGlobal}
          </Label>
          <Switch
            id="ai-global-enabled"
            checked={config.enabled}
            onCheckedChange={(enabled) => updateConfig({ enabled })}
          />
        </div>
      </section>

      <div className="grid gap-6 xl:grid-cols-[360px_minmax(0,1fr)]">
        <section className="min-w-0 space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <RadioTower className="h-4 w-4 text-primary/70" />
              <h3 className="text-sm font-semibold">{t.aiConfig.providers}</h3>
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
              const displayName = provider.name.trim() || `${t.aiConfig.unnamedProvider} ${index + 1}`
              return (
                <button
                  key={provider.id}
                  type="button"
                  className={cn(
                    'w-full rounded-md border bg-card p-3 text-left shadow-sm transition hover:border-primary/30 hover:shadow-md',
                    active && 'border-primary/60 ring-1 ring-primary/20',
                  )}
                  onClick={() => setSelectedProviderId(provider.id)}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-start gap-3">
                      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded bg-muted/60 text-sm font-semibold text-primary">
                        {displayName.slice(0, 1).toUpperCase()}
                      </div>
                      <div className="min-w-0">
                        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                          <span className="truncate text-sm font-medium">{displayName}</span>
                          {provider.id === config.defaultProviderId ? (
                            <Badge
                              variant="secondary"
                              className="h-5 gap-1 border-amber-200 bg-amber-50 px-1.5 text-[10px] text-amber-700 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
                            >
                              <Star className="h-3 w-3 fill-amber-400 text-amber-500" />
                              {t.aiConfig.defaultProvider}
                            </Badge>
                          ) : null}
                        </div>
                        <p className="mt-0.5 truncate text-[11px] text-muted-foreground">
                          {provider.protocol} · {provider.model || t.common.none}
                        </p>
                      </div>
                    </div>
                    {complete ? (
                      <CheckCircle2 className="mt-1 h-4 w-4 shrink-0 text-green-500" />
                    ) : (
                      <span className="mt-1 h-2 w-2 shrink-0 rounded-full bg-muted-foreground/40" />
                    )}
                  </div>
                </button>
              )
            })}
          </div>
        </section>

        <section className="min-w-0 space-y-4">
          {selectedProvider ? (
            <div className="rounded-md border bg-card p-4 shadow-sm">
              <div className="flex flex-wrap items-start justify-between gap-3 border-b pb-3">
                <div className="flex min-w-0 items-center gap-2">
                  <Bot className="h-4 w-4 text-primary/70" />
                  <div>
                    <h3 className="text-sm font-semibold">{t.aiConfig.providerDetail}</h3>
                    <p className="text-xs text-muted-foreground">{t.aiConfig.providerDetailDescription}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
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
                  <summary className="cursor-pointer text-xs font-medium">{t.aiConfig.advancedSettings}</summary>
                  <div className="mt-3 space-y-1.5">
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
                </details>
              </div>

              <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2">
                <div className="flex items-center gap-2">
                  <Switch
                    id="ai-provider-enabled"
                    checked={selectedProvider.enabled}
                    onCheckedChange={(enabled) => updateSelectedProvider({ enabled })}
                  />
                  <Label htmlFor="ai-provider-enabled" className="text-xs">
                    {t.aiConfig.enableProvider}
                  </Label>
                </div>
                <Button
                  type="button"
                  size="sm"
                  variant={selectedProvider.id === config.defaultProviderId ? 'secondary' : 'outline'}
                  className="h-8"
                  onClick={() => selectDefaultProvider(selectedProvider.id)}
                  disabled={saving || !providerStatus(selectedProvider)}
                >
                  <Star
                    className={cn(
                      'mr-1.5 h-3.5 w-3.5',
                      selectedProvider.id === config.defaultProviderId && 'fill-amber-400 text-amber-500',
                    )}
                  />
                  {selectedProvider.id === config.defaultProviderId
                    ? t.aiConfig.currentDefault
                    : t.aiConfig.setAsDefault}
                </Button>
              </div>
            </div>
          ) : null}

          <div className="rounded-md border bg-card p-4 shadow-sm">
            <div className="mb-4 flex items-center gap-2 border-b pb-3">
              <Settings2 className="h-4 w-4 text-primary/70" />
              <div>
                <h3 className="text-sm font-semibold">{t.aiConfig.routingTitle}</h3>
                <p className="text-xs text-muted-foreground">{t.aiConfig.routingDescription}</p>
              </div>
            </div>
            <div className="grid gap-4 md:grid-cols-2">
              <div className="grid gap-1.5">
                <Label className="text-xs">{t.aiConfig.defaultProvider}</Label>
                <Select value={config.defaultProviderId} onValueChange={selectDefaultProvider}>
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {providerOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value} disabled={option.disabled}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-[11px] text-muted-foreground">{t.aiConfig.defaultProviderHint}</p>
              </div>
              {routeSelect('workbench', t.aiConfig.routeWorkbench, t.aiConfig.routeWorkbenchHint)}
              <div className="grid gap-3 border-t pt-4 md:col-span-2">
                <div className="flex items-center justify-between gap-3">
                  <div className="min-w-0">
                    <Label htmlFor="ai-workbench-fallback-enabled" className="text-xs">
                      {t.aiConfig.workbenchFallbackEnabled}
                    </Label>
                    <p className="mt-1 text-[11px] text-muted-foreground">
                      {t.aiConfig.workbenchFallbackEnabledHint}
                    </p>
                  </div>
                  <Switch
                    id="ai-workbench-fallback-enabled"
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
              {routeSelect('jsscan', t.aiConfig.routeJSScan, t.aiConfig.routeJSScanHint)}
              {routeSelect('repeaterPolish', t.aiConfig.routeRepeater, t.aiConfig.routeRepeaterHint)}
              {routeSelect('subdomainAssist', t.aiConfig.routeSubdomain, t.aiConfig.routeSubdomainHint)}
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
