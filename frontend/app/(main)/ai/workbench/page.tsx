'use client'

import { useI18n } from '@/lib/i18n'
import { formatWorkbenchText, localizeAssistantText, localizeBackendText, localizeWorkbenchActivity, workbenchErrorMessage, WorkbenchRequestError, type WorkbenchMessages } from '@/lib/ai-workbench-i18n'
import { useProjectSessions, type ProjectSessionPage } from './use-project-sessions'
import { SESSION_GROUP_VISIBLE_LIMIT, sessionGroupPagination } from './session-pagination'
import { TokenUsage, TokenUsageButton } from './components/TokenUsage'
import { ProjectTokenUsage } from './components/ProjectTokenUsage'
import { useRouter, useSearchParams } from 'next/navigation'

import Image from 'next/image'
import { Suspense, memo, type ChangeEvent, type ClipboardEvent, type KeyboardEvent, type ReactNode, type Ref, useCallback, useDeferredValue, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { renderMarkdownHTML } from '@/lib/markdown'
import { AlertTriangle, BrainCircuit, Check, ChevronDown, ChevronRight, CirclePause, Clock3, Copy, FileText, Folder, FolderOpen, GitBranch, Goal, Images, ListTodo, Loader2, MessageSquare, MessageSquarePlus, MessageSquareQuote, Monitor, MoreHorizontal, PanelLeftClose, PanelLeftOpen, Pencil, Plus, RotateCw, Search, Send, ShieldBan, ShieldCheck, Sparkles, Square, SquareTerminal, Text, Trash2, UserRoundCog, Wrench, X, XCircle } from 'lucide-react'
import toast from '@/lib/toast'
import { copyToClipboard } from '@/lib/clipboard'
import { getAIConfig, saveAIConfig } from '@/lib/api/ai-config'
import { cn } from '@/lib/utils'
import { AIProcessMerger, scheduleProcessFrame, type AIProcessUpdate } from '@/lib/ai-process-stream'
import { activityAnimationStates } from '@/lib/ai-activity-state'
import { foldModelRetryActivities, modelRetryNotice, type ModelRetryNotice } from '@/lib/ai-retry-notice'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Marker, MarkerContent, MarkerIcon } from '@/components/ui/marker'
import { Spinner } from '@/components/ui/spinner'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@/components/ui/resizable'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import {
  createAIWorkbenchPenTestSession,
  createAIWorkbenchPenTestProject,
  deleteAIWorkbenchPenTestProject,
  deleteAIWorkbenchPenTestSession,
  getAIWorkbenchPenTestSession,
  getAIWorkbenchEarlierRounds,
  getAIWorkbenchSessionTurn,
  getAIWorkbenchPenTestSessionBootstrap,
  downloadAIWorkbenchPenTestSessionReport,
  getAIWorkbenchFinding,
  getAIWorkbenchMessageImages,
  getAIWorkbenchSupersededHistory,
  type AIWorkbenchSupersededArtifact,
  listAIWorkbenchPenTestSessions,
  listAIWorkbenchPenTestProjects,
  getAIWorkbenchSideChatHistory,
  runAIWorkbenchPenTestSessionLifecycleAction,
  streamAIWorkbenchPenTestSession,
  streamAIWorkbenchPenTestSessionMessage,
  streamAIWorkbenchSideChat,
  saveAIWorkbenchSideChatHistory,
  updateAIWorkbenchPenTestProject,
  updateAIWorkbenchPenTestSession,
  type AIWorkbenchAgentRun,
  type AIWorkbenchBlockedMatch,
  type AIWorkbenchChatItem,
  type AIWorkbenchConversationSummary,
  type AIWorkbenchConversationTurn,
  type AIWorkbenchModelOption,
  AIWorkbenchStreamError,
  type AIWorkbenchPenTestSessionBootstrapResponse,
  type AIWorkbenchPenTestSessionSnapshot,
  type AIWorkbenchProject,
  type AIWorkbenchSideChatMessage,
} from '@/lib/api/ai-workbench'
import { listAIBatchQueues } from '@/lib/api/ai-batch'
import { ensureAIPlanRunController, isOrchestrationRunId, listAIPlanRunSummaries } from '@/lib/api/ai-plan'
import { turnPreviewText, turnResultPreview } from '@/lib/ai-turn-preview'
import { ConversationRail } from './components/ConversationRail'
import { mergeConversationTurns, readingTurnId } from './conversation-history'
import { ComposerCommandMenu, composerCommandAt, type ComposerCommand, type ComposerCommandMenuHandle } from './components/ComposerCommandMenu'
import type { ReferenceTask } from '@/lib/api/ai-asset-references'
import { listCallableMCPTools, messageWithMCPTools, mcpToolTitle, type CallableMCPTool } from '@/lib/api/ai-mcp-tools'
import { GoalModeCard } from './components/GoalModeCard'
import { ComposerQueuedMessages } from './components/ComposerQueuedMessages'
import { loadQueuedMessages, saveQueuedMessages, workbenchQueueOwnerKey } from '@/lib/ai-workbench-queue-store'
import { goalTurnMessages, type GoalModeAction, type GoalModeRequest, type GoalRun } from '@/lib/ai-goal-mode'
import { AssetReferenceChips, AssetReferencePicker } from './components/AssetReferencePicker'
import type { AssetReferenceSelection, AssetReferenceSnapshot } from '@/lib/api/ai-asset-references'

