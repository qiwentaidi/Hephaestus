import * as React from 'react'

import { cn } from '@/lib/utils'

type BubbleProps = React.HTMLAttributes<HTMLDivElement> & {
  variant?: 'default' | 'user' | 'muted' | 'warning' | 'success'
}

const Bubble = React.forwardRef<HTMLDivElement, BubbleProps>(
  ({ className, variant = 'default', ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        'text-sm leading-6',
        variant === 'default' && 'rounded-none border-0 bg-transparent p-0 text-card-foreground shadow-none',
        variant === 'user' && 'rounded-3xl border-0 bg-muted px-5 py-4 text-foreground shadow-none',
        variant === 'muted' && 'rounded-2xl border border-border bg-card px-5 py-4 text-muted-foreground shadow-sm',
        variant === 'warning' && 'rounded-2xl border border-amber-500/30 bg-amber-500/10 px-5 py-4 text-foreground shadow-sm',
        variant === 'success' && 'rounded-2xl border border-emerald-500/30 bg-emerald-500/10 px-5 py-4 text-foreground shadow-sm',
        className,
      )}
      data-slot="bubble"
      data-variant={variant}
      {...props}
    />
  ),
)
Bubble.displayName = 'Bubble'

export { Bubble }
