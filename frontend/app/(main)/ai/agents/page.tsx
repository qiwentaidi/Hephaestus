'use client'

import dynamic from 'next/dynamic'
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import toast from 'react-hot-toast'
import {
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  Columns2,
  Copy,
  Crosshair,
  FileCode2,
  GitBranch,
  History,
  ListChecks,
  Loader2,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  SearchX,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Target,
  Terminal,
  WrapText,
  X,
  type LucideIcon,
} from 'lucide-react'

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Kbd } from '@/components/ui/kbd'
import { Separator } from '@/components/ui/separator'
import AgentToolVisibilitySheet from './components/AgentToolVisibilitySheet'
import { diffPromptLines, measurePrompt, MAX_AGENT_PROMPT_BYTES } from '@/lib/agent-prompt'
import { listAgentPrompts, saveAgentPrompt, type AgentPrompt, type AgentPromptRole } from '@/lib/api/agent-prompts'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

const PromptEditor = dynamic(() => import('./components/PromptEditor'), {
  ssr: false,
  loading: () => <div className="h-full w-full animate-pulse rounded-lg bg-muted/30" />,
})

type AgentPromptCopy = ReturnType<typeof useI18n>['t']['agentPrompts']
type AgentPromptGroup = 'orchestration' | 'dialogue'
type AgentRoleFilter = 'all' | AgentPromptGroup

const ROLE_ORDER: AgentPromptRole[] = ['planner', 'worker', 'controller', 'pentest']
const GROUP_ORDER: AgentPromptGroup[] = ['orchestration', 'dialogue']
const ROLE_GROUP: Record<AgentPromptRole, AgentPromptGroup> = {
  planner: 'orchestration',
  worker: 'orchestration',
  controller: 'orchestration',
  pentest: 'dialogue',
}
const ROLE_ICONS: Record<AgentPromptRole, LucideIcon> = {
  planner: GitBranch,
  worker: Terminal,
  controller: SlidersHorizontal,
  pentest: Crosshair,
}

