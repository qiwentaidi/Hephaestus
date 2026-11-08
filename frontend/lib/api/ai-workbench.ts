import type { GoalRun } from '@/lib/ai-goal-mode'
import { fetchWithAuth } from '@/lib/api'
import { WorkbenchRequestError } from '@/lib/ai-workbench-i18n'
import { readAIProcessFrames, type AIProcessUpdate } from '@/lib/ai-process-stream'
import { parseFilenameFromContentDisposition } from '@/lib/download'

export class AIWorkbenchStreamError extends Error {
  readonly fatal: boolean
  readonly status: number

  constructor(message: string, options: { fatal?: boolean; status?: number } = {}) {
    super(message)
    this.name = 'AIWorkbenchStreamError'
    this.fatal = options.fatal === true
    this.status = options.status || 0
  }
}

export interface AIPlanItem {
  id: string
  title: string
  rationale: string
  stepType: string
  riskLevel: string
  approvalRequired: boolean
  dependsOn?: string[]
}

export interface AIWorkbenchTask {
  readOnly?: boolean
  id: string
  projectId?: string
  projectName?: string
  name: string
  target: string
  scope: string[]
  mode: string
  agentModel: string
  agentProviderId: string
  status: string
  riskPolicy: string
  objective: string
  successCriteria: string
  deliveryFormat: string[]
  strategyPacks?: string[]
  currentPlan: AIPlanItem[]
  summary: string
  createdBy: string
  pendingApprovalCount?: number
  startedAt?: string | null
  completedAt?: string | null
  // controller/mainagent are reserved for the Phase 2 control plane; the
  // session rail groups them above the planner and workers.
  agentRole?: 'main' | 'controller' | 'mainagent' | 'planner' | 'worker' | ''
  parentConversationId?: string
  planRunId?: string
  planItemTitle?: string
  planItemStatus?: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchProject {
  id: string
  name: string
  description: string
  scope: string[]
  status: string
  conversationCount?: number
  factCount?: number
  findingCount?: number
  conversations?: AIWorkbenchConversationSummary[]
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchStep {
  id: string
  taskId: string
  parentStepId: string
  stepType: string
  title: string
  rationale: string
  status: string
  riskLevel: string
  approvalRequired: boolean
  approvedBy: string
  approvalNote: string
  startedAt?: string | null
  finishedAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchPlanTask {
  id: string
  taskId: string
  stepId: string
  parentId: string
  title: string
  goal: string
  status: string
  priority: number
  dependsOn?: string[]
  evidenceRefs?: string[]
  blockedReason: string
  notes: string
  metadata?: Record<string, unknown> | null
  startedAt?: string | null
  completedAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchEvent {
  id: string
  taskId: string
  stepId: string
  type: string
  level: string
  title: string
  content: string
  payload?: Record<string, unknown> | null
  createdAt: string
}

export interface AIWorkbenchMessage {
  id: string
  taskId: string
  parentMessageId: string
  role: string
  content: string
  metadata?: Record<string, unknown> | null
  createdAt: string
}

export interface AIWorkbenchMessageStreamPayload {
  process?: AIProcessUpdate
  taskId?: string
  userMessageId?: string
  assistantMessageId?: string
  streamId?: string
  delta?: string
  accumulated?: string
  content?: string
  metadata?: Record<string, unknown> | null
  message?: string
  tools?: string[]
  results?: unknown[]
  ok?: boolean
  signature?: string
  session?: AIWorkbenchPenTestSessionSnapshot
  detail?: AIWorkbenchTaskDetail
  timeline?: AIWorkbenchTimelineResponse | null
  agentState?: AIWorkbenchAgentState | null
  findingOverview?: AIWorkbenchFindingOverviewResponse | null
  ts?: string
}

export interface AIWorkbenchMessageStreamEvent {
  type: string
  data: AIWorkbenchMessageStreamPayload
}

export interface AIWorkbenchSideChatMessage {
  role: 'user' | 'assistant'
  content: string
}

export interface AIWorkbenchObservation {
  id: string
  taskId: string
  stepId: string
  source: string
  targetRef: string
  content: string
  confidence: string
  createdAt: string
}

export interface AIWorkbenchEvidenceItem {
  id: string
  taskId: string
  stepId: string
  sourceType: string
  sourceId: string
  targetRef: string
  title: string
  summary: string
  riskLevel: string
  confidence: string
  status: string
  reference: string
  metadata?: Record<string, unknown> | null
  observedAt: string
  createdAt: string
  lastObservedAt: string
}

export interface AIWorkbenchHypothesis {
  id: string
  taskId: string
  title: string
  description: string
  evidenceRefs: string[]
  status: string
  confidence: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchToolCall {
  id: string
  taskId: string
  stepId: string
  agentRunId: string
  executionId: string
  toolName: string
  inputJson: string
  outputSummary: string
  rawOutputRef: string
  status: string
  errorMessage: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchToolExecution {
  id: string
  taskId: string
  stepId: string
  agentRunId: string
  toolCallId: string
  toolName: string
  runtime: string
  inputSummary: string
  arguments?: Record<string, unknown> | null
  status: string
  outputSummary: string
  resultRef: string
  errorMessage: string
  startedAt?: string | null
  completedAt?: string | null
  durationMs: number
  metadata?: Record<string, unknown> | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchAgentRun {
  id: string
  taskId: string
  parentRunId: string
  runIndex: number
  triggerType: string
  agentModel: string
  status: string
  goal: string
  summary: string
  currentStepId: string
  startedAt?: string | null
  completedAt?: string | null
  durationMs: number
  metadata?: Record<string, unknown> | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchAssetBucket {
  id: string
  taskId: string
  bucketType: string
  targetRef: string
  riskScore: number
  summary: string
  signals?: Record<string, unknown> | null
  lastEventId: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchEvaluation {
  id: string
  taskId: string
  status: string
  score: number
  missingItems: string[]
  feedback: string
  traceSummary: string
  evaluator: string
  createdAt: string
}

export interface AIWorkbenchEvaluationDisagreementAudit {
  events: AIWorkbenchEvent[]
  latestEvaluation?: AIWorkbenchEvaluation | null
}

export interface AIWorkbenchAPIEndpoint {
  id: string
  taskId: string
  stepId: string
  method: string
  url: string
  host: string
  path: string
  operationName: string
  authRequired: boolean
  authProfileId: string
  source: string
  tags: string[]
  metadata?: Record<string, unknown> | null
  createdAt: string
  updatedAt: string
  lastObservedAt: string
}

export interface AIWorkbenchAPIRelation {
  id: string
  taskId: string
  sourceEndpointId: string
  targetEndpointId: string
  relationType: string
  variableName: string
  objectType: string
  evidenceRef: string
  confidence: string
  metadata?: Record<string, unknown> | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchAuthProfile {
  id: string
  taskId: string
  name: string
  authType: string
  location: string
  keyName: string
  tokenRef: string
  loginEndpointId: string
  metadata?: Record<string, unknown> | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchExtractedVariable {
  id: string
  taskId: string
  stepId: string
  name: string
  value: string
  valueType: string
  sourceType: string
  sourceId: string
  jsonPath: string
  regex: string
  headerName: string
  endpointId: string
  confidence: string
  metadata?: Record<string, unknown> | null
  createdAt: string
}

export interface AIWorkbenchHTTPRequestEvidence {
  id: string
  taskId: string
  stepId: string
  toolCallId: string
  endpointId: string
  method: string
  url: string
  requestHeaders?: Record<string, string> | null
  requestBodyPreview: string
  statusCode: number
  responseHeaders?: Record<string, string> | null
  responseBodyPreview: string
  responseLength: number
  durationMs: number
  variables?: Record<string, string> | null
  createdAt: string
}

export interface AIWorkbenchFindingAction {
  id: string
  action: string
  label: string
  description?: string
  variant?: string
  enabled: boolean
  disabledReason?: string
  findingId?: string
  targetStatus?: string
  reviewQueue?: string
  payload?: Record<string, unknown>
}

export interface AIWorkbenchVulnerabilityFinding {
 verificationStatus?: string
  id?: string
  taskId?: string
  taskName?: string
  aiRequestId?: string
  agentRunId?: string
  sourceTaskId?: string
  sourceVersion?: number
  sourceType?: string
  sourceId?: string
  dedupKey?: string
  source: string
  url: string
  rawTarget?: string
  name: string
  severity: string
  severityLabel: string
  status: string
  statusLabel: string
  vulnerabilityType?: string
  preconditions?: string
  reproductionSteps?: string
  evidence: string
  impact: string
  remediation: string
  retestNotes?: string
  sourceFactKey?: string
  reference: string
  reviewStatus?: string
  confirmationStatus?: string
  reviewDisposition?: string
  reviewVerdict?: string
  reviewQueue?: string
  lifecycleStatus?: string
  lifecycleLabel?: string
  primaryAction?: AIWorkbenchFindingAction
  actions?: AIWorkbenchFindingAction[]
  firstSeenAt?: string
  lastSeenAt?: string
  createdAt?: string
  updatedAt?: string
}

export interface AIWorkbenchFinding {
  id: string
  aiRequestId: string
  sourceTaskId: string
  sourceVersion: number
  sourceType: string
  sourceId: string
  dedupKey: string
  target: string
  url: string
  name: string
  severity: string
  status: string
  evidence: string
  impact: string
  remediation: string
  reference: string
  metadata?: Record<string, unknown> | null
  firstSeenAt: string
  lastSeenAt: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchReport {
  format: string
  content: string
  findings?: AIWorkbenchVulnerabilityFinding[]
  conclusion?: string
  riskBreakdown?: string
  attachment?: { name: string; downloadUrl: string; mimeType: string }
}

export interface AIWorkbenchFindingsResponse {
  findings: AIWorkbenchVulnerabilityFinding[]
  canonicalFindings: AIWorkbenchFinding[]
  total: number
  page: number
  pageSize: number
  riskBreakdown: string
  severityDistribution: Record<string, number>
  statusDistribution: Record<string, number>
}

export type AIWorkbenchFindingReviewStatus = 'reviewed' | 'all'

export type AIWorkbenchFindingConfirmationStatus =
  | 'unreviewed'
  | 'candidate'
  | 'reviewed_by_http_observe'
  | 'reviewed_by_auth_compare'
  | 'promoted_by_auth_compare'
  | 'auth_required'
  | 'auth_compare_failed'
  | 'all'

export type AIWorkbenchFindingReviewDisposition =
  | 'none'
  | 'promoted_to_result_finding'
  | 'excluded_by_auth_compare'
  | 'retry_with_valid_auth_context'
  | 'needs_manual_confirmation'
  | 'all'

export type AIWorkbenchFindingReviewVerdict =
  | 'none'
  | 'possible_unauthorized'
  | 'auth_required'
  | 'auth_request_failed'
  | 'same_success'
  | 'same_status'
  | 'different_access'
  | 'all'

export type AIWorkbenchFindingReviewQueue =
  | 'unreviewed'
  | 'candidate'
  | 'manual_confirmation'
  | 'auth_retry'
  | 'promoted'
  | 'auth_excluded'
  | 'reviewed'
  | 'all'

export interface AIWorkbenchFindingOverviewBucket {
  key: string
  label: string
  count: number
  openCount: number
  highestSeverity: string
  severityDistribution: Record<string, number>
  statusDistribution: Record<string, number>
  latestSeenAt?: string
  metadata?: Record<string, unknown>
}

export interface AIWorkbenchFindingActionSummary {
  actionableTotal: number
  openTotal: number
  inProgressTotal: number
  resolvedTotal: number
  criticalHighOpenTotal: number
  needsReviewTotal: number
  candidateTotal: number
  manualConfirmationTotal: number
  authRetryTotal: number
  promotedResultTotal: number
  authExcludedTotal: number
}

export interface AIWorkbenchFindingOverviewResponse {
  total: number
  riskBreakdown: string
  actionSummary: AIWorkbenchFindingActionSummary
  severityDistribution: Record<string, number>
  statusDistribution: Record<string, number>
  confirmationStatusDistribution: Record<string, number>
  reviewDispositionDistribution: Record<string, number>
  reviewVerdictDistribution: Record<string, number>
  byTask: AIWorkbenchFindingOverviewBucket[]
  byTarget: AIWorkbenchFindingOverviewBucket[]
  bySource: AIWorkbenchFindingOverviewBucket[]
  byAgentRun: AIWorkbenchFindingOverviewBucket[]
  byReviewQueue: AIWorkbenchFindingOverviewBucket[]
  recent: AIWorkbenchVulnerabilityFinding[]
}

export interface AIWorkbenchFindingManagementAction {
  id: string
  action: string
  label: string
  description?: string
  variant?: string
  enabled: boolean
  disabledReason?: string
  taskId?: string
  reviewQueue?: string
  targetStatus?: string
  filters?: Record<string, unknown>
  count?: number
}

export interface AIWorkbenchFindingManagementQueue {
  key: string
  label: string
  description: string
  count: number
  openCount: number
  highestSeverity: string
  severityDistribution: Record<string, number>
  statusDistribution: Record<string, number>
  latestSeenAt?: string
  filters: Record<string, unknown>
  recommendedAction?: AIWorkbenchFindingManagementAction | null
}

export interface AIWorkbenchFindingManagementResponse {
  taskId?: string
  scope: 'global' | 'session' | string
  defaultReviewQueue: AIWorkbenchFindingReviewQueue
  defaultFilters: Record<string, unknown>
  overview: AIWorkbenchFindingOverviewResponse
  queues: AIWorkbenchFindingManagementQueue[]
  recommendedAction?: AIWorkbenchFindingManagementAction | null
  actions: AIWorkbenchFindingManagementAction[]
  recent: AIWorkbenchVulnerabilityFinding[]
}

export interface AIWorkbenchFindingDetail {
  finding: AIWorkbenchFinding
  view: AIWorkbenchVulnerabilityFinding
  task?: AIWorkbenchTask | null
  agentRun?: AIWorkbenchAgentRun | null
  sourceTask?: Record<string, unknown> | null
  webFinding?: Record<string, unknown> | null
  jsFinding?: Record<string, unknown> | null
  evidence: AIWorkbenchEvidenceItem[]
  toolCalls: AIWorkbenchToolCall[]
  executions: AIWorkbenchToolExecution[]
  events: AIWorkbenchEvent[]
  relatedScanRef?: string
}

export interface AIWorkbenchFindingReport {
  dispositions?: { id: string; previousStatus: string; status: string; note: string; actor: string; createdAt: string }[]
  view: AIWorkbenchVulnerabilityFinding
  project: AIWorkbenchProject
  conversation?: { id: string; title: string; target: string } | null
  facts: AIWorkbenchBlackboardFact[]
  edges: AIWorkbenchBlackboardEdge[]
  executionRefs: Pick<AIWorkbenchExecutionEvidence, 'id' | 'conversationId' | 'toolName' | 'status' | 'startedAt' | 'completedAt'>[]
  retests?: AIWorkbenchFindingRetest[]
  missingEvidenceRefs: string[]
  chainTruncated: boolean
}

export interface AIWorkbenchFindingRetest {
  id: string
  findingId: string
  conversationId: string
  result: 'still_vulnerable' | 'fixed' | 'inconclusive' | string
  summary: string
  evidence: string
  executionRefs: string[]
  createdAt: string
}

export interface AIWorkbenchFindingLineageNode {
  id: string
  kind: 'intent' | 'fact' | 'finding' | string
  title: string
  summary?: string
  body?: string
  status?: string
  confidence?: string
  factKey?: string
  findingId?: string
  intentId?: string
  conversationId?: string
  origin?: string
  priority?: number
}

export interface AIWorkbenchFindingLineageEdge {
  id: string
  source: string
  target: string
  relation: string
  persisted: boolean
}

export interface AIWorkbenchFindingLineage {
  findingId: string
  nodes: AIWorkbenchFindingLineageNode[]
  edges: AIWorkbenchFindingLineageEdge[]
  mode: 'durable' | 'explicit_legacy' | 'empty' | string
  missingRefs?: string[]
  truncated: boolean
}

export type AIWorkbenchFindingStatus = 'open' | 'in_progress' | 'fixed' | 'false_positive' | 'ignored'

export interface AIWorkbenchFindingBatchStatusFailure {
  findingId: string
  error: string
}

export interface AIWorkbenchFindingBatchStatusResponse {
  updated: AIWorkbenchVulnerabilityFinding[]
  canonicalFindings: AIWorkbenchFinding[]
  failed: AIWorkbenchFindingBatchStatusFailure[]
  total: number
  updatedCount: number
  failedCount: number
  sourceUpdatedCount: number
  status: AIWorkbenchFindingStatus
}

export interface AIWorkbenchFindingStats {
  total: number
  severity: Record<string, number>
  status: Record<string, number>
}

export interface AIWorkbenchConversationRiskSummary {
  totalFindings: number
  actionableFindings: number
  criticalOpenFindings: number
  highOpenFindings: number
  highestOpenSeverity: string
  highestOpenSeverityLabel: string
  hasActionableFindings: boolean
  riskBreakdown: string
}

export interface AIWorkbenchTokenUsage {
  totalTokens?: number
  inputTokens?: number
  outputTokens?: number
  cacheReadTokens?: number
}

export interface AIWorkbenchConversationSummary {
  totalTokens?: number
  tokenUsage?: AIWorkbenchTokenUsage
  sessionMode: string
  entrypoint: string
  task: AIWorkbenchTask
  project?: AIWorkbenchProject | null
  model: AIWorkbenchPenTestSessionModel
  launchPlan: AIWorkbenchPenTestSessionLaunchPlan
  latestMessage?: AIWorkbenchMessage | null
  recentChatItems: AIWorkbenchChatItem[]
  activeRun?: AIWorkbenchAgentRun | null
  latestRun?: AIWorkbenchAgentRun | null
  findingStats: AIWorkbenchFindingStats
  findingOverview?: AIWorkbenchFindingOverviewResponse | null
  riskSummary: AIWorkbenchConversationRiskSummary
  phase: AIWorkbenchAgentPhase
  currentActivity: AIWorkbenchAgentActivity
  recommendedAction: string
  primaryAction?: AIWorkbenchPenTestSessionAction | null
  actions: AIWorkbenchPenTestSessionAction[]
  lastActivityAt: string
}

export interface AIWorkbenchProjectListResponse {
  projects: AIWorkbenchProject[]
  total: number
  page?: number
  pageSize?: number
}

export interface AIWorkbenchConversationListResponse {
  conversations: AIWorkbenchConversationSummary[]
  roleCounts?: Record<'main' | 'planner' | 'workers' | 'other', number>
  total: number
  page: number
  pageSize: number
  statusDistribution: Record<string, number>
}

export interface AIWorkbenchConversationOption {
  id: string
  name: string
}

export interface AIWorkbenchConversationOptionsResponse {
  conversations: AIWorkbenchConversationOption[]
  total: number
  page: number
  pageSize: number
  hasMore: boolean
}

export async function listAIWorkbenchProjectConversationOptions(
  projectId: string,
  page = 1,
): Promise<AIWorkbenchConversationOptionsResponse> {
  const query = new URLSearchParams({ page: String(page), pageSize: '20' })
  const response = await fetchWithAuth(`/api/v1/ai/projects/${encodeURIComponent(projectId)}/conversation-options?${query}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchConversationOptionsResponse>(response)
}

export type AIWorkbenchTimelineItemType = 'target' | 'message' | 'intent' | 'marker' | 'tool' | 'finding' | 'error'

export interface AIWorkbenchTimelineTarget {
  target: string
  scope: string[]
  mode: string
  riskPolicy: string
}

export interface AIWorkbenchTimelineMessage {
  role: string
  label: string
  content: string
  parentMessageId?: string
  agentRunId?: string
  streamId?: string
  metadata?: Record<string, unknown> | null
}

export interface AIWorkbenchTimelineTargetView {
  kind: string
  displayName: string
  target?: string
  url?: string
  method?: string
  host?: string
  path?: string
  scope?: string[]
  mode?: string
  riskPolicy?: string
  riskScore?: number
  tags?: string[]
  metadata?: Record<string, unknown> | null
}

export interface AIWorkbenchBlockedMatch {
  ruleId?: string
  ruleName?: string
  matchedText?: string
  message?: string
}

export interface AIWorkbenchTimelineTool {
  name: string
  displayName: string
  status: string
  toolCallId?: string
  executionId?: string
  inputSummary?: string
  outputSummary?: string
  errorMessage?: string
  arguments?: Record<string, unknown> | null
  input?: Record<string, unknown> | null
  resultRef?: string
  scanRef?: string
  scanTaskId?: string
  scanType?: string
  scanMode?: string
  targets?: string[]
  startedAt?: string | null
  completedAt?: string | null
  durationMs?: number
  blockedMatch?: AIWorkbenchBlockedMatch | null
  blockedPhase?: string
  defaultExpanded: boolean
}

export interface AIWorkbenchTimelineMarker {
  status: string
  label: string
  description?: string
  activityKey?: string
  count?: number
  eventType?: string
  durationMs?: number
  metadata?: Record<string, unknown> | null
}

export interface AIWorkbenchTimelineIntent {
  id: string
  intentType: string
  label: string
  summary: string
  source: string
  confidence: string
  userMessageId?: string
  agentRunId?: string
  toolNames?: string[]
  targets?: string[]
  metadata?: Record<string, unknown> | null
}

export interface AIWorkbenchTimelineItem {
  id: string
  type: AIWorkbenchTimelineItemType
  runId?: string
  stepId?: string
  activityKey?: string
  status?: string
  label?: string
  description?: string
  collapsedCount?: number
  collapsedItems?: AIWorkbenchTimelineMarker[]
  metadata?: Record<string, unknown> | null
  message?: AIWorkbenchMessage | null
  messageView?: AIWorkbenchTimelineMessage | null
  event?: AIWorkbenchEvent | null
  marker?: AIWorkbenchTimelineMarker | null
  tool?: AIWorkbenchTimelineTool | null
  toolCall?: AIWorkbenchToolCall | null
  execution?: AIWorkbenchToolExecution | null
  finding?: AIWorkbenchFinding | null
  findingView?: AIWorkbenchVulnerabilityFinding | null
  target?: AIWorkbenchTimelineTarget | null
  targetView?: AIWorkbenchTimelineTargetView | null
  intent?: AIWorkbenchTimelineIntent | null
  assetBucket?: AIWorkbenchAssetBucket | null
  apiEndpoint?: AIWorkbenchAPIEndpoint | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchChatItem {
  id: string
  type: AIWorkbenchTimelineItemType
  runId?: string
  stepId?: string
  activityKey?: string
  status?: string
  label?: string
  description?: string
  collapsedCount?: number
  collapsedItems?: AIWorkbenchTimelineMarker[]
  metadata?: Record<string, unknown> | null
  message?: AIWorkbenchTimelineMessage | null
  marker?: AIWorkbenchTimelineMarker | null
  error?: AIWorkbenchTimelineMarker | null
  tool?: AIWorkbenchTimelineTool | null
  finding?: AIWorkbenchVulnerabilityFinding | null
  target?: AIWorkbenchTimelineTargetView | null
  intent?: AIWorkbenchTimelineIntent | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchTimelineRun {
  run: AIWorkbenchAgentRun
  summary: AIWorkbenchAgentRunActivitySummary
  items: AIWorkbenchTimelineItem[]
  chatItems: AIWorkbenchChatItem[]
}

export interface AIWorkbenchConversationTurn {
  /** Older turns ship previews only (payload control); fetch the full turn on demand. */
  bodyTruncated?: boolean
  /** How many tool/chat items were dropped from a truncated turn. */
  chatItemsOmitted?: number
  totalTokens?: number
  tokenUsage?: AIWorkbenchTokenUsage
  goalRun?: GoalRun | null
  id: string
  userMessage?: AIWorkbenchMessage | null
  userMessageView?: AIWorkbenchTimelineMessage | null
  assistantMessages: AIWorkbenchMessage[]
  assistantMessageViews: AIWorkbenchTimelineMessage[]
  run?: AIWorkbenchAgentRun | null
  runSummary?: AIWorkbenchAgentRunActivitySummary | null
  items: AIWorkbenchTimelineItem[]
  chatItems: AIWorkbenchChatItem[]
  startedAt: string
  updatedAt: string
}

export interface AIWorkbenchAgentRunActivitySummary {
  phase: AIWorkbenchAgentPhase
  itemCount: number
  markerCount: number
  toolCount: number
  findingCount: number
  findingStats: AIWorkbenchFindingStats
  targetCount: number
  errorCount: number
  durationMs: number
}

export interface AIWorkbenchTimelineResponse {
  task: AIWorkbenchTask
  items: AIWorkbenchTimelineItem[]
  chatItems: AIWorkbenchChatItem[]
  runs: AIWorkbenchTimelineRun[]
  turns: AIWorkbenchConversationTurn[]
}

export interface AIWorkbenchTimelineStreamPayload {
  taskId?: string
  signature?: string
  timeline?: AIWorkbenchTimelineResponse
  ts?: string
  error?: string
}

export interface AIWorkbenchAgentStateStreamPayload {
  taskId?: string
  signature?: string
  state?: AIWorkbenchAgentState
  ts?: string
  error?: string
}

export interface AIWorkbenchPenTestSessionStreamPayload {
  taskId?: string
  signature?: string
  session?: AIWorkbenchPenTestSessionSnapshot
  ts?: string
  error?: string
}

export interface AIWorkbenchAgentPhase {
  key: string
  status: string
  label: string
  description: string
  nextAction: string
}

export interface AIWorkbenchModelOption {
  providerId: string
  providerName: string
  protocol: string
  model: string
  enabled: boolean
  configured: boolean
  supportsTools: boolean
  supportsStreaming: boolean
  defaultForWorkbench: boolean
}

export interface AIWorkbenchModelOptionsResponse {
  defaultProviderId: string
  defaultModel: string
  options: AIWorkbenchModelOption[]
}

export interface AIWorkbenchConversationPreviewResponse {
  canStart: boolean
  error?: string
  warnings: string[]
  content: string
  detectedTarget: string
  detectedScope: string[]
  task: Partial<CreateAIWorkbenchTaskPayload>
  modelOptions: AIWorkbenchModelOptionsResponse
  selectedModel?: AIWorkbenchModelOption | null
  recommendedMode: string
  launchPlan: AIWorkbenchConversationLaunchPlan
}

export type AIWorkbenchPenTestSessionPreviewResponse = AIWorkbenchConversationPreviewResponse

export interface AIWorkbenchPenTestSessionBootstrapResponse {
  sessionMode: string
  entrypoint: string
  modelOptions: AIWorkbenchModelOptionsResponse
  defaultPreview: AIWorkbenchPenTestSessionPreviewResponse
  recentSessions: AIWorkbenchConversationSummary[]
  projects?: AIWorkbenchProject[]
  findingManagement: AIWorkbenchFindingManagementResponse
  statusDistribution: Record<string, number>
  recommendedMode: string
  warnings: string[]
  updatedAt: string
}

export interface AIWorkbenchConversationLaunchPlan {
  target: string
  scope: string[]
  mode: string
  riskPolicy: string
  agentProviderId?: string
  agentModel?: string
  modelConfigured: boolean
  supportsTools: boolean
  supportsStreaming: boolean
  autoStart: boolean
  initialMessage?: string
  preferredStrategies: string[]
  preferredCapabilities: string[]
  phases: string[]
  initialActions: AIWorkbenchConversationLaunchPlanAction[]
}

export type AIWorkbenchPenTestSessionLaunchPlan = AIWorkbenchConversationLaunchPlan

export interface AIWorkbenchConversationLaunchPlanAction {
  type: string
  toolName: string
  label: string
  targets?: string[]
  scanType?: string
  scanMode?: string
  riskLevel?: string
  reason?: string
  autoExecute: boolean
}

export interface AIWorkbenchApproval {
  id: string
  taskId: string
  stepId: string
  actionTitle: string
  riskLevel: string
  requestReason: string
  status: string
  decidedBy: string
  decisionNote: string
  requestedAt: string
  decidedAt?: string | null
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchTaskDetail {
  task: AIWorkbenchTask
  steps: AIWorkbenchStep[]
  planTasks: AIWorkbenchPlanTask[]
  events: AIWorkbenchEvent[]
  messages: AIWorkbenchMessage[]
  approvals: AIWorkbenchApproval[]
  observations: AIWorkbenchObservation[]
  evidence: AIWorkbenchEvidenceItem[]
  hypotheses: AIWorkbenchHypothesis[]
  toolCalls: AIWorkbenchToolCall[]
  executions: AIWorkbenchToolExecution[]
  agentRuns: AIWorkbenchAgentRun[]
  findings: AIWorkbenchFinding[]
  assetBuckets: AIWorkbenchAssetBucket[]
  evaluations: AIWorkbenchEvaluation[]
  apiEndpoints: AIWorkbenchAPIEndpoint[]
  apiRelations: AIWorkbenchAPIRelation[]
  authProfiles: AIWorkbenchAuthProfile[]
  variables: AIWorkbenchExtractedVariable[]
  httpRequests: AIWorkbenchHTTPRequestEvidence[]
  timeline?: AIWorkbenchTimelineResponse | null
  agentState?: AIWorkbenchAgentState | null
  findingOverview?: AIWorkbenchFindingOverviewResponse | null
}

export interface AIWorkbenchEvidenceResponse {
  observations: AIWorkbenchObservation[]
  evidence: AIWorkbenchEvidenceItem[]
  hypotheses: AIWorkbenchHypothesis[]
  toolCalls: AIWorkbenchToolCall[]
  assetBuckets: AIWorkbenchAssetBucket[]
}

export interface AIWorkbenchToolExecutionWaitResponse {
  done: boolean
  timedOut: boolean
  execution: AIWorkbenchToolExecution
}

export interface UpdateAIWorkbenchPlanTaskPayload {
  title?: string
  goal?: string
  status?: string
  blockedReason?: string
  notes?: string
  evidenceRefs?: string[]
  metadata?: Record<string, unknown>
}

export interface AIWorkbenchPenTestSessionTarget {
  target: string
  scope: string[]
  mode: string
  riskPolicy: string
  objective: string
  successCriteria: string
}

export interface AIWorkbenchPenTestSessionModel {
  agentModel: string
  agentProviderId: string
}

export interface AIWorkbenchPenTestSessionReportPreview {
  format: string
  content: string
  conclusion: string
  riskBreakdown: string
  findingCount: number
  attachment?: { name: string; downloadUrl: string; mimeType: string }
}

export interface AIWorkbenchPenTestSessionMaterials {
  assetBuckets: AIWorkbenchAssetBucket[]
  apiEndpoints: AIWorkbenchAPIEndpoint[]
  apiRelations: AIWorkbenchAPIRelation[]
  authProfiles: AIWorkbenchAuthProfile[]
  variables: AIWorkbenchExtractedVariable[]
  httpRequests: AIWorkbenchHTTPRequestEvidence[]
  assetBucketCount: number
  apiEndpointCount: number
  apiRelationCount: number
  authProfileCount: number
  variableCount: number
  httpRequestCount: number
  evidenceCount: number
  findingCount: number
}

export type AIWorkbenchPenTestSessionActionName =
  | 'start'
  | 'send_message'
  | 'replan'
  | 'pause'
  | 'resume'
  | 'cancel'
  | 'approve'
  | 'reject'
  | 'review_findings'
  | 'review_finding'
  | 'finding.review'
  | 'set_status'
  | 'finding.set_status'

export interface AIWorkbenchPenTestSessionAction {
  id: string
  action: AIWorkbenchPenTestSessionActionName | string
  label: string
  description: string
  variant: 'primary' | 'secondary' | 'danger' | string
  enabled: boolean
  disabledReason?: string
  stepId?: string
  approvalId?: string
  agentRunId?: string
  findingId?: string
  reviewQueue?: string
  status?: string
  targetStatus?: string
  payload?: Record<string, unknown> | null
}

export interface AIWorkbenchPenTestSessionSnapshot {
  tokenUsage?: AIWorkbenchTokenUsage
  goalRun?: GoalRun | null
  execution?: { id: string; nodeId: string; generation: number; status: string; leaseExpiresAt?: string; cancelRequested: boolean; deleteRequested: boolean; error?: string } | null

  totalTokens?: number
  sessionMode: string
  entrypoint: string
  task: AIWorkbenchTask
  project?: AIWorkbenchProject | null
  target: AIWorkbenchPenTestSessionTarget
  model: AIWorkbenchPenTestSessionModel
  launchPlan: AIWorkbenchPenTestSessionLaunchPlan
  agentState: AIWorkbenchAgentState
  currentActivity: AIWorkbenchAgentActivity
  primaryAction?: AIWorkbenchPenTestSessionAction | null
  actions: AIWorkbenchPenTestSessionAction[]
  materials: AIWorkbenchPenTestSessionMaterials
  apiCoverage?: { counts: { total: number; pending: number; tested: number; blocked: number } }
  todoPlan?: AIWorkbenchTodoPlan | null
  todoPlans?: AIWorkbenchTodoPlan[]
  timeline: AIWorkbenchTimelineResponse
  runs: AIWorkbenchAgentRun[]
  turns: AIWorkbenchConversationTurn[]
  /** Complete planner round index (id/label); bodies are paged in on demand. */
  roundIndex?: Array<{ id: string; label?: string; roundNumber?: number; startedAt: string; preview?: string }>
  chatItems: AIWorkbenchChatItem[]
  evidence: AIWorkbenchEvidenceItem[]
  findings: AIWorkbenchFinding[]
  findingViews: AIWorkbenchVulnerabilityFinding[]
  findingOverview: AIWorkbenchFindingOverviewResponse
  findingManagement: AIWorkbenchFindingManagementResponse
  reportPreview: AIWorkbenchPenTestSessionReportPreview
  updatedAt: string
}

export interface AIWorkbenchTodoPlan {
  conversationId: string
  goal: string
  source: 'agent'
  items: Array<{ id: string; title: string; status: 'pending' | 'in_progress' | 'completed' | 'paused' | 'blocked' | 'exhausted' | 'stopped'; priority?: number; reason?: string; intentId?: string; parentFactKeys?: string[]; parentFindingIds?: string[] }>
  version: number
  updatedAt: string
}

export interface AIWorkbenchBlackboardFact {
  id: string
  projectId: string
  factKey: string
  category: string
  summary: string
  body: string
  confidence: string
  sourceConversationId: string
  sourceMessageId: string
  pinned: boolean
  relatedFindingId: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchBlackboardEdge {
  id: string
  projectId: string
  sourceFactKey: string
  targetFactKey: string
  edgeType: string
  confidence: string
  sourceConversationId: string
  createdAt: string
  updatedAt: string
}

export interface AIWorkbenchExploration {
 digests?: Array<{id: string; conversationId: string; title: string; body: string; members: string[]}>
 goals?: Array<{ id: string; conversationId: string; goalKey: string; title: string; status: 'open' | 'met' | 'abandoned'; reason?: string; evidence: string[] }>
 intents: Array<{ id: string; conversationId: string; todoId?: string; title: string; status: string; priority?: number; reason?: string; origin?: string; reconstructionExecutionId?: string; createdAt?: string; updatedAt?: string }>
 relations: Array<{ source: string; target: string; relation: 'derived_from' | 'yields' | 'proves'; conversationId: string }>
 executions?: AIWorkbenchExplorationExecution[]
 truncated: boolean
}

export type AIWorkbenchExplorationExecution = {
  id: string
  intentId: string
  conversationId: string
} & Partial<Pick<AIWorkbenchExecutionEvidence, 'toolName' | 'status' | 'startedAt' | 'completedAt'>>

export interface AIWorkbenchPenTestSessionBlackboard {
 exploration?: AIWorkbenchExploration
 contextFindings?: AIWorkbenchVulnerabilityFinding[]
  todoPlan?: AIWorkbenchTodoPlan | null
  todoPlans?: AIWorkbenchTodoPlan[]
  // Context nodes resolve links across sessions; they are not session contributions.
  contextFacts?: AIWorkbenchBlackboardFact[]
  sourceConversations?: { id: string; title: string; target: string; status?: string; agentRole?: string; parentConversationId?: string; planRunId?: string; planItemTitle?: string; planItemStatus?: string }[]
  scope?: 'project' | 'session'
  project: AIWorkbenchProject
  conversation?: {
    id: string
    projectId: string
    title: string
    target: string
    mode: string
    status: string
    createdAt: string
    updatedAt: string
  } | null
  facts: AIWorkbenchBlackboardFact[]
  edges: AIWorkbenchBlackboardEdge[]
  findings: AIWorkbenchVulnerabilityFinding[]
  updatedAt: string
}

export interface AIWorkbenchExecutionEvidence {
 intentId?: string
  id: string
  conversationId: string
  toolName: string
  status: string
  arguments: unknown
  result: unknown
  error: string
  startedAt: string
  completedAt?: string | null
}

export interface AIWorkbenchAgentState {
  task: AIWorkbenchTask
  activeRun?: AIWorkbenchAgentRun | null
  latestRun?: AIWorkbenchAgentRun | null
  currentStep?: AIWorkbenchStep | null
  currentPlanTask?: AIWorkbenchPlanTask | null
  currentActivity: AIWorkbenchAgentActivity
  pendingApprovals: AIWorkbenchApproval[]
  nextPlanTasks: AIWorkbenchPlanTask[]
  runningToolCalls: AIWorkbenchToolCall[]
  runningExecutions: AIWorkbenchToolExecution[]
  openFindings: AIWorkbenchFinding[]
  phase: AIWorkbenchAgentPhase
  recommendedAction: string
  blockingReason: string
  canStart: boolean
  canResume: boolean
  canApprove: boolean
  canSendMessage: boolean
  summary: string
  updatedAt: string
}

export interface AIWorkbenchAgentActivity {
  type: string
  status: string
  label: string
  description?: string
  runId?: string
  stepId?: string
  planTaskId?: string
  toolCallId?: string
  executionId?: string
  toolName?: string
  scanRef?: string
  scanTaskId?: string
  scanType?: string
  scanMode?: string
  targets?: string[]
  findingId?: string
  approvalId?: string
  recommendedAction?: string
}

export interface AIWorkbenchTaskListResponse {
  tasks: AIWorkbenchTask[]
  total: number
  page: number
  pageSize: number
  statusDistribution: Record<string, number>
}

export interface AIWorkbenchTool {
  name: string
  title: string
  category: string
  description: string
  riskLevel: string
  approvalDefault: boolean
  capabilities: string[]
  source: string
  mcpName?: string
  autoExecutable: boolean
  resultMapping?: string[]
  inputSchema?: Record<string, unknown>
}

export interface AIWorkbenchStrategyPack {
  id: string
  title: string
  scenario: string
  description: string
  triggers: string[]
  preferredCapabilities: string[]
  evidenceRequirements: string[]
  safetyRules: string[]
}

export interface AIWorkbenchNode {
  id: string
  title: string
  nodeType: string
  status: string
  capabilities: string[]
  lastSeenAt?: string | null
  metadata?: Record<string, unknown> | null
}

export interface CreateAIWorkbenchTaskPayload {
  name: string
  target: string
  scope: string[]
  mode: string
  agentModel?: string
  agentProviderId?: string
  riskPolicy: string
  objective: string
  successCriteria: string
  deliveryFormat: string[]
  initialMessage?: string
  autoStart?: boolean
  metadata?: Record<string, unknown>
}

export interface CreateAIWorkbenchConversationPayload {
  content?: string
  clientRequestId?: string
  target?: string
  projectId?: string
  agentModel?: string
  agentProviderId?: string
  riskPolicy?: string
  metadata?: Record<string, unknown>
}

export type CreateAIWorkbenchPenTestSessionPayload = CreateAIWorkbenchConversationPayload

export interface SaveAIWorkbenchProjectPayload {
  name?: string
  description?: string
  scope?: string[]
  status?: string
}

export interface UpdateAIWorkbenchPenTestSessionPayload {
  projectId?: string
  title?: string
  target?: string
  status?: string
}

async function parseAPIResponse<T>(response: Response): Promise<T> {
  const data = await response.json().catch(() => null)
  if (!response.ok || !data?.success) {
    throw data?.error || data?.message ? new Error(data.error || data.message) : new WorkbenchRequestError('requestFailed', { status: response.status })
  }
  return data.data as T
}

export async function listAIWorkbenchTasks(params: {
  page?: number
  pageSize?: number
  search?: string
  status?: string
} = {}): Promise<AIWorkbenchTaskListResponse> {
  const query = new URLSearchParams()
  query.set('page', String(params.page || 1))
  query.set('pageSize', String(params.pageSize || 20))
  if (params.search) query.set('search', params.search)
  if (params.status && params.status !== 'all') query.set('status', params.status)

  const response = await fetchWithAuth(`/api/v1/ai/tasks?${query.toString()}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchTaskListResponse>(response)
}

export async function listAIWorkbenchConversations(params: {
  page?: number
  pageSize?: number
  status?: string
  search?: string
} = {}): Promise<AIWorkbenchConversationListResponse> {
  const query = new URLSearchParams()
  query.set('page', String(params.page || 1))
  query.set('pageSize', String(params.pageSize || 20))
  if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.search) query.set('search', params.search)
  const response = await fetchWithAuth(`/api/v1/ai/conversations?${query.toString()}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchConversationListResponse>(response)
}

export async function listAIWorkbenchPenTestSessions(params: {
  projectId?: string
  page?: number
  pageSize?: number
  status?: string
  search?: string
} = {}): Promise<AIWorkbenchConversationListResponse> {
  const query = new URLSearchParams()
  query.set('page', String(params.page || 1))
  query.set('pageSize', String(params.pageSize || 20))
  if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.search) query.set('search', params.search)
  if (params.projectId) query.set('projectId', params.projectId)
  const response = await fetchWithAuth(`/api/v1/ai/sessions?${query.toString()}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchConversationListResponse>(response)
}

export async function listAIWorkbenchPenTestProjects(params: {
  page?: number
  pageSize?: number
} = {}): Promise<AIWorkbenchProjectListResponse> {
  const query = new URLSearchParams()
  query.set('page', String(params.page || 1))
  query.set('pageSize', String(params.pageSize || 100))
  const response = await fetchWithAuth(`/api/v1/ai/projects?${query.toString()}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchProjectListResponse>(response)
}

export async function getAIWorkbenchProjectTokenUsage(projectId: string, signal?: AbortSignal): Promise<AIWorkbenchTokenUsage> {
  const response = await fetchWithAuth(`/api/v1/ai/projects/${encodeURIComponent(projectId)}/token-usage`, {
    cache: 'no-store', signal,
  })
  return parseAPIResponse<AIWorkbenchTokenUsage>(response)
}

export interface AIWorkbenchProjectStorageUsage {
  diskBytes: number
  projectId: string
}

export async function getAIWorkbenchProjectStorageUsage(projectId: string, signal?: AbortSignal): Promise<AIWorkbenchProjectStorageUsage> {
  const response = await fetchWithAuth(`/api/v1/ai/projects/${encodeURIComponent(projectId)}/storage`, {
    cache: 'no-store', signal,
  })
  return parseAPIResponse<AIWorkbenchProjectStorageUsage>(response)
}

export async function listAllAIWorkbenchPenTestProjects(): Promise<AIWorkbenchProject[]> {
  const projects: AIWorkbenchProject[] = []
  for (let page = 1; ; page++) {
    const data = await listAIWorkbenchPenTestProjects({ page, pageSize: 100 })
    projects.push(...data.projects)
    if (!data.projects.length || projects.length >= data.total) return projects
  }
}

export async function createAIWorkbenchPenTestProject(
  payload: SaveAIWorkbenchProjectPayload,
): Promise<AIWorkbenchProject> {
  const response = await fetchWithAuth('/api/v1/ai/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchProject>(response)
}

export async function updateAIWorkbenchPenTestProject(
  id: string,
  payload: SaveAIWorkbenchProjectPayload,
): Promise<AIWorkbenchProject> {
  const response = await fetchWithAuth(`/api/v1/ai/projects/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchProject>(response)
}

export async function deleteAIWorkbenchPenTestProject(id: string): Promise<void> {
  const response = await fetchWithAuth(`/api/v1/ai/projects/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  await parseAPIResponse<null>(response)
}

export async function getAIWorkbenchPenTestSessionBootstrap(params: {
  recentLimit?: number
} = {}): Promise<AIWorkbenchPenTestSessionBootstrapResponse> {
  const query = new URLSearchParams()
  if (params.recentLimit) query.set('recentLimit', String(params.recentLimit))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/bootstrap${suffix}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionBootstrapResponse>(response)
}

export async function createAIWorkbenchTask(payload: CreateAIWorkbenchTaskPayload): Promise<AIWorkbenchTaskDetail> {
  const response = await fetchWithAuth('/api/v1/ai/tasks', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchTaskDetail>(response)
}

export async function createAIWorkbenchConversation(
  payload: CreateAIWorkbenchConversationPayload,
): Promise<AIWorkbenchTaskDetail> {
  const response = await fetchWithAuth('/api/v1/ai/conversations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchTaskDetail>(response)
}

export async function createAIWorkbenchPenTestSession(
  payload: CreateAIWorkbenchPenTestSessionPayload,
): Promise<AIWorkbenchPenTestSessionSnapshot> {
  const response = await fetchWithAuth('/api/v1/ai/sessions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionSnapshot>(response)
}

export async function updateAIWorkbenchPenTestSession(
  taskId: string,
  payload: UpdateAIWorkbenchPenTestSessionPayload,
): Promise<AIWorkbenchPenTestSessionSnapshot> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!response.ok && payload.projectId !== undefined) {
    const data = await response.clone().json().catch(() => null)
    const key = (['moveSessionFailed', 'moveSessionBusy', 'moveProjectMissing', 'moveEvidenceConflict', 'moveSharedEvidence', 'moveSessionMissing', 'moveSessionRefreshFailed'] as const).find((candidate) => candidate === data?.errorKey)
    if (key) throw new WorkbenchRequestError(key)
  }
  const snapshot = await parseAPIResponse<AIWorkbenchPenTestSessionSnapshot>(response)
  if (payload.projectId !== undefined && snapshot.task.projectId !== payload.projectId.trim()) {
    throw new WorkbenchRequestError('moveSessionNotApplied')
  }
  return snapshot
}

export async function deleteAIWorkbenchPenTestSession(taskId: string): Promise<void> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}`, {
    method: 'DELETE',
  })
  await parseAPIResponse<null>(response)
}

export async function previewAIWorkbenchConversation(
  payload: CreateAIWorkbenchConversationPayload,
): Promise<AIWorkbenchConversationPreviewResponse> {
  const response = await fetchWithAuth('/api/v1/ai/conversations/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchConversationPreviewResponse>(response)
}

export async function previewAIWorkbenchPenTestSession(
  payload: CreateAIWorkbenchPenTestSessionPayload,
): Promise<AIWorkbenchPenTestSessionPreviewResponse> {
  const response = await fetchWithAuth('/api/v1/ai/sessions/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionPreviewResponse>(response)
}

export async function getAIWorkbenchTask(id: string, options: { signal?: AbortSignal } = {}): Promise<AIWorkbenchTaskDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(id)}`, {
    cache: 'no-store',
    signal: options.signal,
  })
  return parseAPIResponse<AIWorkbenchTaskDetail>(response)
}

export interface AIWorkbenchEarlierRounds {
  turns: AIWorkbenchConversationTurn[]
  hasMore: boolean
}

// getAIWorkbenchEarlierRounds pages a planner conversation's rounds backwards. The
// planner keeps one conversation for the whole run, so older rounds live outside the
// snapshot window and are pulled in on demand.
export async function getAIWorkbenchEarlierRounds(
  sessionId: string,
  before: string,
  limit = 8,
): Promise<AIWorkbenchEarlierRounds> {
  const query = new URLSearchParams({ before, limit: String(limit) })
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/rounds?${query.toString()}`,
    { cache: 'no-store' },
  )
  return parseAPIResponse<AIWorkbenchEarlierRounds>(response)
}

// getAIWorkbenchSessionTurn fetches one turn with its full bodies. The session
// snapshot truncates everything except the newest turns, so jumping to an older
// turn from the conversation rail loads it through this call.
export async function getAIWorkbenchSessionTurn(
  sessionId: string,
  turnId: string,
  options: { signal?: AbortSignal } = {},
): Promise<AIWorkbenchConversationTurn> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/turns/${encodeURIComponent(turnId)}`,
    { cache: 'no-store', signal: options.signal },
  )
  return parseAPIResponse<AIWorkbenchConversationTurn>(response)
}

export async function getAIWorkbenchPenTestSession(
  taskId: string,
  options: { signal?: AbortSignal } = {},
): Promise<AIWorkbenchPenTestSessionSnapshot> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}`, {
    cache: 'no-store',
    signal: options.signal,
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionSnapshot>(response)
}

const workbenchProgressCursors = new Map<string, number>()
function rememberWorkbenchCursor(taskId: string, cursor: unknown) {
  if (typeof cursor === 'number' && Number.isSafeInteger(cursor) && cursor > 0) {
    workbenchProgressCursors.set(taskId, Math.max(workbenchProgressCursors.get(taskId) || 0, cursor))
    if (workbenchProgressCursors.size > 200) {
      const first = workbenchProgressCursors.keys().next().value
      if (first) workbenchProgressCursors.delete(first)
    }
  }
}

export async function streamAIWorkbenchPenTestSession(
  taskId: string,
  options: {
    signature?: string
    intervalMs?: number
    signal?: AbortSignal
    onOpen?: () => void
    onProcess?: (update: AIProcessUpdate) => void
    onSnapshot: (session: AIWorkbenchPenTestSessionSnapshot, payload: AIWorkbenchPenTestSessionStreamPayload) => void
    onHeartbeat?: (payload: AIWorkbenchPenTestSessionStreamPayload) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  if (options.signature) query.set('signature', options.signature)
  if (options.intervalMs) query.set('intervalMs', String(options.intervalMs))
  // Keep resume cursors out of custom headers: older servers/proxies may reject
  // Last-Event-ID during a cross-origin preflight, preventing all live updates.
  const cursor = workbenchProgressCursors.get(taskId)
  if (cursor) query.set('lastEventId', String(cursor))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/stream${suffix}`, {
    cache: 'no-store',
    headers: { Accept: 'text/event-stream' },
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new AIWorkbenchStreamError(`stream failed (${response.status})`, {
      fatal: response.status === 404 || response.status === 410,
      status: response.status,
    })
  }
  options.onOpen?.()

  await readAIProcessFrames(response, (type, data) => {
    rememberWorkbenchCursor(taskId, data.cursor)
    if (type === 'process_update' && data.process) options.onProcess?.(data.process as AIProcessUpdate)
    else if (type === 'session_snapshot' && data.session) options.onSnapshot(data.session as AIWorkbenchPenTestSessionSnapshot, data as AIWorkbenchPenTestSessionStreamPayload)
    else if (type === 'heartbeat') options.onHeartbeat?.(data as AIWorkbenchPenTestSessionStreamPayload)
    else if (type === 'error') {
      // The backend sends pre-run/validation failures as HTTP 200 SSE error
      // frames. They are not retryable stream disconnects.
      throw new AIWorkbenchStreamError(String(data.error || data.message || 'AI stream failed'), { fatal: true, status: 200 })
    } else if (type === 'process_resync') {
      // Reconnect to obtain a fresh durable snapshot before consuming more blocks.
      throw new Error('Process stream requires resync')
    }
  })
}

export async function getAIWorkbenchTimeline(
  id: string,
  options: { signal?: AbortSignal } = {},
): Promise<AIWorkbenchTimelineResponse> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(id)}/timeline`, {
    cache: 'no-store',
    signal: options.signal,
  })
  return parseAPIResponse<AIWorkbenchTimelineResponse>(response)
}

export async function getAIWorkbenchPenTestSessionTimeline(
  taskId: string,
  options: { signal?: AbortSignal } = {},
): Promise<AIWorkbenchTimelineResponse> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/timeline`, {
    cache: 'no-store',
    signal: options.signal,
  })
  return parseAPIResponse<AIWorkbenchTimelineResponse>(response)
}

export async function streamAIWorkbenchTimeline(
  taskId: string,
  options: {
    signature?: string
    intervalMs?: number
    signal?: AbortSignal
    onOpen?: () => void
    onSnapshot: (timeline: AIWorkbenchTimelineResponse, payload: AIWorkbenchTimelineStreamPayload) => void
    onHeartbeat?: (payload: AIWorkbenchTimelineStreamPayload) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  if (options.signature) query.set('signature', options.signature)
  if (options.intervalMs) query.set('intervalMs', String(options.intervalMs))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/timeline/stream${suffix}`, {
    cache: 'no-store',
    headers: { Accept: 'text/event-stream' },
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new WorkbenchRequestError('timelineStreamFailed', { status: response.status })
  }
  options.onOpen?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    const eventType = frame
      .split('\n')
      .find((line) => line.startsWith('event:'))
      ?.slice('event:'.length)
      .trim()
    if (eventType !== 'timeline_snapshot' && eventType !== 'heartbeat') return

    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice('data:'.length).trimStart())
      .join('\n')
    if (!data) return
    const payload = JSON.parse(data) as AIWorkbenchTimelineStreamPayload
    if (eventType === 'timeline_snapshot' && payload.timeline) {
      options.onSnapshot(payload.timeline, payload)
    } else if (eventType === 'heartbeat') {
      options.onHeartbeat?.(payload)
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let frameEnd = buffer.indexOf('\n\n')
    while (frameEnd >= 0) {
      const frame = buffer.slice(0, frameEnd)
      buffer = buffer.slice(frameEnd + 2)
      consumeFrame(frame)
      frameEnd = buffer.indexOf('\n\n')
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) {
    consumeFrame(buffer)
  }
}

export async function streamAIWorkbenchPenTestSessionTimeline(
  taskId: string,
  options: {
    signature?: string
    intervalMs?: number
    signal?: AbortSignal
    onOpen?: () => void
    onSnapshot: (timeline: AIWorkbenchTimelineResponse, payload: AIWorkbenchTimelineStreamPayload) => void
    onHeartbeat?: (payload: AIWorkbenchTimelineStreamPayload) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  if (options.signature) query.set('signature', options.signature)
  if (options.intervalMs) query.set('intervalMs', String(options.intervalMs))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/timeline/stream${suffix}`, {
    cache: 'no-store',
    headers: { Accept: 'text/event-stream' },
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new WorkbenchRequestError('timelineStreamFailed', { status: response.status })
  }
  options.onOpen?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    const eventType = frame
      .split('\n')
      .find((line) => line.startsWith('event:'))
      ?.slice('event:'.length)
      .trim()
    if (eventType !== 'timeline_snapshot' && eventType !== 'heartbeat') return

    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice('data:'.length).trimStart())
      .join('\n')
    if (!data) return
    const payload = JSON.parse(data) as AIWorkbenchTimelineStreamPayload
    if (eventType === 'timeline_snapshot' && payload.timeline) {
      options.onSnapshot(payload.timeline, payload)
    } else if (eventType === 'heartbeat') {
      options.onHeartbeat?.(payload)
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let frameEnd = buffer.indexOf('\n\n')
    while (frameEnd >= 0) {
      const frame = buffer.slice(0, frameEnd)
      buffer = buffer.slice(frameEnd + 2)
      consumeFrame(frame)
      frameEnd = buffer.indexOf('\n\n')
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) {
    consumeFrame(buffer)
  }
}

export async function deleteAIWorkbenchTask(id: string): Promise<void> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  await parseAPIResponse<null>(response)
}

export async function streamAIWorkbenchEvents(
  taskId: string,
  options: {
    since?: string
    signal?: AbortSignal
    onOpen?: () => void
    onEvent: (event: AIWorkbenchEvent) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  if (options.since) query.set('since', options.since)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/events/stream${suffix}`, {
    cache: 'no-store',
    headers: { Accept: 'text/event-stream' },
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new WorkbenchRequestError('streamFailed', { status: response.status })
  }
  options.onOpen?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    const eventType = frame
      .split('\n')
      .find((line) => line.startsWith('event:'))
      ?.slice('event:'.length)
      .trim()
    if (eventType !== 'ai_event') return

    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice('data:'.length).trimStart())
      .join('\n')
    if (!data) return
    options.onEvent(JSON.parse(data) as AIWorkbenchEvent)
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let frameEnd = buffer.indexOf('\n\n')
    while (frameEnd >= 0) {
      const frame = buffer.slice(0, frameEnd)
      buffer = buffer.slice(frameEnd + 2)
      consumeFrame(frame)
      frameEnd = buffer.indexOf('\n\n')
    }
  }
}

export async function runAIWorkbenchAction(
  id: string,
  action: 'plan' | 'start' | 'pause' | 'resume' | 'cancel',
): Promise<AIWorkbenchTaskDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(id)}/${action}`, {
    method: 'POST',
  })
  return parseAPIResponse<AIWorkbenchTaskDetail>(response)
}

export async function listAIWorkbenchAgentRuns(
  taskId: string,
  params: { status?: string; limit?: number } = {},
): Promise<AIWorkbenchAgentRun[]> {
  const query = new URLSearchParams()
  if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.limit) query.set('limit', String(params.limit))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/agent-runs${suffix}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchAgentRun[]>(response)
}

export async function listAIWorkbenchPenTestSessionAgentRuns(
  taskId: string,
  params: { status?: string; limit?: number } = {},
): Promise<AIWorkbenchAgentRun[]> {
  const query = new URLSearchParams()
  if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.limit) query.set('limit', String(params.limit))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/agent-runs${suffix}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchAgentRun[]>(response)
}

export async function listAIWorkbenchPenTestSessionPlanTasks(
  taskId: string,
  params: { status?: string } = {},
): Promise<AIWorkbenchPlanTask[]> {
  const query = new URLSearchParams()
  if (params.status && params.status !== 'all') query.set('status', params.status)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/plan-tasks${suffix}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchPlanTask[]>(response)
}

export async function getAIWorkbenchPenTestSessionPlanTask(
  taskId: string,
  planTaskId: string,
): Promise<AIWorkbenchPlanTask> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(taskId)}/plan-tasks/${encodeURIComponent(planTaskId)}`,
    { cache: 'no-store' },
  )
  return parseAPIResponse<AIWorkbenchPlanTask>(response)
}

export async function updateAIWorkbenchPenTestSessionPlanTask(
  taskId: string,
  planTaskId: string,
  payload: UpdateAIWorkbenchPlanTaskPayload,
): Promise<AIWorkbenchPlanTask> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(taskId)}/plan-tasks/${encodeURIComponent(planTaskId)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    },
  )
  return parseAPIResponse<AIWorkbenchPlanTask>(response)
}

export async function getAIWorkbenchAgentState(taskId: string): Promise<AIWorkbenchAgentState> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/agent-state`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchAgentState>(response)
}

export async function getAIWorkbenchPenTestSessionAgentState(taskId: string): Promise<AIWorkbenchAgentState> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/agent-state`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchAgentState>(response)
}

export async function streamAIWorkbenchAgentState(
  taskId: string,
  options: {
    signature?: string
    intervalMs?: number
    signal?: AbortSignal
    onOpen?: () => void
    onSnapshot: (state: AIWorkbenchAgentState, payload: AIWorkbenchAgentStateStreamPayload) => void
    onHeartbeat?: (payload: AIWorkbenchAgentStateStreamPayload) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  if (options.signature) query.set('signature', options.signature)
  if (options.intervalMs) query.set('intervalMs', String(options.intervalMs))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/agent-state/stream${suffix}`, {
    cache: 'no-store',
    headers: { Accept: 'text/event-stream' },
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new WorkbenchRequestError('streamFailed', { status: response.status })
  }
  options.onOpen?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    const eventType = frame
      .split('\n')
      .find((line) => line.startsWith('event:'))
      ?.slice('event:'.length)
      .trim()
    if (eventType !== 'agent_state_snapshot' && eventType !== 'heartbeat') return

    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice('data:'.length).trimStart())
      .join('\n')
    if (!data) return
    const payload = JSON.parse(data) as AIWorkbenchAgentStateStreamPayload
    if (eventType === 'agent_state_snapshot' && payload.state) {
      options.onSnapshot(payload.state, payload)
    } else if (eventType === 'heartbeat') {
      options.onHeartbeat?.(payload)
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let frameEnd = buffer.indexOf('\n\n')
    while (frameEnd >= 0) {
      const frame = buffer.slice(0, frameEnd)
      buffer = buffer.slice(frameEnd + 2)
      consumeFrame(frame)
      frameEnd = buffer.indexOf('\n\n')
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) {
    consumeFrame(buffer)
  }
}

export async function streamAIWorkbenchPenTestSessionAgentState(
  taskId: string,
  options: {
    signature?: string
    intervalMs?: number
    signal?: AbortSignal
    onOpen?: () => void
    onSnapshot: (state: AIWorkbenchAgentState, payload: AIWorkbenchAgentStateStreamPayload) => void
    onHeartbeat?: (payload: AIWorkbenchAgentStateStreamPayload) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  if (options.signature) query.set('signature', options.signature)
  if (options.intervalMs) query.set('intervalMs', String(options.intervalMs))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/agent-state/stream${suffix}`, {
    cache: 'no-store',
    headers: { Accept: 'text/event-stream' },
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new WorkbenchRequestError('streamFailed', { status: response.status })
  }
  options.onOpen?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    const eventType = frame
      .split('\n')
      .find((line) => line.startsWith('event:'))
      ?.slice('event:'.length)
      .trim()
    if (eventType !== 'agent_state_snapshot' && eventType !== 'heartbeat') return

    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice('data:'.length).trimStart())
      .join('\n')
    if (!data) return
    const payload = JSON.parse(data) as AIWorkbenchAgentStateStreamPayload
    if (eventType === 'agent_state_snapshot' && payload.state) {
      options.onSnapshot(payload.state, payload)
    } else if (eventType === 'heartbeat') {
      options.onHeartbeat?.(payload)
    }
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let frameEnd = buffer.indexOf('\n\n')
    while (frameEnd >= 0) {
      const frame = buffer.slice(0, frameEnd)
      buffer = buffer.slice(frameEnd + 2)
      consumeFrame(frame)
      frameEnd = buffer.indexOf('\n\n')
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) {
    consumeFrame(buffer)
  }
}

export async function listAIWorkbenchPenTestSessionToolExecutions(
  taskId: string,
  params: { status?: string; stepId?: string; limit?: number } = {},
): Promise<AIWorkbenchToolExecution[]> {
  const query = new URLSearchParams()
  if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.stepId) query.set('stepId', params.stepId)
  if (params.limit) query.set('limit', String(params.limit))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/executions${suffix}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchToolExecution[]>(response)
}

export async function getAIWorkbenchPenTestSessionToolExecution(
  taskId: string,
  executionId: string,
): Promise<AIWorkbenchToolExecution> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(taskId)}/executions/${encodeURIComponent(executionId)}`,
    { cache: 'no-store' },
  )
  return parseAPIResponse<AIWorkbenchToolExecution>(response)
}

export async function waitAIWorkbenchPenTestSessionToolExecution(
  taskId: string,
  executionId: string,
  timeoutMs = 1000,
): Promise<AIWorkbenchToolExecutionWaitResponse> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(taskId)}/executions/${encodeURIComponent(executionId)}/wait`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ timeoutMs }),
    },
  )
  const data = await response.json().catch(() => null)
  if (!response.ok || !data?.success) {
    throw data?.error || data?.message ? new Error(data.error || data.message) : new WorkbenchRequestError('requestFailed', { status: response.status })
  }
  return {
    done: Boolean(data.done),
    timedOut: Boolean(data.timedOut),
    execution: data.data as AIWorkbenchToolExecution,
  }
}

