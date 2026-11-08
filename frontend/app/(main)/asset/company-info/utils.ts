import { CompanyInfo, OfficialAccount, App, Applet, EmailInfo } from './types'
import { saveBlobAsFile } from '@/lib/download'

// 获取公司名称首字符
export function getCompanyFirstChar(companyName: string): string {
  if (!companyName) return '?'
  // 移除常见的公司后缀，获取核心名称
  const cleanName = companyName
    .replace(/（.*?）/g, '') // 移除括号内容
    .replace(/\(.*?\)/g, '') // 移除英文括号内容
    .replace(/有限公司/g, '')
    .replace(/股份有限公司/g, '')
    .replace(/集团/g, '')
    .replace(/科技/g, '')
    .trim()
  
  // 获取第一个字符（支持中文和英文）
  const firstChar = cleanName.charAt(0)
  return firstChar || companyName.charAt(0) || '?'
}

// 递归提取子项
export function extractAllChildren<T extends OfficialAccount | App | Applet | EmailInfo>(
  companies: CompanyInfo[],
  key: 'officialAccounts' | 'apps' | 'applets' | 'emails'
): (T & { belongCompany: string })[] {
  const result: (T & { belongCompany: string })[] = []

  const dfs = (list: CompanyInfo[]) => {
    for (const company of list) {
      const children = company[key]
      if (Array.isArray(children)) {
        for (const item of children) {
          result.push({
            ...item,
            belongCompany: company.companyName,
          } as T & { belongCompany: string })
        }
      }

      if (Array.isArray(company.subsidiaries)) {
        dfs(company.subsidiaries)
      }
    }
  }

  dfs(companies)
  return result
}

// 防抖函数
export function debounce<T extends (...args: any[]) => void>(
  func: T,
  wait: number
): T {
  let timeout: NodeJS.Timeout | null = null
  return ((...args: any[]) => {
    if (timeout) clearTimeout(timeout)
    timeout = setTimeout(() => func(...args), wait)
  }) as T
}

// 导出企业信息为JSON
export async function exportCompanyInfoAsJSON(companiesInfo: CompanyInfo[]): Promise<void> {
  const data = JSON.stringify(companiesInfo, null, 2)
  const blob = new Blob([data], { type: 'application/json' })
  await saveBlobAsFile(blob, `company_info_${new Date().getTime()}.json`)
}

// 导出企业信息为CSV（扁平化数据）
export async function exportCompanyInfoAsCSV(companiesInfo: CompanyInfo[]): Promise<void> {
  // 递归展开所有公司和子公司
  const flattenCompanies = (companies: CompanyInfo[], level: number = 0, parentCompany: string = ''): any[] => {
    const result: any[] = []
    for (const company of companies) {
      // 格式化子公司列表（公司名称）
      const subsidiariesList = company.subsidiaries?.map(sub => sub.companyName).join('；') || ''
      
      // 格式化公众号列表（名称(账号)）
      const officialAccountsList = company.officialAccounts?.map(acc => {
        const parts = [acc.name]
        if (acc.numbers) parts.push(`(${acc.numbers})`)
        return parts.join('')
      }).join('；') || ''
      
      // 格式化APP列表（服务名称(许可证)）
      const appsList = company.apps?.map(app => {
        const parts = [app.serviceName]
        if (app.serviceLicence) parts.push(`(${app.serviceLicence})`)
        return parts.join('')
      }).join('；') || ''
      
      // 格式化小程序列表（服务名称(许可证) - 更新时间）
      const appletsList = company.applets?.map(applet => {
        const parts = [applet.serviceName]
        if (applet.serviceLicence) parts.push(`(${applet.serviceLicence})`)
        if (applet.updateRecordTime) parts.push(`-${applet.updateRecordTime}`)
        return parts.join('')
      }).join('；') || ''
      
      // 格式化域名列表
      const domainsList = company.domains?.join('；') || ''
      
      result.push({
        公司名称: company.companyName,
        层级: level,
        母公司: parentCompany || '-',
        投资比例: company.investment + '%',
        注册资本: company.amount,
        经营状态: company.regStatus || '未知',
        域名: domainsList,
        域名数量: company.domains?.length || 0,
        子公司: subsidiariesList,
        子公司数量: company.subsidiaries?.length || 0,
        公众号: officialAccountsList,
        公众号数量: company.officialAccounts?.length || 0,
        APP: appsList,
        APP数量: company.apps?.length || 0,
        小程序: appletsList,
        小程序数量: company.applets?.length || 0,
      })
      
      if (company.subsidiaries && company.subsidiaries.length > 0) {
        result.push(...flattenCompanies(company.subsidiaries, level + 1, company.companyName))
      }
    }
    return result
  }

  const flatData = flattenCompanies(companiesInfo)
  
  if (flatData.length === 0) {
    return
  }

  // 获取表头
  const headers = Object.keys(flatData[0])
  
  // 生成CSV内容
  const csvRows = [
    headers.join(','),
    ...flatData.map(row =>
      headers.map(header => {
        const value = row[header]
        // 处理包含逗号、引号或换行符的值
        if (value == null || value === '') return '""'
        const stringValue = String(value)
        // CSV中如果包含逗号、引号、换行符或分号，需要用引号包裹
        if (stringValue.includes(',') || stringValue.includes('"') || stringValue.includes('\n') || stringValue.includes('；')) {
          return `"${stringValue.replace(/"/g, '""')}"`
        }
        return stringValue
      }).join(',')
    ),
  ]

  const csv = '\ufeff' + csvRows.join('\n') // 添加BOM以支持中文
  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
  await saveBlobAsFile(blob, `company_info_${new Date().getTime()}.csv`)
}
