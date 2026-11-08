'use client'
import { DetailResultToolbar } from './DetailResultToolbar'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronsLeft, ChevronsRight, Copy, ExternalLink, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import mobileTable from '@/components/ui/mobile-record-table.module.css'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { copyToClipboard } from '@/lib/clipboard'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from 'react-hot-toast'

export interface JsScanAsset {
	id: string
	taskId: string
	version?: number
	type: string
	value: string
	source: string
	createdAt: string
}

interface JsScanAssetTableProps {
	assets?: JsScanAsset[]
	taskId?: string
	version?: number
	onTotalChange?: (total: number) => void
}

const EMPTY_PLACEHOLDER = '-'

const defaultAssetTypeOptions = ['email', 'phone', 'idCard', 'ipUrl', 'apiRoot', 'apiRoute']

interface AssetTypeOption {
	value: string
	label: string
	count: number
}

const normalizeAssetType = (raw?: string) => {
	switch ((raw || '').trim()) {
		case 'email':
		case 'phone':
		case 'idCard':
		case 'sensitive':
			return (raw || '').trim()
		case 'domain':
		case 'ip':
		case 'ipUrl':
			return 'ipUrl'
		default:
			return (raw || '').trim()
	}
}

export function JsScanAssetTable({ assets = [], taskId, version, onTotalChange }: JsScanAssetTableProps) {
	const { t } = useI18n()
	const [currentPage, setCurrentPage] = useState(1)
	const [pageSize, setPageSize] = useState(20)
	const [typeFilter, setTypeFilter] = useState<string>('all')
	const [keyword, setKeyword] = useState('')
	const [statsCollapsed, setStatsCollapsed] = useState(false)
	const [serverData, setServerData] = useState<JsScanAsset[]>([])
	const [totalCount, setTotalCount] = useState(0)
	const [aggregatedTotalCount, setAggregatedTotalCount] = useState(0)
	const [serverTypeCounts, setServerTypeCounts] = useState<Record<string, number>>({})
	const [isLoading, setIsLoading] = useState(false)
	const tableHostRef = useRef<HTMLDivElement | null>(null)
	const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)

	const useServerPagination = !!taskId
	const assetTypeMap: Record<string, string> = useMemo(() => ({
		email: t.jsScan.assets.types.email,
		phone: t.jsScan.assets.types.phone,
		idCard: t.jsScan.assets.types.idCard,
		apiRoute: t.jsScan.assets.types.apiRoute,
		apiRoot: t.jsScan.assets.types.apiRoot,
		ipUrl: t.jsScan.assets.types.ipUrl,
		sensitive: t.jsScan.assets.types.sensitive,
	}), [t.jsScan.assets.types])

	const fetchServerData = async () => {
		if (!taskId) return

		setIsLoading(true)
		try {
			const params = new URLSearchParams()
			params.set('page', String(currentPage))
			params.set('pageSize', String(pageSize))
			if (version !== undefined) {
				params.set('version', String(version))
			}
			if (typeFilter !== 'all') {
				params.set('type', typeFilter)
			}
			if (keyword.trim()) {
				params.set('keyword', keyword.trim())
			}

			const response = await fetchWithAuth(`/api/v1/jsscan/assets/${taskId}?${params.toString()}`)
			if (!response.ok) {
				return
			}

			const result = await response.json()
			const list = result?.data?.assets || []
			const total = result?.data?.total || 0
			const totalAll = result?.data?.totalAll || 0
			const typeCounts = result?.data?.typeCounts || {}
			setServerData(list)
			setTotalCount(total)
			setAggregatedTotalCount(totalAll)
			setServerTypeCounts(typeCounts)
			onTotalChange?.(total)
		} catch (error) {
			console.error('Failed to fetch JS assets:', error)
		} finally {
			setIsLoading(false)
		}
	}

	useEffect(() => {
		if (!useServerPagination) return
		const timer = setTimeout(() => {
			fetchServerData()
		}, 250)
		return () => clearTimeout(timer)
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [taskId, version, currentPage, pageSize, typeFilter, keyword])

	useEffect(() => {
		if (!useServerPagination) {
			onTotalChange?.(assets.length)
		}
	}, [onTotalChange, useServerPagination, assets.length])

	const displayData = useServerPagination ? serverData : assets

	const filteredAssets = useMemo(() => {
		let filtered = displayData

		if (!useServerPagination) {
			if (typeFilter !== 'all') {
				filtered = filtered.filter(asset => normalizeAssetType(asset.type) === typeFilter)
			}
			if (keyword.trim()) {
				const query = keyword.toLowerCase()
				filtered = filtered.filter(asset => {
					const value = asset.value?.toLowerCase() || ''
					const source = asset.source?.toLowerCase() || ''
					const type = normalizeAssetType(asset.type)?.toLowerCase() || ''
					return value.includes(query) || source.includes(query) || type.includes(query)
				})
			}
		}

		return filtered
	}, [displayData, keyword, typeFilter, useServerPagination])

	const paginatedAssets = useMemo(() => {
		if (useServerPagination) {
			return filteredAssets
		}
		return filteredAssets.slice((currentPage - 1) * pageSize, currentPage * pageSize)
	}, [filteredAssets, currentPage, pageSize, useServerPagination])

	const updateRightFixedShadow = useCallback(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) {
			setShowRightFixedShadow(false)
			return
		}

		const maxScrollLeft = el.scrollWidth - el.clientWidth
		setShowRightFixedShadow(maxScrollLeft > 1 && el.scrollLeft < maxScrollLeft - 1)
	}, [])

	useEffect(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) return

		updateRightFixedShadow()
		el.addEventListener('scroll', updateRightFixedShadow, { passive: true })
		window.addEventListener('resize', updateRightFixedShadow)

		return () => {
			el.removeEventListener('scroll', updateRightFixedShadow)
			window.removeEventListener('resize', updateRightFixedShadow)
		}
	}, [paginatedAssets.length, updateRightFixedShadow])

	const assetTypes = useMemo(() => {
		const set = new Set<string>(defaultAssetTypeOptions)
		displayData.forEach(asset => {
			const normalizedType = normalizeAssetType(asset.type)
			if (normalizedType) set.add(normalizedType)
		})
		return Array.from(set)
	}, [displayData])

	const keywordScopedAssets = useMemo(() => {
		if (useServerPagination) {
			return []
		}
		if (!keyword.trim()) {
			return assets
		}
		const query = keyword.toLowerCase()
		return assets.filter(asset => {
			const value = asset.value?.toLowerCase() || ''
			const source = asset.source?.toLowerCase() || ''
			const type = normalizeAssetType(asset.type)?.toLowerCase() || ''
			return value.includes(query) || source.includes(query) || type.includes(query)
		})
	}, [assets, keyword, useServerPagination])

	const localTypeCounts = useMemo(() => {
		const counts: Record<string, number> = {}
		keywordScopedAssets.forEach(asset => {
			const normalizedType = normalizeAssetType(asset.type)
			if (!normalizedType) return
			counts[normalizedType] = (counts[normalizedType] || 0) + 1
		})
		return counts
	}, [keywordScopedAssets])

	const typeCountMap = useMemo(
		() => (useServerPagination ? serverTypeCounts : localTypeCounts),
		[localTypeCounts, serverTypeCounts, useServerPagination],
	)

	const allTypeTotal = useMemo(() => {
		if (useServerPagination) {
			return aggregatedTotalCount
		}
		return keywordScopedAssets.length
	}, [aggregatedTotalCount, keywordScopedAssets.length, useServerPagination])

	const typeOptions = useMemo<AssetTypeOption[]>(() => {
		const values = new Set<string>(defaultAssetTypeOptions)
		Object.keys(typeCountMap).forEach(type => values.add(type))
		assetTypes.forEach(type => values.add(type))
		return [
			{ value: 'all', label: t.jsScan.assets.allTypes, count: allTypeTotal },
			...Array.from(values).map(type => ({
				value: type,
				label: assetTypeMap[type] || type,
				count: typeCountMap[type] || 0,
			})),
		]
	}, [allTypeTotal, assetTypeMap, assetTypes, t.jsScan.assets.allTypes, typeCountMap])

	const selectedTypeOption = useMemo(
		() => typeOptions.find(option => option.value === typeFilter) || typeOptions[0] || null,
		[typeFilter, typeOptions],
	)

	const totalItems = useServerPagination ? totalCount : filteredAssets.length

	const maxValueWidthClassName = 'max-w-[520px]'
	const maxSourceWidthClassName = 'max-w-[360px]'

	const handleCopy = (text: string) => {
		copyToClipboard(text)
		toast.success(t.common.copiedToClipboard)
	}

	const formatFullTime = (dateString?: string): string => {
		if (!dateString) return '-'
		return new Date(dateString).toLocaleString()
	}

	return (
		<div className={`grid gap-4 ${statsCollapsed ? 'lg:grid-cols-[56px_minmax(0,1fr)]' : 'lg:grid-cols-[260px_minmax(0,1fr)]'}`}>
			<div className="hidden space-y-4 md:block">
				<div className={cn('rounded-lg border bg-card/60', statsCollapsed ? 'p-3' : 'p-4')}>
					<div className={cn('flex gap-2', statsCollapsed ? 'flex-col items-center' : 'items-center justify-between')}>
						{!statsCollapsed ? (
							<div className="min-w-0">
								<div className="text-sm font-medium">{t.jsScan.assets.summaryTitle}</div>
								<div className="mt-2 flex items-end justify-between gap-3">
									<span className="text-xs text-muted-foreground">{t.jsScan.assets.allTypes}</span>
									<span className="text-3xl font-semibold">{allTypeTotal}</span>
								</div>
							</div>
						) : (
							<div className="flex flex-1 flex-col items-center justify-center gap-2">
								<span className="text-xs font-medium text-muted-foreground [writing-mode:vertical-rl]">{t.jsScan.assets.summaryTitle}</span>
							</div>
						)}
						<Button
							type="button"
							variant="ghost"
							size="icon"
							className={cn(
								'h-8 w-8 shrink-0 text-muted-foreground hover:text-foreground',
								statsCollapsed && 'mt-1'
							)}
							onClick={() => setStatsCollapsed((prev) => !prev)}
						>
							{statsCollapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
						</Button>
					</div>

					{!statsCollapsed && (
						<div className="mt-4 space-y-2">
							{typeOptions.map((option) => {
								const active = option.value === (selectedTypeOption?.value || 'all')
								return (
									<button
										key={option.value}
										type="button"
										onClick={() => {
											setTypeFilter(option.value)
											setCurrentPage(1)
										}}
										className={cn(
											'flex w-full items-center justify-between rounded-md border px-3 py-2 text-left transition-colors',
											active
												? 'border-primary/35 bg-primary/10 text-foreground'
												: 'border-border bg-muted/40 text-muted-foreground hover:bg-muted/65 hover:text-foreground',
										)}
									>
										<span className="truncate pr-3 text-sm">{option.label}</span>
										<span className={cn('text-base font-semibold', active ? 'text-primary' : 'text-foreground')}>
											{option.count}
										</span>
									</button>
								)
							})}
						</div>
					)}
				</div>
			</div>

			<div className="space-y-4 min-w-0">
				<DetailResultToolbar activeCount={Number(Boolean(keyword.trim())) + Number(typeFilter !== 'all')} onReset={() => { setKeyword(''); setTypeFilter('all'); setCurrentPage(1) }}>
<div className="md:hidden">
					<Select value={typeFilter} onValueChange={(value) => {
						setTypeFilter(value)
						setCurrentPage(1)
					}}>
						<SelectTrigger className="min-h-11 w-full" aria-label={t.jsScan.assets.type}><SelectValue /></SelectTrigger>
						<SelectContent>
							{typeOptions.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
						</SelectContent>
					</Select>
				</div>
				<div className="relative min-w-0 flex-1">
					<Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
					<Input
						value={keyword}
						onChange={(e) => {
							setKeyword(e.target.value)
							setCurrentPage(1)
						}}
						placeholder={t.jsScan.assets.searchPlaceholder}
						className="pl-9"
					/>
					</div>

</DetailResultToolbar>
					<div ref={tableHostRef} className="rounded-lg border">
					<Table className={mobileTable.table}>
						<TableHeader>
							<TableRow>
								<TableHead className="w-[120px]">{t.jsScan.assets.type}</TableHead>
								<TableHead>{t.jsScan.assets.value}</TableHead>
								<TableHead className="w-[360px]">{t.jsScan.assets.source}</TableHead>
								<TableHead className="w-[170px]">{t.jsScan.assets.discoveredAt}</TableHead>
								<TableHead className={cn('sticky right-0 z-20 w-[88px] min-w-[88px] border-l border-border bg-card text-center', showRightFixedShadow && 'table-fixed-column-shadow-left')}>{t.common.action}</TableHead>
							</TableRow>
						</TableHeader>
						<TableBody>
							{isLoading ? (
								<TableRow>
									<TableCell colSpan={5} className="py-12 text-center text-muted-foreground">
										{t.common.loading}
									</TableCell>
								</TableRow>
							) : paginatedAssets.length === 0 ? (
								<TableRow>
									<TableCell colSpan={5} className="py-12 text-center text-muted-foreground">
										{t.jsScan.assets.empty}
									</TableCell>
								</TableRow>
							) : (
								paginatedAssets.map((asset) => (
									<TableRow key={asset.id} className="group">
										<TableCell data-label={t.jsScan.assets.type}>
											<Badge variant="outline">{assetTypeMap[normalizeAssetType(asset.type)] || normalizeAssetType(asset.type) || EMPTY_PLACEHOLDER}</Badge>
										</TableCell>
										<TableCell className="font-mono text-sm" data-label={t.jsScan.assets.value}>
											{normalizeAssetType(asset.type) === 'ipUrl' && asset.value ? (
												<a
													href={asset.value}
													target="_blank"
													rel="noopener noreferrer"
													className={cn('flex items-center gap-1 text-primary hover:underline', maxValueWidthClassName)}
													onClick={(e) => e.stopPropagation()}
													title={asset.value}
												>
													<span className="block break-all md:truncate">{asset.value}</span>
													<ExternalLink className="h-3 w-3" />
												</a>
											) : (
												<span className={cn('block break-all md:truncate', maxValueWidthClassName)} title={asset.value || EMPTY_PLACEHOLDER}>
													{asset.value || EMPTY_PLACEHOLDER}
												</span>
											)}
										</TableCell>
										<TableCell
											data-label={t.jsScan.assets.source}
											className={cn('font-mono text-xs text-muted-foreground', maxSourceWidthClassName)}
											title={asset.source || EMPTY_PLACEHOLDER}
										>
											<span className="block break-all md:truncate">{asset.source || EMPTY_PLACEHOLDER}</span>
										</TableCell>
											<TableCell className="text-sm text-muted-foreground" data-label={t.jsScan.assets.discoveredAt}>
												{formatFullTime(asset.createdAt)}
											</TableCell>
											<TableCell data-label={t.common.action} className={cn('sticky right-0 z-10 w-[88px] min-w-[88px] border-l border-border bg-card text-center group-hover:bg-muted', showRightFixedShadow && 'table-fixed-column-shadow-left')}>
											<Button
												variant="ghost"
												size="icon"
												className="h-7 w-7"
												onClick={() => handleCopy(asset.value)}
												disabled={!asset.value}
												title={t.common.copy}
											>
												<Copy className="h-4 w-4" />
											</Button>
										</TableCell>
									</TableRow>
								))
							)}
						</TableBody>
					</Table>
				</div>

				{totalItems > 0 && (
					<PaginationPro
						current={currentPage}
						total={totalItems}
						pageSize={pageSize}
						onChange={(page, size) => {
							setCurrentPage(page)
							if (size !== pageSize) {
								setPageSize(size)
							}
						}}
						onPageSizeChange={(size) => {
							setPageSize(size)
							setCurrentPage(1)
						}}
						pageSizeOptions={[10, 20, 50, 100]}
						showSizeChanger
						showQuickJumper
						showTotal
					/>
				)}
			</div>
		</div>
	)
}
