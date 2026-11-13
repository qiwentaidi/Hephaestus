import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import ts from 'typescript'

class WorkbenchRequestError extends Error {
  constructor(key, values = {}) {
    super(`request failed (${values.status ?? ''})`)
    this.name = 'WorkbenchRequestError'
    this.key = key
    this.values = values
  }
}

function loadWorkbenchAPI(fetchWithAuth) {
  const source = readFileSync(new URL('../lib/api/ai-workbench.ts', import.meta.url), 'utf8')
  const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText
  const exports = {}
  const imports = {
    '@/lib/api': { fetchWithAuth },
    '@/lib/ai-workbench-i18n': { WorkbenchRequestError },
    '@/lib/ai-process-stream': { readAIProcessFrames: async () => {} },
    '@/lib/download': { parseFilenameFromContentDisposition: () => '' },
    '@/lib/ai-goal-mode': {},
  }
  new Function('exports', 'require', compiled)(exports, name => {
    assert.ok(imports[name], `unexpected import ${name}`)
    return imports[name]
  })
  return exports
}

const response = (body, status = 200) => ({ ok: status >= 200 && status < 300, status, json: async () => body })

test('aborting while the body is read stays an AbortError', async () => {
  // The fetch already produced 200 headers, so the network panel shows a normal
  // response; only the body read is interrupted by the session switch.
  const abort = new DOMException('The user aborted a request.', 'AbortError')
  const api = loadWorkbenchAPI(async () => ({ ok: true, status: 200, json: async () => { throw abort } }))
  await assert.rejects(api.getAIWorkbenchPenTestSession('session-1'), error => {
    assert.equal(error.name, 'AbortError', 'callers ignore superseded requests by name')
    assert.equal(error instanceof WorkbenchRequestError, false, 'an abort must not become requestFailed(200)')
    return true
  })
})

test('a non-abort body failure still reports requestFailed with the HTTP status', async () => {
  const api = loadWorkbenchAPI(async () => ({ ok: true, status: 200, json: async () => { throw new SyntaxError('Unexpected token <') } }))
  await assert.rejects(api.getAIWorkbenchPenTestSession('session-1'), error => {
    assert.ok(error instanceof WorkbenchRequestError)
    assert.equal(error.key, 'requestFailed')
    assert.deepEqual(error.values, { status: 200 })
    return true
  })
})

test('an unsuccessful payload still surfaces the backend message', async () => {
  const api = loadWorkbenchAPI(async () => response({ success: false, error: '会话不存在或已被删除' }))
  await assert.rejects(api.getAIWorkbenchPenTestSession('session-1'), { message: '会话不存在或已被删除' })
})

test('a successful payload is unwrapped unchanged', async () => {
  const snapshot = { task: { id: 'session-1' } }
  const api = loadWorkbenchAPI(async () => response({ success: true, data: snapshot }))
  assert.deepEqual(await api.getAIWorkbenchPenTestSession('session-1'), snapshot)
})
