'use client'

import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { listAIModels, type AIModelOption, type AIProviderConfig } from '@/lib/api/ai-config'
import { useI18n } from '@/lib/i18n'

export function AIModelSelect({ provider, onChange }: {
  provider: AIProviderConfig
  onChange: (model: string) => void
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [revision, setRevision] = useState(0)
  const [result, setResult] = useState<{
    api: string; key: string; protocol: string; revision: number
    models: AIModelOption[]; error?: string
  } | null>(null)
  const api = provider.api.trim()
  const key = provider.key.trim()
  const protocol = provider.protocol
  const ready = Boolean(api && key)
  const current = result?.api === api && result?.key === key &&
    result?.protocol === protocol && result?.revision === revision ? result : null
  const loading = ready && !current
  const models = current?.models || []

  useEffect(() => {
    if (!ready) return
    const controller = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const models = await listAIModels({ api, key, protocol }, controller.signal)
        if (!controller.signal.aborted) setResult({ api, key, protocol, revision, models })
      } catch (error) {
        if (!controller.signal.aborted) {
          setResult({ api, key, protocol, revision, models: [], error: error instanceof Error ? error.message : 'request_failed' })
        }
      }
    }, 500)
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [api, key, protocol, ready, revision])

  const errors: Record<string, string> = {
    invalid_config: t.aiConfig.modelsInvalidConfig,
    authentication_failed: t.aiConfig.modelsAuthFailed,
    rate_limited: t.aiConfig.modelsRateLimited,
    unsupported: t.aiConfig.modelsUnsupported,
    invalid_response: t.aiConfig.modelsInvalidResponse,
    timeout: t.aiConfig.modelsTimeout,
    request_failed: t.aiConfig.modelsRequestFailed,
  }
  const status = !ready ? t.aiConfig.modelsConfigureFirst
    : loading ? t.aiConfig.modelsLoading
    : current?.error ? errors[current.error] || t.aiConfig.modelsRequestFailed
    : models.length ? t.aiConfig.modelsLoaded.replace('{count}', String(models.length))
    : t.aiConfig.modelsEmpty
  const query = search.trim()
  const filtered = models.filter(model => `${model.id} ${model.name || ''}`.toLowerCase().includes(query.toLowerCase()))
  const custom = query && !models.some(model => model.id === query)
  const select = (value: string) => {
    onChange(value)
    setOpen(false)
  }

  return (
    <div className="min-w-0 space-y-1.5">
      <div className="flex h-4 items-center justify-between gap-2">
        <Label htmlFor="ai-provider-model" className="text-xs">{t.apiKeys.openai.model}</Label>
        <button type="button" className="text-xs text-muted-foreground underline-offset-4 hover:underline disabled:opacity-50"
          disabled={!ready || loading} onClick={() => setRevision(value => value + 1)}>
          {t.aiConfig.modelsRefresh}
        </button>
      </div>
      <Popover open={open} onOpenChange={next => { setOpen(next); if (next) setSearch('') }}>
        <PopoverTrigger asChild>
          <Button id="ai-provider-model" type="button" variant="outline" role="combobox"
            aria-expanded={open} aria-describedby="ai-model-status"
            className="h-9 w-full justify-between font-normal hover:border-border hover:bg-muted hover:text-foreground data-[state=open]:bg-muted data-[state=open]:text-foreground dark:hover:border-border dark:hover:bg-muted dark:hover:text-foreground dark:data-[state=open]:bg-muted dark:data-[state=open]:text-foreground">
            <span className="truncate">{provider.model || t.aiConfig.modelsPlaceholder}</span>
            <span aria-hidden="true" className="ml-2 text-muted-foreground">▾</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-[var(--radix-popover-trigger-width)] p-0">
          <Command shouldFilter={false}>
            <CommandInput value={search} onValueChange={setSearch} placeholder={t.aiConfig.modelsSearch} aria-label={t.aiConfig.modelsSearch} />
            <CommandList aria-label={t.apiKeys.openai.model}>
              <CommandEmpty>{loading ? t.aiConfig.modelsLoading : t.aiConfig.modelsNoMatch}</CommandEmpty>
              {custom ? (
                <CommandItem value={`custom:${query}`} onSelect={() => select(query)} className="data-[selected=true]:bg-muted data-[selected=true]:text-foreground">
                  <span className="break-all">{t.aiConfig.modelsUseCustom.replace('{model}', query)}</span>
                </CommandItem>
              ) : null}
              {!query && provider.model && !models.some(model => model.id === provider.model) ? (
                <CommandItem value={provider.model} onSelect={() => select(provider.model)} className="data-[selected=true]:bg-muted data-[selected=true]:text-foreground">
                  <span aria-hidden="true">✓</span><span className="break-all">{provider.model}</span>
                </CommandItem>
              ) : null}
              {filtered.map(model => (
                <CommandItem key={model.id} value={model.id} onSelect={() => select(model.id)} className="data-[selected=true]:bg-muted data-[selected=true]:text-foreground">
                  <span aria-hidden="true" className="w-3 shrink-0">{model.id === provider.model ? '✓' : ''}</span>
                  <span className="min-w-0 break-all">{model.id}{model.name && model.name !== model.id ? <span className="block text-xs text-muted-foreground">{model.name}</span> : null}</span>
                </CommandItem>
              ))}
            </CommandList>
          </Command>
          <p className="border-t px-3 py-2 text-[11px] text-muted-foreground">{t.aiConfig.modelsManualHint}</p>
        </PopoverContent>
      </Popover>
      <p id="ai-model-status" role="status" className="text-[11px] text-muted-foreground">{status}</p>
    </div>
  )
}
