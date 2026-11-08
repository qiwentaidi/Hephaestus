'use client'

import { HostDiscoverySettings } from '@/app/(main)/scan/vuln-scan/new/components/HostDiscoverySettings'
import { DEFAULT_TCP_HEURISTIC_CONFIG, normalizeTCPHeuristicConfig, validateTCPHeuristicConfig, type TCPHeuristicConfig } from '@/lib/tcp-heuristic'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { ScrollArea } from '@/components/ui/scroll-area'
import { AlertTriangle, Plus, Edit, Trash2, FileText, Globe, Server, FolderSearch, FileCode, Key, Fingerprint, Scan, Shield, Target, Workflow, Bot } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import toast from 'react-hot-toast'
import { SelectTag, type SelectTagOption } from '@/components/ui/select-tag'
import { useI18n } from '@/lib/i18n'
import { useScreenshotSnapChromeWarning } from '@/lib/use-screenshot-snap-chrome-warning'
import {
  BUILTIN_DATABASE_PORT_GROUP_TEXT,
  BUILTIN_DATABASE_PORT_GROUP_VALUE,
  BUILTIN_TOP1000_PORT_GROUP_ID,
  builtInPortGroupOptions,
  findPortGroupByText,
  findPortGroupByValue,
  mergePortGroupOptions,
  type PortGroupOption,
} from '@/app/(main)/scan/vuln-scan/utils/portGroup'

// 任务类型定义
type TaskType = 'webscan' | 'portscan' | 'dirsearch' | 'jsscan' | 'crack' | 'combination' | 'ai_dialogue'

// 各任务类型配置
interface WebScanConfig {
  webscanOption: 1 | 2 | 3 | 4
  threads: number
  nucleiThreads?: number
  customHeaders: string
  customTemplate: string[]
  rootPathScan: boolean
  skipNucleiWithoutTags: boolean
  skipNucleiWithWAF: boolean
  screenshot: boolean
  activeTimeoutLimit: number
  disableMaxHostErr?: boolean
}

interface PortScanConfig extends TCPHeuristicConfig {
  portGroup: string
  portRange: string
  threads: number
  timeout: number
  aliveDetection: boolean
}

interface DirScanConfig {
  method: string
  extensions: string
  statusFilter: string
  selectedDicts: string[]
  customDict: string
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

interface CrackScanConfig {
  username: string
  password: string
  defaultDict: boolean
}

interface JsScanConfig {
  blackDomain: string[]
  highRiskRouter: string[]
  authentication: string[]
  placeholder: Record<string, string>
  sqlInjectionEnabled: boolean
  lfiEnabled: boolean
  ssrfEnabled: boolean
  redirectEnabled: boolean
  xssEnabled: boolean
  uploadEnabled: boolean
  aiDetectionEnabled: boolean
}

interface CombinationTemplateConfig {
  name?: string
  nodes?: Array<{
    id?: string
    moduleId?: string
    stageIndex?: number
    laneIndex?: number
    config?: Record<string, unknown>
  }>
  scanMode?: 'auto' | 'local' | 'agent'
  agentId?: string
  schedule?: {
    enabled?: boolean
    spec?: { kind?: string } | null
  }
}

interface AIDialogueTemplateConfig {
  prompt: string
}

const defaultCombinationTemplateConfig: CombinationTemplateConfig = {
  name: '',
  nodes: [],
  scanMode: 'auto',
  schedule: {
    enabled: false,
    spec: null,
  },
}

type ScanConfig = WebScanConfig | PortScanConfig | DirScanConfig | CrackScanConfig | JsScanConfig | CombinationTemplateConfig | AIDialogueTemplateConfig

export interface ScanTemplate {
  id?: string
  name: string
  description?: string
  taskType: TaskType
  config: ScanConfig
}

// 默认配置
const defaultWebScanConfig: WebScanConfig = {
  webscanOption: 3,
  threads: 30,
  nucleiThreads: 2,
  customHeaders: '',
  customTemplate: [],
  rootPathScan: true,
  skipNucleiWithoutTags: false,
  skipNucleiWithWAF: true,
  screenshot: true,
  activeTimeoutLimit: 5,
  disableMaxHostErr: false,
}

const normalizeWebScanConfig = (config: WebScanConfig): WebScanConfig => ({
  ...config,
  skipNucleiWithWAF: config.webscanOption === 4 ? false : config.skipNucleiWithWAF,
})

const defaultPortScanConfig: PortScanConfig = {
  ...DEFAULT_TCP_HEURISTIC_CONFIG,
  portGroup: BUILTIN_DATABASE_PORT_GROUP_TEXT,
  portRange: BUILTIN_DATABASE_PORT_GROUP_VALUE,
  threads: 1000,
  timeout: 10,
  aliveDetection: false,
}

const defaultDirScanConfig: DirScanConfig = {
  method: 'GET',
  extensions: 'php,aspx,asp,jsp,html,js',
  statusFilter: '404',
  selectedDicts: [],
  customDict: '',
  thread: 25,
  timeout: 8,
  interval: 0,
  recursion: 0,
  times: 5,
  exclude: '',
  headers: '',
  redirectClient: false,
  backupfileScan: false,
}

const defaultCrackScanConfig: CrackScanConfig = {
  username: '',
  password: '',
  defaultDict: true,
}

const defaultJsScanConfig: JsScanConfig = {
  blackDomain: [],
  highRiskRouter: [],
  authentication: [],
  placeholder: {},
  sqlInjectionEnabled: true,
  lfiEnabled: true,
  ssrfEnabled: true,
  redirectEnabled: true,
  xssEnabled: true,
  uploadEnabled: true,
  aiDetectionEnabled: false,
}

// taskTypeInfo without labels - labels will be fetched from i18n
const taskTypeInfo: Record<TaskType, { icon: typeof Globe; color: string }> = {
  webscan: { icon: Globe, color: 'text-blue-500' },
  portscan: { icon: Server, color: 'text-emerald-500' },
  dirsearch: { icon: FolderSearch, color: 'text-purple-500' },
  jsscan: { icon: FileCode, color: 'text-amber-500' },
  crack: { icon: Key, color: 'text-orange-500' },
  combination: { icon: Workflow, color: 'text-violet-500' },
  ai_dialogue: { icon: Bot, color: 'text-amber-500' },
}

const creatableTaskTypes: TaskType[] = ['webscan', 'portscan', 'dirsearch', 'jsscan', 'crack', 'ai_dialogue']

export function ScanTemplateTab() {
  const { t } = useI18n()
  const showScreenshotSnapChromeWarning = useScreenshotSnapChromeWarning()
  const [templates, setTemplates] = useState<ScanTemplate[]>([])
  const [loading, setLoading] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingTemplate, setEditingTemplate] = useState<ScanTemplate | null>(null)
  const [templateList, setTemplateList] = useState<string[]>([])
  const [dictList, setDictList] = useState<{id: string, name: string}[]>([])
  const [filterTaskType, setFilterTaskType] = useState<TaskType | 'all'>('all')
  const [customPortGroups, setCustomPortGroups] = useState<PortGroupOption[]>([])
  const [loadingPortGroups, setLoadingPortGroups] = useState(false)
  const [showPortGroupDialog, setShowPortGroupDialog] = useState(false)
  const [newPortGroupName, setNewPortGroupName] = useState('')
  const [newPortGroupPorts, setNewPortGroupPorts] = useState('')
  const [savingPortGroup, setSavingPortGroup] = useState(false)
  const [deletingPortGroupId, setDeletingPortGroupId] = useState<string | null>(null)
  const [defaultPortGroupId, setDefaultPortGroupId] = useState(BUILTIN_TOP1000_PORT_GROUP_ID)
  const [savingDefaultPortGroupId, setSavingDefaultPortGroupId] = useState<string | null>(null)
  const [deleteTemplateId, setDeleteTemplateId] = useState<string | null>(null)

