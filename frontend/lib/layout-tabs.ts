export type LayoutTab = {
	href: string;
	title: string;
};

export const getLayoutRoutePath = (href: string) => href.split("?")[0] || href;

// Routes where query parameters only describe in-page state (selected tab, filters,
// selected AI conversation, finding list pagination, asset category), so they must
// reuse a single tab instead of opening a new one per state change. The single tab
// keeps pointing at the latest state.
export const SINGLE_TAB_ROUTE_PATHS = new Set(["/settings", "/ai/tasks", "/ai/workbench", "/vulnerabilities", "/asset-center"]);

export const isSingleTabRoute = (href: string) =>
	SINGLE_TAB_ROUTE_PATHS.has(getLayoutRoutePath(href));

/**
 * Adds a content tab or refreshes the matching one.
 *
 * Routes listed in SINGLE_TAB_ROUTE_PATHS share one tab per route path: navigating
 * from /ai/workbench?session=a to /ai/workbench?session=b moves that tab to the
 * latest session instead of appending another identical "conversation" tab.
 */
export function mergeLayoutTab(tabs: LayoutTab[], href: string, title: string): LayoutTab[] {
	const singleTabRoute = isSingleTabRoute(href);
	const sameTab = (tab: LayoutTab) =>
		tab.href === href ||
		(singleTabRoute && getLayoutRoutePath(tab.href) === getLayoutRoutePath(href));
	const existingIndex = tabs.findIndex(sameTab);
	if (existingIndex < 0) {
		return [...tabs, { href, title }];
	}
	const existing = tabs[existingIndex];
	if (existing.href === href && existing.title === title && tabs.filter(sameTab).length === 1) {
		return tabs;
	}
	return tabs
		.filter((tab, index) => index === existingIndex || !sameTab(tab))
		.map((tab) => (sameTab(tab) ? { href, title } : tab));
}
