'use client'

import {
	ChevronLeft,
	ChevronRight,
	ChevronsDown,
	Loader2,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import type { ResultToolbarProps } from '../types'

export function ResultToolbar({
	tab,
	statsCollapsed,
	onToggleStats,
	onLoadMore,
	statsTitle,
	canLoadMoreLabel,
	cacheLimitReachedLabel,
	queryFailedLabel,
	loadMoreLabel,
	loadingMoreLabel,
	getEngineDisplayName,
	getEngineStatusBadgeText,
	formatText,
}: ResultToolbarProps) {
	const showLoadMore = tab.hasMore && !tab.reachedMaxLimit

	return (
		<div className="flex items-start justify-between gap-3 text-sm mb-4 pb-4 border-b">
			<div className="flex min-w-0 flex-1 items-center gap-2 flex-wrap">
				<Button
					type="button"
					variant="ghost"
					size="sm"
					className="border border-border/60 bg-muted/35 px-3 text-foreground shadow-none hover:bg-muted/60"
					onClick={onToggleStats}
				>
					{statsCollapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronLeft className="h-4 w-4" />}
					{statsTitle}
				</Button>
				{tab.engineStatus && Object.entries(tab.engineStatus).map(([name, status]) => {
					if (!status.success) return null
					return (
						<Badge key={name} variant="secondary" className="text-xs">
							{getEngineStatusBadgeText(tab, name, status)}
						</Badge>
					)
				})}
				{tab.hasMore && !tab.reachedMaxLimit && (
					<Badge variant="outline" className="text-xs text-blue-500 border-blue-500">
						{canLoadMoreLabel}
					</Badge>
				)}
				{tab.reachedMaxLimit && (
					<Badge variant="destructive" className="text-xs">
						{formatText(cacheLimitReachedLabel, { count: tab.maxResults || 0 })}
					</Badge>
				)}
				{tab.engineStatus &&
					Object.entries(tab.engineStatus)
						.filter(([, status]) => !status.success)
						.map(([name, status]) => {
							const engineLabel = getEngineDisplayName(name)
							const message = (status.message || queryFailedLabel).trim()
							return (
								<div
									key={`${name}-error`}
									className="w-full text-xs rounded-md border border-destructive/30 bg-destructive/10 px-2 py-1.5 text-destructive/90 break-all"
								>
									<span className="font-medium mr-1">{engineLabel}</span>
									{message}
								</div>
							)
						})}
			</div>

			{showLoadMore && (
				<div className="flex shrink-0 items-center gap-2">
					<Button
						variant="ghost"
						size="sm"
						className="border border-border/60 bg-muted/35 px-4 text-foreground shadow-none hover:bg-muted/60"
						onClick={() => onLoadMore(tab.id)}
						disabled={tab.loadingMore}
					>
						{tab.loadingMore ? (
							<>
								<Loader2 className="h-4 w-4 mr-1 animate-spin" />
								{loadingMoreLabel}
							</>
						) : (
							<>
								<ChevronsDown className="h-4 w-4 mr-1" />
								{loadMoreLabel}
							</>
						)}
					</Button>
				</div>
			)}
		</div>
	)
}
