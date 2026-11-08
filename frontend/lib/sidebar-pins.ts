const PINS_EVENT = 'hephaestus:sidebar-pins-changed'
const storageKey = (username: string) => `hephaestus:sidebar-pins:${encodeURIComponent(username)}`

export const emptySidebarPinsSnapshot = () => '[]'

export function readSidebarPinsSnapshot(username: string): string {
  if (!username || typeof window === 'undefined') return '[]'
  try {
    return window.localStorage.getItem(storageKey(username)) || '[]'
  } catch {
    return '[]'
  }
}

export function parseSidebarPins(snapshot: string): string[] {
  try {
    const value: unknown = JSON.parse(snapshot)
    if (!Array.isArray(value)) return []
    return [...new Set(value.filter((item): item is string => typeof item === 'string' && item.startsWith('/')))]
  } catch {
    return []
  }
}

export function toggleSidebarPin(username: string, href: string): boolean {
  if (!username || typeof window === 'undefined') return false
  const pins = parseSidebarPins(readSidebarPinsSnapshot(username))
  const next = pins.includes(href) ? pins.filter((pin) => pin !== href) : [...pins, href]
  try {
    window.localStorage.setItem(storageKey(username), JSON.stringify(next))
  } catch {
    return false
  }
  window.dispatchEvent(new Event(PINS_EVENT))
  return true
}

export function subscribeSidebarPins(callback: () => void): () => void {
  const handleStorage = (event: StorageEvent) => {
    if (!event.key || event.key.startsWith('hephaestus:sidebar-pins:')) callback()
  }
  window.addEventListener(PINS_EVENT, callback)
  window.addEventListener('storage', handleStorage)
  return () => {
    window.removeEventListener(PINS_EVENT, callback)
    window.removeEventListener('storage', handleStorage)
  }
}
