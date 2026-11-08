'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import {
  Bookmark,
  Bot,
  CheckCircle2,
  ExternalLink,
  Workflow,
} from 'lucide-react'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NumberInput } from '@/components/ui/number-input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { buildCombinationPresetCards } from '@/app/(main)/scan/vuln-scan/components/combinationPresetUi'
import { fetchWithAuth } from '@/lib/api'
import { getProxyUrl } from '@/lib/proxy'
import { createAIBatchQueue } from '@/lib/api/ai-batch'
import { createAIPlanRun } from '@/lib/api/ai-plan'
import {
  listAIWorkbenchModelOptions,
  listAllAIWorkbenchPenTestProjects,
  type AIWorkbenchProject,
  type AIWorkbenchModelOptionsResponse,
} from '@/lib/api/ai-workbench'
import type { AssetLedgerEntity, AssetLedgerEntityCategory } from '@/lib/api/unified-assets'
import {
  buildAIInstructions,
  deriveAITargets,
  deriveCombinationTargets,
  summarizeSkippedReasons,
  type AssetScanSkipReason,
} from '@/lib/asset-scan'
import {
  ASSET_SCAN_COMBINATION_DRAFT_KEY,
  buildPresetCombinationDraft,
  buildTemplateCombinationDraft,
  combinationTemplateInputTypeFromNodes,
  type CombinationDraftInput,
  type CombinationTargetInputType,
} from '@/lib/combination-presets'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'
import { cn } from '@/lib/utils'
import { AIProjectSelection, matchingAIProjects } from '@/components/ai-project-selection'

const PRESET_PREFIX = 'preset:'
const SAVED_PREFIX = 'saved:'
const BUILT_IN_AI_TEMPLATE = '__builtin__'

type ScanMode = 'combination' | 'ai'

interface CombinationTemplate {
  id: string
  name: string
  description?: string
  config?: unknown
}

interface AIDialogueTemplate {
  id: string
  name: string
  description?: string
  config?: { prompt?: string }
}

type CreatedResult =
  | { kind: 'combination'; taskId: string; name: string }
  | { kind: 'ai'; queueId: string; name: string; count: number }
  | { kind: 'plan'; runId: string; projectId: string; name: string; count: number }

function formatText(template: string, values: Record<string, string | number>) {
  return Object.entries(values).reduce(
    (text, [key, value]) => text.replaceAll(`{${key}}`, String(value)),
    template,
  )
}

function defaultPresetId(category: AssetLedgerEntityCategory) {
  switch (category) {
    case 'website':
      return 'web-deep-scan'
    case 'ip':
    case 'port':
      return 'host-discovery'
    default:
      return 'asset-discovery'
  }
}

async function loadScanTemplates(taskType: string): Promise<any[]> {
  const response = await fetchWithAuth(`/api/v1/config/scan-templates?taskType=${encodeURIComponent(taskType)}`)
  const body = await response.json().catch(() => null)
  if (!response.ok || !body?.success) {
    throw new Error(body?.error || `HTTP ${response.status}`)
  }
  return Array.isArray(body.data) ? body.data : []
}

function countNodeStages(nodes: Array<{ stageIndex?: number }>): number {
  if (!nodes.length) return 0
  const stages = nodes.map((node) => Math.max(0, Number(node.stageIndex) || 0))
  return Math.max(...stages) - Math.min(...stages) + 1
}

