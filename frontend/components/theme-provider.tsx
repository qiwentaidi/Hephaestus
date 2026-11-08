'use client'

import { useEffect } from 'react'
import {
  ThemeProvider as NextThemesProvider,
  useTheme,
  type ThemeProviderProps,
} from 'next-themes'

function LegacyThemeMigration() {
  const { theme, setTheme } = useTheme()

  useEffect(() => {
    if (theme === 'cobalt') {
      setTheme('dark')
    }
  }, [theme, setTheme])

  return null
}

export function ThemeProvider({ children, ...props }: ThemeProviderProps) {
  return (
    <NextThemesProvider {...props}>
      <LegacyThemeMigration />
      {children}
    </NextThemesProvider>
  )
}
