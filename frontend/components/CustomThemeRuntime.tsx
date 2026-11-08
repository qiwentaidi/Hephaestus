'use client'

import { useEffect } from 'react'
import {
  CUSTOM_THEME_CACHE_KEY,
  CUSTOM_THEME_CSS_KEY,
  CUSTOM_THEME_EVENT,
  applyCachedCustomTheme,
} from '@/lib/theme/custom-theme'

/**
 * Keeps the injected custom-theme stylesheet in sync across tabs/windows.
 * The actual <html class="custom"> switch is owned by next-themes.
 */
export function CustomThemeRuntime() {
  useEffect(() => {
    applyCachedCustomTheme()

    const onStorage = (event: StorageEvent) => {
      if (
        !event.key ||
        event.key === CUSTOM_THEME_CACHE_KEY ||
        event.key === CUSTOM_THEME_CSS_KEY
      ) {
        applyCachedCustomTheme()
      }
    }

    const onChange = () => applyCachedCustomTheme()

    const onDesktopStorage = (event: Event) => {
      const detail = (event as CustomEvent<{ items?: Record<string, string>; keys?: string[] }>).detail
      if (!detail) return
      if (
        detail.items?.[CUSTOM_THEME_CACHE_KEY] !== undefined ||
        detail.items?.[CUSTOM_THEME_CSS_KEY] !== undefined
      ) {
        applyCachedCustomTheme()
        return
      }
      if (
        detail.keys?.some((key) => key === CUSTOM_THEME_CACHE_KEY || key === CUSTOM_THEME_CSS_KEY)
      ) {
        applyCachedCustomTheme()
      }
    }

    window.addEventListener('storage', onStorage)
    window.addEventListener(CUSTOM_THEME_EVENT, onChange)
    window.addEventListener('hephaestus:desktop-storage-updated', onDesktopStorage)

    return () => {
      window.removeEventListener('storage', onStorage)
      window.removeEventListener(CUSTOM_THEME_EVENT, onChange)
      window.removeEventListener('hephaestus:desktop-storage-updated', onDesktopStorage)
    }
  }, [])

  return null
}
