'use client'

import { useEffect, useState } from 'react'
import { getAIWorkbenchProjectTokenUsage, type AIWorkbenchTokenUsage } from '@/lib/api/ai-workbench'
import { TokenUsageButton } from './TokenUsage'

export function ProjectTokenUsage({ projectId }: { projectId?: string }) {
  const [result, setResult] = useState<{ projectId: string; usage: AIWorkbenchTokenUsage } | null>(null)
  useEffect(() => {
    if (!projectId) return
    const controller = new AbortController()
    let pending = false
    const refresh = async () => {
      if (document.hidden || pending || controller.signal.aborted) return
      pending = true
      try {
        const usage = await getAIWorkbenchProjectTokenUsage(projectId, controller.signal)
        if (!controller.signal.aborted) setResult({ projectId, usage })
      } catch {
        // Do not present a previously fetched value as current after a failure.
        if (!controller.signal.aborted) setResult(null)
      } finally {
        pending = false
      }
    }
    void refresh()
    const timer = window.setInterval(() => void refresh(), 4000)
    document.addEventListener('visibilitychange', refresh)
    return () => {
      controller.abort()
      window.clearInterval(timer)
      document.removeEventListener('visibilitychange', refresh)
    }
  }, [projectId])
  if (!projectId) return null
  return <TokenUsageButton scope="project" usage={result?.projectId === projectId ? result.usage : undefined} />
}
