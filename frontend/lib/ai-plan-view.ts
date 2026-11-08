import { isOrchestrationRunId, type AIPlanItem, type AIPlanRun } from './api/ai-plan'

/** Orchestration records wrap the human instruction with asset references. */
export function planItemInstruction(item: Pick<AIPlanItem, 'instruction' | 'title'>): string {
  try {
    const value: unknown = JSON.parse(item.instruction)
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      const payload = value as Record<string, unknown>
      if (typeof payload.summary === 'string' && Object.keys(payload).every(key => key === 'summary' || key === 'asset_ids')) return payload.summary
    }
  } catch { /* Plain-text instructions are already display-ready. */ }
  return item.instruction || item.title
}

/** Display-only excerpt: never changes the instruction sent to a worker. */
export function planItemTitle(item: Pick<AIPlanItem, 'title' | 'instruction'>): string {
  const source = (item.title || planItemInstruction(item)).trim()
  // Remove explanatory parentheses before extracting the action, so a target's
  // technology notes don't become the entire title of an older plan record.
  let concise = source
  for (let depth = 0; depth < 3; depth++) concise = concise.replace(/（[^（）]*）|\([^()]*\)/gu, '')
  const clauses = concise.split(/[：\n；;，]/u).map(value => value.trim()).filter(Boolean)
  const firstClause = clauses[0] || source
  let title = firstClause.replace(/^(?:针对|对)\s*.+?\s*(?:进行|执行|做)/u, '')
  if (title === firstClause) title = title.replace(/^(?:针对|对)\s*.+?(?=实测|测试|从|绕过|核查)/u, '')
  if (title.length < 5 && clauses[1]) title = `${title}：${clauses[1]}`
  title = title.replace(/\s+/g, ' ').trim() || source
  return title.length > 64 ? `${title.slice(0, 64)}…` : title
}

export function planItemTarget(item: Pick<AIPlanItem, 'target' | 'title' | 'instruction'>): string {
  if (item.target.trim()) return item.target.trim()
  // Only recover an explicit leading "对/针对 <target>" phrase. Searching the
  // whole instruction would mislabel tools (crt.sh), versions and files as targets.
  const source = (item.title || planItemInstruction(item)).trim()
  if (!/^(?:针对|对)\s*/u.test(source)) return ''
  let rest = source.replace(/^(?:针对|对)\s*/u, '')
  const targets: string[] = []
  while (rest) {
    const match = rest.match(/^(https?:\/\/[^\s<>"'，。；：、（）()]+|(?:[a-z\d-]+\.)+[a-z]{2,}(?::\d+)?|(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?)/i)
    if (!match) break
    try { targets.push(new URL(match[0].includes('://') ? match[0] : `https://${match[0]}`).host) } catch { break }
    rest = rest.slice(match[0].length)
    const separator = rest.match(/^\s*(?:\/|、|·|和|与)\s*/u)
    if (!separator) break
    rest = rest.slice(separator[0].length)
  }
  return [...new Set(targets)].join(' · ')
}

export function filterPlanItems(items: AIPlanItem[], status: string, query: string): AIPlanItem[] {
  const needle = query.trim().toLocaleLowerCase()
  return items.filter(item => (status === 'all' || item.status === status) && (!needle ||
    [item.seq, item.title, item.target, item.instruction, item.resultSummary, item.error]
      .filter(value => value != null).join(' ').toLocaleLowerCase().includes(needle)))
}

export function canPausePlanRun(run: AIPlanRun): boolean {
  return !run.readOnly && isOrchestrationRunId(run.id) && ['planning', 'waiting'].includes(run.status)
}

export function canResumePlanRun(run: AIPlanRun): boolean {
  return !run.readOnly && isOrchestrationRunId(run.id) && run.status === 'paused'
}
