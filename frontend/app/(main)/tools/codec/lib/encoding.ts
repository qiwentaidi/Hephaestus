// 编码/解码函数

type ByteEncodingMode = 'base64' | 'hex' | 'raw'
type Base64DataType = 'raw' | 'utf8' | 'hex'

const getByteEncodingMode = (value?: string): ByteEncodingMode => {
  switch ((value || '').toLowerCase()) {
    case 'hex':
      return 'hex'
    case 'raw':
      return 'raw'
    default:
      return 'base64'
  }
}

const bytesToBase64 = (bytes: Uint8Array): string => {
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary)
}

const base64ToBytes = (input: string): Uint8Array => {
  const normalized = input.trim().replace(/\s+/g, '')
  const binary = atob(normalized)
  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}

const hexToBytes = (input: string): Uint8Array => {
  const normalized = input.replace(/\s+/g, '')
  if (normalized.length % 2 !== 0) {
    throw new Error('无效的 Hex 长度')
  }
  if (!/^[0-9a-fA-F]*$/.test(normalized)) {
    throw new Error('包含无效的 Hex 字符')
  }

  const bytes = new Uint8Array(normalized.length / 2)
  for (let i = 0; i < normalized.length; i += 2) {
    bytes[i / 2] = parseInt(normalized.slice(i, i + 2), 16)
  }
  return bytes
}

const bytesToHex = (bytes: Uint8Array): string => {
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('')
}

const latin1ToBytes = (input: string): Uint8Array => {
  return Uint8Array.from(input, (char) => char.charCodeAt(0) & 0xff)
}

const bytesToLatin1 = (bytes: Uint8Array): string => {
  return Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')
}

const getBase64DataType = (value?: string): Base64DataType => {
  switch ((value || '').toLowerCase()) {
    case 'utf8':
      return 'utf8'
    case 'hex':
      return 'hex'
    default:
      return 'raw'
  }
}

const parseBase64Data = (input: string, type?: string): Uint8Array => {
  switch (getBase64DataType(type)) {
    case 'utf8':
      return new TextEncoder().encode(input)
    case 'hex':
      return hexToBytes(input)
    default:
      return latin1ToBytes(input)
  }
}

const formatBase64Data = (bytes: Uint8Array, type?: string): string => {
  switch (getBase64DataType(type)) {
    case 'utf8':
      return new TextDecoder().decode(bytes)
    case 'hex':
      return bytesToHex(bytes)
    default:
      return bytesToLatin1(bytes)
  }
}

const parseBytesByMode = (input: string, mode?: string): Uint8Array => {
  switch (getByteEncodingMode(mode)) {
    case 'hex':
      return hexToBytes(input)
    case 'raw':
      return latin1ToBytes(input)
    default:
      return base64ToBytes(input)
  }
}

const formatBytesByMode = (bytes: Uint8Array, mode?: string): string => {
  switch (getByteEncodingMode(mode)) {
    case 'hex':
      return bytesToHex(bytes)
    case 'raw':
      return bytesToLatin1(bytes)
    default:
      return bytesToBase64(bytes)
  }
}

const encodeJavaModifiedUtf8 = (input: string): Uint8Array => {
  const bytes: number[] = []

  for (let i = 0; i < input.length; i++) {
    const codeUnit = input.charCodeAt(i)
    if (codeUnit >= 0x0001 && codeUnit <= 0x007f) {
      bytes.push(codeUnit)
      continue
    }
    if (codeUnit <= 0x07ff) {
      bytes.push(0xc0 | (codeUnit >> 6))
      bytes.push(0x80 | (codeUnit & 0x3f))
      continue
    }
    bytes.push(0xe0 | (codeUnit >> 12))
    bytes.push(0x80 | ((codeUnit >> 6) & 0x3f))
    bytes.push(0x80 | (codeUnit & 0x3f))
  }

  return Uint8Array.from(bytes)
}

const decodeJavaModifiedUtf8 = (bytes: Uint8Array): string => {
  const codeUnits: number[] = []

  for (let i = 0; i < bytes.length;) {
    const first = bytes[i]
    if ((first & 0x80) === 0) {
      codeUnits.push(first)
      i += 1
      continue
    }
    if ((first & 0xe0) === 0xc0) {
      if (i + 1 >= bytes.length) {
        throw new Error('MUTF-8 数据截断')
      }
      const second = bytes[i + 1]
      if ((second & 0xc0) !== 0x80) {
        throw new Error('MUTF-8 双字节序列无效')
      }
      codeUnits.push(((first & 0x1f) << 6) | (second & 0x3f))
      i += 2
      continue
    }
    if ((first & 0xf0) === 0xe0) {
      if (i + 2 >= bytes.length) {
        throw new Error('MUTF-8 数据截断')
      }
      const second = bytes[i + 1]
      const third = bytes[i + 2]
      if ((second & 0xc0) !== 0x80 || (third & 0xc0) !== 0x80) {
        throw new Error('MUTF-8 三字节序列无效')
      }
      codeUnits.push(((first & 0x0f) << 12) | ((second & 0x3f) << 6) | (third & 0x3f))
      i += 3
      continue
    }
    throw new Error('不支持的 MUTF-8 序列')
  }

  return String.fromCharCode(...codeUnits)
}

