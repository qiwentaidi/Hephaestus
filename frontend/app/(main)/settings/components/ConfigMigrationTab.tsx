'use client'

import { useMemo, useRef, useState } from 'react'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  AlertTriangle,
  Bell,
  Bot,
  CheckCircle2,
  Download,
  FileJson2,
  FileText,
  Key,
  Loader2,
  ShieldCheck,
  Upload,
  X,
} from 'lucide-react'
import { useI18n, type Locale } from '@/lib/i18n'
import { saveBlobAsFile } from '@/lib/download'
import toast from '@/lib/toast'
import { cn } from '@/lib/utils'
import {
  CONFIG_MIGRATION_MODULES,
  countConfigMigrationModuleItems,
  exportConfigMigration,
  importConfigMigration,
  parseConfigMigrationBundle,
  type ConfigMigrationBundle,
  type ConfigMigrationImportResult,
  type ConfigMigrationModuleId,
} from '@/lib/api/config-migration'

type ModuleSelection = Record<ConfigMigrationModuleId, boolean>

const MAX_IMPORT_FILE_BYTES = 8 * 1024 * 1024

function allSelected(): ModuleSelection {
  return { ai: true, apiKeys: true, scanTemplates: true, notifications: true }
}

function noneSelected(): ModuleSelection {
  return { ai: false, apiKeys: false, scanTemplates: false, notifications: false }
}

