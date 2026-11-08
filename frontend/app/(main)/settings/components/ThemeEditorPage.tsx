'use client'

import { Suspense, useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { useTheme } from 'next-themes'
import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SettingsWindowTitlebar } from '@/components/SettingsWindowTitlebar'
import { useI18n } from '@/lib/i18n'
import { listCustomThemes } from '@/lib/api/themes'
import {
  applyCustomTheme,
  getCachedCustomTheme,
  CUSTOM_THEME_ID,
  type CustomThemeRecord,
} from '@/lib/theme/custom-theme'
import { cn } from '@/lib/utils'
import { CustomThemeEditor } from './CustomThemeEditor'

interface ThemeEditorPageProps {
  returnPath: string
  withTitlebar?: boolean
}

function ThemeEditorPageInner({ returnPath, withTitlebar = false }: ThemeEditorPageProps) {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { setTheme } = useTheme()
  const { t } = useI18n()
  const copy = t.theme.customThemes.editor
  const rawThemeId = searchParams?.get('themeId') ?? 'new'
  const isNew = rawThemeId === 'new'
  const themeId = isNew ? null : Number.parseInt(rawThemeId, 10)
  const validThemeId = themeId !== null && Number.isFinite(themeId) && themeId > 0

  const [loading, setLoading] = useState(true)
  const [themes, setThemes] = useState<CustomThemeRecord[]>([])
  const [theme, setThemeRecord] = useState<CustomThemeRecord | null>(null)
  const [missing, setMissing] = useState(false)
  const [loadError, setLoadError] = useState(false)

  useEffect(() => {
    let cancelled = false

    listCustomThemes()
      .then((items) => {
        if (cancelled) return
        setThemes(items)
        if (isNew) {
          setThemeRecord(null)
          setMissing(false)
          return
        }
        if (!validThemeId) {
          setThemeRecord(null)
          setMissing(true)
          return
        }
        const found = items.find((item) => item.id === themeId) ?? null
        setThemeRecord(found)
        setMissing(!found)
      })
      .catch(() => {
        if (!cancelled) setLoadError(true)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [isNew, themeId, validThemeId])

  const backToLibrary = () => {
    router.push(`${returnPath}?tabs=appearance`)
  }

  const handleSaved = (record: CustomThemeRecord, apply: boolean) => {
    if (apply) {
      applyCustomTheme(record)
      setTheme(CUSTOM_THEME_ID)
    } else if (getCachedCustomTheme()?.id === record.id) {
      applyCustomTheme(record)
    }
    backToLibrary()
  }

  const body = loading ? (
    <div className="flex flex-1 items-center justify-center gap-2 text-sm text-muted-foreground" role="status">
      <Loader2 className="h-4 w-4 animate-spin" />
      {t.common.loading}
    </div>
  ) : loadError ? (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
      <p className="text-sm text-muted-foreground">{t.theme.customThemes.loadFailed}</p>
      <Button type="button" variant="outline" size="sm" onClick={backToLibrary}>
        {copy.backToThemes}
      </Button>
    </div>
  ) : missing ? (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 text-center">
      <h2 className="text-base font-semibold">{copy.notFound}</h2>
      <p className="max-w-md text-sm text-muted-foreground">{copy.notFoundHint}</p>
      <Button type="button" variant="outline" size="sm" onClick={backToLibrary}>
        {copy.backToThemes}
      </Button>
    </div>
  ) : (
    <CustomThemeEditor
      theme={theme}
      existingNames={themes.filter((item) => item.id !== theme?.id).map((item) => item.name)}
      draftKey={`hephaestus:theme-draft:${isNew ? 'new' : theme?.id ?? rawThemeId}`}
      onSaved={handleSaved}
      onBack={backToLibrary}
    />
  )

  return (
    <div
      className={cn(
        'flex min-h-0 flex-col bg-background',
        withTitlebar ? 'h-dvh' : 'h-full',
      )}
    >
      {withTitlebar && <SettingsWindowTitlebar />}
      <div className="flex min-h-0 flex-1 flex-col">{body}</div>
    </div>
  )
}

export function ThemeEditorPage(props: ThemeEditorPageProps) {
  return (
    <Suspense
      fallback={
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground" role="status">
          <Loader2 className="h-4 w-4 animate-spin" />
        </div>
      }
    >
      <ThemeEditorPageInner {...props} />
    </Suspense>
  )
}
