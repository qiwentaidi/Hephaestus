'use client'

import { useRouter, usePathname } from 'next/navigation'
import { useEffect, useState } from 'react'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Settings, Key, BookOpen, Bell, Palette, FileText, Languages, RefreshCw, Bot, ArrowLeftRight } from 'lucide-react'
import { ScanConfigTab } from './ScanConfigTab'
import { ApiKeysTab } from './ApiKeysTab'
import { AIConfigTab } from './AIConfigTab'
import { DictTab } from './DictTab'
import { NotificationTab } from './NotificationTab'
import { ThemeTab } from './ThemeTab'
import { ScanTemplateTab } from './ScanTemplateTabV2'
import { ConfigMigrationTab } from './ConfigMigrationTab'
import { LanguageTab } from './LanguageTab'
import { SystemUpdateTab } from './SystemUpdateTab'
import { useI18n } from '@/lib/i18n'

const SETTINGS_TAB_VALUES = ['scan', 'space', 'ai', 'notification', 'dict', 'appearance', 'template', 'migration', 'language', 'update']
const LEGACY_SETTINGS_TAB_ALIASES: Record<string, string> = { dingtalk: 'notification' }

export function SettingsContent() {
  const { t } = useI18n()
  const router = useRouter()
  const pathname = usePathname()
  const categories = [
    { value: 'scan', label: t.settings.tabs.scan, icon: Settings },
    { value: 'space', label: t.settings.tabs.apiKeys, icon: Key },
    { value: 'ai', label: t.settings.tabs.ai, icon: Bot },
    { value: 'notification', label: t.settings.tabs.notification, icon: Bell },
    { value: 'dict', label: t.settings.tabs.dict, icon: BookOpen },
    { value: 'appearance', label: t.settings.tabs.theme, icon: Palette },
    { value: 'template', label: t.settings.tabs.template, icon: FileText },
    { value: 'migration', label: t.settings.tabs.migration, icon: ArrowLeftRight },
    { value: 'language', label: t.settings.tabs.language, icon: Languages },
    { value: 'update', label: t.settings.tabs.update, icon: RefreshCw },
  ]
  const [activeTab, setActiveTab] = useState('scan')
  useEffect(() => {
    const syncTabFromLocation = () => {
      const requestedTab = new URLSearchParams(window.location.search).get('tabs')
      const normalizedTab = requestedTab ? LEGACY_SETTINGS_TAB_ALIASES[requestedTab] || requestedTab : ''
      setActiveTab(SETTINGS_TAB_VALUES.includes(normalizedTab) ? normalizedTab : 'scan')
      if (requestedTab && normalizedTab !== requestedTab && LEGACY_SETTINGS_TAB_ALIASES[requestedTab]) {
        const params = new URLSearchParams(window.location.search)
        params.set('tabs', normalizedTab)
        router.replace(`${pathname}?${params.toString()}`, { scroll: false })
      }
    }
    syncTabFromLocation()
    window.addEventListener('popstate', syncTabFromLocation)
    return () => window.removeEventListener('popstate', syncTabFromLocation)
  }, [pathname, router])
  const handleTabChange = (value: string) => {
    if (!SETTINGS_TAB_VALUES.includes(value)) return
    setActiveTab(value)
    const params = new URLSearchParams(window.location.search)
    params.set('tabs', value)
    router.push(`${pathname}?${params.toString()}`, { scroll: false })
  }

  return (
    <Tabs value={activeTab} onValueChange={handleTabChange} className="settings-content h-full min-h-0 min-w-0 flex-1 flex-col gap-0 md:flex-row md:items-stretch">
      <div className="shrink-0 border-b border-border/70 bg-background p-3 md:hidden">
        <Select value={activeTab} onValueChange={handleTabChange}>
          <SelectTrigger aria-label={t.settings.category} className="h-11 w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {categories.map(({ value, label, icon: Icon }) => (
              <SelectItem key={value} value={value} className="min-h-11">
                <span className="flex items-center gap-2"><Icon className="size-4 shrink-0" />{label}</span>
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <aside className="hidden h-full w-64 shrink-0 self-stretch overflow-y-auto border-r border-border/70 bg-card/50 p-3 md:block">
        <TabsList className="flex h-auto w-full flex-col items-stretch justify-start gap-1 border-0 bg-transparent p-0 shadow-none">
          {categories.map(({ value, label, icon: Icon }) => (
            <TabsTrigger key={value} value={value} className="h-10 w-full flex-none justify-start gap-2">
              <Icon className="size-4" />{label}
            </TabsTrigger>
          ))}
        </TabsList>
      </aside>
      <section key={activeTab} className="min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain p-3 pb-[max(12px,env(safe-area-inset-bottom))] md:p-6">
          {/* 扫描配置 */}
          <TabsContent value="scan" className="m-0 scan-panel-enter">
            <ScanConfigTab />
          </TabsContent>

          {/* API密钥配置 */}
          <TabsContent value="space" className="m-0 scan-panel-enter">
            <ApiKeysTab />
          </TabsContent>

          {/* AI配置 */}
          <TabsContent value="ai" className="m-0 scan-panel-enter">
            <AIConfigTab />
          </TabsContent>

          {/* 字典管理 */}
          <TabsContent value="dict" className="m-0 scan-panel-enter xl:h-full">
            <DictTab />
          </TabsContent>

          {/* 通知配置 */}
          <TabsContent value="notification" className="m-0 scan-panel-enter">
            <NotificationTab />
          </TabsContent>

          {/* 主题设置 */}
          <TabsContent value="appearance" className="m-0 scan-panel-enter">
            <ThemeTab />
          </TabsContent>

          {/* 扫描模板管理 */}
          <TabsContent value="template" className="m-0 scan-panel-enter">
            <ScanTemplateTab />
          </TabsContent>

          {/* 配置迁移 */}
          <TabsContent value="migration" className="m-0 scan-panel-enter">
            <ConfigMigrationTab />
          </TabsContent>

          {/* 语言设置 */}
          <TabsContent value="language" className="m-0 scan-panel-enter">
            <LanguageTab />
          </TabsContent>

          {/* 系统更新 */}
          <TabsContent value="update" className="m-0 scan-panel-enter">
            <SystemUpdateTab />
          </TabsContent>
      </section>
    </Tabs>
  )
}
