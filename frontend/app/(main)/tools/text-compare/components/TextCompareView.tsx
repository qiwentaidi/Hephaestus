'use client'

import { useMemo } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useI18n } from '@/lib/i18n'

interface TextCompareViewProps {
  text1: string
  text2: string
}

// 简单的 diff 算法：逐行对比
function computeDiff(text1: string, text2: string) {
  const lines1 = text1.split('\n')
  const lines2 = text2.split('\n')
  const maxLines = Math.max(lines1.length, lines2.length)
  const diff: Array<{
    line1: string | null
    line2: string | null
    type: 'equal' | 'delete' | 'insert' | 'modify'
  }> = []

  // 简单的逐行对比
  for (let i = 0; i < maxLines; i++) {
    const line1 = i < lines1.length ? lines1[i] : null
    const line2 = i < lines2.length ? lines2[i] : null

    if (line1 === null && line2 !== null) {
      // 只在 text2 中存在
      diff.push({ line1: null, line2, type: 'insert' })
    } else if (line1 !== null && line2 === null) {
      // 只在 text1 中存在
      diff.push({ line1, line2: null, type: 'delete' })
    } else if (line1 === line2) {
      // 相同
      diff.push({ line1: line1!, line2: line2!, type: 'equal' })
    } else {
      // 不同
      diff.push({ line1: line1!, line2: line2!, type: 'modify' })
    }
  }

  return diff
}

// 字符级别的 diff（用于高亮行内差异）
function computeCharDiff(line1: string, line2: string) {
  const chars1 = line1.split('')
  const chars2 = line2.split('')
  const maxLen = Math.max(chars1.length, chars2.length)
  const diff: Array<{
    char1: string | null
    char2: string | null
    type: 'equal' | 'delete' | 'insert' | 'modify'
  }> = []

  for (let i = 0; i < maxLen; i++) {
    const char1 = i < chars1.length ? chars1[i] : null
    const char2 = i < chars2.length ? chars2[i] : null

    if (char1 === null && char2 !== null) {
      diff.push({ char1: null, char2, type: 'insert' })
    } else if (char1 !== null && char2 === null) {
      diff.push({ char1, char2: null, type: 'delete' })
    } else if (char1 === char2) {
      diff.push({ char1: char1!, char2: char2!, type: 'equal' })
    } else {
      diff.push({ char1: char1!, char2: char2!, type: 'modify' })
    }
  }

  return diff
}

export function TextCompareView({ text1, text2 }: TextCompareViewProps) {
  const { t } = useI18n()
  const diff = useMemo(() => computeDiff(text1, text2), [text1, text2])

  if (!text1 && !text2) {
    return (
      <div className="h-[400px] flex items-center justify-center text-muted-foreground">
        {t.tools.textCompare.emptyHint}
      </div>
    )
  }

  return (
    <ScrollArea className="h-[400px] w-full border rounded-md">
      <div className="p-4 font-mono text-sm">
        <div className="grid grid-cols-2 gap-4">
          {/* 左侧：文本1 */}
          <div className="space-y-0">
            <div className="sticky top-0 bg-muted/50 px-2 py-1 text-xs font-semibold mb-2">
              {t.tools.textCompare.text1}
            </div>
            {diff.map((item, index) => {
              if (item.type === 'insert') {
                return (
                  <div key={index} className="h-5 bg-transparent"></div>
                )
              }
              
              const charDiff = item.line1 && item.line2 && item.type === 'modify'
                ? computeCharDiff(item.line1, item.line2)
                : null

              return (
                <div
                  key={index}
                  className={`px-2 py-0.5 min-h-[20px] ${
                    item.type === 'delete'
                      ? 'bg-destructive/20'
                      : item.type === 'modify'
                      ? 'bg-yellow-500/20'
                      : 'bg-transparent'
                  }`}
                >
                  {charDiff ? (
                    <span>
                      {charDiff.map((charItem, charIndex) => (
                        <span
                          key={charIndex}
                          className={
                            charItem.type === 'delete' || charItem.type === 'modify'
                              ? 'bg-destructive/40'
                              : ''
                          }
                        >
                          {charItem.char1 || ''}
                        </span>
                      ))}
                    </span>
                  ) : (
                    <span className={item.type === 'delete' ? 'line-through opacity-60' : ''}>
                      {item.line1 || ''}
                    </span>
                  )}
                </div>
              )
            })}
          </div>

          {/* 右侧：文本2 */}
          <div className="space-y-0">
            <div className="sticky top-0 bg-muted/50 px-2 py-1 text-xs font-semibold mb-2">
              {t.tools.textCompare.text2}
            </div>
            {diff.map((item, index) => {
              if (item.type === 'delete') {
                return (
                  <div key={index} className="h-5 bg-transparent"></div>
                )
              }
              
              const charDiff = item.line1 && item.line2 && item.type === 'modify'
                ? computeCharDiff(item.line1, item.line2)
                : null

              return (
                <div
                  key={index}
                  className={`px-2 py-0.5 min-h-[20px] ${
                    item.type === 'insert'
                      ? 'bg-green-500/20'
                      : item.type === 'modify'
                      ? 'bg-yellow-500/20'
                      : 'bg-transparent'
                  }`}
                >
                  {charDiff ? (
                    <span>
                      {charDiff.map((charItem, charIndex) => (
                        <span
                          key={charIndex}
                          className={
                            charItem.type === 'insert' || charItem.type === 'modify'
                              ? 'bg-green-500/40'
                              : ''
                          }
                        >
                          {charItem.char2 || ''}
                        </span>
                      ))}
                    </span>
                  ) : (
                    <span>
                      {item.line2 || ''}
                    </span>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </ScrollArea>
  )
}
