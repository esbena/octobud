import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { get } from "svelte/store";
import { fetchNotifications, fetchRepositoryCounts } from "$lib/api/notifications";
import { fetchViews } from "$lib/api/views";
import { fetchTags } from "$lib/api/tags";
import type { NotificationPage, NotificationView } from "$lib/api/types";
import { createNotificationStore } from "$lib/stores/notificationStore";
import { createPaginationStore } from "$lib/stores/paginationStore";
import { createQueryStore } from "$lib/stores/queryStore";
import { createViewStore } from "$lib/stores/viewStore";
import { createRepositoryFilterStore } from "$lib/stores/repositoryFilterStore";
import { createRepositoryPinsStore } from "$lib/stores/repositoryPinsStore";
import { createActionRefresh } from "./actionRefresh";

vi.mock("$lib/api/notifications", () => ({
	fetchNotifications: vi.fn(),
	fetchRepositoryCounts: vi.fn(),
}));
vi.mock("$lib/api/views", () => ({ fetchViews: vi.fn() }));
vi.mock("$lib/api/tags", () => ({ fetchTags: vi.fn() }));

const initialPage: NotificationPage = { items: [], total: 5, page: 1, pageSize: 30 };
const view: NotificationView = {
	id: "work",
	name: "Work",
	slug: "work",
	query: "in:inbox",
	unreadCount: 2,
};

function setup() {
	const stores = {
		notificationStore: createNotificationStore(initialPage),
		paginationStore: createPaginationStore(1, initialPage),
		queryStore: createQueryStore("in:inbox", "in:inbox"),
		viewStore: createViewStore([view], [], "work"),
		repositoryFilterStore: createRepositoryFilterStore([7]),
		repositoryPinsStore: createRepositoryPinsStore(null),
	};
	return { ...stores, refresh: createActionRefresh(stores) };
}

beforeEach(() => {
	vi.useFakeTimers();
	vi.mocked(fetchNotifications).mockResolvedValue({ ...initialPage, total: 4 });
	vi.mocked(fetchViews).mockResolvedValue([{ ...view, unreadCount: 1 }]);
	vi.mocked(fetchTags).mockResolvedValue([]);
	vi.mocked(fetchRepositoryCounts).mockResolvedValue([]);
});

afterEach(() => {
	vi.useRealTimers();
	vi.resetAllMocks();
});

