import DOMPurify from 'dompurify'
import hljs from 'highlight.js/lib/core'
import bash from 'highlight.js/lib/languages/bash'
import c from 'highlight.js/lib/languages/c'
import cpp from 'highlight.js/lib/languages/cpp'
import csharp from 'highlight.js/lib/languages/csharp'
import dockerfile from 'highlight.js/lib/languages/dockerfile'
import go from 'highlight.js/lib/languages/go'
import http from 'highlight.js/lib/languages/http'
import java from 'highlight.js/lib/languages/java'
import javascript from 'highlight.js/lib/languages/javascript'
import json from 'highlight.js/lib/languages/json'
import markdown from 'highlight.js/lib/languages/markdown'
import nginx from 'highlight.js/lib/languages/nginx'
import php from 'highlight.js/lib/languages/php'
import plaintext from 'highlight.js/lib/languages/plaintext'
import powershell from 'highlight.js/lib/languages/powershell'
import ruby from 'highlight.js/lib/languages/ruby'
import sql from 'highlight.js/lib/languages/sql'
import typescript from 'highlight.js/lib/languages/typescript'
import yaml from 'highlight.js/lib/languages/yaml'

for (const [name, language] of Object.entries({ bash, c, cpp, csharp, dockerfile, go, http, java, javascript, json, markdown, nginx, php, plaintext, powershell, ruby, sql, typescript, yaml })) {
  hljs.registerLanguage(name, language)
}

const languageAliases: Record<string, string> = {
  sh: 'bash', shell: 'bash', zsh: 'bash', yml: 'yaml',
  js: 'javascript', jsx: 'javascript', ts: 'typescript', tsx: 'typescript',
  py: 'python', ps1: 'powershell', 'c#': 'csharp', 'c++': 'cpp', text: 'plaintext', txt: 'plaintext',
}

function normalizeCodeLanguage(language: string) {
  const normalized = language.trim().toLowerCase()
  return languageAliases[normalized] || normalized
}

function highlightCode(code: string, language: string) {
  const normalized = normalizeCodeLanguage(language)
  if (normalized && hljs.getLanguage(normalized)) {
    try { return hljs.highlight(code, { language: normalized }).value } catch { /* fall through */ }
  }
  return escapeHTML(code)
}

export interface MarkdownRenderOptions {
  allowImages?: boolean
  remoteImages?: 'click' | 'allow'
  copyCodeLabel?: string
  loadImageLabel?: string
}


