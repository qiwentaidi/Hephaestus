/**
 * 日期时间格式化工具函数
 */

/**
 * 将日期时间字符串格式化为中文本地化格式
 * @param dateString 日期时间字符串（ISO 8601 格式或其他标准格式）
 * @param fallback 当日期无效时返回的默认值，默认为 '-'
 * @returns 格式化后的日期时间字符串，格式：YYYY/MM/DD HH:mm:ss
 */
export function formatDateTime(
  dateString: string | null | undefined,
  fallback: string = '-'
): string {
  if (!dateString) {
    return fallback
  }

  try {
    const date = new Date(dateString)
    if (isNaN(date.getTime())) {
      return fallback
    }

    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return fallback
  }
}

/**
 * 格式化日期（不包含时间）
 * @param dateString 日期字符串
 * @param fallback 当日期无效时返回的默认值，默认为 '-'
 * @returns 格式化后的日期字符串，格式：YYYY/MM/DD
 */
export function formatDate(
  dateString: string | null | undefined,
  fallback: string = '-'
): string {
  if (!dateString) {
    return fallback
  }

  try {
    const date = new Date(dateString)
    if (isNaN(date.getTime())) {
      return fallback
    }

    return date.toLocaleDateString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    })
  } catch {
    return fallback
  }
}

/**
 * 格式化时间（不包含日期）
 * @param dateString 日期时间字符串
 * @param fallback 当日期无效时返回的默认值，默认为 '-'
 * @returns 格式化后的时间字符串，格式：HH:mm:ss
 */
export function formatTime(
  dateString: string | null | undefined,
  fallback: string = '-'
): string {
  if (!dateString) {
    return fallback
  }

  try {
    const date = new Date(dateString)
    if (isNaN(date.getTime())) {
      return fallback
    }

    return date.toLocaleTimeString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return fallback
  }
}

/**
 * 计算两个日期之间的时间差（以分钟为单位）
 * @param dateString1 第一个日期时间字符串
 * @param dateString2 第二个日期时间字符串（可选，默认为当前时间）
 * @returns 时间差（分钟），如果日期无效返回 null
 */
export function getTimeDiffInMinutes(
  dateString1: string | null | undefined,
  dateString2?: string | null | undefined
): number | null {
  if (!dateString1) {
    return null
  }

  try {
    const date1 = new Date(dateString1)
    if (isNaN(date1.getTime())) {
      return null
    }

    const date2 = dateString2 ? new Date(dateString2) : new Date()
    if (isNaN(date2.getTime())) {
      return null
    }

    const diffMs = date2.getTime() - date1.getTime()
    return diffMs / (1000 * 60) // 转换为分钟
  } catch {
    return null
  }
}

