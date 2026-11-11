import { expect, it } from "vitest";
import { matchingActivityBrief } from "./activityBriefs";
import type { ActivityBriefEntry } from "$lib/api/activityBriefs";

it("only displays results for the same notification, version and lookback", () => {
	const entry: ActivityBriefEntry = {
		notificationId: "1",
		version: "2026-11-10T12:00:00.123456Z",
		days: 3,
		since: "",
		updatedAt: "",
		status: "ready",
		bullets: [],
	};
	expect(matchingActivityBrief([entry], "1", "2026-11-10T12:00:00.123Z", 3)).toBe(entry);
	expect(matchingActivityBrief([entry], "2", entry.version, 3)).toBeUndefined();
	expect(matchingActivityBrief([entry], "1", entry.version, 7)).toBeUndefined();
	expect(matchingActivityBrief([entry], "1", "2026-11-10T13:00:00Z", 3)).toBeUndefined();
	expect(matchingActivityBrief([entry], undefined, undefined, 3)).toBeUndefined();
});
