import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import ApiDocTestDetailPageClient from './ApiDocTestDetailPageClient'

export default function Page() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      }
    >
      <ApiDocTestDetailPageClient />
    </Suspense>
  )
}
