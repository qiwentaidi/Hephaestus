'use client'

import Link from 'next/link'
import { useSearchParams, useRouter } from 'next/navigation'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ArrowLeft, ArrowUpRight, Bug, Copy, Link2, Loader2, RefreshCw, Send } from 'lucide-react'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import { fetchWithAuth } from '@/lib/api'
import { getCenterWebDetail, updateCenterFindingStatus } from '@/lib/api/vulnerability-center'
import type { AIWorkbenchFindingStatus } from '@/lib/api/ai-workbench'
import { parseScanFindingReference } from '@/lib/scan-finding-detail'
import { findingReturnHref } from '@/lib/finding-detail'
import { copyToClipboard } from '@/lib/clipboard'
import { formatDateTime } from '@/lib/date'
import { useI18n } from '@/lib/i18n'
import { getFindingLabels } from '@/lib/finding-i18n'
import { findingSeverityBadgeClassName, findingStatusBadgeClassName } from '@/lib/finding-badge'
import { cn } from '@/lib/utils'
import type { VulnerabilityInfo } from '../../scan/vuln-scan/types'
import type { JsScanVulnerability } from '../../scan/vuln-scan/components/JsScanVulnerabilityTable'
import { DetectionMethodBadge } from '../../scan/vuln-scan/components/DetectionMethodBadge'
import { VulnerabilityHttpEvidence } from '../../scan/vuln-scan/components/VulnerabilityHttpEvidence'
import styles from './report.module.css'

const copy = {
  zh: {
    back: '返回', title: '漏洞详情', share: '复制链接', refresh: '刷新', summary: '摘要与影响', description: '漏洞说明', impact: '影响',
    target: '涉及目标', evidence: '请求 / 响应', contexts: 'JavaScript 调用上下文', remediation: '修复建议',
    contents: '本页目录', metadata: '漏洞信息', status: '处置状态', type: '漏洞类型', id: '漏洞 ID',
    template: '模板 ID', protocol: '协议类型', task: '来源任务', version: '扫描版本', scan: '第 {n} 次扫描',
    discovered: '发现时间', method: '请求方法', responseType: '响应类型', responseLength: '响应长度',
    confidence: '扫描置信度', confidenceReason: '置信度说明', references: '参考链接', extract: '提取结果',
    unknown: '未记录', loading: '正在加载漏洞详情', failed: '无法加载漏洞详情', invalid: '链接缺少有效的漏洞标识或扫描版本。',
    retry: '重试', copied: '已复制', copyFailed: '复制失败', copy: '复制', copySection: '复制本节', saved: '状态已更新', saveFailed: '状态更新失败',
    forward: '发送到请求转发', request: '请求包 (Request)', response: '响应包 (Response)', format: '格式化 JSON', sourceFile: '来源文件', snippet: '调用片段',
    noEvidence: '此记录未保存请求或响应报文。', high: '高', medium: '中', low: '低',
  },
  en: {
    back: 'Back', title: 'Finding details', share: 'Copy link', refresh: 'Refresh', summary: 'Summary & impact', description: 'Description', impact: 'Impact',
    target: 'Targets', evidence: 'Request / response', contexts: 'JavaScript call context', remediation: 'Remediation',
    contents: 'On this page', metadata: 'Finding information', status: 'Disposition', type: 'Vulnerability type', id: 'Finding ID',
    template: 'Template ID', protocol: 'Protocol', task: 'Source task', version: 'Scan version', scan: 'Scan {n}',
    discovered: 'Discovered', method: 'Request method', responseType: 'Response type', responseLength: 'Response length',
    confidence: 'Scanner confidence', confidenceReason: 'Confidence rationale', references: 'References', extract: 'Extracted results',
    unknown: 'Not recorded', loading: 'Loading finding details', failed: 'Unable to load finding', invalid: 'The link is missing a valid finding identity or scan version.',
    retry: 'Retry', copied: 'Copied', copyFailed: 'Copy failed', copy: 'Copy', copySection: 'Copy section', saved: 'Status updated', saveFailed: 'Unable to update status',
    forward: 'Send to Repeater', request: 'Request', response: 'Response', format: 'Format JSON', sourceFile: 'Source file', snippet: 'Call snippet',
    noEvidence: 'No request or response was saved for this record.', high: 'High', medium: 'Medium', low: 'Low',
  },
}
const dispositions: AIWorkbenchFindingStatus[] = ['open', 'in_progress', 'fixed', 'false_positive', 'ignored']
type Detail = { source: 'web'; record: VulnerabilityInfo } | { source: 'js'; record: JsScanVulnerability }

