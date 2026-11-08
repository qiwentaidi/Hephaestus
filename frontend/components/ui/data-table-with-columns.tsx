'use client'

import { useState, ReactNode, useEffect, useRef, useCallback } from 'react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
	DropdownMenu,
	DropdownMenuCheckboxItem,
	DropdownMenuContent,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { Settings, ArrowUp, ArrowDown, ArrowUpDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useIsMobile } from '@/hooks/use-mobile'
import { useI18n } from '@/lib/i18n'

export interface ColumnDef<T> {
	id: string
	header: string | ReactNode | (() => ReactNode)
	accessorKey?: keyof T
	cell?: (item: T, index?: number) => ReactNode
	className?: string
	headerClassName?: string
	sortable?: boolean
	defaultVisible?: boolean
	alwaysVisible?: boolean // 始终显示，不可隐藏
}

// 行选择配置
export interface RowSelection<T> {
	// 选中行的 key 列表
	selectedRowKeys?: (string | number)[]
	// 选中状态变化时的回调
	onChange?: (selectedRowKeys: (string | number)[], selectedRows: T[]) => void
	// 获取行的禁用状态
	getCheckboxProps?: (record: T) => { disabled?: boolean }
	// 选择类型：checkbox 或 radio
	type?: 'checkbox' | 'radio'
}

export interface DataTableWithColumnsProps<T> {
	/** Use labeled cards on narrow screens while preserving column renderers. */
	mobileCards?: boolean
	columns: ColumnDef<T>[]
	data: T[]
	loading?: boolean
	emptyMessage?: string
	onSort?: (columnId: string, order: 'asc' | 'desc' | null) => void
	sortState?: { columnId: string; order: 'asc' | 'desc' | null }
	showColumnToggle?: boolean
	className?: string
	getRowKey?: (item: T, index: number) => string | number
	// 工具栏插槽
	toolbarLeft?: ReactNode // 工具栏左侧内容（如统计信息）
	toolbarRight?: ReactNode // 工具栏右侧内容（在列设置按钮左边，如导出、筛选等按钮）
	showToolbar?: boolean // 是否显示工具栏（默认为 true，当有 toolbarLeft、toolbarRight 或 showColumnToggle 时）
	// 行选择配置
	rowSelection?: RowSelection<T>
	// 自定义行渲染（用于添加右键菜单等）
	renderRow?: (item: T, index: number, defaultRow: ReactNode) => ReactNode
}

