'use client'

import { useId, useState, type CSSProperties } from 'react'
import {
  AlertTriangle,
  Calendar as CalendarIcon,
  Check,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  FileText,
  Info,
  MoreHorizontal,
  Plus,
  Search,
  Upload,
  X,
  XCircle,
} from 'lucide-react'
import { format } from 'date-fns'
import { zhCN } from 'date-fns/locale'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Calendar } from '@/components/ui/calendar'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Button, buttonVariants } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationLink,
} from '@/components/ui/pagination'
import { Progress } from '@/components/ui/progress'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useI18n } from '@/lib/i18n'
import { themeToCssVars, type CustomThemeDefinition } from '@/lib/theme/custom-theme'
import { cn } from '@/lib/utils'

const CARD = 'mb-3 break-inside-avoid rounded-lg border border-[var(--border)] bg-[var(--card)] p-3 text-[var(--card-foreground)]'
const TITLE = 'mb-2 text-xs font-semibold text-[var(--card-foreground)]'

function stop(event: React.MouseEvent) {
  event.preventDefault()
}

interface ThemeStyleOverviewProps {
  definition: CustomThemeDefinition
  mode?: 'components' | 'tokens'
}

function ThemeTokenOverview({ definition }: { definition: CustomThemeDefinition }) {
  const { t } = useI18n()
  const copy = t.theme.customThemes.editor.previewSections
  const style = themeToCssVars(definition) as CSSProperties
  const palette = [
    { label: copy.primary, token: '--primary', color: 'var(--primary)' },
    { label: copy.background, token: '--background', color: 'var(--background)' },
    { label: copy.card, token: '--card', color: 'var(--card)' },
    { label: copy.muted, token: '--muted', color: 'var(--muted)' },
    { label: copy.border, token: '--border', color: 'var(--border)' },
  ]

  return (
    <div style={style} className="grid grid-cols-1 gap-3 sm:grid-cols-2">
      <section className={CARD}>
        <div className={TITLE}>{copy.palette}</div>
        <div className="grid grid-cols-2 gap-2">
          {palette.map((item) => (
            <div key={item.token} className="flex items-center gap-2 rounded-md border border-[var(--border)] bg-[var(--background)] p-2">
              <span className="h-7 w-7 shrink-0 rounded-md border border-[var(--border)]" style={{ backgroundColor: item.color }} />
              <div className="min-w-0">
                <div className="truncate text-[11px] text-[var(--foreground)]">{item.label}</div>
                <code className="text-[9px] text-[var(--muted-foreground)]">{item.token}</code>
              </div>
            </div>
          ))}
        </div>
      </section>

      <section className={CARD}>
        <div className={TITLE}>{copy.typography}</div>
        <div className="space-y-2 rounded-md border border-[var(--border)] bg-[var(--card)] p-3">
          <div className="text-lg font-semibold text-[var(--foreground)]">{copy.heading}</div>
          <div className="text-sm text-[var(--foreground)]">{copy.body}</div>
          <div className="text-[11px] text-[var(--muted-foreground)]">{copy.caption}</div>
          <div className="flex items-center gap-2 pt-1">
            <span className="rounded-[var(--radius)] border border-[var(--border)] px-2 py-1 text-[10px] text-[var(--muted-foreground)]">
              radius {definition.radius}
            </span>
            <span className="rounded-full border border-[var(--border)] px-2 py-1 text-[10px] text-[var(--muted-foreground)]">
              pill
            </span>
          </div>
        </div>
      </section>

      <section className={CARD}>
        <div className={TITLE}>{copy.states}</div>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm">Primary</Button>
          <Button size="sm" variant="outline">Default</Button>
          <Button size="sm" disabled>{copy.disabled}</Button>
          <span
            className="rounded-full border px-2 py-1 text-[10px]"
            style={{
              color: 'var(--ring)',
              borderColor: 'var(--ring)',
              boxShadow: '0 0 0 2px color-mix(in srgb, var(--ring) 45%, transparent)',
            }}
          >
            {copy.focus}
          </span>
        </div>
      </section>

      <section className={CARD}>
        <div className={TITLE}>{copy.surfaces}</div>
        <div className="grid grid-cols-2 gap-2">
          <div className="rounded-md border border-[var(--border)] bg-[var(--background)] p-2 text-[10px] text-[var(--muted-foreground)]">
            background
          </div>
          <div className="rounded-md border border-[var(--border)] bg-[var(--card)] p-2 text-[10px] text-[var(--card-foreground)]">
            card
          </div>
          <div className="rounded-md border border-[var(--border)] bg-[var(--muted)] p-2 text-[10px] text-[var(--muted-foreground)]">
            muted
          </div>
          <div className="rounded-md border border-[var(--border)] bg-[var(--primary)] p-2 text-[10px] text-[var(--primary-foreground)]">
            primary
          </div>
        </div>
      </section>
    </div>
  )
}

