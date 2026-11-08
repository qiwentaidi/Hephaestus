'use client'

import { useEffect, useState } from 'react'
import { getBrowserRuntimeInfo } from '@/lib/api/browser-runtime'

export function useScreenshotSnapChromeWarning() {
  const [showWarning, setShowWarning] = useState(false)

  useEffect(() => {
    let cancelled = false

    getBrowserRuntimeInfo().then((runtimeInfo) => {
      if (cancelled) {
        return
      }
      setShowWarning(runtimeInfo?.isLinux === true && runtimeInfo.snapChromeDetected === true)
    })

    return () => {
      cancelled = true
    }
  }, [])

  return showWarning
}
