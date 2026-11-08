import CryptoJS from 'crypto-js'
import sm from 'sm-crypto'
import JSEncrypt from 'jsencrypt'

// 获取加密模式
const getCryptoMode = (mode: string) => {
  const modes: Record<string, typeof CryptoJS.mode.CBC> = {
    'CBC': CryptoJS.mode.CBC,
    'ECB': CryptoJS.mode.ECB,
    'CFB': CryptoJS.mode.CFB,
    'OFB': CryptoJS.mode.OFB,
    'CTR': CryptoJS.mode.CTR,
  }
  return modes[mode.toUpperCase()] || CryptoJS.mode.CBC
}

// 获取填充方式
const getCryptoPadding = (padding: string) => {
  const paddings: Record<string, typeof CryptoJS.pad.Pkcs7> = {
    'Pkcs7': CryptoJS.pad.Pkcs7,
    'Pkcs5': CryptoJS.pad.Pkcs7,
    'ZeroPadding': CryptoJS.pad.ZeroPadding,
    'NoPadding': CryptoJS.pad.NoPadding,
    'Iso10126': CryptoJS.pad.Iso10126,
    'Iso97971': CryptoJS.pad.Iso97971,
    'AnsiX923': CryptoJS.pad.AnsiX923,
  }
  return paddings[padding] || CryptoJS.pad.Pkcs7
}

const modeNeedsIv = (mode: string) => mode.toUpperCase() !== 'ECB'
const modeUsesPadding = (mode: string) => {
  const normalized = mode.toUpperCase()
  return normalized === 'ECB' || normalized === 'CBC'
}

const getBlockSizeByAlgorithm = (algorithm: 'AES' | 'DES' | '3DES' | 'SM4') => {
  switch (algorithm) {
    case 'AES':
    case 'SM4':
      return 16
    case 'DES':
    case '3DES':
      return 8
    default:
      return 16
  }
}

const getInputByteLength = (value: string) => CryptoJS.enc.Utf8.parse(value).sigBytes
const wordArrayToRaw = (value: CryptoJS.lib.WordArray) => CryptoJS.enc.Latin1.stringify(value)
const rawToWordArray = (value: string) => CryptoJS.enc.Latin1.parse(value)

const validateNoPaddingInput = (
  input: string,
  options: Record<string, string> | undefined,
  algorithm: 'AES' | 'DES' | '3DES' | 'SM4'
) => {
  const mode = (options?.mode || 'CBC').toUpperCase()
  const padding = options?.padding || 'Pkcs7'

  if (padding !== 'NoPadding' || !modeUsesPadding(mode)) {
    return null
  }

  const blockSize = getBlockSizeByAlgorithm(algorithm)
  const inputByteLength = getInputByteLength(input)
  if (inputByteLength % blockSize !== 0) {
    return `[${algorithm}错误: NoPadding 模式下输入长度必须是 ${blockSize} 字节的整数倍，当前为 ${inputByteLength} 字节]`
  }

  return null
}

// 解析密钥/IV
const parseKeyOrIv = (value: string, format: string, targetLength: number) => {
  let parsed
  switch (format.toLowerCase()) {
    case 'hex':
      parsed = CryptoJS.enc.Hex.parse(value)
      break
    case 'base64':
      parsed = CryptoJS.enc.Base64.parse(value)
      break
    case 'utf8':
    default:
      const padded = value.padEnd(targetLength, '0').slice(0, targetLength)
      parsed = CryptoJS.enc.Utf8.parse(padded)
      break
  }
  return parsed
}

