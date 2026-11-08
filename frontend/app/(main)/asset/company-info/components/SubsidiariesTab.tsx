'use client'

import { MobileResultCard, MobileResultList } from './MobileResultCard'

import { useMemo, useState } from 'react'
import Image from 'next/image'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ChevronRight, Copy } from 'lucide-react'
import toast from 'react-hot-toast'
import { cn } from '@/lib/utils'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import { CompanyInfo } from '../types'

interface SubsidiariesTabProps {
  companiesInfo: CompanyInfo[]
  searchQuery: string
}

interface CompanyTreeRow {
  company: CompanyInfo
  key: string
  level: number
  hasChildren: boolean
}

export function SubsidiariesTab({
  companiesInfo,
  searchQuery,
}: SubsidiariesTabProps) {
  const { t } = useI18n()
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(new Set())
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const hasQuery = normalizedQuery.length > 0

  const visibleRows = useMemo(() => {
    const rows: CompanyTreeRow[] = []

    const valueMatchesQuery = (value?: string) => {
      return !!value && value.toLowerCase().includes(normalizedQuery)
    }

    const companyMatchesQuery = (company: CompanyInfo): boolean => {
      if (!hasQuery) return true
      return (
        valueMatchesQuery(company.companyName) ||
        valueMatchesQuery(company.regStatus) ||
        valueMatchesQuery(company.amount) ||
        (company.domains?.some(valueMatchesQuery) ?? false) ||
        (company.officialAccounts?.some((account) =>
          [account.name, account.numbers, account.introduction].some(valueMatchesQuery)
        ) ?? false) ||
        (company.emails?.some((email) =>
          [email.address, email.company, email.type].some(valueMatchesQuery)
        ) ?? false) ||
        (company.apps?.some((app) =>
          [app.serviceName, app.serviceLicence].some(valueMatchesQuery)
        ) ?? false) ||
        (company.applets?.some((applet) =>
          [applet.serviceName, applet.serviceLicence, applet.updateRecordTime].some(valueMatchesQuery)
        ) ?? false)
      )
    }

    const companyOrDescendantMatchesQuery = (company: CompanyInfo): boolean => {
      if (companyMatchesQuery(company)) return true
      return company.subsidiaries?.some((child) => companyOrDescendantMatchesQuery(child)) ?? false
    }

    const visit = (companies: CompanyInfo[], level: number, parentKey: string) => {
      const visibleCompanies = hasQuery
        ? companies.filter((company) => companyOrDescendantMatchesQuery(company))
        : companies

      visibleCompanies.forEach((company, index) => {
        const key = `${parentKey}${index}:${company.companyName}`
        const children = company.subsidiaries ?? []
        const hasChildren = children.length > 0

        rows.push({
          company,
          key,
          level,
          hasChildren,
        })

        if (hasChildren && (hasQuery || expandedKeys.has(key))) {
          visit(children, level + 1, `${key}/`)
        }
      })
    }

    visit(companiesInfo, 0, '')
    return rows
  }, [companiesInfo, expandedKeys, hasQuery, normalizedQuery])

  const toggleExpanded = (key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }

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

  const getAssetItems = (company: CompanyInfo) => {
    return [
      { key: 'officialAccounts', label: t.companyInfo.stats.officialAccounts, count: company.officialAccounts?.length ?? 0 },
      { key: 'emails', label: t.companyInfo.stats.emails, count: company.emails?.length ?? 0 },
      { key: 'applets', label: t.companyInfo.stats.applets, count: company.applets?.length ?? 0 },
      { key: 'apps', label: t.companyInfo.stats.apps, count: company.apps?.length ?? 0 },
      { key: 'domains', label: t.companyInfo.stats.domains, count: company.domains?.length ?? 0 },
    ]
  }

  const copyCompanyName = async (companyName: string) => {
    const success = await copyToClipboard(companyName)
    if (success) {
      toast.success(t.companyInfo.breadcrumb.copiedCompanyName)
    } else {
      toast.error(t.companyInfo.breadcrumb.copyFailed)
    }
  }

  const getCompanyFirstChar = (companyName: string) => {
    return companyName.trim().charAt(0) || '?'
  }

  const hasValidLogo = (company: CompanyInfo) => {
    return !!(company.trademark && company.trademark !== '<nil>')
  }

  const emptyText = hasQuery
    ? t.companyInfo.subsidiaries.emptySearch
    : t.companyInfo.subsidiaries.empty

  return (
    <TooltipProvider delayDuration={150}>
      <MobileResultList empty={visibleRows.length === 0} emptyText={emptyText}>
        {visibleRows.map(({ company, key, level, hasChildren }) => {
          const status = getRegStatusDisplay(company.regStatus)
          const isExpanded = hasQuery || expandedKeys.has(key)
          return (
            <div key={key} className={cn(level > 0 && 'border-l-2 border-border pl-2')}>
              <MobileResultCard title={
                <div className="flex min-w-0 items-start gap-2">
                  <span className="min-w-0 flex-1">{company.companyName}</span>
                  <Button variant="ghost" size="icon" className="h-8 w-8 shrink-0" onClick={() => copyCompanyName(company.companyName)} aria-label={t.companyInfo.breadcrumb.copyCompanyName}><Copy className="h-4 w-4" /></Button>
                </div>
              } fields={[
                { label: t.companyInfo.subsidiaries.status, value: <Badge variant="outline" className={cn('max-w-full whitespace-normal', status.className)}>{status.label}</Badge> },
                ...(level > 0 ? [
                  { label: t.companyInfo.taskInfo.subcompanyLevel, value: `${level} ${t.companyInfo.taskInfo.levelUnit}` },
                  { label: t.companyInfo.subsidiaries.investmentRatio, value: typeof company.investment === 'number' ? `${company.investment}%` : '—' },
                ] : []),
                { label: t.companyInfo.subsidiaries.amount, value: company.amount },
                { label: t.companyInfo.subsidiaries.directInvestments, value: company.subsidiaries?.length ?? 0 },
                { label: t.companyInfo.subsidiaries.assetSummary, value: getAssetItems(company).filter(item => item.count > 0).map(item => `${item.label} ${item.count}`).join(' · ') || t.companyInfo.subsidiaries.noAssets },
              ]}>
                {hasChildren && <Button variant="outline" size="sm" className="mt-3" aria-expanded={isExpanded} disabled={hasQuery} onClick={() => toggleExpanded(key)}>
                  <ChevronRight className={cn('h-4 w-4', isExpanded && 'rotate-90')} />
                  {isExpanded ? t.companyInfo.subsidiaries.collapse : t.companyInfo.subsidiaries.expand} ({company.subsidiaries?.length})
                </Button>}
              </MobileResultCard>
            </div>
          )
        })}
      </MobileResultList>
      <div className="hidden md:block rounded-md border">
        <Table className="min-w-[1048px] table-fixed">
          <TableHeader>
            <TableRow className="bg-muted/40 hover:bg-muted/40">
              <TableHead className="w-12" />
              <TableHead className="w-[320px]">{t.companyInfo.subsidiaries.companyName}</TableHead>
              <TableHead className="w-[120px] text-center">{t.companyInfo.subsidiaries.status}</TableHead>
              <TableHead className="w-[120px] text-right">{t.companyInfo.subsidiaries.investmentRatio}</TableHead>
              <TableHead className="w-[190px] text-right">{t.companyInfo.subsidiaries.amount}</TableHead>
              <TableHead className="w-[120px] text-right">{t.companyInfo.subsidiaries.directInvestments}</TableHead>
              <TableHead className="w-[130px] text-center">{t.companyInfo.subsidiaries.assetSummary}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {visibleRows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-12 text-center text-muted-foreground">
                  {emptyText}
                </TableCell>
              </TableRow>
            ) : (
              visibleRows.map((row) => {
                const { company, hasChildren, key, level } = row
                const childrenCount = company.subsidiaries?.length ?? 0
                const statusDisplay = getRegStatusDisplay(company.regStatus)
                const assetItems = getAssetItems(company)
                const visibleAssetItems = assetItems.filter((item) => item.count > 0)
                const assetTotal = assetItems.reduce((sum, item) => sum + item.count, 0)
                const isExpanded = hasQuery || expandedKeys.has(key)

                return (
                  <TableRow key={key}>
                    <TableCell>
                      <div className="flex h-8 items-center justify-center">
                        {hasChildren ? (
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => toggleExpanded(key)}
                            aria-label={
                              isExpanded
                                ? t.companyInfo.subsidiaries.collapse
                                : t.companyInfo.subsidiaries.expand
                            }
                          >
                            <ChevronRight
                              className={cn(
                                'h-4 w-4 transition-transform duration-200 ease-out',
                                isExpanded && 'rotate-90'
                              )}
                            />
                          </Button>
                        ) : (
                          <span className="inline-block h-7 w-7" />
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex min-w-0 items-center gap-2">
                        <div className="relative h-8 w-8 shrink-0 overflow-hidden rounded-md border border-border bg-muted/50">
                          <div className="absolute inset-0 flex items-center justify-center text-sm font-medium text-foreground">
                            {getCompanyFirstChar(company.companyName)}
                          </div>
                          {hasValidLogo(company) && (
                            <Image
                              src={company.trademark}
                              alt={company.companyName}
                              fill
                              sizes="32px"
                              unoptimized
                              className="relative h-full w-full rounded-md object-cover"
                              onError={(event) => {
                                event.currentTarget.style.display = 'none'
                              }}
                            />
                          )}
                        </div>
                        <div className="min-w-0 truncate font-medium" title={company.companyName}>
                          {company.companyName}
                        </div>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="h-7 w-7 shrink-0 text-muted-foreground hover:text-primary"
                              onClick={() => copyCompanyName(company.companyName)}
                              aria-label={t.companyInfo.breadcrumb.copyCompanyName}
                            >
                              <Copy className="h-4 w-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t.companyInfo.breadcrumb.copyCompanyName}</TooltipContent>
                        </Tooltip>
                      </div>
                    </TableCell>
                    <TableCell className="text-center">
                      <Badge variant="outline" className={cn('border', statusDisplay.className)}>
                        {statusDisplay.label}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right tabular-nums">
                      {level === 0 || typeof company.investment !== 'number' ? (
                        <span className="text-muted-foreground">-</span>
                      ) : (
                        `${company.investment}%`
                      )}
                    </TableCell>
                    <TableCell className="max-w-[160px] truncate text-right" title={company.amount || '-'}>
                      {company.amount || '-'}
                    </TableCell>
                    <TableCell className="text-right font-medium tabular-nums">
                      {childrenCount}
                    </TableCell>
                    <TableCell className="text-center">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span
                            className={cn(
                              'inline-flex min-w-[72px] cursor-default items-center justify-center rounded-md border px-2 py-1 text-xs',
                              assetTotal > 0
                                ? 'border-primary/30 bg-primary/10 text-primary'
                                : 'border-border bg-muted/30 text-muted-foreground'
                            )}
                          >
                            {assetTotal > 0
                              ? t.companyInfo.subsidiaries.assetsCount.replace('{count}', String(assetTotal))
                              : t.companyInfo.subsidiaries.noAssets}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent align="end" className="max-w-[220px]">
                          {visibleAssetItems.length > 0 ? (
                            <div className="grid gap-1">
                              {visibleAssetItems.map((item) => (
                                <div key={item.key} className="flex min-w-[120px] items-center justify-between gap-4">
                                  <span>{item.label}</span>
                                  <span className="font-medium tabular-nums">{item.count}</span>
                                </div>
                              ))}
                            </div>
                          ) : (
                            <span>{t.companyInfo.subsidiaries.noAssetsDetail}</span>
                          )}
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
    </TooltipProvider>
  )
}
