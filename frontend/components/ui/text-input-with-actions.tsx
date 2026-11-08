'use client'

import * as React from 'react'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { FileUp, Filter, Copy, X, AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { copyToClipboard } from '@/lib/clipboard'

export interface TextInputWithActionsProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  disabled?: boolean
  className?: string
  height?: string
  // 校验函数，返回错误信息，如果返回空字符串或null则表示校验通过
  validate?: (text: string) => string | null
  // 校验错误时的回调
  onValidationError?: (errors: string[]) => void
  // 是否显示文件上传按钮
  showFileUpload?: boolean
  // 是否显示去重按钮
  showDeduplicate?: boolean
  // 是否显示复制按钮
  showCopy?: boolean
  // 是否显示清空按钮
  showClear?: boolean
  // 文件上传回调
  onFileUpload?: (file: File) => void
  // 去重回调
  onDeduplicate?: (text: string) => void
  // 复制回调
  onCopy?: (text: string) => void
  // 清空回调
  onClear?: () => void
  footerLeft?: React.ReactNode
  footerRight?: React.ReactNode
  onKeyDown?: (event: React.KeyboardEvent<HTMLTextAreaElement>) => void
  onFocus?: () => void
  onBlur?: () => void
  onCompositionStart?: (event: React.CompositionEvent<HTMLTextAreaElement>) => void
  onCompositionEnd?: (event: React.CompositionEvent<HTMLTextAreaElement>) => void
}

export const TextInputWithActions = React.forwardRef<
  HTMLTextAreaElement,
  TextInputWithActionsProps
