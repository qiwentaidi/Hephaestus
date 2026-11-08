'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { listAIWorkbenchPenTestSessions, type AIWorkbenchConversationSummary, type AIWorkbenchConversationListResponse } from '@/lib/api/ai-workbench'

export type ProjectSessionPage = {
  query: string
  revision: number
  conversations: AIWorkbenchConversationSummary[]
  page: number
  total: number
  roleCounts?: AIWorkbenchConversationListResponse['roleCounts']
  loading: boolean
  error: boolean
  retryMore?: boolean
}

// Project pages are independent of the global search/discovery page. Refresh
// every loaded page together so a background refresh cannot truncate history.
export function useProjectSessions(search: string, openProjects: Record<string, boolean>, revision: number) {
  const query = search.trim()
  const [pages, setPages] = useState<Record<string, ProjectSessionPage>>({})
  const pagesRef = useRef(pages)
  const requests = useRef<Record<string, number>>({})
  const scope = useRef({ query, revision })
  useEffect(() => { scope.current = { query, revision } }, [query, revision])

  const publish = useCallback((next: Record<string, ProjectSessionPage>) => {
    pagesRef.current = next
    setPages(next)
  }, [])

  const load = useCallback(async (projectId: string, more = false) => {
    if (!projectId) return false
    const { query, revision } = scope.current
    const previous = pagesRef.current[projectId]
    const current = previous?.query === query ? previous : undefined
    if (current?.loading) return false
    if (more && current?.page && current.conversations.length >= current.total) return false
    const page = Math.max(1, (current?.page || 0) + (more ? 1 : 0))
    const request = (requests.current[projectId] || 0) + 1
    requests.current[projectId] = request
    const pending: ProjectSessionPage = {
      query, revision, conversations: current?.conversations || [],
      page: current?.page || 0, total: current?.total || 0, roleCounts: current?.roleCounts, loading: true, error: false, retryMore: more,
    }
    publish({ ...pagesRef.current, [projectId]: pending })
    try {
      const results = await Promise.all(Array.from({ length: page }, (_, index) =>
        listAIWorkbenchPenTestSessions({ projectId, page: index + 1, pageSize: 80, search: query || undefined }),
      ))
      if (requests.current[projectId] !== request || scope.current.query !== query) return false
      const unique = new Map(results.flatMap(result => result.conversations).map(item => [item.task.id, item]))
      publish({ ...pagesRef.current, [projectId]: {
        ...pending, conversations: [...unique.values()], page,
        total: results[0].total, roleCounts: results[0].roleCounts, loading: false,
      } })
      return true
    } catch {
      if (requests.current[projectId] !== request || scope.current.query !== query) return false
      publish({ ...pagesRef.current, [projectId]: { ...pending, loading: false, error: true } })
      return false
    }
  }, [publish])

  useEffect(() => {
    for (const [id, open] of Object.entries(openProjects)) {
      const page = pagesRef.current[id]
      if (open && id && (!page || page.query !== query || page.revision !== revision)) void load(id)
    }
  }, [load, openProjects, pages, query, revision])

  const updateSession = useCallback((summary: AIWorkbenchConversationSummary) => {
    const projectId = summary.task.projectId || summary.project?.id
    const next = { ...pagesRef.current }
    for (const [id, page] of Object.entries(next)) {
      const index = page.conversations.findIndex(item => item.task.id === summary.task.id)
      if (id === projectId) {
        if (index >= 0) {
          // Keep the row where it is. Prepending on every snapshot made the
          // selected row jump above its neighbour and then snap back when the
          // background refresh restored server order.
          const conversations = page.conversations.slice()
          conversations[index] = { ...conversations[index], ...summary, project: summary.project || conversations[index].project }
          next[id] = { ...page, conversations }
        } else if (!page.query) {
          next[id] = { ...page, conversations: [summary, ...page.conversations], total: Math.max(page.total, page.conversations.length + 1) }
        }
      } else if (index >= 0) {
        next[id] = { ...page, conversations: page.conversations.filter(item => item.task.id !== summary.task.id), total: Math.max(0, page.total - 1) }
      }
    }
    publish(next)
  }, [publish])

  const removeSession = useCallback((sessionId: string) => {
    const next = { ...pagesRef.current }
    for (const [id, page] of Object.entries(next)) {
      const conversations = page.conversations.filter(item => item.task.id !== sessionId)
      if (conversations.length === page.conversations.length) continue
      // Invalidate in-flight responses that still contain the removed session.
      requests.current[id] = (requests.current[id] || 0) + 1
      next[id] = { ...page, conversations, total: Math.max(0, page.total - 1), loading: false }
    }
    publish(next)
  }, [publish])

  return { pages, load, updateSession, removeSession }
}
