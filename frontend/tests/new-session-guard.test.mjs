import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

function loadModule(file) {
  const source = readFileSync(new URL(`../app/(main)/ai/workbench/${file}`, import.meta.url), 'utf8')
  const code = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports = {}
  new Function('exports', 'require', code)(exports, () => { throw new Error('unexpected runtime import') })
  return exports
}
function read(relative) {
  return readFileSync(new URL(`../${relative}`, import.meta.url), 'utf8')
}

const { activePlanRunForProject, isLivePlanRun } = loadModule('active-run-guard.ts')
const run = (id, projectId, status, updatedAt = '2026-11-21T00:00:00Z') => ({
  run: { id, projectId, status, updatedAt },
  project: { id: projectId, name: 'GJ微信小程序' },
  itemCounts: { completed: 3 },
  itemTotal: 9,
})

test('terminal runs no longer guard a new session', () => {
  for (const status of ['completed', 'cancelled', 'failed', 'blocked', ' COMPLETED ']) {
    assert.equal(isLivePlanRun(status), false, status)
  }
  for (const status of ['planning', 'waiting', 'running', 'pending', 'paused', 'pausing']) {
    assert.equal(isLivePlanRun(status), true, status)
  }
})

test('the guard picks the newest live run for the target project only', () => {
  const runs = [
    run('old', 'project', 'running', '2026-11-20T00:00:00Z'),
    run('new', 'project', 'running', '2026-11-22T00:00:00Z'),
    run('done', 'project', 'completed', '2026-11-23T00:00:00Z'),
    run('other-project', 'other', 'running', '2026-11-23T00:00:00Z'),
  ]
  assert.equal(activePlanRunForProject(runs, 'project')?.run.id, 'new')
  assert.equal(activePlanRunForProject(runs, 'other')?.run.id, 'other-project')
  assert.equal(activePlanRunForProject(runs, 'missing'), undefined)
  assert.equal(activePlanRunForProject(runs, ''), undefined)
  assert.equal(activePlanRunForProject([run('done', 'project', 'completed')], 'project'), undefined)
})

test('the guard ignores read-only legacy runs with no writable controller', () => {
  const legacy = run('artex:7', 'project', 'running', '2026-11-23T00:00:00Z')
  legacy.run.readOnly = true
  assert.equal(activePlanRunForProject([legacy], 'project'), undefined)
})

test('new-session entry points consult the guard instead of creating directly', () => {
  const page = read('app/(main)/ai/workbench/page.tsx')
  assert.match(page, /activePlanRunForProject/)
  assert.match(page, /NewSessionGuardDialog/)
  // Project menu, desktop button and mobile button all go through the guard.
  assert.match(page, /action === 'new-session'\) void requestNewConversation\(project\.id\)/)
  assert.equal((page.match(/void requestNewConversation\(\)/g) || []).length, 2)
  assert.match(page, /onContinueInController=\{\(\) => void continueInGuardedRun\(\)\}/)
  assert.match(page, /onCreateTask=\{openGuardedNewTask\}/)
})

test('the guard opens the run controller and hands the new task to the tasks page', () => {
  const page = read('app/(main)/ai/workbench/page.tsx')
  assert.match(page, /ensureAIPlanRunController\(guard\.run\.run\.id\)/)
  assert.match(page, /await loadSession\(controllerId\)/)
  assert.match(page, /params\.set\('source', run\.id\.slice\(ORCHESTRATION_RUN_PREFIX\.length\)\)/)
  assert.match(page, /router\.push\(`\/ai\/tasks\?\$\{params\.toString\(\)\}`\)/)
})

test('new conversation defaults to the selected project, not the first one', () => {
  const page = read('app/(main)/ai/workbench/page.tsx')
  // Both the guard target and the actual switch resolve the same project, so
  // the dialog can never describe a different project than the one created in.
  assert.match(page, /const nextProjectId = projectId \?\? \(selectedProjectId \|\| projects\[0\]\?\.id \|\| ''\)/)
  assert.match(page, /const targetProjectId = projectId \?\? \(selectedProjectId \|\| projects\[0\]\?\.id \|\| ''\)/)
})

test('the tasks page opens the orchestration form from the guard deep link', () => {
  const tasks = read('app/(main)/ai/tasks/components/TaskManagement.tsx')
  const dialog = read('app/(main)/ai/tasks/components/TaskCreateDialog.tsx')
  assert.match(tasks, /params\.get\('create'\) !== 'plan'/)
  assert.match(tasks, /params\.get\('source'\)/)
  assert.match(tasks, /initialMode=\{deepLinkPlanSource \? 'plan' : undefined\}/)
  assert.match(tasks, /initialPlanSourceTaskIds=\{deepLinkPlanSource \|\| undefined\}/)
  assert.match(dialog, /initialPlanSourceTaskIds\?: string\[\]/)
  assert.match(dialog, /initialPlanSourceTaskIds \|\| \[\]/)
})
