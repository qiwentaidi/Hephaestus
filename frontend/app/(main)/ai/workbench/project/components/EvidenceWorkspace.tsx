'use client'

import Link from 'next/link'
import { useEffect, useMemo, useRef, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Spinner } from '@/components/ui/spinner'
import { getAIWorkbenchProjectEvidenceEntry, listAIWorkbenchProjectEvidence, type AIWorkbenchEvidenceCounts, type AIWorkbenchProjectEvidenceItem, type AIWorkbenchPenTestSessionBlackboard as Blackboard } from '@/lib/api/ai-workbench'
import { explorationHref, type ExplorationEdge, type ExplorationGraph, type ExplorationKind } from '@/lib/exploration-graph'
import { findingDetailHref } from '@/lib/finding-detail'
import { useI18n } from '@/lib/i18n'
import { relationNames, stateNames } from '../../exploration/ExplorationClient'
import { ExecutionEvidence } from '../../components/ExecutionEvidence'
import { ReportContent } from '../../../../vulnerabilities/detail/ReportContent'
import styles from '../project.module.css'
import doc from './evidence.module.css'

const PAGE_SIZE = 20

type Props = {
  projectId: string
  source: string
  /** Evidence kind filter from the URL: '', finding, fact or blocker. */
  kind: string
  onKindChange: (kind: string) => void
  /** Changes whenever the project snapshot refreshes, so evidence follows it. */
  revision: string
  selected: string | null
  onSelect: (id: string) => void
  onLocate: (id: string) => void
  board: Blackboard
  graph: ExplorationGraph
  returnHref: string
}

const KIND_FILTERS = [
  { key: '', labels: ['全部', 'All'] },
  { key: 'finding', labels: ['发现', 'Findings'] },
  { key: 'fact', labels: ['事实', 'Facts'] },
  { key: 'blocker', labels: ['阻塞', 'Blockers'] },
] as const

function dayLabel(value: string | undefined, zh: boolean): string {
  if (!value) return zh ? '未记录时间' : 'Undated'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return zh ? '未记录时间' : 'Undated'
  const today = new Date()
  const yesterday = new Date(today)
  yesterday.setDate(today.getDate() - 1)
  if (date.toDateString() === today.toDateString()) return zh ? '今天' : 'Today'
  if (date.toDateString() === yesterday.toDateString()) return zh ? '昨天' : 'Yesterday'
  return date.toLocaleDateString(zh ? 'zh-CN' : 'en-US', { year: 'numeric', month: 'numeric', day: 'numeric' })
}

function clock(value: string | undefined, zh: boolean): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleTimeString(zh ? 'zh-CN' : 'en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
}

const kindLabel = (kind: string | undefined, zh: boolean) => {
  if (kind === 'finding') return zh ? '发现' : 'Finding'
  if (kind === 'blocker') return zh ? '阻塞' : 'Blocker'
  if (kind === 'fact') return zh ? '事实' : 'Fact'
  return zh ? '记录' : 'Record'
}

/**
 * Paged evidence index. The graph and overview stay bounded snapshots, so the
 * index reads every fact and finding from the server instead of deriving its
 * rows from the loaded blackboard.
 */
