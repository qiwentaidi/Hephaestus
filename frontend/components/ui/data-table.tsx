'use client'

import * as React from 'react'
import { Columns, Filter } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export interface ColumnDef<T> {
	key: string
	label: string
	visible?: boolean
	filterable?: boolean
	render?: (value: any, row: T, index: number) => React.ReactNode
	headerClassName?: string
	cellClassName?: string
	sticky?: 'left' | 'right'
	stickyClassName?: string
}

export interface DataTableProps<T> {
	data: T[]
	columns: ColumnDef<T>[]
	loading?: boolean
	emptyMessage?: string
	storageKey?: string // localStorage key for column visibility
	onColumnVisibilityChange?: (visibility: Record<string, boolean>) => void
	filters?: Record<string, string> // 列筛选值
	onFilterChange?: (filters: Record<string, string>) => void
	renderActions?: (row: T, index: number) => React.ReactNode
	actionsColumnLabel?: string
	actionsColumnClassName?: string
	className?: string
	getRowKey?: (row: T, index: number) => string | number
	rowSelection?: {
		selectedRowKeys?: (string | number)[]
		onChange?: (selectedRowKeys: (string | number)[], selectedRows: T[]) => void
		getCheckboxProps?: (record: T) => { disabled?: boolean }
	}
}

