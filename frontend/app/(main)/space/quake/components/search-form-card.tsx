'use client'

import { useIsMobile } from '@/hooks/use-mobile'
import { MobileSearchActions } from '../../components/mobile-search-actions'

import { useRef, type Dispatch, type MutableRefObject, type SetStateAction } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'
import { BookOpen, Hash, ImageIcon, Info, List, Search, Star, User } from 'lucide-react'
import type { SuggestionItem } from '../types'
import { useI18n } from '@/lib/i18n'

interface QuakeSearchFormCardProps {
	query: string
	setQuery: Dispatch<SetStateAction<string>>
	isSearching: boolean
	onStartSearch: () => void
	userInfo: any
	isLoadingUserInfo: boolean
	onFetchUserInfo: (showAlert?: boolean, silent?: boolean) => Promise<void>
	onOpenApiConfigDialog: () => void
	onOpenTipsDialog: () => void
	onOpenIconSearchDialog: () => void
	onOpenFavSyntaxDialog: () => void
	onOpenAddFavDialog: () => void
	onBatchSearch: () => void
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
	latest: boolean
	setLatest: Dispatch<SetStateAction<boolean>>
	canUseLatestFilter: boolean
	availableCredit: number | null
}

export function QuakeSearchFormCard(props: QuakeSearchFormCardProps) {
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
		onOpenIconSearchDialog,
		onOpenFavSyntaxDialog,
		onOpenAddFavDialog,
		onBatchSearch,
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
		latest,
		setLatest,
		canUseLatestFilter,
		availableCredit,
	} = props
	const formatText = (template: string, vars: Record<string, string | number>) =>
		Object.entries(vars).reduce((acc, [key, value]) => acc.replace(`{${key}}`, String(value)), template)

	const accountContent = userInfo ? (
		<div className="space-y-3">
			<div>
				<div className="text-sm font-semibold mb-1">{t.quakePage.searchForm.userInfoTitle}</div>
				<div className="space-y-1 text-xs">
					<div className="flex justify-between"><span className="text-muted-foreground">{t.quakePage.searchForm.username}</span><span className="font-medium">{userInfo.user?.username || 'N/A'}</span></div>
					{userInfo.user?.fullname && <div className="flex justify-between"><span className="text-muted-foreground">{t.quakePage.searchForm.fullName}</span><span className="font-medium">{userInfo.user.fullname}</span></div>}
					{userInfo.user?.email && <div className="flex justify-between"><span className="text-muted-foreground">{t.quakePage.searchForm.email}</span><span className="font-medium">{userInfo.user.email}</span></div>}
				</div>
			</div>
			<div className="pt-2 border-t space-y-2">
				<div className="grid grid-cols-2 gap-3">
					<div><div className="text-xs text-muted-foreground">{t.quakePage.searchForm.points}</div><div className="text-sm font-semibold">{availableCredit?.toLocaleString() || 0}</div></div>
					<div><div className="text-xs text-muted-foreground">{t.quakePage.searchForm.persistentPoints}</div><div className="text-sm font-semibold">{(userInfo.persistent_credit ?? 0).toLocaleString()}</div></div>
				</div>
				<div className="flex justify-between items-center pt-1">
					<span className="text-xs text-muted-foreground">{t.quakePage.searchForm.status}</span>
					<Badge variant={userInfo.baned ? 'destructive' : 'default'} className="text-xs">{userInfo.ban_status || (userInfo.baned ? t.quakePage.searchForm.banned : t.quakePage.searchForm.active)}</Badge>
				</div>
				{userInfo.role && userInfo.role.length > 0 && (
					<div className="pt-1 border-t">
						<div className="text-xs text-muted-foreground mb-1">{t.quakePage.searchForm.rolePermissions}</div>
						<div className="space-y-1">
							{userInfo.role.map((role: any, idx: number) => (
								<div key={idx} className="flex justify-between items-center">
									<span className="text-xs font-medium">{role.fullname}</span>
									<div className="flex items-center gap-2">
										{role.priority !== undefined && <span className="text-xs text-muted-foreground">{formatText(t.quakePage.searchForm.priority, { count: role.priority })}</span>}
										{role.credit !== undefined && <span className="text-xs text-muted-foreground">{formatText(t.quakePage.searchForm.monthlyFreePoints, { count: role.credit?.toLocaleString() || 0 })}</span>}
									</div>
								</div>
							))}
						</div>
					</div>
				)}
				{userInfo.enterprise_information && (
					<div className="pt-1 border-t">
						<div className="text-xs text-muted-foreground mb-1">{t.quakePage.searchForm.enterpriseInfo}</div>
						<div className="text-xs"><div className="flex justify-between"><span className="text-muted-foreground">{t.quakePage.searchForm.status}</span><span className="font-medium">{userInfo.enterprise_information.status || t.quakePage.searchForm.notCertified}</span></div></div>
					</div>
				)}
			</div>
		</div>
	) : (<Button variant="outline" onClick={() => onFetchUserInfo(true)} disabled={isLoadingUserInfo}>{isLoadingUserInfo ? t.quakePage.searchForm.loading : t.quakePage.searchForm.userInfo}</Button>)

	return (
		<Card className={cn('relative min-w-0 p-3 md:p-6', showSuggestions && suggestions.length > 0 && inputFocused && 'z-30')}>
			<div className="space-y-4">
				<div>
					{!isMobile && (<div className="flex flex-wrap items-center justify-between gap-3 mb-2">
						<div className="flex flex-wrap items-center gap-3">
							<div className="flex items-center gap-2">
								<Tooltip>
									<TooltipTrigger asChild>
										<Label htmlFor="latest-switch" className="cursor-pointer flex items-center gap-1.5">
											{t.quakePage.searchForm.latestData}
											<Info className="w-4 h-4 text-muted-foreground" />
										</Label>
									</TooltipTrigger>
									<TooltipContent>
										{canUseLatestFilter ? (
											<p>{t.quakePage.searchForm.latestEnabledTip}</p>
										) : (
											<p>{t.quakePage.searchForm.latestLockedTip}</p>
										)}
									</TooltipContent>
								</Tooltip>
								<Switch
									id="latest-switch"
									checked={latest}
									onCheckedChange={setLatest}
									disabled={!canUseLatestFilter}
								/>
							</div>
						</div>
						<div className="flex gap-2">
							{userInfo ? (
								<HoverCard>
									<HoverCardTrigger asChild>
										<Button variant="outline" size="sm">
											<User className="w-4 h-4 mr-1" />
											{userInfo.user?.fullname || userInfo.user?.username || t.quakePage.searchForm.userFallback} ({availableCredit?.toLocaleString() || 0})
										</Button>
									</HoverCardTrigger>
									<HoverCardContent className="w-96">{accountContent}</HoverCardContent>
								</HoverCard>
							) : (
								<Button variant="outline" size="sm" onClick={() => onFetchUserInfo(true)} disabled={isLoadingUserInfo}>
									<User className="w-4 h-4 mr-1" />
									{isLoadingUserInfo ? t.quakePage.searchForm.loading : t.quakePage.searchForm.userInfo}
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
									onFocus={() => setInputFocused(true)}
									onBlur={() => {
										setTimeout(() => {
											setInputFocused(false)
											setShowSuggestions(false)
										}, 200)
									}}
									onCompositionStart={() => {
										isComposingRef.current = true
									}}
									onCompositionEnd={() => {
										isComposingRef.current = false
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
									placeholder={t.quakePage.searchForm.placeholder}
									className="font-mono resize-none h-20 min-h-20 max-h-20 overflow-y-auto field-sizing-fixed md:field-sizing-content text-base md:text-sm md:h-auto md:max-h-none md:min-h-[110px] rounded-t-xl rounded-b-none border-0 !bg-transparent dark:!bg-transparent shadow-none focus-visible:ring-0"
								/>
							</div>
							{isMobile ? (<MobileSearchActions options={<div className="flex flex-wrap items-center gap-3">
							<div className="flex items-center gap-2">
								<Tooltip>
									<TooltipTrigger asChild>
										<Label htmlFor="latest-switch" className="cursor-pointer flex items-center gap-1.5">
											{t.quakePage.searchForm.latestData}
											<Info className="w-4 h-4 text-muted-foreground" />
										</Label>
									</TooltipTrigger>
									<TooltipContent>
										{canUseLatestFilter ? (
											<p>{t.quakePage.searchForm.latestEnabledTip}</p>
										) : (
											<p>{t.quakePage.searchForm.latestLockedTip}</p>
										)}
									</TooltipContent>
								</Tooltip>
								<Switch
									id="latest-switch"
									checked={latest}
									onCheckedChange={setLatest}
									disabled={!canUseLatestFilter}
								/>
							</div>
						</div>} count={Number(latest)} account={accountContent} actions={[{ label: t.quakePage.searchForm.syntaxHelp, onSelect: onOpenTipsDialog }, { label: t.quakePage.searchForm.favSyntax, onSelect: onOpenFavSyntaxDialog }, { label: t.quakePage.searchForm.saveCurrent, onSelect: onOpenAddFavDialog }, { label: t.quakePage.searchForm.iconSearch, onSelect: onOpenIconSearchDialog }, { label: t.quakePage.searchForm.batchSearch, onSelect: onBatchSearch }]} search={<Button variant="default" onClick={onStartSearch} disabled={isSearching} className="h-9 px-4">
									{isSearching ? (
										<>
											<div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />
											{t.quakePage.searchForm.searching}
										</>
									) : (
										<>
											<Search className="w-4 h-4 mr-2" />
											{t.quakePage.searchForm.search}
										</>
									)}
								</Button>} />) : (<div data-slot="text-input-with-actions-footer" className="flex flex-wrap items-center justify-between gap-3 rounded-b-xl bg-transparent px-3 py-2">
								<div className="flex flex-wrap items-center gap-2">
									<Button variant="ghost" size="sm" onClick={onOpenTipsDialog} className="h-8 px-3">
										<BookOpen className="w-4 h-4 mr-1" />
										{t.quakePage.searchForm.syntaxHelp}
									</Button>
									<Button variant="ghost" size="sm" onClick={onOpenIconSearchDialog} className="h-8 px-3">
										<ImageIcon className="w-4 h-4 mr-1" />
										{t.quakePage.searchForm.iconSearch}
									</Button>
									<Button variant="ghost" size="sm" onClick={onOpenFavSyntaxDialog} className="h-8 px-3">
										<List className="w-4 h-4 mr-1" />
										{t.quakePage.searchForm.favSyntax}
									</Button>
									<Button variant="ghost" size="sm" onClick={onOpenAddFavDialog} className="h-8 px-3">
										<Star className="w-4 h-4 mr-1" />
										{t.quakePage.searchForm.saveCurrent}
									</Button>
									<Button variant="ghost" size="sm" onClick={onBatchSearch} className="h-8 px-3">
										<Hash className="w-4 h-4 mr-1" />
										{t.quakePage.searchForm.batchSearch}
									</Button>
								</div>
								<Button variant="ghost" onClick={onStartSearch} disabled={isSearching} className="h-9 px-4">
									{isSearching ? (
										<>
											<div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />
											{t.quakePage.searchForm.searching}
										</>
									) : (
										<>
											<Search className="w-4 h-4 mr-2" />
											{t.quakePage.searchForm.search}
										</>
									)}
								</Button>
							</div>)}
						</div>
						{showSuggestions && suggestions.length > 0 && inputFocused && (
							<div
								ref={suggestionsRef}
								className="absolute z-50 w-full mt-1 bg-popover border border-border rounded-md shadow-lg max-h-[300px] overflow-y-auto"
							>
								{suggestions.map((match, index) => (
									<div
										key={`${match.category}-${index}`}
										className={cn('px-3 py-2 cursor-pointer hover:bg-muted transition-colors', selectedSuggestionIndex === index && 'bg-muted')}
										onMouseEnter={() => setSelectedSuggestionIndex(index)}
										onMouseDown={(e) => {
											e.preventDefault()
											onSelectSuggestion(match.key)
										}}
									>
										<div className="flex items-start gap-2">
											<div className="flex-1 min-w-0">
												<code className="text-primary font-mono text-sm break-all">{match.key}</code>
												<p className="text-xs text-muted-foreground mt-1">{match.description}</p>
											</div>
											<Badge variant="outline" className="text-xs shrink-0">{match.category}</Badge>
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
