'use client'

import { type ChangeEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchWithAuth } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { signHmac, type ShaAlgorithm } from '@/lib/browser-crypto'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import {
  CheckCircle2,
  Copy,
  KeyRound,
  PauseCircle,
  RefreshCw,
  ShieldCheck,
  Upload,
  XCircle,
} from 'lucide-react'
import toast from 'react-hot-toast'

interface ParsedJwtToken {
  raw: string
  headerSegment: string
  payloadSegment: string
  signatureSegment: string
  headerText: string
  payloadText: string
  header: Record<string, unknown>
  payload: Record<string, unknown>
  signingInput: string
  signatureBytes: Uint8Array
  algorithm: string
}

interface JwtKeyVariant {
  label: string
  displayValue: string
  bytes: Uint8Array
}

interface JwtDictionaryItem {
  name: string
  protocol: string
  group: string
  source: string
  entries: number
  size: string
  description?: string
}

interface JwtAlgorithmOption {
  value: string
  label: string
  alg?: string
}

interface MatchResult {
  matched: boolean
  dictionaryEntry?: string
  variantLabel?: string
  displayValue?: string
  secretPreview?: string
  secretIsBase64UrlEncoded?: boolean
  usedFixedLengthSecret?: boolean
}

interface BruteforceResult extends MatchResult {
  attempts: number
  elapsedMs: number
  totalCandidates: number
  unsupportedAlgorithm?: string
  aborted?: boolean
}

const exampleToken =
  'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWUsImlhdCI6MTUxNjIzOTAyMn0.KMUFsIDTnFmyG3nMiGM6H9FNFUROf3wh7SmqJp-QV30'
const exampleSecret = 'a-string-secret-at-least-256-bits-long'

const textEncoder = new TextEncoder()

const hmacAlgorithms: Record<string, { name: 'HMAC'; hash: ShaAlgorithm }> = {
  HS256: { name: 'HMAC', hash: 'SHA-256' },
  HS384: { name: 'HMAC', hash: 'SHA-384' },
  HS512: { name: 'HMAC', hash: 'SHA-512' },
}

const jwtAlgorithmOptions: readonly JwtAlgorithmOption[] = [
  { value: 'none', label: 'none' },
  { value: 'HS256', label: 'HS256' },
  { value: 'HS384', label: 'HS384' },
  { value: 'HS512', label: 'HS512' },
  { value: 'RS256', label: 'RS256' },
  { value: 'RS384', label: 'RS384' },
  { value: 'RS512', label: 'RS512' },
  { value: 'ES256', label: 'ES256' },
  { value: 'ES384', label: 'ES384' },
  { value: 'ES512', label: 'ES512' },
  { value: 'PS256', label: 'PS256' },
  { value: 'PS384', label: 'PS384' },
  { value: 'PS512', label: 'PS512' },
  { value: 'EdDSA-Ed25519', label: 'EdDSA (Ed25519)', alg: 'EdDSA' },
  { value: 'EdDSA-Ed448', label: 'EdDSA (Ed448)', alg: 'EdDSA' },
]

const hmacFixedSecretLengthMap: Record<string, number> = {
  HS256: 32,
  HS384: 48,
  HS512: 64,
}

const normalizeBase64Url = (input: string): string => {
  const normalized = input.replace(/-/g, '+').replace(/_/g, '/')
  const padding = normalized.length % 4
  return padding === 0 ? normalized : `${normalized}${'='.repeat(4 - padding)}`
}

const decodeBase64UrlToBytes = (input: string): Uint8Array => {
  const normalized = normalizeBase64Url(input.trim())
  const binary = atob(normalized)
  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}

const decodeBase64UrlToString = (input: string): string => {
  return new TextDecoder().decode(decodeBase64UrlToBytes(input))
}

const bytesToBase64 = (bytes: Uint8Array): string => {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary)
}

const encodeBase64Url = (bytes: Uint8Array): string => {
  return bytesToBase64(bytes).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '')
}

const jwtBigIntMarkerPrefix = '__HEPHAESTUS_JWT_BIGINT__'

const replaceUnsafeJsonIntegers = (input: string) => {
  const values: string[] = []
  let output = ''
  let index = 0
  let inString = false
  let escaped = false

  const isUnsafeInteger = (token: string) => {
    if (!/^-?\d+$/.test(token)) return false
    try {
      const value = BigInt(token)
      const limit = BigInt(Number.MAX_SAFE_INTEGER)
      return value > limit || value < -limit
    } catch {
      return false
    }
  }

  while (index < input.length) {
    const char = input[index]

    if (inString) {
      output += char
      if (escaped) {
        escaped = false
      } else if (char === '\\') {
        escaped = true
      } else if (char === '"') {
        inString = false
      }
      index += 1
      continue
    }

    if (char === '"') {
      inString = true
      output += char
      index += 1
      continue
    }

    if (char === '-' || (char >= '0' && char <= '9')) {
      let end = index + 1
      while (end < input.length && /[0-9eE+.-]/.test(input[end])) {
        end += 1
      }
      const token = input.slice(index, end)
      if (isUnsafeInteger(token)) {
        const marker = `${jwtBigIntMarkerPrefix}${values.length}__`
        values.push(token)
        output += `"${marker}"`
      } else {
        output += token
      }
      index = end
      continue
    }

    output += char
    index += 1
  }

  return { json: output, values }
}

const reviveUnsafeJsonIntegers = (value: unknown, values: string[]): unknown => {
  if (typeof value === 'string') {
    const match = value.match(new RegExp(`^${jwtBigIntMarkerPrefix}(\\d+)__$`))
    if (match) {
      return values[Number.parseInt(match[1], 10)] ?? value
    }
    return value
  }
  if (Array.isArray(value)) {
    return value.map((item) => reviveUnsafeJsonIntegers(item, values))
  }
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value).map(([key, item]) => [key, reviveUnsafeJsonIntegers(item, values)])
    )
  }
  return value
}

const parseJsonPreservingLargeIntegers = (input: string): unknown => {
  const transformed = replaceUnsafeJsonIntegers(input)
  const parsed = JSON.parse(transformed.json)
  return reviveUnsafeJsonIntegers(parsed, transformed.values)
}

const formatJsonPreservingLargeIntegers = (input: string): string => {
  const transformed = replaceUnsafeJsonIntegers(input)
  const parsed = JSON.parse(transformed.json)
  let formatted = JSON.stringify(parsed, null, 2)

  transformed.values.forEach((value, index) => {
    const marker = `"${jwtBigIntMarkerPrefix}${index}__"`
    formatted = formatted.replaceAll(marker, value)
  })

  return formatted
}