  // 表单状态
  const [formData, setFormData] = useState<ScanTemplate>({
    name: '',
    description: '',
    taskType: 'webscan',
    config: defaultWebScanConfig,
  })

  const fetchTemplates = useCallback(async () => {
    const response = await fetchWithAuth(`/api/v1/config/scan-templates`)

    if (!response.ok) {
      const errorData = await response.json().catch(() => ({}))
      throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
    }

    const data = await response.json()
    if (data.success) {
      return data.data || []
    }

    console.warn('Failed to load templates', data.error || '')
    return []
  }, [])

  // 获取模板列表
  const loadTemplates = useCallback(async () => {
    setLoading(true)
    try {
      setTemplates(await fetchTemplates())
    } catch (error: any) {
      console.error('Failed to load templates', error)
      if (error.message && (error.message.includes('does not exist') || error.message.includes('relation'))) {
        setTemplates([])
      } else {
        toast.error(t.scanTemplate.messages.loadFailed + ': ' + (error.message || ''))
        setTemplates([])
      }
    } finally {
      setLoading(false)
    }
  }, [fetchTemplates, t.scanTemplate.messages.loadFailed])

  const fetchTemplateList = useCallback(async () => {
    const response = await fetchWithAuth(`/api/v1/webscan/templates`)
    if (!response.ok) return []
    const data = await response.json()
    return data.success && data.data ? data.data : []
  }, [])

  // 获取漏洞模板列表
  const loadTemplateList = useCallback(async () => {
    try {
      setTemplateList(await fetchTemplateList())
    } catch (error) {
      console.error('Failed to load vulnerability templates', error)
    }
  }, [fetchTemplateList])

  const fetchDictList = useCallback(async () => {
    const response = await fetchWithAuth(`/api/v1/dirsearch/dicts`)
    if (!response.ok) return []
    const data = await response.json()
    return data.success && data.dicts
      ? data.dicts.map((d: any) => ({ id: d.name || d.id, name: d.name }))
      : []
  }, [])

  // 获取目录扫描字典列表
  const loadDictList = useCallback(async () => {
    try {
      setDictList(await fetchDictList())
    } catch (error) {
      console.error('Failed to load dict list', error)
    }
  }, [fetchDictList])

  // 过滤后的模板列表
  const filteredTemplates = useMemo(() => {
    if (filterTaskType === 'all') return templates
    return templates.filter(t => t.taskType === filterTaskType)
  }, [templates, filterTaskType])

  // 将模板列表转换为 SelectTagOption 格式
  const templateOptions: SelectTagOption[] = templateList.map((template) => ({
    value: template,
    label: template,
  }))

  const dictOptions: SelectTagOption[] = dictList.map((d) => ({
    value: d.id,
    label: d.name,
  }))

  const portGroupOptions = useMemo(
    () => mergePortGroupOptions(customPortGroups),
    [customPortGroups]
  )

  const fetchCustomPortGroups = useCallback(async () => {
    const response = await fetchWithAuth('/api/v1/config/port-groups', { cache: 'no-store' })
    const data = await response.json()
    if (!response.ok || !data.success) {
      throw new Error(data.error || '加载自定义端口组失败')
    }

    return Array.isArray(data.data)
      ? data.data.map((item: any) => ({
          id: String(item.id || ''),
          text: String(item.name || ''),
          value: String(item.ports || ''),
        }))
      : []
  }, [])

  const fetchDefaultPortGroup = useCallback(async () => {
    const response = await fetchWithAuth('/api/v1/config/port-groups/default', { cache: 'no-store' })
    const data = await response.json()
    if (!response.ok || !data.success) {
      throw new Error(data.error || '加载默认端口组失败')
    }
    return String(data.data?.id || BUILTIN_TOP1000_PORT_GROUP_ID)
  }, [])

