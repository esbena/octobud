import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { writable, get } from "svelte/store";
import ActivityBriefControls from "./ActivityBriefControls.svelte";
import { activityBriefRequest, type ActivityBriefStatus } from "$lib/api/activityBriefs";
import { activityBriefDays, activityBriefEntries } from "$lib/stores/activityBriefs";

vi.mock("$lib/api/activityBriefs", () => ({ activityBriefRequest: vi.fn() }));
const status: ActivityBriefStatus = { enabled: true, entries: [], queued: 0, defaultDays: 3 };

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	vi.useRealTimers();
	activityBriefDays.set(3);
	activityBriefEntries.set([]);
});

it("does not overwrite a newer batch response with a slow older status poll", async () => {
	vi.useFakeTimers();
	setup();
	await vi.advanceTimersByTimeAsync(0);
	const button = screen.getByRole("button", { name: "Summarize 10 more" });
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
				days: 3,
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

it("uses the persisted default lookback without generating summaries", async () => {
	setup({ ...status, defaultDays: 14 });
	await waitFor(() => expect(get(activityBriefDays)).toBe(14));
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("status", undefined);
});

it("only reads cache on mount or horizon change; clicks use the submitted view scope", async () => {
	const pageData = setup();
	const button = await screen.findByRole("button", { name: "Summarize 10 more" });
	await waitFor(() => expect(button).toBeEnabled());
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("status", undefined);
	await fireEvent.change(screen.getByRole("combobox"), { target: { value: "7" } });
	expect(get(activityBriefDays)).toBe(7);
	expect(activityBriefRequest).toHaveBeenCalledTimes(1);
	await fireEvent.click(button);
	await waitFor(() =>
		expect(activityBriefRequest).toHaveBeenCalledWith("batch", {
			query: "in:inbox tags:engaged",
			repositoryIds: [42],
			days: 7,
		})
	);
	pageData.set({ queryScope: { query: "in:inbox author:reader", repositoryIds: [99] } });
	await fireEvent.click(button);
	expect(activityBriefRequest).toHaveBeenLastCalledWith("batch", {
		query: "in:inbox author:reader",
		repositoryIds: [99],
		days: 7,
	});
});

it("surfaces batch errors instead of claiming summaries were generated", async () => {
	setup();
	const button = await screen.findByRole("button", { name: "Summarize 10 more" });
	await waitFor(() => expect(button).toBeEnabled());
	vi.mocked(activityBriefRequest).mockRejectedValueOnce(new Error("Summary queue is full"));
	await fireEvent.click(button);
	expect(await screen.findByRole("alert")).toHaveTextContent("Summary queue is full");
	expect(screen.queryByText(/Queued 10/)).not.toBeInTheDocument();
});

it("shows pending jobs and stops explicitly without another generation request", async () => {
	setup();
	await waitFor(() =>
		expect(screen.getByRole("button", { name: "Summarize 10 more" })).toBeEnabled()
	);
	activityBriefEntries.set([
		{
			notificationId: "1",
			version: "v1",
			days: 3,
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

it("keeps generation controls out of the way until the feature is enabled", async () => {
	setup({ ...status, enabled: false });
	expect(await screen.findByRole("link", { name: "Set up activity highlights" })).toHaveAttribute(
		"href",
		"/settings#copilot"
	);
	expect(screen.queryByRole("button", { name: "Summarize 10 more" })).not.toBeInTheDocument();
	expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("status", undefined);
});
