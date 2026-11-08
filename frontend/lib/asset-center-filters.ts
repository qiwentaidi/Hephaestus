export interface AssetInventoryFilters {
  keyword: string
  type: string
  status: string
  port: string
  service: string
  technology: string
}

export function buildAssetTopologyQuery(filters: AssetInventoryFilters, page: number, pageSize: number) {
  return new URLSearchParams({
    keyword: filters.keyword,
    type: filters.type,
    status: filters.status,
    port: filters.port.trim(),
    service: filters.service,
    technology: filters.technology,
    page: String(page),
    pageSize: String(pageSize),
  })
}

