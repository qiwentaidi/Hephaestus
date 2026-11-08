export interface DisplayFingerprint {
  fp: string
  isHighRisk: boolean
}

const cleanFingerprint = (value: unknown) => (
  String(value).trim().replace(/^["']|["']$/g, '')
)

export const parseFingerprintList = (fpStr?: string): string[] => {
  if (!fpStr || fpStr.toLowerCase() === 'null') return []

  try {
    const parsed = JSON.parse(fpStr)
    if (parsed === null) return []
    if (Array.isArray(parsed)) {
      return parsed
        .map(cleanFingerprint)
        .filter((value) => value && value.toLowerCase() !== 'null')
    }
  } catch {
    if (fpStr.includes(',')) {
      return fpStr
        .split(',')
        .map(cleanFingerprint)
        .filter((value) => value && value.toLowerCase() !== 'null')
    }
  }

  const cleaned = cleanFingerprint(fpStr)
  return cleaned && cleaned.toLowerCase() !== 'null' ? [cleaned] : []
}

export const buildDisplayFingerprints = (
  fingerprints?: string,
  highRiskFingerprints?: string,
): DisplayFingerprint[] => {
  const seen = new Set<string>()
  const result: DisplayFingerprint[] = []

  const add = (value: string, isHighRisk: boolean) => {
    const fp = cleanFingerprint(value)
    if (!fp) return

    const key = fp.toLowerCase()
    if (seen.has(key)) return

    seen.add(key)
    result.push({ fp, isHighRisk })
  }

  parseFingerprintList(highRiskFingerprints).forEach((fp) => add(fp, true))
  parseFingerprintList(fingerprints).forEach((fp) => add(fp, false))

  return result
}