export async function cancelAIWorkbenchPenTestSessionToolExecution(
  taskId: string,
  executionId: string,
): Promise<AIWorkbenchToolExecution> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(taskId)}/executions/${encodeURIComponent(executionId)}/cancel`,
    { method: 'POST' },
  )
  return parseAPIResponse<AIWorkbenchToolExecution>(response)
}

export async function approveAIWorkbenchStep(
  taskId: string,
  stepId: string,
  approved: boolean,
): Promise<AIWorkbenchTaskDetail> {
  const response = await fetchWithAuth(
    `/api/v1/ai/tasks/${encodeURIComponent(taskId)}/${approved ? 'approve' : 'reject'}`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ stepId }),
    },
  )
  return parseAPIResponse<AIWorkbenchTaskDetail>(response)
}

export async function sendAIWorkbenchMessage(taskId: string, content: string): Promise<AIWorkbenchTaskDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/message`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ content }),
  })
  return parseAPIResponse<AIWorkbenchTaskDetail>(response)
}

export async function sendAIWorkbenchPenTestSessionMessage(
  taskId: string,
  content: string,
  options: { parentMessageId?: string; agentProviderId?: string; agentModel?: string; metadata?: Record<string, unknown> } = {},
): Promise<AIWorkbenchPenTestSessionSnapshot> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/message`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      content,
      parentMessageId: options.parentMessageId || '',
      agentProviderId: options.agentProviderId || '',
      agentModel: options.agentModel || '',
      metadata: options.metadata || {},
    }),
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionSnapshot>(response)
}

export async function runAIWorkbenchPenTestSessionAction(
  taskId: string,
  action: Exclude<AIWorkbenchPenTestSessionActionName, 'send_message'>,
  options: { stepId?: string; findingId?: string; reviewQueue?: string; status?: string; targetStatus?: string; note?: string } = {},
): Promise<AIWorkbenchPenTestSessionSnapshot> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/action`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      action,
      stepId: options.stepId || '',
      findingId: options.findingId || '',
      reviewQueue: options.reviewQueue || '',
      status: options.status || '',
      targetStatus: options.targetStatus || '',
      note: options.note || '',
    }),
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionSnapshot>(response)
}

