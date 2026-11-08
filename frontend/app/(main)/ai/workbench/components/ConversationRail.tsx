'use client'

import { useI18n } from '@/lib/i18n'
import { formatWorkbenchText } from '@/lib/ai-workbench-i18n'

import { memo, useCallback, useEffect, useRef, type KeyboardEvent } from 'react'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

// loaded === false marks a round the conversation has not fetched yet; the rail
// renders it shorter and clicking it loads the surrounding chunk first.
export type ConversationRailEntry = { id: string; question: string; summary: string; number?: number; loaded?: boolean; busy?: boolean }

export const ConversationRail = memo(function ConversationRail({ entries, activeId, bottomInset, onNavigate }: {
  entries: ConversationRailEntry[]
  activeId: string | null
  bottomInset: number
  onNavigate: (id: string) => void
}) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  const listRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const list = listRef.current
    const marker = list?.querySelector<HTMLElement>('[aria-current="step"]')
    if (!list || !marker) return
    // Only scroll the rail; scrollIntoView would also move the conversation.
    const top = marker.offsetTop
    if (top < list.scrollTop) list.scrollTop = top
    else if (top + marker.offsetHeight > list.scrollTop + list.clientHeight) {
      list.scrollTop = top + marker.offsetHeight - list.clientHeight
    }
  }, [activeId, entries.length, bottomInset])

  const handleKeyDown = useCallback((event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let next = index
    if (event.key === 'ArrowDown' || event.key === 'ArrowRight') next++
    else if (event.key === 'ArrowUp' || event.key === 'ArrowLeft') next--
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = entries.length - 1
    else return
    event.preventDefault()
    const markers = listRef.current?.querySelectorAll<HTMLButtonElement>('button')
    markers?.[Math.max(0, Math.min(entries.length - 1, next))]?.focus({ preventScroll: true })
    const marker = markers?.[Math.max(0, Math.min(entries.length - 1, next))]
    const list = listRef.current
    if (marker && list) {
      if (marker.offsetTop < list.scrollTop) list.scrollTop = marker.offsetTop
      else if (marker.offsetTop + marker.offsetHeight > list.scrollTop + list.clientHeight) list.scrollTop = marker.offsetTop + marker.offsetHeight - list.clientHeight
    }
  }, [entries.length])

  if (!entries.length) return null
  return (
    <nav
      aria-label={l.turnNavigation}
      data-slot="ai-conversation-rail"
      className="pointer-events-none absolute left-2 top-4 z-20 flex w-9 items-center sm:left-4"
      style={{ bottom: bottomInset + 16 }}
    >
      <div ref={listRef} className="pointer-events-auto relative flex max-h-full w-full flex-col overflow-y-auto overflow-x-hidden overscroll-contain py-2 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden" style={{ maxHeight: 'min(100%, 336px)' }}>
        {entries.map((entry, index) => <ConversationRailMarker key={entry.id} entry={entry} index={index} isActive={activeId === entry.id} onNavigate={onNavigate} onKeyDown={handleKeyDown} />)}
      </div>
    </nav>
  )
})

// Scrolling changes two ticks, not every Tooltip in a long conversation.
const ConversationRailMarker = memo(function ConversationRailMarker({ entry, index, isActive, onNavigate, onKeyDown }: {
  entry: ConversationRailEntry
  index: number
  isActive: boolean
  onNavigate: (id: string) => void
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>, index: number) => void
}) {
  const { t } = useI18n()
  const l = t.aiWorkbench
  return (
          <Tooltip delayDuration={100}>
            <TooltipTrigger asChild>
              <button
                type="button"
                data-ai-turn-marker={entry.id}
                aria-label={formatWorkbenchText(l.turnNavigationLabel, { count: entry.number ?? index + 1, question: entry.question })}
                aria-busy={entry.busy ? 'true' : undefined}
                data-loaded={entry.loaded === false ? 'false' : 'true'}
                aria-current={isActive ? 'step' : undefined}
                onClick={() => onNavigate(entry.id)}
                onKeyDown={(event) => onKeyDown(event, index)}
                className="group/turn-marker flex h-[10px] min-h-[10px] w-9 shrink-0 items-center px-1 outline-none focus-visible:rounded-sm focus-visible:ring-1 focus-visible:ring-inset focus-visible:ring-foreground/60 [@media(pointer:coarse)]:h-7 [@media(pointer:coarse)]:min-h-7"
              >
                {/* Three tick lengths (DSH-style): unloaded 8px, loaded 12px, focused 20px. */}
                <span className={cn(
                  'h-[3px] rounded-full transition-[width,background-color,opacity] duration-150 motion-reduce:transition-none',
                  isActive
                    ? 'w-5 bg-foreground'
                    : entry.loaded === false
                      ? 'w-2 bg-muted-foreground/30 opacity-70 group-hover/turn-marker:w-[22px] group-hover/turn-marker:bg-foreground group-hover/turn-marker:opacity-100'
                      : 'w-3 bg-muted-foreground/45 group-hover/turn-marker:w-[22px] group-hover/turn-marker:bg-foreground',
                  entry.busy ? 'animate-pulse' : '',
                )} />
              </button>
            </TooltipTrigger>
            <TooltipContent
              side="right"
              sideOffset={12}
              collisionPadding={16}
              className="w-[520px] max-w-[calc(100vw-80px)] rounded-[18px] border-border/70 bg-popover px-[18px] py-4 text-left text-pretty text-popover-foreground shadow-[0_14px_38px_rgba(0,0,0,0.16)] [&_svg]:!hidden"
            >
              <div data-slot="ai-turn-preview">
                <p className="truncate text-[15px] font-semibold leading-6">{entry.question}</p>
                <p className="mt-1.5 line-clamp-2 break-words text-sm font-medium leading-[1.55] text-muted-foreground">{entry.summary}</p>
              </div>
            </TooltipContent>
          </Tooltip>
  )
})
