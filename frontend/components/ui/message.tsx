import * as React from 'react'

import { cn } from '@/lib/utils'

type MessageProps = React.HTMLAttributes<HTMLDivElement> & {
  role?: 'user' | 'assistant' | 'system'
}

const Message = React.forwardRef<HTMLDivElement, MessageProps>(
  ({ className, role = 'assistant', ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        'group flex w-full gap-3',
        role === 'user' && 'justify-end',
        role === 'system' && 'justify-center',
        className,
      )}
      data-slot="message"
      data-role={role}
      {...props}
    />
  ),
)
Message.displayName = 'Message'

const MessageAvatar = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        'mt-1 flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-muted text-muted-foreground',
        className,
      )}
      data-slot="message-avatar"
      {...props}
    />
  ),
)
MessageAvatar.displayName = 'MessageAvatar'

const MessageContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn('min-w-0 max-w-[860px] flex-1', className)} data-slot="message-content" {...props} />
  ),
)
MessageContent.displayName = 'MessageContent'

const MessageHeader = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div
      ref={ref}
      className={cn('mb-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground', className)}
      data-slot="message-header"
      {...props}
    />
  ),
)
MessageHeader.displayName = 'MessageHeader'

export { Message, MessageAvatar, MessageContent, MessageHeader }
