'use client'
import { SpaceResultFilters, mobileSpaceResultClassName } from '../../components/mobile-search-actions'

import { useCallback, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { DataTableWithColumns, type ColumnDef as TableWithColumnsDef, type RowSelection } from '@/components/ui/data-table-with-columns'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { Bug, Copy, Database, Download, MoreHorizontal, Radar, RefreshCw, Search, Send, ShieldCheck, Target, X } from 'lucide-react'
import type { ColumnDef } from '@/components/ui/data-table'
import type { PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'
import type { QuakeResult, SearchTab } from '../types'
import { useI18n } from '@/lib/i18n'

interface QuakeResultsCardProps {
	tabs: SearchTab[]
	activeTab: string
	onTabChange: (tabId: string) => void
	onCloseTab: (tabId: string) => void
	isSearching: boolean
	currentTab?: SearchTab
	columns: ColumnDef<QuakeResult>[]
	onOpenExportDialog: () => void
	onCopyUrls: (type: 'current' | 'top100', dedup?: boolean) => void
	onHandleCSegmentQuery: (ip: string) => Promise<void>
	onHandleCertQuery: (certName: string) => Promise<void>
	onRefreshTab: (tabId: string, page: number) => Promise<void>
	onPageSizeChange: (tabId: string, size: number) => Promise<void>
	onTableFiltersChange: (filters: Record<string, string>) => void
	onTabFiltersChange: (tabId: string, latest: boolean) => Promise<void>
	onSendToWebscan: (targets: QuakeResult[]) => Promise<void>
	onSendToCrack: (targets: QuakeResult[]) => Promise<void>
	onSendToPortscan: (targets: QuakeResult[], portGroup: PortGroupOption) => Promise<void>
	onSaveToAssetCenter: (targets: QuakeResult[]) => Promise<void>
	portScanGroups: PortGroupOption[]
}

export function QuakeResultsCard(props: QuakeResultsCardProps) {
	const { t } = useI18n()
	const {
		tabs,
		activeTab,
		onTabChange,
		onCloseTab,
		isSearching,
		currentTab,
		columns,
		onOpenExportDialog,
		onCopyUrls,
		onHandleCSegmentQuery,
		onHandleCertQuery,
		onRefreshTab,
		onPageSizeChange,
		onTableFiltersChange,
		onTabFiltersChange,
		onSendToWebscan,
		onSendToCrack,
		onSendToPortscan,
		onSaveToAssetCenter,
		portScanGroups,
	} = props
	const [selectedRowKeysByTab, setSelectedRowKeysByTab] = useState<Record<string, (string | number)[]>>({})
	const [selectedRowsByTab, setSelectedRowsByTab] = useState<Record<string, QuakeResult[]>>({})
	const formatText = useCallback((template: string, vars: Record<string, string | number>) =>
		Object.entries(vars).reduce((acc, [key, value]) => acc.replace(`{${key}}`, String(value)), template), [])

	const mappedColumns = useMemo<TableWithColumnsDef<QuakeResult>[]>(() => {
		return [
			...columns.map((col) => ({
				id: col.key,
				header: col.label,
				cell: (item: QuakeResult, index?: number) =>
					col.render ? col.render(item[col.key as keyof QuakeResult], item, index ?? 0) : (item[col.key as keyof QuakeResult] ?? '-'),
				className: col.cellClassName,
				headerClassName: col.headerClassName,
				defaultVisible: col.visible !== false,
			})),
			{
				id: '__actions__',
				header: t.quakePage.results.actions,
				cell: (item: QuakeResult) => (
					<div className="flex items-center justify-center gap-1">
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="sm"
									className="h-7 w-7 p-0"
									onClick={() => item.certName && onHandleCertQuery(item.certName)}
									disabled={!item.certName?.trim()}
								>
									<ShieldCheck className="h-3.5 w-3.5" />
								</Button>
							</TooltipTrigger>
							<TooltipContent>{t.quakePage.results.certQuery}</TooltipContent>
						</Tooltip>
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="sm"
									className="h-7 w-7 p-0"
									onClick={() => item.ip && onHandleCSegmentQuery(item.ip)}
									disabled={!item.ip}
								>
									<span
										aria-hidden="true"
										className="inline-block h-3.5 w-3.5 bg-current"
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
							</TooltipTrigger>
							<TooltipContent>{item.ip ? formatText(t.quakePage.results.cSegmentQuery, { ip: item.ip }) : t.quakePage.results.noIpInfo}</TooltipContent>
						</Tooltip>
					</div>
				),
				className: 'w-[88px] text-center sticky right-0 z-10 border-l border-border bg-card group-hover:bg-muted group-data-[state=selected]:bg-muted',
				headerClassName: 'w-[88px] text-center sticky right-0 z-20 border-l border-border bg-card',
				alwaysVisible: true,
			},
		]
	}, [columns, formatText, onHandleCSegmentQuery, onHandleCertQuery, t])

	if (tabs.length === 0 && !isSearching) {
		return (
			<Card className="relative z-0 p-6 md:p-12 scan-panel-enter">
				<div className="text-center text-muted-foreground">
					<Radar className="w-10 h-10 md:w-16 md:h-16 mx-auto mb-3 md:mb-4 opacity-50" />
					<p>{t.quakePage.results.queryEmpty}</p>
				</div>
			</Card>
		)
	}

	if (tabs.length === 0) return null

	return (
		<Card className={cn('relative z-0 min-w-0 p-3 md:p-6 scan-panel-enter', isSearching && 'scan-surface-pending')} aria-busy={isSearching}>
			<Tabs value={activeTab} onValueChange={onTabChange}>
				<div className="flex items-center gap-2 mb-4">
					<div className="flex-1 min-w-0 overflow-x-auto -mx-1 px-1">
						<TabsList className="inline-flex min-w-max">
							{tabs.map(tab => (
								<div key={tab.id} className="flex items-center gap-1 shrink-0">
									<TabsTrigger value={tab.id} className="relative whitespace-nowrap py-1">
										{tab.query.slice(0, 24)}
										{tab.query.length > 24 && '...'}
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
						<div className="flex items-center gap-2 shrink-0">
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<Button variant="outline" size="sm">
										<MoreHorizontal className="w-4 h-4 mr-2" />
										{t.quakePage.results.more}
									</Button>
								</DropdownMenuTrigger>
								<DropdownMenuContent align="end">
									<DropdownMenuItem onClick={onOpenExportDialog}>
										<Download className="w-4 h-4 mr-2" />
										{t.quakePage.results.exportData}
									</DropdownMenuItem>
									<DropdownMenuItem
										onSelect={() => {
											onCopyUrls('current', false)
										}}
									>
										<Copy className="w-4 h-4 mr-2" />
										{t.quakePage.results.copyCurrentUrls}
									</DropdownMenuItem>
									<DropdownMenuItem
										onSelect={() => {
											onCopyUrls('current', true)
										}}
									>
										<Copy className="w-4 h-4 mr-2" />
										{t.quakePage.results.dedupCopyCurrentUrls}
									</DropdownMenuItem>
									<DropdownMenuItem
										onClick={async () => {
											try {
												await onRefreshTab(currentTab.id, currentTab.currentPage)
											} catch (error) {
												console.error('Refresh error:', error)
											}
										}}
									>
										<RefreshCw className="w-4 h-4 mr-2" />
										{t.quakePage.results.reload}
									</DropdownMenuItem>
								</DropdownMenuContent>
							</DropdownMenu>
						</div>
					)}
				</div>

				{tabs.map(tab => {
					const globalKeyword = (tab.tableFilters?.__global || '').trim().toLowerCase()
					const filteredRows = !globalKeyword
						? tab.results
						: tab.results.filter(row =>
								Object.values(row).some(value => {
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
								getRowKey={(item, index) => `${item.ip || '-'}-${item.port || '-'}-${index}`}
								loading={tab.isLoading}
								emptyMessage={tab.error || tab.message || t.quakePage.results.noData}
								rowSelection={{
									selectedRowKeys: selectedRowKeysByTab[tab.id] || [],
									onChange: (keys, rows) => {
										setSelectedRowKeysByTab(prev => ({ ...prev, [tab.id]: keys }))
										setSelectedRowsByTab(prev => ({ ...prev, [tab.id]: rows }))
									},
								} as RowSelection<QuakeResult>}
								toolbarLeft={
									<SpaceResultFilters count={Number(Boolean(tab.tableFilters?.__global))}><div className="relative w-full max-w-sm">
										<Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
										<Input
											value={tab.tableFilters?.__global || ''}
											onChange={(e) => onTableFiltersChange({ __global: e.target.value })}
											placeholder={t.quakePage.results.filterCurrentPage}
											className="pl-9"
										/>
									</div></SpaceResultFilters>
								}
								toolbarRight={
									<DropdownMenu>
										<DropdownMenuTrigger asChild>
											<Button variant="outline" size="sm" className="h-9">
												<Send className="w-4 h-4 mr-2" />
												{t.quakePage.results.sendTo}
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
												{t.quakePage.results.sendToVulnScan}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
											</DropdownMenuItem>
											<DropdownMenuItem
												disabled={(selectedRowsByTab[tab.id] || []).length === 0}
												onClick={() => void onSendToCrack(selectedRowsByTab[tab.id] || [])}
											>
												<Target className="w-4 h-4 mr-2" />
												{t.quakePage.results.sendToCrack}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
											</DropdownMenuItem>
											<DropdownMenuSub>
												<DropdownMenuSubTrigger disabled={(selectedRowsByTab[tab.id] || []).length === 0}>
													<Target className="w-4 h-4 mr-2" />
													{t.quakePage.results.sendToPortScan}{(selectedRowsByTab[tab.id] || []).length > 0 ? ` (${(selectedRowsByTab[tab.id] || []).length})` : ''}
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
								}
							/>

							<div className="mt-4 flex flex-wrap items-center justify-between gap-3">
								<div className={cn('flex items-center gap-2 text-sm text-muted-foreground', tab.error && 'text-destructive')}>
									{tab.isLoading && !tab.error && (
										<div className="w-3 h-3 border-2 border-border/60 border-t-transparent rounded-full animate-spin" />
									)}
									<span>{tab.error ? tab.error : tab.isLoading ? t.quakePage.results.loadingData : tab.message}</span>
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
									onPageSizeChange={(size) => onPageSizeChange(tab.id, size)}
									pageSizeOptions={[10, 20, 50, 100]}
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
