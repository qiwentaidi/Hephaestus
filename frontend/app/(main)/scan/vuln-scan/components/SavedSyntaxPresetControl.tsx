'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bookmark, BookmarkPlus, Pencil, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { get, post, del } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from '@/lib/toast'

export interface SavedQueryPreset {
	id: string
	scene: string
	name: string
	query: string
	createdAt: string
	updatedAt: string
}

interface SavedSyntaxPresetControlProps {
	scene: string
	currentQuery: string
	onApply: (query: string) => void
}

export function SavedSyntaxPresetControl({
	scene,
	currentQuery,
	onApply,
}: SavedSyntaxPresetControlProps) {
	const { t } = useI18n()
	const [presets, setPresets] = useState<SavedQueryPreset[]>([])
	const [loading, setLoading] = useState(false)
	const [menuOpen, setMenuOpen] = useState(false)
	const [editorOpen, setEditorOpen] = useState(false)
	const [editingPreset, setEditingPreset] = useState<SavedQueryPreset | null>(null)
	const [deleteTarget, setDeleteTarget] = useState<SavedQueryPreset | null>(null)
	const [formName, setFormName] = useState('')
	const [formQuery, setFormQuery] = useState('')
	const [saving, setSaving] = useState(false)
	const [deleting, setDeleting] = useState(false)

	const trimmedCurrentQuery = currentQuery.trim()
	const saveDisabled = trimmedCurrentQuery.length === 0
	const presetTexts = t.vulnScan.savedQueries

	const fetchPresetList = useCallback(async () => {
		const response = await get<SavedQueryPreset[]>(`/api/v1/config/query-presets?scene=${encodeURIComponent(scene)}`)
		if (!response.success) {
			throw new Error(response.error || presetTexts.loadFailed)
		}
		return response.data || []
	}, [presetTexts.loadFailed, scene])

	const fetchPresets = useCallback(async () => {
		setLoading(true)
		try {
			setPresets(await fetchPresetList())
		} catch (error: any) {
			toast.error(error?.message || presetTexts.loadFailed)
		} finally {
			setLoading(false)
		}
	}, [fetchPresetList, presetTexts.loadFailed])

	useEffect(() => {
		let cancelled = false

		const loadPresets = async () => {
			setLoading(true)
			try {
				const nextPresets = await fetchPresetList()
				if (!cancelled) {
					setPresets(nextPresets)
				}
			} catch (error: any) {
				if (!cancelled) {
					toast.error(error?.message || presetTexts.loadFailed)
				}
			} finally {
				if (!cancelled) {
					setLoading(false)
				}
			}
		}

		void loadPresets()

		return () => {
			cancelled = true
		}
	}, [fetchPresetList, presetTexts.loadFailed])

	const orderedPresets = useMemo(() => presets, [presets])

	const openCreateDialog = () => {
		if (saveDisabled) {
			toast.error(presetTexts.emptyQuery)
			return
		}
		setEditingPreset(null)
		setFormName('')
		setFormQuery(trimmedCurrentQuery)
		setEditorOpen(true)
	}

	const openEditDialog = (preset: SavedQueryPreset) => {
		setEditingPreset(preset)
		setFormName(preset.name)
		setFormQuery(preset.query)
		setEditorOpen(true)
	}

	const handleApply = (query: string) => {
		const normalized = query.trim()
		if (!normalized) return
		onApply(normalized)
		toast.success(presetTexts.applySuccess)
	}

	const handleSave = async () => {
		const name = formName.trim()
		const query = formQuery.trim()
		if (!name) {
			toast.error(presetTexts.nameRequired)
			return
		}
		if (!query) {
			toast.error(presetTexts.emptyQuery)
			return
		}

		setSaving(true)
		try {
			const response = await post<SavedQueryPreset>('/api/v1/config/query-presets', {
				id: editingPreset?.id,
				scene,
				name,
				query,
			})
			if (!response.success) {
				throw new Error(response.error || presetTexts.saveFailed)
			}

			setEditorOpen(false)
			await fetchPresets()
			toast.success(editingPreset ? presetTexts.updateSuccess : presetTexts.saveSuccess)
		} catch (error: any) {
			toast.error(error?.message || presetTexts.saveFailed)
		} finally {
			setSaving(false)
		}
	}

	const handleDelete = async () => {
		if (!deleteTarget) return
		setDeleting(true)
		try {
			const response = await del(`/api/v1/config/query-presets/${deleteTarget.id}`)
			if (!response.success) {
				throw new Error(response.error || presetTexts.deleteFailed)
			}
			setDeleteTarget(null)
			await fetchPresets()
			toast.success(presetTexts.deleteSuccess)
		} catch (error: any) {
			toast.error(error?.message || presetTexts.deleteFailed)
		} finally {
			setDeleting(false)
		}
	}

	return (
		<>
			<div className="flex items-center gap-1 rounded-lg border border-border/70 bg-background/70 p-0.5 shadow-sm shadow-black/5 shrink-0">
				<Button
					type="button"
					variant="ghost"
					onClick={openCreateDialog}
					disabled={saveDisabled}
					className="h-8 rounded-md px-2.5 text-muted-foreground hover:bg-muted/80 hover:text-foreground"
				>
					<BookmarkPlus className="size-4" />
					{presetTexts.saveCurrent}
				</Button>
				<DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
					<DropdownMenuTrigger asChild>
						<Button
							type="button"
							variant="ghost"
							className={cn(
								'h-8 rounded-md px-2.5 text-muted-foreground hover:bg-muted/80 hover:text-foreground',
								menuOpen && 'bg-muted text-foreground'
							)}
						>
							<Bookmark className="size-4" />
							{presetTexts.presets}
						</Button>
					</DropdownMenuTrigger>
					<DropdownMenuContent align="end" className="w-[360px] overflow-hidden rounded-xl border-border/70 p-0 shadow-lg shadow-black/20">
						<div className="border-b border-border/70 bg-muted/30 px-3 py-2.5">
							<p className="text-sm font-medium">{presetTexts.title}</p>
							<p className="text-xs text-muted-foreground">{presetTexts.description}</p>
						</div>
						<div className="max-h-80 overflow-y-auto p-1.5">
							{loading ? (
								<div className="px-3 py-4 text-sm text-muted-foreground">{t.common.loading}</div>
							) : orderedPresets.length === 0 ? (
								<div className="px-3 py-4 text-sm text-muted-foreground">{presetTexts.empty}</div>
							) : (
								orderedPresets.map((preset) => (
									<div
										key={preset.id}
										className="group flex items-start gap-2 rounded-lg border border-transparent px-2.5 py-2.5 transition-colors hover:border-border/60 hover:bg-muted/50"
									>
										<button
											type="button"
											onClick={() => {
												setMenuOpen(false)
												handleApply(preset.query)
											}}
											className="min-w-0 flex-1 text-left"
										>
											<div className="truncate text-sm font-medium">{preset.name}</div>
											<div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
												{preset.query}
											</div>
										</button>
										<div className="flex items-center gap-1 pt-0.5">
											<Button
												type="button"
												size="icon"
												variant="ghost"
												className="size-7"
												onClick={(event) => {
													event.preventDefault()
													event.stopPropagation()
													setMenuOpen(false)
													openEditDialog(preset)
												}}
												aria-label={presetTexts.edit}
											>
												<Pencil className="size-3.5" />
											</Button>
											<Button
												type="button"
												size="icon"
												variant="ghost"
												className={cn('size-7 text-muted-foreground hover:text-destructive')}
												onClick={(event) => {
													event.preventDefault()
													event.stopPropagation()
													setMenuOpen(false)
													setDeleteTarget(preset)
												}}
												aria-label={t.common.delete}
											>
												<Trash2 className="size-3.5" />
											</Button>
										</div>
									</div>
								))
							)}
						</div>
					</DropdownMenuContent>
				</DropdownMenu>
			</div>

			<Dialog open={editorOpen} onOpenChange={setEditorOpen}>
				<DialogContent className="sm:max-w-xl">
					<DialogHeader>
						<DialogTitle>{editingPreset ? presetTexts.editTitle : presetTexts.saveTitle}</DialogTitle>
						<DialogDescription>{presetTexts.dialogDescription}</DialogDescription>
					</DialogHeader>
					<div className="space-y-4">
						<div className="space-y-2">
							<label className="text-sm font-medium">{presetTexts.nameLabel}</label>
							<Input
								value={formName}
								onChange={(event) => setFormName(event.target.value)}
								placeholder={presetTexts.namePlaceholder}
								maxLength={60}
							/>
						</div>
						<div className="space-y-2">
							<label className="text-sm font-medium">{presetTexts.queryLabel}</label>
							<Textarea
								value={formQuery}
								onChange={(event) => setFormQuery(event.target.value)}
								placeholder={presetTexts.queryPlaceholder}
								className="min-h-28 font-mono text-sm"
							/>
						</div>
					</div>
					<DialogFooter>
						<Button type="button" variant="outline" onClick={() => setEditorOpen(false)}>
							{t.common.cancel}
						</Button>
						<Button type="button" onClick={handleSave} disabled={saving}>
							{saving ? t.common.loading : t.common.save}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{presetTexts.deleteTitle}</AlertDialogTitle>
						<AlertDialogDescription>
							{presetTexts.deleteDescription.replace('{name}', deleteTarget?.name || '')}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t.common.cancel}</AlertDialogCancel>
						<AlertDialogAction onClick={handleDelete} disabled={deleting}>
							{t.common.delete}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</>
	)
}
