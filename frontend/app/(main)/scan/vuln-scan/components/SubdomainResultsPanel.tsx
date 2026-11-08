'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { ResultsTableView } from './SubdomainResultsTableView'
import { fetchWithAuth } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { saveBlobAsFile } from '@/lib/download'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'

type SubdomainResult = {
  domain: string
  subdomain: string
  ips: string[]
  assetType?: string
  assetName?: string
  source: string
  sources?: string[]
  isWildcard?: boolean
}

interface SubdomainResultsPanelProps {
  taskId?: string
  totalHint?: number
  onTotalChange?: (total: number) => void
}

const normalizeAssetType = (assetType?: string): string => {
  if (!assetType) return ''
  const normalized = assetType.trim().toUpperCase()
  if (!normalized) return ''
  if (normalized === 'COMMON') return 'CDN'
  if (normalized.includes('WAF')) return 'WAF'
  if (normalized.includes('CLOUD')) return 'CLOUD'
  if (normalized.includes('CDN')) return 'CDN'
  return normalized
}

const matchesAssetFilter = (result: SubdomainResult, filter: string): boolean => {
  const normalizedType = normalizeAssetType(result.assetType)
  switch (filter) {
    case 'cdn':
      return normalizedType === 'CDN'
    case 'cloud':
      return normalizedType === 'CLOUD'
    case 'waf':
      return normalizedType === 'WAF'
    case 'none':
    case 'no-cdn':
      return normalizedType === ''
    case 'all':
    default:
      return true
  }
}

const getFilenameFromContentDisposition = (contentDisposition: string | null) => {
  if (!contentDisposition) return null

  const encodedMatch = contentDisposition.match(/filename\*=UTF-8''([^;]+)/i)
  if (encodedMatch?.[1]) {
    try {
      return decodeURIComponent(encodedMatch[1].trim())
    } catch {
      return encodedMatch[1].trim()
    }
  }

  const filenameMatch = contentDisposition.match(/filename="?([^";]+)"?/i)
  return filenameMatch?.[1]?.trim() || null
}

const normalizeResultSources = (result: any): string[] => {
  const sourceCandidates = Array.isArray(result?.sources)
    ? result.sources
    : typeof result?.source === 'string'
      ? result.source.split(/[,;]+/)
      : []

  const seen = new Set<string>()
  const sources: string[] = []
  sourceCandidates.forEach((source: unknown) => {
    const normalized = String(source || '').trim()
    if (!normalized || seen.has(normalized)) return
    seen.add(normalized)
    sources.push(normalized)
  })

  if (sources.length === 0) {
    sources.push('-')
  }
  return sources
}

const normalizeResult = (result: any): SubdomainResult => {
  const sources = normalizeResultSources(result)
  return {
    domain: String(result?.domain || ''),
    subdomain: String(result?.subdomain || ''),
    ips: Array.isArray(result?.ips) ? result.ips.map((ip: unknown) => String(ip)).filter(Boolean) : [],
    assetType: typeof result?.assetType === 'string' ? result.assetType : undefined,
    assetName: typeof result?.assetName === 'string' ? result.assetName : undefined,
    source: sources[0] || '-',
    sources,
    isWildcard: Boolean(result?.isWildcard),
  }
}

const noop = () => undefined

const mergeResults = (current: SubdomainResult[], incoming: SubdomainResult[]) => {
  if (incoming.length === 0) return current

  const byKey = new Map(current.map((result) => [`${result.domain}\n${result.subdomain}`, result]))
  incoming.forEach((result) => {
    const key = `${result.domain}\n${result.subdomain}`
    const existing = byKey.get(key)
    if (!existing) {
      byKey.set(key, result)
      return
    }
    const mergedSources = normalizeResultSources({
      sources: [...(existing.sources || [existing.source]), ...(result.sources || [result.source])],
    })
    byKey.set(key, {
      ...existing,
      ...result,
      source: mergedSources[0] || result.source || existing.source,
      sources: mergedSources,
    })
  })
  return Array.from(byKey.values())
}

