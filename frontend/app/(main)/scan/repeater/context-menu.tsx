"use client"

import type * as React from "react"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuShortcut,
  ContextMenuTrigger,
  ContextMenuSub,
  ContextMenuSubTrigger,
  ContextMenuSubContent,
} from "@/components/ui/context-menu"
import { Copy, Scissors, ClipboardPaste, Code2, FileText, Lock, Shuffle, Terminal, Unlock } from "lucide-react"
import { useI18n } from "@/lib/i18n"

interface RequestContextMenuProps {
  children: React.ReactNode
  onCopy: () => void
  onCopyUrl?: () => void
  onCut: () => void
  onPaste: () => void
  onUrlEncode: () => void
  onUrlDecode: () => void
  onBase64Encode: () => void
  onBase64Decode: () => void
  onUnicodeEncode: () => void
  onUnicodeDecode: () => void
  onSelectAll: () => void
  onConvertMethodToGet?: () => void
  onConvertMethodToPost?: () => void
  onConvertMethodToHead?: () => void
  onConvertToMultipart?: () => void
  onConvertToForm?: () => void
  onConvertToJson?: () => void
  onConvertToXml?: () => void
  onReplaceNbsp?: () => void
  onSpoofLocalIp?: () => void
  onSpoofRandomIp?: () => void
  onCopyAsCurl?: () => void
  copyAsCurlLabel?: string
}

interface ResponseContextMenuProps {
  children: React.ReactNode
  onCopy: () => void
  onSelectAll: () => void
  onCopyDownloadCurl?: () => void
}

// 将快捷键文本转换为符号表示
// ⌘ = Command, ⌃ = Control, ⇧ = Shift, ⌥ = Option
const formatShortcut = (shortcut: string): string => {
  return shortcut
    .replace(/Ctrl\+/g, "⌘")  // Command 键
    .replace(/Shift\+/g, "⇧")  // Shift 键
    .replace(/Alt\+/g, "⌥")    // Option 键
    .replace(/Control\+/g, "⌃") // Control 键
}

