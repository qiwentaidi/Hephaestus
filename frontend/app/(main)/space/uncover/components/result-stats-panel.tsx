'use client'

import { ChevronLeft } from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import type { ResultStatsPanelProps } from '../types'

export function ResultStatsPanel({
	stats,
	title,
	sourceStatsLabel,
	portStatsLabel,
	protocolStatsLabel,
	componentStatsLabel,
	onCollapse,
}: ResultStatsPanelProps) {
	return (
		<Card className="p-4 h-full">
			<div className="mb-4 flex items-center justify-between gap-3">
				<h3 className="text-sm font-semibold">{title}</h3>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					className="h-7 px-2 text-xs text-muted-foreground hover:text-foreground"
					onClick={onCollapse}
				>
					<ChevronLeft className="h-3.5 w-3.5" />
				</Button>
			</div>
			<div className="space-y-4">
				{stats.sourceStats.length > 0 && (
					<div className="border-b border-border pb-4 last:border-0 last:pb-0">
						<h4 className="font-medium mb-2 text-xs text-muted-foreground">{sourceStatsLabel}</h4>
						<div className="space-y-1.5 max-h-[200px] overflow-y-auto">
							{stats.sourceStats.map((item, idx) => (
								<div
									key={idx}
									className="flex items-center justify-between text-xs rounded px-1.5 py-1 transition-colors hover:bg-muted/50"
								>
									<span className="truncate flex-1 mr-2" title={item.name}>
										{item.name}
									</span>
									<span className="text-muted-foreground shrink-0 font-medium">{item.count}</span>
								</div>
							))}
						</div>
					</div>
				)}

				{stats.portStats.length > 0 && (
					<div className="border-b border-border pb-4 last:border-0 last:pb-0">
						<h4 className="font-medium mb-2 text-xs text-muted-foreground">{portStatsLabel}</h4>
						<div className="space-y-1.5 max-h-[200px] overflow-y-auto">
							{stats.portStats.map((item, idx) => (
								<div
									key={idx}
									className="flex items-center justify-between text-xs rounded px-1.5 py-1 transition-colors hover:bg-muted/50"
								>
									<span className="truncate flex-1 mr-2 font-mono" title={item.name}>
										{item.name}
									</span>
									<span className="text-muted-foreground shrink-0 font-medium">{item.count}</span>
								</div>
							))}
						</div>
					</div>
				)}

				{stats.protocolStats.length > 0 && (
					<div className="border-b border-border pb-4 last:border-0 last:pb-0">
						<h4 className="font-medium mb-2 text-xs text-muted-foreground">{protocolStatsLabel}</h4>
						<div className="space-y-1.5 max-h-[200px] overflow-y-auto">
							{stats.protocolStats.map((item, idx) => (
								<div
									key={idx}
									className="flex items-center justify-between text-xs rounded px-1.5 py-1 transition-colors hover:bg-muted/50"
								>
									<span className="truncate flex-1 mr-2 font-mono" title={item.name}>
										{item.name}
									</span>
									<span className="text-muted-foreground shrink-0 font-medium">{item.count}</span>
								</div>
							))}
						</div>
					</div>
				)}

				{stats.componentStats.length > 0 && (
					<div className="border-b border-border pb-4 last:border-0 last:pb-0">
						<h4 className="font-medium mb-2 text-xs text-muted-foreground">{componentStatsLabel}</h4>
						<div className="space-y-1.5 max-h-[200px] overflow-y-auto">
							{stats.componentStats.map((item, idx) => (
								<div
									key={idx}
									className="flex items-center justify-between text-xs rounded px-1.5 py-1 transition-colors hover:bg-muted/50"
								>
									<span className="truncate flex-1 mr-2" title={item.name}>
										{item.name}
									</span>
									<span className="text-muted-foreground shrink-0 font-medium">{item.count}</span>
								</div>
							))}
						</div>
					</div>
				)}
			</div>
		</Card>
	)
}
