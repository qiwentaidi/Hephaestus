export type CombinationModuleId = 'subdomain' | 'portscan' | 'webscan' | 'dirscan' | 'jsscan' | 'crack'

export type CombinationTargetInputType = 'domain' | 'network' | 'web' | 'service'

export interface CombinationNodeInput {
  id: string
  moduleId: CombinationModuleId
  stageIndex: number
  laneIndex?: number
  config: Record<string, unknown>
}

export interface CombinationDraftInput {
  name: string
  targets: string[]
  nodes: CombinationNodeInput[]
  scanMode?: 'auto' | 'local' | 'agent'
  agentId?: string
  proxy?: string
}

export interface CombinationPresetDefinition {
  id: string
  stages: CombinationModuleId[][]
}

/** Drafts created from the asset ledger are handed to the full combination
 * editor through sessionStorage so the user can review or tweak them before
 * creating the task. */
export const ASSET_SCAN_COMBINATION_DRAFT_KEY = 'asset-scan-combination-draft'

export const COMBINATION_PRESET_DEFINITIONS: CombinationPresetDefinition[] = [
  {
    id: 'asset-discovery',
    stages: [['subdomain'], ['portscan'], ['webscan']],
  },
  {
    id: 'host-discovery',
    stages: [['portscan'], ['webscan']],
  },
  {
    id: 'credential-attack-chain',
    stages: [['subdomain'], ['portscan'], ['crack']],
  },
  {
    id: 'web-deep-scan',
    stages: [['webscan'], ['jsscan'], ['dirscan']],
  },
  {
    id: 'frontend-attack-surface',
    stages: [['webscan'], ['jsscan']],
  },
  {
    id: 'full-vuln-scan',
    stages: [['subdomain'], ['portscan'], ['crack', 'webscan'], ['jsscan']],
  },
]

const COMBINATION_MODULE_IDS: CombinationModuleId[] = [
  'subdomain',
  'portscan',
  'webscan',
  'dirscan',
  'jsscan',
  'crack',
]

export function isCombinationModuleId(value: string): value is CombinationModuleId {
  return COMBINATION_MODULE_IDS.includes(value as CombinationModuleId)
}

export function combinationModuleTargetInputType(
  moduleId: CombinationModuleId | string,
): CombinationTargetInputType | null {
  switch (String(moduleId).toLowerCase().trim()) {
    case 'subdomain':
      return 'domain'
    case 'portscan':
      return 'network'
    case 'webscan':
    case 'dirscan':
    case 'jsscan':
      return 'web'
    case 'crack':
      return 'service'
    default:
      return null
  }
}

/**
 * Mirrors the backend rule: the input type is decided by the modules that
 * share the earliest stage. Mixed input types in that stage are invalid.
 */
export function combinationTemplateInputTypeFromNodes(
  nodes: Array<Pick<CombinationNodeInput, 'moduleId' | 'stageIndex'>>,
): CombinationTargetInputType | null {
  if (!nodes.length) return null
  const minStage = Math.min(...nodes.map((node) => Math.max(0, Number(node.stageIndex) || 0)))
  const inputTypes = new Set<CombinationTargetInputType>()
  for (const node of nodes) {
    if (Math.max(0, Number(node.stageIndex) || 0) !== minStage) continue
    const inputType = combinationModuleTargetInputType(node.moduleId)
    if (inputType) inputTypes.add(inputType)
  }
  if (inputTypes.size !== 1) return null
  return [...inputTypes][0]
}

export function buildCombinationNodeConfig(moduleId: CombinationModuleId): Record<string, unknown> {
  switch (moduleId) {
    case 'subdomain':
      return { thread: 600, timeout: 5 }
    case 'portscan':
      return {
        threads: 1000,
        timeout: 10,
        aliveDetection: true,
        tcpDiscoveryMode: 'connect',
        portDiscoveryMode: 'connect',
      }
    case 'webscan':
      return {
        threads: 30,
        nucleiThreads: 2,
        webscanOption: 3,
        rootPathScan: true,
        skipNucleiWithoutTags: false,
        skipNucleiWithWAF: true,
        screenshot: true,
        activeTimeoutLimit: 5,
        oobEnabled: true,
      }
    case 'dirscan':
      return {
        method: 'GET',
        exts: ['php', 'aspx', 'asp', 'jsp', 'html', 'js'],
        workers: 25,
        timeout: 8,
        bodyLengthExcludeTimes: 5,
        statusCodeExclude: [404],
        defaultDict: true,
      }
    case 'jsscan':
      return {
        vulnDetection: { enabled: true },
        openai: { enabled: false },
      }
    case 'crack':
      return {
        threads: 10,
        username: '',
        password: '',
        defaultDict: true,
      }
  }
}

export function buildPresetCombinationDraft(
  presetId: string,
  targets: string[],
  name: string,
): CombinationDraftInput | null {
  const preset = COMBINATION_PRESET_DEFINITIONS.find((item) => item.id === presetId)
  if (!preset) return null
  const nodes: CombinationNodeInput[] = []
  preset.stages.forEach((modules, stageIndex) => {
    modules.forEach((moduleId, laneIndex) => {
      nodes.push({
        id: `${preset.id}-stage-${stageIndex}-${laneIndex}-${moduleId}`,
        moduleId,
        stageIndex,
        laneIndex,
        config: buildCombinationNodeConfig(moduleId),
      })
    })
  })
  if (!nodes.length) return null
  return {
    name,
    targets,
    nodes,
    scanMode: 'auto',
  }
}

/**
 * Saved combination templates store a full draft (name, targets, nodes,
 * scanMode). Only the workflow is reused here; the caller supplies the
 * asset-derived targets and name.
 */
export function buildTemplateCombinationDraft(
  templateConfig: unknown,
  targets: string[],
  name: string,
): CombinationDraftInput | null {
  if (!templateConfig || typeof templateConfig !== 'object') return null
  const rawNodes = (templateConfig as { nodes?: unknown }).nodes
  if (!Array.isArray(rawNodes) || rawNodes.length === 0) return null
  const nodes: CombinationNodeInput[] = []
  rawNodes.forEach((rawNode, index) => {
    if (!rawNode || typeof rawNode !== 'object') return
    const node = rawNode as {
      id?: unknown
      moduleId?: unknown
      stageIndex?: unknown
      laneIndex?: unknown
      config?: unknown
    }
    const moduleId = String(node.moduleId || '').toLowerCase().trim()
    if (!isCombinationModuleId(moduleId)) return
    const stageIndex = Number(node.stageIndex)
    const laneIndex = Number(node.laneIndex)
    nodes.push({
      id: typeof node.id === 'string' && node.id.trim() ? node.id.trim() : `${moduleId}-${index}`,
      moduleId,
      stageIndex: Number.isFinite(stageIndex) ? Math.max(0, Math.trunc(stageIndex)) : 0,
      laneIndex: Number.isFinite(laneIndex) ? Math.max(0, Math.trunc(laneIndex)) : 0,
      config: node.config && typeof node.config === 'object' && !Array.isArray(node.config)
        ? { ...(node.config as Record<string, unknown>) }
        : {},
    })
  })
  if (!nodes.length) return null
  return {
    name,
    targets,
    nodes,
    scanMode: 'auto',
  }
}
