'use client'

import { SettingsContent } from '@/app/(main)/settings/components/SettingsContent'
import { SettingsWindowTitlebar } from '@/components/SettingsWindowTitlebar'

export default function SettingsWindowPage() {
  return (
    <main className="flex h-dvh min-w-0 flex-col overflow-hidden bg-background">
      <SettingsWindowTitlebar />
      <SettingsContent />
    </main>
  )
}
