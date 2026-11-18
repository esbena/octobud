import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import PreferencesSection from "./PreferencesSection.svelte";

const mocks = vi.hoisted(() => {
	const readable = (value: string | boolean) => ({
		subscribe(run: (value: string | boolean) => void) {
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
			whatsNewDefault: readable(false),
			whatsNewBaseline: readable("last_read"),
			setTimelineAutoScroll: vi.fn(),
			setHoverActionsEnabled: vi.fn(),
			setShowTotalCounts: vi.fn(),
			setWhatsNewDefault: vi.fn(),
			setWhatsNewBaseline: vi.fn(),
		},
		toastStore: { success: vi.fn() },
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
