'use client'

import { useState, useEffect, useCallback, useSyncExternalStore } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { useTheme } from 'next-themes'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
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
  Check,
  Palette,
  Moon,
  Sun,
  Flame,
  Rows3,
  PanelLeft,
  PanelTop,
  Sparkles,
  Plus,
  Upload,
  Download,
  Trash2,
  Loader2,
  Pencil,
  RefreshCw,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import { persistDesktopConfigItems } from '@/lib/api'
import {
  setContentTabsEnabled as persistContentTabsEnabled,
  setSidebarVariant as persistSidebarVariant,
  subscribeContentTabsEnabled,
  subscribeSidebarVariant,
  type SidebarVariant,
} from '@/lib/appearance'
import { deleteCustomTheme, listCustomThemes } from '@/lib/api/themes'
import {
  applyCustomTheme,
  buildThemeLibraryBundle,
  clearCustomThemeCache,
  getActiveCustomThemeId,
  getCachedCustomTheme,
  CUSTOM_THEME_ID,
  type CustomThemeRecord,
} from '@/lib/theme/custom-theme'
import { saveBlobAsFile } from '@/lib/download'
import toast from '@/lib/toast'
import { CustomThemeImportDialog } from './CustomThemeImportDialog'
import { ThemePreview } from './ThemePreview'

const themes = [
  {
    id: 'flame',
    nameKey: 'flame' as const,
    icon: Flame,
    bg: 'bg-zinc-900',
    skeleton: 'bg-orange-500/30',
    accent: 'bg-orange-500',
  },
  {
    id: 'dark',
    nameKey: 'dark' as const,
    icon: Moon,
    bg: 'bg-zinc-900',
    skeleton: 'bg-zinc-700',
    accent: 'bg-zinc-600',
  },
  {
    id: 'light',
    nameKey: 'light' as const,
    icon: Sun,
    bg: 'bg-white border',
    skeleton: 'bg-gray-200',
    accent: 'bg-gray-300',
  },
  {
    id: 'shadcn-light',
    nameKey: 'shadcnLight' as const,
    icon: Sun,
    bg: 'bg-white border border-neutral-200',
    skeleton: 'bg-neutral-200',
    accent: 'bg-neutral-900',
  },
  {
    id: 'shadcn-dark',
    nameKey: 'shadcnDark' as const,
    icon: Moon,
    bg: 'bg-neutral-950',
    skeleton: 'bg-neutral-700',
    accent: 'bg-neutral-100',
  },
  {
    id: 'paper-ink',
    nameKey: 'paperInk' as const,
    icon: Palette,
    bg: 'bg-[#f7f3eb] border border-[#d8cdbf]',
    skeleton: 'bg-[#d7cabb]',
    accent: 'bg-[#355070]',
  },
  {
    id: 'graphite',
    nameKey: 'graphite' as const,
    icon: Palette,
    bg: 'bg-[#17181b]',
    skeleton: 'bg-[#4b5058]',
    accent: 'bg-[#6f8fb5]',
  },
  {
    id: 'glass',
    nameKey: 'glass' as const,
    icon: Sparkles,
    bg: 'bg-[linear-gradient(135deg,rgba(255,255,255,0.88),rgba(232,238,246,0.64)_54%,rgba(248,250,252,0.82))] border border-slate-300/45 shadow-[inset_0_1px_0_rgba(255,255,255,0.82)]',
    skeleton: 'bg-slate-300/55 shadow-[inset_0_1px_0_rgba(255,255,255,0.78)]',
    accent: 'bg-[#2868d8]',
  },
]

const sidebarVariants: Array<{ id: SidebarVariant; labelKey: 'sidebar' | 'inset' | 'floating' }> = [
  { id: 'sidebar', labelKey: 'sidebar' },
  { id: 'inset', labelKey: 'inset' },
  { id: 'floating', labelKey: 'floating' },
]

