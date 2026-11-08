'use client'

import { MobileResultCard, MobileResultList } from './MobileResultCard'

import { useMemo, useState } from 'react'
import Image from 'next/image'
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
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Image as ImageIcon } from 'lucide-react'
import { OfficialAccount } from '../types'
import { useI18n } from '@/lib/i18n'

interface OfficialAccountsTabProps {
  accounts: OfficialAccount[]
  searchQuery: string
}

export function OfficialAccountsTab({ accounts, searchQuery }: OfficialAccountsTabProps) {
  const { t } = useI18n()
  const normalizedQuery = searchQuery.trim().toLowerCase()
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20)
  const paginationKey = `${normalizedQuery}:${accounts.length}`

  const filteredAccounts = useMemo(() => {
    if (!normalizedQuery) {
      return accounts
    }
    return accounts.filter((account) => {
      return (
        account.name.toLowerCase().includes(normalizedQuery) ||
        account.belongCompany.toLowerCase().includes(normalizedQuery) ||
        account.numbers.toLowerCase().includes(normalizedQuery)
      )
    })
  }, [accounts, normalizedQuery])

  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1

  const pagedAccounts = useMemo(() => {
    const start = (currentPage - 1) * pageSize
    return filteredAccounts.slice(start, start + pageSize)
  }, [filteredAccounts, currentPage, pageSize])

  const rowOffset = (currentPage - 1) * pageSize
  const hasQuery = normalizedQuery.length > 0

  return (
    <div className="space-y-4">
      <MobileResultList empty={pagedAccounts.length === 0} emptyText={hasQuery ? t.companyInfo.officialAccounts.emptySearch : t.companyInfo.officialAccounts.empty}>
        {pagedAccounts.map((account, index) => (
          <MobileResultCard key={index} title={account.name} fields={[
            { label: t.companyInfo.officialAccounts.belongCompany, value: account.belongCompany },
            { label: t.companyInfo.officialAccounts.account, value: account.numbers },
            { label: t.companyInfo.officialAccounts.introduction, value: account.introduction },
          ]}>
            {account.qrcode && (
              <Popover>
                <PopoverTrigger asChild><Button variant="outline" size="sm" className="mt-3"><ImageIcon className="h-4 w-4" />{t.companyInfo.officialAccounts.qrcode}</Button></PopoverTrigger>
                <PopoverContent className="w-auto p-2">
                  <Image src={account.qrcode} alt={t.companyInfo.officialAccounts.qrcodeAlt} width={150} height={150} unoptimized className="rounded" />
                </PopoverContent>
              </Popover>
            )}
          </MobileResultCard>
        ))}
      </MobileResultList>
      <div className="hidden md:block rounded-md border overflow-x-auto">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-12">#</TableHead>
              <TableHead>{t.companyInfo.officialAccounts.belongCompany}</TableHead>
              <TableHead>{t.companyInfo.officialAccounts.name}</TableHead>
              <TableHead>{t.companyInfo.officialAccounts.account}</TableHead>
              <TableHead className="w-24">{t.companyInfo.officialAccounts.qrcode}</TableHead>
              <TableHead>{t.companyInfo.officialAccounts.introduction}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredAccounts.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="text-center text-muted-foreground py-8">
                  {hasQuery ? t.companyInfo.officialAccounts.emptySearch : t.companyInfo.officialAccounts.empty}
                </TableCell>
              </TableRow>
            ) : (
              pagedAccounts.map((account, idx) => (
                <TableRow key={`${account.name}-${rowOffset + idx}`}>
                  <TableCell>{rowOffset + idx + 1}</TableCell>
                  <TableCell>{account.belongCompany}</TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      {account.logo && (
                        <Image
                          src={account.logo}
                          alt={account.name}
                          width={20}
                          height={20}
                          unoptimized
                          className="rounded"
                          onError={(e) => {
                            e.currentTarget.style.display = 'none'
                          }}
                        />
                      )}
                      <span>{account.name}</span>
                    </div>
                  </TableCell>
                  <TableCell>{account.numbers}</TableCell>
                  <TableCell>
                    {account.qrcode && (
                      <Popover>
                        <PopoverTrigger asChild>
                          <Button variant="ghost" size="sm">
                            <ImageIcon className="w-4 h-4" />
                          </Button>
                        </PopoverTrigger>
                        <PopoverContent className="w-auto p-2">
                          <Image
                            src={account.qrcode}
                            alt={t.companyInfo.officialAccounts.qrcodeAlt}
                            width={150}
                            height={150}
                            unoptimized
                            className="block h-[150px] w-[150px] rounded"
                          />
                        </PopoverContent>
                      </Popover>
                    )}
                  </TableCell>
                  <TableCell className="max-w-xs truncate">
                    {account.introduction}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {filteredAccounts.length > 0 && (
        <PaginationPro
          current={currentPage}
          total={filteredAccounts.length}
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
