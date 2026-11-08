'use client'

import * as React from 'react'
import { CalendarDays, Clock3 } from 'lucide-react'
import { zhCN, enUS } from 'date-fns/locale'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

interface SchedulePickerProps {
  id: string
  label: string
  value: string
  disabled?: boolean
  onChange: (value: string) => void
}

const twoDigits = (value: number) => String(value).padStart(2, '0')

function TimeColumn({ label, count, selected, onSelect }: {
  label: string
  count: number
  selected: string
  onSelect: (value: string) => void
}) {
  const host = React.useRef<HTMLDivElement>(null)
  React.useEffect(() => {
    host.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'center' })
  }, [])

  return (
    <div className="min-w-0 flex-1 space-y-2">
      <div className="text-center text-xs text-muted-foreground">{label}</div>
      <div ref={host} role="listbox" aria-label={label} className="h-52 overflow-y-auto overscroll-contain rounded-md border border-border p-1" onKeyDown={event => {
        const changes: Record<string, number> = { ArrowDown: 1, ArrowUp: -1, Home: -Number(selected), End: count - 1 - Number(selected) }
        if (!(event.key in changes)) return
        event.preventDefault()
        const next = twoDigits((Number(selected) + changes[event.key] + count) % count)
        onSelect(next)
        host.current?.querySelector<HTMLButtonElement>(`[data-value="${next}"]`)?.focus()
      }}>
        {Array.from({ length: count }, (_, index) => twoDigits(index)).map(value => (
          <button key={value} type="button" role="option" aria-selected={selected === value} data-value={value} tabIndex={selected === value ? 0 : -1}
            onClick={() => onSelect(value)}
            className={cn('flex h-9 w-full items-center justify-center rounded-sm text-sm tabular-nums outline-none hover:bg-accent hover:text-accent-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset', selected === value && 'bg-accent font-semibold text-accent-foreground ring-1 ring-inset ring-primary/40')}>
            {value}
          </button>
        ))}
      </div>
    </div>
  )
}

export function ScheduleTimePicker({ id, label, value, disabled, onChange }: SchedulePickerProps) {
  const { t } = useI18n()
  const a = t.vulnScan.newScan.alarmSchedule
  const [open, setOpen] = React.useState(false)
  const match = /^(\d{1,2}):(\d{1,2})$/.exec(value)
  const hour = match && Number(match[1]) < 24 ? twoDigits(Number(match[1])) : '02'
  const minute = match && Number(match[2]) < 60 ? twoDigits(Number(match[2])) : '00'

  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <div className="relative w-[112px]">
        <Input id={id} aria-label={label} value={value} disabled={disabled} inputMode="numeric" maxLength={5} placeholder="HH:mm" className="pr-10 tabular-nums"
          onChange={event => onChange(event.target.value)}
          onBlur={() => { if (match && Number(match[1]) < 24 && Number(match[2]) < 60 && value !== `${hour}:${minute}`) onChange(`${hour}:${minute}`) }}
          onKeyDown={event => { if (event.key === 'ArrowDown') { event.preventDefault(); setOpen(true) } }} />
        <PopoverTrigger asChild>
          <button type="button" disabled={disabled} aria-label={`${label} · ${a.openTime}`} className="absolute inset-y-0 right-0 flex w-9 items-center justify-center rounded-r-md text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
            <Clock3 className="size-4" aria-hidden="true" />
          </button>
        </PopoverTrigger>
      </div>
      <PopoverContent align="start" className="w-60 p-3" role="dialog" aria-label={a.selectTime}>
        <div className="mb-3 text-sm font-medium">{a.selectTime}</div>
        <div className="flex gap-3">
          <TimeColumn label={a.hour} count={24} selected={hour} onSelect={next => onChange(`${next}:${minute}`)} />
          <TimeColumn label={a.minute} count={60} selected={minute} onSelect={next => onChange(`${hour}:${next}`)} />
        </div>
        <Button type="button" variant="outline" size="sm" className="mt-3 w-full" onClick={() => setOpen(false)}>{a.done}</Button>
      </PopoverContent>
    </Popover>
  )
}

export function ScheduleDatePicker({ id, label, value, disabled, onChange }: SchedulePickerProps) {
  const { t, language } = useI18n()
  const a = t.vulnScan.newScan.alarmSchedule
  const [open, setOpen] = React.useState(false)
  const candidate = /^\d{4}-\d{2}-\d{2}$/.test(value) ? new Date(`${value}T12:00:00`) : undefined
  const selected = candidate && !Number.isNaN(candidate.getTime()) && `${candidate.getFullYear()}-${twoDigits(candidate.getMonth() + 1)}-${twoDigits(candidate.getDate())}` === value ? candidate : undefined

  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <div className="relative">
        <Input id={id} aria-label={label} value={value} disabled={disabled} placeholder={a.datePlaceholder} maxLength={10} className="pr-10 tabular-nums" onChange={event => onChange(event.target.value)} />
        <PopoverTrigger asChild>
          <button type="button" disabled={disabled} aria-label={`${label} · ${a.openDate}`} className="absolute inset-y-0 right-0 flex w-9 items-center justify-center rounded-r-md text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
            <CalendarDays className="size-4" aria-hidden="true" />
          </button>
        </PopoverTrigger>
      </div>
      <PopoverContent align="start" className="w-auto p-0" role="dialog" aria-label={a.selectDate}>
        <Calendar mode="single" selected={selected} defaultMonth={selected} locale={language === 'zh' ? zhCN : enUS} onSelect={date => {
          if (!date) return
          onChange(`${date.getFullYear()}-${twoDigits(date.getMonth() + 1)}-${twoDigits(date.getDate())}`)
          setOpen(false)
        }} />
      </PopoverContent>
    </Popover>
  )
}
