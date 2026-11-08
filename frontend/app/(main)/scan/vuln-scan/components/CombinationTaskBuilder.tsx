import { useId, useMemo, useRef, useState } from "react"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { cn } from "@/lib/utils"
import { Server, Globe, FolderSearch, FileCode, Target, Workflow, Trash2, AlertTriangle, GripVertical, Plus, ArrowDownWideNarrow, Captions } from "lucide-react"
import { useI18n } from "@/lib/i18n"
import { validateSubdomainTargets } from "@/lib/subdomain-targets"
import type { ScheduleConfig } from "../types"

export type ModuleId = "subdomain" | "portscan" | "webscan" | "dirscan" | "jsscan" | "crack"

interface PipelineNode {
  id: string
  moduleId: ModuleId
  stageIndex?: number
  laneIndex?: number
  config?: Record<string, any>
}

const createModuleCatalog = (t: any): Record<ModuleId, {
  title: string
  description: string
  icon: typeof Server
  tone: string
  badge?: string
  requires?: ModuleId[]
}> => ({
  subdomain: {
    title: t.vulnScan.taskManagement.subdomain,
    description: t.vulnScan.combination.modules.subdomainDesc,
    icon: Captions,
    tone: "text-tone-green",
  },
  portscan: {
    title: t.vulnScan.taskManagement.portscan,
    description: t.vulnScan.combination.modules.portscanDesc,
    icon: Server,
    tone: "text-emerald-500",
  },
  webscan: {
    title: t.vulnScan.taskManagement.webscan,
    description: t.vulnScan.combination.modules.webscanDesc,
    icon: Globe,
    tone: "text-blue-500",
  },
  dirscan: {
    title: t.vulnScan.taskManagement.dirscan,
    description: t.vulnScan.combination.modules.dirscanDesc,
    icon: FolderSearch,
    tone: "text-violet-500",
  },
  jsscan: {
    title: t.vulnScan.taskManagement.jsscan,
    description: t.vulnScan.combination.modules.jsscanDesc,
    icon: FileCode,
    tone: "text-amber-500",
  },
  crack: {
    title: t.vulnScan.taskManagement.crack,
    description: t.vulnScan.combination.modules.crackDesc,
    icon: Target,
    tone: "text-rose-500",
  },
})

const formatText = (template: string, vars: Record<string, string | number>) =>
  Object.entries(vars).reduce(
    (acc, [key, value]) => acc.replaceAll(`{${key}}`, String(value)),
    template
  )

export interface CombinationDraft {
  name: string
  nodes: PipelineNode[]
  targets: string[]
  scanMode?: "auto" | "local" | "agent"
  agentId?: string
  schedule?: ScheduleConfig
  proxy?: string
}

export type CombinationTargetInputType = "domain" | "network" | "web" | "service"

export const parseCombinationTargets = (value: string) => {
  return Array.from(
    new Set(
      value
        .split(/[\r\n,;]+/)
        .map((entry) => entry.trim())
        .filter(Boolean),
    ),
  )
}

const crackTargetSchemes = new Set([
  "ssh",
  "rdp",
  "ftp",
  "telnet",
  "smb",
  "mysql",
  "mssql",
  "postgresql",
  "mongodb",
  "redis",
  "oracle",
  "pop3",
  "ldap",
  "vnc",
  "mqtt",
  "memcached",
  "jdwp",
  "adb",
  "java-rmi",
  "activemq",
  "rsync",
  "kafka",
  "clickhouse",
])

const hostnameLabelPattern = /^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$/
const ipv4SegmentPattern = /^(25[0-5]|2[0-4]\d|1?\d?\d)$/
const ipv4Pattern = /^(\d{1,3}\.){3}\d{1,3}$/

const isIPv4 = (value: string) =>
  ipv4Pattern.test(value) && value.split(".").every((segment) => ipv4SegmentPattern.test(segment))

const isIPv6 = (value: string) =>
  value.includes(":") && /^[0-9a-fA-F:]+$/.test(value)

const isCIDR = (value: string) => {
  const [host, rawMask] = value.split("/", 2)
  const mask = Number(rawMask)
  if (!Number.isInteger(mask)) return false
  if (isIPv4(host)) return mask >= 0 && mask <= 32
  if (isIPv6(host)) return mask >= 0 && mask <= 128
  return false
}

