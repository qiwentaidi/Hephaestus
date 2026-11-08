"use client";

import { useId, useRef, useState, type RefObject } from "react";
import { ExternalLink, Star } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

const repositoryUrl = "https://github.com/qiwentaidi/Hephaestus";

export function ProductRatingDialog({
  open,
  onOpenChange,
  triggerRef,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  triggerRef: RefObject<HTMLButtonElement | null>;
}) {
  const { t } = useI18n();
  const [rating, setRating] = useState(0);
  const titleRef = useRef<HTMLHeadingElement>(null);
  const radioName = useId();
  const messages = t.sidebar.rating;
  const isFiveStars = rating === 5;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-sm"
        aria-describedby={undefined}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          titleRef.current?.focus({ preventScroll: true });
        }}
        onCloseAutoFocus={(event) => {
          setRating(0);
          event.preventDefault();
          triggerRef.current?.focus();
        }}
      >
        <DialogHeader>
          <DialogTitle ref={titleRef} tabIndex={-1} className="outline-none">
            {messages.title}
          </DialogTitle>
        </DialogHeader>
        <div className="space-y-1">
          <div role="radiogroup" aria-label={messages.title} className="flex justify-center gap-1">
            {[1, 2, 3, 4, 5].map((star) => (
              <div key={star} className="relative flex h-11 w-11 min-w-0 items-center justify-center sm:size-12">
                <div className="absolute inset-0 flex">
                  {[star - 0.5, star].map((value) => (
                    <label key={value} className="relative flex-1 cursor-pointer">
                      <input
                        type="radio"
                        name={radioName}
                        value={value}
                        checked={rating === value}
                        onChange={() => setRating(value)}
                        aria-label={messages.starLabel.replace("{count}", String(value))}
                        className="peer sr-only"
                      />
                      <span className="absolute inset-0 rounded-md peer-focus-visible:ring-2 peer-focus-visible:ring-ring" />
                    </label>
                  ))}
                </div>
                <span aria-hidden="true" className="pointer-events-none relative size-6">
                  <Star className={cn("size-6", star <= rating ? "fill-current text-tone-amber" : "text-muted-foreground")} />
                  {rating === star - 0.5 && (
                    <Star className="absolute inset-0 size-6 fill-current text-tone-amber [clip-path:inset(0_50%_0_0)]" />
                  )}
                </span>
              </div>
            ))}
          </div>
          <p aria-hidden="true" className="min-h-5 text-center text-sm text-muted-foreground tabular-nums">
            {rating > 0 ? `${rating} / 5` : null}
          </p>
        </div>
        <DialogFooter className="min-h-9">
          {rating > 0 && (
            <Button asChild>
              <a href={isFiveStars ? repositoryUrl : `${repositoryUrl}/issues`} target="_blank" rel="noopener noreferrer">
                {isFiveStars ? messages.githubStar : messages.feedback}
                <ExternalLink aria-hidden="true" />
              </a>
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
