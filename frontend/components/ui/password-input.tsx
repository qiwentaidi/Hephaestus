'use client'

import * as React from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

export interface PasswordInputProps
  extends Omit<React.ComponentProps<typeof Input>, 'type'> {
  showToggle?: boolean
  inputClassName?: string
}

const PasswordInput = React.forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ className, inputClassName, showToggle = true, ...props }, ref) => {
    const [showPassword, setShowPassword] = React.useState(false)

    return (
      <div className={cn('relative', className)}>
        <Input
          type={showPassword ? 'text' : 'password'}
          className={cn('pr-9', inputClassName)} // 给按钮留出空间
          ref={ref}
          {...props}
        />

        {showToggle && (
          <button
            type="button"
            onClick={() => setShowPassword(!showPassword)}
            disabled={props.disabled}
            tabIndex={0}
            className={cn(
              'absolute right-2 top-1/2 -translate-y-1/2 flex items-center justify-center',
              'h-7 w-7 rounded-md',
              'text-muted-foreground hover:text-foreground transition-colors',
              'outline-none focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-offset-1',
              'disabled:pointer-events-none disabled:opacity-50'
            )}
          >
            {showPassword ? (
              <EyeOff className="h-4 w-4" />
            ) : (
              <Eye className="h-4 w-4" />
            )}
            <span className="sr-only">{showPassword ? '隐藏密码' : '显示密码'}</span>
          </button>
        )}
      </div>
    )
  }
)

PasswordInput.displayName = 'PasswordInput'

export { PasswordInput }
