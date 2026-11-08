"use client";

import { MouseEvent, useCallback, useEffect, useRef, useState } from "react";
import { Coffee, Copy, Minus, Plus, Square, Trash2, X } from "lucide-react";
import { DesktopResizeHandles, type DesktopResizeEdge } from "@/components/DesktopResizeHandles";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from "@/lib/env";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n";
import toast from "@/lib/toast";
import {
	getLauncherConfig,
	saveLauncherJDKs,
	selectLauncherPath,
	type LauncherJDK,
} from "@/lib/api/launcher";

type DesktopMeta = {
	isDesktop: boolean;
	platform: string;
	isMaximised: boolean;
};

async function requestLauncherWindow(path: string, method: "GET" | "POST" = "POST") {
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
		throw new Error(`desktop launcher window request failed: ${response.status}`);
	}

	return response.json().catch(() => ({}));
}

export function LauncherWindowTitlebar() {
	const [meta, setMeta] = useState<DesktopMeta | null>(null);
	const [jdkSettingsOpen, setJdkSettingsOpen] = useState(false);
	const [jdks, setJdks] = useState<LauncherJDK[]>([]);
	const [jdkForm, setJdkForm] = useState<LauncherJDK>({ name: "", path: "" });
	const pendingDragRef = useRef(false);
	const { t } = useI18n();
	const i18n = t.launcher;

	const fetchJDKs = useCallback(async () => {
		const payload = await getLauncherConfig();
		return payload.jdks;
	}, []);

	const loadJDKs = useCallback(async () => {
		setJdks(await fetchJDKs());
	}, [fetchJDKs]);

	useEffect(() => {
		let cancelled = false;

		const loadDesktopMeta = async () => {
			try {
				const payload = await requestLauncherWindow("/api/v1/desktop/meta", "GET");
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

	useEffect(() => {
		let cancelled = false;

		const loadInitialJDKs = async () => {
			try {
				const nextJDKs = await fetchJDKs();
				if (!cancelled) {
					setJdks(nextJDKs);
				}
			} catch {
				// Ignore launcher config load failures; the settings dialog can retry.
			}
		};

		const handleLauncherUpdated = async () => {
			try {
				const nextJDKs = await fetchJDKs();
				if (!cancelled) {
					setJdks(nextJDKs);
				}
			} catch {
				// Ignore transient launcher update refresh failures.
			}
		};

		void loadInitialJDKs();
		window.addEventListener("hephaestus:launcher-updated", handleLauncherUpdated);
		return () => {
			cancelled = true;
			window.removeEventListener("hephaestus:launcher-updated", handleLauncherUpdated);
		};
	}, [fetchJDKs]);

	const syncWindowState = useCallback(async () => {
		try {
			const payload = await requestLauncherWindow("/api/v1/desktop/launcher/window/state", "GET");
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
	const titlebarActionButtonClassName = "h-7 rounded-md px-2.5 text-xs text-muted-foreground hover:bg-muted hover:text-foreground";

	const emitLauncherUpdated = () => {
		window.dispatchEvent(new CustomEvent("hephaestus:launcher-updated"));
	};

	const handleMinimise = async () => {
		await requestLauncherWindow("/api/v1/desktop/launcher/window/minimise");
	};

	const handleToggleMaximise = async () => {
		const payload = await requestLauncherWindow("/api/v1/desktop/launcher/window/toggle-maximise");
		setMeta((prev) => prev ? { ...prev, isMaximised: Boolean(payload?.data?.isMaximised) } : prev);
	};

	const handleClose = async () => {
		await requestLauncherWindow("/api/v1/desktop/launcher/window/close");
	};

	const handleStartResize = (edge: DesktopResizeEdge) => {
		void requestLauncherWindow(`/api/v1/desktop/launcher/window/start-resize?edge=${edge}`).catch(() => undefined);
	};

	const persistJDKs = async (nextJdks: LauncherJDK[]) => {
		setJdks(nextJdks);
		await saveLauncherJDKs(nextJdks);
		emitLauncherUpdated();
	};

	const resetJDKForm = () => {
		setJdkForm({ name: "", path: "" });
	};

	const saveJDK = async () => {
		const name = jdkForm.name.trim();
		const path = jdkForm.path.trim();
		if (!name || !path) {
			toast.error(i18n.messages.jdkRequired);
			return;
		}
		if (jdks.some((item) => item.name === name)) {
			toast.error(i18n.messages.jdkExists);
			return;
		}
		try {
			await persistJDKs([...jdks, { name, path }]);
			resetJDKForm();
			toast.success(i18n.messages.jdkAdded);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.saveJDKFailed);
		}
	};

	const deleteJDK = async (name: string) => {
		try {
			await persistJDKs(jdks.filter((item) => item.name !== name));
			toast.success(i18n.messages.jdkDeleted);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.deleteJDKFailed);
		}
	};

	const handleTitlebarDoubleClick = async (event: MouseEvent<HTMLDivElement>) => {
		const target = event.target as HTMLElement | null;
		if (target?.closest("button, a, input, select, textarea, [data-no-titlebar-toggle='true']")) {
			return;
		}

		const payload = await requestLauncherWindow("/api/v1/desktop/launcher/window/titlebar-double-click");
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
		void requestLauncherWindow("/api/v1/desktop/launcher/window/start-drag");
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
				{t.desktopTitlebar.launcher}
			</div>
			<div
				className="flex items-center gap-1"
				data-no-titlebar-drag="true"
				style={{
					["--wails-draggable" as string]: "no-drag",
					["-webkit-app-region" as string]: "no-drag",
				}}
			>
				<Button
					variant="ghost"
					className={titlebarActionButtonClassName}
					onClick={() => {
						setJdkSettingsOpen(true);
						void loadJDKs().catch(() => undefined);
					}}
				>
					<Coffee className="mr-1.5 h-3.5 w-3.5" />
					JDK
				</Button>
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
						aria-label="Minimise launcher window"
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
						aria-label="Maximise launcher window"
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
						aria-label="Close launcher window"
						onClick={() => {
							void handleClose();
						}}
					>
						<X className="h-3.5 w-3.5" />
					</Button>
				</div>
			)}
			<Dialog
				open={jdkSettingsOpen}
				onOpenChange={(open) => {
					setJdkSettingsOpen(open);
					if (!open) {
						resetJDKForm();
					}
				}}
			>
				<DialogContent
					className="sm:max-w-[720px]"
					data-no-titlebar-drag="true"
					style={{
						["--wails-draggable" as string]: "no-drag",
						["-webkit-app-region" as string]: "no-drag",
					}}
				>
					<DialogHeader>
						<DialogTitle>{i18n.dialogs.jdkConfigTitle}</DialogTitle>
					</DialogHeader>
					<div className="space-y-4 py-1">
						<div className="grid gap-3 rounded-md border border-border/60 bg-background/40 p-3">
							<div className="grid gap-2">
								<Label>{i18n.labels.name}</Label>
								<Input
									value={jdkForm.name}
									onChange={(event) => setJdkForm((prev) => ({ ...prev, name: event.target.value }))}
									placeholder={i18n.placeholders.jdkName}
								/>
							</div>
							<div className="grid gap-2">
								<Label>{i18n.labels.path}</Label>
								<div className="flex gap-2">
									<Input
										value={jdkForm.path}
										onChange={(event) => setJdkForm((prev) => ({ ...prev, path: event.target.value }))}
										placeholder={i18n.placeholders.jdkPath}
									/>
									<Button
										type="button"
										variant="secondary"
										onClick={() => void selectLauncherPath().then((path) => {
											if (path) {
												setJdkForm((prev) => ({ ...prev, path }));
											}
										}).catch((error: any) => toast.error(error?.message || i18n.messages.selectPathFailed))}
									>
										{i18n.actions.choose}
									</Button>
								</div>
							</div>
							<div className="flex justify-end">
								<Button onClick={() => void saveJDK()}>
									<Plus className="mr-2 h-4 w-4" />
									{i18n.actions.addJDK}
								</Button>
							</div>
						</div>

						<div className="space-y-2">
							{jdks.length ? jdks.map((jdk) => (
								<div key={jdk.name} className="flex items-center justify-between gap-3 rounded-md border border-border/60 bg-background/50 px-3 py-2.5">
									<div className="min-w-0">
										<p className="truncate text-sm font-medium">{jdk.name}</p>
										<p className="truncate text-xs text-muted-foreground">{jdk.path}</p>
									</div>
									<Button variant="ghost" size="icon" onClick={() => void deleteJDK(jdk.name)}>
										<Trash2 className="h-4 w-4" />
									</Button>
								</div>
							)) : (
								<div className="rounded-md border border-dashed border-border/60 bg-background/40 p-5 text-sm text-muted-foreground">
									{i18n.empty.noJDK}
								</div>
							)}
						</div>
					</div>
					<DialogFooter>
						<Button variant="secondary" onClick={() => setJdkSettingsOpen(false)}>{t.common.close}</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
			</div>
		</>
	);
}
