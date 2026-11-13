"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { Loader2, RotateCw } from "lucide-react"

import { Button } from "@/components/ui/button"
import { fetchWithAuth, getToken } from "@/lib/api"
import { hasDesktopApiToken } from "@/lib/env"
import { useI18n } from "@/lib/i18n"
import { cn } from "@/lib/utils"

type Phase = "checking" | "ready" | "building" | "failed"

const POLL_INTERVAL_MS = 700
const POLL_FAILURE_LIMIT = 5
const MAX_WAIT_SECONDS = 300
const FADE_OUT_MS = 260

/**
 * 桌面端首次启动（或模板更新后）的模板索引会转到后台构建。
 * 这里在窗口出现后阻塞主界面，把“准备模板库”的过程显式呈现给用户，
 * 索引就绪后淡出；构建失败时提供重试与退出。
 *
 * 仅在桌面 webview 且已登录时生效，Web 模式和设置/启动器子窗口不受影响。
 */
export function PocTemplateCacheGate({ children }: { children: React.ReactNode }) {
  const { t } = useI18n()
  const [phase, setPhase] = useState<Phase>("checking")
  const [errorMessage, setErrorMessage] = useState("")
  const [leaving, setLeaving] = useState(false)
  const [retrying, setRetrying] = useState(false)
  const [retryNonce, setRetryNonce] = useState(0)
  const shownLoaderRef = useRef(false)

  const finish = useCallback(() => {
    setLeaving(true)
    window.setTimeout(() => {
      setPhase("ready")
      setLeaving(false)
    }, FADE_OUT_MS)
  }, [])

  useEffect(() => {
    // 只在桌面端、已登录的主窗口生效。
    if (!hasDesktopApiToken() || !getToken()) {
      // Web 模式或未登录：首帧后立即撤掉遮罩（CSS 默认也会隐藏它）。
      const timer = window.setTimeout(() => setPhase("ready"), 0)
      return () => window.clearTimeout(timer)
    }
    // 登录后首次进入主界面时，内联脚本可能还没打上标记；这里补上，
    // 让首帧的检查遮罩立即生效。
    document.documentElement.classList.add("heph-desktop")

    let cancelled = false
    let timer: number | null = null
    let failures = 0

    const sleep = (ms: number) =>
      new Promise<void>((resolve) => {
        timer = window.setTimeout(resolve, ms)
      })

    const run = async () => {
      while (!cancelled) {
        try {
          const response = await fetchWithAuth("/api/v1/webscan/poc-cache/status", {
            cache: "no-store",
          })
          const payload = await response.json().catch(() => null)
          if (cancelled) return

          if (!response.ok || !payload?.success) {
            failures += 1
            if (failures >= POLL_FAILURE_LIMIT) {
              setPhase("failed")
              setErrorMessage(String(payload?.error || `HTTP ${response.status}`))
              return
            }
          } else {
            failures = 0
            const state = String(payload?.data?.state || "")
            const elapsedSeconds = Number(payload?.data?.elapsedSeconds) || 0

            if (state === "ready") {
              if (shownLoaderRef.current) {
                finish()
              } else {
                // 只是检查阶段：直接撤掉遮罩，不做淡出，尽量少占时间。
                setPhase("ready")
              }
              return
            }
            if (elapsedSeconds >= MAX_WAIT_SECONDS) {
              shownLoaderRef.current = true
              setPhase("failed")
              setErrorMessage(t.pocTemplateCache.timeout)
              return
            }
            if (state === "failed") {
              shownLoaderRef.current = true
              setPhase("failed")
              setErrorMessage(String(payload?.data?.error || ""))
              return
            }
            // idle 表示后台还没开始构建（或正在快速加载索引）：
            // 保持当前界面可用，不闪一下遮罩；真正开始重建时才显示。
            if (state === "building") {
              shownLoaderRef.current = true
              setPhase("building")
            }
          }
        } catch {
          failures += 1
          if (failures >= POLL_FAILURE_LIMIT) {
            setPhase("failed")
            setErrorMessage("")
            return
          }
        }

        await sleep(POLL_INTERVAL_MS)
      }
    }

    void run()

    return () => {
      cancelled = true
      if (timer !== null) {
        window.clearTimeout(timer)
      }
    }
  }, [finish, retryNonce, t.pocTemplateCache.timeout])

  const handleRetry = useCallback(async () => {
    setRetrying(true)
    try {
      await fetchWithAuth("/api/v1/webscan/poc-cache/warmup", { method: "POST" })
    } catch {
      // 失败时保持错误界面，用户可以再次重试。
    }
    setRetrying(false)
    setErrorMessage("")
    shownLoaderRef.current = true
    setPhase("building")
    setRetryNonce((value) => value + 1)
  }, [])

  const handleExit = useCallback(async () => {
    try {
      await fetchWithAuth("/api/v1/desktop/window/close", { method: "POST" })
    } catch {
      // 桌面接口不可用时退回浏览器关闭。
    }
    window.close()
  }, [])

  const overlayVisible = phase !== "ready"

  return (
    <>
      {children}
      {overlayVisible && (
        <div
          role="status"
          aria-live="polite"
          className={cn(
            "heph-poc-loading-overlay fixed inset-0 z-[1500] flex flex-col items-center justify-center gap-7 bg-background px-6 text-center transition-opacity duration-300",
            phase === "checking" && "heph-poc-checking-overlay",
            leaving ? "pointer-events-none opacity-0" : "opacity-100",
          )}
        >
          {phase === "failed" ? (
            <div className="relative w-full max-w-md space-y-4">
              <div className="space-y-2">
                <h1 className="text-xl font-semibold tracking-wide text-foreground">
                  {t.pocTemplateCache.errorTitle}
                </h1>
                <p className="mx-auto max-w-md text-sm leading-6 text-muted-foreground">
                  {t.pocTemplateCache.errorHint}
                </p>
              </div>
              {errorMessage ? (
                <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-md border border-border/70 bg-muted/40 p-3 text-left text-xs text-muted-foreground">
                  {errorMessage}
                </pre>
              ) : null}
              <div className="flex items-center justify-center gap-3">
                <Button variant="outline" onClick={handleExit} disabled={retrying}>
                  {t.pocTemplateCache.exit}
                </Button>
                <Button onClick={handleRetry} disabled={retrying}>
                  {retrying ? (
                    <Loader2 className="size-4 animate-spin" />
                  ) : (
                    <RotateCw className="size-4" />
                  )}
                  {t.pocTemplateCache.retry}
                </Button>
              </div>
            </div>
          ) : phase === "building" ? (
            <div className="relative flex flex-col items-center gap-6">
              <div className="heph-poc-eclipse" aria-hidden="true">
                <div className="heph-eclipse-loader">
                  <div className="heph-eclipse-blackhole">
                    <div className="heph-eclipse-circle" />
                    <div className="heph-eclipse-disc" />
                  </div>
                  <svg className="heph-eclipse-curve" viewBox="0 0 500 500" aria-hidden="true">
                    <path
                      id="heph-eclipse-curve-path"
                      d="M73.2,148.6c4-6.1,65.5-96.8,178.6-95.6c111.3,1.2,170.8,90.3,175.1,97"
                    />
                    <text>
                      <textPath href="#heph-eclipse-curve-path">loading...</textPath>
                    </text>
                  </svg>
                </div>
              </div>
              <p className="text-sm font-medium text-foreground">
                {t.pocTemplateCache.loading}
              </p>
            </div>
          ) : null}
        </div>
      )}
    </>
  )
}
