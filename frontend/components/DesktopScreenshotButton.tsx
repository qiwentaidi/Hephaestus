"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { Camera, Copy, Download, Loader2 } from "lucide-react";
import toast from "react-hot-toast";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { copyImageToClipboard } from "@/lib/clipboard";
import { saveBlobAsFile } from "@/lib/download";
import { useI18n } from "@/lib/i18n";
import {
  captureAppWindow,
  captureNativeAppWindow,
  reportScreenshotCompletion,
  screenshotBlob,
  screenshotFileName,
  type AppScreenshot,
} from "@/lib/screenshot";

// The desktop backend dispatches this event through the Wails webview when an
// MCP client calls the desktop_screenshot tool.
export const DESKTOP_SCREENSHOT_REQUEST_EVENT = "hephaestus:desktop-screenshot-request";
export const DESKTOP_SCREENSHOT_TAKEN_EVENT = "hephaestus:desktop-screenshot-taken";

type ScreenshotRequestDetail = {
  requestId?: string;
  scale?: number;
};

const SCREENSHOT_CAPTURE_CLASS = "hephaestus-screenshot-capture";

function waitForNextPaint(): Promise<void> {
  if (typeof requestAnimationFrame !== "function") {
    return Promise.resolve();
  }
  return new Promise((resolve) => {
    requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
  });
}

