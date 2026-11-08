'use client'

import { useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

export const CHECKERBOARD_BACKGROUND =
  'linear-gradient(45deg, rgb(128 128 128 / 0.28) 25%, transparent 25%), linear-gradient(-45deg, rgb(128 128 128 / 0.28) 25%, transparent 25%), linear-gradient(45deg, transparent 75%, rgb(128 128 128 / 0.28) 75%), linear-gradient(-45deg, transparent 75%, rgb(128 128 128 / 0.28) 75%)'

const PRESET_COLORS = [
  '#722ed1',
  '#13c2c2',
  '#52c41a',
  '#eb2f96',
  '#f5222d',
  '#fa8c16',
  '#fadb14',
  '#1677ff',
  '#faad14',
  '#a0d911',
  '#000000',
]

type ColorFormat = 'hex' | 'hsb' | 'rgb'

function expandHex(value: string): string | null {
  const raw = value.trim().replace('#', '').toLowerCase()
  if (!/^[0-9a-f]+$/.test(raw)) return null
  if (raw.length === 3 || raw.length === 4) {
    return raw
      .split('')
      .map((character) => character + character)
      .join('')
  }
  if (raw.length === 6 || raw.length === 8) return raw
  return null
}

function rgbaToHex8(value: string): string | null {
  const rgba = /^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)(?:\s*,\s*([0-9.]+))?\s*\)$/.exec(
    value.trim(),
  )
  if (!rgba) return null
  const channels = [rgba[1], rgba[2], rgba[3]].map((channel) =>
    Math.max(0, Math.min(255, Number.parseInt(channel, 10))).toString(16).padStart(2, '0'),
  )
  const alpha = rgba[4] === undefined ? 1 : Math.max(0, Math.min(1, Number.parseFloat(rgba[4])))
  return `#${channels.join('')}${Math.round(alpha * 255)
    .toString(16)
    .padStart(2, '0')}`
}

/** Accepts hex #RGB/#RGBA/#RRGGBB/#RRGGBBAA or rgba() and returns #RRGGBBAA. */
export function normalizeToHex8(value: string): string {
  const trimmed = value.trim().toLowerCase()
  if (trimmed === 'transparent') return '#00000000'
  if (trimmed.startsWith('#')) {
    const expanded = expandHex(trimmed)
    if (!expanded) return '#000000ff'
    if (expanded.length === 6) return `#${expanded}ff`
    return `#${expanded}`
  }
  return rgbaToHex8(trimmed) ?? '#000000ff'
}

export function splitHexAlpha(value: string): { rgb: string; alpha: number } {
  const hex8 = normalizeToHex8(value)
  return {
    rgb: `#${hex8.slice(1, 7)}`,
    alpha: Number.parseInt(hex8.slice(7, 9), 16),
  }
}

function splitRgba(value: string): { red: number; green: number; blue: number; alpha: number } {
  const hex8 = normalizeToHex8(value)
  return {
    red: Number.parseInt(hex8.slice(1, 3), 16),
    green: Number.parseInt(hex8.slice(3, 5), 16),
    blue: Number.parseInt(hex8.slice(5, 7), 16),
    alpha: Number.parseInt(hex8.slice(7, 9), 16),
  }
}

function toHex8(red: number, green: number, blue: number, alpha: number): string {
  const channel = (value: number) =>
    Math.max(0, Math.min(255, Math.round(value))).toString(16).padStart(2, '0')
  const base = `#${channel(red)}${channel(green)}${channel(blue)}`
  return alpha >= 255 ? base : `${base}${channel(alpha)}`
}

function rgbToHsv(red: number, green: number, blue: number): { h: number; s: number; v: number } {
  const r = red / 255
  const g = green / 255
  const b = blue / 255
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const delta = max - min

  let hue = 0
  if (delta !== 0) {
    if (max === r) hue = ((g - b) / delta) % 6
    else if (max === g) hue = (b - r) / delta + 2
    else hue = (r - g) / delta + 4
  }

  return {
    h: (hue * 60 + 360) % 360,
    s: max === 0 ? 0 : (delta / max) * 100,
    v: max * 100,
  }
}

