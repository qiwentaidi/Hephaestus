'use client'

import { useEffect, useId, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NumberInput } from '@/components/ui/number-input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetFooter } from '@/components/ui/sheet'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { useI18n } from '@/lib/i18n'
import { createAIBatchQueue, type AIBatchQueue } from '@/lib/api/ai-batch'
import { createAIPlanRun, type AIPlanRunDetail, type AIPlanRunListItem } from '@/lib/api/ai-plan'
import { fetchWithAuth } from '@/lib/api'
import { listAIWorkbenchModelOptions, listAllAIWorkbenchPenTestProjects, type AIWorkbenchModelOption, type AIWorkbenchProject } from '@/lib/api/ai-workbench'
import { parseBatchInstructions, type InstructionMode } from '@/lib/ai-batch-view'
import { cn } from '@/lib/utils'
import { AIProjectSelection, matchingAIProjects } from '@/components/ai-project-selection'
import { MessageSquare, Workflow } from 'lucide-react'

interface Props {
  projects: AIWorkbenchProject[]
  projectId?: string
  source?: AIBatchQueue
  planSource?: AIPlanRunListItem
  onClose: () => void
  onCreated: (queue: AIBatchQueue) => void
  onPlanCreated?: (detail: AIPlanRunDetail) => void
}
const GOAL_PLACEHOLDER = '{{.goal}}'
const MANUAL_TEMPLATE = '__manual__'
interface SavedAIDialogueTemplate {
  id: string
  name: string
  description?: string
  config?: { prompt?: string }
}

function modelValue(providerId?: string, model?: string) {
  return model ? `${providerId || ''}::${model}` : ''
}

function copiedPlanTargets(scopeJson?: string): string {
  try {
    const value: unknown = JSON.parse(scopeJson || '[]')
    return Array.isArray(value) ? value.filter((target): target is string => typeof target === 'string').join('\n') : ''
  } catch { return '' }
}

