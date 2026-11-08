'use client'

import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { PasswordInput } from '@/components/ui/password-input'
import { Switch } from '@/components/ui/switch'
import {
    Save, ChevronDown, ExternalLink,
    CheckCircle2, Search, Globe, Loader2
} from 'lucide-react'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { getSpaceEngineConfig, saveSpaceEngineConfig, notifySpaceEngineConfigChange, type SpaceEngineConfig } from '@/lib/config'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'
import toast from '@/lib/toast'

// Helper to get initials from a string
const getInitials = (name: string) => name.slice(0, 1).toUpperCase()

interface ApiKeyCardProps {
    title: string
    description?: string
    icon?: React.ReactNode
    link?: string
    isOpen: boolean
    onToggle: () => void
    isConfigured?: boolean
    isRecommended?: boolean
    enableSwitch?: {
        checked: boolean
        onChange: (checked: boolean) => void
    }
    children: React.ReactNode
}

function ApiKeyCard({
    title,
    description,
    icon,
    link,
    isOpen,
    onToggle,
    isConfigured,
    isRecommended,
    enableSwitch,
    children
}: ApiKeyCardProps) {
    const { t } = useI18n()

    return (
        <Collapsible open={isOpen} onOpenChange={onToggle} className="min-w-0 w-full">
            <div className={cn(
                "rounded-md border bg-card text-card-foreground shadow-sm transition-all duration-200 group hover:shadow-md",
                isOpen ? "ring-1 ring-primary/20 border-primary/50" : "hover:border-primary/20"
            )}>
                <CollapsibleTrigger asChild>
                    <div className="flex items-center justify-between gap-3 p-3 cursor-pointer select-none">
                        <div className="flex min-w-0 flex-1 items-center gap-3">
                            {/* Logo / Icon Area */}
                            <div className={cn(
                                "flex shrink-0 items-center justify-center w-9 h-9 rounded bg-muted/50 text-muted-foreground font-semibold text-sm transition-colors",
                                isConfigured && "bg-primary/5 text-primary",
                                isOpen && !isConfigured && "bg-muted text-foreground"
                            )}>
                                {icon || getInitials(title)}
                            </div>

                            <div className="flex min-w-0 flex-col gap-0.5 text-left">
                                <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                                    <span className="truncate font-medium text-sm">{title}</span>
                                    {isRecommended && (
                                        <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded-full bg-blue-50 text-blue-600 dark:bg-blue-900/20 dark:text-blue-400 font-medium">
                                            {t.apiKeys.recommended}
                                        </span>
                                    )}
                                    {isConfigured && (
                                        <span className="shrink-0 text-[10px] px-1.5 py-0.5 rounded-full bg-green-50 text-green-600 dark:bg-green-900/20 dark:text-green-400 font-medium flex items-center gap-0.5">
                                            <CheckCircle2 className="w-2.5 h-2.5" />
                                            {t.apiKeys.configured}
                                        </span>
                                    )}
                                </div>
                                {description && <span className="block max-w-[220px] truncate text-[11px] text-muted-foreground">{description}</span>}
                            </div>
                        </div>

                        <div className="flex shrink-0 items-center gap-2">
                            {/* Enable Switch */}
                            {enableSwitch && (
                                <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                                    <Switch
                                        checked={enableSwitch.checked}
                                        onCheckedChange={enableSwitch.onChange}
                                    />
                                </div>
                            )}
                            {/* Register Link in Header */}
                            {link && (
                                <a
                                    href={link}
                                    target="_blank"
                                    rel="noopener noreferrer"
                                    className="inline-flex shrink-0 items-center gap-1 whitespace-nowrap text-[11px] px-2.5 py-1 rounded-full bg-primary/15 text-primary hover:bg-primary/25 transition-colors z-10 mr-2 font-medium"
                                    onClick={(e) => e.stopPropagation()}
                                >
                                    <span>{t.common.register}</span>
                                    <ExternalLink className="w-3 h-3" />
                                </a>
                            )}
                            <div className={cn("transition-transform duration-200 text-muted-foreground/50 group-hover:text-muted-foreground", isOpen && "rotate-180")}>
                                <ChevronDown className="w-4 h-4" />
                            </div>
                        </div>
                    </div>
                </CollapsibleTrigger>
                <CollapsibleContent>
                    <div className="px-3 pb-3 pt-0">
                        <div className="pt-3 border-t space-y-3">
                            {children}
                        </div>
                    </div>
                </CollapsibleContent>
            </div>
        </Collapsible>
    )
}