function hsvToRgb(hue: number, saturation: number, value: number): { r: number; g: number; b: number } {
  const h = ((hue % 360) + 360) % 360
  const s = saturation / 100
  const v = value / 100
  const c = v * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = v - c

  let r = 0
  let g = 0
  let b = 0
  if (h < 60) [r, g, b] = [c, x, 0]
  else if (h < 120) [r, g, b] = [x, c, 0]
  else if (h < 180) [r, g, b] = [0, c, x]
  else if (h < 240) [r, g, b] = [0, x, c]
  else if (h < 300) [r, g, b] = [x, 0, c]
  else [r, g, b] = [c, 0, x]

  return {
    r: (r + m) * 255,
    g: (g + m) * 255,
    b: (b + m) * 255,
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value))
}

function formatColor(value: string, format: ColorFormat): string {
  const { red, green, blue, alpha } = splitRgba(value)
  if (format === 'rgb') return `${red}, ${green}, ${blue}`
  if (format === 'hsb') {
    const { h, s, v } = rgbToHsv(red, green, blue)
    return `${Math.round(h)}, ${Math.round(s)}, ${Math.round(v)}`
  }
  const base = `#${[red, green, blue]
    .map((channel) => channel.toString(16).padStart(2, '0'))
    .join('')}`
  return alpha >= 255 ? base : `${base}${alpha.toString(16).padStart(2, '0')}`
}

