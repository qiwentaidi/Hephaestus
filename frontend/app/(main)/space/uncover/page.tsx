'use client'

import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Search, X, AlertCircle, AlertTriangle, Loader2, Globe, Server, Zap, Target, History, Trash2, ChevronDown, FolderPlus, FolderInput, FolderX, PencilLine, Radar } from 'lucide-react'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { type ColumnDef } from '@/components/ui/data-table-with-columns'
import { fetchWithAuth } from '@/lib/api'
import hotToast from 'react-hot-toast'
import { saveSpaceEngineAssets } from '@/lib/space-engine-assets'
import { useSpaceEngineQueryParam } from '@/lib/space-engine'
import { getSpaceEngineConfig } from '@/lib/config'
import { toast } from '@/hooks/use-toast'
import { builtInPortGroupOptions, mergePortGroupOptions, type PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'
import { useI18n } from '@/lib/i18n'
import { ClientKeepAlive } from '@/components/client-keep-alive'
import {
	type EngineQuota,
	type EngineStatus,
	type QuerySuggestion,
	type SearchTab,
	type SpaceEngineConfig,
	type SpaceHistoryDetail,
	type SpaceHistoryListItem,
	type SpaceResult,
	type HistoryGroup,
	type SearchStats,
} from './types'
import { ResultStatsPanel } from './components/result-stats-panel'
import { ResultToolbar } from './components/result-toolbar'
import { SearchResultTable } from './components/search-result-table'

function getSpaceResultIdentity(item: SpaceResult) {
	const ip = (item.IP || '').trim().toLowerCase()
	let domain = (item.Domain || '').trim().toLowerCase().replace(/^\*\./, '').replace(/\.$/, '')
	if (!domain || !domain.includes('.') || (ip && domain === ip)) {
		domain = ''
	}
	if (!ip && !domain) return ''
	return `${ip}:${(item.Port || '').trim()}:${domain}`
}

function dedupeSpaceResults(results: SpaceResult[]) {
	const seen = new Map<string, SpaceResult>()
	const deduped: SpaceResult[] = []
	for (const item of results) {
		const identity = getSpaceResultIdentity(item)
		if (identity) {
			const kept = seen.get(identity)
			if (kept) {
				if (!kept.ICP && item.ICP) kept.ICP = item.ICP
				if (!kept.ICPName && item.ICPName) kept.ICPName = item.ICPName
				continue
			}
			seen.set(identity, item)
		}
		deduped.push(item)
	}
	return deduped
}

function UncoverPageContent() {
	const { t } = useI18n()
	const [query, setQuery] = useState('')
	useSpaceEngineQueryParam(setQuery)
	const [tabs, setTabs] = useState<SearchTab[]>([])
	const [activeTab, setActiveTab] = useState<string>('')
	const [statsCollapsed, setStatsCollapsed] = useState(false)
	const [loading, setLoading] = useState(false)
	const [enableFofa, setEnableFofa] = useState(false)
	const [enableHunter, setEnableHunter] = useState(false)
	const [enableQuake, setEnableQuake] = useState(false)
	const [enableZoomeye, setEnableZoomeye] = useState(false)
	const [enableCensys, setEnableCensys] = useState(false)
	const [enableShodan, setEnableShodan] = useState(false)
	const [spaceConfig, setSpaceConfig] = useState<SpaceEngineConfig | null>(null)
	const [configLoading, setConfigLoading] = useState(true)
	const hasUserInteracted = useRef(false)
	const isComposing = useRef(false)
	const [showSyntaxHelper, setShowSyntaxHelper] = useState(false)
	const [showQuerySuggestions, setShowQuerySuggestions] = useState(false)
	const [highlightedSuggestionIndex, setHighlightedSuggestionIndex] = useState(0)
	// 配额相关状态
	const [quotas, setQuotas] = useState<EngineQuota[]>([])
	const [quotaLoading, setQuotaLoading] = useState(false)
	const [portScanGroups, setPortScanGroups] = useState<PortGroupOption[]>(builtInPortGroupOptions)
	const [historyOpen, setHistoryOpen] = useState(false)
	const [historyLoading, setHistoryLoading] = useState(false)
	const [historyItems, setHistoryItems] = useState<SpaceHistoryListItem[]>([])
	const [historyOpeningId, setHistoryOpeningId] = useState<string>('')
	const [historyDeletingId, setHistoryDeletingId] = useState<string>('')
	const [confirmDeleteHistoryId, setConfirmDeleteHistoryId] = useState<string | null>(null)
	const [selectedHistoryIds, setSelectedHistoryIds] = useState<string[]>([])
	const [historyMerging, setHistoryMerging] = useState(false)
	const [expandedHistoryGroupIds, setExpandedHistoryGroupIds] = useState<string[]>([])
	const [searchOptionsOpen, setSearchOptionsOpen] = useState(false)
	const [historyGroupDialogOpen, setHistoryGroupDialogOpen] = useState(false)
	const [historyGroupNameInput, setHistoryGroupNameInput] = useState('')
	const [historyGrouping, setHistoryGrouping] = useState(false)
	const [renameHistoryGroupId, setRenameHistoryGroupId] = useState('')
	const [renameHistoryGroupName, setRenameHistoryGroupName] = useState('')
	const [renameHistoryGroupDialogOpen, setRenameHistoryGroupDialogOpen] = useState(false)
	const [historyGroupRenaming, setHistoryGroupRenaming] = useState(false)
	const queryInputRef = useRef<HTMLInputElement | null>(null)
	const formatText = useCallback((template: string, vars: Record<string, string | number>) => {
		return Object.entries(vars).reduce(
			(acc, [key, value]) => acc.replace(`{${key}}`, String(value)),
			template
		)
	}, [])
	const localizedSyntaxOptions = useMemo(() => t.uncoverPage.syntax.categories, [t])
	const localizedFieldSupportTips = useMemo(() => t.uncoverPage.syntax.supportTips, [t])
	const selectedEngineCount = [enableFofa, enableHunter, enableQuake, enableZoomeye, enableCensys, enableShodan].filter(Boolean).length
	const quickSearchTips = useMemo(() => ([
		'ip:1.1.1.1',
		'title:"管理系统"',
		'domain:example.com',
		'port:443 AND protocol:https',
	]), [])

	const engineOptions = [
		{
			key: 'fofa',
			label: 'FOFA',
			icon: Search,
			enabled: enableFofa,
			configured: !!(spaceConfig?.fofa.email && spaceConfig?.fofa.key),
			toggle: () => setEnableFofa(!enableFofa),
		},
		{
			key: 'hunter',
			label: 'Hunter',
			icon: Target,
			enabled: enableHunter,
			configured: !!spaceConfig?.hunter.key,
			toggle: () => setEnableHunter(!enableHunter),
		},
		{
			key: 'quake',
			label: 'Quake',
			icon: Zap,
			enabled: enableQuake,
			configured: !!spaceConfig?.quake.key,
			toggle: () => setEnableQuake(!enableQuake),
		},
		{
			key: 'zoomeye',
			label: 'ZoomEye',
			icon: Server,
			enabled: enableZoomeye,
			configured: !!spaceConfig?.zoomeye?.key,
			toggle: () => setEnableZoomeye(!enableZoomeye),
		},
		{
			key: 'censys',
			label: 'Censys',
			icon: Globe,
			enabled: enableCensys,
			configured: !!spaceConfig?.censys?.token,
			toggle: () => setEnableCensys(!enableCensys),
		},
		{
			key: 'shodan',
			label: 'Shodan',
			icon: Radar,
			enabled: enableShodan,
			configured: !!spaceConfig?.shodan?.key,
			toggle: () => setEnableShodan(!enableShodan),
		},
	]

	const fieldSuggestions = useMemo<QuerySuggestion[]>(() => {
		return localizedSyntaxOptions
			.filter(category => category.category !== '逻辑运算' && category.category !== 'Logic')
			.flatMap(category =>
				category.items.map(item => {
					const fieldKey = item.syntax.split(':')[0]
					return {
						key: item.syntax,
						label: fieldKey,
						insertText: item.syntax,
						description: item.description,
						type: 'field' as const,
					}
				})
			)
	}, [localizedSyntaxOptions])

	const querySuggestions = useMemo<QuerySuggestion[]>(() => {
		const value = query
		const trimmed = value.trim()
		const endsWithWhitespace = /\s$/.test(value)
		const operatorPattern = /\b(AND|OR|NOT)$/i

		if (!trimmed) {
			return fieldSuggestions.slice(0, 8)
		}

		if (endsWithWhitespace && !operatorPattern.test(trimmed)) {
			return [
				{
					key: 'operator-and',
					label: 'AND',
					insertText: 'AND ',
					description: 'AND',
					type: 'operator',
				},
				{
					key: 'operator-or',
					label: 'OR',
					insertText: 'OR ',
					description: 'OR',
					type: 'operator',
				},
			]
		}

		const currentToken = value.split(/\s+/).pop()?.trim().toLowerCase() || ''
		if (!currentToken || operatorPattern.test(currentToken)) {
			return []
		}

		return fieldSuggestions
			.filter(item => {
				const syntaxPrefix = item.insertText.toLowerCase()
				return syntaxPrefix.startsWith(currentToken) || item.label.toLowerCase().startsWith(currentToken)
			})
			.slice(0, 8)
	}, [fieldSuggestions, query])

	useEffect(() => {
		const timer = window.setTimeout(() => {
			setHighlightedSuggestionIndex(0)
		}, 0)

		return () => window.clearTimeout(timer)
	}, [querySuggestions])

	// 获取引擎配额
	const fetchQuotas = useCallback(async () => {
		setQuotaLoading(true)
		try {
			const response = await fetchWithAuth(`/api/v1/space/quota`)
			if (response.ok) {
				const data = await response.json()
				// 转换后端响应格式为前端使用的数组格式
				const quotaList: EngineQuota[] = []
				if (data.fofa && data.fofa.configured) {
					quotaList.push({ engine: 'fofa', ...data.fofa })
				}
				if (data.hunter && data.hunter.configured) {
					quotaList.push({ engine: 'hunter', ...data.hunter })
				}
				if (data.quake && data.quake.configured) {
					quotaList.push({ engine: 'quake', ...data.quake })
				}
				if (data.zoomeye && data.zoomeye.configured) {
					quotaList.push({ engine: 'zoomeye', ...data.zoomeye })
				}
				if (data.censys && data.censys.configured) {
					quotaList.push({ engine: 'censys', ...data.censys })
				}
				if (data.shodan && data.shodan.configured) {
					quotaList.push({ engine: 'shodan', ...data.shodan })
				}
				setQuotas(quotaList)
			}
		} catch (error) {
			console.error('Failed to fetch quotas:', error)
		} finally {
			setQuotaLoading(false)
		}
	}, [])

	useEffect(() => {
		let cancelled = false

		const loadSpaceConfig = async () => {
			try {
				const rawConfig = await getSpaceEngineConfig()
				if (cancelled) return

				const config: SpaceEngineConfig = {
					...rawConfig,
					zoomeye: {
						...(rawConfig.zoomeye ?? { key: '' }),
						api: rawConfig.zoomeye?.api || 'https://api.zoomeye.ai/',
					},
					censys: rawConfig.censys ?? { token: '', organizationId: '' },
					shodan: {
						...(rawConfig.shodan ?? { key: '' }),
						api: rawConfig.shodan?.api || 'https://api.shodan.io',
					},
				}

				// 批量更新状态，减少渲染次数
				const hasFofa = !!(config.fofa?.email && config.fofa?.key)
				const hasHunter = !!config.hunter?.key
				const hasQuake = !!config.quake?.key
				const hasZoomeye = !!config.zoomeye?.key
				const hasCensys = !!config.censys?.token
				const hasShodan = !!config.shodan?.key

				setSpaceConfig(config)
				if (!hasUserInteracted.current) {
					setEnableFofa(hasFofa)
					setEnableHunter(hasHunter)
					setEnableQuake(hasQuake)
					setEnableZoomeye(hasZoomeye)
					setEnableCensys(hasCensys)
					setEnableShodan(hasShodan)
				}
			} catch (error) {
				console.error('Failed to load space engine config:', error)
			} finally {
				if (!cancelled) {
					setConfigLoading(false)
				}
			}
		}

		const timer = window.setTimeout(() => {
			void loadSpaceConfig()
		}, 0)

		return () => {
			cancelled = true
			window.clearTimeout(timer)
		}
	}, [])

	const loadPortScanGroups = useCallback(async () => {
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
	}, [])

	useEffect(() => {
		const timer = window.setTimeout(() => {
			void loadPortScanGroups()
		}, 0)

		return () => window.clearTimeout(timer)
	}, [loadPortScanGroups])

	// 当配置加载完成时获取配额
	useEffect(() => {
		if (configLoading) return

		const timer = window.setTimeout(() => {
			void fetchQuotas()
		}, 0)

		return () => window.clearTimeout(timer)
	}, [configLoading, fetchQuotas])

	const fetchHistoryList = useCallback(async () => {
		setHistoryLoading(true)
		try {
			const response = await fetchWithAuth('/api/v1/space/history?page=1&pageSize=50', { cache: 'no-store' })
			const data = await response.json().catch(() => ({}))
			if (!response.ok || !data?.success) {
				throw new Error(data?.error || t.uncoverPage.historyLoadFailed)
			}
			setHistoryItems(Array.isArray(data?.data?.list) ? data.data.list : [])
		} catch (error: any) {
			toast({
				title: t.uncoverPage.historyLoadFailedTitle,
				description: error?.message || t.uncoverPage.historyLoadFailed,
				variant: 'destructive',
			})
		} finally {
			setHistoryLoading(false)
		}
	}, [t])

	const historyGroups = useMemo<HistoryGroup[]>(() => {
		if (historyItems.length === 0) {
			return []
		}

		const sortedItems = [...historyItems].sort((a, b) => {
			return new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()
		})

		const groupedMap = new Map<string, HistoryGroup>()
		const ungroupedItems: SpaceHistoryListItem[] = []

		for (const item of sortedItems) {
			if (!item.groupId) {
				ungroupedItems.push(item)
				continue
			}

			const existingGroup = groupedMap.get(item.groupId)
			if (existingGroup) {
				existingGroup.items.push(item)
				continue
			}

			groupedMap.set(item.groupId, {
				id: `history-group-${item.groupId}`,
				groupId: item.groupId,
				groupName: item.groupName,
				label: item.groupName || t.uncoverPage.historyUnnamedGroup,
				items: [item],
			})
		}

		const groups = Array.from(groupedMap.values()).sort((a, b) => {
			const aTime = new Date(a.items[0]?.createdAt || '').getTime()
			const bTime = new Date(b.items[0]?.createdAt || '').getTime()
			return bTime - aTime
		})

		if (ungroupedItems.length > 0) {
			groups.push({
				id: 'history-group-ungrouped',
				label: t.uncoverPage.historyUngroupedLabel,
				isUngrouped: true,
				items: ungroupedItems,
			})
		}

		return groups
	}, [historyItems, t])

	const availableHistoryGroups = useMemo(() => {
		return historyGroups.filter(group => !group.isUngrouped && group.groupId)
	}, [historyGroups])

	const hasGroupedHistorySelected = useMemo(() => {
		if (selectedHistoryIds.length === 0) {
			return false
		}
		const selectedIdSet = new Set(selectedHistoryIds)
		return historyItems.some(item => selectedIdSet.has(item.id) && !!item.groupId)
	}, [historyItems, selectedHistoryIds])

	const toggleHistorySelection = useCallback((historyId: string, checked: boolean) => {
		setSelectedHistoryIds(prev => checked
			? Array.from(new Set([...prev, historyId]))
			: prev.filter(id => id !== historyId)
		)
	}, [])

	const handleSelectHistoryGroup = useCallback((group: HistoryGroup, checked: boolean) => {
		const groupIds = group.items.map(item => item.id)
		setSelectedHistoryIds(prev => {
			if (checked) {
				return Array.from(new Set([...prev, ...groupIds]))
			}
			const groupIdSet = new Set(groupIds)
			return prev.filter(id => !groupIdSet.has(id))
		})
	}, [])

	const toggleHistoryGroupExpanded = useCallback((groupId: string, open: boolean) => {
		setExpandedHistoryGroupIds(prev => open
			? Array.from(new Set([...prev, groupId]))
			: prev.filter(id => id !== groupId)
		)
	}, [])

	// 插入语法到查询框
	const insertSyntax = (syntax: string) => {
		setQuery(prev => {
			if (prev.trim()) {
				return prev + ' AND ' + syntax
			}
			return syntax
		})
		setShowSyntaxHelper(false)
	}

	const applyQuerySuggestion = useCallback((suggestion: QuerySuggestion) => {
		setQuery(prev => {
			if (suggestion.type === 'operator') {
				const base = prev.trimEnd()
				return base ? `${base} ${suggestion.insertText}` : suggestion.insertText
			}

			const endsWithWhitespace = /\s$/.test(prev)
			if (!prev.trim()) {
				return suggestion.insertText
			}
			if (endsWithWhitespace) {
				return `${prev}${suggestion.insertText}`
			}
			const tokens = prev.split(/\s+/)
			tokens[tokens.length - 1] = suggestion.insertText
			return tokens.join(' ')
		})
		setShowQuerySuggestions(false)
		queryInputRef.current?.focus()
	}, [])

	const handleSearch = async () => {
		if (!query.trim()) {
			toast({
				title: t.uncoverPage.searchInputRequiredTitle,
				description: t.uncoverPage.searchInputRequiredDescription,
				variant: 'destructive',
			})
			return
		}

		// 检查是否至少启用了一个引擎
		if (!enableFofa && !enableHunter && !enableQuake && !enableZoomeye && !enableCensys && !enableShodan) {
			toast({
				title: t.uncoverPage.selectEngineTitle,
				description: t.uncoverPage.selectEngineDescription,
				variant: 'destructive',
			})
			return
		}

		setLoading(true)
		try {
			// 使用新的会话式搜索 API
			const response = await fetchWithAuth(`/api/v1/space/session/search`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: query.trim(),
					type: 'default',
					pageSize: 100,
					option: {
						enableFofa: enableFofa,
						enableHunter: enableHunter,
						enableQuake: enableQuake,
						enableZoomeye: enableZoomeye,
						enableCensys: enableCensys,
						enableShodan: enableShodan,
					},
				}),
			})

			if (!response.ok) {
				const errorData = await response.json().catch(() => ({ error: t.uncoverPage.unknownError }))
				throw new Error(errorData.error || errorData.message || t.uncoverPage.requestFailed)
			}

			const data = await response.json()
			const uniqueResults = dedupeSpaceResults(data.results || [])
			const newTab: SearchTab = {
				id: Date.now().toString(),
				title: query.trim(),
				query: query.trim(),
				results: uniqueResults,
				total: uniqueResults.length,
				currentPage: 1,
				sessionId: data.sessionId,
				hasMore: data.hasMore,
				engineStatus: data.engineStatus,
				reachedMaxLimit: data.reachedMaxLimit,
				maxResults: data.maxResults,
			}

			setTabs(prev => [...prev, newTab])
			setActiveTab(newTab.id)

			toast({
				title: t.uncoverPage.searchCompletedTitle,
				description: formatText(t.uncoverPage.searchCompletedDescription, {
					count: uniqueResults.length,
					suffix: data.hasMore ? t.uncoverPage.searchCompletedHasMoreSuffix : '',
				}),
			})

			if (historyOpen) {
				void fetchHistoryList()
			}

			// 搜索后刷新配额
			fetchQuotas()
		} catch (error: any) {
			toast({
				title: t.uncoverPage.searchFailedTitle,
				description: error.message || t.uncoverPage.unknownError,
				variant: 'destructive',
			})
		} finally {
			setLoading(false)
		}
	}

	const handleRemoveTab = (tabId: string) => {
		const tab = tabs.find(t => t.id === tabId)
		// 关闭会话
		if (tab?.sessionId) {
			fetchWithAuth(`/api/v1/space/session?sessionId=${tab.sessionId}`, {
				method: 'DELETE',
			}).catch(console.error)
		}
		
		const newTabs = tabs.filter(tab => tab.id !== tabId)
		setTabs(newTabs)
		if (activeTab === tabId) {
			setActiveTab(newTabs.length > 0 ? newTabs[0].id : '')
		}
	}

	const mapHistoryResults = useCallback((results: SpaceHistoryDetail['results']): SpaceResult[] => {
		const mapped = (results || []).map(item => ({
			URL: item.url || '',
			IP: item.ip || '',
			Domain: item.domain || '',
			Port: item.port != null ? String(item.port) : '',
			Protocol: item.protocol || '',
			Title: item.title || '',
			Components: Array.isArray(item.components) ? item.components.join(', ') : '',
			ICP: item.icp || '',
			ICPName: item.icpName || '',
			Source: item.source || '',
		}))
		return dedupeSpaceResults(mapped)
	}, [])

	const fetchHistoryDetail = useCallback(async (historyId: string) => {
		const response = await fetchWithAuth(`/api/v1/space/history/${historyId}`, { cache: 'no-store' })
		const data = await response.json().catch(() => ({}))
		if (!response.ok || !data?.success || !data?.data) {
			throw new Error(data?.error || t.uncoverPage.historyOpenFailed)
		}
		return data.data as SpaceHistoryDetail
	}, [t])

	const getMergedResultKey = useCallback((item: SpaceResult) => {
		return getSpaceResultIdentity(item) || [
			item.URL || '',
			item.IP || '',
			item.Domain || '',
			item.Port || '',
			item.Protocol || '',
			item.Title || '',
			item.Source || '',
		].join('||')
	}, [])

	const handleOpenHistory = useCallback(async (historyId: string) => {
		if (!historyId) return
		setHistoryOpeningId(historyId)
		try {
			const detail = await fetchHistoryDetail(historyId)
			const mappedResults = mapHistoryResults(detail.results)
			const nextTab: SearchTab = {
				id: detail.id,
				title: detail.query,
				query: detail.query,
				results: mappedResults,
				total: mappedResults.length,
				currentPage: 1,
				sessionId: detail.sessionId,
				hasMore: detail.hasMore,
				engineStatus: detail.engineStatus || {},
				reachedMaxLimit: detail.reachedMaxLimit,
				maxResults: detail.maxResults,
				historyId: detail.id,
			}

			setTabs(prev => {
				const exists = prev.some(tab => tab.historyId === detail.id)
				if (exists) {
					return prev.map(tab => (tab.historyId === detail.id ? { ...tab, ...nextTab } : tab))
				}
				return [...prev, nextTab]
			})
			setActiveTab(detail.id)
			setQuery(detail.query)
			const enabledEngines = new Set((detail.engines || []).map(name => name.toLowerCase()))
			hasUserInteracted.current = true
			setEnableFofa(enabledEngines.has('fofa'))
			setEnableHunter(enabledEngines.has('hunter'))
			setEnableQuake(enabledEngines.has('quake'))
			setEnableZoomeye(enabledEngines.has('zoomeye'))
			setEnableCensys(enabledEngines.has('censys'))
			setEnableShodan(enabledEngines.has('shodan'))
			setHistoryOpen(false)
		} catch (error: any) {
			toast({
				title: t.uncoverPage.historyOpenFailedTitle,
				description: error?.message || t.uncoverPage.historyOpenFailed,
				variant: 'destructive',
			})
		} finally {
			setHistoryOpeningId('')
		}
	}, [fetchHistoryDetail, mapHistoryResults, t])

	const handleMergeSelectedHistories = useCallback(async () => {
		if (selectedHistoryIds.length < 2) {
			toast({
				title: t.uncoverPage.historyMergeSelectTitle,
				description: t.uncoverPage.historyMergeSelectDescription,
				variant: 'destructive',
			})
			return
		}

		setHistoryMerging(true)
		try {
			const details = await Promise.all(selectedHistoryIds.map(id => fetchHistoryDetail(id)))
			const mergedMap = new Map<string, SpaceResult>()
			const mergedEngineStatus: Record<string, EngineStatus> = {}
			const mergedQueries: string[] = []
			const mergedEngines = new Set<string>()
			let mergedHasMore = false
			let mergedReachedMaxLimit = false
			let mergedMaxResults = 0

			for (const detail of details) {
				mergedQueries.push(detail.query)
				for (const engineName of detail.engines || []) {
					mergedEngines.add(engineName)
				}
				mergedHasMore = mergedHasMore || !!detail.hasMore
				mergedReachedMaxLimit = mergedReachedMaxLimit || !!detail.reachedMaxLimit
				mergedMaxResults = Math.max(mergedMaxResults, detail.maxResults || 0)

				const mappedResults = mapHistoryResults(detail.results)
				for (const item of mappedResults) {
					const key = getMergedResultKey(item)
					const existing = mergedMap.get(key)
					if (!existing) {
						mergedMap.set(key, item)
						continue
					}
					if (!existing.ICP && item.ICP) existing.ICP = item.ICP
					if (!existing.ICPName && item.ICPName) existing.ICPName = item.ICPName
				}

				for (const [engineName, status] of Object.entries(detail.engineStatus || {})) {
					const current = mergedEngineStatus[engineName]
					if (!current) {
						mergedEngineStatus[engineName] = { ...status }
						continue
					}
					mergedEngineStatus[engineName] = {
						success: current.success || status.success,
						total: Math.max(current.total || 0, status.total || 0),
						count: (current.count || 0) + (status.count || 0),
						message: [current.message, status.message].filter(Boolean).join(' | '),
					}
				}
			}

			const mergedResults = Array.from(mergedMap.values())
			const mergedTitle = formatText(t.uncoverPage.historyMergeTabTitle, { count: selectedHistoryIds.length })
			const mergedQuery = mergedQueries.join(' | ')
			const mergedTab: SearchTab = {
				id: `history-merge-${Date.now()}`,
				title: mergedTitle,
				query: mergedQuery,
				results: mergedResults,
				total: mergedResults.length,
				currentPage: 1,
				hasMore: mergedHasMore,
				engineStatus: mergedEngineStatus,
				reachedMaxLimit: mergedReachedMaxLimit,
				maxResults: mergedMaxResults,
				isMergedHistory: true,
			}

			setTabs(prev => [...prev, mergedTab])
			setActiveTab(mergedTab.id)
			setQuery(mergedTitle)
			const enabledEngines = new Set(Array.from(mergedEngines).map(name => name.toLowerCase()))
			hasUserInteracted.current = true
			setEnableFofa(enabledEngines.has('fofa'))
			setEnableHunter(enabledEngines.has('hunter'))
			setEnableQuake(enabledEngines.has('quake'))
			setEnableZoomeye(enabledEngines.has('zoomeye'))
			setEnableCensys(enabledEngines.has('censys'))
			setEnableShodan(enabledEngines.has('shodan'))
			setSelectedHistoryIds([])
			setHistoryOpen(false)

			toast({
				title: t.uncoverPage.historyMergeSuccessTitle,
				description: formatText(t.uncoverPage.historyMergeSuccessDescription, {
					count: selectedHistoryIds.length,
					total: mergedResults.length,
				}),
			})
		} catch (error: any) {
			toast({
				title: t.uncoverPage.historyMergeFailedTitle,
				description: error?.message || t.uncoverPage.historyMergeFailed,
				variant: 'destructive',
			})
		} finally {
			setHistoryMerging(false)
		}
	}, [fetchHistoryDetail, formatText, getMergedResultKey, mapHistoryResults, selectedHistoryIds, t])

	const assignHistoryGroup = useCallback(async (payload: { groupId?: string; groupName?: string; clearGroup?: boolean }) => {
		if (selectedHistoryIds.length === 0) {
			toast({
				title: t.uncoverPage.historyGroupSelectTitle,
				description: t.uncoverPage.historyGroupSelectDescription,
				variant: 'destructive',
			})
			return false
		}

		setHistoryGrouping(true)
		try {
			const response = await fetchWithAuth('/api/v1/space/history/group', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					historyIds: selectedHistoryIds,
					groupId: payload.groupId || '',
					groupName: payload.groupName || '',
					clearGroup: !!payload.clearGroup,
				}),
			})
			const data = await response.json().catch(() => ({}))
			if (!response.ok || !data?.success) {
				throw new Error(data?.error || t.uncoverPage.historyGroupFailed)
			}
			await fetchHistoryList()
			setHistoryGroupDialogOpen(false)
			setHistoryGroupNameInput('')
			toast({
				title: payload.clearGroup ? t.uncoverPage.historyUngroupSuccessTitle : t.uncoverPage.historyGroupSuccessTitle,
				description: payload.clearGroup
					? t.uncoverPage.historyUngroupSuccessDescription
					: formatText(t.uncoverPage.historyGroupSuccessDescription, { count: selectedHistoryIds.length }),
			})
			return true
		} catch (error: any) {
			toast({
				title: t.uncoverPage.historyGroupFailedTitle,
				description: error?.message || t.uncoverPage.historyGroupFailed,
				variant: 'destructive',
			})
			return false
		} finally {
			setHistoryGrouping(false)
		}
	}, [fetchHistoryList, formatText, selectedHistoryIds, t])

	const handleCreateHistoryGroup = useCallback(async () => {
		const groupName = historyGroupNameInput.trim()
		if (!groupName) {
			toast({
				title: t.uncoverPage.historyGroupNameRequiredTitle,
				description: t.uncoverPage.historyGroupNameRequiredDescription,
				variant: 'destructive',
			})
			return
		}
		await assignHistoryGroup({ groupName })
	}, [assignHistoryGroup, historyGroupNameInput, t])

	const handleRenameHistoryGroup = useCallback(async () => {
		const groupName = renameHistoryGroupName.trim()
		if (!renameHistoryGroupId) {
			return
		}
		if (!groupName) {
			toast({
				title: t.uncoverPage.historyGroupNameRequiredTitle,
				description: t.uncoverPage.historyGroupNameRequiredDescription,
				variant: 'destructive',
			})
			return
		}

		setHistoryGroupRenaming(true)
		try {
			const response = await fetchWithAuth(`/api/v1/space/history/group/${renameHistoryGroupId}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ name: groupName }),
			})
			const data = await response.json().catch(() => ({}))
			if (!response.ok || !data?.success) {
				throw new Error(data?.error || t.uncoverPage.historyRenameGroupFailed)
			}
			await fetchHistoryList()
			setRenameHistoryGroupDialogOpen(false)
			setRenameHistoryGroupId('')
			setRenameHistoryGroupName('')
			toast({
				title: t.uncoverPage.historyRenameGroupSuccessTitle,
				description: t.uncoverPage.historyRenameGroupSuccessDescription,
			})
		} catch (error: any) {
			toast({
				title: t.uncoverPage.historyRenameGroupFailedTitle,
				description: error?.message || t.uncoverPage.historyRenameGroupFailed,
				variant: 'destructive',
			})
		} finally {
			setHistoryGroupRenaming(false)
		}
	}, [fetchHistoryList, renameHistoryGroupId, renameHistoryGroupName, t])

	const handleDeleteHistory = useCallback(async (item: SpaceHistoryListItem) => {
		setHistoryDeletingId(item.id)
		try {
			const response = await fetchWithAuth(`/api/v1/space/history/${item.id}`, {
				method: 'DELETE',
			})
			const data = await response.json().catch(() => ({}))
			if (!response.ok || !data?.success) {
				throw new Error(data?.error || t.uncoverPage.historyDeleteFailed)
			}
			setHistoryItems(prev => prev.filter(history => history.id !== item.id))
			setSelectedHistoryIds(prev => prev.filter(id => id !== item.id))
			setTabs(prev => {
				const nextTabs = prev.filter(tab => tab.historyId !== item.id)
				setActiveTab(currentActiveTab => {
					if (!prev.some(tab => tab.historyId === item.id && tab.id === currentActiveTab)) {
						return currentActiveTab
					}
					return nextTabs[0]?.id || ''
				})
				return nextTabs
			})
			toast({
				title: t.uncoverPage.historyDeleteSuccessTitle,
				description: t.uncoverPage.historyDeleteSuccess,
			})
			setConfirmDeleteHistoryId(current => (current === item.id ? null : current))
		} catch (error: any) {
			toast({
				title: t.uncoverPage.historyDeleteFailedTitle,
				description: error?.message || t.uncoverPage.historyDeleteFailed,
				variant: 'destructive',
			})
		} finally {
			setHistoryDeletingId('')
		}
	}, [t])

	useEffect(() => {
		if (!historyOpen) return

		const timer = window.setTimeout(() => {
			void fetchHistoryList()
		}, 0)

		return () => window.clearTimeout(timer)
	}, [fetchHistoryList, historyOpen])

	// 加载更多数据
	const handleLoadMore = async (tabId: string) => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab?.sessionId || !tab.hasMore || tab.loadingMore) return

		// 设置加载状态
		setTabs(prev => prev.map(t => 
			t.id === tabId ? { ...t, loadingMore: true } : t
		))

		try {
			const response = await fetchWithAuth(`/api/v1/space/session/more`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					sessionId: tab.sessionId,
					pageSize: 100,
				}),
			})

			if (!response.ok) {
				throw new Error(t.uncoverPage.loadMoreFailed)
			}

			const data = await response.json()
			const nextResults = dedupeSpaceResults([...tab.results, ...(data.results || [])])

			setTabs(prev => prev.map(t => {
				if (t.id === tabId) {
					return {
						...t,
						results: nextResults,
						total: nextResults.length,
						hasMore: data.hasMore,
						engineStatus: data.engineStatus,
						loadingMore: false,
						reachedMaxLimit: data.reachedMaxLimit,
						maxResults: data.maxResults,
					}
				}
				return t
			}))

			// 检查是否达到内存限制
			if (data.reachedMaxLimit) {
				toast({
					title: t.uncoverPage.cacheLimitTitle,
					description: formatText(t.uncoverPage.cacheLimitDescription, { count: data.maxResults }),
					variant: 'destructive',
				})
			} else {
				toast({
					title: t.uncoverPage.loadCompletedTitle,
					description: formatText(t.uncoverPage.loadCompletedDescription, {
						added: Math.max(0, nextResults.length - tab.results.length),
						total: nextResults.length,
					}),
				})
			}

			if (historyOpen) {
				void fetchHistoryList()
			}

			// 加载后刷新配额
			fetchQuotas()
		} catch (error: any) {
			setTabs(prev => prev.map(t => 
				t.id === tabId ? { ...t, loadingMore: false } : t
			))
			toast({
				title: t.uncoverPage.searchFailedTitle,
				description: error.message,
				variant: 'destructive',
			})
		}
	}

	const getSpaceResultRowKey = useCallback((row: SpaceResult, index: number) => {
		return `${row.IP || row.Domain || row.URL || 'row'}-${row.Port || '0'}-${row.Source || 'unknown'}-${index}`
	}, [])

	const handleSelectRows = useCallback((tabId: string, selectedRowKeys: (string | number)[]) => {
		setTabs(prev =>
			prev.map(tab => (tab.id === tabId ? { ...tab, selectedRowKeys } : tab))
		)
	}, [])

	const getSelectedResults = useCallback((tab: SearchTab) => {
		const selectedKeySet = new Set(tab.selectedRowKeys || [])
		if (selectedKeySet.size === 0) {
			return []
		}
		return tab.results.filter((row, index) => selectedKeySet.has(getSpaceResultRowKey(row, index)))
	}, [getSpaceResultRowKey])

	const buildWebscanUrl = useCallback((item: SpaceResult) => {
		if (item.URL) {
			return item.URL
		}
		const host = item.Domain || item.IP
		if (!host) {
			return ''
		}
		const protocol = item.Protocol?.toLowerCase() === 'https' || String(item.Port) === '443' ? 'https' : 'http'
		const port = Number(item.Port)
		const portPart = port && port !== 80 && port !== 443 ? `:${port}` : ''
		return `${protocol}://${host}${portPart}`
	}, [])

	const buildCrackTarget = useCallback((item: SpaceResult) => {
		const host = item.IP || item.Domain
		if (!host || !item.Port) {
			return ''
		}
		const protocol = item.Protocol?.toLowerCase() || 'tcp'
		return `${protocol}://${host}:${item.Port}`
	}, [])

	const buildPortscanTarget = useCallback((item: SpaceResult) => {
		const rawHost = String(item.IP || item.Domain || '').trim().replace(/^https?:\/\//i, '')
		if (!rawHost) {
			return ''
		}
		return rawHost.split(':')[0] || rawHost
	}, [])

	const handleSaveToAssetCenter = useCallback(async (targets: SpaceResult[]) => {
		if (!targets.length) {
			hotToast.error(t.common.saveToAssetCenterEmpty)
			return
		}
		const toastId = hotToast.loading(t.common.savingToAssetCenter)
		try {
			const result = await saveSpaceEngineAssets(
				'uncover',
				targets.map(item => ({
					ip: item.IP,
					domain: item.Domain,
					url: item.URL,
					title: item.Title,
					port: item.Port,
					protocol: item.Protocol,
					product: item.Components,
				})),
			)
			if (!result.saved) {
				hotToast.error(t.common.saveToAssetCenterEmpty, { id: toastId })
				return
			}
			hotToast.success(t.common.saveToAssetCenterSuccess.replace('{count}', String(result.saved)), { id: toastId })
		} catch (error: any) {
			hotToast.error(`${t.common.saveToAssetCenterFailed}: ${error?.message || t.common.error}`, { id: toastId })
		}
	}, [t])

	const handleSendToWebscan = useCallback(async (targets: SpaceResult[]) => {
		if (!targets.length) {
			toast({
				title: t.uncoverPage.selectTargetTitle,
				description: t.uncoverPage.selectTargetDescription,
				variant: 'destructive',
			})
			return
		}

		const urls = Array.from(new Set(targets.map(buildWebscanUrl).filter(Boolean)))
		if (urls.length === 0) {
			toast({
				title: t.uncoverPage.noAvailableTargetTitle,
				description: t.uncoverPage.noAvailableUrlDescription,
				variant: 'destructive',
			})
			return
		}

		try {
			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'webscan',
					taskName: `uncover-webscan-${new Date().toLocaleString('zh-CN')}`,
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
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || t.uncoverPage.createTaskFailed)
			}
			const data = await response.json()
			if (!data?.success) {
				throw new Error(data?.error || t.uncoverPage.createTaskFailed)
			}
			toast({
				title: t.uncoverPage.sendSuccess,
				description: formatText(t.uncoverPage.sendWebscanSuccess, { count: urls.length }),
			})
		} catch (error: any) {
			toast({
				title: t.uncoverPage.sendFailed,
				description: error?.message || t.uncoverPage.sendWebscanFailed,
				variant: 'destructive',
			})
		}
	}, [buildWebscanUrl, formatText, t])

	const handleSendToCrack = useCallback(async (targets: SpaceResult[]) => {
		if (!targets.length) {
			toast({
				title: t.uncoverPage.selectTargetTitle,
				description: t.uncoverPage.selectTargetDescription,
				variant: 'destructive',
			})
			return
		}

		const addresses = Array.from(new Set(targets.map(buildCrackTarget).filter(Boolean)))
		if (addresses.length === 0) {
			toast({
				title: t.uncoverPage.noAvailableTargetTitle,
				description: t.uncoverPage.noAvailableAddressDescription,
				variant: 'destructive',
			})
			return
		}

		try {
			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'crack',
					taskName: `uncover-crack-${new Date().toLocaleString('zh-CN')}`,
					config: {
						targets: addresses,
						threads: 10,
						username: '',
						password: '',
						defaultDict: true,
					},
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || t.uncoverPage.createTaskFailed)
			}
			const data = await response.json()
			if (!data?.success) {
				throw new Error(data?.error || t.uncoverPage.createTaskFailed)
			}
			toast({
				title: t.uncoverPage.sendSuccess,
				description: formatText(t.uncoverPage.sendCrackSuccess, { count: addresses.length }),
			})
		} catch (error: any) {
			toast({
				title: t.uncoverPage.sendFailed,
				description: error?.message || t.uncoverPage.sendCrackFailed,
				variant: 'destructive',
			})
		}
	}, [buildCrackTarget, formatText, t])

	const handleSendToPortscan = useCallback(async (targets: SpaceResult[], portGroup: PortGroupOption) => {
		if (!targets.length) {
			toast({
				title: t.uncoverPage.selectTargetTitle,
				description: t.uncoverPage.selectTargetDescription,
				variant: 'destructive',
			})
			return
		}

		const portscanTargets = Array.from(new Set(targets.map(buildPortscanTarget).filter(Boolean)))
		if (portscanTargets.length === 0) {
			toast({
				title: t.uncoverPage.noAvailableTargetTitle,
				description: t.uncoverPage.noAvailableHostDescription,
				variant: 'destructive',
			})
			return
		}

		try {
			const response = await fetchWithAuth('/api/v1/webscan/scan', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'portscan',
					taskName: `${portscanTargets[0]}-${portGroup.text}`,
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
				throw new Error(errorData.error || t.uncoverPage.createTaskFailed)
			}
			const data = await response.json()
			if (!data?.success) {
				throw new Error(data?.error || t.uncoverPage.createTaskFailed)
			}
			toast({
				title: t.uncoverPage.sendSuccess,
				description: formatText(t.uncoverPage.sendPortscanSuccess, { group: portGroup.text, count: portscanTargets.length }),
			})
		} catch (error: any) {
			toast({
				title: t.uncoverPage.sendFailed,
				description: error?.message || t.uncoverPage.sendPortscanFailed,
				variant: 'destructive',
			})
		}
	}, [buildPortscanTarget, formatText, t])

	// 分页相关函数
	const PAGE_SIZE = 100

	const handlePageChange = (tabId: string, page: number) => {
		setTabs(prev =>
			prev.map(tab => (tab.id === tabId ? { ...tab, currentPage: page } : tab))
		)
	}

	const getPaginatedResults = (results: SpaceResult[], currentPage: number) => {
		const startIndex = (currentPage - 1) * PAGE_SIZE
		const endIndex = startIndex + PAGE_SIZE
		return results.slice(startIndex, endIndex)
	}

	const getTotalPages = (total: number) => {
		return Math.ceil(total / PAGE_SIZE)
	}

	const getEngineDisplayName = (name: string) => {
		const mapping: Record<string, string> = {
			fofa: 'FOFA',
			hunter: 'Hunter',
			quake: 'Quake',
			zoomeye: 'ZoomEye',
			censys: 'Censys',
			shodan: 'Shodan',
		}
		return mapping[name.toLowerCase()] || name
	}

	const getEngineStatusBadgeText = useCallback((tab: SearchTab, name: string, status: EngineStatus) => {
		const engineLabel = getEngineDisplayName(name)
		if (tab.isMergedHistory) {
			return `${engineLabel}: ${status.count || 0}`
		}
		return `${engineLabel}: ${status.count}/${status.total ?? '?'}`
	}, [])

	const getQuotaDisplay = useCallback((engineKey: string) => {
		const quota = quotas.find(item => item.engine === engineKey)
		if (!quota) {
			return {
				text: t.uncoverPage.engineQuotaUnavailable,
				tone: 'muted' as const,
			}
		}
		if (quota.error) {
			return {
				text: quota.error === '解析响应失败' ? t.uncoverPage.quotaFetchFailed : t.uncoverPage.quotaError,
				tone: 'danger' as const,
			}
		}

		let details: string[] = []
		let isLow = false

		if (engineKey === 'fofa') {
			if (quota.fofaPoint != null) details.push(formatText(t.uncoverPage.fofaPointUnit, { count: quota.fofaPoint }))
			if (quota.fcoin != null) details.push(formatText(t.uncoverPage.fofaCoinUnit, { count: quota.fcoin }))
			if (quota.remainFreePoint != null) details.push(formatText(t.uncoverPage.freeLabel, { count: quota.remainFreePoint }))
			isLow = (quota.fofaPoint ?? 0) <= 0 && (quota.fcoin ?? 0) <= 0 && (quota.remainFreePoint ?? 0) <= 0
		} else if (engineKey === 'hunter') {
			if (quota.restEquityPoint != null) details.push(formatText(t.uncoverPage.equityLabel, { count: quota.restEquityPoint }))
			if (quota.restFreePoint != null) details.push(`${formatText(t.uncoverPage.freeLabel, { count: quota.restFreePoint })}/${quota.dayFreePoint ?? '?'}`)
			isLow = (quota.restEquityPoint ?? 0) <= 0 && (quota.restFreePoint ?? 0) <= 0
		} else if (engineKey === 'quake') {
			if (quota.credit != null) details.push(`${quota.credit} Credit`)
			if (quota.persistentCredit != null && quota.persistentCredit > 0) details.push(formatText(t.uncoverPage.persistentLabel, { count: quota.persistentCredit }))
			isLow = (quota.credit ?? 0) <= 0
		} else if (engineKey === 'zoomeye') {
			if (quota.points != null) details.push(formatText(t.uncoverPage.commonLabel, { count: quota.points }))
			if (quota.zoomeyePoints != null) details.push(formatText(t.uncoverPage.equityLabel, { count: quota.zoomeyePoints }))
			isLow = (quota.points ?? 0) <= 0 && (quota.zoomeyePoints ?? 0) <= 0
		} else if (engineKey === 'censys') {
			details.push(t.uncoverPage.configured)
		} else if (engineKey === 'shodan') {
			if (quota.plan) details.push(quota.plan)
			if (quota.queryCredits != null) details.push(formatText(t.uncoverPage.queryCreditUnit, { count: quota.queryCredits }))
			if (quota.scanCredits != null) details.push(formatText(t.uncoverPage.scanCreditUnit, { count: quota.scanCredits }))
			isLow = (quota.queryCredits ?? 0) <= 0
		}

		return {
			text: details.length > 0 ? details.join(' / ') : t.uncoverPage.engineQuotaUnavailable,
			tone: isLow ? ('danger' as const) : ('normal' as const),
		}
	}, [formatText, quotas, t])

	// 定义表格列
	const columns: ColumnDef<SpaceResult>[] = [
		{
			id: 'IP',
			header: t.uncoverPage.table.ipPort,
			cell: (row) => (
				<div className="flex items-center gap-2">
					<span className="font-mono font-semibold text-primary">
						{row.IP}:{row.Port}
					</span>
				</div>
			),
		},
		{
			id: 'URL',
			header: t.uncoverPage.table.url,
			cell: (row) => (
				<div className="max-w-[300px] truncate" title={String(row.URL || '')}>
					<span className="font-mono text-xs">{String(row.URL || '')}</span>
				</div>
			),
		},
		{
			id: 'Domain',
			header: t.uncoverPage.table.domain,
			cell: (row) => (
				<div className="max-w-[200px] truncate" title={String(row.Domain || '')}>
					<span className="font-mono text-xs">{String(row.Domain || '')}</span>
				</div>
			),
		},
		{
			id: 'ICP',
			header: t.uncoverPage.table.icp,
			cell: (row) => {
				const icp = String(row.ICP || '')
				if (!icp.trim()) {
					return <span className="text-muted-foreground text-xs">-</span>
				}
				return (
					<div className="max-w-[200px] truncate" title={icp}>
						<span className="font-mono text-xs">{icp}</span>
					</div>
				)
			},
		},
		{
			id: 'ICPName',
			header: t.uncoverPage.table.icpName,
			cell: (row) => {
				const icpName = String(row.ICPName || '')
				if (!icpName.trim()) {
					return <span className="text-muted-foreground text-xs">-</span>
				}
				return (
					<div className="max-w-[200px] truncate" title={icpName}>
						<span className="text-xs">{icpName}</span>
					</div>
				)
			},
		},
		{
			id: 'Protocol',
			header: t.uncoverPage.table.protocol,
			cell: (row) => (
				<Badge variant="secondary" className="font-mono text-xs">
					{String(row.Protocol || '')}
				</Badge>
			),
		},
		{
			id: 'Title',
			header: t.uncoverPage.table.title,
			cell: (row) => (
				<div className="max-w-[250px] truncate" title={String(row.Title || '')}>
					<span className="text-xs">{String(row.Title || '')}</span>
				</div>
			),
		},
		{
			id: 'Components',
			header: t.uncoverPage.table.components,
			cell: (row) => {
				const components = String(row.Components || '')
				if (!components || !components.trim()) {
					return <span className="text-muted-foreground text-xs">-</span>
				}
				const componentList = components.split(',').map(c => c.trim()).filter(Boolean)
				if (componentList.length === 0) {
					return <span className="text-muted-foreground text-xs">-</span>
				}
				return (
					<div className="flex items-center gap-1.5 flex-wrap">
						<Badge variant="secondary" className="font-mono text-xs h-5">
							{componentList[0]}
						</Badge>
						{componentList.length > 1 && (
							<Popover>
								<PopoverTrigger asChild>
									<Button variant="ghost" size="sm" className="h-5 text-xs px-2">
										{formatText(t.uncoverPage.table.componentCount, { count: componentList.length })}
									</Button>
								</PopoverTrigger>
								<PopoverContent className="w-[350px]">
									<div className="flex flex-col gap-2">
										{componentList.map((comp, idx) => (
											<Badge key={idx} variant="outline" className="w-full justify-start">
												{comp}
											</Badge>
										))}
									</div>
								</PopoverContent>
							</Popover>
						)}
					</div>
				)
			},
		},
		{
			id: 'Source',
			header: t.uncoverPage.table.source,
			cell: (row) => (
				<Badge variant="outline" className="text-xs">
					{String(row.Source || '')}
				</Badge>
			),
		},
	]

	const openURL = (url: string) => {
		if (url) {
			window.open(url, '_blank')
		}
	}

	const CsegmentIpv4 = (ip: string): string => {
		if (!ip) return ''
		const parts = ip.split('.')
		if (parts.length === 4) {
			return `${parts[0]}.${parts[1]}.${parts[2]}.0/24`
		}
		return ip
	}

	// 导出数据功能
	const exportData = (data: SpaceResult[], format: 'csv' | 'json' | 'txt' = 'csv') => {
		if (data.length === 0) {
			toast({
				title: t.uncoverPage.noExportDataTitle,
				description: t.uncoverPage.noExportDataDescription,
				variant: 'destructive',
			})
			return
		}

		let content: string
		let filename: string
		let mimeType: string

		const headers = ['URL', 'IP', t.uncoverPage.table.domain, 'Port', t.uncoverPage.table.protocol, t.uncoverPage.table.title, t.uncoverPage.table.components, t.uncoverPage.table.icp, t.uncoverPage.table.icpName, t.uncoverPage.table.source]
		const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, -5)

		if (format === 'csv') {
			const csvRows = [
				headers.join(','),
				...data.map(row => [
					row.URL || '',
					row.IP || '',
					row.Domain || '',
					row.Port || '',
					row.Protocol || '',
					row.Title || '',
					row.Components || '',
					row.ICP || '',
					row.ICPName || '',
					row.Source || '',
				].map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(','))
			]
			content = '\ufeff' + csvRows.join('\n')
			filename = `space-aggregation_${timestamp}.csv`
			mimeType = 'text/csv;charset=utf-8;'
		} else if (format === 'json') {
			content = JSON.stringify(data, null, 2)
			filename = `space-aggregation_${timestamp}.json`
			mimeType = 'application/json'
		} else {
			// txt format - 只导出URL
			content = data.map(row => row.URL).filter(Boolean).join('\n')
			filename = `space-aggregation_${timestamp}.txt`
			mimeType = 'text/plain'
		}

		const blob = new Blob([content], { type: mimeType })
		const link = document.createElement('a')
		const url = URL.createObjectURL(blob)
		link.href = url
		link.download = filename
		link.style.visibility = 'hidden'
		document.body.appendChild(link)
		link.click()
		document.body.removeChild(link)
		URL.revokeObjectURL(url)

		toast({
			title: t.uncoverPage.exportSuccessTitle,
			description: formatText(t.uncoverPage.exportSuccessDescription, { count: data.length, format: format.toUpperCase() }),
		})
	}


	// 计算统计数据
	const calculateStats = (results: SpaceResult[]): SearchStats => {
		const sourceStats: Record<string, number> = {}
		const protocolStats: Record<string, number> = {}
		const portStats: Record<string, number> = {}
		const componentStats: Record<string, number> = {}
		const uniqueIPs = new Set<string>()
		const uniqueDomains = new Set<string>()

		results.forEach(result => {
			// 来源统计
			sourceStats[result.Source] = (sourceStats[result.Source] || 0) + 1

			// 协议统计
			if (result.Protocol) {
				protocolStats[result.Protocol] = (protocolStats[result.Protocol] || 0) + 1
			}

			// 端口统计
			if (result.Port) {
				portStats[result.Port] = (portStats[result.Port] || 0) + 1
			}

			// 组件统计
			if (result.Components) {
				result.Components.split(',').forEach(comp => {
					const trimmed = comp.trim()
					if (trimmed) {
						componentStats[trimmed] = (componentStats[trimmed] || 0) + 1
					}
				})
			}

			// 唯一IP和域名
			if (result.IP) uniqueIPs.add(result.IP)
			if (result.Domain) uniqueDomains.add(result.Domain)
		})

		return {
			sourceStats: Object.entries(sourceStats)
				.map(([name, count]) => ({ name, count }))
				.sort((a, b) => b.count - a.count),
			protocolStats: Object.entries(protocolStats)
				.map(([name, count]) => ({ name, count }))
				.sort((a, b) => b.count - a.count)
				.slice(0, 10),
			portStats: Object.entries(portStats)
				.map(([name, count]) => ({ name, count }))
				.sort((a, b) => b.count - a.count)
				.slice(0, 10),
			componentStats: Object.entries(componentStats)
				.map(([name, count]) => ({ name, count }))
				.sort((a, b) => b.count - a.count)
				.slice(0, 10),
			uniqueIPs: uniqueIPs.size,
			uniqueDomains: uniqueDomains.size,
		}
	}

	// C段查询功能 - 直接执行搜索
	const handleCSegmentQuery = async (ip: string) => {
		const csegment = CsegmentIpv4(ip)
		if (!csegment || csegment === ip) {
			toast({
				title: t.uncoverPage.invalidIpTitle,
				description: t.uncoverPage.invalidIpDescription,
				variant: 'destructive',
			})
			return
		}

		// 使用统一语法格式
		setQuery(`ip:${csegment}`)
		setTimeout(() => {
			handleSearch()
		}, 100)
	}

	const hasResultsView = loading || tabs.length > 0
	const isCompactSearch = hasResultsView

	return (
		<div className="flex-1 overflow-y-auto">
			<div className={`p-6 ${hasResultsView ? 'space-y-6' : ''}`}>
				<div className={hasResultsView ? '' : 'flex min-h-[calc(100vh-140px)] items-center justify-center'}>
					<form
						onSubmit={e => {
							e.preventDefault()
							handleSearch()
						}}
						className={hasResultsView ? '' : 'w-full'}
					>
						<div className={hasResultsView ? 'px-6 py-8 sm:px-10 sm:py-10' : 'px-0 py-0'}>
							<div className={`mx-auto ${isCompactSearch ? 'max-w-5xl space-y-4' : 'max-w-5xl space-y-6'}`}>
								<div className="space-y-4">
									<div
										className={`group relative overflow-visible border bg-background transition-colors ${
											showQuerySuggestions && querySuggestions.length > 0
												? 'rounded-t-2xl rounded-b-none border-primary/60 border-b-0'
												: 'rounded-2xl border-border/80 hover:border-primary/40 focus-within:border-primary/60'
										}`}
									>
										<div
											className={`relative overflow-hidden ${
												isCompactSearch ? 'h-12' : 'h-16'
											} ${
												showQuerySuggestions && querySuggestions.length > 0
													? 'rounded-t-[inherit] rounded-b-none border-b border-primary/20'
													: 'rounded-[inherit]'
											}`}
										>
											<Input
												ref={queryInputRef}
												value={query}
												onChange={e => {
													setQuery(e.target.value)
													setShowQuerySuggestions(true)
												}}
												onFocus={() => {
													if (querySuggestions.length > 0) {
														setShowQuerySuggestions(true)
													}
												}}
												onBlur={() => {
													window.setTimeout(() => setShowQuerySuggestions(false), 120)
												}}
												placeholder={t.uncoverPage.searchPlaceholder}
												className={`${isCompactSearch ? 'h-12 pl-4 pr-28 text-sm' : 'h-16 pl-5 pr-36 text-base'} border-0 bg-transparent font-mono shadow-none focus-visible:ring-0 focus-visible:ring-offset-0 ${
													showQuerySuggestions && querySuggestions.length > 0 ? 'rounded-b-none rounded-t-[inherit]' : 'rounded-[inherit]'
												}`}
												onCompositionStart={() => { isComposing.current = true }}
												onCompositionEnd={() => { isComposing.current = false }}
												onKeyDown={e => {
													if (showQuerySuggestions && querySuggestions.length > 0) {
														if (e.key === 'ArrowDown') {
															e.preventDefault()
															setHighlightedSuggestionIndex(prev => (prev + 1) % querySuggestions.length)
															return
														}
														if (e.key === 'ArrowUp') {
															e.preventDefault()
															setHighlightedSuggestionIndex(prev => (prev - 1 + querySuggestions.length) % querySuggestions.length)
															return
														}
														if (e.key === 'Tab' || e.key === 'Enter') {
															if (e.key === 'Enter' && !e.shiftKey && !isComposing.current) {
																e.preventDefault()
																applyQuerySuggestion(querySuggestions[highlightedSuggestionIndex])
																return
															}
															if (e.key === 'Tab') {
																e.preventDefault()
																applyQuerySuggestion(querySuggestions[highlightedSuggestionIndex])
																return
															}
														}
														if (e.key === 'Escape') {
															setShowQuerySuggestions(false)
															return
														}
													}
													if (e.key === 'Enter' && !e.shiftKey && !isComposing.current) {
														e.preventDefault()
														handleSearch()
													}
												}}
											/>
											<Button
												type="submit"
												disabled={loading}
												variant="ghost"
												className={`absolute right-2 top-1/2 -translate-y-1/2 rounded-xl border-0 bg-transparent text-foreground shadow-none hover:bg-transparent hover:text-primary ${
													isCompactSearch ? 'h-9 px-3 text-sm' : 'h-11 px-4'
												}`}
											>
												{loading ? (
													<>
														<Loader2 className="mr-2 h-4 w-4 animate-spin" />
														{t.uncoverPage.searching}
													</>
												) : (
													<>
														<Search className="mr-2 h-4 w-4" />
														{t.uncoverPage.search}
													</>
												)}
											</Button>
										</div>
										{showQuerySuggestions && querySuggestions.length > 0 && (
											<div className="absolute -left-px -right-px top-full z-30 overflow-hidden rounded-b-2xl border border-primary/60 bg-popover shadow-lg">
												<div className="flex items-center justify-between overflow-hidden border-b border-border/60 bg-muted/30 px-4 py-2">
													<span className="text-[11px] font-medium uppercase tracking-[0.16em] text-muted-foreground">
														{t.uncoverPage.quickTipsLabel}
													</span>
													<span className="text-[11px] text-muted-foreground">
														{querySuggestions.length} 条建议
													</span>
												</div>
												<div className="max-h-72 overflow-y-auto py-1.5">
													{querySuggestions.map((suggestion, index) => (
														<button
															key={suggestion.key}
															type="button"
															className={`flex w-full items-start justify-between gap-3 px-4 py-3 text-left transition-colors ${
																index === highlightedSuggestionIndex ? 'bg-muted/80' : 'hover:bg-muted/50'
															}`}
															onMouseEnter={() => setHighlightedSuggestionIndex(index)}
															onMouseDown={(e) => {
																e.preventDefault()
																applyQuerySuggestion(suggestion)
															}}
														>
															<div className="min-w-0 space-y-1">
																<div className="flex items-center gap-2">
																	<Badge variant="outline" className="h-5 border-border/70 bg-background px-1.5 text-[10px] uppercase">
																		{suggestion.type === 'operator' ? t.uncoverPage.suggestionOperator : t.uncoverPage.suggestionField}
																	</Badge>
																	<code className="font-mono text-xs text-primary">
																		{suggestion.type === 'operator' ? suggestion.insertText.trim() : suggestion.insertText}
																	</code>
																</div>
																<p className="text-xs text-muted-foreground">{suggestion.description}</p>
															</div>
															{suggestion.type === 'field' && (
																<span className="shrink-0 text-[11px] text-muted-foreground">{suggestion.label}</span>
															)}
														</button>
													))}
												</div>
											</div>
										)}
									</div>

									{!isCompactSearch && (
										<div className="flex flex-wrap items-center justify-center gap-2">
											<span className="text-xs text-muted-foreground">{t.uncoverPage.quickTipsLabel}</span>
											{quickSearchTips.map(tip => (
												<Button
													key={tip}
													type="button"
													variant="outline"
													size="sm"
													className="h-8 rounded-full bg-background px-3 text-xs font-mono"
													onClick={() => setQuery(tip)}
												>
													{tip}
												</Button>
											))}
										</div>
									)}

									<div className={`flex flex-wrap items-center gap-2 ${isCompactSearch ? 'justify-end' : 'justify-center'}`}>
										<Popover open={showSyntaxHelper} onOpenChange={setShowSyntaxHelper}>
											<PopoverTrigger asChild>
												<Button
													type="button"
													variant="ghost"
													size="sm"
													className={`rounded-full px-3 text-xs text-muted-foreground hover:text-foreground ${
														isCompactSearch ? 'h-7' : 'h-8'
													}`}
													title={t.uncoverPage.syntaxHelperTitle}
												>
													<AlertCircle className="mr-1.5 h-3.5 w-3.5" />
													{t.uncoverPage.syntaxTitle}
												</Button>
											</PopoverTrigger>
											<PopoverContent className="w-[600px] max-h-[500px] overflow-y-auto" align="center">
												<div className="space-y-4">
													<div>
														<h3 className="mb-2 text-sm font-semibold">{t.uncoverPage.syntaxTitle}</h3>
														<p className="mb-3 text-xs text-muted-foreground">
															{t.uncoverPage.syntaxDescription}
														</p>
														<div className="space-y-2 rounded-md border border-border/70 bg-muted/30 p-2.5">
															<div className="text-[11px] font-medium text-foreground/90">{t.uncoverPage.fieldSupportTitle}</div>
															<div className="space-y-1.5">
																{localizedFieldSupportTips.map((item) => (
																	<div key={item.engine} className="text-[11px] leading-5">
																		<span className="font-medium text-foreground">{item.engine}：</span>
																		<span className="text-muted-foreground">{item.text}</span>
																	</div>
																))}
															</div>
														</div>
													</div>
													{localizedSyntaxOptions.map((category, idx) => (
														<div key={idx} className="border-b border-border pb-3 last:border-0">
															<h4 className="mb-2 text-xs font-medium text-muted-foreground">{category.category}</h4>
															<div className="grid grid-cols-2 gap-1">
																{category.items.map((item, itemIdx) => (
																	<button
																		key={itemIdx}
																		type="button"
																		className="flex items-center justify-between rounded p-1.5 text-left text-xs transition-colors hover:bg-muted/80"
																		onClick={() => insertSyntax(item.syntax)}
																	>
																		<code className="font-mono text-primary">{item.syntax}</code>
																		<span className="ml-2 truncate text-muted-foreground">{item.description}</span>
																	</button>
																))}
															</div>
														</div>
													))}
												</div>
											</PopoverContent>
										</Popover>
										<Popover open={searchOptionsOpen} onOpenChange={setSearchOptionsOpen}>
											<PopoverTrigger asChild>
												<Button
													type="button"
													variant="ghost"
													size="sm"
													className={`rounded-full px-3 text-xs text-muted-foreground hover:text-foreground ${
														isCompactSearch ? 'h-7' : 'h-8'
													}`}
												>
													{t.uncoverPage.searchOptionsButton}
													<ChevronDown className={`ml-1.5 h-3.5 w-3.5 transition-transform ${searchOptionsOpen ? 'rotate-180' : ''}`} />
												</Button>
											</PopoverTrigger>
											<PopoverContent align="center" className="w-[340px] rounded-2xl border-border/70 p-2">
												<div className="space-y-2">
													<div className="flex items-center justify-between px-2 py-1">
														<div className="text-sm font-medium">{t.uncoverPage.searchOptionsButton}</div>
														<Button
															type="button"
															variant="ghost"
															size="sm"
															className="h-7 rounded-lg px-2 text-xs text-muted-foreground hover:text-foreground"
															onClick={() => fetchQuotas()}
															disabled={quotaLoading}
														>
															<Loader2 className={`mr-1.5 h-3.5 w-3.5 ${quotaLoading ? 'animate-spin' : ''}`} />
															{t.common.refresh}
														</Button>
													</div>
													<div className="space-y-1">
														{engineOptions.map(engine => {
															const Icon = engine.icon
															const quotaDisplay = getQuotaDisplay(engine.key)
															return (
																<button
																	key={engine.key}
																	type="button"
																	disabled={!engine.configured}
																	className={`flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-left transition-colors ${
																		!engine.configured
																			? 'cursor-not-allowed opacity-50'
																			: engine.enabled
																				? 'bg-muted text-foreground'
																				: 'hover:bg-muted/60'
																	}`}
																	onClick={() => {
																		if (!engine.configured) return
																		hasUserInteracted.current = true
																		engine.toggle()
																	}}
																>
																	<div className="flex min-w-0 items-center gap-3">
																		<div className={`flex h-8 w-8 items-center justify-center rounded-lg border ${
																			engine.enabled
																				? 'border-primary/35 bg-primary/10 text-primary'
																				: 'border-border/70 bg-background text-muted-foreground'
																		}`}>
																			<Icon className="h-4 w-4" />
																		</div>
																		<div className="min-w-0">
																			<div className="truncate text-sm font-medium">{engine.label}</div>
																			<div className="truncate text-[11px] text-muted-foreground">
																				{engine.configured ? quotaDisplay.text : t.uncoverPage.engineNotConfigured}
																			</div>
																		</div>
																	</div>
																	<div className={`h-2.5 w-2.5 rounded-full ${
																		!engine.configured
																			? 'bg-border'
																			: engine.enabled
																				? 'bg-primary'
																				: 'bg-muted-foreground/30'
																	}`} />
																</button>
															)
														})}
													</div>
												</div>
											</PopoverContent>
										</Popover>
										<Button
											type="button"
											variant="ghost"
											size="sm"
											className={`rounded-full px-3 text-xs text-muted-foreground hover:text-foreground ${
												isCompactSearch ? 'h-7' : 'h-8'
											}`}
											onClick={() => { setExpandedHistoryGroupIds([]); setHistoryOpen(true) }}
										>
											<History className="mr-1.5 h-3.5 w-3.5" />
											{t.uncoverPage.historyButton}
										</Button>
									</div>
								</div>
							</div>
						</div>
					</form>
				</div>

				{/* 全局加载动画 - 首次搜索或没有tabs时显示 */}
				{loading && tabs.length === 0 ? (
					<div className="flex flex-col items-center justify-center space-y-6 py-12">
						<div className="relative">
							<div className="h-14 w-14 rounded-full border-4 border-primary/15"></div>
							<div className="absolute left-0 top-0 h-14 w-14 animate-spin rounded-full border-4 border-transparent border-t-primary"></div>
						</div>
						<div className="text-center space-y-2">
							<p className="text-base font-medium text-foreground">{t.uncoverPage.loadingTitle}</p>
							<p className="text-sm text-muted-foreground">
								{formatText(t.uncoverPage.loadingFrom, {
									engines: [
										enableFofa && 'FOFA',
										enableHunter && 'Hunter',
										enableQuake && 'Quake',
										enableZoomeye && 'ZoomEye',
										enableCensys && 'Censys',
										enableShodan && 'Shodan',
									].filter(Boolean).join('、'),
								})}
							</p>
						</div>
					</div>
				) : null}

				{tabs.length > 0 ? (
					<Tabs value={activeTab} onValueChange={setActiveTab}>
						<TabsList className="flex w-full justify-start gap-2 overflow-x-auto whitespace-nowrap rounded-xl pr-1 [&>*]:shrink-0">
							{tabs.map(tab => (
								<TabsTrigger
									key={tab.id}
									value={tab.id}
									className="relative !flex-none w-[240px] min-w-[240px] max-w-[240px] items-center justify-between"
								>
									<span className="min-w-0 flex-1 truncate" title={tab.title}>
										{tab.title}
									</span>
									<span
										className="ml-2 inline-flex h-4 w-4 shrink-0 cursor-pointer items-center justify-center rounded-sm hover:bg-accent hover:text-accent-foreground"
										onClick={e => {
											e.stopPropagation()
											handleRemoveTab(tab.id)
										}}
										onKeyDown={e => {
											if (e.key === 'Enter' || e.key === ' ') {
												e.stopPropagation()
												handleRemoveTab(tab.id)
											}
										}}
										role="button"
										tabIndex={0}
										aria-label={t.uncoverPage.closableTabLabel}
									>
										<X className="h-3 w-3" />
									</span>
								</TabsTrigger>
							))}
						</TabsList>
						{tabs.map(tab => {
							const stats = calculateStats(tab.results)
							const paginatedResults = getPaginatedResults(tab.results, tab.currentPage)
							const selectedResults = getSelectedResults(tab)
							const selectedCount = selectedResults.length
							const totalPages = getTotalPages(tab.total)
							return (
								<TabsContent key={tab.id} value={tab.id} className="mt-4">
									{loading && activeTab === tab.id ? (
										<Card className="p-12 bg-card border-border">
											<div className="flex flex-col items-center justify-center space-y-6">
												{/* 旋转加载指示器 */}
												<div className="relative">
													<div className="w-16 h-16 border-4 border-primary/20 rounded-full"></div>
													<div className="absolute top-0 left-0 w-16 h-16 border-4 border-transparent border-t-primary rounded-full animate-spin"></div>
												</div>

												{/* 加载文字 */}
												<div className="text-center space-y-2">
													<p className="text-lg font-medium text-foreground">{t.uncoverPage.loadingTitle}</p>
													<p className="text-sm text-muted-foreground">
														{formatText(t.uncoverPage.loadingFrom, {
															engines: [
																enableFofa && 'FOFA',
																enableHunter && 'Hunter',
																enableQuake && 'Quake',
																enableZoomeye && 'ZoomEye',
																enableCensys && 'Censys',
																enableShodan && 'Shodan',
															].filter(Boolean).join('、'),
														})}
													</p>
												</div>
											</div>
										</Card>
									) : (
										<div className={`grid grid-cols-1 gap-6 items-start ${statsCollapsed ? 'lg:grid-cols-1' : 'lg:grid-cols-4'}`}>
											{!statsCollapsed && (
											<div className="lg:col-span-1">
												<ResultStatsPanel
													stats={stats}
													title={t.uncoverPage.statsTitle}
													sourceStatsLabel={t.uncoverPage.sourceStats}
													portStatsLabel={t.uncoverPage.portStats}
													protocolStatsLabel={t.uncoverPage.protocolStats}
													componentStatsLabel={t.uncoverPage.componentStats}
													onCollapse={() => setStatsCollapsed(true)}
												/>
											</div>
											)}

											<div className={statsCollapsed ? 'lg:col-span-1' : 'lg:col-span-3'}>
												<Card className="p-4 h-full flex flex-col">
													<ResultToolbar
														tab={tab}
														statsCollapsed={statsCollapsed}
														onToggleStats={() => setStatsCollapsed(prev => !prev)}
														onLoadMore={handleLoadMore}
														statsTitle={t.uncoverPage.statsTitle}
														canLoadMoreLabel={t.uncoverPage.canLoadMore}
														cacheLimitReachedLabel={t.uncoverPage.cacheLimitReached}
														queryFailedLabel={t.uncoverPage.queryFailed}
														loadMoreLabel={t.uncoverPage.loadMore}
														loadingMoreLabel={t.uncoverPage.loadingMore}
														getEngineDisplayName={getEngineDisplayName}
														getEngineStatusBadgeText={getEngineStatusBadgeText}
														formatText={formatText}
													/>

													<div className="flex-1">
														<SearchResultTable
															tab={tab}
															loading={loading && activeTab === tab.id}
															paginatedResults={paginatedResults}
															columns={columns}
															selectedResults={selectedResults}
															selectedCount={selectedCount}
															emptyMessage={t.uncoverPage.noResults}
															actionsColumnLabel={t.uncoverPage.actions}
															actionsColumnClassName="text-center bg-card"
															onSelectRows={handleSelectRows}
															onExportData={exportData}
															onSendToWebscan={handleSendToWebscan}
															onSendToCrack={handleSendToCrack}
															onSendToPortscan={handleSendToPortscan}
															onSaveToAssetCenter={handleSaveToAssetCenter}
															portScanGroups={portScanGroups}
															getRowKey={getSpaceResultRowKey}
															sendToLabel={t.uncoverPage.sendTo}
															sendToWebscanLabel={t.uncoverPage.sendToWebscan}
															sendToCrackLabel={t.uncoverPage.sendToCrack}
															sendToPortscanLabel={t.uncoverPage.sendToPortscan}
															saveToAssetCenterLabel={t.common.saveToAssetCenter}
															exportLabel={t.uncoverPage.export}
															exportCurrentCsvLabel={t.uncoverPage.exportCurrentCsv}
															exportCurrentJsonLabel={t.uncoverPage.exportCurrentJson}
															exportCurrentTxtLabel={t.uncoverPage.exportCurrentTxt}
															openLinkTitle={t.uncoverPage.openLink}
															cSegmentQueryTitle={t.uncoverPage.cSegmentQuery}
															onOpenURL={openURL}
															onCSegmentQuery={handleCSegmentQuery}
														/>
													</div>

													{/* 分页组件 */}
													{!loading && tab.results.length > 0 && (
														<div className="mt-4 pt-4 border-t">
															<PaginationPro
																current={tab.currentPage}
																total={tab.total}
																pageSize={100}
																onChange={(page) => handlePageChange(tab.id, page)}
																pageSizeOptions={[100]}
																showTotal
																disabled={loading}
															/>
														</div>
													)}
												</Card>
											</div>
										</div>
									)}
								</TabsContent>
							)
						})}
					</Tabs>
				) : null}

				<Sheet open={historyOpen} onOpenChange={setHistoryOpen}>
					<SheetContent side="right" className="w-full sm:max-w-xl">
						<SheetHeader className="pr-10">
							<SheetTitle>{t.uncoverPage.historySheetTitle}</SheetTitle>
							<SheetDescription>{t.uncoverPage.historySheetDescription}</SheetDescription>
						</SheetHeader>
						<div className="flex-1 overflow-y-auto px-4 pb-4">
							<div className="mb-4 flex items-center justify-between gap-2 rounded-lg border border-border/70 bg-muted/30 px-3 py-2">
								<div className="space-y-1">
									<div className="text-sm text-muted-foreground">
										{formatText(t.uncoverPage.historyCount, { count: historyItems.length })}
									</div>
									<div className="text-xs text-muted-foreground">
										{formatText(t.uncoverPage.historySelectedCount, { count: selectedHistoryIds.length })}
									</div>
								</div>
								<div className="flex items-center gap-2">
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => void handleMergeSelectedHistories()}
										disabled={historyLoading || historyMerging || selectedHistoryIds.length < 2}
									>
										{historyMerging ? (
											<>
												<Loader2 className="mr-2 h-4 w-4 animate-spin" />
												{t.uncoverPage.historyMerging}
											</>
										) : (
											t.uncoverPage.historyMergeButton
										)}
									</Button>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => setHistoryGroupDialogOpen(true)}
										disabled={historyLoading || historyGrouping || selectedHistoryIds.length === 0}
									>
										<FolderPlus className="mr-2 h-4 w-4" />
										{t.uncoverPage.historyGroupButton}
									</Button>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={() => void assignHistoryGroup({ clearGroup: true })}
										disabled={historyLoading || historyGrouping || !hasGroupedHistorySelected}
									>
										<FolderX className="mr-2 h-4 w-4" />
										{t.uncoverPage.historyUngroupButton}
									</Button>
									<Button
										type="button"
										variant="ghost"
										size="sm"
										onClick={() => fetchHistoryList()}
										disabled={historyLoading}
									>
										<Loader2 className={`h-4 w-4 ${historyLoading ? 'animate-spin' : ''}`} />
									</Button>
								</div>
							</div>
							{historyLoading ? (
								<div className="flex h-40 items-center justify-center text-sm text-muted-foreground">
									<Loader2 className="mr-2 h-4 w-4 animate-spin" />
									{t.uncoverPage.historyLoading}
								</div>
							) : historyItems.length === 0 ? (
								<div className="flex h-40 items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">
									{t.uncoverPage.historyEmpty}
								</div>
							) : (
								<div className="space-y-4">
									{historyGroups.map(group => {
										const groupIds = group.items.map(item => item.id)
										const selectedCount = groupIds.filter(id => selectedHistoryIds.includes(id)).length
										const allSelected = groupIds.length > 0 && selectedCount === groupIds.length
										const partiallySelected = selectedCount > 0 && selectedCount < groupIds.length
										const isExpanded = expandedHistoryGroupIds.includes(group.id)

										return (
											<Collapsible
												key={group.id}
												open={isExpanded}
												onOpenChange={(open) => toggleHistoryGroupExpanded(group.id, open)}
												className="rounded-xl border border-border/70 bg-muted/20"
											>
												<div className="flex items-center justify-between gap-3 px-4 py-3">
													<div className="flex min-w-0 flex-1 items-center gap-3">
														<Checkbox
															checked={allSelected ? true : partiallySelected ? 'indeterminate' : false}
															onCheckedChange={(checked) => handleSelectHistoryGroup(group, checked === true)}
														/>
													<div className="min-w-0 flex-1">
														<div className="text-sm font-medium text-foreground">
															{group.label}
														</div>
														<div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
															<span>{formatText(t.uncoverPage.historyGroupCount, { count: group.items.length })}</span>
															<span>{formatText(t.uncoverPage.historyGroupSelectedCount, { count: selectedCount })}</span>
														</div>
													</div>
												</div>
													<div className="flex items-center gap-2">
														{!group.isUngrouped && group.groupId && (
															<Button
																type="button"
																variant="ghost"
																size="sm"
																className="gap-1.5 px-2"
																onClick={() => {
																	setRenameHistoryGroupId(group.groupId || '')
																	setRenameHistoryGroupName(group.groupName || group.label)
																	setRenameHistoryGroupDialogOpen(true)
																}}
															>
																<PencilLine className="h-4 w-4" />
																{t.uncoverPage.historyRenameGroupButton}
															</Button>
														)}
														<CollapsibleTrigger asChild>
															<Button type="button" variant="ghost" size="sm" className="gap-1.5 px-2">
															{t.uncoverPage.historyGroupToggle}
															<ChevronDown className={`h-4 w-4 transition-transform ${isExpanded ? 'rotate-180' : ''}`} />
														</Button>
														</CollapsibleTrigger>
													</div>
												</div>
												<CollapsibleContent className="space-y-3 border-t border-border/60 px-3 py-3">
													{group.items.map(item => (
														<div
															key={item.id}
															className="rounded-xl border border-border/70 bg-card p-4 transition-colors hover:border-primary/40"
														>
															<div className="flex items-start justify-between gap-3">
																<div className="flex min-w-0 flex-1 items-start gap-3">
																	<Checkbox
																		checked={selectedHistoryIds.includes(item.id)}
																		onCheckedChange={(checked) => toggleHistorySelection(item.id, checked === true)}
																		className="mt-0.5"
																	/>
																	<div className="min-w-0 flex-1 space-y-2">
																		<div className="truncate font-mono text-sm font-medium" title={item.query}>
																			{item.query}
																		</div>
																		<div className="flex flex-wrap items-center gap-2">
																			<Badge variant="secondary">{formatText(t.uncoverPage.historyResultCount, { count: item.totalCount })}</Badge>
																			{item.engines?.map(engine => (
																				<Badge key={engine} variant="outline" className="uppercase">
																					{getEngineDisplayName(engine)}
																				</Badge>
																			))}
																			{item.hasMore && (
																				<Badge variant="outline">{t.uncoverPage.canLoadMore}</Badge>
																			)}
																		</div>
																		<div className="text-xs text-muted-foreground">
																			{formatText(t.uncoverPage.historyCreatedAt, { time: item.createdAt })}
																		</div>
																	</div>
																</div>
																<div className="flex shrink-0 items-center gap-2">
																	<Button
																		type="button"
																		variant="outline"
																		size="sm"
																		onClick={() => handleOpenHistory(item.id)}
																		disabled={historyOpeningId === item.id}
																	>
																		{historyOpeningId === item.id ? (
																			<Loader2 className="h-4 w-4 animate-spin" />
																		) : (
																			t.uncoverPage.historyOpenButton
																		)}
																	</Button>
																	<Popover
																		open={confirmDeleteHistoryId === item.id}
																		onOpenChange={(open) => setConfirmDeleteHistoryId(open ? item.id : null)}
																	>
																		<PopoverTrigger asChild>
																			<Button
																				type="button"
																				variant="ghost"
																				size="icon"
																				onClick={() => setConfirmDeleteHistoryId(item.id)}
																				disabled={historyDeletingId === item.id}
																				title={t.uncoverPage.historyDeleteButton}
																			>
																				{historyDeletingId === item.id ? (
																					<Loader2 className="h-4 w-4 animate-spin" />
																				) : (
																					<Trash2 className="h-4 w-4" />
																				)}
																			</Button>
																		</PopoverTrigger>
																		<PopoverContent
																			align="end"
																			side="top"
																			sideOffset={8}
																			className="w-80 rounded-2xl border-destructive/15 bg-background/95 p-0 shadow-2xl shadow-black/20 backdrop-blur-xl"
																		>
																			<div>
																				<div className="flex items-start gap-3 px-4 py-3">
																					<div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border border-destructive/20 bg-destructive/10 text-destructive">
																						<AlertTriangle className="h-4 w-4" />
																					</div>
																					<div className="min-w-0 flex-1">
																						<p className="text-sm font-semibold text-foreground">
																							{t.uncoverPage.historyDeleteConfirmTitle}
																						</p>
																						<p className="mt-1 break-all text-xs leading-5 text-muted-foreground">
																							{formatText(t.uncoverPage.historyDeleteConfirmMessage, { target: item.query })}
																						</p>
																					</div>
																				</div>
																				<div className="flex items-center justify-end gap-2 border-t border-border/60 px-4 py-2.5">
																					<Button
																						type="button"
																						variant="outline"
																						size="sm"
																						onClick={() => setConfirmDeleteHistoryId(null)}
																						disabled={historyDeletingId === item.id}
																					>
																						{t.common.cancel}
																					</Button>
																					<Button
																						type="button"
																						variant="outline"
																						size="sm"
																						className="border-destructive/35 bg-destructive/10 text-destructive hover:bg-destructive/15 hover:text-destructive"
																						onClick={() => void handleDeleteHistory(item)}
																						disabled={historyDeletingId === item.id}
																					>
																						{historyDeletingId === item.id ? (
																							<Loader2 className="h-4 w-4 animate-spin" />
																						) : (
																							t.common.delete
																						)}
																					</Button>
																				</div>
																			</div>
																		</PopoverContent>
																	</Popover>
																</div>
															</div>
														</div>
													))}
												</CollapsibleContent>
											</Collapsible>
										)
									})}
								</div>
							)}
						</div>
					</SheetContent>
				</Sheet>

				<Dialog open={historyGroupDialogOpen} onOpenChange={setHistoryGroupDialogOpen}>
					<DialogContent className="sm:max-w-lg">
						<DialogHeader>
							<DialogTitle>{t.uncoverPage.historyGroupDialogTitle}</DialogTitle>
							<DialogDescription>{t.uncoverPage.historyGroupDialogDescription}</DialogDescription>
						</DialogHeader>
						<div className="space-y-4">
							<div className="space-y-2">
								<div className="text-sm font-medium">{t.uncoverPage.historyNewGroupLabel}</div>
								<Input
									value={historyGroupNameInput}
									onChange={e => setHistoryGroupNameInput(e.target.value)}
									placeholder={t.uncoverPage.historyNewGroupPlaceholder}
									disabled={historyGrouping}
								/>
							</div>
							{availableHistoryGroups.length > 0 && (
								<div className="space-y-2">
									<div className="text-sm font-medium">{t.uncoverPage.historyExistingGroupLabel}</div>
									<div className="max-h-56 space-y-2 overflow-y-auto">
										{availableHistoryGroups.map(group => (
											<button
												key={group.id}
												type="button"
												className="flex w-full items-center justify-between rounded-lg border border-border/70 px-3 py-2 text-left transition-colors hover:border-primary/40 hover:bg-muted/40"
												onClick={() => void assignHistoryGroup({ groupId: group.groupId, groupName: group.groupName })}
												disabled={historyGrouping}
											>
												<div className="min-w-0">
													<div className="truncate text-sm font-medium">{group.label}</div>
													<div className="mt-1 text-xs text-muted-foreground">
														{formatText(t.uncoverPage.historyGroupCount, { count: group.items.length })}
													</div>
												</div>
												<FolderInput className="h-4 w-4 shrink-0 text-muted-foreground" />
											</button>
										))}
									</div>
								</div>
							)}
						</div>
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => setHistoryGroupDialogOpen(false)}
								disabled={historyGrouping}
							>
								{t.common.cancel}
							</Button>
							<Button
								type="button"
								onClick={() => void handleCreateHistoryGroup()}
								disabled={historyGrouping}
							>
								{historyGrouping ? (
									<>
										<Loader2 className="mr-2 h-4 w-4 animate-spin" />
										{t.uncoverPage.historyGrouping}
									</>
								) : (
									t.uncoverPage.historyCreateGroupButton
								)}
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>

				<Dialog open={renameHistoryGroupDialogOpen} onOpenChange={setRenameHistoryGroupDialogOpen}>
					<DialogContent className="sm:max-w-md">
						<DialogHeader>
							<DialogTitle>{t.uncoverPage.historyRenameGroupDialogTitle}</DialogTitle>
							<DialogDescription>{t.uncoverPage.historyRenameGroupDialogDescription}</DialogDescription>
						</DialogHeader>
						<Input
							value={renameHistoryGroupName}
							onChange={e => setRenameHistoryGroupName(e.target.value)}
							placeholder={t.uncoverPage.historyRenameGroupPlaceholder}
							disabled={historyGroupRenaming}
						/>
						<DialogFooter>
							<Button
								type="button"
								variant="outline"
								onClick={() => setRenameHistoryGroupDialogOpen(false)}
								disabled={historyGroupRenaming}
							>
								{t.common.cancel}
							</Button>
							<Button
								type="button"
								onClick={() => void handleRenameHistoryGroup()}
								disabled={historyGroupRenaming}
							>
								{historyGroupRenaming ? (
									<>
										<Loader2 className="mr-2 h-4 w-4 animate-spin" />
										{t.common.updating}
									</>
								) : (
									t.common.confirm
								)}
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			</div>
		</div>
	)
}

export default function UncoverPage() {
	return (
		<ClientKeepAlive name="uncover-page">
			<UncoverPageContent />
		</ClientKeepAlive>
	)
}
