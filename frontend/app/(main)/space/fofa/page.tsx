'use client'

import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import Image from 'next/image'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { ColumnDef } from '@/components/ui/data-table'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Search, Download, ExternalLink, Radar, X, Star, Copy, ImageIcon, MoreHorizontal, Settings, Hash, BookOpen, List, Network, User } from 'lucide-react'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { cn } from '@/lib/utils'
import { ClientKeepAlive } from '@/components/client-keep-alive'
import { getSpaceEngineConfig, onSpaceEngineConfigChange } from '@/lib/config'
import { fetchWithAuth, getApiBaseURL } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { saveSpaceEngineAssets } from '@/lib/space-engine-assets'
import { useSpaceEngineQueryParam } from '@/lib/space-engine'
import toast from 'react-hot-toast'
import { SearchFormCard, type FofaSuggestionItem } from './components/search-form-card'
import { ResultsCard as FofaResultsCard } from './components/results-card'
import { SelectTag } from '@/components/ui/select-tag'
import { builtInPortGroupOptions, mergePortGroupOptions, type PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'
import { useI18n } from '@/lib/i18n'

interface FofaResult {
	URL: string
	Host: string
	Title: string
	IP: string
	Port: string
	Domain: string
	Protocol: string
	Country: string
	ICP: string
	Product: string
}

interface FofaUserInfo {
	error: boolean
	email: string
	username: string
	category: string
	fcoin: number
	fofa_point: number
	remain_free_point: number
	remain_api_query: number
	remain_api_data: number
	isvip: boolean
	vip_level: number
	is_verified: boolean
	avatar: string
	message: string
	fofacli_ver: string
	fofa_server: boolean
}


interface TabFilters {
	fraud: boolean
	cert: boolean
}

interface SearchTab {
	id: string
	query: string
	results: FofaResult[]
	total: number
	pageSize: number
	currentPage: number
	message: string
	filters: TabFilters
	tableFilters?: Record<string, string> // 表格筛选状态
	isLoading?: boolean
	error?: string
	lastUpdated?: number
	statsData?: StatsData | null // 统计聚合数据
	isLoadingStats?: boolean // 统计聚合加载状态
}

interface FavSyntax {
	id?: number
	source?: string
	Name: string
	Content: string
}

interface StatsItem {
	name: string
	count: number
}

interface StatsData {
	[field: string]: StatsItem[]
}

interface StatsResponse {
	error: boolean
	message: string
	data: StatsData
}

const DEFAULT_PAGE_SIZE = 10
const BATCH_TARGETS_PER_TASK = 100
const BATCH_TASK_DELAY_MS = 3000
const BATCH_STATS_SKIP_CLAUSE_COUNT = 20
const batchTargetPatterns = {
	ipCandidate: /^\d+(?:\.\d+){3}(?:\/\d+)?$/,
	domain: /^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/,
	simpleOrClause: /^(ip|domain|host)="[^"]+"$/,
}

const ipv4NonPublicRanges = [
	{ name: '0.0.0.0/8', start: '0.0.0.0', end: '0.255.255.255' },
	{ name: '10.0.0.0/8', start: '10.0.0.0', end: '10.255.255.255' },
	{ name: '100.64.0.0/10', start: '100.64.0.0', end: '100.127.255.255' },
	{ name: '127.0.0.0/8', start: '127.0.0.0', end: '127.255.255.255' },
	{ name: '169.254.0.0/16', start: '169.254.0.0', end: '169.254.255.255' },
	{ name: '172.16.0.0/12', start: '172.16.0.0', end: '172.31.255.255' },
	{ name: '192.0.0.0/24', start: '192.0.0.0', end: '192.0.0.255' },
	{ name: '192.0.2.0/24', start: '192.0.2.0', end: '192.0.2.255' },
	{ name: '192.168.0.0/16', start: '192.168.0.0', end: '192.168.255.255' },
	{ name: '198.18.0.0/15', start: '198.18.0.0', end: '198.19.255.255' },
	{ name: '198.51.100.0/24', start: '198.51.100.0', end: '198.51.100.255' },
	{ name: '203.0.113.0/24', start: '203.0.113.0', end: '203.0.113.255' },
	{ name: '224.0.0.0/4', start: '224.0.0.0', end: '239.255.255.255' },
	{ name: '240.0.0.0/4', start: '240.0.0.0', end: '255.255.255.255' },
].map(range => ({
	name: range.name,
	start: parseIPv4ToNumber(range.start) ?? 0,
	end: parseIPv4ToNumber(range.end) ?? 0,
}))

interface BatchAnalysis {
	ipTargets: string[]
	domainTargets: string[]
	invalidIPTargets: Array<{ target: string; reason: string }>
	lineCount: number
}

function parseIPv4ToNumber(value: string): number | null {
	const parts = value.split('.')
	if (parts.length !== 4) return null

	let result = 0
	for (const part of parts) {
		if (!/^\d{1,3}$/.test(part)) return null
		const value = Number(part)
		if (!Number.isInteger(value) || value < 0 || value > 255) return null
		result = result * 256 + value
	}
	return result
}

function validatePublicIPv4Target(target: string): { valid: true } | { valid: false; reason: string } {
	const parts = target.split('/')
	if (parts.length > 2) {
		return { valid: false, reason: '格式无效' }
	}

	const ipNumber = parseIPv4ToNumber(parts[0])
	if (ipNumber === null) {
		return { valid: false, reason: 'IPv4地址无效' }
	}

	const prefix = parts[1] === undefined ? 32 : Number(parts[1])
	if (!Number.isInteger(prefix) || prefix < 0 || prefix > 32 || !/^\d+$/.test(parts[1] ?? '32')) {
		return { valid: false, reason: 'CIDR前缀无效' }
	}

	const blockSize = 2 ** (32 - prefix)
	const rangeStart = Math.floor(ipNumber / blockSize) * blockSize
	const rangeEnd = rangeStart + blockSize - 1
	const matchedRange = ipv4NonPublicRanges.find(range => rangeStart <= range.end && rangeEnd >= range.start)
	if (matchedRange) {
		return { valid: false, reason: `不支持内网或保留地址段 ${matchedRange.name}` }
	}

	return { valid: true }
}

function analyzeBatchTargets(input: string): BatchAnalysis {
	const lines = input.split('\n').filter(l => l.trim())
	const ipTargets: string[] = []
	const domainTargets: string[] = []
	const invalidIPTargets: Array<{ target: string; reason: string }> = []

	for (const line of lines) {
		const trimmed = line.trim()
		if (!trimmed) continue

		if (batchTargetPatterns.ipCandidate.test(trimmed)) {
			const validation = validatePublicIPv4Target(trimmed)
			if (validation.valid) {
				ipTargets.push(trimmed)
			} else {
				invalidIPTargets.push({ target: trimmed, reason: validation.reason })
			}
		} else if (batchTargetPatterns.domain.test(trimmed)) {
			domainTargets.push(trimmed)
		}
	}

	return {
		ipTargets,
		domainTargets,
		invalidIPTargets,
		lineCount: lines.length,
	}
}

const exportFieldOptions = [
	{ value: 'IP', label: 'IP地址' },
	{ value: 'Port', label: '端口' },
	{ value: 'Protocol', label: '协议名' },
	{ value: 'Title', label: '网站标题' },
	{ value: 'Host', label: 'Host' },
	{ value: 'Domain', label: '域名' },
	{ value: 'Country', label: '国家代码' },
	{ value: 'ICP', label: '备案号' },
	{ value: 'URL', label: '网站链接' },
	{ value: 'Product', label: '组件' },
] as const

const exportFieldValueSet: ReadonlySet<string> = new Set(exportFieldOptions.map(item => item.value))

const getEffectiveExportCap = (total: number, userInfo: FofaUserInfo | null): number => {
	const safeTotal = Math.max(0, total || 0)
	if (!userInfo) return safeTotal
	// 会员账号视为 API 导出无限制（以上限结果数为准）
	if (userInfo.isvip) return safeTotal
	if (userInfo.remain_api_data === -1 || userInfo.remain_api_data === undefined) return safeTotal
	return Math.max(0, Math.min(safeTotal, userInfo.remain_api_data))
}

const shouldSkipStatsForBatchQuery = (searchQuery: string): boolean => {
	const clauses = searchQuery.trim().split(/\s*\|\|\s*/).filter(Boolean)
	return clauses.length > BATCH_STATS_SKIP_CLAUSE_COUNT && clauses.every(clause => batchTargetPatterns.simpleOrClause.test(clause.trim()))
}

