"use client";

import { MouseEvent, useCallback, useEffect, useRef, useState } from "react";
import { ArrowLeft, ArrowRight, Copy, Minus, PanelTop, PanelTopClose, Rocket, Settings, Square, X } from "lucide-react";
import { usePathname } from "next/navigation";
import { DesktopResizeHandles, type DesktopResizeEdge } from "@/components/DesktopResizeHandles";
import { Button } from "@/components/ui/button";
import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from "@/lib/env";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import { subscribeContentTabsEnabled, toggleContentTabsEnabled } from "@/lib/appearance";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";

type DesktopMeta = {
	isDesktop: boolean;
	platform: string;
	isMaximised: boolean;
};

async function requestDesktopWindow(path: string, method: "GET" | "POST" = "POST") {
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
		throw new Error(`desktop request failed: ${response.status}`);
	}

	return response.json().catch(() => ({}));
}

export function DesktopTitlebar() {
	const [meta, setMeta] = useState<DesktopMeta | null>(null);
	const [contentTabsEnabled, setContentTabsEnabled] = useState(false);
	const [isLoggedIn, setIsLoggedIn] = useState(false);
	const pendingWindowsDragRef = useRef(false);
	const pathname = usePathname();
	const { t } = useI18n();

	const syncWindowState = useCallback(async () => {
		try {
			const payload = await requestDesktopWindow("/api/v1/desktop/window/state", "GET");
			if (payload?.success) {
				setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
			}
		} catch {
			// Ignore transient state sync failures.
		}
	}, []);

	useEffect(() => {
		let cancelled = false;

		const loadDesktopMeta = async () => {
			try {
				const payload = await requestDesktopWindow("/api/v1/desktop/meta", "GET");
				if (cancelled || !payload?.success || !payload?.data?.isDesktop) {
					return;
				}
				setMeta({
					isDesktop: true,
					platform: String(payload.data.platform || ""),
					isMaximised: Boolean(payload.data.isMaximised),
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

	useEffect(() => {
		if (typeof window === "undefined") {
			return;
		}

		return subscribeContentTabsEnabled(setContentTabsEnabled);
	}, []);

	useEffect(() => {
		if (typeof window === "undefined") {
			return;
		}

		const syncSessionState = () => {
			setIsLoggedIn(Boolean(window.localStorage.getItem("auth_token")));
		};

		syncSessionState();
		window.addEventListener("storage", syncSessionState);
		window.addEventListener("hephaestus:desktop-storage-restored", syncSessionState);
		return () => {
			window.removeEventListener("storage", syncSessionState);
			window.removeEventListener("hephaestus:desktop-storage-restored", syncSessionState);
		};
	}, []);

	if (!meta?.isDesktop) {
		return null;
	}

	const isMac = meta.platform === "darwin";
	const windowsControlButtonClassName = "h-8 w-11 rounded-none text-muted-foreground transition-colors hover:bg-black/6 hover:text-foreground dark:hover:bg-white/8";
	const shouldShowAppActions = pathname !== "/" && isLoggedIn;
	const shouldShowContentTabsToggle = shouldShowAppActions;

	const handleMinimise = async () => {
		await requestDesktopWindow("/api/v1/desktop/window/minimise");
	};

	const handleToggleMaximise = async () => {
		const payload = await requestDesktopWindow("/api/v1/desktop/window/toggle-maximise");
		setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
	};

	const handleClose = async () => {
		await requestDesktopWindow("/api/v1/desktop/window/close");
	};

	const handleStartResize = (edge: DesktopResizeEdge) => {
		void requestDesktopWindow(`/api/v1/desktop/window/start-resize?edge=${edge}`).catch(() => undefined);
	};

	const handleOpenSettings = async () => {
		await requestDesktopWindow("/api/v1/desktop/settings/open");
	};

	const handleOpenLauncher = async () => {
		await requestDesktopWindow("/api/v1/desktop/launcher/open");
	};

	const handleTitlebarDoubleClick = async (event: MouseEvent<HTMLDivElement>) => {
		const target = event.target as HTMLElement | null;
		if (target?.closest("button, a, input, select, textarea, [data-no-titlebar-toggle='true']")) {
			return;
		}

		const payload = await requestDesktopWindow("/api/v1/desktop/window/titlebar-double-click");
		setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
	};

	const shouldIgnoreTitlebarPointer = (target: EventTarget | null) => {
		return target instanceof HTMLElement && Boolean(
			target.closest("button, a, input, select, textarea, [data-no-titlebar-toggle='true'], [data-no-titlebar-drag='true']")
		);
	};

	const handleTitlebarMouseDown = (event: MouseEvent<HTMLDivElement>) => {
		if (meta.platform !== "windows" || event.button !== 0 || shouldIgnoreTitlebarPointer(event.target)) {
			pendingWindowsDragRef.current = false;
			return;
		}
		pendingWindowsDragRef.current = true;
	};

	const handleTitlebarMouseMove = (event: MouseEvent<HTMLDivElement>) => {
		if (meta.platform !== "windows" || !pendingWindowsDragRef.current || (event.buttons & 1) !== 1) {
			return;
		}
		pendingWindowsDragRef.current = false;
		void requestDesktopWindow("/api/v1/desktop/window/start-drag");
	};

	const clearPendingTitlebarDrag = () => {
		pendingWindowsDragRef.current = false;
	};

	return (
		<>
			<DesktopResizeHandles enabled={meta.platform === "windows"} onResizeStart={handleStartResize} />
			<div
			className={`flex h-9 select-none items-center border-b border-border/70 bg-card/96 backdrop-blur-xl ${isMac ? "pl-[76px] pr-2.5" : "pl-2.5 pr-0"}`}
			style={{
				["--wails-draggable" as string]: "drag",
				["-webkit-app-region" as string]: "drag",
			}}
			onDoubleClick={handleTitlebarDoubleClick}
			onMouseDown={handleTitlebarMouseDown}
			onMouseMove={handleTitlebarMouseMove}
			onMouseLeave={clearPendingTitlebarDrag}
			onMouseUp={clearPendingTitlebarDrag}
			onMouseEnter={() => {
				void syncWindowState();
			}}
		>
			<TooltipProvider delayDuration={200}>
				<div
					className={isMac ? "flex items-center gap-0.5 pr-2" : "flex items-center gap-0.5 pr-2"}
					data-no-titlebar-drag="true"
					style={{
						["--wails-draggable" as string]: "no-drag",
						["-webkit-app-region" as string]: "no-drag",
					}}
				>
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="h-7 w-7 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
								onClick={() => {
									window.history.back();
								}}
							>
								<ArrowLeft className="h-3.5 w-3.5" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>{t.desktopTitlebar.back}</TooltipContent>
					</Tooltip>

					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon"
								className="h-7 w-7 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
								onClick={() => {
									window.history.forward();
								}}
							>
								<ArrowRight className="h-3.5 w-3.5" />
							</Button>
						</TooltipTrigger>
						<TooltipContent>{t.desktopTitlebar.forward}</TooltipContent>
					</Tooltip>
				</div>
				<div
					className="flex h-full flex-1"
					style={{
						["--wails-draggable" as string]: "drag",
						["-webkit-app-region" as string]: "drag",
					}}
				/>
				<div
					className="flex items-center gap-0.5"
					data-no-titlebar-drag="true"
					style={{
						["--wails-draggable" as string]: "no-drag",
						["-webkit-app-region" as string]: "no-drag",
					}}
				>
					{shouldShowAppActions && (
						<>
							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										variant="ghost"
										size="icon"
										className="h-7 w-7 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
										onClick={() => {
											void handleOpenLauncher();
										}}
									>
										<Rocket className="h-3.5 w-3.5" />
									</Button>
								</TooltipTrigger>
								<TooltipContent>{t.desktopTitlebar.launcher}</TooltipContent>
							</Tooltip>

							<Tooltip>
								<TooltipTrigger asChild>
									<Button
										variant="ghost"
										size="icon"
										className="h-7 w-7 rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
										onClick={() => {
											void handleOpenSettings();
										}}
									>
										<Settings className="h-3.5 w-3.5" />
									</Button>
								</TooltipTrigger>
								<TooltipContent>{t.sidebar.settings}</TooltipContent>
							</Tooltip>
						</>
					)}

					{shouldShowContentTabsToggle && (
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									size="icon"
									className={cn(
										"h-7 w-7 rounded-md text-muted-foreground transition-all hover:bg-muted hover:text-foreground",
										contentTabsEnabled && "bg-muted text-foreground"
									)}
									onClick={() => {
										setContentTabsEnabled(toggleContentTabsEnabled());
									}}
								>
									{contentTabsEnabled ? (
										<PanelTop className="h-3.5 w-3.5" />
									) : (
										<PanelTopClose className="h-3.5 w-3.5" />
									)}
								</Button>
							</TooltipTrigger>
							<TooltipContent>{t.desktopTitlebar.toggleContentTabs}</TooltipContent>
						</Tooltip>
					)}

					{!isMac && (
						<>
							<Button
								variant="ghost"
								size="icon"
								className={windowsControlButtonClassName}
								onClick={() => {
									void handleMinimise();
								}}
							>
								<Minus className="h-3.5 w-3.5" />
							</Button>
							<Button
								variant="ghost"
								size="icon"
								className={windowsControlButtonClassName}
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
								onClick={() => {
									void handleClose();
								}}
							>
								<X className="h-3.5 w-3.5" />
							</Button>
						</>
					)}
				</div>
			</TooltipProvider>
			</div>
		</>
	);
}
