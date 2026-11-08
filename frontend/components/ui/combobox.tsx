'use client'

import * as React from 'react'
import { Check, ChevronsUpDown, X } from 'lucide-react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Input } from '@/components/ui/input'
import { Popover, PopoverAnchor, PopoverContent } from '@/components/ui/popover'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

type ComboItem = string | Record<string, any>

interface ComboboxContextValue<T extends ComboItem> {
  items: T[]
  multiple: boolean
  selected: T[]
  setSelected: (next: T[]) => void
  toggleItem: (item: T) => void
  isSelected: (item: T) => boolean
  clear: () => void
  open: boolean
  setOpen: (open: boolean) => void
  search: string
  setSearch: (search: string) => void
  filteredItems: T[]
  getItemKey: (item: T) => string
  getItemLabel: (item: T) => string
  disabled: boolean
  listId: string
}

const ComboboxContext = React.createContext<ComboboxContextValue<any> | null>(null)

const normalizeValue = <T extends ComboItem>(
  value: T | T[] | null | undefined,
  multiple: boolean,
): T[] => {
  if (value == null) return []
  if (Array.isArray(value)) {
    return multiple ? value : value.slice(0, 1)
  }
  return [value]
}

const getItemLabel = <T extends ComboItem>(item: T): string => {
  if (typeof item === 'string') return item
  if (typeof item?.label === 'string') return item.label
  if (typeof item?.value === 'string') return item.value
  return String(item ?? '')
}

const getItemKey = <T extends ComboItem>(item: T): string => {
  if (typeof item === 'string') return item
  if (typeof item?.value === 'string') return item.value
  if (typeof item?.label === 'string') return item.label
  return JSON.stringify(item ?? {})
}

const dedupeByKey = <T extends ComboItem>(items: T[]): T[] => {
  const map = new Map<string, T>()
  items.forEach((item) => map.set(getItemKey(item), item))
  return Array.from(map.values())
}

const useComboboxContext = <T extends ComboItem>() => {
  const context = React.useContext(ComboboxContext)
  if (!context) {
    throw new Error('Combobox components must be used inside <Combobox>')
  }
  return context as ComboboxContextValue<T>
}

export interface ComboboxProps<T extends ComboItem> {
  items: readonly T[]
  children: React.ReactNode
  multiple?: boolean
  autoHighlight?: boolean
  value?: T | T[] | null
  defaultValue?: T | T[] | null
  onValueChange?: (value: T | T[] | null) => void
  disabled?: boolean
}

export function Combobox<T extends ComboItem>({
  items,
  children,
  multiple = false,
  value,
  defaultValue,
  onValueChange,
  disabled = false,
}: ComboboxProps<T>) {
  const [open, setOpen] = React.useState(false)
  const [search, setSearch] = React.useState('')
  const listId = React.useId()
  const isControlled = value !== undefined
  const [internalSelected, setInternalSelected] = React.useState<T[]>(() =>
    normalizeValue(defaultValue, multiple),
  )

  const selected = React.useMemo(
    () => normalizeValue(isControlled ? value : internalSelected, multiple),
    [value, internalSelected, isControlled, multiple],
  )

  const commitSelected = React.useCallback(
    (nextRaw: T[]) => {
      const next = multiple ? dedupeByKey(nextRaw) : nextRaw.slice(0, 1)
      if (!isControlled) {
        setInternalSelected(next)
      }
      if (onValueChange) {
        onValueChange(multiple ? next : (next[0] ?? null))
      }
    },
    [isControlled, multiple, onValueChange],
  )

  const isSelected = React.useCallback((item: T) => {
    const key = getItemKey(item)
    return selected.some((current) => getItemKey(current) === key)
  }, [selected])

  const toggleItem = React.useCallback(
    (item: T) => {
      if (disabled) return
      if (multiple) {
        if (isSelected(item)) {
          commitSelected(selected.filter((current) => getItemKey(current) !== getItemKey(item)))
        } else {
          commitSelected([...selected, item])
        }
        return
      }
      commitSelected([item])
      setOpen(false)
    },
    [disabled, multiple, selected, commitSelected, isSelected],
  )

  const clear = React.useCallback(() => {
    if (disabled) return
    commitSelected([])
  }, [commitSelected, disabled])

  const filteredItems = React.useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return [...items]
    return items.filter((item) => {
      const label = getItemLabel(item).toLowerCase()
      const key = getItemKey(item).toLowerCase()
      return label.includes(q) || key.includes(q)
    })
  }, [items, search])

  const contextValue = React.useMemo<ComboboxContextValue<T>>(
    () => ({
      items: [...items],
      multiple,
      selected,
      setSelected: commitSelected,
      toggleItem,
      isSelected,
      clear,
      open,
      setOpen,
      search,
      setSearch,
      filteredItems,
      getItemKey,
      getItemLabel,
      disabled,
      listId,
    }),
    [
      items,
      multiple,
      selected,
      commitSelected,
      toggleItem,
      isSelected,
      clear,
      open,
      search,
      filteredItems,
      disabled,
      listId,
    ],
  )

  return (
    <ComboboxContext.Provider value={contextValue}>
      <Popover open={open} onOpenChange={setOpen}>
        {children}
      </Popover>
    </ComboboxContext.Provider>
  )
}

