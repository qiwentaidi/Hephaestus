'use client'

import { useIsMobile } from '@/hooks/use-mobile'
import { MobileSearchActions } from '../../components/mobile-search-actions'

import { useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { List, Search, Settings, Star, User, BookOpen } from 'lucide-react'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { HunterUserInfo, SuggestionItem } from '../types'
import { useI18n } from '@/lib/i18n'

interface SearchFormCardProps {
	query: string
	setQuery: Dispatch<SetStateAction<string>>
	isSearching: boolean
	onStartSearch: () => void
	userInfo: HunterUserInfo | null
	isLoadingUserInfo: boolean
	onFetchUserInfo: (showDialog?: boolean) => Promise<void>
	onOpenApiConfigDialog: () => void
	onOpenTipsDialog: () => void
	onOpenFavSyntaxDialog: () => void
	onOpenAddFavDialog: () => void
	showSuggestions: boolean
	setShowSuggestions: Dispatch<SetStateAction<boolean>>
	suggestions: SuggestionItem[]
	inputFocused: boolean
	setInputFocused: Dispatch<SetStateAction<boolean>>
	selectedSuggestionIndex: number
	setSelectedSuggestionIndex: Dispatch<SetStateAction<number>>
	onSelectSuggestion: (syntaxKey: string) => void
	suggestionsRef: MutableRefObject<HTMLDivElement | null>
	textareaRef: MutableRefObject<HTMLTextAreaElement | null>
}

const renderQuota = (value?: number) => {
	if (value === -1 || value === undefined) return null
	return (value ?? 0).toLocaleString()
}

export function SearchFormCard(props: SearchFormCardProps) {
	const { t } = useI18n()
	const isMobile = useIsMobile()
	const isComposingRef = useRef(false)

	const {
		query,
		setQuery,
		isSearching,
		onStartSearch,
		userInfo,
		isLoadingUserInfo,
		onFetchUserInfo,
		onOpenTipsDialog,
		onOpenFavSyntaxDialog,
		onOpenAddFavDialog,
		showSuggestions,
		setShowSuggestions,
		suggestions,
		inputFocused,
		setInputFocused,
		selectedSuggestionIndex,
		setSelectedSuggestionIndex,
		onSelectSuggestion,
		suggestionsRef,
		textareaRef,
	} = props

	const availablePointText = (() => {
		if (!userInfo) return null
		const equity = userInfo.data.rest_equity_point
		const free = userInfo.data.rest_free_point
		if (equity === -1 || free === -1) return t.hunterPage.searchForm.unlimited
		return Math.max(0, (equity ?? 0) + (free ?? 0)).toLocaleString()
	})()
	const renderQuotaLabel = (value?: number) => renderQuota(value) ?? t.hunterPage.searchForm.unlimited
	const formatText = (template: string, vars: Record<string, string | number>) =>
		Object.entries(vars).reduce((acc, [key, value]) => acc.replace(`{${key}}`, String(value)), template)

	const accountContent = userInfo ? (
		<div className="space-y-4">
			<div className="flex items-center gap-3">
				<Avatar className="w-12 h-12">
					<AvatarFallback>
						<User className="w-6 h-6" />
					</AvatarFallback>
				</Avatar>
				<div className="flex-1 min-w-0">
					<div className="flex items-center gap-2">
						{userInfo.data.personal_info.is_charge && (
							<Badge variant="default" className="h-4 px-1.5 text-xs bg-yellow-500">
								{t.hunterPage.searchForm.chargedUser}
							</Badge>
						)}
						<Badge variant="default" className="h-4 px-1.5 text-xs bg-yellow-500">{userInfo.data.type || '-'}</Badge>
					</div>
					<p className="text-sm text-muted-foreground truncate mt-1">{userInfo.data.personal_info.phone || '-'}</p>
				</div>
			</div>
			<div className="grid grid-cols-3 gap-2 text-sm">
				<div><div className="text-muted-foreground">{t.hunterPage.searchForm.remainEquityPoints}</div><div className="font-semibold">{renderQuotaLabel(userInfo.data.rest_equity_point)}</div></div>
				<div><div className="text-muted-foreground">{t.hunterPage.searchForm.remainFreePointsToday}</div><div className="font-semibold">{renderQuotaLabel(userInfo.data.rest_free_point)}</div></div>
				<div><div className="text-muted-foreground">{t.hunterPage.searchForm.freePointLimitToday}</div><div className="font-semibold">{renderQuotaLabel(userInfo.data.day_free_point)}</div></div>
				<div><div className="text-muted-foreground">{t.hunterPage.searchForm.remainExportQuotaToday}</div><div className="font-semibold">{renderQuotaLabel(userInfo.data.rest_export_quota)}</div></div>
				<div><div className="text-muted-foreground">{t.hunterPage.searchForm.exportQuotaLimitToday}</div><div className="font-semibold">{renderQuotaLabel(userInfo.data.day_export_quota)}</div></div>
				<div><div className="text-muted-foreground">{t.hunterPage.searchForm.exportQuotaLimitPerOnce}</div><div className="font-semibold">{renderQuotaLabel(userInfo.data.once_export_quota)}</div></div>
			</div>
		</div>
	) : (<Button variant="outline" onClick={() => onFetchUserInfo(true)} disabled={isLoadingUserInfo}>{isLoadingUserInfo ? t.hunterPage.searchForm.loading : t.hunterPage.searchForm.userInfo}</Button>)

	return (
		<Card className={cn('relative min-w-0 p-3 md:p-6', showSuggestions && suggestions.length > 0 && inputFocused && 'z-30')}>
			<div className="space-y-4">
				<div className="space-y-3">
					{!isMobile && (<div className="flex flex-wrap items-center justify-between gap-3 mb-2">
						<Label className="text-sm font-medium">{t.hunterPage.searchForm.queryCondition}</Label>
						<div className="flex items-center gap-2">
							{userInfo ? (
								<HoverCard>
									<HoverCardTrigger asChild>
										<Button variant="outline" size="sm" className="h-8 px-3">
											<Avatar className="w-4 h-4 mr-2">
												<AvatarFallback className="bg-primary/10 text-xs">
													<User className="w-3 h-3" />
												</AvatarFallback>
											</Avatar>
											<span className="font-medium">{userInfo.data.personal_info.username || userInfo.data.personal_info.phone}</span>
											{availablePointText && (
												<Badge variant="secondary" className="ml-2 h-5 px-1.5 text-xs">
													{formatText(t.hunterPage.searchForm.availablePoints, { count: availablePointText })}
												</Badge>
											)}
											{userInfo.data.personal_info.is_charge && (
												<Badge variant="default" className="ml-2 h-4 px-1 text-xs bg-yellow-500">
													{t.hunterPage.searchForm.chargedUser}
												</Badge>
											)}
										</Button>
									</HoverCardTrigger>
									<HoverCardContent className="w-120 max-w-[calc(100vw-2rem)]">{accountContent}</HoverCardContent>
								</HoverCard>
							) : (
								<Button variant="outline" size="sm" onClick={() => onFetchUserInfo(true)} disabled={isLoadingUserInfo}>
									<Avatar className="w-4 h-4 mr-1">
										<AvatarFallback className="bg-primary/10 text-primary text-xs">
											<User className="w-3 h-3" />
										</AvatarFallback>
									</Avatar>
									{isLoadingUserInfo ? t.hunterPage.searchForm.loading : t.hunterPage.searchForm.userInfo}
								</Button>
							)}
						</div>
					</div>)}
					<div className="relative">
						<div
							data-slot="text-input-with-actions-shell"
							className={cn(
								'relative flex flex-col rounded-xl border border-border/60 bg-muted/50 overflow-hidden transition-all',
								inputFocused && 'border-ring ring-ring/50 ring-[3px]'
							)}
						>
							<div data-slot="text-input-with-actions-editor" className="relative overflow-hidden rounded-t-xl rounded-b-none bg-transparent">
								<Textarea
									ref={textareaRef}
									value={query}
									onChange={e => setQuery(e.target.value)}
									onCompositionStart={() => { isComposingRef.current = true }}
									onCompositionEnd={() => { isComposingRef.current = false }}
									onFocus={() => setInputFocused(true)}
									onBlur={() => {
										setTimeout(() => {
											setInputFocused(false)
											setShowSuggestions(false)
										}, 200)
									}}
									onKeyDown={(e) => {
										const nativeEvent = e.nativeEvent as KeyboardEvent
										const isComposing = isComposingRef.current || nativeEvent.isComposing || nativeEvent.keyCode === 229
										if (isComposing) return

										if (e.key === 'Enter' && !e.shiftKey) {
											if (showSuggestions && selectedSuggestionIndex >= 0) {
												e.preventDefault()
												onSelectSuggestion(suggestions[selectedSuggestionIndex].key)
											} else {
												e.preventDefault()
												onStartSearch()
											}
										} else if (e.key === 'ArrowDown' && showSuggestions) {
											e.preventDefault()
											setSelectedSuggestionIndex(prev => (prev < suggestions.length - 1 ? prev + 1 : prev))
										} else if (e.key === 'ArrowUp' && showSuggestions) {
											e.preventDefault()
											setSelectedSuggestionIndex(prev => (prev > 0 ? prev - 1 : -1))
										} else if (e.key === 'Escape' && showSuggestions) {
											e.preventDefault()
											setShowSuggestions(false)
										}
									}}
									placeholder={t.hunterPage.searchForm.placeholder}
									className="font-mono resize-none h-20 min-h-20 max-h-20 overflow-y-auto field-sizing-fixed md:field-sizing-content text-base md:text-sm md:h-auto md:max-h-none md:min-h-[110px] rounded-t-xl rounded-b-none border-0 !bg-transparent dark:!bg-transparent shadow-none focus-visible:ring-0"
								/>
							</div>
							{isMobile ? (<MobileSearchActions account={accountContent} actions={[{ label: t.hunterPage.searchForm.syntaxHelp, onSelect: onOpenTipsDialog }, { label: t.hunterPage.searchForm.favSyntax, onSelect: onOpenFavSyntaxDialog }, { label: t.hunterPage.searchForm.saveCurrent, onSelect: onOpenAddFavDialog }]} search={<Button variant="default" onClick={onStartSearch} disabled={isSearching}>
									{isSearching ? (
										<>
											<div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />
											{t.hunterPage.searchForm.searching}
										</>
									) : (
										<>
											<Search className="w-4 h-4 mr-2" />
											{t.hunterPage.searchForm.search}
										</>
									)}
								</Button>} />) : (<div data-slot="text-input-with-actions-footer" className="flex flex-wrap items-center justify-between gap-3 rounded-b-xl bg-transparent px-3 py-2">
								<div className="flex flex-wrap items-center gap-2">
									<Button variant="ghost" size="sm" onClick={onOpenTipsDialog}><BookOpen className="w-4 h-4 mr-1" />{t.hunterPage.searchForm.syntaxHelp}</Button>
									<Button variant="ghost" size="sm" onClick={onOpenFavSyntaxDialog}><List className="w-4 h-4 mr-1" />{t.hunterPage.searchForm.favSyntax}</Button>
									<Button variant="ghost" size="sm" onClick={onOpenAddFavDialog}><Star className="w-4 h-4 mr-1" />{t.hunterPage.searchForm.saveCurrent}</Button>
								</div>
								<Button variant="ghost" onClick={onStartSearch} disabled={isSearching}>
									{isSearching ? (
										<>
											<div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />
											{t.hunterPage.searchForm.searching}
										</>
									) : (
										<>
											<Search className="w-4 h-4 mr-2" />
											{t.hunterPage.searchForm.search}
										</>
									)}
								</Button>
							</div>)}
						</div>
						{showSuggestions && suggestions.length > 0 && inputFocused && (
							<div
								ref={suggestionsRef}
								className="absolute z-50 left-0 right-0 top-full mt-1 bg-popover border border-border rounded-md shadow-lg max-h-[300px] overflow-y-auto"
							>
								{suggestions.map((suggestion, index) => (
									<div
										key={`${suggestion.category}-${index}`}
										className={cn('px-3 py-2 cursor-pointer hover:bg-muted transition-colors', selectedSuggestionIndex === index && 'bg-muted')}
										onMouseEnter={() => setSelectedSuggestionIndex(index)}
										onMouseDown={(e) => {
											e.preventDefault()
											onSelectSuggestion(suggestion.key)
										}}
									>
										<div className="flex items-start gap-2">
											<div className="flex-1 min-w-0">
												<code className="text-primary font-mono text-sm break-all">{suggestion.key}</code>
												<p className="text-xs text-muted-foreground mt-1">{suggestion.description}</p>
											</div>
											<Badge variant="outline" className="text-xs shrink-0">{suggestion.category}</Badge>
										</div>
									</div>
								))}
							</div>
						)}
					</div>
				</div>
			</div>
		</Card>
	)
}
