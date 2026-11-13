'use client'

import { useLayoutEffect, useRef, useState } from 'react'
import { ChevronDown, X } from 'lucide-react'
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent,
  DropdownMenuCheckboxItem, DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

const TAG_GAP = 4
const TAG_CLASS = 'inline-flex min-w-0 max-w-[10rem] items-center gap-0.5 rounded-md bg-muted px-1.5 py-0.5 text-xs text-foreground'
const TAG_TEXT_CLASS = 'min-w-0 truncate'
const TAG_REMOVE_CLASS = '-mr-0.5 inline-flex size-3.5 shrink-0 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted-foreground/20 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50'
const OVERFLOW_CLASS = 'inline-flex shrink-0 items-center rounded-md bg-muted px-1.5 py-0.5 text-xs text-muted-foreground'

function formatCount(template: string, count: number) {
  return template.replace(/\{count\}/g, String(count))
}

export function MultiSelectFilter<T extends string>({ values, options, allLabel, countLabel, ariaLabel, removeLabel = 'Remove', clearLabel = 'Clear', disabled, onChange, className, contentClassName }: {
  values: T[]
  options: { value: T; label: string }[]
  allLabel: string
  countLabel: string
  ariaLabel: string
  removeLabel?: string
  clearLabel?: string
  disabled?: boolean
  onChange: (values: T[]) => void
  className?: string
  contentClassName?: string
}) {
  const [open, setOpen] = useState(false)
  const [hovered, setHovered] = useState(false)
  const [visibleCount, setVisibleCount] = useState(values.length)
  const triggerRef = useRef<HTMLDivElement>(null)
  const contentRef = useRef<HTMLDivElement>(null)
  const measureRef = useRef<HTMLDivElement>(null)
  const selectedOptions = options.filter(option => values.includes(option.value))
  const overflowCount = selectedOptions.length - Math.min(visibleCount, selectedOptions.length)
  const selectionLabel = selectedOptions.map(option => option.label).join(' / ')

  // Measure the rendered tags off-screen so the trigger can keep as many as fit
  // and collapse the rest into a +N chip.
  const measurementKey = `${values.join('\u0000')}|${options.map(option => `${option.value}:${option.label}`).join('\u0000')}`
  useLayoutEffect(() => {
    const content = contentRef.current
    const measure = measureRef.current
    if (!content || !measure) return

    const update = () => {
      const available = content.clientWidth
      if (available <= 0) return
      const tagWidths = Array.from(measure.querySelectorAll<HTMLElement>('[data-measure-tag]')).map(tag => tag.offsetWidth)
      const overflowWidth = measure.querySelector<HTMLElement>('[data-measure-overflow]')?.offsetWidth ?? 0

      const fit = (limit: number) => {
        let count = 0
        let used = 0
        for (const width of tagWidths) {
          const next = used + (count === 0 ? 0 : TAG_GAP) + width
          if (next > limit) break
          used = next
          count += 1
        }
        return count
      }

      let nextVisible = fit(available)
      if (nextVisible < tagWidths.length && overflowWidth > 0) {
        nextVisible = fit(available - TAG_GAP - overflowWidth)
      }
      // Never hide the first selected tag entirely: let it shrink and truncate
      // instead, so narrow filters still show what is selected.
      if (tagWidths.length > 0 && nextVisible === 0) {
        nextVisible = 1
      }
      setVisibleCount(previous => (previous === nextVisible ? previous : nextVisible))
    }

    update()
    const observer = new ResizeObserver(update)
    observer.observe(content)
    observer.observe(measure)
    return () => observer.disconnect()
  }, [measurementKey])

  const removeValue = (value: T, restoreFocus: boolean) => {
    onChange(values.filter(item => item !== value))
    if (restoreFocus) requestAnimationFrame(() => triggerRef.current?.focus())
  }

  const triggerLabel = values.length
    ? formatCount(countLabel, values.length)
    : allLabel

  return <DropdownMenu open={disabled ? false : open} onOpenChange={next => { if (!disabled) setOpen(next) }}>
    <DropdownMenuTrigger asChild>
      <div
        ref={triggerRef}
        role="button"
        tabIndex={disabled ? -1 : 0}
        aria-disabled={disabled || undefined}
        aria-label={`${ariaLabel}: ${triggerLabel}`}
        title={selectedOptions.length ? selectionLabel : allLabel}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        className={cn(
          'relative inline-flex min-h-9 min-w-0 cursor-pointer overflow-hidden items-center justify-between gap-1 rounded-md border border-border/80 bg-background px-2 py-1 text-sm font-normal text-foreground/90 shadow-xs outline-none transition-all hover:border-primary/50 hover:bg-primary/10 hover:text-primary focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 dark:border-border/80 dark:bg-input/40 dark:text-foreground/90 dark:hover:border-primary/50 dark:hover:bg-primary/10 dark:hover:text-primary',
          disabled && 'pointer-events-none opacity-50',
          className,
        )}
      >
        <div ref={contentRef} className="flex min-w-0 flex-1 items-center gap-1 overflow-hidden">
          {selectedOptions.length === 0 ? (
            <span className="truncate text-muted-foreground">{allLabel}</span>
          ) : (
            <>
              {selectedOptions.slice(0, Math.min(visibleCount, selectedOptions.length)).map(option => (
                <span key={option.value} className={cn(TAG_CLASS, 'group/tag')} title={option.label}>
                  <span className={TAG_TEXT_CLASS}>{option.label}</span>
                  <button
                    type="button"
                    aria-label={`${removeLabel}: ${option.label}`}
                    className={cn(TAG_REMOVE_CLASS, 'opacity-70 group-hover/tag:opacity-100')}
                    onPointerDown={event => event.stopPropagation()}
                    onClick={event => {
                      event.stopPropagation()
                      removeValue(option.value, event.detail === 0)
                    }}
                    onKeyDown={event => event.stopPropagation()}
                  >
                    <X className="size-3" />
                  </button>
                </span>
              ))}
              {overflowCount > 0 ? (
                <span
                  className={OVERFLOW_CLASS}
                  title={selectedOptions.slice(Math.min(visibleCount, selectedOptions.length)).map(option => option.label).join(' / ')}
                >
                  +{overflowCount}
                </span>
              ) : null}
            </>
          )}
        </div>
        <div className="flex size-4 shrink-0 items-center justify-center">
          {hovered && !disabled && values.length > 1 ? (
            <button
              type="button"
              aria-label={clearLabel}
              title={clearLabel}
              className="inline-flex size-4 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted-foreground/20 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
              onPointerDown={event => event.stopPropagation()}
              onClick={event => {
                event.stopPropagation()
                onChange([])
                if (event.detail === 0) requestAnimationFrame(() => triggerRef.current?.focus())
              }}
              onKeyDown={event => event.stopPropagation()}
            >
              <X className="size-3.5" />
            </button>
          ) : (
            <ChevronDown className="size-4 text-muted-foreground opacity-50" />
          )}
        </div>

        <div
          ref={measureRef}
          aria-hidden="true"
          className="pointer-events-none invisible absolute left-0 top-0 flex w-max items-center gap-1 whitespace-nowrap"
        >
          {selectedOptions.map(option => (
            <span key={option.value} data-measure-tag className={TAG_CLASS}>
              <span className={TAG_TEXT_CLASS}>{option.label}</span>
              <span className={TAG_REMOVE_CLASS}>
                <X className="size-3" />
              </span>
            </span>
          ))}
          <span data-measure-overflow className={OVERFLOW_CLASS}>+{selectedOptions.length}</span>
        </div>
      </div>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="start" className={cn('min-w-44', contentClassName)}>
      <DropdownMenuCheckboxItem disabled={disabled} checked={values.length === 0} onCheckedChange={() => onChange([])} onSelect={event => event.preventDefault()}>
        {allLabel}
      </DropdownMenuCheckboxItem>
      <DropdownMenuSeparator />
      {options.map(option => <DropdownMenuCheckboxItem key={option.value} disabled={disabled} checked={values.includes(option.value)} onCheckedChange={checked => onChange(checked ? [...values, option.value] : values.filter(value => value !== option.value))} onSelect={event => event.preventDefault()}>
        {option.label}
      </DropdownMenuCheckboxItem>)}
    </DropdownMenuContent>
  </DropdownMenu>
}
