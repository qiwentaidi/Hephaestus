"use client"

import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent } from "react"
import { Bug, CheckCircle2, Copy, FileCode2, Loader2, Play, Plus, Save, Sparkles, X, XCircle } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { fetchWithAuth } from "@/lib/api"
import { copyToClipboard } from "@/lib/clipboard"
import { saveBlobAsFile } from "@/lib/download"
import { useI18n } from "@/lib/i18n"
import toast from "@/lib/toast"
import { cn } from "@/lib/utils"
import { MonacoEditorWrapper } from "./monaco-editor-wrapper"
import {
  consumeNucleiTemplateImport,
  NucleiTemplateDraft,
  NucleiTemplateWorkspacePayload,
  parseNucleiTemplateIdFromYaml,
  renderNucleiTemplateYaml,
  syncNucleiTemplateDraftFromYaml,
  updateNucleiTemplateYamlMetadata,
} from "./nuclei-template"

interface NucleiTemplateWorkspaceProps {
  initialPayload: NucleiTemplateWorkspacePayload | null
}

interface NucleiTemplateDebugEvent {
  type: "info" | "request" | "response" | "match" | "miss" | "error" | "success"
  message: string
  request?: string
  response?: string
  data?: unknown
}

interface NucleiTemplateDebugResult {
  target: string
  matched: boolean
  status: "matched" | "not_matched" | "failed"
  events: NucleiTemplateDebugEvent[]
  results?: unknown[]
}

type NucleiTemplateWorkspaceView = "template" | "debug"

interface NucleiTemplateWorkspaceTab {
  id: string
  name: string
  draft: NucleiTemplateDraft
  yamlValue: string
  responseValue: string
  target: string
  activeView: NucleiTemplateWorkspaceView
  sourceLabel: string
  source: "manual" | "repeater" | "poc"
  debugResult: NucleiTemplateDebugResult | null
  lastStatus?: NucleiTemplateDebugResult["status"] | ""
}

interface PersistedPoCDebugTab {
  id: string
  name: string
  templateId?: string
  yaml: string
  target: string
  source: "manual" | "repeater" | "poc"
  sourceName: string
  activeView: NucleiTemplateWorkspaceView
  lastStatus?: NucleiTemplateDebugResult["status"] | ""
  isActive?: boolean
  sortOrder?: number
}

function encodeBase64(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ""
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte)
  })
  return btoa(binary)
}

function normalizeRootUrl(value: string): string {
  const target = value.trim()
  if (!target) return ""
  try {
    const parsed = new URL(target)
    return `${parsed.protocol}//${parsed.host}/`
  } catch {
    return target
  }
}

function createDefaultNucleiTemplateDraft(): NucleiTemplateDraft {
  return {
    id: "manual-poc-debug-template",
    name: "Manual PoC Debug Template",
    author: "hephaestus",
    severity: "info",
    description: "Manual PoC debug template",
    impact: "",
    remediation: "",
    rawRequest: [
      "GET / HTTP/1.1",
      "Host: {{Hostname}}",
    ].join("\n"),
    matchersCondition: "and",
    matchers: [{
      type: "status",
      status: [200],
      summary: "status = 200",
    }],
  }
}

