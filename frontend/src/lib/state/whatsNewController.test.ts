import { afterEach, describe, expect, it, vi } from "vitest";
import { get } from "svelte/store";
import { fetchWhatsNew } from "$lib/api/whatsNew";
import { createWhatsNewController } from "./whatsNewController";
import type { WhatsNewSnapshot } from "$lib/types/whatsNew";

vi.mock("$lib/api/whatsNew", () => ({ fetchWhatsNew: vi.fn() }));
const fetchMock = vi.mocked(fetchWhatsNew);
const cutoff = "2026-11-10T12:00:00Z";
const openInput = (githubId: string, cutoff: string | null) => ({
	githubId,
	cutoff,
	selection: "last_read" as const,
	readerUsername: null,
});

function snapshot(id: string): WhatsNewSnapshot {
	return {
		notificationId: id,
		title: "",
		since: cutoff,
		fetchedAt: cutoff,
		totalEntries: 0,
		newActivity: [],
		olderContext: [],
		warnings: [],
	};
}
afterEach(() => vi.restoreAllMocks());

describe("What's new controller", () => {
	it("does not fetch or fabricate a cutoff for a first visit, including retry", async () => {
		fetchMock.mockClear();
		const controller = createWhatsNewController();
		await controller.actions.open(openInput("1", null));
		await controller.actions.retry();
		expect(fetchMock).not.toHaveBeenCalled();
		expect(get(controller.stores.status)).toBe("no_baseline");
		expect(get(controller.stores.model)).toBeNull();
	});

	it("retries the frozen cutoff, not a mutated caller input", async () => {
		vi.spyOn(console, "error").mockImplementation(() => {});
		fetchMock
			.mockRejectedValueOnce(new Error("Incomplete history"))
			.mockResolvedValueOnce(snapshot("1"));
		const input = openInput("1", cutoff);
		const controller = createWhatsNewController();
		await controller.actions.open(input);
		expect(get(controller.stores.status)).toBe("error");
		expect(get(controller.stores.error)).toBe("Incomplete history");
		input.cutoff = "2026-11-12T12:00:00Z";
		await controller.actions.retry();
		expect(fetchMock).toHaveBeenLastCalledWith(
			"1",
			cutoff,
			expect.any(AbortSignal),
			undefined,
			"last_read"
		);
		expect(get(controller.stores.status)).toBe("ready");
	});

	it("cancels stale loads and ignores their completion after navigation or reset", async () => {
		let resolveFirst!: (result: WhatsNewSnapshot) => void;
		fetchMock
			.mockImplementationOnce(
				() =>
					new Promise((resolve) => {
						resolveFirst = resolve;
					})
			)
			.mockResolvedValueOnce(snapshot("2"));
		const controller = createWhatsNewController();
		const first = controller.actions.open(openInput("1", cutoff));
		const signal = fetchMock.mock.lastCall?.[2];
		await controller.actions.open(openInput("2", cutoff));
		expect(signal?.aborted).toBe(true);
		resolveFirst(snapshot("1"));
		await first;
		expect(get(controller.stores.status)).toBe("ready");
		controller.actions.reset();
		expect(get(controller.stores.model)).toBeNull();
		expect(get(controller.stores.status)).toBe("no_baseline");
	});

	it("rejects mismatched response identity and cutoff", async () => {
		vi.spyOn(console, "error").mockImplementation(() => {});
		fetchMock.mockResolvedValueOnce(snapshot("other"));
		const controller = createWhatsNewController();
		await controller.actions.open(openInput("1", cutoff));
		expect(get(controller.stores.status)).toBe("error");
		expect(get(controller.stores.model)).toBeNull();
		fetchMock.mockResolvedValueOnce({ ...snapshot("1"), since: "2026-11-09T12:00:00Z" });
		await controller.actions.retry();
		expect(get(controller.stores.status)).toBe("error");
	});

	it("rejects a response with a different cutoff in the same millisecond", async () => {
		vi.spyOn(console, "error").mockImplementation(() => {});
		fetchMock.mockResolvedValueOnce({
			...snapshot("1"),
			since: "2026-11-10T12:00:00.123456788Z",
		});
		const controller = createWhatsNewController();
		await controller.actions.open(openInput("1", "2026-11-10T12:00:00.123456789Z"));
		expect(get(controller.stores.status)).toBe("error");
		expect(get(controller.stores.model)).toBeNull();
	});
});
