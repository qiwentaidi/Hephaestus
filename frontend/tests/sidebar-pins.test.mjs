import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const source = readFileSync(new URL('../lib/sidebar-pins.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2020 } }).outputText
const { parseSidebarPins, readSidebarPinsSnapshot, toggleSidebarPin, subscribeSidebarPins } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)

test('invalid storage does not break navigation and duplicates preserve the first position', () => {
  for (const value of ['invalid', 'null', '{}', 'true']) assert.deepEqual(parseSidebarPins(value), [])
  assert.deepEqual(parseSidebarPins('["/tools/jwt",null,3,"https://example.com","/tools/codec","/tools/jwt"]'), ['/tools/jwt', '/tools/codec'])
})

test('pins survive rereads, remain account-specific, and unpinning preserves other entries', (t) => {
  const data = new Map()
  const browser = new EventTarget()
  browser.localStorage = { getItem: (key) => data.get(key) ?? null, setItem: (key, value) => data.set(key, value) }
  const originalWindow = globalThis.window
  globalThis.window = browser
  t.after(() => { if (originalWindow === undefined) delete globalThis.window; else globalThis.window = originalWindow })
  assert.equal(toggleSidebarPin('alice', '/tools/jwt'), true)
  assert.equal(toggleSidebarPin('alice', '/tools/codec'), true)
  assert.equal(toggleSidebarPin('bob', '/tools/database'), true)
  assert.deepEqual(parseSidebarPins(readSidebarPinsSnapshot('alice')), ['/tools/jwt', '/tools/codec'])
  assert.deepEqual(parseSidebarPins(readSidebarPinsSnapshot('bob')), ['/tools/database'])
  assert.equal(toggleSidebarPin('alice', '/tools/jwt'), true)
  assert.deepEqual(parseSidebarPins(readSidebarPinsSnapshot('alice')), ['/tools/codec'])
  assert.equal(readSidebarPinsSnapshot(''), '[]')
  assert.equal(toggleSidebarPin('', '/tools/jwt'), false)
})

test('subscribers update after writes and external storage changes, and detach cleanly', (t) => {
  const browser = new EventTarget()
  browser.localStorage = { getItem: () => null, setItem: () => {} }
  const originalWindow = globalThis.window
  globalThis.window = browser
  t.after(() => { if (originalWindow === undefined) delete globalThis.window; else globalThis.window = originalWindow })
  let updates = 0
  const unsubscribe = subscribeSidebarPins(() => updates++)
  toggleSidebarPin('alice', '/tools/jwt')
  assert.equal(updates, 1)
  const storageEvent = new Event('storage')
  Object.defineProperty(storageEvent, 'key', { value: 'hephaestus:sidebar-pins:alice' })
  browser.dispatchEvent(storageEvent)
  assert.equal(updates, 2)
  unsubscribe()
  browser.dispatchEvent(storageEvent)
  assert.equal(updates, 2)
})

test('storage failures report failure without claiming a pin was saved', (t) => {
  const browser = new EventTarget()
  browser.localStorage = { getItem: () => { throw new Error('denied') }, setItem: () => { throw new Error('denied') } }
  const originalWindow = globalThis.window
  globalThis.window = browser
  t.after(() => { if (originalWindow === undefined) delete globalThis.window; else globalThis.window = originalWindow })
  assert.equal(readSidebarPinsSnapshot('alice'), '[]')
  assert.equal(toggleSidebarPin('alice', '/tools/jwt'), false)
})
