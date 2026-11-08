"use client"

import { useRef, useMemo, useEffect, useState, useCallback } from 'react'
import dynamic from "next/dynamic"
import { Send } from "lucide-react"
import { collectPrivacyMatches, offsetToPosition } from "./privacy-mask"
import { ResponseContextMenu } from "./context-menu"
import { useToast } from "@/hooks/use-toast"
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import type { ResponseSelectionSnapshot } from "./nuclei-template"

// 动态导入封装的 Monaco Editor 组件，避免 SSR 问题
const MonacoEditorWrapper = dynamic(
  () => import("./monaco-editor-wrapper").then((mod) => mod.MonacoEditorWrapper),
  {
    ssr: false,
    loading: () => (
      <div className="flex h-full items-center justify-center bg-code-bg">
        <div className="text-sm text-muted-foreground">Loading editor...</div>
      </div>
    ),
  }
)

interface ResponseViewerProps {
  value: string
  isLoading: boolean
  privacyMode?: boolean
  onCopyDownloadCurl?: () => Promise<void> | void
  onSelectionChange?: (selections: ResponseSelectionSnapshot[]) => void
}

// 处理响应内容：格式化 JSON body
const processResponse = (content: string): { processed: string; language: string } => {
  if (!content) return { processed: content, language: 'plaintext' }

  // 检测是否是 HTTP 响应
  const isHttpResponse = content.match(/^HTTP\/[\d.]+ \d+/)

  if (isHttpResponse) {
    // 分离响应头和响应体（\r\n\r\n 或 \n\n）
    const separatorMatch = content.match(/\r?\n\r?\n/)
    if (separatorMatch) {
      const separatorIndex = content.indexOf(separatorMatch[0])
      const headers = content.substring(0, separatorIndex)
      const body = content.substring(separatorIndex + separatorMatch[0].length)

      // 尝试格式化 JSON body
      if (body.trim()) {
        try {
          const parsed = JSON.parse(body.trim())
          const formattedJson = JSON.stringify(parsed, null, 2)
          return {
            processed: `${headers}\n\n${formattedJson}`,
            language: 'http'
          }
        } catch {
          // body 不是 JSON，保持原样
        }
      }
    }
    return { processed: content, language: 'http' }
  }

  // 尝试检测纯 JSON
  try {
    const parsed = JSON.parse(content)
    return {
      processed: JSON.stringify(parsed, null, 2),
      language: 'json'
    }
  } catch {
    // 不是 JSON
  }

  // 检测 HTML
  if (content.trim().startsWith('<') && content.includes('</')) {
    return { processed: content, language: 'html' }
  }

  return { processed: content, language: 'plaintext' }
}

