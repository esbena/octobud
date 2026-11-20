import { writable } from "svelte/store";

export const navigationNewTab = writable<boolean | null>(null);

let revision = 0;
let readRevision = 0;

async function requestSettings(init?: RequestInit): Promise<boolean> {
	const response = await fetch("/api/navigation-settings", init);
	if (!response.ok) {
		throw new Error(`Navigation settings request failed (${response.status})`);
	}
	const data: unknown = await response.json();
	if (
		typeof data !== "object" ||
		data === null ||
		!("newTab" in data) ||
		typeof data.newTab !== "boolean"
	) {
		throw new Error("Invalid navigation settings response");
	}
	return data.newTab;
}

export async function refreshNavigationSettings(): Promise<void> {
	const current = revision;
	const read = ++readRevision;
	const newTab = await requestSettings({ cache: "no-store" });
	if (revision === current && readRevision === read) navigationNewTab.set(newTab);
}

export async function setNavigationNewTab(newTab: boolean): Promise<void> {
	const current = ++revision;
	const saved = await requestSettings({
		method: "PUT",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ newTab }),
	});
	if (revision === current) {
		revision++;
		navigationNewTab.set(saved);
	}
}
