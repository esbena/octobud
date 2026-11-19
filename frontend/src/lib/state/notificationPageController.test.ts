import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { get } from "svelte/store";
import {
	archiveNotification,
	fetchNotifications,
	fetchRepositoryCounts,
	markNotificationRead,
} from "$lib/api/notifications";
import { fetchViews } from "$lib/api/views";
import { fetchTags } from "$lib/api/tags";
import type { Notification, NotificationPage, NotificationView } from "$lib/api/types";
import type { PageData } from "../../routes/views/[slug]/$types";
import { createNotificationPageController } from "./notificationPageController";
import type { NotificationPageController } from "./types";

vi.mock("$lib/api/notifications", async (importOriginal) => ({
	...(await importOriginal<typeof import("$lib/api/notifications")>()),
	archiveNotification: vi.fn(),
	markNotificationRead: vi.fn(),
	fetchNotifications: vi.fn(),
	fetchRepositoryCounts: vi.fn(),
}));
vi.mock("$lib/api/views", () => ({ fetchViews: vi.fn() }));
vi.mock("$lib/api/tags", () => ({
	fetchTags: vi.fn(),
	assignTagToNotification: vi.fn(),
	removeTagFromNotification: vi.fn(),
}));
vi.mock("$lib/stores/undoStore", () => ({ undoStore: { pushAction: vi.fn() } }));

function notification(id: string): Notification {
	return {
		id,
		repoFullName: "example/repo",
		subjectTitle: `PR ${id}`,
		subjectType: "PullRequest",
		reason: "review_requested",
		archived: false,
		isRead: false,
		muted: false,
		starred: false,
		updatedAt: "2026-01-01T00:00:00Z",
		labels: [],
		viewIds: [],
		actionHints: { dismissedOn: ["archive"] },
	};
}

const items = ["1", "2", "3", "4"].map(notification);
const view: NotificationView = {
	id: "work",
	slug: "work",
	name: "Work",
	query: 'in:"Work scope" -in:"Low priority"',
	unreadCount: 4,
};
const initialData: PageData = {
	views: [view],
	tags: [],
	selectedViewSlug: "work",
	selectedViewId: "work",
	initialPage: { items, total: 4, page: 1, pageSize: 30 },
	initialPageNumber: 1,
	initialSearchTerm: "",
	initialQuickFilters: [],
	initialQuery: view.query,
	viewQuery: view.query,
	apiError: null,
};

let controller: NotificationPageController;
const invalidate = vi.fn<() => Promise<void>>();
const navigate = vi.fn<() => Promise<void>>();

beforeEach(() => {
	vi.useFakeTimers();
	localStorage.clear();
	vi.mocked(archiveNotification).mockImplementation(async (id) => ({
		...notification(id),
		archived: true,
	}));
	vi.mocked(fetchNotifications).mockResolvedValue({
		...initialData.initialPage,
		items: items.slice(2),
		total: 2,
	});
	vi.mocked(fetchRepositoryCounts).mockResolvedValue([]);
	vi.mocked(fetchViews).mockResolvedValue([{ ...view, unreadCount: 2 }]);
	vi.mocked(fetchTags).mockResolvedValue([]);
	controller = createNotificationPageController(initialData, {
		onRefresh: invalidate,
		onRefreshViewCounts: invalidate,
		navigateToUrl: navigate,
	});
});

afterEach(() => {
	controller.actions.destroy();
	vi.useRealTimers();
	vi.resetAllMocks();
});