export type AIWorkbenchPenTestSessionLifecycleActionName = 'plan' | 'replan' | 'start' | 'pause' | 'resume' | 'cancel'

export async function runAIWorkbenchPenTestSessionLifecycleAction(
  taskId: string,
  action: AIWorkbenchPenTestSessionLifecycleActionName,
): Promise<AIWorkbenchPenTestSessionSnapshot> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/${action}`, {
    method: 'POST',
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionSnapshot>(response)
}

export async function streamAIWorkbenchMessage(
  taskId: string,
  content: string,
  options: {
    parentMessageId?: string
    metadata?: Record<string, unknown>
    signal?: AbortSignal
    onOpen?: () => void
    onEvent: (event: AIWorkbenchMessageStreamEvent) => void
  },
): Promise<void> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/message/stream`, {
    method: 'POST',
    cache: 'no-store',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      content,
      parentMessageId: options.parentMessageId || '',
      metadata: options.metadata || {},
    }),
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    throw new WorkbenchRequestError('messageStreamFailed', { status: response.status })
  }
  options.onOpen?.()

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  const consumeFrame = (frame: string) => {
    const eventType = frame
      .split('\n')
      .find((line) => line.startsWith('event:'))
      ?.slice('event:'.length)
      .trim()
    const data = frame
      .split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice('data:'.length).trimStart())
      .join('\n')
    if (!eventType || !data) return
    options.onEvent({
      type: eventType,
      data: JSON.parse(data) as AIWorkbenchMessageStreamPayload,
    })
  }

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let frameEnd = buffer.indexOf('\n\n')
    while (frameEnd >= 0) {
      const frame = buffer.slice(0, frameEnd)
      buffer = buffer.slice(frameEnd + 2)
      consumeFrame(frame)
      frameEnd = buffer.indexOf('\n\n')
    }
  }
  buffer += decoder.decode()
  if (buffer.trim()) {
    consumeFrame(buffer)
  }
}

