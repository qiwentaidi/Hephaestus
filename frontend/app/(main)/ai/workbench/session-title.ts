import type { AIWorkbenchConversationSummary } from '@/lib/api/ai-workbench'

/**
 * The rail label for a workbench session.
 *
 * A conversation title is the session's name: orchestration sessions are named
 * after the run objective, while a solo session is named after its target. The
 * first target of an orchestration run covers only part of the objective, so it
 * must not replace the Controller/Planner name.
 */
export function sessionTitle(summary: AIWorkbenchConversationSummary | null | undefined, fallback: string, max = 42) {
  const value = summary?.task.name || summary?.task.target || summary?.latestMessage?.content || fallback
  const text = (value || '').trim().replace(/\s+/g, ' ')
  return text.length <= max ? text : `${text.slice(0, max - 1)}…`
}
