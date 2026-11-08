"use client";

import { useState, useSyncExternalStore, useEffect, useMemo, useRef, useCallback, type WheelEvent } from "react";
import Image from "next/image";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { CaretSortIcon } from "@radix-ui/react-icons";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "@/components/ui/tooltip";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "@/components/ui/popover";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
	LayoutDashboard,
	FolderSearch,
	Settings,
	LogOut,

	ChevronLeft,
	ChevronRight,
	Menu,
	Radar,
	Repeat,
	FileCode2,
	MoreHorizontal,
	Pin,
	PinOff,
	Server,
	ServerCog,
	ScrollText,
	Building2,
	Binary,
	Braces,
	FileSearch,
	ShieldCheck,
	Diff,
	Database,
	PackageOpen,
	HelpCircle,
	KeyRound,
	Crosshair,
	Boxes,
	X,

	RefreshCw,
	BrainCircuit,
	Bug,
	Star,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { AliveScope } from "react-activation";
import { clearToken, fetchWithAuth, getToken } from "@/lib/api";
import toast from "@/lib/toast";
import { AboutDialog } from "@/app/(main)/settings/components/AboutDialog";
import { ChangePasswordDialog } from "@/app/(main)/settings/components/ChangePasswordDialog";
import { UpdateDialog } from "@/components/UpdateDialog";
import { DesktopTitlebar } from "@/components/DesktopTitlebar";
import { useI18n } from "@/lib/i18n";
import { isDesktopRuntime } from "@/lib/desktop";
import { persistStagedUpdateVersion, readStagedUpdateVersion } from "@/lib/system-update";
import { subscribeContentTabsEnabled, subscribeSidebarVariant, type SidebarVariant } from "@/lib/appearance";
import { emptySidebarPinsSnapshot, parseSidebarPins, readSidebarPinsSnapshot, subscribeSidebarPins, toggleSidebarPin } from "@/lib/sidebar-pins";
import { getLayoutRoutePath, mergeLayoutTab, type LayoutTab } from "@/lib/layout-tabs";
import { ProductRatingDialog } from "./ProductRatingDialog";

type NavChild = { title: string; href: string; icon?: typeof LayoutDashboard };
type NavItem = {
	title: string;
	icon: typeof LayoutDashboard;
	presentation?: "popover";
	placement?: "footer";
	pinnable?: boolean;
} & ({ href: string; children?: never } | { children: NavChild[]; href?: never });

const getNavItems = (t: ReturnType<typeof useI18n>["t"], desktopMode: boolean): NavItem[] => [
	{
		title: t.sidebar.dashboard,
		icon: LayoutDashboard,
		href: "/dashboard",
	},
	{
		title: t.sidebar.aiWorkbench,
		icon: BrainCircuit,
		children: [
			{ title: t.sidebar.aiConversation, href: "/ai/workbench" },
			{ title: t.aiBatch.title, href: "/ai/tasks" },
			{ title: t.agentPrompts.title, href: "/ai/agents" },
			{ title: t.sidebar.toolGuard, href: "/ai/tool-guard" },
		],
	},
	{
		title: t.sidebar.vulnScan,
		icon: Crosshair,
		href: "/scan/vuln-scan",
	},
	{
		title: t.sidebar.assetManagement,
		icon: Boxes,
		children: [
			{ title: t.sidebar.assetCenter, href: "/asset-center" },
			{ title: t.sidebar.assetTopology, href: "/asset-center/topology" },
			{ title: t.sidebar.assetMap, href: "/asset-center/map" },
		],
	},
	{
		title: t.sidebar.vulnerabilityManagement,
		icon: Bug,
		href: "/vulnerabilities",
	},
	{
		title: t.sidebar.repeater,
		icon: Repeat,
		href: "/scan/repeater",
	},
	{
		title: t.sidebar.space,
		icon: Radar,
		children: [
			{ title: t.sidebar.uncover, href: "/space/uncover" },
			{ title: t.sidebar.fofa, href: "/space/fofa" },
			{ title: t.sidebar.hunter, href: "/space/hunter" },
			{ title: t.sidebar.quake, href: "/space/quake" },
		],
	},
	{
		title: t.sidebar.downloadCenter,
		icon: FolderSearch,
		href: "/space/download-center",
	},
	{
		title: t.sidebar.tempManagement,
		icon: FileCode2,
		children: [
			{ title: t.sidebar.tempRepository, href: "/poc-manager" },
			{ title: t.sidebar.tempDebug, href: "/poc/nuclei-template" },
		],
	},
	{
		title: t.sidebar.moreFeatures,
		icon: MoreHorizontal,
		pinnable: true,
		presentation: "popover",
		children: [
			{ title: t.sidebar.companyInfo, href: "/asset/company-info", icon: Building2 },
			{ title: t.sidebar.codec, href: "/tools/codec", icon: Binary },
			{ title: t.sidebar.jwt, href: "/tools/jwt", icon: Braces },
			{ title: t.sidebar.apiDocTest, href: "/tools/api-doc-test", icon: FileSearch },
			{ title: t.sidebar.tasklistAV, href: "/tools/tasklist-av", icon: ShieldCheck },
			{ title: t.sidebar.textCompare, href: "/tools/text-compare", icon: Diff },
			{ title: t.sidebar.database, href: "/tools/database", icon: Database },
			{ title: t.sidebar.keyManager, href: "/tools/key-manager", icon: KeyRound },
			...(desktopMode
				? [{ title: t.sidebar.miniProgramDecompile, href: "/tools/mini-program-decompile", icon: PackageOpen }]
				: []),
		],
	},
	{
		placement: "footer",
		title: t.sidebar.systemManagement,
		icon: Settings,
		presentation: "popover",
		children: [
			...(!desktopMode ? [
				{ title: t.vulnScan.agents.tab, href: "/agents", icon: Server },
				{ title: t.sidebar.settings, href: "/settings", icon: Settings },
			] : []),
			{ title: t.sidebar.mcp, href: "/platform/mcp", icon: ServerCog },
			{ title: t.sidebar.logs, href: "/backend-logs", icon: ScrollText },
		],
	},
];

