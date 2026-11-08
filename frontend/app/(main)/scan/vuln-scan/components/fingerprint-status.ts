export function matchesFingerprintStatus(status: number, filter: string): boolean {
  if (filter === 'all' || filter === '') return true
  if (/^[2-5]xx$/.test(filter)) {
    const lowerBound = Number(filter[0]) * 100
    return status >= lowerBound && status < lowerBound + 100
  }
  return status === Number(filter)
}
