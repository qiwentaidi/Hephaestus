'use client'

import { AlertTriangle, MessageSquareReply, RotateCw, SquarePen, Trash2 } from 'lucide-react'
import { useI18n } from '@/lib/i18n'
import { formatWorkbenchText } from '@/lib/ai-workbench-i18n'
import { Button } from '@/components/ui/button'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { AssetReferenceChips } from './AssetReferencePicker'
import type { AssetReferenceSelection } from '@/lib/api/ai-asset-references'

export type ComposerQueuedMessage = {
  id: string
  content: string
  attachments: { id: string; name?: string }[]
  assetReferences?: AssetReferenceSelection[]
  status?: 'queued' | 'failed' | 'paused'
}

type ComposerQueuedMessagesProps = {
  messages: ComposerQueuedMessage[]
  onEdit: (id: string) => void
  onRemove: (id: string) => void
  onRetry: (id: string) => void
}

/**
 * Pending follow-ups shown as a compose strip tucked behind the composer,
 * similar to Codex's queued-message preview.
 */
export function ComposerQueuedMessages({ messages, onEdit, onRemove, onRetry }: ComposerQueuedMessagesProps) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  if (!messages.length) return null
  return <div
    data-slot="ai-workbench-composer-queue"
    role="list"
    aria-label={formatWorkbenchText(l.queuedMessage, { count: messages.length })}
    className="max-h-44 overflow-y-auto overscroll-contain divide-y divide-[var(--input-panel-action-border)]"
  >
    {messages.map((queued, index) => {
      const failed = queued.status === 'failed'
      const paused = queued.status === 'paused'
      const metaVisible = Boolean(failed || paused || queued.attachments.length || queued.assetReferences?.length)
      return <div
        key={queued.id}
        role="listitem"
        data-slot="ai-workbench-composer-queue-item"
        data-status={queued.status || 'queued'}
        className="group flex items-center gap-3 px-3 py-2.5 md:px-4"
      >
        <MessageSquareReply
          aria-hidden="true"
          className={cn('h-4 w-4 shrink-0', failed ? 'text-destructive' : paused ? 'text-tone-amber' : 'text-muted-foreground/70')}
        />
        <div className="min-w-0 flex-1">
          <p
            className={cn(
              'line-clamp-2 whitespace-pre-wrap break-words text-sm leading-5',
              paused ? 'text-muted-foreground' : 'text-foreground',
            )}
            title={queued.content}
          >
            {queued.content || (queued.attachments.length ? (queued.attachments[0].name || l.attachmentHint) : '')}
          </p>
          {metaVisible ? <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] leading-4 text-muted-foreground">
            {failed ? <span className="inline-flex items-center gap-1 text-destructive"><AlertTriangle aria-hidden="true" className="h-3 w-3" />{l.queueFailed}</span> : paused ? <span className="inline-flex items-center gap-1 text-tone-amber">{l.queuePaused}</span> : null}
            {queued.attachments.length ? <span>{formatWorkbenchText(l.attachmentCount, { count: queued.attachments.length })}</span> : null}
          </div> : null}
          {queued.assetReferences?.length ? <div className="mt-1.5"><AssetReferenceChips compact references={queued.assetReferences} /></div> : null}
        </div>
        <div className="-mr-1 flex shrink-0 items-center gap-0.5 text-muted-foreground">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className="h-9 w-9 rounded-lg text-muted-foreground hover:bg-[var(--input-panel-action-bg-hover)] hover:text-foreground md:h-8 md:w-8"
                aria-label={formatWorkbenchText(l.editQueuedMessage, { count: index + 1 })}
                onClick={() => onEdit(queued.id)}
              >
                <SquarePen aria-hidden="true" className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{l.editQueuedMessageShort}</TooltipContent>
          </Tooltip>
          {failed || paused ? <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className="h-9 w-9 rounded-lg text-tone-amber hover:bg-[var(--input-panel-action-bg-hover)] hover:text-foreground md:h-8 md:w-8"
                aria-label={paused ? l.resumeQueued : l.retry}
                onClick={() => onRetry(queued.id)}
              >
                <RotateCw aria-hidden="true" className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{paused ? l.resumeQueued : l.retry}</TooltipContent>
          </Tooltip> : null}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                size="icon"
                variant="ghost"
                className="h-9 w-9 rounded-lg text-muted-foreground hover:bg-[var(--input-panel-action-bg-hover)] hover:text-foreground md:h-8 md:w-8"
                aria-label={formatWorkbenchText(l.removeQueuedMessage, { count: index + 1 })}
                onClick={() => onRemove(queued.id)}
              >
                <Trash2 aria-hidden="true" className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{l.removeQueuedMessageShort}</TooltipContent>
          </Tooltip>
        </div>
      </div>
    })}
  </div>
}
