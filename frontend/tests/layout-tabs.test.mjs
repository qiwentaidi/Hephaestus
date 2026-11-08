import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

const code = readFileSync(new URL('../lib/layout-tabs.ts', import.meta.url), 'utf8')
const js = ts.transpileModule(code, { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { mergeLayoutTab, isSingleTabRoute } = await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)

test('AI conversations share one content tab that follows the latest session', () => {
  let tabs = []
  tabs = mergeLayoutTab(tabs, '/ai/workbench', '对话')
  tabs = mergeLayoutTab(tabs, '/ai/workbench?session=a', '对话')
  tabs = mergeLayoutTab(tabs, '/ai/workbench?session=b', '对话')
  assert.deepEqual(tabs, [{ href: '/ai/workbench?session=b', title: '对话' }])
})

test('single-tab routes collapse duplicates created before the route was unified', () => {
  let tabs = []
  tabs = mergeLayoutTab(tabs, '/ai/workbench?session=a', '对话')
  tabs = mergeLayoutTab(tabs, '/dashboard', '仪表板')
  tabs = mergeLayoutTab(tabs, '/ai/workbench?session=b', '对话')
  tabs = mergeLayoutTab(tabs, '/ai/workbench?session=c', '对话')
  assert.deepEqual(tabs, [
    { href: '/ai/workbench?session=c', title: '对话' },
    { href: '/dashboard', title: '仪表板' },
  ])
})

test('settings and batch task tabs keep their existing single-tab behavior', () => {
  assert.equal(isSingleTabRoute('/settings?tabs=ai'), true)
  assert.equal(isSingleTabRoute('/ai/tasks?task=1'), true)
  assert.equal(isSingleTabRoute('/ai/workbench?session=a'), true)
})

test('finding list pagination reuses one vulnerability management tab', () => {
  let tabs = []
  tabs = mergeLayoutTab(tabs, '/vulnerabilities', '漏洞管理')
  tabs = mergeLayoutTab(tabs, '/vulnerabilities?page=2', '漏洞管理')
  tabs = mergeLayoutTab(tabs, '/vulnerabilities?page=2&pageSize=50', '漏洞管理')
  assert.deepEqual(tabs, [{ href: '/vulnerabilities?page=2&pageSize=50', title: '漏洞管理' }])
  assert.equal(isSingleTabRoute('/vulnerabilities?page=2'), true)
})

test('asset center categories reuse one asset tab without merging sibling pages', () => {
  let tabs = []
  tabs = mergeLayoutTab(tabs, '/asset-center', '资产中心')
  tabs = mergeLayoutTab(tabs, '/asset-center?category=ip', '资产中心')
  tabs = mergeLayoutTab(tabs, '/asset-center?category=port', '资产中心')
  assert.deepEqual(tabs, [{ href: '/asset-center?category=port', title: '资产中心' }])
  assert.equal(isSingleTabRoute('/asset-center?category=ip'), true)
  assert.equal(isSingleTabRoute('/asset-center/topology'), false)
  assert.equal(isSingleTabRoute('/asset-center/map'), false)
})

test('other routes still open one tab per URL', () => {
  let tabs = []
  tabs = mergeLayoutTab(tabs, '/scan/vuln-scan?task=1', '漏洞扫描')
  tabs = mergeLayoutTab(tabs, '/scan/vuln-scan?task=2', '漏洞扫描')
  tabs = mergeLayoutTab(tabs, '/scan/vuln-scan', '漏洞扫描')
  assert.deepEqual(tabs, [
    { href: '/scan/vuln-scan?task=1', title: '漏洞扫描' },
    { href: '/scan/vuln-scan?task=2', title: '漏洞扫描' },
    { href: '/scan/vuln-scan', title: '漏洞扫描' },
  ])
})

test('re-merging the active href leaves the tab list untouched', () => {
  const tabs = [{ href: '/ai/workbench?session=b', title: '对话' }]
  assert.equal(mergeLayoutTab(tabs, '/ai/workbench?session=b', '对话'), tabs)
})
