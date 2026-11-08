'use client'

import mobile from './agents-mobile.module.css'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Clock, Cpu, HardDrive, Info, MemoryStick, Pencil, RefreshCw, Search, Server, Settings, Trash2 } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import toast from '@/lib/toast'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
	DialogTrigger,
} from '@/components/ui/dialog'
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Progress } from '@/components/ui/progress'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import { PythonEnvironmentDialog } from './PythonEnvironmentDialog'
import { AgentInstallSheet } from './AgentInstallSheet'
import { AgentUpdateManager, AgentUpdateStatus, useAgentUpdates, updateActive, updateEligible } from './AgentUpdateManager'

interface AgentNode {
 updateProtocol?: number
 updatePending?: boolean
 updateOS?: string
 updateArch?: string
 aiWorker?: boolean
 scanWorker?: boolean
 aiConcurrency?: number
 scanConcurrency?: number
 aiRunning?: number
 scanRunning?: number

	agentVersion?: string
	localWorker?: boolean
	agentId: string
	displayName: string
	enabled: boolean
	status: string
	isAlive: boolean
	lastHeartbeat: string
	lastSeenHost: string
	lastSeenIp: string
	cpuUsagePercent: number
	memoryUsageBytes: number
	memoryTotalBytes: number
	memoryUsagePercent: number
	diskUsageBytes: number
	diskTotalBytes: number
	diskUsagePercent: number
	createdAt: string
	updatedAt: string
}

interface AgentConfig {
	server: {
		address: string
	}
}

function formatDateTime(value: string | undefined, locale: string): string {
	if (!value) return '-'
	const d = new Date(value)
	if (Number.isNaN(d.getTime())) return value
	return d.toLocaleString(locale)
}

function formatPercent(value: number | undefined): string {
	if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return '-'
	if (value === 0) return '0%'
	return `${value.toFixed(value >= 10 ? 0 : 1)}%`
}

function clampPercent(value: number | undefined): number {
	if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return 0
	return Math.min(100, Math.max(0, value))
}

function formatBytes(value: number | undefined): string {
	if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '-'
	const units = ['B', 'KB', 'MB', 'GB', 'TB']
	let current = value
	let index = 0
	while (current >= 1024 && index < units.length - 1) {
		current /= 1024
		index += 1
	}
	return `${current.toFixed(index >= 3 ? 1 : 0)} ${units[index]}`
}

function ResourceMetric({
	icon: Icon,
	label,
	percent,
	detail,
}: {
	icon: React.ComponentType<{ className?: string }>
	label: string
	percent?: number
	detail?: string
}) {
	const hasValue = typeof percent === 'number' && Number.isFinite(percent) && percent >= 0
	return (
		<div className={cn(mobile.metric, "min-w-0 space-y-2 py-3 first:pt-0 last:pb-0 @[320px]/agent:px-3 @[320px]/agent:py-0 @[320px]/agent:first:pl-0 @[320px]/agent:last:pr-0")}>
			<div className="flex items-center gap-2 text-sm text-muted-foreground">
				<Icon className="h-4 w-4 shrink-0" />
				<span>{label}</span>
			</div>
			<div className={cn(mobile.metricValue, "text-2xl font-semibold leading-tight tabular-nums tracking-tight")}>{formatPercent(percent)}</div>
			<Progress
				value={hasValue ? clampPercent(percent) : undefined}
				aria-label={label}
				aria-valuetext={formatPercent(percent)}
				className={cn(mobile.metricBar, 'h-1.5 bg-muted [&_[data-slot=progress-indicator]]:bg-foreground/85', hasValue && percent >= 90 && '[&_[data-slot=progress-indicator]]:bg-amber-500')}
			/>
			<div className={cn(mobile.metricDetail, "min-h-4 break-words text-xs leading-relaxed tabular-nums text-muted-foreground", !detail && "hidden md:block")} title={detail}>{detail}</div>
		</div>
	)
}

