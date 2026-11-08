'use client'

import React from 'react'
import { Plus, X } from 'lucide-react'
import { ScheduleDatePicker, ScheduleTimePicker } from '@/components/ui/schedule-picker'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { useI18n } from '@/lib/i18n'
import { fetchWithAuth } from '@/lib/api'
import type { ScheduleConfig, ScheduleKind, ScheduleSpec } from '../../types'
import { formatRunLabel } from '../../utils/task-schedule'

const weekdayKeys = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'] as const

export function ScheduleSpecFields({ value, disabled, onChange }: {
  value: ScheduleSpec
  disabled: boolean
  onChange: (patch: Partial<ScheduleSpec>) => void
}) {
  const { t } = useI18n()
  const a = t.vulnScan.newScan.alarmSchedule
  const id = React.useId()
  const repeatLabels: Partial<Record<ScheduleKind, string>> = {
    once: a.once, daily: a.daily, weekdays: a.workdays, weekly: a.weekly, monthly: a.monthly,
  }
  const times = value.times && value.times.length > 0 ? value.times : ['02:00']

  const changeKind = (kind: ScheduleKind) => {
    const patch: Partial<ScheduleSpec> = { kind }
    if (kind === 'once') {
      patch.times = times.slice(0, 1)
      patch.date = value.date || ''
    } else if (kind === 'weekly') {
      patch.weekdays = value.weekdays?.length ? value.weekdays : [1]
    } else if (kind === 'monthly') {
      patch.dayOfMonth = value.dayOfMonth || 1
    }
    onChange(patch)
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-kind`} className="text-xs text-muted-foreground">{a.repeat}</Label>
          <Select value={value.kind} disabled={disabled} onValueChange={(kind: ScheduleKind) => changeKind(kind)}>
            <SelectTrigger id={`${id}-kind`} className="h-10 w-[150px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              {(Object.keys(repeatLabels) as ScheduleKind[]).map(kind => (
                <SelectItem key={kind} value={kind}>{repeatLabels[kind]}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {value.kind === 'once' && <div className="space-y-1.5">
          <Label htmlFor={`${id}-date`} className="text-xs text-muted-foreground">{a.date}</Label>
          <div className="w-[180px]">
            <ScheduleDatePicker id={`${id}-date`} label={a.date} value={value.date || ''} disabled={disabled} onChange={date => onChange({ date })} />
          </div>
        </div>}
        {value.kind === 'monthly' && <div className="space-y-1.5">
          <Label htmlFor={`${id}-day`} className="text-xs text-muted-foreground">{a.dayOfMonth}</Label>
          <Select value={String(value.dayOfMonth || 1)} disabled={disabled} onValueChange={day => onChange({ dayOfMonth: Number(day) })}>
            <SelectTrigger id={`${id}-day`} className="h-10 w-[140px]"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="-1">{a.lastDay}</SelectItem>
              {Array.from({ length: 31 }, (_, i) => i + 1).map(day => <SelectItem key={day} value={String(day)}>{day}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>}
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-time-0`} className="text-xs text-muted-foreground">{a.time}</Label>
          <div className="flex flex-wrap items-center gap-2">
            {times.map((clock, index) => <div key={index} className="flex items-center gap-1">
              <ScheduleTimePicker id={`${id}-time-${index}`} label={`${a.time} ${index + 1}`} value={clock} disabled={disabled} onChange={next => onChange({ times: times.map((time, i) => i === index ? next : time) })} />
              {value.kind !== 'once' && times.length > 1 && <Button type="button" variant="ghost" size="icon" className="size-8 text-muted-foreground hover:text-foreground" disabled={disabled} aria-label={`${a.removeTime} ${index + 1}`} onClick={() => onChange({ times: times.filter((_, i) => i !== index) })}>
                <X className="size-3.5" />
              </Button>}
            </div>)}
            {value.kind !== 'once' && <Button type="button" variant="outline" className="h-10 border-dashed text-muted-foreground" disabled={disabled || times.length >= 24} onClick={() => onChange({ times: [...times, '02:00'] })}>
              <Plus className="mr-1.5 size-3.5" />{a.addTime}
            </Button>}
          </div>
        </div>
      </div>
      {value.kind === 'weekly' && <div className="flex flex-wrap items-center gap-3">
        <Label className="text-xs text-muted-foreground">{a.weekdays}</Label>
        <ToggleGroup type="multiple" value={(value.weekdays || []).map(String)} onValueChange={days => onChange({ weekdays: days.map(Number) })} className="flex flex-wrap justify-start gap-1" disabled={disabled}>
          {weekdayKeys.map((key, i) => <ToggleGroupItem key={key} value={String(i + 1)} className="size-9 rounded-md border border-border/60 p-0 text-xs data-[state=on]:bg-accent data-[state=on]:text-accent-foreground">{t.vulnScan.newScan.weekdays[key]}</ToggleGroupItem>)}
        </ToggleGroup>
      </div>}
      {value.kind === 'monthly' && <p className="text-xs text-muted-foreground">{a.missingDayHint}</p>}
    </div>
  )
}

export function ScheduleRunPreview({ value }: { value: ScheduleConfig }) {
  const { t, language } = useI18n()
  const a = t.vulnScan.newScan.alarmSchedule
  const [preview, setPreview] = React.useState<{ key: string; runs: string[]; error?: 'invalid' | 'previewFailed' }>()
  const key = JSON.stringify(value)
  React.useEffect(() => {
    if (!value.spec || !value.enabled) return
    const controller = new AbortController()
    const timer = setTimeout(async () => {
      try {
        const response = await fetchWithAuth('/api/v1/webscan/schedule/preview', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: key, signal: controller.signal,
        })
        const payload = await response.json()
        if (!controller.signal.aborted) {
          setPreview({
            key,
            runs: (payload.data?.runs || []).slice(0, 3),
            error: response.ok && payload.success ? undefined : response.status === 400 ? 'invalid' : 'previewFailed',
          })
        }
      } catch {
        if (!controller.signal.aborted) setPreview({ key, runs: [], error: 'previewFailed' })
      }
    }, 350)
    return () => { clearTimeout(timer); controller.abort() }
  }, [key, value.spec, value.enabled])
  if (!value.spec) return null
  const current = preview?.key === key ? preview : undefined
  return <div className="flex min-h-10 flex-wrap items-center gap-2 rounded-lg border border-border/60 bg-background/40 px-3 py-2 text-sm" aria-live="polite">
    <span className="text-xs text-muted-foreground">{a.futureRuns}</span>
    {!value.enabled ? <span className="text-xs text-muted-foreground">{a.disabled}</span>
      : !current ? <span className="text-xs text-muted-foreground">{a.loading}</span>
        : current.error ? <span className="text-xs text-muted-foreground">{a[current.error]}</span>
          : current.runs.length === 0 ? <span className="text-xs text-muted-foreground">{a.noFuture}</span>
            : current.runs.map(run => <span key={run} className="rounded-full border border-border/60 bg-muted/40 px-2.5 py-1 text-xs tabular-nums text-muted-foreground" title={new Date(run).toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US')}>
              {formatRunLabel(run, t)}
            </span>)}
  </div>
}
