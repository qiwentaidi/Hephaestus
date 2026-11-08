'use client'
import { DetailResultToolbar } from './DetailResultToolbar'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { ArrowDownWideNarrow, ArrowUpNarrowWide, ChevronsUpDown, ChevronsDownUp, Search, Trees } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { SiteTree, SiteTreeRef } from '@/components/ui/site-tree'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

interface SiteTreeItem {
	id?: string
	target?: string
	targetUrl?: string
	siteTree?: any[]
	treeData?: any[]
	createdAt?: string
}

interface SiteTreeListItem {
	id: string
	targetUrl: string
	createdAt?: string
	apiRouteCount?: number
}

interface SiteTreeDetailItem {
	id: string
	targetUrl: string
	treeData: any[]
	createdAt?: string
}

interface JsScanSiteTreeProps {
	data?: SiteTreeItem[]
	taskId?: string
	version?: number
	onTotalChange?: (total: number) => void
}

const SITE_TREE_PAGE_SIZE = 50
const LOAD_MORE_THRESHOLD = 120
const LIST_ESTIMATED_ROW_HEIGHT = 66

type ListLoadingMode = 'idle' | 'replace' | 'append'

const normalizeTreeChildren = (input: unknown): any[] => {
	if (Array.isArray(input)) return input
	if (input && typeof input === 'object') return [input]
	return []
}

const dedupeSiteTreeItems = (items: SiteTreeListItem[]) => {
	const seen = new Set<string>()
	const result: SiteTreeListItem[] = []
	for (const item of items) {
		if (!item?.id || seen.has(item.id)) continue
		seen.add(item.id)
		result.push(item)
	}
	return result
}

const mergeSiteTreeItems = (prev: SiteTreeListItem[], next: SiteTreeListItem[]) => {
	if (next.length === 0) return prev
	const seen = new Set(prev.map(item => item.id))
	const merged = [...prev]
	for (const item of next) {
		if (!item?.id || seen.has(item.id)) continue
		seen.add(item.id)
		merged.push(item)
	}
	return merged
}