export default function ScanFindingDetailClient() {
  const params = useSearchParams()
  const query = params.toString()
  const reference = useMemo(() => parseScanFindingReference(new URLSearchParams(query)), [query])
  const back = findingReturnHref(params.get('from'))
  const { language, t } = useI18n()
  const l = copy[language]
  const labels = getFindingLabels(t.vulnerabilityManagement)
  const router = useRouter()
  const [detail, setDetail] = useState<Detail | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const [saving, setSaving] = useState(false)
  const [formatJSON, setFormatJSON] = useState(false)
  const [activeSection, setActiveSection] = useState('summary')
  const pageRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!reference) { setLoading(false); return }
    const controller = new AbortController()
    setLoading(true)
    setError('')
    void (async () => {
      if (reference.source === 'web') {
        const record = await getCenterWebDetail(reference, controller.signal)
        if (!controller.signal.aborted) setDetail({ source: 'web', record })
      } else {
        const response = await fetchWithAuth(`/api/v1/jsscan/vulnerabilities/${encodeURIComponent(reference.taskId)}/${encodeURIComponent(reference.nativeId)}?version=${reference.version}`, { signal: controller.signal })
        const body = await response.json()
        if (!response.ok || !body.success || !body.data) throw new Error(body.error || `HTTP ${response.status}`)
        if (!controller.signal.aborted) setDetail({ source: 'js', record: body.data })
      }
    })().catch(e => { if (!controller.signal.aborted) setError(e instanceof Error ? e.message : String(e)) })
      .finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [reference, revision])

  useEffect(() => {
    if (detail?.source !== 'js') return
    const update = () => {
      const nodes = [...(pageRef.current?.querySelectorAll<HTMLElement>('[data-report-section]') || [])]
      const active = nodes.filter(node => node.getBoundingClientRect().top <= 160).at(-1) || nodes[0]
      if (active) setActiveSection(active.dataset.reportSection || 'summary')
    }
    update()
    window.addEventListener('scroll', update, true)
    window.addEventListener('resize', update)
    return () => { window.removeEventListener('scroll', update, true); window.removeEventListener('resize', update) }
  }, [detail])

  const web = detail?.source === 'web' ? detail.record : null
  const js = detail?.source === 'js' ? detail.record : null
  const name = web?.vulnName || js?.title || reference?.name || l.title
  const severity = (web?.severity || js?.level || 'info').toLowerCase()
  const status = detail?.record.disposalStatus || 'open'
  const target = web?.vulnUrl || js?.url || ''
  const createdAt = js?.createdAt || reference?.createdAt
  const contexts = js?.staticContexts || []
  const sections = [
    { key: 'summary', label: l.summary },
    { key: 'evidence', label: l.evidence },
    ...(contexts.length ? [{ key: 'contexts', label: l.contexts }] : []),
  ]
  const copyValue = async (value: string) => {
    if (await copyToClipboard(value)) toast.success(l.copied)
    else toast.error(l.copyFailed)
  }
  const copyButton = (value: string, label = l.copySection) => value.trim() ? <Button variant="ghost" size="sm" className="h-8 shrink-0 gap-1.5 text-xs text-muted-foreground" onClick={() => void copyValue(value)}><Copy className="h-3.5 w-3.5" />{label}</Button> : null
  const changeStatus = async (value: string) => {
    if (!reference || !detail || !dispositions.includes(value as AIWorkbenchFindingStatus)) return
    setSaving(true)
    try {
      await updateCenterFindingStatus({ ...reference, nativeId: detail.source === 'web' ? detail.record.id : reference.nativeId }, value as AIWorkbenchFindingStatus)
      setDetail(prev => prev ? { ...prev, record: { ...prev.record, disposalStatus: value as AIWorkbenchFindingStatus } } as Detail : prev)
      toast.success(l.saved)
    } catch { toast.error(l.saveFailed) } finally { setSaving(false) }
  }
  let formattedResponse: string | null = null
  if (js?.response) { try { formattedResponse = JSON.stringify(JSON.parse(js.response), null, 2) } catch {} }
  const canForward = Boolean(js?.request && /^(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|TRACE|CONNECT)\s+/m.test(js.request))
  const sendToRepeater = () => {
    if (!canForward || !js?.request) return
    try { localStorage.setItem('repeater_pending_request', js.request); router.push('/scan/repeater'); toast.success(t.vulnScan.messages.sentToRepeater) }
    catch { toast.error(t.vulnScan.messages.sendFailed) }
  }
  const section = (key: string, title: string, children: ReactNode, value = '') => <section id={`scan-finding-${key}`} data-report-section={key} className={cn(styles.section, key === 'summary' && styles.summary)}>
    <div className={styles.sectionHeader}><h2>{title}</h2>{copyButton(value)}</div>{children}
  </section>
  const text = (value?: string) => <p className={cn('whitespace-pre-wrap break-words text-sm leading-7', !value?.trim() && 'text-muted-foreground')}>{value?.trim() || l.unknown}</p>
  const prose = (value: string) => <div className="space-y-2 text-sm leading-6">{value.trim().split(/\n\s*\n/).map((paragraph, index) => <p key={index} className="whitespace-normal break-words">{paragraph}</p>)}</div>
  const taskHref = reference ? `/scan/vuln-scan/detail?${new URLSearchParams({ taskId: reference.taskId, version: String(reference.version), tab: reference.source === 'web' ? 'vulnerability' : 'vulnerabilities' })}` : ''
  const references = (web?.reference || '').split(/[\n,]/).map(v => v.trim()).filter(Boolean)
  const failure = !reference ? l.invalid : error

  return <div ref={pageRef} className="min-h-full bg-background text-foreground">
    <header className="border-b border-border bg-background px-5 py-5 lg:px-8">
      <div className="flex flex-wrap items-center gap-3">
        <Button asChild variant="ghost" size="sm" className="-ml-2 gap-1.5"><Link href={back}><ArrowLeft className="h-4 w-4" />{l.back}</Link></Button>
        <span className="hidden h-5 w-px bg-border sm:block" /><Bug className="hidden h-5 w-5 shrink-0 text-muted-foreground sm:block" />
        <h1 className="order-3 w-full min-w-0 break-words text-lg font-semibold leading-7 sm:order-none sm:w-auto sm:flex-1 sm:text-xl">{name}</h1>
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <Button variant="ghost" size="sm" disabled={!detail} className="gap-1.5 text-muted-foreground" onClick={() => { const url = new URL(window.location.href); url.searchParams.delete('from'); void copyValue(url.toString()) }}><Link2 className="h-4 w-4" />{l.share}</Button>
          <Button variant="ghost" size="icon" disabled={loading || !reference} aria-label={l.refresh} onClick={() => setRevision(v => v + 1)}><RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} /></Button>
        </div>
      </div>
      {detail && <div className="mt-3 flex flex-wrap items-center gap-2"><Badge variant="outline" className={findingSeverityBadgeClassName(severity)}>{labels.severity[severity] || severity}</Badge><Badge variant="outline" className={cn(findingStatusBadgeClassName(status), 'font-normal')}>{labels.status[status] || status}</Badge>{web && <DetectionMethodBadge principle={web.principDetection} labels={t.vulnScan.vulnDetail} />}<span className="break-all font-mono text-xs text-muted-foreground">{web?.id || js?.id}</span></div>}
    </header>
    {loading && !detail ? <div role="status" className="flex items-center justify-center gap-3 p-16 text-sm text-muted-foreground"><Loader2 className="h-5 w-5 animate-spin" />{l.loading}</div> : failure ? <div role="alert" className="m-6 rounded-xl border bg-card p-8 text-center"><h2 className="font-semibold">{l.failed}</h2><p className="my-3 break-words text-sm text-muted-foreground">{failure}</p>{reference && <Button variant="outline" onClick={() => setRevision(v => v + 1)}>{l.retry}</Button>}</div> : detail && reference && <div className="mx-auto grid w-full max-w-[1600px] items-start gap-6 p-4 md:p-6 xl:grid-cols-[minmax(0,1fr)_256px] 2xl:grid-cols-[minmax(0,1fr)_280px] 2xl:gap-9">
      <main className={cn(styles.report, web && styles.webReport)}>
        {web ? <>
          {(web.description?.trim() || web.impact?.trim()) && section('summary', l.description,
            <div className="space-y-3">
              {web.description?.trim() && prose(web.description)}
              {web.impact?.trim() && <div><h3 className="mb-1 text-xs font-medium text-muted-foreground">{l.impact}</h3>{prose(web.impact)}</div>}
            </div>, [web.description, web.impact].filter(Boolean).join('\n\n'))}
        </> : section('summary', l.summary, text(detail.record.description), detail.record.description || '')}
        {section('evidence', l.evidence, web ? <VulnerabilityHttpEvidence key={`${query}:${revision}`} vulnerability={web} /> : js?.request || js?.response ? <div className="grid min-w-0 gap-4 lg:grid-cols-2">
          {js?.request && <div className="min-w-0 overflow-hidden rounded-lg border bg-muted/20"><div className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b px-3 py-2"><h3 className="text-sm font-medium">{l.request}</h3><div className="flex items-center gap-1">{copyButton(js.request, l.copy)}{canForward && <Button variant="ghost" size="sm" className="h-8 gap-1.5 text-xs" onClick={sendToRepeater}><Send className="h-3.5 w-3.5" />{l.forward}</Button>}</div></div><pre className="max-h-[480px] overflow-auto whitespace-pre-wrap break-all p-4 font-mono text-xs leading-6">{js.request}</pre></div>}
          {js?.response && <div className="min-w-0 overflow-hidden rounded-lg border bg-muted/20"><div className="flex min-h-12 flex-wrap items-center justify-between gap-2 border-b px-3 py-2"><h3 className="text-sm font-medium">{l.response}</h3><div className="flex flex-wrap items-center gap-2">{formattedResponse !== null && <div className="flex items-center gap-2"><Switch id="scan-json-format" checked={formatJSON} onCheckedChange={setFormatJSON} /><Label htmlFor="scan-json-format" className="text-xs">{l.format}</Label></div>}{copyButton(js.response, l.copy)}</div></div><pre className="max-h-[480px] overflow-auto whitespace-pre-wrap break-all p-4 font-mono text-xs leading-6">{formatJSON ? formattedResponse : js.response}</pre></div>}
        </div> : <p className="text-sm text-muted-foreground">{l.noEvidence}</p>)}
        {contexts.length > 0 && section('contexts', l.contexts, <div className="space-y-5">{contexts.map((context, index) => <div key={`${context.sourceUrl}:${index}`} className="min-w-0"><div className="mb-2 flex items-center justify-between gap-2"><span className="text-xs text-muted-foreground">{l.sourceFile}</span>{copyButton(context.sourceUrl || '', l.copy)}</div><p className="mb-3 break-all font-mono text-xs leading-6 text-muted-foreground">{context.sourceUrl || l.unknown}</p><div className="overflow-hidden rounded-lg border bg-muted/20"><div className="flex items-center justify-between gap-2 border-b px-3 py-2"><span className="text-xs text-muted-foreground">{l.snippet}</span>{copyButton(context.snippet || '', l.copy)}</div><pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all p-4 font-mono text-xs leading-6">{context.snippet || l.unknown}</pre></div></div>)}</div>)}
        {web?.extract?.trim() && section('extract', l.extract, text(web.extract), web.extract)}
        {web?.remediation?.trim() && section('remediation', l.remediation, text(web.remediation), web.remediation)}
        {web && references.length > 0 && section('references', l.references, <ul className="space-y-2 text-sm">{references.map(ref => <li key={ref}>{/^https?:\/\//i.test(ref) ? <a href={ref} target="_blank" rel="noopener noreferrer" className="break-all underline underline-offset-4">{ref}</a> : ref}</li>)}</ul>)}
      </main>
      <aside className="flex min-h-0 min-w-0 flex-col gap-5 xl:sticky xl:top-6 xl:max-h-[calc(100dvh-80px)]">
        {js && <nav aria-label={l.contents} className="hidden shrink-0 border-b border-border/60 px-2 pb-5 xl:block"><h2 className="mb-3 text-xs font-semibold text-muted-foreground">{l.contents}</h2><ul className="space-y-1">{sections.map(item => <li key={item.key}><button aria-current={activeSection === item.key ? 'location' : undefined} className="w-full rounded-lg px-3 py-2 text-left text-sm text-muted-foreground hover:bg-muted hover:text-foreground aria-[current=location]:bg-muted aria-[current=location]:font-medium aria-[current=location]:text-foreground" onClick={() => document.getElementById(`scan-finding-${item.key}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })}>{item.label}</button></li>)}</ul></nav>}
        <section className="min-h-0 px-2 py-2 xl:overflow-y-auto"><h2 className="mb-3 text-xs font-semibold text-muted-foreground">{l.metadata}</h2><dl className="divide-y divide-border text-sm">
          <Meta label={l.id}><Mono>{detail.record.id || l.unknown}</Mono></Meta>
          <Meta label={l.status}><Select value={status} disabled={saving} onValueChange={changeStatus}><SelectTrigger aria-label={l.status} className="h-8 w-full border-0 bg-muted/40 text-xs shadow-none"><SelectValue /></SelectTrigger><SelectContent>{dispositions.map(value => <SelectItem key={value} value={value}>{labels.status[value]}</SelectItem>)}</SelectContent></Select></Meta>
          {web && <><Meta label={l.template}><Mono>{web.templateId || l.unknown}</Mono></Meta><Meta label={l.protocol}>{web.protocol || l.unknown}</Meta><Meta label={l.target}><Mono>{target || l.unknown}</Mono>{target && copyButton(target, l.copy)}</Meta></>}
          {js && <><Meta label={l.type}>{js.type || l.unknown}</Meta><Meta label={l.target}><Mono>{target || l.unknown}</Mono>{target && copyButton(target, l.copy)}</Meta><Meta label={l.method}>{js.method || l.unknown}</Meta><Meta label={l.responseType}>{js.responseType || l.unknown}</Meta><Meta label={l.responseLength}>{typeof js.responseLength === 'number' ? `${js.responseLength.toLocaleString()} B` : l.unknown}</Meta><Meta label={l.confidence}>{js.confidence ? ({ high: l.high, medium: l.medium, low: l.low }[js.confidence] || js.confidence) : l.unknown}</Meta>{js.confidenceReason && <Meta label={l.confidenceReason}>{js.confidenceReason}</Meta>}</>}
          <Meta label={l.task}><Link href={taskHref} className="break-all underline underline-offset-4">{reference.taskName}<ArrowUpRight className="ml-1 inline h-3 w-3" /></Link></Meta>
          <Meta label={l.version}>{l.scan.replace('{n}', String(reference.version))}</Meta>
          {(js || createdAt) && <Meta label={l.discovered}>{createdAt ? formatDateTime(createdAt) : l.unknown}</Meta>}
        </dl></section>
      </aside>
    </div>}
  </div>
}
function Meta({ label, children }: { label: string; children: ReactNode }) {
  return <div className="space-y-1.5 py-3"><dt className="text-xs leading-6 text-muted-foreground">{label}</dt><dd className="min-w-0 break-words text-xs leading-6">{children}</dd></div>
}
function Mono({ children }: { children: ReactNode }) { return <span className="whitespace-pre-wrap break-all font-mono text-xs">{children}</span> }
