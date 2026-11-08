'use client'

import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Download } from 'lucide-react'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import toast from '@/lib/toast'
import type { FingerprintInfo, VulnerabilityInfo, PortInfo } from '../types'

interface ExportDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  fingerprints?: FingerprintInfo[]
  vulnerabilities?: VulnerabilityInfo[]
  ports?: PortInfo[]
}

export function ExportDialog({
  open,
  onOpenChange,
  fingerprints = [],
  vulnerabilities = [],
  ports = [],
}: ExportDialogProps) {
  const [exportType, setExportType] = useState<'fingerprint' | 'vulnerability' | 'port' | 'all'>('all')
  const [exportFormat, setExportFormat] = useState<'csv' | 'json' | 'txt'>('csv')

  const handleExport = () => {
    let data: any[] = []
    let filename = ''

    if (exportType === 'fingerprint') {
      data = fingerprints.map((f) => ({
        url: f.url,
        status: f.status,
        title: f.title,
        fingerprints: f.fingerprints,
        length: f.length,
      }))
      filename = `webscan-fingerprints-${Date.now()}`
    } else if (exportType === 'vulnerability') {
      data = vulnerabilities.map((v) => ({
        vulnName: v.vulnName,
        severity: v.severity,
        url: v.vulnUrl,
        protocol: v.protocol,
        description: v.description,
      }))
      filename = `webscan-vulnerabilities-${Date.now()}`
    } else if (exportType === 'port') {
      data = ports.map((p) => ({
        host: p.host,
        port: p.port,
        protocol: p.protocol,
        state: p.state,
        service: p.service,
        version: p.version,
        banner: p.banner,
      }))
      filename = `webscan-ports-${Date.now()}`
    } else {
      // all
      data = [
        ...fingerprints.map((f) => ({
          type: 'fingerprint',
          url: f.url,
          status: f.status,
          title: f.title,
          fingerprints: f.fingerprints,
          length: f.length,
        })),
        ...ports.map((p) => ({
          type: 'port',
          host: p.host,
          port: p.port,
          protocol: p.protocol,
          state: p.state,
          service: p.service,
          version: p.version,
          banner: p.banner,
        })),
        ...vulnerabilities.map((v) => ({
          type: 'vulnerability',
          vulnName: v.vulnName,
          severity: v.severity,
          url: v.vulnUrl,
          protocol: v.protocol,
          description: v.description,
        })),
      ]
      filename = `webscan-all-${Date.now()}`
    }

    let content = ''
    let mimeType = ''

    if (exportFormat === 'csv') {
      if (data.length === 0) {
        toast.warning('没有数据可导出')
        return
      }
      const headers = Object.keys(data[0])
      const rows = data.map((item) =>
        headers.map((key) => {
          const value = item[key]
          return `"${String(value || '').replace(/"/g, '""')}"`
        }).join(',')
      )
      content = '\ufeff' + [headers.join(','), ...rows].join('\n')
      mimeType = 'text/csv;charset=utf-8;'
      filename += '.csv'
    } else if (exportFormat === 'json') {
      content = JSON.stringify(data, null, 2)
      mimeType = 'application/json'
      filename += '.json'
    } else {
      // txt format
      if (exportType === 'fingerprint') {
        content = fingerprints.map((f) => f.url).join('\n')
      } else if (exportType === 'port') {
        content = ports.map((p) => `${p.host}:${p.port}`).join('\n')
      } else if (exportType === 'vulnerability') {
        content = vulnerabilities.map((v) => v.vulnUrl).join('\n')
      } else {
        // all
        const lines: string[] = []
        if (fingerprints.length > 0) {
          lines.push('# Fingerprints')
          lines.push(...fingerprints.map((f) => f.url))
        }
        if (ports.length > 0) {
          lines.push('\n# Ports')
          lines.push(...ports.map((p) => `${p.host}:${p.port}`))
        }
        if (vulnerabilities.length > 0) {
          lines.push('\n# Vulnerabilities')
          lines.push(...vulnerabilities.map((v) => v.vulnUrl))
        }
        content = lines.join('\n')
      }
      mimeType = 'text/plain'
      filename += '.txt'
    }

    const blob = new Blob([content], { type: mimeType })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = filename
    link.click()
    URL.revokeObjectURL(link.href)

    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>导出数据</DialogTitle>
          <DialogDescription>
            选择要导出的数据类型和格式
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-6 py-4">
          <div className="space-y-2">
            <Label>数据类型</Label>
            <Select value={exportType} onValueChange={(value: any) => setExportType(value)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">
                  全部 ({fingerprints.length} 指纹 + {ports.length} 端口 + {vulnerabilities.length} 漏洞)
                </SelectItem>
                <SelectItem value="fingerprint">
                  指纹信息 ({fingerprints.length} 条)
                </SelectItem>
                <SelectItem value="port">
                  端口信息 ({ports.length} 条)
                </SelectItem>
                <SelectItem value="vulnerability">
                  漏洞信息 ({vulnerabilities.length} 条)
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label>导出格式</Label>
            <Select value={exportFormat} onValueChange={(value: any) => setExportFormat(value)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="csv">CSV</SelectItem>
                <SelectItem value="json">JSON</SelectItem>
                <SelectItem value="txt">TXT (仅URL)</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={handleExport}>
            <Download className="w-4 h-4 mr-2" />
            导出
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
