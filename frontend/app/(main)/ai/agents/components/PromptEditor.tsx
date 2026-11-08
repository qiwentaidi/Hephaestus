'use client'

import { useEffect, useMemo, useState } from 'react'
import Editor, { DiffEditor } from '@monaco-editor/react'
import { Loader2 } from 'lucide-react'
import { useTheme } from 'next-themes'

import { getAppThemeBase } from '@/lib/theme/custom-theme'
import '@/lib/monaco'

const DARK_THEME = 'hephaestus-prompt-dark'
const LIGHT_THEME = 'hephaestus-prompt-light'

const DARK_COLORS = {
  'editor.background': '#0e0e10',
  'editor.foreground': '#d7d7de',
  'editorLineNumber.foreground': '#6a6a74',
  'editorLineNumber.activeForeground': '#a8a8b2',
  'editor.lineHighlightBackground': '#ffffff08',
  'editor.selectionBackground': '#ffffff24',
  'editor.inactiveSelectionBackground': '#ffffff10',
  'editorCursor.foreground': '#f0f0f2',
  'editorIndentGuide.background1': '#ffffff0d',
  'editorIndentGuide.activeBackground1': '#ffffff1a',
  'editorWidget.background': '#151517',
  'editorWidget.border': '#2a2a2f',
  'editorSuggestWidget.background': '#151517',
  'editorSuggestWidget.border': '#2a2a2f',
  'editorHoverWidget.background': '#151517',
  'editorHoverWidget.border': '#2a2a2f',
  'editorGutter.background': '#0e0e10',
  'editorOverviewRuler.border': '#00000000',
  'scrollbarSlider.background': '#ffffff14',
  'scrollbarSlider.hoverBackground': '#ffffff22',
  'scrollbarSlider.activeBackground': '#ffffff2e',
  'diffEditor.insertedTextBackground': '#3f9c6d22',
  'diffEditor.removedTextBackground': '#b46a6422',
  'diffEditor.insertedLineBackground': '#3f9c6d16',
  'diffEditor.removedLineBackground': '#b46a6416',
}

const LIGHT_COLORS = {
  'editor.background': '#ffffff',
  'editor.foreground': '#24292f',
  'editorLineNumber.foreground': '#a3a6ad',
  'editorLineNumber.activeForeground': '#5c6066',
  'editor.lineHighlightBackground': '#00000005',
  'editor.selectionBackground': '#00000018',
  'editor.inactiveSelectionBackground': '#00000008',
  'editorCursor.foreground': '#24292f',
  'editorIndentGuide.background1': '#0000000d',
  'editorIndentGuide.activeBackground1': '#0000001a',
  'editorWidget.background': '#ffffff',
  'editorWidget.border': '#d9d9e0',
  'editorSuggestWidget.background': '#ffffff',
  'editorSuggestWidget.border': '#d9d9e0',
  'editorHoverWidget.background': '#ffffff',
  'editorHoverWidget.border': '#d9d9e0',
  'editorGutter.background': '#ffffff',
  'editorOverviewRuler.border': '#00000000',
  'scrollbarSlider.background': '#00000012',
  'scrollbarSlider.hoverBackground': '#00000022',
  'scrollbarSlider.activeBackground': '#00000030',
  'diffEditor.insertedTextBackground': '#2f9e5f22',
  'diffEditor.removedTextBackground': '#c0504a22',
  'diffEditor.insertedLineBackground': '#2f9e5f16',
  'diffEditor.removedLineBackground': '#c0504a16',
}

function definePromptThemes(monaco: unknown) {
  const instance = monaco as {
    editor: {
      defineTheme: (name: string, theme: Record<string, unknown>) => void
    }
  }
  instance.editor.defineTheme(DARK_THEME, {
    base: 'vs-dark',
    inherit: true,
    rules: [
      { token: 'keyword', foreground: 'f0f0f2', fontStyle: 'bold' },
      { token: 'string', foreground: '9fc0ad' },
      { token: 'variable', foreground: '9db4d6' },
      { token: 'comment', foreground: '66666f', fontStyle: 'italic' },
      { token: 'emphasis', fontStyle: 'italic' },
      { token: 'strong', fontStyle: 'bold' },
    ],
    colors: DARK_COLORS,
  })
  instance.editor.defineTheme(LIGHT_THEME, {
    base: 'vs',
    inherit: true,
    rules: [
      { token: 'keyword', foreground: '1f2329', fontStyle: 'bold' },
      { token: 'string', foreground: '18794e' },
      { token: 'variable', foreground: '2868d8' },
      { token: 'comment', foreground: '8b8f96', fontStyle: 'italic' },
      { token: 'emphasis', fontStyle: 'italic' },
      { token: 'strong', fontStyle: 'bold' },
    ],
    colors: LIGHT_COLORS,
  })
}

