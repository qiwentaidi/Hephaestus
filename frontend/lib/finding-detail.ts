export function findingDetailHref(id: string, from = '/vulnerabilities') {
  return `/vulnerabilities/detail?${new URLSearchParams({ id, from })}`
}

export function findingReturnHref(value: string | null) {
  return value && /^\/(?:dashboard|vulnerabilities|scan\/(?:vuln-scan|js-scan)\/detail|ai\/workbench(?:\/(?:exploration|project))?)(?:\?|$)/.test(value) ? value : '/vulnerabilities'
}