export function ApiKeysTab() {
    const { t } = useI18n()
    const [spaceConfig, setSpaceConfig] = useState<SpaceEngineConfig>({
        fofa: { api: 'https://fofa.info', email: '', key: '' },
        hunter: { api: 'https://hunter.qianxin.com', key: '' },
        quake: { api: 'https://quake.360.cn', key: '' },
        censys: { token: '', organizationId: '' },
        zoomeye: { api: 'https://api.zoomeye.ai/', key: '' },
        shodan: { api: 'https://api.shodan.io', key: '' },
        github: { token: '' },
        chaos: { key: '' },
        bevigil: { key: '' },
        securitytrails: { key: '' },
        ctRadar: { key: '' },
    })
    // All closed by default to save space
    const [openSections, setOpenSections] = useState<Record<string, boolean>>({})
    const [isSaving, setIsSaving] = useState(false)

    useEffect(() => {
        let cancelled = false

        const loadConfigs = async () => {
            const spaceResult = await getSpaceEngineConfig()

            if (cancelled) return

            setSpaceConfig({
                fofa: spaceResult.fofa || { api: 'https://fofa.info', email: '', key: '' },
                hunter: spaceResult.hunter || { api: 'https://hunter.qianxin.com', key: '' },
                quake: spaceResult.quake || { key: '' },
                censys: spaceResult.censys || { token: '', organizationId: '' },
                zoomeye: {
                    ...(spaceResult.zoomeye || { key: '' }),
                    api: spaceResult.zoomeye?.api || 'https://api.zoomeye.ai/',
                },
                shodan: {
                    ...(spaceResult.shodan || { key: '' }),
                    api: spaceResult.shodan?.api || 'https://api.shodan.io',
                },
                github: spaceResult.github || { token: '' },
                chaos: spaceResult.chaos || { key: '' },
                bevigil: spaceResult.bevigil || { key: '' },
                securitytrails: spaceResult.securitytrails || { key: '' },
                ctRadar: spaceResult.ctRadar || { key: '' },
            })
        }

        void loadConfigs().catch((error) => {
            console.error('Failed to load space engine config:', error)
        })

        return () => {
            cancelled = true
        }
    }, [])

    const saveConfig = async () => {
        setIsSaving(true)
        try {
            await saveSpaceEngineConfig(spaceConfig)
            notifySpaceEngineConfigChange(spaceConfig)
            toast.success(t.common.configSaved)
        } catch (error: any) {
            toast.error(t.common.saveFailed + ': ' + (error.message || ''))
        } finally {
            setIsSaving(false)
        }
    }

    const toggleSection = (section: string) => {
        setOpenSections(prev => ({ ...prev, [section]: !prev[section] }))
    }

    return (
        <div className="space-y-6 animate-in fade-in slide-in-from-bottom-2 duration-300">
            <div className="flex flex-col items-start justify-between gap-3 border-b py-3 sm:flex-row sm:items-center">
                <div>
                    <h2 className="text-lg font-semibold tracking-tight">{t.apiKeys.pageTitle}</h2>
                    <p className="text-xs text-muted-foreground">{t.apiKeys.pageDescription}</p>
                </div>
                <Button onClick={saveConfig} size="sm" className="h-8 shadow-sm" disabled={isSaving}>
                    {isSaving ? <Loader2 className="w-3.5 h-3.5 mr-1.5 animate-spin" /> : <Save className="w-3.5 h-3.5 mr-1.5" />}
                    <span aria-live="polite">{t.common.saveConfig}</span>
                </Button>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-x-6 gap-y-8 pb-10">
                {/* Left Column: Space Engines */}
                <div className="min-w-0 space-y-6">
                    {/* Space Engines */}
                    <section className="space-y-3">
                        <div className="flex items-center gap-2 pb-1 border-b">
                            <Globe className="w-4 h-4 text-primary/70" />
                            <h3 className="text-sm font-semibold text-foreground/80">{t.apiKeys.spaceEngine}</h3>
                        </div>

                        <div className="grid grid-cols-1 gap-3">
                            <ApiKeyCard
                                title={t.apiKeys.fofa.title}
                                description={t.apiKeys.fofa.description}
                                link="https://fofa.info/register"
                                isOpen={openSections.fofa}
                                onToggle={() => toggleSection('fofa')}
                                isConfigured={!!spaceConfig.fofa.key}
                                isRecommended
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="fofaEmail" className="text-xs">{t.apiKeys.email}</Label>
                                        <Input
                                            id="fofaEmail"
                                            type="email"
                                            className="h-8 text-sm"
                                            placeholder="your@email.com"
                                            value={spaceConfig.fofa.email}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, fofa: { ...spaceConfig.fofa, email: e.target.value } })}
                                        />
                                    </div>
                                    <div className="space-y-1">
                                        <Label htmlFor="fofaKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="fofaKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.fofa.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, fofa: { ...spaceConfig.fofa, key: e.target.value } })}
                                        />
                                    </div>
                                    <div className="space-y-1">
                                        <Label htmlFor="fofaApi" className="text-xs">{t.apiKeys.apiAddress}</Label>
                                        <Input
                                            id="fofaApi"
                                            className="h-8 text-sm font-mono text-xs"
                                            placeholder="https://fofa.info"
                                            value={spaceConfig.fofa.api}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, fofa: { ...spaceConfig.fofa, api: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.hunter.title}
                                description={t.apiKeys.hunter.description}
                                link="https://hunter.qianxin.com/"
                                isOpen={openSections.hunter}
                                onToggle={() => toggleSection('hunter')}
                                isConfigured={!!spaceConfig.hunter.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="hunterKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="hunterKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.hunter.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, hunter: { ...spaceConfig.hunter, key: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.quake.title}
                                description={t.apiKeys.quake.description}
                                link="https://quake.360.cn/"
                                isOpen={openSections.quake}
                                onToggle={() => toggleSection('quake')}
                                isConfigured={!!spaceConfig.quake.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="quakeKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="quakeKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.quake.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, quake: { ...spaceConfig.quake, key: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.zoomeye.title}
                                description={t.apiKeys.zoomeye.description}
                                link="https://www.zoomeye.org/register"
                                isOpen={openSections.zoomeye}
                                onToggle={() => toggleSection('zoomeye')}
                                isConfigured={!!spaceConfig.zoomeye.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="zoomeyeKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="zoomeyeKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.zoomeye.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, zoomeye: { ...spaceConfig.zoomeye, key: e.target.value } })}
                                        />
                                    </div>
                                    <div className="space-y-1">
                                        <Label htmlFor="zoomeyeApi" className="text-xs">{t.apiKeys.apiAddress}</Label>
                                        <Input
                                            id="zoomeyeApi"
                                            className="h-8 text-sm font-mono text-xs"
                                            placeholder="https://api.zoomeye.ai/"
                                            value={spaceConfig.zoomeye.api}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, zoomeye: { ...spaceConfig.zoomeye, api: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.shodan.title}
                                description={t.apiKeys.shodan.description}
                                link="https://account.shodan.io/"
                                isOpen={openSections.shodan}
                                onToggle={() => toggleSection('shodan')}
                                isConfigured={!!spaceConfig.shodan?.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="shodanKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="shodanKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="Shodan API Key"
                                            value={spaceConfig.shodan?.key || ''}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, shodan: { ...spaceConfig.shodan, key: e.target.value } })}
                                        />
                                    </div>
                                    <div className="space-y-1">
                                        <Label htmlFor="shodanApi" className="text-xs">{t.apiKeys.apiAddress}</Label>
                                        <Input
                                            id="shodanApi"
                                            className="h-8 text-sm font-mono text-xs"
                                            placeholder="https://api.shodan.io"
                                            value={spaceConfig.shodan?.api || ''}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, shodan: { ...spaceConfig.shodan, api: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.censys.title}
                                description={t.apiKeys.censys.description}
                                link="https://search.censys.io/register"
                                isOpen={openSections.censys}
                                onToggle={() => toggleSection('censys')}
                                isConfigured={!!spaceConfig.censys?.token}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="censysToken" className="text-xs">{t.apiKeys.token}</Label>
                                        <PasswordInput
                                            id="censysToken"
                                            inputClassName="h-8 text-sm"
                                            placeholder="censys_..."
                                            value={spaceConfig.censys?.token || ''}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, censys: { ...spaceConfig.censys, token: e.target.value } })}
                                        />
                                    </div>
                                    <div className="space-y-1">
                                        <Label htmlFor="censysOrgId" className="text-xs">Organization ID（可选）</Label>
                                        <Input
                                            id="censysOrgId"
                                            className="h-8 text-sm font-mono"
                                            placeholder="11111111-2222-3333-4444-555555555555"
                                            value={spaceConfig.censys?.organizationId || ''}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, censys: { ...spaceConfig.censys, organizationId: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>
                        </div>
                    </section>
                </div>

                <div className="min-w-0 space-y-6">
                    {/* Domain Collection */}
                    <section className="space-y-3">
                        <div className="flex items-center gap-2 pb-1 border-b">
                            <Search className="w-4 h-4 text-primary/70" />
                            <h3 className="text-sm font-semibold text-foreground/80">{t.apiKeys.domainCollection}</h3>
                        </div>

                        <div className="grid grid-cols-1 gap-3">
                            <ApiKeyCard
                                title={t.apiKeys.chaos.title}
                                description={t.apiKeys.chaos.description}
                                link="https://chaos.projectdiscovery.io/#/register"
                                isOpen={openSections.chaos}
                                onToggle={() => toggleSection('chaos')}
                                isConfigured={!!spaceConfig.chaos.key}
                                isRecommended
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="chaosKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="chaosKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.chaos.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, chaos: { ...spaceConfig.chaos, key: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.bevigil.title}
                                description={t.apiKeys.bevigil.description}
                                link="https://bevigil.com/osint-api"
                                isOpen={openSections.bevigil}
                                onToggle={() => toggleSection('bevigil')}
                                isConfigured={!!spaceConfig.bevigil.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="bevigilKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="bevigilKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.bevigil.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, bevigil: { ...spaceConfig.bevigil, key: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.securitytrails.title}
                                description={t.apiKeys.securitytrails.description}
                                link="https://securitytrails.com/corp/api"
                                isOpen={openSections.securitytrails}
                                onToggle={() => toggleSection('securitytrails')}
                                isConfigured={!!spaceConfig.securitytrails.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="securitytrailsKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="securitytrailsKey"
                                            inputClassName="h-8 text-sm"
                                            placeholder="API Key"
                                            value={spaceConfig.securitytrails.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, securitytrails: { ...spaceConfig.securitytrails, key: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            <ApiKeyCard
                                title={t.apiKeys.github.title}
                                description={t.apiKeys.github.description}
                                link="https://github.com/settings/tokens/new"
                                isOpen={openSections.github}
                                onToggle={() => toggleSection('github')}
                                isConfigured={!!spaceConfig.github.token}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="githubToken" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="githubToken"
                                            inputClassName="h-8 text-sm"
                                            placeholder="ghp_..."
                                            value={spaceConfig.github.token}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, github: { ...spaceConfig.github, token: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard>

                            {/* <ApiKeyCard
                                title={t.apiKeys.ctRadar.title}
                                description={t.apiKeys.ctRadar.description}
                                link="https://cert.imfht.com"
                                isOpen={openSections.ctRadar}
                                onToggle={() => toggleSection('ctRadar')}
                                isConfigured={!!spaceConfig.ctRadar.key}
                            >
                                <div className="space-y-3">
                                    <div className="space-y-1">
                                        <Label htmlFor="ctRadarKey" className="text-xs">{t.apiKeys.apiKey}</Label>
                                        <PasswordInput
                                            id="ctRadarKey"
                                            className="h-8 text-sm"
                                            placeholder="ctr_..."
                                            value={spaceConfig.ctRadar.key}
                                            onChange={(e) => setSpaceConfig({ ...spaceConfig, ctRadar: { ...spaceConfig.ctRadar, key: e.target.value } })}
                                        />
                                    </div>
                                </div>
                            </ApiKeyCard> */} {/* 已下线，暂时注释 */}
                        </div>
                    </section>
                </div>
            </div>
        </div>
    )
}
