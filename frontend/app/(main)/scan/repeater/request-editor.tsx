"use client"

import { useRef, useCallback, useEffect, useState } from "react"
import dynamic from "next/dynamic"
import { RequestContextMenu } from "./context-menu"
import { useToast } from "@/hooks/use-toast"
import { copyToClipboard } from "@/lib/clipboard"
import { collectPrivacyMatches, offsetToPosition } from "./privacy-mask"
import { useI18n } from "@/lib/i18n"

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

interface RequestEditorProps {
  value:  string
  onChange: (value: string) => void
  privacyMode?: boolean
  readOnly?: boolean
  onCopyUrl?: () => void
  onCopyAsCurl?: () => void
  copyAsCurlLabel?: string
}

type FlatParamEntry = {
  key: string
  value: string
}

type ParsedRequest = {
  method: string
  target: string
  version: string
  headers: string[]
  body: string
}

const splitRawRequest = (raw: string) => {
  const normalized = raw.replace(/\r\n/g, "\n")
  const separatorIndex = normalized.indexOf("\n\n")
  if (separatorIndex === -1) {
    return {
      normalized,
      headerPart: normalized,
      bodyPart: "",
      separatorIndex: -1,
      bodyOffset: normalized.length,
    }
  }

  return {
    normalized,
    headerPart: normalized.slice(0, separatorIndex),
    bodyPart: normalized.slice(separatorIndex + 2),
    separatorIndex,
    bodyOffset: separatorIndex + 2,
  }
}

const getHeaderValue = (headerPart: string, headerName: string): string => {
  const lines = headerPart.split("\n")
  const target = `${headerName.toLowerCase()}:`
  const line = lines.find((item) => item.toLowerCase().startsWith(target))
  return line ? line.slice(line.indexOf(":") + 1).trim() : ""
}

const safeUrlDecode = (input: string) => {
  try {
    return decodeURIComponent(input.replace(/\+/g, "%20"))
  } catch {
    return input
  }
}

const setHeaderValue = (headers: string[], headerName: string, value: string | null) => {
  const nextHeaders = [...headers]
  const index = nextHeaders.findIndex((line) => line.toLowerCase().startsWith(`${headerName.toLowerCase()}:`))
  if (value === null) {
    if (index !== -1) {
      nextHeaders.splice(index, 1)
    }
    return nextHeaders
  }

  const nextLine = `${headerName}: ${value}`
  if (index !== -1) {
    nextHeaders[index] = nextLine
  } else {
    nextHeaders.push(nextLine)
  }
  return nextHeaders
}

const parseRequest = (raw: string): ParsedRequest | null => {
  const { headerPart, bodyPart } = splitRawRequest(raw)
  const lines = headerPart.split("\n").filter((line, index) => index === 0 || line.length > 0)
  const requestLine = lines[0] || ""
  const requestMatch = requestLine.match(/^([A-Z]+)\s+(.+?)\s+HTTP\/([\d.]+)$/i)
  if (!requestMatch) return null

  return {
    method: requestMatch[1].toUpperCase(),
    target: requestMatch[2],
    version: requestMatch[3],
    headers: lines.slice(1),
    body: bodyPart,
  }
}

const buildRawRequest = (request: ParsedRequest) => {
  const requestLine = `${request.method} ${request.target} HTTP/${request.version}`
  const headerPart = [requestLine, ...request.headers].join("\n")
  return request.body ? `${headerPart}\n\n${request.body}` : `${headerPart}\n\n`
}

const parseTarget = (target: string) => {
  if (/^https?:\/\//i.test(target)) {
    const url = new URL(target)
    return {
      isAbsolute: true,
      origin: url.origin,
      pathname: url.pathname,
      searchParams: url.searchParams,
    }
  }

  const [pathname, query = ""] = target.split("?", 2)
  return {
    isAbsolute: false,
    origin: "",
    pathname,
    searchParams: new URLSearchParams(query),
  }
}

const buildTarget = (target: ReturnType<typeof parseTarget>) => {
  const query = target.searchParams.toString()
  const pathWithQuery = query ? `${target.pathname}?${query}` : target.pathname
  return target.isAbsolute ? `${target.origin}${pathWithQuery}` : pathWithQuery
}