const parseJwtToken = (input: string): ParsedJwtToken => {
  const raw = input.trim()
  const parts = raw.split('.')
  if (parts.length !== 3) {
    throw new Error('JWT 必须由 3 段组成')
  }

  const headerText = decodeBase64UrlToString(parts[0])
  const payloadText = decodeBase64UrlToString(parts[1])
  const header = parseJsonPreservingLargeIntegers(headerText)
  const payload = parseJsonPreservingLargeIntegers(payloadText)
  if (!header || typeof header !== 'object' || Array.isArray(header)) {
    throw new Error('JWT Header 不是有效的 JSON 对象')
  }
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) {
    throw new Error('JWT Payload 不是有效的 JSON 对象')
  }
  const headerObject = header as Record<string, unknown>
  const payloadObject = payload as Record<string, unknown>

  return {
    raw,
    headerSegment: parts[0],
    payloadSegment: parts[1],
    signatureSegment: parts[2],
    headerText,
    payloadText,
    header: headerObject,
    payload: payloadObject,
    signingInput: `${parts[0]}.${parts[1]}`,
    signatureBytes: decodeBase64UrlToBytes(parts[2]),
    algorithm: typeof headerObject.alg === "string" ? headerObject.alg.toUpperCase() : '',
  }
}

const tryDecodeBase64Bytes = (input: string): Uint8Array | null => {
  const normalized = input.trim().replace(/\s+/g, '')
  if (!normalized || !/^[A-Za-z0-9+/=_-]+$/.test(normalized)) {
    return null
  }

  try {
    const binary = atob(normalizeBase64Url(normalized))
    return Uint8Array.from(binary, (char) => char.charCodeAt(0))
  } catch {
    return null
  }
}

const tryDecodeHexBytes = (input: string): Uint8Array | null => {
  const normalized = input.trim().replace(/^0x/i, '')
  if (!normalized || normalized.length % 2 !== 0 || !/^[0-9a-fA-F]+$/.test(normalized)) {
    return null
  }

  const bytes = new Uint8Array(normalized.length / 2)
  for (let index = 0; index < normalized.length; index += 2) {
    bytes[index / 2] = Number.parseInt(normalized.slice(index, index + 2), 16)
  }
  return bytes
}

const bytesToHex = (bytes: Uint8Array): string => {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

const unwrapQuotedSecret = (input: string): string => {
  const trimmed = input.trim()
  if (trimmed.length >= 2) {
    const first = trimmed[0]
    const last = trimmed[trimmed.length - 1]
    if ((first === '"' && last === '"') || (first === '\'' && last === '\'')) {
      return trimmed.slice(1, -1).trim()
    }
  }
  return trimmed
}

const addVariant = (
  variants: JwtKeyVariant[],
  seen: Set<string>,
  label: string,
  displayValue: string,
  bytes: Uint8Array
) => {
  if (bytes.length === 0) return
  const key = bytesToHex(bytes)
  if (seen.has(key)) return
  seen.add(key)
  variants.push({ label, displayValue, bytes })
}

const buildJwtKeyVariants = (entry: string, includeBase64Variants: boolean): JwtKeyVariant[] => {
  const normalized = unwrapQuotedSecret(entry)
  const variants: JwtKeyVariant[] = []
  const seen = new Set<string>()
  if (!normalized) return variants

  addVariant(variants, seen, '原始文本', normalized, textEncoder.encode(normalized))

  if (normalized !== entry.trim()) {
    addVariant(variants, seen, '去除包裹引号', normalized, textEncoder.encode(normalized))
  }

  if (includeBase64Variants) {
    const decodedBytes = tryDecodeBase64Bytes(normalized)
    if (decodedBytes) {
      addVariant(variants, seen, 'Base64/Base64URL 还原', normalized, decodedBytes)
    }
  }

  const hexBytes = tryDecodeHexBytes(normalized)
  if (hexBytes) {
    addVariant(variants, seen, 'Hex 还原', normalized, hexBytes)
  }

  if (normalized.includes(':')) {
    const passwordPart = normalized.slice(normalized.indexOf(':') + 1).trim()
    if (passwordPart) {
      addVariant(variants, seen, '冒号后半段', passwordPart, textEncoder.encode(passwordPart))
      if (includeBase64Variants) {
        const passwordDecodedBytes = tryDecodeBase64Bytes(passwordPart)
        if (passwordDecodedBytes) {
          addVariant(variants, seen, '后半段 Base64 还原', passwordPart, passwordDecodedBytes)
        }
      }
      const passwordHexBytes = tryDecodeHexBytes(passwordPart)
      if (passwordHexBytes) {
        addVariant(variants, seen, '后半段 Hex 还原', passwordPart, passwordHexBytes)
      }
    }
  }

  return variants
}

const constantTimeEqual = (a: Uint8Array, b: Uint8Array): boolean => {
  if (a.length !== b.length) return false
  let diff = 0
  for (let i = 0; i < a.length; i += 1) {
    diff |= a[i] ^ b[i]
  }
  return diff === 0
}

const signJwtHmac = async (algorithm: string, signingInput: string, secret: Uint8Array): Promise<Uint8Array> => {
  const algo = hmacAlgorithms[algorithm]
  if (!algo) {
    throw new Error(`暂不支持的 JWT 算法: ${algorithm || 'unknown'}`)
  }

  return signHmac(algo.hash, textEncoder.encode(signingInput), secret)
}

const normalizeHmacSecretLength = (algorithm: string, secret: Uint8Array): Uint8Array => {
  const expectedLength = hmacFixedSecretLengthMap[algorithm]
  if (!expectedLength) return secret
  if (secret.length === expectedLength) return secret

  const normalized = new Uint8Array(expectedLength)
  normalized.set(secret.slice(0, expectedLength))
  return normalized
}

const getHmacSecretCandidates = (algorithm: string, secret: Uint8Array) => {
  const candidates: Array<{ bytes: Uint8Array; usedFixedLengthSecret: boolean }> = []
  const seen = new Set<string>()

  const addCandidate = (bytes: Uint8Array, usedFixedLengthSecret: boolean) => {
    const key = bytesToHex(bytes)
    if (seen.has(key)) return
    seen.add(key)
    candidates.push({ bytes, usedFixedLengthSecret })
  }

  addCandidate(secret, false)
  addCandidate(normalizeHmacSecretLength(algorithm, secret), true)
  return candidates
}

const encodeJsonSegment = (input: string): string => {
  const bytes = textEncoder.encode(input)
  return encodeBase64Url(bytes)
}

const getSecretBytes = (secret: string, isBase64UrlEncoded: boolean): Uint8Array => {
  if (!isBase64UrlEncoded) {
    return textEncoder.encode(secret)
  }

  const decoded = tryDecodeBase64Bytes(secret)
  if (!decoded) {
    throw new Error('Secret 不是有效的 Base64URL / Base64 字符串')
  }
  return decoded
}

const formatClaimValue = (value: unknown): string => {
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean' || value === null) {
    return String(value)
  }

  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}

