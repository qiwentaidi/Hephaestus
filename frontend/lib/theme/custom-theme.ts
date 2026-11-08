import { persistDesktopConfigItems, removeDesktopConfigItems } from '@/lib/api'

export const CUSTOM_THEME_ID = 'custom'
export const CUSTOM_THEME_SCHEMA_VERSION = 1
export const CUSTOM_THEME_CACHE_KEY = 'hephaestus:appearance:custom-theme'
export const CUSTOM_THEME_CSS_KEY = 'hephaestus:appearance:custom-theme-css'
export const ACTIVE_CUSTOM_THEME_ID_KEY = 'hephaestus:appearance:active-custom-theme'
export const CUSTOM_THEME_EVENT = 'hephaestus:appearance:custom-theme-changed'

export type ThemeBase = 'dark' | 'light'

export interface CustomThemeColors {
  background: string
  foreground: string
  card: string
  primary: string
  primaryForeground: string
  muted: string
  mutedForeground: string
  border: string
  sidebar: string
  sidebarForeground: string
  success: string
  warning: string
  info: string
  destructive: string
  destructiveForeground: string
  ring: string
}

export interface CustomThemeDefinition {
  schemaVersion: 1
  base: ThemeBase
  radius: string
  colors: CustomThemeColors
  /** Optional per-token color overrides keyed by CSS variable name. */
  overrides?: Record<string, string>
}

export interface CustomThemeRecord {
  id: number
  name: string
  definition: CustomThemeDefinition
  createdAt: string
  updatedAt: string
}

export interface ThemeLibraryEntry {
  name: string
  definition: CustomThemeDefinition
}

export interface ThemeLibraryBundle {
  kind: 'hephaestus.theme.library'
  schemaVersion: 1
  exportedAt: string
  themes: ThemeLibraryEntry[]
}

export type ThemeValidationCode =
  | 'invalid_bundle'
  | 'unsupported_version'
  | 'invalid_theme'
  | 'duplicate_name'

export class ThemeValidationError extends Error {
  code: ThemeValidationCode
  conflicts: string[]
  field?: string

  constructor(code: ThemeValidationCode, message: string, options?: { conflicts?: string[]; field?: string }) {
    super(message)
    this.name = 'ThemeValidationError'
    this.code = code
    this.conflicts = options?.conflicts ?? []
    this.field = options?.field
  }
}

const HEX_COLOR_PATTERN = /^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/
const RADIUS_PATTERN = /^(?:0|[0-9]+(?:\.[0-9]+)?(?:px|rem|em))$/

const COLOR_FIELDS: Array<{ key: keyof CustomThemeColors; label: string }> = [
  { key: 'background', label: 'background' },
  { key: 'foreground', label: 'foreground' },
  { key: 'card', label: 'card' },
  { key: 'primary', label: 'primary' },
  { key: 'primaryForeground', label: 'primaryForeground' },
  { key: 'muted', label: 'muted' },
  { key: 'mutedForeground', label: 'mutedForeground' },
  { key: 'border', label: 'border' },
  { key: 'sidebar', label: 'sidebar' },
  { key: 'sidebarForeground', label: 'sidebarForeground' },
  { key: 'success', label: 'success' },
  { key: 'warning', label: 'warning' },
  { key: 'info', label: 'info' },
  { key: 'destructive', label: 'destructive' },
  { key: 'destructiveForeground', label: 'destructiveForeground' },
  { key: 'ring', label: 'ring' },
]

const DARK_DEFAULTS: CustomThemeDefinition = {
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
    success: '#34d399',
    warning: '#fbbf24',
    info: '#60a5fa',
    destructive: '#e5484d',
    destructiveForeground: '#ffffff',
    ring: '#5b8def',
  },
}

