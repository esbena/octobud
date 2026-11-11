import { fireEvent, render, screen } from "@testing-library/svelte";
import { describe, expect, it, vi } from "vitest";
import type { NotificationView } from "$lib/api/types";
import ViewsSection from "./ViewsSection.svelte";
import SidebarCollapsed from "./SidebarCollapsed.svelte";

const helper: NotificationView = {
	id: "helper",
	name: "Helper",
	slug: "helper",
	query: "in:inbox",
	showInSidebar: false,
	unreadCount: 42,
};
const visible: NotificationView = {
	id: "visible",
	name: "Visible",
	slug: "visible",
	query: 'in:"Helper"',
	showInSidebar: true,
	unreadCount: 7,
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

describe("sidebar view visibility", () => {
	it("groups hidden views below the main list, with navigation and editing but no badges", async () => {
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
		expect(screen.getByLabelText("7 new notifications")).toBeInTheDocument();
		const more = screen.getByRole("button", { name: "More views" });
		expect(more).toHaveAttribute("aria-expanded", "false");
		await fireEvent.click(more);
		const button = screen.getByRole("button", { name: "Helper" });
		expect(button).toHaveAttribute("aria-pressed", "true");
		expect(screen.queryByLabelText("42 new notifications")).not.toBeInTheDocument();
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
	});

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
