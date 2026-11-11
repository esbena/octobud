import { apiErrorFromResponse, fetchWithAuth } from "./fetch";

export interface ActivityBullet {
	text: string;
	recipient: "for_you" | "other_activity";
	evidenceIds: string[];
}

export interface ActivityBriefEntry {
	notificationId: string;
	version: string;
	days: number;
	status: "queued" | "running" | "ready" | "failed" | "cancelled";
	since: string;
	updatedAt: string;
	bullets: ActivityBullet[];
	warnings?: string[];
	error?: string;
}

export interface ActivityBriefStatus {
	enabled: boolean;
	entries: ActivityBriefEntry[];
	queued: number;
	notice?: string;
	defaultDays: number;
}

export async function activityBriefRequest(
	action: "status" | "batch" | "stop",
	input?: { query: string; repositoryIds: number[]; days: number },
	fetchImpl?: typeof fetch
): Promise<ActivityBriefStatus> {
	const response = await fetchWithAuth(
		`/api/notifications/activity-briefs${action === "stop" ? "/stop" : ""}`,
		action === "status"
			? {}
			: {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: input ? JSON.stringify(input) : undefined,
				},
		fetchImpl
	);
	if (!response.ok) {
		throw await apiErrorFromResponse(response, "Unable to load activity summaries");
	}
	return response.json();
}