export function SubdomainResultsPanel({ taskId, totalHint = 0, onTotalChange }: SubdomainResultsPanelProps) {
  const { t } = useI18n()
  const [results, setResults] = useState<SubdomainResult[]>([])
  const [sourceStats, setSourceStats] = useState<Record<string, number>>({})
  const [displayResults, setDisplayResults] = useState<SubdomainResult[]>([])
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(50)
  const [totalResults, setTotalResults] = useState(totalHint)
  const [isLoadingResults, setIsLoadingResults] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [filterSource, setFilterSource] = useState('all')
  const [filterCdn, setFilterCdn] = useState('all')
  const [filterWildcard, setFilterWildcard] = useState('all')
  const [sortBy, setSortBy] = useState<'subdomain' | 'source'>('subdomain')
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc')
  const [selectedResults, setSelectedResults] = useState<Set<string>>(new Set())

  const formatText = useCallback((template: string, vars: Record<string, string | number>) => {
    return Object.entries(vars).reduce(
      (acc, [key, value]) => acc.replace(`{${key}}`, String(value)),
      template,
    )
  }, [])

  const stats = useMemo(() => {
    const categorizedTypes = results.map((result) => normalizeAssetType(result.assetType))
    const total = totalResults || results.length

    return {
      total,
      attempted: 0,
      successRate: 0,
      sourceStats,
      cdnCount: categorizedTypes.filter((type) => type === 'CDN').length,
      cloudCount: categorizedTypes.filter((type) => type === 'CLOUD').length,
      wafCount: categorizedTypes.filter((type) => type === 'WAF').length,
    }
  }, [results, sourceStats, totalResults])

  const allSources = useMemo(() => {
    return Object.keys(sourceStats).sort()
  }, [sourceStats])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setResults([])
      setSourceStats({})
      setDisplayResults([])
      setCurrentPage(1)
      setSelectedResults(new Set())
      setTotalResults(0)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [taskId])

  const fetchResults = useCallback(async (signal: AbortSignal) => {
    if (!taskId) {
      setResults([])
      setSourceStats({})
      setDisplayResults([])
      setTotalResults(0)
      onTotalChange?.(0)
      return
    }

    setIsLoadingResults(true)
    setSelectedResults(new Set())
    try {
      const params = new URLSearchParams({
        limit: String(pageSize),
        offset: String((currentPage - 1) * pageSize),
      })
      const trimmedSearch = searchQuery.trim()
      if (trimmedSearch) {
        params.set('search', trimmedSearch)
      }
      if (filterSource !== 'all') {
        params.set('source', filterSource)
      }
      if (filterWildcard !== 'all') {
        params.set('wildcard', filterWildcard)
      }

      const response = await fetchWithAuth(`/api/v1/subdomain/task/${taskId}/results?${params.toString()}`, { signal })
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      if (signal.aborted) return
      if (!data.success || !data.data) {
        throw new Error(data.error || t.subdomain.toasts.loadTaskResultsFailedMessage)
      }

      const fetchedResults: SubdomainResult[] = Array.isArray(data.data.results)
        ? data.data.results.map(normalizeResult)
        : []
      const nextTotal = Number(data.data.total || fetchedResults.length || 0)
      const nextDisplayResults = fetchedResults
        .filter((result) => matchesAssetFilter(result, filterCdn))
        .sort((a, b) => {
          const comparison = sortBy === 'source'
            ? (a.sources || [a.source]).join(', ').localeCompare((b.sources || [b.source]).join(', '))
            : a.subdomain.localeCompare(b.subdomain)
          return sortOrder === 'asc' ? comparison : -comparison
        })

      setResults((prev) => mergeResults(prev, fetchedResults))
      setSourceStats(data.data.sourceStats || {})
      setDisplayResults(nextDisplayResults)
      setTotalResults(nextTotal)
      onTotalChange?.(nextTotal)
    } catch (error) {
      if (signal.aborted) return
      console.error('Failed to fetch subdomain results:', error)
      setDisplayResults([])
      toast.error({
        title: t.subdomain.toasts.loadTaskResultsFailedTitle,
        message: error instanceof Error ? error.message : t.subdomain.toasts.loadTaskResultsFailedMessage,
      })
    } finally {
      if (!signal.aborted) setIsLoadingResults(false)
    }
  }, [
    currentPage,
    filterCdn,
    filterSource,
    filterWildcard,
    onTotalChange,
    pageSize,
    searchQuery,
    sortBy,
    sortOrder,
    t.subdomain.toasts.loadTaskResultsFailedMessage,
    t.subdomain.toasts.loadTaskResultsFailedTitle,
    taskId,
  ])

  useEffect(() => {
    const controller = new AbortController()
    const timer = window.setTimeout(() => {
      void fetchResults(controller.signal)
    }, 0)

    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [fetchResults])

  const copySingleIpResults = async () => {
    if (!taskId) {
      toast.error({ title: t.subdomain.toasts.taskNotFound })
      return
    }

    try {
      const response = await fetchWithAuth(`/api/v1/subdomain/task/${taskId}/copy?type=single-ip`)
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        if (errorData.code === 'NO_SINGLE_IP_RESULTS') {
          toast.warning({
            title: t.subdomain.toasts.noSingleIpResultsTitle,
            message: t.subdomain.toasts.noSingleIpResultsMessage,
          })
          return
        }
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const data = await response.json()
      const text = data?.data?.text || ''
      const copiedCount = Number(data?.data?.count || 0)
      const copied = await copyToClipboard(text)
      if (!copied) {
        throw new Error(t.vulnScan.messages.copyFailed)
      }
      toast.success({
        title: t.common.copied,
        message: formatText(t.subdomain.toasts.copiedSingleIpCount, { count: copiedCount }),
      })
    } catch (error) {
      console.error('Failed to copy single-IP subdomain results:', error)
      toast.error({
        title: t.subdomain.toasts.loadTaskResultsFailedTitle,
        message: error instanceof Error ? error.message : t.subdomain.toasts.loadTaskResultsFailedMessage,
      })
    }
  }

  const copySubdomain = async (subdomain: string) => {
    const copied = await copyToClipboard(subdomain)
    if (copied) {
      toast.success({
        title: t.common.copied,
        message: formatText(t.subdomain.toasts.copiedValue, { value: subdomain }),
      })
    } else {
      toast.error(t.vulnScan.messages.copyFailed)
    }
  }

  const copyIp = async (ip: string) => {
    const copied = await copyToClipboard(ip)
    if (copied) {
      toast.success({
        title: t.common.copied,
        message: formatText(t.subdomain.toasts.copiedIp, { value: ip }),
      })
    } else {
      toast.error(t.vulnScan.messages.copyFailed)
    }
  }

  const exportData = async () => {
    if (!taskId || (totalResults === 0 && results.length === 0)) {
      toast.error({ title: t.subdomain.toasts.startTaskFirst })
      return
    }

    try {
      const response = await fetchWithAuth(`/api/v1/subdomain/task/${taskId}/export`)
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}))
        throw new Error(errorData.error || `HTTP error! status: ${response.status}`)
      }

      const blob = await response.blob()
      const filename = getFilenameFromContentDisposition(response.headers.get('Content-Disposition')) || `subdomain_${Date.now()}.csv`
      const result = await saveBlobAsFile(blob, filename)
      if (result !== 'saved') {
        return
      }

      const exportedCount = Number(response.headers.get('X-Total-Count') || totalResults || results.length)
      toast.success({
        title: t.subdomain.toasts.exportSuccessTitle,
        message: formatText(t.subdomain.toasts.exportSuccessMessage, {
          count: exportedCount,
          format: 'CSV',
        }),
      })
    } catch (error) {
      console.error('Failed to export subdomain results:', error)
      toast.error({
        title: t.subdomain.toasts.loadTaskResultsFailedTitle,
        message: error instanceof Error ? error.message : t.subdomain.toasts.loadTaskResultsFailedMessage,
      })
    }
  }

  const toggleSelectResult = (resultId: string) => {
    setSelectedResults((prev) => {
      const next = new Set(prev)
      if (next.has(resultId)) {
        next.delete(resultId)
      } else {
        next.add(resultId)
      }
      return next
    })
  }

  const selectAll = () => {
    if (selectedResults.size === displayResults.length) {
      setSelectedResults(new Set())
      return
    }
    setSelectedResults(new Set(displayResults.map((result, index) => `${result.domain}-${result.subdomain}-${index}`)))
  }

  return (
    <ResultsTableView
      results={results}
      viewingHistory={null}
      stats={stats}
      allSources={allSources}
      currentPage={currentPage}
      pageSize={pageSize}
      totalResults={totalResults}
      displayResults={displayResults}
      isLoadingResults={isLoadingResults}
      searchQuery={searchQuery}
      filterSource={filterSource}
      filterCdn={filterCdn}
      filterWildcard={filterWildcard}
      sortBy={sortBy}
      sortOrder={sortOrder}
      selectedResults={selectedResults}
      onPageChange={(page) => setCurrentPage(page)}
      onPageSizeChange={(size) => {
        setPageSize(size)
        setCurrentPage(1)
      }}
      onSearchChange={(query) => {
        setSearchQuery(query)
        setCurrentPage(1)
      }}
      onFilterSourceChange={(source) => {
        setFilterSource(source)
        setCurrentPage(1)
      }}
      onFilterCdnChange={(assetFilter) => {
        setFilterCdn(assetFilter)
        setCurrentPage(1)
      }}
      onSortChange={(by, order) => {
        setSortBy(by)
        setSortOrder(order)
        setCurrentPage(1)
      }}
      onFilterWildcardChange={(wildcard) => {
        setFilterWildcard(wildcard)
        setCurrentPage(1)
      }}
      onClearHistory={noop}
      onClearResults={noop}
      onExportData={exportData}
      onCopyIpsWithOne={copySingleIpResults}
      onCopySubdomain={copySubdomain}
      onCopyIp={copyIp}
      onToggleSelectResult={toggleSelectResult}
      onSelectAll={selectAll}
      showClearButton={false}
      showResultsHeader={false}
      tableContainerClassName="mt-4"
    />
  )
}