function parseFormatted(input: string, format: ColorFormat, currentAlpha: number): string | null {
  const trimmed = input.trim()
  if (format === 'hex') {
    if (!/^#?[0-9a-f]{3,8}$/i.test(trimmed)) return null
    const raw = trimmed.replace('#', '')
    const normalized = normalizeToHex8(trimmed)
    const { red, green, blue, alpha } = splitRgba(normalized)
    return toHex8(red, green, blue, raw.length === 6 || raw.length === 3 ? currentAlpha : alpha)
  }

  const channels = trimmed.match(/\d+(?:\.\d+)?/g)?.map(Number)
  if (!channels || channels.length < 3) return null
  if (format === 'rgb') {
    return toHex8(
      clamp(channels[0], 0, 255),
      clamp(channels[1], 0, 255),
      clamp(channels[2], 0, 255),
      currentAlpha,
    )
  }
  const { r, g, b } = hsvToRgb(
    clamp(channels[0], 0, 360),
    clamp(channels[1], 0, 100),
    clamp(channels[2], 0, 100),
  )
  return toHex8(r, g, b, currentAlpha)
}

interface ThemeColorPickerProps {
  value: string
  onChange: (value: string) => void
  ariaLabel: string
  invalid?: boolean
  compact?: boolean
  triggerOnly?: boolean
  className?: string
}

export function ThemeColorPicker({
  value,
  onChange,
  ariaLabel,
  invalid = false,
  compact = false,
  triggerOnly = false,
  className,
}: ThemeColorPickerProps) {
  const { t } = useI18n()
  const { red, green, blue, alpha } = splitRgba(value)
  const { h, s, v } = rgbToHsv(red, green, blue)
  const alphaPercent = Math.round((alpha / 255) * 100)
  const displayValue = /^rgba?\(/i.test(value.trim()) ? normalizeToHex8(value) : value
  const rgbColor = toHex8(red, green, blue, 255)
  const areaRef = useRef<HTMLDivElement>(null)
  const hueRef = useRef<HTMLDivElement>(null)
  const alphaRef = useRef<HTMLDivElement>(null)
  const [format, setFormat] = useState<ColorFormat>('hex')
  const [formatOpen, setFormatOpen] = useState(false)
  const formatted = formatColor(value, format)
  const [draft, setDraft] = useState<string | null>(null)

  const updateSaturationValue = (clientX: number, clientY: number) => {
    const rect = areaRef.current?.getBoundingClientRect()
    if (!rect) return
    const nextS = clamp(((clientX - rect.left) / rect.width) * 100, 0, 100)
    const nextV = clamp((1 - (clientY - rect.top) / rect.height) * 100, 0, 100)
    const color = hsvToRgb(h, nextS, nextV)
    onChange(toHex8(color.r, color.g, color.b, alpha))
  }

  const updateHue = (clientX: number) => {
    const rect = hueRef.current?.getBoundingClientRect()
    if (!rect) return
    const nextHue = clamp(((clientX - rect.left) / rect.width) * 360, 0, 360)
    const color = hsvToRgb(nextHue, s, v)
    onChange(toHex8(color.r, color.g, color.b, alpha))
  }

  const updateAlpha = (clientX: number) => {
    const rect = alphaRef.current?.getBoundingClientRect()
    if (!rect) return
    const nextAlpha = Math.round(clamp((clientX - rect.left) / rect.width, 0, 1) * 255)
    onChange(toHex8(red, green, blue, nextAlpha))
  }

  return (
    <div className={cn(triggerOnly ? 'inline-flex' : 'space-y-1.5', compact && !triggerOnly && 'space-y-1', className)}>
      <div className={cn('flex items-center', !triggerOnly && 'gap-2')}>
        <Popover>
          <PopoverTrigger asChild>
            <button
              type="button"
              aria-label={`${ariaLabel} ${t.theme.customThemes.editor.palette}`}
              className={cn(
                triggerOnly ? 'relative h-7 w-7 overflow-hidden rounded-md border border-input' : 'relative h-9 w-10 shrink-0 overflow-hidden rounded-md border border-input',
                invalid && 'border-destructive',
              )}
              style={{
                backgroundImage: CHECKERBOARD_BACKGROUND,
                backgroundSize: '8px 8px',
                backgroundPosition: '0 0, 0 4px, 4px -4px, -4px 0',
              }}
            >
              <span className="absolute inset-0" style={{ backgroundColor: normalizeToHex8(value) }} />
            </button>
          </PopoverTrigger>
          <PopoverContent className="w-[320px] rounded-xl p-3.5" align="start">
            <div
              ref={areaRef}
              role="slider"
              tabIndex={0}
              aria-label={`${ariaLabel} ${t.theme.customThemes.editor.saturationValue}`}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(s)}
              className="relative h-40 w-full cursor-crosshair touch-none rounded-lg border border-border/60"
              style={{
                background: `linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, hsl(${h} 100% 50%))`,
              }}
              onPointerDown={(event) => {
                event.currentTarget.setPointerCapture(event.pointerId)
                updateSaturationValue(event.clientX, event.clientY)
              }}
              onPointerMove={(event) => {
                if (event.currentTarget.hasPointerCapture(event.pointerId)) {
                  updateSaturationValue(event.clientX, event.clientY)
                }
              }}
              onPointerUp={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
              onPointerCancel={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
              onKeyDown={(event) => {
                const step = event.shiftKey ? 10 : 1
                let nextS = s
                let nextV = v
                if (event.key === 'ArrowLeft') nextS = clamp(s - step, 0, 100)
                else if (event.key === 'ArrowRight') nextS = clamp(s + step, 0, 100)
                else if (event.key === 'ArrowUp') nextV = clamp(v + step, 0, 100)
                else if (event.key === 'ArrowDown') nextV = clamp(v - step, 0, 100)
                else return
                event.preventDefault()
                const color = hsvToRgb(h, nextS, nextV)
                onChange(toHex8(color.r, color.g, color.b, alpha))
              }}
            >
              <span
                className="pointer-events-none absolute h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgb(0_0_0_/_0.25)]"
                style={{ left: `${s}%`, top: `${100 - v}%` }}
              />
            </div>

            <div className="mt-3 space-y-2.5">
              <div
                ref={hueRef}
                role="slider"
                tabIndex={0}
                aria-label={`${ariaLabel} hue`}
                aria-valuemin={0}
                aria-valuemax={360}
                aria-valuenow={Math.round(h)}
                className="relative h-3.5 w-full touch-none rounded-full"
                style={{
                  background:
                    'linear-gradient(to right, #ff0000, #ffff00, #00ff00, #00ffff, #0000ff, #ff00ff, #ff0000)',
                }}
                onPointerDown={(event) => {
                  event.currentTarget.setPointerCapture(event.pointerId)
                  updateHue(event.clientX)
                }}
                onPointerMove={(event) => {
                  if (event.currentTarget.hasPointerCapture(event.pointerId)) {
                    updateHue(event.clientX)
                  }
                }}
                onPointerUp={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
                onPointerCancel={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
                onKeyDown={(event) => {
                  const step = event.shiftKey ? 10 : 1
                  let nextHue = h
                  if (event.key === 'ArrowLeft') nextHue = (h - step + 360) % 360
                  else if (event.key === 'ArrowRight') nextHue = (h + step) % 360
                  else return
                  event.preventDefault()
                  const color = hsvToRgb(nextHue, s, v)
                  onChange(toHex8(color.r, color.g, color.b, alpha))
                }}
              >
                <span
                  className="pointer-events-none absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgb(0_0_0_/_0.25)]"
                  style={{
                    left: `${(h / 360) * 100}%`,
                    backgroundColor: `hsl(${h} 100% 50%)`,
                  }}
                />
              </div>

              <div className="flex items-center gap-2">
                <div
                  ref={alphaRef}
                  role="slider"
                  tabIndex={0}
                  aria-label={`${ariaLabel} ${t.theme.customThemes.editor.alpha}`}
                  aria-valuemin={0}
                  aria-valuemax={255}
                  aria-valuenow={alpha}
                  className="relative h-3.5 min-w-0 flex-1 touch-none rounded-full border border-border/60"
                  style={{
                    backgroundImage: `linear-gradient(to right, transparent, ${rgbColor}), ${CHECKERBOARD_BACKGROUND}`,
                    backgroundSize: 'auto, 8px 8px',
                    backgroundPosition: '0 0, 0 4px, 4px -4px, -4px 0',
                  }}
                  onPointerDown={(event) => {
                    event.currentTarget.setPointerCapture(event.pointerId)
                    updateAlpha(event.clientX)
                  }}
                  onPointerMove={(event) => {
                    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
                      updateAlpha(event.clientX)
                    }
                  }}
                  onPointerUp={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
                  onPointerCancel={(event) => event.currentTarget.releasePointerCapture(event.pointerId)}
                  onKeyDown={(event) => {
                    const step = event.shiftKey ? 10 : 1
                    let nextAlpha = alpha
                    if (event.key === 'ArrowLeft') nextAlpha = clamp(alpha - step, 0, 255)
                    else if (event.key === 'ArrowRight') nextAlpha = clamp(alpha + step, 0, 255)
                    else return
                    event.preventDefault()
                    onChange(toHex8(red, green, blue, nextAlpha))
                  }}
                >
                  <span
                    className="pointer-events-none absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow-[0_0_0_1px_rgb(0_0_0_/_0.25)]"
                    style={{ left: `${(alpha / 255) * 100}%` }}
                  />
                </div>
                <div
                  className="relative h-8 w-8 shrink-0 overflow-hidden rounded-md border border-border/60"
                  style={{
                    backgroundImage: CHECKERBOARD_BACKGROUND,
                    backgroundSize: '8px 8px',
                    backgroundPosition: '0 0, 0 4px, 4px -4px, -4px 0',
                  }}
                  aria-hidden="true"
                >
                  <span className="absolute inset-0" style={{ backgroundColor: normalizeToHex8(value) }} />
                </div>
              </div>

              <div
                className="flex items-center gap-1.5"
                onBlur={(event) => {
                  if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
                    setFormatOpen(false)
                  }
                }}
              >
                <div className="relative shrink-0">
                  <button
                    type="button"
                    aria-haspopup="listbox"
                    aria-expanded={formatOpen}
                    onClick={() => setFormatOpen((previous) => !previous)}
                    className="flex h-8 w-[74px] items-center justify-between rounded-md border border-input bg-background px-2 text-xs font-medium text-foreground"
                  >
                    {format.toUpperCase()}
                    <ChevronDown className="h-3.5 w-3.5 opacity-60" />
                  </button>
                  {formatOpen && (
                    <div className="absolute left-0 top-full z-50 mt-1 w-full rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md">
                      {(['hex', 'hsb', 'rgb'] as const).map((option) => (
                        <button
                          key={option}
                          type="button"
                          role="option"
                          aria-selected={format === option}
                          onClick={() => {
                            setFormat(option)
                            setDraft(null)
                            setFormatOpen(false)
                          }}
                          className={cn(
                            'flex w-full items-center rounded px-2 py-1 text-left text-xs',
                            format === option
                              ? 'bg-accent text-accent-foreground'
                              : 'hover:bg-accent hover:text-accent-foreground',
                          )}
                        >
                          {option.toUpperCase()}
                        </button>
                      ))}
                    </div>
                  )}
                </div>
                <input
                  value={draft ?? formatted}
                  spellCheck={false}
                  aria-label={ariaLabel}
                  onChange={(event) => {
                    const next = event.target.value
                    setDraft(next)
                    const parsed = parseFormatted(next, format, alpha)
                    if (parsed) onChange(parsed)
                  }}
                  onBlur={() => setDraft(null)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') {
                      const parsed = parseFormatted(draft ?? formatted, format, alpha)
                      if (parsed) onChange(parsed)
                      setDraft(null)
                    }
                  }}
                  className="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 font-mono text-xs text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40"
                />
                <label className="relative w-16 shrink-0">
                  <input
                    type="number"
                    min={0}
                    max={100}
                    value={alphaPercent}
                    aria-label={`${ariaLabel} ${t.theme.customThemes.editor.alpha}`}
                    onChange={(event) => {
                      const next = clamp(Number.parseInt(event.target.value, 10) || 0, 0, 100)
                      onChange(toHex8(red, green, blue, Math.round((next / 100) * 255)))
                    }}
                    className="h-8 w-full rounded-md border border-input bg-background pl-2 pr-5 text-right font-mono text-xs text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40"
                  />
                  <span className="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2 text-[10px] text-muted-foreground">
                    %
                  </span>
                </label>
              </div>

              <div>
                <div className="mb-1.5 text-[10px] text-muted-foreground">
                  {t.theme.customThemes.editor.presets}
                </div>
                <div className="grid grid-cols-7 gap-1.5">
                  {PRESET_COLORS.map((preset) => {
                    const { red: presetRed, green: presetGreen, blue: presetBlue } = splitRgba(preset)
                    const selected = normalizeToHex8(preset).slice(0, 7) === rgbColor
                    return (
                      <button
                        key={preset}
                        type="button"
                        aria-label={preset}
                        aria-pressed={selected}
                        onClick={() => onChange(toHex8(presetRed, presetGreen, presetBlue, alpha))}
                        className={cn(
                          'h-6 w-full rounded border border-border/60 transition-transform hover:scale-105',
                          selected && 'ring-2 ring-ring ring-offset-1 ring-offset-popover',
                        )}
                        style={{ backgroundColor: preset }}
                      />
                    )
                  })}
                </div>
              </div>
            </div>
          </PopoverContent>
        </Popover>

        {!triggerOnly && (
        <div className="relative min-w-0 flex-1">
          <Input
            value={displayValue}
            spellCheck={false}
            aria-label={ariaLabel}
            aria-invalid={invalid}
            maxLength={9}
            onChange={(event) => onChange(event.target.value)}
            className={cn('h-9 pr-9 font-mono text-xs', invalid && 'border-destructive')}
          />
          <span className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 text-[9px] text-muted-foreground">
            {alphaPercent}%
          </span>
        </div>
        )}
      </div>
    </div>
  )
}
