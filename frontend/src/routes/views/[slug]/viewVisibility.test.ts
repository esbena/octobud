import { afterEach, describe, expect, it, vi } from "vitest";
import { get } from "svelte/store";
import { trace, INVALID_SPAN_CONTEXT } from "@opentelemetry/api";
import { load } from "./+page";
import { BUILT_IN_VIEWS } from "$lib/state/types";
import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";
import { fetchNotifications, fetchRepositoryCounts } from "$lib/api/notifications";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/api/notifications", () => ({
	fetchNotifications: vi.fn(),
	fetchRepositoryCounts: vi.fn(),
}));

afterEach(() => {
	for (const { slug } of Object.values(BUILT_IN_VIEWS)) {
		getNotificationSettingsStore().setBuiltInViewVisible(slug, true);
	}
	vi.clearAllMocks();
});

describe("direct navigation to hidden built-ins", () => {
	it.each(Object.values(BUILT_IN_VIEWS))(
		"loads $name contents without restoring sidebar visibility",
		async (view) => {
			const settings = getNotificationSettingsStore();
			for (const { slug } of Object.values(BUILT_IN_VIEWS)) {
				settings.setBuiltInViewVisible(slug, false);
			}
			const page = { items: [], total: 0, page: 1, pageSize: 50 };
			vi.mocked(fetchNotifications).mockResolvedValue(page);
			vi.mocked(fetchRepositoryCounts).mockResolvedValue([]);
			const fetch = vi.fn();
			const span = trace.wrapSpanContext(INVALID_SPAN_CONTEXT);
			const result = await load({
				tracing: { enabled: false, root: span, current: span },
				data: null,
				route: { id: "/views/[slug]" },
				setHeaders: vi.fn(),
				depends: vi.fn(),
				untrack: (fn) => fn(),
				fetch,
				params: { slug: view.slug },
				url: new URL(`http://localhost/views/${view.slug}`),
				parent: async () => ({
					views: Object.values(BUILT_IN_VIEWS).map((builtIn) => ({
						...builtIn,
						systemView: true,
						unreadCount: 0,
					})),
					tags: [],
					apiError: null,
				}),
			});

			expect(result).toMatchObject({
				selectedViewSlug: view.slug,
				initialPage: page,
				viewQuery: view.query,
				apiError: null,
			});
			expect(fetchNotifications).toHaveBeenCalledWith(
				expect.objectContaining({ filters: { query: view.query, filters: [] } }),
				fetch
			);
			expect(Object.values(get(settings.builtInViewVisibility))).not.toContain(true);
			expect(localStorage.getItem(`octobud:sidebar:builtInViewVisible:${view.slug}`)).toBe("false");
		}
	);
});