export async function streamAIWorkbenchSideChat(
  messages: AIWorkbenchSideChatMessage[],
  options: {
    sessionId: string
    selectedText?: string
    agentProviderId?: string
    agentModel?: string
    signal?: AbortSignal
    onOpen?: () => void
    onDelta: (delta: string, accumulated: string) => void
  },
): Promise<string> {
  const response = await fetchWithAuth('/api/v1/ai/side-chat/stream', {
    method: 'POST',
    cache: 'no-store',
    headers: { Accept: 'text/event-stream', 'Content-Type': 'application/json' },
    body: JSON.stringify({
      messages,
      sessionId: options.sessionId,
      selectedText: options.selectedText || '',
      agentProviderId: options.agentProviderId || '',
      agentModel: options.agentModel || '',
    }),
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    const result = await response.json().catch(() => ({}))
    throw result.error ? new Error(result.error) : new WorkbenchRequestError('sideStreamFailed', { status: response.status })
  }
  options.onOpen?.()
  let streamError: Error | null = null
  let doneContent = ''
  await readAIProcessFrames(response, (type, data) => {
    if (type === 'delta') options.onDelta(String(data.delta || ''), String(data.accumulated || ''))
    // Some providers/proxies deliver only the final frame. Keep the server's
    // authoritative answer so a completed request is never treated as empty.
    if (type === 'done') doneContent = String(data.content || '')
    if (type === 'error') streamError = data.error ? new Error(String(data.error)) : new WorkbenchRequestError('sideChatFailed')
  })
  if (streamError) throw streamError
  return doneContent
}

export async function getAIWorkbenchSideChatHistory(
  sessionId: string,
  options: { signal?: AbortSignal } = {},
): Promise<AIWorkbenchSideChatMessage[]> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/side-chat`, {
    cache: 'no-store',
    signal: options.signal,
  })
  return parseAPIResponse<AIWorkbenchSideChatMessage[]>(response)
}

export async function saveAIWorkbenchSideChatHistory(
  sessionId: string,
  messages: AIWorkbenchSideChatMessage[],
): Promise<AIWorkbenchSideChatMessage[]> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/side-chat`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ messages }),
  })
  return parseAPIResponse<AIWorkbenchSideChatMessage[]>(response)
}

