import { DEFAULT_TCP_HEURISTIC_CONFIG, normalizeTCPHeuristicConfig, validateTCPHeuristicConfig, type TCPHeuristicConfig } from "@/lib/tcp-heuristic"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import type { ChangeEvent } from "react"
import { Button } from "@/components/ui/button"
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu"
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { cn } from "@/lib/utils"
import { AlertTriangle, ArrowLeft, GripVertical, Layers, Pencil, Plus, Save, Captions, Trash2, Workflow, Server, Globe, FolderSearch, FileCode, Target } from "lucide-react"
import { fetchWithAuth } from "@/lib/api"
import toast from "@/lib/toast"
import { useI18n } from "@/lib/i18n"
import { PortScanSettings } from "../new/components/PortScanSettings"
import { WebScanSettings } from "../new/components/WebScanSettings"
import { CrackScanSettings } from "../new/components/CrackScanSettings"
import { DirScanSettings, type DictionaryMeta } from "../new/components/DirScanSettings"
import { JsScanSettings, normalizeSQLRules, type SQLRule, type LFIRule, type XSSRule } from "../new/components/JsScanSettings"
import {
  parseCombinationTargets,
  resolveCombinationTargetInputType,
  validateCombinationTargetsForInputType,
  type CombinationDraft,
  type CombinationTargetInputType,
  type ModuleId,
} from "./CombinationTaskBuilder"
import { builtInPortGroupOptions } from "../utils/portGroup"
import { buildCombinationPresetCards } from "./combinationPresetUi"
import { isDesktopRuntime } from "@/lib/desktop"
import { getSpaceEngineConfig, type SpaceEngineConfig } from "@/lib/config"
import type { SYNCapabilityStatus, TCPDiscoveryMode } from "../types"
import { SubdomainConfigPanel } from "./SubdomainConfigPanel"
import { cloneDefaultDNSServers, getDefaultSubdomainEngines, normalizeSubdomainConfig, type SubdomainConfigValue } from "./subdomain-config"
import { TaskScheduleSettings } from "../new/components/TaskScheduleSettings"
import { createDefaultSchedule } from "../utils/task-schedule"

type SubdomainNodeConfig = SubdomainConfigValue

type PortScanNodeConfig = TCPHeuristicConfig & {
  threads: number
  portGroup: string
  portRange: string
  portTimeout: number
  aliveDetection: boolean
  tcpDiscoveryMode: TCPDiscoveryMode
  synRate: number
}

type WebScanNodeConfig = {
  threads: number
  nucleiThreads: number
  webscanOption: 1 | 2 | 3 | 4
  customHeaders: string
  customTags: string[]
  customTemplate: string[]
  rootPathScan: boolean
  skipNucleiWithoutTags: boolean
  skipNucleiWithWAF: boolean
  screenshot: boolean
  activeTimeoutLimit: number
  disableMaxHostErr: boolean
  oobEnabled: boolean
  interactshURL: string
  interactshToken: string
}

type CrackScanNodeConfig = {
  threads: number
  username: string
  password: string
  defaultDict: boolean
}

type DirAdvancedConfig = {
  thread: number
  timeout: number
  interval: number
  recursion: number
  times: number
  exclude: string
  headers: string
  redirectClient: boolean
  backupfileScan: boolean
}

type DirScanNodeConfig = {
  method: 'GET' | 'POST' | 'HEAD' | 'OPTIONS'
  extensions: string
  statusFilter: string
  customDict: string
  selectedDicts: string[]
  advancedConfig: DirAdvancedConfig
}

type JsScanNodeConfig = {
  blackDomain: string[]
  highRiskRouter: string[]
  authentication: string[]
  placeholder: Record<string, string>
  sqlInjectionEnabled: boolean
  sqlInjectionRules: SQLRule[]
  lfiEnabled: boolean
  lfiParamKeywords: string[]
  lfiRules: LFIRule[]
  ssrfEnabled: boolean
  ssrfParamKeywords: string[]
  redirectEnabled: boolean
  redirectParamKeywords: string[]
  xssEnabled: boolean
  xssRules: XSSRule[]
  uploadEnabled: boolean
  uploadParamKeywords: string[]
  uploadTestContent: string
  uploadTestFileName: string
  aiDetectionEnabled: boolean
}

type ModuleConfigMap = {
  subdomain: SubdomainNodeConfig
  portscan: PortScanNodeConfig
  webscan: WebScanNodeConfig
  dirscan: DirScanNodeConfig
  jsscan: JsScanNodeConfig
  crack: CrackScanNodeConfig
}

interface PipelineNode<M extends ModuleId = ModuleId> {
  id: string
  moduleId: M
  config: ModuleConfigMap[M]
}

interface PipelineStage {
  id: string
  title: string
  nodes: PipelineNode[]
}

interface PipelinePreset {
  id: string
  title: string
  description: string
  icon: typeof Server
  tone: string
  stages: Array<{
    title: string
    modules: ModuleId[]
  }>
}

const createModuleCatalog = (t: any, webscanTitle: string): Record<ModuleId, {
  title: string
  description: string
  tone: string
  icon: typeof Server
  requires?: ModuleId[]
}> => ({
  subdomain: {
    title: t.vulnScan.taskManagement.subdomain,
    description: t.vulnScan.combination.modules.subdomainDesc,
    tone: "text-tone-green",
    icon: Captions,
  },
  portscan: {
    title: t.vulnScan.taskManagement.portscan,
    description: t.vulnScan.combination.modules.portscanDesc,
    tone: "text-emerald-400",
    icon: Server,
  },
  webscan: {
    title: webscanTitle,
    description: t.vulnScan.combination.modules.webscanDesc,
    tone: "text-blue-400",
    icon: Globe,
  },
  dirscan: {
    title: t.vulnScan.taskManagement.dirscan,
    description: t.vulnScan.combination.modules.dirscanDesc,
    tone: "text-violet-400",
    icon: FolderSearch,
  },
  jsscan: {
    title: t.vulnScan.taskManagement.jsscan,
    description: t.vulnScan.combination.modules.jsscanDesc,
    tone: "text-amber-400",
    icon: FileCode,
  },
  crack: {
    title: t.vulnScan.taskManagement.crack,
    description: t.vulnScan.combination.modules.crackDesc,
    tone: "text-rose-400",
    icon: Target,
  },
})

const formatText = (template: string, vars: Record<string, string | number>) =>
  Object.entries(vars).reduce(
    (acc, [key, value]) => acc.replaceAll(`{${key}}`, String(value)),
    template
  )

const createId = (prefix: string) => `${prefix}-${Math.random().toString(36).slice(2, 8)}-${Date.now().toString(36)}`
const createStage = (title: string): PipelineStage => ({ id: createId("stage"), title, nodes: [] })

const getInputTypeLabel = (t: any, inputType: CombinationTargetInputType) => {
  switch (inputType) {
    case "network":
      return t.vulnScan.combination.inline.targetTypes.network
    case "domain":
      return t.vulnScan.combination.inline.targetTypes.domain
    case "web":
      return t.vulnScan.combination.inline.targetTypes.web
    case "service":
      return t.vulnScan.combination.inline.targetTypes.service
    default:
      return ""
  }
}

interface CombinationTaskInlineProps {
  onCreate: (draft: CombinationDraft) => void
  isSubmitting?: boolean
  onCancel?: () => void
  initialDraft?: CombinationDraft | null
}

interface NodeRef {
  stageId: string
  nodeId: string
}

type RunMode = "auto" | "local" | "agent"

interface AgentNodeOption {
  scanWorker?: boolean
  localWorker?: boolean
  agentId: string
  displayName: string
  enabled: boolean
  isAlive: boolean
  canDispatch: boolean
}

interface SavedCombinationTemplate {
  id: string
  name: string
  description?: string
  taskType: string
  config?: CombinationDraft
}

const defaultCombinationPortGroup =
  builtInPortGroupOptions.find((option) => option.text === "top1000") ?? {
    text: "top1000",
    value: "7,9,13,21,22,23,25,26,37,53,79,80,81,88,106,110,111,113,119,135,139,143,144,179,199,389,427,443,445,465,513,514,515,543,544,548,554,587,631,646,873,888,990,993,995,1025,1026,1027,1028,1029,1080,1110,1433,1720,1723,1755,1900,2000,2049,2121,3306,3389,5000,5432,5900,6000,6001,8000,8008,8009,8080,8081,8443,8888,9200,9999,10000,11211,27017",
  }

const normalizeTCPDiscoveryMode = (value: unknown): TCPDiscoveryMode => {
  const mode = String(value || "").trim().toLowerCase().replaceAll("_", "-")
  if (["connect", "tcp", "tcp-connect", "full-connect"].includes(mode)) {
    return "connect"
  }
  return "syn"
}

const normalizePortScanNodeConfig = (config: any, fallback: PortScanNodeConfig): PortScanNodeConfig => {
  const next = config ?? {}
  const synRate = Number(next.synRate ?? next.portDiscoveryRate ?? next.discoveryRate ?? fallback.synRate)
  return {
    ...fallback,
    ...next,
    ...normalizeTCPHeuristicConfig(next),
    portTimeout: Number(next.portTimeout ?? next.timeout ?? fallback.portTimeout) || fallback.portTimeout,
    tcpDiscoveryMode: normalizeTCPDiscoveryMode(next.tcpDiscoveryMode ?? next.portDiscoveryMode ?? fallback.tcpDiscoveryMode),
    synRate: Number.isFinite(synRate) && synRate > 0 ? synRate : fallback.synRate,
  }
}

const normalizeJsScanNodeConfig = (config: any, fallback: JsScanNodeConfig): JsScanNodeConfig => ({
  ...fallback,
  ...(config ?? {}),
  sqlInjectionRules: normalizeSQLRules(config?.sqlInjectionRules),
})

