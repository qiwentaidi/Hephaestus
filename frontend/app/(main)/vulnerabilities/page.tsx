'use client'

import {
  Suspense,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import {
  Bot,
  CheckCircle2,
  ChevronDown,
  ClipboardCheck,
  Download,
  Eye,
  FileSearch,
  FileText,
  FilterX,
  Globe,
  Info,
  Loader2,
  MoreHorizontal,
  RefreshCw,
  Search,
  ShieldAlert,
} from 'lucide-react'
import toast from 'react-hot-toast'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
} from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { FindingMultiSelectFilter } from './components/FindingMultiSelectFilter'
import {
  SourceScopeFilter,
  type ScopeSelection,
} from './components/SourceScopeFilter'
import {
  listCenterFindings,
  updateCenterFindingStatus,
  updateCenterFindingStatuses,
  type CenterFinding,
  type CenterResult,
  type FindingSource,
} from '@/lib/api/vulnerability-center'
import {
  exportAIWorkbenchFindingReports,
  type AIWorkbenchFindingStatus,
} from '@/lib/api/ai-workbench'
import { findingDetailHref } from '@/lib/finding-detail'
import { scanFindingDetailHref } from '@/lib/scan-finding-detail'
import {
  FINDING_LIST_PAGE_SIZE_OPTIONS,
  findingListHref,
  parseFindingListPage,
  parseFindingListPageSize,
} from '@/lib/finding-list-query'
import {
  findingSeverityBadgeClassName,
  findingStatusBadgeClassName,
} from '@/lib/finding-badge'
import { formatDateTime } from '@/lib/date'
import { useI18n } from '@/lib/i18n'
import { getFindingLabels } from '@/lib/finding-i18n'
import {
  formatWorkbenchText,
  workbenchErrorMessage,
} from '@/lib/ai-workbench-i18n'
import { copyToClipboard } from '@/lib/clipboard'
import { saveReportBlob, type ReportExportFormat } from '@/lib/report-export'
import { cn } from '@/lib/utils'

const statuses: AIWorkbenchFindingStatus[] = [
  'open',
  'in_progress',
  'fixed',
  'false_positive',
  'ignored',
]
const sources = ['all', 'ai', 'web', 'js'] as const
const scopeKeys = [
  'projectId',
  'conversationId',
  'taskId',
  'version',
  'scopeName',
  'childName',
]

export default function VulnerabilityManagementPage() {
  return (
    <Suspense
      fallback={
        <div className="p-8" role="status">
          <Loader2 className="h-5 w-5 animate-spin" />
        </div>
      }
    >
      <VulnerabilityManagementContent />
    </Suspense>
  )
}

