import { get } from "svelte/store";
import { afterEach, expect, it, vi } from "vitest";
import {
	navigationNewTab,
	refreshNavigationSettings,
	setNavigationNewTab,
} from "./navigationSettings";

afterEach(() => {
	vi.unstubAllGlobals();
	navigationNewTab.set(null);
});

it("loads and saves the server navigation setting", async () => {
	const fetch = vi
		.fn()
		.mockResolvedValueOnce(new Response(JSON.stringify({ newTab: false })))
		.mockResolvedValueOnce(new Response(JSON.stringify({ newTab: true })));
	vi.stubGlobal("fetch", fetch);
	await refreshNavigationSettings();
	expect(get(navigationNewTab)).toBe(false);
	await setNavigationNewTab(true);
	expect(get(navigationNewTab)).toBe(true);
	expect(fetch).toHaveBeenLastCalledWith(
		"/api/navigation-settings",
		expect.objectContaining({
			method: "PUT",
			body: '{"newTab":true}',
		})
	);
});

it("does not change the setting when saving fails", async () => {
	navigationNewTab.set(false);
	vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 500 })));
	await expect(setNavigationNewTab(true)).rejects.toThrow("500");
	expect(get(navigationNewTab)).toBe(false);
});

it("does not let an older read overwrite a newer save", async () => {
	let finishRead!: (value: Response) => void;
	const read = new Promise<Response>((resolve) => {
		finishRead = resolve;
	});
	vi.stubGlobal(
		"fetch",
		vi
			.fn()
			.mockReturnValueOnce(read)
			.mockResolvedValueOnce(new Response(JSON.stringify({ newTab: true })))
	);
	const refreshing = refreshNavigationSettings();
	await setNavigationNewTab(true);
	finishRead(new Response(JSON.stringify({ newTab: false })));
	await refreshing;
	expect(get(navigationNewTab)).toBe(true);
});