export function CombinationTaskInline({
  onCreate,
  isSubmitting = false,
  onCancel,
  initialDraft = null,
}: CombinationTaskInlineProps) {
  const { t, language } = useI18n()
  const moduleCatalog = useMemo(
    () => createModuleCatalog(t, language === "zh" ? "网站扫描" : "Web Scan"),
    [t, language]
  )
  const defaultTaskName = `${t.vulnScan.combination.defaultTaskNamePrefix}-${new Date().toLocaleDateString(language === "zh" ? "zh-CN" : "en-US")}`
  const [taskName, setTaskName] = useState(() => defaultTaskName)
  const [taskNameEditing, setTaskNameEditing] = useState(false)
  const [taskNameDraft, setTaskNameDraft] = useState(() => defaultTaskName)
  const [runMode, setRunMode] = useState<RunMode>("auto")
  const [taskSchedule, setTaskSchedule] = useState(() => createDefaultSchedule())
  const [agentNodes, setAgentNodes] = useState<AgentNodeOption[]>([])
  const [selectedAgentId, setSelectedAgentId] = useState("")
  const [loadingAgents, setLoadingAgents] = useState(false)
  const [desktopRuntime, setDesktopRuntime] = useState(false)
  const [synCapability, setSynCapability] = useState<SYNCapabilityStatus | null>(null)
  const [loadingSynCapability, setLoadingSynCapability] = useState(false)
  const [authorizingSyn, setAuthorizingSyn] = useState(false)
  const [stages, setStages] = useState<PipelineStage[]>([
    createStage(formatText(t.vulnScan.combination.inline.stageName, { n: 1 })),
  ])
  const [targetInput, setTargetInput] = useState("")
  const [selectedNode, setSelectedNode] = useState<NodeRef | null>(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [activePresetId, setActivePresetId] = useState("")
  const initialDraftAppliedRef = useRef(false)
  const [activeConfigTab, setActiveConfigTab] = useState<"flow" | "execution" | "trigger">("flow")
  const [dirAvailableDicts, setDirAvailableDicts] = useState<DictionaryMeta[]>([])
  const [dirDictLoading, setDirDictLoading] = useState(false)
  const [jsDefaultConfig, setJsDefaultConfig] = useState<JsScanNodeConfig | null>(null)
  const [spaceEngineConfig, setSpaceEngineConfig] = useState<SpaceEngineConfig | null>(null)
  const [draggingNode, setDraggingNode] = useState<NodeRef | null>(null)
  const [dragOverStageId, setDragOverStageId] = useState<string | null>(null)
  const [dragOverNodeId, setDragOverNodeId] = useState<string | null>(null)
  const [savedTemplates, setSavedTemplates] = useState<SavedCombinationTemplate[]>([])
  const [loadingSavedTemplates, setLoadingSavedTemplates] = useState(false)
  const [selectedSavedTemplateId, setSelectedSavedTemplateId] = useState("")
  const [saveTemplateDialogOpen, setSaveTemplateDialogOpen] = useState(false)
  const [templateName, setTemplateName] = useState("")
  const [templateDescription, setTemplateDescription] = useState("")
  const [savingTemplate, setSavingTemplate] = useState(false)

  const availableAgents = useMemo(
    () => agentNodes.filter((item) => item.scanWorker !== false && item.enabled && item.isAlive && item.canDispatch),
    [agentNodes]
  )

  const hasPortScanNode = useMemo(
    () => stages.some((stage) => stage.nodes.some((node) => node.moduleId === "portscan")),
    [stages]
  )

  const loadSYNCapabilityStatus = useCallback(async () => {
    setLoadingSynCapability(true)
    try {
      const response = await fetchWithAuth("/api/v1/system/syn/status", { cache: "no-store" })
      const data = await response.json().catch(() => null)
      if (!response.ok || !data?.success) {
        throw new Error(data?.error || t.vulnScan.newScan.synScan.statusLoadFailed)
      }
      setSynCapability(data.data || null)
    } catch (error) {
      console.error("Load SYN capability status failed:", error)
      setSynCapability(null)
    } finally {
      setLoadingSynCapability(false)
    }
  }, [t])

  const handleAuthorizeSYN = useCallback(async () => {
    setAuthorizingSyn(true)
    try {
      const response = await fetchWithAuth("/api/v1/system/syn/authorize", {
        method: "POST",
        cache: "no-store",
      })
      const data = await response.json().catch(() => null)
      if (data?.data) {
        setSynCapability(data.data)
      }
      if (!response.ok || !data?.success) {
        throw new Error(data?.error || t.vulnScan.newScan.synScan.authorizationUnavailable)
      }
      toast.success(t.vulnScan.newScan.synScan.authorizationSuccess)
      await loadSYNCapabilityStatus()
    } catch (error: any) {
      console.error("Authorize SYN scan failed:", error)
      toast.error(error.message || t.vulnScan.newScan.synScan.authorizationFailed)
    } finally {
      setAuthorizingSyn(false)
    }
  }, [loadSYNCapabilityStatus, t])

  const reloadDirDicts = useCallback(async () => {
    setDirDictLoading(true)
    try {
      const response = await fetchWithAuth(`/api/v1/dirsearch/dicts`)
      if (!response.ok) {
        throw new Error(t.vulnScan.combination.inline.messages.loadDictFailed)
      }
      const data = await response.json()
      if (data.success && data.dicts) {
        const dicts: DictionaryMeta[] = data.dicts.map((dict: any, index: number) => ({
          id: dict.name || `dict-${index}`,
          name: dict.name || "unknown",
          path: dict.path || "",
          description: dict.description || dict.name || t.vulnScan.combination.inline.messages.noDescription,
          size: dict.size || 0,
          entries: dict.entries,
        }))
        setDirAvailableDicts(dicts)
      }
    } catch (error: any) {
      console.error(t.vulnScan.combination.inline.messages.loadDictFailed, error)
      toast.error(`${t.vulnScan.combination.inline.messages.loadDictFailed}: ${error.message || t.vulnScan.messages.unknownError}`)
    } finally {
      setDirDictLoading(false)
    }
  }, [t])

  const loadAgents = useCallback(async () => {
    setLoadingAgents(true)
    try {
      const response = await fetchWithAuth("/api/v1/webscan/agents")
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || "加载 Agent 节点失败")
      }
      setAgentNodes(Array.isArray(data.data) ? data.data : [])
    } catch (error: any) {
      toast.error(error.message || "加载 Agent 节点失败")
    } finally {
      setLoadingAgents(false)
    }
  }, [])

  const loadSavedTemplates = useCallback(async () => {
    setLoadingSavedTemplates(true)
    try {
      const response = await fetchWithAuth("/api/v1/config/scan-templates?taskType=combination")
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || t.vulnScan.combination.inline.messages.loadCombinationTemplatesFailed)
      }
      const templates = Array.isArray(data.data) ? data.data : []
      setSavedTemplates(
        templates.map((item: any) => ({
          id: String(item.id || ""),
          name: String(item.name || ""),
          description: typeof item.description === "string" ? item.description : "",
          taskType: String(item.taskType || ""),
          config: item.config,
        }))
      )
    } catch (error: any) {
      toast.error(error.message || t.vulnScan.combination.inline.messages.loadCombinationTemplatesFailed)
      setSavedTemplates([])
    } finally {
      setLoadingSavedTemplates(false)
    }
  }, [t])

  const handleDirDictionaryUpload = useCallback(
    async (event: ChangeEvent<HTMLInputElement>) => {
      const files = event.target.files
      if (!files?.length) return

      for (const file of Array.from(files)) {
        if (!file.name.toLowerCase().endsWith(".txt")) {
          toast.error(formatText(t.vulnScan.combination.inline.messages.invalidTxtFile, { name: file.name }))
          continue
        }
        try {
          const formData = new FormData()
          formData.append("file", file)

          const response = await fetchWithAuth("/api/v1/dirsearch/dicts/upload", {
            method: "POST",
            body: formData,
          })
          if (!response.ok) {
            const errorData = await response.json().catch(() => ({}))
            throw new Error(errorData.error || `${t.vulnScan.combination.inline.messages.uploadFailed}: ${response.status}`)
          }
          const data = await response.json()
          if (!data.success) {
            throw new Error(data.error || t.vulnScan.combination.inline.messages.uploadFailed)
          }
          toast.success(formatText(t.vulnScan.combination.inline.messages.uploadedToServer, { name: file.name }))
        } catch (err: any) {
          console.error(t.vulnScan.combination.inline.messages.uploadDictFailed, err)
          toast.error(`${file.name}: ${err.message}`)
        }
      }
      await reloadDirDicts()
    },
    [reloadDirDicts, t]
  )

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void reloadDirDicts()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [reloadDirDicts])

  useEffect(() => {
    let ignore = false

    const detectRuntime = async () => {
      const desktop = await isDesktopRuntime()
      if (ignore) return
      setDesktopRuntime(desktop)
      if (desktop) {
        setRunMode("auto")
        setSelectedAgentId("")
        return
      }
      await loadAgents()
    }

    void detectRuntime()
    return () => {
      ignore = true
    }
  }, [loadAgents])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadSavedTemplates()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadSavedTemplates])

  useEffect(() => {
    let ignore = false
    getSpaceEngineConfig()
      .then((config) => {
        if (!ignore) setSpaceEngineConfig(config)
      })
      .catch((error) => {
        console.error("Load space engine config failed:", error)
      })
    return () => {
      ignore = true
    }
  }, [])

  useEffect(() => {
    if (!selectedSavedTemplateId) return
    const timer = window.setTimeout(() => {
      if (!savedTemplates.some((item) => item.id === selectedSavedTemplateId)) {
        setSelectedSavedTemplateId("")
      }
    }, 0)

    return () => window.clearTimeout(timer)
  }, [savedTemplates, selectedSavedTemplateId])

  useEffect(() => {
    if (!selectedAgentId) {
      return
    }
    const timer = window.setTimeout(() => {
      if (!availableAgents.some((item) => item.agentId === selectedAgentId)) {
        setSelectedAgentId("")
      }
    }, 0)

    return () => window.clearTimeout(timer)
  }, [availableAgents, selectedAgentId])

  useEffect(() => {
    if (!desktopRuntime) {
      return
    }
    const timer = window.setTimeout(() => {
      if (runMode === "agent") {
        setRunMode("auto")
      }
      if (selectedAgentId) {
        setSelectedAgentId("")
      }
    }, 0)

    return () => window.clearTimeout(timer)
  }, [desktopRuntime, runMode, selectedAgentId])

  useEffect(() => {
    if (!desktopRuntime || activeConfigTab !== "execution") return

    const timer = window.setTimeout(() => {
      setActiveConfigTab("flow")
    }, 0)

    return () => window.clearTimeout(timer)
  }, [activeConfigTab, desktopRuntime])

  useEffect(() => {
    if (!hasPortScanNode) {
      return
    }
    const timer = window.setTimeout(() => {
      void loadSYNCapabilityStatus()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [hasPortScanNode, loadSYNCapabilityStatus])

  useEffect(() => {
    const loadDefault = async () => {
      try {
        const response = await fetchWithAuth("/api/v1/jsscan/default-config")
        if (!response.ok) return
        const data = await response.json()
        if (data.success && data.data) {
          const config = data.data
          const normalized: JsScanNodeConfig = {
            blackDomain: config.blackDomain ?? [],
            highRiskRouter: config.highRiskRouter ?? [],
            authentication: config.authentication ?? [],
            placeholder: config.placeholder ?? {},
            sqlInjectionEnabled: config.vulnDetection?.enabled !== false,
            sqlInjectionRules: normalizeSQLRules(config.vulnDetection?.sqlInjection?.rules),
            lfiEnabled: config.vulnDetection?.lfi?.enabled !== false,
            lfiParamKeywords: config.vulnDetection?.lfi?.paramKeywords ?? [],
            lfiRules: config.vulnDetection?.lfi?.rules ?? [],
            ssrfEnabled: config.vulnDetection?.ssrf?.enabled !== false,
            ssrfParamKeywords: config.vulnDetection?.ssrf?.paramKeywords ?? [],
            redirectEnabled: config.vulnDetection?.redirect?.enabled !== false,
            redirectParamKeywords: config.vulnDetection?.redirect?.paramKeywords ?? [],
            xssEnabled: config.vulnDetection?.xss?.enabled !== false,
            xssRules: config.vulnDetection?.xss?.rules ?? [],
            uploadEnabled: config.vulnDetection?.upload?.enabled !== false,
            uploadParamKeywords: config.vulnDetection?.upload?.paramKeywords ?? [],
            uploadTestContent: config.vulnDetection?.upload?.testContent ?? "<h1>uploadtest</h1>",
            uploadTestFileName: config.vulnDetection?.upload?.testFileName ?? "test.html",
            aiDetectionEnabled: config.openai?.enabled ?? false,
          }
          setJsDefaultConfig(normalized)
        }
      } catch (error) {
        console.error(t.vulnScan.combination.inline.messages.loadJsConfigFailed, error)
      }
    }
    const timer = window.setTimeout(() => {
      void loadDefault()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [t.vulnScan.combination.inline.messages.loadJsConfigFailed])

  useEffect(() => {
    if (dirAvailableDicts.length === 0) return
    const timer = window.setTimeout(() => {
      setStages((prev) =>
        prev.map((stage) => ({
          ...stage,
          nodes: stage.nodes.map((node) => {
            if (node.moduleId !== "dirscan") return node
            const dirConfig = node.config as DirScanNodeConfig
            if (dirConfig.selectedDicts.length > 0) return node
            return {
              ...node,
              config: {
                ...dirConfig,
                selectedDicts: [dirAvailableDicts[0].id],
              },
            }
          }),
        }))
      )
    }, 0)

    return () => window.clearTimeout(timer)
  }, [dirAvailableDicts])

  const buildDefaultConfig = useCallback(
    (moduleId: ModuleId): ModuleConfigMap[ModuleId] => {
      switch (moduleId) {
        case "subdomain":
          return {
            thread: 600,
            timeout: 5,
            appendEngines: getDefaultSubdomainEngines(spaceEngineConfig),
            dnsServers: cloneDefaultDNSServers(),
          } as ModuleConfigMap[ModuleId]
        case "portscan":
          return {
            threads: 1000,
            portGroup: defaultCombinationPortGroup.text,
            portRange: defaultCombinationPortGroup.value,
            portTimeout: 10,
            aliveDetection: true,
            ...DEFAULT_TCP_HEURISTIC_CONFIG,
            tcpDiscoveryMode: "syn",
            synRate: 3000,
          } as ModuleConfigMap[ModuleId]
        case "webscan":
          return {
            threads: 30,
            nucleiThreads: 2,
            webscanOption: 3,
            customHeaders: "",
            customTags: [],
            customTemplate: [],
            rootPathScan: true,
            skipNucleiWithoutTags: false,
            skipNucleiWithWAF: true,
            screenshot: true,
            activeTimeoutLimit: 5,
            disableMaxHostErr: false,
            oobEnabled: true,
            interactshURL: "",
            interactshToken: "",
          } as ModuleConfigMap[ModuleId]
        case "dirscan":
          return {
            method: "GET",
            extensions: "php,aspx,asp,jsp,html,js",
            statusFilter: "404",
            customDict: "",
            selectedDicts: dirAvailableDicts.length > 0 ? [dirAvailableDicts[0].id] : [],
            advancedConfig: {
              thread: 25,
              timeout: 8,
              interval: 0,
              recursion: 0,
              times: 5,
              exclude: "",
              headers: "",
              redirectClient: false,
              backupfileScan: false,
            },
          } as ModuleConfigMap[ModuleId]
        case "jsscan": {
          const fallback: JsScanNodeConfig = {
            blackDomain: [],
            highRiskRouter: [],
            authentication: [],
            placeholder: {},
            sqlInjectionEnabled: true,
            sqlInjectionRules: [],
            lfiEnabled: true,
            lfiParamKeywords: [],
            lfiRules: [],
            ssrfEnabled: true,
            ssrfParamKeywords: [],
            redirectEnabled: true,
            redirectParamKeywords: [],
            xssEnabled: true,
            xssRules: [],
            uploadEnabled: true,
            uploadParamKeywords: [],
            uploadTestContent: "<h1>uploadtest</h1>",
            uploadTestFileName: "test.html",
            aiDetectionEnabled: false,
          }
          return { ...(jsDefaultConfig ?? fallback) } as ModuleConfigMap[ModuleId]
        }
        case "crack":
          return {
            threads: 10,
            username: "",
            password: "",
            defaultDict: true,
          } as ModuleConfigMap[ModuleId]
        default:
          return {
            threads: 30,
            nucleiThreads: 2,
            webscanOption: 3,
            customHeaders: "",
            customTags: [],
            customTemplate: [],
            rootPathScan: true,
            skipNucleiWithoutTags: false,
            skipNucleiWithWAF: true,
            screenshot: true,
            activeTimeoutLimit: 5,
            disableMaxHostErr: false,
            oobEnabled: true,
            interactshURL: "",
            interactshToken: "",
          } as ModuleConfigMap[ModuleId]
      }
    },
    [dirAvailableDicts, jsDefaultConfig, spaceEngineConfig]
  )

  const presets = useMemo<PipelinePreset[]>(
    () => buildCombinationPresetCards(t, formatText) as PipelinePreset[],
    [t]
  )

  const applyDraftToEditor = useCallback((draft: CombinationDraft, options?: { preserveTargets?: boolean }) => {
    const preserveTargets = options?.preserveTargets === true
    const nextName = draft.name?.trim() || defaultTaskName
    setTaskName(nextName)
    setTaskNameDraft(nextName)
    if (!preserveTargets) {
      setTargetInput(Array.isArray(draft.targets) ? draft.targets.join("\n") : "")
    }
    setRunMode(desktopRuntime && draft.scanMode === "agent" ? "auto" : (draft.scanMode ?? "auto"))
    setSelectedAgentId(desktopRuntime ? "" : (draft.agentId ?? ""))

    setTaskSchedule(draft.schedule?.spec ? draft.schedule : createDefaultSchedule())

    const grouped = new Map<number, PipelineNode[]>()
    for (const rawNode of draft.nodes || []) {
      const moduleId = rawNode.moduleId as ModuleId
      const stageIndex = Math.max(0, rawNode.stageIndex ?? 0)
      const stageNodes = grouped.get(stageIndex) ?? []
      const fallbackConfig = buildDefaultConfig(moduleId)
      stageNodes.push({
        id: rawNode.id || createId(moduleId),
        moduleId,
        config: moduleId === "jsscan"
          ? normalizeJsScanNodeConfig(rawNode.config, fallbackConfig as JsScanNodeConfig)
          : moduleId === "portscan"
            ? normalizePortScanNodeConfig(rawNode.config, fallbackConfig as PortScanNodeConfig)
          : moduleId === "subdomain"
            ? normalizeSubdomainConfig(rawNode.config)
          : ((rawNode.config as any) ?? fallbackConfig),
      })
      grouped.set(stageIndex, stageNodes)
    }

    const nextStages = Array.from(grouped.entries())
      .sort((a, b) => a[0] - b[0])
      .map(([stageIndex, nodes]) => ({
        id: createId("stage"),
        title: formatText(t.vulnScan.combination.inline.stageName, { n: stageIndex + 1 }),
        nodes,
      }))

    setStages(
      nextStages.length > 0
        ? nextStages
        : [createStage(formatText(t.vulnScan.combination.inline.stageName, { n: 1 }))]
    )
    setSelectedNode(null)
    setDrawerOpen(false)
    setActivePresetId("")
  }, [buildDefaultConfig, defaultTaskName, desktopRuntime, t])

  const patchSelectedConfig = useCallback(
    (updater: (prev: any) => any) => {
      if (!selectedNode) return
      setStages((prev) =>
        prev.map((stage) => {
          if (stage.id !== selectedNode.stageId) return stage
          return {
            ...stage,
            nodes: stage.nodes.map((node) =>
              node.id === selectedNode.nodeId ? { ...node, config: updater(node.config) } : node
            ),
          }
        })
      )
    },
    [selectedNode]
  )

  const parsedTargets = useMemo(() => parseCombinationTargets(targetInput), [targetInput])

  useEffect(() => {
    if (!initialDraft || initialDraftAppliedRef.current) return
    initialDraftAppliedRef.current = true
    const timer = window.setTimeout(() => {
      applyDraftToEditor(initialDraft)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [applyDraftToEditor, initialDraft])

  const usedModuleIds = useMemo(() => {
    const set = new Set<ModuleId>()
    stages.forEach((stage) => stage.nodes.forEach((node) => set.add(node.moduleId)))
    return set
  }, [stages])

  const availableModules = useMemo(
    () => Object.entries(moduleCatalog).filter(([moduleId]) => !usedModuleIds.has(moduleId as ModuleId)),
    [usedModuleIds, moduleCatalog]
  )

  const isFirstStage = useCallback(
    (stageId: string) => stages.length > 0 && stages[0].id === stageId,
    [stages]
  )

  const canAddModuleToStage = useCallback(
    (stage: PipelineStage, moduleId: ModuleId) => {
      if (usedModuleIds.has(moduleId)) return false

      const stageIndex = stages.findIndex((item) => item.id === stage.id)
      const portScanStageIndex = stages.findIndex((item) => item.nodes.some((node) => node.moduleId === "portscan"))
      const subdomainStageIndex = stages.findIndex((item) => item.nodes.some((node) => node.moduleId === "subdomain"))
      const hasPortScanInStage = stage.nodes.some((n) => n.moduleId === "portscan")
      if (moduleId === "portscan") {
        if (stage.nodes.length > 0) return false
        if (subdomainStageIndex !== -1) {
          return stageIndex > subdomainStageIndex
        }
        return isFirstStage(stage.id)
      }

      if (moduleId === "subdomain" && portScanStageIndex !== -1 && stageIndex >= portScanStageIndex) {
        return false
      }

      // 端口扫描阶段不允许并行其他节点
      if (hasPortScanInStage) return false

      return true
    },
    [isFirstStage, stages, usedModuleIds]
  )

  const orderedNodes = useMemo(() => {
    const list: { node: PipelineNode; stageIndex: number }[] = []
    stages.forEach((stage, stageIndex) => {
      stage.nodes.forEach((node) => list.push({ node, stageIndex }))
    })
    return list
  }, [stages])

  const firstStageNodes = useMemo(
    () => stages.find((stage) => stage.nodes.length > 0)?.nodes ?? [],
    [stages]
  )

  const targetInputRule = useMemo(
    () => resolveCombinationTargetInputType(firstStageNodes.map((node) => node.moduleId)),
    [firstStageNodes]
  )

  const targetValidation = useMemo(
    () => validateCombinationTargetsForInputType(parsedTargets, targetInputRule.inputType),
    [parsedTargets, targetInputRule.inputType]
  )

  const targetInputTypeLabel = useMemo(
    () => (targetInputRule.inputType ? getInputTypeLabel(t, targetInputRule.inputType) : ""),
    [t, targetInputRule.inputType]
  )

  const targetConfigDescription = useMemo(() => {
    if (targetInputRule.hasConflict) {
      return t.vulnScan.combination.inline.errors.firstStageMixedInput
    }
    switch (targetInputRule.inputType) {
      case "domain":
        return t.vulnScan.combination.inline.targetConfigDescDomain
      case "network":
        return t.vulnScan.combination.inline.targetConfigDescNetwork
      case "web":
        return t.vulnScan.combination.inline.targetConfigDescWeb
      case "service":
        return t.vulnScan.combination.inline.targetConfigDescService
      default:
        return t.vulnScan.combination.inline.targetConfigDesc
    }
  }, [t, targetInputRule.hasConflict, targetInputRule.inputType])

  const targetInputPlaceholder = useMemo(() => {
    switch (targetInputRule.inputType) {
      case "domain":
        return t.vulnScan.combination.inline.targetPlaceholderDomain
      case "network":
        return t.vulnScan.combination.inline.targetPlaceholderNetwork
      case "web":
        return t.vulnScan.combination.inline.targetPlaceholderWeb
      case "service":
        return t.vulnScan.combination.inline.targetPlaceholderService
      default:
        return t.vulnScan.combination.inline.targetPlaceholder
    }
  }, [t, targetInputRule.inputType])

  const validationErrors = useMemo(() => {
    const errors: string[] = []
    if (orderedNodes.length === 0) {
      errors.push(t.vulnScan.combination.inline.errors.atLeastOneModule)
    }

    const portScanStageIndex = stages.findIndex((stage) => stage.nodes.some((node) => node.moduleId === "portscan"))
    const subdomainStageIndex = stages.findIndex((stage) => stage.nodes.some((node) => node.moduleId === "subdomain"))
    if (portScanStageIndex !== -1) {
      if (subdomainStageIndex !== -1) {
        if (subdomainStageIndex >= portScanStageIndex) {
          errors.push(t.vulnScan.combination.inline.errors.subdomainBeforePortscan)
        }
      } else if (portScanStageIndex !== 0) {
        errors.push(t.vulnScan.combination.inline.errors.portscanFirstStage)
      }
      const stage = stages[portScanStageIndex]
      if (stage && stage.nodes.length > 1) {
        errors.push(t.vulnScan.combination.inline.errors.portscanExclusiveStage)
      }
    }

    if (targetInputRule.hasConflict) {
      errors.push(t.vulnScan.combination.inline.errors.firstStageMixedInput)
    }
    if (targetValidation.invalidTargets.length > 0 && targetInputRule.inputType) {
      errors.push(
        formatText(t.vulnScan.combination.inline.errors.targetTypeMismatch, {
          count: targetValidation.invalidTargets.length,
          type: getInputTypeLabel(t, targetInputRule.inputType),
        })
      )
    }

    const satisfied = new Set<ModuleId>()
    orderedNodes.forEach(({ node }) => {
      const deps = moduleCatalog[node.moduleId].requires || []
      deps.forEach((dep) => {
        if (!satisfied.has(dep)) {
          errors.push(formatText(t.vulnScan.combination.inline.errors.dependencyBefore, {
            module: moduleCatalog[node.moduleId].title,
            dependency: moduleCatalog[dep].title,
          }))
        }
      })
      satisfied.add(node.moduleId)
    })
    return Array.from(new Set(errors))
  }, [moduleCatalog, orderedNodes, stages, t, targetInputRule, targetValidation.invalidTargets.length])

  const canSave =
    parsedTargets.length > 0 &&
    orderedNodes.length > 0 &&
    validationErrors.length === 0 &&
    (runMode !== "agent" || availableAgents.length > 0) &&
    !isSubmitting

  const handleAddStage = () =>
    setStages((prev) => [...prev, createStage(formatText(t.vulnScan.combination.inline.stageName, { n: prev.length + 1 }))])
  const handleRemoveStage = (stageId: string) => {
    if (stages.length === 1) return
    setStages((prev) => prev.filter((stage) => stage.id !== stageId))
    if (selectedNode?.stageId === stageId) {
      setSelectedNode(null)
      setDrawerOpen(false)
    }
  }

  const handleAddNode = (stageId: string, moduleId: ModuleId) => {
    const stage = stages.find((s) => s.id === stageId)
    if (!stage) return
    if (!canAddModuleToStage(stage, moduleId)) {
      if (moduleId === "portscan") {
        toast.error(usedModuleIds.has("subdomain")
          ? t.vulnScan.combination.inline.errors.portscanAfterSubdomain
          : t.vulnScan.combination.inline.errors.portscanOnlyFirstStage)
      } else if (moduleId === "subdomain" && usedModuleIds.has("portscan")) {
        toast.error(t.vulnScan.combination.inline.errors.subdomainBeforePortscan)
      } else if (stage.nodes.some((n) => n.moduleId === "portscan")) {
        toast.error(t.vulnScan.combination.inline.errors.portscanNoParallelInStage)
      }
      return
    }
    setStages((prev) =>
      prev.map((stage) =>
        stage.id === stageId
          ? { ...stage, nodes: [...stage.nodes, { id: createId(moduleId), moduleId, config: buildDefaultConfig(moduleId) }] }
          : stage
      )
    )
  }

  const handleRemoveNode = (ref: NodeRef) => {
    setStages((prev) =>
      prev.map((stage) => {
        if (stage.id !== ref.stageId) return stage
        return { ...stage, nodes: stage.nodes.filter((node) => node.id !== ref.nodeId) }
      })
    )
    if (selectedNode?.nodeId === ref.nodeId) {
      setSelectedNode(null)
      setDrawerOpen(false)
    }
  }

  const resetDragState = useCallback(() => {
    setDraggingNode(null)
    setDragOverStageId(null)
    setDragOverNodeId(null)
  }, [])

  const handleMoveNode = useCallback(
    (source: NodeRef, targetStageId: string, targetNodeId?: string) => {
      let errorMessage = ""
      let nextSelection: NodeRef | null = null

      setStages((prev) => {
        const sourceStage = prev.find((stage) => stage.id === source.stageId)
        const sourceNode = sourceStage?.nodes.find((node) => node.id === source.nodeId)
        const targetStage = prev.find((stage) => stage.id === targetStageId)
        if (!sourceStage || !sourceNode || !targetStage) return prev

        const targetStageIndex = prev.findIndex((stage) => stage.id === targetStageId)
        const targetNodesWithoutDragged =
          source.stageId === targetStageId
            ? targetStage.nodes.filter((node) => node.id !== source.nodeId)
            : targetStage.nodes
        const portScanStageIndex = prev.findIndex((stage) => stage.nodes.some((node) => node.moduleId === "portscan"))
        const subdomainStageIndex = prev.findIndex((stage) => stage.nodes.some((node) => node.moduleId === "subdomain"))

        if (sourceNode.moduleId === "portscan") {
          if (subdomainStageIndex !== -1 && targetStageIndex <= subdomainStageIndex) {
            errorMessage = t.vulnScan.combination.inline.errors.portscanAfterSubdomain
            return prev
          }
          if (subdomainStageIndex === -1 && targetStageIndex !== 0) {
            errorMessage = t.vulnScan.combination.inline.errors.portscanOnlyFirstStage
            return prev
          }
          if (targetNodesWithoutDragged.length > 0) {
            errorMessage = t.vulnScan.combination.inline.errors.portscanExclusiveStage
            return prev
          }
        } else if (sourceNode.moduleId === "subdomain" && portScanStageIndex !== -1 && targetStageIndex >= portScanStageIndex) {
          errorMessage = t.vulnScan.combination.inline.errors.subdomainBeforePortscan
          return prev
        } else if (targetNodesWithoutDragged.some((node) => node.moduleId === "portscan")) {
          errorMessage = t.vulnScan.combination.inline.errors.portscanNoParallelInStage
          return prev
        }

        const insertIndex = targetNodeId
          ? Math.max(0, targetNodesWithoutDragged.findIndex((node) => node.id === targetNodeId))
          : targetNodesWithoutDragged.length

        const withoutSource = prev.map((stage) =>
          stage.id === source.stageId
            ? { ...stage, nodes: stage.nodes.filter((node) => node.id !== source.nodeId) }
            : stage
        )

        const nextStages = withoutSource.map((stage) => {
          if (stage.id !== targetStageId) return stage
          const nextNodes = [...stage.nodes]
          nextNodes.splice(insertIndex, 0, sourceNode)
          return { ...stage, nodes: nextNodes }
        })

        if (selectedNode?.nodeId === source.nodeId) {
          nextSelection = { stageId: targetStageId, nodeId: source.nodeId }
        }

        return nextStages
      })

      if (errorMessage) {
        toast.error(errorMessage)
        return
      }
      if (nextSelection) {
        setSelectedNode(nextSelection)
      }
    },
    [selectedNode, t]
  )

  const handleClearFlow = useCallback(() => {
    setStages([createStage(formatText(t.vulnScan.combination.inline.stageName, { n: 1 }))])
    setActivePresetId("")
    setSelectedNode(null)
    setDrawerOpen(false)
  }, [t])

  const handleApplyPreset = useCallback(
    (preset: PipelinePreset) => {
      const nextStages = preset.stages.map((stage) => ({
        id: createId("stage"),
        title: stage.title,
        nodes: stage.modules.map((moduleId) => ({
          id: createId(moduleId),
          moduleId,
          config: buildDefaultConfig(moduleId),
        })),
      }))
      setStages(nextStages)
      setActivePresetId(preset.id)
      setSelectedNode(null)
      setDrawerOpen(false)
      toast.success(formatText(t.vulnScan.combination.inline.messages.presetApplied, { name: preset.title }))
    },
    [buildDefaultConfig, t]
  )

  const commitTaskName = useCallback(() => {
    const nextName = taskNameDraft.trim()
    if (nextName && nextName !== taskName) {
      setTaskName(nextName)
      setTaskNameDraft(nextName)
    } else if (!nextName) {
      setTaskNameDraft(taskName)
    }
    setTaskNameEditing(false)
  }, [taskName, taskNameDraft])

  const selectedDetails = useMemo(() => {
    if (!selectedNode) return null
    const stage = stages.find((s) => s.id === selectedNode.stageId)
    const node = stage?.nodes.find((n) => n.id === selectedNode.nodeId)
    return stage && node ? { stage, node } : null
  }, [selectedNode, stages])

  const buildCurrentDraft = useCallback((targets: string[]): CombinationDraft => {
    const exportNodes = stages.flatMap((stage, stageIndex) =>
      stage.nodes.map((node) => {
        let exportedConfig: Record<string, any> = { ...node.config }

        if (node.moduleId === "subdomain") {
          const c = node.config as SubdomainNodeConfig
          exportedConfig = {
            thread: c.thread,
            timeout: c.timeout,
            appendEngines: c.appendEngines,
            dnsServers: c.dnsServers.filter((server) => server.enabled).map((server) => server.address),
          }
        } else if (node.moduleId === "portscan") {
          const c = node.config as PortScanNodeConfig
          exportedConfig = {
            threads: c.threads,
            timeout: c.portTimeout,
            portRange: c.portRange,
            aliveDetection: c.aliveDetection,
            ...normalizeTCPHeuristicConfig(c),
            tcpDiscoveryMode: c.tcpDiscoveryMode,
            portDiscoveryMode: c.tcpDiscoveryMode,
            synRate: c.tcpDiscoveryMode === "syn" ? c.synRate : undefined,
          }
        } else if (node.moduleId === "webscan") {
          const c = node.config as WebScanNodeConfig
          const shouldEnableOOB = c.webscanOption !== 1 && c.webscanOption !== 2 && c.oobEnabled
          exportedConfig = {
            threads: c.threads,
            nucleiThreads: c.nucleiThreads,
            webscanOption: c.webscanOption,
            customHeaders: c.customHeaders || undefined,
            customTags: c.customTags && c.customTags.length > 0 ? c.customTags : undefined,
            customTemplate: c.customTemplate && c.customTemplate.length > 0 ? c.customTemplate : undefined,
            rootPathScan: c.rootPathScan,
            skipNucleiWithoutTags: c.skipNucleiWithoutTags,
            skipNucleiWithWAF: c.skipNucleiWithWAF,
            screenshot: c.screenshot,
            activeTimeoutLimit: c.activeTimeoutLimit,
            oobEnabled: shouldEnableOOB,
            interactshURL: shouldEnableOOB ? c.interactshURL || undefined : undefined,
            interactshToken: shouldEnableOOB ? c.interactshToken || undefined : undefined,
          }
        } else if (node.moduleId === "dirscan") {
          const c = node.config as DirScanNodeConfig

          const customPaths = c.customDict
            .split(/\r?\n/)
            .map((line) => line.trim())
            .filter(Boolean)

          const parseStatusFilter = (input: string): number[] => {
            if (!input) return []
            return input
              .split(",")
              .map((block) => {
                const trimmed = block.trim()
                if (!trimmed) return []
                if (trimmed.includes("-")) {
                  const [start, end] = trimmed.split("-").map((v) => Number(v))
                  if (Number.isFinite(start) && Number.isFinite(end)) {
                    const result: number[] = []
                    for (let i = Math.min(start, end); i <= Math.max(start, end); i++) {
                      result.push(i)
                    }
                    return result
                  }
                  return []
                }
                const numeric = Number(trimmed)
                return Number.isFinite(numeric) ? [numeric] : []
              })
              .flat()
          }

          const excludedStatusList = parseStatusFilter(c.statusFilter)

          let dictPaths: string[] = []
          let paths: string[] = []
          if (customPaths.length > 0) {
            paths = customPaths
          } else if (c.selectedDicts.length > 0) {
            for (const dictId of c.selectedDicts) {
              const dict = dirAvailableDicts.find((d) => d.id === dictId)
              if (!dict) continue
              if (dict.path) dictPaths.push(dict.path)
              else if (dict.entries && dict.entries.length > 0) paths.push(...dict.entries)
            }
          }

          exportedConfig = {
            method: c.method as any,
            dictPaths: dictPaths.length > 0 ? dictPaths : undefined,
            paths: paths.length > 0 ? paths : undefined,
            exts: c.extensions.split(",").map((s) => s.trim()).filter(Boolean),
            workers: c.advancedConfig.thread,
            timeout: c.advancedConfig.timeout,
            bodyExclude: c.advancedConfig.exclude || undefined,
            bodyLengthExcludeTimes: c.advancedConfig.times || undefined,
            statusCodeExclude: excludedStatusList,
            redirect: c.advancedConfig.redirectClient,
            interval: c.advancedConfig.interval || undefined,
            customHeader: c.advancedConfig.headers || undefined,
            backupscan: c.advancedConfig.backupfileScan,
            recursive: c.advancedConfig.recursion || 0,
          }
        } else if (node.moduleId === "jsscan") {
          const c = node.config as JsScanNodeConfig
          exportedConfig = {
            blackDomain: c.blackDomain,
            highRiskRouter: c.highRiskRouter,
            authentication: c.authentication,
            placeholder: c.placeholder,
            vulnDetection: {
              enabled: c.sqlInjectionEnabled,
            },
            openai: { enabled: c.aiDetectionEnabled },
          }
        } else if (node.moduleId === "crack") {
          const c = node.config as CrackScanNodeConfig
          exportedConfig = {
            threads: c.threads,
            username: c.username,
            password: c.password,
            defaultDict: c.defaultDict,
          }
        }

        return {
          id: node.id,
          moduleId: node.moduleId,
          stageIndex,
          laneIndex: 0,
          config: exportedConfig,
        }
      })
    )

    return {
      name: taskName.trim() || t.vulnScan.combination.defaultTaskNamePrefix,
      targets,
      nodes: exportNodes,
      scanMode: desktopRuntime && runMode === "agent" ? "auto" : runMode,
      agentId: !desktopRuntime && runMode === "agent" && selectedAgentId ? selectedAgentId : undefined,
      schedule: taskSchedule,
    }
  }, [desktopRuntime, dirAvailableDicts, runMode, selectedAgentId, stages, t, taskName, taskSchedule])

  const buildCurrentTemplateDraft = useCallback((): CombinationDraft => ({
    name: taskName.trim() || defaultTaskName,
    targets: [],
    nodes: stages.flatMap((stage, stageIndex) =>
      stage.nodes.map((node) => ({
        id: node.id,
        moduleId: node.moduleId,
        stageIndex,
        laneIndex: 0,
          config: JSON.parse(JSON.stringify(
            node.moduleId === "portscan"
              ? normalizePortScanNodeConfig(node.config, buildDefaultConfig("portscan") as PortScanNodeConfig)
              : node.moduleId === "subdomain"
                ? {
                    thread: (node.config as SubdomainNodeConfig).thread,
                    timeout: (node.config as SubdomainNodeConfig).timeout,
                    appendEngines: (node.config as SubdomainNodeConfig).appendEngines,
                    dnsServers: (node.config as SubdomainNodeConfig).dnsServers
                      .filter((server) => server.enabled)
                      .map((server) => server.address),
                  }
                : node.config
          )),
      }))
    ),
    scanMode: desktopRuntime && runMode === "agent" ? "auto" : runMode,
    agentId: !desktopRuntime && runMode === "agent" && selectedAgentId ? selectedAgentId : undefined,
    schedule: taskSchedule,
  }), [buildDefaultConfig, defaultTaskName, desktopRuntime, runMode, selectedAgentId, stages, taskName, taskSchedule])

  const handleSave = () => {
    for (const node of stages.flatMap((stage) => stage.nodes)) {
      if (node.moduleId !== "portscan") continue
      const config = node.config as PortScanNodeConfig
      const error = validateTCPHeuristicConfig(normalizeTCPHeuristicConfig(config), config.portRange)
      if (error) {
        toast.error(t.vulnScan.newScan.tcpHeuristic[error])
        return
      }
    }

    if (parsedTargets.length === 0) {
      toast.error(t.vulnScan.combination.inline.errors.needTarget)
      return
    }
    if (orderedNodes.length === 0) {
      toast.error(t.vulnScan.combination.inline.errors.atLeastOneModule)
      return
    }
    if (validationErrors.length > 0) {
      toast.error(validationErrors[0])
      return
    }
    if (!canSave) return
    if (!desktopRuntime && runMode === "agent" && availableAgents.length === 0) {
      toast.error("当前没有可调度的 Agent 节点，请先启动并启用 Agent")
      return
    }
    if (!desktopRuntime && runMode === "agent" && !selectedAgentId) {
      toast.error(t.vulnScan.combination.inline.execution.selectAgent)
      return
    }
    const effectiveRunMode: RunMode = desktopRuntime && runMode === "agent" ? "auto" : runMode
    const hasUnauthedSYN = stages.some((stage) =>
      stage.nodes.some((node) => {
        if (node.moduleId !== "portscan") return false
        const config = node.config as PortScanNodeConfig
        return config.tcpDiscoveryMode === "syn"
      })
    )
    if (effectiveRunMode === "local" && hasUnauthedSYN && !synCapability?.authorized) {
      toast.error(t.vulnScan.newScan.synScan.strictRequiresAuthorization)
      return
    }
    onCreate(buildCurrentDraft(targetValidation.validTargets))
  }

  const handleSavedTemplateChange = (templateId: string) => {
    setSelectedSavedTemplateId(templateId)
    const template = savedTemplates.find((item) => item.id === templateId)
    if (!template?.config) {
      toast.error(t.vulnScan.combination.inline.messages.loadCombinationTemplatesFailed)
      return
    }
    applyDraftToEditor(template.config, { preserveTargets: true })
    toast.success(formatText(t.vulnScan.combination.inline.messages.combinationTemplateApplied, { name: template.name }))
  }

  const handleOpenSaveTemplateDialog = () => {
    if (orderedNodes.length === 0) {
      toast.error(t.vulnScan.combination.inline.messages.saveCombinationTemplateNeedModule)
      return
    }
    setTemplateName(taskName.trim() || defaultTaskName)
    setTemplateDescription("")
    setSaveTemplateDialogOpen(true)
  }

  const handleSaveCombinationTemplate = async () => {
    const name = templateName.trim()
    if (!name) {
      toast.error(t.vulnScan.combination.inline.messages.saveCombinationTemplateNeedName)
      return
    }

    try {
      setSavingTemplate(true)
      const response = await fetchWithAuth("/api/v1/config/scan-templates", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          description: templateDescription.trim(),
          taskType: "combination",
          config: {
            ...buildCurrentTemplateDraft(),
            name,
          },
        }),
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || t.vulnScan.combination.inline.messages.saveCombinationTemplateFailed)
      }
      setSaveTemplateDialogOpen(false)
      setSelectedSavedTemplateId(String(data?.data?.id || ""))
      await loadSavedTemplates()
      toast.success(formatText(t.vulnScan.combination.inline.messages.combinationTemplateSaved, { name }))
    } catch (error: any) {
      toast.error(error.message || t.vulnScan.combination.inline.messages.saveCombinationTemplateFailed)
    } finally {
      setSavingTemplate(false)
    }
  }

  const openNodeDrawer = (stageId: string, nodeId: string) => {
    setSelectedNode({ stageId, nodeId })
    setDrawerOpen(true)
  }

  const closeNodeDrawer = () => {
    setDrawerOpen(false)
    setSelectedNode(null)
  }

  const getNodeSummary = (node: PipelineNode) => {
    switch (node.moduleId) {
      case "subdomain": {
        const config = node.config as SubdomainNodeConfig
        return formatText(t.vulnScan.combination.inline.nodeSummary.subdomain, {
          thread: config.thread,
          engineCount: config.appendEngines.length,
        })
      }
      case "portscan": {
        const config = node.config as PortScanNodeConfig
        return formatText(t.vulnScan.combination.inline.nodeSummary.portscan, {
          threads: config.threads,
          portGroup: config.portGroup,
        })
      }
      case "webscan": {
        const config = node.config as WebScanNodeConfig
        return formatText(t.vulnScan.combination.inline.nodeSummary.webscan, {
          threads: config.threads,
          mode: config.webscanOption,
        })
      }
      case "dirscan": {
        const config = node.config as DirScanNodeConfig
        return formatText(t.vulnScan.combination.inline.nodeSummary.dirscan, {
          method: config.method,
          dictCount: config.selectedDicts.length || 0,
        })
      }
      case "jsscan": {
        const config = node.config as JsScanNodeConfig
        return formatText(t.vulnScan.combination.inline.nodeSummary.jsscan, {
          blackDomainCount: config.blackDomain.length,
          highRiskCount: config.highRiskRouter.length,
        })
      }
      case "crack": {
        const config = node.config as CrackScanNodeConfig
        return formatText(t.vulnScan.combination.inline.nodeSummary.crack, {
          threads: config.threads,
          dictType: config.defaultDict
            ? t.vulnScan.combination.inline.nodeSummary.systemDict
            : t.vulnScan.combination.inline.nodeSummary.customDict,
        })
      }
      default:
        return ""
    }
  }

  const renderNodeSettings = () => {
    if (!selectedDetails) {
      return <div className="text-sm text-muted-foreground">{t.vulnScan.combination.inline.emptyNodeSelection}</div>
    }
    const node = selectedDetails.node
    switch (node.moduleId) {
      case "subdomain": {
        const config = node.config as SubdomainNodeConfig
        return (
          <SubdomainConfigPanel
            value={config}
            onChange={(next) => patchSelectedConfig(() => next)}
            spaceEngineConfig={spaceEngineConfig}
            onConfigureApi={() => window.location.assign('/settings')}
          />
        )
      }
      case "portscan": {
        const config = normalizePortScanNodeConfig(node.config, buildDefaultConfig("portscan") as PortScanNodeConfig)
        const update = (producer: (prev: PortScanNodeConfig) => PortScanNodeConfig) =>
          patchSelectedConfig((prev) => producer(normalizePortScanNodeConfig(prev, config)))
        return (
          <PortScanSettings
            portGroup={config.portGroup}
            setPortGroup={(val) => update((prev) => ({ ...prev, portGroup: val }))}
            portRange={config.portRange}
            setPortRange={(val) => update((prev) => ({ ...prev, portRange: val }))}
            threads={config.threads}
            setThreads={(val) => update((prev) => ({ ...prev, threads: val }))}
            portTimeout={config.portTimeout}
            setPortTimeout={(val) => update((prev) => ({ ...prev, portTimeout: val }))}
            tcpHeuristic={normalizeTCPHeuristicConfig(config)}
            setTcpHeuristic={(value) => update((previous) => ({ ...previous, ...value }))}
            aliveDetection={config.aliveDetection}
            setAliveDetection={(val) => update((prev) => ({ ...prev, aliveDetection: val }))}
            tcpDiscoveryMode={config.tcpDiscoveryMode}
            setTcpDiscoveryMode={(val) => update((prev) => ({ ...prev, tcpDiscoveryMode: val }))}
            synRate={config.synRate}
            setSynRate={(val) => update((prev) => ({ ...prev, synRate: val }))}
            synCapability={synCapability}
            loadingSynCapability={loadingSynCapability}
            authorizingSyn={authorizingSyn}
            desktopRuntime={desktopRuntime}
            runMode={runMode}
            onRefreshSynCapability={loadSYNCapabilityStatus}
            onAuthorizeSYN={handleAuthorizeSYN}
            scanTemplates={[]}
            selectedTemplateId=""
            setSelectedTemplateId={() => {}}
            onApplyTemplate={() => {}}
            onClearTemplate={() => {}}
          />
        )
      }
      case "webscan": {
        const config = node.config as WebScanNodeConfig
        const update = (producer: (prev: WebScanNodeConfig) => WebScanNodeConfig) =>
          patchSelectedConfig((prev) => producer(prev as WebScanNodeConfig))
        return (
          <WebScanSettings
            webscanOption={config.webscanOption}
            setWebscanOption={(val) => update((prev) => ({ ...prev, webscanOption: val }))}
            customHeaders={config.customHeaders}
            setCustomHeaders={(val) => update((prev) => ({ ...prev, customHeaders: val }))}
            customTags={config.customTags}
            setCustomTags={(val) => update((prev) => ({ ...prev, customTags: val }))}
            customTemplate={config.customTemplate}
            setCustomTemplate={(val) => update((prev) => ({ ...prev, customTemplate: val }))}
            threads={config.threads}
            setThreads={(val) => update((prev) => ({ ...prev, threads: val }))}
            nucleiThreads={config.nucleiThreads}
            setNucleiThreads={(val) => update((prev) => ({ ...prev, nucleiThreads: val }))}
            activeTimeoutLimit={config.activeTimeoutLimit}
            setActiveTimeoutLimit={(val) => update((prev) => ({ ...prev, activeTimeoutLimit: val }))}
            disableMaxHostErr={config.disableMaxHostErr === true}
            setDisableMaxHostErr={(val) => update((prev) => ({ ...prev, disableMaxHostErr: val }))}
            rootPathScan={config.rootPathScan}
            setRootPathScan={(val) => update((prev) => ({ ...prev, rootPathScan: val }))}
            skipNucleiWithoutTags={config.skipNucleiWithoutTags}
            setSkipNucleiWithoutTags={(val) => update((prev) => ({ ...prev, skipNucleiWithoutTags: val }))}
            skipNucleiWithWAF={config.skipNucleiWithWAF}
            setSkipNucleiWithWAF={(val) => update((prev) => ({ ...prev, skipNucleiWithWAF: val }))}
            screenshot={config.screenshot}
            setScreenshot={(val) => update((prev) => ({ ...prev, screenshot: val }))}
            oobEnabled={config.oobEnabled}
            setOobEnabled={(val) => update((prev) => ({ ...prev, oobEnabled: val }))}
            interactshURL={config.interactshURL}
            setInteractshURL={(val) => update((prev) => ({ ...prev, interactshURL: val }))}
            interactshToken={config.interactshToken}
            setInteractshToken={(val) => update((prev) => ({ ...prev, interactshToken: val }))}
            scanTemplates={[]}
            selectedTemplateId=""
            setSelectedTemplateId={() => {}}
            onApplyTemplate={() => {}}
            onClearTemplate={() => {}}
            templateOptions={[]}
            fingerprintOptions={[]}
          />
        )
      }
      case "crack": {
        const config = node.config as CrackScanNodeConfig
        const update = (producer: (prev: CrackScanNodeConfig) => CrackScanNodeConfig) =>
          patchSelectedConfig((prev) => producer(prev as CrackScanNodeConfig))
        return (
          <CrackScanSettings
            username={config.username}
            setUsername={(val) => update((prev) => ({ ...prev, username: val }))}
            password={config.password}
            setPassword={(val) => update((prev) => ({ ...prev, password: val }))}
            defaultDict={config.defaultDict}
            setDefaultDict={(val) => update((prev) => ({ ...prev, defaultDict: val }))}
            isSubmitting={false}
            scanTemplates={[]}
            selectedTemplateId=""
            setSelectedTemplateId={() => {}}
            onApplyTemplate={() => {}}
            onClearTemplate={() => {}}
          />
        )
      }
      case "dirscan": {
        const config = node.config as DirScanNodeConfig
        const update = (producer: (prev: DirScanNodeConfig) => DirScanNodeConfig) =>
          patchSelectedConfig((prev) => producer(prev as DirScanNodeConfig))
        return (
          <DirScanSettings
            method={config.method}
            setMethod={(val) => update((prev) => ({ ...prev, method: val as any }))}
            extensions={config.extensions}
            setExtensions={(val) => update((prev) => ({ ...prev, extensions: val }))}
            statusFilter={config.statusFilter}
            setStatusFilter={(val) => update((prev) => ({ ...prev, statusFilter: val }))}
            customDict={config.customDict}
            setCustomDict={(val) => update((prev) => ({ ...prev, customDict: val }))}
            availableDicts={dirAvailableDicts}
            setAvailableDicts={setDirAvailableDicts}
            selectedDicts={config.selectedDicts}
            setSelectedDicts={(val) => update((prev) => ({ ...prev, selectedDicts: val }))}
            loadingDicts={dirDictLoading}
            advancedConfig={config.advancedConfig}
            setAdvancedConfig={(setter) =>
              update((prev) => ({
                ...prev,
                advancedConfig:
                  typeof setter === "function"
                    ? (setter as (value: DirAdvancedConfig) => DirAdvancedConfig)(prev.advancedConfig)
                    : setter,
              }))
            }
            defaultDictionaries={dirAvailableDicts}
            onDictionaryUpload={handleDirDictionaryUpload}
            onReloadDicts={reloadDirDicts}
            onToast={(message) => toast.success(message)}
          />
        )
      }
      case "jsscan": {
        const config = node.config as JsScanNodeConfig
        const update = (producer: (prev: JsScanNodeConfig) => JsScanNodeConfig) =>
          patchSelectedConfig((prev) => producer(prev as JsScanNodeConfig))
        return (
          <JsScanSettings
            blackDomain={config.blackDomain}
            setBlackDomain={(val) => update((prev) => ({ ...prev, blackDomain: val }))}
            highRiskRouter={config.highRiskRouter}
            setHighRiskRouter={(val) => update((prev) => ({ ...prev, highRiskRouter: val }))}
            authentication={config.authentication}
            setAuthentication={(val) => update((prev) => ({ ...prev, authentication: val }))}
            placeholder={config.placeholder}
            setPlaceholder={(val) => update((prev) => ({ ...prev, placeholder: val }))}
            sqlInjectionEnabled={config.sqlInjectionEnabled}
            setSqlInjectionEnabled={(val) => update((prev) => ({ ...prev, sqlInjectionEnabled: val }))}
            sqlInjectionRules={config.sqlInjectionRules}
            setSqlInjectionRules={(val) => update((prev) => ({ ...prev, sqlInjectionRules: val }))}
            lfiEnabled={config.lfiEnabled}
            setLfiEnabled={(val) => update((prev) => ({ ...prev, lfiEnabled: val }))}
            lfiParamKeywords={config.lfiParamKeywords}
            setLfiParamKeywords={(val) => update((prev) => ({ ...prev, lfiParamKeywords: val }))}
            lfiRules={config.lfiRules}
            setLfiRules={(val) => update((prev) => ({ ...prev, lfiRules: val }))}
            ssrfEnabled={config.ssrfEnabled}
            setSsrfEnabled={(val) => update((prev) => ({ ...prev, ssrfEnabled: val }))}
            ssrfParamKeywords={config.ssrfParamKeywords}
            setSsrfParamKeywords={(val) => update((prev) => ({ ...prev, ssrfParamKeywords: val }))}
            redirectEnabled={config.redirectEnabled}
            setRedirectEnabled={(val) => update((prev) => ({ ...prev, redirectEnabled: val }))}
            redirectParamKeywords={config.redirectParamKeywords}
            setRedirectParamKeywords={(val) => update((prev) => ({ ...prev, redirectParamKeywords: val }))}
            xssEnabled={config.xssEnabled}
            setXssEnabled={(val) => update((prev) => ({ ...prev, xssEnabled: val }))}
            xssRules={config.xssRules}
            setXssRules={(val) => update((prev) => ({ ...prev, xssRules: val }))}
            uploadEnabled={config.uploadEnabled}
            setUploadEnabled={(val) => update((prev) => ({ ...prev, uploadEnabled: val }))}
            uploadParamKeywords={config.uploadParamKeywords}
            setUploadParamKeywords={(val) => update((prev) => ({ ...prev, uploadParamKeywords: val }))}
            uploadTestContent={config.uploadTestContent}
            setUploadTestContent={(val) => update((prev) => ({ ...prev, uploadTestContent: val }))}
            uploadTestFileName={config.uploadTestFileName}
            setUploadTestFileName={(val) => update((prev) => ({ ...prev, uploadTestFileName: val }))}
            aiDetectionEnabled={config.aiDetectionEnabled}
            setAiDetectionEnabled={(val) => update((prev) => ({ ...prev, aiDetectionEnabled: val }))}
            scanTemplates={[]}
            selectedTemplateId=""
            setSelectedTemplateId={() => {}}
            onApplyTemplate={() => {}}
            onClearTemplate={() => {}}
          />
        )
      }
      default:
        return <div className="text-sm text-muted-foreground">{t.vulnScan.combination.inline.moduleConfigUnsupported}</div>
    }
  }

  return (
    <>
    <div className="min-w-0 space-y-5">
      <div className="-mx-3 -mt-3 md:-mx-6 md:-mt-6 border-b border-border/60 bg-background">
        <div className="relative flex flex-wrap items-center justify-between gap-3 px-3 py-3 md:h-[56px] md:flex-nowrap md:py-0">
          <div className="flex min-w-0 flex-1 items-center gap-2 md:flex-initial">
              <Button variant="ghost" onClick={onCancel} disabled={isSubmitting} size="icon" className="h-8 w-8 shrink-0 rounded-md">
                <ArrowLeft className="h-4 w-4" />
              </Button>
              <div className="min-w-0 flex-1 md:max-w-[280px] md:flex-initial">
                {taskNameEditing ? (
                  <div className="flex h-8 items-center">
                    <Input
                      value={taskNameDraft}
                      onChange={(e) => setTaskNameDraft(e.target.value)}
                      onBlur={commitTaskName}
                      onKeyDown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault()
                        commitTaskName()
                      }
                    }}
                    placeholder={t.vulnScan.combination.inline.taskNamePlaceholder}
                      className="h-8 min-w-0 w-full md:min-w-[200px] md:max-w-[320px] border-border/70 bg-background/70 text-base md:text-sm"
                      autoFocus
                    />
                  </div>
                ) : (
                  <div className="flex h-8 items-center gap-1">
                    <div className="truncate text-[15px] font-semibold tracking-tight">{taskName || defaultTaskName}</div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      className="h-7 w-7 shrink-0 rounded-md text-muted-foreground hover:text-foreground"
                      onClick={() => {
                        setTaskNameDraft(taskName)
                        setTaskNameEditing(true)
                      }}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                )}
              </div>
          </div>

          <div className="order-last flex w-full items-center gap-1 overflow-x-auto md:absolute md:left-1/2 md:top-1/2 md:order-none md:w-auto md:-translate-x-1/2 md:-translate-y-1/2">
              <button
                type="button"
                onClick={() => setActiveConfigTab("flow")}
                className={cn(
                  "flex-1 shrink-0 whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-colors md:flex-initial",
                  activeConfigTab === "flow"
                    ? "bg-primary/12 text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                {t.vulnScan.combination.inline.tabs.flow}
              </button>
              {!desktopRuntime && (
                <button
                  type="button"
                  onClick={() => setActiveConfigTab("execution")}
                  className={cn(
                    "flex-1 shrink-0 whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-colors md:flex-initial",
                    activeConfigTab === "execution"
                      ? "bg-primary/12 text-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  {t.vulnScan.combination.inline.tabs.execution}
                </button>
              )}
              <button
                type="button"
                onClick={() => setActiveConfigTab("trigger")}
                className={cn(
                  "flex-1 shrink-0 whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-colors md:flex-initial",
                  activeConfigTab === "trigger"
                    ? "bg-primary/12 text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                {t.vulnScan.combination.inline.tabs.trigger}
              </button>
          </div>

          <div className="flex items-center gap-2">
            <Button onClick={handleSave} disabled={!canSave} className="h-9 px-3 md:h-8 md:px-3.5">
              {t.vulnScan.combination.inline.actions.saveAndRun}
            </Button>
          </div>
        </div>
      </div>

      {activeConfigTab === "flow" && (
      <div className="space-y-4">
          <div className="grid min-w-0 gap-4 xl:grid-cols-[3.5fr_1fr]">
            <Card className="flex min-w-0 h-full flex-col overflow-hidden border-border/70 bg-card">
              <CardHeader className="flex flex-col gap-3 border-b border-border/60 px-4 py-4 md:grid md:flex-row md:items-start md:justify-between">
                <div className="min-w-0 md:max-w-[220px] xl:max-w-[280px]">
                  <CardTitle className="text-sm">{t.vulnScan.combination.inline.presets.title}</CardTitle>
                  <CardDescription className="mt-0.5 line-clamp-2 text-xs">
                    {t.vulnScan.combination.inline.presets.desc}
                  </CardDescription>
                </div>
                <CardAction className="min-w-0 w-full md:w-auto">
                  <div className="flex w-full min-w-0 flex-wrap items-center gap-2 pb-1 md:flex-nowrap md:overflow-x-auto md:justify-end">
                    <Select value={selectedSavedTemplateId} onValueChange={handleSavedTemplateChange}>
                      <SelectTrigger className="h-9 w-full min-w-0 md:w-[220px] md:shrink-0 lg:w-[240px]">
                        <SelectValue placeholder={loadingSavedTemplates ? t.vulnScan.combination.inline.presets.loadingTemplates : t.vulnScan.combination.inline.presets.selectSavedTemplate} />
                      </SelectTrigger>
                      <SelectContent>
                        {savedTemplates.map((template) => (
                          <SelectItem key={template.id} value={template.id}>
                            {template.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <Button variant="outline" size="sm" className="shrink-0" onClick={handleOpenSaveTemplateDialog} disabled={isSubmitting}>
                      <Save className="mr-1 h-4 w-4" />
                      {t.vulnScan.combination.inline.actions.saveAsTemplate}
                    </Button>
                    <Button variant="ghost" size="sm" className="shrink-0" onClick={handleClearFlow} disabled={isSubmitting}>
                      {t.vulnScan.combination.inline.actions.clearFlow}
                    </Button>
                  </div>
                </CardAction>
              </CardHeader>
              <CardContent className="flex flex-1 flex-col px-4">
                <div className="grid min-w-0 grid-cols-1 md:grid-cols-[repeat(auto-fit,minmax(220px,1fr))] gap-2.5">
                  {presets.map((preset) => {
                    const PresetIcon = preset.icon
                    const isActive = activePresetId === preset.id
                    const workflowSummary = preset.stages
                      .map((stage) => stage.modules.map((moduleId) => moduleCatalog[moduleId]?.title).join(" → "))
                      .join(" / ")
                    return (
                      <button
                        key={preset.id}
                        type="button"
                        data-slot="combination-preset-card"
                        data-active={isActive ? "true" : "false"}
                        disabled={isSubmitting}
                        onClick={() => handleApplyPreset(preset)}
                        className={cn(
                          "flex min-h-[108px] w-full flex-col gap-2 rounded-xl border px-3 py-3 text-left transition disabled:cursor-not-allowed disabled:opacity-60",
                          isActive
                            ? "border-primary/40 bg-primary/8 shadow-[0_10px_20px_rgba(15,23,42,0.06)]"
                            : "border-border/70 bg-card shadow-sm hover:border-primary/25 hover:bg-accent/25 hover:shadow-[0_8px_18px_rgba(15,23,42,0.05)]"
                        )}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div className={cn("inline-flex h-8 w-8 items-center justify-center rounded-[10px] border border-border/60 bg-muted/35", preset.tone)}>
                            <PresetIcon className="h-5 w-5" />
                          </div>
                            <span className="shrink-0 rounded-full border border-border/50 bg-muted/30 px-2 py-1 text-[10px] leading-none text-muted-foreground">
                            {formatText(t.vulnScan.combination.inline.presets.stageCount, { count: preset.stages.length })}
                          </span>
                        </div>
                        <div className="space-y-1">
                          <div className="text-[13px] font-semibold text-foreground">{preset.title}</div>
                          <div className="line-clamp-2 text-[12px] leading-5 text-muted-foreground">
                            {workflowSummary}
                          </div>
                        </div>
                      </button>
                    )
                  })}
                </div>
              </CardContent>
            </Card>

            <Card className="flex h-full flex-col overflow-hidden border-border/70 bg-card/90">
              <CardHeader className="min-h-[50px] border-b border-border/60 px-4">
                <div>
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <span>{t.vulnScan.combination.inline.targetConfig}</span>
                  </CardTitle>
                  <CardDescription className="mt-0.5 text-xs">{targetConfigDescription}</CardDescription>
                  </div>
              </CardHeader>
              <CardContent className="flex flex-1 flex-col px-4">
                <Textarea
                  value={targetInput}
                  onChange={(e) => setTargetInput(e.target.value)}
                  placeholder={targetInputPlaceholder}
                  className="min-h-[228px] max-h-[400px] resize-none overflow-y-auto border-border/70 bg-background font-mono"
                />
                <div className="mt-3 flex flex-col gap-1 text-xs text-muted-foreground">
                  <div>{formatText(t.vulnScan.combination.inline.parsedTargets, { count: parsedTargets.length })}</div>
                  {targetInputRule.inputType ? (
                    <div>{formatText(t.vulnScan.combination.inline.currentTargetType, { type: targetInputTypeLabel })}</div>
                  ) : null}
                  {targetValidation.invalidTargets.length > 0 && targetInputRule.inputType ? (
                    <div className="text-destructive">
                      {formatText(t.vulnScan.combination.inline.errors.targetTypeMismatch, {
                        count: targetValidation.invalidTargets.length,
                        type: targetInputTypeLabel,
                      })}
                    </div>
                  ) : null}
                </div>
              </CardContent>
            </Card>
          </div>

          <div className="grid gap-4">
            <Card className="overflow-hidden border-border/70">
              <CardHeader className="flex flex-wrap items-center justify-between gap-3 md:grid md:items-start md:gap-2 min-h-[58px] md:justify-center border-b border-border/60 bg-transparent px-4 md:px-5">
                <div>
                  <CardTitle className="text-sm flex items-center gap-2">
                    <Layers className="w-4 h-4" /> {t.vulnScan.combination.inline.canvasTitle}
                  </CardTitle>
                  <CardDescription className="mt-0.5 text-xs">
                    <span className="md:hidden">{t.vulnScan.combination.inline.mobileCanvasDesc}</span>
                    <span className="hidden md:inline">{t.vulnScan.combination.inline.canvasDesc}</span>
                  </CardDescription>
                </div>
                <CardAction>
                  <Button variant="outline" size="sm" onClick={handleAddStage} disabled={isSubmitting}>
                    <Plus className="w-4 h-4 mr-1" /> {t.vulnScan.combination.inline.newStage}
                  </Button>
                </CardAction>
              </CardHeader>
              <CardContent className="min-w-0 px-4 md:px-6">
                <div className="w-full md:overflow-x-auto">
                  <div className="flex w-full min-w-0 flex-col items-stretch gap-6 pb-4 md:min-h-[360px] md:min-w-full md:w-max md:flex-row md:items-start">
                    {stages.map((stage, stageIndex) => (
                      <div key={stage.id} className="relative min-w-0 w-full md:w-[280px]">
                        {stageIndex < stages.length - 1 && (
                          <div className="absolute bottom-[-24px] left-1/2 h-6 w-px bg-border md:bottom-auto md:left-auto md:right-[-18px] md:top-1/2 md:h-[2px] md:w-10" />
                        )}
                        <div className="rounded-2xl border bg-muted/10 p-3 space-y-3">
                          <div className="flex items-center gap-2">
                            <Input
                              value={stage.title}
                              onChange={(e) =>
                                setStages((prev) => prev.map((s) => (s.id === stage.id ? { ...s, title: e.target.value } : s)))
                              }
                              className="h-8 text-sm"
                              placeholder={formatText(t.vulnScan.combination.inline.stageName, { n: stageIndex + 1 })}
                            />
                            <div className="flex items-center gap-1">
                              <DropdownMenu>
                                <DropdownMenuTrigger asChild>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-8 w-8"
                                    title={t.vulnScan.combination.inline.addTask}
                                    disabled={availableModules.filter(([moduleId]) => canAddModuleToStage(stage, moduleId as ModuleId)).length === 0}
                                  >
                                    <Plus className="w-4 h-4" />
                                  </Button>
                                </DropdownMenuTrigger>
                                <DropdownMenuContent align="end" className="w-44">
                                  {availableModules
                                    .filter(([moduleId]) => canAddModuleToStage(stage, moduleId as ModuleId))
                                    .map(([moduleId, meta]) => (
                                    <DropdownMenuItem key={moduleId} onClick={() => handleAddNode(stage.id, moduleId as ModuleId)}>
                                      <div className="flex items-start gap-2">
                                        <meta.icon className={cn("h-4 w-4 shrink-0", meta.tone)} />
                                        <div className="flex flex-col">
                                          <span className="text-sm font-medium">{meta.title}</span>
                                        <span className="text-[11px] text-muted-foreground">{meta.description}</span>
                                        </div>
                                      </div>
                                    </DropdownMenuItem>
                                  ))}
                                </DropdownMenuContent>
                              </DropdownMenu>
                              {stages.length > 1 && (
                                <Button
                                  size="icon"
                                  variant="ghost"
                                  className="h-8 w-8"
                                  onClick={() => handleRemoveStage(stage.id)}
                                  title={t.vulnScan.combination.inline.deleteStage}
                                >
                                  <Trash2 className="w-4 h-4" />
                                </Button>
                              )}
                            </div>
                          </div>
                          <div
                            className={cn(
                              "rounded-2xl border border-dashed p-3 space-y-3 min-h-[180px] bg-background/40 transition-colors",
                              dragOverStageId === stage.id && "border-primary/50 bg-primary/5"
                            )}
                            onDragOver={(e) => {
                              if (!draggingNode) return
                              e.preventDefault()
                              setDragOverStageId(stage.id)
                              setDragOverNodeId(null)
                            }}
                            onDragLeave={() => {
                              if (dragOverStageId === stage.id && !dragOverNodeId) {
                                setDragOverStageId(null)
                              }
                            }}
                            onDrop={(e) => {
                              if (!draggingNode) return
                              e.preventDefault()
                              handleMoveNode(draggingNode, stage.id)
                              resetDragState()
                            }}
                          >
                            {stage.nodes.length === 0 ? (
                              <div className="text-xs text-muted-foreground py-8 text-center">{t.vulnScan.combination.inline.clickAddTaskTopRight}</div>
                            ) : (
                              <div className="space-y-2">
                                {stage.nodes.map((node) => {
                                  const NodeIcon = moduleCatalog[node.moduleId].icon
                                  return <div
                                    key={node.id}
                                    role="button"
                                    tabIndex={0}
                                    draggable
                                    onDragStart={(e) => {
                                      e.dataTransfer.effectAllowed = "move"
                                      setDraggingNode({ stageId: stage.id, nodeId: node.id })
                                      setDragOverStageId(stage.id)
                                      setDragOverNodeId(node.id)
                                    }}
                                    onDragEnd={resetDragState}
                                    onDragOver={(e) => {
                                      if (!draggingNode || draggingNode.nodeId === node.id) return
                                      e.preventDefault()
                                      e.stopPropagation()
                                      setDragOverStageId(stage.id)
                                      setDragOverNodeId(node.id)
                                    }}
                                    onDrop={(e) => {
                                      if (!draggingNode) return
                                      e.preventDefault()
                                      e.stopPropagation()
                                      handleMoveNode(draggingNode, stage.id, node.id)
                                      resetDragState()
                                    }}
                                    onClick={() => openNodeDrawer(stage.id, node.id)}
                                    onKeyDown={(e) => {
                                      if (e.key === "Enter" || e.key === " ") {
                                        e.preventDefault()
                                        openNodeDrawer(stage.id, node.id)
                                      }
                                    }}
                                    className={cn(
                                      "w-full rounded-xl border bg-background p-3 text-left shadow-sm transition hover:border-primary cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/50",
                                      selectedNode?.nodeId === node.id && "border-primary ring-1 ring-primary/40",
                                      dragOverNodeId === node.id && draggingNode?.nodeId !== node.id && "border-primary/60 bg-primary/5"
                                    )}
                                  >
                                    <div className="flex items-center justify-between text-sm font-medium">
                                      <span className="inline-flex items-center gap-2">
                                        <GripVertical className="w-3.5 h-3.5 text-muted-foreground/50" />
                                        <NodeIcon className={cn("h-4 w-4 shrink-0", moduleCatalog[node.moduleId].tone)} />
                                        <span className={moduleCatalog[node.moduleId].tone}>{moduleCatalog[node.moduleId].title}</span>
                                      </span>
                                      <Button
                                        size="icon"
                                        variant="ghost"
                                        className="h-7 w-7"
                                        onClick={(e) => {
                                          e.stopPropagation()
                                          handleRemoveNode({ stageId: stage.id, nodeId: node.id })
                                        }}
                                      >
                                        <Trash2 className="w-4 h-4 text-muted-foreground" />
                                      </Button>
                                    </div>
                                    <p className="text-xs text-muted-foreground mt-1 line-clamp-2">
                                      {moduleCatalog[node.moduleId].description}
                                    </p>
                                    <p className="text-[11px] text-muted-foreground mt-1">
                                      {getNodeSummary(node)}
                                    </p>
                                  </div>
                                })}
                              </div>
                            )}
                          </div>
                        </div>
                      </div>
                    ))}
                    <button
                      type="button"
                      onClick={handleAddStage}
                      className="flex h-12 w-full md:h-[220px] md:w-[180px] items-center justify-center rounded-2xl border border-dashed text-sm text-muted-foreground hover:border-primary hover:text-primary bg-background/40"
                    >
                      <Plus className="w-4 h-4 mr-1" /> {t.vulnScan.combination.inline.newStage}
                    </button>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
      </div>
      )}

      {activeConfigTab === "execution" && !desktopRuntime && (
      <div>
          <Card className="overflow-hidden border-border/70 bg-card/90">
            <CardContent>
              <div className="grid gap-4 xl:grid-cols-[240px_minmax(0,1fr)]">
                <div className="space-y-2">
                  <label className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
                    {t.vulnScan.combination.inline.execution.modeLabel}
                  </label>
                  <Select value={runMode} onValueChange={(value) => setRunMode(value as RunMode)}>
                    <SelectTrigger className="h-11 w-full bg-background/70">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto">{t.vulnScan.combination.inline.execution.autoOption}</SelectItem>
                      <SelectItem value="local">{t.vulnScan.combination.inline.execution.localOption}</SelectItem>
                      <SelectItem value="agent">{t.vulnScan.combination.inline.execution.agentOption}</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <label className="text-[11px] font-medium uppercase tracking-[0.2em] text-muted-foreground">
                    {runMode === "agent" ? t.vulnScan.combination.inline.execution.nodeLabel : t.vulnScan.combination.inline.execution.runtimeLabel}
                  </label>
                  {runMode === "agent" ? (
                    <Select value={selectedAgentId} onValueChange={setSelectedAgentId}>
                      <SelectTrigger className="h-11 w-full bg-background/70">
                        <SelectValue placeholder={t.vulnScan.combination.inline.execution.selectAgent} />
                      </SelectTrigger>
                      <SelectContent>
                        {availableAgents.map((agent) => (
                          <SelectItem key={agent.agentId} value={agent.agentId}>
                            {agent.displayName || agent.agentId} ({agent.agentId}){agent.localWorker ? ` · ${t.vulnScan.combination.inline.execution.localNode}` : ''}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <div className="flex min-h-11 items-center rounded-md border border-border/70 bg-background/50 px-3 py-2 text-sm leading-6 break-words text-muted-foreground md:h-11 md:py-0 md:leading-5">
                      {runMode === "auto" ? t.vulnScan.combination.inline.execution.autoRuntimeHint : t.vulnScan.combination.inline.execution.localRuntimeHint}
                    </div>
                  )}
                  {runMode === "agent" && (
                    <div className="text-xs text-muted-foreground">
                      {formatText(
                        t.vulnScan.combination.inline.execution.availableAgents,
                        { count: loadingAgents ? t.vulnScan.combination.inline.execution.loadingAgents : availableAgents.length }
                      )}
                    </div>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
      </div>
      )}

      {activeConfigTab === "trigger" && (
      <div>
          <TaskScheduleSettings
            value={taskSchedule}
            onChange={setTaskSchedule}
          />
      </div>
      )}


    </div>

    <Dialog open={saveTemplateDialogOpen} onOpenChange={setSaveTemplateDialogOpen}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t.vulnScan.combination.inline.presets.saveTemplateTitle}</DialogTitle>
          <DialogDescription>{t.vulnScan.combination.inline.presets.saveTemplateDesc}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <div className="text-sm font-medium">{t.vulnScan.combination.inline.presets.templateName}</div>
            <Input
              value={templateName}
              onChange={(e) => setTemplateName(e.target.value)}
              placeholder={t.vulnScan.combination.inline.presets.templateNamePlaceholder}
            />
          </div>
          <div className="space-y-2">
            <div className="text-sm font-medium">{t.vulnScan.combination.inline.presets.templateDescription}</div>
            <Textarea
              value={templateDescription}
              onChange={(e) => setTemplateDescription(e.target.value)}
              placeholder={t.vulnScan.combination.inline.presets.templateDescriptionPlaceholder}
              className="min-h-[96px] resize-none"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => setSaveTemplateDialogOpen(false)} disabled={savingTemplate}>
            {t.vulnScan.combination.inline.actions.cancel}
          </Button>
          <Button onClick={handleSaveCombinationTemplate} disabled={savingTemplate}>
            {t.vulnScan.combination.inline.actions.confirm}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <Sheet open={drawerOpen && Boolean(selectedDetails)} onOpenChange={(open) => (open ? setDrawerOpen(true) : closeNodeDrawer())}>
      <SheetContent side="right" className="w-full sm:w-[620px] sm:max-w-[620px] flex flex-col p-0">
        <SheetHeader className="px-4 pt-4 pb-2">
          <SheetTitle>{t.vulnScan.combination.inline.sheetTitle}</SheetTitle>
          <SheetDescription>{t.vulnScan.combination.inline.sheetDesc}</SheetDescription>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto px-4 pb-6">{renderNodeSettings()}</div>
      </SheetContent>
    </Sheet>
    </>
  )
}