const isHostname = (value: string) => {
  if (!value || value.includes("://") || value.includes("/") || value.includes("?") || value.includes("#")) {
    return false
  }
  if (value.includes(":")) {
    return false
  }
  return value
    .split(".")
    .every((label) => hostnameLabelPattern.test(label))
}

const isHttpUrl = (value: string) => {
  try {
    const parsed = new URL(value)
    return (parsed.protocol === "http:" || parsed.protocol === "https:") && Boolean(parsed.hostname)
  } catch {
    return false
  }
}

const isSupportedCrackUrl = (value: string) => {
  try {
    const parsed = new URL(value)
    const scheme = parsed.protocol.replace(/:$/, "").toLowerCase()
    return crackTargetSchemes.has(scheme) && Boolean(parsed.hostname) && Boolean(parsed.port)
  } catch {
    return false
  }
}

const getModuleInputType = (moduleId: string): CombinationTargetInputType | null => {
  switch (moduleId) {
    case "subdomain":
      return "domain"
    case "portscan":
      return "network"
    case "crack":
      return "service"
    case "webscan":
    case "dirscan":
    case "jsscan":
      return "web"
    default:
      return null
  }
}

export const resolveCombinationTargetInputType = (moduleIds: readonly string[]) => {
  const inputTypes = Array.from(
    new Set(moduleIds.map((moduleId) => getModuleInputType(moduleId)).filter(Boolean)),
  ) as CombinationTargetInputType[]

  if (inputTypes.length === 0) {
    return { inputType: null, hasConflict: false }
  }
  if (inputTypes.length > 1) {
    return { inputType: null, hasConflict: true }
  }
  return { inputType: inputTypes[0], hasConflict: false }
}

export const validateCombinationTargetsForInputType = (
  targets: readonly string[],
  inputType: CombinationTargetInputType | null,
) => {
  if (inputType === "domain") return validateSubdomainTargets(targets)
  if (!inputType) {
    return {
      validTargets: [...targets],
      invalidTargets: [] as string[],
    }
  }

  const validTargets: string[] = []
  const invalidTargets: string[] = []

  for (const rawTarget of targets) {
    const target = rawTarget.trim()
    if (!target) continue

    let isValid = false
    switch (inputType) {
      case "network":
        isValid = isIPv4(target) || isIPv6(target) || isCIDR(target) || isHostname(target)
        break
      case "web":
        isValid = isHttpUrl(target)
        break
      case "service":
        isValid = isSupportedCrackUrl(target)
        break
    }

    if (isValid) {
      validTargets.push(target)
    } else {
      invalidTargets.push(target)
    }
  }

  return { validTargets, invalidTargets }
}

interface CombinationTaskBuilderProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreate: (draft: CombinationDraft) => void
  isSubmitting?: boolean
}

