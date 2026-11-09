'use client'

import { useEffect, useState } from 'react'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Info, Shield, AlertCircle } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

interface AboutInfo {
	app: {
		name: string
		version: string
		deployment?: string
	}
	copyright: {
		year: string
		company: string
		text: string
	}
}

interface AboutDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
}

export function AboutDialog({ open, onOpenChange }: AboutDialogProps) {
	const { t } = useI18n()
	const [aboutInfo, setAboutInfo] = useState<AboutInfo | null>(null)
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		if (!open) return

		let cancelled = false

		const loadAboutInfo = async () => {
			setLoading(true)
			setError(null)
			try {
				const response = await fetchWithAuth(`/api/v1/system/info`)
				if (!response.ok) {
					throw new Error(t.about.loadFailed)
				}
				const data = await response.json()
				if (!cancelled) {
					setAboutInfo(data)
				}
			} catch (err) {
				if (!cancelled) {
					setError(err instanceof Error ? err.message : t.about.loadFailed)
				}
			} finally {
				if (!cancelled) {
					setLoading(false)
				}
			}
		}

		void loadAboutInfo()

		return () => {
			cancelled = true
		}
	}, [open, t.about.loadFailed])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-[500px]">
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						<Shield className="w-5 h-5" />
						{t.about.title}
					</DialogTitle>
					<DialogDescription>{t.about.description}</DialogDescription>
				</DialogHeader>

				{loading && (
					<div className="space-y-4 py-4">
						<Skeleton className="h-20 w-full" />
						<Skeleton className="h-32 w-full" />
						<Skeleton className="h-24 w-full" />
					</div>
				)}

				{error && (
					<div className="py-4">
						<div className="flex items-center gap-2 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
							<AlertCircle className="w-4 h-4 text-destructive shrink-0" />
							<p className="text-sm text-destructive">{error}</p>
						</div>
					</div>
				)}

				{!loading && !error && aboutInfo && (
					<div className="space-y-6 py-4">
						{/* 应用版本信息 */}
						<div className="space-y-3">
							<div className="flex items-center justify-between">
								<div className="flex items-center gap-3">
									<h3 className="text-xl font-bold">{aboutInfo.app.name}</h3>
									{aboutInfo.app.deployment === 'docker' && <Badge variant="secondary">Docker</Badge>}
										<Badge variant="secondary" className="text-sm">
										{aboutInfo.app.version}
									</Badge>
								</div>
							</div>
						</div>

						<Separator />

						{/* 版权信息 */}
						<div className="space-y-2">
							<h4 className="text-sm font-semibold flex items-center gap-2">
								<Info className="w-4 h-4" />
								{t.about.copyright.title}
							</h4>
							<p className="text-sm text-muted-foreground">{aboutInfo.copyright.text}</p>
						</div>

					</div>
				)}
			</DialogContent>
		</Dialog>
	)
}