export function TaskCreateDialog({ projects, source, planSource, onClose, onCreated, onPlanCreated }: Props) {
  const { t } = useI18n(); const l = t.aiBatch; const p = t.aiPlan; const id = useId()
  const sourcePlanModel = modelValue(planSource?.run.plannerProviderId, planSource?.run.plannerModel)
  const sourceWorkerModel = planSource?.run.workerModel ? modelValue(planSource.run.workerProviderId, planSource.run.workerModel) : sourcePlanModel
  const separatePlanModels = Boolean(planSource && sourceWorkerModel !== sourcePlanModel)
  const [taskMode, setTaskMode] = useState<'batch' | 'plan'>(planSource ? 'plan' : 'batch')
  const [name, setName] = useState(planSource ? `${planSource.project?.name || projects.find(project => project.id === planSource.run.projectId)?.name || l.unknownProject} ${l.copySuffix}` : source ? `${source.name} ${l.copySuffix}` : '')
  const [project, setProject] = useState(source?.projectId || '')
  const [mode, setMode] = useState<InstructionMode>(source?.items.length === 1 ? 'single' : 'batch')
  const [instructions, setInstructions] = useState(source?.items.length === 1 ? source.items[0].instruction : '')
  const [batchInput, setBatchInput] = useState('')
  const [batchTemplate, setBatchTemplate] = useState('')
  const [savedTemplates, setSavedTemplates] = useState<SavedAIDialogueTemplate[]>([])
  const [selectedTemplateId, setSelectedTemplateId] = useState('')
  const templateRef = useRef<HTMLTextAreaElement | null>(null)
  // Preserve multiline instructions when duplicating a batch.
  const [copiedRows, setCopiedRows] = useState(source && source.items.length > 1 ? source.items.map(item => item.instruction) : [])
  const [concurrency, setConcurrency] = useState(String(source?.concurrency || 1))
  const [batchModel, setBatchModel] = useState(source?.providerId && source?.model ? `${source.providerId}::${source.model}` : '__default__')
  const [executeNow, setExecuteNow] = useState(false)
  const [planObjective, setPlanObjective] = useState(planSource?.run.objective || '')
  const [planTargets, setPlanTargets] = useState(() => copiedPlanTargets(planSource?.run.scopeJson))
  // 智能编排 Worker 并发默认 4（上限 8）；仅影响新建编排任务，既有任务的
  // workers 存在 hephaestus_task_options 里，不会被这个默认值改动。
  const [planWorkers, setPlanWorkers] = useState(String(planSource?.run.maxWorkers ?? 4))
  const [planModels, setPlanModels] = useState<AIWorkbenchModelOption[]>([])
  const [planModel, setPlanModel] = useState(sourcePlanModel)
  const [planWorkerModel, setPlanWorkerModel] = useState(sourceWorkerModel)
  const [loadingPlanModels, setLoadingPlanModels] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => {
    let cancelled = false
    fetchWithAuth('/api/v1/config/scan-templates?taskType=ai_dialogue')
      .then(async response => {
        if (!response.ok) throw new Error('Failed to load AI dialogue templates')
        return response.json()
      })
      .then(data => {
        if (!cancelled && data.success && Array.isArray(data.data)) setSavedTemplates(data.data)
      })
      .catch(error => console.warn('Failed to load AI dialogue templates', error))
    return () => { cancelled = true }
  }, [])
  useEffect(() => {
    let cancelled = false
    listAIWorkbenchModelOptions()
      .then(data => {
        if (cancelled) return
        const usable = data.options.filter(option => option.enabled && option.configured)
        setPlanModels(usable)
        const preferred = usable.find(option => option.providerId === data.defaultProviderId && option.model === data.defaultModel) || usable[0]
        const preferredValue = preferred ? modelValue(preferred.providerId, preferred.model) : ''
        const resolveSelection = (current: string) => {
          if (!current) return preferredValue
          const [providerId, model] = current.split('::')
          if (providerId) return current
          // Older runs may omit the provider. Resolve only an unambiguous or
          // default-provider match; never silently replace an unavailable model.
          const matches = usable.filter(option => option.model === model)
          const match = matches.find(option => option.providerId === data.defaultProviderId) || (matches.length === 1 ? matches[0] : undefined)
          return match ? modelValue(match.providerId, match.model) : current
        }
        setPlanModel(resolveSelection)
        setPlanWorkerModel(resolveSelection)
      })
      .catch(error => { if (!cancelled) console.warn('Failed to load AI models', error) })
      .finally(() => { if (!cancelled) setLoadingPlanModels(false) })
    return () => { cancelled = true }
  }, [])
  const effectivePlanModel = planModel || (planModels[0] ? `${planModels[0].providerId}::${planModels[0].model}` : '')
  const effectiveWorkerModel = separatePlanModels ? planWorkerModel : effectivePlanModel
  const planModelAvailable = planModels.some(option => modelValue(option.providerId, option.model) === effectivePlanModel)
  const workerModelAvailable = planModels.some(option => modelValue(option.providerId, option.model) === effectiveWorkerModel)
  const effectiveBatchModel = batchModel === '__default__' ? '' : batchModel
  const batchGoals = parseBatchInstructions(batchInput, 'batch')
  const renderedBatchRows = batchGoals.map(goal => batchTemplate.trim()
    ? batchTemplate.replaceAll(GOAL_PLACEHOLDER, goal).trim()
    : goal)
  const rows = mode === 'single' ? parseBatchInstructions(instructions, mode) : [...copiedRows.map(row => row.trim()).filter(Boolean), ...renderedBatchRows]
  const insertGoalPlaceholder = () => {
    const textarea = templateRef.current
    if (!textarea) return
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    const next = `${batchTemplate.slice(0, start)}${GOAL_PLACEHOLDER}${batchTemplate.slice(end)}`
    setBatchTemplate(next)
    requestAnimationFrame(() => {
      textarea.focus()
      const cursor = start + GOAL_PLACEHOLDER.length
      textarea.setSelectionRange(cursor, cursor)
    })
  }
  const selectSavedTemplate = (value: string) => {
    if (value === '__none__') {
      setSelectedTemplateId('')
      setBatchTemplate('')
      return
    }
    if (value === MANUAL_TEMPLATE) return
    const selected = savedTemplates.find(template => template.id === value)
    if (!selected) return
    setSelectedTemplateId(value)
    setBatchTemplate(selected.config?.prompt || '')
  }
  const save = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy) return
    if (taskMode === 'plan') {
      const targets = [...new Set(planTargets.split(/[\n,，;；]+/).map(value => value.trim()).filter(Boolean))]
      const n = Number(planWorkers)
      if (!name.trim() || !targets.length || !planObjective.trim() || !effectivePlanModel || !Number.isInteger(n) || n < 1 || n > 8) {
        setError(p.createValidation)
        return
      }
      if (!planModelAvailable || !workerModelAvailable) { setError(p.modelUnavailable); return }
      if (matchingAIProjects(projects, name).length) { setError(p.existingProjectHint); return }
      if (targets.length > 100) { setError(p.targetsLimit); return }
      const [providerId, model] = effectivePlanModel.split('::')
      const [workerProviderId, workerModel] = effectiveWorkerModel.split('::')
      if (!providerId || !model) { setError(p.createValidation); return }
      setBusy(true); setError('')
      try {
        const latestProjects = await listAllAIWorkbenchPenTestProjects()
        if (matchingAIProjects(latestProjects, name).length) throw new Error(p.existingProjectHint)
        const detail = await createAIPlanRun({
          projectName: name.trim(),
          objective: planObjective.trim(),
          targets,
          scope: targets,
          plannerProviderId: providerId,
          plannerModel: model,
          workerProviderId,
          workerModel,
          maxWorkers: n,
          // Worker total is unlimited; only the Planner's finish_plan ends the run.
          maxTotalWorkers: 0,
        })
        onPlanCreated?.(detail)
      } catch (e) { setError(e instanceof Error ? e.message : p.createValidation) }
      finally { setBusy(false) }
      return
    }
    const n = Number(concurrency)
    if (mode === 'batch' && batchTemplate.trim() && !batchTemplate.includes(GOAL_PLACEHOLDER)) { setError(l.templateMissingGoal); return }
    if (!name.trim() || !rows.length || !Number.isInteger(n) || n < 1 || n > 8) { setError(l.validation); return }
    if (rows.length > 1000 || rows.some(row => new TextEncoder().encode(row).length > 32000)) { setError(l.lengthLimit); return }
    setBusy(true); setError('')
    try {
      const [batchProviderId, batchModelName] = effectiveBatchModel ? effectiveBatchModel.split('::') : ['', '']
      const queue = await createAIBatchQueue({ name: name.trim(), projectId: project || undefined, instructions: rows, concurrency: n, executeNow, providerId: batchProviderId || source?.providerId || '', model: batchModelName || source?.model || '' })
      onCreated(queue)
    } catch (e) { setError(e instanceof Error ? e.message : l.error) }
    finally { setBusy(false) }
  }
  return <Sheet open onOpenChange={open => { if (!open && !busy) onClose() }}>
    <SheetContent side="right" className="h-dvh w-full gap-0 overflow-hidden p-0 sm:max-w-[680px]">
      <SheetHeader className="shrink-0 border-b px-6 py-5 pr-12"><SheetTitle>{source || planSource ? l.copyTask : l.newQueue}</SheetTitle></SheetHeader>
      <form id={id} onSubmit={save} className="min-h-0 flex-1 space-y-5 overflow-y-auto px-6 py-5">
        <div className="grid gap-3 sm:grid-cols-2" role="radiogroup" aria-label={l.title}>
          <button type="button" role="radio" aria-checked={taskMode === 'batch'} disabled={busy} className={cn('flex items-start gap-3 rounded-lg border p-4 text-left transition-colors focus-visible:outline-2 focus-visible:outline-primary', taskMode === 'batch' ? 'border-primary bg-primary/5 ring-1 ring-primary/20' : 'border-border hover:border-primary/40')} onClick={() => { setTaskMode('batch'); setError('') }}>
            <span className={cn('flex size-9 shrink-0 items-center justify-center rounded-lg border', taskMode === 'batch' ? 'border-primary/30 bg-primary/10 text-primary' : 'border-border bg-muted/40 text-muted-foreground')}><MessageSquare className="size-4" /></span>
            <span className="min-w-0 flex-1">
              <span className="flex flex-wrap items-center gap-2 text-sm font-semibold">{l.batchTab}<Badge variant="outline" className="h-5 rounded px-1.5 text-[10px] font-normal text-muted-foreground">{l.modeBatchBadge}</Badge></span>
              <span className="mt-1 block text-xs leading-5 text-muted-foreground">{l.modeBatchHint}</span>
            </span>
          </button>
          <button type="button" role="radio" aria-checked={taskMode === 'plan'} disabled={busy} className={cn('flex items-start gap-3 rounded-lg border p-4 text-left transition-colors focus-visible:outline-2 focus-visible:outline-primary', taskMode === 'plan' ? 'border-primary bg-primary/5 ring-1 ring-primary/20' : 'border-border hover:border-primary/40')} onClick={() => { setTaskMode('plan'); setError('') }}>
            <span className={cn('flex size-9 shrink-0 items-center justify-center rounded-lg border', taskMode === 'plan' ? 'border-primary/30 bg-primary/10 text-primary' : 'border-border bg-muted/40 text-muted-foreground')}><Workflow className="size-4" /></span>
            <span className="min-w-0 flex-1">
              <span className="flex flex-wrap items-center gap-2 text-sm font-semibold">{p.tab}<Badge variant="outline" className="h-5 rounded border-amber-500/35 bg-amber-500/10 px-1.5 text-[10px] font-normal text-amber-700 dark:text-amber-300">{p.modeBadge}</Badge></span>
              <span className="mt-1 block text-xs leading-5 text-muted-foreground">{p.modeHint}</span>
            </span>
          </button>
        </div>
        {error && <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert>}
        <div className="space-y-2"><Label htmlFor={`${id}-name`}>{taskMode === 'plan' ? p.projectName : l.name}</Label><Input id={`${id}-name`} autoFocus maxLength={200} value={name} onChange={e => setName(e.target.value)} disabled={busy} required /></div>
        <AIProjectSelection id={`${id}-project`} projects={projects} projectId={project} onChange={setProject} orchestration={taskMode === 'plan'} name={name} disabled={busy} />
        {taskMode === 'batch' ? <>
        <div className="space-y-3">
          <div className="flex gap-4 border-b" role="group" aria-label={l.instruction}>
            {(['batch', 'single'] as const).map(value => <button key={value} type="button" aria-pressed={mode === value} disabled={busy} className={cn('border-b-2 px-1 pb-2 text-sm focus-visible:outline-2 focus-visible:outline-primary', mode === value ? 'border-primary font-medium text-primary' : 'border-transparent text-muted-foreground')} onClick={() => setMode(value)}>{value === 'single' ? l.singleInstruction : l.batchInstructions}</button>)}
          </div>
          {mode === 'batch' && <div className="space-y-2">
            <Label htmlFor={`${id}-template`}>{l.templateLabel}</Label>
            <Select value={selectedTemplateId || (batchTemplate ? MANUAL_TEMPLATE : '__none__')} onValueChange={selectSavedTemplate} disabled={busy}>
              <SelectTrigger className="w-full"><SelectValue placeholder={l.chooseTemplate} /></SelectTrigger>
              <SelectContent>
                <SelectItem value="__none__">{l.noTemplate}</SelectItem>
                {batchTemplate && <SelectItem value={MANUAL_TEMPLATE}>{l.customTemplate}</SelectItem>}
                {savedTemplates.map(template => <SelectItem key={template.id} value={template.id}>{template.name}</SelectItem>)}
              </SelectContent>
            </Select>
            <Textarea ref={templateRef} id={`${id}-template`} className="min-h-24 leading-6" value={batchTemplate} onChange={e => { setBatchTemplate(e.target.value); setSelectedTemplateId('') }} placeholder={l.templatePlaceholder} disabled={busy} />
            <div className="flex flex-wrap items-center gap-2">
              <Button type="button" size="sm" variant="outline" disabled={busy} onClick={insertGoalPlaceholder}>{l.insertGoal} <code className="ml-1">{GOAL_PLACEHOLDER}</code></Button>
            </div>
            <p className="text-sm text-muted-foreground">{l.templateLibraryHint}</p>
          </div>}
          <Label htmlFor={`${id}-instructions`}>{mode === 'batch' ? l.goalListLabel : l.instruction}</Label>
          <Textarea id={`${id}-instructions`} className="h-36 min-h-32 field-sizing-fixed leading-6" required={mode === 'single' || !copiedRows.length} value={mode === 'single' ? instructions : batchInput} onChange={e => mode === 'single' ? setInstructions(e.target.value) : setBatchInput(e.target.value)} placeholder={mode === 'batch' ? l.goalListPlaceholder : undefined} disabled={busy} />
          <p className="text-sm text-muted-foreground">{mode === 'single' ? l.singleHint : l.goalListHint}</p>
          {mode === 'batch' && rows.length > 0 && <details open={copiedRows.length > 0 || undefined} className="rounded-md border p-3">
            <summary className="cursor-pointer text-sm">{l.preview} · {rows.length}</summary>
            <div className="mt-3 max-h-48 space-y-2 overflow-y-auto">
              {copiedRows.map((row, index) => <div key={index} className="flex items-start gap-2"><span className="pt-2 text-xs text-muted-foreground">{index + 1}</span><Textarea aria-label={`${l.instruction} ${index + 1}`} value={row} disabled={busy} onChange={e => setCopiedRows(prev => prev.map((value, i) => i === index ? e.target.value : value))} /><Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => setCopiedRows(prev => prev.filter((_, i) => i !== index))}>{t.common.delete}</Button></div>)}
              {renderedBatchRows.map((row, index) => <p key={index} className="break-words border-b py-2 text-sm">{copiedRows.length + index + 1}. {row}</p>)}
            </div>
          </details>}
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2"><Label htmlFor={`${id}-batch-model`}>{p.model}</Label><Select value={batchModel} onValueChange={setBatchModel} disabled={busy || loadingPlanModels}><SelectTrigger id={`${id}-batch-model`} className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="__default__">{l.defaultModel}</SelectItem>{planModels.map(option => <SelectItem key={`${option.providerId}::${option.model}`} value={`${option.providerId}::${option.model}`}>{option.providerName} · {option.model}</SelectItem>)}</SelectContent></Select></div>
          <div className="space-y-2"><Label htmlFor={`${id}-concurrency`}>{l.concurrency}</Label><NumberInput id={`${id}-concurrency`} min={1} max={8} step={1} value={concurrency} onChange={e => setConcurrency(e.target.value)} disabled={busy} required /></div>
        </div>
        <div className="flex items-center gap-2"><Checkbox id={`${id}-execute`} checked={executeNow} onCheckedChange={v => setExecuteNow(v === true)} disabled={busy} /><Label htmlFor={`${id}-execute`}>{l.executeNow}</Label></div>
        </> : <>
        <div className="space-y-2"><Label htmlFor={`${id}-plan-objective`}>{p.objective}</Label><Textarea id={`${id}-plan-objective`} className="min-h-24 resize-y" value={planObjective} onChange={e => setPlanObjective(e.target.value)} placeholder={p.objectivePlaceholder} disabled={busy} required /></div>
        <div className="space-y-2"><Label htmlFor={`${id}-plan-targets`}>{p.targets}</Label><Textarea id={`${id}-plan-targets`} className="min-h-36 resize-y font-mono text-sm" value={planTargets} onChange={e => setPlanTargets(e.target.value)} placeholder={p.targetsPlaceholder} disabled={busy} /><p className="text-xs text-muted-foreground">{p.targetsHint}</p></div>
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2"><Label htmlFor={`${id}-plan-model`}>{separatePlanModels ? p.plannerModel : p.model}</Label><Select value={effectivePlanModel} onValueChange={setPlanModel} disabled={busy || loadingPlanModels || !planModels.length}><SelectTrigger id={`${id}-plan-model`} className="w-full"><SelectValue placeholder={p.model} /></SelectTrigger><SelectContent>{effectivePlanModel && !planModelAvailable && <SelectItem value={effectivePlanModel} disabled>{effectivePlanModel.split('::')[1]} · {p.unavailableModel}</SelectItem>}{planModels.map(option => <SelectItem key={`${option.providerId}::${option.model}`} value={`${option.providerId}::${option.model}`}>{option.providerName} · {option.model}</SelectItem>)}</SelectContent></Select></div>
          {separatePlanModels && <div className="space-y-2"><Label htmlFor={`${id}-worker-model`}>{p.workerModel}</Label><Select value={effectiveWorkerModel} onValueChange={setPlanWorkerModel} disabled={busy || loadingPlanModels || !planModels.length}><SelectTrigger id={`${id}-worker-model`} className="w-full"><SelectValue placeholder={p.workerModel} /></SelectTrigger><SelectContent>{effectiveWorkerModel && !workerModelAvailable && <SelectItem value={effectiveWorkerModel} disabled>{effectiveWorkerModel.split('::')[1]} · {p.unavailableModel}</SelectItem>}{planModels.map(option => <SelectItem key={`${option.providerId}::${option.model}`} value={`${option.providerId}::${option.model}`}>{option.providerName} · {option.model}</SelectItem>)}</SelectContent></Select></div>}
          <div className="space-y-2"><Label htmlFor={`${id}-plan-workers`}>{p.maxWorkers}</Label><NumberInput id={`${id}-plan-workers`} min={1} max={8} step={1} value={planWorkers} onChange={e => setPlanWorkers(e.target.value)} disabled={busy} /><p className="text-xs text-muted-foreground">{p.maxWorkersHint}</p></div>
        </div>
        {!loadingPlanModels && planModels.length > 0 && (!planModelAvailable || !workerModelAvailable) ? <Alert><AlertDescription>{p.modelUnavailable}</AlertDescription></Alert> : null}
        {!loadingPlanModels && !planModels.length ? <Alert variant="destructive"><AlertDescription>{p.noModels}</AlertDescription></Alert> : null}
        </>}
      </form>
      <SheetFooter className="shrink-0 flex-row justify-end border-t px-6 py-4"><Button variant="outline" disabled={busy} onClick={onClose}>{l.cancel}</Button><Button type="submit" form={id} disabled={busy || (taskMode === 'plan' && (loadingPlanModels || !planModelAvailable || !workerModelAvailable))}>{busy ? l.loading : taskMode === 'plan' ? p.create : executeNow ? l.createRun : l.save}</Button></SheetFooter>
    </SheetContent>
  </Sheet>
}
