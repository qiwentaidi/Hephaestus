import CryptoJS from 'crypto-js'

export type ShaAlgorithm = 'SHA-1' | 'SHA-256' | 'SHA-384' | 'SHA-512'

const shaAlgorithms = {
  'SHA-1': CryptoJS.algo.SHA1,
  'SHA-256': CryptoJS.algo.SHA256,
  'SHA-384': CryptoJS.algo.SHA384,
  'SHA-512': CryptoJS.algo.SHA512,
}

// Preserve raw bytes (including sliced buffers and non-UTF-8 secrets).
const toWordArray = (bytes: Uint8Array) => {
  const words: number[] = []
  for (let i = 0; i < bytes.length; i++) {
    words[i >>> 2] = (words[i >>> 2] || 0) | (bytes[i] << (24 - (i % 4) * 8))
  }
  return CryptoJS.lib.WordArray.create(words, bytes.length)
}

const toBytes = (value: CryptoJS.lib.WordArray) => {
  const bytes = new Uint8Array(value.sigBytes)
  for (let i = 0; i < bytes.length; i++) {
    bytes[i] = (value.words[i >>> 2] >>> (24 - (i % 4) * 8)) & 0xff
  }
  return bytes
}

export async function digestSha(algorithm: ShaAlgorithm, data: Uint8Array): Promise<Uint8Array> {
  const subtle = globalThis.crypto?.subtle
  if (subtle) {
    return new Uint8Array(await subtle.digest(algorithm, new Uint8Array(data)))
  }
  // Web Crypto is unavailable on LAN HTTP origins; keep computation local.
  return toBytes(shaAlgorithms[algorithm].create().finalize(toWordArray(data)))
}

export async function signHmac(algorithm: ShaAlgorithm, data: Uint8Array, secret: Uint8Array): Promise<Uint8Array> {
  if (secret.byteLength === 0) throw new Error('HMAC 密钥不能为空')
  const subtle = globalThis.crypto?.subtle
  if (subtle) {
    const params = { name: 'HMAC', hash: algorithm }
    const key = await subtle.importKey('raw', new Uint8Array(secret), params, false, ['sign'])
    return new Uint8Array(await subtle.sign(params, key, new Uint8Array(data)))
  }
  return toBytes(CryptoJS.algo.HMAC.create(shaAlgorithms[algorithm], toWordArray(secret)).finalize(toWordArray(data)))
}
