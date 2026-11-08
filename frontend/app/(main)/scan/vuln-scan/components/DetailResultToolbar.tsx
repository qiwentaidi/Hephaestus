'use client'

import { createContext, useContext, useRef, useState, type ReactNode } from 'react'
import { useIsMobile } from '@/hooks/use-mobile'
import { useI18n } from '@/lib/i18n'
import { MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

// The active result panel owns the toolbar, so the category selector shares its row.
export const DetailCategoryContext = createContext<ReactNode>(null)

export function DetailResultToolbar({ children, actions, activeCount = 0, onReset, onApply, className }: {
  children: ReactNode
  actions?: ReactNode
  activeCount?: number
  onReset?: () => void
  onApply?: () => void
  className?: string
}) {
  const isMobile = useIsMobile()
  const category = useContext(DetailCategoryContext)
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [actionsOpen, setActionsOpen] = useState(false)
  const titleRef = useRef<HTMLHeadingElement>(null)

  if (!isMobile) return className ? <div className={className}>{children}{actions}</div> : <>{children}{actions}</>

  return <div className="flex w-full min-w-0 items-center gap-2" data-slot="detail-result-toolbar">
    {category && <div className="min-w-0 flex-1">{category}</div>}
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild><Button variant="outline" className={category ? 'h-9 shrink-0 px-2' : 'h-9 flex-1'}>{t.mobileList.filters}{activeCount > 0 ? ` (${activeCount})` : ''}</Button></SheetTrigger>
      <SheetContent side="bottom" aria-describedby={undefined} className="max-h-[85dvh] gap-0 overflow-hidden rounded-t-2xl pb-[max(1rem,env(safe-area-inset-bottom))]" onOpenAutoFocus={event => { event.preventDefault(); titleRef.current?.focus() }}>
        <SheetHeader className="shrink-0 pr-10"><SheetTitle ref={titleRef} tabIndex={-1} className="outline-none">{t.mobileList.filters}</SheetTitle></SheetHeader>
        <div className="min-h-0 space-y-3 overflow-y-auto overscroll-contain px-4 py-2 [&_input]:text-base [&_input]:min-w-0 [&_[role=combobox]]:w-full [&_[role=combobox]]:min-w-0 [&_[role=combobox]]:max-w-full [&>div]:w-full [&>div]:min-w-0 [&>div]:max-w-full [&>div]:flex-wrap [&>div]:gap-3">{children}</div>
        <div className="flex shrink-0 gap-2 px-4 pt-4">
          {onReset && <Button variant="outline" className="flex-1" onClick={onReset}>{t.mobileList.reset}</Button>}
          <Button className="flex-1" onClick={() => { onApply?.(); setOpen(false) }}>{t.common.confirm}</Button>
        </div>
      </SheetContent>
    </Sheet>
    {actions && <Popover open={actionsOpen} onOpenChange={setActionsOpen}>
      <PopoverTrigger asChild><Button variant="outline" className="h-9 w-9 shrink-0 p-0 text-lg" aria-label={t.mobileList.resultActions}><MoreHorizontal className="h-4 w-4" /></Button></PopoverTrigger>
      <PopoverContent align="end" className="w-auto max-w-[calc(100vw-2rem)] p-2 [&>div]:flex-col [&_button]:w-full [&_button]:justify-start" onClick={event => { if ((event.target as HTMLElement).closest('button')) setActionsOpen(false) }}>{actions}</PopoverContent>
    </Popover>}
  </div>
}
