'use client'
import { DetailResultToolbar } from './DetailResultToolbar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from '@/components/ui/table'
import { Checkbox } from '@/components/ui/checkbox'
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from '@/components/ui/select'
import {
    Tooltip,
    TooltipContent,
    TooltipProvider,
    TooltipTrigger,
} from '@/components/ui/tooltip'
import { PaginationPro } from '@/components/ui/pagination-pro'
import {
    ArrowLeft,
    ArrowUpDown,
    Copy,
    Download,
    Search,
    Trash2,
    Globe2
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { AssetTagBadge } from '@/app/(main)/scan/vuln-scan/components/AssetTagBadge'
import { useI18n } from '@/lib/i18n'

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

const getResultSources = (result: SubdomainResult): string[] => {
    const sourceCandidates = result.sources?.length ? result.sources : [result.source]
    const seen = new Set<string>()
    const sources: string[] = []
    sourceCandidates.forEach((source) => {
        const normalized = String(source || '').trim()
        if (!normalized || seen.has(normalized)) return
        seen.add(normalized)
        sources.push(normalized)
    })
    return sources.length > 0 ? sources : ['-']
}

const getSourceBadgeClassName = (source: string) => {
    if (source.toLowerCase() === 'ai') {
        return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    }
    return 'text-muted-foreground'
}

export interface ResultsTableViewProps {
    results: SubdomainResult[]
    viewingHistory: any | null
    stats: {
        total: number
        attempted: number
        successRate: number
        sourceStats: Record<string, number>
        cdnCount: number
        cloudCount: number
        wafCount: number
    }
    allSources: string[]
    currentPage: number
    pageSize: number
    totalResults: number
    displayResults: SubdomainResult[]
    isLoadingResults: boolean
    searchQuery: string
    filterSource: string
    filterCdn: string
    filterWildcard: string
    sortBy: 'subdomain' | 'source'
    sortOrder: 'asc' | 'desc'
    selectedResults: Set<string>
    onPageChange: (page: number) => void
    onPageSizeChange: (size: number) => void
    onSearchChange: (query: string) => void
    onFilterSourceChange: (source: string) => void
    onFilterCdnChange: (cdn: string) => void
    onFilterWildcardChange: (wildcard: string) => void
    onSortChange: (by: 'subdomain' | 'source', order: 'asc' | 'desc') => void
    onClearHistory: () => void
    onClearResults: () => void
    onExportData: () => void
    onCopyIpsWithOne: () => void
    onCopySubdomain: (subdomain: string) => void
    onCopyIp: (ip: string) => void
    onToggleSelectResult: (resultId: string) => void
    onSelectAll: () => void
    showClearButton?: boolean
    showResultsHeader?: boolean
    tableContainerClassName?: string
}

export function ResultsTableView({
    results,
    viewingHistory,
    stats,
    allSources,
    currentPage,
    pageSize,
    totalResults,
    displayResults,
    isLoadingResults,
    searchQuery,
    filterSource,
    filterCdn,
    filterWildcard,
    sortBy,
    sortOrder,
    selectedResults,
    onPageChange,
    onPageSizeChange,
    onSearchChange,
    onFilterSourceChange,
    onFilterCdnChange,
    onFilterWildcardChange,
    onSortChange,
    onClearHistory,
    onClearResults,
    onExportData,
    onCopyIpsWithOne,
    onCopySubdomain,
    onCopyIp,
    onToggleSelectResult,
    onSelectAll,
    showClearButton = true,
    showResultsHeader = true,
    tableContainerClassName,
}: ResultsTableViewProps) {
    const { t } = useI18n()
    const normalizedFilterCdn = filterCdn === 'no-cdn' ? 'none' : filterCdn
    const formatText = (template: string, vars: Record<string, string | number>) =>
        Object.entries(vars).reduce(
            (acc, [key, value]) => acc.replace(`{${key}}`, String(value)),
            template
        )

    return (
        <>
            <div className="flex flex-col gap-4">
                {showResultsHeader && (
                    <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                            <h3 className="text-lg font-semibold">
                                {viewingHistory ? t.subdomain.resultsView.historyResults : t.subdomain.resultsView.scanResults}
                            </h3>
                            {selectedResults.size > 0 && (
                                <Badge variant="default">
                                    {formatText(t.subdomain.resultsView.selected, { count: selectedResults.size })}
                                </Badge>
                            )}
                        </div>
                        <div className="flex items-center gap-2">
                            {viewingHistory && (
                                <Button
                                    variant="outline"
                                    size="sm"
                                    onClick={onClearHistory}
                                >
                                    <ArrowLeft className="w-4 h-4 mr-2" />
                                    {t.subdomain.resultsView.backToCurrent}
                                </Button>
                            )}
                            {showClearButton && results.length > 0 && !viewingHistory && (
                                <TooltipProvider>
                                    <Tooltip>
                                        <TooltipTrigger asChild>
                                            <Button
                                                variant="outline"
                                                size="sm"
                                                onClick={onClearResults}
                                            >
                                                <Trash2 className="w-4 h-4 mr-2" />
                                                {t.subdomain.resultsView.clear}
                                            </Button>
                                        </TooltipTrigger>
                                        <TooltipContent>{t.subdomain.resultsView.clearTooltip}</TooltipContent>
                                    </Tooltip>
                                </TooltipProvider>
                            )}
                        </div>
                    </div>
                )}
                {/* 过滤搜索功能区 */}
                <DetailResultToolbar className="flex flex-wrap gap-2 items-center justify-between"
 activeCount={Number(Boolean(searchQuery.trim())) + Number(filterSource !== 'all') + Number(normalizedFilterCdn !== 'all') + Number(filterWildcard !== 'all')}
 onReset={() => { onSearchChange(''); onFilterSourceChange('all'); onFilterCdnChange('all'); onFilterWildcardChange('all') }}
 actions={<div className="flex gap-2 items-center">
                        <Button
                            variant="outline"
                            size="sm"
                            disabled={results.length === 0}
                            onClick={onCopyIpsWithOne}
                        >
                            <Copy className="w-4 h-4 mr-2" />
                            {t.subdomain.resultsView.copySingleIp}
                        </Button>

                        <Button
                            variant="outline"
                            size="sm"
                            disabled={results.length === 0}
                            onClick={onExportData}
                        >
                            <Download className="w-4 h-4 mr-2" />
                            {t.subdomain.resultsView.export}
                        </Button>
                    </div>}>
<div className="flex flex-wrap gap-2 items-center flex-1">
                        <div className="relative flex-1 min-w-[200px] max-w-md">
                            <Search className="absolute left-3 top-1/2 transform -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                            <Input
                                placeholder={t.subdomain.resultsView.searchPlaceholder}
                                value={searchQuery}
                                onChange={(e) => onSearchChange(e.target.value)}
                                className="pl-9"
                            />
                        </div>
                        <Select value={filterSource} onValueChange={onFilterSourceChange}>
                            <SelectTrigger className="w-[140px]">
                                <SelectValue placeholder={t.subdomain.resultsView.sourcePlaceholder} />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">{t.subdomain.resultsView.allSources}</SelectItem>
                                {allSources.map((source) => (
                                    <SelectItem key={source} value={source}>
                                        {source} ({stats.sourceStats[source] || 0})
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                        <Select value={normalizedFilterCdn} onValueChange={onFilterCdnChange}>
                            <SelectTrigger className="w-[140px]">
                                <SelectValue placeholder={t.subdomain.resultsView.assetTagPlaceholder} />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">{t.subdomain.resultsView.all}</SelectItem>
                                <SelectItem value="cdn">CDN</SelectItem>
                                <SelectItem value="cloud">CLOUD</SelectItem>
                                <SelectItem value="waf">WAF</SelectItem>
                                <SelectItem value="none">{t.subdomain.resultsView.noTag}</SelectItem>
                            </SelectContent>
                        </Select>
                        <Select value={filterWildcard} onValueChange={onFilterWildcardChange}>
                            <SelectTrigger className="w-[180px]" aria-label={t.subdomain.resultsView.columns.wildcard}>
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">{t.subdomain.resultsView.allWildcard}</SelectItem>
                                <SelectItem value="exclude">{t.subdomain.resultsView.excludeWildcard}</SelectItem>
                                <SelectItem value="only">{t.subdomain.resultsView.onlyWildcard}</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
</DetailResultToolbar>
            </div>
            <div className={cn('border rounded-lg overflow-hidden', tableContainerClassName)}>
                <Table>
                    <TableHeader>
                        <TableRow>
                            <TableHead className="w-[40px]">
                                <Checkbox
                                    checked={selectedResults.size > 0 && selectedResults.size === displayResults.length}
                                    onCheckedChange={onSelectAll}
                                />
                            </TableHead>
                            <TableHead className="w-[60px]">#</TableHead>
                            <TableHead className='w-[180px]'>{t.subdomain.resultsView.columns.rootDomain}</TableHead>
                            <TableHead className='w-[240px]'>{t.subdomain.resultsView.columns.subdomain}</TableHead>
                            <TableHead>{t.subdomain.resultsView.columns.ips}</TableHead>
                            <TableHead className="w-[150px]">{t.subdomain.resultsView.columns.assetTag}</TableHead>
                            <TableHead className="w-[100px]">{t.subdomain.resultsView.columns.wildcard}</TableHead>
                            <TableHead className="w-[180px]">
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-6 -ml-2"
                                    onClick={() => {
                                        if (sortBy === 'source') {
                                            onSortChange('source', sortOrder === 'asc' ? 'desc' : 'asc')
                                        } else {
                                            onSortChange('source', 'asc')
                                        }
                                    }}
                                >
                                    {t.subdomain.resultsView.columns.source}
                                    {sortBy === 'source' && (
                                        <ArrowUpDown className="w-3 h-3 ml-1" />
                                    )}
                                </Button>
                            </TableHead>
                        </TableRow>
                    </TableHeader>
                    <TableBody>
                        {displayResults.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={8} className="text-center py-12">
                                    <Globe2 className="w-12 h-12 mx-auto mb-4 opacity-50" />
                                    <p className="text-muted-foreground">
                                        {results.length === 0
                                            ? t.subdomain.resultsView.empty
                                            : t.subdomain.resultsView.noMatch}
                                    </p>
                                    {results.length > 0 && (
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            className="mt-4"
                                            onClick={() => {
                                                onSearchChange('')
                                                onFilterSourceChange('all')
                                                onFilterCdnChange('all')
                                                onFilterWildcardChange('all')
                                            }}
                                        >
                                            {t.subdomain.resultsView.clearFilters}
                                        </Button>
                                    )}
                                </TableCell>
                            </TableRow>
                        ) : (
                            displayResults.map((result, index) => {
                                const resultId = `${result.domain}-${result.subdomain}-${index}`
                                return (
                                    <TableRow
                                        key={resultId}
                                        className={cn(
                                            'hover:bg-muted/50 transition-colors',
                                            selectedResults.has(resultId) && 'bg-primary/5'
                                        )}
                                    >
                                        <TableCell>
                                            <Checkbox
                                                checked={selectedResults.has(resultId)}
                                                onCheckedChange={() => onToggleSelectResult(resultId)}
                                            />
                                        </TableCell>
                                        <TableCell>
                                            {(currentPage - 1) * pageSize + index + 1}
                                        </TableCell>
                                        <TableCell>
                                            <TooltipProvider>
                                                <Tooltip>
                                                    <TooltipTrigger asChild>
                                                        <span className="cursor-help">{result.domain}</span>
                                                    </TooltipTrigger>
                                                    <TooltipContent>
                                                        <p>{formatText(t.subdomain.resultsView.rootDomainLabel, { value: result.domain })}</p>
                                                    </TooltipContent>
                                                </Tooltip>
                                            </TooltipProvider>
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex items-center gap-2">
                                                <span className="text-foreground truncate max-w-[200px]" title={result.subdomain}>
                                                    {result.subdomain}
                                                </span>
                                                <TooltipProvider>
                                                    <Tooltip>
                                                        <TooltipTrigger asChild>
                                                            <Button
                                                                variant="ghost"
                                                                size="icon"
                                                                className="h-6 w-6"
                                                                onClick={() => onCopySubdomain(result.subdomain)}
                                                            >
                                                                <Copy className="w-3 h-3" />
                                                            </Button>
                                                        </TooltipTrigger>
                                                        <TooltipContent>{t.subdomain.resultsView.copySubdomain}</TooltipContent>
                                                    </Tooltip>
                                                </TooltipProvider>
                                            </div>
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex flex-wrap gap-1">
                                                {result.ips.map((ip, i) => (
                                                    <TooltipProvider key={i}>
                                                        <Tooltip>
                                                            <TooltipTrigger asChild>
                                                                <Badge
                                                                    variant="secondary"
                                                                    className="text-xs cursor-pointer hover:bg-secondary/80 transition-colors"
                                                                    onClick={() => onCopyIp(ip)}
                                                                >
                                                                    {ip}
                                                                </Badge>
                                                            </TooltipTrigger>
                                                            <TooltipContent>
                                                                <p>{t.subdomain.resultsView.copyIp}</p>
                                                            </TooltipContent>
                                                        </Tooltip>
                                                    </TooltipProvider>
                                                ))}
                                            </div>
                                        </TableCell>
                                        <TableCell>
                                            {normalizeAssetType(result.assetType) ? (
                                                <TooltipProvider>
                                                    <Tooltip>
                                                        <TooltipTrigger asChild>
                                                            {(() => {
                                                                const assetType = normalizeAssetType(result.assetType)
                                                                return (
                                                                    <AssetTagBadge
                                                                        label={result.assetName || assetType}
                                                                        assetType={assetType}
                                                                        className="cursor-help"
                                                                    />
                                                                )
                                                            })()}
                                                        </TooltipTrigger>
                                                        <TooltipContent>
                                                            <p>{formatText(t.subdomain.resultsView.assetType, { value: normalizeAssetType(result.assetType) })}</p>
                                                            {result.assetName && <p>{formatText(t.subdomain.resultsView.assetName, { value: result.assetName })}</p>}
                                                        </TooltipContent>
                                                    </Tooltip>
                                                </TooltipProvider>
                                            ) : (
                                                <span className="text-muted-foreground text-xs">-</span>
                                            )}
                                        </TableCell>
                                        <TableCell>
                                            {result.isWildcard ? (
                                                <Badge
                                                    variant="outline"
                                                    className="border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300"
                                                >
                                                    {t.common.yes}
                                                </Badge>
                                            ) : (
                                                <Badge variant="outline" className="text-muted-foreground">
                                                    {t.common.no}
                                                </Badge>
                                            )}
                                        </TableCell>
                                        <TableCell>
                                            {(() => {
                                                const sources = getResultSources(result)
                                                const visibleSources = sources.slice(0, 2)
                                                const hiddenCount = Math.max(sources.length - visibleSources.length, 0)
                                                return (
                                                    <TooltipProvider>
                                                        <Tooltip>
                                                            <TooltipTrigger asChild>
                                                                <div className="inline-flex max-w-[180px] flex-wrap items-center gap-1 cursor-help">
                                                                    {visibleSources.map((source) => (
                                                                        <Badge
                                                                            key={source}
                                                                            variant="outline"
                                                                            className={cn('max-w-[78px] truncate text-xs', getSourceBadgeClassName(source))}
                                                                        >
                                                                            {source}
                                                                        </Badge>
                                                                    ))}
                                                                    {hiddenCount > 0 && (
                                                                        <Badge variant="secondary" className="text-xs">
                                                                            +{hiddenCount}
                                                                        </Badge>
                                                                    )}
                                                                </div>
                                                            </TooltipTrigger>
                                                            <TooltipContent>
                                                                <p>{formatText(t.subdomain.resultsView.sourceLabel, { value: sources.join(', ') })}</p>
                                                            </TooltipContent>
                                                        </Tooltip>
                                                    </TooltipProvider>
                                                )
                                            })()}
                                        </TableCell>
                                    </TableRow>
                                )
                            })
                        )}
                    </TableBody>
                </Table>
            </div>

            {/* 分页 */}
            <div className="flex justify-end mt-6 pt-4 border-t">
                <PaginationPro
                    current={currentPage}
                    total={totalResults}
                    pageSize={pageSize}
                    onChange={(page) => onPageChange(page)}
                    onPageSizeChange={(size) => {
                        onPageSizeChange(size)
                        onPageChange(1)
                    }}
                    pageSizeOptions={[50, 100, 200, 500]}
                    showSizeChanger
                    showQuickJumper
                    showTotal
                />
            </div>
        </>
    )
}
