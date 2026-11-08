'use client'

import { useState } from 'react'
import Image from 'next/image'
import { ExternalLink, RotateCcw, X, ZoomIn, ZoomOut } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { useI18n } from '@/lib/i18n'

interface ScreenshotPreviewDialogProps {
  image: string | null
  onOpenChange: (open: boolean) => void
}

const MIN_ZOOM = 0.5
const MAX_ZOOM = 3
const ZOOM_STEP = 0.25
const DEFAULT_ZOOM = 0.75

function clampZoom(value: number) {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, value))
}

export function ScreenshotPreviewDialog({
  image,
  onOpenChange,
}: ScreenshotPreviewDialogProps) {
  const { t } = useI18n()
  const [zoomState, setZoomState] = useState({ image: '', value: DEFAULT_ZOOM })
  const zoom = image && zoomState.image === image ? zoomState.value : DEFAULT_ZOOM

  const adjustZoom = (delta: number) => {
    if (!image) return
    setZoomState((current) => {
      const currentZoom = current.image === image ? current.value : DEFAULT_ZOOM
      return {
        image,
        value: clampZoom(Number((currentZoom + delta).toFixed(2))),
      }
    })
  }

  return (
    <Dialog open={!!image} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className="inset-0 left-0 top-0 h-full w-full max-w-none translate-x-0 translate-y-0 border-none bg-transparent p-0 shadow-none"
      >
        <DialogHeader className="sr-only">
          <DialogTitle>{t.vulnScan.fingerprint.screenshotPreview}</DialogTitle>
          <DialogDescription>{t.vulnScan.fingerprint.clickToViewOriginal}</DialogDescription>
        </DialogHeader>

        {image && (
          <div className="relative h-screen w-screen overflow-hidden bg-black/45 backdrop-blur-[2px]">
            <DialogClose asChild>
              <Button
                variant="ghost"
                size="icon"
                className="absolute right-6 top-6 z-20 h-11 w-11 rounded-full border border-white/15 bg-black/35 text-white shadow-lg backdrop-blur-md hover:bg-black/55 hover:text-white"
              >
                <X className="h-5 w-5" />
              </Button>
            </DialogClose>

            <div
              className="flex h-full w-full items-center justify-center overflow-auto px-8 py-8"
              onWheel={(event) => {
                event.preventDefault()
                adjustZoom(event.deltaY > 0 ? -ZOOM_STEP : ZOOM_STEP)
              }}
            >
              <Image
                src={image}
                alt={t.vulnScan.fingerprint.screenshotAlt}
                width={1920}
                height={1080}
                unoptimized
                className="max-w-none shadow-2xl transition-transform duration-150"
                style={{
                  transform: `scale(${zoom})`,
                  transformOrigin: 'center center',
                }}
              />
            </div>

            <div className="pointer-events-none absolute inset-x-0 bottom-8 z-20 flex justify-center px-4">
              <div className="pointer-events-auto flex flex-wrap items-center justify-center gap-2 rounded-full border border-white/12 bg-black/40 px-3 py-2 text-white shadow-xl backdrop-blur-xl">
                <div className="px-2 text-sm text-white/80">
                  {Math.round(zoom * 100)}%
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 rounded-full text-white/90 hover:bg-white/10 hover:text-white"
                  onClick={() => adjustZoom(-ZOOM_STEP)}
                  disabled={zoom <= MIN_ZOOM}
                >
                  <ZoomOut className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 rounded-full text-white/90 hover:bg-white/10 hover:text-white"
                  onClick={() => adjustZoom(ZOOM_STEP)}
                  disabled={zoom >= MAX_ZOOM}
                >
                  <ZoomIn className="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 rounded-full text-white/90 hover:bg-white/10 hover:text-white"
                  onClick={() => image && setZoomState({ image, value: DEFAULT_ZOOM })}
                  disabled={zoom === DEFAULT_ZOOM}
                >
                  <RotateCcw className="h-4 w-4" />
                </Button>
                <div className="mx-1 h-5 w-px bg-white/15" />
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 rounded-full text-white/90 hover:bg-white/10 hover:text-white"
                  onClick={() => window.open(image, '_blank', 'noopener,noreferrer')}
                >
                  <ExternalLink className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
