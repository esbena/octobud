import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/svelte";
import { writable } from "svelte/store";
import { fetchAnnotations, type Annotation } from "$lib/api/annotations";
import type { Notification } from "$lib/api/types";
import AnnotationLabels from "./AnnotationLabels.svelte";
import NotificationRow from "./NotificationRow.svelte";
import NotificationListSection from "./NotificationListSection.svelte";

vi.mock("$app/environment", () => ({ browser: true }));
vi.mock("$lib/api/annotations", () => ({ fetchAnnotations: vi.fn() }));
vi.mock("$lib/stores/notificationSettings", () => ({
	getNotificationSettingsStore: () => ({
		hoverActionsEnabled: writable(false),
		catchUpSelection: writable("last_archive"),
	}),
}));

afterEach(cleanup);

const notification: Notification = {
	id: "1",
	repoFullName: "owner/repo",
	subjectTitle: "Update",
	subjectType: "pull_request",
	reason: "subscribed",
	archived: false,
	isRead: false,
	muted: false,
	starred: false,
	updatedAt: "2026-11-10T00:00:00Z",
	labels: [],
	viewIds: [],
	tags: [],
	subjectRaw: { body: "BREAKING CHANGE" },
};

const annotation: Annotation = {
	id: "annotation-1",
	name: "Breaking changes",
	enabled: true,
	viewIds: ["__inbox__"],
	code: `() => [{text: "⚠️ Breaking", color: "#d93f0b", hoverText: "Description mentions a breaking change"}]`,
};

