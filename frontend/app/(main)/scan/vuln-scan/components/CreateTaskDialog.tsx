import { useRef } from "react"
import { useIsMobile } from "@/hooks/use-mobile"
import { Sheet, SheetContent, SheetHeader, SheetTitle, SheetDescription } from "@/components/ui/sheet"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"
import { Globe, Server, Target, Layers, ArrowRight, Sparkles, FolderSearch, FileCode, Grid2x2, Blocks, Captions } from "lucide-react"
import { useI18n } from "@/lib/i18n"
import { ScanType } from "../types"

type TaskTypeOption = {
  type: ScanType
  title: string
  description: string
  icon: typeof Globe
  color: string
  bg: string
  border: string
  gradient: string
}

interface CreateTaskDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSelectType: (type: ScanType) => void
  onSelectCombination?: () => void
}

export function CreateTaskDialog({
  open,
  onOpenChange,
  onSelectType,
  onSelectCombination,
}: CreateTaskDialogProps) {
  const { t } = useI18n()
  const isMobile = useIsMobile()
  const titleRef = useRef<HTMLHeadingElement>(null)
  const Root = isMobile ? Sheet : Dialog
  const Content = isMobile ? SheetContent : DialogContent
  const Header = isMobile ? SheetHeader : DialogHeader
  const Title = isMobile ? SheetTitle : DialogTitle
  const Description = isMobile ? SheetDescription : DialogDescription

  const taskTypes: TaskTypeOption[] = [
    {
      type: ScanType.Subdomain,
      title: t.vulnScan.taskManagement.subdomain,
      description: t.vulnScan.createTaskDialog.taskTypes.subdomainDesc,
      icon: Captions,
      color: "text-tone-green",
      bg: "bg-tone-green-soft",
      border: "hover:border-tone-green/50 hover:shadow-tone-green/10",
      gradient: "from-tone-green/5 to-transparent",
    },
    {
      type: ScanType.WebScan,
      title: t.vulnScan.taskManagement.webscan,
      description: t.vulnScan.createTaskDialog.taskTypes.webscanDesc,
      icon: Globe,
      // 使用更现代的颜色配置
      color: "text-blue-600 dark:text-blue-400",
      bg: "bg-blue-50 dark:bg-blue-900/20",
      border: "hover:border-blue-500/50 hover:shadow-blue-500/10",
      gradient: "from-blue-500/5 to-transparent",
    },
    {
      type: ScanType.PortScan,
      title: t.vulnScan.taskManagement.portscan,
      description: t.vulnScan.createTaskDialog.taskTypes.portscanDesc,
      icon: Server,
      color: "text-emerald-600 dark:text-emerald-400",
      bg: "bg-emerald-50 dark:bg-emerald-900/20",
      border: "hover:border-emerald-500/50 hover:shadow-emerald-500/10",
      gradient: "from-emerald-500/5 to-transparent",
    },
    {
      type: ScanType.Crack,
      title: t.vulnScan.taskManagement.crack,
      description: t.vulnScan.createTaskDialog.taskTypes.crackDesc,
      icon: Target,
      color: "text-orange-600 dark:text-orange-400",
      bg: "bg-orange-50 dark:bg-orange-900/20",
      border: "hover:border-orange-500/50 hover:shadow-orange-500/10",
      gradient: "from-orange-500/5 to-transparent",
    },
    {
      type: ScanType.DirScan,
      title: t.vulnScan.taskManagement.dirscan,
      description: t.vulnScan.createTaskDialog.taskTypes.dirscanDesc,
      icon: FolderSearch,
      color: "text-purple-600 dark:text-purple-400",
      bg: "bg-purple-50 dark:bg-purple-900/20",
      border: "hover:border-purple-500/50 hover:shadow-purple-500/10",
      gradient: "from-purple-500/5 to-transparent",
    },
    {
      type: ScanType.JsScan,
      title: t.vulnScan.taskManagement.jsscan,
      description: t.vulnScan.createTaskDialog.taskTypes.jsscanDesc,
      icon: FileCode,
      color: "text-amber-600 dark:text-amber-400",
      bg: "bg-amber-50 dark:bg-amber-900/20",
      border: "hover:border-amber-500/50 hover:shadow-amber-500/10",
      gradient: "from-amber-500/5 to-transparent",
    }
  ]

  return (
    <Root open={open} onOpenChange={onOpenChange}>
      <Content
        {...(isMobile ? { side: "bottom" as const } : {})}
        className={isMobile
          ? "max-h-[85dvh] gap-0 overflow-hidden rounded-t-2xl p-0 pb-[env(safe-area-inset-bottom)]"
          : "sm:max-w-4xl max-h-[85vh] overflow-y-auto p-0 gap-0"}
        onOpenAutoFocus={isMobile ? (event) => {
          event.preventDefault()
          titleRef.current?.focus()
        } : undefined}
      >
        <div className="shrink-0 p-4 pr-10 md:p-6 md:pb-4 border-b bg-muted/10">
          <Header className="p-0">
            <Title ref={titleRef} tabIndex={-1} className="text-xl flex items-center gap-2 outline-none">
              <Sparkles className="w-5 h-5 text-primary" />
              {t.vulnScan.newTask}
            </Title>
            <Description>
              {t.vulnScan.createTaskDialog.description}
            </Description>
          </Header>
        </div>

        <div className="min-h-0 overflow-y-auto overscroll-contain p-4 md:p-6 grid gap-6">
          <section>
            <div className="flex flex-wrap items-center justify-between gap-2 mb-4">
              <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <Grid2x2 className="h-4 w-4 text-muted-foreground" />
                {t.vulnScan.createTaskDialog.capabilityTitle}
              </h3>
              <span className="text-xs text-muted-foreground">{t.vulnScan.createTaskDialog.capabilityHint}</span>
            </div>

            <div className="grid grid-cols-2 md:grid-cols-3 gap-3 md:gap-4">
              {taskTypes.map((item) => (
                <Card
                  key={item.type}
                  className={cn(
                    "group relative cursor-pointer border transition-all duration-300 overflow-hidden",
                    "hover:-translate-y-1 hover:shadow-lg",
                    item.border
                  )}
                  onClick={() => {
                    onSelectType(item.type)
                    onOpenChange(false)
                  }}
                >
                  <div className={cn("absolute inset-0 opacity-0 group-hover:opacity-100 transition-opacity bg-gradient-to-br", item.gradient)} />

                  <CardHeader className="relative space-y-2 px-4">
                    <div className="flex justify-between items-start">
                      <div className={cn("h-11 w-11 rounded-xl flex items-center justify-center transition-transform group-hover:scale-110", item.bg)}>
                        <item.icon className={cn("h-5.5 w-5.5 shrink-0", item.color)} />
                      </div>
                      <div className="opacity-0 -translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-300">
                        <ArrowRight className="w-5 h-5 text-muted-foreground" />
                      </div>
                    </div>
                    
                    <div className="space-y-1">
                      <CardTitle className="text-base group-hover:text-primary transition-colors">
                        {item.title}
                      </CardTitle>
                      <CardDescription className="line-clamp-2 text-xs">
                        {item.description}
                      </CardDescription>
                    </div>
                  </CardHeader>
                </Card>
              ))}
            </div>
          </section>

          <section>
            <div className="flex flex-wrap items-center justify-between gap-2 mb-4">
              <h3 className="flex items-center gap-2 text-sm font-semibold text-foreground">
                <Blocks className="h-4 w-4 text-muted-foreground" />
                {t.vulnScan.createTaskDialog.attackSurfaceTitle}
              </h3>
              <span className="text-xs text-muted-foreground">{t.vulnScan.createTaskDialog.attackSurfaceHint}</span>
            </div>

            <Card
              className={cn(
                "group border-border/70 bg-card transition-colors duration-200",
                onSelectCombination
                  ? "cursor-pointer hover:border-primary/30 hover:bg-muted/20"
                  : "opacity-80",
              )}
              aria-disabled={!onSelectCombination}
              onClick={() => {
                if (!onSelectCombination) return
                onSelectCombination()
                onOpenChange(false)
              }}
            >
              <CardHeader className="px-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="space-y-2">
                    <div className="flex items-center gap-3">
                      <div className="flex h-9 w-9 items-center justify-center rounded-xl border border-primary/20 bg-muted/30 text-primary">
                        <Layers className="h-5 w-5" />
                      </div>
                      <div className="flex flex-wrap items-center gap-2">
                        <Badge variant="outline" className="h-6 px-2 text-[10px]">
                          {t.vulnScan.taskManagement.combination}
                        </Badge>
                      </div>
                    </div>
                    <div className="space-y-1.5">
                      <CardTitle className="text-base">
                        {t.vulnScan.createTaskDialog.attackSurfaceCardTitle}
                      </CardTitle>
                      <CardDescription className="text-sm leading-6">
                        {t.vulnScan.createTaskDialog.attackSurfaceCardDesc}
                      </CardDescription>
                    </div>
                  </div>
                  <ArrowRight className="mt-1 h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-1 group-hover:text-primary" />
                </div>
              </CardHeader>
            </Card>
          </section>
        </div>
      </Content>
    </Root>
  )
}
