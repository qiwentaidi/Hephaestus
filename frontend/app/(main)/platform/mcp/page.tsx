'use client'

import { Suspense, useState } from 'react'
import { Loader2 } from 'lucide-react'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useI18n } from '@/lib/i18n'
import ExternalMcpPanel from './ExternalMcpPanel'
import PlatformMcpPageClient from './PlatformMcpPageClient'

export default function Page() {
  const { t } = useI18n()
  const [tab, setTab] = useState('external')
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <Tabs value={tab} onValueChange={setTab} className="h-full min-h-0 w-full gap-0 bg-background text-foreground">
        <header className="flex shrink-0 items-center gap-3 border-b border-border px-4 py-3 lg:px-5">
          <TabsList aria-label={t.sidebar.mcp}>
            <TabsTrigger value="external">{t.sidebar.mcpExternal}</TabsTrigger>
            <TabsTrigger value="platform">{t.sidebar.mcpPlatform}</TabsTrigger>
          </TabsList>
        </header>
        <TabsContent value="external" className="min-h-0 overflow-auto">
          <ExternalMcpPanel />
        </TabsContent>
        <TabsContent value="platform" className="min-h-0 overflow-auto">
          <PlatformMcpPageClient />
        </TabsContent>
      </Tabs>
    </Suspense>
  )
}
