// 数据提取函数

// 正则表达式
const regexpPhone = /(13[0-9]|14[01456879]|15[0-35-9]|16[2567]|17[0-8]|18[0-9]|19[0-35-9])\d{8}/g
const regexpIdCard = /[1-9]\d{5}(18|19|20)\d{2}(0[1-9]|1[0-2])(0[1-9]|[12]\d|3[01])\d{3}[\dXx]/g
const regexURL = /https?:\/\/[^\s"'<>]+/g
const regexIP = /\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b/g
const regexDruidWebSession = /"SESSIONID":"([^"]+)"/g
const regexDruidWebURI = /"URI":"([^"]+)"/g

const validateSingleIP = (ip: string): boolean => {
  const ipRegex = /^(\d{1,3}\.){3}\d{1,3}$/
  if (!ipRegex.test(ip)) return false
  const parts = ip.split('.').map(Number)
  return parts.every(part => part >= 0 && part <= 255)
}

// 提取 IP
export const extractIP = (input: string): string => {
  const matches = input.match(regexIP) || []
  const unique = Array.from(new Set(matches.filter(validateSingleIP)))
  return unique.length > 0 ? unique.join('\n') : '[未找到IP地址]'
}

// 提取 IP:port
export const extractIPPort = (input: string): string => {
  const regexIPPort = /\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?):[0-9]{1,5}\b/g
  const matches = input.match(regexIPPort) || []
  const unique = Array.from(new Set(matches.filter(ipPort => {
    const [ip, port] = ipPort.split(':')
    const portNum = parseInt(port)
    return validateSingleIP(ip) && portNum >= 1 && portNum <= 65535
  })))
  return unique.length > 0 ? unique.join('\n') : '[未找到IP:端口]'
}

// 提取 URL
export const extractURL = (input: string): string => {
  const matches = input.match(regexURL) || []
  const unique = Array.from(new Set(matches))
  return unique.length > 0 ? unique.join('\n') : '[未找到URL]'
}

// 提取 URL 根路径
export const extractRootURL = (input: string): string => {
  const matches = input.match(regexURL) || []
  const rootURLs = matches
    .map((url: string) => {
      try {
        return new URL(url).origin
      } catch {
        return null
      }
    })
    .filter((url): url is string => url !== null)
  const unique = Array.from(new Set(rootURLs))
  return unique.length > 0 ? unique.join('\n') : '[未找到URL根路径]'
}

// 提取域名
export const extractDomain = (input: string): string => {
  const matches = input.match(regexURL) || []
  const unique = Array.from(new Set(matches))
  const domains = unique
    .map((url: string) => {
      try {
        return new URL(url).hostname
      } catch {
        return null
      }
    })
    .filter((d): d is string => d !== null)
  const uniqueDomains = Array.from(new Set(domains))
  return uniqueDomains.length > 0 ? uniqueDomains.join('\n') : '[未找到域名]'
}

// 提取根域名
export const extractRootDomain = (input: string): string => {
  const matches = input.match(regexURL) || []
  const unique = Array.from(new Set(matches))
  const rootDomains = unique
    .map((url: string) => {
      try {
        const hostname = new URL(url).hostname
        // 移除 www. 前缀并提取根域名（二级域名.顶级域名）
        const parts = hostname.replace(/^www\./, '').split('.')
        // 如果有多于2个部分，取最后两个部分（例如 api.example.com -> example.com）
        if (parts.length >= 2) {
          return parts.slice(-2).join('.')
        }
        return hostname
      } catch {
        return null
      }
    })
    .filter((d): d is string => d !== null)
  const uniqueRootDomains = Array.from(new Set(rootDomains))
  return uniqueRootDomains.length > 0 ? uniqueRootDomains.join('\n') : '[未找到根域名]'
}

// 提取手机号
export const extractPhone = (input: string): string => {
  const phones = input.match(regexpPhone) || []
  const unique = Array.from(new Set(phones))
  return unique.length > 0 ? unique.join('\n') : '[未找到手机号]'
}

// 提取身份证
export const extractIdCard = (input: string): string => {
  const idcards = input.match(regexpIdCard) || []
  const unique = Array.from(new Set(idcards))
  return unique.length > 0 ? unique.join('\n') : '[未找到身份证号]'
}

// 提取 Druid WebURI
export const extractDruidWebURI = (input: string): string => {
  const matches: string[] = []
  let match
  regexDruidWebURI.lastIndex = 0
  while ((match = regexDruidWebURI.exec(input)) !== null) {
    matches.push(match[1])
  }
  const unique = Array.from(new Set(matches))
  return unique.length > 0 ? unique.join('\n') : '[未找到WebURI]'
}

// 提取 Druid WebSession
export const extractDruidWebSession = (input: string): string => {
  const matches: string[] = []
  let match
  regexDruidWebSession.lastIndex = 0
  while ((match = regexDruidWebSession.exec(input)) !== null) {
    matches.push(match[1])
  }
  const unique = Array.from(new Set(matches))
  return unique.length > 0 ? unique.join('\n') : '[未找到WebSession]'
}

// 去重
export const deduplicateLines = (input: string, options?: Record<string, string>): string => {
  const separator = options?.separator ?? ''
  
  // 解析分隔符，支持转义字符，空值默认换行
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
  
  const parts = input.split(actualSeparator)
  const unique = Array.from(new Set(parts.filter(part => part.trim())))
  return unique.length > 0 ? unique.join(outputSeparator) : '[无内容]'
}
