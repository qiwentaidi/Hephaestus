import { Suspense } from 'react'
import ExplorationClient from './ExplorationClient'

export default function Page() {
  return <Suspense fallback={<div className="p-8" role="status">Loading…</div>}><ExplorationClient /></Suspense>
}
