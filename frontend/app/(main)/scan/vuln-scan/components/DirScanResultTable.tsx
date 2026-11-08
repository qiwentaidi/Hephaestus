'use client'
import { useIsMobile } from '@/hooks/use-mobile'
import { DetailResultToolbar } from './DetailResultToolbar'

import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import {
	ArrowRight,
	Copy,
	ExternalLink,
	Eye,
	Info,
	Search,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { DataTableWithColumns, ColumnDef } from '@/components/ui/data-table-with-columns'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { fetchWithAuth } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { StatusBadge } from '@/lib/status-badge'
import { SavedSyntaxPresetControl } from './SavedSyntaxPresetControl'
import toast from '@/lib/toast'

interface DirScanResult {
	id: string
	url: string
	status: number
	title: string
	location?: string
	contentType?: string
	length: number
	body?: string
	recursion: number
	createdAt: string
}

interface DirScanConfig {
	method?: string
	targets?: string[]
	paths?: string[]
	dictPaths?: string[]
	exts?: string[]
	workers?: number
	timeout?: number
	statusCodeExclude?: number[]
	redirect?: boolean
	backupscan?: boolean
}

interface DirScanResultTableProps {
	taskId: string
	version?: number
	config?: DirScanConfig | null
}

interface SyntaxSuggestion {
	label: string
	description: string
	insert: string
	keywords: string[]
	example?: string
	category?: '字段' | '逻辑'
}

const EMPTY_PLACEHOLDER = '-'

const formatResponseBody = (body?: string) => {
	if (!body) {
		return '暂无响应内容'
	}

	try {
		return JSON.stringify(JSON.parse(body), null, 2)
	} catch {
		return body
	}
}

const syntaxSuggestions: SyntaxSuggestion[] = [
	{
		label: '状态码',
		description: '按状态码筛选，支持 > >= < <=',
		insert: 'dir.status=',
		keywords: ['dir.status', 'status', '状态码'],
		example: 'dir.status=200',
		category: '字段',
	},
	{
		label: '响应类型',
		description: '按 Content-Type 筛选，= 包含，== 精确',
		insert: 'response.type=',
		keywords: ['response.type', 'contenttype', '响应类型'],
		example: 'response.type=json',
		category: '字段',
	},
	{
		label: '响应长度',
		description: '按响应长度筛选，长度单位仅支持 B',
		insert: 'response.length>=',
		keywords: ['response.length', 'length', '长度'],
		example: 'response.length>=1024',
		category: '字段',
	},
	{
		label: '长度排除',
		description: '排除指定响应长度或区间',
		insert: 'response.length.exclude=',
		keywords: ['exclude', '排除', 'response.length.exclude'],
		example: 'response.length.exclude=0-128',
		category: '字段',
	},
	{
		label: 'URL 关键词',
		description: '按 URL 筛选，= 包含，== 精确',
		insert: 'dir.url=',
		keywords: ['dir.url', 'url', '关键词'],
		example: 'dir.url=admin',
		category: '字段',
	},
	{
		label: '标题关键词',
		description: '按页面标题筛选，= 包含，== 精确',
		insert: 'dir.title=',
		keywords: ['dir.title', 'title', '标题'],
		example: 'dir.title=登录',
		category: '字段',
	},
	{
		label: '递归层级',
		description: '按递归深度筛选',
		insert: 'dir.recursion=',
		keywords: ['dir.recursion', 'recursion', '递归'],
		example: 'dir.recursion=0',
		category: '字段',
	},
	{
		label: 'AND 连接符',
		description: '同时满足多个筛选条件',
		insert: ' AND ',
		keywords: ['AND', '组合', '连接'],
		example: 'dir.status=200 AND response.type=json',
		category: '逻辑',
	},
	{
		label: 'OR 连接符',
		description: '匹配任一筛选条件',
		insert: ' OR ',
		keywords: ['OR', '或', '任一'],
		example: 'dir.status=200 OR dir.status=301',
		category: '逻辑',
	},
]

const splitClausesByLogic = (input: string) => {
	const clauses: string[] = []
	let current = ''
	let quote: '"' | "'" | '' = ''

	for (let i = 0; i < input.length; i++) {
		const ch = input[i]

		if (quote) {
			current += ch
			if (ch === quote && input[i - 1] !== '\\') {
				quote = ''
			}
			continue
		}

		if (ch === '"' || ch === "'") {
			quote = ch
			current += ch
			continue
		}

		const logicMatch = input.slice(i).match(/^\s+(AND|OR)\s+/i)
		if (logicMatch) {
			if (current.trim()) {
				clauses.push(current.trim())
			}
			current = ''
			i += logicMatch[0].length - 1
			continue
		}

		current += ch
	}

	if (current.trim()) {
		clauses.push(current.trim())
	}

	return clauses
}

const replaceCurrentClause = (input: string, insert: string) => {
	let quote: '"' | "'" | '' = ''
	let lastOperatorEnd = 0

	for (let i = 0; i < input.length; i++) {
		const ch = input[i]
		if (quote) {
			if (ch === quote && input[i - 1] !== '\\') {
				quote = ''
			}
			continue
		}

		if (ch === '"' || ch === "'") {
			quote = ch
			continue
		}

		const logicMatch = input.slice(i).match(/^\s+(AND|OR)\s+/i)
		if (logicMatch) {
			lastOperatorEnd = i + logicMatch[0].length
			i += logicMatch[0].length - 1
		}
	}

	return input.slice(0, lastOperatorEnd) + insert
}

export function DirScanResultTable({ taskId, version, config }: DirScanResultTableProps) {
	const syntaxInputRef = useRef<HTMLInputElement | null>(null)
	const [results, setResults] = useState<DirScanResult[]>([])
	const [totalResults, setTotalResults] = useState(0)
	const [loadingResults, setLoadingResults] = useState(true)
	const [currentPage, setCurrentPage] = useState(1)
	const [pageSize, setPageSize] = useState(50)
	const [sortState, setSortState] = useState<{ columnId: 'status' | 'length' | ''; order: 'asc' | 'desc' | null }>({
		columnId: '',
		order: null,
	})
	const isMobile = useIsMobile()
	const [searchSyntax, setSearchSyntax] = useState('')
	const [appliedSearchSyntax, setAppliedSearchSyntax] = useState('')
	const [showSyntaxSuggestions, setShowSyntaxSuggestions] = useState(false)
	const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(-1)
	const [syntaxSuggestionStyle, setSyntaxSuggestionStyle] = useState<CSSProperties | null>(null)
	const [portalRoot, setPortalRoot] = useState<HTMLElement | null>(null)
	const [filterError, setFilterError] = useState('')
	const [previewOpen, setPreviewOpen] = useState(false)
	const [previewResult, setPreviewResult] = useState<DirScanResult | null>(null)

	const currentPageRef = useRef(currentPage)
	const pageSizeRef = useRef(pageSize)

	useEffect(() => {
		currentPageRef.current = currentPage
	}, [currentPage])

	useEffect(() => {
		pageSizeRef.current = pageSize
	}, [pageSize])

	useEffect(() => {
		if (typeof window === 'undefined') return
		const timer = window.setTimeout(() => {
			setPortalRoot(document.body)
		}, 0)

		return () => window.clearTimeout(timer)
	}, [])

	const updateSyntaxSuggestionPosition = () => {
		const input = syntaxInputRef.current
		if (!input) return
		setPortalRoot(input.closest('[role="dialog"]') ? input.parentElement : document.body)
		const rect = input.getBoundingClientRect()
		const viewportHeight = window.innerHeight
		const baseMaxHeight = 320
		const margin = 8
		const spaceBelow = viewportHeight - rect.bottom - margin
		const spaceAbove = rect.top - margin
		const openAbove = spaceBelow < 180 && spaceAbove > spaceBelow
		const availableHeight = Math.max(
			140,
			Math.min(baseMaxHeight, openAbove ? spaceAbove - margin : spaceBelow - margin)
		)
		const top = openAbove
			? Math.max(margin, rect.top - availableHeight - 8)
			: rect.bottom + 8

		setSyntaxSuggestionStyle({
			position: 'fixed',
			left: rect.left,
			top,
			width: rect.width,
			maxHeight: availableHeight,
			zIndex: 80,
		})
	}

	const currentClause = useMemo(() => {
		const clauses = splitClausesByLogic(searchSyntax)
		if (clauses.length === 0) return searchSyntax.trim()
		return clauses[clauses.length - 1]
	}, [searchSyntax])

	const filteredSuggestions = useMemo(() => {
		const query = currentClause.trim().toLowerCase()
		if (!query) return syntaxSuggestions.slice(0, 6)
		return syntaxSuggestions
			.filter(item => {
				return (
					item.insert.toLowerCase().includes(query) ||
					item.label.toLowerCase().includes(query) ||
					item.keywords.some(keyword => keyword.toLowerCase().includes(query))
				)
			})
			.slice(0, 8)
	}, [currentClause])

	useEffect(() => {
		const timer = window.setTimeout(() => {
			if (!showSyntaxSuggestions || filteredSuggestions.length === 0) {
				setSelectedSuggestionIndex(-1)
				return
			}
			setSelectedSuggestionIndex(0)
		}, 0)

		return () => window.clearTimeout(timer)
	}, [filteredSuggestions, showSyntaxSuggestions])

	useEffect(() => {
		if (!showSyntaxSuggestions) return
		updateSyntaxSuggestionPosition()
		const handleRelayout = () => updateSyntaxSuggestionPosition()
		window.addEventListener('resize', handleRelayout)
		window.addEventListener('scroll', handleRelayout, true)
		return () => {
			window.removeEventListener('resize', handleRelayout)
			window.removeEventListener('scroll', handleRelayout, true)
		}
	}, [showSyntaxSuggestions])

	const applySuggestion = (insert: string) => {
		setSearchSyntax(prev => {
			const trimmed = prev.trim()
			if (!trimmed) return insert

			if (/\b(AND|OR)\s*$/i.test(trimmed)) {
				return `${trimmed} ${insert}`
			}

			if (['AND', 'OR'].includes(insert.trim().toUpperCase())) {
				return `${trimmed} ${insert.trim().toUpperCase()} `
			}

			const clauses = splitClausesByLogic(trimmed)
			if (clauses.length === 0) return insert
			return replaceCurrentClause(trimmed, insert)
		})
		setShowSyntaxSuggestions(false)
	}

	const applySearch = () => {
		setCurrentPage(1)
		setAppliedSearchSyntax(searchSyntax.trim())
		setShowSyntaxSuggestions(false)
	}

	const applySavedQuery = (query: string) => {
		const normalized = query.trim()
		setSearchSyntax(normalized)
		setFilterError('')
		setCurrentPage(1)
		setAppliedSearchSyntax(normalized)
		setShowSyntaxSuggestions(false)
	}

	const handleSyntaxKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
		const canUseSuggestions = showSyntaxSuggestions && filteredSuggestions.length > 0
		if (e.key === 'ArrowDown') {
			if (!canUseSuggestions) return
			e.preventDefault()
			setSelectedSuggestionIndex(prev => (prev < filteredSuggestions.length - 1 ? prev + 1 : prev))
		} else if (e.key === 'ArrowUp') {
			if (!canUseSuggestions) return
			e.preventDefault()
			setSelectedSuggestionIndex(prev => (prev > 0 ? prev - 1 : 0))
		} else if (e.key === 'Enter') {
			if (canUseSuggestions && selectedSuggestionIndex >= 0) {
				e.preventDefault()
				applySuggestion(filteredSuggestions[selectedSuggestionIndex].insert)
				return
			}
			e.preventDefault()
			applySearch()
		} else if (e.key === 'Escape') {
			setShowSyntaxSuggestions(false)
		}
	}

	const formatSize = (value: number) => {
		if (!value) {
			return '0 B'
		}
		if (value < 1024) {
			return `${value} B`
		}
		if (value < 1024 * 1024) {
			return `${(value / 1024).toFixed(2)} KB`
		}
		if (value < 1024 * 1024 * 1024) {
			return `${(value / (1024 * 1024)).toFixed(2)} MB`
		}
		return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GB`
	}

	const handleCopy = async (value: string, message = '已复制') => {
		if (!value) {
			return
		}
		const success = await copyToClipboard(value)
		if (success) {
			toast.success(message)
		} else {
			toast.error('复制失败')
		}
	}

	const fetchTaskResultsPage = useCallback(async (
		page = currentPageRef.current,
		size = pageSizeRef.current,
		showLoading = true
	) => {
		if (!taskId) {
			return
		}
		if (showLoading) {
			setLoadingResults(true)
		}
		try {
			const offset = (page - 1) * size
			const params = new URLSearchParams()
			params.set('limit', String(size))
			params.set('offset', String(offset))

			if (sortState.columnId && sortState.order) {
				params.set('sortBy', sortState.columnId)
				params.set('sortOrder', sortState.order)
			}
			if (version !== undefined) {
				params.set('version', String(version))
			}
			if (appliedSearchSyntax) {
				params.set('filterExpr', appliedSearchSyntax)
			}

			const response = await fetchWithAuth(`/api/v1/webscan/task/${taskId}/dir-results?${params.toString()}`)
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}))
				throw new Error(errorData.error || '获取扫描结果失败')
			}
			const data = await response.json()
			if (data.success && data.data) {
				setResults(data.data.results || [])
				setTotalResults(data.data.total || 0)
				setFilterError('')
				const computedTotalPages = Math.max(1, Math.ceil((data.data.total || 0) / size))
				if (page > computedTotalPages && data.data.total > 0) {
					setCurrentPage(1)
				}
			}
		} catch (error: any) {
			console.error('Failed to load dir scan results:', error)
			setFilterError(error?.message || '无法获取扫描结果')
		} finally {
			if (showLoading) {
				setLoadingResults(false)
			}
		}
	}, [appliedSearchSyntax, sortState.columnId, sortState.order, taskId, version])

	const handlePreviewResponse = (result: DirScanResult) => {
		setPreviewResult(result)
		setPreviewOpen(true)
	}

	// 定义表格列配置
	const tableColumns: ColumnDef<DirScanResult>[] = [
		{
			id: 'status',
			header: '状态码',
			cell: (item) => <StatusBadge status={item.status ?? 0} className="text-xs font-mono" />,
			headerClassName: 'w-[80px]',
			sortable: true,
		},
		{
			id: 'url',
			header: 'URL',
			cell: (item) => (
				<div className="space-y-1">
					<p className="font-medium break-all">{item.url || EMPTY_PLACEHOLDER}</p>
					<div className="flex flex-wrap gap-2 text-xs text-muted-foreground">
						<span className="font-mono">{config?.method || EMPTY_PLACEHOLDER}</span>
						{item.location && (
							<span className="flex items-center gap-1">
								<ArrowRight className="w-3 h-3" />
								{item.location}
							</span>
						)}
					</div>
				</div>
			),
			alwaysVisible: true,
		},
		{
			id: 'contentType',
			header: '响应类型',
			accessorKey: 'contentType',
			cell: (item) => item.contentType || EMPTY_PLACEHOLDER,
		},
		{
			id: 'title',
			header: '标题',
			cell: (item) => item.title || EMPTY_PLACEHOLDER,
		},
		{
			id: 'length',
			header: '长度',
			cell: (item) => <span className="font-mono">{formatSize(item.length)}</span>,
			headerClassName: 'w-[100px]',
			sortable: true,
		},
		{
			id: 'recursion',
			header: '递归',
			cell: (item) => {
				if (typeof item.recursion === 'number') {
					return <span className="font-mono">{item.recursion}</span>
				}
				return EMPTY_PLACEHOLDER
			},
			headerClassName: 'w-[80px]',
		},
		{
			id: 'actions',
			header: '操作',
			cell: (item) => (
				<div className="text-center space-x-2">
					<Button variant="ghost" size="icon" onClick={() => handlePreviewResponse(item)}>
						<Eye className="w-4 h-4" />
					</Button>
					<Button variant="ghost" size="icon" onClick={() => window.open(item.url, '_blank')}>
						<ExternalLink className="w-4 h-4" />
					</Button>
					<Button variant="ghost" size="icon" onClick={() => handleCopy(item.url)}>
						<Copy className="w-4 h-4" />
					</Button>
				</div>
			),
			className: 'w-[120px] text-center sticky right-0 z-10 border-l border-border bg-card group-hover:bg-muted group-data-[state=selected]:bg-muted',
			headerClassName: 'w-[120px] text-center sticky right-0 z-20 border-l border-border bg-card',
			alwaysVisible: true,
		},
	]

	// 处理排序
	const handleSort = (columnId: string, order: 'asc' | 'desc' | null) => {
		if (columnId !== 'status' && columnId !== 'length') return
		setSortState({
			columnId: order ? (columnId as 'status' | 'length') : '',
			order,
		})
		setCurrentPage(1)
	}

	useEffect(() => {
		if (!taskId) {
			return
		}
		const timer = window.setTimeout(() => {
			void fetchTaskResultsPage(currentPage, pageSize)
		}, 0)

		return () => window.clearTimeout(timer)
	}, [currentPage, fetchTaskResultsPage, pageSize, taskId])

	return (
		<div className="space-y-4">
			<DataTableWithColumns
				mobileCards
				columns={tableColumns}
				data={results}
				loading={loadingResults}
				emptyMessage={appliedSearchSyntax ? '未找到匹配的结果' : '暂无扫描结果'}
				onSort={handleSort}
				sortState={sortState}
				getRowKey={(item) => item.id}
				toolbarLeft={
					<DetailResultToolbar activeCount={Number(Boolean(appliedSearchSyntax))} onApply={applySearch} onReset={() => applySavedQuery('')}>
<div className="flex w-full flex-wrap items-center gap-2 md:flex-nowrap">
						<Tooltip>
							<TooltipTrigger asChild>
								<Button variant="outline" size="icon" className="shrink-0">
									<Info className="h-4 w-4" />
								</Button>
							</TooltipTrigger>
							<TooltipContent className="max-w-[420px] p-0">
								<div className="p-3 space-y-3 text-xs">
									<div>
										<p className="text-muted-foreground">语法结构</p>
										<div className="mt-1 rounded-md border bg-muted/40 p-2 font-mono text-[11px]">
											field op value AND field op value OR field op value
										</div>
									</div>
									<div>
										<p className="text-muted-foreground">可用字段</p>
										<p className="mt-1 font-mono text-[11px] leading-5">
											dir.status / dir.url / dir.title / response.type / response.length / response.length.exclude / dir.recursion
										</p>
									</div>
									<div>
										<p className="text-muted-foreground">可用操作符</p>
										<p className="mt-1 font-mono text-[11px]">= 包含 / == 精确 / ~ 兼容包含 / &gt; &gt;= &lt; &lt;= / 区间(a-b)</p>
									</div>
									<div>
										<p className="text-muted-foreground">示例</p>
										<p className="mt-1 font-mono text-[11px]">
											dir.status=200 OR dir.status=301 AND response.length&gt;=1024
										</p>
									</div>
								</div>
							</TooltipContent>
						</Tooltip>
						<div className="relative order-first w-full min-w-0 flex-none md:order-none md:w-auto md:flex-1">
							<Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
							<Input
								ref={syntaxInputRef}
								value={searchSyntax}
								onChange={(e) => {
									setSearchSyntax(e.target.value)
									setShowSyntaxSuggestions(true)
									updateSyntaxSuggestionPosition()
								}}
								onFocus={() => {
									setShowSyntaxSuggestions(true)
									updateSyntaxSuggestionPosition()
								}}
								onBlur={() => {
									setTimeout(() => setShowSyntaxSuggestions(false), 180)
								}}
								onKeyDown={handleSyntaxKeyDown}
								placeholder='输入语法后点击搜索，例如: dir.status=200 OR dir.status=301'
								className="pl-9"
							/>
							{showSyntaxSuggestions && filteredSuggestions.length > 0 && portalRoot && syntaxSuggestionStyle &&
								createPortal(
								<div style={isMobile ? { maxHeight: 240 } : syntaxSuggestionStyle} className="rounded-lg border bg-popover/95 backdrop-blur shadow-lg overflow-y-auto">
									{filteredSuggestions.map((item, index) => (
										<button
											key={`${item.insert}-${index}`}
											type="button"
											onMouseDown={(e) => {
												e.preventDefault()
												applySuggestion(item.insert)
											}}
											className={`w-full text-left px-3 py-2.5 border-b last:border-b-0 transition-colors ${selectedSuggestionIndex === index ? 'bg-muted' : 'hover:bg-muted/70'}`}
										>
											<div className="flex items-center gap-2">
												{item.category && <Badge variant="outline" className="h-5 px-2 text-[10px]">{item.category}</Badge>}
												<div className="text-sm font-medium">{item.label}</div>
											</div>
											<div className="text-xs font-mono text-primary mt-1">{item.insert}</div>
											<div className="text-[11px] text-muted-foreground mt-1">{item.description}</div>
											{item.example && (
												<div className="text-[11px] text-muted-foreground mt-1">
													例如: <span className="font-mono">{item.example}</span>
												</div>
											)}
										</button>
									))}
								</div>,
								portalRoot
							)}
						</div>
						<Button type="button" onClick={applySearch} className="shrink-0">
							搜索
						</Button>
						<SavedSyntaxPresetControl
							scene="dirscan_result"
							currentQuery={searchSyntax}
							onApply={applySavedQuery}
						/>
					</div>
</DetailResultToolbar>
				}
			/>

			{filterError && <p className="text-xs text-destructive">{filterError}</p>}

			<div className="pt-0">
				<PaginationPro
					current={currentPage}
					total={totalResults}
					pageSize={pageSize}
					onChange={(page, size) => {
						setCurrentPage(page)
						if (size !== pageSize) {
							setPageSize(size)
						}
					}}
					onPageSizeChange={(size) => {
						setPageSize(size)
						setCurrentPage(1)
					}}
					pageSizeOptions={[50, 100, 200]}
					showSizeChanger
					showQuickJumper
					showTotal
				/>
			</div>

			<Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
				<DialogContent className="max-w-2xl">
					<DialogHeader>
						<DialogTitle>响应详情</DialogTitle>
						<DialogDescription>{previewResult?.url || EMPTY_PLACEHOLDER}</DialogDescription>
					</DialogHeader>
					<pre className="bg-muted rounded-lg p-4 max-h-[400px] overflow-auto whitespace-pre-wrap break-words text-sm text-muted-foreground">
						{formatResponseBody(previewResult?.body)}
					</pre>
					<div className="flex justify-end">
						<Button onClick={() => handleCopy(previewResult?.body || '')}>
							<Copy className="w-4 h-4 mr-2" />
							复制内容
						</Button>
					</div>
				</DialogContent>
			</Dialog>
		</div>
	)
}
