import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'

import { cn } from '@/lib/utils'

type MarkerProps = React.HTMLAttributes<HTMLDivElement> & {
  variant?: 'default' | 'border' | 'separator' | 'loading' | 'warning' | 'success'
  asChild?: boolean
}

const Marker = React.forwardRef<HTMLDivElement, MarkerProps>(
  ({ className, variant = 'default', asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : 'div'
    return <Comp
      ref={ref}
      className={cn(
        'flex min-w-0 items-center gap-2 text-sm text-muted-foreground',
        variant === 'border' && 'border-b border-border py-2',
        variant === 'separator' && 'w-full py-2 before:h-px before:flex-1 before:bg-border after:h-px after:flex-1 after:bg-border',
        variant === 'loading' && 'rounded-md border border-border bg-muted/30 px-3 py-2',
        variant === 'warning' && 'rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-amber-700 dark:text-amber-300',
        variant === 'success' && 'rounded-md border border-emerald-500/30 bg-emerald-500/10 px-3 py-2 text-emerald-700 dark:text-emerald-300',
        className,
      )}
      {...props}
      data-slot="marker"
      data-variant={variant}
    />
  },
)
Marker.displayName = 'Marker'

function MarkerIcon({ className, ...props }: React.ComponentProps<'span'>) {
  return <span data-slot="marker-icon" {...props} aria-hidden="true" className={cn('flex size-4 shrink-0 items-center justify-center [&_svg]:size-4', className)} />
}

function MarkerContent({ className, ...props }: React.ComponentProps<'span'>) {
  return <span data-slot="marker-content" className={cn('min-w-0', className)} {...props} />
}

export { Marker, MarkerIcon, MarkerContent }
