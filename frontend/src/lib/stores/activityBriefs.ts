import { writable } from "svelte/store";
import type { ActivityBriefEntry } from "$lib/api/activityBriefs";

export const activityBriefDays = writable(3);
export const activityBriefEntries = writable<ActivityBriefEntry[]>([]);

export function matchingActivityBrief(
	entries: ActivityBriefEntry[],
	notificationId: string | undefined,
	version: string | undefined,
	days: number
): ActivityBriefEntry | undefined {
	if (!notificationId || !version) return undefined;
	return entries.find(
		(entry) =>
			entry.notificationId === notificationId &&
			entry.days === days &&
			new Date(entry.version).getTime() === new Date(version).getTime()
	);
}
