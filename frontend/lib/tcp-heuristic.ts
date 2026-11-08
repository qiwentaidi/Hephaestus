export interface TCPHeuristicConfig {
  subnetDiscoveryEnabled: boolean
  subnetDiscoveryMode: 'fast' | 'full'
  subnetDiscoveryHostSuffixes: string
  subnetDiscoveryRandomSamples: number
  tcpHeuristicEnabled: boolean
  tcpHeuristicPorts: string
  tcpHeuristicTimeoutMs: number
}

export const DEFAULT_TCP_HEURISTIC_CONFIG: TCPHeuristicConfig = {
  subnetDiscoveryEnabled: false,
  subnetDiscoveryMode: 'fast',
  subnetDiscoveryHostSuffixes: '1,2,254',
  subnetDiscoveryRandomSamples: 2,
  tcpHeuristicEnabled: false,
  tcpHeuristicPorts: '80,443,22,3389,445',
  tcpHeuristicTimeoutMs: 3000,
}

export function normalizeTCPHeuristicConfig(config?: Partial<TCPHeuristicConfig> | null): TCPHeuristicConfig {
  return {
    subnetDiscoveryEnabled: config?.subnetDiscoveryEnabled === true,
    // Older enabled configurations without a mode use the backend's full fallback.
    subnetDiscoveryMode: config?.subnetDiscoveryMode ?? (config?.subnetDiscoveryEnabled ? 'full' : DEFAULT_TCP_HEURISTIC_CONFIG.subnetDiscoveryMode),
    subnetDiscoveryHostSuffixes: config?.subnetDiscoveryHostSuffixes ?? '1,2,254',
    subnetDiscoveryRandomSamples: config?.subnetDiscoveryRandomSamples ?? 2,
    tcpHeuristicEnabled: config?.tcpHeuristicEnabled === true || config?.subnetDiscoveryEnabled === true,
    tcpHeuristicPorts: config?.tcpHeuristicPorts ?? DEFAULT_TCP_HEURISTIC_CONFIG.tcpHeuristicPorts,
    tcpHeuristicTimeoutMs: config?.tcpHeuristicTimeoutMs ?? DEFAULT_TCP_HEURISTIC_CONFIG.tcpHeuristicTimeoutMs,
  }
}

export function validateTCPHeuristicConfig(config: TCPHeuristicConfig, portRange: string): 'portsError' | 'timeoutError' | 'udpError' | 'subnetModeError' | 'subnetSuffixError' | 'subnetRandomError' | null {
  if (config.subnetDiscoveryEnabled) {
    if (!['fast', 'full'].includes(config.subnetDiscoveryMode)) return 'subnetModeError'
    const suffixes = (config.subnetDiscoveryHostSuffixes.trim() || '1,2,254').split(',').map((part) => part.trim())
    if (suffixes.some((part) => !/^\d+$/.test(part) || Number(part) < 1 || Number(part) > 254) || new Set(suffixes.map(Number)).size > 32) return 'subnetSuffixError'
    if (!Number.isInteger(config.subnetDiscoveryRandomSamples) || config.subnetDiscoveryRandomSamples < 0 || config.subnetDiscoveryRandomSamples > 32) return 'subnetRandomError'
  }
  if (!config.tcpHeuristicEnabled && !config.subnetDiscoveryEnabled) return null
  const value = config.tcpHeuristicPorts.trim() || DEFAULT_TCP_HEURISTIC_CONFIG.tcpHeuristicPorts
  const ports = value.split(',').map((part) => part.trim())
  if (ports.some((part) => !/^\d+$/.test(part) || Number(part) < 1 || Number(part) > 65535) || new Set(ports.map(Number)).size > 64) {
    return 'portsError'
  }
  if (!Number.isInteger(config.tcpHeuristicTimeoutMs) || config.tcpHeuristicTimeoutMs < 100 || config.tcpHeuristicTimeoutMs > 30000) {
    return 'timeoutError'
  }
  if (/\bU(?:DP)?:/i.test(portRange)) return 'udpError'
  return null
}
