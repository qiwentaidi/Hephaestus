export type ModelRetryNotice = { attempt: number; max: number }

type RetryActivity = {
  eventType?: string
  title: string
  detail?: string
  status?: string
  retry?: ModelRetryNotice
}

const retryDetailPattern = /第\s*(\d+)\s*\/\s*(\d+)\s*次重试/

function positiveInteger(value: unknown) {
  const number = Number(value)
  return Number.isInteger(number) && number > 0 ? number : 0
}

// Newer events carry attempt/maxRetries as structured metadata. Older history
// only has the human-readable detail ("… 后第 3/5 次重试"), so parse that as a
// fallback instead of re-deriving counts from the merged row order.
export function modelRetryNotice(detail?: string | null, attempt?: unknown, maxRetries?: unknown): ModelRetryNotice | null {
  const parsedAttempt = positiveInteger(attempt)
  const parsedMax = positiveInteger(maxRetries)
  if (parsedAttempt && parsedMax && parsedAttempt <= parsedMax) return { attempt: parsedAttempt, max: parsedMax }
  const match = retryDetailPattern.exec(detail || '')
  if (!match) return null
  const fromDetail = { attempt: Number(match[1]), max: Number(match[2]) }
  return fromDetail.attempt > 0 && fromDetail.max >= fromDetail.attempt ? fromDetail : null
}

// A model call retries up to N times and emits one notice per attempt. Collapse
// consecutive notices into a single activity so the process list shows
// "retrying 5/5" instead of five identical rows. Non-adjacent retry sequences
// stay separate because another event separates them.
export function foldModelRetryActivities<T extends RetryActivity>(activities: readonly T[]): T[] {
  const folded: T[] = []
  for (const activity of activities) {
    if (activity.eventType !== 'model_retry') {
      folded.push(activity)
      continue
    }
    const notice = activity.retry || modelRetryNotice(activity.detail) || null
    const previous = folded[folded.length - 1]
    if (!previous || previous.eventType !== 'model_retry') {
      folded.push({ ...activity, retry: notice || undefined })
      continue
    }
    const attempt = Math.max(previous.retry?.attempt || 0, notice?.attempt || 0)
    const max = Math.max(previous.retry?.max || 0, notice?.max || 0)
    folded[folded.length - 1] = {
      ...previous,
      ...activity,
      retry: attempt > 0 && max > 0 ? { attempt: Math.min(attempt, max), max } : undefined,
    }
  }
  return folded
}
