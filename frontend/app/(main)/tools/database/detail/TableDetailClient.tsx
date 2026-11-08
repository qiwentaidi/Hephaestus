'use client'

import { useState, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { ArrowLeft, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import toast from 'react-hot-toast'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import type { RowData } from '../types'
import { decodeTablePreview, parseRowData } from '../utils/data'
import { CollectedDataTable } from '../components/CollectedDataTable'

export default function TableDetailClient() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const { t } = useI18n()
  const [tableData, setTableData] = useState<RowData | null>(null)
  const [tableName, setTableName] = useState<string>('')
  const [matchedType, setMatchedType] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const loadContent = async () => {
      const id = searchParams.get('id')
      const encodedData = searchParams.get('data')
      const name = searchParams.get('tableName')
      const connectionNanoid = searchParams.get('connectionNanoid')

      if (name) {
        setTableName(name)
      }

      try {
        if (id && connectionNanoid) {
          // 如果有 ID，从 results 接口获取所有结果，然后筛选
          const response = await fetchWithAuth(
            `/api/v1/database/collect/results?connectionNanoid=${encodeURIComponent(connectionNanoid)}`
          )
          const data = await response.json()
          if (data.success && data.data) {
            const result = data.data.find((item: any) => item.id === id)
            if (result) {
              setTableData(parseRowData(result))
              setMatchedType(typeof result.matchedType === 'string' ? result.matchedType : '')
              if (result.tableName) {
                setTableName(result.tableName)
              }
            } else {
              toast.error(
                `${t.tools.database.tableDetail.notFoundTitle}\n${t.tools.database.tableDetail.notFoundMessage}`
              )
              router.push('/tools/database')
            }
          } else {
            toast.error(
              `${t.tools.database.toasts.loadFailedTitle}\n${
                data.error || t.tools.database.tableDetail.loadFailedMessage
              }`
            )
            router.push('/tools/database')
          }
        } else if (encodedData) {
          // 未保存的采样通过严格验证的 JSON 预览，旧 HTML 参数不再解析。
          try {
            setTableData(decodeTablePreview(encodedData))
            setMatchedType(searchParams.get('matchedType') || '')
          } catch (error) {
            toast.error(
              `${t.tools.database.tableDetail.decodeFailedTitle}\n${t.tools.database.tableDetail.decodeFailedMessage}`
            )
            router.push('/tools/database')
          }
        } else {
          toast.error(
            `${t.tools.database.tableDetail.paramErrorTitle}\n${t.tools.database.tableDetail.paramErrorMessage}`
          )
          router.push('/tools/database')
        }
      } catch (error) {
        toast.error(
          `${t.tools.database.toasts.loadFailedTitle}\n${
            error instanceof Error ? error.message : t.tools.database.toasts.networkError
          }`
        )
        router.push('/tools/database')
      } finally {
        setLoading(false)
      }
    }

    loadContent()
  }, [searchParams, router, t]) // 修复依赖，避免重复请求

  const handleBack = () => {
    const connectionNanoid = searchParams.get('connectionNanoid')
    if (connectionNanoid) {
      router.push(`/tools/database?nanoid=${connectionNanoid}`)
    } else {
      router.push('/tools/database')
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
      </div>
    )
  }

  return (
    <div className="flex flex-col h-screen">
      {/* 头部导航 */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b flex-shrink-0">
        <Button variant="ghost" onClick={handleBack} className="gap-2">
          <ArrowLeft className="w-4 h-4" />
          {t.common.back}
        </Button>
        <div className="min-w-0 flex-1">
          <h1 className="text-lg font-semibold">{t.tools.database.tableDetail.title}</h1>
          <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
            {tableName && (
              <p className="min-w-0 break-all text-sm text-muted-foreground font-mono">{tableName}</p>
            )}
            <Badge variant="outline" className="max-w-full min-w-0 shrink whitespace-normal break-words [overflow-wrap:anywhere]">
              {t.tools.database.tableDetail.matchedType}: {matchedType.trim() ? matchedType : t.tools.database.tableDetail.matchNotRecorded}
            </Badge>
          </div>
        </div>
      </div>

      {/* 内容区域 */}
      <div className="flex-1 min-h-0 overflow-hidden">
        <div
          className="h-full w-full overflow-auto p-6"
          style={{
            overflowX: 'auto',
            overflowY: 'auto',
          }}
        >
          {tableData && <CollectedDataTable key={searchParams.get('id') || searchParams.get('data') || ''} data={tableData} />}
        </div>
      </div>
    </div>
  )
}