const resolveHeaderAlgorithmValue = (selectedValue: string) =>
  jwtAlgorithmOptions.find((option) => option.value === selectedValue)?.alg ?? selectedValue

const updateHeaderAlgorithm = (headerText: string, nextAlgorithm: string): string => {
  const baseHeader = headerText.trim() ? headerText : '{}'

  try {
    const parsed = parseJsonPreservingLargeIntegers(baseHeader)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return headerText
    }

    const nextHeader = { ...(parsed as Record<string, unknown>), alg: resolveHeaderAlgorithmValue(nextAlgorithm) }
    const nextText = JSON.stringify(nextHeader, null, 2)
    return formatJsonPreservingLargeIntegers(nextText)
  } catch {
    return headerText
  }
}

const jwtTimeClaimKeys = new Set(['iat', 'exp', 'nbf', 'auth_time', 'updated_at'])

const isNumericDateClaim = (key: string, value: unknown): value is number =>
  jwtTimeClaimKeys.has(key.toLowerCase()) && typeof value === 'number' && Number.isFinite(value)

const formatJwtNumericDate = (value: number) => {
  const date = new Date(value * 1000)
  if (Number.isNaN(date.getTime())) return null

  return {
    local: new Intl.DateTimeFormat('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    }).format(date),
    utc: date.toISOString().replace('T', ' ').replace('.000Z', ' UTC'),
  }
}

const ClaimsBreakdownPanel = ({
  claims,
  emptyLabel,
}: {
  claims: Record<string, unknown> | null
  emptyLabel: string
}) => {
  const entries = claims ? Object.entries(claims) : []

  if (entries.length === 0) {
    return (
      <div className="rounded-[5px] border border-dashed border-border/70 bg-background/40 px-4 py-5 text-sm text-muted-foreground">
        {emptyLabel}
      </div>
    )
  }

  return (
    <div className="rounded-[5px] border border-border/70 bg-background/40">
      {entries.map(([key, value], index) => (
        <div
          key={key}
          className={cn(
            'grid gap-2 px-4 py-3 sm:grid-cols-[120px_minmax(0,1fr)]',
            index !== entries.length - 1 && 'border-b border-border/60'
          )}
        >
          <div className="font-mono text-xs uppercase tracking-[0.16em] text-muted-foreground">{key}</div>
          <div className="break-all font-mono text-sm text-foreground">
            {isNumericDateClaim(key, value) && formatJwtNumericDate(value) ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className="inline-flex cursor-help items-center border-b border-dotted border-current/40 text-left text-inherit transition-colors hover:text-primary"
                  >
                    {formatClaimValue(value)}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="top" align="start" className="max-w-[280px] space-y-1">
                  <div className="font-medium">{key.toUpperCase()} 时间</div>
                  <div>本地时间: {formatJwtNumericDate(value)?.local}</div>
                  <div>UTC: {formatJwtNumericDate(value)?.utc}</div>
                </TooltipContent>
              </Tooltip>
            ) : (
              formatClaimValue(value)
            )}
          </div>
        </div>
      ))}
    </div>
  )
}

const HighlightedJwtEditor = ({
  value,
  onChange,
  placeholder,
  className,
  autoFocus = false,
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
  className?: string
  autoFocus?: boolean
}) => {
  const highlightRef = useRef<HTMLPreElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (autoFocus) {
      textareaRef.current?.focus()
    }
  }, [autoFocus])

  const syncScroll = useCallback((event: React.UIEvent<HTMLTextAreaElement>) => {
    if (!highlightRef.current) return
    highlightRef.current.scrollTop = event.currentTarget.scrollTop
    highlightRef.current.scrollLeft = event.currentTarget.scrollLeft
  }, [])

  const tokenSegments = useMemo(() => {
    const parts = value.split('.')
    return parts.length === 3 ? parts : []
  }, [value])

  return (
    <div className={cn('relative min-w-0 overflow-hidden rounded-[5px] border border-border/70 bg-background/70', className)}>
      <pre
        ref={highlightRef}
        aria-hidden="true"
        className="m-0 min-h-[260px] overflow-auto p-4 font-mono text-sm leading-6 whitespace-pre-wrap break-words xl:min-h-[360px]"
      >
        {value ? (
          tokenSegments.length === 3 ? (
            <>
              <span className="text-orange-500">{tokenSegments[0]}</span>
              <span className="text-foreground">.</span>
              <span className="text-violet-500">{tokenSegments[1]}</span>
              <span className="text-foreground">.</span>
              <span className="text-emerald-500">{tokenSegments[2]}</span>
            </>
          ) : (
            <span className="text-foreground">{value}</span>
          )
        ) : (
          <span className="text-muted-foreground">{placeholder}</span>
        )}
      </pre>
      <Textarea
        ref={textareaRef}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onScroll={syncScroll}
        spellCheck={false}
        autoFocus={autoFocus}
        className="absolute inset-0 z-10 min-h-[260px] resize-none border-0 bg-transparent p-4 font-mono text-sm leading-6 text-foreground caret-foreground focus-visible:ring-0 xl:min-h-[360px]"
        style={{
          margin: 0,
          background: 'none',
          boxSizing: 'border-box',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          overflowWrap: 'break-word',
          overflow: 'auto',
          WebkitFontSmoothing: 'antialiased',
          WebkitTextFillColor: 'transparent',
        }}
      />
    </div>
  )
}

const EncodedJwtOutputPanel = ({
  value,
  placeholder,
  className,
}: {
  value: string
  placeholder: string
  className?: string
}) => {
  const tokenSegments = value.split('.')
  const hasThreeSegments = tokenSegments.length === 3

  return (
    <div className={cn('flex h-full min-h-0 flex-col overflow-hidden rounded-[5px] border border-border/70 bg-card/80', className)}>
      <div className="flex flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-5 py-2.5">
        <div className="font-mono text-sm font-semibold text-muted-foreground">&gt;_ Encoded JWT</div>
        <Button variant="ghost" size="icon" onClick={() => copyToClipboard(value)} disabled={!value}>
          <Copy className="h-4 w-4" />
        </Button>
      </div>
      <div className="flex min-h-0 flex-1 p-4">
        <div className="h-full min-h-0 flex-1 rounded-[5px] border border-border/70 bg-background/70 p-4 font-mono text-sm leading-6 whitespace-pre-wrap break-all">
          {value ? (
            hasThreeSegments ? (
              <>
                <span className="text-orange-500">{tokenSegments[0]}</span>
                <span className="text-foreground">.</span>
                <span className="text-violet-500">{tokenSegments[1]}</span>
                <span className="text-foreground">.</span>
                <span className="text-emerald-500">{tokenSegments[2]}</span>
              </>
            ) : (
              <span className="text-foreground">{value}</span>
            )
          ) : (
            <span className="text-muted-foreground">{placeholder}</span>
          )}
        </div>
      </div>
    </div>
  )
}

