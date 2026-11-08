'use client'

import { useState } from 'react'
import {
  Plug,
  PlugZap,
  Edit,
  Trash2,
  Eye,
  Coins,
  FileText,
  Loader2,
} from 'lucide-react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import toast from '@/lib/toast'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import type { DatabaseConnection } from '../types'
import { DatabaseIconComponent, getDbConfig } from '../utils/dbIcons'

interface ConnectionCardProps {
  connection: DatabaseConnection
  onUpdate: (updates: Partial<DatabaseConnection>) => void
  onEdit: () => void
  onConnect: () => void
  onDelete: () => void
}

export function ConnectionCard({
  connection,
  onUpdate,
  onEdit,
  onConnect,
  onDelete,
}: ConnectionCardProps) {
  const { t, language } = useI18n()
  const [loading, setLoading] = useState(false)
  const dbConfig = getDbConfig(connection.scheme)
  const objectLabel =
    connection.scheme === 'mongodb'
      ? t.tools.database.objectLabels.collection
      : connection.scheme === 'redis'
        ? t.tools.database.objectLabels.key
        : t.tools.database.objectLabels.table
  const formatTpl = (tpl: string, vars: Record<string, string | number>) =>
    Object.entries(vars).reduce(
      (acc, [k, v]) => acc.split(`{${k}}`).join(String(v)),
      tpl
    )

  // 连接数据库
  const handleConnect = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (connection.connected) {
      // 断开连接（不清空统计信息，只断开连接状态）
      onUpdate({ connected: false, progress: 0, tablePanes: [] })
      // 从 sessionStorage 移除连接状态
      if (typeof window !== 'undefined') {
        sessionStorage.removeItem(`db_connected_${connection.nanoid}`)
      }
      toast.info({
        title: t.tools.database.toasts.tipTitle,
        message: t.tools.database.toasts.disconnected,
      })
      return
    }

    setLoading(true)
    onUpdate({ loading: true })

    try {
      // 打开新连接前，先断开其他已连接的数据库（通过父组件处理）
      // 这个逻辑会在 page.tsx 的 onConnect 中统一处理
      // 获取数据库信息
      const response = await fetchWithAuth(
        `/api/v1/database/info?nanoid=${connection.nanoid}`
      )

      // 检查 HTTP 状态码
      if (!response.ok) {
        let errorMsg = `${t.tools.database.toasts.serverError} (${response.status})`
        try {
          const errorData = await response.json()
          if (errorData.error) {
            errorMsg = errorData.error
          }
        } catch {
          // 如果无法解析 JSON，使用默认错误信息
        }
        toast.error(
          {
            title: t.tools.database.toasts.connectFailedTitle,
            message: errorMsg,
          },
          { duration: 5000 }
        )
        onUpdate({ loading: false })
        setLoading(false)
        return
      }

      const data = await response.json()

      if (data.success) {
        const dbInfo = data.data || {}
        const dbCount = Object.keys(dbInfo).length
        const tableCount = Object.values(dbInfo).reduce(
          (sum: number, tables: unknown) => sum + (Array.isArray(tables) ? tables.length : 0),
          0
        )

        // 更新统计信息到数据库
        try {
          const statsResponse = await fetchWithAuth(`/api/v1/database/stats`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              nanoid: connection.nanoid,
              databaseCount: dbCount,
              tableCount: tableCount,
            }),
          })
          const statsData = await statsResponse.json()
          if (statsData.success) {
            // 重新加载连接信息以获取更新时间
            const connResponse = await fetchWithAuth(
              `/api/v1/database/connections`
            )
            const connData = await connResponse.json()
            if (connData.success) {
              const updatedConn = connData.data.find(
                (c: DatabaseConnection) => c.nanoid === connection.nanoid
              )
              if (updatedConn) {
                onUpdate({
                  connected: true,
                  databaseCount: dbCount,
                  tableCount: tableCount,
                  loading: false,
                  updatedAt: updatedConn.updatedAt,
                })
                // 保存连接状态到 sessionStorage
                if (typeof window !== 'undefined') {
                  sessionStorage.setItem(`db_connected_${connection.nanoid}`, 'true')
                }
                toast.success({
                  title: t.tools.database.toasts.connectSuccessTitle,
                  message: formatTpl(t.tools.database.toasts.connectSuccessMessage, {
                    dbCount,
                    objectCount: tableCount,
                    objectLabel,
                  }),
                })
                return
              }
            }
          }
        } catch (error) {
          console.error('更新统计信息失败:', error)
        }

        onUpdate({
          connected: true,
          databaseCount: dbCount,
          tableCount: tableCount,
          loading: false,
        })
        
        // 保存连接状态到 sessionStorage
        if (typeof window !== 'undefined') {
          sessionStorage.setItem(`db_connected_${connection.nanoid}`, 'true')
        }

        toast.success({
          title: t.tools.database.toasts.connectSuccessTitle,
          message: formatTpl(t.tools.database.toasts.connectSuccessMessage, {
            dbCount,
            objectCount: tableCount,
            objectLabel,
          }),
        })
      } else {
        // 连接失败，显示详细的错误信息
        const errorMsg = data.error || t.tools.database.toasts.failedGetDatabaseInfo
        toast.error(
          {
            title: t.tools.database.toasts.connectFailedTitle,
            message: errorMsg,
          },
          { duration: 5000 }
        )
        onUpdate({ loading: false })
      }
    } catch (error) {
      const errorMsg =
        error instanceof Error
          ? error.message
          : typeof error === 'string'
            ? error
            : t.tools.database.toasts.networkErrorCheckConnection
      toast.error(`${t.tools.database.toasts.connectFailedTitle}\n${errorMsg}`, {
        duration: 5000,
      })
      onUpdate({ loading: false })
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card
      className={`transition-all overflow-hidden ${
        connection.connected
          ? 'border-green-500/50 bg-gradient-to-br from-green-50/30 to-transparent dark:from-green-950/20 dark:to-transparent'
          : 'border-border hover:border-primary/50'
      }`}
    >
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-start gap-3 flex-1 min-w-0">
            <div className="p-2.5 rounded-xl bg-muted border border-border flex-shrink-0 flex items-center justify-center">
              <DatabaseIconComponent scheme={connection.scheme} size={24} />
            </div>
            <div className="flex-1 min-w-0 space-y-2">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-bold text-lg truncate">
                  {connection.host}:{connection.port}
                </span>
                <Badge variant="outline" className="text-xs font-medium">
                  {dbConfig.label}
                </Badge>
                {connection.connected && (
                  <Badge variant="default" className="bg-green-500 text-white text-xs">
                    {t.tools.database.status.connected}
                  </Badge>
                )}
              </div>
              <div className="flex flex-col gap-1.5">
                {connection.notes && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-xs text-muted-foreground/70 font-medium">
                      {t.tools.database.labels.note}:
                    </span>
                    <span className="text-sm text-foreground truncate">{connection.notes}</span>
                  </div>
                )}
                {connection.updatedAt && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-xs text-muted-foreground/70 font-medium">
                      {t.tools.database.labels.updatedAt}:
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {new Date(connection.updatedAt).toLocaleString(
                        language === 'zh' ? 'zh-CN' : 'en-US'
                      )}
                    </span>
                  </div>
                )}
              </div>
            </div>
          </div>
          <div className="flex gap-1 flex-shrink-0">
            <Button
              variant="ghost"
              size="icon"
              className="h-9 w-9"
              onClick={handleConnect}
              disabled={loading}
              title={
                connection.connected
                  ? t.tools.database.actions.disconnect
                  : t.tools.database.actions.connect
              }
            >
              {loading ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : connection.connected ? (
                <PlugZap className="w-4 h-4 text-green-500" />
              ) : (
                <Plug className="w-4 h-4" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-9 w-9"
              onClick={(e) => {
                e.stopPropagation()
                onEdit()
              }}
              title={t.tools.database.actions.editSettings}
            >
              <Edit className="w-4 h-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              className="h-9 w-9 text-destructive hover:text-destructive"
              onClick={(e) => {
                e.stopPropagation()
                onDelete()
              }}
              title={t.common.delete}
            >
              <Trash2 className="w-4 h-4" />
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className="pt-0">
        <div className="flex items-center justify-between pt-4 border-t border-border/50">
          <div className="flex items-center gap-6 text-sm">
            <div className="flex items-center gap-2">
              <div className="p-1.5 rounded-md bg-muted">
                <Coins className="w-4 h-4 text-muted-foreground" />
              </div>
              <span className="text-muted-foreground">
                {t.tools.database.labels.database}:{' '}
                <span className="font-semibold text-foreground">{connection.databaseCount || 0}</span>
              </span>
            </div>
            <div className="flex items-center gap-2">
              <div className="p-1.5 rounded-md bg-muted">
                <FileText className="w-4 h-4 text-muted-foreground" />
              </div>
              <span className="text-muted-foreground">
                {objectLabel}:{' '}
                <span className="font-semibold text-foreground">{connection.tableCount || 0}</span>
              </span>
            </div>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={(e) => {
              e.stopPropagation()
              onConnect()
            }}
            disabled={loading}
            className="gap-2 border-border/80 text-foreground/90 hover:bg-primary/10 hover:border-primary/50 hover:text-primary transition-colors font-medium disabled:opacity-50"
          >
            <Eye className="w-4 h-4" />
            {t.tools.database.actions.collectInfo}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
