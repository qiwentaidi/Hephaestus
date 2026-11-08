'use client'

import mobile from './agents-mobile.module.css'

import { useCallback, useState, useSyncExternalStore } from 'react'
import { Check, Copy, Download, RefreshCw } from 'lucide-react'
import { fetchWithAuth, getApiBaseURL } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import { useI18n } from '@/lib/i18n'
import toast from 'react-hot-toast'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from '@/components/ui/sheet'

interface Release {
  agentVersion: string
  releaseUrl: string
  packages: { os: string; arch: string; url: string }[]
}

const platforms = [
  // Material Design Icons: linux, apple, microsoft-windows (Apache-2.0).
  { value: 'linux', label: 'Linux', iconPath: 'M14.62 8.35c-.42.28-1.75 1.04-1.95 1.19c-.39.31-.75.29-1.14-.01c-.2-.16-1.53-.92-1.95-1.19c-.48-.31-.45-.7.08-.92c1.64-.69 3.28-.64 4.91.03c.49.21.51.6.05.9m7.22 7.28c-.93-2.09-2.2-3.99-3.84-5.66a4.3 4.3 0 0 1-1.06-1.88c-.1-.33-.17-.67-.24-1.01c-.2-.88-.29-1.78-.7-2.61c-.73-1.58-2-2.4-3.84-2.47c-1.81.05-3.16.81-3.95 2.4c-.21.43-.36.88-.46 1.34c-.17.76-.32 1.55-.5 2.32c-.15.65-.45 1.21-.96 1.71c-1.61 1.57-2.9 3.37-3.88 5.35c-.14.29-.28.58-.37.88c-.19.66.29 1.12.99.96c.44-.09.88-.18 1.3-.31c.41-.15.57-.05.67.35c.65 2.15 2.07 3.66 4.24 4.5c4.12 1.56 8.93-.66 9.97-4.58c.07-.27.17-.37.47-.27c.46.14.93.24 1.4.35c.49.09.85-.16.92-.64c.03-.26-.06-.49-.16-.73' },
  { value: 'darwin', label: 'macOS', iconPath: 'M18.71 19.5c-.83 1.24-1.71 2.45-3.05 2.47c-1.34.03-1.77-.79-3.29-.79c-1.53 0-2 .77-3.27.82c-1.31.05-2.3-1.32-3.14-2.53C4.25 17 2.94 12.45 4.7 9.39c.87-1.52 2.43-2.48 4.12-2.51c1.28-.02 2.5.87 3.29.87c.78 0 2.26-1.07 3.81-.91c.65.03 2.47.26 3.64 1.98c-.09.06-2.17 1.28-2.15 3.81c.03 3.02 2.65 4.03 2.68 4.04c-.03.07-.42 1.44-1.38 2.83M13 3.5c.73-.83 1.94-1.46 2.94-1.5c.13 1.17-.34 2.35-1.04 3.19c-.69.85-1.83 1.51-2.95 1.42c-.15-1.15.41-2.35 1.05-3.11' },
  { value: 'windows', label: 'Windows', iconPath: 'M3 12V6.75l6-1.32v6.48zm17-9v8.75l-10 .15V5.21zM3 13l6 .09v6.81l-6-1.15zm17 .25V22l-10-1.91V13.1z' },
] as const
type AgentPlatform = (typeof platforms)[number]['value']
const platformStorageKey = 'hephaestus-agent-install-platform'
const selectionClassName = 'group cursor-pointer gap-1.5 rounded-md border border-transparent px-1.5 text-xs sm:px-3 sm:text-sm text-muted-foreground first:rounded-l-md last:rounded-r-md hover:bg-muted/60 hover:text-foreground data-[state=on]:border-foreground/40 data-[state=on]:bg-background data-[state=on]:font-semibold data-[state=on]:text-foreground data-[state=on]:shadow-sm'

function isAgentPlatform(value: string | null): value is AgentPlatform {
  return platforms.some(platform => platform.value === value)
}

function bashQuote(value: string) { return `'${value.replace(/'/g, "'\\''")}'` }
function powershellQuote(value: string) { return `'${value.replace(/'/g, "''")}'` }

