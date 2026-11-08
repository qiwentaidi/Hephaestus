'use client'

import { BackendLogsTab } from '@/app/(main)/settings/components/BackendLogsTab'

export default function BackendLogsPage() {
  return (
    <div className="flex h-full min-h-0 min-w-0 flex-1 overflow-hidden p-3 pb-[max(12px,env(safe-area-inset-bottom))] md:overflow-y-auto md:p-6">
      <div className="h-full min-h-0 min-w-0 w-full md:h-auto">
        <BackendLogsTab />
      </div>
    </div>
  )
}