const parseFlatJsonEntries = (body: string): FlatParamEntry[] | null => {
  try {
    const parsed = JSON.parse(body)
    if (!parsed || Array.isArray(parsed) || typeof parsed !== "object") return null

    return Object.entries(parsed).map(([key, value]) => {
      if (value === null) return { key, value: "null" }
      if (["string", "number", "boolean"].includes(typeof value)) {
        return { key, value: String(value) }
      }
      throw new Error("nested")
    })
  } catch {
    return null
  }
}

const extractParameterEntries = (raw: string): FlatParamEntry[] | null => {
  const parsed = parseRequest(raw)
  if (!parsed) return null

  const contentType = getHeaderValue(parsed.headers.join("\n"), "Content-Type").toLowerCase()
  if (parsed.body.trim()) {
    if (contentType.includes("application/x-www-form-urlencoded")) {
      return Array.from(new URLSearchParams(parsed.body).entries()).map(([key, value]) => ({ key, value }))
    }
    if (contentType.includes("application/json")) {
      return parseFlatJsonEntries(parsed.body)
    }
  }

  const target = parseTarget(parsed.target)
  const queryEntries = Array.from(target.searchParams.entries()).map(([key, value]) => ({ key, value }))
  return queryEntries.length > 0 ? queryEntries : null
}

const toFlatJsonObject = (entries: FlatParamEntry[]) => {
  const result: Record<string, string> = {}
  entries.forEach(({ key, value }) => {
    result[key] = value
  })
  return result
}

const updateBodyHeaders = (headers: string[], body: string, contentType?: string | null) => {
  let nextHeaders = [...headers]
  nextHeaders = setHeaderValue(nextHeaders, "Transfer-Encoding", null)
  nextHeaders = setHeaderValue(nextHeaders, "Content-Type", contentType ?? null)
  const bodyBytes = new TextEncoder().encode(body).length
  nextHeaders = setHeaderValue(nextHeaders, "Content-Length", bodyBytes > 0 ? String(bodyBytes) : null)
  return nextHeaders
}

const transformMethod = (raw: string, nextMethod: "GET" | "POST" | "HEAD") => {
  const parsed = parseRequest(raw)
  if (!parsed) return raw

  const target = parseTarget(parsed.target)
  const entries = extractParameterEntries(raw) || []
  parsed.method = nextMethod

  if (nextMethod === "GET" || nextMethod === "HEAD") {
    if (entries.length > 0) {
      target.searchParams = new URLSearchParams()
      entries.forEach(({ key, value }) => target.searchParams.append(key, value))
      parsed.target = buildTarget(target)
    }
    parsed.body = ""
    parsed.headers = setHeaderValue(parsed.headers, "Content-Type", null)
    parsed.headers = setHeaderValue(parsed.headers, "Content-Length", null)
    parsed.headers = setHeaderValue(parsed.headers, "Transfer-Encoding", null)
    return buildRawRequest(parsed)
  }

  if (!parsed.body && entries.length > 0) {
    parsed.body = new URLSearchParams(entries.map(({ key, value }) => [key, value])).toString()
    target.searchParams = new URLSearchParams()
    parsed.target = buildTarget(target)
    parsed.headers = updateBodyHeaders(parsed.headers, parsed.body, "application/x-www-form-urlencoded")
  } else {
    const contentType = getHeaderValue(parsed.headers.join("\n"), "Content-Type") || null
    parsed.headers = updateBodyHeaders(parsed.headers, parsed.body, contentType)
  }

  return buildRawRequest(parsed)
}

