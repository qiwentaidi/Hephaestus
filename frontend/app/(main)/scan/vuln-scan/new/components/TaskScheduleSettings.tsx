'use client'

import React from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { useI18n } from '@/lib/i18n'
import type { ScheduleConfig, ScheduleSpec } from '../../types'
import { createDefaultSpec } from '../../utils/task-schedule'
import { ScheduleSpecFields, ScheduleRunPreview } from './ScheduleSpecFields'

interface TaskScheduleSettingsProps {
  value: ScheduleConfig
  onChange: (value: ScheduleConfig) => void
  disabled?: boolean
}

export function TaskScheduleSettings({ value, onChange, disabled = false }: TaskScheduleSettingsProps) {
  const { t } = useI18n()
  const a = t.vulnScan.newScan.alarmSchedule
  const spec = value.spec
  const enabled = Boolean(spec) && value.enabled
  const custom = spec?.kind === 'cron'
  const basicSpec = React.useRef<ScheduleSpec | null>(null)
  const id = React.useId()

  const updateSpec = (patch: Partial<ScheduleSpec>) => {
    const current = spec ?? createDefaultSpec()
    onChange({ ...value, spec: { ...current, ...patch } })
  }
  const toggleEnabled = (next: boolean) => {
    onChange(next
      ? { enabled: true, spec: spec ?? createDefaultSpec() }
      : { enabled: false, spec: null })
  }
  const toggleCron = (useCron: boolean) => {
    if (useCron) {
      basicSpec.current = spec
      onChange({ ...value, enabled: true, spec: { kind: 'cron', cron: spec && spec.kind === 'cron' ? spec.cron || [] : [] } })
    } else {
      const next = basicSpec.current && basicSpec.current.kind !== 'cron' ? basicSpec.current : createDefaultSpec()
      onChange({ ...value, spec: next })
    }
  }

  return (
    <Card className="gap-3 border-border/70 bg-card/80 py-4 shadow-sm">
      <CardHeader className="flex flex-row items-center justify-between gap-3 px-5">
        <CardTitle className="text-base"><label htmlFor={`${id}-enabled`}>{a.plan}</label></CardTitle>
        <Switch id={`${id}-enabled`} aria-label={a.enabled} checked={enabled} disabled={disabled} onCheckedChange={toggleEnabled} />
      </CardHeader>
      <CardContent className="space-y-3 px-5">
        {!enabled ? (
          <p className="text-sm text-muted-foreground">{a.notEnabled}</p>
        ) : (
          <>
            {custom ? (
              <div className="space-y-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <Label htmlFor={`${id}-cron`} className="text-xs text-muted-foreground">{t.vulnScan.newScan.scheduleCronExpression}</Label>
                  <span className="text-xs text-muted-foreground">{a.cronHintShort}</span>
                </div>
                <Textarea
                  id={`${id}-cron`}
                  value={(spec?.cron || []).join('\n')}
                  onChange={event => updateSpec({ kind: 'cron', cron: event.target.value.split('\n') })}
                  disabled={disabled || !enabled}
                  placeholder={'0 2 * * *'}
                  rows={3}
                  className="font-mono text-sm"
                />
              </div>
            ) : spec ? (
              <ScheduleSpecFields value={spec} disabled={disabled || !enabled} onChange={updateSpec} />
            ) : null}
            <ScheduleRunPreview value={value} />
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p className="text-xs leading-5 text-muted-foreground">{a.policy}{spec?.kind === 'once' ? ` ${a.onceHint}` : ''}</p>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-8 px-2 text-xs text-muted-foreground"
                disabled={disabled || !enabled}
                onClick={() => toggleCron(!custom)}
              >
                {custom ? a.backToVisual : a.cronSwitch}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
