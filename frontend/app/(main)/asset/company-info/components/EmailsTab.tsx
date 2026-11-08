'use client'

import { MobileResultCard, MobileResultList } from './MobileResultCard'

import { Fragment, useMemo, useState } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { ChevronRight } from 'lucide-react'
import { EmailInfo } from '../types'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

interface EmailsTabProps {
  emails: EmailInfo[]
  searchQuery: string
}

export function EmailsTab({ emails, searchQuery }: EmailsTabProps) {
  const { t } = useI18n()
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20)
  const [expandedRowsState, setExpandedRowsState] = useState<{ key: string; rows: Set<string> }>({
    key: '',
    rows: new Set(),
  })
  const paginationKey = `${normalizedQuery}:${emails.length}`

  const filtered = useMemo(() => {
    if (!normalizedQuery) {
      return emails
    }
    return emails.filter((item) => {
      const haystack = [
        item.address,
        item.company,
        item.type,
        ...(item.domains || []),
        ...(item.tags || []),
        ...(item.sources || []),
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
      return haystack.includes(normalizedQuery)
    })
  }, [emails, normalizedQuery])

  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1
  const expandedRows = expandedRowsState.key === paginationKey ? expandedRowsState.rows : new Set<string>()

  const pagedEmails = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filtered.slice(start, start + pageSize)
  }, [filtered, currentPage, pageSize])

  const rowOffset = (currentPage - 1) * pageSize
  const hasQuery = normalizedQuery.length > 0

  const getRowKey = (email: EmailInfo, absoluteIndex: number) =>
    `${email.address}-${email.company || ''}-${absoluteIndex}`

  const toggleExpand = (rowKey: string) => {
    setExpandedRowsState((prev) => {
      const next = new Set(prev.key === paginationKey ? prev.rows : [])
      if (next.has(rowKey)) {
        next.delete(rowKey)
      } else {
        next.add(rowKey)
      }
      return { key: paginationKey, rows: next }
    })
  }

  return (
    <div className="space-y-4">
      <MobileResultList empty={pagedEmails.length === 0} emptyText={hasQuery ? t.companyInfo.emails.emptySearch : t.companyInfo.emails.empty}>
        {pagedEmails.map((email, index) => (
          <MobileResultCard key={getRowKey(email, rowOffset + index)} title={email.address} fields={[
            { label: t.companyInfo.emails.type, value: email.type },
            { label: t.companyInfo.emails.belongCompany, value: email.company },
            { label: t.companyInfo.emails.domains, value: email.domains?.join(', ') },
            { label: t.companyInfo.emails.leakage, value: email.leakageCount },
            ...(email.leakageCount > 0 ? [{ label: t.companyInfo.emails.updatedAt, value: email.leakageUpdatedAt }] : []),
            { label: t.companyInfo.emails.tags, value: email.tags?.join(', ') },
            { label: t.companyInfo.emails.accounts, value: email.leakageAccounts?.join(', ') },
            { label: t.companyInfo.emails.sources, value: <div className="max-h-32 overflow-y-auto [overflow-wrap:anywhere]">{email.sources?.join('\n') || '—'}</div> },
          ]} />
        ))}
      </MobileResultList>
      <div className="hidden md:block rounded-md border overflow-x-auto">
        <Table className="table-fixed min-w-[1024px]">
          <TableHeader>
            <TableRow>
              <TableHead className="w-12" />
              <TableHead className="w-12">#</TableHead>
              <TableHead className="w-[260px]">{t.companyInfo.emails.email}</TableHead>
              <TableHead className="w-[120px]">{t.companyInfo.emails.type}</TableHead>
              <TableHead className="w-[220px]">{t.companyInfo.emails.belongCompany}</TableHead>
              <TableHead className="w-[180px]">{t.companyInfo.emails.domains}</TableHead>
              <TableHead className="w-[200px]">{t.companyInfo.emails.leakage}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="text-center text-muted-foreground py-8">
                  {hasQuery ? t.companyInfo.emails.emptySearch : t.companyInfo.emails.empty}
                </TableCell>
              </TableRow>
            ) : (
              pagedEmails.map((email, index) => {
                const absoluteIndex = rowOffset + index
                const rowKey = getRowKey(email, absoluteIndex)
                const isExpanded = expandedRows.has(rowKey)
                const domainCount = email.domains?.length || 0
                const sourceCount = email.sources?.length || 0

                return (
                  <Fragment key={rowKey}>
                    <TableRow>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 p-0"
                          onClick={() => toggleExpand(rowKey)}
                          aria-label={isExpanded ? t.companyInfo.emails.collapse : t.companyInfo.emails.expand}
                        >
                          <ChevronRight
                            className={cn(
                              'w-4 h-4 transition-transform duration-200 ease-out',
                              isExpanded && 'rotate-90'
                            )}
                          />
                        </Button>
                      </TableCell>
                      <TableCell>{absoluteIndex + 1}</TableCell>
                      <TableCell className="font-medium truncate" title={email.address}>
                        {email.address}
                      </TableCell>
                      <TableCell className="truncate" title={email.type || '-'}>
                        {email.type || '-'}
                      </TableCell>
                      <TableCell className="truncate" title={email.company || '-'}>
                        {email.company || '-'}
                      </TableCell>
                      <TableCell>
                        {domainCount > 0 ? (
                          <div className="truncate" title={email.domains.join(', ')}>
                            {email.domains[0]}
                            {domainCount > 1 ? ` +${domainCount - 1}` : ''}
                          </div>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </TableCell>
                      <TableCell>
                        {email.leakageCount > 0 ? (
                          <div className="space-y-0.5">
                            <p className="text-sm">{t.companyInfo.emails.count}: {email.leakageCount}</p>
                            <p
                              className="text-xs text-muted-foreground truncate"
                              title={email.leakageUpdatedAt || '-'}
                            >
                              {t.companyInfo.emails.updatedAt}: {email.leakageUpdatedAt || '-'}
                            </p>
                          </div>
                        ) : (
                          <span className="text-muted-foreground">-</span>
                        )}
                      </TableCell>
                    </TableRow>
                    {isExpanded && (
                      <TableRow>
                        <TableCell colSpan={7} className="bg-muted/20">
                          <div className="space-y-3 py-1">
                            <div>
                              <p className="text-xs text-muted-foreground mb-1">{t.companyInfo.emails.sources}</p>
                              {sourceCount > 0 ? (
                                <div className="max-h-28 overflow-y-auto space-y-1 text-xs">
                                  {email.sources.map((source, sourceIndex) => (
                                    <p key={`${rowKey}-source-${sourceIndex}`} className="break-all">
                                      {source}
                                    </p>
                                  ))}
                                </div>
                              ) : (
                                <p className="text-xs text-muted-foreground">{t.companyInfo.emails.noSources}</p>
                              )}
                            </div>
                            <div>
                              <p className="text-xs text-muted-foreground mb-1">{t.companyInfo.emails.tags}</p>
                              <div className="flex flex-wrap gap-1">
                                {(email.tags || []).length > 0 ? (
                                  email.tags.map((tag) => (
                                    <Badge key={`${rowKey}-tag-${tag}`} variant="outline" className="text-xs">
                                      {tag}
                                    </Badge>
                                  ))
                                ) : (
                                  <span className="text-xs text-muted-foreground">-</span>
                                )}
                              </div>
                            </div>
                            <div>
                              <p className="text-xs text-muted-foreground mb-1">{t.companyInfo.emails.accounts}</p>
                              <p className="text-xs break-all text-muted-foreground">
                                {(email.leakageAccounts || []).length > 0
                                  ? email.leakageAccounts.join(', ')
                                  : '-'}
                              </p>
                            </div>
                          </div>
                        </TableCell>
                      </TableRow>
                    )}
                  </Fragment>
                )
              })
            )}
          </TableBody>
        </Table>
      </div>
      {filtered.length > 0 && (
        <PaginationPro
          current={currentPage}
          total={filtered.length}
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