export function DataTableWithColumns<T extends Record<string, any>>({
	columns,
	data,
	mobileCards = false,
	loading = false,
	emptyMessage = '暂无数据',
	onSort,
	sortState,
	showColumnToggle = true,
	className,
	getRowKey,
	toolbarLeft,
	toolbarRight,
	showToolbar,
	rowSelection,
	renderRow,
}: DataTableWithColumnsProps<T>) {
	const isMobile = useIsMobile()
	const { t } = useI18n()
	const tableHostRef = useRef<HTMLDivElement | null>(null)
	const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)
	// 内部选中状态（当外部未提供 selectedRowKeys 时使用）
	const [internalSelectedKeys, setInternalSelectedKeys] = useState<Set<string | number>>(new Set())
	
	// 使用外部状态或内部状态
	const selectedKeys = rowSelection?.selectedRowKeys 
		? new Set(rowSelection.selectedRowKeys) 
		: internalSelectedKeys

	// 初始化列可见性状态
	const [columnVisibility, setColumnVisibility] = useState<Record<string, boolean>>(() => {
		const initial: Record<string, boolean> = {}
		columns.forEach((col) => {
			initial[col.id] = col.defaultVisible !== false
		})
		return initial
	})

	// 获取行的 key
	const getItemKey = (item: T, index: number): string | number => {
		return getRowKey ? getRowKey(item, index) : index
	}

	// 获取可选择的行（排除禁用的行）
	const selectableData = data.filter((item, index) => {
		if (!rowSelection?.getCheckboxProps) return true
		const props = rowSelection.getCheckboxProps(item)
		return !props?.disabled
	})

	// 处理单行选择
	const handleSelectRow = (key: string | number, item: T, checked: boolean) => {
		if (!rowSelection) return

		let newSelectedKeys: (string | number)[]
		let newSelectedRows: T[]

		if (rowSelection.type === 'radio') {
			// 单选模式
			newSelectedKeys = checked ? [key] : []
			newSelectedRows = checked ? [item] : []
		} else {
			// 多选模式
			const newSet = new Set(selectedKeys)
			if (checked) {
				newSet.add(key)
			} else {
				newSet.delete(key)
			}
			newSelectedKeys = Array.from(newSet)
			newSelectedRows = data.filter((d, i) => newSet.has(getItemKey(d, i)))
		}

		// 如果没有提供外部状态，更新内部状态
		if (!rowSelection.selectedRowKeys) {
			setInternalSelectedKeys(new Set(newSelectedKeys))
		}

		// 触发回调
		rowSelection.onChange?.(newSelectedKeys, newSelectedRows)
	}

	// 处理全选
	const handleSelectAll = (checked: boolean) => {
		if (!rowSelection || rowSelection.type === 'radio') return

		let newSelectedKeys: (string | number)[]
		let newSelectedRows: T[]

		if (checked) {
			// 选中所有可选择的行
			newSelectedKeys = selectableData.map((item, index) => {
				// 找到原始索引
				const originalIndex = data.indexOf(item)
				return getItemKey(item, originalIndex)
			})
			newSelectedRows = [...selectableData]
		} else {
			newSelectedKeys = []
			newSelectedRows = []
		}

		// 如果没有提供外部状态，更新内部状态
		if (!rowSelection.selectedRowKeys) {
			setInternalSelectedKeys(new Set(newSelectedKeys))
		}

		// 触发回调
		rowSelection.onChange?.(newSelectedKeys, newSelectedRows)
	}

	// 计算全选状态
	const allSelected = selectableData.length > 0 && selectableData.every((item, i) => {
		const originalIndex = data.indexOf(item)
		return selectedKeys.has(getItemKey(item, originalIndex))
	})
	const someSelected = selectableData.some((item, i) => {
		const originalIndex = data.indexOf(item)
		return selectedKeys.has(getItemKey(item, originalIndex))
	})

	// 构建选择列
	const selectionColumn: ColumnDef<T> | null = rowSelection ? {
		id: '__selection__',
		header: () => {
			if (rowSelection.type === 'radio') {
				return null
			}
			return (
				<div>
					<Checkbox
						checked={allSelected ? true : someSelected ? 'indeterminate' : false}
						onCheckedChange={handleSelectAll}
					/>
				</div>
			)
		},
		cell: (item: T, index?: number) => {
			const key = getItemKey(item, index ?? 0)
			const isSelected = selectedKeys.has(key)
			const checkboxProps = rowSelection.getCheckboxProps?.(item)
			const isDisabled = checkboxProps?.disabled ?? false

			return (
				<div>
					<Checkbox
						checked={isSelected}
						disabled={isDisabled}
						onCheckedChange={(checked) => handleSelectRow(key, item, checked === true)}
					/>
				</div>
			)
		},
		headerClassName: 'w-[50px]',
		className: 'w-[50px]',
		alwaysVisible: true,
	} : null

	// 获取可见的列（包含选择列）
	const visibleColumns = [
		...(selectionColumn ? [selectionColumn] : []),
		...columns.filter((col) => col.alwaysVisible || columnVisibility[col.id])
	]

	// 获取可切换的列（排除 alwaysVisible 的列）
	const toggleableColumns = columns.filter((col) => !col.alwaysVisible)

	// 处理排序
	const handleSort = (columnId: string) => {
		if (!onSort) return

		let newOrder: 'asc' | 'desc' | null = 'desc'
		if (sortState?.columnId === columnId) {
			if (sortState.order === 'desc') {
				newOrder = 'asc'
			} else if (sortState.order === 'asc') {
				newOrder = null
			}
		}
		onSort(columnId, newOrder)
	}

	// 渲染表头
	const renderTableHeader = (col: ColumnDef<T>) => {
		const isSorted = sortState?.columnId === col.id

		if (col.sortable && onSort) {
			return (
				<Button
					variant="ghost"
					size="sm"
					className="h-8 px-2"
					onClick={() => handleSort(col.id)}
				>
					{typeof col.header === 'function' ? col.header() : col.header}
					{isSorted && sortState?.order === 'asc' ? (
						<ArrowUp className="ml-2 h-4 w-4" />
					) : isSorted && sortState?.order === 'desc' ? (
						<ArrowDown className="ml-2 h-4 w-4" />
					) : col.sortable ? (
						<ArrowUpDown className="ml-2 h-4 w-4 opacity-50" />
					) : null}
				</Button>
			)
		}

		return typeof col.header === 'function' ? col.header() : col.header
	}

	// 渲染单元格内容
	const renderCell = (col: ColumnDef<T>, item: T, index: number) => {
		if (col.cell) {
			return col.cell(item, index)
		}
		if (col.accessorKey) {
			return item[col.accessorKey] ?? '-'
		}
		return '-'
	}

	// 判断是否需要显示工具栏
	const shouldShowToolbar = showToolbar ?? (!!toolbarLeft || !!toolbarRight || (showColumnToggle && toggleableColumns.length > 0))
	
	// 判断工具栏内容分布
	const hasLeft = !!toolbarLeft
	const hasRight = !!toolbarRight || (showColumnToggle && toggleableColumns.length > 0)

	const updateRightFixedShadow = useCallback(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) {
			setShowRightFixedShadow(false)
			return
		}

		const maxScrollLeft = el.scrollWidth - el.clientWidth
		setShowRightFixedShadow(maxScrollLeft > 1 && el.scrollLeft < maxScrollLeft - 1)
	}, [])

	useEffect(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) return

		updateRightFixedShadow()
		el.addEventListener('scroll', updateRightFixedShadow, { passive: true })
		window.addEventListener('resize', updateRightFixedShadow)

		return () => {
			el.removeEventListener('scroll', updateRightFixedShadow)
			window.removeEventListener('resize', updateRightFixedShadow)
		}
	}, [updateRightFixedShadow, visibleColumns.length, data.length, isMobile, mobileCards])

	const stickyHeaderShadowClass = showRightFixedShadow ? 'table-fixed-column-shadow-left' : ''

	const isRightStickyColumn = (col: ColumnDef<T>) =>
		`${col.className || ''} ${col.headerClassName || ''}`.includes('sticky right-0')

	return (
		<div className={cn('space-y-4', className)}>
			{shouldShowToolbar && (
				<div className={cn(
					'flex flex-wrap items-center gap-4',
					hasLeft && hasRight && 'justify-between',
					hasLeft && !hasRight && 'justify-start',
					!hasLeft && hasRight && 'justify-end'
				)}>
					{hasLeft && <div className="flex-1">{toolbarLeft}</div>}
					{hasRight && (
						<div className="flex items-center gap-2">
							{toolbarRight}
							{showColumnToggle && toggleableColumns.length > 0 && (
								<DropdownMenu>
									<DropdownMenuTrigger asChild>
										<Button variant="outline" size="sm">
											<Settings className="w-4 h-4" />
										</Button>
									</DropdownMenuTrigger>
									<DropdownMenuContent align="end" className="w-[180px]">
										<DropdownMenuLabel>显示列</DropdownMenuLabel>
										<DropdownMenuSeparator />
										{toggleableColumns.map((col) => (
											<DropdownMenuCheckboxItem
												key={col.id}
												checked={columnVisibility[col.id]}
												onCheckedChange={(checked) =>
													setColumnVisibility((prev) => ({ ...prev, [col.id]: checked }))
												}
											>
												{typeof col.header === 'string' ? col.header : col.id}
											</DropdownMenuCheckboxItem>
										))}
									</DropdownMenuContent>
								</DropdownMenu>
							)}
						</div>
					)}
				</div>
			)}

			<div ref={tableHostRef} className="rounded-lg border overflow-hidden">
				{mobileCards && isMobile ? (
				  <div className="divide-y">
				    {(selectionColumn || visibleColumns.some(col => col.sortable)) && (
				      <div className="flex flex-wrap items-center gap-2 bg-muted/30 p-3">
				        {selectionColumn && rowSelection?.type !== 'radio' && (
				          <label className="flex min-h-11 items-center gap-2">
				            <Checkbox aria-label={t.common.selectAll}
				              checked={allSelected ? true : someSelected ? 'indeterminate' : false}
				              onCheckedChange={handleSelectAll} />
				            {t.common.selectAll}
				          </label>
				        )}
				        {visibleColumns.filter(col => col.sortable).map(col => (
				          <div key={col.id}>{renderTableHeader(col)}</div>
				        ))}
				      </div>
				    )}
				    {loading ? (
				      <p role="status" className="p-8 text-center text-muted-foreground">{t.common.loading}</p>
				    ) : data.length === 0 ? (
				      <p className="p-8 text-center text-muted-foreground">{emptyMessage}</p>
				    ) : data.map((item, index) => {
				      const card = (
				        <article key={getItemKey(item, index)} className={cn('space-y-3 p-4', selectedKeys.has(getItemKey(item, index)) && 'bg-muted/40')}>
				          {visibleColumns.map(col => (
				            <div key={col.id} className="grid min-w-0 grid-cols-[5rem_minmax(0,1fr)] gap-3 text-sm">
				              <div className="text-xs text-muted-foreground">
				                {col.id === '__selection__' ? t.common.select : typeof col.header === 'function' ? col.header() : col.header}
				              </div>
				              <div className="min-w-0 break-words [&_button:not([role=checkbox])]:min-h-11">
				                {renderCell(col, item, index)}
				              </div>
				            </div>
				          ))}
				        </article>
				      )
				      return renderRow ? renderRow(item, index, card) : card
				    })}
				  </div>
				) : <Table>
					<TableHeader>
						<TableRow>
							{visibleColumns.map((col, colIndex) => (
								<TableHead 
									key={col.id} 
									className={cn(
											col.headerClassName,
											isRightStickyColumn(col) && stickyHeaderShadowClass
									)}
								>
									{renderTableHeader(col)}
								</TableHead>
							))}
						</TableRow>
					</TableHeader>
					<TableBody>
						{loading ? (
							<TableRow>
								<TableCell colSpan={visibleColumns.length}>
									<div className="py-8 text-center text-muted-foreground">加载中...</div>
								</TableCell>
							</TableRow>
						) : data.length === 0 ? (
							<TableRow>
								<TableCell colSpan={visibleColumns.length}>
									<div className="py-10 text-center text-muted-foreground">{emptyMessage}</div>
								</TableCell>
							</TableRow>
						) : (
							data.map((item, index) => {
								const defaultRow = (
									<TableRow key={getRowKey ? getRowKey(item, index) : index} className="group">
										{visibleColumns.map((col, colIndex) => (
											<TableCell 
												key={col.id} 
											className={cn(
													col.className,
													isRightStickyColumn(col) &&
														showRightFixedShadow &&
														'table-fixed-column-shadow-left'
											)}
											>
												{renderCell(col, item, index)}
											</TableCell>
										))}
									</TableRow>
								)
								
								return renderRow ? renderRow(item, index, defaultRow) : defaultRow
							})
						)}
					</TableBody>
				</Table>}
			</div>
		</div>
	)
}