export async function streamAIWorkbenchPenTestSessionMessage(
  taskId: string,
  content: string,
  options: {
    parentMessageId?: string
    editMessageId?: string
    agentProviderId?: string
    agentModel?: string
    metadata?: Record<string, unknown>
    signal?: AbortSignal
    onOpen?: () => void
    onEvent: (event: AIWorkbenchMessageStreamEvent) => void
  },
): Promise<void> {
  const query = new URLSearchParams()
  const cursor = workbenchProgressCursors.get(taskId)
  if (cursor) query.set('lastEventId', String(cursor))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/message/stream${suffix}`, {
    method: 'POST',
    cache: 'no-store',
    headers: {
      Accept: 'text/event-stream',
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      content,
      editMessageId: options.editMessageId || '',
      parentMessageId: options.parentMessageId || '',
      agentProviderId: options.agentProviderId || '',
      agentModel: options.agentModel || '',
      metadata: options.metadata || {},
    }),
    signal: options.signal,
  })
  if (!response.ok || !response.body) {
    if (!response.ok) {
      const payload = await response.json().catch((error: unknown) => {
        if (error instanceof Error && error.name === 'AbortError') throw error
        return null
      })
      const reason = [payload?.error, payload?.message]
        .find((value): value is string => typeof value === 'string' && Boolean(value.trim()))
      if (reason) {
        throw new WorkbenchRequestError('messageStreamFailedWithReason', { status: response.status, reason: reason.trim() })
      }
    }
    throw new WorkbenchRequestError('messageStreamFailed', { status: response.status })
  }
  options.onOpen?.()

  await readAIProcessFrames(response, (type, data) => {
    rememberWorkbenchCursor(taskId, data.cursor)
    if (type === 'error') {
      throw new AIWorkbenchStreamError(String(data.error || data.message || 'AI stream failed'), { fatal: true, status: 200 })
    }
    if (type === 'done') return
    options.onEvent({ type, data: data as AIWorkbenchMessageStreamPayload })
  })
}

export interface AIWorkbenchSupersededArtifact {
  type: 'fact' | 'finding' | 'tool' | 'intent' | string
  id: string
  title: string
  detail: string
  occurredAt: string
  supersededAt: string
}

export async function getAIWorkbenchSupersededHistory(sessionId: string): Promise<AIWorkbenchSupersededArtifact[]> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/history`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchSupersededArtifact[]>(response)
}