const transformParameters = (raw: string, kind: "form" | "json" | "xml" | "multipart") => {
  const parsed = parseRequest(raw)
  if (!parsed) return null
  const entries = extractParameterEntries(raw) || []

  parsed.method = "POST"
  const target = parseTarget(parsed.target)
  target.searchParams = new URLSearchParams()
  parsed.target = buildTarget(target)

  if (kind === "form") {
    parsed.body = new URLSearchParams(entries.map(({ key, value }) => [key, value])).toString()
    parsed.headers = updateBodyHeaders(parsed.headers, parsed.body, "application/x-www-form-urlencoded")
    return buildRawRequest(parsed)
  }

  if (kind === "json") {
    parsed.body = JSON.stringify(entries.length > 0 ? toFlatJsonObject(entries) : {}, null, 2)
    parsed.headers = updateBodyHeaders(parsed.headers, parsed.body, "application/json")
    return buildRawRequest(parsed)
  }

  if (kind === "xml") {
    parsed.body = entries.length > 0
      ? [
          "<root>",
          ...entries.map(({ key, value }) => `  <${key}>${value}</${key}>`),
          "</root>",
        ].join("\n")
      : "<root>\n</root>"
    parsed.headers = updateBodyHeaders(parsed.headers, parsed.body, "application/xml")
    return buildRawRequest(parsed)
  }

  const boundary = randomWebKitBoundary()
  const multipartEntries = entries.length > 0 ? entries : [{ key: "key", value: "[value]" }]
  parsed.body = multipartEntries.map(({ key, value }) => {
    return `--${boundary}\nContent-Disposition: form-data; name="${key}"\n\n${value}`
  }).join("\n") + `\n--${boundary}--`
  parsed.headers = updateBodyHeaders(parsed.headers, parsed.body, `multipart/form-data; boundary=${boundary}`)
  return buildRawRequest(parsed)
}

const withSpoofedIpHeaders = (raw: string, ip: string) => {
  const parsed = parseRequest(raw)
  if (!parsed) return raw

  parsed.headers = setHeaderValue(parsed.headers, "X-Forwarded-For", ip)
  parsed.headers = setHeaderValue(parsed.headers, "X-Real-IP", ip)
  parsed.headers = setHeaderValue(parsed.headers, "Client-IP", ip)
  return buildRawRequest(parsed)
}

const randomIpv4 = () => {
  const octet = () => Math.floor(Math.random() * 254) + 1
  return `${octet()}.${octet()}.${octet()}.${octet()}`
}

const randomWebKitBoundary = () => {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
  let suffix = ""
  for (let i = 0; i < 16; i++) {
    suffix += alphabet[Math.floor(Math.random() * alphabet.length)]
  }
  return `----WebKitFormBoundary${suffix}`
}

