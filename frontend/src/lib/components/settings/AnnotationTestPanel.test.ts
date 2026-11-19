import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { fetchNotificationDetail } from "$lib/api/notifications";
import type { Notification, NotificationDetail } from "$lib/api/types";
import { createAnnotationContext } from "$lib/utils/annotations";
import AnnotationTestPanel from "./AnnotationTestPanel.svelte";

vi.mock("$lib/api/notifications", () => ({ fetchNotificationDetail: vi.fn() }));

const notification: Notification = {
	id: "1",
	githubId: "27088099001",
	repoFullName: "owner/repo",
	subjectTitle: "Example",
	subjectType: "issue",
	reason: "mention",
	archived: false,
	isRead: false,
	muted: false,
	starred: false,
	updatedAt: "2026-11-10T00:00:00Z",
	labels: [],
	viewIds: [],
	subjectRaw: { body: "Hello @esbena" },
};
const detail: NotificationDetail = { notification, subject: null };
const url = "http://localhost:8808/views/inbox?id=27088099001";
const code = `async ({ subject }) => [{text: subject.body, color: "#0969da"}]`;

beforeEach(() => {
	vi.resetAllMocks();
	vi.mocked(fetchNotificationDetail).mockResolvedValue(detail);
});
afterEach(cleanup);

async function loadInput(value = url) {
	await fireEvent.input(screen.getByLabelText("Notification URL"), { target: { value } });
	await fireEvent.click(screen.getByRole("button", { name: "Load input" }));
}

describe("annotation test panel", () => {
	it("loads the actual function argument and runs unsaved code without saving anything", async () => {
		render(AnnotationTestPanel, { code });
		expect(screen.getByRole("button", { name: "Run annotation" })).toBeDisabled();
		await loadInput();
		await waitFor(() =>
			expect(screen.getByLabelText("Function argument (JSON)")).toHaveValue(
				JSON.stringify(createAnnotationContext(notification), null, 2)
			)
		);
		expect(screen.getByLabelText("Function argument (JSON)")).toHaveAttribute("readonly");
		expect(fetchNotificationDetail).toHaveBeenCalledWith("27088099001", {
			signal: expect.any(AbortSignal),
		});
		await fireEvent.click(screen.getByRole("button", { name: "Run annotation" }));
		await waitFor(() =>
			expect(screen.getByLabelText("Function output (JSON)")).toHaveValue(
				JSON.stringify([{ text: "Hello @esbena", color: "#0969da" }], null, 2)
			)
		);
		expect(screen.getByLabelText("Function output (JSON)")).toHaveAttribute("readonly");
	});

	it.each([
		"not a URL",
		"javascript:alert(1)",
		"http://localhost:8808/views/inbox",
		"http://localhost:8808/settings?id=1",
		"http://localhost:8808/views/inbox?id=1&id=2",
	])("reports invalid URLs without fetching: %s", async (value) => {
		render(AnnotationTestPanel, { code });
		await loadInput(value);
		expect(await screen.findByRole("alert")).toBeInTheDocument();
		expect(fetchNotificationDetail).not.toHaveBeenCalled();
	});

	it("reports load failures and allows retry", async () => {
		vi.mocked(fetchNotificationDetail).mockRejectedValueOnce(new Error("Notification not found"));
		render(AnnotationTestPanel, { code });
		await loadInput();
		expect(await screen.findByRole("alert")).toHaveTextContent("Notification not found");
		await fireEvent.click(screen.getByRole("button", { name: "Load input" }));
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Run annotation" })).toBeEnabled()
		);
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});

	it.each([
		["async (", "SyntaxError"],
		['() => { throw new Error("Broken function"); }', "Broken function"],
		['async () => { throw ""; }', "Annotation failed without an error message."],
		["() => ({})", "Return an array"],
	])("reports syntax, runtime and output errors: %s", async (source, message) => {
		render(AnnotationTestPanel, { code: source });
		await loadInput();
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Run annotation" })).toBeEnabled()
		);
		await fireEvent.click(screen.getByRole("button", { name: "Run annotation" }));
		expect(await screen.findByRole("alert")).toHaveTextContent(message);
		expect(screen.getByLabelText("Function output (JSON)")).toHaveValue("");
	});

	it("clears stale output on code changes and shows an empty array as a successful result", async () => {
		const { rerender } = render(AnnotationTestPanel, { code: "() => []" });
		await loadInput();
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Run annotation" })).toBeEnabled()
		);
		await fireEvent.click(screen.getByRole("button", { name: "Run annotation" }));
		await waitFor(() => expect(screen.getByLabelText("Function output (JSON)")).toHaveValue("[]"));
		await rerender({ code });
		expect(screen.getByLabelText("Function output (JSON)")).toHaveValue("");
		expect(screen.getByLabelText("Function argument (JSON)")).not.toHaveValue("");
	});

	it("ignores an old load after the URL changes, including while code is edited", async () => {
		let resolve!: (value: NotificationDetail) => void;
		vi.mocked(fetchNotificationDetail).mockReturnValueOnce(
			new Promise((done) => {
				resolve = done;
			})
		);
		const { rerender } = render(AnnotationTestPanel, { code });
		await loadInput();
		expect(screen.getByRole("status")).toHaveTextContent("Loading notification");
		await rerender({ code: "() => []" });
		expect(screen.getByRole("status")).toHaveTextContent("Loading notification");
		await fireEvent.input(screen.getByLabelText("Notification URL"), {
			target: { value: "http://localhost:8808/views/inbox?id=2" },
		});
		resolve(detail);
		await waitFor(() => expect(screen.queryByRole("status")).not.toBeInTheDocument());
		expect(screen.getByLabelText("Function argument (JSON)")).toHaveValue("");
		expect(screen.getByRole("button", { name: "Run annotation" })).toBeDisabled();
	});

	it("discards pending function output after code changes", async () => {
		const { rerender } = render(AnnotationTestPanel, {
			code: `async () => {
			await new Promise(resolve => setTimeout(resolve, 30));
			return [{text: "Stale", color: "#ffffff"}];
		}`,
		});
		await loadInput();
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "Run annotation" })).toBeEnabled()
		);
		await fireEvent.click(screen.getByRole("button", { name: "Run annotation" }));
		expect(screen.getByRole("status")).toHaveTextContent("Running annotation");
		await rerender({ code: "() => []" });
		await new Promise((resolve) => setTimeout(resolve, 50));
		expect(screen.getByLabelText("Function output (JSON)")).toHaveValue("");
	});
});