export const useComboboxAnchor = () => React.useRef<HTMLDivElement | null>(null)

export interface ComboboxTriggerProps {
  render: React.ReactElement<React.HTMLAttributes<HTMLElement>>
}

export function ComboboxTrigger<T extends ComboItem>({ render }: ComboboxTriggerProps) {
  const { open, setOpen, disabled, listId } = useComboboxContext<T>()
  const currentOnClick = (render.props as any).onClick as ((event: React.MouseEvent) => void) | undefined

  return (
    <PopoverAnchor asChild>
      {React.cloneElement(render, {
        role: 'combobox',
        'aria-expanded': open,
        'aria-controls': listId,
        'aria-disabled': disabled,
        onClick: (event: React.MouseEvent) => {
          currentOnClick?.(event)
          if (!event.defaultPrevented && !disabled) {
            setOpen(!open)
          }
        },
      })}
    </PopoverAnchor>
  )
}

interface ComboboxInputProps extends Omit<React.ComponentProps<typeof Input>, 'value' | 'onChange'> {
  showClear?: boolean
  showTrigger?: boolean
}

export function ComboboxInput<T extends ComboItem>({
  showClear = false,
  showTrigger = true,
  className,
  ...props
}: ComboboxInputProps) {
  const { search, setSearch, setOpen, clear, selected, disabled } = useComboboxContext<T>()

  const content = (
    <div className="relative">
      <Input
        value={search}
        onFocus={() => !disabled && setOpen(true)}
        onChange={(event) => setSearch(event.target.value)}
        disabled={disabled}
        className={cn(showClear || showTrigger ? 'pr-16' : undefined, className)}
        {...props}
      />
      {(showClear || showTrigger) && (
        <div className="absolute inset-y-0 right-2 flex items-center gap-1">
          {showClear && selected.length > 0 && !disabled && (
            <button
              type="button"
              className="text-muted-foreground hover:text-foreground"
              onClick={(event) => {
                event.preventDefault()
                event.stopPropagation()
                clear()
                setSearch('')
              }}
            >
              <X className="h-4 w-4" />
            </button>
          )}
          {showTrigger && <ChevronsUpDown className="h-4 w-4 text-muted-foreground" />}
        </div>
      )}
    </div>
  )

  if (!showTrigger) {
    return content
  }

  return <PopoverAnchor asChild>{content}</PopoverAnchor>
}

export const ComboboxChips = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  function ComboboxChips({ className, ...props }, ref) {
    const { setOpen, open, disabled, listId } = useComboboxContext<ComboItem>()
    return (
      <PopoverAnchor asChild>
        <div
          role="combobox"
          aria-expanded={open}
          aria-controls={listId}
          aria-disabled={disabled}
          tabIndex={disabled ? -1 : 0}
          ref={ref}
          className={cn(
            'w-full min-h-10 rounded-md border bg-background px-3 py-2 text-sm flex items-center gap-2 flex-wrap cursor-text',
            'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
            disabled && 'cursor-not-allowed opacity-50',
            className,
          )}
          onClick={(event) => {
            if (disabled) return
            const target = event.target as HTMLElement | null
            if (target?.closest('[data-combobox-stop-open="true"]')) {
              return
            }
            setOpen(true)
          }}
          onKeyDown={(event) => {
            if (disabled) return
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault()
              setOpen(!open)
            }
          }}
          {...props}
        />
      </PopoverAnchor>
    )
  }
)

export function ComboboxChipsInput<T extends ComboItem>({
  className,
  ...props
}: React.ComponentProps<'input'>) {
  const { search, setSearch, setOpen, disabled } = useComboboxContext<T>()
  return (
    <input
      value={search}
      onFocus={() => !disabled && setOpen(true)}
      onChange={(event) => setSearch(event.target.value)}
      disabled={disabled}
      className={cn('bg-transparent outline-none border-0 p-0 m-0 min-w-[96px] flex-1', className)}
      {...props}
    />
  )
}

export function ComboboxChip({
  className,
  ...props
}: React.ComponentProps<typeof Badge>) {
  return <Badge variant="secondary" className={cn('gap-1', className)} {...props} />
}

interface ComboboxContentProps extends React.ComponentProps<typeof PopoverContent> {
  anchor?: React.RefObject<HTMLElement | null>
}

