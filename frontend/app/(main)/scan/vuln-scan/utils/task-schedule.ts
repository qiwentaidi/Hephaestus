import type { ScheduleConfig, ScheduleKind, ScheduleSpec, WebScanTaskSummary } from '../types'
import type { Locale } from '@/lib/i18n/locales/zh'

export function createDefaultSpec(): ScheduleSpec {
  return { kind: 'daily', times: ['02:00'] }
}

// A manual task has no spec; the editor opens with the plan switch off.
export function createDefaultSchedule(): ScheduleConfig {
  return { enabled: false, spec: null }
}

interface WebScanTaskLike {
  scheduleEnabled?: boolean | null
  scheduleSpec?: ScheduleSpec | null
}

export function scheduleFromTask(task: WebScanTaskSummary | WebScanTaskLike | null | undefined): ScheduleConfig {
  if (!task) return createDefaultSchedule()
  const spec = task.scheduleSpec ?? null
  const enabled = Boolean(spec) && task.scheduleEnabled !== false
  return { enabled, spec: enabled ? spec : null }
}

const weekdayKeys = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'] as const

export function formatSchedule(spec: ScheduleSpec | null | undefined, t: Locale): string {
  const labels = t.vulnScan.newScan.alarmSchedule
  if (!spec) return labels.off
  const kindLabels: Record<ScheduleKind, string> = {
    once: labels.once,
    daily: labels.daily,
    weekdays: labels.workdays,
    weekly: labels.weekly,
    monthly: labels.monthly,
    cron: labels.custom,
  }
  const detail = spec.kind === 'once'
    ? spec.date || ''
    : spec.kind === 'weekly'
      ? (spec.weekdays || []).map(day => t.vulnScan.newScan.weekdays[weekdayKeys[day - 1]]).filter(Boolean).join(' / ')
      : spec.kind === 'monthly'
        ? spec.dayOfMonth === -1 ? labels.lastDay : `${spec.dayOfMonth} ${t.vulnScan.newScan.scheduleDayUnit}`
        : ''
  const rule = spec.kind === 'cron' ? (spec.cron || []).join(' / ') : (spec.times || []).join(' / ')
  return [kindLabels[spec.kind], detail, rule].filter(Boolean).join(' · ')
}

// Runs are serialized with the server's UTC offset. Display that wall-clock
// time even when the browser is in another zone, without exposing a per-plan
// timezone setting.
function serverWallTime(value: string): { date: Date; offset: number; hasOffset: boolean } {
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return { date: parsed, offset: 0, hasOffset: false }
  const utc = value.endsWith('Z')
  const match = /([+-])(\d{2}):(\d{2})$/.exec(value)
  if (!utc && !match) return { date: parsed, offset: -parsed.getTimezoneOffset(), hasOffset: false }
  const offset = utc ? 0 : (match![1] === '-' ? -1 : 1) * (Number(match![2]) * 60 + Number(match![3]))
  return { date: new Date(parsed.getTime() + offset * 60_000), offset, hasOffset: true }
}

export function formatRunLabel(value: string, t: Locale): string {
  const labels = t.vulnScan.newScan.alarmSchedule
  const { date, offset, hasOffset } = serverWallTime(value)
  if (Number.isNaN(date.getTime())) return value
  const now = hasOffset ? new Date(Date.now() + offset * 60_000) : new Date()
  const parts = (d: Date) => hasOffset
    ? { year: d.getUTCFullYear(), month: d.getUTCMonth(), day: d.getUTCDate(), hours: d.getUTCHours(), minutes: d.getUTCMinutes(), weekday: d.getUTCDay() }
    : { year: d.getFullYear(), month: d.getMonth(), day: d.getDate(), hours: d.getHours(), minutes: d.getMinutes(), weekday: d.getDay() }
  const current = parts(date)
  const today = parts(now)
  const time = `${String(current.hours).padStart(2, '0')}:${String(current.minutes).padStart(2, '0')}`
  const startOfDay = (p: ReturnType<typeof parts>) => Date.UTC(p.year, p.month, p.day)
  const diff = Math.round((startOfDay(current) - startOfDay(today)) / 86_400_000)
  if (diff === 0) return `${labels.today} ${time}`
  if (diff === 1) return `${labels.tomorrow} ${time}`
  if (diff === 2) return `${labels.dayAfterTomorrow} ${time}`
  if (diff > 2 && diff < 7) {
    const key = weekdayKeys[(current.weekday + 6) % 7]
    return `${t.vulnScan.newScan.weekdays[key]} ${time}`
  }
  return `${current.month + 1}/${current.day} ${time}`
}

export function formatScheduleTimestamp(value: string, locale?: string): string {
  const { date, hasOffset } = serverWallTime(value)
  return date.toLocaleString(locale || 'zh-CN', hasOffset ? { timeZone: 'UTC' } : undefined)
}
