import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import PreferencesSection from "./PreferencesSection.svelte";
import { navigationNewTab } from "$lib/stores/navigationSettings";

const mocks = vi.hoisted(() => {
	const readable = <T>(value: T) => ({
		subscribe(run: (value: T) => void) {
			run(value);
			return () => {};
		},
	});
	return {
		themeStore: { theme: readable("dark"), toggle: vi.fn() },
		settingsStore: {
			timelineAutoScroll: readable(false),
			hoverActionsEnabled: readable(true),
			showTotalCounts: readable(false),
			builtInViewVisibility: readable({
				inbox: true,
				starred: true,
				snoozed: true,
				archive: true,
				everything: true,
			}),
			whatsNewDefault: readable(false),
			whatsNewBaseline: readable("last_read"),
			setTimelineAutoScroll: vi.fn(),
			setHoverActionsEnabled: vi.fn(),
			setShowTotalCounts: vi.fn(),
			setBuiltInViewVisible: vi.fn(),
			setWhatsNewDefault: vi.fn(),
			setWhatsNewBaseline: vi.fn(),
		},
		toastStore: { success: vi.fn(), error: vi.fn() },
	};
});

vi.mock("$lib/stores/themeStore", () => ({
	getThemeStore: () => mocks.themeStore,
}));
vi.mock("$lib/stores/notificationSettings", () => ({
	getNotificationSettingsStore: () => mocks.settingsStore,
}));
vi.mock("$lib/stores/toastStore", () => ({
	toastStore: mocks.toastStore,
}));

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	vi.unstubAllGlobals();
	navigationNewTab.set(null);
});

it("saves the menu bar navigation preference", async () => {
	navigationNewTab.set(false);
	const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ newTab: true })));
	vi.stubGlobal("fetch", fetch);
	render(PreferencesSection);
	const toggle = screen.getByRole("checkbox", { name: "Open menu bar links in a new tab" });
	await fireEvent.click(toggle);
	expect(fetch).toHaveBeenCalledWith(
		"/api/navigation-settings",
		expect.objectContaining({ method: "PUT", body: '{"newTab":true}' })
	);
	expect(toggle).toBeChecked();
});

it("restores the menu bar preference when saving fails", async () => {
	navigationNewTab.set(false);
	vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 500 })));
	render(PreferencesSection);
	const toggle = screen.getByRole("checkbox", { name: "Open menu bar links in a new tab" });
	await fireEvent.click(toggle);
	await vi.waitFor(() => expect(mocks.toastStore.error).toHaveBeenCalled());
	expect(toggle).not.toBeChecked();
});

it("saves every offered What's new default range", async () => {
	render(PreferencesSection);
	const select = screen.getByRole("combobox", { name: "What's new default baseline" });

	for (const selection of [
		"today",
		"since_yesterday",
		"past_7_days",
		"my_last_comment",
		"last_read",
		"last_archive",
	]) {
		await fireEvent.change(select, { target: { value: selection } });
	}

	expect(mocks.settingsStore.setWhatsNewBaseline.mock.calls).toEqual([
		["today"],
		["since_yesterday"],
		["past_7_days"],
		["my_last_comment"],
		["last_read"],
		["last_archive"],
	]);
});
