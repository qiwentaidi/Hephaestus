'use client'

import dynamic from 'next/dynamic'
import type { KeepAliveProps } from 'react-activation'

// SSRKeepAlive reads `this.props` in a function component. The DOM-based cache
// is only needed in the browser, so do not execute that broken SSR branch.
const BrowserKeepAlive = dynamic<KeepAliveProps>(
  () => import('react-activation').then(module => module.KeepAlive),
  { ssr: false },
)

export function ClientKeepAlive({ name, ...props }: KeepAliveProps & { name: string }) {
  // A shared wrapper needs a stable key per page. autoFreeze is incompatible
  // with concurrent React roots; disable it without discarding cached state.
  return <BrowserKeepAlive {...props} name={name} cacheKey={props.cacheKey ?? name} autoFreeze={false} />
}
