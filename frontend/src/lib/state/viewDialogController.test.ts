import { afterEach, describe, expect, it, vi } from "vitest";
import { get, writable } from "svelte/store";
import type { NotificationView } from "$lib/api/types";
import { createViewDialogController } from "./viewDialogController";

const helper: NotificationView = {
	id: "helper",
	name: "Helper",
	slug: "helper",
	query: "in:inbox",
	showInSidebar: false,
	isDefault: true,
	unreadCount: 0,
};

function setup() {
	const views = writable<NotificationView[]>([helper]);
	const navigateToSlug = vi.fn().mockResolvedValue(undefined);
	const controller = createViewDialogController({
		views,
		setViews: views.set,
		selectedViewId: writable(helper.id),
		invalidateViews: vi.fn().mockResolvedValue(undefined),
		navigateToSlug,
	});
	return { controller, views, navigateToSlug };
}

afterEach(() => vi.unstubAllGlobals());

describe("view dialog visibility state", () => {
	it("preserves visibility when editing either saved or modified queries, and defaults new views to visible", () => {
		const {
			controller: { actions, stores },
		} = setup();
		actions.startEditing(helper);
		expect(get(stores.draft).showInSidebar).toBe(false);
		actions.startEditingWithQuery(helper, "in:archive");
		expect(get(stores.draft)).toMatchObject({ query: "in:archive", showInSidebar: false });
		actions.openNewDialog();
		expect(get(stores.draft).showInSidebar).toBe(true);
		actions.openNewDialogWithQuery("in:archive");
		expect(get(stores.draft)).toMatchObject({ query: "in:archive", showInSidebar: true });
	});

	it.each([
		["create", false],
		["create", true],
		["update", false],
		["update", true],
	] as const)(
		"sends visibility when performing %s with showInSidebar=%s",
		async (operation, showInSidebar) => {
			const {
				controller: { actions, stores },
				views,
				navigateToSlug,
			} = setup();
			if (operation === "update") {
				actions.startEditing(helper);
			} else {
				views.set([]);
				actions.openNewDialog();
			}
			const saved = { ...helper, showInSidebar };
			const fetchMock = vi
				.fn<typeof fetch>()
				.mockResolvedValue(
					new Response(JSON.stringify({ view: saved }), {
						status: operation === "create" ? 201 : 200,
					})
				);
			vi.stubGlobal("fetch", fetchMock);
			await actions.handleSave({
				name: helper.name,
				description: "",
				icon: "x",
				query: helper.query,
				showInSidebar,
			});
			expect(get(stores.error)).toBeNull();
			expect(fetchMock).toHaveBeenCalledTimes(1);
			const [url, options] = fetchMock.mock.calls[0];
			expect(url).toBe(operation === "create" ? "/api/views" : "/api/views/helper");
			expect(options?.method).toBe(operation === "create" ? "POST" : "PUT");
			expect(JSON.parse(String(options?.body))).toMatchObject({ showInSidebar });
			expect(get(views).find((view) => view.id === helper.id)?.showInSidebar).toBe(showInSidebar);
			expect(navigateToSlug).toHaveBeenCalledWith("helper", true);
		}
	);
});
