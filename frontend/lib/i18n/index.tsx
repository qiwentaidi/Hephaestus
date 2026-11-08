'use client'

import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  useSyncExternalStore,
  ReactNode,
} from 'react'
import { persistDesktopConfigItems } from '@/lib/api'
import { zh, type Locale } from './locales/zh'
import { en } from './locales/en'

export type Language = 'zh' | 'en'

const locales: Record<Language, Locale> = {
  zh,
  en,
}

interface I18nContextType {
  language: Language
  setLanguage: (lang: Language) => void
  t: Locale
}

const I18nContext = createContext<I18nContextType | undefined>(undefined)

const STORAGE_KEY = 'hephaestus_language'

function isLanguage(value: string | null): value is Language {
  return value === 'zh' || value === 'en'
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const mounted = useSyncExternalStore(
    useCallback(() => () => {}, []),
    () => true,
    () => false,
  )
  const [language, setLanguageState] = useState<Language>(() => {
    if (typeof window === 'undefined') {
      return 'zh'
    }
    const stored = window.localStorage.getItem(STORAGE_KEY)
    return isLanguage(stored) ? stored : 'zh'
  })

  useEffect(() => {
    const syncLanguageFromStorage = (value = localStorage.getItem(STORAGE_KEY)) => {
      if (isLanguage(value)) {
        setLanguageState(value)
      }
    }

    const handleStorage = (event: StorageEvent) => {
      if (event.key === null || event.key === STORAGE_KEY) {
        syncLanguageFromStorage(event.newValue)
      }
    }

    const handleDesktopStorageUpdated = (event: Event) => {
      const items = (event as CustomEvent<{ items?: Record<string, string> }>).detail?.items
      syncLanguageFromStorage(items?.[STORAGE_KEY] ?? localStorage.getItem(STORAGE_KEY))
    }

    window.addEventListener('storage', handleStorage)
    window.addEventListener('hephaestus:desktop-storage-restored', handleDesktopStorageUpdated)
    window.addEventListener('hephaestus:desktop-storage-updated', handleDesktopStorageUpdated)
    return () => {
      window.removeEventListener('storage', handleStorage)
      window.removeEventListener('hephaestus:desktop-storage-restored', handleDesktopStorageUpdated)
      window.removeEventListener('hephaestus:desktop-storage-updated', handleDesktopStorageUpdated)
    }
  }, [])

  // 更新 html lang 属性
  useEffect(() => {
    if (mounted) {
      document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en'
    }
  }, [language, mounted])

  const setLanguage = useCallback((lang: Language) => {
    setLanguageState(lang)
    localStorage.setItem(STORAGE_KEY, lang)
    void persistDesktopConfigItems({ [STORAGE_KEY]: lang })
  }, [])

  const t = locales[language]

  // 避免服务端渲染时的 hydration 问题
  if (!mounted) {
    return (
      <I18nContext.Provider value={{ language: 'zh', setLanguage, t: zh }}>
        {children}
      </I18nContext.Provider>
    )
  }

  return (
    <I18nContext.Provider value={{ language, setLanguage, t }}>
      {children}
    </I18nContext.Provider>
  )
}

export function useI18n() {
  const context = useContext(I18nContext)
  if (context === undefined) {
    throw new Error('useI18n must be used within an I18nProvider')
  }
  return context
}

// 导出类型
export type { Locale }
