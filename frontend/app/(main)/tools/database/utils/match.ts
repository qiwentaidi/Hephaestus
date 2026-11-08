import type { DatabaseCollectRule, DatabaseValue } from '../types'

export const DEFAULT_KEYWORDS = [
  'phone',
  'telephone',
  'idcard',
  'card',
  'password',
  'username',
  'mobile',
  'sfz',
  'secret',
  'pwd',
  'passwd',
  'id_card',
  'idcard_no',
]

export const BUILTIN_ROW_MATCH_RULES: DatabaseCollectRule[] = [
  {
    label: 'Email',
    pattern: "^\\w+([-+.']\\w+)*@\\w+([-.]\\w+)*\\.\\w+([-.]\\w+)*$",
  },
  {
    label: 'Phone Number',
    pattern: '^1[3-9]\\d{9}$',
  },
  {
    label: 'ID Card',
    pattern:
      '^[1-9]\\d{5}(18|19|20)\\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\\d|3[01])\\d{3}[\\dXx]$|^[1-9]\\d{7}(0[1-9]|1[0-2])(0[1-9]|[12]\\d|3[01])\\d{3}$',
  },
  {
    label: 'AKSK',
    pattern: '^[A-Za-z0-9]{20,}$',
  },
]

export type CompiledCollectRule = {
  label: string
  regex: RegExp
}

// 获取列名关键词（从 localStorage 或使用默认值）
export function getKeywordColumns(): string[] {
  if (typeof window === 'undefined') {
    return DEFAULT_KEYWORDS
  }

  try {
    const saved = localStorage.getItem('database_keywords')
    if (saved) {
      const keywords = JSON.parse(saved)
      if (Array.isArray(keywords) && keywords.length > 0) {
        return keywords
      }
    }
  } catch {
    // 如果解析失败，使用默认值
  }

  return DEFAULT_KEYWORDS
}

// Base64 解码为 UTF-8
function decodeBase64ToUtf8(base64String: string): string {
  try {
    const binaryString = atob(base64String)
    const bytes = new Uint8Array(binaryString.length)
    for (let i = 0; i < binaryString.length; i++) {
      bytes[i] = binaryString.charCodeAt(i)
    }
    const decoder = new TextDecoder('utf-8', { fatal: true })
    return decoder.decode(bytes)
  } catch {
    return base64String
  }
}

export function compileRowMatchRules(customRules: DatabaseCollectRule[]): CompiledCollectRule[] {
  const sourceRules = [...BUILTIN_ROW_MATCH_RULES, ...customRules]

  return sourceRules.flatMap((rule) => {
    try {
      return [{ label: rule.label, regex: new RegExp(rule.pattern) }]
    } catch {
      return []
    }
  })
}

// 匹配列名
export function matchColumns(columns: string[]): { matched: boolean; keyword?: string } {
  const keywordColumns = getKeywordColumns()
  const lowerColumns = columns.map((col) => col.toLowerCase())
  const matched = keywordColumns.some((keyword) =>
    lowerColumns.some((col) => col.includes(keyword))
  )
  if (matched) {
    const keyword = keywordColumns.find((kw) =>
      lowerColumns.some((col) => col.includes(kw))
    )
    return { matched: true, keyword }
  }
  return { matched: false }
}

// 匹配行数据
export function matchRows(
  rows: DatabaseValue[][],
  compiledRules: CompiledCollectRule[]
): {
  matched: boolean
  matchedLabel?: string
} {
  for (const row of rows) {
    for (const cell of row) {
      if (!cell) continue

      const value = decodeBase64ToUtf8(String(cell))
      for (const rule of compiledRules) {
        if (rule.regex.test(value)) {
          return { matched: true, matchedLabel: rule.label }
        }
      }
    }
  }

  return { matched: false }
}
