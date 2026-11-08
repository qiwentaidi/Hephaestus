'use client'

import type { LucideIcon } from 'lucide-react'
import type { ColumnDef } from '@/components/ui/data-table-with-columns'
import type { PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'

export interface SpaceResult {
	URL: string
	IP: string
	Domain: string
	Port: string
	Protocol: string
	Title: string
	Components: string
	ICP: string
	ICPName: string
	Source: string
}

export interface EngineStatus {
	success: boolean
	total: number
	count: number
	message: string
}

export interface EngineQuota {
	engine: string
	configured: boolean
	username?: string
	fcoin?: number | null
	fofaPoint?: number | null
	remainFreePoint?: number | null
	remainApiQuery?: number | null
	restEquityPoint?: number | null
	restFreePoint?: number | null
	dayFreePoint?: number | null
	credit?: number | null
	persistentCredit?: number | null
	points?: number | null
	zoomeyePoints?: number | null
	queryCredits?: number | null
	scanCredits?: number | null
	plan?: string
	error?: string
}

export interface SearchTab {
	id: string
	title: string
	query: string
	results: SpaceResult[]
	total: number
	currentPage: number
	sessionId?: string
	hasMore?: boolean
	engineStatus?: Record<string, EngineStatus>
	loadingMore?: boolean
	reachedMaxLimit?: boolean
	maxResults?: number
	selectedRowKeys?: (string | number)[]
	historyId?: string
	isMergedHistory?: boolean
}

export interface SpaceEngineConfig {
	fofa: {
		api: string
		email: string
		key: string
	}
	hunter: {
		api: string
		key: string
	}
	quake: {
		key: string
	}
	zoomeye: {
		api: string
		key: string
	}
	censys: {
		token: string
		organizationId: string
	}
	shodan: {
		api: string
		key: string
	}
}

export interface QuerySuggestion {
	key: string
	label: string
	insertText: string
	description: string
	type: 'field' | 'operator'
}

export interface SpaceHistoryListItem {
	id: string
	sessionId: string
	query: string
	queryType: string
	groupId?: string
	groupName?: string
	engines: string[]
	totalCount: number
	hasMore: boolean
	reachedMaxLimit: boolean
	maxResults: number
	createdBy: string
	createdAt: string
	updatedAt: string
}

export interface SpaceHistoryDetail extends SpaceHistoryListItem {
	results: Array<{
		url?: string
		ip?: string
		domain?: string
		port?: number
		protocol?: string
		title?: string
		components?: string[]
		icp?: string
		icpName?: string
		source?: string
	}>
	engineStatus: Record<string, EngineStatus>
}

export interface HistoryGroup {
	id: string
	label: string
	groupId?: string
	groupName?: string
	isUngrouped?: boolean
	items: SpaceHistoryListItem[]
}

export interface SearchStats {
	sourceStats: Array<{ name: string; count: number }>
	protocolStats: Array<{ name: string; count: number }>
	portStats: Array<{ name: string; count: number }>
	componentStats: Array<{ name: string; count: number }>
	uniqueIPs: number
	uniqueDomains: number
}

export interface EngineOption {
	key: string
	label: string
	icon: LucideIcon
	enabled: boolean
	configured: boolean
	toggle: () => void
}

export interface ResultToolbarProps {
	tab: SearchTab
	statsCollapsed: boolean
	onToggleStats: () => void
	onLoadMore: (tabId: string) => void
	statsTitle: string
	canLoadMoreLabel: string
	cacheLimitReachedLabel: string
	queryFailedLabel: string
	loadMoreLabel: string
	loadingMoreLabel: string
	getEngineDisplayName: (name: string) => string
	getEngineStatusBadgeText: (tab: SearchTab, name: string, status: EngineStatus) => string
	formatText: (template: string, vars: Record<string, string | number>) => string
}

export interface ResultStatsPanelProps {
	stats: SearchStats
	title: string
	sourceStatsLabel: string
	portStatsLabel: string
	protocolStatsLabel: string
	componentStatsLabel: string
	onCollapse: () => void
}

export interface SearchResultTableProps {
	tab: SearchTab
	loading: boolean
	paginatedResults: SpaceResult[]
	columns: ColumnDef<SpaceResult>[]
	selectedResults: SpaceResult[]
	selectedCount: number
	emptyMessage: string
	actionsColumnLabel: string
	actionsColumnClassName?: string
	onSelectRows: (tabId: string, selectedRowKeys: (string | number)[]) => void
	onExportData: (data: SpaceResult[], format: 'csv' | 'json' | 'txt') => void
	onSendToWebscan: (selectedResults: SpaceResult[]) => void
	onSendToCrack: (selectedResults: SpaceResult[]) => void
	onSendToPortscan: (selectedResults: SpaceResult[], group: PortGroupOption) => Promise<void>
	onSaveToAssetCenter: (selectedResults: SpaceResult[]) => Promise<void>
	portScanGroups: PortGroupOption[]
	getRowKey: (row: SpaceResult, index: number) => string
	sendToLabel: string
	sendToWebscanLabel: string
	sendToCrackLabel: string
	sendToPortscanLabel: string
	saveToAssetCenterLabel: string
	exportLabel: string
	exportCurrentCsvLabel: string
	exportCurrentJsonLabel: string
	exportCurrentTxtLabel: string
	openLinkTitle: string
	cSegmentQueryTitle: string
	onOpenURL: (url: string) => void
	onCSegmentQuery: (ip: string) => void
}
