import { apiErrorFromResponse, fetchWithAuth } from "./fetch";
import type { WhatsNewSnapshot } from "$lib/types/whatsNew";
import type { WhatsNewSelection } from "$lib/utils/whatsNewCutoff";

export async function fetchWhatsNew(
	githubId: string,
	cutoff: string,
	signal?: AbortSignal,
	fetchImpl?: typeof fetch,
	selection: WhatsNewSelection = "last_read"
): Promise<WhatsNewSnapshot> {
	const response = await fetchWithAuth(
		`/api/notifications/${encodeURIComponent(githubId)}/whats-new`,
		{
			method: "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ since: cutoff, selection }),
			signal,
		},
		fetchImpl
	);
	if (!response.ok) {
		throw await apiErrorFromResponse(response, "Unable to load what's new");
	}
	return response.json();
}
