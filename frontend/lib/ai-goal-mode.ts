export type GoalModeAction = 'chat' | 'start' | 'continue'
export interface GoalModeRequest { action: GoalModeAction; successCriteria?: string; maxIterations?: number }
export interface GoalRun {
  id: string
  mode: 'goal' | 'chat'
  objective: string
  successCriteria: string
  status: string
  plan: { id: string; title: string; status: string; result?: string }[]
  currentStep: string
  reason: string
  evidence: string[]
  iterations: number
  maxIterations: number
  updatedAt: string
}

// The completion tool is saved before the final assistant message. Only fold
// stage replies once that saved conclusion is present, including in history.
export function goalTurnMessages<T extends { content: string }>(messages: T[], goal?: GoalRun | null) {
  const final = messages.at(-1)
  const completed = goal?.status === 'completed' && Boolean(goal.reason.trim())
    && final?.content.trim() === goal.reason.trim()
  return completed
    ? { completed: true, conclusions: messages.slice(-1), details: messages.slice(0, -1) }
    : { completed: false, conclusions: messages, details: [] as T[] }
}
