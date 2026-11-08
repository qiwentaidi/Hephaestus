'use client'

import { Card } from '@/components/ui/card'
import { AppWindow, Building2, Share2, Package, Smartphone, Globe, Mail } from 'lucide-react'
import { useI18n } from '@/lib/i18n'

interface StatsCardsProps {
  stats: {
    companies: number
    subsidiaries: number
    officialAccounts: number
    apps: number
    applets: number
    emails: number
    domains: number
  }
}

export function StatsCards({ stats }: StatsCardsProps) {
  const { t } = useI18n()
  return (
    <div className="grid grid-cols-3 md:grid-cols-2 lg:grid-cols-7 gap-2 md:gap-4">
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.companies}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.companies}</p>
          </div>
          <Building2 className="hidden md:block w-8 h-8 text-primary opacity-50" />
        </div>
      </Card>
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.subsidiaries}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.subsidiaries}</p>
          </div>
          <Share2 className="hidden md:block w-8 h-8 text-blue-500 opacity-50" />
        </div>
      </Card>
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.officialAccounts}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.officialAccounts}</p>
          </div>
          <Package className="hidden md:block w-8 h-8 text-green-500 opacity-50" />
        </div>
      </Card>
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.emails}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.emails}</p>
          </div>
          <Mail className="hidden md:block w-8 h-8 text-rose-500 opacity-50" />
        </div>
      </Card>
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.applets}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.applets}</p>
          </div>
          <Smartphone className="hidden md:block w-8 h-8 text-purple-500 opacity-50" />
        </div>
      </Card>
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.apps}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.apps}</p>
          </div>
          <AppWindow className="hidden md:block w-8 h-8 text-orange-500 opacity-50" />
        </div>
      </Card>
      <Card className="min-w-0 p-3 md:p-4">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs md:text-sm text-muted-foreground">{t.companyInfo.stats.domains}</p>
            <p className="text-xl md:text-2xl font-bold mt-1 break-all">{stats.domains}</p>
          </div>
          <Globe className="hidden md:block w-8 h-8 text-cyan-500 opacity-50" />
        </div>
      </Card>
    </div>
  )
}
