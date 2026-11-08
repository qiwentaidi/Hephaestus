'use client'

import { useMemo, useState } from 'react'
import { Shield, ShieldAlert, ShieldCheck, ShieldQuestion } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import toast from '@/lib/toast'
import { useI18n } from '@/lib/i18n'
import { antivirusProcessMap } from '@/lib/tasklist-antivirus-map'

type TasklistProcess = {
  imageName: string
  pid: string
  services: string[]
  servicesCount: number
  hasServices: boolean
}

type DetectionResult = TasklistProcess & {
  antivirusList: string[]
}

const SYSTEM_PROCESSES = new Set([
  'system',
  'system idle process',
  'smss.exe',
  'csrss.exe',
  'winlogon.exe',
  'services.exe',
  'lsass.exe',
  'svchost.exe',
  'spoolsv.exe',
  'explorer.exe',
  'dwm.exe',
  'conhost.exe',
])

function parseProcessLine(line: string): TasklistProcess | null {
  const match = line.match(/^(\S+)\s+(\d+)/)
  if (!match) return null

  const imageName = match[1].trim()
  const pid = match[2]
  const servicesMatch = line.match(/^\S+\s+\d+\s+(.+)$/)
  const servicesText = servicesMatch?.[1]?.trim() || ''
  const services =
    servicesText && servicesText !== 'N/A'
      ? servicesText
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean)
      : []

  return {
    imageName,
    pid,
    services,
    servicesCount: services.length,
    hasServices: services.length > 0,
  }
}

function parseTasklistOutput(output: string) {
  const lines = output.split(/\r?\n/)
  const processes: TasklistProcess[] = []
  let headerLineIndex = -1

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i].trim()
    if (line.includes('Image Name') && line.includes('PID') && line.includes('Services')) {
      headerLineIndex = i
      break
    }
  }

  const startIndex = headerLineIndex >= 0 ? headerLineIndex + 1 : 0
  for (let i = startIndex; i < lines.length; i += 1) {
    const line = lines[i].trim()
    if (!line || /^[=\-]+$/.test(line)) continue
    const process = parseProcessLine(line)
    if (process) {
      processes.push(process)
    }
  }

  return processes
}

function dedupe(list: string[]) {
  return [...new Set(list)]
}