describe("annotation label rendering", () => {
	it("runs enabled annotations in all views when unrestricted, or only their selected views", async () => {
		vi.mocked(fetchAnnotations).mockResolvedValue([
			annotation,
			{
				...annotation,
				id: "disabled",
				enabled: false,
				viewIds: [],
				code: `() => { throw new Error("Disabled code ran"); }`,
			},
			{
				...annotation,
				id: "global",
				viewIds: [],
				code: `() => [{text: "All views label", color: "#ffffff"}]`,
			},
			{
				...annotation,
				id: "elsewhere",
				viewIds: ["custom-view"],
				code: `() => [{text: "Other view", color: "#ffffff"}]`,
			},
		]);
		const selectedViewId = writable("__inbox__");
		const context = new Map([
			[
				"notificationPageController",
				{
					stores: {
						selectedViewId,
						pageData: writable({ items: [notification] }),
						splitModeEnabled: writable(false),
						pendingFocusIndex: writable(null),
						keyboardFocusIndex: writable(-1),
						savedListScrollPosition: writable(0),
						snoozeDropdownState: writable(null),
						tagDropdownState: writable(null),
						repositoryFilterOpen: writable(false),
					},
					derived: {
						hasRepositoryFilter: writable(false),
						selectedRepositories: writable([]),
						unreadOutsideSelection: writable(0),
					},
				},
			],
		]);
		render(NotificationListSection, {
			props: {
				items: [notification],
				hasActiveFilters: false,
				totalCount: 1,
				pageRangeStart: 1,
				pageRangeEnd: 1,
				selectionEnabled: false,
				selectionMap: new Map(),
			},
			context,
		});
		expect(await screen.findByText("⚠️ Breaking")).toBeInTheDocument();
		expect(await screen.findByText("All views label")).toBeInTheDocument();
		expect(screen.queryByText("Other view")).not.toBeInTheDocument();
		expect(screen.queryByText(/Annotation error/)).not.toBeInTheDocument();
		selectedViewId.set("custom-view");
		expect(await screen.findByText("Other view")).toBeInTheDocument();
		expect(await screen.findByText("All views label")).toBeInTheDocument();
		expect(screen.queryByText("⚠️ Breaking")).not.toBeInTheDocument();
		expect(screen.queryByText(/Annotation error/)).not.toBeInTheDocument();
	});

	it("renders labels on entries without existing tags", async () => {
		const context = new Map([
			[
				"notificationPageController",
				{
					stores: {
						pageData: writable({ items: [notification] }),
						splitModeEnabled: writable(false),
						pendingFocusIndex: writable(null),
						keyboardFocusIndex: writable(-1),
						savedListScrollPosition: writable(0),
						snoozeDropdownState: writable(null),
						tagDropdownState: writable(null),
					},
				},
			],
		]);
		render(NotificationRow, { props: { notification, annotations: [annotation] }, context });
		expect(await screen.findByText("⚠️ Breaking")).toHaveAttribute(
			"title",
			"Description mentions a breaking change"
		);
	});

	it("shows pending state for an async function, then its labels", async () => {
		render(AnnotationLabels, {
			notification,
			annotations: [
				{
					...annotation,
					code: `async () => {
					await new Promise(resolve => setTimeout(resolve, 20));
					return [{text: "Ready", color: "#ffffff"}];
				}`,
				},
			],
		});
		expect(screen.getByRole("status")).toHaveTextContent("Running Breaking changes");
		expect(await screen.findByText("Ready")).toBeInTheDocument();
		expect(screen.queryByRole("status")).not.toBeInTheDocument();
	});

	it("renders text literally without interpreting HTML", async () => {
		const { container } = render(AnnotationLabels, {
			notification,
			annotations: [
				{
					...annotation,
					code: `() => [{text: "<img src=x>", color: "#ffffff", hoverText: "<b>Plain text</b>"}]`,
				},
			],
		});
		expect(await screen.findByText("<img src=x>")).toHaveAttribute("title", "<b>Plain text</b>");
		expect(container.querySelector("img")).toBeNull();
		expect(container.querySelector("b")).toBeNull();
	});

	it("shows errors without suppressing results from other annotations", async () => {
		render(AnnotationLabels, {
			notification,
			annotations: [
				annotation,
				{
					...annotation,
					id: "invalid",
					name: "Invalid",
					code: `async () => { throw new Error("Cannot inspect body"); }`,
				},
			],
		});

		expect(await screen.findByText("⚠️ Breaking")).toBeInTheDocument();
		expect(await screen.findByText("Annotation error: Invalid")).toHaveAttribute(
			"title",
			"Invalid: Error: Cannot inspect body"
		);
	});

	it("reports sparse label arrays as annotation errors", async () => {
		render(AnnotationLabels, {
			notification,
			annotations: [{ ...annotation, code: "() => Array(1)" }],
		});
		expect(await screen.findByText("Annotation error: Breaking changes")).toHaveAttribute(
			"title",
			"Breaking changes: Error: Each label must be an object."
		);
	});

	it("stops spinning and shows an error when the thrown value cannot be formatted", async () => {
		render(AnnotationLabels, {
			notification,
			annotations: [{ ...annotation, code: "() => { throw Object.create(null); }" }],
		});
		expect(await screen.findByText("Annotation error: Breaking changes")).toHaveAttribute(
			"title",
			"Breaking changes: Annotation failed without an error message."
		);
		expect(screen.queryByRole("status")).not.toBeInTheDocument();
	});

	it("discards late results after the definitions or view assignment change", async () => {
		const { rerender } = render(AnnotationLabels, {
			notification,
			annotations: [
				{
					...annotation,
					code: `async () => {
					await new Promise(resolve => setTimeout(resolve, 30));
					return [{text: "Stale", color: "#ffffff"}];
				}`,
				},
			],
		});
		await rerender({ annotations: [annotation] });
		expect(await screen.findByText("⚠️ Breaking")).toBeInTheDocument();
		await new Promise((resolve) => setTimeout(resolve, 50));
		expect(screen.queryByText("Stale")).not.toBeInTheDocument();
		await rerender({ annotations: [] });
		await waitFor(() => expect(screen.queryByText("⚠️ Breaking")).not.toBeInTheDocument());
	});
});
