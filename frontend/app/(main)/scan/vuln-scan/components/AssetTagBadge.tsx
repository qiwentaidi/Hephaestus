'use client'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { Cloud, Globe2, ShieldAlert, Tags } from 'lucide-react'
import { forwardRef } from 'react'

type AssetTagLike = {
  productName?: string
  assetType?: string
  source?: string
}

export function normalizeAssetTag(raw: unknown): AssetTagLike | null {
  if (!raw) return null

  const parsed = typeof raw === 'string' ? safeParseAssetTag(raw) : raw
  if (!parsed || typeof parsed !== 'object') {
    return null
  }

  const normalized = {
    productName: (parsed as any).productName ?? (parsed as any).ProductName,
    assetType: normalizeAssetType((parsed as any).assetType ?? (parsed as any).AssetType),
    source: (parsed as any).source ?? (parsed as any).Source,
  }

  if (!normalized.productName && !normalized.assetType && !normalized.source) {
    return null
  }

  return normalized
}

function safeParseAssetTag(raw: string): unknown {
  try {
    return JSON.parse(raw)
  } catch {
    return null
  }
}

function normalizeAssetType(assetType?: string): string {
  if (!assetType) return ''
  const normalized = assetType.trim().toUpperCase()
  if (!normalized) return ''
  if (normalized === 'COMMON') return 'CDN'
  if (normalized.includes('WAF')) return 'WAF'
  if (normalized.includes('CLOUD')) return 'CLOUD'
  if (normalized.includes('CDN')) return 'CDN'
  return normalized
}

function getAssetTypeStyle(assetType?: string) {
  switch (normalizeAssetType(assetType)) {
    case 'WAF':
      return {
        icon: ShieldAlert,
        className: 'border-rose-500/25 bg-gradient-to-r from-rose-500/10 via-rose-500/5 to-transparent text-rose-300 hover:from-rose-500/15 hover:via-rose-500/10',
      }
    case 'CLOUD':
      return {
        icon: Cloud,
        className: 'border-sky-500/25 bg-gradient-to-r from-sky-500/10 via-sky-500/5 to-transparent text-sky-300 hover:from-sky-500/15 hover:via-sky-500/10',
      }
    case 'CDN':
      return {
        icon: Globe2,
        className: 'border-amber-500/25 bg-gradient-to-r from-amber-500/10 via-amber-500/5 to-transparent text-amber-300 hover:from-amber-500/15 hover:via-amber-500/10',
      }
    default:
      return {
        icon: Tags,
        className: 'border-border/70 bg-gradient-to-r from-muted/70 via-muted/40 to-transparent text-foreground/85 hover:from-muted hover:via-muted/60',
      }
  }
}

type AssetTagBadgeProps = React.ComponentPropsWithoutRef<typeof Badge> & {
  label: string
  assetType?: string
}

export const AssetTagBadge = forwardRef<HTMLDivElement, AssetTagBadgeProps>(
  ({ label, assetType, className, children, ...props }, ref) => {
    const style = getAssetTypeStyle(assetType)
    const Icon = style.icon
    const normalizedAssetType = normalizeAssetType(assetType).toLowerCase() || 'default'

    return (
      <Badge
        ref={ref}
        variant="outline"
        data-asset-tag-badge="true"
        data-asset-type={normalizedAssetType}
        className={cn(
          'gap-1.5 rounded-md border px-2 py-0.5 font-medium shadow-sm backdrop-blur-sm transition-colors',
          style.className,
          className
        )}
        {...props}
      >
        <Icon className="h-3.5 w-3.5" />
        <span>{label}</span>
        {children}
      </Badge>
    )
  }
)

AssetTagBadge.displayName = 'AssetTagBadge'
