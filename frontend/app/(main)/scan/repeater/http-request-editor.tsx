"use client"

import { useState, useRef, useEffect, useCallback, type WheelEvent } from "react"
import { useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Send, Plus, X, Globe, Maximize2, Minimize2, FileText, Wifi, WifiOff, XCircle, Binary, Terminal, Sparkles, MoreVertical, Loader2 } from "lucide-react"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"
import { Switch } from "@/components/ui/switch"
import { Label } from "@/components/ui/label"
import { RequestEditor } from "./request-editor"
import { ResponseViewer } from "./response-viewer"
import { HexViewer } from "./hex-viewer"
import toast from '@/lib/toast'
import { fetchWithAuth } from '@/lib/api'
import {
  fetchSystemProxyInfo,
  getProxyUrl,
  getSystemProxyUrl,
  maskProxyUrl,
  resolveProxyUrl,
  shouldBypassProxy,
  type SystemProxyInfo,
} from '@/lib/proxy'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import { getMaskedUrl } from "./privacy-mask"
import {
  queueNucleiTemplateImport,
  createNucleiTemplateDraft,
  renderNucleiTemplateYaml,
  type NucleiTemplateWorkspacePayload,
  type ResponseSelectionSnapshot,
} from "./nuclei-template"

const TABS_DISPLAY_MODE_STORAGE_KEY = 'hephaestus:repeater:tabs-display-mode'

const DEFAULT_RAW_REQUEST = `GET /api/users HTTP/1.1
Host: api.example.com
User-Agent: BurpSuite/2.0
Accept: application/json
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9
Content-Type: application/json

`

interface RepeaterTab {
  id: string
  name: string
  method: string
  url: string
  rawRequest: string
  response: string
  responseLength: number
  responseTime: number
  hasRedirect: boolean
  isLoading: boolean
  forceHttps: boolean
  viewMode: 'raw' | 'hex'  // 新增：视图模式
}

type TabsDisplayMode = 'wrap' | 'scroll'

type PendingRepeaterRequestPayload = {
  raw: string
  method?: string
  url?: string
  forceHttps?: boolean
  name?: string
}

const shellEscape = (value: string) => `'${value.replace(/'/g, `'\"'\"'`)}'`
type ParsedRepeaterRequest = {
  method: string
  requestTarget: string
  body: string
  headers: string[]
  headerMap: Record<string, string>
}

const parseRepeaterRawRequest = (rawRequest: string): ParsedRepeaterRequest | null => {
  const normalized = rawRequest.replace(/\r\n/g, '\n')
  const sections = normalized.split('\n\n')
  const headerLines = sections[0]?.split('\n') || []
  if (headerLines.length === 0) return null

  const requestLine = headerLines[0].trim()
  const requestParts = requestLine.match(/^([A-Z]+)\s+(.+?)\s+HTTP\/[\d.]+$/i)
  if (!requestParts) return null

  const headers = headerLines.slice(1).map((line) => line.trim()).filter(Boolean)
  const headerMap = headers.reduce<Record<string, string>>((acc, line) => {
    const separatorIndex = line.indexOf(':')
    if (separatorIndex <= 0) return acc
    const key = line.slice(0, separatorIndex).trim()
    const value = line.slice(separatorIndex + 1).trim()
    acc[key] = value
    return acc
  }, {})

  return {
    method: requestParts[1].toUpperCase(),
    requestTarget: requestParts[2].trim(),
    body: sections.slice(1).join('\n\n'),
    headers,
    headerMap,
  }
}

const parsePendingRepeaterRequest = (value: string): PendingRepeaterRequestPayload | null => {
  try {
    const parsed = JSON.parse(value)
    if (typeof parsed === 'string') {
      return { raw: parsed }
    }
    if (parsed && typeof parsed === 'object' && typeof parsed.raw === 'string') {
      return {
        raw: parsed.raw,
        method: typeof parsed.method === 'string' ? parsed.method : undefined,
        url: typeof parsed.url === 'string' ? parsed.url : undefined,
        forceHttps: typeof parsed.forceHttps === 'boolean' ? parsed.forceHttps : undefined,
        name: typeof parsed.name === 'string' ? parsed.name : undefined,
      }
    }
  } catch {
    return { raw: value }
  }
  return { raw: value }
}