export function DataTable<T extends Record<string, any>>({
	data,
	columns,
	loading = false,
	emptyMessage = '暂无数据',
	storageKey,
	onColumnVisibilityChange,
	filters = {},
	onFilterChange,
	renderActions,
	actionsColumnLabel = '操作',
	actionsColumnClassName,
	className,
	getRowKey,
	rowSelection,
}: DataTableProps<T>) {
	const tableHostRef = React.useRef<HTMLDivElement | null>(null)
	const [showLeftFixedShadow, setShowLeftFixedShadow] = React.useState(false)
	const [showRightFixedShadow, setShowRightFixedShadow] = React.useState(false)
	const [internalSelectedKeys, setInternalSelectedKeys] = React.useState<Set<string | number>>(new Set())

	const selectedKeys = React.useMemo(
		() => new Set(rowSelection?.selectedRowKeys ?? Array.from(internalSelectedKeys)),
		[rowSelection?.selectedRowKeys, internalSelectedKeys]
	)

	const getItemKey = React.useCallback(
		(row: T, index: number) => (getRowKey ? getRowKey(row, index) : index),
		[getRowKey]
	)

	const selectableData = React.useMemo(() => {
		if (!rowSelection?.getCheckboxProps) {
			return data
		}
		return data.filter((row) => !rowSelection.getCheckboxProps?.(row)?.disabled)
	}, [data, rowSelection])

	const updateSelection = React.useCallback(
		(nextKeys: (string | number)[]) => {
			const nextRows = data.filter((row, index) => nextKeys.includes(getItemKey(row, index)))
			if (!rowSelection?.selectedRowKeys) {
				setInternalSelectedKeys(new Set(nextKeys))
			}
			rowSelection?.onChange?.(nextKeys, nextRows)
		},
		[data, getItemKey, rowSelection]
	)

	const handleSelectRow = React.useCallback(
		(row: T, index: number, checked: boolean) => {
			if (!rowSelection) return
			const key = getItemKey(row, index)
			const nextKeys = new Set(selectedKeys)
			if (checked) {
				nextKeys.add(key)
			} else {
				nextKeys.delete(key)
			}
			updateSelection(Array.from(nextKeys))
		},
		[getItemKey, rowSelection, selectedKeys, updateSelection]
	)

	const handleSelectAll = React.useCallback(
		(checked: boolean) => {
			if (!rowSelection) return
			if (checked) {
				updateSelection(selectableData.map((row, index) => getItemKey(row, data.indexOf(row))))
				return
			}
			updateSelection([])
		},
		[data, getItemKey, rowSelection, selectableData, updateSelection]
	)

	const allSelected =
		rowSelection &&
		selectableData.length > 0 &&
		selectableData.every((row) => selectedKeys.has(getItemKey(row, data.indexOf(row))))
	const someSelected =
		rowSelection &&
		selectableData.some((row) => selectedKeys.has(getItemKey(row, data.indexOf(row))))

	// 初始化列可见性
	const [columnVisibility, setColumnVisibility] = React.useState<Record<string, boolean>>(() => {
		const defaultVisibility: Record<string, boolean> = {}
		columns.forEach(col => {
			defaultVisibility[col.key] = col.visible !== false
		})

		if (storageKey && typeof window !== 'undefined') {
			const saved = window.localStorage.getItem(storageKey)
			if (saved) {
				try {
					const parsed = JSON.parse(saved)
					// 合并默认值和保存的值
					return { ...defaultVisibility, ...parsed }
				} catch (e) {
					console.error('Failed to parse column visibility:', e)
				}
			}
		}
		return defaultVisibility
	})

	const [showColumnSettings, setShowColumnSettings] = React.useState(false)
	const [showFilters, setShowFilters] = React.useState(false)

	// 保存列可见性到localStorage
	const updateColumnVisibility = (key: string, value: boolean) => {
		const newVisibility = { ...columnVisibility, [key]: value }
		setColumnVisibility(newVisibility)
		if (storageKey && typeof window !== 'undefined') {
			window.localStorage.setItem(storageKey, JSON.stringify(newVisibility))
		}
		onColumnVisibilityChange?.(newVisibility)
	}

	// 过滤数据
	const filteredData = React.useMemo(() => {
		// 如果没有提供筛选功能，返回原始数据
		if (!onFilterChange) {
			return data
		}

		// 检查是否有有效的筛选条件（非空字符串）
		const activeFilters = Object.entries(filters).filter(([_, value]) => {
			const strValue = String(value || '').trim()
			return strValue.length > 0
		})
		
		if (activeFilters.length === 0) {
			return data
		}

		const result = data.filter(row => {
			// 所有筛选条件都必须满足（AND 逻辑）
			return activeFilters.every(([key, filterValue]) => {
				const column = columns.find(col => col.key === key)
				if (!column || !column.filterable) {
					return true
				}

				const cellValue = row[key]
				const searchValue = String(filterValue || '').trim().toLowerCase()
				
				// 如果筛选值为空，不过滤
				if (searchValue === '') {
					return true
				}

				// 处理单元格值
				// 如果单元格值为 null 或 undefined，且筛选值不为空，则不匹配
				if (cellValue == null || cellValue === undefined) {
					return false
				}

				// 转换为字符串进行匹配（不区分大小写）
				const cellString = String(cellValue).trim().toLowerCase()
				
				// 执行包含匹配
				return cellString.includes(searchValue)
			})
		})
		
		return result
	}, [data, filters, columns, onFilterChange])

	// 检查是否有活动的筛选条件
	const hasActiveFilters = React.useMemo(() => {
		if (!onFilterChange) return false
		return Object.entries(filters).some(([_, value]) => {
			const strValue = String(value || '').trim()
			return strValue.length > 0
		})
	}, [filters, onFilterChange])

	// 计算活动筛选条件的数量
	const activeFilterCount = React.useMemo(() => {
		if (!onFilterChange) return 0
		return Object.entries(filters).filter(([_, value]) => {
			const strValue = String(value || '').trim()
			return strValue.length > 0
		}).length
	}, [filters, onFilterChange])

	// 可见列
	const visibleColumns = columns.filter(col => columnVisibility[col.key] !== false)

	// 固定列（左侧和右侧）
	const leftStickyColumns = visibleColumns.filter(col => col.sticky === 'left')
	const rightStickyColumns = visibleColumns.filter(col => col.sticky === 'right')
	const normalColumns = visibleColumns.filter(col => !col.sticky)

	const updateStickyShadows = React.useCallback(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) {
			setShowLeftFixedShadow(false)
			setShowRightFixedShadow(false)
			return
		}

		const maxScrollLeft = el.scrollWidth - el.clientWidth
		const hasOverflow = maxScrollLeft > 1
		setShowLeftFixedShadow(hasOverflow && el.scrollLeft > 1)
		setShowRightFixedShadow(hasOverflow && el.scrollLeft < maxScrollLeft - 1)
	}, [])

	React.useEffect(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) return

		updateStickyShadows()
		el.addEventListener('scroll', updateStickyShadows, { passive: true })
		window.addEventListener('resize', updateStickyShadows)

		return () => {
			el.removeEventListener('scroll', updateStickyShadows)
			window.removeEventListener('resize', updateStickyShadows)
		}
	}, [updateStickyShadows, visibleColumns.length, filteredData.length, loading])

	return (
		<div className={cn('space-y-4', className)}>
			{/* 工具栏 */}
			<div className="flex items-center justify-end gap-2">
				{onFilterChange && (
					<Button
						variant={hasActiveFilters ? "default" : "outline"}
						size="sm"
						onClick={() => setShowFilters(true)}
						className={cn(
							"h-8 relative",
							hasActiveFilters && "bg-primary text-primary-foreground hover:bg-primary/90"
						)}
					>
						<Filter className="w-4 h-4 mr-2" />
						筛选
						{hasActiveFilters && (
							<span className="ml-1.5 px-1.5 py-0.5 text-xs font-medium bg-primary-foreground/20 rounded-full">
								{activeFilterCount}
							</span>
						)}
					</Button>
				)}
				<Button
					variant="outline"
					size="sm"
					onClick={() => setShowColumnSettings(true)}
					className="h-8"
				>
					<Columns className="w-4 h-4 mr-2" />
					列设置
				</Button>
			</div>

			{/* 表格 */}
			<div ref={tableHostRef} className="border rounded-lg overflow-hidden relative">
				{loading && (
					<div className="absolute inset-0 bg-background/80 backdrop-blur-sm z-10 flex items-center justify-center">
						<div className="flex items-center gap-2 text-sm text-muted-foreground">
							<div className="w-5 h-5 border-2 border-primary border-t-transparent rounded-full animate-spin" />
							<span>数据加载中...</span>
						</div>
					</div>
				)}
				<div className="overflow-x-auto">
					<Table>
						<TableHeader>
							<TableRow>
								{rowSelection && (
									<TableHead className="w-[50px]">
										<Checkbox
											checked={allSelected ? true : someSelected ? 'indeterminate' : false}
											onCheckedChange={(checked) => handleSelectAll(checked === true)}
										/>
									</TableHead>
								)}
								{leftStickyColumns.map(col => (
									<TableHead
										key={col.key}
										className={cn(
											col.headerClassName,
											col.stickyClassName,
											col.sticky === 'left' && 'sticky left-0 z-20 border-r border-border bg-background',
											col.sticky === 'left' && showLeftFixedShadow && 'table-fixed-column-shadow-right'
										)}
									>
										{col.label}
									</TableHead>
								))}
								{normalColumns.map(col => (
									<TableHead key={col.key} className={col.headerClassName}>
										{col.label}
									</TableHead>
								))}
								{rightStickyColumns.map(col => (
									<TableHead
										key={col.key}
										className={cn(
											col.headerClassName,
											col.stickyClassName,
											col.sticky === 'right' && 'sticky right-0 z-20 border-l border-border bg-background',
											col.sticky === 'right' && showRightFixedShadow && 'table-fixed-column-shadow-left'
										)}
									>
										{col.label}
									</TableHead>
								))}
								{renderActions && (
									<TableHead
										className={cn(
											'w-24 sticky right-0 z-20 border-l border-border bg-background text-center',
											showRightFixedShadow && 'table-fixed-column-shadow-left',
											actionsColumnClassName
										)}
									>
										{actionsColumnLabel}
									</TableHead>
								)}
							</TableRow>
						</TableHeader>
						<TableBody>
							{!loading && filteredData.length === 0 && (
								<TableRow>
									<TableCell
										colSpan={visibleColumns.length + (renderActions ? 1 : 0) + (rowSelection ? 1 : 0)}
										className="text-center text-sm text-muted-foreground py-8"
									>
										{emptyMessage}
									</TableCell>
								</TableRow>
							)}
							{filteredData.map((row, idx) => (
								<TableRow key={idx} className="group">
									{rowSelection && (
										<TableCell className="w-[50px]">
											<Checkbox
												checked={selectedKeys.has(getItemKey(row, idx))}
												disabled={rowSelection.getCheckboxProps?.(row)?.disabled}
												onCheckedChange={(checked) => handleSelectRow(row, idx, checked === true)}
											/>
										</TableCell>
									)}
									{leftStickyColumns.map(col => (
										<TableCell
											key={col.key}
											className={cn(
												col.cellClassName,
												col.stickyClassName,
												col.sticky === 'left' && 'sticky left-0 z-10 border-r border-border bg-background group-hover:bg-muted group-data-[state=selected]:bg-muted',
													col.sticky === 'left' && showLeftFixedShadow && 'table-fixed-column-shadow-right'
											)}
										>
											{col.render
												? col.render(row[col.key], row, idx)
												: row[col.key] ?? '-'}
										</TableCell>
									))}
									{normalColumns.map(col => (
										<TableCell key={col.key} className={col.cellClassName}>
											{col.render
												? col.render(row[col.key], row, idx)
												: row[col.key] ?? '-'}
										</TableCell>
									))}
									{rightStickyColumns.map(col => (
										<TableCell
											key={col.key}
											className={cn(
												col.cellClassName,
												col.stickyClassName,
												col.sticky === 'right' && 'sticky right-0 z-10 border-l border-border bg-background group-hover:bg-muted group-data-[state=selected]:bg-muted',
												col.sticky === 'right' && showRightFixedShadow && 'table-fixed-column-shadow-left'
											)}
										>
											{col.render
												? col.render(row[col.key], row, idx)
												: row[col.key] ?? '-'}
										</TableCell>
									))}
									{renderActions && (
										<TableCell
											className={cn(
												'sticky right-0 z-10 border-l border-border bg-background text-center group-hover:bg-muted group-data-[state=selected]:bg-muted',
												showRightFixedShadow && 'table-fixed-column-shadow-left',
												actionsColumnClassName
											)}
										>
											{renderActions(row, idx)}
										</TableCell>
									)}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</div>
			</div>

			{/* 列设置对话框 */}
			<Dialog open={showColumnSettings} onOpenChange={setShowColumnSettings}>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>列设置</DialogTitle>
						<DialogDescription>选择要在表格中显示的列</DialogDescription>
					</DialogHeader>
					<div className="space-y-3 py-4">
						{columns.map((col) => (
							<div key={col.key} className="flex items-center space-x-2">
								<Checkbox
									id={`col-${col.key}`}
									checked={columnVisibility[col.key] !== false}
									onCheckedChange={(checked) => {
										updateColumnVisibility(col.key, checked as boolean)
									}}
								/>
								<Label
									htmlFor={`col-${col.key}`}
									className="text-sm font-normal cursor-pointer flex-1"
								>
									{col.label}
								</Label>
							</div>
						))}
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setShowColumnSettings(false)}>
							关闭
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			{/* 筛选对话框 */}
			{onFilterChange && (
				<Dialog open={showFilters} onOpenChange={setShowFilters}>
					<DialogContent>
						<DialogHeader>
							<DialogTitle>筛选</DialogTitle>
							<DialogDescription>根据列值筛选数据</DialogDescription>
						</DialogHeader>
						<div className="space-y-4 py-4">
							{columns
								.filter(col => col.filterable)
								.map((col) => (
									<div key={col.key}>
										<Label htmlFor={`filter-${col.key}`}>{col.label}</Label>
										<Input
											id={`filter-${col.key}`}
											value={filters[col.key] || ''}
											onChange={(e) => {
												onFilterChange({
													...filters,
													[col.key]: e.target.value,
												})
											}}
											placeholder={`筛选 ${col.label}...`}
											className="mt-2"
										/>
									</div>
								))}
						</div>
						<DialogFooter>
							<Button
								variant="outline"
								onClick={() => {
									onFilterChange({})
									setShowFilters(false)
								}}
							>
								清除
							</Button>
							<Button variant="outline" onClick={() => setShowFilters(false)}>
								关闭
							</Button>
						</DialogFooter>
					</DialogContent>
				</Dialog>
			)}
		</div>
	)
}
