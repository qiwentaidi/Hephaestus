import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

function loadModule(file, imports = {}) {
  const source = readFileSync(new URL(`../app/(main)/ai/workbench/${file}`, import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports = {}
  new Function('exports', 'require', code)(exports, (name) => {
    assert.ok(imports[name], `unexpected import ${name}`)
    return imports[name]
  })
  return exports
}
const { sessionGroupPagination } = loadModule('session-pagination.ts')

// Execute the real hook with deterministic effects and a paginated API fixture.
function mountSessions(api) {
  const slots = []
  let cursor = 0, dirty = true, effects = [], value
  let props = ['', { project: true }, 0]
  const same = (a, b) => a && a.length === b.length && a.every((v, i) => Object.is(v, b[i]))
  const react = {
    useRef(initial) { const i = cursor++; return slots[i] ??= { current: initial } },
    useState(initial) {
      const i = cursor++
      if (!(i in slots)) slots[i] = initial
      return [slots[i], (next) => { slots[i] = typeof next === 'function' ? next(slots[i]) : next; dirty = true }]
    },
    useCallback(fn, deps) { const i = cursor++; if (!same(slots[i]?.deps, deps)) slots[i] = { fn, deps }; return slots[i].fn },
    useEffect(fn, deps) { const i = cursor++; if (!same(slots[i], deps)) { slots[i] = deps; effects.push(fn) } },
  }
  const { useProjectSessions } = loadModule('use-project-sessions.ts', { react, '@/lib/api/ai-workbench': { listAIWorkbenchPenTestSessions: api } })
  const render = () => {
    for (let n = 0; dirty; n++) {
      assert.ok(n < 30, 'effect render loop')
      dirty = false; cursor = 0; effects = []
      value = useProjectSessions(...props)
      for (const effect of effects) effect()
    }
  }
  return {
    get current() { render(); return value },
    async settle() { for (let i = 0; i < 5; i++) { render(); await new Promise(setImmediate) }; render() },
    setProps(query, revision = 0) { props = [query, { project: true }, revision]; dirty = true; render() },
  }
}
const summary = (id, role) => ({ task: { id, projectId: 'project', agentRole: role } })
const rows = () => [summary('main', 'controller'), summary('planner', 'planner'), ...Array.from({ length: 207 }, (_, i) => summary(`worker-${i}`, 'worker'))]
const counts = { main: 1, planner: 1, workers: 207, other: 0 }
function fixture(data = rows()) {
  const calls = [], state = { data, fail: false }
  const api = async ({ page, pageSize, ...params }) => {
    calls.push({ page, pageSize, ...params })
    if (state.fail && page > 1) throw new Error('page unavailable')
    return { conversations: state.data.slice((page - 1) * pageSize, page * pageSize), total: state.data.length, page, pageSize, roleCounts: { ...counts, workers: state.data.length - 2 } }
  }
  return { calls, state, api }
}

test('207 workers load beyond 78 and retain all pages on refresh', async () => {
  const f = fixture(), hook = mountSessions(f.api)
  await hook.settle()
  assert.equal(hook.current.pages.project.conversations.length, 80)
  assert.equal(hook.current.pages.project.roleCounts.workers, 207)
  let visible = 5
  for (let i = 0; i < 50 && visible < 207; i++) {
    const page = hook.current.pages.project
    const loaded = page.conversations.filter(s => s.task.agentRole === 'worker').length
    const action = sessionGroupPagination(loaded, visible, page.conversations.length < page.total, page.roleCounts.workers)
    assert.equal(action.complete, false)
    if (action.needsFetch) assert.equal(await hook.current.load('project', true), true)
    visible = action.nextVisible
    await hook.settle()
  }
  assert.equal(visible, 207)
  assert.equal(hook.current.pages.project.conversations.length, 209)
  assert.ok(f.calls.some(c => c.page === 3))
  assert.equal(sessionGroupPagination(207, visible, false, 207).nextVisible, 5)
  f.state.data.splice(2, 0, summary('new-worker', 'worker'))
  hook.setProps('', 1)
  await hook.settle()
  assert.equal(hook.current.pages.project.page, 3)
  assert.equal(hook.current.pages.project.roleCounts.workers, 208)
  assert.ok(hook.current.pages.project.conversations.some(s => s.task.id === 'worker-206'))
})

test('failed next page retains loaded data and retries without duplicates', async () => {
  const f = fixture(), hook = mountSessions(f.api)
  await hook.settle()
  f.state.fail = true
  assert.equal(await hook.current.load('project', true), false)
  await hook.settle()
  assert.equal(hook.current.pages.project.page, 1)
  assert.equal(hook.current.pages.project.conversations.length, 80)
  assert.equal(hook.current.pages.project.error, true)
  assert.equal(hook.current.pages.project.retryMore, true)
  f.state.fail = false
  assert.equal(await hook.current.load('project', true), true)
  await hook.settle()
  const ids = hook.current.pages.project.conversations.map(s => s.task.id)
  assert.equal(ids.length, 160)
  assert.equal(new Set(ids).size, ids.length)
})

test('search changes discard an older in-flight page and expose filtered totals', async () => {
  let release
  const hook = mountSessions(async ({ search }) => {
    if (!search) return new Promise(resolve => { release = resolve })
    return { conversations: [summary('match', 'worker')], total: 1, roleCounts: { ...counts, main: 0, planner: 0, workers: 1 } }
  })
  hook.current
  hook.setProps('needle')
  await hook.settle()
  release({ conversations: rows().slice(0, 80), total: 209, roleCounts: counts })
  await hook.settle()
  assert.equal(hook.current.pages.project.query, 'needle')
  assert.equal(hook.current.pages.project.total, 1)
  assert.equal(hook.current.pages.project.roleCounts.workers, 1)
  assert.equal(hook.current.pages.project.conversations[0].task.id, 'match')
})

test('unloaded roles and ordinary sessions can fetch remaining pages', () => {
  assert.equal(sessionGroupPagination(0, 5, true, 207).needsFetch, true)
  assert.equal(sessionGroupPagination(80, 80, true, 90).needsFetch, true)
  assert.equal(sessionGroupPagination(1, 5, true, 1).showControl, false)
  assert.equal(sessionGroupPagination(90, 90, false, 90).complete, true)
})
