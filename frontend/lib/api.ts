// 统一 API 请求封装，支持 JWT 鉴权
import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from './env'

// 重新导出 getApiBaseURL，方便统一从 lib/api 导入
export { getApiBaseURL }

// Token 存储 key
const TOKEN_KEY = 'auth_token'
const USER_DATA_KEY = 'userData'
const TOKEN_COOKIE_MAX_AGE = 60 * 60 * 24 * 30
const getDesktopEndpoint = (path: string) => `${getApiBaseURL()}${path}`
const isDesktopAPIRequest = (url: string) => url.includes('/api/v1/desktop/')

async function persistDesktopToken(token: string): Promise<void> {
  if (!hasDesktopApiToken()) return
  try {
    await fetch(getDesktopEndpoint('/api/v1/desktop/auth/session'), {
      method: 'PUT',
      keepalive: true,
      headers: {
        'Content-Type': 'application/json',
        ...getDesktopApiHeaders(),
      },
      body: JSON.stringify({ token }),
    })
  } catch {
    // ignore in web mode or when desktop backend is unavailable
  }
}

async function clearDesktopToken(): Promise<void> {
  if (!hasDesktopApiToken()) return
  try {
    await fetch(getDesktopEndpoint('/api/v1/desktop/auth/session'), {
      method: 'DELETE',
      keepalive: true,
      headers: getDesktopApiHeaders(),
    })
  } catch {
    // ignore in web mode or when desktop backend is unavailable
  }
}

export async function persistDesktopConfigItems(items: Record<string, string>): Promise<void> {
  if (!hasDesktopApiToken()) return
  try {
    if (Object.keys(items).length === 0) return
    await fetch(getDesktopEndpoint('/api/v1/desktop/storage'), {
      method: 'PUT',
      keepalive: true,
      headers: {
        'Content-Type': 'application/json',
        ...getDesktopApiHeaders(),
      },
      body: JSON.stringify({ items }),
    })
  } catch {
    // ignore in web mode or when desktop backend is unavailable
  }
}

export async function removeDesktopConfigItems(keys: string[]): Promise<void> {
  if (!hasDesktopApiToken()) return
  try {
    if (keys.length === 0) return
    await fetch(getDesktopEndpoint('/api/v1/desktop/storage'), {
      method: 'DELETE',
      keepalive: true,
      headers: {
        'Content-Type': 'application/json',
        ...getDesktopApiHeaders(),
      },
      body: JSON.stringify({ keys }),
    })
  } catch {
    // ignore in web mode or when desktop backend is unavailable
  }
}

export async function restoreDesktopToken(): Promise<string | null> {
  if (!hasDesktopApiToken()) return null
  try {
    const response = await fetch(getDesktopEndpoint('/api/v1/desktop/auth/session'), {
      method: 'GET',
      cache: 'no-store',
      headers: getDesktopApiHeaders(),
    })
    if (!response.ok) {
      return null
    }
    const payload = await response.json().catch(() => null)
    const token = payload?.data?.token
    if (typeof token === 'string' && token.trim()) {
      return token.trim()
    }
  } catch {
    // ignore in web mode or when desktop backend is unavailable
  }
  return null
}

// 获取存储的 token
export function getToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem(TOKEN_KEY)
}

// 设置 token（同时设置 localStorage 和 cookie，cookie 用于 middleware 验证）
export function setToken(token: string): void {
  if (typeof window === 'undefined') return
  localStorage.setItem(TOKEN_KEY, token)
  // 设置 cookie，用于 middleware 路由守卫
  document.cookie = `${TOKEN_KEY}=${token}; path=/; max-age=${TOKEN_COOKIE_MAX_AGE}; SameSite=Lax`
  void persistDesktopToken(token)
  void persistDesktopConfigItems({ [TOKEN_KEY]: token })
}

// 清除 token（同时清除 localStorage 和 cookie）
export function clearToken(): void {
  if (typeof window === 'undefined') return
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_DATA_KEY)
  // 清除 cookie
  document.cookie = `${TOKEN_KEY}=; path=/; max-age=0; SameSite=Lax`
  void clearDesktopToken()
  void removeDesktopConfigItems([TOKEN_KEY, USER_DATA_KEY])
}

// 检查是否已登录
export function isAuthenticated(): boolean {
  return !!getToken()
}

// 退出登录
export function logout(): void {
  clearToken()
  window.location.href = '/'
}

// API 响应类型
export interface ApiResponse<T = any> {
  success: boolean
  data?: T
  error?: string
  message?: string
}

// 请求配置
export interface RequestConfig extends RequestInit {
  skipAuth?: boolean // 是否跳过鉴权
  timeout?: number   // 超时时间（毫秒）
}

