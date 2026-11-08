import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { test } from 'node:test'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import ts from 'typescript'

const require = createRequire(import.meta.url)
const source = readFileSync(new URL('../app/(main)/ai/workbench/components/ConversationRail.tsx', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText
const passthrough = ({ children }) => React.createElement(React.Fragment, null, children)
const imports = {
  '@/lib/i18n': { useI18n: () => ({ t: { aiWorkbench: { turnNavigation: '对话轮次导航', turnNavigationLabel: '第 {count} 轮：{question}' } } }) },
  '@/lib/ai-workbench-i18n': { formatWorkbenchText: (template, values) => template.replace(/\{(\w+)\}/g, (_, k) => values[k]) },
  '@/components/ui/tooltip': { Tooltip: passthrough, TooltipContent: passthrough, TooltipTrigger: passthrough },
  '@/lib/utils': { cn: (...parts) => parts.filter(Boolean).join(' ') },
}
const exports = {}
new Function('exports', 'require', compiled)(exports, name => imports[name] || require(name))

test('tooltip and accessible label share the canonical history number, including round zero', () => {
  // A paged rail must use the server's global numbers rather than page indexes.
  const html = renderToStaticMarkup(React.createElement(exports.ConversationRail, {
    entries: [
      { id: 'setup', number: 0, question: '第 0 轮目标拆解', summary: 'setup' },
      { id: 'last', number: 331, question: '第 331 轮规划', summary: 'result' },
    ], activeId: 'last', bottomInset: 0, onNavigate() {},
  }))
  assert.match(html, /aria-label="第 0 轮：第 0 轮目标拆解"/)
  assert.match(html, /aria-label="第 331 轮：第 331 轮规划"/)
  assert.match(html, /第 331 轮规划<\/p>/)
  assert.match(html, /aria-current="step"/)
})

test('ordinary conversation entries still use their sequential positions', () => {
  const html = renderToStaticMarkup(React.createElement(exports.ConversationRail, {
    entries: [{ id: 'normal', question: 'hello', summary: 'result' }], activeId: 'normal', bottomInset: 0, onNavigate() {},
  }))
  assert.match(html, /aria-label="第 1 轮：hello"/)
})
