'use client'

import mobile from './agents-mobile.module.css'

import { useCallback, useEffect, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

type PythonState = {
  mode: 'auto' | 'manual'
  path: string
  version: string
  status: string
  requestId: string
  detectedPath: string
  detectedVersion: string
  error: string
  checkedAt?: string
  updatedAt: string
}

export function PythonEnvironmentDialog({ agentId, name, online }: { agentId: string; name: string; online: boolean }) {
  const { t } = useI18n()
  const text = t.vulnScan.agents.python
  const [open, setOpen] = useState(false)
  const [state, setState] = useState<PythonState | null>(null)
  const [mode, setMode] = useState<'auto' | 'manual'>('auto')
  const [path, setPath] = useState('')
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [timedOut, setTimedOut] = useState(false)
  const [loading, setLoading] = useState(false)
  const savedRequest = useRef('')
  const endpoint = `/api/v1/webscan/agents/${encodeURIComponent(agentId)}/python`

  const read = useCallback(async () => {
    const response = await fetchWithAuth(endpoint)
    const result = await response.json().catch(() => ({ success: false, error: text.loadFailed }))
    if (!response.ok || !result.success) throw new Error(result.error || text.loadFailed)
    return result.data as PythonState
  }, [endpoint, text.loadFailed])

  useEffect(() => {
    if (!open) return
    let cancelled = false
    read().then(value => {
      if (cancelled) return
      setState(value)
      setMode(value.mode || 'auto')
      setPath(value.path || '')
    }).catch(err => { if (!cancelled) setError(String(err.message)) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [open, read])

  useEffect(() => {
    if (!open || state?.status !== 'pending') return
    let cancelled = false
    const started = Date.now()
    const timer = setInterval(async () => {
      if (Date.now() - started > 30000) {
        setTimedOut(true)
        clearInterval(timer)
        return
      }
      try {
        const value = await read()
        if (cancelled) return
        setState(value)
        if (value.status === 'ready' && savedRequest.current === value.requestId) {
          savedRequest.current = ''
          setPath(value.path)
          toast.success(text.saved)
        }
      } catch (err) { if (!cancelled) setError(err instanceof Error ? err.message : text.loadFailed) }
    }, 1500)
    return () => { cancelled = true; clearInterval(timer) }
  }, [open, state?.status, state?.requestId, read, text.saved, text.loadFailed])

  async function check(save: boolean) {
    setSubmitting(true)
    setError('')
    setTimedOut(false)
    try {
      const response = await fetchWithAuth(endpoint, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ mode: save ? mode : 'auto', path: save ? path : '', save }),
      })
      const result = await response.json().catch(() => ({ success: false, error: text.loadFailed }))
      if (!response.ok || !result.success) throw new Error(result.error || text.checkFailed)
      savedRequest.current = save ? result.data.requestId : ''
      setState(current => ({ ...current!, status: 'pending', requestId: result.data.requestId, error: '', detectedPath: '', detectedVersion: '' }))
    } catch (err) { setError(err instanceof Error ? err.message : text.checkFailed) }
    finally { setSubmitting(false) }
  }

  const pending = submitting || (state?.status === 'pending' && !timedOut)
  return (
    <Dialog open={open} onOpenChange={value => {
      setOpen(value)
      if (value) { setLoading(true); setState(null); setError(''); setTimedOut(false) }
    }}>
      <DialogTrigger asChild><Button variant="outline" size="sm">{text.title}</Button></DialogTrigger>
      <DialogContent className={`${mobile.dialog} sm:max-w-xl`}>
        <DialogHeader>
          <DialogTitle>{name} · {text.title}</DialogTitle>
          <DialogDescription>{text.description}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="node-python-mode">{text.mode}</Label>
            <Select value={mode} onValueChange={value => setMode(value as 'auto' | 'manual')} disabled={loading || pending || !online}>
              <SelectTrigger id="node-python-mode" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="auto">{text.auto}</SelectItem>
                <SelectItem value="manual">{text.manual}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="node-python-path">{text.path}</Label>
            <Input id="node-python-path" value={path} onChange={e => setPath(e.target.value)} disabled={mode === 'auto' || loading || pending || !online} placeholder={text.pathPlaceholder} className="font-mono text-sm" />
          </div>
          <div className="space-y-1 rounded-md border border-border bg-muted/30 p-3 text-sm" aria-live="polite">
            <p>{loading ? text.loading : pending ? text.pending : timedOut ? text.timeout : state?.status === 'ready' ? text.ready : state?.status === 'error' ? text.failed : text.unchecked}</p>
            {state?.detectedPath && !pending && <p className="break-all font-mono text-xs">{state.detectedPath}</p>}
            {state?.detectedVersion && !pending && <p>Python {state.detectedVersion}</p>}
            {state?.version && <p className="break-all text-xs text-muted-foreground">{text.applied}: Python {state.version} · {state.path}</p>}
          </div>
          {(error || state?.error) && <p role="alert" className="break-words text-sm text-destructive">{error || state?.error}</p>}
          {!online && <p className="text-sm text-muted-foreground">{text.offline}</p>}
          <p className="text-xs text-muted-foreground">{text.scope}</p>
          <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
            <Button variant="outline" onClick={() => void check(false)} disabled={!state || loading || pending || !online}>{text.detect}</Button>
            <Button onClick={() => void check(true)} disabled={!state || loading || pending || !online || (mode === 'manual' && !path.trim())}>{text.save}</Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
