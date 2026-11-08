'use client'

import { useEffect, useRef, useState } from 'react'
import { renderMarkdownHTML } from '@/lib/markdown'
import { findingReportBlocks } from '@/lib/finding-report-content'
import { copyToClipboard } from '@/lib/clipboard'
import toast from 'react-hot-toast'
import styles from './report.module.css'

export function ReportContent({ value, language }: { value: string; language: 'zh' | 'en' }) {
  const zh = language === 'zh'
  const [copied, setCopied] = useState<number | null>(null)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])
  return <div className={styles.content}>{findingReportBlocks(value).map((block, index) => block.kind === 'markdown'
    ? <div key={index} dangerouslySetInnerHTML={{ __html: renderMarkdownHTML(block.content, { allowImages: false }) }} />
    : <figure key={index} className={styles.codeBlock}>
      <figcaption className={styles.codeHeader}>
        <span><span className={styles.codeLanguage}>{block.language}</span><span className={styles.codeCaption}>{block.recorded ? (zh ? '原始记录' : 'Recorded excerpt') : (zh ? '代码 / 原文' : 'Code / source')}</span></span>
        <button type="button" aria-label={`${zh ? '复制代码块' : 'Copy code block'} ${index + 1}`} onClick={async () => {
          if (!await copyToClipboard(block.content)) { toast.error(zh ? '复制失败' : 'Copy failed'); return }
          setCopied(index)
          if (timer.current) clearTimeout(timer.current)
          timer.current = setTimeout(() => setCopied(null), 1800)
        }}>{copied === index ? (zh ? '已复制' : 'Copied') : (zh ? '复制' : 'Copy')}</button>
      </figcaption>
      <pre tabIndex={0} className={block.recorded ? styles.recorded : undefined}><code>{block.content}</code></pre>
    </figure>)}</div>
}
