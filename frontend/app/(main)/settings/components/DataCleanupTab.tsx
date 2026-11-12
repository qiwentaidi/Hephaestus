'use client'

import { useEffect, useRef, useState } from 'react'
import {
  AlertTriangle,
  Bot,
  Boxes,
  Check,
  CheckCircle2,
  FolderX,
  History,
  Loader2,
  MessageSquare,
  Palette,
  RefreshCw,
  ScanLine,
  Server,
  Settings,
  ShieldCheck,
  Trash2,
  User,
  X,
} from 'lucide-react'
import {
  Alert,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'
import {
  DataPurgeError,
  purgeData,
  type DataPurgeActiveTask,
  type DataPurgeResult,
} from '@/lib/api/data-cleanup'

const KEEP_KEYS = ['settings', 'account', 'appearance', 'prompts', 'infrastructure'] as const
const CLEAR_KEYS = ['tasks', 'assets', 'ai', 'workspace', 'files'] as const

const KEEP_ICONS = {
  settings: Settings,
  account: User,
  appearance: Palette,
  prompts: Bot,
  infrastructure: Server,
}

const CLEAR_ICONS = {
  tasks: ScanLine,
  assets: Boxes,
  ai: MessageSquare,
  workspace: History,
  files: FolderX,
}

export function DataCleanupTab() {
  const { t, language } = useI18n()
  const copy = t.dataCleanup

  const [includeLogs, setIncludeLogs] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [confirmation, setConfirmation] = useState('')
  const [running, setRunning] = useState(false)
  const [activeTasks, setActiveTasks] = useState<DataPurgeActiveTask[]>([])
  const [errorMessage, setErrorMessage] = useState('')
  const [result, setResult] = useState<DataPurgeResult | null>(null)
  const reloadTimer = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (reloadTimer.current !== null) {
        window.clearTimeout(reloadTimer.current)
      }
    }
  }, [])

  const confirmed =
    language === 'en'
      ? confirmation.trim().toUpperCase() === copy.confirmPhrase.toUpperCase()
      : confirmation.trim() === copy.confirmPhrase

  const resetDialog = () => {
    setConfirmation('')
    setActiveTasks([])
    setErrorMessage('')
  }

  const openDialog = () => {
    resetDialog()
    setDialogOpen(true)
  }

  const handleDialogChange = (open: boolean) => {
    if (running) return
    setDialogOpen(open)
    if (!open) resetDialog()
  }

  const activeTaskText = (tasks: DataPurgeActiveTask[]) => {
    const kinds = copy.activeTaskKinds as Record<string, string>
    return tasks
      .filter((task) => task.count > 0)
      .map((task) => `${kinds[task.kind] || task.kind} ×${task.count}`)
      .join(language === 'zh' ? '、' : ', ')
  }

  const warningText = (code: string) => {
    const messages = copy.warnings as Record<string, string>
    return messages[code] || code
  }

  const handlePurge = async () => {
    if (!confirmed || running) return
    setRunning(true)
    setErrorMessage('')
    setActiveTasks([])
    setResult(null)
    try {
      const data = await purgeData(copy.confirmPhrase, includeLogs)
      setResult(data)
      setDialogOpen(false)
      resetDialog()
      toast.success(copy.successTitle)
      if (data.warnings.length === 0) {
        reloadTimer.current = window.setTimeout(() => window.location.reload(), 1200)
      }
    } catch (error) {
      if (error instanceof DataPurgeError) {
        if (error.code === 'active_tasks') {
          setActiveTasks(error.activeTasks)
        } else {
          const errors = copy.errors as Record<string, string>
          setErrorMessage(errors[error.code] || copy.errors.purge_failed)
        }
      } else {
        setErrorMessage(copy.errors.request_failed)
      }
    } finally {
      setRunning(false)
    }
  }

  const refreshNow = () => {
    if (reloadTimer.current !== null) {
      window.clearTimeout(reloadTimer.current)
      reloadTimer.current = null
    }
    window.location.reload()
  }

  return (
    <div className="mx-auto w-full max-w-4xl space-y-5">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold tracking-tight text-foreground">{copy.title}</h2>
        <p className="max-w-2xl text-sm leading-6 text-muted-foreground">{copy.description}</p>
      </header>

      <div className="flex items-start gap-2.5 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3.5 py-3 text-xs leading-5 text-amber-700 dark:text-amber-300">
        <AlertTriangle className="mt-0.5 size-4 shrink-0" />
        <p>
          <span className="font-semibold">{copy.warningTitle}</span> {copy.warningText}
        </p>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600 dark:text-emerald-400">
                <ShieldCheck className="size-4" />
              </span>
              <span className="space-y-0.5">
                <span className="block">{copy.keepTitle}</span>
                <span className="block text-xs font-normal text-muted-foreground">{copy.keepDescription}</span>
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="divide-y divide-border/70">
              {KEEP_KEYS.map((key) => {
                const Icon = KEEP_ICONS[key]
                return (
                  <li key={key} className="flex gap-3 py-2.5 first:pt-0 last:pb-0">
                    <Icon className="mt-0.5 size-4 shrink-0 text-emerald-600 dark:text-emerald-400" />
                    <div className="min-w-0 space-y-0.5">
                      <p className="text-sm font-medium text-foreground">{copy.keep[key].title}</p>
                      <p className="text-xs leading-5 text-muted-foreground">{copy.keep[key].description}</p>
                    </div>
                  </li>
                )
              })}
            </ul>
          </CardContent>
        </Card>

        <Card className="border-destructive/25">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <span className="flex size-8 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
                <Trash2 className="size-4" />
              </span>
              <span className="space-y-0.5">
                <span className="block">{copy.clearTitle}</span>
                <span className="block text-xs font-normal text-muted-foreground">{copy.clearDescription}</span>
              </span>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="divide-y divide-border/70">
              {CLEAR_KEYS.map((key) => {
                const Icon = CLEAR_ICONS[key]
                return (
                  <li key={key} className="flex gap-3 py-2.5 first:pt-0 last:pb-0">
                    <Icon className="mt-0.5 size-4 shrink-0 text-destructive" />
                    <div className="min-w-0 space-y-0.5">
                      <p className="text-sm font-medium text-foreground">{copy.clear[key].title}</p>
                      <p className="text-xs leading-5 text-muted-foreground">{copy.clear[key].description}</p>
                    </div>
                  </li>
                )
              })}
            </ul>
          </CardContent>
        </Card>
      </div>

      <div className="rounded-lg border border-border bg-card px-4 py-3">
        <label htmlFor="data-cleanup-logs" className="flex cursor-pointer items-start gap-3">
          <Checkbox
            id="data-cleanup-logs"
            checked={includeLogs}
            onCheckedChange={(checked) => setIncludeLogs(checked === true)}
            className="mt-0.5"
          />
          <span className="min-w-0 space-y-0.5">
            <span className="block text-sm font-medium text-foreground">{copy.includeLogs}</span>
            <span className="block text-xs leading-5 text-muted-foreground">{copy.includeLogsDescription}</span>
          </span>
        </label>
      </div>

      {result && result.warnings.length > 0 ? (
        <div className="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3.5 py-3 text-amber-700 dark:text-amber-300">
          <p className="flex items-center gap-2 text-sm font-medium">
            <AlertTriangle className="size-4" />
            {copy.partialTitle}
          </p>
          <p className="mt-1 text-xs leading-5">
            {copy.successDescription.replace(
              '{tables}',
              String(result.clearedTables + result.clearedOrchestrationTables),
            )}
          </p>
          <ul className="mt-2 space-y-1 pl-6 text-xs leading-5">
            {result.warnings.map((code) => (
              <li key={code} className="list-disc">
                {warningText(code)}
              </li>
            ))}
          </ul>
          <Button type="button" variant="outline" size="sm" className="mt-3" onClick={refreshNow}>
            <RefreshCw className="mr-1.5 size-3.5" />
            {copy.refresh}
          </Button>
        </div>
      ) : null}

      <div className="flex flex-col-reverse gap-3 border-t border-border/70 pt-4 sm:flex-row sm:items-center sm:justify-between">
        <p className="flex min-w-0 flex-1 items-start gap-1.5 text-xs leading-5 text-muted-foreground">
          <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-primary" />
          {copy.scopeNote}
        </p>
        <Button type="button" variant="destructive" className="w-full shrink-0 sm:w-auto" onClick={openDialog}>
          <Trash2 className="mr-2 size-4" />
          {copy.action}
        </Button>
      </div>

      <Dialog open={dialogOpen} onOpenChange={handleDialogChange}>
        <DialogContent showCloseButton={!running} className="sm:max-w-lg">
          <DialogHeader>
            <div className="mb-1 flex size-10 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
              <AlertTriangle className="size-5" />
            </div>
            <DialogTitle>{copy.dialogTitle}</DialogTitle>
            <DialogDescription className="space-y-2 leading-5">
              <span className="block">{copy.dialogDescription}</span>
              <span className="block rounded-md border border-emerald-500/25 bg-emerald-500/10 px-2.5 py-2 text-xs text-emerald-700 dark:text-emerald-300">
                {copy.dialogKeep}
              </span>
            </DialogDescription>
          </DialogHeader>

          {activeTasks.length > 0 ? (
            <Alert variant="destructive">
              <AlertTriangle />
              <AlertTitle>{copy.activeTitle}</AlertTitle>
              <AlertDescription>
                {copy.activeDescription.replace('{tasks}', activeTaskText(activeTasks))}
              </AlertDescription>
            </Alert>
          ) : null}
          {errorMessage ? (
            <p className="text-sm text-destructive" role="alert">
              {errorMessage}
            </p>
          ) : null}

          <div className="space-y-2">
            <label htmlFor="data-cleanup-confirmation" className="block text-sm font-medium">
              {copy.confirmLabel.replace('{phrase}', copy.confirmPhrase)}
            </label>
            <Input
              id="data-cleanup-confirmation"
              value={confirmation}
              onChange={(event) => setConfirmation(event.target.value)}
              autoComplete="off"
              spellCheck={false}
              autoFocus
              disabled={running}
              aria-invalid={confirmation.length > 0 && !confirmed}
            />
            <p className="text-xs leading-5 text-muted-foreground">{copy.confirmHint}</p>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleDialogChange(false)}
              disabled={running}
            >
              {t.common.cancel}
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={() => void handlePurge()}
              disabled={!confirmed || running}
            >
              {running ? (
                <Loader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <Check className="mr-2 size-4" />
              )}
              {running ? copy.purging : copy.confirmAction}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
