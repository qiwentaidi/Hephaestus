'use client'

import { type CSSProperties } from 'react'
import { Button } from '@/components/ui/button'
import { CircleAlert, Sparkles, UserRound } from 'lucide-react'
import { cn } from '@/lib/utils'
import { themeToCssVars, type CustomThemeDefinition } from '@/lib/theme/custom-theme'
import { ThemeStyleOverview } from './ThemeStyleOverview'

interface ThemePreviewProps {
  definition: CustomThemeDefinition
  variant?: 'full' | 'compact'
  view?: 'page' | 'styles'
  className?: string
}

/**
 * Scoped miniature of the app. All colors resolve from inline CSS variables,
 * so it matches the real UI without touching global styles. The full variant
 * contains real controls so hover/focus/destructive states can be inspected.
 */
export function ThemePreview({
  definition,
  variant = 'full',
  view = 'page',
  className,
}: ThemePreviewProps) {
  const style = themeToCssVars(definition) as CSSProperties

  if (variant === 'compact') {
    return (
      <div
        aria-hidden="true"
        data-theme-preview="compact"
        style={style}
        className={cn(
          'flex h-20 gap-2 overflow-hidden rounded-lg border border-[var(--border)] bg-[var(--background)] p-2 text-[var(--foreground)]',
          className,
        )}
      >
        <div className="flex w-1/4 flex-col gap-1.5 rounded-md bg-[var(--sidebar)] p-1.5">
          <div className="h-2 w-4 rounded-sm bg-[var(--primary)]" />
          <div className="h-1.5 w-full rounded-sm bg-[var(--sidebar-accent)]" />
          <div className="h-1.5 w-3/4 rounded-sm bg-[var(--sidebar-accent)]" />
          <div className="mt-auto h-1.5 w-1/2 rounded-sm bg-[var(--sidebar-hover)]" />
        </div>
        <div className="flex min-w-0 flex-1 flex-col gap-1.5">
          <div className="flex items-center gap-1">
            <div className="h-2 w-10 rounded-sm bg-[var(--foreground)]" />
            <div className="ml-auto h-2.5 w-6 rounded-sm bg-[var(--primary)]" />
          </div>
          <div className="grid grid-cols-3 gap-1">
            <div className="h-4 rounded-sm border border-[var(--border)] bg-[var(--card)]" />
            <div className="h-4 rounded-sm border border-[var(--border)] bg-[var(--card)]" />
            <div className="h-4 rounded-sm border border-[var(--border)] bg-[var(--card)]" />
          </div>
          <div className="flex gap-1">
            <div className="h-1.5 w-1/2 rounded-sm bg-[var(--muted)]" />
            <div className="h-1.5 w-1/4 rounded-sm bg-[var(--tone-green-soft)]" />
            <div className="h-1.5 w-1/4 rounded-sm bg-[var(--destructive)]" />
          </div>
        </div>
      </div>
    )
  }

  if (view === 'styles') {
    return (
      <div
        role="group"
        aria-label="Theme style overview"
        data-theme-preview="styles"
        style={style}
        className={cn(
          'min-h-[430px] overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--background)] text-[var(--foreground)] shadow-sm',
          className,
        )}
      >
        <div className="flex items-center justify-between border-b border-[var(--border)] bg-[var(--card)] px-3 py-2">
          <div className="text-[10px] font-semibold text-[var(--card-foreground)]">Style overview</div>
          <div className="text-[10px] text-[var(--muted-foreground)]">Live tokens</div>
        </div>
        <div className="p-3">
          <ThemeStyleOverview definition={definition} mode="tokens" />
        </div>
      </div>
    )
  }

  const showcaseStyle = {
    ...style,
    backgroundImage:
      'radial-gradient(circle at 8% 0%, color-mix(in srgb, var(--primary) 18%, transparent), transparent 33%), radial-gradient(circle at 95% 85%, color-mix(in srgb, var(--tone-blue) 13%, transparent), transparent 30%)',
  } as CSSProperties

  return (
    <div
      role="group"
      aria-label="Theme preview"
      data-theme-preview="full"
      style={showcaseStyle}
      className={cn(
        'shrink-0 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--background)] text-[var(--foreground)] shadow-sm',
        className,
      )}
    >
      <div className="flex items-center gap-1.5 border-b border-[var(--border)] bg-[var(--card)]/80 px-3 py-2 backdrop-blur-sm">
        <span className="h-2 w-2 rounded-full bg-[var(--tone-red)]" />
        <span className="h-2 w-2 rounded-full bg-[var(--tone-amber)]" />
        <span className="h-2 w-2 rounded-full bg-[var(--tone-green)]" />
        <span className="ml-2 text-[10px] font-medium text-[var(--muted-foreground)]">Theme showcase</span>
        <span className="ml-auto rounded-md border border-[var(--border)] bg-[var(--background)] px-2 py-0.5 text-[10px] text-[var(--muted-foreground)]">
          Live preview
        </span>
      </div>

      <div className="p-3 sm:p-5">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div className="inline-flex items-center rounded-full border border-[var(--border)] bg-[var(--card)]/80 p-0.5 text-[10px]">
            <span className="rounded-full bg-[var(--primary)] px-3 py-1 font-medium text-[var(--primary-foreground)]">Components</span>
            <span className="px-3 py-1 text-[var(--muted-foreground)]">Dashboard</span>
          </div>
          <div className="flex items-center gap-1.5">
            {[definition.colors.primary, definition.colors.success, definition.colors.warning, definition.colors.info].map((color) => (
              <span key={color} className="h-4 w-4 rounded-full border border-[var(--card)] shadow-sm" style={{ backgroundColor: color }} />
            ))}
            <span className="ml-1 text-[10px] text-[var(--muted-foreground)]">Live tokens</span>
          </div>
        </div>

        <div className="rounded-2xl border border-[var(--border)] bg-[var(--card)]/65 p-3 shadow-sm backdrop-blur-sm sm:p-5">
          <div className="mb-4 flex flex-wrap items-end justify-between gap-3">
            <div>
              <div className="text-base font-semibold sm:text-lg">Component gallery</div>
              <div className="mt-1 text-[10px] text-[var(--muted-foreground)] sm:text-xs">A dense preview of surfaces, controls, feedback and content hierarchy.</div>
            </div>
            <div className="flex items-center gap-1.5">
              <Button size="sm" variant="outline" className="h-7 px-2 text-[10px]">Reset</Button>
              <Button size="sm" className="h-7 px-2 text-[10px]"><Sparkles className="mr-1 h-3 w-3" />Try theme</Button>
            </div>
          </div>

          <div className="mb-3 grid gap-3 xl:grid-cols-[1.25fr_.85fr]">
            <section className="rounded-xl border border-[var(--border)] bg-[var(--background)]/80 p-3">
              <div className="mb-3 flex items-center justify-between">
                <div className="text-xs font-semibold">Build a scan</div>
                <span className="rounded-full bg-[var(--tone-green-soft)] px-2 py-0.5 text-[10px] text-[var(--tone-green)]">Ready</span>
              </div>
              <div className="grid gap-2 sm:grid-cols-2">
                <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] px-2.5 py-2 text-[10px] text-[var(--muted-foreground)]">Target URL <span className="ml-1 text-[var(--foreground)]">api.example.com</span></div>
                <div className="rounded-[var(--radius)] border border-[var(--border)] bg-[var(--card)] px-2.5 py-2 text-[10px] text-[var(--muted-foreground)]">Scan profile <span className="ml-1 text-[var(--foreground)]">Balanced</span></div>
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <Button size="sm" className="h-7 px-2 text-[10px]">Start scan</Button>
                <Button size="sm" variant="outline" className="h-7 px-2 text-[10px]">Schedule</Button>
                <span className="ml-auto text-[10px] text-[var(--muted-foreground)]">2 targets · 18 checks</span>
              </div>
              <div className="mt-4 space-y-2">
                <div className="flex items-center justify-between text-[10px] text-[var(--muted-foreground)]"><span>Discovery</span><span>72%</span></div>
                <div className="h-1.5 overflow-hidden rounded-full bg-[var(--muted)]"><div className="h-full w-[72%] rounded-full bg-[var(--primary)]" /></div>
                <div className="flex items-center justify-between text-[10px] text-[var(--muted-foreground)]"><span>Verification</span><span>38%</span></div>
                <div className="h-1.5 overflow-hidden rounded-full bg-[var(--muted)]"><div className="h-full w-[38%] rounded-full bg-[var(--tone-orange)]" /></div>
              </div>
            </section>

            <section className="rounded-xl border border-[var(--border)] bg-[var(--background)]/80 p-3">
              <div className="mb-3 flex items-center gap-2">
                <div className="flex h-8 w-8 items-center justify-center rounded-full bg-[var(--primary)] text-[var(--primary-foreground)]"><UserRound className="h-4 w-4" /></div>
                <div><div className="text-xs font-semibold">Theme profile</div><div className="text-[10px] text-[var(--muted-foreground)]">Applied to every surface</div></div>
              </div>
              <div className="rounded-lg border border-[var(--border)] bg-[var(--card)] p-2.5">
                <div className="text-[10px] text-[var(--muted-foreground)]">Contrast score</div>
                <div className="mt-1 flex items-end justify-between"><span className="text-xl font-semibold">AA</span><span className="text-[10px] text-[var(--tone-green)]">Accessible</span></div>
                <div className="mt-2 flex gap-1.5">{['var(--primary)', 'var(--success)', 'var(--warning)', 'var(--destructive)'].map((token) => <span key={token} className="h-2 flex-1 rounded-full" style={{ backgroundColor: token }} />)}</div>
              </div>
              <div className="mt-2 flex items-center gap-2 rounded-lg border border-[var(--tone-blue-border)] bg-[var(--tone-blue-soft)] px-2.5 py-2 text-[10px] text-[var(--tone-blue)]"><CircleAlert className="h-3.5 w-3.5" />All tokens update in real time.</div>
            </section>
          </div>

          <ThemeStyleOverview definition={definition} />
        </div>
      </div>
    </div>
  )
}
