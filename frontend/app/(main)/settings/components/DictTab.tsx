'use client'

import Editor from '@monaco-editor/react'
import { useCallback, useDeferredValue, useEffect, useMemo, useState } from 'react'
import '@/lib/monaco'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { DataTableWithColumns, type ColumnDef as TableColumnDef } from '@/components/ui/data-table-with-columns'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from 'react-hot-toast'
import {
  Database,
  HelpCircle,
  Info,
  Layers3,
  MoreVertical,
  Network,
  Plus,
  RefreshCw,
  Rows3,
  Search,
} from 'lucide-react'

interface Dictionary {
  id: string
  name: string
  source: string
  protocol: string
  group: string
  size: string
  entries: number
  lastModified: string
  description?: string
  origin: 'bundled' | 'custom'
}

interface FilterStat {
  key: string
  label: string
  count: number
}

const sourceLabels: Record<string, string> = {
  crack: 'Crack',
  dirsearch: 'Dirsearch',
  jwt: 'JWT',
}

const summarizeDescription = (value?: string): string => {
  if (!value || !value.trim()) {
    return '无'
  }

  const normalized = value
    .replace(/\r?\n+/g, ' | ')
    .replace(/\s{2,}/g, ' ')
    .replace(/\s*\|\s*/g, ' | ')
    .trim()

  if (normalized.length <= 72) {
    return normalized
  }

  return `${normalized.slice(0, 69)}...`
}

const metaInputClass =
  'h-11 rounded-lg border-border/80 bg-muted/35 px-3 font-mono text-sm shadow-none disabled:cursor-default disabled:opacity-100'

