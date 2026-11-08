'use client'

import { MobileResultCard, MobileResultList } from './MobileResultCard'
import { useMemo, useState } from 'react'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { App } from '../types'
import { useI18n } from '@/lib/i18n'

interface AppsTabProps {
  apps: App[]
  searchQuery: string
}

export function AppsTab({ apps, searchQuery }: AppsTabProps) {
  const { t } = useI18n()
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20)
  const paginationKey = `${normalizedQuery}:${apps.length}`

  const filteredApps = useMemo(() => {
    if (!normalizedQuery) {
      return apps
    }
    return apps.filter((app) => {
      return (
        app.serviceName.toLowerCase().includes(normalizedQuery) ||
        app.belongCompany.toLowerCase().includes(normalizedQuery) ||
        app.serviceLicence.toLowerCase().includes(normalizedQuery)
      )
    })
  }, [apps, normalizedQuery])

  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1

  const pagedApps = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filteredApps.slice(start, start + pageSize)
  }, [filteredApps, currentPage, pageSize])

  const rowOffset = (currentPage - 1) * pageSize
  const hasQuery = normalizedQuery.length > 0

  return (
    <div className="space-y-4">
      <MobileResultList empty={pagedApps.length === 0} emptyText={hasQuery ? t.companyInfo.apps.emptySearch : t.companyInfo.apps.empty}>
        {pagedApps.map((app, index) => (
          <MobileResultCard key={index} title={app.serviceName} fields={[
              { label: t.companyInfo.apps.belongCompany, value: app.belongCompany },
              { label: t.companyInfo.apps.licence, value: app.serviceLicence }
          ]} />
        ))}
      </MobileResultList>
      <div className="hidden md:block rounded-md border overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">#</TableHead>
              <TableHead>{t.companyInfo.apps.belongCompany}</TableHead>
              <TableHead>{t.companyInfo.apps.name}</TableHead>
              <TableHead>{t.companyInfo.apps.licence}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredApps.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="text-center text-muted-foreground py-8">
                  {hasQuery ? t.companyInfo.apps.emptySearch : t.companyInfo.apps.empty}
                </TableCell>
              </TableRow>
            ) : (
              pagedApps.map((app, idx) => (
                <TableRow key={`${app.serviceName}-${rowOffset + idx}`}>
                  <TableCell>{rowOffset + idx + 1}</TableCell>
                  <TableCell>{app.belongCompany}</TableCell>
                  <TableCell>{app.serviceName}</TableCell>
                  <TableCell>{app.serviceLicence}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {filteredApps.length > 0 && (
        <PaginationPro
          current={currentPage}
          total={filteredApps.length}
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
