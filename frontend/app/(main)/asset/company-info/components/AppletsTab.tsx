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
import { Applet } from '../types'
import { useI18n } from '@/lib/i18n'

interface AppletsTabProps {
  applets: Applet[]
  searchQuery: string
}

export function AppletsTab({ applets, searchQuery }: AppletsTabProps) {
  const { t } = useI18n()
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20)
  const paginationKey = `${normalizedQuery}:${applets.length}`

  const filteredApplets = useMemo(() => {
    if (!normalizedQuery) {
      return applets
    }
    return applets.filter((applet) => {
      return (
        applet.serviceName.toLowerCase().includes(normalizedQuery) ||
        applet.belongCompany.toLowerCase().includes(normalizedQuery) ||
        applet.serviceLicence.toLowerCase().includes(normalizedQuery)
      )
    })
  }, [applets, normalizedQuery])

  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1

  const pagedApplets = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filteredApplets.slice(start, start + pageSize)
  }, [filteredApplets, currentPage, pageSize])

  const rowOffset = (currentPage - 1) * pageSize
  const hasQuery = normalizedQuery.length > 0

  return (
    <div className="space-y-4">
      <MobileResultList empty={pagedApplets.length === 0} emptyText={hasQuery ? t.companyInfo.applets.emptySearch : t.companyInfo.applets.empty}>
        {pagedApplets.map((applet, index) => (
          <MobileResultCard key={index} title={applet.serviceName} fields={[
              { label: t.companyInfo.applets.belongCompany, value: applet.belongCompany },
              { label: t.companyInfo.applets.licence, value: applet.serviceLicence },
              { label: t.companyInfo.applets.updatedAt, value: applet.updateRecordTime }
          ]} />
        ))}
      </MobileResultList>
      <div className="hidden md:block rounded-md border overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">#</TableHead>
              <TableHead>{t.companyInfo.applets.belongCompany}</TableHead>
              <TableHead>{t.companyInfo.applets.name}</TableHead>
              <TableHead>{t.companyInfo.applets.licence}</TableHead>
              <TableHead>{t.companyInfo.applets.updatedAt}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredApplets.length === 0 ? (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                  {hasQuery ? t.companyInfo.applets.emptySearch : t.companyInfo.applets.empty}
                </TableCell>
              </TableRow>
            ) : (
              pagedApplets.map((applet, idx) => (
                <TableRow key={`${applet.serviceName}-${rowOffset + idx}`}>
                  <TableCell>{rowOffset + idx + 1}</TableCell>
                  <TableCell>{applet.belongCompany}</TableCell>
                  <TableCell>{applet.serviceName}</TableCell>
                  <TableCell>{applet.serviceLicence}</TableCell>
                  <TableCell>{applet.updateRecordTime}</TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {filteredApplets.length > 0 && (
        <PaginationPro
          current={currentPage}
          total={filteredApplets.length}
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
