"use client";

import { MouseEvent, useCallback, useEffect, useRef, useState } from "react";
import { Copy, Minus, Square, X } from "lucide-react";
import { DesktopResizeHandles, type DesktopResizeEdge } from "@/components/DesktopResizeHandles";
import { Button } from "@/components/ui/button";
import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from "@/lib/env";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

type DesktopMeta = {
	isDesktop: boolean;
	platform: string;
	isMaximised: boolean;
};

async function requestSettingsWindow(path: string, method: "GET" | "POST" = "POST") {
	if (!hasDesktopApiToken()) {
		return { success: false };
	}
	const response = await fetch(`${getApiBaseURL()}${path}`, {
		method,
		cache: "no-store",
		headers: {
			"Content-Type": "application/json",
			...getDesktopApiHeaders(),
		},
	});

	if (!response.ok) {
		throw new Error(`desktop settings window request failed: ${response.status}`);
	}

	return response.json().catch(() => ({}));
}

export function SettingsWindowTitlebar() {
	const [meta, setMeta] = useState<DesktopMeta | null>(null);
	const pendingDragRef = useRef(false);
	const { t } = useI18n();

	useEffect(() => {
		let cancelled = false;

		const loadDesktopMeta = async () => {
			try {
				const payload = await requestSettingsWindow("/api/v1/desktop/meta", "GET");
				if (cancelled || !payload?.success || !payload?.data?.isDesktop) {
					return;
				}
				setMeta({
					isDesktop: true,
					platform: String(payload.data.platform || ""),
					isMaximised: false,
				});
			} catch {
				if (!cancelled) {
					setMeta(null);
				}
			}
		};

		void loadDesktopMeta();
		return () => {
			cancelled = true;
		};
	}, []);

	const syncWindowState = useCallback(async () => {
		try {
			const payload = await requestSettingsWindow("/api/v1/desktop/settings/window/state", "GET");
			if (payload?.success) {
				setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
			}
		} catch {
			// Ignore transient state sync failures.
		}
	}, []);

	useEffect(() => {
		if (!meta?.isDesktop) {
			return;
		}

		let syncTimeout: ReturnType<typeof setTimeout> | null = null;
		const scheduleSync = () => {
			if (syncTimeout) {
				clearTimeout(syncTimeout);
			}
			syncTimeout = setTimeout(() => {
				void syncWindowState();
			}, 120);
		};

		window.addEventListener("resize", scheduleSync);
		window.addEventListener("focus", scheduleSync);
		document.addEventListener("visibilitychange", scheduleSync);

		return () => {
			if (syncTimeout) {
				clearTimeout(syncTimeout);
			}
			window.removeEventListener("resize", scheduleSync);
			window.removeEventListener("focus", scheduleSync);
			document.removeEventListener("visibilitychange", scheduleSync);
		};
	}, [meta?.isDesktop, syncWindowState]);

	if (!meta?.isDesktop) {
		return null;
	}

	const isMac = meta.platform === "darwin";
	const controlButtonClassName = "h-8 w-11 rounded-none text-muted-foreground transition-colors hover:bg-black/6 hover:text-foreground dark:hover:bg-white/8";

	const handleMinimise = async () => {
		await requestSettingsWindow("/api/v1/desktop/settings/window/minimise");
	};

	const handleToggleMaximise = async () => {
		const payload = await requestSettingsWindow("/api/v1/desktop/settings/window/toggle-maximise");
		setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
	};

	const handleClose = async () => {
		await requestSettingsWindow("/api/v1/desktop/settings/window/close");
	};

	const handleStartResize = (edge: DesktopResizeEdge) => {
		void requestSettingsWindow(`/api/v1/desktop/settings/window/start-resize?edge=${edge}`).catch(() => undefined);
	};

	const handleTitlebarDoubleClick = async (event: MouseEvent<HTMLDivElement>) => {
		const target = event.target as HTMLElement | null;
		if (target?.closest("button, a, input, select, textarea, [data-no-titlebar-toggle='true']")) {
			return;
		}

		const payload = await requestSettingsWindow("/api/v1/desktop/settings/window/titlebar-double-click");
		setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
	};

	const shouldIgnoreTitlebarPointer = (target: EventTarget | null) => {
		return target instanceof HTMLElement && Boolean(
			target.closest("button, a, input, select, textarea, [data-no-titlebar-toggle='true'], [data-no-titlebar-drag='true']")
		);
	};

	const handleTitlebarMouseDown = (event: MouseEvent<HTMLDivElement>) => {
		if (isMac || event.button !== 0 || shouldIgnoreTitlebarPointer(event.target)) {
			pendingDragRef.current = false;
			return;
		}
		pendingDragRef.current = true;
	};

	const handleTitlebarMouseMove = (event: MouseEvent<HTMLDivElement>) => {
		if (isMac || !pendingDragRef.current || (event.buttons & 1) !== 1) {
			return;
		}
		pendingDragRef.current = false;
		void requestSettingsWindow("/api/v1/desktop/settings/window/start-drag");
	};

	const clearPendingDrag = () => {
		pendingDragRef.current = false;
	};

	return (
		<>
			<DesktopResizeHandles enabled={meta.platform === "windows"} onResizeStart={handleStartResize} />
			<div
			className={cn(
				"relative flex h-9 shrink-0 select-none items-center border-b border-border/70 bg-card/96 backdrop-blur-xl",
				isMac ? "pl-[76px] pr-2.5" : "pl-2.5 pr-0"
			)}
			style={{
				["--wails-draggable" as string]: "drag",
				["-webkit-app-region" as string]: "drag",
			}}
			onDoubleClick={handleTitlebarDoubleClick}
			onMouseDown={handleTitlebarMouseDown}
			onMouseMove={handleTitlebarMouseMove}
			onMouseLeave={clearPendingDrag}
			onMouseUp={clearPendingDrag}
			onMouseEnter={() => {
				void syncWindowState();
			}}
		>
			<div className="pointer-events-none absolute inset-x-24 top-1/2 -translate-y-1/2 truncate text-center text-sm font-semibold text-foreground">
				{t.settings.title}
			</div>
			<div className="min-w-0 flex-1" />
			{!isMac && (
				<div
					className="flex items-center"
					data-no-titlebar-drag="true"
					style={{
						["--wails-draggable" as string]: "no-drag",
						["-webkit-app-region" as string]: "no-drag",
					}}
				>
					<Button
						variant="ghost"
						size="icon"
						className={controlButtonClassName}
						aria-label="Minimise settings window"
						onClick={() => {
							void handleMinimise();
						}}
					>
						<Minus className="h-3.5 w-3.5" />
					</Button>
					<Button
						variant="ghost"
						size="icon"
						className={controlButtonClassName}
						aria-label="Maximise settings window"
						onClick={() => {
							void handleToggleMaximise();
						}}
					>
						{meta.isMaximised ? (
							<Copy className="h-3.5 w-3.5" />
						) : (
							<Square className="h-3.5 w-3.5" />
						)}
					</Button>
					<Button
						variant="ghost"
						size="icon"
						className="h-8 w-11 rounded-none text-muted-foreground transition-colors hover:bg-[#e81123] hover:text-white dark:hover:bg-[#e81123]"
						aria-label="Close settings window"
						onClick={() => {
							void handleClose();
						}}
					>
						<X className="h-3.5 w-3.5" />
					</Button>
				</div>
			)}
			</div>
		</>
	);
}