const LIGHT_DEFAULTS: CustomThemeDefinition = {
  schemaVersion: 1,
  base: 'light',
  radius: '0.5rem',
  colors: {
    background: '#fbfbfd',
    foreground: '#1c2024',
    card: '#ffffff',
    primary: '#3e63dd',
    primaryForeground: '#ffffff',
    muted: '#f3f4f7',
    mutedForeground: '#60646c',
    border: '#d9d9e0',
    sidebar: '#f6f7f9',
    sidebarForeground: '#1c2024',
    success: '#18794e',
    warning: '#946800',
    info: '#0b6bcb',
    destructive: '#e5484d',
    destructiveForeground: '#ffffff',
    ring: '#3e63dd',
  },
}

const TONE_COLORS: Record<ThemeBase, Record<string, string>> = {
  dark: {
    blue: '#60a5fa',
    green: '#34d399',
    purple: '#c084fc',
    amber: '#fbbf24',
    orange: '#fb923c',
    red: '#f87171',
    slate: '#94a3b8',
  },
  light: {
    blue: '#0b6bcb',
    green: '#18794e',
    purple: '#6e56cf',
    amber: '#946800',
    orange: '#b35900',
    red: '#cd2b31',
    slate: '#5c6670',
  },
}

const BUILT_IN_THEME_BASES: Record<string, ThemeBase> = {
  flame: 'dark',
  dark: 'dark',
  'shadcn-dark': 'dark',
  graphite: 'dark',
  light: 'light',
  'shadcn-light': 'light',
  'paper-ink': 'light',
  glass: 'light',
}

export function createDefaultThemeDefinition(base: ThemeBase = 'dark'): CustomThemeDefinition {
  return structuredClone(base === 'dark' ? DARK_DEFAULTS : LIGHT_DEFAULTS)
}

export function normalizeThemeName(raw: string): string {
  return raw.trim().replace(/\s+/g, ' ')
}

export function themeNameKey(name: string): string {
  return normalizeThemeName(name).toLowerCase()
}

export function isHexColor(value: string): boolean {
  return HEX_COLOR_PATTERN.test(value.trim())
}

export function isValidRadius(value: string): boolean {
  return value.trim().length <= 24 && RADIUS_PATTERN.test(value.trim())
}

function relativeLuminance(color: string): number | null {
  const hex = color.trim().replace('#', '')
  const expanded =
    hex.length === 3
      ? hex
          .split('')
          .map((character) => character + character)
          .join('')
      : hex.slice(0, 6)
  if (expanded.length !== 6) return null
  const channels = [0, 2, 4].map(
    (offset) => Number.parseInt(expanded.slice(offset, offset + 2), 16) / 255,
  )
  if (channels.some((channel) => !Number.isFinite(channel))) return null
  const linear = (channel: number) =>
    channel <= 0.03928 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
  return 0.2126 * linear(channels[0]) + 0.7152 * linear(channels[1]) + 0.0722 * linear(channels[2])
}

export function contrastForeground(color: string): string {
  const luminance = relativeLuminance(color)
  if (luminance === null) return '#ffffff'
  const darkLuminance = relativeLuminance('#101114') ?? 0.005
  const whiteContrast = 1.05 / (luminance + 0.05)
  const darkContrast = (luminance + 0.05) / (darkLuminance + 0.05)
  return whiteContrast >= darkContrast ? '#ffffff' : '#101114'
}

