'use client'

import { useEffect, useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
  listAIWorkbenchProjectEvidence,
  type AIWorkbenchProjectEvidenceItem,
  type AIWorkbenchProjectStats,
} from '@/lib/api/ai-workbench'
import { useI18n } from '@/lib/i18n'
import styles from './overview.module.css'

export type OverviewEvidenceKind = 'finding' | 'fact' | 'blocker'

type Props = {
  projectId: string
  source: string
  /** Aggregates from /projects/:id/stats; null while the first load is running. */
  stats: AIWorkbenchProjectStats | null
  /** Latest activity stamp from stats; a change refreshes the recent list. */
  activity?: string
  onOpenEvidence: (kind?: OverviewEvidenceKind) => void
  onOpenPlan: () => void
}

const RISK_LEVELS = [
  { key: 'critical', labels: ['严重', 'Critical'] },
  { key: 'high', labels: ['高危', 'High'] },
  { key: 'medium', labels: ['中危', 'Medium'] },
  { key: 'low', labels: ['低危', 'Low'] },
] as const

function clock(value: string | undefined, zh: boolean): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime()) || date.getTime() <= 0) return ''
  const now = new Date()
  return date.toDateString() === now.toDateString()
    ? date.toLocaleTimeString(zh ? 'zh-CN' : 'en-US', { hour: '2-digit', minute: '2-digit', hour12: false })
    : date.toLocaleDateString(zh ? 'zh-CN' : 'en-US', { month: 'numeric', day: 'numeric' })
}

/**
 * Conclusions-first overview. It renders from the stats endpoint (counts,
 * severity, per-asset aggregates, a few pending rows) plus the five newest
 * evidence records, so opening the project never loads the full snapshot.
 */
