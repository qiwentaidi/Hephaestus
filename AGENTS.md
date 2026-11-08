# AGENTS.md

## Backend

### Scan task pause

- Persist the pause intent, cancel every active task and subtask context, and mark the parent `paused` only after all active modules acknowledge exit. If the wait expires before exit is confirmed, leave the parent in a diagnosable non-terminal state.
- At startup, finalize local and combination tasks with `pause_requested = true` as `paused` before interrupted-task recovery. Exclude them from automatic re-enqueue.
- Regression tests must cover the pause request, context cancellation, worker exit acknowledgement, final parent state, and service-restart recovery.

### AI Workbench

- For standalone pentest dialogue, follow [the AI Workbench design](docs/ai-workbench-design.md) and use the local ARTEX project as the behavior reference. Adapt tools, APIs, and storage to Hephaestus while preserving its authorization boundaries.
- Keep standalone dialogue free of legacy scan ordering, native-tool priority, automatic scan dispatch, API coverage queues, role templates, and keyword- or Todo-based completion gates.
- Intelligent orchestration follows [the ARTEX task engine migration design](docs/artex-task-engine-migration.md).

## Frontend

### Design approval and visual verification

- For UI, layout, theme, or interaction changes, present a visual proposal first and wait for explicit user approval before editing.
- Use Ego Lite first for screenshot, visual state, and browser flow verification. If the target page is unauthenticated in Ego Lite, hand off the task space to the user and resume after they confirm login is complete.

### Fixed table columns

- Apply `sticky right-0` to both the action header and its corresponding body cell. Layer the header above body cells and keep the fixed cell background in sync with row hover and selection. Use an opaque table surface for standard themes.
- In `DataTableWithColumns`, set the header and body styles through `headerClassName` and `className`. Keep shared table padding unchanged.
- Show `table-fixed-column-shadow-left` while a right fixed column covers scrollable content. Use `table-fixed-column-shadow-right` for left fixed columns. In raw `Table`, read horizontal overflow and scroll position from `[data-slot="table-container"]`.
- In Glass, use a blurred, translucent neutral gray or white surface that obscures the scrolled content. Keep the default and hover styles centralized in `frontend/styles/globals.css` through `--glass-table-sticky-bg` and `--glass-table-sticky-hover-bg`.

### Flame theme

- Use neutral surfaces, muted borders, amber, or Flame orange for ordinary states, including selection, active tabs and navigation, pending, progress, focus, and hover. Reserve destructive red for destructive actions, danger, and errors.
- Visually verify the affected states in Flame before finishing theme-sensitive UI changes.

### Settings

- Keep `frontend/app/(main)/settings/page.tsx` and `frontend/app/settings-window/page.tsx` in sync. Update the matching `TabsTrigger` and `TabsContent` in both shells, share tab components under `frontend/app/(main)/settings/components/`, and update `settings.tabs` in both `frontend/lib/i18n/locales/zh.ts` and `en.ts`. Verify both the in-app page and standalone desktop window.
- Keep third-party asset search keys in `ApiKeysTab.tsx`. Keep AI providers, protocols, defaults, and feature routing in `AIConfigTab.tsx`. Use `frontend/lib/api/ai-config.ts` and `backend/pkg/config/storage.go` for AI configuration. Preserve legacy single-provider compatibility alongside multi-provider storage.

### Shared conventions

- Use `fetchWithAuth` from `@/lib/api` for authenticated requests.
- Use `copyToClipboard` from `frontend/lib/clipboard.ts` for all copy actions. Keep browser capability checks and fallbacks in that module and leave user feedback with the caller.
- Use the global `react-hot-toast` provider for notifications.
- Use the shared `getStatusBadge` from `@/lib/status-badge` and `PaginationPro` for task lists.
