import { fetchWithAuth } from '@/lib/api'

export const CONFIG_MIGRATION_SCHEMA = 'hephaestus.config.migration'
export const CONFIG_MIGRATION_VERSION = 1

export const CONFIG_MIGRATION_MODULES = ['ai', 'apiKeys', 'scanTemplates', 'notifications'] as const

export type ConfigMigrationModuleId = (typeof CONFIG_MIGRATION_MODULES)[number]

export interface ConfigMigrationModule {
  id: ConfigMigrationModuleId
  data: unknown
}

export interface ConfigMigrationBundle {
  schema: string
  version: number
  exportedAt: string
  appVersion?: string
  encrypted?: boolean
  modules: ConfigMigrationModule[]
}

export interface ConfigMigrationModuleResult {
  id: ConfigMigrationModuleId
  applied: number
  created?: number
  updated?: number
}

export interface ConfigMigrationImportResult {
  modules: ConfigMigrationModuleResult[]
}

function isModuleId(value: unknown): value is ConfigMigrationModuleId {
  return typeof value === 'string' && (CONFIG_MIGRATION_MODULES as readonly string[]).includes(value)
}

/**
 * Validates a parsed JSON file before it is shown in the import preview.
 * Throws `invalid_bundle`, `unsupported_version` or `empty_bundle` codes that
 * the settings UI translates into user-facing messages.
 */
export function parseConfigMigrationBundle(raw: unknown): ConfigMigrationBundle {
  if (!raw || typeof raw !== 'object') {
    throw new Error('invalid_bundle')
  }
  const candidate = raw as Partial<ConfigMigrationBundle>
  if (candidate.schema !== CONFIG_MIGRATION_SCHEMA || !Array.isArray(candidate.modules)) {
    throw new Error('invalid_bundle')
  }
  if (candidate.version !== CONFIG_MIGRATION_VERSION) {
    throw new Error('unsupported_version')
  }

  const seen = new Set<ConfigMigrationModuleId>()
  const modules: ConfigMigrationModule[] = []
  for (const item of candidate.modules) {
    if (!item || typeof item !== 'object') continue
    const moduleItem = item as ConfigMigrationModule
    if (!isModuleId(moduleItem.id) || seen.has(moduleItem.id)) continue
    if (moduleItem.data === undefined || moduleItem.data === null) continue
    seen.add(moduleItem.id)
    modules.push({ id: moduleItem.id, data: moduleItem.data })
  }
  if (modules.length === 0) {
    throw new Error('empty_bundle')
  }

  return {
    schema: CONFIG_MIGRATION_SCHEMA,
    version: CONFIG_MIGRATION_VERSION,
    exportedAt: typeof candidate.exportedAt === 'string' ? candidate.exportedAt : '',
    appVersion: typeof candidate.appVersion === 'string' ? candidate.appVersion : undefined,
    encrypted: candidate.encrypted !== false,
    modules,
  }
}

export async function exportConfigMigration(
  modules: ConfigMigrationModuleId[],
): Promise<ConfigMigrationBundle> {
  const params = new URLSearchParams({ modules: modules.join(',') })
  const response = await fetchWithAuth(`/api/v1/config/migration/export?${params.toString()}`)
  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    throw new Error(result.error || 'export_failed')
  }
  return parseConfigMigrationBundle(result.data)
}

export async function importConfigMigration(
  bundle: ConfigMigrationBundle,
  modules: ConfigMigrationModuleId[],
): Promise<ConfigMigrationImportResult> {
  const response = await fetchWithAuth('/api/v1/config/migration/import', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bundle, modules }),
  })
  const result = await response.json().catch(() => ({}))
  if (!response.ok || !result.success) {
    throw new Error(result.error || 'import_failed')
  }
  return result.data as ConfigMigrationImportResult
}

/** Returns a human count used by the export/import preview cards. */
export function countConfigMigrationModuleItems(id: ConfigMigrationModuleId, data: unknown): number {
  if (!data || typeof data !== 'object') return 0

  if (id === 'ai') {
    const providers = (data as { providers?: unknown }).providers
    return Array.isArray(providers) ? providers.length : 0
  }
  if (id === 'scanTemplates') {
    return Array.isArray(data) ? data.length : 0
  }
  if (id === 'notifications') {
    const channels = (data as { channels?: unknown }).channels
    return Array.isArray(channels) ? channels.length : 0
  }
  if (id === 'apiKeys') {
    const engines = data as Record<string, unknown>
    let count = 0
    for (const value of Object.values(engines)) {
      if (!value || typeof value !== 'object') continue
      const entry = value as Record<string, unknown>
      const secret = entry.key ?? entry.token
      if (typeof secret === 'string' && secret.trim() !== '') count += 1
    }
    return count
  }
  return 0
}
