'use client'

import Link from 'next/link'
import { useEffect, useRef, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { Spinner } from '@/components/ui/spinner'
import { getAIWorkbenchProjectEvidenceEntry, listAIWorkbenchProjectEvidence, type AIWorkbenchProjectEvidenceItem, type AIWorkbenchPenTestSessionBlackboard as Blackboard } from '@/lib/api/ai-workbench'
import { explorationHref, type ExplorationEdge, type ExplorationGraph, type ExplorationKind } from '@/lib/exploration-graph'
import { findingDetailHref } from '@/lib/finding-detail'
import { useI18n } from '@/lib/i18n'
import { kinds, relationNames, stateNames } from '../../exploration/ExplorationClient'
import { ExecutionEvidence } from '../../components/ExecutionEvidence'
import { ReportContent } from '../../../../vulnerabilities/detail/ReportContent'
import styles from '../project.module.css'

const PAGE_SIZE = 20

type Props = {
  projectId: string
  source: string
  /** Changes whenever the project snapshot refreshes, so evidence follows it. */
  revision: string
  selected: string | null
  onSelect: (id: string) => void
  onLocate: (id: string) => void
  board: Blackboard
  graph: ExplorationGraph
  returnHref: string
}

/**
 * Paged evidence index. The graph and overview stay bounded snapshots, so the
 * index reads every fact and finding from the server instead of deriving its
 * rows from the loaded blackboard.
 */
export function EvidenceWorkspace({ projectId, source, revision, selected, onSelect, onLocate, board, graph, returnHref }: Props) {
  const { language } = useI18n(), zh = language === 'zh'
  const [query, setQuery] = useState('')
  const [search, setSearch] = useState('')
  const [page, setPage] = useState(1)
  const [items, setItems] = useState<AIWorkbenchProjectEvidenceItem[]>([])
  const [total, setTotal] = useState(0)
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

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    void listAIWorkbenchProjectEvidence(projectId, {
      page,
      pageSize: PAGE_SIZE,
      search,
      source: source === 'all' ? '' : source,
      signal: controller.signal,
    }).then(data => {
      if (controller.signal.aborted) return
      setItems(data.items)
      setTotal(data.total)
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
  }, [projectId, page, search, source, revision, zh])

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
        <Input
          aria-label={zh ? '搜索证据' : 'Search evidence'}
          placeholder={zh ? '搜索结论与证据…' : 'Search evidence…'}
          value={query}
          onChange={event => setQuery(event.target.value)}
          className="mb-4"
        />
        {error ? <p role="alert" className="mb-3 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive">{error}</p> : null}
        {items.map(item => (
          <button
            key={item.id}
            className={styles.row}
            data-selected={active?.id === item.id}
            onClick={() => onSelect(item.id)}
          >
            <span className="mb-1 block text-xs text-muted-foreground">{kinds[item.kind as ExplorationKind]?.[language] || item.kind}</span>
            <span className="line-clamp-3 text-sm leading-6">{item.title}</span>
          </button>
        ))}
        {loading && !items.length ? <p className="flex items-center gap-2 py-4 text-sm text-muted-foreground"><Spinner />{zh ? '加载证据记录…' : 'Loading evidence…'}</p> : null}
        {!loading && !items.length && !error ? <p className={styles.empty}>{search ? (zh ? '没有匹配记录' : 'No matches') : (zh ? '暂无记录' : 'No records')}</p> : null}
        {total > PAGE_SIZE ? (
          <PaginationPro
            current={page}
            total={total}
            pageSize={PAGE_SIZE}
            onChange={nextPage => setPage(nextPage)}
            showSizeChanger={false}
            disabled={loading}
          />
        ) : null}
      </aside>
      <div ref={reportRef} className={styles.report}>
        {active ? (
          <article key={active.id}>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="outline">{kinds[active.kind as ExplorationKind]?.[language] || active.kind}</Badge>
              {active.status ? <Badge variant="outline" className="bg-muted/40 text-muted-foreground">{stateNames[active.status]?.[zh ? 0 : 1] || active.status}</Badge> : null}
              {activeInGraph ? <Button size="sm" variant="outline" onClick={() => onLocate(active.id)}>{zh ? '图中定位' : 'Locate in graph'}</Button> : null}
              {active.findingId ? <Button variant="ghost" size="sm" asChild><Link href={findingDetailHref(active.findingId, returnHref)}>{zh ? '完整漏洞报告 ↗' : 'Full report ↗'}</Link></Button> : null}
            </div>
            <h2>{active.title}</h2>
            <div className="flex flex-wrap items-center gap-3 border-y py-3 text-xs text-muted-foreground">
              <span>{zh ? '来源：' : 'Source: '}{sourceName || (zh ? '未记录来源会话' : 'No source conversation')}</span>
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