const buildDownloadBaseName = (url: string) => {
  try {
    const parsed = new URL(url)
    const segments = parsed.pathname.split('/').filter(Boolean)
    const lastSegment = segments[segments.length - 1] || 'response'
    const decoded = decodeURIComponent(lastSegment)
    const sanitized = decoded.replace(/[\\/:*?"<>|]+/g, '_').replace(/^\.+/, '').trim()
    return sanitized || 'response'
  } catch {
    return 'response'
  }
}

const buildRootUrl = (url: string | null | undefined) => {
  const value = url?.trim()
  if (!value) return ''
  try {
    const parsed = new URL(value)
    return `${parsed.protocol}//${parsed.host}/`
  } catch {
    return value
  }
}

export function HttpRequestEditor() {
  const { t } = useI18n()
  const router = useRouter()
  const [tabs, setTabs] = useState<RepeaterTab[]>([])
  const [activeTab, setActiveTab] = useState<string | null>(null)
  const [editingTabId, setEditingTabId] = useState<string | null>(null)
  const [editingName, setEditingName] = useState('')
  const editInputRef = useRef<HTMLInputElement>(null)
  const [fullscreenPanel, setFullscreenPanel] = useState<'request' | 'response' | null>(null)
  const [isInitialized, setIsInitialized] = useState(false)
  const saveTimeoutRef = useRef<NodeJS.Timeout | null>(null)
  const [systemProxy, setSystemProxy] = useState<SystemProxyInfo | null>(null)
  const [appProxyUrl, setAppProxyUrl] = useState('')
  const [draggedTabId, setDraggedTabId] = useState<string | null>(null)
  const [dragOverTabId, setDragOverTabId] = useState<string | null>(null)
  const [curlDialogOpen, setCurlDialogOpen] = useState(false)
  const [curlInput, setCurlInput] = useState('')
  const [privacyMode, setPrivacyMode] = useState(false)
  const [closedTabs, setClosedTabs] = useState<RepeaterTab[]>([])
  const abortControllersRef = useRef<Record<string, AbortController>>({})
  const responseSelectionRef = useRef<Record<string, ResponseSelectionSnapshot[]>>({})
  const [tabsDisplayMode, setTabsDisplayMode] = useState<TabsDisplayMode>(() => {
    if (typeof window === 'undefined') return 'wrap'
    const savedMode = window.localStorage.getItem(TABS_DISPLAY_MODE_STORAGE_KEY)
    return savedMode === 'scroll' ? 'scroll' : 'wrap'
  })
  const currentTab = tabs.find((tab) => tab.id === activeTab)
  const activeSystemProxyUrl = getSystemProxyUrl(systemProxy)
  const activeProxyUrl = appProxyUrl || activeSystemProxyUrl
  const activeProxySource: 'app' | 'system' | 'none' = appProxyUrl
    ? 'app'
    : activeSystemProxyUrl
      ? 'system'
      : 'none'

  const handleTabsDisplayModeChange = (mode: string) => {
    const nextMode: TabsDisplayMode = mode === 'scroll' ? 'scroll' : 'wrap'
    setTabsDisplayMode(nextMode)
    window.localStorage.setItem(TABS_DISPLAY_MODE_STORAGE_KEY, nextMode)
  }

  const handleTabsWheel = (event: WheelEvent<HTMLDivElement>) => {
    if (tabsDisplayMode !== 'scroll') return

    const container = event.currentTarget
    const maxScrollLeft = container.scrollWidth - container.clientWidth

    if (maxScrollLeft <= 0 || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) {
      return
    }

    event.preventDefault()
    event.stopPropagation()
    container.scrollLeft = Math.max(
      0,
      Math.min(maxScrollLeft, container.scrollLeft + event.deltaY)
    )
  }

  const buildCurlCommand = (tab: RepeaterTab): string | null => {
    const parsedRequest = parseRepeaterRawRequest(tab.rawRequest)
    if (!parsedRequest) return null

    const { method, requestTarget, body } = parsedRequest
    const headers = parsedRequest.headers
      .filter((line) => !/^content-length\s*:/i.test(line))

    let finalUrl = tab.url?.trim() || ''
    if (/^https?:\/\//i.test(requestTarget)) {
      finalUrl = requestTarget
    } else if (finalUrl) {
      try {
        const parsed = new URL(finalUrl)
        finalUrl = new URL(requestTarget, `${parsed.protocol}//${parsed.host}`).toString()
      } catch {
        finalUrl = ''
      }
    }

    if (!finalUrl) {
      const hostLine = headers.find((line) => /^host\s*:/i.test(line))
      const host = hostLine?.split(/:\s*/, 2)?.[1]?.trim()
      if (!host) return null
      const scheme = tab.forceHttps ? 'https' : 'http'
      finalUrl = `${scheme}://${host}${requestTarget}`
    }

    const parts = ['curl', '-X', method, shellEscape(finalUrl)]
    headers.forEach((header) => {
      parts.push('-H', shellEscape(header))
    })
    if (body) {
      parts.push('--data-raw', shellEscape(body))
    }
    return parts.join(' ')
  }

  const buildRequestUrl = (tab: RepeaterTab): string | null => {
    const parsedRequest = parseRepeaterRawRequest(tab.rawRequest)
    if (!parsedRequest) return tab.url?.trim() || null

    const requestTarget = parsedRequest.requestTarget
    if (/^https?:\/\//i.test(requestTarget)) {
      return requestTarget
    }

    const finalUrl = tab.url?.trim()
    if (finalUrl) {
      try {
        const parsed = new URL(finalUrl)
        return new URL(requestTarget, `${parsed.protocol}//${parsed.host}`).toString()
      } catch {
        // ignore and fallback to Host header
      }
    }

    const host = parsedRequest.headerMap.Host || parsedRequest.headerMap.host
    if (!host) return finalUrl || null

    const scheme = tab.forceHttps ? 'https' : 'http'
    return `${scheme}://${host}${requestTarget}`
  }

  const buildDownloadCurlCommand = (tab: RepeaterTab): string | null => {
    const curlCommand = buildCurlCommand(tab)
    const requestUrl = buildRequestUrl(tab)
    if (!curlCommand || !requestUrl) return null

    const fileName = buildDownloadBaseName(requestUrl)
    return `${curlCommand} --include --remote-time --output ${shellEscape(fileName)}`
  }

  const handleCopyAsCurl = async (tab: RepeaterTab) => {
    const curlCommand = buildCurlCommand(tab)
    if (!curlCommand) {
      toast.error({ title: t.common.error, message: t.repeater.curlExportError })
      return
    }
    const success = await copyToClipboard(curlCommand)
    if (!success) {
      toast.error({ title: t.common.error, message: t.common.copyFailed })
      return
    }
    toast.success({ title: t.common.success, message: t.repeater.curlCopySuccess })
  }

  const handleCopyDownloadCurl = async (tab: RepeaterTab) => {
    const command = buildDownloadCurlCommand(tab)
    if (!command) {
      toast.error({ title: t.common.error, message: t.repeater.downloadCommandExportError })
      return
    }
    const success = await copyToClipboard(command)
    if (!success) {
      toast.error({ title: t.common.error, message: t.common.copyFailed })
      return
    }
    toast.success({ title: t.common.success, message: t.repeater.downloadCurlCopySuccess })
  }

  const handleCopyUrl = async (tab: RepeaterTab) => {
    const requestUrl = buildRequestUrl(tab)
    if (!requestUrl) {
      toast.error({ title: t.common.error, message: t.repeater.invalidUrl })
      return
    }

    const success = await copyToClipboard(requestUrl)
    if (!success) {
      toast.error({ title: t.common.error, message: t.common.error })
      return
    }

    toast.success({ title: t.common.success, message: 'URL 已复制' })
  }

  const openNucleiTemplatePage = useCallback((tab: RepeaterTab, selection?: ResponseSelectionSnapshot | null) => {
    if (!tab.rawRequest.trim() || !tab.response.trim()) {
      toast.warning({ title: t.common.warning, message: t.repeater.nucleiTemplate.missingResponse })
      return
    }

    const nextDraft = createNucleiTemplateDraft({
      rawRequest: tab.rawRequest,
      response: tab.response,
      selections: selection ? [selection] : responseSelectionRef.current[tab.id] || [],
    })
    const target = buildRootUrl(buildRequestUrl(tab) || tab.url)
    const payload: NucleiTemplateWorkspacePayload = {
      draft: nextDraft,
      yaml: renderNucleiTemplateYaml(nextDraft),
      response: tab.response,
      target,
      source: 'repeater',
      sourceTabName: tab.name,
      returnPath: '/scan/repeater',
      initialView: 'template',
      createdAt: Date.now(),
    }
    queueNucleiTemplateImport(payload)
    router.push('/poc/nuclei-template?source=repeater')
  }, [router, t.common.warning, t.repeater.nucleiTemplate.missingResponse])

  // 刷新应用代理与后端主机的系统代理状态
  useEffect(() => {
    let cancelled = false

    const refreshProxy = async () => {
      const appProxy = getProxyUrl()
      if (!cancelled) {
        setAppProxyUrl(appProxy)
      }
      const proxy = await fetchSystemProxyInfo()
      if (!cancelled) {
        setSystemProxy(proxy)
      }
    }

    void refreshProxy()

    // 每 30 秒检查一次系统代理状态
    const intervalId = setInterval(() => {
      void refreshProxy()
    }, 30000)

    // 页面获取焦点、代理设置变更时也检查
    const handleRefresh = () => {
      void refreshProxy()
    }
    window.addEventListener('focus', handleRefresh)
    window.addEventListener('storage', handleRefresh)
    window.addEventListener('hephaestus:proxy-config-changed', handleRefresh)

    return () => {
      cancelled = true
      clearInterval(intervalId)
      window.removeEventListener('focus', handleRefresh)
      window.removeEventListener('storage', handleRefresh)
      window.removeEventListener('hephaestus:proxy-config-changed', handleRefresh)
    }
  }, [])

  // 从 API 加载保存的标签页数据
  useEffect(() => {
    const loadTabs = async () => {
      try {
        const response = await fetchWithAuth(`/api/v1/repeater/tabs`)
        if (response.ok) {
          const data = await response.json()
          if (data.success && data.data && Array.isArray(data.data)) {
            const loadedTabs = data.data.map((tab: RepeaterTab) => ({
              ...tab,
              responseLength: typeof tab.responseLength === 'number' ? tab.responseLength : 0,
              viewMode: tab.viewMode === 'hex' ? 'hex' : 'raw',
            }))
            setTabs(loadedTabs)
            // 恢复最后一个激活的标签页，或者选择第一个
            if (loadedTabs.length > 0) {
              // 从 localStorage 恢复 activeTab（临时存储，仅用于页面刷新时恢复）
              const savedActiveTab = localStorage.getItem('repeater_active_tab')
              if (savedActiveTab && loadedTabs.some((tab: RepeaterTab) => tab.id === savedActiveTab)) {
                setActiveTab(savedActiveTab)
              } else {
                setActiveTab(loadedTabs[0].id)
              }
            }
          }
        }
      } catch (error) {
        console.error('Failed to load tabs from API:', error)
      } finally {
        setIsInitialized(true)
      }
    }

    loadTabs()
  }, [])

  // 保存标签页数据到 API（使用防抖，避免频繁保存）
  const saveTabsToAPI = async (tabsToSave: RepeaterTab[]) => {
    try {
      const response = await fetchWithAuth(`/api/v1/repeater/tabs`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(tabsToSave),
      })

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        console.error('Failed to save tabs:', errorData.error || 'Unknown error')
      }
    } catch (error) {
      console.error('Failed to save tabs to API:', error)
    }
  }

  // 当标签页发生变化时，保存到 API（使用防抖）
  useEffect(() => {
    if (!isInitialized) return

    // 清除之前的定时器
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current)
    }

    // 保存 activeTab 到 localStorage（用于页面刷新时恢复）
    if (activeTab) {
      localStorage.setItem('repeater_active_tab', activeTab)
    }

    // 延迟保存，避免频繁请求
    saveTimeoutRef.current = setTimeout(() => {
      saveTabsToAPI(tabs)
    }, 500)

    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
      }
    }
  }, [activeTab, isInitialized, tabs])

  // 检查是否有待处理的请求（从漏洞详情发送过来的）
  // 使用定时检查支持重复发送
  useEffect(() => {
    if (!isInitialized) return // 等待初始化完成
    const checkAndLoadPendingRequest = () => {
      try {
        const pendingRequest = localStorage.getItem('repeater_pending_request')
        if (pendingRequest) {
          // 立即清除 localStorage，防止重复处理
          localStorage.removeItem('repeater_pending_request')
          const pendingPayload = parsePendingRepeaterRequest(pendingRequest)
          if (!pendingPayload?.raw.trim()) {
            return
          }

          // 解析请求获取 URL 和 method
          const lines = pendingPayload.raw.split("\n")
          let method = pendingPayload.method?.trim() || 'GET'
          let url = pendingPayload.url?.trim() || ''
          let forceHttps = typeof pendingPayload.forceHttps === 'boolean' ? pendingPayload.forceHttps : false

          if (!url && lines.length > 0) {
            const requestLine = lines[0]
            const hostLine = lines.find((line) => line.toLowerCase().startsWith("host:"))

            const requestMatch = requestLine.match(/^(\w+)\s+(.+?)\s+HTTP/)
            const hostMatch = hostLine?.match(/Host:\s*(.+)/)

            if (requestMatch && hostMatch) {
              method = requestMatch[1]
              const path = requestMatch[2].trim()
              const host = hostMatch[1].trim()
              forceHttps = typeof pendingPayload.forceHttps === 'boolean' ? pendingPayload.forceHttps : true
              url = `${forceHttps ? 'https' : 'http'}://${host}${path}`
            }
          }

          // 创建新标签页并加载请求（始终创建新标签页，不替换）
          setTabs((prevTabs) => {
            const maxNumber = prevTabs.reduce((max, tab) => {
              const match = tab.name.match(/请求-(\d+)/)
              if (match) {
                const num = parseInt(match[1], 10)
                return Math.max(max, num)
              }
              return max
            }, 0)

            const newId = `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
            const pendingName = pendingPayload.name?.trim()
            const newTab: RepeaterTab = {
              id: newId,
              name: pendingName ? pendingName.slice(0, 40) : `${t.repeater.request}-${maxNumber + 1}`,
              method: method,
              url: url,
              rawRequest: pendingPayload.raw,
              response: '',
              responseLength: 0,
              responseTime: 0,
              hasRedirect: false,
              isLoading: false,
              forceHttps: forceHttps,
              viewMode: 'raw',
            }

            // 在回调中设置 activeTab 确保同步
            setTimeout(() => {
              setActiveTab(newId)
            }, 0)

            // 始终返回新的数组，包含所有旧标签页和新标签页
            return [...prevTabs, newTab]
          })
        }
      } catch (error) {
        console.error('Failed to load pending request:', error)
      }
    }

    // 初始检查
    checkAndLoadPendingRequest()

    // 使用定时器定期检查，支持重复发送
    const intervalId = setInterval(checkAndLoadPendingRequest, 300)

    // 监听 storage 事件（当其他标签页/窗口修改 localStorage 时触发）
    const handleStorageChange = (e: StorageEvent) => {
      if (e.key === 'repeater_pending_request' && e.newValue) {
        checkAndLoadPendingRequest()
      }
    }
    window.addEventListener('storage', handleStorageChange)

    // 监听页面焦点变化（当从其他页面返回时检查）
    const handleFocus = () => {
      checkAndLoadPendingRequest()
    }
    window.addEventListener('focus', handleFocus)

    return () => {
      clearInterval(intervalId)
      window.removeEventListener('storage', handleStorageChange)
      window.removeEventListener('focus', handleFocus)
    }
  }, [isInitialized, t.repeater.request])

  const handleAddTab = () => {
    const newId = String(Date.now())
    const maxNumber = tabs.reduce((max, tab) => {
      const match = tab.name.match(new RegExp(`${t.repeater.request}-(\\d+)`))
      if (match) {
        const num = parseInt(match[1], 10)
        return Math.max(max, num)
      }
      return max
    }, 0)
    const newTab: RepeaterTab = {
      id: newId,
      name: `${t.repeater.request}-${maxNumber + 1}`,
      method: 'GET',
      url: '',
      rawRequest: DEFAULT_RAW_REQUEST,
      response: '',
      responseLength: 0,
      responseTime: 0,
      hasRedirect: false,
      isLoading: false,
      forceHttps: false,
      viewMode: 'raw',
    }
    setTabs([...tabs, newTab])
    setActiveTab(newId)
  }

  useEffect(() => {
    return () => {
      Object.values(abortControllersRef.current).forEach((controller) => controller.abort())
      abortControllersRef.current = {}
    }
  }, [])

  const createNextTabName = useCallback((existingTabs: RepeaterTab[]) => {
    const maxNumber = existingTabs.reduce((max, tab) => {
      const match = tab.name.match(new RegExp(`${t.repeater.request}-(\\d+)`))
      if (match) {
        const num = parseInt(match[1], 10)
        return Math.max(max, num)
      }
      return max
    }, 0)

    return `${t.repeater.request}-${maxNumber + 1}`
  }, [t.repeater.request])

  const duplicateCurrentTab = useCallback(() => {
    if (!currentTab) return

    const newId = `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
    setTabs((prevTabs) => {
      const duplicatedTab: RepeaterTab = {
        ...currentTab,
        id: newId,
        name: createNextTabName(prevTabs),
        response: '',
        responseLength: 0,
        responseTime: 0,
        hasRedirect: false,
        isLoading: false,
      }
      return [...prevTabs, duplicatedTab]
    })
    setActiveTab(newId)
  }, [createNextTabName, currentTab])

  useEffect(() => {
    const handleDuplicateShortcut = (event: KeyboardEvent) => {
      if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'r') {
        return
      }

      event.preventDefault()
      duplicateCurrentTab()
    }

    window.addEventListener('keydown', handleDuplicateShortcut)
    return () => {
      window.removeEventListener('keydown', handleDuplicateShortcut)
    }
  }, [duplicateCurrentTab])

  const rememberClosedTabs = (tabsToRemember: RepeaterTab[]) => {
    if (tabsToRemember.length === 0) return

    setClosedTabs((prev) => [
      ...prev,
      ...tabsToRemember.map((tab) => ({ ...tab, isLoading: false })),
    ].slice(-20))
  }

  const handleReopenClosedTab = () => {
    setClosedTabs((prevClosedTabs) => {
      const tabToReopen = prevClosedTabs[prevClosedTabs.length - 1]
      if (!tabToReopen) return prevClosedTabs

      const remainingClosedTabs = prevClosedTabs.slice(0, -1)
      setTabs((prevTabs) => {
        const nextTab = prevTabs.some((tab) => tab.id === tabToReopen.id)
          ? { ...tabToReopen, id: `${Date.now()}-${Math.random().toString(36).slice(2, 9)}` }
          : tabToReopen

        setActiveTab(nextTab.id)
        return [...prevTabs, nextTab]
      })

      return remainingClosedTabs
    })
  }

  const clearAbortController = useCallback((tabId: string, controller?: AbortController) => {
    const activeController = abortControllersRef.current[tabId]
    if (!activeController) return
    if (controller && activeController !== controller) return
    delete abortControllersRef.current[tabId]
  }, [])

  const handleCancelRequest = useCallback((tabId: string) => {
    const controller = abortControllersRef.current[tabId]
    if (!controller) return
    controller.abort()
    clearAbortController(tabId, controller)
  }, [clearAbortController])

  const handleRemoveTab = (id: string) => {
    handleCancelRequest(id)
    delete responseSelectionRef.current[id]
    const tabToRemove = tabs.find((tab) => tab.id === id)
    if (tabToRemove) {
      rememberClosedTabs([tabToRemove])
    }

    const newTabs = tabs.filter((tab) => tab.id !== id)
    setTabs(newTabs)
    if (activeTab === id) {
      // 如果删除的是当前激活的标签页
      if (newTabs.length > 0) {
        setActiveTab(newTabs[0].id)
      } else {
        setActiveTab(null)
      }
    }
  }

  // 关闭其他标签页
  const handleCloseOtherTabs = (id: string) => {
    tabs.forEach((tab) => {
      if (tab.id !== id) {
        handleCancelRequest(tab.id)
      }
    })
    rememberClosedTabs(tabs.filter((tab) => tab.id !== id))
    const newTabs = tabs.filter((tab) => tab.id === id)
    setTabs(newTabs)
    setActiveTab(id)
  }

  // 关闭所有标签页
  const handleCloseAllTabs = () => {
    tabs.forEach((tab) => handleCancelRequest(tab.id))
    rememberClosedTabs(tabs)
    setTabs([])
    setActiveTab(null)
  }

  const handleUpdateTab = (id: string, updates: Partial<RepeaterTab>) => {
    setTabs((prevTabs) => prevTabs.map((tab) => (tab.id === id ? { ...tab, ...updates } : tab)))
  }

  // 开始编辑tab名称
  const handleStartEdit = (tabId: string, currentName: string) => {
    setEditingTabId(tabId)
    setEditingName(currentName)
    // 使用 setTimeout 确保 DOM 更新后再聚焦
    setTimeout(() => {
      editInputRef.current?.focus()
      editInputRef.current?.select()
    }, 0)
  }

  // 完成编辑
  const handleFinishEdit = () => {
    if (editingTabId && editingName.trim()) {
      handleUpdateTab(editingTabId, { name: editingName.trim() })
    }
    setEditingTabId(null)
    setEditingName('')
  }

  // 取消编辑
  const handleCancelEdit = () => {
    setEditingTabId(null)
    setEditingName('')
  }

  // 拖拽开始
  const handleDragStart = (e: React.DragEvent, tabId: string) => {
    setDraggedTabId(tabId)
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', tabId)
  }

  // 拖拽经过
  const handleDragOver = (e: React.DragEvent, tabId: string) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    if (draggedTabId !== tabId) {
      setDragOverTabId(tabId)
    }
  }

  // 拖拽离开
  const handleDragLeave = () => {
    setDragOverTabId(null)
  }

  // 拖拽结束
  const handleDragEnd = () => {
    setDraggedTabId(null)
    setDragOverTabId(null)
  }

  // 放置
  const handleDrop = (e: React.DragEvent, targetTabId: string) => {
    e.preventDefault()
    if (!draggedTabId || draggedTabId === targetTabId) {
      handleDragEnd()
      return
    }

    const draggedIndex = tabs.findIndex(tab => tab.id === draggedTabId)
    const targetIndex = tabs.findIndex(tab => tab.id === targetTabId)

    if (draggedIndex === -1 || targetIndex === -1) {
      handleDragEnd()
      return
    }

    // 重新排序
    const newTabs = [...tabs]
    const [draggedTab] = newTabs.splice(draggedIndex, 1)
    newTabs.splice(targetIndex, 0, draggedTab)

    setTabs(newTabs)
    handleDragEnd()
  }

  // Parse RAW request to extract URL
  // forceHttps 用于决定拼接出来的 URL 使用 http 还是 https
  const parseRawToUrl = (
    raw: string,
    tabId: string,
    forceHttps: boolean
  ): { method: string; url: string } | null => {
    const lines = raw.split("\n")
    if (lines.length === 0) return null

    const requestLine = lines[0]
    const hostLine = lines.find((line) => line.toLowerCase().startsWith("host:"))

    const requestMatch = requestLine.match(/^(\w+)\s+(.+?)\s+HTTP/)
    const hostMatch = hostLine?.match(/Host:\s*(.+)/)

    if (requestMatch && hostMatch) {
      const newMethod = requestMatch[1]
      const path = requestMatch[2].trim()
      const host = hostMatch[1].trim()

      const scheme = forceHttps ? "https" : "http"
      const parsedUrl = `${scheme}://${host}${path}`
      handleUpdateTab(tabId, { method: newMethod, url: parsedUrl })
      return { method: newMethod, url: parsedUrl }
    }
    return null
  }

  // Parse URL to update RAW request
  // 这里避免直接使用 new URL，以防止在输入不完整 URL 时抛出异常
  const parseUrlToRaw = (newUrl: string, newMethod: string, tabId: string) => {
    setTabs((prevTabs) => {
      const tab = prevTabs.find((t) => t.id === tabId)
      if (!tab) return prevTabs

      const trimmed = newUrl.trim()
      if (!trimmed) return prevTabs

      // 简单解析 URL，支持以下形式：
      // - https://example.com/path?query
      // - http://example.com/path
      // - example.com/path
      // - example.com
      const match = trimmed.match(/^(https?:\/\/)?([^/]+)(\/.*)?$/)
      if (!match) {
        console.warn("Invalid URL, skip syncing to RAW:", newUrl)
        return prevTabs
      }

      const host = match[2] // 域名:端口
      const pathWithQuery = match[3] || "/" // path + query，默认为 /

      const lines = tab.rawRequest.split("\n")

      // 更新请求行
      lines[0] = `${newMethod} ${pathWithQuery} HTTP/1.1`

      // 更新或添加 Host 头
      const hostIndex = lines.findIndex((line) => line.toLowerCase().startsWith("host:"))
      if (hostIndex !== -1) {
        lines[hostIndex] = `Host: ${host}`
      } else {
        lines.splice(1, 0, `Host: ${host}`)
      }

      return prevTabs.map((t) =>
        t.id === tabId ? { ...t, rawRequest: lines.join("\n") } : t
      )
    })
  }

  const handleUrlChange = (newUrl: string, tabId: string) => {
    handleUpdateTab(tabId, { url: newUrl })
    setTabs((prevTabs) => {
      const tab = prevTabs.find((t) => t.id === tabId)
      if (tab) {
        parseUrlToRaw(newUrl, tab.method, tabId)
      }
      return prevTabs
    })
  }

  const handleMethodChange = (newMethod: string, tabId: string) => {
    handleUpdateTab(tabId, { method: newMethod })
    setTabs((prevTabs) => {
      const tab = prevTabs.find((t) => t.id === tabId)
      if (tab && tab.url) {
        parseUrlToRaw(tab.url, newMethod, tabId)
      }
      return prevTabs
    })
  }

  const handleRawRequestChange = (newRaw: string, tabId: string) => {
    handleUpdateTab(tabId, { rawRequest: newRaw })
    // 当 RAW 请求变化时，自动解析并更新 URL（特别是 Host 头变化时）
    const tab = tabs.find((t) => t.id === tabId)
    parseRawToUrl(newRaw, tabId, tab?.forceHttps ?? false)
  }

  const handleSendRequest = async (needRedirect: boolean = false) => {
    if (!currentTab || !activeTab) return

    const tabId = activeTab
    const tabSnapshot = currentTab

    if (!tabSnapshot.rawRequest.trim()) {
      toast.warning({ title: t.common.warning, message: t.repeater.enterRequest })
      return
    }

    // 如果 URL 为空，尝试从 RAW 请求中解析 URL
    if (!tabSnapshot.url || !tabSnapshot.url.trim()) {
      const parsed = parseRawToUrl(tabSnapshot.rawRequest, tabId, tabSnapshot.forceHttps)
      if (!parsed) {
        toast.warning({ title: t.common.warning, message: t.repeater.invalidUrl })
        return
      }
    }

    const controller = new AbortController()
    abortControllersRef.current[tabId] = controller

    handleUpdateTab(tabId, { isLoading: true, response: '', responseLength: 0 })
    if (needRedirect) {
      handleUpdateTab(tabId, { hasRedirect: false })
    }

    try {
      // 使用开关直接控制 forceHttps，不依赖 URL 协议
      const forceHttps = tabSnapshot.forceHttps
      const targetURL = buildRequestUrl(tabSnapshot) || tabSnapshot.url || ''
      const resolvedProxy = resolveProxyUrl(systemProxy)
      const bypassSystemProxy = resolvedProxy.source === 'system'
        && targetURL !== ''
        && shouldBypassProxy(targetURL, systemProxy?.noProxy)
      const proxyURL = bypassSystemProxy ? '' : resolvedProxy.url

      const response = await fetchWithAuth(`/api/v1/repeater/send`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        signal: controller.signal,
        body: JSON.stringify({
          raw: tabSnapshot.rawRequest,
          forceHttps: forceHttps,
          redirect: needRedirect,
          proxyURL,
        }),
      })

      const data = await response.json()

      if (!data.success) {
        handleUpdateTab(tabId, {
          response: data.error || '请求失败',
          responseLength: 0,
          isLoading: false,
        })
        clearAbortController(tabId, controller)
        return
      }

      handleUpdateTab(tabId, {
        response: data.response || '',
        responseLength: typeof data.responseLength === 'number' ? data.responseLength : 0,
        responseTime: data.responseTime || 0,
        hasRedirect: data.hasRedirect || false,
        isLoading: false,
      })
      clearAbortController(tabId, controller)

    } catch (error: any) {
      handleUpdateTab(tabId, {
        response: error.name === 'AbortError' ? t.repeater.requestCancelled : (error.message || '请求失败'),
        responseLength: 0,
        isLoading: false,
      })
      clearAbortController(tabId, controller)
    }
  }

  // HEX 转文本（简化版，只提取 ASCII 可打印字符或尝试解析 HEX）
  const hexToText = (hexView: string): string => {
    // 如果输入看起来像 HEX 视图格式，尝试解析
    const lines = hexView.split('\n')
    const bytes: number[] = []

    for (const line of lines) {
      // 匹配格式: 00000000  48 65 6c 6c 6f 20 57 6f  72 6c 64 21 ...  |Hello World!|
      const match = line.match(/^[0-9a-f]{8}\s+([0-9a-f\s]+)\s+\|.*\|$/i)
      if (match) {
        const hexPart = match[1].trim()
        const hexBytes = hexPart.split(/\s+/).filter(h => h.length === 2)
        for (const hb of hexBytes) {
          const val = parseInt(hb, 16)
          if (!isNaN(val)) {
            bytes.push(val)
          }
        }
      }
    }

    if (bytes.length > 0) {
      return new TextDecoder().decode(new Uint8Array(bytes))
    }

    // 如果无法解析，返回原文
    return hexView
  }

  // 切换 HEX 视图
  const toggleHexView = (tabId: string) => {
    const tab = tabs.find(t => t.id === tabId)
    if (!tab) return

    handleUpdateTab(tabId, { viewMode: tab.viewMode === 'hex' ? 'raw' : 'hex' })
  }

  function shellSplit(input: string): string[] {
    const tokens: string[] = []
    let current = ''
    let quote: '"' | "'" | null = null

    for (let i = 0; i < input.length; i++) {
      const c = input[i]

      if ((c === '"' || c === "'")) {
        if (quote === c) {
          quote = null
        } else if (!quote) {
          quote = c
        } else {
          current += c
        }
        continue
      }

      if (c === ' ' && !quote) {
        if (current) {
          tokens.push(current)
          current = ''
        }
      } else {
        current += c
      }
    }

    if (current) tokens.push(current)
    return tokens
  }


  // 解析 cURL 命令为 RAW 请求
  const parseCurlToRaw = (curlCommand: string): { raw: string; method: string; url: string; forceHttps: boolean } | null => {
    try {
      let cmd = curlCommand
        .replace(/\\\r?\n/g, ' ')
        .trim()

      if (!cmd.toLowerCase().startsWith('curl ')) {
        return null
      }

      cmd = cmd.replace(/^curl\s+/i, '')

      const tokens = shellSplit(cmd)

      let method = 'GET'
      let url = ''
      let body = ''
      let forceHttps = false
      let cookie = ''

      const headers: Record<string, string> = {}

      for (let i = 0; i < tokens.length; i++) {
        const token = tokens[i]

        switch (token) {
          case '-X':
          case '--request':
            method = (tokens[i + 1] || 'GET').toUpperCase()
            i++
            break

          case '-H':
          case '--header': {
            const headerLine = tokens[i + 1]
            if (headerLine) {
              const idx = headerLine.indexOf(':')
              if (idx > 0) {
                headers[headerLine.slice(0, idx).trim()] =
                  headerLine.slice(idx + 1).trim()
              }
            }
            i++
            break
          }

          case '-d':
          case '--data':
          case '--data-raw':
          case '--data-binary':
            body = tokens[i + 1] || ''
            if (!tokens.includes('-X') && !tokens.includes('--request')) {
              method = 'POST'
            }
            i++
            break

          case '-u':
          case '--user': {
            const auth = tokens[i + 1]
            if (auth) {
              headers['Authorization'] = `Basic ${btoa(auth)}`
            }
            i++
            break
          }

          case '-A':
          case '--user-agent':
            headers['User-Agent'] = tokens[i + 1] || ''
            i++
            break

          case '-b':
          case '--cookie':
            cookie = tokens[i + 1] || ''
            i++
            break

          case '--compressed':
            if (!headers['Accept-Encoding']) {
              headers['Accept-Encoding'] = 'gzip, deflate'
            }
            break

          default:
            if (!token.startsWith('-') && token.startsWith('http')) {
              url = token
            }
        }
      }

      if (!url) return null

      forceHttps = url.startsWith('https://')

      const urlObj = new URL(url)
      const host = urlObj.host
      const path = urlObj.pathname + urlObj.search

      // 默认 UA
      if (!headers['User-Agent']) {
        headers['User-Agent'] =
          'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
      }

      if (cookie && !headers['Cookie']) {
        headers['Cookie'] = cookie
      }

      const lines: string[] = []

      lines.push(`${method} ${path || '/'} HTTP/1.1`)
      lines.push(`Host: ${host}`)

      for (const [k, v] of Object.entries(headers)) {
        if (k.toLowerCase() !== 'host') {
          lines.push(`${k}: ${v}`)
        }
      }

      if (body) {
        lines.push(`Content-Length: ${new TextEncoder().encode(body).length}`)
      }

      lines.push('')

      if (body) {
        lines.push(body)
      }

      return {
        raw: lines.join('\n'),
        method,
        url,
        forceHttps
      }
    } catch (e) {
      console.error('parseCurlToRaw error:', e)
      return null
    }
  }

  // 美化/格式化 HTTP 请求
  const beautifyRequest = (raw: string): string => {
    // 分离请求头和请求体
    const parts = raw.split(/\r?\n\r?\n/)
    if (parts.length === 0) return raw

    const headersPart = parts[0]
    const bodyPart = parts.slice(1).join('\n\n')

    // 格式化请求头
    const headerLines = headersPart.split(/\r?\n/)
    const formattedHeaders: string[] = []
    
    for (let i = 0; i < headerLines.length; i++) {
      const line = headerLines[i]
      
      if (i === 0) {
        // 请求行：保持原样，但清理多余空格
        formattedHeaders.push(line.replace(/\s+/g, ' ').trim())
      } else {
        // 头部行：格式化 "Header: Value" 格式
        const colonIndex = line.indexOf(':')
        if (colonIndex > 0) {
          const headerName = line.substring(0, colonIndex).trim()
          const headerValue = line.substring(colonIndex + 1).trim()
          formattedHeaders.push(`${headerName}: ${headerValue}`)
        } else if (line.trim()) {
          formattedHeaders.push(line.trim())
        }
      }
    }

    // 格式化请求体
    let formattedBody = bodyPart

    if (bodyPart.trim()) {
      // 尝试解析为 JSON 并格式化
      try {
        // 先尝试直接解析
        const parsed = JSON.parse(bodyPart.trim())
        formattedBody = JSON.stringify(parsed, null, 2)
      } catch {
        // 如果解析失败，尝试处理 URL 编码的表单数据
        if (bodyPart.includes('=') && !bodyPart.includes('{')) {
          // 可能是 form-urlencoded 格式
          try {
            const pairs = bodyPart.trim().split('&')
            const formatted = pairs.map(pair => {
              const [key, value] = pair.split('=')
              if (key && value !== undefined) {
                return `${decodeURIComponent(key)}=${decodeURIComponent(value)}`
              }
              return pair
            })
            formattedBody = formatted.join('&\n')
          } catch {
            // 保持原样
            formattedBody = bodyPart
          }
        } else {
          // 尝试处理可能有转义或格式问题的 JSON
          try {
            // 移除可能的转义字符
            const cleaned = bodyPart.trim()
              .replace(/\\"/g, '"')
              .replace(/\\n/g, '\n')
              .replace(/\\t/g, '\t')
            const parsed = JSON.parse(cleaned)
            formattedBody = JSON.stringify(parsed, null, 2)
          } catch {
            // 保持原样
            formattedBody = bodyPart
          }
        }
      }
    }

    // 组合结果
    if (formattedBody.trim()) {
      return formattedHeaders.join('\n') + '\n\n' + formattedBody
    }
    return formattedHeaders.join('\n') + '\n'
  }

  // 处理美化按钮点击
  const handleBeautify = (tabId: string) => {
    const tab = tabs.find(t => t.id === tabId)
    if (!tab) return

    const beautified = beautifyRequest(tab.rawRequest)
    handleUpdateTab(tabId, { rawRequest: beautified })
    
    // 同步更新 URL
    parseRawToUrl(beautified, tabId, tab.forceHttps)
    
    toast.success({ title: t.common.success, message: t.repeater.beautifySuccess })
  }

  // 从 cURL 导入请求
  const handleImportCurl = () => {
    if (!curlInput.trim()) {
      toast.warning({ title: t.common.warning, message: t.repeater.enterCurlCommand })
      return
    }

    const parsed = parseCurlToRaw(curlInput)
    if (!parsed) {
      toast.error({ title: t.common.error, message: t.repeater.curlParseError })
      return
    }

    // 创建新标签页
    const newId = String(Date.now())
    const maxNumber = tabs.reduce((max, tab) => {
      const match = tab.name.match(new RegExp(`${t.repeater.request}-(\\d+)`))
      if (match) {
        const num = parseInt(match[1], 10)
        return Math.max(max, num)
      }
      return max
    }, 0)

    const newTab: RepeaterTab = {
      id: newId,
      name: `${t.repeater.request}-${maxNumber + 1}`,
      method: parsed.method,
      url: parsed.url,
      rawRequest: parsed.raw,
      response: '',
      responseLength: 0,
      responseTime: 0,
      hasRedirect: false,
      isLoading: false,
      forceHttps: parsed.forceHttps,
      viewMode: 'raw',
    }

    setTabs([...tabs, newTab])
    setActiveTab(newId)
    setCurlDialogOpen(false)
    setCurlInput('')
    toast.success({ title: t.common.success, message: t.repeater.curlImportSuccess })
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      {/* Tabs */}
      {(tabs.length > 0 || closedTabs.length > 0) && (
        <div className="heph-repeater-tabs-bar flex shrink-0 items-start gap-1 border-b border-border bg-card p-2">
          <Tabs value={activeTab || ''} onValueChange={setActiveTab} className="min-w-0 flex-1">
            <div
              className={cn(
                'heph-tabs-scrollbar overflow-x-auto md:overflow-x-visible',
                tabsDisplayMode === 'scroll' && 'md:overflow-x-auto'
              )}
              onWheel={handleTabsWheel}
            >
              <TabsList
                className={cn(
                  'heph-repeater-tabs-list h-auto justify-start rounded-none border-0 bg-transparent p-0',
                  tabsDisplayMode === 'scroll'
                    ? 'inline-flex min-w-max flex-nowrap'
                    : 'inline-flex min-w-max flex-nowrap md:flex md:min-w-0 md:w-full md:flex-wrap'
                )}
              >
                {tabs.map((tab) => (
                  <ContextMenu key={tab.id}>
                    <ContextMenuTrigger asChild>
                      <div
                        className={cn(
                          'relative group',
                          tabsDisplayMode === 'scroll' && 'shrink-0',
                          draggedTabId === tab.id && 'opacity-50',
                          dragOverTabId === tab.id && 'border-l-2 border-primary'
                        )}
                        draggable={editingTabId !== tab.id}
                        onDragStart={(e) => handleDragStart(e, tab.id)}
                        onDragOver={(e) => handleDragOver(e, tab.id)}
                        onDragLeave={handleDragLeave}
                        onDragEnd={handleDragEnd}
                        onDrop={(e) => handleDrop(e, tab.id)}
                      >
                        {editingTabId === tab.id ? (
                          <Input
                            ref={editInputRef}
                            value={editingName}
                            onChange={(e) => setEditingName(e.target.value)}
                            onBlur={handleFinishEdit}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') {
                                handleFinishEdit()
                              } else if (e.key === 'Escape') {
                                handleCancelEdit()
                              }
                            }}
                            className="h-9 min-w-[100px] px-2 text-sm"
                            onClick={(e) => e.stopPropagation()}
                            onDoubleClick={(e) => e.stopPropagation()}
                          />
                        ) : (
                          <TabsTrigger
                            value={tab.id}
                            className="relative pr-8 data-[state=active]:bg-muted cursor-grab active:cursor-grabbing"
                            onDoubleClick={(e) => {
                              e.stopPropagation()
                              handleStartEdit(tab.id, tab.name)
                            }}
                          >
                            {tab.name}
                          </TabsTrigger>
                        )}
                        {tabs.length > 0 && editingTabId !== tab.id && (
                          <button
                            type="button"
                            className="absolute right-2 top-1/2 -translate-y-1/2 h-4 w-4 rounded-full bg-muted hover:bg-destructive/80 hover:text-white text-muted-foreground flex items-center justify-center text-xs transition-colors z-10 opacity-100 md:opacity-0 md:group-hover:opacity-100"
                            onClick={(e) => {
                              e.stopPropagation()
                              handleRemoveTab(tab.id)
                            }}
                            aria-label="删除标签页"
                          >
                            <X className="h-3 w-3" />
                          </button>
                        )}
                      </div>
                    </ContextMenuTrigger>
                    <ContextMenuContent>
                      <ContextMenuItem onClick={() => handleRemoveTab(tab.id)}>
                        {t.repeater.contextMenu.closeCurrentTab}
                      </ContextMenuItem>
                      <ContextMenuItem
                        onClick={() => handleCloseOtherTabs(tab.id)}
                        disabled={tabs.length <= 1}
                      >
                        {t.repeater.contextMenu.closeOtherTabs}
                      </ContextMenuItem>
                      <ContextMenuSeparator />
                      <ContextMenuItem
                        onClick={handleCloseAllTabs}
                        className="text-destructive focus:text-destructive"
                      >
                        {t.repeater.contextMenu.closeAllTabs}
                      </ContextMenuItem>
                    </ContextMenuContent>
                  </ContextMenu>
                ))}
                {/* 新增按钮放在最后一个tab后面 */}
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleAddTab}
                  className="h-9 px-3 ml-1 shrink-0"
                >
                  <Plus className="h-4 w-4" />
                </Button>
              </TabsList>
            </div>
          </Tabs>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-9 w-9 shrink-0 text-muted-foreground hover:text-foreground"
                aria-label={t.repeater.tabsLayout.menuLabel}
              >
                <MoreVertical className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuItem onClick={handleReopenClosedTab} disabled={closedTabs.length === 0}>
                {t.repeater.tabsLayout.reopenClosedTab}
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleCloseAllTabs} disabled={tabs.length === 0}>
                {t.repeater.tabsLayout.closeAllTabs}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuSub>
                <DropdownMenuSubTrigger>
                  {t.repeater.tabsLayout.menuLabel}
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-48">
                  <DropdownMenuRadioGroup value={tabsDisplayMode} onValueChange={handleTabsDisplayModeChange}>
                    <DropdownMenuRadioItem value="scroll">
                      {t.repeater.tabsLayout.scroll}
                    </DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="wrap">
                      {t.repeater.tabsLayout.wrap}
                    </DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}

      {/* Main Content */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {tabs.length === 0 ? (
          /* 空状态 */
          <div className="flex flex-1 items-center justify-center">
            <div className="text-center space-y-6">
              <div className="inline-flex items-center justify-center w-20 h-20 rounded-full bg-gradient-to-br from-primary/10 to-primary/5">
                <FileText className="w-10 h-10 text-primary/60" />
              </div>
              <div>
                <h3 className="text-lg font-semibold mb-2">{t.repeater.noRequests}</h3>
                <p className="text-muted-foreground mb-6">
                  {t.repeater.noRequestsDesc}
                </p>
                <Button onClick={handleAddTab} className="gap-2">
                  <Plus className="w-4 h-4" />
                  {t.repeater.createNewRequest}
                </Button>
              </div>
            </div>
          </div>
        ) : (
          tabs.map((tab) => {
            if (activeTab !== tab.id) return null

            return (
              <div
                key={tab.id}
                className="flex min-h-0 flex-1 flex-col overflow-hidden w-full scan-panel-enter"
              >
                {/* Effective Proxy Status Indicator */}
                {(systemProxy || appProxyUrl) && (
                  <div className={`flex items-center gap-2 px-3 py-1.5 text-xs border-b ${activeProxyUrl
                      ? 'bg-amber-500/10 border-amber-500/20 text-amber-600 dark:text-amber-400'
                      : 'bg-muted/50 border-border text-muted-foreground'
                    }`}>
                    {activeProxyUrl ? (
                      <>
                        <Wifi className="h-3.5 w-3.5" />
                        <span>
                          {activeProxySource === 'app'
                            ? t.repeater.appProxyEnabled
                            : t.repeater.systemProxyEnabled}
                        </span>
                        <span className="font-mono bg-background/50 px-1.5 py-0.5 rounded">
                          {maskProxyUrl(activeProxyUrl)}
                        </span>
                      </>
                    ) : (
                      <>
                        <WifiOff className="h-3.5 w-3.5" />
                        <span>{t.repeater.proxyDisabled}</span>
                      </>
                    )}
                  </div>
                )}

                {/* URL Bar - 顶层共享区域 */}
                <div className="flex shrink-0 flex-col gap-2 border-b border-border bg-card p-3 md:flex-row md:items-center md:gap-2">
                  <div className="flex min-w-0 w-full items-center gap-2 md:flex-1">
                    <Select value={tab.method} onValueChange={(method) => handleMethodChange(method, tab.id)}>
                      <SelectTrigger className="heph-repeater-method-select !h-11 w-28 shrink-0 bg-secondary font-mono text-sm md:!h-9">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="GET">GET</SelectItem>
                        <SelectItem value="POST">POST</SelectItem>
                        <SelectItem value="PUT">PUT</SelectItem>
                        <SelectItem value="DELETE">DELETE</SelectItem>
                        <SelectItem value="PATCH">PATCH</SelectItem>
                        <SelectItem value="HEAD">HEAD</SelectItem>
                        <SelectItem value="OPTIONS">OPTIONS</SelectItem>
                      </SelectContent>
                    </Select>
                    <div className="relative min-w-0 flex-1">
                      <Input
                        value={tab.url}
                        onChange={(e) => handleUrlChange(e.target.value, tab.id)}
                        placeholder={t.repeater.urlPlaceholder}
                        className="heph-repeater-url-input h-11 bg-input font-mono text-base md:h-9 md:text-sm"
                        readOnly={privacyMode}
                      />
                      {privacyMode && (
                        <div className="heph-repeater-url-privacy-mask pointer-events-none absolute inset-0 flex items-center rounded-md border border-transparent bg-input px-3 font-mono text-sm text-foreground">
                          <span className="truncate">{getMaskedUrl(tab.url)}</span>
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="flex w-full items-center gap-2 md:w-auto">
                    <div className="flex shrink-0 items-center gap-2 px-1">
                      <Label htmlFor={`https-switch-${tab.id}`} className="cursor-pointer whitespace-nowrap text-xs text-muted-foreground">
                        {t.repeater.https}
                      </Label>
                      <Switch
                        id={`https-switch-${tab.id}`}
                        checked={tab.forceHttps}
                        onCheckedChange={(checked) => handleUpdateTab(tab.id, { forceHttps: checked })}
                      />
                    </div>
                    <Button onClick={() => handleSendRequest(false)} disabled={tab.isLoading} className="h-11 w-28 flex-none justify-center gap-2 md:h-9 md:w-auto">
                      {tab.isLoading ? (
                        <>
                          <Loader2 className="h-4 w-4 animate-spin" />
                          <span aria-live="polite">{t.repeater.sending}</span>
                        </>
                      ) : (
                        <>
                          <Send className="h-4 w-4" />
                          {t.repeater.send}
                        </>
                      )}
                    </Button>
                    {tab.isLoading && (
                      <Button
                        variant="outline"
                        onClick={() => handleCancelRequest(tab.id)}
                        className="h-11 w-28 flex-none justify-center gap-2 md:h-9 md:w-auto"
                      >
                        <XCircle className="h-4 w-4" />
                        {t.repeater.cancelRequest}
                      </Button>
                    )}
                  </div>
                </div>

                {/* Request and Response Panels */}
                <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:flex-row md:overflow-hidden">
                  {/* Request Panel */}
                  <div className={`relative flex min-h-0 flex-col border-r border-border transition-all ${fullscreenPanel === 'response' ? 'hidden' : fullscreenPanel === 'request' ? 'w-full flex-1' : 'w-full min-h-[320px] shrink-0 md:min-h-0 md:w-1/2 md:shrink'
                    }`}>
                    {/* Request Title */}
                    <div className="relative z-0 flex h-12 min-h-12 shrink-0 flex-nowrap items-center gap-1 overflow-x-auto border-b border-border bg-card px-2 md:gap-2 md:px-4">
                      <h2 className="shrink-0 font-mono text-sm font-semibold text-foreground">{t.repeater.request}</h2>
                      <div className="ml-auto flex shrink-0 items-center gap-1 md:gap-2">
                        <Button
                          variant={privacyMode ? "secondary" : "ghost"}
                          size="sm"
                          onClick={() => setPrivacyMode((prev) => !prev)}
                          className="h-11 md:h-7 px-2"
                          title={t.repeater.privacyMode}
                        >
                          {t.repeater.privacyMode}
                        </Button>
                        {tab.hasRedirect && (
                          <Button
                            variant="default"
                            size="sm"
                            onClick={() => handleSendRequest(true)}
                            disabled={tab.isLoading}
                            className="h-11 md:h-7 px-3 gap-1.5"
                            title={t.repeater.followRedirect}
                          >
                            <Send className="h-3.5 w-3.5" />
                            {t.repeater.follow}
                          </Button>
                        )}
                        {/* cURL 导入按钮 */}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => setCurlDialogOpen(true)}
                          className="h-11 md:h-7 px-2"
                          title={t.repeater.importCurl}
                        >
                          cURL
                        </Button>
                        
                        {/* HEX 视图切换按钮 */}
                        <Button
                          variant={tab.viewMode === 'hex' ? 'secondary' : 'ghost'}
                          size="sm"
                          onClick={() => toggleHexView(tab.id)}
                          className="h-11 md:h-7 px-2"
                          title={tab.viewMode === 'hex' ? t.repeater.switchToRaw : t.repeater.switchToHex}
                        >
                          HEX
                        </Button>
                        {/* 美化按钮 */}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => handleBeautify(tab.id)}
                          className="h-11 md:h-7 px-2"
                          title={t.repeater.beautify}
                          disabled={tab.viewMode === 'hex'}
                        >
                          <Sparkles className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setFullscreenPanel(fullscreenPanel === 'request' ? null : 'request')
                          }}
                          className="h-11 md:h-7 px-2"
                          title={fullscreenPanel === 'request' ? t.repeater.exitFullscreen : t.repeater.fullscreen}
                        >
                          {fullscreenPanel === 'request' ? (
                            <Minimize2 className="h-3.5 w-3.5" />
                          ) : (
                            <Maximize2 className="h-3.5 w-3.5" />
                          )}
                        </Button>
                      </div>
                    </div>

                    {/* Request Editor - Raw/HEX */}
                    <div className="relative z-10 min-h-0 flex-1 overflow-hidden">
                      {tab.viewMode === 'hex' ? (
                        <HexViewer
                          key={`hex-${tab.id}-${privacyMode ? 'masked' : 'raw'}`}
                          data={tab.rawRequest}
                        />
                      ) : (
                        <RequestEditor
                          key={`request-${tab.id}-${privacyMode ? 'masked' : 'raw'}`}
                          value={tab.rawRequest}
                          onChange={(newRaw) => handleRawRequestChange(newRaw, tab.id)}
                          privacyMode={privacyMode}
                          readOnly={privacyMode}
                          onCopyUrl={() => handleCopyUrl(tab)}
                          onCopyAsCurl={() => handleCopyAsCurl(tab)}
                          copyAsCurlLabel={t.repeater.copyAsCurl}
                        />
                      )}
                    </div>
                  </div>

                  {/* Response Panel */}
                  <div className={`relative flex min-h-0 flex-col transition-all ${fullscreenPanel === 'request' ? 'hidden' : fullscreenPanel === 'response' ? 'w-full flex-1' : 'w-full min-h-[320px] shrink-0 md:min-h-0 md:w-1/2 md:shrink'
                    }`}>
                    <div className="relative z-0 flex h-12 min-h-12 shrink-0 flex-nowrap items-center gap-1 overflow-x-auto border-b border-border bg-card px-2 md:gap-2 md:px-4">
                      <h2 className="shrink-0 font-mono text-sm font-semibold text-foreground">{t.repeater.response}</h2>
                      <div className="ml-auto flex shrink-0 items-center gap-1 md:gap-2">
                        {tab.responseTime > 0 && (
                          <span className="text-xs text-muted-foreground">
                            {tab.responseTime} ms
                          </span>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => openNucleiTemplatePage(tab)}
                          className="h-11 md:h-7 px-2 gap-1.5"
                          title={t.repeater.nucleiTemplate.generate}
                          disabled={!tab.response}
                        >
                          <FileText className="h-3.5 w-3.5" />
                          <span className="hidden sm:inline">{t.repeater.nucleiTemplate.button}</span>
                        </Button>
                        {tab.url && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => {
                              if (!tab.url || !tab.url.trim()) {
                                return
                              }
                              try {
                                const urlToOpen = tab.url.trim()
                                new URL(urlToOpen)
                                const link = window.open(urlToOpen, "_blank", "noopener,noreferrer")
                                if (!link) {
                                  toast.warning({ title: t.common.warning, message: t.repeater.popupBlocked })
                                }
                              } catch (error) {
                                toast.warning({ title: t.common.warning, message: t.repeater.invalidUrlFormat })
                              }
                            }}
                            className="h-11 md:h-7 px-2"
                            title={t.repeater.openInBrowser}
                          >
                            <Globe className="h-3.5 w-3.5" />
                          </Button>
                        )}
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => {
                            setFullscreenPanel(fullscreenPanel === 'response' ? null : 'response')
                          }}
                          className="h-11 md:h-7 px-2"
                          title={fullscreenPanel === 'response' ? t.repeater.exitFullscreen : t.repeater.fullscreen}
                        >
                          {fullscreenPanel === 'response' ? (
                            <Minimize2 className="h-3.5 w-3.5" />
                          ) : (
                            <Maximize2 className="h-3.5 w-3.5" />
                          )}
                        </Button>
                      </div>
                    </div>
                    {/* Response Viewer - Raw only */}
                    <div className="relative z-10 min-h-0 flex-1 overflow-hidden">
                      <ResponseViewer
                        key={`response-${tab.id}-${privacyMode ? 'masked' : 'raw'}`}
                        value={tab.response}
                        isLoading={tab.isLoading}
                        privacyMode={privacyMode}
                        onCopyDownloadCurl={tab.response ? () => handleCopyDownloadCurl(tab) : undefined}
                        onSelectionChange={(selections) => {
                          responseSelectionRef.current[tab.id] = selections
                        }}
                      />
                    </div>
                  </div>
                </div>
              </div>
            )
          })
        )}
      </div>

      {/* cURL 导入对话框 */}
      <Dialog open={curlDialogOpen} onOpenChange={setCurlDialogOpen}>
        <DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle>{t.repeater.importCurl}</DialogTitle>
            <DialogDescription>
              {t.repeater.curlDialogDesc}
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <Textarea
              value={curlInput}
              onChange={(e) => setCurlInput(e.target.value)}
              placeholder={t.repeater.curlPlaceholder}
              className="min-h-[240px] max-h-[60vh] resize-y overflow-auto whitespace-pre-wrap break-all font-mono text-sm [field-sizing:fixed]"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCurlDialogOpen(false)}>
              {t.common.cancel}
            </Button>
            <Button onClick={handleImportCurl}>
              {t.repeater.import}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </div>
  )
}
