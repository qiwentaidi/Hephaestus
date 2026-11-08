import * as React from 'react'

const MOBILE_BREAKPOINT = 768
const MOBILE_QUERY = `(max-width: ${MOBILE_BREAKPOINT - 1}px)`

const getMobileSnapshot = () => {
  if (typeof window === 'undefined') {
    return false
  }

  return window.matchMedia(MOBILE_QUERY).matches
}

const subscribeToMobileChanges = (onStoreChange: () => void) => {
  if (typeof window === 'undefined') {
    return () => {}
  }

  const mql = window.matchMedia(MOBILE_QUERY)
  mql.addEventListener('change', onStoreChange)
  return () => mql.removeEventListener('change', onStoreChange)
}

export function useIsMobile() {
  return React.useSyncExternalStore(
    subscribeToMobileChanges,
    getMobileSnapshot,
    () => false,
  )
}
