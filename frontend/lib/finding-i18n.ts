import type { Locale } from '@/lib/i18n'

export type FindingMessages = Locale['vulnerabilityManagement']

export function getFindingLabels(l: FindingMessages): {
  severity: Record<string, string>
  status: Record<string, string>
  reviewQueue: Record<string, string>
  execution: Record<string, string>
  confidence: Record<string, string>
} {
  return {
    severity: { critical: l.critical, high: l.high, medium: l.medium, low: l.low, info: l.info },
    status: {
      open: l.open, in_progress: l.confirmed, confirmed: l.confirmed,
      fixed: l.fixed, false_positive: l.falsePositive,
      ignored: l.acceptedRisk, accepted_risk: l.acceptedRisk,
    },
    reviewQueue: {
      all: l.allQueues, unreviewed: l.unreviewed, candidate: l.candidate,
      manual_confirmation: l.manualConfirmation, auth_retry: l.authRetry,
      promoted: l.promoted, auth_excluded: l.authExcluded, reviewed: l.reviewed,
    },
    execution: {
      pending: l.executionPending, queued: l.executionPending,
      running: l.executionRunning, in_progress: l.executionRunning,
      completed: l.executionCompleted, success: l.executionCompleted,
      failed: l.executionFailed, error: l.executionFailed,
      cancelled: l.executionCancelled, canceled: l.executionCancelled,
      blocked: l.executionBlocked, paused: l.executionPaused,
      timeout: l.executionTimeout, timed_out: l.executionTimeout,
      waiting_tool: l.waitingTool, waiting_approval: l.waitingApproval,
    },
    confidence: {
      confirmed: l.confidenceConfirmed,
      tentative: l.confidenceTentative,
      deprecated: l.confidenceDeprecated,
    },
  }
}
