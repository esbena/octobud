import { fireEvent, render, screen } from "@testing-library/svelte";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { NotificationView } from "$lib/api/types";
import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";
import ViewsSection from "./ViewsSection.svelte";
import SidebarCollapsed from "./SidebarCollapsed.svelte";
import SidebarHover from "./SidebarHover.svelte";

vi.mock("svelte/transition", () => ({ fly: () => undefined }));

const helper: NotificationView = {
	id: "helper",
	name: "Helper",
	slug: "helper",
	query: "in:inbox",
	showInSidebar: false,
	unreadCount: 42,
	totalCount: 64,
};
const visible: NotificationView = {
	id: "visible",
	name: "Visible",
	slug: "visible",
	query: 'in:"Helper"',
	showInSidebar: true,
	unreadCount: 7,
	totalCount: 12,
};
const legacy: NotificationView = {
	id: "legacy",
	name: "Legacy",
	slug: "legacy",
	query: "in:inbox",
	unreadCount: 0,
};

function sharedProps() {
	return {
		builtInViewList: [],
		sortedUserViews: [helper, visible, legacy],
		selectedViewId: helper.id,
		selectedViewSlug: helper.slug,
		inboxView: undefined,
		isSettingsRoute: false,
		viewsCollapsed: false,
		onSelectView: vi.fn(),
		onToggleViewsCollapsed: vi.fn(),
	};
}

afterEach(() => getNotificationSettingsStore().setShowTotalCounts(false));

describe("sidebar view visibility", () => {
	it.each([false, true])(
		"keeps hover helper views hidden without badges (totals %s)",
		async (showTotals) => {
			getNotificationSettingsStore().setShowTotalCounts(showTotals);
			const props = {
				...sharedProps(),
				userViews: [helper, visible, legacy],
				reorderMode: false,
				reordering: false,
				draggedOverViewId: null,
				onNewView: vi.fn(),
				onEditView: vi.fn(),
				onToggleReorderMode: vi.fn(),
				onDragStart: vi.fn(),
				onDragOver: vi.fn(),
				onDrop: vi.fn(),
				onDragEnd: vi.fn(),
				onSelectTag: vi.fn(),
				onToggleTagsCollapsed: vi.fn(),
				onNewTag: vi.fn(),
				onEditTag: vi.fn(),
				onToggleTagReorderMode: vi.fn(),
				onTagDragStart: vi.fn(),
				onTagDragOver: vi.fn(),
				onTagDrop: vi.fn(),
				onTagDragEnd: vi.fn(),
				onMouseEnter: vi.fn(),
				onMouseLeave: vi.fn(),
			};
			render(SidebarHover, props);
			expect(screen.queryByRole("button", { name: "Helper" })).not.toBeInTheDocument();
			expect(screen.getByRole("button", { name: "Visible" })).toBeInTheDocument();
			await fireEvent.click(screen.getByRole("button", { name: "More views" }));
			expect(screen.queryByLabelText("42 new notifications")).not.toBeInTheDocument();
			expect(screen.queryByLabelText("0 unread, 64 total")).not.toBeInTheDocument();
			expect(
				screen.getByLabelText(showTotals ? "7 unread, 12 total" : "7 new notifications")
			).toBeInTheDocument();
			await fireEvent.click(screen.getByRole("button", { name: "Helper" }));
			expect(props.onSelectView).toHaveBeenCalledWith("helper");
		}
	);

	it.each([false, true])(
		"groups hidden views with navigation and editing but no badges (totals %s)",
		async (showTotals) => {
			getNotificationSettingsStore().setShowTotalCounts(showTotals);
			const props = {
				...sharedProps(),
				userViews: [helper, visible, legacy],
				reorderMode: false,
				reordering: false,
				draggedOverViewId: null,
				onNewView: vi.fn(),
				onEditView: vi.fn(),
				onToggleReorderMode: vi.fn(),
				onDragStart: vi.fn(),
				onDragOver: vi.fn(),
				onDrop: vi.fn(),
				onDragEnd: vi.fn(),
			};
			const { rerender } = render(ViewsSection, props);
			expect(screen.queryByRole("button", { name: "Helper" })).not.toBeInTheDocument();
			expect(screen.getByRole("button", { name: "Visible" })).toBeInTheDocument();
			expect(screen.getByRole("button", { name: "Legacy" })).toBeInTheDocument();
			expect(
				screen.getByLabelText(showTotals ? "7 unread, 12 total" : "7 new notifications")
			).toBeInTheDocument();
			const more = screen.getByRole("button", { name: "More views" });
			expect(more).toHaveAttribute("aria-expanded", "false");
			await fireEvent.click(more);
			const button = screen.getByRole("button", { name: "Helper" });
			expect(button).toHaveAttribute("aria-pressed", "true");
			expect(screen.queryByLabelText("42 new notifications")).not.toBeInTheDocument();
			expect(screen.queryByLabelText("0 unread, 64 total")).not.toBeInTheDocument();
			expect(screen.getAllByRole("button", { name: /^(Visible|Legacy|Helper)$/ })).toEqual([
				screen.getByRole("button", { name: "Visible" }),
				screen.getByRole("button", { name: "Legacy" }),
				button,
			]);
			await fireEvent.click(button);
			expect(props.onSelectView).toHaveBeenCalledWith("helper");
			await fireEvent.mouseEnter(button.parentElement!);
			await fireEvent.click(screen.getByRole("button", { name: "Edit view Helper" }));
			await fireEvent.click(screen.getByRole("button", { name: "Edit view" }));
			expect(props.onEditView).toHaveBeenCalledWith(helper);

			await rerender({ reorderMode: true });
			const item = button.closest("li")!;
			expect(item).toHaveAttribute("draggable", "true");
			await fireEvent.dragStart(item, { dataTransfer: { setData: vi.fn() } });
			expect(props.onDragStart).toHaveBeenCalledWith("helper");
			await fireEvent.click(more);
			expect(screen.queryByRole("button", { name: "Helper" })).not.toBeInTheDocument();
		}
	);

	it("keeps hidden icons behind More views and omits their unread dots", async () => {
		const props = { ...sharedProps(), onSelectTag: vi.fn(), onToggleTagsCollapsed: vi.fn() };
		render(SidebarCollapsed, props);
		expect(screen.queryByRole("button", { name: "Helper" })).not.toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Visible" })).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Legacy" })).toBeInTheDocument();
		expect(screen.getByLabelText("7 unread")).toBeInTheDocument();
		await fireEvent.click(screen.getByRole("button", { name: "More views" }));
		expect(screen.queryByLabelText("42 unread")).not.toBeInTheDocument();
		await fireEvent.click(screen.getByRole("button", { name: "Helper" }));
		expect(props.onSelectView).toHaveBeenCalledWith("helper");
	});
});
