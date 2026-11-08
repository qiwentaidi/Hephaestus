const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const file = path.join(__dirname, '../app/(main)/scan/vuln-scan/components/fingerprint-status.ts')
const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText
const loaded = { exports: {} }
new Function('exports', 'require', 'module', compiled)(loaded.exports, require, loaded)
const { matchesFingerprintStatus } = loaded.exports

const statuses = [0, 199, 200, 201, 204, 299, 300, 301, 302, 399, 400, 401, 403, 404, 429, 499, 500, 502, 503, 599, 600]
for (const [filter, expected] of [
  ['2xx', [200, 201, 204, 299]],
  ['3xx', [300, 301, 302, 399]],
  ['4xx', [400, 401, 403, 404, 429, 499]],
  ['5xx', [500, 502, 503, 599]],
  ['all', statuses],
  ['', statuses],
  ['400', [400]],
  ['404', [404]],
]) {
  test(`${filter || 'empty'} matches the expected status codes including range boundaries`, () => {
    assert.deepEqual(statuses.filter(status => matchesFingerprintStatus(status, filter)), expected)
  })
}
