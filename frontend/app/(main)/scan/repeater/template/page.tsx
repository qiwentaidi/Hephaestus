"use client"

import { useEffect, useState } from "react"
import {
  readPendingNucleiTemplateImport,
  type NucleiTemplateWorkspacePayload,
} from "../nuclei-template"
import { NucleiTemplateWorkspace } from "../nuclei-template-workspace"

export default function Page() {
  const [payload, setPayload] = useState<NucleiTemplateWorkspacePayload | null>(null)
  const [loaded, setLoaded] = useState(false)

  useEffect(() => {
    const timerId = window.setTimeout(() => {
      try {
        setPayload(readPendingNucleiTemplateImport())
      } catch {
        setPayload(null)
      } finally {
        setLoaded(true)
      }
    }, 0)

    return () => window.clearTimeout(timerId)
  }, [])

  if (!loaded) {
    return <div className="h-full min-h-0 bg-background" />
  }

  return <NucleiTemplateWorkspace initialPayload={payload} />
}
