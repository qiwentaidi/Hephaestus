import React, { useEffect, useState } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { fetchWithAuth } from '@/lib/api'
import { AlertTriangle, HelpCircle, Fingerprint, Scan, Shield, Target, Loader2 } from 'lucide-react'
import { SelectTag, type SelectTagOption } from '@/components/ui/select-tag'
import { useI18n } from '@/lib/i18n'
import { useScreenshotSnapChromeWarning } from '@/lib/use-screenshot-snap-chrome-warning'

interface WebScanSettingsProps {
  webscanOption: 1 | 2 | 3 | 4
  setWebscanOption: (val: 1 | 2 | 3 | 4) => void
  customHeaders: string
  setCustomHeaders: (val: string) => void
  customTemplate: string[]
  setCustomTemplate: (val: string[]) => void
  customTags: string[]
  setCustomTags: (val: string[]) => void
  threads: number
  setThreads: (val: number) => void
  nucleiThreads: number
  setNucleiThreads: (val: number) => void
  activeTimeoutLimit: number
  setActiveTimeoutLimit: (val: number) => void
  disableMaxHostErr: boolean
  setDisableMaxHostErr: (val: boolean) => void
  rootPathScan: boolean
  setRootPathScan: (val: boolean) => void
  skipNucleiWithoutTags: boolean
  setSkipNucleiWithoutTags: (val: boolean) => void
  skipNucleiWithWAF: boolean
  setSkipNucleiWithWAF: (val: boolean) => void
  screenshot: boolean
  setScreenshot: (val: boolean) => void
  oobEnabled: boolean
  setOobEnabled: (val: boolean) => void
  interactshURL: string
  setInteractshURL: (val: string) => void
  interactshToken: string
  setInteractshToken: (val: string) => void
  scanTemplates: any[]
  selectedTemplateId: string
  setSelectedTemplateId: (val: string) => void
  onApplyTemplate: (id: string) => void
  onClearTemplate: () => void
  templateOptions: SelectTagOption[]
  fingerprintOptions: SelectTagOption[]
}