function formatStamp(value: string, language: 'zh' | 'en') {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString(language === 'zh' ? 'zh-CN' : 'en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  })
}

export default function AgentManagementPage() {
  const { t, language } = useI18n()
  const copy = t.agentPrompts
  const [items, setItems] = useState<AgentPrompt[]>([])
  const [selected, setSelected] = useState<AgentPromptRole>('planner')
  const [drafts, setDrafts] = useState<Partial<Record<AgentPromptRole, string>>>({})
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<AgentRoleFilter>('all')
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [compare, setCompare] = useState(false)
  const [wrap, setWrap] = useState(true)
  const [resetOpen, setResetOpen] = useState(false)
  const [modKey, setModKey] = useState('⌘')
  const [toolsOpen, setToolsOpen] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)

  const load = useCallback(async () => {
    setLoadFailed(false)
    try {
      const value = await listAgentPrompts()
      setItems(value)
      setDrafts({})
      setSaveError('')
    } catch {
      setLoadFailed(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    if (!/Mac|iPhone|iPad|iPod/.test(navigator.userAgent)) setModKey('Ctrl')
  }, [])

  const orderedItems = useMemo(
    () => [...items].sort((a, b) => ROLE_ORDER.indexOf(a.role) - ROLE_ORDER.indexOf(b.role)),
    [items],
  )

  const visibleItems = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    return orderedItems.filter((entry) => {
      if (filter !== 'all' && ROLE_GROUP[entry.role] !== filter) return false
      if (!keyword) return true
      const roleCopy = copy.roles[entry.role]
      const text = `${roleCopy.name} ${roleCopy.description} ${drafts[entry.role] ?? entry.prompt}`
      return text.toLowerCase().includes(keyword)
    })
  }, [orderedItems, filter, query, drafts, copy.roles])

  const activeRole = useMemo(() => {
    if (visibleItems.length > 0 && !visibleItems.some((entry) => entry.role === selected)) {
      return visibleItems[0].role
    }
    return selected
  }, [selected, visibleItems])

  const item = useMemo(
    () => items.find((entry) => entry.role === activeRole) ?? items.find((entry) => entry.role === selected) ?? items[0],
    [activeRole, items, selected],
  )

  const roleCopy = item ? copy.roles[item.role] : null
  const prompt = item ? (drafts[item.role] ?? item.prompt) : ''
  const selectedDirty = !!item && prompt !== item.prompt
  const dirty = items.some((entry) => drafts[entry.role] !== undefined && drafts[entry.role] !== entry.prompt)
  const metrics = useMemo(() => measurePrompt(prompt), [prompt])
  const overLimit = metrics.bytes > MAX_AGENT_PROMPT_BYTES
  const invalidPrompt = overLimit || !prompt.trim()
  const byteUsage = Math.min(100, Math.round((metrics.bytes / MAX_AGENT_PROMPT_BYTES) * 100))
  const diff = useMemo(
    () => (item ? diffPromptLines(item.defaultPrompt, prompt) : { added: 0, removed: 0 }),
    [item, prompt],
  )
  const diffSame = diff.added === 0 && diff.removed === 0
  const filterCounts: Record<AgentRoleFilter, number> = {
    all: items.length,
    orchestration: items.filter((entry) => ROLE_GROUP[entry.role] === 'orchestration').length,
    dialogue: items.filter((entry) => ROLE_GROUP[entry.role] === 'dialogue').length,
  }

  const reload = useCallback(() => {
    if (dirty && !window.confirm(copy.reloadConfirm)) return
    setLoading(true)
    void load()
  }, [dirty, copy.reloadConfirm, load])

  const save = useCallback(async (reset = false) => {
    if (!item || saving) return
    if (!reset && (!prompt.trim() || metrics.bytes > MAX_AGENT_PROMPT_BYTES)) {
      toast.error(copy.invalid)
      return
    }
    setSaving(true)
    setSaveError('')
    try {
      const updated = await saveAgentPrompt(item, prompt, reset)
      setItems((current) => current.map((entry) => (entry.role === updated.role ? updated : entry)))
      setDrafts((current) => {
        const next = { ...current }
        delete next[updated.role]
        return next
      })
      setResetOpen(false)
      setCompare(false)
      toast.success(reset ? copy.resetSuccess : copy.saved)
    } catch (cause) {
      const code = cause instanceof Error ? cause.message : ''
      const message = code === 'version_conflict' ? copy.conflict : code === 'invalid_prompt' ? copy.invalid : copy.saveFailed
      setSaveError(message)
      toast.error(message)
    } finally {
      setSaving(false)
    }
  }, [item, prompt, metrics.bytes, saving, copy])

  useEffect(() => {
    if (!dirty) return
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    const beforeNavigate = (event: Event) => {
      if (saving || !window.confirm(copy.leaveConfirm)) event.preventDefault()
    }
    window.addEventListener('beforeunload', beforeUnload)
    window.addEventListener('hephaestus:before-layout-navigation', beforeNavigate)
    return () => {
      window.removeEventListener('beforeunload', beforeUnload)
      window.removeEventListener('hephaestus:before-layout-navigation', beforeNavigate)
    }
  }, [dirty, saving, copy.leaveConfirm])

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (!event.metaKey && !event.ctrlKey) return
      const key = event.key.toLowerCase()
      if (key === 'k') {
        event.preventDefault()
        searchRef.current?.focus()
        searchRef.current?.select()
      }
      if (key === 's') {
        event.preventDefault()
        if (item && selectedDirty && !saving && !invalidPrompt) void save()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [item, selectedDirty, saving, invalidPrompt, save])

  const selectRole = (role: AgentPromptRole) => {
    setSelected(role)
    setCompare(false)
  }

  const handlePromptChange = useCallback((next: string) => {
    if (!item) return
    setDrafts((current) => ({ ...current, [item.role]: next }))
  }, [item])

  const handleCopy = async () => {
    if (!prompt) return
    const copied = await copyToClipboard(prompt)
    if (copied) toast.success(copy.copied)
    else toast.error(copy.copyFailed)
  }


  return (
    <div className="flex min-h-full w-full flex-col gap-3 p-4 md:px-6 md:pb-6 md:pt-5 2xl:h-full 2xl:min-h-0 2xl:overflow-hidden">
      <header className="flex shrink-0 flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h1 className="text-lg font-semibold tracking-tight">{copy.title}</h1>
          {dirty && (
            <Badge variant="outline" className="border-border bg-muted text-foreground">
              {copy.unsaved}
            </Badge>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1.5 px-2.5 text-[11px]"
            onClick={() => setToolsOpen(true)}
          >
            <ListChecks className="h-3.5 w-3.5" />
            {copy.toolVisibility}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1.5 px-2.5 text-[11px]"
            onClick={reload}
            disabled={loading || saving}
          >
            <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
            {copy.reload}
          </Button>
        </div>
      </header>

      <AgentToolVisibilitySheet open={toolsOpen} onOpenChange={setToolsOpen} initialRole={activeRole} />

      {saveError && (
        <div
          role="alert"
          className="flex shrink-0 flex-wrap items-center justify-between gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-xs text-destructive"
        >
          <span className="flex min-w-0 items-center gap-2">
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0">{saveError}</span>
          </span>
          <Button
            variant="outline"
            size="sm"
            className="h-6 border-destructive/30 bg-transparent px-2 text-[11px] text-destructive hover:bg-destructive/10"
            onClick={reload}
          >
            {copy.reload}
          </Button>
        </div>
      )}

      {loading ? (
        <WorkbenchSkeleton />
      ) : loadFailed || !item || !roleCopy ? (
        <div className="flex min-h-[320px] flex-1 items-center justify-center rounded-xl border border-dashed border-border bg-card/40">
          <div className="flex max-w-sm flex-col items-center gap-3 px-6 text-center">
            <span className="grid h-10 w-10 place-items-center rounded-full bg-muted text-muted-foreground">
              <AlertTriangle className="h-4 w-4" />
            </span>
            <p className="text-sm font-medium">{copy.loadFailed}</p>
            <Button size="sm" onClick={reload}>{copy.retry}</Button>
          </div>
        </div>
      ) : (
        <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 xl:grid-cols-[248px_minmax(0,1fr)] 2xl:grid-cols-[280px_minmax(0,1fr)_304px]">
          <aside className="flex min-h-0 flex-col rounded-xl border border-border/80 bg-card shadow-sm">
            <div className="shrink-0 p-3 pb-0">
              <div className="flex h-6 items-center justify-between">
                <span className="flex items-center gap-1.5 text-xs font-medium">
                  {copy.rolesLabel}
                  <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] font-normal text-muted-foreground">
                    {items.length}
                  </span>
                </span>
              </div>

              <div className="relative mt-2.5">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground/70" />
                <Input
                  ref={searchRef}
                  value={query}
                  onChange={(event) => {
                    setQuery(event.target.value)
                    setCompare(false)
                  }}
                  placeholder={copy.searchPlaceholder}
                  aria-label={copy.searchPlaceholder}
                  className="h-8 rounded-lg pl-8 pr-12 text-xs"
                />
                {query ? (
                  <button
                    type="button"
                    aria-label={copy.clearSearch}
                    onClick={() => setQuery('')}
                    className="absolute right-1.5 top-1/2 grid h-5 w-5 -translate-y-1/2 place-items-center rounded text-muted-foreground/70 hover:bg-muted hover:text-foreground"
                  >
                    <X className="h-3 w-3" />
                  </button>
                ) : (
                  <Kbd className="absolute right-1.5 top-1/2 h-4 min-w-0 -translate-y-1/2 px-1 text-[9px]">{modKey}K</Kbd>
                )}
              </div>

              <div
                role="group"
                aria-label={copy.rolesLabel}
                className="mt-2 grid grid-cols-3 gap-0.5 rounded-lg border border-border/70 bg-muted/30 p-0.5"
              >
                {(['all', 'orchestration', 'dialogue'] as AgentRoleFilter[]).map((value) => (
                  <button
                    key={value}
                    type="button"
                    onClick={() => {
                      setFilter(value)
                      setCompare(false)
                    }}
                    aria-pressed={filter === value}
                    className={cn(
                      'flex h-6 items-center justify-center gap-1 rounded-md text-[10.5px] transition-colors',
                      filter === value
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground',
                    )}
                  >
                    {copy.filters[value]}
                    <span className={cn('text-[9px]', filter === value ? 'text-muted-foreground' : 'text-muted-foreground/60')}>
                      {filterCounts[value]}
                    </span>
                  </button>
                ))}
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-2 pt-1">
              {visibleItems.length === 0 ? (
                <div className="flex flex-col items-center justify-center px-4 py-10 text-center">
                  <SearchX className="h-5 w-5 text-muted-foreground/50" />
                  <p className="mt-2 text-xs font-medium">{copy.searchEmpty}</p>
                  <p className="mt-1 text-[10.5px] leading-5 text-muted-foreground">{copy.searchEmptyHint}</p>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="mt-2 h-7 text-[11px]"
                    onClick={() => {
                      setQuery('')
                      setFilter('all')
                    }}
                  >
                    {copy.clearSearch}
                  </Button>
                </div>
              ) : (
                GROUP_ORDER.map((group) => {
                  const groupItems = visibleItems.filter((entry) => ROLE_GROUP[entry.role] === group)
                  if (groupItems.length === 0) return null
                  return (
                    <div key={group} className="mt-2 first:mt-0">
                      <div className="flex items-center justify-between px-1.5 py-1 text-[10px] tracking-wide text-muted-foreground/70">
                        <span>{copy.groups[group]}</span>
                        <span className="font-mono text-[9px]">{groupItems.length}</span>
                      </div>
                      <div className="space-y-0.5">
                        {groupItems.map((entry) => (
                          <RoleCard
                            key={entry.role}
                            item={entry}
                            copy={copy}
                            active={entry.role === activeRole}
                            draft={drafts[entry.role] !== undefined && drafts[entry.role] !== entry.prompt}
                            disabled={saving}
                            onSelect={() => selectRole(entry.role)}
                          />
                        ))}
                      </div>
                    </div>
                  )
                })
              )}
            </div>

            <div className="shrink-0 border-t border-border/70 p-3">
              <div className="rounded-lg border border-border/70 bg-muted/20 p-2.5">
                <p className="text-[10px] leading-4 text-muted-foreground">{copy.snapshotHint}</p>
                <div className="mt-2.5 flex items-center justify-between text-[10.5px]">
                  <span className="flex items-center gap-1.5 text-muted-foreground">
                    <ShieldCheck className="h-3 w-3" />
                    {copy.byteLimit}
                  </span>
                  <span className={cn('font-mono text-[10px]', byteUsage >= 80 ? 'text-foreground' : 'text-muted-foreground')}>
                    {byteUsage}%
                  </span>
                </div>
                <div className="mt-1.5 h-1 overflow-hidden rounded-full bg-border/60">
                  <div
                    className={cn('h-full rounded-full transition-all', byteUsage >= 80 ? 'bg-foreground/50' : 'bg-muted-foreground/30')}
                    style={{ width: `${Math.max(byteUsage, metrics.bytes > 0 ? 1 : 0)}%` }}
                  />
                </div>
                <div className="mt-2.5 hidden space-y-1 border-t border-border/60 pt-2 2xl:block">
                  <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                    <span>{copy.shortcutSearch}</span>
                    <Kbd className="h-4 min-w-0 px-1 text-[9px]">{modKey}K</Kbd>
                  </div>
                  <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                    <span>{copy.shortcutSave}</span>
                    <Kbd className="h-4 min-w-0 px-1 text-[9px]">{modKey}S</Kbd>
                  </div>
                </div>
              </div>
            </div>
          </aside>

          <section className="flex h-[70vh] min-h-[520px] min-w-0 flex-col overflow-hidden rounded-xl border border-border/80 bg-card shadow-sm xl:h-auto xl:min-h-[520px] 2xl:h-auto 2xl:min-h-0">
            <div className="flex shrink-0 items-start gap-3 border-b border-border/70 px-4 pb-3.5 pt-4 md:px-5">
              <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl border border-border bg-muted/50 text-foreground">
                <RoleIcon role={item.role} className="h-5 w-5" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-1.5">
                  <h2 className="text-base font-semibold tracking-tight">{roleCopy.name}</h2>
                  <Badge variant="outline" className="h-5 px-1.5 text-[9.5px] font-normal">{copy.builtin}</Badge>
                  <Badge variant="outline" className="h-5 px-1.5 text-[9.5px] font-normal">
                    {copy.roleKinds[ROLE_GROUP[item.role]]}
                  </Badge>
                  <Badge
                    variant="outline"
                    className={cn(
                      'h-5 px-1.5 text-[9.5px] font-normal',
                      (item.customized || selectedDirty) && 'border-border bg-muted text-foreground',
                    )}
                  >
                    {selectedDirty ? copy.unsavedShort : item.customized ? copy.customized : copy.default}
                  </Badge>
                </div>
                <p className="mt-1.5 text-[12px] leading-5 text-muted-foreground">{roleCopy.description}</p>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <span className="hidden items-center gap-1.5 rounded-full border border-border/70 bg-muted/25 px-2 py-1 text-[10px] text-muted-foreground lg:inline-flex">
                  <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
                  {copy.active}
                </span>
                <DropdownMenu>
                  <DropdownMenuTrigger asChild>
                    <Button variant="ghost" size="icon" className="h-7 w-7" aria-label={copy.moreActions}>
                      <MoreHorizontal className="h-4 w-4" />
                    </Button>
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="w-44">
                    <DropdownMenuItem onClick={() => void handleCopy()}>
                      <Copy className="mr-2 h-3.5 w-3.5" />
                      {copy.copyPrompt}
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => setCompare((value) => !value)}>
                      <Columns2 className="mr-2 h-3.5 w-3.5" />
                      {compare ? copy.backToEdit : copy.comparePrompt}
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            </div>

            <div className="flex h-11 shrink-0 items-center gap-1 border-b border-border/70 px-3">
              <span className="text-xs font-medium">{copy.prompt}</span>
              <Badge variant="outline" className="ml-1 h-5 px-1.5 text-[9.5px] font-normal text-muted-foreground">
                Markdown
              </Badge>
              <Separator orientation="vertical" className="mx-2 h-4" />
              <Button
                variant="ghost"
                size="sm"
                className={cn('h-7 gap-1.5 px-2 text-[11px]', wrap && 'bg-muted/50')}
                aria-pressed={wrap}
                onClick={() => setWrap((value) => !value)}
              >
                <WrapText className="h-3.5 w-3.5" />
                <span className="hidden md:inline">{copy.wrap}</span>
              </Button>
              <div className="ml-auto flex items-center gap-2">
                <Button
                  variant={compare ? 'secondary' : 'outline'}
                  size="sm"
                  className="h-7 gap-1.5 px-2 text-[11px]"
                  onClick={() => setCompare((value) => !value)}
                >
                  {compare ? <Pencil className="h-3.5 w-3.5" /> : <Columns2 className="h-3.5 w-3.5" />}
                  <span className="hidden sm:inline">{compare ? copy.backToEdit : copy.comparePrompt}</span>
                </Button>
              </div>
            </div>

            {compare && (
              <div className="flex h-8 shrink-0 items-center gap-2 border-b border-border/70 bg-muted/25 px-3 text-[10.5px] text-muted-foreground">
                <span className="font-medium text-foreground/80">{copy.defaultTitle}</span>
                <ArrowRight className="h-3 w-3" />
                <span>
                  {copy.currentVersion} v{item.version}
                  {selectedDirty ? ` · ${copy.unsavedShort}` : ''}
                </span>
                <span className="ml-auto hidden truncate lg:inline">{copy.compareHint}</span>
              </div>
            )}

            <div className="min-h-0 flex-1 p-3">
              <div className="h-full overflow-hidden rounded-lg border border-border/60">
                <PromptEditor
                  role={item.role}
                  value={prompt}
                  defaultValue={item.defaultPrompt}
                  compare={compare}
                  wrap={wrap}
                  readOnly={saving}
                  onChange={handlePromptChange}
                />
              </div>
            </div>

            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border/70 px-4 py-2.5">
              <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-[11px]">
                {invalidPrompt ? (
                  <span className="flex items-center gap-1.5 text-destructive">
                    <AlertTriangle className="h-3.5 w-3.5" />
                    {copy.invalid}
                  </span>
                ) : selectedDirty ? (
                  <span className="flex items-center gap-1.5 text-foreground">
                    <span className="h-1.5 w-1.5 rounded-full bg-foreground/60" />
                    {copy.unsavedShort}
                  </span>
                ) : (
                  <span className="flex items-center gap-1.5 text-muted-foreground">
                    <CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />
                    {copy.savedState}
                  </span>
                )}
                <span className="h-2.5 w-px bg-border" />
                <span className="text-muted-foreground">
                  {metrics.chars.toLocaleString()} {copy.chars}
                </span>
                <span className="h-2.5 w-px bg-border" />
                <span className="text-muted-foreground">
                  {copy.approx}{metrics.tokens.toLocaleString()} {copy.tokens}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 gap-1.5 px-2.5 text-[11px]"
                  onClick={() => setResetOpen(true)}
                  disabled={saving || (!item.customized && !selectedDirty)}
                >
                  <RotateCcw className="h-3.5 w-3.5" />
                  {copy.reset}
                </Button>
                <Button
                  size="sm"
                  className="h-7 gap-1.5 px-3 text-[11px] disabled:border-transparent disabled:bg-muted disabled:text-muted-foreground disabled:opacity-100"
                  onClick={() => void save()}
                  disabled={saving || !selectedDirty || invalidPrompt}
                >
                  {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                  {saving ? copy.saving : copy.save}
                </Button>
              </div>
            </div>
          </section>

          <aside className="grid min-h-0 gap-3 sm:grid-cols-2 xl:col-span-2 xl:grid-cols-2 2xl:col-span-1 2xl:flex 2xl:flex-col 2xl:overflow-y-auto">
            <SidePanel
              title={copy.versionState}
              action={<span className="text-[10px] text-muted-foreground">{selectedDirty ? copy.unsavedShort : item.customized ? copy.customized : copy.default}</span>}
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="rounded-md border border-border/70 bg-muted/30 px-1.5 py-0.5 font-mono text-[11px]">v{item.version}</span>
                <span className="text-[10.5px] text-muted-foreground">
                  {copy.lastSaved} {formatStamp(item.updatedAt, language)}
                </span>
              </div>
              <div className="mt-3 rounded-lg border border-border/60 bg-muted/15 px-2.5 py-2">
                <div className="flex items-center justify-between text-[10px] text-muted-foreground">
                  <span>{copy.diffVsDefault}</span>
                  <span className="font-mono">{diffSame ? '0' : `+${diff.added} / −${diff.removed}`}</span>
                </div>
                <div className="mt-1.5 flex items-center gap-2">
                  {diffSame ? (
                    <span className="text-[10.5px] text-muted-foreground">{copy.diffSame}</span>
                  ) : (
                    <>
                      <span className="font-mono text-[11px] text-emerald-600 dark:text-emerald-400">+{diff.added}</span>
                      <span className="font-mono text-[11px] text-rose-600/80 dark:text-rose-400/80">−{diff.removed}</span>
                      <span className="text-[10px] text-muted-foreground">{copy.diffUnit}</span>
                    </>
                  )}
                </div>
              </div>
              <Button
                variant={compare ? 'secondary' : 'outline'}
                size="sm"
                className="mt-3 w-full gap-1.5 text-[11px]"
                onClick={() => setCompare((value) => !value)}
              >
                {compare ? <Pencil className="h-3.5 w-3.5" /> : <Columns2 className="h-3.5 w-3.5" />}
                {compare ? copy.backToEdit : copy.comparePrompt}
              </Button>
            </SidePanel>

            <SidePanel title={copy.scopeTitle}>
              <div className="space-y-2">
                <ScopeRow icon={Sparkles} label={copy.scopeNew} value={copy.scopeNewValue} />
                <ScopeRow icon={History} label={copy.scopeRunning} value={copy.scopeRunningValue} />
                <ScopeRow icon={GitBranch} label={copy.scopeVersion} value={copy.scopeVersionValue} />
              </div>
            </SidePanel>

            <SidePanel title={copy.delegationTitle}>
              {ROLE_GROUP[item.role] === 'dialogue' ? (
                <div className="rounded-lg border border-border/60 bg-muted/15 p-2.5">
                  <div className="flex items-center gap-2">
                    <span className="grid h-6 w-6 place-items-center rounded-md border border-border bg-muted text-foreground">
                      <Crosshair className="h-3 w-3" />
                    </span>
                    <span className="text-[11px]">{copy.roles.pentest.name}</span>
                    <span className="ml-auto text-[9.5px] text-muted-foreground">{copy.delegationIndependent}</span>
                  </div>
                  <p className="mt-2 text-[10px] leading-5 text-muted-foreground">{copy.delegationNoWorker}</p>
                </div>
              ) : (
                <div>
                  <DelegationNode
                    role="controller"
                    current={item.role === 'controller'}
                    label={copy.roles.controller.name}
                    sub={copy.delegationMonitor}
                    currentLabel={copy.delegationCurrent}
                  />
                  <div className="ml-3 border-l border-border/70 py-1 pl-4 text-[9px] text-muted-foreground/70">
                    {copy.delegationQuery}
                  </div>
                  <DelegationNode
                    role="planner"
                    current={item.role === 'planner'}
                    label={copy.roles.planner.name}
                    sub={copy.delegationPlan}
                    currentLabel={copy.delegationCurrent}
                  />
                  <div className="ml-3 border-l border-border/70 py-1 pl-4 font-mono text-[9px] text-muted-foreground/70">
                    delegate_tasks
                  </div>
                  <DelegationNode
                    role="worker"
                    current={item.role === 'worker'}
                    label={copy.roles.worker.name}
                    sub={copy.delegationExecute}
                    currentLabel={copy.delegationCurrent}
                  />
                </div>
              )}
            </SidePanel>

            <SidePanel title={copy.runtimeTitle}>
              <div className="space-y-2.5">
                <RuntimeRow icon={Target} text={copy.runtimeGoal} />
                <RuntimeRow icon={ShieldCheck} text={copy.runtimeTools} />
                <RuntimeRow icon={FileCode2} text={copy.runtimeLimit} />
              </div>
            </SidePanel>
          </aside>
        </div>
      )}

      <AlertDialog open={resetOpen} onOpenChange={(open) => { if (!saving) setResetOpen(open) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{copy.resetTitle}</AlertDialogTitle>
            <AlertDialogDescription>{copy.resetDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saving}>{copy.cancel}</AlertDialogCancel>
            <AlertDialogAction disabled={saving} onClick={(event) => { event.preventDefault(); void save(true) }}>
              {saving ? copy.saving : copy.confirmReset}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function RoleCard({
  item,
  copy,
  active,
  draft,
  disabled,
  onSelect,
}: {
  item: AgentPrompt
  copy: AgentPromptCopy
  active: boolean
  draft: boolean
  disabled: boolean
  onSelect: () => void
}) {
  const roleCopy = copy.roles[item.role]
  const status = draft ? copy.unsavedShort : item.customized ? copy.customized : copy.default
  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={disabled}
      aria-pressed={active}
      className={cn(
        'group relative flex w-full items-start gap-2.5 rounded-lg border px-2.5 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-60',
        active ? 'border-border bg-muted shadow-sm' : 'border-transparent hover:bg-muted/40',
      )}
    >
      <span
        className={cn(
          'mt-0.5 grid h-7 w-7 shrink-0 place-items-center rounded-md border',
          active
            ? 'border-border bg-card text-foreground'
            : 'border-border/70 bg-background/60 text-muted-foreground',
        )}
      >
        <RoleIcon role={item.role} className="h-3.5 w-3.5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          <span className="truncate text-[12.5px] font-medium">{roleCopy.name}</span>
          <span className="shrink-0 rounded border border-border/70 px-1 py-px text-[9px] text-muted-foreground">
            {copy.builtin}
          </span>
        </span>
        <span className="mt-0.5 line-clamp-2 block text-[10.5px] leading-4 text-muted-foreground">{roleCopy.description}</span>
        <span className="mt-1.5 flex items-center gap-1.5 text-[10px] text-muted-foreground/80">
          <span
            className={cn(
              'h-1 w-1 rounded-full',
              draft || item.customized ? 'bg-foreground/50' : 'bg-muted-foreground/40',
            )}
          />
          {status}
          <span className="text-border">·</span>
          <span className="font-mono">v{item.version}</span>
        </span>
      </span>
    </button>
  )
}

function RoleIcon({ role, className }: { role: AgentPromptRole; className?: string }) {
  const Icon = ROLE_ICONS[role]
  return <Icon className={className} />
}

function SidePanel({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="rounded-xl border border-border/80 bg-card p-3.5 shadow-sm">
      <div className="flex h-5 items-center justify-between gap-2">
        <h3 className="text-[11px] font-medium">{title}</h3>
        {action}
      </div>
      <div className="mt-2.5">{children}</div>
    </section>
  )
}

function ScopeRow({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: string }) {
  return (
    <div className="flex items-center gap-2">
      <Icon className="h-3.5 w-3.5 shrink-0 text-muted-foreground/70" />
      <span className="min-w-0 truncate text-[11px] text-muted-foreground">{label}</span>
      <span className="ml-auto shrink-0 text-[10.5px] text-foreground/80">{value}</span>
    </div>
  )
}

function RuntimeRow({ icon: Icon, text }: { icon: LucideIcon; text: string }) {
  return (
    <div className="flex items-start gap-2 text-[10.5px] leading-5 text-muted-foreground">
      <Icon className="mt-[3px] h-3 w-3 shrink-0 text-muted-foreground/70" />
      <span>{text}</span>
    </div>
  )
}

function DelegationNode({
  role,
  current,
  label,
  sub,
  currentLabel,
}: {
  role: AgentPromptRole
  current: boolean
  label: string
  sub: string
  currentLabel: string
}) {
  return (
    <div className={cn('flex items-center gap-2 rounded-md px-1.5 py-1', current && 'bg-muted/35')}>
      <span
        className={cn(
          'grid h-6 w-6 shrink-0 place-items-center rounded-md border',
          current
            ? 'border-border bg-card text-foreground'
            : 'border-border/60 bg-background/50 text-muted-foreground',
        )}
      >
        <RoleIcon role={role} className="h-3 w-3" />
      </span>
      <span className="text-[11px]">{label}</span>
      <span className="ml-auto truncate text-[9.5px] text-muted-foreground/70">{sub}</span>
      {current && (
        <span className="shrink-0 rounded border border-border bg-muted px-1 py-px text-[8.5px] text-foreground">
          {currentLabel}
        </span>
      )}
    </div>
  )
}

function WorkbenchSkeleton() {
  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-3 xl:grid-cols-[248px_minmax(0,1fr)] 2xl:grid-cols-[280px_minmax(0,1fr)_304px]" aria-hidden>
      <div className="min-h-[280px] rounded-xl border border-border/80 bg-card p-3">
        <div className="h-4 w-24 animate-pulse rounded bg-muted" />
        <div className="mt-3 h-8 w-full animate-pulse rounded-md bg-muted" />
        <div className="mt-2 h-7 w-full animate-pulse rounded-md bg-muted" />
        <div className="mt-4 space-y-2">
          {[0, 1, 2, 3].map((value) => (
            <div key={value} className="h-[62px] animate-pulse rounded-lg bg-muted/60" />
          ))}
        </div>
      </div>
      <div className="flex h-[70vh] min-h-[520px] flex-col rounded-xl border border-border/80 bg-card p-4 xl:h-auto xl:min-h-[520px] 2xl:h-auto 2xl:min-h-0">
        <div className="flex items-center gap-3">
          <div className="h-10 w-10 animate-pulse rounded-xl bg-muted" />
          <div className="flex-1 space-y-2">
            <div className="h-4 w-40 animate-pulse rounded bg-muted" />
            <div className="h-3 w-64 animate-pulse rounded bg-muted/70" />
          </div>
        </div>
        <div className="mt-5 h-9 w-full animate-pulse rounded-md bg-muted/60" />
        <div className="mt-3 min-h-0 flex-1 animate-pulse rounded-lg bg-muted/40" />
      </div>
      <div className="hidden flex-col gap-3 xl:col-span-2 xl:grid xl:grid-cols-2 2xl:col-span-1 2xl:flex">
        {[0, 1, 2].map((value) => (
          <div key={value} className="h-28 animate-pulse rounded-xl border border-border/80 bg-card" />
        ))}
      </div>
    </div>
  )
}
