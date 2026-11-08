"use client"

export interface PrivacyMatch {
  start: number
  end: number
}

const ADDRESS_HEADER_NAMES = new Set([
  "host",
  "origin",
  "referer",
  "referrer",
  "location",
  "x-forwarded-for",
  "x-forwarded-host",
  "x-real-ip",
  "forwarded",
])

export const maskIPv4 = (value: string) => {
  const parts = value.split(".")
  if (parts.length !== 4) return value
  return `${parts[0]}.${parts[1]}.*.*`
}

export const maskIPv6 = (value: string) => {
  const parts = value.split(":").filter(Boolean)
  if (parts.length <= 2) return "****"
  return `${parts.slice(0, 2).join(":")}:****:****`
}

export const maskDomain = (value: string) => {
  const parts = value.split(".")
  if (parts.length < 2) return value
  return parts
    .map((part, index) => {
      if (!part) return part
      if (index === parts.length - 1) return part
      if (part.length <= 2) return `${part[0] || ""}*`
      return `${part[0]}${"*".repeat(Math.max(part.length - 2, 1))}${part[part.length - 1]}`
    })
    .join(".")
}

export const maskHost = (host: string) => {
  const cleanHost = host.replace(/^\[|\]$/g, "")
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(cleanHost)) {
    return maskIPv4(cleanHost)
  }
  if (cleanHost.includes(":") && /^[0-9a-f:]+$/i.test(cleanHost)) {
    return maskIPv6(cleanHost)
  }
  return maskDomain(cleanHost)
}

export const getMaskedUrl = (value: string) => {
  if (!value?.trim()) return value
  try {
    const parsed = new URL(value.trim())
    const maskedHost = parsed.port ? `[hidden-host]:${parsed.port}` : "[hidden-host]"
    return value.replace(parsed.host, maskedHost)
  } catch {
    return value
  }
}

export const collectPrivacyMatches = (value: string): PrivacyMatch[] => {
  if (!value) return []

  const matches: PrivacyMatch[] = []
  const lines = value.split("\n")
  let offset = 0

  lines.forEach((line, index) => {
    const trimmed = line.trim()

    // 请求行：METHOD /path HTTP/1.1 或 METHOD http://host/path HTTP/1.1
    if (index === 0) {
      const requestLineMatch = line.match(/^([A-Z]+)\s+(.+?)\s+(HTTP\/[\d.]+)$/)
      if (requestLineMatch) {
        const target = requestLineMatch[2]
        if (/^https?:\/\//i.test(target)) {
          const targetStart = line.indexOf(target)
          matches.push({ start: offset + targetStart, end: offset + targetStart + target.length })
        }
      }

      // 响应首行一般不遮，只遮 Location 头即可
    }

    const headerMatch = line.match(/^([^:]+):\s*(.*)$/)
    if (headerMatch) {
      const headerName = headerMatch[1].trim().toLowerCase()
      const headerValue = headerMatch[2] ?? ""
      if (ADDRESS_HEADER_NAMES.has(headerName) && headerValue) {
        const valueStart = line.indexOf(headerValue)
        if (valueStart >= 0) {
          matches.push({ start: offset + valueStart, end: offset + valueStart + headerValue.length })
        }
      }
    }

    offset += line.length + 1
  })

  return matches
}

export const offsetToPosition = (value: string, offset: number) => {
  let lineNumber = 1
  let column = 1
  for (let i = 0; i < offset; i++) {
    if (value[i] === "\n") {
      lineNumber += 1
      column = 1
    } else {
      column += 1
    }
  }
  return { lineNumber, column }
}
