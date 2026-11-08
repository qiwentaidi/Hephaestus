import type { Metadata } from 'next'
import { Analytics } from '@vercel/analytics/next'
import { ThemeProvider } from '@/components/theme-provider'
import { CustomThemeRuntime } from '@/components/CustomThemeRuntime'
import { DesktopExternalLinkBridge } from '@/components/DesktopExternalLinkBridge'
import { DesktopStorageBridge } from '@/components/DesktopStorageBridge'
import { DesktopWindowBootstrap } from '@/components/DesktopWindowBootstrap'
import { AppToaster } from '@/components/ui/toaster'
import { I18nProvider } from '@/lib/i18n'
import { CUSTOM_THEME_BOOTSTRAP_SCRIPT } from '@/lib/theme/custom-theme-bootstrap'
import '@/styles/globals.css'

export const metadata: Metadata = {
  title: 'Hephaestus',
  description: '以希腊神话火神为原型的安全工具集，为您的系统打造坚不可摧的防护',
  icons: {
		icon: [
			{
				url: '/icon.png',
				media: '(prefers-color-scheme: light)',
			},
			{
				url: '/icon.png',
				media: '(prefers-color-scheme: dark)',
			},
			{
				url: '/icon.png',
				type: 'image/png',
			},
		],
		apple: '/icon.png',
	},
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="zh-CN" suppressHydrationWarning>
      <body className={`font-sans antialiased`}>
        <script dangerouslySetInnerHTML={{ __html: CUSTOM_THEME_BOOTSTRAP_SCRIPT }} />
        <CustomThemeRuntime />
        <DesktopStorageBridge />
        <DesktopWindowBootstrap />
        <DesktopExternalLinkBridge />
        <ThemeProvider
          attribute="class"
          defaultTheme="flame"
          themes={['flame', 'dark', 'light', 'shadcn-light', 'shadcn-dark', 'paper-ink', 'graphite', 'glass', 'custom']}
          value={{
            flame: 'flame',
            dark: 'dark',
            light: 'light',
            'shadcn-light': 'shadcn-light',
            'shadcn-dark': 'shadcn-dark',
            'paper-ink': 'paper-ink',
            graphite: 'graphite',
            glass: 'glass',
            custom: 'custom',
            cobalt: 'dark',
          }}
          enableSystem={false}
          enableColorScheme={false}
          disableTransitionOnChange
        >
          <I18nProvider>
            {children}
          </I18nProvider>
          <AppToaster />
        </ThemeProvider>
        <Analytics />
      </body>
    </html>
  )
}
