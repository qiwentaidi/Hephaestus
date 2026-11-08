import type { AssetLedgerEntity } from '@/lib/api/unified-assets'
import type { CombinationTargetInputType } from '@/lib/combination-presets'

export type AssetScanSkipReason =
  | 'needs-domain'
  | 'needs-network'
  | 'needs-web'
  | 'needs-service'
  | 'non-web-port'
  | 'no-host'
  | 'invalid-template'

export interface AssetScanSkippedEntity {
  entity: AssetLedgerEntity
  reason: AssetScanSkipReason
}

export interface DerivedCombinationTargets {
  targets: string[]
  skipped: AssetScanSkippedEntity[]
}

export type AssetScanTargetCategory = 'root' | 'subdomain' | 'ip' | 'port' | 'website'

export interface DerivedAITarget {
  target: string
  category: AssetScanTargetCategory
}

const HTTP_PORTS = new Set([80, 81, 3000, 5000, 8000, 8080, 8081, 8088, 8888, 9000, 9090, 10000])
const HTTPS_PORTS = new Set([443, 8443, 9443, 10443, 2053, 2083, 2087, 2096])

const CRACK_SCHEMES = new Set([
  'ssh',
  'rdp',
  'ftp',
  'telnet',
  'smb',
  'mysql',
  'mssql',
  'postgresql',
  'mongodb',
  'redis',
  'oracle',
  'pop3',
  'ldap',
  'vnc',
  'mqtt',
  'memcached',
  'jdwp',
  'adb',
  'java-rmi',
  'activemq',
  'rsync',
  'kafka',
  'clickhouse',
])

const DOMAIN_LABEL_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/
const IPV4_SEGMENT_PATTERN = /^(25[0-5]|2[0-4]\d|1?\d?\d)$/

function normalizeHost(value: string | undefined): string {
  let host = (value || '').trim().toLowerCase()
  if (!host) return ''
  if (host.startsWith('[') && host.endsWith(']')) host = host.slice(1, -1)
  if (host.endsWith('.')) host = host.slice(0, -1)
  return host
}

function hostFromEntity(entity: AssetLedgerEntity): string {
  const direct = normalizeHost(entity.host)
  if (direct) return direct
  if (entity.category === 'website' || entity.value.includes('://')) {
    try {
      return normalizeHost(new URL(entity.value).hostname)
    } catch {
      return ''
    }
  }
  return normalizeHost(entity.value)
}

function hostForURL(host: string): string {
  return host.includes(':') ? `[${host}]` : host
}

function formatHostPort(host: string, port: number): string {
  return `${hostForURL(host)}:${port}`
}

function normalizeURL(value: string): string {
  try {
    const parsed = new URL(value.trim())
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return ''
    if (!parsed.hostname) return ''
    parsed.hash = ''
    if ((parsed.protocol === 'http:' && parsed.port === '80') || (parsed.protocol === 'https:' && parsed.port === '443')) {
      parsed.port = ''
    }
    const normalized = parsed.toString()
    if (parsed.pathname === '/' && !parsed.search) {
      return normalized.replace(/\/$/, '')
    }
    return normalized
  } catch {
    return ''
  }
}

function isIPv4(value: string): boolean {
  const parts = value.split('.')
  return parts.length === 4 && parts.every((part) => IPV4_SEGMENT_PATTERN.test(part))
}

function isIPv6(value: string): boolean {
  return value.includes(':') && /^[0-9a-f:.]+$/.test(value)
}

function isCIDR(value: string): boolean {
  const [host, mask] = value.split('/', 2)
  if (!host || mask === undefined || !/^\d+$/.test(mask)) return false
  if (isIPv4(host)) return Number(mask) >= 0 && Number(mask) <= 32
  if (isIPv6(host)) return Number(mask) >= 0 && Number(mask) <= 128
  return false
}

function isHostname(value: string): boolean {
  if (!value || value.includes(':') || value.includes('/') || value.includes('?') || value.includes('#')) return false
  return value.split('.').every((label) => DOMAIN_LABEL_PATTERN.test(label))
}

