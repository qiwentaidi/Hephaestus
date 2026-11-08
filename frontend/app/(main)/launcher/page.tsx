"use client";

import { useEffect, useMemo, useState } from "react";
import Image from "next/image";
import {
	Delete,
	Edit,
	FolderOpen,
	ImagePlus,
	Info,
	Plus,
	Search,
	Terminal,
	Trash2,
	Waypoints,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
	ContextMenu,
	ContextMenuContent,
	ContextMenuItem,
	ContextMenuLabel,
	ContextMenuSeparator,
	ContextMenuSub,
	ContextMenuSubContent,
	ContextMenuSubTrigger,
	ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import toast from "@/lib/toast";
import { useI18n } from "@/lib/i18n";
import {
	generateLauncherIcon,
	getLauncherConfig,
	openLauncherFolder,
	openLauncherTerminal,
	runLauncherItem,
	saveLauncherNavigation,
	selectLauncherPath,
	type LauncherGroup,
	type LauncherItem,
	type LauncherJDK,
} from "@/lib/api/launcher";

type LauncherItemForm = LauncherItem & {
	groupName: string;
};

type LauncherIconDescriptor = {
	src: string;
	alt: string;
	className?: string;
};

const launcherTypeOptions = [
	{ value: "CMD", label: "CMD" },
	{ value: "APP", label: "APP" },
	{ value: "JAR", label: "JAR" },
	{ value: "Link", label: "Link" },
];

const emptyItemForm: LauncherItemForm = {
	groupName: "",
	name: "",
	type: "CMD",
	path: "",
	target: "",
	favicon: "",
	jdk: "",
};

function getDefaultIcon(itemType: string): LauncherIconDescriptor {
	switch (itemType) {
		case "JAR":
			return { src: "/launcher-icons/java.svg", alt: "Java", className: "scale-[1.08]" };
		case "APP":
			return { src: "/launcher-icons/app.svg", alt: "App" };
		case "Link":
			return { src: "/launcher-icons/chrome.svg", alt: "Link" };
		case "CMD":
			return { src: "/launcher-icons/iterm.svg", alt: "Terminal" };
		default:
			return { src: "/launcher-icons/iterm.svg", alt: "Terminal" };
	}
}

function inferNameFromPath(path: string) {
	if (!path) return "";
	const parts = path.split(/[\\/]/);
	return parts[parts.length - 1] || path;
}

function formatLauncherText(template: string, values: Record<string, string>) {
	return Object.entries(values).reduce((result, [key, value]) => result.replace(`{${key}}`, value), template);
}

export default function LauncherPage() {
	const { t } = useI18n();
	const i18n = t.launcher;
	const [groups, setGroups] = useState<LauncherGroup[]>([]);
	const [jdks, setJdks] = useState<LauncherJDK[]>([]);
	const [keyword, setKeyword] = useState("");
	const [itemDialogOpen, setItemDialogOpen] = useState(false);
	const [groupDialogOpen, setGroupDialogOpen] = useState(false);
	const [tipsDialogOpen, setTipsDialogOpen] = useState(false);
	const [isEditing, setIsEditing] = useState(false);
	const [editingRef, setEditingRef] = useState<{ groupName: string; itemPath: string; itemName: string } | null>(null);
	const [itemForm, setItemForm] = useState<LauncherItemForm>(emptyItemForm);
	const [groupName, setGroupName] = useState("");

	useEffect(() => {
		let cancelled = false;

		const loadLauncherConfig = async (showError = false) => {
			try {
				const payload = await getLauncherConfig();
				if (!cancelled) {
					setGroups(payload.groups);
					setJdks(payload.jdks);
				}
			} catch (error: any) {
				if (!cancelled && showError) {
					toast.error(error?.message || i18n.messages.loadFailed);
				}
			}
		};

		void loadLauncherConfig(true);

		if (typeof window !== "undefined") {
			const handleLauncherUpdated = () => {
				void loadLauncherConfig();
			};
			window.addEventListener("hephaestus:launcher-updated", handleLauncherUpdated);
			return () => {
				cancelled = true;
				window.removeEventListener("hephaestus:launcher-updated", handleLauncherUpdated);
			};
		}

		return () => {
			cancelled = true;
		};
	}, [i18n.messages.loadFailed]);

	const filteredGroups = useMemo(() => {
		if (!keyword.trim()) {
			return groups;
		}
		const lowerKeyword = keyword.trim().toLowerCase();
		return groups
			.map((group) => ({
				...group,
				children: (group.children || []).filter((item) =>
					item.name.toLowerCase().includes(lowerKeyword) ||
					item.path.toLowerCase().includes(lowerKeyword),
				),
			}))
			.filter((group) => group.children.length > 0 || group.name.toLowerCase().includes(lowerKeyword));
	}, [groups, keyword]);

	const persistGroups = async (nextGroups: LauncherGroup[]) => {
		setGroups(nextGroups);
		await saveLauncherNavigation(nextGroups);
	};

	const openGroupDialog = () => {
		setGroupName("");
		setGroupDialogOpen(true);
	};

	const createGroup = async () => {
		const normalized = groupName.trim();
		if (!normalized) {
			toast.error(i18n.messages.groupNameRequired);
			return;
		}
		if (groups.some((group) => group.name === normalized)) {
			toast.error(i18n.messages.groupNameExists);
			return;
		}
		try {
			await persistGroups([...groups, { name: normalized, children: [] }]);
			setGroupDialogOpen(false);
			setGroupName("");
			toast.success(i18n.messages.groupAdded);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.addGroupFailed);
		}
	};

	const deleteGroup = async (groupName: string) => {
		if (!window.confirm(formatLauncherText(i18n.confirm.deleteGroup, { name: groupName }))) return;
		try {
			await persistGroups(groups.filter((group) => group.name !== groupName));
			toast.success(i18n.messages.groupDeleted);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.deleteGroupFailed);
		}
	};

	const openAddItemDialog = (groupName: string) => {
		setIsEditing(false);
		setEditingRef(null);
		setItemForm({ ...emptyItemForm, groupName });
		setItemDialogOpen(true);
	};

	const openEditItemDialog = (groupName: string, item: LauncherItem) => {
		setIsEditing(true);
		setEditingRef({ groupName, itemPath: item.path, itemName: item.name });
		setItemForm({ groupName, ...item });
		setItemDialogOpen(true);
	};

	const resetItemDialog = () => {
		setItemDialogOpen(false);
		setIsEditing(false);
		setEditingRef(null);
		setItemForm(emptyItemForm);
	};

	const saveItem = async () => {
		if (!itemForm.groupName || !itemForm.name.trim() || !itemForm.path.trim()) {
			toast.error(i18n.messages.itemRequired);
			return;
		}

		let nextIcon = itemForm.favicon;
		if (!nextIcon) {
			try {
				nextIcon = itemForm.type === "Link"
					? await generateLauncherIcon("url", itemForm.path)
					: await generateLauncherIcon("file", itemForm.path);
			} catch {
				nextIcon = "";
			}
		}

		const nextItem: LauncherItem = {
			name: itemForm.name.trim(),
			type: itemForm.type,
			path: itemForm.path.trim(),
			target: itemForm.target.trim(),
			favicon: nextIcon,
			jdk: itemForm.jdk.trim(),
		};

		try {
			const nextGroups = groups.map((group) => {
				if (group.name !== itemForm.groupName) {
					return group;
				}
				if (!isEditing || !editingRef) {
					return { ...group, children: [...(group.children || []), nextItem] };
				}
				return {
					...group,
					children: (group.children || []).map((item) =>
						item.path === editingRef.itemPath && item.name === editingRef.itemName ? nextItem : item,
					),
				};
			});
			await persistGroups(nextGroups);
			resetItemDialog();
			toast.success(isEditing ? i18n.messages.itemUpdated : i18n.messages.itemAdded);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.saveItemFailed);
		}
	};

	const deleteItem = async (groupName: string, item: LauncherItem) => {
		if (!window.confirm(formatLauncherText(i18n.confirm.deleteItem, { name: item.name }))) return;
		try {
			const nextGroups = groups.map((group) => {
				if (group.name !== groupName) {
					return group;
				}
				return {
					...group,
					children: (group.children || []).filter((child) => !(child.name === item.name && child.path === item.path)),
				};
			});
			await persistGroups(nextGroups);
			toast.success(i18n.messages.itemDeleted);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.deleteItemFailed);
		}
	};

	const moveItem = async (fromGroupName: string, toGroupName: string, item: LauncherItem) => {
		if (fromGroupName === toGroupName) return;
		try {
			let movedItem: LauncherItem | null = null;
			const nextGroups = groups.map((group) => {
				if (group.name === fromGroupName) {
					const filtered = (group.children || []).filter((child) => {
						const matched = child.name === item.name && child.path === item.path;
						if (matched) {
							movedItem = child;
						}
						return !matched;
					});
					return { ...group, children: filtered };
				}
				if (group.name === toGroupName && movedItem) {
					return { ...group, children: [...(group.children || []), movedItem] };
				}
				return group;
			});

			const fallbackGroups = movedItem && !nextGroups.some((group) => group.name === toGroupName && group.children.some((child) => child.name === item.name && child.path === item.path))
				? nextGroups.map((group) => group.name === toGroupName ? { ...group, children: [...(group.children || []), movedItem as LauncherItem] } : group)
				: nextGroups;

			await persistGroups(fallbackGroups);
			toast.success(i18n.messages.itemMoved);
		} catch (error: any) {
			toast.error(error?.message || i18n.messages.moveItemFailed);
		}
	};

	const renderItemActions = (group: LauncherGroup, item: LauncherItem) => (
		<>
			<ContextMenuItem className="gap-3" onClick={() => void openLauncherFolder(item.path).catch((error: any) => toast.error(error?.message || i18n.messages.openFolderFailed))}>
				<FolderOpen className="h-4 w-4" />
				{i18n.actions.openFolder}
			</ContextMenuItem>
			<ContextMenuItem className="gap-3" onClick={() => void openLauncherTerminal(item.path).catch((error: any) => toast.error(error?.message || i18n.messages.openTerminalFailed))}>
				<Terminal className="h-4 w-4" />
				{i18n.actions.openTerminal}
			</ContextMenuItem>
			<ContextMenuSub>
				<ContextMenuSubTrigger className="gap-3">
					<Waypoints className="h-4 w-4" />
					{i18n.actions.moveTo}
				</ContextMenuSubTrigger>
				<ContextMenuSubContent>
					{groups.map((targetGroup) => (
						<ContextMenuItem key={targetGroup.name} onClick={() => void moveItem(group.name, targetGroup.name, item)}>
							{targetGroup.name}
						</ContextMenuItem>
					))}
				</ContextMenuSubContent>
			</ContextMenuSub>
			<ContextMenuSeparator />
			<ContextMenuItem className="gap-3" onClick={() => openEditItemDialog(group.name, item)}>
				<Edit className="h-4 w-4" />
				{t.common.edit}
			</ContextMenuItem>
			<ContextMenuItem className="gap-3" variant="destructive" onClick={() => void deleteItem(group.name, item)}>
				<Delete className="h-4 w-4" />
				{t.common.delete}
			</ContextMenuItem>
		</>
	);

	return (
		<div className="space-y-3 p-4">
			<div className="flex flex-wrap items-center gap-2">
				<Button variant="outline" className="h-10 rounded-md px-4 text-[15px]" onClick={() => setTipsDialogOpen(true)}>
					<Info className="mr-2 h-4 w-4" />
					{i18n.dialogs.tipsTitle}
				</Button>
				<div className="relative min-w-[300px] flex-1">
					<Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
					<Input
						value={keyword}
						onChange={(event) => setKeyword(event.target.value)}
						placeholder={i18n.placeholders.search}
						className="h-10 rounded-md border-border/70 bg-background/80 pl-9 text-[15px]"
					/>
				</div>
				<Button variant="outline" className="h-10 rounded-md px-4 text-[15px]" onClick={openGroupDialog}>
					<Plus className="mr-2 h-4 w-4" />
					{i18n.actions.addGroup}
				</Button>
			</div>

			<div className="space-y-2">
				{filteredGroups.length > 0 ? filteredGroups.map((group) => (
					<ContextMenu key={group.name}>
						<ContextMenuTrigger asChild>
							<Card
								className="gap-0 rounded-md border-border/70 bg-card/95 py-0 shadow-none transition-colors"
								data-file-drop-target=""
								data-group={group.name}
							>
								<CardHeader className="flex flex-row items-center justify-between space-y-0 px-3 pb-0 pt-3">
									<CardTitle className="text-[16px] font-bold">{group.name}</CardTitle>
									<div className="flex items-center gap-1">
										<Button variant="ghost" size="sm" className="h-8 px-2 text-muted-foreground hover:text-foreground" onClick={() => openAddItemDialog(group.name)}>
											<Plus className="h-4 w-4" />
										</Button>
										<Button variant="ghost" size="icon" onClick={() => void deleteGroup(group.name)}>
											<Trash2 className="h-4 w-4" />
										</Button>
									</div>
								</CardHeader>
								<CardContent className="px-3 pb-3 pt-2">
									{group.children?.length ? (
										<div className="grid grid-cols-10 gap-x-2.5 gap-y-2">
											{group.children.map((item) => {
												const icon = getDefaultIcon(item.type);
												return (
													<ContextMenu key={`${group.name}-${item.path}-${item.name}`}>
														<ContextMenuTrigger asChild>
															<button
																type="button"
																className="flex h-[80px] w-full min-w-0 flex-col items-center justify-center rounded-md border border-transparent px-2 py-[5px] text-center transition-[background-color,box-shadow,border-color] duration-150 hover:border-border/60 hover:bg-muted/80 hover:shadow-[0_8px_20px_rgba(15,23,42,0.08)]"
																onClick={() => void runLauncherItem(item).catch((error: any) => toast.error(error?.message || i18n.messages.runFailed))}
																title={item.name}
															>
																<div className="flex h-9 w-9 items-center justify-center overflow-hidden">
																	{item.favicon ? (
																		<Image src={item.favicon} alt={item.name} width={36} height={36} unoptimized className="h-9 w-9 object-contain" />
																	) : (
																		<Image src={icon.src} alt={icon.alt} width={36} height={36} unoptimized className={cn("h-9 w-9 object-contain", icon.className)} />
																	)}
																</div>
																<span className="mt-2.5 block max-w-[80px] truncate text-center text-[12px] leading-4 text-foreground">{item.name}</span>
															</button>
														</ContextMenuTrigger>
														<ContextMenuContent className="w-56">
															<ContextMenuLabel>{item.name}</ContextMenuLabel>
															<ContextMenuSeparator />
															{renderItemActions(group, item)}
														</ContextMenuContent>
													</ContextMenu>
												);
											})}
										</div>
									) : (
										<div className="rounded-md border border-dashed border-border/60 bg-background/40 p-8 text-center text-sm text-muted-foreground">
											{i18n.empty.dropFiles}
										</div>
									)}
								</CardContent>
							</Card>
						</ContextMenuTrigger>
						<ContextMenuContent className="w-56">
							<ContextMenuLabel>{group.name}</ContextMenuLabel>
							<ContextMenuSeparator />
							<ContextMenuItem onClick={() => openAddItemDialog(group.name)}>
								<Plus className="h-4 w-4" />
								{i18n.actions.addItem}
							</ContextMenuItem>
							<ContextMenuItem onClick={openGroupDialog}>
								<Plus className="h-4 w-4" />
								{i18n.actions.addGroup}
							</ContextMenuItem>
							<ContextMenuSeparator />
							<ContextMenuItem variant="destructive" onClick={() => void deleteGroup(group.name)}>
								<Trash2 className="h-4 w-4" />
								{t.common.delete}
							</ContextMenuItem>
						</ContextMenuContent>
					</ContextMenu>
				)) : (
					<div className="rounded-xl border border-dashed border-border/60 bg-background/40 p-10 text-center text-sm text-muted-foreground">
						{i18n.empty.noMatches}
					</div>
				)}
			</div>

			<Dialog open={itemDialogOpen} onOpenChange={(open) => !open && resetItemDialog()}>
				<DialogContent className="sm:max-w-[680px]">
					<DialogHeader>
						<DialogTitle>{isEditing ? i18n.dialogs.editItemTitle : i18n.dialogs.addItemTitle}</DialogTitle>
					</DialogHeader>
					<div className="grid gap-4 py-2">
						<div className="grid gap-2">
							<Label>{i18n.labels.groupName}</Label>
							<Select value={itemForm.groupName} onValueChange={(value) => setItemForm((prev) => ({ ...prev, groupName: value }))}>
								<SelectTrigger>
									<SelectValue placeholder={i18n.placeholders.selectGroup} />
								</SelectTrigger>
								<SelectContent>
									{groups.map((group) => (
										<SelectItem key={group.name} value={group.name}>{group.name}</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-2">
							<Label>{i18n.labels.name}</Label>
							<Input value={itemForm.name} onChange={(event) => setItemForm((prev) => ({ ...prev, name: event.target.value }))} />
						</div>
						<div className="grid gap-2">
							<Label>{i18n.labels.type}</Label>
							<Select value={itemForm.type} onValueChange={(value) => setItemForm((prev) => ({ ...prev, type: value }))}>
								<SelectTrigger>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									{launcherTypeOptions.map((option) => (
										<SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
									))}
								</SelectContent>
							</Select>
						</div>
						<div className="grid gap-2">
							<Label>{i18n.labels.path}</Label>
							<div className="flex gap-2">
								<Input
									value={itemForm.path}
									onChange={(event) => setItemForm((prev) => ({
										...prev,
										path: event.target.value,
										name: prev.name || inferNameFromPath(event.target.value),
									}))}
								/>
								<Button
									type="button"
									variant="secondary"
									onClick={() => void selectLauncherPath().then((path) => {
										if (!path) return;
										setItemForm((prev) => ({
											...prev,
											path,
											name: prev.name || inferNameFromPath(path),
										}));
									}).catch((error: any) => toast.error(error?.message || i18n.messages.selectPathFailed))}
								>
									{i18n.actions.choose}
								</Button>
							</div>
						</div>
						{itemForm.type === "JAR" ? (
							<div className="grid gap-2">
								<Label>{i18n.labels.jdk}</Label>
								<Select value={itemForm.jdk || "__default__"} onValueChange={(value) => setItemForm((prev) => ({ ...prev, jdk: value === "__default__" ? "" : value }))}>
									<SelectTrigger>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value="__default__">{i18n.options.defaultJava}</SelectItem>
										{jdks.map((jdk) => (
											<SelectItem key={jdk.name} value={jdk.path}>{jdk.name}</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
						) : null}
						{itemForm.type === "CMD" ? (
							<div className="grid gap-2">
								<Label>{i18n.labels.command}</Label>
								<Textarea rows={4} value={itemForm.target} onChange={(event) => setItemForm((prev) => ({ ...prev, target: event.target.value }))} placeholder={i18n.placeholders.command} />
							</div>
						) : null}
						<div className="grid gap-2">
							<Label>{i18n.labels.icon}</Label>
							<div className="flex gap-2">
								<Input value={itemForm.favicon} onChange={(event) => setItemForm((prev) => ({ ...prev, favicon: event.target.value }))} placeholder={i18n.placeholders.icon} />
								<Button
									type="button"
									variant="secondary"
									onClick={() => void selectLauncherPath({ icon: true }).then(async (path) => {
										if (!path) return;
										const icon = await generateLauncherIcon("file", path);
										setItemForm((prev) => ({ ...prev, favicon: icon }));
									}).catch((error: any) => toast.error(error?.message || i18n.messages.selectIconFailed))}
								>
									<ImagePlus className="mr-2 h-4 w-4" />
									{i18n.actions.icon}
								</Button>
							</div>
						</div>
					</div>
					<DialogFooter>
						<Button variant="secondary" onClick={resetItemDialog}>{t.common.cancel}</Button>
						<Button onClick={() => void saveItem()}>{t.common.save}</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<Dialog open={groupDialogOpen} onOpenChange={setGroupDialogOpen}>
				<DialogContent className="sm:max-w-[460px]">
					<DialogHeader>
						<DialogTitle>{i18n.dialogs.addGroupTitle}</DialogTitle>
					</DialogHeader>
					<div className="grid gap-2 py-2">
						<Label htmlFor="launcher-group-name">{i18n.labels.groupNameFull}</Label>
						<Input
							id="launcher-group-name"
							value={groupName}
							onChange={(event) => setGroupName(event.target.value)}
							onKeyDown={(event) => {
								if (event.key === "Enter") {
									event.preventDefault();
									void createGroup();
								}
							}}
							placeholder={i18n.placeholders.groupName}
							autoFocus
						/>
					</div>
					<DialogFooter>
						<Button variant="secondary" onClick={() => setGroupDialogOpen(false)}>{t.common.cancel}</Button>
						<Button onClick={() => void createGroup()}>{t.common.save}</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<Dialog open={tipsDialogOpen} onOpenChange={setTipsDialogOpen}>
				<DialogContent className="sm:max-w-[920px]">
					<DialogHeader>
						<DialogTitle>{i18n.dialogs.tipsTitle}</DialogTitle>
					</DialogHeader>
					<ScrollArea className="max-h-[60vh] pr-4">
						<div className="space-y-4 text-sm leading-7 text-muted-foreground">
							<p>1. {i18n.tips.jar}</p>
							<p>2. {i18n.tips.customCommand}</p>
							<div className="rounded-lg border border-border/60 bg-background/60 p-3 font-mono text-xs text-foreground">
								java -javaagent:%path% -jar %path%
							</div>
							<p>3. {i18n.tips.dragFiles}</p>
							<p>4. {i18n.tips.contextMenu}</p>
							<p>5. {i18n.tips.link}</p>
							<p>6. {i18n.tips.icon}</p>
						</div>
					</ScrollArea>
					<DialogFooter>
						<Button onClick={() => setTipsDialogOpen(false)}>{i18n.actions.gotIt}</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	);
}
