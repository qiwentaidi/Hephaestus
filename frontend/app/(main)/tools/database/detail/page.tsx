import { Suspense } from 'react'
import { Loader2 } from 'lucide-react'
import TableDetailClient from './TableDetailClient'

export default function Page() {
  return (
    <Suspense fallback={
      <div className="flex items-center justify-center h-screen">
        <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
      </div>
    }>
      <TableDetailClient />
    </Suspense>
  )
}