export function parseThemeDefinition(value: unknown): CustomThemeDefinition {
  if (!value || typeof value !== 'object') {
    throw new ThemeValidationError('invalid_theme', 'theme definition must be an object')
  }
  const source = value as Record<string, unknown>
  const schemaVersion = source.schemaVersion ?? CUSTOM_THEME_SCHEMA_VERSION
  if (schemaVersion !== CUSTOM_THEME_SCHEMA_VERSION) {
    throw new ThemeValidationError('unsupported_version', 'unsupported theme schema version')
  }

  const base = String(source.base ?? '').toLowerCase()
  if (base !== 'dark' && base !== 'light') {
    throw new ThemeValidationError('invalid_theme', 'theme base must be dark or light')
  }

  const radius = String(source.radius ?? '').trim() || '0.5rem'
  if (radius.length > 24 || !RADIUS_PATTERN.test(radius)) {
    throw new ThemeValidationError('invalid_theme', 'invalid theme radius')
  }

  const colorsSource = source.colors
  if (!colorsSource || typeof colorsSource !== 'object') {
    throw new ThemeValidationError('invalid_theme', 'theme colors are required')
  }

  const baseDefaults = base === 'dark' ? DARK_DEFAULTS.colors : LIGHT_DEFAULTS.colors
  const statusDefaults: Partial<Record<keyof CustomThemeColors, string>> = {
    success: baseDefaults.success,
    warning: baseDefaults.warning,
    info: baseDefaults.info,
  }

  const colors = {} as CustomThemeColors
  for (const { key, label } of COLOR_FIELDS) {
    const rawValue =
      String((colorsSource as Record<string, unknown>)[key] ?? '').trim().toLowerCase() ||
      statusDefaults[key] ||
      ''
    if (!HEX_COLOR_PATTERN.test(rawValue)) {
      throw new ThemeValidationError('invalid_theme', `invalid color for ${label}`, { field: label })
    }
    colors[key] = rawValue
  }

  let overrides: Record<string, string> | undefined
  if (source.overrides !== undefined) {
    if (!source.overrides || typeof source.overrides !== 'object' || Array.isArray(source.overrides)) {
      throw new ThemeValidationError('invalid_theme', 'theme overrides must be an object')
    }
    const entries = Object.entries(source.overrides as Record<string, unknown>)
    if (entries.length > 160) {
      throw new ThemeValidationError('invalid_theme', 'too many theme overrides')
    }
    const normalized: Record<string, string> = {}
    for (const [key, value] of entries) {
      const token = key.trim()
      if (!/^--[a-z0-9-]+$/.test(token) || token.length > 64 || token === '--radius') {
        throw new ThemeValidationError('invalid_theme', `invalid token override ${key}`)
      }
      const color = String(value ?? '').trim().toLowerCase()
      if (!HEX_COLOR_PATTERN.test(color)) {
        throw new ThemeValidationError('invalid_theme', `invalid override color for ${key}`)
      }
      normalized[token] = color
    }
    if (Object.keys(normalized).length > 0) overrides = normalized
  }

  return {
    schemaVersion: CUSTOM_THEME_SCHEMA_VERSION,
    base,
    radius,
    colors,
    ...(overrides ? { overrides } : {}),
  }
}

