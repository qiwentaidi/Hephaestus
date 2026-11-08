'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { FileText, RefreshCw, Search, SlidersHorizontal, WrapText } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription, SheetFooter, SheetClose } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

interface LogFileItem {
  name: string
  size: number
  modifiedAt: string
}

interface LogFilesPayload {
  dir: string
  files: LogFileItem[]
}

interface LogReadPayload {
  file: string
  path: string
  size: number
  lines: string[]
  truncated: boolean
  maxLines: number
  suggestDirectRead: boolean
}

const LINE_OPTIONS = ['200', '500', '1000']
const LEVEL_OPTIONS = ['ALL', 'DEBUG', 'INFO', 'WARNING', 'ERROR', 'VULN'] as const

export function BackendLogsTab() {
  const { t } = useI18n()
  const i18n = t.settings.logs
  const [filtersOpen, setFiltersOpen] = useState(false)
  const [wrapLines, setWrapLines] = useState(true)
  const [logsDir, setLogsDir] = useState('')
  const [files, setFiles] = useState<LogFileItem[]>([])
  const [selectedFile, setSelectedFile] = useState('')
  const [selectedLines, setSelectedLines] = useState('200')
  const [selectedLevel, setSelectedLevel] = useState<(typeof LEVEL_OPTIONS)[number]>('ALL')
  const [keyword, setKeyword] = useState('')
  const [appliedKeyword, setAppliedKeyword] = useState('')
  const [logPayload, setLogPayload] = useState<LogReadPayload | null>(null)
  const [loadingFiles, setLoadingFiles] = useState(false)
  const [loadingContent, setLoadingContent] = useState(false)

  const selectedMeta = useMemo(
    () => files.find((item) => item.name === selectedFile) || null,
    [files, selectedFile]
  )

  const fetchLogFiles = useCallback(async () => {
    const res = await fetchWithAuth('/api/v1/system/logs/files')
    const data = await res.json()
    if (!data.success) {
      throw new Error(data.error || i18n.messages.loadFilesFailed)
    }
    return data.data as LogFilesPayload
  }, [i18n.messages.loadFilesFailed])

  const applyLogFilesPayload = useCallback((payload: LogFilesPayload) => {
    setLogsDir(payload.dir)
    setFiles(payload.files || [])
    setSelectedFile((prev) => {
      if (prev && payload.files.some((item) => item.name === prev)) {
        return prev
      }
      return payload.files[0]?.name || ''
    })
  }, [])

  const loadFiles = useCallback(async () => {
    setLoadingFiles(true)
    try {
      applyLogFilesPayload(await fetchLogFiles())
    } catch (error: any) {
      setFiles([])
      setSelectedFile('')
      setLogPayload(null)
    } finally {
      setLoadingFiles(false)
    }
  }, [applyLogFilesPayload, fetchLogFiles])

  const fetchLogContent = useCallback(async (
    file: string,
    lines: string,
    level: string,
    currentKeyword: string
  ) => {
    const search = new URLSearchParams({
      file,
      lines,
    })
    if (level && level !== 'ALL') {
      search.set('level', level)
    }
    if (currentKeyword.trim()) {
      search.set('keyword', currentKeyword.trim())
    }

    const res = await fetchWithAuth(`/api/v1/system/logs/read?${search.toString()}`)
    const data = await res.json()
    if (!data.success) {
      throw new Error(data.error || i18n.messages.loadContentFailed)
    }

    return data.data as LogReadPayload
  }, [i18n.messages.loadContentFailed])

  const loadLogContent = useCallback(async (
    file: string,
    lines: string,
    level: string,
    currentKeyword: string
  ) => {
    setLoadingContent(true)
    try {
      setLogPayload(await fetchLogContent(file, lines, level, currentKeyword))
    } catch (error: any) {
      setLogPayload(null)
    } finally {
      setLoadingContent(false)
    }
  }, [fetchLogContent])

  useEffect(() => {
    let cancelled = false

    const loadInitialFiles = async () => {
      setLoadingFiles(true)
      try {
        const payload = await fetchLogFiles()
        if (!cancelled) {
          applyLogFilesPayload(payload)
        }
      } catch (error) {
        if (!cancelled) {
          setFiles([])
          setSelectedFile('')
          setLogPayload(null)
        }
      } finally {
        if (!cancelled) {
          setLoadingFiles(false)
        }
      }
    }

    void loadInitialFiles()

    return () => {
      cancelled = true
    }
  }, [applyLogFilesPayload, fetchLogFiles])

  useEffect(() => {
    if (!selectedFile) {
      return
    }

    let cancelled = false

    const loadSelectedLogContent = async () => {
      setLoadingContent(true)
      try {
        const payload = await fetchLogContent(selectedFile, selectedLines, selectedLevel, appliedKeyword)
        if (!cancelled) {
          setLogPayload(payload)
        }
      } catch (error) {
        if (!cancelled) {
          setLogPayload(null)
        }
      } finally {
        if (!cancelled) {
          setLoadingContent(false)
        }
      }
    }

    void loadSelectedLogContent()

    return () => {
      cancelled = true
    }
  }, [appliedKeyword, fetchLogContent, selectedFile, selectedLevel, selectedLines])

  const handleSearch = () => {
    setAppliedKeyword(keyword.trim())
  }

  const handleRefresh = async () => {
    await loadFiles()
    if (selectedFile) {
      await loadLogContent(selectedFile, selectedLines, selectedLevel, appliedKeyword)
    }
  }

  const formatBytes = (value?: number) => {
    if (!value) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB']
    let size = value
    let unitIndex = 0
    while (size >= 1024 && unitIndex < units.length - 1) {
      size /= 1024
      unitIndex += 1
    }
    return `${size.toFixed(size >= 10 || unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`
  }

  const formatDateTime = (value?: string) => {
    if (!value) return '-'
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) {
      return value
    }
    return date.toLocaleString()
  }

  const visibleLogPayload = selectedFile ? logPayload : null

  const filterFields = <>
    <div className="min-w-0 space-y-2">
      <Label>{i18n.file}</Label>
      <Select value={selectedFile} onValueChange={setSelectedFile} disabled={loadingFiles || files.length === 0}>
        <SelectTrigger aria-label={i18n.file} className="h-11 min-h-11 w-full min-w-0 md:h-9 md:min-h-9"><SelectValue placeholder={i18n.emptyFiles} /></SelectTrigger>
        <SelectContent>{files.map((file) => <SelectItem className="min-h-11 md:min-h-0" key={file.name} value={file.name}>{file.name}</SelectItem>)}</SelectContent>
      </Select>
    </div>
    <div className="min-w-0 space-y-2">
      <Label>{i18n.lines}</Label>
      <Select value={selectedLines} onValueChange={setSelectedLines}>
        <SelectTrigger aria-label={i18n.lines} className="h-11 min-h-11 w-full md:h-9 md:min-h-9"><SelectValue /></SelectTrigger>
        <SelectContent>{LINE_OPTIONS.map((option) => <SelectItem className="min-h-11 md:min-h-0" key={option} value={option}>{option}</SelectItem>)}</SelectContent>
      </Select>
    </div>
    <div className="min-w-0 space-y-2">
      <Label>{i18n.level}</Label>
      <Select value={selectedLevel} onValueChange={(value) => setSelectedLevel(value as (typeof LEVEL_OPTIONS)[number])}>
        <SelectTrigger aria-label={i18n.level} className="h-11 min-h-11 w-full md:h-9 md:min-h-9"><SelectValue /></SelectTrigger>
        <SelectContent>{LEVEL_OPTIONS.map((option) => <SelectItem className="min-h-11 md:min-h-0" key={option} value={option}>{i18n.levelOptions[option.toLowerCase() as keyof typeof i18n.levelOptions]}</SelectItem>)}</SelectContent>
      </Select>
    </div>
  </>

  const fileDetails = <dl className="flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground">
    <div className="min-w-0"><dt className="inline text-foreground">{i18n.directory}: </dt><dd className="inline break-all font-mono">{logsDir || '-'}</dd></div>
    <div><dt className="inline text-foreground">{i18n.fileSize}: </dt><dd className="inline">{formatBytes(selectedMeta?.size ?? visibleLogPayload?.size)}</dd></div>
    <div><dt className="inline text-foreground">{i18n.updatedAt}: </dt><dd className="inline">{formatDateTime(selectedMeta?.modifiedAt)}</dd></div>
  </dl>

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-3 md:h-auto md:gap-6">
      <Card className="shrink-0 gap-3 py-3 md:gap-4 md:py-6">
        <CardHeader className="hidden md:block"><CardTitle>{i18n.title}</CardTitle></CardHeader>
        <CardContent className="space-y-3 px-3 md:space-y-5 md:px-6">
          <div className="hidden gap-3 md:grid md:grid-cols-3">{filterFields}</div>
          <form className="flex min-w-0 items-center gap-2" onSubmit={(event) => { event.preventDefault(); handleSearch() }}>
            <Input aria-label={i18n.keyword} value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder={i18n.keywordPlaceholder} className="h-11 min-w-0 flex-1 text-base md:h-9 md:text-sm" />
            <Button type="submit" variant="outline" aria-label={i18n.search} className="size-11 shrink-0 p-0 md:h-9 md:w-auto md:px-3"><Search className="size-4" /><span className="hidden md:inline">{i18n.search}</span></Button>
            <Button type="button" variant="outline" aria-label={i18n.filters} onClick={() => setFiltersOpen(true)} className="relative size-11 shrink-0 p-0 md:hidden"><SlidersHorizontal className="size-4" />{selectedLevel !== 'ALL' || selectedLines !== '200' ? <span className="absolute right-1.5 top-1.5 size-1.5 rounded-full bg-primary" /> : null}</Button>
            <Button type="button" variant="outline" onClick={handleRefresh} disabled={loadingFiles || loadingContent} aria-label={i18n.refresh} className="size-11 shrink-0 p-0 md:h-9 md:w-auto md:px-3"><RefreshCw className={cn('size-4', (loadingFiles || loadingContent) && 'animate-spin')} /><span className="hidden md:inline">{i18n.refresh}</span></Button>
          </form>
          <div className="hidden border-t border-border pt-4 md:block">{fileDetails}</div>
          <details className="group md:hidden">
            <summary className="flex min-h-11 cursor-pointer list-none flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground [&::-webkit-details-marker]:hidden">
              <span>{i18n.levelOptions[selectedLevel.toLowerCase() as keyof typeof i18n.levelOptions]} · {selectedLines} {i18n.lineUnit}</span>
              <span className="underline underline-offset-4">{i18n.fileInfo}</span>
            </summary>
            <div className="max-h-32 overflow-y-auto border-t pt-3">{fileDetails}</div>
          </details>
        </CardContent>
      </Card>

      <Card className="min-h-0 min-w-0 flex-1 gap-3 overflow-hidden py-3 md:flex-none md:gap-4 md:py-6">
        <CardHeader className="flex shrink-0 flex-row items-center justify-between gap-2 px-3 md:px-6">
          <CardTitle className="flex min-w-0 items-center gap-2 text-sm md:text-base"><FileText className="size-4 shrink-0 text-primary" /><span className="truncate">{selectedFile || i18n.preview}</span></CardTitle>
          <Button type="button" variant="ghost" size="icon" aria-label={i18n.wrapLines} title={i18n.wrapLines} aria-pressed={wrapLines} onClick={() => setWrapLines((value) => !value)} className={cn('size-11 shrink-0 md:size-9', wrapLines && 'bg-muted text-foreground')}><WrapText className="size-4" /></Button>
        </CardHeader>
        <CardContent className="flex min-h-0 min-w-0 flex-1 flex-col gap-2 px-3 md:gap-4 md:px-6">
          {visibleLogPayload?.suggestDirectRead ? <p className="shrink-0 rounded-lg border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-200">{i18n.directReadHint}</p> : null}
          <div role="region" aria-label={i18n.preview} aria-busy={loadingContent} tabIndex={0} className="min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain rounded-xl border bg-muted/20 md:h-[520px] md:flex-none">
            <pre className={cn('p-3 font-mono text-xs leading-6 text-foreground/90 md:p-4', wrapLines ? 'whitespace-pre-wrap break-words [overflow-wrap:anywhere]' : 'w-max min-w-full whitespace-pre')}>
              {loadingContent ? i18n.loading : visibleLogPayload?.lines?.length ? visibleLogPayload.lines.join('\n') : i18n.emptyContent}
            </pre>
          </div>
          <p className="shrink-0 text-[11px] leading-4 text-muted-foreground md:text-xs">{i18n.maxLinesHint.replace('{maxLines}', String(visibleLogPayload?.maxLines || 1000))}</p>
        </CardContent>
      </Card>

      <Sheet open={filtersOpen} onOpenChange={setFiltersOpen}>
        <SheetContent side="bottom" className="max-h-[calc(100dvh-16px)] gap-0 rounded-t-2xl [&>button]:size-11 [&>button]:top-1 [&>button]:right-2 [&>button]:flex [&>button]:items-center [&>button]:justify-center">
          <SheetHeader><SheetTitle>{i18n.filters}</SheetTitle><SheetDescription>{i18n.filterDescription}</SheetDescription></SheetHeader>
          <div className="grid min-h-0 gap-4 overflow-y-auto px-4 py-2">{filterFields}</div>
          <SheetFooter className="pb-[max(16px,env(safe-area-inset-bottom))]"><SheetClose asChild><Button className="h-11">{t.common.confirm}</Button></SheetClose></SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  )
}
