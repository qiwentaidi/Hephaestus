import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import ApiDocTestPageClient from './ApiDocTestPageClient'

export default function Page() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <ApiDocTestPageClient />
    </Suspense>
  )
}
