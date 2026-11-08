// 文本处理函数

// ROT13
export const rot13 = (input: string): string => {
  return input.replace(/[a-zA-Z]/g, (c) => {
    const base = c <= 'Z' ? 65 : 97
    return String.fromCharCode(((c.charCodeAt(0) - base + 13) % 26) + base)
  })
}

// 反转字符串
export const reverseString = (input: string): string => {
  return Array.from(input).reverse().join('')
}

// 转大写
export const toUpperCase = (input: string): string => input.toUpperCase()

// 转小写
export const toLowerCase = (input: string): string => input.toLowerCase()

// 首字母大写
export const toTitleCase = (input: string): string => {
  return input.replace(/\w\S*/g, (txt) => txt.charAt(0).toUpperCase() + txt.substr(1).toLowerCase())
}

// 去除首尾空白
export const trimWhitespace = (input: string): string => input.trim()

// 移除所有空格
export const removeAllSpaces = (input: string): string => input.replace(/\s/g, '')

// 移除换行
export const removeNewlines = (input: string): string => input.replace(/[\r\n]/g, '')

// 字符串排序
export const sortLines = (input: string, options?: Record<string, string>): string => {
  const order = options?.order ?? 'asc'
  const mode = options?.mode ?? 'alphabetical'
  
  // 解析分隔符
  const separator = options?.separator ?? ''
  let actualSeparator: string | RegExp
  let outputSeparator: string
  
  if (separator === '' || separator === '\\n') {
    actualSeparator = /\r?\n/
    outputSeparator = '\n'
  } else if (separator === '\\t') {
    actualSeparator = '\t'
    outputSeparator = '\t'
  } else if (separator === '\\r\\n') {
    actualSeparator = /\r\n/
    outputSeparator = '\n'
  } else {
    actualSeparator = separator
    outputSeparator = separator
  }
  
  const lines = input.split(actualSeparator).filter(line => line.trim())
  
  if (lines.length === 0) return '[无内容]'
  
  // 自然排序（对数字友好）
  if (mode === 'natural') {
    lines.sort((a, b) => {
      return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' })
    })
  } else {
    // 字母排序
    lines.sort((a, b) => a.localeCompare(b))
  }
  
  // 反转（降序）
  if (order === 'desc') {
    lines.reverse()
  }
  
  return lines.join(outputSeparator)
}

const escapeRegExp = (value: string): string => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

// 将正则替换字符串中的常用转义序列转换为实际字符。
// 保留 $1 等 JavaScript 替换占位符，便于正则模式下使用捕获组。
const decodeReplacementEscapes = (value: string): string => {
  return value.replace(/\\(\\|n|r|t)/g, (_, escape: string) => {
    if (escape === '\\') return '\\'
    if (escape === 'n') return '\n'
    if (escape === 'r') return '\r'
    return '\t'
  })
}

// 查找替换：默认按普通文本处理，开启正则模式后才按正则表达式处理
export const regexFindReplace = (input: string, options?: Record<string, string>): string => {
  const find = options?.find ?? ''
  const replace = options?.replace ?? ''
  const useRegex = options?.useRegex === 'true'
  const globalMatch = options?.globalMatch === 'true'
  const caseInsensitive = options?.caseInsensitive === 'true'
  const multiline = options?.multiline === 'true'
  const dotAll = options?.dotAll === 'true'
  
  if (!find) {
    return '[请输入查找内容]'
  }

  try {
    if (!useRegex) {
      const flags = `${globalMatch ? 'g' : ''}${caseInsensitive ? 'i' : ''}`
      const matcher = flags ? new RegExp(escapeRegExp(find), flags) : find

      // 使用回调返回替换内容，避免普通模式下 $&、$1 等被特殊解析。
      return input.replace(matcher, () => replace)
    }

    // 构建正则表达式标志
    let flags = ''
    if (globalMatch) flags += 'g'
    if (caseInsensitive) flags += 'i'
    if (multiline) flags += 'm'
    if (dotAll) flags += 's'
    
    const regex = new RegExp(find, flags)
    const result = input.replace(regex, decodeReplacementEscapes(replace))
    
    return result
  } catch (error) {
    return `[正则表达式错误: ${error instanceof Error ? error.message : '未知错误'}]`
  }
}
