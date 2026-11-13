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

// --- Project token usage polling -------------------------------------------------
// The component is driven through deterministic fake timers and a minimal hook
// renderer: rapid session switches must settle into one read and must never abort
// a request that the browser already sent.

function createTimers() {
  let now = 0
  let nextId = 1
  const handles = new Map()
  const schedule = (fn, delay = 0, repeat = false) => {
    const wait = Math.max(repeat ? 1 : 0, delay)
    const id = nextId++
    handles.set(id, { time: now + wait, delay: wait, fn, repeat })
    return id
  }
  return {
    setTimeout: (fn, delay) => schedule(fn, delay, false),
    clearTimeout: (id) => { handles.delete(id) },
    setInterval: (fn, delay) => schedule(fn, delay, true),
    clearInterval: (id) => { handles.delete(id) },
    advance(ms) {
      const target = now + ms
      for (;;) {
        let due
        for (const [id, handle] of handles) {
          if (handle.time > target) continue
          if (!due || handle.time < due.handle.time || (handle.time === due.handle.time && id < due.id)) due = { id, handle }
        }
        if (!due) break
        now = due.handle.time
        if (due.handle.repeat) due.handle.time = now + due.handle.delay
        else handles.delete(due.id)
        due.handle.fn()
      }
      now = target
    },
  }
}

function createDocument() {
  const listeners = new Map()
  return {
    hidden: false,
    addEventListener(type, fn) {
      if (!listeners.has(type)) listeners.set(type, new Set())
      listeners.get(type).add(fn)
    },
    removeEventListener(type, fn) { listeners.get(type)?.delete(fn) },
    dispatch(type) { for (const fn of [...(listeners.get(type) || [])]) fn() },
  }
}

