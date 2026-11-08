'use client'

import { useTheme } from 'next-themes'
import { Toaster as Sonner, ToasterProps } from 'sonner'
import { getAppThemeBase } from '@/lib/theme/custom-theme'

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = 'system' } = useTheme()
  const sonnerTheme = theme === 'system' ? 'system' : getAppThemeBase(theme)

  return (
    <Sonner
      theme={sonnerTheme}
      className="toaster group"
      position="top-right"
      richColors
      closeButton
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
        } as React.CSSProperties
      }
      {...props}
    />
  )
}

export { Toaster }