export default function ScanAgentsPage() {
	const { t, language } = useI18n()
	const [agents, setAgents] = useState<AgentNode[]>([])
	const [loading, setLoading] = useState(false)
	const [updatingId, setUpdatingId] = useState<string>('')
	const [deletingId, setDeletingId] = useState<string>('')
	const [deleteTarget, setDeleteTarget] = useState<AgentNode | null>(null)
	const [editTarget, setEditTarget] = useState<AgentNode | null>(null)
	const [editingDisplayName, setEditingDisplayName] = useState('')
	const [savingDisplayName, setSavingDisplayName] = useState(false)
	const nameInputRef = useRef<HTMLInputElement>(null)
	const editingAgentRef = useRef<AgentNode | null>(null)
	const savingNameRef = useRef(false)
	const [settingsOpen, setSettingsOpen] = useState(false)
	const [agentConfig, setAgentConfig] = useState<AgentConfig>({ server: { address: '' } })
	const [agentConfigLoading, setAgentConfigLoading] = useState(false)
	const [agentSearch, setAgentSearch] = useState('')
	const updates = useAgentUpdates()
	const [updatesOpen, setUpdatesOpen] = useState(false)
	const [updateTarget, setUpdateTarget] = useState<string | null>(null)
	const locale = language === 'en' ? 'en-US' : 'zh-CN'

	const sortedAgents = useMemo(() => {
		return [...agents].sort((a, b) => {
			const ta = new Date(a.lastHeartbeat || 0).getTime()
			const tb = new Date(b.lastHeartbeat || 0).getTime()
			return tb - ta
		})
	}, [agents])

	const filteredAgents = useMemo(() => {
		const keyword = agentSearch.trim().toLowerCase()
		if (!keyword) {
			return sortedAgents
		}
		return sortedAgents.filter((agent) => {
			return [
				agent.displayName,
				agent.lastSeenHost,
				agent.lastSeenIp,
				agent.status,
			].some((value) => String(value || '').toLowerCase().includes(keyword))
		})
	}, [agentSearch, sortedAgents])

	const fetchAgents = useCallback(async () => {
		const response = await fetchWithAuth('/api/v1/webscan/agents')
		const data = await response.json().catch(() => ({}))
		if (!response.ok || !data.success) {
			throw new Error(data.error || t.vulnScan.agents.messages.loadFailed)
		}
		return Array.isArray(data.data) ? data.data : []
	}, [t.vulnScan.agents.messages.loadFailed])

	const loadAgents = useCallback(async () => {
		setLoading(true)
		try {
			setAgents(await fetchAgents())
		} catch (error: any) {
			toast.error(error.message || t.vulnScan.agents.messages.loadFailed)
		} finally {
			setLoading(false)
		}
	}, [fetchAgents, t.vulnScan.agents.messages.loadFailed])

	const fetchAgentConfig = useCallback(async () => {
		const response = await fetchWithAuth('/api/v1/system/agent-config')
		const data = await response.json().catch(() => ({}))
		if (!response.ok || !data.success) {
			throw new Error(data.error || t.vulnScan.agents.settings.loadFailed)
		}
		return {
			server: {
				address: String(data?.data?.server?.address || '').trim(),
			},
		}
	}, [t.vulnScan.agents.settings.loadFailed])

	const loadAgentConfig = useCallback(async () => {
		setAgentConfigLoading(true)
		try {
			setAgentConfig(await fetchAgentConfig())
		} catch (error: any) {
			toast.error(error.message || t.vulnScan.agents.settings.loadFailed)
		} finally {
			setAgentConfigLoading(false)
		}
	}, [fetchAgentConfig, t.vulnScan.agents.settings.loadFailed])

	useEffect(() => {
		let cancelled = false
		const loadInitialData = async () => {
			setLoading(true)
			setAgentConfigLoading(true)
			try {
				const [nextAgents, nextAgentConfig] = await Promise.all([
					fetchAgents(),
					fetchAgentConfig(),
				])
				if (!cancelled) {
					setAgents(nextAgents)
					setAgentConfig(nextAgentConfig)
				}
			} catch (error: any) {
				if (!cancelled) {
					toast.error(error.message || t.vulnScan.agents.messages.loadFailed)
				}
			} finally {
				if (!cancelled) {
					setLoading(false)
					setAgentConfigLoading(false)
				}
			}
		}

		void loadInitialData()
		const timer = setInterval(loadAgents, 5000)
		return () => {
			cancelled = true
			clearInterval(timer)
		}
	}, [fetchAgentConfig, fetchAgents, loadAgents, t.vulnScan.agents.messages.loadFailed])

	const setEnabled = async (agentId: string, enabled: boolean) => {
		setUpdatingId(agentId)
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/agents/${encodeURIComponent(agentId)}/enabled`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ enabled }),
			})
			const data = await response.json()
			if (!response.ok || !data.success) {
				throw new Error(data.error || t.vulnScan.agents.messages.updateFailed)
			}
			setAgents((prev) =>
				prev.map((item) =>
					item.agentId === agentId
						? { ...item, enabled }
						: item,
				),
			)
			toast.success(enabled ? t.vulnScan.agents.messages.enabled : t.vulnScan.agents.messages.disabled)
		} catch (error: any) {
			toast.error(error.message || t.vulnScan.agents.messages.updateFailed)
		} finally {
			setUpdatingId('')
		}
	}

	const deleteAgent = async (agent: AgentNode) => {
		setDeletingId(agent.agentId)
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/agents/${encodeURIComponent(agent.agentId)}`, {
				method: 'DELETE',
			})
			const data = await response.json()
			if (!response.ok || !data.success) {
				throw new Error(data.error || t.vulnScan.agents.messages.deleteFailed)
			}
			setAgents((prev) => prev.filter((item) => item.agentId !== agent.agentId))
			setDeleteTarget(null)
			toast.success(t.vulnScan.agents.messages.deleted)
		} catch (error: any) {
			toast.error(error.message || t.vulnScan.agents.messages.deleteFailed)
		} finally {
			setDeletingId('')
		}
	}

	useEffect(() => {
		if (editTarget && !savingDisplayName) {
			nameInputRef.current?.focus()
			nameInputRef.current?.select()
		}
	}, [editTarget, savingDisplayName])

	const cancelNameEdit = () => {
		editingAgentRef.current = null
		setEditTarget(null)
		setEditingDisplayName('')
	}

	const startNameEdit = (agent: AgentNode) => {
		if (savingNameRef.current) return
		editingAgentRef.current = agent
		setEditTarget(agent)
		setEditingDisplayName(agent.displayName || '')
	}

	const saveDisplayName = async () => {
		const agent = editingAgentRef.current
		if (!agent || savingNameRef.current) return
		const displayName = editingDisplayName.trim()
		if (!displayName) {
			toast.error(t.vulnScan.agents.edit.required)
			return
		}
		if (displayName === agent.displayName) {
			cancelNameEdit()
			return
		}

		savingNameRef.current = true
		setSavingDisplayName(true)
		try {
			const response = await fetchWithAuth(`/api/v1/webscan/agents/${encodeURIComponent(agent.agentId)}`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ displayName }),
			})
			const data = await response.json().catch(() => ({}))
			if (!response.ok || !data.success) {
				throw new Error(data.error || t.vulnScan.agents.edit.saveFailed)
			}
			setAgents((prev) =>
				prev.map((item) =>
					item.agentId === agent.agentId
						? { ...item, displayName }
						: item,
				),
			)
			cancelNameEdit()
			toast.success(t.vulnScan.agents.edit.saveSuccess)
		} catch (error: any) {
			toast.error(error.message || t.vulnScan.agents.edit.saveFailed)
		} finally {
			savingNameRef.current = false
			setSavingDisplayName(false)
		}
	}

	const saveAgentConfig = async () => {
		const address = agentConfig.server.address.trim()
		if (!address) {
			toast.error(t.vulnScan.agents.settings.required)
			return
		}

		setAgentConfigLoading(true)
		try {
			const response = await fetchWithAuth('/api/v1/system/agent-config', {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ server: { address } }),
			})
			const data = await response.json()
			if (!response.ok || !data.success) {
				throw new Error(data.error || t.vulnScan.agents.settings.saveFailed)
			}
			setAgentConfig({ server: { address } })
			setSettingsOpen(false)
			toast.success(t.vulnScan.agents.settings.saveSuccess)
		} catch (error: any) {
			toast.error(error.message || t.vulnScan.agents.settings.saveFailed)
		} finally {
			setAgentConfigLoading(false)
		}
	}

	return (
        <div className={cn(mobile.page, "min-w-0 flex-1")} aria-busy={loading}>
			<div className="p-3 pb-[max(12px,env(safe-area-inset-bottom))] md:p-6">
				<section className="space-y-4">
					<div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
						<div className="relative w-full lg:w-[360px]">
							<Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
							<Input
								value={agentSearch}
								onChange={(e) => setAgentSearch(e.target.value)}
								placeholder="搜索显示名、主机或 IP"
								className="pl-9"
							/>
						</div>
						<div className={cn(mobile.toolbar, "grid grid-cols-3 items-center gap-2 md:flex md:flex-wrap md:justify-end")}>
							<Badge variant="outline" className="col-span-3 justify-self-start md:col-auto">{filteredAgents.length} 个节点</Badge>
							<AgentInstallSheet serverAddress={agentConfig.server.address} agents={agents} />
							<Button variant="outline" className="h-9" onClick={() => {setUpdateTarget(null);setUpdatesOpen(true)}}><RefreshCw className="h-4 w-4" />{t.vulnScan.agents.updates.title}</Button>
							<Dialog open={settingsOpen} onOpenChange={setSettingsOpen}>
								<DialogTrigger asChild>
									<Button variant="outline" className="h-9" onClick={() => void loadAgentConfig()}>
										<Settings className="mr-1 h-4 w-4" />
										{t.vulnScan.agents.settings.button}
									</Button>
								</DialogTrigger>
								<DialogContent className={cn(mobile.dialog, "sm:max-w-xl")}>
									<DialogHeader>
										<DialogTitle>{t.vulnScan.agents.settings.title}</DialogTitle>
										<DialogDescription>{t.vulnScan.agents.settings.description}</DialogDescription>
									</DialogHeader>
									<div className="space-y-4">
										<div className="space-y-2">
											<Label>{t.vulnScan.agents.settings.addressLabel}</Label>
										<Input
											value={agentConfig.server.address}
											onChange={(e) =>
												setAgentConfig({ server: { address: e.target.value } })
											}
											placeholder={t.vulnScan.agents.settings.placeholder}
											className="font-mono"
											disabled={agentConfigLoading}
										/>
									</div>
										<div className="flex justify-end gap-2">
											<Button
												variant="outline"
												onClick={() => setSettingsOpen(false)}
												disabled={agentConfigLoading}
											>
												{t.common.cancel}
											</Button>
											<Button onClick={() => void saveAgentConfig()} disabled={agentConfigLoading}>
												{agentConfigLoading ? <RefreshCw className="mr-2 h-4 w-4 animate-spin" /> : null}
												{t.common.save}
											</Button>
										</div>
									</div>
								</DialogContent>
							</Dialog>

						</div>
					</div>
					{filteredAgents.length === 0 ? (
						<Card>
							<CardContent className="py-10 text-center text-muted-foreground">
								{loading ? t.common.loading : agentSearch.trim() ? '未找到匹配 Agent 节点' : t.vulnScan.agents.messages.empty}
							</CardContent>
						</Card>
					) : (
						<div className="grid grid-cols-1 gap-4 min-[1100px]:grid-cols-2 min-[1600px]:grid-cols-3">
							{filteredAgents.map((agent) => (
								<Card key={agent.agentId} data-agent-card className="@container/agent min-w-0 gap-0 overflow-hidden py-0 shadow-none scan-panel-enter">
									<CardHeader className="block space-y-3 p-3 md:space-y-5 md:p-5">
										<div className="flex flex-col items-start justify-between gap-2 md:flex-row md:gap-4 @max-[320px]/agent:flex-col">
											<div className="w-full min-w-0 flex-1 md:w-auto @max-[320px]/agent:w-full">
												<div className="flex items-center gap-3">
													<div className="flex h-10 w-10 md:h-12 md:w-12 shrink-0 items-center justify-center rounded-lg border bg-muted/30 @max-[240px]/agent:hidden">
														<Server className="h-5 w-5 text-muted-foreground" />
													</div>
													<div className={cn('min-w-0 flex-1', editTarget?.agentId === agent.agentId && 'w-72 max-w-full')}>
														<div className="group/name flex min-h-11 max-w-full items-center gap-1 md:h-7 md:min-h-0">
															{editTarget?.agentId === agent.agentId ? (
																<>
																	<Input
																		ref={nameInputRef}
																		value={editingDisplayName}
																		onChange={(e) => setEditingDisplayName(e.target.value)}
																		onBlur={() => void saveDisplayName()}
																		onKeyDown={(e) => {
																			if (e.nativeEvent.isComposing) return
																			if (e.key === 'Enter') {
																				e.preventDefault()
																				void saveDisplayName()
																			} else if (e.key === 'Escape' && !savingNameRef.current) {
																				e.preventDefault()
																				cancelNameEdit()
																			}
																		}}
																		disabled={savingDisplayName}
																		aria-label={t.vulnScan.agents.edit.displayName}
																		className="h-7 w-72 min-w-0 max-w-full px-2 text-base font-semibold"
																	/>
																	{savingDisplayName && <RefreshCw className="h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />}
																</>
															) : (
																<>
																	<CardTitle className="min-w-0 break-all text-base md:truncate" title={agent.displayName}>{agent.displayName || '-'}</CardTitle>
																	<Button
																		variant="ghost"
																		size="icon"
																		className="h-6 w-6 shrink-0 text-muted-foreground hover:text-foreground"
																		onClick={() => startNameEdit(agent)}
																		disabled={savingDisplayName}
																		title={t.vulnScan.agents.actions.edit}
																		aria-label={t.vulnScan.agents.actions.edit}
																	>
																		<Pencil className="h-3.5 w-3.5" />
																	</Button>
																</>
															)}
														</div>
														<div className={cn('mt-1 flex items-center gap-2 text-sm', agent.isAlive ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground')}>
															<span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-current" />
															<span>{agent.isAlive ? t.vulnScan.agents.states.online : t.vulnScan.agents.states.offline}</span>
														</div>
													</div>
												</div>
											</div>
											<label className="flex min-h-11 w-full shrink-0 cursor-pointer items-center justify-between gap-2 text-sm text-muted-foreground md:min-h-0 md:w-auto md:justify-start md:pt-1 md:text-xs">
												{t.vulnScan.agents.columns.enabled}
												<Switch
													checked={agent.enabled}
													disabled={!agent.isAlive || updatingId === agent.agentId}
													aria-label={`${t.vulnScan.agents.columns.enabled} ${agent.displayName || agent.lastSeenHost}`}
													onCheckedChange={(checked) => setEnabled(agent.agentId, checked)}
												/>
											</label>
										</div>
										<dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-2 text-sm @max-[240px]/agent:grid-cols-1">
											<dt className="text-muted-foreground">{t.vulnScan.agents.card.host}</dt>
											<dd className="min-w-0 break-all md:truncate" title={agent.lastSeenHost}>{agent.lastSeenHost || '-'}</dd>
											<dt className="text-muted-foreground">{t.vulnScan.agents.card.ip}</dt>
											<dd className="min-w-0 break-all font-mono md:truncate" title={agent.lastSeenIp}>{agent.lastSeenIp || '-'}</dd>
										</dl>
									</CardHeader>
									<CardContent className="flex flex-1 flex-col px-3 pb-3 md:px-5 md:pb-4">
										<div className="flex flex-wrap items-center justify-between gap-2 border-t py-3">
											<span className="font-mono text-xs text-muted-foreground">{agent.agentVersion || '—'}</span>
											<AgentUpdateStatus node={agent} target={updates.targetVersion} job={updates.updates[agent.agentId]} />
											{!agent.localWorker && <Button variant="outline" size="sm" disabled={!updateEligible(agent,updates.targetVersion,updates.updates[agent.agentId]) && !updateActive(updates.updates[agent.agentId])}
												onClick={() => {setUpdateTarget(agent.agentId);setUpdatesOpen(true)}}>{updateActive(updates.updates[agent.agentId]) ? t.vulnScan.agents.updates.details : t.vulnScan.agents.updates.update}</Button>}
										</div>
										<div className="flex min-h-14 flex-wrap items-center gap-x-2 gap-y-2 border-t py-4 text-xs @[360px]/agent:gap-x-4">
											{agent.aiWorker && <span className="flex items-center gap-1 @[360px]/agent:gap-2"><span className="text-muted-foreground">AI Worker</span><span className="font-medium tabular-nums" title={t.vulnScan.agents.card.workerUsage}>{agent.aiRunning ?? 0}/{agent.aiConcurrency ?? 1}</span></span>}
											{agent.scanWorker !== false && <span className={cn('flex items-center gap-1 @[360px]/agent:gap-2', agent.aiWorker && 'border-l pl-2 @[360px]/agent:pl-4')}><span className="text-muted-foreground">Scan Worker</span><span className="font-medium tabular-nums" title={t.vulnScan.agents.card.workerUsage}>{agent.scanRunning ?? 0}/{agent.scanConcurrency ?? 1}</span></span>}
											<Tooltip>
												<TooltipTrigger asChild>
													<Button variant="ghost" size="icon" className="ml-auto h-6 w-6 shrink-0 text-muted-foreground hover:bg-muted hover:text-foreground" aria-label={t.vulnScan.agents.card.quotaHelp}>
														<Info className="h-4 w-4" aria-hidden="true" />
													</Button>
												</TooltipTrigger>
												<TooltipContent className="max-w-xs whitespace-normal text-left">
													{t.vulnScan.agents.card.quotaHint}
												</TooltipContent>
											</Tooltip>
										</div>
										<div className={cn("mb-2 text-xs text-muted-foreground md:min-h-4", agent.isAlive && "hidden md:block")}>{!agent.isAlive && t.vulnScan.agents.card.lastReported}</div>
										<div className={cn(mobile.metrics, "grid grid-cols-1 divide-y pb-3 md:pb-5 @[320px]/agent:grid-cols-3 @[320px]/agent:divide-x @[320px]/agent:divide-y-0")}>
											<ResourceMetric icon={Cpu} label="CPU" percent={agent.memoryTotalBytes > 0 || agent.diskTotalBytes > 0 ? agent.cpuUsagePercent : undefined} />
											<ResourceMetric icon={MemoryStick} label={t.vulnScan.agents.card.memory} percent={agent.memoryTotalBytes > 0 ? agent.memoryUsagePercent : undefined} detail={`${formatBytes(agent.memoryUsageBytes)} / ${formatBytes(agent.memoryTotalBytes)}`} />
											<ResourceMetric icon={HardDrive} label={t.vulnScan.agents.card.disk} percent={agent.diskTotalBytes > 0 ? agent.diskUsagePercent : undefined} detail={`${formatBytes(agent.diskUsageBytes)} / ${formatBytes(agent.diskTotalBytes)}`} />
										</div>
										<div className="mt-auto flex flex-wrap items-center justify-between gap-3 border-t pt-4">
											<div className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground" title={formatDateTime(agent.lastHeartbeat, locale)}>
												<Clock className="h-3.5 w-3.5 shrink-0" />
												<div className="flex flex-wrap gap-x-2 gap-y-1">
													<span>{agent.isAlive ? t.vulnScan.agents.columns.lastHeartbeat : t.vulnScan.agents.card.lastHeartbeat}</span>
													<time dateTime={agent.lastHeartbeat} className="tabular-nums text-foreground">{formatDateTime(agent.lastHeartbeat, locale)}</time>
												</div>
											</div>
											<div className="ml-auto flex w-full shrink-0 items-center justify-end gap-2 md:w-auto">
												{!agent.isAlive && <Button variant="outline" size="icon" className="h-8 w-8 text-muted-foreground hover:text-destructive" onClick={() => setDeleteTarget(agent)} title={t.vulnScan.agents.actions.delete} aria-label={t.vulnScan.agents.actions.delete}><Trash2 className="h-4 w-4" /></Button>}
												<PythonEnvironmentDialog agentId={agent.agentId} name={agent.displayName || agent.lastSeenHost} online={agent.isAlive} />
											</div>
										</div>
									</CardContent>
								</Card>
							))}
						</div>
					)}
				</section>
			</div>
			<AgentUpdateManager nodes={agents} state={updates} open={updatesOpen} onOpenChange={setUpdatesOpen} initialAgent={updateTarget} />
			<AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && !deletingId && setDeleteTarget(null)}>
				<AlertDialogContent className={mobile.dialog}>
					<AlertDialogHeader>
						<AlertDialogTitle>{t.vulnScan.agents.deleteConfirm.title}</AlertDialogTitle>
						<AlertDialogDescription>
							{t.vulnScan.agents.deleteConfirm.description.replace('{name}', deleteTarget?.displayName || deleteTarget?.agentId || '-')}
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel disabled={!!deletingId}>{t.common.cancel}</AlertDialogCancel>
						<AlertDialogAction
							className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
							disabled={!!deletingId}
							onClick={(e) => {
								e.preventDefault()
								if (deleteTarget) {
									void deleteAgent(deleteTarget)
								}
							}}
						>
							{deletingId ? t.common.deleting : t.vulnScan.agents.actions.delete}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	)
}
