const { test } = require('node:test')
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const ts = require('typescript')

function loadTS(file, resolve = require) {
  const compiled = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
  }).outputText
  const loaded = { exports: {} }
  new Function('exports', 'require', 'module', compiled)(loaded.exports, resolve, loaded)
  return loaded.exports
}

const theme = loadTS(path.join(__dirname, 'custom-theme.ts'), (id) => {
  if (id === '@/lib/api') {
    return {
      persistDesktopConfigItems: async () => {},
      removeDesktopConfigItems: async () => {},
    }
  }
  return require(id)
})

function validDefinition(overrides = {}) {
  return {
    schemaVersion: 1,
    base: 'dark',
    radius: '0.5rem',
    colors: {
      background: '#101114',
      foreground: '#ececf1',
      card: '#181a20',
      primary: '#5b8def',
      primaryForeground: '#ffffff',
      muted: '#1c1f26',
      mutedForeground: '#a0a7b4',
      border: '#2a2f3a',
      sidebar: '#0c0d10',
      sidebarForeground: '#d7dae2',
      destructive: '#e5484d',
      destructiveForeground: '#ffffff',
      ring: '#5b8def',
    },
    ...overrides,
  }
}

test('builds a scoped stylesheet with derived semantic tokens', () => {
  const css = theme.buildThemeCss(theme.parseThemeDefinition(validDefinition()))
  assert.match(css, /^html\.custom \{/)
  assert.match(css, /color-scheme: dark;/)
  assert.match(css, /--background: #101114;/)
  assert.match(css, /--tone-blue: #60a5fa;/)
  assert.match(css, /--sidebar-accent: color-mix/)
  assert.doesNotMatch(css, /--chart-/)
})

test('rejects invalid colors, radii, and schema versions', () => {
  const cases = [
    validDefinition({ colors: { ...validDefinition().colors, primary: 'red' } }),
    validDefinition({ colors: { ...validDefinition().colors, primary: '#fff;background:url(x)' } }),
    validDefinition({ radius: 'calc(1rem)' }),
    validDefinition({ base: 'sepia' }),
    validDefinition({ schemaVersion: 2 }),
  ]
  for (const definition of cases) {
    assert.throws(() => theme.parseThemeDefinition(definition))
  }
})

test('normalizes names case-insensitively and detects bundle duplicates', () => {
  assert.equal(theme.normalizeThemeName('  Ocean   Blue '), 'Ocean Blue')
  assert.equal(theme.themeNameKey('  Ocean   Blue '), 'ocean blue')

  const bundle = {
    kind: 'hephaestus.theme.library',
    schemaVersion: 1,
    themes: [
      { name: 'Ocean', definition: validDefinition() },
      { name: ' ocean ', definition: validDefinition() },
    ],
  }
  assert.throws(
    () => theme.parseThemeLibraryBundle(bundle),
    (error) => error.code === 'duplicate_name' && error.conflicts.includes('ocean'),
  )
})

test('accepts a valid bundle and wraps exported themes', () => {
  const bundle = {
    kind: 'hephaestus.theme.library',
    schemaVersion: 1,
    themes: [
      { name: 'Ocean', definition: validDefinition() },
      { name: 'Sand', definition: validDefinition({ base: 'light' }) },
    ],
  }
  const parsed = theme.parseThemeLibraryBundle(bundle)
  assert.equal(parsed.themes.length, 2)
  assert.equal(parsed.themes[1].definition.base, 'light')

  const exported = theme.buildThemeLibraryBundle([
    { name: 'Ocean', definition: validDefinition() },
  ])
  assert.equal(exported.kind, 'hephaestus.theme.library')
  assert.equal(exported.themes.length, 1)
})

test('maps built-in themes to their base mode', () => {
  assert.equal(theme.getAppThemeBase('flame'), 'dark')
  assert.equal(theme.getAppThemeBase('graphite'), 'dark')
  assert.equal(theme.getAppThemeBase('paper-ink'), 'light')
  assert.equal(theme.getAppThemeBase('glass'), 'light')
  assert.equal(theme.getAppThemeBase('unknown'), 'light')
})

test('defaults status seeds and applies advanced token overrides', () => {
  const definition = theme.parseThemeDefinition(validDefinition())
  assert.equal(definition.colors.success, '#34d399')
  assert.equal(definition.colors.warning, '#fbbf24')
  assert.equal(definition.colors.info, '#60a5fa')

  const withStatus = theme.parseThemeDefinition({
    ...validDefinition(),
    colors: { ...validDefinition().colors, success: '#00ff88', warning: '#ffcc00', info: '#22aaff' },
  })
  const vars = theme.themeToCssVars(withStatus)
  assert.equal(vars['--success'], '#00ff88')
  assert.equal(vars['--tone-green'], '#00ff88')
  assert.equal(vars['--warning'], '#ffcc00')
  assert.equal(vars['--tone-amber'], '#ffcc00')

  const overridden = theme.parseThemeDefinition({
    ...validDefinition(),
    overrides: { '--card': '#123456', '--primary': '#abcdef' },
  })
  const overriddenVars = theme.themeToCssVars(overridden)
  assert.equal(overriddenVars['--card'], '#123456')
  assert.equal(overriddenVars['--primary'], '#abcdef')
  assert.equal(theme.resolveThemeTokenColor('--primary', overriddenVars), '#abcdef')
})

test('rejects invalid or radius token overrides', () => {
  for (const overrides of [
    { '--radius': '#ffffff' },
    { '--card': 'red' },
    { '--not a token': '#ffffff' },
  ]) {
    assert.throws(() => theme.parseThemeDefinition({ ...validDefinition(), overrides }))
  }
})

test('resolves derived token colors for the token inspector', () => {
  const vars = theme.themeToCssVars(theme.parseThemeDefinition(validDefinition()))
  assert.equal(theme.resolveThemeTokenColor('--background', vars), '#101114')
  assert.equal(theme.resolveThemeTokenColor('--tone-blue', vars), '#60a5fa')
  const soft = theme.resolveThemeTokenColor('--tone-blue-soft', vars)
  assert.match(soft, /^#|^rgba/)
})

test('supports alpha colors end to end', () => {
  const definition = theme.parseThemeDefinition({
    ...validDefinition(),
    colors: { ...validDefinition().colors, background: '#11223380' },
  })
  const vars = theme.themeToCssVars(definition)
  assert.equal(vars['--background'], '#11223380')
  assert.equal(theme.resolveThemeTokenColor('--background', vars), 'rgba(17, 34, 51, 0.50)')
})
