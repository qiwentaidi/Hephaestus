import { Suspense } from 'react'
import FindingDetailClient from './FindingDetailClient'

export default function Page() {
  return <Suspense fallback={<div className="p-8 text-sm text-muted-foreground" role="status">Loading…</div>}><FindingDetailClient /></Suspense>
}