  const handleSetDefaultPortGroup = useCallback(async (group: PortGroupOption) => {
    try {
      setSavingDefaultPortGroupId(group.id)
      const response = await fetchWithAuth('/api/v1/config/port-groups/default', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: group.id,
          name: group.text,
          ports: group.value,
          builtIn: Boolean(group.builtIn),
        }),
      })
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || '设置默认端口组失败')
      }
      setDefaultPortGroupId(String(data.data?.id || group.id))
      toast.success(`已将「${group.text}」设为默认端口组`)
    } catch (error: any) {
      toast.error(error.message || '设置默认端口组失败')
    } finally {
      setSavingDefaultPortGroupId(null)
    }
  }, [])

  const loadCustomPortGroups = useCallback(async () => {
    try {
      setLoadingPortGroups(true)
      setCustomPortGroups(await fetchCustomPortGroups())
    } catch (error: any) {
      toast.error(error.message || '加载自定义端口组失败')
    } finally {
      setLoadingPortGroups(false)
    }
  }, [fetchCustomPortGroups])

  useEffect(() => {
    let cancelled = false

    const loadInitialOptions = async () => {
      setLoading(true)
      setLoadingPortGroups(true)
      const [templatesResult, templateListResult, dictListResult, portGroupsResult] = await Promise.allSettled([
        fetchTemplates(),
        fetchTemplateList(),
        fetchDictList(),
        fetchCustomPortGroups(),
      ])
      if (cancelled) return

      if (templatesResult.status === 'fulfilled') {
        setTemplates(templatesResult.value)
      } else {
        const error = templatesResult.reason
        console.error('Failed to load templates', error)
        if (!(error instanceof Error && (error.message.includes('does not exist') || error.message.includes('relation')))) {
          toast.error(t.scanTemplate.messages.loadFailed + ': ' + (error instanceof Error ? error.message : ''))
        }
        setTemplates([])
      }

      if (templateListResult.status === 'fulfilled') {
        setTemplateList(templateListResult.value)
      } else {
        console.error('Failed to load vulnerability templates', templateListResult.reason)
      }

      if (dictListResult.status === 'fulfilled') {
        setDictList(dictListResult.value)
      } else {
        console.error('Failed to load dict list', dictListResult.reason)
      }

      if (portGroupsResult.status === 'fulfilled') {
        setCustomPortGroups(portGroupsResult.value)
      } else {
        const error = portGroupsResult.reason
        toast.error(error instanceof Error ? error.message : '加载自定义端口组失败')
      }

      setLoading(false)
      setLoadingPortGroups(false)
    }

    void loadInitialOptions()

    return () => {
      cancelled = true
    }
  }, [
    fetchCustomPortGroups,
    fetchDictList,
    fetchTemplateList,
    fetchTemplates,
    t.scanTemplate.messages.loadFailed,
  ])

  useEffect(() => {
    void fetchDefaultPortGroup()
      .then(setDefaultPortGroupId)
      .catch((error) => toast.error(error instanceof Error ? error.message : '加载默认端口组失败'))
  }, [fetchDefaultPortGroup])

  const handleTaskTypeChange = (taskType: TaskType) => {
    // 对于 jsscan，从 API 获取默认配置
    if (taskType === 'jsscan') {
      // 先设置本地默认值，然后异步加载 API 配置
      setFormData({ ...formData, taskType, config: defaultJsScanConfig })
      
      fetchWithAuth('/api/v1/jsscan/default-config')
        .then((res) => res.json())
        .then((data) => {
          if (data.success && data.data) {
            const apiConfig = data.data
            const jsScanConfig: JsScanConfig = {
              blackDomain: apiConfig.blackDomain || [],
              highRiskRouter: apiConfig.highRiskRouter || [],
              authentication: apiConfig.authentication || [],
              placeholder: apiConfig.placeholder || {},
              sqlInjectionEnabled: apiConfig.vulnDetection?.sqlInjection?.enabled !== false,
              lfiEnabled: apiConfig.vulnDetection?.lfi?.enabled !== false,
              ssrfEnabled: apiConfig.vulnDetection?.ssrf?.enabled !== false,
              redirectEnabled: apiConfig.vulnDetection?.redirect?.enabled !== false,
              xssEnabled: apiConfig.vulnDetection?.xss?.enabled !== false,
              uploadEnabled: apiConfig.vulnDetection?.upload?.enabled !== false,
              aiDetectionEnabled: apiConfig.openai?.enabled === true,
            }
            setFormData(prev => ({ ...prev, config: jsScanConfig }))
          }
        })
        .catch((error) => {
          console.error('Failed to load JS scan default config', error)
        })
      return
    }

    let config: ScanConfig
    switch (taskType) {
      case 'webscan':
        config = defaultWebScanConfig
        break
      case 'portscan':
        config = defaultPortScanConfig
        break
      case 'dirsearch':
        config = defaultDirScanConfig
        break
      case 'crack':
        config = defaultCrackScanConfig
        break
      case 'combination':
        config = defaultCombinationTemplateConfig
        break
      case 'ai_dialogue':
        config = { prompt: '' }
        break
      default:
        config = defaultWebScanConfig
    }
    setFormData({ ...formData, taskType, config })
  }

  const normalizePortScanConfig = (config: PortScanConfig): PortScanConfig => {
    const legacyMap: Record<string, { label: string; value: string }> = {
      common: {
        label: BUILTIN_DATABASE_PORT_GROUP_TEXT,
        value: BUILTIN_DATABASE_PORT_GROUP_VALUE,
      },
      database: {
        label: BUILTIN_DATABASE_PORT_GROUP_TEXT,
        value: BUILTIN_DATABASE_PORT_GROUP_VALUE,
      },
      highRisk: {
        label: '高危端口',
        value: '21,22,23,53,80,443,8080,8000,139,445,3389,1521,3306,6379,7001,2375,27017,11211',
      },
      full: {
        label: '全端口',
        value: '1-65535',
      },
      custom: {
        label: '',
        value: config.portRange,
      },
    }

    const next = { ...config, ...normalizeTCPHeuristicConfig(config) }
    const legacy = legacyMap[next.portGroup]
    if (legacy) {
      next.portGroup = legacy.label
      if (!next.portRange && legacy.value) {
        next.portRange = legacy.value
      }
    }

    const matchedByText = findPortGroupByText(portGroupOptions, next.portGroup)
    if (matchedByText) {
      return {
        ...next,
        portGroup: matchedByText.text,
        portRange: next.portRange || matchedByText.value,
      }
    }

    const matchedByValue = findPortGroupByValue(portGroupOptions, next.portRange)
    if (matchedByValue) {
      return {
        ...next,
        portGroup: matchedByValue.text,
        portRange: matchedByValue.value,
      }
    }

    return {
      ...next,
      portGroup: '',
    }
  }

  const handleOpenDialog = (template?: ScanTemplate) => {
    if (template) {
      setEditingTemplate(template)
      if (template.taskType === 'portscan') {
        setFormData({
          ...template,
          config: normalizePortScanConfig(template.config as PortScanConfig),
        })
      } else {
        setFormData(template)
      }
    } else {
      setEditingTemplate(null)
      setFormData({
        name: '',
        description: '',
        taskType: 'webscan',
        config: defaultWebScanConfig,
      })
    }
    setDialogOpen(true)
  }

  const handleCloseDialog = () => {
    setDialogOpen(false)
    setEditingTemplate(null)
  }

  const handleSave = async () => {
    if (formData.taskType === 'portscan') {
      const config = formData.config as PortScanConfig
      const error = validateTCPHeuristicConfig(normalizeTCPHeuristicConfig(config), config.portRange)
      if (error) {
        toast.error(t.vulnScan.newScan.tcpHeuristic[error])
        return
      }
    }
    if (!formData.name.trim()) {
      toast.error(t.scanTemplate.messages.nameRequired)
      return
    }

    if (formData.taskType === 'combination') {
      const config = formData.config as CombinationTemplateConfig
      const nodes = Array.isArray(config.nodes) ? config.nodes : []
      if (nodes.length === 0) {
        toast.error(t.scanTemplate.messages.combinationCreateFromFlow)
        return
      }
    }

    if (formData.taskType === 'ai_dialogue') {
      const prompt = (formData.config as AIDialogueTemplateConfig).prompt || ''
      if (!prompt.trim()) {
        toast.error(t.scanTemplate.aiDialogue.promptRequired)
        return
      }
      if (!prompt.includes('{{.goal}}')) {
        toast.error(t.scanTemplate.aiDialogue.placeholderRequired)
        return
      }
    }

    try {
      const url = editingTemplate?.id
        ? `/api/v1/config/scan-templates/${editingTemplate.id}`
        : `/api/v1/config/scan-templates`

      const method = editingTemplate?.id ? 'PUT' : 'POST'

      const response = await fetchWithAuth(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(formData),
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.scanTemplate.messages.saveFailed)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.scanTemplate.messages.saveFailed)
      }

      toast.success(editingTemplate ? t.scanTemplate.messages.updated : t.scanTemplate.messages.created)
      handleCloseDialog()
      loadTemplates()
    } catch (error: any) {
      console.error('Failed to save template', error)
      toast.error(t.scanTemplate.messages.saveFailed + ': ' + (error.message || ''))
    }
  }

  const handleDelete = async (id: string) => {
    try {
      const response = await fetchWithAuth(`/api/v1/config/scan-templates/${id}`, {
        method: 'DELETE',
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || t.scanTemplate.messages.deleteFailed)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.scanTemplate.messages.deleteFailed)
      }

      toast.success(t.scanTemplate.messages.deleted)
      setDeleteTemplateId(null)
      loadTemplates()
    } catch (error: any) {
      console.error('Failed to delete template', error)
      toast.error(t.scanTemplate.messages.deleteFailed + ': ' + (error.message || ''))
    }
  }

  // 渲染配置表单
  const renderConfigForm = () => {
    switch (formData.taskType) {
      case 'webscan':
        return renderWebScanConfig()
      case 'portscan':
        return renderPortScanConfig()
      case 'dirsearch':
        return renderDirScanConfig()
      case 'jsscan':
        return renderJsScanConfig()
      case 'crack':
        return renderCrackScanConfig()
      case 'combination':
        return renderCombinationConfig()
      case 'ai_dialogue':
        return renderAIDialogueConfig()
      default:
        return null
    }
  }

  const renderAIDialogueConfig = () => {
    const config = formData.config as AIDialogueTemplateConfig
    return (
      <div className="space-y-2">
        <Label>{t.scanTemplate.aiDialogue.prompt}</Label>
        <Textarea
          value={config.prompt || ''}
          onChange={(e) => setFormData({ ...formData, config: { ...config, prompt: e.target.value } })}
          placeholder={t.scanTemplate.aiDialogue.promptPlaceholder}
          className="min-h-40 leading-6"
        />
        <p className="text-xs text-muted-foreground">{t.scanTemplate.aiDialogue.promptHint}</p>
      </div>
    )
  }

  const renderCombinationConfig = () => {
    const config = formData.config as CombinationTemplateConfig
    const nodes = Array.isArray(config.nodes) ? config.nodes : []
    const stageCount = nodes.reduce((max, node) => Math.max(max, (node.stageIndex ?? 0) + 1), 0)
    const scheduleKind = config.schedule?.spec?.kind || 'manual'

    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-border/60 bg-muted/20 p-4">
          <div className="text-sm font-medium">{t.scanTemplate.combination.readonlyTitle}</div>
          <p className="mt-1 text-xs text-muted-foreground">
            {t.scanTemplate.combination.readonlyDesc}
          </p>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="rounded-lg border border-border/60 p-3">
            <div className="text-xs text-muted-foreground">{t.scanTemplate.combination.stageCount}</div>
            <div className="mt-1 text-lg font-semibold">{stageCount}</div>
          </div>
          <div className="rounded-lg border border-border/60 p-3">
            <div className="text-xs text-muted-foreground">{t.scanTemplate.combination.moduleCount}</div>
            <div className="mt-1 text-lg font-semibold">{nodes.length}</div>
          </div>
          <div className="rounded-lg border border-border/60 p-3">
            <div className="text-xs text-muted-foreground">{t.scanTemplate.combination.runMode}</div>
            <div className="mt-1 text-lg font-semibold">{config.scanMode || 'auto'}</div>
          </div>
        </div>
        <div className="rounded-lg border border-border/60 p-3">
          <div className="text-xs text-muted-foreground">{t.scanTemplate.combination.scheduleMode}</div>
          <div className="mt-1 text-sm font-medium">{scheduleKind}</div>
        </div>
      </div>
    )
  }

  // 网站扫描配置
  const renderWebScanConfig = () => {
    const config = normalizeWebScanConfig(formData.config as WebScanConfig)
    const updateConfig = (updates: Partial<WebScanConfig>) => {
      setFormData({ ...formData, config: normalizeWebScanConfig({ ...config, ...updates }) })
    }

    return (
      <div className="space-y-4">
        {/* 扫描模式 */}
        <div className="space-y-2">
          <Label>{t.scanTemplate.webscan.scanMode}</Label>
          <Tabs
            value={String(config.webscanOption)}
            onValueChange={(v) => updateConfig({ webscanOption: Number(v) as 1 | 2 | 3 | 4 })}
            className="w-full"
          >
            <TabsList className="grid h-auto w-full grid-cols-2 sm:grid-cols-4">
              <TabsTrigger value="1" className="text-xs">
                <Fingerprint className="w-3 h-3 mr-1" />
                {t.scanTemplate.webscan.fingerprintScan}
              </TabsTrigger>
              <TabsTrigger value="2" className="text-xs">
                <Scan className="w-3 h-3 mr-1" />
                {t.scanTemplate.webscan.fullFingerprintScan}
              </TabsTrigger>
              <TabsTrigger value="3" className="text-xs">
                <Shield className="w-3 h-3 mr-1" />
                {t.scanTemplate.webscan.vulnScan}
              </TabsTrigger>
              <TabsTrigger value="4" className="text-xs">
                <Target className="w-3 h-3 mr-1" />
                {t.scanTemplate.webscan.specialScan}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>{t.scanTemplate.webscan.threads}</Label>
            <Input
              type="number"
              min="1"
              max="100"
              value={config.threads}
              onChange={(e) => updateConfig({ threads: parseInt(e.target.value) || 30 })}
            />
          </div>
          <div className="space-y-2">
            <Label>{t.scanTemplate.webscan.nucleiThreads}</Label>
            <Input
              type="number"
              min="1"
              max="100"
              value={config.nucleiThreads ?? 2}
              onChange={(e) => updateConfig({ nucleiThreads: parseInt(e.target.value) || 2 })}
            />
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>{t.scanTemplate.webscan.activeTimeoutLimit}</Label>
            <Input
              type="number"
              min="1"
              max="10"
              value={config.activeTimeoutLimit}
              onChange={(e) => updateConfig({ activeTimeoutLimit: parseInt(e.target.value) || 3 })}
            />
          </div>
        </div>

        {config.webscanOption === 4 && (
          <div className="space-y-2">
            <Label>{t.scanTemplate.webscan.customTemplate}</Label>
            <SelectTag
              options={templateOptions}
              value={config.customTemplate || []}
              onChange={(selected) => updateConfig({ customTemplate: selected })}
              placeholder={t.scanTemplate.webscan.selectTemplate}
            />
          </div>
        )}

        <div className="space-y-2">
          <Label>{t.scanTemplate.webscan.customHeaders}</Label>
          <Textarea
            value={config.customHeaders}
            onChange={(e) => updateConfig({ customHeaders: e.target.value })}
            placeholder="Header-Name: Header-Value"
            className="font-mono text-sm min-h-[80px]"
          />
        </div>

        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.webscan.disableMaxHostErr}</Label>
            <Switch
              checked={config.disableMaxHostErr === true}
              onCheckedChange={(checked) => updateConfig({ disableMaxHostErr: checked })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.webscan.rootPathScan}</Label>
            <Switch
              checked={config.rootPathScan}
              onCheckedChange={(checked) => updateConfig({ rootPathScan: checked })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.webscan.skipNucleiWithoutTags}</Label>
            <Switch
              checked={config.skipNucleiWithoutTags}
              onCheckedChange={(checked) => updateConfig({ skipNucleiWithoutTags: checked })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.webscan.skipNucleiWithWAF}</Label>
            <Switch
              checked={config.skipNucleiWithWAF !== false}
              onCheckedChange={(checked) => updateConfig({ skipNucleiWithWAF: checked })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.webscan.screenshot}</Label>
            <Switch
              checked={config.screenshot}
              onCheckedChange={(checked) => updateConfig({ screenshot: checked })}
            />
          </div>
          {config.screenshot && showScreenshotSnapChromeWarning && (
            <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <p>{t.scanTemplate.webscan.screenshotSnapChromeWarning}</p>
            </div>
          )}
        </div>
      </div>
    )
  }

  // 端口扫描配置
  const renderPortScanConfig = () => {
    const config = formData.config as PortScanConfig
    const updateConfig = (updates: Partial<PortScanConfig>) => {
      setFormData({ ...formData, config: { ...config, ...updates } })
    }
    const currentCustomGroup = customPortGroups.find((group) => group.text === config.portGroup) || null
    const selectedPortGroup = findPortGroupByText(portGroupOptions, config.portGroup) || null

    const handleDeleteCustomPortGroup = async (group: PortGroupOption) => {
      try {
        setDeletingPortGroupId(group.id)
        const response = await fetchWithAuth(`/api/v1/config/port-groups/${group.id}`, {
          method: 'DELETE',
        })
        const data = await response.json()
        if (!response.ok || !data.success) {
          throw new Error(data.error || '删除端口组失败')
        }

        setCustomPortGroups((prev) => prev.filter((item) => item.id !== group.id))
        if (defaultPortGroupId === group.id) {
          setDefaultPortGroupId(BUILTIN_TOP1000_PORT_GROUP_ID)
        }
        if (config.portGroup === group.text) {
          const fallback = builtInPortGroupOptions[0]
          updateConfig({ portGroup: fallback.text, portRange: fallback.value })
        }
        toast.success('端口组已删除')
      } catch (error: any) {
        toast.error(error.message || '删除端口组失败')
      } finally {
        setDeletingPortGroupId(null)
      }
    }

    return (
      <div className="space-y-4">
        <div className="space-y-2">
          <Label>{t.scanTemplate.portscan.portRange}</Label>
          <div className="space-y-3">
            <Select
              value={config.portGroup}
              onValueChange={(value) => {
                if (value === '__create_custom_group__') {
                  setNewPortGroupName(currentCustomGroup?.text || '')
                  setNewPortGroupPorts(config.portRange)
                  setShowPortGroupDialog(true)
                  return
                }

                const option = findPortGroupByText(portGroupOptions, value)
                if (option) {
                  updateConfig({
                    portGroup: option.text,
                    portRange: option.value,
                  })
                } else {
                  updateConfig({ portGroup: value })
                }
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue placeholder={t.scanTemplate.portscan.selectPortGroup} />
              </SelectTrigger>
              <SelectContent>
                {portGroupOptions.map((option) => (
                  <SelectItem key={option.id} value={option.text}>
                    <div className="flex w-full items-center justify-between gap-2">
                      <span>{option.text}</span>
                      {!option.builtIn && (
                        <Badge variant="secondary" className="h-5 px-1.5 text-[10px] bg-orange-500/10 text-orange-600 border border-orange-500/20">
                          自定义
                        </Badge>
                      )}
                    </div>
                  </SelectItem>
                ))}
                <SelectItem value="__create_custom_group__">
                  <div className="flex items-center gap-2 text-primary">
                    <Plus className="h-4 w-4" />
                    <span>新增端口组</span>
                  </div>
                </SelectItem>
              </SelectContent>
            </Select>
            <Textarea
              value={config.portRange}
              onChange={(e) => {
                const nextValue = e.target.value
                const matchedOption = findPortGroupByValue(portGroupOptions, nextValue)
                const selectedOption = findPortGroupByText(portGroupOptions, config.portGroup)
                if (matchedOption) {
                  updateConfig({ portRange: nextValue, portGroup: matchedOption.text })
                } else if (selectedOption?.builtIn && selectedOption.value === '') {
                  updateConfig({ portRange: nextValue, portGroup: selectedOption.text })
                } else if (currentCustomGroup) {
                  updateConfig({ portRange: nextValue, portGroup: currentCustomGroup.text })
                } else {
                  updateConfig({ portRange: nextValue, portGroup: '' })
                }
              }}
              placeholder={t.scanTemplate.portscan.portPlaceholder}
              className="min-h-[92px] max-h-[180px] overflow-y-auto resize-y font-mono"
            />
          </div>
          {currentCustomGroup && (
            <div className="flex items-center justify-between gap-3 rounded-lg border border-orange-500/20 bg-orange-500/5 px-3 py-2">
              <div className="flex min-w-0 items-center gap-2">
                <span className="truncate text-sm font-medium">{currentCustomGroup.text}</span>
                <Badge variant="secondary" className="h-5 px-1.5 text-[10px] bg-orange-500/10 text-orange-600 border border-orange-500/20">
                  自定义
                </Badge>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8 px-2 text-destructive"
                disabled={deletingPortGroupId === currentCustomGroup.id}
                onClick={() => void handleDeleteCustomPortGroup(currentCustomGroup)}
              >
                <Trash2 className="h-4 w-4 mr-1" />
                删除
              </Button>
            </div>
          )}
          {selectedPortGroup && (
            <div className="flex items-center justify-between gap-3 rounded-lg border border-border bg-muted/30 px-3 py-2">
              <div className="min-w-0">
                <div className="flex items-center gap-2">
                  <span className="truncate text-sm font-medium">{selectedPortGroup.text}</span>
                  {defaultPortGroupId === selectedPortGroup.id && (
                    <Badge variant="outline" className="h-5 px-1.5 text-[10px] border-amber-500/40 bg-amber-500/10 text-amber-700 dark:text-amber-300">
                      系统默认
                    </Badge>
                  )}
                </div>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  AI 和 MCP 未明确指定端口时使用此端口组。
                </p>
              </div>
              {defaultPortGroupId !== selectedPortGroup.id && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="shrink-0"
                  disabled={savingDefaultPortGroupId === selectedPortGroup.id}
                  onClick={() => void handleSetDefaultPortGroup(selectedPortGroup)}
                >
                  设为默认
                </Button>
              )}
            </div>
          )}
          {loadingPortGroups && (
            <p className="text-xs text-muted-foreground">正在加载自定义端口组...</p>
          )}
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>{t.scanTemplate.portscan.threads}</Label>
            <Input
              type="number"
              min="1"
              max="10000"
              value={config.threads}
              onChange={(e) => updateConfig({ threads: parseInt(e.target.value) || 500 })}
            />
          </div>
          <div className="space-y-2">
            <Label>{t.scanTemplate.portscan.timeout}</Label>
            <Input
              type="number"
              min="1"
              max="300"
              value={config.timeout}
              onChange={(e) => updateConfig({ timeout: parseInt(e.target.value) || 3 })}
            />
          </div>
        </div>

        <HostDiscoverySettings
          aliveDetection={config.aliveDetection}
          onAliveDetectionChange={(aliveDetection) => updateConfig({ aliveDetection })}
          value={normalizeTCPHeuristicConfig(config)}
          onChange={updateConfig}
          portRange={config.portRange}
        />
      </div>
    )
  }

  // 目录扫描配置
  const renderDirScanConfig = () => {
    const config = formData.config as DirScanConfig
    const updateConfig = (updates: Partial<DirScanConfig>) => {
      setFormData({ ...formData, config: { ...config, ...updates } })
    }

    return (
      <div className="space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="space-y-2">
            <Label>{t.scanTemplate.dirscan.method}</Label>
            <Select value={config.method} onValueChange={(v) => updateConfig({ method: v })}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {['GET', 'POST', 'HEAD', 'OPTIONS'].map((m) => (
                  <SelectItem key={m} value={m}>{m}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>{t.scanTemplate.dirscan.statusFilter}</Label>
            <Input
              value={config.statusFilter}
              onChange={(e) => updateConfig({ statusFilter: e.target.value })}
              placeholder="200,301,302,403"
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label>{t.scanTemplate.dirscan.extensions}</Label>
          <Input
            value={config.extensions}
            onChange={(e) => updateConfig({ extensions: e.target.value })}
            placeholder="php,asp,jsp,html"
          />
        </div>

        <div className="space-y-2">
          <Label>{t.scanTemplate.dirscan.selectDict}</Label>
          <SelectTag
            options={dictOptions}
            value={config.selectedDicts || []}
            onChange={(selected) => updateConfig({ selectedDicts: selected })}
            placeholder={t.scanTemplate.dirscan.selectDictFile}
          />
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="space-y-2">
            <Label>{t.scanTemplate.dirscan.threads}</Label>
            <Input
              type="number"
              value={config.thread}
              onChange={(e) => updateConfig({ thread: parseInt(e.target.value) || 50 })}
            />
          </div>
          <div className="space-y-2">
            <Label>{t.scanTemplate.dirscan.timeout}</Label>
            <Input
              type="number"
              value={config.timeout}
              onChange={(e) => updateConfig({ timeout: parseInt(e.target.value) || 10 })}
            />
          </div>
          <div className="space-y-2">
            <Label>{t.scanTemplate.dirscan.interval}</Label>
            <Input
              type="number"
              value={config.interval}
              onChange={(e) => updateConfig({ interval: parseInt(e.target.value) || 0 })}
            />
          </div>
        </div>

        <div className="space-y-2">
          <Label>{t.scanTemplate.dirscan.customHeaders}</Label>
          <Textarea
            value={config.headers}
            onChange={(e) => updateConfig({ headers: e.target.value })}
            placeholder="Header-Name: Header-Value"
            className="font-mono text-sm min-h-[60px]"
          />
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.dirscan.followRedirect}</Label>
            <Switch
              checked={config.redirectClient}
              onCheckedChange={(checked) => updateConfig({ redirectClient: checked })}
            />
          </div>
          <div className="flex items-center justify-between">
            <Label>{t.scanTemplate.dirscan.backupFileScan}</Label>
            <Switch
              checked={config.backupfileScan}
              onCheckedChange={(checked) => updateConfig({ backupfileScan: checked })}
            />
          </div>
        </div>
      </div>
    )
  }

  // 弱口令破解配置
  const renderCrackScanConfig = () => {
    const config = formData.config as CrackScanConfig
    const updateConfig = (updates: Partial<CrackScanConfig>) => {
      setFormData({ ...formData, config: { ...config, ...updates } })
    }

    return (
      <div className="space-y-4">
        <div className="space-y-2">
          <Label>{t.scanTemplate.crack.usernameDict}</Label>
          <Textarea
            value={config.username}
            onChange={(e) => {
              updateConfig({ username: e.target.value })
              if (e.target.value.length === 0) {
                updateConfig({ defaultDict: true })
              }
            }}
            placeholder={t.scanTemplate.crack.usernamePlaceholder}
            className="font-mono text-sm min-h-[100px]"
          />
        </div>

        <div className="space-y-2">
          <Label>{t.scanTemplate.crack.passwordDict}</Label>
          <Textarea
            value={config.password}
            onChange={(e) => {
              updateConfig({ password: e.target.value })
              if (e.target.value.length === 0) {
                updateConfig({ defaultDict: true })
              }
            }}
            placeholder={t.scanTemplate.crack.passwordPlaceholder}
            className="font-mono text-sm min-h-[100px]"
          />
          <div className="flex items-center space-x-2">
            <Checkbox
              id="useSystemDict"
              checked={config.defaultDict}
              onCheckedChange={(checked) => updateConfig({ defaultDict: checked === true })}
              disabled={config.password.length > 0}
            />
            <Label htmlFor="useSystemDict" className="text-sm font-normal cursor-pointer">
              {t.scanTemplate.crack.useSystemDict}
            </Label>
          </div>
        </div>
      </div>
    )
  }

  // JS扫描配置
  const renderJsScanConfig = () => {
    const config = formData.config as JsScanConfig
    const updateConfig = (updates: Partial<JsScanConfig>) => {
      setFormData({ ...formData, config: { ...config, ...updates } })
    }

    return (
      <div className="space-y-4">
        {/* AI 检测选项 */}
        <div className="flex items-center justify-between p-3 border rounded-lg bg-gradient-to-r from-purple-500/5 to-blue-500/5">
          <div className="space-y-0.5">
            <Label className="text-sm font-medium">{t.scanTemplate.jsscan.aiDetection}</Label>
            <p className="text-xs text-muted-foreground">
              {t.scanTemplate.jsscan.aiDetectionDesc}
            </p>
          </div>
          <Switch
            checked={config.aiDetectionEnabled}
            onCheckedChange={(checked) => updateConfig({ aiDetectionEnabled: checked })}
          />
        </div>

        <div className="flex items-center justify-between rounded-lg border p-3">
          <div className="space-y-0.5">
            <Label className="text-sm font-medium">{t.scanTemplate.jsscan.vulnDetection}</Label>
          </div>
          <Switch
            checked={config.sqlInjectionEnabled}
            onCheckedChange={(checked) => updateConfig({ sqlInjectionEnabled: checked })}
          />
        </div>


        {/* 占位符配置 */}
        <div className="space-y-2">
          <Label>{t.scanTemplate.jsscan.placeholder}</Label>
          <p className="text-xs text-muted-foreground">
            {t.scanTemplate.jsscan.placeholderDesc}
          </p>
          <div className="space-y-2 mt-2">
            {Object.entries(config.placeholder || {}).map(([key, value]) => (
              <div key={key} className="flex items-center gap-2">
                <Input
                  value={key}
                  onChange={(e) => {
                    const newPlaceholder = { ...config.placeholder }
                    delete newPlaceholder[key]
                    newPlaceholder[e.target.value] = value
                    updateConfig({ placeholder: newPlaceholder })
                  }}
                  placeholder={t.scanTemplate.jsscan.keyName}
                  className="text-xs font-mono w-28"
                />
                <span className="text-muted-foreground">=</span>
                <Input
                  value={value}
                  onChange={(e) => updateConfig({ placeholder: { ...config.placeholder, [key]: e.target.value } })}
                  placeholder={t.scanTemplate.jsscan.valueName}
                  className="text-xs font-mono flex-1"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-destructive hover:text-destructive"
                  onClick={() => {
                    const newPlaceholder = { ...config.placeholder }
                    delete newPlaceholder[key]
                    updateConfig({ placeholder: newPlaceholder })
                  }}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => {
                const newKey = `param_${Object.keys(config.placeholder || {}).length + 1}`
                updateConfig({ placeholder: { ...config.placeholder, [newKey]: '' } })
              }}
              className="w-full"
            >
              <Plus className="h-4 w-4 mr-2" />
              {t.scanTemplate.jsscan.addPlaceholder}
            </Button>
          </div>
        </div>

        {/* 域名黑名单 */}
        <div className="space-y-2">
          <Label>{t.scanTemplate.jsscan.blackDomain}</Label>
          <Textarea
            value={config.blackDomain?.join('\n') || ''}
            onChange={(e) => updateConfig({ blackDomain: e.target.value.split('\n').filter(Boolean) })}
            placeholder={t.scanTemplate.jsscan.blackDomainPlaceholder}
            className="font-mono text-sm min-h-[80px]"
          />
        </div>

        {/* 高危路由 */}
        <div className="space-y-2">
          <Label>{t.scanTemplate.jsscan.highRiskRouter}</Label>
          <Textarea
            value={config.highRiskRouter?.join('\n') || ''}
            onChange={(e) => updateConfig({ highRiskRouter: e.target.value.split('\n').filter(Boolean) })}
            placeholder={t.scanTemplate.jsscan.highRiskRouterPlaceholder}
            className="font-mono text-sm min-h-[80px]"
          />
        </div>

        {/* 认证失败关键词 */}
        <div className="space-y-2">
          <Label>{t.scanTemplate.jsscan.authentication}</Label>
          <Textarea
            value={config.authentication?.join('\n') || ''}
            onChange={(e) => updateConfig({ authentication: e.target.value.split('\n').filter(Boolean) })}
            placeholder={t.scanTemplate.jsscan.authenticationPlaceholder}
            className="font-mono text-sm min-h-[80px]"
          />
        </div>
      </div>
    )
  }

  // 获取配置摘要
  const getConfigSummary = (template: ScanTemplate) => {
    const config = template.config
    const badges: string[] = []

    switch (template.taskType) {
      case 'webscan': {
        const c = config as WebScanConfig
        const modes = ['', t.scanTemplate.configBadges.fingerprint, t.scanTemplate.configBadges.fullFingerprint, t.scanTemplate.configBadges.fingerprintVuln, t.scanTemplate.configBadges.special]
        badges.push(modes[c.webscanOption] || t.scanTemplate.configBadges.default)
        badges.push(`${c.threads}${t.scanTemplate.configBadges.threads}`)
        if (c.disableMaxHostErr) badges.push(t.scanTemplate.configBadges.disableMaxHostErr)
        if (c.rootPathScan) badges.push(t.scanTemplate.configBadges.rootPath)
        if (c.screenshot) badges.push(t.scanTemplate.configBadges.screenshot)
        break
      }
      case 'portscan': {
        const c = config as PortScanConfig
        badges.push(c.portGroup || t.scanTemplate.configBadges.customPort)
        badges.push(`${c.threads}${t.scanTemplate.configBadges.threads}`)
        if (c.aliveDetection) badges.push(t.scanTemplate.configBadges.aliveDetection)
        break
      }
      case 'dirsearch': {
        const c = config as DirScanConfig
        badges.push(c.method)
        badges.push(`${c.thread}${t.scanTemplate.configBadges.threads}`)
        if (c.selectedDicts?.length) badges.push(`${c.selectedDicts.length}${t.scanTemplate.configBadges.dicts}`)
        if (c.backupfileScan) badges.push(t.scanTemplate.configBadges.backupScan)
        break
      }
      case 'crack': {
        const c = config as CrackScanConfig
        if (c.defaultDict) badges.push(t.scanTemplate.configBadges.defaultDict)
        else badges.push(t.scanTemplate.configBadges.customDict)
        break
      }
      case 'jsscan': {
        const c = config as JsScanConfig
        if (c.aiDetectionEnabled) badges.push(t.scanTemplate.configBadges.aiDetection)
        const enabledDetects = [
          c.sqlInjectionEnabled && 'SQL',
          c.lfiEnabled && 'LFI',
          c.ssrfEnabled && 'SSRF',
          c.redirectEnabled && 'Redirect',
          c.xssEnabled && 'XSS',
          c.uploadEnabled && 'Upload',
        ].filter(Boolean)
        badges.push(`${enabledDetects.length}${t.scanTemplate.configBadges.detections}`)
        if (Object.keys(c.placeholder || {}).length) badges.push(`${Object.keys(c.placeholder).length}${t.scanTemplate.configBadges.placeholders}`)
        break
      }
      case 'combination': {
        const c = config as CombinationTemplateConfig
        const nodes = Array.isArray(c.nodes) ? c.nodes : []
        const stageCount = nodes.reduce((max, node) => Math.max(max, (node.stageIndex ?? 0) + 1), 0)
        badges.push(`${stageCount}${t.scanTemplate.configBadges.stages}`)
        badges.push(`${nodes.length}${t.scanTemplate.configBadges.modules}`)
        badges.push(c.scanMode === 'agent' ? t.scanTemplate.configBadges.agentMode : c.scanMode === 'local' ? t.scanTemplate.configBadges.localMode : t.scanTemplate.configBadges.autoMode)
        if (c.schedule?.spec?.kind) {
          badges.push(`${t.scanTemplate.configBadges.schedulePrefix}${c.schedule.spec.kind}`)
        }
        break
      }
      case 'ai_dialogue': {
        const c = config as AIDialogueTemplateConfig
        badges.push(`${(c.prompt || '').length}${t.scanTemplate.configBadges.characters}`)
        break
      }
    }

    return badges
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col items-start justify-between gap-3 sm:flex-row sm:items-center">
        <div>
          <h3 className="text-lg font-semibold">{t.scanTemplate.title}</h3>
          <p className="text-sm text-muted-foreground mt-1">
            {t.scanTemplate.description}
          </p>
        </div>
        <Button onClick={() => handleOpenDialog()}>
          <Plus className="w-4 h-4 mr-2" />
          {t.scanTemplate.create}
        </Button>
      </div>

      {/* 任务类型过滤 */}
      <div className="flex items-center gap-2">
        <span className="text-sm text-muted-foreground">{t.scanTemplate.filter}:</span>
        <Select value={filterTaskType} onValueChange={(v) => setFilterTaskType(v as TaskType | 'all')}>
          <SelectTrigger className="w-[160px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t.scanTemplate.filterAll}</SelectItem>
            {(Object.keys(taskTypeInfo) as TaskType[]).map((type) => {
              const info = taskTypeInfo[type]
              const Icon = info.icon
              return (
                <SelectItem key={type} value={type}>
                  <div className="flex items-center gap-2">
                    <Icon className={`w-4 h-4 ${info.color}`} />
                    {t.scanTemplate.taskTypes[type]}
                  </div>
                </SelectItem>
              )
            })}
          </SelectContent>
        </Select>
      </div>

      {loading ? (
        <div className="text-center py-8 text-muted-foreground">{t.scanTemplate.loading}</div>
      ) : filteredTemplates.length === 0 ? (
        <div className="py-12 text-center border rounded-lg">
          <FileText className="w-12 h-12 mx-auto text-muted-foreground mb-4" />
          <h3 className="text-lg font-semibold mb-2">{t.scanTemplate.noTemplates}</h3>
          <p className="text-sm text-muted-foreground mb-4">
            {t.scanTemplate.noTemplatesDesc}
          </p>
          <Button onClick={() => handleOpenDialog()}>
            <Plus className="w-4 h-4 mr-2" />
            {t.scanTemplate.createFirst}
          </Button>
        </div>
      ) : (
        <div>
          <div className="space-y-3 md:hidden">
            {filteredTemplates.map((template) => (
              <article key={template.id} className="space-y-3 rounded-lg border bg-card p-3">
                <h3 className="break-words font-medium">{template.name}</h3>
                {template.description ? <p className="break-words text-sm text-muted-foreground">{template.description}</p> : null}
                <Badge variant="outline">{t.scanTemplate.taskTypes[template.taskType]}</Badge>
                <div className="flex flex-wrap gap-1">{getConfigSummary(template).map((badge, index) => <Badge key={index} variant="secondary" className="max-w-full whitespace-normal break-all">{badge}</Badge>)}</div>
                <div className="flex justify-end gap-2 border-t pt-2">
                  <Button variant="outline" onClick={() => handleOpenDialog(template)}><Edit className="size-4" />{t.common.edit}</Button>
                  <Button variant="ghost" className="text-destructive" onClick={() => template.id && setDeleteTemplateId(template.id)}><Trash2 className="size-4" />{t.common.delete}</Button>
                </div>
              </article>
            ))}
          </div>
          <div className="hidden overflow-hidden rounded-lg border md:block"><Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t.scanTemplate.table.name}</TableHead>
                <TableHead>{t.scanTemplate.table.description}</TableHead>
                <TableHead>{t.scanTemplate.table.taskType}</TableHead>
                <TableHead>{t.scanTemplate.table.configSummary}</TableHead>
                <TableHead className="text-center">{t.scanTemplate.table.action}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredTemplates.map((template) => {
                const typeInfo = taskTypeInfo[template.taskType] || taskTypeInfo.webscan
                const Icon = typeInfo.icon
                return (
                  <TableRow key={template.id}>
                    <TableCell className="font-medium">{template.name}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {template.description || '-'}
                    </TableCell>
                    <TableCell>
                      <Badge variant="outline" className="gap-1">
                        <Icon className={`w-3 h-3 ${typeInfo.color}`} />
                        {t.scanTemplate.taskTypes[template.taskType]}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-1">
                        {getConfigSummary(template).map((badge, i) => (
                          <Badge key={i} variant="secondary" className="text-xs">
                            {badge}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <div className="flex items-center justify-center gap-2">
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleOpenDialog(template)}
                        >
                          <Edit className="w-4 h-4" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => template.id && setDeleteTemplateId(template.id)}
                        >
                          <Trash2 className="w-4 h-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table></div>
        </div>
      )}

      {/* 编辑/创建对话框 */}
      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="settings-dialog sm:max-w-2xl max-h-[90dvh] flex flex-col p-4 sm:p-6">
          <DialogHeader className="flex-shrink-0">
            <DialogTitle>{editingTemplate ? t.scanTemplate.dialog.edit : t.scanTemplate.dialog.create}</DialogTitle>
            <DialogDescription>
              {editingTemplate ? t.scanTemplate.dialog.editDesc : t.scanTemplate.dialog.createDesc}
            </DialogDescription>
          </DialogHeader>

          <div className="min-h-0 flex-1 overflow-y-auto pr-1">
            <div className="space-y-4 py-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="name">{t.scanTemplate.dialog.templateName} *</Label>
                  <Input
                    id="name"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    placeholder={t.scanTemplate.dialog.templateNamePlaceholder}
                  />
                </div>
                <div className="space-y-2">
                  <Label>{t.scanTemplate.dialog.taskType}</Label>
                  <Select
                    value={formData.taskType}
                    onValueChange={(v) => handleTaskTypeChange(v as TaskType)}
                    disabled={!!editingTemplate}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {((editingTemplate ? Object.keys(taskTypeInfo) : creatableTaskTypes) as TaskType[]).map((type) => {
                        const info = taskTypeInfo[type]
                        const Icon = info.icon
                        return (
                          <SelectItem key={type} value={type}>
                            <div className="flex items-center gap-2">
                              <Icon className={`w-4 h-4 ${info.color}`} />
                              {t.scanTemplate.taskTypes[type]}
                            </div>
                          </SelectItem>
                        )
                      })}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="description">{t.scanTemplate.dialog.templateDesc}</Label>
                <Input
                  id="description"
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  placeholder={t.scanTemplate.dialog.templateDescPlaceholder}
                />
              </div>

              <div className="border-t pt-4">
                <h4 className="text-sm font-medium mb-4">
                  {t.scanTemplate.taskTypes[formData.taskType] || formData.taskType} {t.scanTemplate.dialog.configTitle}
                </h4>
                {renderConfigForm()}
              </div>
            </div>
          </div>

          <DialogFooter className="flex-shrink-0 pt-4 border-t">
            <Button variant="outline" onClick={handleCloseDialog}>
              {t.scanTemplate.cancel}
            </Button>
            <Button onClick={handleSave}>
              {editingTemplate ? t.scanTemplate.update : t.scanTemplate.save}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={showPortGroupDialog}
        onOpenChange={(open) => {
          setShowPortGroupDialog(open)
          if (!open) {
            setNewPortGroupName('')
            setNewPortGroupPorts('')
          }
        }}
      >
        <DialogContent className="settings-dialog overflow-y-auto">
          <DialogHeader>
            <DialogTitle>新增自定义端口组</DialogTitle>
            <DialogDescription>
              输入自定义端口组名称和端口范围，保存后会持久化到系统配置。
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="template-custom-port-group-name">端口组名称</Label>
              <Input
                id="template-custom-port-group-name"
                value={newPortGroupName}
                onChange={(e) => setNewPortGroupName(e.target.value)}
                placeholder="例如：中间件端口"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="template-custom-port-group-ports">端口范围</Label>
              <Textarea
                id="template-custom-port-group-ports"
                value={newPortGroupPorts}
                onChange={(e) => setNewPortGroupPorts(e.target.value)}
                placeholder="例如：7001,8080,8081,8443,9000-9010"
                className="min-h-[92px] max-h-[180px] overflow-y-auto resize-y font-mono break-all whitespace-pre-wrap [overflow-wrap:anywhere]"
              />
            </div>
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setShowPortGroupDialog(false)}
                disabled={savingPortGroup}
              >
                取消
              </Button>
              <Button
                type="button"
                onClick={() => void (async () => {
                  if (!newPortGroupName.trim()) {
                    toast.error('请输入端口组名称')
                    return
                  }
                  if (!newPortGroupPorts.trim()) {
                    toast.error('请输入端口范围')
                    return
                  }

                  try {
                    setSavingPortGroup(true)
                    const response = await fetchWithAuth('/api/v1/config/port-groups', {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({
                        name: newPortGroupName.trim(),
                        ports: newPortGroupPorts.trim(),
                      }),
                    })
                    const data = await response.json()
                    if (!response.ok || !data.success) {
                      throw new Error(data.error || '保存端口组失败')
                    }

                    const savedGroup: PortGroupOption = {
                      id: String(data.data?.id || ''),
                      text: String(data.data?.name || ''),
                      value: String(data.data?.ports || ''),
                    }

                    setCustomPortGroups((prev) => {
                      const next = prev.filter((item) => item.id !== savedGroup.id)
                      next.push(savedGroup)
                      return next.sort((a, b) => a.text.localeCompare(b.text, 'zh-CN'))
                    })

                    if (formData.taskType === 'portscan') {
                      const currentConfig = formData.config as PortScanConfig
                      setFormData({
                        ...formData,
                        config: {
                          ...currentConfig,
                          portGroup: savedGroup.text,
                          portRange: savedGroup.value,
                        },
                      })
                    }

                    setNewPortGroupName('')
                    setNewPortGroupPorts('')
                    setShowPortGroupDialog(false)
                    toast.success('端口组已保存')
                  } catch (error: any) {
                    toast.error(error.message || '保存端口组失败')
                  } finally {
                    setSavingPortGroup(false)
                  }
                })()}
                disabled={savingPortGroup}
              >
                {savingPortGroup ? '保存中...' : '保存端口组'}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
      <AlertDialog open={!!deleteTemplateId} onOpenChange={(open) => !open && setDeleteTemplateId(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.common.delete}</AlertDialogTitle>
            <AlertDialogDescription>{t.scanTemplate.confirmDelete}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t.common.cancel}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteTemplateId && handleDelete(deleteTemplateId)}
            >
              {t.common.delete}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
