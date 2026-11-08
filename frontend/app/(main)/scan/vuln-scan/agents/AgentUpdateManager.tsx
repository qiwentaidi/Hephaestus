'use client'

import mobile from './agents-mobile.module.css'

import { useCallback, useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import toast from 'react-hot-toast'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Progress } from '@/components/ui/progress'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'

export interface UpdateNode {
  agentId: string
  displayName: string
  lastSeenHost: string
  agentVersion?: string
  localWorker?: boolean
  isAlive: boolean
  updateProtocol?: number
  updateOS?: string
  updateArch?: string
  aiRunning?: number
  scanRunning?: number
}

export interface AgentUpdate {
  agentId: string
  requestId: string
  targetVersion: string
  status: string
  progress: number
  downloadedBytes: number
  error: string
  cancelRequested: boolean
}

const activeStates = ['queued', 'waiting', 'downloading', 'verifying', 'restarting', 'confirming', 'rolling_back']
export const updateActive = (job?: AgentUpdate) => !!job && activeStates.includes(job.status)
const cancellable = (job: AgentUpdate) => ['queued', 'waiting', 'downloading', 'verifying'].includes(job.status)
export function updateEligible(node: UpdateNode, target: string, job?: AgentUpdate) {
  return !!target && !node.localWorker && node.isAlive && node.updateProtocol === 1
    && node.agentVersion !== target && !updateActive(job)
}

export function useAgentUpdates() {
  const [updates, setUpdates] = useState<Record<string, AgentUpdate>>({})
  const [targetVersion, setTargetVersion] = useState('')
  const [error, setError] = useState(false)
  const reload = useCallback(async () => {
    try {
      const response = await fetchWithAuth('/api/v1/webscan/agent-updates')
      const data = await response.json()
      if (!response.ok || !data.success) throw new Error()
      setTargetVersion(data.data.targetVersion)
      setUpdates(Object.fromEntries((data.data.updates as AgentUpdate[]).map(job => [job.agentId, job])))
      setError(false)
    } catch { setError(true) }
  }, [])
  useEffect(() => {
    const initial = setTimeout(() => void reload(), 0)
    // Match the system update check cadence: once per hour.
    const timer = setInterval(reload, 60 * 60 * 1000)
    return () => { clearTimeout(initial); clearInterval(timer) }
  }, [reload])
  return { updates, targetVersion, error, reload }
}

export function AgentUpdateStatus({ node, target, job }: { node: UpdateNode; target: string; job?: AgentUpdate }) {
  const { t } = useI18n()
  const text = t.vulnScan.agents.updates
  let label: string
  if (node.localWorker) label = text.local
  else if (updateActive(job)) {
    label = job!.cancelRequested ? text.cancelling : text.states[job!.status as keyof typeof text.states] || job!.status
    if (job!.status === 'waiting') label += ` · ${(node.aiRunning || 0) + (node.scanRunning || 0)} ${text.tasks}`
  } else if (job && job.targetVersion === target && ['failed', 'rolled_back', 'cancelled'].includes(job.status)) {
    label = text.states[job.status as keyof typeof text.states]
  } else if (!node.isAlive) label = text.offline
  else if (node.updateProtocol !== 1) label = text.bootstrap
  else label = node.agentVersion === target ? text.matched : text.available
  const failure = job?.targetVersion === target && ['failed', 'rolled_back'].includes(job.status)
  return <Badge variant="outline" className={failure ? 'border-destructive/40 text-destructive' : updateActive(job) ? 'border-amber-500/40 text-amber-600 dark:text-amber-400' : 'text-muted-foreground'}>{label}</Badge>
}

