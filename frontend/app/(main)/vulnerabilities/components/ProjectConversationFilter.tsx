'use client'

import { useEffect, useId, useRef, useState } from 'react'
import { Check, ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  listAIWorkbenchProjectConversationOptions,
  type AIWorkbenchConversationOption,
  type AIWorkbenchProject,
} from '@/lib/api/ai-workbench'
import { formatWorkbenchText, workbenchErrorMessage } from '@/lib/ai-workbench-i18n'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

type ConversationPage = {
  items: AIWorkbenchConversationOption[]
  page: number
  hasMore: boolean
  loading: boolean
  error?: string
}

export function ProjectConversationFilter({ projects, projectId, conversation, disabled, onChange }: {
  projects: AIWorkbenchProject[]
  projectId: string
  conversation: AIWorkbenchConversationOption | null
  disabled?: boolean
  onChange: (projectId: string, conversation: AIWorkbenchConversationOption | null) => void
}) {
  const { t } = useI18n()
  const l = t.vulnerabilityManagement
  const [open, setOpen] = useState(false)
  const [expandedProject, setExpandedProject] = useState<string | null>(null)
  const [pages, setPages] = useState<Record<string, ConversationPage>>({})
  const pending = useRef(new Set<string>())
  const mounted = useRef(true)
  const groupId = useId()
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])

  const projectName = projects.find(project => project.id === projectId)?.name || projectId
  const label = projectId === 'all' ? l.allProjects : conversation ? `${projectName} / ${conversation.name}` : projectName

  const loadConversations = async (id: string) => {
    if (pending.current.has(id)) return
    const previous = pages[id]
    const nextPage = (previous?.page || 0) + 1
    pending.current.add(id)
    setPages(current => ({ ...current, [id]: {
      items: previous?.items || [], page: previous?.page || 0,
      hasMore: previous?.hasMore ?? true, loading: true,
    } }))
    try {
      const response = await listAIWorkbenchProjectConversationOptions(id, nextPage)
      if (!mounted.current) return
      setPages(current => {
        const items = new Map((current[id]?.items || []).map(item => [item.id, item]))
        response.conversations.forEach(item => items.set(item.id, item))
        return { ...current, [id]: {
          items: [...items.values()], page: response.page,
          hasMore: response.hasMore, loading: false,
        } }
      })
    } catch (error) {
      if (!mounted.current) return
      setPages(current => ({ ...current, [id]: {
        ...current[id], loading: false,
        error: workbenchErrorMessage(error, t.aiWorkbench, l.loadConversationsFailed),
      } }))
    } finally {
      pending.current.delete(id)
    }
  }

  const select = (id: string, item: AIWorkbenchConversationOption | null) => {
    onChange(id, item)
    setOpen(false)
  }

  const toggleProject = (id: string) => {
    if (expandedProject === id) { setExpandedProject(null); return }
    setExpandedProject(id)
    if (!pages[id] || (pages[id].error && pages[id].page === 0)) void loadConversations(id)
  }

  const rowClass = 'flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50'
  const projectRowClass = 'flex w-full items-center gap-1 rounded-md hover:bg-muted'
  const projectButtonClass = 'flex min-w-0 flex-1 items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-transparent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50'

  return <Popover open={open} onOpenChange={setOpen}>
    <PopoverTrigger asChild>
      <Button type="button" variant="outline" disabled={disabled} aria-label={l.projectSessionFilter} title={label} className="w-full justify-between rounded-lg px-3 font-normal hover:border-border hover:bg-muted hover:text-foreground dark:hover:border-border dark:hover:bg-muted dark:hover:text-foreground xl:w-44">
        <span className="truncate">{label}</span>
        <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground opacity-50" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="end" aria-label={l.projectSessionFilter} className="w-80 max-w-[calc(100vw-2rem)] p-1">
      <div className="max-h-80 overflow-y-auto overscroll-contain">
        <button type="button" disabled={disabled} aria-pressed={projectId === 'all'} onClick={() => select('all', null)} className={cn(rowClass, projectId === 'all' && 'bg-muted')}>
          <span className="w-5 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{l.allProjects}</span>
          {projectId === 'all' && <Check className="h-4 w-4 shrink-0" />}
        </button>
        {projects.map(project => {
          const expanded = expandedProject === project.id
          const selected = projectId === project.id && !conversation
          const page = pages[project.id]
          const childId = `${groupId}-${project.id}`
          return <div key={project.id}>
            <div className={cn(projectRowClass, selected && 'bg-muted')}>
              <button type="button" disabled={disabled} aria-expanded={expanded} aria-controls={expanded ? childId : undefined} aria-label={formatWorkbenchText(expanded ? l.collapseProject : l.expandProject, { name: project.name })} onClick={() => toggleProject(project.id)} className="shrink-0 rounded-md p-2 text-muted-foreground hover:bg-transparent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <ChevronRight className={cn('h-4 w-4 transition-transform', expanded && 'rotate-90')} />
              </button>
              <button type="button" disabled={disabled} aria-pressed={selected} onClick={() => select(project.id, null)} title={project.name} className={cn(projectButtonClass, selected && 'bg-transparent')}>
                <span className="min-w-0 flex-1 truncate">{project.name}</span>
                {selected && <Check className="h-4 w-4 shrink-0" />}
              </button>
            </div>
            {expanded && <div id={childId} role="group" aria-label={project.name} aria-busy={page?.loading} className="mb-1 ml-5 border-l border-border pl-3">
              {page?.items.map(item => <button key={item.id} type="button" disabled={disabled} aria-pressed={projectId === project.id && conversation?.id === item.id} title={item.name} onClick={() => select(project.id, item)} className={cn(rowClass, projectId === project.id && conversation?.id === item.id && 'bg-muted')}>
                <span className="min-w-0 flex-1 truncate">{item.name}</span>
                {projectId === project.id && conversation?.id === item.id && <Check className="h-4 w-4 shrink-0" />}
              </button>)}
              {page?.loading && <div role="status" className="flex items-center gap-2 px-2 py-2 text-xs text-muted-foreground"><Loader2 className="h-3.5 w-3.5 animate-spin" />{l.loadingConversations}</div>}
              {page?.error && <div role="alert" className="px-2 py-2 text-xs text-muted-foreground">{page.error}</div>}
              {page && !page.loading && !page.error && page.items.length === 0 && <div className="px-2 py-2 text-xs text-muted-foreground">{l.noConversations}</div>}
              {page && !page.loading && (page.error || page.hasMore) && <button type="button" disabled={disabled} onClick={() => void loadConversations(project.id)} className={cn(rowClass, 'text-xs text-muted-foreground')}>
                {page.error ? l.retry : l.loadMoreConversations}
              </button>}
            </div>}
          </div>
        })}
      </div>
    </PopoverContent>
  </Popover>
}
