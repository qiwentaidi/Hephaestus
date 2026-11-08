export const MAX_AGENT_PROMPT_BYTES = 128 * 1024

export interface PromptMetrics {
  chars: number
  bytes: number
  tokens: number
}

/**
 * Cheap character/token estimate for the editor status bar. The byte count is
 * authoritative because the backend rejects prompts above 128 KiB.
 */
export function measurePrompt(text: string): PromptMetrics {
  const cjk = (text.match(/[\u3400-\u9fff\uf900-\ufaff]/g) || []).length
  const chars = text.length
  const bytes = typeof TextEncoder === 'undefined' ? chars : new TextEncoder().encode(text).length
  const tokens = Math.ceil(cjk * 1.15 + (chars - cjk) / 4)
  return { chars, bytes, tokens }
}

export interface PromptDiffStats {
  added: number
  removed: number
}

/**
 * Line-level diff stats against the built-in default. Prompts are small, so a
 * rolling-row LCS is enough; very large inputs fall back to a multiset count.
 */
export function diffPromptLines(before: string, after: string): PromptDiffStats {
  const a = before.split('\n')
  const b = after.split('\n')
  if (a.length * b.length > 4_000_000) {
    return multisetDiff(a, b)
  }

  let previous = new Uint32Array(b.length + 1)
  let current = new Uint32Array(b.length + 1)
  for (let i = a.length - 1; i >= 0; i -= 1) {
    current.fill(0)
    for (let j = b.length - 1; j >= 0; j -= 1) {
      current[j] = a[i] === b[j]
        ? previous[j + 1] + 1
        : Math.max(previous[j], current[j + 1])
    }
    const swap = previous
    previous = current
    current = swap
  }
  const common = previous[0]
  return { added: b.length - common, removed: a.length - common }
}

function multisetDiff(a: string[], b: string[]): PromptDiffStats {
  const counts = new Map<string, number>()
  for (const line of a) {
    counts.set(line, (counts.get(line) || 0) + 1)
  }
  let common = 0
  for (const line of b) {
    const remaining = counts.get(line) || 0
    if (remaining > 0) {
      counts.set(line, remaining - 1)
      common += 1
    }
  }
  return { added: b.length - common, removed: a.length - common }
}