const targetPattern = /\b(?:https?:\/\/[^\s，。；,;]+|(?:\d{1,3}\.){3}\d{1,3}(?:\/\d{1,2})?|[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+)\b/gi

function extractScopeTargets(content: string) {
  return Array.from(new Set(content.match(targetPattern) || []))
}
// Keep this list aligned with backend pentestStatusIsActive. Scan waiting is
// represented as an idle conversation while its continuation runs in the
// background.
const activeStatuses = new Set(['queued', 'cancelling', 'recovering', 'running', 'draining', 'waiting_tool', 'waiting_approval', 'pending'])
const activeBatchQueueStatuses = new Set(['pending', 'running', 'pausing'])
const maxAttachmentSize = 8 * 1024 * 1024
const maxTotalAttachmentSize = 24 * 1024 * 1024

function formatTokenMillions(total: number) {
  if (total === 0) return '0M'
  const millions = total / 1_000_000
  return millions < 0.01 ? '<0.01M' : `${millions.toFixed(millions < 1 ? 2 : 1)}M`
}

type ComposerAttachment = { id: string; name: string; type: string; size: number; dataUrl: string }
type MessageQuote = { content: string; role?: 'user' | 'assistant' }
type QueuedWorkbenchMessage = { id: string; sessionId: string; content: string; attachments: ComposerAttachment[]; assetReferences?: AssetReferenceSelection[]; goalMode?: GoalModeRequest; agentProviderId: string; agentModel: string; status?: 'queued' | 'failed' | 'paused' }
type ComposerSnapshot = {
  draft: string
  quote: MessageQuote | null
  attachments: ComposerAttachment[]
  assetReferences: AssetReferenceSelection[]
  mcpTools: CallableMCPTool[]
}
type SessionSideChatMessage = AIWorkbenchSideChatMessage & { id: string }
type ThinkingActivity = { id?: string; eventType?: string; toolName?: string; title: string; detail?: string; status?: string; type?: AIWorkbenchChatItem['type']; blockedMatch?: AIWorkbenchBlockedMatch | null; blockedPhase?: string; retry?: ModelRetryNotice }

function displayText(value?: string | null) { return (value || '').trim() }
function compactText(value?: string | null, max = 96) {
  const text = displayText(value).replace(/\s+/g, ' ')
  return text.length <= max ? text : `${text.slice(0, max - 1)}…`
}
function turnElementId(turnId: string) { return `ai-workbench-turn-${turnId}` }
function modelValue(option?: AIWorkbenchModelOption | null) { return option ? `${option.providerId}::${option.model}` : '' }
function splitModelValue(value: string) { const [providerId = '', model = ''] = value.split('::'); return { providerId, model } }
function defaultWorkbenchModelValue(options: AIWorkbenchModelOption[]) {
  const available = (option: AIWorkbenchModelOption) => option.configured && option.enabled
  const selected = options.find((option) => option.defaultForWorkbench && available(option))
    || options.find(available)
    || options.find((option) => option.defaultForWorkbench && option.configured)
    || options.find((option) => option.configured)
  return modelValue(selected)
}
function formatDateTime(value?: string | null, language: 'zh' | 'en' = 'zh') {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return new Intl.DateTimeFormat(language === 'zh' ? 'zh-CN' : 'en-US', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false,
  }).format(date)
}
function formatDuration(milliseconds: number, l: WorkbenchMessages) {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1000))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  return minutes ? formatWorkbenchText(l.durationMinutes, { minutes, seconds }) : formatWorkbenchText(l.durationSeconds, { seconds })
}
function getRunDuration(run: AIWorkbenchAgentRun | null | undefined, now: number) {
  if (!run) return 0
  if (run.durationMs > 0 && !activeStatuses.has(run.status)) return run.durationMs
  const startedAt = new Date(run.startedAt || run.createdAt).getTime()
  const completedAt = run.completedAt ? new Date(run.completedAt).getTime() : now
  return Number.isNaN(startedAt) || Number.isNaN(completedAt) ? run.durationMs || 0 : Math.max(0, completedAt - startedAt)
}
function runIsActive(run?: AIWorkbenchAgentRun | null) { return Boolean(run && activeStatuses.has(displayText(run.status).toLowerCase())) }
function sessionIsActive(session: AIWorkbenchPenTestSessionSnapshot | null) { return Boolean(session && activeStatuses.has(displayText(session.task.status).toLowerCase())) }
function attachmentId() { return typeof crypto !== 'undefined' && 'randomUUID' in crypto ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(16).slice(2)}` }
function messageContentWithQuote(content: string, quote: MessageQuote | null, l: WorkbenchMessages) {
  const body = content.trim()
  const quoted = quote?.content.trim()
  if (!quoted) return body
  const quoteBlock = formatWorkbenchText(quote?.role === 'assistant' ? l.quotedAssistantMessage : l.quotedUserMessage, { content: quoted.split('\n').map((line) => `> ${line}`).join('\n') })
  return body ? `${quoteBlock}\n\n${body}` : quoteBlock
}
function optimisticActivity() { return { type: 'message', status: 'running', label: 'Agent 正在思考并执行低影响验证' } }
function optimisticPhase() { return { id: 'running', key: 'running', status: 'running', label: '思考中', description: 'Agent 正在推进本轮测试', nextAction: 'wait' } }
function optimisticRun(taskId: string, model: string, objective: string, timestamp: string): AIWorkbenchAgentRun {
  return { id: `${taskId}-optimistic-run-${Date.now()}`, taskId, parentRunId: '', runIndex: 1, triggerType: 'message', agentModel: model, status: 'running', goal: objective, summary: 'Agent 正在理解目标、调用工具并更新黑板', currentStepId: '', startedAt: timestamp, completedAt: null, durationMs: 0, metadata: { source: 'optimistic' }, createdAt: timestamp, updatedAt: timestamp }
}
function optimisticTurn(taskId: string, content: string, run: AIWorkbenchAgentRun, timestamp: string): AIWorkbenchConversationTurn {
  const id = `optimistic-${attachmentId()}`
  const message = { id, taskId, parentMessageId: '', role: 'user', content, metadata: null, createdAt: timestamp }
  return { id, userMessage: message, userMessageView: { role: 'user', label: compactText(content), content }, assistantMessages: [], assistantMessageViews: [], run, runSummary: null, items: [], chatItems: [], startedAt: timestamp, updatedAt: timestamp }
}
function buildOptimisticSession(content: string, target: string, selectedModelValue: string): AIWorkbenchPenTestSessionSnapshot {
  const timestamp = new Date().toISOString()
  const { providerId, model } = splitModelValue(selectedModelValue)
  const taskId = `local-${attachmentId()}`
  const activity = optimisticActivity()
  const phase = optimisticPhase()
  const run = optimisticRun(taskId, model, target, timestamp)
  const task = { id: taskId, name: target || compactText(content, 42) || 'AI 安全测试', target, scope: [], mode: 'solo_pentest', agentModel: model, agentProviderId: providerId, status: 'running', riskPolicy: 'manual_approval', objective: target || compactText(content, 96), successCriteria: '产出可复现证据与清晰结论', deliveryFormat: ['markdown'], strategyPacks: ['solo-pentest'], currentPlan: [], summary: '', createdBy: '', createdAt: timestamp, updatedAt: timestamp }
  const turn = optimisticTurn(taskId, content, run, timestamp)
  return { sessionMode: 'solo_pentest', entrypoint: 'ai_penetration_workbench', task, target: { target, scope: [], mode: 'solo_pentest', riskPolicy: 'manual_approval', objective: task.objective, successCriteria: task.successCriteria }, model: { agentModel: model, agentProviderId: providerId }, launchPlan: { target, scope: [], mode: 'solo_pentest', riskPolicy: 'manual_approval', agentProviderId: providerId, agentModel: model, modelConfigured: true, supportsTools: true, supportsStreaming: true, autoStart: true, initialMessage: content, preferredStrategies: ['solo-pentest'], preferredCapabilities: ['system-tools', 'blackboard'], phases: ['observe', 'orient', 'verify', 'report'], initialActions: [] }, agentState: { task, activeRun: run, latestRun: null, currentActivity: activity, pendingApprovals: [], nextPlanTasks: [], runningToolCalls: [], runningExecutions: [], openFindings: [], phase, recommendedAction: 'send_message', blockingReason: '', canStart: false, canResume: false, canApprove: false, canSendMessage: false, summary: '', updatedAt: timestamp }, currentActivity: activity, primaryAction: { id: 'send_message', action: 'send_message', label: '继续对话', description: '', variant: 'primary', enabled: true }, actions: [], materials: { assetBuckets: [], apiEndpoints: [], apiRelations: [], authProfiles: [], variables: [], httpRequests: [], assetBucketCount: 0, apiEndpointCount: 0, apiRelationCount: 0, authProfileCount: 0, variableCount: 0, httpRequestCount: 0, evidenceCount: 0, findingCount: 0 }, timeline: { task, items: [], chatItems: [], runs: [], turns: [turn] }, runs: [], turns: [turn], chatItems: [], evidence: [], findings: [], findingViews: [], findingOverview: { total: 0, riskBreakdown: '', actionSummary: { actionableTotal: 0, openTotal: 0, inProgressTotal: 0, resolvedTotal: 0, criticalHighOpenTotal: 0, needsReviewTotal: 0, candidateTotal: 0, manualConfirmationTotal: 0, authRetryTotal: 0, promotedResultTotal: 0, authExcludedTotal: 0 }, severityDistribution: {}, statusDistribution: {}, confirmationStatusDistribution: {}, reviewDispositionDistribution: {}, reviewVerdictDistribution: {}, byTask: [], byTarget: [], bySource: [], byAgentRun: [], byReviewQueue: [], recent: [] }, findingManagement: { taskId, scope: 'session', defaultReviewQueue: 'all', defaultFilters: {}, overview: { total: 0, riskBreakdown: '', actionSummary: { actionableTotal: 0, openTotal: 0, inProgressTotal: 0, resolvedTotal: 0, criticalHighOpenTotal: 0, needsReviewTotal: 0, candidateTotal: 0, manualConfirmationTotal: 0, authRetryTotal: 0, promotedResultTotal: 0, authExcludedTotal: 0 }, severityDistribution: {}, statusDistribution: {}, confirmationStatusDistribution: {}, reviewDispositionDistribution: {}, reviewVerdictDistribution: {}, byTask: [], byTarget: [], bySource: [], byAgentRun: [], byReviewQueue: [], recent: [] }, queues: [], actions: [], recent: [] }, reportPreview: { format: 'markdown', content: '', conclusion: 'Agent 正在思考', riskBreakdown: '', findingCount: 0 }, updatedAt: timestamp }
}
function appendOptimisticTurn(session: AIWorkbenchPenTestSessionSnapshot | null, content: string): AIWorkbenchPenTestSessionSnapshot | null {
  if (!session) return session
  const timestamp = new Date().toISOString()
  const activity = optimisticActivity()
  const phase = optimisticPhase()
  const run = optimisticRun(session.task.id, session.task.agentModel, session.task.objective, timestamp)
  const turn = optimisticTurn(session.task.id, content, run, timestamp)
  const task = { ...session.task, status: 'running', updatedAt: timestamp }
  return { ...session, task, agentState: { ...session.agentState, task, activeRun: run, currentActivity: activity, phase, canSendMessage: false, updatedAt: timestamp }, currentActivity: activity, turns: [...(session.turns || []), turn], timeline: { ...session.timeline, task, turns: [...(session.timeline?.turns || session.turns || []), turn] }, updatedAt: timestamp }
}

function activityLabel(item: AIWorkbenchChatItem) {
  switch (item.type) {
    case 'intent': return compactText(item.intent?.label || item.intent?.summary || item.label, 72)
    case 'target': return '正在识别目标服务与攻击面'
    case 'tool': return compactText(item.label || (item.tool?.status === 'running' ? `正在调用工具：${item.tool?.name || '安全验证'}` : `工具调用完成：${item.tool?.name || '安全验证'}`), 72)
    case 'finding': return '发现一条待复核的安全线索'
    case 'error': return '执行过程中遇到问题，正在调整策略'
    case 'marker': return compactText(item.marker?.label || item.marker?.description || item.label, 72)
    default: return compactText(item.label || item.description, 72)
  }
}
function activityDetail(item: AIWorkbenchChatItem) {
  if (['reasoning', 'assistant_commentary'].includes(String(item.metadata?.eventType))) return item.description || item.marker?.description || ''
  const detail = item.tool?.errorMessage || item.tool?.outputSummary || item.tool?.inputSummary || item.marker?.description || item.description
  const title = activityLabel(item)
  const text = compactText(detail, 520)
  return text && text !== title ? text : ''
}
function activityStatus(item: AIWorkbenchChatItem) {
  return displayText(item.tool?.status || item.marker?.status || item.status).toLowerCase()
}
function activityEntry(item: AIWorkbenchChatItem): ThinkingActivity | null {
  if (item.type === 'message') return null
  const eventType = String(item.metadata?.eventType || item.marker?.eventType || '')
  // Iteration markers are internal orchestration ticks, never part of the visible stream.
  if (eventType === 'iteration') return null
  const title = activityLabel(item)
  if (!title) return null
  const detail = activityDetail(item)
  const retry = eventType === 'model_retry'
    ? modelRetryNotice(item.marker?.description || item.description || detail, item.metadata?.attempt, item.metadata?.maxRetries)
    : null
  return { id: item.id, eventType, toolName: item.tool?.name, title, detail, status: activityStatus(item), type: item.type, blockedMatch: item.tool?.blockedMatch || null, blockedPhase: item.tool?.blockedPhase, retry: retry || undefined }
}
function getActivities(items?: AIWorkbenchChatItem[] | null) {
  const activities: ThinkingActivity[] = []
  const seen = new Set<string>()
  for (const item of items || []) {
    const activity = activityEntry(item)
    if (!activity) continue
    const key = activity.id || `${activity.title}\n${activity.detail || ''}\n${activity.status || ''}`
    if (seen.has(key)) continue
    seen.add(key)
    activities.push(activity)
  }
  return foldModelRetryActivities(activities)
}
function completedActivityTitle(activity: ThinkingActivity) {
  if (activity.type === 'tool' && activity.title.startsWith('正在调用工具')) {
    return activity.title.replace(/^正在调用工具/, '工具调用完成')
  }
  if (activity.title.startsWith('正在基于已有证据综合结论')) return '已完成证据综合'
  return activity.title
}
function turnFailureText(turn: AIWorkbenchConversationTurn) {
  const assistantMessages = [...(turn.assistantMessageViews || []), ...(turn.assistantMessages || [])]
  for (let index = assistantMessages.length - 1; index >= 0; index -= 1) {
    const content = displayText(assistantMessages[index]?.content)
    if (content.includes('执行失败')) return content
  }
  return ''
}
function normalizeTurnActivities(activities: ThinkingActivity[], allowActive: boolean) {
  if (allowActive) return activities
  return activities.map((activity) => activity.status === 'running' ? { ...activity, title: completedActivityTitle(activity), status: 'completed' } : activity)
}
function turnActivities(turn: AIWorkbenchConversationTurn, allowActive = true) {
  const activities = normalizeTurnActivities(getActivities(turn.chatItems), allowActive)
  const failure = turnFailureText(turn)
  if (failure && !activities.some((activity) => activity.status === 'failed' || activity.title.includes('执行失败'))) {
    activities.push({ title: '本轮执行失败', detail: compactText(failure, 520), status: 'failed' })
  }
  return activities
}
function inferTurnRun(turn: AIWorkbenchConversationTurn, activities: ThinkingActivity[], sessionStatus?: string, allowActive = true): AIWorkbenchAgentRun | null {
  const failure = turnFailureText(turn)
  if (turn.run) {
    if (!runIsActive(turn.run) || (allowActive && !failure)) return turn.run
    const startedAt = turn.run.startedAt || turn.startedAt || turn.userMessage?.createdAt || new Date().toISOString()
    const completedAt = turn.updatedAt || turn.run.updatedAt || turn.run.completedAt || startedAt
    const started = new Date(startedAt).getTime()
    const completed = new Date(completedAt).getTime()
    const durationMs = turn.run.durationMs > 0 ? turn.run.durationMs : Number.isNaN(started) || Number.isNaN(completed) ? 0 : Math.max(0, completed - started)
    return { ...turn.run, status: failure ? 'failed' : sessionStatus === 'cancelled' ? 'cancelled' : 'completed', completedAt, durationMs, updatedAt: completedAt }
  }
  if (!activities.length) return null
  const hasRunning = activities.some((activity) => activity.status === 'running')
  const normalizedSessionStatus = displayText(sessionStatus).toLowerCase()
  const sessionActive = activeStatuses.has(normalizedSessionStatus)
  const canUseSessionActive = allowActive && sessionActive
  const status = failure ? 'failed' : hasRunning && canUseSessionActive ? 'running' : (normalizedSessionStatus === 'failed' || normalizedSessionStatus === 'cancelled') ? normalizedSessionStatus : 'completed'
  const startedAt = turn.startedAt || turn.userMessage?.createdAt || new Date().toISOString()
  const updatedAt = turn.updatedAt || startedAt
  const started = new Date(startedAt).getTime()
  const updated = new Date(updatedAt).getTime()
  const durationMs = status === 'running' || Number.isNaN(started) || Number.isNaN(updated) ? 0 : Math.max(0, updated - started)
  return {
    id: `${turn.id}-reconstructed-run`,
    taskId: turn.userMessage?.taskId || '',
    parentRunId: '',
    runIndex: 1,
    triggerType: 'message',
    agentModel: '',
    status,
    goal: turn.userMessageView?.content || turn.userMessage?.content || '',
    summary: activities[activities.length - 1]?.title || '',
    currentStepId: '',
    startedAt,
    completedAt: status === 'running' ? null : updatedAt,
    durationMs,
    metadata: { source: 'reconstructed_from_pentest_messages' },
    createdAt: startedAt,
    updatedAt,
  }
}
function turnHasActiveThinking(turn: AIWorkbenchConversationTurn, sessionActive = true, allowActive = true) {
  if (turnFailureText(turn)) return false
  if (runIsActive(turn.run)) return sessionActive && allowActive
  if (!sessionActive || !allowActive) return false
  return getActivities(turn.chatItems).some((activity) => activity.status === 'running')
}
function sessionTitle(summary: AIWorkbenchConversationSummary | null | undefined, l: WorkbenchMessages) {
  return compactText(summary?.task.target || summary?.task.name || summary?.latestMessage?.content || l.unnamedTarget, 42)
}
function projectFromSummary(summary: AIWorkbenchConversationSummary, fallbackName = 'Untitled project'): AIWorkbenchProject | null {
  const project = summary.project
  const projectId = displayText(project?.id || summary.task.projectId)
  if (!projectId) return null
  return {
    id: projectId,
    name: displayText(project?.name || summary.task.projectName || summary.task.target || fallbackName),
    description: project?.description || '',
    scope: project?.scope || summary.task.scope || [],
    status: project?.status || 'active',
    conversationCount: project?.conversationCount,
    factCount: project?.factCount,
    findingCount: project?.findingCount,
    conversations: [],
    createdAt: project?.createdAt || summary.task.createdAt,
    updatedAt: project?.updatedAt || summary.task.updatedAt,
  }
}
function latestSnapshotMessage(snapshot: AIWorkbenchPenTestSessionSnapshot): AIWorkbenchConversationSummary['latestMessage'] {
  const turns = snapshot.turns || snapshot.timeline?.turns || []
  for (let index = turns.length - 1; index >= 0; index -= 1) {
    const turn = turns[index]
    const assistantMessages = turn.assistantMessages || []
    for (let messageIndex = assistantMessages.length - 1; messageIndex >= 0; messageIndex -= 1) {
      if (displayText(assistantMessages[messageIndex]?.content)) return assistantMessages[messageIndex]
    }
    if (displayText(turn.userMessage?.content)) return turn.userMessage
  }
  return null
}
function findingStatsFromSnapshot(snapshot: AIWorkbenchPenTestSessionSnapshot): AIWorkbenchConversationSummary['findingStats'] {
  return {
    total: snapshot.findingOverview?.total ?? snapshot.findingViews?.length ?? snapshot.findings?.length ?? 0,
    severity: snapshot.findingOverview?.severityDistribution || {},
    status: snapshot.findingOverview?.statusDistribution || {},
  }
}
function riskSummaryFromSnapshot(snapshot: AIWorkbenchPenTestSessionSnapshot): AIWorkbenchConversationSummary['riskSummary'] {
  const overview = snapshot.findingOverview
  const total = overview?.total ?? snapshot.findingViews?.length ?? snapshot.findings?.length ?? 0
  const action = overview?.actionSummary
  const severity = overview?.severityDistribution || {}
  const severityOrder = ['critical', 'high', 'medium', 'low', 'info']
  const highest = severityOrder.find((key) => (severity[key] || 0) > 0) || ''
  const labels: Record<string, string> = { critical: '严重', high: '高危', medium: '中危', low: '低危', info: '信息' }
  return {
    totalFindings: total,
    actionableFindings: action?.actionableTotal ?? total,
    criticalOpenFindings: severity.critical || 0,
    highOpenFindings: severity.high || 0,
    highestOpenSeverity: highest,
    highestOpenSeverityLabel: labels[highest] || '',
    hasActionableFindings: (action?.actionableTotal ?? total) > 0,
    riskBreakdown: overview?.riskBreakdown || snapshot.reportPreview?.riskBreakdown || '',
  }
}
function phaseFromSnapshot(snapshot: AIWorkbenchPenTestSessionSnapshot): AIWorkbenchConversationSummary['phase'] {
  if (snapshot.agentState?.phase) return snapshot.agentState.phase
  const status = displayText(snapshot.task.status || 'pending')
  return { key: status, status, label: status === 'completed' ? '已完成' : status || '待处理', description: '', nextAction: '' }
}
function activityFromSnapshot(snapshot: AIWorkbenchPenTestSessionSnapshot): AIWorkbenchConversationSummary['currentActivity'] {
  if (snapshot.currentActivity) return snapshot.currentActivity
  if (snapshot.agentState?.currentActivity) return snapshot.agentState.currentActivity
  const status = displayText(snapshot.task.status || 'pending')
  return { type: 'session', status, label: status === 'completed' ? '测试已完成' : '等待下一步' }
}
function summaryFromSnapshot(snapshot: AIWorkbenchPenTestSessionSnapshot, unnamedProject = 'Untitled project'): AIWorkbenchConversationSummary {
  return {
    totalTokens: snapshot.totalTokens,
    sessionMode: snapshot.sessionMode,
    entrypoint: snapshot.entrypoint,
    task: snapshot.task,
    project: snapshot.project || projectFromSummary({ task: snapshot.task } as AIWorkbenchConversationSummary, unnamedProject),
    model: snapshot.model,
    launchPlan: snapshot.launchPlan,
    latestMessage: latestSnapshotMessage(snapshot),
    recentChatItems: snapshot.chatItems || [],
    activeRun: snapshot.agentState?.activeRun || null,
    latestRun: snapshot.agentState?.latestRun || null,
    findingStats: findingStatsFromSnapshot(snapshot),
    findingOverview: snapshot.findingOverview || null,
    riskSummary: riskSummaryFromSnapshot(snapshot),
    phase: phaseFromSnapshot(snapshot),
    currentActivity: activityFromSnapshot(snapshot),
    recommendedAction: snapshot.agentState?.recommendedAction || '',
    primaryAction: snapshot.primaryAction || null,
    actions: snapshot.actions || [],
    lastActivityAt: snapshot.updatedAt || snapshot.task.updatedAt,
  }
}
function mergeSessionSummaryList(list: AIWorkbenchConversationSummary[], summary: AIWorkbenchConversationSummary) {
  let matched = false
  const next = list.map((item) => {
    if (item.task.id !== summary.task.id) return item
    matched = true
    return { ...item, ...summary, project: summary.project || item.project }
  })
  return matched ? next : [summary, ...next]
}
function mergeProjectSessionSnapshot(projects: AIWorkbenchProject[], snapshot: AIWorkbenchPenTestSessionSnapshot, summary: AIWorkbenchConversationSummary) {
  const projectId = displayText(snapshot.project?.id || summary.task.projectId)
  if (!projectId) return projects
  if (!snapshot.project) return projects
  return mergeProjectList(projects, {
    ...snapshot.project,
    updatedAt: snapshot.project.updatedAt || summary.lastActivityAt,
  })
}
function mergeProjectList(projects: AIWorkbenchProject[], nextProject: AIWorkbenchProject) {
  let matched = false
  const next = projects.map((project) => {
    if (project.id !== nextProject.id) return project
    matched = true
    return { ...project, ...nextProject }
  })
  return matched ? next : [nextProject, ...next]
}

type AIWorkbenchRouteCache = {
  bootstrap: AIWorkbenchPenTestSessionBootstrapResponse | null
  projects: AIWorkbenchProject[]
  sessions: AIWorkbenchConversationSummary[]
  activeSessionId: string | null
  selectedProjectId: string
  openProjects: Record<string, boolean>
  session: AIWorkbenchPenTestSessionSnapshot | null
  selectedModel: string
  queuedMessages?: QueuedWorkbenchMessage[]
  search: string
  sidebarCollapsed?: boolean
  savedAt: number
}

let aiWorkbenchRouteCache: AIWorkbenchRouteCache | null = null

function hasAIWorkbenchRouteCache(cache: AIWorkbenchRouteCache | null) {
  return Boolean(cache && (cache.bootstrap || cache.projects.length > 0 || cache.sessions.length > 0 || cache.session))
}

type SessionMenuAction = 'blackboard' | 'rename' | 'history' | 'copy-target' | 'export-markdown' | 'delete'
type ProjectMenuAction = 'blackboard' | 'new-session' | 'rename' | 'delete'

function sessionStatusMeta(status: string, l: WorkbenchMessages) {
  switch (displayText(status).toLowerCase()) {
    case 'running':
    case 'draining':
      return { label: l.running, icon: Loader2, className: 'text-primary', animate: true }
    case 'waiting_tool':
      return { label: l.waitingTool, icon: Wrench, className: 'text-primary' }
    case 'waiting':
      return { label: l.waiting, icon: Clock3, className: 'text-muted-foreground' }
    case 'waiting_approval':
      return { label: l.waitingApproval, icon: ShieldCheck, className: 'text-tone-amber' }
    case 'pending':
    case 'queued':
      return { label: l.queued, icon: Clock3, className: 'text-muted-foreground' }
    case 'blocked':
      return { label: l.incomplete, icon: CirclePause, className: 'text-tone-amber' }
    case 'failed':
    case 'error':
      return { label: l.executionFailed, icon: AlertTriangle, className: 'text-destructive' }
    case 'paused':
    case 'pausing':
      return { label: l.paused, icon: CirclePause, className: 'text-tone-amber' }
    case 'not_executed':
      return { label: l.taskEndedUnexecuted, icon: CirclePause, className: 'text-muted-foreground' }
    case 'goal_stopped':
      return { label: l.taskGoalStopped, icon: XCircle, className: 'text-muted-foreground' }
    case 'task_stopped':
      return { label: l.taskEndedStopped, icon: XCircle, className: 'text-muted-foreground' }
    case 'cancelled':
    case 'canceled':
      return { label: l.interrupted, icon: XCircle, className: 'text-muted-foreground' }
    default:
      return null
  }
}

function SessionStatusIndicator({ status }: { status: string }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const meta = sessionStatusMeta(status, l)
  if (!meta) return <span className="h-4 w-4 shrink-0" aria-hidden="true" />
  const Icon = meta.icon
  return (
    <span className={cn('flex h-4 w-4 shrink-0 items-center justify-center', meta.className)} aria-label={meta.label} title={meta.label}>
      <Icon className={cn('h-3.5 w-3.5', meta.animate && 'animate-spin')} />
    </span>
  )
}

function SessionListItem({ summary, active, projects, moving, hideRoleBadge = false, onMove, onSelect, onMenuAction }: { summary: AIWorkbenchConversationSummary; active: boolean; projects: AIWorkbenchProject[]; moving: boolean; hideRoleBadge?: boolean; onMove: (summary: AIWorkbenchConversationSummary, project: AIWorkbenchProject) => void; onSelect: () => void; onMenuAction: (action: SessionMenuAction, summary: AIWorkbenchConversationSummary) => void }) {
  const { t, language } = useI18n()
  const l = t.aiWorkbench
  const role = summary.task.agentRole || ''
  const isMain = role === 'main' || role === 'controller' || role === 'mainagent'
  const isWorker = role === 'worker'
  const isOrchestrated = isMain || role === 'planner' || role === 'worker'
  const roleLabel = isMain ? t.aiPlan.mainAgent : role === 'planner' ? t.aiPlan.planner : role === 'worker' ? t.aiPlan.worker : ''
  const status = displayText(summary.task.status).toLowerCase()
  const target = displayText(summary.task.target || summary.project?.scope?.[0] || summary.task.name || l.unnamedTarget)
  const title = role === 'worker' ? (summary.task.planItemTitle || sessionTitle(summary, l)) : sessionTitle(summary, l)
  const time = formatDateTime(summary.lastActivityAt, language)
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className={cn('group flex w-full max-w-full min-w-0 items-center gap-1 overflow-hidden rounded-lg border px-1.5 py-1 text-left transition-colors', active ? 'border-primary/40 bg-primary/10 shadow-sm' : 'border-transparent hover:border-border hover:bg-muted/45')}>
          <button type="button" onClick={onSelect} className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden rounded-md py-1 pl-2.5 pr-1 text-left">
            <SessionStatusIndicator status={status} />
            <span className="flex min-w-0 flex-1 items-center gap-1.5 overflow-hidden">
              <span className="block min-w-0 flex-1 truncate text-sm font-medium text-foreground">{title}</span>
              {roleLabel && !hideRoleBadge ? <Badge variant="outline" className={cn('h-4 shrink-0 rounded px-1 text-[10px] font-normal', isMain ? 'border-primary/40 bg-primary/10 text-primary' : role === 'planner' ? 'border-primary/30 text-primary' : 'border-border text-muted-foreground')}>{roleLabel}</Badge> : null}
            </span>
          </button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button type="button" variant="ghost" size="icon" aria-label={l.sessionMenu} onClick={(event) => event.stopPropagation()} className="h-7 w-7 shrink-0 rounded-md text-muted-foreground opacity-0 transition-opacity hover:bg-muted hover:text-foreground group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100 [@media(pointer:coarse)]:opacity-100">
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-52">
              <DropdownMenuItem onSelect={() => onMenuAction('blackboard', summary)}><GitBranch className="h-4 w-4" />{l.exploration}</DropdownMenuItem>
              {!isWorker ? <><DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => onMenuAction('rename', summary)}><Pencil className="h-4 w-4" />{l.rename}</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onMenuAction('history', summary)}><Clock3 className="h-4 w-4" />{l.history}</DropdownMenuItem></> : null}
              {!isOrchestrated ? <DropdownMenuSub>
                <DropdownMenuSubTrigger disabled={moving}>
                  <Folder className="h-4 w-4" />{l.sessionProject}
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-64 max-w-[calc(100vw-2rem)] max-h-[min(24rem,var(--radix-dropdown-menu-content-available-height))] overflow-y-auto">
                  {activeStatuses.has(status) || status === 'pausing' ? <div className="px-2 py-1.5 text-xs text-muted-foreground">{l.moveSessionBusy}</div> : null}
                  {projects.map((project) => {
                    const current = project.id === (summary.project?.id || summary.task.projectId)
                    return <DropdownMenuItem key={project.id} textValue={project.name} title={project.name} disabled={current || activeStatuses.has(status) || status === 'pausing'} onSelect={() => onMove(summary, project)}>
                      <Folder className="h-4 w-4" />
                      <span className="min-w-0 flex-1 truncate">{project.name || l.noProject}</span>
                      {current ? <Check className="h-4 w-4" aria-label={l.currentProject} /> : null}
                    </DropdownMenuItem>
                  })}
                  {!projects.length ? <DropdownMenuItem disabled>{l.noMoveProjects}</DropdownMenuItem> : null}
                </DropdownMenuSubContent>
              </DropdownMenuSub> : null}
              <DropdownMenuItem onSelect={() => onMenuAction('copy-target', summary)}><Copy className="h-4 w-4" />{l.copyTarget}</DropdownMenuItem>
              <DropdownMenuItem onSelect={() => onMenuAction('export-markdown', summary)}><FileText className="h-4 w-4" />{l.downloadMarkdown}</DropdownMenuItem>
              {!isOrchestrated ? <><DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onSelect={() => onMenuAction('delete', summary)}><Trash2 className="h-4 w-4" />{l.deleteSession}</DropdownMenuItem></> : null}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </TooltipTrigger>
      <TooltipContent side="right" align="start" className="max-w-[360px]">
        <div className="space-y-1">
          <div className="line-clamp-2 break-all text-sm font-medium">{title}</div>
          {role === 'worker' && summary.task.planItemStatus ? <div className="text-xs text-muted-foreground">{t.aiPlan.statuses[summary.task.planItemStatus as keyof typeof t.aiPlan.statuses] || summary.task.planItemStatus}</div> : null}
          <div className="break-all font-mono text-xs text-muted-foreground">{target}</div>
          <div className="break-all font-mono text-xs text-muted-foreground">{summary.task.id}</div>
          {time ? <div className="text-xs text-muted-foreground">{time}</div> : null}
          <TokenUsage total={summary.totalTokens} />
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

function ProjectGroup({ project, projects, sessionPage, onLoadSessions, moving, onMove, activeSessionId, selectedProjectId, open, onOpenChange, onSelectProject, onSelectSession, onProjectMenuAction, onSessionMenuAction }: {
  project: AIWorkbenchProject
  projects: AIWorkbenchProject[]
  sessionPage?: ProjectSessionPage
  onLoadSessions: (more?: boolean) => Promise<boolean>
  moving: boolean
  onMove: (summary: AIWorkbenchConversationSummary, project: AIWorkbenchProject) => void
  activeSessionId: string | null
  selectedProjectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelectProject: () => void
  onSelectSession: (id: string) => void
  onProjectMenuAction: (action: ProjectMenuAction, project: AIWorkbenchProject) => void
  onSessionMenuAction: (action: SessionMenuAction, summary: AIWorkbenchConversationSummary) => void
}) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const conversations = project.conversations || []
  const selected = selectedProjectId === project.id
  const hasProject = Boolean(project.id)
  const [visibleCounts, setVisibleCounts] = useState<Record<string, number>>({})
  const hasMorePages = Boolean(sessionPage && sessionPage.conversations.length < sessionPage.total)
  const expandGroup = async (key: string, pagination: ReturnType<typeof sessionGroupPagination>) => {
    if (pagination.needsFetch && !await onLoadSessions(true)) return
    setVisibleCounts((previous) => ({ ...previous, [key]: pagination.nextVisible }))
  }
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} className="min-w-0 overflow-hidden">
      <div className={cn('group flex min-w-0 items-center gap-1 overflow-hidden rounded-lg px-1.5 py-1.5 transition-colors hover:bg-sidebar-hover', selected && 'bg-muted/45')}>
        <CollapsibleTrigger asChild>
          <Button type="button" variant="ghost" size="icon" aria-label={project.name || l.noProject} className="h-7 w-7 shrink-0 rounded-md hover:bg-transparent">
            {open ? <FolderOpen className="h-4 w-4 text-muted-foreground group-hover:hidden" /> : <Folder className="h-4 w-4 text-muted-foreground group-hover:hidden" />}
            <ChevronRight className={cn('hidden h-4 w-4 text-muted-foreground transition-transform group-hover:block', open && 'rotate-90')} />
          </Button>
        </CollapsibleTrigger>
        <button type="button" onClick={() => { onSelectProject(); onOpenChange(!open) }} className="flex min-w-0 flex-1 items-center gap-2 overflow-hidden rounded-md py-1 pl-0.5 pr-1.5 text-left">
          <span className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">{project.name || l.noProject}</span>
        </button>
        {hasProject ? <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="ghost" size="icon" className="h-7 w-7 shrink-0 rounded-md text-muted-foreground opacity-0 group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100 [@media(pointer:coarse)]:opacity-100" aria-label={l.projectMenu}>
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuItem onSelect={() => onProjectMenuAction('blackboard', project)}><Monitor className="h-4 w-4" />{l.projectEvidence}</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => onProjectMenuAction('new-session', project)}><MessageSquarePlus className="h-4 w-4" />{l.newSession}</DropdownMenuItem>
            <DropdownMenuItem onSelect={() => onProjectMenuAction('rename', project)}><Pencil className="h-4 w-4" />{l.renameProject}</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={() => onProjectMenuAction('delete', project)}><Trash2 className="h-4 w-4" />{l.deleteProject}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu> : <div className="h-7 w-7 shrink-0" />}
      </div>
      <CollapsibleContent>
        <div className="mt-1 min-w-0 max-w-full space-y-1 overflow-hidden">
          {conversations.length ? (() => {
            const roleOf = (summary: AIWorkbenchConversationSummary) => summary.task.agentRole || ''
            const isMainRole = (role: string) => role === 'main' || role === 'controller' || role === 'mainagent'
            const mainSessions = conversations.filter((summary) => isMainRole(roleOf(summary)))
            const plannerSessions = conversations.filter((summary) => roleOf(summary) === 'planner')
            const workerSessions = conversations.filter((summary) => roleOf(summary) === 'worker')
            const otherSessions = conversations.filter((summary) => {
              const role = roleOf(summary)
              return !isMainRole(role) && role !== 'planner' && role !== 'worker'
            })
            const grouped = mainSessions.length + plannerSessions.length + workerSessions.length > 0
              || Boolean(sessionPage?.roleCounts && (sessionPage.roleCounts.main + sessionPage.roleCounts.planner + sessionPage.roleCounts.workers > 0))
            const row = (summary: AIWorkbenchConversationSummary, hideRoleBadge = true) => (
              <SessionListItem key={summary.task.id} projects={projects} moving={moving} hideRoleBadge={hideRoleBadge} onMove={onMove} summary={summary} active={summary.task.id === activeSessionId} onSelect={() => onSelectSession(summary.task.id)} onMenuAction={onSessionMenuAction} />
            )
            const section = (key: 'main' | 'planner' | 'workers' | 'other', icon: ReactNode, label: string, items: AIWorkbenchConversationSummary[], hideRoleBadge = true) => {
              const total = sessionPage?.roleCounts?.[key]
              if (!items.length && !total) return null
              const count = visibleCounts[key] ?? SESSION_GROUP_VISIBLE_LIMIT
              const visible = items.slice(0, count)
              const pagination = sessionGroupPagination(items.length, count, hasMorePages, total)
              const { complete } = pagination
              return (
                <section key={key} data-agent-group={key} aria-label={label} className="min-w-0">
                  <div className="flex min-w-0 items-center gap-1.5 pl-[1.125rem] pr-2 pb-1 pt-2 text-[11px] font-medium text-muted-foreground">
                    <span className="flex size-4 shrink-0 items-center justify-center" aria-hidden="true">{icon}</span>
                    <span className="min-w-0 truncate">{label}</span>
                    <span className="ml-auto shrink-0 tabular-nums text-[10px] opacity-70">{pagination.total}</span>
                  </div>
                  <div className="space-y-1">
                    {visible.map((summary) => row(summary, hideRoleBadge))}
                    {pagination.showControl ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        aria-expanded={complete}
                        disabled={pagination.needsFetch && sessionPage?.loading}
                        className="h-7 w-full justify-start rounded-none px-0 pl-10 text-xs font-normal text-muted-foreground hover:bg-transparent hover:text-foreground focus-visible:bg-transparent active:bg-transparent"
                        onClick={() => { void expandGroup(key, pagination) }}
                      >
                        {pagination.needsFetch && sessionPage?.loading ? l.loadingSessions : complete ? l.collapseSessions : l.loadMoreSessions}
                      </Button>
                    ) : null}
                  </div>
                </section>
              )
            }
            if (!grouped) {
              const count = visibleCounts.all ?? SESSION_GROUP_VISIBLE_LIMIT
              const visible = conversations.slice(0, count)
              const pagination = sessionGroupPagination(conversations.length, count, hasMorePages, sessionPage?.total)
              const { complete } = pagination
              return (
                <div className="space-y-1">
                  {visible.map((summary) => row(summary, false))}
                  {pagination.showControl ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-expanded={complete}
                      disabled={pagination.needsFetch && sessionPage?.loading}
                      className="h-7 w-full justify-start rounded-none px-0 pl-10 text-xs font-normal text-muted-foreground hover:bg-transparent hover:text-foreground focus-visible:bg-transparent active:bg-transparent"
                      onClick={() => { void expandGroup('all', pagination) }}
                    >
                      {pagination.needsFetch && sessionPage?.loading ? l.loadingSessions : complete ? l.collapseSessions : l.loadMoreSessions}
                    </Button>
                  ) : null}
                </div>
              )
            }
            const hasPinned = mainSessions.length + plannerSessions.length > 0
            return (
              <div className="space-y-0.5">
                {/* The main agent and planner are anchored above the worker list:
                    every new worker appears below them, never pushes them down. */}
                <div data-pinned="true" className="space-y-0.5">
                  {section('main', <UserRoundCog className="h-3.5 w-3.5" />, t.aiPlan.mainAgentGroup, mainSessions)}
                  {section('planner', <BrainCircuit className="h-3.5 w-3.5" />, t.aiPlan.plannerGroup, plannerSessions)}
                </div>
                {hasPinned && (workerSessions.length || otherSessions.length) ? <div className="mx-2 my-1 border-t border-border/60" role="separator" /> : null}
                {section('workers', <ListTodo className="h-3.5 w-3.5" />, t.aiPlan.workersGroup, workerSessions)}
                {section('other', <MessageSquare className="h-3.5 w-3.5" />, t.aiPlan.otherConversations, otherSessions, false)}
              </div>
            )
          })() : (!hasProject || (sessionPage?.page && !sessionPage.loading && !sessionPage.error)) ? <div className="px-3 py-2 pl-10 text-xs text-muted-foreground">{l.noSessions}</div> : null}
          {hasProject && (!sessionPage || (sessionPage.loading && sessionPage.conversations.length === 0)) ? <div role="status" className="flex items-center gap-1.5 px-3 py-2 pl-10 text-xs text-muted-foreground"><Loader2 className="h-3.5 w-3.5 animate-spin" />{l.loadingSessions}</div> : null}
          {sessionPage?.error ? <div className="px-3 py-2 pl-10 text-xs text-muted-foreground"><span>{l.loadSessionFailed}</span><Button type="button" variant="ghost" size="sm" className="ml-1 h-7 px-2 text-xs" onClick={() => { void onLoadSessions(sessionPage.retryMore) }}>{l.retry}</Button></div> : null}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function toolResultBadge(activity: ThinkingActivity, l: WorkbenchMessages) {
  const status = displayText(activity.status).toLowerCase()
  if (status === 'failed' || status === 'error') return <Badge variant="destructive" className="h-6 rounded-full px-2 text-xs">{l.executionFailed}</Badge>
  if (status === 'blocked') return <Badge variant="outline" className="h-6 rounded-full border-amber-500/35 bg-amber-500/10 px-2 text-xs text-amber-700 dark:text-amber-300">{l.blocked}</Badge>
  if (status === 'cancelled') return <Badge variant="outline" className="h-6 rounded-full px-2 text-xs text-muted-foreground">{l.interrupted}</Badge>
  return <Badge variant="outline" className="h-6 rounded-full px-2 text-xs text-muted-foreground">{l.callCompleted}</Badge>
}

function renderActivityStepIcon(activity: ThinkingActivity) {
  if (activity.eventType === 'reasoning') return <MessageSquare />
  if (activity.eventType === 'assistant_commentary') return <Text />
  if (activity.type === 'finding') return <ShieldCheck />
  if (displayText(activity.status).toLowerCase() === 'blocked') return <ShieldBan />

  if (activity.type !== 'tool' && !activity.toolName) {
    if (/规划下一步|planning/i.test(activity.title)) return <ListTodo />
    if (/重试|retry/i.test(activity.title)) return <RotateCw />
    if (activity.type === 'error' || /调整策略|主备切换|strategy|failover/i.test(activity.title)) return <GitBranch />
  }

  // Prefer the actual operation name; old history may only carry a display label.
  // Never classify from detail text, which can mention unrelated files or commands.
  const operation = (activity.toolName || activity.title).replace(/([a-z])([A-Z])/g, '$1 $2').replace(/[_:./-]+/g, ' ').toLowerCase()
  if (/\b(image|images|screenshot|screenshots)\b|图片|图像|截图/.test(operation)) return <Images />
  if (/\b(file|files|filesystem|directory|directories|folder|folders|patch)\b|文件|目录/.test(operation)) return <FileText />
  if (/\b(exec|execute|command|commands|shell|terminal|bash|powershell|python|script|scripts|stdin)\b|命令|终端|脚本/.test(operation)) return <SquareTerminal />
  if (activity.type === 'tool' || activity.toolName || /工具/.test(activity.title)) return <Wrench />
  return <MessageSquare />
}

function redactBlockedText(value: string, language: 'zh' | 'en' = 'zh', max = 420) {
  const redaction = language === 'zh' ? '[已脱敏]' : '[redacted]'
  let text = value.replace(/(authorization|proxy-authorization|cookie|token|api[_-]?key|password)(\s*[:=]\s*)([^\s;,]+)/gi, `$1$2${redaction}`)
  text = text.replace(/Bearer\s+[A-Za-z0-9._~+/=-]+/gi, `Bearer ${redaction}`)
  return compactText(text, max)
}

function BlockedRuleDetails({ match, phase, language }: { match: AIWorkbenchBlockedMatch; phase?: string; language: 'zh' | 'en' }) {
  const zh = language === 'zh'
  const phaseLabel = phase === 'egress_gateway' ? (zh ? 'HTTP 出口网关' : 'HTTP egress gateway') : phase === 'tool_preflight' ? (zh ? '工具调用前' : 'Tool preflight') : ''
  const rows = [
    [zh ? '规则名称' : 'Rule name', match.ruleName],
    [zh ? '规则 ID' : 'Rule ID', match.ruleId],
    [zh ? '拦截阶段' : 'Phase', phaseLabel],
    [zh ? '匹配内容' : 'Matched content', match.matchedText ? redactBlockedText(match.matchedText, language) : (zh ? '未返回具体匹配内容' : 'No matching fragment returned')],
    [zh ? '规则说明' : 'Rule message', match.message ? redactBlockedText(match.message, language) : ''],
  ].filter(([, value]) => Boolean(value))
  return <div className="rounded-md border border-amber-500/25 bg-amber-500/5 px-3 py-2 text-xs leading-5">
    <div className="mb-1 font-medium text-foreground">{zh ? '命中规则' : 'Matched rule'}</div>
    <dl className="grid min-w-0 gap-x-3 gap-y-1 sm:grid-cols-[auto_minmax(0,1fr)]">
      {rows.map(([label, value]) => <div key={label} className="contents"><dt className="text-muted-foreground">{label}</dt><dd className="min-w-0 break-words font-mono text-foreground">{value}</dd></div>)}
    </dl>
  </div>
}

const ActivityStep = memo(function ActivityStep({ activity, activeNode }: { activity: ThinkingActivity; activeNode: boolean }) {
  const { t, language } = useI18n()
  const l = t.aiWorkbench
  const [detailsOpen, setDetailsOpen] = useState(false)
  const [analysisOpen, setAnalysisOpen] = useState(true)
  const status = displayText(activity.status).toLowerCase()
  const detail = status === 'failed' || status === 'blocked' ? localizeBackendText(activity.detail || '', language) : activity.detail || ''
  const isAnalysis = activity.eventType === 'reasoning' || activity.eventType === 'assistant_commentary'
  const markerIcon = <MarkerIcon className="mt-0.5 text-foreground [&_svg]:stroke-[1.5]">
    <span key={activeNode ? 'active' : 'idle'} className="ai-status-enter inline-flex">
      {activeNode ? <Spinner aria-hidden="true" className="motion-reduce:animate-none" /> : renderActivityStepIcon(activity)}
    </span>
  </MarkerIcon>
  const markerTitle = <MarkerContent className="min-w-0 [overflow-wrap:anywhere]">
    <span className={cn(activeNode && 'ai-thinking-shimmer')}>{localizeWorkbenchActivity(activity.title, l)}{activity.retry ? ` ${activity.retry.attempt}/${activity.retry.max}` : ''}</span>
  </MarkerContent>
  if (isAnalysis) {
    return <Collapsible open={analysisOpen} onOpenChange={setAnalysisOpen} className="min-w-0 max-w-full">
      <CollapsibleTrigger asChild>
        <Marker asChild className="items-start text-foreground">
          <button type="button" className="w-full rounded-md py-1 text-left font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            {markerIcon}
            {markerTitle}
            {activeNode && <span className="shrink-0 text-xs font-normal leading-5 text-muted-foreground">{l.generating}</span>}
            {(status === 'cancelled' || status === 'failed') && <span className="text-xs font-normal leading-5 text-muted-foreground">{l.interruptedRetained}</span>}
            <ChevronDown aria-hidden="true" className={cn('ml-auto mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform duration-200 motion-reduce:transition-none', analysisOpen && 'rotate-180')} />
          </button>
        </Marker>
      </CollapsibleTrigger>
      <CollapsibleContent className="ai-process-content min-w-0 max-w-full"><div className="min-w-0 max-w-full pb-3 pl-6 pt-2 text-sm leading-7">{activeNode ? <div className="min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]">{detail}</div> : <MarkdownMessage content={detail} />}</div></CollapsibleContent>
    </Collapsible>
  }
  const terminalTool = activity.type === 'tool' && Boolean(status) && !activeStatuses.has(status)
  const terminalToolDetail = terminalTool && Boolean(activity.detail)
  const showInlineDetail = Boolean(activity.detail) && !terminalTool
  return (
    <div className="min-w-0">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <Marker role={activeNode ? 'status' : undefined} className="items-start py-1 text-foreground">
          {markerIcon}
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            {markerTitle}
            {terminalTool ? toolResultBadge(activity, l) : null}
          </div>
        </Marker>
        {terminalToolDetail ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 shrink-0 gap-1 rounded-md px-2 text-xs text-muted-foreground hover:text-foreground"
            onClick={() => setDetailsOpen((open) => !open)}
            aria-expanded={detailsOpen}
          >
            {l.viewDetails}<ChevronDown className={cn('h-3.5 w-3.5 transition-transform duration-200 motion-reduce:transition-none', detailsOpen && 'rotate-180')} />
          </Button>
        ) : null}
      </div>
      {showInlineDetail ? <div className="mt-1 min-w-0 max-w-full pl-6 text-xs leading-5 text-muted-foreground [overflow-wrap:anywhere]">{detail}</div> : null}
      {terminalToolDetail && detailsOpen ? (
        <div className="ml-6 mt-2 max-h-80 space-y-2 overflow-auto rounded-md border border-border/70 bg-muted/25 px-3 py-2 text-xs leading-5 text-muted-foreground">
          {status === 'blocked' && activity.blockedMatch ? <BlockedRuleDetails match={activity.blockedMatch} phase={activity.blockedPhase} language={language} /> : null}
          {activity.detail ? <div className="min-w-0 max-w-full whitespace-pre-wrap [overflow-wrap:anywhere]">{status === 'blocked' ? redactBlockedText(activity.detail, language, 900) : activity.detail}</div> : null}
        </div>
      ) : null}
    </div>
  )
}, (previous, next) => previous.activeNode === next.activeNode
  && previous.activity.id === next.activity.id && previous.activity.status === next.activity.status
  && previous.activity.title === next.activity.title && previous.activity.detail === next.activity.detail
  && previous.activity.eventType === next.activity.eventType && previous.activity.type === next.activity.type
  && previous.activity.toolName === next.activity.toolName
  && previous.activity.retry?.attempt === next.activity.retry?.attempt && previous.activity.retry?.max === next.activity.retry?.max
  && previous.activity.blockedPhase === next.activity.blockedPhase
  && previous.activity.blockedMatch === next.activity.blockedMatch)


function ThinkingDisclosure({ totalTokens, run, activities, fallbackActive = false, fallbackLabel, completedGoal, stageMessages = [], onQuote }: { totalTokens?: number; run?: AIWorkbenchAgentRun | null; activities: ThinkingActivity[]; fallbackActive?: boolean; fallbackLabel?: string; completedGoal?: GoalRun | null; stageMessages?: TurnAssistantMessage[]; onQuote?: (quote: MessageQuote) => void }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const active = !completedGoal && (runIsActive(run) || fallbackActive)
  const disclosurePhase = `${run?.id || ''}:${active}:${completedGoal?.id || ''}`
  const [manualOpen, setManualOpen] = useState<{ phase: string; open: boolean } | null>(null)
  const [visibleCount, setVisibleCount] = useState(20)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (active) {
      const timer = window.setInterval(() => setNow(Date.now()), 1000)
      return () => window.clearInterval(timer)
    }
    return undefined
  }, [active])
  const open = manualOpen?.phase === disclosurePhase ? manualOpen.open : active
  const duration = getRunDuration(run, now)
  const status = displayText(run?.status).toLowerCase()
  const animationStates = activityAnimationStates(activities, active)
  const current = activities.filter((_, index) => animationStates[index]).at(-1)
  const activeLabel = current?.eventType === 'reasoning' ? l.reasoning : current?.eventType === 'assistant_commentary' ? l.commentary : current?.type === 'tool' ? l.callingTool : l.waitingModel
  const label = completedGoal ? l.analysisCompleted : active ? activeLabel : status === 'cancelled' ? l.interrupted : status === 'blocked' ? l.incomplete : status === 'failed' ? l.executionFailed : l.analysisCompleted
  const visibleActivities: ThinkingActivity[] = activities.length ? activities.slice(-visibleCount) : [{ title: localizeWorkbenchActivity(fallbackLabel || '', l) || (active ? l.planning : l.turnCompleted), status: active ? 'running' : 'completed' }]
  return (
    <Collapsible open={open} onOpenChange={(open) => setManualOpen({ phase: disclosurePhase, open })} className="py-1" data-slot="ai-execution-details">
      {/* Announce phase changes, without announcing the elapsed timer every second. */}
      <span role="status" aria-atomic="true" className="sr-only">{label}</span>
      <div className="flex flex-wrap items-center gap-x-3">
        <CollapsibleTrigger asChild>
          <button type="button" className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 rounded-md py-2 text-left text-sm font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none" aria-label={formatWorkbenchText(l.toggleProcess, { label })}>
            <span>{label}</span>
            {completedGoal ? <span className="text-xs font-normal">{l.viewExecutionDetails}</span> : <><span className="whitespace-nowrap text-xs font-normal tabular-nums text-muted-foreground">{l.elapsed}{' '}{formatDuration(duration, l)}</span><TokenUsage total={totalTokens} scope="turn" /></>}
            <ChevronDown aria-hidden="true" className={cn('h-4 w-4 shrink-0 text-muted-foreground transition-transform duration-200 motion-reduce:transition-none', open && 'rotate-180')} />
          </button>
        </CollapsibleTrigger>
      </div>
      <CollapsibleContent className="ai-process-content">
        <div className="py-3 pr-1">
          {completedGoal ? <div className="mb-4 space-y-3"><div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground"><span>{l.elapsed}{' '}{formatDuration(duration, l)}</span><TokenUsage total={totalTokens} scope="turn" /></div><GoalModeCard goal={completedGoal} /></div> : null}
          {activities.length > visibleCount && <Button variant="ghost" size="sm" className="mb-3 text-muted-foreground" onClick={() => setVisibleCount(count => count + 20)}>{formatWorkbenchText(l.earlierActivities, { count: activities.length - visibleCount })}</Button>}
          <div className="space-y-4">
            {visibleActivities.map((activity, index) => {
              const activeNode = activities.length ? animationStates[Math.max(0, activities.length - visibleCount) + index] : active
              return <ActivityStep key={activity.id || `activity-${Math.max(0, activities.length - visibleCount) + index}`} activity={activity} activeNode={activeNode} />
            })}
          </div>
          {stageMessages.length ? <div className="mt-6 space-y-6">{stageMessages.map((message) => <AssistantMessage key={message.id} content={message.content} createdAt={message.createdAt} onQuote={onQuote ? () => onQuote({ content: message.content, role: 'assistant' }) : undefined} />)}</div> : null}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

async function copyMessageContent(content: string, l: WorkbenchMessages) {
  const ok = await copyToClipboard(content)
  if (ok) toast.success(l.copied)
  else toast.error(l.copyFailed)
}

function MessageFooter({ content, createdAt, align, onEdit, onQuote, visible = true }: { content: string; createdAt?: string | null; align: 'left' | 'right'; onEdit?: () => void; onQuote?: () => void; visible?: boolean }) {
  const { t, language } = useI18n()
  const l = t.aiWorkbench
  return <div className={cn('mt-1.5 flex items-center gap-1.5 text-xs text-muted-foreground', align === 'right' ? 'justify-end pr-1' : 'justify-start pl-1', !visible && 'pointer-events-none opacity-0 focus-within:pointer-events-auto focus-within:opacity-100 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100')}>
    <span>{formatDateTime(createdAt, language)}</span>
    <Tooltip>
      <TooltipTrigger asChild>
        <Button type="button" variant="ghost" size="icon" className="h-6 w-6 rounded-md text-muted-foreground hover:text-foreground" aria-label={l.copyMessage} onClick={() => void copyMessageContent(content, l)}>
          <Copy className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>{l.copy}</TooltipContent>
    </Tooltip>
    {onQuote ? <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="h-6 w-6 rounded-md text-muted-foreground hover:text-foreground" aria-label={l.quoteMessage} onClick={onQuote}><MessageSquareQuote className="h-3.5 w-3.5" /></Button></TooltipTrigger><TooltipContent>{l.quote}</TooltipContent></Tooltip> : null}
    {onEdit ? <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="h-6 w-6 rounded-md text-muted-foreground hover:text-foreground" aria-label={l.edit} onClick={onEdit}><Pencil className="h-3.5 w-3.5" /></Button></TooltipTrigger><TooltipContent>{l.edit}</TooltipContent></Tooltip> : null}
  </div>
}

const MarkdownMessage = memo(function MarkdownMessage({ content, onMouseEnter }: { content: string; onMouseEnter?: () => void }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const html = useMemo(() => renderMarkdownHTML(content, { remoteImages: 'click', copyCodeLabel: l.copyCode, loadImageLabel: l.loadRemoteImage }), [content, l.copyCode, l.loadRemoteImage])
  const handleClick = (event: React.MouseEvent<HTMLDivElement>) => {
    const target = event.target as HTMLElement
    const copyButton = target.closest<HTMLButtonElement>('[data-copy-code]')
    if (copyButton) {
      const code = copyButton.closest('.markdown-code-block')?.querySelector('code')?.textContent || ''
      void copyToClipboard(code).then(ok => {
        copyButton.textContent = ok ? l.copied : l.copyFailed
        window.setTimeout(() => { copyButton.textContent = l.copyCode }, 1500)
      })
      return
    }
    const placeholder = target.closest<HTMLButtonElement>('[data-remote-image]')
    if (placeholder) {
      const src = placeholder.dataset.remoteImage
      if (!src) return
      const image = document.createElement('img')
      image.src = src
      image.alt = placeholder.dataset.imageAlt || ''
      image.loading = 'lazy'
      image.referrerPolicy = 'no-referrer'
      image.className = 'markdown-remote-image'
      placeholder.replaceWith(image)
    }
  }
  return <div
    data-ai-message-quote="assistant"
    onMouseEnter={onMouseEnter}
    onClick={handleClick}
    className="ai-markdown min-w-0 max-w-full [overflow-wrap:anywhere] text-sm leading-7 text-foreground [&_.markdown-table]:max-w-full [&_.markdown-table]:my-5 [&_.markdown-table]:overflow-x-auto [&_a]:text-primary [&_a]:underline-offset-4 hover:[&_a]:underline [&_blockquote]:my-4 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:text-muted-foreground [&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.92em] [&_h1]:mb-4 [&_h1]:mt-7 [&_h1]:text-2xl [&_h1]:font-semibold [&_h1]:leading-tight [&_h2]:mb-3 [&_h2]:mt-7 [&_h2]:text-xl [&_h2]:font-semibold [&_h2]:leading-tight [&_h3]:mb-2.5 [&_h3]:mt-6 [&_h3]:text-lg [&_h3]:font-semibold [&_h3]:leading-tight [&_h4]:mb-2 [&_h4]:mt-5 [&_h4]:text-base [&_h4]:font-semibold [&_h4]:leading-tight [&_hr]:my-6 [&_hr]:border-border/80 [&_li]:pl-1 [&_li]:leading-7 [&_ol]:my-3 [&_ol]:list-decimal [&_ol]:space-y-2 [&_ol]:pl-5 [&_p]:my-3 [&_p]:leading-7 [&_pre]:max-w-full [&_pre]:whitespace-pre [&_pre]:[overflow-wrap:normal] [&_pre]:my-4 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:border-border/70 [&_pre]:bg-muted/30 [&_pre]:p-3 [&_pre]:text-xs [&_pre]:leading-6 [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_strong]:font-semibold [&_table]:w-full [&_table]:min-w-[560px] [&_table]:border-collapse [&_table]:text-sm [&_td]:border-b [&_td]:border-border/70 [&_td]:px-3 [&_td]:py-2 [&_td]:align-top [&_td]:leading-6 [&_th]:border-b [&_th]:border-border [&_th]:px-3 [&_th]:py-2 [&_th]:text-left [&_th]:font-semibold [&_ul]:my-3 [&_ul]:list-disc [&_ul]:space-y-2 [&_ul]:pl-5 first:[&_h1]:mt-0 first:[&_h2]:mt-0 first:[&_h3]:mt-0"
    dangerouslySetInnerHTML={{ __html: html }}
  />
})

const messageImageCache = new Map<string, string[]>()

function MessageImages({ sessionId, messageId, count }: { sessionId: string; messageId: string; count: number }) {
  const { t } = useI18n()
  const cacheKey = `${sessionId}:${messageId}`
  const [images, setImages] = useState<string[]>(() => messageImageCache.get(cacheKey) || [])
  const [failed, setFailed] = useState(false)
  useEffect(() => {
    if (images.length) return
    const controller = new AbortController()
    getAIWorkbenchMessageImages(sessionId, messageId)
      .then(next => {
        if (controller.signal.aborted) return
        messageImageCache.set(cacheKey, next)
        if (!next.length) setFailed(true)
        setImages(next)
      })
      .catch(() => { if (!controller.signal.aborted) setFailed(true) })
    return () => controller.abort()
  }, [cacheKey, images.length, messageId, sessionId])
  if (failed) return <div className="mt-2 text-right text-xs text-muted-foreground">{t.aiWorkbench.imageLoadFailed} · {count}</div>
  if (!images.length) return <div className="mt-2 flex justify-end gap-2">{[0, 1, 2].slice(0, Math.min(count, 3)).map(index => <div key={index} className="h-20 w-20 animate-pulse rounded-lg border border-border/70 bg-muted/40" />)}</div>
  return <div className="mt-2 flex max-w-full flex-wrap justify-end gap-2">{images.map((src, index) => <Image key={index} src={src} alt={`attachment ${index + 1}`} width={80} height={80} unoptimized className="h-20 w-20 rounded-lg border border-border/70 object-cover" />)}</div>
}

function UserMessage({ content, createdAt, onEdit, onQuote }: { content: string; createdAt?: string | null; onEdit?: () => void; onQuote?: () => void }) {
  const [toolsVisible, setToolsVisible] = useState(false)
  return (
    <div className="flex min-w-0 justify-end">
      <div className="flex min-w-0 max-w-[92%] flex-col items-end" onMouseLeave={() => setToolsVisible(false)}>
        <div data-ai-message-quote="user" onMouseEnter={() => setToolsVisible(true)} className="w-fit max-w-full rounded-2xl rounded-tr-md border border-primary/20 bg-primary/10 px-4 py-3 text-sm leading-7 text-foreground">
          <div className="whitespace-pre-wrap [overflow-wrap:anywhere]">{content}</div>
        </div>
        <MessageFooter content={content} createdAt={createdAt} align="right" onEdit={onEdit} onQuote={onQuote} visible={toolsVisible} />
      </div>
    </div>
  )
}
type ReportAttachment = { name: string; downloadUrl: string; mimeType: string }

function ReportAttachmentLink({ attachment, onDownload }: { attachment: ReportAttachment; onDownload: () => void }) {
  return <div className="mt-3 flex w-full items-center" data-slot="report-attachment"><button type="button" onClick={onDownload} className="inline-flex max-w-full items-center gap-2 rounded-md px-1 py-1 text-sm font-medium text-blue-600 transition-colors hover:bg-blue-500/10 hover:text-blue-700 hover:underline dark:text-blue-400 dark:hover:text-blue-300 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"><FileText className="h-4 w-4 shrink-0" /><span className="truncate" title={attachment.name}>{attachment.name}</span></button></div>
}

function AssistantMessage({ content, createdAt, onQuote, reportAttachment, onDownloadReport }: { content: string; createdAt?: string | null; onQuote?: () => void; reportAttachment?: ReportAttachment; onDownloadReport?: () => void }) {
  const { language } = useI18n()
  const [toolsVisible, setToolsVisible] = useState(false)
  const displayContent = localizeAssistantText(content, language)
  return (
    <div className="flex justify-start">
      <div className="min-w-0 w-[92%] px-1" onMouseLeave={() => setToolsVisible(false)}>
        <MarkdownMessage content={displayContent} onMouseEnter={() => setToolsVisible(true)} />
        <MessageFooter content={displayContent} createdAt={createdAt} align="left" onQuote={onQuote} visible={toolsVisible} />
        {reportAttachment && onDownloadReport ? <ReportAttachmentLink attachment={reportAttachment} onDownload={onDownloadReport} /> : null}
      </div>
    </div>
  )
}

type TurnAssistantMessage = { id: string; content: string; createdAt: string }
type LegacyTurn = { id: string; userContent: string; userCreatedAt?: string; assistantMessages: TurnAssistantMessage[] }
type ConversationRow =
  | { kind: 'turn'; key: string; turn: AIWorkbenchConversationTurn }
  | { kind: 'legacy'; key: string; turn: LegacyTurn }
  | { kind: 'live'; key: string }
  | { kind: 'footer'; key: string }

function LegacyMessageTurn({ id, userContent, userCreatedAt, assistantMessages, onQuote, reportAttachment, onDownloadReport }: { id: string; userContent: string; userCreatedAt?: string; assistantMessages: TurnAssistantMessage[]; onQuote: (quote: MessageQuote) => void; reportAttachment?: ReportAttachment; onDownloadReport?: () => void }) {
  return (
    <div id={turnElementId(id)} data-ai-turn-id={id} className="scroll-mt-8 space-y-6">
      {userContent ? <UserMessage content={userContent} createdAt={userCreatedAt} onQuote={() => onQuote({ content: userContent, role: 'user' })} /> : null}
      {assistantMessages.map((message, index) => <AssistantMessage key={message.id} content={message.content} createdAt={message.createdAt} onQuote={() => onQuote({ content: message.content, role: 'assistant' })} reportAttachment={index === assistantMessages.length - 1 ? reportAttachment : undefined} onDownloadReport={index === assistantMessages.length - 1 ? onDownloadReport : undefined} />)}
    </div>
  )
}

const ConversationTurn = memo(function ConversationTurn({ turn, sessionId, goalRun, sessionStatus, allowActive, fallbackLabel, editable, editing, editedContent, onEditStart, onEditChange, onEditCancel, onEditSubmit, onQuote, reportAttachment, onDownloadReport }: { turn: AIWorkbenchConversationTurn; sessionId?: string; goalRun?: GoalRun | null; sessionStatus?: string; allowActive: boolean; fallbackLabel?: string; editable: boolean; editing: boolean; editedContent: string; onEditStart: (turn: AIWorkbenchConversationTurn) => void; onEditChange: (value: string) => void; onEditCancel: () => void; onEditSubmit: () => void; onQuote: (quote: MessageQuote) => void; reportAttachment?: ReportAttachment; onDownloadReport?: () => void }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const userContent = turn.userMessageView?.content || turn.userMessage?.content || ''
  const imageCount = Math.max(0, Number(turn.userMessage?.metadata?.imageCount || 0))
  const references = (turn.userMessage?.metadata?.assetReferences || []) as AssetReferenceSnapshot[]
  const userCreatedAt = turn.userMessage?.createdAt || turn.startedAt
  const assistantMessageViews = turn.assistantMessageViews || []
  const storedAssistantMessages = turn.assistantMessages || []
  const assistantMessages = assistantMessageViews.length ? assistantMessageViews.map((message, index) => ({ id: `${turn.id}-assistant-view-${index}`, content: message.content, createdAt: storedAssistantMessages[index]?.createdAt || turn.updatedAt })) : storedAssistantMessages.map((message) => ({ id: message.id, content: message.content, createdAt: message.createdAt }))
  const activities = turnActivities(turn, allowActive)
  const run = inferTurnRun(turn, activities, sessionStatus, allowActive)
  const goal = turn.goalRun || goalRun
  const presentation = goalTurnMessages(assistantMessages, goal)
  return <div id={turnElementId(turn.id)} data-ai-turn-id={turn.id} className="scroll-mt-8 space-y-6"><div className="space-y-2" data-slot="user-message-with-references">{references.length ? <div className="ml-auto max-w-[92%]" data-slot="sent-asset-references"><AssetReferenceChips references={references} align="end" /></div> : null}{userContent ? editing ? <div className="ml-auto w-full max-w-[92%] space-y-2"><Textarea value={editedContent} onChange={(event) => onEditChange(event.target.value)} aria-label={l.editMessage} autoFocus className="min-h-24 rounded-2xl border-primary/30 bg-primary/5 text-sm leading-7" /><div className="flex justify-end gap-2"><Button type="button" size="sm" variant="ghost" onClick={onEditCancel}>{l.cancel}</Button><Button type="button" size="sm" onClick={onEditSubmit} disabled={!editedContent.trim() || allowActive}><Send className="mr-1.5 h-3.5 w-3.5" />{l.send}</Button></div></div> : <UserMessage content={userContent} createdAt={userCreatedAt} onEdit={editable ? () => onEditStart(turn) : undefined} onQuote={() => onQuote({ content: userContent, role: 'user' })} /> : null}{imageCount > 0 && sessionId && turn.userMessage?.id ? <div className="ml-auto max-w-[92%]"><MessageImages sessionId={sessionId} messageId={turn.userMessage.id} count={imageCount} /></div> : null}</div>{run || presentation.completed ? <ThinkingDisclosure totalTokens={turn.totalTokens} run={run} activities={activities} fallbackLabel={fallbackLabel} completedGoal={presentation.completed ? goal : null} stageMessages={presentation.details} onQuote={onQuote} /> : null}{presentation.conclusions.map((message, index) => <AssistantMessage key={message.id} content={message.content} createdAt={message.createdAt} onQuote={() => onQuote({ content: message.content, role: 'assistant' })} reportAttachment={index === presentation.conclusions.length - 1 ? reportAttachment : undefined} onDownloadReport={index === presentation.conclusions.length - 1 ? onDownloadReport : undefined} />)}</div>
})

function EmptyConversation({ bottomInset }: { bottomInset: number }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  return <div className="flex h-full items-center justify-center overflow-y-auto px-5 py-4 md:px-6" style={{ paddingBottom: bottomInset + 32, overflowAnchor: 'none' }}><div className="my-auto max-w-lg text-center"><div className="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl border border-primary/30 bg-primary/10 text-primary shadow-sm"><Sparkles className="h-5 w-5" /></div><h2 className="mt-3 text-lg md:mt-5 md:text-xl font-semibold tracking-tight text-foreground">{l.emptyTitle}</h2><p className="mt-2 text-sm leading-6 text-muted-foreground">{l.emptyDescription}</p></div></div>
}

const bottomAnchorThreshold = 96
const emptyConversationTurns: AIWorkbenchConversationTurn[] = []
const emptyRoundIndex: NonNullable<AIWorkbenchPenTestSessionSnapshot['roundIndex']> = []

const ConversationStream = memo(function ConversationStream({ session, liveAssistantText, working, bottomInset, editingMessageId, editedContent, onEditStart, onEditChange, onEditCancel, onEditSubmit, onQuote, onSideChat, onDownloadReport }: { session: AIWorkbenchPenTestSessionSnapshot | null; liveAssistantText: string; working: boolean; bottomInset: number; editingMessageId: string | null; editedContent: string; onEditStart: (turn: AIWorkbenchConversationTurn) => void; onEditChange: (value: string) => void; onEditCancel: () => void; onEditSubmit: () => void; onQuote: (quote: MessageQuote) => void; onSideChat: (selectedText: string) => void; onDownloadReport: () => void }) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const deferredLiveText = useDeferredValue(liveAssistantText)
  const scrollHostRef = useRef<HTMLDivElement | null>(null)
  const atBottomRef = useRef(true)
  const lastScrollTopRef = useRef(0)
  const navigationLockRef = useRef(0)
  const contentRef = useRef<HTMLDivElement | null>(null)
  const [activeTurnId, setActiveTurnId] = useState<string | null>(null)
  const [showBottomAnchor, setShowBottomAnchor] = useState(false)
  const [bottomAnchorOffset, setBottomAnchorOffset] = useState(0)
  const [viewport, setViewport] = useState<HTMLDivElement | null>(null)
  const [selectionQuoteAction, setSelectionQuoteAction] = useState<{ text: string; x: number; y: number; role: 'user' | 'assistant' } | null>(null)
  const [expandedTurns, setExpandedTurns] = useState<Record<string, AIWorkbenchConversationTurn>>({})
  const pendingTurnScrollRef = useRef<string | null>(null)
  // Rounds paged in from before the snapshot window, oldest first.
  const [earlierTurns, setEarlierTurns] = useState<AIWorkbenchConversationTurn[]>([])
  const [loadingEarlier, setLoadingEarlier] = useState(false)
  const [loadingRailId, setLoadingRailId] = useState<string | null>(null)
  const sessionKeyRef = useRef<string | null>(null)
  const currentSessionRef = useRef(session?.task.id)
  currentSessionRef.current = session?.task.id
  const pagingRequestRef = useRef<symbol | null>(null)
  const prependAnchorRef = useRef<{ id: string; offset: number } | null>(null)
  useEffect(() => {
    const id = session?.task.id
    // A transient null (snapshot refetch) must not reset the loaded rounds, otherwise
    // the rail drops back to all-unloaded mid-stream. Only a real identity change resets.
    if (!id || id === sessionKeyRef.current) return
    sessionKeyRef.current = id
    setExpandedTurns({})
    setEarlierTurns([])
    setLoadingEarlier(false)
    setLoadingRailId(null)
    pendingTurnScrollRef.current = null
    prependAnchorRef.current = null
    pagingRequestRef.current = null
  }, [session?.task.id])
  const baseTurns = useMemo(
    () => (Array.isArray(session?.turns) ? session.turns : emptyConversationTurns),
    [session?.turns],
  )
  const roundIndex = session?.roundIndex || emptyRoundIndex
  useEffect(() => {
    if (roundIndex.length) setEarlierTurns(previous => mergeConversationTurns(previous, baseTurns, roundIndex))
  }, [baseTurns, roundIndex])
  // Older turns arrive as previews with bodyTruncated; a rail jump pulls the full
  // turn and overlays it here, so the rail itself stays complete.
  const turns = useMemo(() => {
    const resolved = Object.keys(expandedTurns).length ? baseTurns.map(turn => expandedTurns[turn.id] ?? turn) : baseTurns
    return mergeConversationTurns(sessionKeyRef.current === session?.task.id ? earlierTurns : [], resolved, roundIndex)
  }, [baseTurns, earlierTurns, expandedTurns, roundIndex, session?.task.id])
  const oldestRoundIndex = roundIndex.findIndex(entry => entry.id === turns[0]?.id)
  const hasEarlierRounds = oldestRoundIndex > 0
  const messageItems = useMemo(() => session?.chatItems?.filter((item) => item.type === 'message' && item.message?.content) || [], [session?.chatItems])
  const legacyTurns = useMemo(() => {
    const grouped: Array<{ id: string; userContent: string; userCreatedAt?: string; assistantMessages: TurnAssistantMessage[] }> = []
    let current: { id: string; userContent: string; userCreatedAt?: string; assistantMessages: TurnAssistantMessage[] } | null = null
    messageItems.forEach((item) => {
      const message = item.message
      if (!message?.content) return
      if (message.role === 'user') {
        if (current) grouped.push(current)
        current = { id: item.id, userContent: message.content, userCreatedAt: item.createdAt, assistantMessages: [] }
        return
      }
      if (message.role === 'assistant') {
        if (!current) current = { id: item.id, userContent: '', userCreatedAt: item.createdAt, assistantMessages: [] }
        current.assistantMessages.push({ id: item.id, content: message.content, createdAt: item.createdAt })
      }
    })
    if (current) grouped.push(current)
    return grouped
  }, [messageItems])
  const active = sessionIsActive(session)
  const sessionStatus = session?.task.status
  const latestTurnIndex = Math.max(0, turns.length - 1)
  const hasActiveTurn = turns.some((turn, index) => turnHasActiveThinking(turn, active, index === latestTurnIndex))
  const apiCoverage = session?.apiCoverage?.counts
  const rows = useMemo<ConversationRow[]>(() => {
    const list: ConversationRow[] = turns.length
      ? turns.map(turn => ({ kind: 'turn' as const, key: turn.id, turn }))
      : legacyTurns.map(turn => ({ kind: 'legacy' as const, key: turn.id, turn }))
    if (deferredLiveText) list.push({ kind: 'live', key: 'live-assistant' })
    if ((apiCoverage && apiCoverage.total > 0) || (working && !hasActiveTurn)) list.push({ kind: 'footer', key: 'conversation-footer' })
    return list
  }, [apiCoverage, deferredLiveText, hasActiveTurn, legacyTurns, turns, working])
  const railEntries = useMemo(() => {
    const fallback = (status?: string) => {
      if (status === 'waiting_approval') return l.approvalPending
      if (status && activeStatuses.has(status)) return l.processing
      if (status === 'failed') return l.failedSummary
      if (status === 'blocked') return l.incompleteSummary
      if (status === 'cancelled' || status === 'paused') return l.stoppedSummary
      return l.noSummary
    }
    // The planner keeps one conversation for the whole run, so the round index (not
    // the loaded turns) is what makes the rail complete. Rounds outside the loaded
    // window render as unloaded ticks that load on click.
    if (roundIndex.length) {
      const loaded = new Map(turns.map(turn => [turn.id, turn]))
      return roundIndex.map((entry, index) => {
        const turn = loaded.get(entry.id)
        const messages = turn ? (turn.assistantMessageViews?.length ? turn.assistantMessageViews : turn.assistantMessages || []) : []
        const question = turn?.userMessageView?.content || turn?.userMessage?.content || entry.label || formatWorkbenchText(l.turnNumber, { count: index + 1 })
        const latest = index === roundIndex.length - 1
        return {
          id: entry.id,
          number: entry.roundNumber,
          question: turnPreviewText(question, 160) || formatWorkbenchText(l.turnNumber, { count: index + 1 }),
          // Unloaded rounds still preview their conclusion from the rail index, so
          // hovering the rail is informative before the body is fetched.
          summary: turn
            ? turnResultPreview(latest && liveAssistantText ? [...messages, { content: liveAssistantText }] : messages, fallback(latest && active ? sessionStatus : turn.run?.status))
            : entry.preview || '',
          loaded: Boolean(turn),
          busy: loadingRailId === entry.id,
        }
      })
    }
    if (turns.length) return turns.map((turn, index) => {
      const messages = turn.assistantMessageViews?.length ? turn.assistantMessageViews : turn.assistantMessages || []
      const latest = index === turns.length - 1
      return {
        id: turn.id,
        question: turnPreviewText(turn.userMessageView?.content || turn.userMessage?.content || '', 160) || formatWorkbenchText(l.turnNumber, { count: index + 1 }),
        summary: turnResultPreview(latest && liveAssistantText ? [...messages, { content: liveAssistantText }] : messages, fallback(latest && active ? sessionStatus : turn.run?.status)),
      }
    })
    return legacyTurns.map((turn, index) => ({
      id: turn.id,
      question: turnPreviewText(turn.userContent, 160) || formatWorkbenchText(l.turnNumber, { count: index + 1 }),
      summary: turnResultPreview(index === legacyTurns.length - 1 && liveAssistantText ? [...turn.assistantMessages, { content: liveAssistantText }] : turn.assistantMessages, fallback(index === legacyTurns.length - 1 ? sessionStatus : undefined)),
    }))
  }, [turns, legacyTurns, liveAssistantText, active, sessionStatus, l, roundIndex, loadingRailId])
  const getViewport = useCallback(() => scrollHostRef.current?.querySelector('[data-slot="scroll-area-viewport"]') as HTMLDivElement | null, [])
  useEffect(() => {
    setViewport(getViewport())
  }, [getViewport, session?.task.id])
  const listRef = useRef<HTMLDivElement | null>(null)
  const [listOffset, setListOffset] = useState(32)
  useLayoutEffect(() => {
    const list = listRef.current
    if (!list || !viewport) return
    const measure = () => setListOffset(list.getBoundingClientRect().top - viewport.getBoundingClientRect().top + viewport.scrollTop)
    measure()
    const observer = new ResizeObserver(measure)
    if (contentRef.current) observer.observe(contentRef.current)
    return () => observer.disconnect()
  }, [viewport, hasEarlierRounds, loadingEarlier])
  // Change the key function only when the ordered ids change. Keep measured
  // heights by id when prepending; a permanently stable function hides reorders.
  const rowKeys = JSON.stringify(rows.map(row => row.key))
  const virtualItemKey = useMemo(() => {
    const keys: string[] = JSON.parse(rowKeys)
    return (index: number) => keys[index] ?? index
  }, [rowKeys])
  const virtualItemSize = useCallback(() => 1040, [])
  // eslint-disable-next-line react-hooks/incompatible-library
  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    enabled: Boolean(session && viewport),
    getScrollElement: () => viewport,
    estimateSize: virtualItemSize,
    overscan: 3,
    getItemKey: virtualItemKey,
    scrollMargin: listOffset,
  })
  rowVirtualizer.shouldAdjustScrollPositionOnItemSizeChange = (item, _delta, instance) =>
    !prependAnchorRef.current && item.start < (instance.scrollOffset ?? 0)
  const virtualItems = rowVirtualizer.getVirtualItems()
  const getBottomAnchorOffset = useCallback(() => {
    const host = scrollHostRef.current
    const composer = host?.closest('section')?.querySelector<HTMLElement>('[data-slot="ai-workbench-composer-shell"]')
    if (!host || !composer) return bottomInset + 20
    return Math.max(12, Math.ceil(host.getBoundingClientRect().bottom - composer.getBoundingClientRect().top + 12))
  }, [bottomInset])
  const updateBottomState = useCallback(() => {
    const viewport = getViewport()
    if (!viewport) return
    const distanceFromBottom = viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop
    const atBottom = distanceFromBottom <= bottomAnchorThreshold
    setShowBottomAnchor(!atBottom && viewport.scrollHeight > viewport.clientHeight + bottomAnchorThreshold)
    setBottomAnchorOffset(getBottomAnchorOffset())
    const readingOffset = viewport.scrollTop + Math.min(160, Math.max(0, viewport.clientHeight - bottomInset) * 0.34)
    const current = readingTurnId(rowVirtualizer.getVirtualItems(), rows, readingOffset, atBottom)
    if (current) setActiveTurnId(current)
  }, [getViewport, getBottomAnchorOffset, bottomInset, rowVirtualizer, rows])
  useLayoutEffect(() => {
    updateBottomState()
  }, [virtualItems, updateBottomState])
  const scrollToBottom = useCallback((behavior: ScrollBehavior = 'auto') => {
    const viewport = getViewport()
    if (!viewport) return
    navigationLockRef.current = 0
    viewport.scrollTo({ top: viewport.scrollHeight, behavior })
    atBottomRef.current = true
    lastScrollTopRef.current = viewport.scrollTop
    setShowBottomAnchor(false)
  }, [getViewport])
  const scrollToTurn = useCallback((turnId: string) => {
    const viewport = getViewport()
    if (!viewport) return
    const index = rows.findIndex(row => (row.kind === 'turn' || row.kind === 'legacy') && row.turn.id === turnId)
    if (index < 0) return
    navigationLockRef.current = Date.now() + 1000
    atBottomRef.current = false
    setActiveTurnId(turnId)
    rowVirtualizer.scrollToIndex(index, { align: 'start' })
    window.requestAnimationFrame(updateBottomState)
  }, [getViewport, rows, rowVirtualizer, updateBottomState])
  useEffect(() => {
    const target = pendingTurnScrollRef.current
    if (!target) return
    pendingTurnScrollRef.current = null
    scrollToTurn(target)
  }, [rows, scrollToTurn])
  const captureReadingAnchor = useCallback(() => {
    const viewport = getViewport()
    if (!viewport) return
    const visible = rowVirtualizer.getVirtualItems().find(item => item.end > viewport.scrollTop)
    if (visible && rows[visible.index]) {
      prependAnchorRef.current = { id: rows[visible.index].key, offset: viewport.scrollTop - visible.start }
    }
  }, [getViewport, rowVirtualizer, rows])
  // Hold the reading row while newly mounted rows acquire real heights. Native
  // scroll anchoring and virtualizer compensation must not both adjust this jump.
  useLayoutEffect(() => {
    const anchor = prependAnchorRef.current
    const viewport = getViewport()
    if (!anchor || !viewport) return
    let frame = 0
    let stable = 0
    let attempts = 0
    const restore = () => {
      if (prependAnchorRef.current !== anchor) return
      const index = rows.findIndex(row => row.key === anchor.id)
      const offset = rowVirtualizer.getOffsetForIndex(index, 'start')
      if (index < 0 || !offset) { prependAnchorRef.current = null; return }
      const target = Math.max(0, offset[0] + anchor.offset)
      if (Math.abs(viewport.scrollTop - target) > 1) {
        stable = 0
        rowVirtualizer.scrollToOffset(target)
      } else stable++
      if (stable >= 3 || ++attempts >= 30) { prependAnchorRef.current = null; return }
      frame = window.requestAnimationFrame(restore)
    }
    const cancel = () => { prependAnchorRef.current = null }
    viewport.addEventListener('wheel', cancel, { passive: true })
    viewport.addEventListener('touchstart', cancel, { passive: true })
    restore()
    return () => {
      window.cancelAnimationFrame(frame)
      viewport.removeEventListener('wheel', cancel)
      viewport.removeEventListener('touchstart', cancel)
    }
  }, [rows, rowVirtualizer, getViewport])
  const fetchEarlierRounds = useCallback(async (targetId?: string) => {
    const sessionId = session?.task.id
    if (!sessionId || oldestRoundIndex <= 0 || pagingRequestRef.current) return
    const request = Symbol('round-page')
    pagingRequestRef.current = request
    setLoadingEarlier(true)
    if (targetId) setLoadingRailId(targetId)
    const targetIndex = targetId ? roundIndex.findIndex(entry => entry.id === targetId) : Math.max(0, oldestRoundIndex - 8)
    let cursor = oldestRoundIndex
    let fetched: AIWorkbenchConversationTurn[] = []
    try {
      while (cursor > Math.max(0, targetIndex)) {
        const chunk = await getAIWorkbenchEarlierRounds(sessionId, roundIndex[cursor].id, Math.min(60, cursor - Math.max(0, targetIndex)))
        if (currentSessionRef.current !== sessionId || pagingRequestRef.current !== request) return
        const next = roundIndex.findIndex(entry => entry.id === chunk.turns[0]?.id)
        if (next < 0 || next >= cursor) throw new Error(l.loadEarlier)
        fetched = mergeConversationTurns(chunk.turns, fetched, roundIndex)
        cursor = next
        if (!chunk.hasMore) break
      }
      if (targetId) pendingTurnScrollRef.current = targetId
      else captureReadingAnchor()
      atBottomRef.current = false
      setEarlierTurns(previous => mergeConversationTurns(previous, fetched, roundIndex))
    } catch (error) {
      if (currentSessionRef.current === sessionId) toast.error(workbenchErrorMessage(error, l, l.loadSessionFailed))
    } finally {
      if (pagingRequestRef.current === request) {
        pagingRequestRef.current = null
        setLoadingEarlier(false)
        setLoadingRailId(null)
      }
    }
  }, [session?.task.id, oldestRoundIndex, roundIndex, captureReadingAnchor, l])
  const loadEarlierRounds = useCallback(() => fetchEarlierRounds(), [fetchEarlierRounds])
  const handleRailNavigate = useCallback(async (turnId: string) => {
    if (pagingRequestRef.current) return
    const sessionId = session?.task.id
    const turn = turns.find(candidate => candidate.id === turnId)
    if (!turn && roundIndex.length) { await fetchEarlierRounds(turnId); return }
    if (turn?.bodyTruncated && !expandedTurns[turnId] && sessionId) {
      try {
        const full = await getAIWorkbenchSessionTurn(sessionId, turnId)
        if (currentSessionRef.current !== sessionId) return
        pendingTurnScrollRef.current = turnId
        setExpandedTurns(previous => ({ ...previous, [turnId]: full }))
        return
      } catch (error) {
        if (currentSessionRef.current === sessionId) toast.error(workbenchErrorMessage(error, l, l.loadSessionFailed))
        return
      }
    }
    scrollToTurn(turnId)
  }, [session?.task.id, turns, roundIndex.length, expandedTurns, fetchEarlierRounds, scrollToTurn, l])
  const handleMessageMouseUp = useCallback(() => {
    window.requestAnimationFrame(() => {
      const selection = window.getSelection()
      const text = selection?.toString().trim() || ''
      const range = selection?.rangeCount ? selection.getRangeAt(0) : null
      const anchor = range?.commonAncestorContainer instanceof Element ? range.commonAncestorContainer : range?.commonAncestorContainer.parentElement
      const message = anchor?.closest<HTMLElement>('[data-ai-message-quote]')
      if (!text || !range || !message) { setSelectionQuoteAction(null); return }
      const rect = range.getBoundingClientRect()
      if (!rect.width && !rect.height) { setSelectionQuoteAction(null); return }
      setSelectionQuoteAction({ text, x: Math.max(12, rect.left + rect.width / 2), y: Math.max(12, rect.bottom + 8), role: message.dataset.aiMessageQuote === 'user' ? 'user' : 'assistant' })
    })
  }, [])
  useEffect(() => {
    const viewport = getViewport()
    if (!viewport) return
    let frame = window.requestAnimationFrame(updateBottomState)
    const scheduleUpdate = () => {
      window.cancelAnimationFrame(frame)
      frame = window.requestAnimationFrame(updateBottomState)
    }
    const observer = new ResizeObserver(() => {
      if (atBottomRef.current && Date.now() >= navigationLockRef.current) {
        viewport.scrollTop = viewport.scrollHeight
        lastScrollTopRef.current = viewport.scrollTop
      }
      scheduleUpdate()
    })
    observer.observe(viewport)
    if (contentRef.current) observer.observe(contentRef.current)
    const host = scrollHostRef.current
    const composer = host?.closest('section')?.querySelector<HTMLElement>('[data-slot="ai-workbench-composer-shell"]')
    if (composer) observer.observe(composer)
    const handleScroll = () => {
      const distanceFromBottom = viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop
      // Content growth can increase this distance without the reader scrolling away.
      if (Date.now() < navigationLockRef.current) atBottomRef.current = false
      else if (distanceFromBottom <= bottomAnchorThreshold) atBottomRef.current = true
      else if (viewport.scrollTop < lastScrollTopRef.current - 1) atBottomRef.current = false
      lastScrollTopRef.current = viewport.scrollTop
      scheduleUpdate()
    }
    viewport.addEventListener('scroll', handleScroll, { passive: true })
    window.addEventListener('resize', scheduleUpdate)
    return () => {
      window.cancelAnimationFrame(frame)
      observer.disconnect()
      viewport.removeEventListener('scroll', handleScroll)
      window.removeEventListener('resize', scheduleUpdate)
    }
  }, [getViewport, updateBottomState, session?.task.id])
  useEffect(() => {
    atBottomRef.current = true
    lastScrollTopRef.current = 0
    navigationLockRef.current = 0
    let secondFrame: number | null = null
    const firstFrame = window.requestAnimationFrame(() => {
      setActiveTurnId(null)
      scrollToBottom('auto')
      secondFrame = window.requestAnimationFrame(() => scrollToBottom('auto'))
    })
    return () => {
      window.cancelAnimationFrame(firstFrame)
      if (secondFrame !== null) window.cancelAnimationFrame(secondFrame)
    }
  }, [scrollToBottom, session?.task.id])
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (atBottomRef.current) scrollToBottom('auto')
      else updateBottomState()
    })
    return () => window.cancelAnimationFrame(frame)
  }, [bottomInset, liveAssistantText.length, messageItems.length, scrollToBottom, session?.updatedAt, turns.length, updateBottomState, working])
  if (!session) return <EmptyConversation bottomInset={bottomInset} />
  // An active conversation with no turns is still being prepared; keep an
  // explicit, animated status instead of a bare sentence that looks final.
  const legacyReadOnly = Boolean(session.task.planRunId && !isOrchestrationRunId(session.task.planRunId))
  const preparingActive = !legacyReadOnly && sessionIsActive(session)
  const preparingLabel = legacyReadOnly
    ? l.legacyOrchestrationReadOnly
    : (session.task.agentRole === 'main' || session.task.agentRole === 'controller' || session.task.agentRole === 'mainagent')
      ? l.mainAgentPreparing
      : l.sessionPreparing
  return <div ref={scrollHostRef} className="relative h-full min-w-0 max-w-full">
    {/* Radix uses a table wrapper by default; constrain it so long content cannot widen the conversation. */}
    <ScrollArea className="h-full w-full min-w-0 max-w-full [&_[data-radix-scroll-area-viewport]>div]:!block [&_[data-radix-scroll-area-viewport]>div]:!w-full [&_[data-radix-scroll-area-viewport]>div]:!min-w-0 [&_[data-radix-scroll-area-viewport]>div]:!max-w-full">
      <div ref={contentRef} onMouseUp={handleMessageMouseUp} onKeyUp={handleMessageMouseUp} className="relative mx-auto min-w-0 w-full max-w-full py-8 pl-12 pr-5 sm:pl-16 sm:pr-8 2xl:pl-20 2xl:pr-12" style={{ paddingBottom: bottomInset + 32, overflowAnchor: 'none' }}>
        {/* Sits in the flow above the oldest loaded round, matching the DSH boundary
            control: it scrolls with the content instead of covering messages. */}
        {session.task.agentRole === 'planner' && (hasEarlierRounds || loadingEarlier) ? (
          <div className="flex justify-center pb-6">
            <button
              type="button"
              onClick={() => void loadEarlierRounds()}
              disabled={loadingEarlier}
              className="inline-flex items-center gap-2 rounded-md border border-border/70 bg-muted/30 px-4 py-1.5 text-xs text-muted-foreground transition hover:bg-muted/60 disabled:cursor-progress disabled:opacity-70"
            >
              {loadingEarlier ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : null}
              {loadingEarlier ? l.loadingEarlier : l.loadEarlier}
            </button>
          </div>
        ) : null}
        {rows.length ? <div ref={listRef} style={{ height: rowVirtualizer.getTotalSize(), position: 'relative', width: '100%' }}>
          {virtualItems.map(item => {
            const row = rows[item.index]
            if (!row) return null
            let content: React.ReactNode = null
            if (row.kind === 'turn') {
              const index = turns.indexOf(row.turn)
              const latest = index === latestTurnIndex
              content = <ConversationTurn turn={row.turn} sessionId={session.task.id} goalRun={latest && session.goalRun?.mode === 'goal' ? session.goalRun : null} sessionStatus={latest ? sessionStatus : undefined} allowActive={working && latest} fallbackLabel={latest && working && ['queued', 'cancelling'].includes(displayText(sessionStatus).toLowerCase()) ? (session.currentActivity?.label || session.agentState?.currentActivity?.label) : undefined} editable={latest && Boolean(row.turn.userMessage?.id) && !working && !session.task.planRunId} editing={editingMessageId === row.turn.userMessage?.id} editedContent={editedContent} onEditStart={onEditStart} onEditChange={onEditChange} onEditCancel={onEditCancel} onEditSubmit={onEditSubmit} onQuote={onQuote} reportAttachment={latest ? session.reportPreview?.attachment : undefined} onDownloadReport={latest ? onDownloadReport : undefined} />
            } else if (row.kind === 'legacy') {
              const index = legacyTurns.indexOf(row.turn)
              content = <LegacyMessageTurn id={row.turn.id} userContent={row.turn.userContent} userCreatedAt={row.turn.userCreatedAt} assistantMessages={row.turn.assistantMessages} onQuote={onQuote} reportAttachment={index === legacyTurns.length - 1 ? session.reportPreview?.attachment : undefined} onDownloadReport={index === legacyTurns.length - 1 ? onDownloadReport : undefined} />
            } else if (row.kind === 'live') {
              content = <AssistantMessage content={deferredLiveText} createdAt={new Date().toISOString()} onQuote={() => onQuote({ content: deferredLiveText, role: 'assistant' })} />
            } else {
              content = <div className="space-y-6">
                {apiCoverage && apiCoverage.total > 0 ? <div className="flex flex-wrap items-center gap-x-4 gap-y-1 rounded-xl border border-border/70 bg-muted/20 px-4 py-3 text-xs text-muted-foreground"><span className="font-medium text-foreground">{l.apiCoverage}</span><span>{l.candidates}{' '}{apiCoverage.total}</span><span>{l.testedEvidence}{' '}{apiCoverage.tested}</span><span>{l.planBlocked}{' '}{apiCoverage.blocked}</span><span>{l.pending}{' '}{apiCoverage.pending}</span></div> : null}
                {working && !hasActiveTurn ? <ThinkingDisclosure totalTokens={turns.at(-1)?.totalTokens} run={session.agentState?.activeRun} activities={getActivities(session.chatItems)} fallbackActive fallbackLabel={session.currentActivity?.label || session.agentState?.currentActivity?.label} /> : null}
              </div>
            }
            return <div key={item.key} data-index={item.index} ref={rowVirtualizer.measureElement} className="absolute left-0 top-0 w-full scroll-mt-8 pb-6" style={{ transform: `translateY(${item.start - listOffset}px)` }}>{content}</div>
          })}
        </div> : <div role={preparingActive ? 'status' : undefined} className="flex min-h-[40vh] flex-col items-center justify-center gap-2 px-6 text-center text-sm text-muted-foreground">{preparingActive ? <Spinner aria-hidden="true" className="size-4 motion-reduce:animate-none" /> : null}<div>{preparingLabel}</div></div>}
      </div>
    </ScrollArea>
    {selectionQuoteAction && session.task.agentRole !== 'planner' ? <div className="fixed z-50 flex -translate-x-1/2 items-center gap-1 rounded-lg border border-border bg-popover p-1 text-popover-foreground shadow-lg" style={{ left: selectionQuoteAction.x, top: selectionQuoteAction.y }}><Button type="button" size="sm" variant="ghost" className="h-8 rounded-md px-2.5" onMouseDown={(event) => event.preventDefault()} onClick={() => { onQuote({ content: selectionQuoteAction.text, role: selectionQuoteAction.role }); window.getSelection()?.removeAllRanges(); setSelectionQuoteAction(null) }}><MessageSquareQuote className="mr-1.5 h-4 w-4" />{l.addToConversation}</Button><Button type="button" size="sm" variant="ghost" className="h-8 rounded-md px-2.5" onMouseDown={(event) => event.preventDefault()} onClick={() => { onSideChat(selectionQuoteAction.text); window.getSelection()?.removeAllRanges(); setSelectionQuoteAction(null) }}><MessageSquarePlus className="mr-1.5 h-4 w-4" />{l.askSideChat}</Button></div> : null}
    <ConversationRail key={session.task.id} entries={railEntries} activeId={activeTurnId} bottomInset={bottomInset} onNavigate={handleRailNavigate} />
    {showBottomAnchor ? (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            size="icon"
            variant="secondary"
            className="absolute right-8 z-30 h-10 w-10 rounded-full border border-border/80 bg-card/95 text-foreground shadow-lg backdrop-blur hover:bg-muted"
            style={{ bottom: bottomAnchorOffset }}
            aria-label={l.scrollBottom}
            onClick={() => scrollToBottom('smooth')}
          >
            <ChevronDown className="h-4 w-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{l.scrollBottom}</TooltipContent>
      </Tooltip>
    ) : null}
  </div>
})

function CompoundComposer({ composerRef, bootstrap, projects, selectedProjectId, value, attachments, assetReferences, onAssetReferencesChange, mcpTools, onMCPToolsChange, goalMode, goalRun, onGoalModeChange, quote, selectedModel, queuedMessages, hasSession, disabled, working, sending, stopping, creating, onChange, onQuoteChange, onQuoteRemove, onAttachmentsChange, onProjectChange, onModelChange, onQueuedMessageEdit, onQueuedMessageRemove, onQueuedMessageRetry, onSubmit, onStop }: {
  composerRef?: Ref<HTMLDivElement>; bootstrap: AIWorkbenchPenTestSessionBootstrapResponse | null; projects: AIWorkbenchProject[]; selectedProjectId: string; value: string; attachments: ComposerAttachment[]; assetReferences: AssetReferenceSelection[]; onAssetReferencesChange: (value: AssetReferenceSelection[]) => void; mcpTools: CallableMCPTool[]; onMCPToolsChange: (tools: CallableMCPTool[]) => void; goalMode: GoalModeAction; goalRun?: GoalRun | null; onGoalModeChange: (mode: GoalModeAction) => void; quote: MessageQuote | null; selectedModel: string; queuedMessages: QueuedWorkbenchMessage[]; hasSession: boolean; disabled: boolean; working: boolean; sending: boolean; stopping: boolean; creating: boolean; onChange: (value: string) => void; onQuoteChange: (value: MessageQuote) => void; onQuoteRemove: () => void; onAttachmentsChange: (attachments: ComposerAttachment[]) => void; onProjectChange: (value: string) => void; onModelChange: (value: string) => void; onQueuedMessageEdit: (id: string) => void; onQueuedMessageRemove: (id: string) => void; onQueuedMessageRetry: (id: string) => void; onSubmit: () => void; onStop: () => void
}) {
  const { t, language } = useI18n()
  const l = t.aiWorkbench
  const placeholder = !hasSession && !selectedProjectId ? l.createProjectFirst : goalMode !== 'chat' ? (language === 'zh' ? '描述你的目标，定义可衡量的成果，以获得最佳效果。' : 'Describe your goal and define measurable outcomes for the best results.') : l.composerPlaceholder
  const [command, setCommand] = useState<ComposerCommand | null>(null)
  const [referenceTask, setReferenceTask] = useState<ReferenceTask | null>(null)
  const [toolCatalog, setToolCatalog] = useState<CallableMCPTool[] | null>(null)
  const [toolCatalogError, setToolCatalogError] = useState('')
  const [toolCatalogRevision, setToolCatalogRevision] = useState(0)
  const toolMenuOpen = command?.kind === '/'
  useEffect(() => {
    if (!toolMenuOpen) return
    const controller = new AbortController()
    void listCallableMCPTools(controller.signal).then(tools => {
      if (!controller.signal.aborted) setToolCatalog(tools)
    }).catch(error => {
      if (!controller.signal.aborted) setToolCatalogError(error instanceof Error ? error.message : String(error))
    })
    return () => controller.abort()
  }, [toolMenuOpen, toolCatalogRevision])
  const commandMenuRef = useRef<ComposerCommandMenuHandle>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const commandPointerDownRef = useRef(false)
  const dismissCommands = () => { setCommand(null); setReferenceTask(null); setToolCatalog(null); setToolCatalogError('') }
  const focusComposer = () => requestAnimationFrame(() => textareaRef.current?.focus())
  const consumeCommand = () => {
    if (!command) return
    onChange(value.slice(0, command.start) + value.slice(command.end))
    const cursor = command.start
    dismissCommands()
    requestAnimationFrame(() => { textareaRef.current?.focus(); textareaRef.current?.setSelectionRange(cursor, cursor) })
  }
  const updateCommand = (next: string, cursor: number) => {
    const token = composerCommandAt(next, cursor)
    if (token?.kind === '/' && !toolMenuOpen) { setToolCatalog(null); setToolCatalogError('') }
    setReferenceTask(null)
    setCommand(token?.kind === '@' && assetReferences.length >= 12 ? null : token)
  }
  const chooseMode = (action: GoalModeAction) => { onGoalModeChange(action); consumeCommand() }
  const chooseTool = (tool: CallableMCPTool) => {
    if (!mcpTools.some(selected => selected.name === tool.name)) onMCPToolsChange([...mcpTools, tool])
    consumeCommand()
  }
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [editingQuote, setEditingQuote] = useState(false)
  const [quoteDraft, setQuoteDraft] = useState(quote?.content || '')
  const models = bootstrap?.modelOptions.options || []
  useEffect(() => {
    setEditingQuote(false)
    setQuoteDraft(quote?.content || '')
  }, [quote?.content, quote?.role])
  const addFiles = useCallback(async (files: File[]) => {
    const validFiles = files.filter((file) => {
      if (file.size <= maxAttachmentSize) return true
      toast.error(formatWorkbenchText(l.fileTooLarge, { name: file.name }))
      return false
    })
    const room = Math.max(0, 6 - attachments.length)
    if (validFiles.length > room) toast.error(l.attachmentLimit)
    const accepted = validFiles.slice(0, room)
    const currentBytes = attachments.reduce((total, attachment) => total + attachment.size, 0)
    const nextBytes = accepted.reduce((total, file) => total + file.size, 0)
    if (currentBytes + nextBytes > maxTotalAttachmentSize) {
      toast.error(l.attachmentTotalTooLarge)
      return
    }
    try {
      const next = await Promise.all(accepted.map((file) => new Promise<ComposerAttachment>((resolve, reject) => {
        const reader = new FileReader()
        reader.onload = () => resolve({ id: attachmentId(), name: file.name || l.pastedImage, type: file.type || 'application/octet-stream', size: file.size, dataUrl: String(reader.result || '') })
        reader.onerror = () => reject(reader.error || new Error('read failed'))
        reader.readAsDataURL(file)
      })))
      if (next.length) onAttachmentsChange([...attachments, ...next])
    } catch {
      toast.error(l.attachmentReadFailed)
    }
  }, [attachments, onAttachmentsChange, l])
  const handlePaste = (event: ClipboardEvent<HTMLTextAreaElement>) => {
    const files = Array.from(event.clipboardData.items).filter((item) => item.kind === 'file').map((item) => item.getAsFile()).filter((file): file is File => Boolean(file))
    if (!files.length) return
    event.preventDefault(); void addFiles(files)
  }
  const handleFileChange = (event: ChangeEvent<HTMLInputElement>) => { const files = Array.from(event.target.files || []); if (files.length) void addFiles(files); event.target.value = '' }
  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (command && !referenceTask && !event.nativeEvent.isComposing && commandMenuRef.current?.handleKey(event)) return
    if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); if (!disabled && (working ? hasSession : !sending) && !creating && !stopping) onSubmit() } }
  const canSubmit = Boolean(value.trim() || quote?.content.trim() || assetReferences.length || mcpTools.length || attachments.length)
  const showGoalCompose = Boolean(goalRun && goalRun.mode === 'goal' && goalRun.status !== 'completed')
  const editQueuedMessage = (id: string) => {
    if (value.trim() || quote || attachments.length || assetReferences.length || mcpTools.length) {
      toast.error(l.queueEditBlocked)
      return
    }
    onQueuedMessageEdit(id)
    requestAnimationFrame(() => textareaRef.current?.focus())
  }
  return <div ref={composerRef} onBlurCapture={(event) => {
    const composer = event.currentTarget
    const relatedTarget = event.relatedTarget as Node | null
    if (composer.contains(relatedTarget) || (relatedTarget instanceof Element && relatedTarget.closest('[data-composer-command-portal]'))) return
    requestAnimationFrame(() => {
      if (commandPointerDownRef.current) return
      const activeElement = document.activeElement
      if (composer.contains(activeElement) || (activeElement instanceof Element && activeElement.closest('[data-composer-command-portal]'))) return
      dismissCommands()
    })
  }} className={cn('pointer-events-none absolute inset-x-0 bottom-0 z-20 bg-gradient-to-t from-background via-background/95 to-transparent px-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] md:px-5 md:pb-4', hasSession ? 'pt-2 md:pt-4' : 'pt-2 md:pt-12')}>
    {command && !disabled ? <div className="pointer-events-auto absolute inset-x-3 bottom-full z-40 mb-2 md:inset-x-5" data-slot="composer-command-popup" data-composer-command-portal onPointerDownCapture={() => { commandPointerDownRef.current = true }} onPointerUpCapture={() => { requestAnimationFrame(() => { commandPointerDownRef.current = false }) }} onPointerCancelCapture={() => { commandPointerDownRef.current = false }}>
      {referenceTask ? <AssetReferencePicker inline initialTask={referenceTask} open onOpenChange={(open) => { if (!open) { dismissCommands(); focusComposer() } }} onSelect={(reference) => { onAssetReferencesChange([...assetReferences, reference]); consumeCommand() }} /> : <ComposerCommandMenu key={`${command.kind}:${command.start}:${command.query}`} ref={commandMenuRef} command={command} onMode={chooseMode} onTask={setReferenceTask} tools={toolCatalog} toolError={toolCatalogError} onTool={chooseTool} onReloadTools={() => { setToolCatalog(null); setToolCatalogError(''); setToolCatalogRevision(revision => revision + 1) }} onDismiss={dismissCommands} />}
    </div> : null}
    {!hasSession ? <div data-slot="ai-workbench-project-strip" className="pointer-events-auto mx-auto flex w-[calc(100%-1rem)] md:w-[calc(100%-3rem)] max-w-none items-center gap-2 overflow-hidden rounded-t-2xl border border-b-0 border-[var(--input-panel-border)] bg-[var(--input-panel-bg)] px-2 py-1 text-sm md:px-4 md:py-2.5 shadow-[0_10px_34px_rgba(0,0,0,0.16)] backdrop-blur-xl">
      <Select value={selectedProjectId} onValueChange={onProjectChange} disabled={working || !projects.length}>
        <SelectTrigger data-slot="ai-workbench-project-trigger" className="h-8 w-auto min-w-0 gap-2 rounded-lg border-0 !bg-transparent px-3 text-sm font-medium !shadow-none hover:!bg-[var(--input-panel-action-bg-hover)] focus:ring-0" style={{ maxWidth: 'min(100%, 420px)' }}>
          <Folder className="h-4 w-4 shrink-0 text-muted-foreground" /><SelectValue placeholder={l.projectRequired} />
        </SelectTrigger>
        <SelectContent align="start">
          {projects.map((project) => <SelectItem key={project.id} value={project.id}>{project.name || l.noProject}</SelectItem>)}
        </SelectContent>
      </Select>
    </div> : null}
    {showGoalCompose || queuedMessages.length ? <div
      data-slot="ai-workbench-composer-context"
      className="pointer-events-auto relative z-0 mx-auto -mb-4 w-[calc(100%-2rem)] divide-y divide-[var(--input-panel-action-border)] rounded-t-2xl border border-b-0 border-[var(--input-panel-border)] bg-[var(--input-panel-bg)] pb-5 shadow-[0_-10px_30px_rgba(0,0,0,0.14)] backdrop-blur-xl md:w-[calc(100%-3rem)]"
    >
      {showGoalCompose ? <GoalModeCard goal={goalRun} variant="compose" /> : null}
      {queuedMessages.length ? <ComposerQueuedMessages messages={queuedMessages} onEdit={editQueuedMessage} onRemove={onQueuedMessageRemove} onRetry={onQueuedMessageRetry} /> : null}
    </div> : null}
    <div data-slot="ai-workbench-composer-shell" className="relative z-10 max-h-[calc(var(--app-viewport-height,100dvh)-10rem)] overflow-y-auto overscroll-contain md:max-h-none md:overflow-visible pointer-events-auto mx-auto w-full max-w-none rounded-2xl border border-[var(--input-panel-border)] bg-[var(--input-panel-bg)] p-3 shadow-[0_18px_54px_rgba(0,0,0,0.28)] backdrop-blur-xl">
    {quote ? <div className="group relative mb-2 inline-flex px-1">
      <HoverCard openDelay={120} closeDelay={100}>
        <HoverCardTrigger asChild>
          <button type="button" aria-label={l.viewQuote} className="inline-flex h-8 items-center gap-1.5 rounded-xl border border-[var(--input-panel-action-border)] px-2 pr-8 text-xs text-muted-foreground transition-colors hover:bg-[var(--input-panel-action-bg-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            <MessageSquareQuote className="h-3.5 w-3.5" /><span>{l.oneQuote}</span>
          </button>
        </HoverCardTrigger>
        <HoverCardContent data-ai-workbench-quote-hover-card="true" side="top" align="start" sideOffset={8} className="w-[calc(100vw-2rem)] max-w-3xl rounded-2xl border-border bg-card p-0 text-card-foreground shadow-2xl">
          <div className="flex items-start gap-3 px-4 py-3">
            <span className="pt-0.5 text-sm text-muted-foreground">1.</span>
            <div className="min-w-0 flex-1">
              <div className="text-sm text-muted-foreground">{l.selectedText}</div>
              {editingQuote ? <Textarea autoFocus value={quoteDraft} onChange={(event) => setQuoteDraft(event.target.value)} aria-label={l.editQuoteText} className="mt-1 min-h-[48px] max-h-28 resize-y rounded-lg border-0 bg-background/40 px-2 py-1.5 text-sm leading-6 shadow-none focus-visible:ring-1 focus-visible:ring-ring" /> : <p className="mt-1 max-h-28 overflow-y-auto whitespace-pre-wrap break-words text-sm leading-6 text-foreground">{quote.content}</p>}
            </div>
            <div className="flex shrink-0 items-center gap-1">
              {editingQuote ? <><Button type="button" size="icon" variant="ghost" className="h-7 w-7 rounded-md" aria-label={l.saveQuoteEdit} disabled={!quoteDraft.trim()} onClick={() => { onQuoteChange({ ...quote, content: quoteDraft }); setEditingQuote(false) }}><Check className="h-4 w-4" /></Button><Button type="button" size="icon" variant="ghost" className="h-7 w-7 rounded-md text-muted-foreground" aria-label={l.cancelQuoteEdit} onClick={() => { setQuoteDraft(quote.content); setEditingQuote(false) }}><X className="h-4 w-4" /></Button></> : <><Button type="button" size="icon" variant="ghost" className="h-7 w-7 rounded-md text-muted-foreground hover:text-foreground" aria-label={l.editQuote} onClick={() => { setQuoteDraft(quote.content); setEditingQuote(true) }}><Pencil className="h-4 w-4" /></Button><Button type="button" size="icon" variant="ghost" className="h-7 w-7 rounded-md text-muted-foreground hover:text-foreground" aria-label={l.deleteQuote} onClick={onQuoteRemove}><Trash2 className="h-4 w-4" /></Button></>}
            </div>
          </div>
        </HoverCardContent>
      </HoverCard>
      <Button type="button" size="icon" variant="ghost" className="pointer-events-none absolute right-1 top-1/2 h-6 w-6 -translate-y-1/2 rounded-full border border-[var(--input-panel-action-border)] text-muted-foreground opacity-0 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100 transition-opacity group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100" aria-label={l.removeQuote} onClick={onQuoteRemove}><X className="h-3.5 w-3.5" /></Button>
    </div> : null}
    {mcpTools.length ? <div data-slot="composer-mcp-tools" aria-label={t.mcpComposer.selected} className="mb-2 flex flex-wrap gap-2 px-1">{mcpTools.map(tool => {
      const title = mcpToolTitle(tool, t.mcpConnections.capabilityLabels)
      return <span key={tool.name} className="inline-flex max-w-full items-center gap-1.5 rounded-xl border border-[var(--input-panel-action-border)] bg-[var(--input-panel-action-bg)] py-1 pl-2.5 pr-1 text-xs text-muted-foreground"><Wrench aria-hidden="true" className="h-3.5 w-3.5 shrink-0" /><span className="min-w-0 truncate" title={tool.name}>{title}{tool.sourceName ? ` · ${tool.sourceName}` : ''}</span><Button type="button" size="icon" variant="ghost" className="h-6 w-6 shrink-0 rounded-full text-muted-foreground hover:bg-muted hover:text-foreground" aria-label={formatWorkbenchText(t.mcpComposer.remove, { name: title })} onClick={() => onMCPToolsChange(mcpTools.filter(selected => selected.name !== tool.name))}><X aria-hidden="true" className="h-3.5 w-3.5" /></Button></span>
    })}</div> : null}
    {assetReferences.length ? <div className="mb-2 px-1"><AssetReferenceChips references={assetReferences} onRemove={(index) => onAssetReferencesChange(assetReferences.filter((_, i) => i !== index))} /></div> : null}
    {attachments.length ? <div className="mb-2 flex flex-wrap gap-2 px-1 pt-1">{attachments.map((attachment) => <div key={attachment.id} className="group relative flex h-16 min-w-16 max-w-48 items-center gap-2 overflow-hidden rounded-lg border border-[var(--input-panel-action-border)] bg-[var(--input-panel-action-bg)] pr-2">{attachment.type.startsWith('image/') ? <Image src={attachment.dataUrl} alt={attachment.name} width={64} height={64} unoptimized className="h-16 w-16 object-cover" /> : <div className="flex h-16 w-12 shrink-0 items-center justify-center bg-[var(--input-panel-action-bg)]"><FileText className="h-5 w-5 text-muted-foreground" /></div>}{!attachment.type.startsWith('image/') ? <span className="truncate text-xs text-foreground">{attachment.name}</span> : null}<button type="button" aria-label={formatWorkbenchText(l.removeAttachment, { name: attachment.name })} onClick={() => onAttachmentsChange(attachments.filter((item) => item.id !== attachment.id))} className="absolute right-1 top-1 flex h-5 w-5 items-center justify-center rounded-full bg-background/90 text-foreground shadow-sm hover:bg-background"><X className="h-3 w-3" /></button></div>)}</div> : null}
    <Textarea ref={textareaRef} data-slot="ai-workbench-composer-textarea" value={value} onChange={(event) => {
      const next = event.target.value
      onChange(next)
      if (!(event.nativeEvent as InputEvent).isComposing) updateCommand(next, event.target.selectionStart)
      else dismissCommands()
    }} onCompositionEnd={(event) => updateCommand(event.currentTarget.value, event.currentTarget.selectionStart)} onClick={(event) => updateCommand(event.currentTarget.value, event.currentTarget.selectionStart)} onKeyUp={(event) => { if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) updateCommand(event.currentTarget.value, event.currentTarget.selectionStart) }} aria-haspopup="listbox" aria-controls={command && !referenceTask ? 'ai-composer-command-list' : undefined} aria-expanded={Boolean(command && !referenceTask)} onPaste={handlePaste} onKeyDown={handleKeyDown} disabled={disabled} placeholder={placeholder} aria-label={placeholder} className="h-20 min-h-20 max-h-20 md:h-24 md:min-h-24 md:max-h-24 resize-none overflow-y-auto border-0 !bg-transparent px-2 py-2 text-base md:text-[15px] leading-6 shadow-none focus-visible:ring-0" />
    <div data-slot="ai-workbench-composer-footer" className="mt-1 flex flex-wrap items-center gap-1 bg-transparent"><input ref={fileInputRef} type="file" multiple className="hidden" onChange={handleFileChange} /><Tooltip><TooltipTrigger asChild><Button type="button" size="icon" variant="ghost" onClick={() => fileInputRef.current?.click()} disabled={disabled || attachments.length >= 6} className="h-11 w-11 md:h-9 md:w-9 rounded-lg hover:bg-[var(--input-panel-action-bg-hover)]" aria-label={l.addAttachment}><Plus className="h-4 w-4" /></Button></TooltipTrigger><TooltipContent>{l.attachmentHint}</TooltipContent></Tooltip>{goalMode !== 'chat' ? <><span aria-hidden="true" className="mx-1 h-4 w-px shrink-0 bg-border" /><Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" disabled={disabled} data-slot="goal-mode-chip" aria-label={language === 'zh' ? '移除目标模式' : 'Remove goal mode'} onClick={() => { onGoalModeChange('chat'); focusComposer() }} className="group h-8 gap-2 rounded-full border-0 bg-transparent px-2.5 text-sm font-normal text-muted-foreground shadow-none hover:bg-muted/60 hover:text-muted-foreground focus-visible:bg-muted/60"><span aria-hidden="true" className="relative size-[18px] shrink-0"><Goal data-slot="goal-mode-icon" className="absolute inset-0 size-[18px] transition-opacity group-hover:opacity-0 group-focus-visible:opacity-0 [@media(hover:none)]:opacity-0" /><XCircle data-slot="goal-mode-remove" className="absolute inset-0 size-[18px] fill-current opacity-0 transition-opacity [&_path]:stroke-[var(--input-panel-bg)] group-hover:opacity-100 group-focus-visible:opacity-100 [@media(hover:none)]:opacity-100" /></span><span>{language === 'zh' ? '目标' : 'Goal'}</span></Button></TooltipTrigger><TooltipContent side="top" sideOffset={8} className="rounded-xl px-3 py-2 text-sm font-semibold [&_[data-slot=tooltip-arrow]]:hidden">{language === 'zh' ? '明确目标' : 'Define a goal'}</TooltipContent></Tooltip></> : null}<div className="min-w-0 flex-1" />
      <Select value={selectedModel} onValueChange={onModelChange} disabled={disabled || working}><SelectTrigger aria-label={l.selectModel} className="flex h-11 w-11 justify-center rounded-lg border-0 !bg-transparent px-0 text-xs shadow-none hover:!bg-[var(--input-panel-action-bg-hover)] focus:ring-0 md:h-9 md:w-auto md:min-w-[180px] md:max-w-[260px] md:justify-start md:px-2"><BrainCircuit className="h-4 w-4 shrink-0 text-primary md:mr-2" /><SelectValue className="hidden md:block" placeholder={l.selectModel} /></SelectTrigger><SelectContent align="end">{models.map((model) => { const value = modelValue(model); return <SelectItem key={value} value={value} disabled={!model.configured || !model.enabled}>{model.providerName} · {model.model}</SelectItem> })}</SelectContent></Select>
      {working ? <Button type="button" size="icon" variant="outline" onClick={onStop} disabled={stopping} aria-label={l.stopExecution} className="ml-1 h-11 w-11 md:h-9 md:w-9 shrink-0 rounded-lg">{stopping ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Square className="h-3.5 w-3.5 fill-current" />}</Button> : null}
      <Button type="button" size="icon" onClick={onSubmit} disabled={creating || disabled || stopping || (!working && sending) || !canSubmit} aria-label={working ? l.queueSend : l.send} className="ml-1 h-11 w-11 md:h-9 md:w-9 shrink-0 rounded-lg shadow-sm">{stopping ? <Loader2 className="h-4 w-4 animate-spin" /> : sending && !working ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}</Button>
    </div>
  </div></div>
}

function SessionSideChat({ messages, contextText, draft, model, modelOptions, liveText, sending, loadingHistory, retryable, onClose, onContextRemove, onDraftChange, onModelChange, onSubmit, onStop, onRetry }: {
  messages: SessionSideChatMessage[]
  contextText: string
  draft: string
  model: string
  modelOptions: AIWorkbenchModelOption[]
  liveText: string
  sending: boolean
  loadingHistory: boolean
  retryable: boolean
  onClose: () => void
  onContextRemove: () => void
  onDraftChange: (value: string) => void
  onModelChange: (value: string) => void
  onSubmit: () => void
  onStop: () => void
  onRetry: () => void
}) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const canSend = Boolean(draft.trim()) && !sending && !loadingHistory
  return <aside className="flex h-full min-h-0 min-w-0 w-full flex-col border-l border-border bg-background" aria-label={l.sideChatPanel}>
    <header className="flex h-12 shrink-0 items-center gap-2 border-b border-border/80 px-4">
      <MessageSquarePlus className="h-4 w-4 text-primary" />
      <span className="text-sm font-semibold">{l.sideChat}</span>
      <Badge variant="outline" className="h-5 max-w-[45%] truncate px-1.5 text-[10px] font-normal text-muted-foreground">{l.linkedSession}</Badge>
      <div className="min-w-0 flex-1" />
      <Button type="button" variant="ghost" size="icon" className="h-11 w-11 md:h-8 md:w-8 rounded-md text-muted-foreground" aria-label={l.closeSideChat} onClick={onClose}><X className="h-4 w-4" /></Button>
    </header>
    <ScrollArea className="min-h-0 flex-1">
      <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col gap-6 px-5 py-6">
        {!messages.length && !liveText ? <div className="flex flex-1 flex-col items-center justify-center py-20 text-center">{loadingHistory ? <Loader2 className="h-8 w-8 animate-spin text-muted-foreground/70" /> : <MessageSquarePlus className="h-8 w-8 text-muted-foreground/70" />}<h2 className="mt-4 text-base font-semibold">{loadingHistory ? l.loadingSideHistory : l.sideQuestion}</h2><p className="mt-2 max-w-xs text-sm leading-6 text-muted-foreground">{l.sideDescription}</p></div> : null}
        {messages.map((message) => message.role === 'user'
          ? <UserMessage key={message.id} content={message.content} />
          : <AssistantMessage key={message.id} content={message.content} />)}
        {sending ? <div className="flex min-w-0 gap-2 text-sm text-muted-foreground">{liveText ? <div className="min-w-0 flex-1"><MarkdownMessage content={liveText} /></div> : <><Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin" />{l.thinking}</>}</div> : null}
        {retryable && !sending ? <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border/70 bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
          <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
          <span className="min-w-0 flex-1">{l.sideChatIncomplete}</span>
          <Button type="button" size="sm" variant="outline" className="h-7 rounded-md px-2" onClick={onRetry}><RotateCw className="mr-1 h-3.5 w-3.5" />{l.retry}</Button>
        </div> : null}
      </div>
    </ScrollArea>
    <div className="shrink-0 p-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] md:p-4">
      <div className="rounded-2xl border border-[var(--input-panel-border)] bg-[var(--input-panel-bg)] p-3 shadow-lg backdrop-blur-xl">
        {contextText ? <div className="mb-2 flex items-center gap-2 rounded-lg border border-[var(--input-panel-action-border)] bg-[var(--input-panel-action-bg)] px-2.5 py-1.5 text-xs text-muted-foreground">
          <MessageSquareQuote className="h-3.5 w-3.5 shrink-0" /><span className="min-w-0 flex-1 truncate">{l.selectedExcerpt}{' '}{compactText(contextText, 72)}</span>
          <Button type="button" variant="ghost" size="icon" className="-mr-1 h-6 w-6 shrink-0 rounded-md" aria-label={l.removeExcerpt} onClick={onContextRemove}><X className="h-3.5 w-3.5" /></Button>
        </div> : null}
        <Textarea autoFocus value={draft} disabled={loadingHistory} onChange={(event) => onDraftChange(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter' && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); if (canSend) onSubmit() } }} placeholder={l.sidePlaceholder} aria-label={l.sideInput} className="h-24 min-h-24 max-h-24 resize-none overflow-y-auto border-0 !bg-transparent px-1 py-2 text-base md:text-sm leading-6 shadow-none focus-visible:ring-0" />
        <div className="mt-1 flex min-w-0 items-center gap-1">
          <div className="min-w-0 flex-1" />
          <Select value={model} onValueChange={onModelChange} disabled={sending || loadingHistory}>
            <SelectTrigger aria-label={l.selectModel} className="flex h-9 w-9 items-center justify-center rounded-lg border-0 !bg-transparent px-0 text-xs shadow-none hover:!bg-[var(--input-panel-action-bg-hover)] focus:ring-0 md:w-auto md:max-w-[210px] md:justify-start md:gap-1.5 md:px-2"><BrainCircuit className="h-4 w-4 shrink-0 text-primary" /><SelectValue className="hidden md:block" placeholder={l.selectModel} /></SelectTrigger>
            <SelectContent align="end">{modelOptions.map((option) => { const value = modelValue(option); return <SelectItem key={value} value={value} disabled={!option.configured || !option.enabled}>{option.providerName} · {option.model}</SelectItem> })}</SelectContent>
          </Select>
          {sending ? <Button type="button" size="icon" variant="outline" className="h-9 w-9 shrink-0 rounded-lg" aria-label={l.stopSideChat} onClick={onStop}><Square className="h-3.5 w-3.5 fill-current" /></Button> : <Button type="button" size="icon" className="h-9 w-9 shrink-0 rounded-lg" aria-label={l.sendSideQuestion} disabled={!canSend} onClick={onSubmit}><Send className="h-4 w-4" /></Button>}
        </div>
      </div>
    </div>
  </aside>
}

export default function AIWorkbenchPage() {
  return <Suspense><AIWorkbenchContent /></Suspense>
}

function AIWorkbenchContent() {
  const { t, language } = useI18n()
  const l = t.aiWorkbench
  const workbenchMessagesRef = useRef(l)
  useEffect(() => { workbenchMessagesRef.current = l }, [l])
  const showWorkbenchError = useCallback((error: unknown, key: keyof WorkbenchMessages) => {
    const messages = workbenchMessagesRef.current
    toast.error(workbenchErrorMessage(error, messages, messages[key]))
  }, [])
  const router = useRouter()
  const searchParams = useSearchParams()
  const requestedSessionId = searchParams.get('session')
  const requestedFindingId = searchParams.get('findingId')
  const requestedRetest = searchParams.get('retest') === '1'
  const requestedToolName = searchParams.get('tool') || 'retest_vulnerability'
  const [bootstrap, setBootstrap] = useState<AIWorkbenchPenTestSessionBootstrapResponse | null>(() => aiWorkbenchRouteCache?.bootstrap || null)
  const [projects, setProjects] = useState<AIWorkbenchProject[]>(() => aiWorkbenchRouteCache?.projects || [])
  const [sessions, setSessions] = useState<AIWorkbenchConversationSummary[]>(() => aiWorkbenchRouteCache?.sessions || [])
  const [activeSessionId, setActiveSessionId] = useState<string | null>(() => requestedSessionId && aiWorkbenchRouteCache?.session?.task.id === requestedSessionId ? requestedSessionId : null)
  const [projectSelection, setSelectedProjectId] = useState(() => aiWorkbenchRouteCache?.selectedProjectId || aiWorkbenchRouteCache?.session?.project?.id || aiWorkbenchRouteCache?.session?.task.projectId || '')
  const selectedProjectId = projects.some(project => project.id === projectSelection) ? projectSelection : projects[0]?.id || ''
  const [openProjects, setOpenProjects] = useState<Record<string, boolean>>(() => aiWorkbenchRouteCache?.openProjects || {})
  const [session, setSession] = useState<AIWorkbenchPenTestSessionSnapshot | null>(() => requestedSessionId && aiWorkbenchRouteCache?.session?.task.id === requestedSessionId ? aiWorkbenchRouteCache.session : null)
  const [draft, setDraft] = useState('')
  const [messageQuote, setMessageQuote] = useState<MessageQuote | null>(null)
  const [queuedMessages, setQueuedMessages] = useState<QueuedWorkbenchMessage[]>(() => aiWorkbenchRouteCache?.queuedMessages || [])
  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => Boolean(aiWorkbenchRouteCache?.sidebarCollapsed))
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)
  const [mobileSearchOpen, setMobileSearchOpen] = useState(false)
  const [editingMessageId, setEditingMessageId] = useState<string | null>(null)
  const [editedMessageContent, setEditedMessageContent] = useState('')
  const [attachments, setAttachments] = useState<ComposerAttachment[]>([])
  const [assetReferences, setAssetReferences] = useState<AssetReferenceSelection[]>([])
  const [mcpTools, setMCPTools] = useState<CallableMCPTool[]>([])
  const retestPrefillRef = useRef<string | null>(null)
  const [goalOverride, setGoalOverride] = useState<{ sessionId: string | null; action: GoalModeAction } | null>(null)
  const goalMode: GoalModeAction = goalOverride?.sessionId === activeSessionId && !(goalOverride.action === 'continue' && session?.goalRun?.status === 'completed') ? goalOverride.action : session?.goalRun?.mode === 'goal' && session.goalRun.status !== 'completed' ? 'continue' : 'chat'
  const changeGoalMode = (action: GoalModeAction) => setGoalOverride({ sessionId: activeSessionId, action })

  const [sideChatOpen, setSideChatOpen] = useState(false)
  const [sideChatMessages, setSideChatMessages] = useState<SessionSideChatMessage[]>([])
  // Session that owns the side-chat state below. Kept separate from
  // activeSessionId so a main-session refresh can never reset the panel.
  const [sideChatStateSessionId, setSideChatStateSessionId] = useState<string | null>(null)
  const [sideChatHistoryState, setSideChatHistoryState] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle')
  const [sideChatFailedRequest, setSideChatFailedRequest] = useState<{ messages: AIWorkbenchSideChatMessage[]; selectedText: string } | null>(null)
  const [sideChatContextText, setSideChatContextText] = useState('')
  const [sideChatDraft, setSideChatDraft] = useState('')
  const [sideChatSelectedModel, setSideChatSelectedModel] = useState('')
  const [sideChatSending, setSideChatSending] = useState(false)
  const [sideChatLiveText, setSideChatLiveText] = useState('')
  const [selectedModel, setSelectedModel] = useState(() => aiWorkbenchRouteCache?.selectedModel || '')
  const modelOptions = bootstrap?.modelOptions.options || []
  const selectedModelIsAvailable = modelOptions.some((option) => modelValue(option) === selectedModel)
  const effectiveSelectedModel = (selectedModelIsAvailable ? selectedModel : '') || defaultWorkbenchModelValue(modelOptions) || modelValue(bootstrap?.defaultPreview.selectedModel)
  const [search, setSearch] = useState(() => aiWorkbenchRouteCache?.search || '')
  const [projectSessionRevision, setProjectSessionRevision] = useState(0)
  const { pages: projectSessionPages, load: loadProjectSessions, updateSession: updateProjectSession, removeSession: removeProjectSession } = useProjectSessions(search, openProjects, projectSessionRevision)
  const [sessionTotal, setSessionTotal] = useState(0)
  const [sessionPage, setSessionPage] = useState(1)
  const [loadingMoreSessions, setLoadingMoreSessions] = useState(false)
  const [projectTotal, setProjectTotal] = useState(0)
  const [projectPage, setProjectPage] = useState(1)
  const [loadingMoreProjects, setLoadingMoreProjects] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const [loading, setLoading] = useState(() => !hasAIWorkbenchRouteCache(aiWorkbenchRouteCache))
  // A deep link starts fetching in an effect, one frame after the first paint.
  // Start in the loading state so the conversation never flashes its empty hero.
  const [sessionLoading, setSessionLoading] = useState(() => Boolean(requestedSessionId) && aiWorkbenchRouteCache?.session?.task.id !== requestedSessionId)
  const [sessionLoadError, setSessionLoadError] = useState('')
  const [creating, setCreating] = useState(false)
  const [sendingSessions, setSendingSessions] = useState<Set<string>>(() => new Set())
  const [stoppingSessions, setStoppingSessions] = useState<Set<string>>(() => new Set())
  const [liveTextBySession, setLiveTextBySession] = useState<Record<string, string>>({})
  const sending = activeSessionId ? sendingSessions.has(activeSessionId) : false
  const stopping = activeSessionId ? stoppingSessions.has(activeSessionId) : false
  const liveAssistantText = activeSessionId ? liveTextBySession[activeSessionId] || '' : ''
  const [projectDialog, setProjectDialog] = useState<{ mode: 'create' | 'rename'; project?: AIWorkbenchProject } | null>(null)
  const [projectName, setProjectName] = useState('')
  const [projectSaving, setProjectSaving] = useState(false)
  const [sessionRenameTarget, setSessionRenameTarget] = useState<AIWorkbenchConversationSummary | null>(null)
  const [historyTarget, setHistoryTarget] = useState<AIWorkbenchConversationSummary | null>(null)
  const [historyItems, setHistoryItems] = useState<AIWorkbenchSupersededArtifact[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [sessionName, setSessionName] = useState('')
  const [movingSessionId, setMovingSessionId] = useState<string | null>(null)
  const movingSessionRef = useRef<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<{ type: 'project'; project: AIWorkbenchProject } | { type: 'session'; summary: AIWorkbenchConversationSummary } | null>(null)
  const composerRef = useRef<HTMLDivElement | null>(null)
  const [composerHeight, setComposerHeight] = useState(224)
  const activeSessionRef = useRef<string | null>(activeSessionId)
  const sessionAbortRef = useRef<AbortController | null>(null)
  const messageControllersRef = useRef<Map<string, AbortController>>(new Map())
  const composerBySessionRef = useRef<Map<string, ComposerSnapshot>>(new Map())
  const composerSessionKeyRef = useRef<string | null>(null)
  const composerSnapshotRef = useRef<ComposerSnapshot>({ draft: '', quote: null, attachments: [], assetReferences: [], mcpTools: [] })
  const createRequestIdRef = useRef('')
  const sideChatAbortRef = useRef<AbortController | null>(null)
  const sideChatTriggerRef = useRef<HTMLButtonElement | null>(null)
  const sideChatHistoryAbortRef = useRef<AbortController | null>(null)
  // Side chat owns its own lifecycle: these refs make sure only an explicit
  // stop, a finished request or a real conversation switch ends the request.
  const sideChatStateSessionRef = useRef<string | null>(null)
  const sideChatAbortReasonRef = useRef<'stop' | 'session' | 'unmount' | null>(null)
  const sideChatRequestIdRef = useRef(0)
  const sideChatHistorySeqRef = useRef(0)
  const queuedMessageDrainRef = useRef(false)
  const pausedQueueSessionsRef = useRef<Set<string>>(new Set())
  const searchRef = useRef(search)
  const bootstrapRequestSeqRef = useRef(0)
  const workspaceRequestSeqRef = useRef(0)
  const sessionRef = useRef<AIWorkbenchPenTestSessionSnapshot | null>(session)
  const batchRefreshInFlightRef = useRef(false)
  const batchQueueWasActiveRef = useRef(false)
  const planRunWasActiveRef = useRef(false)
  const processMerger = useRef(new AIProcessMerger())
  const processFrame = useRef<(() => void) | null>(null)
  const receiveProcess = useCallback((update: AIProcessUpdate) => {
    if (activeSessionRef.current !== update.sessionId) return
    processMerger.current.reset(update.sessionId)
    if (!processMerger.current.accept(update) || processFrame.current !== null) return
    processFrame.current = scheduleProcessFrame(() => {
      processFrame.current = null
      setSession(previous => previous ? processMerger.current.merge(previous) : previous)
    })
  }, [])
  const receiveSnapshot = useCallback((snapshot: AIWorkbenchPenTestSessionSnapshot) => {
    if (activeSessionRef.current !== snapshot.task.id) return
    processMerger.current.reset(snapshot.task.id)
    setSession(processMerger.current.merge(snapshot))
  }, [])
  useEffect(() => () => { if (processFrame.current !== null) processFrame.current() }, [])
  useEffect(() => {
    activeSessionRef.current = activeSessionId
  }, [activeSessionId])
  useEffect(() => {
    sessionRef.current = session
  }, [session])
  useEffect(() => {
    searchRef.current = search
  }, [search])
  useEffect(() => {
    if (!activeSessionId) return
    const projectId = session?.project?.id || session?.task.projectId
    if (!projectId) return
    setOpenProjects(previous => previous[projectId] ? previous : { ...previous, [projectId]: true })
  }, [activeSessionId, session?.project?.id, session?.task.projectId])
  const markSessionSending = useCallback((sessionId: string | null, value: boolean) => {
    if (!sessionId) return
    setSendingSessions(previous => {
      if (previous.has(sessionId) === value) return previous
      const next = new Set(previous)
      if (value) next.add(sessionId)
      else next.delete(sessionId)
      return next
    })
  }, [])
  const markSessionStopping = useCallback((sessionId: string | null, value: boolean) => {
    if (!sessionId) return
    setStoppingSessions(previous => {
      if (previous.has(sessionId) === value) return previous
      const next = new Set(previous)
      if (value) next.add(sessionId)
      else next.delete(sessionId)
      return next
    })
  }, [])
  const updateLiveText = useCallback((sessionId: string | null, text: string) => {
    if (!sessionId) return
    setLiveTextBySession(previous => {
      if (text) {
        if (previous[sessionId] === text) return previous
        return { ...previous, [sessionId]: text }
      }
      if (!(sessionId in previous)) return previous
      const next = { ...previous }
      delete next[sessionId]
      return next
    })
  }, [])
  const clearLiveText = useCallback((sessionId: string | null) => updateLiveText(sessionId, ''), [updateLiveText])
  const appendLiveText = useCallback((sessionId: string | null, delta: string) => {
    if (!sessionId || !delta) return
    setLiveTextBySession(previous => ({ ...previous, [sessionId]: `${previous[sessionId] || ''}${delta}` }))
  }, [])
  const abortMessageStream = useCallback((sessionId: string | null) => {
    if (!sessionId) return
    const controller = messageControllersRef.current.get(sessionId)
    if (!controller) return
    messageControllersRef.current.delete(sessionId)
    controller.abort()
  }, [])
  useEffect(() => {
    composerSnapshotRef.current = { draft, quote: messageQuote, attachments, assetReferences, mcpTools }
  }, [assetReferences, attachments, draft, mcpTools, messageQuote])
  useEffect(() => {
    const nextKey = activeSessionId || 'new'
    const previousKey = composerSessionKeyRef.current
    if (previousKey === nextKey) return
    if (previousKey) composerBySessionRef.current.set(previousKey, composerSnapshotRef.current)
    const saved = composerBySessionRef.current.get(nextKey) || { draft: '', quote: null, attachments: [], assetReferences: [], mcpTools: [] }
    setDraft(saved.draft)
    setMessageQuote(saved.quote)
    setAttachments(saved.attachments)
    setAssetReferences(saved.assetReferences)
    setMCPTools(saved.mcpTools)
    composerSnapshotRef.current = saved
    composerSessionKeyRef.current = nextKey
    setEditingMessageId(null)
    setEditedMessageContent('')
    clearLiveText(nextKey)
  }, [activeSessionId, clearLiveText])
  useEffect(() => {
    aiWorkbenchRouteCache = {
      bootstrap,
      projects,
      sessions,
      activeSessionId,
      selectedProjectId,
      openProjects,
      session,
      selectedModel,
      queuedMessages,
      search,
      sidebarCollapsed,
      savedAt: Date.now(),
    }
  }, [activeSessionId, bootstrap, openProjects, projects, queuedMessages, search, selectedModel, selectedProjectId, session, sessions, sidebarCollapsed])
  const queuePersistenceReadyRef = useRef(false)
  const restoreQueueFromStoreRef = useRef(aiWorkbenchRouteCache === null)
  useEffect(() => {
    // The in-memory route cache is fresher than IndexedDB while the workbench
    // stays mounted; only a full reload needs to restore the persisted queue.
    if (!restoreQueueFromStoreRef.current) {
      queuePersistenceReadyRef.current = true
      return
    }
    let disposed = false
    void loadQueuedMessages<QueuedWorkbenchMessage>(workbenchQueueOwnerKey()).then((stored) => {
      if (disposed) return
      const restored = stored.filter((message) => Boolean(message)
        && typeof message.id === 'string'
        && typeof message.sessionId === 'string'
        && typeof message.content === 'string'
        && Array.isArray(message.attachments))
      if (restored.length) {
        setQueuedMessages((previous) => {
          if (!previous.length) return restored
          const known = new Set(previous.map((message) => message.id))
          return [...restored.filter((message) => !known.has(message.id)), ...previous]
        })
      }
      queuePersistenceReadyRef.current = true
    })
    return () => { disposed = true }
  }, [])
  useEffect(() => {
    if (!queuePersistenceReadyRef.current) return
    void saveQueuedMessages(workbenchQueueOwnerKey(), queuedMessages)
  }, [queuedMessages])
  const refreshWorkspace = useCallback(async (options: { query?: string; page?: number; append?: boolean } = {}) => {
    const requestSeq = ++workspaceRequestSeqRef.current
    const query = options.query ?? searchRef.current
    const page = options.page ?? 1
    const [sessionData, projectData] = await Promise.all([
      listAIWorkbenchPenTestSessions({ page, pageSize: 80, search: query.trim() || undefined }),
      listAIWorkbenchPenTestProjects({ pageSize: 100 }),
    ])
    if (requestSeq !== workspaceRequestSeqRef.current) return
    setSessions(previous => {
      if (!options.append) return sessionData.conversations
      const known = new Set(previous.map(item => item.task.id))
      return [...previous, ...sessionData.conversations.filter(item => !known.has(item.task.id))]
    })
    setSessionTotal(sessionData.total)
    setSessionPage(sessionData.page || page)
    if (!options.append) {
      setProjects(projectData.projects)
      setProjectTotal(projectData.total)
      setProjectPage(projectData.page || 1)
      setProjectSessionRevision(previous => previous + 1)
    }
  }, [])
  const loadMoreSessions = useCallback(async () => {
    if (loadingMoreSessions || sessions.length >= sessionTotal) return
    setLoadingMoreSessions(true)
    try {
      await refreshWorkspace({ page: sessionPage + 1, append: true })
    } catch (error) {
      showWorkbenchError(error, 'loadWorkbenchFailed')
    } finally {
      setLoadingMoreSessions(false)
    }
  }, [loadingMoreSessions, refreshWorkspace, sessionPage, sessionTotal, sessions.length, showWorkbenchError])
  const loadMoreProjects = useCallback(async () => {
    if (loadingMoreProjects || projects.length >= projectTotal) return
    setLoadingMoreProjects(true)
    try {
      const data = await listAIWorkbenchPenTestProjects({ page: projectPage + 1, pageSize: 100 })
      setProjects(previous => {
        const known = new Set(previous.map(item => item.id))
        return [...previous, ...data.projects.filter(item => !known.has(item.id))]
      })
      setProjectTotal(data.total)
      setProjectPage(data.page || projectPage + 1)
    } catch (error) {
      showWorkbenchError(error, 'loadWorkbenchFailed')
    } finally {
      setLoadingMoreProjects(false)
    }
  }, [loadingMoreProjects, projectPage, projectTotal, projects.length, showWorkbenchError])
  useEffect(() => {
    const timer = window.setTimeout(() => { void refreshWorkspace({ query: search }).catch(error => showWorkbenchError(error, 'loadWorkbenchFailed')) }, 250)
    return () => window.clearTimeout(timer)
  }, [refreshWorkspace, search, showWorkbenchError])
  const refreshWorkspaceForActiveQueues = useCallback(async () => {
    if (typeof document !== 'undefined' && document.hidden) return
    if (batchRefreshInFlightRef.current) return
    batchRefreshInFlightRef.current = true
    try {
      const [queues, planRuns] = await Promise.all([listAIBatchQueues(), listAIPlanRunSummaries()])
      const hasActiveQueue = queues.some((queue) => {
        const queueActive = activeBatchQueueStatuses.has(displayText(queue.status).toLowerCase())
        const itemActive = queue.items.some((item) => activeBatchQueueStatuses.has(displayText(item.status).toLowerCase()))
        return queueActive || itemActive
      })
      // A plan run changes the sidebar outside the active session's SSE stream:
      // new Workers appear and their items move pending/running/completed while
      // the Planner session is idle. Poll until the run is terminal.
      const hasActivePlanRun = planRuns.some(({ run, itemCounts }) => {
        const runStatus = displayText(run.status).toLowerCase()
        if (runStatus === 'planning' || runStatus === 'waiting') return true
        return Boolean(itemCounts.pending || itemCounts.running)
      })
      if (hasActiveQueue || batchQueueWasActiveRef.current || hasActivePlanRun || planRunWasActiveRef.current) {
        await refreshWorkspace()
      }
      batchQueueWasActiveRef.current = hasActiveQueue
      planRunWasActiveRef.current = hasActivePlanRun
    } catch {
      // Queue polling is only used to keep the sidebar fresh; keep the chat view quiet on transient failures.
    } finally {
      batchRefreshInFlightRef.current = false
    }
  }, [refreshWorkspace])
  const syncSessionSnapshotToSidebar = useCallback((nextSession: AIWorkbenchPenTestSessionSnapshot) => {
    const summary = summaryFromSnapshot(nextSession, l.unnamedProject)
    updateProjectSession(summary)
    setSessions((previous) => mergeSessionSummaryList(previous, summary))
    setProjects((previous) => mergeProjectSessionSnapshot(previous, nextSession, summary))
  }, [updateProjectSession])
  const groupedProjects = useMemo(() => {
    const query = search.trim().toLowerCase()
    const matchesSession = (item: AIWorkbenchConversationSummary) => [item.task.name, item.task.target, item.task.planItemTitle, item.latestMessage?.content, item.launchPlan?.initialMessage].filter(Boolean).join(' ').toLowerCase().includes(query)
    const seen = new Set<string>()
    const groups: AIWorkbenchProject[] = []
    const groupIndex = new Map<string, AIWorkbenchProject>()
    const visibleProjectIds = new Set<string>()
    for (const project of projects) {
      const page = projectSessionPages[project.id]
      const projectPage = page?.query === search.trim() ? page : undefined
      const conversations = projectPage ? projectPage.conversations : project.conversations || []
      conversations.forEach((item) => seen.add(item.task.id))
      const group = { ...project, conversations }
      if (project.id) groupIndex.set(project.id, group)
      if (!query) {
        groups.push(group)
        if (group.id) visibleProjectIds.add(group.id)
        continue
      }
      const projectMatched = [project.name, project.description, ...(project.scope || [])].filter(Boolean).join(' ').toLowerCase().includes(query)
      // Project-scoped search also matches archived message text on the server.
      const filtered = projectPage || projectMatched ? conversations : conversations.filter(matchesSession)
      if (projectMatched || filtered.length) {
        groups.push({ ...group, conversations: filtered })
        if (group.id) visibleProjectIds.add(group.id)
      }
    }
    const orphanSessions: AIWorkbenchConversationSummary[] = []
    for (const item of sessions) {
      if (seen.has(item.task.id)) continue
      const summaryProject = projectFromSummary(item, l.unnamedProject)
      if (summaryProject?.id) {
        const existing = groupIndex.get(summaryProject.id)
        const page = projectSessionPages[summaryProject.id]
        if (existing && page?.query === search.trim() && page.page > 0) continue
        if (existing && visibleProjectIds.has(summaryProject.id)) {
          existing.conversations = [...(existing.conversations || []), item]
          seen.add(item.task.id)
          continue
        }
        const projectMatched = query && [summaryProject.name, summaryProject.description, ...(summaryProject.scope || [])].filter(Boolean).join(' ').toLowerCase().includes(query)
        if (!query || projectMatched || matchesSession(item)) {
          const nextProject = { ...summaryProject, conversations: page?.query === search.trim() && page.page > 0 ? page.conversations : [item] }
          groups.push(nextProject)
          groupIndex.set(nextProject.id, nextProject)
          visibleProjectIds.add(nextProject.id)
        }
        seen.add(item.task.id)
        continue
      }
      if (!query || matchesSession(item)) orphanSessions.push(item)
    }
    if (orphanSessions.length) {
      groups.push({ id: '', name: l.noProject, description: '', scope: [], status: 'active', conversationCount: orphanSessions.length, factCount: 0, findingCount: 0, conversations: orphanSessions, createdAt: '', updatedAt: '' })
    }
    return groups
  }, [projects, projectSessionPages, search, sessions, l])
  useEffect(() => {
    if (!searchOpen && !mobileSearchOpen) return
    const timer = window.setTimeout(() => {
      document.querySelectorAll<HTMLInputElement>('[data-ai-workbench-search-input]').forEach((input) => {
        if (input.getClientRects().length) input.focus()
      })
    }, 0)
    return () => window.clearTimeout(timer)
  }, [mobileSearchOpen, searchOpen])
  useEffect(() => {
    const node = composerRef.current
    if (!node) return
    const update = () => {
      // Detached or hidden composers must not erase the conversation's bottom reservation.
      if (!node.isConnected) return
      const height = Math.ceil(node.getBoundingClientRect().height)
      if (height > 0) setComposerHeight(height)
    }
    update()
    const observer = new ResizeObserver(update)
    observer.observe(node)
    window.addEventListener('resize', update)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', update)
    }
    // Switching between the full-width and split layouts remounts the composer.
  }, [sideChatOpen])
  const replaceSessionRoute = useCallback((sessionId: string | null) => {
    const url = new URL(window.location.href)
    if (sessionId) {
      if (url.searchParams.get('session') === sessionId) return
      url.searchParams.set('session', sessionId)
    } else {
      if (!url.searchParams.has('session')) return
      url.searchParams.delete('session')
    }
    router.push(`${url.pathname}${url.search}${url.hash}`, { scroll: false })
  }, [router])
  const loadSession = useCallback(async (sessionId: string, options: { background?: boolean } = {}) => {
    const background = options.background === true
    setActiveSessionId(sessionId); activeSessionRef.current = sessionId
    replaceSessionRoute(sessionId)
    sessionAbortRef.current?.abort()
    if (!background) {
      setSessionLoading(true)
      setSessionLoadError('')
      setSession(null)
    }
    const controller = new AbortController(); sessionAbortRef.current = controller
    try {
      const data = await getAIWorkbenchPenTestSession(sessionId, { signal: controller.signal })
      if (activeSessionRef.current === sessionId) {
        setSessionLoadError('')
        receiveSnapshot(data)
        syncSessionSnapshotToSidebar(data)
        setSelectedProjectId(data.project?.id || data.task.projectId || '')
        const sessionModel = data.model?.agentProviderId && data.model?.agentModel
          ? `${data.model.agentProviderId}::${data.model.agentModel}`
          : ''
        if (sessionModel) setSelectedModel(sessionModel)
      }
    }
    catch (error: any) {
      if (error?.name !== 'AbortError') {
        if (activeSessionRef.current === sessionId) {
          setSessionLoadError(workbenchErrorMessage(error, l, l.loadSessionFailed))
          setSession(null)
        }
        showWorkbenchError(error, 'loadSessionFailed')
      }
    }
    finally { if (sessionAbortRef.current === controller) sessionAbortRef.current = null; if (activeSessionRef.current === sessionId && !background) setSessionLoading(false) }
  }, [syncSessionSnapshotToSidebar, receiveSnapshot, showWorkbenchError, l.loadSessionFailed, replaceSessionRoute])
  const clearSessionRoute = useCallback(() => {
    const url = new URL(window.location.href)
    if (!url.searchParams.has('session')) return
    url.searchParams.delete('session')
    router.push(`${url.pathname}${url.search}${url.hash}`, { scroll: false })
  }, [router])
  useEffect(() => {
    if (!requestedSessionId) {
      if (activeSessionRef.current) {
        sessionAbortRef.current?.abort()
        activeSessionRef.current = null
        processMerger.current.reset('')
        setActiveSessionId(null)
        setSession(null)
        setSessionLoadError('')
        setSessionLoading(false)
      }
      return
    }
    // The click path already called loadSession and wrote the URL; only load
    // from the URL on a fresh mount or browser history navigation.
    if (activeSessionRef.current === requestedSessionId) return
    void loadSession(requestedSessionId, { background: false })
  }, [requestedSessionId, loadSession])
  useEffect(() => {
    if (!requestedRetest || !requestedFindingId || retestPrefillRef.current === requestedFindingId) return
    let cancelled = false
    void (async () => {
      try {
        const [finding, tools] = await Promise.all([getAIWorkbenchFinding(requestedFindingId), listCallableMCPTools()])
        if (cancelled) return
        retestPrefillRef.current = requestedFindingId
        const title = finding.view.name || requestedFindingId
        const target = finding.view.rawTarget || finding.view.url || finding.conversation?.target || ''
        const prompt = language === 'zh'
          ? `请复测漏洞 #${requestedFindingId}：${title}。${target ? `目标：${target}。` : ''}读取原始复现证据，在授权范围内使用最小化请求完成复测；完成后记录 still_vulnerable、fixed 或 inconclusive 结论，并附上真实证据。`
          : `Retest finding #${requestedFindingId}: ${title}.${target ? ` Target: ${target}.` : ''} Read the original reproduction evidence, stay within the authorized scope, use the smallest necessary request, then record a still_vulnerable, fixed, or inconclusive result with real evidence.`
        setDraft((previous) => previous.trim() ? previous : prompt)
        if (!requestedSessionId && finding.project?.id) setSelectedProjectId(finding.project.id)
        const selected = tools.find((tool) => tool.name === requestedToolName)
        if (selected) setMCPTools((previous) => previous.some((tool) => tool.name === selected.name) ? previous : [...previous, selected])
        const url = new URL(window.location.href)
        url.searchParams.delete('findingId')
        url.searchParams.delete('retest')
        url.searchParams.delete('tool')
        router.replace(`${url.pathname}${url.search}${url.hash}`, { scroll: false })
      } catch (error) {
        if (!cancelled) showWorkbenchError(error, 'loadSessionFailed')
        retestPrefillRef.current = null
      }
    })()
    return () => {
      cancelled = true
      if (retestPrefillRef.current === requestedFindingId) retestPrefillRef.current = null
    }
  }, [language, requestedFindingId, requestedRetest, requestedSessionId, requestedToolName, router, showWorkbenchError])
  const loadBootstrap = useCallback(async () => {
    const requestSeq = ++bootstrapRequestSeqRef.current
    const hasCachedState = hasAIWorkbenchRouteCache(aiWorkbenchRouteCache)
    if (!hasCachedState) setLoading(true)
    try {
      const data = await getAIWorkbenchPenTestSessionBootstrap({ recentLimit: 80 })
      if (requestSeq !== bootstrapRequestSeqRef.current) return
      setBootstrap(data); setSessions(data.recentSessions); setProjects(data.projects || [])
      const defaultModel = defaultWorkbenchModelValue(data.modelOptions.options) || modelValue(data.defaultPreview.selectedModel)
      setSelectedModel((previous) => previous && data.modelOptions.options.some((option) => modelValue(option) === previous) ? previous : defaultModel)
    } catch (error: any) { showWorkbenchError(error, 'loadWorkbenchFailed') } finally { if (requestSeq === bootstrapRequestSeqRef.current) setLoading(false) }
  }, [showWorkbenchError])
  const handleModelChange = useCallback((value: string) => {
    setSelectedModel(value)
    const { providerId } = splitModelValue(value)
    if (!providerId) return
    void (async () => {
      try {
        const aiConfig = await getAIConfig()
        if ((aiConfig.routing?.workbench || '') === providerId) return
        await saveAIConfig({
          ...aiConfig,
          routing: {
            ...aiConfig.routing,
            workbench: providerId,
          },
        })
        setBootstrap((previous) => previous ? {
          ...previous,
          modelOptions: {
            ...previous.modelOptions,
            defaultProviderId: providerId,
            options: previous.modelOptions.options.map((option) => ({
              ...option,
              defaultForWorkbench: option.providerId === providerId,
            })),
          },
        } : previous)
        toast.success(l.defaultModelChanged)
      } catch (error: any) {
        showWorkbenchError(error, 'saveDefaultModelFailed')
      }
    })()
  }, [l, showWorkbenchError])
  const openSideChat = useCallback((selectedText = '') => {
    if (session?.task.planRunId && !isOrchestrationRunId(session.task.planRunId)) return
    if (!activeSessionId) {
      toast.error(l.sideSessionRequired)
      return
    }
    setSideChatContextText(selectedText)
    setSideChatSelectedModel((previous) => previous || effectiveSelectedModel)
    setSideChatOpen(true)
  }, [activeSessionId, effectiveSelectedModel, l, session?.task.planRunId])
  const closeSideChat = useCallback(() => {
    setSideChatOpen(false)
    requestAnimationFrame(() => sideChatTriggerRef.current?.focus())
  }, [])
  const stopSideChat = useCallback(() => {
    const controller = sideChatAbortRef.current
    if (!controller) return
    sideChatAbortReasonRef.current = 'stop'
    controller.abort()
    // End the thinking state right away; the request's own finally only guards
    // against a late callback from an older request.
    setSideChatSending(false)
    setSideChatLiveText('')
  }, [])
  const loadSideChatHistory = useCallback((sessionId: string) => {
    if (!sessionId) return
    sideChatHistoryAbortRef.current?.abort()
    const controller = new AbortController()
    sideChatHistoryAbortRef.current = controller
    const seq = ++sideChatHistorySeqRef.current
    const requestId = sideChatRequestIdRef.current
    setSideChatHistoryState('loading')
    void getAIWorkbenchSideChatHistory(sessionId, { signal: controller.signal }).then((messages) => {
      if (seq !== sideChatHistorySeqRef.current || controller.signal.aborted) return
      if (requestId !== sideChatRequestIdRef.current || sideChatStateSessionRef.current !== sessionId) return
      setSideChatMessages(messages.map((message) => ({ ...message, id: attachmentId() })))
      setSideChatHistoryState('ready')
    }).catch((error: any) => {
      if (error?.name === 'AbortError' || seq !== sideChatHistorySeqRef.current) return
      // A failed history read must not leave the panel stuck in "loading".
      setSideChatHistoryState('error')
      showWorkbenchError(error, 'loadSideHistoryFailed')
    }).finally(() => {
      if (sideChatHistoryAbortRef.current === controller) sideChatHistoryAbortRef.current = null
    })
  }, [showWorkbenchError])
  const sendSideChat = useCallback(async (options: { retry?: boolean } = {}) => {
    const retry = options.retry === true
    const sessionId = activeSessionId
    const failed = sideChatFailedRequest
    const failedMessages = retry ? failed?.messages || [] : []
    const content = (retry ? failedMessages[failedMessages.length - 1]?.content || '' : sideChatDraft).trim()
    // Gate on the conversation that owns this panel. Main-session sending,
    // refreshing or streaming state must never block or clear a side request.
    if (!sessionId || sideChatStateSessionRef.current !== sessionId || sideChatHistoryState === 'loading' || !content || sideChatSending || sideChatAbortRef.current) return
    const selectedText = retry && failed ? failed.selectedText : sideChatContextText
    const requestMessages: AIWorkbenchSideChatMessage[] = retry && failed && failed.messages.length
      ? failed.messages
      : [...sideChatMessages.slice(-39), { role: 'user', content }]
    const displayMessages = requestMessages.map((message) => ({ ...message, id: attachmentId() }))
    const controller = new AbortController()
    const requestId = ++sideChatRequestIdRef.current
    sideChatAbortRef.current = controller
    sideChatAbortReasonRef.current = null
    setSideChatFailedRequest(null)
    setSideChatMessages(displayMessages)
    if (!retry) setSideChatDraft('')
    setSideChatSending(true)
    setSideChatLiveText('')
    const { providerId, model } = splitModelValue(sideChatSelectedModel || effectiveSelectedModel)
    // A late callback from an older request (or another conversation) is ignored
    // even when its abort lands a microtask after the panel was reset.
    const isCurrent = () => sideChatAbortRef.current === controller && sideChatStateSessionRef.current === sessionId && sideChatRequestIdRef.current === requestId
    let answer = ''
    try {
      const doneContent = await streamAIWorkbenchSideChat(requestMessages, {
        sessionId,
        selectedText,
        agentProviderId: providerId,
        agentModel: model,
        signal: controller.signal,
        onDelta: (_delta, accumulated) => { if (!isCurrent()) return; answer = accumulated; setSideChatLiveText(accumulated) },
      })
      if (!isCurrent()) return
      // Some providers only send the final frame. Use it so a completed request
      // is never mistaken for an empty answer.
      const finalAnswer = (answer || doneContent).trim()
      if (!finalAnswer) {
        setSideChatFailedRequest({ messages: requestMessages, selectedText })
        showWorkbenchError(new WorkbenchRequestError('sideChatEmpty'), 'sendSideChatFailed')
        return
      }
      const completedMessages: AIWorkbenchSideChatMessage[] = [...requestMessages, { role: 'assistant', content: finalAnswer }]
      setSideChatMessages(completedMessages.map((message) => ({ ...message, id: attachmentId() })))
      try {
        await saveAIWorkbenchSideChatHistory(sessionId, completedMessages)
      } catch (error: any) {
        showWorkbenchError(error, 'saveSideHistoryFailed')
      }
    } catch (error: any) {
      if (!isCurrent()) return
      // A stop keeps whatever was already streamed. Other failures keep the
      // question in the transcript and offer retry instead of restoring the
      // draft, which made the send look like it bounced back.
      if (sideChatAbortReasonRef.current === 'stop' && answer.trim()) {
        const stoppedMessages: AIWorkbenchSideChatMessage[] = [...requestMessages, { role: 'assistant', content: answer }]
        setSideChatMessages(stoppedMessages.map((message) => ({ ...message, id: attachmentId() })))
        return
      }
      setSideChatMessages(displayMessages)
      setSideChatFailedRequest({ messages: requestMessages, selectedText })
      if (error?.name !== 'AbortError') showWorkbenchError(error, 'sendSideChatFailed')
    } finally {
      if (sideChatAbortRef.current === controller) {
        sideChatAbortRef.current = null
        sideChatAbortReasonRef.current = null
        setSideChatSending(false)
        setSideChatLiveText('')
      }
    }
  }, [activeSessionId, effectiveSelectedModel, sideChatContextText, sideChatDraft, sideChatFailedRequest, sideChatHistoryState, sideChatMessages, sideChatSelectedModel, sideChatSending, showWorkbenchError])
  useEffect(() => {
    const sessionId = activeSessionId
    if (sideChatStateSessionRef.current === sessionId) return
    // Only a real conversation switch tears down side chat. Main-conversation
    // progress updates re-render the page but never change this id.
    sideChatStateSessionRef.current = sessionId
    setSideChatStateSessionId(sessionId)
    sideChatHistorySeqRef.current += 1
    sideChatRequestIdRef.current += 1
    const pendingChat = sideChatAbortRef.current
    sideChatAbortRef.current = null
    sideChatAbortReasonRef.current = 'session'
    pendingChat?.abort()
    sideChatHistoryAbortRef.current?.abort()
    sideChatHistoryAbortRef.current = null
    setSideChatMessages([])
    setSideChatDraft('')
    setSideChatContextText('')
    setSideChatLiveText('')
    setSideChatSending(false)
    setSideChatFailedRequest(null)
    setSideChatHistoryState('idle')
    if (sessionId) loadSideChatHistory(sessionId)
  }, [activeSessionId, loadSideChatHistory])
  useEffect(() => {
    const controllers = messageControllersRef.current
    const timer = window.setTimeout(() => void loadBootstrap(), 0)
    return () => {
      window.clearTimeout(timer)
      sessionAbortRef.current?.abort()
      for (const controller of controllers.values()) controller.abort()
      controllers.clear()
      sideChatAbortReasonRef.current = 'unmount'
      sideChatAbortRef.current?.abort()
      sideChatAbortRef.current = null
      sideChatHistoryAbortRef.current?.abort()
      sideChatHistoryAbortRef.current = null
      // Drop the session binding so a strict-mode remount reloads side chat.
      sideChatStateSessionRef.current = null
      sideChatHistorySeqRef.current += 1
      sideChatRequestIdRef.current += 1
    }
  }, [loadBootstrap])
  useEffect(() => {
    const initial = window.setTimeout(() => void refreshWorkspaceForActiveQueues(), 0)
    // 4s keeps Planner/Worker item state and the Workers rail fresh without
    // waiting for the operator to click the session again.
    const timer = window.setInterval(() => void refreshWorkspaceForActiveQueues(), 4000)
    return () => {
      window.clearTimeout(initial)
      window.clearInterval(timer)
    }
  }, [refreshWorkspaceForActiveQueues])
  useEffect(() => {
    if (!activeSessionId) return
    const controller = new AbortController()
    let retryTimer: ReturnType<typeof setTimeout> | undefined
    let retryDelay = 500
    let failures = 0
    const maxFailures = 6
    const connect = async () => {
      let terminal = false
      try {
        await streamAIWorkbenchPenTestSession(activeSessionId, {
          intervalMs: 2500, signal: controller.signal, onProcess: receiveProcess,
          onSnapshot: nextSession => {
            terminal = !activeStatuses.has(displayText(nextSession.task.status).toLowerCase())
            retryDelay = 500
            failures = 0
            if (activeSessionRef.current === activeSessionId) {
              receiveSnapshot(nextSession)
              syncSessionSnapshotToSidebar(nextSession)
            }
          },
        })
      } catch (error) {
        if (controller.signal.aborted) return
        if (error instanceof AIWorkbenchStreamError && error.fatal) {
          if (error.status === 404 || error.status === 410) {
            setSessionLoadError(l.loadSessionFailed)
            setSession(null)
            toast.error(l.loadSessionFailed)
          } else {
            toast.error(localizeBackendText(error.message || '', language) || l.streamDisconnected)
          }
          return
        }
        failures += 1
        if (failures >= maxFailures) {
          toast.error(l.streamDisconnected)
          return
        }
      }
      if (!controller.signal.aborted && terminal) {
        // Reconcile the left queue with the terminal snapshot and invalidate
        // any older refresh that may have returned while the Agent finished.
        await refreshWorkspace().catch(() => {})
      }
      if (!controller.signal.aborted && !terminal) {
        retryTimer = setTimeout(() => void connect(), retryDelay)
        retryDelay = Math.min(retryDelay * 2, 3000)
      }
    }
    void connect()
    return () => { controller.abort(); clearTimeout(retryTimer) }
  }, [activeSessionId, refreshWorkspace, syncSessionSnapshotToSidebar, receiveProcess, receiveSnapshot, l.loadSessionFailed, l.streamDisconnected, setSessionLoadError])
  const newConversation = useCallback((projectId?: string) => {
    sessionAbortRef.current?.abort(); activeSessionRef.current = null; processMerger.current.reset(''); setActiveSessionId(null); setSession(null); setSessionLoadError(''); setGoalOverride(null); createRequestIdRef.current=''; setSideChatOpen(false); setEditingMessageId(null); setEditedMessageContent('')
    clearSessionRoute()
    const nextProjectId = projectId ?? projects[0]?.id ?? ''
    setSelectedProjectId(nextProjectId)
    if (nextProjectId) setOpenProjects((previous) => ({ ...previous, [nextProjectId]: true }))
  }, [clearSessionRoute, projects])
  useEffect(() => {
    const handleShortcut = (event: globalThis.KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        event.stopPropagation()
        if (window.matchMedia('(max-width: 1023px)').matches) {
          setMobileSidebarOpen(true)
          setMobileSearchOpen(true)
        } else {
          setSearchOpen(true)
        }
        return
      }
    }
    window.addEventListener('keydown', handleShortcut, { capture: true })
    return () => window.removeEventListener('keydown', handleShortcut, { capture: true })
  }, [])
  useEffect(() => {
    const desktop = window.matchMedia('(min-width: 1024px)')
    const closeOnDesktop = () => {
      if (desktop.matches) {
        setMobileSidebarOpen(false)
        setMobileSearchOpen(false)
      }
    }
    desktop.addEventListener('change', closeOnDesktop)
    return () => desktop.removeEventListener('change', closeOnDesktop)
  }, [])
  const handleCreate = useCallback(async () => {
    const content = messageWithMCPTools(messageContentWithQuote(draft, messageQuote, l) || (assetReferences.length ? (language === 'zh' ? '请概览引用资产，并询问我希望测试的目标。' : 'Summarize the referenced assets and ask what I want to test.') : ''), mcpTools, language); const currentQuote = messageQuote; const normalizedTarget = extractScopeTargets(draft)[0] || ''
    if (!content && !attachments.length) return
    if (!selectedProjectId) { toast.error(l.projectRequired); return }
    const createSessionId = activeSessionRef.current
    const requestId = createRequestIdRef.current || attachmentId()
    createRequestIdRef.current = requestId
    const { providerId, model } = splitModelValue(effectiveSelectedModel); const currentAttachments = attachments; const currentReferences = assetReferences; const currentMCPTools = mcpTools; const currentGoalMode: GoalModeRequest = { action: goalMode }
    setSessionLoadError(''); setCreating(true); setDraft(''); setMessageQuote(null); setAttachments([]); setAssetReferences([]); setMCPTools([]); setSession(buildOptimisticSession(content, normalizedTarget, effectiveSelectedModel))
    try {
      const nextSession = await createAIWorkbenchPenTestSession({ content, target: normalizedTarget, projectId: selectedProjectId, agentProviderId: providerId, agentModel: model, clientRequestId: requestId, metadata: { entrypoint: 'ai_penetration_workbench', mode: 'solo_pentest', attachments: currentAttachments, assetReferences: currentReferences, goalMode: currentGoalMode } })
      createRequestIdRef.current = ''
      syncSessionSnapshotToSidebar(nextSession)
      if (activeSessionRef.current !== createSessionId) {
        await refreshWorkspace()
        return
      }
      processMerger.current.reset(nextSession.task.id); setSession(nextSession); setActiveSessionId(nextSession.task.id); activeSessionRef.current = nextSession.task.id; replaceSessionRoute(nextSession.task.id)
      setGoalOverride({ sessionId: nextSession.task.id, action: currentGoalMode.action === 'chat' ? 'chat' : 'continue' })
      const projectId = nextSession.project?.id || nextSession.task.projectId || selectedProjectId
      if (projectId) { setSelectedProjectId(projectId); setOpenProjects((previous) => ({ ...previous, [projectId]: true })) }
      await refreshWorkspace()
    } catch (error: any) { if (activeSessionRef.current === createSessionId) { setDraft(draft); setMessageQuote(currentQuote); setAttachments(currentAttachments); setAssetReferences(currentReferences); setMCPTools(currentMCPTools); if (!activeSessionRef.current) setSession(null) }; showWorkbenchError(error, 'createSessionFailed') } finally { setCreating(false) }
  }, [goalMode, language, assetReferences, mcpTools, attachments, draft, effectiveSelectedModel, messageQuote, refreshWorkspace, selectedProjectId, syncSessionSnapshotToSidebar, l, showWorkbenchError, replaceSessionRoute])
  const handleSend = useCallback(async (queuedMessage?: QueuedWorkbenchMessage) => {
    const sessionId = queuedMessage?.sessionId || activeSessionId
    const content = queuedMessage?.content ?? (messageWithMCPTools(messageContentWithQuote(draft, messageQuote, l) || (assetReferences.length ? (language === 'zh' ? '请概览引用资产，并询问我希望测试的目标。' : 'Summarize the referenced assets and ask what I want to test.') : ''), mcpTools, language))
    const currentMCPTools = queuedMessage ? [] : mcpTools
    const currentAttachments = queuedMessage?.attachments ?? attachments
    const currentReferences = queuedMessage?.assetReferences ?? assetReferences
    const currentGoalMode: GoalModeRequest = queuedMessage?.goalMode ?? { action: goalMode }
    if (!sessionId || (!content.trim() && !currentAttachments.length) || movingSessionRef.current === sessionId) return
    if (stopping) { toast.error(l.stoppingPleaseWait); return }
    if (sessionId !== activeSessionRef.current) return
    if (!session || session.task.id !== sessionId) return
    const runInProgress = sessionIsActive(session) || sending || messageControllersRef.current.has(sessionId)
    if (!queuedMessage && runInProgress) {
      const { providerId, model } = splitModelValue(effectiveSelectedModel)
      setQueuedMessages((previous) => [...previous, { id: attachmentId(), sessionId, content, attachments: currentAttachments, assetReferences: currentReferences, goalMode: currentGoalMode, agentProviderId: providerId, agentModel: model, status: 'queued' }])
      setDraft(''); setMessageQuote(null); setAttachments([]); setAssetReferences([]); setMCPTools([])
      return
    }
    if (queuedMessage && runInProgress) return
    const selectedProvider = splitModelValue(effectiveSelectedModel)
    const providerId = queuedMessage?.agentProviderId || selectedProvider.providerId
    const model = queuedMessage?.agentModel || selectedProvider.model
    const currentQuote = queuedMessage ? null : messageQuote
    const currentDraft = queuedMessage ? '' : draft
    const controller = new AbortController()
    messageControllersRef.current.set(sessionId, controller)
    const previousSession = session
    if (!queuedMessage) { setDraft(''); setMessageQuote(null); setAttachments([]); setAssetReferences([]); setMCPTools([]) }
    markSessionSending(sessionId, true)
    clearLiveText(sessionId)
    setSession((previous) => appendOptimisticTurn(previous, content))
    try {
      await streamAIWorkbenchPenTestSessionMessage(sessionId, content, {
        signal: controller.signal,
        agentProviderId: providerId,
        agentModel: model,
        metadata: { entrypoint: 'ai_penetration_workbench', attachments: currentAttachments, assetReferences: currentReferences, goalMode: currentGoalMode },
        onOpen: () => clearLiveText(sessionId),
        onEvent: (event) => {
          if (activeSessionRef.current !== sessionId) return
          const data = event.data
          if (data.process) { receiveProcess(data.process); return }
          if (data.accumulated) updateLiveText(sessionId, data.accumulated)
          else if (data.delta) appendLiveText(sessionId, data.delta)
          if (data.session) {
            receiveSnapshot(data.session)
            syncSessionSnapshotToSidebar(data.session)
            clearLiveText(sessionId)
          }
        },
      })
      if (!queuedMessage) setGoalOverride({ sessionId, action: currentGoalMode.action === 'chat' ? 'chat' : 'continue' })
      const nextSession = await getAIWorkbenchPenTestSession(sessionId)
      receiveSnapshot(nextSession)
      syncSessionSnapshotToSidebar(nextSession)
      await refreshWorkspace()
    } catch (error: any) {
      if (error?.name === 'AbortError' && queuedMessage) {
        // A queued prompt may have been dequeued just before the user pressed
        // stop. Preserve it as paused instead of silently losing it.
        setQueuedMessages((previous) => [{ ...queuedMessage, status: 'paused' }, ...previous.filter((message) => message.id !== queuedMessage.id)])
        pausedQueueSessionsRef.current.add(sessionId)
      }
      if (error?.name !== 'AbortError') {
        let accepted = false
        // An explicit client-error response rejected this request. An older
        // message with identical text must not suppress its error or recovery.
        const rejected = error instanceof WorkbenchRequestError
          && (error.key === 'messageStreamFailed' || error.key === 'messageStreamFailedWithReason')
          && Number(error.values.status) >= 400 && Number(error.values.status) < 500
        try {
          const latest = await getAIWorkbenchPenTestSession(sessionId)
          const latestTurn = [...(latest.turns || [])].at(-1)
          const latestUser = (latestTurn?.userMessageView?.content || latestTurn?.userMessage?.content || '').trim()
          const normalizedContent = content.trim()
          const manifest = latestUser.includes('[附件]')
          accepted = !rejected && (normalizedContent
            ? Boolean(latestUser) && (latestUser === normalizedContent || latestUser.startsWith(`${normalizedContent}\n\n[附件]`))
            : Number(latestTurn?.userMessage?.metadata?.imageCount || 0) >= currentAttachments.length || (currentAttachments.length > 0 && manifest))
          receiveSnapshot(latest)
          syncSessionSnapshotToSidebar(latest)
          await refreshWorkspace()
        } catch {
          if (activeSessionRef.current === sessionId) setSession(previousSession)
        }
        if (!accepted) {
          const currentComposer = composerSnapshotRef.current
          const canRestore = activeSessionRef.current === sessionId
            && !currentComposer.draft.trim()
            && currentComposer.attachments.length === 0
            && currentComposer.assetReferences.length === 0
            && !currentComposer.quote
          if (queuedMessage) {
            setQueuedMessages((previous) => [{ ...queuedMessage, status: 'failed' }, ...previous])
          } else if (canRestore) {
            setDraft(currentDraft)
            setMessageQuote(currentQuote)
            setAttachments(currentAttachments)
            setAssetReferences(currentReferences)
            setMCPTools(currentMCPTools)
          } else {
            // The user kept typing (or switched away). Preserve both the new
            // composer content and the failed request instead of overwriting
            // either one.
            setQueuedMessages((previous) => [{
              id: attachmentId(),
              sessionId,
              content,
              attachments: currentAttachments,
              assetReferences: currentReferences,
              goalMode: currentGoalMode,
              agentProviderId: providerId,
              agentModel: model,
              status: 'failed',
            }, ...previous])
          }
          showWorkbenchError(error, 'sendMessageFailed')
        }
      }
    } finally {
      if (messageControllersRef.current.get(sessionId) === controller) messageControllersRef.current.delete(sessionId)
      markSessionSending(sessionId, false)
      clearLiveText(sessionId)
    }
  }, [goalMode, language, receiveProcess, receiveSnapshot, activeSessionId, assetReferences, mcpTools, attachments, draft, effectiveSelectedModel, messageQuote, refreshWorkspace, sending, session, stopping, syncSessionSnapshotToSidebar, l, showWorkbenchError, markSessionSending, clearLiveText, updateLiveText, appendLiveText])
  const editQueuedMessage = useCallback((id: string) => {
    const target = queuedMessages.find((message) => message.id === id)
    if (!target || target.sessionId !== activeSessionRef.current) return
    setDraft(target.content)
    setMessageQuote(null)
    setAttachments(target.attachments)
    setAssetReferences(target.assetReferences || [])
    setMCPTools([])
    setGoalOverride({ sessionId: target.sessionId, action: target.goalMode?.action || 'chat' })
    setQueuedMessages((previous) => {
      const next = previous.filter((message) => message.id !== id)
      if (!next.some((message) => message.sessionId === target.sessionId)) pausedQueueSessionsRef.current.delete(target.sessionId)
      return next
    })
  }, [queuedMessages])
  const removeQueuedMessage = useCallback((id: string) => {
    setQueuedMessages((previous) => {
      const target = previous.find((message) => message.id === id)
      const next = previous.filter((message) => message.id !== id)
      if (target && !next.some((message) => message.sessionId === target.sessionId)) {
        pausedQueueSessionsRef.current.delete(target.sessionId)
      }
      return next
    })
  }, [])
  const retryQueuedMessage = useCallback((id: string) => {
    const target = queuedMessages.find((message) => message.id === id)
    if (target) pausedQueueSessionsRef.current.delete(target.sessionId)
    setQueuedMessages((previous) => previous.map((message) => message.id === id ? { ...message, status: 'queued' } : message))
  }, [queuedMessages])
  useEffect(() => {
    if (!activeSessionId || !session || session.task.id !== activeSessionId || pausedQueueSessionsRef.current.has(activeSessionId) || sessionIsActive(session) || sending || stopping || creating || sessionLoading || movingSessionId === activeSessionId || movingSessionRef.current === activeSessionId || messageControllersRef.current.has(activeSessionId) || queuedMessageDrainRef.current) return
    const nextIndex = queuedMessages.findIndex((message) => message.sessionId === activeSessionId)
    if (nextIndex < 0 || queuedMessages[nextIndex].status !== 'queued') return
    const next = queuedMessages[nextIndex]
    queuedMessageDrainRef.current = true
    // Dequeue before sending so rerenders cannot submit the same queue head twice.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setQueuedMessages((previous) => previous.filter((message) => message.id !== next.id))
    void handleSend(next).finally(() => { queuedMessageDrainRef.current = false })
  }, [activeSessionId, creating, handleSend, movingSessionId, queuedMessages, sending, session, sessionLoading, stopping])
  const handleEditStart = useCallback((turn: AIWorkbenchConversationTurn) => {
    const messageId = turn.userMessage?.id
    if (!messageId || sending || stopping || sessionIsActive(session)) return
    setEditingMessageId(messageId)
    setEditedMessageContent(turn.userMessageView?.content || turn.userMessage?.content || '')
  }, [sending, stopping, session])
  const handleEditCancel = useCallback(() => {
    setEditingMessageId(null)
    setEditedMessageContent('')
  }, [])
  const handleEditResend = useCallback(async () => {
    if (movingSessionRef.current === activeSessionId) return
    const content = editedMessageContent.trim()
    const messageId = editingMessageId
    const sessionId = activeSessionId
    if (!sessionId || !messageId || !content || sending || stopping || sessionIsActive(session)) return
    const { providerId, model } = splitModelValue(effectiveSelectedModel)
    const controller = new AbortController()
    messageControllersRef.current.set(sessionId, controller)
    setEditingMessageId(null)
    setEditedMessageContent('')
    markSessionSending(sessionId, true)
    clearLiveText(sessionId)
    try {
      await streamAIWorkbenchPenTestSessionMessage(sessionId, content, {
        editMessageId: messageId,
        signal: controller.signal,
        agentProviderId: providerId,
        agentModel: model,
        metadata: { entrypoint: 'ai_penetration_workbench', attachments: [] },
        onOpen: () => clearLiveText(sessionId),
        onEvent: (event) => {
          if (activeSessionRef.current !== sessionId) return
          const data = event.data
          if (data.process) { receiveProcess(data.process); return }
          if (data.accumulated) updateLiveText(sessionId, data.accumulated)
          else if (data.delta) appendLiveText(sessionId, data.delta)
          if (data.session) {
            receiveSnapshot(data.session)
            syncSessionSnapshotToSidebar(data.session)
            clearLiveText(sessionId)
          }
        },
      })
      const nextSession = await getAIWorkbenchPenTestSession(sessionId)
      receiveSnapshot(nextSession)
      syncSessionSnapshotToSidebar(nextSession)
      await refreshWorkspace()
    } catch (error: any) {
      if (error?.name !== 'AbortError') {
        try {
          const latest = await getAIWorkbenchPenTestSession(sessionId)
          receiveSnapshot(latest)
          syncSessionSnapshotToSidebar(latest)
          await refreshWorkspace()
        } catch { /* Keep the current snapshot when refresh also fails. */ }
        if (activeSessionRef.current === sessionId) {
          setEditingMessageId(messageId)
          setEditedMessageContent(content)
        }
        showWorkbenchError(error, 'resendFailed')
      }
    } finally {
      if (messageControllersRef.current.get(sessionId) === controller) messageControllersRef.current.delete(sessionId)
      markSessionSending(sessionId, false)
      clearLiveText(sessionId)
    }
  }, [activeSessionId, editingMessageId, editedMessageContent, receiveProcess, receiveSnapshot, refreshWorkspace, effectiveSelectedModel, sending, session, stopping, syncSessionSnapshotToSidebar, showWorkbenchError, markSessionSending, clearLiveText, updateLiveText, appendLiveText])
  const handleStop = useCallback(async () => {
    const sessionId = activeSessionId
    if (!sessionId || stopping) return
    const controller = messageControllersRef.current.get(sessionId)
    if (controller) {
      messageControllersRef.current.delete(sessionId)
      controller.abort()
    }
    markSessionStopping(sessionId, true)
    // A stop is a stop, not "run the next queued prompt immediately". Keep the
    // queue visible but paused until the user explicitly resumes it.
    pausedQueueSessionsRef.current.add(sessionId)
    setQueuedMessages((previous) => previous.map((message) => message.sessionId === sessionId && message.status !== 'failed' ? { ...message, status: 'paused' } : message))
    try {
      const nextSession = await runAIWorkbenchPenTestSessionLifecycleAction(sessionId, 'cancel')
      receiveSnapshot(nextSession)
      syncSessionSnapshotToSidebar(nextSession)
      await refreshWorkspace()
    } catch (error: any) {
      showWorkbenchError(error, 'stopFailed')
    } finally {
      markSessionStopping(sessionId, false)
      if (controller) clearLiveText(sessionId)
    }
  }, [activeSessionId, receiveSnapshot, refreshWorkspace, stopping, syncSessionSnapshotToSidebar, showWorkbenchError, markSessionStopping, clearLiveText])
  const openProjectDialog = useCallback((mode: 'create' | 'rename', project?: AIWorkbenchProject) => {
    setProjectDialog({ mode, project })
    setProjectName(project?.name || '')
  }, [])
  const saveProjectDialog = useCallback(async () => {
    const name = projectName.trim()
    if (!name) { toast.error(l.projectNameRequired); return }
    if (projectSaving) return
    setProjectSaving(true)
    try {
      if (projectDialog?.mode === 'rename' && projectDialog.project) {
        const nextProject = await updateAIWorkbenchPenTestProject(projectDialog.project.id, { name })
        setProjects((previous) => mergeProjectList(previous, nextProject))
        setSelectedProjectId(nextProject.id)
      } else {
        const nextProject = await createAIWorkbenchPenTestProject({ name })
        setProjects((previous) => mergeProjectList(previous, nextProject))
        setSelectedProjectId(nextProject.id)
        setOpenProjects((previous) => ({ ...previous, [nextProject.id]: true }))
      }
      setProjectDialog(null)
    } catch (error: any) {
      showWorkbenchError(error, 'saveProjectFailed')
    } finally {
      setProjectSaving(false)
    }
  }, [projectDialog, projectName, projectSaving, l, showWorkbenchError])
  const moveSessionToProject = useCallback(async (summary: AIWorkbenchConversationSummary, project: AIWorkbenchProject) => {
    const id = summary.task.id
    const previousProjectId = summary.project?.id || summary.task.projectId
    if (movingSessionRef.current || previousProjectId === project.id) return
    if (activeStatuses.has(summary.task.status) || messageControllersRef.current.has(id) || stoppingSessions.has(id)) {
      toast.error(l.moveSessionBusy)
      return
    }
    movingSessionRef.current = id
    setMovingSessionId(id)
    try {
      const nextSession = await updateAIWorkbenchPenTestSession(id, { projectId: project.id })
      // Invalidate sidebar reads started before the move, then reconcile both groups.
      workspaceRequestSeqRef.current++
      const nextSummary = summaryFromSnapshot(nextSession, l.unnamedProject)
      removeProjectSession(id)
      updateProjectSession(nextSummary)
      setSessions((previous) => mergeSessionSummaryList(previous, nextSummary))
      setProjects((previous) => previous.map((item) => {
        const conversations = (item.conversations || []).filter((entry) => entry.task.id !== id)
        if (item.id === project.id) return {
          ...item, ...nextSession.project,
          conversations: mergeSessionSummaryList(conversations, nextSummary),
          conversationCount: nextSession.project?.conversationCount ?? (item.conversationCount || 0) + 1,
        }
        if (item.id === previousProjectId) return { ...item, conversations, conversationCount: Math.max(0, (item.conversationCount || 0) - 1) }
        return item
      }))
      setOpenProjects((previous) => ({ ...previous, [project.id]: true }))
      if (activeSessionRef.current === id) {
        sessionAbortRef.current?.abort()
        setSessionLoading(false)
        receiveSnapshot(nextSession)
        setSelectedProjectId(project.id)
      }
      setSearch('')
      toast.success(formatWorkbenchText(l.sessionMoved, { project: project.name }))
      await refreshWorkspace().catch(() => toast.error(l.moveSessionRefreshFailed))
    } catch (error) {
      showWorkbenchError(error, 'moveSessionFailed')
      // Also reconciles a committed move whose snapshot response was interrupted.
      void refreshWorkspace().catch(() => {})
    } finally {
      movingSessionRef.current = null
      setMovingSessionId(null)
    }
  }, [l, receiveSnapshot, refreshWorkspace, showWorkbenchError, stoppingSessions, removeProjectSession, updateProjectSession])
  const saveSessionRename = useCallback(async () => {
    const name = sessionName.trim()
    if (!sessionRenameTarget || !name) { toast.error(l.sessionNameRequired); return }
    try {
      const nextSession = await updateAIWorkbenchPenTestSession(sessionRenameTarget.task.id, { title: name })
      if (activeSessionId === sessionRenameTarget.task.id) setSession(nextSession)
      setSessionRenameTarget(null)
      await refreshWorkspace()
    } catch (error: any) {
      showWorkbenchError(error, 'renameSessionFailed')
    }
  }, [activeSessionId, refreshWorkspace, sessionName, sessionRenameTarget, l, showWorkbenchError])
  const deleteSelectedTarget = useCallback(async () => {
    if (!deleteTarget) return
    const target = deleteTarget
    setDeleteTarget(null)
    // Optimistically remove the entry from the sidebar so it cannot be clicked
    // while the delete request is still in flight; a failure toast + refresh
    // restores the real state.
    if (target.type === 'project') {
      const projectId = target.project.id
      const removedSessionIds = new Set([
        ...(target.project.conversations || []).map((item) => item.task.id),
        ...sessions.filter((item) => item.task.projectId === projectId).map((item) => item.task.id),
      ])
      setQueuedMessages((previous) => previous.filter((message) => !removedSessionIds.has(message.sessionId)))
      for (const sessionId of removedSessionIds) pausedQueueSessionsRef.current.delete(sessionId)
      setProjects((previous) => previous.filter((project) => project.id !== projectId))
      setSessions((previous) => previous.filter((item) => item.task.projectId !== projectId))
      if (selectedProjectId === projectId || session?.task.projectId === projectId) {
        newConversation('')
      }
    } else {
      const sessionId = target.summary.task.id
      removeProjectSession(sessionId)
      abortMessageStream(sessionId)
      pausedQueueSessionsRef.current.delete(sessionId)
      setQueuedMessages((previous) => previous.filter((message) => message.sessionId !== sessionId))
      setSessions((previous) => previous.filter((item) => item.task.id !== sessionId))
      setProjects((previous) => previous.map((project) => ({
        ...project,
        conversations: (project.conversations || []).filter((item) => item.task.id !== sessionId),
      })))
      if (activeSessionId === sessionId) newConversation()
    }
    try {
      if (target.type === 'project') {
        await deleteAIWorkbenchPenTestProject(target.project.id)
      } else {
        await deleteAIWorkbenchPenTestSession(target.summary.task.id)
      }
    } catch (error: any) {
      showWorkbenchError(error, 'deleteFailed')
    } finally {
      await refreshWorkspace()
    }
  }, [activeSessionId, deleteTarget, newConversation, refreshWorkspace, selectedProjectId, session, sessions, showWorkbenchError, abortMessageStream, removeProjectSession])
  const downloadSessionMarkdown = useCallback(async (summary: AIWorkbenchConversationSummary) => {
    try {
      const report = await downloadAIWorkbenchPenTestSessionReport(summary.task.id)
      const blob = report.blob
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = report.filename || `${(summary.task.target || summary.task.name || 'pentest-report').replace(/[\\/:*?"<>|]+/g, '_')}.md`
      a.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error: any) {
      showWorkbenchError(error, 'downloadReportFailed')
    }
  }, [showWorkbenchError])
  const downloadActiveReport = useCallback(async () => {
    if (!activeSessionId) return
    try {
      const report = await downloadAIWorkbenchPenTestSessionReport(activeSessionId)
      const url = URL.createObjectURL(report.blob)
      const a = document.createElement('a')
      a.href = url
      a.download = session?.reportPreview?.attachment?.name || report.filename
      a.click()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) {
      showWorkbenchError(error, 'downloadReportFailed')
    }
  }, [activeSessionId, session?.reportPreview?.attachment?.name, showWorkbenchError])
  const openSessionBlackboard = useCallback((summary: AIWorkbenchConversationSummary) => {
    router.push(`/ai/workbench/exploration?session=${encodeURIComponent(summary.task.id)}`)
  }, [router])
  const openProjectBlackboard = useCallback((project: AIWorkbenchProject) => {
    if (project.id) router.push(`/ai/workbench/project?project=${encodeURIComponent(project.id)}`)
  }, [router])
  const handleProjectMenuAction = useCallback((action: ProjectMenuAction, project: AIWorkbenchProject) => {
    if (action === 'blackboard') void openProjectBlackboard(project)
    if (action === 'new-session') newConversation(project.id)
    if (action === 'rename') openProjectDialog('rename', project)
    if (action === 'delete') setDeleteTarget({ type: 'project', project })
  }, [newConversation, openProjectBlackboard, openProjectDialog])
  const handleSessionMenuAction = useCallback((action: SessionMenuAction, summary: AIWorkbenchConversationSummary) => {
    if (action === 'blackboard') void openSessionBlackboard(summary)
    if (action === 'rename') { setSessionRenameTarget(summary); setSessionName(summary.task.name || summary.task.target || '') }
    if (action === 'history') {
      setHistoryTarget(summary); setHistoryItems([]); setHistoryLoading(true)
      void getAIWorkbenchSupersededHistory(summary.task.id).then(items => { setHistoryItems(items); setHistoryLoading(false) }).catch(error => { setHistoryLoading(false); showWorkbenchError(error, 'loadSessionFailed') })
    }
    if (action === 'copy-target') void copyToClipboard(summary.task.target || summary.task.name || '').then((ok) => ok ? toast.success(l.targetCopied) : toast.error(l.copyFailed))
    if (action === 'export-markdown') void downloadSessionMarkdown(summary)
    if (action === 'delete') setDeleteTarget({ type: 'session', summary })
  }, [downloadSessionMarkdown, openSessionBlackboard, l, showWorkbenchError])
  const sendingCurrentSession = sending
  const stoppingCurrentSession = stopping
  const working = !(session?.task.planRunId && !isOrchestrationRunId(session.task.planRunId)) && (sendingCurrentSession || stoppingCurrentSession || sessionIsActive(session))
  const sideChatModelIsAvailable = modelOptions.some((option) => modelValue(option) === sideChatSelectedModel)
  const effectiveSideChatModel = (sideChatModelIsAvailable ? sideChatSelectedModel : '') || effectiveSelectedModel
  const sessionTokenUsage = session?.task.id === activeSessionId ? session.tokenUsage : undefined
  const sessionNodeName = session?.task.id === activeSessionId ? session.execution?.nodeId?.trim() : ''
  const openPlanController = useCallback(async () => {
    const runId = session?.task.planRunId || ''
    if (!runId) {
      toast.error(l.controllerUnavailable)
      return
    }
    try {
      const detail = await ensureAIPlanRunController(runId)
      const controllerId = detail.controllerConversation?.id || detail.run.controllerConversationId || ''
      if (!controllerId) throw new Error(l.controllerUnavailable)
      await loadSession(controllerId)
    } catch (error) {
      showWorkbenchError(error, 'loadSessionFailed')
    }
  }, [session?.task.planRunId, loadSession, showWorkbenchError, l.controllerUnavailable])
  const composerDisabled = Boolean(session?.task.readOnly || (session?.task.planRunId && !isOrchestrationRunId(session.task.planRunId))) || loading || sessionLoading || creating || session?.task.agentRole === 'worker' || session?.task.agentRole === 'planner' || Boolean(movingSessionId && movingSessionId === activeSessionId) || (!activeSessionId && !selectedProjectId) || Boolean(activeSessionId && (!session || session.task.id !== activeSessionId)) || Boolean(sessionLoadError) || (!bootstrap?.modelOptions.options.length && !selectedModel)
  // requestedSessionId covers the first paint of a deep link, before the
  // effect below moves it into activeSessionId.
  const pendingSessionId = activeSessionId || requestedSessionId
  const conversationLoading = sessionLoading && Boolean(pendingSessionId) && session?.task.id !== pendingSessionId
  const mainChatContent = <>
    <div className="min-h-0 flex-1" style={{ marginBottom: composerHeight }}>
      {sessionLoadError && !session ? <div className="flex h-full items-center justify-center px-6 py-10"><div className="max-w-md text-center"><AlertTriangle className="mx-auto h-8 w-8 text-tone-amber" aria-hidden="true" /><h2 className="mt-3 text-base font-semibold text-foreground">{l.loadSessionFailed}</h2><p className="mt-2 break-words text-sm leading-6 text-muted-foreground">{sessionLoadError}</p><Button type="button" className="mt-4" onClick={() => { if (activeSessionId) void loadSession(activeSessionId) }}>{l.retry}</Button></div></div> : conversationLoading ? <div role="status" aria-live="polite" className="flex h-full flex-col items-center justify-center gap-3 px-6 py-10 text-center"><Spinner aria-hidden="true" className="size-6 text-muted-foreground motion-reduce:animate-none" /><p className="text-sm text-muted-foreground">{l.loadingSession}</p></div> : <ConversationStream key={activeSessionId || 'empty'} session={session} liveAssistantText={liveAssistantText} working={working} bottomInset={0} editingMessageId={editingMessageId} editedContent={editedMessageContent} onEditStart={handleEditStart} onEditChange={setEditedMessageContent} onEditCancel={handleEditCancel} onEditSubmit={handleEditResend} onQuote={setMessageQuote} onSideChat={openSideChat} onDownloadReport={downloadActiveReport} />}
    </div>
    {session?.task.agentRole === 'planner' ? (
      <div className="mx-3 mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/35 px-3 py-2 text-xs text-muted-foreground">
        <Badge variant="outline" className="h-5 rounded px-1.5 text-[10px]">{t.aiPlan.planner}</Badge>
        <span className="min-w-0 flex-1">{l.plannerReadOnlyHint}</span>
        <Button type="button" variant="outline" size="sm" className="h-7 rounded-md px-2 text-xs" onClick={() => void openPlanController()}>{l.openController}</Button>
      </div>
    ) : null}
    {session?.task.planRunId && !isOrchestrationRunId(session.task.planRunId) && session.task.agentRole === 'controller' ? <p className="mx-3 mb-2 text-xs text-muted-foreground">{l.legacyOrchestrationReadOnly}</p> : null}
    {session?.task.agentRole === 'worker' ? (
      <div className="mx-3 mb-2 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/35 px-3 py-2 text-xs text-muted-foreground">
        <Badge variant="outline" className="h-5 rounded px-1.5 text-[10px]">{l.worker}</Badge>
        <span className="min-w-0 flex-1">{l.workerReadOnlyHint}</span>
        {session.task.projectId ? <Button type="button" variant="ghost" size="sm" className="h-7 rounded-md px-2 text-xs" onClick={() => router.push(`/ai/workbench/project?project=${encodeURIComponent(session.task.projectId!)}&tab=plan`)}>{l.viewPlan}</Button> : null}
      </div>
    ) : null}
    <CompoundComposer composerRef={composerRef} bootstrap={bootstrap} projects={projects} selectedProjectId={selectedProjectId} value={draft} attachments={attachments} assetReferences={assetReferences} onAssetReferencesChange={setAssetReferences} mcpTools={mcpTools} onMCPToolsChange={setMCPTools} goalMode={goalMode} goalRun={session?.goalRun} onGoalModeChange={changeGoalMode} quote={messageQuote} selectedModel={effectiveSelectedModel} queuedMessages={queuedMessages.filter((message) => message.sessionId === activeSessionId)} hasSession={Boolean(activeSessionId)} disabled={composerDisabled} working={working} sending={sendingCurrentSession || stoppingCurrentSession} stopping={stoppingCurrentSession} creating={creating} onChange={setDraft} onQuoteChange={setMessageQuote} onQuoteRemove={() => setMessageQuote(null)} onAttachmentsChange={setAttachments} onProjectChange={setSelectedProjectId} onModelChange={handleModelChange} onQueuedMessageEdit={editQueuedMessage} onQueuedMessageRemove={removeQueuedMessage} onQueuedMessageRetry={retryQueuedMessage} onSubmit={() => void (activeSessionId ? handleSend() : handleCreate())} onStop={() => void handleStop()} />
  </>
  const sideChatVisible = sideChatStateSessionId === activeSessionId
  const sideChatPanel = <SessionSideChat messages={sideChatVisible ? sideChatMessages : []} contextText={sideChatVisible ? sideChatContextText : ''} draft={sideChatDraft} model={effectiveSideChatModel} modelOptions={modelOptions} liveText={sideChatLiveText} sending={sideChatSending} loadingHistory={sideChatHistoryState === 'loading'} retryable={sideChatVisible && Boolean(sideChatFailedRequest)} onClose={closeSideChat} onContextRemove={() => setSideChatContextText('')} onDraftChange={setSideChatDraft} onModelChange={setSideChatSelectedModel} onSubmit={() => void sendSideChat()} onStop={stopSideChat} onRetry={() => void sendSideChat({ retry: true })} />
  const projectSidebar = (mobile: boolean) => {
    const closeMobileSidebar = () => {
      if (mobile) {
        setMobileSidebarOpen(false)
        setMobileSearchOpen(false)
      }
    }
    return <aside className={cn('flex h-full min-h-0 shrink-0 flex-col overflow-hidden border-r border-border/80 bg-muted/10', mobile ? 'w-full' : 'w-[310px]')}>
      {mobile ? <SheetHeader className="flex h-12 shrink-0 flex-row items-center justify-between border-b border-border/80 px-4 py-0">
        <SheetTitle className="text-sm">{l.projectsAndSessions}</SheetTitle>
        <Button type="button" variant="ghost" size="icon" className="h-8 w-8 rounded-md" aria-label={l.hideSidebar} onClick={closeMobileSidebar}><X className="h-4 w-4" /></Button>
      </SheetHeader> : null}
      <div className="border-b border-border/80 p-4">
        <div className="flex gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button type="button" onClick={() => { newConversation(); closeMobileSidebar() }} aria-label={l.newConversation} className="h-11 min-w-0 flex-1 gap-2 rounded-xl text-sm font-semibold"><MessageSquarePlus className="h-4 w-4" />{l.newConversation}</Button>
            </TooltipTrigger>
            <TooltipContent>{l.newConversation}</TooltipContent>
          </Tooltip>
          <Popover open={mobile ? mobileSearchOpen : searchOpen} onOpenChange={mobile ? setMobileSearchOpen : setSearchOpen}>
            <Tooltip>
              <TooltipTrigger asChild>
                <PopoverTrigger asChild>
                  <Button type="button" variant={search ? 'default' : 'outline'} size="icon" className="h-11 w-11 shrink-0 rounded-xl" aria-label={l.searchConversations}>
                    <Search className="h-4 w-4" />
                  </Button>
                </PopoverTrigger>
              </TooltipTrigger>
              <TooltipContent>{l.searchShortcut}</TooltipContent>
            </Tooltip>
            <PopoverContent align="end" className="w-72 p-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground/70" />
                <Input data-ai-workbench-search-input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={l.searchPlaceholder} className="h-9 rounded-lg border-border/70 bg-background pl-8 pr-8 text-sm shadow-none" />
                {search ? <Button type="button" variant="ghost" size="icon" onClick={() => setSearch('')} className="absolute right-0.5 top-1/2 h-8 w-8 -translate-y-1/2 rounded-md text-muted-foreground" aria-label={l.clearSearch}><X className="h-3.5 w-3.5" /></Button> : null}
              </div>
            </PopoverContent>
          </Popover>
        </div>
      </div>
      <ScrollArea className="min-h-0 w-full max-w-full flex-1 overflow-hidden [&_[data-radix-scroll-area-viewport]>div]:!block [&_[data-radix-scroll-area-viewport]>div]:!max-w-full [&_[data-radix-scroll-area-viewport]>div]:!min-w-0 [&_[data-radix-scroll-area-viewport]>div]:!w-full">
        <div className="box-border w-full max-w-full min-w-0 space-y-2 overflow-hidden p-3">
          <div className="group/project-heading flex items-center justify-between px-2 py-1">
            <span className="text-xs font-semibold text-muted-foreground">{l.projects}</span>
            <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" onClick={() => { closeMobileSidebar(); openProjectDialog('create') }} className="pointer-events-none h-7 w-7 rounded-md text-muted-foreground opacity-0 [@media(hover:none)]:pointer-events-auto [@media(hover:none)]:opacity-100 group-hover/project-heading:pointer-events-auto group-hover/project-heading:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100" aria-label={l.createProject}><Plus className="h-4 w-4" /></Button></TooltipTrigger><TooltipContent>{l.createProject}</TooltipContent></Tooltip>
          </div>
          {loading ? [0, 1, 2].map((item) => (
            <div key={item} className="flex h-10 animate-pulse items-center gap-2.5 rounded-lg border border-border/60 bg-muted-foreground/[0.07] px-3">
              <div className="h-4 w-4 shrink-0 rounded bg-muted-foreground/20" />
              <div className="h-3 flex-1 rounded-full bg-muted-foreground/20" />
            </div>
          )) : groupedProjects.length ? groupedProjects.map((project) => <ProjectGroup key={project.id || 'unassigned'} projects={projects} sessionPage={projectSessionPages[project.id]?.query === search.trim() ? projectSessionPages[project.id] : undefined} onLoadSessions={(more) => loadProjectSessions(project.id, more)} moving={Boolean(movingSessionId)} onMove={(summary, destination) => { closeMobileSidebar(); void moveSessionToProject(summary, destination) }} project={project} activeSessionId={activeSessionId} selectedProjectId={selectedProjectId} open={Boolean(openProjects[project.id])} onOpenChange={(open) => setOpenProjects((previous) => ({ ...previous, [project.id]: open }))} onSelectProject={() => setSelectedProjectId(project.id)} onSelectSession={(id) => { closeMobileSidebar(); void loadSession(id) }} onProjectMenuAction={(action, selectedProject) => { closeMobileSidebar(); handleProjectMenuAction(action, selectedProject) }} onSessionMenuAction={(action, selectedSession) => { closeMobileSidebar(); handleSessionMenuAction(action, selectedSession) }} />) : !projects.length && !sessions.length ? (
            <div className="mx-2 mt-12 flex flex-col items-center rounded-2xl border border-dashed border-border/80 bg-muted/20 px-5 py-8 text-center">
              <div className="mb-4 flex h-12 w-12 items-center justify-center rounded-2xl border border-border/80 bg-background text-muted-foreground shadow-sm">
                <Folder className="h-5 w-5" aria-hidden="true" />
              </div>
              <p className="text-sm font-semibold text-foreground">{l.noProjects}</p>
              <p className="mt-2 text-xs leading-5 text-muted-foreground">{l.noProjectsHint}</p>
              <Button type="button" variant="outline" size="sm" onClick={() => { closeMobileSidebar(); openProjectDialog('create') }} className="mt-5 gap-2 rounded-lg">
                <Plus className="h-4 w-4" aria-hidden="true" />{l.createProject}</Button>
            </div>
          ) : <div className="px-4 py-14 text-center text-sm text-muted-foreground">{l.noMatches}</div>}
          {search.trim() && groupedProjects.length > 0 && sessions.length < sessionTotal ? <div className="px-2 pt-2"><Button type="button" variant="outline" size="sm" className="w-full rounded-lg" disabled={loadingMoreSessions} onClick={() => void loadMoreSessions()}>{loadingMoreSessions ? <><Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />{l.loadingSessions}</> : l.loadMoreSessions}</Button></div> : null}
          {projects.length < projectTotal ? <div className="px-2 pt-2"><Button type="button" variant="outline" size="sm" className="w-full rounded-lg" disabled={loadingMoreProjects} onClick={() => void loadMoreProjects()}>{loadingMoreProjects ? <><Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />{l.loadingSessions}</> : l.loadMoreProjects}</Button></div> : null}
        </div>
      </ScrollArea>
    </aside>
  }
  return <>
  <style jsx global>{`
    @keyframes ai-session-loader {
      0%, 100% {
        opacity: 0.28;
        transform: translateY(0) scaleX(0.72);
      }
      35% {
        opacity: 1;
        transform: translateY(-1px) scaleX(1.45);
      }
      70% {
        opacity: 0.42;
        transform: translateY(0) scaleX(0.86);
      }
    }
  `}</style>
  <div className="flex h-full min-h-0 overflow-hidden bg-background text-foreground">
    {!sidebarCollapsed ? <div className="hidden h-full lg:block">{projectSidebar(false)}</div> : null}
    <main className="relative flex min-w-0 flex-1 flex-col">
      <div className="flex min-h-12 shrink-0 flex-wrap items-center justify-between gap-x-1 border-b border-border/70 px-2 md:h-11 md:min-h-0 md:flex-nowrap md:gap-2 md:px-4">
        <div className="flex shrink-0 items-center gap-1">
          <Button type="button" variant="ghost" size="icon" className="h-11 w-11 shrink-0 rounded-lg text-muted-foreground hover:text-foreground lg:hidden" aria-label={l.showSidebar} aria-expanded={mobileSidebarOpen} onClick={() => setMobileSidebarOpen(true)}><PanelLeftOpen className="h-4 w-4" /></Button>
          <Tooltip><TooltipTrigger asChild><Button type="button" variant="ghost" size="icon" className="hidden h-8 w-8 shrink-0 rounded-lg text-muted-foreground hover:text-foreground lg:inline-flex" aria-label={sidebarCollapsed ? l.showSidebar : l.hideSidebar} aria-expanded={!sidebarCollapsed} onClick={() => { if (!sidebarCollapsed) setSearchOpen(false); setSidebarCollapsed((value) => !value) }}>{sidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}</Button></TooltipTrigger><TooltipContent side="bottom">{sidebarCollapsed ? l.showList : l.hideList}</TooltipContent></Tooltip>
          <Button type="button" variant="ghost" size="sm" className={cn('h-11 md:h-8 gap-1.5 rounded-lg text-muted-foreground hover:text-foreground', !sidebarCollapsed && 'lg:hidden')} onClick={() => newConversation()}><MessageSquarePlus className="h-4 w-4" />{l.newConversation}</Button>
        </div>
        <div className="flex min-w-0 items-center gap-2">
          {sessionNodeName ? <span className="hidden md:inline max-w-24 truncate text-xs text-muted-foreground sm:max-w-48 xl:max-w-72" title={sessionNodeName}>{sessionNodeName}</span> : null}
          <ProjectTokenUsage projectId={activeSessionId ? (session?.task.id === activeSessionId ? session.task.projectId : undefined) : selectedProjectId} />
          {sessionTokenUsage ? <TokenUsageButton usage={sessionTokenUsage} /> : null}
          {!sideChatOpen ? <Button ref={sideChatTriggerRef} disabled={Boolean(session?.task.planRunId && !isOrchestrationRunId(session.task.planRunId))} type="button" variant="ghost" size="sm" className="h-11 md:h-8 gap-2 rounded-lg text-muted-foreground hover:text-foreground" onClick={() => openSideChat()}><MessageSquarePlus className="h-4 w-4" />{l.sideChat}</Button> : <span className="text-xs text-muted-foreground">{l.mainConversation}</span>}
        </div>
      </div>
      {sideChatOpen ? <div className="relative min-h-0 flex-1">
        <div className="hidden h-full min-h-0 md:block">
        <ResizablePanelGroup direction="horizontal" className="min-h-0 flex-1">
          <ResizablePanel defaultSize={58} minSize={38} maxSize={72} className="min-w-0"><section className="relative flex h-full min-h-0 min-w-0 flex-col">{mainChatContent}</section></ResizablePanel>
          <ResizableHandle withHandle className="z-10 w-px bg-border/60 transition-colors hover:bg-primary/40 focus-visible:bg-primary/50" />
          <ResizablePanel defaultSize={42} minSize={28} maxSize={62} className="min-w-0">{sideChatPanel}</ResizablePanel>
        </ResizablePanelGroup>
        </div>
        <div className="absolute inset-0 z-40 md:hidden" role="dialog" aria-modal="true" aria-label={l.sideChatPanel} onKeyDown={(event) => {
          if (event.key === 'Escape') { event.preventDefault(); closeSideChat(); return }
          if (event.key !== 'Tab') return
          const root = event.currentTarget
          const focusables = Array.from(root.querySelectorAll<HTMLElement>('button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])')).filter(element => element.getClientRects().length > 0)
          if (!focusables.length) return
          const first = focusables[0]
          const last = focusables[focusables.length - 1]
          if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
          else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
        }}>{sideChatPanel}</div>
      </div> : <section className="relative flex min-h-0 min-w-0 flex-1 flex-col">{mainChatContent}</section>}
    </main>
    <Sheet open={mobileSidebarOpen} onOpenChange={(open) => { setMobileSidebarOpen(open); if (!open) setMobileSearchOpen(false) }}>
      <SheetContent side="left" className="w-[min(320px,calc(100vw-48px))] max-w-none gap-0 p-0 lg:hidden [&>button]:hidden">{projectSidebar(true)}</SheetContent>
    </Sheet>
    <Dialog open={Boolean(projectDialog)} onOpenChange={(open) => { if (!open && !projectSaving) setProjectDialog(null) }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{projectDialog?.mode === 'rename' ? l.renameProject : l.createProject}</DialogTitle></DialogHeader>
        <Input value={projectName} onChange={(event) => setProjectName(event.target.value)} placeholder={l.projectName} autoFocus disabled={projectSaving} onKeyDown={(event) => { if (event.key === 'Enter') void saveProjectDialog() }} />
        <DialogFooter><Button type="button" variant="outline" disabled={projectSaving} onClick={() => setProjectDialog(null)}>{l.cancel}</Button><Button type="button" disabled={projectSaving} onClick={() => void saveProjectDialog()}>{projectSaving ? <><Loader2 className="h-4 w-4 animate-spin" />{l.saving}</> : l.save}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <Dialog open={Boolean(sessionRenameTarget)} onOpenChange={(open) => { if (!open) setSessionRenameTarget(null) }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{l.renameSession}</DialogTitle></DialogHeader>
        <Input value={sessionName} onChange={(event) => setSessionName(event.target.value)} placeholder={l.sessionName} autoFocus onKeyDown={(event) => { if (event.key === 'Enter') void saveSessionRename() }} />
        <DialogFooter><Button type="button" variant="outline" onClick={() => setSessionRenameTarget(null)}>{l.cancel}</Button><Button type="button" onClick={() => void saveSessionRename()}>{l.save}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <Dialog open={Boolean(historyTarget)} onOpenChange={(open) => { if (!open) { setHistoryTarget(null); setHistoryItems([]) } }}>
      <DialogContent className="max-w-2xl">
        <DialogHeader><DialogTitle>{l.supersededHistory}</DialogTitle><DialogDescription>{historyTarget?.task.name || historyTarget?.task.target || ''}</DialogDescription></DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          {historyLoading ? <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" />{l.historyLoading}</div> : historyItems.length ? <div className="space-y-2">{historyItems.map(item => <div key={`${item.type}:${item.id}`} className="rounded-lg border border-border/70 bg-muted/20 px-3 py-2"><div className="flex min-w-0 items-center gap-2"><Badge variant="outline" className="h-5 shrink-0 px-1.5 text-[10px]">{item.type}</Badge><span className="min-w-0 flex-1 truncate text-sm font-medium" title={item.title}>{item.title || item.id}</span><span className="shrink-0 text-[11px] text-muted-foreground">{formatDateTime(item.supersededAt, language)}</span></div>{item.detail ? <p className="mt-1 truncate text-xs text-muted-foreground" title={item.detail}>{item.detail}</p> : null}</div>)}</div> : <p className="py-8 text-center text-sm text-muted-foreground">{l.historyEmpty}</p>}
        </div>
        <DialogFooter><Button type="button" variant="outline" onClick={() => { setHistoryTarget(null); setHistoryItems([]) }}>{l.close}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <AlertDialog open={Boolean(deleteTarget)} onOpenChange={(open) => { if (!open) setDeleteTarget(null) }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{deleteTarget?.type === 'project' ? l.deleteProject : l.deleteSession}</AlertDialogTitle>
          <AlertDialogDescription>{deleteTarget?.type === 'project' ? formatWorkbenchText(l.deleteProjectDescription, { name: deleteTarget.project.name }) : formatWorkbenchText(l.deleteSessionDescription, { name: deleteTarget?.summary.task.name || deleteTarget?.summary.task.target || '' })}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter><AlertDialogCancel>{l.cancel}</AlertDialogCancel><AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90" onClick={() => void deleteSelectedTarget()}>{l.delete}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </div>
  </>
}
