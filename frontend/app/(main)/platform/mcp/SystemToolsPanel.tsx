'use client'

import mobileTable from '@/components/ui/mobile-record-table.module.css'

import { useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import ToolCallCount from './ToolCallCount'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

export type SystemTool = {
  id?: string
  name: string
  command: string
  description: string
  details: string
  parameters: unknown
  source: string
  sourceName?: string
  enabled: boolean
  toolEnabled?: boolean
  serviceEnabled?: boolean
  serverId?: string
  externalToolName?: string
  callCount?: number
  externalBusy?: boolean
}

type SystemToolsPanelProps = {
  callCounts?: Record<string, number>
  externalTools?: SystemTool[]
  onRefreshCounts: () => void
  onToggleExternalTool?: (tool: SystemTool, enabled: boolean) => void
  onBatchToggleExternalTools?: (tools: SystemTool[], enabled: boolean) => Promise<{ failedTools: number }>
}

type Parameter = {
  name: string
  type?: string
  description?: string
  required?: boolean | string[]
  isRequired?: boolean
  default?: unknown
  enum?: unknown[]
  options?: unknown[]
  properties?: Record<string, unknown>
  items?: unknown
}

function parametersOf(value: unknown): Parameter[] {
  if (Array.isArray(value)) return value.filter((item): item is Parameter => !!item && typeof item === 'object' && typeof item.name === 'string').map(item => ({ ...item, isRequired: item.required === true }))
  if (!value || typeof value !== 'object') return []
  const schema = value as { properties?: Record<string, unknown>; required?: string[] }
  return Object.entries(schema.properties || {}).map(([name, property]) => ({ ...(property && typeof property === 'object' ? property : {}), name, isRequired: Array.isArray(schema.required) && schema.required.includes(name) }))
}

function ParameterList({ value }: { value: unknown }) {
  const { t, language } = useI18n()
  const c = t.mcpConnections
  const parameters = parametersOf(value)
  const types: Record<string, string> = language === 'zh' ? { string: '文本', integer: '整数', int: '整数', number: '数值', float: '数值', boolean: '布尔值', bool: '布尔值', object: '对象', array: '数组' } : {}
  if (!parameters.length) return <p className="text-sm text-muted-foreground">{c.noParameters}</p>
  return <dl className="divide-y divide-border">
    {parameters.map(parameter => <div key={parameter.name} className="py-4 first:pt-0">
      <dt className="flex flex-wrap items-center gap-2 text-sm">
        <code className="break-all font-medium">{parameter.name}</code>
        {parameter.type && <span className="text-xs text-muted-foreground">{types[parameter.type] || parameter.type}</span>}
        <span className="rounded border border-border bg-muted/40 px-1.5 py-0.5 text-xs text-muted-foreground">{parameter.isRequired ? c.requiredLabel : c.optionalLabel}</span>
      </dt>
      <dd className="mt-2 space-y-2 text-sm">
        {parameter.description && <p className="whitespace-pre-wrap break-words leading-relaxed">{parameter.description}</p>}
        {parameter.default !== undefined && <p className="break-all text-xs text-muted-foreground">{c.defaultLabel}：<code>{JSON.stringify(parameter.default)}</code></p>}
        {!!(parameter.enum || parameter.options)?.length && <p className="break-all text-xs text-muted-foreground">{c.optionsLabel}：<code>{(parameter.enum || parameter.options)?.map(option => String(option)).join(' / ')}</code></p>}
        {parameter.properties && <div className="border-l border-border pl-3 pt-2"><ParameterList value={parameter} /></div>}
        {!!parameter.items && parametersOf(parameter.items).length > 0 && <div className="border-l border-border pl-3 pt-2"><ParameterList value={parameter.items} /></div>}
      </dd>
    </div>)}
  </dl>
}

export default function SystemToolsPanel({ callCounts, externalTools = [], onRefreshCounts, onToggleExternalTool, onBatchToggleExternalTools }: SystemToolsPanelProps) {
  const { t } = useI18n()
  const c = t.mcpConnections
  const labels: Record<string, { title: string; summary: string }> = c.capabilityLabels
  const [tools, setTools] = useState<SystemTool[]>([])
  const [query, setQuery] = useState('')
  const [source, setSource] = useState('all')
  const [state, setState] = useState('all')
  const [selectedName, setSelectedName] = useState<string | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(() => new Set())
  const [batchBusy, setBatchBusy] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [revision, setRevision] = useState(0)
  const allTools = [...tools, ...externalTools]

  useEffect(() => {
    let active = true
    void fetchWithAuth('/api/v1/mcp/system-tools')
      .then(async response => {
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const payload = await response.json()
        if (!payload.success) throw new Error(payload.error)
        if (active) { setTools(payload.data); setError('') }
      })
      .catch(e => { if (active) setError(e instanceof Error ? e.message : 'Failed to load system tools') })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [revision])

  const toggle = async (tool: SystemTool, enabled: boolean) => {
    if (tool.source === 'external') {
      onToggleExternalTool?.(tool, enabled)
      return
    }
    setBusy(tool.name)
    try {
      const response = await fetchWithAuth(`/api/v1/mcp/system-tools/${encodeURIComponent(tool.name)}`, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled }) })
      const payload = await response.json()
      if (!response.ok || !payload.success) throw new Error(payload.error || c.error)
      setTools(items => items.map(item => item.name === tool.name ? { ...item, enabled } : item))
    } catch (error) { toast.error(error instanceof Error ? error.message : c.error) }
    finally { setBusy(null) }
  }

  const needle = query.trim().toLowerCase()
  const filtered = allTools.filter(tool => (source === 'all' || tool.source === source)
    && (state === 'all' || tool.enabled === (state === 'enabled'))
    && `${labels[tool.name]?.title || ''} ${labels[tool.name]?.summary || ''} ${tool.name} ${tool.command} ${tool.description} ${tool.sourceName || ''}`.toLowerCase().includes(needle))
  const selected = allTools.find(tool => (tool.id || tool.name) === selectedName)
  const title = (tool: SystemTool) => tool.source === 'external' ? tool.name : labels[tool.name]?.title || tool.name
  const summary = (tool: SystemTool) => tool.source === 'external' ? tool.description : labels[tool.name]?.summary || tool.description
  const sourceLabel = (tool: SystemTool) => tool.source === 'platform' ? c.platformSource : tool.source === 'external' ? `${c.externalSource} · ${tool.sourceName || ''}` : c.systemSource
  const callCount = (tool: SystemTool) => tool.source === 'external' ? tool.callCount : callCounts ? (callCounts[tool.name] || 0) : undefined
  const toolId = (tool: SystemTool) => tool.id || tool.name
  const switchChecked = (tool: SystemTool) => tool.source === 'external' ? tool.toolEnabled === true : tool.enabled
  const switchStatus = (tool: SystemTool) => tool.source === 'external' && !tool.serviceEnabled ? c.serviceDisabled : tool.enabled ? c.enabled : c.disabled
  const canBulkToggle = (tool: SystemTool) => tool.source !== 'external' || (tool.serviceEnabled && !tool.externalBusy)
  const selectedTools = allTools.filter(tool => selectedIds.has(toolId(tool)))
  const selectableFiltered = filtered.filter(canBulkToggle)
  const filteredSelectedCount = selectableFiltered.filter(tool => selectedIds.has(toolId(tool))).length
  const allFilteredSelected = selectableFiltered.length > 0 && filteredSelectedCount === selectableFiltered.length
  const hasUnavailableSelection = selectedTools.some(tool => tool.source === 'external' && (!tool.serviceEnabled || tool.externalBusy))

  const setFilteredSelection = (checked: boolean) => {
    setSelectedIds(current => {
      const next = new Set(current)
      for (const tool of filtered) {
        if (!canBulkToggle(tool)) continue
        if (checked) next.add(toolId(tool))
        else next.delete(toolId(tool))
      }
      return next
    })
  }

  const applyBatchToggle = async (enabled: boolean) => {
    if (!selectedTools.length || hasUnavailableSelection) return
    const systemTools = selectedTools.filter(tool => tool.source !== 'external')
    const external = selectedTools.filter(tool => tool.source === 'external')
    let updated = 0
    let failed = 0
    setBatchBusy(true)
    try {
      if (systemTools.length) {
        try {
          const response = await fetchWithAuth('/api/v1/mcp/system-tools/bulk', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ names: systemTools.map(tool => tool.name), enabled }),
          })
          const payload = await response.json()
          if (!response.ok || !payload.success) throw new Error(payload.error || c.error)
          const names = new Set(systemTools.map(tool => tool.name))
          setTools(items => items.map(tool => names.has(tool.name) ? { ...tool, enabled } : tool))
          updated += systemTools.length
        } catch (error) {
          failed += systemTools.length
          toast.error(error instanceof Error ? error.message : c.error)
        }
      }
      if (external.length && onBatchToggleExternalTools) {
        try {
          const result = await onBatchToggleExternalTools(external, enabled)
          failed += result.failedTools
          updated += external.length - result.failedTools
        } catch (error) {
          failed += external.length
          toast.error(error instanceof Error ? error.message : c.error)
        }
      }
      if (updated) toast.success(`${updated} ${c.batchUpdated}`)
      if (failed) toast.error(`${failed} ${c.batchFailed}`)
      if (!failed) {
        setSelectedIds(new Set())
      }
    } finally {
      setBatchBusy(false)
    }
  }

  return (
    <section className="flex w-full min-w-0 flex-1 flex-col gap-3">
      <h2 className="text-base font-semibold">{c.title}</h2>
      <div className="flex flex-wrap items-center gap-2">
        <Input className="min-w-48 flex-1 basis-64" aria-label={c.systemSearch} placeholder={c.systemSearch} value={query} onChange={event => setQuery(event.target.value)} />
        <Select value={source} onValueChange={setSource}><SelectTrigger className="w-40" aria-label={c.sourceFilter}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">{c.allSources}</SelectItem><SelectItem value="platform">{c.platformSource}</SelectItem><SelectItem value="system">{c.systemSource}</SelectItem><SelectItem value="external">{c.externalSource}</SelectItem></SelectContent></Select>
        <Select value={state} onValueChange={setState}><SelectTrigger className="w-32" aria-label={c.stateFilter}><SelectValue /></SelectTrigger><SelectContent><SelectItem value="all">{c.allStates}</SelectItem><SelectItem value="enabled">{c.enabled}</SelectItem><SelectItem value="disabled">{c.disabled}</SelectItem></SelectContent></Select>
        <span className="whitespace-nowrap px-2 text-xs tabular-nums text-muted-foreground">{c.currentItems} {filtered.length} / {allTools.length} · {c.enabledCount} {allTools.filter(tool => tool.enabled).length}</span>
        <Button variant="outline" disabled={loading} onClick={() => { setLoading(true); setRevision(value => value + 1); onRefreshCounts() }}>{c.refresh}</Button>
      </div>
      {selectedTools.length > 0 && <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/20 p-3">
        <span className="px-1 text-sm text-muted-foreground">{selectedTools.length} {c.selectedCount}</span>
        <Button size="sm" variant="ghost" disabled={batchBusy || selectableFiltered.length === 0} onClick={() => setFilteredSelection(true)}>{c.selectCurrent}</Button>
        <Button size="sm" variant="ghost" disabled={batchBusy || selectedIds.size === 0} onClick={() => setSelectedIds(new Set())}>{c.clearSelection}</Button>
        <Button size="sm" variant="outline" disabled={batchBusy || selectedTools.length === 0 || hasUnavailableSelection || busy !== null} onClick={() => void applyBatchToggle(true)}>{c.batchEnable}</Button>
        <Button size="sm" variant="outline" disabled={batchBusy || selectedTools.length === 0 || hasUnavailableSelection || busy !== null} onClick={() => void applyBatchToggle(false)}>{c.batchDisable}</Button>
      </div>}
      {error && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
      <label className="flex min-h-11 items-center gap-3 text-sm md:hidden"><Checkbox aria-label={c.selectCurrent} checked={allFilteredSelected ? true : filteredSelectedCount > 0 ? 'indeterminate' : false} disabled={batchBusy || selectableFiltered.length === 0} onCheckedChange={checked => setFilteredSelection(checked === true)} />{c.selectCurrent}</label>
      <div className="w-full overflow-x-auto rounded-lg border border-border bg-card">
        <table className={`w-full min-w-[790px] table-fixed text-left text-sm ${mobileTable.table}`}>
          <caption className="sr-only">{c.title}</caption>
          <colgroup><col className="w-12" /><col className="w-[28%]" /><col /><col className="w-[110px]" /><col className="w-[142px]" /><col className="w-[80px]" /></colgroup>
          <thead className="border-b border-border bg-muted/40 text-xs text-muted-foreground"><tr>
            <th scope="col" className="px-3 py-3"><Checkbox aria-label={c.selectCurrent} checked={allFilteredSelected ? true : filteredSelectedCount > 0 ? 'indeterminate' : false} disabled={batchBusy || selectableFiltered.length === 0} onCheckedChange={checked => setFilteredSelection(checked === true)} /></th>
            <th scope="col" className="px-4 py-3 font-medium">{c.toolName}</th><th scope="col" className="px-4 py-3 font-medium">{c.purpose}</th><th scope="col" className="px-4 py-3 font-medium">{c.sourceLabel}</th><th scope="col" className="px-4 py-3 font-medium">{c.stateLabel}</th><th scope="col" className="px-4 py-3 text-right font-medium">{c.detailsAction}</th>
          </tr></thead>
          <tbody className="divide-y divide-border">
            {loading && tools.length === 0 && externalTools.length === 0 ? <tr><td colSpan={6} className="p-8 text-center text-muted-foreground" role="status">{c.loading}</td></tr> : filtered.map(tool => <tr key={toolId(tool)} className="transition-colors hover:bg-muted/30">
              <td className="px-3 py-3"><Checkbox aria-label={`${c.selectTool}: ${title(tool)}`} checked={selectedIds.has(toolId(tool))} disabled={batchBusy || !canBulkToggle(tool)} onCheckedChange={checked => setSelectedIds(current => { const next = new Set(current); if (checked) next.add(toolId(tool)); else next.delete(toolId(tool)); return next })} /></td>
              <th scope="row" className="px-4 py-3 font-normal"><div className="flex flex-wrap items-center gap-2"><p className="font-medium">{title(tool)}</p><ToolCallCount count={callCount(tool)} /></div><p className="mt-1 break-all font-mono text-[11px] leading-4 text-muted-foreground">{tool.name}</p></th>
              <td data-label={c.purpose} className="px-4 py-3 leading-relaxed text-muted-foreground">{summary(tool)}</td>
              <td data-label={c.sourceLabel} className="px-4 py-3"><span className="inline-flex rounded-md border border-border bg-muted/30 px-2 py-1 text-xs text-muted-foreground">{sourceLabel(tool)}</span></td>
              <td data-label={c.stateLabel} className="px-4 py-3"><div className="flex items-center gap-2"><Switch aria-label={`${c.enable}: ${title(tool)}`} checked={switchChecked(tool)} disabled={batchBusy || busy !== null || (tool.source === 'external' && (!tool.serviceEnabled || tool.externalBusy))} onCheckedChange={enabled => void toggle(tool, enabled)} /><span className="whitespace-nowrap text-xs text-muted-foreground">{switchStatus(tool)}</span></div></td>
              <td className="px-3 py-3 text-right"><Button size="sm" variant="ghost" aria-label={`${c.detailsAction}: ${title(tool)}`} onClick={() => setSelectedName(toolId(tool))}>{c.detailsAction}</Button></td>
            </tr>)}
            {!loading && !error && filtered.length === 0 && <tr><td colSpan={6} className="p-8 text-center"><p className="text-muted-foreground">{c.systemNoMatches}</p><Button variant="ghost" className="mt-2" onClick={() => { setQuery(''); setSource('all'); setState('all') }}>{c.clearFilters}</Button></td></tr>}
          </tbody>
        </table>
      </div>
      <Sheet open={!!selected} onOpenChange={open => { if (!open) setSelectedName(null) }}>
        <SheetContent className="w-full gap-0 sm:max-w-xl">
          <SheetHeader className="border-b border-border p-5 pr-12"><SheetTitle>{selected && title(selected)}</SheetTitle><SheetDescription className="break-all font-mono text-xs">{selected?.name}</SheetDescription></SheetHeader>
          {selected && <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-5">
            <div className="flex items-center justify-between gap-3"><span className="rounded-md border border-border bg-muted/30 px-2 py-1 text-xs text-muted-foreground">{sourceLabel(selected)}</span><div className="flex items-center gap-3"><span className="text-sm text-muted-foreground">{switchStatus(selected)}</span><Switch aria-label={`${c.enable}: ${title(selected)}`} checked={switchChecked(selected)} disabled={batchBusy || busy !== null || (selected.source === 'external' && (!selected.serviceEnabled || selected.externalBusy))} onCheckedChange={enabled => void toggle(selected, enabled)} /></div></div>
            <p className="whitespace-pre-wrap break-words text-sm leading-7">{selected.source === 'external' ? selected.description : selected.details || selected.description}</p>
            {selected.command && <div className="space-y-2"><h3 className="text-sm font-medium">{c.systemCommand}</h3><code className="block break-all rounded-md bg-muted/40 p-3 text-xs">{selected.command}</code></div>}
            <section className="space-y-4 border-t border-border pt-5"><h3 className="text-sm font-semibold">{c.parametersLabel}</h3><ParameterList value={selected.parameters} /></section>
          </div>}
        </SheetContent>
      </Sheet>
    </section>
  )
}
