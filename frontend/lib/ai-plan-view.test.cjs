const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const ts = require('typescript')
function load(file, requireModule) {
  const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
  const module = { exports: {} }
  new Function('exports', 'require', 'module', compiled)(module.exports, requireModule, module)
  return module.exports
}
const api = load(__dirname + '/api/ai-plan.ts', () => ({}))
const { canPausePlanRun, canResumePlanRun, filterPlanItems } = load(__dirname + '/ai-plan-view.ts', () => api)

test('only writable native tasks expose lifecycle actions, and resume requires acknowledged pause', () => {
  for (const id of ['orchestration:1', 'artex:1']) {
    for (const status of ['planning', 'waiting', 'pausing', 'paused', 'cancelled', 'completed', 'failed', 'blocked']) {
      assert.equal(canPausePlanRun({ id, status }), ['planning', 'waiting'].includes(status), status)
      assert.equal(canResumePlanRun({ id, status }), status === 'paused', status)
      assert.equal(canPausePlanRun({ id, status, readOnly: true }), false)
      assert.equal(canResumePlanRun({ id, status, readOnly: true }), false)
    }
  }
  assert.equal(canPausePlanRun({ id: 'historical-run', status: 'planning' }), false)
  assert.equal(canResumePlanRun({ id: 'historical-run', status: 'paused' }), false)
})

test('paused filter stays separate from completed and cancelled work', () => {
  const items = [{ id: 'a', status: 'completed' }, { id: 'b', status: 'paused' }, { id: 'c', status: 'cancelled' }]
  assert.deepEqual(filterPlanItems(items, 'paused', '').map(item => item.id), ['b'])
  assert.deepEqual(items.map(item => item.status), ['completed', 'paused', 'cancelled'])
})

test('pause and resume use the original run ID and propagate API failures', async () => {
  const requests = []
  let response = { success: true, data: { id: 'orchestration:42', status: 'planning' } }
  const client = load(__dirname + '/api/ai-plan.ts', () => ({ fetchWithAuth: async (path, options) => {
    requests.push({ path, method: options.method })
    return { ok: response.success, status: response.success ? 200 : 409, json: async () => response }
  } }))
  assert.equal((await client.resumeAIPlanRun('orchestration:42')).status, 'planning')
  response = { success: true, data: { run: { id: 'orchestration:42', status: 'pausing' }, stopPending: true } }
  assert.equal((await client.pauseAIPlanRun('orchestration:42')).stopPending, true)
  assert.deepEqual(requests, [
    { path: '/api/v1/ai/plan-runs/orchestration%3A42/resume', method: 'POST' },
    { path: '/api/v1/ai/plan-runs/orchestration%3A42/pause', method: 'POST' },
  ])
  response = { success: false, error: 'task is still stopping' }
  await assert.rejects(client.resumeAIPlanRun('orchestration:42'), /task is still stopping/)
})
