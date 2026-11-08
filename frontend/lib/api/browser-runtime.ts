import { fetchWithAuth } from '@/lib/api'

interface BrowserRuntimeInfo {
  os: string
  isLinux: boolean
  chromePath: string
  snapChromeDetected: boolean
}

interface BrowserRuntimeResponse {
  success?: boolean
  data?: BrowserRuntimeInfo
}

let browserRuntimePromise: Promise<BrowserRuntimeInfo | null> | null = null

export function getBrowserRuntimeInfo(): Promise<BrowserRuntimeInfo | null> {
  if (!browserRuntimePromise) {
    browserRuntimePromise = fetchWithAuth('/api/v1/system/browser-runtime', { cache: 'no-store' })
      .then(async (response) => {
        if (!response.ok) {
          return null
        }
        const payload = (await response.json()) as BrowserRuntimeResponse
        return payload.data ?? null
      })
      .catch(() => null)
  }

  return browserRuntimePromise
}