export function CombinationTaskBuilder({ open, onOpenChange, onCreate, isSubmitting = false }: CombinationTaskBuilderProps) {
  const { t, language } = useI18n()
  const nodeIdPrefix = useId()
  const nextNodeIdRef = useRef(0)
  const defaultTaskName = `${t.vulnScan.combination.defaultTaskNamePrefix}-${new Date().toLocaleDateString(language === "zh" ? "zh-CN" : "en-US")}`
  const moduleCatalog = useMemo(() => createModuleCatalog(t), [t])

  const [taskName, setTaskName] = useState(() => defaultTaskName)
  const [pipeline, setPipeline] = useState<PipelineNode[]>([])
  const [targetInput, setTargetInput] = useState("")
  const [draggingIndex, setDraggingIndex] = useState<number | null>(null)

  const parsedTargets = useMemo(() => parseCombinationTargets(targetInput), [targetInput])
  const usedModuleIds = useMemo(() => new Set(pipeline.map(node => node.moduleId)), [pipeline])
  const paletteModules = useMemo(
    () => Object.entries(moduleCatalog).filter(([moduleId]) => !usedModuleIds.has(moduleId as ModuleId)),
    [usedModuleIds, moduleCatalog],
  )

  const pipelineWithMeta = useMemo(() => (
    pipeline.map(node => ({
      ...node,
      meta: moduleCatalog[node.moduleId],
    }))
  ), [pipeline, moduleCatalog])

  const validationErrors = useMemo(() => {
    const errors: string[] = []
    pipeline.forEach((node, idx) => {
      const deps = moduleCatalog[node.moduleId].requires || []
      deps.forEach(dep => {
        const depIndex = pipeline.findIndex(n => n.moduleId === dep)
        if (depIndex === -1) {
          errors.push(formatText(t.vulnScan.combination.builder.errors.needAddFirst, {
            module: moduleCatalog[node.moduleId].title,
            dependency: moduleCatalog[dep].title,
          }))
        } else if (depIndex > idx) {
          errors.push(formatText(t.vulnScan.combination.builder.errors.mustAfter, {
            module: moduleCatalog[node.moduleId].title,
            dependency: moduleCatalog[dep].title,
          }))
        }
      })
    })

    const subdomainIndex = pipeline.findIndex((node) => node.moduleId === "subdomain")
    const portscanIndex = pipeline.findIndex((node) => node.moduleId === "portscan")
    if (portscanIndex !== -1) {
      if (subdomainIndex !== -1) {
        if (subdomainIndex >= portscanIndex) {
          errors.push(t.vulnScan.combination.inline.errors.subdomainBeforePortscan)
        }
      } else if (portscanIndex !== 0) {
        errors.push(t.vulnScan.combination.inline.errors.portscanFirstStage)
      }
    }

    const firstInputType = pipeline[0] ? getModuleInputType(pipeline[0].moduleId) : null
    const { invalidTargets } = validateCombinationTargetsForInputType(parsedTargets, firstInputType)
    if (invalidTargets.length > 0 && firstInputType) {
      errors.push(formatText(t.vulnScan.combination.inline.errors.targetTypeMismatch, {
        count: invalidTargets.length,
        type: t.vulnScan.combination.inline.targetTypes[firstInputType],
      }))
    }

    return Array.from(new Set(errors))
  }, [parsedTargets, pipeline, moduleCatalog, t])

  const canSave = pipeline.length > 0 && validationErrors.length === 0 && parsedTargets.length > 0 && !isSubmitting

  const handleAddModule = (moduleId: ModuleId, index: number = pipeline.length) => {
    const nodeSequence = nextNodeIdRef.current
    nextNodeIdRef.current += 1
    const newNode: PipelineNode = {
      id: `${moduleId}-${nodeIdPrefix}-${nodeSequence}`,
      moduleId,
    }
    setPipeline(prev => {
      if (prev.some(node => node.moduleId === moduleId)) {
        return prev
      }
      const next = [...prev]
      next.splice(index, 0, newNode)
      return next
    })
  }

  const handleMoveNode = (from: number, to: number) => {
    setPipeline(prev => {
      const next = [...prev]
      const [removed] = next.splice(from, 1)
      next.splice(to, 0, removed)
      return next
    })
  }

  const handleDrop = (event: React.DragEvent, index: number) => {
    event.preventDefault()
    const origin = event.dataTransfer.getData("origin")
    if (origin === "palette") {
      const moduleId = event.dataTransfer.getData("moduleId") as ModuleId
      handleAddModule(moduleId, index)
    } else if (origin === "node" && draggingIndex !== null) {
      handleMoveNode(draggingIndex, index)
      setDraggingIndex(null)
    }
  }

  const handleDragStartPalette = (event: React.DragEvent, moduleId: ModuleId) => {
    event.dataTransfer.effectAllowed = "copy"
    event.dataTransfer.setData("origin", "palette")
    event.dataTransfer.setData("moduleId", moduleId)
  }

  const handleDragStartNode = (event: React.DragEvent, index: number) => {
    event.dataTransfer.effectAllowed = "move"
    event.dataTransfer.setData("origin", "node")
    setDraggingIndex(index)
  }

  const handleSave = () => {
    if (!canSave) return
    const firstInputType = pipeline[0] ? getModuleInputType(pipeline[0].moduleId) : null
    const { validTargets } = validateCombinationTargetsForInputType(parsedTargets, firstInputType)
    onCreate({
      name: taskName.trim() || t.vulnScan.combination.defaultTaskNamePrefix,
      nodes: pipeline,
      targets: validTargets,
    })
  }

  const handleReset = () => {
    setPipeline([])
    setTaskName(defaultTaskName)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-5xl h-[85vh] flex flex-col gap-4">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Workflow className="w-5 h-5 text-primary" />
            {t.vulnScan.combination.builder.dialogTitle}
          </DialogTitle>
          <DialogDescription>{t.vulnScan.combination.builder.dialogDesc}</DialogDescription>
        </DialogHeader>

        <div className="flex-1 grid grid-cols-12 gap-4 overflow-hidden">
          <Card className="col-span-4 flex flex-col">
            <CardHeader>
              <CardTitle className="text-base">{t.vulnScan.combination.builder.moduleResources}</CardTitle>
              <CardDescription>{t.vulnScan.combination.builder.moduleResourcesDesc}</CardDescription>
            </CardHeader>
            <ScrollArea className="flex-1 px-4 pb-4">
              <div className="grid gap-3">
                {paletteModules.length === 0 && (
                  <Card className="border-dashed">
                    <CardHeader className="py-3">
                      <CardTitle className="text-sm text-muted-foreground">{t.vulnScan.combination.builder.allModulesAdded}</CardTitle>
                      <CardDescription className="text-xs mt-1">{t.vulnScan.combination.builder.removeNodeToAdd}</CardDescription>
                    </CardHeader>
                  </Card>
                )}
                {paletteModules.map(([moduleId, meta]) => (
                  <Card
                    key={moduleId}
                    draggable
                    onDragStart={(e) => handleDragStartPalette(e, moduleId as ModuleId)}
                    className="border-dashed hover:border-primary/60 transition-colors cursor-grab active:cursor-grabbing"
                  >
                    <CardHeader className="py-3">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <meta.icon className={cn("h-4 w-4 shrink-0", meta.tone)} />
                          <CardTitle className="text-sm">{meta.title}</CardTitle>
                        </div>
                        <Button
                          variant="ghost"
                          size="sm"
                          className="text-xs h-7 px-2"
                          onClick={() => handleAddModule(moduleId as ModuleId)}
                        >
                          <Plus className="w-3 h-3 mr-1" />
                          {t.vulnScan.combination.builder.add}
                        </Button>
                      </div>
                      <CardDescription className="text-xs mt-2">{meta.description}</CardDescription>
                      {meta.requires && (
                        <p className="text-[11px] text-muted-foreground mt-1">
                          {t.vulnScan.combination.builder.requiresPrefix}{meta.requires.map(req => moduleCatalog[req].title).join(" / ")}
                        </p>
                      )}
                    </CardHeader>
                  </Card>
                ))}
              </div>
            </ScrollArea>
          </Card>

          <Card className="col-span-8 flex flex-col relative overflow-hidden">
            <CardHeader className="flex-row items-center justify-between gap-3">
              <div className="space-y-1">
                <CardTitle className="text-base">{t.vulnScan.combination.builder.canvasTitle}</CardTitle>
                <CardDescription>{t.vulnScan.combination.builder.canvasDesc}</CardDescription>
              </div>
              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={handleReset}>
                  <Trash2 className="w-4 h-4 mr-1" />
                  {t.vulnScan.combination.builder.clear}
                </Button>
                <Button variant="outline" size="sm" onClick={() => setPipeline(prev => [...prev].reverse())}>
                  <ArrowDownWideNarrow className="w-4 h-4 mr-1" />
                  {t.vulnScan.combination.builder.reverse}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="flex-1 overflow-auto">
              <div
                className={cn(
                  "min-h-[320px] rounded-lg border border-dashed px-6 py-6",
                  pipeline.length === 0 && "flex items-center justify-center text-sm text-muted-foreground"
                )}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => handleDrop(e, pipeline.length)}
              >
                {pipeline.length === 0 ? (
                  t.vulnScan.combination.builder.dragHint
                ) : (
                  <div className="space-y-6">
                    {pipelineWithMeta.map((node, index) => (
                      <div key={node.id} className="relative">
                        {index < pipeline.length - 1 && (
                          <div className="absolute left-[22px] top-12 bottom-0 w-px bg-border pointer-events-none" />
                        )}
                        <div
                          className="pl-12"
                          onDragOver={(e) => e.preventDefault()}
                          onDrop={(e) => handleDrop(e, index)}
                        >
                          <div
                            draggable
                            onDragStart={(e) => handleDragStartNode(e, index)}
                            className="relative flex items-center gap-4 rounded-xl border bg-background shadow-sm p-4 cursor-grab active:cursor-grabbing"
                          >
                            <div className="absolute left-3 top-1/2 -translate-y-1/2 h-8 w-8 rounded-full border flex items-center justify-center bg-background">
                              <GripVertical className="w-4 h-4 text-muted-foreground" />
                            </div>
                            <node.meta.icon className={cn("h-5 w-5 shrink-0", node.meta.tone)} />
                            <div className="flex-1">
                              <div className="flex items-center gap-2">
                                <p className="font-medium text-sm">{node.meta.title}</p>
                                <Badge variant="outline" className="text-[11px]">
                                  {formatText(t.vulnScan.combination.builder.stepLabel, { n: index + 1 })}
                                </Badge>
                              </div>
                              <p className="text-xs text-muted-foreground mt-1 line-clamp-2">{node.meta.description}</p>
                            </div>
                            <div className="flex items-center gap-2">
                              <Button variant="ghost" size="icon" onClick={() => handleMoveNode(index, Math.max(0, index - 1))} disabled={index === 0}>
                                ↑
                              </Button>
                              <Button variant="ghost" size="icon" onClick={() => handleMoveNode(index, Math.min(pipeline.length - 1, index + 1))} disabled={index === pipeline.length - 1}>
                                ↓
                              </Button>
                              <Button variant="ghost" size="icon" onClick={() => setPipeline(prev => prev.filter((_, i) => i !== index))}>
                                <Trash2 className="w-4 h-4" />
                              </Button>
                            </div>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </div>

        <Separator />

        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <div className="lg:col-span-3 space-y-2">
            <p className="text-sm font-medium">{t.vulnScan.target}</p>
            <Textarea
              value={targetInput}
              onChange={(e) => setTargetInput(e.target.value)}
              placeholder={t.vulnScan.combination.builder.targetPlaceholder}
              className="min-h-[120px] font-mono text-sm"
            />
            <p className="text-xs text-muted-foreground">
              {formatText(t.vulnScan.combination.builder.targetCountSummary, { count: parsedTargets.length })}
            </p>
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium">{t.vulnScan.taskName}</p>
            <Input value={taskName} onChange={(e) => setTaskName(e.target.value)} placeholder={t.vulnScan.combination.builder.taskNamePlaceholder} />
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium">{t.vulnScan.combination.builder.flowSummary}</p>
            <div className="min-h-[56px] rounded-lg border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
              {pipeline.length === 0 ? t.vulnScan.combination.builder.noSteps : pipeline.map((node, idx) => (
                <span key={node.id}>
                  {idx > 0 && " → "}
                  {moduleCatalog[node.moduleId].title}
                </span>
              ))}
            </div>
          </div>

          <div className="space-y-2">
            <p className="text-sm font-medium">{t.vulnScan.combination.builder.validation}</p>
            <div className={cn(
              "min-h-[56px] rounded-lg border px-3 py-2 text-sm",
              validationErrors.length ? "border-destructive bg-destructive/5 text-destructive" : "border-emerald-500/40 bg-emerald-500/5 text-emerald-600"
            )}>
              {validationErrors.length === 0 ? t.vulnScan.combination.builder.validationOk : (
                <ul className="list-disc pl-4 space-y-1">
                  {validationErrors.map(err => <li key={err}>{err}</li>)}
                </ul>
              )}
            </div>
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-4">
          {validationErrors.length > 0 && (
            <div className="flex items-center gap-2 text-sm text-destructive">
              <AlertTriangle className="w-4 h-4" />
              {t.vulnScan.combination.builder.fixBeforeSave}
            </div>
          )}
          <div className="flex-1" />
          <Button variant="ghost" onClick={() => onOpenChange(false)}>{t.vulnScan.newScan.cancel}</Button>
          <Button onClick={handleSave} disabled={!canSave}>
            {isSubmitting ? t.vulnScan.combination.builder.saving : t.vulnScan.combination.builder.saveTask}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
