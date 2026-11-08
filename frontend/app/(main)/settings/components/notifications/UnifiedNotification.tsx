'use client'

import { useEffect, useMemo, useState } from 'react'
import Image from 'next/image'
import { Check, CircleAlert, Link2, Mail } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import toast from 'react-hot-toast'
import { useI18n } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PasswordInput } from '@/components/ui/password-input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

type NotificationEvent = 'vulnerability_found' | 'scan_complete'
type NotificationType = 'dingtalk' | 'wechat' | 'feishu' | 'webhook' | 'jira' | 'email'

type NotificationChannel = {
  id: string
  type: NotificationType
  name: string
  enabled: boolean
  events: NotificationEvent[]
  webhookUrl: string
  secret: string
  baseUrl: string
  projectKey: string
  username: string
  token: string
  issueType: string
  smtpHost: string
  smtpPort: number
  smtpUsername: string
  smtpPassword: string
  from: string
  recipients: string
}

const eventDefaults: NotificationEvent[] = ['vulnerability_found', 'scan_complete']

const channelDefaults: NotificationChannel[] = [
  { id: 'dingtalk', type: 'dingtalk', name: 'DingTalk', enabled: false, events: eventDefaults, webhookUrl: '', secret: '', baseUrl: '', projectKey: '', username: '', token: '', issueType: '', smtpHost: '', smtpPort: 587, smtpUsername: '', smtpPassword: '', from: '', recipients: '' },
  { id: 'wechat', type: 'wechat', name: 'WeCom', enabled: false, events: eventDefaults, webhookUrl: '', secret: '', baseUrl: '', projectKey: '', username: '', token: '', issueType: '', smtpHost: '', smtpPort: 587, smtpUsername: '', smtpPassword: '', from: '', recipients: '' },
  { id: 'feishu', type: 'feishu', name: 'Feishu', enabled: false, events: eventDefaults, webhookUrl: '', secret: '', baseUrl: '', projectKey: '', username: '', token: '', issueType: '', smtpHost: '', smtpPort: 587, smtpUsername: '', smtpPassword: '', from: '', recipients: '' },
  { id: 'webhook', type: 'webhook', name: 'Webhook', enabled: false, events: eventDefaults, webhookUrl: '', secret: '', baseUrl: '', projectKey: '', username: '', token: '', issueType: '', smtpHost: '', smtpPort: 587, smtpUsername: '', smtpPassword: '', from: '', recipients: '' },
  { id: 'jira', type: 'jira', name: 'Jira', enabled: false, events: eventDefaults, webhookUrl: '', secret: '', baseUrl: '', projectKey: '', username: '', token: '', issueType: 'Task', smtpHost: '', smtpPort: 587, smtpUsername: '', smtpPassword: '', from: '', recipients: '' },
  { id: 'email', type: 'email', name: 'Email', enabled: false, events: eventDefaults, webhookUrl: '', secret: '', baseUrl: '', projectKey: '', username: '', token: '', issueType: '', smtpHost: '', smtpPort: 587, smtpUsername: '', smtpPassword: '', from: '', recipients: '' },
]

const eventLabels = {
  zh: { vulnerability_found: '发现漏洞', scan_complete: '扫描完成' },
  en: { vulnerability_found: 'Vulnerability found', scan_complete: 'Scan completed' },
}

const typeLabels = {
  zh: { dingtalk: '钉钉', wechat: '企业微信', feishu: '飞书', webhook: 'Webhook', jira: 'Jira', email: '邮件' },
  en: { dingtalk: 'DingTalk', wechat: 'WeCom', feishu: 'Feishu', webhook: 'Webhook', jira: 'Jira', email: 'Email' },
}

const typeIconAssets: Partial<Record<NotificationType, string>> = {
  dingtalk: '/icon/dingding.svg',
  wechat: '/icon/wechat.svg',
  feishu: '/icon/feishu.svg',
  jira: '/icon/jira.svg',
}

const typeIconComponents: Partial<Record<NotificationType, typeof Link2>> = { webhook: Link2, email: Mail }

function ChannelIcon({ type, className = 'h-4 w-4' }: { type: NotificationType; className?: string }) {
  const asset = typeIconAssets[type]
  if (asset) return <Image src={asset} alt="" aria-hidden="true" width={16} height={16} unoptimized className={className} />
  const Icon = typeIconComponents[type] || Link2
  return <Icon aria-hidden="true" className={className} />
}

function normalizeChannels(value: unknown): NotificationChannel[] {
  const incoming = Array.isArray(value) ? value as Partial<NotificationChannel>[] : []
  return channelDefaults.map((fallback) => {
    const found = incoming.find((item) => item.type === fallback.type || item.id === fallback.id)
    return {
      ...fallback,
      ...(found || {}),
      events: Array.isArray(found?.events) ? found.events as NotificationEvent[] : fallback.events,
      smtpPort: Number(found?.smtpPort || fallback.smtpPort),
    }
  })
}

