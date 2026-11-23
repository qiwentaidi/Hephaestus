'use client'

import { useEffect, useState } from 'react'
import Image from 'next/image'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, Container, Info, Tag } from 'lucide-react'
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
			} catch {
				// 网络错误与外层抛出的失败都归一到本地化文案，避免出现英文的
				// “Failed to fetch” 之类的浏览器原文。
				if (!cancelled) {
					setError(t.about.loadFailed)
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

	const deploymentLabel = (() => {
		const mode = aboutInfo?.app.deployment?.trim().toLowerCase()
		switch (mode) {
			case 'docker':
				return t.about.deploymentDocker
			case 'desktop':
				return t.about.deploymentDesktop
			case 'native':
				return t.about.deploymentNative
			default:
				return aboutInfo?.app.deployment?.trim() ?? ''
		}
	})()

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-[460px]">
				{/* 品牌头：图标 + 标题 + 版本胶囊 */}
				<div className="relative px-6 pt-6 pb-5">
					<div
						aria-hidden="true"
						className="pointer-events-none absolute -top-24 -left-6 h-44 w-80 rounded-full bg-primary/10 blur-3xl"
					/>
					<div className="relative flex items-center gap-3.5 pr-8">
						<div className="flex size-14 shrink-0 items-center justify-center rounded-2xl border border-border/70 bg-gradient-to-br from-muted/60 to-transparent shadow-sm">
							<Image
								src="/brand-mark.png"
								alt="Hephaestus"
								width={40}
								height={40}
								unoptimized
								className="app-brand-mark-image size-10"
							/>
						</div>
						<div className="min-w-0">
							<DialogTitle className="text-xl font-semibold tracking-tight">
								{t.about.title}
							</DialogTitle>
							<DialogDescription className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
								{t.about.description}
							</DialogDescription>
						</div>
					</div>
				</div>

				{loading && (
					<>
						<div className="mx-6 overflow-hidden rounded-xl border border-border/60 bg-muted/20">
							<div className="flex items-center justify-between px-3.5 py-3">
								<Skeleton className="h-3.5 w-16" />
								<Skeleton className="h-3.5 w-14" />
							</div>
							<div className="flex items-center justify-between border-t border-border/60 px-3.5 py-3">
								<Skeleton className="h-3.5 w-20" />
								<Skeleton className="h-3.5 w-16" />
							</div>
						</div>
						<div className="mt-4 border-t border-border/60 bg-muted/20 px-6 py-3.5">
							<Skeleton className="h-3 w-16" />
							<Skeleton className="mt-2.5 h-3 w-56" />
						</div>
					</>
				)}

				{error && (
					<div className="px-6 pb-6">
						<div className="flex items-start gap-2 rounded-xl border border-destructive/25 bg-destructive/10 px-3.5 py-3">
							<AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" aria-hidden="true" />
							<p className="text-sm text-destructive">{error}</p>
						</div>
					</div>
				)}

				{!loading && !error && aboutInfo && (
					<>
						<div className="mx-6 overflow-hidden rounded-xl border border-border/60 bg-muted/25">
							<div className="flex items-center justify-between gap-3 px-3.5 py-2.5">
								<span className="flex items-center gap-2 text-xs text-muted-foreground">
									<Tag className="size-3.5" aria-hidden="true" />
									{t.about.version}
								</span>
								<span className="font-mono text-xs font-medium tabular-nums">
									{aboutInfo.app.version}
								</span>
							</div>
							{deploymentLabel && (
								<div className="flex items-center justify-between gap-3 border-t border-border/60 px-3.5 py-2.5">
									<span className="flex items-center gap-2 text-xs text-muted-foreground">
										<Container className="size-3.5" aria-hidden="true" />
										{t.about.deployment}
									</span>
									<span className="flex items-center gap-1.5 text-[13px] font-medium">
										<span className="size-1.5 rounded-full bg-primary/80" aria-hidden="true" />
										{deploymentLabel}
									</span>
								</div>
							)}
						</div>

						<div className="mt-4 border-t border-border/60 bg-muted/20 px-6 py-3.5">
							<div className="flex items-center gap-1.5 text-[11.5px] font-medium text-muted-foreground">
								<Info className="size-3.5" aria-hidden="true" />
								{t.about.copyright.title}
							</div>
							<p className="mt-1 text-xs leading-relaxed text-muted-foreground">
								{aboutInfo.copyright.text}
							</p>
						</div>
					</>
				)}
			</DialogContent>
		</Dialog>
	)
}
