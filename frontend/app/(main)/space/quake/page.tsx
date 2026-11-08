'use client'

import { useState, useEffect, useRef, useMemo, useCallback } from 'react'
import Image from 'next/image'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Search, X, Star, Copy, ImageIcon, ExternalLink } from 'lucide-react'
import { cn } from '@/lib/utils'
import { ClientKeepAlive } from '@/components/client-keep-alive'
import { getSpaceEngineConfig, onSpaceEngineConfigChange } from '@/lib/config'
import { fetchWithAuth, getApiBaseURL } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { saveSpaceEngineAssets } from '@/lib/space-engine-assets'
import { useSpaceEngineQueryParam } from '@/lib/space-engine'
import { PasswordInput } from '@/components/ui/password-input'
import toast from 'react-hot-toast'
import { DEFAULT_PAGE_SIZE, booleanLogicData, quakeOptions } from './constants'
import { QuakeSearchFormCard } from './components/search-form-card'
import { QuakeResultsCard } from './components/results-card'
import { SelectTag } from '@/components/ui/select-tag'
import { builtInPortGroupOptions, mergePortGroupOptions, type PortGroupOption } from '@/app/(main)/scan/vuln-scan/utils/portGroup'
import type { ColumnDef } from '@/components/ui/data-table'
import type { FavSyntax, QuakeResult, SearchTab, SuggestionItem, TabFilters } from './types'
import { useI18n } from '@/lib/i18n'

const defaultExportFields: string[] = ['URL', 'IP', 'Port', 'Protocol', 'Host', 'Title']

