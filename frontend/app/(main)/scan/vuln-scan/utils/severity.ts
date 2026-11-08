/**
 * 漏洞等级转换工具函数
 * 将英文的漏洞等级转换为中文
 */

export type Severity = 'critical' | 'high' | 'medium' | 'low' | 'info'

/**
 * 将英文漏洞等级转换为中文
 */
export function getSeverityText(severity: string): string {
  switch (severity.toLowerCase()) {
    case 'critical':
      return '严重'
    case 'high':
      return '高危'
    case 'medium':
      return '中危'
    case 'low':
      return '低危'
    case 'info':
      return '信息'
    default:
      return severity
  }
}

/**
 * 获取漏洞等级的颜色
 */
export function getSeverityColor(severity: string): string {
  switch (severity.toLowerCase()) {
    case 'critical':
      return 'text-red-600'
    case 'high':
      return 'text-orange-600'
    case 'medium':
      return 'text-yellow-600'
    case 'low':
      return 'text-blue-600'
    case 'info':
      return 'text-gray-600'
    default:
      return 'text-gray-600'
  }
}

/**
 * 获取漏洞等级的 Badge 颜色类
 */
export function getSeverityBadgeVariant(severity: string): 'destructive' | 'default' | 'secondary' | 'outline' {
  switch (severity.toLowerCase()) {
    case 'critical':
      return 'destructive'
    case 'high':
      return 'destructive'
    case 'medium':
      return 'default'
    case 'low':
      return 'secondary'
    case 'info':
      return 'outline'
    default:
      return 'outline'
  }
}
