export interface QuakeResult {
	url: string
	ip: string
	port: number
	protocol: string
	host: string
	title: string
	components: string[]
	icpName: string
	icpNumber: string
	certName: string
	faviconURL: string
	isp: string
	position: string
}

export interface TabFilters {
	latest: boolean
}

export interface SearchTab {
	id: string
	query: string
	results: QuakeResult[]
	total: number
	pageSize: number
	currentPage: number
	message: string
	filters: TabFilters
	ipList?: string[]
	isLoading?: boolean
	error?: string
	tableFilters?: Record<string, string>
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
	isVip?: boolean
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
