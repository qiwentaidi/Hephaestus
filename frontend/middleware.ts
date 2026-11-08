import { NextResponse } from 'next/server'
import type { NextRequest } from 'next/server'

// 需要保护的路由（需要登录才能访问）
const protectedRoutes = [
  '/dashboard',
  '/scan',
  '/poc-manager',
  '/asset',
  '/space',
  '/tools',
  '/settings',
]

// 公开路由（无需登录）
const publicRoutes = ['/']

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl

  // 检查是否是受保护的路由
  const isProtectedRoute = protectedRoutes.some(
    route => pathname === route || pathname.startsWith(route + '/')
  )

  // 检查是否是公开路由
  const isPublicRoute = publicRoutes.includes(pathname)

  // 从 cookie 中获取 token（更安全的方式）
  // 由于当前使用 localStorage，我们需要在客户端设置一个 cookie
  const authToken = request.cookies.get('auth_token')?.value

  // 如果是受保护的路由且没有 token，重定向到登录页
  if (isProtectedRoute && !authToken) {
    const loginUrl = new URL('/', request.url)
    // 保存原始请求路径，登录后可以重定向回来
    loginUrl.searchParams.set('redirect', pathname)
    return NextResponse.redirect(loginUrl)
  }

  // 注意：不再自动重定向已登录用户到 dashboard
  // 由登录页恢复会话或检查初始化状态

  return NextResponse.next()
}

// 配置 middleware 匹配的路由
export const config = {
  matcher: [
    /*
     * 匹配所有路由除了:
     * - api (API routes)
     * - _next/static (static files)
     * - _next/image (image optimization files)
     * - favicon.ico (favicon file)
     * - 静态资源
     */
    '/((?!api|_next/static|_next/image|favicon.ico|icon.png|.*\\.png$|.*\\.jpg$|.*\\.svg$).*)',
  ],
}
