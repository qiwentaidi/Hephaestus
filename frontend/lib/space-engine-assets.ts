import { fetchWithAuth } from '@/lib/api'
import { isIPAddress, normalizeAssetHost, type SpaceEngine } from './space-engine'

const MAX_OBSERVATIONS_PER_REQUEST = 500

export interface SpaceEngineAssetRecord {
  ip?: string | number | null
  domain?: string | number | null
  url?: string | number | null
  title?: string | number | null
  port?: string | number | null
  protocol?: string | number | null
  product?: string | number | null
  country?: string | number | null
  host?: string | number | null
  status?: string | number | null
}

interface SpaceEngineObservation {
  type: 'ip' | 'domain' | 'url'
  value: string
  displayValue: string
  source: SpaceEngine
  sourceId: string
  origin: 'external'
  confidence: 'medium'
  observedAt: string
  evidenceRef?: string
  attributes: Record<string, string>
}

function text(value: string | number | null | undefined) {
  if (value === null || value === undefined) return ''
  return String(value).trim()
}

function URLValue(value: string) {
  if (!/^https?:\/\//i.test(value)) return ''
  try {
    const parsed = new URL(value)
    if (!parsed.hostname) return ''
    return value
  } catch {
    return ''
  }
}

/**
 * Converts selected space engine results into unified asset observations.
 * Invalid values are skipped so a single malformed row cannot reject the batch.
 */
export function buildSpaceEngineObservations(engine: SpaceEngine, records: SpaceEngineAssetRecord[]): SpaceEngineObservation[] {
  const observedAt = new Date().toISOString()
  const seen = new Set<string>()
  const observations: SpaceEngineObservation[] = []

  for (const record of records) {
    const ip = text(record.ip)
    const rawDomain = text(record.domain)
    const rawURL = text(record.url)
    const sourceId = [rawURL, ip, rawDomain, text(record.host), text(record.port)].filter(Boolean).join('|')
    if (!sourceId) continue

    const attributes: Record<string, string> = { engine }
    const addAttribute = (key: string, value: string) => {
      if (value) attributes[key] = value
    }
    addAttribute('ip', ip)
    addAttribute('domain', rawDomain)
    addAttribute('host', text(record.host))
    addAttribute('port', text(record.port))
    addAttribute('protocol', text(record.protocol))
    addAttribute('title', text(record.title))
    addAttribute('product', text(record.product))
    addAttribute('country', text(record.country))
    addAttribute('status', text(record.status))

    const add = (type: SpaceEngineObservation['type'], value: string) => {
      // Distinct results for the same host (for example http and https on the
      // same port) must stay separate observations: each one carries the port,
      // protocol, title and status evidence that the ledger aggregates from.
      const key = `${type}:${value}:${sourceId}`
      if (seen.has(key)) return
      seen.add(key)
      observations.push({
        type,
        value,
        displayValue: value,
        source: engine,
        sourceId,
        origin: 'external',
        confidence: 'medium',
        observedAt,
        evidenceRef: rawURL || undefined,
        attributes,
      })
    }

    if (ip && isIPAddress(ip)) add('ip', ip)
    const url = rawURL ? URLValue(rawURL) : ''
    if (url) add('url', url)

    const domains: string[] = []
    const addDomain = (value: string) => {
      const host = normalizeAssetHost(value)
      if (host && !domains.includes(host)) domains.push(host)
    }
    if (rawDomain) addDomain(rawDomain)
    if (url) {
      try {
        addDomain(new URL(url).hostname)
      } catch {
        // Ignore an unparsable URL host; the URL itself is still recorded.
      }
    }
    addDomain(text(record.host))
    for (const domain of domains) add('domain', domain)
  }

  return observations
}

/**
 * Writes selected space engine results into the unified asset ledger. The
 * assets are attributed to the engine and appear as observed identities.
 */
export async function saveSpaceEngineAssets(engine: SpaceEngine, records: SpaceEngineAssetRecord[]): Promise<{ saved: number }> {
  const observations = buildSpaceEngineObservations(engine, records)
  if (!observations.length) return { saved: 0 }

  for (let start = 0; start < observations.length; start += MAX_OBSERVATIONS_PER_REQUEST) {
    const chunk = observations.slice(start, start + MAX_OBSERVATIONS_PER_REQUEST)
    const response = await fetchWithAuth('/api/v1/assets/catalog/observations', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ observations: chunk }),
    })
    const body = await response.json().catch(() => null)
    if (!response.ok || !body?.success) {
      throw new Error(body?.error || `Asset catalog request failed (HTTP ${response.status})`)
    }
  }

  return { saved: new Set(observations.map((item) => `${item.type}:${item.value}`)).size }
}