export function themeToCssVars(definition: CustomThemeDefinition): Record<string, string> {
  const { colors, base } = definition
  const tone = {
    ...TONE_COLORS[base],
    blue: colors.info,
    green: colors.success,
    amber: colors.warning,
    red: colors.destructive,
  }

  const vars: Record<string, string> = {
    '--background': colors.background,
    '--foreground': colors.foreground,
    '--card': colors.card,
    '--card-foreground': colors.foreground,
    '--popover': colors.card,
    '--popover-foreground': colors.foreground,
    '--primary': colors.primary,
    '--primary-foreground': colors.primaryForeground,
    '--secondary': 'color-mix(in srgb, var(--muted) 88%, var(--foreground) 12%)',
    '--secondary-foreground': colors.foreground,
    '--muted': colors.muted,
    '--muted-foreground': colors.mutedForeground,
    '--accent': 'color-mix(in srgb, var(--muted) 84%, var(--foreground) 16%)',
    '--accent-foreground': colors.foreground,
    '--destructive': colors.destructive,
    '--destructive-foreground': colors.destructiveForeground,
    '--border': colors.border,
    '--input': colors.border,
    '--input-panel-bg': colors.card,
    '--input-panel-bg-hover': 'color-mix(in srgb, var(--card) 92%, var(--foreground) 8%)',
    '--input-panel-border': colors.border,
    '--input-panel-action-bg': 'color-mix(in srgb, var(--foreground) 5%, transparent)',
    '--input-panel-action-bg-hover': 'color-mix(in srgb, var(--foreground) 9%, transparent)',
    '--input-panel-action-border': 'color-mix(in srgb, var(--foreground) 7%, transparent)',
    '--ring': colors.ring,
    '--radius': definition.radius,
    '--sidebar': colors.sidebar,
    '--sidebar-foreground': colors.sidebarForeground,
    '--sidebar-primary': colors.primary,
    '--sidebar-primary-foreground': colors.primaryForeground,
    '--sidebar-accent': 'color-mix(in srgb, var(--sidebar) 84%, var(--foreground) 16%)',
    '--sidebar-accent-foreground': colors.sidebarForeground,
    '--sidebar-border': colors.border,
    '--sidebar-ring': colors.ring,
    '--sidebar-hover': 'color-mix(in srgb, var(--sidebar) 90%, var(--foreground) 10%)',
    '--success': colors.success,
    '--success-foreground': contrastForeground(colors.success),
    '--info': colors.info,
    '--info-foreground': contrastForeground(colors.info),
    '--warning': colors.warning,
    '--warning-foreground': contrastForeground(colors.warning),
    '--error': colors.destructive,
    '--error-foreground': contrastForeground(colors.destructive),
  }

  for (const [name, color] of Object.entries(tone)) {
    vars[`--tone-${name}`] = color
    vars[`--tone-${name}-soft`] = `color-mix(in srgb, var(--tone-${name}) 14%, var(--background))`
    vars[`--tone-${name}-border`] = `color-mix(in srgb, var(--tone-${name}) 34%, var(--background))`
    vars[`--tone-${name}-icon-bg`] = `color-mix(in srgb, var(--tone-${name}) 18%, transparent)`
    vars[`--tone-${name}-icon-border`] = `color-mix(in srgb, var(--tone-${name}) 26%, transparent)`
  }

  for (const [name, value] of Object.entries(definition.overrides ?? {})) {
    if (name === '--radius' || !(name in vars) || !HEX_COLOR_PATTERN.test(value)) continue
    vars[name] = value.toLowerCase()
  }

  return vars
}

export function buildThemeCss(definition: CustomThemeDefinition, selector = `html.${CUSTOM_THEME_ID}`): string {
  const vars = themeToCssVars(definition)
  const declarations = Object.entries(vars)
    .map(([name, value]) => `  ${name}: ${value};`)
    .join('\n')
  return `${selector} {\n  color-scheme: ${definition.base};\n${declarations}\n}\n`
}

type Rgba = [number, number, number, number]

function parseHexChannels(value: string): Rgba | null {
  const hex = value.trim().replace('#', '')
  if (![3, 4, 6, 8].includes(hex.length)) return null
  const expanded =
    hex.length === 3 || hex.length === 4
      ? hex
          .split('')
          .map((character) => character + character)
          .join('')
      : hex
  const channels = [0, 2, 4, 6].map((offset) =>
    offset < expanded.length ? Number.parseInt(expanded.slice(offset, offset + 2), 16) : 255,
  )
  if (channels.some((channel) => !Number.isFinite(channel))) return null
  return [channels[0], channels[1], channels[2], channels[3] / 255]
}

