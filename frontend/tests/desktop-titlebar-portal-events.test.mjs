import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const read = (path) => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8')

// Radix dialogs/popovers/tooltips render into document.body but still bubble
// their events through the React tree, so every titlebar handler that reacts to
// a window-level gesture must confirm the event landed inside the titlebar DOM.
const titlebars = [
  ['DesktopTitlebar', 'components/DesktopTitlebar.tsx'],
  ['LauncherWindowTitlebar', 'components/LauncherWindowTitlebar.tsx'],
  ['SettingsWindowTitlebar', 'components/SettingsWindowTitlebar.tsx'],
]

for (const [name, path] of titlebars) {
  test(`${name} ignores gestures coming from portaled content`, () => {
    const source = read(path)

    assert.match(
      source,
      /const isFromTitlebar = \(event: MouseEvent<HTMLDivElement>\) =>\s*\n\s*event\.target instanceof Node && event\.currentTarget\.contains\(event\.target\)/,
      `${name} must derive titlebar membership from DOM containment`,
    )

    const guards = source.match(/!isFromTitlebar\(event\)/g) || []
    assert.equal(
      guards.length,
      2,
      `${name} must guard both double-click (maximise toggle) and mouse-down (window drag)`,
    )
  })
}

test('screenshot preview dialog is exempt from titlebar gestures', () => {
  const source = read('components/DesktopScreenshotButton.tsx')
  const dialogContent = source.slice(source.indexOf('<DialogContent'))

  assert.match(dialogContent, /data-no-titlebar-toggle="true"/)
  assert.match(dialogContent, /data-no-titlebar-drag="true"/)
})
