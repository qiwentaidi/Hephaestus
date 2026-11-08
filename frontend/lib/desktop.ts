import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from './env'

const desktopEndpoint = (path: string) => `${getApiBaseURL()}${path}`
const desktopExternalSchemes = new Set(['http:', 'https:', 'mailto:', 'tel:'])

export type DesktopRuntimeMeta = {
	isDesktop: boolean
	platform?: string
	isMaximised?: boolean
	isFullscreen?: boolean
	serverURL?: string
	mcpServerURL?: string
	mcpListenHost?: string
	mcpListenPort?: string
	backendPort?: string
	preferredBackendPort?: string
	isTemporaryBackendPort?: boolean
	backendPortFallbackError?: string
	mcpAccessScope?: 'local' | string
}

export type DesktopWindowState = {
	isMaximised?: boolean
	isFullscreen?: boolean
}

type DesktopApiPayload<T> = {
	success?: boolean
	data?: T
	error?: string
}

function parseDesktopNavigationTarget(input: string | URL): URL | null {
	try {
		if (input instanceof URL) {
			return input
		}
		if (typeof window !== 'undefined') {
			return new URL(input, window.location.href)
		}
		return new URL(input)
	} catch {
		return null
	}
}

async function requestDesktopAPI<T>(path: string, method: 'GET' | 'POST' = 'POST'): Promise<T | null> {
	if (!hasDesktopApiToken()) {
		return null
	}

	try {
		const response = await fetch(desktopEndpoint(path), {
			method,
			cache: 'no-store',
			headers: {
				'Content-Type': 'application/json',
				...getDesktopApiHeaders(),
			},
		})
		if (!response.ok) {
			return null
		}
		const payload = await response.json().catch(() => null) as DesktopApiPayload<T> | null
		if (!payload || payload.success === false) {
			return null
		}
		return payload.data ?? null
	} catch {
		return null
	}
}

export async function getDesktopRuntimeMeta(): Promise<DesktopRuntimeMeta | null> {
	if (!hasDesktopApiToken()) {
		return null
	}
	try {
		const response = await fetch(desktopEndpoint('/api/v1/desktop/meta'), {
			method: "GET",
			cache: "no-store",
			headers: getDesktopApiHeaders(),
		});
		if (!response.ok) {
			return null;
		}
		const payload = await response.json().catch(() => null);
		if (!payload?.success || !payload?.data?.isDesktop) {
			return null
		}
		return payload.data as DesktopRuntimeMeta
	} catch {
		return null;
	}
}

export async function isDesktopRuntime(): Promise<boolean> {
	const meta = await getDesktopRuntimeMeta()
	return Boolean(meta?.isDesktop)
}

export async function getDesktopWindowState(): Promise<DesktopWindowState | null> {
	return requestDesktopAPI<DesktopWindowState>('/api/v1/desktop/window/state', 'GET')
}

export async function toggleDesktopWindowFullscreen(): Promise<DesktopWindowState | null> {
	return requestDesktopAPI<DesktopWindowState>('/api/v1/desktop/window/toggle-fullscreen')
}

export function shouldUseDesktopExternalNavigation(input?: string | URL, target?: string): boolean {
	if (!input) {
		return false
	}

	const parsed = parseDesktopNavigationTarget(input)
	if (!parsed || !desktopExternalSchemes.has(parsed.protocol)) {
		return false
	}

	if (parsed.protocol === 'mailto:' || parsed.protocol === 'tel:') {
		return true
	}

	if (typeof window === 'undefined') {
		return true
	}

	if (parsed.origin !== window.location.origin) {
		return true
	}

	return target === '_blank'
}

export async function openDesktopExternalURL(input: string | URL): Promise<boolean> {
	if (!hasDesktopApiToken()) {
		return false
	}

	const parsed = parseDesktopNavigationTarget(input)
	if (!parsed || !desktopExternalSchemes.has(parsed.protocol)) {
		return false
	}

	try {
		const response = await fetch(desktopEndpoint('/api/v1/desktop/open-external'), {
			method: 'POST',
			cache: 'no-store',
			headers: {
				'Content-Type': 'application/json',
				...getDesktopApiHeaders(),
			},
			body: JSON.stringify({
				url: parsed.toString(),
			}),
		})
		const payload = await response.json().catch(() => null)
		return Boolean(response.ok && payload?.success !== false)
	} catch {
		return false
	}
}
