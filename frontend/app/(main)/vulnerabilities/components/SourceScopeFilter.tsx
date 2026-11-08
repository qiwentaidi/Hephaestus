'use client'

import { useEffect, useState } from 'react'
import { Check, ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  listCenterScopes,
  type CenterScope,
  type FindingSource,
} from '@/lib/api/vulnerability-center'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

type ScopeOption = CenterScope & { parent?: CenterScope }

export interface ScopeSelection {
  source: FindingSource
  id: string
  name: string
  childId?: string
  childName?: string
}
function ScopeOptions({
  source,
  parentId = '',
  onSelect,
  selection,
}: {
  source: string
  parentId?: string
  onSelect: (item: ScopeOption) => void
  selection?: ScopeSelection | null
}) {
  const { t } = useI18n()
  const l = t.vulnerabilityManagement
  const [items, setItems] = useState<CenterScope[]>([])
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  const [expanded, setExpanded] = useState(() =>
    selection ? `${selection.source}:${selection.id}` : '',
  )
  useEffect(() => {
    const controller = new AbortController()
    const timer = setTimeout(() => {
      setLoading(true)
      setError('')
      void listCenterScopes(source, parentId, '', page, controller.signal)
        .then((result) => {
          if (controller.signal.aborted) return
          setItems((previous) =>
            page === 1
              ? result.items
              : [
                  ...previous,
                  ...result.items.filter(
                    (item) =>
                      !previous.some(
                        (p) => p.source === item.source && p.id === item.id,
                      ),
                  ),
                ],
          )
          setHasMore(result.hasMore)
        })
        .catch((e) => {
          if (!controller.signal.aborted)
            setError(e instanceof Error ? e.message : l.loadProjectsFailed)
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false)
        })
    }, 150)
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
  }, [source, parentId, page, retry, l.loadProjectsFailed])
  return (
    <div className="space-y-0.5">
      {items.map((item) => {
        const key = `${item.source}:${item.id}`
        const expandable =
          !parentId && (item.source === 'ai' || item.childCount > 1)
        const selected =
          selection?.source === item.source &&
          (parentId
            ? selection.id === parentId && selection.childId === item.id
            : selection.id === item.id && (!selection.childId || !expandable))
        const label =
          parentId && source !== 'ai'
            ? l.scanVersion.replace('{version}', item.id)
            : item.name
        return (
          <div key={key}>
            <div
              className={cn(
                'flex min-w-0 items-center rounded-md hover:bg-muted',
                selected && 'bg-muted',
              )}
            >
              {!parentId &&
                (expandable ? (
                  <button
                    type="button"
                    aria-label={`${l.expandScope}: ${item.name}`}
                    aria-expanded={expanded === key}
                    className="shrink-0 rounded p-2 text-muted-foreground hover:text-foreground"
                    onClick={() =>
                      setExpanded((value) => (value === key ? '' : key))
                    }
                  >
                    <ChevronRight
                      className={cn(
                        'h-3.5 w-3.5 transition-transform',
                        expanded === key && 'rotate-90',
                      )}
                    />
                  </button>
                ) : (
                  <span className="w-[30px] shrink-0" />
                ))}
              <button
                type="button"
                title={label}
                aria-pressed={selected}
                className="flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-2 text-left text-sm"
                onClick={() => onSelect(item)}
              >
                <span className="min-w-0 flex-1 truncate">{label}</span>
                {!parentId && source === 'all' && (
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {l.sources[item.source]}
                  </span>
                )}
                {selected && <Check className="h-4 w-4 shrink-0" />}
              </button>
            </div>
            {expandable && expanded === key && (
              <div
                role="group"
                aria-label={item.name}
                className="mb-1 ml-5 border-l border-border pl-3"
              >
                <ScopeOptions
                  source={item.source}
                  parentId={item.id}
                  selection={selection}
                  onSelect={(child) => onSelect({ ...child, parent: item })}
                />
              </div>
            )}
          </div>
        )
      })}
      {loading && (
        <div
          role="status"
          className="flex items-center gap-2 p-2 text-xs text-muted-foreground"
        >
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          {l.loadingScopes}
        </div>
      )}
      {error && (
        <div role="alert" className="p-2 text-xs text-muted-foreground">
          {error}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setRetry((n) => n + 1)}
          >
            {l.retry}
          </Button>
        </div>
      )}
      {!loading && !error && !items.length && (
        <p className="p-2 text-xs text-muted-foreground">{l.noScopes}</p>
      )}
      {!loading && !error && hasMore && (
        <Button
          variant="ghost"
          size="sm"
          className="w-full"
          onClick={() => setPage((n) => n + 1)}
        >
          {l.loadMoreScopes}
        </Button>
      )}
    </div>
  )
}
export function SourceScopeFilter({
  source,
  value,
  disabled,
  onChange,
}: {
  source: string
  value: ScopeSelection | null
  disabled?: boolean
  onChange: (value: ScopeSelection | null) => void
}) {
  const { t } = useI18n()
  const l = t.vulnerabilityManagement
  const [open, setOpen] = useState(false)
  const allLabel =
    source === 'ai'
      ? l.allAIProjects
      : source === 'all'
        ? l.allScopes
        : l.allScanTasks
  const label = value
    ? [
        value.name,
        value.childId
          ? value.source === 'ai'
            ? value.childName
            : l.scanVersion.replace('{version}', value.childId)
          : '',
      ]
        .filter(Boolean)
        .join(' / ')
    : allLabel
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          disabled={disabled}
          aria-label={l.scopeFilter}
          className="w-full justify-between gap-2 font-normal md:w-56"
          title={label}
        >
          <span className="truncate">{label}</span>
          <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 max-w-[calc(100vw-2rem)] p-2">
        <div className="max-h-72 overflow-y-auto overscroll-contain">
          <Button
            variant="ghost"
            className={cn(
              'mb-1 w-full justify-between font-normal',
              !value && 'bg-muted',
            )}
            onClick={() => {
              onChange(null)
              setOpen(false)
            }}
          >
            <span className="pl-[30px]">{allLabel}</span>
            {!value && <Check className="h-4 w-4 shrink-0" />}
          </Button>
          <ScopeOptions
            key={source}
            source={source}
            selection={value}
            onSelect={(item) => {
              onChange({
                source: item.source,
                id: item.parent?.id || item.id,
                name: item.parent?.name || item.name,
                childId: item.parent ? item.id : undefined,
                childName: item.parent ? item.name : undefined,
              })
              setOpen(false)
            }}
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}
