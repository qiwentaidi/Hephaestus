'use client'

import { type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { fetchWithAuth, getApiBaseURL } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { getDesktopRuntimeMeta, type DesktopRuntimeMeta } from '@/lib/desktop'
import { getDesktopApiHeaders } from '@/lib/env'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import {
  CheckCircle2,
  CircleAlert,
  Copy,
  Globe2,
  KeyRound,
  Loader2,
  RefreshCw,
  Search,
  Send,
  Server,
  ShieldCheck,
  XCircle,
} from 'lucide-react'

const MCP_PROTOCOL_VERSION = '2025-06-18'
const MCP_ENDPOINT_PATH = '/api/v1/mcp/hephaestus'
const MCP_AUDIT_ENDPOINT_PATH = '/api/v1/mcp/space-engine/audit'
const MCP_BASIC_AUTH_PLACEHOLDER = 'Basic BASE64(username:password)'
const PAGE_BACKGROUND_CLASS = 'bg-background'
const PANEL_CLASS = 'border border-border/70 bg-card text-card-foreground shadow-sm'
const SUBTLE_PANEL_CLASS = 'border border-border/70 bg-muted/30'
const CODE_SURFACE_CLASS = 'border border-border bg-muted/40'

type MCPToolSchemaField = {
  type?: string
  description?: string
  items?: { type?: string }
  minimum?: number
}

type MCPToolSchema = {
  type?: string
  properties?: Record<string, MCPToolSchemaField>
  required?: string[]
}

type MCPTool = {
  name: string
  moduleKey?: string
  moduleName?: string
  moduleNameI18n?: Record<string, string>
  title?: string
  titleI18n?: Record<string, string>
  version?: string
  description?: string
  descriptionI18n?: Record<string, string>
  inputSchema?: MCPToolSchema
  inputSchemaI18n?: Record<string, MCPToolSchema>
}

type MCPResponse<T = unknown> = {
  jsonrpc: string
  id?: string | number | null
  result?: T
  error?: {
    code: number
    message: string
    data?: unknown
  }
}

type MCPAuditLog = {
  id: string
  server: string
  method: string
  toolName?: string
  sessionId?: string
  clientName?: string
  clientVersion?: string
  userAgent?: string
  sourceIp?: string
  status: 'success' | 'failed' | string
  httpStatus?: number
  errorCode?: number
  errorMessage?: string
  durationMs?: number
  createdAt: string
}

type MCPAuditPagePayload = {
  items: MCPAuditLog[]
  total: number
  page: number
  pageSize: number
}

type NetworkCard = {
  name: string
  ip: string
}
type ClientConfigKey = 'claude' | 'cursor' | 'codex' | 'raw'
type HandshakeStepKey = 'initialize' | 'session' | 'initialized' | 'tools'
type HandshakeStepStatus = 'pending' | 'success' | 'error'
type ToolModuleKey = 'all' | string
type AuditKindFilter = 'important' | 'all' | 'tool_call' | 'protocol'
type AuditStatusFilter = 'all' | 'success' | 'failed'
type AuditStatus = Exclude<AuditStatusFilter, 'all'>

const TOOL_ARGUMENT_EXAMPLES: Record<string, string> = {
  space_engine_search: JSON.stringify(
    {
      query: 'title:"login"',
      queryType: 'default',
      engines: ['fofa', 'hunter'],
      page: 1,
      pageSize: 20,
    },
    null,
    2,
  ),
  space_engine_get_quota: '{}',
  space_engine_get_config_status: '{}',
}

const CLIENT_CONFIG_LABELS: Record<ClientConfigKey, string> = {
  claude: 'Claude',
  cursor: 'Cursor',
  codex: 'Codex',
  raw: 'Raw JSON',
}

const HANDSHAKE_STEPS: HandshakeStepKey[] = ['initialize', 'session', 'initialized', 'tools']

function createInitialHandshakeStatus(): Record<HandshakeStepKey, HandshakeStepStatus> {
  return {
    initialize: 'pending',
    session: 'pending',
    initialized: 'pending',
    tools: 'pending',
  }
}

function maskSessionId(sessionId: string | null) {
  if (!sessionId) return 'not established'
  if (sessionId.length <= 12) return sessionId
  return `${sessionId.slice(0, 8)}...${sessionId.slice(-6)}`
}

function parseJsonSafe(value: string) {
  try {
    return JSON.parse(value)
  } catch {
    return {}
  }
}

function formatAuditTime(value: string) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function getAuditClientLabel(log: MCPAuditLog) {
  const clientName = (log.clientName || '').trim()
  const clientVersion = (log.clientVersion || '').trim()
  if (clientName && clientVersion) return `${clientName}/${clientVersion}`
  if (clientName) return clientName
  return '-'
}

function getAuditDisplayTarget(log: MCPAuditLog) {
  const toolName = (log.toolName || '').trim()
  if (toolName) return toolName
  return log.method || '-'
}

export default function PlatformMcpPageClient() {
  const { t, language } = useI18n()
  const sessionIdRef = useRef<string | null>(null)
  const auditPageRef = useRef(1)
  const auditPageSizeRef = useRef(10)
  const auditKindRef = useRef<AuditKindFilter>('important')
  const auditStatusRef = useRef<AuditStatus[]>([])
  const auditKeywordRef = useRef('')
  const [apiBaseUrl, setApiBaseUrl] = useState('http://localhost:8080')
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [debugLoading, setDebugLoading] = useState(false)
  const [auditLoading, setAuditLoading] = useState(false)
  const [networkLoading, setNetworkLoading] = useState(false)
  const [listenApplying, setListenApplying] = useState(false)
  const [connected, setConnected] = useState(false)
  const [protocolVersion, setProtocolVersion] = useState(MCP_PROTOCOL_VERSION)
  const [tools, setTools] = useState<MCPTool[]>([])
  const [auditLogs, setAuditLogs] = useState<MCPAuditLog[]>([])
  const [auditTotal, setAuditTotal] = useState(0)
  const [auditPage, setAuditPage] = useState(1)
  const [auditPageSize, setAuditPageSize] = useState(10)
  const [auditKind, setAuditKind] = useState<AuditKindFilter>('important')
  const [auditStatuses, setAuditStatuses] = useState<AuditStatus[]>([])
  const [auditKeyword, setAuditKeyword] = useState('')
  const [submittedAuditKeyword, setSubmittedAuditKeyword] = useState('')
  const [selectedTool, setSelectedTool] = useState('space_engine_search')
  const [pageError, setPageError] = useState('')
  const [debugError, setDebugError] = useState('')
  const [auditError, setAuditError] = useState('')
  const [debugResult, setDebugResult] = useState('')
  const [lastSyncedAt, setLastSyncedAt] = useState('')
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [networkCards, setNetworkCards] = useState<NetworkCard[]>([])
  const [selectedListenHost, setSelectedListenHost] = useState('127.0.0.1')
  const [selectedListenPort, setSelectedListenPort] = useState('8080')
  const [appliedListenHost, setAppliedListenHost] = useState('127.0.0.1')
  const [appliedListenPort, setAppliedListenPort] = useState('8080')
  const [appliedMcpBaseUrl, setAppliedMcpBaseUrl] = useState('')
  const [clientConfigKey, setClientConfigKey] = useState<ClientConfigKey>('claude')
  const [toolModuleFilter, setToolModuleFilter] = useState<ToolModuleKey>('all')
  const [toolSearchQuery, setToolSearchQuery] = useState('')
  const [handshakeStatus, setHandshakeStatus] = useState(createInitialHandshakeStatus)
  const [desktopMeta, setDesktopMeta] = useState<DesktopRuntimeMeta | null>(null)

  const pageCopy = (t as any).mcpPage ?? {
    copySuccess: 'Copied to clipboard',
    copyFailed: 'Copy failed',
    refresh: 'Refresh',
  }
  const formatCopy = useCallback((template: string, values: Record<string, string | number>) => {
    return Object.entries(values).reduce(
      (text, [key, value]) => text.replaceAll(`{${key}}`, String(value)),
      template,
    )
  }, [])

  const buildMcpBaseUrlForHost = useCallback((host: string, port: string) => {
    const fallback = apiBaseUrl || getApiBaseURL()
    const normalizedPort = port.trim()
    const displayHost = host === '0.0.0.0'
      ? (networkCards.find((card) => {
        const ip = card.ip?.trim()
        return ip && ip !== '127.0.0.1' && ip !== '0.0.0.0'
      })?.ip || '127.0.0.1')
      : host
    try {
      const current = new URL(desktopMeta?.serverURL || fallback)
      current.hostname = displayHost
      if (normalizedPort) {
        current.port = normalizedPort
      }
      return current.toString().replace(/\/$/, '')
    } catch {
      return `http://${displayHost}${normalizedPort ? `:${normalizedPort}` : ''}`
    }
  }, [apiBaseUrl, desktopMeta, networkCards])

  const loadNetworkCards = useCallback(async () => {
    setNetworkLoading(true)
    try {
      const response = await fetchWithAuth('/api/v1/system/network-cards')
      const payload = await response.json().catch(() => null) as { data?: NetworkCard[]; error?: string } | null
      if (!response.ok) {
        throw new Error(payload?.error || `HTTP ${response.status}`)
      }
      setNetworkCards(Array.isArray(payload?.data) ? payload.data : [])
    } catch (error) {
      toast.error(error instanceof Error ? error.message : pageCopy.loadNetworkCardsFailed)
    } finally {
      setNetworkLoading(false)
    }
  }, [pageCopy.loadNetworkCardsFailed])

  useEffect(() => {
    let cancelled = false
    const timer = window.setTimeout(() => {
      setApiBaseUrl(getApiBaseURL())

      void getDesktopRuntimeMeta()
        .then((meta) => {
          if (cancelled) return
          setDesktopMeta(meta)
          if (meta?.isDesktop) {
            if (meta.serverURL) {
              setApiBaseUrl(meta.serverURL)
            }
            if (meta.mcpListenHost) {
              setSelectedListenHost(meta.mcpListenHost)
              setAppliedListenHost(meta.mcpListenHost)
            }
            if (meta.mcpListenPort || meta.backendPort) {
              const port = meta.mcpListenPort || meta.backendPort || '8080'
              setSelectedListenPort(port)
              setAppliedListenPort(port)
            }
            if (meta.mcpServerURL) {
              setAppliedMcpBaseUrl(meta.mcpServerURL)
            }
          } else {
            try {
              const current = new URL(getApiBaseURL())
              if (current.port) {
                setSelectedListenPort(current.port)
                setAppliedListenPort(current.port)
              }
            } catch {
              // Keep default port when the configured base URL is not parseable.
            }
            void fetchWithAuth('/api/v1/mcp/listener')
              .then(async (response) => {
                const payload = await response.json().catch(() => null) as {
                  success?: boolean
                  data?: {
                    mcpListenHost?: string
                    mcpListenPort?: string
                    mcpServerURL?: string
                  }
                } | null
                if (cancelled || !response.ok || !payload?.success) return
                const host = payload.data?.mcpListenHost?.trim()
                if (host) {
                  setSelectedListenHost(host)
                  setAppliedListenHost(host)
                }
                const port = payload.data?.mcpListenPort?.trim()
                if (port) {
                  setSelectedListenPort(port)
                  setAppliedListenPort(port)
                }
                if (payload.data?.mcpServerURL) {
                  setAppliedMcpBaseUrl(payload.data.mcpServerURL)
                }
              })
              .catch(() => {
                // Silently keep the URL-derived fallback values.
              })
          }
        })
        .catch(() => {
          if (!cancelled) {
            setDesktopMeta(null)
          }
        })
    }, 0)

    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadNetworkCards()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadNetworkCards])

  const isDesktopMode = Boolean(desktopMeta?.isDesktop)
  const listenOptions = useMemo(() => {
    const items: Array<{ key: string; label: string; description: string }> = [{
      key: '127.0.0.1',
      label: '127.0.0.1',
      description: pageCopy.localAccess,
    }, {
      key: '0.0.0.0',
      label: '0.0.0.0',
      description: pageCopy.allNetworkCards,
    }]
    const seen = new Set(items.map((item) => item.key))
    for (const card of networkCards) {
      const ip = card.ip?.trim()
      if (!ip || seen.has(ip)) continue
      seen.add(ip)
      items.push({
        key: ip,
        label: ip,
        description: card.name || pageCopy.networkCardAddress,
      })
    }
    return items
  }, [networkCards, pageCopy.allNetworkCards, pageCopy.localAccess, pageCopy.networkCardAddress])

  const effectiveBaseUrl = useMemo(() => {
    return appliedMcpBaseUrl || desktopMeta?.mcpServerURL || desktopMeta?.serverURL || apiBaseUrl
  }, [apiBaseUrl, appliedMcpBaseUrl, desktopMeta?.mcpServerURL, desktopMeta?.serverURL])

  const accessScopeLabel = isDesktopMode
    ? (appliedListenHost === '127.0.0.1' ? pageCopy.localAiAccessible : pageCopy.lanClientAccessible)
    : (appliedListenHost === '127.0.0.1' ? pageCopy.localhostOnly : pageCopy.lanClientAccessible)
  const baseUrlStatus = isDesktopMode
    ? (desktopMeta?.isTemporaryBackendPort ? pageCopy.temporaryPort : pageCopy.localFixed)
    : (connected ? pageCopy.reachable : pageCopy.pendingVerification)
  const baseUrlTone = isDesktopMode && desktopMeta?.isTemporaryBackendPort
    ? 'warning'
    : (connected ? 'success' : 'warning')

  const absoluteEndpoint = useMemo(
    () => `${effectiveBaseUrl}${MCP_ENDPOINT_PATH}`,
    [effectiveBaseUrl],
  )

  const getLocalizedToolTitle = useCallback((tool?: MCPTool) => {
    if (!tool) return ''
    return tool.titleI18n?.[language] || tool.title || tool.name
  }, [language])

  const getLocalizedToolDescription = useCallback((tool?: MCPTool) => {
    if (!tool) return pageCopy.waitingToolDescription
    return tool.descriptionI18n?.[language] || tool.description || pageCopy.noDescription
  }, [language, pageCopy.noDescription, pageCopy.waitingToolDescription])

  const getToolModuleKey = useCallback((tool?: MCPTool) => {
    return tool?.moduleKey || 'space_engine'
  }, [])

  const getLocalizedToolModuleName = useCallback((tool?: MCPTool) => {
    if (!tool) return pageCopy.spaceEngine
    return tool.moduleNameI18n?.[language] || tool.moduleName || pageCopy.spaceEngine
  }, [language, pageCopy.spaceEngine])

  const markFirstPendingHandshakeStepAsError = useCallback(() => {
    setHandshakeStatus((current) => {
      const next = { ...current }
      const firstPending = HANDSHAKE_STEPS.find((step) => next[step] === 'pending')
      if (firstPending) {
        next[firstPending] = 'error'
      }
      return next
    })
  }, [])

  const callMcp = useCallback(
    async <T,>(method: string, params?: Record<string, unknown>, init?: { sessionId?: string | null }) => {
      const headers: Record<string, string> = {
        'Content-Type': 'application/json',
        'MCP-Protocol-Version': protocolVersion,
      }

      const activeSessionId = init?.sessionId ?? sessionIdRef.current
      if (activeSessionId) {
        headers['Mcp-Session-Id'] = activeSessionId
      }

      const response = await fetchWithAuth(MCP_ENDPOINT_PATH, {
        method: 'POST',
        headers,
        body: JSON.stringify({
          jsonrpc: '2.0',
          id: Date.now(),
          method,
          params,
        }),
      })

      const sessionHeader = response.headers.get('Mcp-Session-Id')
      if (sessionHeader) {
        sessionIdRef.current = sessionHeader
        setSessionId(sessionHeader)
      }

      if (response.status === 202) {
        return undefined as T
      }

      const rawText = await response.text()
      if (!rawText.trim()) {
        if (!response.ok) {
          throw new Error(`MCP request failed with HTTP ${response.status}`)
        }
        return undefined as T
      }

      let payload: MCPResponse<T>
      try {
        payload = JSON.parse(rawText) as MCPResponse<T>
      } catch {
        throw new Error(rawText)
      }

      if (payload.error) {
        throw new Error(payload.error.message || 'MCP request failed')
      }
      if (!response.ok) {
        throw new Error(`MCP request failed with HTTP ${response.status}`)
      }

      return payload.result as T
    },
    [protocolVersion],
  )

  const destroySession = useCallback(async () => {
    const activeSessionId = sessionIdRef.current
    if (!activeSessionId) return

    try {
      await fetchWithAuth(MCP_ENDPOINT_PATH, {
        method: 'DELETE',
        headers: {
          'Mcp-Session-Id': activeSessionId,
        },
      })
    } catch {
      // Session cleanup is best-effort; expired sessions are also cleaned by the server.
    } finally {
      sessionIdRef.current = null
      setSessionId(null)
    }
  }, [])

  const loadAuditLogs = useCallback(async (page = auditPageRef.current, pageSize = auditPageSizeRef.current) => {
    setAuditLoading(true)
    setAuditError('')

    try {
      const params = new URLSearchParams({
        page: String(page),
        pageSize: String(pageSize),
        hours: '24',
      })
      if (auditKindRef.current !== 'all') {
        params.set('kind', auditKindRef.current)
      }
      if (auditStatusRef.current.length > 0) {
        params.set('status', auditStatusRef.current.join(','))
      }
      const auditKeyword = auditKeywordRef.current.trim()
      if (auditKeyword) {
        params.set('keyword', auditKeyword)
      }
      const response = await fetchWithAuth(`${MCP_AUDIT_ENDPOINT_PATH}?${params.toString()}`)
      const payload = await response.json().catch(() => null) as {
        success?: boolean
        data?: MCPAuditPagePayload | MCPAuditLog[]
        error?: string
      } | null

      if (!response.ok || !payload?.success) {
        throw new Error(payload?.error || formatCopy(pageCopy.loadAuditFailedWithStatus, { status: response.status }))
      }

      if (Array.isArray(payload.data)) {
        auditPageRef.current = 1
        auditPageSizeRef.current = pageSize
        setAuditLogs(payload.data)
        setAuditTotal(payload.data.length)
        setAuditPage(1)
        setAuditPageSize(pageSize)
        return
      }

      const data = payload.data
      const nextItems = Array.isArray(data?.items) ? data.items : []
      const nextPage = typeof data?.page === 'number' ? data.page : page
      const nextPageSize = typeof data?.pageSize === 'number' ? data.pageSize : pageSize
      const nextTotal = typeof data?.total === 'number' ? data.total : nextItems.length

      auditPageRef.current = nextPage
      auditPageSizeRef.current = nextPageSize
      setAuditLogs(nextItems)
      setAuditTotal(nextTotal)
      setAuditPage(nextPage)
      setAuditPageSize(nextPageSize)
    } catch (error) {
      setAuditError(error instanceof Error ? error.message : pageCopy.loadAuditFailed)
      setAuditLogs([])
    } finally {
      setAuditLoading(false)
    }
  }, [formatCopy, pageCopy.loadAuditFailed, pageCopy.loadAuditFailedWithStatus])

  const bootstrap = useCallback(
    async (mode: 'initial' | 'refresh' = 'initial') => {
      if (mode === 'initial') {
        setLoading(true)
      } else {
        setRefreshing(true)
      }

      setPageError('')
      setDebugError('')
      setHandshakeStatus(createInitialHandshakeStatus())

      try {
        await destroySession()

        const initializeResult = await callMcp<{
          protocolVersion: string
          serverInfo?: { name?: string; version?: string }
        }>(
          'initialize',
          {
            protocolVersion: MCP_PROTOCOL_VERSION,
            clientInfo: {
              name: 'hephaestus-web-console',
              version: '1.0.0',
            },
          },
          { sessionId: null },
        )

        setHandshakeStatus((current) => ({ ...current, initialize: 'success' }))
        setProtocolVersion(initializeResult?.protocolVersion || MCP_PROTOCOL_VERSION)

        if (!sessionIdRef.current) {
          setHandshakeStatus((current) => ({ ...current, session: 'error' }))
          throw new Error(pageCopy.sessionHeaderMissing)
        }

        setHandshakeStatus((current) => ({ ...current, session: 'success' }))

        await callMcp('notifications/initialized')
        setHandshakeStatus((current) => ({ ...current, initialized: 'success' }))

        const listResult = await callMcp<{ tools: MCPTool[] }>('tools/list')
        const nextTools = listResult?.tools || []
        setTools(nextTools)
        setSelectedTool((current) => (
          nextTools.some((tool) => tool.name === current) ? current : (nextTools[0]?.name || current)
        ))
        setConnected(true)
        setLastSyncedAt(new Date().toLocaleString())
        setHandshakeStatus((current) => ({ ...current, tools: 'success' }))
        await loadAuditLogs(1, auditPageSizeRef.current)
      } catch (error) {
        setConnected(false)
        markFirstPendingHandshakeStepAsError()
        setPageError(error instanceof Error ? error.message : pageCopy.bootstrapFailed)
        void loadAuditLogs()
      } finally {
        setLoading(false)
        setRefreshing(false)
      }
    },
    [callMcp, destroySession, loadAuditLogs, markFirstPendingHandshakeStepAsError, pageCopy.bootstrapFailed, pageCopy.sessionHeaderMissing],
  )

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void bootstrap('initial')
    }, 0)

    return () => {
      window.clearTimeout(timer)
      void destroySession()
    }
  }, [bootstrap, destroySession])

  const handleCopy = useCallback(async (text: string) => {
    const ok = await copyToClipboard(text)
    if (ok) {
      toast.success(pageCopy.copySuccess)
    } else {
      toast.error(pageCopy.copyFailed)
    }
  }, [pageCopy.copyFailed, pageCopy.copySuccess])

  const selectedToolPayloadPreview = useMemo(() => {
    if (selectedTool === 'space_engine_search') {
      return JSON.stringify(
        {
          query: pageCopy.debugSearchQueryExample,
          queryType: 'default',
          engines: ['fofa', 'hunter'],
          page: 1,
          pageSize: 20,
        },
        null,
        2,
      )
    }
    return TOOL_ARGUMENT_EXAMPLES[selectedTool] || '{}'
  }, [pageCopy.debugSearchQueryExample, selectedTool])

  const debugRequestPreview = useMemo(() => {
    return JSON.stringify(
      {
        jsonrpc: '2.0',
        id: 3,
        method: 'tools/call',
        params: {
          name: selectedTool,
          arguments: parseJsonSafe(selectedToolPayloadPreview),
        },
      },
      null,
      2,
    )
  }, [selectedTool, selectedToolPayloadPreview])

  const moduleFilterOptions = useMemo(() => {
    const modules = new Map<string, { key: string; label: string; count: number }>()
    modules.set('space_engine', { key: 'space_engine', label: pageCopy.spaceEngine, count: 0 })

    tools.forEach((tool) => {
      const key = getToolModuleKey(tool)
      const existing = modules.get(key)
      if (existing) {
        existing.count += 1
        existing.label = getLocalizedToolModuleName(tool)
        return
      }
      modules.set(key, {
        key,
        label: getLocalizedToolModuleName(tool),
        count: 1,
      })
    })

    const activeModules = Array.from(modules.values()).filter((module) => module.count > 0 || module.key === 'space_engine')
    return [
      { key: 'all', label: pageCopy.all, count: tools.length },
      ...activeModules,
    ]
  }, [getLocalizedToolModuleName, getToolModuleKey, pageCopy.all, pageCopy.spaceEngine, tools])

  const filteredTools = useMemo(() => {
    const query = toolSearchQuery.trim().toLowerCase()

    return tools.filter((tool) => {
      const matchesModule = toolModuleFilter === 'all' || getToolModuleKey(tool) === toolModuleFilter
      if (!matchesModule) return false
      if (!query) return true

      return [
        tool.name,
        getLocalizedToolTitle(tool),
        getLocalizedToolDescription(tool),
        getLocalizedToolModuleName(tool),
      ].some((value) => value.toLowerCase().includes(query))
    })
  }, [
    getLocalizedToolDescription,
    getLocalizedToolModuleName,
    getLocalizedToolTitle,
    getToolModuleKey,
    toolModuleFilter,
    toolSearchQuery,
    tools,
  ])

  const handleSelectTool = useCallback((toolName: string) => {
    setSelectedTool(toolName)
    setDebugError('')
    setDebugResult('')
  }, [])

  const handleAuditPageChange = useCallback((page: number, pageSize: number) => {
    void loadAuditLogs(page, pageSize)
  }, [loadAuditLogs])

  const handleAuditPageSizeChange = useCallback((pageSize: number) => {
    void loadAuditLogs(1, pageSize)
  }, [loadAuditLogs])

  const handleAuditKindChange = useCallback((value: AuditKindFilter) => {
    auditKindRef.current = value
    setAuditKind(value)
    void loadAuditLogs(1, auditPageSizeRef.current)
  }, [loadAuditLogs])

  const handleAuditStatusChange = useCallback((value: AuditStatusFilter) => {
    const next = value === 'all'
      ? []
      : auditStatusRef.current.includes(value)
        ? auditStatusRef.current.filter((item) => item !== value)
        : [...auditStatusRef.current, value]
    auditStatusRef.current = next
    setAuditStatuses(next)
    void loadAuditLogs(1, auditPageSizeRef.current)
  }, [loadAuditLogs])

  const handleAuditKeywordSubmit = useCallback(() => {
    const nextKeyword = auditKeyword.trim()
    auditKeywordRef.current = nextKeyword
    setSubmittedAuditKeyword(nextKeyword)
    void loadAuditLogs(1, auditPageSizeRef.current)
  }, [auditKeyword, loadAuditLogs])

  const handleAuditKeywordClear = useCallback(() => {
    auditKeywordRef.current = ''
    setAuditKeyword('')
    setSubmittedAuditKeyword('')
    void loadAuditLogs(1, auditPageSizeRef.current)
  }, [loadAuditLogs])

  const handleConfirmListenHost = useCallback(async () => {
    const normalizedPort = selectedListenPort.trim()
    const portNumber = Number(normalizedPort)
    if (!/^\d+$/.test(normalizedPort) || portNumber < 1 || portNumber > 65535) {
      toast.error(pageCopy.invalidListenPort)
      return
    }

    setListenApplying(true)
    try {
      if (isDesktopMode) {
        const response = await fetch(`${apiBaseUrl}/api/v1/desktop/mcp/listener`, {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
            ...getDesktopApiHeaders(),
          },
          body: JSON.stringify({ host: selectedListenHost, port: normalizedPort }),
        })
        const payload = await response.json().catch(() => null) as {
          success?: boolean
          data?: {
            mcpServerURL?: string
            mcpListenHost?: string
            mcpListenPort?: string
            mcpAccessScope?: string
          }
          error?: string
        } | null
        if (!response.ok || !payload?.success) {
          throw new Error(payload?.error || formatCopy(pageCopy.listenAddressSetFailedWithStatus, { status: response.status }))
        }
        setDesktopMeta((current) => current ? {
          ...current,
          mcpServerURL: payload.data?.mcpServerURL || current.mcpServerURL,
          mcpListenHost: payload.data?.mcpListenHost || selectedListenHost,
          mcpListenPort: payload.data?.mcpListenPort || normalizedPort,
          mcpAccessScope: payload.data?.mcpAccessScope || current.mcpAccessScope,
        } : current)
        setAppliedListenHost(payload.data?.mcpListenHost || selectedListenHost)
        setAppliedListenPort(payload.data?.mcpListenPort || normalizedPort)
        setAppliedMcpBaseUrl(payload.data?.mcpServerURL || buildMcpBaseUrlForHost(selectedListenHost, normalizedPort))
      } else {
        const response = await fetchWithAuth('/api/v1/mcp/listener', {
          method: 'PUT',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ host: selectedListenHost, port: normalizedPort }),
        })
        const payload = await response.json().catch(() => null) as {
          success?: boolean
          data?: {
            mcpServerURL?: string
            mcpListenHost?: string
            mcpListenPort?: string
            mcpAccessScope?: string
          }
          error?: string
        } | null
        if (!response.ok || !payload?.success) {
          throw new Error(payload?.error || formatCopy(pageCopy.listenAddressSetFailedWithStatus, { status: response.status }))
        }
        setAppliedListenHost(payload.data?.mcpListenHost || selectedListenHost)
        setAppliedListenPort(payload.data?.mcpListenPort || normalizedPort)
        setAppliedMcpBaseUrl(payload.data?.mcpServerURL || buildMcpBaseUrlForHost(selectedListenHost, normalizedPort))
      }
      toast.success(pageCopy.listenChanged)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : pageCopy.listenChangeFailed)
    } finally {
      setListenApplying(false)
    }
  }, [
    apiBaseUrl,
    buildMcpBaseUrlForHost,
    formatCopy,
    isDesktopMode,
    pageCopy.invalidListenPort,
    pageCopy.listenAddressSetFailedWithStatus,
    pageCopy.listenChangeFailed,
    pageCopy.listenChanged,
    selectedListenHost,
    selectedListenPort,
  ])

  const clientConfigTabs = useMemo<Record<ClientConfigKey, string>>(() => {
    return {
      claude: JSON.stringify(
        {
          mcpServers: {
            hephaestus: {
              type: 'http',
              url: absoluteEndpoint,
              headers: {
                Authorization: MCP_BASIC_AUTH_PLACEHOLDER,
                'MCP-Protocol-Version': protocolVersion,
              },
              timeout: 60000,
            },
          },
        },
        null,
        2,
      ),
      cursor: JSON.stringify(
        {
          mcpServers: {
            hephaestus: {
              url: absoluteEndpoint,
              headers: {
                Authorization: MCP_BASIC_AUTH_PLACEHOLDER,
                'MCP-Protocol-Version': protocolVersion,
              },
            },
          },
        },
        null,
        2,
      ),
      codex: [
        '[mcp_servers.hephaestus]',
        'transport = "http"',
        `url = "${absoluteEndpoint}"`,
        '',
        '[mcp_servers.hephaestus.headers]',
        `Authorization = "${MCP_BASIC_AUTH_PLACEHOLDER}"`,
        `"MCP-Protocol-Version" = "${protocolVersion}"`,
      ].join('\n'),
      raw: JSON.stringify(
        {
          endpoint: absoluteEndpoint,
          initialize: {
            jsonrpc: '2.0',
            id: 1,
            method: 'initialize',
            params: {
              protocolVersion,
              clientInfo: {
                name: 'external-ai-client',
                version: '1.0.0',
              },
            },
          },
          next: ['notifications/initialized', 'tools/list', 'tools/call'],
        },
        null,
        2,
      ),
    }
  }, [absoluteEndpoint, protocolVersion])

  const handleDebugSend = useCallback(async () => {
    if (!sessionIdRef.current) {
      setDebugError(pageCopy.noSessionDebug)
      return
    }

    setDebugLoading(true)
    setDebugError('')

    try {
      const result = await callMcp('tools/call', {
        name: selectedTool,
        arguments: parseJsonSafe(selectedToolPayloadPreview),
      })
      setDebugResult(JSON.stringify(result || {}, null, 2))
      void loadAuditLogs(1, auditPageSizeRef.current)
    } catch (error) {
      setDebugError(error instanceof Error ? error.message : pageCopy.debugRequestFailed)
      void loadAuditLogs(1, auditPageSizeRef.current)
    } finally {
      setDebugLoading(false)
    }
  }, [callMcp, loadAuditLogs, pageCopy.debugRequestFailed, pageCopy.noSessionDebug, selectedTool, selectedToolPayloadPreview])

  const auditKindOptions = useMemo<Array<{ key: AuditKindFilter; label: string; description: string }>>(() => [
    { key: 'important', label: pageCopy.auditKindImportant, description: pageCopy.auditKindImportantDesc },
    { key: 'all', label: pageCopy.auditKindAll, description: pageCopy.auditKindAllDesc },
    { key: 'tool_call', label: pageCopy.auditKindToolCall, description: pageCopy.auditKindToolCallDesc },
    { key: 'protocol', label: pageCopy.auditKindProtocol, description: pageCopy.auditKindProtocolDesc },
  ], [
    pageCopy.auditKindAll,
    pageCopy.auditKindAllDesc,
    pageCopy.auditKindImportant,
    pageCopy.auditKindImportantDesc,
    pageCopy.auditKindProtocol,
    pageCopy.auditKindProtocolDesc,
    pageCopy.auditKindToolCall,
    pageCopy.auditKindToolCallDesc,
  ])
  const auditStatusOptions = useMemo<Array<{ key: AuditStatusFilter; label: string }>>(() => [
    { key: 'all', label: pageCopy.auditStatusAll },
    { key: 'success', label: pageCopy.success },
    { key: 'failed', label: pageCopy.failed },
  ], [pageCopy.auditStatusAll, pageCopy.failed, pageCopy.success])
  const activeAuditKindLabel = auditKindOptions.find((option) => option.key === auditKind)?.label || pageCopy.auditLog
  const activeAuditStatusLabel = auditStatuses.length === 0
    ? pageCopy.auditStatusAll
    : auditStatusOptions
        .filter((option) => option.key !== 'all' && auditStatuses.includes(option.key as AuditStatus))
        .map((option) => option.label)
        .join(' / ')
  const currentConfigCode = clientConfigTabs[clientConfigKey]
  const defaultDebugResponse = connected
    ? pageCopy.waitingToolCall
    : JSON.stringify({ error: pageError || 'waiting for MCP handshake' }, null, 2)
  const desktopListenMessage = appliedListenHost === '127.0.0.1'
    ? formatCopy(pageCopy.desktopLocalMode, { port: appliedListenPort })
    : appliedListenHost === '0.0.0.0'
      ? formatCopy(pageCopy.desktopAllNetworkMode, { port: appliedListenPort })
      : formatCopy(pageCopy.desktopLanMode, { host: appliedListenHost, port: appliedListenPort })

  if (loading) {
    return (
      <div
        className={cn('flex h-full min-h-full items-center justify-center text-foreground', PAGE_BACKGROUND_CLASS, 'scan-surface-pending')}
        aria-busy="true"
      >
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className={cn('h-full min-h-full overflow-x-auto text-foreground', PAGE_BACKGROUND_CLASS, 'scan-panel-enter')}>
      <div className="mx-auto flex min-w-[960px] max-w-[1760px] flex-col gap-4 p-5 lg:p-6">
        <header className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap items-center gap-3">
            <StatusBadge tone={connected ? 'success' : 'danger'}>
              {connected ? pageCopy.serverOnline : pageCopy.serverOffline}
            </StatusBadge>
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs text-muted-foreground">{pageCopy.accessScope}</span>
              <Select value={selectedListenHost} onValueChange={setSelectedListenHost}>
                <SelectTrigger className="h-10 w-[240px] rounded-xl border-border/70 bg-muted/30 font-mono text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {listenOptions.map((item) => (
                    <SelectItem key={item.key} value={item.key}>
                      <div className="flex items-center justify-between gap-4">
                        <span className="font-mono">{item.label}</span>
                        <span className="text-xs text-muted-foreground">{item.description}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <span className="text-xs text-muted-foreground">{pageCopy.port}</span>
              <Input
                value={selectedListenPort}
                onChange={(event) => setSelectedListenPort(event.target.value.replace(/[^\d]/g, '').slice(0, 5))}
                inputMode="numeric"
                pattern="[0-9]*"
                className="h-10 w-[96px] rounded-xl border-border/70 bg-muted/30 font-mono text-xs"
                placeholder="8080"
              />
              <Button
                variant="outline"
                onClick={() => void loadNetworkCards()}
                disabled={networkLoading}
                className="h-10 rounded-xl border-border/70 bg-muted/30 px-3 text-foreground hover:bg-muted/30 hover:text-foreground"
                title={pageCopy.refreshNetworkCards}
              >
                <RefreshCw className={cn('h-4 w-4', networkLoading && 'animate-spin')} />
              </Button>
              <Button
                onClick={() => void handleConfirmListenHost()}
                disabled={listenApplying}
                className="h-10 rounded-xl"
              >
                {listenApplying ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
                {pageCopy.changeListen}
              </Button>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="outline"
              onClick={() => void bootstrap('refresh')}
              disabled={refreshing}
              className="h-10 rounded-xl border-border/70 bg-muted/30 text-foreground hover:bg-muted/30 hover:text-foreground"
            >
              {refreshing ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
              {lastSyncedAt || pageCopy.refresh}
            </Button>
          </div>
        </header>

        <section className="grid gap-3 [grid-template-columns:repeat(auto-fit,minmax(220px,1fr))]">
          <AccessCard
            icon={<Globe2 className="h-4 w-4" />}
            label="Base URL"
            value={effectiveBaseUrl}
            status={connected ? accessScopeLabel : baseUrlStatus}
            tone={baseUrlTone}
            onCopy={() => void handleCopy(effectiveBaseUrl)}
            copyAriaLabel={pageCopy.copyField}
          />
          <AccessCard
            icon={<Server className="h-4 w-4" />}
            label="Endpoint"
            value={MCP_ENDPOINT_PATH}
            status={tools.length ? pageCopy.sessionAvailable : pageCopy.pendingSync}
            tone={tools.length ? 'success' : 'warning'}
            onCopy={() => void handleCopy(absoluteEndpoint)}
            copyAriaLabel={pageCopy.copyField}
          />
          <AccessCard
            icon={<KeyRound className="h-4 w-4" />}
            label="Authorization"
            value="Basic Auth"
            status={pageCopy.systemAccountPassword}
            tone="success"
          />
          <AccessCard
            icon={<ShieldCheck className="h-4 w-4" />}
            label="Protocol Version"
            value={`MCP ${protocolVersion}`}
            status={connected ? pageCopy.handshakeCompatible : pageCopy.pendingHandshake}
            tone={connected ? 'success' : 'warning'}
          />
        </section>

        {isDesktopMode ? (
          <div
            className={cn(
              'flex flex-wrap items-center gap-3 rounded-xl border px-4 py-3 text-sm',
              desktopMeta?.isTemporaryBackendPort
                ? 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-100'
                : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-100',
            )}
          >
            {desktopMeta?.isTemporaryBackendPort ? (
              <CircleAlert className="h-4 w-4" />
            ) : (
              <CheckCircle2 className="h-4 w-4" />
            )}
            <span>
              {desktopMeta?.isTemporaryBackendPort
                ? formatCopy(pageCopy.temporaryBackendPortMessage, {
                  preferredPort: desktopMeta.preferredBackendPort || '38456',
                  currentPort: desktopMeta.backendPort || pageCopy.unknown,
                })
                : desktopListenMessage}
            </span>
          </div>
        ) : null}

        {pageError ? (
          <div className="flex items-start gap-3 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-700 dark:text-red-100">
            <XCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <div className="flex min-w-0 flex-col gap-1">
              <span className="font-medium">{pageCopy.handshakeDiagnosisFailed}</span>
              <span className="break-words text-red-700/80 dark:text-red-100/80">{pageError}</span>
            </div>
          </div>
        ) : (
          <div className="flex flex-wrap items-center gap-3 rounded-xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-100">
            <CheckCircle2 className="h-4 w-4" />
            <span>{formatCopy(pageCopy.handshakeCompleteMessage, { scope: accessScopeLabel, count: tools.length })}</span>
          </div>
        )}

        <section className="min-h-[680px]">
          <ResizablePanelGroup direction="horizontal" className="min-h-[680px]">
            <ResizablePanel defaultSize={30} minSize={20} className="min-w-0">
              <SurfacePanel
                title={pageCopy.toolList}
                className="h-full rounded-none"
                bodyClassName="flex min-h-0 flex-1 flex-col"
                action={<StatusBadge tone="neutral">{formatCopy(pageCopy.toolsUnit, { count: tools.length })}</StatusBadge>}
              >
            <div className="flex min-h-0 flex-1 flex-col gap-4">
              <label className="relative block">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <input
                  value={toolSearchQuery}
                  onChange={(event) => setToolSearchQuery(event.target.value)}
                  placeholder={pageCopy.searchToolPlaceholder}
                  className="h-10 w-full rounded-xl border border-border/70 bg-background pl-9 pr-3 text-sm text-foreground outline-none transition placeholder:text-muted-foreground focus:border-primary/60"
                />
              </label>

              <div className="flex flex-wrap gap-2">
                {moduleFilterOptions.map((module) => (
                  <button
                    key={module.key}
                    type="button"
                    onClick={() => setToolModuleFilter(module.key)}
                    className={cn(
                      'rounded-full border px-3 py-1.5 text-xs transition',
                      toolModuleFilter === module.key
                        ? 'border-primary/50 bg-primary/10 text-primary'
                        : 'border-border/70 bg-muted/30 text-muted-foreground hover:bg-muted/30 hover:text-foreground',
                    )}
                  >
                    {module.label}
                    <span className="ml-1 font-mono text-muted-foreground">{module.count}</span>
                  </button>
                ))}
              </div>

              <div className="flex max-h-[420px] min-h-0 flex-1 flex-col gap-2 overflow-y-auto pr-1">
                {filteredTools.length ? filteredTools.map((tool) => {
                  const isSelected = selectedTool === tool.name

                  return (
                    <button
                      key={tool.name}
                      type="button"
                      onClick={() => handleSelectTool(tool.name)}
                      className={cn(
                        'min-w-0 rounded-xl border px-3 py-2.5 text-left transition',
                        isSelected
                          ? 'border-primary/45 bg-primary/10 shadow-sm'
                          : 'border-border/60 bg-background/30 hover:bg-muted/40',
                      )}
                    >
                      <div className="flex min-w-0 items-center justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium text-foreground">{getLocalizedToolTitle(tool)}</div>
                          <div className="mt-1 truncate font-mono text-xs text-muted-foreground">{tool.name}</div>
                        </div>
                        <span className={cn('h-2 w-2 shrink-0 rounded-full', isSelected ? 'bg-primary' : 'bg-muted-foreground/35')} />
                      </div>
                    </button>
                  )
                }) : (
                  <div className="rounded-xl border border-border/60 bg-muted/20 px-3 py-8 text-center text-sm text-muted-foreground">
                    {tools.length ? pageCopy.noMatchingTools : pageCopy.waitingToolsList}
                  </div>
                )}
              </div>
            </div>
          </SurfacePanel>
            </ResizablePanel>

            <ResizableHandle withHandle />

            <ResizablePanel defaultSize={40} minSize={30} className="min-w-0">
            <SurfacePanel
              title={pageCopy.liveDebugger}
              className="h-full rounded-none"
              bodyClassName="flex min-h-0 flex-1 flex-col overflow-y-auto"
              action={sessionId ? (
                <button
                  type="button"
                  onClick={() => void handleCopy(sessionId)}
                  className="flex max-w-[180px] items-center gap-2 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-3 py-1.5 text-xs text-emerald-700 transition hover:bg-emerald-500/15 dark:text-emerald-100"
                >
                  <span className="truncate font-mono">{maskSessionId(sessionId)}</span>
                  <Copy className="h-3.5 w-3.5" />
                </button>
              ) : (
                <StatusBadge tone="warning">{pageCopy.notConnected}</StatusBadge>
              )}
            >
              <div className="flex flex-col gap-4">
                <div className="flex items-center justify-between gap-3">
                  <div className="text-xs text-muted-foreground">{pageCopy.request}</div>
                  <CopyIconButton label={pageCopy.copyRequest} onClick={() => void handleCopy(debugRequestPreview)} />
                </div>
                <CodeBlock code={debugRequestPreview} height="220px" />

                <div className="flex flex-wrap items-center justify-between gap-3">
                  <Button
                    onClick={() => void handleDebugSend()}
                    disabled={debugLoading || !connected}
                    className="h-10 rounded-xl bg-primary text-primary-foreground hover:bg-primary/90"
                  >
                    {debugLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Send className="mr-2 h-4 w-4" />}
                    {pageCopy.send}
                  </Button>
                  <span className={cn('text-xs', debugError ? 'text-red-700 dark:text-red-200' : 'text-emerald-700 dark:text-emerald-200')}>
                    {debugError || (connected ? pageCopy.readyStatus : pageCopy.waitingForSession)}
                  </span>
                </div>

                <div className="flex items-center justify-between gap-3">
                  <div className="text-xs text-muted-foreground">{pageCopy.response}</div>
                  <CopyIconButton label={pageCopy.copyResponse} onClick={() => void handleCopy(debugResult || defaultDebugResponse)} />
                </div>
                <CodeBlock code={debugError ? JSON.stringify({ error: debugError }, null, 2) : (debugResult || defaultDebugResponse)} height="180px" />
              </div>
            </SurfacePanel>
            </ResizablePanel>

            <ResizableHandle withHandle />

            <ResizablePanel defaultSize={30} minSize={20} className="min-w-0">
          <SurfacePanel
            title={pageCopy.clientConfig}
            className="h-full rounded-none"
            bodyClassName="flex min-h-0 flex-1 flex-col"
            action={(
              <CopyIconButton
                label={pageCopy.copyConfig}
                onClick={() => void handleCopy(currentConfigCode)}
              />
            )}
          >
            <div className="flex min-h-0 flex-1 flex-col gap-4">
              <SegmentedControl
                value={clientConfigKey}
                options={(Object.keys(CLIENT_CONFIG_LABELS) as ClientConfigKey[]).map((key) => ({
                  key,
                  label: CLIENT_CONFIG_LABELS[key],
                }))}
                onChange={(value) => setClientConfigKey(value as ClientConfigKey)}
              />

              <CodeBlock code={currentConfigCode} minHeight="300px" className="min-h-0 flex-1" />
            </div>
          </SurfacePanel>
            </ResizablePanel>
          </ResizablePanelGroup>
        </section>

        <SurfacePanel
          title={pageCopy.auditLog}
          action={(
            <div className="flex flex-wrap items-center gap-2">
              <StatusBadge tone={auditError ? 'danger' : 'neutral'}>{formatCopy(pageCopy.auditCountUnit, { count: auditTotal })}</StatusBadge>
              <StatusBadge tone="neutral">{activeAuditKindLabel}</StatusBadge>
              {auditStatuses.length > 0 ? <StatusBadge tone="neutral">{activeAuditStatusLabel}</StatusBadge> : null}
              <button
                type="button"
                onClick={() => void loadAuditLogs()}
                disabled={auditLoading}
                className="rounded-lg border border-border/70 bg-muted/30 p-2 text-muted-foreground transition hover:bg-muted/30 hover:text-foreground"
              >
                <RefreshCw className={cn('h-4 w-4', auditLoading && 'animate-spin')} />
              </button>
            </div>
          )}
        >
          <div className="mb-4 flex flex-col gap-3 border-b border-border/70 pb-4 xl:flex-row xl:items-end xl:justify-between">
            <div className="flex flex-col gap-2">
              <div className="text-xs text-muted-foreground">{pageCopy.recordScope}</div>
              <div className="flex flex-wrap gap-2">
                {auditKindOptions.map((option) => (
                  <button
                    key={option.key}
                    type="button"
                    title={option.description}
                    onClick={() => handleAuditKindChange(option.key)}
                    className={cn(
                      'rounded-full border px-3 py-1.5 text-xs transition',
                      auditKind === option.key
                        ? 'border-primary/50 bg-primary/10 text-primary'
                        : 'border-border/70 bg-muted/30 text-muted-foreground hover:bg-muted/30 hover:text-foreground',
                    )}
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </div>

            <div className="flex flex-wrap items-end gap-2">
              <div className="flex flex-col gap-2">
                <div className="text-xs text-muted-foreground">{pageCopy.status}</div>
                <div className="flex flex-wrap gap-2">
                  {auditStatusOptions.map((option) => {
                    const isActive = option.key === 'all'
                      ? auditStatuses.length === 0
                      : auditStatuses.includes(option.key as AuditStatus)
                    return (
                      <button
                        key={option.key}
                        type="button"
                        aria-pressed={isActive}
                        onClick={() => handleAuditStatusChange(option.key)}
                        className={cn(
                          'rounded-full border px-3 py-1.5 text-xs transition',
                          isActive
                            ? 'border-emerald-500/35 bg-emerald-500/10 text-emerald-700 dark:text-emerald-100'
                            : 'border-border/70 bg-muted/30 text-muted-foreground hover:bg-muted/30 hover:text-foreground',
                        )}
                      >
                        {option.label}
                      </button>
                    )
                  })}
                </div>
              </div>

              <label className="relative block">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <input
                  value={auditKeyword}
                  onChange={(event) => setAuditKeyword(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      handleAuditKeywordSubmit()
                    }
                  }}
                  placeholder={pageCopy.auditSearchPlaceholder}
                  className="h-9 w-[240px] rounded-xl border border-border/70 bg-background pl-9 pr-3 text-sm text-foreground outline-none transition placeholder:text-muted-foreground focus:border-primary/60"
                />
              </label>
              <Button
                variant="outline"
                onClick={handleAuditKeywordSubmit}
                className="h-9 rounded-xl border-border/70 bg-muted/30 px-3 text-foreground hover:bg-muted/30 hover:text-foreground"
              >
                {pageCopy.search}
              </Button>
              {auditKeyword.trim() || submittedAuditKeyword ? (
                <Button
                  variant="outline"
                  onClick={handleAuditKeywordClear}
                  className="h-9 rounded-xl border-border/70 bg-muted/30 px-3 text-foreground hover:bg-muted/30 hover:text-foreground"
                >
                  {pageCopy.clear}
                </Button>
              ) : null}
            </div>
          </div>
          {auditError ? (
            <div className="mb-3 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-700 dark:text-red-100">
              {auditError}
            </div>
          ) : null}
          <div className="overflow-x-auto">
            <table className="w-full min-w-[980px] border-collapse text-left text-sm">
              <thead>
                <tr className="border-b border-border/70 text-xs text-muted-foreground">
                  <th className="px-3 py-3 font-medium">{pageCopy.time}</th>
                  <th className="px-3 py-3 font-medium">{pageCopy.client}</th>
                  <th className="px-3 py-3 font-medium">{pageCopy.methodTool}</th>
                  <th className="px-3 py-3 font-medium">{pageCopy.type}</th>
                  <th className="px-3 py-3 font-medium">{pageCopy.status}</th>
                  <th className="px-3 py-3 font-medium">{pageCopy.duration}</th>
                  <th className="px-3 py-3 font-medium">{pageCopy.sourceIp}</th>
                  <th className="px-3 py-3 font-medium">User Agent</th>
                </tr>
              </thead>
              <tbody>
                {auditLogs.length ? auditLogs.map((log) => {
                  const isSuccess = log.status === 'success'
                  const isToolCall = log.method === 'tools/call'
                  const kind = log.method === 'tools/call'
                    ? pageCopy.toolCall
                    : (log.status === 'failed' ? pageCopy.exception : pageCopy.protocolEvent)

                  return (
                    <tr
                      key={log.id}
                      className={cn(
                        'border-b border-border/70 text-muted-foreground',
                        isToolCall && isSuccess && 'bg-primary/5 text-foreground',
                        !isSuccess && 'bg-red-500/10 text-red-700 dark:text-red-100',
                      )}
                    >
                      <td className="px-3 py-3 font-mono text-xs">{formatAuditTime(log.createdAt)}</td>
                      <td className="px-3 py-3">{getAuditClientLabel(log)}</td>
                      <td className="px-3 py-3 font-mono text-xs text-muted-foreground">{getAuditDisplayTarget(log)}</td>
                      <td className="px-3 py-3">
                        <span className={cn(
                          'rounded-full border px-2 py-1 text-xs',
                          kind === pageCopy.toolCall && 'border-primary/30 bg-primary/10 text-primary',
                          kind === pageCopy.exception && 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-100',
                          kind === pageCopy.protocolEvent && 'border-border/70 bg-muted/30 text-muted-foreground',
                        )}>
                          {kind}
                        </span>
                      </td>
                      <td className="px-3 py-3">
                        <span className={cn('inline-flex items-center gap-1.5', isSuccess ? 'text-emerald-700 dark:text-emerald-200' : 'text-red-700 dark:text-red-200')}>
                          <span className={cn('h-2 w-2 rounded-full', isSuccess ? 'bg-emerald-500 dark:bg-emerald-300' : 'bg-red-500 dark:bg-red-300')} />
                          {isSuccess ? pageCopy.success : pageCopy.failed}
                        </span>
                      </td>
                      <td className="px-3 py-3 font-mono text-xs">{typeof log.durationMs === 'number' ? `${log.durationMs} ms` : '-'}</td>
                      <td className="px-3 py-3 font-mono text-xs">{log.sourceIp || '-'}</td>
                      <td className="px-3 py-3 font-mono text-xs">
                        <div className="max-w-[280px] truncate" title={log.userAgent || '-'}>
                          {log.userAgent || '-'}
                        </div>
                      </td>
                    </tr>
                  )
                }) : (
                  <tr>
                    <td colSpan={8} className="px-3 py-10 text-center text-sm text-muted-foreground">
                      {auditLoading ? pageCopy.auditLoading : pageCopy.auditEmpty}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
          <div className="mt-4 border-t border-border/70 pt-4">
            <PaginationPro
              current={auditPage}
              total={auditTotal}
              pageSize={auditPageSize}
              onChange={handleAuditPageChange}
              onPageSizeChange={handleAuditPageSizeChange}
              pageSizeOptions={[10, 20, 50]}
              showSizeChanger
              showTotal
              disabled={auditLoading}
              className="text-muted-foreground"
            />
          </div>
        </SurfacePanel>
      </div>
    </div>
  )
}
function SurfacePanel({
  title,
  action,
  className,
  bodyClassName,
  children,
}: {
  title: string
  action?: ReactNode
  className?: string
  bodyClassName?: string
  children: ReactNode
}) {
  return (
    <section className={cn('flex min-w-0 flex-col overflow-hidden rounded-2xl', PANEL_CLASS, className)}>
      <div className="flex min-h-[58px] items-center justify-between gap-3 border-b border-border/70 px-4 py-3">
        <div className="min-w-0 text-base font-medium text-foreground">{title}</div>
        {action}
      </div>
      <div className={cn('p-4', bodyClassName)}>{children}</div>
    </section>
  )
}

function AccessCard({
  icon,
  label,
  value,
  status,
  tone,
  onCopy,
  copyAriaLabel,
}: {
  icon: ReactNode
  label: string
  value: string
  status: string
  tone: 'success' | 'warning'
  onCopy?: () => void
  copyAriaLabel?: string
}) {
  return (
    <div className={cn('flex min-h-[92px] min-w-0 flex-col justify-between gap-3 rounded-2xl px-4 py-4', PANEL_CLASS)}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground">
          {icon}
          <span className="truncate">{label}</span>
        </div>
        {onCopy ? (
          <button
            type="button"
            onClick={onCopy}
            className="rounded-lg border border-border/70 bg-muted/30 p-2 text-muted-foreground transition hover:bg-muted/30 hover:text-foreground"
            aria-label={copyAriaLabel ? copyAriaLabel.replace('{label}', label) : label}
          >
            <Copy className="h-4 w-4" />
          </button>
        ) : null}
      </div>
      <div className="min-w-0">
        <div className="break-all font-mono text-sm text-foreground">{value}</div>
        <div className={cn('mt-2 flex items-center gap-2 text-xs', tone === 'success' ? 'text-emerald-700 dark:text-emerald-200' : 'text-amber-700 dark:text-amber-200')}>
          <span className={cn('h-2 w-2 rounded-full', tone === 'success' ? 'bg-emerald-500 dark:bg-emerald-300' : 'bg-amber-500 dark:bg-amber-300')} />
          {status}
        </div>
      </div>
    </div>
  )
}

function SegmentedControl({
  value,
  options,
  onChange,
}: {
  value: string
  options: Array<{ key: string; label: string }>
  onChange: (value: string) => void
}) {
  return (
    <div className="inline-flex w-fit overflow-hidden rounded-xl border border-border/70 bg-muted/30 p-1">
      {options.map((option) => (
        <button
          key={option.key}
          type="button"
          onClick={() => onChange(option.key)}
          className={cn(
            'min-h-8 rounded-lg px-3 text-xs font-medium transition',
            value === option.key
              ? 'bg-primary text-primary-foreground shadow-sm'
              : 'text-muted-foreground hover:bg-muted/30 hover:text-foreground',
          )}
        >
          {option.label}
        </button>
      ))}
    </div>
  )
}

function StatusBadge({
  tone,
  children,
}: {
  tone: 'success' | 'danger' | 'warning' | 'neutral'
  children: ReactNode
}) {
  return (
    <span className={cn(
      'inline-flex w-fit items-center gap-2 rounded-full border px-3 py-1 text-xs font-medium',
      tone === 'success' && 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-100',
      tone === 'danger' && 'border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-100',
      tone === 'warning' && 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-100',
      tone === 'neutral' && 'border-border/70 bg-muted/30 text-muted-foreground',
    )}>
      <span className={cn(
        'h-2 w-2 rounded-full',
        tone === 'success' && 'bg-emerald-500 dark:bg-emerald-300',
        tone === 'danger' && 'bg-red-500 dark:bg-red-300',
        tone === 'warning' && 'bg-amber-500 dark:bg-amber-300',
        tone === 'neutral' && 'bg-muted-foreground/40',
      )} />
      {children}
    </span>
  )
}

function CopyIconButton({
  label,
  onClick,
}: {
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-2 rounded-lg border border-border/70 bg-muted/30 px-2.5 py-1.5 text-xs text-muted-foreground transition hover:bg-muted/30 hover:text-foreground"
    >
      <Copy className="h-3.5 w-3.5" />
      <span>{label}</span>
    </button>
  )
}

function CodeBlock({
  code,
  minHeight,
  height,
  className,
}: {
  code: string
  minHeight?: string
  height?: string
  className?: string
}) {
  return (
    <pre
      className={cn('overflow-auto rounded-xl p-4 font-mono text-[12px] leading-6 text-muted-foreground', CODE_SURFACE_CLASS, className)}
      style={{ minHeight, height }}
    >
      {code}
    </pre>
  )
}
