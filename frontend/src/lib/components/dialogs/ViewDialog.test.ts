import { fireEvent, render, screen } from "@testing-library/svelte";
import { describe, expect, it, vi } from "vitest";
import ViewDialog from "./ViewDialog.svelte";

vi.mock("$lib/api/tags", () => ({
	fetchTags: vi.fn().mockResolvedValue([]),
}));

describe("view sidebar visibility checkbox", () => {
	it("starts checked and submits the user's choice for a new view", async () => {
		const onSave = vi.fn();
		render(ViewDialog, { open: true, onSave });
		const checkbox = screen.getByRole("checkbox", { name: "Show in sidebar" });
		expect(checkbox).toBeChecked();
		await fireEvent.input(screen.getByLabelText("Name"), { target: { value: "Helper" } });
		await fireEvent.click(checkbox);
		await fireEvent.click(screen.getByRole("button", { name: "Save view" }));
		expect(onSave).toHaveBeenCalledWith(
			expect.objectContaining({ name: "Helper", showInSidebar: false })
		);
	});

	it("loads an unchecked saved value and resets it for the next new view", async () => {
		const onSave = vi.fn();
		const { rerender } = render(ViewDialog, {
			open: true,
			onSave,
			initialValue: { id: "helper", name: "Helper", query: "in:inbox", showInSidebar: false },
		});
		const checkbox = screen.getByRole("checkbox", { name: "Show in sidebar" });
		expect(checkbox).not.toBeChecked();
		await fireEvent.click(checkbox);
		await fireEvent.click(screen.getByRole("button", { name: "Save view" }));
		expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ showInSidebar: true }));
		await rerender({ open: false, initialValue: null });
		await rerender({ open: true });
		expect(screen.getByRole("checkbox", { name: "Show in sidebar" })).toBeChecked();
	});
});
