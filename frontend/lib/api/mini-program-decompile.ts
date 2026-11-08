import { fetchWithAuth } from '@/lib/api'

export type MiniProgramExtractionRule = {
  id: string
  pattern: string
  enabled: boolean
}

export type MiniProgramRuleGroup = {
  id: string
  name: string
  enabled: boolean
  rules: MiniProgramExtractionRule[]
}

export type MiniProgramDecompileSettings = {
  appletPath: string
  ruleGroups: MiniProgramRuleGroup[]
  defaultRuleGroups: MiniProgramRuleGroup[]
  rules?: string[]
  outputRoot: string
}

export type MiniProgramVersion = {
  number: string
  updateDate: string
  wxapkgCount: number
  decompiled: boolean
  outputDir?: string
}

export type MiniProgramInfo = {
  appId: string
  nickname?: string
  username?: string
  description?: string
  avatar?: string
  source?: string
}

export type MiniProgramItem = {
  appId: string
  updateDate: string
  info?: MiniProgramInfo
  versions: MiniProgramVersion[]
}

export type MiniProgramPackageFile = {
  path: string
  name: string
  size: number
}

export type MiniProgramDecompileResult = {
  appId: string
  version: string
  outputDir: string
  packages: MiniProgramPackageFile[]
  fileCount: number
  sampleFiles: string[]
  matchGroups: MiniProgramMatchGroup[]
  durationMs: number
  errors?: string[]
}

export type MiniProgramSensitiveMatch = {
  value: string
  groupId: string
  ruleId?: string
}

export type MiniProgramMatchGroup = {
  id: string
  name: string
  matches: MiniProgramSensitiveMatch[]
}

export type MiniProgramMatchesResult = {
  groups: MiniProgramMatchGroup[]
}

async function readPayload<T>(response: Response, fallback: string): Promise<T> {
  const payload = await response.json()
  if (!payload?.success) {
    throw new Error(payload?.error || fallback)
  }
  return payload.data as T
}

export async function getMiniProgramDecompileSettings() {
  const response = await fetchWithAuth('/api/v1/tools/mini-program-decompile/settings', {
    cache: 'no-store',
  })
  return readPayload<MiniProgramDecompileSettings>(response, '加载小程序反编译配置失败')
}

export async function saveMiniProgramDecompileSettings(input: {
  appletPath: string
  ruleGroups: MiniProgramRuleGroup[]
}) {
  const response = await fetchWithAuth('/api/v1/tools/mini-program-decompile/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  const payload = await response.json()
  if (!payload?.success) {
    throw new Error(payload?.error || '保存小程序反编译配置失败')
  }
}

export async function listMiniPrograms(appletPath?: string) {
  const query = appletPath ? `?appletPath=${encodeURIComponent(appletPath)}` : ''
  const response = await fetchWithAuth(`/api/v1/tools/mini-program-decompile/programs${query}`, {
    cache: 'no-store',
  })
  return readPayload<{ items: MiniProgramItem[]; outputRoot: string }>(
    response,
    '扫描小程序列表失败',
  )
}

export async function decompileMiniProgram(input: {
  appletPath?: string
  appId: string
  version: string
  ruleGroups?: MiniProgramRuleGroup[]
}) {
  const response = await fetchWithAuth('/api/v1/tools/mini-program-decompile/decompile', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  return readPayload<MiniProgramDecompileResult>(response, '反编译失败')
}

export async function getMiniProgramMatches(appId: string, version: string) {
  const query = new URLSearchParams({ appId, version }).toString()
  const response = await fetchWithAuth(`/api/v1/tools/mini-program-decompile/matches?${query}`, {
    cache: 'no-store',
  })
  return readPayload<MiniProgramMatchesResult>(response, '读取敏感信息失败')
}

export async function clearMiniProgramDecompileOutput(input: { appId?: string; version?: string }) {
  const response = await fetchWithAuth('/api/v1/tools/mini-program-decompile/clear', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  const payload = await response.json()
  if (!payload?.success) {
    throw new Error(payload?.error || '清理反编译输出失败')
  }
}
