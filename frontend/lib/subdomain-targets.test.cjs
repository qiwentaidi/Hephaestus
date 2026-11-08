const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

function loadTS(file, resolve = require) {
  const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText
  const loaded = { exports: {} }
  new Function('exports', 'require', 'module', compiled)(loaded.exports, resolve, loaded)
  return loaded.exports
}

const helpers = loadTS(path.join(__dirname, 'subdomain-targets.ts'))
const { normalizeSubdomainTarget, validateSubdomainTargets } = helpers
const { validateCombinationTargetsForInputType } = loadTS(
  path.join(__dirname, '../app/(main)/scan/vuln-scan/components/CombinationTaskBuilder.tsx'),
  (id) => id === '@/lib/subdomain-targets' ? helpers : {},
)

test('Chinese domains are submitted as DNS-safe ASCII in standalone and combination tasks', () => {
  const targets = ['淘宝网.com', '例子.中国', 'XN--PBT173BPXM.COM', ' Example.COM. ']
  const expected = {
    validTargets: ['xn--pbt173bpxm.com', 'xn--fsqu00a.xn--fiqs8s', 'xn--pbt173bpxm.com', 'example.com'],
    invalidTargets: [],
  }
  assert.deepEqual(validateSubdomainTargets(targets), expected)
  assert.deepEqual(validateCombinationTargetsForInputType(targets, 'domain'), expected)
})

test('rejects non-domain targets and preserves the original invalid input for feedback', () => {
  const invalid = ['https://淘宝网.com', 'example.com:443', 'example.com/path', 'example.com?x=1',
    'example.com#fragment', 'user@example.com', '192.168.1.1', '127.1', '[::1]',
    '*.example.com', 'bad_name.com', '-bad.com', 'bad-.com', 'bad..com', 'localhost',
    'example.com\\path', 'exam\nple.com', 'exam\u0000ple.com', '%65xample.com', 'xn--.com']
  const expected = { validTargets: [], invalidTargets: invalid }
  assert.deepEqual(validateSubdomainTargets(invalid), expected)
  assert.deepEqual(validateCombinationTargetsForInputType(invalid, 'domain'), expected)
})

test('domain lengths are checked after IDNA conversion', () => {
  assert.ok(normalizeSubdomainTarget(`${'a'.repeat(63)}.com`))
  assert.equal(normalizeSubdomainTarget(`${'a'.repeat(64)}.com`), null)
  assert.ok(normalizeSubdomainTarget(`${'a'.repeat(63)}.${'b'.repeat(63)}.${'c'.repeat(63)}.${'d'.repeat(61)}`))
  assert.equal(normalizeSubdomainTarget(`${'a'.repeat(63)}.${'b'.repeat(63)}.${'c'.repeat(63)}.${'d'.repeat(62)}`), null)
  assert.equal(normalizeSubdomainTarget(`${'淘'.repeat(60)}.com`), null)
})

test('mixed batches retain usable domains and report only invalid targets', () => {
  assert.deepEqual(validateSubdomainTargets([' ', '淘宝网.com', ' https://example.com ', 'www.example.com']), {
    validTargets: ['xn--pbt173bpxm.com', 'www.example.com'],
    invalidTargets: ['https://example.com'],
  })
})

test('other combination input types keep their existing validation rules', () => {
  assert.deepEqual(validateCombinationTargetsForInputType(['192.168.1.1', '192.168.0.0/24'], 'network').invalidTargets, [])
  assert.deepEqual(validateCombinationTargetsForInputType(['https://example.com/path'], 'web').invalidTargets, [])
  assert.deepEqual(validateCombinationTargetsForInputType(['ssh://192.168.1.1:22', '淘宝网.com'], 'service'), {
    validTargets: ['ssh://192.168.1.1:22'], invalidTargets: ['淘宝网.com'],
  })
})
