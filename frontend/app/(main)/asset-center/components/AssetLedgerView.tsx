'use client'

import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useSearchParams } from 'next/navigation'
import {
  Archive,
  ChevronDown,
  Download,
  Info,
  Plus,
  RefreshCw,
  RotateCcw,
  ScanLine,
  Search,
  Upload,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PaginationPro } from '@/components/ui/pagination-pro'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import {
  AssetImportPreview,
  AssetLedgerEntity,
  AssetLedgerEntityCategory,
  AssetLedgerEntityRef,
  AssetLedgerEntityResponse,
  batchArchiveAssetLedger,
  createAssetLedger,
  exportAssetLedgerEntities,
  importAssetLedger,
  listAssetLedgerEntities,
  updateAssetLedger,
} from '@/lib/api/unified-assets'
import { saveBlobAsFile } from '@/lib/download'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'
import { cn } from '@/lib/utils'
import { AssetDetailSheet } from './AssetDetailSheet'
import { SourceBadges } from './SourceBadges'
import { AssetRootSheet } from './AssetRootSheet'
import { AssetScanDialog } from './AssetScanDialog'

const initialFilters = {
  keyword: '',
  status: 'all',
  source: 'all',
}

function scopeArchiveSummary(entities: AssetLedgerEntity[]) {
  return entities.reduce(
    (summary, entity) => {
      if (entity.category !== 'root') return summary
      summary.roots += 1
      summary.subdomains += entity.archiveSubdomainCount ?? entity.subdomainCount ?? 0
      summary.ports += entity.archivePortCount ?? entity.portCount ?? 0
      summary.websites += entity.archiveWebsiteCount ?? entity.websiteCount ?? 0
      return summary
    },
    { roots: 0, subdomains: 0, ports: 0, websites: 0 },
  )
}

const assetLedgerCategories: AssetLedgerEntityCategory[] = ['root', 'ip', 'subdomain', 'port', 'website']

function isAssetLedgerCategory(value: string | null): value is AssetLedgerEntityCategory {
  return value !== null && assetLedgerCategories.includes(value as AssetLedgerEntityCategory)
}

function formatDate(value: string | undefined, locale: string) {
  if (!value || value.startsWith('0001-')) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale, { hour12: false })
}

function AssetStatusBadge({ status }: { status: string }) {
  const { t } = useI18n()
  const text = t.assetInventory
  const labels: Record<string, string> = {
    active: text.statusActive,
    stale: text.statusStale,
    retired: text.statusRetired,
  }
  const classes: Record<string, string> = {
    active: 'border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300',
    stale: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    retired: 'border-muted-foreground/30 bg-muted text-muted-foreground',
  }
  return (
    <Badge variant="outline" className={classes[status] || classes.retired}>
      {labels[status] || status || text.statusUnknown}
    </Badge>
  )
}



