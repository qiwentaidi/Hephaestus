"use client"

export type ResponseSelectionPart = "header" | "body"

export interface ResponseSelectionSnapshot {
  text: string
  part: ResponseSelectionPart
  startOffset: number
  endOffset: number
}

export type NucleiSeverity = "info" | "low" | "medium" | "high" | "critical"

const nucleiSeverities = new Set<NucleiSeverity>(["info", "low", "medium", "high", "critical"])
const nucleiTemplateRootKeys = new Set([
  "id",
  "info",
  "variables",
  "flow",
  "http",
  "dns",
  "ssl",
  "file",
  "headless",
  "javascript",
  "code",
  "self-contained",
  "stop-at-first-match",
])

export interface NucleiTemplateMatcherDraft {
  type: "status" | "word"
  part?: ResponseSelectionPart
  status?: number[]
  words?: string[]
  condition?: "and" | "or"
  summary: string
}

export interface NucleiTemplateDraft {
  id: string
  name: string
  author: string
  severity: NucleiSeverity
  description: string
  impact: string
  remediation: string
  rawRequest: string
  matchersCondition: "and"
  matchers: NucleiTemplateMatcherDraft[]
}

export interface NucleiTemplateWorkspacePayload {
  draft: NucleiTemplateDraft
  yaml: string
  response: string
  target: string
  source?: "repeater" | "poc"
  sourceTabName?: string
  returnPath?: string
  initialView?: "template" | "debug"
  createdAt: number
  importId?: string
}

export const NUCLEI_TEMPLATE_WORKSPACE_STORAGE_KEY = "hephaestus:repeater:nuclei-template-workspace"

// A pending import represents one explicit click, not a reusable workspace snapshot.
export function queueNucleiTemplateImport(payload: NucleiTemplateWorkspacePayload): void {
  const pending = {
    ...payload,
    importId: `${Date.now()}-${Math.random().toString(36).slice(2)}`,
  }
  window.sessionStorage.setItem(NUCLEI_TEMPLATE_WORKSPACE_STORAGE_KEY, JSON.stringify(pending))
  window.dispatchEvent(new CustomEvent("hephaestus:nuclei-template-workspace-open"))
}

export function readPendingNucleiTemplateImport(): NucleiTemplateWorkspacePayload | null {
  const raw = window.sessionStorage.getItem(NUCLEI_TEMPLATE_WORKSPACE_STORAGE_KEY)
  if (!raw) return null
  try {
    const payload = JSON.parse(raw)
    // Old versions left already-imported snapshots here indefinitely. Do not replay them.
    if (typeof payload?.importId === "string" && payload.importId && payload.draft && typeof payload.yaml === "string") {
      return payload
    }
  } catch {
    // Discard malformed pending data without affecting saved workspace tabs.
  }
  window.sessionStorage.removeItem(NUCLEI_TEMPLATE_WORKSPACE_STORAGE_KEY)
  return null
}

export function consumeNucleiTemplateImport(payload: NucleiTemplateWorkspacePayload | null): boolean {
  if (!payload?.importId) return false
  const pending = readPendingNucleiTemplateImport()
  if (pending?.importId !== payload.importId) return false
  // Consume only when the workspace is ready to append, not when a route reads it.
  // This also prevents cached props and duplicate events from replaying an import.
  window.sessionStorage.removeItem(NUCLEI_TEMPLATE_WORKSPACE_STORAGE_KEY)
  return true
}

interface ParsedResponse {
  statusCode: number | null
}

const MAX_WORD_LENGTH = 180
const MAX_SELECTION_WORDS = 3

function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
}

function splitHttpMessage(content: string): { head: string; body: string } {
  const normalized = content.replace(/\r\n/g, "\n")
  const separatorIndex = normalized.indexOf("\n\n")
  if (separatorIndex === -1) {
    return { head: normalized, body: "" }
  }
  return {
    head: normalized.slice(0, separatorIndex),
    body: normalized.slice(separatorIndex + 2),
  }
}

function parseResponse(content: string): ParsedResponse {
  const { head } = splitHttpMessage(content)
  const headerLines = head.split("\n").filter(Boolean)
  const statusMatch = headerLines[0]?.match(/^HTTP\/[\d.]+\s+(\d+)/i)
  return {
    statusCode: statusMatch ? Number.parseInt(statusMatch[1], 10) : null,
  }
}

