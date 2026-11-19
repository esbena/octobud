import { get } from "svelte/store";
import { fetchNotifications, fetchRepositoryCounts } from "$lib/api/notifications";
import { fetchViews } from "$lib/api/views";
import { fetchTags } from "$lib/api/tags";
import type { NotificationStore } from "$lib/stores/notificationStore";
import type { PaginationStore } from "$lib/stores/paginationStore";
import type { QueryStore } from "$lib/stores/queryStore";
import type { ViewStore } from "$lib/stores/viewStore";
import type { RepositoryFilterStore } from "$lib/stores/repositoryFilterStore";
import type { RepositoryPinsStore } from "$lib/stores/repositoryPinsStore";
import { sameRepositoryIds } from "$lib/stores/repositoryFilterStore";

interface RefreshStores {
	notificationStore: NotificationStore;
	paginationStore: PaginationStore;
	queryStore: QueryStore;
	viewStore: ViewStore;
	repositoryFilterStore: RepositoryFilterStore;
	repositoryPinsStore: RepositoryPinsStore;
}

export function createActionRefresh(stores: RefreshStores) {
	const {
		notificationStore,
		paginationStore,
		queryStore,
		viewStore,
		repositoryFilterStore,
		repositoryPinsStore,
	} = stores;
	let timer: ReturnType<typeof setTimeout> | null = null;
	let maxTimer: ReturnType<typeof setTimeout> | null = null;
	let running = false;
	let ready = false;
	let destroyed = false;
	let generation = 0;
	const waiters: (() => void)[] = [];

	function settle() {
		for (const resolve of waiters.splice(0)) resolve();
	}

	function clearTimers() {
		if (timer !== null) clearTimeout(timer);
		if (maxTimer !== null) clearTimeout(maxTimer);
		timer = maxTimer = null;
	}

	async function run() {
		clearTimers();
		if (running) {
			ready = true;
			return;
		}
		if (destroyed || waiters.length === 0) return;
		running = true;
		ready = false;
		const revision = generation;
		const viewId = get(viewStore.selectedViewId);
		const query = get(queryStore.quickQuery);
		const page = get(paginationStore.page);
		const repositoryIds = get(repositoryFilterStore.selectedRepositoryIds);
		const pageAtStart = get(notificationStore.pageData);

		try {
			const [notifications, views, tags, repositoryCounts] = await Promise.allSettled([
				fetchNotifications({
					page,
					repositoryIds,
					filters: { query, filters: [] },
				}),
				fetchViews(),
				fetchTags(),
				fetchRepositoryCounts(query, [
					...repositoryIds,
					...get(repositoryPinsStore.pinnedRepositoryIds),
				]),
			]);
			if (destroyed || revision !== generation) return;
			// Manual refresh, undo, or polling may have supplied newer data meanwhile.
			if (pageAtStart !== get(notificationStore.pageData)) {
				void refresh();
				return;
			}
			if (notifications.status === "rejected") throw notifications.reason;
			if (views.status === "rejected") throw views.reason;
			if (tags.status === "rejected") throw tags.reason;
			if (repositoryCounts.status === "rejected") throw repositoryCounts.reason;
			viewStore.setViews(views.value);
			viewStore.setTags(tags.value);
			// A background refresh must not replace a newly selected view or search.
			if (
				viewId === get(viewStore.selectedViewId) &&
				query === get(queryStore.quickQuery) &&
				page === get(paginationStore.page) &&
				sameRepositoryIds(repositoryIds, get(repositoryFilterStore.selectedRepositoryIds))
			) {
				const items = notifications.value.items.map((item) => {
					const key = item.githubId ?? item.id;
					return notificationStore.hasPendingMarkRead(key) &&
						notificationStore.getNotification(key)?.isRead
						? { ...item, isRead: true }
						: item;
				});
				notificationStore.setPageData({ ...notifications.value, items });
				paginationStore.setTotal(notifications.value.total);
				paginationStore.setPageSize(notifications.value.pageSize);
				repositoryFilterStore.setRepositoryCounts(repositoryCounts.value, query);
			}
		} catch (error) {
			console.error("Failed to refresh notifications after action:", error);
		} finally {
			running = false;
			if (revision === generation) {
				settle();
			}
			if (ready && !destroyed) void run();
		}
	}

	function refresh(): Promise<void> {
		if (destroyed) return Promise.resolve();
		generation++;
		if (timer !== null) clearTimeout(timer);
		timer = setTimeout(() => void run(), 250);
		maxTimer ??= setTimeout(() => void run(), 1000);
		return new Promise<void>((resolve) => waiters.push(resolve));
	}

	return {
		refresh,
		destroy: () => {
			destroyed = true;
			generation++;
			clearTimers();
			settle();
		},
	};
}
