'use client'

import React, { useCallback, useEffect, useState, Suspense } from 'react'
import { DEFAULT_TCP_HEURISTIC_CONFIG, normalizeTCPHeuristicConfig, validateTCPHeuristicConfig } from '@/lib/tcp-heuristic'
import { useI18n } from '@/lib/i18n'
import { fetchWithAuth } from '@/lib/api'
import toast from '@/lib/toast'
import { SimpleMultiSelect, type MultiSelectOption } from '@/components/ui/simple-multi-select'
import { getProxyUrl } from '@/lib/proxy'
import { getSpaceEngineConfig, type SpaceEngineConfig } from '@/lib/config'
import { getAIConfig, type AIConfig } from '@/lib/api/ai-config'
import type {
  PortScanTask,
  SubdomainScanTask,
  SubdomainScanConfig,
  WebScanConfig,
  PortScanConfig,
  SYNCapabilityStatus,
  CrackScanConfig,
  DirScanConfig,
  JsScanConfig,
  ScheduleConfig,
  TCPDiscoveryMode,
} from '@/app/(main)/scan/vuln-scan/types'
import { ScanType } from '@/app/(main)/scan/vuln-scan/types'
import type { MouseEvent } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { TextInputWithActions } from '@/components/ui/text-input-with-actions'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { ArrowLeft, Download, Loader2, RefreshCw } from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { WebScanSettings } from './components/WebScanSettings'
import { PortScanSettings } from './components/PortScanSettings'
import { CrackScanSettings } from './components/CrackScanSettings'
import { JsScanSettings, normalizeSQLRules } from './components/JsScanSettings'
import { DirScanSettings } from './components/DirScanSettings'
import { TaskScheduleSettings } from './components/TaskScheduleSettings'
import { createDefaultSchedule } from '../utils/task-schedule'
import type { DictionaryMeta } from '../types'
import { CombinationTaskInline } from '../components/CombinationTaskInline'
import type { CombinationDraft } from '../components/CombinationTaskBuilder'
import { ASSET_SCAN_COMBINATION_DRAFT_KEY } from '@/lib/combination-presets'
import { SubdomainConfigPanel } from '../components/SubdomainConfigPanel'
import {
  getSubdomainDNSServerSettings,
  isSubdomainEngineConfigured,
  normalizeDNSServersFromResponse,
  DEFAULT_SUBDOMAIN_ENGINES,
  SPACE_ENGINES,
  SPACE_ENGINE_NAMES,
  cloneDefaultDNSServers,
  getDefaultSubdomainEngines,
  isLegacyDefaultSubdomainEngineSelection,
  type DNSServerSetting,
  type SpaceEngineName,
} from '../components/subdomain-config'
import { BUILTIN_DATABASE_PORT_GROUP_TEXT, BUILTIN_DATABASE_PORT_GROUP_VALUE } from '../utils/portGroup'
import { isDesktopRuntime } from '@/lib/desktop'
import { validateSubdomainTargets } from '@/lib/subdomain-targets'

type RunMode = 'auto' | 'local' | 'agent'

const SUBDOMAIN_SELECTED_ENGINES_STORAGE_KEY = 'subdomain_selected_engines'
const IMPORTABLE_PORT_SCAN_STATUSES = new Set<PortScanTask['status']>(['completed', 'failed', 'cancelled'])
const IMPORTABLE_SUBDOMAIN_TASK_STATUSES = new Set<SubdomainScanTask['status']>(['completed', 'failed', 'cancelled'])

type DNSServerSettings = {
  servers: DNSServerSetting[]
}

interface AgentNodeOption {
  scanWorker?: boolean
  localWorker?: boolean
  agentId: string
  displayName: string
  enabled: boolean
  isAlive: boolean
  canDispatch: boolean
}

const hasUsableAIConfig = (config: AIConfig | null | undefined) => {
  return Boolean(config?.api?.trim() && config?.key?.trim() && config?.model?.trim())
}