function QuakePageContent() {
	const { t } = useI18n()
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
	const [showBatchSearchDialog, setShowBatchSearchDialog] = useState(false)
	const [showIconSearchDialog, setShowIconSearchDialog] = useState(false)
	const [showExportDialog, setShowExportDialog] = useState(false)
	const [newSyntaxName, setNewSyntaxName] = useState('')
	const [newSyntaxContent, setNewSyntaxContent] = useState('')
	const [latest, setLatest] = useState(true)
	const [tipsKeyword, setTipsKeyword] = useState('基本信息')
	const [batchInput, setBatchInput] = useState('')
	const [iconUrl, setIconUrl] = useState('')
	const [uploadedImage, setUploadedImage] = useState<File | null>(null)
	const [imagePreview, setImagePreview] = useState<string | null>(null)
	const [isCalculatingHash, setIsCalculatingHash] = useState(false)
	const [userInfo, setUserInfo] = useState<any>(null)
	const [quakeQuotaCredit, setQuakeQuotaCredit] = useState<number | null>(null)
	const [isLoadingUserInfo, setIsLoadingUserInfo] = useState(false)
	const [portScanGroups, setPortScanGroups] = useState<PortGroupOption[]>(builtInPortGroupOptions)
	const canUseLatestFilter = useMemo(() => {
		if (!userInfo?.role || userInfo.role.length === 0) return true
		return userInfo.role.some((role: any) => {
			if (typeof role.priority === 'number') {
				return role.priority > 4
			}
			if (typeof role.fullname === 'string') {
				return !/注册|基础/.test(role.fullname)
			}
			return true
		})
	}, [userInfo])
	const [exportFileName, setExportFileName] = useState(`quake_${new Date().toISOString().slice(0, 10).replace(/-/g, '')}`)
	const [exportFormat, setExportFormat] = useState('csv')
	const [exportCount, setExportCount] = useState(100)
	const [selectedExportFields, setSelectedExportFields] = useState<string[]>(defaultExportFields)
	const [isExporting, setIsExporting] = useState(false)
	const quakeExportFieldLabelMap: Record<string, string> = useMemo(() => ({
		URL: 'URL',
		IP: t.quakePage.exportFields.IP,
		Port: t.quakePage.exportFields.Port,
		Protocol: t.quakePage.exportFields.Protocol,
		Host: 'Host',
		Title: t.quakePage.exportFields.Title,
		Components: t.quakePage.exportFields.Components,
		ICPName: t.quakePage.exportFields.ICPName,
		ICPNumber: t.quakePage.exportFields.ICPNumber,
		CertName: t.quakePage.exportFields.CertName,
		ISP: t.quakePage.exportFields.ISP,
		Position: t.quakePage.exportFields.Position,
	}), [t])
	const exportFieldOptions = useMemo(
		() => Object.entries(quakeExportFieldLabelMap).map(([value, label]) => ({ value, label })),
		[quakeExportFieldLabelMap]
	)
	const quakeColumns: ColumnDef<QuakeResult>[] = useMemo(() => [
		{
			key: 'url',
			label: 'URL',
			headerClassName: 'text-left w-[420px] max-w-[420px]',
			cellClassName: 'w-[420px] max-w-[420px]',
			render: (value, row) => {
				if (!row.url) {
					return <span className="text-xs text-muted-foreground">-</span>
				}
				return (
					<a href={row.url} target="_blank" rel="noopener noreferrer" title={row.url} className="text-primary hover:underline flex items-center gap-1 max-w-full">
						<span className="truncate block max-w-[380px]">{row.url}</span>
						<ExternalLink className="w-3 h-3 shrink-0" />
					</a>
				)
			},
		},
		{ key: 'ip', label: 'IP', render: (value) => <span className="font-mono text-sm">{value || '-'}</span> },
		{ key: 'port', label: t.quakePage.table.port, render: (value) => value ?? '-' },
		{
			key: 'protocol',
			label: t.quakePage.table.protocol,
			render: (value) => (
				<Badge variant="outline" className="bg-orange-500/10 text-orange-600 border-orange-500/20 hover:bg-orange-500/20">
					{value || '-'}
				</Badge>
			),
		},
		{ key: 'host', label: 'Host', render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
		{ key: 'title', label: t.quakePage.table.title, render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
		{
			key: 'components',
			label: t.quakePage.table.components,
			render: (value: string[], row) => {
				if (!row.components || row.components.length === 0) return <span className="text-muted-foreground">-</span>
				return (
					<div className="flex flex-wrap gap-1">
						{row.components.slice(0, 2).map((comp, idx) => (
							<Badge key={`${comp}-${idx}`} variant="secondary" className="text-xs bg-orange-500/10 text-orange-600 hover:bg-orange-500/20">
								{comp}
							</Badge>
						))}
						{row.components.length > 2 && <span className="text-xs text-muted-foreground">+{row.components.length - 2}</span>}
					</div>
				)
			},
		},
		{ key: 'icpName', label: t.quakePage.table.icpName, render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
		{ key: 'icpNumber', label: t.quakePage.table.icpNumber, render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
		{ key: 'certName', label: t.quakePage.table.certName, render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
		{ key: 'isp', label: t.quakePage.table.isp, render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
		{ key: 'position', label: t.quakePage.table.position, render: (value) => <span className="max-w-xs truncate block">{value || '-'}</span> },
	], [t])
	const [suggestions, setSuggestions] = useState<SuggestionItem[]>([])
	const [showSuggestions, setShowSuggestions] = useState(false)
	const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(-1)
	const [inputFocused, setInputFocused] = useState(false)
	const suggestionsRef = useRef<HTMLDivElement>(null)
	const textareaRef = useRef<HTMLTextAreaElement>(null)
	const [searchHistory, setSearchHistory] = useState<string[]>([])
	const currentTab = useMemo(() => tabs.find(t => t.id === activeTab), [tabs, activeTab])
	const availableCredit = useMemo(() => {
		if (quakeQuotaCredit !== null) {
			return Math.max(0, quakeQuotaCredit)
		}

		if (Array.isArray(userInfo?.role) && userInfo.role.length > 0) {
			let sum = 0
			let hasRoleCredit = false
			for (const role of userInfo.role) {
				const rawCredit = role?.credit
				const parsedCredit = typeof rawCredit === 'number' ? rawCredit : Number(rawCredit)
				if (Number.isFinite(parsedCredit)) {
					sum += parsedCredit
					hasRoleCredit = true
				}
			}
			if (hasRoleCredit) {
				return Math.max(0, sum)
			}
		}

		const fallbackCredit = userInfo?.credit
		const parsedFallbackCredit = typeof fallbackCredit === 'number' ? fallbackCredit : Number(fallbackCredit)
		if (Number.isFinite(parsedFallbackCredit)) {
			return Math.max(0, parsedFallbackCredit)
		}
		return null
	}, [quakeQuotaCredit, userInfo])
	const maxExportable = useMemo(() => {
		if (!currentTab) return 0
		const total = Math.max(0, currentTab.total || 0)
		if (availableCredit === null) {
			return total
		}
		return Math.max(0, Math.min(total, availableCredit))
	}, [currentTab, availableCredit])
	const expectedExportCost = Math.min(exportCount, maxExportable || 0)
	const remainingCredit = availableCredit === null ? null : Math.max(0, availableCredit - expectedExportCost)
	useEffect(() => {
		if (!showExportDialog) return
		if (maxExportable <= 0) {
			setExportCount(0)
			return
		}
		setExportCount(prev => {
			if (prev > 0) {
				return Math.min(prev, maxExportable)
			}
			return maxExportable
		})
	}, [showExportDialog, maxExportable])

	// API 配置从后端加载
	const [apiConfig, setApiConfig] = useState({
		key: '',
	})

	// 加载配置
	const loadApiConfig = async () => {
		try {
			const spaceConfig = await getSpaceEngineConfig()
			setApiConfig({
				key: spaceConfig.quake.key || '',
			})
		} catch (error) {
			console.error('Failed to load API config:', error)
			// 如果后端不可用，尝试从localStorage加载（兼容旧数据）
			const savedConfig = localStorage.getItem('spaceConfig')
			if (savedConfig) {
				try {
					const parsed = JSON.parse(savedConfig)
					setApiConfig({
						key: parsed.quake?.key || '',
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
			setApiConfig({
				key: config.quake.key || '',
			})
		})

		return () => {
			unsubscribe()
		}
	}, [])

	const loadFavSyntax = async () => {
		try {
			const response = await fetchWithAuth(`/api/v1/syntax?source=quake`, {
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
				const saved = localStorage.getItem('quake_fav_syntax')
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
					source: 'quake',
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
					source: 'quake',
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
			toast.error('删除收藏失败: ' + (error.message || '未知错误'))
		}
	}

	const setTableFilters = (filters: Record<string, string>) => {
		if (!activeTab) return
		setTabs(prev => prev.map(tab => (tab.id === activeTab ? { ...tab, tableFilters: filters } : tab)))
	}

	const fetchSearchHistory = async () => {
		try {
			const response = await fetchWithAuth('/api/v1/quake/history?limit=50', { cache: 'no-store' })
			if (!response.ok) {
				throw new Error(`HTTP ${response.status}`)
			}
			const data = await response.json()
			const nextHistory = Array.isArray(data?.data) ? data.data.filter((item: unknown): item is string => typeof item === 'string') : []
			setSearchHistory(nextHistory)
			return nextHistory
		} catch (error) {
			console.error('Failed to load Quake search history:', error)
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
			await fetchWithAuth('/api/v1/quake/history', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({ query: normalizedQuery }),
			})
		} catch (error) {
			console.error('Failed to save Quake search history:', error)
		}
	}

	// 获取用户信息
	const fetchUserInfo = async (showAlert = false, silent = false) => {
		if (!apiConfig.key) {
			if (showAlert) {
				toast.error('请先在设置中配置 Quake API Token')
			}
			return
		}

		if (!silent) {
			setIsLoadingUserInfo(true)
		}
		try {
			const backendBase = getBackendBaseUrl()
			const response = await fetchWithAuth(`/api/v1/quake/user`, {
				method: 'GET',
			}).catch((err) => {
				throw new Error(`无法连接到后端服务: ${err.message}。请确保后端服务正在运行在 ${backendBase}`)
			})

			if (!response.ok) {
				const errorData = await response.json().catch(() => ({ error: '未知错误' }))
				throw new Error(errorData.error || errorData.message || '获取用户信息失败')
			}

			const data = await response.json()
			// Quake API 返回 code: 0 表示成功
			if (data.code === 0 || data.code === 200) {
				const userData = data.data || data
				setUserInfo(userData)
			} else {
				throw new Error(data.message || '获取用户信息失败')
			}
		} catch (error: any) {
			console.error('Failed to fetch user info:', error)
			if (showAlert) {
				toast.error('获取用户信息失败: ' + (error.message || '未知错误'))
			}
		} finally {
			if (!silent) {
				setIsLoadingUserInfo(false)
			}
		}
	}

	const fetchQuakeQuota = async (silent = false) => {
		if (!apiConfig.key) {
			setQuakeQuotaCredit(null)
			return
		}

		try {
			const response = await fetchWithAuth('/api/v1/space/quota')
			if (!response.ok) {
				throw new Error(`HTTP error! status: ${response.status}`)
			}

			const data = await response.json()
			const rawCredit = data?.quake?.credit
			const parsedCredit = typeof rawCredit === 'number' ? rawCredit : Number(rawCredit)

			if (Number.isFinite(parsedCredit)) {
				setQuakeQuotaCredit(parsedCredit)
			} else {
				setQuakeQuotaCredit(null)
			}
		} catch (error) {
			console.error('Failed to fetch quake quota:', error)
			if (!silent) {
				setQuakeQuotaCredit(null)
			}
		}
	}

	// 自动加载用户信息
	useEffect(() => {
		if (apiConfig.key) {
			fetchUserInfo()
			fetchQuakeQuota(true)
		}
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [apiConfig.key])

	useEffect(() => {
		if (!canUseLatestFilter && !latest) {
			setLatest(true)
		}
	}, [canUseLatestFilter, latest])

	// 计算图标 hash (MD5 for Quake)
	const calculateIconHash = async (url?: string, imageData?: string): Promise<string> => {
		const requestBody: { url?: string; imageData?: string } = {}
		if (url) {
			requestBody.url = url
		}
		if (imageData) {
			requestBody.imageData = imageData
		}

		const response = await fetchWithAuth(`/api/v1/quake/icon-hash`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify(requestBody),
		})

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({ error: '未知错误' }))
			throw new Error(errorData.error || errorData.message || '计算hash失败')
		}

		const data = await response.json()
		if (!data.hash) {
			throw new Error('未返回有效的hash值')
		}

		return data.hash
	}

	// 图片上传处理
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

	// 图标搜索
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
					const searchQuery = `favicon:${hash}`
					setQuery(searchQuery)
					setShowIconSearchDialog(false)
					// 重置状态
					setIconUrl('')
					setUploadedImage(null)
					setImagePreview(null)
					// 直接执行搜索
					await startSearch(searchQuery)
				} else {
					toast.error('计算hash失败，未返回有效结果')
				}
			} catch (error: any) {
				toast.error('计算hash失败: ' + (error.message || '未知错误'))
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
					const searchQuery = `favicon:${hash}`
					setQuery(searchQuery)
					setShowIconSearchDialog(false)
					// 重置状态
					setIconUrl('')
					setUploadedImage(null)
					setImagePreview(null)
					// 直接执行搜索
					await startSearch(searchQuery)
				} else {
					toast.error('计算hash失败，未返回有效结果')
				}
			} catch (error: any) {
				toast.error('计算hash失败: ' + (error.message || '未知错误'))
			} finally {
				setIsCalculatingHash(false)
			}
		} else {
			toast.error('请输入 Favicon URL 或上传图片')
			return
		}
	}

	// 批量搜索
	const batchSearch = () => {
		setShowBatchSearchDialog(true)
		setBatchInput('')
	}

	const handleBatchSearch = () => {
		if (!batchInput.trim()) {
			toast.error('请输入查询内容')
			return
		}

		const lines = batchInput.split('\n').filter(l => l.trim())
		const ipList: string[] = []
		const queries: string[] = []

		for (const line of lines) {
			const trimmed = line.trim()
			if (!trimmed) continue

			// 匹配IP地址（包括CIDR格式）
			if (/^\d+\.\d+\.\d+\.\d+(\/\d+)?$/.test(trimmed)) {
				ipList.push(trimmed)
			}
			// 匹配域名
			else if (/^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/.test(trimmed)) {
				queries.push(`domain:"${trimmed}"`)
			}
		}

		if (ipList.length === 0 && queries.length === 0) {
			toast.error('没有有效的目标')
			return
		}

		// 如果有IP列表，使用批量查询
		if (ipList.length > 0) {
			const newTabId = Date.now().toString()
			const newTab: SearchTab = {
				id: newTabId,
				query: query || 'ip:"*"',
				results: [],
				total: 0,
				pageSize: DEFAULT_PAGE_SIZE,
				currentPage: 1,
				message: '',
				filters: { latest },
				ipList,
				isLoading: true,
				tableFilters: {},
			}
			setTabs(prev => [...prev, newTab])
			setActiveTab(newTabId)
			// 执行批量搜索
			setTimeout(() => {
				startBatchSearch(newTabId, ipList)
			}, 100)
		} else if (queries.length > 0) {
			// 否则使用普通查询
			setQuery(queries.join(' || '))
		} else {
			toast.error('没有有效的目标')
		}

		setShowBatchSearchDialog(false)
		setBatchInput('')
	}

	const startBatchSearch = async (tabId: string, ipList: string[]) => {
		if (isSearching) {
			toast.error('当前已有查询进行中，请稍候')
			return
		}
		setIsSearching(true)
		try {
			const tab = tabs.find(t => t.id === tabId)
			if (!tab) {
				setIsSearching(false)
				return
			}

			setTabs(prev => prev.map(t => (t.id === tabId ? { ...t, isLoading: true, error: undefined } : t)))

			const data = await fetchSearchResults(tab.query, 1, DEFAULT_PAGE_SIZE, tab.filters, ipList)
			setTabs(current => current.map(t => {
				if (t.id === tabId) {
					return {
						...t,
						results: data.results || [],
						total: data.total || 0,
						message: data.message || `共找到 ${data.total || 0} 条结果`,
						isLoading: false,
						error: undefined,
					}
				}
				return t
			}))
			if (apiConfig.key) {
				fetchUserInfo(false, true)
				fetchQuakeQuota(true)
			}
			setIsSearching(false)
		} catch (error: any) {
			toast.error('批量搜索失败: ' + (error.message || '未知错误'))
			setTabs(prev => prev.map(t => {
				if (t.id === tabId) {
					return {
						...t,
						isLoading: false,
						error: error.message || '搜索失败',
					}
				}
				return t
			}))
			setIsSearching(false)
		}
	}

	const getBackendBaseUrl = () => {
		return getApiBaseURL().trim().replace(/\/$/, '')
	}

	const fetchSearchResults = async (
		searchQuery: string,
		pageNum: number,
		pageSize: number,
		filters: TabFilters,
		ipList?: string[]
	) => {
		const response = await fetchWithAuth(`/api/v1/quake/search`, {
			method: 'POST',
			headers: {
				'Content-Type': 'application/json',
			},
			body: JSON.stringify({
				query: searchQuery,
				pageNum,
				pageSize,
				latest: filters.latest,
				ipList: ipList || [],
			}),
		}).catch((err) => {
			throw new Error(`无法连接到后端服务: ${err.message}`)
		})

		if (!response.ok) {
			const errorData = await response.json().catch(() => ({ error: '未知错误' }))
			throw new Error(errorData.error || errorData.message || '请求失败')
		}

		const data = await response.json()

		// 调试日志
		console.log('Quake search response:', data)

		// 后端返回的字段名可能是大写（Code）或小写（code），需要兼容处理
		const code = data.Code !== undefined ? data.Code : data.code
		const message = data.Message !== undefined ? data.Message : data.message
		const responseData = data.Data !== undefined ? data.Data : data.data
		const total = data.Total !== undefined ? data.Total : data.total
		const credit = data.Credit !== undefined ? data.Credit : data.credit

		// Quake API 返回 code: 0 表示成功
		// 如果 code 不是 0，说明有错误
		if (code !== 0 && code !== 200 && code !== undefined) {
			// 如果有错误信息，使用错误信息；否则使用默认消息
			const errorMsg = message || data.error || `搜索失败 (code: ${code})`
			console.error('Quake search error:', errorMsg, 'Code:', code)
			throw new Error(errorMsg)
		}

		// 检查是否有数据字段
		if (!responseData || !Array.isArray(responseData)) {
			// 如果没有数据，但 code 是 0 或 undefined，可能是空结果
			return {
				results: [],
				total: total || 0,
				message: message || '查询成功，但未找到结果',
				credit: credit || 0,
			}
		}

		return {
			results: responseData || [],
			total: total || 0,
			message: message || '查询成功',
			credit: credit || 0,
		}
	}

	const startSearch = async (searchQuery?: string) => {
		if (isSearching) {
			toast.error('当前已有查询进行中，请稍候')
			return
		}
		const queryToUse = searchQuery || query
		if (!queryToUse.trim()) {
			toast.error('请输入查询内容')
			return
		}

		if (!apiConfig.key) {
			toast.error('请先在设置中配置 Quake API Token')
			return
		}

		setIsSearching(true)
		const filters: TabFilters = { latest }
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
				message: data.message || `共找到 ${data.total || 0} 条结果`,
				filters,
				isLoading: false,
				error: undefined,
				tableFilters: {},
			}

			setTabs(prev => [...prev, newTab])
			setActiveTab(newTab.id)

			// 保存搜索历史记录
			void saveSearchHistory(trimmedQuery)

				// 搜索成功后，同步更新用户信息（因为查询会扣除积分）
				if (apiConfig.key) {
					fetchUserInfo(false, true) // silent = true，不显示加载状态
					fetchQuakeQuota(true)
				}
		} catch (error: any) {
			const errorMessage = error.message || '未知错误'
			toast.error('搜索失败: ' + errorMessage)
			console.error('Search error:', error)
			console.error('Search query:', trimmedQuery)
		} finally {
			setIsSearching(false)
		}
	}

	const closeTab = (tabId: string) => {
		const newTabs = tabs.filter(t => t.id !== tabId)
		setTabs(newTabs)
		if (activeTab === tabId && newTabs.length > 0) {
			setActiveTab(newTabs[0].id)
		} else if (activeTab === tabId) {
			setActiveTab('')
		}
	}

	const runQuickQuery = async (searchQuery: string, failedPrefix: string) => {
		if (isSearching) {
			toast.error('当前已有查询进行中，请稍候')
			return
		}
		if (!apiConfig.key) {
			toast.error('请先在设置中配置 Quake API Token')
			return
		}

		setIsSearching(true)
		const filters: TabFilters = { latest }
		try {
			const data = await fetchSearchResults(searchQuery, 1, DEFAULT_PAGE_SIZE, filters)
			const newTab: SearchTab = {
				id: Date.now().toString(),
				query: searchQuery,
				results: data.results || [],
				total: data.total || 0,
				pageSize: DEFAULT_PAGE_SIZE,
				currentPage: 1,
				message: data.message || `共找到 ${data.total || 0} 条结果`,
				filters,
				isLoading: false,
				error: undefined,
				tableFilters: {},
			}

			setTabs(prev => [...prev, newTab])
			setActiveTab(newTab.id)
			void fetchUserInfo(false, true)
		} catch (error: any) {
			toast.error(`${failedPrefix}失败: ${error.message || '未知错误'}`)
		} finally {
			setIsSearching(false)
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
		const cSegmentQuery = `ip:"${cSegment}"`
		await runQuickQuery(cSegmentQuery, 'C段查询')
	}

	// 证书查询功能
	const handleCertQuery = async (certName: string) => {
		const normalized = certName.trim()
		if (!normalized) {
			toast.error('无有效证书名称')
			return
		}

		const escaped = normalized.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
		const certQuery = `cert:"${escaped}"`
		await runQuickQuery(certQuery, '证书查询')
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

	// 根据输入内容获取提示：优先匹配平台内置语法，未匹配时再回退到 Quake 搜索提示
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

		// 平台内置语法、布尔逻辑和收藏语法优先，避免引擎的应用提示覆盖字段语法提示
		for (const category of quakeOptions) {
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

		// 从布尔逻辑语法中查找匹配的
		for (const item of booleanLogicData) {
			const keyLower = item.key.toLowerCase()
			const descLower = item.description.toLowerCase()

			if (keyLower.includes(inputLower) || descLower.includes(inputLower)) {
				matches.push({
					key: item.key,
					description: item.description,
					category: '布尔逻辑',
				})
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

		// 平台语法没有匹配时，才调用 Quake Tips API 作为补充
		if (apiConfig.key) {
			try {
				const response = await fetchWithAuth(`/api/v1/quake/tips?keyword=${encodeURIComponent(input)}`, {
					method: 'GET',
				})

				if (response.ok) {
					const data = await response.json()
					// Quake API 返回的数据格式：{ code: 0/200, data: [{ product_name, vendor_name, ... }] }
					if ((data.code === 0 || data.code === 200) && data.data && Array.isArray(data.data) && data.data.length > 0) {
						// 将后端返回的数据转换为提示格式
						data.data.forEach((item: any) => {
							if (item.product_name) {
								matches.push({
									key: `app:"${item.product_name}"`,
									description: `应用: ${item.product_name}${item.vendor_name ? ` (${item.vendor_name})` : ''}`,
									category: '搜索提示',
								})
							}
						})
						if (matches.length > 0) {
							return matches.slice(0, 10)
						}
					}
				}
			} catch (error) {
				console.error('Failed to fetch tips from API:', error)
				// API 调用失败，继续使用历史记录
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
	}, [apiConfig.key, favSyntax, searchHistory])

	// 当输入变化时获取提示
	useEffect(() => {
		const debounceTimer = setTimeout(() => {
			if (inputFocused) {
				getSuggestions(query || '').then(matches => {
					setSuggestions(matches)
					setShowSuggestions(matches.length > 0)
					setSelectedSuggestionIndex(-1)
				})
			} else {
				setShowSuggestions(false)
				setSuggestions([])
			}
		}, 300) // 300ms 防抖

		return () => clearTimeout(debounceTimer)
	}, [getSuggestions, inputFocused, query])

	useEffect(() => {
		void fetchSearchHistory()
	}, [])

	const copyURLs = (type: 'current' | 'top100', dedup: boolean = false) => {
		const currentTab = tabs.find(t => t.id === activeTab)
		if (!currentTab) return

		let results = currentTab.results
		if (type === 'top100') {
			results = currentTab.results.slice(0, 100)
		}

		if (dedup) {
			const seen = new Set()
			results = results.filter(item => {
				const key = `${item.ip}_${item.url}`
				if (seen.has(key)) return false
				seen.add(key)
				return true
			})
		}

		const urls = results.map(r => r.url).filter(Boolean).join('\n')
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

	const openExportDialog = () => {
		const tab = currentTab || tabs.find(t => t.id === activeTab)
		if (!tab) {
			toast.error('请先执行查询')
			return
		}
		if (availableCredit !== null && availableCredit <= 0) {
			toast.error('可用积分不足，无法导出')
			return
		}
		const limit = Math.max(0, availableCredit === null ? (tab.total || 0) : maxExportable)
		if (limit <= 0) {
			toast.error('当前查询暂无可导出的数据')
			return
		}
		setExportCount(prev => {
			if (prev > 0) {
				return Math.min(prev, limit)
			}
			return limit
		})
		setSelectedExportFields(prev => (prev.length ? prev : defaultExportFields))
		setExportFileName(`quake_${new Date().toISOString().slice(0, 10).replace(/-/g, '')}`)
		setShowExportDialog(true)
	}

	const runExport = async () => {
		const tab = tabs.find(t => t.id === activeTab)
		if (!tab) return

		const totalToExport = Math.max(0, Math.min(exportCount, tab.total || 0))
		if (totalToExport <= 0) {
			toast.error('导出条数必须大于 0')
			return
		}

		const exportLimit = Math.max(0, availableCredit === null ? (tab.total || 0) : maxExportable)
		const limitedCount = Math.max(0, Math.min(totalToExport, exportLimit))
		if (limitedCount <= 0) {
			toast.error('可用积分不足或无可导出的数据')
			return
		}

		const normalizedFields = selectedExportFields.filter(field => quakeExportFieldLabelMap[field])
		if (normalizedFields.length === 0) {
			toast.error('请至少选择一个导出字段')
			return
		}

		setIsExporting(true)
		try {
			const response = await fetchWithAuth(`/api/v1/quake/export/tasks`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify({
					query: tab.query,
					exportCount: limitedCount,
					fields: normalizedFields,
					format: exportFormat,
					fileName: exportFileName,
					latest: tab.filters.latest,
				}),
			})

			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || errorData.message || '创建导出任务失败')
			}
			const data = await response.json()
			if (!data.success) {
				throw new Error(data.error || '创建导出任务失败')
			}
				toast.success('导出任务已创建，请到报告管理查看进度')
			setShowExportDialog(false)
			void fetchUserInfo(false, true)
		} catch (error: any) {
			toast.error('导出失败: ' + (error?.message || '未知错误'))
		} finally {
			setIsExporting(false)
		}
	}

	const handleSaveToAssetCenter = async (targets: QuakeResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error(t.common.saveToAssetCenterEmpty)
			return
		}
		const toastId = toast.loading(t.common.savingToAssetCenter)
		try {
			const result = await saveSpaceEngineAssets(
				'quake',
				targets.map(item => ({
					ip: item.ip,
					domain: item.host,
					url: item.url,
					title: item.title,
					port: item.port,
					protocol: item.protocol,
					product: Array.isArray(item.components) ? item.components.join(', ') : item.components,
					host: item.host,
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

	const handleSendToWebscan = async (targets: QuakeResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error('请先选择目标资产')
			return
		}

		const urls = Array.from(
			new Set(
				targets
					.map((item) => {
						if (item.url) return item.url
						const protocol = item.protocol?.toLowerCase() === 'https' || item.port === 443 ? 'https' : 'http'
						const host = item.host || item.ip
						if (!host) return ''
						const portPart = item.port && item.port !== 80 && item.port !== 443 ? `:${item.port}` : ''
						return `${protocol}://${host}${portPart}`
					})
					.filter(Boolean)
			)
		) as string[]

		if (urls.length === 0) {
			toast.error('无可用 URL 目标')
			return
		}

		const taskName = `quake-webscan-${new Date().toLocaleString('zh-CN')}`
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
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
				}),
			})
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || '创建任务失败')
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

	const handleSendToCrack = async (targets: QuakeResult[]) => {
		if (!targets || targets.length === 0) {
			toast.error('请先选择目标资产')
			return
		}

		const addresses = Array.from(
			new Set(
				targets
					.map((item) => {
						if (!item.ip || !item.port) return ''
						const protocol = item.protocol?.toLowerCase() || 'tcp'
						return `${protocol}://${item.ip}:${item.port}`
					})
					.filter(Boolean)
			)
		) as string[]

		if (addresses.length === 0) {
			toast.error('无可用地址目标')
			return
		}

		const taskName = `quake-crack-${new Date().toLocaleString('zh-CN')}`
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/scan`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					scanType: 'crack',
					taskName,
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
				throw new Error(errorData.error || '创建任务失败')
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

	const buildPortscanTarget = (item: QuakeResult): string | null => {
		const rawHost = String(item.ip || item.host || '').trim().replace(/^https?:\/\//i, '')
		if (!rawHost) return null
		return rawHost.split(':')[0] || rawHost
	}

	const handleSendToPortscan = async (targets: QuakeResult[], portGroup: PortGroupOption) => {
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

	const handleLatestFilterChange = async (tabId: string, latestValue: boolean) => {
		setTabs(prev => prev.map(t => (t.id === tabId ? { ...t, filters: { ...t.filters, latest: latestValue } } : t)))
		try {
			await refreshTab(tabId, 1)
		} catch (error) {
			console.error('Latest filter change error:', error)
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

	// 刷新当前tab
	const refreshTab = async (tabId: string, pageNum: number, pageSizeOverride?: number) => {
		const tab = tabs.find(t => t.id === tabId)
		if (!tab) return

		const requestPageSize = pageSizeOverride ?? tab.pageSize
		setTabs(prev =>
			prev.map(t =>
				t.id === tabId
					? { ...t, currentPage: pageNum, pageSize: requestPageSize, isLoading: true, error: undefined }
					: t
			)
		)

		try {
			const data = await fetchSearchResults(tab.query, pageNum, requestPageSize, tab.filters, tab.ipList)
			setTabs(current => current.map(t => {
				if (t.id === tabId) {
					return {
						...t,
						results: data.results || [],
						total: data.total || 0,
						message: data.message || `共找到 ${data.total || 0} 条结果`,
						currentPage: pageNum,
						pageSize: requestPageSize,
						isLoading: false,
						error: undefined,
					}
				}
				return t
			}))
		} catch (error: any) {
			setTabs(current => current.map(t => {
				if (t.id === tabId) {
					return {
						...t,
						isLoading: false,
						error: error.message || '加载失败',
					}
				}
				return t
			}))
		}
	}

	const handlePageSizeChange = async (tabId: string, size: number) => {
		try {
			await refreshTab(tabId, 1, size)
		} catch (error: any) {
			console.error('Page size change error:', error)
		}
	}

	return (
		<div className="h-full">

			<div className="min-w-0 p-3 space-y-3 md:p-6 md:space-y-6">
				<QuakeSearchFormCard
					query={query}
					setQuery={setQuery}
					isSearching={isSearching}
					onStartSearch={() => startSearch()}
					userInfo={userInfo}
					isLoadingUserInfo={isLoadingUserInfo}
					onFetchUserInfo={fetchUserInfo}
					onOpenApiConfigDialog={() => setShowApiConfigDialog(true)}
					onOpenTipsDialog={() => setShowTipsDialog(true)}
					onOpenIconSearchDialog={() => setShowIconSearchDialog(true)}
					onOpenFavSyntaxDialog={() => setShowFavSyntaxDialog(true)}
					onOpenAddFavDialog={() => {
						setShowAddFavDialog(true)
						setNewSyntaxContent(query)
					}}
					onBatchSearch={batchSearch}
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
					latest={latest}
					setLatest={setLatest}
					canUseLatestFilter={canUseLatestFilter}
					availableCredit={availableCredit}
				/>

				<QuakeResultsCard
					tabs={tabs}
					activeTab={activeTab}
					onTabChange={handleTabChange}
					onCloseTab={closeTab}
					isSearching={isSearching}
					currentTab={currentTab}
					columns={quakeColumns}
					onOpenExportDialog={openExportDialog}
					onCopyUrls={copyURLs}
					onHandleCSegmentQuery={handleCSegmentQuery}
					onHandleCertQuery={handleCertQuery}
					onRefreshTab={refreshTab}
					onPageSizeChange={handlePageSizeChange}
					onTableFiltersChange={setTableFilters}
					onTabFiltersChange={handleLatestFilterChange}
					onSaveToAssetCenter={handleSaveToAssetCenter}
					onSendToWebscan={handleSendToWebscan}
					onSendToCrack={handleSendToCrack}
					onSendToPortscan={handleSendToPortscan}
					portScanGroups={portScanGroups}
				/>
			</div>

			{/* 收藏语法对话框 */}
			<Dialog open={showFavSyntaxDialog} onOpenChange={setShowFavSyntaxDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-3xl max-h-[70vh]">
					<DialogHeader>
						<DialogTitle>{t.quakePage.page.favTitle}</DialogTitle>
						<DialogDescription>{t.quakePage.page.favDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-2 max-h-[50vh] overflow-y-auto pr-2">
						{favSyntax.length === 0 ? (
							<div className="text-center py-12 text-sm text-muted-foreground">
								<Star className="w-12 h-12 mx-auto mb-4 opacity-30" />
								<p>{t.quakePage.page.noFav}</p>
								<p className="text-xs mt-2">{t.quakePage.page.favHint}</p>
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
											{t.quakePage.page.use}
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
						<DialogTitle>{t.quakePage.page.addFavTitle}</DialogTitle>
						<DialogDescription>{t.quakePage.page.addFavDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="syntax-name">{t.quakePage.page.name}</Label>
							<Input
								id="syntax-name"
								value={newSyntaxName}
								onChange={e => setNewSyntaxName(e.target.value)}
								placeholder={t.quakePage.page.exampleHttpService}
								className="mt-2"
							/>
						</div>
						<div>
							<Label htmlFor="syntax-content">{t.quakePage.page.syntaxContent}</Label>
							<Textarea
								id="syntax-content"
								value={newSyntaxContent}
								onChange={e => setNewSyntaxContent(e.target.value)}
								placeholder='service:"http" && port:"80"'
								className="mt-2 font-mono min-h-[80px]"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowAddFavDialog(false)}>
							{t.quakePage.page.cancel}
						</Button>
						<Button onClick={addFavSyntax} disabled={!newSyntaxName || !newSyntaxContent}>
							{t.quakePage.page.save}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 导出对话框 */}
			<Dialog open={showExportDialog} onOpenChange={setShowExportDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-2xl max-h-[calc(100vh-4rem)] overflow-y-auto">
					<DialogHeader>
						<DialogTitle>{t.quakePage.page.exportTitle}</DialogTitle>
						<DialogDescription>{t.quakePage.page.exportDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div>
							<Label htmlFor="quake-export-name">{t.quakePage.page.fileName}</Label>
							<Input
								id="quake-export-name"
								value={exportFileName}
								onChange={e => setExportFileName(e.target.value)}
								className="mt-2"
								placeholder="quake_assets"
							/>
						</div>
						<div>
							<Label>{t.quakePage.page.fileFormat}</Label>
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
							<Label htmlFor="quake-export-count">{t.quakePage.page.exportCount}</Label>
							<Input
								id="quake-export-count"
								type="number"
								min={0}
								max={maxExportable || undefined}
								value={exportCount}
								onChange={e => setExportCount(Math.max(0, Math.min(Number(e.target.value) || 0, maxExportable || 0)))}
								className="mt-2"
								disabled={maxExportable <= 0}
							/>
							<p className="text-xs text-muted-foreground mt-1">
								{formatText(t.quakePage.page.exportLimit, { count: maxExportable?.toLocaleString() || 0 })}
							</p>
							<div className="mt-3 grid gap-2 rounded-lg border border-border/50 bg-muted/40 p-3 text-sm">
								<div className="flex items-center justify-between">
									<span className="text-muted-foreground">{t.quakePage.page.queryResults}</span>
									<span>{currentTab?.total?.toLocaleString() || 0}</span>
								</div>
								<div className="flex items-center justify-between">
									<span className="text-muted-foreground">{t.quakePage.page.availableCredit}</span>
									<span>{availableCredit === null ? t.quakePage.page.notFetched : availableCredit.toLocaleString()}</span>
								</div>
								<div className="flex items-center justify-between">
									<span className="text-muted-foreground">{t.quakePage.page.expectedCost}</span>
									<span>
										{expectedExportCost?.toLocaleString() || 0}
										{remainingCredit !== null ? formatText(t.quakePage.page.remainingCredit, { count: remainingCredit.toLocaleString() }) : ''}
									</span>
								</div>
								<p className="text-xs text-muted-foreground">
									{t.quakePage.page.exportTip}
								</p>
							</div>
						</div>
						<div>
							<Label>{t.quakePage.page.exportFields}</Label>
							<div className="mt-2">
								<SelectTag
									options={exportFieldOptions}
									value={selectedExportFields}
									onChange={setSelectedExportFields}
									placeholder={t.quakePage.page.selectExportFields}
								/>
							</div>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowExportDialog(false)} disabled={isExporting}>
							{t.quakePage.page.cancel}
						</Button>
						<Button
							onClick={runExport}
							disabled={isExporting || !selectedExportFields.length || exportCount <= 0 || maxExportable <= 0}
						>
							{isExporting ? t.quakePage.page.creating : t.quakePage.page.createExportTask}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 语法提示对话框 */}
			<Dialog open={showTipsDialog} onOpenChange={setShowTipsDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-4xl max-h-[85vh] flex flex-col">
					<DialogHeader className="flex-shrink-0">
						<DialogTitle>{t.quakePage.page.syntaxTitle}</DialogTitle>
						<DialogDescription>{t.quakePage.page.syntaxDescription}</DialogDescription>
					</DialogHeader>

					<div className="flex-1 flex flex-col min-h-0 overflow-hidden">
						{/* 布尔逻辑单独展示 */}
						<div className="mb-4 pb-4 border-b flex-shrink-0">
							<h4 className="text-sm font-semibold mb-2">{t.quakePage.page.booleanLogic}</h4>
							<div className="grid gap-2 max-h-[120px] overflow-y-auto pr-2">
								{booleanLogicData.map((item, idx) => (
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
											</div>
											<p className="text-xs text-muted-foreground">
												{item.description}
											</p>
										</div>
									</div>
								))}
							</div>
						</div>

						<Tabs value={tipsKeyword} onValueChange={setTipsKeyword} className="w-full flex-1 flex flex-col min-h-0">
							<div className="overflow-x-auto -mx-1 px-1 flex-shrink-0">
								<TabsList className="inline-flex h-auto min-w-max">
									{quakeOptions.map((item) => (
										<TabsTrigger key={item.title} value={item.title} className="whitespace-nowrap shrink-0">
											{item.title}
										</TabsTrigger>
									))}
								</TabsList>
							</div>

							{quakeOptions.map((category) => (
								<TabsContent key={category.title} value={category.title} className="mt-4 flex-1 min-h-0 overflow-hidden">
									<div className="space-y-2 h-full max-h-[300px] overflow-y-auto pr-2">
										<div className="grid gap-2">
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
															{item.isVip && (
																<Badge variant="outline" className="text-xs shrink-0 bg-transparent text-emerald-600 border-emerald-300">
																	VIP
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
									</div>
								</TabsContent>
							))}
						</Tabs>
					</div>
				</DialogContent>
			</Dialog>

			{/* 图标搜索对话框 */}
			<Dialog open={showIconSearchDialog} onOpenChange={setShowIconSearchDialog}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>{t.quakePage.page.iconTitle}</DialogTitle>
						<DialogDescription>{t.quakePage.page.iconDescription}</DialogDescription>
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
								{t.quakePage.page.iconUrlHint}
							</p>
						</div>

						<div className="relative">
							<div className="flex items-center gap-2 mb-2">
								<Label>{t.quakePage.page.uploadImage}</Label>
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
										{t.quakePage.page.clear}
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
											{t.quakePage.page.clickUpload}
										</span>
										<span className="text-xs text-muted-foreground">
											{t.quakePage.page.uploadFormats}
										</span>
									</label>
								</div>
							) : (
								<div className="border border-border rounded-lg p-4">
									<div className="flex items-center gap-4">
										{imagePreview && (
											<Image
												src={imagePreview}
												alt={t.quakePage.page.previewAlt}
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
							{t.quakePage.page.cancel}
						</Button>
						<Button onClick={handleIconSearch} disabled={(!iconUrl && !uploadedImage) || isCalculatingHash}>
							{isCalculatingHash ? (
								<>
									<div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />
									{t.quakePage.page.calculating}
								</>
							) : (
								<>
									<Search className="w-4 h-4 mr-2" />
									{t.quakePage.page.search}
								</>
							)}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 批量查询对话框 */}
			<Dialog open={showBatchSearchDialog} onOpenChange={setShowBatchSearchDialog}>
				<DialogContent className="max-md:max-w-[calc(100%-1.5rem)] max-md:max-h-[85dvh] max-md:overflow-y-auto max-md:p-4 max-md:[&>*]:min-w-0 max-w-3xl">
					<DialogHeader>
						<DialogTitle>{t.quakePage.page.batchTitle}</DialogTitle>
						<DialogDescription>
							{t.quakePage.page.batchDescription}
						</DialogDescription>
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
											const lines = batchInput.split('\n').filter(l => l.trim())
											const ipCount = lines.filter(l => /^\d+\.\d+\.\d+\.\d+(\/\d+)?$/.test(l.trim())).length
											const domainCount = lines.filter(l => /^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/.test(l.trim())).length
											const validCount = ipCount + domainCount
											return `${validCount} / ${lines.length}`
										})()}
									</div>
								)}
							</div>
							<div className="space-y-1 text-xs text-muted-foreground bg-muted/50 p-3 rounded-md">
								<p>{t.quakePage.page.batchHelp}</p>
								<p>{t.quakePage.page.batchLimit}</p>
								<p className="text-primary font-medium">{t.quakePage.page.batchShortcut}</p>
							</div>
						</div>
						{batchInput && (
							<div className="flex items-center gap-4 text-sm">
								{(() => {
									const lines = batchInput.split('\n').filter(l => l.trim())
									const ipCount = lines.filter(l => /^\d+\.\d+\.\d+\.\d+(\/\d+)?$/.test(l.trim())).length
									const domainCount = lines.filter(l => /^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/.test(l.trim())).length
									const validCount = ipCount + domainCount
									return (
										<>
											<span className={cn("text-muted-foreground", ipCount > 1000 && "text-destructive")}>
												{formatText(t.quakePage.page.batchIpCount, { count: ipCount })}
											</span>
											<span className={cn("text-muted-foreground", domainCount > 1000 && "text-destructive")}>
												{formatText(t.quakePage.page.batchDomainCount, { count: domainCount })}
											</span>
											<span className="text-muted-foreground">
												{formatText(t.quakePage.page.batchValidCount, { valid: validCount, total: lines.length })}
											</span>
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
							{t.quakePage.page.cancel}
						</Button>
						<Button onClick={handleBatchSearch} disabled={!batchInput.trim()}>
							<Search className="w-4 h-4 mr-2" />
							{t.quakePage.page.runSearch}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	)
}

export default function QuakePage() {
	return (
		<ClientKeepAlive name="quake-page">
			<QuakePageContent />
		</ClientKeepAlive>
	)
}