export function ThemeTab() {
  const { theme, setTheme } = useTheme()
  const { t } = useI18n()
  const router = useRouter()
  const pathname = usePathname()
  const themeEditorBasePath = pathname?.startsWith('/settings-window') ? '/settings-window' : '/settings'
  const mounted = useSyncExternalStore(
    useCallback(() => () => {}, []),
    () => true,
    () => false,
  )
  const [contentTabsEnabled, setContentTabsEnabled] = useState(false)
  const [sidebarVariant, setSidebarVariant] = useState<SidebarVariant>('sidebar')
  const [customThemes, setCustomThemes] = useState<CustomThemeRecord[]>([])
  const [customLoading, setCustomLoading] = useState(true)
  const [customError, setCustomError] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<CustomThemeRecord | null>(null)
  const [deleting, setDeleting] = useState(false)

  const loadCustomThemes = useCallback(async () => {
    setCustomLoading(true)
    setCustomError(false)
    try {
      setCustomThemes(await listCustomThemes())
    } catch {
      setCustomError(true)
    } finally {
      setCustomLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    listCustomThemes()
      .then((items) => {
        if (!cancelled) setCustomThemes(items)
      })
      .catch(() => {
        if (!cancelled) setCustomError(true)
      })
      .finally(() => {
        if (!cancelled) setCustomLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    if (!mounted || customLoading || theme !== CUSTOM_THEME_ID) return
    if (getCachedCustomTheme()) return

    const activeId = getActiveCustomThemeId()
    const record = activeId ? customThemes.find((item) => item.id === activeId) : undefined
    if (record) {
      applyCustomTheme(record)
      return
    }

    setTheme('flame')
    void persistDesktopConfigItems({ theme: 'flame' })
  }, [mounted, customLoading, theme, customThemes, setTheme])

  useEffect(() => {
    if (typeof window === 'undefined') return
    return subscribeContentTabsEnabled(setContentTabsEnabled)
  }, [])

  useEffect(() => {
    if (typeof window === 'undefined') return
    return subscribeSidebarVariant(setSidebarVariant)
  }, [])

  useEffect(() => {
    if (!mounted || !theme) return
    void persistDesktopConfigItems({ theme })
  }, [mounted, theme])

  const handleToggleContentTabs = (checked: boolean) => {
    setContentTabsEnabled(checked)
    persistContentTabsEnabled(checked)
  }

  const handleThemeSelect = (themeId: string) => {
    setTheme(themeId)
    void persistDesktopConfigItems({ theme: themeId })
  }

  const handleSidebarVariantSelect = (variant: SidebarVariant) => {
    setSidebarVariant(variant)
    persistSidebarVariant(variant)
  }

  const activeCustomThemeId =
    mounted && theme === CUSTOM_THEME_ID ? getActiveCustomThemeId() : null

  const handleCustomThemeSelect = (record: CustomThemeRecord) => {
    applyCustomTheme(record)
    setTheme(CUSTOM_THEME_ID)
  }

  const handleDeleteTheme = async () => {
    const target = deleteTarget
    if (!target) return
    setDeleting(true)
    try {
      await deleteCustomTheme(target.id)
      toast.success(t.theme.customThemes.deleteSuccess)
      if (activeCustomThemeId === target.id) {
        clearCustomThemeCache()
        setTheme('flame')
        void persistDesktopConfigItems({ theme: 'flame' })
      }
      setDeleteTarget(null)
      await loadCustomThemes()
    } catch {
      toast.error(t.theme.customThemes.deleteFailed)
    } finally {
      setDeleting(false)
    }
  }

  const openThemeEditor = (record: CustomThemeRecord | null) => {
    const query = record ? `?themeId=${record.id}` : '?themeId=new'
    router.push(`${themeEditorBasePath}/theme${query}`)
  }

  const exportCustomThemes = async (records: CustomThemeRecord[]) => {
    if (records.length === 0) return
    try {
      const bundle = buildThemeLibraryBundle(
        records.map((record) => ({ name: record.name, definition: record.definition })),
      )
      const blob = new Blob([JSON.stringify(bundle, null, 2)], { type: 'application/json' })
      const date = new Date().toISOString().slice(0, 10)
      const result = await saveBlobAsFile(blob, `hephaestus-themes-${date}.json`)
      if (result === 'saved') {
        toast.success(t.theme.customThemes.exportSuccess)
      }
    } catch {
      toast.error(t.theme.customThemes.exportFailed)
    }
  }

  if (!mounted) {
    return (
      <div className="space-y-5">
        <div>
          <h3 className="text-lg font-semibold mb-2">{t.theme.title}</h3>
          <p className="text-sm text-muted-foreground">{t.theme.description}</p>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {themes.map((themeItem) => (
            <Card key={themeItem.id} className="p-3">
              <div className="h-16 rounded-md bg-muted animate-pulse" />
            </Card>
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <Card data-appearance-section="layout-tabs" className="p-4 border-border/80 bg-card/70">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <PanelTop className="w-4 h-4 text-primary" />
              <Label className="text-sm font-semibold">{t.theme.layoutTabs.title}</Label>
            </div>
            <p className="text-sm text-muted-foreground">{t.theme.layoutTabs.description}</p>
            <div data-layout-tabs-preview="shell" className="rounded-md border border-border/70 bg-muted/30 p-2.5">
              <div data-layout-tabs-preview="frame" className="flex h-14 rounded-md overflow-hidden border border-border/60 bg-background">
                <div className="w-14 border-r border-border/60 bg-muted/50" />
                <div className="flex-1 p-2">
                  <div className="flex items-center gap-1.5">
                    <div className={cn('h-6 rounded-md border px-3', contentTabsEnabled ? 'bg-background' : 'bg-muted/60')} />
                    <div className={cn('h-6 rounded-md border px-3', contentTabsEnabled ? 'bg-muted/60' : 'bg-background')} />
                    <div className={cn('h-6 rounded-md border px-3', contentTabsEnabled ? 'bg-muted/60' : 'bg-background')} />
                  </div>
                  <div className="mt-2 h-5 w-24 rounded bg-muted/60" />
                </div>
              </div>
            </div>
          </div>
          <div data-layout-tabs-toggle="true" className="flex items-center gap-3 rounded-lg border border-border/70 bg-muted/30 px-3 py-2">
            <Rows3 className="w-4 h-4 text-muted-foreground" />
            <Switch checked={contentTabsEnabled} onCheckedChange={handleToggleContentTabs} />
          </div>
        </div>
      </Card>

      <section data-appearance-section="sidebar-variant" className="space-y-3">
        <div className="flex items-center gap-2">
          <PanelLeft className="h-4 w-4 text-primary" />
          <div>
            <h3 className="text-sm font-semibold">{t.theme.sidebarStyle.title}</h3>
            <p className="text-sm text-muted-foreground">{t.theme.sidebarStyle.description}</p>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          {sidebarVariants.map(({ id, labelKey }) => {
            const isActive = sidebarVariant === id
            return (
              <button
                key={id}
                type="button"
                data-sidebar-variant-option={id}
                data-active={isActive ? 'true' : 'false'}
                aria-pressed={isActive}
                onClick={() => handleSidebarVariantSelect(id)}
                className={cn(
                  'rounded-xl border p-3 text-left transition-colors hover:border-primary/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                  isActive ? 'border-primary/60 bg-primary/5 ring-1 ring-primary/25' : 'border-border bg-card/70',
                )}
              >
                <div className="rounded-lg border border-border/70 bg-muted/30 p-2">
                  <div className={cn('flex h-14 gap-1.5', id !== 'sidebar' && 'p-1', id === 'inset' && 'rounded-md bg-background/65')}>
                    <div className={cn(
                      'w-10 shrink-0 bg-sidebar',
                      id === 'sidebar' && 'border-r border-border',
                      id === 'inset' && 'rounded-l-md border border-border',
                      id === 'floating' && 'rounded-md border border-border shadow-sm',
                    )} />
                    <div className={cn(
                      'flex-1 bg-background',
                      id === 'inset' && 'rounded-r-md border border-border',
                      id === 'floating' && 'rounded-md',
                    )} />
                  </div>
                </div>
                <div className="mt-3 flex items-center justify-between gap-2">
                  <span className="text-sm font-medium">{t.theme.sidebarStyle.options[labelKey].title}</span>
                  {isActive && <Check className="h-4 w-4 text-primary" />}
                </div>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t.theme.sidebarStyle.options[labelKey].description}
                </p>
              </button>
            )
          })}
        </div>
      </section>

      <section data-appearance-section="theme-switcher" className="space-y-3">
        <div className="flex items-center gap-2">
          <Palette className="h-4 w-4 text-primary" />
          <div>
            <h3 className="text-sm font-semibold">{t.theme.themeSwitcher.title}</h3>
            <p className="text-sm text-muted-foreground">{t.theme.themeSwitcher.description}</p>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-3">
          {themes.map((themeItem) => {
            const Icon = themeItem.icon
            const isActive = theme === themeItem.id

            return (
              <Card
                key={themeItem.id}
                data-theme-option="true"
                data-active={isActive ? 'true' : 'false'}
                className={cn(
                  'p-3 cursor-pointer transition-all hover:border-primary/60',
                  isActive && 'border-primary/60 ring-1 ring-primary/30'
                )}
                onClick={() => handleThemeSelect(themeItem.id)}
              >
                <div className="space-y-2.5">
                  {/* 主题预览 - 骨架图样式 */}
                  <div className={cn('h-16 rounded-md relative overflow-hidden p-1.5', themeItem.bg)}>
                    <div className="flex h-full gap-2">
                      {/* 侧边栏 */}
                      <div className="w-8 flex flex-col gap-1.5">
                        <div className={cn('h-2.5 w-2.5 rounded-sm', themeItem.accent)} />
                        <div className={cn('h-1.5 w-full rounded-sm', themeItem.skeleton)} />
                        <div className={cn('h-1.5 w-full rounded-sm', themeItem.skeleton)} />
                        <div className={cn('h-1.5 w-5 rounded-sm', themeItem.skeleton)} />
                      </div>
                      {/* 主内容区 */}
                      <div className="flex-1 flex flex-col gap-1">
                        <div className={cn('h-2.5 w-14 rounded-sm', themeItem.skeleton)} />
                        <div className={cn('h-1.5 w-full rounded-sm', themeItem.skeleton)} />
                        <div className={cn('h-1.5 w-3/4 rounded-sm', themeItem.skeleton)} />
                        <div className={cn('h-1.5 w-4/5 rounded-sm', themeItem.skeleton)} />
                        <div className={cn('h-1.5 w-2/3 rounded-sm', themeItem.skeleton)} />
                        <div className="flex gap-1 mt-auto">
                          <div className={cn('h-2.5 w-7 rounded-sm', themeItem.accent)} />
                          <div className={cn('h-2.5 w-7 rounded-sm', themeItem.skeleton)} />
                        </div>
                      </div>
                    </div>
                    {isActive && (
                      <div className="absolute top-2 right-2">
                        <div data-theme-current-indicator="true" className="w-5 h-5 rounded-full bg-primary flex items-center justify-center">
                          <Check className="w-3 h-3 text-primary-foreground" />
                        </div>
                      </div>
                    )}
                  </div>

                  {/* 主题信息 */}
                  <div className="space-y-0.5">
                    <div className="flex items-center gap-2">
                      <Icon className="w-4 h-4 text-muted-foreground" />
                      <h4 className="text-sm font-semibold">{t.theme.modes[themeItem.nameKey]}</h4>
                    </div>
                  </div>

                  {/* 选择按钮 */}
                  <Button
                    variant="outline"
                    size="sm"
                    data-current-theme-button={isActive ? 'true' : 'false'}
                    className={cn(
                      'w-full h-8',
                      isActive && 'border-primary/40 bg-primary/10 text-primary hover:bg-primary/15'
                    )}
                    onClick={(e) => {
                      e.stopPropagation()
                      handleThemeSelect(themeItem.id)
                    }}
                  >
                    {isActive ? t.language.current : t.language.select}
                  </Button>
                </div>
              </Card>
            )
          })}
        </div>
      </section>

      <section data-appearance-section="custom-themes" className="space-y-3">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div className="flex items-center gap-2">
            <Palette className="h-4 w-4 text-primary" />
            <div>
              <h3 className="text-sm font-semibold">{t.theme.customThemes.title}</h3>
              <p className="text-sm text-muted-foreground">{t.theme.customThemes.description}</p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => setImportOpen(true)}>
              <Upload className="h-4 w-4" />
              {t.theme.customThemes.import}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={customThemes.length === 0}
              onClick={() => void exportCustomThemes(customThemes)}
            >
              <Download className="h-4 w-4" />
              {t.theme.customThemes.exportAll}
            </Button>
            <Button
              size="sm"
              onClick={() => openThemeEditor(null)}
            >
              <Plus className="h-4 w-4" />
              {t.theme.customThemes.create}
            </Button>
          </div>
        </div>

        {customLoading ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
            {[0, 1, 2].map((item) => (
              <Card key={item} className="p-3">
                <div className="h-20 animate-pulse rounded-lg bg-muted" />
                <div className="mt-2 h-4 w-1/2 animate-pulse rounded bg-muted" />
              </Card>
            ))}
          </div>
        ) : customError ? (
          <Card className="flex flex-col items-center justify-center gap-2 border-dashed p-6 text-center">
            <p className="text-sm text-muted-foreground">{t.theme.customThemes.loadFailed}</p>
            <Button variant="outline" size="sm" onClick={() => void loadCustomThemes()}>
              <RefreshCw className="h-4 w-4" />
              {t.theme.customThemes.retry}
            </Button>
          </Card>
        ) : customThemes.length === 0 ? (
          <Card className="flex flex-col items-center justify-center gap-3 border-dashed p-8 text-center">
            <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
              <Palette className="h-5 w-5 text-primary" />
            </div>
            <div>
              <p className="text-sm font-medium">{t.theme.customThemes.empty}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t.theme.customThemes.emptyHint}</p>
            </div>
            <Button
              size="sm"
              onClick={() => openThemeEditor(null)}
            >
              <Plus className="h-4 w-4" />
              {t.theme.customThemes.create}
            </Button>
          </Card>
        ) : (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3">
            {customThemes.map((record) => {
              const isActive = theme === CUSTOM_THEME_ID && activeCustomThemeId === record.id
              return (
                <Card
                  key={record.id}
                  data-custom-theme-option="true"
                  data-active={isActive ? 'true' : 'false'}
                  className={cn(
                    'p-3 transition-all hover:border-primary/60',
                    isActive && 'border-primary/60 ring-1 ring-primary/30',
                  )}
                >
                  <ThemePreview definition={record.definition} variant="compact" />
                  <div className="mt-2 flex items-start gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-1.5">
                        <h4 className="truncate text-sm font-semibold">{record.name}</h4>
                        {isActive && (
                          <Badge variant="secondary" className="shrink-0">
                            {t.theme.customThemes.active}
                          </Badge>
                        )}
                      </div>
                      <p className="mt-0.5 text-xs text-muted-foreground">
                        {record.definition.base === 'dark'
                          ? t.theme.customThemes.editor.dark
                          : t.theme.customThemes.editor.light}
                      </p>
                    </div>
                  </div>
                  <div className="mt-2 flex items-center gap-1">
                    <Button
                      size="sm"
                      variant={isActive ? 'outline' : 'default'}
                      className="min-w-0 flex-1"
                      onClick={() => handleCustomThemeSelect(record)}
                    >
                      {isActive && <Check className="h-3.5 w-3.5" />}
                      <span className="truncate">
                        {isActive ? t.theme.customThemes.active : t.theme.customThemes.apply}
                      </span>
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="outline"
                      title={t.theme.customThemes.edit}
                      aria-label={t.theme.customThemes.edit}
                      onClick={() => openThemeEditor(record)}
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="outline"
                      title={t.theme.customThemes.export}
                      aria-label={t.theme.customThemes.export}
                      onClick={() => void exportCustomThemes([record])}
                    >
                      <Download className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      title={t.theme.customThemes.delete}
                      aria-label={t.theme.customThemes.delete}
                      onClick={() => setDeleteTarget(record)}
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </Card>
              )
            })}
          </div>
        )}
      </section>

      <CustomThemeImportDialog
        key={importOpen ? 'open' : 'closed'}
        open={importOpen}
        onOpenChange={setImportOpen}
        existingNames={customThemes.map((item) => item.name)}
        onImported={() => void loadCustomThemes()}
      />

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.theme.customThemes.deleteTitle}</AlertDialogTitle>
            <AlertDialogDescription>{t.theme.customThemes.deleteDescription}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>{t.common.cancel}</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
              disabled={deleting}
              onClick={(event) => {
                event.preventDefault()
                void handleDeleteTheme()
              }}
            >
              {deleting && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              {t.common.delete}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