const JAVA_STREAM_MAGIC = [0xac, 0xed, 0x00, 0x05]
const TC_STRING = 0x74
const TC_LONGSTRING = 0x7c

interface ParsedJavaStringStream {
  magic: string
  version: number
  typeCode: number
  typeName: 'TC_STRING' | 'TC_LONGSTRING'
  utfLength: number
  value: string
  payloadHex: string
  streamHex: string
}

const parseJavaSerializedStringStream = (input: string, inputMode?: string): ParsedJavaStringStream => {
  const bytes = parseBytesByMode(input, inputMode)
  if (bytes.length < 5) {
    throw new Error('输入长度不足')
  }
  for (let i = 0; i < JAVA_STREAM_MAGIC.length; i++) {
    if (bytes[i] !== JAVA_STREAM_MAGIC[i]) {
      throw new Error('不是有效的 Java 序列化流')
    }
  }

  const typeCode = bytes[4]
  let offset = 5
  let length = 0
  let typeName: ParsedJavaStringStream['typeName']

  if (typeCode === TC_STRING) {
    if (bytes.length < offset + 2) {
      throw new Error('String 长度字段缺失')
    }
    length = (bytes[offset] << 8) | bytes[offset + 1]
    offset += 2
    typeName = 'TC_STRING'
  } else if (typeCode === TC_LONGSTRING) {
    if (bytes.length < offset + 8) {
      throw new Error('Long String 长度字段缺失')
    }
    let longLength = 0
    for (let i = 0; i < 8; i++) {
      longLength = longLength * 256 + bytes[offset + i]
    }
    if (!Number.isSafeInteger(longLength)) {
      throw new Error('Long String 长度过大')
    }
    length = longLength
    offset += 8
    typeName = 'TC_LONGSTRING'
  } else {
    throw new Error(`暂不支持的对象类型: 0x${typeCode.toString(16)}`)
  }

  if (bytes.length < offset + length) {
    throw new Error('序列化流内容不完整')
  }

  const payload = bytes.slice(offset, offset + length)
  return {
    magic: '0xaced',
    version: (bytes[2] << 8) | bytes[3],
    typeCode,
    typeName,
    utfLength: length,
    value: decodeJavaModifiedUtf8(payload),
    payloadHex: bytesToHex(payload),
    streamHex: bytesToHex(bytes),
  }
}

export const javaSerialize = (input: string, options?: Record<string, string>): string => {
  try {
    const payload = encodeJavaModifiedUtf8(input)
    const bytes: number[] = [...JAVA_STREAM_MAGIC]

    if (payload.length <= 0xffff) {
      bytes.push(TC_STRING, (payload.length >> 8) & 0xff, payload.length & 0xff)
    } else {
      bytes.push(TC_LONGSTRING)
      let remaining = payload.length
      const lengthBytes = new Array<number>(8).fill(0)
      for (let i = 7; i >= 0; i--) {
        lengthBytes[i] = remaining & 0xff
        remaining = Math.floor(remaining / 256)
      }
      bytes.push(...lengthBytes)
    }

    bytes.push(...payload)
    return formatBytesByMode(Uint8Array.from(bytes), options?.outputMode)
  } catch (error) {
    return `[编码错误: ${error instanceof Error ? error.message : 'Java 序列化失败'}]`
  }
}

export const javaDeserialize = (input: string, options?: Record<string, string>): string => {
  try {
    const parsed = parseJavaSerializedStringStream(input, options?.inputMode)
    const outputMode = (options?.outputMode || 'object-stream').toLowerCase()

    if (outputMode === 'dumper') {
      return [
        'Java Serialization Dumper',
        `STREAM_MAGIC: ${parsed.magic}`,
        `STREAM_VERSION: ${parsed.version}`,
        `CONTENT_TYPE: ${parsed.typeName} (0x${parsed.typeCode.toString(16)})`,
        `UTF_LENGTH: ${parsed.utfLength}`,
        `PAYLOAD_HEX: ${parsed.payloadHex}`,
        `VALUE: ${parsed.value}`,
      ].join('\n')
    }

    return JSON.stringify(
      {
        streamMagic: parsed.magic,
        streamVersion: parsed.version,
        contentType: parsed.typeName,
        typeCode: `0x${parsed.typeCode.toString(16)}`,
        utfLength: parsed.utfLength,
        value: parsed.value,
        streamHex: parsed.streamHex,
      },
      null,
      2
    )
  } catch (error) {
    return `[解码错误: ${error instanceof Error ? error.message : 'Java 反序列化失败'}]`
  }
}

