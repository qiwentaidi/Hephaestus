'use client'

import Image from 'next/image'

import { Checkbox } from '@/components/ui/checkbox'

export interface ReportFormatOption<T extends string> {
  value: T
  label: string
  icon: string
}

interface ReportFormatSelectorProps<T extends string> {
  options: ReportFormatOption<T>[]
  selected: T[]
  onToggle: (value: T) => void
  ariaLabel: string
  multiple?: boolean
  className?: string
}

/**
 * Shared report format picker used by every export tab in report management.
 * The AI Workbench export uses the same card, checkbox, icon, and selection
 * treatment as scan and company-info exports instead of a separate control.
 */
export function ReportFormatSelector<T extends string>({
  options,
  selected,
  onToggle,
  ariaLabel,
  multiple = true,
  className = 'sm:grid-cols-3',
}: ReportFormatSelectorProps<T>) {
  const activate = (value: T, checked: boolean) => {
    if (!multiple && checked) return
    onToggle(value)
  }

  return (
    <div
      className={`grid gap-2 ${className}`}
      role={multiple ? 'group' : 'radiogroup'}
      aria-label={ariaLabel}
    >
      {options.map((option) => {
        const checked = selected.includes(option.value)
        return (
          <div
            key={option.value}
            role={multiple ? 'checkbox' : 'radio'}
            aria-checked={checked}
            aria-label={option.label}
            tabIndex={0}
            onClick={() => activate(option.value, checked)}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                activate(option.value, checked)
              }
            }}
            className={`flex cursor-pointer items-center gap-2 rounded-md border px-3 py-2 text-sm transition-colors ${
              checked
                ? 'border-primary/50 bg-primary/10 text-foreground'
                : 'border-border bg-background hover:bg-muted/40 text-muted-foreground'
            }`}
          >
            <Checkbox checked={checked} className="pointer-events-none" tabIndex={-1} aria-hidden="true" />
            <Image src={option.icon} alt={option.label} width={16} height={16} className="h-4 w-4 shrink-0" />
            <span>{option.label}</span>
          </div>
        )
      })}
    </div>
  )
}
