'use client'

import { useState, useEffect, useCallback, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { NumberInput } from '@/components/ui/number-input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { PasswordInput } from '@/components/ui/password-input'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import { Badge } from '@/components/ui/badge'
import { Save, RefreshCw, Network, Globe, Shield, Cpu, Zap, Server } from 'lucide-react'
import { fetchWithAuth } from '@/lib/api'
import { persistDesktopConfigItems } from '@/lib/api'
import toast from 'react-hot-toast'
import { useI18n } from '@/lib/i18n'

// 类型定义保持不变
interface NetworkConfig {
  defaultNetwork: string
}

interface ProxyConfig {
  enabled: boolean
  mode: 'HTTP' | 'SOCKS5'
  address: string
  port: number
  username: string
  password: string
}

interface ScanRuntimeConfig {
  maxConcurrentTasks: number
  memoryLimitMB: number
  maxMemoryLimitMB?: number
}

type ScanConfigTabProps = {
  variant?: 'card' | 'settings'
}

function SettingsSection({
  icon,
  title,
  description,
  action,
  children,
}: {
  icon: ReactNode
  title: string
  description?: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <section className="border-b border-border/70 pb-7 last:border-b-0">
      <div className="mb-3 flex items-center justify-between gap-5">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            {icon}
          </div>
          <div className="min-w-0 space-y-1">
            <h2 className="text-base font-semibold tracking-tight text-foreground">{title}</h2>
            {description ? (
              <p className="max-w-2xl text-sm leading-5 text-muted-foreground">{description}</p>
            ) : null}
          </div>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      <div className="divide-y divide-border/70">{children}</div>
    </section>
  )
}

function SettingsRow({
  label,
  value,
  description,
  children,
}: {
  label: string
  value?: ReactNode
  description?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="grid items-center gap-3 py-4 lg:grid-cols-[220px_minmax(0,1fr)]">
      <div className="min-w-0 space-y-1">
        <div className="flex min-w-0 items-center gap-2">
          <Label className="whitespace-nowrap text-sm font-medium text-foreground">{label}</Label>
          {value ? <div className="shrink-0">{value}</div> : null}
        </div>
        {description ? (
          <div className="text-sm leading-5 text-muted-foreground">{description}</div>
        ) : null}
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

export function ScanConfigTab({ variant = 'settings' }: ScanConfigTabProps = {}) {
  const { t } = useI18n()
  
  // State 保持不变
  const [networkConfig, setNetworkConfig] = useState<NetworkConfig>({ defaultNetwork: 'auto' })
  const [proxyConfig, setProxyConfig] = useState<ProxyConfig>({
    enabled: false, mode: 'HTTP', address: '127.0.0.1', port: 8080, username: '', password: '',
  })
  const [networkCards, setNetworkCards] = useState<Array<{ name: string; ip: string }>>([])
  const [networkLoading, setNetworkLoading] = useState(false)
  const [loading, setLoading] = useState(false)
  const [numCPU, setNumCPU] = useState<number>(0)
  const [gomax, setGomax] = useState<number>(0)
  const [requestedCores, setRequestedCores] = useState<number>(0)
  const [cpuLoading, setCpuLoading] = useState(false)
  const [scanRuntimeConfig, setScanRuntimeConfig] = useState<ScanRuntimeConfig>({ maxConcurrentTasks: 3, memoryLimitMB: 0, maxMemoryLimitMB: 8192 })
  const [scanRuntimeLoading, setScanRuntimeLoading] = useState(false)
  const [proxyConfigLoaded, setProxyConfigLoaded] = useState(false)
  const clampRequestedCores = (value: number) => {
    const maxCores = Math.max(1, numCPU || 1)
    return Math.max(1, Math.min(maxCores, value || 1))
  }

  const clampMemoryLimitMB = (value: number) => {
    const maxMemoryLimitMB = Math.max(0, scanRuntimeConfig.maxMemoryLimitMB || 0)
    return Math.max(0, Math.min(maxMemoryLimitMB, value || 0))
  }

  // 保存网卡配置到后端
  const saveNetworkConfigToBackend = useCallback(async (networkName: string) => {
    try {
      const response = await fetchWithAuth(`/api/v1/system/network-cards`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ defaultNetwork: networkName }),
      })
      if (!response.ok) throw new Error()
      return true
    } catch (error) {
      console.error('保存网卡配置失败:', error)
      return false
    }
  }, [])

  // 加载网卡配置
  const loadNetworkConfig = useCallback(async (cards: Array<{ name: string; ip: string }>) => {
    try {
      // 从后端获取保存的网卡配置
      const response = await fetchWithAuth(`/api/v1/system/network-cards`, { method: 'GET' })
      if (response.ok) {
        const data = await response.json()
        if (data.data && data.data.defaultNetwork) {
          const savedNetwork = data.data.defaultNetwork
          // 验证保存的网卡是否在当前可用列表中
          const isValidNetwork = savedNetwork === 'auto' || cards.some(card => card.name === savedNetwork)
          setNetworkConfig({ 
            defaultNetwork: isValidNetwork ? savedNetwork : 'auto' 
          })
          // 如果网卡不存在，自动保存为 auto
          if (!isValidNetwork && savedNetwork !== 'auto') {
            await saveNetworkConfigToBackend('auto')
          }
        }
      }
    } catch (error) {
      console.error('加载网卡配置失败:', error)
      // 如果后端读取失败，尝试从 localStorage 读取（向后兼容）
      const savedConfig = localStorage.getItem('networkConfig')
      if (savedConfig) {
        try { 
          const config = JSON.parse(savedConfig)
          const isValidNetwork = config.defaultNetwork === 'auto' || cards.some(card => card.name === config.defaultNetwork)
          setNetworkConfig({ 
            defaultNetwork: isValidNetwork ? config.defaultNetwork : 'auto' 
          })
        } catch (e) { console.error(e) }
      }
    }
  }, [saveNetworkConfigToBackend])

  // 加载代理配置
  const loadProxyConfig = useCallback(async () => {
    // 代理配置继续从 localStorage 读取（因为是客户端配置）
    const savedProxyConfig = localStorage.getItem('proxyConfig')
    if (savedProxyConfig) {
      try { setProxyConfig(JSON.parse(savedProxyConfig)) } catch (e) { console.error(e) }
    }
    setProxyConfigLoaded(true)
  }, [])

  useEffect(() => {
    if (!proxyConfigLoaded) {
      return
    }
    localStorage.setItem('proxyConfig', JSON.stringify(proxyConfig))
    void persistDesktopConfigItems({ proxyConfig: JSON.stringify(proxyConfig) })
    window.dispatchEvent(new CustomEvent('hephaestus:proxy-config-changed'))
  }, [proxyConfig, proxyConfigLoaded])

  // API 方法 - 加载网卡列表
  const loadNetworkCards = useCallback(async () => {
    setNetworkLoading(true)
    try {
      const response = await fetchWithAuth(`/api/v1/system/network-cards`, { method: 'GET' })
      if (!response.ok) throw new Error()
      const data = await response.json()
      if (data.data && Array.isArray(data.data)) {
        setNetworkCards(data.data)
        // 加载网卡列表后，读取保存的配置
        await loadNetworkConfig(data.data)
      }
    } catch (error) {
      toast.error(t.scanConfig.network.loadFailed)
    } finally {
      setNetworkLoading(false)
    }
  }, [loadNetworkConfig, t.scanConfig.network.loadFailed])

  const loadCPUInfo = useCallback(async () => {
    try {
      const response = await fetchWithAuth(`/api/v1/system/cpu`, { method: 'GET' })
      if (!response.ok) throw new Error()
      const data = await response.json()
      if (data.data) {
        const cpuCount = Number(data.data.numCPU) || 0
        const appliedCores = Number(data.data.gomaxprocs) || 0
        setNumCPU(cpuCount)
        setGomax(appliedCores)
        setRequestedCores(Math.max(1, Math.min(cpuCount || 1, appliedCores || 1)))
      }
    } catch (e) { console.error(e) }
  }, [])

  const loadScanRuntimeConfig = useCallback(async () => {
    try {
      const response = await fetchWithAuth(`/api/v1/system/scan-runtime`, { method: 'GET' })
      if (!response.ok) throw new Error()
      const data = await response.json()
      if (data.data) {
        const maxMemoryLimitMB = Math.max(0, Number(data.data.maxMemoryLimitMB) || 0)
        setScanRuntimeConfig({
          maxConcurrentTasks: Number(data.data.maxConcurrentTasks) || 3,
          memoryLimitMB: Math.max(0, Math.min(maxMemoryLimitMB, Number(data.data.memoryLimitMB) || 0)),
          maxMemoryLimitMB,
        })
      }
    } catch (error) {
      toast.error(t.scanConfig.runtime.loadFailed)
    }
  }, [t.scanConfig.runtime.loadFailed])

  // Effects - 加载配置和网卡列表
  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadNetworkCards()
      void loadCPUInfo()
      void loadProxyConfig()
      void loadScanRuntimeConfig()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadCPUInfo, loadNetworkCards, loadProxyConfig, loadScanRuntimeConfig])

  const saveConfig = async () => {
    setLoading(true)
    try {
      // 保存网卡配置到后端
      const networkSaved = await saveNetworkConfigToBackend(networkConfig.defaultNetwork)
      
      // 保存代理配置到 localStorage（客户端配置）
      localStorage.setItem('proxyConfig', JSON.stringify(proxyConfig))
      void persistDesktopConfigItems({ proxyConfig: JSON.stringify(proxyConfig) })
      window.dispatchEvent(new CustomEvent('hephaestus:proxy-config-changed'))
      
      // 同时保存到 localStorage 作为备份
      localStorage.setItem('networkConfig', JSON.stringify(networkConfig))
      void persistDesktopConfigItems({ networkConfig: JSON.stringify(networkConfig) })
      
      if (networkSaved) {
        toast.success(t.scanConfig.saveSuccess)
      } else {
        toast.error(t.scanConfig.saveFailed)
      }
    } catch (error) {
      toast.error(t.scanConfig.saveFailed)
    } finally {
      setLoading(false)
    }
  }

  const applyCores = async () => {
    setCpuLoading(true)
    try {
      const response = await fetchWithAuth(`/api/v1/system/cpu`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ cores: clampRequestedCores(requestedCores) }),
      })
      if (!response.ok) throw new Error()
      const res = await response.json()
      if (res.success) {
        const applied = clampRequestedCores(Number(res.data.applied) || requestedCores)
        setGomax(applied)
        setRequestedCores(applied)
        toast.success(t.scanConfig.cpu?.applySuccess || '已应用 CPU 设置')
      } else { throw new Error() }
    } catch (error) {
      toast.error(t.scanConfig.cpu?.applyFailed || '应用失败')
    } finally {
      setCpuLoading(false)
    }
  }

  const saveScanRuntimeConfig = async () => {
    setScanRuntimeLoading(true)
    try {
      const response = await fetchWithAuth(`/api/v1/system/scan-runtime`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          maxConcurrentTasks: Math.max(1, Math.min(32, scanRuntimeConfig.maxConcurrentTasks || 3)),
          memoryLimitMB: clampMemoryLimitMB(scanRuntimeConfig.memoryLimitMB),
        }),
      })
      if (!response.ok) throw new Error()
      const data = await response.json()
      const applied = Number(data?.data?.config?.maxConcurrentTasks) || scanRuntimeConfig.maxConcurrentTasks
      const maxMemoryLimitMB = Math.max(0, Number(data?.data?.maxMemoryLimitMB) || scanRuntimeConfig.maxMemoryLimitMB || 0)
      setScanRuntimeConfig({
        maxConcurrentTasks: applied,
        memoryLimitMB: Math.max(0, Math.min(maxMemoryLimitMB, Number(data?.data?.config?.memoryLimitMB) || 0)),
        maxMemoryLimitMB,
      })
      toast.success(t.scanConfig.runtime.saveSuccess)
    } catch (error) {
      toast.error(t.scanConfig.runtime.saveFailed)
    } finally {
      setScanRuntimeLoading(false)
    }
  }

  if (variant === 'settings') {
    return (
      <div className="w-full space-y-7 px-2 py-3">
        <SettingsSection
          icon={<Network className="h-5 w-5" />}
          title={t.scanConfig.network.title}
        >
          <SettingsRow label={t.scanConfig.network.defaultNic}>
            <div className="flex items-center gap-2">
              <Select
                value={networkConfig.defaultNetwork}
                onValueChange={async (value) => {
                  setNetworkConfig({ ...networkConfig, defaultNetwork: value })
                  const saved = await saveNetworkConfigToBackend(value)
                  if (saved) {
                    localStorage.setItem('networkConfig', JSON.stringify({ defaultNetwork: value }))
                    toast.success('网卡配置已保存')
                  }
                }}
              >
                <SelectTrigger id="defaultNetwork" className="min-w-0 flex-1 font-mono">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">
                    <span className="font-medium">{t.scanConfig.network.autoSelect}</span>
                  </SelectItem>
                  {networkCards.map((card, index) => (
                    <SelectItem key={index} value={card.name}>
                      <div className="flex w-full items-center justify-between gap-4">
                        <span className="font-medium">{card.name}</span>
                        <span className="font-mono text-xs text-muted-foreground">{card.ip}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="icon"
                onClick={loadNetworkCards}
                title="Refresh Network Cards"
                disabled={networkLoading}
              >
                <RefreshCw className={`h-4 w-4 ${networkLoading ? 'animate-spin' : ''}`} />
              </Button>
            </div>
          </SettingsRow>
        </SettingsSection>

        <SettingsSection
          icon={<Cpu className="h-5 w-5" />}
          title={t.scanConfig.cpu.performanceTitle || t.scanConfig.cpu.title}
        >
          <SettingsRow
            label={t.scanConfig.runtime.countLabel}
          >
            <div className="flex flex-col gap-3 sm:flex-row">
              <NumberInput
                min={1}
                max={32}
                value={scanRuntimeConfig.maxConcurrentTasks}
                onValueChange={(nextValue) =>
                  setScanRuntimeConfig(prev => ({
                    ...prev,
                    maxConcurrentTasks: Math.max(1, Math.min(32, nextValue || 1)),
                  }))
                }
                className="sm:flex-1"
                inputClassName="font-mono"
              />
              <Button
                variant="secondary"
                type="button"
                onClick={saveScanRuntimeConfig}
                disabled={scanRuntimeLoading}
              >
                {scanRuntimeLoading ? <RefreshCw className="mr-2 h-3 w-3 animate-spin" /> : null}
                {t.scanConfig.runtime.apply}
              </Button>
            </div>
          </SettingsRow>

          <SettingsRow
            label={t.scanConfig.cpu.requestedLabel}
          >
            <div className="flex flex-col gap-3 sm:flex-row">
              <NumberInput
                min={1}
                max={numCPU || 1}
                value={requestedCores}
                onValueChange={(nextValue) => setRequestedCores(clampRequestedCores(nextValue))}
                className="sm:flex-1"
                inputClassName="font-mono"
              />
              <Button
                variant="secondary"
                type="button"
                onClick={applyCores}
                disabled={cpuLoading}
              >
                {cpuLoading ? <RefreshCw className="mr-2 h-3 w-3 animate-spin" /> : null}
                {t.scanConfig.cpu.apply}
              </Button>
            </div>
          </SettingsRow>

          <SettingsRow
            label={t.scanConfig.runtime.memoryLabel}
          >
            <div className="flex flex-col gap-3 sm:flex-row">
              <NumberInput
                min={0}
                max={scanRuntimeConfig.maxMemoryLimitMB || 0}
                value={scanRuntimeConfig.memoryLimitMB}
                onValueChange={(nextValue) =>
                  setScanRuntimeConfig(prev => ({
                    ...prev,
                    memoryLimitMB: clampMemoryLimitMB(nextValue),
                  }))
                }
                className="sm:flex-1"
                inputClassName="font-mono"
              />
              <Button
                variant="secondary"
                type="button"
                onClick={saveScanRuntimeConfig}
                disabled={scanRuntimeLoading}
              >
                {scanRuntimeLoading ? <RefreshCw className="mr-2 h-3 w-3 animate-spin" /> : null}
                {t.scanConfig.cpu.apply}
              </Button>
            </div>
          </SettingsRow>
        </SettingsSection>

        <SettingsSection
          icon={<Globe className="h-5 w-5" />}
          title={t.scanConfig.proxy.title}
          action={
            <Switch
              checked={proxyConfig.enabled}
              onCheckedChange={(checked) => setProxyConfig({ ...proxyConfig, enabled: checked })}
            />
          }
        >
          <div className={`transition-opacity duration-200 ${!proxyConfig.enabled ? 'opacity-50 pointer-events-none grayscale' : ''}`}>
            <SettingsRow label={t.scanConfig.proxy.mode}>
              <div className="grid grid-cols-2 gap-2">
                {['HTTP', 'SOCKS5'].map((m) => (
                  <button
                    key={m}
                    type="button"
                    className={`flex h-10 items-center justify-center gap-2 rounded-md border text-sm font-medium transition-colors ${
                      proxyConfig.mode === m
                        ? 'border-primary bg-primary/5 text-primary'
                        : 'border-border hover:bg-muted'
                    }`}
                    onClick={() => setProxyConfig({ ...proxyConfig, mode: m as ProxyConfig['mode'] })}
                  >
                    {m === 'HTTP' ? <Server className="h-4 w-4" /> : <Shield className="h-4 w-4" />}
                    {m}
                  </button>
                ))}
              </div>
            </SettingsRow>

            <SettingsRow label={`${t.scanConfig.proxy.address} / ${t.scanConfig.proxy.port}`}>
              <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_160px]">
                <div className="relative">
                  <Input
                    placeholder="127.0.0.1"
                    value={proxyConfig.address}
                    onChange={(e) => setProxyConfig({ ...proxyConfig, address: e.target.value })}
                    className="font-mono pl-8"
                  />
                  <Globe className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground/50" />
                </div>
                <NumberInput
                  min={1}
                  max={65535}
                  value={proxyConfig.port}
                  onValueChange={(nextValue) => setProxyConfig({ ...proxyConfig, port: nextValue || 8080 })}
                  inputClassName="font-mono"
                />
              </div>
            </SettingsRow>

            <SettingsRow label="认证信息 (可选)">
              <div className="grid gap-3 sm:grid-cols-2">
                <Input
                  placeholder={t.scanConfig.proxy.placeholder.username}
                  value={proxyConfig.username}
                  onChange={(e) => setProxyConfig({ ...proxyConfig, username: e.target.value })}
                />
                <PasswordInput
                  placeholder={t.scanConfig.proxy.placeholder.password}
                  value={proxyConfig.password}
                  onChange={(e) => setProxyConfig({ ...proxyConfig, password: e.target.value })}
                />
              </div>
            </SettingsRow>
          </div>
        </SettingsSection>
      </div>
    )
  }

  return (
    <div className="space-y-4 p-1">
      
      {/* 顶部标题区（可选，如果Tab页已有标题可省略） */}
      {/* <div className="flex items-center justify-between">
       
        <Button onClick={saveConfig} disabled={loading} className="gap-2 shadow-sm">
          {loading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
          {t.common.save}
        </Button>
      </div> */}

      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        
        {/* 左侧列：网络与性能 */}
        <div className="space-y-6">
          
          {/* 网络接口配置卡片 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center gap-2">
                <Network className="w-5 h-5 text-primary" />
                <CardTitle className="text-base">{t.scanConfig.network.title}</CardTitle>
              </div>
              <CardDescription>{t.scanConfig.network.nicDescription}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-2">
                <Label htmlFor="defaultNetwork" className="text-xs font-semibold uppercase text-muted-foreground tracking-wider">
                  {t.scanConfig.network.defaultNic}
                </Label>
                <div className="flex gap-2 items-center">
                  <div className="flex-1 min-w-0">
                    <Select
                      value={networkConfig.defaultNetwork}
                      onValueChange={async (value) => {
                        setNetworkConfig({ ...networkConfig, defaultNetwork: value })
                        // 自动保存网卡配置到后端
                        const saved = await saveNetworkConfigToBackend(value)
                        if (saved) {
                          // 同时保存到 localStorage 作为备份
                          localStorage.setItem('networkConfig', JSON.stringify({ defaultNetwork: value }))
                          toast.success('网卡配置已保存')
                        }
                      }}
                    >
                      <SelectTrigger id="defaultNetwork" className="w-full font-mono">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="auto">
                          <span className="font-medium">{t.scanConfig.network.autoSelect}</span>
                        </SelectItem>
                        {networkCards.map((card, index) => (
                          <SelectItem key={index} value={card.name}>
                            <div className="flex items-center justify-between w-full gap-4">
                              <span className="font-medium">{card.name}</span>
                              <span className="text-muted-foreground font-mono text-xs">{card.ip}</span>
                            </div>
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={loadNetworkCards}
                    title="Refresh Network Cards"
                    disabled={networkLoading}
                  >
                    <RefreshCw className={`w-4 h-4 ${networkLoading ? 'animate-spin' : ''}`} />
                  </Button>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* CPU 性能卡片 */}
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Cpu className="w-5 h-5 text-primary" />
                  <CardTitle className="text-base">{t.scanConfig.cpu.performanceTitle || t.scanConfig.cpu.title}</CardTitle>
                </div>
              </div>
              <CardDescription>{t.scanConfig.cpu.performanceHelp || t.scanConfig.cpu.help}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="space-y-3">
                  <div className="flex justify-between items-center text-sm">
                    <Label className="text-muted-foreground">
                      {t.scanConfig.cpu.requestedLabel}
                    </Label>
                    <span className="font-mono font-medium">{requestedCores} Cores</span>
                  </div>
                  <div className="flex flex-col sm:flex-row gap-3">
                    <NumberInput
                      min={1}
                      max={numCPU || 1}
                      value={requestedCores}
                      onValueChange={(nextValue) => setRequestedCores(clampRequestedCores(nextValue))}
                      className="sm:flex-1"
                      inputClassName="font-mono"
                    />
                    <div className="flex gap-2">
                      <Button
                        variant="secondary"
                        type="button"
                        onClick={applyCores}
                        disabled={cpuLoading}
                      >
                        {cpuLoading ? (
                          <RefreshCw className="w-3 h-3 mr-2 animate-spin" />
                        ) : null}
                        {t.scanConfig.cpu.apply}
                      </Button>
                    </div>
                  </div>
                </div>

                <Separator />

                <div className="space-y-3">
                  <div className="flex justify-between items-center text-sm">
                    <Label className="text-muted-foreground">
                      {t.scanConfig.runtime.memoryLabel}
                    </Label>
                    <span className="font-mono font-medium">
                      {scanRuntimeConfig.memoryLimitMB > 0 ? `${scanRuntimeConfig.memoryLimitMB} MB` : (t.scanConfig.runtime.unlimited || '不限制')}
                    </span>
                  </div>
                  <div className="flex flex-col sm:flex-row gap-3">
                    <NumberInput
                      min={0}
                      max={scanRuntimeConfig.maxMemoryLimitMB || 0}
                      value={scanRuntimeConfig.memoryLimitMB}
                      onValueChange={(nextValue) =>
                        setScanRuntimeConfig(prev => ({
                          ...prev,
                          memoryLimitMB: clampMemoryLimitMB(nextValue),
                        }))
                      }
                      className="sm:flex-1"
                      inputClassName="font-mono"
                    />
                    <div className="flex gap-2">
                      <Button
                        variant="secondary"
                        type="button"
                        onClick={saveScanRuntimeConfig}
                        disabled={scanRuntimeLoading}
                      >
                        {scanRuntimeLoading ? (
                          <RefreshCw className="w-3 h-3 mr-2 animate-spin" />
                        ) : null}
                        {t.scanConfig.cpu.apply}
                      </Button>
                    </div>
                  </div>
                </div>

                <div className="bg-muted/50 rounded-md p-3 text-xs text-muted-foreground flex gap-2 items-start">
                  <Zap className="w-4 h-4 mt-0.5 text-yellow-500/80 shrink-0" />
                  <p>{t.scanConfig.runtime.help}</p>
                </div>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* 右侧列：代理配置 */}
        <div className="space-y-6">
          <Card>
            <CardHeader className="pb-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Server className="w-5 h-5 text-primary" />
                  <CardTitle className="text-base">{t.scanConfig.runtime.title}</CardTitle>
                </div>
                <Badge variant="secondary" className="font-mono px-2 py-1 text-xs">
                  {scanRuntimeConfig.maxConcurrentTasks}
                </Badge>
              </div>
              <CardDescription>{t.scanConfig.runtime.description}</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="space-y-3">
                  <Label className="text-muted-foreground">
                    {t.scanConfig.runtime.countLabel}
                  </Label>
                  <div className="flex flex-col sm:flex-row gap-3">
                    <NumberInput
                      min={1}
                      max={32}
                      value={scanRuntimeConfig.maxConcurrentTasks}
                      onValueChange={(nextValue) =>
                        setScanRuntimeConfig(prev => ({
                          ...prev,
                          maxConcurrentTasks: Math.max(1, Math.min(32, nextValue || 1)),
                        }))
                      }
                      className="sm:flex-1"
                      inputClassName="font-mono"
                    />
                    <Button
                      variant="secondary"
                      type="button"
                      onClick={saveScanRuntimeConfig}
                      disabled={scanRuntimeLoading}
                    >
                      {scanRuntimeLoading ? <RefreshCw className="w-3 h-3 mr-2 animate-spin" /> : null}
                      {t.scanConfig.runtime.apply}
                    </Button>
                  </div>
                </div>

                <div className="bg-muted/50 rounded-md p-3 text-xs text-muted-foreground flex gap-2 items-start">
                  <Zap className="w-4 h-4 mt-0.5 text-yellow-500/80 shrink-0" />
                  <p>{t.scanConfig.runtime.description}</p>
                </div>
              </div>
            </CardContent>
          </Card>

          <Card className={`border-l-4 ${proxyConfig.enabled ? 'border-l-primary' : 'border-l-transparent'}`}>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Globe className="w-5 h-5 text-primary" />
                  <CardTitle className="text-base">{t.scanConfig.proxy.title}</CardTitle>
                </div>
                <Switch
                  checked={proxyConfig.enabled}
                  onCheckedChange={(checked) => setProxyConfig({ ...proxyConfig, enabled: checked })}
                />
              </div>
              <CardDescription>{t.scanConfig.proxy.description}</CardDescription>
            </CardHeader>

            <Separator />
            
            <CardContent className={`space-y-6 pt-6 transition-opacity duration-200 ${!proxyConfig.enabled ? 'opacity-50 pointer-events-none grayscale' : ''}`}>
              
              {/* 代理模式选择 */}
              <div className="grid grid-cols-1 gap-1.5">
                <Label className="text-xs font-semibold text-muted-foreground uppercase">{t.scanConfig.proxy.mode}</Label>
                <div className="flex gap-2">
                   {['HTTP', 'SOCKS5'].map((m) => (
                      <div 
                        key={m}
                        className={`flex-1 border rounded-md p-3 cursor-pointer flex items-center justify-center gap-2 transition-colors ${proxyConfig.mode === m ? 'bg-primary/5 border-primary text-primary' : 'hover:bg-muted'}`}
                        onClick={() => setProxyConfig({...proxyConfig, mode: m as any})}
                      >
                         {m === 'HTTP' ? <Server className="w-4 h-4"/> : <Shield className="w-4 h-4"/>}
                         <span className="font-medium text-sm">{m}</span>
                      </div>
                   ))}
                </div>
              </div>

              {/* 地址与端口 */}
              <div className="grid grid-cols-3 gap-4">
                <div className="col-span-2 space-y-1.5">
                  <Label className="text-xs font-semibold text-muted-foreground uppercase">{t.scanConfig.proxy.address}</Label>
                  <div className="relative">
                    <Input
                      placeholder="127.0.0.1"
                      value={proxyConfig.address}
                      onChange={(e) => setProxyConfig({ ...proxyConfig, address: e.target.value })}
                      className="font-mono pl-8"
                    />
                    <Globe className="w-4 h-4 absolute left-2.5 top-2.5 text-muted-foreground/50" />
                  </div>
                </div>
                <div className="space-y-1.5">
                  <Label className="text-xs font-semibold text-muted-foreground uppercase">{t.scanConfig.proxy.port}</Label>
                  <NumberInput
                    min={1}
                    max={65535}
                    value={proxyConfig.port}
                    onValueChange={(nextValue) => setProxyConfig({ ...proxyConfig, port: nextValue || 8080 })}
                    inputClassName="font-mono"
                  />
                </div>
              </div>

              {/* 认证信息 */}
              <div className="space-y-3 pt-2">
                <div className="flex items-center justify-between">
                  <Label className="text-xs font-semibold text-muted-foreground uppercase flex items-center gap-1">
                    <Shield className="w-3 h-3" /> 认证信息 (可选)
                  </Label>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <Input
                    placeholder={t.scanConfig.proxy.placeholder.username}
                    value={proxyConfig.username}
                    onChange={(e) => setProxyConfig({ ...proxyConfig, username: e.target.value })}
                  />
                  <PasswordInput
                    placeholder={t.scanConfig.proxy.placeholder.password}
                    value={proxyConfig.password}
                    onChange={(e) => setProxyConfig({ ...proxyConfig, password: e.target.value })}
                  />
                </div>
              </div>

            </CardContent>
            
          </Card>
        </div>
      </div>
    </div>
  )
}
