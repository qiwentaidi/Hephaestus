'use client'

import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  ResizablePanelGroup,
  ResizablePanel,
  ResizableHandle,
} from '@/components/ui/resizable'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import mobileStyles from './codec-mobile.module.css'
import {
  Copy,
  Trash2,
  ArrowRight,
  ChevronRight,
  Search,
  Wand2,
  X,
  ArrowUp,
  GripVertical,
  Plus,
  Repeat2,
} from 'lucide-react'
import toast from 'react-hot-toast'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'

// 导入类型和操作定义
import type { Operation, AddedOperation } from './types'
import { categoryIds, getCategories, getOperations } from './operations'
import { sha256, sha1, sha512 } from './lib/hash'

// 标签页数据结构
interface TabData {
  id: string
  name: string
  input: string
  output: string
  addedOps: AddedOperation[]
}

// 本地存储 key
const STORAGE_KEY = 'codec-tabs-data'

// 生成唯一ID
const generateId = () => `tab-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`

// 创建新标签页
const createNewTab = (index: number, tabLabel = 'Tab'): TabData => ({
  id: generateId(),
  name: `${tabLabel} ${index}`,
  input: '',
  output: '',
  addedOps: [],
})

export default function CodecPage() {
  const { t } = useI18n()
  const i18n = t.tools.codec
  const operationIdPrefix = useId()
  const nextOperationIdRef = useRef(0)
  const categories = useMemo(() => getCategories(i18n), [i18n])
  const operations = useMemo(() => getOperations(i18n), [i18n])
  const [initialTab] = useState(() => createNewTab(1, i18n.tab))

  // 标签页状态
  const [tabs, setTabs] = useState<TabData[]>(() => [initialTab])
  const [activeTabId, setActiveTabId] = useState<string>('')
  const [isLoaded, setIsLoaded] = useState(false)
  
  // 通用状态
  const [searchQuery, setSearchQuery] = useState('')
  const [expandedCategories, setExpandedCategories] = useState<string[]>(categoryIds)
  const [draggedIndex, setDraggedIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

  // 从 localStorage 加载数据
  useEffect(() => {
    const timer = window.setTimeout(() => {
      try {
        const saved = localStorage.getItem(STORAGE_KEY)
        if (saved) {
          const data = JSON.parse(saved)
          if (data.tabs && data.tabs.length > 0) {
            setTabs(data.tabs)
            setActiveTabId(data.activeTabId || data.tabs[0].id)
          } else {
            setActiveTabId(initialTab.id)
          }
        } else {
          setActiveTabId(initialTab.id)
        }
      } catch (e) {
        console.error('Failed to load codec data:', e)
        setActiveTabId(initialTab.id)
      }
      setIsLoaded(true)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [initialTab.id])

  // 保存到 localStorage
  useEffect(() => {
    if (isLoaded) {
      try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify({ tabs, activeTabId }))
      } catch (e) {
        console.error('Failed to save codec data:', e)
      }
    }
  }, [tabs, activeTabId, isLoaded])

  useEffect(() => {
    if (!isLoaded) return

    const timer = window.setTimeout(() => {
      setTabs(prev => prev.map((tab, index) => {
        const isGeneratedName = /^标签 \d+$/.test(tab.name) || /^Tab \d+$/.test(tab.name)
        if (isGeneratedName && !tab.input && !tab.output && tab.addedOps.length === 0) {
          return { ...tab, name: `${i18n.tab} ${index + 1}` }
        }
        return tab
      }))
    }, 0)

    return () => window.clearTimeout(timer)
  }, [i18n.tab, isLoaded])

  // 获取当前标签页
  const activeTab = useMemo(() => {
    return tabs.find(t => t.id === activeTabId) || tabs[0]
  }, [tabs, activeTabId])

  // 更新当前标签页数据
  const updateActiveTab = useCallback((updates: Partial<TabData>) => {
    setTabs(prev => prev.map(tab => 
      tab.id === activeTabId ? { ...tab, ...updates } : tab
    ))
  }, [activeTabId])

  // 过滤操作
  const filteredOperations = useMemo(() => {
    if (!searchQuery) return operations
    const query = searchQuery.toLowerCase()
    return operations.filter(
      (op) =>
        op.name.toLowerCase().includes(query) ||
        op.description.toLowerCase().includes(query)
    )
  }, [operations, searchQuery])

  // 按分类分组
  const groupedOperations = useMemo(() => {
    const groups: Record<string, Operation[]> = {}
    for (const op of filteredOperations) {
      if (!groups[op.category]) groups[op.category] = []
      groups[op.category].push(op)
    }
    return groups
  }, [filteredOperations])

  // 计算输出 - 处理异步哈希
  const calculateOutput = useCallback(async (inputText: string, ops: AddedOperation[]) => {
    let result = inputText

    for (const addedOp of ops) {
      const operation = operations.find((op) => op.id === addedOp.operationId)
      if (!operation) continue

      try {
        if (addedOp.mode === 'encode' && operation.encode) {
          result = operation.encode(result, addedOp.options)
        } else if (addedOp.mode === 'decode' && operation.decode) {
          result = operation.decode(result, addedOp.options)
        } else if (addedOp.mode === 'transform') {
          if (operation.id === 'sha256') {
            result = await sha256(result)
          } else if (operation.id === 'sha1') {
            result = await sha1(result)
          } else if (operation.id === 'sha512') {
            result = await sha512(result)
          } else if (operation.transform) {
            result = operation.transform(result, addedOp.options)
          }
        }
      } catch (error) {
        result = i18n.errorTemplate.replace('{error}', String(error))
        break
      }
    }

    return result
  }, [i18n.errorTemplate, operations])

  // 当输入或操作变化时重新计算
  useEffect(() => {
    if (activeTab) {
      calculateOutput(activeTab.input, activeTab.addedOps).then(output => {
        if (output !== activeTab.output) {
          updateActiveTab({ output })
        }
      })
    }
  }, [activeTab, activeTab?.addedOps, activeTab?.input, activeTab?.output, calculateOutput, updateActiveTab])

  // 添加操作
  const addOperation = (operationId: string, mode: 'encode' | 'decode' | 'transform') => {
    const operation = operations.find((op) => op.id === operationId)
    if (!operation || !activeTab) return

    const operationSequence = nextOperationIdRef.current
    nextOperationIdRef.current += 1
    const newOp: AddedOperation = {
      id: `${operationId}-${operationIdPrefix}-${operationSequence}`,
      operationId,
      mode,
      options: {},
    }

    if (operation.options) {
      for (const opt of operation.options) {
        newOp.options[opt.key] = opt.default
      }
    }

    updateActiveTab({ addedOps: [...activeTab.addedOps, newOp] })
  }

  // 移除操作
  const removeOperation = (id: string) => {
    if (!activeTab) return
    updateActiveTab({ addedOps: activeTab.addedOps.filter((op) => op.id !== id) })
  }

  // 清空操作
  const clearOperations = () => {
    updateActiveTab({ addedOps: [] })
  }

  // 更新操作选项
  const updateOperationOption = (opId: string, key: string, value: string) => {
    if (!activeTab) return
    updateActiveTab({
      addedOps: activeTab.addedOps.map(op =>
        op.id === opId
          ? { ...op, options: { ...op.options, [key]: value } }
          : op
      )
    })
  }

  // 快速切换支持双向处理的方法，例如加密/解密。
  const toggleOperationMode = (opId: string) => {
    if (!activeTab) return

    updateActiveTab({
      addedOps: activeTab.addedOps.map(op => {
        if (op.id !== opId) return op

        const operation = operations.find(item => item.id === op.operationId)
        if (!operation?.encode || !operation?.decode) return op

        return {
          ...op,
          mode: op.mode === 'encode' ? 'decode' : 'encode',
        }
      })
    })
  }

  // 拖拽排序处理
  const handleDragStart = (index: number) => {
    setDraggedIndex(index)
  }

  const handleDragOver = (e: React.DragEvent, index: number) => {
    e.preventDefault()
    setDragOverIndex(index)
  }

  const handleDragEnd = () => {
    if (!activeTab) return
    
    // 只处理排序逻辑
    if (draggedIndex !== null && dragOverIndex !== null && draggedIndex !== dragOverIndex) {
      const newOps = [...activeTab.addedOps]
      const [removed] = newOps.splice(draggedIndex, 1)
      newOps.splice(dragOverIndex, 0, removed)
      updateActiveTab({ addedOps: newOps })
    }
    
    setDraggedIndex(null)
    setDragOverIndex(null)
  }

  const handleDragLeave = () => {
    setDragOverIndex(null)
  }
  
  // 在外部区域 drop 时删除操作
  const handleDropOutside = (e: React.DragEvent) => {
    e.preventDefault()
    if (!activeTab || draggedIndex === null) return
    
    const newOps = [...activeTab.addedOps]
    newOps.splice(draggedIndex, 1)
    updateActiveTab({ addedOps: newOps })    
    setDraggedIndex(null)
    setDragOverIndex(null)
  }

  // 复制到剪贴板
  const handleCopy = (text: string) => {
    copyToClipboard(text)
    toast.success(i18n.copySuccess)
  }

  // 切换分类展开
  const toggleCategory = (categoryId: string) => {
    setExpandedCategories((prev) =>
      prev.includes(categoryId)
        ? prev.filter((id) => id !== categoryId)
        : [...prev, categoryId]
    )
  }

  // 添加新标签页
  const addTab = () => {
    const newTab = createNewTab(tabs.length + 1, i18n.tab)
    setTabs([...tabs, newTab])
    setActiveTabId(newTab.id)
  }

  // 关闭标签页
  const closeTab = (tabId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    if (tabs.length === 1) {
      // 至少保留一个标签页
      const newTab = createNewTab(1, i18n.tab)
      setTabs([newTab])
      setActiveTabId(newTab.id)
      return
    }
    
    const newTabs = tabs.filter(t => t.id !== tabId)
    setTabs(newTabs)
    
    // 如果关闭的是当前标签页，切换到其他标签页
    if (tabId === activeTabId) {
      const index = tabs.findIndex(t => t.id === tabId)
      const newActiveTab = newTabs[Math.min(index, newTabs.length - 1)]
      setActiveTabId(newActiveTab.id)
    }
  }

  if (!isLoaded) {
    return null // 等待加载完成
  }

  return (
    <TooltipProvider delayDuration={200}>
      <div className="flex-1 flex flex-col h-full overflow-hidden">
        {/* 标签栏 */}
        <div className="border-b border-border bg-card/50 px-2 flex items-center gap-1 overflow-x-auto flex-shrink-0">
          <div className="flex-1 overflow-x-auto scrollbar-thin scrollbar-thumb-muted scrollbar-track-transparent">
            <div className="flex items-center gap-1 py-1.5">
              {tabs.map((tab) => (
                <div
                  key={tab.id}
                  onClick={() => setActiveTabId(tab.id)}
                  className={cn(
                    "group flex items-center gap-2 px-3 py-1.5 rounded-md cursor-pointer transition-colors min-w-[100px] max-w-[180px]",
                    tab.id === activeTabId
                      ? "bg-muted text-foreground"
                      : "hover:bg-muted/50 text-muted-foreground hover:text-foreground"
                  )}
                >
                  <span className="text-sm truncate flex-1">{tab.name}</span>
                  {tab.addedOps.length > 0 && (
                    <Badge variant="secondary" className="text-[10px] h-4 px-1">
                      {tab.addedOps.length}
                    </Badge>
                  )}
                  <button
                    onClick={(e) => closeTab(tab.id, e)}
                    className="opacity-100 md:opacity-0 md:group-hover:opacity-100 hover:bg-muted rounded p-0.5 transition-opacity"
                  >
                    <X className="w-3 h-3" />
                  </button>
                </div>
              ))}
            </div>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="h-7 w-7 flex-shrink-0" onClick={addTab}>
                <Plus className="w-4 h-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{i18n.newTab}</TooltipContent>
          </Tooltip>
        </div>

        {/* 主内容区 */}
        <ResizablePanelGroup direction="horizontal" className={cn("flex-1", mobileStyles.workspace)}>
          {/* 左侧：操作列表 */}
          <ResizablePanel defaultSize={20} minSize={15} maxSize={30}>
            <div 
              className="h-full flex flex-col bg-card/50 overflow-hidden"
              onDragOver={(e) => e.preventDefault()}
              onDrop={handleDropOutside}
            >
              <div className="p-3 border-b border-border flex-shrink-0">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
                  <Input
                    placeholder={i18n.searchPlaceholder}
                    value={searchQuery}
                    onChange={(e) => setSearchQuery(e.target.value)}
                    className="pl-9 h-8 text-sm"
                  />
                </div>
              </div>
              <ScrollArea className="flex-1 overflow-hidden">
                <div className="p-2 space-y-1 overflow-hidden">
                  {categories.map((category) => {
                    const ops = groupedOperations[category.id] || []
                    if (ops.length === 0) return null
                    const isExpanded = expandedCategories.includes(category.id)

                    return (
                      <div key={category.id} className="overflow-hidden">
                        <button
                          onClick={() => toggleCategory(category.id)}
                          className="w-full flex items-center gap-2 px-2 py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground rounded-md hover:bg-muted/50 transition-colors overflow-hidden"
                        >
                          <ChevronRight className={cn(
                            "w-4 h-4 flex-shrink-0 transition-transform duration-200",
                            isExpanded && "rotate-90"
                          )} />
                          <span className="flex-shrink-0">{category.icon}</span>
                          <span className="truncate flex-1 text-left">{category.name}</span>
                          <Badge data-codec-count-badge="true" variant="secondary" className="ml-auto text-xs h-5 flex-shrink-0">
                            {ops.length}
                          </Badge>
                        </button>
                        <div className={cn(
                          "grid transition-all duration-200 ease-in-out",
                          isExpanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
                        )}>
                          <div className="overflow-hidden">
                            <div className="ml-4 mt-1 space-y-0.5 pb-1">
                              {ops.map((op) => {
                                const modes = [op.encode, op.decode, op.transform].filter(Boolean)
                                const isSingleMode = modes.length === 1
                                const singleMode = op.transform ? 'transform' : op.encode ? 'encode' : 'decode'
                                
                                return (
                                  <div
                                    key={op.id}
                                    className={cn(
                                      "group flex items-center gap-1 px-2 py-1.5 rounded-md hover:bg-muted/50 transition-colors overflow-hidden",
                                      isSingleMode && "cursor-pointer"
                                    )}
                                    onClick={isSingleMode ? () => addOperation(op.id, singleMode) : undefined}
                                    title={op.name}
                                  >
                                    <span className="flex-1 text-sm text-foreground truncate min-w-0">
                                      {op.name}
                                    </span>
                                    {!isSingleMode && (
                                      <div className="flex items-center gap-0.5 opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity flex-shrink-0">
                                        {op.encode && (
                                          <Button
                                            variant="ghost"
                                            size="icon"
                                            className="h-6 w-6"
                                            onClick={(e) => { e.stopPropagation(); addOperation(op.id, 'encode') }}
                                            title={i18n.encode}
                                          >
                                            <span className="text-xs font-medium text-emerald-400">E</span>
                                          </Button>
                                        )}
                                        {op.decode && (
                                          <Button
                                            variant="ghost"
                                            size="icon"
                                            className="h-6 w-6"
                                            onClick={(e) => { e.stopPropagation(); addOperation(op.id, 'decode') }}
                                            title={i18n.decode}
                                          >
                                            <span className="text-xs font-medium text-blue-400">D</span>
                                          </Button>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                )
                              })}
                            </div>
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </ScrollArea>
            </div>
          </ResizablePanel>

          <ResizableHandle withHandle />

          {/* 中间：已添加的操作 */}
          <ResizablePanel defaultSize={20} minSize={12} maxSize={30}>
            <div className="h-full w-full flex flex-col bg-card/30 overflow-hidden">
              <div className="px-3 py-2 border-b border-border flex-shrink-0 overflow-hidden">
                <div className="flex items-center gap-2 overflow-hidden">
                  <ArrowRight className="w-4 h-4 text-primary flex-shrink-0" />
                  <span className="text-sm font-medium text-foreground truncate flex-1 min-w-0">
                    {i18n.workflow}
                  </span>
                  {activeTab && activeTab.addedOps.length > 0 && (
                    <Badge data-codec-count-badge="true" variant="secondary" className="text-xs flex-shrink-0">
                      {activeTab.addedOps.length}
                    </Badge>
                  )}
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button 
                        variant="ghost" 
                        size="icon" 
                        className="h-6 w-6 flex-shrink-0" 
                        onClick={clearOperations} 
                        disabled={!activeTab || activeTab.addedOps.length === 0}
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>{i18n.clearOperations}</TooltipContent>
                  </Tooltip>
                </div>
              </div>
              <div className="flex-1 overflow-y-auto overflow-x-hidden">
                <div className="p-2 space-y-1.5 w-full">
                  {!activeTab || activeTab.addedOps.length === 0 ? (
                    <div className="text-center py-8 text-muted-foreground">
                      <Wand2 className="w-8 h-8 mx-auto mb-2 opacity-30" />
                      <p className="text-xs">{i18n.empty.addFromLeft}</p>
                      <p className="text-xs opacity-60 mt-1">{i18n.empty.chainSupported}</p>
                      <p className="text-xs opacity-60 mt-1">{i18n.empty.dragToDelete}</p>
                    </div>
                  ) : (
                    activeTab.addedOps.map((addedOp, index) => {
                      const operation = operations.find((op) => op.id === addedOp.operationId)
                      if (!operation) return null

                      const modeLabel =
                        addedOp.mode === 'encode' ? i18n.encode : addedOp.mode === 'decode' ? i18n.decode : i18n.transform
                      const modeColor =
                        addedOp.mode === 'encode'
                          ? 'text-emerald-400 bg-emerald-500/20'
                          : addedOp.mode === 'decode'
                          ? 'text-blue-400 bg-blue-500/20'
                          : 'text-amber-400 bg-amber-500/20'
                      const canToggleMode = Boolean(operation.encode && operation.decode)

                      return (
                        <div
                          key={addedOp.id}
                          draggable
                          onDragStart={() => handleDragStart(index)}
                          onDragOver={(e) => handleDragOver(e, index)}
                          onDragEnd={handleDragEnd}
                          onDragLeave={handleDragLeave}
                          className={cn(
                            "group p-2 rounded-lg bg-muted/30 border transition-all cursor-grab active:cursor-grabbing w-full overflow-hidden",
                            draggedIndex === index && "opacity-50 border-primary",
                            dragOverIndex === index && draggedIndex !== index && "border-primary border-dashed",
                            draggedIndex === null && "border-border/50 hover:border-border"
                          )}
                        >
                          <div className="flex items-center gap-1.5 w-full overflow-hidden" title={operation.name}>
                            <GripVertical className="w-3.5 h-3.5 text-muted-foreground flex-shrink-0" />
                            <div className="flex items-center justify-center w-5 h-5 rounded bg-muted text-xs font-medium text-muted-foreground flex-shrink-0">
                              {index + 1}
                            </div>
                            <span className="text-sm font-medium text-foreground truncate flex-1 min-w-0">
                              {operation.name}
                            </span>
                            {canToggleMode ? (
                              <button
                                type="button"
                                draggable={false}
                                onClick={(e) => {
                                  e.stopPropagation()
                                  toggleOperationMode(addedOp.id)
                                }}
                                onMouseDown={(e) => e.stopPropagation()}
                                className={cn(
                                  'inline-flex h-4 flex-shrink-0 items-center gap-0.5 rounded-full px-1 text-[10px] font-semibold leading-none whitespace-nowrap transition-colors hover:ring-1 hover:ring-current/30 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
                                  modeColor
                                )}
                                title={`${i18n.encode} / ${i18n.decode}`}
                              >
                                <Repeat2 className="h-2.5 w-2.5" />
                                <span>{modeLabel}</span>
                              </button>
                            ) : (
                              <Badge className={cn('text-[10px] h-4 px-1 flex-shrink-0 whitespace-nowrap', modeColor)}>
                                {modeLabel}
                              </Badge>
                            )}
                            <Button
                              variant="ghost"
                              size="icon"
                              className="h-6 w-6 opacity-100 md:opacity-0 md:group-hover:opacity-100 transition-opacity flex-shrink-0"
                              onClick={() => removeOperation(addedOp.id)}
                            >
                              <X className="w-3 h-3" />
                            </Button>
                          </div>
                          {operation.options && operation.options.length > 0 && (
                            <div className="mt-2 space-y-1.5 overflow-hidden">
                              {operation.options.filter((opt) => {
                                if (!opt.visibleWhen) return true
                                return addedOp.options[opt.visibleWhen.key] === opt.visibleWhen.value
                              }).map((opt) => (
                                <div key={opt.key} className={cn(
                                  'grid grid-cols-[72px_minmax(0,1fr)] gap-x-2 gap-y-1 overflow-hidden',
                                  opt.type === 'textarea' ? 'items-start' : 'items-center',
                                )}>
                                  <label className="block min-w-0 text-[10px] text-muted-foreground truncate" title={opt.label}>
                                    {opt.label}
                                  </label>
                                  {opt.type === 'select' && opt.options ? (
                                    <Select
                                      value={addedOp.options[opt.key] || opt.default}
                                      onValueChange={(value) => updateOperationOption(addedOp.id, opt.key, value)}
                                    >
                                      <SelectTrigger data-codec-option-control="true" className="h-7 text-xs w-full min-w-0 focus-visible:ring-0 focus-visible:ring-offset-0">
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        {opt.options.map((option) => {
                                          const optionValue = typeof option === 'string' ? option : option.value
                                          const optionLabel = typeof option === 'string' ? option : option.label

                                          return (
                                            <SelectItem key={optionValue} value={optionValue} className="text-xs">
                                              {optionLabel}
                                            </SelectItem>
                                          )
                                        })}
                                      </SelectContent>
                                    </Select>
                                  ) : opt.type === 'switch' ? (
                                    <Switch
                                      checked={addedOp.options[opt.key] === 'true'}
                                      onCheckedChange={(checked) => updateOperationOption(addedOp.id, opt.key, String(checked))}
                                      aria-label={opt.label}
                                    />
                                  ) : opt.type === 'textarea' ? (
                                    <Textarea
                                      data-codec-option-control="true"
                                      value={addedOp.options[opt.key] || opt.default}
                                      onChange={(e) => updateOperationOption(addedOp.id, opt.key, e.target.value)}
                                      className="h-20 resize-none overflow-auto text-xs w-full min-w-0 focus-visible:ring-0 focus-visible:ring-offset-0"
                                      placeholder={opt.default}
                                    />
                                  ) : (
                                    <Input
                                      data-codec-option-control="true"
                                      value={addedOp.options[opt.key] || opt.default}
                                      onChange={(e) => updateOperationOption(addedOp.id, opt.key, e.target.value)}
                                      className="h-7 text-xs w-full min-w-0 focus-visible:ring-0 focus-visible:ring-offset-0"
                                      placeholder={opt.default}
                                    />
                                  )}
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )
                    })
                  )}
                </div>
              </div>
            </div>
          </ResizablePanel>

          <ResizableHandle withHandle />

          {/* 右侧：输入输出 */}
          <ResizablePanel defaultSize={60} minSize={40}>
            <div 
              className="h-full"
              onDragOver={(e) => e.preventDefault()}
              onDrop={handleDropOutside}
            >
            <ResizablePanelGroup direction="vertical" className="h-full">
              {/* 输入区 */}
              <ResizablePanel defaultSize={50} minSize={20}>
                <div className="h-full flex flex-col">
                  <div className="px-4 py-2 border-b border-border flex items-center justify-between bg-card/50 flex-shrink-0">
                    <span className="text-sm font-medium text-foreground">{i18n.input}</span>
                    <div className="flex items-center gap-1">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => updateActiveTab({ input: '' })}
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{i18n.clear}</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => handleCopy(activeTab?.input || '')}
                          >
                            <Copy className="w-3.5 h-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{i18n.copy}</TooltipContent>
                      </Tooltip>
                    </div>
                  </div>
                  <Textarea
                    value={activeTab?.input || ''}
                    onChange={(e) => updateActiveTab({ input: e.target.value })}
                    placeholder={i18n.inputPlaceholder}
                    className="flex-1 resize-none border-0 rounded-none focus-visible:ring-0 font-mono text-sm"
                  />
                </div>
              </ResizablePanel>

              <ResizableHandle withHandle />

              {/* 输出区 */}
              <ResizablePanel defaultSize={50} minSize={20}>
                <div className="h-full flex flex-col">
                  <div className="px-4 py-2 border-b border-border flex items-center justify-between bg-card/50 flex-shrink-0">
                    <span className="text-sm font-medium text-foreground">{i18n.output}</span>
                    <div className="flex items-center gap-1">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => updateActiveTab({ input: activeTab?.output || '' })}
                            disabled={!activeTab?.output}
                          >
                            <ArrowUp className="w-3.5 h-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{i18n.useAsInput}</TooltipContent>
                      </Tooltip>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7"
                            onClick={() => handleCopy(activeTab?.output || '')}
                            disabled={!activeTab?.output}
                          >
                            <Copy className="w-3.5 h-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{i18n.copy}</TooltipContent>
                      </Tooltip>
                    </div>
                  </div>
                  <Textarea
                    value={activeTab?.output || ''}
                    readOnly
                    placeholder={i18n.outputPlaceholder}
                    className="flex-1 resize-none border-0 rounded-none focus-visible:ring-0 font-mono text-sm bg-muted/20"
                  />
                </div>
              </ResizablePanel>
            </ResizablePanelGroup>
            </div>
          </ResizablePanel>
        </ResizablePanelGroup>
      </div>
    </TooltipProvider>
  )
}
