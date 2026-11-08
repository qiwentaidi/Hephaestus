"use client"

import { loader } from "@monaco-editor/react"

let configured = false

export function configureMonacoLoader() {
  if (configured) {
    return
  }

  loader.config({
    paths: {
      vs: "/monaco-editor/vs",
    },
  })
  configured = true
}

configureMonacoLoader()
