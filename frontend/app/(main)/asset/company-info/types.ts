// 类型定义
export interface CompanyInfo {
  companyName: string
  trademark: string
  investment: number
  amount: string
  regStatus: string
  domains: string[]
  subsidiaries: CompanyInfo[]
  officialAccounts: OfficialAccount[]
  emails?: EmailInfo[]
  apps: App[]
  applets: Applet[]
}

export interface OfficialAccount {
  name: string
  logo: string
  numbers: string
  qrcode: string
  introduction: string
  belongCompany: string
}

export interface EmailInfo {
  address: string
  type: string
  company: string
  domains: string[]
  sources: string[]
  tags: string[]
  leakageCount: number
  leakageAccounts: string[]
  leakageUpdatedAt: string
}

export interface App {
  serviceName: string
  serviceLicence: string
  iconUrl: string
  belongCompany: string
}

export interface Applet {
  serviceName: string
  serviceLicence: string
  updateRecordTime: string
  belongCompany: string
}

export interface FormData {
  taskName: string
  company: string
  invest: number
  dataSource: string[]
  zeroZoneEnable: boolean
  miitEnable: boolean
  riskbirdCookie: string
  zeroZoneKey: string
  subcompanyLevel: number
  miitApi: string
}
