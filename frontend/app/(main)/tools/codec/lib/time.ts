// 时间转换函数

// Unix时间戳 -> 北京时间（支持秒/毫秒）
export const unixToBeijing = (input: string, options?: Record<string, string>): string => {
  try {
    const unit = options?.unit ?? 'second'
    const timestamp = parseInt(input.trim())
    if (isNaN(timestamp)) {
      return '[请输入有效的Unix时间戳]'
    }
    const ms = unit === 'millisecond' ? timestamp : timestamp * 1000
    const date = new Date(ms)
    const beijingTime = new Date(date.getTime() + 8 * 60 * 60 * 1000)
    const formatted = beijingTime.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, '')
    
    return formatted
  } catch (error) {
    return `[转换失败: ${error instanceof Error ? error.message : '未知错误'}]`
  }
}

// 北京时间 -> Unix时间戳（支持秒/毫秒）
export const beijingToUnix = (input: string, options?: Record<string, string>): string => {
  try {
    const unit = options?.unit ?? 'second'
    const dateStr = input.trim()
    const date = new Date(dateStr)
    if (isNaN(date.getTime())) {
      return '[请输入有效的日期时间格式 (如: 2024-01-01 12:00:00)]'
    }
    const timestamp = date.getTime() - 8 * 60 * 60 * 1000
    
    if (unit === 'millisecond') {
      return timestamp.toString()
    }
    return Math.floor(timestamp / 1000).toString()
  } catch (error) {
    return `[转换失败: ${error instanceof Error ? error.message : '未知错误'}]`
  }
}

// 标准时间格式 -> 北京时间（支持 ISO/UTC）
export const standardToBeijing = (input: string, options?: Record<string, string>): string => {
  try {
    const format = options?.format ?? 'iso'
    const date = new Date(input.trim())
    if (isNaN(date.getTime())) {
      return `[请输入有效的${format === 'iso' ? 'ISO 8601' : 'UTC'}格式]`
    }
    const beijingTime = new Date(date.getTime() + 8 * 60 * 60 * 1000)
    return beijingTime.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, '')
  } catch (error) {
    return `[转换失败: ${error instanceof Error ? error.message : '未知错误'}]`
  }
}

// 北京时间 -> 标准时间格式（支持 ISO/UTC）
export const beijingToStandard = (input: string, options?: Record<string, string>): string => {
  try {
    const format = options?.format ?? 'iso'
    const dateStr = input.trim()
    const date = new Date(dateStr)
    if (isNaN(date.getTime())) {
      return '[请输入有效的日期时间格式]'
    }
    const utcTime = new Date(date.getTime() - 8 * 60 * 60 * 1000)
    
    if (format === 'iso') {
      return utcTime.toISOString()
    } else {
      return utcTime.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, '')
    }
  } catch (error) {
    return `[转换失败: ${error instanceof Error ? error.message : '未知错误'}]`
  }
}

// 获取当前时间（支持多种格式）
export const getCurrentTime = (input: string, options?: Record<string, string>): string => {
  const format = options?.format ?? 'timestamp-second'
  const now = Date.now()
  
  switch (format) {
    case 'timestamp-second':
      return Math.floor(now / 1000).toString()
    case 'timestamp-millisecond':
      return now.toString()
    case 'beijing':
      const beijingTime = new Date(now + 8 * 60 * 60 * 1000)
      return beijingTime.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, '')
    case 'iso':
      return new Date(now).toISOString()
    case 'utc':
      return new Date(now).toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, '')
    default:
      return now.toString()
  }
}

// 日期格式化
export const formatDate = (input: string, options?: Record<string, string>): string => {
  try {
    const format = options?.format ?? 'YYYY-MM-DD HH:mm:ss'
    const date = new Date(input.trim())
    if (isNaN(date.getTime())) {
      return '[请输入有效的日期时间]'
    }
    
    const year = date.getFullYear()
    const month = (date.getMonth() + 1).toString().padStart(2, '0')
    const day = date.getDate().toString().padStart(2, '0')
    const hours = date.getHours().toString().padStart(2, '0')
    const minutes = date.getMinutes().toString().padStart(2, '0')
    const seconds = date.getSeconds().toString().padStart(2, '0')
    
    let result = format
      .replace('YYYY', year.toString())
      .replace('MM', month)
      .replace('DD', day)
      .replace('HH', hours)
      .replace('mm', minutes)
      .replace('ss', seconds)
    
    return result
  } catch (error) {
    return `[格式化失败: ${error instanceof Error ? error.message : '未知错误'}]`
  }
}