function VulnerabilityManagementContent() {
  const { t } = useI18n()
  const l = t.vulnerabilityManagement
  const labels = useMemo(() => getFindingLabels(l), [l])
  const router = useRouter()
  const params = useSearchParams()
  const currentSearch = params.toString()
  const source =
    sources.find((value) => value === params.get('source')) || 'all'
  const page = parseFindingListPage(params.get('page'))
  const pageSize = parseFindingListPageSize(params.get('pageSize'))
  const search = params.get('search') || ''
  const severities = (params.get('severity') || '').split(',').filter(Boolean)
  const selectedStatuses = (params.get('status') || '')
    .split(',')
    .filter(Boolean) as AIWorkbenchFindingStatus[]
  const needsReview = params.get('needsReview') === 'true'
  const projectId = params.get('projectId') || ''
  const taskId = params.get('taskId') || ''
  const scope: ScopeSelection | null = projectId
    ? {
        source: 'ai',
        id: projectId,
        name: params.get('scopeName') || projectId,
        childId: params.get('conversationId') || undefined,
        childName: params.get('childName') || undefined,
      }
    : taskId && (source === 'web' || source === 'js')
      ? {
          source,
          id: taskId,
          name: params.get('scopeName') || taskId,
          childId: params.get('version') || undefined,
        }
      : null
  const [draft, setDraft] = useState({ query: search, value: search })
  const searchDraft = draft.query === search ? draft.value : search
  const setSearchDraft = (value: string) => setDraft({ query: search, value })
  const [result, setResult] = useState<CenterResult | null>(null)
  const [loadedQuery, setLoadedQuery] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState(false)
  const savingRef = useRef(false)
  const [dispositionOpen, setDispositionOpen] = useState(false)
  const [nextStatus, setNextStatus] =
    useState<AIWorkbenchFindingStatus>('in_progress')
  const [note, setNote] = useState('')
  const [exporting, setExporting] = useState(false)
  const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)
  const tableHostRef = useRef<HTMLDivElement>(null)
  const busy = loading || saving || loadedQuery !== currentSearch
  const findings = result?.findings || []
  const selectedFindings = findings.filter((finding) =>
    selected.has(finding.id),
  )
  const allSelected =
    findings.length > 0 && selectedFindings.length === findings.length
  const selectedOnlyAI =
    selectedFindings.length > 0 &&
    selectedFindings.every((finding) => finding.source === 'ai')
  const activeFilters = Boolean(
    search ||
      severities.length ||
      selectedStatuses.length ||
      needsReview ||
      scope,
  )
  const refresh = () => setRevision((value) => value + 1)
  const messagesRef = useRef({ l, workbench: t.aiWorkbench })
  useEffect(() => {
    messagesRef.current = { l, workbench: t.aiWorkbench }
  }, [l, t.aiWorkbench])

  const changeQuery = useCallback(
    (values: Record<string, string | null>, resetPage = true) => {
      const query = new URLSearchParams(currentSearch)
      for (const [key, value] of Object.entries(values)) {
        if (value) query.set(key, value)
        else query.delete(key)
      }
      if (resetPage) query.delete('page')
      setSelected(new Set())
      router.replace(
        query.size ? `/vulnerabilities?${query}` : '/vulnerabilities',
        { scroll: false },
      )
    },
    [currentSearch, router],
  )
  useEffect(() => {
    if (searchDraft === search) return
    const timer = setTimeout(
      () => changeQuery({ search: searchDraft.trim() }),
      250,
    )
    return () => clearTimeout(timer)
  }, [searchDraft, search, changeQuery])
  useEffect(() => {
    const controller = new AbortController()
    // The URL is external navigation state; mark its request pending before loading.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true)
    setError('')
    const query = new URLSearchParams(currentSearch)
    query.set('page', String(page))
    query.set('pageSize', String(pageSize))
    void listCenterFindings(query, controller.signal)
      .then((next) => {
        if (controller.signal.aborted) return
        const last = Math.max(1, Math.ceil(next.total / pageSize))
        if (page > last) {
          router.replace(findingListHref(last, pageSize, currentSearch), {
            scroll: false,
          })
          return
        }
        setResult(next)
        setLoadedQuery(currentSearch)
        setSelected(
          (previous) =>
            new Set(
              [...previous].filter((id) =>
                next.findings.some((finding) => finding.id === id),
              ),
            ),
        )
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          setResult(null)
          setError(
            workbenchErrorMessage(
              e,
              messagesRef.current.workbench,
              messagesRef.current.l.loadFailed,
            ),
          )
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [currentSearch, page, pageSize, revision, router])
  const updateShadow = useCallback(() => {
    const el = tableHostRef.current?.querySelector<HTMLDivElement>(
      '[data-slot="table-container"]',
    )
    setShowRightFixedShadow(
      Boolean(
        el &&
          el.scrollWidth - el.clientWidth > 1 &&
          el.scrollLeft < el.scrollWidth - el.clientWidth - 1,
      ),
    )
  }, [])
  useEffect(() => {
    const el = tableHostRef.current?.querySelector<HTMLDivElement>(
      '[data-slot="table-container"]',
    )
    if (!el) return
    const observer = new ResizeObserver(updateShadow)
    observer.observe(el)
    const table = el.querySelector('table')
    if (table) observer.observe(table)
    el.addEventListener('scroll', updateShadow)
    updateShadow()
    return () => {
      observer.disconnect()
      el.removeEventListener('scroll', updateShadow)
    }
  }, [findings.length, loading, updateShadow])

  const changeSource = (value: string) => {
    changeQuery({
      source: value === 'all' ? null : value,
      ...Object.fromEntries(scopeKeys.map((key) => [key, null])),
    })
  }
  const changeScope = (value: ScopeSelection | null) => {
    const next: Record<string, string | null> = Object.fromEntries(
      scopeKeys.map((key) => [key, null]),
    )
    if (value) {
      next.source = value.source
      next.scopeName = value.name
      if (value.source === 'ai') {
        next.projectId = value.id
        next.conversationId = value.childId || null
        next.childName = value.childName || null
      } else {
        next.taskId = value.id
        next.version = value.childId || null
      }
    }
    changeQuery(next)
  }
  const reset = () => {
    setSearchDraft('')
    changeQuery({
      search: null,
      severity: null,
      status: null,
      needsReview: null,
      ...Object.fromEntries(scopeKeys.map((key) => [key, null])),
    })
  }
  const openDetail = (finding: CenterFinding) => {
    if (finding.source === 'ai')
      router.push(
        findingDetailHref(
          finding.nativeId,
          findingListHref(page, pageSize, currentSearch),
        ),
      )
    else router.push(scanFindingDetailHref(finding, findingListHref(page, pageSize, currentSearch)))
  }

  const toggle = (id: string, checked: boolean) =>
    setSelected((previous) => {
      const next = new Set(previous)
      if (checked) next.add(id)
      else next.delete(id)
      return next
    })
  const updateStatus = async (
    finding: CenterFinding,
    status: AIWorkbenchFindingStatus,
  ) => {
    if (savingRef.current) return
    savingRef.current = true
    setSaving(true)
    try {
      await updateCenterFindingStatus(finding, status)
      toast.success(l.statusUpdated)
      refresh()
    } catch (e) {
      toast.error(workbenchErrorMessage(e, t.aiWorkbench, l.updateFailed))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }
  const updateBatch = async () => {
    if (savingRef.current || busy || !selectedFindings.length) return
    savingRef.current = true
    setSaving(true)
    try {
      const response = await updateCenterFindingStatuses(
        selectedFindings,
        nextStatus,
        selectedOnlyAI ? note.trim() : '',
      )
      setSelected(new Set(response.failed.map((item) => item.findingId)))
      if (response.failedCount)
        toast.error(
          formatWorkbenchText(l.batchPartial, {
            updated: response.updatedCount,
            failed: response.failedCount,
            error: response.failed[0]?.error || l.retry,
          }),
        )
      else {
        toast.success(
          formatWorkbenchText(l.batchSuccess, {
            count: response.updatedCount,
            status: labels.status[nextStatus],
          }),
        )
        setDispositionOpen(false)
      }
      refresh()
    } catch (e) {
      toast.error(workbenchErrorMessage(e, t.aiWorkbench, l.batchFailed))
    } finally {
      savingRef.current = false
      setSaving(false)
    }
  }
  const exportCSV = (rows: CenterFinding[]) => {
    const values = [
      [
        l.finding,
        l.severity,
        l.status,
        l.target,
        l.sourceLabel,
        l.sourceScope,
        l.sourceSession,
        l.scanVersionLabel,
        l.discovered,
      ],
      ...rows.map((f) => [
        f.name,
        labels.severity[f.severity] || f.severity,
        labels.status[f.status] || f.status,
        f.url,
        l.sources[f.source],
        f.source === 'ai' ? f.projectName : f.taskName,
        f.source === 'ai' ? f.taskName : '',
        f.version || '',
        formatDateTime(f.createdAt),
      ]),
    ]
    const csv = values
      .map((row) =>
        row
          .map((value) => `"${String(value).replaceAll('"', '""')}"`)
          .join(','),
      )
      .join('\n')
    const url = URL.createObjectURL(
      new Blob(['\ufeff', csv], { type: 'text/csv;charset=utf-8' }),
    )
    const a = document.createElement('a')
    a.href = url
    a.download = `vulnerability-center-${Date.now()}.csv`
    a.click()
    URL.revokeObjectURL(url)
  }
  const exportReport = async (
    rows: CenterFinding[],
    format: ReportExportFormat,
  ) => {
    if (exporting || !rows.length || rows.some((row) => row.source !== 'ai'))
      return
    setExporting(true)
    const id = toast.loading(l.exportingReport)
    try {
      const report = await exportAIWorkbenchFindingReports(
        rows.map((row) => row.nativeId),
        undefined,
        format,
      )
      const state = await saveReportBlob(report.blob, report.filename, format)
      if (state === 'saved') toast.success(l.reportExported, { id })
      else toast.dismiss(id)
    } catch (e) {
      toast.error(
        workbenchErrorMessage(e, t.aiWorkbench, l.reportExportFailed),
        { id },
      )
    } finally {
      setExporting(false)
    }
  }
  const exportMenu = (rows: CenterFinding[]) => (
    <>
      {rows.length > 0 && rows.every((row) => row.source === 'ai') && (
        <>
          <DropdownMenuItem
            disabled={busy || exporting}
            onClick={() => void exportReport(rows, 'markdown')}
          >
            <FileText className="mr-2 h-4 w-4" />
            {l.exportMarkdown}
          </DropdownMenuItem>
          <DropdownMenuItem
            disabled={busy || exporting}
            onClick={() => void exportReport(rows, 'html')}
          >
            <Globe className="mr-2 h-4 w-4" />
            {l.exportHTML}
          </DropdownMenuItem>
          <DropdownMenuSeparator />
        </>
      )}
      <DropdownMenuItem
        disabled={busy || !rows.length}
        onClick={() => exportCSV(rows)}
      >
        <Download className="mr-2 h-4 w-4" />
        {l.exportCSV}
      </DropdownMenuItem>
    </>
  )
  const rowMenu = (finding: CenterFinding) => (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          disabled={busy}
          aria-label={l.actions}
        >
          <MoreHorizontal className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {statuses.map((status) => (
          <DropdownMenuItem
            key={status}
            disabled={saving}
            onClick={() => void updateStatus(finding, status)}
          >
            {status === 'open' ? l.reopen : labels.status[status]}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={async () => {
            if (await copyToClipboard(finding.url))
              toast.success(
                formatWorkbenchText(l.copiedLabel, { label: l.target }),
              )
            else toast.error(l.copyFailed)
          }}
        >
          {l.copyTarget}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
  const origin = (finding: CenterFinding) => (
    <Badge variant="outline" className="shrink-0 font-normal">
      {l.sources[finding.source]}
    </Badge>
  )
  const summary = [
    {
      key: 'all',
      label: l.allFindings,
      value: result?.overview.total || 0,
      icon: FileSearch,
    },
    {
      key: 'open',
      label: l.awaitingDisposition,
      value: result?.overview.open || 0,
      icon: ClipboardCheck,
    },
    {
      key: 'high',
      label: l.highAndAbove,
      value: result?.overview.high || 0,
      icon: ShieldAlert,
    },
    {
      key: 'review',
      label: l.needsReview,
      value: result?.overview.needsReview || 0,
      icon: Eye,
    },
  ]

  return (
    <div className="flex min-h-full flex-col bg-background text-foreground">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 lg:px-6">
        <h1 className="text-base font-semibold">{l.centerTitle}</h1>
        <div className="flex gap-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={busy || !findings.length || exporting}
              >
                <Download className="mr-2 h-4 w-4" />
                {l.exportPage}
                <ChevronDown className="ml-2 h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {exportMenu(findings)}
            </DropdownMenuContent>
          </DropdownMenu>
          <Button
            variant="outline"
            size="sm"
            onClick={refresh}
            disabled={loading || saving}
            aria-label={l.refresh}
          >
            <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
            <span className="ml-2 hidden sm:inline">{l.refresh}</span>
          </Button>
        </div>
      </header>
      <div className="space-y-4 p-3 sm:p-4 lg:p-6">
        <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
          {summary.map((item) => (
            <button
              key={item.key}
              disabled={saving}
              type="button"
              className="rounded-lg border border-border bg-card p-4 text-left hover:bg-muted/40"
              onClick={() =>
                changeQuery({
                  severity: item.key === 'high' ? 'critical,high' : null,
                  status: item.key === 'open' ? 'open,in_progress' : null,
                  needsReview: item.key === 'review' ? 'true' : null,
                })
              }
            >
              <span className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                {item.label}
                <item.icon className="h-4 w-4" />
              </span>
              <span className="mt-2 block text-2xl font-semibold tabular-nums">
                {item.value}
              </span>
            </button>
          ))}
        </div>
        <div
          role="tablist"
          aria-label={l.sourceLabel}
          className="flex flex-wrap gap-6 border-b border-border"
        >
          {sources.map((value) => (
            <button
              key={value}
              role="tab"
              aria-selected={source === value}
              disabled={saving || dispositionOpen}
              onClick={() => changeSource(value)}
              className={cn(
                'flex items-center gap-2 border-b-2 px-0 pb-3 text-sm',
                source === value
                  ? 'border-foreground text-foreground'
                  : 'border-transparent text-muted-foreground hover:text-foreground',
              )}
            >
              <span>{l.sources[value]}</span>
              <span className="rounded bg-muted px-1.5 text-xs tabular-nums text-muted-foreground">
                {value === 'all'
                  ? Object.values(result?.sourceCounts || {}).reduce(
                      (sum, count) => sum + count,
                      0,
                    )
                  : result?.sourceCounts[value] || 0}
              </span>
            </button>
          ))}
        </div>
        <fieldset
          disabled={saving || dispositionOpen}
          className="min-w-0 rounded-lg border border-border bg-card p-3"
        >
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-[180px] flex-1">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
              <Input
                value={searchDraft}
                onChange={(e) => setSearchDraft(e.target.value)}
                placeholder={l.centerSearch}
                aria-label={l.centerSearch}
                className="pl-9"
              />
            </div>
            <SourceScopeFilter
              source={source}
              value={scope}
              disabled={saving}
              onChange={changeScope}
            />
            <FindingMultiSelectFilter
              values={severities}
              options={Object.entries(labels.severity).map(
                ([value, label]) => ({ value, label }),
              )}
              allLabel={l.allSeverities}
              countLabel={l.selectedSeverities}
              ariaLabel={l.severityFilter}
              onChange={(values) => changeQuery({ severity: values.join(',') })}
            />
            <FindingMultiSelectFilter
              values={selectedStatuses}
              options={statuses.map((value) => ({
                value,
                label: labels.status[value],
              }))}
              allLabel={l.allStatuses}
              countLabel={l.selectedStatuses}
              ariaLabel={l.statusFilterTitle}
              onChange={(values) => changeQuery({ status: values.join(',') })}
            />
          </div>
          {(scope || needsReview || activeFilters) && (
            <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                {scope && (
                  <span className="break-all">
                    {l.sources[scope.source]} / {scope.name}
                    {scope.childId &&
                      ` / ${scope.source === 'ai' ? scope.childName || scope.childId : formatWorkbenchText(l.scanVersion, { version: scope.childId })}`}
                  </span>
                )}
                {needsReview && (
                  <Badge variant="outline" className="font-normal">
                    {l.needsReview}
                  </Badge>
                )}
              </div>
              {activeFilters && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={reset}
                >
                  <FilterX className="mr-1 h-3.5 w-3.5" />
                  {l.clearFilters}
                </Button>
              )}
            </div>
          )}
          {source === 'js' && (
            <p className="mt-2 flex items-start gap-1.5 text-xs leading-relaxed text-muted-foreground">
              <Info aria-hidden="true" className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{l.jsAdvancedFeaturesHint}</span>
            </p>
          )}
        </fieldset>
        {selectedFindings.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 p-3">
            <span className="mr-auto text-sm">
              {formatWorkbenchText(l.selectedCount, {
                count: selectedFindings.length,
              })}{' '}
              <span className="text-muted-foreground">{l.pageOnly}</span>
            </span>
            <Button
              size="sm"
              disabled={busy}
              onClick={() => {
                setNote('')
                setNextStatus('in_progress')
                setDispositionOpen(true)
              }}
            >
              {l.bulkDisposition}
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={busy || exporting}
                >
                  {l.exportSelected}
                  <ChevronDown className="ml-2 h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                {exportMenu(selectedFindings)}
              </DropdownMenuContent>
            </DropdownMenu>
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setSelected(new Set())}
              disabled={saving}
            >
              {l.clearSelection}
            </Button>
          </div>
        )}
        {error ? (
          <div
            role="alert"
            className="rounded-lg border border-border bg-card p-8 text-center"
          >
            <p className="mb-3 text-sm">{error}</p>
            <Button variant="outline" onClick={refresh}>
              {l.retry}
            </Button>
          </div>
        ) : (
          <>
            <div className="space-y-3 md:hidden">
              <label className="flex items-center gap-3 p-2 text-sm">
                <Checkbox
                  checked={
                    allSelected
                      ? true
                      : selectedFindings.length
                        ? 'indeterminate'
                        : false
                  }
                  disabled={busy || !findings.length}
                  onCheckedChange={(value) =>
                    setSelected(
                      value === true
                        ? new Set(findings.map((f) => f.id))
                        : new Set(),
                    )
                  }
                />
                {l.selectPage}
              </label>
              {loading ? (
                <div role="status" className="p-8 text-center">
                  {l.loading}
                </div>
              ) : !findings.length ? (
                <p className="p-8 text-center text-muted-foreground">
                  {l.empty}
                </p>
              ) : (
                findings.map((f) => (
                  <article
                    key={f.id}
                    className="space-y-3 rounded-lg border border-border bg-card p-4"
                  >
                    <div className="flex items-start gap-3">
                      <Checkbox
                        checked={selected.has(f.id)}
                        disabled={busy}
                        aria-label={formatWorkbenchText(l.selectFinding, {
                          name: f.name,
                        })}
                        onCheckedChange={(value) =>
                          toggle(f.id, value === true)
                        }
                      />
                      <button
                        className="min-w-0 flex-1 break-words text-left text-sm font-medium"
                        disabled={busy}
                        onClick={() => openDetail(f)}
                      >
                        {f.name || l.unnamed}
                      </button>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <Badge
                        variant="outline"
                        className={findingSeverityBadgeClassName(f.severity)}
                      >
                        {labels.severity[f.severity] || f.severity}
                      </Badge>
                      <Badge
                        variant="outline"
                        className={findingStatusBadgeClassName(f.status)}
                      >
                        {labels.status[f.status] || f.status}
                      </Badge>
                    </div>
                    <p className="break-all font-mono text-xs">{f.url}</p>
                    {origin(f)}
                    <p className="break-words text-xs text-muted-foreground">
                      {l.sourceSession}: {f.taskName || f.taskId || '-'}
                    </p>
                    <div className="flex items-center justify-between border-t border-border pt-3">
                      <span className="text-xs text-muted-foreground">
                        {formatDateTime(f.createdAt)}
                      </span>
                      <div className="flex">
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={busy}
                          onClick={() => openDetail(f)}
                        >
                          {l.view}
                        </Button>
                        {rowMenu(f)}
                      </div>
                    </div>
                  </article>
                ))
              )}
            </div>
            <div
              ref={tableHostRef}
              className="hidden overflow-hidden rounded-lg border border-border bg-card md:block"
            >
              <Table className="min-w-[1460px]">
                <TableHeader className="bg-muted/35 text-xs text-muted-foreground">
                  <TableRow>
                    <TableHead className="w-12 px-4">
                      <Checkbox
                        aria-label={l.selectPage}
                        checked={
                          allSelected
                            ? true
                            : selectedFindings.length
                              ? 'indeterminate'
                              : false
                        }
                        disabled={busy || !findings.length}
                        onCheckedChange={(value) =>
                          setSelected(
                            value === true
                              ? new Set(findings.map((f) => f.id))
                              : new Set(),
                          )
                        }
                      />
                    </TableHead>
                    <TableHead className="w-[28%] px-4 py-3">{l.findingOrLead}</TableHead>
                    <TableHead className="w-[88px] px-4 py-3">{l.severity}</TableHead>
                    <TableHead className="w-[24%] px-4 py-3">{l.target}</TableHead>
                    <TableHead className="w-[108px] px-4 py-3">{l.status}</TableHead>
                    <TableHead className="w-[180px] px-4 py-3">{l.discovered}</TableHead>
                    <TableHead className="w-[200px] px-4 py-3">{l.sourceScope}</TableHead>
                    <TableHead className="w-[180px] px-4 py-3">{l.sourceSession}</TableHead>
                    <TableHead
                      className={cn(
                        'sticky right-0 z-20 w-[156px] border-l border-border bg-card px-4 py-3 text-center',
                        showRightFixedShadow &&
                          'table-fixed-column-shadow-left',
                      )}
                    >
                      {l.actions}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {loading ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className="px-4 py-16 text-center text-muted-foreground"
                      >
                        <Loader2 className="mx-auto mb-2 h-5 w-5 animate-spin" />
                        {l.loading}
                      </TableCell>
                    </TableRow>
                  ) : !findings.length ? (
                    <TableRow>
                      <TableCell
                        colSpan={9}
                        className="px-4 py-16 text-center text-muted-foreground"
                      >
                        {l.empty}
                      </TableCell>
                    </TableRow>
                  ) : (
                    findings.map((f) => (
                      <TableRow
                        key={f.id}
                        data-state={selected.has(f.id) ? 'selected' : undefined}
                        className="group cursor-pointer hover:bg-muted/35 data-[state=selected]:bg-muted/60"
                        onClick={() => { if (!busy) openDetail(f) }}
                      >
                        <TableCell className="px-4 py-4 align-top" onClick={(event) => event.stopPropagation()}>
                          <Checkbox
                            checked={selected.has(f.id)}
                            disabled={busy}
                            aria-label={formatWorkbenchText(l.selectFinding, {
                              name: f.name,
                            })}
                            onCheckedChange={(value) =>
                              toggle(f.id, value === true)
                            }
                          />
                        </TableCell>
                        <TableCell className="max-w-[380px] px-4 py-4 align-top whitespace-normal">
                          <div
                            className={cn(
                              'font-medium leading-5',
                              f.source === 'ai' ? 'wrap-anywhere' : 'line-clamp-2',
                            )}
                          >
                            {f.name || l.unnamed}
                            {f.source === 'ai' && (
                              <>
                                {'\u00a0'}
                                <Bot
                                  className="inline-block size-3.5 align-middle text-muted-foreground/70"
                                  aria-hidden="true"
                                />
                              </>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="px-4 py-4 align-top">
                          <Badge
                            variant="outline"
                            className={findingSeverityBadgeClassName(
                              f.severity,
                            )}
                          >
                            {labels.severity[f.severity] || f.severity}
                          </Badge>
                        </TableCell>
                        <TableCell className="max-w-[360px] px-4 py-4 align-top">
                          <div
                            className="truncate font-mono text-xs"
                            title={f.url}
                          >
                            {f.url || '-'}
                          </div>
                        </TableCell>
                        <TableCell className="px-4 py-4 align-top">
                          <Badge
                            variant="outline"
                            className={findingStatusBadgeClassName(f.status)}
                          >
                            {labels.status[f.status] || f.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="px-4 py-4 align-top">
                          <div className="whitespace-nowrap font-mono text-xs text-muted-foreground">
                            {formatDateTime(f.createdAt)}
                          </div>
                        </TableCell>
                        <TableCell className="max-w-[200px] px-4 py-4 align-top">
                          {origin(f)}
                        </TableCell>
                        <TableCell className="max-w-40 px-4 py-4 align-top">
                          <div className="truncate text-xs text-muted-foreground" title={f.taskName || f.taskId || undefined}>
                            {f.taskName || f.taskId || '-'}
                          </div>
                        </TableCell>
                        <TableCell
                          className={cn(
                            'sticky right-0 z-10 border-l border-border bg-card px-3 py-4 text-center group-hover:bg-muted group-data-[state=selected]:bg-muted',
                            showRightFixedShadow &&
                              'table-fixed-column-shadow-left',
                          )}
                          onClick={(event) => event.stopPropagation()}
                        >
                          <div className="flex items-center justify-center gap-1">
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-8 px-2"
                              disabled={busy}
                              onClick={() => openDetail(f)}
                            >
                              <Eye className="mr-1 h-4 w-4" />
                              {l.view}
                            </Button>
                            {rowMenu(f)}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
            <PaginationPro
              current={page}
              total={result?.total || 0}
              pageSize={pageSize}
              pageSizeOptions={FINDING_LIST_PAGE_SIZE_OPTIONS}
              disabled={busy || dispositionOpen}
              showQuickJumper
              showTotal
              showSizeChanger
              onChange={(nextPage, size) =>
                changeQuery(
                  { page: String(nextPage), pageSize: String(size) },
                  false,
                )
              }
            />
          </>
        )}
      </div>
      <Dialog
        open={dispositionOpen}
        onOpenChange={(open) => {
          if (!savingRef.current) setDispositionOpen(open)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{l.bulkTitle}</DialogTitle>
            <DialogDescription>
              {formatWorkbenchText(l.centerBulkDescription, {
                count: selectedFindings.length,
              })}
            </DialogDescription>
          </DialogHeader>
          <Select
            value={nextStatus}
            onValueChange={(value) =>
              setNextStatus(value as AIWorkbenchFindingStatus)
            }
            disabled={saving}
          >
            <SelectTrigger aria-label={l.nextStatus}>
              {labels.status[nextStatus]}
            </SelectTrigger>
            <SelectContent>
              {statuses.map((value) => (
                <SelectItem key={value} value={value}>
                  {labels.status[value]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {selectedOnlyAI && (
            <div className="space-y-2">
              <label htmlFor="center-disposition-note" className="text-sm">
                {l.note}
                {l.optional}
              </label>
              <Textarea
                id="center-disposition-note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
                disabled={saving}
                maxLength={2000}
                placeholder={l.notePlaceholder}
              />
              <p className="text-xs text-muted-foreground">{l.noteHint}</p>
            </div>
          )}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDispositionOpen(false)}
              disabled={saving}
            >
              {l.cancel}
            </Button>
            <Button
              disabled={busy || !selectedFindings.length}
              onClick={() => void updateBatch()}
            >
              {saving ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <CheckCircle2 className="mr-2 h-4 w-4" />
              )}
              {formatWorkbenchText(l.confirmCount, {
                count: selectedFindings.length,
              })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </div>
  )
}