export function JsScanSiteTree({ data = [], taskId, version, onTotalChange }: JsScanSiteTreeProps) {
	const { t } = useI18n()
	const siteTreeRef = useRef<SiteTreeRef>(null)
	const listScrollRef = useRef<HTMLDivElement | null>(null)
	const listRequestSeqRef = useRef(0)
	const listLoadingModeRef = useRef<ListLoadingMode>('idle')

	const [keyword, setKeyword] = useState('')
	const [routeSortOrder, setRouteSortOrder] = useState<'desc' | 'asc'>('desc')
	const [currentPage, setCurrentPage] = useState(1)
	const [listLoadingMode, setListLoadingMode] = useState<ListLoadingMode>('idle')
	const [isLoadingDetail, setIsLoadingDetail] = useState(false)
	const [totalCount, setTotalCount] = useState(0)
	const [siteTreeList, setSiteTreeList] = useState<SiteTreeListItem[]>([])
	const [selectedTreeId, setSelectedTreeId] = useState<string | null>(null)
	const [selectedTreeDetail, setSelectedTreeDetail] = useState<SiteTreeDetailItem | null>(null)

	const useServerPagination = !!taskId
	const isLoadingList = listLoadingMode === 'replace'
	const isLoadingMore = listLoadingMode === 'append'
	const hasMore = siteTreeList.length < totalCount

	// TanStack Virtual returns mutable helpers that React Compiler intentionally skips.
	// eslint-disable-next-line react-hooks/incompatible-library
	const rowVirtualizer = useVirtualizer({
		count: siteTreeList.length,
		getScrollElement: () => listScrollRef.current,
		estimateSize: () => LIST_ESTIMATED_ROW_HEIGHT,
		overscan: 6,
	})

	const handleExpandAll = () => {
		siteTreeRef.current?.expandAll()
	}

	const handleCollapseAll = () => {
		siteTreeRef.current?.collapseAll()
	}

	const fetchSiteTreeList = useCallback(async (page: number, mode: 'replace' | 'append') => {
		if (!taskId) return
		if (mode === 'append' && listLoadingModeRef.current !== 'idle') return

		setListLoadingMode(mode)
		listLoadingModeRef.current = mode
		const requestSeq = ++listRequestSeqRef.current

		try {
			const params = new URLSearchParams()
			params.set('page', String(page))
			params.set('pageSize', String(SITE_TREE_PAGE_SIZE))
			params.set('sortBy', 'apiRoutes')
			params.set('sortOrder', routeSortOrder)
			if (version !== undefined) {
				params.set('version', String(version))
			}
			if (keyword.trim()) {
				params.set('keyword', keyword.trim())
			}

			const response = await fetchWithAuth(`/api/v1/jsscan/sitetree/${taskId}?${params.toString()}`)
			if (!response.ok) {
				return
			}

			const result = await response.json()
			const incoming = dedupeSiteTreeItems(result?.data?.siteTrees || [])
			const total = result?.data?.total || 0

			if (requestSeq !== listRequestSeqRef.current) {
				return
			}

			setTotalCount(total)
			onTotalChange?.(total)
			if (mode === 'replace') {
				setSiteTreeList(incoming)
				setCurrentPage(1)
			} else {
				setSiteTreeList(prev => mergeSiteTreeItems(prev, incoming))
				setCurrentPage(page)
			}
		} catch (error) {
			console.error('Failed to fetch JS site tree list:', error)
		} finally {
			if (requestSeq === listRequestSeqRef.current) {
				setListLoadingMode('idle')
				listLoadingModeRef.current = 'idle'
			}
		}
	}, [keyword, onTotalChange, routeSortOrder, taskId, version])

	const fetchSiteTreeDetail = useCallback(async (treeId: string) => {
		if (!taskId || !treeId) return

		setIsLoadingDetail(true)
		try {
			const params = new URLSearchParams()
			if (version !== undefined) {
				params.set('version', String(version))
			}
			const query = params.toString()
			const url = query
				? `/api/v1/jsscan/sitetree/${taskId}/${treeId}?${query}`
				: `/api/v1/jsscan/sitetree/${taskId}/${treeId}`

			const response = await fetchWithAuth(url)
			if (!response.ok) {
				setSelectedTreeDetail(null)
				return
			}

			const result = await response.json()
			const detail = result?.data
			if (!detail) {
				setSelectedTreeDetail(null)
				return
			}

			setSelectedTreeDetail({
				id: detail.id,
				targetUrl: detail.targetUrl,
				treeData: normalizeTreeChildren(detail.treeData),
				createdAt: detail.createdAt,
			})
		} catch (error) {
			console.error('Failed to fetch JS site tree detail:', error)
			setSelectedTreeDetail(null)
		} finally {
			setIsLoadingDetail(false)
		}
	}, [taskId, version])

	const loadNextPage = useCallback(() => {
		if (!useServerPagination) return
		if (!hasMore) return
		if (listLoadingModeRef.current !== 'idle') return
		void fetchSiteTreeList(currentPage + 1, 'append')
	}, [currentPage, fetchSiteTreeList, hasMore, useServerPagination])

	const handleListScroll = useCallback(() => {
		const el = listScrollRef.current
		if (!el) return
		if (el.scrollHeight - el.scrollTop - el.clientHeight <= LOAD_MORE_THRESHOLD) {
			loadNextPage()
		}
	}, [loadNextPage])

	useEffect(() => {
		if (!useServerPagination) {
			onTotalChange?.(data.length)
			return
		}

		setCurrentPage(1)
		setTotalCount(0)
		setSiteTreeList([])
		setSelectedTreeId(null)
		setSelectedTreeDetail(null)

		const timer = setTimeout(() => {
			void fetchSiteTreeList(1, 'replace')
		}, 250)
		return () => clearTimeout(timer)
	}, [data.length, fetchSiteTreeList, keyword, onTotalChange, routeSortOrder, taskId, useServerPagination, version])

	useEffect(() => {
		if (!useServerPagination) return
		if (siteTreeList.length === 0) {
			setSelectedTreeId(null)
			setSelectedTreeDetail(null)
			return
		}
		if (!selectedTreeId || !siteTreeList.some(item => item.id === selectedTreeId)) {
			setSelectedTreeId(siteTreeList[0].id)
		}
	}, [selectedTreeId, siteTreeList, useServerPagination])

	useEffect(() => {
		if (!useServerPagination || !selectedTreeId) return
		void fetchSiteTreeDetail(selectedTreeId)
	}, [fetchSiteTreeDetail, selectedTreeId, useServerPagination])

	useEffect(() => {
		if (!useServerPagination || !hasMore) return
		if (listLoadingMode !== 'idle') return
		const el = listScrollRef.current
		if (!el) return
		if (el.scrollHeight <= el.clientHeight + LOAD_MORE_THRESHOLD) {
			void fetchSiteTreeList(currentPage + 1, 'append')
		}
	}, [currentPage, fetchSiteTreeList, hasMore, listLoadingMode, siteTreeList.length, useServerPagination])

	const selectedTreeNodes = useMemo(() => {
		if (!selectedTreeDetail) return []
		return [
			{
				id: `target-${selectedTreeDetail.id}`,
				label: selectedTreeDetail.targetUrl || t.jsScan.siteTree.targetFallback,
				children: normalizeTreeChildren(selectedTreeDetail.treeData),
			},
		]
	}, [selectedTreeDetail, t.jsScan.siteTree.targetFallback])

	const legacyTreeNodes = useMemo(() => {
		return (data || []).map((item, index) => {
			const targetUrl = item.target || item.targetUrl || `${t.jsScan.siteTree.targetFallback} ${index + 1}`
			const treeData = item.siteTree || item.treeData || []
			return {
				id: item.id || `target-${index}`,
				label: targetUrl,
				children: normalizeTreeChildren(treeData),
			}
		})
	}, [data, t.jsScan.siteTree.targetFallback])

	if (!useServerPagination) {
		if (!legacyTreeNodes.length) {
			return (
				<div className="flex flex-col items-center justify-center h-64 text-muted-foreground">
					<Trees className="h-12 w-12 mb-4 opacity-50" />
					<p className="text-lg font-medium">{t.jsScan.siteTree.empty}</p>
				</div>
			)
		}

		return (
			<div className="space-y-3">
				<div className="flex items-center gap-2">
					<Button variant="outline" size="sm" onClick={handleExpandAll}>
						<ChevronsUpDown className="h-4 w-4 mr-1" />
						{t.jsScan.siteTree.expandAll}
					</Button>
					<Button variant="outline" size="sm" onClick={handleCollapseAll}>
						<ChevronsDownUp className="h-4 w-4 mr-1" />
						{t.jsScan.siteTree.collapseAll}
					</Button>
				</div>
				<div className="border rounded-lg p-4 bg-muted/50">
					<SiteTree ref={siteTreeRef} data={legacyTreeNodes} defaultExpandLevel={1} />
				</div>
			</div>
		)
	}

	return (
		<div className="space-y-4">
			<DetailResultToolbar activeCount={Number(Boolean(keyword.trim()))} onReset={() => setKeyword('')}>
<div className="relative">
				<Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
				<Input
					value={keyword}
					onChange={(e) => {
						setKeyword(e.target.value)
					}}
					placeholder={t.jsScan.siteTree.searchPlaceholder}
					className="pl-9"
				/>
			</div>
</DetailResultToolbar>

			<div className="grid gap-4 lg:grid-cols-[360px_minmax(0,1fr)]">
				<div className="border rounded-lg p-3 bg-muted/30 space-y-3">
					<div className="flex items-center justify-between">
						<div className="min-w-0">
							<div className="text-sm font-medium">{t.jsScan.siteTree.targetList}</div>
							<div className="text-xs text-muted-foreground">{t.jsScan.siteTree.totalCount.replace('{count}', String(totalCount))}</div>
						</div>
						<Button
							type="button"
							variant="outline"
							size="sm"
							className="h-8 shrink-0 gap-1.5 text-xs"
							onClick={() => setRouteSortOrder(prev => (prev === 'desc' ? 'asc' : 'desc'))}
						>
							{routeSortOrder === 'desc' ? <ArrowDownWideNarrow className="h-3.5 w-3.5" /> : <ArrowUpNarrowWide className="h-3.5 w-3.5" />}
							{routeSortOrder === 'desc' ? t.jsScan.siteTree.sortRouteDesc : t.jsScan.siteTree.sortRouteAsc}
						</Button>
					</div>
					<div ref={listScrollRef} onScroll={handleListScroll} className="max-h-[440px] overflow-y-auto pr-1">
						{isLoadingList && siteTreeList.length === 0 ? (
							<div className="text-sm text-muted-foreground py-4 text-center">{t.common.loading}</div>
						) : siteTreeList.length === 0 ? (
							<div className="text-sm text-muted-foreground py-4 text-center">{t.jsScan.siteTree.empty}</div>
						) : (
							<>
								<div
									style={{
										height: `${rowVirtualizer.getTotalSize()}px`,
										position: 'relative',
										width: '100%',
									}}
								>
									{rowVirtualizer.getVirtualItems().map((virtualItem) => {
										const item = siteTreeList[virtualItem.index]
										if (!item) return null
										const isActive = selectedTreeId === item.id
										return (
											<div
												key={virtualItem.key}
												style={{
													position: 'absolute',
													top: 0,
													left: 0,
													width: '100%',
													transform: `translateY(${virtualItem.start}px)`,
												}}
											>
												<button
													type="button"
													onClick={() => {
														setSelectedTreeId(item.id)
													}}
													className={`w-full h-[66px] text-left rounded-md border px-2 py-2 transition-colors ${isActive ? 'bg-primary/10 border-primary/40' : 'hover:bg-muted border-border'}`}
												>
													<div className="text-xs font-mono truncate" title={item.targetUrl || '-'}>
														{item.targetUrl || '-'}
													</div>
													<div className="mt-2 flex items-center justify-between gap-3 text-[11px] text-muted-foreground">
														<div className="min-w-0 truncate">
															{item.createdAt ? new Date(item.createdAt).toLocaleString() : '-'}
														</div>
														<div className="shrink-0 text-[10px] font-medium text-primary/80">
															{t.jsScan.siteTree.apiRoutes.replace('{count}', String(item.apiRouteCount ?? 0))}
														</div>
													</div>
												</button>
											</div>
										)
									})}
								</div>
								{isLoadingMore && (
									<div className="py-2 text-xs text-center text-muted-foreground">{t.jsScan.siteTree.loadingMore}</div>
								)}
								{!isLoadingMore && !hasMore && totalCount > 0 && (
									<div className="py-2 text-xs text-center text-muted-foreground">{t.jsScan.siteTree.allLoaded}</div>
								)}
							</>
						)}
					</div>
				</div>

				<div className="min-w-0 space-y-3">
					<div className="flex items-center gap-2">
						<Button variant="outline" size="sm" onClick={handleExpandAll} disabled={!selectedTreeNodes.length || isLoadingDetail}>
							<ChevronsUpDown className="h-4 w-4 mr-1" />
							{t.jsScan.siteTree.expandAll}
						</Button>
						<Button variant="outline" size="sm" onClick={handleCollapseAll} disabled={!selectedTreeNodes.length || isLoadingDetail}>
							<ChevronsDownUp className="h-4 w-4 mr-1" />
							{t.jsScan.siteTree.collapseAll}
						</Button>
					</div>
					<div className="min-w-0 overflow-hidden rounded-lg border bg-muted/50 p-4 min-h-[520px]">
						{isLoadingDetail ? (
							<div className="h-full flex items-center justify-center text-sm text-muted-foreground">{t.jsScan.siteTree.detailLoading}</div>
						) : selectedTreeNodes.length === 0 ? (
							<div className="h-full flex items-center justify-center text-sm text-muted-foreground">{t.jsScan.siteTree.selectTarget}</div>
						) : (
							<SiteTree ref={siteTreeRef} data={selectedTreeNodes} defaultExpandLevel={1} />
						)}
					</div>
				</div>
			</div>
		</div>
	)
}