export function WebScanSettings({
  webscanOption,
  setWebscanOption,
  customHeaders,
  setCustomHeaders,
  customTemplate,
  setCustomTemplate,
  customTags,
  setCustomTags,
  threads,
  setThreads,
  nucleiThreads,
  setNucleiThreads,
  activeTimeoutLimit,
  setActiveTimeoutLimit,
  disableMaxHostErr,
  setDisableMaxHostErr,
  rootPathScan,
  setRootPathScan,
  skipNucleiWithoutTags,
  setSkipNucleiWithoutTags,
  skipNucleiWithWAF,
  setSkipNucleiWithWAF,
  screenshot,
  setScreenshot,
  oobEnabled,
  setOobEnabled,
  interactshURL,
  setInteractshURL,
  interactshToken,
  setInteractshToken,
  scanTemplates,
  selectedTemplateId,
  setSelectedTemplateId,
  onApplyTemplate,
  onClearTemplate,
  templateOptions,
  fingerprintOptions,
}: WebScanSettingsProps) {
  const { t } = useI18n()
  const showScreenshotSnapChromeWarning = useScreenshotSnapChromeWarning()
  const [testingOOB, setTestingOOB] = useState(false)
  const [oobTestResult, setOobTestResult] = useState<{ success: boolean; message: string } | null>(null)
  const showOOBConfig = webscanOption !== 1 && webscanOption !== 2

  useEffect(() => {
    if (disableMaxHostErr && !skipNucleiWithoutTags) {
      setSkipNucleiWithoutTags(true)
    }
  }, [disableMaxHostErr, skipNucleiWithoutTags, setSkipNucleiWithoutTags])

  useEffect(() => {
    if (webscanOption === 4 && skipNucleiWithWAF) {
      setSkipNucleiWithWAF(false)
    }
  }, [webscanOption, skipNucleiWithWAF, setSkipNucleiWithWAF])

  const visibleOobTestResult = showOOBConfig ? oobTestResult : null

  const handleTestOOB = async () => {
    const targetURL = interactshURL.trim()
    if (!targetURL) {
      setOobTestResult({ success: false, message: t.vulnScan.newScan.oobUrlRequired })
      return
    }

    setTestingOOB(true)
    setOobTestResult(null)
    try {
      const resp = await fetchWithAuth('/api/v1/webscan/oob/test', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          interactshURL: targetURL,
          interactshToken: interactshToken.trim(),
        }),
      })

      const data = await resp.json().catch(() => ({}))
      if (!resp.ok || data?.success === false) {
        setOobTestResult({
          success: false,
          message: data?.error || data?.message || t.vulnScan.newScan.oobTestFailedHttp.replace('{status}', String(resp.status)),
        })
        return
      }

      setOobTestResult({
        success: true,
        message: data?.message || t.vulnScan.newScan.oobTestSuccess,
      })
    } catch (error: any) {
      setOobTestResult({
        success: false,
        message: error?.message || t.vulnScan.newScan.oobTestRequestFailed,
      })
    } finally {
      setTestingOOB(false)
    }
  }

  return (
    <div className="space-y-6 pt-2">
      {/* 模板选择 */}
      {scanTemplates.length > 0 && (
        <div className="space-y-2">
          <Label>{t.vulnScan.newScan.applyTemplate}</Label>
          <div className="flex min-h-11 items-center gap-2 md:min-h-0">
            <Select
              value={selectedTemplateId}
              onValueChange={(value) => {
                setSelectedTemplateId(value)
                if (value) {
                  onApplyTemplate(value)
                }
              }}
            >
              <SelectTrigger className="flex-1">
                <SelectValue placeholder={t.vulnScan.newScan.selectTemplate} />
              </SelectTrigger>
              <SelectContent>
                {scanTemplates.map((template) => (
                  <SelectItem key={template.id} value={template.id}>
                    {template.name}
                    {template.description && (
                      <span className="text-muted-foreground ml-2">
                        - {template.description}
                      </span>
                    )}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {selectedTemplateId && (
              <Button
                variant="outline"
                size="sm"
                onClick={onClearTemplate}
              >
                {t.vulnScan.newScan.clear}
              </Button>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            {t.vulnScan.newScan.templateTip}
          </p>
        </div>
      )}

      {/* 模式选择 */}
      <div className="space-y-2">
        <div className="flex min-h-11 items-center gap-2 md:min-h-0">
          <Label>{t.vulnScan.newScan.mode}</Label>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <HelpCircle className="w-4 h-4 text-muted-foreground cursor-help" />
              </TooltipTrigger>
              <TooltipContent className="max-w-md">
                <div className="space-y-1 text-sm">
                  <p>{t.vulnScan.newScan.modeHelp.fingerprint}</p>
                  <p>{t.vulnScan.newScan.modeHelp.fullFingerprint}</p>
                  <p>{t.vulnScan.newScan.modeHelp.fingerprintVuln}</p>
                  <p>{t.vulnScan.newScan.modeHelp.specialScan}</p>
                </div>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
        <Tabs
          value={String(webscanOption)}
          onValueChange={(value) => setWebscanOption(Number(value) as 1 | 2 | 3 | 4)}
          className="w-full"
        >
          <TabsList className="grid h-auto w-full grid-cols-2 md:flex">
            <TabsTrigger value="1" className="flex min-h-11 items-center gap-2 md:min-h-0">
              <Fingerprint className="w-4 h-4" />
              {t.vulnScan.newScan.modes.fingerprint}
            </TabsTrigger>
            <TabsTrigger value="2" className="flex min-h-11 items-center gap-2 md:min-h-0">
              <Scan className="w-4 h-4" />
              {t.vulnScan.newScan.modes.fullFingerprint}
            </TabsTrigger>
            <TabsTrigger value="3" className="flex min-h-11 items-center gap-2 md:min-h-0">
              <Shield className="w-4 h-4" />
              {t.vulnScan.newScan.modes.fingerprintVuln}
            </TabsTrigger>
            <TabsTrigger value="4" className="flex min-h-11 items-center gap-2 md:min-h-0">
              <Target className="w-4 h-4" />
              {t.vulnScan.newScan.modes.specialScan}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {/* 请求头 */}
      <div className="space-y-2">
        <Label htmlFor="customHeaders">{t.vulnScan.newScan.customHeaders}</Label>
        <Textarea
          id="customHeaders"
          placeholder={t.vulnScan.newScan.customHeadersPlaceholder}
          value={customHeaders}
          onChange={(e) => setCustomHeaders(e.target.value)}
          className="font-mono text-sm min-h-[80px]"
        />
      </div>

      {showOOBConfig && (
        <div className="space-y-3 rounded-lg border p-4 bg-muted/20">
          {/* OOB 配置 */}
          <div className="flex items-center justify-between">
            <Label htmlFor="oobEnabled">{t.vulnScan.newScan.oobEnabled}</Label>
            <Switch
              id="oobEnabled"
              checked={oobEnabled}
              onCheckedChange={setOobEnabled}
            />
          </div>
          <p className="text-xs text-muted-foreground">{t.vulnScan.newScan.oobDesc}</p>

          <div className="space-y-2">
            <Label htmlFor="interactshURL">{t.vulnScan.newScan.interactshURL}</Label>
            <div className="flex min-h-11 items-center gap-2 md:min-h-0">
              <Input
                id="interactshURL"
                placeholder="interact.sh"
                value={interactshURL}
                onChange={(e) => {
                  setInteractshURL(e.target.value)
                  setOobTestResult(null)
                }}
                disabled={!oobEnabled}
              />
              <Button
                type="button"
                variant="outline"
                onClick={handleTestOOB}
                disabled={!oobEnabled || !interactshURL.trim() || testingOOB}
                className="shrink-0"
              >
                {testingOOB ? (
                  <>
                    <Loader2 className="w-4 h-4 mr-1 animate-spin" />
                    {t.vulnScan.newScan.testing}
                  </>
                ) : (
                  t.vulnScan.newScan.testConnection
                )}
              </Button>
            </div>
            {visibleOobTestResult && (
              <p className={`text-xs ${visibleOobTestResult.success ? 'text-green-600' : 'text-destructive'}`}>
                {visibleOobTestResult.message}
              </p>
            )}
          </div>

          <div className="space-y-2">
            <Label htmlFor="interactshToken">{t.vulnScan.newScan.interactshTokenOptional}</Label>
            <Input
              id="interactshToken"
              placeholder={t.vulnScan.newScan.interactshTokenPlaceholder}
              value={interactshToken}
              onChange={(e) => setInteractshToken(e.target.value)}
              disabled={!oobEnabled}
            />
          </div>
        </div>
      )}

      {/* 指定漏洞（专项扫描时显示） */}
      {webscanOption === 4 && (
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t.vulnScan.newScan.specifyFingerprint}</Label>
            <SelectTag
              options={fingerprintOptions}
              value={customTags}
              onChange={setCustomTags}
              placeholder={t.vulnScan.newScan.selectFingerprint}
              searchPlaceholder={t.vulnScan.newScan.searchFingerprint}
              emptyText={t.vulnScan.newScan.noFingerprint}
            />
          </div>

          <div className="space-y-2">
            <Label>{t.vulnScan.newScan.specifyVuln}</Label>
            <SelectTag
              options={templateOptions}
              value={customTemplate}
              onChange={setCustomTemplate}
              placeholder={t.vulnScan.newScan.selectVulnTemplate}
              searchPlaceholder={t.vulnScan.newScan.searchTemplate}
              emptyText={t.vulnScan.newScan.noTemplate}
            />
          </div>
        </div>
      )}

      {/* 配置项 */}
      <div className="space-y-4 pt-4 border-t">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="threads">{t.vulnScan.newScan.threads}</Label>
            <Input
              id="threads"
              type="number"
              min="1"
              max="100"
              value={threads}
              onChange={(e) => {
                const value = parseInt(e.target.value, 10)
                if (!isNaN(value) && value >= 1 && value <= 100) {
                  setThreads(value)
                } else if (e.target.value === '') {
                  setThreads(30)
                }
              }}
              placeholder="1-100"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="nucleiThreads">{t.vulnScan.newScan.nucleiThreads}</Label>
            <Input
              id="nucleiThreads"
              type="number"
              min="1"
              max="100"
              value={nucleiThreads}
              onChange={(e) => {
                const value = parseInt(e.target.value, 10)
                if (!isNaN(value) && value >= 1 && value <= 100) {
                  setNucleiThreads(value)
                } else if (e.target.value === '') {
                  setNucleiThreads(2)
                }
              }}
              placeholder="1-100"
            />
          </div>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="activeTimeoutLimit">{t.vulnScan.newScan.activeTimeoutLimit}</Label>
            <Input
              id="activeTimeoutLimit"
              type="number"
              min="0"
              value={activeTimeoutLimit}
              onChange={(e) => {
                const value = parseInt(e.target.value, 10)
                if (!isNaN(value)) {
                  setActiveTimeoutLimit(value)
                } else if (e.target.value === '') {
                  setActiveTimeoutLimit(5)
                }
              }}
              placeholder="1-max"
            />
          </div>
          <div />
        </div>

        <div className="space-y-3">
          <div className="flex items-center space-x-2">
            <Checkbox
              id="disableMaxHostErr"
              checked={disableMaxHostErr}
              onCheckedChange={(checked) => {
                const nextValue = checked === true
                setDisableMaxHostErr(nextValue)
                if (nextValue) {
                  setSkipNucleiWithoutTags(true)
                }
              }}
            />
            <Label
              htmlFor="disableMaxHostErr"
              className="text-sm font-normal cursor-pointer"
            >
              {t.vulnScan.newScan.disableMaxHostErr}
            </Label>
          </div>

          <div className="flex items-center space-x-2">
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <div className="flex items-center space-x-2">
                    <Checkbox
                      id="rootPathScan"
                      checked={rootPathScan}
                      onCheckedChange={(checked) =>
                        setRootPathScan(checked === true)
                      }
                    />
                    <Label
                      htmlFor="rootPathScan"
                      className="text-sm font-normal cursor-pointer flex items-center gap-1"
                    >
                      {t.vulnScan.newScan.rootPathScan}
                      <HelpCircle className="w-4 h-4 text-muted-foreground" />
                    </Label>
                  </div>
                </TooltipTrigger>
                <TooltipContent>
                  <p>{t.vulnScan.newScan.rootPathScanTip}</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>

          <div className="flex items-center space-x-2">
            <Checkbox
              id="skipNucleiWithoutTags"
              checked={skipNucleiWithoutTags}
              disabled={disableMaxHostErr}
              onCheckedChange={(checked) =>
                setSkipNucleiWithoutTags(checked === true)
              }
            />
            <Label
              htmlFor="skipNucleiWithoutTags"
              className={`text-sm font-normal ${disableMaxHostErr ? 'cursor-not-allowed opacity-70' : 'cursor-pointer'}`}
            >
              {t.vulnScan.newScan.skipNucleiWithoutTags}
            </Label>
          </div>

          <div className="flex items-center space-x-2">
            <Checkbox
              id="skipNucleiWithWAF"
              checked={skipNucleiWithWAF}
              onCheckedChange={(checked) =>
                setSkipNucleiWithWAF(checked === true)
              }
            />
            <Label
              htmlFor="skipNucleiWithWAF"
              className="text-sm font-normal cursor-pointer"
            >
              {t.vulnScan.newScan.skipNucleiWithWAF}
            </Label>
          </div>

          <div className="flex items-center space-x-2">
            <Checkbox
              id="screenshot"
              checked={screenshot}
              onCheckedChange={(checked) =>
                setScreenshot(checked === true)
              }
            />
            <Label
              htmlFor="screenshot"
              className="text-sm font-normal cursor-pointer"
            >
              {t.vulnScan.newScan.screenshot}
            </Label>
          </div>
          {screenshot && showScreenshotSnapChromeWarning && (
            <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-200">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <p>{t.vulnScan.newScan.screenshotSnapChromeWarning}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
