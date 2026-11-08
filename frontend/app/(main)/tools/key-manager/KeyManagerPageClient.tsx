'use client'

import mobileTable from '@/components/ui/mobile-record-table.module.css'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useRouter, useSearchParams } from 'next/navigation'
import { Plus, Search, Save, ArrowRight, Pencil, Trash2, Send, ChevronLeft, Slack, Bird, Loader2, KeyRound } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { useToast } from '@/components/ui/use-toast'
import { fetchWithAuth } from '@/lib/api'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { RequestEditor } from '@/app/(main)/scan/repeater/request-editor'
import { ResponseViewer } from '@/app/(main)/scan/repeater/response-viewer'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

const WechatIcon = ({ className }: { className?: string }) => (
	<svg className={className} viewBox="0 0 24 24" fill="currentColor">
		<path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 0 1 .213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.326.326 0 0 0 .167-.054l1.903-1.114a.864.864 0 0 1 .717-.098 10.16 10.16 0 0 0 2.837.403c.276 0 .543-.027.811-.05-.857-2.578.157-4.972 1.932-6.446 1.703-1.415 3.882-1.98 5.853-1.838-.576-3.583-4.196-6.348-8.596-6.348zM5.785 5.991c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178A1.17 1.17 0 0 1 4.623 7.17c0-.651.52-1.18 1.162-1.18zm5.813 0c.642 0 1.162.529 1.162 1.18a1.17 1.17 0 0 1-1.162 1.178 1.17 1.17 0 0 1-1.162-1.178c0-.651.52-1.18 1.162-1.18zm5.34 2.867c-1.797-.052-3.746.512-5.28 1.786-1.72 1.428-2.687 3.72-1.78 6.22.942 2.453 3.666 4.229 6.884 4.229.826 0 1.622-.12 2.361-.336a.722.722 0 0 1 .598.082l1.584.926a.272.272 0 0 0 .14.047c.134 0 .24-.111.24-.247 0-.06-.023-.12-.038-.177l-.327-1.233a.582.582 0 0 1-.023-.156.49.49 0 0 1 .201-.398C23.024 18.48 24 16.82 24 14.98c0-3.21-2.931-5.837-6.656-6.088V8.87l-.005-.012h-.4zm-2.53 3.274c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.97-.982zm4.844 0c.535 0 .969.44.969.982a.976.976 0 0 1-.969.983.976.976 0 0 1-.969-.983c0-.542.434-.982.969-.982z" />
	</svg>
)

const WeComIcon = ({ className }: { className?: string }) => (
	<svg className={className} viewBox="0 0 1228 1024" fill="currentColor">
		<path d="M1045.84 747.027a153.563 153.563 0 0 0-53.156 21.515 129.094 129.094 0 0 1-58.092 35.1c2.953-19.828 12.783-37.926 27.633-51.3a191.186 191.186 0 0 0 26.452-62.142 56.953 56.953 0 1 1 57.164 56.827zM941.639 610.634a190.814 190.814 0 0 0-61.932-26.747 56.953 56.953 0 1 1 56.953-56.953 155.266 155.266 0 0 0 21.263 53.325 129.666 129.666 0 0 1 34.762 58.346 85.978 85.978 0 0 1-50.878-27.97h-0.21z m-93.826-200.728c-17.17-143.817-166.092-256.5-346.274-256.5-191.954 0-348.132 127.744-348.132 284.85a266.33 266.33 0 0 0 124.369 216.169 351.762 351.762 0 0 0 37.969 24.384l-15.44 61.636c5.568 2.616 10.968 5.4 16.663 7.805l77.963-38.981c11.39 2.953 23.372 4.851 35.268 6.876 7.594 1.35 15.188 2.742 22.993 3.67a401.119 401.119 0 0 0 145.547-8.353 281.011 281.011 0 0 0 11.474 62.185 481.153 481.153 0 0 1-108.675 12.698 472.5 472.5 0 0 1-97.621-10.758L262.46 846.21a31.219 31.219 0 0 1-33.877-3.543 31.64 31.64 0 0 1-10.926-32.316l25.312-101.925A330.075 330.075 0 0 1 90.125 438.256c0-192.29 184.19-348.131 411.413-348.131 215.746 0 392.428 140.653 409.64 319.444a276.919 276.919 0 0 0-29.91-2.953c-11.18 0.422-22.36 1.476-33.456 3.248zM716.399 634.47c18.943-3.797 36.957-11.053 53.157-21.515a129.094 129.094 0 0 1 58.134-35.016 86.358 86.358 0 0 1-27.675 51.216c-12.445 18.984-21.389 40.078-26.451 62.184a56.953 56.953 0 1 1-57.165-56.869z m102.6 137.025c18.816 12.614 39.741 21.727 61.763 27a56.953 56.953 0 1 1-56.953 56.953 154.406 154.406 0 0 0-21.094-53.409 129.558 129.558 0 0 1-34.51-58.514 85.888 85.888 0 0 1 50.794 28.308v-0.338z" />
	</svg>
)

const DingtalkIcon = ({ className }: { className?: string }) => (
	<svg className={className} viewBox="0 0 1024 1024" fill="currentColor">
		<path d="M512 64C264.6 64 64 264.6 64 512s200.6 448 448 448 448-200.6 448-448S759.4 64 512 64zm227 385.3c-1 4.2-3.5 10.4-7 17.8h.1l-.4.7c-20.3 43.1-73.1 127.7-73.1 127.7s-.1-.2-.3-.5l-15.5 26.8h74.5L575.1 810l32.3-128h-58.6l20.4-84.7c-16.5 3.9-35.9 9.4-59 16.8 0 0-31.2 18.2-89.9-35 0 0-39.6-34.7-16.6-43.4 9.8-3.7 47.4-8.4 77-12.3 40-5.4 64.6-8.2 64.6-8.2S422 517 392.7 512.5c-29.3-4.6-66.4-53.1-74.3-95.8 0 0-12.2-23.4 26.3-12.3 38.5 11.1 197.9 43.2 197.9 43.2S335.2 384.3 321.4 368.9c-13.8-15.4-40.6-84.2-37.1-126.5 0 0 1.5-10.5 12.4-7.7 0 0 153.3 69.7 258.1 107.9 104.8 37.9 195.9 57.3 184.2 106.7z" />
	</svg>
)

