'use client'

import { useEffect, useRef, useState } from 'react'
import { getAIWorkbenchProjectTokenUsage, type AIWorkbenchTokenUsage } from '@/lib/api/ai-workbench'
import { TokenUsageButton } from './TokenUsage'

// A session switch briefly clears the project id while the next session loads,
// so this button is re-targeted on every click. Waiting for the selection to
// settle keeps a burst of switches from firing - and then cancelling - a
// token-usage request per click.
const SETTLE_MS = 250
const REFRESH_INTERVAL_MS = 4000

export function ProjectTokenUsage({ projectId }: { projectId?: string }) {
  const [result, setResult] = useState<{ projectId: string; usage: AIWorkbenchTokenUsage } | null>(null)
  const controllerRef = useRef<AbortController | null>(null)
  const requestSeqRef = useRef(0)
  // The browser request is aborted only when the button goes away. Cancelling it
  // on every re-target is what surfaced as aborted token-usage requests during
  // rapid session switching, so a late answer is discarded by sequence instead.
  useEffect(() => () => {
    controllerRef.current?.abort()
    controllerRef.current = null
  }, [])
  useEffect(() => {
    if (!projectId) return
    const sequence = ++requestSeqRef.current
    let pending = false
    const refresh = async () => {
      if (document.hidden || pending) return
      const controller = controllerRef.current ?? (controllerRef.current = new AbortController())
      if (controller.signal.aborted) return
      pending = true
      try {
        const usage = await getAIWorkbenchProjectTokenUsage(projectId, controller.signal)
        if (sequence === requestSeqRef.current && !controller.signal.aborted) setResult({ projectId, usage })
      } catch {
        // Do not present a previously fetched value as current after a failure.
        if (sequence === requestSeqRef.current && !controller.signal.aborted) setResult(null)
      } finally {
        pending = false
      }
    }
    let interval: number | undefined
    const settle = window.setTimeout(() => {
      void refresh()
      interval = window.setInterval(() => void refresh(), REFRESH_INTERVAL_MS)
    }, SETTLE_MS)
    document.addEventListener('visibilitychange', refresh)
    return () => {
      window.clearTimeout(settle)
      if (interval !== undefined) window.clearInterval(interval)
      document.removeEventListener('visibilitychange', refresh)
    }
  }, [projectId])
  if (!projectId) return null
  return <TokenUsageButton scope="project" usage={result?.projectId === projectId ? result.usage : undefined} />
}
