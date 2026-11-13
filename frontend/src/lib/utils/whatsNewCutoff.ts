export type WhatsNewSelection =
	| "last_read"
	| "last_archive"
	| "my_last_comment"
	| "today"
	| "since_yesterday"
	| "past_7_days";

export const WHATS_NEW_SELECTIONS: { value: WhatsNewSelection; label: string }[] = [
	{ value: "today", label: "Today" },
	{ value: "since_yesterday", label: "Since yesterday" },
	{ value: "past_7_days", label: "Past 7 days" },
	{ value: "my_last_comment", label: "Since my last comment" },
	{ value: "last_read", label: "Since last read" },
	{ value: "last_archive", label: "Since last archive" },
];

export const ALL_AVAILABLE_ACTIVITY_CUTOFF = "1970-01-01T00:00:00.000Z";

export function getWhatsNewCutoff(
	selection: WhatsNewSelection,
	lastReadAt: string | null,
	lastArchivedAt: string | null,
	now = new Date()
): string | null {
	if (selection === "today") {
		return new Date(now.getFullYear(), now.getMonth(), now.getDate()).toISOString();
	}
	if (selection === "since_yesterday") {
		return new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1).toISOString();
	}
	if (selection === "past_7_days") {
		return new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000).toISOString();
	}
	if (selection === "my_last_comment") return ALL_AVAILABLE_ACTIVITY_CUTOFF;
	if (selection === "last_read") return lastReadAt ?? ALL_AVAILABLE_ACTIVITY_CUTOFF;
	if (selection === "last_archive") return lastArchivedAt ?? ALL_AVAILABLE_ACTIVITY_CUTOFF;
	throw new Error(`Unsupported What's new range: ${selection}`);
}

export function whatsNewRangeLabel(selection: WhatsNewSelection): string {
	if (selection === "today") return "since the start of today";
	if (selection === "since_yesterday") return "since yesterday";
	if (selection === "past_7_days") return "in the past 7 days";
	if (selection === "my_last_comment") return "since your last comment";
	if (selection === "last_read") return "since your last read";
	if (selection === "last_archive") return "since your last archive";
	return "";
}
