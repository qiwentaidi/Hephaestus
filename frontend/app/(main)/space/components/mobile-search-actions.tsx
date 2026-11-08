'use client'

import { useRef, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { useI18n } from '@/lib/i18n'
import { useIsMobile } from '@/hooks/use-mobile'

export function MobileSpaceSheet({ label, children, count = 0 }: { label: string; children: ReactNode; count?: number }) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const titleRef = useRef<HTMLHeadingElement>(null)
  return <Sheet open={open} onOpenChange={setOpen}>
    <SheetTrigger asChild><Button variant="outline" className="h-9 px-2">{label}{count > 0 ? ` (${count})` : ''}</Button></SheetTrigger>
    <SheetContent side="bottom" aria-describedby={undefined} className="max-h-[85dvh] gap-0 overflow-hidden rounded-t-2xl pb-[max(1rem,env(safe-area-inset-bottom))]" onOpenAutoFocus={event => { event.preventDefault(); titleRef.current?.focus() }}>
      <SheetHeader className="shrink-0 pr-10"><SheetTitle ref={titleRef} tabIndex={-1} className="outline-none">{label}</SheetTitle></SheetHeader>
      <div className="min-h-0 space-y-3 overflow-y-auto overscroll-contain px-4 py-2 [&>div]:w-full [&>div]:min-w-0 [&>div]:max-w-full [&>div]:flex-wrap [&>div]:gap-3 [&_[role=combobox]]:w-full [&_input]:text-base">{children}</div>
      <div className="shrink-0 px-4 pt-4"><Button className="w-full" onClick={() => setOpen(false)}>{t.common.confirm}</Button></div>
    </SheetContent>
  </Sheet>
}

export function MobileSearchActions({ options, count, account, actions, search }: {
  options?: ReactNode
  count?: number
  account: ReactNode
  actions: { label: string; onSelect: () => void }[]
  search: ReactNode
}) {
  const { t } = useI18n()
  const [accountOpen, setAccountOpen] = useState(false)
  return <div className="flex items-center gap-1.5 p-2" data-slot="mobile-space-search-actions">
    {options && <MobileSpaceSheet label={t.mobileSpace.options} count={count}>{options}</MobileSpaceSheet>}
    <DropdownMenu><DropdownMenuTrigger asChild><Button variant="outline" className="h-9 px-2">{t.mobileSpace.more}</Button></DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {actions.map(action => <DropdownMenuItem key={action.label} onSelect={action.onSelect}>{action.label}</DropdownMenuItem>)}
        <DropdownMenuItem onSelect={() => setAccountOpen(true)}>{t.mobileSpace.account}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
    <div className="ml-auto [&>button]:h-9 [&>button]:px-2">{search}</div>
    <Sheet open={accountOpen} onOpenChange={setAccountOpen}>
      <SheetContent side="bottom" aria-describedby={undefined} className="max-h-[85dvh] overflow-y-auto rounded-t-2xl pb-[max(1rem,env(safe-area-inset-bottom))]">
        <SheetHeader className="pr-10"><SheetTitle>{t.mobileSpace.account}</SheetTitle></SheetHeader>
        <div className="min-w-0 px-4 [overflow-wrap:anywhere] [&_.truncate]:whitespace-normal [&_.truncate]:overflow-visible [&_.flex]:flex-wrap [&_.grid-cols-3]:grid-cols-2">{account}</div>
      </SheetContent>
    </Sheet>
  </div>
}

export function SpaceResultFilters({ children, count }: { children: ReactNode; count: number }) {
  const isMobile = useIsMobile()
  const { t } = useI18n()
  return isMobile ? <MobileSpaceSheet label={t.mobileList.filters} count={count}>{children}</MobileSpaceSheet> : <>{children}</>
}

// Keep desktop truncation while allowing all result fields to wrap inside mobile cards.
export const mobileSpaceResultClassName = [
  'max-md:[&_article_*]:[overflow-wrap:anywhere]',
  'max-md:[&_article_.truncate]:min-w-0',
  'max-md:[&_article_.truncate]:whitespace-normal',
  'max-md:[&_article_.truncate]:overflow-visible',
  'max-md:[&_article_a]:min-w-0',
  'max-md:[&_article_svg]:shrink-0',
  'max-md:[&_article_.line-clamp-2]:line-clamp-none',
  'max-md:[&_article_[data-slot=badge]]:max-w-full',
  'max-md:[&_article_[data-slot=badge]]:whitespace-normal',
].join(' ')
