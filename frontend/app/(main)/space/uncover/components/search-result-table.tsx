'use client'

import {
	Download,
	ExternalLink,
	FileJson,
	FileSpreadsheet,
	FileText,
	KeyRound,
	Database,
	Send,
	Shield,
	Target,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { DataTableWithColumns, type ColumnDef } from '@/components/ui/data-table-with-columns'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import type { SearchResultTableProps, SpaceResult } from '../types'

export function SearchResultTable({
	tab,
	loading,
	paginatedResults,
	columns,
	selectedResults,
	selectedCount,
	emptyMessage,
	actionsColumnLabel,
	actionsColumnClassName,
	onSelectRows,
	onExportData,
	onSendToWebscan,
	onSendToCrack,
	onSendToPortscan,
	onSaveToAssetCenter,
	portScanGroups,
	getRowKey,
	sendToLabel,
	sendToWebscanLabel,
	sendToCrackLabel,
	sendToPortscanLabel,
	saveToAssetCenterLabel,
	exportLabel,
	exportCurrentCsvLabel,
	exportCurrentJsonLabel,
	exportCurrentTxtLabel,
	openLinkTitle,
	cSegmentQueryTitle,
	onOpenURL,
	onCSegmentQuery,
}: SearchResultTableProps) {
	const tableColumns: ColumnDef<SpaceResult>[] = [
		...columns,
		{
			id: 'actions',
			header: actionsColumnLabel,
			headerClassName: `w-24 sticky right-0 z-20 border-l border-border bg-card text-center ${actionsColumnClassName || ''}`,
			className: `w-24 sticky right-0 z-10 border-l border-border bg-card text-center group-hover:bg-muted group-data-[state=selected]:bg-muted ${actionsColumnClassName || ''}`,
			alwaysVisible: true,
			cell: row => (
				<div className="flex items-center justify-center gap-1">
					<Button
						variant="ghost"
						size="sm"
						className="h-7 w-7 p-0"
						onClick={() => onOpenURL(row.URL)}
						title={openLinkTitle}
					>
						<ExternalLink className="h-3.5 w-3.5" />
					</Button>
					<Button
						variant="ghost"
						size="sm"
						className="h-7 w-7 p-0"
						onClick={() => onCSegmentQuery(row.IP)}
						title={cSegmentQueryTitle}
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
				</div>
			),
		},
	]

	return (
		<div className={cn('scan-panel-enter', loading && 'scan-surface-pending')} aria-busy={loading}>
			<DataTableWithColumns
        mobileCards
			data={paginatedResults}
			columns={tableColumns}
			loading={loading}
			emptyMessage={emptyMessage}
			getRowKey={(row, index) => {
				const absoluteIndex = (tab.currentPage - 1) * 100 + index
				return getRowKey(row, absoluteIndex)
			}}
			rowSelection={{
				selectedRowKeys: tab.selectedRowKeys || [],
				onChange: selectedRowKeys => onSelectRows(tab.id, selectedRowKeys),
			}}
			toolbarRight={(
				<>
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="ghost"
								size="sm"
								className="border border-border/60 bg-muted/35 px-4 text-foreground shadow-none hover:bg-muted/60"
								disabled={selectedCount === 0}
							>
								<Send className="h-4 w-4 mr-1" />
								{sendToLabel}
								{selectedCount > 0 && (
									<span className="ml-1 text-xs text-muted-foreground">({selectedCount})</span>
								)}
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem disabled={selectedCount === 0} onClick={() => void onSaveToAssetCenter(selectedResults)}>
								<Database className="h-4 w-4 mr-2" />
								{saveToAssetCenterLabel}{selectedCount > 0 ? ` (${selectedCount})` : ''}
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => onSendToWebscan(selectedResults)}>
								<Shield className="h-4 w-4 mr-2" />
								{sendToWebscanLabel}
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => onSendToCrack(selectedResults)}>
								<KeyRound className="h-4 w-4 mr-2" />
								{sendToCrackLabel}
							</DropdownMenuItem>
							<DropdownMenuSub>
								<DropdownMenuSubTrigger disabled={selectedCount === 0}>
									<Target className="h-4 w-4 mr-2" />
									{sendToPortscanLabel}
								</DropdownMenuSubTrigger>
								<DropdownMenuSubContent>
									{portScanGroups.map(group => (
										<DropdownMenuItem
											key={group.id}
											onClick={() => void onSendToPortscan(selectedResults, group)}
										>
											{group.text}
										</DropdownMenuItem>
									))}
								</DropdownMenuSubContent>
							</DropdownMenuSub>
						</DropdownMenuContent>
					</DropdownMenu>

					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								variant="ghost"
								size="sm"
								className="border border-border/60 bg-muted/35 px-4 text-foreground shadow-none hover:bg-muted/60"
							>
								<Download className="h-4 w-4 mr-1" />
								{exportLabel}
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem onClick={() => onExportData(tab.results, 'csv')}>
								<FileSpreadsheet className="mr-2 h-4 w-4 text-muted-foreground" />
								{exportCurrentCsvLabel}
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => onExportData(tab.results, 'json')}>
								<FileJson className="mr-2 h-4 w-4 text-muted-foreground" />
								{exportCurrentJsonLabel}
							</DropdownMenuItem>
							<DropdownMenuItem onClick={() => onExportData(tab.results, 'txt')}>
								<FileText className="mr-2 h-4 w-4 text-muted-foreground" />
								{exportCurrentTxtLabel}
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>
				</>
			)}
			/>
		</div>
	)
}