export async function getAIWorkbenchMessageImages(sessionId: string, messageId: string): Promise<string[]> {
  const response = await fetchWithAuth(
    `/api/v1/ai/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}/images`,
    { cache: 'no-store' },
  )
  return parseAPIResponse<string[]>(response)
}

export async function getAIWorkbenchReport(taskId: string): Promise<AIWorkbenchReport> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/report`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchReport>(response)
}

export async function listAIWorkbenchPenTestSessionEvidence(taskId: string): Promise<AIWorkbenchEvidenceResponse> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/evidence`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchEvidenceResponse>(response)
}

export async function getAIWorkbenchPenTestSessionReport(taskId: string): Promise<AIWorkbenchReport> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/report`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchReport>(response)
}

export type AIWorkbenchReportExportFormat = 'markdown' | 'html'

export interface AIWorkbenchReportExportResult {
  blob: Blob
  filename: string
}

async function readAIWorkbenchExportResponse(response: Response, fallbackFilename: string): Promise<AIWorkbenchReportExportResult> {
  if (!response.ok) {
    let message = `Report export failed (${response.status})`
    try {
      const payload = await response.json() as { error?: string; message?: string }
      message = payload.error || payload.message || message
    } catch { /* keep status fallback */ }
    throw new Error(message)
  }
  return {
    blob: await response.blob(),
    filename: parseFilenameFromContentDisposition(response.headers.get('content-disposition')) || fallbackFilename,
  }
}

export async function exportAIWorkbenchProjectReports(
  projectIds: string[],
  fileName?: string,
  format: AIWorkbenchReportExportFormat = 'markdown',
): Promise<AIWorkbenchReportExportResult> {
  const response = await fetchWithAuth('/api/v1/ai/reports/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ projectIds, fileName: fileName || '', format }),
  })
  return readAIWorkbenchExportResponse(response, `ai-workbench-project-report.${format === 'html' ? 'html' : 'md'}`)
}

export async function exportAIWorkbenchFindingReports(
  findingIds: string[],
  fileName?: string,
  format: AIWorkbenchReportExportFormat = 'markdown',
): Promise<AIWorkbenchReportExportResult> {
  const response = await fetchWithAuth('/api/v1/ai/reports/findings', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ findingIds, fileName: fileName || '', format }),
  })
  return readAIWorkbenchExportResponse(response, `vulnerability-findings.${format === 'html' ? 'html' : 'md'}`)
}

export async function downloadAIWorkbenchPenTestSessionReport(taskId: string): Promise<{ blob: Blob; filename: string }> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/report/download`, { cache: 'no-store' })
  if (!response.ok) {
    let message = `Report download failed (${response.status})`
    try {
      const payload = await response.json() as { error?: string; message?: string }
      message = payload.error || payload.message || message
    } catch { /* keep status fallback */ }
    throw new Error(message)
  }
  return {
    blob: await response.blob(),
    filename: parseFilenameFromContentDisposition(response.headers.get('content-disposition')) || 'pentest-report.md',
  }
}

