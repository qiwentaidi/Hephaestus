import { fetchWithAuth } from "@/lib/api";

export type LauncherItem = {
	name: string;
	type: string;
	path: string;
	target: string;
	favicon: string;
	jdk: string;
};

export type LauncherGroup = {
	name: string;
	children: LauncherItem[];
};

export type LauncherJDK = {
	name: string;
	path: string;
};

export async function getLauncherConfig(): Promise<{ groups: LauncherGroup[]; jdks: LauncherJDK[] }> {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/config", { cache: "no-store" });
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "加载启动器配置失败");
	}
	return {
		groups: payload?.data?.groups || [],
		jdks: payload?.data?.jdks || [],
	};
}

export async function saveLauncherNavigation(groups: LauncherGroup[]) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/navigation", {
		method: "PUT",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(groups),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "保存启动器分组失败");
	}
}

export async function saveLauncherJDKs(jdks: LauncherJDK[]) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/jdks", {
		method: "PUT",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(jdks),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "保存 JDK 配置失败");
	}
}

export async function runLauncherItem(item: LauncherItem) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/run", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(item),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "启动失败");
	}
}

export async function openLauncherFolder(path: string) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/open-folder", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ path }),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "打开目录失败");
	}
}

export async function openLauncherTerminal(path: string) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/open-terminal", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ path }),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "打开终端失败");
	}
}

export async function selectLauncherPath(options: { directory?: boolean; icon?: boolean } = {}) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/select-path", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(options),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "选择路径失败");
	}
	return String(payload?.data?.path || "");
}

export async function generateLauncherIcon(kind: "file" | "url", value: string) {
	const response = await fetchWithAuth("/api/v1/desktop/launcher/generate-icon", {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ kind, value }),
	});
	const payload = await response.json();
	if (!payload?.success) {
		throw new Error(payload?.error || "生成图标失败");
	}
	return String(payload?.data?.icon || "");
}
