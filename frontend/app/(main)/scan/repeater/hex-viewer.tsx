"use client"

import { useState, useCallback, useMemo } from "react"

interface HexViewerProps {
  data: string
}

interface HexLine {
  offset: number
  bytes: number[]
  hexParts: string[]
  asciiParts: string[]
}

export function HexViewer({ data }: HexViewerProps) {
  const [hoveredByteIndex, setHoveredByteIndex] = useState<number | null>(null)
  const [selectedRange, setSelectedRange] = useState<{ start: number; end: number } | null>(null)
  const [isSelecting, setIsSelecting] = useState(false)
  const [selectionStart, setSelectionStart] = useState<number | null>(null)

  const bytesPerLine = 16

  // 将文本转换为字节和 HEX 行数据
  const hexLines = useMemo((): HexLine[] => {
    const bytes = new TextEncoder().encode(data)
    const lines: HexLine[] = []

    for (let offset = 0; offset < bytes.length; offset += bytesPerLine) {
      const chunk = bytes.slice(offset, offset + bytesPerLine)
      const lineBytes: number[] = Array.from(chunk)
      
      // HEX 部分
      const hexParts: string[] = []
      for (let i = 0; i < bytesPerLine; i++) {
        if (i < chunk.length) {
          hexParts.push(chunk[i].toString(16).padStart(2, '0'))
        } else {
          hexParts.push('  ')
        }
      }
      
      // ASCII 部分
      const asciiParts: string[] = []
      for (let i = 0; i < chunk.length; i++) {
        const byte = chunk[i]
        if (byte >= 32 && byte <= 126) {
          asciiParts.push(String.fromCharCode(byte))
        } else {
          asciiParts.push('.')
        }
      }

      lines.push({
        offset,
        bytes: lineBytes,
        hexParts,
        asciiParts,
      })
    }

    return lines
  }, [data])

  // 检查字节索引是否在选中范围内
  const isInSelectedRange = useCallback((byteIndex: number): boolean => {
    if (!selectedRange) return false
    const { start, end } = selectedRange
    const min = Math.min(start, end)
    const max = Math.max(start, end)
    return byteIndex >= min && byteIndex <= max
  }, [selectedRange])

  // 检查字节索引是否被悬停
  const isHovered = useCallback((byteIndex: number): boolean => {
    return hoveredByteIndex === byteIndex
  }, [hoveredByteIndex])

  // 处理鼠标进入
  const handleMouseEnter = useCallback((byteIndex: number) => {
    setHoveredByteIndex(byteIndex)
    if (isSelecting && selectionStart !== null) {
      setSelectedRange({ start: selectionStart, end: byteIndex })
    }
  }, [isSelecting, selectionStart])

  // 处理鼠标离开
  const handleMouseLeave = useCallback(() => {
    if (!isSelecting) {
      setHoveredByteIndex(null)
    }
  }, [isSelecting])

  // 处理鼠标按下（开始选择）
  const handleMouseDown = useCallback((byteIndex: number) => {
    setIsSelecting(true)
    setSelectionStart(byteIndex)
    setSelectedRange({ start: byteIndex, end: byteIndex })
  }, [])

  // 处理鼠标抬起（结束选择）
  const handleMouseUp = useCallback(() => {
    setIsSelecting(false)
    setSelectionStart(null)
  }, [])

  // 获取字节的样式类
  const getByteClassName = useCallback((byteIndex: number): string => {
    const classes = ['cursor-pointer', 'rounded-sm', 'transition-colors', 'duration-75']
    
    if (isInSelectedRange(byteIndex)) {
      classes.push('bg-primary/30', 'text-primary-foreground')
    } else if (isHovered(byteIndex)) {
      classes.push('bg-primary/20')
    }
    
    return classes.join(' ')
  }, [isInSelectedRange, isHovered])

  if (hexLines.length === 0) {
    return (
      <div className="h-full overflow-auto bg-muted/30 p-4">
        <pre className="font-mono text-xs leading-relaxed whitespace-pre text-muted-foreground">
          (empty)
        </pre>
      </div>
    )
  }

  return (
    <div 
      className="h-full overflow-auto bg-muted/30 p-4 select-none"
      onMouseUp={handleMouseUp}
      onMouseLeave={() => {
        handleMouseLeave()
        if (isSelecting) {
          handleMouseUp()
        }
      }}
    >
      <div className="font-mono text-xs leading-relaxed">
        {hexLines.map((line, lineIndex) => (
          <div key={lineIndex} className="flex whitespace-pre">
            {/* 偏移地址 */}
            <span className="text-muted-foreground select-none w-[72px]">
              {line.offset.toString(16).padStart(8, '0')}
            </span>
            
            <span className="text-muted-foreground select-none mx-1">│</span>
            
            {/* HEX 部分 - 前 8 字节 */}
            <span className="flex gap-[3px]">
              {line.hexParts.slice(0, 8).map((hex, i) => {
                const byteIndex = line.offset + i
                const isEmpty = hex === '  '
                return (
                  <span
                    key={`hex-${lineIndex}-${i}`}
                    className={isEmpty ? 'w-[18px]' : getByteClassName(byteIndex)}
                    onMouseEnter={() => !isEmpty && handleMouseEnter(byteIndex)}
                    onMouseDown={() => !isEmpty && handleMouseDown(byteIndex)}
                    style={{ minWidth: '18px', textAlign: 'center' }}
                  >
                    {hex}
                  </span>
                )
              })}
            </span>
            
            {/* 分隔符 */}
            <span className="text-muted-foreground select-none mx-1">│</span>
            
            {/* HEX 部分 - 后 8 字节 */}
            <span className="flex gap-[3px]">
              {line.hexParts.slice(8).map((hex, i) => {
                const byteIndex = line.offset + 8 + i
                const isEmpty = hex === '  '
                return (
                  <span
                    key={`hex-${lineIndex}-${i + 8}`}
                    className={isEmpty ? 'w-[18px]' : getByteClassName(byteIndex)}
                    onMouseEnter={() => !isEmpty && handleMouseEnter(byteIndex)}
                    onMouseDown={() => !isEmpty && handleMouseDown(byteIndex)}
                    style={{ minWidth: '18px', textAlign: 'center' }}
                  >
                    {hex}
                  </span>
                )
              })}
            </span>
            
            <span className="text-muted-foreground select-none mx-2">│</span>
            
            {/* ASCII 部分 */}
            <span className="flex">
              {line.asciiParts.map((char, i) => {
                const byteIndex = line.offset + i
                return (
                  <span
                    key={`ascii-${lineIndex}-${i}`}
                    className={getByteClassName(byteIndex)}
                    onMouseEnter={() => handleMouseEnter(byteIndex)}
                    onMouseDown={() => handleMouseDown(byteIndex)}
                    style={{ minWidth: '8px', textAlign: 'center' }}
                  >
                    {char}
                  </span>
                )
              })}
            </span>
            
            <span className="text-muted-foreground select-none">│</span>
          </div>
        ))}
      </div>
    </div>
  )
}
