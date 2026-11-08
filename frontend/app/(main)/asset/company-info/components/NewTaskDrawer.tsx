'use client'

import Image from 'next/image'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ExternalLink, Info } from 'lucide-react'
import { FormData } from '../types'
import { useI18n } from '@/lib/i18n'

interface NewTaskDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  formData: FormData
  onFormDataChange: (data: FormData) => void
  onCreateTask: () => void
}

export function NewTaskDrawer({
  open,
  onOpenChange,
  formData,
  onFormDataChange,
  onCreateTask,
}: NewTaskDrawerProps) {
  const { t } = useI18n()

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side="right"
        className="w-full sm:w-[600px] sm:max-w-[600px] flex flex-col h-full p-0" // p-0 去掉默认 padding，方便自己布局
      >
        <SheetHeader>
          <SheetTitle>{t.companyInfo.drawer.title}</SheetTitle>
          <SheetDescription>{t.companyInfo.drawer.description}</SheetDescription>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto px-4">
          <div className="space-y-6 max-w-2xl mx-auto">
            <div className="space-y-2">
              <Label htmlFor="taskName">{t.companyInfo.drawer.taskName}</Label>
              <Input
                id="taskName"
                placeholder={t.companyInfo.drawer.taskNamePlaceholder}
                value={formData.taskName}
                onChange={(e) =>
                  onFormDataChange({ ...formData, taskName: e.target.value })
                }
              />
              <p className="text-xs text-muted-foreground">
                {t.companyInfo.drawer.taskNameHint}
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="company">{t.companyInfo.drawer.companyName}</Label>
              <Textarea
                id="company"
                placeholder={t.companyInfo.drawer.companyPlaceholder}
                className="min-h-[130px]"
                value={formData.company}
                onChange={(e) =>
                  onFormDataChange({ ...formData, company: e.target.value })
                }
              />
              <p className="text-xs text-muted-foreground">
                {t.companyInfo.drawer.companyHint}
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="invest">{t.companyInfo.drawer.invest}</Label>
                <Input
                  id="invest"
                  type="number"
                  min={1}
                  max={100}
                  value={formData.invest}
                  onChange={(e) =>
                    onFormDataChange({
                      ...formData,
                      invest: parseInt(e.target.value) || 100,
                    })
                  }
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="subcompanyLevel">{t.companyInfo.drawer.subcompanyLevel}</Label>
                <Input
                  id="subcompanyLevel"
                  type="number"
                  min={1}
                  max={2}
                  value={formData.subcompanyLevel}
                  onChange={(e) =>
                    onFormDataChange({
                      ...formData,
                      subcompanyLevel: parseInt(e.target.value) || 1,
                    })
                  }
                />
              </div>
            </div>

            <div className="space-y-3">
              <Label>{t.companyInfo.drawer.dataSource}</Label>
              <p className="text-sm text-muted-foreground">
                {t.companyInfo.drawer.dataSourceHint}
              </p>
              <div className="space-y-4 rounded-xl border bg-muted/30 p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="flex items-center gap-2">
                    <Image src="/icon/riskbird.ico" alt={t.companyInfo.taskInfo.riskbird} width={18} height={18} unoptimized />
                    <div>
                      <p className="text-sm font-medium">{t.companyInfo.drawer.riskbirdTitle}</p>
                      <p className="text-xs text-muted-foreground">
                        {t.companyInfo.drawer.riskbirdHint}
                      </p>
                    </div>
                  </div>
                  <span className="text-xs px-2 py-0.5 rounded-full bg-green-100 text-green-700">
                    {t.common.recommended}
                  </span>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="riskbirdCookie">{t.companyInfo.drawer.riskbirdCookie}</Label>
                  <Textarea
                    id="riskbirdCookie"
                    placeholder={t.companyInfo.drawer.riskbirdCookiePlaceholder}
                    className="min-h-[110px]"
                    value={formData.riskbirdCookie}
                    onChange={(e) =>
                      onFormDataChange({ ...formData, riskbirdCookie: e.target.value })
                    }
                  />
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      window.open('https://riskbird.com', '_blank')
                    }
                  >
                    <ExternalLink className="w-4 h-4 mr-1" />
                    {t.companyInfo.drawer.visitRiskbird}
                  </Button>
                  <p className="text-xs text-muted-foreground">
                    {t.companyInfo.drawer.riskbirdCookieHelp}
                  </p>
                </div>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="zerozone"
                  checked={formData.zeroZoneEnable || formData.dataSource.includes('zerozone')}
                  onCheckedChange={(checked) => {
                    if (checked) {
                      onFormDataChange({
                        ...formData,
                        zeroZoneEnable: true,
                        dataSource: formData.dataSource.includes('zerozone')
                          ? formData.dataSource
                          : [...formData.dataSource, 'zerozone'],
                      })
                    } else {
                      onFormDataChange({
                        ...formData,
                        zeroZoneEnable: false,
                        dataSource: formData.dataSource.filter((s) => s !== 'zerozone'),
                      })
                    }
                  }}
                />
                <Label htmlFor="zerozone" className="flex items-center gap-2 cursor-pointer">
                  <span className="inline-flex h-4 w-4 items-center justify-center rounded-sm bg-white p-[1px] ring-1 ring-border/60">
                    <Image src="/icon/0zone.png" alt="0.zone" width={14} height={14} unoptimized />
                  </span>
                  {t.companyInfo.drawer.zerozone}
                </Label>
              </div>
            </div>

            {(formData.zeroZoneEnable || formData.dataSource.includes('zerozone')) && (
              <div className="space-y-2">
                <Label htmlFor="zeroZoneKey">{t.companyInfo.drawer.zerozoneKey}</Label>
                <Input
                  id="zeroZoneKey"
                  placeholder={t.companyInfo.drawer.zerozoneKeyPlaceholder}
                  value={formData.zeroZoneKey}
                  onChange={(e) =>
                    onFormDataChange({ ...formData, zeroZoneKey: e.target.value })
                  }
                />
                <p className="text-xs text-muted-foreground">
                  {t.companyInfo.drawer.zerozoneHint}
                </p>
              </div>
            )}

            <div className="space-y-2">
              <div className="flex items-center space-x-2">
                <Checkbox
                  id="miit"
                  checked={formData.miitEnable || formData.dataSource.includes('miit')}
                  onCheckedChange={(checked) => {
                    if (checked) {
                      onFormDataChange({
                        ...formData,
                        miitEnable: true,
                        dataSource: formData.dataSource.includes('miit')
                          ? formData.dataSource
                          : [...formData.dataSource, 'miit'],
                      })
                    } else {
                      onFormDataChange({
                        ...formData,
                        miitEnable: false,
                        dataSource: formData.dataSource.filter((s) => s !== 'miit'),
                      })
                    }
                  }}
                />
                <Label htmlFor="miit" className="flex items-center gap-2 cursor-pointer">
                  <Image src="/icon/icp.ico" alt={t.companyInfo.taskInfo.miit} width={16} height={16} unoptimized />
                  {t.companyInfo.drawer.miit}
                </Label>
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger>
                      <Info className="w-4 h-4 text-muted-foreground" />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{t.companyInfo.drawer.miitHint}</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              </div>
            </div>

            {(formData.miitEnable || formData.dataSource.includes('miit')) && (
              <div className="space-y-2">
                <Label htmlFor="miitApi">{t.companyInfo.drawer.miitApi}</Label>
                <div className="flex gap-2">
                  <Input
                    id="miitApi"
                    placeholder="http://127.0.0.1:16181/"
                    value={formData.miitApi}
                    onChange={(e) =>
                      onFormDataChange({ ...formData, miitApi: e.target.value })
                    }
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() =>
                      window.open('https://github.com/HG-ha/ICP_Query', '_blank')
                    }
                  >
                    <ExternalLink className="w-4 h-4" />
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground">
                  {t.companyInfo.drawer.miitApiHint}
                </p>
              </div>
            )}
          </div>
        </div>
        <SheetFooter className="mt-auto border-t">
          <div className="flex items-center gap-2 w-full">
            <Button onClick={onCreateTask} className="flex-1">{t.companyInfo.drawer.createTask}</Button>
            <SheetClose asChild>
              <Button variant="outline" className="flex-1">{t.common.cancel}</Button>
            </SheetClose>
          </div>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
