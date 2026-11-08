// Run with: node --test tests/desktop-api-isolation.test.mjs
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import vm from 'node:vm'
import { fileURLToPath } from 'node:url'
import { test } from 'node:test'
import ts from 'typescript'

const frontendRoot = fileURLToPath(new URL('..', import.meta.url))

function createRuntime({ desktop = false, ssr = false, mode = 'production', status = 200 } = {}) {
  const calls = []
  const effects = []
  const storage = new Map()
  const sessionStorage = new Map()
  const makeStorage = (values) => ({
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
  })
  const context = vm.createContext({
    process: { env: { NODE_ENV: mode } },
    URL, URLSearchParams, Headers, AbortController, setTimeout, clearTimeout,
    fetch: async (url, config = {}) => {
      calls.push({ url, config })
      return new Response(JSON.stringify({
        success: true,
        data: { token: 'restored-token', isDesktop: true, items: {}, jdks: [] },
      }), { status, headers: { 'Content-Type': 'application/json' } })
    },
    ...(!ssr && {
      localStorage: makeStorage(storage),
      document: { cookie: '', documentElement: { classList: { remove() {}, add() {} } } },
      window: {
        location: {
          search: desktop ? '?desktop_api_token=desktop-secret' : '',
          protocol: 'http:', hostname: 'localhost', host: 'localhost:3000', pathname: '/',
        },
        localStorage: makeStorage(storage), sessionStorage: makeStorage(sessionStorage),
        screen: { availWidth: 1920, availHeight: 1080 },
        addEventListener() {}, removeEventListener() {}, dispatchEvent() {},
      },
      Event, CustomEvent,
    }),
  })
  const modules = new Map()
  const stubs = {
    react: {
      useEffect: (effect) => effects.push(effect),
      useState: (initial) => [initial, () => {}],
      useRef: (initial) => ({ current: initial }),
      useCallback: (callback) => callback,
    },
    'react/jsx-runtime': {},
    'lucide-react': {},
    'next/navigation': { usePathname: () => '/' },
    '@/lib/i18n': { useI18n: () => ({ t: { launcher: {} } }) },
    '@/lib/appearance': { subscribeContentTabsEnabled: () => () => {} },
    '@/lib/utils': {},
    '@/lib/toast': {},
  }

  function load(relativePath) {
    const filename = path.resolve(frontendRoot, relativePath)
    if (modules.has(filename)) return modules.get(filename).exports
    const moduleRecord = { exports: {} }
    modules.set(filename, moduleRecord)
    const output = ts.transpileModule(fs.readFileSync(filename, 'utf8'), {
      fileName: filename,
      compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
    }).outputText
    const requireModule = (specifier) => {
      if (Object.hasOwn(stubs, specifier)) return stubs[specifier]
      if (specifier.startsWith('@/components/')) return {}
      const target = specifier.startsWith('@/')
        ? path.resolve(frontendRoot, specifier.slice(2))
        : path.resolve(path.dirname(filename), specifier)
      return load(`${target}.ts`)
    }
    vm.runInContext(`(function(require, module, exports) {\n${output}\n})`, context, { filename })(
      requireModule, moduleRecord, moduleRecord.exports,
    )
    return moduleRecord.exports
  }

  async function mount(file, component) {
    const start = effects.length
    load(`components/${file}.tsx`)[component]()
    for (const effect of effects.slice(start)) effect()
    // Drain async bootstrap work without waiting on timers or a browser.
    await new Promise(setImmediate)
  }

  return { calls, context, storage, load, mount }
}

for (const mode of ['production', 'development']) {
  test(`Web ${mode}: login, restore, config save, logout and 401 never call desktop APIs`, async () => {
    const runtime = createRuntime({ mode, status: 401 })
    const api = runtime.load('lib/api.ts')
    api.setToken('web-token')
    assert.equal(runtime.storage.get('auth_token'), 'web-token')
    assert.match(runtime.context.document.cookie, /auth_token=web-token/)
    assert.equal(await api.restoreDesktopToken(), null)
    await api.persistDesktopConfigItems({ theme: 'dark' })
    await api.removeDesktopConfigItems(['theme'])
    api.clearToken()
    assert.equal(runtime.storage.has('auth_token'), false)
    api.setToken('expired-token')
    await api.fetchWithAuth('/api/v1/auth/user')
    await api.request('/api/v1/auth/user')
    assert.equal(runtime.storage.has('auth_token'), false)
    assert.equal(runtime.calls.length, 2)
    assert.ok(runtime.calls.every(({ url }) => !url.includes('/desktop/')))
  })
}

