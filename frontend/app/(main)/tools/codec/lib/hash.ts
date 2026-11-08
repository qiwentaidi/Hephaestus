import CryptoJS from 'crypto-js'
import sm from 'sm-crypto'
import { digestSha } from '@/lib/browser-crypto'

// SHA-256 哈希 (异步)
export const sha256 = async (input: string): Promise<string> => {
  const encoder = new TextEncoder()
  const data = encoder.encode(input)
  const hashBuffer = await digestSha('SHA-256', data)
  return Array.from(hashBuffer)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

// SHA-1 哈希 (异步)
export const sha1 = async (input: string): Promise<string> => {
  const encoder = new TextEncoder()
  const data = encoder.encode(input)
  const hashBuffer = await digestSha('SHA-1', data)
  return Array.from(hashBuffer)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

// SHA-512 哈希 (异步)
export const sha512 = async (input: string): Promise<string> => {
  const encoder = new TextEncoder()
  const data = encoder.encode(input)
  const hashBuffer = await digestSha('SHA-512', data)
  return Array.from(hashBuffer)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

// MD5 哈希
export const md5Hash = (input: string): string => {
  try {
    return CryptoJS.MD5(input).toString()
  } catch (e) {
    return `[MD5哈希错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

// SM3 国密哈希
export const sm3Hash = (input: string): string => {
  try {
    return sm.sm3(input)
  } catch (e) {
    return `[SM3哈希错误: ${e instanceof Error ? e.message : '未知错误'}]`
  }
}

