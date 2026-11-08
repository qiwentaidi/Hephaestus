// 数据库连接类型
export type DatabaseConnection = {
  nanoid: string
  scheme: string
  host: string
  port: number
  username: string
  password: string
  serverName: string
  notes: string
  // 运行时状态
  connected?: boolean
  loading?: boolean
  databaseCount?: number
  tableCount?: number
  progress?: number
  tablePanes?: CollectResult[]
  updatedAt?: string // 更新时间
}

// 数据库信息类型
export type DatabaseInfo = {
  [dbName: string]: string[]
}

// 表数据行类型
export type DatabaseValue = string | number | boolean | null | DatabaseValue[] | { [key: string]: DatabaseValue }

export type RowData = {
  columns: string[]
  rows: DatabaseValue[][]
  rowsCount: number
}

// 采集结果类型
export type CollectResult = RowData & {
  id?: string // 唯一标识符（从数据库加载时会有）
  name: string // 格式: dbName.tableName
  dataError?: string // 历史记录转换失败时保留列表和删除功能
  matchedType: string // 匹配类型，如 "phone", "idcard", "password" 等
}

export type DatabaseCollectRule = {
  id?: string
  label: string
  pattern: string
}
