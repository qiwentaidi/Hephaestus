'use client'

import { useEffect, useId, useRef, useState } from 'react'
import Link from 'next/link'
import { ExternalLink, RefreshCw, Search } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field'
import { PaginationPro } from '@/components/ui/pagination-pro'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Sheet, SheetClose, SheetContent, SheetFooter, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'
import { AssetDetailSheet } from './AssetDetailSheet'
import { useIsMobile } from '@/hooks/use-mobile'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { fetchWithAuth } from '@/lib/api'
import { buildAssetTopologyQuery } from '@/lib/asset-center-filters'
import { useI18n } from '@/lib/i18n'

interface TopologyRow {
  id: string
  assetId?: string
  type: 'domain' | 'ip'
  rootDomain: string
  domain: string
  ip: string
  endpoint: string
  endpointHost: string
  port: number
  protocol: string
  service: string
  technologies: string[]
  sourceTaskIds: string[]
  sourceTypes?: string[]
  lastSeenAt: string
  status: string
}
interface SourceTask {
  id: string
  name: string
}
interface TableData {
  items: TopologyRow[]
  total: number
  page: number
  pageSize: number
  tasks: SourceTask[]
  stats: { roots: number; domains: number; ips: number; endpoints: number }
}
const initialFilters = {
  keyword: '',
  type: 'all',
  status: 'all',
  port: '',
  service: '',
  technology: '',
}

function AssetStatusBadge({ status }: { status: string }) {
  const { t } = useI18n()
  const labels: Record<string, string> = {
    active: t.assetInventory.statusActive,
    stale: t.assetInventory.statusStale,
    retired: t.assetInventory.statusRetired,
  }
  const classes: Record<string, string> = {
    active: 'border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300',
    stale: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    retired: 'border-muted-foreground/30 bg-muted text-muted-foreground',
  }
  return <Badge variant="outline" className={classes[status] || classes.retired}>{labels[status] || status || t.assetInventory.statusUnknown}</Badge>
}

function TechnologyLabels({ values }: { values: string[] }) {
  const { t } = useI18n()
  const label = (value: string) => (
    <Badge
      key={value}
      variant="outline"
      className="max-w-full break-words whitespace-normal text-xs font-normal"
      title={value}
    >
      {value}
    </Badge>
  )
  if (!values.length) return <span className="text-muted-foreground">—</span>
  return (
    <div className="flex flex-wrap items-center gap-1">
      {values.slice(0, 3).map(label)}
      {values.length > 3 ? (
        <Popover>
          <PopoverTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-1 text-xs text-muted-foreground"
              aria-label={t.assetInventory.allTechnologies}
            >
              +{values.length - 3}
            </Button>
          </PopoverTrigger>
          <PopoverContent
            className="max-h-80 w-80 overflow-y-auto"
            align="start"
          >
            <p className="mb-3 text-sm font-medium">
              {t.assetInventory.technologies} ({values.length})
            </p>
            <div className="flex flex-wrap gap-1">{values.map(label)}</div>
          </PopoverContent>
        </Popover>
      ) : null}
    </div>
  )
}

