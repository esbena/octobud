import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import { activityBriefRequest, type ActivityBriefEntry } from "$lib/api/activityBriefs";
import {
	activityBriefDays,
	activityBriefEnabled,
	activityBriefEntries,
} from "$lib/stores/activityBriefs";
import ActivityBriefButton from "./ActivityBriefButton.svelte";

vi.mock("$lib/api/activityBriefs", () => ({ activityBriefRequest: vi.fn() }));
const notification = {
	githubId: "selected",
	subjectType: "Issue" as const,
	effectiveSortDate: "2026-11-11T12:00:00Z",
};
const entry: ActivityBriefEntry = {
	notificationId: "selected",
	version: notification.effectiveSortDate,
	days: 7,
	status: "queued",
	since: "",
	updatedAt: "",
	bullets: [],
};

afterEach(() => {
	cleanup();
	vi.clearAllMocks();
	activityBriefEnabled.set(false);
	activityBriefEntries.set([]);
	activityBriefDays.set(3);
});

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

it("queues only the selected notification using the current lookback without opening the card", async () => {
	activityBriefEnabled.set(true);
	activityBriefDays.set(7);
	vi.mocked(activityBriefRequest).mockResolvedValue({
		enabled: true,
		entries: [entry],
		queued: 1,
		defaultDays: 3,
	});
	render(ActivityBriefButton, { notification });
	const parentClick = vi.fn();
	const button = screen.getByRole("button", { name: "Summarize activity" });
	button.parentElement?.addEventListener("click", parentClick);
	await fireEvent.click(button);
	expect(parentClick).not.toHaveBeenCalled();
	expect(activityBriefRequest).toHaveBeenCalledExactlyOnceWith("batch", {
		notificationId: "selected",
		days: 7,
	});
	expect(screen.getByRole("button", { name: "Summarizing activity" })).toBeDisabled();
});

it("reuses a ready result instead of regenerating", () => {
	activityBriefEnabled.set(true);
	activityBriefDays.set(7);
	activityBriefEntries.set([{ ...entry, status: "ready" }]);
	render(ActivityBriefButton, { notification });
	expect(screen.getByRole("button", { name: "Activity already summarized" })).toBeDisabled();
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
