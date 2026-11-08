'use client'

import { cloneElement, isValidElement, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Bug, ChevronDown, Copy, FolderPlus, Loader2, RefreshCw, Search, Trash2, Upload, X } from 'lucide-react'
import { useRouter } from 'next/navigation'
import { createPortal } from 'react-dom'
import { fetchWithAuth } from '@/lib/api'
import { copyToClipboard } from '@/lib/clipboard'
import toast from '@/lib/toast'
import { useI18n } from '@/lib/i18n'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { DataTableWithColumns, type ColumnDef } from '@/components/ui/data-table-with-columns'
import { PaginationPro } from '@/components/ui/pagination-pro'
import { SelectTag, type SelectTagOption } from '@/components/ui/select-tag'
import { MonacoEditorWrapper } from '@/app/(main)/scan/repeater/monaco-editor-wrapper'
import {
  createNucleiTemplateDraftFromYaml,
  queueNucleiTemplateImport,
  type NucleiTemplateWorkspacePayload,
} from '@/app/(main)/scan/repeater/nuclei-template'
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'

interface PoCItem {
  fileName: string
  id: string
  name: string
  author: string
  severity: string
  description: string
  tags: string[]
  relatedFingerprint: string
  detectionMethod: 'exploit' | 'principle' | string
  dast?: boolean
  vulnerabilityCategory: string
  vulnLabels: string[]
  createdTime?: string
  updatedTime?: string
  modifiedTime: string
}

interface PoCListResponse {
  items: PoCItem[]
  total: number
  totalAll?: number
  page: number
  pageSize: number
  vulnLabelGroups?: Array<{
    label: string
    count: number
  }>
}

interface PoCSearchSuggestion {
  label: string
  description: string
  insert: string
  keywords: string[]
  example?: string
  category: string
}

const severityClassName = (severity: string) => {
  switch (severity.toLowerCase()) {
    case 'critical':
      return 'bg-red-500/15 text-red-400 border-red-500/30'
    case 'high':
      return 'bg-orange-500/15 text-orange-400 border-orange-500/30'
    case 'medium':
      return 'bg-amber-500/15 text-amber-400 border-amber-500/30'
    case 'low':
      return 'bg-blue-500/15 text-blue-400 border-blue-500/30'
    case 'info':
      return 'bg-slate-500/15 text-slate-300 border-slate-500/30'
    default:
      return 'bg-muted text-muted-foreground border-border'
  }
}

const normalizePoCId = (raw: string) => raw.trim().replace(/\.ya?ml$/i, '')

const formatPoCDate = (value?: string) => {
  const text = (value || '').trim()
  if (!text) return '-'
  return text.split(/[T\s]/)[0] || text
}

const splitPoCSearchClauses = (input: string, separator: string) => {
  const text = input.trim()
  if (!text) return []

  const result: string[] = []
  let current = ''
  let quote = ''

  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i]
    if (quote) {
      current += ch
      if (ch === quote && text[i - 1] !== '\\') {
        quote = ''
      }
      continue
    }

    if (ch === '"' || ch === '\'') {
      quote = ch
      current += ch
      continue
    }

    if (separator && i + separator.length <= text.length && text.slice(i, i + separator.length).toUpperCase() === separator) {
      const prevOK = i === 0 || /\s/.test(text[i - 1])
      const nextIndex = i + separator.length
      const nextOK = nextIndex >= text.length || /\s/.test(text[nextIndex])
      if (prevOK && nextOK) {
        const part = current.trim()
        if (part) result.push(part)
        current = ''
        i = nextIndex - 1
        continue
      }
    }

    current += ch
  }

  const tail = current.trim()
  if (tail) result.push(tail)
  return result
}