function resolveColorValue(value: string, vars: Record<string, string>, depth: number): Rgba | null {
  const normalized = value.trim()
  if (!normalized || depth > 12) return null
  if (normalized === 'transparent') return [0, 0, 0, 0]
  if (normalized.startsWith('#')) return parseHexChannels(normalized)

  const variableMatch = /^var\((--[a-z0-9-]+)\)$/.exec(normalized)
  if (variableMatch) {
    const next = vars[variableMatch[1]]
    return next ? resolveColorValue(next, vars, depth + 1) : null
  }

  const mixMatch = /^color-mix\(in srgb,\s*(.+?)\s+([0-9.]+)%,\s*(.+?)(?:\s+([0-9.]+)%)?\)$/.exec(
    normalized,
  )
  if (!mixMatch) return null
  const first = resolveColorValue(mixMatch[1], vars, depth + 1)
  const second = resolveColorValue(mixMatch[3], vars, depth + 1)
  if (!first || !second) return null

  let firstShare = Number.parseFloat(mixMatch[2])
  let secondShare = mixMatch[4] ? Number.parseFloat(mixMatch[4]) : 100 - firstShare
  if (!Number.isFinite(firstShare) || !Number.isFinite(secondShare)) return null
  const total = firstShare + secondShare
  if (total <= 0) return null
  firstShare /= total
  secondShare /= total

  return [
    first[0] * firstShare + second[0] * secondShare,
    first[1] * firstShare + second[1] * secondShare,
    first[2] * firstShare + second[2] * secondShare,
    first[3] * firstShare + second[3] * secondShare,
  ]
}

/** Resolve a generated CSS variable to a concrete color for display in the token table. */
export function resolveThemeTokenColor(
  name: string,
  vars: Record<string, string>,
  depth = 0,
): string {
  const rgba = resolveColorValue(vars[name] ?? '', vars, depth)
  if (!rgba) return ''
  const [red, green, blue, alpha] = rgba
  if (alpha >= 0.999) {
    const toHex = (channel: number) =>
      Math.max(0, Math.min(255, Math.round(channel))).toString(16).padStart(2, '0')
    return `#${toHex(red)}${toHex(green)}${toHex(blue)}`
  }
  return `rgba(${Math.round(red)}, ${Math.round(green)}, ${Math.round(blue)}, ${alpha.toFixed(2)})`
}

export function themeStyleElement(): HTMLStyleElement | null {
  if (typeof document === 'undefined') return null
  let element = document.getElementById('hephaestus-custom-theme') as HTMLStyleElement | null
  if (!element) {
    element = document.createElement('style')
    element.id = 'hephaestus-custom-theme'
    document.head.appendChild(element)
  }
  return element
}

export function getCachedCustomTheme(): CustomThemeRecord | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(CUSTOM_THEME_CACHE_KEY)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as Partial<CustomThemeRecord>
    if (typeof parsed.id !== 'number' || typeof parsed.name !== 'string' || !parsed.definition) {
      return null
    }
    return {
      id: parsed.id,
      name: parsed.name,
      definition: parseThemeDefinition(parsed.definition),
      createdAt: parsed.createdAt ?? '',
      updatedAt: parsed.updatedAt ?? '',
    }
  } catch {
    return null
  }
}

