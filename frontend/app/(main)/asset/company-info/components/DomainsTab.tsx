'use client'

import { MobileResultCard, MobileResultList } from './MobileResultCard'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Copy, Download } from 'lucide-react'
import { copyToClipboard } from '@/lib/clipboard'
import toast from 'react-hot-toast'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

interface DomainItem {
  domain: string
  belongCompany: string
}

interface DomainsTabProps {
  domains: DomainItem[]
  searchQuery: string
}

export function DomainsTab({ domains, searchQuery }: DomainsTabProps) {
  const { t } = useI18n()
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20)
  const tableHostRef = useRef<HTMLDivElement | null>(null)
  const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)
  const paginationKey = `${normalizedQuery}:${domains.length}`

  const filteredDomains = useMemo(() => {
    if (!normalizedQuery) {
      return domains
    }
    return domains.filter((item) => {
      return (
        item.domain.toLowerCase().includes(normalizedQuery) ||
        item.belongCompany.toLowerCase().includes(normalizedQuery)
      )
    })
  }, [domains, normalizedQuery])

  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1

  const pagedDomains = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filteredDomains.slice(start, start + pageSize)
  }, [filteredDomains, currentPage, pageSize])

  const updateRightFixedShadow = useCallback(() => {
    const host = tableHostRef.current
    const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
    if (!el) {
      setShowRightFixedShadow(false)
      return
    }

    const maxScrollLeft = el.scrollWidth - el.clientWidth
    setShowRightFixedShadow(maxScrollLeft > 1 && el.scrollLeft < maxScrollLeft - 1)
  }, [])

  useEffect(() => {
    const host = tableHostRef.current
    const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
    if (!el) return

    updateRightFixedShadow()
    el.addEventListener('scroll', updateRightFixedShadow, { passive: true })
    window.addEventListener('resize', updateRightFixedShadow)

    return () => {
      el.removeEventListener('scroll', updateRightFixedShadow)
      window.removeEventListener('resize', updateRightFixedShadow)
    }
  }, [pagedDomains.length, updateRightFixedShadow])

  const rowOffset = (currentPage - 1) * pageSize
  const hasQuery = normalizedQuery.length > 0

  const copyDomains = async (domains: string[]) => {
    const text = domains.join('\n')
    const success = await copyToClipboard(text)
    if (success) {
      toast.success(t.companyInfo.domains.copied.replace('{count}', String(domains.length)))
    } else {
      toast.error(t.companyInfo.breadcrumb.copyFailed)
    }
  }

  const exportDomains = (domains: string[]) => {
    const text = domains.join('\n')
    const blob = new Blob([text], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `domains_${new Date().getTime()}.txt`
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    toast.success(t.companyInfo.domains.exported.replace('{count}', String(domains.length)))
  }

  return (
    <div className="space-y-4">
      {/* 操作按钮 */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-sm text-muted-foreground">
          {t.companyInfo.domains.total.replace('{count}', String(filteredDomains.length))}
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => copyDomains(filteredDomains.map(d => d.domain))}
            disabled={filteredDomains.length === 0}
          >
            <Copy className="w-4 h-4 mr-2" />
            {t.companyInfo.domains.copyAll}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => exportDomains(filteredDomains.map(d => d.domain))}
            disabled={filteredDomains.length === 0}
          >
            <Download className="w-4 h-4 mr-2" />
            {t.companyInfo.domains.exportAll}
          </Button>
        </div>
      </div>

      {/* 域名列表 */}
      <MobileResultList empty={pagedDomains.length === 0} emptyText={hasQuery ? t.companyInfo.domains.emptySearch : t.companyInfo.domains.empty}>
        {pagedDomains.map((item, index) => (
          <MobileResultCard key={index} title={item.domain} fields={[
            { label: t.companyInfo.domains.belongCompany, value: item.belongCompany },
          ]}>
            <Button variant="outline" size="sm" className="mt-3" onClick={() => copyDomains([item.domain])}>
              <Copy className="h-4 w-4" />{t.companyInfo.domains.copyOne}
            </Button>
          </MobileResultCard>
        ))}
      </MobileResultList>
      <div ref={tableHostRef} className="hidden md:block rounded-md border overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">#</TableHead>
              <TableHead>{t.companyInfo.domains.domain}</TableHead>
              <TableHead>{t.companyInfo.domains.belongCompany}</TableHead>
              <TableHead className={cn('w-32 text-center sticky right-0 z-20 border-l border-border bg-card', showRightFixedShadow && 'table-fixed-column-shadow-left')}>{t.companyInfo.domains.actions}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredDomains.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                  {hasQuery ? t.companyInfo.domains.emptySearch : t.companyInfo.domains.empty}
                </TableCell>
              </TableRow>
            ) : (
              pagedDomains.map((item, idx) => (
                <TableRow key={`${item.domain}-${rowOffset + idx}`} className="group">
                  <TableCell>{rowOffset + idx + 1}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <code className="text-sm font-mono bg-muted px-2 py-1 rounded">
                        {item.domain}
                      </code>
                    </div>
                  </TableCell>
                  <TableCell>{item.belongCompany}</TableCell>
                  <TableCell className={cn('text-center sticky right-0 z-10 border-l border-border bg-card group-hover:bg-muted group-data-[state=selected]:bg-muted', showRightFixedShadow && 'table-fixed-column-shadow-left')}>
                    <div className="flex items-center justify-center gap-2">
                      <TooltipProvider>
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-8 w-8 p-0"
                              onClick={() => copyDomains([item.domain])}
                            >
                              <Copy className="w-4 h-4" />
                            </Button>
                          </TooltipTrigger>
                          <TooltipContent>{t.companyInfo.domains.copyOne}</TooltipContent>
                        </Tooltip>
                      </TooltipProvider>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {filteredDomains.length > 0 && (
        <PaginationPro
          current={currentPage}
          total={filteredDomains.length}
          pageSize={pageSize}
          onChange={(page, size) => {
            setPaginationState({ key: paginationKey, currentPage: page })
            setPageSize(size)
          }}
          onPageSizeChange={(size) => {
            setPageSize(size)
            setPaginationState({ key: paginationKey, currentPage: 1 })
          }}
          pageSizeOptions={[10, 20, 50, 100]}
          showSizeChanger
          showQuickJumper
          showTotal
        />
      )}
    </div>
  )
}
