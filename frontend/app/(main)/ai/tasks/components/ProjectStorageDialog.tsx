'use client'

import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { getAIWorkbenchProjectStorageUsage } from '@/lib/api/ai-workbench'
import { useI18n } from '@/lib/i18n'

function formatDiskBytes(bytes: number, language: string) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const unit = bytes > 0 ? Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1) : 0
  const value = bytes / 1024 ** unit
  return `${value.toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', { maximumFractionDigits: unit === 0 ? 0 : 1 })} ${units[unit]}`
}

export function ProjectStorageDialog({ project, onClose }: { project: { id: string; name: string }; onClose: () => void }) {
  const { t, language } = useI18n()
  const l = t.aiBatch
  const [revision, setRevision] = useState(0)
  const [usage, setUsage] = useState<{ bytes: number | null; error: string; loading: boolean }>({ bytes: null, error: '', loading: true })

  useEffect(() => {
    const controller = new AbortController()
    let active = true
    void getAIWorkbenchProjectStorageUsage(project.id, controller.signal)
      .then(result => { if (active) setUsage({ bytes: result.diskBytes, error: '', loading: false }) })
      .catch(error => { if (active) setUsage({ bytes: null, error: error instanceof Error ? error.message : String(error), loading: false }) })
    return () => { active = false; controller.abort() }
  }, [project.id, revision])

  return <Dialog open onOpenChange={open => { if (!open) onClose() }}>
    <DialogContent className="sm:max-w-md">
      <DialogHeader>
        <DialogTitle>{l.projectStorageTitle}</DialogTitle>
        <DialogDescription className="break-words">{project.name}</DialogDescription>
      </DialogHeader>
      <div className="rounded-lg border bg-muted/20 p-5" aria-live="polite" aria-busy={usage.loading}>
        {usage.loading ? <p role="status" className="text-sm text-muted-foreground">{l.projectStorageLoading}</p>
          : usage.error ? <div role="alert" className="space-y-2 text-sm"><p>{l.projectStorageFailed}</p><p className="break-words text-muted-foreground">{usage.error}</p></div>
            : <p className="text-3xl font-semibold tabular-nums">{l.projectStorageSize.replace('{size}', formatDiskBytes(usage.bytes ?? 0, language))}</p>}
      </div>
      <p className="text-xs leading-5 text-muted-foreground">{l.projectStorageHint}</p>
      <div className="flex justify-end"><Button variant="outline" disabled={usage.loading} onClick={() => { setUsage({ bytes: null, error: '', loading: true }); setRevision(value => value + 1) }}>{usage.error ? l.retry : l.refresh}</Button></div>
    </DialogContent>
  </Dialog>
}