function FofaPageContent() {
	const { t } = useI18n()
	const formatText = (template: string, values: Record<string, string | number>) =>
		template.replace(/\{(\w+)\}/g, (_, key) => String(values[key] ?? ''))
	const [query, setQuery] = useState('')
	useSpaceEngineQueryParam(setQuery)
	const [fraud, setFraud] = useState(false)
	const [cert, setCert] = useState(false)
	const [isSearching, setIsSearching] = useState(false)
	const [tabs, setTabs] = useState<SearchTab[]>([])
	const [activeTab, setActiveTab] = useState<string>('')
	const [suggestions, setSuggestions] = useState<FofaSuggestionItem[]>([])
	const [showSuggestions, setShowSuggestions] = useState(false)
	const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(-1)
	const selectedSuggestionIndexRef = useRef(-1)
	const [inputFocused, setInputFocused] = useState(false)
	const suggestionsRef = useRef<HTMLDivElement>(null)
	const textareaRef = useRef<HTMLTextAreaElement>(null)
	const [syntaxKeyword, setSyntaxKeyword] = useState('基础类')
	const [favSyntax, setFavSyntax] = useState<FavSyntax[]>([])
	const [showAddFavDialog, setShowAddFavDialog] = useState(false)
	const [showSyntaxHelpDialog, setShowSyntaxHelpDialog] = useState(false)
	const [showFavSyntaxDialog, setShowFavSyntaxDialog] = useState(false)
	const [showIconSearchDialog, setShowIconSearchDialog] = useState(false)
	const [showApiConfigDialog, setShowApiConfigDialog] = useState(false)
	const [showBatchSearchDialog, setShowBatchSearchDialog] = useState(false)
	const [showExportDialog, setShowExportDialog] = useState(false)
	const [exportFileName, setExportFileName] = useState(`fofa_${new Date().toISOString().slice(0, 10).replace(/-/g, '')}`)
	const [exportFormat, setExportFormat] = useState('csv')
	const [exportCount, setExportCount] = useState(0)
	const localizedExportFieldOptions = useMemo(() => [
		{ value: 'IP', label: 'IP' },
		{ value: 'Port', label: t.hunterPage.exportFields.Port },
		{ value: 'Protocol', label: t.hunterPage.exportFields.Protocol },
		{ value: 'Title', label: t.hunterPage.exportFields.Title },
		{ value: 'Host', label: 'Host' },
		{ value: 'Domain', label: t.hunterPage.exportFields.Domain },
		{ value: 'Country', label: 'Country' },
		{ value: 'ICP', label: t.hunterPage.exportFields.ICP },
		{ value: 'URL', label: 'URL' },
		{ value: 'Product', label: t.hunterPage.exportFields.Component },
	], [t])
	const [selectedExportFields, setSelectedExportFields] = useState<string[]>(exportFieldOptions.map(item => item.value))
	const [isExporting, setIsExporting] = useState(false)
	const [batchInput, setBatchInput] = useState('')
	const [userInfo, setUserInfo] = useState<FofaUserInfo | null>(null)
	const [isLoadingUserInfo, setIsLoadingUserInfo] = useState(false)
	const [portScanGroups, setPortScanGroups] = useState<PortGroupOption[]>(builtInPortGroupOptions)
	const [isCalculatingHash, setIsCalculatingHash] = useState(false)
	const [newSyntaxName, setNewSyntaxName] = useState('')
	const [newSyntaxContent, setNewSyntaxContent] = useState('')
	const [iconUrl, setIconUrl] = useState('')
	const [uploadedImage, setUploadedImage] = useState<File | null>(null)
	const [imagePreview, setImagePreview] = useState<string | null>(null)
	const [statsFields, setStatsFields] = useState<string>('protocol,server,domain,port,title')
	const [searchHistory, setSearchHistory] = useState<string[]>([])

	// 更新当前 tab 的表格筛选状态
	const setTableFilters = (filters: Record<string, string>) => {
		if (!activeTab) return
		updateTabState(activeTab, tab => ({
			...tab,
			tableFilters: filters,
		}))
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

	// API 配置从后端加载
	const [apiConfig, setApiConfig] = useState({
		email: '',
		key: '',
		address: 'https://fofa.info',
	})

	// 加载配置
	const loadApiConfig = async () => {
		try {
			const spaceConfig = await getSpaceEngineConfig()
			setApiConfig({
				email: spaceConfig?.fofa?.email || '',
				key: spaceConfig?.fofa?.key || '',
				address: spaceConfig?.fofa?.api || 'https://fofa.info',
			})
		} catch (error) {
			console.error('Failed to load API config:', error)
			// 如果后端不可用，尝试从localStorage加载（兼容旧数据）
			const savedConfig = localStorage.getItem('fofa_config')
			if (savedConfig) {
				try {
					const parsed = JSON.parse(savedConfig)
					setApiConfig(prev => ({ ...prev, ...parsed }))
				} catch (e) {
					console.error('Failed to parse saved config:', e)
				}
			}
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
				email: config?.fofa?.email || '',
				key: config?.fofa?.key || '',
				address: config?.fofa?.api || 'https://fofa.info',
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


	const saveApiConfig = async () => {
		try {
			const { saveSpaceEngineConfig, notifySpaceEngineConfigChange, getSpaceEngineConfig } = await import('@/lib/config')
			
			// 获取当前配置，只更新FOFA部分，保留其他引擎配置
			const currentConfig = await getSpaceEngineConfig()
			const newConfig = {
				...currentConfig,
				fofa: {
					api: apiConfig.address,
					email: apiConfig.email,
					key: apiConfig.key,
				},
			}
			await saveSpaceEngineConfig(newConfig)
			// 触发配置变更事件，通知其他页面（如设置页面）更新配置
			notifySpaceEngineConfigChange(newConfig)
			setShowApiConfigDialog(false)
			toast.success('配置已保存')
			// 重新加载配置以确保同步
			await loadApiConfig()
		} catch (error: any) {
			toast.error('保存配置失败: ' + (error.message || '未知错误'))
			console.error('Failed to save API config:', error)
		}
	}

	const loadFavSyntax = async () => {
		try {
			const response = await fetchWithAuth(`/api/v1/syntax?source=fofa`, {
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
				const saved = localStorage.getItem('fofa_fav_syntax')
				if (saved) {
					const parsed = JSON.parse(saved)
					setFavSyntax(Array.isArray(parsed) ? parsed : [])
				}
			} catch (e) {
				console.error('Failed to load from localStorage:', e)
			}
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
					source: 'fofa',
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
					source: 'fofa',
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

	const fetchSearchHistory = async () => {
		try {
			const response = await fetchWithAuth('/api/v1/fofa/history?limit=50', { cache: 'no-store' })
			if (!response.ok) {
				throw new Error(`HTTP ${response.status}`)
			}
			const data = await response.json()
			const nextHistory = Array.isArray(data?.data) ? data.data.filter((item: unknown): item is string => typeof item === 'string') : []
			setSearchHistory(nextHistory)
			return nextHistory
		} catch (error) {
			console.error('Failed to load FOFA search history:', error)
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
			await fetchWithAuth('/api/v1/fofa/history', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({ query: normalizedQuery }),
			})
		} catch (error) {
			console.error('Failed to save FOFA search history:', error)
		}
	}

	const getBackendBaseUrl = () => {
		return getApiBaseURL().trim().replace(/\/$/, '')
	}

	// 获取用户信息
	const fetchUserInfo = async (showDialog = false, silent = false) => {
		if (!silent) {
			setIsLoadingUserInfo(true)
		}
		try {
			const response = await fetchWithAuth(`/api/v1/fofa/user`, {
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

			const data: FofaUserInfo = await response.json()
			if (data.error) {
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

	const getResultMessage = (size?: number, customMessage?: string) => {
		if (customMessage && customMessage.trim()) {
			return customMessage
		}
		if (typeof size === 'number') {
			return size > 0 ? `共找到 ${size} 条结果` : '未查询到数据'
		}
		return '未查询到数据'
	}

	useEffect(() => {
		void fetchSearchHistory()
	}, [])

	const fetchSearchResults = async (
		searchQuery: string,
		pageNum: number,
		pageSize: number,
		filters: TabFilters
	) => {
		// 不再传递敏感信息，后端从配置文件读取
		let response: Response
		try {
			response = await fetchWithAuth(`/api/v1/fofa/search`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: searchQuery,
					pageSize,
					pageNum,
					fraud: filters.fraud,
					cert: filters.cert,
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

		if (data?.error) {
			throw new Error(data?.message || 'FOFA API 返回错误')
		}

		return data
	}

	// 获取统计聚合数据
	const fetchStats = async (tabId: string, searchQuery: string, fields: string) => {
		// 更新tab的加载状态
		updateTabState(tabId, tab => ({
			...tab,
			isLoadingStats: true,
		}))

		try {
			const response = await fetchWithAuth(`/api/v1/fofa/stats`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: searchQuery,
					fields: fields,
				}),
			})

			if (!response.ok) {
				const errorData = await response.json().catch(() => ({ error: '未知错误' }))
				throw new Error(errorData.error || errorData.message || '请求失败')
			}

			const data: StatsResponse = await response.json()
			if (data.error) {
				throw new Error(data.message || '获取统计聚合失败')
			}

			// 更新tab的统计数据
			if (data.data && Object.keys(data.data).length > 0) {
				updateTabState(tabId, tab => ({
					...tab,
					statsData: data.data,
					isLoadingStats: false,
				}))
			} else {
				updateTabState(tabId, tab => ({
					...tab,
					statsData: null,
					isLoadingStats: false,
				}))
			}
		} catch (error: any) {
			console.error('Failed to fetch stats:', error)
			// 不显示alert，避免频繁弹窗，只在控制台记录错误
			if (error.message && !error.message.includes('请求速度过快')) {
				console.warn('获取统计聚合失败: ' + (error.message || '未知错误'))
			}
			// 更新tab的加载状态
			updateTabState(tabId, tab => ({
				...tab,
				isLoadingStats: false,
			}))
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
				total: data.size || 0,
				message: getResultMessage(data.size, data.message),
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

	const handlePageChange = async (tabId: string, direction: 'prev' | 'next') => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab || tab.isLoading) return

		const totalPages = Math.max(1, Math.ceil((tab.total || 0) / tab.pageSize))
		const targetPage = direction === 'prev' ? tab.currentPage - 1 : tab.currentPage + 1
		if (targetPage < 1 || targetPage > totalPages) return

		try {
			await refreshTab(tabId, targetPage)
		} catch (error: any) {
			console.error('Pagination error:', error)
			toast.error('分页加载失败: ' + (error?.message || '未知错误'))
		}
	}

	const handlePageSizeChange = async (tabId: string, newPageSize: number) => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab || tab.isLoading) return

		// 先更新状态为加载中
		updateTabState(tabId, current => ({
			...current,
			pageSize: newPageSize,
			currentPage: 1,
			isLoading: true,
			error: undefined,
		}))

		// 使用新的 pageSize 重新加载第一页数据
		try {
			const data = await fetchSearchResults(tab.query, 1, newPageSize, tab.filters)
			updateTabState(tabId, current => ({
				...current,
				results: data.results || [],
				total: data.size || 0,
				message: getResultMessage(data.size, data.message),
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

	// 格式化组件数据
	const formatProduct = (product: string): string[] => {
		if (!product || !product.trim()) return []
		return product.split(',').map(p => p.trim()).filter(p => p.length > 0)
	}

	// 定义表格列配置（使用 useMemo 以便访问 currentTab）
	const columns: ColumnDef<FofaResult>[] = useMemo(() => {
		const currentTab = tabs.find(t => t.id === activeTab)
		return [
			{
				key: '#',
				label: '#',
				visible: true,
				filterable: false,
				cellClassName: 'w-16',
				render: (value, row, index) => {
					if (!currentTab) return index + 1
					return (currentTab.currentPage - 1) * currentTab.pageSize + index + 1
				},
			},
			{
				key: 'URL',
				label: 'URL',
				visible: false,
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
			key: 'Host',
			label: 'Host',
			visible: true,
			filterable: true,
			cellClassName: 'font-mono text-sm',
			render: (value, row) => {
				const host = row.Host || ''
				const protocol = row.Protocol || ''
				const hostLower = host.toLowerCase()
				const protocolLower = protocol.toLowerCase()
				let hostLink: string | null = null

				if (hostLower.startsWith('http://') || hostLower.startsWith('https://')) {
					hostLink = host
				} else if (host && (protocolLower === 'http' || protocolLower === 'https')) {
					hostLink = `${protocolLower}://${host}`
				}

				if (hostLink) {
					return (
						<a
							href={hostLink}
							target="_blank"
							rel="noopener noreferrer"
							className="text-primary hover:underline flex items-center gap-1"
						>
							{host}
							<ExternalLink className="w-3 h-3" />
						</a>
					)
				}

				return <span>{host || '-'}</span>
			},
		},
		{
			key: 'Domain',
			label: '域名',
			visible: true,
			filterable: true,
			cellClassName: 'max-w-xs truncate',
		},
		{
			key: 'IP',
			label: 'IP',
			visible: true,
			filterable: true,
			cellClassName: 'font-mono text-sm',
		},
		{
			key: 'Port',
			label: '端口',
			visible: true,
			filterable: true,
		},
		{
			key: 'Protocol',
			label: '协议',
			visible: true,
			filterable: true,
			render: (value) => (
				<Badge variant="secondary" className="bg-primary/10 text-primary hover:bg-primary/20">
					{value}
				</Badge>
			),
		},
		{
			key: 'Title',
			label: '标题',
			visible: true,
			filterable: true,
			cellClassName: 'max-w-xs truncate',
		},
		{
			key: 'Country',
			label: '地区',
			visible: true,
			filterable: true,
		},
		{
			key: 'Product',
			label: '组件',
			visible: true,
			filterable: true,
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
			key: 'ICP',
			label: '备案号',
			visible: true,
			filterable: true,
		},
		]
	}, [tabs, activeTab])

	// 内置语法提示
	const syntaxData = useMemo<Array<{ title: string; data: { key: string; description: string; filed1: string; filed2: string; filed3: string, level?: string }[] }>>(() => [
		{
			title: "基础类",
			data: [
				{ key: 'ip="1.1.1.1"', description: '通过单一IPv4地址进行查询', filed1: "✓", filed2: "✓", filed3: "-" },
				{ key: 'port="6379"', description: '通过端口号进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'domain="qq.com"', description: '通过根域名进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'host=".fofa.info"', description: '通过主机名进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'os="centos"', description: '通过操作系统进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'server="Microsoft-IIS/10"', description: '通过服务器进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'asn="19551"', description: '通过自治系统号进行搜索', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'org="LLC Baxet"', description: '通过所属组织进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'is_domain=(true/false)', description: '筛选(拥有/没有)域名的资产', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'is_ipv6=(true/false)', description: '筛选是(ipv6/ipv4)的资产', filed1: "✓", filed2: "-", filed3: "-" },
			]
		},
		{
			title: "标记类",
			data: [
				{ key: 'app="Microsoft-Exchange"', description: '通过FOFA整理的规则进行查询', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'fid="sSXXGNUO2FefBTcCLIT/2Q=="', description: '通过FOFA聚合的站点指纹进行查询', filed1: "✓", filed2: "✓", filed3: "-" },
				{ key: 'product="NGINX"', description: '通过FOFA标记的产品名进行查询', filed1: "✓", filed2: "✓", filed3: "-" },
				{ key: 'category="服务"', description: '通过FOFA标记的分类进行查询', filed1: "✓", filed2: "✓", filed3: "-" },
				{ key: 'type="subdomain"', description: '筛选服务（网站类）资产', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'cloud_name="Aliyundun"', description: '通过云服务商进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'is_cloud=(true/false)', description: '筛选(是/不是)云服务的资产', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'is_fraud=(true/false)', description: '筛选(是/不是)仿冒垃圾站群的资产', filed1: "✓", filed2: "-", filed3: "-", level: "专业版" },
				{ key: 'is_honeypot=(true/false)', description: '筛选(是/不是)蜜罐的资产', filed1: "✓", filed2: "-", filed3: "-", level: "专业版" },
			]
		},
		{
			title: "协议类",
			data: [
				{ key: 'protocol="quic"', description: '通过协议名称进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'banner="users"', description: '通过协议返回信息进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'base_protocol="tcp"', description: '查询传输层为tcp协议的资产', filed1: "✓", filed2: "✓", filed3: "-" },
			]
		},
		{
			title: "证书类",
			data: [
				{ key: 'cert="baidu"', description: '通过证书进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.subject="Oracle Corporation"', description: '通过证书的持有者进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.issuer="DigiCert"', description: '通过证书的颁发者进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.subject.org="Oracle Corporation"', description: '通过证书持有者的组织进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.subject.cn="baidu.com"', description: '通过证书持有者的通用名称进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.issuer.org="cPanel, Inc."', description: '通过证书颁发者的组织进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.issuer.cn="Synology Inc. CA"', description: '通过证书颁发者的通用名称进行查询', filed1: "✓", filed2: "✓", filed3: "✓" },
				{ key: 'cert.is_valid=(true/false)', description: '筛选证书(是/不是)有效证书的资产', filed1: "✓", filed2: "-", filed3: "-", level: "个人版" },
				{ key: 'cert.is_match=(true/false)', description: '筛选证书和域名(匹配/不匹配)的资产', filed1: "✓", filed2: "-", filed3: "-", level: "个人版" },
				{ key: 'cert.is_expired=(true/false)', description: '筛选证书(已过期/未过期)的资产', filed1: "✓", filed2: "-", filed3: "-", level: "个人版" },
				{ key: 'tls.version="TLS 1.3"', description: '通过tls的协议版本进行查询', filed1: "✓", filed2: "✓", filed3: "-" },
			]
		},
		{
			title: "时间类",
			data: [
				{ key: 'after="2023-01-01"', description: '筛选某一时间之后有更新的资产', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'before="2023-12-01"', description: '筛选某一时间之前有更新的资产', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'after="2023-01-01" && before="2023-12-01"', description: '筛选某一时间区间有更新的资产', filed1: "✓", filed2: "-", filed3: "-" }
			]
		},
		{
			title: "独立IP语法",
			data: [
				{ key: 'port_size="6"', description: '筛选开放端口数量等于6个的独立IP', filed1: "✓", filed2: "✓", filed3: "-", level: "个人版" },
				{ key: 'port_size_gt="6"', description: '筛选开放端口数量大于6个的独立IP', filed1: "✓", filed2: "-", filed3: "-", level: "个人版" },
				{ key: 'port_size_lt="12"', description: '筛选开放端口数量小于12个的独立IP', filed1: "✓", filed2: "-", filed3: "-", level: "个人版" },
				{ key: 'ip_ports="80,161"', description: '筛选同时开放不同端口的独立IP', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'ip_country="CN"', description: '通过国家的简称代码进行查询独立IP', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'ip_region="Zhejiang"', description: '通过省份/地区英文名称进行查询独立IP', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'ip_city="Hangzhou"', description: '通过城市英文名称进行查询独立IP', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'ip_after="2021-03-18"', description: '筛选某一时间之后有更新的独立IP', filed1: "✓", filed2: "-", filed3: "-" },
				{ key: 'ip_before="2019-09-09"', description: '筛选某一时间之前有更新的独立IP', filed1: "✓", filed2: "-", filed3: "-" }
			],
		}
	], [])

	// 根据输入内容匹配语法提示（使用本地语法数据）
	const getSuggestions = useCallback(async (input: string): Promise<{ key: string; description: string; category: string }[]> => {
		const matches: { key: string; description: string; category: string }[] = []

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

		// 从 syntaxData 中查找匹配的语法
		for (const category of syntaxData) {
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
	}, [favSyntax, searchHistory, syntaxData])

	// 当输入变化时更新提示
	useEffect(() => {
		let cancelled = false

		if (inputFocused) {
			getSuggestions(query || '').then(matches => {
				if (cancelled) return
				setSuggestions(matches)
				setShowSuggestions(matches.length > 0)
				selectedSuggestionIndexRef.current = -1
				setSelectedSuggestionIndex(-1)
			})
		} else {
			setShowSuggestions(false)
			setSuggestions([])
		}

		return () => {
			cancelled = true
		}
	}, [getSuggestions, inputFocused, query])

	const startSearch = async (searchQuery?: string) => {
		const queryToUse = searchQuery || query
		if (!queryToUse.trim()) {
			toast.error('请输入查询语法')
			return
		}

		// API配置检查由后端处理，前端不再检查

		setIsSearching(true)
		const filters: TabFilters = { fraud, cert }
		const trimmedQuery = queryToUse.trim()
		const shouldFetchStats = !shouldSkipStatsForBatchQuery(trimmedQuery)

		// 先创建 tab，以便同时发起统计请求
		const newTabId = Date.now().toString()
		const newTab: SearchTab = {
			id: newTabId,
			query: trimmedQuery,
			results: [],
			total: 0,
			pageSize: DEFAULT_PAGE_SIZE,
			currentPage: 1,
			message: '加载中...',
			filters,
			tableFilters: {},
			isLoading: true,
			error: undefined,
			lastUpdated: Date.now(),
			statsData: null,
			isLoadingStats: shouldFetchStats,
		}

		setTabs(prev => [...prev, newTab])
		setActiveTab(newTabId)

		// 保存搜索历史记录
		void saveSearchHistory(trimmedQuery)

		try {
			// 同时发起表格数据和统计数据的请求
			const searchPromise = fetchSearchResults(trimmedQuery, 1, DEFAULT_PAGE_SIZE, filters)
			const [data] = shouldFetchStats
				? await Promise.all([
					searchPromise,
					// 统计请求在后台执行，不阻塞主流程
					fetchStats(newTabId, trimmedQuery, statsFields).catch(err => {
						console.error('Failed to fetch stats:', err)
						// 即使统计请求失败，也不影响主流程
					})
				])
				: [await searchPromise]

			// 更新表格数据
			updateTabState(newTabId, current => ({
				...current,
				results: data.results || [],
				total: data.size || 0,
				message: getResultMessage(data.size, data.message),
				isLoading: false,
				error: undefined,
				lastUpdated: Date.now(),
			}))
			void fetchUserInfo(false, true)
		} catch (error: any) {
			toast.error('搜索失败: ' + (error.message || '未知错误'))
			console.error('Search error:', error)
			// 如果搜索失败，更新 tab 状态
			updateTabState(newTabId, current => ({
				...current,
				isLoading: false,
				error: error?.message || '加载失败',
			}))
		} finally {
			setIsSearching(false)
		}
	}

	const closeTab = (tabId: string) => {
		const newTabs = tabs.filter(t => t.id !== tabId)
		setTabs(newTabs)
		if (activeTab === tabId) {
			setActiveTab(newTabs[0]?.id || '')
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

		// API配置检查由后端处理，前端不再检查
		setIsSearching(true)
		const filters: TabFilters = { fraud, cert }

		try {
			const data = await fetchSearchResults(cSegmentQuery, 1, DEFAULT_PAGE_SIZE, filters)

			const newTab: SearchTab = {
				id: Date.now().toString(),
				query: cSegmentQuery,
				results: data.results || [],
				total: data.size || 0,
				pageSize: DEFAULT_PAGE_SIZE,
				currentPage: 1,
				message: getResultMessage(data.size, data.message),
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

	const buildWebscanTarget = (item: FofaResult): string | null => {
		const rawUrl = (item.URL || '').trim()
		if (rawUrl) {
			if (/^https?:\/\//i.test(rawUrl)) return rawUrl
		}

		const host = (item.Host || item.Domain || item.IP || '').trim().replace(/^https?:\/\//i, '')
		if (!host) return null
		const protocol = (item.Protocol || '').toLowerCase() === 'https' || String(item.Port || '') === '443' ? 'https' : 'http'
		const port = String(item.Port || '').trim()
		const hostWithPort = port && !host.includes(`:${port}`) ? `${host}:${port}` : host
		return `${protocol}://${hostWithPort}`
	}

	const buildCrackTarget = (item: FofaResult): string | null => {
		const host = (item.IP || item.Host || item.Domain || '').trim().replace(/^https?:\/\//i, '')
		const port = String(item.Port || '').trim()
		if (!host || !port) return null
		const protocol = (item.Protocol || 'tcp').toLowerCase()
		return `${protocol}://${host}:${port}`
	}

	const handleSaveToAssetCenter = async (targets: FofaResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error(t.common.saveToAssetCenterEmpty)
			return
		}
		const toastId = toast.loading(t.common.savingToAssetCenter)
		try {
			const result = await saveSpaceEngineAssets(
				'fofa',
				targets.map(item => ({
					ip: item.IP,
					domain: item.Domain,
					url: item.URL,
					title: item.Title,
					port: item.Port,
					protocol: item.Protocol,
					product: item.Product,
					country: item.Country,
					host: item.Host,
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

	const handleSendToWebscan = async (targets: FofaResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error('请先勾选要转发的资产')
			return
		}
		const urlTargets = Array.from(
			new Set(
				targets
					.map(buildWebscanTarget)
					.filter((item): item is string => !!item)
			)
		)
		if (urlTargets.length === 0) {
			toast.error('没有可用于网站扫描的有效目标')
			return
		}

		const taskName = `webscan - ${urlTargets[0]} - ${new Date().toLocaleString('zh-CN')}`
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'webscan',
					taskName,
					config: {
						targets: urlTargets,
						threads: 30,
						webscanOption: 3,
						rootPathScan: true,
						skipNucleiWithoutTags: false,
						skipNucleiWithWAF: true,
						screenshot: true,
						activeTimeoutLimit: 5,
					},
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || '创建任务失败')
			}
			const data = await response.json()
			if (!data.success || !data.taskId) {
				throw new Error(data.error || '创建任务失败')
			}
			toast.success(`已转发到漏洞扫描（${urlTargets.length} 个目标）`)
		} catch (error: any) {
			toast.error('转发失败: ' + (error?.message || '未知错误'))
		}
	}

	const handleSendToCrack = async (targets: FofaResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error('请先勾选要转发的资产')
			return
		}
		const crackTargets = Array.from(
			new Set(
				targets
					.map(buildCrackTarget)
					.filter((item): item is string => !!item)
			)
		)
		if (crackTargets.length === 0) {
			toast.error('没有可用于密码喷洒的有效目标')
			return
		}

		const taskName = `crack - ${crackTargets[0]} - ${new Date().toLocaleString('zh-CN')}`
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'crack',
					taskName,
					config: {
						targets: crackTargets,
						threads: 10,
						username: '',
						password: '',
						defaultDict: true,
					},
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || '创建任务失败')
			}
			const data = await response.json()
			if (!data.success || !data.taskId) {
				throw new Error(data.error || '创建任务失败')
			}
			toast.success(`已转发到密码喷洒（${crackTargets.length} 个目标）`)
		} catch (error: any) {
			toast.error('转发失败: ' + (error?.message || '未知错误'))
		}
	}

	const buildPortscanTarget = (item: FofaResult): string | null => {
		const rawHost = (item.IP || item.Host || item.Domain || '').trim().replace(/^https?:\/\//i, '')
		if (!rawHost) return null
		return rawHost.split(':')[0] || rawHost
	}

	const handleSendToPortscan = async (targets: FofaResult[], portGroup: PortGroupOption) => {
		if (!targets || targets.length === 0) {
			toast.error('请先勾选要转发的资产')
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
				throw new Error(errorData.error || '创建任务失败')
			}
			const data = await response.json()
			if (!data.success || !data.taskId) {
				throw new Error(data.error || '创建任务失败')
			}
			toast.success(`已转发到端口扫描 ${portGroup.text}（${portscanTargets.length} 个目标）`)
		} catch (error: any) {
			toast.error('转发失败: ' + (error?.message || '未知错误'))
		}
	}

	const addSyntaxToQuery = (syntaxKey: string) => {
		if (!query) {
			setQuery(syntaxKey)
		} else {
			setQuery(`${query} && ${syntaxKey}`)
		}
	}

	// 输入提示选择函数：直接替换查询内容
	const selectSuggestion = (syntaxKey: string) => {
		setQuery(syntaxKey)
		setShowSuggestions(false)
		selectedSuggestionIndexRef.current = -1
		setSelectedSuggestionIndex(-1)
	}

	const openExportDialog = () => {
		const currentTab = tabs.find(t => t.id === activeTab)
		if (!currentTab) return
		const maxByQuota = getEffectiveExportCap(currentTab.total || 0, userInfo)
		setExportCount(maxByQuota)
		setSelectedExportFields(exportFieldOptions.map(item => item.value))
		setShowExportDialog(true)
	}

	const runExport = async () => {
		const currentTab = tabs.find(t => t.id === activeTab)
		if (!currentTab) return
		if (selectedExportFields.length === 0) {
			toast.error('请至少选择一个导出字段')
			return
		}
		const maxByQuota = getEffectiveExportCap(currentTab.total || 0, userInfo)
		const totalToExport = Math.max(0, Math.min(Math.floor(exportCount || 0), maxByQuota))
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
			const response = await fetchWithAuth(`/api/v1/fofa/export/tasks`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: currentTab.query,
					exportCount: totalToExport,
					fields: selectedFields,
					format: exportFormat,
					fileName: exportFileName,
					fraud: currentTab.filters.fraud,
					cert: currentTab.filters.cert,
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
			const seen = new Set()
			results = results.filter(item => {
				const key = `${item.IP}_${item.Title}`
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
					toast.success('已复制到剪贴板')
				} else {
					toast.error('复制失败，请手动复制')
				}
			})
		}

	// 调用后端API计算图标hash
	const calculateIconHash = async (url?: string, imageData?: string): Promise<string> => {
		const requestBody: { url?: string; imageData?: string } = {}
		if (url) {
			requestBody.url = url
		}
		if (imageData) {
			requestBody.imageData = imageData
		}

		let response: Response
		try {
			response = await fetchWithAuth(`/api/v1/fofa/icon-hash`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify(requestBody),
			})
		} catch (error: any) {
			console.error('网络请求失败:', error)
			throw new Error('无法连接后端服务: ' + (error.message || '未知错误'))
		}

		if (!response.ok) {
			let errorMessage = '计算hash失败'
			try {
				const data = await response.json()
				errorMessage = data?.error || errorMessage
				console.error('后端返回错误:', data)
			} catch (e) {
				const text = await response.text().catch(() => '')
				console.error('响应内容:', text)
				errorMessage = `HTTP ${response.status}: ${text || response.statusText}`
			}
			throw new Error(errorMessage)
		}

		const data = await response.json()
		console.log('后端返回数据:', data)
		
		if (!data.hash) {
			throw new Error('后端未返回hash值')
		}

		return data.hash
	}

	const handleImageUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
		const file = e.target.files?.[0]
		if (file) {
			if (!file.type.startsWith('image/')) {
				toast.error('请上传图片文件')
				return
			}
			setUploadedImage(file)
			const reader = new FileReader()
			reader.onloadend = () => {
				setImagePreview(reader.result as string)
			}
			reader.readAsDataURL(file)
		}
	}

	const handleIconSearch = async () => {
		if (isCalculatingHash) return

		let imageUrl: string | undefined
		let imageData: string | undefined

		if (uploadedImage) {
			// 对于上传的图片，转换为base64
			setIsCalculatingHash(true)
			try {
				const base64Data = await new Promise<string>((resolve, reject) => {
					const reader = new FileReader()
					reader.onloadend = () => {
						resolve(reader.result as string)
					}
					reader.onerror = () => {
						reject(new Error('读取图片文件失败'))
					}
					reader.readAsDataURL(uploadedImage!)
				})

				imageData = base64Data
				const hash = await calculateIconHash(undefined, imageData)
				if (hash) {
					const searchQuery = `icon_hash="${hash}"`
					setQuery(searchQuery)
					setShowIconSearchDialog(false)
					// 重置状态
					setIconUrl('')
					setUploadedImage(null)
					setImagePreview(null)
					// 直接执行搜索，传入查询字符串
					await startSearch(searchQuery)
				} else {
					toast.error('计算hash失败，未返回有效结果')
				}
			} catch (error: any) {
				toast.error('计算hash失败: ' + (error.message || '未知错误'))
				console.error('Icon hash calculation error:', error)
			} finally {
				setIsCalculatingHash(false)
			}
		} else if (iconUrl) {
			imageUrl = iconUrl.trim()
			if (!imageUrl) {
				toast.error('请输入 Favicon URL 或上传图片')
				return
			}

			setIsCalculatingHash(true)
			try {
				const hash = await calculateIconHash(imageUrl)
				if (hash) {
					const searchQuery = `icon_hash="${hash}"`
					setQuery(searchQuery)
					setShowIconSearchDialog(false)
					// 重置状态
					setIconUrl('')
					setUploadedImage(null)
					setImagePreview(null)
					// 直接执行搜索，传入查询字符串
					await startSearch(searchQuery)
				} else {
					toast.error('计算hash失败，未返回有效结果')
				}
			} catch (error: any) {
				toast.error('计算hash失败: ' + (error.message || '未知错误'))
				console.error('Icon hash calculation error:', error)
			} finally {
				setIsCalculatingHash(false)
			}
		} else {
			toast.error('请输入 Favicon URL 或上传图片')
			return
		}
	}

	const iconHashSearch = () => {
		setShowIconSearchDialog(true)
	}

	const batchSearch = () => {
		setShowBatchSearchDialog(true)
		setBatchInput('')
	}

	// 检查批量输入是否有有效目标
	const hasValidBatchTargets = (input: string): boolean => {
		if (!input.trim()) return false
		const analysis = analyzeBatchTargets(input)
		return analysis.ipTargets.length + analysis.domainTargets.length > 0
	}

	const handleBatchSearch = async () => {
		if (!batchInput.trim()) {
			toast.error('请输入查询内容')
			return
		}

		// 检查是否有有效目标
		if (!hasValidBatchTargets(batchInput)) {
			toast.error('没有有效的目标，请输入有效的IP地址、IP段或域名')
			return
		}

		const { ipTargets, domainTargets, invalidIPTargets } = analyzeBatchTargets(batchInput)
		if (invalidIPTargets.length > 0) {
			const firstInvalid = invalidIPTargets[0]
			toast.error(`包含不支持的IP目标: ${firstInvalid.target}（${firstInvalid.reason}）`)
			return
		}

		const ipQueries = ipTargets.map(target => `ip="${target}"`)
		const domainQueries = domainTargets.map(target => `domain="${target}" || host="${target}"`)
		const allQueries = [...ipQueries, ...domainQueries]
		if (allQueries.length === 0) {
			toast.error('没有有效的目标')
			return
		}

		const queryTasks = Array.from(
			{ length: Math.ceil(allQueries.length / BATCH_TARGETS_PER_TASK) },
			(_, index) => allQueries.slice(index * BATCH_TARGETS_PER_TASK, (index + 1) * BATCH_TARGETS_PER_TASK).join(' || '),
		)
		setShowBatchSearchDialog(false)
		setBatchInput('')
		toast.success(formatText(t.fofaPage.page.batchTaskStarted, { count: queryTasks.length }))

		for (const [index, taskQuery] of queryTasks.entries()) {
			setQuery(taskQuery)
			await startSearch(taskQuery)
			if (index < queryTasks.length - 1) {
				await new Promise(resolve => setTimeout(resolve, BATCH_TASK_DELAY_MS))
			}
		}
	}

	// 处理标签页切换，同步查询语句到文本框
	const handleTabChange = (tabId: string) => {
		setActiveTab(tabId)
		const tab = tabs.find(t => t.id === tabId)
		if (tab) {
			setQuery(tab.query)
		}
	}

	const currentTab = tabs.find(t => t.id === activeTab)
	const currentExportMaxCount = useMemo(() => {
		if (!currentTab) return 0
		return getEffectiveExportCap(currentTab.total || 0, userInfo)
	}, [currentTab, userInfo])
	const currentExportCost = useMemo(() => {
		return Math.max(0, Math.min(Math.floor(exportCount || 0), currentExportMaxCount))
	}, [exportCount, currentExportMaxCount])

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
					onOpenSyntaxHelpDialog={() => setShowSyntaxHelpDialog(true)}
					onOpenFavSyntaxDialog={() => setShowFavSyntaxDialog(true)}
					onOpenAddFavDialog={() => {
						setShowAddFavDialog(true)
						setNewSyntaxContent(query)
					}}
					onIconHashSearch={iconHashSearch}
					onBatchSearch={batchSearch}
					showSuggestions={showSuggestions}
					suggestions={suggestions}
					inputFocused={inputFocused}
					setInputFocused={setInputFocused}
					setShowSuggestions={setShowSuggestions}
					selectedSuggestionIndex={selectedSuggestionIndex}
					selectedSuggestionIndexRef={selectedSuggestionIndexRef}
					setSelectedSuggestionIndex={setSelectedSuggestionIndex}
					onSelectSuggestion={selectSuggestion}
					suggestionsRef={suggestionsRef}
					textareaRef={textareaRef}
					fraud={fraud}
					setFraud={setFraud}
					cert={cert}
					setCert={setCert}
				/>

				{/* 搜索结果 */}
				{tabs.length > 0 && (
					<div className="flex gap-4">
						{/* 左侧统计聚合区域 */}
						{currentTab && ((currentTab.statsData && Object.keys(currentTab.statsData).length > 0) || currentTab.isLoadingStats) ? (
							<div className="hidden w-64 shrink-0 md:block">
								<Card className="p-4">
									<div className="mb-4">
										<h3 className="text-sm font-semibold">{t.fofaPage.page.statsTitle}</h3>
									</div>
									{currentTab.isLoadingStats ? (
										<div className="flex items-center justify-center py-8">
											<div className="w-4 h-4 border-2 border-primary/30 border-t-primary rounded-full animate-spin" />
										</div>
									) : currentTab.statsData && Object.keys(currentTab.statsData).length > 0 ? (
										<div className="space-y-4">
											{Object.entries(currentTab.statsData).map(([field, items]) => (
												<div key={field} className="border-b border-border pb-4 last:border-0 last:pb-0">
													<h4 className="font-medium mb-2 text-xs text-muted-foreground">
														{formatText(t.fofaPage.page.fieldRank, {
															field: field === 'protocol' ? 'Protocol'
																: field === 'server' ? 'Server'
																: field === 'domain' ? 'Domain'
																: field === 'port' ? 'Port'
																: field === 'title' ? 'Title'
																: field === 'ip' ? 'IP'
																: field === 'country' ? 'Country'
																: field === 'product' ? 'Product'
																: field === 'app' ? 'App'
																: field === 'os' ? 'OS'
																: field === 'region' ? 'Region'
																: field === 'city' ? 'City'
																: field === 'isp' ? 'ISP'
																: field,
														})}
													</h4>
													<div className="space-y-1.5 max-h-[300px] overflow-y-auto">
														{items.map((item, idx) => {
															const handleStatsItemClick = () => {
																if (!currentTab || !item.name) return
																
																// 构建新的查询条件
																let newCondition = ''
																switch (field) {
																	case 'domain':
																		newCondition = `domain="${item.name}"`
																		break
																	case 'port':
																		newCondition = `port="${item.name}"`
																		break
																	case 'protocol':
																		newCondition = `protocol="${item.name}"`
																		break
																	case 'server':
																		newCondition = `server="${item.name}"`
																		break
																	case 'title':
																		newCondition = `title="${item.name}"`
																		break
																	default:
																		return
																}
																
																// 在原有查询条件基础上添加 && 新条件
																const originalQuery = currentTab.query
																let newQuery = ''
																
																if (originalQuery) {
																	// 如果原查询包含 || 符号，需要添加括号
																	if (originalQuery.includes('||')) {
																		// 检查是否已经有括号包裹（简单检查，避免重复添加）
																		const trimmed = originalQuery.trim()
																		if (trimmed.startsWith('(') && trimmed.endsWith(')')) {
																			// 已经有括号，直接使用
																			newQuery = `${originalQuery} && ${newCondition}`
																		} else {
																			// 没有括号，添加括号
																			newQuery = `(${originalQuery}) && ${newCondition}`
																		}
																	} else {
																		// 不包含 ||，直接拼接
																		newQuery = `${originalQuery} && ${newCondition}`
																	}
																} else {
																	// 没有原查询，直接使用新条件
																	newQuery = newCondition
																}
																
																// 执行搜索
																setQuery(newQuery)
																setTimeout(() => {
																	startSearch(newQuery)
																}, 100)
															}
															
															// 只有可直接追加查询条件的字段支持点击钻取
															const isClickable = ['domain', 'port', 'protocol', 'server', 'title'].includes(field)
															
															return (
																<div 
																	key={idx} 
																	onClick={isClickable ? handleStatsItemClick : undefined}
																	className={cn(
																		"flex items-center justify-between text-xs rounded px-1.5 py-1 transition-colors",
																		isClickable 
																			? "hover:bg-primary/10 hover:text-primary cursor-pointer" 
																			: "hover:bg-muted/50"
																	)}
																>
																	<span className="truncate flex-1 mr-2" title={item.name || t.fofaPage.page.emptyValue}>
																		{item.name || t.fofaPage.page.emptyValue}
																	</span>
																	<span className="text-muted-foreground shrink-0 font-medium">
																		{item.count}
																	</span>
																</div>
															)
														})}
													</div>
												</div>
											))}
										</div>
									) : (
										<div className="text-center py-8 text-sm text-muted-foreground">
											{t.fofaPage.page.noStats}
										</div>
									)}
								</Card>
							</div>
						) : null}
						
						{/* 右侧搜索结果表格 */}
						<div className="flex-1 min-w-0">
							<FofaResultsCard
								tabs={tabs}
								activeTab={activeTab}
								onTabChange={handleTabChange}
								onCloseTab={closeTab}
								isSearching={isSearching}
								currentTab={currentTab}
								columns={columns}
								onOpenExportDialog={openExportDialog}
								onCopyUrls={copyURLs}
								onHandleCSegmentQuery={handleCSegmentQuery}
								onSaveToAssetCenter={handleSaveToAssetCenter}
								onSendToWebscan={handleSendToWebscan}
								onSendToCrack={handleSendToCrack}
								onSendToPortscan={handleSendToPortscan}
								portScanGroups={portScanGroups}
								onRefreshTab={refreshTab}
								onPageSizeChange={handlePageSizeChange}
								onTableFiltersChange={setTableFilters}
							/>
						</div>
					</div>
				)}

				{tabs.length === 0 && !isSearching && (
					<Card className="p-6 md:p-12">
						<div className="text-center text-muted-foreground">
							<Radar className="w-10 h-10 md:w-16 md:h-16 mx-auto mb-3 md:mb-4 opacity-50" />
							<p>{t.fofaPage.results.queryEmpty}</p>
						</div>
					</Card>
				)}
			</div>

			<Dialog open={showExportDialog} onOpenChange={setShowExportDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-xl max-h-[calc(100vh-4rem)] overflow-y-auto">
					<DialogHeader>
						<DialogTitle>{t.fofaPage.page.exportTitle}</DialogTitle>
						<DialogDescription>{t.fofaPage.page.exportDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						{currentTab && (
							<div className="rounded-md border bg-muted/30 p-3 text-sm space-y-1">
								<div className="max-h-32 overflow-y-auto break-all whitespace-pre-wrap pr-2">
									{formatText(t.fofaPage.page.queryLabel, { value: currentTab.query })}
								</div>
								<div>{formatText(t.fofaPage.page.resultLabel, { count: (currentTab.total || 0).toLocaleString() })}</div>
								<div>{formatText(t.fofaPage.page.exportLimitLabel, { count: currentExportMaxCount.toLocaleString() })}</div>
							</div>
						)}
						<div>
							<Label htmlFor="fofa-export-file-name">{t.fofaPage.page.fileName}</Label>
							<Input
								id="fofa-export-file-name"
								value={exportFileName}
								onChange={e => setExportFileName(e.target.value)}
								className="mt-2"
							/>
						</div>
						<div>
							<Label>{t.fofaPage.page.fileFormat}</Label>
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
							<Label>{t.fofaPage.page.exportCount}</Label>
							<div className="mt-2">
								<Input
									type="number"
									min={0}
									max={currentExportMaxCount}
									value={exportCount}
									onChange={e => {
										const next = Number(e.target.value || 0)
										if (!Number.isFinite(next)) return
										setExportCount(Math.max(0, Math.min(Math.floor(next), currentExportMaxCount)))
									}}
									disabled={isExporting}
								/>
							</div>
						</div>
						<div>
							<Label>{t.fofaPage.page.exportFields}</Label>
							<div className="mt-2">
								<SelectTag
									options={localizedExportFieldOptions}
									value={selectedExportFields}
									onChange={next => {
										const normalized = next.filter(item => exportFieldValueSet.has(item))
										if (normalized.length === 0) return
										setSelectedExportFields(normalized)
									}}
									placeholder={t.fofaPage.page.selectExportFields}
									disabled={isExporting}
								/>
							</div>
						</div>
						<div className="text-xs text-muted-foreground">
							{formatText(t.fofaPage.page.currentQuota, {
								value: userInfo?.fofa_point === undefined ? t.fofaPage.page.unknown : userInfo.fofa_point.toLocaleString(),
							})}
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowExportDialog(false)} disabled={isExporting}>{t.fofaPage.page.cancel}</Button>
						<Button onClick={runExport} disabled={isExporting || currentExportCost <= 0 || selectedExportFields.length === 0}>
							{isExporting ? t.fofaPage.page.exporting : t.fofaPage.page.export}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 语法提示对话框 */}
			<Dialog open={showSyntaxHelpDialog} onOpenChange={setShowSyntaxHelpDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-[calc(100%-2rem)] sm:max-w-[800px] max-h-[80vh]">
					<DialogHeader>
						<DialogTitle>{t.fofaPage.page.syntaxTitle}</DialogTitle>
						<DialogDescription>{t.fofaPage.page.syntaxDescription}</DialogDescription>
					</DialogHeader>
					<Tabs value={syntaxKeyword} onValueChange={setSyntaxKeyword} className="w-full">
						<div className="overflow-x-auto -mx-1 px-1">
							<TabsList className="inline-flex h-auto min-w-max">
								{syntaxData.map((item) => (
									<TabsTrigger key={item.title} value={item.title} className="whitespace-nowrap shrink-0">
										{item.title}
									</TabsTrigger>
								))}
							</TabsList>
						</div>

						{syntaxData.map((category) => (
							<TabsContent key={category.title} value={category.title} className="mt-4">
								<div className="space-y-2 max-h-[50vh] overflow-y-auto pr-2">
									<div className="grid gap-2">
										{category.data.map((item, idx) => (
											<div
												key={idx}
												onClick={() => {
													addSyntaxToQuery(item.key)
													setShowSyntaxHelpDialog(false)
												}}
												className="flex items-start justify-between p-3 hover:bg-muted rounded-lg cursor-pointer border border-border/50 hover:border-primary/50 transition-colors group"
											>
												<div className="flex-1 min-w-0">
													<div className="flex items-center gap-2 mb-1">
														<code className="text-primary font-mono text-sm break-all">
															{item.key}
														</code>
														{item.level && (
															<Badge variant="outline" className="text-xs shrink-0">
																{item.level}
															</Badge>
														)}
													</div>
													<p className="text-xs text-muted-foreground">
														{item.description}
													</p>
												</div>
												<div className="flex items-center gap-6 ml-4 shrink-0">
													<div className="flex flex-col items-center gap-1">
														<span className="text-xs text-muted-foreground">=</span>
														<span className={cn(
															"text-sm font-semibold",
															item.filed1 === "✓" ? "text-green-500" : "text-muted-foreground"
														)}>
															{item.filed1}
														</span>
													</div>
													<div className="flex flex-col items-center gap-1">
														<span className="text-xs text-muted-foreground">!=</span>
														<span className={cn(
															"text-sm font-semibold",
															item.filed2 === "✓" ? "text-green-500" : "text-muted-foreground"
														)}>
															{item.filed2}
														</span>
													</div>
													<div className="flex flex-col items-center gap-1">
														<span className="text-xs text-muted-foreground">*=</span>
														<span className={cn(
															"text-sm font-semibold",
															item.filed3 === "✓" ? "text-green-500" : "text-muted-foreground"
														)}>
															{item.filed3}
														</span>
													</div>
												</div>
											</div>
										))}
									</div>
								</div>
							</TabsContent>
						))}
					</Tabs>
				</DialogContent>
			</Dialog>

			{/* 收藏语法对话框 */}
			<Dialog open={showFavSyntaxDialog} onOpenChange={setShowFavSyntaxDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-3xl max-h-[70vh]">
					<DialogHeader>
						<DialogTitle>{t.fofaPage.page.favTitle}</DialogTitle>
						<DialogDescription>{t.fofaPage.page.favDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-2 max-h-[50vh] overflow-y-auto pr-2">
						{favSyntax.length === 0 ? (
							<div className="text-center py-12 text-sm text-muted-foreground">
								<Star className="w-12 h-12 mx-auto mb-4 opacity-30" />
								<p>{t.fofaPage.page.noFav}</p>
								<p className="text-xs mt-2">{t.fofaPage.page.favHint}</p>
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
											{t.fofaPage.page.use}
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
						<DialogTitle>{t.fofaPage.page.addFavTitle}</DialogTitle>
						<DialogDescription>{t.fofaPage.page.addFavDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="syntax-name">{t.fofaPage.page.name}</Label>
							<Input
								id="syntax-name"
								value={newSyntaxName}
								onChange={e => setNewSyntaxName(e.target.value)}
								placeholder={t.fofaPage.page.exampleWordPress}
								className="mt-2"
							/>
						</div>
						<div>
							<Label htmlFor="syntax-content">{t.fofaPage.page.syntaxContent}</Label>
							<Textarea
								id="syntax-content"
								value={newSyntaxContent}
								onChange={e => setNewSyntaxContent(e.target.value)}
								placeholder='domain="example.com"'
								className="mt-2 font-mono min-h-[80px]"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowAddFavDialog(false)}>
							{t.fofaPage.page.cancel}
						</Button>
						<Button onClick={addFavSyntax} disabled={!newSyntaxName || !newSyntaxContent}>
							{t.fofaPage.page.save}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 图标搜索对话框 */}
			<Dialog open={showIconSearchDialog} onOpenChange={setShowIconSearchDialog}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t.fofaPage.page.iconTitle}</DialogTitle>
						<DialogDescription>{t.fofaPage.page.iconDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="icon-url">Favicon URL</Label>
							<Input
								id="icon-url"
								value={iconUrl}
								onChange={e => setIconUrl(e.target.value)}
								placeholder="https://example.com/favicon.ico"
								className="mt-2"
								disabled={!!uploadedImage}
							/>
							<p className="text-xs text-muted-foreground mt-1">
								{t.fofaPage.page.iconUrlHint}
							</p>
						</div>

						<div className="relative">
							<div className="flex items-center gap-2 mb-2">
								<Label>{t.fofaPage.page.uploadImage}</Label>
								{uploadedImage && (
									<Button
										variant="ghost"
										size="sm"
										onClick={() => {
											setUploadedImage(null)
											setImagePreview(null)
										}}
									>
										<X className="w-3 h-3 mr-1" />
										{t.fofaPage.page.clear}
									</Button>
								)}
							</div>
							{!uploadedImage ? (
								<div className="border-2 border-dashed border-border rounded-lg p-6 text-center">
									<input
										type="file"
										accept="image/*"
										onChange={handleImageUpload}
										className="hidden"
										id="icon-upload"
									/>
									<label
										htmlFor="icon-upload"
										className="cursor-pointer flex flex-col items-center gap-2"
									>
										<ImageIcon className="w-8 h-8 text-muted-foreground" />
										<span className="text-sm text-muted-foreground">
											{t.fofaPage.page.clickUpload}
										</span>
										<span className="text-xs text-muted-foreground">
											{t.fofaPage.page.uploadFormats}
										</span>
									</label>
								</div>
							) : (
								<div className="border border-border rounded-lg p-4">
									<div className="flex items-center gap-4">
										{imagePreview && (
											<Image
												src={imagePreview}
												alt={t.fofaPage.page.previewAlt}
												width={64}
												height={64}
												unoptimized
												className="w-16 h-16 object-contain border border-border rounded"
											/>
										)}
										<div className="flex-1">
											<p className="text-sm font-medium">{uploadedImage.name}</p>
											<p className="text-xs text-muted-foreground">
												{(uploadedImage.size / 1024).toFixed(2)} KB
											</p>
										</div>
									</div>
								</div>
							)}
						</div>
					</div>
					<DialogFooter>
						<Button
							variant="outline"
							onClick={() => {
								setShowIconSearchDialog(false)
								setIconUrl('')
								setUploadedImage(null)
								setImagePreview(null)
							}}
						>
							{t.fofaPage.page.cancel}
						</Button>
						<Button onClick={handleIconSearch} disabled={(!iconUrl && !uploadedImage) || isCalculatingHash}>
							{isCalculatingHash ? (
								<>
									<div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />
									{t.fofaPage.page.calculating}
								</>
							) : (
								t.fofaPage.page.search
							)}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 批量查询对话框 */}
			<Dialog open={showBatchSearchDialog} onOpenChange={setShowBatchSearchDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-3xl">
					<DialogHeader>
						<DialogTitle>{t.fofaPage.page.batchTitle}</DialogTitle>
					</DialogHeader>
					<div className="space-y-4">
						<div className="space-y-2">
							<div className="relative">
								<Textarea
									value={batchInput}
									onChange={e => setBatchInput(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === 'Enter' && e.shiftKey) {
											e.preventDefault()
											handleBatchSearch()
										}
									}}
									placeholder="192.0.2.10&#10;198.51.100.0/24&#10;baidu.com&#10;example.com"
									className="font-mono resize-none pr-10 min-h-[300px] max-h-[420px] overflow-y-auto"
								/>
								{batchInput && (
									<div className="absolute top-2 right-2 text-xs text-muted-foreground bg-background/80 px-2 py-1 rounded">
										{(() => {
											const analysis = analyzeBatchTargets(batchInput)
											const ipCount = analysis.ipTargets.length
											const domainCount = analysis.domainTargets.length
											const validCount = ipCount + domainCount
											return `${validCount} / ${analysis.lineCount}`
										})()}
									</div>
								)}
							</div>
							<div className="space-y-1 text-xs text-muted-foreground bg-muted/50 p-3 rounded-md">
								<p>{t.fofaPage.page.batchDescription}</p>
								<p>{t.fofaPage.page.batchLimit}</p>
								<p className="text-primary font-medium">{t.fofaPage.page.batchShortcut}</p>
							</div>
						</div>
						{batchInput && (
							<div className="flex items-center gap-4 text-sm">
								{(() => {
									const analysis = analyzeBatchTargets(batchInput)
									const ipCount = analysis.ipTargets.length
									const domainCount = analysis.domainTargets.length
									const invalidIPCount = analysis.invalidIPTargets.length
									const validCount = ipCount + domainCount
									return (
										<>
											<span className="text-muted-foreground">
												{formatText(t.fofaPage.page.batchIpCount, { count: ipCount })}
											</span>
											<span className="text-muted-foreground">
												{formatText(t.fofaPage.page.batchDomainCount, { count: domainCount })}
											</span>
											<span className="text-muted-foreground">
												{formatText(t.fofaPage.page.batchValidCount, { valid: validCount, total: analysis.lineCount })}
											</span>
											<span className="text-muted-foreground">
												{formatText(t.fofaPage.page.batchTaskCount, { count: Math.ceil(validCount / BATCH_TARGETS_PER_TASK) })}
											</span>
											{invalidIPCount > 0 && (
												<span className="text-destructive">
													不支持IP: {invalidIPCount}
												</span>
											)}
										</>
									)
								})()}
							</div>
						)}
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => {
							setShowBatchSearchDialog(false)
							setBatchInput('')
						}}>
							{t.fofaPage.page.cancel}
						</Button>
						<Button onClick={handleBatchSearch} disabled={!batchInput.trim() || !hasValidBatchTargets(batchInput)}>
							<Search className="w-4 h-4 mr-2" />
							{t.fofaPage.page.runSearch}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* API 配置对话框 */}
			<Dialog open={showApiConfigDialog} onOpenChange={setShowApiConfigDialog}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t.fofaPage.page.apiTitle}</DialogTitle>
						<DialogDescription>{t.fofaPage.page.apiDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="fofa-address">{t.fofaPage.page.apiAddress}</Label>
							<Input
								id="fofa-address"
								value={apiConfig.address}
								onChange={e => setApiConfig({ ...apiConfig, address: e.target.value })}
								placeholder="https://fofa.info"
								className="mt-2"
							/>
						</div>
						<div>
							<Label htmlFor="fofa-email">FOFA Email</Label>
							<Input
								id="fofa-email"
								type="email"
								value={apiConfig.email}
								onChange={e => setApiConfig({ ...apiConfig, email: e.target.value })}
								placeholder="your-email@example.com"
								className="mt-2"
							/>
						</div>
						<div>
							<Label htmlFor="fofa-key">FOFA Key</Label>
							<PasswordInput
								id="fofa-key"
								value={apiConfig.key}
								onChange={e => setApiConfig({ ...apiConfig, key: e.target.value })}
								placeholder="your-api-key"
								className="mt-2"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowApiConfigDialog(false)}>
							{t.fofaPage.page.cancel}
						</Button>
						<Button onClick={saveApiConfig}>
							{t.fofaPage.page.save}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

		</div>
	)
}

export default function FofaPage() {
	return (
		<ClientKeepAlive name="fofa-page">
			<FofaPageContent />
		</ClientKeepAlive>
	)
}
