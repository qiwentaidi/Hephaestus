'use client'

import { useEffect, useRef } from 'react'
import { useSearchParams } from 'next/navigation'

export type SpaceEngine = 'fofa' | 'hunter' | 'quake' | 'uncover'

export const SPACE_ENGINES: SpaceEngine[] = ['fofa', 'hunter', 'quake', 'uncover']

export const SPACE_ENGINE_LABELS: Record<SpaceEngine, string> = {
  fofa: 'FOFA',
  hunter: 'Hunter',
  quake: 'Quake',
  uncover: 'Uncover',
}

export const SPACE_ENGINE_PATHS: Record<SpaceEngine, string> = {
  fofa: '/space/fofa',
  hunter: '/space/hunter',
  quake: '/space/quake',
  uncover: '/space/uncover',
}

export interface SpaceEngineTarget {
  kind: 'ip' | 'domain'
  host: string
}

const IPV4_PATTERN = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/

function isIPv4(value: string) {
  return IPV4_PATTERN.test(value)
}

function isIPv6(value: string) {
  if (!value.includes(':')) return false
  try {
    // The URL parser validates the IPv6 literal for us.
    new URL(`http://[${value}]/`)
    return true
  } catch {
    return false
  }
}

export function isIPAddress(value: string) {
  return isIPv4(value) || isIPv6(value)
}

export function normalizeAssetHost(value: string) {
  let host = value.trim().toLowerCase()
  if (host.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1)
  if (isIPAddress(host)) return ''
  host = host.replace(/^\*\./, '').replace(/\.$/, '')
  if (!host) return ''
  try {
    // Parsing also strips a trailing port and rejects malformed hosts.
    host = new URL(host.includes('://') ? host : `http://${host}`).hostname
  } catch {
    return ''
  }
  if (!host || host.length > 253 || /[\s/?#]/.test(host)) return ''
  if (isIPAddress(host)) return ''
  if (!host.includes('.')) return ''
  return host
}

function targetFromURL(value: string): SpaceEngineTarget | null {
  try {
    const parsed = new URL(value)
    const host = parsed.hostname.replace(/^\[|\]$/g, '')
    if (!host) return null
    return isIPAddress(host) ? { kind: 'ip', host } : { kind: 'domain', host: host.toLowerCase() }
  } catch {
    return null
  }
}

/**
 * Maps an asset ledger identity to the value a space engine can query.
 * Service assets have no stable host, so they are intentionally unsupported.
 */
export function resolveSpaceEngineTarget(assetType: string, rawValue: string): SpaceEngineTarget | null {
  const value = (rawValue || '').trim()
  if (!value) return null
  const type = (assetType || '').toLowerCase()
  if (type === 'service') return null
  if (type === 'ip') return isIPAddress(value) ? { kind: 'ip', host: value } : null
  if (type === 'url') return targetFromURL(value)
  if (type === 'domain') {
    const host = normalizeAssetHost(value)
    return host ? { kind: 'domain', host } : null
  }
  if (isIPAddress(value)) return { kind: 'ip', host: value }
  if (/^https?:\/\//i.test(value)) return targetFromURL(value)
  const host = normalizeAssetHost(value)
  return host ? { kind: 'domain', host } : null
}

const QUERY_TEMPLATES: Record<SpaceEngine, (target: SpaceEngineTarget) => string> = {
  fofa: (target) => `${target.kind}="${target.host}"`,
  hunter: (target) => `${target.kind}="${target.host}"`,
  quake: (target) => (target.host.includes(':') ? `${target.kind}:"${target.host}"` : `${target.kind}:${target.host}`),
  uncover: (target) => (target.host.includes(':') ? `${target.kind}:"${target.host}"` : `${target.kind}:${target.host}`),
}

export function buildSpaceEngineQuery(engine: SpaceEngine, target: SpaceEngineTarget): string {
  return QUERY_TEMPLATES[engine](target)
}

export function spaceEngineSearchPath(engine: SpaceEngine, query: string) {
  return `${SPACE_ENGINE_PATHS[engine]}?q=${encodeURIComponent(query)}`
}

/**
 * Seeds a space engine search box from the `q` query parameter so asset
 * center links can hand an asset value over to the corresponding engine.
 */
export function useSpaceEngineQueryParam(setQuery: (query: string) => void) {
  const searchParams = useSearchParams()
  const appliedRef = useRef('')

  useEffect(() => {
    const incoming = (searchParams.get('q') || '').trim()
    if (!incoming || incoming === appliedRef.current) return
    appliedRef.current = incoming
    setQuery(incoming)
  }, [searchParams, setQuery])
}
