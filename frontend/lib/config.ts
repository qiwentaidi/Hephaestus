// 配置管理服务
import { get, put } from './api'

export interface SpaceEngineConfig {
	fofa: {
		api: string
		email: string
		key: string
	}
	hunter: {
		api: string
		key: string
	}
	quake: {
		api: string
		key: string
	}
	censys: {
		token: string
		organizationId: string
	}
	zoomeye: {
		api: string
		key: string
	}
	shodan: {
		api: string
		key: string
	}
	github: {
		token: string
	}
	chaos: {
		key: string
	}
	bevigil: {
		key: string
	}
	securitytrails: {
		key: string
	}
	ctRadar: {
		key: string
	}
}

export interface ScanConfig {
	webThread: number
	crackThread: number
	portThread: number
	portTimeout: number
	aliveModule: string
	defaultNetwork: string
}

export interface ProxyConfig {
	enabled: boolean
	mode: 'HTTP' | 'SOCKS5'
	address: string
	port: number
	username: string
	password: string
}

export interface SystemConfig {
	spaceEngines: SpaceEngineConfig
	scan: ScanConfig
	proxy: ProxyConfig
	plugins: Record<string, any>
}

// 默认空间引擎配置
const defaultSpaceEngineConfig: SpaceEngineConfig = {
	fofa: { api: 'https://fofa.info', email: '', key: '' },
	hunter: { api: 'https://hunter.qianxin.com', key: '' },
	quake: { api: 'https://quake.360.cn', key: '' },
	censys: { token: '', organizationId: '' },
	zoomeye: { api: 'https://api.zoomeye.ai/', key: '' },
	shodan: { api: 'https://api.shodan.io', key: '' },
	github: { token: '' },
	chaos: { key: '' },
	bevigil: { key: '' },
	securitytrails: { key: '' },
	ctRadar: { key: '' },
}

// 获取空间引擎配置
export async function getSpaceEngineConfig(): Promise<SpaceEngineConfig> {
	const response = await get<SpaceEngineConfig>('/api/v1/config/space-engines')
	if (response.success && response.data) {
		return normalizeSpaceEngineConfig(response.data)
	}
	// 返回默认配置
	console.warn('Failed to load space engine config, using default')
	return defaultSpaceEngineConfig
}

export function normalizeSpaceEngineConfig(config: Partial<SpaceEngineConfig>): SpaceEngineConfig {
	return {
		...defaultSpaceEngineConfig,
		...config,
		fofa: { ...defaultSpaceEngineConfig.fofa, ...config.fofa },
		hunter: { ...defaultSpaceEngineConfig.hunter, ...config.hunter },
		quake: { ...defaultSpaceEngineConfig.quake, ...config.quake },
		censys: { ...defaultSpaceEngineConfig.censys, ...config.censys },
		zoomeye: { ...defaultSpaceEngineConfig.zoomeye, ...config.zoomeye },
		shodan: { ...defaultSpaceEngineConfig.shodan, ...config.shodan },
		github: { ...defaultSpaceEngineConfig.github, ...config.github },
		chaos: { ...defaultSpaceEngineConfig.chaos, ...config.chaos },
		bevigil: { ...defaultSpaceEngineConfig.bevigil, ...config.bevigil },
		securitytrails: { ...defaultSpaceEngineConfig.securitytrails, ...config.securitytrails },
		ctRadar: { ...defaultSpaceEngineConfig.ctRadar, ...config.ctRadar },
	}
}

// 保存空间引擎配置
export async function saveSpaceEngineConfig(config: SpaceEngineConfig): Promise<void> {
	const response = await put('/api/v1/config/space-engines', config)
	if (!response.success) {
		throw new Error(response.error || '保存配置失败')
	}
}

// 获取完整配置
export async function getConfig(): Promise<SystemConfig> {
	const response = await get<SystemConfig>('/api/v1/config')
	if (response.success && response.data) {
		return {
			...response.data,
			spaceEngines: normalizeSpaceEngineConfig(response.data.spaceEngines),
		}
	}
	throw new Error(response.error || '获取配置失败')
}

// 保存完整配置
export async function saveConfig(config: SystemConfig): Promise<void> {
	const response = await put('/api/v1/config', config)
	if (!response.success) {
		throw new Error(response.error || '保存配置失败')
	}
}

// 配置变更事件监听器（用于跨页面同步）
type ConfigChangeListener = (config: SpaceEngineConfig) => void

const listeners: Set<ConfigChangeListener> = new Set()

// 监听配置变更
export function onSpaceEngineConfigChange(listener: ConfigChangeListener): () => void {
	listeners.add(listener)
	return () => {
		listeners.delete(listener)
	}
}

// 触发配置变更事件
export function notifySpaceEngineConfigChange(config: SpaceEngineConfig) {
	listeners.forEach(listener => listener(config))
}

// 获取后端URL（使用环境配置，保持向后兼容）
export { getApiBaseURL as getBackendUrlFromStorage } from './env'
