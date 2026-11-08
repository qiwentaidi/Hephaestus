import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from './env'

export function parseFilenameFromContentDisposition(headerValue: string | null): string | null {
	if (!headerValue) return null

	// RFC 5987: filename*=UTF-8''...
	const filenameStar = headerValue.match(/filename\*\s*=\s*([^;]+)/i)
	if (filenameStar?.[1]) {
		const raw = filenameStar[1].trim().replace(/^UTF-8''/i, '')
		try {
			return decodeURIComponent(raw.replace(/^"+|"+$/g, ''))
		} catch {
			return raw.replace(/^"+|"+$/g, '')
		}
	}

	const filename = headerValue.match(/filename\s*=\s*([^;]+)/i)
	if (filename?.[1]) return filename[1].trim().replace(/^"+|"+$/g, '')
	return null
}

export function ensureFileExtension(filename: string, extension: string) {
	const cleanExtension = extension.startsWith('.') ? extension.toLowerCase() : `.${extension.toLowerCase()}`
	const trimmed = filename.trim()
	if (!trimmed) return trimmed
	return trimmed.toLowerCase().endsWith(cleanExtension) ? trimmed : `${trimmed}${cleanExtension}`
}

function triggerBrowserDownload(blob: Blob, filename: string) {
	const url = URL.createObjectURL(blob)
	const link = document.createElement('a')
	link.href = url
	link.download = filename
	link.click()
	URL.revokeObjectURL(url)
}

export async function saveBlobAsFile(blob: Blob, filename: string): Promise<'saved' | 'cancelled'> {
	if (!hasDesktopApiToken()) {
		triggerBrowserDownload(blob, filename)
		return 'saved'
	}

	const formData = new FormData()
	formData.append('filename', filename)
	formData.append('file', blob, filename)

	const response = await fetch(`${getApiBaseURL()}/api/v1/desktop/save-download`, {
		method: 'POST',
		headers: getDesktopApiHeaders(),
		body: formData,
	})

	if (!response.ok) {
		const payload = await response.json().catch(() => ({}))
		throw new Error(payload?.error || `save download failed: ${response.status}`)
	}

	const payload = await response.json().catch(() => ({}))
	if (payload?.data?.cancelled) {
		return 'cancelled'
	}

	return 'saved'
}