function createWorkbench(getUsage) {
  const slots = []
  const effects = new Map()
  let cursor = 0, dirty = false, pending = [], props = { projectId: undefined }, output
  let unmounted = false
  const sameDeps = (a, b) => Boolean(a && b) && a.length === b.length && a.every((value, index) => Object.is(value, b[index]))
  const hooks = {
    useState(initial) {
      const index = cursor++
      if (!(index in slots)) slots[index] = typeof initial === 'function' ? initial() : initial
      const set = (next) => {
        if (unmounted) return
        const value = typeof next === 'function' ? next(slots[index]) : next
        if (Object.is(value, slots[index])) return
        slots[index] = value
        dirty = true
      }
      return [slots[index], set]
    },
    useRef(initial) {
      const index = cursor++
      if (!(index in slots)) slots[index] = { current: initial }
      return slots[index]
    },
    useEffect(fn, deps) { pending.push({ index: cursor++, fn, deps }) },
  }
  const jsxRuntime = {
    jsx: (type, jsxProps) => ({ type, props: jsxProps }),
    jsxs: (type, jsxProps) => ({ type, props: jsxProps }),
  }
  const source = readFileSync(new URL('../app/(main)/ai/workbench/components/ProjectTokenUsage.tsx', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText
  const exports = {}
  const imports = {
    react: hooks,
    'react/jsx-runtime': jsxRuntime,
    '@/lib/api/ai-workbench': { getAIWorkbenchProjectTokenUsage: getUsage },
    './TokenUsage': { TokenUsageButton: tokenProps => ({ type: 'TokenUsageButton', props: tokenProps }) },
  }
  new Function('exports', 'require', compiled)(exports, name => {
    assert.ok(imports[name], `unexpected import ${name}`)
    return imports[name]
  })
  const commit = () => {
    const nextEffects = new Map()
    for (const { index, fn, deps } of pending) {
      const previous = effects.get(index)
      if (previous && sameDeps(previous.deps, deps)) { nextEffects.set(index, previous); continue }
      previous?.cleanup?.()
      nextEffects.set(index, { deps, cleanup: fn() })
    }
    for (const [index, previous] of effects) if (!nextEffects.has(index)) previous.cleanup?.()
    effects.clear()
    for (const [index, effect] of nextEffects) effects.set(index, effect)
    pending = []
  }
  const render = () => {
    for (let guard = 0; dirty; guard++) {
      assert.ok(guard < 25, 'render loop did not settle')
      dirty = false
      cursor = 0
      pending = []
      output = exports.ProjectTokenUsage(props)
      commit()
    }
    return output
  }
  return {
    get output() { return output },
    setProject(projectId) { props = { projectId }; dirty = true; return render() },
    render,
    unmount() {
      unmounted = true
      for (const effect of effects.values()) effect.cleanup?.()
      effects.clear()
    },
  }
}

function createUsageAPI() {
  const calls = []
  const get = (projectId, signal) => {
    const call = { projectId, signal, aborted: false }
    const promise = new Promise((resolve, reject) => {
      call.resolve = resolve
      signal?.addEventListener('abort', () => {
        call.aborted = true
        const error = new Error('The user aborted a request.')
        error.name = 'AbortError'
        reject(error)
      })
    })
    calls.push(call)
    return promise
  }
  return { calls, get }
}

const flush = async () => { for (let i = 0; i < 4; i++) await new Promise(resolve => setImmediate(resolve)) }

function withBrowser(timers, doc, body) {
  const previousWindow = globalThis.window
  const previousDocument = globalThis.document
  globalThis.window = {
    setTimeout: timers.setTimeout,
    clearTimeout: timers.clearTimeout,
    setInterval: timers.setInterval,
    clearInterval: timers.clearInterval,
  }
  globalThis.document = doc
  return body().finally(() => {
    globalThis.window = previousWindow
    globalThis.document = previousDocument
  })
}

test('project token usage: rapid session switches settle into one read', async () => {
  const timers = createTimers(), doc = createDocument(), api = createUsageAPI(), bench = createWorkbench(api.get)
  await withBrowser(timers, doc, async () => {
    try {
      bench.setProject('project-a')
      assert.equal(api.calls.length, 0, 'waits for the selection to settle')
      timers.advance(200)
      bench.setProject(undefined) // the next session is still loading
      timers.advance(100)
      bench.setProject('project-b')
      timers.advance(240)
      assert.equal(api.calls.length, 0, 'a burst of switches must not fire a request')
      timers.advance(10)
      assert.equal(api.calls.length, 1)
      assert.equal(api.calls[0].projectId, 'project-b')
      api.calls[0].resolve({ totalTokens: 42 })
      await flush()
      bench.render()
      assert.deepEqual(bench.output.props.usage, { totalTokens: 42 })
    } finally {
      bench.unmount()
    }
  })
})

test('project token usage: re-targeting never cancels an in-flight read', async () => {
  const timers = createTimers(), doc = createDocument(), api = createUsageAPI(), bench = createWorkbench(api.get)
  await withBrowser(timers, doc, async () => {
    try {
      bench.setProject('project-a')
      timers.advance(250)
      assert.equal(api.calls.length, 1)
      bench.setProject(undefined)
      bench.setProject('project-b')
      timers.advance(250)
      assert.equal(api.calls.length, 2)
      assert.equal(api.calls[0].aborted, false, 'the previous project read is left to finish')
      api.calls[0].resolve({ totalTokens: 1 })
      await flush()
      bench.render()
      assert.equal(bench.output.props.usage, undefined, 'a late previous answer is discarded')
      api.calls[1].resolve({ totalTokens: 2 })
      await flush()
      bench.render()
      assert.deepEqual(bench.output.props.usage, { totalTokens: 2 })
    } finally {
      bench.unmount()
    }
  })
})

test('project token usage: polls while visible and aborts only on unmount', async () => {
  const timers = createTimers(), doc = createDocument(), api = createUsageAPI(), bench = createWorkbench(api.get)
  await withBrowser(timers, doc, async () => {
    try {
      bench.setProject('project-a')
      timers.advance(250)
      api.calls[0].resolve({ totalTokens: 1 })
      await flush()
      bench.render()
      timers.advance(4000)
      assert.equal(api.calls.length, 2, 'polls again after the refresh interval')
      api.calls[1].resolve({ totalTokens: 1 })
      await flush()
      bench.render()
      doc.hidden = true
      timers.advance(4000)
      assert.equal(api.calls.length, 2, 'does not poll while the document is hidden')
      doc.hidden = false
      doc.dispatch('visibilitychange')
      await flush()
      bench.render()
      assert.equal(api.calls.length, 3, 'refreshes when the page becomes visible again')
    } finally {
      bench.unmount()
    }
    assert.equal(api.calls[2].aborted, true, 'leaving the workbench aborts the outstanding read')
    await flush()
  })
})
