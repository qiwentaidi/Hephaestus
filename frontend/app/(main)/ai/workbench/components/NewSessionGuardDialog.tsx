'use client'

import { ArrowRight, ArrowUpRight, Loader2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { formatWorkbenchText } from '@/lib/ai-workbench-i18n'
import { useI18n } from '@/lib/i18n'

import type { NewSessionGuardTarget } from '../active-run-guard'

// Creating a standalone session inside a project that is still orchestrating
// splits the operator's intent two ways: the controller already steers the run,
// while a fresh standalone session ignores its plan. Ask first, once, and route
// the answer explicitly. Terminal runs never reach this dialog.
export function NewSessionGuardDialog({ target, busy, onClose, onContinueInController, onCreateTask }: {
  target: NewSessionGuardTarget
  busy: boolean
  onClose: () => void
  onContinueInController: () => void
  onCreateTask: () => void
}) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const run = target.run.run
  const status = (t.aiPlan.statuses as Record<string, string>)[run.status] || run.status
  const total = target.run.itemTotal || 0
  const done = Number(target.run.itemCounts?.completed || 0)
  const progress = total > 0 ? formatWorkbenchText(l.newSessionGuardProgress, { status, done, total }) : status
  return (
    <Dialog open onOpenChange={(open) => { if (!open && !busy) onClose() }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{l.newSessionGuardTitle}</DialogTitle>
          <DialogDescription>{l.newSessionGuardDescription}</DialogDescription>
        </DialogHeader>
        <div className="rounded-lg border border-border/70 bg-muted/30 px-3 py-2 text-xs">
          <p className="truncate font-medium text-foreground" title={target.projectName}>{target.projectName || l.unnamedProject}</p>
          <p className="mt-0.5 text-muted-foreground">{progress}</p>
        </div>
        <div className="space-y-2">
          <Button type="button" autoFocus disabled={busy} onClick={onContinueInController} className="h-auto w-full justify-between gap-3 whitespace-normal py-2.5 text-left">
            <span className="space-y-0.5">
              <span className="block text-sm font-medium">{l.newSessionGuardPrimary}</span>
              <span className="block text-xs font-normal opacity-85">{l.newSessionGuardPrimaryHint}</span>
            </span>
            {busy ? <Loader2 className="h-4 w-4 shrink-0 animate-spin" /> : <ArrowRight className="h-4 w-4 shrink-0" />}
          </Button>
          <Button type="button" variant="outline" disabled={busy} onClick={onCreateTask} className="h-auto w-full justify-between gap-3 whitespace-normal py-2.5 text-left">
            <span className="space-y-0.5">
              <span className="block text-sm font-medium">{l.newSessionGuardSecondary}</span>
              <span className="block text-xs font-normal text-muted-foreground">{l.newSessionGuardSecondaryHint}</span>
            </span>
            <ArrowUpRight className="h-4 w-4 shrink-0" />
          </Button>
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" disabled={busy} onClick={onClose}>{l.cancel}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