export default function TasklistAVPage() {
  const { t } = useI18n()
  const [content, setContent] = useState('')
  const [skipSystemProcesses, setSkipSystemProcesses] = useState(true)
  const [results, setResults] = useState<DetectionResult[]>([])
  const [parsedCount, setParsedCount] = useState(0)
  const [skippedCount, setSkippedCount] = useState(0)
  const [hasScanned, setHasScanned] = useState(false)

  const avProductCount = useMemo(() => {
    const products = new Set<string>()
    results.forEach((item) => item.antivirusList.forEach((name) => products.add(name)))
    return products.size
  }, [results])

  const handleAnalyze = () => {
    const trimmed = content.trim()
    if (!trimmed) {
      toast.error({
        title: t.common.error,
        message: t.tools.tasklistAV.emptyInput,
      })
      return
    }

    const processes = parseTasklistOutput(trimmed)
    if (processes.length === 0) {
      toast.error({
        title: t.common.error,
        message: t.tools.tasklistAV.parseFailed,
      })
      return
    }

    let skipped = 0
    const detected: DetectionResult[] = []

    processes.forEach((process) => {
      const processName = process.imageName.toLowerCase()
      if (skipSystemProcesses && SYSTEM_PROCESSES.has(processName)) {
        skipped += 1
        return
      }

      const antivirusList = dedupe(antivirusProcessMap[processName] || [])
      if (antivirusList.length > 0) {
        detected.push({
          ...process,
          antivirusList,
        })
      }
    })

    setParsedCount(processes.length)
    setSkippedCount(skipped)
    setResults(detected)
    setHasScanned(true)

    toast.success({
      title: t.tools.tasklistAV.detectComplete,
      message: t.tools.tasklistAV.detectSummary
        .replace('{processes}', String(processes.length))
        .replace('{matched}', String(detected.length)),
    })
  }

  const handleClear = () => {
    setContent('')
    setResults([])
    setParsedCount(0)
    setSkippedCount(0)
    setHasScanned(false)
  }

  return (
    <div className="flex-1 overflow-auto p-6">
      <div className="flex w-full flex-col gap-5">
        <Card className="border-border/80 bg-card/95 shadow-sm">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2">
              <ShieldAlert className="h-5 w-5 text-primary" />
              {t.tools.tasklistAV.title}
            </CardTitle>
            <p className="text-sm text-muted-foreground">{t.tools.tasklistAV.description}</p>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex items-center justify-between rounded-lg border border-border bg-card/50 px-4 py-3">
              <div className="space-y-1">
                <Label>{t.tools.tasklistAV.skipSystemProcesses}</Label>
                <p className="text-xs text-muted-foreground">{t.tools.tasklistAV.skipSystemDescription}</p>
              </div>
              <Switch checked={skipSystemProcesses} onCheckedChange={setSkipSystemProcesses} />
            </div>

            <div className="space-y-2">
              <Label>{t.tools.tasklistAV.inputLabel}</Label>
              <Textarea
                value={content}
                onChange={(e) => setContent(e.target.value)}
                placeholder={t.tools.tasklistAV.inputPlaceholder}
                className="min-h-[240px] max-h-[420px] resize-y overflow-y-auto font-mono text-sm"
              />
            </div>

            <div className="flex items-center gap-3">
              <Button onClick={handleAnalyze}>
                <Shield className="mr-2 h-4 w-4" />
                {t.tools.tasklistAV.startDetect}
              </Button>
              <Button variant="outline" onClick={handleClear}>
                {t.common.clear}
              </Button>
            </div>
          </CardContent>
        </Card>

        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
          <Card className="gap-0 border-border/80 bg-card/95 py-0 shadow-sm">
            <CardContent className="flex h-full flex-col justify-start gap-2 px-5 py-5">
              <div className="text-sm text-muted-foreground">{t.tools.tasklistAV.stats.totalProcesses}</div>
              <div className="text-3xl font-semibold">{parsedCount}</div>
            </CardContent>
          </Card>
          <Card className="gap-0 border-border/80 bg-card/95 py-0 shadow-sm">
            <CardContent className="flex h-full flex-col justify-start gap-2 px-5 py-5">
              <div className="text-sm text-muted-foreground">{t.tools.tasklistAV.stats.detectedProcesses}</div>
              <div className="text-3xl font-semibold text-orange-400">{results.length}</div>
            </CardContent>
          </Card>
          <Card className="gap-0 border-border/80 bg-card/95 py-0 shadow-sm">
            <CardContent className="flex h-full flex-col justify-start gap-2 px-5 py-5">
              <div className="text-sm text-muted-foreground">{t.tools.tasklistAV.stats.detectedProducts}</div>
              <div className="text-3xl font-semibold text-emerald-400">{avProductCount}</div>
            </CardContent>
          </Card>
          <Card className="gap-0 border-border/80 bg-card/95 py-0 shadow-sm">
            <CardContent className="flex h-full flex-col justify-start gap-2 px-5 py-5">
              <div className="text-sm text-muted-foreground">{t.tools.tasklistAV.stats.skippedProcesses}</div>
              <div className="text-3xl font-semibold text-muted-foreground">{skippedCount}</div>
            </CardContent>
          </Card>
        </div>

        <Card className="border-border/80 bg-card/95 shadow-sm">
          <CardHeader className="pb-3">
            <CardTitle>{t.tools.tasklistAV.resultsTitle}</CardTitle>
          </CardHeader>
          <CardContent>
            {!hasScanned ? (
              <div className="flex min-h-[200px] items-center justify-center text-muted-foreground">
                <div className="flex items-center gap-2">
                  <ShieldQuestion className="h-4 w-4" />
                  <span>{t.tools.tasklistAV.emptyHint}</span>
                </div>
              </div>
            ) : results.length === 0 ? (
              <div className="flex min-h-[200px] items-center justify-center text-muted-foreground">
                <div className="flex items-center gap-2">
                  <ShieldCheck className="h-4 w-4 text-emerald-400" />
                  <span>{t.tools.tasklistAV.noMatchedResults}</span>
                </div>
              </div>
            ) : (
              <div className="overflow-x-auto rounded-lg border border-border">
                <table className="min-w-full text-sm">
                  <thead className="bg-muted/30 text-left">
                    <tr>
                      <th className="px-4 py-3 font-medium">{t.tools.tasklistAV.columns.process}</th>
                      <th className="px-4 py-3 font-medium">{t.tools.tasklistAV.columns.pid}</th>
                      <th className="px-4 py-3 font-medium">{t.tools.tasklistAV.columns.services}</th>
                      <th className="px-4 py-3 font-medium">{t.tools.tasklistAV.columns.antivirus}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {results.map((result) => (
                      <tr key={`${result.imageName}-${result.pid}`} className="border-t border-border">
                        <td className="px-4 py-3 font-mono">{result.imageName}</td>
                        <td className="px-4 py-3">{result.pid}</td>
                        <td className="px-4 py-3">
                          {result.services.length > 0 ? (
                            <div className="flex flex-wrap gap-2">
                              {result.services.map((service) => (
                                <Badge key={service} variant="outline" className="bg-muted/20">
                                  {service}
                                </Badge>
                              ))}
                            </div>
                          ) : (
                            <span className="text-muted-foreground">N/A</span>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <div className="flex flex-wrap gap-2">
                            {result.antivirusList.map((name) => (
                              <Badge key={name} className="border-orange-500/30 bg-orange-500/15 text-orange-300 hover:bg-orange-500/15">
                                {name}
                              </Badge>
                            ))}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