export function RequestEditor({ value, onChange, privacyMode = false, readOnly = false, onCopyUrl, onCopyAsCurl, copyAsCurlLabel }: RequestEditorProps) {
  const { t } = useI18n()
  const editorRef = useRef<any>(null)
  const monacoRef = useRef<any>(null)
  const decorationsRef = useRef<string[]>([])
  const [editorReady, setEditorReady] = useState(false)
  const { toast } = useToast()

  // 编辑器挂载后的处理（用于获取编辑器实例和注册快捷键）
  const handleEditorMount = (
    editorInstance: any,
    monacoInstance: any
  ) => {
    editorRef.current = editorInstance
    monacoRef.current = monacoInstance
    setEditorReady(true)
    // 注意：主题、语言注册、右键菜单禁用等已在 wrapper 中统一处理
  }

  const getSelectedText = useCallback((): string => {
    if (!editorRef.current) return ""
    const selection = editorRef.current.getSelection()
    if (!selection) return ""
    return editorRef.current.getModel()?.getValueInRange(selection) || ""
  }, [])

  const replaceSelection = useCallback((newText: string) => {
    if (!editorRef.current) return
    const selection = editorRef.current.getSelection()
    if (!selection) return

    const model = editorRef.current.getModel()
    if (!model) return

    // 计算新的光标位置
    const startLine = selection.startLineNumber
    const startColumn = selection.startColumn
    const newEndColumn = startColumn + newText.length

    // 使用 pushEditOperations 方法，确保操作被正确记录到撤销栈
    // 这个方法会正确处理撤销/重做功能
    model.pushEditOperations(
      [selection], // 当前光标状态
      [
        {
          range: selection,
          text: newText,
        },
      ],
      (inverseEditOperations: any) => {
        // 返回编辑后的光标状态，使用 Selection 类创建正确的选择对象
        // Selection 从 monacoRef 获取
        if (monacoRef.current && monacoRef.current.Selection) {
          return [
            new monacoRef.current.Selection(
              startLine,
              startColumn,
              startLine,
              newEndColumn
            ),
          ]
        }
        // 如果 Selection 不可用，返回简单的选择对象
        return [{
          startLineNumber: startLine,
          startColumn: startColumn,
          endLineNumber: startLine,
          endColumn: newEndColumn,
        }]
      }
    )

    // 移动光标到替换后的文本末尾
    editorRef.current.setSelection({
      startLineNumber: startLine,
      startColumn: startColumn,
      endLineNumber: startLine,
      endColumn: newEndColumn,
    })

    editorRef.current.focus()
  }, [])

  const handleCopy = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      copyToClipboard(selected)
      toast({ description: "Copied to clipboard" })
    }
  }, [getSelectedText, toast])

  const handleCut = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      copyToClipboard(selected)
      replaceSelection("")
      toast({ description: "Cut to clipboard" })
    }
  }, [getSelectedText, replaceSelection, toast])

  const handlePaste = useCallback(async () => {
    try {
      const text = await navigator.clipboard.readText()
      replaceSelection(text)
      toast({ description: "Pasted from clipboard" })
    } catch (error) {
      toast({ description: "Failed to paste", variant: "destructive" })
    }
  }, [replaceSelection, toast])

  const handleUrlEncode = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      const encoded = encodeURIComponent(selected)
      replaceSelection(encoded)
      toast({ description: "URL encoded" })
    } else {
      toast({
        description: "Please select text to encode",
        variant: "destructive",
      })
    }
  }, [getSelectedText, replaceSelection, toast])

  const handleUrlDecode = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      try {
        const decoded = decodeURIComponent(selected)
        replaceSelection(decoded)
        toast({ description: "URL decoded" })
      } catch (error) {
        toast({
          description: "Invalid URL encoded text",
          variant: "destructive",
        })
      }
    } else {
      toast({
        description: "Please select text to decode",
        variant: "destructive",
      })
    }
  }, [getSelectedText, replaceSelection, toast])

  const handleBase64Encode = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      const encoded = btoa(selected)
      replaceSelection(encoded)
      toast({ description: "Base64 encoded" })
    } else {
      toast({
        description: "Please select text to encode",
        variant: "destructive",
      })
    }
  }, [getSelectedText, replaceSelection, toast])

  const handleBase64Decode = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      try {
        const decoded = atob(selected)
        replaceSelection(decoded)
        toast({ description: "Base64 decoded" })
      } catch (error) {
        toast({
          description: "Invalid Base64 text",
          variant: "destructive",
        })
      }
    } else {
      toast({
        description: "Please select text to decode",
        variant: "destructive",
      })
    }
  }, [getSelectedText, replaceSelection, toast])

  const handleUnicodeEncode = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      // Unicode 编码：将字符串转换为 \uXXXX 格式
      const encoded = selected
        .split("")
        .map((char) => {
          const code = char.charCodeAt(0)
          if (code > 0xffff) {
            // 处理大于 0xFFFF 的字符（使用代理对）
            const high = Math.floor((code - 0x10000) / 0x400) + 0xd800
            const low = ((code - 0x10000) % 0x400) + 0xdc00
            return `\\u${high.toString(16).padStart(4, "0")}\\u${low.toString(16).padStart(4, "0")}`
          }
          return `\\u${code.toString(16).padStart(4, "0")}`
        })
        .join("")
      replaceSelection(encoded)
      toast({ description: "Unicode encoded" })
    } else {
      toast({
        description: "Please select text to encode",
        variant: "destructive",
      })
    }
  }, [getSelectedText, replaceSelection, toast])

  const handleUnicodeDecode = useCallback(() => {
    const selected = getSelectedText()
    if (selected) {
      try {
        // Unicode 解码：将 \uXXXX 格式转换为字符
        // 支持 \uXXXX 和 \u{XXXXXX} 格式
        const decoded = selected.replace(/\\(u[0-9a-fA-F]{4}|u\{[0-9a-fA-F]+\})/g, (match) => {
          if (match.startsWith("\\u{")) {
            // \u{XXXXXX} 格式
            const code = parseInt(match.slice(3, -1), 16)
            return String.fromCodePoint(code)
          } else {
            // \uXXXX 格式
            const code = parseInt(match.slice(2), 16)
            return String.fromCharCode(code)
          }
        })
        replaceSelection(decoded)
        toast({ description: "Unicode decoded" })
      } catch (error) {
        toast({
          description: "Invalid Unicode text",
          variant: "destructive",
        })
      }
    } else {
      toast({
        description: "Please select text to decode",
        variant: "destructive",
      })
    }
  }, [getSelectedText, replaceSelection, toast])


  const handleSelectAll = useCallback(() => {
    if (editorRef.current) {
      const model = editorRef.current.getModel()
      if (model) {
        const fullRange = model.getFullModelRange()
        editorRef.current.setSelection(fullRange)
      }
    }
  }, [])

  const handleApplyMutation = useCallback((label: string, transform: (raw: string) => string | null) => {
    const nextRaw = transform(value)
    if (!nextRaw) {
      toast({
        description: t.repeater.mutations.applyFailed.replace('{label}', label),
        variant: "destructive",
      })
      return
    }

    onChange(nextRaw)
    toast({ description: t.repeater.mutations.applySuccess.replace('{label}', label) })
  }, [onChange, t.repeater.mutations.applyFailed, t.repeater.mutations.applySuccess, toast, value])

  const handleConvertMethodToGet = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.changeMethodToGet, (raw) => transformMethod(raw, "GET"))
  }, [handleApplyMutation, t.repeater.contextMenu.changeMethodToGet])

  const handleConvertMethodToPost = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.changeMethodToPost, (raw) => transformMethod(raw, "POST"))
  }, [handleApplyMutation, t.repeater.contextMenu.changeMethodToPost])

  const handleConvertMethodToHead = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.changeMethodToHead, (raw) => transformMethod(raw, "HEAD"))
  }, [handleApplyMutation, t.repeater.contextMenu.changeMethodToHead])

  const handleConvertToMultipart = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.convertToMultipart, (raw) => transformParameters(raw, "multipart"))
  }, [handleApplyMutation, t.repeater.contextMenu.convertToMultipart])

  const handleConvertToForm = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.convertToForm, (raw) => transformParameters(raw, "form"))
  }, [handleApplyMutation, t.repeater.contextMenu.convertToForm])

  const handleConvertToJson = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.convertToJson, (raw) => transformParameters(raw, "json"))
  }, [handleApplyMutation, t.repeater.contextMenu.convertToJson])

  const handleConvertToXml = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.convertToXml, (raw) => transformParameters(raw, "xml"))
  }, [handleApplyMutation, t.repeater.contextMenu.convertToXml])

  const handleReplaceNbsp = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.replaceNbsp, (raw) => raw.replace(/\u00A0/g, " "))
  }, [handleApplyMutation, t.repeater.contextMenu.replaceNbsp])

  const handleSpoofLocalIp = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.spoofLocalIp, (raw) => withSpoofedIpHeaders(raw, "127.0.0.1"))
  }, [handleApplyMutation, t.repeater.contextMenu.spoofLocalIp])

  const handleSpoofRandomIp = useCallback(() => {
    handleApplyMutation(t.repeater.contextMenu.spoofRandomIp, (raw) => withSpoofedIpHeaders(raw, randomIpv4()))
  }, [handleApplyMutation, t.repeater.contextMenu.spoofRandomIp])

  // 注册快捷键（在所有函数定义之后）
  useEffect(() => {
    if (editorRef.current && monacoRef.current) {
      const editorInstance = editorRef.current
      const monacoInstance = monacoRef.current

      // Ctrl+U: URL Encode
      editorInstance.addCommand(
        monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.KeyU,
        () => {
          handleUrlEncode()
        }
      )

      // Ctrl+Shift+U: URL Decode
      editorInstance.addCommand(
        monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyMod.Shift | monacoInstance.KeyCode.KeyU,
        () => {
          handleUrlDecode()
        }
      )

      // Ctrl+B: Base64 Encode
      editorInstance.addCommand(
        monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.KeyB,
        () => {
          handleBase64Encode()
        }
      )

      // Ctrl+Shift+B: Base64 Decode
      editorInstance.addCommand(
        monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyMod.Shift | monacoInstance.KeyCode.KeyB,
        () => {
          handleBase64Decode()
        }
      )

      // Ctrl+Shift+E: Unicode Encode
      editorInstance.addCommand(
        monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyMod.Shift | monacoInstance.KeyCode.KeyE,
        () => {
          handleUnicodeEncode()
        }
      )

      // Ctrl+Shift+D: Unicode Decode
      editorInstance.addCommand(
        monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyMod.Shift | monacoInstance.KeyCode.KeyD,
        () => {
          handleUnicodeDecode()
        }
      )

    }
  }, [handleUrlEncode, handleUrlDecode, handleBase64Encode, handleBase64Decode, handleUnicodeEncode, handleUnicodeDecode])

  useEffect(() => {
    const editorInstance = editorRef.current
    const monacoInstance = monacoRef.current
    if (!editorInstance || !monacoInstance) return

    if (!privacyMode) {
      decorationsRef.current = editorInstance.deltaDecorations(decorationsRef.current, [])
      return
    }

    const matches = collectPrivacyMatches(value)
    const nextDecorations = matches.map((match) => {
      const start = offsetToPosition(value, match.start)
      const end = offsetToPosition(value, match.end)
      return {
        range: new monacoInstance.Range(start.lineNumber, start.column, end.lineNumber, end.column),
        options: {
          inlineClassName: "heph-repeater-privacy-mask-inline",
          hoverMessage: { value: "Sensitive content hidden" },
        },
      }
    })

    decorationsRef.current = editorInstance.deltaDecorations(decorationsRef.current, nextDecorations)
  }, [editorReady, privacyMode, value])

  return (
    <>
      <RequestContextMenu
        onCopy={handleCopy}
        onCopyUrl={onCopyUrl}
        onCut={handleCut}
        onPaste={handlePaste}
        onUrlEncode={handleUrlEncode}
        onUrlDecode={handleUrlDecode}
        onBase64Encode={handleBase64Encode}
        onBase64Decode={handleBase64Decode}
        onUnicodeEncode={handleUnicodeEncode}
        onUnicodeDecode={handleUnicodeDecode}
        onSelectAll={handleSelectAll}
        onConvertMethodToGet={handleConvertMethodToGet}
        onConvertMethodToPost={handleConvertMethodToPost}
        onConvertMethodToHead={handleConvertMethodToHead}
        onConvertToMultipart={handleConvertToMultipart}
        onConvertToForm={handleConvertToForm}
        onConvertToJson={handleConvertToJson}
        onConvertToXml={handleConvertToXml}
        onReplaceNbsp={handleReplaceNbsp}
        onSpoofLocalIp={handleSpoofLocalIp}
        onSpoofRandomIp={handleSpoofRandomIp}
        onCopyAsCurl={onCopyAsCurl}
        copyAsCurlLabel={copyAsCurlLabel}
      >
        <div 
          className="heph-repeater-request relative z-10 h-full w-full overflow-visible bg-code-bg rounded-md border border-border"
          onContextMenu={(e) => {
            // 阻止 Monaco Editor 的默认右键菜单
            // RequestContextMenu 会自动处理右键事件
          }}
        >
          <MonacoEditorWrapper
            height="100%"
            language="http-raw"
            theme="http-theme"
            value={value}
            onChange={readOnly ? undefined : onChange}
            readOnly={readOnly}
            onMount={handleEditorMount}
          />
        </div>
      </RequestContextMenu>
      <style jsx global>{`
        .heph-repeater-privacy-mask-inline {
          background: rgba(148, 163, 184, 0.28);
          border-radius: 4px;
          color: transparent !important;
          text-shadow: none !important;
        }

        .heph-repeater-request .monaco-editor .editor-widget.find-widget {
          right: 48px !important;
        }
      `}</style>
    </>
  )
}
