'use client'

import React, { useState, KeyboardEvent } from 'react'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Separator } from '@/components/ui/separator'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { ChevronDown, ChevronRight, Settings, Database, FileCode, Globe, Link, Zap, Upload as UploadIcon, Info, Plus, Trash2, X, Sparkles } from 'lucide-react'
import { useI18n } from '@/lib/i18n'
import { cn } from '@/lib/utils'

// 类型定义
export interface SQLRule {
  payloads: string[]
  type: 'error-based'
  bodyContains: string[]
}

export function normalizeSQLRules(rules: unknown): SQLRule[] {
  if (!Array.isArray(rules)) {
    return []
  }

  return rules.map((rule) => {
    const current = rule && typeof rule === 'object' ? rule as Partial<SQLRule> & { payloads?: unknown; bodyContains?: unknown } : {}
    return {
      payloads: Array.isArray(current.payloads) ? current.payloads.filter((item): item is string => typeof item === 'string') : [],
      type: 'error-based',
      bodyContains: Array.isArray(current.bodyContains) ? current.bodyContains.filter((item): item is string => typeof item === 'string') : [],
    }
  })
}

export interface LFIRule {
  payloads: string[]
  matchType: 'regex' | 'word'
  regex?: string[]
  words?: string[]
  condition?: 'and' | 'or'
}

export interface XSSRule {
  payloads: string[]
  type: string
  words?: string[]
  condition?: string
}

// 标签输入组件
function TagInput({
  value,
  onChange,
  placeholder,
}: {
  value: string[]
  onChange: (value: string[]) => void
  placeholder?: string
}) {
  const [inputValue, setInputValue] = useState('')

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && inputValue.trim()) {
      e.preventDefault()
      if (!value.includes(inputValue.trim())) {
        onChange([...value, inputValue.trim()])
      }
      setInputValue('')
    }
  }

  const removeTag = (tagToRemove: string) => {
    onChange(value.filter((tag) => tag !== tagToRemove))
  }

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        {value.map((tag) => (
          <Badge key={tag} variant="secondary" className="pl-2 pr-1">
            {tag}
            <button
              type="button"
              onClick={() => removeTag(tag)}
              className="ml-1 hover:bg-muted rounded-full p-0.5"
            >
              <X className="h-3 w-3" />
            </button>
          </Badge>
        ))}
      </div>
      <Input
        value={inputValue}
        onChange={(e) => setInputValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        className="text-sm"
      />
    </div>
  )
}

const vulnSectionCardClassName = cn(
  'group overflow-hidden rounded-[1.1rem] border-border/60 bg-card/75 py-0 gap-0',
  'shadow-[0_8px_24px_rgba(0,0,0,0.14)] transition-[border-color,background-color,box-shadow] duration-200',
  'hover:border-border hover:bg-card/90 hover:shadow-[0_12px_30px_rgba(0,0,0,0.18)]',
)

const vulnSectionHeaderClassName = cn(
  'cursor-pointer rounded-none px-6 py-4 transition-colors duration-200',
  'group-hover:bg-muted/20',
)

const vulnSectionContentClassName = cn(
  'space-y-4 border-t border-border/50 bg-background/35 px-6 py-5',
)

const vulnSectionIconWrapClassName =
  'flex h-12 w-12 items-center justify-center rounded-xl border border-white/5 shadow-inner'

const vulnSectionTitleClassName = 'text-[1.05rem] font-semibold tracking-tight'

const vulnSectionDescriptionClassName = 'mt-1 text-[13px] leading-none text-muted-foreground/90'