export function DictTab() {
  const { t } = useI18n()
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedGroup, setSelectedGroup] = useState('all')
  const [dictionaries, setDictionaries] = useState<Dictionary[]>([])
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false)
  const [isAddDialogOpen, setIsAddDialogOpen] = useState(false)
  const [isSaving, setIsSaving] = useState(false)
  const [selectedFileName, setSelectedFileName] = useState('')
  const [selectedDictionary, setSelectedDictionary] = useState<Dictionary | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Dictionary | null>(null)
  const [loading, setLoading] = useState(false)
  const [editorContent, setEditorContent] = useState('')
  const [formData, setFormData] = useState({
    name: '',
    source: 'crack',
    protocol: '',
    group: '',
    description: '',
  })
  const deferredEditorContent = useDeferredValue(editorContent)

  const fetchDictionaries = useCallback(async () => {
    const res = await fetchWithAuth('/api/v1/dict/crack')
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}`)
    }
    const data = await res.json()
    if (data.success && data.data?.dicts) {
      return data.data.dicts.map((d: any, idx: number) => ({
        id: `${d.source || 'crack'}-${d.name}-${idx}`,
        name: d.name,
        source: d.source || 'crack',
        protocol: d.protocol || '',
        group: d.group || 'crack',
        size: d.size || '0 B',
        entries: d.entries || 0,
        lastModified: d.lastModified || '-',
        description: d.description || '',
        origin: d.origin === 'custom' ? 'custom' : 'bundled',
      })) as Dictionary[]
    }
    return []
  }, [])

  const loadDictionaries = useCallback(async () => {
    setLoading(true)
    try {
      setDictionaries(await fetchDictionaries())
    } catch (error) {
      console.error(t.dict.loadFailed, error)
    } finally {
      setLoading(false)
    }
  }, [fetchDictionaries, t.dict.loadFailed])

  const loadDictionaryContent = useCallback(async (dict: Pick<Dictionary, 'name' | 'source'>) => {
    try {
      const res = await fetchWithAuth(
        `/api/v1/dict/crack/content?source=${encodeURIComponent(dict.source)}&name=${encodeURIComponent(dict.name)}`
      )
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`)
      }
      const data = await res.json()
      if (data.success && data.data) {
        return {
          content: String(data.data.content || ''),
          protocol: String(data.data.protocol || ''),
          group: String(data.data.group || ''),
          description: String(data.data.description || ''),
        }
      }
    } catch (error) {
      console.error(t.dict.loadContentFailed, error)
    }
    return { content: '', protocol: '', group: '', description: '' }
  }, [t.dict.loadContentFailed])

  useEffect(() => {
    let cancelled = false

    const loadInitialDictionaries = async () => {
      setLoading(true)
      try {
        const dicts = await fetchDictionaries()
        if (!cancelled) {
          setDictionaries(dicts)
        }
      } catch (error) {
        if (!cancelled) {
          console.error(t.dict.loadFailed, error)
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    void loadInitialDictionaries()

    return () => {
      cancelled = true
    }
  }, [fetchDictionaries, t.dict.loadFailed])

  const groupStats = useMemo<FilterStat[]>(() => {
    const map = new Map<string, number>()
    for (const dict of dictionaries) {
      map.set(dict.group, (map.get(dict.group) || 0) + 1)
    }
    return Array.from(map.entries())
      .map(([key, count]) => ({ key, label: key, count }))
      .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
  }, [dictionaries])

  const protocolStats = useMemo<FilterStat[]>(() => {
    const map = new Map<string, number>()
    for (const dict of dictionaries) {
      map.set(dict.protocol || 'Unknown', (map.get(dict.protocol || 'Unknown') || 0) + 1)
    }
    return Array.from(map.entries())
      .map(([key, count]) => ({ key, label: key, count }))
      .sort((a, b) => b.count - a.count || a.label.localeCompare(b.label))
  }, [dictionaries])

  const filteredDictionaries = useMemo(() => {
    const q = searchQuery.trim().toLowerCase()
    return dictionaries.filter((dict) => {
      const matchesGroup = selectedGroup === 'all' || dict.group === selectedGroup
      const matchesSearch =
        q === '' ||
        [dict.name, dict.source, dict.group, dict.protocol, dict.description || '']
          .join(' ')
          .toLowerCase()
          .includes(q)
      return matchesGroup && matchesSearch
    })
  }, [dictionaries, searchQuery, selectedGroup])

  const totalEntries = useMemo(
    () => filteredDictionaries.reduce((sum, item) => sum + item.entries, 0),
    [filteredDictionaries]
  )

  const resetFilters = () => {
    setSelectedGroup('all')
    setSearchQuery('')
  }

  const openEditDialog = useCallback(async (dict: Dictionary) => {
    const detail = await loadDictionaryContent(dict)
    setSelectedDictionary(dict)
    setFormData({
      name: dict.name,
      source: dict.source,
      protocol: detail.protocol || dict.protocol,
      group: detail.group || dict.group,
      description: detail.description || dict.description || '',
    })
    setEditorContent(detail.content)
    setIsEditDialogOpen(true)
  }, [loadDictionaryContent])

  const openAddDialog = () => {
    setFormData({ name: '', source: 'crack', protocol: '', group: 'crack', description: '' })
    setEditorContent('')
    setSelectedFileName('')
    setIsAddDialogOpen(true)
  }

  const handleAddFile = async (file?: File) => {
    if (!file) return
    if (!file.name.toLowerCase().endsWith('.txt')) {
      toast.error(t.dict.invalidFile)
      return
    }
    setFormData((current) => ({ ...current, name: file.name }))
    setEditorContent(await file.text())
    setSelectedFileName(file.name)
  }

  const handleAdd = async () => {
    const name = formData.name.trim()
    if (!name || !editorContent.trim()) {
      toast.error(t.dict.addRequired)
      return
    }
    if (formData.source === 'crack' && !formData.protocol.trim()) {
      toast.error(t.dict.crackProtocolRequired)
      return
    }
    setIsSaving(true)
    try {
      const res = await fetchWithAuth('/api/v1/dict/crack/content?create=true', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ...formData, name, content: editorContent }),
      })
      const data = await res.json()
      if (!res.ok || !data.success) {
        throw new Error(data.error || t.dict.uploadFailed)
      }
      await loadDictionaries()
      setIsAddDialogOpen(false)
      toast.success(t.dict.uploadSuccess)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t.dict.uploadFailed)
    } finally {
      setIsSaving(false)
    }
  }

  const handleEdit = async () => {
    if (!selectedDictionary) return
    try {
      const res = await fetchWithAuth('/api/v1/dict/crack/content', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          ...formData,
          content: editorContent,
        }),
      })
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`)
      }
      const data = await res.json()
      if (!data.success) {
        throw new Error(data.error || t.dict.editFailed)
      }
      await loadDictionaries()
      setIsEditDialogOpen(false)
      setSelectedDictionary(null)
      setEditorContent('')
    } catch (error) {
      console.error(t.dict.editFailed, error)
    }
  }

  const handleDelete = async (dict: Dictionary) => {
    try {
      const res = await fetchWithAuth(
        `/api/v1/dict/crack/content?source=${encodeURIComponent(dict.source)}&name=${encodeURIComponent(dict.name)}`,
        { method: 'DELETE' }
      )
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`)
      }
      const data = await res.json()
      if (!data.success) {
        throw new Error(data.error || t.dict.deleteFailed)
      }
      await loadDictionaries()
      setDeleteTarget(null)
    } catch (error) {
      console.error(t.dict.deleteFailed, error)
    }
  }

  const tableColumns = useMemo<TableColumnDef<Dictionary>[]>(() => {
    return [
      {
        id: 'name',
        header: t.dict.name,
        headerClassName: 'w-[320px]',
        className: 'w-[320px]',
        cell: (dict) => (
          <div className="flex items-center gap-3">
            <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-background">
              <Database className="h-4 w-4 text-primary" />
            </div>
            <div className="min-w-0 truncate font-medium text-foreground" title={dict.name}>
              {dict.name}
            </div>
          </div>
        ),
      },
      {
        id: 'source',
        header: t.dict.source,
        headerClassName: 'w-[120px]',
        className: 'w-[120px]',
        cell: (dict) => (
          <Badge variant="outline">{dict.origin === 'custom' ? t.dict.custom : t.dict.bundled}</Badge>
        ),
      },
      {
        id: 'group',
        header: t.dict.group,
        headerClassName: 'w-[120px]',
        className: 'w-[120px]',
        cell: (dict) => <Badge variant="secondary">{dict.group}</Badge>,
      },
      {
        id: 'protocol',
        header: t.dict.protocolLabel,
        headerClassName: 'w-[140px]',
        className: 'w-[140px]',
        cell: (dict) => (
          <div className="truncate" title={dict.protocol || t.common.none}>
            {dict.protocol || t.common.none}
          </div>
        ),
      },
      {
        id: 'entries',
        header: t.dict.entries,
        headerClassName: 'w-[110px] text-right',
        className: 'w-[110px] text-right font-medium',
        cell: (dict) => dict.entries.toLocaleString(),
      },
      {
        id: 'size',
        header: t.dict.size,
        headerClassName: 'w-[120px]',
        className: 'w-[120px]',
        cell: (dict) => dict.size,
      },
      {
        id: 'lastModified',
        header: t.dict.updatedAt,
        headerClassName: 'w-[140px]',
        className: 'w-[140px]',
        cell: (dict) => dict.lastModified,
      },
      {
        id: 'description',
        header: t.common.description,
        headerClassName: 'min-w-[260px]',
        className: 'min-w-[260px]',
        cell: (dict) => (
          <div className="truncate text-sm text-muted-foreground" title={dict.description || t.common.none}>
            {summarizeDescription(dict.description)}
          </div>
        ),
      },
      {
        id: '__actions__',
        header: '',
        headerClassName: 'w-[64px] min-w-[64px] !px-0 text-center sticky right-0 z-20 border-l border-border bg-card',
        className:
          'w-[64px] min-w-[64px] !px-0 text-center sticky right-0 z-10 border-l border-border bg-card group-hover:bg-muted group-data-[state=selected]:bg-muted',
        alwaysVisible: true,
        cell: (dict) => (
          <div className="flex w-full items-center justify-center">
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8">
                  <MoreVertical className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={() => openEditDialog(dict)}>{t.dict.edit}</DropdownMenuItem>
                {dict.origin === 'custom' && (
                  <DropdownMenuItem onClick={() => setDeleteTarget(dict)} className="text-destructive">
                    {t.common.delete}
                  </DropdownMenuItem>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        ),
      },
    ]
  }, [openEditDialog, t])

  return (
    <div className="xl:flex xl:h-full xl:min-h-0 xl:flex-col">
      <div className="xl:flex xl:min-h-0 xl:flex-1 xl:flex-col">
        <section className="xl:flex xl:min-h-0 xl:flex-1 xl:flex-col">
          <div className="border-b border-border pb-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold">{t.dict.managementTitle}</h2>
                <p className="mt-1 text-sm text-muted-foreground">{t.dict.managementSubtitle}</p>
              </div>
              <Button onClick={openAddDialog}>
                <Plus className="mr-2 h-4 w-4" />
                {t.dict.add}
              </Button>
            </div>

            <div className="mt-4 flex gap-3 rounded-lg border border-border bg-muted/40 px-4 py-3">
              <Info className="mt-0.5 h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
              <div className="min-w-0 text-sm">
                <div className="font-medium text-foreground">{t.dict.crackPriorityTitle}</div>
                <p className="mt-0.5 text-muted-foreground">{t.dict.crackPriorityDescription}</p>
              </div>
            </div>

            <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
              <div className="relative min-w-0 flex-1 sm:min-w-[240px]">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder={t.dict.searchPlaceholder}
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-10"
                />
              </div>
              <Select value={selectedGroup} onValueChange={setSelectedGroup}>
                <SelectTrigger className="w-full sm:w-[220px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">
                    {t.dict.allGroups} ({dictionaries.length})
                  </SelectItem>
                  {groupStats.map((item) => (
                    <SelectItem key={item.key} value={item.key}>
                      {item.label} ({item.count})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="flex gap-2">
                <Button variant="outline" onClick={resetFilters}>
                  {t.dict.resetFilters}
                </Button>
                <Button variant="outline" size="icon" onClick={loadDictionaries} aria-label={t.dict.refresh}>
                  <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
                </Button>
              </div>
            </div>

            <div className="mt-4 flex flex-wrap items-center gap-2">
              <Badge variant="outline">{t.dict.totalDicts}: {filteredDictionaries.length}</Badge>
              <Badge variant="outline" className="gap-1">
                <Layers3 className="h-3 w-3" />
                {t.dict.allGroups}: {groupStats.length}
              </Badge>
              <Badge variant="outline" className="gap-1">
                <Network className="h-3 w-3" />
                {t.dict.allProtocols}: {protocolStats.length}
              </Badge>
              <Badge variant="outline" className="gap-1">
                <Rows3 className="h-3 w-3" />
                {t.dict.entries}: {totalEntries.toLocaleString()}
              </Badge>
              {selectedGroup !== 'all' ? <Badge>{selectedGroup}</Badge> : null}
            </div>
          </div>

          <div className="min-h-0 pt-4 xl:flex-1 xl:overflow-y-auto">
            <div className="hidden md:block"><DataTableWithColumns
              data={filteredDictionaries}
              columns={tableColumns}
              loading={loading}
              emptyMessage={t.dict.noResults}
              showToolbar={false}
              showColumnToggle={false}
              getRowKey={(dict) => dict.id}
            /></div>
            <div className="space-y-3 md:hidden">
              {loading ? <p className="py-8 text-center text-muted-foreground">{t.common.loading}</p> : filteredDictionaries.length === 0 ? <p className="py-8 text-center text-muted-foreground">{t.dict.noResults}</p> : filteredDictionaries.map((dict) => (
                <article key={dict.id} className="space-y-3 rounded-lg border bg-card p-3">
                  <div className="flex items-start justify-between gap-2">
                    <h3 className="min-w-0 break-all font-medium">{dict.name}</h3>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild><Button variant="ghost" size="icon" aria-label={`${t.common.action}: ${dict.name}`}><MoreVertical className="size-4" /></Button></DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem onClick={() => openEditDialog(dict)}>{t.dict.edit}</DropdownMenuItem>
                        {dict.origin === 'custom' ? <DropdownMenuItem onClick={() => setDeleteTarget(dict)} className="text-destructive">{t.common.delete}</DropdownMenuItem> : null}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  <div className="flex flex-wrap gap-2"><Badge variant="outline">{dict.origin === 'custom' ? t.dict.custom : t.dict.bundled}</Badge><Badge variant="secondary">{dict.group}</Badge>{dict.protocol ? <Badge variant="outline">{dict.protocol}</Badge> : null}</div>
                  <dl className="grid grid-cols-2 gap-2 text-xs text-muted-foreground">
                    <div><dt>{t.dict.entries}</dt><dd className="mt-1 text-foreground">{dict.entries.toLocaleString()}</dd></div>
                    <div><dt>{t.dict.size}</dt><dd className="mt-1 text-foreground">{dict.size}</dd></div>
                    <div className="col-span-2"><dt>{t.dict.updatedAt}</dt><dd className="mt-1 text-foreground">{dict.lastModified}</dd></div>
                  </dl>
                  {dict.description ? <p className="break-words text-sm text-muted-foreground">{dict.description}</p> : null}
                </article>
              ))}
            </div>
          </div>
        </section>
      </div>

      <Dialog open={isAddDialogOpen} onOpenChange={setIsAddDialogOpen}>
        <DialogContent className="settings-dialog flex h-[calc(100dvh-2rem)] w-[min(960px,calc(100vw-2rem))] sm:!max-w-[960px] flex-col overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border px-4 py-4 sm:px-6 sm:py-5">
            <DialogTitle>{t.dict.add}</DialogTitle>
            <DialogDescription>{t.dict.addDescription}</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-5">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="add-source">{t.dict.module}</Label>
                <Select
                  value={formData.source}
                  onValueChange={(source) => setFormData((current) => ({
                    ...current, source, group: current.group === current.source ? source : current.group,
                  }))}
                >
                  <SelectTrigger id="add-source"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {Object.entries(sourceLabels).map(([value, label]) => (
                      <SelectItem key={value} value={value}>{label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="add-name">{t.dict.name}</Label>
                <Input
                  id="add-name"
                  placeholder="example.txt"
                  value={formData.name}
                  onChange={(e) => setFormData((current) => ({ ...current, name: e.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="add-group">{t.dict.group}</Label>
                <Input
                  id="add-group"
                  value={formData.group}
                  onChange={(e) => setFormData((current) => ({ ...current, group: e.target.value }))}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="add-protocol">{t.dict.protocolLabel}</Label>
                <Input
                  id="add-protocol"
                  value={formData.protocol}
                  onChange={(e) => setFormData((current) => ({ ...current, protocol: e.target.value }))}
                />
                {formData.source === 'crack' && <p className="text-xs text-muted-foreground">{t.dict.crackProtocolHint}</p>}
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="add-description">{t.common.description}</Label>
                <Input
                  id="add-description"
                  placeholder={t.dict.descriptionPlaceholder}
                  value={formData.description}
                  onChange={(e) => setFormData((current) => ({ ...current, description: e.target.value }))}
                />
              </div>
              <div className="space-y-2 md:col-span-2">
                <Label htmlFor="add-file">{t.dict.upload}</Label>
                <Input
                  id="add-file"
                  type="file"
                  accept=".txt,text/plain"
                  onChange={(e) => void handleAddFile(e.target.files?.[0])}
                />
                {selectedFileName && <p className="text-xs text-muted-foreground">{selectedFileName}</p>}
              </div>
            </div>
            <Separator className="my-5" />
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label>{t.dict.addContent}</Label>
                <span className="text-xs text-muted-foreground">
                  {t.dict.editDialog.totalLines.replace('{count}', String(deferredEditorContent.split('\n').filter((line) => line.trim()).length))}
                </span>
              </div>
              <div className="h-[36vh] min-h-[220px] overflow-hidden rounded-lg border border-border bg-background">
                <Editor
                  height="100%"
                  defaultLanguage="plaintext"
                  value={editorContent}
                  onChange={(value) => setEditorContent(value ?? '')}
                  options={{ automaticLayout: true, wordWrap: 'on', minimap: { enabled: false }, scrollBeyondLastLine: false, fontSize: 13 }}
                  theme="vs-dark"
                />
              </div>
            </div>
          </div>
          <DialogFooter className="shrink-0 border-t border-border px-6 py-4">
            <Button variant="outline" onClick={() => setIsAddDialogOpen(false)}>{t.common.cancel}</Button>
            <Button disabled={isSaving} onClick={handleAdd}>{isSaving ? t.dict.saving : t.dict.add}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={isEditDialogOpen} onOpenChange={setIsEditDialogOpen}>
        <DialogContent className="settings-dialog flex h-[calc(100dvh-2rem)] w-[min(960px,calc(100vw-2rem))] sm:!max-w-[960px] flex-col overflow-hidden p-0">
          <DialogHeader className="shrink-0 border-b border-border px-4 py-4 sm:px-6 sm:py-5">
            <DialogTitle>{t.dict.editDialog.title}</DialogTitle>
            <DialogDescription>
              {selectedDictionary?.origin === 'bundled' ? t.dict.bundledEditHint : t.dict.editDialog.description}
            </DialogDescription>
          </DialogHeader>

          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="flex min-h-full flex-col gap-5 px-4 py-4 sm:px-6 sm:py-5">
              <div className="grid shrink-0 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label htmlFor="edit-name">{t.dict.name}</Label>
                  <Input id="edit-name" value={formData.name} disabled className={metaInputClass} />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-source">{t.dict.module}</Label>
                  <Input
                    id="edit-source"
                    value={sourceLabels[formData.source] || formData.source}
                    disabled
                    className={metaInputClass}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-group">{t.dict.group}</Label>
                  <Input
                    id="edit-group"
                    placeholder="jwt / crack / web / dirsearch"
                    value={formData.group}
                    onChange={(e) => setFormData((current) => ({ ...current, group: e.target.value }))}
                    className={metaInputClass}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-protocol">{t.dict.protocolLabel}</Label>
                  <Input
                    id="edit-protocol"
                    placeholder="JWT / SSH / Dirsearch"
                    value={formData.protocol}
                    onChange={(e) => setFormData((current) => ({ ...current, protocol: e.target.value }))}
                    className={metaInputClass}
                  />
                </div>
              </div>

              <div className="shrink-0 space-y-2">
                <Label htmlFor="edit-description">{t.common.description}</Label>
                <Input
                  id="edit-description"
                  placeholder={t.dict.descriptionPlaceholder}
                  value={formData.description}
                  onChange={(e) => setFormData((current) => ({ ...current, description: e.target.value }))}
                  className={metaInputClass.replace('font-mono', '')}
                />
              </div>

              <Separator className="shrink-0" />

              <div className="flex min-h-[240px] flex-1 flex-col gap-2">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Label htmlFor="edit-content">{t.dict.editDialog.content}</Label>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <HelpCircle className="h-3.5 w-3.5 text-muted-foreground" />
                        </TooltipTrigger>
                        <TooltipContent side="right">
                          <p className="max-w-xs text-xs">{t.dict.editDialog.contentHelp}</p>
                        </TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {t.dict.editDialog.totalLines.replace(
                      '{count}',
                      String(deferredEditorContent.split('\n').filter((line) => line.trim()).length)
                    )}
                  </div>
                </div>
                <div className="relative min-h-[200px] flex-1 overflow-hidden rounded-lg border border-border bg-background">
                  <div className="absolute inset-0">
                    <Editor
                      height="100%"
                      defaultLanguage="plaintext"
                      value={editorContent}
                      onChange={(value) => setEditorContent(value ?? '')}
                      options={{
                        automaticLayout: true,
                        wordWrap: 'on',
                        minimap: { enabled: false },
                        scrollBeyondLastLine: false,
                        lineNumbers: 'on',
                        glyphMargin: false,
                        folding: false,
                        fontSize: 13,
                        fontFamily:
                          'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, monospace',
                        renderWhitespace: 'selection',
                        padding: { top: 12, bottom: 12 },
                      }}
                      theme="vs-dark"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <DialogFooter className="shrink-0 border-t border-border px-6 py-4">
            <Button
              variant="outline"
              onClick={() => {
                setIsEditDialogOpen(false)
                setSelectedDictionary(null)
                setEditorContent('')
              }}
            >
              {t.common.cancel}
            </Button>
            <Button onClick={handleEdit}>{t.dict.editDialog.save}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.common.delete}</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget ? t.dict.confirmDelete.replace('{name}', deleteTarget.name) : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t.common.cancel}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              onClick={() => deleteTarget && handleDelete(deleteTarget)}
            >
              {t.common.delete}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
