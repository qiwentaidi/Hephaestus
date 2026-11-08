'use client'

import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Card, CardContent } from '@/components/ui/card'
import { Save, RefreshCw, AlertCircle } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import toast from 'react-hot-toast'
import { useI18n } from '@/lib/i18n'

interface FeishuConfig {
	webhookUrl: string
	enabled: boolean
	notifyOnVulnFound: boolean
	notifyOnScanComplete: boolean
}

export function FeishuNotification() {
	const { t } = useI18n()
	const [config, setConfig] = useState<FeishuConfig>({
		webhookUrl: '',
		enabled: false,
		notifyOnVulnFound: true,
		notifyOnScanComplete: true,
	})
	const [loading, setLoading] = useState(false)
	const [testLoading, setTestLoading] = useState(false)

	useEffect(() => {
		let cancelled = false

		const loadConfig = async () => {
			try {
				const res = await fetchWithAuth(`/api/v1/config/feishu`)
				if (!res.ok) {
					throw new Error(`HTTP ${res.status}`)
				}
				const result = await res.json()
				if (!cancelled && result.success && result.data) {
					const data = result.data
					setConfig({
						webhookUrl: data.webhookUrl || '',
						enabled: data.enabled || false,
						notifyOnVulnFound: data.notifyOnVulnFound !== false,
						notifyOnScanComplete: data.notifyOnScanComplete !== false,
					})
				}
			} catch (error) {
				if (!cancelled) {
					console.error(t.notification.loadFailed, error)
				}
			}
		}

		void loadConfig()

		return () => {
			cancelled = true
		}
	}, [t.notification.loadFailed])

	const saveConfig = async () => {
		setLoading(true)
		try {
			const res = await fetchWithAuth(`/api/v1/config/feishu`, {
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(config),
			})
			if (!res.ok) {
				throw new Error(`HTTP ${res.status}`)
			}
			const data = await res.json()
			if (!data.success) {
				throw new Error(data.error || t.notification.saveFailed)
			}
			toast.success(t.notification.saveSuccess)
		} catch (error: any) {
			toast.error(`${t.notification.saveFailed}: ${error.message || ''}`)
		} finally {
			setLoading(false)
		}
	}

	const testConnection = async () => {
		if (!config.webhookUrl) {
			toast.error(t.notification.fillWebhookFirst)
			return
		}
		setTestLoading(true)
		try {
			const res = await fetchWithAuth(`/api/v1/config/feishu/test`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ webhookUrl: config.webhookUrl }),
			})
			if (!res.ok) {
				throw new Error(`HTTP ${res.status}`)
			}
			const data = await res.json()
			if (data.success) {
				toast.success(t.notification.feishu.testSuccess)
			} else {
				throw new Error(data.error || t.notification.feishu.testFailed)
			}
		} catch (error: any) {
			toast.error(`${t.notification.commonTestFailed}: ${error.message || ''}`)
		} finally {
			setTestLoading(false)
		}
	}

	const isConfigured = config.webhookUrl

	return (
		<div className="space-y-6">
			{/* 配置卡片 */}
			<Card>
				<CardContent className="pt-6">
					<div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
						{/* 左侧：Webhook 配置 */}
						<div className="space-y-6">
							<div>
								<h3 className="text-sm font-semibold mb-4 flex items-center gap-2">
									<span className="inline-block w-1 h-4 bg-blue-500 rounded-full"></span>
									{t.notification.feishu.configTitle}
								</h3>
								<div className="space-y-4">
									{/* Webhook URL */}
									<div className="space-y-2">
										<Label htmlFor="webhookUrl" className="text-xs font-semibold uppercase text-muted-foreground tracking-wider">
											{t.notification.feishu.webhook}
										</Label>
										<Input
											id="webhookUrl"
											type="url"
											placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..."
											value={config.webhookUrl}
											onChange={(e) => setConfig({ ...config, webhookUrl: e.target.value })}
											className="font-mono text-xs"
										/>
										<p className="text-xs text-muted-foreground">
											{t.notification.feishu.webhookHelp}
										</p>
									</div>

									{/* 测试连接 */}
									<Button
										variant="outline"
										onClick={testConnection}
										disabled={testLoading || !isConfigured}
										className="w-full"
									>
										{testLoading ? (
											<RefreshCw className="w-4 h-4 mr-2 animate-spin" />
										) : null}
										{t.notification.testConnection}
									</Button>
								</div>
							</div>
						</div>

						{/* 右侧：通知选项 */}
						<div className="space-y-6">
							<div>
								<div className="flex items-center justify-between mb-4">
									<h3 className="text-sm font-semibold flex items-center gap-2">
										<span className="inline-block w-1 h-4 bg-blue-500 rounded-full"></span>
										{t.notification.optionsTitle}
									</h3>
									<div className="flex items-center gap-2">
										<span className="text-xs text-muted-foreground">{t.notification.enable}</span>
										<Switch
											id="enabled"
											checked={config.enabled}
											onCheckedChange={(checked) => setConfig({ ...config, enabled: checked })}
										/>
									</div>
								</div>

								<div className={`space-y-4 rounded-lg border p-4 transition-opacity ${config.enabled ? 'opacity-100 bg-background' : 'opacity-50 bg-muted/30'}`}>
									{/* 发现漏洞时通知 */}
									<div className="flex items-start justify-between gap-4 pb-4 border-b last:border-b-0 last:pb-0">
										<div className="space-y-1.5 flex-1 min-w-0">
											<Label htmlFor="notifyOnVulnFound" className="text-sm font-medium">
												{t.notification.feishu.notifyOnVulnFound}
											</Label>
											<p className="text-xs text-muted-foreground">
												{t.notification.feishu.notifyOnVulnFoundDesc}
											</p>
										</div>
										<Switch
											id="notifyOnVulnFound"
											checked={config.notifyOnVulnFound}
											onCheckedChange={(checked) =>
												setConfig({ ...config, notifyOnVulnFound: checked })
											}
											disabled={!config.enabled}
										/>
									</div>

									{/* 扫描完成时通知 */}
									<div className="flex items-start justify-between gap-4">
										<div className="space-y-1.5 flex-1 min-w-0">
											<Label htmlFor="notifyOnScanComplete" className="text-sm font-medium">
												{t.notification.feishu.notifyOnScanComplete}
											</Label>
											<p className="text-xs text-muted-foreground">
												{t.notification.scanCompleteSimpleDesc}
											</p>
										</div>
										<Switch
											id="notifyOnScanComplete"
											checked={config.notifyOnScanComplete}
											onCheckedChange={(checked) =>
												setConfig({ ...config, notifyOnScanComplete: checked })
											}
											disabled={!config.enabled}
										/>
									</div>
								</div>
							</div>

							{/* 提示信息 */}
							<div className="bg-blue-500/10 border border-blue-500/20 rounded-lg p-3 text-xs text-blue-600 dark:text-blue-400 flex gap-2 items-start">
								<AlertCircle className="w-4 h-4 mt-0.5 shrink-0" />
								<p>{t.notification.feishu.tip}</p>
							</div>
						</div>
					</div>
				</CardContent>
			</Card>

			{/* 保存按钮 */}
			<div className="flex justify-end gap-2">
				<Button
					onClick={saveConfig}
					disabled={loading}
					className="gap-2 shadow-sm"
				>
					{loading ? (
						<RefreshCw className="w-4 h-4 animate-spin" />
					) : (
						<Save className="w-4 h-4" />
					)}
					{t.notification.save}
				</Button>
			</div>
		</div>
	)
}
