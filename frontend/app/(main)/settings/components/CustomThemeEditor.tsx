'use client'

import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import {
  ArrowLeft,
  Loader2,
  Maximize2,
  Minus,
  Palette,
  Plus,
  RotateCcw,
  Sparkles,
} from 'lucide-react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { useI18n } from '@/lib/i18n'
import { createCustomTheme, generateCustomTheme, updateCustomTheme, ThemeApiError } from '@/lib/api/themes'
import {
  contrastForeground,
  createDefaultThemeDefinition,
  isHexColor,
  isValidRadius,
  normalizeThemeName,
  parseThemeDefinition,
  themeNameKey,
  type CustomThemeColors,
  type CustomThemeDefinition,
  type CustomThemeRecord,
  type ThemeBase,
} from '@/lib/theme/custom-theme'
import { cn } from '@/lib/utils'
import toast from '@/lib/toast'
import { ThemePreview } from './ThemePreview'
import { ThemeColorPicker } from './ThemeColorPicker'
import { ThemeTokenSection } from './ThemeTokenSection'
import {
  DESTRUCTIVE_TOKENS,
  INFO_TOKENS,
  PRIMARY_TOKENS,
  SIDEBAR_TOKENS,
  SUCCESS_TOKENS,
  WARNING_TOKENS,
} from './theme-token-groups'
import { ThemeTokenInspector } from './ThemeTokenInspector'

interface CustomThemeEditorProps {
  theme: CustomThemeRecord | null
  existingNames: string[]
  draftKey: string
  onSaved: (theme: CustomThemeRecord, apply: boolean) => void
  onBack: () => void
}

interface StoredDraft {
  version: 1
  name: string
  definition: CustomThemeDefinition
  updatedAt: string
}

const CORE_COLOR_KEYS: Array<keyof CustomThemeColors> = [
  'background',
  'foreground',
  'card',
  'primary',
  'primaryForeground',
  'ring',
]

const SURFACE_COLOR_KEYS: Array<keyof CustomThemeColors> = [
  'muted',
  'mutedForeground',
  'border',
]

const SIDEBAR_COLOR_KEYS: Array<keyof CustomThemeColors> = ['sidebar', 'sidebarForeground']

const STATUS_COLOR_KEYS: Array<keyof CustomThemeColors> = [
  'success',
  'warning',
  'info',
  'destructive',
  'destructiveForeground',
]

const PRESET_PRIMARY_COLORS = [
  '#3e63dd',
  '#0ea5e9',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#ec4899',
  '#64748b',
]

function radiusToRem(value: string): number {
  const normalized = value.trim().toLowerCase()
  const parsed = Number.parseFloat(normalized)
  if (!Number.isFinite(parsed)) return 0.5
  if (normalized.endsWith('px')) return parsed / 16
  return parsed
}

function readDraft(key: string): StoredDraft | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.sessionStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<StoredDraft>
    if (parsed.version !== 1 || typeof parsed.name !== 'string' || !parsed.definition) {
      return null
    }
    return {
      version: 1,
      name: parsed.name,
      definition: parseThemeDefinition(parsed.definition),
      updatedAt: typeof parsed.updatedAt === 'string' ? parsed.updatedAt : '',
    }
  } catch {
    return null
  }
}

function clearDraft(key: string) {
  if (typeof window === 'undefined') return
  window.sessionStorage.removeItem(key)
}

function uniqueThemeName(candidate: string, existingKeys: Set<string>): string {
  const root = normalizeThemeName(candidate) || 'AI 主题'
  let name = root
  let index = 2
  while (existingKeys.has(themeNameKey(name))) {
    name = `${root} ${index}`
    index += 1
  }
  return name
}

interface ColorFieldProps {
  label: string
  value: string
  invalidLabel: string
  onChange: (value: string) => void
}

function ColorField({ label, value, invalidLabel, onChange }: ColorFieldProps) {
  const valid = isHexColor(value)
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between gap-2 rounded-md border border-border/70 bg-muted/10 px-2 py-1.5">
        <Label className="text-xs text-muted-foreground">{label}</Label>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[10px] text-muted-foreground">{value}</span>
          <ThemeColorPicker
            triggerOnly
            value={value}
            onChange={onChange}
            ariaLabel={label}
            invalid={!valid}
          />
        </div>
      </div>
      {!valid && <p className="text-[11px] text-destructive">{invalidLabel}</p>}
    </div>
  )
}

