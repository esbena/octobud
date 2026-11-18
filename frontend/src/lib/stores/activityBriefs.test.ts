import { expect, it } from "vitest";
import { matchingActivityBrief } from "./activityBriefs";
import type { ActivityBriefEntry } from "$lib/api/activityBriefs";

it("only displays results for the same notification, version and catch-up range", () => {
	const entry: ActivityBriefEntry = {
		notificationId: "1",
		version: "2026-11-10T12:00:00.123456Z",
		selection: "past_7_days",
		since: "",
		updatedAt: "",
		status: "ready",
		bullets: [],
	};
	expect(matchingActivityBrief([entry], "1", "2026-11-10T12:00:00.123Z", "past_7_days")).toBe(
		entry
	);
	expect(matchingActivityBrief([entry], "2", entry.version, "past_7_days")).toBeUndefined();
	expect(matchingActivityBrief([entry], "1", entry.version, "today")).toBeUndefined();
	expect(
		matchingActivityBrief([entry], "1", "2026-11-10T13:00:00Z", "past_7_days")
	).toBeUndefined();
	expect(matchingActivityBrief([entry], undefined, undefined, "past_7_days")).toBeUndefined();
});

it("does not reuse checkpoint summaries after their cutoff changes", () => {
	const entry: ActivityBriefEntry = {
		notificationId: "1",
		version: "2026-11-10T12:00:00Z",
		selection: "last_read",
		since: "2026-11-09T12:00:00.123456Z",
		updatedAt: "",
		status: "ready",
		bullets: [],
	};
	expect(
		matchingActivityBrief([entry], "1", entry.version, "last_read", "2026-11-09T13:00:00.123456+01:00")
	).toBe(entry);
	expect(
		matchingActivityBrief([entry], "1", entry.version, "last_read", "2026-11-10T12:00:00Z")
	).toBeUndefined();
	expect(matchingActivityBrief([entry], "1", entry.version, "last_read", null)).toBeUndefined();
	expect(
		matchingActivityBrief([entry], "1", entry.version, "last_read", "2026-11-09T12:00:00.123456001Z")
	).toBeUndefined();
});

it.each(["today", "since_yesterday"] as const)(
	"does not reuse %s summaries across calendar boundaries",
	(selection) => {
		const entry: ActivityBriefEntry = {
			notificationId: "1",
			version: "2026-11-10T12:00:00Z",
			selection,
			since: "2026-11-12T00:00:00Z",
			updatedAt: "",
			status: "ready",
			bullets: [],
		};
		expect(matchingActivityBrief([entry], "1", entry.version, selection, entry.since)).toBe(entry);
		expect(
			matchingActivityBrief([entry], "1", entry.version, selection, "2026-11-13T00:00:00Z")
		).toBeUndefined();
	}
);