export function OverviewWorkspace({ projectId, source, stats, activity, onOpenEvidence, onOpenPlan }: Props) {
  const { language } = useI18n()
  const zh = language === 'zh'
  const [recent, setRecent] = useState<AIWorkbenchProjectEvidenceItem[]>([])

  useEffect(() => {
    const controller = new AbortController()
    void listAIWorkbenchProjectEvidence(projectId, {
      page: 1,
      pageSize: 5,
      source: source === 'all' ? '' : source,
      signal: controller.signal,
    }).then(data => {
      if (!controller.signal.aborted) setRecent(data.items)
    }).catch(() => {
      if (!controller.signal.aborted) setRecent([])
    })
    return () => controller.abort()
  }, [projectId, source, activity])

  const counts = stats?.counts
  const confirmedCount = counts?.confirmedFindings ?? 0
  const candidateCount = counts?.candidateFindings ?? 0
  const factCount = counts?.facts ?? 0
  const blockerCount = counts?.blockers ?? 0
  const assets = stats?.assets ?? []
  const testedAssets = assets.filter(asset => asset.conversations > 0).length

  const severity = useMemo(() => RISK_LEVELS.map(level => ({
    ...level,
    count: stats?.severity?.[level.key] || 0,
  })), [stats])

  const severityMax = Math.max(1, ...severity.map(item => item.count))
  const highPlus = severity.filter(item => (item.key === 'critical' || item.key === 'high') && item.count > 0).reduce((sum, item) => sum + item.count, 0)
  const pending = (stats?.pending || []).slice(0, 4)
  const untested = assets.filter(asset => asset.conversations === 0).length

  return (
    <div className={styles.page}>
      <div className={styles.kpis}>
        <button type="button" className={styles.kpi} onClick={() => onOpenEvidence('finding')}>
          <span className={styles.kpiLabel}><span className={styles.dot} data-tone="red" />{zh ? '已确认漏洞' : 'Confirmed findings'}</span>
          <div className={styles.kpiValue}>{confirmedCount}<small>{zh ? `高危及以上 ${highPlus}` : `${highPlus} high+`}</small></div>
          <div className={styles.kpiFoot}>{zh ? '点击查看全部结论与证据' : 'Open every conclusion and its evidence'}</div>
          <span className={styles.arrow}>↗</span>
        </button>
        <button type="button" className={styles.kpi} onClick={() => onOpenEvidence('finding')}>
          <span className={styles.kpiLabel}><span className={styles.dot} data-tone="amber" />{zh ? '待验证发现' : 'Candidate findings'}</span>
          <div className={styles.kpiValue}>{candidateCount}<small>{zh ? '等待复现或补证' : 'Awaiting reproduction'}</small></div>
          <div className={styles.kpiFoot}>{zh ? '未计入风险统计，先补证据' : 'Not counted as risk until verified'}</div>
          <span className={styles.arrow}>↗</span>
        </button>
        <button type="button" className={styles.kpi} onClick={() => onOpenEvidence('fact')}>
          <span className={styles.kpiLabel}><span className={styles.dot} data-tone="blue" />{zh ? '关键事实' : 'Recorded facts'}</span>
          <div className={styles.kpiValue}>{factCount}<small>{zh ? `覆盖 ${testedAssets} 个资产` : `${testedAssets} assets`}</small></div>
          <div className={styles.kpiFoot}>{zh ? '攻击面与验证过程的可追溯记录' : 'Traceable attack-surface records'}</div>
          <span className={styles.arrow}>↗</span>
        </button>
        <button type="button" className={styles.kpi} onClick={() => onOpenEvidence('blocker')}>
          <span className={styles.kpiLabel}><span className={styles.dot} data-tone="muted" />{zh ? '阻塞' : 'Blockers'}</span>
          <div className={styles.kpiValue}>{blockerCount}<small>{zh ? '需要人工处理' : 'Needs a decision'}</small></div>
          <div className={styles.kpiFoot}>{zh ? '目标不可达、权限不足或被外部条件卡住' : 'Unreachable, unauthorized or stuck'}</div>
          <span className={styles.arrow}>↗</span>
        </button>
      </div>

      <div className={styles.cols}>
        <div className={styles.col}>
          <section className={styles.card}>
            <div className={styles.cardHead}>
              <h3 className={styles.cardTitle}>{zh ? '风险概览' : 'Risk overview'}</h3>
              <span className={styles.hint}>{zh ? `已确认 ${confirmedCount} 条 · 待验证 ${candidateCount} 条未计入` : `${confirmedCount} confirmed · ${candidateCount} candidates excluded`}</span>
            </div>
            <div className={styles.sevList}>
              {severity.map(level => (
                <div key={level.key} className={styles.sevRow}>
                  <span>{level.labels[zh ? 0 : 1]}</span>
                  <span className={styles.sevTrack}><span className={styles.sevFill} data-level={level.key} style={{ width: `${level.count ? Math.max(6, Math.round((level.count / severityMax) * 100)) : 0}%` }} /></span>
                  <span className={styles.sevCount}>{level.count}</span>
                </div>
              ))}
            </div>
          </section>

          <section className={styles.card}>
            <div className={styles.cardHead}>
              <h3 className={styles.cardTitle}>{zh ? '待处理' : 'Needs attention'}</h3>
              <span className={styles.hint}>{zh ? '阻塞与待验证优先' : 'Blockers and candidates first'}</span>
            </div>
            <div className={styles.todo}>
              {pending.length === 0 ? <p className={styles.empty}>{zh ? '没有需要处理的阻塞或待验证发现' : 'No blockers or candidates waiting'}</p> : pending.map(item => (
                <div key={item.id} className={styles.todoItem}>
                  <span className={styles.dot} data-tone={item.kind === 'blocker' ? 'amber' : 'blue'} />
                  <div className={styles.todoMain}>
                    <div className={styles.todoTitle}>{item.title}</div>
                    <div className={styles.todoMeta}>
                      <span>{item.kind === 'blocker' ? (zh ? '阻塞' : 'Blocked') : (zh ? '待验证' : 'Candidate')}</span>
                      {item.sourceTitle ? <><span>·</span><span className="truncate">{item.sourceTitle}</span></> : null}
                      {item.updatedAt ? <><span>·</span><span>{clock(item.updatedAt, zh)}</span></> : null}
                    </div>
                  </div>
                  <Button variant="outline" size="sm" className={styles.todoAction} onClick={() => onOpenEvidence(item.kind === 'blocker' ? 'blocker' : 'finding')}>
                    {item.kind === 'blocker' ? (zh ? '查看阻塞' : 'Open') : (zh ? '查看证据' : 'Evidence')}
                  </Button>
                </div>
              ))}
            </div>
            <div className={styles.cardFoot}>
              {zh ? `共 ${blockerCount + candidateCount} 项待处理` : `${blockerCount + candidateCount} items waiting`}
              <Button variant="ghost" size="sm" className={styles.go} onClick={() => onOpenEvidence(undefined)}>{zh ? '查看全部' : 'View all'} ↗</Button>
            </div>
          </section>
        </div>

        <div className={styles.col}>
          <section className={styles.card}>
            <div className={styles.cardHead}>
              <h3 className={styles.cardTitle}>{zh ? '攻击面' : 'Attack surface'}</h3>
              <span className={styles.hint}>{zh ? '有结论的会话占比' : 'Sessions that produced conclusions'}</span>
            </div>
            <div className={styles.assets}>
              {assets.length === 0 ? <p className={styles.empty}>{zh ? '项目尚未记录资产范围' : 'No asset scope recorded'}</p> : assets.slice(0, 6).map(asset => {
                const ratio = asset.conversations ? asset.covered / asset.conversations : 0
                return (
                  <div key={asset.target} className={styles.asset}>
                    <div className={styles.assetTop}>
                      <span className={styles.dot} data-tone={asset.findings ? 'green' : asset.conversations ? 'amber' : 'muted'} />
                      <span className={styles.assetName} title={asset.target}>{asset.target}</span>
                      <span className={styles.assetMeta}>
                        {zh ? `${asset.conversations} 会话 · ${asset.findings} 发现` : `${asset.conversations} sessions · ${asset.findings} findings`}
                      </span>
                    </div>
                    <div className={styles.cov}>
                      <span className={styles.covTrack}><span className={styles.covFill} style={{ width: `${Math.round(ratio * 100)}%` }} /></span>
                      <span className={styles.covLabel}>{asset.conversations ? `${asset.covered}/${asset.conversations}` : (zh ? '未测试' : 'Untested')}</span>
                    </div>
                  </div>
                )
              })}
            </div>
            {assets.length > 6 ? <div className={styles.cardFoot}>{zh ? `仅显示前 6 个资产，共 ${assets.length} 个` : `Showing 6 of ${assets.length} assets`}</div> : null}
            {untested > 0 ? <div className={styles.cardFoot}>{zh ? `${untested} 个资产还没有任何测试会话` : `${untested} assets have no session yet`}</div> : null}
          </section>

          <section className={styles.card}>
            <div className={styles.cardHead}>
              <h3 className={styles.cardTitle}>{zh ? '最近证据' : 'Latest evidence'}</h3>
              <span className={styles.hint}>{source === 'all' ? (zh ? '全部会话' : 'All sessions') : (zh ? '当前会话' : 'Current session')}</span>
              <Button variant="ghost" size="sm" className={styles.cardHeadRight} onClick={() => onOpenEvidence(undefined)}>{zh ? '查看全部' : 'View all'} ↗</Button>
            </div>
            <div className={styles.timeline}>
              {recent.length === 0 ? <p className={styles.empty}>{zh ? '暂无证据记录' : 'No evidence recorded yet'}</p> : recent.map(item => (
                <div key={item.id} className={styles.tl}>
                  <span className={styles.tlTime}>{clock(item.updatedAt || item.createdAt, zh)}</span>
                  <div className={styles.tlBody}>
                    <div className={styles.tlTitle}>{item.title}</div>
                    <div className={styles.tlMeta}>
                      {item.kind === 'finding' ? (zh ? '发现' : 'Finding') : item.kind === 'blocker' ? (zh ? '阻塞' : 'Blocker') : (zh ? '事实' : 'Fact')}
                      {item.sourceTitle ? ` · ${item.sourceTitle}` : ''}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </section>

          <Button variant="outline" size="sm" onClick={onOpenPlan}>{zh ? '打开编排' : 'Open orchestration'}</Button>
        </div>
      </div>
    </div>
  )
}
