import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import CopilotSettingsSection from "./CopilotSettingsSection.svelte";
import { copilotSettingsRequest, type CopilotSettings } from "$lib/api/copilotSettings";

vi.mock("$lib/api/copilotSettings", () => ({ copilotSettingsRequest: vi.fn() }));
const defaults: CopilotSettings = {
	enabled: false,
	activityHighlights: { enabled: false, defaultDays: 3, model: "gpt-6-luna" },
};

afterEach(() => {
	cleanup();
	vi.resetAllMocks();
});

it("loads disabled defaults and explains standard authentication without a token field", async () => {
	vi.mocked(copilotSettingsRequest).mockResolvedValue(structuredClone(defaults));
	render(CopilotSettingsSection);
	const master = await screen.findByRole("checkbox", { name: "Enable Copilot" });
	expect(master).not.toBeChecked();
	expect(screen.getByRole("checkbox", { name: "Enable activity highlights" })).toBeDisabled();
	expect(
		screen.getAllByText("COPILOT_GITHUB_TOKEN", { selector: "code", exact: true })
	).toHaveLength(2);
	expect(screen.queryByLabelText(/token/i)).not.toBeInTheDocument();
	expect(copilotSettingsRequest).toHaveBeenCalledExactlyOnceWith();
});

it("saves independent feature preferences without generating summaries", async () => {
	vi.mocked(copilotSettingsRequest).mockResolvedValue(structuredClone(defaults));
	render(CopilotSettingsSection);
	await fireEvent.click(await screen.findByRole("checkbox", { name: "Enable Copilot" }));
	await fireEvent.click(screen.getByRole("checkbox", { name: "Enable activity highlights" }));
	await fireEvent.change(screen.getByRole("combobox", { name: "Default lookback" }), {
		target: { value: "7" },
	});
	await fireEvent.input(screen.getByRole("textbox", { name: "Model" }), {
		target: { value: "another-model" },
	});
	await fireEvent.click(screen.getByRole("button", { name: "Save" }));
	await waitFor(() =>
		expect(copilotSettingsRequest).toHaveBeenLastCalledWith({
			enabled: true,
			activityHighlights: { enabled: true, defaultDays: 7, model: "another-model" },
		})
	);
	expect(await screen.findByRole("status")).toHaveTextContent("Copilot settings saved.");
});

it("surfaces load failures with a retry", async () => {
	vi.mocked(copilotSettingsRequest).mockRejectedValue(new Error("Settings unavailable"));
	render(CopilotSettingsSection);
	expect(await screen.findByRole("alert")).toHaveTextContent("Settings unavailable");
	expect(screen.getByRole("button", { name: "Retry loading settings" })).toBeEnabled();
});