// 统一请求方法
export async function request<T = any>(
  url: string,
  config: RequestConfig = {}
): Promise<ApiResponse<T>> {
  if (isDesktopAPIRequest(url) && !hasDesktopApiToken()) {
    return { success: false, error: 'Desktop API is unavailable in web mode' }
  }
  const { skipAuth = false, timeout = 30000, ...fetchConfig } = config
  const baseURL = getApiBaseURL()
  const fullURL = url.startsWith('http') ? url : `${baseURL}${url}`

  // 构建请求头
  const headers = new Headers(fetchConfig.headers || {})

  // 添加默认 Content-Type
  if (!headers.has('Content-Type') && fetchConfig.body) {
    headers.set('Content-Type', 'application/json')
  }

  // 添加 Authorization 头
  if (!skipAuth) {
    const token = getToken()
    if (token) {
      headers.set('Authorization', `Bearer ${token}`)
    }
  }

  if (isDesktopAPIRequest(url)) {
    for (const [key, value] of Object.entries(getDesktopApiHeaders())) {
      headers.set(key, value)
    }
  }

  // 创建超时控制
  const controller = new AbortController()
  const timeoutId = setTimeout(() => controller.abort(), timeout)

  try {
    const response = await fetch(fullURL, {
      ...fetchConfig,
      headers,
      signal: controller.signal,
    })

    clearTimeout(timeoutId)

    // 处理 401 未授权
    if (response.status === 401) {
      // 清除本地存储并跳转到登录页
      clearToken()
      // 只在浏览器环境且不在登录页时跳转
      if (typeof window !== 'undefined' && !window.location.pathname.match(/^\/?$/)) {
        window.location.href = '/'
      }
      return {
        success: false,
        error: '登录已过期，请重新登录',
      }
    }

    // 尝试解析 JSON 响应
    const contentType = response.headers.get('content-type')
    if (contentType && contentType.includes('application/json')) {
      const data = await response.json()
      return data
    }

    // 非 JSON 响应
    if (response.ok) {
      return {
        success: true,
        data: await response.text() as any,
      }
    }

    return {
      success: false,
      error: `HTTP error! status: ${response.status}`,
    }
  } catch (error: any) {
    clearTimeout(timeoutId)

    if (error.name === 'AbortError') {
      return {
        success: false,
        error: '请求超时',
      }
    }

    // 网络错误
    if (error.message?.includes('Failed to fetch') || error.message?.includes('NetworkError')) {
      return {
        success: false,
        error: '无法连接到服务器，请检查网络连接',
      }
    }

    return {
      success: false,
      error: error.message || '请求失败',
    }
  }
}

// GET 请求
export async function get<T = any>(
  url: string,
  config: RequestConfig = {}
): Promise<ApiResponse<T>> {
  return request<T>(url, { ...config, method: 'GET' })
}

// POST 请求
export async function post<T = any>(
  url: string,
  data?: any,
  config: RequestConfig = {}
): Promise<ApiResponse<T>> {
  return request<T>(url, {
    ...config,
    method: 'POST',
    body: data ? JSON.stringify(data) : undefined,
  })
}

// PUT 请求
export async function put<T = any>(
  url: string,
  data?: any,
  config: RequestConfig = {}
): Promise<ApiResponse<T>> {
  return request<T>(url, {
    ...config,
    method: 'PUT',
    body: data ? JSON.stringify(data) : undefined,
  })
}

// DELETE 请求
export async function del<T = any>(
  url: string,
  config: RequestConfig = {}
): Promise<ApiResponse<T>> {
  return request<T>(url, { ...config, method: 'DELETE' })
}

// 原始 fetch 请求（用于 SSE 等特殊场景）
export async function fetchWithAuth(
  url: string,
  config: RequestInit = {}
): Promise<Response> {
  if (isDesktopAPIRequest(url) && !hasDesktopApiToken()) {
    throw new Error('Desktop API is unavailable in web mode')
  }
  const baseURL = getApiBaseURL()
  const fullURL = url.startsWith('http') ? url : `${baseURL}${url}`

  const headers = new Headers(config.headers || {})
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (isDesktopAPIRequest(url)) {
    for (const [key, value] of Object.entries(getDesktopApiHeaders())) {
      headers.set(key, value)
    }
  }

  const response = await fetch(fullURL, {
    ...config,
    headers,
  })

  // 处理 401 未授权
  if (response.status === 401) {
    clearToken()
    // 只在浏览器环境且不在登录页时跳转
    if (typeof window !== 'undefined' && !window.location.pathname.match(/^\/?$/)) {
      window.location.href = '/'
    }
  }

  return response
}

// 导出便捷方法
export const api = {
  get,
  post,
  put,
  delete: del,
  request,
  fetchWithAuth,
  getToken,
  setToken,
  clearToken,
  isAuthenticated,
  logout,
}

export default api
