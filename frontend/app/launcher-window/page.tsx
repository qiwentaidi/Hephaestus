"use client";

import LauncherPage from "@/app/(main)/launcher/page";
import { LauncherWindowTitlebar } from "@/components/LauncherWindowTitlebar";

export default function LauncherWindowPage() {
	return (
		<main className="flex h-screen flex-col bg-background">
			<LauncherWindowTitlebar />
			<section className="min-h-0 flex-1 overflow-y-auto">
				<LauncherPage />
			</section>
		</main>
	);
}
