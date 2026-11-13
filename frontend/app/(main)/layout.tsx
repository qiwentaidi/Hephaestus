import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import { PocTemplateCacheGate } from '@/components/poc-template-cache-gate'
import MainLayoutClient from './MainLayoutClient'

export default function MainLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <Suspense
      fallback={
        <div className="flex items-center justify-center h-screen">
          <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <PocTemplateCacheGate>
        <MainLayoutClient>{children}</MainLayoutClient>
      </PocTemplateCacheGate>
    </Suspense>
  )
}
