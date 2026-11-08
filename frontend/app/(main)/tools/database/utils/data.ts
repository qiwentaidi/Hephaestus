import type { CollectResult, DatabaseValue, RowData } from '../types'

function isDatabaseValue(value: unknown, depth = 0): value is DatabaseValue {
  if (depth > 128) return false
  if (value === null || typeof value === 'string' || typeof value === 'boolean') return true
  if (typeof value === 'number') return Number.isFinite(value)
  if (Array.isArray(value)) return value.every(cell => isDatabaseValue(cell, depth + 1))
  return typeof value === 'object' && Object.values(value).every(cell => isDatabaseValue(cell, depth + 1))
}

// All API and preview inputs are validated before React receives table values.
export function parseRowData(value: unknown): RowData {
  if (!value || typeof value !== 'object') throw new Error('Invalid collected table data')
  const data = value as Record<string, unknown>
  if (data.dataError) throw new Error(String(data.dataError))
  if (!Array.isArray(data.columns) || !data.columns.every(column => typeof column === 'string') ||
      !Array.isArray(data.rows) || !Number.isSafeInteger(data.rowsCount) || (data.rowsCount as number) < 0) {
    throw new Error('Invalid collected table data')
  }
  const columns = data.columns as string[]
  if (!data.rows.every(row => Array.isArray(row) && row.length === columns.length && row.every(cell => isDatabaseValue(cell)))) {
    throw new Error('Invalid collected table rows')
  }
  return { columns, rows: data.rows as DatabaseValue[][], rowsCount: data.rowsCount as number }
}

export function formatDatabaseValue(value: DatabaseValue): string {
  if (value === null) return 'NULL'
  if (typeof value === 'object') return JSON.stringify(value)
  // A Redis string that resembles Base64 remains a string. Actual SQL byte
  // values are normalized by the backend, not guessed from their contents.
  return String(value)
}

export function collectResultDetailURL(pane: CollectResult, connectionNanoid: string): string {
  const params = new URLSearchParams({ connectionNanoid })
  if (pane.id) {
    params.set('id', pane.id)
  } else {
    params.set('data', btoa(encodeURIComponent(JSON.stringify(parseRowData(pane)))))
    params.set('tableName', pane.name)
    if (typeof pane.matchedType === 'string') params.set('matchedType', pane.matchedType)
  }
  return `/tools/database/detail?${params.toString()}`
}

export function decodeTablePreview(encoded: string): RowData {
  return parseRowData(JSON.parse(decodeURIComponent(atob(encoded))))
}
