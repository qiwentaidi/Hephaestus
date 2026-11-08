'use client'

import { useId, useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { useI18n } from '@/lib/i18n'

interface MobileListToolbarProps {
  search: string
  searchPlaceholder: string
  onSearchChange: (value: string) => void
  filters: ReactNode
  activeFilterCount: number
  onResetFilters: () => void
  createLabel: string
  onCreate?: () => void
  selecting: boolean
  onSelectingChange: (value: boolean) => void
  selectedCount: number
  allSelected: boolean
  someSelected: boolean
  canSelect: boolean
  onSelectAll: (checked: boolean) => void
  onDelete: () => void
}

/** Compact mobile actions. Search/filter controls only appear when requested. */
export function MobileListToolbar(props: MobileListToolbarProps) {
  const { t } = useI18n()
  const [filtersOpen, setFiltersOpen] = useState(false)
  const titleRef = useRef<HTMLHeadingElement>(null)
  const searchId = useId()

  return (
    <div className="md:hidden" data-slot="mobile-list-toolbar">
      {props.selecting ? (
        <div className="flex min-w-0 items-center gap-2">
          <label className="flex h-9 min-w-0 flex-1 items-center gap-2 whitespace-nowrap text-sm">
            <Checkbox checked={props.allSelected ? true : props.someSelected ? 'indeterminate' : false} disabled={!props.canSelect} onCheckedChange={checked => props.onSelectAll(checked === true)} aria-label={t.mobileList.selectPage} />
            {t.common.selectAll}
            <span className="tabular-nums text-muted-foreground" aria-live="polite">{t.mobileList.selectedCount.replace('{count}', String(props.selectedCount))}</span>
          </label>
          <Button variant="outline" className="h-9 px-3 text-destructive" disabled={!props.selectedCount} onClick={props.onDelete}>{t.common.delete}</Button>
          <Button variant="outline" className="h-9 px-3" onClick={() => props.onSelectingChange(false)}>{t.mobileList.done}</Button>
        </div>
      ) : (
        <div className="flex min-w-0 items-center gap-2">
          <Button variant="outline" className="h-9 min-w-0 flex-1 px-3" onClick={() => setFiltersOpen(true)}>{t.mobileList.filters}{props.activeFilterCount > 0 ? ` (${props.activeFilterCount})` : ''}</Button>
          <Button variant="outline" className="h-9 px-3" disabled={!props.canSelect} onClick={() => props.onSelectingChange(true)}>{t.mobileList.batch}</Button>
          {props.onCreate && <Button className="h-9 px-3" onClick={props.onCreate}>{props.createLabel}</Button>}
        </div>
      )}
      <Sheet open={filtersOpen} onOpenChange={setFiltersOpen}>
        <SheetContent side="bottom" aria-describedby={undefined} className="max-h-[85dvh] gap-0 overflow-hidden rounded-t-2xl pb-[max(1rem,env(safe-area-inset-bottom))]" onOpenAutoFocus={event => { event.preventDefault(); titleRef.current?.focus() }}>
          <SheetHeader className="shrink-0 pr-10"><SheetTitle ref={titleRef} tabIndex={-1} className="outline-none">{t.mobileList.filters}</SheetTitle></SheetHeader>
          <div className="min-h-0 space-y-4 overflow-y-auto overscroll-contain px-4 py-2">
            <div className="space-y-2">
              <label htmlFor={searchId} className="text-sm font-medium">{t.common.search}</label>
              <Input id={searchId} value={props.search} onChange={event => props.onSearchChange(event.target.value)} placeholder={props.searchPlaceholder} className="text-base" />
            </div>
            {props.filters}
          </div>
          <div className="flex shrink-0 gap-2 px-4 pt-4">
            <Button variant="outline" className="flex-1" onClick={props.onResetFilters}>{t.mobileList.reset}</Button>
            <Button className="flex-1" onClick={() => setFiltersOpen(false)}>{t.common.confirm}</Button>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  )
}