export async function getAIWorkbenchPenTestSessionBlackboard(taskId: string): Promise<AIWorkbenchPenTestSessionBlackboard> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/blackboard`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionBlackboard>(response)
}

export async function getAIWorkbenchPenTestExecutionEvidence(conversationId: string, executionId: string, signal?: AbortSignal): Promise<AIWorkbenchExecutionEvidence> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(conversationId)}/executions/${encodeURIComponent(executionId)}`, {
    cache: 'no-store',
    signal,
  })
  return parseAPIResponse<AIWorkbenchExecutionEvidence>(response)
}

export async function getAIWorkbenchPenTestProjectBlackboard(projectId: string): Promise<AIWorkbenchPenTestSessionBlackboard> {
  const response = await fetchWithAuth(`/api/v1/ai/projects/${encodeURIComponent(projectId)}/blackboard`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchPenTestSessionBlackboard>(response)
}

export interface AIWorkbenchFindingQueryParams {
  highRiskOnly?: boolean
  projectId?: string
  statuses?: AIWorkbenchFindingStatus[]
  severities?: string[]
  reviewQueues?: AIWorkbenchFindingReviewQueue[]
  taskId?: string
  agentRunId?: string
  sourceTaskId?: string
  sourceType?: string
  scanRef?: string
  severity?: string
  status?: string
  reviewStatus?: AIWorkbenchFindingReviewStatus
  confirmationStatus?: AIWorkbenchFindingConfirmationStatus
  reviewDisposition?: AIWorkbenchFindingReviewDisposition
  reviewVerdict?: AIWorkbenchFindingReviewVerdict
  reviewQueue?: AIWorkbenchFindingReviewQueue
  search?: string
  page?: number
  pageSize?: number
  limit?: number
}

function buildAIWorkbenchFindingQuery(params: AIWorkbenchFindingQueryParams = {}) {
  const query = new URLSearchParams()
  if (params.highRiskOnly) query.set('highRiskOnly', 'true')
  if (params.projectId) query.set('projectId', params.projectId)
  if (params.taskId) query.set('taskId', params.taskId)
  if (params.agentRunId) query.set('agentRunId', params.agentRunId)
  if (params.sourceTaskId) query.set('sourceTaskId', params.sourceTaskId)
  if (params.sourceType && params.sourceType !== 'all') query.set('sourceType', params.sourceType)
  if (params.scanRef) query.set('scanRef', params.scanRef)
  if (params.severities?.length) query.set('severity', params.severities.join(','))
  else if (params.severity && params.severity !== 'all') query.set('severity', params.severity)
  if (params.statuses?.length) query.set('status', params.statuses.join(','))
  else if (params.status && params.status !== 'all') query.set('status', params.status)
  if (params.reviewStatus && params.reviewStatus !== 'all') query.set('reviewStatus', params.reviewStatus)
  if (params.confirmationStatus && params.confirmationStatus !== 'all') query.set('confirmationStatus', params.confirmationStatus)
  if (params.reviewDisposition && params.reviewDisposition !== 'all') query.set('reviewDisposition', params.reviewDisposition)
  if (params.reviewVerdict && params.reviewVerdict !== 'all') query.set('reviewVerdict', params.reviewVerdict)
  if (params.reviewQueues?.length) query.set('reviewQueue', params.reviewQueues.join(','))
  else if (params.reviewQueue && params.reviewQueue !== 'all') query.set('reviewQueue', params.reviewQueue)
  if (params.search) query.set('search', params.search)
  if (params.page) query.set('page', String(params.page))
  if (params.pageSize) query.set('pageSize', String(params.pageSize))
  if (params.limit) query.set('limit', String(params.limit))
  return query
}

export async function listAIWorkbenchFindings(params: AIWorkbenchFindingQueryParams = {}): Promise<AIWorkbenchFindingsResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/findings${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingsResponse>(response)
}

export async function listAIWorkbenchPenTestSessionFindings(
  taskId: string,
  params: Omit<AIWorkbenchFindingQueryParams, 'taskId'> = {},
): Promise<AIWorkbenchFindingsResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/findings${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingsResponse>(response)
}

export async function listAIWorkbenchTaskFindings(
  taskId: string,
  params: Omit<AIWorkbenchFindingQueryParams, 'taskId'> = {},
): Promise<AIWorkbenchFindingsResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/findings${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingsResponse>(response)
}

export async function getAIWorkbenchFindingsOverview(params: AIWorkbenchFindingQueryParams = {}): Promise<AIWorkbenchFindingOverviewResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/findings/overview${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingOverviewResponse>(response)
}

export async function getAIWorkbenchFindingsManagement(params: AIWorkbenchFindingQueryParams = {}): Promise<AIWorkbenchFindingManagementResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/findings/management${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingManagementResponse>(response)
}

export async function getAIWorkbenchPenTestSessionFindingsOverview(
  taskId: string,
  params: Omit<AIWorkbenchFindingQueryParams, 'taskId'> = {},
): Promise<AIWorkbenchFindingOverviewResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/findings/overview${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingOverviewResponse>(response)
}

export async function getAIWorkbenchTaskFindingsOverview(
  taskId: string,
  params: Omit<AIWorkbenchFindingQueryParams, 'taskId'> = {},
): Promise<AIWorkbenchFindingOverviewResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/findings/overview${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingOverviewResponse>(response)
}

export async function getAIWorkbenchPenTestSessionFindingsManagement(
  taskId: string,
  params: Omit<AIWorkbenchFindingQueryParams, 'taskId'> = {},
): Promise<AIWorkbenchFindingManagementResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/findings/management${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingManagementResponse>(response)
}

export async function getAIWorkbenchTaskFindingsManagement(
  taskId: string,
  params: Omit<AIWorkbenchFindingQueryParams, 'taskId'> = {},
): Promise<AIWorkbenchFindingManagementResponse> {
  const query = buildAIWorkbenchFindingQuery(params)
  const suffix = query.toString() ? `?${query.toString()}` : ''
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/findings/management${suffix}`, { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchFindingManagementResponse>(response)
}

export async function getAIWorkbenchFinding(findingId: string): Promise<AIWorkbenchFindingReport> {
  const response = await fetchWithAuth(`/api/v1/ai/findings/${encodeURIComponent(findingId)}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchFindingReport>(response)
}

export async function getAIWorkbenchFindingLineage(findingId: string): Promise<AIWorkbenchFindingLineage> {
  const response = await fetchWithAuth(`/api/v1/ai/findings/${encodeURIComponent(findingId)}/lineage`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchFindingLineage>(response)
}

export async function getAIWorkbenchPenTestSessionFinding(taskId: string, findingId: string): Promise<AIWorkbenchFindingDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/findings/${encodeURIComponent(findingId)}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchFindingDetail>(response)
}

export async function getAIWorkbenchTaskFinding(taskId: string, findingId: string): Promise<AIWorkbenchFindingDetail> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/findings/${encodeURIComponent(findingId)}`, {
    cache: 'no-store',
  })
  return parseAPIResponse<AIWorkbenchFindingDetail>(response)
}

export async function updateAIWorkbenchFindingStatus(
  findingId: string,
  status: AIWorkbenchFindingStatus,
): Promise<AIWorkbenchVulnerabilityFinding> {
  const response = await fetchWithAuth(`/api/v1/ai/findings/${encodeURIComponent(findingId)}/status`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
  return parseAPIResponse<AIWorkbenchVulnerabilityFinding>(response)
}

export async function updateAIWorkbenchPenTestSessionFindingStatus(
  taskId: string,
  findingId: string,
  status: AIWorkbenchFindingStatus,
): Promise<AIWorkbenchVulnerabilityFinding> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/findings/${encodeURIComponent(findingId)}/status`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
  return parseAPIResponse<AIWorkbenchVulnerabilityFinding>(response)
}

