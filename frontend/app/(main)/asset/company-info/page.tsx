'use client'

import { useCallback, useEffect, useMemo, useState } from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Building2, Search, Globe, Package, Smartphone, ArrowLeft, Eye, EyeOff, Download, FileText, Mail } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import { saveBlobAsFile } from '@/lib/download'
import toast from 'react-hot-toast'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { TaskManagementView, type CompanyInfoTask } from './components/TaskManagementView'
import { TaskInfoCard } from './components/TaskInfoCard'
import { StatsCards } from './components/StatsCards'
import { SubsidiariesTab } from './components/SubsidiariesTab'
import { OfficialAccountsTab } from './components/OfficialAccountsTab'
import { EmailsTab } from './components/EmailsTab'
import { AppletsTab } from './components/AppletsTab'
import { AppsTab } from './components/AppsTab'
import { DomainsTab } from './components/DomainsTab'
import { NewTaskDrawer } from './components/NewTaskDrawer'
import { CompanyInfo, FormData, OfficialAccount, App, Applet, EmailInfo } from './types'
import { extractAllChildren, debounce, exportCompanyInfoAsJSON } from './utils'
import { useI18n } from '@/lib/i18n'

export default function CompanyInfoPage() {
  const { t } = useI18n()
  const [companiesInfo, setCompaniesInfo] = useState<CompanyInfo[]>([])
  const [loading, setLoading] = useState(false)
  const [newTaskOpen, setNewTaskOpen] = useState(false)
  const [activeTab, setActiveTab] = useState('subcompany')
  const [searchQuery, setSearchQuery] = useState('')
  const [tasks, setTasks] = useState<CompanyInfoTask[]>([])
  const [tasksLoading, setTasksLoading] = useState(true)
  const [viewMode, setViewMode] = useState<'tasks' | 'results'>('tasks') // 视图模式：任务管理或结果展示
  const [currentTaskId, setCurrentTaskId] = useState<string | null>(null)
  const [currentTask, setCurrentTask] = useState<CompanyInfoTask | null>(null) // 当前任务的详细信息
  const [showTaskInfo, setShowTaskInfo] = useState(false) // 控制任务基础信息的显示/隐藏

  const [formData, setFormData] = useState<FormData>({
    taskName: '',
    company: '',
    invest: 100,
    dataSource: ['riskbird'],
    zeroZoneEnable: false,
    miitEnable: false,
    riskbirdCookie: '',
    zeroZoneKey: '',
    subcompanyLevel: 1,
    miitApi: 'http://127.0.0.1:16181/',
  })

  // 生成测试数据
  const generateTestData = (): CompanyInfo[] => {
    return [
      {
        companyName: '阿里巴巴集团',
        trademark: 'https://via.placeholder.com/20',
        investment: 100,
        amount: '1000万',
        regStatus: '存续',
        domains: ['alibaba.com', 'taobao.com', 'tmall.com'],
        subsidiaries: [
          {
            companyName: '淘宝（中国）软件有限公司',
            trademark: 'https://via.placeholder.com/20',
            investment: 100,
            amount: '500万',
            regStatus: '存续',
            domains: ['taobao.com'],
            subsidiaries: [
              {
                companyName: '淘宝网络技术有限公司',
                trademark: 'https://via.placeholder.com/20',
                investment: 80,
                amount: '300万',
                regStatus: '存续',
                domains: ['taobao.net'],
                subsidiaries: [
                  {
                    companyName: '淘宝移动技术有限公司',
                    trademark: 'https://via.placeholder.com/20',
                    investment: 60,
                    amount: '200万',
                    regStatus: '存续',
                    domains: ['m.taobao.com'],
                    subsidiaries: [],
                    officialAccounts: [],
                    apps: [
                      {
                        serviceName: '淘宝移动端',
                        serviceLicence: 'ICP123456-M',
                        iconUrl: 'https://via.placeholder.com/20',
                        belongCompany: '淘宝移动技术有限公司',
                      },
                    ],
                    applets: [],
                  },
                ],
                officialAccounts: [],
                apps: [],
                applets: [],
              },
              {
                companyName: '天猫网络技术有限公司',
                trademark: 'https://via.placeholder.com/20',
                investment: 70,
                amount: '400万',
                regStatus: '存续',
                domains: ['tmall.com'],
                subsidiaries: [],
                officialAccounts: [
                  {
                    name: '天猫',
                    logo: 'https://via.placeholder.com/20',
                    numbers: 'tmall',
                    qrcode: 'https://via.placeholder.com/150',
                    introduction: '天猫官方公众号',
                    belongCompany: '天猫网络技术有限公司',
                  },
                ],
                apps: [
                  {
                    serviceName: '手机天猫',
                    serviceLicence: 'ICP123457',
                    iconUrl: 'https://via.placeholder.com/20',
                    belongCompany: '天猫网络技术有限公司',
                  },
                ],
                applets: [],
              },
            ],
            officialAccounts: [
              {
                name: '淘宝',
                logo: 'https://via.placeholder.com/20',
                numbers: 'taobao',
                qrcode: 'https://via.placeholder.com/150',
                introduction: '淘宝官方公众号',
                belongCompany: '淘宝（中国）软件有限公司',
              },
            ],
            apps: [
              {
                serviceName: '手机淘宝',
                serviceLicence: 'ICP123456',
                iconUrl: 'https://via.placeholder.com/20',
                belongCompany: '淘宝（中国）软件有限公司',
              },
            ],
            applets: [
              {
                serviceName: '淘宝小程序',
                serviceLicence: 'ICP123456',
                updateRecordTime: '2024-01-15',
                belongCompany: '淘宝（中国）软件有限公司',
              },
            ],
          },
          {
            companyName: '支付宝（中国）网络技术有限公司',
            trademark: 'https://via.placeholder.com/20',
            investment: 100,
            amount: '800万',
            regStatus: '存续',
            domains: ['alipay.com'],
            subsidiaries: [
              {
                companyName: '蚂蚁金服科技有限公司',
                trademark: 'https://via.placeholder.com/20',
                investment: 90,
                amount: '600万',
                regStatus: '存续',
                domains: ['antgroup.com'],
                subsidiaries: [
                  {
                    companyName: '蚂蚁财富管理有限公司',
                    trademark: 'https://via.placeholder.com/20',
                    investment: 75,
                    amount: '400万',
                    regStatus: '存续',
                    domains: ['antfortune.com'],
                    subsidiaries: [],
                    officialAccounts: [],
                    apps: [
                      {
                        serviceName: '蚂蚁财富',
                        serviceLicence: 'ICP789013',
                        iconUrl: 'https://via.placeholder.com/20',
                        belongCompany: '蚂蚁财富管理有限公司',
                      },
                    ],
                    applets: [],
                  },
                ],
                officialAccounts: [],
                apps: [],
                applets: [],
              },
            ],
            officialAccounts: [
              {
                name: '支付宝',
                logo: 'https://via.placeholder.com/20',
                numbers: 'alipay',
                qrcode: 'https://via.placeholder.com/150',
                introduction: '支付宝官方公众号',
                belongCompany: '支付宝（中国）网络技术有限公司',
              },
            ],
            apps: [
              {
                serviceName: '支付宝',
                serviceLicence: 'ICP789012',
                iconUrl: 'https://via.placeholder.com/20',
                belongCompany: '支付宝（中国）网络技术有限公司',
              },
            ],
            applets: [
              {
                serviceName: '支付宝小程序',
                serviceLicence: 'ICP789012',
                updateRecordTime: '2024-02-20',
                belongCompany: '支付宝（中国）网络技术有限公司',
              },
            ],
          },
        ],
        officialAccounts: [
          {
            name: '阿里巴巴',
            logo: 'https://via.placeholder.com/20',
            numbers: 'alibaba',
            qrcode: 'https://via.placeholder.com/150',
            introduction: '阿里巴巴集团官方公众号',
            belongCompany: '阿里巴巴集团',
          },
        ],
        apps: [
          {
            serviceName: '阿里巴巴',
            serviceLicence: 'ICP111111',
            iconUrl: 'https://via.placeholder.com/20',
            belongCompany: '阿里巴巴集团',
          },
        ],
        applets: [],
      },
      {
        companyName: '腾讯科技（深圳）有限公司',
        trademark: 'https://via.placeholder.com/20',
        investment: 100,
        amount: '2000万',
        regStatus: '存续',
        domains: ['tencent.com', 'qq.com', 'weixin.qq.com'],
        subsidiaries: [
          {
            companyName: '深圳市腾讯计算机系统有限公司',
            trademark: 'https://via.placeholder.com/20',
            investment: 100,
            amount: '1500万',
            regStatus: '存续',
            domains: ['qq.com'],
            subsidiaries: [
              {
                companyName: '腾讯游戏科技有限公司',
                trademark: 'https://via.placeholder.com/20',
                investment: 85,
                amount: '1000万',
                regStatus: '存续',
                domains: ['game.qq.com'],
                subsidiaries: [
                  {
                    companyName: '腾讯互娱科技有限公司',
                    trademark: 'https://via.placeholder.com/20',
                    investment: 70,
                    amount: '800万',
                    regStatus: '存续',
                    domains: ['igame.qq.com'],
                    subsidiaries: [],
                    officialAccounts: [],
                    apps: [
                      {
                        serviceName: '王者荣耀',
                        serviceLicence: 'ICP222223',
                        iconUrl: 'https://via.placeholder.com/20',
                        belongCompany: '腾讯互娱科技有限公司',
                      },
                    ],
                    applets: [],
                  },
                ],
                officialAccounts: [],
                apps: [],
                applets: [],
              },
              {
                companyName: '腾讯云科技有限公司',
                trademark: 'https://via.placeholder.com/20',
                investment: 80,
                amount: '900万',
                regStatus: '存续',
                domains: ['cloud.tencent.com'],
                subsidiaries: [],
                officialAccounts: [],
                apps: [
                  {
                    serviceName: '腾讯云',
                    serviceLicence: 'ICP222224',
                    iconUrl: 'https://via.placeholder.com/20',
                    belongCompany: '腾讯云科技有限公司',
                  },
                ],
                applets: [],
              },
            ],
            officialAccounts: [
              {
                name: '腾讯',
                logo: 'https://via.placeholder.com/20',
                numbers: 'tencent',
                qrcode: 'https://via.placeholder.com/150',
                introduction: '腾讯官方公众号',
                belongCompany: '深圳市腾讯计算机系统有限公司',
              },
            ],
            apps: [
              {
                serviceName: 'QQ',
                serviceLicence: 'ICP222222',
                iconUrl: 'https://via.placeholder.com/20',
                belongCompany: '深圳市腾讯计算机系统有限公司',
              },
              {
                serviceName: '微信',
                serviceLicence: 'ICP333333',
                iconUrl: 'https://via.placeholder.com/20',
                belongCompany: '深圳市腾讯计算机系统有限公司',
              },
            ],
            applets: [
              {
                serviceName: '微信小程序',
                serviceLicence: 'ICP333333',
                updateRecordTime: '2024-03-10',
                belongCompany: '深圳市腾讯计算机系统有限公司',
              },
            ],
          },
        ],
        officialAccounts: [
          {
            name: '腾讯科技',
            logo: 'https://via.placeholder.com/20',
            numbers: 'tencent-tech',
            qrcode: 'https://via.placeholder.com/150',
            introduction: '腾讯科技官方公众号',
            belongCompany: '腾讯科技（深圳）有限公司',
          },
        ],
        apps: [],
        applets: [],
      },
      {
        companyName: '北京字节跳动科技有限公司',
        trademark: 'https://via.placeholder.com/20',
        investment: 100,
        amount: '3000万',
        regStatus: '存续',
        domains: ['bytedance.com', 'douyin.com', 'toutiao.com'],
        subsidiaries: [
          {
            companyName: '抖音科技有限公司',
            trademark: 'https://via.placeholder.com/20',
            investment: 95,
            amount: '2000万',
            regStatus: '存续',
            domains: ['douyin.com'],
            subsidiaries: [
              {
                companyName: '抖音短视频技术有限公司',
                trademark: 'https://via.placeholder.com/20',
                investment: 80,
                amount: '1500万',
                regStatus: '存续',
                domains: ['v.douyin.com'],
                subsidiaries: [],
                officialAccounts: [],
                apps: [],
                applets: [
                  {
                    serviceName: '抖音短视频小程序',
                    serviceLicence: 'ICP444445',
                    updateRecordTime: '2024-05-01',
                    belongCompany: '抖音短视频技术有限公司',
                  },
                ],
              },
            ],
            officialAccounts: [
              {
                name: '抖音',
                logo: 'https://via.placeholder.com/20',
                numbers: 'douyin',
                qrcode: 'https://via.placeholder.com/150',
                introduction: '抖音官方公众号',
                belongCompany: '抖音科技有限公司',
              },
            ],
            apps: [
              {
                serviceName: '抖音',
                serviceLicence: 'ICP444444',
                iconUrl: 'https://via.placeholder.com/20',
                belongCompany: '抖音科技有限公司',
              },
            ],
            applets: [],
          },
          {
            companyName: '今日头条科技有限公司',
            trademark: 'https://via.placeholder.com/20',
            investment: 90,
            amount: '1800万',
            regStatus: '存续',
            domains: ['toutiao.com'],
            subsidiaries: [],
            officialAccounts: [],
            apps: [
              {
                serviceName: '今日头条',
                serviceLicence: 'ICP555555',
                iconUrl: 'https://via.placeholder.com/20',
                belongCompany: '今日头条科技有限公司',
              },
            ],
            applets: [],
          },
        ],
        officialAccounts: [
          {
            name: '字节跳动',
            logo: 'https://via.placeholder.com/20',
            numbers: 'bytedance',
            qrcode: 'https://via.placeholder.com/150',
            introduction: '字节跳动官方公众号',
            belongCompany: '北京字节跳动科技有限公司',
          },
          {
            name: '抖音',
            logo: 'https://via.placeholder.com/20',
            numbers: 'douyin',
            qrcode: 'https://via.placeholder.com/150',
            introduction: '抖音官方公众号',
            belongCompany: '北京字节跳动科技有限公司',
          },
        ],
        apps: [
          {
            serviceName: '抖音',
            serviceLicence: 'ICP444444',
            iconUrl: 'https://via.placeholder.com/20',
            belongCompany: '北京字节跳动科技有限公司',
          },
          {
            serviceName: '今日头条',
            serviceLicence: 'ICP555555',
            iconUrl: 'https://via.placeholder.com/20',
            belongCompany: '北京字节跳动科技有限公司',
          },
        ],
        applets: [
          {
            serviceName: '抖音小程序',
            serviceLicence: 'ICP444444',
            updateRecordTime: '2024-04-05',
            belongCompany: '北京字节跳动科技有限公司',
          },
        ],
      },
    ]
  }

  // 获取任务列表
  const fetchTasks = useCallback(async (showLoading = false) => {
    // 只在首次加载或明确要求时显示加载状态
    if (showLoading) {
      setTasksLoading(true)
    }
    try {
      const res = await fetchWithAuth(`/api/v1/company/tasks`)
      if (!res.ok) {
        throw new Error(t.companyInfo.messages.fetchTasksFailed)
      }
      const data = await res.json()
      if (data.success) {
        setTasks(data.data)
      } else {
        throw new Error(data.error || t.companyInfo.messages.fetchTasksFailed)
      }
    } catch (error: any) {
      // 只在显式加载时显示错误提示，避免频繁刷新时的干扰
      if (showLoading) {
        toast.error(`${t.companyInfo.messages.fetchTasksFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
      }
    } finally {
      if (showLoading) {
        setTasksLoading(false)
      }
    }
  }, [
    t.companyInfo.messages.fetchTasksFailed,
    t.companyInfo.messages.unknownError,
  ])

  // 自动刷新任务列表（如果有运行中的任务）
  useEffect(() => {
    const hasRunningTasks = tasks.some(t => t.status === 'running' || t.status === 'pending')
    if (!hasRunningTasks) {
      return
    }

    const interval = setInterval(() => {
      // 静默刷新，不显示加载状态
      fetchTasks(false)
    }, 2000)

    return () => clearInterval(interval)
  }, [fetchTasks, tasks])

  // 从任务加载结果
  const loadTaskResults = useCallback(async (taskId: string) => {
    setLoading(true)
    try {
      // 同时获取任务详情和结果
      const [taskRes, resultsRes] = await Promise.all([
        fetchWithAuth(`/api/v1/company/task/${taskId}`),
        fetchWithAuth(`/api/v1/company/task/${taskId}/results`),
      ])

      if (!taskRes.ok) {
        throw new Error(t.companyInfo.messages.fetchTaskDetailFailed)
      }
      if (!resultsRes.ok) {
        throw new Error(t.companyInfo.messages.fetchTaskResultsFailed)
      }

      const taskData = await taskRes.json()
      const resultsData = await resultsRes.json()

      if (taskData.success && taskData.data) {
        setCurrentTask(taskData.data)
      }
      
      if (resultsData.success && resultsData.data) {
        setCompaniesInfo(resultsData.data)
        setViewMode('results')
        setCurrentTaskId(taskId)
        toast.success(t.companyInfo.messages.taskResultsLoaded)
      } else {
        throw new Error(resultsData.error || t.companyInfo.messages.fetchTaskResultsFailed)
      }
    } catch (error: any) {
      toast.error(`${t.companyInfo.messages.loadTaskResultsFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
    } finally {
      setLoading(false)
    }
  }, [
    t.companyInfo.messages.fetchTaskDetailFailed,
    t.companyInfo.messages.fetchTaskResultsFailed,
    t.companyInfo.messages.loadTaskResultsFailed,
    t.companyInfo.messages.taskResultsLoaded,
    t.companyInfo.messages.unknownError,
  ])

  // 从localStorage加载配置
  useEffect(() => {
    const timer = window.setTimeout(() => {
      const storedMiit = localStorage.getItem('miit-api')
      const storedRiskbirdCookie = localStorage.getItem('riskbird-cookie')
      const storedZeroZoneKey = localStorage.getItem('zerozone-key')

      if (storedMiit || storedRiskbirdCookie || storedZeroZoneKey) {
        setFormData((prev) => ({
          ...prev,
          ...(storedMiit ? { miitApi: storedMiit } : {}),
          ...(storedRiskbirdCookie ? { riskbirdCookie: storedRiskbirdCookie } : {}),
          ...(storedZeroZoneKey ? { zeroZoneKey: storedZeroZoneKey } : {}),
        }))
      }

      const urlParams = new URLSearchParams(window.location.search)
      const taskId = urlParams.get('taskId')
      if (taskId) {
        setCurrentTaskId(taskId)
        setViewMode('results')
        void loadTaskResults(taskId)
      }

      void fetchTasks(true)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [fetchTasks, loadTaskResults])

  // 查看任务结果
  const handleViewTask = (taskId: string) => {
    setCurrentTaskId(taskId)
    loadTaskResults(taskId)
  }

  // 删除任务
  const handleDeleteTask = async (taskId: string) => {
    try {
      const res = await fetchWithAuth(`/api/v1/company/task/${taskId}`, {
        method: 'DELETE',
      })
      if (!res.ok) {
        throw new Error(t.companyInfo.messages.deleteTaskFailed)
      }
      const data = await res.json()
      if (data.success) {
        toast.success(t.companyInfo.messages.taskDeleted)
        fetchTasks(true) // 用户操作，显示加载状态
        // 如果删除的是当前查看的任务，返回任务列表
        if (currentTaskId === taskId) {
          setViewMode('tasks')
          setCurrentTaskId(null)
          setCurrentTask(null)
          setCompaniesInfo([])
        }
      } else {
        throw new Error(data.error || t.companyInfo.messages.deleteTaskFailed)
      }
    } catch (error: any) {
      toast.error(`${t.companyInfo.messages.deleteTaskFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
    }
  }

  // 取消任务
  const handleCancelTask = async (taskId: string) => {
    try {
      const res = await fetchWithAuth(`/api/v1/company/task/${taskId}/cancel`, {
        method: 'POST',
      })
      if (!res.ok) {
        throw new Error(t.companyInfo.messages.cancelTaskFailed)
      }
      const data = await res.json()
      if (data.success) {
        toast.success(t.companyInfo.messages.taskCancelled)
        fetchTasks()
      } else {
        throw new Error(data.error || t.companyInfo.messages.cancelTaskFailed)
      }
    } catch (error: any) {
      toast.error(`${t.companyInfo.messages.cancelTaskFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
    }
  }

  // 批量删除任务
  const handleBatchDeleteTasks = async (taskIds: string[]) => {
    const toastId = toast.loading(`正在删除 ${taskIds.length} 个任务...`)
    let successCount = 0
    let failCount = 0

    try {
      for (const taskId of taskIds) {
        try {
          const res = await fetchWithAuth(`/api/v1/company/task/${taskId}`, {
            method: 'DELETE',
          })
          if (res.ok) {
            const data = await res.json()
            if (data.success) {
              successCount++
            } else {
              failCount++
            }
          } else {
            failCount++
          }
        } catch {
          failCount++
        }
      }

      if (failCount === 0) {
        toast.success(`成功删除 ${successCount} 个任务`, { id: toastId })
      } else {
        toast.error(`删除完成: ${successCount} 成功, ${failCount} 失败`, { id: toastId })
      }

      fetchTasks(true)

      // 如果删除的任务包含当前查看的任务，返回任务列表
      if (currentTaskId && taskIds.includes(currentTaskId)) {
        setViewMode('tasks')
        setCurrentTaskId(null)
        setCurrentTask(null)
        setCompaniesInfo([])
      }
    } catch (error: any) {
      toast.error('批量删除失败: ' + (error.message || t.companyInfo.messages.unknownError), { id: toastId })
    }
  }


  // 保存配置到localStorage（防抖）
  const saveConfig = useMemo(
    () =>
      debounce(() => {
        localStorage.setItem('miit-api', formData.miitApi)
        localStorage.setItem('riskbird-cookie', formData.riskbirdCookie)
        localStorage.setItem('zerozone-key', formData.zeroZoneKey)
      }, 2000),
    [formData.miitApi, formData.riskbirdCookie, formData.zeroZoneKey]
  )

  useEffect(() => {
    saveConfig()
  }, [formData.miitApi, formData.riskbirdCookie, formData.zeroZoneKey, saveConfig])

  // 验证工信部API
  const validateMiitApi = async () => {
    try {
      const res = await fetchWithAuth(`/api/v1/company/validate-miit`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ api: formData.miitApi }),
      })
      if (!res.ok) {
        throw new Error(t.companyInfo.messages.validateApiFailed)
      }
      toast.success(t.companyInfo.messages.apiConnectionOk)
    } catch (error: any) {
      toast.error(`${t.companyInfo.messages.validateApiFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
    }
  }

  // 收集企业信息（任务模式）
  const handleCollectAsTask = async () => {
    if (!formData.company.trim()) {
      toast.error(t.companyInfo.messages.enterCompanyName)
      return
    }

    if (formData.dataSource.length === 0) {
      toast.error(t.companyInfo.messages.selectDataSource)
      return
    }

    if (formData.dataSource.includes('riskbird')) {
      if (!formData.riskbirdCookie.trim()) {
        toast.error(t.companyInfo.messages.enterRiskbirdCookie)
        return
      }
    }

    if ((formData.zeroZoneEnable || formData.dataSource.includes('zerozone')) && !formData.zeroZoneKey.trim()) {
      toast.error(t.companyInfo.messages.enterZeroZoneKey)
      return
    }

    if ((formData.miitEnable || formData.dataSource.includes('miit')) && !formData.miitApi.trim()) {
      toast.error(t.companyInfo.messages.enterMiitApi)
      return
    }

    setNewTaskOpen(false)

    const payloadDataSource = formData.dataSource.includes('riskbird')
      ? formData.dataSource
      : [...formData.dataSource, 'riskbird']

    try {
      const res = await fetchWithAuth(`/api/v1/company/task`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          taskName: formData.taskName.trim(),
          company: formData.company,
          invest: formData.invest,
          dataSource: payloadDataSource,
          riskbirdCookie: formData.riskbirdCookie.replace(/[\r\n]/g, ''),
          zeroZoneKey: formData.zeroZoneKey.trim(),
          zeroZoneEnable: formData.zeroZoneEnable || formData.dataSource.includes('zerozone'),
          subcompanyLevel: formData.subcompanyLevel,
          miitApi: formData.miitApi,
          miitEnable: formData.miitEnable || formData.dataSource.includes('miit'),
        }),
      })

      if (!res.ok) {
        const error = await res.json()
        throw new Error(error.error || t.companyInfo.messages.requestFailed)
      }

      const data = await res.json()
      if (data.success && data.data) {
        toast.success(t.companyInfo.messages.taskCreated)
        // 刷新任务列表（静默刷新，不显示加载状态）
        fetchTasks(false)
      } else {
        throw new Error(data.error || t.companyInfo.messages.createTaskFailed)
      }
    } catch (error: any) {
      toast.error(`${t.companyInfo.messages.createTaskFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
    }
  }


  // 提取所有公众号
  const allOfficialAccounts = useMemo<OfficialAccount[]>(() => {
    return extractAllChildren<OfficialAccount>(companiesInfo, 'officialAccounts')
  }, [companiesInfo])

  const allEmails = useMemo<EmailInfo[]>(() => {
    return extractAllChildren<EmailInfo>(companiesInfo, 'emails')
  }, [companiesInfo])

  // 提取所有APP
  const allApps = useMemo<App[]>(() => {
    return extractAllChildren<App>(companiesInfo, 'apps')
  }, [companiesInfo])

  // 提取所有小程序
  const allApplets = useMemo<Applet[]>(() => {
    return extractAllChildren<Applet>(companiesInfo, 'applets')
  }, [companiesInfo])

  // 提取所有域名
  const allDomains = useMemo<Array<{ domain: string; belongCompany: string }>>(() => {
    const result: Array<{ domain: string; belongCompany: string }> = []
    
    const extractDomains = (companies: CompanyInfo[]) => {
      for (const company of companies) {
        if (company.domains && company.domains.length > 0) {
          for (const domain of company.domains) {
            const normalizedDomain = domain.trim()
            if (!normalizedDomain) continue
            result.push({
              domain: normalizedDomain,
              belongCompany: company.companyName,
            })
          }
        }
        if (company.subsidiaries && company.subsidiaries.length > 0) {
          extractDomains(company.subsidiaries)
        }
      }
    }
    
    extractDomains(companiesInfo)
    // 去重
    const uniqueDomains = new Map<string, { domain: string; belongCompany: string }>()
    for (const item of result) {
      const key = `${item.domain}::${item.belongCompany}`
      if (!uniqueDomains.has(key)) {
        uniqueDomains.set(key, item)
      }
    }
    return Array.from(uniqueDomains.values())
  }, [companiesInfo])

  // 统计数据
  const stats = useMemo(() => {
    const totalCompanies = companiesInfo.length
    const totalSubsidiaries = companiesInfo.reduce((acc, company) => {
      const countSubsidiaries = (list: CompanyInfo[]): number => {
        return list.reduce((sum, c) => {
          return sum + 1 + (c.subsidiaries ? countSubsidiaries(c.subsidiaries) : 0)
        }, 0)
      }
      return acc + (company.subsidiaries ? countSubsidiaries(company.subsidiaries) : 0)
    }, 0)
    return {
      companies: totalCompanies,
      subsidiaries: totalSubsidiaries,
      officialAccounts: allOfficialAccounts.length,
      emails: allEmails.length,
      apps: allApps.length,
      applets: allApplets.length,
      domains: allDomains.length,
    }
  }, [companiesInfo, allOfficialAccounts, allEmails, allApps, allApplets, allDomains])

  return (
    <div className="min-w-0 flex-1 overflow-y-auto" style={{ scrollbarGutter: 'stable' }}>
      <div className="p-3 md:p-6 space-y-4">
        {/* 顶部栏 - 仅结果视图显示返回按钮和基础信息切换按钮 */}
        {viewMode === 'results' && (
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Button
              variant="outline"
              onClick={() => {
                setViewMode('tasks')
                setCurrentTaskId(null)
                setCurrentTask(null)
                setCompaniesInfo([])
                setShowTaskInfo(false)
                // 清除URL参数
                if (typeof window !== 'undefined') {
                  window.history.replaceState({}, '', '/asset/company-info')
                }
              }}
            >
              <ArrowLeft className="w-4 h-4 mr-2" />
              {t.companyInfo.actions.backToTasks}
            </Button>
            {currentTask && (
              <Button
                variant="outline"
                onClick={() => setShowTaskInfo(!showTaskInfo)}
              >
                {showTaskInfo ? (
                  <>
                    <EyeOff className="w-4 h-4 mr-2" />
                    {t.companyInfo.actions.hideTaskInfo}
                  </>
                ) : (
                  <>
                    <Eye className="w-4 h-4 mr-2" />
                    {t.companyInfo.actions.showTaskInfo}
                  </>
                )}
              </Button>
            )}
          </div>
        )}

        {/* 任务管理视图 */}
        {viewMode === 'tasks' && (
          <TaskManagementView
            tasks={tasks}
            loading={tasksLoading}
            onDeleteTask={handleDeleteTask}
            onBatchDeleteTasks={handleBatchDeleteTasks}
            onViewTask={handleViewTask}
            onCancelTask={handleCancelTask}
            onCreateTask={() => setNewTaskOpen(true)}
          />
        )}

        {/* 结果展示视图 */}
        {viewMode === 'results' && (
          <>
            {loading && (
              <Card>
                <CardContent className="pt-6">
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span>{t.companyInfo.loading.results}</span>
                      <span>{t.companyInfo.loading.pleaseWait}</span>
                    </div>
                    <Progress value={undefined} className="h-2" />
                  </div>
                </CardContent>
              </Card>
            )}

            {/* 任务基本信息 */}
            {currentTask && !loading && showTaskInfo && (
              <TaskInfoCard
                taskName={currentTask.taskName}
                config={currentTask.config || '{}'}
                status={currentTask.status}
                createdAt={currentTask.createdAt}
                startedAt={currentTask.startedAt}
                completedAt={currentTask.completedAt}
              />
            )}

            {companiesInfo.length > 0 && (
              <>
                {/* 统计卡片 */}
                <StatsCards stats={stats} />

          <Card className="min-w-0">
            <CardHeader className="px-3 md:px-6">
              <div className="flex min-w-0 flex-col gap-3 md:flex-row md:items-center md:justify-between">
                <div className="min-w-0">
                  <CardTitle>{t.companyInfo.result.title}</CardTitle>
                  <CardDescription>{t.companyInfo.result.description.replace('{count}', String(companiesInfo.length))}</CardDescription>
                </div>
                <div className="flex min-w-0 flex-wrap items-center gap-2 md:flex-nowrap">
                  <div className="relative min-w-0 w-full md:w-64">
                    <Search className="absolute left-2 top-1/2 transform -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                    <Input
                      placeholder={t.companyInfo.result.searchPlaceholder}
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      className="pl-8"
                    />
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="outline" size="sm" disabled={companiesInfo.length === 0 || !currentTaskId}>
                        <Download className="w-4 h-4 mr-2" />
                        {t.companyInfo.actions.export}
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        onClick={async () => {
                          if (!currentTaskId) {
                            toast.error(t.companyInfo.messages.selectTaskFirst)
                            return
                          }
                          try {
                            const response = await fetchWithAuth(`/api/v1/company/task/${currentTaskId}/export`, {
                              method: 'GET',
                            })
                            if (!response.ok) {
                              const error = await response.json()
                              throw new Error(error.error || t.companyInfo.messages.exportFailed)
                            }
                            const blob = await response.blob()
                            const contentDisposition = response.headers.get('Content-Disposition')
                            let filename = `company_info_${currentTaskId}_${Date.now()}.xlsx`
                            if (contentDisposition) {
                              const filenameMatch = contentDisposition.match(/filename="?(.+)"?/i)
                              if (filenameMatch) {
                                filename = filenameMatch[1]
                              }
                            }
                            const result = await saveBlobAsFile(blob, filename)
                            if (result === 'saved') {
                              toast.success(t.companyInfo.messages.exportedXlsx)
                            }
                          } catch (error: any) {
                            toast.error(`${t.companyInfo.messages.exportFailed}: ${error.message || t.companyInfo.messages.unknownError}`)
                          }
                        }}
                      >
                        <FileText className="w-4 h-4 mr-2" />
                        {t.companyInfo.actions.exportAsXlsx}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        onClick={async () => {
                          await exportCompanyInfoAsJSON(companiesInfo)
                          toast.success(t.companyInfo.messages.exportedJson)
                        }}
                      >
                        <FileText className="w-4 h-4 mr-2" />
                        {t.companyInfo.actions.exportAsJson}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>
            </CardHeader>
            <CardContent className="min-w-0 px-3 md:px-6">
            <Tabs value={activeTab} onValueChange={setActiveTab} className="min-w-0">
              <div className="min-w-0 overflow-x-auto pb-1">
              <TabsList className="w-max justify-start md:w-full [&>button]:shrink-0">
                <TabsTrigger value="subcompany">
                  <Building2 className="w-4 h-4 mr-2" />
                  {t.companyInfo.tabs.subcompany}
                </TabsTrigger>
                <TabsTrigger value="wechat">
                  <Package className="w-4 h-4 mr-2" />
                  {t.companyInfo.tabs.wechat}
                </TabsTrigger>
                <TabsTrigger value="email">
                  <Mail className="w-4 h-4 mr-2" />
                  {t.companyInfo.tabs.email}
                </TabsTrigger>
                <TabsTrigger value="applet">
                  <Smartphone className="w-4 h-4 mr-2" />
                  {t.companyInfo.tabs.applet}
                </TabsTrigger>
                <TabsTrigger value="app">
                  <Package className="w-4 h-4 mr-2" />
                  {t.companyInfo.tabs.app}
                </TabsTrigger>
                <TabsTrigger value="domain">
                  <Globe className="w-4 h-4 mr-2" />
                  {t.companyInfo.tabs.domain}
                </TabsTrigger>
              </TabsList>
              </div>

              <TabsContent value="subcompany" className="mt-4">
                <SubsidiariesTab
                  companiesInfo={companiesInfo}
                  searchQuery={searchQuery}
                />
              </TabsContent>

              <TabsContent value="wechat" className="mt-4">
                <OfficialAccountsTab
                  accounts={allOfficialAccounts}
                  searchQuery={searchQuery}
                />
              </TabsContent>

              <TabsContent value="email" className="mt-4">
                <EmailsTab
                  emails={allEmails}
                  searchQuery={searchQuery}
                />
              </TabsContent>

              <TabsContent value="applet" className="mt-4">
                <AppletsTab
                  applets={allApplets}
                  searchQuery={searchQuery}
                />
              </TabsContent>

              <TabsContent value="app" className="mt-4">
                <AppsTab
                  apps={allApps}
                  searchQuery={searchQuery}
                />
              </TabsContent>

              <TabsContent value="domain" className="mt-4">
                <DomainsTab
                  domains={allDomains}
                  searchQuery={searchQuery}
                />
              </TabsContent>
            </Tabs>
          </CardContent>
        </Card>
              </>
            )}
          </>
        )}
      </div>

      {/* 新建任务抽屉 */}
      <NewTaskDrawer
        open={newTaskOpen}
        onOpenChange={setNewTaskOpen}
        formData={formData}
        onFormDataChange={setFormData}
        onCreateTask={handleCollectAsTask}
      />
    </div>
  )
}
