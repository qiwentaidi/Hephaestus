'use client'

import { ChevronDown } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu, DropdownMenuTrigger, DropdownMenuContent,
  DropdownMenuCheckboxItem, DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { formatWorkbenchText } from '@/lib/ai-workbench-i18n'

export function FindingMultiSelectFilter<T extends string>({ values, options, allLabel, countLabel, ariaLabel, disabled, onChange }: {
  values: T[]
  options: { value: T; label: string }[]
  allLabel: string
  countLabel: string
  ariaLabel: string
  disabled?: boolean
  onChange: (values: T[]) => void
}) {
  const selectedLabels = options.filter(option => values.includes(option.value)).map(option => option.label)
  const label = values.length === 0 ? allLabel : values.length === 1 ? selectedLabels[0] || values[0] : formatWorkbenchText(countLabel, { count: values.length })
  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="outline" disabled={disabled} aria-label={`${ariaLabel}: ${label}`} title={selectedLabels.length ? selectedLabels.join(' / ') : allLabel} className="w-full justify-between rounded-lg px-3 font-normal hover:border-border hover:bg-muted hover:text-foreground dark:hover:border-border dark:hover:bg-muted dark:hover:text-foreground xl:w-36">
        <span className="truncate">{label}</span>
        <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground opacity-50" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="start" className="min-w-44">
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
