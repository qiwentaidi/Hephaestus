'use client'

import { useCallback, useState } from 'react'
import Image from 'next/image'
import { ImageIcon, ImageOff, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { getApiBaseURL } from '@/lib/env'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

interface IconAggregate {
  iconHash: string
  favicon: string
  count: number
}

interface FingerprintIconAggregationProps {
  taskId?: string
  version?: number
  onSelectHash: (iconHash: string) => void
}

function normalizeAssetFilename(value: string) {
  const normalized = value
    .replace(/\\/g, '/')
    .replace(/^data\/favicon\//, '')
    .replace(/^favicon\//, '')
    .replace(/^\/+/, '')
  const segments = normalized.split('/').filter(Boolean)
  return segments.length > 0 ? segments[segments.length - 1] : ''
}

function faviconURL(favicon: string) {
  if (!favicon) return ''
  if (favicon.startsWith('data:image') || favicon.startsWith('http://') || favicon.startsWith('https://')) return favicon
  const filename = normalizeAssetFilename(favicon)
  return filename ? `${getApiBaseURL()}/api/v1/webscan/favicon/${filename}` : ''
}

function formatOccurrenceCount(count: number) {
  return count > 99 ? '99+' : String(count)
}

export function FingerprintIconAggregation({ taskId, version, onSelectHash }: FingerprintIconAggregationProps) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [icons, setIcons] = useState<IconAggregate[]>([])
  const [loading, setLoading] = useState(false)

  const loadIcons = useCallback(async () => {
    if (!taskId) return
    setLoading(true)
    try {
      const versionParam = version ? `?version=${version}` : ''
      const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/fingerprint-icons${versionParam}`)
      const result = await response.json()
      setIcons(response.ok && result.success ? result.data?.icons || [] : [])
    } catch {
      setIcons([])
    } finally {
      setLoading(false)
    }
  }, [taskId, version])

  const handleOpenChange = (nextOpen: boolean) => {
    setOpen(nextOpen)
    if (nextOpen) void loadIcons()
  }

  return (
    <HoverCard open={open} onOpenChange={handleOpenChange} openDelay={180} closeDelay={120}>
      <HoverCardTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t.vulnScan.fingerprint.iconAggregation}
          disabled={!taskId}
          className="shrink-0"
        >
          <ImageIcon className="size-4" />
        </Button>
      </HoverCardTrigger>
      <HoverCardContent align="start" className="w-80 p-2">
        <div className="px-2 py-1.5">
          <p className="text-sm font-medium">{t.vulnScan.fingerprint.iconAggregation}</p>
        </div>
        {loading ? <div className="flex h-28 items-center justify-center"><Loader2 className="size-4 animate-spin text-muted-foreground" /></div> : icons.length === 0 ? (
          <div className="flex h-28 flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageOff className="size-5" />{t.vulnScan.fingerprint.noIconAggregation}</div>
        ) : <div className="max-h-96 space-y-1 overflow-y-auto px-1 pb-1">
          {icons.map((icon) => <button
            key={icon.iconHash}
            type="button"
            onClick={() => onSelectHash(icon.iconHash)}
            className="flex w-full items-center gap-3 rounded-md px-2 py-2 text-left transition-colors hover:bg-muted"
          >
            <span className="relative flex size-8 shrink-0 items-center justify-center">
              <span className="flex size-full items-center justify-center overflow-hidden rounded border border-border bg-muted/40">
                {faviconURL(icon.favicon) ? <Image src={faviconURL(icon.favicon)} alt="" width={20} height={20} unoptimized className="size-5 object-contain" /> : <ImageOff className="size-4 text-muted-foreground" />}
              </span>
              <span
                className="absolute -right-1.5 -top-1 flex size-4 items-center justify-center rounded-full bg-primary px-0.5 text-[9px] font-semibold leading-none text-primary-foreground shadow-sm"
                aria-label={t.vulnScan.fingerprint.iconOccurrence.replace('{count}', String(icon.count))}
              >
                {formatOccurrenceCount(icon.count)}
              </span>
            </span>
            <span className="min-w-0 flex-1"><span className="block truncate font-mono text-xs">{icon.iconHash}</span></span>
          </button>)}
        </div>}
      </HoverCardContent>
    </HoverCard>
  )
}