export function EvidenceWorkspace({ projectId, source, kind, onKindChange, revision, selected, onSelect, onLocate, board, graph, returnHref }: Props) {
  const { language } = useI18n(), zh = language === 'zh'
  const [query, setQuery] = useState('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [items, setItems] = useState<AIWorkbenchProjectEvidenceItem[]>([])
  const [total, setTotal] = useState(0)
  const [counts, setCounts] = useState<AIWorkbenchEvidenceCounts | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [detail, setDetail] = useState<AIWorkbenchProjectEvidenceItem | null>(null)
  const [detailMissing, setDetailMissing] = useState(false)
  const indexRef = useRef<HTMLElement>(null)
  const reportRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(query.trim())
      setPage(1)
    }, 300)
    return () => window.clearTimeout(timer)
  }, [query])

  // A new kind filter starts a new result set; page 1 keeps paging consistent.
  useEffect(() => { setPage(1) }, [kind])

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    void listAIWorkbenchProjectEvidence(projectId, {
      page,
      pageSize: PAGE_SIZE,
      search,
      kind,
      source: source === 'all' ? '' : source,
      signal: controller.signal,
    }).then(data => {
      if (controller.signal.aborted) return
      setItems(data.items)
      setTotal(data.total)
      setCounts(data.counts)
      setError('')
      // Records removed elsewhere can shrink the list under the current page.
      if (!data.items.length && data.total > 0 && page > 1) setPage(Math.max(1, Math.ceil(data.total / PAGE_SIZE)))
    }).catch(err => {
      if (controller.signal.aborted) return
      setItems([])
      setTotal(0)
      setError(err instanceof Error ? err.message : (zh ? '加载证据记录失败' : 'Failed to load evidence'))
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false)
    })
    return () => controller.abort()
  }, [projectId, page, search, source, kind, revision, zh])

  useEffect(() => {
    if (!selected) {
      setDetail(null)
      setDetailMissing(false)
      return
    }
    if (items.some(item => item.id === selected)) {
      setDetail(null)
      setDetailMissing(false)
      return
    }
    if (detail?.id === selected) return
    const controller = new AbortController()
    setDetailMissing(false)
    void getAIWorkbenchProjectEvidenceEntry(projectId, selected, controller.signal)
      .then(item => { if (!controller.signal.aborted) setDetail(item) })
      .catch(() => {
        if (controller.signal.aborted) return
        setDetail(null)
        setDetailMissing(true)
      })
    return () => controller.abort()
  }, [projectId, selected, items, detail?.id])

  const active = selected ? items.find(item => item.id === selected) ?? (detail?.id === selected ? detail : null) : items[0]
  const detailPending = Boolean(selected && !active && !detailMissing && !loading)
  const activeInGraph = Boolean(active && graph.nodes.some(node => node.id === active.id))
  const relations: ExplorationEdge[] = active && activeInGraph
    ? graph.edges.filter(edge => !edge.provenance && (edge.source === active.id || edge.target === active.id))
    : []
  const sourceName = active?.sourceTitle || board.sourceConversations?.find(item => item.id === active?.conversationId)?.title

  const sections = useMemo(() => {
    const out: { key: string; label: string; rows: AIWorkbenchProjectEvidenceItem[] }[] = []
    for (const item of items) {
      const stamp = item.updatedAt || item.createdAt
      const key = dayLabel(stamp, false)
      const last = out[out.length - 1]
      if (last && last.key === key) last.rows.push(item)
      else out.push({ key, label: dayLabel(stamp, zh), rows: [item] })
    }
    return out
  }, [items, zh])

  const chipCount = (key: string) => {
    if (!counts) return key === '' ? total : 0
    if (key === 'finding') return counts.findings
    if (key === 'blocker') return counts.blockers
    if (key === 'fact') return Math.max(0, counts.facts - counts.blockers)
    return counts.facts + counts.findings
  }

  useEffect(() => {
    const index = indexRef.current
    const item = index?.querySelector('[data-selected="true"]')
    if (index && item) index.scrollTo({ top: index.scrollTop + item.getBoundingClientRect().top - index.getBoundingClientRect().top - 80 })
  }, [active?.id])
  useEffect(() => { reportRef.current?.scrollTo({ top: 0 }) }, [active?.id])

  const section = (title: string, content?: string) => (
    <section>
      <h3>{title}</h3>
      {content
        ? <ReportContent value={content} language={language} />
        : <p className="text-sm text-muted-foreground">{zh ? '尚未记录' : 'Not recorded'}</p>}
    </section>
  )

  return (
    <div className={styles.evidence}>
      <aside ref={indexRef} className={styles.index} aria-label={zh ? '证据目录' : 'Evidence index'}>
        <div className={doc.filters}>
          <Input
            aria-label={zh ? '搜索证据' : 'Search evidence'}
            placeholder={zh ? '搜索结论与证据…' : 'Search evidence…'}
            value={query}
            onChange={event => setQuery(event.target.value)}
          />
          <div className={doc.chips} role="group" aria-label={zh ? '证据类型' : 'Evidence kind'}>
            {KIND_FILTERS.map(option => (
              <button key={option.key || 'all'} type="button" className={doc.chip} data-active={kind === option.key} onClick={() => onKindChange(option.key)}>
                {option.labels[zh ? 0 : 1]}
                <span className={doc.chipCount}>{chipCount(option.key)}</span>
              </button>
            ))}
          </div>
        </div>
        {error ? <p role="alert" className="mx-3 mb-3 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">{error}</p> : null}
        {sections.map(group => (
          <div key={group.key}>
            <div className={doc.dayLabel}>{group.label}</div>
            {group.rows.map(item => (
              <button
                key={item.id}
                className={`${styles.row} ${doc.row}`}
                data-selected={active?.id === item.id}
                onClick={() => onSelect(item.id)}
              >
                <span className={doc.rowTitle}>
                  <span className={doc.dot} data-kind={item.kind} />
                  <span className={doc.rowText}>{item.title}</span>
                </span>
                <span className={doc.rowMeta}>
                  {kindLabel(item.kind, zh)}
                  {item.sourceTitle ? ` · ${item.sourceTitle}` : ''}
                  {` · ${clock(item.updatedAt || item.createdAt, zh) || '—'}`}
                </span>
              </button>
            ))}
          </div>
        ))}
        {loading && !items.length ? <p className="flex items-center gap-2 px-3 py-4 text-sm text-muted-foreground"><Spinner />{zh ? '加载证据记录…' : 'Loading evidence…'}</p> : null}
        {!loading && !items.length && !error ? <p className={styles.empty}>{search || kind ? (zh ? '没有匹配记录' : 'No matches') : (zh ? '暂无记录' : 'No records')}</p> : null}
        {total > PAGE_SIZE ? (
          <div className={doc.pager}>
            <PaginationPro
              current={page}
              total={total}
              pageSize={PAGE_SIZE}
              onChange={nextPage => setPage(nextPage)}
              showSizeChanger={false}
              disabled={loading}
            />
          </div>
        ) : null}
      </aside>
      <div ref={reportRef} className={styles.report}>
        {active ? (
          <article key={active.id}>
            <div className={doc.bar}>
              <Badge variant="outline">{kindLabel(active.kind, zh)}</Badge>
              {active.severity ? <Badge variant="outline" className={doc.severityBadge}>{active.severityLabel || active.severity}</Badge> : null}
              {active.status ? <Badge variant="outline" className="bg-muted/40 text-muted-foreground">{stateNames[active.status]?.[zh ? 0 : 1] || active.status}</Badge> : null}
              <div className={doc.barActions}>
                {activeInGraph ? <Button size="sm" variant="ghost" onClick={() => onLocate(active.id)}>{zh ? '图中定位' : 'Locate in graph'}</Button> : null}
                {active.findingId ? <Button variant="outline" size="sm" asChild><Link href={findingDetailHref(active.findingId, returnHref)}>{zh ? '完整漏洞报告' : 'Full report'} ↗</Link></Button> : null}
              </div>
            </div>
            <h2>{active.title}</h2>
            <div className={doc.meta}>
              <span>{zh ? '记录于 ' : 'Recorded '}<b>{clock(active.updatedAt || active.createdAt, zh) || '—'}</b></span>
              <span>{zh ? '来源 ' : 'Source '}<b>{sourceName || (zh ? '未记录来源会话' : 'No source conversation')}</b></span>
              {active.category ? <span>{zh ? '分类 ' : 'Category '}<b>{active.category}</b></span> : null}
              {active.conversationId ? (
                <>
                  <Link className="underline underline-offset-4" href={`/ai/workbench?session=${encodeURIComponent(active.conversationId)}`}>{zh ? '打开会话' : 'Conversation'}</Link>
                  <Link className="underline underline-offset-4" href={explorationHref(active.conversationId)}>{zh ? '会话探索链路' : 'Session exploration'}</Link>
                </>
              ) : null}
            </div>
            {active.kind === 'finding' ? (
              <>
                {section(zh ? '影响与结论' : 'Impact and conclusion', active.impact)}
                {section(zh ? '验证步骤' : 'Reproduction', active.reproductionSteps)}
                {section(zh ? '原始证据' : 'Evidence', active.evidence)}
              </>
            ) : section(zh ? '事实与证据' : 'Fact and evidence', active.body)}
            {active.executionId && active.conversationId ? (
              <section>
                <h3>{zh ? '原始工具执行' : 'Original execution'}</h3>
                <ExecutionEvidence conversationId={active.conversationId} executionId={active.executionId} language={language} />
              </section>
            ) : null}
            <section>
              <h3>{zh ? '关联依据与结论' : 'Related evidence and conclusions'}</h3>
              {!activeInGraph ? (
                <p className="text-sm text-muted-foreground">{zh ? '该记录超出图谱快照，关联关系请在会话探索链路中查看。' : 'This record is outside the graph snapshot; open the session exploration for its relationships.'}</p>
              ) : relations.length ? relations.map(edge => {
                const other = graph.nodes.find(node => node.id === (edge.source === active.id ? edge.target : edge.source))
                if (!other) return null
                return (
                  <button key={edge.id} className={styles.row} onClick={() => other.factKey || other.findingId ? onSelect(other.id) : onLocate(other.id)}>
                    <span className="mb-1 block text-xs text-muted-foreground">{edge.source === active.id ? '→ ' : '← '}{relationNames[edge.relation]?.[zh ? 0 : 1] || edge.relation}</span>
                    <span className="text-sm">{other.title}</span>
                  </button>
                )
              }) : <p className="text-sm text-muted-foreground">{zh ? '尚未记录关联关系' : 'No relationships recorded'}</p>}
            </section>
          </article>
        ) : detailPending ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><Spinner />{zh ? '加载记录…' : 'Loading record…'}</p>
        ) : (
          <p className={styles.empty}>
            {selected
              ? (detailMissing ? (zh ? '该记录不在证据范围，或已被移除。' : 'This record is not available in evidence, or it was removed.') : (zh ? '该记录不在当前范围，请切换来源会话或重新选择。' : 'This record is outside the current scope.'))
              : (zh ? '暂无可展示的证据' : 'No evidence recorded')}
          </p>
        )}
      </div>
    </div>
  )
}
