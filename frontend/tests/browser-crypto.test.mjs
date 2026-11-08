import assert from 'node:assert/strict'
import { createHash, createHmac, webcrypto } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const source = readFileSync(new URL('../lib/browser-crypto.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
}).outputText
const load = (environment) => {
  const exports = {}
  new Function('require', 'exports', 'globalThis', compiled)(require, exports, environment)
  return exports
}
const hex = (bytes) => Buffer.from(bytes).toString('hex')
const messages = [
  new Uint8Array(),
  new TextEncoder().encode('中文 🔐 / JWT Header.Payload'),
  new TextEncoder().encode('long message '.repeat(1000)),
  new Uint8Array([99, 0, 255, 128, 1, 88]).subarray(1, 5),
]

for (const [name, environment] of [
  ['native Web Crypto', { crypto: webcrypto }],
  ['LAN HTTP without subtle', { crypto: {} }],
  ['without crypto', {}],
]) {
  test(`${name}: SHA digests match Node crypto for empty, Unicode and binary inputs`, async () => {
    const { digestSha } = load(environment)
    for (const algorithm of ['SHA-1', 'SHA-256', 'SHA-384', 'SHA-512']) {
      for (const message of messages) {
        const expected = createHash(algorithm.replace('-', '').toLowerCase()).update(message).digest('hex')
        assert.equal(hex(await digestSha(algorithm, message)), expected)
      }
    }
  })

  test(`${name}: JWT HMAC hashes preserve binary keys and buffer offsets`, async () => {
    const { signHmac } = load(environment)
    await assert.rejects(signHmac('SHA-256', messages[0], new Uint8Array()), /HMAC 密钥不能为空/)
    const secrets = [
      new TextEncoder().encode('a-string-secret-at-least-256-bits-long'),
      new Uint8Array([99, 0, 255, 128, 1, 88]).subarray(1, 5),
      new Uint8Array(257).fill(0xab),
    ]
    for (const algorithm of ['SHA-256', 'SHA-384', 'SHA-512']) {
      for (const secret of secrets) {
        for (const message of messages) {
          const expected = createHmac(algorithm.replace('-', '').toLowerCase(), secret).update(message).digest('hex')
          assert.equal(hex(await signHmac(algorithm, message, secret)), expected)
        }
      }
    }
  })
}

test('native crypto failures are propagated, not silently retried with a fallback', async () => {
  const error = new Error('native operation failed')
  const { digestSha, signHmac } = load({ crypto: { subtle: {
    digest: async () => { throw error },
    importKey: async () => { throw error },
  } } })
  await assert.rejects(digestSha('SHA-256', messages[0]), (caught) => caught === error)
  await assert.rejects(signHmac('SHA-256', messages[0], messages[1]), (caught) => caught === error)
})