export default function JwtToolPage() {
  const [jwtMode, setJwtMode] = useState<'decoder' | 'encoder'>('decoder')
  const [autoFocusEnabled, setAutoFocusEnabled] = useState(true)
  const [jwtToken, setJwtToken] = useState(exampleToken)
  const [manualSecret, setManualSecret] = useState(exampleSecret)
  const [jwtDictOptions, setJwtDictOptions] = useState<JwtDictionaryItem[]>([])
  const [selectedJwtDict, setSelectedJwtDict] = useState('')
  const [jwtDictLoading, setJwtDictLoading] = useState(false)
  const [jwtDictImporting, setJwtDictImporting] = useState(false)
  const [jwtBruteForcing, setJwtBruteForcing] = useState(false)
  const [jwtAttempts, setJwtAttempts] = useState(0)
  const [jwtTotalCandidates, setJwtTotalCandidates] = useState(0)
  const [secretIsBase64UrlEncoded, setSecretIsBase64UrlEncoded] = useState(false)
  const [manualVerifyResult, setManualVerifyResult] = useState<MatchResult | null>(null)
  const [headerEditor, setHeaderEditor] = useState('')
  const [payloadEditor, setPayloadEditor] = useState('')
  const syncFromRawRef = useRef(false)
  const bruteForceAbortRef = useRef<AbortController | null>(null)
  const jwtDictFileInputRef = useRef<HTMLInputElement>(null)
  const useBase64Variants = true

  const parsedJwt = useMemo(() => {
    if (!jwtToken.trim()) {
      return { token: null as ParsedJwtToken | null, error: '' }
    }

    try {
      return { token: parseJwtToken(jwtToken), error: '' }
    } catch (error) {
      return { token: null, error: error instanceof Error ? error.message : 'JWT 解析失败' }
    }
  }, [jwtToken])

  useEffect(() => {
    if (!parsedJwt.token) {
      return
    }
    const timer = window.setTimeout(() => {
      if (!parsedJwt.token) {
        return
      }
      syncFromRawRef.current = true
      setHeaderEditor(formatJsonPreservingLargeIntegers(parsedJwt.token.headerText))
      setPayloadEditor(formatJsonPreservingLargeIntegers(parsedJwt.token.payloadText))
    }, 0)
    return () => window.clearTimeout(timer)
  }, [parsedJwt.token])

  const parsedHeaderEditor = useMemo(() => {
    if (!headerEditor.trim()) return { value: null as Record<string, unknown> | null, error: 'Header 不能为空' }
    try {
      const parsed = parseJsonPreservingLargeIntegers(headerEditor)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('Header 必须是 JSON 对象')
      }
      return { value: parsed as Record<string, unknown>, error: '' }
    } catch (error) {
      return { value: null, error: error instanceof Error ? error.message : 'Header JSON 无效' }
    }
  }, [headerEditor])

  const encoderHeaderAlgorithm = useMemo(() => {
    const value = parsedHeaderEditor.value?.alg
    if (typeof value !== 'string' || !value.trim()) return 'none'
    const matchedOption = jwtAlgorithmOptions.find((option) => (option.alg ?? option.value) === value)
    return matchedOption?.value ?? value
  }, [parsedHeaderEditor.value])

  const parsedPayloadEditor = useMemo(() => {
    if (!payloadEditor.trim()) return { value: null as Record<string, unknown> | null, error: 'Payload 不能为空' }
    try {
      const parsed = parseJsonPreservingLargeIntegers(payloadEditor)
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('Payload 必须是 JSON 对象')
      }
      return { value: parsed as Record<string, unknown>, error: '' }
    } catch (error) {
      return { value: null, error: error instanceof Error ? error.message : 'Payload JSON 无效' }
    }
  }, [payloadEditor])

  const generatedJwt = useMemo(() => {
    if (!parsedHeaderEditor.value || !parsedPayloadEditor.value) {
      return { token: '', error: parsedHeaderEditor.error || parsedPayloadEditor.error }
    }

    const algorithm =
      typeof parsedHeaderEditor.value.alg === 'string' ? String(parsedHeaderEditor.value.alg).toUpperCase() : ''
    if (!algorithm) {
      return { token: '', error: 'Header 中缺少 alg 字段' }
    }
    if (!hmacAlgorithms[algorithm]) {
      return { token: '', error: `暂不支持 ${algorithm} 算法自动签名` }
    }
    if (!manualSecret.trim()) {
      return { token: '', error: '请输入 Secret 后才能生成新的 JWT' }
    }
    if (secretIsBase64UrlEncoded && !tryDecodeBase64Bytes(manualSecret)) {
      return { token: '', error: 'BASE64URL ENCODED 已开启，但 Secret 不是有效编码' }
    }

    return { token: '__pending__', error: '' }
  }, [parsedHeaderEditor, parsedPayloadEditor, manualSecret, secretIsBase64UrlEncoded])

  const sameHeaderSemanticsAsOriginal = useMemo(() => {
    if (!parsedJwt.token || !parsedHeaderEditor.value) return false
    return JSON.stringify(parsedJwt.token.header) === JSON.stringify(parsedHeaderEditor.value)
  }, [parsedHeaderEditor.value, parsedJwt.token])

  const samePayloadSemanticsAsOriginal = useMemo(() => {
    if (!parsedJwt.token || !parsedPayloadEditor.value) return false
    return JSON.stringify(parsedJwt.token.payload) === JSON.stringify(parsedPayloadEditor.value)
  }, [parsedJwt.token, parsedPayloadEditor.value])

  const canVerifySecret = Boolean(parsedJwt.token && hmacAlgorithms[parsedJwt.token.algorithm])
  const canRunManualVerification = Boolean(manualSecret.trim() && canVerifySecret)
  const visibleManualVerifyResult = canRunManualVerification ? manualVerifyResult : null

  useEffect(() => {
    let cancelled = false

    const run = async () => {
      if (syncFromRawRef.current) {
        syncFromRawRef.current = false
        return
      }

      if (jwtMode !== 'encoder') {
        return
      }

      if (generatedJwt.token !== '__pending__' || !parsedHeaderEditor.value || !parsedPayloadEditor.value) {
        return
      }

      try {
        const headerText = headerEditor.trim()
        const payloadText = payloadEditor.trim()
        const headerSegment =
          sameHeaderSemanticsAsOriginal && parsedJwt.token?.headerSegment
            ? parsedJwt.token.headerSegment
            : encodeJsonSegment(headerText)
        const payloadSegment =
          samePayloadSemanticsAsOriginal && parsedJwt.token?.payloadSegment
            ? parsedJwt.token.payloadSegment
            : encodeJsonSegment(payloadText)
        const signingInput = `${headerSegment}.${payloadSegment}`
        const secretBytes = visibleManualVerifyResult?.usedFixedLengthSecret
          ? normalizeHmacSecretLength(
              String(parsedHeaderEditor.value.alg).toUpperCase(),
              getSecretBytes(manualSecret, secretIsBase64UrlEncoded)
            )
          : getSecretBytes(manualSecret, secretIsBase64UrlEncoded)
        const signature = await signJwtHmac(
          String(parsedHeaderEditor.value.alg).toUpperCase(),
          signingInput,
          secretBytes
        )

        if (!cancelled) {
          const nextToken = `${signingInput}.${encodeBase64Url(signature)}`
          setJwtToken((current) => (current === nextToken ? current : nextToken))
        }
      } catch (error) {
        if (!cancelled) {
          console.error('JWT regenerate failed:', error)
        }
      }
    }

    run()
    return () => {
      cancelled = true
    }
  }, [generatedJwt, headerEditor, jwtMode, manualSecret, parsedHeaderEditor.value, parsedJwt.token?.headerSegment, parsedJwt.token?.payloadSegment, parsedPayloadEditor.value, payloadEditor, sameHeaderSemanticsAsOriginal, samePayloadSemanticsAsOriginal, secretIsBase64UrlEncoded, visibleManualVerifyResult?.usedFixedLengthSecret])

  const loadJwtDicts = useCallback(async (preferredDictName?: string) => {
    setJwtDictLoading(true)
    try {
      const response = await fetchWithAuth('/api/v1/dict/crack?group=jwt')
      const data = await response.json()
      if (!response.ok || !data.success) {
        throw new Error(data.error || '加载字典列表失败')
      }

      const dicts = Array.isArray(data.data?.dicts) ? data.data.dicts : []
      const normalizedDicts: JwtDictionaryItem[] = dicts.map((item: Record<string, unknown>) => ({
        name: String(item.name || ''),
        protocol: String(item.protocol || 'Unknown'),
        group: String(item.group || 'jwt'),
        source: String(item.source || 'jwt'),
        entries: Number(item.entries || 0),
        size: String(item.size || '-'),
        description: String(item.description || ''),
      }))
      setJwtDictOptions(normalizedDicts)
      setSelectedJwtDict((current) => {
        if (preferredDictName && normalizedDicts.some((dict) => dict.name === preferredDictName)) {
          return preferredDictName
        }
        if (current && normalizedDicts.some((dict) => dict.name === current)) {
          return current
        }
        return normalizedDicts[0]?.name || ''
      })
    } catch (error) {
      console.error('Failed to load JWT dict list:', error)
      toast.error(error instanceof Error ? error.message : '加载 JWT 字典列表失败')
    } finally {
      setJwtDictLoading(false)
    }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadJwtDicts()
    }, 0)
    return () => window.clearTimeout(timer)
  }, [loadJwtDicts])

  const handleJwtDictImport = useCallback(
    async (event: ChangeEvent<HTMLInputElement>) => {
      const files = event.target.files
      if (!files?.length) return

      setJwtDictImporting(true)
      let lastImportedName = ''

      try {
        for (const file of Array.from(files)) {
          if (!file.name.toLowerCase().endsWith('.txt')) {
            toast.error(`${file.name} 不是 .txt 文件`)
            continue
          }

          const content = await file.text()
          const response = await fetchWithAuth('/api/v1/dict/crack/content', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              name: file.name,
              source: 'jwt',
              protocol: 'JWT',
              group: 'jwt',
              description: '',
              content,
            }),
          })
          const data = await response.json().catch(() => ({}))
          if (!response.ok || !data.success) {
            throw new Error(data.error || `${file.name} 导入失败`)
          }

          lastImportedName = file.name
          toast.success(`${file.name} 已导入 JWT 字典`)
        }

        await loadJwtDicts(lastImportedName || undefined)
      } catch (error) {
        console.error('JWT dict import failed:', error)
        toast.error(error instanceof Error ? error.message : '导入 JWT 字典失败')
      } finally {
        event.target.value = ''
        setJwtDictImporting(false)
      }
    },
    [loadJwtDicts]
  )

  const verifyManualSecret = useCallback(
    async (candidate: string): Promise<MatchResult> => {
      if (!parsedJwt.token) {
        throw new Error(parsedJwt.error || '请输入有效的 JWT')
      }
      const baseLabel = secretIsBase64UrlEncoded ? 'Base64URL 解码后校验' : '原始文本'
      const originalSecretBytes = getSecretBytes(candidate, secretIsBase64UrlEncoded)

      for (const secretCandidate of getHmacSecretCandidates(parsedJwt.token.algorithm, originalSecretBytes)) {
        const signature = await signJwtHmac(parsedJwt.token.algorithm, parsedJwt.token.signingInput, secretCandidate.bytes)
        if (constantTimeEqual(signature, parsedJwt.token.signatureBytes)) {
          return {
            matched: true,
            variantLabel: secretCandidate.usedFixedLengthSecret ? `${baseLabel} + 固定长度兼容` : baseLabel,
            displayValue: candidate,
            secretPreview: encodeBase64Url(secretCandidate.bytes),
            usedFixedLengthSecret: secretCandidate.usedFixedLengthSecret,
          }
        }
      }

      return { matched: false }
    },
    [parsedJwt, secretIsBase64UrlEncoded]
  )

  useEffect(() => {
    let cancelled = false

    if (!canRunManualVerification) {
      return
    }

    const timer = window.setTimeout(async () => {
      try {
        const result = await verifyManualSecret(manualSecret)
        if (!cancelled) {
          setManualVerifyResult(result)
        }
      } catch (error) {
        if (!cancelled) {
          console.error('manual verify failed:', error)
          setManualVerifyResult(null)
        }
      }
    }, 180)

    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [canRunManualVerification, manualSecret, verifyManualSecret])

  const stopJwtBruteforce = useCallback(() => {
    if (!jwtBruteForcing) return
    bruteForceAbortRef.current?.abort()
    bruteForceAbortRef.current = null
    setJwtBruteForcing(false)
    toast.error('爆破已停止')
  }, [jwtBruteForcing])

  const runJwtBruteforce = useCallback(async () => {
    if (!parsedJwt.token) {
      toast.error(parsedJwt.error || '请输入有效的 JWT')
      return
    }
    if (!selectedJwtDict) {
      toast.error('请先选择一个 JWT 字典')
      return
    }
    if (!hmacAlgorithms[parsedJwt.token.algorithm]) {
      toast.error(`暂不支持 ${parsedJwt.token.algorithm || 'unknown'} 算法的签名爆破`)
      return
    }

    const controller = new AbortController()
    bruteForceAbortRef.current = controller
    setJwtBruteForcing(true)
    setJwtAttempts(0)
    setJwtTotalCandidates(0)

    try {
      const response = await fetchWithAuth('/api/v1/tools/jwt/bruteforce', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          token: parsedJwt.token.raw,
          dictionary: selectedJwtDict,
          source: 'jwt',
          tryBase64Variants: useBase64Variants,
        }),
        signal: controller.signal,
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'JWT 爆破失败')
      }

      const result = (data.data || {}) as BruteforceResult
      setJwtAttempts(result.attempts || 0)
      setJwtTotalCandidates(result.totalCandidates || 0)

      if (result.unsupportedAlgorithm) {
        toast.error(`暂不支持 ${result.unsupportedAlgorithm} 算法的签名爆破`)
        return
      }

      if (result.matched) {
        if (result.displayValue) {
          const nextSecretIsBase64UrlEncoded = Boolean(result.secretIsBase64UrlEncoded)
          setManualSecret(result.displayValue)
          setSecretIsBase64UrlEncoded(nextSecretIsBase64UrlEncoded)
          setManualVerifyResult({
            matched: true,
            displayValue: result.displayValue,
            dictionaryEntry: result.dictionaryEntry,
            variantLabel: result.variantLabel,
            secretPreview: result.secretPreview,
            secretIsBase64UrlEncoded: nextSecretIsBase64UrlEncoded,
            usedFixedLengthSecret: result.usedFixedLengthSecret,
          })
        }
        toast.success(`命中密钥，尝试次数 ${result.attempts}`)
        return
      }

      toast.error(`未命中字典，共尝试 ${result.attempts} 次`)
    } catch (error: any) {
      if (error?.name === 'AbortError') {
        return
      }
      console.error('JWT brute force failed:', error)
      toast.error(error instanceof Error ? error.message : 'JWT 爆破失败')
    } finally {
      bruteForceAbortRef.current = null
      setJwtBruteForcing(false)
    }
  }, [parsedJwt, selectedJwtDict, useBase64Variants])

  return (
    <div className="min-h-full bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.08),transparent_28%),linear-gradient(180deg,hsl(var(--background)),hsl(var(--muted))/0.22)] text-foreground">
      <div className="mx-auto flex w-full max-w-[1760px] flex-col gap-6 px-4 py-4">
        <div className="mx-auto flex rounded-full border border-border/70 bg-muted/50 p-1 shadow-sm">
          <Button
            variant="ghost"
            onClick={() => setJwtMode('decoder')}
            className={cn('h-10 rounded-full px-6 text-base', jwtMode === 'decoder' && 'bg-background shadow-sm')}
          >
            JWT Decoder
          </Button>
          <Button
            variant="ghost"
            onClick={() => setJwtMode('encoder')}
            className={cn('h-10 rounded-full px-6 text-base', jwtMode === 'encoder' && 'bg-background shadow-sm')}
          >
            JWT Encoder
          </Button>
        </div>

        {jwtMode === 'decoder' ? (
          <div className="grid min-w-0 grid-cols-1 gap-5 [&>div]:min-w-0 xl:grid-cols-[minmax(0,1.02fr)_minmax(420px,0.98fr)] xl:items-stretch">
            <div className="flex h-full flex-col gap-3">
              <div className="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <h2 className="text-lg font-semibold">Encoded Token</h2>
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Switch checked={autoFocusEnabled} onCheckedChange={setAutoFocusEnabled} />
                  Enable auto-focus
                </div>
              </div>

              <Card className="flex flex-1 flex-col gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0 shadow-[0_18px_55px_rgba(0,0,0,0.13)]">
                <div className="flex flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-5 py-2.5">
                  <div className="font-mono text-sm font-semibold text-muted-foreground">&gt;_ JSON Web Token (JWT)</div>
                  <div className="flex items-center gap-2">
                    <Button variant="ghost" size="icon" onClick={() => copyToClipboard(jwtToken)} disabled={!jwtToken}>
                      <Copy className="h-4 w-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => {
                        setJwtToken('')
                        setManualVerifyResult(null)
                        setJwtAttempts(0)
                        setJwtTotalCandidates(0)
                      }}
                    >
                      <XCircle className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <CardContent className="flex flex-1 flex-col gap-2.5 p-3">
                  <HighlightedJwtEditor
                    className="flex-1"
                    autoFocus={autoFocusEnabled}
                    value={jwtToken}
                    onChange={(nextValue) => {
                      setJwtMode('decoder')
                      setJwtToken(nextValue)
                      setManualVerifyResult(null)
                      setJwtAttempts(0)
                      setJwtTotalCandidates(0)
                    }}
                    placeholder="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
                  />
                  <div className="flex flex-wrap gap-3 text-sm">
                    <div className={cn('inline-flex items-center gap-2', parsedJwt.token ? 'text-emerald-500' : 'text-red-500')}>
                      {parsedJwt.token ? <CheckCircle2 className="h-4 w-4" /> : <XCircle className="h-4 w-4" />}
                      {parsedJwt.token ? 'Valid JWT' : 'Invalid JWT'}
                    </div>
                    <div className={cn('inline-flex items-center gap-2', visibleManualVerifyResult?.matched ? 'text-emerald-500' : 'text-muted-foreground')}>
                      <ShieldCheck className="h-4 w-4" />
                      {visibleManualVerifyResult?.matched ? 'Signature Verified' : 'Signature not verified'}
                    </div>
                  </div>
                </CardContent>
              </Card>
            </div>

            <div className="flex h-full flex-col gap-3">
              <h2 className="text-lg font-semibold">Decoded Header</h2>

              <div className="flex flex-1 flex-col gap-2">
                <Card className="flex flex-1 flex-col gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0">
                  <Tabs defaultValue="json" className="flex h-full w-full flex-col">
                    <div className="flex min-h-[57px] flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-5 py-2.5">
                      <TabsList className="h-9 gap-0 rounded-[5px] border border-border/70 bg-background/35 p-0.5">
                        <TabsTrigger value="json" className="rounded-[4px] px-4 py-1 text-sm text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground">
                          JSON
                        </TabsTrigger>
                        <TabsTrigger value="claims" className="rounded-[4px] px-4 py-1 text-sm text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground">
                          Claims Breakdown
                        </TabsTrigger>
                      </TabsList>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="icon" onClick={() => copyToClipboard(headerEditor)} disabled={!headerEditor}>
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                    <CardContent className="flex flex-1 flex-col p-4">
                      <TabsContent value="json" className="mt-0 flex-1">
                        <Textarea
                          value={headerEditor}
                          onChange={(e) => {
                            setJwtMode('encoder')
                            setHeaderEditor(e.target.value)
                          }}
                          placeholder='{"alg":"HS256","typ":"JWT"}'
                          className="min-h-[96px] resize-none rounded-[5px] border-border/70 bg-background/70 font-mono text-sm leading-6"
                        />
                      </TabsContent>
                      <TabsContent value="claims" className="mt-0 flex-1">
                        <ClaimsBreakdownPanel claims={parsedHeaderEditor.value} emptyLabel="Header 暂无可展示字段" />
                      </TabsContent>
                      {parsedHeaderEditor.error ? <div className="mt-2 text-xs text-red-500">{parsedHeaderEditor.error}</div> : null}
                    </CardContent>
                  </Tabs>
                </Card>
              </div>

              <div className="flex flex-1 flex-col gap-2">
                <h3 className="text-lg font-semibold">Decoded Payload</h3>
                <Card className="flex flex-1 flex-col gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0">
                  <Tabs defaultValue="json" className="flex h-full w-full flex-col">
                    <div className="flex min-h-[57px] flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-5 py-2.5">
                      <TabsList className="h-9 gap-0 rounded-[5px] border border-border/70 bg-background/35 p-0.5">
                        <TabsTrigger value="json" className="rounded-[4px] px-4 py-1 text-sm text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground">
                          JSON
                        </TabsTrigger>
                        <TabsTrigger value="claims" className="rounded-[4px] px-4 py-1 text-sm text-muted-foreground data-[state=active]:bg-background data-[state=active]:text-foreground">
                          Claims Breakdown
                        </TabsTrigger>
                      </TabsList>
                      <div className="flex items-center gap-1">
                        <Button variant="ghost" size="icon" onClick={() => copyToClipboard(payloadEditor)} disabled={!payloadEditor}>
                          <Copy className="h-4 w-4" />
                        </Button>
                      </div>
                    </div>
                    <CardContent className="flex flex-1 flex-col p-4">
                      <TabsContent value="json" className="mt-0 flex-1">
                        <Textarea
                          value={payloadEditor}
                          onChange={(e) => {
                            setJwtMode('encoder')
                            setPayloadEditor(e.target.value)
                          }}
                          placeholder='{"sub":"1234567890"}'
                          className="min-h-[136px] resize-none rounded-[5px] border-border/70 bg-background/70 font-mono text-sm leading-6"
                        />
                      </TabsContent>
                      <TabsContent value="claims" className="mt-0 flex-1">
                        <ClaimsBreakdownPanel claims={parsedPayloadEditor.value} emptyLabel="Payload 暂无可展示字段" />
                      </TabsContent>
                      {parsedPayloadEditor.error ? <div className="mt-2 text-xs text-red-500">{parsedPayloadEditor.error}</div> : null}
                    </CardContent>
                  </Tabs>
                </Card>
              </div>

              <div className="flex flex-1 flex-col gap-2">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <h3 className="text-lg font-semibold">
                      JWT Signature Verification <span className="font-normal text-muted-foreground">(Optional)</span>
                    </h3>
                  </div>
                  <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
                    BASE64URL ENCODED
                    <Switch checked={secretIsBase64UrlEncoded} onCheckedChange={setSecretIsBase64UrlEncoded} />
                  </div>
                </div>
                <Card className="flex flex-1 flex-col gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0">
                  <div className="flex flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-4 py-3">
                    <div className="flex items-center gap-2 font-mono text-sm font-semibold text-muted-foreground">
                      <KeyRound className="h-4 w-4" />
                      &gt;_ Secret
                    </div>
                    <div className="flex items-center gap-1">
                      <Button variant="ghost" size="icon" onClick={() => copyToClipboard(manualSecret)} disabled={!manualSecret}>
                        <Copy className="h-4 w-4" />
                      </Button>
                      <Button variant="ghost" size="icon" onClick={() => setManualSecret('')} disabled={!manualSecret}>
                        <XCircle className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                  <CardContent className="p-4">
                    <Textarea
                      value={manualSecret}
                      onChange={(e) => {
                        setManualSecret(e.target.value)
                        setManualVerifyResult(null)
                      }}
                      placeholder="a-string-secret-at-least-256-bits-long"
                      className="min-h-[120px] resize-none rounded-[5px] border-border/70 bg-background/70 px-4 py-4 font-mono text-sm leading-6 focus-visible:ring-0"
                    />
                    <div
                      role="status"
                      className={cn(
                        'mt-4 flex items-center justify-between rounded-[5px] border px-4 py-3 text-sm',
                        (!manualSecret.trim() || !canVerifySecret) && 'border-border/70 bg-muted/20 text-muted-foreground',
                        manualSecret.trim() && canVerifySecret && !visibleManualVerifyResult?.matched && 'border-red-500/25 bg-red-500/8 text-red-600 dark:text-red-300',
                        visibleManualVerifyResult?.matched && 'border-emerald-500/20 bg-emerald-500/8 text-emerald-700 dark:text-emerald-300'
                      )}
                    >
                      <div className="flex items-center gap-2">
                        {visibleManualVerifyResult?.matched ? (
                          <CheckCircle2 className="h-4 w-4" />
                        ) : manualSecret.trim() && canVerifySecret ? (
                          <XCircle className="h-4 w-4" />
                        ) : (
                          <ShieldCheck className="h-4 w-4" />
                        )}
                        <span>
                          {visibleManualVerifyResult?.matched
                            ? 'Valid secret'
                            : manualSecret.trim() && canVerifySecret
                              ? 'Invalid secret'
                              : canVerifySecret
                                ? 'Awaiting secret'
                                : 'Verification unavailable'}
                        </span>
                      </div>
                    </div>
                    <div className="mt-4 flex flex-wrap items-center gap-3">
                      <Select value={selectedJwtDict} onValueChange={setSelectedJwtDict}>
                        <SelectTrigger className="min-w-[240px] rounded-[5px] border-border/70 bg-background/50">
                          <SelectValue placeholder={jwtDictLoading ? '加载字典中...' : '选择已有 JWT 字典'} />
                        </SelectTrigger>
                        <SelectContent>
                          {jwtDictOptions.map((dict) => (
                            <SelectItem key={`${dict.source}:${dict.name}`} value={dict.name}>
                              {dict.name} · {dict.protocol}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <input
                        ref={jwtDictFileInputRef}
                        type="file"
                        accept=".txt,text/plain"
                        multiple
                        className="hidden"
                        onChange={handleJwtDictImport}
                      />
                      <Button
                        variant="outline"
                        onClick={() => jwtDictFileInputRef.current?.click()}
                        disabled={jwtDictImporting}
                      >
                        <Upload className="mr-2 h-4 w-4" />
                        {jwtDictImporting ? '导入中...' : '导入字典'}
                      </Button>
                      {jwtBruteForcing ? (
                        <Button variant="destructive" onClick={stopJwtBruteforce}>
                          <PauseCircle className="mr-2 h-4 w-4" />
                          停止识别
                        </Button>
                      ) : (
                        <Button onClick={runJwtBruteforce} disabled={!parsedJwt.token || !selectedJwtDict}>
                          密钥暴破
                        </Button>
                      )}
                    </div>
                  </CardContent>
                  {generatedJwt.error ? (
                    <div className="border-t border-amber-500/35 bg-amber-500/12 px-4 py-3 text-xs leading-5 text-amber-700 dark:text-amber-200">
                      {generatedJwt.error}
                    </div>
                  ) : null}
                  <div className="border-t border-border/70 bg-muted/10 px-4 py-3">
                    <div className="flex items-center justify-between text-xs text-muted-foreground">
                      <span>Progress</span>
                      <span>
                        {jwtBruteForcing ? '识别中' : jwtAttempts > 0 ? `${jwtAttempts} / ${jwtTotalCandidates || jwtAttempts}` : ''}
                      </span>
                    </div>
                    {jwtBruteForcing ? (
                      <div className="mt-3 h-2 w-full overflow-hidden rounded-full bg-primary/20">
                        <div className="h-full w-full animate-pulse bg-primary" />
                      </div>
                    ) : (
                      <Progress value={jwtAttempts > 0 ? 100 : 0} className="mt-3 h-2" />
                    )}
                  </div>
                </Card>
              </div>
            </div>
          </div>
        ) : (
          <div className="grid min-w-0 grid-cols-1 gap-5 [&>div]:min-w-0 xl:grid-cols-[minmax(0,0.98fr)_minmax(420px,1.02fr)] xl:items-stretch">
            <div className="flex h-full min-h-0 flex-col gap-6">
              <div className="space-y-2">
                <h2 className="text-lg font-semibold">Header</h2>
                <Card className="gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0">
                  <div className="flex flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-5 py-2.5">
                    <div className="font-mono text-sm font-semibold text-muted-foreground">&gt;_ Algorithm &amp; Token Type</div>
                    <div className="flex items-center gap-2">
                      <Select
                        value={encoderHeaderAlgorithm}
                        onValueChange={(value) => {
                          setHeaderEditor((current) => updateHeaderAlgorithm(current, value))
                        }}
                      >
                        <SelectTrigger className="h-8 w-[110px] rounded-[5px] border-border/70 bg-background/60 text-xs">
                          <SelectValue placeholder="alg" />
                        </SelectTrigger>
                        <SelectContent>
                          {jwtAlgorithmOptions.map((algorithm) => (
                            <SelectItem key={algorithm.value} value={algorithm.value}>
                              {algorithm.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <Button variant="ghost" size="icon" onClick={() => setHeaderEditor('')} disabled={!headerEditor}>
                        <XCircle className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                  <CardContent className="p-4">
                    <Textarea
                      value={headerEditor}
                      onChange={(e) => setHeaderEditor(e.target.value)}
                      placeholder='{"alg":"HS256","typ":"JWT"}'
                      className="min-h-[96px] resize-none rounded-[5px] border-border/70 bg-background/70 font-mono text-sm leading-6"
                    />
                  </CardContent>
                  <div
                    className={cn(
                      'border-t px-5 py-3 text-sm',
                      parsedHeaderEditor.error
                        ? 'border-red-500/25 bg-red-500/8 text-red-600 dark:text-red-300'
                        : 'border-emerald-500/20 bg-emerald-500/8 text-emerald-700 dark:text-emerald-300'
                    )}
                  >
                    {parsedHeaderEditor.error ? parsedHeaderEditor.error : 'Valid header'}
                  </div>
                </Card>
              </div>

              <div className="space-y-2">
                <h2 className="text-lg font-semibold">Payload</h2>
                <Card className="gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0">
                  <div className="flex flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-5 py-2.5">
                    <div className="font-mono text-sm font-semibold text-muted-foreground">&gt;_ Data</div>
                    <Button variant="ghost" size="icon" onClick={() => setPayloadEditor('')} disabled={!payloadEditor}>
                      <XCircle className="h-4 w-4" />
                    </Button>
                  </div>
                  <CardContent className="p-4">
                    <Textarea
                      value={payloadEditor}
                      onChange={(e) => setPayloadEditor(e.target.value)}
                      placeholder='{"sub":"1234567890"}'
                      className="min-h-[136px] resize-none rounded-[5px] border-border/70 bg-background/70 font-mono text-sm leading-6"
                    />
                  </CardContent>
                  <div
                    className={cn(
                      'border-t px-5 py-3 text-sm',
                      parsedPayloadEditor.error
                        ? 'border-red-500/25 bg-red-500/8 text-red-600 dark:text-red-300'
                        : 'border-emerald-500/20 bg-emerald-500/8 text-emerald-700 dark:text-emerald-300'
                    )}
                  >
                    {parsedPayloadEditor.error ? parsedPayloadEditor.error : 'Valid payload'}
                  </div>
                </Card>
              </div>

              <div className="space-y-2">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <h2 className="text-lg font-semibold">Sign JWT</h2>
                  <div className="flex items-center gap-2 text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
                    BASE64URL ENCODED
                    <Switch checked={secretIsBase64UrlEncoded} onCheckedChange={setSecretIsBase64UrlEncoded} />
                  </div>
                </div>
                <Card className="gap-0 overflow-hidden rounded-[5px] border-border/70 bg-card/80 py-0">
                  <div className="flex flex-wrap items-center gap-2 justify-between border-b border-border/70 bg-muted/45 px-4 py-3">
                    <div className="flex items-center gap-2 font-mono text-sm font-semibold text-muted-foreground">
                      <KeyRound className="h-4 w-4" />
                      &gt;_ Secret
                    </div>
                    <Button variant="ghost" size="icon" onClick={() => setManualSecret('')} disabled={!manualSecret}>
                      <XCircle className="h-4 w-4" />
                    </Button>
                  </div>
                  <CardContent className="p-4">
                    <Textarea
                      value={manualSecret}
                      onChange={(e) => setManualSecret(e.target.value)}
                      placeholder="a-string-secret-at-least-256-bits-long"
                      className="min-h-[120px] resize-none rounded-[5px] border-border/70 bg-background/70 px-4 py-4 font-mono text-sm leading-6 focus-visible:ring-0"
                    />
                  </CardContent>
                </Card>
              </div>
            </div>

            <div className="flex h-full min-h-0 flex-col gap-3">
              <h2 className="text-lg font-semibold">JWT Signature</h2>
              <EncodedJwtOutputPanel
                className="flex-1"
                value={jwtToken}
                placeholder="The signed JWT will appear here after the header, payload, and secret are valid."
              />
            </div>
          </div>
        )}

      </div>
    </div>
  )
}
