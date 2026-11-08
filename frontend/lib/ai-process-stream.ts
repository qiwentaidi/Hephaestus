import type { AIWorkbenchChatItem, AIWorkbenchPenTestSessionSnapshot } from './api/ai-workbench'

export interface AIProcessUpdate {
  sessionId: string
  blockId: string
  runId: string
  sequence: number
  item: AIWorkbenchChatItem | null
}

// Prefer the next paint, but don't strand text in an occluded window where rAF
// may be suspended even while the SSE connection is still receiving data.
export function scheduleProcessFrame(flush: () => void): () => void {
  let finished = false
  const run = () => {
    if (finished) return
    finished = true
    cancelAnimationFrame(frame)
    clearTimeout(timer)
    flush()
  }
  const frame = requestAnimationFrame(run)
  const timer = setTimeout(run, 50)
  return () => { finished = true; cancelAnimationFrame(frame); clearTimeout(timer) }
}

// A page owns one merger shared by the POST stream and reconnect subscription.
// Accumulated block text + sequence makes duplicate delivery idempotent.
export class AIProcessMerger {
  private sessionId = ''
  private blocks = new Map<string, AIProcessUpdate>()

  reset(sessionId: string) {
    if (this.sessionId === sessionId) return
    this.sessionId = sessionId
    this.blocks.clear()
  }

  accept(update: AIProcessUpdate): boolean {
    if (update.sessionId !== this.sessionId || !update.blockId) return false
    const previous = this.blocks.get(update.blockId)
    if (previous && previous.sequence >= update.sequence) return false
    this.blocks.set(update.blockId, update)
    return true
  }

  merge(snapshot: AIWorkbenchPenTestSessionSnapshot): AIWorkbenchPenTestSessionSnapshot {
    if (snapshot.task.id !== this.sessionId) return snapshot
    const terminal = !['running', 'pending', 'pausing'].includes(snapshot.task.status)
    if (terminal) { this.blocks.clear(); return snapshot }
    // GET and POST snapshots can cross in flight. Remember newer durable block
    // revisions as well, so a later stale snapshot cannot roll the text back.
    for (const item of snapshot.chatItems || []) {
      const previous = this.blocks.get(item.id)
      const isProcess = ['reasoning', 'assistant_commentary'].includes(String(item.metadata?.eventType))
      const sequence = Number(item.metadata?.sequence || 0)
      if ((previous || isProcess) && (!previous || sequence > previous.sequence)) {
        this.blocks.set(item.id, { sessionId: this.sessionId, blockId: item.id, runId: item.runId || '', sequence, item })
      }
    }
    const updates = [...this.blocks.values()].sort((a, b) => a.sequence - b.sequence)
    if (!updates.length) return snapshot
    const sourceTurns = snapshot.turns || snapshot.timeline?.turns || []
    const turnOwners = new Set(sourceTurns.flatMap(turn => (turn.chatItems || []).map(item => item.id)))
    const apply = (items: AIWorkbenchChatItem[], append: boolean, inTurn = false) => {
      let next = items
      for (const update of updates) {
        const index = next.findIndex(item => item.id === update.blockId)
        if (index >= 0) {
          const old = next[index]
          if (Number(old.metadata?.sequence || 0) > update.sequence) continue
          if (update.item === null) next = next.filter(item => item.id !== update.blockId)
          else if (old !== update.item) next = next.map((item, i) => i === index ? update.item! : item)
        } else if (append && update.item && (!inTurn || !turnOwners.has(update.blockId))) next = [...next, update.item]
      }
      return next
    }
    const chatItems = apply(snapshot.chatItems || [], true)
    const turns = sourceTurns.map((turn, index) => {
      // New blocks belong to the current user turn. Existing blocks keep their turn.
      const eligible = index === sourceTurns.length - 1
      const items = apply(turn.chatItems || [], eligible, true)
      return items === turn.chatItems ? turn : { ...turn, chatItems: items }
    })
    return { ...snapshot, chatItems, turns, timeline: { ...snapshot.timeline, chatItems, turns } }
  }
}

// A single network read can contain hundreds of SSE frames. Yield periodically
// so React and the browser can paint before parsing the rest of a burst.
export async function readAIProcessFrames(
  response: Response,
  onEvent: (type: string, data: Record<string, unknown>) => void,
) {
  if (!response.body) return
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let frames = 0
  const dispatch = (frame: string) => {
    const lines = frame.split(/\r?\n/)
    const type = lines.find(line => line.startsWith('event:'))?.slice(6).trim()
    const data = lines.filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
    if (type && data) onEvent(type, JSON.parse(data))
  }
  try {
    while (true) {
      const { value, done } = await reader.read()
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true })
      let match: RegExpExecArray | null
      while ((match = /\r?\n\r?\n/.exec(buffer))) {
        dispatch(buffer.slice(0, match.index))
        buffer = buffer.slice(match.index + match[0].length)
        if (++frames % 32 === 0) await new Promise(resolve => setTimeout(resolve, 0))
      }
      if (done) { if (buffer.trim()) dispatch(buffer); break }
    }
  } finally {
    try { await reader.cancel() } catch { /* The request may already be aborted. */ }
    reader.releaseLock()
  }
}
