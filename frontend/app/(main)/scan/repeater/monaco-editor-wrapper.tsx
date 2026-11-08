"use client"

import { useEffect, useMemo, useRef } from "react"
import Editor from "@monaco-editor/react"
import type { editor } from "monaco-editor"
import { useTheme } from "next-themes"
import { getAppThemeBase } from "@/lib/theme/custom-theme"
import "@/lib/monaco"

interface MonacoEditorWrapperProps {
  value: string
  onChange?: (value: string) => void
  language?: string
  theme?: string
  readOnly?: boolean
  onMount?: (editor: editor.IStandaloneCodeEditor, monaco: any) => void
  beforeMount?: (monaco: any) => void
  options?: any
  height?: string
}

// 统一的主题定义
const defineHttpTheme = (monacoInstance: any) => {
  monacoInstance.editor.defineTheme("http-theme-dark", {
    base: "vs-dark",
    inherit: true,
    rules: [
      { token: "keyword", foreground: "ff7a3d" }, // 火焰橙：HTTP 方法/版本 (oklch(0.70 0.22 35))
      { token: "string", foreground: "ff9a5d" }, // 浅橙色：URL
      { token: "variable", foreground: "9cdcfe" }, // 浅蓝色：请求头
      { token: "delimiter", foreground: "d4d4d4" }, // 灰色：冒号
      { token: "number", foreground: "b5cea8" }, // 绿色：值/状态码
      { token: "comment", foreground: "6a9955" }, // 绿色：注释
    ],
    colors: {
      "editor.background": "#1a1a1a", // oklch(0.12 0.01 270) 的近似值 - 深色背景
      "editor.foreground": "#f2f2f2", // oklch(0.95 0.01 90) 的近似值 - 浅色前景
      "editor.lineNumbersBackground": "#252525", // 行号背景
      "editor.lineNumbersForeground": "#858585", // 行号前景
      "editorCursor.foreground": "#ff7a3d", // oklch(0.70 0.22 35) 火焰橙 - 光标颜色
      "editor.selectionBackground": "#ff7a3d40", // 选择背景（带透明度）
      "editor.lineHighlightBackground": "#2a2a2a", // oklch(0.20 0.02 270) 行高亮背景
      "editorWidget.background": "#2a2a2a", // 小部件背景
      "editorWidget.border": "#4a4a4a", // oklch(0.35 0.02 260) 小部件边框
      "editorIndentGuide.activeBackground": "#4a4a4a", // 缩进指南
      "editorIndentGuide.background": "#3a3a3a", // 缩进指南背景
      "editorBracketMatch.background": "#ff7a3d20", // 括号匹配背景
      "editorBracketMatch.border": "#ff7a3d", // 括号匹配边框
    },
  })

  monacoInstance.editor.defineTheme("http-theme-light", {
    base: "vs",
    inherit: true,
    rules: [
      { token: "keyword", foreground: "d84a1b" },
      { token: "string", foreground: "1d4ed8" },
      { token: "variable", foreground: "0f766e" },
      { token: "delimiter", foreground: "6b7280" },
      { token: "number", foreground: "166534" },
      { token: "comment", foreground: "6b7280" },
    ],
    colors: {
      "editor.background": "#ffffff",
      "editor.foreground": "#1f2329",
      "editor.lineNumbersBackground": "#f7f8fa",
      "editor.lineNumbersForeground": "#9ca3af",
      "editorCursor.foreground": "#d84a1b",
      "editor.selectionBackground": "#1677ff26",
      "editor.lineHighlightBackground": "#f5f7fa",
      "editorWidget.background": "#ffffff",
      "editorWidget.border": "#d9d9d9",
      "editorIndentGuide.activeBackground": "#cbd5e1",
      "editorIndentGuide.background": "#e5e7eb",
      "editorBracketMatch.background": "#1677ff14",
      "editorBracketMatch.border": "#1677ff",
    },
  })

  monacoInstance.editor.defineTheme("http-theme", {
    base: "vs-dark",
    inherit: true,
    rules: [],
    colors: {},
  })
}

