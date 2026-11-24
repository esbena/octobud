import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/svelte";
import { tick } from "svelte";
import { getVersion } from "$lib/api/user";
import WhatsNewBanner from "./WhatsNewBanner.svelte";

vi.mock("$lib/api/user", () => ({
	getVersion: vi.fn(),
}));

vi.mock("svelte/transition", () => ({
	slide: () => undefined,
}));

const STORAGE_KEY = "octobud:whats-new-last-seen";
const storage = new Map<string, string>();

beforeEach(() => {
	storage.clear();
	vi.stubGlobal("localStorage", {
		getItem: (key: string) => storage.get(key) ?? null,
		setItem: (key: string, value: string) => storage.set(key, value),
		removeItem: (key: string) => storage.delete(key),
		clear: () => storage.clear(),
	});
});

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	vi.unstubAllGlobals();
});

describe("WhatsNewBanner", () => {
	it.each([null, "0.3.3"])(
		"does not show or update the baseline for development builds (last seen: %s)",
		async (lastSeen) => {
			if (lastSeen) localStorage.setItem(STORAGE_KEY, lastSeen);
			vi.mocked(getVersion).mockResolvedValue({
				version: "0.3.4-56-g9479bd0",
				isDevelopment: true,
			});

			render(WhatsNewBanner);

			await waitFor(() => expect(getVersion).toHaveBeenCalledOnce());
			await tick();
			expect(screen.queryByText(/Octobud updated to/)).not.toBeInTheDocument();
			expect(localStorage.getItem(STORAGE_KEY)).toBe(lastSeen);
		}
	);

	it("continues to show the banner for a release build", async () => {
		vi.mocked(getVersion).mockResolvedValue({ version: "0.4.0" });

		render(WhatsNewBanner);

		expect(await screen.findByText("Octobud updated to 0.4.0")).toBeInTheDocument();
	});
});