export function getActiveCustomThemeId(): number | null {
  if (typeof window === 'undefined') return null
  const raw = window.localStorage.getItem(ACTIVE_CUSTOM_THEME_ID_KEY)
  if (!raw) return null
  const parsed = Number.parseInt(raw, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

export function applyCustomTheme(record: CustomThemeRecord): void {
  if (typeof window === 'undefined') return
  const css = buildThemeCss(record.definition)
  const style = themeStyleElement()
  if (style) style.textContent = css

  window.localStorage.setItem(CUSTOM_THEME_CACHE_KEY, JSON.stringify(record))
  window.localStorage.setItem(CUSTOM_THEME_CSS_KEY, css)
  window.localStorage.setItem(ACTIVE_CUSTOM_THEME_ID_KEY, String(record.id))
  document.documentElement.dataset.customThemeBase = record.definition.base

  void persistDesktopConfigItems({
    theme: CUSTOM_THEME_ID,
    [CUSTOM_THEME_CACHE_KEY]: JSON.stringify(record),
    [ACTIVE_CUSTOM_THEME_ID_KEY]: String(record.id),
  })

  window.dispatchEvent(new CustomEvent(CUSTOM_THEME_EVENT, { detail: { theme: record } }))
}

export function applyCachedCustomTheme(): void {
  if (typeof window === 'undefined') return
  const theme = getCachedCustomTheme()
  if (!theme) {
    removeCustomThemeStyle()
    return
  }
  const style = themeStyleElement()
  if (style) style.textContent = buildThemeCss(theme.definition)
  document.documentElement.dataset.customThemeBase = theme.definition.base
}

export function removeCustomThemeStyle(): void {
  if (typeof document === 'undefined') return
  document.getElementById('hephaestus-custom-theme')?.remove()
  delete document.documentElement.dataset.customThemeBase
}

export function clearCustomThemeCache(): void {
  if (typeof window === 'undefined') return
  window.localStorage.removeItem(CUSTOM_THEME_CACHE_KEY)
  window.localStorage.removeItem(CUSTOM_THEME_CSS_KEY)
  window.localStorage.removeItem(ACTIVE_CUSTOM_THEME_ID_KEY)
  removeCustomThemeStyle()
  void removeDesktopConfigItems([
    CUSTOM_THEME_CACHE_KEY,
    CUSTOM_THEME_CSS_KEY,
    ACTIVE_CUSTOM_THEME_ID_KEY,
  ])
  window.dispatchEvent(new CustomEvent(CUSTOM_THEME_EVENT, { detail: { theme: null } }))
}

export function getAppThemeBase(themeName?: string | null): ThemeBase {
  if (!themeName) return 'light'
  if (themeName === CUSTOM_THEME_ID) {
    return getCachedCustomTheme()?.definition.base ?? 'dark'
  }
  return BUILT_IN_THEME_BASES[themeName] ?? 'light'
}

export function parseThemeLibraryBundle(value: unknown): ThemeLibraryBundle {
  if (!value || typeof value !== 'object') {
    throw new ThemeValidationError('invalid_bundle', 'theme bundle must be an object')
  }

  const source = value as Record<string, unknown>
  if (source.kind !== 'hephaestus.theme.library') {
    throw new ThemeValidationError('invalid_bundle', 'unsupported theme bundle')
  }
  if (source.schemaVersion !== CUSTOM_THEME_SCHEMA_VERSION) {
    throw new ThemeValidationError('unsupported_version', 'unsupported theme bundle version')
  }
  if (!Array.isArray(source.themes) || source.themes.length === 0) {
    throw new ThemeValidationError('invalid_bundle', 'theme bundle is empty')
  }

  const themes: ThemeLibraryEntry[] = []
  const seen = new Map<string, string>()
  const conflicts: string[] = []

  for (const item of source.themes) {
    if (!item || typeof item !== 'object') {
      throw new ThemeValidationError('invalid_theme', 'theme entry must be an object')
    }
    const entry = item as Record<string, unknown>
    const name = normalizeThemeName(String(entry.name ?? ''))
    if (!name || name.length > 60) {
      throw new ThemeValidationError('invalid_theme', 'invalid theme name')
    }
    const key = themeNameKey(name)
    if (seen.has(key)) {
      conflicts.push(name)
      continue
    }
    seen.set(key, name)
    themes.push({
      name,
      definition: parseThemeDefinition(entry.definition),
    })
  }

  if (conflicts.length > 0) {
    throw new ThemeValidationError('duplicate_name', 'duplicate theme names in bundle', { conflicts })
  }

  return {
    kind: 'hephaestus.theme.library',
    schemaVersion: CUSTOM_THEME_SCHEMA_VERSION,
    exportedAt: typeof source.exportedAt === 'string' ? source.exportedAt : new Date().toISOString(),
    themes,
  }
}

export function buildThemeLibraryBundle(
  records: Array<{ name: string; definition: CustomThemeDefinition }>,
): ThemeLibraryBundle {
  return {
    kind: 'hephaestus.theme.library',
    schemaVersion: CUSTOM_THEME_SCHEMA_VERSION,
    exportedAt: new Date().toISOString(),
    themes: records.map((record) => ({
      name: record.name,
      definition: record.definition,
    })),
  }
}
