'use client'

import { useRef, useState } from 'react'
import { AlertTriangle, FileJson2, Loader2, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useI18n } from '@/lib/i18n'
import { importCustomThemes, ThemeApiError } from '@/lib/api/themes'
import {
  parseThemeLibraryBundle,
  themeNameKey,
  ThemeValidationError,
  type ThemeLibraryEntry,
} from '@/lib/theme/custom-theme'
import { cn } from '@/lib/utils'
import toast from '@/lib/toast'
import { ThemePreview } from './ThemePreview'

const MAX_IMPORT_FILE_BYTES = 1024 * 1024

interface CustomThemeImportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  existingNames: string[]
  onImported: (count: number) => void
}

export function CustomThemeImportDialog({
  open,
  onOpenChange,
  existingNames,
  onImported,
}: CustomThemeImportDialogProps) {
  const { t } = useI18n()
  const copy = t.theme.customThemes.importDialog
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const [fileName, setFileName] = useState('')
  const [entries, setEntries] = useState<ThemeLibraryEntry[]>([])
  const [conflicts, setConflicts] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const [dragActive, setDragActive] = useState(false)
  const [importing, setImporting] = useState(false)

  const existingKeys = new Set(existingNames.map((name) => themeNameKey(name)))

  const applyBundle = (raw: string, name: string) => {
    setError(null)
    setConflicts([])
    setEntries([])
    setFileName(name)

    let parsed: unknown
    try {
      parsed = JSON.parse(raw)
    } catch {
      setError(copy.invalidFile)
      return
    }

    try {
      const bundle = parseThemeLibraryBundle(parsed)
      const duplicatedWithLibrary = bundle.themes
        .filter((entry) => existingKeys.has(themeNameKey(entry.name)))
        .map((entry) => entry.name)

      setEntries(bundle.themes)
      setConflicts(duplicatedWithLibrary)
    } catch (error) {
      if (error instanceof ThemeValidationError) {
        if (error.code === 'unsupported_version') {
          setError(copy.unsupportedVersion)
        } else if (error.code === 'duplicate_name') {
          setError(copy.duplicateInFile.replace('{names}', error.conflicts.join(', ')))
        } else {
          setError(copy.invalidFile)
        }
      } else {
        setError(copy.invalidFile)
      }
    }
  }

  const readFile = async (file: File) => {
    if (file.size > MAX_IMPORT_FILE_BYTES) {
      setError(copy.tooLarge)
      setFileName(file.name)
      setEntries([])
      setConflicts([])
      return
    }
    const raw = await file.text()
    applyBundle(raw, file.name)
  }

  const onDrop = (event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault()
    setDragActive(false)
    const file = event.dataTransfer.files?.[0]
    if (file) void readFile(file)
  }

  const runImport = async () => {
    if (entries.length === 0 || conflicts.length > 0) return
    setImporting(true)
    setError(null)
    try {
      const created = await importCustomThemes(entries)
      toast.success(copy.success.replace('{count}', String(created)))
      onImported(created)
      onOpenChange(false)
    } catch (error) {
      if (error instanceof ThemeApiError) {
        if (error.code === 'duplicate_name') {
          setConflicts(error.conflicts.length > 0 ? error.conflicts : entries.map((entry) => entry.name))
          setError(copy.conflicts)
        } else {
          setError(copy.failed)
        }
      } else {
        setError(copy.failed)
      }
    } finally {
      setImporting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="settings-dialog flex max-h-[calc(100dvh-2rem)] w-[min(760px,calc(100vw-2rem))] flex-col gap-0 overflow-hidden p-0 sm:!max-w-[760px]">
        <DialogHeader className="border-b border-border px-5 py-4">
          <DialogTitle className="flex items-center gap-2 text-base">
            <FileJson2 className="h-4 w-4 text-primary" />
            {copy.title}
          </DialogTitle>
          <DialogDescription>{copy.description}</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <input
            ref={fileInputRef}
            type="file"
            accept=".json,application/json"
            className="hidden"
            onChange={(event) => {
              const file = event.target.files?.[0]
              if (file) void readFile(file)
              event.target.value = ''
            }}
          />

          <div
            role="button"
            tabIndex={0}
            onClick={() => fileInputRef.current?.click()}
            onKeyDown={(event) => {
              if (event.key === 'Enter' || event.key === ' ') {
                event.preventDefault()
                fileInputRef.current?.click()
              }
            }}
            onDragOver={(event) => {
              event.preventDefault()
              setDragActive(true)
            }}
            onDragLeave={() => setDragActive(false)}
            onDrop={onDrop}
            className={cn(
              'flex cursor-pointer flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-4 py-8 text-center transition-colors',
              dragActive ? 'border-primary bg-primary/5' : 'border-border bg-muted/20 hover:border-primary/50',
            )}
          >
            <Upload className="h-6 w-6 text-muted-foreground" />
            <div className="text-sm font-medium">{copy.chooseFile}</div>
            <div className="text-xs text-muted-foreground">{copy.dropHint}</div>
            {fileName && <div className="mt-1 text-xs text-muted-foreground">{fileName}</div>}
          </div>

          {error && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {conflicts.length > 0 && (
            <div className="mt-3 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
              <div className="font-medium">{copy.conflicts}</div>
              <div className="mt-1 break-words">{conflicts.join('、')}</div>
            </div>
          )}

          {entries.length > 0 && (
            <div className="mt-4 space-y-2">
              <div className="flex items-center justify-between">
                <div className="text-sm font-semibold">{copy.preview}</div>
                <div className="text-xs text-muted-foreground">
                  {t.theme.customThemes.count.replace('{count}', String(entries.length))}
                </div>
              </div>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                {entries.map((entry) => (
                  <div
                    key={themeNameKey(entry.name)}
                    className={cn(
                      'rounded-lg border bg-card p-2',
                      conflicts.some((name) => themeNameKey(name) === themeNameKey(entry.name))
                        ? 'border-destructive/50'
                        : 'border-border',
                    )}
                  >
                    <ThemePreview definition={entry.definition} variant="compact" />
                    <div className="mt-1.5 truncate text-xs font-medium">{entry.name}</div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <DialogFooter className="border-t border-border px-5 py-3">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={importing}>
            {copy.close}
          </Button>
          <Button
            type="button"
            onClick={() => void runImport()}
            disabled={importing || entries.length === 0 || conflicts.length > 0}
          >
            {importing && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            {importing
              ? copy.importing
              : copy.importButton.replace('{count}', String(entries.length))}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