describe("action refresh", () => {
	it("coalesces a burst and updates list and counts without marking the list loading", async () => {
		const state = setup();
		const first = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(200);
		const second = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(249);
		expect(fetchViews).not.toHaveBeenCalled();
		expect(get(state.paginationStore.isLoading)).toBe(false);
		await vi.advanceTimersByTimeAsync(1);
		await Promise.all([first, second]);
		expect(fetchViews).toHaveBeenCalledTimes(1);
		expect(fetchTags).toHaveBeenCalledTimes(1);
		expect(fetchNotifications).toHaveBeenCalledWith({
			page: 1,
			repositoryIds: [7],
			filters: { query: "in:inbox", filters: [] },
		});
		expect(get(state.viewStore.views)[0].unreadCount).toBe(1);
		expect(get(state.paginationStore.total)).toBe(4);
	});

	it("starts a refresh within one second even if actions continue", async () => {
		const state = setup();
		const pending = [state.refresh.refresh()];
		for (let i = 0; i < 4; i++) {
			await vi.advanceTimersByTimeAsync(200);
			pending.push(state.refresh.refresh());
		}
		expect(fetchViews).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(200);
		await Promise.all(pending);
		expect(fetchViews).toHaveBeenCalledTimes(1);
	});

	it("discards superseded responses and runs only one follow-up after the in-flight work", async () => {
		const state = setup();
		let finishViews: ((views: NotificationView[]) => void) | undefined;
		vi.mocked(fetchViews).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishViews = resolve;
				})
		);
		const first = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(250);
		const second = state.refresh.refresh();
		const third = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(1000);
		expect(fetchViews).toHaveBeenCalledTimes(1);
		expect(fetchNotifications).toHaveBeenCalledTimes(1);
		const setViews = vi.spyOn(state.viewStore, "setViews");
		finishViews?.([{ ...view, unreadCount: 99 }]);
		await vi.advanceTimersByTimeAsync(0);
		await Promise.all([first, second, third]);
		expect(fetchViews).toHaveBeenCalledTimes(2);
		expect(setViews).toHaveBeenCalledTimes(1);
		expect(get(state.viewStore.views)[0].unreadCount).toBe(1);
		expect(get(state.paginationStore.total)).toBe(4);
	});

	it.each(["view", "query", "page", "repositories"])(
		"does not overwrite navigation when the %s changes during a refresh",
		async (changed) => {
			const state = setup();
			let finish: ((page: NotificationPage) => void) | undefined;
			vi.mocked(fetchNotifications).mockImplementationOnce(
				() =>
					new Promise((resolve) => {
						finish = resolve;
					})
			);
			const pending = state.refresh.refresh();
			await vi.advanceTimersByTimeAsync(250);
			if (changed === "view") state.viewStore.selectView("another");
			if (changed === "query") state.queryStore.setQuickQuery("repo:other");
			if (changed === "page") state.paginationStore.setPage(2);
			if (changed === "repositories") state.repositoryFilterStore.setSelectedRepositoryIds([9]);
			finish?.({ ...initialPage, total: 99 });
			await pending;
			expect(get(state.notificationStore.pageData).total).toBe(5);
			expect(get(state.paginationStore.total)).toBe(5);
			expect(get(state.viewStore.views)[0].unreadCount).toBe(1);
		}
	);

	it("waits for all requests to settle on failure and permits a later refresh", async () => {
		const state = setup();
		const error = new Error("Offline");
		const log = vi.spyOn(console, "error").mockImplementation(() => {});
		let finishViews: ((views: NotificationView[]) => void) | undefined;
		vi.mocked(fetchViews).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishViews = resolve;
				})
		);
		vi.mocked(fetchNotifications).mockRejectedValueOnce(error);
		const first = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(250);
		const second = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(250);
		expect(fetchViews).toHaveBeenCalledTimes(1);
		finishViews?.([view]);
		await vi.advanceTimersByTimeAsync(0);
		await Promise.all([first, second]);
		expect(fetchViews).toHaveBeenCalledTimes(2);
		expect(get(state.paginationStore.total)).toBe(4);
		// The superseded response (including its error) is discarded.
		expect(log).not.toHaveBeenCalled();
		log.mockRestore();
	});

	it("logs a current failure without rolling back a successful mutation", async () => {
		const state = setup();
		const error = new Error("Offline");
		const log = vi.spyOn(console, "error").mockImplementation(() => {});
		vi.mocked(fetchNotifications).mockRejectedValueOnce(error);
		const first = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(250);
		await first;
		expect(log).toHaveBeenCalledWith("Failed to refresh notifications after action:", error);
		expect(get(state.notificationStore.pageData)).toEqual(initialPage);
		const second = state.refresh.refresh();
		await vi.advanceTimersByTimeAsync(250);
		await second;
		expect(get(state.paginationStore.total)).toBe(4);
		log.mockRestore();
	});

	it("cancels scheduled work and ignores in-flight results after destruction", async () => {
		const state = setup();
		const pending = state.refresh.refresh();
		state.refresh.destroy();
		await pending;
		await vi.advanceTimersByTimeAsync(1000);
		expect(fetchViews).not.toHaveBeenCalled();

		const running = setup();
		let finish: ((page: NotificationPage) => void) | undefined;
		vi.mocked(fetchNotifications).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finish = resolve;
				})
		);
		const inFlight = running.refresh.refresh();
		await vi.advanceTimersByTimeAsync(250);
		running.refresh.destroy();
		await inFlight;
		finish?.({ ...initialPage, total: 99 });
		await vi.advanceTimersByTimeAsync(0);
		expect(get(running.notificationStore.pageData)).toEqual(initialPage);
	});
});
