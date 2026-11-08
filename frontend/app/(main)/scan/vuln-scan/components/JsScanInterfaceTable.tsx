'use client'
import { DetailResultToolbar } from './DetailResultToolbar'

import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import {
	Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'
import {
	Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from '@/components/ui/table'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

interface JSInterfaceParameter {
	name: string
	in: string
}

interface JSInterface {
	method: string
	url: string
	targetUrl: string
	parameters: JSInterfaceParameter[]
}

interface JsScanInterfaceTableProps {
	taskId?: string
	version?: number
	onTotalChange?: (total: number) => void
}

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS', 'TRACE']

export function JsScanInterfaceTable({ taskId, version, onTotalChange }: JsScanInterfaceTableProps) {
	const { t } = useI18n()
	const labels = t.jsScan.interfaces
	const [keyword, setKeyword] = useState('')
	const [method, setMethod] = useState('all')
	const [target, setTarget] = useState('all')
	const [targets, setTargets] = useState<string[]>([])
	const [page, setPage] = useState(1)
	const [pageSize, setPageSize] = useState(20)
	const [items, setItems] = useState<JSInterface[]>([])
	const [total, setTotal] = useState(0)
	const [loading, setLoading] = useState(false)
	const [error, setError] = useState(false)

	useEffect(() => {
		if (!taskId) return
		const controller = new AbortController()
		const timer = window.setTimeout(async () => {
			setLoading(true)
			setError(false)
			try {
				const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
				if (version !== undefined) params.set('version', String(version))
				if (keyword.trim()) params.set('keyword', keyword.trim())
				if (method !== 'all') params.set('method', method)
				if (target !== 'all') params.set('target', target)
				const response = await fetchWithAuth(`/api/v1/jsscan/interfaces/${taskId}?${params}`, { signal: controller.signal })
				if (!response.ok) throw new Error(`HTTP ${response.status}`)
				const result = await response.json()
				if (!result.success) throw new Error(result.error || 'Failed to load interfaces')
				if (controller.signal.aborted) return
				setItems(result.data?.interfaces || [])
				setTotal(result.data?.total || 0)
				setTargets(result.data?.targets || [])
				onTotalChange?.(result.data?.totalAll || 0)
			} catch (cause) {
				if (controller.signal.aborted) return
				console.error('Failed to fetch JS interfaces:', cause)
				setItems([])
				setError(true)
			} finally {
				if (!controller.signal.aborted) setLoading(false)
			}
		}, 200)
		return () => {
			window.clearTimeout(timer)
			controller.abort()
		}
	}, [taskId, version, keyword, method, target, page, pageSize, onTotalChange])

	const resetPage = () => setPage(1)

	return (
		<div className="space-y-4">
			<DetailResultToolbar activeCount={Number(Boolean(keyword.trim())) + Number(method !== 'all') + Number(target !== 'all')} onReset={() => { setKeyword(''); setMethod('all'); setTarget('all'); resetPage() }}>
<div className="flex flex-wrap items-center gap-3">
				<Input
					value={keyword}
					onChange={(event) => { setKeyword(event.target.value); resetPage() }}
					placeholder={labels.searchPlaceholder}
					className="min-w-[240px] max-w-md flex-1"
				/>
				<Select value={method} onValueChange={(value) => { setMethod(value); resetPage() }}>
					<SelectTrigger className="w-[140px]"><SelectValue placeholder={labels.allMethods} /></SelectTrigger>
					<SelectContent>
						<SelectItem value="all">{labels.allMethods}</SelectItem>
						{methods.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}
					</SelectContent>
				</Select>
				<Select value={target} onValueChange={(value) => { setTarget(value); resetPage() }}>
					<SelectTrigger className="max-w-[300px] min-w-[180px]"><SelectValue placeholder={labels.allTargets} /></SelectTrigger>
					<SelectContent>
						<SelectItem value="all">{labels.allTargets}</SelectItem>
						{targets.map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}
					</SelectContent>
				</Select>
			</div>
</DetailResultToolbar>
			<Table>
				<TableHeader>
					<TableRow>
						<TableHead className="w-[110px]">{labels.method}</TableHead>
						<TableHead className="min-w-[360px]">{labels.url}</TableHead>
						<TableHead className="min-w-[280px]">{labels.parameters}</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody>
					{items.map((item) => (
						<TableRow key={`${item.targetUrl}\u0000${item.method}\u0000${item.url}`}>
							<TableCell><Badge variant="outline" className="font-mono">{item.method}</Badge></TableCell>
							<TableCell className="break-all font-mono text-xs" title={item.url}>{item.url}</TableCell>
							<TableCell>
								{item.parameters.length === 0 ? <span className="text-muted-foreground">—</span> : (
									<div className="flex flex-wrap gap-1.5">
										{item.parameters.map((parameter) => (
											<Badge key={`${parameter.in}\u0000${parameter.name}`} variant="secondary" className="font-normal">
												{parameter.name} ({parameter.in})
											</Badge>
										))}
									</div>
								)}
							</TableCell>
						</TableRow>
					))}
					{items.length === 0 && (
						<TableRow><TableCell colSpan={3} className="py-12 text-center text-muted-foreground">
							{loading ? labels.loading : error ? labels.loadFailed : labels.empty}
						</TableCell></TableRow>
					)}
				</TableBody>
			</Table>
			<PaginationPro
				current={page}
				total={total}
				pageSize={pageSize}
				onChange={(nextPage, nextSize) => { setPage(nextPage); setPageSize(nextSize) }}
				showSizeChanger
				showQuickJumper
				showTotal
			/>
		</div>
	)
}