export function UnifiedNotification() {
  const { language } = useI18n()
  const zh = language === 'zh'
  const copy = zh ? {
    title: '统一通知中心', description: '为不同事件配置多个通知渠道，所有渠道共享同一套事件模型。', enabled: '启用渠道', events: '订阅事件', vulnerability: '漏洞发现', complete: '扫描完成', save: '保存全部配置', test: '发送测试', saved: '通知配置已保存', tested: '测试通知已发送', loadFailed: '加载通知配置失败', saveFailed: '保存通知配置失败', testFailed: '发送测试通知失败', config: '渠道配置', webhook: 'Webhook 地址', secret: '签名密钥（可选）', jiraUrl: 'Jira 地址', projectKey: '项目 Key', username: '用户名', token: 'API Token', issueType: 'Issue 类型', smtpHost: 'SMTP 主机', smtpPort: 'SMTP 端口', smtpUsername: 'SMTP 用户名', smtpPassword: 'SMTP 密码', from: '发件人', recipients: '收件人（逗号或分号分隔）', loading: '正在加载…', saveHint: '保存后，扫描任务和漏洞事件会按订阅配置自动分发。', invalid: '请完善当前渠道的必要配置。',
  } : {
    title: 'Unified Notifications', description: 'Configure multiple channels with one event model for every notification.', enabled: 'Enable channel', events: 'Subscribed events', vulnerability: 'Vulnerability found', complete: 'Scan completed', save: 'Save all configuration', test: 'Send test', saved: 'Notification configuration saved', tested: 'Test notification sent', loadFailed: 'Failed to load notification configuration', saveFailed: 'Failed to save notification configuration', testFailed: 'Failed to send test notification', config: 'Channel configuration', webhook: 'Webhook URL', secret: 'Signing secret (optional)', jiraUrl: 'Jira URL', projectKey: 'Project key', username: 'Username', token: 'API token', issueType: 'Issue type', smtpHost: 'SMTP host', smtpPort: 'SMTP port', smtpUsername: 'SMTP username', smtpPassword: 'SMTP password', from: 'From', recipients: 'Recipients (comma or semicolon separated)', loading: 'Loading…', saveHint: 'After saving, scan and vulnerability events are dispatched according to subscriptions.', invalid: 'Complete the required fields for this channel.',
  }
  const [channels, setChannels] = useState<NotificationChannel[]>(channelDefaults)
  const [active, setActive] = useState<NotificationType>('dingtalk')
  const channelHints: Partial<Record<NotificationType, string>> = zh
    ? { jira: '该渠道会为事件创建 Jira Issue。' }
    : { jira: 'This channel creates a Jira issue for each event.' }
  const channelHint = channelHints[active]
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  useEffect(() => {
    let cancelled = false
    void fetchWithAuth('/api/v1/config/notifications').then(async (response) => {
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      const result = await response.json()
      if (!result.success) throw new Error(result.error || copy.loadFailed)
      if (!cancelled) setChannels(normalizeChannels(result.data?.channels))
    }).catch((error) => {
      if (!cancelled) toast.error(`${copy.loadFailed}: ${error instanceof Error ? error.message : String(error)}`)
    }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [copy.loadFailed])

  const current = useMemo(() => channels.find((channel) => channel.type === active) || channelDefaults[0], [active, channels])
  const updateCurrent = (updates: Partial<NotificationChannel>) => setChannels((old) => old.map((channel) => channel.type === active ? { ...channel, ...updates } : channel))
  const toggleEvent = (event: NotificationEvent, checked: boolean) => updateCurrent({ events: checked ? [...new Set([...current.events, event])] : current.events.filter((item) => item !== event) })

  const save = async () => {
    setSaving(true)
    try {
      const response = await fetchWithAuth('/api/v1/config/notifications', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ version: 1, channels }) })
      const result = await response.json()
      if (!response.ok || !result.success) throw new Error(result.error || copy.saveFailed)
      toast.success(copy.saved)
    } catch (error) { toast.error(`${copy.saveFailed}: ${error instanceof Error ? error.message : String(error)}`) } finally { setSaving(false) }
  }

  const test = async () => {
    setTesting(true)
    try {
      const response = await fetchWithAuth('/api/v1/config/notifications/test', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ channel: current }) })
      const result = await response.json()
      if (!response.ok || !result.success) throw new Error(result.error || copy.testFailed)
      toast.success(copy.tested)
    } catch (error) { toast.error(`${copy.testFailed}: ${error instanceof Error ? error.message : String(error)}`) } finally { setTesting(false) }
  }

  if (loading) return <div className="p-6 text-sm text-muted-foreground">{copy.loading}</div>

  return <div className="space-y-6 p-1">
    <Tabs value={active} onValueChange={(value) => setActive(value as NotificationType)}>
      <TabsList className="grid h-auto w-full grid-cols-2 gap-1 sm:grid-cols-3 lg:grid-cols-6">
        {channels.map((channel) => <TabsTrigger key={channel.type} value={channel.type} className="gap-1 px-2 text-xs sm:gap-2 sm:text-sm"><ChannelIcon type={channel.type} /><span>{typeLabels[language === 'zh' ? 'zh' : 'en'][channel.type]}</span></TabsTrigger>)}
      </TabsList>
      <TabsContent value={active} className="mt-4 space-y-4">
        <Card><CardHeader><div className="flex items-start justify-between gap-4"><div><CardTitle className="flex items-center gap-2"><ChannelIcon type={current.type} className="h-5 w-5" />{typeLabels[language === 'zh' ? 'zh' : 'en'][current.type]}</CardTitle>{channelHint ? <CardDescription className="mt-2">{channelHint}</CardDescription> : null}</div><div className="flex items-center gap-2 text-sm"><Label htmlFor={`enabled-${current.type}`}>{copy.enabled}</Label><Switch id={`enabled-${current.type}`} checked={current.enabled} onCheckedChange={(checked) => updateCurrent({ enabled: checked })} /></div></div></CardHeader><CardContent className="space-y-6">
          <div className="grid gap-4 md:grid-cols-2">
            {current.type === 'dingtalk' || current.type === 'wechat' || current.type === 'feishu' || current.type === 'webhook' ? <><Field label={copy.webhook} value={current.webhookUrl} onChange={(value) => updateCurrent({ webhookUrl: value })} placeholder="https://…" /><Field label={copy.secret} value={current.secret} secret onChange={(value) => updateCurrent({ secret: value })} /></> : null}
            {current.type === 'jira' ? <><Field label={copy.jiraUrl} value={current.baseUrl} onChange={(value) => updateCurrent({ baseUrl: value })} placeholder="https://jira.example.com" /><Field label={copy.projectKey} value={current.projectKey} onChange={(value) => updateCurrent({ projectKey: value })} /><Field label={copy.username} value={current.username} onChange={(value) => updateCurrent({ username: value })} /><Field label={copy.token} value={current.token} secret onChange={(value) => updateCurrent({ token: value })} /><Field label={copy.issueType} value={current.issueType} onChange={(value) => updateCurrent({ issueType: value })} /></> : null}
            {current.type === 'email' ? <><Field label={copy.smtpHost} value={current.smtpHost} onChange={(value) => updateCurrent({ smtpHost: value })} placeholder="smtp.example.com" /><Field label={copy.smtpPort} value={String(current.smtpPort || '')} onChange={(value) => updateCurrent({ smtpPort: Number(value) || 0 })} type="number" /><Field label={copy.smtpUsername} value={current.smtpUsername} onChange={(value) => updateCurrent({ smtpUsername: value })} /><Field label={copy.smtpPassword} value={current.smtpPassword} secret onChange={(value) => updateCurrent({ smtpPassword: value })} /><Field label={copy.from} value={current.from} onChange={(value) => updateCurrent({ from: value })} type="email" /><Field label={copy.recipients} value={current.recipients} onChange={(value) => updateCurrent({ recipients: value })} /></> : null}
          </div>
          <div className="space-y-3 rounded-xl border border-border/70 bg-muted/20 p-4"><div className="flex items-center gap-2 text-sm font-medium"><CircleAlert className="h-4 w-4 text-muted-foreground" />{copy.events}</div><div className="grid gap-3 sm:grid-cols-2"><EventToggle label={zh ? eventLabels.zh.vulnerability_found : eventLabels.en.vulnerability_found} checked={current.events.includes('vulnerability_found')} onCheckedChange={(checked) => toggleEvent('vulnerability_found', checked)} /><EventToggle label={zh ? eventLabels.zh.scan_complete : eventLabels.en.scan_complete} checked={current.events.includes('scan_complete')} onCheckedChange={(checked) => toggleEvent('scan_complete', checked)} /></div></div>
          <div className="flex flex-wrap justify-end gap-2"><Button variant="outline" type="button" onClick={test} disabled={testing}>{testing ? copy.loading : copy.test}</Button><Button type="button" onClick={save} disabled={saving}>{saving ? copy.loading : <><Check className="mr-2 h-4 w-4" />{copy.save}</>}</Button></div>
        </CardContent></Card>
        <p className="text-xs text-muted-foreground">{copy.saveHint}</p>
      </TabsContent>
    </Tabs>
  </div>
}

function Field({ label, value, onChange, secret = false, type = 'text', placeholder }: { label: string; value: string; onChange: (value: string) => void; secret?: boolean; type?: string; placeholder?: string }) {
  return <div className="space-y-2"><Label>{label}</Label>{secret ? <PasswordInput value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} /> : <Input type={type} value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} />}</div>
}

function EventToggle({ label, checked, onCheckedChange }: { label: string; checked: boolean; onCheckedChange: (checked: boolean) => void }) {
  return <div className="flex items-center justify-between rounded-lg border border-border/60 bg-background px-3 py-2.5 text-sm"><span>{label}</span><Switch checked={checked} onCheckedChange={onCheckedChange} /></div>
}