const SYSTEM_UPDATE_SNAPSHOT_KEY = "hephaestus:system-update-snapshot";
const CONFIG_UPDATE_SNAPSHOT_KEY = "hephaestus:config-update-snapshot";

type UpdateSnapshotPayload = {
	currentVersion?: string;
	hasUpdate: boolean;
	updateInfo?: any | null;
	noUpdate?: boolean;
};

const getClientUpdateTarget = async () => {
	return (await isDesktopRuntime()) ? "desktop" : "client";
};

const getIgnoredVersionsStorageKey = async () => {
	return (await isDesktopRuntime())
		? "ignoredDesktopVersions"
		: "ignoredVersions";
};

const persistSystemUpdateSnapshot = (payload: UpdateSnapshotPayload) => {
	if (typeof window === "undefined") return;
	const snapshot = {
		currentVersion: payload.currentVersion || "",
		hasUpdate: payload.hasUpdate,
		noUpdate: payload.noUpdate ?? !payload.hasUpdate,
		updateInfo: payload.updateInfo ?? null,
		checkedAt: Date.now(),
	};
	localStorage.setItem(SYSTEM_UPDATE_SNAPSHOT_KEY, JSON.stringify(snapshot));
	window.dispatchEvent(
		new CustomEvent("hephaestus:system-update-changed", { detail: snapshot })
	);
};

const persistConfigUpdateSnapshot = (payload: UpdateSnapshotPayload) => {
	if (typeof window === "undefined") return;
	const snapshot = {
		currentVersion: payload.currentVersion || "v0.0.0",
		hasUpdate: payload.hasUpdate,
		noUpdate: payload.noUpdate ?? !payload.hasUpdate,
		updateInfo: payload.updateInfo ?? null,
		checkedAt: Date.now(),
	};
	localStorage.setItem(CONFIG_UPDATE_SNAPSHOT_KEY, JSON.stringify(snapshot));
	window.dispatchEvent(
		new CustomEvent("hephaestus:config-update-changed", { detail: snapshot })
	);
};

