export type PlannerRoundBoundaryCandidate = {
  title?: string | null
  eventType?: string | null
  roundBoundary?: boolean
}

// Planner round boundaries render as thin dividers instead of commentary cards.
// Newer progress payloads mark them explicitly; older history is identified by
// the stored label the engine wrote ("第 N 轮规划").
export function isPlannerRoundBoundary(activity: PlannerRoundBoundaryCandidate): boolean {
  if (activity.roundBoundary) return true
  if (activity.eventType !== 'assistant_commentary') return false
  return /^第\s*\d+\s*轮/.test((activity.title || '').trim())
}
