'use client'

import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Archive, ChevronDown, ChevronRight, Network, Radar, RotateCcw } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useI18n } from '@/lib/i18n'
import {
  SPACE_ENGINES,
  SPACE_ENGINE_LABELS,
  buildSpaceEngineQuery,
  resolveSpaceEngineTarget,
  spaceEngineSearchPath,
  type SpaceEngine,
} from '@/lib/space-engine'
import type { AssetLedgerEntity } from '@/lib/api/unified-assets'
import { SourceBadges } from './SourceBadges'

function formatDate(value: string | undefined, locale: string) {
  if (!value || value.startsWith('0001-')) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale, { hour12: false })
}

export function AssetRootSheet({
  entity,
  open,
  onOpenChange,
  onArchive,
  onViewSubdomains,
}: {
  entity: AssetLedgerEntity | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onArchive: (entity: AssetLedgerEntity) => void
  onViewSubdomains: (entity: AssetLedgerEntity) => void
}) {
  const { t, language } = useI18n()
  const router = useRouter()
  const text = t.assetLedger
  const inventory = t.assetInventory
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'
  const statusLabels: Record<string, string> = {
    active: inventory.statusActive,
    stale: inventory.statusStale,
    retired: inventory.statusRetired,
  }
  const statusClasses: Record<string, string> = {
    active: 'border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300',
    stale: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    retired: 'border-muted-foreground/30 bg-muted text-muted-foreground',
  }
  const emptyState = (message: string) => (
    <div className="rounded-md border border-dashed p-4 text-center text-xs text-muted-foreground">{message}</div>
  )
  const spaceEngineTarget = entity ? resolveSpaceEngineTarget('domain', entity.value) : null
  const openSpaceEngine = (engine: SpaceEngine) => {
    if (!spaceEngineTarget) return
    router.push(spaceEngineSearchPath(engine, buildSpaceEngineQuery(engine, spaceEngineTarget)))
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right" aria-describedby={undefined} className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{text.rootDetailTitle}</SheetTitle>
        </SheetHeader>
        {entity ? (
          <div className="space-y-6 px-4 pb-6">
            <div className="space-y-4 rounded-lg border bg-card p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  {entity.name ? <p className="break-words text-sm font-medium">{entity.name}</p> : null}
                  <p className="break-all font-mono text-sm">{entity.displayValue || entity.value}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {text.tabRoot}{entity.scopeArchived ? ` · ${text.scopeBadgeRoot}` : ''}
                  </p>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <Badge variant="outline" className={statusClasses[entity.status] || statusClasses.retired}>
                    {statusLabels[entity.status] || entity.status || inventory.statusUnknown}
                  </Badge>
                </div>
              </div>

              <div className="flex flex-wrap gap-2">
                {!entity.archived ? (
                  <Button size="sm" variant="outline" onClick={() => onViewSubdomains(entity)}>
                    <Network className="size-3.5" />
                    {text.rootDetailViewSubdomains}
                  </Button>
                ) : null}
                <Button size="sm" variant="outline" onClick={() => onArchive(entity)}>
                  {entity.archived ? <RotateCcw className="size-3.5" /> : <Archive className="size-3.5" />}
                  {entity.archived ? text.restore : text.archive}
                </Button>
                {spaceEngineTarget ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button size="sm" variant="outline">
                        <Radar className="size-3.5" />
                        {text.spaceEngineQuery}
                        <ChevronDown className="size-3.5 opacity-60" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="start">
                      {SPACE_ENGINES.map((engine) => (
                        <DropdownMenuItem key={engine} onClick={() => openSpaceEngine(engine)}>
                          {SPACE_ENGINE_LABELS[engine]}
                        </DropdownMenuItem>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                ) : null}
              </div>
              <p className="text-xs text-muted-foreground">
                {entity.archived ? text.rootDetailArchivedHint : text.rootDetailHint}
              </p>
            </div>

            <Tabs defaultValue="overview" className="space-y-4">
              <TabsList className="h-auto w-full flex-wrap justify-start">
                <TabsTrigger value="overview" className="gap-2">
                  {text.overview}
                </TabsTrigger>
                <TabsTrigger value="sources" className="gap-2">
                  {text.sourceTasks}
                  <span className="rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">
                    {entity.sourceTaskIds.length}
                  </span>
                </TabsTrigger>
              </TabsList>

              <TabsContent value="overview" className="space-y-3">
                <dl className="grid grid-cols-2 gap-3 text-xs">
                  <div><dt className="text-muted-foreground">{text.subdomains}</dt><dd className="mt-1 tabular-nums">{entity.subdomainCount ?? 0}</dd></div>
                  <div><dt className="text-muted-foreground">{text.ips}</dt><dd className="mt-1 tabular-nums">{entity.ipCount ?? 0}</dd></div>
                  <div><dt className="text-muted-foreground">{text.ports}</dt><dd className="mt-1 tabular-nums">{entity.portCount ?? 0}</dd></div>
                  <div><dt className="text-muted-foreground">{text.websites}</dt><dd className="mt-1 tabular-nums">{entity.websiteCount ?? 0}</dd></div>
                  <div><dt className="text-muted-foreground">{inventory.firstSeen}</dt><dd className="mt-1">{formatDate(entity.firstSeenAt, locale)}</dd></div>
                  <div><dt className="text-muted-foreground">{text.lastSeen}</dt><dd className="mt-1">{formatDate(entity.lastSeenAt, locale)}</dd></div>
                  <div className="col-span-2">
                    <dt className="text-muted-foreground">{text.sources}</dt>
                    <dd className="mt-1">
                      <SourceBadges sources={entity.sources} />
                    </dd>
                  </div>
                  <div className="col-span-2">
                    <dt className="text-muted-foreground">{text.associatedIPs}</dt>
                    <dd className="mt-1 flex flex-wrap gap-1">
                      {entity.ips?.length ? entity.ips.map((ip) => (
                        <Badge key={ip} variant="outline" className="font-mono">{ip}</Badge>
                      )) : '—'}
                    </dd>
                  </div>
                </dl>
              </TabsContent>

              <TabsContent value="sources" className="space-y-2">
                {entity.sourceTaskIds.length ? entity.sourceTaskIds.map((taskID) => {
                  const name = taskID === entity.lastTaskId ? entity.lastTaskName : ''
                  return (
                    <Link
                      key={taskID}
                      href={`/scan/vuln-scan/detail?taskId=${encodeURIComponent(taskID)}`}
                      onClick={() => onOpenChange(false)}
                      className="flex items-start gap-3 rounded-md border bg-card p-3 text-xs transition-colors hover:border-primary/40 hover:bg-muted/40"
                    >
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-medium" title={name || taskID}>{name || taskID}</p>
                        {name ? <p className="mt-1 break-all font-mono text-[11px] text-muted-foreground">{taskID}</p> : null}
                      </div>
                      <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                    </Link>
                  )
                }) : emptyState(text.noSourceTasks)}
              </TabsContent>
            </Tabs>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
