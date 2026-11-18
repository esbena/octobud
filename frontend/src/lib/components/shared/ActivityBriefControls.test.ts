import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { writable, get } from "svelte/store";
import ActivityBriefControls from "./ActivityBriefControls.svelte";
import { activityBriefRequest, type ActivityBriefStatus } from "$lib/api/activityBriefs";
import { activityBriefEntries } from "$lib/stores/activityBriefs";
import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";

vi.mock("$lib/api/activityBriefs", () => ({ activityBriefRequest: vi.fn() }));
const status: ActivityBriefStatus = {
	enabled: true,
	autoSummarize: false,
	entries: [],
	queued: 0,
	skipped: 0,
};

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	vi.useRealTimers();
	getNotificationSettingsStore().setCatchUpSelection("last_read");
	activityBriefEntries.set([]);
});

it("does not overwrite a newer batch response with a slow older status poll", async () => {
	vi.useFakeTimers();
	setup();
	await vi.advanceTimersByTimeAsync(0);
	const button = screen.getByRole("button", { name: "Summarize next batch" });
	expect(button).toBeEnabled();
	let completePoll!: (value: ActivityBriefStatus) => void;
	vi.mocked(activityBriefRequest).mockImplementationOnce(
		() =>
			new Promise((resolve) => {
				completePoll = resolve;
			})
	);
	await vi.advanceTimersByTimeAsync(2500);
	vi.mocked(activityBriefRequest).mockResolvedValueOnce({
		...status,
		queued: 1,
		entries: [
			{
				notificationId: "new",
				version: "v1",
				selection: "last_read",
				since: "",
				updatedAt: "",
				status: "queued",
				bullets: [],
			},
		],
	});
	await fireEvent.click(button);
	expect(get(activityBriefEntries)).toHaveLength(1);
	completePoll(status);
	await Promise.resolve();
	expect(get(activityBriefEntries)).toHaveLength(1);
});

function setup(initialStatus: ActivityBriefStatus = status) {
	vi.mocked(activityBriefRequest).mockResolvedValue(initialStatus);
	const pageData = writable({
		queryScope: { query: "in:inbox tags:engaged", repositoryIds: [42] },
	});
	render(ActivityBriefControls, {
		context: new Map([
			["notificationPageController", { stores: { pageData, isLoading: writable(false) } }],
		]),
	});
	return pageData;
}

it("uses the shared catch-up selection without generating summaries", async () => {
	setup();
	expect(get(getNotificationSettingsStore().catchUpSelection)).toBe("last_read");
	expect(screen.getByRole("group", { name: "Activity window" })).toBeInTheDocument();
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("status", undefined);
});

it("only reads cache on mount; clicks use the selected range and submitted view scope", async () => {
	const pageData = setup();
	const button = await screen.findByRole("button", { name: "Summarize next batch" });
	await waitFor(() => expect(button).toBeEnabled());
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("status", undefined);
	await fireEvent.click(screen.getByRole("button", { name: "Past 7 days" }));
	expect(get(getNotificationSettingsStore().catchUpSelection)).toBe("past_7_days");
	expect(activityBriefRequest).toHaveBeenCalledTimes(1);
	await fireEvent.click(button);
	await waitFor(() =>
		expect(activityBriefRequest).toHaveBeenCalledWith("batch", {
			query: "in:inbox tags:engaged",
			repositoryIds: [42],
			selection: "past_7_days",
			timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
		})
	);
	pageData.set({ queryScope: { query: "in:inbox author:reader", repositoryIds: [99] } });
	await fireEvent.click(button);
	expect(activityBriefRequest).toHaveBeenLastCalledWith("batch", {
		query: "in:inbox author:reader",
		repositoryIds: [99],
		selection: "past_7_days",
		timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
	});
});

it("surfaces batch errors instead of claiming summaries were generated", async () => {
	setup();
	const button = await screen.findByRole("button", { name: "Summarize next batch" });
	await waitFor(() => expect(button).toBeEnabled());
	vi.mocked(activityBriefRequest).mockRejectedValueOnce(new Error("Summary queue is full"));
	await fireEvent.click(button);
	expect(await screen.findByRole("alert")).toHaveTextContent("Summary queue is full");
	expect(screen.queryByText(/Queued 10/)).not.toBeInTheDocument();
});

it("shows pending jobs and stops explicitly without another generation request", async () => {
	setup();
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Summarize next batch" })).toBeEnabled()
	);
	activityBriefEntries.set([
		{
			notificationId: "1",
			version: "v1",
			selection: "last_read",
			since: "",
			updatedAt: "",
			status: "running",
			bullets: [],
		},
	]);
	await fireEvent.click(await screen.findByRole("button", { name: "Stop" }));
	expect(activityBriefRequest).toHaveBeenLastCalledWith("stop", undefined);
	expect(screen.getByText("Stopped pending summaries.")).toBeInTheDocument();
});

it("keeps the global look-back selector available when summaries are disabled", async () => {
	setup({ ...status, enabled: false });
	expect(await screen.findByRole("link", { name: "Set up Copilot summaries" })).toHaveAttribute(
		"href",
		"/settings#copilot"
	);
	expect(screen.queryByRole("button", { name: "Summarize next batch" })).not.toBeInTheDocument();
	expect(screen.getByRole("group", { name: "Activity window" })).toBeInTheDocument();
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("status", undefined);
});
