import type { AIPlanRunSummary } from '@/lib/api/ai-plan'

// A terminal run no longer owns the project's next action, so the operator may
// start a fresh assessment without a prompt. Every other state still has a
// controller that can steer, resume or delegate, which is why creating a plain
// standalone session there has to be an explicit choice.
const TERMINAL_RUN_STATUSES = new Set(['completed', 'cancelled', 'failed', 'blocked'])

export type NewSessionGuardTarget = {
  projectId: string
  projectName: string
  run: AIPlanRunSummary
}

export function isLivePlanRun(status: string) {
  return !TERMINAL_RUN_STATUSES.has(String(status || '').trim().toLowerCase())
}

// Pick the newest live run for the project the sidebar is about to create a
// session in. Newest first keeps the guard pointed at the run the user just
// looked at when a project briefly has more than one. Read-only (legacy) runs
// have no writable controller, so they cannot be the guard's target.
export function activePlanRunForProject(runs: AIPlanRunSummary[], projectId?: string): AIPlanRunSummary | undefined {
  const id = String(projectId || '').trim()
  if (!id) return undefined
  return runs
    .filter((item) => Boolean(item?.run?.id) && !item.run.readOnly && item.run.projectId === id && isLivePlanRun(item.run.status))
    .sort((a, b) => {
      const left = String(b.run.updatedAt || b.run.createdAt || '')
      const right = String(a.run.updatedAt || a.run.createdAt || '')
      return left.localeCompare(right)
    })[0]
}
