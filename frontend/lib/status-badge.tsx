import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { Loader2 } from 'lucide-react'

const statusBadgeBase =
	'bg-transparent px-2 py-0.5 rounded-md font-medium border'

export function getStatusBadgeClassName(status: number) {
	if (status >= 200 && status < 300) {
		return cn(statusBadgeBase, 'border-tone-green-border text-tone-green bg-tone-green-soft')
	}
	if (status >= 300 && status < 400) {
		return cn(statusBadgeBase, 'border-tone-blue-border text-tone-blue bg-tone-blue-soft')
	}
	if (status >= 400 && status < 500) {
		return cn(statusBadgeBase, 'border-tone-amber-border text-tone-amber bg-tone-amber-soft')
	}
	if (status >= 500) {
		return cn(statusBadgeBase, 'border-tone-red-border text-tone-red bg-tone-red-soft')
	}
	return cn(statusBadgeBase, 'border-tone-slate-border text-tone-slate bg-tone-slate-soft')
}

export interface TaskStatusLabels {
	pending: string
	running: string
	pausing?: string
	paused?: string
	recovering?: string
	deleting?: string
	completed: string
	failed: string
	cancelled: string
}

export function getStatusBadge(status: string, labels?: TaskStatusLabels) {
	const fallbackLabels: TaskStatusLabels = {
		pending: 'Pending',
		running: 'Running',
		pausing: 'Pausing',
		paused: 'Paused',
		recovering: 'Recovering',
		deleting: 'Deleting',
		completed: 'Completed',
		failed: 'Failed',
		cancelled: 'Cancelled',
	}
	const statusLabels = labels || fallbackLabels
	switch (status) {
		case 'pending':
			return <Badge variant="outline" className="border-tone-amber-border bg-tone-amber-soft text-tone-amber">{statusLabels.pending}</Badge>
		case 'running':
			return <Badge variant="outline" className="border-primary/25 bg-primary/10 text-primary">{statusLabels.running}</Badge>
		case 'pausing':
			return (
				<Badge variant="outline" className="border-tone-amber-border bg-tone-amber-soft text-tone-amber">
					<span className="inline-flex items-center gap-1.5">
						<Loader2 className="h-3 w-3 animate-spin" />
						{statusLabels.pausing || fallbackLabels.pausing}
					</span>
				</Badge>
			)
		case 'paused':
			return <Badge variant="outline" className="border-tone-blue-border bg-tone-blue-soft text-tone-blue">{statusLabels.paused || fallbackLabels.paused}</Badge>
		case 'recovering':
			return <Badge variant="outline" className="border-tone-purple-border bg-tone-purple-soft text-tone-purple">{statusLabels.recovering || fallbackLabels.recovering}</Badge>
		case 'deleting':
			return (
				<Badge variant="outline" className="border-tone-slate-border bg-tone-slate-soft text-tone-slate">
					<span className="inline-flex items-center gap-1.5">
						<Loader2 className="h-3 w-3 animate-spin" />
						{statusLabels.deleting || fallbackLabels.deleting}
					</span>
				</Badge>
			)
		case 'completed':
			return <Badge variant="outline" className="border-tone-green-border bg-tone-green-soft text-tone-green">{statusLabels.completed}</Badge>
		case 'failed':
			return <Badge variant="outline" className="border-tone-red-border bg-tone-red-soft text-tone-red">{statusLabels.failed}</Badge>
		case 'cancelled':
			return <Badge variant="outline" className="border-tone-slate-border bg-tone-slate-soft text-tone-slate">{statusLabels.cancelled}</Badge>
		default:
			return <Badge variant="outline">{status}</Badge>
	}
}

export function StatusBadge({
	status,
	className,
}: {
	status: number
	className?: string
}) {
	return (
		<Badge variant="outline" className={cn(getStatusBadgeClassName(status), className)}>
			{status}
		</Badge>
	)
}
