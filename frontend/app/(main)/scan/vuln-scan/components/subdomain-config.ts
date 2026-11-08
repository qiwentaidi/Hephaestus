import { fetchWithAuth } from '@/lib/api'
import type { SpaceEngineConfig } from '@/lib/config'

export const SPACE_ENGINES = [
  { name: 'Chaos', requiresApi: true, recommended: true },
  { name: 'FOFA', requiresApi: true, recommended: false },
  { name: 'Hunter', requiresApi: true, recommended: false },
  { name: 'Quake', requiresApi: true, recommended: false },
  { name: 'Bevigil', requiresApi: true, recommended: false },
  { name: 'ZoomEye', requiresApi: true, recommended: false },
  { name: 'GitHub', requiresApi: true, recommended: false },
  { name: 'Securitytrails', requiresApi: true, recommended: false },
  { name: 'Digitorus', requiresApi: false, recommended: false },
  { name: 'HackerTarget', requiresApi: false, recommended: false },
  { name: 'THC', requiresApi: false, recommended: false },
  { name: 'Anubis', requiresApi: false, recommended: false },
  { name: 'crt.name', requiresApi: false, recommended: false },
  { name: 'IP138', requiresApi: false, recommended: false },
] as const

export const SPACE_ENGINE_NAMES = SPACE_ENGINES.map((engine) => engine.name)
export type SpaceEngineName = typeof SPACE_ENGINE_NAMES[number]
export const DEFAULT_SUBDOMAIN_ENGINES: SpaceEngineName[] = [
  'Digitorus',
  'HackerTarget',
  'THC',
  'Anubis',
  'crt.name',
  'IP138',
]

export type DNSServerSetting = {
  address: string
  name: string
  enabled: boolean
}

export type SubdomainConfigValue = {
  thread: number
  timeout: number
  appendEngines: string[]
  dnsServers: DNSServerSetting[]
}

export const DEFAULT_DNS_SERVERS: DNSServerSetting[] = [
  { address: '223.6.6.6:53', name: '阿里DNS', enabled: true },
  { address: '8.8.8.8:53', name: 'Google DNS', enabled: true },
  { address: '114.114.114.114:53', name: '114 DNS', enabled: false },
  { address: '1.1.1.1:53', name: 'Cloudflare DNS', enabled: false },
  { address: '119.29.29.29:53', name: '腾讯DNS', enabled: false },
]

export const cloneDefaultDNSServers = () => DEFAULT_DNS_SERVERS.map((server) => ({ ...server }))

export const normalizeDNSServersFromResponse = (servers: unknown): DNSServerSetting[] => {
  if (!Array.isArray(servers)) {
    return cloneDefaultDNSServers()
  }

  const normalized = servers
    .map((server) => {
      if (!server || typeof server !== 'object') return null
      const item = server as Partial<DNSServerSetting>
      const address = typeof item.address === 'string' ? item.address.trim() : ''
      if (!address) return null
      return {
        address,
        name: typeof item.name === 'string' ? item.name.trim() : '',
        enabled: Boolean(item.enabled),
      }
    })
    .filter((server): server is DNSServerSetting => server !== null)

  return normalized.length > 0 ? normalized : cloneDefaultDNSServers()
}

export const getSubdomainDNSServerSettings = async () => {
  const response = await fetchWithAuth('/api/v1/subdomain/dns-servers', { cache: 'no-store' })
  const data = await response.json().catch(() => null)
  if (!response.ok || !data?.success) {
    throw new Error(data?.error || 'Failed to load DNS server settings')
  }
  return normalizeDNSServersFromResponse(data.data?.servers)
}

export const isSubdomainEngineConfigured = (
  config: SpaceEngineConfig | null,
  engineName: SpaceEngineName,
) => {
  if (!config) return false
  switch (engineName) {
    case 'Chaos': return Boolean(config.chaos.key)
    case 'FOFA': return Boolean(config.fofa.key && config.fofa.email)
    case 'Hunter': return Boolean(config.hunter.key)
    case 'Quake': return Boolean(config.quake.key)
    case 'Bevigil': return Boolean(config.bevigil.key)
    case 'ZoomEye': return Boolean(config.zoomeye.key)
    case 'GitHub': return Boolean(config.github.token)
    case 'Securitytrails': return Boolean(config.securitytrails.key)
    case 'Digitorus':
    case 'HackerTarget':
    case 'THC':
    case 'Anubis':
    case 'crt.name': return true
    case 'IP138': return true
    default: return false
  }
}

export const getDefaultSubdomainEngines = (config: SpaceEngineConfig | null): SpaceEngineName[] => {
  const configuredEngines = SPACE_ENGINES
    .filter((engine) => engine.requiresApi && isSubdomainEngineConfigured(config, engine.name))
    .map((engine) => engine.name)
  return Array.from(new Set([...configuredEngines, ...DEFAULT_SUBDOMAIN_ENGINES]))
}

export const isLegacyDefaultSubdomainEngineSelection = (engines: string[]) => {
  const normalized = engines.map((engine) => String(engine).trim()).filter(Boolean)
  if (normalized.length === 0) return true
  const legacyDefaults = new Set(['crt.name', 'IP138'])
  return normalized.every((engine) => legacyDefaults.has(engine))
}

export const normalizeSubdomainConfig = (
  rawConfig: unknown,
  fallbackDnsServers: DNSServerSetting[] = cloneDefaultDNSServers(),
): SubdomainConfigValue => {
  const config = rawConfig && typeof rawConfig === 'object' ? rawConfig as Record<string, unknown> : {}
  const rawEngines = Array.isArray(config.appendEngines)
    ? config.appendEngines
    : typeof config.appendEngines === 'string'
      ? config.appendEngines.split(',')
      : [...DEFAULT_SUBDOMAIN_ENGINES]
  const appendEngines = rawEngines
    .map((engine) => String(engine).trim())
    .filter((engine): engine is SpaceEngineName => SPACE_ENGINE_NAMES.includes(engine as SpaceEngineName))
  const normalizedAppendEngines = isLegacyDefaultSubdomainEngineSelection(appendEngines)
    ? [...DEFAULT_SUBDOMAIN_ENGINES]
    : appendEngines

  const rawDnsServers = Array.isArray(config.dnsServers) ? config.dnsServers : fallbackDnsServers
  const dnsServers = rawDnsServers
    .map((server) => {
      const address = typeof server === 'string'
        ? server.trim()
        : server && typeof server === 'object'
          ? String((server as Partial<DNSServerSetting>).address || '').trim()
          : ''
      if (!address) return null
      const source = fallbackDnsServers.find((item) => item.address === address)
      return {
        address,
        name: source?.name || (typeof server === 'object' && server ? String((server as Partial<DNSServerSetting>).name || '') : ''),
        enabled: typeof server === 'object' && server ? Boolean((server as Partial<DNSServerSetting>).enabled ?? true) : true,
      }
    })
    .filter((server): server is DNSServerSetting => server !== null)

  return {
    thread: Number(config.thread) || 600,
    timeout: Number(config.timeout) || 5,
    appendEngines: normalizedAppendEngines.length > 0 ? normalizedAppendEngines : [...DEFAULT_SUBDOMAIN_ENGINES],
    dnsServers,
  }
}
