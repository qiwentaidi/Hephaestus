'use client'

import React, { ChangeEvent, useRef, useState } from 'react'
import { Label } from '@/components/ui/label'
import { Card } from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
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
import {
	Collapsible,
	CollapsibleContent,
	CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { FormItem } from '@/components/ui/form'
import { Globe, ArrowRightLeft, FileSearch, Upload, RefreshCcw, HelpCircle, ChevronDown, X } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useI18n } from '@/lib/i18n'

export type DictionaryMeta = {
	id: string
	name: string
	path: string
	description: string
	size?: number
	entries?: string[]
}

interface DirScanSettingsProps {
  method: string
  setMethod: (value: string) => void
  extensions: string
  setExtensions: (value: string) => void
  statusFilter: string
  setStatusFilter: (value: string) => void
  customDict: string
  setCustomDict: (value: string) => void
  availableDicts: DictionaryMeta[]
  setAvailableDicts: (dicts: DictionaryMeta[]) => void
  selectedDicts: string[]
  setSelectedDicts: (selected: string[]) => void
  loadingDicts: boolean
  advancedConfig: {
    thread: number
    timeout: number
    interval: number
    recursion: number
    times: number
    exclude: string
    headers: string
    redirectClient: boolean
    backupfileScan: boolean
  }
  setAdvancedConfig: React.Dispatch<React.SetStateAction<{
    thread: number
    timeout: number
    interval: number
    recursion: number
    times: number
    exclude: string
    headers: string
    redirectClient: boolean
    backupfileScan: boolean
  }>>
  defaultDictionaries: DictionaryMeta[]
  onDictionaryUpload: (event: ChangeEvent<HTMLInputElement>) => Promise<void>
  onReloadDicts: () => Promise<void>
  onToast: (message: string) => void
  // 模板相关
  scanTemplates?: any[]
  selectedTemplateId?: string
  setSelectedTemplateId?: (val: string) => void
  onApplyTemplate?: (id: string) => void
  onClearTemplate?: () => void
}

