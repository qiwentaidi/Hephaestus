'use client'

import { useState, useEffect } from 'react'
import Image from 'next/image'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Field,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/ui/password-input'
import { AlertCircle, CheckCircle2 } from 'lucide-react'
import { isDesktopRuntime } from '@/lib/desktop'
import { getApiBaseURL } from '@/lib/env'
import { restoreDesktopToken, setToken, clearToken, persistDesktopConfigItems } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

type Mode = 'login' | 'create-admin'

const AUTH_RETRY_STATUSES = new Set([408, 425, 429, 500, 502, 503, 504])

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

export function LoginForm({
  className,
  ...props
}: React.ComponentProps<'div'>) {
  const { t } = useI18n()
  const authAsideClassName =
    'relative hidden h-full flex-col items-center justify-center gap-10 border-l p-10 md:flex ' +
    'border-slate-200/80 bg-[linear-gradient(180deg,rgba(255,255,255,0.88),rgba(238,244,255,0.84))] ' +
    'dark:border-border/60 dark:bg-gradient-to-br dark:from-slate-900/80 dark:via-slate-800/60 dark:to-slate-900/70'
  const authAsideOverlayClassName =
    'absolute inset-0 pointer-events-none opacity-100 dark:opacity-70'
  const authAsideOrbTopClassName =
    'absolute -top-20 -right-24 h-64 w-64 rounded-full bg-sky-400/18 blur-[80px] dark:bg-primary/20'
  const authAsideOrbBottomClassName =
    'absolute -bottom-24 -left-16 h-72 w-72 rounded-full bg-indigo-300/22 blur-[90px] dark:bg-blue-500/20'
  const authAsideLogoClassName =
    'flex h-14 w-14 items-center justify-center rounded-2xl border shadow-lg ' +
    'border-slate-200/80 bg-white/85 shadow-slate-200/70 dark:border-white/15 dark:bg-white/10 dark:shadow-black/20'
  const authAsideTitleClassName = 'text-2xl font-semibold text-slate-900 dark:text-foreground'
  const authAsideDescClassName = 'mt-1 text-sm text-slate-600 dark:text-muted-foreground'
  const authAsideFeatureClassName =
    'flex items-center gap-3 rounded-2xl border px-5 py-4 ' +
    'border-slate-200/80 bg-white/55 shadow-sm shadow-slate-200/40 ' +
    'dark:border-white/10 dark:bg-white/5 dark:shadow-none'
  const authAsideFeatureTextClassName = 'text-base text-slate-700 dark:text-foreground/90'

  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const [mode, setMode] = useState<Mode | null>(null)
  
  const [loginData, setLoginData] = useState({
    username: '',
    password: ''
  })
  
  const [adminData, setAdminData] = useState({
    username: '',
    password: '',
    confirmPassword: ''
  })

  const [desktopMode, setDesktopMode] = useState(false)
  const [forgotPasswordDialogOpen, setForgotPasswordDialogOpen] = useState(false)
  useEffect(() => {
    void isDesktopRuntime().then(setDesktopMode).catch(() => setDesktopMode(false))
  }, [])

  useEffect(() => {
    const checkStatus = async () => {
      try {
        const apiBase = getApiBaseURL()

        const existingToken = localStorage.getItem('auth_token') || await restoreDesktopToken()
        if (existingToken) {
          let lastStatus = 0
          let shouldClearToken = false

          for (let attempt = 0; attempt < 6; attempt++) {
            try {
              const userResponse = await fetch(`${apiBase}/api/v1/auth/user`, {
                headers: {
                  'Authorization': `Bearer ${existingToken}`
                },
                cache: 'no-store',
              })

              if (userResponse.ok) {
                // 确保 cookie 与 localStorage 同步，避免 middleware 读不到 token 导致 /dashboard <-> / 重定向循环
                setToken(existingToken)
                // token 有效，直接跳转到 dashboard
                window.location.href = '/dashboard'
                return
              }

              lastStatus = userResponse.status
              if (userResponse.status === 401 || userResponse.status === 403) {
                shouldClearToken = true
                break
              }

              if (!AUTH_RETRY_STATUSES.has(userResponse.status) || attempt === 5) {
                break
              }
            } catch (error) {
              if (attempt === 5) {
                throw error
              }
            }

            await sleep(400)
          }

          if (shouldClearToken) {
            clearToken()
          } else if (lastStatus !== 0) {
            console.warn('Keep local token and retry login restoration after backend stabilizes', lastStatus)
          }
        }

        // 检查系统是否初始化
        const response = await fetch(`${apiBase}/api/v1/auth/check`)
        const data = await response.json()

        if (data.success) {
          setMode(data.initialized ? 'login' : 'create-admin')
        } else {
          // 如果API调用失败，默认显示登录
          setMode('login')
        }
      } catch (error) {
        console.error('检查状态失败:', error)
        setMode('login')
        setError(t.auth.errors.serverUnavailable)
      }
    }

    checkStatus()
  }, [])

  const handleLoginSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setIsLoading(true)
    setError('')
    
    try {
      const apiBase = getApiBaseURL()
      const response = await fetch(`${apiBase}/api/v1/auth/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          username: loginData.username,
          password: loginData.password,
        }),
      })
      
      const data = await response.json()
      
      if (data.success && data.data) {
        // 保存 JWT token 和用户数据到 localStorage
        if (data.data.token) {
          setToken(data.data.token)
        }
        localStorage.setItem('userData', JSON.stringify(data.data))
        await persistDesktopConfigItems({
          userData: JSON.stringify(data.data),
        })
        window.location.href = '/dashboard'
      } else {
        setError(data.error || t.auth.errors.loginFailed)
      }
    } catch (error) {
      console.error('Login request failed:', error)
      setError(t.auth.errors.serverUnavailable)
    } finally {
      setIsLoading(false)
    }
  }

  // 验证密码复杂性
  const validatePassword = (password: string): { valid: boolean; message: string } => {
    if (password.length < 8) {
      return { valid: false, message: t.auth.errors.passwordMinLength }
    }
    if (!/[A-Z]/.test(password)) {
      return { valid: false, message: t.auth.errors.passwordUppercase }
    }
    if (!/[a-z]/.test(password)) {
      return { valid: false, message: t.auth.errors.passwordLowercase }
    }
    if (!/[0-9]/.test(password)) {
      return { valid: false, message: t.auth.errors.passwordNumber }
    }
    return { valid: true, message: '' }
  }

  const handleAdminSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    setIsLoading(true)
    setError('')
    
    if (adminData.password !== adminData.confirmPassword) {
      setError(t.auth.errors.passwordMismatch)
      setIsLoading(false)
      return
    }
    
    // 验证密码复杂性
    const validation = validatePassword(adminData.password)
    if (!validation.valid) {
      setError(validation.message)
      setIsLoading(false)
      return
    }
    
    try {
      const apiBase = getApiBaseURL()
      const response = await fetch(`${apiBase}/api/v1/auth/init`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          username: adminData.username,
          password: adminData.password,
        }),
      })
      
      const data = await response.json()
      
      if (data.success && data.data) {
        // 保存 JWT token 和用户数据到 localStorage
        if (data.data.token) {
          setToken(data.data.token)
        }
        localStorage.setItem('userData', JSON.stringify(data.data))
        await persistDesktopConfigItems({
          userData: JSON.stringify(data.data),
        })
        window.location.href = '/dashboard'
      } else {
        setError(data.error || t.auth.errors.createAdminFailed)
      }
    } catch (error) {
      console.error('Create admin request failed:', error)
      setError(t.auth.errors.serverUnavailable)
    } finally {
      setIsLoading(false)
    }
  }

  const handleForgotPassword = () => {
    setForgotPasswordDialogOpen(true)
  }

  // 加载状态
  if (mode === null) {
    return (
      <div className={cn('flex flex-col gap-6 w-full max-w-4xl', className)} {...props}>
        <Card className="overflow-hidden p-0">
          <CardContent className="flex items-center justify-center py-12">
            <div className="flex items-center gap-3 text-muted-foreground">
              <span className="w-5 h-5 border-2 border-muted-foreground/30 border-t-muted-foreground rounded-full animate-spin" />
              <span>{t.auth.loading.checkingStatus}</span>
            </div>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (mode === 'login') {
    return (
      <div className={cn('flex flex-col gap-6 w-full max-w-4xl', className)} {...props}>
        <Card className="overflow-hidden border-white/70 bg-white/88 p-0 shadow-2xl shadow-slate-300/25 backdrop-blur dark:border-border/60 dark:bg-background/95 dark:shadow-black/20">
          <CardContent className="grid p-0 md:grid-cols-2">
            <form className="p-6 md:p-8" onSubmit={handleLoginSubmit}>
              <FieldGroup>
                <div className="flex flex-col items-center gap-2 text-center">
                  <h1 className="text-2xl font-bold">{t.auth.login.title}</h1>
                  <p className="text-muted-foreground text-balance">
                    {t.auth.login.description}
                  </p>
                </div>
                <Field>
                  <FieldLabel htmlFor="login-username">{t.auth.login.username}</FieldLabel>
                  <Input
                    id="login-username"
                    type="text"
                    placeholder={t.auth.login.usernamePlaceholder}
                    className="bg-background/60 border-border/60 focus-visible:ring-0 focus:border-primary"
                    value={loginData.username}
                    onChange={(e) => setLoginData({ ...loginData, username: e.target.value })}
                    required
                  />
                </Field>
                <Field>
                  <div className="flex items-center">
                    <FieldLabel htmlFor="login-password">{t.auth.login.password}</FieldLabel>
                    <button
                      type="button"
                      onClick={handleForgotPassword}
                      className="ml-auto text-sm underline-offset-2 hover:underline"
                    >
                      {t.auth.login.forgotPassword}
                    </button>
                  </div>
                  <PasswordInput
                    id="login-password"
                    placeholder={t.auth.login.passwordPlaceholder}
                    className="w-full"
                    inputClassName="bg-background/60 border-border/60 focus-visible:ring-0 focus:border-primary"
                    value={loginData.password}
                    onChange={(e) => setLoginData({ ...loginData, password: e.target.value })}
                    required
                  />
                </Field>
                {error && (
                  <Field>
                    <div className="flex items-start gap-2 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                      <AlertCircle className="w-4 h-4 text-destructive shrink-0 mt-0.5" />
                      <p className="text-sm text-destructive">{error}</p>
                    </div>
                  </Field>
                )}
                <Field>
                  <Button type="submit" className="w-full" disabled={isLoading}>
                    {isLoading ? (
                      <span className="flex items-center gap-2">
                        <span className="w-4 h-4 border-2 border-muted-foreground/30 border-t-muted-foreground rounded-full animate-spin" />
                        {t.auth.login.loggingIn}
                      </span>
                    ) : (
                      t.auth.login.submit
                    )}
                  </Button>
                </Field>
              </FieldGroup>
            </form>
            <div className={authAsideClassName}>
              <div className={authAsideOverlayClassName} aria-hidden="true">
                <div className={authAsideOrbTopClassName} />
                <div className={authAsideOrbBottomClassName} />
              </div>
              <div className="relative z-10 flex flex-col items-center gap-4 text-center">
                <div className={authAsideLogoClassName}>
                  <Image src="/brand-mark.png" alt="Hephaestus" width={40} height={40} unoptimized className="h-10 w-10" />
                </div>
                <div>
                  <h2 className={authAsideTitleClassName}>Hephaestus</h2>
                </div>
              </div>
              <div className="relative z-10 w-full space-y-4">
                <div className={authAsideFeatureClassName}>
                  <CheckCircle2 className="w-5 h-5 text-primary" />
                  <span className={authAsideFeatureTextClassName}>
                    {t.auth.aside.pentestFeature}
                  </span>
                </div>
                <div className={authAsideFeatureClassName}>
                  <CheckCircle2 className="w-5 h-5 text-primary" />
                  <span className={authAsideFeatureTextClassName}>
                    {t.auth.aside.assetFeature}
                  </span>
                </div>
                <div className={authAsideFeatureClassName}>
                  <CheckCircle2 className="w-5 h-5 text-primary" />
                  <span className={authAsideFeatureTextClassName}>
                    {t.auth.aside.scanFeature}
                  </span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
        <AlertDialog open={forgotPasswordDialogOpen} onOpenChange={setForgotPasswordDialogOpen}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t.auth.messages.forgotPasswordTitle}</AlertDialogTitle>
              <AlertDialogDescription className="space-y-3">
                <p>
                  {desktopMode
                    ? t.auth.messages.forgotPasswordDescription
                    : t.auth.messages.forgotPasswordWebDescription}
                </p>
                <div>
                  <p className="font-medium text-foreground">
                    {desktopMode
                      ? t.auth.messages.forgotPasswordDirectoryLabel
                      : t.auth.messages.forgotPasswordWebDatabaseLabel}
                  </p>
                  <p className="break-all font-mono text-xs text-foreground/90">
                    {desktopMode ? '~/.config/hephaestus/data' : t.auth.messages.forgotPasswordWebDatabaseValue}
                  </p>
                </div>
                <div>
                  <p>{t.auth.messages.forgotPasswordUserTableHint}</p>
                </div>
                <div>
                  <p className="font-medium text-foreground">{t.auth.messages.forgotPasswordResetValueLabel}</p>
                  <p className="break-all font-mono text-xs text-foreground/90">
                    98d738abd01713fc6497c05c62c09f5bf487907d4f8b20124fba83a447cee033
                  </p>
                </div>
                <div>
                  <p className="font-medium text-foreground">{t.auth.messages.forgotPasswordPlaintextLabel}</p>
                  <p className="break-all font-mono text-xs text-foreground/90">Aa123456.</p>
                </div>
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogAction>{t.common.confirm}</AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
    )
  }

  return (
    <div className={cn('flex flex-col gap-6 w-full max-w-4xl', className)} {...props}>
      <Card className="overflow-hidden p-0">
        <CardContent className="grid p-0 md:grid-cols-2">
          <form onSubmit={handleAdminSubmit} className="p-6 md:p-8">
            <FieldGroup>
              <div className="flex flex-col items-center gap-2 text-center">
                <h1 className="text-2xl font-bold">{t.auth.init.title}</h1>
                <p className="text-muted-foreground text-balance">
                  {t.auth.init.description}
                </p>
              </div>
              <Field>
                <FieldLabel htmlFor="username">{t.auth.init.adminUsername}</FieldLabel>
                <Input
                  id="username"
                  type="text"
                  placeholder={t.auth.login.usernamePlaceholder}
                  className="bg-background/60 border-border/60 focus-visible:ring-0 focus:border-primary"
                  value={adminData.username}
                  onChange={(e) => setAdminData({ ...adminData, username: e.target.value })}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="password">{t.auth.login.password}</FieldLabel>
                <PasswordInput
                  id="password"
                  placeholder={t.auth.init.passwordPlaceholder}
                  className="w-full"
                  inputClassName="bg-background/60 border-border/60 focus-visible:ring-0 focus:border-primary"
                  value={adminData.password}
                  onChange={(e) => setAdminData({ ...adminData, password: e.target.value })}
                  required
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="confirm-password">{t.auth.init.confirmPassword}</FieldLabel>
                <PasswordInput
                  id="confirm-password"
                  placeholder={t.auth.init.confirmPasswordPlaceholder}
                  className="w-full"
                  inputClassName="bg-background/60 border-border/60 focus-visible:ring-0 focus:border-primary"
                  value={adminData.confirmPassword}
                  onChange={(e) => setAdminData({ ...adminData, confirmPassword: e.target.value })}
                  required
                />
              </Field>
              {error && (
                <Field>
                  <div className="flex items-start gap-2 p-3 rounded-lg bg-destructive/10 border border-destructive/20">
                    <AlertCircle className="w-4 h-4 text-destructive shrink-0 mt-0.5" />
                    <p className="text-sm text-destructive">{error}</p>
                  </div>
                </Field>
              )}
              <Field>
                <Button type="submit" className="w-full" disabled={isLoading}>
                  {isLoading ? (
                    <span className="flex items-center gap-2">
                      <span className="w-4 h-4 border-2 border-muted-foreground/30 border-t-muted-foreground rounded-full animate-spin" />
                      {t.auth.init.creating}
                    </span>
                  ) : (
                    t.auth.init.submit
                  )}
                </Button>
              </Field>
            </FieldGroup>
          </form>
          <div className="bg-muted relative hidden md:block">
            <Image
              src="/placeholder.svg"
              alt={t.auth.init.imageAlt}
              fill
              sizes="50vw"
              unoptimized
              className="absolute inset-0 h-full w-full object-cover dark:brightness-[0.2] dark:grayscale"
            />
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
