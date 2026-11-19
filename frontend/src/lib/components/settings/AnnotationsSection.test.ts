import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/svelte";
import {
	fetchAnnotations,
	fetchAnnotationViews,
	saveAnnotation,
	deleteAnnotation,
	type Annotation,
} from "$lib/api/annotations";
import AnnotationsSection from "./AnnotationsSection.svelte";
import { toastStore } from "$lib/stores/toastStore";

vi.mock("$lib/api/annotations", () => ({
	fetchAnnotations: vi.fn(),
	fetchAnnotationViews: vi.fn(),
	saveAnnotation: vi.fn(),
	deleteAnnotation: vi.fn(),
}));
vi.mock("$lib/stores/toastStore", () => ({ toastStore: { show: vi.fn() } }));

const annotation: Annotation = {
	id: "a1",
	name: "Existing",
	code: "() => []",
	viewIds: ["__inbox__"],
	enabled: true,
};

beforeEach(() => {
	vi.resetAllMocks();
	vi.mocked(fetchAnnotations).mockResolvedValue([]);
	vi.mocked(fetchAnnotationViews).mockResolvedValue([
		{
			id: "v1",
			name: "My view",
			slug: "my-view",
			query: "is:unread",
			unreadCount: 0,
		},
	]);
});
afterEach(cleanup);

async function openExisting() {
	await fireEvent.click(await screen.findByRole("button", { name: /Existing/ }));
	await fireEvent.click(screen.getByRole("button", { name: "Edit annotation" }));
}

