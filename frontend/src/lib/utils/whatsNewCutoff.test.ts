import { describe, expect, it } from "vitest";
import { getWhatsNewCutoff, whatsNewRangeLabel } from "./whatsNewCutoff";

describe("getWhatsNewCutoff", () => {
	const now = new Date("2026-11-12T10:00:00Z");

	it("uses the recorded read and archive timestamps", () => {
		expect(getWhatsNewCutoff("last_read", "2026-11-11T10:00:00Z", null, now)).toBe(
			"2026-11-11T10:00:00Z"
		);
		expect(getWhatsNewCutoff("last_archive", null, "2026-11-10T10:00:00Z", now)).toBe(
			"2026-11-10T10:00:00Z"
		);
	});

	it("uses local calendar boundaries for today and since yesterday", () => {
		const today = new Date(now.getFullYear(), now.getMonth(), now.getDate()).toISOString();
		const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1).toISOString();
		expect(getWhatsNewCutoff("today", null, null, now)).toBe(today);
		expect(getWhatsNewCutoff("since_yesterday", null, null, now)).toBe(yesterday);
	});

	it("uses a rolling seven-day cutoff and all history for my last comment", () => {
		expect(getWhatsNewCutoff("past_7_days", null, null, now)).toBe("2026-11-05T10:00:00.000Z");
		expect(getWhatsNewCutoff("my_last_comment", null, null, now)).toBe("1970-01-01T00:00:00.000Z");
		expect(whatsNewRangeLabel("today")).toBe("since the start of today");
		expect(whatsNewRangeLabel("since_yesterday")).toBe("since yesterday");
		expect(whatsNewRangeLabel("past_7_days")).toBe("in the past 7 days");
		expect(whatsNewRangeLabel("my_last_comment")).toBe("since your last comment");
	});

	it("shows all available activity when no read or archive checkpoint exists", () => {
		expect(getWhatsNewCutoff("last_read", null, null, now)).toBe("1970-01-01T00:00:00.000Z");
		expect(getWhatsNewCutoff("last_archive", null, null, now)).toBe("1970-01-01T00:00:00.000Z");
	});
});