export function ComboboxContent<T extends ComboItem>({
  className,
  anchor,
  style,
  children,
  ...props
}: ComboboxContentProps) {
  const [anchorWidth, setAnchorWidth] = React.useState<number | null>(null)

  React.useEffect(() => {
    if (!anchor?.current) return
    const update = () => setAnchorWidth(anchor.current?.offsetWidth ?? null)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(anchor.current)
    return () => observer.disconnect()
  }, [anchor])

  return (
    <PopoverContent
      align="start"
      className={cn('p-0', className)}
      style={{
        width: anchorWidth ? `${anchorWidth}px` : 'var(--radix-popover-trigger-width)',
        minWidth: anchorWidth ? `${anchorWidth}px` : 'var(--radix-popover-trigger-width)',
        maxWidth: anchorWidth ? `${anchorWidth}px` : 'var(--radix-popover-trigger-width)',
        ...style,
      }}
      {...props}
    >
      {children}
    </PopoverContent>
  )
}

export function ComboboxEmpty<T extends ComboItem>({
  children,
  className,
  ...props
}: React.ComponentProps<'div'>) {
  const { filteredItems } = useComboboxContext<T>()
  if (filteredItems.length > 0) return null
  return (
    <div className={cn('py-6 text-center text-sm text-muted-foreground', className)} {...props}>
      {children}
    </div>
  )
}

interface ComboboxListProps<T extends ComboItem> extends Omit<React.ComponentProps<'div'>, 'children'> {
  children: (item: T) => React.ReactNode
  virtualThreshold?: number
  estimateSize?: number
}

export function ComboboxList<T extends ComboItem>({
  className,
  children,
  virtualThreshold = 200,
  estimateSize = 34,
  id,
  ...props
}: ComboboxListProps<T>) {
  const { filteredItems, getItemKey, listId } = useComboboxContext<T>()
  const parentRef = React.useRef<HTMLDivElement>(null)
  const shouldVirtualize = filteredItems.length >= virtualThreshold
  const resolvedListId = id ?? listId

  // TanStack Virtual returns mutable helpers that React Compiler intentionally skips.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: shouldVirtualize ? filteredItems.length : 0,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan: 10,
  })

  if (!shouldVirtualize) {
    return (
      <div
        id={resolvedListId}
        role="listbox"
        ref={parentRef}
        className={cn('max-h-72 overflow-auto p-1 space-y-1', className)}
        onWheelCapture={(event) => {
          const el = parentRef.current
          if (!el) return
          if (Math.abs(event.deltaY) > 0) {
            el.scrollTop += event.deltaY
            event.preventDefault()
            event.stopPropagation()
          }
        }}
        {...props}
      >
        {filteredItems.map((item) => children(item))}
      </div>
    )
  }

  const virtualRows = virtualizer.getVirtualItems()
  const totalSize = virtualizer.getTotalSize()

  return (
    <div
      id={resolvedListId}
      role="listbox"
      ref={parentRef}
      className={cn('max-h-72 overflow-auto p-1', className)}
      onWheelCapture={(event) => {
        const el = parentRef.current
        if (!el) return
        if (Math.abs(event.deltaY) > 0) {
          el.scrollTop += event.deltaY
          event.preventDefault()
          event.stopPropagation()
        }
      }}
      {...props}
    >
      <div style={{ height: totalSize, position: 'relative', width: '100%' }}>
        {virtualRows.map((row) => {
          const item = filteredItems[row.index]
          if (!item) return null
          return (
            <div
              key={getItemKey(item)}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${row.start}px)`,
              }}
            >
              {children(item)}
            </div>
          )
        })}
      </div>
    </div>
  )
}

interface ComboboxItemProps<T extends ComboItem> extends Omit<React.ComponentProps<'button'>, 'value'> {
  value: T
}

export function ComboboxItem<T extends ComboItem>({
  value,
  className,
  children,
  ...props
}: ComboboxItemProps<T>) {
  const { isSelected, toggleItem, disabled } = useComboboxContext<T>()
  const selected = isSelected(value)

  return (
    <button
      type="button"
      data-slot="combobox-item"
      role="option"
      aria-selected={selected}
      disabled={disabled}
      className={cn(
        'w-full flex items-center gap-2 rounded-sm px-2 py-1.5 text-sm text-left hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50',
        className,
      )}
      onClick={() => toggleItem(value)}
      {...props}
    >
      <Check className={cn('h-4 w-4', selected ? 'opacity-100' : 'opacity-0')} />
      <span className="truncate flex-1">{children ?? getItemLabel(value)}</span>
    </button>
  )
}

interface ComboboxValueProps<T extends ComboItem> {
  children?: React.ReactNode | ((value: T[] | T | null) => React.ReactNode)
}

export function ComboboxValue<T extends ComboItem>({ children }: ComboboxValueProps<T>) {
  const { selected, multiple, getItemLabel } = useComboboxContext<T>()
  const payload: T[] | T | null = multiple ? selected : (selected[0] ?? null)

  if (typeof children === 'function') {
    return <>{children(payload)}</>
  }
  if (children) {
    return <>{children}</>
  }
  if (multiple) {
    return <>{selected.map((item) => getItemLabel(item)).join(', ')}</>
  }
  return <>{selected[0] ? getItemLabel(selected[0]) : ''}</>
}
