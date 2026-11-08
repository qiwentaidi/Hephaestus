'use client'

import { type FormEvent, useCallback, useEffect, useState } from 'react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { FormItem } from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { Plus, RefreshCw, Server, Trash2 } from 'lucide-react'
import SystemToolsPanel, { type SystemTool } from './SystemToolsPanel'

type MCPTransport = 'stdio' | 'http' | 'sse'

type MCPTool = {
  name: string
  description?: string
  inputSchema?: unknown
}

type MCPServer = {
  id: string
  name: string
  transport: MCPTransport
  command?: string
  args?: string[]
  workDir?: string
  hasEnv: boolean
  disabledTools: string[]
  url: string
  enabled: boolean
  insecure: boolean
  hasHeaders: boolean
  tools: MCPTool[]
  checkedAt?: string
  lastError?: string
}

type ServerForm = {
  name: string
  transport: MCPTransport
  command: string
  args: string
  workDir: string
  env: string
  url: string
  headers: string
  insecure: boolean
}

type ExternalCallCounts = Record<string, Record<string, number>>
type ConnectionsPanelProps = {
  callCounts?: Record<string, number>
  externalCallCounts?: ExternalCallCounts
  onRefreshCounts: () => void
}

function normalizeServer(server: MCPServer): MCPServer {
  return {
    ...server,
    tools: Array.isArray(server.tools) ? server.tools : [],
    disabledTools: Array.isArray(server.disabledTools) ? server.disabledTools : [],
  }
}

const emptyForm = (): ServerForm => ({
  name: '',
  transport: 'stdio',
  command: '',
  args: '',
  workDir: '',
  env: '',
  url: '',
  headers: '',
  insecure: false,
})

function parseStringMap(value: string): Record<string, string> | undefined {
  if (!value.trim()) return undefined
  if (value.trimStart().startsWith('{')) {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('object')
    const entries = Object.entries(parsed)
    if (entries.some(([key, item]) => !key.trim() || typeof item !== 'string')) throw new Error('object')
    return Object.fromEntries(entries) as Record<string, string>
  }
  const result: Record<string, string> = {}
  for (const line of value.split(/\r?\n/)) {
    if (!line.trim()) continue
    const separator = line.indexOf('=')
    if (separator <= 0) throw new Error('object')
    const key = line.slice(0, separator).trim()
    if (!key) throw new Error('object')
    result[key] = line.slice(separator + 1).trim()
  }
  return result
}

function parseArgs(value: string): string[] {
  if (!value.trim()) return []
  return value.trim().split(/\s+/)
}

async function readPayload<T>(response: Response): Promise<T> {
  const payload = await response.json().catch(() => ({}))
  if (!response.ok || !payload.success) throw new Error(payload.error || `HTTP ${response.status}`)
  return payload.data as T
}

