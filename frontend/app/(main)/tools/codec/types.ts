// 操作类型定义
export interface Operation {
  id: string
  name: string
  category: string
  description: string
  icon: React.ReactNode
  encode?: (input: string, options?: Record<string, string>) => string
  decode?: (input: string, options?: Record<string, string>) => string
  transform?: (input: string, options?: Record<string, string>) => string
  options?: OperationOption[]
}

export interface OperationOption {
  key: string
  label: string
  default: string
  type?: 'input' | 'textarea' | 'select' | 'switch'  // 默认为 input
  options?: Array<string | { label: string; value: string }> // select 类型的选项列表
  visibleWhen?: { key: string; value: string }
}

// 已添加的操作
export interface AddedOperation {
  id: string
  operationId: string
  mode: 'encode' | 'decode' | 'transform'
  options: Record<string, string>
}

// 分类
export interface Category {
  id: string
  name: string
  icon: React.ReactNode
}
