'use client'

import { useEffect, useMemo, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { Archive, ChevronDown, ChevronRight, Pencil, Radar, RotateCcw, Save } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { useI18n } from '@/lib/i18n'
import toast from '@/lib/toast'
import {
  SPACE_ENGINES,
  SPACE_ENGINE_LABELS,
  buildSpaceEngineQuery,
  resolveSpaceEngineTarget,
  spaceEngineSearchPath,
  type SpaceEngine,
} from '@/lib/space-engine'
import {
  getUnifiedAssetDetail,
  listUnifiedAssetRelations,
  UnifiedAsset,
  UnifiedAssetObservation,
  UnifiedAssetRelation,
  UnifiedAssetSourceTask,
  updateAssetLedger,
} from '@/lib/api/unified-assets'

function formatDate(value: string | undefined, locale: string) {
  if (!value || value.startsWith('0001-')) return '—'
  return new Date(value).toLocaleString(locale, { hour12: false })
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useI18n()
  const labels: Record<string, string> = {
    active: t.assetInventory.statusActive,
    stale: t.assetInventory.statusStale,
    retired: t.assetInventory.statusRetired,
  }
  const classes: Record<string, string> = {
    active: 'border-green-500/30 bg-green-500/10 text-green-700 dark:text-green-300',
    stale: 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300',
    retired: 'border-muted-foreground/30 bg-muted text-muted-foreground',
  }
  return <Badge variant="outline" className={classes[status] || classes.retired}>{labels[status] || status || t.assetInventory.statusUnknown}</Badge>
}


export function AssetDetailSheet({
  assetId,
  open,
  onOpenChange,
  onChanged,
}: {
  assetId: string | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onChanged?: () => void
}) {
  const { t, language } = useI18n()
  const router = useRouter()
  const text = t.assetInventory
  const ledger = t.assetLedger
  const locale = language === 'zh' ? 'zh-CN' : 'en-US'
  const [asset, setAsset] = useState<UnifiedAsset | null>(null)
  const [observations, setObservations] = useState<UnifiedAssetObservation[]>([])
  const [relations, setRelations] = useState<UnifiedAssetRelation[]>([])
  const [relatedAssets, setRelatedAssets] = useState<Record<string, UnifiedAsset>>({})
  const [sourceTasks, setSourceTasks] = useState<UnifiedAssetSourceTask[]>([])
  const [sources, setSources] = useState<string[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [editing, setEditing] = useState(false)
  const [saving, setSaving] = useState(false)
  const [editName, setEditName] = useState('')
  const [editTags, setEditTags] = useState('')
  const [editGroup, setEditGroup] = useState('')
  const [editNotes, setEditNotes] = useState('')
  const [detailTab, setDetailTab] = useState('overview')

  const applyAsset = (next: UnifiedAsset) => {
    setAsset(next)
    setEditName(next.name || '')
    setEditTags((next.tags || []).join(', '))
    setEditGroup(next.groupKey || '')
    setEditNotes(next.notes || '')
  }

  useEffect(() => {
    if (!open || !assetId) return
    const controller = new AbortController()
    setLoading(true)
    setError('')
    setEditing(false)
    setDetailTab('overview')
    Promise.all([
      getUnifiedAssetDetail(assetId, controller.signal),
      listUnifiedAssetRelations(assetId, controller.signal),
    ]).then(async ([detail, relationData]) => {
      if (controller.signal.aborted) return
      applyAsset(detail.asset)
      setObservations(detail.observations || [])
      setSourceTasks(detail.sourceTasks || [])
      setSources(detail.sources || [])
      setRelations(relationData.items || [])
      const ids = Array.from(new Set(relationData.items.flatMap((relation) => [relation.fromAssetId, relation.toAssetId]).filter((id) => id && id !== assetId)))
      const resolved = await Promise.all(ids.map(async (id) => {
        try {
          const response = await getUnifiedAssetDetail(id, controller.signal)
          return [id, response.asset] as const
        } catch {
          return null
        }
      }))
      if (!controller.signal.aborted) setRelatedAssets(Object.fromEntries(resolved.filter((item): item is readonly [string, UnifiedAsset] => Boolean(item))))
    }).catch((err) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : text.detailError)
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [assetId, open, text.detailError])

  const sourceLabel = (source: string) => {
    switch (source) {
      case 'manual':
        return ledger.sourceManual
      case 'import':
        return ledger.sourceImport
      case 'portscan':
        return ledger.sourcePortscan
      case 'fingerprint':
      case 'webscan':
        return ledger.sourceFingerprint
      case 'subdomain':
        return ledger.sourceSubdomain
      case 'dirscan':
        return ledger.sourceDirscan
      case 'jsscan':
        return ledger.sourceJsscan
      case 'jsscan-vulnerability':
        return ledger.sourceJsscanVulnerability
      case 'webscan-vulnerability':
        return ledger.sourceVulnerability
      case 'findings':
        return ledger.sourceFindings
      case 'ai':
        return ledger.sourceAI
      case 'fofa':
        return ledger.sourceFofa
      case 'hunter':
        return ledger.sourceHunter
      case 'quake':
        return ledger.sourceQuake
      case 'uncover':
        return ledger.sourceUncover
      default:
        return source
    }
  }

  const relationRows = useMemo(() => relations.map((relation) => {
    const otherID = relation.fromAssetId === assetId ? relation.toAssetId : relation.fromAssetId
    return { relation, other: relatedAssets[otherID], otherID }
  }), [assetId, relatedAssets, relations])

  const spaceEngineTarget = asset ? resolveSpaceEngineTarget(asset.type, asset.displayValue || asset.canonicalValue) : null

  const openSpaceEngine = (engine: SpaceEngine) => {
    if (!spaceEngineTarget) return
    router.push(spaceEngineSearchPath(engine, buildSpaceEngineQuery(engine, spaceEngineTarget)))
  }

  const tasksByID = useMemo(() => {
    const map: Record<string, UnifiedAssetSourceTask> = {}
    for (const task of sourceTasks) map[task.id] = task
    return map
  }, [sourceTasks])

  const observationAttributes = (observation: UnifiedAssetObservation) => {
    const attributes = observation.attributes || {}
    const chips: Array<{ label: string; value: string }> = []
    const add = (label: string, key: string) => {
      const value = attributes[key]
      if (value === undefined || value === null || value === '' || typeof value === 'object') return
      chips.push({ label, value: String(value) })
    }
    add(text.port, 'port')
    add(ledger.protocol, 'protocol')
    add(ledger.transport, 'transport')
    const service = [attributes.product, attributes.serviceVersion]
      .filter((value) => value !== undefined && value !== null && String(value).trim() !== '')
      .map((value) => String(value))
      .join(' ')
    if (service) chips.push({ label: ledger.service, value: service })
    add(ledger.statusCode, 'status')
    add(ledger.websiteTitle, 'title')
    add(ledger.severity, 'severity')
    add(ledger.discoveryStatus, 'discoveryStatus')
    const fingerprints = Array.isArray(attributes.fingerprints) ? attributes.fingerprints.map((value) => String(value)) : []
    if (fingerprints.length) chips.push({ label: text.fingerprints, value: fingerprints.join(', ') })
    const highRisk = Array.isArray(attributes.highRiskFingerprints) ? attributes.highRiskFingerprints.map((value) => String(value)) : []
    if (highRisk.length) chips.push({ label: ledger.highRisk, value: highRisk.join(', ') })
    return chips.slice(0, 8)
  }

  const observationGroups = useMemo(() => {
    type Group = {
      key: string
      source: string
      taskId?: string
      taskName?: string
      version: number
      latest: string
      items: UnifiedAssetObservation[]
    }
    const groups = new Map<string, Group>()
    for (const observation of observations) {
      const task = observation.taskId ? tasksByID[observation.taskId] : undefined
      const reference = observation.taskId || observation.importBatchId || observation.origin || ''
      const version = observation.taskVersion || observation.sourceVersion || 0
      const key = [observation.source, reference, version].join('\u0000')
      let group = groups.get(key)
      if (!group) {
        group = {
          key,
          source: observation.source,
          taskId: observation.taskId,
          taskName: task?.name,
          version,
          latest: observation.observedAt,
          items: [],
        }
        groups.set(key, group)
      }
      group.items.push(observation)
      if (observation.observedAt > group.latest) group.latest = observation.observedAt
      if (!group.taskName && task?.name) group.taskName = task.name
    }
    return Array.from(groups.values()).sort((left, right) => right.latest.localeCompare(left.latest))
  }, [observations, tasksByID])

  const relationGroups = useMemo(() => {
    type Group = {
      key: string
      relationType: string
      direction: 'out' | 'in'
      latest: string
      items: typeof relationRows
    }
    const groups = new Map<string, Group>()
    for (const row of relationRows) {
      const direction: 'out' | 'in' = row.relation.fromAssetId === assetId ? 'out' : 'in'
      const key = [row.relation.relationType, direction].join('\u0000')
      let group = groups.get(key)
      if (!group) {
        group = {
          key,
          relationType: row.relation.relationType,
          direction,
          latest: row.relation.lastSeenAt,
          items: [],
        }
        groups.set(key, group)
      }
      group.items.push(row)
      if (row.relation.lastSeenAt > group.latest) group.latest = row.relation.lastSeenAt
    }
    return Array.from(groups.values()).sort((left, right) => right.latest.localeCompare(left.latest))
  }, [assetId, relationRows])

  const saveEdit = async () => {
    if (!assetId) return
    setSaving(true)
    try {
      const updated = await updateAssetLedger(assetId, {
        name: editName,
        notes: editNotes,
        tags: editTags.split(',').map((item) => item.trim()).filter(Boolean),
        groupKey: editGroup,
      })
      applyAsset(updated)
      setEditing(false)
      toast.success(t.common.updateSuccess)
      onChanged?.()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t.common.updateFailed)
    } finally {
      setSaving(false)
    }
  }

  const toggleArchive = async () => {
    if (!assetId || !asset) return
    setSaving(true)
    try {
      const updated = await updateAssetLedger(assetId, { archived: !asset.archivedAt })
      applyAsset(updated)
      toast.success(asset.archivedAt ? ledger.restoredToast : ledger.archivedToast)
      onChanged?.()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t.common.updateFailed)
    } finally {
      setSaving(false)
    }
  }

  const emptyState = (message: string) => (
    <div className="rounded-md border border-dashed p-4 text-center text-xs text-muted-foreground">{message}</div>
  )

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent data-testid="asset-detail-sheet" side="right" aria-describedby={undefined} className="w-full overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{text.details}</SheetTitle>
        </SheetHeader>
        {loading ? <div className="flex items-center justify-center p-12"><Spinner /></div> : error ? (
          <div className="space-y-3 p-4"><p className="text-sm text-destructive">{error}</p><Button variant="outline" onClick={() => onOpenChange(false)}>{t.common.close}</Button></div>
        ) : asset ? (
          <div className="space-y-6 px-4 pb-6">
            <div className="space-y-4 rounded-lg border bg-card p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  {asset.name ? <p className="break-words text-sm font-medium">{asset.name}</p> : null}
                  <p className="break-all font-mono text-sm">{asset.displayValue || asset.canonicalValue}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{asset.type}</p>
                </div>
                <div className="flex shrink-0 flex-col items-end gap-1">
                  <StatusBadge status={asset.status} />
                </div>
              </div>

              <div className="flex flex-wrap gap-2">
                <Button size="sm" variant="outline" onClick={() => { setEditing((value) => !value); setDetailTab('overview') }} disabled={saving}>
                  <Pencil className="size-3.5" />{ledger.edit}
                </Button>
                <Button size="sm" variant="outline" onClick={toggleArchive} disabled={saving}>
                  {asset.archivedAt ? <RotateCcw className="size-3.5" /> : <Archive className="size-3.5" />}
                  {asset.archivedAt ? ledger.restore : ledger.archive}
                </Button>
                {spaceEngineTarget ? (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button size="sm" variant="outline">
                        <Radar className="size-3.5" />
                        {ledger.spaceEngineQuery}
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
            </div>

            <Tabs value={detailTab} onValueChange={setDetailTab} className="space-y-4">
              <TabsList className="h-auto w-full flex-wrap justify-start">
                <TabsTrigger value="overview" className="gap-2">
                  {ledger.overview}
                </TabsTrigger>
                <TabsTrigger value="sources" className="gap-2">
                  {ledger.sourceTasks}
                  <span className="rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">{sourceTasks.length}</span>
                </TabsTrigger>
                <TabsTrigger value="observations" className="gap-2">
                  {text.observations}
                  <span className="rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">{observations.length}</span>
                </TabsTrigger>
                <TabsTrigger value="relations" className="gap-2">
                  {text.relations}
                  <span className="rounded-full bg-muted px-1.5 text-[11px] tabular-nums text-muted-foreground">{relationRows.length}</span>
                </TabsTrigger>
              </TabsList>

              <TabsContent value="overview" className="space-y-3">
                {editing ? (
                  <div className="space-y-2 rounded-md border p-3">
                    <div className="space-y-1"><Label htmlFor="asset-edit-name">{ledger.name}</Label><Input id="asset-edit-name" value={editName} onChange={(event) => setEditName(event.target.value)} /></div>
                    <div className="space-y-1"><Label htmlFor="asset-edit-tags">{ledger.tags}</Label><Input id="asset-edit-tags" value={editTags} placeholder={ledger.tagsPlaceholder} onChange={(event) => setEditTags(event.target.value)} /></div>
                    <div className="space-y-1"><Label htmlFor="asset-edit-group">{ledger.group}</Label><Input id="asset-edit-group" value={editGroup} onChange={(event) => setEditGroup(event.target.value)} /></div>
                    <div className="space-y-1"><Label htmlFor="asset-edit-notes">{ledger.notes}</Label><Textarea id="asset-edit-notes" rows={3} value={editNotes} onChange={(event) => setEditNotes(event.target.value)} /></div>
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="outline" onClick={() => setEditing(false)} disabled={saving}>{t.common.cancel}</Button>
                      <Button size="sm" onClick={saveEdit} disabled={saving}>{saving ? <Spinner className="size-3.5" /> : <Save className="size-3.5" />}{t.common.save}</Button>
                    </div>
                  </div>
                ) : null}

                <dl className="grid grid-cols-2 gap-3 text-xs">
                  <div><dt className="text-muted-foreground">{text.firstSeen}</dt><dd className="mt-1">{formatDate(asset.firstSeenAt, locale)}</dd></div>
                  <div><dt className="text-muted-foreground">{text.lastSeen}</dt><dd className="mt-1">{formatDate(asset.lastSeenAt, locale)}</dd></div>
                  <div><dt className="text-muted-foreground">{text.confidence}</dt><dd className="mt-1">{asset.confidence || '—'}</dd></div>
                  <div><dt className="text-muted-foreground">{text.scope}</dt><dd className="mt-1 break-all">{asset.scopeKey || '—'}</dd></div>
                  <div><dt className="text-muted-foreground">{ledger.group}</dt><dd className="mt-1 break-all">{asset.groupKey || '—'}</dd></div>
                  <div className="col-span-2"><dt className="text-muted-foreground">{ledger.tags}</dt><dd className="mt-1 flex flex-wrap gap-1">{(asset.tags || []).length ? (asset.tags || []).map((tag) => <Badge key={tag} variant="outline" className="font-normal">{tag}</Badge>) : '—'}</dd></div>
                  <div className="col-span-2"><dt className="text-muted-foreground">{ledger.notes}</dt><dd className="mt-1 whitespace-pre-wrap break-words">{asset.notes || '—'}</dd></div>
                  <div className="col-span-2"><dt className="text-muted-foreground">{ledger.sources}</dt><dd className="mt-1 flex flex-wrap gap-1">{sources.length ? sources.map((source) => <Badge key={source} variant="secondary" className="font-normal">{sourceLabel(source)}</Badge>) : '—'}</dd></div>
                </dl>
              </TabsContent>

              <TabsContent value="sources" className="space-y-2">
                {sourceTasks.length ? sourceTasks.map((task) => {
                  const missing = Boolean(task.missing) || task.status === 'missing'
                  const body = (
                    <>
                      <div className="min-w-0 flex-1">
                        {missing ? (
                          <>
                            <p className="text-sm font-medium text-muted-foreground">{ledger.sourceTaskDeleted}</p>
                            <p className="mt-1 break-all font-mono text-[11px] text-muted-foreground">{task.id} · v{task.version}</p>
                          </>
                        ) : (
                          <>
                            <p className="truncate text-sm font-medium" title={task.name || task.id}>{task.name || task.id}</p>
                            <p className="mt-1 text-muted-foreground">{task.scanType ? sourceLabel(task.scanType) : '—'} · v{task.version}</p>
                            {task.name && task.name !== task.id ? <p className="mt-1 truncate font-mono text-[11px] text-muted-foreground" title={task.id}>{task.id}</p> : null}
                          </>
                        )}
                      </div>
                      <div className="flex shrink-0 items-center gap-2">
                        <Badge variant="outline" className={missing ? 'text-muted-foreground' : undefined}>
                          {missing ? ledger.deletedLabel : task.status || '—'}
                        </Badge>
                        {missing ? null : <ChevronRight className="size-4 text-muted-foreground" />}
                      </div>
                    </>
                  )
                  if (missing) {
                    return (
                      <div key={`${task.id}:${task.version}`} className="flex items-start gap-3 rounded-md border bg-card p-3 text-xs" title={ledger.sourceTaskDeletedHint}>
                        {body}
                      </div>
                    )
                  }
                  return (
                    <Link
                      key={`${task.id}:${task.version}`}
                      href={`/scan/vuln-scan/detail?taskId=${encodeURIComponent(task.id)}`}
                      onClick={() => onOpenChange(false)}
                      className="flex items-start gap-3 rounded-md border bg-card p-3 text-xs transition-colors hover:border-primary/40 hover:bg-muted/40"
                    >
                      {body}
                    </Link>
                  )
                }) : emptyState(ledger.noSourceTasks)}
              </TabsContent>

              <TabsContent value="observations" className="space-y-2">
                {observationGroups.length ? observationGroups.map((group) => (
                  <details key={group.key} className="group rounded-md border bg-card text-xs">
                    <summary className="flex cursor-pointer list-none items-center gap-2 p-3 [&::-webkit-details-marker]:hidden">
                      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
                        <Badge variant="secondary" className="font-normal">{group.source ? sourceLabel(group.source) : '—'}</Badge>
                        {group.taskName ? <span className="max-w-[220px] truncate text-xs font-medium">{group.taskName}</span> : group.taskId ? <span className="max-w-[220px] truncate font-mono text-[11px] text-muted-foreground">{group.taskId}</span> : null}
                        {group.version ? <span className="text-[10px] text-muted-foreground">v{group.version}</span> : null}
                        {group.taskId && !tasksByID[group.taskId] ? <Badge variant="outline" className="text-muted-foreground">{ledger.deletedLabel}</Badge> : null}
                      </div>
                      <span className="shrink-0 text-[10px] text-muted-foreground">
                        {ledger.observationCount.replace('{count}', String(group.items.length))} · {ledger.latest} {formatDate(group.latest, locale)}
                      </span>
                      <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
                    </summary>
                    <div className="divide-y border-t">
                      {group.items.map((observation) => {
                        const rawSourceID = observation.sourceId || ''
                        const urlMatch = rawSourceID.match(/https?:\/\/[^\s]+/i)
                        const observationTarget = urlMatch
                          ? urlMatch[0].replace(/:(host|ip|domain|url|port)(?::.*)?$/i, '')
                          : observation.canonicalValue || observation.observationKey.slice(0, 12)
                        const chips = observationAttributes(observation)
                        const fullSummary = chips.map((chip) => `${chip.label} ${chip.value}`).join(' · ')
                        const summary = chips.slice(0, 3).map((chip) => `${chip.label} ${chip.value}`).join(' · ')
                        return (
                          <div key={observation.id} className="space-y-1 p-3">
                            <div className="flex items-center justify-between gap-2">
                              <span className="min-w-0 truncate font-mono text-[10px] text-muted-foreground">{observationTarget}</span>
                              <span className="shrink-0 text-[10px] text-muted-foreground">{formatDate(observation.observedAt, locale)}</span>
                            </div>
                            {summary ? <p className="truncate text-[11px] text-foreground/85" title={fullSummary}>{summary}</p> : null}
                            {observation.confidence && observation.confidence !== 'high' && observation.confidence !== 'medium' ? (
                              <Badge variant="outline" className="font-normal">{observation.confidence}</Badge>
                            ) : null}
                          </div>
                        )
                      })}
                    </div>
                  </details>
                )) : emptyState(text.noObservations)}
              </TabsContent>

              <TabsContent value="relations" className="space-y-2">
                {relationGroups.length ? relationGroups.map((group) => (
                  <details key={group.key} className="group rounded-md border bg-card text-xs">
                    <summary className="flex cursor-pointer list-none items-center gap-2 p-3 [&::-webkit-details-marker]:hidden">
                      <Badge variant="outline">{group.relationType}</Badge>
                      <span aria-hidden="true" className="text-muted-foreground">{group.direction === 'out' ? '→' : '←'}</span>
                      <Badge variant="secondary" className="font-normal">{group.items.length}</Badge>
                      <span className="ml-auto shrink-0 text-[10px] text-muted-foreground">{formatDate(group.latest, locale)}</span>
                      <ChevronRight className="size-3.5 shrink-0 text-muted-foreground transition-transform group-open:rotate-90" />
                    </summary>
                    <div className="divide-y border-t">
                      {group.items.map(({ relation, other, otherID }) => (
                        <div key={relation.id} className="flex items-start justify-between gap-3 p-3">
                          <div className="min-w-0">
                            <p className="break-all font-mono">{other?.displayValue || other?.canonicalValue || otherID}</p>
                            <p className="mt-1 text-[10px] text-muted-foreground">{other?.type || '—'}{relation.source ? ` · ${sourceLabel(relation.source)}` : ''}</p>
                          </div>
                          <span className="shrink-0 text-[10px] text-muted-foreground">{formatDate(relation.lastSeenAt, locale)}</span>
                        </div>
                      ))}
                    </div>
                  </details>
                )) : emptyState(text.noRelations)}
              </TabsContent>
            </Tabs>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}
