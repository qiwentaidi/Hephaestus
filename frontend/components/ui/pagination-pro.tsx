'use client'

import { useState } from 'react'
import { ChevronLeft, ChevronRight, MoreHorizontal } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'
import { formatWorkbenchText } from '@/lib/ai-workbench-i18n'

export interface PaginationProProps {
  /** 当前页码 */
  current: number
  /** 总条数 */
  total: number
  /** 每页条数 */
  pageSize: number
  /** 页码改变回调 */
  onChange?: (page: number, pageSize: number) => void
  /** 每页条数改变回调 */
  onPageSizeChange?: (pageSize: number) => void
  /** 每页条数选项 */
  pageSizeOptions?: number[]
  /** 是否显示每页条数选择器 */
  showSizeChanger?: boolean
  /** 是否显示快速跳转（仅桌面端） */
  showQuickJumper?: boolean
  /** 是否显示总条数 */
  showTotal?: boolean
  /** 中间连续显示的页码数量，默认 5 */
  middlePageCount?: number
  /** 紧凑模式：显示精简页码与前后翻页，适合狭窄侧栏 */
  compact?: boolean
  /** 是否禁用 */
  disabled?: boolean
  /** 自定义类名 */
  className?: string
}

export function PaginationPro({
  current,
  total,
  pageSize,
  onChange,
  onPageSizeChange,
  pageSizeOptions = [10, 20, 50, 100],
  showSizeChanger = true,
  showQuickJumper = false,
  showTotal = true,
  middlePageCount = 5,
  compact = false,
  disabled = false,
  className,
}: PaginationProProps) {
  const { t } = useI18n()
  const l = t.pagination
  const [jumpValue, setJumpValue] = useState('')

  const totalPages = Math.ceil(total / pageSize) || 1

  const handlePageChange = (page: number) => {
    if (page < 1 || page > totalPages || page === current || disabled) return
    onChange?.(page, pageSize)
  }

  const handlePageSizeChange = (newSize: string) => {
    const size = parseInt(newSize, 10)
    onPageSizeChange?.(size)
    // 页大小变更由 onPageSizeChange 自行决定是否重置到第一页，
    // 避免同时触发 onChange 导致重复请求。
    if (!onPageSizeChange) {
      onChange?.(1, size)
    }
  }

  const handleJump = () => {
    const page = parseInt(jumpValue, 10)
    if (!isNaN(page) && page >= 1 && page <= totalPages) {
      handlePageChange(page)
    }
    setJumpValue('')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      handleJump()
    }
  }

  // 生成页码列表
  const generatePages = (): (number | 'prev' | 'next')[] => {
    const pages: (number | 'prev' | 'next')[] = []
    const normalizedMiddlePageCount = Math.max(1, Math.floor(middlePageCount))

    if (totalPages <= normalizedMiddlePageCount + 2) {
      for (let i = 1; i <= totalPages; i++) {
        pages.push(i)
      }
      return pages
    }

    // 始终显示第1页
    pages.push(1)

    const pagesBeforeCurrent = Math.floor((normalizedMiddlePageCount - 1) / 2)
    const pagesAfterCurrent = normalizedMiddlePageCount - pagesBeforeCurrent - 1

    if (current > pagesBeforeCurrent + 2) {
      pages.push('prev')
    }

    let start = Math.max(2, current - pagesBeforeCurrent)
    let end = Math.min(totalPages - 1, current + pagesAfterCurrent)

    if (normalizedMiddlePageCount === 5) {
      // Preserve the existing default windowing behavior for all current callers.
      if (current <= 4) {
        end = Math.min(5, totalPages - 1)
      } else if (current >= totalPages - 3) {
        start = Math.max(2, totalPages - 4)
      }
    } else if (current <= pagesBeforeCurrent + 1) {
      start = 2
      end = Math.min(normalizedMiddlePageCount, totalPages - 1)
    } else if (current >= totalPages - pagesAfterCurrent) {
      end = totalPages - 1
      start = Math.max(2, end - normalizedMiddlePageCount + 1)
    }

    for (let i = start; i <= end; i++) {
      pages.push(i)
    }

    if (current < totalPages - pagesAfterCurrent - 1) {
      pages.push('next')
    }

    // 始终显示最后一页
    if (totalPages > 1) {
      pages.push(totalPages)
    }

    return pages
  }

  const pages = generatePages()

  return (
    <div className={cn('flex items-center justify-end gap-4 flex-wrap', className)}>
      {/* 总条数 */}
      {showTotal && (
        <div className="text-sm text-muted-foreground whitespace-nowrap">
          {formatWorkbenchText(total === 1 ? l.totalOne : l.total, { count: total })}
        </div>
      )}

      <div className="flex items-center gap-2 flex-wrap">
        {/* 分页按钮 */}
        <div className="flex items-center gap-1">
          {/* 上一页 */}
          <Button
            variant="outline"
            size="icon"
            className={cn('shrink-0', compact ? 'h-9 w-9 md:h-7 md:w-7' : 'h-9 w-9 md:h-8 md:w-8')}
            onClick={() => handlePageChange(current - 1)}
            disabled={current === 1 || disabled}
            aria-label={l.previous}
          >
            <ChevronLeft className="h-4 w-4" />
          </Button>

          {/* 页码 */}
          <span className="min-w-16 text-center text-sm tabular-nums md:hidden" aria-live="polite">{current} / {totalPages}</span>
          <div className="hidden items-center gap-1 md:flex">
          {pages.map((page, idx) => {
            if (page === 'prev') {
              return (
                <Button
                  key={`prev-${idx}`}
                  variant="outline"
                  size="icon"
                  className={cn('shrink-0', compact ? 'h-9 w-9 md:h-7 md:w-7' : 'h-9 w-9 md:h-8 md:w-8')}
                  onClick={() => handlePageChange(Math.max(1, current - 5))}
                  disabled={disabled}
                  title={l.previousFive}
                  aria-label={l.previousFive}
                >
                  <MoreHorizontal className="h-4 w-4" />
                </Button>
              )
            }

            if (page === 'next') {
              return (
                <Button
                  key={`next-${idx}`}
                  variant="outline"
                  size="icon"
                  className={cn('shrink-0', compact ? 'h-9 w-9 md:h-7 md:w-7' : 'h-9 w-9 md:h-8 md:w-8')}
                  onClick={() => handlePageChange(Math.min(totalPages, current + 5))}
                  disabled={disabled}
                  title={l.nextFive}
                  aria-label={l.nextFive}
                >
                  <MoreHorizontal className="h-4 w-4" />
                </Button>
              )
            }

            return (
              <Button
                key={page}
                variant={current === page ? 'default' : 'outline'}
                size="sm"
                className={cn(
                  'shrink-0',
                  compact ? 'h-7 min-w-7 px-1 text-xs' : 'h-8 min-w-[32px] px-2',
                  current === page && 'pointer-events-none'
                )}
                onClick={() => handlePageChange(page)}
                disabled={disabled}
                aria-label={formatWorkbenchText(l.page, { count: page })}
                aria-current={current === page ? 'page' : undefined}
              >
                {page}
              </Button>
            )
          })}

          </div>

          {/* 下一页 */}
          <Button
            variant="outline"
            size="icon"
            className={cn('shrink-0', compact ? 'h-9 w-9 md:h-7 md:w-7' : 'h-9 w-9 md:h-8 md:w-8')}
            onClick={() => handlePageChange(current + 1)}
            disabled={current === totalPages || disabled}
            aria-label={l.next}
          >
            <ChevronRight className="h-4 w-4" />
          </Button>
        </div>

        {/* 每页条数选择器 */}
        {showSizeChanger && (
          <div className="hidden md:block">
            <Select
              value={pageSize.toString()}
              onValueChange={handlePageSizeChange}
              disabled={disabled}
            >
              <SelectTrigger className="h-11 w-[120px] md:h-8">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={size.toString()}>
                    {formatWorkbenchText(l.perPage, { count: size })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* 快速跳转 */}
        {showQuickJumper && (
          <div className="hidden items-center gap-1 text-sm md:flex">
            <span className="text-muted-foreground">{l.jumpTo}</span>
            <Input
              className="h-11 w-14 text-center text-base md:h-8 md:text-sm"
              inputMode="numeric"
              value={jumpValue}
              onChange={(e) => setJumpValue(e.target.value.replace(/\D/g, ''))}
              onKeyDown={handleKeyDown}
              onBlur={handleJump}
              disabled={disabled}
              aria-label={l.jumpInput}
            />
            {l.pageSuffix && <span className="text-muted-foreground">{l.pageSuffix}</span>}
          </div>
        )}
      </div>
    </div>
  )
}
