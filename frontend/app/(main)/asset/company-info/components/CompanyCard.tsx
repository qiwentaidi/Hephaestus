'use client'

import Image from 'next/image'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  AppWindow,
  Building2,
  ChevronRight,
  Copy,
  Globe,
  Mail,
  Package,
  Share2,
  Smartphone,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { CompanyInfo } from '../types'
import { copyToClipboard } from '@/lib/clipboard'
import toast from 'react-hot-toast'
import { useI18n } from '@/lib/i18n'

interface CompanyCardProps {
  company: CompanyInfo
  isParent?: boolean
  level?: number
  hasSubsidiaries?: boolean
  hasOfficialAccounts?: boolean
  hasApps?: boolean
  hasApplets?: boolean
  firstChar: string
  hasValidLogo: boolean
  logoLoaded: boolean
  loadedLogos: Set<string>
  onLogoLoad: (companyName: string) => void
  onClick?: (e: React.MouseEvent) => void
}

export function CompanyCard({
  company,
  isParent = false,
  level = 0,
  hasSubsidiaries = false,
  hasOfficialAccounts = false,
  hasApps = false,
  hasApplets = false,
  firstChar,
  hasValidLogo,
  logoLoaded,
  loadedLogos,
  onLogoLoad,
  onClick,
}: CompanyCardProps) {
  const { t } = useI18n()

  const getRegStatusDisplay = (status?: string) => {
    const raw = status?.trim() || ''
    if (!raw) {
      return {
        label: t.companyInfo.companyCard.statusUnknown,
        className: 'bg-slate-500/15 text-slate-300 border-slate-500/30',
      }
    }

    const normalized = raw.toLowerCase()
    if (normalized === 'ok') {
      return {
        label: t.companyInfo.companyCard.statusActive,
        className: 'bg-emerald-500/20 text-emerald-300 border-emerald-500/35',
      }
    }

    const activeKeywords = ['存续', '在营', '在业', '开业', '正常', '成立', '登记', '续存', '运营', '有效']
    if (activeKeywords.some((keyword) => raw.includes(keyword))) {
      return {
        label: raw,
        className: 'bg-emerald-500/20 text-emerald-300 border-emerald-500/35',
      }
    }

    const closedKeywords = ['注销', '吊销', '撤销', '解散', '关闭', '终止', '破产']
    if (closedKeywords.some((keyword) => raw.includes(keyword))) {
      return {
        label: raw,
        className: 'bg-rose-500/20 text-rose-300 border-rose-500/35',
      }
    }

    const warningKeywords = ['异常', '停业', '歇业', '清算', '迁出', '冻结', '责令']
    if (warningKeywords.some((keyword) => raw.includes(keyword))) {
      return {
        label: raw,
        className: 'bg-amber-500/20 text-amber-300 border-amber-500/35',
      }
    }

    return {
      label: raw,
      className: 'bg-sky-500/15 text-sky-300 border-sky-500/30',
    }
  }

  const regStatusDisplay = getRegStatusDisplay(company.regStatus)
  const subsidiaries = company.subsidiaries ?? []
  const officialAccounts = company.officialAccounts ?? []
  const applets = company.applets ?? []
  const apps = company.apps ?? []
  const emails = company.emails ?? []
  const domains = company.domains ?? []

  const summaryItems = [
    {
      key: 'subsidiaries',
      icon: Share2,
      label: t.companyInfo.stats.subsidiaries,
      count: subsidiaries.length,
      className: 'text-blue-500',
    },
    {
      key: 'officialAccounts',
      icon: Package,
      label: t.companyInfo.stats.officialAccounts,
      count: officialAccounts.length,
      className: 'text-green-500',
    },
    {
      key: 'applets',
      icon: Smartphone,
      label: t.companyInfo.stats.applets,
      count: applets.length,
      className: 'text-purple-500',
    },
    {
      key: 'apps',
      icon: AppWindow,
      label: t.companyInfo.stats.apps,
      count: apps.length,
      className: 'text-orange-500',
    },
    {
      key: 'emails',
      icon: Mail,
      label: t.companyInfo.stats.emails,
      count: emails.length,
      className: 'text-rose-500',
    },
    {
      key: 'domains',
      icon: Globe,
      label: t.companyInfo.stats.domains,
      count: domains.length,
      className: 'text-cyan-500',
    },
  ] as Array<{
    key: string
    icon: typeof Building2
    label: string
    count: number
    className: string
  }>

  const handleCopyName = async (e: React.MouseEvent) => {
    e.stopPropagation()
    const success = await copyToClipboard(company.companyName)
    if (success) {
      toast.success(t.companyInfo.breadcrumb.copiedCompanyName)
    } else {
      toast.error(t.companyInfo.breadcrumb.copyFailed)
    }
  }

  return (
    <Card
      className={cn(
        "hover:shadow-lg transition-shadow",
        hasSubsidiaries && "cursor-pointer"
      )}
      onClick={onClick}
    >
      <CardHeader>
        <div className="flex min-w-0 items-start justify-between">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            {/* 公司图标区域 */}
            <div className="shrink-0 relative w-12 h-12">
              {/* 首字符图标 - 始终显示 */}
              <div className="absolute inset-0 w-12 h-12 rounded-lg flex items-center justify-center border border-border bg-muted/50 font-semibold text-lg text-foreground">
                {firstChar}
              </div>
              {/* Logo - 如果加载成功则覆盖显示 */}
              {hasValidLogo && (
                <Popover>
                  <PopoverTrigger asChild>
                    <div className="absolute inset-0 cursor-pointer">
                      <Image
                        src={company.trademark}
                        alt={company.companyName}
                        width={48}
                        height={48}
                        unoptimized
                        className={cn(
                          "rounded-lg transition-opacity duration-200",
                          logoLoaded ? "opacity-100" : "opacity-0"
                        )}
                        onLoad={() => onLogoLoad(company.companyName)}
                        onError={() => {
                          // Logo加载失败，保持首字符图标显示
                        }}
                      />
                    </div>
                  </PopoverTrigger>
                  <PopoverContent className="w-48 p-2">
                    <Image
                      src={company.trademark}
                      alt={company.companyName}
                      width={150}
                      height={150}
                      unoptimized
                      className="rounded"
                    />
                  </PopoverContent>
                </Popover>
              )}
            </div>
            <div className="min-w-0 flex-1">
              <div className="group flex min-w-0 max-w-full items-center gap-2">
                <CardTitle
                  className="min-w-0 flex-1 truncate whitespace-nowrap text-lg cursor-pointer hover:text-primary transition-colors"
                  title={company.companyName}
                  onClick={handleCopyName}
                >
                  {company.companyName}
                </CardTitle>
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 w-6 shrink-0 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
                        onClick={handleCopyName}
                      >
                        <Copy className="w-3.5 h-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{t.companyInfo.breadcrumb.copyCompanyName}</TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              </div>
              <div className="mt-1 flex min-w-0 flex-wrap items-center gap-2">
                <Badge variant="outline" className={cn('shrink-0 border', regStatusDisplay.className)}>
                  {regStatusDisplay.label}
                </Badge>
                {!isParent && (
                  <>
                    <Badge variant="outline" className="shrink-0">{company.investment}%</Badge>
                    <Badge variant="outline" className="max-w-full truncate" title={company.amount}>{company.amount}</Badge>
                  </>
                )}
              </div>
            </div>
          </div>
          {hasSubsidiaries && (
            <ChevronRight className="w-5 h-5 text-muted-foreground shrink-0" />
          )}
        </div>
      </CardHeader>
      <CardContent>
        <TooltipProvider>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm">
            {summaryItems.map((item) => {
              const Icon = item.icon
              return (
                <Tooltip key={item.key}>
                  <TooltipTrigger asChild>
                    <div className={cn('flex items-center gap-1.5', item.className)}>
                      <Icon className="h-4 w-4 shrink-0" />
                      <span className="font-semibold tabular-nums">{item.count}</span>
                    </div>
                  </TooltipTrigger>
                  <TooltipContent>
                    {item.label}: {item.count}
                  </TooltipContent>
                </Tooltip>
              )
            })}
          </div>
        </TooltipProvider>
      </CardContent>
    </Card>
  )
}
