const domainLabelPattern = /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/
const ipv4Pattern = /^(\d{1,3}\.){3}\d{1,3}$/

/** Validate a bare domain and convert internationalized names to DNS-safe ASCII. */
export const normalizeSubdomainTarget = (value: string): string | null => {
  const target = value.trim()
  // Reject URL syntax and escaped/control characters before URL's IDNA conversion.
  if (!target || /[\s\u0000-\u001f\u007f/\\?#@:%\[\]]/.test(target)) return null

  try {
    const domain = new URL(`http://${target}`).hostname.replace(/\.$/, '')
    if (domain.length > 253 || ipv4Pattern.test(domain)) return null
    const labels = domain.split('.')
    if (labels.length < 2 || !labels.every((label) => domainLabelPattern.test(label))) return null
    return domain
  } catch {
    return null
  }
}

export const validateSubdomainTargets = (targets: readonly string[]) => {
  const validTargets: string[] = []
  const invalidTargets: string[] = []
  for (const rawTarget of targets) {
    const target = rawTarget.trim()
    if (!target) continue
    const domain = normalizeSubdomainTarget(target)
    if (domain) validTargets.push(domain)
    else invalidTargets.push(target)
  }
  return { validTargets, invalidTargets }
}
