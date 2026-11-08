'use client'

import { useIsMobile } from '@/hooks/use-mobile'
import { MobileSearchActions } from '../../components/mobile-search-actions'

import { type MutableRefObject } from 'react'
import Image from 'next/image'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { HoverCard, HoverCardContent, HoverCardTrigger } from '@/components/ui/hover-card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { BookOpen, Hash, ImageIcon, List, Search, Star, User } from 'lucide-react'
import { useI18n } from '@/lib/i18n'

export interface FofaSuggestionItem {
	key: string
	description: string
	category: string
}

export interface FofaUserInfo {
	error: boolean
	email: string
	username: string
	category: string
	fcoin: number
	fofa_point: number
	remain_free_point: number
	remain_api_query: number
	remain_api_data: number
	isvip: boolean
	vip_level: number
	is_verified: boolean
	avatar: string
	message: string
	fofacli_ver: string
	fofa_server: boolean
}

interface SearchFormCardProps {
	query: string
	setQuery: (value: string) => void
	isSearching: boolean
	onStartSearch: () => void
	userInfo: FofaUserInfo | null
	isLoadingUserInfo: boolean
	onFetchUserInfo: (showDialog?: boolean) => Promise<void>
	onOpenApiConfigDialog: () => void
	onOpenSyntaxHelpDialog: () => void
	onOpenFavSyntaxDialog: () => void
	onOpenAddFavDialog: () => void
	onIconHashSearch: () => void
	onBatchSearch: () => void
	showSuggestions: boolean
	suggestions: FofaSuggestionItem[]
	inputFocused: boolean
	setInputFocused: (value: boolean) => void
	setShowSuggestions: (value: boolean) => void
	selectedSuggestionIndex: number
	selectedSuggestionIndexRef: MutableRefObject<number>
	setSelectedSuggestionIndex: (value: number) => void
	onSelectSuggestion: (syntaxKey: string) => void
	suggestionsRef: MutableRefObject<HTMLDivElement | null>
	textareaRef: MutableRefObject<HTMLTextAreaElement | null>
	fraud: boolean
	setFraud: (value: boolean) => void
	cert: boolean
	setCert: (value: boolean) => void
}