>(
  (
    {
      value,
      onChange,
      placeholder,
      disabled = false,
      className,
      height = '120px',
      validate,
      onValidationError,
      showFileUpload = true,
      showDeduplicate = true,
      showCopy = true,
      showClear = true,
      onFileUpload,
      onDeduplicate,
      onCopy,
      onClear,
      footerLeft,
      footerRight,
      onKeyDown,
      onFocus,
      onBlur,
      onCompositionStart,
      onCompositionEnd,
    },
    ref
  ) => {
    // 防抖定时器引用
    const debounceTimerRef = React.useRef<NodeJS.Timeout | null>(null)
    // 错误信息状态
    const [errorMessage, setErrorMessage] = React.useState<string | null>(null)

    const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newValue = e.target.value
      onChange(newValue)

      // 清除之前的定时器
      if (debounceTimerRef.current) {
        clearTimeout(debounceTimerRef.current)
      }

      // 如果输入为空，清除错误信息
      if (!newValue.trim()) {
        setErrorMessage(null)
        return
      }

      // 防抖：延迟 500ms 后执行校验
      debounceTimerRef.current = setTimeout(() => {
        if (validate) {
          try {
            const error = validate(newValue)
            if (error) {
              setErrorMessage(error)
              if (onValidationError) {
                onValidationError([error])
              }
            } else {
              setErrorMessage(null)
            }
          } catch (err) {
            console.error('Validation error:', err)
            setErrorMessage(null)
          }
        } else {
          setErrorMessage(null)
        }
      }, 500)
    }

    // 组件卸载时清理定时器
    React.useEffect(() => {
      return () => {
        if (debounceTimerRef.current) {
          clearTimeout(debounceTimerRef.current)
        }
      }
    }, [])

    const handleFileUpload = () => {
      const input = document.createElement('input')
      input.type = 'file'
      input.accept = '.txt'
      input.onchange = (e) => {
        const file = (e.target as HTMLInputElement).files?.[0]
        if (file) {
          if (onFileUpload) {
            onFileUpload(file)
          } else {
            // 默认处理
            const reader = new FileReader()
            reader.onload = (event) => {
              const text = event.target?.result as string
              onChange(text)
            }
            reader.readAsText(file)
          }
        }
      }
      input.click()
    }

    const handleDeduplicate = () => {
      const lines = value.split('\n').filter((line) => line.trim())
      const unique = Array.from(new Set(lines))
      const newValue = unique.join('\n')
      onChange(newValue)
      if (onDeduplicate) {
        onDeduplicate(newValue)
      }
    }

    const handleCopy = () => {
      copyToClipboard(value)
      if (onCopy) {
        onCopy(value)
      }
    }

    const handleClear = () => {
      onChange('')
      if (onClear) {
        onClear()
      }
    }

    const [isFocused, setIsFocused] = React.useState(false)
    const actionControlClass =
      'rounded-lg border border-[var(--input-panel-action-border)] bg-[var(--input-panel-action-bg)] p-2 transition-colors hover:bg-[var(--input-panel-action-bg-hover)] disabled:cursor-not-allowed disabled:opacity-50'

    return (
      <div className={cn('relative max-w-full w-full', className)}>
        <div 
          data-slot="text-input-with-actions-shell"
          className={cn(
            "relative flex flex-col overflow-hidden rounded-xl border border-[var(--input-panel-border)] bg-[var(--input-panel-bg)] transition-all hover:bg-[var(--input-panel-bg-hover)]",
            isFocused && "border-ring ring-ring/50 ring-[3px]"
          )}
        >
          {/* 文本输入区域 - 固定高度容器 */}
          <div
            data-slot="text-input-with-actions-editor"
            className="relative overflow-hidden rounded-t-xl rounded-b-none bg-transparent"
            style={{ height }}
          >
            <Textarea
              ref={ref}
              placeholder={placeholder}
              value={value}
              onChange={handleChange}
              onFocus={() => {
                setIsFocused(true)
                onFocus?.()
              }}
              onBlur={() => {
                setIsFocused(false)
                onBlur?.()
              }}
              onCompositionStart={(event) => {
                onCompositionStart?.(event)
              }}
              onCompositionEnd={(event) => {
                onCompositionEnd?.(event)
              }}
              onKeyDown={onKeyDown}
              className={cn(
                "w-full h-full rounded-t-xl rounded-b-none px-4 py-3 !bg-transparent !shadow-none border-0 outline-none resize-none focus-visible:ring-0 focus-visible:outline-none leading-[1.2] font-mono text-sm overflow-y-auto",
                errorMessage && "border-destructive"
              )}
              disabled={disabled}
            />
          </div>

          {/* 底部功能区 */}
          <div
            data-slot="text-input-with-actions-footer"
            className="h-12 rounded-b-xl bg-transparent px-3"
          >
            <div className="w-full h-full flex items-center justify-between gap-3">
              <div className="flex items-center gap-2 flex-wrap">
                {showFileUpload && (
                  <TooltipProvider>
                    <Tooltip>
                    <TooltipTrigger asChild>
                      <label
                        data-slot="text-input-with-actions-action"
                        className={cn('cursor-pointer', actionControlClass)}
                        onClick={handleFileUpload}
                      >
                        <FileUp className="w-4 h-4 text-muted-foreground hover:text-foreground transition-colors" />
                      </label>
                    </TooltipTrigger>
                    <TooltipContent>从文件导入</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}
                {footerLeft}
              </div>

              <div className="flex items-center gap-2">
                {showDeduplicate && (
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          data-slot="text-input-with-actions-action"
                          className={actionControlClass}
                          type="button"
                          onClick={handleDeduplicate}
                          disabled={disabled || !value.trim()}
                        >
                          <Filter className="w-4 h-4 text-muted-foreground hover:text-foreground transition-colors" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>去重</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}

                {showCopy && (
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          data-slot="text-input-with-actions-action"
                          className={actionControlClass}
                          type="button"
                          onClick={handleCopy}
                          disabled={disabled || !value.trim()}
                        >
                          <Copy className="w-4 h-4 text-muted-foreground hover:text-foreground transition-colors" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>复制</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}

                {showClear && (
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <button
                          data-slot="text-input-with-actions-action"
                          className={actionControlClass}
                          type="button"
                          onClick={handleClear}
                          disabled={disabled || !value.trim()}
                        >
                          <X className="w-4 h-4 text-muted-foreground hover:text-foreground transition-colors" />
                        </button>
                      </TooltipTrigger>
                      <TooltipContent>清空</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}

                {footerRight}
              </div>
            </div>
          </div>
        </div>
        
        {/* 错误提示区域 */}
        {errorMessage && (
          <Alert variant="destructive" className="mt-2">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription className="whitespace-pre-line text-sm">
              {errorMessage}
            </AlertDescription>
          </Alert>
        )}
      </div>
    )
  }
)

TextInputWithActions.displayName = 'TextInputWithActions'
