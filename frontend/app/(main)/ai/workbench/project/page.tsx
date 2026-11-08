import { Suspense } from 'react'
import ProjectClient from './ProjectClient'

export default function Page() {
  return <Suspense fallback={<p className="p-8" role="status">Loading…</p>}><ProjectClient /></Suspense>
}