const DgWorkIcon = ({ className }: { className?: string }) => (
	<svg className={className} viewBox="0 0 24 24" fill="currentColor">
		<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 3c1.66 0 3 1.34 3 3s-1.34 3-3 3-3-1.34-3-3 1.34-3 3-3zm0 14.2c-2.5 0-4.71-1.28-6-3.22.03-1.99 4-3.08 6-3.08 1.99 0 5.97 1.09 6 3.08-1.29 1.94-3.5 3.22-6 3.22z" />
	</svg>
)

type KeyEntry = {
	id: string
	name: string
	type: string
	baseUrl: string
	credentials: string
	headers: string
	notes: string
	createdAt: string
	updatedAt: string
}

type CredentialState = {
	wechat: { appid: string; secret: string; accessToken: string }
	enterprise_wechat: { corpid: string; corpsecret: string; accessToken: string }
	dingtalk: { appid: string; secret: string; accessToken: string }
	dgwork: { appkey: string; secret: string; accessToken: string }
	feishu: { appId: string; appSecret: string; accessToken: string }
	slack: { token: string }
}

type ApiAction = {
	name: string
	method: string
	url: string
	body?: string
	auth?: 'bearer'
	tokenInQuery?: boolean
}

const wechatApiOptions: ApiAction[] = [
	{ name: 'queryDomainConfig', method: 'POST', url: 'https://api.weixin.qq.com/wxa/getwxadevinfo?access_token=', body: '{}' },
	{ name: 'getLongTermSubscribers', method: 'POST', url: 'https://api.weixin.qq.com/wxa/business/get_wxa_followers?access_token=', body: '{}' },
	{ name: 'getUserList', method: 'POST', url: 'https://api.weixin.qq.com/cgi-bin/user/get?count=10000&access_token=', body: '{}' },
	{ name: 'getUserFeedbackList', method: 'GET', url: 'https://api.weixin.qq.com/wxaapi/feedback/list?access_token=' },
	{ name: 'getTags', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/tags/get?access_token=' },
	{ name: 'getMenu', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/menu/get?access_token=' },
	{ name: 'getCurrentMenu', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/get_current_selfmenu_info?access_token=' },
	{ name: 'getAutoReplyRules', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/get_current_autoreply_info?access_token=' },
	{ name: 'getMaterialCount', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/material/get_materialcount?access_token=' },
	{ name: 'getTemplateList', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/template/get_all_private_template?access_token=' },
	{ name: 'getIndustryInfo', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/template/get_industry?access_token=' },
	{ name: 'getAccountBasicInfo', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/account/getaccountbasicinfo?access_token=' },
	{ name: 'getWxaSearchStatus', method: 'GET', url: 'https://api.weixin.qq.com/wxa/getwxasearchstatus?access_token=' },
	{ name: 'getMiniProgramCategory', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/wxopen/getcategory?access_token=' },
	{ name: 'getMiniProgramAllCategories', method: 'GET', url: 'https://api.weixin.qq.com/cgi-bin/wxopen/getallcategories?access_token=' },
]

const enterpriseWechatApiOptions: ApiAction[] = [
	{ name: 'getMemberIdList', method: 'POST', url: 'https://qyapi.weixin.qq.com/cgi-bin/user/list_id?access_token=', body: '{"limit": 10000}' },
	{ name: 'getDepartmentList', method: 'GET', url: 'https://qyapi.weixin.qq.com/cgi-bin/department/list?access_token=' },
	{ name: 'getDepartmentMembers', method: 'GET', url: 'https://qyapi.weixin.qq.com/cgi-bin/user/simplelist?department_id=1&access_token=' },
]

const dingtalkApiOptions: ApiAction[] = [
	{ name: 'getEmployeeCount', method: 'POST', url: 'https://oapi.dingtalk.com/topapi/user/count?access_token=', body: '{"only_active":"true"}' },
	{ name: 'getAdminList', method: 'GET', url: 'https://oapi.dingtalk.com/topapi/user/listadmin?access_token=' },
	{ name: 'getDepartmentUserInfo', method: 'POST', url: 'https://oapi.dingtalk.com/topapi/v2/user/list?access_token=', body: '{"dept_id":1,"cursor":0,"size":100}' },
]

const feishuApiOptions: ApiAction[] = [
	{ name: 'feishuGetScopes', method: 'GET', url: 'https://open.feishu.cn/open-apis/contact/v3/scopes', auth: 'bearer', tokenInQuery: false },
]

const slackApiOptions: ApiAction[] = [
	{ name: 'slackAuthTest', method: 'POST', url: 'https://slack.com/api/auth.test', auth: 'bearer', tokenInQuery: false },
	{ name: 'slackListConversations', method: 'GET', url: 'https://slack.com/api/conversations.list?limit=20', auth: 'bearer', tokenInQuery: false },
]

const maskSecret = (value: string) => {
	if (!value) return '-'
	if (value.length <= 20) return `${value.slice(0, 2)}***${value.slice(-2)}`
	return `${value.slice(0, 10)}******${value.slice(-10)}`
}

const emptyCredentials: CredentialState = {
	wechat: { appid: '', secret: '', accessToken: '' },
	enterprise_wechat: { corpid: '', corpsecret: '', accessToken: '' },
	dingtalk: { appid: '', secret: '', accessToken: '' },
	dgwork: { appkey: '', secret: '', accessToken: '' },
	feishu: { appId: '', appSecret: '', accessToken: '' },
	slack: { token: '' },
}

const parseJsonObject = (value: string) => {
	if (!value || !value.trim()) return null
	try {
		const parsed = JSON.parse(value)
		if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
			return parsed as Record<string, unknown>
		}
	} catch { }
	return null
}

const normalizeHeadersText = (value: string) => {
	const parsed = parseJsonObject(value)
	if (parsed) {
		return Object.entries(parsed)
			.map(([key, val]) => `${key}: ${String(val)}`)
			.join('\n')
	}
	return value || ''
}

const parseHeadersToMap = (value: string) => {
	const parsed = parseJsonObject(value)
	if (parsed) {
		return Object.fromEntries(Object.entries(parsed).map(([k, v]) => [k, String(v)]))
	}
	const headers: Record<string, string> = {}
	value
		.split('\n')
		.map((line) => line.trim())
		.filter(Boolean)
		.forEach((line) => {
			const index = line.indexOf(':')
			if (index === -1) return
			const key = line.slice(0, index).trim()
			const val = line.slice(index + 1).trim()
			if (key) headers[key] = val
		})
	return headers
}

const parseCredentialsState = (value: string): CredentialState => {
	const parsed = parseJsonObject(value) || {}
	return {
		wechat: {
			appid: String(parsed.appid || ''),
			secret: String(parsed.secret || ''),
			accessToken: String(parsed.accessToken || ''),
		},
		enterprise_wechat: {
			corpid: String(parsed.corpid || ''),
			corpsecret: String(parsed.corpsecret || ''),
			accessToken: String(parsed.accessToken || ''),
		},
		dingtalk: {
			appid: String(parsed.appid || ''),
			secret: String(parsed.secret || ''),
			accessToken: String(parsed.accessToken || ''),
		},
		dgwork: {
			appkey: String(parsed.appkey || ''),
			secret: String(parsed.secret || ''),
			accessToken: String(parsed.accessToken || ''),
		},
		feishu: {
			appId: String(parsed.appId || ''),
			appSecret: String(parsed.appSecret || ''),
			accessToken: String(parsed.accessToken || ''),
		},
		slack: {
			token: String(parsed.token || ''),
		},
	}
}

const buildCredentialsPayload = (type: string, creds: CredentialState) => {
	const payload: Record<string, string> = {}
	if (type === 'wechat') {
		payload.appid = creds.wechat.appid
		payload.secret = creds.wechat.secret
		payload.accessToken = creds.wechat.accessToken
	} else if (type === 'enterprise_wechat') {
		payload.corpid = creds.enterprise_wechat.corpid
		payload.corpsecret = creds.enterprise_wechat.corpsecret
		payload.accessToken = creds.enterprise_wechat.accessToken
	} else if (type === 'dingtalk') {
		payload.appid = creds.dingtalk.appid
		payload.secret = creds.dingtalk.secret
		payload.accessToken = creds.dingtalk.accessToken
	} else if (type === 'dgwork') {
		payload.appkey = creds.dgwork.appkey
		payload.secret = creds.dgwork.secret
		payload.accessToken = creds.dgwork.accessToken
	} else if (type === 'feishu') {
		payload.appId = creds.feishu.appId
		payload.appSecret = creds.feishu.appSecret
		payload.accessToken = creds.feishu.accessToken
	} else if (type === 'slack') {
		payload.token = creds.slack.token
	}
	return JSON.stringify(payload, null, 2)
}

const getCredentialPair = (type: string, creds: CredentialState) => {
	if (type === 'wechat') return { key: creds.wechat.appid, secret: creds.wechat.secret }
	if (type === 'enterprise_wechat') return { key: creds.enterprise_wechat.corpid, secret: creds.enterprise_wechat.corpsecret }
	if (type === 'dingtalk') return { key: creds.dingtalk.appid, secret: creds.dingtalk.secret }
	if (type === 'dgwork') return { key: creds.dgwork.appkey, secret: creds.dgwork.secret }
	if (type === 'feishu') return { key: creds.feishu.appId, secret: creds.feishu.appSecret }
	return { key: '', secret: '' }
}

const applyCredentialPairIfEmpty = (type: string, creds: CredentialState, pair: { key: string; secret: string }) => {
	if (!pair.key && !pair.secret) return creds
	if (type === 'wechat') {
		return { ...creds, wechat: { ...creds.wechat, appid: creds.wechat.appid || pair.key, secret: creds.wechat.secret || pair.secret } }
	}
	if (type === 'enterprise_wechat') {
		return { ...creds, enterprise_wechat: { ...creds.enterprise_wechat, corpid: creds.enterprise_wechat.corpid || pair.key, corpsecret: creds.enterprise_wechat.corpsecret || pair.secret } }
	}
	if (type === 'dingtalk') {
		return { ...creds, dingtalk: { ...creds.dingtalk, appid: creds.dingtalk.appid || pair.key, secret: creds.dingtalk.secret || pair.secret } }
	}
	if (type === 'dgwork') {
		return { ...creds, dgwork: { ...creds.dgwork, appkey: creds.dgwork.appkey || pair.key, secret: creds.dgwork.secret || pair.secret } }
	}
	if (type === 'feishu') {
		return { ...creds, feishu: { ...creds.feishu, appId: creds.feishu.appId || pair.key, appSecret: creds.feishu.appSecret || pair.secret } }
	}
	return creds
}

export default function KeyManagerPageClient() {
	const router = useRouter()
	const searchParams = useSearchParams()
	const { toast } = useToast()
	const { t, language } = useI18n()
	const i18n = t.tools.keyManager
	const entryId = searchParams.get('id')
	const dateLocale = language === 'en' ? 'en-US' : 'zh-CN'
	const typeLabels: Record<string, string> = i18n.types

	const [entries, setEntries] = useState<KeyEntry[]>([])
	const [searchQuery, setSearchQuery] = useState('')
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editing, setEditing] = useState<KeyEntry | null>(null)
	const [isSavingEntry, setIsSavingEntry] = useState(false)
	const [dialogCreds, setDialogCreds] = useState<CredentialState>(emptyCredentials)
	const tableHostRef = useRef<HTMLDivElement | null>(null)
	const [showRightFixedShadow, setShowRightFixedShadow] = useState(false)
	const [formData, setFormData] = useState({
		name: '',
		type: 'wechat',
		baseUrl: '',
		credentials: '',
		headers: '',
		notes: '',
	})

	const fetchEntryList = useCallback(async () => {
		const res = await fetchWithAuth(`/api/v1/tools/key-manager/entries`)
		const data = await res.json()
		if (!data.success) {
			throw new Error(data.error || i18n.toasts.unknownError)
		}
		return data.data || []
	}, [i18n.toasts.unknownError])

	const fetchEntries = useCallback(async () => {
		setEntries(await fetchEntryList())
	}, [fetchEntryList])

	useEffect(() => {
		if (!entryId) {
			let cancelled = false

			const loadEntries = async () => {
				try {
					const nextEntries = await fetchEntryList()
					if (!cancelled) {
						setEntries(nextEntries)
					}
				} catch (error) {
					// Keep the list empty when the overview cannot load entries.
				}
			}

			void loadEntries()

			return () => {
				cancelled = true
			}
		}
	}, [entryId, fetchEntryList])

	const filteredEntries = useMemo(() => {
		if (!searchQuery.trim()) return entries
		const query = searchQuery.toLowerCase()
		return entries.filter((entry) =>
			entry.name.toLowerCase().includes(query) ||
			entry.type.toLowerCase().includes(query) ||
			entry.notes?.toLowerCase().includes(query)
		)
	}, [entries, searchQuery])

	const updateRightFixedShadow = useCallback(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) {
			setShowRightFixedShadow(false)
			return
		}

		const maxScrollLeft = el.scrollWidth - el.clientWidth
		setShowRightFixedShadow(maxScrollLeft > 1 && el.scrollLeft < maxScrollLeft - 1)
	}, [])

	useEffect(() => {
		const host = tableHostRef.current
		const el = host?.querySelector('[data-slot="table-container"]') as HTMLDivElement | null
		if (!el) return

		updateRightFixedShadow()
		el.addEventListener('scroll', updateRightFixedShadow, { passive: true })
		window.addEventListener('resize', updateRightFixedShadow)

		return () => {
			el.removeEventListener('scroll', updateRightFixedShadow)
			window.removeEventListener('resize', updateRightFixedShadow)
		}
	}, [filteredEntries.length, updateRightFixedShadow])

	const openDialog = (entry?: KeyEntry) => {
		if (entry) {
			const creds = parseCredentialsState(entry.credentials || '')
			setEditing(entry)
			setDialogCreds(creds)
			setFormData({
				name: entry.name,
				type: entry.type,
				baseUrl: entry.baseUrl || '',
				credentials: entry.credentials || '',
				headers: normalizeHeadersText(entry.headers || ''),
				notes: entry.notes || '',
			})
		} else {
			setEditing(null)
			setDialogCreds(emptyCredentials)
			setFormData({
				name: '',
				type: 'wechat',
				baseUrl: '',
				credentials: '',
				headers: '',
				notes: '',
			})
		}
		setDialogOpen(true)
	}

	const handleTypeChange = (value: string) => {
		setDialogCreds((prev) => applyCredentialPairIfEmpty(value, prev, getCredentialPair(formData.type, prev)))
		setFormData((prev) => ({ ...prev, type: value }))
	}

	const saveEntry = async () => {
		setIsSavingEntry(true)
		try {
		const payload = {
			...formData,
			credentials: buildCredentialsPayload(formData.type, dialogCreds),
		}
		const url = editing
			? `/api/v1/tools/key-manager/entries/${editing.id}`
			: `/api/v1/tools/key-manager/entries`
		const method = editing ? 'PUT' : 'POST'

		const res = await fetchWithAuth(url, {
			method,
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(payload),
		})
		const data = await res.json()
		if (!data.success) {
			toast({ title: i18n.toasts.saveFailed, description: data.error || i18n.toasts.unknownError, variant: 'destructive' })
			return
		}
		toast({ title: i18n.toasts.saveSuccess })
		setDialogOpen(false)
		setEditing(null)
		fetchEntries()
		} catch (error: any) {
			toast({ title: i18n.toasts.saveFailed, description: error.message || i18n.toasts.unknownError, variant: 'destructive' })
		} finally {
			setIsSavingEntry(false)
		}
	}

	const deleteEntry = async (id: string) => {
		const res = await fetchWithAuth(`/api/v1/tools/key-manager/entries/${id}`, { method: 'DELETE' })
		const data = await res.json()
		if (!data.success) {
			toast({ title: i18n.toasts.deleteFailed, description: data.error || i18n.toasts.unknownError, variant: 'destructive' })
			return
		}
		toast({ title: i18n.toasts.deleteSuccess })
		fetchEntries()
	}

	if (entryId) {
		return <KeyManagerDetail entryId={entryId} onBack={() => router.push('/tools/key-manager')} />
	}

	return (
		<div className="flex-1 overflow-y-auto scan-panel-enter">
			<div className="flex flex-wrap items-center justify-between gap-3 px-3 md:px-6 py-4">
				<div className="min-w-0 flex-1 md:max-w-md">
					<div className="relative">
						<Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
						<Input
							placeholder={i18n.list.searchPlaceholder}
							value={searchQuery}
							onChange={(e) => setSearchQuery(e.target.value)}
							className="pl-9"
						/>
					</div>
				</div>
				<Button onClick={() => openDialog()}>
					<Plus className="w-4 h-4 mr-2" />
					{i18n.list.addKey}
				</Button>
			</div>

			<div className="px-3 md:px-6 pb-6">
				{filteredEntries.length === 0 ? (
					<Card className="rounded-lg p-12 text-center">
						<KeyRound className="mx-auto mb-4 h-12 w-12 text-muted-foreground opacity-50" />
						<p className="text-muted-foreground">{i18n.list.empty}</p>
					</Card>
				) : (
					<div ref={tableHostRef} className="rounded-lg border overflow-hidden">
						<Table className={cn("min-w-[1120px]", mobileTable.table)}>
							<TableHeader>
								<TableRow>
									<TableHead>{i18n.columns.name}</TableHead>
									<TableHead className="w-[160px]">{i18n.columns.type}</TableHead>
									<TableHead>Key</TableHead>
									<TableHead>Secret</TableHead>
									<TableHead className="w-[200px]">{i18n.columns.notes}</TableHead>
									<TableHead className="w-[160px]">{i18n.columns.updatedAt}</TableHead>
									<TableHead
										className={cn(
											'w-[160px] text-center sticky right-0 z-20 border-l border-border bg-background',
											showRightFixedShadow && 'table-fixed-column-shadow-left',
										)}
									>
										{i18n.columns.actions}
									</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{filteredEntries.map((entry) => (
									<TableRow key={entry.id} className="group hover:bg-muted">
										<TableCell data-label={i18n.columns.name} className="font-medium">{entry.name}</TableCell>
										<TableCell data-label={i18n.columns.type}>
											<Badge variant="outline">
												<div className="flex items-center gap-2">
													{entry.type === 'wechat' && <WechatIcon className="h-4 w-4 text-emerald-500" />}
													{entry.type === 'enterprise_wechat' && <WeComIcon className="h-4 w-4 text-sky-500" />}
													{entry.type === 'dingtalk' && <DingtalkIcon className="h-4 w-4 text-blue-500" />}
													{entry.type === 'dgwork' && <DgWorkIcon className="h-4 w-4 text-indigo-500" />}
													{entry.type === 'feishu' && <Bird className="h-4 w-4 text-sky-500" />}
													{entry.type === 'slack' && <Slack className="h-4 w-4 text-fuchsia-500" />}
													{typeLabels[entry.type] || entry.type}
												</div>
											</Badge>
										</TableCell>
										{(() => {
											const creds = parseCredentialsState(entry.credentials || '')
											let key = '-'
											let secret = '-'
											if (entry.type === 'wechat') {
												key = creds.wechat.appid || '-'
												secret = creds.wechat.secret || '-'
											} else if (entry.type === 'enterprise_wechat') {
												key = creds.enterprise_wechat.corpid || '-'
												secret = creds.enterprise_wechat.corpsecret || '-'
											} else if (entry.type === 'dingtalk') {
												key = creds.dingtalk.appid || '-'
												secret = creds.dingtalk.secret || '-'
											} else if (entry.type === 'dgwork') {
												key = creds.dgwork.appkey || '-'
												secret = creds.dgwork.secret || '-'
											} else if (entry.type === 'feishu') {
												key = creds.feishu.appId || '-'
												secret = creds.feishu.appSecret || '-'
											} else if (entry.type === 'slack') {
												key = 'OAuth / App Token'
												secret = creds.slack.token || '-'
											}
											return (
												<>
													<TableCell data-label="Key" className="max-w-[240px] font-mono text-xs text-muted-foreground truncate" title={key}>
														{key}
													</TableCell>
													<TableCell data-label="Secret" className="max-w-[240px] font-mono text-xs text-muted-foreground truncate">
														{maskSecret(secret)}
													</TableCell>
												</>
											)
										})()}
										<TableCell data-label={i18n.columns.notes} className="text-muted-foreground">
											{entry.notes || '-'}
										</TableCell>
										<TableCell data-label={i18n.columns.updatedAt} className="text-muted-foreground">
											{entry.updatedAt ? new Date(entry.updatedAt).toLocaleString(dateLocale) : '-'}
										</TableCell>
										<TableCell
											className={cn(
												'text-center sticky right-0 z-10 border-l border-border bg-background group-hover:bg-muted',
												showRightFixedShadow && 'table-fixed-column-shadow-left',
											)}
										>
											<div className="flex items-center justify-center gap-2">
												<Button size="icon" variant="ghost" onClick={() => router.push(`/tools/key-manager?id=${entry.id}`)} aria-label={i18n.actions.connectAndEnter}>
													<ArrowRight className="h-4 w-4" />
												</Button>
												<Button size="icon" variant="ghost" onClick={() => openDialog(entry)} aria-label={i18n.actions.edit}>
													<Pencil className="h-4 w-4" />
												</Button>
												<Button size="icon" variant="ghost" onClick={() => deleteEntry(entry.id)} aria-label={i18n.actions.delete}>
													<Trash2 className="h-4 w-4" />
												</Button>
											</div>
										</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					</div>
				)}
			</div>

			<Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
				<DialogContent className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-xl">
					<DialogHeader>
						<DialogTitle>{editing ? i18n.dialog.editTitle : i18n.dialog.addTitle}</DialogTitle>
					</DialogHeader>
					<div className="grid gap-4">
						<div className="space-y-2">
							<Label>{i18n.dialog.name}</Label>
							<Input
								value={formData.name}
								onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
							/>
						</div>
						<div className="space-y-2">
							<Label>{i18n.dialog.type}</Label>
							<Select
								value={formData.type}
								onValueChange={handleTypeChange}
							>
								<SelectTrigger className="w-[220px]">
									<SelectValue placeholder={i18n.dialog.typePlaceholder} />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="wechat">
										<span className="flex items-center gap-2">
											<WechatIcon className="h-4 w-4 text-emerald-500" />
											{i18n.types.wechat}
										</span>
									</SelectItem>
									<SelectItem value="enterprise_wechat">
										<span className="flex items-center gap-2">
											<WeComIcon className="h-4 w-4 text-sky-500" />
											{i18n.types.enterprise_wechat}
										</span>
									</SelectItem>
									<SelectItem value="dingtalk">
										<span className="flex items-center gap-2">
											<DingtalkIcon className="h-4 w-4 text-blue-500" />
											{i18n.types.dingtalk}
										</span>
									</SelectItem>
									<SelectItem value="dgwork">
										<span className="flex items-center gap-2">
											<DgWorkIcon className="h-4 w-4 text-indigo-500" />
											{i18n.types.dgwork}
										</span>
									</SelectItem>
									<SelectItem value="feishu">
										<span className="flex items-center gap-2">
											<Bird className="h-4 w-4 text-sky-500" />
											{i18n.types.feishu}
										</span>
									</SelectItem>
									<SelectItem value="slack">
										<span className="flex items-center gap-2">
											<Slack className="h-4 w-4 text-fuchsia-500" />
											{i18n.types.slack}
										</span>
									</SelectItem>
								</SelectContent>
							</Select>
						</div>
						{formData.type === 'dgwork' && (
							<div className="space-y-2">
								<Label>BaseURL</Label>
								<Input
									value={formData.baseUrl}
									onChange={(e) => setFormData((prev) => ({ ...prev, baseUrl: e.target.value }))}
								/>
							</div>
						)}
						<div className="space-y-2">
							<Label>{i18n.dialog.credentials}</Label>
							{formData.type === 'wechat' && (
								<div className="grid gap-3">
									<Input placeholder="Appid" value={dialogCreds.wechat.appid} onChange={(e) => setDialogCreds((prev) => ({ ...prev, wechat: { ...prev.wechat, appid: e.target.value } }))} />
									<Input placeholder="Secret" value={dialogCreds.wechat.secret} onChange={(e) => setDialogCreds((prev) => ({ ...prev, wechat: { ...prev.wechat, secret: e.target.value } }))} />
								</div>
							)}
							{formData.type === 'enterprise_wechat' && (
								<div className="grid gap-3">
									<Input placeholder="Corpid" value={dialogCreds.enterprise_wechat.corpid} onChange={(e) => setDialogCreds((prev) => ({ ...prev, enterprise_wechat: { ...prev.enterprise_wechat, corpid: e.target.value } }))} />
									<Input placeholder="CorpSecret" value={dialogCreds.enterprise_wechat.corpsecret} onChange={(e) => setDialogCreds((prev) => ({ ...prev, enterprise_wechat: { ...prev.enterprise_wechat, corpsecret: e.target.value } }))} />
								</div>
							)}
							{formData.type === 'dingtalk' && (
								<div className="grid gap-3">
									<Input placeholder="Appid" value={dialogCreds.dingtalk.appid} onChange={(e) => setDialogCreds((prev) => ({ ...prev, dingtalk: { ...prev.dingtalk, appid: e.target.value } }))} />
									<Input placeholder="Secret" value={dialogCreds.dingtalk.secret} onChange={(e) => setDialogCreds((prev) => ({ ...prev, dingtalk: { ...prev.dingtalk, secret: e.target.value } }))} />
								</div>
							)}
							{formData.type === 'dgwork' && (
								<div className="grid gap-3">
									<Input placeholder="AppKey" value={dialogCreds.dgwork.appkey} onChange={(e) => setDialogCreds((prev) => ({ ...prev, dgwork: { ...prev.dgwork, appkey: e.target.value } }))} />
									<Input placeholder="Secret" value={dialogCreds.dgwork.secret} onChange={(e) => setDialogCreds((prev) => ({ ...prev, dgwork: { ...prev.dgwork, secret: e.target.value } }))} />
								</div>
							)}
							{formData.type === 'feishu' && (
								<div className="grid gap-3">
									<Input placeholder="App ID (cli_...)" value={dialogCreds.feishu.appId} onChange={(e) => setDialogCreds((prev) => ({ ...prev, feishu: { ...prev.feishu, appId: e.target.value } }))} />
									<Input placeholder="App Secret" type="password" value={dialogCreds.feishu.appSecret} onChange={(e) => setDialogCreds((prev) => ({ ...prev, feishu: { ...prev.feishu, appSecret: e.target.value } }))} />
								</div>
							)}
							{formData.type === 'slack' && (
								<Input placeholder="Slack Bot / User / App Token" type="password" value={dialogCreds.slack.token} onChange={(e) => setDialogCreds((prev) => ({ ...prev, slack: { ...prev.slack, token: e.target.value } }))} />
							)}
						</div>
						<div className="space-y-2">
							<Label>Headers</Label>
							<Textarea
								value={formData.headers}
								onChange={(e) => setFormData((prev) => ({ ...prev, headers: e.target.value }))}
								placeholder="X-Forwarded-For: 127.0.0.1"
								className="min-h-[100px] font-mono text-xs"
							/>
						</div>
						<div className="space-y-2">
							<Label>{i18n.dialog.notes}</Label>
							<Textarea
								value={formData.notes}
								onChange={(e) => setFormData((prev) => ({ ...prev, notes: e.target.value }))}
							/>
						</div>
					</div>
					<DialogFooter>
						<Button variant="outline" onClick={() => setDialogOpen(false)} disabled={isSavingEntry}>
							{t.common.cancel}
						</Button>
						<Button onClick={saveEntry} disabled={isSavingEntry}>
							{isSavingEntry ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Save className="w-4 h-4 mr-2" />}
							<span aria-live="polite">{t.common.save}</span>
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	)
}

function KeyManagerDetail({ entryId, onBack }: { entryId: string; onBack: () => void }) {
  const { toast } = useToast()
  const { t } = useI18n()
  const i18n = t.tools.keyManager
  const typeLabels: Record<string, string> = i18n.types
  const [entry, setEntry] = useState<KeyEntry | null>(null)
  const [accessToken, setAccessToken] = useState('')

  // 请求编辑器状态
  const [rawRequest, setRawRequest] = useState('')
  const [response, setResponse] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [forceHttps, setForceHttps] = useState(true)

  // 加载 entry 数据
  useEffect(() => {
    const fetchEntry = async () => {
      try {
        const res = await fetchWithAuth(`/api/v1/tools/key-manager/entries/${entryId}`)
        const data = await res.json()
        if (!data.success) {
          toast({ title: i18n.toasts.loadFailed, description: data.error || i18n.toasts.unknownError, variant: 'destructive' })
          return
        }
        const entryData = data.data as KeyEntry
        setEntry(entryData)
        // 从 credentials 中恢复 accessToken
        const creds = parseCredentialsState(entryData.credentials || '')
        if (entryData.type === 'wechat') setAccessToken(creds.wechat.accessToken || '')
        else if (entryData.type === 'enterprise_wechat') setAccessToken(creds.enterprise_wechat.accessToken || '')
        else if (entryData.type === 'dingtalk') setAccessToken(creds.dingtalk.accessToken || '')
        else if (entryData.type === 'dgwork') setAccessToken(creds.dgwork.accessToken || '')
        else if (entryData.type === 'feishu') setAccessToken(creds.feishu.accessToken || '')
        else if (entryData.type === 'slack') setAccessToken(creds.slack.token || '')
      } catch (error: any) {
        toast({ title: i18n.toasts.loadFailed, description: error.message, variant: 'destructive' })
      }
    }
    fetchEntry()
  }, [entryId, i18n.toasts.loadFailed, i18n.toasts.unknownError, toast])

  // 构建请求内容
  const buildRequest = (action: ApiAction, token: string, customHeaders?: Record<string, string>) => {
    const url = new URL(action.url + (action.tokenInQuery === false ? '' : token))
    const host = url.host
    const path = url.pathname + url.search

    let headers = `Host: ${host}\nAccept: application/json`
    if (action.auth === 'bearer') {
      headers += `\nAuthorization: Bearer ${token}`
    }
    if (customHeaders) {
      Object.entries(customHeaders).forEach(([key, value]) => {
        headers += `\n${key}: ${value}`
      })
    }

    let raw = `${action.method} ${path} HTTP/1.1\n${headers}`

    if (action.body) {
      raw += `\nContent-Type: application/json\nContent-Length: ${action.body.length}\n\n${action.body}`
    } else {
      raw += '\n\n'
    }

    return raw
  }

  // 填充获取 Token 的请求并自动发送
  const fillGetTokenRequest = async () => {
    if (!entry || isLoading) return
    const creds = parseCredentialsState(entry.credentials || '')
    const customHeaders = parseHeadersToMap(normalizeHeadersText(entry.headers || ''))

    // 专有钉钉需要通过后端获取 token
    if (entry.type === 'dgwork') {
      setIsLoading(true)
      setRawRequest(i18n.detail.dgworkTokenRequestComment)
      setResponse('')

      try {
        const res = await fetchWithAuth(`/api/v1/tools/key-manager/dgwork/token`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            baseURL: entry.baseUrl,
            appKey: creds.dgwork.appkey || '',
            appSecret: creds.dgwork.secret || '',
            headers: customHeaders,
            proxy: '',
          }),
        })
        const data = await res.json()

        if (!data.success) {
          setResponse(data.error || i18n.detail.getTokenFailed)
          return
        }

        setResponse(data.body || '')

        // 尝试提取 token
        try {
          const parsed = JSON.parse(data.body || '{}')
          const tokenValue = parsed?.content?.data?.accessToken
          if (tokenValue) {
            setAccessToken(tokenValue)
            // 保存到后端
            const nextCreds = { ...creds }
            nextCreds.dgwork.accessToken = tokenValue
            await fetchWithAuth(`/api/v1/tools/key-manager/entries/${entry.id}`, {
              method: 'PUT',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                ...entry,
                credentials: buildCredentialsPayload(entry.type, nextCreds),
              }),
            })
            toast({ title: i18n.toasts.tokenUpdated })
          }
        } catch {
          // 忽略解析错误
        }
      } catch (error: any) {
        setResponse(`${i18n.detail.getTokenFailed}: ${error.message}`)
      } finally {
        setIsLoading(false)
      }
      return
    }

    if (entry.type === 'feishu') {
      const body = JSON.stringify({ app_id: creds.feishu.appId, app_secret: creds.feishu.appSecret })
      const raw = `POST /open-apis/auth/v3/tenant_access_token/internal HTTP/1.1\nHost: open.feishu.cn\nAccept: application/json\nContent-Type: application/json\nContent-Length: ${body.length}\n\n${body}`
      setRawRequest(raw)
      setResponse('')
      await sendRequest(raw)
      return
    }

    if (entry.type === 'slack') {
      const token = creds.slack.token
      if (!token) {
        toast({ title: i18n.toasts.getTokenFirst, variant: 'destructive' })
        return
      }
      setAccessToken(token)
      const raw = buildRequest(slackApiOptions[0], token, customHeaders)
      setRawRequest(raw)
      setResponse('')
      await sendRequest(raw)
      return
    }

    // 其他类型通过 HTTP 请求获取
    let url = ''
    if (entry.type === 'wechat') {
      url = `https://api.weixin.qq.com/cgi-bin/token?grant_type=client_credential&appid=${creds.wechat.appid}&secret=${creds.wechat.secret}`
    } else if (entry.type === 'enterprise_wechat') {
      url = `https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=${creds.enterprise_wechat.corpid}&corpsecret=${creds.enterprise_wechat.corpsecret}`
    } else if (entry.type === 'dingtalk') {
      url = `https://oapi.dingtalk.com/gettoken?appkey=${creds.dingtalk.appid}&appsecret=${creds.dingtalk.secret}`
    }

    if (!url) return

    const urlObj = new URL(url)
    let headers = `Host: ${urlObj.host}\nAccept: application/json`
    Object.entries(customHeaders).forEach(([key, value]) => {
      headers += `\n${key}: ${value}`
    })

    const raw = `GET ${urlObj.pathname}${urlObj.search} HTTP/1.1\n${headers}\n\n`
    setRawRequest(raw)
    setResponse('')

    // 自动发送请求
    await sendRequest(raw)
  }

  // 填充 API 操作请求并自动发送
  const fillApiRequest = async (action: ApiAction) => {
    if (!entry || isLoading) return
    const token = accessToken
    if (!token) {
      toast({ title: i18n.toasts.getTokenFirst, variant: 'destructive' })
      return
    }

    const customHeaders = parseHeadersToMap(normalizeHeadersText(entry.headers || ''))
    const raw = buildRequest(action, token, customHeaders)
    setRawRequest(raw)
    setResponse('')

    // 自动发送请求
    await sendRequest(raw)
  }

  // 发送请求（核心逻辑）
  const sendRequest = async (raw: string) => {
    if (!raw.trim()) return

    setIsLoading(true)
    setResponse('')

    try {
      const res = await fetchWithAuth(`/api/v1/repeater/send`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          raw,
          forceHttps,
          redirect: false,
          proxyURL: '',
        }),
      })
      const data = await res.json()

      if (!data.success) {
        setResponse(data.error || i18n.detail.requestFailed)
        return
      }

      setResponse(data.response || '')

      // 尝试从响应中提取 access_token
      try {
        const bodyMatch = data.response?.match(/\r?\n\r?\n([\s\S]*)/)
        if (bodyMatch) {
          const bodyStr = bodyMatch[1]
          const parsed = JSON.parse(bodyStr)
          const tokenValue = parsed.access_token || parsed.tenant_access_token || parsed?.content?.data?.accessToken
          if (tokenValue) {
            setAccessToken(tokenValue)
            // 保存到后端
            if (entry) {
              const creds = parseCredentialsState(entry.credentials || '')
              const nextCreds = { ...creds }
              if (entry.type === 'wechat') nextCreds.wechat.accessToken = tokenValue
              else if (entry.type === 'enterprise_wechat') nextCreds.enterprise_wechat.accessToken = tokenValue
              else if (entry.type === 'dingtalk') nextCreds.dingtalk.accessToken = tokenValue
              else if (entry.type === 'dgwork') nextCreds.dgwork.accessToken = tokenValue
              else if (entry.type === 'feishu') nextCreds.feishu.accessToken = tokenValue

              await fetchWithAuth(`/api/v1/tools/key-manager/entries/${entry.id}`, {
                method: 'PUT',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                  ...entry,
                  credentials: buildCredentialsPayload(entry.type, nextCreds),
                }),
              })
            }
            toast({ title: i18n.toasts.tokenUpdated })
          }
        }
      } catch {
        // 忽略解析错误
      }
    } catch (error: any) {
      setResponse(`${i18n.detail.requestFailed}: ${error.message}`)
    } finally {
      setIsLoading(false)
    }
  }

  // 手动发送按钮
  const handleSendRequest = async () => {
    if (!rawRequest.trim()) {
      toast({ title: i18n.toasts.selectOperationOrInputRequest, variant: 'destructive' })
      return
    }
    await sendRequest(rawRequest)
  }

  // 可用操作列表
  const actions = useMemo(() => {
    if (!entry) return []
    if (entry.type === 'wechat') return wechatApiOptions
    if (entry.type === 'enterprise_wechat') return enterpriseWechatApiOptions
    if (entry.type === 'dingtalk') return dingtalkApiOptions
    if (entry.type === 'feishu') return feishuApiOptions
    if (entry.type === 'slack') return slackApiOptions
    return []
  }, [entry])

  if (!entry) {
    return (
      <div className="flex items-center justify-center h-full p-10 text-muted-foreground">{t.common.loading}</div>
    )
  }

  return (
    <div className="flex flex-col h-full overflow-hidden scan-panel-enter">
      {/* 顶部信息栏 */}
      <div className="shrink-0 border-b bg-card p-4">
        <div className="flex flex-wrap items-center justify-between gap-3 mb-3">
          <div className="flex items-center gap-3">
            <Button variant="ghost" size="icon" onClick={onBack}>
              <ChevronLeft className="h-5 w-5" />
            </Button>
            {entry.type === 'wechat' && <WechatIcon className="h-5 w-5 text-emerald-500" />}
            {entry.type === 'enterprise_wechat' && <WeComIcon className="h-5 w-5 text-sky-500" />}
            {entry.type === 'dingtalk' && <DingtalkIcon className="h-5 w-5 text-blue-500" />}
            {entry.type === 'dgwork' && <DgWorkIcon className="h-5 w-5 text-indigo-500" />}
            {entry.type === 'feishu' && <Bird className="h-5 w-5 text-sky-500" />}
            {entry.type === 'slack' && <Slack className="h-5 w-5 text-fuchsia-500" />}
            <div>
              <div className="font-semibold">{entry.name}</div>
              <div className="text-xs text-muted-foreground">{typeLabels[entry.type] || entry.type}</div>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Label className="text-xs text-muted-foreground">HTTPS</Label>
            <Switch checked={forceHttps} onCheckedChange={setForceHttps} />
            <Button onClick={handleSendRequest} disabled={isLoading} className="ml-2">
              {isLoading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Send className="h-4 w-4 mr-2" />}
              <span aria-live="polite">{i18n.actions.send}</span>
            </Button>
          </div>
        </div>

        {/* Token 和操作按钮 */}
        <div className="flex items-center gap-3">
          <div className="flex-1">
            <Input
              value={accessToken}
              onChange={(e) => setAccessToken(e.target.value)}
              placeholder={i18n.detail.accessTokenPlaceholder}
              className="font-mono text-xs h-8"
            />
          </div>
          <Button variant="outline" size="sm" onClick={fillGetTokenRequest}>
            {i18n.actions.getToken}
          </Button>
        </div>

        {/* 快捷操作 */}
        {actions.length > 0 && (
          <div className="flex flex-wrap gap-2 mt-3">
            <span className="text-xs text-muted-foreground self-center">{i18n.detail.quickActions}</span>
            {actions.map((action) => (
              <Button
                key={action.name}
                variant="outline"
                size="sm"
                className="h-7 text-xs"
                onClick={() => fillApiRequest(action)}
              >
                {i18n.apiActions[action.name as keyof typeof i18n.apiActions] || action.name}
              </Button>
            ))}
          </div>
        )}
      </div>

      {/* 请求/响应编辑器 */}
      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto md:flex-row md:overflow-hidden">
        {/* 请求面板 */}
        <div className="w-full min-h-[320px] shrink-0 md:min-h-0 md:shrink md:w-1/2 flex flex-col border-r">
          <div className="h-10 flex items-center px-4 border-b bg-muted/30">
            <span className="text-sm font-medium">Request</span>
          </div>
          <div className="flex-1 overflow-hidden">
            <RequestEditor value={rawRequest} onChange={setRawRequest} />
          </div>
        </div>

        {/* 响应面板 */}
        <div className="w-full min-h-[320px] shrink-0 md:min-h-0 md:shrink md:w-1/2 flex flex-col">
          <div className="h-10 flex items-center px-4 border-b bg-muted/30">
            <span className="text-sm font-medium">Response</span>
          </div>
          <div className="flex-1 overflow-hidden">
            <ResponseViewer value={response} isLoading={isLoading} />
          </div>
        </div>
      </div>
    </div>
  )
}