export function ResponseViewer({
  value,
  isLoading,
  privacyMode = false,
  onCopyDownloadCurl,
  onSelectionChange,
}: ResponseViewerProps) {
  const { t } = useI18n()
  const { toast } = useToast()
  const editorRef = useRef<any>(null)
  const monacoRef = useRef<any>(null)
  const decorationsRef = useRef<string[]>([])
  const onSelectionChangeRef = useRef(onSelectionChange)
  const processedRef = useRef("")
  const [editorReady, setEditorReady] = useState(false)

  // 处理响应内容
  const { processed, language } = useMemo(() => processResponse(value), [value])

  useEffect(() => {
    onSelectionChangeRef.current = onSelectionChange
  }, [onSelectionChange])

  useEffect(() => {
    processedRef.current = processed
  }, [processed])

  // 编辑器挂载后的处理（用于获取编辑器实例）
  const handleEditorMount = (
    editorInstance: any,
    monacoInstance: any
  ) => {
    editorRef.current = editorInstance
    monacoRef.current = monacoInstance
    setEditorReady(true)
    editorInstance.onDidChangeCursorSelection(() => {
      onSelectionChangeRef.current?.(getCurrentSelectionSnapshots())
    })
    // 注意：主题、语言注册、右键菜单禁用、编辑器选项等已在 wrapper 中统一处理
  }

  const getCurrentSelectionSnapshots = (): ResponseSelectionSnapshot[] => {
    const editorInstance = editorRef.current
    const model = editorInstance?.getModel?.()
    const selections = editorInstance?.getSelections?.() || []
    if (!editorInstance || !model || selections.length === 0) {
      return []
    }

    const normalized = processedRef.current.replace(/\r\n/g, "\n")
    const separatorIndex = normalized.indexOf("\n\n")
    const bodyOffset = separatorIndex === -1 ? normalized.length : separatorIndex + 2

    return selections
      .filter((selection: any) => selection && !selection.isEmpty())
      .map((selection: any) => {
        const text = model.getValueInRange(selection)
        if (!text.trim()) {
          return null
        }

        const startOffset = model.getOffsetAt(selection.getStartPosition())
        const endOffset = model.getOffsetAt(selection.getEndPosition())

        return {
          text,
          part: startOffset >= bodyOffset ? "body" : "header",
          startOffset,
          endOffset,
        } satisfies ResponseSelectionSnapshot
      })
      .filter(Boolean) as ResponseSelectionSnapshot[]
  }

  useEffect(() => {
    const editorInstance = editorRef.current
    const monacoInstance = monacoRef.current
    if (!editorInstance || !monacoInstance) return

    if (!privacyMode) {
      decorationsRef.current = editorInstance.deltaDecorations(decorationsRef.current, [])
      return
    }

    const matches = collectPrivacyMatches(processed)
    const nextDecorations = matches.map((match) => {
      const start = offsetToPosition(processed, match.start)
      const end = offsetToPosition(processed, match.end)
      return {
        range: new monacoInstance.Range(start.lineNumber, start.column, end.lineNumber, end.column),
        options: {
          inlineClassName: "heph-repeater-privacy-mask-inline",
          hoverMessage: { value: "Sensitive content hidden" },
        },
      }
    })

    decorationsRef.current = editorInstance.deltaDecorations(decorationsRef.current, nextDecorations)
  }, [editorReady, privacyMode, processed])

  const handleCopy = useCallback(async () => {
    const editorInstance = editorRef.current
    if (!editorInstance) return

    const selection = editorInstance.getSelection()
    const selectedText = selection ? editorInstance.getModel()?.getValueInRange(selection) : ""
    const textToCopy = selectedText || processed

    const success = await copyToClipboard(textToCopy)
    if (success) {
      toast({ description: t.repeater.copied })
    } else {
      toast({
        description: t.common.copyFailed,
        variant: "destructive",
      })
    }
  }, [processed, t.common.copyFailed, t.repeater.copied, toast])

  const handleSelectAll = useCallback(() => {
    const editorInstance = editorRef.current
    if (!editorInstance) return

    const model = editorInstance.getModel()
    if (!model) return
    editorInstance.setSelection(model.getFullModelRange())
  }, [])

  useEffect(() => {
    onSelectionChangeRef.current?.([])
  }, [processed])

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center bg-code-bg">
        <div className="flex flex-col items-center gap-4 px-8 text-center">
          <div className="relative">
            <div className="absolute inset-0 rounded-full bg-primary/20 blur-xl animate-pulse" />
            <div className="relative flex h-20 w-20 items-center justify-center rounded-full bg-muted/50 border border-border/50">
              <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
            </div>
          </div>
          <div className="flex flex-col gap-2">
            <h3 className="text-base font-medium text-foreground">
              {t.repeater.responseState.loadingTitle}
            </h3>
            <p className="text-sm text-muted-foreground">
              {t.repeater.responseState.loadingDescription}
            </p>
          </div>
        </div>
      </div>
    )
  }

  if (!value) {
    return (
      <div className="flex h-full items-center justify-center bg-code-bg">
        <div className="flex flex-col items-center gap-4 px-8 text-center">
          <div className="relative">
            <div className="absolute inset-0 rounded-full bg-primary/20 blur-xl" />
            <div className="relative flex h-20 w-20 items-center justify-center rounded-full bg-muted/50 border border-border/50">
              <Send className="h-8 w-8 text-muted-foreground" />
            </div>
          </div>
          <div className="flex flex-col gap-2">
            <h3 className="text-base font-medium text-foreground">
              {t.repeater.responseState.emptyTitle}
            </h3>
            <p className="text-sm text-muted-foreground max-w-sm">
              {t.repeater.responseState.emptyDescriptionPrefix}{' '}
              <span className="inline-flex items-center gap-1 text-primary">
                <Send className="h-3.5 w-3.5" />
                {t.repeater.send}
              </span>{' '}
              {t.repeater.responseState.emptyDescriptionSuffix}
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <>
      <ResponseContextMenu
        onCopy={handleCopy}
        onSelectAll={handleSelectAll}
        onCopyDownloadCurl={onCopyDownloadCurl}
      >
        <div 
          className="relative h-full w-full overflow-visible rounded-md border border-border bg-code-bg scan-panel-enter"
          onContextMenu={() => {
            // 阻止 Monaco Editor 的默认右键菜单
          }}
        >
          <MonacoEditorWrapper
            height="100%"
            language={language}
            theme="http-theme"
            value={processed}
            readOnly={true}
            onMount={handleEditorMount}
            options={{
              multiCursorModifier: "alt",
            }}
          />
        </div>
      </ResponseContextMenu>
      <style jsx global>{`
        .heph-repeater-privacy-mask-inline {
          background: rgba(148, 163, 184, 0.28);
          border-radius: 4px;
          color: transparent !important;
          text-shadow: none !important;
        }
      `}</style>
    </>
  )
}
