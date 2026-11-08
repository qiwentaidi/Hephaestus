'use client'

import { useEffect, useState } from 'react'
import { ArrowLeft, Plus, Save, Trash2 } from 'lucide-react'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'
import type { DatabaseCollectRule } from '../types'
import { BUILTIN_ROW_MATCH_RULES, DEFAULT_KEYWORDS } from '../utils/match'
import { loadCustomCollectRules, saveCustomCollectRules } from '../utils/rules'

interface SettingsDialogProps {
  onBack: () => void
  onSave?: (payload: { keywords: string[]; customRuleCount: number }) => void
}

const createEmptyRule = (): DatabaseCollectRule => ({
  label: '',
  pattern: '',
})

export function SettingsDialog({ onBack, onSave }: SettingsDialogProps) {
  const { t } = useI18n()
  const [keywords, setKeywords] = useState<string>(DEFAULT_KEYWORDS.join(','))
  const [customRules, setCustomRules] = useState<DatabaseCollectRule[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let cancelled = false

    const loadSettings = async () => {
      setLoading(true)

      const savedKeywords =
        typeof window !== 'undefined' ? localStorage.getItem('database_keywords') : null
      if (savedKeywords) {
        try {
          const keywordsArray = JSON.parse(savedKeywords)
          setKeywords(keywordsArray.join(','))
        } catch {
          setKeywords(DEFAULT_KEYWORDS.join(','))
        }
      } else {
        setKeywords(DEFAULT_KEYWORDS.join(','))
      }

      try {
        const rules = await loadCustomCollectRules()
        if (!cancelled) {
          setCustomRules(rules)
        }
      } catch (error) {
        if (!cancelled) {
          setCustomRules([])
          toast.error({
            title: t.tools.database.toasts.loadFailedTitle,
            message:
              error instanceof Error ? error.message : t.tools.database.toasts.networkError,
          })
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    loadSettings()
    return () => {
      cancelled = true
    }
  }, [t.tools.database.toasts.loadFailedTitle, t.tools.database.toasts.networkError])

  const handleSave = async () => {
    const keywordsArray = keywords
      .split(',')
      .map((k) => k.trim())
      .filter((k) => k.length > 0)

    const normalizedRules = customRules
      .map((rule) => ({
        label: rule.label.trim(),
        pattern: rule.pattern.trim(),
      }))
      .filter((rule) => rule.label.length > 0 || rule.pattern.length > 0)

    const hasIncompleteRule = normalizedRules.some(
      (rule) => rule.label.length === 0 || rule.pattern.length === 0
    )
    if (hasIncompleteRule) {
      toast.error({
        title: t.tools.database.toasts.errorTitle,
        message: t.tools.database.settingsDialog.customRulesIncomplete,
      })
      return
    }

    setSaving(true)
    try {
      localStorage.setItem('database_keywords', JSON.stringify(keywordsArray))
      await saveCustomCollectRules(normalizedRules)
      onSave?.({ keywords: keywordsArray, customRuleCount: normalizedRules.length })
    } catch (error) {
      toast.error({
        title: t.tools.database.toasts.errorTitle,
        message:
          error instanceof Error ? error.message : t.tools.database.toasts.networkError,
      })
    } finally {
      setSaving(false)
    }
  }

  const handleReset = () => {
    setKeywords(DEFAULT_KEYWORDS.join(','))
    setCustomRules([])
  }

  const updateRule = (index: number, field: 'label' | 'pattern', value: string) => {
    setCustomRules((prev) =>
      prev.map((rule, ruleIndex) =>
        ruleIndex === index ? { ...rule, [field]: value } : rule
      )
    )
  }

  const removeRule = (index: number) => {
    setCustomRules((prev) => prev.filter((_, ruleIndex) => ruleIndex !== index))
  }

  const addRule = () => {
    setCustomRules((prev) => [...prev, createEmptyRule()])
  }

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="border-b border-border">
        <div className="flex h-[56px] items-center justify-between gap-4 px-6 py-2">
          <div className="flex items-center gap-3">
            <Button variant="ghost" onClick={onBack} className="gap-2">
              <ArrowLeft className="h-4 w-4" />
              {t.common.back}
            </Button>
            <div className="h-6 w-px bg-border" />
            <span className="text-sm font-medium">{t.tools.database.settingsDialog.title}</span>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={handleReset} disabled={loading || saving}>
              {t.tools.database.settingsDialog.reset}
            </Button>
            <Button onClick={handleSave} disabled={loading || saving}>
              <Save className="mr-2 h-4 w-4" />
              {t.common.save}
            </Button>
          </div>
        </div>
      </div>

      <div className="mx-auto w-full max-w-7xl px-6 py-6">
        <Tabs defaultValue="keywords" className="w-full">
          <TabsList className="h-10 gap-1 rounded-[8px] border border-border/70 bg-background/35 p-1">
            <TabsTrigger
              value="keywords"
              className="rounded-[6px] px-4 text-sm text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground"
            >
              {t.tools.database.settingsDialog.keywordsLabel}
            </TabsTrigger>
            <TabsTrigger
              value="regex"
              className="rounded-[6px] px-4 text-sm text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground"
            >
              {t.tools.database.settingsDialog.regexSectionTitle}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="keywords" className="mt-5">
            <section className="rounded-2xl border border-border/70 bg-card/60 p-6 shadow-sm">
              <div className="space-y-1">
                <h2 className="text-lg font-semibold">
                  {t.tools.database.settingsDialog.keywordsLabel}
                </h2>
              </div>

              <div className="mt-5 space-y-3">
                <Label htmlFor="keywords">{t.tools.database.settingsDialog.keywordsLabel}</Label>
                <Textarea
                  id="keywords"
                  value={keywords}
                  onChange={(e) => setKeywords(e.target.value)}
                  placeholder={t.tools.database.settingsDialog.keywordsPlaceholder}
                  rows={14}
                  className="min-h-[320px] resize-y"
                  disabled={loading || saving}
                />
              </div>
            </section>
          </TabsContent>

          <TabsContent value="regex" className="mt-5">
            <section className="rounded-2xl border border-border/70 bg-card/60 p-6 shadow-sm">
              <div className="space-y-1">
                <h2 className="text-lg font-semibold">
                  {t.tools.database.settingsDialog.regexSectionTitle}
                </h2>
              </div>

              <div className="mt-5 space-y-5">
                <div className="rounded-xl border border-border/60 bg-muted/30 p-4">
                  <div className="space-y-1">
                    <p className="text-sm font-semibold">
                      {t.tools.database.settingsDialog.builtinRulesLabel}
                    </p>
                    <p className="text-sm text-muted-foreground">
                      {t.tools.database.settingsDialog.builtinRulesHint}
                    </p>
                  </div>
                  <div className="mt-4 space-y-3">
                    {BUILTIN_ROW_MATCH_RULES.map((rule) => (
                      <div
                        key={`${rule.label}-${rule.pattern}`}
                        className="grid grid-cols-1 gap-3 lg:grid-cols-[220px_minmax(0,1fr)]"
                      >
                        <Input value={rule.label} readOnly />
                        <Input value={rule.pattern} readOnly className="font-mono text-xs" />
                      </div>
                    ))}
                  </div>
                </div>

                <div className="rounded-xl border border-border/60 p-4">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="space-y-1">
                      <p className="text-sm font-semibold">
                        {t.tools.database.settingsDialog.customRulesLabel}
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      type="button"
                      onClick={addRule}
                      disabled={loading || saving}
                    >
                      <Plus className="mr-2 h-4 w-4" />
                      {t.tools.database.settingsDialog.addRule}
                    </Button>
                  </div>

                  <div className="mt-4 space-y-3">
                    {customRules.length === 0 ? (
                      <div className="rounded-xl border border-dashed px-4 py-8 text-sm text-muted-foreground">
                        {t.tools.database.settingsDialog.emptyCustomRules}
                      </div>
                    ) : null}
                    {customRules.map((rule, index) => (
                      <div
                        key={rule.id || `rule-${index}`}
                        className="grid grid-cols-1 gap-3 lg:grid-cols-[220px_minmax(0,1fr)_auto]"
                      >
                        <Input
                          value={rule.label}
                          onChange={(e) => updateRule(index, 'label', e.target.value)}
                          placeholder={t.tools.database.settingsDialog.ruleLabelPlaceholder}
                          disabled={loading || saving}
                        />
                        <Input
                          value={rule.pattern}
                          onChange={(e) => updateRule(index, 'pattern', e.target.value)}
                          placeholder={t.tools.database.settingsDialog.rulePatternPlaceholder}
                          className="font-mono text-xs"
                          disabled={loading || saving}
                        />
                        <Button
                          variant="outline"
                          type="button"
                          onClick={() => removeRule(index)}
                          disabled={loading || saving}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </section>
          </TabsContent>
        </Tabs>
      </div>
    </div>
  )
}
