'use client'

import { useEffect, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { TaskDetailView } from '../components/TaskDetailView'
import type { WebScanTaskSummary } from '../types'
import { fetchWithAuth } from '@/lib/api'
import { useI18n } from '@/lib/i18n'

export default function TaskDetailClient() {
  const { t } = useI18n()
  const router = useRouter()
  const searchParams = useSearchParams()
  const taskId = searchParams?.get('taskId') || ''
  const tab = searchParams?.get('tab') || 'info'
  const version = searchParams?.get('version') // 获取版本参数

  const [tasks, setTasks] = useState<WebScanTaskSummary[]>([])

  // 当 taskId 或 tab 变化时，确保 URL 正确
  useEffect(() => {
    if (taskId && !searchParams?.get('tab')) {
      // 如果没有 tab 参数，添加默认的 tab 参数，同时保留 version 参数
      const params = new URLSearchParams()
      params.set('taskId', taskId)
      params.set('tab', 'info')
      if (version) {
        params.set('version', version)
      }
      router.replace(`/scan/vuln-scan/detail?${params.toString()}`)
    }
  }, [taskId, searchParams, router, version])

  // 加载任务列表（用于任务选择器）
  useEffect(() => {
    const loadTasks = async () => {
      try {
        const response = await fetchWithAuth(`/api/v1/webscan/tasks?limit=50`)
        if (response.ok) {
          const data = await response.json()
          if (data.success && data.data) {
            setTasks(data.data.tasks || [])
          }
        }
      } catch (error) {
        console.error('Failed to load tasks:', error)
      }
    }

    loadTasks()
  }, [])

  const handleBack = () => {
    router.push('/scan/vuln-scan')
  }

  const handleTaskChange = (newTaskId: string, nextTab = tab) => {
    // 移动端从任务切换器进入概览，避免不同任务类型沿用不支持的结果页。
    const params = new URLSearchParams()
    params.set('tab', nextTab)
    params.set('taskId', newTaskId)
    if (version) {
      params.set('version', version)
    }
    router.push(`/scan/vuln-scan/detail?${params.toString()}`)
  }

  const handleTabChange = (newTab: string) => {
    // 更新 URL 参数，保持任务ID和version不变
    const params = new URLSearchParams()
    params.set('taskId', taskId)
    params.set('tab', newTab)
    if (version) {
      params.set('version', version)
    }
    router.push(`/scan/vuln-scan/detail?${params.toString()}`)
  }

  if (!taskId) {
    return (
      <div className="flex items-center justify-center h-full">
        <p className="text-muted-foreground">{t.vulnScan.taskDetail.invalidTaskId}</p>
      </div>
    )
  }

  return (
    <div className="flex-1 overflow-hidden flex flex-col">
      <TaskDetailView
        tasks={tasks}
        currentTaskId={taskId}
        currentTab={tab}
        version={version ? parseInt(version, 10) : undefined}
        onBack={handleBack}
        onTaskChange={handleTaskChange}
        onTabChange={handleTabChange}
      />
    </div>
  )
}
