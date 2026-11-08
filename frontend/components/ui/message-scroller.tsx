'use client'

import * as React from 'react'
import { ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'

type MessageScrollerProps = React.HTMLAttributes<HTMLDivElement> & {
  follow?: boolean
}

const MessageScroller = React.forwardRef<HTMLDivElement, MessageScrollerProps>(
  ({ className, children, follow = true, ...props }, forwardedRef) => {
    const localRef = React.useRef<HTMLDivElement | null>(null)
    const [isAtBottom, setIsAtBottom] = React.useState(true)

    const setRefs = React.useCallback(
      (node: HTMLDivElement | null) => {
        localRef.current = node
        if (typeof forwardedRef === 'function') {
          forwardedRef(node)
        } else if (forwardedRef) {
          forwardedRef.current = node
        }
      },
      [forwardedRef],
    )

    const scrollToBottom = React.useCallback((behavior: ScrollBehavior = 'smooth') => {
      const el = localRef.current
      if (!el) return
      el.scrollTo({ top: el.scrollHeight, behavior })
    }, [])

    React.useEffect(() => {
      if (follow && isAtBottom) {
        scrollToBottom('auto')
      }
    }, [children, follow, isAtBottom, scrollToBottom])

    const handleScroll = React.useCallback(() => {
      const el = localRef.current
      if (!el) return
      const distance = el.scrollHeight - el.scrollTop - el.clientHeight
      setIsAtBottom(distance < 80)
    }, [])

    return (
      <div className="relative min-h-0 flex-1 overflow-hidden">
        <div
          ref={setRefs}
          className={cn('h-full min-h-0 overflow-y-auto scroll-smooth px-5 py-5', className)}
          onScroll={handleScroll}
          data-slot="message-scroller"
          {...props}
        >
          {children}
        </div>
        {!isAtBottom && (
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            className="absolute bottom-4 right-4 rounded-full shadow-md"
            onClick={() => scrollToBottom()}
            aria-label="滚动到底部"
          >
            <ChevronDown className="h-4 w-4" />
          </Button>
        )}
      </div>
    )
  },
)
MessageScroller.displayName = 'MessageScroller'

export { MessageScroller }