export function DesktopScreenshotButton({ visible }: { visible: boolean }) {
  const { t, language } = useI18n();
  const [open, setOpen] = useState(false);
  const [screenshot, setScreenshot] = useState<AppScreenshot | null>(null);
  const [saving, setSaving] = useState(false);
  const [copying, setCopying] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const capturingRef = useRef(false);

  const takeScreenshot = useCallback(async (scale?: number) => {
    if (capturingRef.current) {
      return null;
    }
    capturingRef.current = true;
    try {
      return await captureAppWindow(scale === undefined ? {} : { scale });
    } finally {
      capturingRef.current = false;
    }
  }, []);

  const handleOpenPreview = useCallback(async () => {
    if (capturingRef.current) {
      return;
    }
    capturingRef.current = true;
    const root = document.documentElement;
    let loadingToast: string | undefined;

    try {
      // The native snapshot captures the live webview, so hide transient
      // overlays (tooltip/toast) before asking the host to capture.
      root.classList.add(SCREENSHOT_CAPTURE_CLASS);
      await waitForNextPaint();

      const native = await captureNativeAppWindow({ scale: 2 });
      if (native) {
        setScreenshot(native);
        setOpen(true);
        return;
      }

      // Native capture is unavailable (for example on Linux): restore the
      // overlays and fall back to DOM capture, which filters them itself.
      root.classList.remove(SCREENSHOT_CAPTURE_CLASS);
      loadingToast = toast.loading(t.desktopTitlebar.screenshotCapturing);
      const fallback = await captureAppWindow({ scale: 2 });
      if (!fallback) {
        toast.error(t.desktopTitlebar.screenshotFailed);
        return;
      }
      setScreenshot(fallback);
      setOpen(true);
    } catch {
      toast.error(t.desktopTitlebar.screenshotFailed);
    } finally {
      root.classList.remove(SCREENSHOT_CAPTURE_CLASS);
      if (loadingToast) {
        toast.dismiss(loadingToast);
      }
      capturingRef.current = false;
    }
  }, [t]);

  useEffect(() => {
    const handleTaken = () => {
      toast.success(t.desktopTitlebar.screenshotForAI);
    };
    window.addEventListener(DESKTOP_SCREENSHOT_TAKEN_EVENT, handleTaken);
    return () => {
      window.removeEventListener(DESKTOP_SCREENSHOT_TAKEN_EVENT, handleTaken);
    };
  }, [t]);

  useEffect(() => {
    const handleRequest = async (event: Event) => {
      const detail = (event as CustomEvent<ScreenshotRequestDetail>).detail;
      const requestId = String(detail?.requestId || "").trim();
      if (!requestId) {
        return;
      }

      try {
        const next = await takeScreenshot(typeof detail?.scale === "number" ? detail.scale : 1);
        if (!next) {
          throw new Error("another capture is already running");
        }
        await reportScreenshotCompletion({
          requestId,
          dataUrl: next.dataUrl,
          width: next.width,
          height: next.height,
          route: window.location.pathname,
          title: document.title,
        });
        toast.success(t.desktopTitlebar.screenshotForAI);
      } catch (error) {
        try {
          await reportScreenshotCompletion({
            requestId,
            error: error instanceof Error ? error.message : String(error),
          });
        } catch {
          // The pending tool call may have timed out, or the window is closing.
        }
        toast.error(t.desktopTitlebar.screenshotFailed);
      }
    };

    window.addEventListener(DESKTOP_SCREENSHOT_REQUEST_EVENT, handleRequest);
    return () => {
      window.removeEventListener(DESKTOP_SCREENSHOT_REQUEST_EVENT, handleRequest);
    };
  }, [t, takeScreenshot]);

  const handleCopy = useCallback(async () => {
    if (!screenshot || saving || copying) {
      return;
    }
    setCopying(true);
    try {
      const copied = await copyImageToClipboard(screenshot.dataUrl);
      if (!copied) {
        toast.error(t.desktopTitlebar.screenshotCopyFailed);
        return;
      }
      setOpen(false);
      toast.success(t.desktopTitlebar.screenshotCopied);
    } finally {
      setCopying(false);
    }
  }, [screenshot, saving, copying, t]);

  const handleSave = useCallback(async () => {
    if (!screenshot || saving || copying) {
      return;
    }
    setSaving(true);
    try {
      const blob = await screenshotBlob(screenshot);
      const result = await saveBlobAsFile(blob, screenshotFileName(new Date(screenshot.capturedAt)));
      if (result === "saved") {
        toast.success(t.desktopTitlebar.screenshotSaved);
      }
    } catch {
      toast.error(t.desktopTitlebar.screenshotSaveFailed);
    } finally {
      setSaving(false);
    }
  }, [screenshot, saving, copying, t]);

  const meta = screenshot
    ? `${new Date(screenshot.capturedAt).toLocaleString(language === "zh" ? "zh-CN" : "en-US", {
        hour12: false,
      })} · ${screenshot.width} × ${screenshot.height} · PNG${
        process.env.NODE_ENV === "development" ? ` · ${screenshot.source}` : ""
      }`
    : "";

  if (!visible) {
    return null;
  }

  return (
    <>
      <Tooltip open={tooltipOpen} onOpenChange={setTooltipOpen}>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            aria-label={t.desktopTitlebar.screenshot}
            onClick={() => {
              setTooltipOpen(false);
              void handleOpenPreview();
            }}
          >
            <Camera className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t.desktopTitlebar.screenshot}</TooltipContent>
      </Tooltip>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent
          data-screenshot-ignore="true"
          className="max-w-[calc(100vw-48px)] gap-0 overflow-hidden p-0 sm:max-w-[920px]"
        >
          <div className="border-b border-border/70 px-5 py-3.5 pr-14">
            <DialogTitle className="text-sm font-semibold">{t.desktopTitlebar.screenshotPreview}</DialogTitle>
            <DialogDescription className="mt-1 text-xs tabular-nums text-muted-foreground">
              {meta}
            </DialogDescription>
          </div>
          <div className="bg-muted/30 p-4">
            {screenshot && (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={screenshot.dataUrl}
                alt={t.desktopTitlebar.screenshotPreview}
                className="mx-auto max-h-[62vh] w-auto max-w-full rounded-md border border-border/70 shadow-sm"
              />
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2 border-t border-border/70 px-5 py-3">
            <p className="mr-auto text-xs text-muted-foreground">{t.desktopTitlebar.screenshotHint}</p>
            <Button
              variant="outline"
              size="sm"
              disabled={!screenshot || saving || copying}
              onClick={() => void handleSave()}
            >
              {saving ? <Loader2 className="animate-spin" /> : <Download />}
              {t.desktopTitlebar.screenshotSave}
            </Button>
            <Button size="sm" disabled={!screenshot || saving || copying} onClick={() => void handleCopy()}>
              {copying ? <Loader2 className="animate-spin" /> : <Copy />}
              {t.desktopTitlebar.screenshotCopy}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