export function AgentUpdateManager({ nodes, state, open, onOpenChange, initialAgent }: {
  nodes: UpdateNode[]
  state: ReturnType<typeof useAgentUpdates>
  open: boolean
  onOpenChange: (open: boolean) => void
  initialAgent: string | null
}) {
  const { t } = useI18n()
  const text = t.vulnScan.agents.updates
  const { updates, targetVersion, error, reload } = state
  const [selected, setSelected] = useState<string[]>([])
  const [busy, setBusy] = useState(false)
  // Seed selection only when opening; regular progress polling preserves choices.
  const [previousOpen, setPreviousOpen] = useState(false)
  if (open !== previousOpen) {
    setPreviousOpen(open)
    if (open) setSelected(initialAgent ? [initialAgent] : [])
  }
  const eligible = nodes.filter(node => updateEligible(node, targetVersion, updates[node.agentId]))
  const chosen = eligible.filter(node => selected.includes(node.agentId)).map(node => node.agentId)
  const action = async (path: string, body: object) => {
    setBusy(true)
    try {
      const response = await fetchWithAuth(path, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
      const data = await response.json()
      if (!response.ok || !data.success) throw new Error(data.error || text.failed)
      toast.success(text.submitted)
      await reload()
      setSelected([])
    } catch (error) { toast.error(error instanceof Error ? error.message : text.failed) }
    finally { setBusy(false) }
  }
  const request = (ids: string[]) => action('/api/v1/webscan/agent-updates', { agentIds: ids })

  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className={`${mobile.dialog} max-h-[90dvh] overflow-y-auto sm:max-w-4xl`}>
      <DialogHeader>
        <DialogTitle>{text.title}</DialogTitle>
        <DialogDescription>{text.description}</DialogDescription>
      </DialogHeader>
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border bg-muted/20 p-3">
        <span className="text-sm">{text.target}: <span className="font-mono">{targetVersion || '—'}</span></span>
        <Button variant="outline" size="sm" onClick={() => void reload()}><RefreshCw className="size-4" />{text.refresh}</Button>
      </div>
      {error && <p className="text-sm text-destructive" role="alert">{text.loadFailed}</p>}
      <div className="grid grid-cols-1 gap-2 sm:flex sm:flex-wrap sm:items-center sm:justify-between">
        <Button variant="ghost" size="sm" disabled={busy || !eligible.length} onClick={() => setSelected(eligible.map(node => node.agentId))}>{text.selectAll}</Button>
        <Button variant="outline" size="sm" disabled={busy || !chosen.length} onClick={() => void request(chosen)}>{text.batch.replace('{count}', String(chosen.length))}</Button>
      </div>
      <div className="divide-y rounded-lg border">
        {!nodes.length && <p className="p-5 text-sm text-muted-foreground">{text.empty}</p>}
        {nodes.map(node => {
          const job = updates[node.agentId]
          const allowed = updateEligible(node, targetVersion, job)
          const pending = updateActive(job)
          return <div key={node.agentId} role="group" aria-label={node.displayName || node.lastSeenHost || node.agentId} className="grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] md:items-center">
            <div className="flex min-w-0 items-start gap-3">
              <label className="flex size-11 shrink-0 items-center justify-center md:size-auto"><Checkbox disabled={!allowed || busy} checked={allowed && selected.includes(node.agentId)}
                aria-label={text.select.replace('{name}', node.displayName || node.lastSeenHost || node.agentId)}
                onCheckedChange={checked => setSelected(ids => checked ? [...ids.filter(id => id !== node.agentId), node.agentId] : ids.filter(id => id !== node.agentId))} /></label>
              <div className="min-w-0 space-y-1">
                <div className="break-all text-sm font-medium md:truncate" title={node.displayName || node.lastSeenHost}>{node.displayName || node.lastSeenHost || node.agentId}</div>
                <div className="break-words font-mono text-xs text-muted-foreground">{node.agentVersion || '—'} → {pending ? job!.targetVersion : targetVersion || '—'}</div>
                {!!node.updateOS && <div className="text-xs text-muted-foreground">{node.updateOS}/{node.updateArch}</div>}
              </div>
            </div>
            <div className="min-w-0 space-y-2">
              <AgentUpdateStatus node={node} target={targetVersion} job={job} />
              {pending && job!.status !== 'queued' && job!.status !== 'waiting' && <Progress value={job!.progress} className="h-1.5 [&_[data-slot=progress-indicator]]:bg-foreground/80" />}
              {pending && job!.status === 'downloading' && <p className="text-xs text-muted-foreground">{Math.floor((job!.downloadedBytes || 0) / 1048576)} MiB {text.downloaded}</p>}
              {job?.error && <p className="break-words text-xs text-destructive">{job.error}</p>}
            </div>
            <div className="flex justify-end">
              {pending && cancellable(job!) ? <Button variant="outline" size="sm" disabled={busy || job!.cancelRequested}
                onClick={() => void action(`/api/v1/webscan/agent-updates/${encodeURIComponent(node.agentId)}/cancel`, { requestId: job!.requestId })}>{text.cancel}</Button>
                : <Button variant="outline" size="sm" disabled={busy || !allowed} onClick={() => void request([node.agentId])}>{text.update}</Button>}
            </div>
          </div>
        })}
      </div>
      <p className="text-xs leading-relaxed text-muted-foreground">{text.bootstrapHint}</p>
    </DialogContent>
  </Dialog>
}