export function CustomThemeEditor({
  theme,
  existingNames,
  draftKey,
  onSaved,
  onBack,
}: CustomThemeEditorProps) {
  const { t } = useI18n()
  const copy = t.theme.customThemes.editor
  const baseline = useMemo(
    () =>
      theme
        ? { name: theme.name, definition: structuredClone(theme.definition) }
        : { name: '', definition: createDefaultThemeDefinition('dark') },
    [theme],
  )

  const storedDraft = useMemo(() => {
    const draft = readDraft(draftKey)
    if (!draft) return null
    const unchanged =
      normalizeThemeName(draft.name) === normalizeThemeName(baseline.name) &&
      JSON.stringify(draft.definition) === JSON.stringify(baseline.definition)
    return unchanged ? null : draft
  }, [draftKey, baseline])

  const [name, setName] = useState(storedDraft?.name ?? baseline.name)
  const [definition, setDefinition] = useState<CustomThemeDefinition>(() =>
    storedDraft ? structuredClone(storedDraft.definition) : structuredClone(baseline.definition),
  )
  const [restoredDraft, setRestoredDraft] = useState(Boolean(storedDraft))
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState<'save' | 'apply' | null>(null)
  const [previewZoom, setPreviewZoom] = useState(0.6)
  const [aiOpen, setAiOpen] = useState(false)
  const [leaveOpen, setLeaveOpen] = useState(false)
  const [keepColorsOnBaseChange, setKeepColorsOnBaseChange] = useState(false)
  const [aiPrompt, setAiPrompt] = useState('')
  const [aiGenerating, setAiGenerating] = useState(false)
  const [aiError, setAiError] = useState<string | null>(null)
  const [aiSuccess, setAiSuccess] = useState<string | null>(null)
  const previewHostRef = useRef<HTMLDivElement | null>(null)

  const dirty =
    normalizeThemeName(name) !== normalizeThemeName(baseline.name) ||
    JSON.stringify(definition) !== JSON.stringify(baseline.definition)

  const existingKeys = useMemo(
    () => new Set(existingNames.map((item) => themeNameKey(item))),
    [existingNames],
  )

  useEffect(() => {
    if (typeof window === 'undefined') return
    if (!dirty) {
      window.sessionStorage.removeItem(draftKey)
      return
    }
    const payload: StoredDraft = {
      version: 1,
      name,
      definition,
      updatedAt: new Date().toISOString(),
    }
    window.sessionStorage.setItem(draftKey, JSON.stringify(payload))
  }, [dirty, name, definition, draftKey])

  useEffect(() => {
    if (!dirty) return
    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [dirty])

  const updateColor = (key: keyof CustomThemeColors, value: string) => {
    setDefinition((previous) => ({
      ...previous,
      colors: {
        ...previous.colors,
        [key]: value,
      },
    }))
  }

  const updateOverrides = (overrides: Record<string, string>) => {
    setDefinition((previous) => {
      const next = { ...previous }
      if (Object.keys(overrides).length > 0) {
        next.overrides = overrides
      } else {
        delete next.overrides
      }
      return next
    })
  }

  const applyPrimary = (color: string) => {
    setDefinition((previous) => ({
      ...previous,
      colors: {
        ...previous.colors,
        primary: color,
        primaryForeground: contrastForeground(color),
        ring: color,
      },
    }))
  }

  const changeBase = (base: ThemeBase) => {
    setDefinition((previous) => {
      if (previous.base === base) return previous
      if (keepColorsOnBaseChange) {
        return { ...previous, base }
      }
      const next = createDefaultThemeDefinition(base)
      next.radius = previous.radius
      return next
    })
  }

  const resetPalette = () => {
    setDefinition((previous) => createDefaultThemeDefinition(previous.base))
  }

  const discardDraft = () => {
    clearDraft(draftKey)
    setRestoredDraft(false)
    setName(baseline.name)
    setDefinition(structuredClone(baseline.definition))
  }

  const requestBack = () => {
    if (dirty) {
      setLeaveOpen(true)
      return
    }
    clearDraft(draftKey)
    onBack()
  }

  const confirmLeave = () => {
    clearDraft(draftKey)
    setLeaveOpen(false)
    onBack()
  }

  const generateWithAI = async () => {
    const prompt = aiPrompt.trim()
    if (prompt.length < 3) {
      setAiError(copy.ai.promptRequired)
      return
    }
    setAiGenerating(true)
    setAiError(null)
    setAiSuccess(null)
    try {
      const result = await generateCustomTheme(prompt, definition.base)
      setDefinition(result.definition)
      if (!normalizeThemeName(name)) {
        setName(uniqueThemeName(result.name || copy.ai.nameFallback, existingKeys))
      }
      setRestoredDraft(false)
      setAiSuccess(result.description ? `${copy.ai.success} ${result.description}` : copy.ai.success)
    } catch (error) {
      if (error instanceof ThemeApiError && error.code === 'ai_unavailable') {
        setAiError(copy.ai.unavailable)
      } else {
        setAiError(copy.ai.failed)
      }
    } finally {
      setAiGenerating(false)
    }
  }

  const changePreviewZoom = (delta: number) => {
    setPreviewZoom((previous) => {
      const next = Math.round((previous + delta) * 100) / 100
      return Math.min(1.2, Math.max(0.5, next))
    })
  }

  const togglePreviewFullscreen = async () => {
    const host = previewHostRef.current
    if (!host) return
    if (document.fullscreenElement) {
      await document.exitFullscreen()
      return
    }
    await host.requestFullscreen?.()
  }

  const validate = (): string | null => {
    const normalizedName = normalizeThemeName(name)
    if (!normalizedName) return copy.nameRequired
    if (normalizedName.length > 60) return copy.nameTooLong
    if (existingKeys.has(themeNameKey(normalizedName))) return copy.nameDuplicate

    for (const [key, value] of Object.entries(definition.colors) as Array<
      [keyof CustomThemeColors, string]
    >) {
      if (!isHexColor(value)) {
        return `${copy.invalidColor} (${copy.colorLabels[key]})`
      }
    }
    if (!isValidRadius(definition.radius)) {
      return copy.invalidRadius
    }
    for (const [token, value] of Object.entries(definition.overrides ?? {})) {
      if (token === '--radius' || !isHexColor(value)) {
        return `${copy.invalidColor} (${token})`
      }
    }
    return null
  }

  const save = async (apply: boolean) => {
    const validationError = validate()
    if (validationError) {
      setError(validationError)
      return
    }

    setError(null)
    setSaving(apply ? 'apply' : 'save')
    try {
      const normalizedName = normalizeThemeName(name)
      const saved = theme
        ? await updateCustomTheme(theme.id, normalizedName, definition)
        : await createCustomTheme(normalizedName, definition)
      clearDraft(draftKey)
      setRestoredDraft(false)
      toast.success(apply ? copy.savedAndApplied : copy.saved)
      onSaved(saved, apply)
    } catch (error) {
      if (error instanceof ThemeApiError) {
        if (error.code === 'duplicate_name') {
          setError(copy.nameDuplicate)
        } else if (error.code === 'invalid_name') {
          setError(copy.nameTooLong)
        } else if (error.code === 'invalid_definition') {
          setError(copy.invalidColor)
        } else {
          setError(copy.saveFailed)
        }
      } else {
        setError(copy.saveFailed)
      }
    } finally {
      setSaving(null)
    }
  }

  const renderColorField = (
    key: keyof CustomThemeColors,
    gradientTokens?: string[],
  ) => (
    <div key={key} className="space-y-2">
      <ColorField
        label={copy.colorLabels[key]}
        value={definition.colors[key]}
        invalidLabel={copy.invalidColor}
        onChange={(value) => updateColor(key, value)}
      />
      {gradientTokens && (
        <ThemeTokenSection
          definition={definition}
          onChange={updateOverrides}
          tokens={gradientTokens}
        />
      )}
    </div>
  )

  const renderColorFields = (keys: Array<keyof CustomThemeColors>) =>
    keys.map((key) => {
      const tokens =
        key === 'primary'
          ? PRIMARY_TOKENS
          : key === 'success'
            ? SUCCESS_TOKENS
            : key === 'warning'
              ? WARNING_TOKENS
              : key === 'info'
                ? INFO_TOKENS
                : key === 'destructive'
                  ? DESTRUCTIVE_TOKENS
                  : key === 'sidebar'
                    ? SIDEBAR_TOKENS
                    : undefined
      return renderColorField(key, tokens)
    })

  const radiusRem = radiusToRem(definition.radius)
  const radiusSliderValue = Math.min(1.5, Math.max(0, radiusRem))

  return (
    <>
      <div className="flex h-full min-h-0 flex-col bg-background">
        <header className="flex flex-wrap items-start gap-3 border-b border-border bg-background px-4 py-3 md:px-6">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="-ml-2 shrink-0"
            onClick={requestBack}
          >
            <ArrowLeft className="h-4 w-4" />
            {copy.back}
          </Button>
          <div className="min-w-0 flex-1">
            <h1 className="flex items-center gap-2 text-base font-semibold">
              <Palette className="h-4 w-4 text-primary" />
              <span className="truncate">{theme ? copy.editTitle : copy.createTitle}</span>
            </h1>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void save(false)}
              disabled={!!saving}
            >
              {saving === 'save' ? copy.saving : copy.save}
            </Button>
            <Button type="button" size="sm" onClick={() => void save(true)} disabled={!!saving}>
              {saving === 'apply' ? copy.saving : copy.saveAndApply}
            </Button>
          </div>
        </header>

        <div className="border-b border-border bg-background px-4 py-3 md:px-6">
          <div className="flex flex-wrap items-center gap-3">
            <Button
              type="button"
              variant={aiOpen ? 'secondary' : 'outline'}
              size="sm"
              onClick={() => setAiOpen((open) => !open)}
            >
              <Sparkles className="h-3.5 w-3.5" />
              {copy.ai.title}
            </Button>
            <div className="flex min-w-[220px] flex-1 items-center gap-2 sm:max-w-[420px]">
              <Label htmlFor="custom-theme-name" className="shrink-0 text-xs text-muted-foreground">
                {copy.name}
              </Label>
              <Input
                id="custom-theme-name"
                value={name}
                maxLength={60}
                placeholder={copy.namePlaceholder}
                onChange={(event) => setName(event.target.value)}
                className="h-8"
              />
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-muted-foreground">{copy.base}</span>
              <div className="flex rounded-md border border-border bg-muted/30 p-0.5">
                {(['dark', 'light'] as ThemeBase[]).map((base) => (
                  <button
                    key={base}
                    type="button"
                    onClick={() => changeBase(base)}
                    className={cn(
                      'rounded px-2.5 py-1 text-xs transition-colors',
                      definition.base === base
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {base === 'dark' ? copy.dark : copy.light}
                  </button>
                ))}
              </div>
            </div>
            <label className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <input
                type="checkbox"
                checked={keepColorsOnBaseChange}
                onChange={(event) => setKeepColorsOnBaseChange(event.target.checked)}
                className="h-3.5 w-3.5 accent-[var(--primary)]"
              />
              <span>{copy.keepColors}</span>
            </label>
          </div>
          {aiOpen && (
            <div className="mt-3 rounded-lg border border-primary/25 bg-primary/5 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-xs text-muted-foreground">{copy.ai.description}</p>
                <span className="text-[10px] text-muted-foreground">{copy.ai.baseHint}</span>
              </div>
              <Textarea
                value={aiPrompt}
                maxLength={2000}
                placeholder={copy.ai.placeholder}
                onChange={(event) => setAiPrompt(event.target.value)}
                className="mt-2 min-h-[64px] resize-y bg-background text-xs"
              />
              <div className="mt-1.5 flex flex-wrap gap-1">
                {copy.ai.suggestions.map((suggestion) => (
                  <button
                    key={suggestion}
                    type="button"
                    onClick={() => setAiPrompt(suggestion)}
                    className="rounded-full border border-border/70 bg-background px-2 py-0.5 text-[10px] text-muted-foreground transition-colors hover:border-primary/50 hover:text-primary"
                  >
                    {suggestion}
                  </button>
                ))}
              </div>
              <div className="mt-2 flex items-center justify-end gap-2">
                {aiError && <p className="mr-auto text-[11px] text-destructive">{aiError}</p>}
                {aiSuccess && <p className="mr-auto text-[11px] text-muted-foreground">{aiSuccess}</p>}
                <Button
                  type="button"
                  size="sm"
                  onClick={() => void generateWithAI()}
                  disabled={aiGenerating || aiPrompt.trim().length < 3}
                >
                  {aiGenerating ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Sparkles className="h-3.5 w-3.5" />
                  )}
                  {aiGenerating ? copy.ai.generating : copy.ai.generate}
                </Button>
              </div>
            </div>
          )}
        </div>

        {restoredDraft && dirty && (
          <div className="flex flex-wrap items-center justify-between gap-2 border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-xs text-amber-700 md:px-6 dark:text-amber-300">
            <span>{copy.draftRestored}</span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-7 px-2"
              onClick={discardDraft}
            >
              {copy.discardDraft}
            </Button>
          </div>
        )}

        <div className="grid min-h-0 flex-1 grid-cols-1 lg:grid-cols-[440px_minmax(0,1fr)]">
          <div className="min-h-0 overflow-y-auto border-r border-border px-4 py-5 md:px-6">
            <div className="space-y-5">
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <Label>{copy.colors}</Label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2"
                    onClick={resetPalette}
                  >
                    <RotateCcw className="mr-1 h-3.5 w-3.5" />
                    {copy.resetPalette}
                  </Button>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs text-muted-foreground">{copy.presetColors}</Label>
                  <div className="flex flex-wrap gap-2">
                    {PRESET_PRIMARY_COLORS.map((color) => (
                      <button
                        key={color}
                        type="button"
                        aria-label={color}
                        title={color}
                        onClick={() => applyPrimary(color)}
                        className={cn(
                          'h-7 w-7 rounded-full border-2 transition-transform hover:scale-110',
                          definition.colors.primary.toLowerCase() === color
                            ? 'border-foreground'
                            : 'border-transparent',
                        )}
                        style={{ backgroundColor: color }}
                      />
                    ))}
                  </div>
                </div>
                <div className="space-y-3">{renderColorFields(CORE_COLOR_KEYS)}</div>
              </div>

              <div className="space-y-3">
                <Label>{copy.moreColors}</Label>
                <div className="space-y-3">
                  {renderColorFields(SURFACE_COLOR_KEYS)}
                  {renderColorFields(SIDEBAR_COLOR_KEYS)}
                  {renderColorFields(STATUS_COLOR_KEYS)}
                  <ThemeTokenInspector definition={definition} onChange={updateOverrides} />
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <Label htmlFor="custom-theme-radius">{copy.radius}</Label>
                  <span className="font-mono text-xs text-muted-foreground">
                    {definition.radius}
                  </span>
                </div>
                <input
                  type="range"
                  min={0}
                  max={1.5}
                  step={0.125}
                  value={radiusSliderValue}
                  aria-label={copy.radius}
                  onChange={(event) =>
                    setDefinition((previous) => ({
                      ...previous,
                      radius: `${event.target.value}rem`,
                    }))
                  }
                  className="w-full accent-[var(--primary)]"
                />
                <div className="flex justify-between text-[10px] text-muted-foreground">
                  <span>0</span>
                  <span>0.75rem</span>
                  <span>1.5rem</span>
                </div>
                <Input
                  id="custom-theme-radius"
                  value={definition.radius}
                  spellCheck={false}
                  placeholder={copy.radiusPlaceholder}
                  onChange={(event) =>
                    setDefinition((previous) => ({ ...previous, radius: event.target.value }))
                  }
                />
              </div>

              {error && (
                <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {error}
                </div>
              )}

              <div className="space-y-2 lg:hidden">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-xs font-medium text-foreground">{copy.preview}</span>
                  <Badge variant="outline">
                    {definition.base === 'dark' ? copy.dark : copy.light}
                  </Badge>
                </div>
                <ThemePreview definition={definition} />
              </div>
            </div>
          </div>

          <div
            ref={previewHostRef}
            className="hidden min-h-0 flex-col overflow-y-auto bg-muted/30 p-4 md:p-5 lg:flex"
          >
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border/70 bg-background/80 p-1.5">
              <div className="px-2 text-xs font-medium text-foreground">{copy.preview}</div>
              <div className="flex items-center gap-1">
                <Badge variant="outline" className="mr-1">
                  {definition.base === 'dark' ? copy.dark : copy.light}
                </Badge>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={copy.zoomOut}
                  onClick={() => changePreviewZoom(-0.05)}
                >
                  <Minus className="h-3.5 w-3.5" />
                </Button>
                <span className="w-8 text-center font-mono text-[10px] text-muted-foreground">
                  {Math.round(previewZoom * 100)}%
                </span>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={copy.zoomIn}
                  onClick={() => changePreviewZoom(0.05)}
                >
                  <Plus className="h-3.5 w-3.5" />
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={copy.zoomReset}
                  onClick={() => setPreviewZoom(0.6)}
                >
                  <RotateCcw className="h-3.5 w-3.5" />
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={copy.previewModes.fullscreen}
                  onClick={() => void togglePreviewFullscreen()}
                >
                  <Maximize2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
            <div className="min-h-0 overflow-auto rounded-xl border border-border/70 bg-background/40 p-3">
              <div style={{ zoom: previewZoom } as CSSProperties}>
                <ThemePreview definition={definition} />
              </div>
            </div>
          </div>
        </div>
      </div>

      <AlertDialog open={leaveOpen} onOpenChange={setLeaveOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{copy.leaveTitle}</AlertDialogTitle>
            <AlertDialogDescription>{copy.leaveDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{copy.keepEditing}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={confirmLeave}
            >
              {copy.discardAndLeave}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