test('Web: shared API clients block desktop-only calls before fetch', async () => {
  const runtime = createRuntime()
  const api = runtime.load('lib/api.ts')
  const result = await api.request('/api/v1/desktop/launcher/config')
  assert.equal(result.success, false)
  await assert.rejects(api.fetchWithAuth('/api/v1/desktop/update/install-latest'), /web mode/)
  await assert.rejects(api.fetchWithAuth('http://localhost:3000/api/v1/desktop/storage'), /web mode/)
  assert.equal(runtime.calls.length, 0)
})

test('Web: main/settings/launcher titlebars and desktop bridges bootstrap without desktop requests', async () => {
  const runtime = createRuntime()
  for (const name of ['DesktopTitlebar', 'SettingsWindowTitlebar', 'LauncherWindowTitlebar',
    'DesktopStorageBridge', 'DesktopWindowBootstrap', 'DesktopExternalLinkBridge']) {
    await runtime.mount(name, name)
  }
  assert.equal(await runtime.load('lib/desktop.ts').getDesktopRuntimeMeta(), null)
  assert.equal(runtime.calls.length, 0)
})

test('Desktop: auth and config persistence and shared API clients keep the desktop token', async () => {
  const runtime = createRuntime({ desktop: true })
  const api = runtime.load('lib/api.ts')
  api.setToken('desktop-auth')
  assert.equal(await api.restoreDesktopToken(), 'restored-token')
  await api.persistDesktopConfigItems({ theme: 'dark' })
  await api.removeDesktopConfigItems(['theme'])
  api.clearToken()
  await api.request('/api/v1/desktop/launcher/config')
  await api.fetchWithAuth('/api/v1/desktop/launcher/config')
  assert.deepEqual(runtime.calls.map(({ url, config }) => [new URL(url).pathname, config.method || 'GET']), [
    ['/api/v1/desktop/auth/session', 'PUT'],
    ['/api/v1/desktop/storage', 'PUT'],
    ['/api/v1/desktop/auth/session', 'GET'],
    ['/api/v1/desktop/storage', 'PUT'],
    ['/api/v1/desktop/storage', 'DELETE'],
    ['/api/v1/desktop/auth/session', 'DELETE'],
    ['/api/v1/desktop/storage', 'DELETE'],
    ['/api/v1/desktop/launcher/config', 'GET'],
    ['/api/v1/desktop/launcher/config', 'GET'],
  ])
  for (const { config } of runtime.calls) {
    assert.equal(new Headers(config.headers).get('X-Hephaestus-Desktop-Token'), 'desktop-secret')
  }
})

test('Desktop: all three titlebars still fetch metadata', async () => {
  const runtime = createRuntime({ desktop: true })
  for (const name of ['DesktopTitlebar', 'SettingsWindowTitlebar', 'LauncherWindowTitlebar']) {
    await runtime.mount(name, name)
  }
  const metadataCalls = runtime.calls.filter(({ url }) => url.endsWith('/desktop/meta'))
  assert.equal(metadataCalls.length, 3)
  assert.ok(metadataCalls.every(({ config }) => config.headers['X-Hephaestus-Desktop-Token'] === 'desktop-secret'))
})

test('SSR: desktop persistence skips all requests', async () => {
  const runtime = createRuntime({ ssr: true })
  const api = runtime.load('lib/api.ts')
  assert.equal(await api.restoreDesktopToken(), null)
  await api.persistDesktopConfigItems({ theme: 'dark' })
  await api.removeDesktopConfigItems(['theme'])
  assert.equal(runtime.calls.length, 0)
})