export default function ExternalMcpConnectionsPanel({ callCounts, externalCallCounts, onRefreshCounts }: ConnectionsPanelProps) {
  const { t } = useI18n()
  const c = t.mcpConnections
  const [servers, setServers] = useState<MCPServer[]>([])
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [formOpen, setFormOpen] = useState(false)
  const [formServerId, setFormServerId] = useState<string | null>(null)
  const [form, setForm] = useState<ServerForm>(emptyForm)
  const [formError, setFormError] = useState('')

  const refresh = useCallback(async () => {
    try {
      const serverResponse = await fetchWithAuth('/api/v1/mcp/external-servers')
      const nextServers = (await readPayload<MCPServer[]>(serverResponse)).map(normalizeServer)
      setServers(nextServers)
      setError('')
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : c.error)
    } finally {
      setLoading(false)
    }
  }, [c.error])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const beginAdd = () => {
    setFormServerId(null)
    setForm(emptyForm())
    setFormError('')
    setFormOpen(true)
  }

  const beginEdit = (server: MCPServer) => {
    setFormServerId(server.id)
    setForm({
      name: server.name,
      transport: server.transport,
      command: server.command || '',
      args: JSON.stringify(server.args || [], null, 2),
      workDir: server.workDir || '',
      env: '',
      url: server.url || '',
      headers: '',
      insecure: server.insecure || false,
    })
    setFormError('')
    setFormOpen(true)
  }

  const buildPayload = (enabled = false, disabledTools: string[] = []) => {
    let args: string[] = []
    let env: Record<string, string> | undefined
    let headers: Record<string, string> | undefined
    if (form.transport === 'stdio') {
      args = parseArgs(form.args)
      try {
        env = parseStringMap(form.env)
      } catch {
        throw new Error(c.invalidEnv)
      }
    } else {
      try {
        headers = parseStringMap(form.headers)
      } catch {
        throw new Error(c.invalidHeaders)
      }
    }
    return {
      name: form.name.trim(),
      transport: form.transport,
      enabled,
      insecure: form.transport !== 'stdio' && form.insecure,
      disabledTools,
      ...(form.transport === 'stdio'
        ? { command: form.command.trim(), args, workDir: form.workDir.trim(), ...(env !== undefined ? { env } : {}) }
        : { url: form.url.trim(), ...(headers !== undefined ? { headers } : {}) }),
    }
  }

  const submitForm = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setBusy('form')
    setFormError('')
    try {
      const currentServer = servers.find(server => server.id === formServerId)
      const payload = buildPayload(currentServer?.enabled || false, currentServer?.disabledTools || [])
      if (!payload.name) throw new Error(c.nameRequired)
      if (form.transport === 'stdio' && !form.command.trim()) throw new Error(c.commandRequired)
      if (form.transport !== 'stdio' && !form.url.trim()) throw new Error(c.urlRequired)
      const route = formServerId
        ? `/api/v1/mcp/external-servers/${encodeURIComponent(formServerId)}`
        : '/api/v1/mcp/external-servers'
      const saved = await readPayload<MCPServer>(await fetchWithAuth(route, {
        method: formServerId ? 'PUT' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      }))
      setFormServerId(saved.id)
      const discovered = await readPayload<MCPServer>(await fetchWithAuth(
        `/api/v1/mcp/external-servers/${encodeURIComponent(saved.id)}/discover`,
        { method: 'POST' },
      ))
      setServers(items => [normalizeServer(discovered), ...items.filter(item => item.id !== discovered.id)])
      toast.success(c.discovered)
      setFormOpen(false)
      setFormServerId(null)
      onRefreshCounts()
      void refresh()
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : c.error
      setFormError(message)
      void refresh()
    } finally {
      setBusy(null)
    }
  }

  const updateServer = async (server: MCPServer, updates: { enabled?: boolean; disabledTools?: string[] }): Promise<boolean> => {
    setBusy(server.id)
    try {
      const payload = {
        name: server.name,
        transport: server.transport,
        enabled: updates.enabled ?? server.enabled,
        insecure: server.insecure,
        disabledTools: updates.disabledTools ?? server.disabledTools,
        ...(server.transport === 'stdio'
          ? { command: server.command || '', args: server.args || [], workDir: server.workDir || '' }
          : { url: server.url }),
      }
      const updated = await readPayload<MCPServer>(await fetchWithAuth(
        `/api/v1/mcp/external-servers/${encodeURIComponent(server.id)}`,
        { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) },
      ))
      setServers(items => items.map(item => item.id === updated.id ? normalizeServer(updated) : item))
      onRefreshCounts()
      return true
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : c.error)
      return false
    } finally {
      setBusy(null)
    }
  }

  const discover = async (server: MCPServer) => {
    setBusy(server.id)
    try {
      const discovered = await readPayload<MCPServer>(await fetchWithAuth(
        `/api/v1/mcp/external-servers/${encodeURIComponent(server.id)}/discover`,
        { method: 'POST' },
      ))
      setServers(items => items.map(item => item.id === discovered.id ? normalizeServer(discovered) : item))
      toast.success(c.discovered)
      onRefreshCounts()
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : c.error)
      void refresh()
    } finally {
      setBusy(null)
    }
  }

  const remove = async (server: MCPServer) => {
    if (!window.confirm(c.confirmDelete)) return
    setBusy(server.id)
    try {
      await readPayload(await fetchWithAuth(`/api/v1/mcp/external-servers/${encodeURIComponent(server.id)}`, { method: 'DELETE' }))
      setServers(items => items.filter(item => item.id !== server.id))
      toast.success(c.deleted)
      onRefreshCounts()
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : c.error)
    } finally {
      setBusy(null)
    }
  }

  const externalTools = servers.flatMap(server => server.tools.map(tool => {
    const toolEnabled = !server.disabledTools.includes(tool.name)
    return {
      id: `${server.id}:${tool.name}`,
      name: tool.name,
      command: '',
      description: tool.description || '',
      details: tool.description || '',
      parameters: tool.inputSchema,
      source: 'external',
      sourceName: server.name,
      enabled: server.enabled && toolEnabled,
      toolEnabled,
      serviceEnabled: server.enabled,
      serverId: server.id,
      externalToolName: tool.name,
      callCount: externalCallCounts ? externalCallCounts[server.id]?.[tool.name] || 0 : undefined,
      externalBusy: busy === server.id,
    }
  }))

  const toggleExternalTool = (tool: { serverId?: string; externalToolName?: string }, enabled: boolean) => {
    const server = servers.find(item => item.id === tool.serverId)
    if (!server || !tool.externalToolName) return
    const disabledTools = enabled
      ? server.disabledTools.filter(name => name !== tool.externalToolName)
      : [...new Set([...server.disabledTools, tool.externalToolName])]
    void updateServer(server, { disabledTools })
  }

  const batchToggleExternalTools = async (tools: SystemTool[], enabled: boolean) => {
    const byServer = new Map<string, SystemTool[]>()
    for (const tool of tools) {
      if (!tool.serverId || !tool.externalToolName) continue
      const group = byServer.get(tool.serverId) || []
      group.push(tool)
      byServer.set(tool.serverId, group)
    }
    let failedTools = 0
    for (const [serverId, group] of byServer) {
      const server = servers.find(item => item.id === serverId)
      if (!server || !server.enabled) {
        failedTools += group.length
        continue
      }
      const names = new Set(group.map(tool => tool.externalToolName!))
      const disabledTools = enabled
        ? server.disabledTools.filter(name => !names.has(name))
        : [...new Set([...server.disabledTools, ...names])]
      if (!await updateServer(server, { disabledTools })) failedTools += group.length
    }
    return { failedTools }
  }

  const setField = <K extends keyof ServerForm,>(key: K, value: ServerForm[K]) => setForm(current => ({ ...current, [key]: value }))

  return (
    <div className="flex min-h-full w-full min-w-0 flex-col gap-4 p-4 lg:p-5">
      <section className="flex flex-col gap-3">
        <div className="space-y-1">
          <h2 className="text-base font-semibold">{c.remoteTitle}</h2>
          <p className="max-w-3xl text-sm text-muted-foreground">{c.connectionHint}</p>
        </div>
        {error && <p role="alert" className="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2 2xl:grid-cols-3">
          <button type="button" onClick={beginAdd} className="flex min-h-36 items-center justify-center gap-2 rounded-lg border border-dashed border-border bg-card/40 text-sm text-muted-foreground transition-colors hover:bg-muted/30 hover:text-foreground">
            <Plus className="h-4 w-4" />{c.add}
          </button>
          {loading ? <p className="self-center text-sm text-muted-foreground" role="status">{c.loading}</p> : servers.map(server => {
            const transportLabel = server.transport === 'stdio' ? c.localTransport : server.transport === 'sse' ? c.sseTransport : c.httpTransport
            const busyThis = busy === server.id
            return (
              <article key={server.id} className="min-w-0 rounded-lg border border-border bg-card p-4">
                <header className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="mt-0.5 rounded-md border border-border bg-muted/40 p-2 text-muted-foreground"><Server className="h-4 w-4" /></span>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2"><h3 className="break-words font-medium">{server.name}</h3><span className="rounded-md border border-border bg-muted/30 px-2 py-0.5 text-xs text-muted-foreground">{transportLabel}</span></div>
                      <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{server.transport === 'stdio' ? [server.command, ...(server.args || [])].join(' ') : server.url}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="whitespace-nowrap text-xs text-muted-foreground">{server.enabled ? c.enabled : c.disabled}</span>
                    <Switch aria-label={`${c.enable}: ${server.name}`} checked={server.enabled} disabled={busy !== null || (!server.enabled && !server.checkedAt)} onCheckedChange={enabled => void updateServer(server, { enabled })} />
                  </div>
                </header>
                <p className="mt-3 text-xs text-muted-foreground">{server.checkedAt ? `${c.checked} · ${new Date(server.checkedAt).toLocaleString()}` : c.unchecked} · {server.tools.length} {c.toolCount}</p>
                {server.lastError && <p role="alert" className="mt-3 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">{server.lastError}</p>}
                <div className="mt-3 flex flex-wrap gap-2 border-t border-border pt-3">
                  <Button size="sm" variant="outline" disabled={busyThis} onClick={() => void discover(server)}><RefreshCw className={`mr-2 h-3.5 w-3.5 ${busyThis ? 'animate-spin' : ''}`} />{c.discover}</Button>
                  <Button size="sm" variant="outline" disabled={busyThis} onClick={() => beginEdit(server)}>{c.edit}</Button>
                  <Button size="sm" variant="outline" disabled={busyThis} onClick={() => void remove(server)}><Trash2 className="mr-2 h-3.5 w-3.5" />{c.remove}</Button>
                </div>
              </article>
            )
          })}
        </div>
        {!loading && servers.length === 0 && !error && <p className="text-sm text-muted-foreground">{c.empty} · {c.emptyHint}</p>}
      </section>

      <SystemToolsPanel
        callCounts={callCounts}
        externalTools={externalTools}
        onRefreshCounts={onRefreshCounts}
        onToggleExternalTool={toggleExternalTool}
        onBatchToggleExternalTools={batchToggleExternalTools}
      />

      <Sheet open={formOpen} onOpenChange={open => { if (!open && busy !== 'form') { setFormOpen(false); setFormError('') } }}>
        <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-2xl">
          <SheetHeader className="border-b border-border pb-5">
            <SheetTitle>{formServerId ? c.edit : c.add}</SheetTitle>
            <SheetDescription>{c.connectionHint}</SheetDescription>
          </SheetHeader>
          <form className="space-y-5 p-5 pt-6" onSubmit={event => void submitForm(event)}>
            <FormItem className="gap-2.5">
              <Label htmlFor="mcp-name">{c.name}</Label>
              <Input id="mcp-name" required maxLength={100} value={form.name} onChange={event => setField('name', event.target.value)} />
            </FormItem>
            <FormItem className="gap-2.5">
              <Label htmlFor="mcp-transport">{c.transport}</Label>
              <Select value={form.transport} onValueChange={value => setField('transport', value as MCPTransport)}>
                <SelectTrigger id="mcp-transport"><SelectValue /></SelectTrigger><SelectContent>
                  <SelectItem value="stdio">{c.localTransport}</SelectItem>
                  <SelectItem value="http">{c.httpTransport}</SelectItem>
                  <SelectItem value="sse">{c.sseTransport}</SelectItem>
                </SelectContent>
              </Select>
            </FormItem>
            {form.transport === 'stdio' ? <>
              <FormItem className="gap-2.5">
                <Label htmlFor="mcp-command">{c.command}</Label>
                <Input id="mcp-command" required value={form.command} onChange={event => setField('command', event.target.value)} placeholder="npx" />
              </FormItem>
              <FormItem className="gap-2.5">
                <Label htmlFor="mcp-args">{c.args}</Label>
                <Input id="mcp-args" value={form.args} onChange={event => setField('args', event.target.value)} placeholder="-y @modelcontextprotocol/server-filesystem /data" />
              </FormItem>
              <FormItem className="gap-2.5">
                <Label htmlFor="mcp-workdir">{c.workDir}</Label>
                <Input id="mcp-workdir" value={form.workDir} onChange={event => setField('workDir', event.target.value)} />
              </FormItem>
              <FormItem className="gap-2.5">
                <Label htmlFor="mcp-env">{c.env}</Label>
                <Textarea id="mcp-env" rows={4} spellCheck={false} value={form.env} onChange={event => setField('env', event.target.value)} placeholder={formServerId && servers.find(item => item.id === formServerId)?.hasEnv ? c.savedValuesPlaceholder : 'API_KEY=xxxx\nFOO=bar'} />
                <p className="text-xs leading-relaxed text-muted-foreground">{c.localHint} {c.envHint}</p>
              </FormItem>
            </> : <>
              <FormItem className="gap-2.5">
                <Label htmlFor="mcp-url">{c.url}</Label>
                <Input id="mcp-url" required type="url" value={form.url} onChange={event => setField('url', event.target.value)} placeholder="https://example.com/mcp" />
              </FormItem>
              <FormItem className="flex flex-row items-center gap-2.5">
                <Checkbox id="mcp-insecure-tls" checked={form.insecure} onCheckedChange={checked => setField('insecure', checked === true)} />
                <Label htmlFor="mcp-insecure-tls" className="font-normal">{c.insecureTLS}</Label>
              </FormItem>
              <FormItem className="gap-2.5">
                <Label htmlFor="mcp-headers">{c.headers}</Label>
                <Textarea id="mcp-headers" rows={5} spellCheck={false} value={form.headers} onChange={event => setField('headers', event.target.value)} placeholder={formServerId && servers.find(item => item.id === formServerId)?.hasHeaders ? c.savedValuesPlaceholder : 'Authorization=Bearer ...'} />
                <p className="text-xs leading-relaxed text-muted-foreground">{c.headersHint}</p>
              </FormItem>
            </>}
            {formError && <p role="alert" className="rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{formError}</p>}
            <div className="flex justify-end gap-2 border-t border-border pt-4">
              <Button type="button" variant="outline" disabled={busy === 'form'} onClick={() => setFormOpen(false)}>{c.cancel}</Button>
              <Button type="submit" disabled={busy === 'form'}>{busy === 'form' ? c.discovering : c.saveAndDiscover}</Button>
            </div>
          </form>
        </SheetContent>
      </Sheet>
    </div>
  )
}
