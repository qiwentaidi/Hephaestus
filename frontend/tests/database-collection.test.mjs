import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const root = new URL('../app/(main)/tools/database/', import.meta.url)
function load(relative, dependencies = {}) {
  const source = readFileSync(new URL(relative, root), 'utf8')
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX },
  }).outputText
  const exports = {}
  new Function('require', 'exports', compiled)((name) => dependencies[name] || require(name), exports)
  return exports
}

const data = load('utils/data.ts')
const utils = load('../../../../lib/utils.ts')
const table = load('../../../../components/ui/table.tsx', { '@/lib/utils': utils })
const button = load('../../../../components/ui/button.tsx', { '@/lib/utils': utils })
const badge = load('../../../../components/ui/badge.tsx', { '@/lib/utils': utils })
const numberInput = load('../../../../components/ui/number-input.tsx', { '@/lib/utils': utils })
const input = load('../../../../components/ui/input.tsx', { '@/lib/utils': utils, '@/components/ui/number-input': numberInput })
const dropdown = load('../../../../components/ui/dropdown-menu.tsx', { '@/lib/utils': utils })
const { zh } = load('../../../../lib/i18n/locales/zh.ts')
const { CollectedDataTable } = load('components/CollectedDataTable.tsx', {
  '../utils/data': data,
  '@/components/ui/table': table,
  '@/lib/utils': utils,
  '@/components/ui/button': button,
  '@/components/ui/input': input,
  '@/components/ui/dropdown-menu': dropdown,
  '@/lib/i18n': { useI18n: () => ({ t: zh }) },
})
const encode = value => btoa(encodeURIComponent(JSON.stringify(value)))
const render = value => renderToStaticMarkup(React.createElement(CollectedDataTable, { data: data.parseRowData(value) }))
const svgCount = markup => (markup.match(/<svg\b/g) || []).length
const toolbarSVGCount = svgCount(render({ columns: [], rows: [], rowsCount: 0 }))

test('unsaved structured previews round-trip Redis values without Base64 guessing', () => {
  for (const kind of ['string', 'hash', 'list', 'set', 'zset', 'stream']) {
    const value = kind === 'zset' ? 1.25 : kind === 'stream' ? '{"消息":"中文"}' : 'dGVzdA=='
    const pane = { name: 'db0.key', columns: ['key', 'type', 'field', 'value'], rows: [['key', kind, '<field>', value]], rowsCount: 42, matchedType: 'phone & 自定义规则' }
    const url = new URL(data.collectResultDetailURL(pane, 'conn&test'), 'https://example.invalid')
    assert.equal(url.searchParams.get('connectionNanoid'), 'conn&test')
    assert.equal(url.searchParams.has('content'), false)
    assert.equal(url.searchParams.get('matchedType'), pane.matchedType)
    assert.deepEqual(data.decodeTablePreview(url.searchParams.get('data')), data.parseRowData(pane))
    assert.ok(render(pane).includes(String(value).replaceAll('"', '&quot;')))
  }
  const saved = new URL(data.collectResultDetailURL({ id: 'saved', name: 'key' }, 'conn'), 'https://example.invalid')
  assert.equal(saved.searchParams.get('id'), 'saved')
  assert.equal(saved.searchParams.has('data'), false)
})

test('original HTML URL payload and malformed JSON do not become table data', () => {
  for (const value of ['<img src=x onerror="window.pwned=true">', '<svg onload="window.pwned=true">', { content: '<script>bad()</script>' }, { columns: ['x'], rows: [[]], rowsCount: 1 }, { columns: [1], rows: [], rowsCount: 0 }]) {
    assert.throws(() => data.decodeTablePreview(encode(value)))
  }
  assert.throws(() => data.decodeTablePreview('%%%'))
  assert.throws(() => data.decodeTablePreview(btoa('{')))
  assert.throws(() => data.parseRowData({ columns: ['x'], rows: [[Infinity]], rowsCount: 1 }))
  assert.throws(() => data.parseRowData({ columns: [], rows: [], rowsCount: 0, dataError: 'Historical record cannot be converted' }))
})

test('saved and preview HTML-shaped values are escaped by the real React table', () => {
  const payload = '<img src=x onerror="window.pwned=true"><svg onload="window.pwned=true">'
  const value = { columns: [payload, 'quote'], rows: [[payload, '" onmouseover="window.pwned=true']], rowsCount: 1 }
  for (const input of [value, data.decodeTablePreview(encode(value))]) {
    const markup = render(input)
    assert.ok(markup.includes('&lt;img'))
    assert.ok(!markup.includes('<img'))
    assert.equal(svgCount(markup), toolbarSVGCount, 'data must not create SVG elements beyond toolbar icons')
    assert.ok(!markup.includes('<script'))
    assert.ok(!markup.includes(' onmouseover="'))
  }
})

