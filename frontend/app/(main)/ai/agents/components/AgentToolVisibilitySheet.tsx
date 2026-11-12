'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'
import { Loader2, Search, ShieldCheck } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  listAgentTools,
  saveAgentTools,
  type AgentTool,
  type AgentToolRole,
  type AgentToolSource,
  type AgentToolVisibilityConfig,
} from '@/lib/api/agent-tools'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

const ROLE_ORDER: AgentToolRole[] = ['planner', 'worker', 'controller', 'pentest']
const SOURCES: AgentToolSource[] = ['platform', 'system', 'mcp']

export default function AgentToolVisibilitySheet({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useI18n()
  const copy = t.agentPrompts
  const [config, setConfig] = useState<AgentToolVisibilityConfig | null>(null)
  const [role, setRole] = useState<AgentToolRole>('planner')
  const [disabled, setDisabled] = useState<Partial<Record<AgentToolRole, string[]>>>({})
  const [query, setQuery] = useState('')
  const [source, setSource] = useState<'all' | AgentToolSource>('all')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [failed, setFailed] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setFailed(false)
    try {
      const value = await listAgentTools()
      setConfig(value)
      setDisabled({ ...value.visibility })
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (open) void load()
  }, [open, load])

  const tools = config?.tools ?? []
  const disabledSet = useMemo(() => new Set(disabled[role] ?? []), [disabled, role])
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase()
    return tools.filter((tool) => {
      if (source !== 'all' && tool.source !== source) return false
      if (!needle) return true
      return [tool.name, tool.title, tool.description, tool.serverName].filter(Boolean).join(' ').toLowerCase().includes(needle)
    })
  }, [tools, query, source])
  const grouped = useMemo(() => {
    return SOURCES.map((item) => ({ source: item, tools: filtered.filter((tool) => tool.source === item) })).filter((item) => item.tools.length > 0)
  }, [filtered])
  const mcpServerGroups = useMemo(() => {
    const map = new Map<string, AgentTool[]>()
    for (const tool of filtered) {
      if (tool.source !== 'mcp') continue
      const key = tool.serverName || tool.serverId || copy.toolSourceMCP
      const items = map.get(key)
      if (items) items.push(tool)
      else map.set(key, [tool])
    }
    return [...map.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [filtered, copy.toolSourceMCP])

  const toggle = (tool: AgentTool) => {
    setDisabled((current) => {
      const next = new Set(current[role] ?? [])
      if (next.has(tool.name)) next.delete(tool.name)
      else next.add(tool.name)
      return { ...current, [role]: [...next] }
    })
  }

  const toggleMany = (items: AgentTool[], on: boolean) => {
    setDisabled((current) => {
      const next = new Set(current[role] ?? [])
      for (const tool of items) {
        if (!tool.enabled) continue
        if (on) next.delete(tool.name)
        else next.add(tool.name)
      }
      return { ...current, [role]: [...next] }
    })
  }

  const setAll = (on: boolean) => {
    setDisabled((current) => ({
      ...current,
      [role]: on ? [] : tools.map((tool) => tool.name),
    }))
  }

  const save = async () => {
    if (!config || saving) return
    setSaving(true)
    try {
      const visibility = {} as Record<AgentToolRole, string[]>
      for (const item of ROLE_ORDER) visibility[item] = disabled[item] ?? []
      await saveAgentTools(visibility)
      toast.success(copy.toolSaved)
      onOpenChange(false)
    } catch {
      toast.error(copy.toolSaveFailed)
    } finally {
      setSaving(false)
    }
  }

  const sourceLabel = (value: AgentToolSource) => value === 'platform' ? copy.toolSourcePlatform : value === 'system' ? copy.toolSourceSystem : copy.toolSourceMCP

  const renderTool = (tool: AgentTool) => {
    const checked = !disabledSet.has(tool.name)
    return (
      <label
        key={tool.name}
        className={cn(
          'flex cursor-pointer items-start gap-2 rounded-md border border-border/60 px-2.5 py-2',
          !tool.enabled && 'cursor-not-allowed opacity-55',
          checked && tool.enabled && 'border-primary/35 bg-primary/5',
        )}
      >
        <Checkbox
          className="mt-0.5"
          checked={checked}
          disabled={!tool.enabled}
          onCheckedChange={() => toggle(tool)}
        />
        <span className="min-w-0 flex-1">
          <span className="flex flex-wrap items-center gap-1.5">
            <span className="truncate text-xs font-medium">{tool.title || tool.name}</span>
            <Badge variant="outline" className="h-4 rounded px-1 text-[9px] font-normal">{sourceLabel(tool.source)}</Badge>
            {!tool.enabled ? <Badge variant="outline" className="h-4 rounded border-amber-500/40 px-1 text-[9px] font-normal text-amber-700 dark:text-amber-300">{copy.toolDisabledGlobally}</Badge> : null}
          </span>
          <span className="mt-0.5 block truncate font-mono text-[10px] text-muted-foreground">{tool.name}</span>
          {tool.description ? <span className="mt-1 block line-clamp-2 text-[11px] leading-4 text-muted-foreground">{tool.description}</span> : null}
        </span>
      </label>
    )
  }

  return (
    <Sheet open={open} onOpenChange={(next) => { if (!saving) onOpenChange(next) }}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-2xl">
        <SheetHeader className="border-b border-border/70 px-5 py-4">
          <SheetTitle className="flex items-center gap-2">
            <ShieldCheck className="h-4 w-4" />
            {copy.toolTitle}
          </SheetTitle>
          <SheetDescription>{copy.toolDescription}</SheetDescription>
        </SheetHeader>

        <div className="flex flex-wrap items-center gap-1.5 border-b border-border/70 px-5 py-3">
          {ROLE_ORDER.map((item) => {
            const hidden = new Set(disabled[item] ?? [])
            const available = tools.filter((tool) => tool.enabled && !hidden.has(tool.name)).length
            return (
              <Button
                key={item}
                type="button"
                size="sm"
                variant={role === item ? 'default' : 'outline'}
                className="h-7 px-2.5 text-[11px]"
                onClick={() => setRole(item)}
              >
                {copy.roles[item].name}
                <span className={cn('ml-1.5 tabular-nums text-[10px]', role === item ? 'opacity-80' : 'text-muted-foreground')}>
                  {available}
                </span>
              </Button>
            )
          })}
          <span className="ml-auto text-[11px] text-muted-foreground">{copy.toolHiddenCount.replace('{n}', String(disabledSet.size))}</span>
        </div>

        <div className="flex flex-wrap items-center gap-2 px-5 py-3">
          <div className="relative min-w-[200px] flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground/70" />
            <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={copy.toolSearch} className="h-8 rounded-lg pl-8 text-xs" />
          </div>
          <div className="flex items-center gap-1">
            <Button type="button" size="sm" variant={source === 'all' ? 'default' : 'outline'} className="h-7 px-2 text-[11px]" onClick={() => setSource('all')}>{copy.toolAllSources}</Button>
            {SOURCES.map((item) => (
              <Button key={item} type="button" size="sm" variant={source === item ? 'default' : 'outline'} className="h-7 px-2 text-[11px]" onClick={() => setSource(item)}>{sourceLabel(item)}</Button>
            ))}
          </div>
          <div className="ml-auto flex items-center gap-1">
            <Button type="button" size="sm" variant="ghost" className="h-7 px-2 text-[11px]" onClick={() => setAll(true)}>{copy.toolSelectAll}</Button>
            <Button type="button" size="sm" variant="ghost" className="h-7 px-2 text-[11px]" onClick={() => setAll(false)}>{copy.toolClear}</Button>
          </div>
        </div>

        <ScrollArea className="min-h-0 flex-1 px-5">
          {loading ? (
            <div className="flex min-h-[240px] items-center justify-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              {copy.toolLoading}
            </div>
          ) : failed ? (
            <div className="flex min-h-[240px] flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
              <span>{copy.toolLoadFailed}</span>
              <Button size="sm" variant="outline" onClick={() => void load()}>{copy.retry}</Button>
            </div>
          ) : !filtered.length ? (
            <div className="flex min-h-[200px] items-center justify-center text-sm text-muted-foreground">{copy.toolEmpty}</div>
          ) : (
            <div className="space-y-4 pb-4">
              {grouped.map((group) => (
                <section key={group.source}>
                  <div className="sticky top-0 z-10 flex items-center gap-2 bg-background/95 py-2 text-[11px] font-medium text-muted-foreground backdrop-blur">
                    <span>{sourceLabel(group.source)}</span>
                    <span className="tabular-nums opacity-70">{group.tools.length}</span>
                  </div>
                  {group.source === 'mcp' ? <div className="space-y-3">
                    {mcpServerGroups.map(([serverName, serverTools]) => {
                      const selectable = serverTools.filter((tool) => tool.enabled)
                      const enabledCount = selectable.filter((tool) => !disabledSet.has(tool.name)).length
                      const allOn = selectable.length > 0 && enabledCount === selectable.length
                      const state: boolean | 'indeterminate' = allOn ? true : enabledCount > 0 ? 'indeterminate' : false
                      return <div key={serverName} className="space-y-1">
                        <div className="flex items-center gap-2 rounded-md border border-border/50 bg-muted/25 px-2.5 py-1.5">
                          <Checkbox
                            checked={state}
                            disabled={!selectable.length}
                            aria-label={`${copy.toolSelectAll}: ${serverName}`}
                            onCheckedChange={(value) => toggleMany(serverTools, value === true)}
                          />
                          <span className="truncate text-[11px] font-medium">{serverName}</span>
                          <span className="ml-auto tabular-nums text-[10px] text-muted-foreground">{enabledCount}/{serverTools.length}</span>
                        </div>
                        <div className="space-y-1 pl-1">{serverTools.map(renderTool)}</div>
                      </div>
                    })}
                  </div> : <div className="space-y-1">{group.tools.map(renderTool)}</div>}
                </section>
              ))}
            </div>
          )}
        </ScrollArea>

        <SheetFooter className="border-t border-border/70 px-5 py-3 sm:flex-row sm:items-center">
          <p className="mr-auto max-w-md text-[11px] leading-4 text-muted-foreground">{copy.toolHint}</p>
          <Button variant="outline" size="sm" disabled={saving} onClick={() => onOpenChange(false)}>{copy.cancel}</Button>
          <Button size="sm" disabled={loading || failed || saving || !tools.length} onClick={() => void save()}>
            {saving ? <><Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />{copy.toolSaving}</> : copy.toolSave}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