export function AgentInstallSheet({ serverAddress, agents }: {
  serverAddress: string
  agents: { agentVersion?: string; localWorker?: boolean }[]
}) {
  const { t } = useI18n()
  const text = t.vulnScan.agents.install
  const apiBase = useSyncExternalStore(useCallback(() => () => {}, []), getApiBaseURL, () => '')
  const [address, setAddress] = useState<string | null>(null)
  const [platform, setPlatform] = useState<AgentPlatform>('linux')
  const [release, setRelease] = useState<Release | null>(null)
  const [checking, setChecking] = useState(false)
  const server = (address ?? (serverAddress || apiBase)).trim().replace(/\/$/, '')
  const validServer = /^https?:\/\/[^\s]+$/.test(server)
  const isWindows = platform === 'windows'
  const supportsService = platform === 'linux'
  const filename = isWindows ? 'install.bat' : 'install.sh'
  const scriptURL = `${server}/api/v1/agent/${filename}`
  const command = !validServer ? '' : isWindows
    ? `Invoke-WebRequest -Uri ${powershellQuote(scriptURL)} -OutFile install.bat; if ($?) { & .\\install.bat -Server ${powershellQuote(server)} }`
    : `curl -fsSL ${bashQuote(scriptURL)} -o install.sh && bash install.sh --server ${bashQuote(server)}`
  const backgroundCommand = !validServer || !supportsService ? ''
    : `curl -fsSL ${bashQuote(scriptURL)} -o install.sh && sudo bash install.sh --server ${bashQuote(server)} --service`
  const updateCommand = isWindows
    ? '& ".\\hephaestus-agent\\update.bat"'
    : `${supportsService ? 'sudo ' : ''}bash ./hephaestus-agent/update.sh`
  const mismatched = release ? agents.filter(node => !node.localWorker && node.agentVersion !== release.agentVersion).length : 0

  const checkRelease = useCallback(async (notify = false) => {
    setChecking(true)
    try {
      const response = await fetchWithAuth('/api/v1/agent/release')
      const result = await response.json()
      if (!response.ok || !result.success) throw new Error(text.loadFailed)
      setRelease(result.data)
      if (notify) toast.success(text.checked)
    } catch {
      toast.error(text.loadFailed)
    } finally { setChecking(false) }
  }, [text.loadFailed, text.checked])

  const copy = async (value: string) => {
    if (await copyToClipboard(value)) toast.success(text.copied)
    else toast.error(text.copyFailed)
  }

  const selectPlatform = (value: string) => {
    // A single-value toggle group may emit an empty value when clicked again.
    if (!isAgentPlatform(value)) return
    setPlatform(value)
    try {
      localStorage.setItem(platformStorageKey, value)
    } catch {
      // The current selection still works when browser storage is unavailable.
    }
  }

  const handleOpenChange = (open: boolean) => {
    if (!open) return
    try {
      const saved = localStorage.getItem(platformStorageKey)
      if (isAgentPlatform(saved)) setPlatform(saved)
    } catch {
      // Keep the in-memory selection when browser storage is unavailable.
    }
    void checkRelease()
  }

  return (
    <Sheet onOpenChange={handleOpenChange}>
      <SheetTrigger asChild>
        <Button variant="outline" className="h-9"><Download className="mr-1 h-4 w-4" />{text.button}</Button>
      </SheetTrigger>
      <SheetContent side="right" className={`${mobile.sheet} w-full gap-0 overflow-hidden sm:max-w-[720px]`}>
        <SheetHeader className="shrink-0 border-b px-4 py-5 pr-12 sm:px-6 sm:pr-12">
          <SheetTitle>{text.title}</SheetTitle>
          <SheetDescription>{text.description}</SheetDescription>
        </SheetHeader>
        <div className="min-h-0 min-w-0 flex-1 space-y-5 overflow-y-auto overscroll-contain px-4 py-5 sm:px-6">
          <ToggleGroup type="single" value={platform} onValueChange={selectPlatform}
            aria-label={text.platform} className="w-full max-w-full gap-1 rounded-lg border bg-muted/30 p-1">
            {platforms.map(({ value, label, iconPath }) => (
              <ToggleGroupItem key={value} value={value}
                className={selectionClassName}>
                <svg aria-hidden="true" viewBox="0 0 24 24" className="size-4 shrink-0" fill="currentColor">
                  <path d={iconPath} />
                </svg>
                {label}
                <Check aria-hidden="true" className="hidden size-4 opacity-0 group-data-[state=on]:opacity-100 sm:block" />
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <p className="text-xs text-muted-foreground">{text.installDirectory}: <code className="font-mono text-foreground">{isWindows ? '.\\hephaestus-agent' : './hephaestus-agent'}</code></p>
          <div className="space-y-2">
            <Label htmlFor="agent-install-server">{text.server}</Label>
            <Input id="agent-install-server" value={server} onChange={event => setAddress(event.target.value)} placeholder="https://hephaestus.example.com" />
            <p className="text-xs text-muted-foreground">{text.serverHint}</p>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 p-3">
            <div className="space-y-1">
              <div className="text-sm">{text.version}: <span className="font-mono">{release?.agentVersion || '—'}</span></div>
              {release && <Badge variant="outline" className={mismatched ? 'border-amber-500/40 text-amber-600 dark:text-amber-400' : ''}>
                {mismatched ? text.mismatched.replace('{count}', String(mismatched)) : text.matched}
              </Badge>}
            </div>
            <Button variant="outline" size="sm" disabled={checking} onClick={() => void checkRelease(true)}>
              <RefreshCw className={`mr-1 h-4 w-4 ${checking ? 'animate-spin' : ''}`} />{text.check}
            </Button>
          </div>
          <div className="space-y-2">
            <Label htmlFor="agent-install-command">{isWindows ? text.windowsCommand : text.command}</Label>
            <div className={mobile.command}>
              <Textarea id="agent-install-command" readOnly rows={3} value={command} className="min-w-0 font-mono text-xs" />
              <Button variant="outline" size="icon" disabled={!command} onClick={() => void copy(command)} aria-label={text.copy}>
                <Copy className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">{text.foregroundHint}</p>
            {!supportsService && <p className="text-xs leading-relaxed text-muted-foreground">
              {text.backgroundUnsupported.replace('{platform}', isWindows ? 'Windows' : 'macOS')}
            </p>}
          </div>
          {supportsService && <div className="space-y-2">
            <Label htmlFor="agent-background-install-command">{text.backgroundCommand}</Label>
            <div className={mobile.command}>
              <Textarea id="agent-background-install-command" readOnly rows={3} value={backgroundCommand} className="min-w-0 font-mono text-xs" />
              <Button variant="outline" size="icon" disabled={!backgroundCommand} onClick={() => void copy(backgroundCommand)} aria-label={text.copyBackground}>
                <Copy className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">{text.backgroundHint}</p>
          </div>}
          <div className="space-y-2">
            {validServer && <Button variant="outline" size="sm" asChild><a href={scriptURL} download={filename}>
              <Download className="mr-1 h-4 w-4" />{text.download.replace('{file}', filename)}
            </a></Button>}
            <p className="text-xs leading-relaxed text-muted-foreground">{text.configureHint}</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="agent-update-command">{text.update}</Label>
            <div className={mobile.command}>
              <Textarea id="agent-update-command" readOnly rows={1} value={updateCommand} className="min-w-0 font-mono text-xs" />
              <Button variant="outline" size="icon" onClick={() => void copy(updateCommand)} aria-label={text.copyUpdate}>
                <Copy className="h-4 w-4" />
              </Button>
            </div>
            <p className="text-xs leading-relaxed text-muted-foreground">{supportsService ? text.backgroundUpdateHint : text.updateHint}</p>
          </div>
          {release && <div className="flex flex-wrap gap-2">
            {release.packages.filter(item => item.os === platform).map(item => (
              <Button key={item.arch} variant="outline" size="sm" asChild><a href={item.url}>
                <Download className="mr-1 h-4 w-4" />{item.arch} ZIP
              </a></Button>
            ))}
            <Button variant="ghost" size="sm" asChild><a href={release.releaseUrl} target="_blank" rel="noreferrer">{text.release}</a></Button>
          </div>}
        </div>
      </SheetContent>
    </Sheet>
  )
}