function NewScanForm() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const scanModeParam = searchParams.get('type')
  const plannerMode = searchParams.get('planner')

  useEffect(() => {
    if (scanModeParam === 'combination' && plannerMode === 'smart') {
      router.replace('/scan/vuln-scan/new?type=combination')
    }
  }, [plannerMode, router, scanModeParam])

  const { t } = useI18n()
  const formatText = (template: string, values: Record<string, string | number>) =>
    Object.entries(values).reduce((text, [key, value]) => text.replaceAll(`{${key}}`, String(value)), template)
  // scanMode is now a ScanType string enum (e.g. 'webscan')
  let scanMode: ScanType = ScanType.WebScan
  if (scanModeParam && (Object.values(ScanType) as string[]).includes(scanModeParam)) {
    scanMode = scanModeParam as ScanType
  }

  const urlsParam = searchParams.get('urls')

  const [taskName, setTaskName] = useState('')
  const [targets, setTargets] = useState(urlsParam ? decodeURIComponent(urlsParam) : '')
  const [threads, setThreads] = useState(scanMode === ScanType.Subdomain ? 600 : scanMode === ScanType.WebScan ? 30 : scanMode === ScanType.PortScan ? 1000 : 10)
  const [subdomainTimeout, setSubdomainTimeout] = useState(5)
  const [selectedSubdomainEngines, setSelectedSubdomainEngines] = useState<SpaceEngineName[]>(() => [...DEFAULT_SUBDOMAIN_ENGINES])
  const [subdomainDnsServers, setSubdomainDnsServers] = useState<DNSServerSetting[]>(() => cloneDefaultDNSServers())
  const [isSavingSubdomainDnsServers, setIsSavingSubdomainDnsServers] = useState(false)
  const [spaceEngineConfig, setSpaceEngineConfig] = useState<SpaceEngineConfig | null>(null)
  // 网站扫描特有配置
  const [webscanOption, setWebscanOption] = useState<1 | 2 | 3 | 4>(3) // 默认指纹漏洞扫描
  const [nucleiThreads, setNucleiThreads] = useState(2)
  const [activeTimeoutLimit, setActiveTimeoutLimit] = useState(5) // 主动指纹最多失败次数
  const [disableMaxHostErr, setDisableMaxHostErr] = useState(false)
  const [customHeaders, setCustomHeaders] = useState('')
  const [customTags, setCustomTags] = useState<string[]>([])
  const [customTemplate, setCustomTemplate] = useState<string[]>([])
  const [rootPathScan, setRootPathScan] = useState(true)
  const [skipNucleiWithoutTags, setSkipNucleiWithoutTags] = useState(false)
  const [skipNucleiWithWAF, setSkipNucleiWithWAF] = useState(true)
  const [screenshot, setScreenshot] = useState(true)
  const [oobEnabled, setOobEnabled] = useState(true)
  const [interactshURL, setInteractshURL] = useState('')
  const [interactshToken, setInteractshToken] = useState('')
  // 密码暴破特有配置
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [defaultDict, setDefaultDict] = useState(true)
  // 端口扫描特有配置
  const [portRange, setPortRange] = useState(BUILTIN_DATABASE_PORT_GROUP_VALUE) // 默认数据库端口
  const [portGroup, setPortGroup] = useState(BUILTIN_DATABASE_PORT_GROUP_TEXT)
  const [portTimeout, setPortTimeout] = useState(10)
  const [tcpHeuristic, setTcpHeuristic] = useState({ ...DEFAULT_TCP_HEURISTIC_CONFIG })
  const [aliveDetection, setAliveDetection] = useState(true)
  const [tcpDiscoveryMode, setTcpDiscoveryMode] = useState<TCPDiscoveryMode>('connect')
  const [synRate, setSynRate] = useState(3000)
  const [synCapability, setSynCapability] = useState<SYNCapabilityStatus | null>(null)
  const [loadingSynCapability, setLoadingSynCapability] = useState(false)
  const [authorizingSyn, setAuthorizingSyn] = useState(false)
  // JS扫描特有配置 - 从 API 加载默认值
  const [blackDomain, setBlackDomain] = useState<string[]>([])
  const [highRiskRouter, setHighRiskRouter] = useState<string[]>([])
  const [authentication, setAuthentication] = useState<string[]>([])
  const [placeholder, setPlaceholder] = useState<Record<string, string>>({})
  const [sqlInjectionEnabled, setSqlInjectionEnabled] = useState(true)
  const [sqlInjectionRules, setSqlInjectionRules] = useState<any[]>([])
  const [lfiEnabled, setLfiEnabled] = useState(true)
  const [lfiParamKeywords, setLfiParamKeywords] = useState<string[]>([])
  const [lfiRules, setLfiRules] = useState<any[]>([])
  const [ssrfEnabled, setSsrfEnabled] = useState(true)
  const [ssrfParamKeywords, setSsrfParamKeywords] = useState<string[]>([])
  const [redirectEnabled, setRedirectEnabled] = useState(true)
  const [redirectParamKeywords, setRedirectParamKeywords] = useState<string[]>([])
  const [xssEnabled, setXssEnabled] = useState(true)
  const [xssRules, setXssRules] = useState<any[]>([])
  const [uploadEnabled, setUploadEnabled] = useState(true)
  const [uploadParamKeywords, setUploadParamKeywords] = useState<string[]>([])
  const [uploadTestContent, setUploadTestContent] = useState('<h1>uploadtest</h1>')
  const [uploadTestFileName, setUploadTestFileName] = useState('test.html')
  const [aiDetectionEnabled, setAiDetectionEnabled] = useState(false)
  const [aiDetectionAvailable, setAiDetectionAvailable] = useState(false)
  // 目录扫描特有配置
  const [dirMethod, setDirMethod] = useState('GET')
  const [dirExtensions, setDirExtensions] = useState('php,aspx,asp,jsp,html,js')
  const [dirStatusFilter, setDirStatusFilter] = useState('404')
  const [dirCustomDict, setDirCustomDict] = useState('')
  const [dirAvailableDicts, setDirAvailableDicts] = useState<any[]>([])
  const [dirSelectedDicts, setDirSelectedDicts] = useState<string[]>([])
  const [dirLoadingDicts, setDirLoadingDicts] = useState(false)
  const [dirAdvancedConfig, setDirAdvancedConfig] = useState({
    thread: 25,
    timeout: 8,
    interval: 0,
    recursion: 0,
    times: 5,
    exclude: '',
    headers: '',
    redirectClient: false,
    backupfileScan: false,
  })
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [runMode, setRunMode] = useState<RunMode>('auto')
  const [agentNodes, setAgentNodes] = useState<AgentNodeOption[]>([])
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [loadingAgents, setLoadingAgents] = useState(false)
  const [desktopRuntime, setDesktopRuntime] = useState(false)
  const [taskSchedule, setTaskSchedule] = useState<ScheduleConfig>(() => createDefaultSchedule())
  
  // 模板列表
  const [templateList, setTemplateList] = useState<string[]>([])
  const [fingerprintList, setFingerprintList] = useState<string[]>([])
  // 扫描模板列表
  const [scanTemplates, setScanTemplates] = useState<any[]>([])
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('')

  // 端口扫描导入相关
  const [portScanDialogOpen, setPortScanDialogOpen] = useState(false)
  const [portScanTasks, setPortScanTasks] = useState<PortScanTask[]>([])
  const [loadingPortScanTasks, setLoadingPortScanTasks] = useState(false)
  const [importingPortScan, setImportingPortScan] = useState(false)
  const [subdomainTaskDialogOpen, setSubdomainTaskDialogOpen] = useState(false)
  const [subdomainTasks, setSubdomainTasks] = useState<SubdomainScanTask[]>([])
  const [loadingSubdomainTasks, setLoadingSubdomainTasks] = useState(false)
  const [importingSubdomainTask, setImportingSubdomainTask] = useState(false)

  const availableAgents = React.useMemo(() => {
    return agentNodes.filter((item) => item.scanWorker !== false && item.enabled && item.isAlive && item.canDispatch)
  }, [agentNodes])

  const enabledSubdomainDnsServerAddresses = React.useMemo(() => {
    return subdomainDnsServers
      .filter((server) => server.enabled)
      .map((server) => server.address.trim())
      .filter(Boolean)
  }, [subdomainDnsServers])

  const loadSYNCapabilityStatus = React.useCallback(async () => {
    setLoadingSynCapability(true)
    try {
      const response = await fetchWithAuth('/api/v1/system/syn/status', { cache: 'no-store' })
      const data = await response.json().catch(() => null)
      if (!response.ok || !data?.success) {
        throw new Error(data?.error || t.vulnScan.newScan.synScan.statusLoadFailed)
      }
      setSynCapability(data.data || null)
    } catch (error: any) {
      console.error('Load SYN capability status failed:', error)
      setSynCapability(null)
      toast.error(error.message || t.vulnScan.newScan.synScan.statusLoadFailed)
    } finally {
      setLoadingSynCapability(false)
    }
  }, [t])

  const handleAuthorizeSYN = React.useCallback(async () => {
    setAuthorizingSyn(true)
    try {
      const response = await fetchWithAuth('/api/v1/system/syn/authorize', {
        method: 'POST',
        cache: 'no-store',
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
      console.error('Authorize SYN scan failed:', error)
      toast.error(error.message || t.vulnScan.newScan.synScan.authorizationFailed)
    } finally {
      setAuthorizingSyn(false)
    }
  }, [loadSYNCapabilityStatus, t])

  const buildSubdomainDnsServerSettingsPayload = React.useCallback((sourceServers: DNSServerSetting[]): DNSServerSettings | null => {
    const servers = sourceServers.map((server) => ({
      address: server.address.trim(),
      name: server.name.trim(),
      enabled: server.enabled,
    }))

    if (servers.some((server) => !server.address)) {
      toast.error(t.subdomain.configSheet.dnsServerAddressRequired)
      return null
    }
    if (!servers.some((server) => server.enabled)) {
      toast.error(t.subdomain.configSheet.dnsServerEnabledRequired)
      return null
    }

    return { servers }
  }, [t.subdomain.configSheet.dnsServerAddressRequired, t.subdomain.configSheet.dnsServerEnabledRequired])

  const persistSubdomainDnsServers = React.useCallback(async (nextServers: DNSServerSetting[], previousServers: DNSServerSetting[] = subdomainDnsServers) => {
    if (isSavingSubdomainDnsServers) {
      return false
    }

    const payload = buildSubdomainDnsServerSettingsPayload(nextServers)
    if (!payload) {
      return false
    }

    setSubdomainDnsServers(nextServers)
    setIsSavingSubdomainDnsServers(true)
    try {
      const response = await fetchWithAuth('/api/v1/subdomain/dns-servers', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      const data = await response.json().catch(() => null)
      if (!response.ok || !data?.success) {
        throw new Error(data?.error || t.subdomain.configSheet.dnsServersSaveFailed)
      }
      setSubdomainDnsServers(normalizeDNSServersFromResponse(data.data?.servers))
      return true
    } catch (error) {
      setSubdomainDnsServers(previousServers)
      const message = error instanceof Error ? error.message : t.subdomain.configSheet.dnsServersSaveFailed
      toast.error(message)
      return false
    } finally {
      setIsSavingSubdomainDnsServers(false)
    }
  }, [buildSubdomainDnsServerSettingsPayload, isSavingSubdomainDnsServers, subdomainDnsServers, t.subdomain.configSheet.dnsServersSaveFailed])

  const subdomainEngineConfigStatus = React.useMemo(() => {
    const missing = selectedSubdomainEngines.filter((engineName) => {
      const engine = SPACE_ENGINES.find((item) => item.name === engineName)
      return Boolean(engine?.requiresApi) && !isSubdomainEngineConfigured(spaceEngineConfig, engineName)
    })

    return { missing }
  }, [selectedSubdomainEngines, spaceEngineConfig])

  const loadAgents = useCallback(async () => {
    setLoadingAgents(true)
    try {
      const response = await fetchWithAuth('/api/v1/webscan/agents')
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || t.vulnScan.newScan.messages.loadAgentsFailed)
      }
      setAgentNodes(Array.isArray(data.data) ? data.data : [])
    } catch (error: any) {
      toast.error(error.message || t.vulnScan.newScan.messages.loadAgentsFailed)
    } finally {
      setLoadingAgents(false)
    }
  }, [t.vulnScan.newScan.messages.loadAgentsFailed])

  useEffect(() => {
    if (scanMode !== ScanType.Subdomain) {
      return
    }
    if (typeof window === 'undefined') {
      return
    }

    const timer = window.setTimeout(() => {
      try {
        const saved = window.localStorage.getItem(SUBDOMAIN_SELECTED_ENGINES_STORAGE_KEY)
        if (!saved) {
          return
        }
        const parsed = JSON.parse(saved)
        if (!Array.isArray(parsed) || parsed.length === 0) {
          return
        }
        const engines = isLegacyDefaultSubdomainEngineSelection(parsed)
          ? DEFAULT_SUBDOMAIN_ENGINES
          : parsed
        const validEngines = engines.filter((engine: string): engine is SpaceEngineName =>
          SPACE_ENGINE_NAMES.includes(engine as SpaceEngineName)
        )
        if (validEngines.length > 0) {
          setSelectedSubdomainEngines(validEngines)
        }
      } catch (error) {
        console.error('Failed to load saved subdomain engines:', error)
      }
    }, 0)

    return () => window.clearTimeout(timer)
  }, [scanMode])

  useEffect(() => {
    if (scanMode !== ScanType.Subdomain || selectedSubdomainEngines.length === 0) {
      return
    }
    if (typeof window === 'undefined') {
      return
    }

    try {
      window.localStorage.setItem(SUBDOMAIN_SELECTED_ENGINES_STORAGE_KEY, JSON.stringify(selectedSubdomainEngines))
    } catch (error) {
      console.error('Failed to save selected subdomain engines:', error)
    }
  }, [scanMode, selectedSubdomainEngines])

  useEffect(() => {
    if (scanMode !== ScanType.Subdomain) {
      return
    }

    let ignore = false
    const loadSubdomainConfig = async () => {
      const [spaceConfigResult, dnsServerResult] = await Promise.allSettled([
        getSpaceEngineConfig(),
        getSubdomainDNSServerSettings(),
      ])

      if (ignore) return

      if (spaceConfigResult.status === 'fulfilled') {
        const nextSpaceConfig = spaceConfigResult.value
        setSpaceEngineConfig(nextSpaceConfig)
        setSelectedSubdomainEngines((prev) => {
          const defaults = getDefaultSubdomainEngines(nextSpaceConfig)
          const shouldUpgradeDefaultSelection = isLegacyDefaultSubdomainEngineSelection(prev)
            || DEFAULT_SUBDOMAIN_ENGINES.every((engine) => prev.includes(engine))
          return shouldUpgradeDefaultSelection
            ? Array.from(new Set([...prev, ...defaults]))
            : prev
        })
      } else {
        console.error('Failed to load space engine config:', spaceConfigResult.reason)
      }

      if (dnsServerResult.status === 'fulfilled') {
        setSubdomainDnsServers(dnsServerResult.value)
      } else {
        console.error('Failed to load subdomain DNS server settings:', dnsServerResult.reason)
      }
    }

    void loadSubdomainConfig()
    return () => {
      ignore = true
    }
  }, [scanMode])

  useEffect(() => {
    let ignore = false

    const detectRuntime = async () => {
      const desktop = await isDesktopRuntime()
      if (ignore) return
      setDesktopRuntime(desktop)
      if (desktop) {
        setRunMode('auto')
        setSelectedAgentId('')
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
    if (!desktopRuntime) {
      return
    }
    const timer = window.setTimeout(() => {
      if (runMode === 'agent') {
        setRunMode('auto')
      }
      if (selectedAgentId) {
        setSelectedAgentId('')
      }
    }, 0)

    return () => window.clearTimeout(timer)
  }, [desktopRuntime, runMode, selectedAgentId])

  useEffect(() => {
    if (scanMode !== ScanType.PortScan) {
      return
    }
    const timer = window.setTimeout(() => {
      void loadSYNCapabilityStatus()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadSYNCapabilityStatus, scanMode])

  useEffect(() => {
    if (!selectedAgentId) {
      return
    }
    const timer = window.setTimeout(() => {
      if (!availableAgents.some((item) => item.agentId === selectedAgentId)) {
        setSelectedAgentId('')
      }
    }, 0)

    return () => window.clearTimeout(timer)
  }, [availableAgents, selectedAgentId])

  useEffect(() => {
    if (scanMode !== ScanType.JsScan) return

    let ignore = false

    getAIConfig()
      .then((config) => {
        if (ignore) return
        const available = hasUsableAIConfig(config)
        setAiDetectionAvailable(available)
        if (!available) {
          setAiDetectionEnabled(false)
        }
      })
      .catch((error) => {
        console.error('Load AI config failed:', error)
        if (ignore) return
        setAiDetectionAvailable(false)
        setAiDetectionEnabled(false)
      })

    return () => {
      ignore = true
    }
  }, [scanMode])

  // 加载 JS 扫描默认配置
  useEffect(() => {
    if (scanMode === ScanType.JsScan) {
      fetchWithAuth('/api/v1/jsscan/default-config')
        .then((res) => res.json())
        .then((data) => {
          if (data.success && data.data) {
            const config = data.data
            // 基本配置
            if (config.blackDomain) setBlackDomain(config.blackDomain)
            if (config.highRiskRouter) setHighRiskRouter(config.highRiskRouter)
            if (config.authentication) setAuthentication(config.authentication)
            if (config.placeholder) setPlaceholder(config.placeholder)
            // 漏洞检测配置
            if (config.vulnDetection) {
              const vuln = config.vulnDetection
              setSqlInjectionEnabled(vuln.enabled !== false)
            }
            setAiDetectionEnabled((config.openai?.enabled ?? false) && aiDetectionAvailable)
          }
        })
        .catch((error) => {
          console.error('Load JS scan default config failed:', error)
          // 静默失败，使用组件内部的默认值
        })
    }
  }, [scanMode, aiDetectionAvailable])

  // 将模板列表转换为 MultiSelectOption 格式
  const templateOptions = React.useMemo<MultiSelectOption[]>(() => {
    return templateList.map((template) => ({
      value: template,
      label: template,
    }))
  }, [templateList])

  const fingerprintOptions = React.useMemo<MultiSelectOption[]>(() => {
    return fingerprintList.map((fingerprint) => ({
      value: fingerprint,
      label: fingerprint,
    }))
  }, [fingerprintList])

  // 获取指纹列表
  useEffect(() => {
    if (scanMode === ScanType.WebScan && webscanOption === 4) {
      fetchWithAuth('/api/fingerprint/list')
        .then((response) => {
          if (!response.ok) {
            throw new Error(t.vulnScan.newScan.messages.loadFingerprintsFailed)
          }
          return response.json()
        })
        .then((data) => {
          const payload = data?.data && typeof data.data === 'object' ? data.data : data
          const nameSet = new Set<string>()
          Object.values(payload as Record<string, any[]>).forEach((group) => {
            if (!Array.isArray(group)) return
            group.forEach((item) => {
              const name = typeof item === 'string'
                ? item.trim()
                : typeof item?.Name === 'string'
                  ? item.Name.trim()
                  : typeof item?.name === 'string'
                    ? item.name.trim()
                    : ''
              if (name) nameSet.add(name)
            })
          })
          setFingerprintList(Array.from(nameSet).sort((a, b) => a.localeCompare(b)))
        })
        .catch((error) => {
          console.error('Failed to get fingerprint list:', error)
          setFingerprintList([])
        })
    }
  }, [scanMode, t.vulnScan.newScan.messages.loadFingerprintsFailed, webscanOption])

  // 获取模板列表
  useEffect(() => {
    if ((scanMode === ScanType.WebScan || scanMode === ScanType.DirScan) && webscanOption === 4) {
      fetchWithAuth(`/api/v1/webscan/templates`)
        .then((response) => {
          if (!response.ok) {
            throw new Error(t.vulnScan.newScan.messages.loadTemplatesFailed)
          }
          return response.json()
        })
        .then((data) => {
          if (data.success && data.data) {
            setTemplateList(data.data)
          }
        })
        .catch((error) => {
          console.error('Failed to get template list:', error)
        })
    }
  }, [scanMode, t.vulnScan.newScan.messages.loadTemplatesFailed, webscanOption])

  // 获取扫描模板列表 - 使用新的配置模板 API
  useEffect(() => {
    // 根据当前扫描模式确定任务类型
  const taskTypeMap: Record<ScanType, string> = {
    [ScanType.Subdomain]: 'subdomain',
    [ScanType.WebScan]: 'webscan',
    [ScanType.PortScan]: 'portscan',
    [ScanType.DirScan]: 'dirsearch',
    [ScanType.Crack]: 'crack',
    [ScanType.JsScan]: 'jsscan',
    [ScanType.Combination]: 'combination',
  }
    const taskType = taskTypeMap[scanMode]
    
    if (taskType) {
      fetchWithAuth(`/api/v1/config/scan-templates?taskType=${taskType}`)
        .then(async (response) => {
          if (!response.ok) {
            const errorData = await response.json().catch(() => ({}))
            throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
          }
          return response.json()
        })
        .then((data) => {
          if (data.success && data.data) {
            setScanTemplates(data.data)
          } else {
            console.warn('Failed to get scan templates:', data.error || 'Unknown error')
            setScanTemplates([])
          }
        })
        .catch((error) => {
          console.error('Failed to get scan templates:', error)
          // 静默失败，不影响页面使用
          setScanTemplates([])
        })
    }
  }, [scanMode])

  // 加载目录扫描字典列表
  useEffect(() => {
    if (scanMode === ScanType.DirScan) {
      const timer = window.setTimeout(() => {
        setDirLoadingDicts(true)
        fetchWithAuth(`/api/v1/dirsearch/dicts`)
          .then((response) => {
            if (!response.ok) {
              throw new Error(t.vulnScan.newScan.messages.loadDictsFailed)
            }
            return response.json()
          })
          .then((data) => {
            if (data.success && data.dicts) {
              const dicts: DictionaryMeta[] = data.dicts.map((dict: any, index: number) => ({
                id: dict.name || `dict-${index}`,
                name: dict.name || 'unknown',
                path: dict.path || '',
                description: dict.description || dict.name || t.vulnScan.newScan.messages.noDescription,
                size: dict.size || 0,
                entries: Array.isArray(dict.entries) ? dict.entries : [],
              }))
              setDirAvailableDicts(dicts)
              // 只在初始加载时自动选中第一个字典
              if (dicts.length > 0 && dirSelectedDicts.length === 0) {
                setDirSelectedDicts([dicts[0].id])
              }
            }
          })
          .catch((error) => {
            console.error('Load dictionary list failed:', error)
            toast.error(t.vulnScan.newScan.messages.loadDictsFailed)
          })
          .finally(() => {
            setDirLoadingDicts(false)
          })
      }, 0)

      return () => window.clearTimeout(timer)
    }
  }, [
    dirSelectedDicts.length,
    scanMode,
    t.vulnScan.newScan.messages.loadDictsFailed,
    t.vulnScan.newScan.messages.noDescription,
  ])

  // 应用模板
  const handleApplyTemplate = (templateId: string) => {
    const template = scanTemplates.find((t) => t.id === templateId)
    if (!template || !template.config) return

    const config = template.config

    switch (template.taskType) {
      case 'subdomain':
        if (config.thread !== undefined) setThreads(config.thread)
        if (config.timeout !== undefined) setSubdomainTimeout(config.timeout)
        if (config.appendEngines !== undefined) {
          const engines = Array.isArray(config.appendEngines)
            ? config.appendEngines
            : String(config.appendEngines).split(',')
          const validEngines = engines
            .map((item: string) => item.trim())
            .filter((item: string): item is SpaceEngineName => SPACE_ENGINE_NAMES.includes(item as SpaceEngineName))
          if (validEngines.length > 0) {
            setSelectedSubdomainEngines(validEngines)
          }
        }
        if (config.dnsServers !== undefined && Array.isArray(config.dnsServers)) {
          setSubdomainDnsServers(config.dnsServers.map((address: string) => ({
            address,
            name: t.subdomain.configSheet.customDns,
            enabled: true,
          })))
        }
        break

      case 'webscan':
        if (config.webscanOption !== undefined) setWebscanOption(config.webscanOption)
        if (config.threads !== undefined) setThreads(config.threads)
        if (config.nucleiThreads !== undefined) setNucleiThreads(config.nucleiThreads)
        if (config.customHeaders !== undefined) setCustomHeaders(config.customHeaders)
        if (config.customTags !== undefined) setCustomTags(config.customTags)
        if (config.customTemplate !== undefined) setCustomTemplate(config.customTemplate)
        if (config.rootPathScan !== undefined) setRootPathScan(config.rootPathScan)
        if (config.skipNucleiWithoutTags !== undefined) setSkipNucleiWithoutTags(config.skipNucleiWithoutTags)
        if (config.skipNucleiWithWAF !== undefined) setSkipNucleiWithWAF(config.skipNucleiWithWAF)
        if (config.screenshot !== undefined) setScreenshot(config.screenshot)
        if (config.activeTimeoutLimit !== undefined) setActiveTimeoutLimit(config.activeTimeoutLimit)
        if (config.disableMaxHostErr !== undefined) setDisableMaxHostErr(config.disableMaxHostErr)
        if (config.oobEnabled !== undefined) setOobEnabled(config.oobEnabled)
        if (config.interactshURL !== undefined) setInteractshURL(config.interactshURL)
        if (config.interactshUrl !== undefined) setInteractshURL(config.interactshUrl)
        if (config.interactshToken !== undefined) setInteractshToken(config.interactshToken)
        if (config.oob && typeof config.oob === 'object') {
          const oob = config.oob as any
          if (oob.enabled !== undefined) setOobEnabled(oob.enabled === true)
          if (oob.url !== undefined) setInteractshURL(oob.url)
          if (oob.interactshURL !== undefined) setInteractshURL(oob.interactshURL)
          if (oob.token !== undefined) setInteractshToken(oob.token)
          if (oob.interactshToken !== undefined) setInteractshToken(oob.interactshToken)
        }
        if (config.interactsh && typeof config.interactsh === 'object') {
          const interactsh = config.interactsh as any
          if (interactsh.enabled !== undefined) setOobEnabled(interactsh.enabled === true)
          if (interactsh.url !== undefined) setInteractshURL(interactsh.url)
          if (interactsh.serverURL !== undefined) setInteractshURL(interactsh.serverURL)
          if (interactsh.token !== undefined) setInteractshToken(interactsh.token)
          if (interactsh.authorization !== undefined) setInteractshToken(interactsh.authorization)
        }
        if (
          config.oobEnabled === undefined &&
          (config.interactshURL || config.interactshUrl || config.interactshToken)
        ) {
          setOobEnabled(true)
        }
        break

      case 'portscan':
        setTcpHeuristic(normalizeTCPHeuristicConfig(config))
        if (config.portGroup !== undefined) setPortGroup(config.portGroup)
        if (config.portRange !== undefined) setPortRange(config.portRange)
        if (config.threads !== undefined) setThreads(config.threads)
        if (config.timeout !== undefined) setPortTimeout(config.timeout)
        if (config.aliveDetection !== undefined) setAliveDetection(config.aliveDetection)
        {
          const discoveryMode = String(config.tcpDiscoveryMode || config.portDiscoveryMode || '').trim().toLowerCase().replaceAll('_', '-')
          if (['syn', 'tcp-syn', 'syn-scan'].includes(discoveryMode)) {
            setTcpDiscoveryMode('syn')
          } else if (['connect', 'tcp', 'tcp-connect', 'full-connect'].includes(discoveryMode)) {
            setTcpDiscoveryMode('connect')
          }
        }
        if (config.synRate !== undefined || config.portDiscoveryRate !== undefined || config.discoveryRate !== undefined) {
          const nextRate = Number(config.synRate ?? config.portDiscoveryRate ?? config.discoveryRate)
          if (Number.isFinite(nextRate) && nextRate > 0) {
            setSynRate(nextRate)
          }
        }
        break

      case 'dirsearch':
        if (config.method !== undefined) setDirMethod(config.method)
        if (config.extensions !== undefined) setDirExtensions(config.extensions)
        if (config.statusFilter !== undefined) setDirStatusFilter(config.statusFilter)
        if (config.selectedDicts !== undefined) setDirSelectedDicts(config.selectedDicts)
        if (config.customDict !== undefined) setDirCustomDict(config.customDict)
        setDirAdvancedConfig(prev => ({
          ...prev,
          ...(config.thread !== undefined && { thread: config.thread }),
          ...(config.timeout !== undefined && { timeout: config.timeout }),
          ...(config.interval !== undefined && { interval: config.interval }),
          ...(config.recursion !== undefined && { recursion: config.recursion }),
          ...(config.times !== undefined && { times: config.times }),
          ...(config.exclude !== undefined && { exclude: config.exclude }),
          ...(config.headers !== undefined && { headers: config.headers }),
          ...(config.redirectClient !== undefined && { redirectClient: config.redirectClient }),
          ...(config.backupfileScan !== undefined && { backupfileScan: config.backupfileScan }),
        }))
        break

      case 'crack':
        if (config.username !== undefined) setUsername(config.username)
        if (config.password !== undefined) setPassword(config.password)
        if (config.defaultDict !== undefined) setDefaultDict(config.defaultDict)
        break

      case 'jsscan':
        if (config.blackDomain !== undefined) setBlackDomain(config.blackDomain)
        if (config.highRiskRouter !== undefined) setHighRiskRouter(config.highRiskRouter)
        if (config.authentication !== undefined) setAuthentication(config.authentication)
        if (config.placeholder !== undefined) setPlaceholder(config.placeholder)
        if (config.sqlInjectionEnabled !== undefined) setSqlInjectionEnabled(config.sqlInjectionEnabled)
        if (config.lfiEnabled !== undefined) setLfiEnabled(config.lfiEnabled)
        if (config.ssrfEnabled !== undefined) setSsrfEnabled(config.ssrfEnabled)
        if (config.redirectEnabled !== undefined) setRedirectEnabled(config.redirectEnabled)
        if (config.xssEnabled !== undefined) setXssEnabled(config.xssEnabled)
        if (config.uploadEnabled !== undefined) setUploadEnabled(config.uploadEnabled)
        if (config.aiDetectionEnabled !== undefined) {
          setAiDetectionEnabled(config.aiDetectionEnabled && aiDetectionAvailable)
        }
        break
    }

    toast.success(t.vulnScan.messages.templateApplied.replace('{name}', template.name))
  }

  // 获取端口扫描任务列表
  const loadPortScanTasks = useCallback(async () => {
    setLoadingPortScanTasks(true)
    try {
      // 查询 vuln_scan_task 表中类型为端口扫描(scanMode=1)的任务
      const response = await fetchWithAuth(`/api/v1/webscan/tasks?limit=100&scanMode=${ScanType.PortScan}`)
      
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.newScan.messages.loadPortScanTasksFailed)
      }

      if (data.data && data.data.tasks && Array.isArray(data.data.tasks)) {
        // 允许导入所有终止态且已有结果的端口扫描任务
        const importableTasks = data.data.tasks.filter(
          (task: PortScanTask) =>
            IMPORTABLE_PORT_SCAN_STATUSES.has(task.status) && (task.portCount || 0) > 0
        )
        setPortScanTasks(importableTasks)
      } else {
        setPortScanTasks([])
      }
    } catch (error: any) {
      console.error('Load port scan task list failed:', error)
      toast.error(t.vulnScan.messages.getPortScanFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
      setPortScanTasks([])
    } finally {
      setLoadingPortScanTasks(false)
    }
  }, [
    t.vulnScan.messages.getPortScanFailed,
    t.vulnScan.messages.unknownError,
    t.vulnScan.newScan.messages.loadPortScanTasksFailed,
  ])

  // 打开端口扫描导入对话框时加载任务列表
  useEffect(() => {
    if (portScanDialogOpen) {
      const timer = window.setTimeout(() => {
        void loadPortScanTasks()
      }, 0)

      return () => window.clearTimeout(timer)
    }
  }, [loadPortScanTasks, portScanDialogOpen])

  // 获取可导入的子域名搜集任务列表
  const loadSubdomainTasks = useCallback(async () => {
    setLoadingSubdomainTasks(true)
    try {
      const response = await fetchWithAuth('/api/v1/subdomain/tasks?limit=100')

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.newScan.messages.loadSubdomainTasksFailed)
      }

      if (data.data && data.data.tasks && Array.isArray(data.data.tasks)) {
        const importableTasks = data.data.tasks.filter(
          (task: SubdomainScanTask) =>
            IMPORTABLE_SUBDOMAIN_TASK_STATUSES.has(task.status) && (task.resultCount || 0) > 0
        )
        setSubdomainTasks(importableTasks)
      } else {
        setSubdomainTasks([])
      }
    } catch (error: any) {
      console.error('Load subdomain task list failed:', error)
      toast.error(t.vulnScan.newScan.messages.loadSubdomainTasksFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
      setSubdomainTasks([])
    } finally {
      setLoadingSubdomainTasks(false)
    }
  }, [
    t.vulnScan.messages.unknownError,
    t.vulnScan.newScan.messages.loadSubdomainTasksFailed,
  ])

  // 打开子域名导入对话框时加载任务列表
  useEffect(() => {
    if (subdomainTaskDialogOpen) {
      const timer = window.setTimeout(() => {
        void loadSubdomainTasks()
      }, 0)

      return () => window.clearTimeout(timer)
    }
  }, [loadSubdomainTasks, subdomainTaskDialogOpen])

  // 导入端口扫描结果
  const handleImportPortScan = async (task: PortScanTask) => {
    setImportingPortScan(true)
    try {
      const taskId = task.id
      // 根据 scanMode 选择 mode
      const mode =
        scanMode === ScanType.Crack
          ? 'brute'
          : 'web'
      const params = new URLSearchParams({ mode })
      if (task.version) {
        params.set('version', String(task.version))
      }
      const response = await fetchWithAuth(`/api/v1/portscan/task/${taskId}/results/urls?${params.toString()}`)
      
      if (!response.ok) {
        throw new Error(t.vulnScan.newScan.messages.loadPortScanResultFailed)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.messages.importFailed)
      }

      // 检查是否有目标数据
      if (!data.data || !data.data.targets || data.data.targets.length === 0) {
        toast.error(t.vulnScan.messages.noTargetsToImport)
        return
      }

      const rawTargets = (data.data.targets as string[]) || []
      const importedTargets =
        scanMode === ScanType.WebScan || scanMode === ScanType.DirScan || scanMode === ScanType.JsScan
          ? rawTargets.filter((target) => /^https?:\/\//i.test(target))
          : rawTargets

      if (importedTargets.length === 0) {
        toast.error(t.vulnScan.messages.noTargetsToImport)
        return
      }

      // 合并到现有目标（去重）
      const existingTargets = targets
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => !!line)

      const allTargets = [...existingTargets, ...importedTargets]
      const uniqueTargets = Array.from(new Set(allTargets))

      setTargets(uniqueTargets.join('\n'))
      toast.success(t.vulnScan.messages.importSuccess.replace('{count}', String(importedTargets.length)).replace('{total}', String(uniqueTargets.length)))
      setPortScanDialogOpen(false)
    } catch (error: any) {
      toast.error(t.vulnScan.messages.importFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    } finally {
      setImportingPortScan(false)
    }
  }

  // 导入子域名搜集结果到端口扫描目标
  const handleImportSubdomainTask = async (task: SubdomainScanTask) => {
    setImportingSubdomainTask(true)
    try {
      const response = await fetchWithAuth(`/api/v1/subdomain/task/${task.id}/copy?type=portscan-targets`)

      if (!response.ok) {
        throw new Error(t.vulnScan.newScan.messages.loadSubdomainResultsFailed)
      }

      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.vulnScan.messages.importFailed)
      }

      const importedTargets = String(data.data?.text || '')
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => !!line)

      if (importedTargets.length === 0) {
        toast.error(t.vulnScan.messages.noTargetsToImport)
        return
      }

      const existingTargets = targets
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => !!line)

      const allTargets = [...existingTargets, ...importedTargets]
      const uniqueTargets = Array.from(new Set(allTargets))

      setTargets(uniqueTargets.join('\n'))
      toast.success(t.vulnScan.messages.importSuccess.replace('{count}', String(importedTargets.length)).replace('{total}', String(uniqueTargets.length)))
      setSubdomainTaskDialogOpen(false)
    } catch (error: any) {
      toast.error(t.vulnScan.messages.importFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    } finally {
      setImportingSubdomainTask(false)
    }
  }

  // 重新加载目录扫描字典列表
  const handleReloadDirDicts = async () => {
    setDirLoadingDicts(true)
    try {
      const response = await fetchWithAuth(`/api/v1/dirsearch/dicts`)
      
      if (!response.ok) {
        throw new Error(t.vulnScan.newScan.messages.loadDictsFailed)
      }

      const data = await response.json()
      if (data.success && data.dicts) {
        const dicts: DictionaryMeta[] = data.dicts.map((dict: any, index: number) => ({
          id: dict.name || `dict-${index}`,
          name: dict.name || 'unknown',
          path: dict.path || '',
          description: dict.description || dict.name || t.vulnScan.newScan.messages.noDescription,
          size: dict.size || 0,
          entries: Array.isArray(dict.entries) ? dict.entries : [],
        }))
        
        setDirAvailableDicts(dicts)
        // 只在初始加载时自动选中第一个字典
        if (dicts.length > 0 && dirSelectedDicts.length === 0) {
          setDirSelectedDicts([dicts[0].id])
        }
      }
    } catch (error: any) {
      console.error('Load dictionary list failed:', error)
      toast.error(`${t.vulnScan.newScan.messages.loadDictsFailed}: ${error.message}`)
    } finally {
      setDirLoadingDicts(false)
    }
  }

  // 目录扫描字典上传
  const handleDirDictionaryUpload = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = event.target.files
    if (!files?.length) return

    // 上传每个文件到后端
    for (const file of Array.from(files)) {
      // 验证文件扩展名
      if (!file.name.toLowerCase().endsWith('.txt')) {
        toast.error(formatText(t.vulnScan.newScan.messages.invalidTxtFile, { name: file.name }))
        continue
      }

      try {
        // 创建 FormData
        const formData = new FormData()
        formData.append('file', file)

        // 上传到后端
        const response = await fetchWithAuth('/api/v1/dirsearch/dicts/upload', {
          method: 'POST',
          body: formData,
        })

        if (!response.ok) {
          const errorData = await response.json().catch(() => ({}))
          throw new Error(errorData.error || formatText(t.vulnScan.newScan.messages.uploadFailedWithStatus, { status: response.status }))
        }

        const data = await response.json()
        if (!data.success) {
          throw new Error(data.error || t.vulnScan.newScan.messages.uploadFailed)
        }

        toast.success(formatText(t.vulnScan.newScan.messages.uploadedToServer, { name: file.name }))
      } catch (err: any) {
        console.error('Upload dictionary failed:', err)
        toast.error(`${file.name}: ${err.message}`)
      }
    }

    // 上传完成后重新加载字典列表
    await handleReloadDirDicts()

    // 清空 input
    event.target.value = ''
  }

  const handleStart = async (e?: MouseEvent<HTMLButtonElement>) => {
    
    // 阻止默认行为
    if (e) {
      e.preventDefault()
      e.stopPropagation()
    }

    const targetList = targets
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => !!line)

    if (targetList.length === 0) {
      toast.error(t.vulnScan.messages.enterAtLeastOneTarget)
      return
    }

    // 密码暴破模式：验证输入格式为 协议://ip:port
    if (scanMode === ScanType.Crack) {
      const urlPattern = /^[a-zA-Z][a-zA-Z0-9+.-]*:\/\/[^\s]+$/
      const invalidTargets = targetList.filter(target => !urlPattern.test(target))
      if (invalidTargets.length > 0) {
        toast.error(`${t.vulnScan.messages.invalidTargetFormat}:\n${invalidTargets.join('\n')}`)
        return
      }
    }

    if (scanMode === ScanType.PortScan) {
      const error = validateTCPHeuristicConfig(tcpHeuristic, portRange)
      if (error) {
        toast.error(t.vulnScan.newScan.tcpHeuristic[error])
        return
      }
    }

    type ScanConfig = SubdomainScanConfig | WebScanConfig | PortScanConfig | CrackScanConfig | DirScanConfig | JsScanConfig

    if (!desktopRuntime && runMode === 'agent' && availableAgents.length === 0) {
      toast.error(t.vulnScan.newScan.messages.noSchedulableAgents)
      return
    }

    if (!desktopRuntime && runMode === 'agent' && !selectedAgentId) {
      toast.error(t.vulnScan.newScan.execution.selectAgent)
      return
    }

    const effectiveRunMode: RunMode = desktopRuntime && runMode === 'agent' ? 'auto' : runMode
    if (
      scanMode === ScanType.PortScan &&
      effectiveRunMode === 'local' &&
      tcpDiscoveryMode === 'syn' &&
      !synCapability?.authorized
    ) {
      toast.error(t.vulnScan.newScan.synScan.strictRequiresAuthorization)
      return
    }

    // 根据不同的 scanMode 构建独立的 config 对象
    let config: ScanConfig
    if (scanMode === ScanType.Subdomain) {
      const { validTargets, invalidTargets } = validateSubdomainTargets(targetList)
      if (invalidTargets.length > 0) {
        toast.error(`${t.vulnScan.messages.invalidDomainFormat}\n${invalidTargets.join('\n')}`)
        return
      }
      if (selectedSubdomainEngines.length === 0) {
        toast.error(t.subdomain.configSheet.engineRequired)
        return
      }
      if (subdomainEngineConfigStatus.missing.length > 0) {
        toast.error(formatText(t.subdomain.configSheet.missingEngines, {
          engines: subdomainEngineConfigStatus.missing.join(', '),
        }))
        return
      }
      if (enabledSubdomainDnsServerAddresses.length === 0) {
        toast.error(t.subdomain.configSheet.dnsServerEnabledRequired)
        return
      }
      const dnsPayload = buildSubdomainDnsServerSettingsPayload(subdomainDnsServers)
      if (!dnsPayload) {
        return
      }

      config = {
        targets: validTargets,
        thread: threads,
        timeout: subdomainTimeout,
        appendEngines: selectedSubdomainEngines,
        dnsServers: enabledSubdomainDnsServerAddresses,
      }
    } else if (scanMode === ScanType.WebScan) {
      const shouldEnableOOB = webscanOption !== 1 && webscanOption !== 2 && oobEnabled
      config = {
        targets: targetList,
        threads,
        nucleiThreads,
        proxy: getProxyUrl() || undefined,
        webscanOption,
        customHeaders: customHeaders || undefined,
        customTags: webscanOption === 4 ? (customTags.length > 0 ? customTags : undefined) : undefined,
        customTemplate: webscanOption === 3 || webscanOption === 4 ? (customTemplate.length > 0 ? customTemplate : undefined) : undefined,
        rootPathScan,
        skipNucleiWithoutTags,
        skipNucleiWithWAF,
        screenshot,
        activeTimeoutLimit,
        disableMaxHostErr,
        oobEnabled: shouldEnableOOB,
        interactshURL: shouldEnableOOB ? interactshURL.trim() || undefined : undefined,
        interactshToken: shouldEnableOOB ? interactshToken.trim() || undefined : undefined,
      }
    } else if (scanMode === ScanType.DirScan) {
      // 构建目录扫描配置
      const customPaths = dirCustomDict
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => !!line)

      // 解析状态码排除列表
      const parseStatusFilter = (input: string): number[] => {
        if (!input) return []
        return input
          .split(',')
          .map((block) => {
            const trimmed = block.trim()
            if (!trimmed) return []
            if (trimmed.includes('-')) {
              const [start, end] = trimmed.split('-').map((v) => Number(v))
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

      const excludedStatusList = parseStatusFilter(dirStatusFilter)

      // 构建字典路径列表
      let dictPaths: string[] = []
      let paths: string[] = []

      if (customPaths.length > 0) {
        paths = customPaths
      } else if (dirSelectedDicts.length > 0) {
        for (const dictId of dirSelectedDicts) {
          const dict = dirAvailableDicts.find((d: any) => d.id === dictId)
          if (!dict) continue

          if (dict.path) {
            dictPaths.push(dict.path)
          } else if (dict.entries && dict.entries.length > 0) {
            paths.push(...dict.entries)
          }
        }
      }

      if (dictPaths.length === 0 && paths.length === 0) {
        toast.error(t.vulnScan.newScan.messages.selectDictRequired)
        return
      }

      config = {
        targets: targetList,
        method: dirMethod as any,
        dictPaths: dictPaths.length > 0 ? dictPaths : undefined,
        paths: paths.length > 0 ? paths : undefined,
        exts: dirExtensions.split(',').map((s) => s.trim()).filter(Boolean),
        workers: dirAdvancedConfig.thread,
        timeout: dirAdvancedConfig.timeout,
        bodyExclude: dirAdvancedConfig.exclude || undefined,
        bodyLengthExcludeTimes: dirAdvancedConfig.times || undefined,
        statusCodeExclude: excludedStatusList,
        redirect: dirAdvancedConfig.redirectClient,
        interval: dirAdvancedConfig.interval || undefined,
        customHeader: dirAdvancedConfig.headers || undefined,
        backupscan: dirAdvancedConfig.backupfileScan,
        proxy: getProxyUrl() || undefined,
        wordlist: undefined,
        recursive: dirAdvancedConfig.recursion || 0,
      }
    } else if (scanMode === ScanType.JsScan) {
      config = {
        targets: targetList,
        proxy: getProxyUrl() || undefined,
        blackDomain,
        highRiskRouter,
        authentication,
        placeholder,
        vulnDetection: {
          enabled: sqlInjectionEnabled,
        },
        // AI 检测设置：启用时后端会从系统配置中获取 AI 参数
        openai: { enabled: aiDetectionEnabled && aiDetectionAvailable },
      }
    } else if (scanMode === ScanType.PortScan) {
      config = {
        targets: targetList,
        threads,
        timeout: portTimeout,
        portRange,
        proxy: getProxyUrl() || undefined,
        aliveDetection,
        ...tcpHeuristic,
        tcpDiscoveryMode,
        portDiscoveryMode: tcpDiscoveryMode,
        synRate: tcpDiscoveryMode === 'syn' ? synRate : undefined,
      }
    } else {
      // Crack / brute-force
      config = {
        targets: targetList,
        threads,
        proxy: getProxyUrl() || undefined,
        username,
        password,
        defaultDict,
      }
    }

    const schedulePayload: ScheduleConfig = taskSchedule

    const requestBody: any = {
      scanType: scanMode,
      scanMode: effectiveRunMode,
      config: config,
      schedule: schedulePayload,
    }
    if (!desktopRuntime && effectiveRunMode === 'agent' && selectedAgentId) {
      requestBody.agentId = selectedAgentId
    }
    if (taskName) requestBody.taskName = taskName

    setIsSubmitting(true)
    try {
 
      const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody),
      })
      
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        console.error('Request failed:', errorData)
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (!data.success || !data.taskId) {
        throw new Error(data.error || t.vulnScan.messages.createTaskFailed)
      }

      toast.success(t.vulnScan.messages.taskCreated)
      // 跳回任务列表页面
      router.push('/scan/vuln-scan')
    } catch (error: any) {
      console.error('Start scan error:', error)
      toast.error(t.vulnScan.messages.createTaskFailed + ': ' + (error.message || t.vulnScan.messages.unknownError))
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleCancel = () => {
    router.push('/scan/vuln-scan')
  }

  if (scanModeParam === 'combination') {
    return <CombinationTaskCreation plannerMode="manual" />
  }

  return (
    <div className="flex-1 scan-panel-enter">
      {/* 顶部栏 */}
      <div className="sticky top-0 z-30 border-b border-border bg-card/95 backdrop-blur supports-[backdrop-filter]:bg-card/80">
        <div className="flex items-center justify-between px-6 py-2 h-[56px]">
          <Button
            variant="ghost"
            size="sm"
            onClick={handleCancel}
            className="h-8"
          >
            <ArrowLeft className="h-4 w-4 mr-2" />
            {t.vulnScan.backToList}
          </Button>
        </div>  
      </div>

      {/* 表单内容 */}
      <div className="p-3 pb-28 md:p-6 md:pb-28">
        <div className="max-w-3xl mx-auto space-y-6">
          {/* 任务名称 */}
          <div className="space-y-2">
            <Label htmlFor="taskName">{t.vulnScan.taskName}</Label>
            <Input
              id="taskName"
              placeholder={t.vulnScan.taskNamePlaceholder}
              value={taskName}
              onChange={(e) => setTaskName(e.target.value)}
            />
          </div>

          {!desktopRuntime && (
          <div className="space-y-2">
            <Label>{t.vulnScan.newScan.execution.modeLabel}</Label>
            <div className="rounded-lg border border-border/70 bg-muted/20 p-3 space-y-2">
              <div className="grid gap-2 grid-cols-1 lg:grid-cols-[220px_minmax(0,1fr)_auto] items-center">
                <Select value={runMode} onValueChange={(value) => setRunMode(value as RunMode)}>
                  <SelectTrigger className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">{t.vulnScan.newScan.execution.autoOption}</SelectItem>
                    <SelectItem value="local">{t.vulnScan.newScan.execution.localOption}</SelectItem>
                    <SelectItem value="agent">{t.vulnScan.newScan.execution.agentOption}</SelectItem>
                  </SelectContent>
                </Select>

                {runMode === 'agent' ? (
                  <Select value={selectedAgentId} onValueChange={setSelectedAgentId}>
                    <SelectTrigger className="w-full">
                      <SelectValue placeholder={t.vulnScan.newScan.execution.selectAgent} />
                    </SelectTrigger>
                    <SelectContent>
                      {availableAgents.map((agent) => (
                        <SelectItem key={agent.agentId} value={agent.agentId}>
                          {agent.displayName || agent.agentId} ({agent.agentId}){agent.localWorker ? ` · ${t.vulnScan.newScan.execution.localNode}` : ''}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <div className="text-xs text-muted-foreground border border-border rounded-md px-3 h-10 flex items-center w-full">
                    {runMode === 'auto' ? t.vulnScan.newScan.execution.autoRuntimeHint : t.vulnScan.newScan.execution.localRuntimeHint}
                  </div>
                )}

                {runMode === 'agent' ? (
                  <Button
                    type="button"
                    variant="outline"
                    onClick={loadAgents}
                    disabled={loadingAgents}
                    className="w-full lg:w-auto"
                  >
                    <RefreshCw className={`w-4 h-4 mr-2 ${loadingAgents ? 'animate-spin' : ''}`} />
                    {t.vulnScan.newScan.execution.refreshAgents}
                  </Button>
                ) : (
                  <div className="hidden lg:block" />
                )}
              </div>
              {runMode === 'agent' && (
                <p className="text-xs text-muted-foreground">
                  {formatText(t.vulnScan.newScan.execution.availableAgents, { count: availableAgents.length })}
                </p>
              )}
            </div>
          </div>
          )}

          <TaskScheduleSettings
            value={taskSchedule}
            onChange={setTaskSchedule}
            disabled={isSubmitting}
          />

          {/* 目标输入 */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <Label htmlFor="targets">
                {scanMode === ScanType.Subdomain
                  ? t.vulnScan.taskManagement.subdomain
                  : scanMode === ScanType.WebScan
                  ? t.vulnScan.target
                  : scanMode === ScanType.JsScan || scanMode === ScanType.DirScan
                    ? t.vulnScan.targetUrl
                    : scanMode === ScanType.PortScan
                      ? t.vulnScan.newScan.scanTarget
                      : t.vulnScan.targetAddress}
              </Label>
              {scanMode === ScanType.PortScan ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setSubdomainTaskDialogOpen(true)}
                  disabled={isSubmitting}
                >
                  <Download className="w-4 h-4 mr-2" />
                  {t.vulnScan.newScan.importSubdomainTask}
                </Button>
              ) : scanMode !== ScanType.Subdomain && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setPortScanDialogOpen(true)}
                  disabled={isSubmitting}
                >
                  <Download className="w-4 h-4 mr-2" />
                  {t.vulnScan.newScan.importPortScan}
                </Button>
              )}
            </div>
            {scanMode === ScanType.Subdomain ? (
              <TextInputWithActions
                value={targets}
                onChange={setTargets}
                placeholder={t.vulnScan.combination.inline.targetPlaceholderDomain}
                disabled={isSubmitting}
                height="160px"
                onFileUpload={(file) => {
                  const reader = new FileReader()
                  reader.onload = (event) => {
                    const text = event.target?.result as string
                    setTargets(text)
                    toast.success(t.vulnScan.messages.imported.replace('{count}', String(text.split('\n').filter(Boolean).length)))
                  }
                  reader.readAsText(file)
                }}
                onDeduplicate={(text) => {
                  const lines = text.split('\n').filter((line) => line.trim())
                  const unique = Array.from(new Set(lines))
                  setTargets(unique.join('\n'))
                  toast.success(t.vulnScan.messages.deduplicated.replace('{count}', String(unique.length)))
                }}
                onCopy={() => {
                  toast.success(t.vulnScan.messages.copied)
                }}
                onClear={() => {
                  toast.success(t.vulnScan.messages.cleared)
                }}
              />
            ) : scanMode === ScanType.WebScan || scanMode === ScanType.JsScan || scanMode === ScanType.DirScan ? (
              <TextInputWithActions
                value={targets}
                onChange={setTargets}
                placeholder={scanMode === ScanType.DirScan ? t.vulnScan.newScan.dirscanTargetPlaceholder : scanMode === ScanType.WebScan ? t.vulnScan.newScan.webscanTargetPlaceholder : t.vulnScan.newScan.urlPlaceholder}
                disabled={isSubmitting}
                height="160px"
                onFileUpload={(file) => {
                  const reader = new FileReader()
                  reader.onload = (event) => {
                    const text = event.target?.result as string
                    setTargets(text)
                    toast.success(t.vulnScan.messages.imported.replace('{count}', String(text.split('\n').filter(Boolean).length)))
                  }
                  reader.readAsText(file)
                }}
                onDeduplicate={(text) => {
                  const lines = text.split('\n').filter((line) => line.trim())
                  const unique = Array.from(new Set(lines))
                  const newValue = unique.join('\n')
                  setTargets(newValue)
                  toast.success(t.vulnScan.messages.deduplicated.replace('{count}', String(unique.length)))
                }}
                onCopy={() => {
                  toast.success(t.vulnScan.messages.copied)
                }}
                onClear={() => {
                  toast.success(t.vulnScan.messages.cleared)
                }}
              />
            ) : scanMode === ScanType.PortScan ? (
              <div className="space-y-2">
                <TextInputWithActions
                  value={targets}
                  onChange={setTargets}
                  placeholder={t.vulnScan.newScan.portscanTargetPlaceholder}
                  disabled={isSubmitting}
                  height="160px"
                  onFileUpload={(file) => {
                    const reader = new FileReader()
                    reader.onload = (event) => {
                      const text = event.target?.result as string
                      setTargets(text)
                      toast.success(t.vulnScan.messages.imported.replace('{count}', String(text.split('\n').filter(Boolean).length)))
                    }
                    reader.readAsText(file)
                  }}
                  onDeduplicate={(text) => {
                    const lines = text.split('\n').filter((line) => line.trim())
                    const unique = Array.from(new Set(lines))
                    setTargets(unique.join('\n'))
                    toast.success(t.vulnScan.messages.deduplicated.replace('{count}', String(unique.length)))
                  }}
                  onCopy={() => {
                    toast.success(t.vulnScan.messages.copied)
                  }}
                  onClear={() => {
                    toast.success(t.vulnScan.messages.cleared)
                  }}
                />
              </div>
            ) : scanMode === ScanType.Crack ? (
              <div className="space-y-2">
                <TextInputWithActions
                  value={targets}
                  onChange={setTargets}
                  placeholder={t.vulnScan.newScan.addressPlaceholder}
                  disabled={isSubmitting}
                  height="200px"
                  onFileUpload={(file) => {
                    const reader = new FileReader()
                    reader.onload = (event) => {
                      const text = event.target?.result as string
                      setTargets(text)
                      toast.success(t.vulnScan.messages.imported.replace('{count}', String(text.split('\n').filter(Boolean).length)))
                    }
                    reader.readAsText(file)
                  }}
                  onDeduplicate={(text) => {
                    const lines = text.split('\n').filter((line) => line.trim())
                    const unique = Array.from(new Set(lines))
                    setTargets(unique.join('\n'))
                    toast.success(t.vulnScan.messages.deduplicated.replace('{count}', String(unique.length)))
                  }}
                  onCopy={() => {
                    toast.success(t.vulnScan.messages.copied)
                  }}
                  onClear={() => {
                    toast.success(t.vulnScan.messages.cleared)
                  }}
                />
                <p className="text-xs text-muted-foreground">
                  {t.vulnScan.newScan.supportedProtocols}
                </p>
              </div>
            ) : null}
          </div>

          {scanMode === ScanType.Subdomain && (
            <SubdomainConfigPanel
              value={{
                thread: threads,
                timeout: subdomainTimeout,
                appendEngines: selectedSubdomainEngines,
                dnsServers: subdomainDnsServers,
              }}
              onChange={(next) => {
                setThreads(next.thread)
                setSubdomainTimeout(next.timeout)
                setSelectedSubdomainEngines(next.appendEngines as SpaceEngineName[])
                if (next.dnsServers !== subdomainDnsServers) {
                  void persistSubdomainDnsServers(next.dnsServers)
                }
              }}
              spaceEngineConfig={spaceEngineConfig}
              isSavingDnsServers={isSavingSubdomainDnsServers}
              onConfigureApi={() => router.push('/settings')}
            />
          )}

          {/* 网站扫描特有配置 */}
          {scanMode === ScanType.WebScan && (
            <WebScanSettings
              webscanOption={webscanOption}
              setWebscanOption={setWebscanOption}
              customHeaders={customHeaders}
              setCustomHeaders={setCustomHeaders}
              customTemplate={customTemplate}
              setCustomTemplate={setCustomTemplate}
              customTags={customTags}
              setCustomTags={setCustomTags}
              threads={threads}
              setThreads={setThreads}
              nucleiThreads={nucleiThreads}
              setNucleiThreads={setNucleiThreads}
              activeTimeoutLimit={activeTimeoutLimit}
              setActiveTimeoutLimit={setActiveTimeoutLimit}
              disableMaxHostErr={disableMaxHostErr}
              setDisableMaxHostErr={setDisableMaxHostErr}
              rootPathScan={rootPathScan}
              setRootPathScan={setRootPathScan}
              skipNucleiWithoutTags={skipNucleiWithoutTags}
              setSkipNucleiWithoutTags={setSkipNucleiWithoutTags}
              skipNucleiWithWAF={skipNucleiWithWAF}
              setSkipNucleiWithWAF={setSkipNucleiWithWAF}
              screenshot={screenshot}
              setScreenshot={setScreenshot}
              oobEnabled={oobEnabled}
              setOobEnabled={setOobEnabled}
              interactshURL={interactshURL}
              setInteractshURL={setInteractshURL}
              interactshToken={interactshToken}
              setInteractshToken={setInteractshToken}
              scanTemplates={scanTemplates}
              selectedTemplateId={selectedTemplateId}
              setSelectedTemplateId={setSelectedTemplateId}
              onApplyTemplate={handleApplyTemplate}
              onClearTemplate={() => {
                setSelectedTemplateId('')
                setWebscanOption(3)
                setThreads(30)
                setNucleiThreads(2)
                setCustomHeaders('')
                setCustomTags([])
                setCustomTemplate([])
                setRootPathScan(false)
                setSkipNucleiWithoutTags(false)
                setScreenshot(false)
                setActiveTimeoutLimit(3)
                setDisableMaxHostErr(true)
                setOobEnabled(true)
                setInteractshURL('')
                setInteractshToken('')
              }}
              templateOptions={templateOptions}
              fingerprintOptions={fingerprintOptions}
            />
          )}

          {/* 目录扫描特有配置 */}
          {scanMode === ScanType.DirScan && (
            <DirScanSettings
              method={dirMethod}
              setMethod={setDirMethod}
              extensions={dirExtensions}
              setExtensions={setDirExtensions}
              statusFilter={dirStatusFilter}
              setStatusFilter={setDirStatusFilter}
              customDict={dirCustomDict}
              setCustomDict={setDirCustomDict}
              availableDicts={dirAvailableDicts}
              setAvailableDicts={setDirAvailableDicts}
              selectedDicts={dirSelectedDicts}
              setSelectedDicts={setDirSelectedDicts}
              loadingDicts={dirLoadingDicts}
              advancedConfig={dirAdvancedConfig}
              setAdvancedConfig={setDirAdvancedConfig}
              defaultDictionaries={[
                {
                  id: 'dicc',
                  name: 'dicc.txt',
                  path: '',
                  description: 'back/#/login',
                  size: 0,
                },
                {
                  id: 'springboot',
                  name: 'springboot.txt',
                  path: '',
                  description: '/actuator',
                  size: 0,
                },
                {
                  id: 'backup',
                  name: 'backup.txt',
                  path: '',
                  description: 'sql.zip',
                  size: 0,
                },
              ]}
              onDictionaryUpload={handleDirDictionaryUpload}
              onReloadDicts={handleReloadDirDicts}
              onToast={(message) => toast.info(message)}
              scanTemplates={scanTemplates}
              selectedTemplateId={selectedTemplateId}
              setSelectedTemplateId={setSelectedTemplateId}
              onApplyTemplate={handleApplyTemplate}
              onClearTemplate={() => {
                setSelectedTemplateId('')
                setDirMethod('GET')
                setDirExtensions('php,aspx,asp,jsp,html,js')
                setDirStatusFilter('404')
                setDirCustomDict('')
                setDirSelectedDicts([])
                setDirAdvancedConfig({
                  thread: 25,
                  timeout: 8,
                  interval: 0,
                  recursion: 0,
                  times: 5,
                  exclude: '',
                  headers: '',
                  redirectClient: false,
                  backupfileScan: false,
                })
              }}
            />
          )}

          {/* 端口扫描特有配置 */}
          {scanMode === ScanType.PortScan && (
            <PortScanSettings
              portGroup={portGroup}
              setPortGroup={setPortGroup}
              portRange={portRange}
              setPortRange={setPortRange}
              threads={threads}
              setThreads={setThreads}
              portTimeout={portTimeout}
              setPortTimeout={setPortTimeout}
              tcpHeuristic={tcpHeuristic}
              setTcpHeuristic={(value) => setTcpHeuristic((previous) => ({ ...previous, ...value }))}
              aliveDetection={aliveDetection}
              setAliveDetection={setAliveDetection}
              tcpDiscoveryMode={tcpDiscoveryMode}
              setTcpDiscoveryMode={setTcpDiscoveryMode}
              synRate={synRate}
              setSynRate={setSynRate}
              synCapability={synCapability}
              loadingSynCapability={loadingSynCapability}
              authorizingSyn={authorizingSyn}
              desktopRuntime={desktopRuntime}
              runMode={runMode}
              onRefreshSynCapability={loadSYNCapabilityStatus}
              onAuthorizeSYN={handleAuthorizeSYN}
              scanTemplates={scanTemplates}
              selectedTemplateId={selectedTemplateId}
              setSelectedTemplateId={setSelectedTemplateId}
              onApplyTemplate={handleApplyTemplate}
              onClearTemplate={() => {
                setSelectedTemplateId('')
                setPortGroup(BUILTIN_DATABASE_PORT_GROUP_TEXT)
                setPortRange(BUILTIN_DATABASE_PORT_GROUP_VALUE)
                setThreads(30)
                setPortTimeout(10)
                setAliveDetection(true)
                setTcpDiscoveryMode('connect')
                setSynRate(3000)
              }}
            />
          )}

          {/* 密码暴破特有配置 */}
          {scanMode === ScanType.Crack && (
            <CrackScanSettings
              username={username}
              setUsername={setUsername}
              password={password}
              setPassword={setPassword}
              defaultDict={defaultDict}
              setDefaultDict={setDefaultDict}
              isSubmitting={isSubmitting}
              scanTemplates={scanTemplates}
              selectedTemplateId={selectedTemplateId}
              setSelectedTemplateId={setSelectedTemplateId}
              onApplyTemplate={handleApplyTemplate}
              onClearTemplate={() => {
                setSelectedTemplateId('')
                setUsername('')
                setPassword('')
                setDefaultDict(true)
              }}
            />
          )}

          {/* JS扫描特有配置 */}
          {scanMode === ScanType.JsScan && (
            <JsScanSettings
              blackDomain={blackDomain}
              setBlackDomain={setBlackDomain}
              highRiskRouter={highRiskRouter}
              setHighRiskRouter={setHighRiskRouter}
              authentication={authentication}
              setAuthentication={setAuthentication}
              placeholder={placeholder}
              setPlaceholder={setPlaceholder}
              sqlInjectionEnabled={sqlInjectionEnabled}
              setSqlInjectionEnabled={setSqlInjectionEnabled}
              sqlInjectionRules={sqlInjectionRules}
              setSqlInjectionRules={setSqlInjectionRules}
              lfiEnabled={lfiEnabled}
              setLfiEnabled={setLfiEnabled}
              lfiParamKeywords={lfiParamKeywords}
              setLfiParamKeywords={setLfiParamKeywords}
              lfiRules={lfiRules}
              setLfiRules={setLfiRules}
              ssrfEnabled={ssrfEnabled}
              setSsrfEnabled={setSsrfEnabled}
              ssrfParamKeywords={ssrfParamKeywords}
              setSsrfParamKeywords={setSsrfParamKeywords}
              redirectEnabled={redirectEnabled}
              setRedirectEnabled={setRedirectEnabled}
              redirectParamKeywords={redirectParamKeywords}
              setRedirectParamKeywords={setRedirectParamKeywords}
              xssEnabled={xssEnabled}
              setXssEnabled={setXssEnabled}
              xssRules={xssRules}
              setXssRules={setXssRules}
              uploadEnabled={uploadEnabled}
              setUploadEnabled={setUploadEnabled}
              uploadParamKeywords={uploadParamKeywords}
              setUploadParamKeywords={setUploadParamKeywords}
              uploadTestContent={uploadTestContent}
              setUploadTestContent={setUploadTestContent}
              uploadTestFileName={uploadTestFileName}
              setUploadTestFileName={setUploadTestFileName}
              aiDetectionEnabled={aiDetectionEnabled}
              setAiDetectionEnabled={setAiDetectionEnabled}
              aiDetectionAvailable={aiDetectionAvailable}
              scanTemplates={scanTemplates}
              selectedTemplateId={selectedTemplateId}
              setSelectedTemplateId={setSelectedTemplateId}
              onApplyTemplate={handleApplyTemplate}
              onClearTemplate={() => {
                setSelectedTemplateId('')
                // 重新加载默认配置
                fetchWithAuth('/api/v1/jsscan/default-config')
                  .then((res) => res.json())
                  .then((data) => {
                    if (data.success && data.data) {
                      const config = data.data
                      if (config.blackDomain) setBlackDomain(config.blackDomain)
                      if (config.highRiskRouter) setHighRiskRouter(config.highRiskRouter)
                      if (config.authentication) setAuthentication(config.authentication)
                      if (config.placeholder) setPlaceholder(config.placeholder)
                      if (config.vulnDetection) {
                        const vuln = config.vulnDetection
                        setSqlInjectionEnabled(vuln.enabled !== false)
                      }
                      setAiDetectionEnabled((config.openai?.enabled ?? false) && aiDetectionAvailable)
                    }
                  })
                  .catch(() => {
                    // 回退到默认值
                    setBlackDomain([])
                    setHighRiskRouter([])
                    setAuthentication([])
                    setPlaceholder({})
                    setSqlInjectionEnabled(true)
                    setLfiEnabled(true)
                    setSsrfEnabled(true)
                    setRedirectEnabled(true)
                    setXssEnabled(true)
                    setUploadEnabled(true)
                    setAiDetectionEnabled(false)
                  })
              }}
            />
          )}

        </div>
      </div>

      {/* 操作按钮 */}
      <div className="sticky bottom-0 z-30 border-t border-border bg-background/95 px-6 py-3 shadow-[0_-8px_24px_rgba(15,23,42,0.06)] backdrop-blur supports-[backdrop-filter]:bg-background/85">
        <div className="mx-auto flex max-w-3xl justify-end gap-2">
          <Button
            variant="outline"
            onClick={handleCancel}
            disabled={isSubmitting}
            type="button"
          >
            {t.vulnScan.newScan.cancel}
          </Button>
          <Button
            onClick={(e) => {
              handleStart(e)
            }}
            disabled={isSubmitting}
            type="button"
          >
            {isSubmitting && <Loader2 className="h-4 w-4 animate-spin" />}
            <span aria-live="polite">
              {isSubmitting
                ? t.vulnScan.newScan.creating
                : taskSchedule.spec === null
                  ? t.vulnScan.start
                  : t.vulnScan.newScan.createScheduledTask}
            </span>
          </Button>
        </div>
      </div>

      {/* 子域名任务导入对话框 */}
      <Dialog open={subdomainTaskDialogOpen} onOpenChange={setSubdomainTaskDialogOpen}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>{t.vulnScan.newScan.subdomainTaskImport.title}</DialogTitle>
            <DialogDescription>
              {t.vulnScan.newScan.subdomainTaskImport.desc}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {loadingSubdomainTasks ? (
              <div className="text-center py-8 text-muted-foreground">{t.vulnScan.newScan.subdomainTaskImport.loading}</div>
            ) : subdomainTasks.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">{t.vulnScan.newScan.subdomainTaskImport.noTasks}</div>
            ) : (
              <div className="max-h-[50vh] overflow-y-auto space-y-2">
                {subdomainTasks.map((task) => (
                  <div
                    key={task.id}
                    className="flex items-center justify-between p-4 border rounded-lg hover:bg-muted/50 cursor-pointer transition-colors"
                    onClick={() => handleImportSubdomainTask(task)}
                  >
                    <div className="flex-1">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="font-medium" title={task.taskName || task.id}>
                          {task.taskName || task.id}
                        </span>
                        <Badge
                          variant="outline"
                          className={
                            task.status === 'completed'
                              ? 'bg-green-500/20 text-green-400 border-green-500/30'
                              : task.status === 'failed'
                                ? 'bg-red-500/20 text-red-400 border-red-500/30'
                                : 'bg-amber-500/20 text-amber-400 border-amber-500/30'
                          }
                        >
                          {task.status === 'completed'
                            ? t.vulnScan.newScan.subdomainTaskImport.completed
                            : task.status === 'failed'
                              ? t.vulnScan.statuses.failed
                              : t.vulnScan.statuses.cancelled}
                        </Badge>
                      </div>
                      <div className="text-sm text-muted-foreground">
                        {t.vulnScan.newScan.subdomainTaskImport.resultCount}: {task.resultCount || 0} | {t.vulnScan.newScan.subdomainTaskImport.createTime}:{' '}
                        {new Date(task.createdAt).toLocaleString('zh-CN')}
                      </div>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation()
                        handleImportSubdomainTask(task)
                      }}
                      disabled={importingSubdomainTask}
                    >
                      {importingSubdomainTask ? t.vulnScan.newScan.subdomainTaskImport.importing : t.vulnScan.newScan.subdomainTaskImport.import}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* 端口扫描导入对话框 */}
      <Dialog open={portScanDialogOpen} onOpenChange={setPortScanDialogOpen}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl">
            <DialogHeader>
            <DialogTitle>{t.vulnScan.newScan.portScanImport.title}</DialogTitle>
            <DialogDescription>
              {scanMode === ScanType.WebScan
                ? t.vulnScan.newScan.portScanImport.descWeb
                : t.vulnScan.newScan.portScanImport.descCrack}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            {loadingPortScanTasks ? (
              <div className="text-center py-8 text-muted-foreground">{t.vulnScan.newScan.portScanImport.loading}</div>
            ) : portScanTasks.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground">{t.vulnScan.newScan.portScanImport.noTasks}</div>
            ) : (
              <div className="max-h-[50vh] overflow-y-auto space-y-2">
                {portScanTasks.map((task) => (
                  <div
                    key={task.id}
                    className="flex items-center justify-between p-4 border rounded-lg hover:bg-muted/50 cursor-pointer transition-colors"
                    onClick={() => handleImportPortScan(task)}
                  >
                    <div className="flex-1">
                      <div className="flex items-center gap-2 mb-1">
                        <span className="font-medium">
                          {task.taskName || task.id}
                        </span>
                        <Badge
                          variant="outline"
                          className={
                            task.status === 'completed'
                              ? 'bg-green-500/20 text-green-400 border-green-500/30'
                              : task.status === 'failed'
                                ? 'bg-red-500/20 text-red-400 border-red-500/30'
                                : 'bg-amber-500/20 text-amber-400 border-amber-500/30'
                          }
                        >
                          {task.status === 'completed'
                            ? t.vulnScan.newScan.portScanImport.completed
                            : task.status === 'failed'
                              ? t.vulnScan.statuses.failed
                              : t.vulnScan.statuses.cancelled}
                        </Badge>
                      </div>
                      <div className="text-sm text-muted-foreground">
                        {t.vulnScan.newScan.portScanImport.portCount}: {task.portCount || 0} | {t.vulnScan.newScan.portScanImport.createTime}:{' '}
                        {new Date(task.createdAt).toLocaleString('zh-CN')}
                      </div>
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation()
                        handleImportPortScan(task)
                      }}
                      disabled={importingPortScan}
                    >
                      {importingPortScan ? t.vulnScan.newScan.portScanImport.importing : t.vulnScan.newScan.portScanImport.import}
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function LoadingFallback() {
  const { t } = useI18n()
  return (
    <div className="flex-1 flex items-center justify-center">
      <div className="text-muted-foreground">{t.vulnScan.newScan.portScanImport.loading}</div>
    </div>
  )
}

export default function NewScanPage() {
  return (
    <Suspense fallback={<LoadingFallback />}>
      <NewScanForm />
    </Suspense>
  )
}

function CombinationTaskCreation({ plannerMode = 'manual' }: { plannerMode?: 'manual' | 'smart' }) {
  const { t } = useI18n()
  const router = useRouter()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [initialDraft, setInitialDraft] = useState<CombinationDraft | null>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => {
      try {
        const raw = window.sessionStorage.getItem(ASSET_SCAN_COMBINATION_DRAFT_KEY)
        if (!raw) return
        window.sessionStorage.removeItem(ASSET_SCAN_COMBINATION_DRAFT_KEY)
        const parsed = JSON.parse(raw)
        if (parsed && Array.isArray(parsed.nodes) && Array.isArray(parsed.targets)) {
          setInitialDraft(parsed as CombinationDraft)
        }
      } catch {
        // A malformed hand-off should not block the normal combination editor.
      }
    }, 0)
    return () => window.clearTimeout(timer)
  }, [])

  const handleCreateCombinationTask = async (draft: CombinationDraft) => {
    try {
      setIsSubmitting(true)
      const proxy = draft.proxy?.trim() || getProxyUrl()
      const payload = proxy ? { ...draft, proxy } : draft
      const response = await fetchWithAuth(`/api/v1/webscan/combination/tasks`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json().catch(() => ({}))
      if (data && data.success === false) {
        throw new Error(data.error || t.vulnScan.newScan.messages.createCombinationFailed)
      }

      const created = data?.data || {}

      toast.success(t.vulnScan.newScan.messages.combinationSaved)
      router.push('/scan/vuln-scan')
      return created
    } catch (error: any) {
      console.error('Create combination task error:', error)
      toast.error(`${t.vulnScan.newScan.messages.createCombinationFailed}: ${error?.message || t.vulnScan.messages.unknownError}`)
      throw error
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="min-w-0 p-3 md:p-6 space-y-6">
      <CombinationTaskInline
        onCreate={handleCreateCombinationTask}
        isSubmitting={isSubmitting}
        onCancel={() => router.back()}
        initialDraft={initialDraft}
      />
    </div>
  )
}
