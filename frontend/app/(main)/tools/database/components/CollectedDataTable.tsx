'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { Search, Settings, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import type { RowData } from '../types'
import { formatDatabaseValue } from '../utils/data'

export function CollectedDataTable({ data }: { data: RowData }) {
  const { t } = useI18n()
  const labels = t.tools.database.tableDetail
  const tableHostRef = useRef<HTMLDivElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [hiddenColumns, setHiddenColumns] = useState<Set<number>>(() => new Set())
  const [fixedShadows, setFixedShadows] = useState({ left: false, right: false })
  const visibleColumnIndexes = useMemo(
    () => data.columns.flatMap((_, index) => hiddenColumns.has(index) ? [] : [index]),
    [data.columns, hiddenColumns],
  )
  const filteredRows = useMemo(() => {
    const query = searchQuery.trim().toLowerCase()
    if (!query) return data.rows
    return data.rows.filter(row => row.some(value => formatDatabaseValue(value).toLowerCase().includes(query)))
  }, [data.rows, searchQuery])

  const clearSearch = () => {
    setSearchQuery('')
    searchInputRef.current?.focus()
  }

  useEffect(() => {
    const container = tableHostRef.current?.querySelector<HTMLDivElement>('[data-slot="table-container"]')
    const table = container?.querySelector('table')
    if (!container || !table) return

    const updateShadows = () => {
      const maxScrollLeft = container.scrollWidth - container.clientWidth
      const left = maxScrollLeft > 1 && container.scrollLeft > 1
      const right = maxScrollLeft > 1 && container.scrollLeft < maxScrollLeft - 1
      setFixedShadows(previous => previous.left === left && previous.right === right ? previous : { left, right })
    }

    updateShadows()
    container.addEventListener('scroll', updateShadows, { passive: true })
    const observer = new ResizeObserver(updateShadows)
    observer.observe(container)
    observer.observe(table)
    return () => {
      container.removeEventListener('scroll', updateShadows)
      observer.disconnect()
    }
  }, [visibleColumnIndexes, filteredRows])

  const fixedColumnClass = (index: number, header = false) => cn(
    index === 0
      ? ['sticky left-0 border-r border-border', fixedShadows.left && 'table-fixed-column-shadow-right']
      : index === visibleColumnIndexes.length - 1
        ? ['sticky right-0 border-l border-border', fixedShadows.right && 'table-fixed-column-shadow-left']
        : undefined,
    (index === 0 || index === visibleColumnIndexes.length - 1) && [
      'bg-card',
      header ? 'z-20' : 'z-10 group-hover:bg-muted group-data-[state=selected]:bg-muted',
    ],
  )

  return (
    <div className="w-full min-w-0 space-y-4">
      <div className="flex w-full min-w-0 items-center gap-4">
        <div className="relative min-w-0 flex-1">
          <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            ref={searchInputRef}
            value={searchQuery}
            onChange={event => setSearchQuery(event.target.value)}
            onKeyDown={event => { if (event.key === 'Escape') clearSearch() }}
            placeholder={labels.searchPlaceholder}
            aria-label={labels.searchPlaceholder}
            title={labels.searchScope}
            className="pl-9 pr-9"
          />
          {searchQuery && (
            <Button type="button" variant="ghost" size="icon-sm" className="absolute right-1 top-1/2 size-7 -translate-y-1/2 text-muted-foreground" aria-label={labels.clearSearch} onClick={clearSearch}>
              <X aria-hidden="true" className="h-4 w-4" />
            </Button>
          )}
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type="button" variant="outline" size="icon" className="h-8 w-9" aria-label={labels.showColumns} title={labels.showColumns} disabled={data.columns.length === 0}>
              <Settings aria-hidden="true" className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-64 max-w-[calc(100vw-2rem)]">
            <DropdownMenuLabel>{labels.showColumns}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem disabled={hiddenColumns.size === 0} onSelect={event => { event.preventDefault(); setHiddenColumns(new Set()) }}>
              {labels.showAllColumns}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <div className="max-h-[min(320px,50vh)] overflow-y-auto">
              {data.columns.map((column, index) => (
                <DropdownMenuCheckboxItem
                  key={index}
                  checked={!hiddenColumns.has(index)}
                  disabled={!hiddenColumns.has(index) && visibleColumnIndexes.length === 1}
                  onSelect={event => event.preventDefault()}
                  onCheckedChange={checked => {
                    if (!checked && visibleColumnIndexes.length <= 1) return
                    setHiddenColumns(previous => {
                      const next = new Set(previous)
                      if (checked) next.delete(index)
                      else next.add(index)
                      return next
                    })
                  }}
                >
                  <span className="truncate" title={column}>{column}</span>
                </DropdownMenuCheckboxItem>
              ))}
            </div>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <div ref={tableHostRef} className="w-full min-w-0 overflow-hidden rounded-lg border border-border bg-card">
        <Table className="min-w-max border-separate border-spacing-0 table-auto">
          <TableHeader>
            <TableRow className="hover:bg-transparent">
              {visibleColumnIndexes.map((columnIndex, index) => (
                <TableHead key={columnIndex} className={cn('border-b border-border px-3', fixedColumnClass(index, true))}>
                  <div className={cn('truncate', index === 0 || index === visibleColumnIndexes.length - 1 ? 'max-w-[min(180px,25vw)]' : 'max-w-[300px]')} title={data.columns[columnIndex]}>
                    {data.columns[columnIndex]}
                  </div>
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredRows.map((row, rowIndex) => (
              <TableRow key={rowIndex} className="group hover:bg-muted [&:last-child>td]:border-b-0">
                {visibleColumnIndexes.map((columnIndex, index) => {
                  const value = row[columnIndex]
                  const text = formatDatabaseValue(value)
                  return (
                    <TableCell key={columnIndex} className={cn('border-b border-border px-3 py-3', fixedColumnClass(index))}>
                      <div className={cn('cell-content truncate', index === 0 || index === visibleColumnIndexes.length - 1 ? 'max-w-[min(180px,25vw)]' : 'max-w-[300px]')} title={text.substring(0, 200)}>
                        {value === null ? <span className="text-muted-foreground italic">NULL</span> : text}
                      </div>
                    </TableCell>
                  )
                })}
              </TableRow>
            ))}
          </TableBody>
        </Table>
        {filteredRows.length === 0 && (
          <div role="status" className="p-8 text-center text-sm text-muted-foreground">
            {searchQuery.trim() ? labels.noMatchingRows : labels.noSampleRows}
          </div>
        )}
      </div>
    </div>
  )
}
