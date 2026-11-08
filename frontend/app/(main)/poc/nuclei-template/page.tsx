"use client"

import { useCallback, useEffect, useState } from "react"
import {
  readPendingNucleiTemplateImport,
  type NucleiTemplateWorkspacePayload,
} from "../../scan/repeater/nuclei-template"
import { NucleiTemplateWorkspace } from "../../scan/repeater/nuclei-template-workspace"

export default function Page() {
  const [payload, setPayload] = useState<NucleiTemplateWorkspacePayload | null>(null)
  const [loaded, setLoaded] = useState(false)

  const readWorkspacePayload = useCallback((force = false) => {
    try {
      const searchParams = new URLSearchParams(window.location.search)
      const source = searchParams.get("source")
      if (!force && source !== "repeater" && source !== "poc") {
        setPayload(null)
        return
      }

      setPayload(readPendingNucleiTemplateImport())
    } catch {
      setPayload(null)
    } finally {
      setLoaded(true)
    }
  }, [])

  useEffect(() => {
    const timerId = window.setTimeout(() => {
      readWorkspacePayload(false)
    }, 0)

    const handleOpenWorkspace = () => readWorkspacePayload(true)
    window.addEventListener("hephaestus:nuclei-template-workspace-open", handleOpenWorkspace)

    return () => {
      window.clearTimeout(timerId)
      window.removeEventListener("hephaestus:nuclei-template-workspace-open", handleOpenWorkspace)
    }
  }, [readWorkspacePayload])

  if (!loaded) {
    return <div className="h-full min-h-0 bg-background" />
  }

  return <NucleiTemplateWorkspace initialPayload={payload} />
}