test('Mongo JSON, booleans, SQL binary metadata, NULL and literal NULL stay readable', () => {
  const values = [null, 'NULL', false, 0, { nested: ['中文', '<script>'] }, { encoding: 'base64', data: '/wA=' }]
  const markup = render({ columns: values.map((_, i) => `c${i}`), rows: [values], rowsCount: 1 })
  assert.ok(markup.includes('italic">NULL</span>'))
  assert.ok(markup.includes('title="NULL">NULL</div>'))
  assert.ok(markup.includes('>false</div>'))
  assert.ok(markup.includes('>0</div>'))
  assert.ok(markup.includes('中文'))
  assert.ok(markup.includes('&lt;script&gt;'))
  assert.ok(markup.includes('/wA='))
  assert.deepEqual(data.parseRowData({ columns: [], rows: [], rowsCount: 0 }), { columns: [], rows: [], rowsCount: 0 })
})

// Execute the actual detail route's loading effect with its I/O mocked. This
// checks the old malicious link entry point, in addition to the table sink.
async function loadDetailRoute(params, response) {
  const state = []
  let stateCursor = 0
  const effects = []
  const redirects = []
  const errors = []
  const requests = []
  const translations = zh
  const { default: Detail } = load('detail/TableDetailClient.tsx', {
    react: {
      useState(initial) {
        const index = stateCursor++
        if (!(index in state)) state[index] = initial
        return [state[index], value => { state[index] = value }]
      },
      useEffect(effect) { effects.push(effect) },
    },
    'next/navigation': { useSearchParams: () => params, useRouter: () => ({ push: path => redirects.push(path) }) },
    'lucide-react': { ArrowLeft: () => null, Loader2: () => null },
    '@/components/ui/button': { Button: () => null },
    '@/components/ui/badge': badge,
    'react-hot-toast': { default: { error: message => errors.push(message) } },
    '@/lib/api': { fetchWithAuth: async url => { requests.push(url); return { json: async () => response } } },
    '@/lib/i18n': { useI18n: () => ({ t: translations }) },
    '../utils/data': data,
    '../components/CollectedDataTable': { CollectedDataTable },
  })
  Detail()
  effects.forEach(effect => effect())
  await new Promise(resolve => setImmediate(resolve))
  stateCursor = 0
  const markup = renderToStaticMarkup(Detail())
  return { state, redirects, errors, requests, markup }
}

test('detail route rejects original malicious content links and malformed structured previews', async () => {
  const payload = '<img src=x onerror="window.pwned=true">'
  for (const query of [{ content: btoa(encodeURIComponent(payload)) }, { data: encode(payload) }]) {
    const result = await loadDetailRoute(new URLSearchParams(query))
    assert.equal(result.state[0], null)
    assert.deepEqual(result.redirects, ['/tools/database'])
    assert.equal(result.errors.length, 1)
    assert.equal(result.requests.length, 0)
  }
})

test('detail route loads saved and unsaved structured tables and their match type intact', async () => {
  const value = { columns: ['value'], rows: [['<svg onload="window.pwned=true"> 中文']], rowsCount: 42 }
  const matchedType = 'phone & 自定义规则 <img src=x onerror=bad()>'
  const preview = await loadDetailRoute(new URLSearchParams({ data: encode(value), tableName: 'db.table', matchedType }))
  const saved = await loadDetailRoute(new URLSearchParams({ id: 'saved', connectionNanoid: 'conn&other=bad', matchedType: 'URL must not override saved metadata' }), {
    success: true, data: [{ ...value, id: 'saved', tableName: 'db.table', matchedType }],
  })
  for (const result of [preview, saved]) {
    assert.deepEqual(result.state[0], value)
    assert.equal(result.state[1], 'db.table')
    assert.equal(result.state[2], matchedType)
    assert.ok(result.markup.includes('匹配类型'))
    assert.ok(result.markup.includes('&lt;img'))
    assert.ok(!result.markup.includes('<img'), 'match type must remain text in the header badge')
    assert.deepEqual(result.redirects, [])
    assert.deepEqual(result.errors, [])
    assert.equal(svgCount(render(result.state[0])), toolbarSVGCount)
  }
  assert.deepEqual(saved.requests, ['/api/v1/database/collect/results?connectionNanoid=conn%26other%3Dbad'])
})

test('older detail links and records without match metadata stay readable', async () => {
  const value = { columns: ['value'], rows: [['sample']], rowsCount: 1 }
  const preview = await loadDetailRoute(new URLSearchParams({ data: encode(value), tableName: 'db.table' }))
  const saved = await loadDetailRoute(new URLSearchParams({ id: 'saved', connectionNanoid: 'conn' }), {
    success: true, data: [{ ...value, id: 'saved', tableName: 'db.table' }],
  })
  for (const result of [preview, saved]) {
    assert.equal(result.state[2], '')
    assert.ok(result.markup.includes('未记录'))
    assert.deepEqual(result.state[0], value)
    assert.deepEqual(result.redirects, [])
  }
})
