'use client'

import { useState, useRef, useEffect } from 'react'
import { Play, Pause, Square, Eye, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import toast from 'react-hot-toast'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'
import type { DatabaseConnection, CollectResult } from '../types'
import { compileRowMatchRules, matchColumns, matchRows } from '../utils/match'
import { collectResultDetailURL, parseRowData } from '../utils/data'
import { loadCustomCollectRules } from '../utils/rules'

interface ConnectionTabsProps {
  connection: DatabaseConnection
  onConnectionUpdate: (updates: Partial<DatabaseConnection>) => void
  onCollectStart: () => void
}

export function ConnectionTabs({
  connection,
  onConnectionUpdate,
}: ConnectionTabsProps) {
  const { t } = useI18n()
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
  const [isRunning, setIsRunning] = useState(false)
  const [isPaused, setIsPaused] = useState(false)
  const currentDbName = useRef<string>('')
  const currentTableIndex = useRef<number>(0)
  const abortControllerRef = useRef<AbortController | null>(null)

  // 加载已保存的采集结果
  useEffect(() => {
    const loadSavedResults = async () => {
      // 如果已经有 tablePanes，不重复加载
      if (connection.tablePanes && connection.tablePanes.length > 0) {
        return
      }

      try {
        const response = await fetchWithAuth(
          `/api/v1/database/collect/results?connectionNanoid=${connection.nanoid}`
        )
        const data = await response.json()
        if (data.success && data.data && data.data.length > 0) {
          // 转换为前端格式
          const savedResults: CollectResult[] = data.data.map((item: any) => ({
            id: item.id,
            name: item.tableName,
            columns: item.columns,
            rows: item.rows,
            dataError: item.dataError,
            rowsCount: item.rowsCount,
            matchedType: item.matchedType,
            createdAt: item.createdAt,
          }))
          onConnectionUpdate({ tablePanes: savedResults })
        }
      } catch (error) {
        console.error('加载已保存的采集结果失败:', error)
      }
    }

    // 如果连接存在（无论是否已连接），都尝试加载已保存的结果
    loadSavedResults()
  }, [connection.nanoid, connection.tablePanes, onConnectionUpdate])

  // 获取数据库信息
  const getDatabaseInfo = async (): Promise<{ [dbName: string]: string[] }> => {
    try {
      const response = await fetchWithAuth(
        `/api/v1/database/info?nanoid=${connection.nanoid}`
      )
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      
      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.tools.database.toasts.getDatabaseInfoFailed)
      }
      return data.data || {}
    } catch (error) {
      const errorMessage = error instanceof Error 
        ? error.message 
        : t.tools.database.toasts.networkRequestFailed
      throw new Error(`${t.tools.database.toasts.getDatabaseInfoFailed}: ${errorMessage}`)
    }
  }

  // 获取表数据
  const getTableData = async (schemaName: string, tableName: string) => {
    try {
      const response = await fetchWithAuth(`/api/v1/database/table`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          nanoid: connection.nanoid,
          schemaName,
          tableName,
        }),
      })
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      
      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.tools.database.toasts.getTableDataFailed)
      }
      return data.data
    } catch (error) {
      const errorMessage = error instanceof Error 
        ? error.message 
        : t.tools.database.toasts.networkRequestFailed
      throw new Error(
        formatTpl(t.tools.database.toasts.getObjectDataFailed, {
          objectLabel,
          name: `${schemaName}.${tableName}`,
          error: errorMessage,
        })
      )
    }
  }

  // 保存采集结果到数据库
  const saveCollectResult = async (result: CollectResult): Promise<string | null> => {
    try {
      const response = await fetchWithAuth(`/api/v1/database/collect/save`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          connectionNanoid: connection.nanoid,
          result: {
            tableName: result.name,
            columns: result.columns,
            rows: result.rows,
            rowsCount: result.rowsCount,
            matchedType: result.matchedType,
          },
        }),
      })
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`)
      }
      
      const data = await response.json()
      if (!data.success) {
        throw new Error(data.error || t.tools.database.toasts.saveFailedMessage)
      }
      return data.data?.id || null
    } catch (error) {
      console.error('保存采集结果失败:', error)
      // 这里不显示 toast，因为会在调用处统一处理
      return null
    }
  }

  // 开始采集
  const handleStartCollect = async () => {
    if (isRunning && !isPaused) {
      toast.error(t.tools.database.toasts.collectingInProgress)
      return
    }

    if (!connection.connected) {
      toast.error(
        `${t.tools.database.toasts.connectionNotEstablished}\n${t.tools.database.toasts.connectDbFirst}`
      )
      return
    }


    try {
        const wasPaused = isPaused
        setIsRunning(true)
        setIsPaused(false)
        abortControllerRef.current = new AbortController()

        console.log('[采集] 开始采集任务')
        const compiledRowRules = compileRowMatchRules(await loadCustomCollectRules())

        // 获取数据库信息
        const dbInfo = await getDatabaseInfo()
        const dbEntries = Object.entries(dbInfo)
        const totalTables = Object.values(dbInfo).reduce(
          (sum: number, tables: unknown) => sum + (Array.isArray(tables) ? tables.length : 0),
          0
        )

        console.log('[采集] 数据库信息:', { dbEntries: dbEntries.length, totalTables })

        if (dbEntries.length === 0) {
          toast(
            `${t.tools.database.toasts.noDatabaseFound}\n${t.tools.database.toasts.skipDataCollection}`
          )
          setIsRunning(false)
          return
        }

        // 确定起始位置
        let startDbIndex = 0
        let startTableIndex = 0

        if (wasPaused && currentDbName.current) {
          startDbIndex = dbEntries.findIndex(([dbname]) => dbname === currentDbName.current)
          if (startDbIndex !== -1) {
            startTableIndex = currentTableIndex.current
          }
        } else {
          // 重置状态（仅在非暂停恢复时）
          onConnectionUpdate({ tablePanes: [], progress: 0 })
          currentDbName.current = ''
          currentTableIndex.current = 0
        }

        // 遍历数据库和表
        let currentProgress = wasPaused ? (connection.progress || 0) : 0
        let collectedPanes: CollectResult[] = wasPaused ? [...(connection.tablePanes || [])] : []

        console.log(`[采集] 开始采集，总表数: ${totalTables}, 起始进度: ${currentProgress}, 已收集: ${collectedPanes.length}`)

        for (let i = startDbIndex; i < dbEntries.length; i++) {
          const [dbName, tables] = dbEntries[i]
          currentDbName.current = dbName

          for (let j = i === startDbIndex ? startTableIndex : 0; j < tables.length; j++) {
          // 检查是否中止
          if (abortControllerRef.current?.signal.aborted) {
            setIsRunning(false)
            setIsPaused(false)
            return
          }

          // 检查是否暂停
          if (isPaused) {
            currentTableIndex.current = j
            setIsRunning(false)
            return
          }

          const table = tables[j]

          try {
            // 获取表数据
            const tableData = await getTableData(dbName, table)

            if (tableData.rows && tableData.rows.length > 0) {
              // 匹配列名和行数据
              const columnResult = matchColumns(tableData.columns)
              const rowsResult = matchRows(tableData.rows, compiledRowRules)

              console.log(`[采集] 表 ${dbName}.${table}:`, {
                columns: tableData.columns,
                columnMatched: columnResult.matched,
                rowsMatched: rowsResult.matched,
                rowsCount: tableData.rowsCount,
              })

              if (columnResult.matched || rowsResult.matched) {
                const collectedData = parseRowData(tableData)
                const matchedType = columnResult.keyword || rowsResult.matchedLabel || 'Unknown'

                // 添加到结果列表
                const newPane: CollectResult = {
                  name: `${dbName}.${table}`,
                  columns: collectedData.columns,
                  rows: collectedData.rows,
                  rowsCount: tableData.rowsCount,
                  matchedType,
                }

                // 保存到数据库
                const savedId = await saveCollectResult(newPane)
                if (savedId) {
                  newPane.id = savedId
                } else {
                  // 保存失败时提示用户，但不中断采集
                  toast.error(
                    `${t.tools.database.toasts.saveCollectResultFailedTitle}\n${formatTpl(
                      t.tools.database.toasts.saveCollectResultFailedMessage,
                      {
                        objectLabel,
                        name: `${dbName}.${table}`,
                      }
                    )}`
                  )
                }

                collectedPanes.push(newPane)
                console.log(`[采集] 已添加结果: ${newPane.name}`)
              }
            }

            // 更新进度（无论是否匹配都更新）
            currentProgress++
            console.log(`[采集] 进度更新: ${currentProgress}/${totalTables}, 已收集: ${collectedPanes.length}`)
            
            // 更新状态（每次都要传递新的数组引用）
            onConnectionUpdate({
              progress: currentProgress,
              tablePanes: [...collectedPanes],
            })
          } catch (error) {
            const errorMessage = error instanceof Error 
              ? error.message 
              : t.tools.database.toasts.unknownError
            console.error(`[extractdbinfo] Processing table ${dbName}.${table} error:`, error)
            
            // 显示错误提示给用户
            toast.error(
              `${formatTpl(t.tools.database.toasts.processObjectFailedTitle, {
                objectLabel,
                name: `${dbName}.${table}`,
              })}\n${errorMessage}`
            )
            
            // 即使出错也更新进度
            currentProgress++
            onConnectionUpdate({
              progress: currentProgress,
              tablePanes: [...collectedPanes],
            })
            continue
          }
        }
      }

      setIsRunning(false)
      setIsPaused(false)
      currentDbName.current = ''
      currentTableIndex.current = 0

      // 更新连接统计信息
      try {
        const statsResponse = await fetchWithAuth(`/api/v1/database/stats`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            nanoid: connection.nanoid,
            databaseCount: dbEntries.length,
            tableCount: totalTables,
          }),
        })
        const statsData = await statsResponse.json()
        if (statsData.success) {
          // 重新加载连接信息以获取更新时间
          const connResponse = await fetchWithAuth(`/api/v1/database/connections`)
          const connData = await connResponse.json()
          if (connData.success) {
            const updatedConn = connData.data.find(
              (c: any) => c.nanoid === connection.nanoid
            )
            if (updatedConn) {
              // 更新本地状态
              onConnectionUpdate({
                databaseCount: dbEntries.length,
                tableCount: totalTables,
                updatedAt: updatedConn.updatedAt,
              })
            }
          }
        }
      } catch (error) {
        const errorMessage = error instanceof Error 
          ? error.message 
          : t.tools.database.toasts.updateStatsFailedMessage
        console.error('更新统计信息失败:', error)
        toast.error(`${t.tools.database.toasts.updateStatsFailedTitle}\n${errorMessage}`)
      }

      toast.success(
        `${t.tools.database.toasts.collectCompletedTitle}\n${t.tools.database.toasts.collectCompletedMessage}`
      )
    } catch (error) {
      console.error(`[extractdbinfo] Collection process error:`, error)
      const errorMessage = error instanceof Error 
        ? error.message 
        : t.tools.database.toasts.collectUnknownError
      
      toast.error(`${t.tools.database.toasts.collectFailedTitle}\n${errorMessage}`)
      setIsRunning(false)
      setIsPaused(false)
      currentDbName.current = ''
      currentTableIndex.current = 0
    }
  }

  // 暂停/继续采集
  const handleTogglePause = () => {
    if (isPaused) {
      setIsPaused(false)
      handleStartCollect()
    } else {
      setIsPaused(true)
      setIsRunning(false)
      toast(t.tools.database.toasts.collectPaused)
    }
  }

  // 退出采集
  const handleExitCollect = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
    setIsRunning(false)
    setIsPaused(false)
    currentDbName.current = ''
    currentTableIndex.current = 0
    onConnectionUpdate({ progress: 0 })
    toast(t.tools.database.toasts.collectStopped)
  }

  // 查看表格详情 - 跳转到详情页面
  const handleViewTable = (pane: CollectResult) => {
    window.location.assign(collectResultDetailURL(pane, connection.nanoid))
  }

  // 删除表格
  const handleRemoveTable = async (pane: CollectResult) => {
    // 如果有ID，从数据库删除
    if (pane.id) {
      try {
        const response = await fetchWithAuth(`/api/v1/database/collect/result`, {
          method: 'DELETE',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ id: pane.id }),
        })
        const data = await response.json()
        if (!data.success) {
          toast.error(
            `${t.tools.database.toasts.deleteFailedTitle}\n${
              data.error || t.tools.database.toasts.deleteFailedFromDatabase
            }`
          )
          return
        }
      } catch (error) {
        toast.error(
          `${t.tools.database.toasts.deleteFailedTitle}\n${
            error instanceof Error ? error.message : t.tools.database.toasts.networkError
          }`
        )
        return
      }
    }

    // 从本地状态删除
    const existingPanes = connection.tablePanes || []
    onConnectionUpdate({
      tablePanes: existingPanes.filter((p) => p.name !== pane.name),
    })
  }

  const totalTables = connection.tableCount || 0
  const progress = connection.progress || 0

  return (
    <div className="space-y-4">
      {/* 采集结果表格 */}
      <div className="border rounded-lg">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[60px]">#</TableHead>
              <TableHead>
                {formatTpl(t.tools.database.columns.objectName, { objectLabel })}
              </TableHead>
              <TableHead>{t.tools.database.columns.matchType}</TableHead>
              <TableHead>{t.tools.database.columns.rowCount}</TableHead>
              <TableHead className="w-[200px] text-center">{t.common.action}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {connection.tablePanes && connection.tablePanes.length > 0 ? (
              connection.tablePanes.map((pane, index) => (
                <TableRow key={index}>
                  <TableCell>{index + 1}</TableCell>
                  <TableCell className="font-mono">{pane.name}</TableCell>
                  <TableCell>
                    <span className="px-2 py-1 rounded bg-muted text-muted-foreground text-xs border border-border">
                      {pane.matchedType}
                    </span>
                  </TableCell>
                  <TableCell>{pane.rowsCount}</TableCell>
                  <TableCell className="text-center">
                    <div className="flex gap-2 justify-center">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => handleViewTable(pane)}
                      >
                        <Eye className="w-4 h-4 mr-1" />
                        {t.tools.database.actions.view}
                      </Button>
                      <Button
                        size="sm"
                        variant="destructive"
                        onClick={() => handleRemoveTable(pane)}
                      >
                        <Trash2 className="w-4 h-4 mr-1" />
                        {t.common.delete}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={5} className="text-center text-muted-foreground py-8">
                  {t.tools.database.table.noResults}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      {/* 采集控制 */}
      <div className="flex items-center justify-between">
        <span className="text-sm text-muted-foreground">
          {t.tools.database.labels.currentCollectProgress}: {progress}/{totalTables}
        </span>
        <div className="flex gap-2">
          {!isRunning ? (
            <Button onClick={handleStartCollect} disabled={!connection.connected}>
              <Play className="w-4 h-4 mr-2" />
              {connection.tablePanes && connection.tablePanes.length > 0
                ? t.tools.database.actions.recollect
                : t.tools.database.actions.startCollect}
            </Button>
          ) : (
            <>
              <Button
                variant={isPaused ? 'default' : 'outline'}
                onClick={handleTogglePause}
              >
                {isPaused ? (
                  <>
                    <Play className="w-4 h-4 mr-2" />
                    {t.tools.database.actions.resumeCollect}
                  </>
                ) : (
                  <>
                    <Pause className="w-4 h-4 mr-2" />
                    {t.tools.database.actions.pauseCollect}
                  </>
                )}
              </Button>
              <Button variant="destructive" onClick={handleExitCollect}>
                <Square className="w-4 h-4 mr-2" />
                {t.tools.database.actions.exitCollect}
              </Button>
            </>
          )}
        </div>
      </div>

    </div>
  )
}
