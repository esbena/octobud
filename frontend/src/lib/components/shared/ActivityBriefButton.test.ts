import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { activityBriefRequest, type ActivityBriefEntry } from "$lib/api/activityBriefs";
import {
	activityBriefAutoSummarize,
	activityBriefEnabled,
	activityBriefEntries,
} from "$lib/stores/activityBriefs";
import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";
import ActivityBriefButton from "./ActivityBriefButton.svelte";

vi.mock("$lib/api/activityBriefs", () => ({ activityBriefRequest: vi.fn() }));
const notification = {
	githubId: "selected",
	subjectType: "Issue" as const,
	effectiveSortDate: "2026-11-11T12:00:00Z",
	timelineLastSeenAt: "2026-11-10T12:00:00Z",
	lastArchivedAt: "2026-11-09T12:00:00Z",
};
const entry: ActivityBriefEntry = {
	notificationId: "selected",
	version: notification.effectiveSortDate,
	selection: "past_7_days",
	status: "queued",
	since: "",
	updatedAt: "",
	bullets: [],
};

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	activityBriefEnabled.set(false);
	activityBriefAutoSummarize.set(false);
	activityBriefEntries.set([]);
	getNotificationSettingsStore().setCatchUpSelection("last_read");
});

it.each(["last_read", "last_archive"] as const)(
	"allows all available activity for %s without a checkpoint",
	async (selection) => {
		activityBriefEnabled.set(true);
		getNotificationSettingsStore().setCatchUpSelection(selection);
		vi.mocked(activityBriefRequest).mockResolvedValue({
			enabled: true,
			autoSummarize: false,
			entries: [],
			queued: 1,
			skipped: 0,
		});
		render(ActivityBriefButton, {
			notification: { ...notification, timelineLastSeenAt: null, lastArchivedAt: null },
		});
		const button = screen.getByRole("button", { name: "Summarize activity" });
		expect(button).toBeEnabled();
		await fireEvent.click(button);
		expect(activityBriefRequest).toHaveBeenCalledWith(
			"batch",
			expect.objectContaining({
				notificationId: "selected",
				selection,
				since: "1970-01-01T00:00:00.000Z",
			})
		);
	}
);

it("hides the action until enabled and for unsupported notifications", async () => {
	const { rerender } = render(ActivityBriefButton, { notification });
	expect(screen.queryByRole("button")).not.toBeInTheDocument();
	activityBriefEnabled.set(true);
	await rerender({ notification: { ...notification, subjectType: "Release" } });
	expect(screen.queryByRole("button")).not.toBeInTheDocument();
});

it("shows the individual action for normalized pull-request notifications", () => {
	activityBriefEnabled.set(true);
	render(ActivityBriefButton, {
		notification: { ...notification, subjectType: "pull_request" },
	});
	expect(screen.getByRole("button", { name: "Summarize activity" })).toBeEnabled();
});

it("queues only the selected notification using the current catch-up range without opening the card", async () => {
	activityBriefEnabled.set(true);
	getNotificationSettingsStore().setCatchUpSelection("past_7_days");
	vi.mocked(activityBriefRequest).mockResolvedValue({
		enabled: true,
		autoSummarize: false,
		entries: [entry],
		queued: 1,
		skipped: 0,
	});
	render(ActivityBriefButton, { notification });
	const parentClick = vi.fn();
	const button = screen.getByRole("button", { name: "Summarize activity" });
	button.parentElement?.addEventListener("click", parentClick);
	await fireEvent.click(button);
	expect(parentClick).not.toHaveBeenCalled();
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("batch", {
		notificationId: "selected",
		selection: "past_7_days",
		timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone,
		since: expect.any(String),
	});
	expect(screen.getByRole("button", { name: "Summarizing activity" })).toBeDisabled();
});

it("uses the detail reader's frozen cutoff for an individual summary", async () => {
	activityBriefEnabled.set(true);
	getNotificationSettingsStore().setCatchUpSelection("last_read");
	vi.mocked(activityBriefRequest).mockResolvedValue({
		enabled: true,
		autoSummarize: false,
		entries: [],
		queued: 1,
		skipped: 0,
	});
	render(ActivityBriefButton, {
		notification,
		sinceOverride: "2026-11-09T12:00:00Z",
	});
	await fireEvent.click(screen.getByRole("button", { name: "Summarize activity" }));
	expect(activityBriefRequest).toHaveBeenCalledWith(
		"batch",
		expect.objectContaining({
			notificationId: "selected",
			selection: "last_read",
			since: "2026-11-09T12:00:00Z",
		})
	);
});

it("reuses a ready result instead of regenerating", () => {
	activityBriefEnabled.set(true);
	getNotificationSettingsStore().setCatchUpSelection("past_7_days");
	activityBriefEntries.set([{ ...entry, status: "ready" }]);
	render(ActivityBriefButton, { notification });
	expect(screen.getByRole("button", { name: "Activity already summarized" })).toBeDisabled();
	expect(activityBriefRequest).not.toHaveBeenCalled();
});

it("keeps a ready detail summary actionable so it can be viewed", async () => {
	activityBriefEnabled.set(true);
	getNotificationSettingsStore().setCatchUpSelection("past_7_days");
	activityBriefEntries.set([{ ...entry, status: "ready" }]);
	const onShowReadySummary = vi.fn();
	render(ActivityBriefButton, { notification, onShowReadySummary });
	const button = screen.getByRole("button", { name: "View activity summary" });
	expect(button).toBeEnabled();
	await fireEvent.click(button);
	expect(onShowReadySummary).toHaveBeenCalledExactlyOnceWith();
	expect(activityBriefRequest).not.toHaveBeenCalled();
});

it("reports failures and allows explicit retry", async () => {
	activityBriefEnabled.set(true);
	vi.mocked(activityBriefRequest).mockRejectedValue(new Error("Queue full"));
	render(ActivityBriefButton, { notification });
	await fireEvent.click(screen.getByRole("button", { name: "Summarize activity" }));
	expect(await screen.findByRole("alert")).toHaveTextContent("Queue full");
	expect(screen.getByRole("button", { name: "Summarize activity" })).toBeEnabled();
});
