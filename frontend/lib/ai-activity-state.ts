interface ActivityState {
  type?: string
  eventType?: string
  status?: string
  title: string
}

const activeStatuses = new Set(['running', 'draining', 'waiting', 'waiting_tool', 'waiting_approval', 'pending', 'cancelling'])
const modelPhaseEvents = new Set(['iteration', 'model_retry', 'model_fallback', 'synthesis', 'finalization_auto_continue'])

function isModelPhaseNotice(activity: ActivityState) {
  if (activity.type !== 'marker') return false
  if (activity.eventType) return modelPhaseEvents.has(activity.eventType)
  // Older history may not contain eventType. Match only known phase labels.
  return /请求模型规划下一步|模型调用遇到临时问题，正在重试|模型主备切换中|正在基于已有证据综合结论|正在自动续跑验证/.test(activity.title)
}

export function activityAnimationStates(activities: readonly ActivityState[], runActive: boolean): boolean[] {
  return activities.map((activity, index) => {
    if (!runActive) return false
    const status = (activity.status || '').trim().toLowerCase()
    const latest = index === activities.length - 1
    if (!activeStatuses.has(status) && !(latest && !status)) return false
    // Phase notices are append-only events, not independently running jobs.
    // New progress supersedes their animation even when the stored status is running.
    // Tools and subtasks retain their own lifecycle, including parallel execution.
    return !isModelPhaseNotice(activity) || latest
  })
}
