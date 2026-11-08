import { Suspense } from 'react'
import TaskManagement from './components/TaskManagement'

export default function AITasksPage() {
  return <Suspense fallback={<div className="p-6" role="status">Loading…</div>}><TaskManagement /></Suspense>
}