export async function updateAIWorkbenchTaskFindingStatus(
  taskId: string,
  findingId: string,
  status: AIWorkbenchFindingStatus,
): Promise<AIWorkbenchVulnerabilityFinding> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/findings/${encodeURIComponent(findingId)}/status`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status }),
  })
  return parseAPIResponse<AIWorkbenchVulnerabilityFinding>(response)
}

export async function updateAIWorkbenchFindingStatuses(
  findingIds: string[],
  status: AIWorkbenchFindingStatus,
  note?: string,
): Promise<AIWorkbenchFindingBatchStatusResponse> {
  const response = await fetchWithAuth('/api/v1/ai/findings/status', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ findingIds, status, note }),
  })
  return parseAPIResponse<AIWorkbenchFindingBatchStatusResponse>(response)
}

export async function updateAIWorkbenchTaskFindingStatuses(
  taskId: string,
  findingIds: string[],
  status: AIWorkbenchFindingStatus,
  note?: string,
): Promise<AIWorkbenchFindingBatchStatusResponse> {
  const response = await fetchWithAuth(`/api/v1/ai/tasks/${encodeURIComponent(taskId)}/findings/status`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ findingIds, status, note }),
  })
  return parseAPIResponse<AIWorkbenchFindingBatchStatusResponse>(response)
}

export async function updateAIWorkbenchPenTestSessionFindingStatuses(
  taskId: string,
  findingIds: string[],
  status: AIWorkbenchFindingStatus,
  note?: string,
): Promise<AIWorkbenchFindingBatchStatusResponse> {
  const response = await fetchWithAuth(`/api/v1/ai/sessions/${encodeURIComponent(taskId)}/findings/status`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ findingIds, status, note }),
  })
  return parseAPIResponse<AIWorkbenchFindingBatchStatusResponse>(response)
}

export async function listAIWorkbenchEvaluationDisagreements(
  taskId: string,
  limit = 20,
): Promise<AIWorkbenchEvaluationDisagreementAudit> {
  const params = new URLSearchParams({ limit: String(limit) })
  const response = await fetchWithAuth(
    `/api/v1/ai/tasks/${encodeURIComponent(taskId)}/evaluation-disagreements?${params.toString()}`,
    { cache: 'no-store' },
  )
  return parseAPIResponse<AIWorkbenchEvaluationDisagreementAudit>(response)
}

export async function listAIWorkbenchTools(): Promise<AIWorkbenchTool[]> {
  const response = await fetchWithAuth('/api/v1/ai/tools', { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchTool[]>(response)
}

export async function listAIWorkbenchModelOptions(): Promise<AIWorkbenchModelOptionsResponse> {
  const response = await fetchWithAuth('/api/v1/ai/model-options', { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchModelOptionsResponse>(response)
}

export async function listAIWorkbenchStrategyPacks(): Promise<AIWorkbenchStrategyPack[]> {
  const response = await fetchWithAuth('/api/v1/ai/strategy-packs', { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchStrategyPack[]>(response)
}

export async function listAIWorkbenchNodes(): Promise<AIWorkbenchNode[]> {
  const response = await fetchWithAuth('/api/v1/ai/nodes', { cache: 'no-store' })
  return parseAPIResponse<AIWorkbenchNode[]>(response)
}
