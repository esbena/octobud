import { beforeEach, afterEach, describe, expect, it, vi } from "vitest";
import { render, cleanup, fireEvent } from "@testing-library/svelte";
import NotificationCounts from "./NotificationCounts.svelte";
import BuiltInViewCard from "./BuiltInViewCard.svelte";
import ViewCard from "./ViewCard.svelte";
import TagCard from "./TagCard.svelte";
import SidebarCollapsed from "./SidebarCollapsed.svelte";
import PreferencesSection from "../settings/PreferencesSection.svelte";
import { tick } from "svelte";
import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";

const settings = getNotificationSettingsStore();
const view = {
	id: "1",
	slug: "reviews",
	name: "Reviews",
	query: "reason:review_requested",
	unreadCount: 4,
	totalCount: 12,
};

beforeEach(() => settings.setShowTotalCounts(false));
afterEach(cleanup);
vi.mock("$app/environment", () => ({ browser: true }));

describe("sidebar notification counts", () => {
	it("preserves unread-only badges by default", () => {
		const { container } = render(NotificationCounts, { unread: 4, total: 12 });
		const body = container.innerHTML;
		expect(body).toContain('aria-label="4 new notifications"');
		expect(body).not.toContain("12");
		expect(render(NotificationCounts, { unread: 0, total: 12 }).container.innerHTML).not.toContain(
			"aria-label"
		);
	});

	it.each([
		[4, 12],
		[0, 12],
		[1234, 56789],
	])("shows %i unread and %i total when enabled", (unread, total) => {
		settings.setShowTotalCounts(true);
		const body = render(NotificationCounts, { unread, total }).container.innerHTML;
		expect(body).toContain(`title="${unread} unread, ${total} total"`);
		expect(body).toContain(`aria-label="${unread} unread, ${total} total"`);
		expect(body).toContain("font-semibold");
		expect(body).toContain("font-normal text-gray-600 dark:text-gray-400");
		expect(render(NotificationCounts, { unread, total }).container.textContent).toMatch(
			new RegExp(`${unread}\\s*/\\s*${total}`)
		);
	});

	it("does not invent totals when none were returned", () => {
		settings.setShowTotalCounts(true);
		const body = render(NotificationCounts, { unread: 4 }).container.innerHTML;
		expect(body).toContain('aria-label="4 new notifications"');
		expect(body).not.toContain("total");
	});

	it("hides empty views while retaining read-but-nonempty counts", () => {
		settings.setShowTotalCounts(true);
		expect(render(NotificationCounts, { unread: 0, total: 0 }).container.textContent).toBe("");
		expect(render(NotificationCounts, { total: 0 }).container.textContent).toBe("");
		expect(render(NotificationCounts, { unread: 0, total: 12 }).container.textContent).toMatch(
			/0\s*\/\s*12/
		);
	});

	it("uses both counts in built-in, custom and tag cards shared by expanded and hover layouts", () => {
		settings.setShowTotalCounts(true);
		expect(
			render(BuiltInViewCard, { view, icon: "inbox", count: 4 }).container.innerHTML
		).toContain("4 unread, 12 total");
		expect(render(ViewCard, { view, count: view.unreadCount }).container.innerHTML).toContain(
			"4 unread, 12 total"
		);
		expect(render(TagCard, { tag: { ...view, unreadCount: 0 } }).container.innerHTML).toContain(
			"0 unread, 12 total"
		);
	});

	it("does not add built-in unread badges when disabled", () => {
		expect(
			render(BuiltInViewCard, { view: { ...view, slug: "starred" }, icon: "star" }).container
				.innerHTML
		).not.toContain("new notifications");
	});

	it.each([4, 0])("shows Starred counts with %i unread when enabled", async (unreadCount) => {
		const { container, getByLabelText } = render(BuiltInViewCard, {
			view: { ...view, slug: "starred", unreadCount },
			icon: "star",
		});
		expect(container.querySelector("[title]")).toBeNull();
		settings.setShowTotalCounts(true);
		await tick();
		expect(getByLabelText(`${unreadCount} unread, 12 total`)).toHaveTextContent(
			new RegExp(`${unreadCount}\\s*/\\s*12`)
		);
	});

	it.each(["archive", "everything", "snoozed"])(
		"keeps %s number-free with totals enabled",
		(slug) => {
			settings.setShowTotalCounts(true);
			const { container } = render(BuiltInViewCard, {
				view: { ...view, slug },
				icon: slug === "archive" ? "archive" : "infinity",
			});
			expect(container.textContent?.trim()).toBe("Reviews");
			expect(container.querySelector("[title]")).toBeNull();
		}
	);

	it("updates counts immediately when toggled in Preferences", async () => {
		const { getByRole } = render(PreferencesSection);
		const { container } = render(NotificationCounts, { unread: 0, total: 12 });
		expect(container.textContent).not.toContain("12");
		await fireEvent.click(getByRole("checkbox", { name: "Show total counts" }));
		expect(container.textContent).toContain("12");
		expect(localStorage.getItem("octobud:sidebar:showTotalCounts")).toBe("true");
		await fireEvent.click(getByRole("checkbox", { name: "Show total counts" }));
		expect(container.textContent).not.toContain("12");
	});

	it("reserves stable right-aligned badge space while showing hover actions", async () => {
		settings.setShowTotalCounts(true);
		const custom = render(ViewCard, { view, count: view.unreadCount });
		const customBadge = custom.container.querySelector('[title="4 unread, 12 total"]')!;
		expect(customBadge.parentElement).toHaveClass("mr-2");
		await fireEvent.mouseEnter(custom.getByRole("group"));
		expect(customBadge.parentElement).toHaveClass("invisible");
		expect(custom.getByRole("button", { name: "Edit view Reviews" })).toBeVisible();
		await fireEvent.mouseLeave(custom.getByRole("group"));
		expect(customBadge.parentElement).not.toHaveClass("invisible");
		const tag = render(TagCard, { tag: { ...view, id: "tag-1" } });
		const tagBadge = tag.container.querySelector('[title="4 unread, 12 total"]')!;
		expect(tagBadge.parentElement).toHaveClass("mr-2");
		await fireEvent.mouseEnter(tag.container.querySelector('[role="group"]')!);
		expect(tagBadge.parentElement).toHaveClass("invisible");
	});

	it.each([false, true])(
		"reserves edit-button space without a badge (totals %s)",
		async (enabled) => {
			settings.setShowTotalCounts(enabled);
			const custom = render(ViewCard, {
				view: { ...view, unreadCount: 0, totalCount: 0 },
				count: 0,
			});
			const tag = render(TagCard, { tag: { ...view, unreadCount: 0, totalCount: 0 } });
			for (const card of [custom, tag]) {
				const reservedSpace = card.container.querySelector(".min-w-6");
				expect(reservedSpace).not.toBeNull();
				expect(reservedSpace).toHaveTextContent("");
				await fireEvent.mouseEnter(card.container.querySelector('[role="group"]')!);
				expect(reservedSpace).toHaveClass("min-w-6", "invisible");
			}
		}
	);

	it("updates collapsed-sidebar tooltips without replacing the unread indicators", async () => {
		const { getByRole } = render(SidebarCollapsed, {
			builtInViewList: [
				{ ...view, slug: "inbox" },
				{ ...view, id: "archive", slug: "archive", name: "Archive" },
				{ ...view, id: "everything", slug: "everything", name: "Everything" },
				{ ...view, id: "starred", slug: "starred", name: "Starred" },
				{ ...view, id: "snoozed", slug: "snoozed", name: "Snoozed" },
			],
			sortedUserViews: [],
			sortedTags: [],
			selectedViewId: "",
			selectedViewSlug: "",
			inboxView: undefined,
			isSettingsRoute: false,
			viewsCollapsed: true,
			onSelectView: vi.fn(),
			onToggleViewsCollapsed: vi.fn(),
			onSelectTag: vi.fn(),
			onToggleTagsCollapsed: vi.fn(),
		});
		expect(getByRole("button", { name: "Reviews" })).toHaveAttribute("title", "Reviews");
		settings.setShowTotalCounts(true);
		await tick();
		expect(getByRole("button", { name: "Reviews: 4 unread, 12 total" })).toHaveAttribute(
			"title",
			"Reviews: 4 unread, 12 total"
		);
		expect(getByRole("button", { name: "Archive" })).toHaveAttribute("title", "Archive");
		expect(getByRole("button", { name: "Everything" })).toHaveAttribute("title", "Everything");
		expect(getByRole("button", { name: "Snoozed" })).toHaveAttribute("title", "Snoozed");
		expect(getByRole("button", { name: "Starred: 4 unread, 12 total" })).toHaveAttribute(
			"title",
			"Starred: 4 unread, 12 total"
		);
	});
});
