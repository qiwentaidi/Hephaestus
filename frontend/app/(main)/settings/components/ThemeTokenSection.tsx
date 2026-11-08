'use client'

import { ChevronRight, RotateCcw } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { useI18n } from '@/lib/i18n'
import {
  resolveThemeTokenColor,
  themeToCssVars,
  type CustomThemeDefinition,
} from '@/lib/theme/custom-theme'
import { ThemeColorPicker } from './ThemeColorPicker'

interface ThemeTokenSectionProps {
  definition: CustomThemeDefinition
  onChange: (overrides: Record<string, string>) => void
  tokens: string[]
  title?: string
  defaultOpen?: boolean
  className?: string
}

interface TokenRowProps {
  token: string
  resolved: string
  override?: string
  onOverride: (color: string) => void
  labels: {
    overridden: string
  }
}

function TokenRow({ token, resolved, override, onOverride, labels }: TokenRowProps) {
  return (
    <div className="flex items-center gap-2 rounded-md px-1.5 py-1 hover:bg-muted/30">
      <code className="min-w-0 flex-1 truncate text-[10px] text-foreground/80">{token}</code>
      <span className="hidden shrink-0 font-mono text-[10px] text-muted-foreground sm:inline">
        {override || resolved || '—'}
      </span>
      {override && (
        <Badge variant="secondary" className="shrink-0 px-1 py-0 text-[9px]">
          {labels.overridden}
        </Badge>
      )}
      <ThemeColorPicker
        triggerOnly
        value={override || resolved}
        onChange={onOverride}
        ariaLabel={`${token} color`}
        className="shrink-0"
      />
    </div>
  )
}

export function ThemeTokenSection({
  definition,
  onChange,
  tokens,
  title,
  defaultOpen = false,
  className,
}: ThemeTokenSectionProps) {
  const { t } = useI18n()
  const copy = t.theme.customThemes.editor.tokenTable
  const vars = themeToCssVars(definition)
  const overrides = definition.overrides ?? {}
  const overrideCount = tokens.filter((token) => overrides[token]).length

  const setOverride = (token: string, color: string) => {
    onChange({ ...overrides, [token]: color.toLowerCase() })
  }

  const resetGroup = () => {
    const next = { ...overrides }
    for (const token of tokens) delete next[token]
    onChange(next)
  }

  return (
    <details
      open={defaultOpen || undefined}
      className={`group overflow-hidden rounded-md border border-border/70 bg-muted/10 ${className ?? ''}`}
    >
      <summary className="flex cursor-pointer list-none items-center gap-1.5 px-2.5 py-1.5 text-[11px] text-muted-foreground hover:bg-muted/30">
        <ChevronRight className="h-3 w-3 transition-transform group-open:rotate-90" />
        <span>{title || copy.mapToken}</span>
        <span className="ml-auto text-[10px]">{tokens.length}</span>
        {overrideCount > 0 && (
          <span className="text-[10px] text-primary">
            {copy.overridden} {overrideCount}
          </span>
        )}
      </summary>

      <div className="border-t border-border/60 p-1.5">
        {overrideCount > 0 && (
          <div className="mb-1 flex justify-end">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-[10px]"
              onClick={resetGroup}
            >
              <RotateCcw className="mr-1 h-3 w-3" />
              {copy.resetAll}
            </Button>
          </div>
        )}
        <div className="max-h-[280px] space-y-0.5 overflow-y-auto">
          {tokens.map((token) => (
            <TokenRow
              key={token}
              token={token}
              resolved={resolveThemeTokenColor(token, vars)}
              override={overrides[token]}
              onOverride={(color) => setOverride(token, color)}
              labels={{ overridden: copy.overridden }}
            />
          ))}
        </div>
      </div>
    </details>
  )
}
