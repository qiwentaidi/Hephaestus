'use client'

import { type UIEvent, useCallback, useMemo, useRef, useState } from 'react'
import { Copy, Plus, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import toast from '@/lib/toast'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

interface CompareTab {
  id: string
  name: string
  text1: string
  text2: string
}

type DiffType = 'equal' | 'delete' | 'insert' | 'modify'
type CompareSide = 'left' | 'right'

const compareEditorTextInset = '3.5rem'

function getDiffLineTone(type: DiffType, side: CompareSide) {
  if (type === 'equal') {
    return {
      row: 'bg-transparent',
      gutter: 'bg-muted/25 text-muted-foreground/65',
      marker: '',
      markerTone: 'text-muted-foreground/40',
    }
  }

  if (type === 'modify') {
    return side === 'left'
      ? {
          row: 'bg-red-500/12',
          gutter: 'bg-red-500/15 text-red-700 dark:text-red-300',
          marker: '-',
          markerTone: 'text-red-700 dark:text-red-300',
        }
      : {
          row: 'bg-emerald-500/12',
          gutter: 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300',
          marker: '+',
          markerTone: 'text-emerald-700 dark:text-emerald-300',
        }
  }

  if (type === 'delete') {
    return {
      row: 'bg-red-500/14',
      gutter: 'bg-red-500/18 text-red-700 dark:text-red-300',
      marker: '-',
      markerTone: 'text-red-700 dark:text-red-300',
    }
  }

  return {
    row: 'bg-emerald-500/14',
    gutter: 'bg-emerald-500/18 text-emerald-700 dark:text-emerald-300',
    marker: '+',
    markerTone: 'text-emerald-700 dark:text-emerald-300',
  }
}

function computeCharDiff(line: string, otherLine: string, side: CompareSide) {
  const chars = line.split('')
  const otherChars = otherLine.split('')
  const maxLen = Math.max(chars.length, otherChars.length)
  const diff: Array<{ char: string; type: DiffType }> = []

  for (let i = 0; i < maxLen; i++) {
    const char = i < chars.length ? chars[i] : null
    const otherChar = i < otherChars.length ? otherChars[i] : null

    if (char === null) continue

    if (otherChar === null) {
      diff.push({ char, type: side === 'left' ? 'delete' : 'insert' })
    } else if (char === otherChar) {
      diff.push({ char, type: 'equal' })
    } else {
      diff.push({ char, type: 'modify' })
    }
  }

  return diff
}

function computeLineDiff(value: string, otherValue: string, side: CompareSide) {
  const lines = value.split('\n')
  const otherLines = otherValue.split('\n')

  return lines.map((line, index) => {
    const otherLine = index < otherLines.length ? otherLines[index] : null
    let type: DiffType = 'equal'

    if (otherLine === null) {
      type = side === 'left' ? 'delete' : 'insert'
    } else if (line !== otherLine) {
      type = 'modify'
    }

    return {
      line,
      otherLine,
      type,
      chars:
        type === 'modify' && otherLine !== null
          ? computeCharDiff(line, otherLine, side)
          : null,
    }
  })
}

function HighlightedCompareEditor({
  value,
  otherValue,
  side,
  placeholder,
  className,
  onChange,
}: {
  value: string
  otherValue: string
  side: CompareSide
  placeholder: string
  className?: string
  onChange: (value: string) => void
}) {
  const highlightRef = useRef<HTMLDivElement>(null)
  const lineDiff = useMemo(() => computeLineDiff(value, otherValue, side), [otherValue, side, value])

  const syncScroll = useCallback((event: UIEvent<HTMLTextAreaElement>) => {
    if (!highlightRef.current) return
    highlightRef.current.scrollTop = event.currentTarget.scrollTop
    highlightRef.current.scrollLeft = event.currentTarget.scrollLeft
  }, [])

  return (
    <div className={cn('relative min-h-0 overflow-hidden rounded-md bg-background', className)}>
      <div
        ref={highlightRef}
        aria-hidden="true"
        className="m-0 h-full overflow-auto font-mono text-sm leading-6 whitespace-pre-wrap break-words"
      >
        {value ? (
          lineDiff.map((item, index) => {
            const tone = getDiffLineTone(item.type, side)

            return (
              <div key={index} className={cn('grid min-h-6 grid-cols-[2.75rem_minmax(0,1fr)]', tone.row)}>
                <div className={cn('flex select-none items-center pr-2 text-xs', tone.gutter)}>
                  <span className={cn('w-3 text-center font-semibold', tone.markerTone)}>{tone.marker}</span>
                  <span className="ml-auto tabular-nums">{index + 1}</span>
                </div>
                <div className="min-w-0 px-3 text-foreground">
                  {item.chars ? (
                    item.chars.map((charItem, charIndex) => (
                      <span
                        key={charIndex}
                        className={cn(
                          side === 'left' &&
                            (charItem.type === 'delete' || charItem.type === 'modify') &&
                            'rounded-[2px] bg-red-500/28',
                          side === 'right' &&
                            (charItem.type === 'insert' || charItem.type === 'modify') &&
                            'rounded-[2px] bg-emerald-500/28',
                        )}
                      >
                        {charItem.char}
                      </span>
                    ))
                  ) : item.line ? (
                    <span>{item.line}</span>
                  ) : (
                    <span>&#8203;</span>
                  )}
                </div>
              </div>
            )
          })
        ) : (
          <div className="py-0 pr-3 font-mono text-sm leading-6 text-muted-foreground" style={{ paddingLeft: compareEditorTextInset }}>
            {placeholder}
          </div>
        )}
      </div>
      <Textarea
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onScroll={syncScroll}
        spellCheck={false}
        aria-label={placeholder}
        className="absolute inset-0 z-10 h-full min-h-0 resize-none border-0 bg-transparent py-0 pr-3 font-mono text-sm leading-6 text-foreground caret-foreground focus-visible:ring-0"
        style={{
          margin: 0,
          background: 'none',
          boxSizing: 'border-box',
          paddingLeft: compareEditorTextInset,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          overflowWrap: 'break-word',
          overflow: 'auto',
          WebkitFontSmoothing: 'antialiased',
          WebkitTextFillColor: 'transparent',
        }}
      />
    </div>
  )
}

export default function TextComparePage() {
  const { t } = useI18n()
  const tabPrefix = t.tools.textCompare.tabPrefix
  const [tabs, setTabs] = useState<CompareTab[]>([
    { id: '1', name: `${tabPrefix}-1`, text1: '', text2: '' },
  ])
  const [activeTab, setActiveTab] = useState('1')

  const handleCopy = async (text: string, label: string) => {
    if (text) {
      await copyToClipboard(text)
      toast.success({ title: t.tools.textCompare.copySuccessTitle, message: `${label}${t.common.copiedToClipboard}` })
    } else {
      toast.warning({ title: t.common.warning, message: t.tools.textCompare.noContentToCopy })
    }
  }

  const handleAddTab = () => {
    const newId = String(Date.now())
    // 从现有标签页名称中提取最大编号
    const maxNumber = tabs.reduce((max, tab) => {
      const match = tab.name.match(/-(\d+)$/)
      if (match) {
        const num = parseInt(match[1], 10)
        return Math.max(max, num)
      }
      return max
    }, 0)
    const newTab: CompareTab = {
      id: newId,
      name: `${tabPrefix}-${maxNumber + 1}`,
      text1: '',
      text2: '',
    }
    setTabs([...tabs, newTab])
    setActiveTab(newId)
  }

  const handleRemoveTab = (id: string) => {
    if (tabs.length === 1) {
      toast.warning({ title: t.common.warning, message: t.tools.textCompare.keepOneTab })
      return
    }
    const newTabs = tabs.filter((tab) => tab.id !== id)
    setTabs(newTabs)
    if (activeTab === id) {
      setActiveTab(newTabs[0].id)
    }
  }

  const handleUpdateTab = (id: string, updates: Partial<CompareTab>) => {
    setTabs(tabs.map((tab) => (tab.id === id ? { ...tab, ...updates } : tab)))
  }

  return (
    <div className="flex-1 flex flex-col h-full overflow-hidden">
      {/* 标签栏 */}
      <div className="border-b border-border bg-card/50 px-3 flex items-center gap-2 overflow-x-auto flex-shrink-0">
        <div className="flex-1 overflow-x-auto scrollbar-thin scrollbar-thumb-muted scrollbar-track-transparent">
          <div className="flex items-center gap-2 py-2">
            {tabs.map((tab) => (
              <div
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={`group flex items-center gap-2 px-4 py-2 rounded-md cursor-pointer transition-colors min-w-[120px] max-w-[200px] ${
                  tab.id === activeTab
                    ? "bg-muted text-foreground"
                    : "hover:bg-muted/50 text-muted-foreground hover:text-foreground"
                }`}
              >
                <span className="text-sm truncate flex-1">{tab.name}</span>
                {tabs.length > 1 && (
                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      handleRemoveTab(tab.id)
                    }}
                    className="opacity-100 md:opacity-0 md:group-hover:opacity-100 hover:bg-muted rounded p-0.5 transition-opacity"
                  >
                    <X className="w-3 h-3" />
                  </button>
                )}
              </div>
            ))}
          </div>
        </div>
        <Button variant="ghost" size="icon" className="h-11 w-11 md:h-8 md:w-8 flex-shrink-0" onClick={handleAddTab}>
          <Plus className="w-4 h-4" />
        </Button>
      </div>

      {/* 主内容区 */}
      <div className="flex-1 min-h-0 overflow-hidden">
        <div className="h-full">
          {tabs.map((tab) => (
            <div key={tab.id} className={tab.id === activeTab ? 'flex h-full min-h-0 flex-col' : 'hidden'}>
              {/* 输入区域 */}
              <div className="grid min-h-0 flex-1 grid-cols-1 grid-rows-2 gap-3 md:grid-cols-2 md:grid-rows-1">
                {/* 左侧文本 */}
                <div className="flex min-h-0 flex-col bg-card">
                  <div className="flex min-h-0 flex-1 flex-col">
                    <div className="flex h-12 items-center justify-between gap-3 px-3" style={{ paddingLeft: compareEditorTextInset }}>
                      <h3 className="text-sm font-semibold">{t.tools.textCompare.text1}</h3>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-11 w-11 md:h-8 md:w-8 shrink-0"
                        onClick={() => handleCopy(tab.text1, t.tools.textCompare.text1)}
                        disabled={!tab.text1}
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                    </div>
                    <HighlightedCompareEditor
                      className="flex-1"
                      value={tab.text1}
                      otherValue={tab.text2}
                      side="left"
                      onChange={(value) => handleUpdateTab(tab.id, { text1: value })}
                      placeholder={t.tools.textCompare.inputFirstPlaceholder}
                    />
                  </div>
                </div>

                {/* 右侧文本 */}
                <div className="flex min-h-0 flex-col bg-card">
                  <div className="flex min-h-0 flex-1 flex-col">
                    <div className="flex h-12 items-center justify-between gap-3 px-3" style={{ paddingLeft: compareEditorTextInset }}>
                      <h3 className="text-sm font-semibold">{t.tools.textCompare.text2}</h3>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-11 w-11 md:h-8 md:w-8 shrink-0"
                        onClick={() => handleCopy(tab.text2, t.tools.textCompare.text2)}
                        disabled={!tab.text2}
                      >
                        <Copy className="h-4 w-4" />
                      </Button>
                    </div>
                    <HighlightedCompareEditor
                      className="flex-1"
                      value={tab.text2}
                      otherValue={tab.text1}
                      side="right"
                      onChange={(value) => handleUpdateTab(tab.id, { text2: value })}
                      placeholder={t.tools.textCompare.inputSecondPlaceholder}
                    />
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
