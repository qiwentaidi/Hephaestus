"use client"

import * as React from 'react'
import { ChevronsUpDown, X, Search } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useVirtualizer } from '@tanstack/react-virtual'

export interface MultiSelectOption {
  value: string
  label: string
  group?: string
}

interface SimpleMultiSelectProps {
  options: MultiSelectOption[]
  selected: string[]
  onSelectionChange: (selected: string[]) => void
  placeholder?: string
  searchPlaceholder?: string
  emptyText?: string
  className?: string
  disabled?: boolean
  showSelectedBadges?: boolean
}

export function SimpleMultiSelect({
  options,
  selected,
  onSelectionChange,
  placeholder = '选择选项',
  searchPlaceholder = '搜索...',
  emptyText = '未找到选项',
  className,
  disabled = false,
  showSelectedBadges = true,
}: SimpleMultiSelectProps) {
  const [open, setOpen] = React.useState(false)
  const triggerRef = React.useRef<HTMLButtonElement>(null)
  const parentRef = React.useRef<HTMLDivElement>(null)
  const [popoverWidth, setPopoverWidth] = React.useState<number | undefined>(undefined)
  const [searchQuery, setSearchQuery] = React.useState('')

  // 预先计算宽度
  React.useEffect(() => {
    const updateWidth = () => {
      if (triggerRef.current) {
        setPopoverWidth(triggerRef.current.offsetWidth)
      }
    }
    updateWidth()
    window.addEventListener('resize', updateWidth)
    return () => window.removeEventListener('resize', updateWidth)
  }, [])

  // 按组分类选项
  const groupedOptions = React.useMemo(() => {
    if (!options || options.length === 0) {
      return {}
    }
    const groups: Record<string, MultiSelectOption[]> = {}
    options.forEach((option) => {
      const group = option.group || '其他'
      if (!groups[group]) {
        groups[group] = []
      }
      groups[group].push(option)
    })
    return groups
  }, [options])

  // 过滤选项
  const filteredGroups = React.useMemo(() => {
    if (!searchQuery.trim()) {
      return groupedOptions
    }
    const query = searchQuery.toLowerCase()
    const filtered: Record<string, MultiSelectOption[]> = {}
    Object.entries(groupedOptions).forEach(([group, groupOptions]) => {
      const matched = groupOptions.filter(
        (option) =>
          option.label.toLowerCase().includes(query) ||
          option.value.toLowerCase().includes(query)
      )
      if (matched.length > 0) {
        filtered[group] = matched
      }
    })
    return filtered
  }, [groupedOptions, searchQuery])

  // 将分组数据展平为虚拟列表项（包含分组标题和选项）
  interface VirtualItem {
    type: 'group' | 'option'
    group?: string
    option?: MultiSelectOption
    index: number
  }

  const virtualItems = React.useMemo<VirtualItem[]>(() => {
    const items: VirtualItem[] = []
    let index = 0
    Object.entries(filteredGroups).forEach(([group, groupOptions]) => {
      // 添加分组标题
      items.push({ type: 'group', group, index: index++ })
      // 添加该组下的所有选项
      groupOptions.forEach((option) => {
        items.push({ type: 'option', group, option, index: index++ })
      })
    })
    return items
  }, [filteredGroups])

  // 虚拟滚动配置
  // TanStack Virtual returns mutable helpers that React Compiler intentionally skips.
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count: virtualItems.length,
    getScrollElement: () => parentRef.current,
    estimateSize: (index: number) => {
      if (index >= virtualItems.length || index < 0) return 36
      const item = virtualItems[index]
      if (!item) return 36
      return item.type === 'group' ? 32 : 36 // 分组标题 32px，选项 36px
    },
    overscan: 10, // 预渲染10个额外项，提升滚动体验
  })

  // 当 Popover 打开时，确保虚拟滚动重新计算
  React.useEffect(() => {
    if (open && virtualItems.length > 0) {
      // 延迟一下，确保 DOM 已经渲染
      const timer = setTimeout(() => {
        if (parentRef.current) {
          virtualizer.measure()
        }
      }, 50)
      return () => clearTimeout(timer)
    }
  }, [open, virtualItems.length, virtualizer])

  const handleSelect = (value: string) => {
    if (selected.includes(value)) {
      onSelectionChange(selected.filter((v) => v !== value))
    } else {
      onSelectionChange([...selected, value])
    }
  }

  const handleRemove = (value: string) => {
    onSelectionChange(selected.filter((v) => v !== value))
  }

  const displayText = selected.length > 0 
    ? `已选择 ${selected.length} 个选项`
    : placeholder

  const totalOptions = virtualItems.filter(item => item.type === 'option').length

  // 调试信息（开发环境）
  React.useEffect(() => {
    if (process.env.NODE_ENV === 'development') {
      console.log('SimpleMultiSelect Debug:', {
        optionsCount: options.length,
        virtualItemsCount: virtualItems.length,
        totalOptions,
        groupedOptionsKeys: Object.keys(groupedOptions),
      })
    }
  }, [options.length, virtualItems.length, totalOptions, groupedOptions])

  return (
    <div className={cn('space-y-2', className)}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            ref={triggerRef}
            variant="outline"
            role="combobox"
            aria-expanded={open}
            className="w-full justify-between"
            disabled={disabled}
          >
            {displayText}
            <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent 
          className="p-0 w-auto" 
          align="start"
          sideOffset={4}
          style={popoverWidth ? { width: `${popoverWidth}px`, minWidth: `${popoverWidth}px`, maxWidth: `${popoverWidth}px` } : undefined}
        >
          <div className="flex flex-col">
            {/* 搜索框 */}
            <div className="p-2 border-b">
              <div className="relative">
                <Search className="absolute left-2 top-1/2 transform -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder={searchPlaceholder}
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-8"
                  autoFocus
                />
              </div>
            </div>

            {/* 选项列表 - 使用虚拟滚动 */}
            {virtualItems.length === 0 ? (
              <div className="p-4 text-center text-sm text-muted-foreground min-h-[100px]">
                {emptyText}
              </div>
            ) : (
              <div
                ref={parentRef}
                className="h-[300px] overflow-auto"
                style={{ minHeight: '300px' }}
              >
                {(() => {
                  // 确保 parentRef 已经设置
                  if (!parentRef.current) {
                    return (
                      <div className="p-4 text-center text-sm text-muted-foreground">
                        正在加载...
                      </div>
                    )
                  }

                  const virtualItemsToRender = virtualizer.getVirtualItems()
                  const totalSize = virtualizer.getTotalSize()
                  
                  if (totalSize === 0 || virtualItemsToRender.length === 0) {
                    // 如果虚拟滚动还没有准备好，直接渲染所有项（降级方案）
                    return (
                      <div className="p-1">
                        {Object.entries(filteredGroups).map(([group, groupOptions]) => (
                          <div key={group} className="mb-3">
                            <div className="px-2 py-2 text-xs font-semibold text-muted-foreground">
                              {group}
                            </div>
                            {groupOptions.map((option) => {
                              const isSelected = selected.includes(option.value)
                              return (
                                <div
                                  key={option.value}
                                  className={cn(
                                    "relative flex items-center gap-2 px-2 py-1.5 text-sm rounded-sm cursor-pointer select-none outline-none",
                                    "hover:bg-accent hover:text-accent-foreground"
                                  )}
                                  onClick={() => handleSelect(option.value)}
                                >
                                  <Checkbox
                                    checked={isSelected}
                                    onCheckedChange={() => handleSelect(option.value)}
                                    onClick={(e) => e.stopPropagation()}
                                    className="shrink-0"
                                  />
                                  <span className="flex-1">{option.label}</span>
                                </div>
                              )
                            })}
                          </div>
                        ))}
                      </div>
                    )
                  }
                  
                  return (
                    <div
                      style={{
                        height: `${totalSize}px`,
                        width: '100%',
                        position: 'relative',
                      }}
                    >
                      {virtualItemsToRender.map((virtualItem) => {
                        const item = virtualItems[virtualItem.index]
                        if (!item) return null
                        
                        if (item.type === 'group') {
                          return (
                            <div
                              key={`group-${item.group}-${virtualItem.index}`}
                              style={{
                                position: 'absolute',
                                top: 0,
                                left: 0,
                                width: '100%',
                                height: `${virtualItem.size}px`,
                                transform: `translateY(${virtualItem.start}px)`,
                              }}
                              className="px-2 py-2 text-xs font-semibold text-muted-foreground"
                            >
                              {item.group}
                            </div>
                          )
                        } else {
                          const isSelected = selected.includes(item.option!.value)
                          return (
                            <div
                              key={`option-${item.option!.value}-${virtualItem.index}`}
                              style={{
                                position: 'absolute',
                                top: 0,
                                left: 0,
                                width: '100%',
                                height: `${virtualItem.size}px`,
                                transform: `translateY(${virtualItem.start}px)`,
                              }}
                              className={cn(
                                "relative flex items-center gap-2 px-2 py-1.5 text-sm rounded-sm cursor-pointer select-none outline-none",
                                "hover:bg-accent hover:text-accent-foreground"
                              )}
                              onClick={() => handleSelect(item.option!.value)}
                            >
                              <Checkbox
                                checked={isSelected}
                                onCheckedChange={() => handleSelect(item.option!.value)}
                                onClick={(e) => e.stopPropagation()}
                                className="shrink-0"
                              />
                              <span className="flex-1">{item.option!.label}</span>
                            </div>
                          )
                        }
                      })}
                    </div>
                  )
                })()}
              </div>
            )}
          </div>
        </PopoverContent>
      </Popover>
      
      {showSelectedBadges && selected.length > 0 && (
        <div className="flex flex-wrap gap-2 mt-2">
          {selected.map((value) => {
            const option = options.find((opt) => opt.value === value)
            return (
              <Badge 
                key={value} 
                variant="secondary" 
                className="flex items-center gap-1.5 px-2 py-1"
              >
                {option?.label || value}
                <button
                  type="button"
                  onClick={() => handleRemove(value)}
                  className="ml-0.5 hover:text-destructive transition-colors"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      handleRemove(value)
                    }
                  }}
                >
                  <X className="w-3 h-3" />
                </button>
              </Badge>
            )
          })}
        </div>
      )}

    </div>
  )
}