export default function PoCManagerPage() {
  const { t } = useI18n()
  const router = useRouter()
  const fileInputRef = useRef<HTMLInputElement | null>(null)
  const syntaxInputRef = useRef<HTMLInputElement | null>(null)
  const [expandedDescriptions, setExpandedDescriptions] = useState<Record<string, boolean>>({})

  const [pocList, setPocList] = useState<PoCItem[]>([])
  const [loadingList, setLoadingList] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [debouncedKeyword, setDebouncedKeyword] = useState('')
  const [currentPage, setCurrentPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [total, setTotal] = useState(0)
  const [totalAll, setTotalAll] = useState(0)
  const [selectedVulnLabel, setSelectedVulnLabel] = useState('')
  const [vulnLabelGroups, setVulnLabelGroups] = useState<Array<{ label: string; count: number }>>([])
  const [showSyntaxSuggestions, setShowSyntaxSuggestions] = useState(false)
  const [selectedSuggestionIndex, setSelectedSuggestionIndex] = useState(-1)
  const [syntaxSuggestionStyle, setSyntaxSuggestionStyle] = useState<React.CSSProperties | null>(null)
  const [portalRoot, setPortalRoot] = useState<HTMLElement | null>(null)

  const [editorOpen, setEditorOpen] = useState(false)
  const [editingFileName, setEditingFileName] = useState('')
  const [editorContent, setEditorContent] = useState('')
  const [editorOriginalContent, setEditorOriginalContent] = useState('')
  const [loadingContent, setLoadingContent] = useState(false)
  const [savingContent, setSavingContent] = useState(false)
  const [openingDebugFileName, setOpeningDebugFileName] = useState('')

  const [importDialogOpen, setImportDialogOpen] = useState(false)
  const [importedPoc, setImportedPoc] = useState<{ filename: string; content: string } | null>(null)
  const [importedTags, setImportedTags] = useState<string[]>([])
  const [fingerprintNames, setFingerprintNames] = useState<string[]>([])
  const [loadingFingerprints, setLoadingFingerprints] = useState(false)
  const [savingImported, setSavingImported] = useState(false)
  const [templateFoldersOpen, setTemplateFoldersOpen] = useState(false)
  const [templateFolders, setTemplateFolders] = useState<string[]>([])
  const [newTemplateFolder, setNewTemplateFolder] = useState('')
  const [loadingTemplateFolders, setLoadingTemplateFolders] = useState(false)
  const [savingTemplateFolders, setSavingTemplateFolders] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<PoCItem | null>(null)
  const [deleting, setDeleting] = useState(false)
  const hotkeySaveRef = useRef<() => void>(() => {})
  const hasEditorChanges = editorContent !== editorOriginalContent

  const fingerprintOptions = useMemo<SelectTagOption[]>(() => {
    return fingerprintNames.map((name) => ({ value: name, label: name }))
  }, [fingerprintNames])

  useEffect(() => {
    if (typeof window === 'undefined') return
    const timer = window.setTimeout(() => {
      setPortalRoot(document.body)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [])

  const updateSyntaxSuggestionPosition = useCallback(() => {
    const input = syntaxInputRef.current
    if (!input || typeof window === 'undefined') return
    const rect = input.getBoundingClientRect()
    const viewportHeight = window.innerHeight
    const baseMaxHeight = 320
    const margin = 8
    const spaceBelow = viewportHeight - rect.bottom - margin
    const spaceAbove = rect.top - margin
    const openAbove = spaceBelow < 180 && spaceAbove > spaceBelow
    const availableHeight = Math.max(
      140,
      Math.min(baseMaxHeight, openAbove ? spaceAbove - margin : spaceBelow - margin),
    )
    const top = openAbove ? Math.max(margin, rect.top - availableHeight - 8) : rect.bottom + 8

    setSyntaxSuggestionStyle({
      position: 'fixed',
      left: rect.left,
      top,
      width: rect.width,
      maxHeight: availableHeight,
      zIndex: 80,
    })
  }, [])

  useEffect(() => {
    if (!showSyntaxSuggestions) return

    const handleReposition = () => updateSyntaxSuggestionPosition()
    handleReposition()
    window.addEventListener('resize', handleReposition)
    window.addEventListener('scroll', handleReposition, true)
    return () => {
      window.removeEventListener('resize', handleReposition)
      window.removeEventListener('scroll', handleReposition, true)
    }
  }, [showSyntaxSuggestions, updateSyntaxSuggestionPosition])

  const loadPoCs = useCallback(async (page: number, size: number, search: string, vulnLabel: string) => {
    setLoadingList(true)
    try {
      const query = new URLSearchParams({
        page: String(page),
        pageSize: String(size),
      })
      if (search) {
        query.set('keyword', search)
      }
      if (vulnLabel.trim()) {
        query.set('vulnLabel', vulnLabel.trim())
      }

      const response = await fetchWithAuth(`/api/v1/webscan/pocs?${query.toString()}`)
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'load pocs failed')
      }

      const payload: PoCListResponse = data.data || { items: [], total: 0, page, pageSize: size }
      setPocList(payload.items || [])
      setTotal(payload.total || 0)
      setTotalAll(payload.totalAll ?? payload.total ?? 0)
      setVulnLabelGroups(payload.vulnLabelGroups || [])
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.editor.loadFailed}: ${error?.message || ''}`,
      })
    } finally {
      setLoadingList(false)
    }
  }, [t])

  const loadTemplateFolders = useCallback(async () => {
    setLoadingTemplateFolders(true)
    try {
      const response = await fetchWithAuth('/api/v1/webscan/poc-template-folders')
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'load template folders failed')
      }
      setTemplateFolders(Array.isArray(data?.data?.folders) ? data.data.folders : [])
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.templateFolders.loadFailed}: ${error?.message || ''}`,
      })
      setTemplateFolders([])
    } finally {
      setLoadingTemplateFolders(false)
    }
  }, [t])

  const addTemplateFolder = useCallback(() => {
    const folder = newTemplateFolder.trim()
    if (!folder) return
    setTemplateFolders((prev) => {
      if (prev.includes(folder)) return prev
      return [...prev, folder]
    })
    setNewTemplateFolder('')
  }, [newTemplateFolder])

  const removeTemplateFolder = useCallback((folder: string) => {
    setTemplateFolders((prev) => prev.filter((item) => item !== folder))
  }, [])

  const saveTemplateFolders = useCallback(async () => {
    setSavingTemplateFolders(true)
    try {
      const folders = Array.from(new Set(templateFolders.map((item) => item.trim()).filter(Boolean)))
      const response = await fetchWithAuth('/api/v1/webscan/poc-template-folders', {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ folders }),
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'save template folders failed')
      }
      const savedFolders = Array.isArray(data?.data?.folders) ? data.data.folders : folders
      setTemplateFolders(savedFolders)
      toast.success({
        title: t.common.success,
        message: t.pocManager.templateFolders.saveSuccess,
      })
      setTemplateFoldersOpen(false)
      await loadPoCs(currentPage, pageSize, debouncedKeyword, selectedVulnLabel)
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.templateFolders.saveFailed}: ${error?.message || ''}`,
      })
    } finally {
      setSavingTemplateFolders(false)
    }
  }, [
    currentPage,
    debouncedKeyword,
    loadPoCs,
    pageSize,
    selectedVulnLabel,
    t,
    templateFolders,
  ])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedKeyword(keyword.trim())
    }, 300)
    return () => window.clearTimeout(timer)
  }, [keyword])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setCurrentPage(1)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [debouncedKeyword, selectedVulnLabel])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadPoCs(currentPage, pageSize, debouncedKeyword, selectedVulnLabel)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [currentPage, pageSize, debouncedKeyword, selectedVulnLabel, loadPoCs])

  const clauses = splitPoCSearchClauses(keyword, 'OR')
  const lastOrClause = clauses.length === 0 ? keyword.trim() : (clauses[clauses.length - 1] ?? '')
  const andClauses = splitPoCSearchClauses(lastOrClause, 'AND')
  const currentSearchClause = andClauses.length === 0 ? lastOrClause.trim() : (andClauses[andClauses.length - 1] ?? '')

  const searchSuggestions = useMemo<PoCSearchSuggestion[]>(() => {
    const texts = t.pocManager.filters.suggestions
    return [
      {
        label: texts.id.label,
        description: texts.id.description,
        insert: 'id:',
        keywords: ['id', 'cve', '模板id', 'template id'],
        example: 'id:CVE-2024-1234',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.name.label,
        description: texts.name.description,
        insert: 'name:',
        keywords: ['name', '名称'],
        example: 'name:Apache Tomcat',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.author.label,
        description: texts.author.description,
        insert: 'author:',
        keywords: ['author', '作者'],
        example: 'author:"dwisiswant0"',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.tag.label,
        description: texts.tag.description,
        insert: 'tag:',
        keywords: ['tag', 'tags', '标签'],
        example: 'tag:tomcat',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.severity.label,
        description: texts.severity.description,
        insert: 'severity:',
        keywords: ['severity', '风险', '等级'],
        example: 'severity:high',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.cwe.label,
        description: texts.cwe.description,
        insert: 'cwe:',
        keywords: ['cwe', '漏洞分类'],
        example: 'cwe:CWE-79',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.keyword.label,
        description: texts.keyword.description,
        insert: 'keyword:',
        keywords: ['keyword', '全文', 'full text'],
        example: 'keyword:principle-detection',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.principleDetection.label,
        description: texts.principleDetection.description,
        insert: 'principle-detection:',
        keywords: ['principle', 'principle-detection', 'metadata', '原理检测'],
        example: 'principle-detection:true',
        category: t.pocManager.filters.suggestionCategoryField,
      },
      {
        label: texts.principleTrue.label,
        description: texts.principleTrue.description,
        insert: 'principle-detection:true',
        keywords: ['true', 'principle true', '原理检测 true'],
        example: 'severity:high AND principle-detection:true',
        category: t.pocManager.filters.suggestionCategoryValue,
      },
      {
        label: texts.principleFalse.label,
        description: texts.principleFalse.description,
        insert: 'principle-detection:false',
        keywords: ['false', 'principle false', '原理检测 false'],
        example: 'tag:tomcat AND principle-detection:false',
        category: t.pocManager.filters.suggestionCategoryValue,
      },
      {
        label: texts.and.label,
        description: texts.and.description,
        insert: ' AND ',
        keywords: ['and', '并且', '组合'],
        example: texts.and.example,
        category: t.pocManager.filters.suggestionCategoryLogic,
      },
      {
        label: texts.or.label,
        description: texts.or.description,
        insert: ' OR ',
        keywords: ['or', '或者', '任一'],
        example: texts.or.example,
        category: t.pocManager.filters.suggestionCategoryLogic,
      },
    ]
  }, [t])

  const filteredSuggestions = useMemo(() => {
    const clause = currentSearchClause.trim().toLowerCase()
    const normalizedKeyword = keyword.trim().toLowerCase()

    if (
      clause.startsWith('principle-detection:') ||
      clause.startsWith('info.metadata.principle-detection:') ||
      normalizedKeyword.endsWith('principle-detection:') ||
      normalizedKeyword.endsWith('info.metadata.principle-detection:')
    ) {
      return searchSuggestions.filter((item) => item.insert === 'principle-detection:true' || item.insert === 'principle-detection:false')
    }

    if (!clause) {
      return searchSuggestions.slice(0, 8)
    }

    return searchSuggestions.filter((item) => (
      item.insert.toLowerCase().includes(clause) ||
      item.label.toLowerCase().includes(clause) ||
      item.keywords.some((value) => value.toLowerCase().includes(clause))
    )).slice(0, 8)
  }, [currentSearchClause, keyword, searchSuggestions])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSelectedSuggestionIndex(filteredSuggestions.length > 0 ? 0 : -1)
    }, 0)

    return () => window.clearTimeout(timer)
  }, [filteredSuggestions])

  const applySuggestion = useCallback((insert: string) => {
    const trimmedInsert = insert.trim()
    const upperInsert = trimmedInsert.toUpperCase()
    const normalizedKeyword = keyword.replace(/\s+$/, '')

    if (upperInsert === 'AND' || upperInsert === 'OR') {
      const nextValue = normalizedKeyword ? `${normalizedKeyword} ${upperInsert} ` : `${upperInsert} `
      setKeyword(nextValue)
      setShowSyntaxSuggestions(true)
      setTimeout(() => {
        syntaxInputRef.current?.focus()
        updateSyntaxSuggestionPosition()
      }, 0)
      return
    }

    let nextValue = keyword
    const currentClause = currentSearchClause
    if (!currentClause.trim()) {
      nextValue = keyword && !/\s$/.test(keyword) ? `${keyword} ${insert}` : `${keyword}${insert}`
    } else {
      const clauseIndex = keyword.lastIndexOf(currentClause)
      if (clauseIndex >= 0) {
        nextValue = `${keyword.slice(0, clauseIndex)}${insert}${keyword.slice(clauseIndex + currentClause.length)}`
      } else {
        nextValue = keyword && !/\s$/.test(keyword) ? `${keyword} ${insert}` : `${keyword}${insert}`
      }
    }

    setKeyword(nextValue)
    setShowSyntaxSuggestions(true)
    setTimeout(() => {
      syntaxInputRef.current?.focus()
      updateSyntaxSuggestionPosition()
    }, 0)
  }, [currentSearchClause, keyword, updateSyntaxSuggestionPosition])

  const handleSyntaxKeyDown = useCallback((event: React.KeyboardEvent<HTMLInputElement>) => {
    const canUseSuggestions = showSyntaxSuggestions && filteredSuggestions.length > 0
    if (event.key === 'ArrowDown') {
      if (!canUseSuggestions) return
      event.preventDefault()
      setSelectedSuggestionIndex((prev) => (prev < filteredSuggestions.length - 1 ? prev + 1 : prev))
      return
    }
    if (event.key === 'ArrowUp') {
      if (!canUseSuggestions) return
      event.preventDefault()
      setSelectedSuggestionIndex((prev) => (prev > 0 ? prev - 1 : 0))
      return
    }
    if (event.key === 'Enter') {
      if (canUseSuggestions && selectedSuggestionIndex >= 0) {
        event.preventDefault()
        applySuggestion(filteredSuggestions[selectedSuggestionIndex].insert)
        return
      }
      setShowSyntaxSuggestions(false)
      return
    }
    if (event.key === 'Escape') {
      setShowSyntaxSuggestions(false)
    }
  }, [applySuggestion, filteredSuggestions, selectedSuggestionIndex, showSyntaxSuggestions])

  const openEditor = useCallback(async (fileName: string) => {
    setEditingFileName(fileName)
    setEditorOpen(true)
    setLoadingContent(true)
    setEditorContent('')
    setEditorOriginalContent('')

    try {
      const response = await fetchWithAuth(`/api/v1/webscan/pocs/${encodeURIComponent(fileName)}`)
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'read poc failed')
      }
      const content = data?.data?.content || ''
      setEditorContent(content)
      setEditorOriginalContent(content)
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.editor.loadFailed}: ${error?.message || ''}`,
      })
      setEditorOpen(false)
    } finally {
      setLoadingContent(false)
    }
  }, [t])

  const openDebugWorkspace = useCallback(async (item: PoCItem) => {
    setOpeningDebugFileName(item.fileName)
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/pocs/${encodeURIComponent(item.fileName)}`)
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'read poc failed')
      }

      const content = data?.data?.content || ''
      const payload: NucleiTemplateWorkspacePayload = {
        draft: createNucleiTemplateDraftFromYaml(content),
        yaml: content,
        response: '',
        target: '',
        source: 'poc',
        sourceTabName: item.fileName,
        returnPath: '/poc-manager',
        initialView: 'debug',
        createdAt: Date.now(),
      }
      queueNucleiTemplateImport(payload)
      router.push('/poc/nuclei-template?source=poc')
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.editor.loadFailed}: ${error?.message || ''}`,
      })
    } finally {
      setOpeningDebugFileName('')
    }
  }, [router, t])

  const handleSavePoC = useCallback(async () => {
    if (!editingFileName) return

    setSavingContent(true)
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/pocs/${encodeURIComponent(editingFileName)}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          content: editorContent,
        }),
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'save poc failed')
      }

      toast.success({
        title: t.common.success,
        message: t.pocManager.editor.saveSuccess,
      })
      setEditorOriginalContent(editorContent)
      await loadPoCs(currentPage, pageSize, debouncedKeyword, selectedVulnLabel)
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.editor.saveFailed}: ${error?.message || ''}`,
      })
    } finally {
      setSavingContent(false)
    }
  }, [
    currentPage,
    debouncedKeyword,
    editingFileName,
    editorContent,
    loadPoCs,
    pageSize,
    selectedVulnLabel,
    t,
  ])

  const handleEditorChange = useCallback((value: string) => {
    setEditorContent(value)
  }, [])

  const handleCopyPoCContent = useCallback(async () => {
    if (!editorContent.trim()) {
      toast.error({
        title: t.common.error,
        message: t.pocManager.editor.copyEmpty,
      })
      return
    }
    const success = await copyToClipboard(editorContent)
    if (success) {
      toast.success({
        title: t.common.success,
        message: t.common.copiedToClipboard,
      })
      return
    }
    toast.error({
      title: t.common.error,
      message: t.pocManager.editor.copyFailed,
    })
  }, [editorContent, t])

  useEffect(() => {
    hotkeySaveRef.current = () => {
      if (!editorOpen || loadingContent || savingContent || !hasEditorChanges) return
      void handleSavePoC()
    }
  }, [editorOpen, loadingContent, savingContent, hasEditorChanges, handleSavePoC])

  useEffect(() => {
    if (!editorOpen) return
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
        event.preventDefault()
        hotkeySaveRef.current()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [editorOpen])

  const handleEditorMount = useCallback((editorInstance: any, monacoInstance: any) => {
    if (!editorInstance?.addCommand || !monacoInstance?.KeyMod || !monacoInstance?.KeyCode) return
    editorInstance.addCommand(monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.KeyS, () => {
      hotkeySaveRef.current()
    })
  }, [])

  const loadFingerprintNames = useCallback(async () => {
    setLoadingFingerprints(true)
    try {
      const response = await fetchWithAuth('/api/fingerprint/list')
      const data = await response.json().catch(() => null)
      if (!response.ok || !data || typeof data !== 'object') {
        throw new Error('load fingerprints failed')
      }
      const payload = (data as Record<string, any>).data && typeof (data as Record<string, any>).data === 'object'
        ? (data as Record<string, any>).data
        : data
      const nameSet = new Set<string>()
      Object.values(payload as Record<string, any[]>).forEach((group) => {
        if (!Array.isArray(group)) return
        group.forEach((item) => {
          const name = typeof item === 'string'
            ? item.trim()
            : typeof item?.Name === 'string'
              ? item.Name.trim()
              : typeof item?.name === 'string'
                ? item.name.trim()
                : ''
          if (name) {
            nameSet.add(name)
          }
        })
      })
      setFingerprintNames(Array.from(nameSet).sort((a, b) => a.localeCompare(b)))
    } catch {
      toast.error({
        title: t.common.error,
        message: t.pocManager.loadFingerprintsFailed,
      })
      setFingerprintNames([])
    } finally {
      setLoadingFingerprints(false)
    }
  }, [t])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadFingerprintNames()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadFingerprintNames])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadTemplateFolders()
    }, 0)

    return () => window.clearTimeout(timer)
  }, [loadTemplateFolders])

  const handleSelectImportFile = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file) return

    try {
      const content = await file.text()
      if (!content.includes('id:') || !content.includes('info:')) {
        toast.error({
          title: t.common.error,
          message: t.pocManager.invalidYaml,
        })
        return
      }
      setImportedPoc({ filename: file.name, content })
      setImportedTags([])
      setImportDialogOpen(true)
    } catch {
      toast.error({
        title: t.common.error,
        message: t.pocManager.importFailed,
      })
    } finally {
      event.target.value = ''
    }
  }

  const upsertInfoTags = (content: string, tags: string[]) => {
    if (tags.length === 0) return content
    const tagsLine = `  tags: ${tags.join(',')}`
    const lines = content.split('\n')
    const infoIndex = lines.findIndex((line) => line.trim() === 'info:')
    if (infoIndex === -1) return content

    let endIndex = lines.length
    for (let i = infoIndex + 1; i < lines.length; i += 1) {
      if (/^\S/.test(lines[i])) {
        endIndex = i
        break
      }
    }

    let tagLineIndex = -1
    for (let i = infoIndex + 1; i < endIndex; i += 1) {
      if (/^\s+tags\s*:/.test(lines[i])) {
        tagLineIndex = i
        break
      }
    }

    if (tagLineIndex >= 0) {
      lines[tagLineIndex] = tagsLine
    } else {
      lines.splice(endIndex, 0, tagsLine)
    }
    return lines.join('\n')
  }

  const saveImportedPoc = async () => {
    if (!importedPoc) return
    const pocID = normalizePoCId(importedPoc.filename)
    if (!pocID || pocID.includes('/') || pocID.includes('\\')) {
      toast.error({
        title: t.common.error,
        message: t.pocManager.editor.createInvalidId,
      })
      return
    }

    setSavingImported(true)
    try {
      const finalContent = upsertInfoTags(importedPoc.content, importedTags)
      const response = await fetchWithAuth(`/api/v1/webscan/pocs/${encodeURIComponent(pocID)}`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({ content: finalContent }),
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'save imported poc failed')
      }

      toast.success({
        title: t.common.success,
        message: t.pocManager.importSuccess.replace('{count}', '1'),
      })
      setImportDialogOpen(false)
      setImportedPoc(null)
      setImportedTags([])
      setCurrentPage(1)
      await loadPoCs(1, pageSize, debouncedKeyword, selectedVulnLabel)
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.importFailed}: ${error?.message || ''}`,
      })
    } finally {
      setSavingImported(false)
    }
  }

  const handleConfirmDelete = async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      const response = await fetchWithAuth(`/api/v1/webscan/pocs/${encodeURIComponent(deleteTarget.fileName)}`, {
        method: 'DELETE',
      })
      const data = await response.json().catch(() => ({}))
      if (!response.ok || !data.success) {
        throw new Error(data.error || 'delete poc failed')
      }
      toast.success({
        title: t.common.success,
        message: t.pocManager.editor.deleteSuccess,
      })
      setDeleteTarget(null)

      const nextPage = currentPage > 1 && pocList.length === 1 ? currentPage - 1 : currentPage
      setCurrentPage(nextPage)
      await loadPoCs(nextPage, pageSize, debouncedKeyword, selectedVulnLabel)
    } catch (error: any) {
      toast.error({
        title: t.common.error,
        message: `${t.pocManager.editor.deleteFailed}: ${error?.message || ''}`,
      })
    } finally {
      setDeleting(false)
    }
  }

  const toggleDescription = (fileName: string) => {
    setExpandedDescriptions((prev) => ({
      ...prev,
      [fileName]: !prev[fileName],
    }))
  }

  const columns: ColumnDef<PoCItem>[] = useMemo(() => [
    {
      id: 'id',
      header: t.pocManager.table.id,
      headerClassName: 'w-[180px] pl-6',
      className: 'w-[180px] max-w-[180px] pl-6 text-foreground/90',
      cell: (item) => (
        <span className="block truncate" title={item.id || '-'}>
          {item.id || '-'}
        </span>
      ),
    },
    {
      id: 'severity',
      header: t.pocManager.table.severity,
      headerClassName: 'w-[130px]',
      className: 'w-[130px]',
      cell: (item) => (
        <Badge variant="outline" className={severityClassName(item.severity || 'info')}>
          {(item.severity || 'INFO').toUpperCase()}
        </Badge>
      ),
    },
    {
      id: 'name',
      header: t.pocManager.table.name,
      className: 'min-w-[220px] whitespace-normal',
      cell: (item) => (
        <div className="space-y-1 py-1 max-w-full">
          <div className="text-sm font-medium break-words line-clamp-1">{item.name || item.fileName}</div>
          {item.author && (
            <div className="text-[11px]">
              <Badge
                variant="outline"
                className="max-w-[180px] justify-start text-left bg-tone-blue-soft border-tone-blue-border text-tone-blue"
                title={item.author}
              >
                <span className="block min-w-0 truncate">@{item.author}</span>
              </Badge>
            </div>
          )}
          <div className="space-y-1">
            <p className={`${expandedDescriptions[item.fileName] ? '' : 'line-clamp-2'} break-words text-xs text-muted-foreground`}>
              {item.description || '-'}
            </p>
            {(item.description || '').length > 400 && (
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0 text-xs text-primary"
                onClick={(event) => {
                  event.stopPropagation()
                  toggleDescription(item.fileName)
                }}
              >
                {expandedDescriptions[item.fileName] ? t.pocManager.collapse : t.pocManager.expand}
              </Button>
            )}
          </div>
        </div>
      ),
    },
    {
      id: 'relatedFingerprint',
      header: t.pocManager.table.relatedFingerprint,
      headerClassName: 'w-[200px]',
      className: 'w-[200px] max-w-[200px]',
      cell: (item) => (
        <div className="flex flex-wrap gap-1">
          {(item.tags || []).slice(0, 3).map((tag) => (
            <Tooltip key={`${item.fileName}-${tag}`}>
              <TooltipTrigger asChild>
                <Badge
                  variant="outline"
                  className="max-w-[170px] justify-start text-left bg-tone-blue-soft border-tone-blue-border text-tone-blue"
                >
                  <span className="block min-w-0 truncate">{tag}</span>
                </Badge>
              </TooltipTrigger>
              <TooltipContent sideOffset={6}>{tag}</TooltipContent>
            </Tooltip>
          ))}
          {(item.tags || []).length > 3 && (
            <Badge variant="outline" className="bg-muted text-muted-foreground">
              +{(item.tags || []).length - 3}
            </Badge>
          )}
          {(item.tags || []).length === 0 && (item.relatedFingerprint ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Badge
                  variant="outline"
                  className="max-w-[170px] justify-start text-left bg-tone-blue-soft border-tone-blue-border text-tone-blue"
                >
                  <span className="block min-w-0 truncate">{item.relatedFingerprint}</span>
                </Badge>
              </TooltipTrigger>
              <TooltipContent sideOffset={6}>{item.relatedFingerprint}</TooltipContent>
            </Tooltip>
          ) : '-')}
        </div>
      ),
    },
    /*
    {
      id: 'detectionMethod',
      header: t.pocManager.table.detectionMethod,
      headerClassName: 'w-[90px] text-center',
      className: 'w-[90px] text-center',
      cell: (item) => (
        <div className="flex flex-col items-center gap-1">
          {item.detectionMethod === 'principle' ? (
            <Badge
              variant="outline"
              className="bg-tone-amber-soft border-tone-amber-border text-tone-amber"
            >
              {t.pocManager.methods.principle}
            </Badge>
          ) : (
            <Badge
              variant="outline"
              className="bg-tone-green-soft border-tone-green-border text-tone-green"
            >
              {t.pocManager.methods.exploit}
            </Badge>
          )}
          {item.dast && (
            <Badge
              variant="outline"
              className="bg-tone-blue-soft border-tone-blue-border text-tone-blue"
            >
              {t.pocManager.methods.allTargets}
            </Badge>
          )}
        </div>
      ),
    },
    {
      id: 'vulnerabilityCategory',
      header: t.pocManager.table.vulnerabilityCategory,
      headerClassName: 'w-[110px] text-center',
      className: 'w-[110px] text-center',
      cell: (item) => {
        const cweList = (item.vulnerabilityCategory || '')
          .split(',')
          .map((it) => it.trim())
          .filter(Boolean)
          .slice(0, 2)
        if (cweList.length === 0) return '-'
        return (
          <div className="flex flex-col items-center gap-1">
            {cweList.map((cwe) => {
              const cweEntry = getCWEEntry(cwe)
              return (
                <Tooltip key={`${item.fileName}-${cwe}`}>
                  <TooltipTrigger asChild>
                    <Badge
                      variant="outline"
                      className="max-w-[90px] cursor-help justify-start text-left bg-tone-amber-soft border-tone-amber-border text-tone-amber"
                    >
                      <span className="block min-w-0 truncate">{cwe}</span>
                    </Badge>
                  </TooltipTrigger>
                  <TooltipContent side="top" className="max-w-[260px] text-left">
                    <div className="space-y-1">
                      <div className="font-medium text-popover-foreground">{cwe}</div>
                      <div className="text-muted-foreground">
                        {cweEntry ? getCWEName(cwe, language) : '暂无内置名称'}
                      </div>
                    </div>
                  </TooltipContent>
                </Tooltip>
              )
            })}
          </div>
        )
      },
    },
    */
    {
      id: 'modifiedTime',
      header: t.pocManager.table.modifiedTime,
      headerClassName: 'w-[140px]',
      className: 'w-[140px] whitespace-nowrap',
      cell: (item) => {
        const createdTime = item.createdTime || '-'
        const updatedTime = item.updatedTime || item.modifiedTime || '-'
        return (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex cursor-help items-center">
                {formatPoCDate(updatedTime)}
              </span>
            </TooltipTrigger>
            <TooltipContent side="top" sideOffset={6} className="text-left">
              <div className="space-y-1 whitespace-nowrap">
                <div>{t.pocManager.table.createdTime}: {createdTime}</div>
                <div>{t.pocManager.table.updatedTime}: {updatedTime}</div>
              </div>
            </TooltipContent>
          </Tooltip>
        )
      },
    },
    {
      id: 'actions',
      header: t.pocManager.table.actions,
      headerClassName: 'min-w-[112px] text-center sticky right-0 z-20 border-l border-border bg-card',
      className: 'min-w-[112px] text-center sticky right-0 z-10 border-l border-border bg-card group-hover:bg-muted group-data-[state=selected]:bg-muted',
      alwaysVisible: true,
      cell: (item) => (
        <div className="flex items-center justify-center gap-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={(event) => {
                  event.stopPropagation()
                  void openDebugWorkspace(item)
                }}
                disabled={openingDebugFileName === item.fileName}
                aria-label={t.pocManager.actions.debug}
              >
                {openingDebugFileName === item.fileName ? (
                  <Loader2 className="w-4 h-4 animate-spin" />
                ) : (
                  <Bug className="w-4 h-4" />
                )}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t.pocManager.actions.debug}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                onClick={(event) => {
                  event.stopPropagation()
                  setDeleteTarget(item)
                }}
                aria-label={t.pocManager.actions.delete}
              >
                <Trash2 className="w-4 h-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t.pocManager.actions.delete}</TooltipContent>
          </Tooltip>
        </div>
      ),
    },
  ], [expandedDescriptions, openDebugWorkspace, openingDebugFileName, t])

  return (
    <div className="flex-1 overflow-y-auto">
      <div className="p-3 md:p-6 space-y-4">
        <div className="rounded-lg border bg-card p-4 space-y-4">
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={selectedVulnLabel || '__all__'}
              onValueChange={(value) => setSelectedVulnLabel(value === '__all__' ? '' : value)}
            >
              <SelectTrigger className="w-[180px]">
                <SelectValue placeholder={t.pocManager.table.vulnLabels} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="__all__">
                  {t.pocManager.table.vulnLabels} ({totalAll})
                </SelectItem>
                {vulnLabelGroups.map((group) => (
                  <SelectItem key={group.label} value={group.label}>
                    {group.label} ({group.count})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            <div className="relative min-w-0 basis-full flex-1 md:basis-auto">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                ref={syntaxInputRef}
                value={keyword}
                onChange={(event) => {
                  setKeyword(event.target.value)
                  setShowSyntaxSuggestions(true)
                  updateSyntaxSuggestionPosition()
                }}
                onFocus={() => {
                  setShowSyntaxSuggestions(true)
                  updateSyntaxSuggestionPosition()
                }}
                onBlur={() => {
                  window.setTimeout(() => setShowSyntaxSuggestions(false), 180)
                }}
                onKeyDown={handleSyntaxKeyDown}
                placeholder={t.pocManager.filters.searchPlaceholder}
                className="pl-9"
              />
            </div>

            <Button
              variant="outline"
              onClick={() => void loadPoCs(currentPage, pageSize, debouncedKeyword, selectedVulnLabel)}
              disabled={loadingList}
            >
              <RefreshCw className={`h-4 w-4 mr-2 ${loadingList ? 'animate-spin' : ''}`} />
              {t.pocManager.refresh}
            </Button>

            <Button variant="outline" onClick={() => setTemplateFoldersOpen(true)}>
              <FolderPlus className="h-4 w-4 mr-2" />
              {t.pocManager.templateFolders.button}
            </Button>

            <Button variant="outline" onClick={() => fileInputRef.current?.click()}>
              <Upload className="h-4 w-4 mr-2" />
              {t.pocManager.importPoc}
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              className="hidden"
              accept=".yaml,.yml"
              onChange={(event) => void handleSelectImportFile(event)}
            />
          </div>

          <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span className="shrink-0">{t.pocManager.filters.searchSyntaxHint}</span>
            {t.pocManager.filters.searchSyntaxExamples.map((example) => (
              <button
                key={example.value}
                type="button"
                className="rounded border border-border bg-muted/30 px-2 py-1 font-mono text-[11px] leading-none text-foreground/80 transition-colors hover:border-primary/40 hover:bg-primary/10 hover:text-primary"
                onClick={() => {
                  setKeyword(example.value)
                  setShowSyntaxSuggestions(false)
                  syntaxInputRef.current?.focus()
                }}
              >
                {example.value}
              </button>
            ))}
          </div>
          {showSyntaxSuggestions && filteredSuggestions.length > 0 && portalRoot && syntaxSuggestionStyle && createPortal(
            <div
              style={syntaxSuggestionStyle}
              className="rounded-lg border bg-popover/95 backdrop-blur shadow-lg overflow-y-auto"
            >
              {filteredSuggestions.map((item, index) => (
                <button
                  key={`${item.insert}-${index}`}
                  type="button"
                  onMouseDown={(event) => {
                    event.preventDefault()
                    applySuggestion(item.insert)
                  }}
                  className={`w-full text-left px-3 py-2.5 border-b last:border-b-0 transition-colors ${selectedSuggestionIndex === index ? 'bg-muted' : 'hover:bg-muted/70'}`}
                >
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline" className="h-5 px-2 text-[10px]">
                      {item.category}
                    </Badge>
                    <div className="text-sm font-medium">{item.label}</div>
                  </div>
                  <div className="text-xs font-mono text-primary mt-1">{item.insert}</div>
                  <div className="text-[11px] text-muted-foreground mt-1">{item.description}</div>
                  {item.example ? (
                    <div className="text-[11px] text-muted-foreground mt-1">
                      {t.pocManager.filters.suggestionExamplePrefix}: <span className="font-mono">{item.example}</span>
                    </div>
                  ) : null}
                </button>
              ))}
            </div>,
            portalRoot,
          )}

          <div className="min-w-0 space-y-4">
            <DataTableWithColumns
              mobileCards
              columns={columns}
              data={pocList}
              loading={loadingList}
              emptyMessage={t.pocManager.empty}
              showColumnToggle={false}
              showToolbar={false}
              getRowKey={(item) => item.fileName}
              renderRow={(item, _index, defaultRow) => {
                if (!isValidElement<{ className?: string; onClick?: () => void }>(defaultRow)) {
                  return defaultRow
                }
                return cloneElement(defaultRow, {
                  className: `${defaultRow.props.className || ''} cursor-pointer`,
                  onClick: () => void openEditor(item.fileName),
                })
              }}
            />

            <PaginationPro
              current={currentPage}
              total={total}
              pageSize={pageSize}
              onChange={(page, size) => {
                setCurrentPage(page)
                setPageSize(size)
              }}
              onPageSizeChange={(size) => {
                setPageSize(size)
                setCurrentPage(1)
              }}
              pageSizeOptions={[10, 20, 50, 100]}
              showSizeChanger
              showQuickJumper
              showTotal
            />
          </div>
        </div>
      </div>

      <Dialog open={templateFoldersOpen} onOpenChange={setTemplateFoldersOpen}>
        <DialogContent className="w-[95vw] max-w-[95vw] md:w-[680px] md:max-w-[680px]">
          <DialogHeader>
            <DialogTitle>{t.pocManager.templateFolders.title}</DialogTitle>
            <DialogDescription>{t.pocManager.templateFolders.description}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="flex gap-2">
              <Input
                value={newTemplateFolder}
                onChange={(event) => setNewTemplateFolder(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    addTemplateFolder()
                  }
                }}
                placeholder={t.pocManager.templateFolders.placeholder}
              />
              <Button type="button" variant="outline" onClick={addTemplateFolder}>
                <FolderPlus className="h-4 w-4 mr-2" />
                {t.pocManager.templateFolders.add}
              </Button>
            </div>

            <div className="max-h-[320px] overflow-y-auto rounded-md border">
              {loadingTemplateFolders ? (
                <div className="flex h-24 items-center justify-center text-sm text-muted-foreground">
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  {t.common.loading}
                </div>
              ) : templateFolders.length === 0 ? (
                <div className="flex h-24 items-center justify-center text-sm text-muted-foreground">
                  {t.pocManager.templateFolders.empty}
                </div>
              ) : (
                <div className="divide-y">
                  {templateFolders.map((folder) => (
                    <div key={folder} className="flex items-center gap-3 px-3 py-2">
                      <div className="min-w-0 flex-1 truncate font-mono text-xs" title={folder}>
                        {folder}
                      </div>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="h-8 w-8 shrink-0"
                        onClick={() => removeTemplateFolder(folder)}
                        aria-label={t.pocManager.templateFolders.remove}
                      >
                        <X className="h-4 w-4" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setTemplateFoldersOpen(false)} disabled={savingTemplateFolders}>
              {t.common.cancel}
            </Button>
            <Button onClick={() => void saveTemplateFolders()} disabled={savingTemplateFolders || loadingTemplateFolders}>
              {savingTemplateFolders && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {t.pocManager.templateFolders.save}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={importDialogOpen} onOpenChange={setImportDialogOpen}>
        <DialogContent className="w-[95vw] max-w-[95vw] md:w-[60vw] md:max-w-[60vw]">
          <DialogHeader>
            <DialogTitle>{t.pocManager.importDialogTitle}</DialogTitle>
            <DialogDescription>{t.pocManager.importDialogDescription}</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="space-y-2">
              <Label>{t.pocManager.fileName}</Label>
              <div className="h-10 rounded-md border px-3 flex items-center text-sm text-muted-foreground bg-muted/20">
                {importedPoc?.filename || '-'}
              </div>
            </div>

            <div className="space-y-2">
              <Label>{t.pocManager.fingerprints}</Label>
              <SelectTag
                options={fingerprintOptions}
                value={importedTags}
                onChange={setImportedTags}
                placeholder={t.pocManager.fingerprintsPlaceholder}
                disabled={loadingFingerprints}
              />
              {fingerprintNames.length === 0 && (
                <p className="text-xs text-muted-foreground">{t.pocManager.fingerprintsEmptyHint}</p>
              )}
            </div>

            <div className="space-y-2">
              <Label>{t.pocManager.preview}</Label>
              <div className="h-[240px] rounded-md border overflow-hidden">
                <MonacoEditorWrapper
                  language="yaml"
                  value={importedPoc?.content || ''}
                  readOnly
                  height="100%"
                />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setImportDialogOpen(false)}>
              {t.common.cancel}
            </Button>
            <Button onClick={() => void saveImportedPoc()} disabled={!importedPoc || savingImported}>
              {savingImported && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {t.pocManager.saveImport}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Sheet open={editorOpen} onOpenChange={(open) => !savingContent && setEditorOpen(open)}>
        <SheetContent side="right" className="w-[50vw] sm:max-w-none p-0 gap-0">
          <SheetHeader className="border-b">
            <SheetTitle>{t.pocManager.editor.titlePrefix}: {editingFileName}</SheetTitle>
            <SheetDescription>{editingFileName}.yaml</SheetDescription>
          </SheetHeader>
          <div className="px-4 py-2 text-xs border-b text-muted-foreground flex items-center justify-between">
            <span>{hasEditorChanges ? t.pocManager.editor.unsavedChanges : t.pocManager.editor.noChanges}</span>
            <span>{t.pocManager.editor.saveShortcut}</span>
          </div>

          <div className="flex-1 min-h-0 overflow-hidden p-4">
            {loadingContent ? (
              <div className="h-full flex items-center justify-center text-muted-foreground border rounded-md">
                <Loader2 className="h-4 w-4 mr-2 animate-spin" />
                {t.common.loading}
              </div>
            ) : (
              <div className="h-full border rounded-md overflow-hidden">
                <MonacoEditorWrapper
                  language="yaml"
                  value={editorContent}
                  onChange={handleEditorChange}
                  onMount={handleEditorMount}
                  height="100%"
                />
              </div>
            )}
          </div>

          <SheetFooter className="border-t flex-row justify-end">
            <Button variant="outline" onClick={() => void handleCopyPoCContent()} disabled={loadingContent}>
              <Copy className="h-4 w-4 mr-2" />
              {t.common.copy}
            </Button>
            <Button variant="outline" onClick={() => setEditorOpen(false)} disabled={savingContent}>
              {t.common.cancel}
            </Button>
            <Button onClick={() => void handleSavePoC()} disabled={savingContent || loadingContent || !hasEditorChanges}>
              {savingContent && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {t.pocManager.actions.save}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t.pocManager.deleteConfirm.title}</AlertDialogTitle>
            <AlertDialogDescription>
              {t.pocManager.deleteConfirm.description.replace('{name}', deleteTarget?.fileName || '')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleting}>{t.common.cancel}</AlertDialogCancel>
            <AlertDialogAction onClick={() => void handleConfirmDelete()} disabled={deleting}>
              {deleting && <Loader2 className="h-4 w-4 mr-2 animate-spin" />}
              {t.common.delete}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
