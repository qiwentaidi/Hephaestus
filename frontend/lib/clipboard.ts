/**
 * 剪贴板工具函数（惰性函数版本）
 * 第一次调用时做能力检测并重写实现，后续调用直接走最优路径
 */

export let copyToClipboard: (text: string) => Promise<boolean> = async (text: string) => {
  // 第一次调用：做能力检测，然后把 copyToClipboard 重写成最终实现
  // 这里的“最优”指：尽量保持同步、绑定用户手势，复制成功率更高（类似 clipboard.js 的策略）
  const impl = chooseLazyImpl()
  copyToClipboard = impl
  return impl(text)
}

function hasDom(): boolean {
  return typeof window !== 'undefined' && typeof document !== 'undefined'
}

function canExecCommandCopy(): boolean {
  if (!hasDom()) return false
  if (typeof document.execCommand !== 'function') return false
  if (typeof document.queryCommandSupported === 'function') {
    try {
      return document.queryCommandSupported('copy')
    } catch {
      return true
    }
  }
  return true
}

function canClipboardApi(): boolean {
  if (!hasDom()) return false
  // Clipboard API 在非安全上下文经常不可用/报错
  if (typeof window.isSecureContext === 'boolean' && !window.isSecureContext) return false
  return !!navigator.clipboard && typeof navigator.clipboard.writeText === 'function'
}

function warn(code: string, extra?: Record<string, unknown>) {
  try {
    console.warn('[clipboard]', code, {
      secureContext: typeof window !== 'undefined' ? window.isSecureContext : undefined,
      protocol: typeof window !== 'undefined' ? window.location?.protocol : undefined,
      visibility: typeof document !== 'undefined' ? document.visibilityState : undefined,
      hasFocus: typeof document !== 'undefined' && typeof document.hasFocus === 'function' ? document.hasFocus() : undefined,
      userActivation: (navigator as any)?.userActivation
        ? {
            isActive: (navigator as any).userActivation.isActive,
            hasBeenActive: (navigator as any).userActivation.hasBeenActive,
          }
        : undefined,
      ...extra,
    })
  } catch {
    // ignore
  }
}

function chooseLazyImpl(): (text: string) => Promise<boolean> {
  // 优先 execCommand-first：同步路径更不容易丢失用户手势
  if (canExecCommandCopy()) {
    return copyExecFirst
  }
  // execCommand 不可用时再用 Clipboard API（只在 secure context）
  if (canClipboardApi()) {
    return copyWithClipboardApi
  }
  return copyWithExecCommandAsync
}

/**
 * 现代 Clipboard API 实现
 */
async function copyWithClipboardApi(text: string): Promise<boolean> {
  if (!text) return true

  try {
    return navigator.clipboard.writeText(text).then(() => true).catch((err) => {
      // 可能权限/非安全上下文/用户手势等失败
      warn('clipboard_api_failed', { name: (err as any)?.name, message: (err as any)?.message })
      return false
    })
  } catch (err) {
    warn('clipboard_api_threw', { message: (err as any)?.message })
    return false
  }
}

/**
 * execCommand 回退实现（包装成 Promise，与主函数统一签名）
 */
async function copyWithExecCommandAsync(text: string): Promise<boolean> {
  if (!text) return true
  return copyWithExecCommand(text)
}

/**
 * execCommand-first：更贴近 clipboard.js，尽量在用户手势回调的同步路径完成复制。
 * 只有在 execCommand 失败时，才尝试 Clipboard API（如果可用）。
 */
async function copyExecFirst(text: string): Promise<boolean> {
  if (!text) return true
  const ok = copyWithExecCommand(text)
  if (ok) return true
  if (canClipboardApi()) return copyWithClipboardApi(text)
  return false
}

/**
 * 使用 execCommand 复制文本（回退方案）
 */
function copyWithExecCommand(text: string): boolean {
  if (!hasDom()) return false
  if (!document.body) return false
  if (typeof document.execCommand !== 'function') {
    warn('execCommand_unavailable')
    return false
  }

  // 通过 copy 事件直接写入 clipboardData，比“选中文本再 copy”更稳（尤其在一些组件/菜单中）
  let copiedByEvent = false
  const onCopy = (e: ClipboardEvent) => {
    try {
      if (!e.clipboardData) return
      e.clipboardData.setData('text/plain', text)
      e.preventDefault()
      copiedByEvent = true
    } catch (err: any) {
      warn('copy_event_setData_failed', { message: err?.message, name: err?.name })
    }
  }

  document.addEventListener('copy', onCopy as any)

  const textarea = document.createElement('textarea')

  // 对齐 clipboard.js 的做法：放到屏幕外，top 跟随当前 scroll，避免页面滚动跳动
  textarea.style.fontSize = '12pt'
  textarea.style.border = '0'
  textarea.style.padding = '0'
  textarea.style.margin = '0'
  textarea.style.position = 'absolute'
  const dir = document.documentElement?.getAttribute('dir')
  if (dir === 'rtl') textarea.style.right = '-9999px'
  else textarea.style.left = '-9999px'
  const y = window.pageYOffset || document.documentElement?.scrollTop || 0
  textarea.style.top = `${y}px`

  // iOS 下加 readonly 可降低弹键盘概率
  textarea.setAttribute('readonly', '')
  textarea.value = text
  document.body.appendChild(textarea)

  try {
    textarea.select()
    textarea.setSelectionRange(0, text.length)
    const execOk = document.execCommand('copy')
    if (execOk && !copiedByEvent) {
      // 有些情况下 execCommand 返回 true，但 copy 事件未触发/未写入，容易产生“提示成功但实际未复制”的假象
      warn('execCommand_ok_but_no_copy_event')
    }
    if (!execOk) warn('execCommand_returned_false')
    return copiedByEvent || execOk
  } catch (err) {
    warn('execCommand_threw', { message: (err as any)?.message, name: (err as any)?.name })
    return false
  } finally {
    document.body.removeChild(textarea)
    document.removeEventListener('copy', onCopy as any)
  }
}

/**
 * 检测浏览器是否支持现代 Clipboard API
 */
export function isClipboardApiSupported(): boolean {
  return canClipboardApi()
}
