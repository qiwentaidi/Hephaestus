import type { AIWorkbenchConversationTurn } from '@/lib/api/ai-workbench'

// New snapshots replace overlapping bodies, while already fetched history remains
// available. The authoritative index also orders same-timestamp rounds correctly.
export function mergeConversationTurns(
  cached: AIWorkbenchConversationTurn[],
  incoming: AIWorkbenchConversationTurn[],
  index: ReadonlyArray<{ id: string }>,
): AIWorkbenchConversationTurn[] {
  const byId = new Map(cached.map(turn => [turn.id, turn]))
  for (const turn of incoming) byId.set(turn.id, turn)
  if (index.length) return index.flatMap(entry => {
    const turn = byId.get(entry.id)
    return turn ? [turn] : []
  })
  return [...byId.values()]
}

export function readingTurnId(
  items: ReadonlyArray<{ index: number; start: number }>,
  rows: ReadonlyArray<{ key: string; kind: string }>,
  readingOffset: number,
  atBottom: boolean,
): string | null {
  const candidates = items.filter(item => rows[item.index]?.kind === 'turn' || rows[item.index]?.kind === 'legacy')
  if (!candidates.length) return null
  let current = candidates[0]
  for (const item of candidates) {
    if (!atBottom && item.start > readingOffset) break
    current = item
  }
  return rows[current.index].key
}