export function AssetLedgerView() {
  const { t, language } = useI18n()
  const searchParams = useSearchParams()
  const text = t.assetLedger
  const inventory = t.assetInventory
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'

  const [category, setCategory] = useState<AssetLedgerEntityCategory>(() => {
    const requestedCategory = searchParams.get('category')
    return isAssetLedgerCategory(requestedCategory) ? requestedCategory : 'root'
  })
  const [filters, setFilters] = useState(initialFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [includeArchived, setIncludeArchived] = useState(false)
  const [refresh, setRefresh] = useState(0)
  const [data, setData] = useState<AssetLedgerEntityResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedAssetId, setSelectedAssetId] = useState<string | null>(null)
  const [selectedRoot, setSelectedRoot] = useState<AssetLedgerEntity | null>(null)
  const [selectedEntities, setSelectedEntities] = useState<Record<string, AssetLedgerEntity>>({})
  const [importOpen, setImportOpen] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const [scanOpen, setScanOpen] = useState(false)
  const [exporting, setExporting] = useState<'current' | 'all' | 'selected' | null>(null)
  const [batchAction, setBatchAction] = useState<'archive' | 'restore' | null>(null)
  const [scopeDialog, setScopeDialog] = useState<{ mode: 'single' | 'batch'; entities: AssetLedgerEntity[] } | null>(null)
  const [scopeAction, setScopeAction] = useState<'archive' | null>(null)
  const [scopeError, setScopeError] = useState('')

  const updateFilter = (key: keyof typeof initialFilters, value: string) => {
    setLoading(true)
    setFilters((previous) => ({ ...previous, [key]: value }))
    setPage(1)
  }

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(async () => {
      setLoading(true)
      setError('')
      const params = new URLSearchParams({ category, page: String(page), pageSize: String(pageSize) })
      if (filters.keyword.trim()) params.set('keyword', filters.keyword.trim())
      if (filters.status !== 'all') params.set('status', filters.status)
      if (filters.source !== 'all') params.set('source', filters.source)
      if (includeArchived) params.set('includeArchived', 'true')
      try {
        const result = await listAssetLedgerEntities(params, controller.signal)
        if (!controller.signal.aborted) setData(result)
      } catch (err) {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : text.loadFailed)
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }, 200)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [category, filters, page, pageSize, includeArchived, refresh, text.loadFailed])

  const counts = data?.counts
  const items = data?.items ?? []
  const selectedCount = Object.keys(selectedEntities).length
  const pageSelectedCount = items.filter((entity) => selectedEntities[entity.id]).length
  const allPageSelected = items.length > 0 && pageSelectedCount === items.length
  const somePageSelected = pageSelectedCount > 0 && !allPageSelected
  const isSelected = (entity: AssetLedgerEntity) => Boolean(selectedEntities[entity.id])
  const toggleEntitySelection = (entity: AssetLedgerEntity, checked: boolean) => {
    setSelectedEntities((previous) => {
      if (checked) return { ...previous, [entity.id]: entity }
      const next = { ...previous }
      delete next[entity.id]
      return next
    })
  }
  const togglePageSelection = (checked: boolean) => {
    setSelectedEntities((previous) => {
      const next = { ...previous }
      for (const entity of items) {
        if (checked) next[entity.id] = entity
        else delete next[entity.id]
      }
      return next
    })
  }
  const clearSelection = () => setSelectedEntities({})
  const activeFilterCount = [
    filters.keyword.trim(),
    filters.status !== 'all' ? filters.status : '',
    filters.source !== 'all' ? filters.source : '',
  ].filter(Boolean).length

  const categoryLabel = (value: AssetLedgerEntityCategory) => {
    switch (value) {
      case 'root':
        return text.tabRoot
      case 'ip':
        return text.tabIP
      case 'subdomain':
        return text.tabSubdomain
      case 'port':
        return text.tabPort
      case 'website':
        return text.tabWebsite
    }
  }
  const categoryCount = (value: AssetLedgerEntityCategory) => {
    if (!counts) return 0
    return counts[value]
  }
  const categories = assetLedgerCategories
  const scopeDialogSummary = scopeDialog ? scopeArchiveSummary(scopeDialog.entities) : null

  const applyScopeArchive = useCallback(async (entities: AssetLedgerEntity[], archived: boolean) => {
    const result = await batchArchiveAssetLedger({
      action: archived ? 'archive' : 'restore',
      items: entities.map((entity) => ({
        id: entity.id,
        category: entity.category,
        assetId: entity.assetId,
        value: entity.value,
        host: entity.host,
        port: entity.port,
        transport: entity.transport,
        scope: true,
      })),
    })
    const removed = new Set(entities.map((entity) => entity.id))
    setSelectedEntities((previous) => {
      const next = { ...previous }
      for (const id of removed) delete next[id]
      return next
    })
    setLoading(true)
    setRefresh((value) => value + 1)
    if (archived && selectedAssetId && entities.some((entity) => entity.assetId === selectedAssetId)) {
      setSelectedAssetId(null)
    }
    return result
  }, [selectedAssetId])

  const toggleArchive = useCallback(async (entity: AssetLedgerEntity, archived: boolean) => {
    // Unarchived aggregate rows are archived by scope; archived scope rows are
    // restored by scope. Root/port rows archived as part of their underlying
    // asset (legacy host/website archive) fall through to the asset path.
    const aggregateRow = entity.category === 'root' || entity.category === 'port'
    if (aggregateRow && (!entity.archived || entity.scopeArchived)) {
      if (entity.category === 'root' && archived) {
        setScopeError('')
        setScopeDialog({ mode: 'single', entities: [entity] })
        return
      }
      try {
        const result = await applyScopeArchive([entity], archived)
        if (result.failed.length) {
          toast.error(result.failed[0]?.error || t.common.updateFailed)
          return
        }
        const template = entity.category === 'root'
          ? (archived ? text.archiveRootSuccess : text.restoreRootSuccess)
          : (archived ? text.archivePortSuccess : text.restorePortSuccess)
        if (entity.category === 'root' && archived) {
          const counts = scopeArchiveSummary([entity])
          toast.success(
            template
              .replace('{value}', entity.value)
              .replace('{subdomains}', String(counts.subdomains))
              .replace('{ports}', String(counts.ports))
              .replace('{websites}', String(counts.websites)),
          )
        } else {
          toast.success(template.replace('{value}', entity.value))
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : t.common.updateFailed)
      }
      return
    }
    if (!entity.assetId) return
    try {
      await updateAssetLedger(entity.assetId, { archived, website: entity.category === 'website' })
      toast.success(archived ? text.archivedToast : text.restoredToast)
      setSelectedEntities((previous) => {
        const next = { ...previous }
        delete next[entity.id]
        return next
      })
      setLoading(true)
      setRefresh((value) => value + 1)
      if (archived && selectedAssetId === entity.assetId) setSelectedAssetId(null)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t.common.updateFailed)
    }
  }, [applyScopeArchive, selectedAssetId, t.common.updateFailed, text])

  const refreshData = useCallback(() => {
    setLoading(true)
    setRefresh((value) => value + 1)
  }, [])

  const changeCategory = (value: AssetLedgerEntityCategory) => {
    if (value === category) return
    setLoading(true)
    setCategory(value)
    setPage(1)
  }

  const selectedList = Object.values(selectedEntities)
  const archivableSelection = selectedList.filter((entity) => !entity.archived)
  const restorableSelection = selectedList.filter((entity) => entity.archived)
  const allFilteredCount = data?.filteredCounts
    ? data.filteredCounts.root + data.filteredCounts.ip + data.filteredCounts.subdomain
      + data.filteredCounts.port + data.filteredCounts.website
    : 0

  const toSelectionRefs = (entities: AssetLedgerEntity[]): AssetLedgerEntityRef[] => entities.map((entity) => ({
    id: entity.id,
    category: entity.category,
    assetId: entity.assetId,
    value: entity.value,
    host: entity.host,
    port: entity.port,
    transport: entity.transport,
    scope: Boolean(entity.scopeArchived) || (!entity.archived && (entity.category === 'root' || entity.category === 'port')),
  }))

  const batchArchiveSelection = async (archived: boolean) => {
    if (batchAction) return
    const targets = archived ? archivableSelection : restorableSelection
    if (!targets.length) return
    if (archived && targets.some((entity) => entity.category === 'root')) {
      setScopeError('')
      setScopeDialog({ mode: 'batch', entities: targets })
      return
    }
    setBatchAction(archived ? 'archive' : 'restore')
    try {
      const result = await batchArchiveAssetLedger({
        action: archived ? 'archive' : 'restore',
        items: toSelectionRefs(targets),
      })
      const summary = text.batchResult
        .replace('{updated}', String(result.updated))
        .replace('{skipped}', String(result.skipped))
      if (result.failed.length) {
        toast.error(`${summary} · ${text.batchFailed.replace('{count}', String(result.failed.length))}`)
      } else if (result.updated > 0) {
        toast.success(summary)
      } else {
        toast.info(summary)
      }
      if (result.updated > 0) {
        setSelectedEntities({})
        refreshData()
        if (archived && selectedAssetId && targets.some((entity) => entity.assetId === selectedAssetId)) {
          setSelectedAssetId(null)
        }
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t.common.updateFailed)
    } finally {
      setBatchAction(null)
    }
  }

  const confirmScopeArchive = async () => {
    if (!scopeDialog || scopeAction) return
    const targets = scopeDialog.entities
    setScopeAction('archive')
    setScopeError('')
    try {
      const result = await applyScopeArchive(targets, true)
      const summary = text.batchResult
        .replace('{updated}', String(result.updated))
        .replace('{skipped}', String(result.skipped))
      if (result.failed.length) {
        setScopeError(text.batchFailed.replace('{count}', String(result.failed.length)))
        toast.error(`${summary} · ${text.batchFailed.replace('{count}', String(result.failed.length))}`)
        return
      }
      if (scopeDialog.mode === 'single') {
        const entity = targets[0]
        const counts = scopeArchiveSummary(targets)
        toast.success(
          text.archiveRootSuccess
            .replace('{value}', entity.value)
            .replace('{subdomains}', String(counts.subdomains))
            .replace('{ports}', String(counts.ports))
            .replace('{websites}', String(counts.websites)),
        )
      } else {
        toast.success(summary)
      }
      setScopeDialog(null)
    } catch (err) {
      setScopeError(err instanceof Error ? err.message : t.common.updateFailed)
    } finally {
      setScopeAction(null)
    }
  }

  const exportEntities = async (scope: 'current' | 'all' | 'selected') => {
    if (exporting) return
    setExporting(scope)
    try {
      let result: { blob: Blob; filename: string }
      if (scope === 'selected') {
        if (!selectedList.length) return
        result = await exportAssetLedgerEntities({ items: toSelectionRefs(selectedList), lang: language })
      } else {
        const params = new URLSearchParams({ category: scope === 'all' ? 'all' : category })
        if (filters.keyword.trim()) params.set('keyword', filters.keyword.trim())
        if (filters.status !== 'all') params.set('status', filters.status)
        if (filters.source !== 'all') params.set('source', filters.source)
        if (includeArchived) params.set('includeArchived', 'true')
        result = await exportAssetLedgerEntities({ params, lang: language })
      }
      const saved = await saveBlobAsFile(result.blob, result.filename)
      if (saved === 'saved') toast.success(text.exportSuccess)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : text.exportFailed)
    } finally {
      setExporting(null)
    }
  }

  const fingerprintCell = (values?: string[]) => {
    const fingerprints = values || []
    if (!fingerprints.length) return <span className="text-muted-foreground">—</span>
    return (
      <div className="flex max-w-[260px] flex-wrap gap-1">
        {fingerprints.slice(0, 4).map((value) => (
          <Badge key={value} variant="secondary" className="max-w-full break-all font-normal">{value}</Badge>
        ))}
        {fingerprints.length > 4 ? <Badge variant="outline" className="font-normal">+{fingerprints.length - 4}</Badge> : null}
      </div>
    )
  }

  const domainCell = (domains?: string[]) => {
    const values = domains || []
    if (!values.length) return <span className="text-muted-foreground">—</span>
    const label = `${values[0]}${values.length > 1 ? ` +${values.length - 1}` : ''}`
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="block max-w-full cursor-default truncate font-mono text-xs" tabIndex={0}>{label}</span>
        </TooltipTrigger>
        <TooltipContent className="max-w-[360px] whitespace-normal break-all">{values.join(', ')}</TooltipContent>
      </Tooltip>
    )
  }

  const ipListCell = (ips?: string[]) => {
    const values = ips || []
    if (!values.length) return <span className="text-muted-foreground">—</span>
    const label = `${values[0]}${values.length > 1 ? ` +${values.length - 1}` : ''}`
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="block max-w-full cursor-default truncate font-mono text-xs" tabIndex={0}>{label}</span>
        </TooltipTrigger>
        <TooltipContent className="max-w-[360px] whitespace-normal break-all">{values.join(', ')}</TooltipContent>
      </Tooltip>
    )
  }

  const formatPortProtocol = (protocol?: string, transport?: string) => {
    const transportLabel = (transport || '').trim()
    const labels = (protocol || '')
      .split(' / ')
      .map((value) => value.trim())
      .filter(Boolean)
      .filter((value) => value.toLowerCase() !== transportLabel.toLowerCase())
    if (transportLabel && !labels.some((value) => value.toLowerCase() === transportLabel.toLowerCase())) {
      labels.push(transportLabel)
    }
    return labels.join(' / ') || '—'
  }

  const truncateCell = (value?: string) => (
    value ? <span className="block max-w-full truncate text-xs" title={value}>{value}</span> : <span className="text-muted-foreground">—</span>
  )

  const subdomainCell = (entity: AssetLedgerEntity) => {
    const display = entity.displayValue || entity.value
    return (
      <div className="min-w-0 max-w-full">
        <Tooltip>
          <TooltipTrigger asChild>
            <p className="max-w-full cursor-default truncate font-mono text-sm" tabIndex={0}>{display}</p>
          </TooltipTrigger>
          <TooltipContent className="max-w-[420px] whitespace-normal break-all">{display}</TooltipContent>
        </Tooltip>
        {entity.rootDomain ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <p className="mt-0.5 max-w-full cursor-default truncate text-[13px] text-muted-foreground" tabIndex={0}>
                {text.rootDomain}: {entity.rootDomain}
              </p>
            </TooltipTrigger>
            <TooltipContent className="max-w-[360px] whitespace-normal break-all">{entity.rootDomain}</TooltipContent>
          </Tooltip>
        ) : null}
      </div>
    )
  }

  const sourceCell = (entity: AssetLedgerEntity) => <SourceBadges sources={entity.sources} />

  const lastSeenCell = (entity: AssetLedgerEntity) => (
    <span className="text-xs text-muted-foreground">{formatDate(entity.lastSeenAt, locale)}</span>
  )

  const selectionHeader = (label: string, className?: string) => (
    <TableHead className={className}>
      <div className="flex items-center gap-2">
        <Checkbox
          checked={allPageSelected ? true : somePageSelected ? 'indeterminate' : false}
          onCheckedChange={(value) => togglePageSelection(value === true)}
          disabled={!items.length}
          aria-label={text.selectAll}
        />
        <span>{label}</span>
      </div>
    </TableHead>
  )

  const selectionCell = (entity: AssetLedgerEntity, content: ReactNode) => (
    <TableCell className="whitespace-normal">
      <div className="flex min-w-0 items-start gap-2">
        <div
          className="pt-0.5"
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <Checkbox
            checked={isSelected(entity)}
            onCheckedChange={(value) => toggleEntitySelection(entity, value === true)}
            aria-label={text.selectAsset}
          />
        </div>
        <div className="min-w-0">{content}</div>
      </div>
    </TableCell>
  )

  const isEntityClickable = (entity: AssetLedgerEntity) => Boolean(entity.assetId) || entity.category === 'root'

  const openEntity = (entity: AssetLedgerEntity) => {
    if (entity.assetId) {
      setSelectedAssetId(entity.assetId)
      return
    }
    if (entity.category === 'root') {
      setSelectedRoot(entity)
    }
  }

  const viewRootSubdomains = (entity: AssetLedgerEntity) => {
    setSelectedRoot(null)
    setLoading(true)
    setCategory('subdomain')
    setFilters({ ...initialFilters, keyword: entity.value })
    setPage(1)
  }

  const archiveRootFromSheet = (entity: AssetLedgerEntity) => {
    setSelectedRoot(null)
    void toggleArchive(entity, !entity.archived)
  }

  const actionCell = (entity: AssetLedgerEntity) => {
    const label = entity.scopeArchived
      ? (entity.category === 'root' ? text.restoreRoot : text.restorePort)
      : entity.category === 'root'
        ? (entity.archived ? text.restore : text.archiveRoot)
        : entity.category === 'port'
          ? (entity.archived ? text.restore : text.archivePort)
          : (entity.archived ? text.restore : text.archive)
    return (
      <Button
        variant="ghost"
        size="icon-sm"
        title={label}
        aria-label={`${label} ${entity.value}`}
        onClick={(event) => { event.stopPropagation(); void toggleArchive(entity, !entity.archived) }}
      >
        {entity.archived ? <RotateCcw className="size-4" /> : <Archive className="size-4" />}
      </Button>
    )
  }

  const scopeBadge = (entity: AssetLedgerEntity) => {
    if (!entity.scopeArchived) return null
    return (
      <Badge
        variant="outline"
        className="border-dashed border-muted-foreground/40 bg-transparent text-muted-foreground"
      >
        {entity.category === 'root' ? text.scopeBadgeRoot : text.scopeBadgePort}
      </Badge>
    )
  }

  const valueCell = (entity: AssetLedgerEntity) => (
    <div className="min-w-0">
      {entity.name ? <p className="truncate text-sm font-medium">{entity.name}</p> : null}
      <div className="flex flex-wrap items-center gap-2">
        <p className="break-all font-mono text-sm">{entity.displayValue || entity.value}</p>
        {scopeBadge(entity)}
      </div>
      {category !== 'port' && category !== 'website' && entity.host && entity.host !== entity.value ? <p className="mt-1 break-all text-xs text-muted-foreground">{entity.host}</p> : null}
    </div>
  )

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-4 overflow-y-auto p-4 md:p-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold">{text.title}</h1>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">{text.description}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="outline" onClick={refreshData} disabled={loading}>
            <RefreshCw className={cn('size-4', loading && 'animate-spin')} />
            {inventory.refresh}
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="outline" disabled={exporting !== null}>
                {exporting ? <Spinner className="size-4" /> : <Download className="size-4" />}
                {text.export}
                <ChevronDown className="size-3.5 opacity-60" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuLabel>{text.exportScope}</DropdownMenuLabel>
              <DropdownMenuItem
                disabled={exporting !== null || !(data?.total ?? 0)}
                onSelect={() => { void exportEntities('current') }}
              >
                {text.exportCurrent}
                <span className="ml-auto pl-4 text-xs tabular-nums text-muted-foreground">
                  {data?.total.toLocaleString() ?? 0}
                </span>
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={exporting !== null || !allFilteredCount}
                onSelect={() => { void exportEntities('all') }}
              >
                {text.exportAll}
                <span className="ml-auto pl-4 text-xs tabular-nums text-muted-foreground">
                  {allFilteredCount.toLocaleString()}
                </span>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                disabled={exporting !== null || !selectedCount}
                onSelect={() => { void exportEntities('selected') }}
              >
                {text.exportSelected}
                <span className="ml-auto pl-4 text-xs tabular-nums text-muted-foreground">{selectedCount}</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button variant="outline" onClick={() => setImportOpen(true)}>
            <Upload className="size-4" />
            {text.import}
          </Button>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="size-4" />
            {text.create}
          </Button>
          <Button
            variant="outline"
            disabled={!selectedCount}
            title={selectedCount ? text.scanSelectedHint : text.scanHint}
            onClick={() => setScanOpen(true)}
          >
            <ScanLine className="size-4" />
            {text.scan}{selectedCount ? ` (${selectedCount})` : ''}
          </Button>
        </div>
      </div>

      <Tabs value={category} onValueChange={(value) => changeCategory(value as AssetLedgerEntityCategory)}>
        <TabsList className="h-auto w-full flex-wrap justify-start">
          {categories.map((value) => (
            <TabsTrigger key={value} value={value} className="gap-2">
              {categoryLabel(value)}
              <span className="rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">{categoryCount(value)}</span>
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {selectedCount > 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border bg-muted/40 px-3 py-2 text-xs" aria-live="polite">
          <span>{text.selectedCount.replace('{count}', String(selectedCount))}</span>
          <div className="flex flex-wrap items-center gap-1">
            <Button
              variant="outline"
              size="sm"
              disabled={batchAction !== null || !archivableSelection.length}
              title={text.batchArchiveHint}
              onClick={() => { void batchArchiveSelection(true) }}
            >
              {batchAction === 'archive' ? <Spinner className="size-3.5" /> : <Archive className="size-3.5" />}
              {text.batchArchive}
              {archivableSelection.length ? ` (${archivableSelection.length})` : ''}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={batchAction !== null || !restorableSelection.length}
              title={restorableSelection.length ? text.batchRestoreHint : text.batchRestoreUnavailableHint}
              onClick={() => { void batchArchiveSelection(false) }}
            >
              {batchAction === 'restore' ? <Spinner className="size-3.5" /> : <RotateCcw className="size-3.5" />}
              {text.batchRestore}
              {restorableSelection.length ? ` (${restorableSelection.length})` : ''}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={exporting !== null}
              title={text.exportSelected}
              onClick={() => { void exportEntities('selected') }}
            >
              {exporting === 'selected' ? <Spinner className="size-3.5" /> : <Download className="size-3.5" />}
              {text.export}
            </Button>
            <Button variant="ghost" size="sm" onClick={clearSelection}>{text.clearSelection}</Button>
          </div>
        </div>
      ) : null}

      <div className="rounded-lg border bg-card p-3">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-0 flex-1 basis-full md:basis-auto md:min-w-[240px]">
            <Search className="absolute left-3 top-2.5 size-4 text-muted-foreground" />
            <Input
              aria-label={inventory.search}
              className="pl-9"
              placeholder={text.searchPlaceholder}
              value={filters.keyword}
              onChange={(event) => updateFilter('keyword', event.target.value)}
            />
          </div>
          <Select value={filters.status} onValueChange={(value) => updateFilter('status', value)}>
            <SelectTrigger className="w-[132px]" aria-label={inventory.status}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{inventory.statusAll}</SelectItem>
              <SelectItem value="active">{inventory.statusActive}</SelectItem>
              <SelectItem value="stale">{inventory.statusStale}</SelectItem>
              <SelectItem value="retired">{inventory.statusRetired}</SelectItem>
            </SelectContent>
          </Select>
          <Select value={filters.source} onValueChange={(value) => updateFilter('source', value)}>
            <SelectTrigger className="w-[132px]" aria-label={text.source}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{text.sourceAll}</SelectItem>
              <SelectItem value="manual">{text.sourceManual}</SelectItem>
              <SelectItem value="import">{text.sourceImport}</SelectItem>
              <SelectItem value="scan">{text.scanned}</SelectItem>
              <SelectItem value="fofa">{text.sourceFofa}</SelectItem>
              <SelectItem value="hunter">{text.sourceHunter}</SelectItem>
              <SelectItem value="quake">{text.sourceQuake}</SelectItem>
              <SelectItem value="uncover">{text.sourceUncover}</SelectItem>
            </SelectContent>
          </Select>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <Checkbox checked={includeArchived} onCheckedChange={(value) => { setLoading(true); setIncludeArchived(Boolean(value)); setPage(1) }} />
            {text.showArchived}
          </label>
          {activeFilterCount > 0 ? (
            <Button
              variant="ghost"
              onClick={() => {
                setLoading(true)
                setFilters(initialFilters)
                setPage(1)
              }}
            >
              {text.reset}
            </Button>
          ) : null}
          <span className="ml-auto border-l border-border pl-3 text-xs tabular-nums text-muted-foreground">
            {text.total} {data?.total.toLocaleString() ?? '—'}
          </span>
        </div>
        {includeArchived ? (
          <div className="mt-2 flex items-start gap-1.5 text-xs text-muted-foreground">
            <Info className="mt-0.5 size-3.5 shrink-0" />
            <span>{text.archivedHint}</span>
          </div>
        ) : null}
      </div>

      <div className="relative rounded-lg border bg-card">
        {loading && data ? (
          <div
            role="status"
            aria-live="polite"
            aria-busy="true"
            className="asset-table-loading-surface absolute inset-0 z-20 flex items-center justify-center rounded-lg"
          >
            <div className="flex flex-col items-center gap-2 text-sm text-muted-foreground">
              <Spinner className="size-7" />
              <span>{text.switching}</span>
            </div>
          </div>
        ) : null}
        {error ? (
          <div role="alert" className="flex flex-wrap items-center justify-center gap-3 p-8">
            <p className="text-sm">{error}</p>
            <Button variant="outline" onClick={refreshData}>{inventory.retry}</Button>
          </div>
        ) : loading && !data ? (
          <div role="status" className="flex items-center justify-center gap-2 p-12">
            <Spinner />
            <span className="text-sm text-muted-foreground">{inventory.loading}</span>
          </div>
        ) : !items.length ? (
          <div className="p-12 text-center">
            <p className="font-medium">{activeFilterCount ? text.noMatch : text.noEntities}</p>
            <p className="mt-2 text-sm text-muted-foreground">{activeFilterCount ? text.noMatchHint : text.noEntitiesHint}</p>
            {!activeFilterCount ? (
              <div className="mt-4 flex justify-center gap-2">
                <Button onClick={() => setCreateOpen(true)}><Plus className="size-4" />{text.create}</Button>
                <Button variant="outline" onClick={() => setImportOpen(true)}><Upload className="size-4" />{text.import}</Button>
              </div>
            ) : null}
          </div>
        ) : (
          <>
            <div className="divide-y md:hidden" aria-busy={loading}>
              {items.map((entity) => (
                <article
                  key={entity.id}
                  className={cn('min-w-0 space-y-3 p-4', isEntityClickable(entity) && 'cursor-pointer', entity.archived && 'opacity-70', isSelected(entity) && 'bg-muted/40')}
                  role={isEntityClickable(entity) ? 'button' : undefined}
                  tabIndex={isEntityClickable(entity) ? 0 : undefined}
                  onClick={() => openEntity(entity)}
                  onKeyDown={(event) => {
                    if (isEntityClickable(entity) && (event.key === 'Enter' || event.key === ' ')) {
                      event.preventDefault()
                      openEntity(entity)
                    }
                  }}
                >
                  <div className="flex items-start gap-3">
                    <div
                      className="pt-0.5"
                      onClick={(event) => event.stopPropagation()}
                      onKeyDown={(event) => event.stopPropagation()}
                    >
                      <Checkbox
                        checked={isSelected(entity)}
                        onCheckedChange={(value) => toggleEntitySelection(entity, value === true)}
                        aria-label={text.selectAsset}
                      />
                    </div>
                    <div className="min-w-0 flex-1">{valueCell(entity)}</div>
                    <AssetStatusBadge status={entity.status} />
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    <SourceBadges sources={entity.sources} />
                  </div>
                  <dl className="grid grid-cols-[6rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
                    {category === 'root' ? (
                      <>
                        <dt className="text-muted-foreground">{text.subdomains}</dt><dd>{entity.subdomainCount ?? 0}</dd>
                        <dt className="text-muted-foreground">{text.ips}</dt><dd>{entity.ipCount ?? 0}</dd>
                        <dt className="text-muted-foreground">{text.ports}</dt><dd>{entity.portCount ?? 0}</dd>
                        <dt className="text-muted-foreground">{text.websites}</dt><dd>{entity.websiteCount ?? 0}</dd>
                      </>
                    ) : category === 'ip' ? (
                      <>
                        <dt className="text-muted-foreground">{text.resolvedDomains}</dt><dd>{domainCell(entity.domains)}</dd>
                        <dt className="text-muted-foreground">{text.ports}</dt><dd>{entity.portCount ?? 0}</dd>
                        <dt className="text-muted-foreground">{text.websites}</dt><dd>{entity.websiteCount ?? 0}</dd>
                      </>
                    ) : category === 'subdomain' ? (
                      <>
                        <dt className="text-muted-foreground">{text.rootDomain}</dt><dd className="break-all">{entity.rootDomain || '—'}</dd>
                        <dt className="text-muted-foreground">{text.associatedIPs}</dt><dd>{ipListCell(entity.ips)}</dd>
                        <dt className="text-muted-foreground">{text.ports}</dt><dd>{entity.portCount ?? 0}</dd>
                        <dt className="text-muted-foreground">{text.websites}</dt><dd>{entity.websiteCount ?? 0}</dd>
                      </>
                    ) : category === 'port' ? (
                      <>
                        <dt className="text-muted-foreground">{text.service}</dt><dd>{entity.service || '—'}</dd>
                        <dt className="text-muted-foreground">{text.protocol}</dt><dd>{formatPortProtocol(entity.protocol, entity.transport)}</dd>
                      </>
                    ) : (
                      <>
                        <dt className="text-muted-foreground">{text.websiteTitle}</dt><dd className="break-all">{entity.title || '—'}</dd>
                        <dt className="text-muted-foreground">{text.statusCode}</dt><dd>{entity.httpStatus || '—'}</dd>
                        <dt className="text-muted-foreground">{text.fingerprints}</dt><dd>{fingerprintCell(entity.fingerprints)}</dd>
                      </>
                    )}
                    <dt className="text-muted-foreground">{text.lastSeen}</dt><dd>{formatDate(entity.lastSeenAt, locale)}</dd>
                  </dl>
                  <div className="flex justify-end">{actionCell(entity)}</div>
                </article>
              ))}
            </div>
            <div className="hidden md:block">
              <Table className="min-w-[1120px] table-fixed">
                <TableHeader>
                  <TableRow>
                    {category === 'root' ? (
                      <>
                        {selectionHeader(text.rootDomain, 'w-[26%]')}
                        <TableHead className="w-[11%]">{text.subdomains}</TableHead>
                        <TableHead className="w-[11%]">{text.ips}</TableHead>
                        <TableHead className="w-[11%]">{text.ports}</TableHead>
                        <TableHead className="w-[11%]">{text.websites}</TableHead>
                      </>
                    ) : category === 'ip' ? (
                      <>
                        {selectionHeader(text.asset, 'w-[20%]')}
                        <TableHead className="w-[32%]">{text.resolvedDomains}</TableHead>
                        <TableHead className="w-[9%]">{text.ports}</TableHead>
                        <TableHead className="w-[9%]">{text.websites}</TableHead>
                      </>
                    ) : category === 'subdomain' ? (
                      <>
                        {selectionHeader(`${text.subdomainLabel} / ${text.rootDomain}`, 'w-[34%]')}
                        <TableHead className="w-[20%]">{text.associatedIPs}</TableHead>
                        <TableHead className="w-[8%]">{text.ports}</TableHead>
                        <TableHead className="w-[8%]">{text.websites}</TableHead>
                      </>
                    ) : category === 'port' ? (
                      <>
                        {selectionHeader(text.hostPort, 'w-[30%]')}
                        <TableHead className="w-[14%]">{text.protocol}</TableHead>
                        <TableHead className="w-[26%]">{text.service}</TableHead>
                      </>
                    ) : (
                      <>
                        {selectionHeader(text.websiteUrl, 'w-[28%]')}
                        <TableHead className="w-[18%]">{text.websiteTitle}</TableHead>
                        <TableHead className="w-[8%]">{text.statusCode}</TableHead>
                        <TableHead className="w-[16%]">{text.fingerprints}</TableHead>
                      </>
                    )}
                    <TableHead className="w-[15%]">{text.sources}</TableHead>
                    <TableHead className="w-[10%]">{text.lastSeen}</TableHead>
                    <TableHead className="w-[5%] text-center">{t.common.action}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((entity) => (
                    <TableRow
                      key={entity.id}
                      data-state={isSelected(entity) ? 'selected' : undefined}
                      className={cn(isEntityClickable(entity) && 'cursor-pointer', entity.archived && 'opacity-70')}
                      tabIndex={isEntityClickable(entity) ? 0 : undefined}
                      onClick={() => openEntity(entity)}
                      onKeyDown={(event) => {
                        if (isEntityClickable(entity) && (event.key === 'Enter' || event.key === ' ')) {
                          event.preventDefault()
                          openEntity(entity)
                        }
                      }}
                    >
                      {category === 'root' ? (
                        <>
                          {selectionCell(entity, valueCell(entity))}
                          <TableCell>{entity.subdomainCount ?? 0}</TableCell>
                          <TableCell>{entity.ipCount ?? 0}</TableCell>
                          <TableCell>{entity.portCount ?? 0}</TableCell>
                          <TableCell>{entity.websiteCount ?? 0}</TableCell>
                        </>
                      ) : category === 'ip' ? (
                        <>
                          {selectionCell(entity, valueCell(entity))}
                          <TableCell className="whitespace-normal">{domainCell(entity.domains)}</TableCell>
                          <TableCell>{entity.portCount ?? 0}</TableCell>
                          <TableCell>{entity.websiteCount ?? 0}</TableCell>
                        </>
                      ) : category === 'subdomain' ? (
                        <>
                          {selectionCell(entity, subdomainCell(entity))}
                          <TableCell className="whitespace-normal">{ipListCell(entity.ips)}</TableCell>
                          <TableCell>{entity.portCount ?? 0}</TableCell>
                          <TableCell>{entity.websiteCount ?? 0}</TableCell>
                        </>
                      ) : category === 'port' ? (
                        <>
                          {selectionCell(entity, valueCell(entity))}
                          <TableCell className="text-xs">{formatPortProtocol(entity.protocol, entity.transport)}</TableCell>
                          <TableCell className="whitespace-normal">{truncateCell(entity.service)}</TableCell>
                        </>
                      ) : (
                        <>
                          {selectionCell(entity, valueCell(entity))}
                          <TableCell className="max-w-[200px] truncate text-xs" title={entity.title}>{entity.title || '—'}</TableCell>
                          <TableCell className="text-xs">{entity.httpStatus || '—'}</TableCell>
                          <TableCell>{fingerprintCell(entity.fingerprints)}</TableCell>
                        </>
                      )}
                      <TableCell>{sourceCell(entity)}</TableCell>
                      <TableCell>{lastSeenCell(entity)}</TableCell>
                      <TableCell className="text-center">{actionCell(entity)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            <div className="border-t px-4 py-3">
              <PaginationPro
                current={data?.page ?? page}
                total={data?.total ?? 0}
                pageSize={pageSize}
                onChange={(next, size) => { setLoading(true); setPage(next); setPageSize(size) }}
                showQuickJumper
                disabled={loading}
              />
            </div>
          </>
        )}
      </div>

      <p className="text-xs text-muted-foreground">{text.hint}</p>

      <AssetLedgerImportDialog
        key={importOpen ? 'import-open' : 'import-closed'}
        open={importOpen}
        onOpenChange={setImportOpen}
        onImported={refreshData}
      />
      <AssetLedgerCreateDialog
        key={createOpen ? 'create-open' : 'create-closed'}
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={refreshData}
      />
      <AssetScanDialog
        key={scanOpen ? 'scan-open' : 'scan-closed'}
        open={scanOpen}
        onOpenChange={setScanOpen}
        entities={Object.values(selectedEntities)}
        activeCategory={category}
        onCreated={refreshData}
      />
      <AssetDetailSheet
        assetId={selectedAssetId}
        open={Boolean(selectedAssetId)}
        onOpenChange={(open) => { if (!open) setSelectedAssetId(null) }}
        onChanged={refreshData}
      />
      <AssetRootSheet
        entity={selectedRoot}
        open={Boolean(selectedRoot)}
        onOpenChange={(open) => { if (!open) setSelectedRoot(null) }}
        onArchive={archiveRootFromSheet}
        onViewSubdomains={viewRootSubdomains}
      />

      <AlertDialog
        open={scopeDialog !== null}
        onOpenChange={(open) => {
          if (!open && !scopeAction) {
            setScopeDialog(null)
            setScopeError('')
          }
        }}
      >
        <AlertDialogContent className="sm:max-w-[460px]">
          {scopeDialog ? (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  {scopeDialog.mode === 'single'
                    ? text.archiveRootTitle
                    : text.batchArchiveTitle.replace('{count}', String(scopeDialog.entities.length))}
                </AlertDialogTitle>
                <AlertDialogDescription>
                  {scopeDialog.mode === 'single'
                    ? text.archiveRootDescription
                    : text.batchScopeSummary.replace('{count}', String(scopeDialogSummary?.roots ?? 0))}
                </AlertDialogDescription>
              </AlertDialogHeader>
              {scopeDialog.mode === 'single' ? (
                <p className="break-all font-mono text-sm">{scopeDialog.entities[0]?.value}</p>
              ) : null}
              <div className="grid grid-cols-3 gap-2">
                {[
                  { label: text.subdomains, value: scopeDialogSummary?.subdomains ?? 0 },
                  { label: text.ports, value: scopeDialogSummary?.ports ?? 0 },
                  { label: text.websites, value: scopeDialogSummary?.websites ?? 0 },
                ].map((item) => (
                  <div key={item.label} className="rounded-md border bg-muted/40 px-2 py-1 text-center">
                    <p className="text-xs text-muted-foreground">{item.label}</p>
                    <p className="text-sm font-medium tabular-nums">{item.value}</p>
                  </div>
                ))}
              </div>
              <div className="space-y-1 text-xs text-muted-foreground">
                <p>{scopeDialog.mode === 'single' ? text.archiveRootNewHint : text.archiveScopeNewHint}</p>
                <p>{text.archiveIpUnaffected}</p>
              </div>
              {scopeError ? <p className="text-xs text-destructive">{scopeError}</p> : null}
              <AlertDialogFooter>
                <AlertDialogCancel disabled={scopeAction !== null}>{t.common.cancel}</AlertDialogCancel>
                <AlertDialogAction
                  disabled={scopeAction !== null}
                  onClick={(event) => {
                    event.preventDefault()
                    void confirmScopeArchive()
                  }}
                >
                  {scopeAction ? <Spinner className="size-4" /> : <Archive className="size-4" />}
                  {scopeDialog.mode === 'single' ? text.archiveRoot : text.batchArchive}
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          ) : null}
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function AssetLedgerImportDialog({
  open,
  onOpenChange,
  onImported,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImported: () => void
}) {
  const { t } = useI18n()
  const text = t.assetLedger
  const [content, setContent] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [groupKey, setGroupKey] = useState('')
  const [preview, setPreview] = useState<AssetImportPreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState('')

  const handlePreview = async () => {
    setError('')
    setPreviewing(true)
    try {
      setPreview(await importAssetLedger({ content, file: file || undefined, groupKey, dryRun: true }))
    } catch (err) {
      setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      setPreviewing(false)
    }
  }

  const handleImport = async () => {
    setError('')
    setImporting(true)
    try {
      const result = await importAssetLedger({ content, file: file || undefined, groupKey, dryRun: false })
      toast.success(text.importSuccess.replace('{count}', String(result.created ?? result.valid)))
      onImported()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      setImporting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{text.importTitle}</DialogTitle>
          <DialogDescription>{text.importHint}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="asset-import-file">{text.importFile}</Label>
            <Input
              id="asset-import-file"
              type="file"
              accept=".csv,.txt,text/csv,text/plain"
              onChange={(event) => {
                setFile(event.target.files?.[0] || null)
                setPreview(null)
              }}
            />
            {file ? <p className="text-xs text-muted-foreground">{file.name}</p> : null}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asset-import-content">{text.importPaste}</Label>
            <Textarea
              id="asset-import-content"
              rows={7}
              value={content}
              placeholder={text.importPlaceholder}
              onChange={(event) => {
                setContent(event.target.value)
                setPreview(null)
              }}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asset-import-group">{text.group}</Label>
            <Input
              id="asset-import-group"
              value={groupKey}
              placeholder={text.groupPlaceholder}
              onChange={(event) => setGroupKey(event.target.value)}
            />
          </div>
          {preview ? (
            <div className="rounded-md border bg-muted/30 p-3 text-xs">
              <p className="font-medium">
                {text.previewSummary
                  .replace('{valid}', String(preview.valid))
                  .replace('{invalid}', String(preview.invalid))
                  .replace('{duplicates}', String(preview.duplicates))}
              </p>
              <div className="mt-2 flex flex-wrap gap-2 text-muted-foreground">
                <span>{text.typeDomain}: {preview.byType.domain ?? 0}</span>
                <span>{text.typeIP}: {preview.byType.ip ?? 0}</span>
                <span>{text.typeURL}: {preview.byType.url ?? 0}</span>
              </div>
              {preview.errors.length ? (
                <div className="mt-2 max-h-32 overflow-y-auto">
                  <p className="mb-1 font-medium">{text.previewErrors}</p>
                  {preview.errors.map((item, index) => (
                    <p key={`${item.row}-${index}`} className="text-destructive">
                      {item.row}: {item.value || '—'} · {item.reason}
                    </p>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t.common.cancel}</Button>
          <Button variant="outline" onClick={handlePreview} disabled={previewing || importing || (!content.trim() && !file)}>
            {previewing ? <Spinner className="size-4" /> : null}
            {text.preview}
          </Button>
          <Button onClick={handleImport} disabled={importing || (!content.trim() && !file)}>
            {importing ? <Spinner className="size-4" /> : null}
            {text.importAction}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function AssetLedgerCreateDialog({
  open,
  onOpenChange,
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onCreated: () => void
}) {
  const { t } = useI18n()
  const text = t.assetLedger
  const [type, setType] = useState('auto')
  const [value, setValue] = useState('')
  const [name, setName] = useState('')
  const [tags, setTags] = useState('')
  const [notes, setNotes] = useState('')
  const [groupKey, setGroupKey] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const handleCreate = async () => {
    setError('')
    setSaving(true)
    try {
      await createAssetLedger({
        type: type === 'auto' ? '' : type,
        value,
        name,
        notes,
        tags: tags.split(',').map((item) => item.trim()).filter(Boolean),
        groupKey,
      })
      toast.success(text.createSuccess)
      onCreated()
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : text.loadFailed)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{text.createTitle}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="grid grid-cols-[120px_minmax(0,1fr)] gap-2">
            <div className="space-y-1.5">
              <Label>{text.type}</Label>
              <Select value={type} onValueChange={setType}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">{text.typeAuto}</SelectItem>
                  <SelectItem value="domain">{text.typeDomain}</SelectItem>
                  <SelectItem value="ip">{text.typeIP}</SelectItem>
                  <SelectItem value="url">{text.typeURL}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="asset-create-value">{text.value}</Label>
              <Input id="asset-create-value" value={value} placeholder={text.valuePlaceholder} onChange={(event) => setValue(event.target.value)} />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asset-create-name">{text.name}</Label>
            <Input id="asset-create-name" value={name} placeholder={text.namePlaceholder} onChange={(event) => setName(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asset-create-tags">{text.tags}</Label>
            <Input id="asset-create-tags" value={tags} placeholder={text.tagsPlaceholder} onChange={(event) => setTags(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asset-create-group">{text.group}</Label>
            <Input id="asset-create-group" value={groupKey} placeholder={text.groupPlaceholder} onChange={(event) => setGroupKey(event.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="asset-create-notes">{text.notes}</Label>
            <Textarea id="asset-create-notes" rows={3} value={notes} placeholder={text.notesPlaceholder} onChange={(event) => setNotes(event.target.value)} />
          </div>
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{t.common.cancel}</Button>
          <Button onClick={handleCreate} disabled={saving || !value.trim()}>
            {saving ? <Spinner className="size-4" /> : null}
            {t.common.save}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
