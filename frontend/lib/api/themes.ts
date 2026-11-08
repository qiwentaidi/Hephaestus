import { fetchWithAuth } from '@/lib/api'
import {
  parseThemeDefinition,
  type CustomThemeDefinition,
  type CustomThemeRecord,
  type ThemeBase,
} from '@/lib/theme/custom-theme'

export class ThemeApiError extends Error {
  status: number
  code?: string
  conflicts: string[]

  constructor(message: string, options: { status: number; code?: string; conflicts?: string[] }) {
    super(message)
    this.name = 'ThemeApiError'
    this.status = options.status
    this.code = options.code
    this.conflicts = options.conflicts ?? []
  }
}

interface ApiEnvelope<T> {
  success?: boolean
  data?: T
  error?: string
  code?: string
  conflicts?: string[]
}

async function readEnvelope<T>(response: Response): Promise<ApiEnvelope<T>> {
  const payload = (await response.json().catch(() => null)) as ApiEnvelope<T> | null
  if (!response.ok || !payload?.success) {
    throw new ThemeApiError(payload?.error || response.statusText || 'theme request failed', {
      status: response.status,
      code: payload?.code,
      conflicts: payload?.conflicts,
    })
  }
  return payload
}

export async function listCustomThemes(): Promise<CustomThemeRecord[]> {
  const response = await fetchWithAuth('/api/v1/themes', { cache: 'no-store' })
  const payload = await readEnvelope<CustomThemeRecord[]>(response)
  return Array.isArray(payload.data) ? payload.data : []
}

export async function createCustomTheme(
  name: string,
  definition: CustomThemeDefinition,
): Promise<CustomThemeRecord> {
  const response = await fetchWithAuth('/api/v1/themes', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, definition }),
  })
  const payload = await readEnvelope<CustomThemeRecord>(response)
  if (!payload.data) {
    throw new ThemeApiError('missing theme in response', { status: response.status })
  }
  return payload.data
}

export async function updateCustomTheme(
  id: number,
  name: string,
  definition: CustomThemeDefinition,
): Promise<CustomThemeRecord> {
  const response = await fetchWithAuth(`/api/v1/themes/${id}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, definition }),
  })
  const payload = await readEnvelope<CustomThemeRecord>(response)
  if (!payload.data) {
    throw new ThemeApiError('missing theme in response', { status: response.status })
  }
  return payload.data
}

export async function deleteCustomTheme(id: number): Promise<void> {
  const response = await fetchWithAuth(`/api/v1/themes/${id}`, { method: 'DELETE' })
  await readEnvelope<unknown>(response)
}

export async function importCustomThemes(
  themes: Array<{ name: string; definition: CustomThemeDefinition }>,
): Promise<number> {
  const response = await fetchWithAuth('/api/v1/themes/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ themes }),
  })
  const payload = await readEnvelope<{ created?: number }>(response)
  return payload.data?.created ?? themes.length
}

export interface GeneratedTheme {
  name: string
  description?: string
  definition: CustomThemeDefinition
}

export async function generateCustomTheme(
  prompt: string,
  base?: ThemeBase,
): Promise<GeneratedTheme> {
  const response = await fetchWithAuth('/api/v1/themes/generate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt, base }),
  })
  const payload = await readEnvelope<{
    name?: string
    description?: string
    definition?: unknown
  }>(response)

  if (!payload.data?.definition) {
    throw new ThemeApiError('missing generated theme', {
      status: response.status,
      code: 'invalid_ai_response',
    })
  }

  try {
    return {
      name: payload.data.name?.trim() || 'AI 主题',
      description: payload.data.description?.trim() || undefined,
      definition: parseThemeDefinition(payload.data.definition),
    }
  } catch {
    throw new ThemeApiError('invalid generated theme', {
      status: response.status,
      code: 'invalid_ai_response',
    })
  }
}
