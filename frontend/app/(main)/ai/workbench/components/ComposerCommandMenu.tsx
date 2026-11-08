'use client'

import { forwardRef, useEffect, useImperativeHandle, useRef, useState, type KeyboardEvent } from 'react'
import { useI18n } from '@/lib/i18n'
import { listReferenceTasks, type ReferenceTask } from '@/lib/api/ai-asset-references'
import type { GoalModeAction } from '@/lib/ai-goal-mode'
import { mcpToolTitle, mcpToolSummary, type CallableMCPTool } from '@/lib/api/ai-mcp-tools'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Command, CommandList, CommandItem } from '@/components/ui/command'

export type ComposerCommand = { kind: '@' | '/'; query: string; start: number; end: number }
export function composerCommandAt(value: string, cursor: number): ComposerCommand | null {
  const match = value.slice(0, cursor).match(/(?:^|\s)([@/])([^\s@/]*)$/)
  if (!match) return null
  return { kind: match[1] as '@' | '/', query: match[2], start: cursor - match[2].length - 1, end: cursor }
}
export type ComposerCommandMenuHandle = { handleKey: (event: KeyboardEvent<HTMLTextAreaElement>) => boolean }

export const ComposerCommandMenu = forwardRef<ComposerCommandMenuHandle, {
  command: ComposerCommand
  onMode: (mode: GoalModeAction) => void
  onTask: (task: ReferenceTask) => void
  tools: CallableMCPTool[] | null
  toolError: string
  onTool: (tool: CallableMCPTool) => void
  onReloadTools: () => void
  onDismiss: () => void
}>(function ComposerCommandMenu({ command, onMode, onTask, tools, toolError, onTool, onReloadTools, onDismiss }, ref) {
  const { t, language } = useI18n()
  const zh = language === 'zh'
  const labels = t.mcpConnections.capabilityLabels
  const mcpCopy = t.mcpComposer
  const [index, setIndex] = useState(0)
  const [page, setPage] = useState(1)
  const [retry, setRetry] = useState(0)
  const key = `${command.query}:${page}:${retry}`
  const [result, setResult] = useState<{ key: string; tasks: ReferenceTask[]; total: number; error?: string } | null>(null)
  const list = useRef<HTMLDivElement>(null)
  const loading = command.kind === '@' && result?.key !== key
  useEffect(() => {
    if (command.kind !== '@') return
    const controller = new AbortController()
    const timer = setTimeout(() => {
      void listReferenceTasks(command.query, page, controller.signal).then((data) => {
        if (!controller.signal.aborted) setResult({ key, ...data })
      }).catch((error: Error) => { if (!controller.signal.aborted) setResult({ key, tasks: [], total: 0, error: error.message }) })
    }, 150)
    return () => { clearTimeout(timer); controller.abort() }
  }, [command.kind, command.query, page, key])
  const modes = [
    { id: 'start' as const, name: zh ? '目标' : 'Goal', aliases: 'goal 目标 计划', description: zh ? '自动规划并持续推进，默认不限轮数' : 'Plan and keep working, with no round limit' },
  ].filter((item) => item.aliases.includes(command.query.toLowerCase()))
  const needle = command.query.toLowerCase()
  const matchingTools = (tools || []).filter(tool => needle === 'mcp' || `${mcpToolTitle(tool, labels)} ${mcpToolSummary(tool, labels)} ${tool.name} ${tool.displayName} ${tool.sourceName || ''}`.toLowerCase().includes(needle))
  const tasks = !loading && !result?.error ? result?.tasks || [] : []
  const count = command.kind === '/' ? modes.length + matchingTools.length : tasks.length
  const active = Math.min(index, Math.max(0, count - 1))
  const choose = (i: number) => {
    if (command.kind === '@') { if (tasks[i]) onTask(tasks[i]); return }
    if (modes[i]) onMode(modes[i].id)
    else if (matchingTools[i - modes.length]) onTool(matchingTools[i - modes.length])
  }
  useEffect(() => { list.current?.querySelector('[aria-selected="true"]')?.scrollIntoView({ block: 'nearest' }) }, [active, count])
  useEffect(() => { if (list.current) list.current.scrollTop = 0 }, [page])
  useImperativeHandle(ref, () => ({ handleKey(event) {
    if (event.key === 'Escape') { event.preventDefault(); onDismiss(); return true }
    if (['ArrowUp', 'ArrowDown', 'Enter', 'Tab'].includes(event.key)) {
      event.preventDefault()
      if (event.key === 'Enter' || event.key === 'Tab') { if (count) choose(active) }
      else if (count) setIndex((active + (event.key === 'ArrowDown' ? 1 : -1) + count) % count)
      return true
    }
    return false
  } }))
  return <div data-slot="composer-command-menu" className="overflow-hidden rounded-2xl border border-border bg-popover p-1.5 text-popover-foreground shadow-2xl">
    <div className="flex items-center justify-between px-3 py-2 text-xs text-muted-foreground"><span>{command.kind === '@' ? (zh ? '引用攻击面任务 · 输入名称或目标搜索' : 'Reference a task · type to search') : (zh ? '命令' : 'Commands')}</span><span>↑ ↓ · Enter · Esc</span></div>
    <Command shouldFilter={false} value={String(active)} onValueChange={(value) => { if (value !== '') setIndex(Number(value)) }} label={zh ? '输入命令' : 'Input commands'}><CommandList id="ai-composer-command-list" ref={list} className="max-h-[min(22rem,40vh)] overflow-y-auto">
      {command.kind === '/' ? <>
        {modes.map((item, i) => <CommandItem key={item.id} value={String(i)} onMouseDown={(e) => e.preventDefault()} onMouseEnter={() => setIndex(i)} onSelect={() => choose(i)} className={cn('flex w-full items-center gap-3 rounded-xl px-3 py-3 text-left text-sm hover:bg-muted', 'data-[selected=true]:bg-muted data-[selected=true]:text-foreground', active === i && 'bg-muted')}><span className="w-5 text-center text-muted-foreground">/</span><span className="font-medium">{item.name}</span><span className="min-w-0 flex-1 text-muted-foreground">{item.description}</span></CommandItem>)}
        <div data-slot="composer-mcp-label" className="mx-3 flex items-center gap-2 border-t border-border pb-2 pt-3 text-xs text-muted-foreground"><span className="rounded-md border border-border bg-muted/40 px-1.5 py-0.5">MCP</span><span>{mcpCopy.available}</span>{tools && !toolError ? <span className="ml-auto tabular-nums">{matchingTools.length}</span> : null}</div>
        {toolError ? <div role="alert" className="px-3 py-3 text-sm">{mcpCopy.loadFailed}<p className="mt-1 text-xs text-muted-foreground">{toolError}</p><Button type="button" variant="ghost" size="sm" className="mt-2" onMouseDown={(event) => event.preventDefault()} onClick={onReloadTools}>{t.aiWorkbench.retry}</Button></div> : !tools ? <p role="status" className="px-3 py-3 text-sm text-muted-foreground">{mcpCopy.loading}</p> : !matchingTools.length ? <p className="px-3 py-3 text-sm text-muted-foreground">{tools.length ? mcpCopy.noMatches : mcpCopy.empty}</p> : matchingTools.map((tool, i) => {
          const itemIndex = modes.length + i
          const title = mcpToolTitle(tool, labels)
          const source = tool.source === 'external' ? tool.sourceName || t.mcpConnections.externalSource : tool.source === 'platform' ? t.mcpConnections.platformSource : t.mcpConnections.systemSource
          return <CommandItem key={tool.name} data-tool-name={tool.name} value={String(itemIndex)} onMouseDown={(event) => event.preventDefault()} onMouseEnter={() => setIndex(itemIndex)} onSelect={() => choose(itemIndex)} className={cn('flex w-full items-start gap-3 rounded-xl px-3 py-2.5 text-left text-sm hover:bg-muted', 'data-[selected=true]:bg-muted data-[selected=true]:text-foreground', active === itemIndex && 'bg-muted')}>
            <span aria-hidden="true" className="w-5 shrink-0 text-center text-muted-foreground">/</span>
            <span className="min-w-0 flex-1"><span className="flex items-center gap-2"><span className="min-w-0 flex-1 truncate font-medium" title={title}>{title}</span><span className="max-w-[35%] truncate text-xs text-muted-foreground" title={source}>{source}</span></span><span className="block truncate font-mono text-[11px] text-muted-foreground" title={tool.name}>{tool.name}</span><span className="line-clamp-2 text-xs leading-5 text-muted-foreground">{mcpToolSummary(tool, labels)}</span></span>
          </CommandItem>
        })}
      </> : loading ? <p role="status" className="px-3 py-4 text-sm text-muted-foreground">{zh ? '正在读取任务…' : 'Loading tasks…'}</p> : result?.error ? <div role="alert" className="px-3 py-4 text-sm">{result.error}<Button type="button" variant="ghost" size="sm" className="ml-3" onMouseDown={(event) => event.preventDefault()} onClick={() => setRetry(retry + 1)}>{zh ? '重试' : 'Retry'}</Button></div> : !count ? <p className="px-3 py-4 text-sm text-muted-foreground">{zh ? '没有匹配项，继续输入搜索或按 Esc 关闭' : 'No matches. Keep typing or press Esc.'}</p> : tasks.map((task, i) => <CommandItem key={`${task.id}:${task.version}`} value={String(i)} onMouseDown={(e) => e.preventDefault()} onMouseEnter={() => setIndex(i)} onSelect={() => choose(i)} className={cn('flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left text-sm hover:bg-muted', 'data-[selected=true]:bg-muted data-[selected=true]:text-foreground', active === i && 'bg-muted')}><span className="text-muted-foreground">@</span><span className="min-w-0 flex-1"><span className="block truncate font-medium">{task.name || task.id}</span><span className="block truncate text-xs text-muted-foreground">{task.targets} · {task.scanType} · v{task.version || '—'}</span></span><span className="text-muted-foreground">›</span></CommandItem>)}
    </CommandList></Command>
    {command.kind === '@' && result && result.total > 20 ? <div className="flex justify-between px-3 py-2 text-xs text-muted-foreground"><Button type="button" variant="ghost" size="sm" disabled={page === 1 || loading} onMouseDown={(event) => event.preventDefault()} onClick={() => { setPage(page - 1); setIndex(0) }}>{zh ? '上一页' : 'Previous'}</Button><span>{page} / {Math.ceil(result.total / 20)}</span><Button type="button" variant="ghost" size="sm" disabled={page * 20 >= result.total || loading} onMouseDown={(event) => event.preventDefault()} onClick={() => { setPage(page + 1); setIndex(0) }}>{zh ? '下一页' : 'Next'}</Button></div> : null}
  </div>
})
