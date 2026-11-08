import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import ts from 'typescript'

const require = createRequire(import.meta.url)
function load(path, imports = {}) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText
  const exports = {}
  new Function('exports', 'require', compiled)(exports, name => imports[name] || require(name))
  return exports
}
const passthrough = ({ children }) => React.createElement(React.Fragment, null, children)
for (const language of ['zh', 'en']) {
  const locale = load(`../lib/i18n/locales/${language}.ts`)
  const translations = Object.values(locale).find(value => value?.tokenUsage)
  const { TokenUsageButton } = load('../app/(main)/ai/workbench/components/TokenUsage.tsx', {
    '@/lib/i18n': { useI18n: () => ({ language, t: translations }) },
    '@/components/ui/tooltip': { Tooltip: passthrough, TooltipContent: passthrough, TooltipTrigger: passthrough },
    '@/lib/utils': { cn: (...parts) => parts.filter(Boolean).join(' ') },
  })
  test(`${language}: project and session are labeled separately with exact breakdowns`, () => {
    const usage = { totalTokens: 235500000, inputTokens: 200000000, outputTokens: 35500000, cacheReadTokens: 100000000 }
    const html = renderToStaticMarkup(React.createElement(TokenUsageButton, { scope: 'project', usage }))
    assert.ok(html.includes(`aria-label="${translations.tokenUsage.projectTotal}: 235,500,000"`))
    for (const value of ['235.5M', '200,000,000', '35,500,000', '100,000,000', translations.tokenUsage.projectRecordedOnly]) assert.ok(html.includes(value), value)
    const session = renderToStaticMarkup(React.createElement(TokenUsageButton, { usage: { totalTokens: 0 } }))
    assert.ok(session.includes(`aria-label="${translations.tokenUsage.currentSession}: 0"`))
    const pending = renderToStaticMarkup(React.createElement(TokenUsageButton, { scope: 'project' }))
    assert.ok(pending.includes(`aria-label="${translations.tokenUsage.projectTotal}: —"`))
  })
}