export function ThemeStyleOverview({ definition, mode = 'components' }: ThemeStyleOverviewProps) {
  const { t, language } = useI18n()
  const copy = t.theme.customThemes.editor.previewSections
  const id = useId()
  const popoverStyle = themeToCssVars(definition) as CSSProperties
  const [risk, setRisk] = useState('medium')
  const [auto, setAuto] = useState(true)
  const [threshold, setThreshold] = useState(60)
  const [date, setDate] = useState<Date | undefined>(new Date())
  const [tab, setTab] = useState('overview')
  const [page, setPage] = useState(1)

  if (mode === 'tokens') {
    return <ThemeTokenOverview definition={definition} />
  }

  return (
    <div className="mt-3 columns-1 [column-gap:0.75rem] sm:columns-2">
        <section className={CARD}>
          <div className={TITLE}>{copy.buttons}</div>
          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm">Primary</Button>
            <Button size="sm" variant="outline">Default</Button>
            <Button size="sm" variant="secondary">Secondary</Button>
            <Button size="sm" variant="destructive">Danger</Button>
            <Button size="sm" variant="ghost">Text</Button>
            <Button size="sm" variant="link">Link</Button>
            <Button size="sm" disabled>Disabled</Button>
            <Button size="icon-sm" variant="outline" aria-label="Add">
              <Plus className="h-3.5 w-3.5" />
            </Button>
            <Button size="icon-sm" variant="outline" aria-label="More">
              <MoreHorizontal className="h-3.5 w-3.5" />
            </Button>
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>{copy.forms}</div>
          <div className="grid grid-cols-2 gap-2">
            <Input placeholder="Scan target" className="h-8 text-xs" />
            <div className="relative">
              <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input placeholder="Search" className="h-8 pl-7 text-xs" />
            </div>
            <Select value={risk} onValueChange={setRisk}>
              <SelectTrigger size="sm" className="h-8 w-full text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent style={popoverStyle}>
                <SelectItem value="low">Low risk</SelectItem>
                <SelectItem value="medium">Medium risk</SelectItem>
                <SelectItem value="high">High risk</SelectItem>
              </SelectContent>
            </Select>
            <Popover>
              <PopoverTrigger asChild>
                <button
                  type="button"
                  className={cn(
                    buttonVariants({ variant: 'outline', size: 'sm' }),
                    'h-8 w-full justify-start px-2 text-xs font-normal',
                  )}
                >
                  <CalendarIcon className="mr-1.5 h-3.5 w-3.5 text-muted-foreground" />
                  {date ? format(date, 'yyyy-MM-dd') : 'Pick a date'}
                </button>
              </PopoverTrigger>
              <PopoverContent style={popoverStyle} className="w-auto p-0" align="start">
                <Calendar
                  mode="single"
                  selected={date}
                  onSelect={setDate}
                  locale={language === 'zh' ? zhCN : undefined}
                  initialFocus
                />
              </PopoverContent>
            </Popover>
          </div>

          <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-2">
            <label className="flex items-center gap-2 text-xs">
              <Checkbox defaultChecked />
              Checkbox
            </label>
            <RadioGroup value={risk} onValueChange={setRisk} className="flex flex-wrap items-center gap-3">
              {['low', 'medium', 'high'].map((level) => (
                <label key={level} className="flex items-center gap-1.5 text-xs capitalize">
                  <RadioGroupItem value={level} id={`${id}-${level}`} />
                  <Label htmlFor={`${id}-${level}`} className="text-xs capitalize">
                    {level}
                  </Label>
                </label>
              ))}
            </RadioGroup>
            <label className="flex items-center gap-2 text-xs">
              <Switch checked={auto} onCheckedChange={setAuto} />
              Switch
            </label>
          </div>

          <div className="mt-3">
            <div className="mb-1 flex items-center justify-between text-xs text-muted-foreground">
              <span>Threshold</span>
              <span>{threshold}%</span>
            </div>
            <Slider
              value={[threshold]}
              onValueChange={([value]) => setThreshold(value)}
              max={100}
              step={1}
            />
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>{copy.tabs}</div>
          <Tabs value={tab} onValueChange={setTab}>
            <TabsList className="h-8">
              <TabsTrigger value="overview" className="px-2 text-xs">Overview</TabsTrigger>
              <TabsTrigger value="findings" className="px-2 text-xs">Findings</TabsTrigger>
              <TabsTrigger value="settings" className="px-2 text-xs">Settings</TabsTrigger>
            </TabsList>
          </Tabs>
          <div className="mt-3 flex items-start">
            {[
              { label: 'Finished', state: 'done' },
              { label: 'Running', state: 'active' },
              { label: 'Waiting', state: 'todo' },
            ].map((step, index) => (
              <div key={step.label} className="flex min-w-0 flex-1 items-start last:flex-none">
                <span
                  className={cn(
                    'flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[10px]',
                    step.state === 'done' && 'bg-[var(--tone-green-soft)] text-[var(--tone-green)]',
                    step.state === 'active' && 'bg-[var(--primary)] text-[var(--primary-foreground)]',
                    step.state === 'todo' && 'bg-[var(--muted)] text-[var(--muted-foreground)]',
                  )}
                >
                  {step.state === 'done' ? <Check className="h-3 w-3" /> : index + 1}
                </span>
                <span className="ml-1.5 min-w-0 truncate text-[10px] text-[var(--muted-foreground)]">
                  {step.label}
                </span>
                {index < 2 && <span className="mx-1.5 mt-2.5 h-px min-w-2 flex-1 bg-[var(--border)]" />}
              </div>
            ))}
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>{copy.table}</div>
          <div className="overflow-hidden rounded-md border border-[var(--border)]">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Asset</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {[
                  ['api.example.com', 'High', 'orange'],
                  ['cdn.example.com', 'Medium', 'amber'],
                  ['docs.example.com', 'Low', 'green'],
                ].map(([host, level, tone]) => (
                  <TableRow key={host}>
                    <TableCell className="text-xs">{host}</TableCell>
                    <TableCell>
                      <Badge
                        variant="outline"
                        className="text-[10px]"
                        style={{
                          backgroundColor: `var(--tone-${tone}-soft)`,
                          borderColor: `var(--tone-${tone}-border)`,
                          color: `var(--tone-${tone})`,
                        }}
                      >
                        {level}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <Button size="sm" variant="ghost" className="h-6 px-2 text-[10px]">
                        Edit
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
          <Pagination className="mt-2 justify-end">
            <PaginationContent>
              <PaginationItem>
                <PaginationLink href="#" size="icon" className="h-7 w-7" onClick={stop}>
                  <ChevronLeft className="h-3.5 w-3.5" />
                </PaginationLink>
              </PaginationItem>
              {[1, 2, 3].map((item) => (
                <PaginationItem key={item}>
                  <PaginationLink
                    href="#"
                    size="icon"
                    isActive={page === item}
                    className="h-7 w-7 text-xs"
                    onClick={(event) => {
                      event.preventDefault()
                      setPage(item)
                    }}
                  >
                    {item}
                  </PaginationLink>
                </PaginationItem>
              ))}
              <PaginationItem>
                <PaginationLink href="#" size="icon" className="h-7 w-7" onClick={stop}>
                  <ChevronRight className="h-3.5 w-3.5" />
                </PaginationLink>
              </PaginationItem>
            </PaginationContent>
          </Pagination>
        </section>
        <section className={CARD}>
          <div className={TITLE}>{copy.alerts}</div>
          <div className="space-y-2">
            <Alert className="border-[var(--tone-red-border)] bg-[var(--tone-red-soft)] py-2 text-[var(--tone-red)]">
              <XCircle />
              <AlertTitle className="text-xs">Error</AlertTitle>
              <AlertDescription className="text-[10px] text-[var(--tone-red)]/80">
                This is an error message.
              </AlertDescription>
            </Alert>
            <Alert className="border-[var(--tone-amber-border)] bg-[var(--tone-amber-soft)] py-2 text-[var(--tone-amber)]">
              <AlertTriangle />
              <AlertTitle className="text-xs">Warning</AlertTitle>
              <AlertDescription className="text-[10px] text-[var(--tone-amber)]/80">
                This is a warning message.
              </AlertDescription>
            </Alert>
            <Alert className="border-[var(--tone-green-border)] bg-[var(--tone-green-soft)] py-2 text-[var(--tone-green)]">
              <CheckCircle2 />
              <AlertTitle className="text-xs">Success</AlertTitle>
              <AlertDescription className="text-[10px] text-[var(--tone-green)]/80">
                The operation completed.
              </AlertDescription>
            </Alert>
            <Alert className="border-[var(--tone-blue-border)] bg-[var(--tone-blue-soft)] py-2 text-[var(--tone-blue)]">
              <Info />
              <AlertTitle className="text-xs">Info</AlertTitle>
              <AlertDescription className="text-[10px] text-[var(--tone-blue)]/80">
                A neutral informational message.
              </AlertDescription>
            </Alert>
          </div>
        </section>


        <section className={CARD}>
          <div className={TITLE}>{copy.progress}</div>
          <div className="space-y-2">
            <Progress value={65} />
            <Progress
              value={35}
              className="[&_[data-slot=progress-indicator]]:bg-[var(--destructive)]"
            />
          </div>
          <div className="mt-3 flex items-center gap-2">
            <span className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--tone-green-soft)] text-[var(--tone-green)]">
              <Check className="h-4 w-4" />
            </span>
            <div className="min-w-0">
              <div className="text-xs font-semibold">Completed</div>
              <div className="text-[10px] text-muted-foreground">Saved successfully</div>
            </div>
            <Button size="sm" variant="outline" className="ml-auto">View</Button>
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>{copy.upload}</div>
          <div className="flex flex-col items-center justify-center rounded-md border border-dashed border-[var(--border)] px-3 py-4 text-center">
            <Upload className="h-4 w-4 text-muted-foreground" />
            <div className="mt-1 text-xs">Drop file or browse</div>
            <Button size="sm" variant="outline" className="mt-2">Browse</Button>
          </div>
          <div className="mt-2 flex items-center gap-2 rounded-md border border-[var(--border)] px-2 py-1.5 text-xs">
            <FileText className="h-3.5 w-3.5 text-[var(--tone-blue)]" />
            <span className="min-w-0 flex-1 truncate">report.pdf</span>
            <Button size="icon-sm" variant="ghost" className="h-5 w-5" aria-label="Remove">
              <X className="h-3 w-3" />
            </Button>
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>{copy.surfaces}</div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-md border border-[var(--border)] bg-[var(--popover)] px-2 py-1 text-xs text-[var(--popover-foreground)]">
              Popover
            </span>
          </div>
          <div className="mt-2 flex flex-wrap gap-1">
            {['blue', 'green', 'purple', 'amber', 'orange', 'red', 'slate'].map((name) => (
              <Badge
                key={name}
                variant="outline"
                className="text-[10px]"
                style={{
                  backgroundColor: `var(--tone-${name}-soft)`,
                  borderColor: `var(--tone-${name}-border)`,
                  color: `var(--tone-${name})`,
                }}
              >
                {name}
              </Badge>
            ))}
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>{copy.overlays}</div>
          <div className="flex flex-wrap items-start gap-2">
            <div className="inline-flex items-center gap-1.5 rounded-md border border-[var(--border)] bg-[var(--popover)] px-2 py-1 text-xs text-[var(--popover-foreground)] shadow-sm">
              <CheckCircle2 className="h-3.5 w-3.5 text-[var(--tone-green)]" />
              Tooltip
            </div>
            <div className="w-32 rounded-md border border-[var(--border)] bg-[var(--popover)] p-1 text-xs text-[var(--popover-foreground)] shadow-md">
              <div className="rounded px-1.5 py-1 hover:bg-[var(--accent)]">Duplicate</div>
              <div className="rounded px-1.5 py-1 text-[var(--destructive)] hover:bg-[var(--accent)]">
                Delete
              </div>
            </div>
          </div>
          <div className="mt-2 rounded-md border border-[var(--border)] bg-[var(--popover)] p-2 text-[var(--popover-foreground)] shadow-md">
            <div className="text-xs font-semibold">Delete this theme?</div>
            <div className="mt-0.5 text-[10px] text-muted-foreground">This action cannot be undone.</div>
            <div className="mt-2 flex justify-end gap-1.5">
              <Button size="sm" variant="outline">Cancel</Button>
              <Button size="sm" variant="destructive">Delete</Button>
            </div>
          </div>
        </section>

        <section className={CARD}>
          <div className={TITLE}>Input panel</div>
          <div
            className="rounded-md border p-2"
            style={{
              backgroundColor: 'var(--input-panel-bg)',
              borderColor: 'var(--input-panel-border)',
            }}
          >
            <Input
              placeholder="Panel input"
              className="h-8 text-xs"
              style={{ borderColor: 'var(--input)' }}
            />
            <Button
              size="sm"
              variant="outline"
              className="mt-2 w-full hover:bg-[var(--input-panel-action-bg-hover)]"
              style={{
                backgroundColor: 'var(--input-panel-action-bg)',
                borderColor: 'var(--input-panel-action-border)',
              }}
            >
              Panel action
            </Button>
          </div>
        </section>
    </div>
  )
}
