import { apiErrorFromResponse, fetchWithAuth } from "./fetch";
import type { WhatsNewSelection } from "$lib/utils/whatsNewCutoff";
import type { WhatsNewEvidence } from "$lib/types/whatsNew";

export interface ActivityBullet {
	text: string;
	recipient: "for_you" | "other_activity";
	evidenceIds: string[];
}

export interface ActivityBriefEntry {
	notificationId: string;
	version: string;
	selection: WhatsNewSelection;
	status: "queued" | "running" | "ready" | "failed" | "cancelled";
	since: string;
	updatedAt: string;
	bullets: ActivityBullet[];
	warnings?: string[];
	error?: string;
}

export interface ActivityBriefStatus {
	enabled: boolean;
	autoSummarize: boolean;
	entries: ActivityBriefEntry[];
	queued: number;
	skipped: number;
	notice?: string;
}

export interface RichActivitySummary {
	text: string;
	evidenceIds: string[];
}

export interface RichActivitySummaryResponse {
	richSummary: RichActivitySummary;
	newActivity: WhatsNewEvidence[];
	olderContext: WhatsNewEvidence[];
}

export async function fetchRichActivitySummary(
	notificationId: string,
	since: string,
	fetchImpl?: typeof fetch
): Promise<RichActivitySummaryResponse> {
	const response = await fetchWithAuth(
		`/api/notifications/${encodeURIComponent(notificationId)}/activity-brief`,
		{
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ since, rich: true }),
		},
		fetchImpl
	);
	if (!response.ok) {
		throw await apiErrorFromResponse(response, "Unable to generate a detailed activity summary");
	}
	return response.json();
}

export async function activityBriefRequest(
	action: "status" | "batch" | "stop",
	input?:
		| { query: string; repositoryIds: number[]; selection: WhatsNewSelection; timeZone: string }
		| {
				notificationId: string;
				selection: WhatsNewSelection;
				timeZone: string;
				since?: string;
		  },
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