// Base64 编解码 (支持多种模式)
export const base64Encode = (input: string, options?: Record<string, string>): string => {
  try {
    const mode = (options?.mode || 'Standard').toUpperCase()
    const inputType = options?.inputType || 'Raw'
    const bytes = parseBase64Data(input, inputType)
    let result = ''
    
    // 标准 Base64 编码
    const base64 = btoa(String.fromCharCode(...bytes))
    
    if (mode === 'URL') {
      // URL 安全模式: 替换 +/ 为 -_，移除填充
      result = base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
    } else if (mode === 'MIME') {
      // MIME 模式: 每76字符添加换行 (RFC 2045)
      for (let i = 0; i < base64.length; i += 76) {
        result += base64.slice(i, i + 76) + '\n'
      }
    } else {
      // 标准模式
      result = base64
    }
    
    return result
  } catch {
    return '[编码错误: 无效输入]'
  }
}

export const base64Decode = (input: string, options?: Record<string, string>): string => {
  try {
    const mode = (options?.mode || 'Standard').toUpperCase()
    const outputType = options?.outputType || 'Raw'
    let base64 = input.trim()
    
    if (mode === 'URL') {
      // URL 安全模式: 替换 -_ 为 +/，添加填充
      base64 = base64.replace(/-/g, '+').replace(/_/g, '/')
      const padding = base64.length % 4
      if (padding) {
        base64 += '='.repeat(4 - padding)
      }
    } else if (mode === 'MIME') {
      // MIME 模式: 移除换行
      base64 = base64.replace(/\n/g, '')
    }
    
    const binaryString = atob(base64)
    const bytes = Uint8Array.from(binaryString, c => c.charCodeAt(0))
    return formatBase64Data(bytes, outputType)
  } catch {
    return '[解码错误: 无效的 Base64]'
  }
}

// Base32 字符表
const BASE32_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'

// Base32 编解码
export const base32Encode = (input: string, options?: Record<string, string>): string => {
  try {
    const padding = (options?.padding || 'True').toUpperCase() !== 'FALSE'
    const bytes = new TextEncoder().encode(input)
    let bits = ''
    let result = ''
    
    // 转换为二进制
    for (const byte of bytes) {
      bits += byte.toString(2).padStart(8, '0')
    }
    
    // 每5位转换为一个 Base32 字符
    for (let i = 0; i < bits.length; i += 5) {
      const chunk = bits.slice(i, i + 5).padEnd(5, '0')
      result += BASE32_ALPHABET[parseInt(chunk, 2)]
    }
    
    // 添加填充
    if (padding) {
      const padLength = (8 - (result.length % 8)) % 8
      result += '='.repeat(padLength)
    }
    
    return result
  } catch {
    return '[编码错误: 无效输入]'
  }
}

export const base32Decode = (input: string): string => {
  try {
    const base32 = input.trim().toUpperCase().replace(/=+$/, '')
    let bits = ''
    
    // 每个字符转换为5位二进制
    for (const char of base32) {
      const index = BASE32_ALPHABET.indexOf(char)
      if (index === -1) return '[解码错误: 无效的 Base32 字符]'
      bits += index.toString(2).padStart(5, '0')
    }
    
    // 每8位转换为一个字节
    const bytes: number[] = []
    for (let i = 0; i + 8 <= bits.length; i += 8) {
      bytes.push(parseInt(bits.slice(i, i + 8), 2))
    }
    
    return new TextDecoder().decode(Uint8Array.from(bytes))
  } catch {
    return '[解码错误: 无效的 Base32]'
  }
}

// URL 编解码
export const urlEncode = (input: string): string => {
  try {
    return encodeURIComponent(input)
  } catch {
    return '[编码错误]'
  }
}

export const urlDecode = (input: string): string => {
  try {
    return decodeURIComponent(input)
  } catch {
    return '[解码错误: 无效的 URL 编码]'
  }
}