export default function MainLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	const { t } = useI18n();
	const [desktopMode, setDesktopMode] = useState(false);
	const navItems = useMemo(() => getNavItems(t, desktopMode), [desktopMode, t]);
	const extraRouteTitles = useMemo<Record<string, string>>(
		() => ({
			"/agents": t.vulnScan.agents.tab,
			"/backend-logs": t.settings.tabs.logs,
			"/launcher": "应用启动器",
		}),
		[t]
	);
	const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
	const [narrowViewport, setNarrowViewport] = useState(false);
	const [mobileNavigationOpen, setMobileNavigationOpen] = useState(false);
	const layoutRef = useRef<HTMLDivElement>(null);
	const [expandedItem, setExpandedItem] = useState<string | null>(null);
	const [hoveredMenuItem, setHoveredMenuItem] = useState<string | null>(null);
	const hoverTimeoutRef = useRef<NodeJS.Timeout | null>(null);
	const [user, setUser] = useState({ username: "", email: "" });
	const getPinsSnapshot = useCallback(() => readSidebarPinsSnapshot(user.username), [user.username]);
	const pinsSnapshot = useSyncExternalStore(subscribeSidebarPins, getPinsSnapshot, emptySidebarPinsSnapshot);
	const pinnedRoutes = useMemo(() => parseSidebarPins(pinsSnapshot), [pinsSnapshot]);
	const pinnableItems = navItems.flatMap((item) => item.pinnable ? item.children || [] : []);
	const pinnedItems = pinnedRoutes.flatMap((href) => {
		const child = pinnableItems.find((item) => item.href === href);
		return child ? [{ title: child.title, href: child.href, icon: child.icon || MoreHorizontal }] : [];
	});
	const [aboutDialogOpen, setAboutDialogOpen] = useState(false);
	const [changePasswordDialogOpen, setChangePasswordDialogOpen] = useState(false);
	const [ratingDialogOpen, setRatingDialogOpen] = useState(false);
	const userMenuTriggerRef = useRef<HTMLButtonElement>(null);
	const [contentTabsEnabled, setContentTabsEnabled] = useState(false);
	const [sidebarVariant, setSidebarVariant] = useState<SidebarVariant>("sidebar");
	const [layoutTabs, setLayoutTabs] = useState<LayoutTab[]>([]);
	const [updateDialogOpen, setUpdateDialogOpen] = useState(false);
	const [currentVersion, setCurrentVersion] = useState("");
	const [updateInfo, setUpdateInfo] = useState<any>(null);
	const [currentConfigVersion, setCurrentConfigVersion] = useState("v0.0.0");
	const [configUpdateInfo, setConfigUpdateInfo] = useState<any>(null);
	const [logoutConfirmOpen, setLogoutConfirmOpen] = useState(false);
	const handleTabsWheel = (event: WheelEvent<HTMLDivElement>) => {
		const container = event.currentTarget;
		const maxScrollLeft = container.scrollWidth - container.clientWidth;

		if (maxScrollLeft <= 0 || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) {
			return;
		}

		event.preventDefault();
		event.stopPropagation();
		container.scrollLeft = Math.max(
			0,
			Math.min(maxScrollLeft, container.scrollLeft + event.deltaY)
		);
	};
	const pathname = usePathname();
	const searchParams = useSearchParams();
	const router = useRouter();
	const isSidebarCompact = sidebarCollapsed && !narrowViewport;
	const currentRoute = pathname + (searchParams?.toString() ? `?${searchParams.toString()}` : "");

	// Prefer the most specific route so nested asset views do not also select Asset Center.
	const activeHref = navItems.flatMap((item) => item.children || [item])
		.filter((item) => pathname === item.href || pathname.startsWith(`${item.href}/`))
		.sort((a, b) => b.href.length - a.href.length)[0]?.href;
	const activeMenu = navItems.find((item) => item.children?.some((child) => child.href === activeHref));
	const routeExpansionKey = `${pathname}:${activeMenu?.title || ""}`;
	const [expandedRoute, setExpandedRoute] = useState<string | null>(null);
	if (expandedRoute !== routeExpansionKey) {
		setExpandedRoute(routeExpansionKey);
		setExpandedItem(activeMenu?.presentation === "popover" ? null : activeMenu?.title || null);
	}

	const resolveRouteTitle = useCallback((href: string) => {
		const routePath = getLayoutRoutePath(href);
		if (extraRouteTitles[routePath]) {
			return extraRouteTitles[routePath];
		}

		for (const item of navItems) {
			if (item.href === routePath) return item.title;
			if (item.children) {
				const child = item.children.find((entry) => entry.href === routePath);
				if (child) return child.title;
			}
		}

		const parent = navItems.flatMap(item => item.children || (item.href ? [item] : []))
			.filter(item => routePath.startsWith(`${item.href}/`))
			.sort((a, b) => b.href.length - a.href.length)[0];
		return parent?.title || "Hephaestus";
	}, [extraRouteTitles, navItems]);

	const isTabEligibleRoute = useCallback((href: string) => {
		const routePath = getLayoutRoutePath(href);
		if (extraRouteTitles[routePath]) {
			return true;
		}

		for (const item of navItems) {
			if (item.href === routePath) {
				return true;
			}
			if (item.children?.some((entry) => entry.href === routePath)) {
				return true;
			}
		}

		return false;
	}, [extraRouteTitles, navItems]);

	const upsertLayoutTab = useCallback((href: string, title?: string) => {
		if (!contentTabsEnabled) return;
		setLayoutTabs((prev) => mergeLayoutTab(prev, href, title || resolveRouteTitle(href)));
	}, [contentTabsEnabled, resolveRouteTitle]);

	useEffect(() => {
		const userData = localStorage.getItem("userData");

		if (!getToken()) {
			router.push("/");
			return;
		}

		const hydrateUser = async () => {
			try {
				const parsed = JSON.parse(userData || "{}");
				setUser({
					username: parsed?.username || "",
					email: parsed?.email || "",
				});
			} catch {
				setUser({ username: "", email: "" });
			}

			try {
				const response = await fetchWithAuth("/api/v1/auth/user", { cache: "no-store" });
				if (!response.ok) {
					return;
				}
				const data = await response.json().catch(() => ({}));
				if (!data?.success || !data?.data) {
					return;
				}
				const nextUser = {
					username: String(data.data.username || ""),
					email: String(data.data.email || ""),
				};
				setUser(nextUser);
				try {
					const cached = JSON.parse(userData || "{}");
					localStorage.setItem("userData", JSON.stringify({ ...cached, ...nextUser }));
				} catch {
					localStorage.setItem("userData", JSON.stringify(nextUser));
				}
			} catch {
				// 使用本地缓存回退
			}
		};

		void hydrateUser();
	}, [router]);

	useEffect(() => {
		let cancelled = false;

		const detectDesktop = async () => {
			const desktop = await isDesktopRuntime();
			if (!cancelled) {
				setDesktopMode(desktop);
			}
		};

		void detectDesktop();
		return () => {
			cancelled = true;
		};
	}, []);

	useEffect(() => {
		if (typeof window === "undefined") return;

		const media = window.matchMedia("(max-width: 767px)");
		const viewport = window.visualViewport;
		const updateViewport = () => {
			setNarrowViewport(media.matches);
			if (!media.matches) setMobileNavigationOpen(false);
			// Follow the visible area when the mobile keyboard or browser bars resize it.
			const height = media.matches && viewport ? viewport.height : window.innerHeight;
			layoutRef.current?.style.setProperty("--app-viewport-height", `${height}px`);
			layoutRef.current?.style.setProperty("--app-viewport-top", `${media.matches ? viewport?.offsetTop || 0 : 0}px`);
		};
		updateViewport();
		media.addEventListener("change", updateViewport);
		window.addEventListener("resize", updateViewport);
		viewport?.addEventListener("resize", updateViewport);
		viewport?.addEventListener("scroll", updateViewport);
		return () => {
			media.removeEventListener("change", updateViewport);
			window.removeEventListener("resize", updateViewport);
			viewport?.removeEventListener("resize", updateViewport);
			viewport?.removeEventListener("scroll", updateViewport);
		};
	}, []);

	useEffect(() => {
		if (typeof window === "undefined") return;

		return subscribeContentTabsEnabled((enabled) => {
			setContentTabsEnabled(enabled);
		});
	}, []);

	useEffect(() => {
		if (typeof window === "undefined") return;

		return subscribeSidebarVariant((variant) => {
			setSidebarVariant(variant);
		});
	}, []);

	useEffect(() => {
		if (!contentTabsEnabled || !pathname || !isTabEligibleRoute(currentRoute)) return;
		const title = resolveRouteTitle(currentRoute);
		const timer = window.setTimeout(() => {
			upsertLayoutTab(currentRoute, title);
		}, 0);

		return () => window.clearTimeout(timer);
	}, [contentTabsEnabled, currentRoute, isTabEligibleRoute, pathname, resolveRouteTitle, upsertLayoutTab]);

	useEffect(() => {
		if (!contentTabsEnabled) return;
		const timer = window.setTimeout(() => {
			setLayoutTabs((prev) => {
				let changed = false;
				const nextTabs = prev.map((tab) => {
					const nextTitle = resolveRouteTitle(tab.href);
					if (tab.title === nextTitle) {
						return tab;
					}
					changed = true;
					return { ...tab, title: nextTitle };
				});
				return changed ? nextTabs : prev;
			});
		}, 0);

		return () => window.clearTimeout(timer);
	}, [contentTabsEnabled, resolveRouteTitle]);

	const renderUserMenuContent = () => (
		<DropdownMenuContent
			align="start"
			side={narrowViewport ? "top" : "right"}
			alignOffset={-28}
			sideOffset={isSidebarCompact ? 20 : 12}
			collisionPadding={{ top: 12, right: 12, bottom: 20, left: 12 }}
			onCloseAutoFocus={(event) => {
				if (ratingDialogOpen) event.preventDefault();
			}}
			className="w-64 ml-1 rounded-2xl border-border/60 bg-popover/98 p-2 shadow-2xl backdrop-blur-xl"
		>
			<DropdownMenuLabel className="p-0 font-normal">
				<div className="rounded-xl bg-muted/40 px-3 py-3">
					<div className="flex items-center gap-3">
						<Avatar className="size-10">
							<AvatarFallback className="bg-gradient-to-br from-primary/20 to-primary/10 text-primary font-semibold">
								{user.username.slice(0, 2).toUpperCase()}
							</AvatarFallback>
						</Avatar>
						<p className="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">{user.username}</p>
					</div>
				</div>
			</DropdownMenuLabel>
			<DropdownMenuSeparator className="my-2" />
			<DropdownMenuItem
				className="cursor-pointer rounded-xl px-3 py-2.5"
				onClick={() => setChangePasswordDialogOpen(true)}
			>
				<KeyRound className="w-4 h-4" />
				<span>{t.sidebar.changePassword}</span>
			</DropdownMenuItem>
			<DropdownMenuItem
				className="cursor-pointer rounded-xl px-3 py-2.5"
				onClick={handleManualCheckUpdate}
			>
				<RefreshCw className="w-4 h-4" />
				<span>{t.sidebar.checkUpdate}</span>
			</DropdownMenuItem>
			<DropdownMenuItem
				className="cursor-pointer rounded-xl px-3 py-2.5"
				onClick={() => setAboutDialogOpen(true)}
			>
				<HelpCircle className="w-4 h-4" />
				<span>{t.sidebar.about}</span>
			</DropdownMenuItem>
			<DropdownMenuItem
				aria-haspopup="dialog"
				className="cursor-pointer rounded-xl px-3 py-2.5"
				onSelect={() => setRatingDialogOpen(true)}
			>
				<Star className="w-4 h-4" />
				<span>{t.sidebar.rating.title}</span>
			</DropdownMenuItem>
			<DropdownMenuSeparator className="my-2" />
			<DropdownMenuItem
				variant="destructive"
				className="cursor-pointer rounded-xl px-3 py-2.5"
				onClick={handleLogout}
			>
				<LogOut className="w-4 h-4" />
				<span>{t.sidebar.logout}</span>
			</DropdownMenuItem>
		</DropdownMenuContent>
	);

	// 清理定时器
	useEffect(() => {
		return () => {
			if (hoverTimeoutRef.current) {
				clearTimeout(hoverTimeoutRef.current);
			}
		};
	}, []);

	const toggleExpand = (title: string) => {
		setExpandedItem((prev) => prev === title ? null : title);
	};

	const allowLayoutNavigation = () => window.dispatchEvent(new Event("hephaestus:before-layout-navigation", { cancelable: true }));

	const navigateTo = (href: string, title?: string) => {
		if (href !== currentRoute && !allowLayoutNavigation()) return;
		setMobileNavigationOpen(false);
		upsertLayoutTab(href, title);
		router.push(href);
	};

	const renderNavChildren = (children: NavChild[], pinnable = false) => children.map((child) => {
		const pinned = pinnedRoutes.includes(child.href);
		const pinLabel = `${pinned ? t.sidebar.unpinFromSidebar : t.sidebar.pinToSidebar}: ${child.title}`;
		return (
			<div key={child.href} className="group/pin relative min-w-0">
				<Button
					variant="ghost"
					data-sidebar-nav-item="true"
					data-nav-level="child"
					data-nav-active={activeHref === child.href ? "true" : undefined}
					aria-current={activeHref === child.href ? "page" : undefined}
					title={child.title}
					className={cn(
						"w-full min-w-0 justify-start rounded-md text-sm text-foreground",
						narrowViewport && "h-11",
						pinnable && "pr-9 has-[>svg]:pr-9",
						activeHref === child.href && "bg-muted/55 text-primary shadow-sm ring-1 ring-border/60"
					)}
					onClick={() => {
						navigateTo(child.href, child.title);
						setHoveredMenuItem(null);
					}}
				>
					{child.icon && <child.icon className="h-4 w-4 shrink-0" />}
					<span className="truncate">{child.title}</span>
				</Button>
				{pinnable && (
					<Tooltip>
						<TooltipTrigger asChild>
							<Button
								variant="ghost"
								size="icon-sm"
								aria-label={pinLabel}
								aria-pressed={pinned}
								disabled={!user.username}
								className={cn(
									"absolute right-0.5 top-1/2 -translate-y-1/2 rounded-md",
									pinned ? "text-primary" : "text-muted-foreground hover:text-foreground"
								)}
								onClick={() => {
									if (!toggleSidebarPin(user.username, child.href)) toast.error(t.sidebar.pinSaveFailed);
								}}
							>
								{pinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
							</Button>
						</TooltipTrigger>
						<TooltipContent side="right">
							{pinned ? t.sidebar.unpinFromSidebar : t.sidebar.pinToSidebar}
						</TooltipContent>
					</Tooltip>
				)}
			</div>
		);
	});

	const isNavItemActive = (item: NavItem) => item.children
		? item.children.some((child) => child.href === activeHref) && !(item.pinnable && pinnedRoutes.includes(activeHref || ""))
		: item.href === activeHref;

	const handleCloseLayoutTab = (href: string) => {
		if (href === currentRoute && !allowLayoutNavigation()) return;
		const nextTabs = layoutTabs.filter((tab) => tab.href !== href);
		setLayoutTabs(nextTabs);
		if (currentRoute === href) {
			const fallbackTab = nextTabs[nextTabs.length - 1];
			router.push(fallbackTab?.href || "/dashboard");
		}
	};

	const handleCloseOtherLayoutTabs = (href: string) => {
		const targetTab = layoutTabs.find((tab) => tab.href === href);
		const nextTabs = targetTab ? [targetTab] : [];
		setLayoutTabs(nextTabs);
	};

	const handleCloseRightLayoutTabs = (href: string) => {
		const targetIndex = layoutTabs.findIndex((tab) => tab.href === href);
		if (targetIndex < 0) return;
		const nextTabs = layoutTabs.slice(0, targetIndex + 1);
		if (!nextTabs.some((tab) => tab.href === currentRoute) && !allowLayoutNavigation()) return;
		setLayoutTabs(nextTabs);
		if (!nextTabs.some((tab) => tab.href === currentRoute)) {
			router.push(nextTabs[nextTabs.length - 1]?.href || "/dashboard");
		}
	};

	const handleCloseAllLayoutTabs = () => {
		if (!allowLayoutNavigation()) return;
		setLayoutTabs([]);
		router.push("/dashboard");
	};

	// 检查更新
	const checkForUpdates = useCallback(async (_silent = true) => {
		try {
			const target = await getClientUpdateTarget();
			const infoRes = await fetchWithAuth(`/api/v1/system/update/info?target=${target}`);
			const infoData = await infoRes.json().catch(() => null);
			let latestVersion = "";
			if (infoRes.ok && infoData?.success) {
				latestVersion = infoData.data.version;
				setCurrentVersion(latestVersion);
			}

			const res = await fetchWithAuth(`/api/v1/system/update/check?target=${target}`);
			if (!res.ok) {
				throw new Error('检查更新请求失败');
			}
			const data = await res.json().catch(() => null);

			if (!data?.success) {
				throw new Error(data?.error || '检查更新失败');
			}

			if (data.hasUpdate) {
				// 暂存的更新包只对同一版本有效，检测到新版本时丢弃旧的“已下载完成”状态。
				if (readStagedUpdateVersion() !== data.updateInfo.version) {
					persistStagedUpdateVersion(null);
				}
				persistSystemUpdateSnapshot({
					currentVersion: latestVersion,
					hasUpdate: true,
					updateInfo: data.updateInfo,
					noUpdate: false,
				});
				const ignoredVersionsKey = await getIgnoredVersionsStorageKey();
				const ignoredVersions = JSON.parse(localStorage.getItem(ignoredVersionsKey) || '[]');
				const ignored = ignoredVersions.includes(data.updateInfo.version);
				return {
					hasUpdate: true,
					ignored,
					updateInfo: ignored ? null : data.updateInfo,
					currentVersion: latestVersion,
				};
			}
			persistStagedUpdateVersion(null);
			persistSystemUpdateSnapshot({
				currentVersion: latestVersion,
				hasUpdate: false,
				updateInfo: null,
				noUpdate: true,
			});
			return { hasUpdate: false, ignored: false, updateInfo: null, currentVersion: latestVersion };
		} catch (error) {
			throw error;
		}
	}, []);

	const checkForConfigUpdates = useCallback(async (silent = true) => {
		try {
			const infoRes = await fetchWithAuth('/api/v1/system/update/config/info');
			const infoData = await infoRes.json().catch(() => null);
			let currentConfigVersion = "v0.0.0";
			if (infoRes.ok && infoData?.success) {
				currentConfigVersion = infoData.data?.version || "v0.0.0";
			}
			setCurrentConfigVersion(currentConfigVersion);

			const res = await fetchWithAuth('/api/v1/system/update/config/check');
			if (!res.ok) {
				throw new Error('检查配置更新请求失败');
			}
			const data = await res.json().catch(() => null);

			if (!data?.success) {
				throw new Error(data?.error || '检查配置更新失败');
			}

			if (data.hasUpdate) {
				persistConfigUpdateSnapshot({
					currentVersion: currentConfigVersion,
					hasUpdate: true,
					updateInfo: data.updateInfo,
					noUpdate: false,
				});
				return {
					hasUpdate: true,
					updateInfo: data.updateInfo,
					currentVersion: currentConfigVersion,
				};
			}

			persistConfigUpdateSnapshot({
				currentVersion: data.currentVersion || currentConfigVersion || "v0.0.0",
				hasUpdate: false,
				updateInfo: null,
				noUpdate: true,
			});
			return {
				hasUpdate: false,
				updateInfo: null,
				currentVersion: data.currentVersion || currentConfigVersion || "v0.0.0",
			};
		} catch (error) {
			if (!silent) {
				throw error;
			}
			console.error('检查配置更新失败:', error);
			return { hasUpdate: false, updateInfo: null, currentVersion: currentConfigVersion };
		}
	}, [currentConfigVersion]);

	const checkAllUpdates = useCallback(async (silent = true) => {
		const [clientResult, configResult] = await Promise.all([
			checkForUpdates(silent),
			checkForConfigUpdates(silent),
		]);

		setCurrentVersion(clientResult.currentVersion || "");
		setCurrentConfigVersion(configResult.currentVersion || "v0.0.0");
		setUpdateInfo(clientResult.updateInfo || null);
		setConfigUpdateInfo(configResult.updateInfo || null);

		const hasVisibleUpdate = Boolean(clientResult.updateInfo || configResult.updateInfo);
		setUpdateDialogOpen(hasVisibleUpdate);

		return {
			clientHasUpdate: clientResult.hasUpdate,
			configHasUpdate: configResult.hasUpdate,
			hasVisibleUpdate,
		};
	}, [checkForUpdates, checkForConfigUpdates]);

	// 启动检查和定时检查更新
	useEffect(() => {
		// 启动3秒后检查更新
		const startupTimer = setTimeout(() => {
			void checkAllUpdates(true);
		}, 3000);

		// 每小时检查一次更新
		const interval = setInterval(() => {
			void checkAllUpdates(true);
		}, 60 * 60 * 1000);

		return () => {
			clearTimeout(startupTimer);
			clearInterval(interval);
		};
	}, [checkAllUpdates]);

	// 忽略版本
	const handleIgnoreVersion = async (version: string) => {
		const ignoredVersionsKey = await getIgnoredVersionsStorageKey();
		const ignoredVersions = JSON.parse(localStorage.getItem(ignoredVersionsKey) || '[]');
		ignoredVersions.push(version);
		localStorage.setItem(ignoredVersionsKey, JSON.stringify(ignoredVersions));
	};

	const handleUpdateComplete = (target: 'client' | 'config', nextVersion: string) => {
		if (target !== 'config') {
			return;
		}

		const completedVersion = nextVersion
			? (nextVersion.startsWith('v') ? nextVersion : `v${nextVersion}`)
			: "v0.0.0";
		setCurrentConfigVersion(completedVersion);
		setConfigUpdateInfo(null);
		persistConfigUpdateSnapshot({
			currentVersion: completedVersion,
			hasUpdate: false,
			updateInfo: null,
			noUpdate: true,
		});

		if (!updateInfo) {
			setUpdateDialogOpen(false);
		}
	};

	// 是否锁住关闭由 UpdateDialog 内部判断（下载中/更新包就绪），这里只同步开关状态。
	const handleUpdateDialogOpenChange = (nextOpen: boolean) => {
		setUpdateDialogOpen(nextOpen);
	};

	// 手动检查更新
	const handleManualCheckUpdate = async () => {
		const toastId = toast.loading('正在检查更新...');
		try {
			const result = await checkAllUpdates(false);
			if (result.hasVisibleUpdate) {
				toast.dismiss(toastId);
			} else {
				toast.success('当前已是最新版本', { id: toastId });
			}
		} catch (error) {
			toast.error('检查更新失败', { id: toastId });
		}
	};

	const performLogout = () => {
		if (!allowLayoutNavigation()) return;
		clearToken();

		setLogoutConfirmOpen(false);
		router.push("/");
	};

	const handleLogout = () => {
		setLogoutConfirmOpen(true);
	};

	const renderNavItem = (item: NavItem) => (
		<div key={item.title}>
			{item.children ? (
					isSidebarCompact || item.presentation === "popover" ? (
						<Popover
							open={hoveredMenuItem === item.title}
							onOpenChange={(open) => {
								if (hoverTimeoutRef.current) {
									clearTimeout(hoverTimeoutRef.current);
									hoverTimeoutRef.current = null;
								}
								setHoveredMenuItem(open ? item.title : null);
							}}
						>
							<PopoverTrigger asChild>
								<Button
									variant="ghost"
									data-sidebar-nav-item="true"
									data-nav-level="top"
									aria-label={item.title}
									aria-expanded={hoveredMenuItem === item.title}
									data-nav-active={isNavItemActive(item) ? "true" : undefined}
									className={cn(
										"w-full mb-1 text-foreground rounded-md",
										isSidebarCompact ? "justify-center px-2" : "justify-start",
										isNavItemActive(item) && "bg-muted/55 text-primary shadow-sm ring-1 ring-border/60"
									)}
									onMouseEnter={() => {
										if (item.presentation === "popover") return;
										// 清除之前的定时器
										if (hoverTimeoutRef.current) {
											clearTimeout(hoverTimeoutRef.current);
											hoverTimeoutRef.current = null;
										}
										setHoveredMenuItem(item.title);
									}}
									onMouseLeave={() => {
										if (item.presentation === "popover") return;
										// 延迟清除，给用户时间移动到 Popover
										hoverTimeoutRef.current = setTimeout(() => {
											setHoveredMenuItem(null);
											hoverTimeoutRef.current = null;
										}, 150);
									}}
								>
									<item.icon className="w-4 h-4 shrink-0" />
									{!isSidebarCompact && <span className="ml-2">{item.title}</span>}
								</Button>
							</PopoverTrigger>
							<PopoverContent
								side={narrowViewport ? "top" : "right"}
								align={narrowViewport ? "center" : item.placement === "footer" ? "end" : "start"}
								sideOffset={item.presentation === "popover" ? (isSidebarCompact ? 20 : 12) : 8}
								collisionPadding={12}
								aria-label={item.title}
								className={cn(
									"max-h-[var(--radix-popover-content-available-height)] max-w-[calc(100vw-24px)] overflow-y-auto p-2",
									item.presentation === "popover" ? "w-64 ml-1 rounded-xl" : "w-56"
								)}
								onMouseEnter={() => {
									if (item.presentation === "popover") return;
									// 清除定时器，保持 Popover 打开
									if (hoverTimeoutRef.current) {
										clearTimeout(hoverTimeoutRef.current);
										hoverTimeoutRef.current = null;
									}
									setHoveredMenuItem(item.title);
								}}
								onMouseLeave={() => {
									if (item.presentation === "popover") return;
									setHoveredMenuItem(null);
								}}
							>
								<div className="space-y-1">
									{renderNavChildren(item.children, item.pinnable)}
								</div>
							</PopoverContent>
						</Popover>
					) : (
				<div>
					<Button
						variant="ghost"
						data-sidebar-nav-item="true"
						data-nav-level="top"
						aria-label={item.title}
						aria-expanded={expandedItem === item.title}
						data-nav-active={isNavItemActive(item) ? "true" : undefined}
						className={cn(
							"w-full justify-start mb-1 text-foreground rounded-md group/item",
							isNavItemActive(item) && "bg-muted/55 text-primary shadow-sm ring-1 ring-border/60"
						)}
						onClick={() => toggleExpand(item.title)}
					>
						<item.icon className="w-4 h-4 flex-shrink-0" />
						<span className={cn(
							"ml-2 transition-opacity duration-300 ease-in-out whitespace-nowrap flex-1 text-left",
							isSidebarCompact ? "opacity-0 w-0 overflow-hidden" : "opacity-100"
						)}>{item.title}</span>
						{!isSidebarCompact && (
							<ChevronRight
								className={cn(
									"w-4 h-4 ml-auto flex-shrink-0 transition-transform duration-300 ease-in-out",
									(expandedItem === item.title) && "rotate-90"
								)}
							/>
						)}
					</Button>
					<div
						className={cn(
							"grid transition-[grid-template-rows,opacity] duration-300 ease-in-out",
							expandedItem === item.title ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
						)}
						aria-hidden={expandedItem !== item.title}
						inert={expandedItem !== item.title}
					>
						<div className="min-h-0 overflow-hidden">
							<div className="mb-2 ml-4 space-y-1 border-l border-border/50 py-1 pl-2 pr-1">
								{renderNavChildren(item.children)}
							</div>
						</div>
					</div>
				</div>
					)
				) : (
					isSidebarCompact ? (
						<Tooltip>
							<TooltipTrigger asChild>
								<Button
									variant="ghost"
									data-sidebar-nav-item="true"
									data-nav-level="top"
									aria-label={item.title}
									aria-current={activeHref === item.href ? "page" : undefined}
									data-nav-active={activeHref === item.href ? "true" : undefined}
									className={cn(
										"w-full justify-center px-2 mb-1 text-foreground rounded-md",
					activeHref === item.href && "bg-muted/55 text-primary shadow-sm ring-1 ring-border/60"
									)}
									onClick={() => navigateTo(item.href, item.title)}
								>
									<item.icon className="w-4 h-4" />
								</Button>
							</TooltipTrigger>
							<TooltipContent side="right">
								{item.title}
							</TooltipContent>
						</Tooltip>
			) : (
				<Button
					variant="ghost"
					data-sidebar-nav-item="true"
					data-nav-level="top"
					aria-label={item.title}
					aria-current={activeHref === item.href ? "page" : undefined}
					data-nav-active={activeHref === item.href ? "true" : undefined}
					className={cn(
						"w-full justify-start mb-1 text-foreground rounded-md",
				activeHref === item.href && "bg-muted/55 text-primary shadow-sm ring-1 ring-border/60"
					)}
					onClick={() => navigateTo(item.href, item.title)}
				>
					<item.icon className="w-4 h-4 flex-shrink-0" />
						<span className={cn(
							"ml-2 transition-opacity duration-300 ease-in-out whitespace-nowrap",
							isSidebarCompact ? "opacity-0 w-0 overflow-hidden" : "opacity-100"
						)}>{item.title}</span>
				</Button>
					)
			)}
		</div>
	);

	const navigation = (
			<aside
				className={cn(
					"flex h-full min-h-0 shrink-0 flex-col bg-card transition-all duration-300 ease-in-out overflow-hidden",
					!narrowViewport && sidebarVariant === "sidebar" && "border-r border-border",
					!narrowViewport && sidebarVariant === "inset" && "rounded-xl border border-border shadow-sm",
					!narrowViewport && sidebarVariant === "floating" && "rounded-xl border border-border shadow-xl shadow-black/10",
					narrowViewport ? "w-full [&_nav_button]:min-h-11" : isSidebarCompact ? "w-16" : "w-64"
				)}
			>
				<div
					className={cn(
							"relative flex h-[58px] items-center",
						isSidebarCompact ? "justify-center px-0" : "px-3"
					)}
				>
					<div
						className={cn(
							"flex items-center transition-opacity duration-300 ease-in-out min-w-0",
							desktopMode ? "gap-2.5" : "gap-2",
							isSidebarCompact ? "opacity-0 w-0 overflow-hidden" : "opacity-100 w-auto"
						)}
					>
						{desktopMode ? (
							<>
									<div className="flex h-8 w-8 items-center justify-center rounded-xl border border-border/60 bg-muted/20 shadow-[0_10px_24px_rgba(0,0,0,0.16)]">
										<Image
											src="/brand-mark.png"
											alt="Hephaestus"
											width={24}
											height={24}
											unoptimized
											className="app-brand-mark-image h-6 w-6 flex-shrink-0"
										/>
									</div>
								<div className="min-w-0">
									<p className="truncate text-[15px] font-semibold leading-5 text-foreground">Hephaestus</p>
									<p className="truncate text-[10px] font-medium uppercase tracking-[0.18em] text-muted-foreground/80">Desktop</p>
								</div>
							</>
						) : (
							<>
									<div className="flex h-8 w-8 items-center justify-center rounded-lg border border-border/60 bg-muted/35 shadow-sm">
										<Image
											src="/brand-mark.png"
											alt="Hephaestus"
											width={18}
											height={18}
											unoptimized
											className="app-brand-mark-image h-[18px] w-[18px] flex-shrink-0"
										/>
									</div>
								<span className="font-bold text-lg whitespace-nowrap text-foreground">Hephaestus</span>
							</>
						)}
					</div>
					<Button
						variant="ghost"
						size="icon"
						aria-label={narrowViewport ? t.common.close : t.sidebar.navigation}
						onClick={() => narrowViewport ? setMobileNavigationOpen(false) : setSidebarCollapsed(!sidebarCollapsed)}
						className={cn(
							"h-8 w-8 rounded-lg text-muted-foreground hover:bg-muted/60 hover:text-foreground",
							isSidebarCompact ? "static" : "absolute right-3"
						)}
					>
						{narrowViewport ? <X className="w-4 h-4" /> : isSidebarCompact ? (
							<Menu className="w-4 h-4" />
						) : (
							<ChevronLeft className="w-4 h-4" />
						)}
					</Button>
				</div>

				<nav aria-label={t.sidebar.navigation} className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden p-2">
					<TooltipProvider delayDuration={200}>
					{navItems.filter((item) => item.placement !== "footer").map(renderNavItem)}
					{pinnedItems.length > 0 && (
						<div data-sidebar-pinned="true" role="group" aria-label={t.sidebar.pinnedFeatures}>
							<div className="mx-2 my-3 border-t border-border/50" role="separator" />
							{pinnedItems.map(renderNavItem)}
						</div>
					)}
					</TooltipProvider>
					</nav>

				<div className="shrink-0 px-2 pt-2" data-sidebar-footer="true">
					<TooltipProvider delayDuration={200}>
						{navItems.filter((item) => item.placement === "footer").map(renderNavItem)}
					</TooltipProvider>
				</div>

					{/* 用户区域 */}
					<div className="p-2 [&_button[data-state=open]]:border-t-0">
						{!isSidebarCompact ? (
							<DropdownMenu>
							<DropdownMenuTrigger asChild>
								<button ref={userMenuTriggerRef} className="w-full p-3 hover:bg-sidebar-hover transition-colors rounded-lg outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-0 data-[state=open]:bg-sidebar-hover data-[state=open]:border-t-0">
									<div className="flex items-center gap-3">
										<Avatar className="size-8">
											<AvatarFallback className="bg-gradient-to-br from-primary/20 to-primary/10 text-primary font-semibold">
												{user.username.slice(0, 2).toUpperCase()}
											</AvatarFallback>
										</Avatar>
										<div className="flex-1 overflow-hidden min-w-0 text-left">
											<p className="text-sm font-medium truncate text-foreground">
												{user.username}
											</p>
										</div>
										<CaretSortIcon className="w-4 h-4 text-muted-foreground flex-shrink-0" />
									</div>
								</button>
							</DropdownMenuTrigger>
							{renderUserMenuContent()}
						</DropdownMenu>
					) : (
						<div className="p-2">
							<DropdownMenu>
								<DropdownMenuTrigger asChild>
									<button ref={userMenuTriggerRef} className="flex w-full aspect-square items-center justify-center rounded-full transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:bg-sidebar-hover hover:bg-sidebar-hover/70">
										<div className="relative">
											<Avatar className="size-9">
											<AvatarFallback className="bg-gradient-to-br from-primary/20 to-primary/10 text-primary font-semibold text-xs">
												{user.username.slice(0, 2).toUpperCase()}
											</AvatarFallback>
											</Avatar>
										</div>
									</button>
								</DropdownMenuTrigger>
								{renderUserMenuContent()}
							</DropdownMenu>
						</div>
					)}
				</div>
			</aside>
	);

	return (
		<div ref={layoutRef} className="fixed inset-x-0 top-[var(--app-viewport-top,0px)] flex h-[var(--app-viewport-height,100dvh)] min-w-0 flex-col overflow-hidden bg-background md:static md:h-dvh">
			<header className="flex h-12 shrink-0 items-center gap-2 border-b border-border bg-card px-2 md:hidden">
				<Button variant="ghost" size="icon" className="h-11 w-11 shrink-0" aria-label={t.sidebar.navigation} aria-expanded={mobileNavigationOpen} onClick={() => setMobileNavigationOpen(true)}><Menu className="h-5 w-5" /></Button>
				<span className="min-w-0 truncate text-sm font-semibold">{resolveRouteTitle(pathname)}</span>
			</header>
			{desktopMode && <DesktopTitlebar />}
			<div
				data-sidebar-variant={sidebarVariant}
				className={cn(
					"flex min-h-0 flex-1",
					!narrowViewport && sidebarVariant !== "sidebar" && "md:gap-3 md:p-3",
				)}
			>
{narrowViewport ? <Sheet open={mobileNavigationOpen} onOpenChange={setMobileNavigationOpen}>
				<SheetContent side="left" aria-describedby={undefined} className="w-[min(320px,calc(100vw-48px))] gap-0 p-0 [&>button]:hidden">
					<SheetTitle className="sr-only">{t.sidebar.navigation}</SheetTitle>
					{navigation}
				</SheetContent>
			</Sheet> : <div className="hidden min-h-0 shrink-0 md:block">{navigation}</div>}

		<main
			className={cn(
				"min-h-0 min-w-0 flex-1 flex flex-col overflow-hidden",
				!narrowViewport && sidebarVariant === "inset" && "rounded-xl border border-border bg-card shadow-sm",
			)}
		>
			<AliveScope >
					{!narrowViewport && contentTabsEnabled && layoutTabs.length > 0 && (
						<div className="border-b border-border bg-card px-2 py-1">
							<Tabs value={currentRoute} className="w-full">
								<div className="heph-tabs-scrollbar overflow-x-auto" onWheel={handleTabsWheel}>
									<TabsList className="inline-flex h-auto min-w-max flex-nowrap justify-start rounded-lg border-0 bg-transparent py-0.5 shadow-none">
									{layoutTabs.map((tab) => (
										<div
											key={tab.href}
											className={cn(
												"group mr-1.5 flex shrink-0 items-center rounded-md border px-2.5 py-1 text-sm transition-colors",
												currentRoute === tab.href
													? "border-primary/40 bg-primary/10 text-primary"
													: "border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground"
											)}
										>
											<ContextMenu>
												<ContextMenuTrigger asChild>
													<TabsTrigger
														value={tab.href}
														className="h-auto shrink-0 whitespace-nowrap bg-transparent p-0 text-left shadow-none data-[state=active]:bg-transparent data-[state=active]:text-inherit"
														onClick={() => navigateTo(tab.href, tab.title)}
													>
														{tab.title}
													</TabsTrigger>
												</ContextMenuTrigger>
												<ContextMenuContent className="min-w-[180px]">
													<ContextMenuItem onClick={() => handleCloseLayoutTab(tab.href)}>
														{t.theme.layoutTabs.contextMenu.close}
													</ContextMenuItem>
													<ContextMenuItem onClick={() => handleCloseOtherLayoutTabs(tab.href)}>
														{t.theme.layoutTabs.contextMenu.closeOthers}
													</ContextMenuItem>
													<ContextMenuItem onClick={() => handleCloseRightLayoutTabs(tab.href)}>
														{t.theme.layoutTabs.contextMenu.closeRight}
													</ContextMenuItem>
													<ContextMenuItem onClick={handleCloseAllLayoutTabs}>
														{t.theme.layoutTabs.contextMenu.closeAll}
													</ContextMenuItem>
												</ContextMenuContent>
											</ContextMenu>
											<Button
												variant="ghost"
												size="icon"
												className="ml-1 h-5 w-5 shrink-0 rounded-full opacity-70 hover:opacity-100"
												onClick={(e) => {
													e.stopPropagation();
													handleCloseLayoutTab(tab.href);
												}}
											>
												<X className="h-3 w-3" />
											</Button>
										</div>
									))}
								</TabsList>
							</div>
						</Tabs>
					</div>
				)}
				<div className="min-h-0 min-w-0 flex-1 overflow-y-auto">{children}</div>
			</AliveScope>
		</main>
			</div>
		<AboutDialog open={aboutDialogOpen} onOpenChange={setAboutDialogOpen} />
		<ChangePasswordDialog open={changePasswordDialogOpen} onOpenChange={setChangePasswordDialogOpen} />
		<ProductRatingDialog open={ratingDialogOpen} onOpenChange={setRatingDialogOpen} triggerRef={userMenuTriggerRef} />
		{(updateInfo || configUpdateInfo) && (
			<UpdateDialog
				open={updateDialogOpen}
				onOpenChange={handleUpdateDialogOpenChange}
				clientCurrentVersion={currentVersion}
				clientUpdateInfo={updateInfo}
				configCurrentVersion={currentConfigVersion}
				configUpdateInfo={configUpdateInfo}
				onIgnoreVersion={handleIgnoreVersion}
				onUpdateComplete={handleUpdateComplete}
			/>
		)}
		<AlertDialog open={logoutConfirmOpen} onOpenChange={setLogoutConfirmOpen}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>{t.sidebar.logout}</AlertDialogTitle>
					<AlertDialogDescription>{t.sidebar.logoutConfirm}</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>{t.common.cancel}</AlertDialogCancel>
					<AlertDialogAction
						className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
						onClick={performLogout}
					>
						{t.sidebar.logout}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	</div>
);
}