function normalizeSelectedWords(text: string): string[] {
  const normalized = text.replace(/\r\n/g, "\n").trim()
  if (!normalized) {
    return []
  }

  const words = normalized
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => (line.length > MAX_WORD_LENGTH ? line.slice(0, MAX_WORD_LENGTH) : line))

  const uniqueWords: string[] = []
  for (const word of words) {
    if (!uniqueWords.includes(word)) {
      uniqueWords.push(word)
    }
    if (uniqueWords.length >= MAX_SELECTION_WORDS) {
      break
    }
  }

  if (uniqueWords.length > 0) {
    return uniqueWords
  }

  return [normalized.slice(0, MAX_WORD_LENGTH)]
}

export function createWordMatcherFromSelection(selection: ResponseSelectionSnapshot): NucleiTemplateMatcherDraft | null {
  const selectionWords = normalizeSelectedWords(selection.text)
  if (selectionWords.length === 0) {
    return null
  }

  return {
    type: "word",
    part: selection.part,
    words: selectionWords,
    summary: `${selection.part} contains selected content`,
  }
}

function isSameWordMatcher(left: NucleiTemplateMatcherDraft, right: NucleiTemplateMatcherDraft): boolean {
  if (left.type !== "word" || right.type !== "word") {
    return false
  }

  return left.part === right.part && (left.words || []).join("\x00") === (right.words || []).join("\x00")
}

function mergeWordMatchersByPart(matchers: NucleiTemplateMatcherDraft[]): NucleiTemplateMatcherDraft[] {
  const mergedMatchers: NucleiTemplateMatcherDraft[] = []
  const wordMatcherByPart = new Map<ResponseSelectionPart, NucleiTemplateMatcherDraft>()

  for (const matcher of matchers) {
    if (matcher.type !== "word" || !matcher.part || !matcher.words?.length) {
      mergedMatchers.push(matcher)
      continue
    }

    const existing = wordMatcherByPart.get(matcher.part)
    if (!existing) {
      const nextMatcher = {
        ...matcher,
        words: [...matcher.words],
      }
      wordMatcherByPart.set(matcher.part, nextMatcher)
      mergedMatchers.push(nextMatcher)
      continue
    }

    for (const word of matcher.words) {
      if (!existing.words?.includes(word)) {
        existing.words = [...(existing.words || []), word]
      }
    }
    if ((existing.words?.length || 0) > 1) {
      existing.condition = "and"
      existing.summary = `${matcher.part} contains selected contents`
    }
  }

  for (const matcher of wordMatcherByPart.values()) {
    if ((matcher.words?.length || 0) > 1) {
      matcher.condition = "and"
    }
  }

  return mergedMatchers
}

function normalizeRawRequestForNuclei(rawRequest: string): string {
  const normalized = rawRequest.replace(/\r\n/g, "\n")
  const sections = normalized.split("\n\n")
  const headerLines = sections[0]?.split("\n") || []
  if (headerLines.length === 0) {
    return normalized
  }

  const requestLine = headerLines[0]
  const requestMatch = requestLine.match(/^([A-Z]+)\s+(.+?)\s+HTTP\/([\d.]+)$/i)
  if (!requestMatch) {
    return normalized
  }

  let requestTarget = requestMatch[2]
  if (/^https?:\/\//i.test(requestTarget)) {
    try {
      const parsed = new URL(requestTarget)
      requestTarget = `${parsed.pathname || "/"}${parsed.search}${parsed.hash}`
    } catch {
      requestTarget = "/"
    }
  }

  headerLines[0] = `${requestMatch[1].toUpperCase()} ${requestTarget} HTTP/${requestMatch[3]}`

  let hostReplaced = false
  for (let index = 1; index < headerLines.length; index += 1) {
    if (/^host\s*:/i.test(headerLines[index])) {
      headerLines[index] = "Host: {{Hostname}}"
      hostReplaced = true
      break
    }
  }

  if (!hostReplaced) {
    headerLines.splice(1, 0, "Host: {{Hostname}}")
  }

  return [headerLines.join("\n"), ...sections.slice(1)].join("\n\n")
}

