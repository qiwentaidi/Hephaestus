'use client'

import { useEffect, useMemo, useState } from 'react'
import { Loader2, Pencil, Plus, Save, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { SelectTag, type SelectTagOption } from '@/components/ui/select-tag'
import { Switch } from '@/components/ui/switch'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import {
  getToolGuardConfig,
  saveToolGuardConfig,
  type ToolGuardConfig,
  type ToolGuardRule,
} from '@/lib/api/tool-guard'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'

const emptyConfig: ToolGuardConfig = { enabled: true, rules: [] }
const commonTools = ['exec', 'exec_run', 'python_execute', 'execute-python-script', 'http-framework-test']
type RuleTarget = NonNullable<ToolGuardRule['target']>

function normalizeConfig(value: ToolGuardConfig): ToolGuardConfig {
  return {
    enabled: true,
    rules: Array.isArray(value?.rules)
      ? value.rules.map((rule) => ({
          ...rule,
          name: rule.name.replace(/^\s*\[内置\]\s*/, ''),
          tools: Array.isArray(rule.tools) ? rule.tools : [],
          target: rule.target || 'tool_input',
          priority: Number.isFinite(rule.priority) ? rule.priority : 0,
        }))
      : [],
  }
}

export default function ToolGuardPage() {
  const { t } = useI18n()
  const copy = t.toolGuard
  const [config, setConfig] = useState<ToolGuardConfig>(emptyConfig)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [draft, setDraft] = useState<ToolGuardRule | null>(null)

  useEffect(() => {
    let active = true
    getToolGuardConfig()
      .then((value) => {
        if (active) setConfig(normalizeConfig(value))
      })
      .catch((error) => {
        if (active) toast.error(copy.loadFailed + ': ' + (error instanceof Error ? error.message : String(error)))
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => { active = false }
  }, [copy.loadFailed])

  const toolOptions = useMemo<SelectTagOption[]>(() => {
    const names = new Set([...commonTools, ...config.rules.flatMap((rule) => rule.tools || [])])
    return [...names].map((name) => ({ value: name, label: name }))
  }, [config.rules])

  const sortedRules = useMemo(
    () => config.rules.map((rule, index) => ({ rule, index }))
      .sort((a, b) => (b.rule.priority || 0) - (a.rule.priority || 0) || a.index - b.index),
    [config.rules],
  )

  const targetLabel = (target?: RuleTarget) => {
    if (target === 'executable_input') return copy.targetExecutable
    if (target === 'http_method') return copy.targetHttpMethod
    return copy.targetAllInput
  }

  const updateRule = (index: number, updates: Partial<ToolGuardRule>) => {
    setConfig((current) => ({
      ...current,
      rules: current.rules.map((rule, itemIndex) => itemIndex === index ? { ...rule, ...updates } : rule),
    }))
  }

  const openNew = () => {
    setEditingIndex(-1)
    setDraft({
      name: copy.newRuleName,
      enabled: true,
      priority: 0,
      target: 'tool_input',
      tools: [],
      pattern: '',
      message: copy.defaultMessage,
    })
  }

  const openEdit = (index: number) => {
    setEditingIndex(index)
    setDraft({ ...config.rules[index], tools: [...(config.rules[index].tools || [])] })
  }

  const applyDraft = () => {
    if (!draft || editingIndex === null) return
    if (!draft.name.trim() || !draft.pattern.trim()) {
      toast.error(copy.ruleRequired)
      return
    }
    if (editingIndex === -1) {
      setConfig((current) => ({ ...current, rules: [...current.rules, draft] }))
    } else {
      updateRule(editingIndex, draft)
    }
    setDraft(null)
    setEditingIndex(null)
  }

  const removeRule = (index: number) => {
    setConfig((current) => ({ ...current, rules: current.rules.filter((_, itemIndex) => itemIndex !== index) }))
  }

  const save = async () => {
    setSaving(true)
    try {
      const saved = normalizeConfig(await saveToolGuardConfig(config))
      setConfig(saved)
      toast.success(copy.saved)
    } catch (error) {
      toast.error(copy.saveFailed + ': ' + (error instanceof Error ? error.message : String(error)))
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return <div className="flex h-full items-center justify-center text-muted-foreground"><Loader2 className="mr-2 h-5 w-5 animate-spin" />{copy.title}</div>
  }

  return (
    <div className="h-full min-h-0 overflow-y-auto bg-background">
      <div className="w-full space-y-4 p-3 md:space-y-6 md:p-5 lg:p-7">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 className="text-xl font-semibold">{copy.rulesTitle}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{copy.rulesHint}</p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={openNew} className="min-h-11 gap-2 md:min-h-0"><Plus className="h-4 w-4" />{copy.addRule}</Button>
            <Button onClick={save} disabled={saving} className="min-h-11 gap-2 md:min-h-0">
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              {saving ? copy.saving : copy.save}
            </Button>
          </div>
        </div>

        <div className="rounded-lg border bg-card">
          <div className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3 text-sm">
            <div className="font-medium">{copy.ruleCount}: {config.rules.length}</div>
          </div>
          <div className="divide-y md:hidden">
            {sortedRules.map(({ rule, index }) => (
              <article key={rule.id || 'draft-' + index} className="space-y-3 p-4">
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <h2 className="break-words font-medium">{rule.name}</h2>
                    <p className="mt-1 text-xs text-muted-foreground">{rule.origin === 'preset' ? copy.presetBadge : copy.customBadge} · {copy.priority}: {rule.priority || 0}</p>
                  </div>
                  <label className="flex min-h-11 shrink-0 items-center gap-2 text-xs">
                    {copy.ruleEnabled}<Switch checked={rule.enabled} onCheckedChange={(enabled) => updateRule(index, { enabled })} aria-label={rule.name + ' ' + copy.ruleEnabled} />
                  </label>
                </div>
                <p className="text-xs text-muted-foreground">{targetLabel(rule.target)} · RE2 · {copy.deny}{(rule.tools || []).length > 0 ? ' · ' + (rule.tools || []).length + ' ' + copy.toolsSelected : ''}</p>
                <code className="block max-h-32 overflow-y-auto whitespace-pre-wrap break-all rounded bg-muted/60 p-2 text-xs">{rule.pattern}</code>
                <div className="flex justify-end gap-2">
                  <Button variant="outline" className="h-11" onClick={() => openEdit(index)} aria-label={copy.editRule + ': ' + rule.name}><Pencil className="h-4 w-4" />{copy.editRule}</Button>
                  <Button variant="ghost" className="h-11 text-destructive" onClick={() => removeRule(index)} aria-label={copy.remove + ': ' + rule.name}><Trash2 className="h-4 w-4" />{copy.remove}</Button>
                </div>
              </article>
            ))}
          </div>
          <div className="hidden md:block">
          <Table className="min-w-[1100px]">
            <TableHeader>
              <TableRow className="bg-muted/30">
                <TableHead className="w-20 pl-4">{copy.priority}</TableHead>
                <TableHead className="min-w-56">{copy.ruleName}</TableHead>
                <TableHead className="w-40">{copy.matchTarget}</TableHead>
                <TableHead className="w-20">{copy.matchType}</TableHead>
                <TableHead className="w-[38%] min-w-56">{copy.pattern}</TableHead>
                <TableHead className="w-24">{copy.strategy}</TableHead>
                <TableHead className="w-20">{copy.ruleEnabled}</TableHead>
                <TableHead className="w-24 text-right pr-4">{copy.actions}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedRules.map(({ rule, index }) => (
                <TableRow key={rule.id || 'draft-' + index}>
                  <TableCell className="pl-4 font-mono text-xs">{rule.priority || 0}</TableCell>
                  <TableCell>
                    <div className="font-medium">{rule.name}</div>
                    <div className="mt-0.5 text-[11px] text-muted-foreground">
                      {rule.origin === 'preset' ? copy.presetBadge : copy.customBadge}
                      {(rule.tools || []).length > 0 ? ' · ' + (rule.tools || []).length + ' ' + copy.toolsSelected : ''}
                    </div>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">{targetLabel(rule.target)}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">RE2</TableCell>
                  <TableCell className="w-[38%]">
                    <code className="block truncate rounded bg-muted/60 px-2 py-1 font-mono text-xs" title={rule.pattern}>{rule.pattern}</code>
                  </TableCell>
                  <TableCell><span className="rounded-md bg-destructive/10 px-2 py-1 text-xs font-medium text-destructive">{copy.deny}</span></TableCell>
                  <TableCell>
                    <Switch checked={rule.enabled} onCheckedChange={(enabled) => updateRule(index, { enabled })} aria-label={rule.name + ' ' + copy.ruleEnabled} />
                  </TableCell>
                  <TableCell className="pr-4 text-right">
                    <div className="flex justify-end gap-1">
                      <Button variant="ghost" size="icon-sm" onClick={() => openEdit(index)} title={copy.editRule} aria-label={copy.editRule + ': ' + rule.name}><Pencil className="h-4 w-4" /></Button>
                      <Button variant="ghost" size="icon-sm" onClick={() => removeRule(index)} title={copy.remove} aria-label={copy.remove + ': ' + rule.name} className="text-destructive hover:text-destructive"><Trash2 className="h-4 w-4" /></Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          </div>
          {config.rules.length === 0 && <p className="px-4 py-10 text-center text-sm text-muted-foreground">{copy.emptyRules}</p>}
        </div>

      </div>

      <Dialog open={draft !== null} onOpenChange={(open) => { if (!open) { setDraft(null); setEditingIndex(null) } }}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl max-md:[&_input]:min-h-11 max-md:[&_input]:text-base max-md:[&_textarea]:text-base max-md:[&_button:not([role=switch])]:min-h-11">
          <DialogHeader>
            <DialogTitle>{editingIndex === -1 ? copy.addRule : copy.editRule}</DialogTitle>
            <DialogDescription>{copy.editorHint}</DialogDescription>
          </DialogHeader>
          {draft && (
            <div className="grid gap-4">
              <div className="grid gap-4 sm:grid-cols-[1fr_120px]">
                <div className="space-y-2">
                  <Label htmlFor="guard-rule-name">{copy.ruleName}</Label>
                  <Input id="guard-rule-name" value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="guard-rule-priority">{copy.priority}</Label>
                  <Input id="guard-rule-priority" type="number" min={-1000} max={1000} value={draft.priority || 0} onChange={(event) => setDraft({ ...draft, priority: Number(event.target.value) })} />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="guard-rule-target">{copy.matchTarget}</Label>
                <Select value={draft.target || 'tool_input'} onValueChange={(value) => setDraft({ ...draft, target: value as RuleTarget })}>
                  <SelectTrigger id="guard-rule-target" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="tool_input">{copy.targetAllInput}</SelectItem>
                    <SelectItem value="executable_input">{copy.targetExecutable}</SelectItem>
                    <SelectItem value="http_method">{copy.targetHttpMethod}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label>{copy.tools}</Label>
                <SelectTag options={toolOptions} value={draft.tools || []} onChange={(tools) => setDraft({ ...draft, tools })} placeholder={copy.toolsPlaceholder} searchPlaceholder={copy.toolsSearchPlaceholder} emptyText={copy.toolsEmpty} />
                <p className="text-xs text-muted-foreground">{copy.toolScopeHint}</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="guard-rule-pattern">{copy.pattern}</Label>
                <Textarea id="guard-rule-pattern" value={draft.pattern} onChange={(event) => setDraft({ ...draft, pattern: event.target.value })} rows={4} className="max-h-40 whitespace-pre-wrap break-all overflow-y-auto font-mono text-xs" spellCheck={false} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="guard-rule-message">{copy.message}</Label>
                <Textarea id="guard-rule-message" value={draft.message} onChange={(event) => setDraft({ ...draft, message: event.target.value })} rows={2} />
                <p className="text-xs text-muted-foreground">{copy.messageHint}</p>
              </div>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => { setDraft(null); setEditingIndex(null) }}>{copy.cancel}</Button>
            <Button onClick={applyDraft}>{copy.applyEdit}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
