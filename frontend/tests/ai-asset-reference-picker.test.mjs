import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const picker = readFileSync(new URL('../app/(main)/ai/workbench/components/AssetReferencePicker.tsx', import.meta.url), 'utf8')
const composer = readFileSync(new URL('../app/(main)/ai/workbench/page.tsx', import.meta.url), 'utf8')

test('asset reference pagination keeps composer focus instead of bubbling a dismiss', () => {
  // The picker renders inside the composer command popup; a page button that
  // takes focus used to drop the popup. Page controls must keep focus in the
  // search field, matching the task list pagination in ComposerCommandMenu.
  assert.match(picker, /disabled=\{loading \|\| page <= 1\} onMouseDown=\{\(event\) => event\.preventDefault\(\)\} onClick=\{\(\) => setPage\(page - 1\)\}/)
  assert.match(picker, /disabled=\{loading \|\| page \* pageSize >= total\} onMouseDown=\{\(event\) => event\.preventDefault\(\)\} onClick=\{\(\) => setPage\(page \+ 1\)\}/)
  assert.match(picker, /onMouseDown=\{\(event\) => event\.preventDefault\(\)\} onClick=\{\(\) => \{ setTask\(null\)/)
})

test('composer command popup dismisses only on pointer gestures outside itself', () => {
  // Dismissal must key off where the pointer starts, not on blur timing, so
  // clicking inside the popup (including its scrollbars and controls) keeps it.
  assert.match(composer, /data-slot="ai-workbench-composer"/)
  assert.match(composer, /target\.closest\('\[data-slot="ai-workbench-composer"\], \[data-composer-command-portal\]'\)/)
  assert.match(composer, /document\.addEventListener\('pointerdown', handlePointerDown, true\)/)
  assert.match(composer, /if \(!relatedTarget \|\| event\.currentTarget\.contains\(relatedTarget\)/)
  assert.doesNotMatch(composer, /commandPointerDownRef/)
})