function escapeHTML(value: string) {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

function unescapeMarkdown(value: string) {
  return value.replace(/\\([\\`*_[\]()>#+\-.!|])/g, '$1')
}

function renderInlineMarkdown(value: string, options: MarkdownRenderOptions = {}) {
  const codeSpans: string[] = []
  let html = escapeHTML(value).replace(/`([^`]+)`/g, (_match, code: string) => {
    codeSpans.push(`<code>${code}</code>`)
    return `\u0000${codeSpans.length - 1}\u0000`
  })
  html = html.replace(/!\[([^\]]*)\]\((https?:\/\/[^)\s]+)\)/g, (_match, alt: string, url: string) => {
    const safeAlt = escapeHTML(alt)
    if (options.allowImages === false) return escapeHTML(alt)
    const label = options.loadImageLabel || 'Load remote image'
    if (options.remoteImages === 'allow') {
      return `<img src="${url}" alt="${safeAlt}" loading="lazy" referrerpolicy="no-referrer" />`
    }
    return `<button type="button" class="markdown-image-placeholder" data-remote-image="${url}" data-image-alt="${safeAlt}" aria-label="${escapeHTML(label)}${safeAlt ? ': ' + safeAlt : ''}">${escapeHTML(label)}${safeAlt ? ` · ${safeAlt}` : ''}</button>`
  })
  html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noreferrer">$1</a>')
  html = html.replace(/\*\*((?:\\.|(?!\*\*).)+?)\*\*/g, (_match, text: string) => `<strong>${unescapeMarkdown(text)}</strong>`)
  html = html.replace(/(^|[^a-zA-Z0-9_])__((?:\\.|(?!__).)+?)__(?![a-zA-Z0-9_])/g, (_match, prefix: string, text: string) => `${prefix}<strong>${unescapeMarkdown(text)}</strong>`)
  html = html.replace(/(^|[^\*])\*((?:\\.|[^*])+?)\*(?!\*)/g, (_match, prefix: string, text: string) => `${prefix}<em>${unescapeMarkdown(text)}</em>`)
  html = html.replace(/(^|[^a-zA-Z0-9_])_((?:\\.|[^_])+?)_(?![a-zA-Z0-9_])/g, (_match, prefix: string, text: string) => `${prefix}<em>${unescapeMarkdown(text)}</em>`)
  return unescapeMarkdown(html).replace(/\u0000(\d+)\u0000/g, (_match, index: string) => codeSpans[Number(index)] || '')
}

function isMarkdownTableSeparator(line: string) {
  return /^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?$/.test(line.trim())
}

function parseMarkdownTableRow(line: string) {
  let source = line.trim()
  if (source.startsWith('|')) source = source.slice(1)
  if (source.endsWith('|') && !source.endsWith('\\|')) source = source.slice(0, -1)
  const cells: string[] = []
  let current = ''
  let escaped = false
  let inCode = false
  for (const char of source) {
    if (char === '`' && !escaped) inCode = !inCode
    if (char === '|' && !escaped && !inCode) {
      cells.push(current.trim())
      current = ''
      continue
    }
    current += char
    escaped = char === '\\' && !escaped
    if (char !== '\\') escaped = false
  }
  cells.push(current.trim())
  return cells
}

export function renderMarkdownHTML(markdown: string, options: MarkdownRenderOptions = {}) {
  if (!markdown) return ''
  const lines = markdown.replace(/\r\n/g, '\n').split('\n')
  const html: string[] = []
  let paragraph: string[] = []
  let listType: 'ul' | 'ol' | null = null
  let codeBlock: string[] = []
  let codeLanguage = ''
  let inCodeBlock = false
  let blockquoteLines: string[] = []

  const flushParagraph = () => {
    if (!paragraph.length) return
    html.push(`<p>${paragraph.map(line => renderInlineMarkdown(line, options)).join('<br />')}</p>`)
    paragraph = []
  }
  const flushList = () => {
    if (!listType) return
    html.push(`</${listType}>`)
    listType = null
  }
  const flushCodeBlock = () => {
    if (!inCodeBlock) return
    const code = codeBlock.join('\n')
    const language = normalizeCodeLanguage(codeLanguage)
    const label = language || 'text'
    const highlighted = highlightCode(code, language)
    html.push(`<div class="markdown-code-block" data-code-language="${escapeHTML(label)}"><div class="markdown-code-header"><span class="markdown-code-language">${escapeHTML(label)}</span><button type="button" class="markdown-code-copy" data-copy-code aria-label="${escapeHTML(options.copyCodeLabel || 'Copy code')}">${escapeHTML(options.copyCodeLabel || 'Copy')}</button></div><pre><code class="hljs${language ? ` language-${escapeHTML(language)}` : ''}">${highlighted}</code></pre></div>`)
    codeBlock = []
    codeLanguage = ''
    inCodeBlock = false
  }
  const flushBlockquote = () => {
    if (!blockquoteLines.length) return
    html.push(`<blockquote>${blockquoteLines.join('<br />')}</blockquote>`)
    blockquoteLines = []
  }

  for (let index = 0; index < lines.length; index += 1) {
    const rawLine = lines[index]
    const line = rawLine.trimEnd()
    const trimmed = line.trim()

    if (trimmed.startsWith('```')) {
      flushParagraph()
      flushList()
      flushBlockquote()
      if (inCodeBlock) flushCodeBlock()
      else { inCodeBlock = true; codeBlock = []; codeLanguage = trimmed.slice(3).trim().split(/\s+/)[0] || '' }
      continue
    }
    if (inCodeBlock) {
      codeBlock.push(rawLine)
      continue
    }
    if (!trimmed) {
      flushParagraph()
      flushList()
      flushBlockquote()
      continue
    }
    if (trimmed.includes('|') && lines[index + 1] && isMarkdownTableSeparator(lines[index + 1])) {
      flushParagraph()
      flushList()
      flushBlockquote()
      const headers = parseMarkdownTableRow(trimmed)
      index += 2
      const rows: string[][] = []
      while (index < lines.length && lines[index].trim().includes('|')) {
        rows.push(parseMarkdownTableRow(lines[index]))
        index += 1
      }
      index -= 1
      html.push('<div class="markdown-table"><table><thead><tr>')
      html.push(headers.map((cell) => `<th>${renderInlineMarkdown(cell, options)}</th>`).join(''))
      html.push('</tr></thead><tbody>')
      for (const row of rows) html.push(`<tr>${row.map((cell) => `<td>${renderInlineMarkdown(cell, options)}</td>`).join('')}</tr>`)
      html.push('</tbody></table></div>')
      continue
    }
    if (trimmed.startsWith('> ')) {
      flushParagraph()
      flushList()
      blockquoteLines.push(renderInlineMarkdown(trimmed.slice(2), options))
      continue
    }
    flushBlockquote()
    const heading = trimmed.match(/^(#{1,4})\s+(.*)$/)
    if (heading) {
      flushParagraph()
      flushList()
      const level = heading[1].length
      html.push(`<h${level}>${renderInlineMarkdown(heading[2], options)}</h${level}>`)
      continue
    }
    if (/^---+$/.test(trimmed) || /^\*\*\*+$/.test(trimmed)) {
      flushParagraph()
      flushList()
      html.push('<hr />')
      continue
    }
    const unordered = trimmed.match(/^[-*]\s+(.*)$/)
    if (unordered) {
      flushParagraph()
      if (listType !== 'ul') { flushList(); listType = 'ul'; html.push('<ul>') }
      html.push(`<li>${renderInlineMarkdown(unordered[1], options)}</li>`)
      continue
    }
    const ordered = trimmed.match(/^\d+\.\s+(.*)$/)
    if (ordered) {
      flushParagraph()
      if (listType !== 'ol') { flushList(); listType = 'ol'; html.push('<ol>') }
      html.push(`<li>${renderInlineMarkdown(ordered[1], options)}</li>`)
      continue
    }
    flushList()
    paragraph.push(trimmed)
  }

  flushParagraph()
  flushList()
  flushBlockquote()
  flushCodeBlock()

  return DOMPurify.sanitize(html.join(''), {
    USE_PROFILES: { html: true },
    ADD_ATTR: ['target', 'rel', 'class', 'loading', 'referrerpolicy', 'data-remote-image', 'data-image-alt', 'data-copy-code', 'data-code-language', 'aria-label'],
    FORBID_TAGS: options?.allowImages === false ? ['img'] : [],
  })
}