function createWorkspaceTab(
  payload: NucleiTemplateWorkspacePayload | null,
  index: number,
  fallbackLabel: string
): NucleiTemplateWorkspaceTab {
  const draft = payload?.draft || createDefaultNucleiTemplateDraft()
  const sourceLabel = payload?.sourceTabName || `${fallbackLabel}-${index}`
  const source = payload?.source === "repeater" || payload?.source === "poc" ? payload.source : "manual"

  return {
    id: `poc-debug-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
    name: sourceLabel,
    draft,
    yamlValue: payload?.yaml || renderNucleiTemplateYaml(draft),
    responseValue: payload?.response || "",
    target: normalizeRootUrl(payload?.target || ""),
    activeView: payload?.initialView === "debug" ? "debug" : "template",
    sourceLabel,
    source,
    debugResult: null,
    lastStatus: "",
  }
}

function createManualWorkspaceTab(index: number, fallbackLabel: string): NucleiTemplateWorkspaceTab {
  const draft = createDefaultNucleiTemplateDraft()
  const sourceLabel = `${fallbackLabel}-${index}`
  return {
    id: `poc-debug-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`,
    name: sourceLabel,
    draft,
    yamlValue: renderNucleiTemplateYaml(draft),
    responseValue: "",
    target: "",
    activeView: "template",
    sourceLabel,
    source: "manual",
    debugResult: null,
    lastStatus: "",
  }
}

function createWorkspaceTabFromPersisted(tab: PersistedPoCDebugTab): NucleiTemplateWorkspaceTab {
  const draft = syncNucleiTemplateDraftFromYaml(createDefaultNucleiTemplateDraft(), tab.yaml)
  const sourceLabel = tab.sourceName || tab.name || draft.id
  return {
    id: tab.id,
    name: tab.name || sourceLabel,
    draft,
    yamlValue: tab.yaml,
    responseValue: "",
    target: normalizeRootUrl(tab.target || ""),
    activeView: tab.activeView === "debug" ? "debug" : "template",
    sourceLabel,
    source: tab.source || "manual",
    debugResult: null,
    lastStatus: tab.lastStatus || "",
  }
}

export function NucleiTemplateWorkspace({ initialPayload }: NucleiTemplateWorkspaceProps) {
  const { t, language } = useI18n()
  const initialPayloadRef = useRef(initialPayload)
  const manualSourceRef = useRef(t.repeater.nucleiTemplate.manualSource)
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [workspaceTabs, setWorkspaceTabs] = useState<NucleiTemplateWorkspaceTab[]>([])
  const [activeTabId, setActiveTabId] = useState("")
  const [isLoaded, setIsLoaded] = useState(false)
  const [isPolishing, setIsPolishing] = useState(false)
  const [isDebugging, setIsDebugging] = useState(false)
  const activeTab = useMemo(
    () => workspaceTabs.find((tab) => tab.id === activeTabId) || workspaceTabs[0],
    [activeTabId, workspaceTabs]
  )
  const draft = activeTab?.draft
  const yamlValue = activeTab?.yamlValue || ""
  const responseValue = activeTab?.responseValue || ""
  const target = activeTab?.target || ""
  const activeView = activeTab?.activeView || "template"
  const debugResult = activeTab?.debugResult || null
  const sourceLabel = activeTab?.sourceLabel || t.repeater.nucleiTemplate.manualSource

  const serializeTabsForPersistence = useCallback((tabs: NucleiTemplateWorkspaceTab[], activeId: string): PersistedPoCDebugTab[] => (
    tabs.map((tab, index) => ({
      id: tab.id,
      name: tab.name,
      templateId: parseNucleiTemplateIdFromYaml(tab.yamlValue) || tab.draft.id || "",
      yaml: tab.yamlValue,
      target: tab.target,
      source: tab.source,
      sourceName: tab.sourceLabel,
      activeView: tab.activeView,
      lastStatus: tab.lastStatus || tab.debugResult?.status || "",
      isActive: tab.id === activeId,
      sortOrder: index,
    }))
  ), [])

  const persistTabs = useCallback(async (tabs: NucleiTemplateWorkspaceTab[], activeId: string) => {
    try {
      await fetchWithAuth("/api/v1/poc-debug/tabs", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(serializeTabsForPersistence(tabs, activeId)),
      })
    } catch (error) {
      console.error("Failed to save PoC debug tabs:", error)
    }
  }, [serializeTabsForPersistence])

  useEffect(() => {
    let cancelled = false
    const loadTabs = async () => {
      try {
        const response = await fetchWithAuth("/api/v1/poc-debug/tabs")
        const data = await response.json().catch(() => ({}))
        if (!response.ok || !data.success) {
          throw new Error(data.error || "load poc debug tabs failed")
        }

        if (cancelled) return
        const persistedTabs: PersistedPoCDebugTab[] = Array.isArray(data.data) ? data.data : []
        const loadedTabs = persistedTabs.map(createWorkspaceTabFromPersisted)
        const payload = initialPayloadRef.current
        let incomingTabId = ""

        if (payload && consumeNucleiTemplateImport(payload)) {
          const incomingTab = createWorkspaceTab(payload, loadedTabs.length + 1, manualSourceRef.current)
          loadedTabs.push(incomingTab)
          incomingTabId = incomingTab.id
        }

        setWorkspaceTabs(loadedTabs)
        setActiveTabId(
          incomingTabId || persistedTabs.find((tab) => tab.isActive)?.id || loadedTabs[0]?.id || ""
        )
      } catch (error) {
        console.error("Failed to load PoC debug tabs:", error)
        const payload = initialPayloadRef.current
        if (!cancelled && payload && consumeNucleiTemplateImport(payload)) {
          const incomingTab = createWorkspaceTab(payload, 1, manualSourceRef.current)
          setWorkspaceTabs([incomingTab])
          setActiveTabId(incomingTab.id)
        }
      } finally {
        if (!cancelled) {
          setIsLoaded(true)
        }
      }
    }

    void loadTabs()
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!isLoaded) return
    if (saveTimeoutRef.current) {
      clearTimeout(saveTimeoutRef.current)
    }
    saveTimeoutRef.current = setTimeout(() => {
      void persistTabs(workspaceTabs, activeTabId)
    }, 500)

    return () => {
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current)
      }
    }
  }, [activeTabId, isLoaded, persistTabs, workspaceTabs])

  useEffect(() => {
    if (!isLoaded || !initialPayload || !consumeNucleiTemplateImport(initialPayload)) {
      return
    }

    const nextTab = createWorkspaceTab(initialPayload, workspaceTabs.length + 1, t.repeater.nucleiTemplate.manualSource)
    setWorkspaceTabs((currentTabs) => [...currentTabs, nextTab])
    setActiveTabId(nextTab.id)
  }, [initialPayload, isLoaded, t.repeater.nucleiTemplate.manualSource, workspaceTabs.length])

  const updateActiveTab = useCallback((updates: Partial<NucleiTemplateWorkspaceTab>) => {
    setWorkspaceTabs((currentTabs) => (
      currentTabs.map((tab) => (tab.id === activeTabId ? { ...tab, ...updates } : tab))
    ))
  }, [activeTabId])

  const addWorkspaceTab = useCallback(() => {
    const nextTab = createManualWorkspaceTab(workspaceTabs.length + 1, t.repeater.nucleiTemplate.manualSource)
    setWorkspaceTabs((currentTabs) => [...currentTabs, nextTab])
    setActiveTabId(nextTab.id)
  }, [t.repeater.nucleiTemplate.manualSource, workspaceTabs.length])

  const closeWorkspaceTab = useCallback((tabId: string, event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation()
    if (workspaceTabs.length <= 1) {
      setWorkspaceTabs([])
      setActiveTabId("")
      void persistTabs([], "")
      return
    }

    const nextTabs = workspaceTabs.filter((tab) => tab.id !== tabId)
    const nextActiveId = tabId === activeTabId
      ? nextTabs[Math.min(workspaceTabs.findIndex((tab) => tab.id === tabId), nextTabs.length - 1)]?.id || nextTabs[0]?.id || ""
      : activeTabId
    setWorkspaceTabs(nextTabs)
    if (tabId === activeTabId) {
      setActiveTabId(nextActiveId)
    }
    void persistTabs(nextTabs, nextActiveId)
  }, [activeTabId, persistTabs, workspaceTabs])

  const handleYamlChange = useCallback((yaml: string) => {
    if (!draft) return
    updateActiveTab({
      yamlValue: yaml,
      draft: syncNucleiTemplateDraftFromYaml(draft, yaml),
    })
  }, [draft, updateActiveTab])

  const handlePolish = useCallback(async () => {
    if (!draft) return

    setIsPolishing(true)
    try {
      const response = await fetchWithAuth("/api/v1/repeater/nuclei-template/polish", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          id: draft.id,
          name: draft.name,
          severity: draft.severity,
          description: draft.description,
          impact: draft.impact,
          remediation: draft.remediation,
          rawRequest: draft.rawRequest,
          response: responseValue,
          templateYaml: yamlValue,
          locale: language,
          matchers: draft.matchers,
        }),
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success || !data.data) {
        throw new Error(data.error || t.repeater.nucleiTemplate.polishFailed)
      }

      const nextDraft: NucleiTemplateDraft = {
        ...draft,
        id: data.data.id || draft.id,
        name: data.data.name || draft.name,
        severity: data.data.severity || draft.severity,
        description: data.data.description || draft.description,
        impact: data.data.impact || draft.impact,
        remediation: data.data.remediation || draft.remediation,
      }
      updateActiveTab({
        draft: nextDraft,
        yamlValue: updateNucleiTemplateYamlMetadata(yamlValue, nextDraft),
      })
      toast.success({ title: t.common.success, message: t.repeater.nucleiTemplate.polishSuccess })
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.repeater.nucleiTemplate.polishFailed}: ${error?.message || ""}`,
      })
    } finally {
      setIsPolishing(false)
    }
  }, [draft, language, responseValue, t.common.error, t.common.success, t.repeater.nucleiTemplate.polishFailed, t.repeater.nucleiTemplate.polishSuccess, updateActiveTab, yamlValue])

  const handleCopy = useCallback(async () => {
    if (!draft || !yamlValue.trim()) return

    const success = await copyToClipboard(yamlValue)
    if (!success) {
      toast.error({ title: t.common.error, message: t.common.copyFailed })
      return
    }

    toast.success({ title: t.common.success, message: t.repeater.nucleiTemplate.copySuccess })
  }, [draft, t.common.copyFailed, t.common.error, t.common.success, t.repeater.nucleiTemplate.copySuccess, yamlValue])

  const handleSave = useCallback(async () => {
    if (!draft || !yamlValue.trim()) return

    try {
      const result = await saveBlobAsFile(
        new Blob([yamlValue], { type: "application/x-yaml;charset=utf-8" }),
        `${parseNucleiTemplateIdFromYaml(yamlValue) || draft.id || "generated-http-template"}.yaml`
      )

      if (result === "cancelled") {
        toast.warning({ title: t.common.warning, message: t.repeater.nucleiTemplate.saveCancelled })
        return
      }

      toast.success({ title: t.common.success, message: t.repeater.nucleiTemplate.saveSuccess })
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.repeater.nucleiTemplate.saveFailed}: ${error?.message || ""}`,
      })
    }
  }, [draft, t.common.error, t.common.success, t.common.warning, t.repeater.nucleiTemplate.saveCancelled, t.repeater.nucleiTemplate.saveFailed, t.repeater.nucleiTemplate.saveSuccess, yamlValue])

  const handleDebug = useCallback(async () => {
    const normalizedTarget = normalizeRootUrl(target)
    updateActiveTab({ activeView: "debug" })
    if (!draft || !yamlValue.trim() || !normalizedTarget) {
      toast.warning({ title: t.common.warning, message: t.repeater.nucleiTemplate.debugMissingTarget })
      return
    }

    updateActiveTab({
      activeView: "debug",
      target: normalizedTarget,
      debugResult: null,
    })
    setIsDebugging(true)
    try {
      const response = await fetchWithAuth("/api/v1/repeater/nuclei-template/debug", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          target: normalizedTarget,
          templateBase64: encodeBase64(yamlValue),
        }),
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success || !data.data) {
        throw new Error(data.error || t.repeater.nucleiTemplate.debugFailed)
      }

      updateActiveTab({ debugResult: data.data, lastStatus: data.data.status })
      toast.success({
        title: t.common.success,
        message: data.data.matched
          ? t.repeater.nucleiTemplate.debugMatched
          : t.repeater.nucleiTemplate.debugNotMatched,
      })
    } catch (error: any) {
      updateActiveTab({
        debugResult: {
          target: normalizedTarget,
          matched: false,
          status: "failed",
          events: [{
            type: "error",
            message: error?.message || t.repeater.nucleiTemplate.debugFailed,
          }],
        },
        lastStatus: "failed",
      })
      toast.error({
        title: t.common.error,
        message: `${t.repeater.nucleiTemplate.debugFailed}: ${error?.message || ""}`,
      })
    } finally {
      setIsDebugging(false)
    }
  }, [draft, target, t.common.error, t.common.success, t.common.warning, t.repeater.nucleiTemplate.debugFailed, t.repeater.nucleiTemplate.debugMatched, t.repeater.nucleiTemplate.debugMissingTarget, t.repeater.nucleiTemplate.debugNotMatched, updateActiveTab, yamlValue])

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      <div className="flex shrink-0 items-center gap-1 overflow-x-auto border-b border-border bg-card/50 px-2">
        <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto py-1.5">
          {workspaceTabs.map((tab) => (
            <div
              key={tab.id}
              role="button"
              tabIndex={0}
              onClick={() => setActiveTabId(tab.id)}
              onKeyDown={(event) => {
                if (event.key === "Enter" || event.key === " ") {
                  setActiveTabId(tab.id)
                }
              }}
              className={cn(
                "group flex h-8 min-w-[116px] max-w-[220px] items-center gap-2 rounded-md px-3 text-left text-sm transition-colors",
                tab.id === activeTabId
                  ? "bg-muted text-foreground"
                  : "text-muted-foreground hover:bg-muted/50 hover:text-foreground"
              )}
            >
              <span className="min-w-0 flex-1 truncate">{tab.name}</span>
              {(tab.debugResult?.status || tab.lastStatus) === "matched" ? (
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-green-500" />
              ) : (tab.debugResult?.status || tab.lastStatus) === "failed" ? (
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-destructive" />
              ) : null}
              <button
                type="button"
                onClick={(event) => closeWorkspaceTab(tab.id, event)}
                className="rounded p-0.5 opacity-100 md:opacity-0 transition-opacity hover:bg-muted md:group-hover:opacity-100"
                aria-label={t.repeater.nucleiTemplate.closeTab}
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          ))}
        </div>
        <Button variant="ghost" size="icon" className="h-7 w-7 shrink-0" onClick={addWorkspaceTab} title={t.repeater.nucleiTemplate.newTab}>
          <Plus className="h-4 w-4" />
        </Button>
      </div>
      {!isLoaded ? (
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">
          {t.common.loading}
        </div>
      ) : !activeTab ? (
        <div className="flex flex-1 items-center justify-center p-6">
          <div className="flex max-w-md flex-col items-center text-center">
            <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-lg border border-border bg-card">
              <Bug className="h-5 w-5 text-muted-foreground" />
            </div>
            <h2 className="text-base font-semibold">{t.repeater.nucleiTemplate.emptyTitle}</h2>
            <p className="mt-2 text-sm text-muted-foreground">{t.repeater.nucleiTemplate.emptyDescription}</p>
            <Button className="mt-4" onClick={addWorkspaceTab}>
              <Plus className="mr-2 h-4 w-4" />
              {t.repeater.nucleiTemplate.newTab}
            </Button>
          </div>
        </div>
      ) : (
        <>
      <div className="shrink-0 border-b bg-card/70">
        <div className="relative flex min-h-[72px] flex-col items-stretch gap-2 px-3 py-3 md:flex-row md:items-center md:gap-3 md:px-5">
          <div className="w-full shrink-0 rounded-lg border border-border bg-background/80 p-1 shadow-lg backdrop-blur md:w-auto">
            <Tabs value={activeView} onValueChange={(value) => updateActiveTab({ activeView: value as NucleiTemplateWorkspaceView })} className="w-full shrink-0 md:w-auto">
              <TabsList className="h-8 w-full bg-transparent p-0 md:w-auto">
                <TabsTrigger value="template" className="h-8 flex-1 gap-1.5 px-4 text-xs md:flex-none">
                  <FileCode2 className="h-3.5 w-3.5" />
                  {t.repeater.nucleiTemplate.templateTab}
                </TabsTrigger>
                <TabsTrigger value="debug" className="h-8 flex-1 gap-1.5 px-4 text-xs md:flex-none">
                  <Bug className="h-3.5 w-3.5" />
                  {t.repeater.nucleiTemplate.debugTab}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
          <div className="flex min-w-0 w-full flex-col gap-2 md:min-w-[280px] md:flex-1 md:flex-row md:items-center md:gap-3">
            <div className="flex min-w-0 w-full flex-col items-stretch gap-2 md:flex-1 md:flex-row md:items-center md:gap-3">
              <Input
                value={target}
                onChange={(event) => updateActiveTab({ target: event.target.value })}
                onBlur={() => updateActiveTab({ target: normalizeRootUrl(target) })}
                aria-label={t.repeater.nucleiTemplate.debugTarget}
                placeholder="https://example.com"
                className="h-11 min-w-0 w-full flex-1 font-mono md:h-9"
              />
              <Button
                size="sm"
                onClick={() => void handleDebug()}
                disabled={isDebugging}
                className="h-11 w-full shrink-0 whitespace-nowrap px-4 md:h-9 md:w-auto"
              >
                {isDebugging ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Play className="mr-2 h-4 w-4" />}
                {t.repeater.nucleiTemplate.validate}
              </Button>
            </div>
            {debugResult ? (
              <span
                className={cn(
                  "inline-flex h-9 w-full items-center justify-center rounded-md border px-3 text-xs font-medium md:w-auto md:justify-start",
                  debugResult.status === "matched"
                    ? "border-green-500/40 bg-green-500/10 text-green-500"
                    : debugResult.status === "failed"
                      ? "border-destructive/40 bg-destructive/10 text-destructive"
                      : "border-muted-foreground/30 bg-background text-muted-foreground"
                )}
              >
                {debugResult.status === "matched" ? (
                  <CheckCircle2 className="mr-1.5 h-3.5 w-3.5" />
                ) : (
                  <XCircle className="mr-1.5 h-3.5 w-3.5" />
                )}
                {debugResult.status === "matched"
                  ? t.repeater.nucleiTemplate.matched
                  : debugResult.status === "failed"
                    ? t.repeater.nucleiTemplate.failed
                    : t.repeater.nucleiTemplate.notMatched}
              </span>
            ) : null}
          </div>
        </div>
      </div>

      {activeView === "template" ? (
        <div className="min-h-0 flex-1 overflow-hidden p-3 md:p-5">
          <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-lg border border-border bg-card/40">
            <div className="flex min-h-12 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-2 md:px-4">
              <div className="min-w-0">
                <h2 className="text-sm font-semibold">{t.repeater.nucleiTemplate.preview}</h2>
                <p className="truncate text-xs text-muted-foreground">{draft.id || "generated-http-template"}.yaml</p>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void handlePolish()}
                  disabled={isPolishing}
                  className={cn(
                    "h-9 min-w-[112px] transition-colors",
                    isPolishing && "border-primary/30 bg-primary/5 text-primary shadow-sm hover:bg-primary/5",
                  )}
                >
                  {isPolishing ? (
                    <span className="mr-2 flex h-4 w-4 items-center justify-center gap-0.5" aria-hidden="true">
                      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-current" />
                      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-current [animation-delay:150ms]" />
                      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-current [animation-delay:300ms]" />
                    </span>
                  ) : (
                    <Sparkles className="mr-2 h-4 w-4" />
                  )}
                  <span aria-live="polite">{isPolishing ? t.repeater.nucleiTemplate.polishing : t.repeater.nucleiTemplate.polish}</span>
                </Button>
                <Button variant="outline" size="sm" onClick={() => void handleCopy()} className="h-9">
                  <Copy className="mr-2 h-4 w-4" />
                  {t.repeater.nucleiTemplate.copy}
                </Button>
                <Button variant="outline" size="sm" onClick={() => void handleSave()} className="h-9">
                  <Save className="mr-2 h-4 w-4" />
                  {t.repeater.nucleiTemplate.save}
                </Button>
              </div>
            </div>
            <div className="heph-repeater-monaco min-h-0 flex-1 overflow-hidden">
              <MonacoEditorWrapper
                language="yaml"
                value={yamlValue}
                onChange={handleYamlChange}
                height="100%"
                options={{
                  wordWrap: "on",
                  lineNumbers: "on",
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                }}
              />
            </div>
          </section>
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden p-5">
          <section className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card/40">
            <div className="flex min-h-12 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-2 md:px-4">
              <div className="flex items-center gap-2">
                <Bug className="h-4 w-4 text-muted-foreground" />
                <h2 className="text-sm font-semibold">{t.repeater.nucleiTemplate.debugConsole}</h2>
              </div>
              {debugResult ? (
                <span className="rounded border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground">
                  {debugResult.events.length} {t.repeater.nucleiTemplate.events}
                </span>
              ) : null}
            </div>
            <div className="min-h-0 flex-1 overflow-auto bg-background font-mono text-xs">
              {debugResult?.events?.length ? (
                <div className="divide-y divide-border/80">
                  {debugResult.events.map((event, index) => (
                    <div key={`${event.type}-${index}`} className="grid grid-cols-[96px_minmax(0,1fr)] gap-3 px-4 py-3">
                      <div
                        className={cn(
                          "text-xs uppercase",
                          event.type === "match" || event.type === "success"
                            ? "text-green-500"
                            : event.type === "error"
                              ? "text-destructive"
                              : event.type === "miss"
                                ? "text-muted-foreground"
                                : "text-blue-500"
                        )}
                      >
                        {event.type}
                      </div>
                      <div className="min-w-0">
                        <div className="break-words text-foreground">{event.message}</div>
                        {event.request ? <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap rounded border border-border bg-muted/30 p-3 text-foreground">{event.request}</pre> : null}
                        {event.response ? <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap rounded border border-border bg-muted/30 p-3 text-foreground">{event.response}</pre> : null}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="flex h-full items-center justify-center text-muted-foreground">
                  {t.repeater.nucleiTemplate.debugEmpty}
                </div>
              )}
            </div>
          </section>
        </div>
      )}
        </>
      )}
    </div>
  )
}
