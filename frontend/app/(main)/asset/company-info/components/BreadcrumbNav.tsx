'use client'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ChevronRight, Copy } from 'lucide-react'
import { CompanyInfo } from '../types'
import { copyToClipboard } from '@/lib/clipboard'
import toast from 'react-hot-toast'
import { useI18n } from '@/lib/i18n'

interface BreadcrumbNavProps {
  viewStack: Array<{ company: CompanyInfo; level: number }>
  onNavigateToLevel: (index: number) => void
  onNavigateToHome: () => void
}

export function BreadcrumbNav({
  viewStack,
  onNavigateToLevel,
  onNavigateToHome,
}: BreadcrumbNavProps) {
  const { t } = useI18n()
  const handleCopyName = async (companyName: string, e: React.MouseEvent) => {
    e.stopPropagation()
    const success = await copyToClipboard(companyName)
    if (success) {
      toast.success(t.companyInfo.breadcrumb.copiedCompanyName)
    } else {
      toast.error(t.companyInfo.breadcrumb.copyFailed)
    }
  }

  if (viewStack.length === 0) {
    return null
  }

  return (
    <div className="mb-4 flex items-center gap-2 flex-wrap">
      <Button
        variant="ghost"
        size="sm"
        onClick={onNavigateToHome}
        className="h-8"
      >
        {t.companyInfo.breadcrumb.home}
      </Button>
      {viewStack.map((item, index) => (
        <div key={index} className="flex items-center gap-2">
          <ChevronRight className="w-4 h-4 text-muted-foreground" />
          {index === viewStack.length - 1 ? (
            <div className="flex items-center gap-1.5 group">
              <span className="font-semibold">{item.company.companyName}</span>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-5 w-5 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={(e) => handleCopyName(item.company.companyName, e)}
                    >
                      <Copy className="w-3.5 h-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t.companyInfo.breadcrumb.copyCompanyName}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          ) : (
            <div className="flex items-center gap-1.5 group">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => onNavigateToLevel(index)}
                className="h-8 text-primary hover:underline"
              >
                {item.company.companyName}
              </Button>
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-5 w-5 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
                      onClick={(e) => handleCopyName(item.company.companyName, e)}
                    >
                      <Copy className="w-3.5 h-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t.companyInfo.breadcrumb.copyCompanyName}</TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          )}
        </div>
      ))}
    </div>
  )
}