function isDomainTarget(value: string): boolean {
  const normalized = normalizeHost(value)
  if (!normalized || normalized.includes('://') || /[/?#@:]/.test(normalized)) return false
  if (isIPv4(normalized) || isIPv6(normalized)) return false
  const labels = normalized.split('.')
  if (labels.length < 2) return false
  return labels.every((label) => DOMAIN_LABEL_PATTERN.test(label))
}

function isNetworkTarget(value: string): boolean {
  const normalized = normalizeHost(value)
  if (!normalized || normalized.includes('://') || /[?#@]/.test(normalized)) return false
  if (isIPv4(normalized) || isIPv6(normalized)) return true
  if (normalized.includes('/')) return isCIDR(normalized)
  return isHostname(normalized)
}

function webSchemeForEntity(entity: AssetLedgerEntity): 'http' | 'https' | '' {
  if (entity.category === 'website') {
    try {
      const protocol = new URL(entity.value).protocol
      if (protocol === 'https:') return 'https'
      if (protocol === 'http:') return 'http'
    } catch {
      return ''
    }
    return ''
  }
  if (entity.category === 'port') {
    const protocol = (entity.protocol || '').toLowerCase()
    if (protocol.includes('https')) return 'https'
    if (protocol.includes('http')) return 'http'
    if (entity.port && HTTPS_PORTS.has(entity.port)) return 'https'
    if (entity.port && HTTP_PORTS.has(entity.port)) return 'http'
  }
  return ''
}

function crackSchemeForEntity(entity: AssetLedgerEntity): string {
  const labels = (entity.protocol || '')
    .toLowerCase()
    .split('/')
    .map((label) => label.trim())
    .filter(Boolean)
  return labels.find((label) => CRACK_SCHEMES.has(label)) || ''
}

function pushTarget(list: string[], seen: Set<string>, target: string) {
  if (!target) return
  const key = target.toLowerCase()
  if (seen.has(key)) return
  seen.add(key)
  list.push(target)
}

export function deriveCombinationTargets(
  entities: AssetLedgerEntity[],
  inputType: CombinationTargetInputType,
): DerivedCombinationTargets {
  const targets: string[] = []
  const skipped: AssetScanSkippedEntity[] = []
  const seen = new Set<string>()

  for (const entity of entities) {
    switch (inputType) {
      case 'domain': {
        if (entity.category !== 'root' && entity.category !== 'subdomain') {
          skipped.push({ entity, reason: 'needs-domain' })
          break
        }
        const value = normalizeHost(entity.value)
        if (!isDomainTarget(value)) {
          skipped.push({ entity, reason: 'needs-domain' })
          break
        }
        pushTarget(targets, seen, value)
        break
      }
      case 'network': {
        const host = hostFromEntity(entity)
        if (!host || !isNetworkTarget(host)) {
          skipped.push({ entity, reason: host ? 'needs-network' : 'no-host' })
          break
        }
        pushTarget(targets, seen, host)
        break
      }
      case 'web': {
        if (entity.category === 'website') {
          const url = normalizeURL(entity.value)
          if (!url) {
            skipped.push({ entity, reason: 'needs-web' })
            break
          }
          pushTarget(targets, seen, url)
          break
        }
        if (entity.category === 'port') {
          const scheme = webSchemeForEntity(entity)
          const host = hostFromEntity(entity)
          if (!scheme || !host || !entity.port) {
            skipped.push({ entity, reason: 'non-web-port' })
            break
          }
          pushTarget(targets, seen, `${scheme}://${formatHostPort(host, entity.port)}`)
          break
        }
        if (entity.category === 'root' || entity.category === 'subdomain') {
          const host = hostFromEntity(entity)
          if (!host) {
            skipped.push({ entity, reason: 'no-host' })
            break
          }
          pushTarget(targets, seen, `https://${hostForURL(host)}`)
          pushTarget(targets, seen, `http://${hostForURL(host)}`)
          break
        }
        skipped.push({ entity, reason: 'needs-web' })
        break
      }
      case 'service': {
        if (entity.category !== 'port') {
          skipped.push({ entity, reason: 'needs-service' })
          break
        }
        const scheme = crackSchemeForEntity(entity)
        const host = hostFromEntity(entity)
        if (!scheme || !host || !entity.port) {
          skipped.push({ entity, reason: 'needs-service' })
          break
        }
        pushTarget(targets, seen, `${scheme}://${formatHostPort(host, entity.port)}`)
        break
      }
    }
  }

  return { targets, skipped }
}

export function deriveAITargets(entities: AssetLedgerEntity[]): DerivedAITarget[] {
  const result: DerivedAITarget[] = []
  const seen = new Set<string>()
  const add = (target: string, category: AssetScanTargetCategory) => {
    if (!target) return
    const key = target.toLowerCase()
    if (seen.has(key)) return
    seen.add(key)
    result.push({ target, category })
  }

  for (const entity of entities) {
    const category = entity.category as AssetScanTargetCategory
    switch (category) {
      case 'root':
      case 'subdomain':
      case 'ip': {
        const target = normalizeHost(entity.value)
        if (target) add(target, category)
        break
      }
      case 'website': {
        const target = normalizeURL(entity.value)
        if (target) add(target, category)
        break
      }
      case 'port': {
        const host = hostFromEntity(entity)
        if (!host || !entity.port) break
        const scheme = webSchemeForEntity(entity)
        add(scheme ? `${scheme}://${formatHostPort(host, entity.port)}` : formatHostPort(host, entity.port), category)
        break
      }
    }
  }

  return result
}

export function buildAIGoal(target: DerivedAITarget, language: 'zh' | 'en'): string {
  if (language === 'zh') {
    switch (target.category) {
      case 'root':
      case 'subdomain':
        return `域名 ${target.target}`
      case 'ip':
        return `主机 ${target.target}`
      case 'port':
        return `服务 ${target.target}`
      case 'website':
        return `网站 ${target.target}`
    }
  }
  switch (target.category) {
    case 'root':
    case 'subdomain':
      return `domain ${target.target}`
    case 'ip':
      return `host ${target.target}`
    case 'port':
      return `service ${target.target}`
    case 'website':
      return `website ${target.target}`
  }
}

const BUILT_IN_AI_PROMPT: Record<'zh' | 'en', string> = {
  zh: '请对{{goal}}进行授权渗透测试。基于可验证证据自主规划和推进测试，记录发现、证据与未完成项，避免破坏性操作。',
  en: 'Conduct an authorized penetration test against {{goal}}. Plan and progress from verifiable evidence, record findings, evidence, and unfinished items, and avoid destructive actions.',
}

export function buildAIInstructions(
  targets: DerivedAITarget[],
  templatePrompt: string | undefined,
  language: 'zh' | 'en',
): string[] {
  const prompt = (templatePrompt || '').trim() || BUILT_IN_AI_PROMPT[language]
  return targets.map((target) => {
    const goal = buildAIGoal(target, language)
    if (prompt.includes('{{.goal}}')) {
      return prompt.replaceAll('{{.goal}}', goal)
    }
    if (prompt.includes('{{goal}}')) {
      return prompt.replaceAll('{{goal}}', goal)
    }
    return `${prompt}\n\n${language === 'zh' ? '目标' : 'Target'}: ${goal}`
  })
}

export function summarizeSkippedReasons(
  skipped: AssetScanSkippedEntity[],
): Array<{ reason: AssetScanSkipReason; count: number }> {
  const counts = new Map<AssetScanSkipReason, number>()
  for (const item of skipped) {
    counts.set(item.reason, (counts.get(item.reason) || 0) + 1)
  }
  return [...counts.entries()]
    .map(([reason, count]) => ({ reason, count }))
    .sort((left, right) => right.count - left.count)
}
