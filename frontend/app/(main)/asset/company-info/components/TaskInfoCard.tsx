'use client'

import { useMemo, useState } from 'react'
import Image from 'next/image'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { getStatusBadge as renderStatusBadge } from '@/lib/status-badge'
import { Building2, Target, Settings, Calendar } from 'lucide-react'
import { useI18n } from '@/lib/i18n'

interface TaskConfig {
  company?: string
  companies?: string[]
  invest?: number
  dataSource?: string[]
  riskbirdCookie?: string
  zeroZoneKey?: string
  subcompanyLevel?: number
  miitApi?: string
}

interface TaskInfoCardProps {
  taskName: string
  config: string // JSON 字符串
  status: string
  createdAt: string
  startedAt?: string
  completedAt?: string
}

export function TaskInfoCard({
  taskName,
  config,
  status,
  createdAt,
  startedAt,
  completedAt,
}: TaskInfoCardProps) {
  const { t } = useI18n()
  const [paginationState, setPaginationState] = useState({ key: '', currentPage: 1 })
  const [pageSize, setPageSize] = useState(20) // 每页显示20个公司
  const taskStatusLabels = {
    pending: t.companyInfo.taskManagement.pending,
    running: t.companyInfo.taskManagement.running,
    completed: t.companyInfo.taskManagement.completed,
    failed: t.companyInfo.taskManagement.failed,
    cancelled: t.companyInfo.taskManagement.cancelled,
  }

  const taskConfig = useMemo(() => {
    try {
      return JSON.parse(config) as TaskConfig
    } catch (e) {
      console.error('Failed to parse task config:', e)
      return {}
    }
  }, [config])

  // 获取公司列表
  const companies = useMemo(
    () => taskConfig.companies || (taskConfig.company ? taskConfig.company.split('\n').filter(c => c.trim()) : []),
    [taskConfig.companies, taskConfig.company]
  )
  const paginationKey = String(companies.length)
  const currentPage = paginationState.key === paginationKey ? paginationState.currentPage : 1

  // 分页计算
  const paginatedCompanies = companies.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize
  )

  // 格式化时间
  const formatDate = (dateString?: string) => {
    if (!dateString) return '-'
    return new Date(dateString).toLocaleString('zh-CN')
  }

  return (
    <Card>
      <CardHeader className="px-3 md:px-6">
        <div className="flex flex-wrap items-start justify-between gap-2">
          <div>
            <CardTitle className="flex flex-wrap items-center gap-2">
              <Building2 className="w-5 h-5" />
              {taskName}
            </CardTitle>
            <CardDescription className="mt-1">
              {t.companyInfo.taskInfo.description}
            </CardDescription>
          </div>
          {renderStatusBadge(status, taskStatusLabels)}
        </div>
      </CardHeader>
      <CardContent className="space-y-6 px-3 md:px-6">
        {/* 时间信息 */}
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <Calendar className="w-4 h-4 text-muted-foreground" />
            {t.companyInfo.taskInfo.timeInfo}
          </div>
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
            <div>
              <div className="text-muted-foreground">{t.companyInfo.taskInfo.createdAt}</div>
              <div className="mt-1">{formatDate(createdAt)}</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t.companyInfo.taskInfo.startedAt}</div>
              <div className="mt-1">{formatDate(startedAt)}</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t.companyInfo.taskInfo.completedAt}</div>
              <div className="mt-1">{formatDate(completedAt)}</div>
            </div>
          </div>
        </div>

        <Separator />

        {/* 目标公司 */}
        <div className="space-y-3">
          <div className="flex items-center gap-2 text-sm font-medium">
            <Target className="w-4 h-4 text-muted-foreground" />
            {t.companyInfo.taskInfo.targetCompanies.replace('{count}', String(companies.length))}
          </div>
          <div className="flex flex-wrap gap-2">
            {paginatedCompanies.map((company, index) => (
              <Badge key={index} variant="outline" className="text-xs max-w-full whitespace-normal break-all">
                {company}
              </Badge>
            ))}
          </div>
          {/* 分页控制 */}
          {companies.length > pageSize && (
            <div className="pt-2">
              <PaginationPro
                current={currentPage}
                total={companies.length}
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
            </div>
          )}
        </div>

        <Separator />

        {/* 参数配置 */}
        <div className="space-y-4">
          <div className="flex items-center gap-2 text-sm font-medium">
            <Settings className="w-4 h-4 text-muted-foreground" />
            {t.companyInfo.taskInfo.settings}
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 text-sm">
            <div>
              <div className="text-muted-foreground">{t.companyInfo.taskInfo.investThreshold}</div>
              <div className="mt-1 font-medium">{taskConfig.invest || 100}%</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t.companyInfo.taskInfo.subcompanyLevel}</div>
              <div className="mt-1 font-medium">{taskConfig.subcompanyLevel || 1} {t.companyInfo.taskInfo.levelUnit}</div>
            </div>
            <div>
              <div className="text-muted-foreground">{t.companyInfo.taskInfo.dataSource}</div>
              <div className="mt-1 flex items-center gap-2">
                {taskConfig.dataSource?.includes('riskbird') && (
                  <Badge variant="outline" className="flex items-center gap-1">
                    <Image src="/icon/riskbird.ico" alt={t.companyInfo.taskInfo.riskbird} width={14} height={14} unoptimized />
                    {t.companyInfo.taskInfo.riskbird}
                  </Badge>
                )}
                {taskConfig.dataSource?.includes('miit') && (
                  <Badge variant="outline" className="flex items-center gap-1">
                    <Image src="/icon/icp.ico" alt={t.companyInfo.taskInfo.miit} width={14} height={14} unoptimized />
                    {t.companyInfo.taskInfo.miit}
                  </Badge>
                )}
                {taskConfig.dataSource?.includes('zerozone') && (
                  <Badge variant="outline" className="flex items-center gap-1">
                    0.zone
                  </Badge>
                )}
              </div>
            </div>
          </div>


        </div>
      </CardContent>
    </Card>
  )
}
