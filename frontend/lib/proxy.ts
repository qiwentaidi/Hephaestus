/**
 * 代理配置工具函数
 * 从 localStorage 读取系统设置中的代理配置，并可回退到后端主机的系统代理
 */

import { fetchWithAuth } from './api'

export interface ProxyConfig {
  enabled: boolean
  mode: 'HTTP' | 'SOCKS5'
  address: string
  port: number
  username?: string
  password?: string
}

const PROXY_CONFIG_KEY = 'proxyConfig'

/**
 * 获取代理配置
 */
export function getProxyConfig(): ProxyConfig | null {
  if (typeof window === 'undefined') return null

  const savedConfig = localStorage.getItem(PROXY_CONFIG_KEY)
  if (!savedConfig) return null

  try {
    return JSON.parse(savedConfig) as ProxyConfig
  } catch {
    return null
  }
}

/**
 * 获取代理 URL 字符串
 * 如果代理未启用或配置无效，返回空字符串
 * @returns 格式如: http://127.0.0.1:8080 或 socks5://user:pass@127.0.0.1:1080
 */
export function getProxyUrl(): string {
  const config = getProxyConfig()

  if (!config || !config.enabled || !config.address || !config.port) {
    return ''
  }

  const protocol = config.mode === 'SOCKS5' ? 'socks5' : 'http'

  // 如果有用户名和密码
  if (config.username && config.password) {
    return `${protocol}://${encodeURIComponent(config.username)}:${encodeURIComponent(config.password)}@${config.address}:${config.port}`
  }

  return `${protocol}://${config.address}:${config.port}`
}

/**
 * 检查代理是否启用
 */
export function isProxyEnabled(): boolean {
  const config = getProxyConfig()
  return config?.enabled ?? false
}

/** 后端主机检测到的操作系统代理 */
export interface SystemProxyInfo {
  enabled: boolean
  httpProxy: string
  httpsProxy: string
  noProxy: string
}

export type ProxySource = 'app' | 'system' | 'none'

export interface ResolvedProxy {
  url: string
  source: ProxySource
}

/**
 * 读取后端主机的系统代理配置。
 * 读取失败时返回 null，调用方应按“未启用”处理。
 */
export async function fetchSystemProxyInfo(): Promise<SystemProxyInfo | null> {
  try {
    const response = await fetchWithAuth('/api/v1/system/proxy', { cache: 'no-store' })
    if (!response.ok) {
      return null
    }
    const data = await response.json()
    return data?.success && data.data ? (data.data as SystemProxyInfo) : null
  } catch {
    return null
  }
}

export function getSystemProxyUrl(info: SystemProxyInfo | null | undefined): string {
  if (!info?.enabled) {
    return ''
  }
  return (info.httpProxy || '').trim() || (info.httpsProxy || '').trim()
}

/**
 * 解析本次请求应使用的代理。
 * 优先级：应用设置里的扫描代理 > 后端主机的系统代理 > 直连。
 */
export function resolveProxyUrl(info?: SystemProxyInfo | null): ResolvedProxy {
  const appProxy = getProxyUrl()
  if (appProxy) {
    return { url: appProxy, source: 'app' }
  }
  const systemProxy = getSystemProxyUrl(info)
  if (systemProxy) {
    return { url: systemProxy, source: 'system' }
  }
  return { url: '', source: 'none' }
}

function ipv4ToNumber(value: string): number | null {
  const parts = value.split('.')
  if (parts.length !== 4) {
    return null
  }
  let result = 0
  for (const part of parts) {
    if (!/^\d{1,3}$/.test(part)) {
      return null
    }
    const octet = Number(part)
    if (octet < 0 || octet > 255) {
      return null
    }
    result = result * 256 + octet
  }
  return result >>> 0
}

function matchesIPv4CIDR(host: string, cidr: string): boolean {
  const [network, prefixRaw] = cidr.split('/')
  const prefix = Number(prefixRaw)
  if (!Number.isInteger(prefix) || prefix < 0 || prefix > 32) {
    return false
  }
  const hostNumber = ipv4ToNumber(host)
  const networkNumber = ipv4ToNumber(network)
  if (hostNumber === null || networkNumber === null) {
    return false
  }
  if (prefix === 0) {
    return true
  }
  const mask = (0xffffffff << (32 - prefix)) >>> 0
  return ((hostNumber & mask) >>> 0) === ((networkNumber & mask) >>> 0)
}

/**
 * 系统代理的 noProxy / 代理例外列表匹配。
 * 始终放过本机回环地址，避免本地调试请求被系统代理劫持。
 */
export function shouldBypassProxy(targetURL: string, noProxy?: string): boolean {
  let host = ''
  try {
    host = new URL(targetURL).hostname.toLowerCase()
  } catch {
    return false
  }
  const bareHost = host.replace(/^\[|\]$/g, '')
  if (bareHost === 'localhost' || bareHost === '::1' || bareHost.startsWith('127.')) {
    return true
  }

  const entries = (noProxy || '')
    .split(/[\s,;]+/)
    .map((entry) => entry.trim())
    .filter(Boolean)

  for (const entry of entries) {
    if (entry === '*') {
      return true
    }
    if (/^<local>$/i.test(entry) && !bareHost.includes('.')) {
      return true
    }
    if (entry.includes('/')) {
      if (matchesIPv4CIDR(bareHost, entry)) {
        return true
      }
      continue
    }
    const normalized = entry.replace(/^\*?\./, '').toLowerCase()
    if (!normalized) {
      continue
    }
    if (bareHost === normalized || bareHost.endsWith(`.${normalized}`)) {
      return true
    }
  }

  return false
}

/** 隐藏代理 URL 中的用户名和密码，供界面展示。 */
export function maskProxyUrl(url: string): string {
  return url.replace(/^([a-z0-9+.-]+:\/\/)([^/@]+)@/i, '$1***@')
}