function formatBytes(size: number): string {
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / (1024 * 1024)).toFixed(1)} MB`
}

function describeError(error: unknown, t: Locale): string {
  if (error instanceof SyntaxError) {
    return t.configMigration.errors.invalidBundle
  }
  const code = error instanceof Error ? error.message : ''
  switch (code) {
    case 'invalid_bundle':
      return t.configMigration.errors.invalidBundle
    case 'unsupported_version':
      return t.configMigration.errors.unsupportedVersion
    case 'empty_bundle':
      return t.configMigration.errors.emptyBundle
    case 'export_failed':
      return t.configMigration.errors.exportFailed
    case 'import_failed':
      return t.configMigration.errors.importFailed
    default:
      return code || t.configMigration.errors.unknown
  }
}

export function ConfigMigrationTab() {
  const { t } = useI18n()
  const fileInputRef = useRef<HTMLInputElement | null>(null)

  const [exportSelection, setExportSelection] = useState<ModuleSelection>(allSelected)
  const [exporting, setExporting] = useState(false)

  const [importBundle, setImportBundle] = useState<ConfigMigrationBundle | null>(null)
  const [importFileName, setImportFileName] = useState('')
  const [importFileSize, setImportFileSize] = useState(0)
  const [importSelection, setImportSelection] = useState<ModuleSelection>(noneSelected)
  const [importing, setImporting] = useState(false)
  const [dragActive, setDragActive] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [importResult, setImportResult] = useState<ConfigMigrationImportResult | null>(null)

  const moduleOptions = useMemo(
    () => [
      {
        id: 'ai' as const,
        icon: Bot,
        label: t.configMigration.modules.ai,
        description: t.configMigration.modules.aiDescription,
      },
      {
        id: 'apiKeys' as const,
        icon: Key,
        label: t.configMigration.modules.apiKeys,
        description: t.configMigration.modules.apiKeysDescription,
      },
      {
        id: 'scanTemplates' as const,
        icon: FileText,
        label: t.configMigration.modules.scanTemplates,
        description: t.configMigration.modules.scanTemplatesDescription,
      },
      {
        id: 'notifications' as const,
        icon: Bell,
        label: t.configMigration.modules.notifications,
        description: t.configMigration.modules.notificationsDescription,
      },
    ],
    [t],
  )

  const selectedExportIds = CONFIG_MIGRATION_MODULES.filter((id) => exportSelection[id])
  const selectedImportIds = importBundle
    ? CONFIG_MIGRATION_MODULES.filter((id) => importSelection[id] && importBundle.modules.some((module) => module.id === id))
    : []

  const toggleExportModule = (id: ConfigMigrationModuleId, checked: boolean) => {
    setExportSelection((current) => ({ ...current, [id]: checked }))
  }

  const toggleImportModule = (id: ConfigMigrationModuleId, checked: boolean) => {
    setImportSelection((current) => ({ ...current, [id]: checked }))
  }

  const toggleAllExport = () => {
    const shouldSelectAll = selectedExportIds.length !== CONFIG_MIGRATION_MODULES.length
    setExportSelection(shouldSelectAll ? allSelected() : noneSelected())
  }

  const handleExport = async () => {
    if (selectedExportIds.length === 0) {
      toast.error(t.configMigration.selectAtLeastOne)
      return
    }
    setExporting(true)
    try {
      const bundle = await exportConfigMigration(selectedExportIds)
      const content = JSON.stringify(bundle, null, 2)
      const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')
      const outcome = await saveBlobAsFile(
        new Blob([content], { type: 'application/json;charset=utf-8' }),
        `hephaestus-config-${stamp}.json`,
      )
      if (outcome === 'saved') {
        toast.success(t.configMigration.exportSuccess)
      }
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setExporting(false)
    }
  }

  const loadFile = async (file: File) => {
    setImportResult(null)
    if (file.size > MAX_IMPORT_FILE_BYTES) {
      toast.error(t.configMigration.fileTooLarge)
      return
    }
    try {
      const parsed = JSON.parse(await file.text())
      const bundle = parseConfigMigrationBundle(parsed)
      const selection = noneSelected()
      for (const moduleItem of bundle.modules) {
        selection[moduleItem.id] = true
      }
      setImportBundle(bundle)
      setImportFileName(file.name)
      setImportFileSize(file.size)
      setImportSelection(selection)
    } catch (error) {
      setImportBundle(null)
      setImportFileName('')
      setImportFileSize(0)
      toast.error(describeError(error, t))
    }
  }

  const resetImport = () => {
    setImportBundle(null)
    setImportFileName('')
    setImportFileSize(0)
    setImportSelection(noneSelected())
    setImportResult(null)
    if (fileInputRef.current) {
      fileInputRef.current.value = ''
    }
  }

  const handleImport = async () => {
    if (!importBundle) return
    if (selectedImportIds.length === 0) {
      toast.error(t.configMigration.selectAtLeastOne)
      return
    }
    setImporting(true)
    try {
      const result = await importConfigMigration(importBundle, selectedImportIds)
      setImportResult(result)
      setConfirmOpen(false)
      toast.success(t.configMigration.importSuccess)
    } catch (error) {
      toast.error(describeError(error, t))
    } finally {
      setImporting(false)
    }
  }

  const resultText = (id: ConfigMigrationModuleId, applied: number, created = 0, updated = 0) => {
    if (id === 'scanTemplates') {
      return t.configMigration.resultTemplates
        .replace('{created}', String(created))
        .replace('{updated}', String(updated))
    }
    return t.configMigration.resultApplied.replace('{count}', String(applied))
  }

  return (
    <div className="mx-auto w-full max-w-3xl space-y-6">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold tracking-tight text-foreground">{t.configMigration.title}</h2>
        <p className="max-w-2xl text-sm leading-6 text-muted-foreground">{t.configMigration.description}</p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Download className="size-4 text-primary" />
            {t.configMigration.exportTitle}
          </CardTitle>
          <CardDescription>{t.configMigration.exportDescription}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center justify-between gap-3">
            <span className="text-xs font-medium text-muted-foreground">
              {t.configMigration.selectedModules.replace('{count}', String(selectedExportIds.length))}
            </span>
            <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-xs" onClick={toggleAllExport}>
              {selectedExportIds.length === CONFIG_MIGRATION_MODULES.length
                ? t.configMigration.clearAll
                : t.configMigration.selectAll}
            </Button>
          </div>

          <div className="grid gap-2 sm:grid-cols-2">
            {moduleOptions.map((option) => {
              const Icon = option.icon
              const selected = exportSelection[option.id]
              return (
                <label
                  key={option.id}
                  htmlFor={`config-export-${option.id}`}
                  className={cn(
                    'flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors',
                    selected ? 'border-primary/40 bg-primary/5' : 'border-border hover:bg-muted/40',
                  )}
                >
                  <Checkbox
                    id={`config-export-${option.id}`}
                    checked={selected}
                    onCheckedChange={(checked) => toggleExportModule(option.id, checked === true)}
                    className="mt-0.5"
                  />
                  <span className="min-w-0 space-y-0.5">
                    <span className="flex items-center gap-2 text-sm font-medium">
                      <Icon className="size-4 text-muted-foreground" />
                      {option.label}
                    </span>
                    <span className="block text-xs leading-5 text-muted-foreground">{option.description}</span>
                  </span>
                </label>
              )
            })}
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border/70 pt-3">
            <p className="flex items-start gap-1.5 text-xs leading-5 text-muted-foreground">
              <ShieldCheck className="mt-0.5 size-3.5 shrink-0 text-primary" />
              {t.configMigration.encryptionHint}
            </p>
            <Button type="button" onClick={() => void handleExport()} disabled={exporting || selectedExportIds.length === 0}>
              {exporting ? <Loader2 className="mr-2 size-4 animate-spin" /> : <Download className="mr-2 size-4" />}
              {t.configMigration.exportButton}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Upload className="size-4 text-primary" />
            {t.configMigration.importTitle}
          </CardTitle>
          <CardDescription>{t.configMigration.importDescription}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) void loadFile(file)
              event.target.value = ''
            }}
          />

          {!importBundle ? (
            <button
              type="button"
              onClick={() => fileInputRef.current?.click()}
              onDragOver={(event) => {
                event.preventDefault()
                setDragActive(true)
              }}
              onDragLeave={() => setDragActive(false)}
              onDrop={(event) => {
                event.preventDefault()
                setDragActive(false)
                const file = event.dataTransfer.files?.[0]
                if (file) void loadFile(file)
              }}
              className={cn(
                'flex w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center transition-colors',
                dragActive ? 'border-primary/60 bg-primary/5' : 'border-border hover:border-primary/40 hover:bg-muted/30',
              )}
            >
              <span className="flex size-10 items-center justify-center rounded-full bg-primary/10 text-primary">
                <Upload className="size-5" />
              </span>
              <span className="text-sm font-medium">{t.configMigration.chooseFile}</span>
              <span className="text-xs text-muted-foreground">{t.configMigration.dropHint}</span>
            </button>
          ) : (
            <div className="space-y-4">
              <div className="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-muted/30 px-3 py-2.5">
                <div className="flex min-w-0 items-center gap-2">
                  <FileJson2 className="size-4 shrink-0 text-primary" />
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{importFileName}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatBytes(importFileSize)}
                      {importBundle.appVersion ? ` · ${t.configMigration.sourceVersion.replace('{version}', importBundle.appVersion)}` : ''}
                      {importBundle.exportedAt
                        ? ` · ${t.configMigration.exportedAt.replace('{time}', new Date(importBundle.exportedAt).toLocaleString())}`
                        : ''}
                    </p>
                  </div>
                </div>
                <Button type="button" variant="ghost" size="sm" onClick={resetImport} disabled={importing}>
                  <X className="mr-1.5 size-4" />
                  {t.configMigration.removeFile}
                </Button>
              </div>

              <div className="grid gap-2 sm:grid-cols-2">
                {moduleOptions.map((option) => {
                  const moduleItem = importBundle.modules.find((item) => item.id === option.id)
                  if (!moduleItem) return null
                  const Icon = option.icon
                  const selected = importSelection[option.id]
                  const count = countConfigMigrationModuleItems(option.id, moduleItem.data)
                  return (
                    <label
                      key={option.id}
                      htmlFor={`config-import-${option.id}`}
                      className={cn(
                        'flex cursor-pointer items-start gap-3 rounded-md border p-3 transition-colors',
                        selected ? 'border-primary/40 bg-primary/5' : 'border-border hover:bg-muted/40',
                      )}
                    >
                      <Checkbox
                        id={`config-import-${option.id}`}
                        checked={selected}
                        onCheckedChange={(checked) => toggleImportModule(option.id, checked === true)}
                        className="mt-0.5"
                      />
                      <span className="min-w-0 flex-1 space-y-0.5">
                        <span className="flex items-center justify-between gap-2 text-sm font-medium">
                          <span className="flex items-center gap-2">
                            <Icon className="size-4 text-muted-foreground" />
                            {option.label}
                          </span>
                          <Badge variant="secondary" className="shrink-0 font-normal">
                            {t.configMigration.itemCount.replace('{count}', String(count))}
                          </Badge>
                        </span>
                        <span className="block text-xs leading-5 text-muted-foreground">{option.description}</span>
                      </span>
                    </label>
                  )
                })}
              </div>

              <div className="flex items-start gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2.5 text-xs leading-5 text-amber-700 dark:text-amber-300">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <p>
                  <span className="font-medium">{t.configMigration.warningTitle}</span> {t.configMigration.warningText}
                </p>
              </div>

              <div className="flex flex-wrap items-center justify-end gap-2">
                <Button type="button" variant="outline" onClick={resetImport} disabled={importing}>
                  {t.common.cancel}
                </Button>
                <Button
                  type="button"
                  onClick={() => setConfirmOpen(true)}
                  disabled={importing || selectedImportIds.length === 0}
                >
                  {importing ? <Loader2 className="mr-2 size-4 animate-spin" /> : <Upload className="mr-2 size-4" />}
                  {t.configMigration.importButton}
                </Button>
              </div>

              {importResult ? (
                <div className="space-y-2 rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3 py-3">
                  <p className="flex items-center gap-2 text-sm font-medium text-emerald-700 dark:text-emerald-300">
                    <CheckCircle2 className="size-4" />
                    {t.configMigration.resultTitle}
                  </p>
                  <ul className="space-y-1 pl-6 text-xs text-emerald-700/90 dark:text-emerald-300/90">
                    {importResult.modules.map((module) => (
                      <li key={module.id} className="list-disc">
                        {moduleOptions.find((option) => option.id === module.id)?.label || module.id}
                        {': '}
                        {resultText(module.id, module.applied, module.created, module.updated)}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
            </div>
          )}
        </CardContent>
      </Card>

      <AlertDialog open={confirmOpen} onOpenChange={(open) => !importing && setConfirmOpen(open)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.configMigration.confirmTitle}</AlertDialogTitle>
            <AlertDialogDescription>{t.configMigration.confirmDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={importing}>{t.common.cancel}</AlertDialogCancel>
            <AlertDialogAction
              disabled={importing}
              onClick={(event) => {
                event.preventDefault()
                void handleImport()
              }}
            >
              {importing ? <Loader2 className="mr-2 size-4 animate-spin" /> : null}
              {t.configMigration.confirmAction}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
