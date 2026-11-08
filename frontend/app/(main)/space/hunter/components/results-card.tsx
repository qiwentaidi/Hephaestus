'use client'
import { SpaceResultFilters, mobileSpaceResultClassName } from '../../components/mobile-search-actions'

import { useCallback, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { DataTableWithColumns, type ColumnDef as TableWithColumnsDef, type RowSelection } from '@/components/ui/data-table-with-columns'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { Bug, Copy, Database, Download, MoreHorizontal, Radar, RefreshCw, Search, Send, Target, X } from 'lucide-react'
import type { ColumnDef } from '@/components/ui/data-table'
import type { PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'
import type { HunterResult, SearchTab, TabFilters } from '../types'
import { useI18n } from '@/lib/i18n'

interface ResultsCardProps {
	tabs: SearchTab[]
	activeTab: string
	setActiveTab: (value: string) => void
	currentTab?: SearchTab
	columns: ColumnDef<HunterResult>[]
	isSearching: boolean
	onCloseTab: (tabId: string) => void
	onOpenExportDialog: () => void
	onCopyUrls: (dedup: boolean) => void
	onRefreshCurrentTab: () => Promise<void>
	onRefreshTab: (tabId: string, page: number) => Promise<void>
	onHandlePageSizeChange: (tabId: string, size: number) => Promise<void>
	onHandleCSegmentQuery: (ip: string) => Promise<void>
	onSendToWebscan: (targets: HunterResult[]) => Promise<void>
	onSendToCrack: (targets: HunterResult[]) => Promise<void>
	onSendToPortscan: (targets: HunterResult[], portGroup: PortGroupOption) => Promise<void>
	onSaveToAssetCenter: (targets: HunterResult[]) => Promise<void>
	portScanGroups: PortGroupOption[]
	onTableFiltersChange: (filters: Record<string, string>) => void
	onTabFiltersChange: (tabId: string, nextFilters: Pick<TabFilters, 'timeFilter' | 'isWeb'>) => Promise<void>
}

export function ResultsCard(props: ResultsCardProps) {
	const { t } = useI18n()
	const {
		tabs,
		activeTab,
		setActiveTab,
		currentTab,
		columns,
		isSearching,
		onCloseTab,
		onOpenExportDialog,
		onCopyUrls,
		onRefreshCurrentTab,
		onRefreshTab,
		onHandlePageSizeChange,
		onHandleCSegmentQuery,
		onSendToWebscan,
		onSendToCrack,
		onSendToPortscan,
		onSaveToAssetCenter,
		portScanGroups,
		onTableFiltersChange,
		onTabFiltersChange,
	} = props
	const [selectedRowKeysByTab, setSelectedRowKeysByTab] = useState<Record<string, (string | number)[]>>({})
	const [selectedRowsByTab, setSelectedRowsByTab] = useState<Record<string, HunterResult[]>>({})
	const formatText = useCallback((template: string, vars: Record<string, string | number>) =>
		Object.entries(vars).reduce((acc, [key, value]) => acc.replace(`{${key}}`, String(value)), template), [])

	const mappedColumns = useMemo<TableWithColumnsDef<HunterResult>[]>(() => {
		return [
			...columns.map((col) => ({
				id: col.key,
				header: col.label,
				cell: (item: HunterResult, index?: number) =>
					col.render ? col.render(item[col.key], item, index ?? 0) : (item[col.key] ?? '-'),
				className: col.cellClassName,
				headerClassName: col.headerClassName,
				defaultVisible: col.visible !== false,
			})),
			{
				id: '__actions__',
				header: t.hunterPage.results.actions,
				cell: (item: HunterResult) => (
					<Button
						variant="ghost"
						size="sm"
						className="h-7 px-2"
						onClick={() => onHandleCSegmentQuery(item.IP)}
						title={formatText(t.hunterPage.results.cSegmentQuery, { ip: item.IP })}
					>
						<span
							aria-hidden="true"
							className="mr-1 inline-block h-3.5 w-3.5 bg-current"
							style={{
								WebkitMaskImage: 'url(/icon/csegment.svg)',
								maskImage: 'url(/icon/csegment.svg)',
								WebkitMaskRepeat: 'no-repeat',
								maskRepeat: 'no-repeat',
								WebkitMaskSize: 'contain',
								maskSize: 'contain',
								WebkitMaskPosition: 'center',
								maskPosition: 'center',
							}}
						/>
					</Button>
				),
				className: 'w-[88px] text-center sticky right-0 z-10 border-l border-border bg-card group-hover:bg-muted group-data-[state=selected]:bg-muted',
				headerClassName: 'w-[88px] text-center sticky right-0 z-20 border-l border-border bg-card',
				alwaysVisible: true,
			},
		]
	}, [columns, formatText, onHandleCSegmentQuery, t])

	if (tabs.length === 0 && !isSearching) {
		return (
			<Card className="relative z-0 p-6 md:p-12 scan-panel-enter">
				<div className="text-center text-muted-foreground">
					<Radar className="w-10 h-10 md:w-16 md:h-16 mx-auto mb-3 md:mb-4 opacity-50" />
					<p>{t.hunterPage.results.queryEmpty}</p>
				</div>
			</Card>
		)
	}

	if (tabs.length === 0) return null

	return (
		<Card className={cn('relative z-0 min-w-0 p-3 md:p-6 scan-panel-enter', isSearching && 'scan-surface-pending')} aria-busy={isSearching}>
			<Tabs value={activeTab} onValueChange={setActiveTab}>
				<div className="flex items-center gap-2 mb-4">
					<div className="flex-1 min-w-0 overflow-x-auto -mx-1 px-1">
						<TabsList className="inline-flex min-w-max">
							{tabs.map(tab => (
								<div key={tab.id} className="flex items-center gap-1 shrink-0">
									<TabsTrigger value={tab.id} className="relative whitespace-nowrap py-1">
										{tab.query.slice(0, 20)}
										{tab.query.length > 20 && '...'}
									</TabsTrigger>
									<Button
										variant="ghost"
										size="sm"
										className="h-6 w-6 p-0 shrink-0"
										onClick={e => {
											e.stopPropagation()
											onCloseTab(tab.id)
										}}
									>
										<X className="w-3 h-3" />
									</Button>
								</div>
							))}
						</TabsList>
					</div>
					{currentTab && (
						<div className="shrink-0">
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" size="sm">
										<MoreHorizontal className="w-4 h-4 mr-2" />{t.hunterPage.results.more}
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuItem onClick={onOpenExportDialog}><Download className="w-4 h-4 mr-2" />{t.hunterPage.results.exportData}</DropdownMenuItem>
									<DropdownMenuItem onClick={() => onCopyUrls(false)}><Copy className="w-4 h-4 mr-2" />{t.hunterPage.results.copyCurrentUrls}</DropdownMenuItem>
									<DropdownMenuItem onClick={() => onCopyUrls(true)}><Copy className="w-4 h-4 mr-2" />{t.hunterPage.results.dedupCopyCurrentUrls}</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
						</div>
					)}
				</div>

				{tabs.map(tab => {
					const globalKeyword = (tab.tableFilters?.__global || '').trim().toLowerCase()
					const filteredRows = !globalKeyword
						? tab.results
						: tab.results.filter((row) =>
								Object.values(row).some((value) => {
									if (value === null || value === undefined) return false
									try {
										return String(value).toLowerCase().includes(globalKeyword)
									} catch {
										return false
									}
								})
						  )

					return (
					<TabsContent key={tab.id} value={tab.id} className="mt-0">
						<DataTableWithColumns
							className={mobileSpaceResultClassName}
                mobileCards
							data={filteredRows}
							columns={mappedColumns}
							getRowKey={(item, index) => `${item.IP || '-'}-${item.Port || '-'}-${item.URL || '-'}-${index}`}
							rowSelection={{
								selectedRowKeys: selectedRowKeysByTab[tab.id] || [],
								onChange: (keys, rows) => {
									setSelectedRowKeysByTab(prev => ({ ...prev, [tab.id]: keys }))
									setSelectedRowsByTab(prev => ({ ...prev, [tab.id]: rows }))
								},
							} as RowSelection<HunterResult>}
							loading={tab.isLoading}
							emptyMessage={globalKeyword ? t.hunterPage.results.noFilteredData : tab.message}
							toolbarLeft={(
								<SpaceResultFilters count={Number(Boolean(tab.tableFilters?.__global)) + Number(tab.filters.timeFilter !== '0') + Number(tab.filters.isWeb !== 3)}><div className="flex flex-wrap items-center gap-2">
									<Select
										value={tab.filters.timeFilter}
										onValueChange={(value) => {
											void onTabFiltersChange(tab.id, { timeFilter: value, isWeb: tab.filters.isWeb })
										}}
									>
										<SelectTrigger className="w-[140px] h-9">
											<SelectValue placeholder={t.hunterPage.results.timeFilter} />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="0">{t.hunterPage.results.latestMonth}</SelectItem>
											<SelectItem value="1">{t.hunterPage.results.latestHalfYear}</SelectItem>
											<SelectItem value="2">{t.hunterPage.results.latestYear}</SelectItem>
										</SelectContent>
									</Select>
									<Select
										value={String(tab.filters.isWeb)}
										onValueChange={(value) => {
											void onTabFiltersChange(tab.id, { timeFilter: tab.filters.timeFilter, isWeb: Number(value) })
										}}
									>
										<SelectTrigger className="w-[160px] h-9">
											<SelectValue placeholder={t.hunterPage.results.assetType} />
										</SelectTrigger>
										<SelectContent>
											<SelectItem value="3">{t.hunterPage.results.allAssets}</SelectItem>
											<SelectItem value="1">{t.hunterPage.results.webAssets}</SelectItem>
											<SelectItem value="2">{t.hunterPage.results.nonWebAssets}</SelectItem>
										</SelectContent>
									</Select>
									<div className="relative w-full max-w-sm">
										<Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
										<Input
											value={tab.tableFilters?.__global || ''}
											onChange={(e) => onTableFiltersChange({ __global: e.target.value })}
											placeholder={t.hunterPage.results.filterCurrentPage}
											className="pl-9"
										/>
									</div>
								</div></SpaceResultFilters>
							)}
							toolbarRight={(
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button variant="outline" size="sm" className="h-9">
											<Send className="w-4 h-4 mr-2" />
											{t.hunterPage.results.sendTo}
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end">
										<DropdownMenuItem
											disabled={(selectedRowsByTab[tab.id] || []).length === 0}
											onClick={() => void onSaveToAssetCenter(selectedRowsByTab[tab.id] || [])}
										>
											<Database className="w-4 h-4 mr-2" />
											{t.common.saveToAssetCenter}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
										</DropdownMenuItem>
										<DropdownMenuItem
											disabled={(selectedRowsByTab[tab.id] || []).length === 0}
											onClick={() => void onSendToWebscan(selectedRowsByTab[tab.id] || [])}
										>
											<Bug className="w-4 h-4 mr-2" />
											{t.hunterPage.results.sendToVulnScan}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
										</DropdownMenuItem>
										<DropdownMenuItem
											disabled={(selectedRowsByTab[tab.id] || []).length === 0}
											onClick={() => void onSendToCrack(selectedRowsByTab[tab.id] || [])}
										>
											<Target className="w-4 h-4 mr-2" />
											{t.hunterPage.results.sendToCrack}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
										</DropdownMenuItem>
										<DropdownMenuSub>
											<DropdownMenuSubTrigger disabled={(selectedRowsByTab[tab.id] || []).length === 0}>
												<Target className="w-4 h-4 mr-2" />
												{t.hunterPage.results.sendToPortScan}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
											</DropdownMenuSubTrigger>
											<DropdownMenuSubContent>
												{portScanGroups.map((group) => (
													<DropdownMenuItem
														key={group.id}
														onClick={() => void onSendToPortscan(selectedRowsByTab[tab.id] || [], group)}
													>
														{group.text}
													</DropdownMenuItem>
												))}
											</DropdownMenuSubContent>
										</DropdownMenuSub>
									</DropdownMenuContent>
								</DropdownMenu>
							)}
						/>

						<div className="mt-4 flex flex-wrap items-center justify-between">
							<div className="flex items-cente">
								<div className={cn('flex items-center gap-2 text-sm text-muted-foreground', tab.error && 'text-destructive')}>
									{tab.isLoading && !tab.error && (
										<div className="w-3 h-3 border-2 border-border/60 border-t-transparent rounded-full animate-spin" />
									)}
									<span>{tab.error ? tab.error : tab.isLoading ? t.hunterPage.results.loadingData : tab.message}</span>
								</div>
							</div>
							<PaginationPro
								current={tab.currentPage}
								total={tab.total}
								pageSize={tab.pageSize}
								onChange={async (page) => {
									try {
										await onRefreshTab(tab.id, page)
									} catch (error: any) {
										console.error('Pagination error:', error)
									}
								}}
								onPageSizeChange={(size) => onHandlePageSizeChange(tab.id, size)}
								pageSizeOptions={[10, 50, 100]}
								showSizeChanger
								showQuickJumper
								showTotal
								disabled={tab.isLoading}
							/>
						</div>
					</TabsContent>
					)
				})}
			</Tabs>
		</Card>
	)
}
