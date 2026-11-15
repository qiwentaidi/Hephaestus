'use client'

import { Badge } from '@/components/ui/badge'
import { useI18n } from '@/lib/i18n'

export function SourceBadges({ sources }: { sources: string[] }) {
  const { t } = useI18n()
  const text = t.assetLedger
  if (!sources.length) return <span className="text-muted-foreground">—</span>
  const label = (source: string) => {
    switch (source) {
      case 'manual':
        return text.sourceManual
      case 'import':
        return text.sourceImport
      case 'portscan':
        return text.sourcePortscan
      case 'fingerprint':
      case 'webscan':
        return text.sourceFingerprint
      case 'subdomain':
        return text.sourceSubdomain
      case 'dirscan':
        return text.sourceDirscan
      case 'jsscan':
        return text.sourceJsscan
      case 'jsscan-vulnerability':
        return text.sourceJsscanVulnerability
      case 'webscan-vulnerability':
        return text.sourceVulnerability
      case 'findings':
        return text.sourceFindings
      case 'ai':
        return text.sourceAI
      case 'fofa':
        return text.sourceFofa
      case 'hunter':
        return text.sourceHunter
      case 'quake':
        return text.sourceQuake
      case 'uncover':
        return text.sourceUncover
      default:
        return source
    }
  }
  return (
    <div className="flex flex-wrap items-center gap-1">
      {sources.slice(0, 3).map((source) => (
        <Badge key={source} variant="secondary" className="font-normal">
          {label(source)}
        </Badge>
      ))}
      {sources.length > 3 ? (
        <Badge variant="outline" className="font-normal" title={sources.slice(2).map(label).join(', ')}>
          +{sources.length - 3}
        </Badge>
      ) : null}
    </div>
  )
}
