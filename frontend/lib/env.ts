// 环境配置

export const envConfig = {
	// 开发环境后端端口
	BACKEND_PORT: process.env.NEXT_PUBLIC_BACKEND_PORT || '8080',

	// 是否为开发环境
	IS_DEV: process.env.NODE_ENV === 'development',

	// 是否为生产环境
	IS_PROD: process.env.NODE_ENV === 'production',
}

const DESKTOP_API_TOKEN_PARAM = 'desktop_api_token'
const DESKTOP_API_TOKEN_STORAGE_KEY = 'hephaestus:desktop-api-token'
const DESKTOP_API_TOKEN_COOKIE = 'hephaestus_desktop_api_token'

const getCookieValue = (name: string) => {
	if (typeof document === 'undefined') return null
	const prefix = `${name}=`
	for (const part of document.cookie.split(';')) {
		const trimmed = part.trim()
		if (trimmed.startsWith(prefix)) {
			const value = trimmed.slice(prefix.length).trim()
			return value || null
		}
	}
	return null
}

// 获取URL参数中的后端端口
const getBackendPortFromURL = () => {
	if (typeof window === 'undefined') return null
	const urlParams = new URLSearchParams(window.location.search)
	return urlParams.get('backend_port')
}

export const getDesktopApiToken = () => {
	if (typeof window === 'undefined') return null

	// 仅开发模式回退到 URL 参数：页面由 Next dev server 提供，桌面后端看不到
	// 携带一次性 nonce 的文档请求，无法完成 Cookie 下发。
	const urlParams = new URLSearchParams(window.location.search)
	const urlToken = urlParams.get(DESKTOP_API_TOKEN_PARAM)?.trim()
	if (urlToken) {
		window.sessionStorage.setItem(DESKTOP_API_TOKEN_STORAGE_KEY, urlToken)
		return urlToken
	}

	// 打包后的桌面端：原生宿主只在窗口 URL 中放入一次性 nonce，后端校验并消费
	// 该 nonce 后才通过会话 Cookie 把 token 交给当前 webview。
	const cookieToken = getCookieValue(DESKTOP_API_TOKEN_COOKIE)?.trim()
	if (cookieToken) {
		window.sessionStorage.setItem(DESKTOP_API_TOKEN_STORAGE_KEY, cookieToken)
		return cookieToken
	}

	// 同一 webview 内后续导航的缓存。Cookie 优先，避免沿用上一次启动的旧 token。
	return window.sessionStorage.getItem(DESKTOP_API_TOKEN_STORAGE_KEY)?.trim() || null
}

export const hasDesktopApiToken = () => Boolean(getDesktopApiToken())

export const getDesktopApiHeaders = (): Record<string, string> => {
	const token = getDesktopApiToken()
	if (!token) {
		return {}
	}
	return {
		'X-Hephaestus-Desktop-Token': token,
	}
}

// 获取API基础URL
export const getApiBaseURL = () => {
	if (typeof window === 'undefined') {
		// SSR 环境：返回默认值
		return envConfig.IS_DEV
			? `http://localhost:${envConfig.BACKEND_PORT}`
			: 'http://localhost'
	}

	// 优先使用URL参数中的后端端口（适用于开发环境）
	const urlBackendPort = getBackendPortFromURL()

	if (envConfig.IS_DEV && urlBackendPort) {
		// 开发环境 + URL参数指定端口
		return `${window.location.protocol}//${window.location.hostname}:${urlBackendPort}`
	} else if (envConfig.IS_DEV) {
		// 开发环境：使用配置的后端端口
		return `${window.location.protocol}//${window.location.hostname}:${envConfig.BACKEND_PORT}`
	} else {
		// 生产环境：使用当前端口（前后端同端口）
		return `${window.location.protocol}//${window.location.host}`
	}
}

// 获取WebSocket基础URL（与getApiBaseURL逻辑一致，只是协议不同）
export const getWebSocketBaseURL = () => {
	if (typeof window === 'undefined') {
		// SSR 环境：返回默认值
		return envConfig.IS_DEV
			? `ws://localhost:${envConfig.BACKEND_PORT}`
			: 'ws://localhost'
	}

	// 获取 WebSocket 协议（根据当前页面协议）
	const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

	// 优先使用URL参数中的后端端口（适用于开发环境）
	const urlBackendPort = getBackendPortFromURL()

	if (envConfig.IS_DEV && urlBackendPort) {
		// 开发环境 + URL参数指定端口
		return `${wsProtocol}//${window.location.hostname}:${urlBackendPort}`
	} else if (envConfig.IS_DEV) {
		// 开发环境：使用配置的后端端口
		return `${wsProtocol}//${window.location.hostname}:${envConfig.BACKEND_PORT}`
	} else {
		// 生产环境：使用当前端口（前后端同端口）
		return `${wsProtocol}//${window.location.host}`
	}
}
