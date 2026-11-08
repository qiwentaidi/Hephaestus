"use client";

import { useEffect } from "react";
import { hasDesktopApiToken } from "@/lib/env";
import { openDesktopExternalURL, shouldUseDesktopExternalNavigation } from "@/lib/desktop";

export function DesktopExternalLinkBridge() {
	useEffect(() => {
		if (!hasDesktopApiToken()) {
			return;
		}

		const originalOpen = window.open.bind(window);

		const handleClick = (event: MouseEvent) => {
			if (event.defaultPrevented || event.button !== 0) {
				return;
			}

			const target = event.target;
			if (!(target instanceof Element)) {
				return;
			}

			const anchor = target.closest("a[href]");
			if (!(anchor instanceof HTMLAnchorElement) || anchor.hasAttribute("download")) {
				return;
			}

			const href = anchor.getAttribute("href");
			if (!href || !shouldUseDesktopExternalNavigation(href, anchor.target)) {
				return;
			}

			event.preventDefault();
			void openDesktopExternalURL(href);
		};

		window.open = function patchedWindowOpen(url?: string | URL, target?: string, features?: string) {
			if (url && shouldUseDesktopExternalNavigation(url, target)) {
				void openDesktopExternalURL(url);
				return window;
			}
			return originalOpen(url as string | URL | undefined, target, features);
		};

		document.addEventListener("click", handleClick, true);
		return () => {
			document.removeEventListener("click", handleClick, true);
			window.open = originalOpen;
		};
	}, []);

	return null;
}
