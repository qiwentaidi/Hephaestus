const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

const file = path.join(__dirname, 'finding-list-query.ts')
const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.CommonJS },
}).outputText
const loaded = { exports: {} }
new Function('exports', 'require', 'module', compiled)(loaded.exports, require, loaded)
const { findingListHref, parseFindingListPage, parseFindingListPageSize } = loaded.exports

test('finding list page params fall back to the first page for invalid values', () => {
  assert.equal(parseFindingListPage('3'), 3)
  assert.equal(parseFindingListPage('1'), 1)
  assert.equal(parseFindingListPage('0'), 1)
  assert.equal(parseFindingListPage('-2'), 1)
  assert.equal(parseFindingListPage('abc'), 1)
  assert.equal(parseFindingListPage(''), 1)
  assert.equal(parseFindingListPage(null), 1)
})

test('finding list page size only accepts the offered sizes', () => {
  assert.equal(parseFindingListPageSize('50'), 50)
  assert.equal(parseFindingListPageSize('10'), 10)
  assert.equal(parseFindingListPageSize('15'), 20)
  assert.equal(parseFindingListPageSize(null), 20)
})

test('finding list href keeps non-default pagination and drops defaults', () => {
  assert.equal(findingListHref(1, 20), '/vulnerabilities')
  assert.equal(findingListHref(3, 20), '/vulnerabilities?page=3')
  assert.equal(findingListHref(1, 50), '/vulnerabilities?pageSize=50')
  assert.equal(findingListHref(3, 50), '/vulnerabilities?page=3&pageSize=50')
})

test('finding list href replaces stale pagination params instead of duplicating them', () => {
  assert.equal(findingListHref(4, 20, '?page=1&pageSize=50'), '/vulnerabilities?page=4')
  assert.equal(findingListHref(1, 20, '?page=7'), '/vulnerabilities')
})
