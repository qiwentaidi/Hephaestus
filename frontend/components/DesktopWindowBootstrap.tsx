"use client";

import { useEffect } from "react";
import { getApiBaseURL, getDesktopApiHeaders, hasDesktopApiToken } from "@/lib/env";

export function DesktopWindowBootstrap() {
	useEffect(() => {
		if (!hasDesktopApiToken()) {
			return;
		}

		const pathname = window.location.pathname;
		if (pathname === "/settings-window" || pathname === "/launcher-window") {
			return;
		}

		const availWidth = window.screen?.availWidth ?? 0;
		const availHeight = window.screen?.availHeight ?? 0;
		if (availWidth <= 0 || availHeight <= 0) {
			return;
		}

		void fetch(`${getApiBaseURL()}/api/v1/desktop/window/fit`, {
			method: "POST",
			cache: "no-store",
			headers: {
				"Content-Type": "application/json",
				...getDesktopApiHeaders(),
			},
			body: JSON.stringify({
				availWidth,
				availHeight,
			}),
		}).catch(() => {
			// Ignore fit failures and keep the current window state.
		});
	}, []);

	return null;
}
