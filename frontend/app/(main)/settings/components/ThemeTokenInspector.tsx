'use client'

import { useI18n } from '@/lib/i18n'
import type { CustomThemeDefinition } from '@/lib/theme/custom-theme'
import { GENERAL_TOKEN_GROUPS } from './theme-token-groups'
import { ThemeTokenSection } from './ThemeTokenSection'

interface ThemeTokenInspectorProps {
  definition: CustomThemeDefinition
  onChange: (overrides: Record<string, string>) => void
}

export function ThemeTokenInspector({ definition, onChange }: ThemeTokenInspectorProps) {
  const { t } = useI18n()
  const copy = t.theme.customThemes.editor.tokenTable

  return (
    <div className="space-y-2 border-t border-border/70 pt-3">
      <div className="text-xs font-medium text-muted-foreground">{copy.otherTitle}</div>
      {GENERAL_TOKEN_GROUPS.map((group) => (
        <ThemeTokenSection
          key={group.id}
          definition={definition}
          onChange={onChange}
          tokens={group.tokens}
          title={copy.groups[group.id]}
        />
      ))}
    </div>
  )
}
