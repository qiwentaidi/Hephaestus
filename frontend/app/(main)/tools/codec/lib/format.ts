// 格式转换函数

// JSON 格式化
export const jsonFormat = (input: string): string => {
  try {
    return JSON.stringify(JSON.parse(input), null, 2)
  } catch {
    return '[格式化错误: 无效的 JSON]'
  }
}

// JSON 压缩
export const jsonMinify = (input: string): string => {
  try {
    return JSON.stringify(JSON.parse(input))
  } catch {
    return '[压缩错误: 无效的 JSON]'
  }
}

