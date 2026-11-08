import type { Locale } from '@/lib/i18n'
import { zh } from '@/lib/i18n/locales/zh'
import { en } from '@/lib/i18n/locales/en'

export type WorkbenchMessages = Locale['aiWorkbench']

export function formatWorkbenchText(template: string, values: Record<string, string | number>) {
  return template.replace(/\{(\w+)\}/g, (placeholder, key: string) => String(values[key] ?? placeholder))
}

// Translate only known system activity labels. Model output, evidence and user
// content remain verbatim, including when they contain these words.
const activityKeys = [
  'agentThinking', 'agentPreparing', 'agentProgress', 'recognizingTarget',
  'findingLead', 'adjustingStrategy', 'analysisCommentary', 'reasoningProcess',
  'callingTool', 'toolCompleted', 'toolFailed', 'toolBlocked', 'toolCancelled',
  'toolInterrupted', 'toolTimeout', 'toolEnvironmentFailed', 'compactingContext',
  'synthesizing', 'synthesisCompleted', 'synthesisInterrupted', 'modelRetry',
  'modelRetriesExhausted', 'modelFallback', 'turnFailed', 'turnInterrupted',
  'workerQueued', 'workerCancelling', 'workerRecovering', 'dialogueCompleted', 'manualApproval', 'missingResults', 'sessionPreparing', 'legacyOrchestrationReadOnly',
] as const satisfies readonly (keyof WorkbenchMessages)[]
const toolActivityKeys = new Set<keyof WorkbenchMessages>([
  'callingTool', 'toolCompleted', 'toolFailed', 'toolBlocked', 'toolCancelled',
  'toolInterrupted', 'toolTimeout', 'toolEnvironmentFailed',
])

export function localizeWorkbenchActivity(label: string, messages: WorkbenchMessages) {
  for (const key of activityKeys) {
    for (const source of [zh.aiWorkbench[key], en.aiWorkbench[key]]) {
      if (label === source) return messages[key]
      if (toolActivityKeys.has(key) && (label.startsWith(`${source}：`) || label.startsWith(`${source}: `))) {
        const rawName = label.slice(source.length + 1).trim()
        const name = rawName === zh.aiWorkbench.securityCheck || rawName === en.aiWorkbench.securityCheck ? messages.securityCheck : rawName
        return `${messages[key]}${messages === zh.aiWorkbench ? '：' : ': '}${name}`
      }
    }
  }
  return label
}

export class WorkbenchRequestError extends Error {
  constructor(public readonly key: keyof WorkbenchMessages, public readonly values: Record<string, string | number> = {}) {
    // Preserve the existing default for callers that have not adopted keyed errors.
    super(formatWorkbenchText(zh.aiWorkbench[key], values))
    this.name = 'WorkbenchRequestError'
  }
}

const backendEnglishReplacements: Array<[string, string]> = [
  ['工具执行失败：', 'Tool execution failed: '],
  ['工具执行失败', 'Tool execution failed'],
  ['执行失败：', 'Execution failed: '],
  ['任务已取消。\n\n本轮未生成新的最终结论；已完成的工具调用、黑板事实和漏洞证据已保存。', 'Task cancelled.\n\nNo new final conclusion was produced in this turn; completed tool calls, blackboard facts, and vulnerability evidence were saved.'],
  ['任务已取消。', 'Task cancelled.'],
  ['会话不存在或已被删除', 'Conversation not found or deleted'],
  ['会话不存在或使用已移除的执行模式', 'Conversation not found or uses a removed execution mode'],
  ['主会话不存在或不可用', 'Main conversation not found or unavailable'],
  ['独立渗透 Agent 正在执行，请在本轮结束或停止后发送下一条消息', 'The agent is still running. Wait for this turn to finish or stop it before sending another message.'],
  ['会话正在执行，请在本轮结束或停止后重试', 'The conversation is still running. Wait for this turn to finish or stop it before retrying.'],
  ['请先配置可用的 AI 工作台模型', 'Configure an available AI workbench model first.'],
  ['所选 AI 渠道不可用', 'The selected AI provider is unavailable.'],
  ['会话已停止，部分关联扫描任务取消失败: ', 'Conversation stopped, but some owned scans could not be cancelled: '],
  ['本轮尚未完成，请查看缺失结果或阻断原因', 'This turn is incomplete; review missing results or blocking reasons'],
  ['每条消息最多上传 6 张图片', 'Up to 6 images can be uploaded per message'],
  ['仅支持上传 PNG、JPEG、GIF、WebP 图片', 'Only PNG, JPEG, GIF, and WebP images are supported'],
  ['图片不能超过 8 MB', 'Image must not exceed 8 MB'],
  ['图片数据无效', 'Invalid image data'],
  ['无法读取图片，请上传有效的 PNG、JPEG、GIF、WebP 图片', 'Unable to read image; upload a valid PNG, JPEG, GIF, or WebP image'],
  ['不支持的图片格式', 'Unsupported image format'],
  ['侧边聊天请求格式无效', 'Invalid side chat request'],
  ['侧边聊天消息数量无效', 'Invalid side chat message count'],
  ['侧边聊天仅支持用户和助手消息', 'Side chat supports only user and assistant messages'],
  ['侧边聊天消息过长，请缩短后重试', 'Side chat message is too long; shorten it and retry'],
  ['侧边聊天最后一条消息必须来自用户', 'The last side chat message must come from the user'],
  ['选中文本过长，请缩短后重试', 'Selected text is too long; shorten it and retry'],
  ['读取主会话上下文失败', 'Failed to read the main conversation context'],
  ['AI 服务返回内容为空', 'The AI service returned an empty response'],
  ['会话不存在或不可用', 'Conversation not found or unavailable'],
]

/** Translate system-authored Chinese text. Model/tool output is left verbatim. */
export function localizeBackendText(text: string, language: 'zh' | 'en') {
  if (language === 'zh' || !text) return text
  let result = text
  for (const [source, target] of backendEnglishReplacements) {
    result = result.split(source).join(target)
  }
  result = result.replace(/附件 (\d+)：/g, 'Attachment $1: ')
  return result
}

const cancelledTurnChinese = '任务已取消。\n\n本轮未生成新的最终结论；已完成的工具调用、黑板事实和漏洞证据已保存。'

/** Translate only system-authored prefixes in an assistant message. */
export function localizeAssistantText(text: string, language: 'zh' | 'en') {
  if (language === 'zh' || !text) return text
  if (text.trim() === cancelledTurnChinese || text.trim() === '任务已取消。') {
    return 'Task cancelled.\n\nNo new final conclusion was produced in this turn; completed tool calls, blackboard facts, and vulnerability evidence were saved.'
  }
  if (text.startsWith('工具执行失败：')) return `Tool execution failed: ${text.slice('工具执行失败：'.length)}`
  if (text.startsWith('执行失败：')) return `Execution failed: ${text.slice('执行失败：'.length)}`
  return text
}

export function workbenchErrorMessage(error: unknown, messages: WorkbenchMessages, fallback: string) {
  if (error instanceof WorkbenchRequestError) return formatWorkbenchText(messages[error.key], error.values)
  const language = messages === en.aiWorkbench ? 'en' : 'zh'
  return error instanceof Error && error.message ? localizeBackendText(error.message, language) : fallback
}
