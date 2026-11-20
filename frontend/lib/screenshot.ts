import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from './env'

export type AppScreenshot = {
  dataUrl: string
  /** Logical (CSS pixel) width of the captured window. */
  width: number
  /** Logical (CSS pixel) height of the captured window. */
  height: number
  /** Raster scale used for the PNG (1 = one image pixel per CSS pixel). */
  scale: number
  capturedAt: number
  /** Capture backend that produced this image. */
  source: 'native' | 'dom'
}

export type ScreenshotCaptureOptions = {
  /** Desired raster scale, clamped to 1..2. Defaults to the device pixel ratio. */
  scale?: number
}

const SKIP_ELEMENTS = [
  '[data-screenshot-ignore="true"]',
  '[data-rht-toaster]',
  '[data-sonner-toaster]',
  '[data-radix-popper-content-wrapper]',
].join(',')

function clampScale(value: number | undefined): number {
  const deviceScale =
    typeof window !== 'undefined' && Number.isFinite(window.devicePixelRatio) && window.devicePixelRatio > 0
      ? window.devicePixelRatio
      : 1
  const requested = value ?? Math.min(deviceScale, 2)
  if (!Number.isFinite(requested) || requested <= 0) {
    return 1
  }
  return Math.min(2, Math.max(1, requested))
}

/**
 * Capture the current desktop client window as a PNG data URL.
 *
 * html2canvas-pro is loaded lazily so the screenshot entry point does not add to
 * the initial bundle. It renders the live DOM (including scrolled containers) to
 * a canvas without requiring any OS screen-recording permission.
 */
export async function captureAppWindow(options: ScreenshotCaptureOptions = {}): Promise<AppScreenshot> {
  if (typeof window === 'undefined' || typeof document === 'undefined' || !document.body) {
    throw new Error('screenshot is only available in the browser')
  }

  const width = Math.max(1, Math.floor(window.innerWidth))
  const height = Math.max(1, Math.floor(window.innerHeight))
  const scale = clampScale(options.scale)

  const { default: html2canvas } = await import('html2canvas-pro')
  const canvas = await html2canvas(document.body, {
    width,
    height,
    windowWidth: width,
    windowHeight: height,
    scale,
    logging: false,
    useCORS: true,
    backgroundColor: null,
    imageSmoothingQuality: 'high',
    ignoreElements: (element) => {
      try {
        return element.matches(SKIP_ELEMENTS)
      } catch {
        return false
      }
    },
  })

  return {
    dataUrl: canvas.toDataURL('image/png'),
    width,
    height,
    scale,
    capturedAt: Date.now(),
    source: 'dom',
  }
}

/**
 * Capture the main desktop window through the native host (WKWebView snapshot
 * on macOS, PrintWindow on Windows). Returns null when native capture is
 * unavailable, so callers can fall back to DOM capture.
 */
export async function captureNativeAppWindow(options: { scale?: number } = {}): Promise<AppScreenshot | null> {
  if (typeof window === 'undefined' || !hasDesktopApiToken()) {
    return null
  }

  try {
    const response = await fetch(`${getApiBaseURL()}/api/v1/desktop/screenshot/capture`, {
      method: 'POST',
      cache: 'no-store',
      headers: {
        'Content-Type': 'application/json',
        ...getDesktopApiHeaders(),
      },
      body: JSON.stringify({ scale: options.scale ?? 2 }),
    })
    if (!response.ok) {
      const failure = await response.json().catch(() => null) as { error?: string } | null
      console.warn('[screenshot] native capture unavailable', response.status, failure?.error || '')
      return null
    }
    const payload = await response.json().catch(() => null) as
      | { success?: boolean; data?: { dataUrl?: string; width?: number; height?: number; scale?: number; capturedAt?: string } }
      | null
    const data = payload?.data
    if (!payload?.success || !data || typeof data.dataUrl !== 'string') {
      return null
    }
    return {
      dataUrl: data.dataUrl,
      width: Number(data.width) || 0,
      height: Number(data.height) || 0,
      scale: Number(data.scale) || 1,
      capturedAt: Date.parse(String(data.capturedAt || '')) || Date.now(),
      source: 'native',
    }
  } catch {
    return null
  }
}

export async function screenshotBlob(screenshot: AppScreenshot): Promise<Blob> {
  const response = await fetch(screenshot.dataUrl)
  return response.blob()
}

export function screenshotFileName(date = new Date()): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `Hephaestus-${date.getFullYear()}${pad(date.getMonth() + 1)}${pad(date.getDate())}-${pad(
    date.getHours()
  )}${pad(date.getMinutes())}${pad(date.getSeconds())}.png`
}

export type ScreenshotCompletionPayload = {
  requestId: string
  dataUrl?: string
  width?: number
  height?: number
  route?: string
  title?: string
  error?: string
}

/**
 * Report an MCP-triggered capture back to the desktop backend. The backend
 * correlates the payload with the pending MCP tool call and returns the image
 * to the MCP client.
 */
export async function reportScreenshotCompletion(payload: ScreenshotCompletionPayload): Promise<void> {
  const response = await fetch(`${getApiBaseURL()}/api/v1/desktop/screenshot/complete`, {
    method: 'POST',
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...getDesktopApiHeaders(),
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    throw new Error(`screenshot completion failed: ${response.status}`)
  }
}
