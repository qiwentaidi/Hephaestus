'use client'

import { useCallback, useEffect, useState } from 'react'
import { fetchWithAuth } from '@/lib/api'
import ExternalMcpConnectionsPanel from './ExternalMcpConnectionsPanel'

type CallCounts = {
  tools: Record<string, number>
  external: Record<string, Record<string, number>>
}

async function loadToolCallCounts(): Promise<CallCounts> {
  const response = await fetchWithAuth('/api/v1/mcp/ai-tool-call-counts')
  const payload = await response.json().catch(() => {
    throw new Error(`HTTP ${response.status}: MCP service API unavailable`)
  })

  if (!response.ok || !payload.success) {
    throw new Error(payload.error || `HTTP ${response.status}`)
  }

  return payload.data as CallCounts
}

/** AI capabilities and external MCP connections share this management page. */
export default function ExternalMcpPanel() {
  const [callCounts, setCallCounts] = useState<CallCounts | null>(null)

  const refreshCallCounts = useCallback(async () => {
    try {
      setCallCounts(await loadToolCallCounts())
    } catch {
      setCallCounts(null)
    }
  }, [])

  useEffect(() => {
    void refreshCallCounts()
  }, [refreshCallCounts])

  return (
    <main className="flex min-h-full w-full min-w-0 flex-col p-4 lg:p-5">
      <ExternalMcpConnectionsPanel
        callCounts={callCounts?.tools}
        externalCallCounts={callCounts?.external}
        onRefreshCounts={() => void refreshCallCounts()}
      />
    </main>
  )
}