interface JsScanSettingsProps {
  blackDomain: string[]
  setBlackDomain: (value: string[]) => void
  highRiskRouter: string[]
  setHighRiskRouter: (value: string[]) => void
  authentication: string[]
  setAuthentication: (value: string[]) => void
  placeholder: Record<string, string>
  setPlaceholder: (value: Record<string, string>) => void
  sqlInjectionEnabled: boolean
  setSqlInjectionEnabled: (value: boolean) => void
  sqlInjectionRules: SQLRule[]
  setSqlInjectionRules: (value: SQLRule[]) => void
  lfiEnabled: boolean
  setLfiEnabled: (value: boolean) => void
  lfiParamKeywords: string[]
  setLfiParamKeywords: (value: string[]) => void
  lfiRules: LFIRule[]
  setLfiRules: (value: LFIRule[]) => void
  ssrfEnabled: boolean
  setSsrfEnabled: (value: boolean) => void
  ssrfParamKeywords: string[]
  setSsrfParamKeywords: (value: string[]) => void
  redirectEnabled: boolean
  setRedirectEnabled: (value: boolean) => void
  redirectParamKeywords: string[]
  setRedirectParamKeywords: (value: string[]) => void
  xssEnabled: boolean
  setXssEnabled: (value: boolean) => void
  xssRules: XSSRule[]
  setXssRules: (value: XSSRule[]) => void
  uploadEnabled: boolean
  setUploadEnabled: (value: boolean) => void
  uploadParamKeywords: string[]
  setUploadParamKeywords: (value: string[]) => void
  uploadTestContent: string
  setUploadTestContent: (value: string) => void
  uploadTestFileName: string
  setUploadTestFileName: (value: string) => void
  aiDetectionEnabled: boolean
  setAiDetectionEnabled: (value: boolean) => void
  aiDetectionAvailable?: boolean
  // 模板相关
  scanTemplates?: any[]
  selectedTemplateId?: string
  setSelectedTemplateId?: (val: string) => void
  onApplyTemplate?: (id: string) => void
  onClearTemplate?: () => void
}