describe("notification action refresh wiring", () => {
	it("removes archived rows promptly, shares the recount, and preserves newer focus and detail", async () => {
		controller.stores.splitModeEnabled.set(true);
		controller.stores.keyboardFocusIndex.set(0);
		controller.stores.detailNotificationId.set("1");
		const first = controller.actions.archive(items[0]);
		await vi.advanceTimersByTimeAsync(0);
		expect(get(controller.stores.pageData).items.map((item) => item.id)).toEqual(["2", "3", "4"]);
		expect(get(controller.stores.detailNotificationId)).toBe("2");
		expect(get(controller.stores.isLoading)).toBe(false);
		const second = controller.actions.archive(items[1]);
		await vi.advanceTimersByTimeAsync(0);
		expect(get(controller.stores.pageData).items.map((item) => item.id)).toEqual(["3", "4"]);
		expect(get(controller.stores.detailNotificationId)).toBe("3");
		expect(fetchViews).not.toHaveBeenCalled();

		controller.stores.keyboardFocusIndex.set(1);
		controller.stores.detailNotificationId.set("4");
		await vi.advanceTimersByTimeAsync(250);
		await Promise.all([first, second]);
		expect(fetchViews).toHaveBeenCalledTimes(1);
		expect(fetchTags).toHaveBeenCalledTimes(1);
		expect(fetchNotifications).toHaveBeenCalledTimes(1);
		expect(invalidate).not.toHaveBeenCalled();
		expect(get(controller.stores.keyboardFocusIndex)).toBe(1);
		expect(get(controller.stores.detailNotificationId)).toBe("4");
		expect(navigate).toHaveBeenCalledWith(expect.stringContaining("id=2"), {
			replace: true,
			shallow: true,
		});
	});

	it("does not resurrect a row archived while an older refresh is in flight", async () => {
		let finishViews: ((views: NotificationView[]) => void) | undefined;
		vi.mocked(fetchViews).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishViews = resolve;
				})
		);
		vi.mocked(fetchNotifications).mockResolvedValueOnce({
			...initialData.initialPage,
			items: items.slice(1),
			total: 3,
		});
		const first = controller.actions.archive(items[0]);
		await vi.advanceTimersByTimeAsync(250);
		const second = controller.actions.archive(items[1]);
		await vi.advanceTimersByTimeAsync(0);
		finishViews?.([{ ...view, unreadCount: 3 }]);
		await vi.advanceTimersByTimeAsync(0);
		expect(get(controller.stores.pageData).items.map((item) => item.id)).toEqual(["3", "4"]);
		expect(fetchViews).toHaveBeenCalledTimes(1);
		await vi.advanceTimersByTimeAsync(250);
		await Promise.all([first, second]);
		expect(fetchViews).toHaveBeenCalledTimes(2);
		expect(get(controller.stores.pageData).items.map((item) => item.id)).toEqual(["3", "4"]);
	});

	it("refreshes again after a slow soft-mark-read write completes", async () => {
		let finishWrite: ((item: Notification) => void) | undefined;
		vi.mocked(markNotificationRead).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishWrite = resolve;
				})
		);
		vi.mocked(fetchViews).mockResolvedValueOnce([view]);
		controller.actions.softMarkRead(items[0]);
		await vi.advanceTimersByTimeAsync(250);
		expect(fetchViews).toHaveBeenCalledTimes(1);
		finishWrite?.({ ...items[0], isRead: true });
		await vi.advanceTimersByTimeAsync(250);
		expect(fetchViews).toHaveBeenCalledTimes(2);
		expect(get(controller.stores.views)[0].unreadCount).toBe(2);
	});

	it("does not undo a newer manual refresh when an older action recount finishes", async () => {
		controller.stores.splitModeEnabled.set(true);
		controller.stores.keyboardFocusIndex.set(0);
		controller.stores.detailNotificationId.set("1");
		let finishViews: ((views: NotificationView[]) => void) | undefined;
		vi.mocked(fetchViews).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishViews = resolve;
				})
		);
		const pending = controller.actions.archive(items[0]);
		await vi.advanceTimersByTimeAsync(250);

		// Undo and manual refresh use this separate refresh path.
		vi.mocked(fetchNotifications).mockResolvedValue(initialData.initialPage);
		await controller.actions.refresh();
		expect(get(controller.stores.pageData).items).toEqual(items);
		finishViews?.([{ ...view, unreadCount: 2 }]);
		await vi.advanceTimersByTimeAsync(0);
		expect(get(controller.stores.pageData).items).toEqual(items);
		await vi.advanceTimersByTimeAsync(250);
		await pending;
		expect(get(controller.stores.pageData).items).toEqual(items);
		expect(get(controller.stores.detailNotificationId)).toBe("2");
		expect(get(controller.stores.keyboardFocusIndex)).toBe(1);
	});

	it("preserves an optimistic read while the coalesced page response predates the write", async () => {
		let finishWrite: ((item: Notification) => void) | undefined;
		vi.mocked(markNotificationRead).mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					finishWrite = resolve;
				})
		);
		vi.mocked(fetchNotifications).mockResolvedValue(initialData.initialPage);
		controller.actions.softMarkRead(items[0]);
		await vi.advanceTimersByTimeAsync(250);
		expect(get(controller.stores.pageData).items[0].isRead).toBe(true);
		vi.mocked(fetchNotifications).mockResolvedValue({
			...initialData.initialPage,
			items: [{ ...items[0], isRead: true }, ...items.slice(1)],
		});
		finishWrite?.({ ...items[0], isRead: true });
		await vi.advanceTimersByTimeAsync(250);
		expect(get(controller.stores.pageData).items[0].isRead).toBe(true);
	});

	it("resumes keyboard triage when a burst empties the visible first page before it refills", async () => {
		controller.actions.destroy();
		controller = createNotificationPageController(
			{ ...initialData, initialPage: { ...initialData.initialPage, items: items.slice(0, 2) } },
			{ navigateToUrl: navigate }
		);
		controller.stores.splitModeEnabled.set(true);
		controller.stores.keyboardFocusIndex.set(0);
		controller.stores.detailNotificationId.set("1");
		const first = controller.actions.archive(items[0]);
		await vi.advanceTimersByTimeAsync(0);
		const second = controller.actions.archive(items[1]);
		await vi.advanceTimersByTimeAsync(0);
		expect(get(controller.stores.pageData).items).toEqual([]);
		await vi.advanceTimersByTimeAsync(250);
		await Promise.all([first, second]);
		expect(get(controller.stores.pageData).items.map((item) => item.id)).toEqual(["3", "4"]);
		expect(get(controller.stores.keyboardFocusIndex)).toBe(0);
		expect(get(controller.stores.detailNotificationId)).toBe("3");
		expect(navigate).toHaveBeenLastCalledWith(expect.stringContaining("id=3"), {
			replace: true,
			shallow: true,
		});
	});

	it("loads the previous page when archiving the last item on a later page", async () => {
		controller.actions.destroy();
		controller = createNotificationPageController(
			{
				...initialData,
				initialPageNumber: 2,
				initialPage: { items: [items[0]], total: 31, page: 2, pageSize: 30 },
			},
			{ navigateToUrl: navigate }
		);
		const previousPage: NotificationPage = {
			items: Array.from({ length: 30 }, (_, i) => notification(`previous-${i}`)),
			total: 30,
			page: 1,
			pageSize: 30,
		};
		vi.mocked(fetchNotifications).mockResolvedValue(previousPage);
		controller.stores.keyboardFocusIndex.set(0);
		const pending = controller.actions.archive(items[0]);
		await vi.advanceTimersByTimeAsync(250);
		await pending;
		expect(fetchNotifications).toHaveBeenCalledWith(expect.objectContaining({ page: 1 }));
		expect(get(controller.stores.page)).toBe(1);
		expect(get(controller.stores.keyboardFocusIndex)).toBe(29);
		expect(get(controller.stores.pageData)).toEqual(previousPage);
	});
});
