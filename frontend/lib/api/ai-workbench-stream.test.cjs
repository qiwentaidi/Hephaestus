const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const ts = require('typescript')

function loadTS(path, requireModule) {
  const compiled = ts.transpileModule(fs.readFileSync(path, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const loadedModule = { exports: {} }
  new Function('exports', 'require', 'module', compiled)(loadedModule.exports, requireModule, loadedModule)
  return loadedModule.exports
}

function loadWorkbenchI18n() {
  return loadTS(__dirname + '/../ai-workbench-i18n.ts', name => {
    if (name === '@/lib/i18n/locales/zh') return loadTS(__dirname + '/../i18n/locales/zh.ts', require)
    if (name === '@/lib/i18n/locales/en') return loadTS(__dirname + '/../i18n/locales/en.ts', require)
    throw new Error(`Unexpected import: ${name}`)
  })
}

test('message admission errors preserve the backend reason and localize the send failure', async () => {
  const i18n = loadWorkbenchI18n()
  const { en } = loadTS(__dirname + '/../i18n/locales/en.ts', require)
  let response
  const api = loadTS(__dirname + '/ai-workbench.ts', name => {
    if (name === '@/lib/ai-process-stream') return loadTS(__dirname + '/../ai-process-stream.ts', require)
    if (name === '@/lib/ai-workbench-i18n') return i18n
    if (name === '@/lib/download') return { parseFilenameFromContentDisposition: () => '' }
    if (name === '@/lib/api') return { fetchWithAuth: async () => response }
    throw new Error(`Unexpected import: ${name}`)
  })
  for (const [body, reason] of [
    [{ success: false, error: '超过 10000 个资产，请缩小筛选范围后引用' }, '超过 10000 个资产，请缩小筛选范围后引用'],
    [{ message: '  source task/version no longer exists  ' }, 'source task/version no longer exists'],
    [{ error: {}, message: 'Readable fallback' }, 'Readable fallback'],
    [{ error: ' ' }, ''],
    [null, ''],
    ['<html>Proxy error</html>', ''],
  ]) {
    response = new Response(typeof body === 'string' ? body : JSON.stringify(body), { status: 400 })
    await assert.rejects(api.streamAIWorkbenchPenTestSessionMessage('main', 'continue', {
      onOpen() { assert.fail('rejected request opened a stream') },
      onEvent() { assert.fail('rejected request emitted an event') },
    }), error => {
      assert.ok(error instanceof i18n.WorkbenchRequestError)
      assert.equal(error.values.status, 400)
      assert.equal(error.message, `发送失败（400）${reason ? `：${reason}` : ''}`)
      assert.equal(i18n.workbenchErrorMessage(error, en.aiWorkbench, 'fallback'), `Failed to send message (400)${reason ? `: ${reason}` : ''}`)
      return true
    })
  }
  const aborted = new DOMException('Cancelled', 'AbortError')
  response = { ok: false, status: 400, body: {}, json: async () => { throw aborted } }
  await assert.rejects(api.streamAIWorkbenchPenTestSessionMessage('main', 'continue', { onEvent() {} }), error => error === aborted)
})

test('rejected repeated text restores input and references without overwriting newer input', async () => {
  const { WorkbenchRequestError } = loadWorkbenchI18n()
  const source = ts.createSourceFile('page.tsx', fs.readFileSync(__dirname + '/../../app/(main)/ai/workbench/page.tsx', 'utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  let callback
  function visit(node) {
    if (ts.isVariableDeclaration(node) && node.name.getText(source) === 'handleSend') callback = node.initializer.arguments[0].getText(source)
    ts.forEachChild(node, visit)
  }
  visit(source)
  assert.ok(callback, 'send handler must exist')
  const compiled = ts.transpileModule(`module.exports = ${callback}`, { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText
  for (const newerInput of ['', 'New text while request was pending']) {
    const references = [{ taskId: 'ports', version: 1, selection: 'filtered', kind: 'port', count: 10001, name: 'Ports' }]
    const state = { draft: 'continue', attachments: [], assetReferences: references, quote: null, mcpTools: [] }
    const queued = []
    const errors = []
    const noop = () => {}
    const env = {
      WorkbenchRequestError, activeSessionId: 'main', activeSessionRef: { current: 'main' },
      draft: 'continue', messageQuote: null, attachments: [], assetReferences: references, mcpTools: [],
      language: 'zh', l: {}, goalMode: 'chat', effectiveSelectedModel: 'fixture',
      movingSessionRef: { current: null }, stopping: false, sending: false,
      session: { task: { id: 'main' } }, sessionIsActive: () => false,
      messageControllersRef: { current: new Map() }, composerSnapshotRef: { current: state },
      messageWithMCPTools: value => value, messageContentWithQuote: value => value,
      splitModelValue: () => ({ providerId: 'fixture', model: 'fixture' }),
      setDraft: value => { state.draft = value }, setMessageQuote: value => { state.quote = value },
      setAttachments: value => { state.attachments = value }, setAssetReferences: value => { state.assetReferences = value },
      setMCPTools: value => { state.mcpTools = value }, markSessionSending: noop, clearLiveText: noop,
      setSession: noop, receiveSnapshot: noop, syncSessionSnapshotToSidebar: noop, refreshWorkspace: noop,
      streamAIWorkbenchPenTestSessionMessage: async () => {
        state.draft = newerInput
        throw new WorkbenchRequestError('messageStreamFailedWithReason', { status: 400, reason: 'Asset limit' })
      },
      // The old message has the same text but does not represent this rejected request.
      getAIWorkbenchPenTestSession: async () => ({ turns: [{ userMessage: { content: 'continue' } }] }),
      showWorkbenchError: error => errors.push(error), attachmentId: () => 'failed-request',
      setQueuedMessages: update => { queued.splice(0, queued.length, ...update(queued)) },
    }
    const mod = { exports: {} }
    new Function('module', ...Object.keys(env), compiled)(mod, ...Object.values(env))
    await mod.exports()
    assert.equal(errors.length, 1)
    if (!newerInput) {
      assert.equal(state.draft, 'continue')
      assert.deepEqual(state.assetReferences, references)
      assert.equal(queued.length, 0)
    } else {
      assert.equal(state.draft, newerInput)
      assert.equal(queued.length, 1)
      assert.equal(queued[0].content, 'continue')
      assert.equal(queued[0].status, 'failed')
      assert.deepEqual(queued[0].assetReferences, references)
    }
  }
})

test('GET and POST live subscriptions resume without a blocked custom CORS header', async () => {
  const { readAIProcessFrames } = loadTS(__dirname + '/../ai-process-stream.ts', require)
  const requests = []
  const batches = []
  const api = loadTS(__dirname + '/ai-workbench.ts', name => {
    if (name === '@/lib/ai-process-stream') return { readAIProcessFrames }
    if (name === '@/lib/ai-workbench-i18n') return { WorkbenchRequestError: Error }
    if (name === '@/lib/download') return { parseFilenameFromContentDisposition: () => '' }
    if (name === '@/lib/api') return {
      fetchWithAuth: async (url, options) => {
        requests.push({ url: new URL(url, 'http://backend.test'), options })
        const frames = (batches.shift() || []).map(event => `event: process_update\ndata: ${JSON.stringify(event)}\n\n`).join('')
        return new Response(frames, { headers: { 'Content-Type': 'text/event-stream' } })
      },
    }
    throw new Error(`Unexpected import: ${name}`)
  })
  const seen = []
  const process = { sessionId: 'session', blockId: 'block', sequence: 1, item: null }
  batches.push([{ cursor: 42, process }])
  await api.streamAIWorkbenchPenTestSession('session', {
    onProcess: update => seen.push(update), onSnapshot() {},
  })
  assert.deepEqual(seen, [process])
  assert.equal(requests[0].url.searchParams.has('lastEventId'), false)

  // The POST stream can receive an older duplicate while GET is already ahead.
  batches.push([{ cursor: 40, process }])
  await api.streamAIWorkbenchPenTestSessionMessage('session', 'Continue', { onEvent() {} })
  assert.equal(requests[1].url.searchParams.get('lastEventId'), '42')
  assert.equal(requests[1].options.method, 'POST')
  assert.equal(JSON.parse(requests[1].options.body).content, 'Continue')
  await api.streamAIWorkbenchPenTestSession('session', {
    signature: 'current', intervalMs: 300, onSnapshot() {},
  })
  assert.equal(requests[2].url.searchParams.get('lastEventId'), '42')
  assert.equal(requests[2].url.searchParams.get('signature'), 'current')
  for (const { options } of requests) {
    const headers = new Headers(options.headers)
    assert.equal(headers.has('Last-Event-ID'), false)
    assert.equal(headers.get('Accept'), 'text/event-stream')
  }
  await api.streamAIWorkbenchPenTestSession('other/session', { onSnapshot() {} })
  assert.equal(requests[3].url.pathname, '/api/v1/ai/sessions/other%2Fsession/stream')
  assert.equal(requests[3].url.searchParams.has('lastEventId'), false)
})

test('side chat returns the final frame so a completed answer is never treated as empty', async () => {
  const { readAIProcessFrames } = loadTS(__dirname + '/../ai-process-stream.ts', require)
  const responses = [
    // Deltas plus the authoritative completion frame.
    'event: open\ndata: {}\n\n' +
      'event: delta\ndata: {"delta":"partial","accumulated":"partial"}\n\n' +
      'event: done\ndata: {"content":"partial answer"}\n\n',
    // Some providers/proxies only deliver the completion frame.
    'event: open\ndata: {}\n\n' +
      'event: done\ndata: {"content":"final only"}\n\n',
  ]
  const api = loadTS(__dirname + '/ai-workbench.ts', name => {
    if (name === '@/lib/ai-process-stream') return { readAIProcessFrames }
    if (name === '@/lib/ai-workbench-i18n') return { WorkbenchRequestError: Error }
    if (name === '@/lib/download') return { parseFilenameFromContentDisposition: () => '' }
    if (name === '@/lib/api') return {
      fetchWithAuth: async () => new Response(responses.shift(), { headers: { 'Content-Type': 'text/event-stream' } }),
    }
    throw new Error(`Unexpected import: ${name}`)
  })
  const deltas = []
  const first = await api.streamAIWorkbenchSideChat([{ role: 'user', content: 'hi' }], {
    sessionId: 'session',
    onDelta: (delta, accumulated) => deltas.push({ delta, accumulated }),
  })
  assert.equal(first, 'partial answer')
  assert.deepEqual(deltas, [{ delta: 'partial', accumulated: 'partial' }])

  const second = await api.streamAIWorkbenchSideChat([{ role: 'user', content: 'hi' }], {
    sessionId: 'session',
    onDelta: () => { throw new Error('no delta expected') },
  })
  assert.equal(second, 'final only')
})

test('SSE error frames reject instead of resolving silently', async () => {
  const { readAIProcessFrames } = loadTS(__dirname + '/../ai-process-stream.ts', require)
  const responses = [
    'event: error\ndata: {"error":"attachment is not a supported image"}\n\n',
    'event: error\ndata: {"error":"snapshot unavailable"}\n\n',
    new Response('not found', { status: 404 }),
  ]
  const api = loadTS(__dirname + '/ai-workbench.ts', name => {
    if (name === '@/lib/ai-process-stream') return { readAIProcessFrames }
    if (name === '@/lib/ai-workbench-i18n') return { WorkbenchRequestError: Error }
    if (name === '@/lib/download') return { parseFilenameFromContentDisposition: () => '' }
    if (name === '@/lib/api') return {
      fetchWithAuth: async () => {
        const item = responses.shift()
        return typeof item === 'string'
          ? new Response(item, { headers: { 'Content-Type': 'text/event-stream' } })
          : item
      },
    }
    throw new Error(`Unexpected import: ${name}`)
  })

  await assert.rejects(
    api.streamAIWorkbenchPenTestSessionMessage('session', 'hello', { onEvent() {} }),
    /attachment is not a supported image/,
  )
  await assert.rejects(
    api.streamAIWorkbenchPenTestSession('session', { onSnapshot() {} }),
    /snapshot unavailable/,
  )
  await assert.rejects(
    api.streamAIWorkbenchPenTestSession('missing', { onSnapshot() {} }),
    error => {
      assert.equal(error.fatal, true)
      assert.equal(error.status, 404)
      return true
    },
  )
})
