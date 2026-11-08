import type { AIBatchItem, AIBatchQueue } from './api/ai-batch'

export type ItemFilter = 'all' | 'unfinished' | 'failed' | 'completed'
export type InstructionMode = 'single' | 'batch'
export const isActiveQueue = (queue: AIBatchQueue) => ['running', 'pausing'].includes(queue.status)
export const canPauseQueue = (queue: AIBatchQueue) => ['pending', 'running'].includes(queue.status)
export const canResumeQueue = (queue: AIBatchQueue) => ['pending', 'paused'].includes(queue.status) && queue.items.some(item => ['pending', 'paused'].includes(item.status))
export const canRetryQueue = (queue: AIBatchQueue) => !isActiveQueue(queue) && queue.items.some(item => item.status === 'failed')
export function summarizeBatch(queue: AIBatchQueue) {
  const completed = queue.items.filter(item => item.status === 'completed').length
  const failed = queue.items.filter(item => item.status === 'failed').length
  return { completed, failed, processed: completed + failed, total: queue.items.length, unfinished: queue.items.length - completed }
}
export function filterBatchItems(items: AIBatchItem[], filter: ItemFilter, query = '') {
  const needle = query.trim().toLocaleLowerCase()
  return items.filter(item => (filter === 'all' || (filter === 'unfinished' ? item.status !== 'completed' : item.status === filter)) && (!needle || `${item.instruction} ${item.error}`.toLocaleLowerCase().includes(needle)))
}
export function parseBatchInstructions(value: string, mode: InstructionMode) {
  return (mode === 'single' ? [value] : value.split(/\r?\n/)).map(row => row.trim()).filter(Boolean)
}
export function batchDetailHref(path: string, current: string, queueId: string, itemId?: string, filter?: ItemFilter) {
  const params = new URLSearchParams(current)
  params.set('queue', queueId)
  params.delete('item')
  if (itemId) params.set('item', itemId)
  if (filter) params.set('items', filter)
  else params.delete('items')
  return `${path}?${params.toString()}`
}
export interface QueueDeleteResult { id: string; name: string; error?: string; skipped?: boolean }
// Each server deletion enforces the stop acknowledgement too. A concurrent resume
// can therefore reject one item without preventing deletion of the other items.
export async function deleteQueueSelection(queues: AIBatchQueue[], ids: string[], remove: (id: string) => Promise<unknown>, activeMessage: string): Promise<QueueDeleteResult[]> {
  const results: QueueDeleteResult[] = []
  for (const id of new Set(ids)) {
    const queue = queues.find(q => q.id === id)
    if (!queue || isActiveQueue(queue)) {
      results.push({ id, name: queue?.name || id, skipped: true, error: activeMessage })
      continue
    }
    try { await remove(id); results.push({ id, name: queue.name }) }
    catch (error) { results.push({ id, name: queue.name, error: error instanceof Error ? error.message : String(error) }) }
  }
  return results
}