// AES 加密
export const aesEncrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const noPaddingError = validateNoPaddingInput(input, options, 'AES')
    if (noPaddingError) {
      return noPaddingError
    }

    const key = options?.key || '0123456789abcdef'
    const mode = options?.mode || 'CBC'
    const padding = options?.padding || 'Pkcs7'
    const keyFormat = options?.keyFormat || 'UTF8'
    const ivFormat = options?.ivFormat || 'UTF8'
    const outputFormat = (options?.outputFormat || 'Base64').toLowerCase()
    const keySize = parseInt(options?.keySize || '128')

    const keyLength = keySize / 8
    const keyBytes = parseKeyOrIv(key, keyFormat, keyLength)
    const config: { iv?: CryptoJS.lib.WordArray; mode: typeof CryptoJS.mode.CBC; padding: typeof CryptoJS.pad.Pkcs7 } = {
      mode: getCryptoMode(mode),
      padding: modeUsesPadding(mode) ? getCryptoPadding(padding) : CryptoJS.pad.NoPadding,
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '0123456789abcdef'
      config.iv = parseKeyOrIv(iv, ivFormat, 16)
    }

    const encrypted = CryptoJS.AES.encrypt(input, keyBytes, config)

    if (outputFormat === 'hex') {
      return encrypted.ciphertext.toString(CryptoJS.enc.Hex)
    }
    if (outputFormat === 'raw') {
      return wordArrayToRaw(encrypted.ciphertext)
    }
    return encrypted.toString()
  } catch (e) {
    return `[AES加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// AES 解密
export const aesDecrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || '0123456789abcdef'
    const mode = options?.mode || 'CBC'
    const padding = options?.padding || 'Pkcs7'
    const keyFormat = options?.keyFormat || 'UTF8'
    const ivFormat = options?.ivFormat || 'UTF8'
    const inputFormat = (options?.inputFormat || 'Base64').toLowerCase()
    const keySize = parseInt(options?.keySize || '128')

    const keyLength = keySize / 8
    const keyBytes = parseKeyOrIv(key, keyFormat, keyLength)
    const config: { iv?: CryptoJS.lib.WordArray; mode: typeof CryptoJS.mode.CBC; padding: typeof CryptoJS.pad.Pkcs7 } = {
      mode: getCryptoMode(mode),
      padding: modeUsesPadding(mode) ? getCryptoPadding(padding) : CryptoJS.pad.NoPadding,
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '0123456789abcdef'
      config.iv = parseKeyOrIv(iv, ivFormat, 16)
    }

    let cipherParams
    if (inputFormat === 'hex') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: CryptoJS.enc.Hex.parse(input.trim())
      })
    } else if (inputFormat === 'raw') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: rawToWordArray(input)
      })
    } else {
      cipherParams = input.trim()
    }

    const decrypted = CryptoJS.AES.decrypt(cipherParams, keyBytes, config)
    return decrypted.toString(CryptoJS.enc.Utf8) || '[解密结果为空，请检查密钥、模式与填充是否正确]'
  } catch (e) {
    return `[AES解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// DES 加密
export const desEncrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const noPaddingError = validateNoPaddingInput(input, options, 'DES')
    if (noPaddingError) {
      return noPaddingError
    }

    const key = options?.key || '12345678'
    const mode = options?.mode || 'CBC'
    const padding = options?.padding || 'Pkcs7'
    const keyFormat = options?.keyFormat || 'UTF8'
    const ivFormat = options?.ivFormat || 'UTF8'
    const outputFormat = (options?.outputFormat || 'Base64').toLowerCase()

    const keyBytes = parseKeyOrIv(key, keyFormat, 8)
    const config: { iv?: CryptoJS.lib.WordArray; mode: typeof CryptoJS.mode.CBC; padding: typeof CryptoJS.pad.Pkcs7 } = {
      mode: getCryptoMode(mode),
      padding: modeUsesPadding(mode) ? getCryptoPadding(padding) : CryptoJS.pad.NoPadding,
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '12345678'
      config.iv = parseKeyOrIv(iv, ivFormat, 8)
    }

    const encrypted = CryptoJS.DES.encrypt(input, keyBytes, config)

    if (outputFormat === 'hex') {
      return encrypted.ciphertext.toString(CryptoJS.enc.Hex)
    }
    if (outputFormat === 'raw') {
      return wordArrayToRaw(encrypted.ciphertext)
    }
    return encrypted.toString()
  } catch (e) {
    return `[DES加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// DES 解密
export const desDecrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || '12345678'
    const mode = options?.mode || 'CBC'
    const padding = options?.padding || 'Pkcs7'
    const keyFormat = options?.keyFormat || 'UTF8'
    const ivFormat = options?.ivFormat || 'UTF8'
    const inputFormat = (options?.inputFormat || 'Base64').toLowerCase()

    const keyBytes = parseKeyOrIv(key, keyFormat, 8)
    const config: { iv?: CryptoJS.lib.WordArray; mode: typeof CryptoJS.mode.CBC; padding: typeof CryptoJS.pad.Pkcs7 } = {
      mode: getCryptoMode(mode),
      padding: modeUsesPadding(mode) ? getCryptoPadding(padding) : CryptoJS.pad.NoPadding,
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '12345678'
      config.iv = parseKeyOrIv(iv, ivFormat, 8)
    }

    let cipherParams
    if (inputFormat === 'hex') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: CryptoJS.enc.Hex.parse(input.trim())
      })
    } else if (inputFormat === 'raw') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: rawToWordArray(input)
      })
    } else {
      cipherParams = input.trim()
    }

    const decrypted = CryptoJS.DES.decrypt(cipherParams, keyBytes, config)
    return decrypted.toString(CryptoJS.enc.Utf8) || '[解密结果为空]'
  } catch (e) {
    return `[DES解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// 3DES 加密
export const tripleDesEncrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const noPaddingError = validateNoPaddingInput(input, options, '3DES')
    if (noPaddingError) {
      return noPaddingError
    }

    const key = options?.key || '123456789012345678901234'
    const mode = options?.mode || 'CBC'
    const padding = options?.padding || 'Pkcs7'
    const keyFormat = options?.keyFormat || 'UTF8'
    const ivFormat = options?.ivFormat || 'UTF8'
    const outputFormat = (options?.outputFormat || 'Base64').toLowerCase()

    const keyBytes = parseKeyOrIv(key, keyFormat, 24)
    const config: { iv?: CryptoJS.lib.WordArray; mode: typeof CryptoJS.mode.CBC; padding: typeof CryptoJS.pad.Pkcs7 } = {
      mode: getCryptoMode(mode),
      padding: modeUsesPadding(mode) ? getCryptoPadding(padding) : CryptoJS.pad.NoPadding,
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '12345678'
      config.iv = parseKeyOrIv(iv, ivFormat, 8)
    }

    const encrypted = CryptoJS.TripleDES.encrypt(input, keyBytes, config)

    if (outputFormat === 'hex') {
      return encrypted.ciphertext.toString(CryptoJS.enc.Hex)
    }
    if (outputFormat === 'raw') {
      return wordArrayToRaw(encrypted.ciphertext)
    }
    return encrypted.toString()
  } catch (e) {
    return `[3DES加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// 3DES 解密
export const tripleDesDecrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || '123456789012345678901234'
    const mode = options?.mode || 'CBC'
    const padding = options?.padding || 'Pkcs7'
    const keyFormat = options?.keyFormat || 'UTF8'
    const ivFormat = options?.ivFormat || 'UTF8'
    const inputFormat = (options?.inputFormat || 'Base64').toLowerCase()

    const keyBytes = parseKeyOrIv(key, keyFormat, 24)
    const config: { iv?: CryptoJS.lib.WordArray; mode: typeof CryptoJS.mode.CBC; padding: typeof CryptoJS.pad.Pkcs7 } = {
      mode: getCryptoMode(mode),
      padding: modeUsesPadding(mode) ? getCryptoPadding(padding) : CryptoJS.pad.NoPadding,
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '12345678'
      config.iv = parseKeyOrIv(iv, ivFormat, 8)
    }

    let cipherParams
    if (inputFormat === 'hex') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: CryptoJS.enc.Hex.parse(input.trim())
      })
    } else if (inputFormat === 'raw') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: rawToWordArray(input)
      })
    } else {
      cipherParams = input.trim()
    }

    const decrypted = CryptoJS.TripleDES.decrypt(cipherParams, keyBytes, config)
    return decrypted.toString(CryptoJS.enc.Utf8) || '[解密结果为空]'
  } catch (e) {
    return `[3DES解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// RC4 加密
export const rc4Encrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || 'secretkey'
    const keyFormat = (options?.keyFormat || 'UTF8').toLowerCase()
    const outputFormat = (options?.outputFormat || 'Base64').toLowerCase()

    let keyBytes
    if (keyFormat === 'hex') {
      keyBytes = CryptoJS.enc.Hex.parse(key)
    } else {
      keyBytes = CryptoJS.enc.Utf8.parse(key)
    }

    const encrypted = CryptoJS.RC4.encrypt(input, keyBytes)

    if (outputFormat === 'hex') {
      return encrypted.ciphertext.toString(CryptoJS.enc.Hex)
    }
    if (outputFormat === 'raw') {
      return wordArrayToRaw(encrypted.ciphertext)
    }
    return encrypted.toString()
  } catch (e) {
    return `[RC4加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// RC4 解密
export const rc4Decrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || 'secretkey'
    const keyFormat = (options?.keyFormat || 'UTF8').toLowerCase()
    const inputFormat = (options?.inputFormat || 'Base64').toLowerCase()

    let keyBytes
    if (keyFormat === 'hex') {
      keyBytes = CryptoJS.enc.Hex.parse(key)
    } else {
      keyBytes = CryptoJS.enc.Utf8.parse(key)
    }

    let cipherParams
    if (inputFormat === 'hex') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: CryptoJS.enc.Hex.parse(input.trim())
      })
    } else if (inputFormat === 'raw') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: rawToWordArray(input)
      })
    } else {
      cipherParams = input.trim()
    }

    const decrypted = CryptoJS.RC4.decrypt(cipherParams, keyBytes)
    return decrypted.toString(CryptoJS.enc.Utf8) || '[解密结果为空]'
  } catch (e) {
    return `[RC4解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// Rabbit 加密
export const rabbitEncrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || 'secretkey'
    const iv = options?.iv || ''
    const outputFormat = (options?.outputFormat || 'Base64').toLowerCase()

    const keyBytes = CryptoJS.enc.Utf8.parse(key)
    const config: { iv?: CryptoJS.lib.WordArray } = {}
    if (iv) {
      config.iv = CryptoJS.enc.Utf8.parse(iv)
    }

    const encrypted = CryptoJS.Rabbit.encrypt(input, keyBytes, config)

    if (outputFormat === 'hex') {
      return encrypted.ciphertext.toString(CryptoJS.enc.Hex)
    }
    if (outputFormat === 'raw') {
      return wordArrayToRaw(encrypted.ciphertext)
    }
    return encrypted.toString()
  } catch (e) {
    return `[Rabbit加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// Rabbit 解密
export const rabbitDecrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || 'secretkey'
    const iv = options?.iv || ''
    const inputFormat = (options?.inputFormat || 'Base64').toLowerCase()

    const keyBytes = CryptoJS.enc.Utf8.parse(key)
    const config: { iv?: CryptoJS.lib.WordArray } = {}
    if (iv) {
      config.iv = CryptoJS.enc.Utf8.parse(iv)
    }

    let cipherParams
    if (inputFormat === 'hex') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: CryptoJS.enc.Hex.parse(input.trim())
      })
    } else if (inputFormat === 'raw') {
      cipherParams = CryptoJS.lib.CipherParams.create({
        ciphertext: rawToWordArray(input)
      })
    } else {
      cipherParams = input.trim()
    }

    const decrypted = CryptoJS.Rabbit.decrypt(cipherParams, keyBytes, config)
    return decrypted.toString(CryptoJS.enc.Utf8) || '[解密结果为空]'
  } catch (e) {
    return `[Rabbit解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// SM4 国密加密
export const sm4Encrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || '0123456789abcdef'
    const mode = (options?.mode || 'CBC').toUpperCase()
    const keyFormat = (options?.keyFormat || 'UTF8').toLowerCase()
    const ivFormat = (options?.ivFormat || 'UTF8').toLowerCase()
    const outputFormat = (options?.outputFormat || 'Hex').toLowerCase()

    let keyHex: string
    let ivHex: string | undefined

    if (keyFormat === 'hex') {
      keyHex = key.padEnd(32, '0').slice(0, 32)
    } else {
      keyHex = Array.from(key.padEnd(16, '\0').slice(0, 16)).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '0123456789abcdef'
      if (ivFormat === 'hex') {
        ivHex = iv.padEnd(32, '0').slice(0, 32)
      } else {
        ivHex = Array.from(iv.padEnd(16, '\0').slice(0, 16)).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
      }
    }

    const config: { mode: 'ecb' | 'cbc'; iv?: string; output: 'array' } = {
      mode: mode === 'ECB' ? 'ecb' : 'cbc',
      output: 'array'
    }
    if (ivHex) {
      config.iv = ivHex
    }

    const encrypted = sm.sm4.encrypt(input, keyHex, config)

    const hexResult = Array.from(encrypted as number[]).map(b => b.toString(16).padStart(2, '0')).join('')

    const rawResult = String.fromCharCode(...(encrypted as number[]))
    if (outputFormat === 'base64') {
      return btoa(rawResult)
    }
    if (outputFormat === 'raw') {
      return rawResult
    }
    return hexResult
  } catch (e) {
    return `[SM4加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// SM4 国密解密
export const sm4Decrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const key = options?.key || '0123456789abcdef'
    const mode = (options?.mode || 'CBC').toUpperCase()
    const keyFormat = (options?.keyFormat || 'UTF8').toLowerCase()
    const ivFormat = (options?.ivFormat || 'UTF8').toLowerCase()
    const inputFormat = (options?.inputFormat || 'Hex').toLowerCase()

    let keyHex: string
    let ivHex: string | undefined

    if (keyFormat === 'hex') {
      keyHex = key.padEnd(32, '0').slice(0, 32)
    } else {
      keyHex = Array.from(key.padEnd(16, '\0').slice(0, 16)).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
    }

    if (modeNeedsIv(mode)) {
      const iv = options?.iv || '0123456789abcdef'
      if (ivFormat === 'hex') {
        ivHex = iv.padEnd(32, '0').slice(0, 32)
      } else {
        ivHex = Array.from(iv.padEnd(16, '\0').slice(0, 16)).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
      }
    }

    let inputHex: string
    if (inputFormat === 'base64') {
      const bytes = atob(input.trim())
      inputHex = Array.from(bytes).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
    } else if (inputFormat === 'raw') {
      inputHex = Array.from(input).map(c => c.charCodeAt(0).toString(16).padStart(2, '0')).join('')
    } else {
      inputHex = input.trim()
    }

    const config: { mode: 'ecb' | 'cbc'; iv?: string; output: 'string' } = {
      mode: mode === 'ECB' ? 'ecb' : 'cbc',
      output: 'string'
    }
    if (ivHex) {
      config.iv = ivHex
    }

    const decrypted = sm.sm4.decrypt(inputHex, keyHex, config) as string
    return decrypted || '[解密结果为空]'
  } catch (e) {
    return `[SM4解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// SM2 国密加密
export const sm2Encrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const publicKey = options?.publicKey || ''
    if (!publicKey) {
      return '[错误: 请提供SM2公钥]'
    }
    const encrypted = sm.sm2.doEncrypt(input, publicKey, 1)
    return encrypted
  } catch (e) {
    return `[SM2加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// SM2 国密解密
export const sm2Decrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const privateKey = options?.privateKey || ''
    if (!privateKey) {
      return '[错误: 请提供SM2私钥]'
    }
    const decrypted = sm.sm2.doDecrypt(input.trim(), privateKey, 1)
    return decrypted || '[解密结果为空]'
  } catch (e) {
    return `[SM2解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// SM2 生成密钥对
export const sm2GenerateKeyPair = (_input: string, _options?: Record<string, string>): string => {
  try {
    const keypair = sm.sm2.generateKeyPairHex()
    return `=== SM2 密钥对 ===

【公钥 - 用于加密】
${keypair.publicKey}

【私钥 - 用于解密】
${keypair.privateKey}`
  } catch (e) {
    return `[生成密钥对错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// RSA 生成密钥对
export const rsaGenerateKeyPair = (_input: string, options?: Record<string, string>): string => {
  try {
    const keySize = parseInt(options?.keySize || '2048')
    const encrypt = new JSEncrypt({ default_key_size: keySize.toString() })
    encrypt.getKey()

    const publicKey = encrypt.getPublicKey()
    const privateKey = encrypt.getPrivateKey()

    return `=== RSA ${keySize} 密钥对 ===

【公钥 - 用于加密】
${publicKey}

【私钥 - 用于解密】
${privateKey}`
  } catch (e) {
    return `[生成密钥对错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

const normalizeBase64 = (value: string) => {
  const compact = value.replace(/\s+/g, '').replace(/-/g, '+').replace(/_/g, '/')
  return compact + '='.repeat((4 - (compact.length % 4)) % 4)
}

const base64ToBytes = (value: string) => {
  const normalized = normalizeBase64(value)
  const binary = typeof atob === 'function'
    ? atob(normalized)
    : Buffer.from(normalized, 'base64').toString('binary')

  return Uint8Array.from(binary, (char) => char.charCodeAt(0))
}

const bytesToHex = (bytes: Uint8Array) => Array.from(bytes)
  .map((byte) => byte.toString(16).padStart(2, '0'))
  .join('')

const latin1ToBytes = (input: string) => Uint8Array.from(input, (char) => char.charCodeAt(0) & 0xff)

const bytesToLatin1 = (bytes: Uint8Array) => Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')

const decodeUtf8Strict = (bytes: Uint8Array) => new TextDecoder('utf-8', { fatal: true }).decode(bytes)

const formatPemBody = (body: string) => body.match(/.{1,64}/g)?.join('\n') || body

const normalizeRsaKey = (key: string, keyType: 'public' | 'private', keyFormat = 'Auto') => {
  const trimmed = key.trim()
  if (!trimmed) return ''
  if (trimmed.includes('-----BEGIN')) return trimmed
  if (keyFormat === 'PEM') return trimmed

  const body = normalizeBase64(trimmed)
  const label = keyType === 'private' ? 'PRIVATE KEY' : 'PUBLIC KEY'
  return `-----BEGIN ${label}-----\n${formatPemBody(body)}\n-----END ${label}-----`
}

interface Asn1Node {
  tag: number
  start: number
  end: number
  children?: Asn1Node[]
}

interface RsaPrivateKeyParts {
  n: bigint
  d: bigint
}

interface RsaPublicKeyParts {
  n: bigint
  e: bigint
}

type RsaPadding = 'PKCS1_v1_5' | 'OAEP'
type RsaHash = 'SHA-1' | 'SHA-256' | 'SHA-384' | 'SHA-512'

const parseAsn1Node = (bytes: Uint8Array, offset = 0): { node: Asn1Node; nextOffset: number } => {
  const tag = bytes[offset]
  let cursor = offset + 1
  let length = bytes[cursor++]

  if (length & 0x80) {
    const lengthBytes = length & 0x7f
    length = 0
    for (let i = 0; i < lengthBytes; i++) {
      length = (length << 8) | bytes[cursor++]
    }
  }

  const start = cursor
  const end = start + length
  const node: Asn1Node = { tag, start, end }

  if ((tag & 0x20) === 0x20 || tag === 0x30) {
    const children: Asn1Node[] = []
    let childOffset = start
    while (childOffset < end) {
      const parsed = parseAsn1Node(bytes, childOffset)
      children.push(parsed.node)
      childOffset = parsed.nextOffset
    }
    node.children = children
  }

  return { node, nextOffset: end }
}

const asn1Value = (bytes: Uint8Array, node: Asn1Node) => bytes.slice(node.start, node.end)

const bytesToBigInt = (bytes: Uint8Array) => {
  const hex = bytesToHex(bytes).replace(/^00+/, '')
  return hex ? BigInt(`0x${hex}`) : BigInt(0)
}

const extractPemBody = (keyPem: string) => keyPem
  .replace(/-----BEGIN [^-]+-----/g, '')
  .replace(/-----END [^-]+-----/g, '')
  .replace(/\s+/g, '')

const parseRsaPublicKey = (keyPem: string): RsaPublicKeyParts => {
  const keyBytes = base64ToBytes(extractPemBody(keyPem))
  const root = parseAsn1Node(keyBytes).node

  if (!root.children) {
    throw new Error('RSA公钥ASN.1结构无效')
  }

  let rsaRoot = root
  const publicKeyBitString = root.children.find((child) => child.tag === 0x03)
  if (publicKeyBitString) {
    const bitString = asn1Value(keyBytes, publicKeyBitString)
    rsaRoot = parseAsn1Node(bitString.slice(1)).node
  }

  if (!rsaRoot.children || rsaRoot.children.length < 2) {
    throw new Error('RSA公钥参数不完整')
  }

  return {
    n: bytesToBigInt(asn1Value(publicKeyBitString ? asn1Value(keyBytes, publicKeyBitString).slice(1) : keyBytes, rsaRoot.children[0])),
    e: bytesToBigInt(asn1Value(publicKeyBitString ? asn1Value(keyBytes, publicKeyBitString).slice(1) : keyBytes, rsaRoot.children[1])),
  }
}

const parseRsaPrivateKey = (keyPem: string): RsaPrivateKeyParts => {
  const keyBytes = base64ToBytes(extractPemBody(keyPem))
  const root = parseAsn1Node(keyBytes).node

  if (!root.children) {
    throw new Error('RSA私钥ASN.1结构无效')
  }

  let rsaRoot = root
  let rsaBytes = keyBytes
  const privateKeyOctetString = root.children.find((child) => child.tag === 0x04)
  if (privateKeyOctetString) {
    rsaBytes = asn1Value(keyBytes, privateKeyOctetString)
    rsaRoot = parseAsn1Node(rsaBytes).node
  }

  if (!rsaRoot.children || rsaRoot.children.length < 4) {
    throw new Error('RSA私钥参数不完整')
  }

  return {
    n: bytesToBigInt(asn1Value(rsaBytes, rsaRoot.children[1])),
    d: bytesToBigInt(asn1Value(rsaBytes, rsaRoot.children[3])),
  }
}

const modPow = (baseValue: bigint, exponentValue: bigint, modulus: bigint) => {
  const zero = BigInt(0)
  const one = BigInt(1)
  const two = BigInt(2)
  let result = one
  let base = baseValue % modulus
  let exponent = exponentValue

  while (exponent > zero) {
    if (exponent % two === one) {
      result = (result * base) % modulus
    }
    exponent = exponent / two
    base = (base * base) % modulus
  }

  return result
}

const bigIntToFixedBytes = (value: bigint, length: number) => {
  let hex = value.toString(16)
  if (hex.length % 2) hex = `0${hex}`
  const bytes = Uint8Array.from(hex.match(/.{1,2}/g)?.map((part) => parseInt(part, 16)) || [])
  if (bytes.length > length) return bytes.slice(bytes.length - length)

  const padded = new Uint8Array(length)
  padded.set(bytes, length - bytes.length)
  return padded
}

const concatBytes = (chunks: Uint8Array[]) => {
  const totalLength = chunks.reduce((total, chunk) => total + chunk.length, 0)
  const output = new Uint8Array(totalLength)
  let offset = 0

  for (const chunk of chunks) {
    output.set(chunk, offset)
    offset += chunk.length
  }

  return output
}

const getRsaPadding = (padding?: string): RsaPadding => {
  if (padding === 'OAEP') return padding
  return 'PKCS1_v1_5'
}

const getRsaHash = (hash?: string): RsaHash => {
  switch (hash) {
    case 'SHA-256':
    case 'SHA-384':
    case 'SHA-512':
      return hash
    default:
      return 'SHA-1'
  }
}

const bytesToWordArray = (bytes: Uint8Array) => {
  const words: number[] = []
  for (let i = 0; i < bytes.length; i++) {
    words[i >>> 2] = (words[i >>> 2] || 0) | (bytes[i] << (24 - (i % 4) * 8))
  }
  return CryptoJS.lib.WordArray.create(words, bytes.length)
}

const wordArrayToBytes = (wordArray: CryptoJS.lib.WordArray) => {
  const bytes = new Uint8Array(wordArray.sigBytes)
  for (let i = 0; i < wordArray.sigBytes; i++) {
    bytes[i] = (wordArray.words[i >>> 2] >>> (24 - (i % 4) * 8)) & 0xff
  }
  return bytes
}

const hashBytes = (bytes: Uint8Array, hash: RsaHash) => {
  const wordArray = bytesToWordArray(bytes)
  switch (hash) {
    case 'SHA-256':
      return wordArrayToBytes(CryptoJS.SHA256(wordArray))
    case 'SHA-384':
      return wordArrayToBytes(CryptoJS.SHA384(wordArray))
    case 'SHA-512':
      return wordArrayToBytes(CryptoJS.SHA512(wordArray))
    case 'SHA-1':
    default:
      return wordArrayToBytes(CryptoJS.SHA1(wordArray))
  }
}

const mgf1 = (seed: Uint8Array, maskLength: number, hash: RsaHash) => {
  const chunks: Uint8Array[] = []
  for (let counter = 0; concatBytes(chunks).length < maskLength; counter++) {
    const counterBytes = new Uint8Array([
      (counter >>> 24) & 0xff,
      (counter >>> 16) & 0xff,
      (counter >>> 8) & 0xff,
      counter & 0xff,
    ])
    chunks.push(hashBytes(concatBytes([seed, counterBytes]), hash))
  }
  return concatBytes(chunks).slice(0, maskLength)
}

const xorBytes = (left: Uint8Array, right: Uint8Array) => {
  const output = new Uint8Array(left.length)
  for (let i = 0; i < left.length; i++) {
    output[i] = left[i] ^ right[i]
  }
  return output
}

const randomBytes = (length: number) => {
  const output = new Uint8Array(length)
  const cryptoApi = globalThis.crypto
  if (!cryptoApi?.getRandomValues) {
    throw new Error('当前环境不支持安全随机数')
  }
  cryptoApi.getRandomValues(output)
  return output
}

const randomNonZeroBytes = (length: number) => {
  const output = new Uint8Array(length)
  let offset = 0

  while (offset < length) {
    const bytes = randomBytes(length - offset)
    for (const byte of bytes) {
      if (byte !== 0) {
        output[offset++] = byte
        if (offset === length) break
      }
    }
  }

  return output
}

const rsaPadPkcs1V15 = (message: Uint8Array, keyLength: number) => {
  if (message.length > keyLength - 11) {
    throw new Error(`PKCS#1 v1.5单块明文最多${keyLength - 11}字节，当前为${message.length}字节`)
  }

  const paddingLength = keyLength - message.length - 3
  return concatBytes([
    new Uint8Array([0, 2]),
    randomNonZeroBytes(paddingLength),
    new Uint8Array([0]),
    message,
  ])
}

const rsaUnpadPkcs1V15 = (decryptedBlock: Uint8Array) => {
  if (decryptedBlock[0] !== 0 || decryptedBlock[1] !== 2) {
    throw new Error('PKCS#1 v1.5填充无效')
  }

  const separatorIndex = decryptedBlock.indexOf(0, 2)
  if (separatorIndex < 10) {
    throw new Error('PKCS#1 v1.5填充长度无效')
  }

  return decryptedBlock.slice(separatorIndex + 1)
}

const rsaPadOaep = (message: Uint8Array, keyLength: number, hash: RsaHash) => {
  const labelHash = hashBytes(new Uint8Array(), hash)
  const hashLength = labelHash.length

  if (message.length > keyLength - 2 * hashLength - 2) {
    throw new Error(`OAEP(${hash})单块明文最多${keyLength - 2 * hashLength - 2}字节，当前为${message.length}字节`)
  }

  const paddingString = new Uint8Array(keyLength - message.length - 2 * hashLength - 2)
  const dataBlock = concatBytes([labelHash, paddingString, new Uint8Array([1]), message])
  const seed = randomBytes(hashLength)
  const maskedDataBlock = xorBytes(dataBlock, mgf1(seed, keyLength - hashLength - 1, hash))
  const maskedSeed = xorBytes(seed, mgf1(maskedDataBlock, hashLength, hash))

  return concatBytes([new Uint8Array([0]), maskedSeed, maskedDataBlock])
}

const rsaUnpadOaep = (decryptedBlock: Uint8Array, hash: RsaHash) => {
  const labelHash = hashBytes(new Uint8Array(), hash)
  const hashLength = labelHash.length

  if (decryptedBlock.length < 2 * hashLength + 2 || decryptedBlock[0] !== 0) {
    throw new Error(`OAEP(${hash})填充无效`)
  }

  const maskedSeed = decryptedBlock.slice(1, hashLength + 1)
  const maskedDataBlock = decryptedBlock.slice(hashLength + 1)
  const seed = xorBytes(maskedSeed, mgf1(maskedDataBlock, hashLength, hash))
  const dataBlock = xorBytes(maskedDataBlock, mgf1(seed, decryptedBlock.length - hashLength - 1, hash))
  const actualLabelHash = dataBlock.slice(0, hashLength)

  if (bytesToHex(actualLabelHash) !== bytesToHex(labelHash)) {
    throw new Error(`OAEP(${hash})标签哈希不匹配`)
  }

  let separatorIndex = -1
  for (let i = hashLength; i < dataBlock.length; i++) {
    if (dataBlock[i] === 1) {
      separatorIndex = i
      break
    }
    if (dataBlock[i] !== 0) {
      throw new Error(`OAEP(${hash})填充数据无效`)
    }
  }

  if (separatorIndex < 0) {
    throw new Error(`OAEP(${hash})填充分隔符缺失`)
  }

  return dataBlock.slice(separatorIndex + 1)
}

const rsaEncryptBlock = (message: Uint8Array, n: bigint, e: bigint, keyLength: number, padding: RsaPadding, hash: RsaHash) => {
  let encodedMessage: Uint8Array

  if (padding === 'OAEP') {
    encodedMessage = rsaPadOaep(message, keyLength, hash)
  } else {
    encodedMessage = rsaPadPkcs1V15(message, keyLength)
  }

  return bigIntToFixedBytes(modPow(bytesToBigInt(encodedMessage), e, n), keyLength)
}

const rsaDecryptBlock = (ciphertext: Uint8Array, n: bigint, d: bigint, keyLength: number, padding: RsaPadding, hash: RsaHash) => {
  if (ciphertext.length !== keyLength) {
    throw new Error(`RSA密文块长度应为${keyLength}字节，当前为${ciphertext.length}字节`)
  }

  const decryptedBlock = bigIntToFixedBytes(modPow(bytesToBigInt(ciphertext), d, n), keyLength)

  if (padding === 'OAEP') {
    return rsaUnpadOaep(decryptedBlock, hash)
  }
  return rsaUnpadPkcs1V15(decryptedBlock)
}

const rsaEncryptBytes = (message: Uint8Array, publicKeyPem: string, padding: RsaPadding, hash: RsaHash) => {
  const { n, e } = parseRsaPublicKey(publicKeyPem)
  const keyLength = Math.ceil(n.toString(16).length / 2)
  const maxBlockLength = padding === 'OAEP'
    ? keyLength - 2 * hashBytes(new Uint8Array(), hash).length - 2
    : keyLength - 11

  if (maxBlockLength <= 0) {
    throw new Error(`${padding === 'OAEP' ? `OAEP(${hash})` : padding}不适用于当前RSA密钥长度`)
  }

  const chunks: Uint8Array[] = []
  for (let offset = 0; offset < message.length; offset += maxBlockLength) {
    chunks.push(rsaEncryptBlock(message.slice(offset, offset + maxBlockLength), n, e, keyLength, padding, hash))
  }

  return concatBytes(chunks)
}

const rsaDecryptBytes = (ciphertext: Uint8Array, privateKeyPem: string, padding: RsaPadding, hash: RsaHash) => {
  const { n, d } = parseRsaPrivateKey(privateKeyPem)
  const keyLength = Math.ceil(n.toString(16).length / 2)

  if (ciphertext.length % keyLength !== 0) {
    throw new Error(`RSA密文长度必须是${keyLength}字节的整数倍，当前为${ciphertext.length}字节`)
  }

  const chunks: Uint8Array[] = []
  for (let offset = 0; offset < ciphertext.length; offset += keyLength) {
    chunks.push(rsaDecryptBlock(ciphertext.slice(offset, offset + keyLength), n, d, keyLength, padding, hash))
  }

  return concatBytes(chunks)
}

// RSA 加密
export const rsaEncrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const publicKey = normalizeRsaKey(options?.publicKey || '', 'public', options?.keyFormat)
    if (!publicKey) {
      return '[错误: 请提供RSA公钥]'
    }

    const encrypted = rsaEncryptBytes(
      new TextEncoder().encode(input),
      publicKey,
      getRsaPadding(options?.padding),
      getRsaHash(options?.oaepHash),
    )
    return bytesToLatin1(encrypted)
  } catch (e) {
    return `[RSA加密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// RSA 解密
export const rsaDecrypt = (input: string, options?: Record<string, string>): string => {
  try {
    const privateKey = normalizeRsaKey(options?.privateKey || '', 'private', options?.keyFormat)
    if (!privateKey) {
      return '[错误: 请提供RSA私钥]'
    }

    const decrypted = rsaDecryptBytes(
      latin1ToBytes(input),
      privateKey,
      getRsaPadding(options?.padding),
      getRsaHash(options?.oaepHash),
    )
    if (!decrypted.length) {
      return '[解密失败: 请检查私钥格式或密文是否正确]'
    }
    return decodeUtf8Strict(decrypted)
  } catch (e) {
    return `[RSA解密错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}
