'use client'

import { persistDesktopConfigItems } from '@/lib/api'

export const LAYOUT_TABS_STORAGE_KEY = 'hephaestus:appearance:content-tabs'
export const APPEARANCE_TABS_EVENT = 'hephaestus:appearance-tabs-changed'
export const SIDEBAR_VARIANT_STORAGE_KEY = 'hephaestus:appearance:sidebar-variant'
export const APPEARANCE_SIDEBAR_VARIANT_EVENT = 'hephaestus:appearance:sidebar-variant-changed'

export type SidebarVariant = 'sidebar' | 'inset' | 'floating'

const isSidebarVariant = (value: string | null): value is SidebarVariant =>
  value === 'sidebar' || value === 'inset' || value === 'floating'

export function getContentTabsEnabled() {
  if (typeof window === 'undefined') {
    return false
  }
  return window.localStorage.getItem(LAYOUT_TABS_STORAGE_KEY) === 'true'
}

export function setContentTabsEnabled(enabled: boolean) {
  if (typeof window === 'undefined') {
    return
  }

  window.localStorage.setItem(LAYOUT_TABS_STORAGE_KEY, String(enabled))
  void persistDesktopConfigItems({ [LAYOUT_TABS_STORAGE_KEY]: String(enabled) })
  window.dispatchEvent(new CustomEvent(APPEARANCE_TABS_EVENT, { detail: { enabled } }))
}

export function subscribeContentTabsEnabled(callback: (enabled: boolean) => void) {
  if (typeof window === 'undefined') {
    return () => {}
  }

  const sync = () => {
    callback(getContentTabsEnabled())
  }

  const handleStorage = (event: StorageEvent) => {
    if (!event.key || event.key === LAYOUT_TABS_STORAGE_KEY) {
      sync()
    }
  }

  const handleDesktopStorageUpdated = (event: Event) => {
    const detail = (event as CustomEvent<{ items?: Record<string, string>; keys?: string[] }>).detail
    if (
      !detail ||
      detail.items?.[LAYOUT_TABS_STORAGE_KEY] !== undefined ||
      detail.keys?.includes(LAYOUT_TABS_STORAGE_KEY)
    ) {
      sync()
    }
  }

  sync()
  window.addEventListener(APPEARANCE_TABS_EVENT, sync as EventListener)
  window.addEventListener('storage', handleStorage)
  window.addEventListener('hephaestus:desktop-storage-restored', sync as EventListener)
  window.addEventListener('hephaestus:desktop-storage-updated', handleDesktopStorageUpdated as EventListener)

  return () => {
    window.removeEventListener(APPEARANCE_TABS_EVENT, sync as EventListener)
    window.removeEventListener('storage', handleStorage)
    window.removeEventListener('hephaestus:desktop-storage-restored', sync as EventListener)
    window.removeEventListener('hephaestus:desktop-storage-updated', handleDesktopStorageUpdated as EventListener)
  }
}

export function toggleContentTabsEnabled() {
  const nextEnabled = !getContentTabsEnabled()
  setContentTabsEnabled(nextEnabled)
  return nextEnabled
}

export function getSidebarVariant(): SidebarVariant {
  if (typeof window === 'undefined') {
    return 'sidebar'
  }

  const value = window.localStorage.getItem(SIDEBAR_VARIANT_STORAGE_KEY)
  return isSidebarVariant(value) ? value : 'sidebar'
}

export function setSidebarVariant(variant: SidebarVariant) {
  if (typeof window === 'undefined') {
    return
  }

  window.localStorage.setItem(SIDEBAR_VARIANT_STORAGE_KEY, variant)
  void persistDesktopConfigItems({ [SIDEBAR_VARIANT_STORAGE_KEY]: variant })
  window.dispatchEvent(
    new CustomEvent(APPEARANCE_SIDEBAR_VARIANT_EVENT, { detail: { variant } }),
  )
}

export function subscribeSidebarVariant(callback: (variant: SidebarVariant) => void) {
  if (typeof window === 'undefined') {
    return () => {}
  }

  const sync = () => {
    callback(getSidebarVariant())
  }

  const handleStorage = (event: StorageEvent) => {
    if (!event.key || event.key === SIDEBAR_VARIANT_STORAGE_KEY) {
      sync()
    }
  }

  const handleDesktopStorageUpdated = (event: Event) => {
    const detail = (event as CustomEvent<{ items?: Record<string, string>; keys?: string[] }>).detail
    if (
      !detail ||
      detail.items?.[SIDEBAR_VARIANT_STORAGE_KEY] !== undefined ||
      detail.keys?.includes(SIDEBAR_VARIANT_STORAGE_KEY)
    ) {
      sync()
    }
  }

  sync()
  window.addEventListener(APPEARANCE_SIDEBAR_VARIANT_EVENT, sync as EventListener)
  window.addEventListener('storage', handleStorage)
  window.addEventListener('hephaestus:desktop-storage-restored', sync as EventListener)
  window.addEventListener('hephaestus:desktop-storage-updated', handleDesktopStorageUpdated as EventListener)

  return () => {
    window.removeEventListener(APPEARANCE_SIDEBAR_VARIANT_EVENT, sync as EventListener)
    window.removeEventListener('storage', handleStorage)
    window.removeEventListener('hephaestus:desktop-storage-restored', sync as EventListener)
    window.removeEventListener('hephaestus:desktop-storage-updated', handleDesktopStorageUpdated as EventListener)
  }
}
