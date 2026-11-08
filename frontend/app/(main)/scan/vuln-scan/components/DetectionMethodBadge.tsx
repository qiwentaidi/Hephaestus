import { Badge } from '@/components/ui/badge'

interface DetectionMethodBadgeProps {
  principle?: boolean
  labels: { principleDetection: string; actualExploit: string }
}

export function DetectionMethodBadge({ principle, labels }: DetectionMethodBadgeProps) {
  return (
    <Badge
      variant="outline"
      className={principle
        ? 'bg-tone-amber-soft border-tone-amber-border text-tone-amber'
        : 'bg-tone-green-soft border-tone-green-border text-tone-green'}
    >
      {principle ? labels.principleDetection : labels.actualExploit}
    </Badge>
  )
}
