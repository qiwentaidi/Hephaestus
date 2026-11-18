import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const page = readFileSync(new URL('../app/(main)/ai/workbench/page.tsx', import.meta.url), 'utf8')

test('project list collapses by animating the wrapper width instead of unmounting', () => {
  const wrapper = page.match(/<div\s+data-slot="ai-workbench-project-sidebar"[\s\S]*?>\{projectSidebar\(false\)\}<\/div>/)
  assert.ok(wrapper, 'project sidebar wrapper is rendered unconditionally')
  const markup = wrapper[0]

  // 300ms width wipe, matching the main navigation collapse.
  assert.match(markup, /transition-\[width,border-color\]/)
  assert.match(markup, /duration-300/)
  assert.match(markup, /ease-in-out/)
  assert.match(markup, /motion-reduce:transition-none/)
  // Collapsed is a zero-width clip, not a remount.
  assert.match(markup, /sidebarCollapsed \? 'w-0 border-transparent' : 'w-\[310px\] border-border\/80'/)
  // Hidden content must leave the tab order and the accessibility tree.
  assert.match(markup, /inert=\{sidebarCollapsed\}/)
  assert.match(markup, /aria-hidden=\{sidebarCollapsed \|\| undefined\}/)
})

test('project list content keeps a fixed width so text does not re-wrap mid-transition', () => {
  // The animated wrapper clips; the aside itself must not shrink with it.
  assert.match(page, /mobile \? 'w-full border-r border-border\/80' : 'w-\[310px\]'/)
  // The desktop border lives on the animated wrapper so it fades with the width.
  const desktopAside = page.match(/projectSidebar = \(mobile: boolean\) => \{[\s\S]*?return <aside className=\{cn\(([^)]*)\)\}/)
  assert.ok(desktopAside, 'projectSidebar renders an aside')
  // Only the mobile sheet keeps its own border; the animated wrapper owns the
  // desktop one so it can fade out with the width.
  assert.doesNotMatch(desktopAside[1].split("mobile ?")[0], /border/)
  // No instant mount/unmount of the sidebar remains.
  assert.doesNotMatch(page, /\{!sidebarCollapsed \? <div className="hidden h-full lg:block">/)
})

test('the toolbar new-conversation shortcut fades in when the list collapses', () => {
  assert.match(page, /animate-in fade-in duration-200 motion-reduce:animate-none', !sidebarCollapsed && 'lg:hidden'\)/)
})