function TemplateCard({
  title,
  description,
  icon: Icon,
  tone,
  selected,
  disabled,
  badge,
  footnote,
  onSelect,
}: {
  title: string
  description: string
  icon: typeof Bookmark
  tone: string
  selected: boolean
  disabled?: boolean
  badge: string | number
  footnote: string
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      disabled={disabled}
      onClick={onSelect}
      className={cn(
        'flex min-w-0 flex-col gap-2 rounded-lg border bg-card p-3 text-left transition-colors',
        selected ? 'border-primary bg-primary/5' : 'border-border',
        !disabled && 'hover:border-primary/50',
        disabled && 'cursor-not-allowed opacity-60',
        !disabled && !selected && badge === 0 && 'opacity-70',
      )}
    >
      <div className="flex min-w-0 items-start gap-2">
        <span className={cn('mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md bg-muted/60', tone)}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-sm font-medium">{title}</span>
            <Badge variant={selected ? 'default' : 'outline'} className="shrink-0 text-[11px] tabular-nums">
              {badge}
            </Badge>
          </div>
          <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{description}</p>
        </div>
      </div>
      <span className="line-clamp-1 text-[11px] text-muted-foreground" title={footnote}>{footnote}</span>
    </button>
  )
}

export function AssetScanDialog({
  open,
  onOpenChange,
  entities,
  activeCategory,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  entities: AssetLedgerEntity[]
  activeCategory: AssetLedgerEntityCategory
  onCreated?: () => void
}) {
  const router = useRouter()
  const { t, language } = useI18n()
  const text = t.assetScan
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'

  const [mode, setMode] = useState<ScanMode>('combination')
  const [combinationTemplates, setCombinationTemplates] = useState<CombinationTemplate[]>([])
  const [loadingCombinationTemplates, setLoadingCombinationTemplates] = useState(true)
  const [combinationTemplateId, setCombinationTemplateId] = useState(
    () => `${PRESET_PREFIX}${defaultPresetId(activeCategory)}`,
  )
  const [aiTemplates, setAITemplates] = useState<AIDialogueTemplate[]>([])
  const [loadingAITemplates, setLoadingAITemplates] = useState(true)
  const [selectedAITemplateId, setSelectedAITemplateId] = useState('')
  const [modelOptions, setModelOptions] = useState<AIWorkbenchModelOptionsResponse | null>(null)
  const [selectedModel, setSelectedModel] = useState('')
  const [concurrency, setConcurrency] = useState('3')
  // 智能编排的 Worker 并发默认 4（与「新建任务」对齐）；批量模式的 concurrency
  // 是另一套语义（同时跑的会话数），保持原默认 3，两者互不影响。
  const [planWorkers, setPlanWorkers] = useState('4')
  const [executeNow, setExecuteNow] = useState(true)
  const [aiExecution, setAIExecution] = useState<'orchestration' | 'batch'>('orchestration')
  const [planObjective, setPlanObjective] = useState('')
  const [taskNameOverride, setTaskNameOverride] = useState<string | null>(null)
  const [projects, setProjects] = useState<AIWorkbenchProject[]>([])
  const [projectId, setProjectId] = useState('')
  const [loadingProjects, setLoadingProjects] = useState(true)
  const [projectsFailed, setProjectsFailed] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [created, setCreated] = useState<CreatedResult | null>(null)

  const timeLabel = useMemo(
    () => new Date().toLocaleString(locale, {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    }),
    [locale],
  )

  useEffect(() => {
    let cancelled = false
    listAllAIWorkbenchPenTestProjects().then(data => { if (!cancelled) setProjects(data) })
      .catch(() => { if (!cancelled) setProjectsFailed(true) })
      .finally(() => { if (!cancelled) setLoadingProjects(false) })
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      const results = await Promise.allSettled([
        loadScanTemplates('combination'),
        loadScanTemplates('ai_dialogue'),
        listAIWorkbenchModelOptions(),
      ])
      if (cancelled) return

      if (results[0].status === 'fulfilled') {
        setCombinationTemplates(
          results[0].value
            .map((item: any) => ({
              id: String(item?.id || ''),
              name: String(item?.name || ''),
              description: typeof item?.description === 'string' ? item.description : '',
              config: item?.config,
            }))
            .filter((item) => item.id && item.name),
        )
      }
      setLoadingCombinationTemplates(false)

      if (results[1].status === 'fulfilled') {
        setAITemplates(
          results[1].value
            .map((item: any) => ({
              id: String(item?.id || ''),
              name: String(item?.name || ''),
              description: typeof item?.description === 'string' ? item.description : '',
              config: item?.config && typeof item.config === 'object'
                ? { prompt: typeof item.config.prompt === 'string' ? item.config.prompt : '' }
                : undefined,
            }))
            .filter((item) => item.id && item.name),
        )
      }
      setLoadingAITemplates(false)

      if (results[2].status === 'fulfilled') {
        setModelOptions(results[2].value)
      }

    }
    void load()
    return () => {
      cancelled = true
    }
  }, [])


  const presetCards = useMemo(() => buildCombinationPresetCards(t, formatText), [t])

  const selectedPresetId = combinationTemplateId.startsWith(PRESET_PREFIX)
    ? combinationTemplateId.slice(PRESET_PREFIX.length)
    : ''
  const selectedPresetCard = presetCards.find((card) => card.id === selectedPresetId)
  const selectedSavedTemplate = combinationTemplateId.startsWith(SAVED_PREFIX)
    ? combinationTemplates.find((item) => item.id === combinationTemplateId.slice(SAVED_PREFIX.length))
    : undefined

  const presetMeta = useMemo(() => {
    const meta: Record<string, { inputType: CombinationTargetInputType | null; count: number }> = {}
    for (const card of presetCards) {
      const draft = buildPresetCombinationDraft(card.id, [], '')
      const inputType = draft ? combinationTemplateInputTypeFromNodes(draft.nodes) : null
      meta[card.id] = {
        inputType,
        count: inputType ? deriveCombinationTargets(entities, inputType).targets.length : 0,
      }
    }
    return meta
  }, [entities, presetCards])

  const savedTemplateCards = useMemo(
    () => combinationTemplates.map((template) => {
      const nodes = buildTemplateCombinationDraft(template.config, [], '')?.nodes || []
      const inputType = combinationTemplateInputTypeFromNodes(nodes)
      return {
        ...template,
        inputType,
        count: inputType ? deriveCombinationTargets(entities, inputType).targets.length : 0,
        stageCount: countNodeStages(nodes),
        moduleCount: nodes.length,
      }
    }),
    [combinationTemplates, entities],
  )

  const selectedTemplateNodes = useMemo(() => {
    if (selectedSavedTemplate) {
      return buildTemplateCombinationDraft(selectedSavedTemplate.config, [], '')?.nodes || []
    }
    if (selectedPresetId) {
      return buildPresetCombinationDraft(selectedPresetId, [], '')?.nodes || []
    }
    return []
  }, [selectedPresetId, selectedSavedTemplate])

  const combinationInputType = useMemo(
    () => combinationTemplateInputTypeFromNodes(selectedTemplateNodes),
    [selectedTemplateNodes],
  )

  const combinationStageCount = useMemo(() => {
    if (!selectedTemplateNodes.length) return 0
    const stages = selectedTemplateNodes.map((node) => node.stageIndex)
    return Math.max(...stages) - Math.min(...stages) + 1
  }, [selectedTemplateNodes])

  const combinationTargets = useMemo(() => {
    if (!combinationInputType) {
      return {
        targets: [] as string[],
        skipped: entities.map((entity) => ({ entity, reason: 'invalid-template' as AssetScanSkipReason })),
      }
    }
    return deriveCombinationTargets(entities, combinationInputType)
  }, [combinationInputType, entities])

  const skippedSummary = useMemo(
    () => summarizeSkippedReasons(combinationTargets.skipped),
    [combinationTargets.skipped],
  )

  const aiTargets = useMemo(() => deriveAITargets(entities), [entities])
  const selectedAITemplate = aiTemplates.find((item) => item.id === selectedAITemplateId)
  const aiInstructions = useMemo(
    () => buildAIInstructions(aiTargets, selectedAITemplate?.config?.prompt, language),
    [aiTargets, selectedAITemplate, language],
  )

  const usableModelOptions = useMemo(
    () => (modelOptions?.options || []).filter((option) => option.enabled && option.configured),
    [modelOptions],
  )

  const defaultModelValue = useMemo(() => {
    if (!usableModelOptions.length) return ''
    const preferred = usableModelOptions.find((option) => option.defaultForWorkbench)
      || usableModelOptions.find((option) => option.providerId === modelOptions?.defaultProviderId)
      || usableModelOptions[0]
    return `${preferred.providerId}::${preferred.model}`
  }, [modelOptions?.defaultProviderId, usableModelOptions])
  const activeModel = selectedModel || defaultModelValue

  const templateLabel = mode === 'combination'
    ? selectedSavedTemplate?.name || selectedPresetCard?.title || text.combination.title
    : text.ai.title
  const defaultTaskName = formatText(text.taskName, {
    template: templateLabel,
    count: mode === 'combination' ? combinationTargets.targets.length : aiTargets.length,
    time: timeLabel,
  })
  const taskName = taskNameOverride ?? defaultTaskName
  const taskNameInvalid = !taskName.trim()
  const existingPlanProject = mode === 'ai' && aiExecution === 'orchestration' && matchingAIProjects(projects, taskName).length > 0

  const buildCombinationDraft = useCallback((targets: string[]): CombinationDraftInput | null => {
    const name = taskName.trim()
    if (selectedSavedTemplate) {
      return buildTemplateCombinationDraft(selectedSavedTemplate.config, targets, name)
    }
    if (selectedPresetId) {
      return buildPresetCombinationDraft(selectedPresetId, targets, name)
    }
    return null
  }, [selectedPresetId, selectedSavedTemplate, taskName])

  const createDisabled = creating
    || !taskName.trim()
    || (mode === 'combination'
      ? !combinationInputType || combinationTargets.targets.length === 0
      : aiTargets.length === 0 || !activeModel || loadingProjects || projectsFailed || existingPlanProject)

  const handleCreate = async () => {
    if (creating) return
    setError('')
    setCreating(true)
    try {
      if (taskNameInvalid) throw new Error(text.taskNameRequired)
      if (mode === 'combination') {
        if (!combinationInputType) throw new Error(text.invalidTemplate)
        if (!combinationTargets.targets.length) throw new Error(text.combination.noTargets)
        const draft = buildCombinationDraft(combinationTargets.targets)
        if (!draft || !draft.nodes.length) throw new Error(text.invalidTemplate)
        const proxy = draft.proxy?.trim() || getProxyUrl()
        const payload = proxy ? { ...draft, proxy } : draft
        const response = await fetchWithAuth('/api/v1/webscan/combination/tasks', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        })
        const body = await response.json().catch(() => null)
        if (!response.ok || !body?.success) {
          throw new Error(body?.error || text.createFailed)
        }
        const taskId = String(body.data?.parentTaskId || body.data?.combinationId || '')
        setCreated({ kind: 'combination', taskId, name: draft.name })
        toast.success(text.combination.created)
        onCreated?.()
      } else {
        if (loadingProjects || projectsFailed) throw new Error(t.aiPlan.projectsLoadFailed)
        if (existingPlanProject) throw new Error(t.aiPlan.existingProjectHint)
        if (!aiTargets.length) throw new Error(text.ai.noTargets)
        if (!activeModel) throw new Error(text.ai.noModels)
        const [providerId, model] = activeModel.split('::')
        if (!providerId || !model) throw new Error(text.ai.noModels)
        const targets = aiTargets.map((target) => target.target)
        if (aiExecution === 'orchestration') {
          const n = Number(planWorkers)
          if (!Number.isInteger(n) || n < 1 || n > 8) throw new Error(text.ai.concurrencyInvalid)
          const latestProjects = await listAllAIWorkbenchPenTestProjects()
          setProjects(latestProjects)
          if (matchingAIProjects(latestProjects, taskName).length) throw new Error(t.aiPlan.existingProjectHint)
          const objective = planObjective.trim() || formatText(text.ai.defaultObjective, { count: targets.length })
          const plan = await createAIPlanRun({
            objective,
            targets,
            scope: targets,
            projectName: taskName.trim(),
            plannerProviderId: providerId,
            plannerModel: model,
            workerProviderId: providerId,
            workerModel: model,
            maxWorkers: n,
            // Worker total is unlimited; the Planner keeps delegating until it
            // explicitly finishes the run.
            maxTotalWorkers: 0,
          })
          setCreated({ kind: 'plan', runId: plan.run.id, projectId: plan.project?.id || plan.run.projectId, name: plan.project?.name || taskName.trim(), count: targets.length })
          toast.success(text.ai.planCreated)
        } else {
          const n = Number(concurrency)
          if (!Number.isInteger(n) || n < 1 || n > 8) throw new Error(text.ai.concurrencyInvalid)
          const queue = await createAIBatchQueue({
            name: taskName.trim(),
            projectId: projectId || undefined,
            instructions: aiInstructions,
            concurrency: n,
            providerId,
            model,
            executeNow,
          })
          const count = queue.items.length || aiInstructions.length
          setCreated({ kind: 'ai', queueId: queue.id, name: queue.name, count })
          toast.success(formatText(text.ai.created, { count }))
        }
        onCreated?.()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : text.createFailed)
    } finally {
      setCreating(false)
    }
  }

  const handleAdvanced = () => {
    if (taskNameInvalid) {
      setError(text.taskNameRequired)
      return
    }
    const draft = buildCombinationDraft(combinationTargets.targets)
    if (!draft) return
    try {
      window.sessionStorage.setItem(ASSET_SCAN_COMBINATION_DRAFT_KEY, JSON.stringify(draft))
    } catch {
      toast.error(text.createFailed)
      return
    }
    onOpenChange(false)
    router.push('/scan/vuln-scan/new?type=combination')
  }

  return (
    <Sheet open={open} onOpenChange={(next) => { if (!creating) onOpenChange(next) }}>
      <SheetContent side="right" className="flex h-dvh w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <SheetHeader className="shrink-0 border-b px-6 py-5 pr-12">
          <SheetTitle>{text.title}</SheetTitle>
          <SheetDescription>
            {formatText(text.description, { count: entities.length })}
          </SheetDescription>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {created ? (
          <div className="flex flex-col items-center gap-3 py-6 text-center">
            <div className="flex size-12 items-center justify-center rounded-full bg-emerald-500/10">
              <CheckCircle2 className="size-6 text-emerald-600" />
            </div>
            <div>
              <p className="font-medium">
                {created.kind === 'combination'
                  ? text.combination.created
                  : created.kind === 'plan'
                    ? text.ai.planCreated
                    : formatText(text.ai.created, { count: created.count })}
              </p>
              <p className="mt-1 break-all text-sm text-muted-foreground">{created.name}</p>
            </div>
            <div className="flex flex-wrap justify-center gap-2">
              {created.kind === 'combination' && created.taskId ? (
                <Button asChild>
                  <Link href={`/scan/vuln-scan/detail?taskId=${encodeURIComponent(created.taskId)}`}>
                    <ExternalLink className="size-4" />
                    {text.combination.viewTask}
                  </Link>
                </Button>
              ) : null}
              {created.kind === 'ai' ? (
                <Button asChild>
                  <Link href={`/ai/tasks?queue=${encodeURIComponent(created.queueId)}`}>
                    <ExternalLink className="size-4" />
                    {text.ai.viewTasks}
                  </Link>
                </Button>
              ) : null}
              {created.kind === 'plan' ? (
                <Button asChild>
                  <Link href={`/ai/workbench/project?project=${encodeURIComponent(created.projectId)}&tab=plan`}>
                    <ExternalLink className="size-4" />
                    {text.ai.viewPlan}
                  </Link>
                </Button>
              ) : null}
              <Button variant="outline" onClick={() => onOpenChange(false)}>{text.close}</Button>
            </div>
          </div>
        ) : (
          <>
            <Tabs value={mode} onValueChange={(value) => setMode(value as ScanMode)}>
              <TabsList className="grid h-auto w-full grid-cols-2 p-0.5">
                <TabsTrigger value="combination" className="gap-2 hover:border-border/40 hover:bg-muted/25">
                  <Workflow className="size-4" />
                  {text.modes.combination}
                </TabsTrigger>
                <TabsTrigger value="ai" className="gap-2 hover:border-border/40 hover:bg-muted/25">
                  <Bot className="size-4" />
                  {text.modes.ai}
                </TabsTrigger>
              </TabsList>

              <div className="mt-4 space-y-1.5">
                <Label htmlFor="asset-scan-task-name">{text.taskNameLabel}</Label>
                <Input
                  id="asset-scan-task-name"
                  value={taskName}
                  placeholder={text.taskNamePlaceholder}
                  required
                  aria-invalid={taskNameInvalid}
                  aria-describedby={taskNameInvalid ? 'asset-scan-task-name-error' : undefined}
                  onChange={(event) => setTaskNameOverride(event.target.value)}
                />
                {taskNameInvalid ? (
                  <p id="asset-scan-task-name-error" role="alert" className="text-sm text-destructive">
                    {text.taskNameRequired}
                  </p>
                ) : null}
              </div>

              {mode === 'ai' && <div className="mt-4 space-y-2">
                <AIProjectSelection id="asset-scan-project" projects={projects} projectId={projectId} onChange={setProjectId} orchestration={aiExecution === 'orchestration'} name={taskName} disabled={creating || loadingProjects || projectsFailed} />
                {projectsFailed && <p role="alert" className="text-sm text-destructive">{t.aiPlan.projectsLoadFailed}</p>}
              </div>}

              <TabsContent value="combination" className="mt-4 space-y-4">
                <div className="space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <Label>{text.combination.presets}</Label>
                    <span className="text-xs text-muted-foreground">
                      {formatText(text.combination.stageCount, { count: combinationStageCount })}
                    </span>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-2">
                    {presetCards.map((card) => {
                      const selected = selectedPresetId === card.id
                      const count = presetMeta[card.id]?.count ?? 0
                      return (
                        <TemplateCard
                          key={card.id}
                          title={card.title}
                          description={card.description}
                          icon={card.icon}
                          tone={card.tone}
                          selected={selected}
                          badge={count}
                          footnote={formatText(text.combination.targetCount, { count })}
                          onSelect={() => setCombinationTemplateId(`${PRESET_PREFIX}${card.id}`)}
                        />
                      )
                    })}
                  </div>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <Label>{text.combination.savedTemplates}</Label>
                    {loadingCombinationTemplates ? (
                      <span className="text-xs text-muted-foreground">{text.combination.loadingTemplates}</span>
                    ) : null}
                  </div>
                  {savedTemplateCards.length ? (
                    <div className="grid gap-2 sm:grid-cols-2">
                      {savedTemplateCards.map((card) => {
                        const selected = selectedSavedTemplate?.id === card.id
                        return (
                          <TemplateCard
                            key={card.id}
                            title={card.name}
                            description={card.description || formatText(text.combination.savedTemplateSummary, {
                              stages: card.stageCount,
                              modules: card.moduleCount,
                            })}
                            icon={Bookmark}
                            tone="text-sky-500"
                            selected={selected}
                            disabled={!card.inputType}
                            badge={card.inputType ? card.count : '—'}
                            footnote={card.inputType
                              ? formatText(text.combination.targetCount, { count: card.count })
                              : text.invalidTemplate}
                            onSelect={() => setCombinationTemplateId(`${SAVED_PREFIX}${card.id}`)}
                          />
                        )
                      })}
                    </div>
                  ) : !loadingCombinationTemplates ? (
                    <div className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">
                      {text.combination.noSavedTemplates}
                    </div>
                  ) : null}
                </div>

                <div className="flex flex-wrap items-center gap-2 text-xs">
                  <Badge variant="secondary">
                    {text.combination.targetType}: {text.targetTypes[combinationInputType || 'none']}
                  </Badge>
                  <span>{formatText(text.combination.matched, { count: combinationTargets.targets.length })}</span>
                  {combinationTargets.skipped.length ? (
                    <span className="text-muted-foreground">
                      {formatText(text.combination.skipped, { count: combinationTargets.skipped.length })}
                    </span>
                  ) : null}
                </div>

                {skippedSummary.length ? (
                  <div className="flex flex-wrap gap-1.5">
                    {skippedSummary.map((item) => (
                      <Badge key={item.reason} variant="outline" className="text-[11px] font-normal">
                        {text.skip[item.reason] || item.reason}: {item.count}
                      </Badge>
                    ))}
                  </div>
                ) : null}

                {combinationTargets.targets.length ? (
                  <div className="max-h-40 overflow-y-auto rounded-md border bg-muted/20 p-2 font-mono text-xs">
                    {combinationTargets.targets.slice(0, 100).map((target) => (
                      <div key={target} className="truncate" title={target}>{target}</div>
                    ))}
                    {combinationTargets.targets.length > 100 ? (
                      <div className="pt-1 text-muted-foreground">+{combinationTargets.targets.length - 100}</div>
                    ) : null}
                  </div>
                ) : (
                  <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
                    {combinationInputType ? text.combination.noTargets : text.invalidTemplate}
                  </div>
                )}

              </TabsContent>

              <TabsContent value="ai" className="mt-4 space-y-4">
                <div className="grid gap-2 sm:grid-cols-2" role="group" aria-label={text.ai.title}>
                  <button
                    type="button"
                    aria-pressed={aiExecution === 'orchestration'}
                    onClick={() => setAIExecution('orchestration')}
                    className={cn(
                      'flex flex-col gap-1 rounded-lg border p-3 text-left transition-colors',
                      aiExecution === 'orchestration' ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50',
                    )}
                  >
                    <span className="flex items-center gap-2 text-sm font-medium"><Workflow className="size-4" />{text.ai.orchestration}</span>
                    <span className="text-xs text-muted-foreground">{text.ai.orchestrationHint}</span>
                  </button>
                  <button
                    type="button"
                    aria-pressed={aiExecution === 'batch'}
                    onClick={() => setAIExecution('batch')}
                    className={cn(
                      'flex flex-col gap-1 rounded-lg border p-3 text-left transition-colors',
                      aiExecution === 'batch' ? 'border-primary bg-primary/5' : 'border-border hover:border-primary/50',
                    )}
                  >
                    <span className="flex items-center gap-2 text-sm font-medium"><Bot className="size-4" />{text.ai.batch}</span>
                    <span className="text-xs text-muted-foreground">{text.ai.batchHint}</span>
                  </button>
                </div>

                <div className="grid gap-3 sm:grid-cols-2">
                  {aiExecution === 'batch' ? (
                    <div className="space-y-1.5">
                      <Label>{text.ai.template}</Label>
                      <Select
                        value={selectedAITemplateId || BUILT_IN_AI_TEMPLATE}
                        onValueChange={(value) => setSelectedAITemplateId(value === BUILT_IN_AI_TEMPLATE ? '' : value)}
                        disabled={loadingAITemplates}
                      >
                        <SelectTrigger>
                          <SelectValue placeholder={loadingAITemplates ? text.combination.loadingTemplates : text.ai.builtInTemplate} />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={BUILT_IN_AI_TEMPLATE}>{text.ai.builtInTemplate}</SelectItem>
                          {aiTemplates.map((template) => (
                            <SelectItem key={template.id} value={template.id}>{template.name}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ) : (
                    <div className="space-y-1.5 sm:col-span-2">
                      <Label htmlFor="asset-scan-plan-objective">{text.ai.objective}</Label>
                      <Textarea
                        id="asset-scan-plan-objective"
                        value={planObjective}
                        placeholder={text.ai.objectivePlaceholder}
                        onChange={(event) => setPlanObjective(event.target.value)}
                        className="min-h-20 resize-y"
                      />
                    </div>
                  )}
                  <div className="space-y-1.5">
                    <Label>{text.ai.model}</Label>
                    <Select value={activeModel} onValueChange={setSelectedModel} disabled={!usableModelOptions.length}>
                      <SelectTrigger>
                        <SelectValue placeholder={text.ai.model} />
                      </SelectTrigger>
                      <SelectContent>
                        {usableModelOptions.map((option) => (
                          <SelectItem key={`${option.providerId}::${option.model}`} value={`${option.providerId}::${option.model}`}>
                            {option.providerName} · {option.model}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="asset-scan-ai-concurrency">{aiExecution === 'orchestration' ? text.ai.maxWorkers : text.ai.concurrency}</Label>
                    <NumberInput
                      id="asset-scan-ai-concurrency"
                      min={1}
                      max={8}
                      step={1}
                      value={aiExecution === 'orchestration' ? planWorkers : concurrency}
                      onChange={(event) =>
                        (aiExecution === 'orchestration' ? setPlanWorkers : setConcurrency)(event.target.value)
                      }
                    />
                  </div>
                  {aiExecution === 'batch' ? (
                    <label className="flex items-end gap-2 pb-2 text-sm">
                      <Checkbox checked={executeNow} onCheckedChange={(value) => setExecuteNow(value === true)} />
                      {text.ai.executeNow}
                    </label>
                  ) : null}
                </div>

                {!usableModelOptions.length ? (
                  <Alert>
                    <AlertDescription>
                      {text.ai.noModels}{' '}
                      <Link href="/settings" className="underline underline-offset-4">{text.ai.openSettings}</Link>
                    </AlertDescription>
                  </Alert>
                ) : null}

                <div className="rounded-md border bg-muted/20 p-3 text-sm">
                  <p className="font-medium">
                    {aiExecution === 'orchestration'
                      ? formatText(text.ai.planTargetCount, { count: aiTargets.length })
                      : formatText(text.ai.targetCount, { count: aiTargets.length })}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {aiExecution === 'orchestration' ? text.ai.orchestrationHint : text.ai.tokenHint}
                  </p>
                </div>

                {aiTargets.length ? (
                  <div className="max-h-40 overflow-y-auto rounded-md border bg-muted/20 p-2 font-mono text-xs">
                    {aiTargets.slice(0, 100).map((target) => (
                      <div key={target.target} className="truncate" title={target.target}>{target.target}</div>
                    ))}
                    {aiTargets.length > 100 ? (
                      <div className="pt-1 text-muted-foreground">+{aiTargets.length - 100}</div>
                    ) : null}
                  </div>
                ) : (
                  <div className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">
                    {text.ai.noTargets}
                  </div>
                )}
              </TabsContent>
            </Tabs>

            {error ? (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            ) : null}
          </>
        )}
        </div>

        {!created ? (
          <SheetFooter className="shrink-0 flex-row justify-end gap-2 border-t px-6 py-4">
            <Button variant="outline" onClick={() => onOpenChange(false)} disabled={creating}>
              {t.common.cancel}
            </Button>
            {mode === 'combination' ? (
              <Button
                variant="outline"
                onClick={handleAdvanced}
                disabled={creating || taskNameInvalid || !combinationTargets.targets.length || !combinationInputType}
              >
                {text.combination.advanced}
              </Button>
            ) : null}
            <Button onClick={() => void handleCreate()} disabled={createDisabled}>
              {creating ? <Spinner className="size-4" /> : null}
              {mode === 'combination'
                ? text.combination.create
                : aiExecution === 'orchestration'
                  ? text.ai.createPlan
                  : text.ai.create}
            </Button>
          </SheetFooter>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