describe("annotation settings", () => {
	it("creates an annotation with selected built-in and custom views", async () => {
		vi.mocked(saveAnnotation).mockResolvedValue({
			...annotation,
			name: "New annotation",
			viewIds: ["__inbox__", "v1"],
		});
		render(AnnotationsSection);
		await screen.findByText("No annotations yet.");
		await fireEvent.click(screen.getByRole("button", { name: "New annotation" }));
		await fireEvent.input(screen.getByRole("textbox", { name: "Name" }), {
			target: { value: "New annotation" },
		});
		await fireEvent.input(screen.getByRole("textbox", { name: "JavaScript function expression" }), {
			target: { value: "async ({ subject }) => []" },
		});
		await fireEvent.click(screen.getByRole("checkbox", { name: "Inbox" }));
		await fireEvent.click(screen.getByRole("checkbox", { name: "My view" }));
		await fireEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", { name: "Create annotation" })
		);
		await waitFor(() =>
			expect(saveAnnotation).toHaveBeenCalledWith(
				{
					name: "New annotation",
					code: "async ({ subject }) => []",
					viewIds: ["__inbox__", "v1"],
					enabled: true,
				},
				undefined
			)
		);
		expect(await screen.findByText("Inbox, My view")).toBeInTheDocument();
	});

	it("edits an annotation and saves an empty view selection", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		vi.mocked(saveAnnotation).mockResolvedValue({ ...annotation, viewIds: [] });
		render(AnnotationsSection);
		await openExisting();
		expect(screen.getByRole("textbox", { name: "JavaScript function expression" })).toHaveValue(
			"() => []"
		);
		expect(screen.getByRole("checkbox", { name: "Inbox" })).toBeChecked();
		await fireEvent.click(screen.getByRole("checkbox", { name: "Inbox" }));
		await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
		await waitFor(() =>
			expect(saveAnnotation).toHaveBeenCalledWith(
				{
					name: "Existing",
					code: "() => []",
					viewIds: [],
					enabled: true,
				},
				"a1"
			)
		);
		expect(
			await screen.findByRole("button", { name: /Existing.*All views/ })
		).toBeInTheDocument();
	});

	it("does not save a syntactically invalid function", async () => {
		render(AnnotationsSection);
		await screen.findByText("No annotations yet.");
		await fireEvent.click(screen.getByRole("button", { name: "New annotation" }));
		await fireEvent.input(screen.getByRole("textbox", { name: "Name" }), {
			target: { value: "Invalid" },
		});
		await fireEvent.input(screen.getByRole("textbox", { name: "JavaScript function expression" }), {
			target: { value: "async (" },
		});
		await fireEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", { name: "Create annotation" })
		);
		expect(await screen.findByRole("alert")).toHaveTextContent("SyntaxError");
		expect(saveAnnotation).not.toHaveBeenCalled();
	});

	it("keeps the editor open and shows server save failures", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		vi.mocked(saveAnnotation).mockRejectedValue(new Error("Cannot save"));
		render(AnnotationsSection);
		await openExisting();
		await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
		expect(await screen.findByRole("alert")).toHaveTextContent("Cannot save");
		expect(screen.getByRole("dialog", { name: "Edit annotation" })).toBeInTheDocument();
	});

	it("confirms deletion before removing an annotation", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		vi.mocked(deleteAnnotation).mockResolvedValue();
		render(AnnotationsSection);
		await openExisting();
		await fireEvent.click(screen.getByRole("button", { name: "Delete annotation" }));
		expect(deleteAnnotation).not.toHaveBeenCalled();
		await fireEvent.click(screen.getByRole("button", { name: "Delete" }));
		await waitFor(() => expect(deleteAnnotation).toHaveBeenCalledWith("a1"));
		expect(await screen.findByText("No annotations yet.")).toBeInTheDocument();
	});

	it("surfaces loading failures and supports retry", async () => {
		vi.mocked(fetchAnnotations).mockRejectedValueOnce(new Error("Cannot load"));
		render(AnnotationsSection);
		expect(await screen.findByRole("alert")).toHaveTextContent("Cannot load");
		await fireEvent.click(screen.getByRole("button", { name: "Retry" }));
		expect(await screen.findByText("No annotations yet.")).toBeInTheDocument();
	});

	it("expands an overview without opening the editor", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		render(AnnotationsSection);
		const overview = await screen.findByRole("button", { name: /Existing/ });
		expect(overview).toHaveAttribute("aria-expanded", "false");
		expect(screen.queryByText("() => []")).not.toBeInTheDocument();
		await fireEvent.click(overview);
		expect(overview).toHaveAttribute("aria-expanded", "true");
		expect(screen.getByText("() => []")).toBeInTheDocument();
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		await fireEvent.click(overview);
		expect(screen.queryByRole("button", { name: "Edit annotation" })).not.toBeInTheDocument();
	});

	it("disables and reenables independently of view assignments", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		vi.mocked(saveAnnotation)
			.mockResolvedValueOnce({ ...annotation, enabled: false })
			.mockResolvedValueOnce(annotation);
		render(AnnotationsSection);
		await fireEvent.click(await screen.findByRole("button", { name: /Existing/ }));
		await fireEvent.click(screen.getByRole("checkbox", { name: "Enable Existing" }));
		await waitFor(() =>
			expect(saveAnnotation).toHaveBeenLastCalledWith(
				{
					name: annotation.name,
					code: annotation.code,
					viewIds: annotation.viewIds,
					enabled: false,
				},
				annotation.id
			)
		);
		await waitFor(() =>
			expect(screen.getByRole("checkbox", { name: "Enable Existing" })).not.toBeChecked()
		);
		await fireEvent.click(screen.getByRole("checkbox", { name: "Enable Existing" }));
		await waitFor(() =>
			expect(saveAnnotation).toHaveBeenLastCalledWith(
				{
					name: annotation.name,
					code: annotation.code,
					viewIds: annotation.viewIds,
					enabled: true,
				},
				annotation.id
			)
		);
		await waitFor(() =>
			expect(screen.getByRole("checkbox", { name: "Enable Existing" })).toBeChecked()
		);
	});

	it("saves editor enablement and keeps the selected views", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		vi.mocked(saveAnnotation).mockResolvedValue({ ...annotation, enabled: false });
		render(AnnotationsSection);
		await openExisting();
		await fireEvent.click(screen.getByRole("checkbox", { name: "Enable annotation" }));
		await fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
		await waitFor(() =>
			expect(saveAnnotation).toHaveBeenCalledWith(
				{
					name: "Existing",
					code: "() => []",
					viewIds: ["__inbox__"],
					enabled: false,
				},
				"a1"
			)
		);
	});

	it("keeps the previous enablement and reports toggle failures", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([annotation]);
		vi.mocked(saveAnnotation).mockRejectedValue(new Error("Cannot update"));
		render(AnnotationsSection);
		await fireEvent.click(await screen.findByRole("button", { name: /Existing/ }));
		await fireEvent.click(screen.getByRole("checkbox", { name: "Enable Existing" }));
		await waitFor(() =>
			expect(toastStore.show).toHaveBeenCalledWith(
				"Failed to update annotation: Error: Cannot update",
				"error"
			)
		);
		expect(screen.getByRole("checkbox", { name: "Enable Existing" })).toBeChecked();
	});
});
