'use client'

import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { type ColumnDef } from '@/components/ui/data-table'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { ExternalLink, X, Star, Copy } from 'lucide-react'
import { ClientKeepAlive } from '@/components/client-keep-alive'
import { getSpaceEngineConfig, onSpaceEngineConfigChange } from '@/lib/config'
import { fetchWithAuth } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { saveSpaceEngineAssets } from '@/lib/space-engine-assets'
import { useSpaceEngineQueryParam } from '@/lib/space-engine'
import toast from 'react-hot-toast'
import { SearchFormCard } from './components/search-form-card'
import { ResultsCard } from './components/results-card'
import { SelectTag } from '@/components/ui/select-tag'
import { builtInPortGroupOptions, mergePortGroupOptions, type PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'
import { DEFAULT_PAGE_SIZE, hunterSyntaxData } from './constants'
import type { FavSyntax, HunterResult, HunterUserInfo, SearchTab, SuggestionItem, TabFilters } from './types'
import { formatProduct, getResultMessage, transformHunterData } from './utils'
import { useI18n } from '@/lib/i18n'
import { useIsMobile } from '@/hooks/use-mobile'

const EXPORT_PAGE_SIZE = 100

const defaultExportFields: string[] = ['IP', 'Port', 'Domain', 'URL', 'Protocol']
const DEFAULT_TAB_FILTERS: TabFilters = {
	timeFilter: '0',
	isWeb: 3,
	deduplication: false,
}

function HunterPageContent() {
	const { t } = useI18n()
	const isMobile = useIsMobile()
	const formatText = (template: string, values: Record<string, string | number>) =>
		template.replace(/\{(\w+)\}/g, (_, key) => String(values[key] ?? ''))
	const [query, setQuery] = useState('')
	useSpaceEngineQueryParam(setQuery)
	const [isSearching, setIsSearching] = useState(false)
	const [tabs, setTabs] = useState<SearchTab[]>([])
	const [activeTab, setActiveTab] = useState<string>('')
	const [favSyntax, setFavSyntax] = useState<FavSyntax[]>([])
	const [showAddFavDialog, setShowAddFavDialog] = useState(false)
	const [showFavSyntaxDialog, setShowFavSyntaxDialog] = useState(false)
	const [showApiConfigDialog, setShowApiConfigDialog] = useState(false)
	const [showTipsDialog, setShowTipsDialog] = useState(false)
	const [showExportDialog, setShowExportDialog] = useState(false)
	const [syntaxKeyword, setSyntaxKeyword] = useState('IP')
	const [newSyntaxName, setNewSyntaxName] = useState('')
	const [newSyntaxContent, setNewSyntaxContent] = useState('')
	const [suggestions, setSuggestions] = useState<SuggestionItem[]>([])
	const [showSuggestions, setShowSuggestions] = useState(false)
	const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(-1)
	const [inputFocused, setInputFocused] = useState(false)
	const suggestionsRef = useRef<HTMLDivElement>(null)
	const textareaRef = useRef<HTMLTextAreaElement>(null)
	const [searchHistory, setSearchHistory] = useState<string[]>([])
	const [userInfo, setUserInfo] = useState<HunterUserInfo | null>(null)
	const [isLoadingUserInfo, setIsLoadingUserInfo] = useState(false)
	const [portScanGroups, setPortScanGroups] = useState<PortGroupOption[]>(builtInPortGroupOptions)
	const [exportFileName, setExportFileName] = useState(`hunter_${new Date().toISOString().slice(0, 10).replace(/-/g, '')}`)
	const [exportFormat, setExportFormat] = useState('csv')
	const [exportStartPage, setExportStartPage] = useState(1)
	const [exportEndPage, setExportEndPage] = useState(1)
	const [selectedExportFields, setSelectedExportFields] = useState<string[]>(defaultExportFields)
	const [isExporting, setIsExporting] = useState(false)
	const exportFieldLabelMap: Record<string, string> = useMemo(() => ({
		URL: 'URL',
		IP: 'IP',
		Port: t.hunterPage.exportFields.Port,
		Protocol: t.hunterPage.exportFields.Protocol,
		Domain: t.hunterPage.exportFields.Domain,
		Component: t.hunterPage.exportFields.Component,
		Title: t.hunterPage.exportFields.Title,
		Status: t.hunterPage.exportFields.Status,
		ICP: t.hunterPage.exportFields.ICP,
		ISP: t.hunterPage.exportFields.ISP,
		Position: t.hunterPage.exportFields.Position,
		UpdateTime: t.hunterPage.exportFields.UpdateTime,
	}), [t])

	// 更新当前 tab 的表格筛选状态
	const setTableFilters = (filters: Record<string, string>) => {
		if (!activeTab) return
		updateTabState(activeTab, tab => ({
			...tab,
			tableFilters: filters,
		}))
	}

	// API 配置从后端加载
	const [apiConfig, setApiConfig] = useState({
		api: '',
		key: '',
	})

	// 加载配置
	const loadApiConfig = async () => {
		try {
			const spaceConfig = await getSpaceEngineConfig()
			setApiConfig({
				api: spaceConfig.hunter.api || 'https://hunter.qianxin.com',
				key: spaceConfig.hunter.key || '',
			})
		} catch (error) {
			console.error('Failed to load API config:', error)
			// 如果后端不可用，尝试从localStorage加载（兼容旧数据）
			const savedConfig = localStorage.getItem('spaceConfig')
			if (savedConfig) {
				try {
					const parsed = JSON.parse(savedConfig)
					setApiConfig({
						api: parsed.hunter?.api || 'https://hunter.qianxin.com',
						key: parsed.hunter?.key || '',
					})
				} catch (e) {
					console.error('Failed to parse saved config:', e)
				}
			}
		}
	}

	const loadPortScanGroups = async () => {
		try {
			const response = await fetchWithAuth('/api/v1/config/port-groups', { cache: 'no-store' })
			const data = await response.json()
			if (!response.ok || !data.success) {
				throw new Error(data.error || '加载端口组失败')
			}
			const customGroups: PortGroupOption[] = Array.isArray(data.data)
				? data.data.map((item: any) => ({
					id: String(item.id || ''),
					text: String(item.name || ''),
					value: String(item.ports || ''),
				}))
				: []
			setPortScanGroups(mergePortGroupOptions(customGroups))
		} catch (error) {
			console.error('Failed to load port scan groups:', error)
			setPortScanGroups(builtInPortGroupOptions)
		}
	}

	useEffect(() => {
		// 加载配置
		loadApiConfig()
		loadPortScanGroups()
		// 加载收藏语法
		loadFavSyntax().catch(console.error)

		// 监听配置变更（当设置页面保存配置时自动更新）
		const unsubscribe = onSpaceEngineConfigChange((config) => {
			const newConfig = {
				api: config.hunter.api || 'https://hunter.qianxin.com',
				key: config.hunter.key || '',
			}
			setApiConfig(newConfig)
			// 如果配置了 API Key，自动获取用户信息
			if (newConfig.key) {
				fetchUserInfo(false)
			}
		})

		return () => {
			unsubscribe()
		}
	}, [])

	// 当 API 配置加载完成后，如果有 Key，自动获取用户信息
	useEffect(() => {
		if (apiConfig.key && !userInfo && !isLoadingUserInfo) {
			fetchUserInfo(false)
		}
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [apiConfig.key])

	const loadFavSyntax = async () => {
		try {
			const response = await fetchWithAuth(`/api/v1/syntax?source=hunter`, {
				method: 'GET',
				headers: {
					'Content-Type': 'application/json',
				},
			})

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`)
			}

			const data = await response.json()
			if (data.success && data.data) {
				// 转换后端数据格式到前端格式
				const formatted = data.data.map((item: any) => ({
					id: item.id,
					source: item.source,
					Name: item.name,
					Content: item.content,
				}))
				setFavSyntax(formatted)
			}
		} catch (error) {
			console.error('Failed to load favorite syntax:', error)
			// 如果API调用失败，尝试从localStorage加载（向后兼容）
			try {
				const saved = localStorage.getItem('hunter_fav_syntax')
				if (saved) {
					const parsed = JSON.parse(saved)
					setFavSyntax(Array.isArray(parsed) ? parsed : [])
				}
			} catch (e) {
				console.error('Failed to load from localStorage:', e)
			}
		}
	}

	const fetchSearchHistory = async () => {
		try {
			const response = await fetchWithAuth('/api/v1/hunter/history?limit=50', { cache: 'no-store' })
			if (!response.ok) {
				throw new Error(`HTTP ${response.status}`)
			}
			const data = await response.json()
			const nextHistory = Array.isArray(data?.data) ? data.data.filter((item: unknown): item is string => typeof item === 'string') : []
			setSearchHistory(nextHistory)
			return nextHistory
		} catch (error) {
			console.error('Failed to load Hunter search history:', error)
			return []
		}
	}

	const saveSearchHistory = async (searchQuery: string) => {
		const normalizedQuery = searchQuery.trim()
		if (!normalizedQuery) {
			return
		}

		setSearchHistory(prev => [normalizedQuery, ...prev.filter(item => item !== normalizedQuery)].slice(0, 50))

		try {
			await fetchWithAuth('/api/v1/hunter/history', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({ query: normalizedQuery }),
			})
		} catch (error) {
			console.error('Failed to save Hunter search history:', error)
		}
	}

	const addFavSyntax = async () => {
		try {
			const response = await fetchWithAuth(`/api/v1/syntax`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					source: 'hunter',
					name: newSyntaxName,
					content: newSyntaxContent,
				}),
			})

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`)
			}

			const data = await response.json()
			if (data.success) {
				// 重新加载列表
				await loadFavSyntax()
				setShowAddFavDialog(false)
				setNewSyntaxName('')
				setNewSyntaxContent('')
			} else {
				throw new Error(data.error || '保存失败')
			}
		} catch (error: any) {
			console.error('Failed to add favorite syntax:', error)
			toast.error('添加收藏失败: ' + (error.message || '未知错误'))
		}
	}

	const removeFavSyntax = async (name: string) => {
		try {
			const response = await fetchWithAuth(`/api/v1/syntax/by-name`, {
				method: 'DELETE',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					source: 'hunter',
					name: name,
				}),
			})

			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`)
			}

			const data = await response.json()
			if (data.success) {
				// 重新加载列表
				await loadFavSyntax()
			} else {
				throw new Error(data.error || '删除失败')
			}
		} catch (error: any) {
			console.error('Failed to remove favorite syntax:', error)
			toast.error('删除收藏失败: ' + (error.message || '未知错误'))
		}
	}

	const fetchUserInfo = async (showDialog = false, silent = false) => {
		if (!silent) {
			setIsLoadingUserInfo(true)
		}
		try {
			const response = await fetchWithAuth(`/api/v1/hunter/user`, {
				method: 'GET',
				headers: {
					'Content-Type': 'application/json',
				},
			})

			if (!response.ok) {
				const errorData = await response.json().catch(() => ({ error: '未知错误' }))
				if (showDialog) {
					throw new Error(errorData.error || `HTTP ${response.status}`)
				}
				return
			}

			const data: HunterUserInfo = await response.json()
			if (data.code !== 200) {
				if (showDialog) {
					throw new Error(data.message || '获取用户信息失败')
				}
				return
			}

			setUserInfo(data)
		} catch (error: any) {
			if (showDialog) {
				toast.error('获取用户信息失败: ' + (error.message || '未知错误'))
			}
			console.error('Failed to fetch user info:', error)
		} finally {
			if (!silent) {
				setIsLoadingUserInfo(false)
			}
		}
	}

	const fetchSearchResults = async (
		searchQuery: string,
		pageNum: number,
		pageSize: number,
		filters: TabFilters
	) => {
		let response: Response
		try {
			response = await fetchWithAuth(`/api/v1/hunter/search`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: searchQuery,
					page: pageNum,
					pageSize,
					timeFilter: filters.timeFilter,
					isWeb: filters.isWeb,
					deduplication: filters.deduplication,
				}),
			})
		} catch (error) {
			throw new Error('无法连接后端服务，请检查地址是否正确')
		}

		let data: any = null
		try {
			data = await response.json()
		} catch (error) {
			// ignore convert error
		}

		if (!response.ok) {
			throw new Error(data?.error || data?.message || '请求失败')
		}

		// Hunter API 错误判断逻辑（参考 Vue 实现）
		// code == 0: 请求失败
		// code == 40205: 需要提示的错误
		// code != 200: 显示提示但继续处理（可能是警告）
		// code == 200: 成功
		if (data?.code === 0) {
			throw new Error('请求失败，请检查网络')
		}

		if (data?.code === 40205) {
			throw new Error(data?.message || '请求失败')
		}

		// code != 200 时显示提示，但不抛出错误（继续处理数据）
		if (data?.code && data.code !== 200 && data.code !== 0 && data.code !== 40205) {
			console.warn('Hunter API 警告:', data?.message)
		}

		return {
			results: transformHunterData(data.data?.arr || []),
			total: data.data?.total || 0,
			consumeQuota: data.data?.consume_quota || '',
			restQuota: data.data?.rest_quota || '',
			message: data.message || 'success',
		}
	}

	const updateTabState = (tabId: string, updater: (tab: SearchTab) => SearchTab) => {
		setTabs(prev => prev.map(tab => (tab.id === tabId ? updater(tab) : tab)))
	}

	const refreshTab = async (tabId: string, pageNum: number) => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab) return

		updateTabState(tabId, current => ({
			...current,
			isLoading: true,
			error: undefined,
		}))

		try {
			const data = await fetchSearchResults(tab.query, pageNum, tab.pageSize, tab.filters)
			updateTabState(tabId, current => ({
				...current,
				results: data.results || [],
				total: data.total || 0,
				consumeQuota: data.consumeQuota,
				restQuota: data.restQuota,
				message: getResultMessage(data.message, data.total, data.restQuota),
				currentPage: pageNum,
				isLoading: false,
				error: undefined,
				lastUpdated: Date.now(),
			}))
			void fetchUserInfo(false, true)
		} catch (error: any) {
			updateTabState(tabId, current => ({
				...current,
				isLoading: false,
				error: error?.message || '加载失败',
			}))
			throw error
		}
	}

	const onTabFiltersChange = async (tabId: string, nextFilters: Pick<TabFilters, 'timeFilter' | 'isWeb'>) => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab || tab.isLoading) return

		const mergedFilters: TabFilters = {
			...tab.filters,
			timeFilter: nextFilters.timeFilter,
			isWeb: nextFilters.isWeb,
			deduplication: false,
		}

		updateTabState(tabId, current => ({
			...current,
			filters: mergedFilters,
			currentPage: 1,
			isLoading: true,
			error: undefined,
		}))

		try {
			const data = await fetchSearchResults(tab.query, 1, tab.pageSize, mergedFilters)
			updateTabState(tabId, current => ({
				...current,
				results: data.results || [],
				total: data.total || 0,
				consumeQuota: data.consumeQuota,
				restQuota: data.restQuota,
				message: getResultMessage(data.message, data.total, data.restQuota),
				currentPage: 1,
				isLoading: false,
				error: undefined,
				lastUpdated: Date.now(),
			}))
			void fetchUserInfo(false, true)
		} catch (error: any) {
			updateTabState(tabId, current => ({
				...current,
				isLoading: false,
				error: error?.message || '加载失败',
			}))
			console.error('Filter change error:', error)
			toast.error('筛选失败: ' + (error?.message || '未知错误'))
		}
	}

	const handlePageSizeChange = async (tabId: string, newPageSize: number) => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab || tab.isLoading) return

		updateTabState(tabId, current => ({
			...current,
			pageSize: newPageSize,
			currentPage: 1,
			isLoading: true,
			error: undefined,
		}))

		try {
			const data = await fetchSearchResults(tab.query, 1, newPageSize, tab.filters)
			updateTabState(tabId, current => ({
				...current,
				results: data.results || [],
				total: data.total || 0,
				consumeQuota: data.consumeQuota,
				restQuota: data.restQuota,
				message: getResultMessage(data.message, data.total, data.restQuota),
				currentPage: 1,
				pageSize: newPageSize,
				isLoading: false,
				error: undefined,
				lastUpdated: Date.now(),
			}))
			void fetchUserInfo(false, true)
		} catch (error: any) {
			updateTabState(tabId, current => ({
				...current,
				isLoading: false,
				error: error?.message || '加载失败',
			}))
			console.error('Page size change error:', error)
			toast.error('加载失败: ' + (error?.message || '未知错误'))
		}
	}

	// 定义表格列配置（使用 useMemo 以便访问 currentTab）
	const columns: ColumnDef<HunterResult>[] = useMemo(() => {
		const currentTab = tabs.find(t => t.id === activeTab)
		return [
			{
				key: 'URL',
				label: 'URL',
				visible: true,
				filterable: true,
				headerClassName: 'text-left w-[420px] max-w-[420px]',
				cellClassName: 'w-[420px] max-w-[420px]',
				render: (value, row) => {
					if (!row.URL) {
						return <span className="text-xs text-muted-foreground">-</span>
					}
					return (
						<a
							href={row.URL}
							target="_blank"
							rel="noopener noreferrer"
							title={row.URL}
							className="text-primary hover:underline flex items-center gap-1 max-w-full"
						>
							<span className="truncate block max-w-[380px]">{row.URL}</span>
							<ExternalLink className="w-3 h-3 shrink-0" />
						</a>
					)
				},
			},
			{
				key: 'IP',
				label: 'IP',
				visible: true,
				filterable: true,
				headerClassName: 'w-[170px] text-left',
				cellClassName: 'w-[170px] font-mono text-sm',
			},
			{
				key: 'Port',
				label: '端口',
				visible: true,
				filterable: true,
				headerClassName: 'w-[90px] text-left',
				cellClassName: 'w-[90px]',
			},
			{
				key: 'Protocol',
				label: '协议',
				visible: true,
				filterable: true,
				headerClassName: 'w-[110px] text-left',
				cellClassName: 'w-[110px]',
				render: (value) => (
					<Badge variant="secondary" className="bg-primary/10 text-primary hover:bg-primary/20">
						{value}
					</Badge>
				),
			},
			{
				key: 'Domain',
				label: '域名',
				visible: true,
				filterable: true,
				headerClassName: 'min-w-[200px] text-left',
				cellClassName: 'min-w-[200px] max-w-xs truncate',
			},
			{
				key: 'Title',
				label: '标题',
				visible: true,
				filterable: true,
				headerClassName: 'min-w-[180px] text-left',
				cellClassName: 'min-w-[180px] max-w-xs truncate',
			},
			{
				key: 'Component',
				label: '组件',
				visible: true,
				filterable: true,
				headerClassName: 'min-w-[140px] text-left',
				cellClassName: 'min-w-[140px]',
				render: (value) => {
					const products = formatProduct(value)
					if (products.length === 0) {
						return <span className="text-xs text-muted-foreground">-</span>
					}
					return (
						<div className="flex items-center gap-1">
							{products.length === 1 ? (
								<Badge variant="secondary" className="text-xs">
									{products[0]}
								</Badge>
							) : (
								<HoverCard>
									<HoverCardTrigger asChild>
										<div className="cursor-pointer inline-block">
											<Badge variant="secondary" className="text-xs">
												{products[0]}
												<span className="ml-1 text-xs opacity-70">
													+{products.length - 1}
												</span>
											</Badge>
										</div>
									</HoverCardTrigger>
									<HoverCardContent className="w-80">
										<div className="space-y-2">
											<div className="font-semibold text-sm mb-2">
												全部组件 ({products.length}个)
											</div>
											<div className="flex flex-wrap gap-2">
												{products.map((product, idx) => (
													<Badge key={idx} variant="secondary" className="text-xs">
														{product}
													</Badge>
												))}
											</div>
										</div>
									</HoverCardContent>
								</HoverCard>
							)}
						</div>
					)
				},
			},
			{
				key: 'Status',
				label: '状态码',
				visible: true,
				filterable: true,
				headerClassName: 'w-[90px] text-left',
				cellClassName: 'w-[90px]',
			},
			{
				key: 'ICP',
				label: '备案号',
				visible: true,
				filterable: true,
				headerClassName: 'min-w-[150px] text-left',
				cellClassName: 'min-w-[150px] max-w-xs truncate',
			},
			{
				key: 'ISP',
				label: '运营商',
				visible: true,
				filterable: true,
				headerClassName: 'min-w-[120px] text-left',
				cellClassName: 'min-w-[120px] max-w-xs truncate',
			},
			{
				key: 'Position',
				label: '地理位置',
				visible: true,
				filterable: true,
				headerClassName: 'min-w-[150px] text-left',
				cellClassName: 'min-w-[150px] max-w-xs truncate',
			},
			{
				key: 'UpdateTime',
				label: '更新时间',
				visible: true,
				filterable: true,
				headerClassName: 'w-[130px] text-left',
				cellClassName: 'w-[130px] text-xs',
			},
		]
	}, [tabs, activeTab])

	const startSearch = async (searchQuery?: string) => {
		const queryToUse = searchQuery || query
		if (!queryToUse.trim()) {
			toast.error('请输入查询语法')
			return
		}

		setIsSearching(true)
		const currentTab = tabs.find(t => t.id === activeTab)
		const filters: TabFilters = currentTab?.filters || DEFAULT_TAB_FILTERS
		const trimmedQuery = queryToUse.trim()

		try {
			const data = await fetchSearchResults(trimmedQuery, 1, DEFAULT_PAGE_SIZE, filters)

			const newTab: SearchTab = {
				id: Date.now().toString(),
				query: trimmedQuery,
				results: data.results || [],
				total: data.total || 0,
				pageSize: DEFAULT_PAGE_SIZE,
				currentPage: 1,
				consumeQuota: data.consumeQuota,
				restQuota: data.restQuota,
				message: getResultMessage(data.message, data.total, data.restQuota),
				filters,
				tableFilters: {},
				isLoading: false,
				error: undefined,
				lastUpdated: Date.now(),
			}

			setTabs(prev => [...prev, newTab])
			setActiveTab(newTab.id)

			// 保存搜索历史记录
			void saveSearchHistory(trimmedQuery)
			void fetchUserInfo(false, true)
		} catch (error: any) {
			toast.error('搜索失败: ' + (error.message || '未知错误'))
			console.error('Search error:', error)
		} finally {
			setIsSearching(false)
		}
	}

	const closeTab = (tabId: string) => {
		const newTabs = tabs.filter(t => t.id !== tabId)
		setTabs(newTabs)
		if (activeTab === tabId && newTabs.length > 0) {
			setActiveTab(newTabs[0].id)
		} else if (newTabs.length === 0) {
			setActiveTab('')
		}
	}

	const addSyntaxToQuery = (syntaxKey: string) => {
		if (!query) {
			setQuery(syntaxKey)
		} else {
			setQuery(`${query} && ${syntaxKey}`)
		}
		setShowSuggestions(false)
		setSelectedSuggestionIndex(-1)
	}

	// 输入提示选择函数：直接替换查询内容
	const selectSuggestion = (syntaxKey: string) => {
		setQuery(syntaxKey)
		setShowSuggestions(false)
		setSelectedSuggestionIndex(-1)
	}

	// 根据输入内容获取提示：优先匹配平台内置语法，未匹配时再回退到 Hunter 搜索提示
	const getSuggestions = useCallback(async (input: string): Promise<SuggestionItem[]> => {
		const matches: SuggestionItem[] = []

		// 如果输入为空，直接显示历史搜索记录（最近10条）
		if (!input || input.trim().length === 0) {
			searchHistory.slice(0, 10).forEach(query => {
				matches.push({
					key: query,
					description: `历史搜索记录`,
					category: '历史记录',
				})
			})
			return matches
		}

		const inputLower = input.toLowerCase().trim()

		// 平台内置语法和收藏语法优先，避免引擎的 app 搜索提示覆盖字段语法提示
		for (const category of hunterSyntaxData) {
			for (const item of category.data) {
				const keyLower = item.key.toLowerCase()
				const descLower = item.description.toLowerCase()

				// 如果 key 或 description 包含输入内容
				if (keyLower.includes(inputLower) || descLower.includes(inputLower)) {
					matches.push({
						key: item.key,
						description: item.description,
						category: category.title,
					})
				}
			}
		}

		// 从收藏语法中查找匹配的
		for (const fav of favSyntax) {
			const contentLower = fav.Content.toLowerCase()
			const nameLower = fav.Name.toLowerCase()

			if (contentLower.includes(inputLower) || nameLower.includes(inputLower)) {
				matches.push({
					key: fav.Content,
					description: fav.Name,
					category: '收藏语法',
				})
			}
		}

		if (matches.length > 0) {
			return matches.slice(0, 10)
		}

		// 平台语法没有匹配时，才调用 Hunter Tips API 作为补充
		try {
			const response = await fetchWithAuth(`/api/v1/hunter/tips?keyword=${encodeURIComponent(input)}`, {
				method: 'GET',
			})

			if (response.ok) {
				const data = await response.json()
				// Hunter API 返回的数据格式：{ code: int, data: { app: [{ name, asset_num, tags }], collect: [] }, message: string }
				if (data.code === 200 && data.data) {
					// 处理 app 数组
					if (data.data.app && Array.isArray(data.data.app) && data.data.app.length > 0) {
						data.data.app.forEach((item: any) => {
							if (item.name) {
								const assetInfo = item.asset_num ? ` (${item.asset_num} 资产)` : ''
								const tagsInfo = item.tags && item.tags.length > 0 ? ` [${item.tags.join(', ')}]` : ''
								matches.push({
									key: `app.name="${item.name}"`,
									description: `应用: ${item.name}${assetInfo}${tagsInfo}`,
									category: '搜索提示',
								})
							}
						})
						if (matches.length > 0) {
							return matches.slice(0, 10)
						}
					}
				}
			}
		} catch (error) {
			console.error('Failed to fetch tips from API:', error)
			// API 调用失败，继续使用历史记录
		}

		// 如果搜索提示为空，显示历史搜索记录
		if (matches.length === 0) {
			// 过滤历史记录，只显示包含输入内容的记录
			const filteredHistory = searchHistory.filter(h => h.toLowerCase().includes(inputLower))
			filteredHistory.slice(0, 10).forEach(query => {
				matches.push({
					key: query,
					description: `历史搜索记录`,
					category: '历史记录',
				})
			})
		}

		return matches.slice(0, 10) // 最多显示10条
	}, [favSyntax, searchHistory])

	// 当输入变化时更新提示（使用 debounce）
	useEffect(() => {
		if (!inputFocused) {
			setShowSuggestions(false)
			setSuggestions([])
			return
		}

		// 300ms 防抖
		const timer = setTimeout(async () => {
			const matches = await getSuggestions(query || '')
			setSuggestions(matches)
			setShowSuggestions(matches.length > 0)
			setSelectedSuggestionIndex(-1)
		}, 300)

		return () => clearTimeout(timer)
	}, [getSuggestions, inputFocused, query])

	useEffect(() => {
		void fetchSearchHistory()
	}, [])

	const activeTabData = tabs.find(t => t.id === activeTab)

	const openExportDialog = () => {
		if (!activeTabData) return
		const totalRows = Math.max(0, Math.min(currentMaxExportCount, activeTabData.total || 0))
		const maxPage = Math.max(1, Math.ceil(totalRows / EXPORT_PAGE_SIZE))
		setExportStartPage(1)
		setExportEndPage(maxPage)
		setShowExportDialog(true)
	}

	const exportFieldOptions = useMemo(() => {
		const knownKeys = Object.keys(exportFieldLabelMap)
		const dynamicKeySet = new Set<string>(knownKeys)
		if (activeTabData?.results) {
			for (const row of activeTabData.results) {
				for (const key of Object.keys(row)) {
					dynamicKeySet.add(key)
				}
			}
		}
		return Array.from(dynamicKeySet).map(key => ({
			value: key,
			label: exportFieldLabelMap[key] || key,
		}))
	}, [activeTabData, exportFieldLabelMap])

	const runExport = async () => {
		if (!activeTabData) return
		if (exportStartPage <= 0 || exportEndPage <= 0 || exportEndPage < exportStartPage) {
			toast.error('导出页码范围不合法')
			return
		}
		if (selectedExportFields.length === 0) {
			toast.error('请至少选择一个导出字段')
			return
		}

		const maxPage = currentMaxExportPage
		if (maxPage <= 0) {
			toast.error('当前导出额度不足')
			return
		}
		const startPage = Math.max(1, Math.min(exportStartPage, maxPage))
		const endPage = Math.max(startPage, Math.min(exportEndPage, maxPage))
		const totalToExport = Math.max(0, Math.min((endPage - startPage + 1) * EXPORT_PAGE_SIZE, currentMaxExportCount))
		if (totalToExport <= 0) {
			toast.error('导出条数必须大于 0')
			return
		}

		setIsExporting(true)
		try {
			const selectedFields = selectedExportFields.filter(item => exportFieldValueSet.has(item))
			if (selectedFields.length === 0) {
				toast.error('请至少选择一个导出字段')
				return
			}

			const response = await fetchWithAuth(`/api/v1/hunter/export/tasks`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: activeTabData.query,
					exportCount: totalToExport,
					fields: selectedFields,
					format: exportFormat,
					fileName: exportFileName,
					timeFilter: activeTabData.filters.timeFilter,
					isWeb: activeTabData.filters.isWeb,
					deduplication: activeTabData.filters.deduplication,
					startPage,
					endPage,
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || '创建导出任务失败')
			}
			const data = await response.json()
			if (!data.success) {
				throw new Error(data.error || '创建导出任务失败')
			}

				toast.success('导出任务已创建，请到报告管理查看进度并下载')
			setShowExportDialog(false)
		} catch (error: any) {
			toast.error('导出失败: ' + (error?.message || '未知错误'))
		} finally {
			setIsExporting(false)
		}
	}

	const copyURLs = (dedup: boolean = false) => {
		const currentTab = tabs.find(t => t.id === activeTab)
		if (!currentTab) return

		let results = currentTab.results

		if (dedup) {
			const seen = new Set<string>()
			results = results.filter(item => {
				const key = `${item.IP}_${item.URL}`
				if (seen.has(key)) return false
				seen.add(key)
				return true
			})
		}

		const urls = results.map(r => r.URL).filter(Boolean).join('\n')
		if (!urls) {
			toast.error('没有可复制的URL')
			return
		}
		void copyToClipboard(urls).then((success) => {
			if (success) {
				toast.success('URL已复制到剪贴板')
			} else {
				toast.error('复制失败，请手动复制')
			}
		})
	}

	const handleSaveToAssetCenter = async (targets: HunterResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error(t.common.saveToAssetCenterEmpty)
			return
		}
		const toastId = toast.loading(t.common.savingToAssetCenter)
		try {
			const result = await saveSpaceEngineAssets(
				'hunter',
				targets.map(item => ({
					ip: item.IP,
					domain: item.Domain,
					url: item.URL,
					title: item.Title,
					port: item.Port,
					protocol: item.Protocol,
					product: item.Component,
					status: item.Status,
				})),
			)
			if (!result.saved) {
				toast.error(t.common.saveToAssetCenterEmpty, { id: toastId })
				return
			}
			toast.success(t.common.saveToAssetCenterSuccess.replace('{count}', String(result.saved)), { id: toastId })
		} catch (error: any) {
			toast.error(`${t.common.saveToAssetCenterFailed}: ${error?.message || t.common.error}`, { id: toastId })
		}
	}

	const handleSendToWebscan = async (targets: HunterResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error('请先选择目标资产')
			return
		}

		const urls = Array.from(
			new Set(
				targets
					.map((item) => {
						const rawUrl = String(item.URL || '').trim()
						if (rawUrl) {
							if (/^https?:\/\//i.test(rawUrl)) return rawUrl
							const protocol = item.Protocol?.toLowerCase() === 'https' || Number(item.Port) === 443 ? 'https' : 'http'
							return `${protocol}://${rawUrl}`
						}
						const host = String(item.Domain || item.IP || '').trim()
						if (!host) return ''
						const protocol = item.Protocol?.toLowerCase() === 'https' || Number(item.Port) === 443 ? 'https' : 'http'
						const port = item.Port ? `:${item.Port}` : ''
						return `${protocol}://${host}${port}`
					})
					.filter(Boolean)
			)
		) as string[]

		if (urls.length === 0) {
			toast.error('无可用 URL 目标')
			return
		}

		const taskName = `hunter-webscan-${new Date().toLocaleString('zh-CN')}`
		try {
			const requestBody = {
				scanType: 'webscan',
				taskName,
				config: {
					targets: urls,
					threads: 30,
					webscanOption: 3,
					rootPathScan: true,
					skipNucleiWithoutTags: false,
					skipNucleiWithWAF: true,
					screenshot: true,
					activeTimeoutLimit: 5,
				},
			}

			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(requestBody),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData?.error || '创建任务失败')
			}
			const data = await response.json()
			if (!data?.success) {
				throw new Error(data?.error || '创建任务失败')
			}
			toast.success(`已创建漏洞扫描任务（${urls.length} 个目标）`)
		} catch (error: any) {
			toast.error(`创建漏洞扫描任务失败: ${error?.message || '未知错误'}`)
		}
	}

	const handleSendToCrack = async (targets: HunterResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error('请先选择目标资产')
			return
		}

		const addresses = Array.from(
			new Set(
				targets
					.map((item) => {
						const host = String(item.IP || item.Domain || '').trim()
						const port = Number(item.Port || 0)
						if (!host || !port) return ''
						const protocol = String(item.Protocol || 'tcp').toLowerCase()
						const normalizedProtocol = protocol && protocol !== 'unknown' ? protocol : 'tcp'
						return `${normalizedProtocol}://${host}:${port}`
					})
					.filter(Boolean)
			)
		) as string[]

		if (addresses.length === 0) {
			toast.error('无可用地址目标')
			return
		}

		const taskName = `hunter-crack-${new Date().toLocaleString('zh-CN')}`
		try {
			const requestBody = {
				scanType: 'crack',
				taskName,
				config: {
					targets: addresses,
					threads: 10,
					username: '',
					password: '',
					defaultDict: true,
				},
			}

			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(requestBody),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData?.error || '创建任务失败')
			}
			const data = await response.json()
			if (!data?.success) {
				throw new Error(data?.error || '创建任务失败')
			}
			toast.success(`已创建密码喷洒任务（${addresses.length} 个目标）`)
		} catch (error: any) {
			toast.error(`创建密码喷洒任务失败: ${error?.message || '未知错误'}`)
		}
	}

	const buildPortscanTarget = (item: HunterResult): string | null => {
		const rawHost = String(item.IP || item.Domain || '').trim().replace(/^https?:\/\//i, '')
		if (!rawHost) return null
		return rawHost.split(':')[0] || rawHost
	}

	const handleSendToPortscan = async (targets: HunterResult[], portGroup: PortGroupOption) => {
		if (!targets || targets.length === 0) {
			toast.error('请先选择目标资产')
			return
		}

		const portscanTargets = Array.from(new Set(targets.map(buildPortscanTarget).filter((item): item is string => !!item)))
		if (portscanTargets.length === 0) {
			toast.error('没有可用于端口扫描的有效目标')
			return
		}

		const taskName = `${portscanTargets[0]}-${portGroup.text}`
		try {
			const response = await fetchWithAuth('/api/v1/webscan/scan', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'portscan',
					taskName,
					config: {
						targets: portscanTargets,
						threads: 1000,
						portGroup: portGroup.text,
						portRange: portGroup.value,
						portTimeout: 10,
						aliveDetection: true,
					},
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData?.error || '创建任务失败')
			}
			const data = await response.json()
			if (!data?.success) {
				throw new Error(data?.error || '创建任务失败')
			}
			toast.success(`已创建端口扫描任务 ${portGroup.text}（${portscanTargets.length} 个目标）`)
		} catch (error: any) {
			toast.error(`创建端口扫描任务失败: ${error?.message || '未知错误'}`)
		}
	}

	// C段查询功能
	const handleCSegmentQuery = async (ip: string) => {
		// 提取IP的C段（前三个段）
		const ipParts = ip.split('.')
		if (ipParts.length !== 4) {
			toast.error('无效的IP地址')
			return
		}

		// 构建C段查询语法：ip="xxx.xxx.xxx.0/24"
		const cSegment = `${ipParts[0]}.${ipParts[1]}.${ipParts[2]}.0/24`
		const cSegmentQuery = `ip="${cSegment}"`

		setIsSearching(true)
		const currentTab = tabs.find(t => t.id === activeTab)
		const filters: TabFilters = currentTab?.filters || DEFAULT_TAB_FILTERS

		try {
			const data = await fetchSearchResults(cSegmentQuery, 1, DEFAULT_PAGE_SIZE, filters)

			const newTab: SearchTab = {
				id: Date.now().toString(),
				query: cSegmentQuery,
				results: data.results || [],
				total: data.total || 0,
				pageSize: DEFAULT_PAGE_SIZE,
				currentPage: 1,
				consumeQuota: data.consumeQuota,
				restQuota: data.restQuota,
				message: getResultMessage(data.message, data.total, data.restQuota),
				filters,
				tableFilters: {},
				isLoading: false,
				error: undefined,
				lastUpdated: Date.now(),
			}

			setTabs(prev => [...prev, newTab])
			setActiveTab(newTab.id)
		} catch (error: any) {
			toast.error('C段查询失败: ' + (error.message || '未知错误'))
			console.error('C segment query error:', error)
		} finally {
			setIsSearching(false)
		}
	}

	const saveApiConfig = async () => {
		try {
			const { saveSpaceEngineConfig } = await import('@/lib/config')
			const spaceConfig = await getSpaceEngineConfig()
			await saveSpaceEngineConfig({
				...spaceConfig,
				hunter: {
					api: apiConfig.api,
					key: apiConfig.key,
				},
			})
			setShowApiConfigDialog(false)
			toast.success('配置保存成功')
			// 如果配置了 API Key，自动获取用户信息
			if (apiConfig.key) {
				fetchUserInfo(false)
			}
		} catch (error: any) {
			toast.error('保存配置失败: ' + (error.message || '未知错误'))
			console.error('Failed to save API config:', error)
		}
	}

	const normalizeQuota = (value?: number) => (value === undefined || value < 0 ? Number.POSITIVE_INFINITY : value)
	const currentAvailablePoints = useMemo(() => {
		const equity = normalizeQuota(userInfo?.data.rest_equity_point)
		const free = normalizeQuota(userInfo?.data.rest_free_point)
		if (!Number.isFinite(equity) && !Number.isFinite(free)) {
			return Number.POSITIVE_INFINITY
		}
		const finiteEquity = Number.isFinite(equity) ? equity : 0
		const finiteFree = Number.isFinite(free) ? free : 0
		return Math.max(0, Math.floor(finiteEquity + finiteFree))
	}, [userInfo])

	const currentMaxExportCount = useMemo(() => {
		if (!activeTabData) return 0
		const total = activeTabData.total || 0
		const onceQuota = normalizeQuota(userInfo?.data.once_export_quota)
		const restQuota = normalizeQuota(userInfo?.data.rest_export_quota)
		return Math.max(0, Math.min(total, onceQuota, restQuota, currentAvailablePoints))
	}, [activeTabData, userInfo, currentAvailablePoints])

	const currentMaxExportPage = useMemo(() => {
		if (currentMaxExportCount <= 0) return 0
		return Math.ceil(currentMaxExportCount / EXPORT_PAGE_SIZE)
	}, [currentMaxExportCount])

	const currentSelectedExportCount = useMemo(() => {
		if (currentMaxExportPage <= 0) return 0
		const start = Math.max(1, Math.min(exportStartPage, currentMaxExportPage))
		const end = Math.max(start, Math.min(exportEndPage, currentMaxExportPage))
		const beginIndex = (start - 1) * EXPORT_PAGE_SIZE + 1
		const endIndex = Math.min(end * EXPORT_PAGE_SIZE, currentMaxExportCount)
		return Math.max(0, endIndex - beginIndex + 1)
	}, [exportStartPage, exportEndPage, currentMaxExportPage, currentMaxExportCount])

	const expectedRemainingPoints = useMemo(() => {
		if (!Number.isFinite(currentAvailablePoints)) return '无限制'
		return Math.max(0, currentAvailablePoints - Math.max(0, currentSelectedExportCount || 0)).toLocaleString()
	}, [currentAvailablePoints, currentSelectedExportCount])

	const exportFieldValueSet = useMemo(() => new Set(exportFieldOptions.map(item => item.value)), [exportFieldOptions])

	useEffect(() => {
		setSelectedExportFields(prev => {
			const normalized = prev.filter(field => exportFieldValueSet.has(field))
			if (normalized.length === 0) return defaultExportFields
			if (normalized.length !== prev.length) return normalized
			return prev
		})
	}, [exportFieldValueSet])

	useEffect(() => {
		if (!showExportDialog) return
		if (currentMaxExportCount <= 0) {
			setExportStartPage(1)
			setExportEndPage(1)
			return
		}
		if (exportStartPage <= 0 || exportStartPage > currentMaxExportPage) {
			setExportStartPage(1)
		}
		if (exportEndPage <= 0 || exportEndPage > currentMaxExportPage) {
			setExportEndPage(currentMaxExportPage)
		}
	}, [showExportDialog, currentMaxExportCount, currentMaxExportPage, exportStartPage, exportEndPage])

	return (
		<div className="h-full">

			<div className="min-w-0 p-3 space-y-3 md:p-6 md:space-y-6">
				<SearchFormCard
					query={query}
					setQuery={setQuery}
					isSearching={isSearching}
					onStartSearch={() => startSearch()}
					userInfo={userInfo}
					isLoadingUserInfo={isLoadingUserInfo}
					onFetchUserInfo={fetchUserInfo}
					onOpenApiConfigDialog={() => setShowApiConfigDialog(true)}
					onOpenTipsDialog={() => setShowTipsDialog(true)}
					onOpenFavSyntaxDialog={() => setShowFavSyntaxDialog(true)}
					onOpenAddFavDialog={() => {
						setShowAddFavDialog(true)
						setNewSyntaxContent(query)
					}}
					showSuggestions={showSuggestions}
					setShowSuggestions={setShowSuggestions}
					suggestions={suggestions}
					inputFocused={inputFocused}
					setInputFocused={setInputFocused}
					selectedSuggestionIndex={selectedSuggestionIndex}
					setSelectedSuggestionIndex={setSelectedSuggestionIndex}
					onSelectSuggestion={selectSuggestion}
					suggestionsRef={suggestionsRef}
					textareaRef={textareaRef}
				/>

				<ResultsCard
					tabs={tabs}
					activeTab={activeTab}
					setActiveTab={setActiveTab}
					currentTab={activeTabData}
					columns={columns}
					isSearching={isSearching}
					onCloseTab={closeTab}
					onOpenExportDialog={openExportDialog}
					onCopyUrls={copyURLs}
					onRefreshCurrentTab={async () => {
						if (!activeTabData) return
						await refreshTab(activeTabData.id, activeTabData.currentPage)
					}}
					onRefreshTab={refreshTab}
					onHandlePageSizeChange={handlePageSizeChange}
					onHandleCSegmentQuery={handleCSegmentQuery}
					onSaveToAssetCenter={handleSaveToAssetCenter}
					onSendToWebscan={handleSendToWebscan}
					onSendToCrack={handleSendToCrack}
					onSendToPortscan={handleSendToPortscan}
					portScanGroups={portScanGroups}
					onTableFiltersChange={setTableFilters}
					onTabFiltersChange={onTabFiltersChange}
				/>
			</div>

			{/* 语法提示对话框 */}
			<Dialog open={showTipsDialog} onOpenChange={setShowTipsDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-[calc(100%-2rem)] sm:max-w-[800px] max-h-[80vh]">
					<DialogHeader>
						<DialogTitle>{t.hunterPage.page.syntaxTitle}</DialogTitle>
						<DialogDescription>{t.hunterPage.page.syntaxDescription}</DialogDescription>
					</DialogHeader>
					<Tabs value={syntaxKeyword} onValueChange={setSyntaxKeyword} className="w-full" orientation={isMobile ? "horizontal" : "vertical"}>
						<div className="flex min-w-0 flex-col gap-3 h-[60vh] md:flex-row md:gap-4">
							{/* 左侧分类列表 */}
							<TabsList className="h-auto w-full shrink-0 justify-start overflow-x-auto bg-muted/50 p-2 md:h-full md:w-[200px] md:flex-col">
								{hunterSyntaxData.map((item) => (
									<TabsTrigger
										key={item.title}
										value={item.title}
										className="shrink-0 justify-start whitespace-nowrap data-[state=active]:bg-background md:w-full"
									>
										{item.title}
									</TabsTrigger>
								))}
							</TabsList>

							{/* 右侧内容区域 */}
							<div className="min-h-0 min-w-0 flex-1 overflow-hidden">
								{hunterSyntaxData.map((category) => (
									<TabsContent key={category.title} value={category.title} className="mt-0 h-full overflow-y-auto pr-2">
										<div className="space-y-2">
											{category.data.map((item, idx) => (
												<div
													key={idx}
													onClick={() => {
														addSyntaxToQuery(item.key)
														setShowTipsDialog(false)
													}}
													className="flex items-start justify-between p-3 hover:bg-muted rounded-lg cursor-pointer border border-border/50 hover:border-primary/50 transition-colors group"
												>
													<div className="flex-1 min-w-0">
														<div className="flex items-center gap-2 mb-1">
															<code className="text-primary font-mono text-sm break-all">
																{item.key}
															</code>
															{item.hot && (
																<Badge variant="secondary" className="text-xs shrink-0 bg-primary/10 text-primary border-primary/20">
																	{t.hunterPage.page.hot}
																</Badge>
															)}
															{item.characteristic && (
																<Badge variant="secondary" className="text-xs shrink-0 bg-accent/10 text-accent border-accent/20">
																	{t.hunterPage.page.featured}
																</Badge>
															)}
														</div>
														<p className="text-xs text-muted-foreground">
															{item.description}
														</p>
													</div>
												</div>
											))}
										</div>
									</TabsContent>
								))}
							</div>
						</div>
					</Tabs>
				</DialogContent>
			</Dialog>

			{/* 收藏语法对话框 */}
			<Dialog open={showFavSyntaxDialog} onOpenChange={setShowFavSyntaxDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-3xl max-h-[70vh]">
					<DialogHeader>
						<DialogTitle>{t.hunterPage.page.favTitle}</DialogTitle>
						<DialogDescription>{t.hunterPage.page.favDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-2 max-h-[50vh] overflow-y-auto pr-2">
						{favSyntax.length === 0 ? (
							<div className="text-center py-12 text-sm text-muted-foreground">
								<Star className="w-12 h-12 mx-auto mb-4 opacity-30" />
								<p>{t.hunterPage.page.noFav}</p>
								<p className="text-xs mt-2">{t.hunterPage.page.favHint}</p>
							</div>
						) : (
							favSyntax.map((item, idx) => (
								<div
									key={idx}
									className="flex items-center justify-between p-3 hover:bg-muted rounded-lg border border-border/50 hover:border-primary/50 transition-colors group"
								>
									<div className="flex-1 min-w-0">
										<div className="flex items-center gap-2 mb-1">
											<code
												className="text-primary cursor-pointer font-mono text-sm break-all"
												onClick={() => {
													addSyntaxToQuery(item.Content)
													setShowFavSyntaxDialog(false)
												}}
											>
												{item.Content}
											</code>
										</div>
										<p className="text-xs text-muted-foreground">{item.Name}</p>
									</div>
									<div className="flex items-center gap-2 ml-4 shrink-0">
										<Button
											variant="ghost"
											size="sm"
											onClick={() => {
												addSyntaxToQuery(item.Content)
												setShowFavSyntaxDialog(false)
											}}
										>
											<Copy className="w-4 h-4 mr-1" />
											{t.hunterPage.page.use}
										</Button>
										<Button
											variant="ghost"
											size="sm"
											className="h-8 w-8 p-0"
											onClick={() => removeFavSyntax(item.Name)}
										>
											<X className="w-4 h-4" />
										</Button>
									</div>
								</div>
							))
						)}
					</div>
				</DialogContent>
			</Dialog>

			{/* 添加收藏语法对话框 */}
			<Dialog open={showAddFavDialog} onOpenChange={setShowAddFavDialog}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t.hunterPage.page.addFavTitle}</DialogTitle>
						<DialogDescription>{t.hunterPage.page.addFavDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="syntax-name">{t.hunterPage.page.name}</Label>
							<Input
								id="syntax-name"
								value={newSyntaxName}
								onChange={e => setNewSyntaxName(e.target.value)}
								placeholder={t.hunterPage.page.exampleWordPress}
								className="mt-2"
							/>
						</div>
						<div>
							<Label htmlFor="syntax-content">{t.hunterPage.page.syntaxContent}</Label>
							<Textarea
								id="syntax-content"
								value={newSyntaxContent}
								onChange={e => setNewSyntaxContent(e.target.value)}
								placeholder='ip="1.1.1.1"'
								className="mt-2 font-mono min-h-[80px]"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowAddFavDialog(false)}>
							{t.hunterPage.page.cancel}
						</Button>
						<Button onClick={addFavSyntax} disabled={!newSyntaxName || !newSyntaxContent}>
							{t.hunterPage.page.save}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* API 配置对话框 */}
			<Dialog open={showApiConfigDialog} onOpenChange={setShowApiConfigDialog}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t.hunterPage.page.apiTitle}</DialogTitle>
						<DialogDescription>{t.hunterPage.page.apiDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="hunter-api">{t.hunterPage.page.apiAddress}</Label>
							<Input
								id="hunter-api"
								value={apiConfig.api}
								onChange={e => setApiConfig({ ...apiConfig, api: e.target.value })}
								placeholder="https://hunter.qianxin.com"
								className="mt-2"
							/>
						</div>
						<div>
							<Label htmlFor="hunter-key">Hunter Key</Label>
							<PasswordInput
								id="hunter-key"
								value={apiConfig.key}
								onChange={e => setApiConfig({ ...apiConfig, key: e.target.value })}
								placeholder="your-api-key"
								className="mt-2"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowApiConfigDialog(false)}>
							{t.hunterPage.page.cancel}
						</Button>
						<Button onClick={saveApiConfig}>
							{t.hunterPage.page.save}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 导出对话框 */}
			<Dialog open={showExportDialog} onOpenChange={setShowExportDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-2xl max-h-[calc(100vh-4rem)] overflow-y-auto">
					<DialogHeader>
						<DialogTitle>{t.hunterPage.page.exportTitle}</DialogTitle>
						<DialogDescription>
							{formatText(t.hunterPage.page.exportDescription, { pageSize: EXPORT_PAGE_SIZE })}
						</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="export-file-name">{t.hunterPage.page.fileName}</Label>
							<Input
								id="export-file-name"
								value={exportFileName}
								onChange={e => setExportFileName(e.target.value)}
								placeholder="assets_202626"
								className="mt-2"
							/>
						</div>
						<div>
							<Label>{t.hunterPage.page.fileFormat}</Label>
							<Select value={exportFormat} onValueChange={setExportFormat}>
								<SelectTrigger className="mt-2">
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="csv">CSV</SelectItem>
								</SelectContent>
							</Select>
						</div>
						<div>
							<Label>{t.hunterPage.page.exportPageRange}</Label>
							<div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-2">
								<div>
									<Label className="text-xs text-muted-foreground">{t.hunterPage.page.startPage}</Label>
									<Input
										type="number"
										min={1}
										max={Math.max(1, currentMaxExportPage)}
										value={exportStartPage}
										onChange={e => {
											const next = Number(e.target.value || 1)
											if (!Number.isFinite(next)) return
											setExportStartPage(Math.min(Math.max(1, Math.floor(next)), Math.max(1, currentMaxExportPage)))
										}}
										disabled={isExporting}
									/>
								</div>
								<div>
									<Label className="text-xs text-muted-foreground">{t.hunterPage.page.endPage}</Label>
									<Input
										type="number"
										min={1}
										max={Math.max(1, currentMaxExportPage)}
										value={exportEndPage}
										onChange={e => {
											const next = Number(e.target.value || 1)
											if (!Number.isFinite(next)) return
											setExportEndPage(Math.min(Math.max(1, Math.floor(next)), Math.max(1, currentMaxExportPage)))
										}}
										disabled={isExporting}
									/>
								</div>
							</div>
							<div className="mt-2 text-xs text-muted-foreground">
								{formatText(t.hunterPage.page.maxExportPage, {
									page: currentMaxExportPage,
									count: currentSelectedExportCount.toLocaleString(),
								})}
							</div>
						</div>
						<div>
							<Label>{t.hunterPage.page.exportFields}</Label>
							<div className="mt-2">
								<SelectTag
									options={exportFieldOptions}
									value={selectedExportFields}
									onChange={next => {
										const normalized = next.filter(item => exportFieldValueSet.has(item))
										if (normalized.length === 0) return
										setSelectedExportFields(normalized)
									}}
									placeholder={t.hunterPage.page.selectExportFields}
									disabled={isExporting}
								/>
							</div>
						</div>
						<div className="text-xs text-muted-foreground">
							{formatText(t.hunterPage.page.remainingPoints, { value: expectedRemainingPoints })}
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowExportDialog(false)} disabled={isExporting}>
							{t.hunterPage.page.cancel}
						</Button>
						<Button onClick={runExport} disabled={isExporting || currentMaxExportCount <= 0 || currentSelectedExportCount <= 0}>
							{isExporting ? t.hunterPage.page.exporting : t.hunterPage.page.export}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

		</div>
	)
}

export default function HunterPage() {
	return (
		<ClientKeepAlive name="hunter-page">
			<HunterPageContent />
		</ClientKeepAlive>
	)
}