interface AgentPromptEditorProps {
  role: string
  value: string
  defaultValue: string
  compare: boolean
  wrap: boolean
  readOnly?: boolean
  onChange: (value: string) => void
}

const loading = (
  <div className="flex h-full items-center justify-center">
    <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
  </div>
)

export default function AgentPromptEditor({
  role,
  value,
  defaultValue,
  compare,
  wrap,
  readOnly = false,
  onChange,
}: AgentPromptEditorProps) {
  const { resolvedTheme } = useTheme()
  const [mounted, setMounted] = useState(false)

  useEffect(() => {
    setMounted(true)
  }, [])

  const theme = useMemo(
    () => (getAppThemeBase(resolvedTheme || 'dark') === 'dark' ? DARK_THEME : LIGHT_THEME),
    [resolvedTheme],
  )

  const options = useMemo(
    () => ({
      automaticLayout: true,
      readOnly,
      fontFamily: "'SFMono-Regular', Menlo, 'Cascadia Code', 'Liberation Mono', monospace",
      fontSize: 13,
      lineHeight: 24,
      letterSpacing: 0.1,
      wordWrap: wrap ? ('on' as const) : ('off' as const),
      wrappingIndent: 'same' as const,
      minimap: { enabled: false },
      lineNumbersMinChars: 3,
      lineDecorationsWidth: 8,
      glyphMargin: false,
      folding: true,
      renderLineHighlight: 'all' as const,
      renderWhitespace: 'none' as const,
      unicodeHighlight: {
        ambiguousCharacters: false,
        invisibleCharacters: false,
        nonBasicASCII: false,
        includeComments: false,
        includeStrings: false,
      },
      scrollBeyondLastLine: false,
      smoothScrolling: true,
      cursorBlinking: 'smooth' as const,
      cursorSmoothCaretAnimation: 'on' as const,
      padding: { top: 14, bottom: 16 },
      tabSize: 2,
      overviewRulerLanes: 0,
      hideCursorInOverviewRuler: true,
      scrollbar: {
        verticalScrollbarSize: 8,
        horizontalScrollbarSize: 8,
        useShadows: false,
      },
    }),
    [wrap, readOnly],
  )

  const diffOptions = useMemo(
    () => ({
      automaticLayout: true,
      readOnly: true,
      originalEditable: false,
      renderSideBySide: false,
      enableSplitViewResizing: false,
      ignoreTrimWhitespace: false,
      renderIndicators: true,
      minimap: { enabled: false },
      fontFamily: "'SFMono-Regular', Menlo, 'Cascadia Code', 'Liberation Mono', monospace",
      fontSize: 13,
      lineHeight: 23,
      wordWrap: 'on' as const,
      diffWordWrap: 'on' as const,
      unicodeHighlight: {
        ambiguousCharacters: false,
        invisibleCharacters: false,
        nonBasicASCII: false,
        includeComments: false,
        includeStrings: false,
      },
      scrollBeyondLastLine: false,
      padding: { top: 12, bottom: 12 },
      overviewRulerLanes: 0,
      renderOverviewRuler: false,
      scrollbar: {
        verticalScrollbarSize: 8,
        horizontalScrollbarSize: 8,
        useShadows: false,
      },
    }),
    [],
  )

  if (!mounted || !resolvedTheme) {
    return loading
  }

  if (compare) {
    return (
      <DiffEditor
        height="100%"
        width="100%"
        language="markdown"
        theme={theme}
        original={defaultValue}
        modified={value}
        // @monaco-editor/react disposes diff models before the widget on unmount,
        // which throws "TextModel got disposed..." when toggling back to edit.
        // Keep the role-scoped models alive; onMount refreshes their content.
        originalModelPath={`inmemory://agent-prompt-diff/${role}/default.md`}
        modifiedModelPath={`inmemory://agent-prompt-diff/${role}/current.md`}
        keepCurrentOriginalModel
        keepCurrentModifiedModel
        beforeMount={definePromptThemes}
        onMount={(editor) => {
          const model = editor.getModel()
          if (!model) return
          if (model.original.getValue() !== defaultValue) model.original.setValue(defaultValue)
          if (model.modified.getValue() !== value) model.modified.setValue(value)
        }}
        options={diffOptions}
        loading={loading}
      />
    )
  }

  return (
    <Editor
      height="100%"
      width="100%"
      path={`agent-prompt-${role}.md`}
      language="markdown"
      theme={theme}
      value={value}
      beforeMount={definePromptThemes}
      onChange={(next) => onChange(next ?? '')}
      options={options}
      loading={loading}
    />
  )
}