// HTML 实体编解码
export const htmlEncode = (input: string, options?: Record<string, string>): string => {
  const namedEntities: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }
  const mode = (options?.entityMode || 'Named entities').toLowerCase()
  const convertAllCharacters = (options?.convertAllCharacters || 'False').toLowerCase() === 'true'

  let result = ''

  for (const char of input) {
    const codePoint = char.codePointAt(0)
    if (codePoint === undefined) {
      result += char
      continue
    }

    const isPreservedAscii =
      char === '\n' ||
      char === '\r' ||
      char === '\t' ||
      (codePoint >= 0x20 && codePoint <= 0x7e)

    if (!convertAllCharacters && isPreservedAscii && !namedEntities[char]) {
      result += char
      continue
    }

    if (mode === 'numeric entities') {
      result += `&#${codePoint};`
      continue
    }

    if (mode === 'hex entities') {
      result += `&#x${codePoint.toString(16).toUpperCase()};`
      continue
    }

    if (namedEntities[char]) {
      result += namedEntities[char]
      continue
    }

    result += `&#${codePoint};`
  }

  return result
}

export const htmlDecode = (input: string): string => {
  const namedEntities: Record<string, string> = {
    '&amp;': '&',
    '&lt;': '<',
    '&gt;': '>',
    '&quot;': '"',
    '&apos;': "'",
    '&#39;': "'",
    '&nbsp;': ' ',
  }
  const isValidCodePoint = (codePoint: number) =>
    Number.isInteger(codePoint) && codePoint >= 0 && codePoint <= 0x10ffff

  return input.replace(/&(?:amp|lt|gt|quot|apos|#39|nbsp|#\d+|#x[0-9a-fA-F]+);/g, (entity) => {
    if (namedEntities[entity]) {
      return namedEntities[entity]
    }

    if (entity.startsWith('&#x') || entity.startsWith('&#X')) {
      const codePoint = Number.parseInt(entity.slice(3, -1), 16)
      return isValidCodePoint(codePoint) ? String.fromCodePoint(codePoint) : entity
    }

    if (entity.startsWith('&#')) {
      const codePoint = Number.parseInt(entity.slice(2, -1), 10)
      return isValidCodePoint(codePoint) ? String.fromCodePoint(codePoint) : entity
    }

    return entity
  })
}

// Hex 编解码
export const hexEncode = (input: string): string => {
  return Array.from(input)
    .map((c) => c.charCodeAt(0).toString(16).padStart(2, '0'))
    .join('')
}

export const hexDecode = (input: string): string => {
  try {
    const hex = input.replace(/\s/g, '')
    if (hex.length % 2 !== 0) return '[解码错误: 无效的 Hex 长度]'
    let result = ''
    for (let i = 0; i < hex.length; i += 2) {
      result += String.fromCharCode(parseInt(hex.substr(i, 2), 16))
    }
    return result
  } catch {
    return '[解码错误: 无效的 Hex]'
  }
}

// Unicode 编解码
const escapeRegExp = (value: string): string => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

export const unicodeEncode = (input: string, options?: Record<string, string>): string => {
  const prefix = options?.prefix || '\\u'
  const padding = Math.max(1, Number.parseInt(options?.padding || '4', 10) || 4)
  const encodeAllCharacters = (options?.encodeAllCharacters || 'False').toLowerCase() === 'true'
  const uppercaseHex = (options?.uppercaseHex || 'True').toLowerCase() === 'true'

  let result = ''

  for (const char of input) {
    const codePoint = char.codePointAt(0)
    if (codePoint === undefined) {
      result += char
      continue
    }

    const isPreservedAscii =
      char === '\n' ||
      char === '\r' ||
      char === '\t' ||
      (codePoint >= 0x20 && codePoint <= 0x7e)

    if (!encodeAllCharacters && isPreservedAscii) {
      result += char
      continue
    }

    const hex = codePoint.toString(16).padStart(padding, '0')
    result += `${prefix}${uppercaseHex ? hex.toUpperCase() : hex.toLowerCase()}`
  }

  return result
}

export const unicodeDecode = (input: string, options?: Record<string, string>): string => {
  try {
    const prefix = options?.prefix || '\\u'
    const escapedPrefix = escapeRegExp(prefix)

    return input.replace(new RegExp(`${escapedPrefix}([0-9a-fA-F]{4,6})`, 'g'), (_, code) =>
      String.fromCodePoint(parseInt(code, 16))
    )
  } catch {
    return '[解码错误: 无效的 Unicode]'
  }
}

// JWT 解码
export const jwtDecode = (input: string): string => {
  try {
    const parts = input.trim().split('.')
    if (parts.length !== 3) return '[错误: 无效的 JWT 格式]'
    const header = JSON.parse(atob(parts[0]))
    const payload = JSON.parse(atob(parts[1]))
    return JSON.stringify({ header, payload }, null, 2)
  } catch {
    return '[解码错误: 无效的 JWT]'
  }
}
