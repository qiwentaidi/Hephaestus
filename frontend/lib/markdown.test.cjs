const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const ts = require('typescript')

function loadMarkdown() {
  const source = fs.readFileSync(__dirname + '/markdown.ts', 'utf8')
  const compiled = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, esModuleInterop: true },
  }).outputText
  const loadedModule = { exports: {} }
  const hljs = { registerLanguage() {}, getLanguage() { return null }, highlight() { return '' } }
  const requireModule = (name) => {
    if (name === 'dompurify') return { __esModule: true, default: { sanitize: html => html } }
    if (name === 'highlight.js/lib/core') return { __esModule: true, default: hljs }
    if (name.startsWith('highlight.js/lib/languages/')) return { __esModule: true, default: () => ({}) }
    throw new Error(`Unexpected import: ${name}`)
  }
  new Function('exports', 'require', 'module', compiled)(loadedModule.exports, requireModule, loadedModule)
  return loadedModule.exports
}

test('remote markdown images are click-to-load and never auto-requested', () => {
  const { renderMarkdownHTML } = loadMarkdown()
  const html = renderMarkdownHTML('![shot](https://example.test/a.png)', {
    remoteImages: 'click', loadImageLabel: 'Load remote image',
  })
  assert.match(html, /data-remote-image="https:\/\/example\.test\/a\.png"/)
  assert.match(html, /markdown-image-placeholder/)
  assert.doesNotMatch(html, /<img /)
})

test('code fences expose language, copy control and highlighted markup', () => {
  const { renderMarkdownHTML } = loadMarkdown()
  const html = renderMarkdownHTML('```json\n{"a":1}\n```', {
    copyCodeLabel: 'Copy code',
  })
  assert.match(html, /markdown-code-block/)
  assert.match(html, /data-copy-code/)
  assert.match(html, /markdown-code-language">json/)
  assert.match(html, /class="hljs language-json"/)
  assert.match(html, /\{&quot;a&quot;:1\}/)
})

test('report mode removes images entirely', () => {
  const { renderMarkdownHTML } = loadMarkdown()
  const html = renderMarkdownHTML('![shot](https://example.test/a.png)', { allowImages: false })
  assert.doesNotMatch(html, /<img |data-remote-image/)
})