export function JsScanSettings({
  blackDomain,
  setBlackDomain,
  highRiskRouter,
  setHighRiskRouter,
  authentication,
  setAuthentication,
  placeholder,
  setPlaceholder,
  sqlInjectionEnabled,
  setSqlInjectionEnabled,
  sqlInjectionRules,
  setSqlInjectionRules,
  lfiEnabled,
  setLfiEnabled,
  lfiParamKeywords,
  setLfiParamKeywords,
  lfiRules,
  setLfiRules,
  ssrfEnabled,
  setSsrfEnabled,
  ssrfParamKeywords,
  setSsrfParamKeywords,
  redirectEnabled,
  setRedirectEnabled,
  redirectParamKeywords,
  setRedirectParamKeywords,
  xssEnabled,
  setXssEnabled,
  xssRules,
  setXssRules,
  uploadEnabled,
  setUploadEnabled,
  uploadParamKeywords,
  setUploadParamKeywords,
  uploadTestContent,
  setUploadTestContent,
  uploadTestFileName,
  setUploadTestFileName,
  aiDetectionEnabled,
  setAiDetectionEnabled,
  aiDetectionAvailable = true,
  scanTemplates = [],
  selectedTemplateId = '',
  setSelectedTemplateId,
  onApplyTemplate,
  onClearTemplate,
}: JsScanSettingsProps) {
  const { t } = useI18n()
  const js = t.vulnScan.newScan.jsScan
  const [openSections, setOpenSections] = useState({
    basic: true,
    vulnDetection: false,
    sqli: false,
    lfi: false,
    ssrf: false,
    redirect: false,
    xss: false,
    upload: false,
  })

  const toggleSection = (section: keyof typeof openSections) => {
    setOpenSections({ ...openSections, [section]: !openSections[section] })
  }

  return (
    <div className="space-y-6">
      {/* 模板选择 */}
      {scanTemplates.length > 0 && (
        <div className="space-y-2">
          <Label>{t.vulnScan.newScan.applyTemplate}</Label>
          <div className="flex items-center gap-2">
            <Select
              value={selectedTemplateId}
              onValueChange={(value) => {
                setSelectedTemplateId?.(value)
                if (value && onApplyTemplate) {
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
                type="button"
                variant="outline"
                size="icon"
                onClick={onClearTemplate}
                title={t.vulnScan.newScan.clearTemplate}
              >
                <X className="h-4 w-4" />
              </Button>
            )}
          </div>
        </div>
      )}

      {/* AI 辅助检测 */}
      <div className="flex items-center justify-between p-4 border rounded-lg bg-gradient-to-r from-purple-500/5 to-blue-500/5">
        <div className="flex items-center gap-3">
          <div className="p-2 rounded-lg bg-gradient-to-r from-purple-500/10 to-blue-500/10">
            <Sparkles className="h-5 w-5 text-purple-500" />
          </div>
          <div className="space-y-0.5">
            <Label className="text-sm font-medium">{js.aiDetection}</Label>
            <p className="text-xs text-muted-foreground">
              {js.aiDetectionDesc}
            </p>
          </div>
        </div>
        <Switch
          checked={aiDetectionAvailable && aiDetectionEnabled}
          disabled={!aiDetectionAvailable}
          onCheckedChange={setAiDetectionEnabled}
        />
      </div>

      {/* 基本配置 */}
      <Collapsible open={openSections.basic} onOpenChange={() => toggleSection('basic')}>
        <CollapsibleTrigger asChild>
          <div className="flex items-center justify-between py-3 cursor-pointer hover:bg-muted/30 rounded-lg px-2 -mx-2">
            <div className="flex items-center gap-2">
              <Settings className="h-4 w-4" />
              <span className="font-semibold">{js.basicConfig}</span>
            </div>
            {openSections.basic ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
          </div>
        </CollapsibleTrigger>
        <CollapsibleContent className="space-y-4 pt-4">
          {/* 黑名单域名 */}
          <div className="space-y-2">
            <Label>{js.blackDomain}</Label>
            <Textarea
              value={blackDomain.join('\n')}
              onChange={(e) => setBlackDomain(e.target.value.split('\n').filter((s) => s.trim()))}
              placeholder={js.blackDomainPlaceholder}
              rows={4}
              className="font-mono text-sm"
            />
          </div>

          {/* 高风险路由关键词 */}
          <div className="space-y-2">
            <Label>{js.highRiskRouter}</Label>
            <Textarea
              value={highRiskRouter.join('\n')}
              onChange={(e) => setHighRiskRouter(e.target.value.split('\n').filter((s) => s.trim()))}
              placeholder={js.highRiskRouterPlaceholder}
              rows={4}
              className="font-mono text-sm"
            />
          </div>

          {/* 认证失败关键词 */}
          <div className="space-y-2">
            <Label>{js.authentication}</Label>
            <Textarea
              value={authentication.join('\n')}
              onChange={(e) => setAuthentication(e.target.value.split('\n').filter((s) => s.trim()))}
              placeholder={js.authenticationPlaceholder}
              rows={4}
              className="font-mono text-sm"
            />
          </div>

          {/* 占位符配置 */}
          <div className="space-y-2">
            <Label>{js.placeholderReplace}</Label>
            <Alert>
              <Info className="h-4 w-4" />
              <AlertDescription className="text-xs">
                {js.placeholderReplaceDesc}
              </AlertDescription>
            </Alert>
            <div className="space-y-2 mt-2">
              {Object.entries(placeholder || {}).map(([key, value]) => (
                <div key={key} className="flex items-center gap-2">
                  <Input
                    value={key}
                    onChange={(e) => {
                      const newPlaceholder = { ...placeholder }
                      delete newPlaceholder[key]
                      newPlaceholder[e.target.value] = value
                      setPlaceholder(newPlaceholder)
                    }}
                    placeholder={js.keyName}
                    className="text-xs font-mono w-28"
                  />
                  <span className="text-muted-foreground">=</span>
                  <Input
                    value={value}
                    onChange={(e) => setPlaceholder({ ...placeholder, [key]: e.target.value })}
                    placeholder={js.value}
                    className="text-xs font-mono flex-1"
                  />
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-8 w-8 text-destructive hover:text-destructive"
                    onClick={() => {
                      const newPlaceholder = { ...placeholder }
                      delete newPlaceholder[key]
                      setPlaceholder(newPlaceholder)
                    }}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  const newKey = `placeholder_${Object.keys(placeholder || {}).length + 1}`
                  setPlaceholder({ ...placeholder, [newKey]: '' })
                }}
                className="w-full"
              >
                <Plus className="h-4 w-4 mr-2" />
                {js.addPlaceholder}
              </Button>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>

      <Separator />

      <div className="flex items-center justify-between rounded-lg border border-border/60 bg-muted/20 px-4 py-3">
        <div className="space-y-1">
          <Label htmlFor="jsscan-vuln-detection" className="font-medium">{js.vulnDetectionConfig}</Label>
        </div>
        <Switch
          id="jsscan-vuln-detection"
          checked={sqlInjectionEnabled}
          onCheckedChange={setSqlInjectionEnabled}
        />
      </div>

    </div>
  )
}
