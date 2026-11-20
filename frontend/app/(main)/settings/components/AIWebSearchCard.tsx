'use client'

import { useEffect, useState } from 'react'
import { Info, Loader2, Search } from 'lucide-react'

import { Alert, AlertDescription } from '@/components/ui/alert'
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
  getAIWebSearchSettings,
  saveAIWebSearchSettings,
  testAIWebSearch,
  type AIWebSearchBackend,
  type AIWebSearchSettings,
} from '@/lib/api/ai-web-search'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'

const BACKENDS: AIWebSearchBackend[] = ['ddgs', 'brave-free', 'tavily', 'deepseek']

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

export function AIWebSearchCard() {
  const { t } = useI18n()
  const [settings, setSettings] = useState<AIWebSearchSettings>({
    enabled: false,
    backend: 'ddgs',
    proxy: '',
    braveKeySet: false,
    tavilyKeySet: false,
  })
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [braveKey, setBraveKey] = useState('')
  const [tavilyKey, setTavilyKey] = useState('')

  useEffect(() => {
    let cancelled = false
    void getAIWebSearchSettings()
      .then((next) => {
        if (!cancelled) setSettings(next)
      })
      .catch((error) => {
        if (!cancelled) toast.error(`${t.aiConfig.webSearchLoadFailed}: ${errorMessage(error)}`)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [t.aiConfig.webSearchLoadFailed])

  const backendLabel = (backend: AIWebSearchBackend) => {
    switch (backend) {
      case 'brave-free':
        return t.aiConfig.webSearchBackendBrave
      case 'tavily':
        return t.aiConfig.webSearchBackendTavily
      case 'deepseek':
        return t.aiConfig.webSearchBackendDeepSeek
      default:
        return t.aiConfig.webSearchBackendDDGS
    }
  }

  const update = (next: Partial<AIWebSearchSettings>) => {
    setSettings((current) => ({ ...current, ...next }))
  }

  // 开关、来源、代理与 Key 都直接落盘，不再提供单独的保存按钮。
  const persistSettings = async (next: AIWebSearchSettings) => {
    // Brave/Tavily 必须先有 Key，否则后端会直接拒绝；先留在本地状态，等 Key 填好再落盘。
    const missingKey =
      (next.backend === 'brave-free' && !next.braveKeySet && !braveKey.trim()) ||
      (next.backend === 'tavily' && !next.tavilyKeySet && !tavilyKey.trim())
    if (missingKey) return

    setSaving(true)
    try {
      const saved = await saveAIWebSearchSettings({
        enabled: next.enabled,
        backend: next.backend,
        proxy: next.proxy,
        braveApiKey: braveKey.trim() || undefined,
        tavilyApiKey: tavilyKey.trim() || undefined,
      })
      setSettings(saved)
      setBraveKey('')
      setTavilyKey('')
    } catch (error) {
      toast.error(`${t.aiConfig.webSearchSaveFailed}: ${errorMessage(error)}`)
    } finally {
      setSaving(false)
    }
  }

  const updateAndSave = (next: Partial<AIWebSearchSettings>) => {
    const merged = { ...settings, ...next }
    setSettings(merged)
    void persistSettings(merged)
  }

  const test = async () => {
    setTesting(true)
    try {
      const results = await testAIWebSearch({
        backend: settings.backend,
        proxy: settings.proxy,
        braveApiKey: braveKey.trim() || undefined,
        tavilyApiKey: tavilyKey.trim() || undefined,
      })
      toast.success(t.aiConfig.webSearchTestSuccess.replace('{count}', String(results.length)))
    } catch (error) {
      toast.error(`${t.aiConfig.webSearchTestFailed}: ${errorMessage(error)}`)
    } finally {
      setTesting(false)
    }
  }

  return (
    <div className="rounded-md border bg-card p-4 shadow-sm">
      <div className="mb-3 flex items-center justify-between gap-3 border-b pb-3">
        <div className="flex min-w-0 items-center gap-2">
          <Search className="h-4 w-4 shrink-0 text-primary/70" />
          <h3 className="truncate text-sm font-semibold">{t.aiConfig.webSearchTitle}</h3>
        </div>
        <Switch
          aria-label={t.aiConfig.webSearchTitle}
          checked={settings.enabled}
          disabled={loading || saving}
          onCheckedChange={(enabled) => updateAndSave({ enabled })}
        />
      </div>

      <Alert className="mb-4 border-border/70 bg-muted/30 py-2.5">
        <Info className="h-4 w-4 text-primary/70" />
        <AlertDescription className="text-xs leading-relaxed">
          {t.aiConfig.webSearchDescription}
        </AlertDescription>
      </Alert>

      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label className="text-xs">{t.aiConfig.webSearchSource}</Label>
          <Select
            value={settings.backend}
            disabled={loading || saving}
            onValueChange={(value: AIWebSearchBackend) => updateAndSave({ backend: value })}
          >
            <SelectTrigger className="h-9">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {BACKENDS.map((backend) => (
                <SelectItem key={backend} value={backend}>
                  {backendLabel(backend)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {settings.backend === 'brave-free' ? (
          <div className="grid gap-1.5">
            <Label htmlFor="ai-web-search-brave-key" className="text-xs">
              {t.aiConfig.webSearchBraveKey}
            </Label>
            <PasswordInput
              id="ai-web-search-brave-key"
              className="h-9"
              value={braveKey}
              disabled={loading || saving}
              placeholder={settings.braveKeySet ? t.aiConfig.webSearchKeyConfigured : t.aiConfig.webSearchKeyRequired}
              onChange={(event) => setBraveKey(event.target.value)}
              onBlur={() => void persistSettings(settings)}
            />
          </div>
        ) : null}

        {settings.backend === 'tavily' ? (
          <div className="grid gap-1.5">
            <Label htmlFor="ai-web-search-tavily-key" className="text-xs">
              {t.aiConfig.webSearchTavilyKey}
            </Label>
            <PasswordInput
              id="ai-web-search-tavily-key"
              className="h-9"
              value={tavilyKey}
              disabled={loading || saving}
              placeholder={settings.tavilyKeySet ? t.aiConfig.webSearchKeyConfigured : t.aiConfig.webSearchKeyRequired}
              onChange={(event) => setTavilyKey(event.target.value)}
              onBlur={() => void persistSettings(settings)}
            />
          </div>
        ) : null}

        <div className="grid gap-1.5">
          <Label htmlFor="ai-web-search-proxy" className="text-xs">
            {t.aiConfig.webSearchProxy}
          </Label>
          <Input
            id="ai-web-search-proxy"
            className="h-9 font-mono text-xs"
            value={settings.proxy}
            disabled={loading || saving}
            placeholder={t.aiConfig.webSearchProxyPlaceholder}
            onChange={(event) => update({ proxy: event.target.value })}
            onBlur={() => void persistSettings(settings)}
          />
          <p className="text-[11px] leading-relaxed text-muted-foreground">{t.aiConfig.webSearchProxyHint}</p>
        </div>

        <p className="text-[11px] leading-relaxed text-muted-foreground">
          {settings.enabled ? t.aiConfig.webSearchTotalEnabled : t.aiConfig.webSearchTotalDisabled}
        </p>

        <p className="text-[11px] leading-relaxed text-muted-foreground">
          {t.aiConfig.webSearchAutoSaveHint}
        </p>

        <div className="flex flex-wrap justify-end gap-2">
          <Button type="button" size="sm" variant="outline" className="h-8" onClick={() => void test()} disabled={loading || saving || testing}>
            {testing ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}
            {testing ? t.aiConfig.webSearchTesting : t.aiConfig.webSearchTest}
          </Button>
        </div>
      </div>
    </div>
  )
}
