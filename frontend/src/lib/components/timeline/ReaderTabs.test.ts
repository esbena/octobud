import { afterEach, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/svelte";
import ReaderTabs from "./ReaderTabs.svelte";

afterEach(cleanup);

it("shows both entry counts, including zero, and leaves unknown counts unlabeled", async () => {
	const { rerender } = render(ReaderTabs, {
		id: "reader",
		counts: { full: 8, whatsNew: 0 },
	});
	expect(screen.getByRole("tab", { name: "Full view (8)" })).toBeInTheDocument();
	const whatsNew = screen.getByRole("tab", { name: "What's new (0)" });
	await fireEvent.click(whatsNew);
	expect(whatsNew).toHaveAttribute("aria-selected", "true");
	await rerender({ counts: null });
	expect(screen.getByRole("tab", { name: "Full view" })).toBeInTheDocument();
	expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true");
});

it("always exposes What's new so the reader can choose another baseline", async () => {
	render(ReaderTabs, { id: "reader" });
	expect(screen.getByRole("tab", { name: "What's new" })).toBeInTheDocument();
	expect(screen.getByRole("tab", { name: "Full view" })).toHaveAttribute("aria-selected", "true");
	await fireEvent.keyDown(screen.getByRole("tab", { name: "Full view" }), { key: "ArrowRight" });
	expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true");
});

it("supports keyboard navigation and identifies the selected panel", async () => {
	render(ReaderTabs, { id: "reader" });
	const full = screen.getByRole("tab", { name: "Full view" });
	const whatsNew = screen.getByRole("tab", { name: "What's new" });
	await fireEvent.keyDown(full, { key: "ArrowRight" });
	expect(whatsNew).toHaveAttribute("aria-selected", "true");
	expect(whatsNew).toHaveAttribute("aria-controls", "reader-whats_new-panel");
	expect(whatsNew).toHaveFocus();
	await fireEvent.keyDown(whatsNew, { key: "Home" });
	expect(full).toHaveAttribute("aria-selected", "true");
	expect(full).toHaveFocus();
});
