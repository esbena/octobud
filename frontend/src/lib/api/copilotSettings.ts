import { apiErrorFromResponse, fetchWithAuth } from "./fetch";

export interface CopilotSettings {
	enabled: boolean;
	activityHighlights: { enabled: boolean; defaultDays: number; model: string };
}

export async function copilotSettingsRequest(settings?: CopilotSettings): Promise<CopilotSettings> {
	const response = await fetchWithAuth(
		"/api/user/copilot-settings",
		settings
			? {
					method: "PUT",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify(settings),
				}
			: {}
	);
	if (!response.ok)
		throw await apiErrorFromResponse(response, "Unable to load or save Copilot settings");
	return response.json();
}
