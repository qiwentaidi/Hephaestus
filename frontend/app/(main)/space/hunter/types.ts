export interface HunterResult {
	URL: string
	IP: string
	Port: number
	Protocol: string
	Domain: string
	Component: string
	Title: string
	Status: number
	ICP: string
	ISP: string
	Position: string
	UpdateTime: string
	[key: string]: any
}

export interface HunterUserInfo {
	code: number
	message: string
	data: {
		type: string
		rest_equity_point: number
		rest_free_point: number
		rest_export_quota: number
		day_free_point: number
		day_export_quota: number
		once_export_quota: number
		personal_info: {
			username: string
			phone: string
			is_charge: boolean
		}
	}
}

export interface TabFilters {
	timeFilter: string
	isWeb: number
	deduplication: boolean
}

export interface SearchTab {
	id: string
	query: string
	results: HunterResult[]
	total: number
	pageSize: number
	currentPage: number
	message: string
	filters: TabFilters
	tableFilters?: Record<string, string>
	isLoading?: boolean
	error?: string
	lastUpdated?: number
	consumeQuota?: string
	restQuota?: string
}

export interface FavSyntax {
	id?: number
	source?: string
	Name: string
	Content: string
}

export interface SyntaxItem {
	key: string
	description: string
	hot?: boolean
	characteristic?: boolean
}

export interface SyntaxCategory {
	title: string
	data: SyntaxItem[]
}

export interface SuggestionItem {
	key: string
	description: string
	category: string
}
