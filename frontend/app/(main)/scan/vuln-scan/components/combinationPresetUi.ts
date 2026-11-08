import { Captions, Code2, GitBranch, Globe2, KeyRound, Server } from 'lucide-react'

import {
  COMBINATION_PRESET_DEFINITIONS,
  type CombinationModuleId,
} from '@/lib/combination-presets'

export interface CombinationPresetCard {
  id: string
  title: string
  description: string
  icon: typeof Server
  tone: string
  stages: Array<{ title: string; modules: CombinationModuleId[] }>
}

interface PresetVisual {
  icon: typeof Server
  tone: string
  titleKey: string
  descriptionKey: string
}

const PRESET_VISUALS: Record<string, PresetVisual> = {
  'asset-discovery': {
    icon: Captions,
    tone: 'text-emerald-500',
    titleKey: 'assetDiscoveryTitle',
    descriptionKey: 'assetDiscoveryDesc',
  },
  'host-discovery': {
    icon: Server,
    tone: 'text-cyan-500',
    titleKey: 'hostDiscoveryTitle',
    descriptionKey: 'hostDiscoveryDesc',
  },
  'credential-attack-chain': {
    icon: KeyRound,
    tone: 'text-rose-500',
    titleKey: 'credentialAttackTitle',
    descriptionKey: 'credentialAttackDesc',
  },
  'web-deep-scan': {
    icon: Globe2,
    tone: 'text-blue-500',
    titleKey: 'webDeepScanTitle',
    descriptionKey: 'webDeepScanDesc',
  },
  'frontend-attack-surface': {
    icon: Code2,
    tone: 'text-amber-500',
    titleKey: 'frontendAttackSurfaceTitle',
    descriptionKey: 'frontendAttackSurfaceDesc',
  },
  'full-vuln-scan': {
    icon: GitBranch,
    tone: 'text-violet-500',
    titleKey: 'fullVulnScanTitle',
    descriptionKey: 'fullVulnScanDesc',
  },
}

const FALLBACK_VISUAL: PresetVisual = {
  icon: Server,
  tone: 'text-muted-foreground',
  titleKey: '',
  descriptionKey: '',
}

export function buildCombinationPresetCards(
  t: any,
  formatText: (template: string, values: Record<string, string | number>) => string,
): CombinationPresetCard[] {
  const presets = t.vulnScan.combination.inline.presets
  return COMBINATION_PRESET_DEFINITIONS.map((definition) => {
    const visual = PRESET_VISUALS[definition.id] || FALLBACK_VISUAL
    return {
      id: definition.id,
      title: visual.titleKey ? presets[visual.titleKey] : definition.id,
      description: visual.descriptionKey ? presets[visual.descriptionKey] : '',
      icon: visual.icon,
      tone: visual.tone,
      stages: definition.stages.map((modules, index) => ({
        title: formatText(t.vulnScan.combination.inline.stageName, { n: index + 1 }),
        modules,
      })),
    }
  })
}
