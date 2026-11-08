const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const ts = require('typescript')
const source = fs.readFileSync(__dirname + '/ai-turn-preview.ts', 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS } }).outputText
const loaded = { exports: {} }
new Function('exports', 'require', 'module', compiled)(loaded.exports, require, loaded)
const { turnPreviewText, turnResultPreview } = loaded.exports

test('preview uses the final answer, skipping blank and reasoning-only trailing messages', () => {
  const messages = [
    { content: '准备开始验证' },
    { content: '## 检测结论\n**未发现注入证据**，请查看 [报告](https://example.test/report)。' },
    { content: '<think>内部推理</think>' },
    { content: '  ' },
  ]
  assert.equal(turnResultPreview(messages, '正在处理…'), '检测结论 未发现注入证据，请查看 报告。')
  assert.equal(messages.length, 4)
})

test('empty and streaming reasoning content retain the caller-provided actual state', () => {
  assert.equal(turnResultPreview([], '等待审批…'), '等待审批…')
  assert.equal(turnResultPreview([{ content: '<think>尚未结束的推理' }], '正在处理…'), '正在处理…')
  assert.equal(turnResultPreview([{ content: '' }], '本轮已停止，暂无结果摘要'), '本轮已停止，暂无结果摘要')
})

test('preview strips formatting but preserves code identifiers and Unicode characters', () => {
  assert.equal(turnPreviewText('> **结论**\n- `user_id` 返回正常\n```json\n{"ok":true}\n```'), '结论 user_id 返回正常 {"ok":true}')
  assert.equal(turnPreviewText('中文🙂🙂结束', 5), '中文🙂🙂…')
})