export function SearchFormCard(props: SearchFormCardProps) {
	const { t } = useI18n()
	const isMobile = useIsMobile()
	const {
		query,
		setQuery,
		isSearching,
		onStartSearch,
		userInfo,
		isLoadingUserInfo,
		onFetchUserInfo,
		onOpenSyntaxHelpDialog,
		onOpenFavSyntaxDialog,
		onOpenAddFavDialog,
		onIconHashSearch,
		onBatchSearch,
		showSuggestions,
		suggestions,
		inputFocused,
		setInputFocused,
		setShowSuggestions,
		selectedSuggestionIndex,
		selectedSuggestionIndexRef,
		setSelectedSuggestionIndex,
		onSelectSuggestion,
		suggestionsRef,
		textareaRef,
		fraud,
		setFraud,
		cert,
		setCert,
	} = props
	const accountContent = userInfo ? (
		<div className="space-y-4">
			<div className="flex items-center gap-3">
				{userInfo.avatar && (
					<Image
						src={userInfo.avatar}
						alt={userInfo.username}
						width={48}
						height={48}
						unoptimized
						className="w-12 h-12 rounded-full border border-border"
					/>
				)}
				<div className="flex-1 min-w-0">
					<div className="flex items-center gap-2">
						<h4 className="font-semibold truncate">{userInfo.username}</h4>
						{userInfo.isvip && (
							<Badge variant="default" className="h-4 px-1.5 text-xs bg-yellow-500">
								VIP {userInfo.vip_level}
							</Badge>
						)}
					</div>
					<p className="text-sm text-muted-foreground truncate">{userInfo.email || '-'}</p>
				</div>
			</div>

			<div className="grid grid-cols-2 gap-2 text-sm">
				<div>
					<div className="text-muted-foreground">{t.fofaPage.searchForm.fCoin}</div>
					<div className="font-semibold">{(userInfo.fcoin ?? 0).toLocaleString()}</div>
				</div>
				<div>
					<div className="text-muted-foreground">{t.fofaPage.searchForm.fPoint}</div>
					<div className="font-semibold">{(userInfo.fofa_point ?? 0).toLocaleString()}</div>
				</div>
				<div>
					<div className="text-muted-foreground">{t.fofaPage.searchForm.remainFreeFPoint}</div>
					<div className="font-semibold">{(userInfo.remain_free_point ?? 0).toLocaleString()}</div>
				</div>
				<div>
					<div className="text-muted-foreground">{t.fofaPage.searchForm.apiQueryCount}</div>
					<div className="font-semibold">
						{userInfo.remain_api_query === -1 || userInfo.remain_api_query === undefined
							? t.fofaPage.searchForm.unlimited
							: (userInfo.remain_api_query ?? 0).toLocaleString()}
					</div>
				</div>
				<div>
					<div className="text-muted-foreground">{t.fofaPage.searchForm.apiReturnCount}</div>
					<div className="font-semibold">
						{userInfo.remain_api_data === -1 || userInfo.remain_api_data === undefined
							? t.fofaPage.searchForm.unlimited
							: (userInfo.remain_api_data ?? 0).toLocaleString()}
					</div>
				</div>
			</div>
		</div>
	) : (<Button variant="outline" onClick={() => onFetchUserInfo(true)} disabled={isLoadingUserInfo}>{isLoadingUserInfo ? t.fofaPage.searchForm.loading : t.fofaPage.searchForm.userInfo}</Button>)

	return (
		<Card className={cn('relative min-w-0 p-3 md:p-6', showSuggestions && suggestions.length > 0 && inputFocused && 'z-30')}>
			<div className="space-y-4">
				<div className="space-y-3">
					{!isMobile && (<div className="flex flex-wrap items-center justify-between gap-3 mb-2">
						<div className="flex flex-wrap items-center gap-3">
							<div className="flex items-center gap-3 p-2 rounded-lg border border-border bg-card hover:bg-muted/50 transition-colors cursor-pointer" onClick={() => setFraud(!fraud)}>
								<Checkbox id="fraud" checked={fraud} onCheckedChange={checked => setFraud(checked as boolean)} onClick={(e) => e.stopPropagation()} />
								<Label htmlFor="fraud" className="text-sm cursor-pointer flex-1">{t.fofaPage.searchForm.excludeNoise}</Label>
								<Badge variant="outline" className="text-xs shrink-0">{t.fofaPage.searchForm.pro}</Badge>
							</div>
							<div className="flex items-center gap-3 p-2 rounded-lg border border-border bg-card hover:bg-muted/50 transition-colors cursor-pointer" onClick={() => setCert(!cert)}>
								<Checkbox id="cert" checked={cert} onCheckedChange={checked => setCert(checked as boolean)} onClick={(e) => e.stopPropagation()} />
								<Label htmlFor="cert" className="text-sm cursor-pointer flex-1">{t.fofaPage.searchForm.certVerify}</Label>
								<Badge variant="outline" className="text-xs shrink-0">{t.fofaPage.searchForm.personal}</Badge>
							</div>
						</div>
						<div className="flex items-center gap-2">
							{userInfo ? (
								<HoverCard>
									<HoverCardTrigger asChild>
										<Button variant="outline" size="sm" className="h-8 px-3">
											{userInfo.avatar && (
												<Image
													src={userInfo.avatar}
													alt={userInfo.username}
													width={16}
													height={16}
													unoptimized
													className="w-4 h-4 rounded-full mr-2"
												/>
											)}
											<span className="font-medium">{userInfo.username}</span>
											{userInfo.isvip && (
												<Badge variant="default" className="ml-2 h-4 px-1 text-xs bg-yellow-500">
													VIP {userInfo.vip_level}
												</Badge>
											)}
										</Button>
									</HoverCardTrigger>
									<HoverCardContent className="w-80">{accountContent}</HoverCardContent>
								</HoverCard>
							) : (
								<Button variant="outline" size="sm" onClick={() => onFetchUserInfo(true)} disabled={isLoadingUserInfo}>
									<User className="w-4 h-4 mr-1" />
									{isLoadingUserInfo ? t.fofaPage.searchForm.loading : t.fofaPage.searchForm.userInfo}
								</Button>
							)}
						</div>
					</div>)}
					<div className="relative">
						<div
							data-slot="text-input-with-actions-shell"
							className={cn(
								'relative flex flex-col overflow-hidden rounded-xl border border-border/60 bg-muted/50 transition-all',
								inputFocused && 'border-ring ring-ring/50 ring-[3px]'
							)}
						>
							<div data-slot="text-input-with-actions-editor" className="relative h-20 md:h-[110px] overflow-hidden rounded-t-xl rounded-b-none bg-transparent">
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
									onKeyDown={(e) => {
										if (e.nativeEvent.isComposing || e.nativeEvent.keyCode === 229) return
										if (e.key === 'Enter' && !e.shiftKey) {
											if (showSuggestions && selectedSuggestionIndexRef.current >= 0 && suggestions[selectedSuggestionIndexRef.current]) {
												e.preventDefault()
												onSelectSuggestion(suggestions[selectedSuggestionIndexRef.current].key)
											} else {
												e.preventDefault()
												onStartSearch()
											}
										} else if (e.key === 'ArrowDown' && showSuggestions) {
											e.preventDefault()
											const newIndex = selectedSuggestionIndexRef.current < suggestions.length - 1 ? selectedSuggestionIndexRef.current + 1 : selectedSuggestionIndexRef.current
											selectedSuggestionIndexRef.current = newIndex
											setSelectedSuggestionIndex(newIndex)
										} else if (e.key === 'ArrowUp' && showSuggestions) {
											e.preventDefault()
											const newIndex = selectedSuggestionIndexRef.current > 0 ? selectedSuggestionIndexRef.current - 1 : -1
											selectedSuggestionIndexRef.current = newIndex
											setSelectedSuggestionIndex(newIndex)
										} else if (e.key === 'Escape' && showSuggestions) {
											e.preventDefault()
											setShowSuggestions(false)
											selectedSuggestionIndexRef.current = -1
										}
									}}
									placeholder={t.fofaPage.searchForm.placeholder}
									className="h-20 max-h-20 min-h-20 md:h-[110px] md:max-h-[110px] md:min-h-[110px] text-base md:text-sm resize-none overflow-y-auto rounded-t-xl rounded-b-none border-0 !bg-transparent font-mono shadow-none field-sizing-fixed focus-visible:ring-0"
								/>
							</div>
							{isMobile ? (<MobileSearchActions options={<div className="flex flex-wrap items-center gap-3">
							<div className="flex items-center gap-3 p-2 rounded-lg border border-border bg-card hover:bg-muted/50 transition-colors cursor-pointer" onClick={() => setFraud(!fraud)}>
								<Checkbox id="fraud" checked={fraud} onCheckedChange={checked => setFraud(checked as boolean)} onClick={(e) => e.stopPropagation()} />
								<Label htmlFor="fraud" className="text-sm cursor-pointer flex-1">{t.fofaPage.searchForm.excludeNoise}</Label>
								<Badge variant="outline" className="text-xs shrink-0">{t.fofaPage.searchForm.pro}</Badge>
							</div>
							<div className="flex items-center gap-3 p-2 rounded-lg border border-border bg-card hover:bg-muted/50 transition-colors cursor-pointer" onClick={() => setCert(!cert)}>
								<Checkbox id="cert" checked={cert} onCheckedChange={checked => setCert(checked as boolean)} onClick={(e) => e.stopPropagation()} />
								<Label htmlFor="cert" className="text-sm cursor-pointer flex-1">{t.fofaPage.searchForm.certVerify}</Label>
								<Badge variant="outline" className="text-xs shrink-0">{t.fofaPage.searchForm.personal}</Badge>
							</div>
						</div>} count={Number(fraud) + Number(cert)} account={accountContent} actions={[{ label: t.fofaPage.searchForm.syntaxHelp, onSelect: onOpenSyntaxHelpDialog }, { label: t.fofaPage.searchForm.favSyntax, onSelect: onOpenFavSyntaxDialog }, { label: t.fofaPage.searchForm.saveCurrent, onSelect: onOpenAddFavDialog }, { label: t.fofaPage.searchForm.iconSearch, onSelect: onIconHashSearch }, { label: t.fofaPage.searchForm.batchSearch, onSelect: onBatchSearch }]} search={<Button variant="default" size="sm" onClick={onStartSearch} disabled={isSearching}>
										{isSearching ? (<><div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />{t.fofaPage.searchForm.searching}</>) : (<><Search className="w-4 h-4 mr-2" />{t.fofaPage.searchForm.search}</>)}
									</Button>} />) : (<div data-slot="text-input-with-actions-footer" className="min-h-12 rounded-b-xl bg-transparent px-3 py-2">
								<div className="flex w-full flex-wrap items-center justify-between gap-3">
									<div className="flex flex-wrap items-center gap-2">
										<Button variant="ghost" size="sm" onClick={onOpenSyntaxHelpDialog}><BookOpen className="w-4 h-4 mr-1" />{t.fofaPage.searchForm.syntaxHelp}</Button>
										<Button variant="ghost" size="sm" onClick={onOpenFavSyntaxDialog}><List className="w-4 h-4 mr-1" />{t.fofaPage.searchForm.favSyntax}</Button>
										<Button variant="ghost" size="sm" onClick={onOpenAddFavDialog}><Star className="w-4 h-4 mr-1" />{t.fofaPage.searchForm.saveCurrent}</Button>
										<Button variant="ghost" size="sm" onClick={onIconHashSearch}><ImageIcon className="w-4 h-4 mr-1" />{t.fofaPage.searchForm.iconSearch}</Button>
										<Button variant="ghost" size="sm" onClick={onBatchSearch}><Hash className="w-4 h-4 mr-1" />{t.fofaPage.searchForm.batchSearch}</Button>
									</div>
									<Button variant="ghost" size="sm" onClick={onStartSearch} disabled={isSearching}>
										{isSearching ? (<><div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin mr-2" />{t.fofaPage.searchForm.searching}</>) : (<><Search className="w-4 h-4 mr-2" />{t.fofaPage.searchForm.search}</>)}
									</Button>
								</div>
							</div>)}
						</div>
						{showSuggestions && suggestions.length > 0 && inputFocused && (
							<div ref={suggestionsRef} className="absolute left-0 right-0 top-full z-50 mt-2 bg-popover border border-border rounded-md shadow-lg max-h-[300px] overflow-y-auto">
								{suggestions.map((match, index) => (
									<div
										key={`${match.category}-${index}`}
										className={cn("h-14 cursor-pointer overflow-hidden px-3 py-2 transition-colors hover:bg-muted", selectedSuggestionIndex === index && "bg-muted")}
										onMouseEnter={() => {
											selectedSuggestionIndexRef.current = index
											setSelectedSuggestionIndex(index)
										}}
										onMouseDown={(e) => {
											e.preventDefault()
											onSelectSuggestion(match.key)
										}}
									>
										<div className="flex min-w-0 items-start gap-2">
											<div className="flex-1 min-w-0">
												<code className="block truncate whitespace-nowrap text-primary font-mono text-sm" title={match.key}>{match.key}</code>
												<p className="text-xs text-muted-foreground mt-1 truncate" title={match.description}>{match.description}</p>
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