export function DirScanSettings({
  method,
  setMethod,
  extensions,
  setExtensions,
  statusFilter,
  setStatusFilter,
  customDict,
  setCustomDict,
  availableDicts,
  setAvailableDicts,
  selectedDicts,
  setSelectedDicts,
  loadingDicts,
  advancedConfig,
  setAdvancedConfig,
  defaultDictionaries,
  onDictionaryUpload,
  onReloadDicts,
  onToast,
  scanTemplates = [],
  selectedTemplateId = '',
  setSelectedTemplateId,
  onApplyTemplate,
  onClearTemplate,
}: DirScanSettingsProps) {
  const { t } = useI18n()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [isAdvancedOpen, setIsAdvancedOpen] = useState(false)
  const [expandedDictIds, setExpandedDictIds] = useState<string[]>([])

  const toggleDictSelection = (dictId: string, checked: boolean) => {
    if (checked) {
      if (selectedDicts.includes(dictId)) {
        return
      }
      setSelectedDicts([...selectedDicts, dictId])
      return
    }
    setSelectedDicts(selectedDicts.filter((id) => id !== dictId))
  }

  const toggleDictExpand = (dictId: string) => {
    setExpandedDictIds((prev) =>
      prev.includes(dictId) ? prev.filter((id) => id !== dictId) : [...prev, dictId]
    )
  }

  return (
    <TooltipProvider delayDuration={200}>
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

        {/* 基本参数 */}
        <div className="space-y-3">
          <h4 className="text-sm font-semibold text-foreground mb-3">{t.vulnScan.newScan.dirScan.basicParams}</h4>
          <div className="space-y-4">
            {/* HTTP 方法 */}
            <Card className="p-4 bg-muted/30">
              <div className="flex items-start gap-3">
                <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-info/10 text-info flex-shrink-0">
                  <Globe className="w-5 h-5" />
                </div>
                <div className="flex-1">
                  <Label className="text-sm font-medium block">{t.vulnScan.newScan.dirScan.httpMethod}</Label>
                  <p className="text-xs text-muted-foreground mt-0.5">
                    {t.vulnScan.newScan.dirScan.httpMethodHelp}
                  </p>
                </div>
                <div className="flex-shrink-0">
                  <Select value={method} onValueChange={setMethod}>
                    <SelectTrigger className="w-[120px]">
                      <SelectValue placeholder={t.vulnScan.newScan.dirScan.selectMethod} />
                    </SelectTrigger>
                    <SelectContent>
                      {['GET', 'POST', 'HEAD', 'OPTIONS'].map((item) => (
                        <SelectItem value={item} key={item}>
                          {item}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </Card>

            {/* 记录 30x 跳转 */}
            <Card className="p-4 bg-muted/30">
              <div className="flex items-start gap-3">
                <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-orange-500/10 text-orange-500 flex-shrink-0">
                  <ArrowRightLeft className="w-5 h-5" />
                </div>
                <div className="flex-1 space-y-2">
                  <div className="flex items-center justify-between">
                    <div>
                      <Label className="text-sm font-medium cursor-pointer">{t.vulnScan.newScan.dirScan.recordRedirect}</Label>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        {t.vulnScan.newScan.dirScan.recordRedirectHelp}
                      </p>
                    </div>
                    <Switch
                      checked={advancedConfig.redirectClient}
                      onCheckedChange={(checked) =>
                        setAdvancedConfig((prev) => ({ ...prev, redirectClient: checked }))
                      }
                    />
                  </div>
                </div>
              </div>
            </Card>

            {/* 备份文件扫描 */}
            <Card className="p-4 bg-muted/30">
              <div className="flex items-start gap-3">
                <div className="flex items-center justify-center w-10 h-10 rounded-lg bg-success/10 text-success flex-shrink-0">
                  <FileSearch className="w-5 h-5" />
                </div>
                <div className="flex-1 space-y-2">
                  <div className="flex items-center justify-between">
                    <div>
                      <Label className="text-sm font-medium cursor-pointer">{t.vulnScan.newScan.dirScan.backupScan}</Label>
                      <p className="text-xs text-muted-foreground mt-0.5">
                        {t.vulnScan.newScan.dirScan.backupScanHelp}
                      </p>
                    </div>
                    <Switch
                      checked={advancedConfig.backupfileScan}
                      onCheckedChange={(checked) =>
                        setAdvancedConfig((prev) => ({ ...prev, backupfileScan: checked }))
                      }
                    />
                  </div>
                </div>
              </div>
            </Card>
          </div>
        </div>

        {/* 字典管理 */}
        <div className="space-y-3 pt-4 border-t">
          <div className="flex items-center justify-between">
            <Label className="text-sm font-medium">{t.vulnScan.newScan.dirScan.dictionaryList}</Label>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => fileInputRef.current?.click()}
              >
                <Upload className="w-4 h-4 mr-2" />
                {t.vulnScan.newScan.dirScan.importDict}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={async () => {
                  if (onReloadDicts) {
                    // 如果提供了 onReload 回调，重新从后端加载字典列表
                    await onReloadDicts()
                    onToast(t.vulnScan.newScan.dirScan.reloadDictSuccess)
                  } else {
                    // 否则使用默认字典（兼容旧代码）
                    setAvailableDicts(defaultDictionaries)
                    setSelectedDicts(['dicc'])
                    onToast(t.vulnScan.newScan.dirScan.restoreDefaultDictSuccess)
                  }
                }}
                disabled={loadingDicts}
              >
                <RefreshCcw className={`w-4 h-4 mr-2 ${loadingDicts ? 'animate-spin' : ''}`} />
                {loadingDicts ? t.vulnScan.newScan.dirScan.loading : t.vulnScan.newScan.dirScan.reset}
              </Button>
            </div>
          </div>

          <div className="border rounded-lg">
            <ScrollArea className="h-64">
              <div className="divide-y">
                {availableDicts.map((dict) => {
                  const isSelected = selectedDicts.includes(dict.id)
                  const isExpanded = expandedDictIds.includes(dict.id)
                  const description = dict.description?.trim() || t.vulnScan.newScan.messages.noDescription
                  const sourceLines = (dict.entries && dict.entries.length > 0
                    ? dict.entries
                    : description.split(/\r?\n/g)
                  )
                    .map((line) => line.trim())
                    .filter(Boolean)
                  const previewLines = sourceLines.slice(0, 3)
                  const extraLines = sourceLines.slice(3)
                  const showExpand = extraLines.length > 0
                  return (
                    <div
                      key={dict.id}
                      className={cn(
                        'flex items-start gap-3 p-4 transition-colors cursor-pointer',
                        isSelected ? 'bg-muted/40' : 'hover:bg-muted/50'
                      )}
                      onClick={() => toggleDictSelection(dict.id, !isSelected)}
                    >
                      <Checkbox
                        checked={isSelected}
                        onCheckedChange={(checked) => toggleDictSelection(dict.id, checked === true)}
                        onClick={(event) => event.stopPropagation()}
                        className="mt-1"
                      />
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center justify-between gap-3">
                          <p className="font-medium text-sm truncate">{dict.name}</p>
                          <Badge variant="secondary" className="text-xs shrink-0">
                            {dict.size
                              ? `${(dict.size / 1024).toFixed(1)} KB`
                              : dict.entries
                                ? t.vulnScan.newScan.dirScan.entryCount.replace('{count}', String(dict.entries.length))
                                : ''}
                          </Badge>
                        </div>
                        <div className="mt-1 space-y-1">
                          {previewLines.map((line, index) => (
                            <p
                              key={`${dict.id}-preview-${index}`}
                              className="text-xs text-muted-foreground break-words font-mono"
                            >
                              {line}
                            </p>
                          ))}
                        </div>
                        <div className="mt-2 flex items-center justify-between gap-2">
                          <code className="text-[11px] text-muted-foreground/90 font-mono truncate">
                            {dict.path}
                          </code>
                          {showExpand && (
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              className="h-6 px-2 text-xs"
                              onClick={(event) => {
                                event.stopPropagation()
                                toggleDictExpand(dict.id)
                              }}
                            >
                              {isExpanded ? t.vulnScan.newScan.dirScan.collapse : t.vulnScan.newScan.dirScan.expand}
                            </Button>
                          )}
                        </div>
                        {isExpanded && extraLines.length > 0 && (
                          <div className="mt-2 space-y-1">
                            {extraLines.map((line, index) => (
                              <p
                                key={`${dict.id}-extra-${index}`}
                                className="text-xs text-muted-foreground/90 break-words font-mono"
                              >
                                {line}
                              </p>
                            ))}
                          </div>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            </ScrollArea>
          </div>
          <input
            ref={fileInputRef}
            type="file"
            accept=".txt"
            multiple
            className="hidden"
            onChange={onDictionaryUpload}
          />

          <div className="space-y-2">
            <Label className="text-sm font-medium">{t.vulnScan.newScan.dirScan.customDict}</Label>
            <Textarea
              value={customDict}
              onChange={(e) => setCustomDict(e.target.value)}
              className="font-mono text-sm min-h-[200px] resize-none"
            />
            <p className="text-xs text-muted-foreground">
              {t.vulnScan.newScan.dirScan.customDictHelp}
            </p>
          </div>
        </div>

        {/* 高级参数 */}
        <div className="space-y-3 pt-4 border-t">
          <Collapsible open={isAdvancedOpen} onOpenChange={setIsAdvancedOpen}>
            <CollapsibleTrigger className="flex items-center gap-2 w-full">
              <h4 className="text-sm font-semibold">{t.vulnScan.newScan.dirScan.advancedParams}</h4>
              <ChevronDown
                className={cn(
                  'w-4 h-4 text-muted-foreground transition-transform',
                  isAdvancedOpen && 'transform rotate-180'
                )}
              />
            </CollapsibleTrigger>
            <CollapsibleContent className="space-y-4 mt-4">
              {/* 扩展名 (%EXT%) */}
              <FormItem>
                <div className="flex items-center gap-2">
                  <Label>{t.vulnScan.newScan.dirScan.extensions}</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="w-4 h-4 text-muted-foreground cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t.vulnScan.newScan.dirScan.extensionsHelp}</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  value={extensions}
                  onChange={(e) => setExtensions(e.target.value)}
                  placeholder="php,aspx,asp,jsp"
                />
              </FormItem>

              {/* 排除状态码 */}
              <FormItem>
                <div className="flex items-center gap-2">
                  <Label>{t.vulnScan.newScan.dirScan.excludeStatusCode}</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="w-4 h-4 text-muted-foreground cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t.vulnScan.newScan.dirScan.excludeStatusCodeHelp}</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  placeholder={t.vulnScan.newScan.dirScan.excludeStatusCodePlaceholder}
                />
              </FormItem>

              {/* 线程数 */}
              <FormItem>
                <Label>{t.vulnScan.newScan.dirScan.threadCount}</Label>
                <Input
                  type="number"
                  min={1}
                  max={200}
                  value={advancedConfig.thread}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, thread: Number(e.target.value) }))
                  }
                />
              </FormItem>

              {/* 超时时长 */}
              <FormItem>
                <Label>{t.vulnScan.newScan.dirScan.timeoutSeconds}</Label>
                <Input
                  type="number"
                  min={1}
                  max={60}
                  value={advancedConfig.timeout}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, timeout: Number(e.target.value) }))
                  }
                />
              </FormItem>

              {/* 请求间隔 */}
              <FormItem>
                <Label>{t.vulnScan.newScan.dirScan.requestInterval}</Label>
                <Input
                  type="number"
                  min={0}
                  max={60}
                  value={advancedConfig.interval}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, interval: Number(e.target.value) }))
                  }
                />
              </FormItem>

              {/* 递归层级 */}
              <FormItem>
                <Label>{t.vulnScan.newScan.dirScan.recursionDepth}</Label>
                <Input
                  type="number"
                  min={0}
                  max={5}
                  value={advancedConfig.recursion}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, recursion: Number(e.target.value) }))
                  }
                />
              </FormItem>

              {/* 过滤长度次数 */}
              <FormItem>
                <div className="flex items-center gap-2">
                  <Label>{t.vulnScan.newScan.dirScan.filterLengthTimes}</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="w-4 h-4 text-muted-foreground cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t.vulnScan.newScan.dirScan.filterLengthTimesHelp}</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  type="number"
                  min={0}
                  max={10000}
                  value={advancedConfig.times}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, times: Number(e.target.value) }))
                  }
                />
              </FormItem>

              {/* 过滤响应体内容 */}
              <FormItem>
                <div className="flex items-center gap-2">
                  <Label>{t.vulnScan.newScan.dirScan.filterBodyContent}</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="w-4 h-4 text-muted-foreground cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t.vulnScan.newScan.dirScan.filterBodyContentHelp}</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  value={advancedConfig.exclude}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, exclude: e.target.value }))
                  }
                  placeholder={t.vulnScan.newScan.dirScan.filterBodyContentPlaceholder}
                />
              </FormItem>

              {/* 自定义请求头 */}
              <FormItem>
                <div className="flex items-center gap-2">
                  <Label>{t.vulnScan.newScan.dirScan.customHeaders}</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <HelpCircle className="w-4 h-4 text-muted-foreground cursor-help" />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t.vulnScan.newScan.dirScan.customHeadersHelp}</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Textarea
                  className="min-h-[110px]"
                  placeholder={`User-Agent: dirsearch/0.4.2\nCookie: session=xxx\nAuthorization: Bearer token`}
                  value={advancedConfig.headers}
                  onChange={(e) =>
                    setAdvancedConfig((prev) => ({ ...prev, headers: e.target.value }))
                  }
                />
              </FormItem>
            </CollapsibleContent>
          </Collapsible>
        </div>
      </div>
    </TooltipProvider>
  )
}
