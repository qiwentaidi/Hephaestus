"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Plus, Settings, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import toast from "@/lib/toast";
import { fetchWithAuth } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import type { DatabaseConnection } from "./types";
import { ConnectionPool } from "./components/ConnectionPool";
import { ConnectionDialog } from "./components/ConnectionDialog";
import { SettingsDialog } from "./components/SettingsDialog";
import DatabaseDetail from "./components/DatabaseDetail";

export default function DatabasePageClient() {
	const router = useRouter();
	const searchParams = useSearchParams();
	const { t } = useI18n();
	const nanoid = searchParams.get("nanoid");
	const view = searchParams.get("view");
	const isSettingsView = view === "settings";
	const [connections, setConnections] = useState<DatabaseConnection[]>([]);
	const [isDialogOpen, setIsDialogOpen] = useState(false);
	const [editingConnection, setEditingConnection] =
		useState<DatabaseConnection | null>(null);
	const [deleteConnectionId, setDeleteConnectionId] = useState<string | null>(null);
	const [searchQuery, setSearchQuery] = useState("");
	const formatTpl = (tpl: string, vars: Record<string, string | number>) =>
		Object.entries(vars).reduce(
			(acc, [k, v]) => acc.split(`{${k}}`).join(String(v)),
			tpl
		);

	// 从 sessionStorage 恢复连接状态
	const getConnectedState = useCallback((nanoid: string): boolean => {
		if (typeof window === "undefined") return false;
		const connected = sessionStorage.getItem(`db_connected_${nanoid}`);
		return connected === "true";
	}, []);

	// 保存连接状态到 sessionStorage
	const setConnectedState = useCallback((nanoid: string, connected: boolean) => {
		if (typeof window === "undefined") return;
		if (connected) {
			sessionStorage.setItem(`db_connected_${nanoid}`, "true");
		} else {
			sessionStorage.removeItem(`db_connected_${nanoid}`);
		}
	}, []);

	const fetchConnectionList = useCallback(async () => {
		const response = await fetchWithAuth(`/api/v1/database/connections`);
		const data = await response.json();
		if (!data.success) {
			throw new Error(data.error || t.tools.database.toasts.loadConnectionsFailed);
		}
		return (data.data || []) as DatabaseConnection[];
	}, [t.tools.database.toasts.loadConnectionsFailed]);

	const mergeConnections = useCallback((prev: DatabaseConnection[], next: DatabaseConnection[]) => {
		const prevMap = new Map(prev.map((c) => [c.nanoid, c]));
		return next.map((conn) => {
			const existing = prevMap.get(conn.nanoid);
			const isConnected = getConnectedState(conn.nanoid);

			if ((existing && existing.connected) || isConnected) {
				return {
					...existing,
					...conn,
					databaseCount: conn.databaseCount || existing?.databaseCount || 0,
					tableCount: conn.tableCount || existing?.tableCount || 0,
					updatedAt: conn.updatedAt || existing?.updatedAt,
					connected: true,
					loading: existing?.loading || false,
					progress: existing?.progress || 0,
					tablePanes: existing?.tablePanes || [],
				};
			}

			return {
				...conn,
				connected: false,
				loading: false,
				databaseCount: conn.databaseCount || 0,
				tableCount: conn.tableCount || 0,
				progress: 0,
				tablePanes: [],
			};
		});
	}, [getConnectedState]);

	const showLoadConnectionsError = useCallback((error: unknown) => {
		toast.error({
			title: t.tools.database.toasts.loadFailedTitle,
			message:
				error instanceof Error
					? error.message
					: t.tools.database.toasts.networkError,
		});
	}, [
		t.tools.database.toasts.loadFailedTitle,
		t.tools.database.toasts.networkError,
	]);

	// 加载所有连接
	const loadConnections = useCallback(async () => {
		try {
			const nextConnections = await fetchConnectionList();
			setConnections((prev) => mergeConnections(prev, nextConnections));
		} catch (error) {
			showLoadConnectionsError(error);
		}
	}, [fetchConnectionList, mergeConnections, showLoadConnectionsError]);

	useEffect(() => {
		if (!nanoid && !isSettingsView) {
			let cancelled = false;

			const loadInitialConnections = async () => {
				try {
					const nextConnections = await fetchConnectionList();
					if (!cancelled) {
						setConnections((prev) => mergeConnections(prev, nextConnections));
					}
				} catch (error) {
					if (!cancelled) {
						showLoadConnectionsError(error);
					}
				}
			};

			void loadInitialConnections();

			return () => {
				cancelled = true;
			};
		}
	}, [fetchConnectionList, isSettingsView, mergeConnections, nanoid, showLoadConnectionsError]);

	// 更新连接状态
	const updateConnection = useCallback(
		(nanoid: string, updates: Partial<DatabaseConnection>) => {
			setConnections((prev) => {
				return prev.map((conn) => {
					if (conn.nanoid === nanoid) {
						const updated = { ...conn, ...updates };
						// 同步更新 sessionStorage
						if (updates.connected !== undefined) {
							setConnectedState(nanoid, updates.connected);
						}
						return updated;
					}
					return conn;
				});
			});
		},
		[setConnectedState]
	);

	// 打开添加/编辑对话框
	const openDialog = (connection?: DatabaseConnection) => {
		setEditingConnection(connection || null);
		setIsDialogOpen(true);
	};

	// 保存连接后的回调
	const handleConnectionSaved = () => {
		setIsDialogOpen(false);
		setEditingConnection(null);
		loadConnections();
	};

	const handleDeleteConnection = async (connectionId: string) => {
		try {
			const response = await fetchWithAuth(
				`/api/v1/database/connection`,
				{
					method: "DELETE",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ nanoid: connectionId }),
				}
			);
			const data = await response.json();
			if (data.success) {
				toast.success({
					title: t.tools.database.toasts.deleteSuccessTitle,
					message: t.tools.database.toasts.connectionDeleted,
				});
				setDeleteConnectionId(null);
				loadConnections();
			} else {
				toast.error({
					title: t.tools.database.toasts.deleteFailedTitle,
					message:
						data.error || t.tools.database.toasts.deleteConnectionFailed,
				});
			}
		} catch (error) {
			toast.error({
				title: t.tools.database.toasts.deleteFailedTitle,
				message:
					error instanceof Error
						? error.message
						: t.tools.database.toasts.networkError,
			});
		}
	};

	// 过滤连接列表
	const filteredConnections = useMemo(() => {
		if (!searchQuery.trim()) {
			return connections;
		}
		const query = searchQuery.toLowerCase();
		return connections.filter((conn) => {
			return (
				conn.serverName?.toLowerCase().includes(query) ||
				conn.host?.toLowerCase().includes(query) ||
				conn.scheme?.toLowerCase().includes(query) ||
				conn.notes?.toLowerCase().includes(query) ||
				`${conn.host}:${conn.port}`.toLowerCase().includes(query)
			);
		});
	}, [connections, searchQuery]);

	if (nanoid) {
		return <DatabaseDetail nanoid={nanoid} />;
	}

	if (isSettingsView) {
		return (
			<SettingsDialog
				onBack={() => router.push("/tools/database")}
				onSave={({ keywords, customRuleCount }) => {
					toast.success({
						title: t.tools.database.toasts.saveSuccessTitle,
						message: formatTpl(t.tools.database.page.settingsUpdated, {
							keywordCount: keywords.length,
							ruleCount: customRuleCount,
						}),
					});
				}}
			/>
		);
	}

	return (
		<div className="flex-1 overflow-y-auto">
			<div className="flex flex-wrap items-center justify-between gap-3 px-3 md:px-6 py-4">
				<div className="min-w-0 basis-full flex-1 md:basis-auto md:max-w-md">
					<div className="relative">
						<Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
						<Input
							placeholder={t.tools.database.page.searchPlaceholder}
							value={searchQuery}
							onChange={(e) => setSearchQuery(e.target.value)}
							className="pl-9"
						/>
					</div>
				</div>
				<div className="flex items-center gap-2">
					<Button
						variant="outline"
						onClick={() => router.push("/tools/database?view=settings")}
						className="border-border/80 text-foreground/90 hover:bg-primary/10 hover:border-primary/50 hover:text-primary transition-colors"
					>
						<Settings className="w-4 h-4" />
						{t.tools.database.page.configManage}
					</Button>
					<Button onClick={() => openDialog()}>
						<Plus className="w-4 h-4 mr-2" />
						{t.tools.database.page.addConnection}
					</Button>

				</div>
			</div>

			<div className="px-3 md:px-6 pb-4">
				<ConnectionPool
					connections={filteredConnections}
					onConnectionUpdate={updateConnection}
					onEdit={openDialog}
					onConnect={(conn) => {
						// 点击"采集信息"按钮时跳转到子页面
						router.push(`/tools/database?nanoid=${conn.nanoid}`);
					}}
					onDelete={(nanoid) => setDeleteConnectionId(nanoid)}
				/>
			</div>

			<ConnectionDialog
				open={isDialogOpen}
				onOpenChange={setIsDialogOpen}
				connection={editingConnection}
				onSaved={handleConnectionSaved}
			/>
			<AlertDialog open={!!deleteConnectionId} onOpenChange={(open) => !open && setDeleteConnectionId(null)}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>{t.common.delete}</AlertDialogTitle>
						<AlertDialogDescription>{t.tools.database.page.deleteConnectionConfirm}</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>{t.common.cancel}</AlertDialogCancel>
						<AlertDialogAction
							className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
							onClick={() => deleteConnectionId && handleDeleteConnection(deleteConnectionId)}
						>
							{t.common.delete}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</div>
	);
}