function TaskSources({
  ids,
  tasks,
  sourceTypes = [],
}: {
  ids: string[]
  tasks: Map<string, SourceTask>
  sourceTypes?: string[]
}) {
  const { t } = useI18n()
  const text = t.assetInventory
  const ledger = t.assetLedger
  const sourceLabel = (source: string) => {
    switch (source) {
      case 'manual':
        return ledger.sourceManual
      case 'import':
        return ledger.sourceImport
      case 'portscan':
        return ledger.sourcePortscan
      case 'fingerprint':
      case 'webscan':
        return ledger.sourceFingerprint
      case 'subdomain':
        return ledger.sourceSubdomain
      case 'dirscan':
        return ledger.sourceDirscan
      case 'jsscan':
        return ledger.sourceJsscan
      case 'jsscan-vulnerability':
        return ledger.sourceJsscanVulnerability
      case 'webscan-vulnerability':
        return ledger.sourceVulnerability
      case 'findings':
        return ledger.sourceFindings
      case 'ai':
        return ledger.sourceAI
      case 'fofa':
        return ledger.sourceFofa
      case 'hunter':
        return ledger.sourceHunter
      case 'quake':
        return ledger.sourceQuake
      case 'uncover':
        return ledger.sourceUncover
      default:
        return source
    }
  }
  if (!ids.length && sourceTypes.length) {
    return (
      <div className="flex flex-wrap items-center gap-1">
        {sourceTypes.map((source) => (
          <Badge key={source} variant="secondary" className="font-normal">
            {sourceLabel(source)}
          </Badge>
        ))}
      </div>
    )
  }
  if (!ids.length) return <span className="text-muted-foreground">—</span>
  const first = tasks.get(ids[0])?.name || ids[0]
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          variant="ghost"
          className="h-auto w-full min-w-0 justify-start px-0 py-1 text-left font-normal"
          aria-label={`${text.sourceTasks}: ${ids.length}`}
        >
          <span className="block min-w-0">
            <span className="block text-sm tabular-nums">
              {ids.length} {text.taskCount}
            </span>
            <span
              className="mt-1 block truncate text-xs text-muted-foreground"
              title={first}
            >
              {first}
            </span>
          </span>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="max-h-80 w-80 overflow-y-auto" align="end">
        <p className="text-sm font-medium">
          {text.sourceTasks} ({ids.length})
        </p>
        <p className="mt-1 text-xs text-muted-foreground">{text.sourceHint}</p>
        <div className="mt-3 space-y-2">
          {ids.map((id) => (
            <Link
              key={id}
              href={`/scan/vuln-scan/detail?${new URLSearchParams({ taskId: id })}`}
              className="flex items-center justify-between gap-3 rounded-md border p-2 hover:bg-muted"
            >
              <span className="min-w-0">
                <span className="block break-words text-sm">
                  {tasks.get(id)?.name || id}
                </span>
                <span className="mt-1 block break-all font-mono text-xs text-muted-foreground">
                  {id}
                </span>
              </span>
              <ExternalLink className="size-4 shrink-0 text-muted-foreground" />
            </Link>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

export function AssetInventoryView() {
  const { t, language } = useI18n()
  const text = t.assetInventory
  const filterId = useId()
  const isMobile = useIsMobile()
  const filterTitleRef = useRef<HTMLHeadingElement>(null)
  const [filters, setFilters] = useState(initialFilters)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [refresh, setRefresh] = useState(0)
  const [data, setData] = useState<TableData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [selectedAssetId, setSelectedAssetId] = useState<string | null>(null)

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible')
        setRefresh((value) => value + 1)
    }, 30000)
    return () => window.clearInterval(timer)
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(async () => {
      setLoading(true)
      setError('')
      const port = filters.port.trim()
      if (
        port &&
        (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535)
      ) {
        setError(text.invalidPort)
        setLoading(false)
        return
      }
      try {
        const params = buildAssetTopologyQuery({ ...filters, port }, page, pageSize)
        const response = await fetchWithAuth(
          `/api/v1/assets/topology/table?${params}`,
          { signal: controller.signal },
        )
        const payload = await response.json().catch(() => null)
        if (!response.ok || !payload?.success || !payload.data)
          throw new Error(
            payload?.error || `${text.error} (HTTP ${response.status})`,
          )
        if (!controller.signal.aborted) {
          setData(payload.data as TableData)
          setPage(payload.data.page)
        }
      } catch (err) {
        if (!controller.signal.aborted)
          setError(err instanceof Error ? err.message : text.error)
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }, 250)
    return () => {
      controller.abort()
      window.clearTimeout(timer)
    }
  }, [filters, page, pageSize, refresh, text.error, text.invalidPort])

  const updateFilter = (key: keyof typeof initialFilters, value: string) => {
    setFilters((previous) => ({ ...previous, [key]: value }))
    setPage(1)
  }
  const tasks = new Map(data?.tasks.map((task) => [task.id, task]))
  const stats = data?.stats
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'
  const advancedFilters = [
    { key: 'port' as const, label: text.port, value: filters.port.trim() },
    {
      key: 'service' as const,
      label: text.service,
      value: filters.service.trim(),
    },
    {
      key: 'technology' as const,
      label: text.technologies,
      value: filters.technology.trim(),
    },
  ]
  const activeAdvancedFilters = advancedFilters.filter((filter) => filter.value)

  const filterTrigger = (
    <Button variant="outline" aria-label={text.moreFilters}>
      {text.moreFilters}
      {activeAdvancedFilters.length > 0 ? (
        <span className="ml-2 rounded bg-muted px-1.5 text-xs tabular-nums">
          {activeAdvancedFilters.length}
        </span>
      ) : null}
    </Button>
  )
  const filterFields = (
    <FieldGroup className="gap-4">
      {advancedFilters.map((filter) => (
        <Field key={filter.key}>
          <FieldLabel htmlFor={`${filterId}-${filter.key}`}>
            {filter.label}
          </FieldLabel>
          <Input
            id={`${filterId}-${filter.key}`}
            className="h-11 text-base md:h-9 md:text-sm"
            aria-label={filter.label}
            inputMode={filter.key === 'port' ? 'numeric' : undefined}
            placeholder={
              filter.key === 'port'
                ? '1–65535'
                : filter.key === 'service'
                  ? 'HTTPS / SSH / nginx'
                  : 'Nginx / GitLab'
            }
            value={filters[filter.key]}
            onChange={(event) =>
              updateFilter(filter.key, event.target.value)
            }
          />
        </Field>
      ))}
    </FieldGroup>
  )

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col gap-4 overflow-y-auto p-4 md:p-6">
      <div className="flex items-center justify-between gap-3">
        <h1 className="text-xl font-semibold">{text.title}</h1>
        <Button
          variant="outline"
          onClick={() => setRefresh((value) => value + 1)}
          disabled={loading}
        >
          <RefreshCw className="mr-2 size-4" />
          {text.refresh}
        </Button>
      </div>
      <div className="grid grid-cols-2 rounded-lg border bg-card md:grid-cols-4">
        {[
          { label: text.rootDomain, value: stats?.roots },
          { label: text.domainAssets, value: stats?.domains },
          { label: text.ipAssets, value: stats?.ips },
          { label: text.endpoints, value: stats?.endpoints },
        ].map((stat, index) => (
          <div
            key={stat.label}
            className={`flex items-center gap-3 px-4 py-3 ${index % 2 ? 'border-l' : ''} ${index > 1 ? 'border-t md:border-t-0' : ''} ${index === 2 ? 'md:border-l' : ''}`}
          >
            <span className="text-xs text-muted-foreground">{stat.label}</span>
            <span className="text-lg font-semibold tabular-nums">
              {stat.value?.toLocaleString() ?? '—'}
            </span>
          </div>
        ))}
      </div>
      <div className="rounded-lg border bg-card p-3">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-0 basis-full md:basis-auto md:min-w-[220px] flex-1">
            <Search className="absolute left-3 top-2.5 size-4 text-muted-foreground" />
            <Input
              aria-label={text.search}
              placeholder={text.searchCompact}
              className="pl-9"
              value={filters.keyword}
              onChange={(event) => updateFilter('keyword', event.target.value)}
            />
          </div>
          <Select
            value={filters.type}
            onValueChange={(value) => updateFilter('type', value)}
          >
            <SelectTrigger className="w-[144px] min-w-0" aria-label={text.type}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{text.allPathTypes}</SelectItem>
              <SelectItem value="domain">{text.domainPaths}</SelectItem>
              <SelectItem value="ip">{text.ipPaths}</SelectItem>
            </SelectContent>
          </Select>
          <Select
            value={filters.status}
            onValueChange={(value) => updateFilter('status', value)}
          >
            <SelectTrigger data-testid="asset-status-filter" className="w-[144px] min-w-0" aria-label={text.status}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{text.statusAll}</SelectItem>
              <SelectItem value="active">{text.statusActive}</SelectItem>
              <SelectItem value="stale">{text.statusStale}</SelectItem>
              <SelectItem value="retired">{text.statusRetired}</SelectItem>
            </SelectContent>
          </Select>
          {isMobile ? (
            <Sheet>
              <SheetTrigger asChild>{filterTrigger}</SheetTrigger>
              <SheetContent
                side="bottom"
                aria-describedby={undefined}
                className="max-h-[85dvh] gap-0 overflow-hidden rounded-t-2xl [&>button]:top-2 [&>button]:right-2 [&>button]:flex [&>button]:size-11 [&>button]:items-center [&>button]:justify-center"
                onOpenAutoFocus={(event) => {
                  event.preventDefault()
                  filterTitleRef.current?.focus()
                }}
              >
                <SheetHeader className="shrink-0 pr-14">
                  <SheetTitle ref={filterTitleRef} tabIndex={-1} className="outline-none">
                    {text.moreFilters}
                  </SheetTitle>
                </SheetHeader>
                <div className="min-h-0 overflow-y-auto overscroll-contain px-4 py-2">
                  {filterFields}
                </div>
                <SheetFooter className="shrink-0 pb-[max(1rem,env(safe-area-inset-bottom))]">
                  <SheetClose asChild><Button className="h-11 w-full">{t.common.confirm}</Button></SheetClose>
                </SheetFooter>
              </SheetContent>
            </Sheet>
          ) : (
            <Popover>
              <PopoverTrigger asChild>{filterTrigger}</PopoverTrigger>
              <PopoverContent
                align="end"
                className="flex w-80 max-w-[calc(100vw-2rem)] flex-col gap-4"
                aria-labelledby={`${filterId}-title`}
              >
                <p id={`${filterId}-title`} className="text-sm font-medium">{text.moreFilters}</p>
                {filterFields}
              </PopoverContent>
            </Popover>
          )}
          <Button
            variant="outline"
            onClick={() => {
              setFilters(initialFilters)
              setPage(1)
            }}
          >
            {text.reset}
          </Button>
        </div>
        <div className="mt-3 flex flex-wrap items-center gap-2 border-t pt-2">
          {activeAdvancedFilters.map((filter) => (
            <Button
              key={filter.key}
              variant="outline"
              size="sm"
              className="h-6 min-w-0 max-w-full gap-2 px-2 text-xs font-normal"
              onClick={() => updateFilter(filter.key, '')}
              aria-label={`${text.clearFilter}: ${filter.label}`}
              title={`${filter.label}: ${filter.value}`}
            >
              <span className="truncate">
                {filter.label}: {filter.value}
              </span>
              <span
                aria-hidden="true"
                className="shrink-0 text-muted-foreground"
              >
                ×
              </span>
            </Button>
          ))}
          <span
            className="ml-auto text-xs tabular-nums text-muted-foreground"
            aria-live="polite"
          >
            {data?.total.toLocaleString() ?? '—'} {text.pathCount}
          </span>
        </div>
      </div>
      <div className="rounded-lg border bg-card">
        {error ? (
          <div
            role="alert"
            className="flex flex-wrap items-center justify-center gap-3 p-8"
          >
            <p className="text-sm">{error}</p>
            <Button
              variant="outline"
              onClick={() => setRefresh((value) => value + 1)}
            >
              {text.retry}
            </Button>
          </div>
        ) : loading && !data ? (
          <div
            role="status"
            className="flex items-center justify-center gap-2 p-12"
          >
            <Spinner />
            <span className="text-sm text-muted-foreground">
              {text.loading}
            </span>
          </div>
        ) : !data?.items.length ? (
          <div className="p-12 text-center">
            <p className="font-medium">{text.empty}</p>
            <p className="mt-2 text-sm text-muted-foreground">
              {text.emptyHint}
            </p>
          </div>
        ) : (
          <>
          <div className="divide-y md:hidden" aria-busy={loading}>
            {data.items.map((row) => (
              <article
                key={row.id}
                data-testid="asset-row"
                className={row.assetId ? 'min-w-0 cursor-pointer space-y-3 p-4' : 'min-w-0 space-y-3 p-4'}
                role={row.assetId ? 'button' : undefined}
                tabIndex={row.assetId ? 0 : undefined}
                onClick={() => row.assetId && setSelectedAssetId(row.assetId)}
                onKeyDown={(event) => {
                  if (row.assetId && (event.key === 'Enter' || event.key === ' ')) {
                    event.preventDefault()
                    setSelectedAssetId(row.assetId)
                  }
                }}
              >
                <div className="flex items-center justify-between gap-3"><h2 className="break-all font-mono text-sm font-medium">{row.domain || row.ip || row.rootDomain || '—'}</h2><AssetStatusBadge status={row.status} /></div>
                <dl className="grid grid-cols-[5rem_minmax(0,1fr)] gap-x-3 gap-y-2 text-xs">
                  <dt className="text-muted-foreground">{text.rootDomain}</dt><dd className="break-all">{row.rootDomain || '—'}</dd>
                  <dt className="text-muted-foreground">{text.ips}</dt><dd className="break-all font-mono">{row.ip || (row.endpoint && row.type === 'domain' ? text.directEndpoint : '—')}</dd>
                  <dt className="text-muted-foreground">{text.portProtocol}</dt><dd className="break-all font-mono">{row.endpoint ? `${row.port || '—'} / ${row.protocol.toUpperCase()} · ${row.endpoint}` : '—'}</dd>
                  <dt className="text-muted-foreground">{text.technologies}</dt><dd className="min-w-0"><TechnologyLabels values={row.technologies} /></dd>
                  <dt className="self-center text-muted-foreground">{text.sourceTasks}</dt><dd className="min-w-0"><TaskSources ids={row.sourceTaskIds} tasks={tasks} sourceTypes={row.sourceTypes} /></dd>
                  <dt className="text-muted-foreground">{text.lastSeen}</dt><dd>{row.lastSeenAt && !row.lastSeenAt.startsWith('0001-') ? new Date(row.lastSeenAt).toLocaleString(locale, { hour12: false }) : '—'}</dd>
                </dl>
              </article>
            ))}
          </div>
          <div className="hidden md:block">
          <Table className="min-w-[1300px] table-fixed" aria-busy={loading}>
            <colgroup>
              <col className="w-[11%]" />
              <col className="w-[21%]" />
              <col className="w-[14%]" />
              <col className="w-[14%]" />
              <col className="w-[20%]" />
              <col className="w-[11%]" />
              <col className="w-[9%]" />
              <col className="w-[8%]" />
            </colgroup>
            <TableHeader>
              <TableRow>
                {[
                  text.rootDomain,
                  text.subdomain,
                  text.ips,
                  text.portProtocol,
                  text.technologies,
                  text.sourceTasks,
                  text.lastSeen,
                  text.status,
                ].map((label) => (
                  <TableHead key={label}>{label}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {data.items.map((row) => {
                const observed =
                  row.lastSeenAt && !row.lastSeenAt.startsWith('0001-')
                    ? new Date(row.lastSeenAt)
                    : null
                return (
                  <TableRow
                    key={row.id}
                    data-testid="asset-row"
                    className={row.assetId ? 'cursor-pointer' : undefined}
                    tabIndex={row.assetId ? 0 : undefined}
                    onClick={() => row.assetId && setSelectedAssetId(row.assetId)}
                    onKeyDown={(event) => {
                      if (row.assetId && (event.key === 'Enter' || event.key === ' ')) {
                        event.preventDefault()
                        setSelectedAssetId(row.assetId)
                      }
                    }}
                  >
                    <TableCell className="whitespace-normal py-3 font-mono text-xs">
                      <span className="break-all">{row.rootDomain || '—'}</span>
                    </TableCell>
                    <TableCell className="whitespace-normal py-3 font-mono text-sm">
                      <span className="break-all">{row.domain || '—'}</span>
                    </TableCell>
                    <TableCell className="whitespace-normal py-3">
                      <span className="break-all font-mono text-xs">
                        {row.ip || '—'}
                      </span>
                      {row.endpoint && !row.ip && row.type === 'domain' ? (
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          {text.directEndpoint}
                        </p>
                      ) : null}
                    </TableCell>
                    <TableCell className="py-3">
                      {row.endpoint ? (
                        <>
                          <p className="font-mono text-sm">
                            {row.port > 0 ? row.port : '—'}
                            <span className="ml-1.5 text-xs text-muted-foreground">
                              / {row.protocol.toUpperCase() || '—'}
                            </span>
                          </p>
                          <p
                            className="mt-1 truncate font-mono text-[11px] text-muted-foreground"
                            title={row.endpoint}
                          >
                            {row.endpoint}
                          </p>
                        </>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-normal py-3">
                      <TechnologyLabels values={row.technologies} />
                    </TableCell>
                    <TableCell className="py-3">
                      <TaskSources ids={row.sourceTaskIds} tasks={tasks} sourceTypes={row.sourceTypes} />
                    </TableCell>
                    <TableCell className="py-3 text-xs text-muted-foreground">
                      {observed ? (
                        <>
                          <p>{observed.toLocaleDateString(locale)}</p>
                          <p className="mt-1 text-[11px]">
                            {observed.toLocaleTimeString(locale, {
                              hour12: false,
                            })}
                          </p>
                        </>
                      ) : (
                        '—'
                      )}
                    </TableCell>
                    <TableCell className="py-3"><AssetStatusBadge status={row.status} /></TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
          </div>
          </>
        )}
        <div className="border-t px-4 py-3">
          <PaginationPro
            current={data?.page ?? page}
            total={data?.total ?? 0}
            pageSize={pageSize}
            onChange={(next, size) => {
              setPage(next)
              setPageSize(size)
            }}
            showQuickJumper
            disabled={loading}
          />
        </div>
      </div>
      <p className="text-xs text-muted-foreground">{text.historyHint}</p>
      <AssetDetailSheet
        assetId={selectedAssetId}
        open={Boolean(selectedAssetId)}
        onOpenChange={(open) => {
          if (!open) setSelectedAssetId(null)
        }}
      />
    </div>
  )
}