function deriveTemplateId(rawRequest: string): string {
  const requestLine = rawRequest.replace(/\r\n/g, "\n").split("\n")[0] || ""
  const match = requestLine.match(/^([A-Z]+)\s+(.+?)\s+HTTP\/[\d.]+$/i)
  if (!match) {
    return "generated-http-template"
  }

  const method = match[1].toLowerCase()
  let target = match[2]
  if (/^https?:\/\//i.test(target)) {
    try {
      const parsed = new URL(target)
      target = parsed.pathname || "/"
    } catch {
      target = "/"
    }
  }

  const segments = target
    .split(/[/?#]/)
    .map((segment) => slugify(segment))
    .filter(Boolean)
    .slice(-3)

  return slugify([method, ...segments].join("-")) || "generated-http-template"
}

function deriveTemplateName(rawRequest: string): string {
  const requestLine = rawRequest.replace(/\r\n/g, "\n").split("\n")[0] || ""
  const match = requestLine.match(/^([A-Z]+)\s+(.+?)\s+HTTP\/[\d.]+$/i)
  if (!match) {
    return "Generated HTTP Template"
  }

  return `Generated ${match[1].toUpperCase()} ${match[2]}`
}

export function createNucleiTemplateDraft(input: {
  rawRequest: string
  response: string
  selection?: ResponseSelectionSnapshot | null
  selections?: ResponseSelectionSnapshot[]
}): NucleiTemplateDraft {
  const normalizedRawRequest = normalizeRawRequestForNuclei(input.rawRequest)
  const parsedResponse = parseResponse(input.response)
  const matchers: NucleiTemplateMatcherDraft[] = []

  if (parsedResponse.statusCode !== null) {
    matchers.push({
      type: "status",
      status: [parsedResponse.statusCode],
      summary: `status = ${parsedResponse.statusCode}`,
    })
  }

  const selections = input.selections?.length ? input.selections : input.selection ? [input.selection] : []
  for (const selection of selections) {
    const matcher = createWordMatcherFromSelection(selection)
    if (matcher && !matchers.some((current) => isSameWordMatcher(current, matcher))) {
      matchers.push(matcher)
    }
  }

  return {
    id: deriveTemplateId(normalizedRawRequest),
    name: deriveTemplateName(normalizedRawRequest),
    author: "hephaestus",
    severity: "info",
    description: "Generated from Request Repeater",
    impact: "",
    remediation: "",
    rawRequest: normalizedRawRequest,
    matchersCondition: "and",
    matchers: mergeWordMatchersByPart(matchers),
  }
}

export function parseNucleiTemplateIdFromYaml(yaml: string): string | null {
  const match = yaml.match(/^\s*id\s*:\s*["']?([^"'\s#]+)["']?\s*(?:#.*)?$/m)
  return match?.[1]?.trim() || null
}

function parseNucleiTemplateNameFromYaml(yaml: string): string | null {
  const match = yaml.match(/^\s{2,}name\s*:\s*(.+?)\s*(?:#.*)?$/m)
  if (!match?.[1]) return null
  return parseYamlScalar(match[1]).trim() || null
}

function parseNucleiTemplateAuthorFromYaml(yaml: string): string | null {
  const match = yaml.match(/^\s{2,}author\s*:\s*(.+?)\s*(?:#.*)?$/m)
  if (!match?.[1]) return null
  return parseYamlScalar(match[1]).trim() || null
}

function parseNucleiTemplateSeverityFromYaml(yaml: string): NucleiSeverity {
  const match = yaml.match(/^\s{2,}severity\s*:\s*(.+?)\s*(?:#.*)?$/m)
  const severity = parseYamlScalar(match?.[1] || "").toLowerCase() as NucleiSeverity
  return nucleiSeverities.has(severity) ? severity : "info"
}

function parseNucleiTemplateDescriptionFromYaml(yaml: string): string | null {
  const match = yaml.match(/^\s{2,}description\s*:\s*(.+?)\s*(?:#.*)?$/m)
  if (!match?.[1]) return null
  return parseYamlScalar(match[1]).trim() || null
}

export function createNucleiTemplateDraftFromYaml(yaml: string): NucleiTemplateDraft {
  const id = parseNucleiTemplateIdFromYaml(yaml) || "saved-poc-template"
  return {
    id,
    name: parseNucleiTemplateNameFromYaml(yaml) || id,
    author: parseNucleiTemplateAuthorFromYaml(yaml) || "hephaestus",
    severity: parseNucleiTemplateSeverityFromYaml(yaml),
    description: parseNucleiTemplateDescriptionFromYaml(yaml) || "",
    impact: "",
    remediation: "",
    rawRequest: "",
    matchersCondition: "and",
    matchers: [],
  }
}

function parseYamlScalar(value: string): string {
  const trimmed = value.trim()
  if (!trimmed || trimmed === "|" || trimmed === ">") {
    return ""
  }

  if (trimmed.startsWith('"')) {
    try {
      return JSON.parse(trimmed)
    } catch {
      return trimmed.slice(1).replace(/"$/, "")
    }
  }

  if (trimmed.startsWith("'")) {
    return trimmed.slice(1).replace(/'$/, "").replace(/''/g, "'")
  }

  return trimmed.replace(/\s+#.*$/, "").trim()
}

interface NucleiTemplateInfoBlock {
  index: number
  end: number
  indent: number
}

function yamlIndent(line: string): number {
  return line.match(/^\s*/)?.[0].length || 0
}

function yamlKeyAtIndent(line: string, indent: number): string | null {
  const match = line.match(new RegExp(`^ {${indent}}([A-Za-z][A-Za-z0-9_-]*)\\s*:`))
  return match?.[1] || null
}

function findNucleiTemplateInfoBlock(lines: string[]): NucleiTemplateInfoBlock | null {
  const infoIndex = lines.findIndex((line) => /^info\s*:\s*(?:#.*)?$/.test(line))
  if (infoIndex < 0) {
    return null
  }

  const indent = yamlIndent(lines[infoIndex])
  let end = lines.length
  for (let index = infoIndex + 1; index < lines.length; index += 1) {
    const line = lines[index]
    if (!line.trim()) {
      continue
    }

    const lineIndent = yamlIndent(line)
    if (lineIndent <= indent) {
      end = index
      break
    }

    // A Nuclei root section accidentally indented below `info` must never be
    // treated as metadata. This also prevents subsequent writes from being
    // inserted between that section and the next real root section.
    const key = yamlKeyAtIndent(line, lineIndent)
    if (key && nucleiTemplateRootKeys.has(key) && key !== "info") {
      end = index
      break
    }
  }

  return { index: infoIndex, end, indent }
}

export function syncNucleiTemplateDraftFromYaml(
  draft: NucleiTemplateDraft,
  yaml: string
): NucleiTemplateDraft {
  const updates: Partial<NucleiTemplateDraft> = {}
  const lines = yaml.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n")
  const infoBlock = findNucleiTemplateInfoBlock(lines)
  const infoChildIndent = infoBlock ? infoBlock.indent + 2 : 0

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    if (/^id\s*:/.test(line)) {
      const separatorIndex = line.indexOf(":")
      updates.id = parseYamlScalar(separatorIndex >= 0 ? line.slice(separatorIndex + 1) : "")
      continue
    }

    if (!infoBlock || index <= infoBlock.index || index >= infoBlock.end) {
      continue
    }

    const match = line.match(new RegExp(`^ {${infoChildIndent}}(name|author|severity|description|impact|remediation)\\s*:\\s*(.*)$`))
    if (!match) {
      continue
    }

    const key = match[1]
    const value = parseYamlScalar(match[2] || "")
    if (key === "severity") {
      const severity = value.toLowerCase() as NucleiSeverity
      if (nucleiSeverities.has(severity)) {
        updates.severity = severity
      }
      continue
    }

    if (key === "name") updates.name = value
    if (key === "author") updates.author = value
    if (key === "description") updates.description = value
    if (key === "impact") updates.impact = value
    if (key === "remediation") updates.remediation = value
  }

  return {
    ...draft,
    ...updates,
  }
}

function removeNestedYamlBlock(lines: string[], keyIndex: number, keyIndent: number): void {
  let nextIndex = keyIndex + 1
  while (nextIndex < lines.length) {
    const line = lines[nextIndex]
    if (!line.trim()) {
      nextIndex += 1
      continue
    }
    const indent = line.match(/^\s*/)?.[0].length || 0
    if (indent <= keyIndent) {
      break
    }
    lines.splice(nextIndex, 1)
  }
}

function findDirectYamlKeyLine(lines: string[], key: string, start: number, end: number, indent: number): number {
  const linePattern = new RegExp(`^ {${indent}}${key}\\s*:`)
  for (let index = start; index < end; index += 1) {
    if (linePattern.test(lines[index])) {
      return index
    }
  }
  return -1
}

function yamlBlockEnd(lines: string[], keyIndex: number, end: number, keyIndent: number): number {
  let nextIndex = keyIndex + 1
  while (nextIndex < end) {
    const line = lines[nextIndex]
    if (line.trim() && yamlIndent(line) <= keyIndent) {
      break
    }
    nextIndex += 1
  }
  return nextIndex
}

function metadataInsertionIndex(lines: string[], infoBlock: NucleiTemplateInfoBlock): number {
  const metadataKeys = ["name", "author", "severity", "description", "impact", "remediation"]
  const childIndent = infoBlock.indent + 2
  let insertAt = infoBlock.index + 1
  for (const key of metadataKeys) {
    const keyIndex = findDirectYamlKeyLine(lines, key, infoBlock.index + 1, infoBlock.end, childIndent)
    if (keyIndex >= 0) {
      insertAt = Math.max(insertAt, yamlBlockEnd(lines, keyIndex, infoBlock.end, childIndent))
    }
  }
  return insertAt
}

function upsertYamlScalarLine(lines: string[], key: string, value: string, infoBlock: NucleiTemplateInfoBlock): void {
  const indent = infoBlock.indent + 2
  const lineValue = `${" ".repeat(indent)}${key}: ${key === "severity" ? value : yamlMetadataScalar(value)}`
  const keyIndex = findDirectYamlKeyLine(lines, key, infoBlock.index + 1, infoBlock.end, indent)
  if (keyIndex >= 0) {
    lines[keyIndex] = lineValue
    removeNestedYamlBlock(lines, keyIndex, indent)
    return
  }

  lines.splice(metadataInsertionIndex(lines, infoBlock), 0, lineValue)
}

export function updateNucleiTemplateYamlMetadata(
  yaml: string,
  metadata: Pick<NucleiTemplateDraft, "id" | "name" | "author" | "severity" | "description" | "impact" | "remediation">
): string {
  const lines = yaml.replace(/\r\n/g, "\n").replace(/\r/g, "\n").split("\n")

  const idLine = `id: ${metadata.id || "generated-http-template"}`
  const idIndex = lines.findIndex((line) => /^id\s*:/.test(line))
  if (idIndex >= 0) {
    lines[idIndex] = idLine
  } else {
    lines.unshift(idLine, "")
  }

  let infoBlock = findNucleiTemplateInfoBlock(lines)
  if (!infoBlock) {
    const insertIndex = Math.min(lines.findIndex((line) => /^id\s*:/.test(line)) + 1 || 1, lines.length)
    lines.splice(insertIndex, 0, "", "info:")
    infoBlock = findNucleiTemplateInfoBlock(lines)
    if (!infoBlock) {
      return lines.join("\n")
    }
  }

  const metadataEntries: Array<[string, string]> = [
    ["name", metadata.name || "Generated HTTP Template"],
    ["author", metadata.author || "hephaestus"],
    ["severity", metadata.severity || "info"],
    ["description", metadata.description || "Generated from Request Repeater"],
  ]

  if (metadata.impact.trim()) {
    metadataEntries.push(["impact", metadata.impact])
  }
  if (metadata.remediation.trim()) {
    metadataEntries.push(["remediation", metadata.remediation])
  }

  for (const [key, value] of metadataEntries) {
    const currentInfoBlock = findNucleiTemplateInfoBlock(lines)
    if (!currentInfoBlock) {
      break
    }
    upsertYamlScalarLine(lines, key, value, currentInfoBlock)
  }

  return lines.join("\n")
}

function indentBlock(content: string, spaces: number): string {
  const prefix = " ".repeat(spaces)
  return content
    .replace(/\r\n/g, "\n")
    .split("\n")
    .map((line) => `${prefix}${line}`)
    .join("\n")
}

function yamlQuote(value: string): string {
  return JSON.stringify(value)
}

function yamlSingleQuote(value: string): string {
  return `'${value.replace(/'/g, "''")}'`
}

function stripWrappingQuotes(value: string): string {
  let nextValue = value.trim()
  const quotePairs: Array<[string, string]> = [
    ['"', '"'],
    ["'", "'"],
    ["“", "”"],
    ["‘", "’"],
  ]

  let changed = true
  while (changed) {
    changed = false
    for (const [leftQuote, rightQuote] of quotePairs) {
      if (nextValue.length >= 2 && nextValue.startsWith(leftQuote) && nextValue.endsWith(rightQuote)) {
        nextValue = nextValue.slice(leftQuote.length, -rightQuote.length).trim()
        changed = true
      }
    }
  }

  return nextValue
}

function yamlPlainScalar(value: string): string | null {
  const trimmed = value.trim()
  if (!trimmed) {
    return null
  }

  if (/[\r\n]/.test(trimmed)) {
    return null
  }
  if (/^[\s"'`]|[\s"'`]$/.test(value)) {
    return null
  }
  if (/^[-?:,[\]{}#&*!|>'"%@`]/.test(trimmed)) {
    return null
  }
  if (/(^|[\s])#/.test(trimmed) || /:\s/.test(trimmed)) {
    return null
  }
  if (/^(?:null|Null|NULL|true|True|TRUE|false|False|FALSE|~)$/u.test(trimmed)) {
    return null
  }

  return trimmed
}

function yamlMetadataScalar(value: string): string {
  const normalized = stripWrappingQuotes(value).replace(/\s+/g, " ")
  return yamlPlainScalar(normalized) || yamlSingleQuote(normalized)
}

function yamlPlainOrQuoted(value: string): string {
  const trimmed = value.trim()
  if (!trimmed) {
    return '""'
  }

  const plainScalarPattern = /^[A-Za-z0-9 _./:@?&=+\-()[\]{}#,;*%!~|<>]+$/
  const hasUnsafeEdge = /^[\s"'`]|[:\-[\]{}!,&*#?|>@%]$/.test(value)
  const hasNewline = /[\r\n]/.test(value)
  if (!hasNewline && !hasUnsafeEdge && plainScalarPattern.test(value)) {
    return value
  }

  return yamlQuote(value)
}

export function renderNucleiTemplateYaml(template: NucleiTemplateDraft): string {
  const lines: string[] = [
    `id: ${template.id || "generated-http-template"}`,
    "",
    "info:",
    `  name: ${yamlPlainOrQuoted(template.name || "Generated HTTP Template")}`,
    `  author: ${yamlPlainOrQuoted(template.author || "hephaestus")}`,
    `  severity: ${template.severity || "info"}`,
    `  description: ${yamlPlainOrQuoted(template.description || "Generated from Request Repeater")}`,
  ]

  if (template.impact.trim()) {
    lines.push(`  impact: ${yamlPlainOrQuoted(template.impact)}`)
  }
  if (template.remediation.trim()) {
    lines.push(`  remediation: ${yamlPlainOrQuoted(template.remediation)}`)
  }

  lines.push(
    "",
    "http:",
    "  - raw:",
    "      - |",
    indentBlock(template.rawRequest.trimEnd(), 10),
    `    matchers-condition: ${template.matchersCondition}`,
    "    matchers:",
  )

  for (const matcher of template.matchers) {
    lines.push(`      - type: ${matcher.type}`)
    if (matcher.type === "status" && matcher.status?.length) {
      lines.push("        status:")
      for (const status of matcher.status) {
        lines.push(`          - ${status}`)
      }
    }
    if (matcher.type === "word" && matcher.words?.length) {
      if (matcher.part) {
        lines.push(`        part: ${matcher.part}`)
      }
      lines.push("        words:")
      for (const word of matcher.words) {
        lines.push(`          - ${yamlQuote(word)}`)
      }
      if (matcher.condition) {
        lines.push(`        condition: ${matcher.condition}`)
      }
    }
  }

  return lines.join("\n")
}
