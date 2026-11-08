'use client'

import { useEffect } from 'react'
import { persistDesktopConfigItems } from '@/lib/api'
import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from '@/lib/env'
import {
  ACTIVE_CUSTOM_THEME_ID_KEY,
  CUSTOM_THEME_CACHE_KEY,
  applyCachedCustomTheme,
} from '@/lib/theme/custom-theme'

const DESKTOP_STORAGE_KEYS = [
  'theme',
  'hephaestus_language',
  'auth_token',
  'userData',
  'hephaestus:appearance:content-tabs',
  CUSTOM_THEME_CACHE_KEY,
  ACTIVE_CUSTOM_THEME_ID_KEY,
  'proxyConfig',
  'networkConfig',
]

const APP_THEMES = new Set([
  'flame',
  'dark',
  'light',
  'shadcn-light',
  'shadcn-dark',
  'paper-ink',
  'graphite',
  'glass',
  'custom',
])

const getDesktopEndpoint = (path: string) => `${getApiBaseURL()}${path}`

async function isDesktopMode() {
  if (!hasDesktopApiToken()) {
    return false
  }
  try {
    const response = await fetch(getDesktopEndpoint('/api/v1/desktop/meta'), {
      method: 'GET',
      cache: 'no-store',
      headers: getDesktopApiHeaders(),
    })
    if (!response.ok) {
      return false
    }
    const payload = await response.json().catch(() => null)
    return Boolean(payload?.success && payload?.data?.isDesktop)
  } catch {
    return false
  }
}

async function loadDesktopStorage(): Promise<Record<string, string>> {
  const response = await fetch(getDesktopEndpoint('/api/v1/desktop/storage'), {
    method: 'GET',
    cache: 'no-store',
    headers: getDesktopApiHeaders(),
  })
  if (!response.ok) {
    return {}
  }
  const payload = await response.json().catch(() => null)
  return payload?.data?.items && typeof payload.data.items === 'object' ? payload.data.items : {}
}

function applyThemeFromStorage(theme: string) {
  if (!APP_THEMES.has(theme)) {
    return
  }
  const root = document.documentElement
  root.classList.remove('cobalt')
  for (const candidate of APP_THEMES) {
    root.classList.remove(candidate)
  }
  root.classList.add(theme)
}

export function DesktopStorageBridge() {
  useEffect(() => {
    let cancelled = false

    const bootstrap = async () => {
      if (!(await isDesktopMode()) || cancelled) {
        return
      }

      const items = await loadDesktopStorage()
      if (cancelled) {
        return
      }

      for (const key of DESKTOP_STORAGE_KEYS) {
        const value = key === 'theme' && items[key] === 'cobalt' ? 'dark' : items[key]
        if (typeof value !== 'string' || value === '') {
          continue
        }
        window.localStorage.setItem(key, value)
        if (key === 'theme') {
          applyThemeFromStorage(value)
        }
        window.dispatchEvent(new StorageEvent('storage', { key, newValue: value }))
      }

      if (items.theme === 'cobalt') {
        void persistDesktopConfigItems({ theme: 'dark' })
      }

      applyCachedCustomTheme()

      window.dispatchEvent(new Event('storage'))
      window.dispatchEvent(new CustomEvent('hephaestus:desktop-storage-restored'))
    }

    void bootstrap()
    return () => {
      cancelled = true
    }
  }, [])

  return null
}