// 注册 HTTP Raw Request 语言
const registerHttpRawLanguage = (monacoInstance: any) => {
  if (!monacoInstance.languages.getLanguages().find((lang: any) => lang.id === "http-raw")) {
    monacoInstance.languages.register({ id: "http-raw" })
    
    monacoInstance.languages.setMonarchTokensProvider("http-raw", {
      tokenizer: {
        root: [
          // HTTP 方法
          [/^(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS|TRACE|CONNECT)\b/, "keyword"],
          // URL
          [/https?:\/\/[^\s]+/, "string"],
          // 请求头名称
          [/^[A-Za-z0-9-]+(?:: )/, "variable"],
          // 冒号分隔符
          [/: /, "delimiter"],
          // 值
          [/:\s+.+/, "number"],
          // 注释
          [/#.*$/, "comment"],
          // 空行
          [/^\s*$/, "whitespace"],
        ],
      },
    })
  }
}

// 注册 HTTP 响应语言
const registerHttpLanguage = (monacoInstance: any) => {
  if (!monacoInstance.languages.getLanguages().find((lang: any) => lang.id === "http")) {
    monacoInstance.languages.register({ id: "http" })
    
    monacoInstance.languages.setMonarchTokensProvider("http", {
      tokenizer: {
        root: [
          // HTTP 版本和状态码
          [/^HTTP\/[\d.]+ \d+/, "keyword"],
          // 状态码
          [/\b\d{3}\b/, "number"],
          // 响应头名称
          [/^[A-Za-z0-9-]+(?:: )/, "variable"],
          // 冒号分隔符
          [/: /, "delimiter"],
          // URL
          [/https?:\/\/[^\s]+/, "string"],
          // 注释
          [/#.*$/, "comment"],
          // 空行
          [/^\s*$/, "whitespace"],
        ],
      },
    })
  }
}

// 统一的默认编辑器选项
const getDefaultOptions = (readOnly: boolean, customOptions: any = {}) => ({
  readOnly,
  lineNumbers: "on" as const,
  glyphMargin: false,
  folding: true,
  foldingStrategy: "indentation" as const, // 基于缩进的折叠，适合 JSON
  contextmenu: false, // 禁用默认右键菜单
  fontFamily: "var(--font-mono), 'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  fontSize: 14,
  lineHeight: 24,
  fontLigatures: false,
  scrollBeyondLastLine: false,
  minimap: {
    enabled: false,
  },
  fixedOverflowWidgets: true,
  find: {
    addExtraSpaceOnTop: true,
  },
  wordWrap: "on" as const,
  wordWrapColumn: 120,
  wrappingIndent: "same" as const,
  renderWhitespace: "none" as const,
  autoIndent: "full" as const,
  trimAutoWhitespace: true,
  padding: {
    top: 8,
    bottom: 8,
  },
  renderLineHighlight: "all" as const,
  occurrencesHighlight: "off" as const,
  selectionHighlight: true,
  bracketPairColorization: {
    enabled: false,
  },
  ...customOptions,
})

export function MonacoEditorWrapper({
  value,
  onChange,
  language = "plaintext",
  theme = "http-theme",
  readOnly = false,
  onMount,
  beforeMount,
  options = {},
  height = "100%",
}: MonacoEditorWrapperProps) {
  const editorRef = useRef<any>(null)
  const monacoRef = useRef<any>(null)
  const { theme: appTheme, resolvedTheme } = useTheme()

  const monacoTheme = useMemo(() => {
    if (theme && theme !== "http-theme") {
      return theme
    }

    const currentTheme = resolvedTheme || appTheme || "light"
    return getAppThemeBase(currentTheme) === "dark" ? "http-theme-dark" : "http-theme-light"
  }, [appTheme, resolvedTheme, theme])

  // 统一的 beforeMount 处理
  const handleBeforeMount = (monacoInstance: any) => {
    // 定义主题
    defineHttpTheme(monacoInstance)
    
    // 注册语言
    registerHttpRawLanguage(monacoInstance)
    registerHttpLanguage(monacoInstance)
    
    // 调用用户自定义的 beforeMount
    if (beforeMount) {
      beforeMount(monacoInstance)
    }
  }

  // 统一的 onMount 处理
  const handleEditorMount = (editorInstance: any, monacoInstance: any) => {
    editorRef.current = editorInstance
    monacoRef.current = monacoInstance

    // 禁用 Monaco Editor 的默认右键菜单
    const contextMenuService = (editorInstance as any)._contextMenuService
    if (contextMenuService) {
      contextMenuService.showContextMenu = () => {
        // 阻止显示默认右键菜单
      }
    }

    // 确保主题已应用
    monacoInstance.editor.setTheme(monacoTheme)

    // 注意：选项已在 Editor 组件的 options prop 中设置，这里不再重复设置
    // 调用用户自定义的 onMount（用户可以在这里进行额外的配置）
    if (onMount) {
      onMount(editorInstance, monacoInstance)
    }
  }

  useEffect(() => {
    const editorInstance = editorRef.current
    if (!editorInstance) return

    const monacoInstance = monacoRef.current
    if (monacoInstance?.editor) {
      monacoInstance.editor.setTheme(monacoTheme)
    }
  }, [monacoTheme])

  return (
    <>
      <Editor
        height={height}
        language={language}
        theme={monacoTheme}
        value={value}
        onChange={onChange ? (newValue) => onChange(newValue || "") : undefined}
        beforeMount={handleBeforeMount}
        onMount={handleEditorMount}
        options={getDefaultOptions(readOnly, options)}
      />
      <style jsx global>{`
        .monaco-editor .overflowWidgets,
        .monaco-editor .overflowingContentWidgets,
        .monaco-editor .monaco-hover,
        .monaco-editor .suggest-widget,
        .monaco-editor .editor-widget.find-widget {
          z-index: 80 !important;
        }

        .heph-repeater-monaco .monaco-hover {
          max-width: none !important;
          white-space: nowrap !important;
        }

        .heph-repeater-monaco .monaco-hover .hover-row,
        .heph-repeater-monaco .monaco-hover .hover-contents,
        .heph-repeater-monaco .monaco-hover .markdown-hover,
        .heph-repeater-monaco .monaco-hover .markdown-hover p {
          white-space: nowrap !important;
        }
      `}</style>
    </>
  )
}
