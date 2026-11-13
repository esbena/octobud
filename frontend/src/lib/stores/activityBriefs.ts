import { writable } from "svelte/store";
import type { ActivityBriefEntry } from "$lib/api/activityBriefs";
import type { WhatsNewSelection } from "$lib/utils/whatsNewCutoff";
import { compareWhatsNewTimestamps } from "$lib/utils/whatsNewGrouping";

export const activityBriefEnabled = writable(false);
export const activityBriefAutoSummarize = writable(false);
export const activityBriefEntries = writable<ActivityBriefEntry[]>([]);

export function matchingActivityBrief(
	entries: ActivityBriefEntry[],
	notificationId: string | undefined,
	version: string | undefined,
	selection: WhatsNewSelection,
	since?: string | null
): ActivityBriefEntry | undefined {
	if (!notificationId || !version) return undefined;
	return entries.find((entry) => {
		const checkpointMatches =
			selection === "past_7_days" || selection === "my_last_comment"
				? true
				: !!since &&
					compareWhatsNewTimestamps(entry.since, since) === 0;
		return (
			entry.notificationId === notificationId &&
			entry.selection === selection &&
			new Date(entry.version).getTime() === new Date(version).getTime() &&
			checkpointMatches
		);
	});
}