export function RequestContextMenu({
  children,
  onCopy,
  onCopyUrl,
  onCut,
  onPaste,
  onUrlEncode,
  onUrlDecode,
  onBase64Encode,
  onBase64Decode,
  onUnicodeEncode,
  onUnicodeDecode,
  onSelectAll,
  onConvertMethodToGet,
  onConvertMethodToPost,
  onConvertMethodToHead,
  onConvertToMultipart,
  onConvertToForm,
  onConvertToJson,
  onConvertToXml,
  onReplaceNbsp,
  onSpoofLocalIp,
  onSpoofRandomIp,
  onCopyAsCurl,
  copyAsCurlLabel,
}: RequestContextMenuProps) {
  const { t } = useI18n()

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-64">
        <ContextMenuItem onClick={onCut}>
          <Scissors className="mr-2 h-4 w-4" />
          {t.repeater.contextMenu.cut}
          <ContextMenuShortcut>{formatShortcut("Ctrl+X")}</ContextMenuShortcut>
        </ContextMenuItem>
        <ContextMenuItem onClick={onCopy}>
          <Copy className="mr-2 h-4 w-4" />
          {t.common.copy}
          <ContextMenuShortcut>{formatShortcut("Ctrl+C")}</ContextMenuShortcut>
        </ContextMenuItem>
        {onCopyUrl && (
          <ContextMenuItem onClick={onCopyUrl}>
            <Copy className="mr-2 h-4 w-4" />
            {t.repeater.contextMenu.copyUrl}
          </ContextMenuItem>
        )}
        {onCopyAsCurl && (
          <ContextMenuItem onClick={onCopyAsCurl}>
            <Terminal className="mr-2 h-4 w-4" />
            {copyAsCurlLabel || 'Copy as cURL'}
          </ContextMenuItem>
        )}
        <ContextMenuItem onClick={onPaste}>
          <ClipboardPaste className="mr-2 h-4 w-4" />
          {t.repeater.contextMenu.paste}
          <ContextMenuShortcut>{formatShortcut("Ctrl+V")}</ContextMenuShortcut>
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={onSelectAll}>
          <FileText className="mr-2 h-4 w-4" />
          {t.common.selectAll}
          <ContextMenuShortcut>{formatShortcut("Ctrl+A")}</ContextMenuShortcut>
        </ContextMenuItem>
        <ContextMenuSeparator />
        {(onConvertMethodToGet || onConvertMethodToPost || onConvertMethodToHead || onConvertToMultipart || onConvertToForm || onConvertToJson || onConvertToXml || onReplaceNbsp || onSpoofLocalIp || onSpoofRandomIp) && (
          <ContextMenuSub>
            <ContextMenuSubTrigger>
              <Shuffle className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.httpPacketMutation}
            </ContextMenuSubTrigger>
            <ContextMenuSubContent className="w-72">
              {onConvertMethodToGet && (
                <ContextMenuItem onClick={onConvertMethodToGet}>
                  {t.repeater.contextMenu.changeMethodToGet}
                </ContextMenuItem>
              )}
              {onConvertMethodToPost && (
                <ContextMenuItem onClick={onConvertMethodToPost}>
                  {t.repeater.contextMenu.changeMethodToPost}
                </ContextMenuItem>
              )}
              {onConvertMethodToHead && (
                <ContextMenuItem onClick={onConvertMethodToHead}>
                  {t.repeater.contextMenu.changeMethodToHead}
                </ContextMenuItem>
              )}
              {onConvertToMultipart && (
                <ContextMenuItem onClick={onConvertToMultipart}>
                  {t.repeater.contextMenu.convertToMultipart}
                </ContextMenuItem>
              )}
              {onConvertToForm && (
                <ContextMenuItem onClick={onConvertToForm}>
                  {t.repeater.contextMenu.convertToForm}
                </ContextMenuItem>
              )}
              {onConvertToJson && (
                <ContextMenuItem onClick={onConvertToJson}>
                  {t.repeater.contextMenu.convertToJson}
                </ContextMenuItem>
              )}
              {onConvertToXml && (
                <ContextMenuItem onClick={onConvertToXml}>
                  {t.repeater.contextMenu.convertToXml}
                </ContextMenuItem>
              )}
              {onReplaceNbsp && (
                <ContextMenuItem onClick={onReplaceNbsp}>
                  {t.repeater.contextMenu.replaceNbsp}
                </ContextMenuItem>
              )}
              {onSpoofLocalIp && (
                <ContextMenuItem onClick={onSpoofLocalIp}>
                  {t.repeater.contextMenu.spoofLocalIp}
                </ContextMenuItem>
              )}
              {onSpoofRandomIp && (
                <ContextMenuItem onClick={onSpoofRandomIp}>
                  {t.repeater.contextMenu.spoofRandomIp}
                </ContextMenuItem>
              )}
            </ContextMenuSubContent>
          </ContextMenuSub>
        )}
        <ContextMenuSub>
          <ContextMenuSubTrigger>
            <Code2 className="mr-2 h-4 w-4" />
            {t.repeater.contextMenu.urlEncode}
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <ContextMenuItem onClick={onUrlEncode}>
              <Lock className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.encode}
              <ContextMenuShortcut>{formatShortcut("Ctrl+U")}</ContextMenuShortcut>
            </ContextMenuItem>
            <ContextMenuItem onClick={onUrlDecode}>
              <Unlock className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.decode}
              <ContextMenuShortcut>{formatShortcut("Ctrl+Shift+U")}</ContextMenuShortcut>
            </ContextMenuItem>
          </ContextMenuSubContent>
        </ContextMenuSub>
        <ContextMenuSub>
          <ContextMenuSubTrigger>
            <Code2 className="mr-2 h-4 w-4" />
            {t.repeater.contextMenu.base64}
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <ContextMenuItem onClick={onBase64Encode}>
              <Lock className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.encode}
              <ContextMenuShortcut>{formatShortcut("Ctrl+B")}</ContextMenuShortcut>
            </ContextMenuItem>
            <ContextMenuItem onClick={onBase64Decode}>
              <Unlock className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.decode}
              <ContextMenuShortcut>{formatShortcut("Ctrl+Shift+B")}</ContextMenuShortcut>
            </ContextMenuItem>
          </ContextMenuSubContent>
        </ContextMenuSub>
        <ContextMenuSub>
          <ContextMenuSubTrigger>
            <Code2 className="mr-2 h-4 w-4" />
            {t.repeater.contextMenu.unicode}
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <ContextMenuItem onClick={onUnicodeEncode}>
              <Lock className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.encode}
              <ContextMenuShortcut>{formatShortcut("Ctrl+Shift+E")}</ContextMenuShortcut>
            </ContextMenuItem>
            <ContextMenuItem onClick={onUnicodeDecode}>
              <Unlock className="mr-2 h-4 w-4" />
              {t.repeater.contextMenu.decode}
              <ContextMenuShortcut>{formatShortcut("Ctrl+Shift+D")}</ContextMenuShortcut>
            </ContextMenuItem>
          </ContextMenuSubContent>
        </ContextMenuSub>
      </ContextMenuContent>
    </ContextMenu>
  )
}

export function ResponseContextMenu({
  children,
  onCopy,
  onSelectAll,
  onCopyDownloadCurl,
}: ResponseContextMenuProps) {
  const { t } = useI18n()

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-64">
        <ContextMenuItem onClick={onCopy}>
          <Copy className="mr-2 h-4 w-4" />
          {t.common.copy}
          <ContextMenuShortcut>{formatShortcut("Ctrl+C")}</ContextMenuShortcut>
        </ContextMenuItem>
        <ContextMenuItem onClick={onSelectAll}>
          <FileText className="mr-2 h-4 w-4" />
          {t.common.selectAll}
          <ContextMenuShortcut>{formatShortcut("Ctrl+A")}</ContextMenuShortcut>
        </ContextMenuItem>
        {onCopyDownloadCurl && <ContextMenuSeparator />}
        {onCopyDownloadCurl && (
          <ContextMenuItem onClick={onCopyDownloadCurl}>
            <Terminal className="mr-2 h-4 w-4" />
            {t.repeater.contextMenu.copyDownloadCurl}
          </ContextMenuItem>
        )}
      </ContextMenuContent>
    </ContextMenu>
  )
}
