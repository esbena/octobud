import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/svelte";
import { fetchWhatsNew } from "$lib/api/whatsNew";
import { fetchRichActivitySummary } from "$lib/api/activityBriefs";
import type { WhatsNewSnapshot } from "$lib/types/whatsNew";
import { formatRelativeShort } from "$lib/utils/time";
import { ALL_AVAILABLE_ACTIVITY_CUTOFF } from "$lib/utils/whatsNewCutoff";
import WhatsNewReader from "./WhatsNewReader.svelte";

vi.mock("$lib/api/whatsNew", () => ({ fetchWhatsNew: vi.fn() }));
vi.mock("$lib/api/activityBriefs", () => ({
	fetchRichActivitySummary: vi.fn(),
	activityBriefRequest: vi.fn(),
}));
beforeEach(() => vi.clearAllMocks());
afterEach(cleanup);

const cutoff = "2026-11-11T08:00:00Z";
const createdAt = "2026-11-12T08:00:00Z";
const readerProps = {
	githubId: "github-id",
	lastReadAt: cutoff,
	lastArchivedAt: null,
	selection: "last_read" as const,
};
const snapshot: WhatsNewSnapshot = {
	notificationId: "github-id",
	title: "Example",
	since: cutoff,
	fetchedAt: createdAt,
	totalEntries: 4,
	newActivity: [
		{
			id: "bot",
			kind: "commented",
			change: "created",
			actor: "ci[bot]",
			actorType: "bot",
			body: "Bot comment",
			createdAt,
			at: createdAt,
		},
		{
			id: "human",
			kind: "commented",
			change: "created",
			actor: "reviewer",
			actorType: "human",
			body: "Human comment",
			url: "https://github.com/o/r/issues/1#issuecomment-1",
			createdAt,
			at: createdAt,
		},
		{
			id: "reply",
			parentId: "opening",
			kind: "review_comment",
			change: "created",
			actor: "reviewer",
			actorType: "human",
			body: "Human thread reply",
			createdAt,
			at: createdAt,
		},
	],
	olderContext: [
		{
			id: "opening",
			kind: "review_comment",
			change: "created",
			actor: "ci[bot]",
			actorType: "bot",
			body: "Bot thread opening",
			createdAt: cutoff,
			at: cutoff,
		},
	],
	warnings: [],
};

it("interleaves human comments and threads while keeping bot comments separate", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValueOnce(snapshot);
	const onReady = vi.fn(() => {
		expect(screen.getByText("Human comment")).toBeInTheDocument();
	});
	const { container } = render(WhatsNewReader, { ...readerProps, onReady });
	await screen.findByText("Human comment");
	expect(
		within(screen.getByRole("article", { name: "New activity by reviewer" })).getByTitle(
			"Open activity on GitHub"
		)
	).toHaveAttribute("href", "https://github.com/o/r/issues/1#issuecomment-1");
	const cutoffLabel = container.querySelector("time");
	expect(cutoffLabel).toHaveAttribute("datetime", cutoff);
	expect(cutoffLabel).toHaveTextContent(formatRelativeShort(new Date(cutoff)));
	expect(screen.getByText("Showing Since last read")).toBeInTheDocument();
	expect(screen.queryByRole("group", { name: "Look back" })).not.toBeInTheDocument();
	expect(screen.queryByText(/Open an item on GitHub/)).not.toBeInTheDocument();
	expect(
		Array.from(container.querySelectorAll("h3")).map((heading) => heading.textContent)
	).toEqual(["Human activity", "Bot activity"]);
	const thread = screen.getByRole("region", { name: "Human activity" });
	expect(thread).toHaveTextContent("Bot thread opening");
	expect(thread).toHaveTextContent("Human thread reply");
	expect(thread).toHaveTextContent("Human comment");
	expect(screen.getByRole("region", { name: "Bot activity" })).toHaveTextContent("Bot comment");
	expect(screen.queryByText("Unknown actor activity")).not.toBeInTheDocument();
	expect(
		within(screen.getByRole("region", { name: "Human activity" })).queryByRole("separator")
	).not.toBeInTheDocument();
	await waitFor(() => expect(onReady).toHaveBeenCalledExactlyOnceWith(createdAt));
});

it("loads counts in the background without rendering or advancing seen, then reuses the result", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (_id, since) => ({
		...snapshot,
		since,
		totalEntries: 5,
		newActivity: [
			...snapshot.newActivity,
			{ id: "label", kind: "labeled", change: "created", actor: "reviewer", at: createdAt },
		],
	}));
	const onEntryCounts = vi.fn();
	const onReady = vi.fn();
	const { rerender } = render(WhatsNewReader, {
		...readerProps,
		active: false,
		readerUsername: "reader",
		onEntryCounts,
		onReady,
	});
	await waitFor(() => expect(onEntryCounts).toHaveBeenLastCalledWith({ full: 5, whatsNew: 4 }));
	expect(screen.queryByText("Human comment")).not.toBeInTheDocument();
	expect(onReady).not.toHaveBeenCalled();
	expect(fetchWhatsNew).toHaveBeenCalledTimes(1);
	await rerender({ active: true });
	await screen.findByText("Human comment");
	await waitFor(() => expect(onReady).toHaveBeenCalledExactlyOnceWith(createdAt));
	expect(
		vi.mocked(fetchWhatsNew).mock.calls.filter((call) => call[4] === "last_read")
	).toHaveLength(1);
});

it("clears previous counts while a different activity window loads", async () => {
	vi.mocked(fetchWhatsNew)
		.mockResolvedValueOnce(snapshot)
		.mockImplementationOnce(() => new Promise<WhatsNewSnapshot>(() => {}));
	const onEntryCounts = vi.fn();
	const { rerender } = render(WhatsNewReader, { ...readerProps, onEntryCounts });
	await waitFor(() => expect(onEntryCounts).toHaveBeenLastCalledWith({ full: 4, whatsNew: 4 }));
	await rerender({ selection: "last_archive" });
	await screen.findByText("Loading What's new...");
	expect(onEntryCounts).toHaveBeenLastCalledWith(null);
});

it("generates an on-demand rich summary with evidence links for the selected range", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValueOnce(snapshot);
	vi.mocked(fetchRichActivitySummary).mockResolvedValueOnce({
		richSummary: { text: "The author addressed the reviewer's request.", evidenceIds: ["human"] },
		newActivity: snapshot.newActivity,
		olderContext: snapshot.olderContext,
	});
	render(WhatsNewReader, { ...readerProps, richSummaryTrigger: 1 });
	await screen.findByText("Human comment");
	expect(
		await screen.findByText("The author addressed the reviewer's request.")
	).toBeInTheDocument();
	expect(fetchRichActivitySummary).toHaveBeenCalledExactlyOnceWith("github-id", cutoff);
	expect(screen.getByRole("link", { name: "Source: reviewer" })).toHaveAttribute(
		"href",
		"https://github.com/o/r/issues/1#issuecomment-1"
	);
});

it("accepts another detailed-summary request after the toolbar resets its trigger", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValue(snapshot);
	vi.mocked(fetchRichActivitySummary).mockResolvedValue({
		richSummary: { text: "The reviewer requested a change.", evidenceIds: ["human"] },
		newActivity: snapshot.newActivity,
		olderContext: snapshot.olderContext,
	});
	const onRichSummaryRequestHandled = vi.fn();
	const { rerender } = render(WhatsNewReader, {
		...readerProps,
		richSummaryTrigger: 1,
		onRichSummaryRequestHandled,
	});
	await screen.findByText("The reviewer requested a change.");
	await rerender({ richSummaryTrigger: 0 });
	await rerender({ richSummaryTrigger: 1 });
	await waitFor(() => expect(fetchRichActivitySummary).toHaveBeenCalledTimes(2));
	expect(onRichSummaryRequestHandled).toHaveBeenCalledTimes(2);
});

it("allows a summary for a new range while an obsolete request is still pending", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (_id, since) => ({ ...snapshot, since }));
	let completeOldSummary!: (result: Awaited<ReturnType<typeof fetchRichActivitySummary>>) => void;
	vi.mocked(fetchRichActivitySummary)
		.mockImplementationOnce(
			() =>
				new Promise((resolve) => {
					completeOldSummary = resolve;
				})
		)
		.mockResolvedValueOnce({
			richSummary: { text: "Summary for the archive range.", evidenceIds: ["human"] },
			newActivity: snapshot.newActivity,
			olderContext: snapshot.olderContext,
		});
	const { rerender } = render(WhatsNewReader, {
		...readerProps,
		lastArchivedAt: "2026-11-10T08:00:00Z",
		richSummaryTrigger: 1,
	});
	await waitFor(() => expect(fetchRichActivitySummary).toHaveBeenCalledTimes(1));
	await rerender({ selection: "last_archive", richSummaryTrigger: 0 });
	await screen.findByText("Showing Since last archive");
	await rerender({ richSummaryTrigger: 2 });
	await screen.findByText("Summary for the archive range.");
	completeOldSummary({
		richSummary: { text: "Obsolete summary.", evidenceIds: ["human"] },
		newActivity: snapshot.newActivity,
		olderContext: snapshot.olderContext,
	});
	await Promise.resolve();
	expect(screen.queryByText("Obsolete summary.")).not.toBeInTheDocument();
});

it("separates prior top-level context from new comments in human and non-human streams", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValueOnce({
		...snapshot,
		newActivity: snapshot.newActivity.slice(0, 2),
		olderContext: [
			{
				id: "human-context",
				kind: "commented",
				change: "created",
				actor: "reviewer",
				actorType: "human",
				body: "Prior human comment",
				createdAt: cutoff,
				at: cutoff,
			},
			{
				id: "bot-context",
				kind: "commented",
				change: "created",
				actor: "ci[bot]",
				actorType: "bot",
				body: "Prior bot comment",
				createdAt: cutoff,
				at: cutoff,
			},
		],
	});
	render(WhatsNewReader, readerProps);
	await screen.findByText("Prior human comment");
	for (const section of [
		screen.getByRole("region", { name: "Human activity" }),
		screen.getByRole("region", { name: "Bot activity" }),
	]) {
		const cards = within(section).getAllByRole("article");
		expect(cards).toHaveLength(2);
		expect(cards[0]).toHaveTextContent("Earlier context");
		expect(cards[1]).not.toHaveTextContent("Earlier context");
		expect(cards[0].querySelector(".opacity-75")).not.toBeNull();
		expect(cards[1].querySelector(".border-indigo-400")).not.toBeNull();
		expect(within(section).queryByText("New activity")).not.toBeInTheDocument();
	}
});

it("shows failures explicitly and retries the same frozen boundary", async () => {
	const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
	try {
		vi.mocked(fetchWhatsNew)
			.mockRejectedValueOnce(new Error("Incomplete coverage"))
			.mockResolvedValueOnce({ ...snapshot, newActivity: [], olderContext: [] });
		const onReady = vi.fn();
		render(WhatsNewReader, { ...readerProps, onReady });
		expect(await screen.findByRole("alert")).toHaveTextContent("Incomplete coverage");
		expect(onReady).not.toHaveBeenCalled();
		expect(screen.queryByText("No activity since your last read.")).not.toBeInTheDocument();
		await fireEvent.click(screen.getByRole("button", { name: "Try again" }));
		await screen.findByText("No activity since your last read.");
		expect(onReady).not.toHaveBeenCalled();
		expect(vi.mocked(fetchWhatsNew).mock.calls.map((call) => call.slice(0, 2))).toEqual([
			["github-id", cutoff],
			["github-id", cutoff],
		]);
		expect(vi.mocked(fetchWhatsNew).mock.calls.map((call) => call[4])).toEqual([
			"last_read",
			"last_read",
		]);
	} finally {
		consoleError.mockRestore();
	}
});

it("does not advance seen after leaving a loading reader", async () => {
	let complete!: (snapshot: WhatsNewSnapshot) => void;
	vi.mocked(fetchWhatsNew).mockImplementationOnce(
		() =>
			new Promise((resolve) => {
				complete = resolve;
			})
	);
	const onReady = vi.fn();
	const { unmount } = render(WhatsNewReader, { ...readerProps, onReady });
	await screen.findByRole("status");
	unmount();
	complete(snapshot);
	await Promise.resolve();
	expect(onReady).not.toHaveBeenCalled();
});

it("reports source-backed activity newer than a cutoff within the same millisecond", async () => {
	const previous = "2026-11-12T08:00:00.000000001Z";
	const candidate = "2026-11-12T08:00:00.000000002Z";
	vi.mocked(fetchWhatsNew).mockResolvedValueOnce({
		...snapshot,
		since: previous,
		olderContext: [],
		newActivity: [
			{
				id: "precise",
				kind: "commented",
				change: "created",
				actor: "reviewer",
				body: "Precise activity",
				at: candidate,
				createdAt: candidate,
			},
		],
	});
	const onReady = vi.fn();
	render(WhatsNewReader, { ...readerProps, lastReadAt: previous, onReady });
	await screen.findByText("Precise activity");
	await waitFor(() => expect(onReady).toHaveBeenCalledExactlyOnceWith(candidate));
});

it("shows the opening post and advances seen on a first read without comments", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValue({
		...snapshot,
		since: ALL_AVAILABLE_ACTIVITY_CUTOFF,
		newActivity: [
			{
				id: "opening",
				kind: "opened",
				change: "created",
				actor: "author",
				actorType: "human",
				body: "Opening post content\n\n`inline code` and [details](https://example.com/details)",
				createdAt,
				at: createdAt,
				url: "https://github.com/o/r/issues/1",
			},
		],
		olderContext: [],
	});
	const onReady = vi.fn();
	const onShowFullView = vi.fn();
	render(WhatsNewReader, { ...readerProps, lastReadAt: null, onReady, onShowFullView });
	expect(screen.getByText("Showing Since last read")).toBeInTheDocument();
	await screen.findByText("Opening post content");
	expect(screen.getByRole("img", { name: "author's avatar" })).toHaveAttribute(
		"src",
		"https://github.com/author.png"
	);
	expect(screen.getByText("inline code").tagName).toBe("CODE");
	expect(screen.getByRole("link", { name: "details" })).toHaveAttribute(
		"href",
		"https://example.com/details"
	);
	expect(screen.getByTitle("Open activity on GitHub")).toHaveAttribute(
		"href",
		"https://github.com/o/r/issues/1"
	);
	expect(screen.getByLabelText("Cutoff: all available history")).toBeInTheDocument();
	expect(fetchWhatsNew).toHaveBeenCalledExactlyOnceWith(
		"github-id",
		ALL_AVAILABLE_ACTIVITY_CUTOFF,
		expect.any(AbortSignal),
		undefined,
		"last_read"
	);
	await waitFor(() => expect(onReady).toHaveBeenCalledExactlyOnceWith(createdAt));
	expect(screen.queryByText(/No activity/)).not.toBeInTheDocument();
	expect(screen.getByText(/Showing activity from the start/)).toBeInTheDocument();
	await fireEvent.click(screen.getByRole("button", { name: "Switch to Full view" }));
	expect(onShowFullView).toHaveBeenCalledOnce();
});

it.each(["Updated opening description", ""])(
	"shows avatars and edit metadata for an opening post with body %j",
	async (body) => {
		vi.mocked(fetchWhatsNew).mockResolvedValue({
			...snapshot,
			newActivity: [
				{
					id: "opening",
					kind: "opened",
					change: "edited",
					actor: "author",
					actorType: "human",
					body,
					createdAt: cutoff,
					updatedAt: createdAt,
					at: createdAt,
					url: "https://github.com/o/r/issues/1",
				},
			],
			olderContext: [],
		});
		render(WhatsNewReader, readerProps);
		await screen.findByRole("img", { name: "author's avatar" });
		expect(screen.getByText("(edited)")).toBeInTheDocument();
		expect(screen.getByText("opened")).toBeInTheDocument();
		expect(screen.getByTitle("Open activity on GitHub")).toHaveAttribute(
			"href",
			"https://github.com/o/r/issues/1"
		);
		expect(screen.queryByText(/Showing activity from the start/)).not.toBeInTheDocument();
		if (body) expect(screen.getByText(body)).toBeInTheDocument();
	}
);

it.each(["past_7_days", "last_read"] as const)(
	"recommends Full view when the %s window includes the opening event",
	async (selection) => {
		vi.mocked(fetchWhatsNew).mockImplementation(async (_notificationId, since) => ({
			...snapshot,
			since,
			newActivity: [
				{
					id: "opening",
					kind: "opened",
					change: "created",
					actor: "author",
					actorType: "human",
					body: "Opening post inside the selected window",
					createdAt,
					at: createdAt,
				},
			],
			olderContext: [],
		}));
		const onShowFullView = vi.fn();
		render(WhatsNewReader, { ...readerProps, selection, onShowFullView });
		await screen.findByText("Opening post inside the selected window");
		expect(screen.getByText(/Showing activity from the start/)).toBeInTheDocument();
		expect(screen.queryByLabelText("Cutoff: all available history")).not.toBeInTheDocument();
		await fireEvent.click(screen.getByRole("button", { name: "Switch to Full view" }));
		expect(onShowFullView).toHaveBeenCalledOnce();
	}
);

it("does not recommend Full view for a delta without the opening event", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValue(snapshot);
	render(WhatsNewReader, { ...readerProps, onShowFullView: vi.fn() });
	await screen.findByText("Human comment");
	expect(screen.queryByText(/Showing activity from the start/)).not.toBeInTheDocument();
	expect(screen.queryByRole("button", { name: "Switch to Full view" })).not.toBeInTheDocument();
});

it("checks for a prior comment on open and disables its range when none exists", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (notificationId, since) => ({
		...snapshot,
		notificationId,
		since,
	}));
	render(WhatsNewReader, { ...readerProps, readerUsername: "reader" });

	await screen.findByText(/No comment by you was found/);
	expect(screen.getByText(/"Since my last comment" is unavailable/)).toBeInTheDocument();
	expect(fetchWhatsNew).toHaveBeenNthCalledWith(
		1,
		"github-id",
		ALL_AVAILABLE_ACTIVITY_CUTOFF,
		expect.any(AbortSignal)
	);
	expect(fetchWhatsNew).toHaveBeenNthCalledWith(
		2,
		"github-id",
		cutoff,
		expect.any(AbortSignal),
		undefined,
		"last_read"
	);
});

it("enables the personal range when the full-history check finds the reader's comment", async () => {
	const myComment = {
		id: "my-comment",
		kind: "commented",
		change: "created",
		actor: "reader",
		actorType: "human" as const,
		body: "My prior comment",
		createdAt: cutoff,
		at: cutoff,
	};
	vi.mocked(fetchWhatsNew).mockImplementation(async (notificationId, since) => ({
		...snapshot,
		notificationId,
		since,
		newActivity: [...snapshot.newActivity, myComment],
	}));
	render(WhatsNewReader, { ...readerProps, readerUsername: "reader" });

	await waitFor(() =>
		expect(screen.queryByText(/No comment by you was found/)).not.toBeInTheDocument()
	);
});

it("omits irrelevant activity and reader notes", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValueOnce({
		...snapshot,
		warnings: ["Evidence has no deep link: timeline:head_ref_deleted:1"],
		newActivity: [
			...snapshot.newActivity,
			{
				id: "branch-deleted",
				kind: "head_ref_deleted",
				change: "created",
				actor: "reviewer",
				actorType: "human",
				createdAt,
				at: createdAt,
			},
		],
	});
	render(WhatsNewReader, readerProps);

	await screen.findByText("Human comment");
	expect(screen.queryByText("Other human activity (1)")).not.toBeInTheDocument();
	expect(screen.queryByText("head ref deleted")).not.toBeInTheDocument();
	expect(screen.queryByText("Reader notes (1)")).not.toBeInTheDocument();
	expect(screen.queryByText(/Evidence has no deep link/)).not.toBeInTheDocument();
});

it("uses all available activity when no archive checkpoint exists without advancing last read", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (_githubId, since) => ({ ...snapshot, since }));
	const onReady = vi.fn();
	render(WhatsNewReader, { ...readerProps, lastReadAt: null, selection: "last_archive", onReady });
	await screen.findByText("Human comment");
	expect(screen.getByText("All available history")).toBeInTheDocument();
	expect(screen.queryByText(/Cutoff:/)).not.toBeInTheDocument();
	expect(screen.queryByText(/Activity in all available history/)).not.toBeInTheDocument();
	expect(screen.getByText("Showing Since last archive")).toBeInTheDocument();
	expect(fetchWhatsNew).toHaveBeenCalledWith(
		"github-id",
		"1970-01-01T00:00:00.000Z",
		expect.any(AbortSignal),
		undefined,
		"last_archive"
	);
	expect(onReady).not.toHaveBeenCalled();
});

it("enables the archive range and shows its recorded cutoff", async () => {
	vi.mocked(fetchWhatsNew).mockResolvedValueOnce(snapshot);
	const { container } = render(WhatsNewReader, {
		...readerProps,
		lastArchivedAt: cutoff,
		selection: "last_archive",
	});

	expect(screen.getByText("Showing Since last archive")).toBeInTheDocument();
	await screen.findByText("Human comment");
	expect(container.querySelector("time")).toHaveAttribute("datetime", cutoff);
});

it("uses the global personal range and anchors it at the last comment", async () => {
	const myComment = {
		id: "my-comment",
		kind: "commented",
		change: "created",
		actor: "reader",
		actorType: "human" as const,
		body: "My last comment",
		createdAt: cutoff,
		at: cutoff,
	};
	const newerComment = {
		id: "newer-comment",
		kind: "commented",
		change: "created",
		actor: "reviewer",
		actorType: "human" as const,
		body: "New since my comment",
		createdAt,
		at: createdAt,
	};
	vi.mocked(fetchWhatsNew).mockImplementation(async (_githubId, since) => ({
		...snapshot,
		since,
		newActivity: [myComment, newerComment],
		olderContext: [],
	}));
	const onReady = vi.fn();
	render(WhatsNewReader, {
		...readerProps,
		selection: "my_last_comment",
		readerUsername: "reader",
		onReady,
	});

	expect(screen.queryByRole("group", { name: "Look back" })).not.toBeInTheDocument();
	expect(screen.getByText("Showing Since my last comment")).toBeInTheDocument();
	await screen.findByText("New since my comment");
	expect(screen.getByText("My last comment")).toBeInTheDocument();
	expect(document.querySelector("time")).toHaveAttribute("datetime", cutoff);
	expect(screen.queryByText("Human comment")).not.toBeInTheDocument();
	expect(fetchWhatsNew).toHaveBeenCalledWith(
		"github-id",
		"1970-01-01T00:00:00.000Z",
		expect.any(AbortSignal),
		undefined,
		"my_last_comment"
	);
	expect(onReady).not.toHaveBeenCalled();
});

it("disables since my last comment after finding no matching comment", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (_githubId, since) => ({ ...snapshot, since }));
	render(WhatsNewReader, {
		...readerProps,
		selection: "my_last_comment",
		readerUsername: "reader",
	});

	await screen.findByText("All available history");
	expect(screen.getByText("Showing Since my last comment")).toBeInTheDocument();
	expect(screen.queryByRole("group", { name: "Look back" })).not.toBeInTheDocument();
});

it("loads Today from the local calendar-day boundary", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (_githubId, since) => ({ ...snapshot, since }));
	render(WhatsNewReader, { ...readerProps, selection: "today" });
	await screen.findByText("Human comment");

	const now = new Date();
	const expectedCutoff = new Date(now.getFullYear(), now.getMonth(), now.getDate()).toISOString();
	expect(vi.mocked(fetchWhatsNew).mock.calls[0][1]).toBe(expectedCutoff);
});

it("does not advance last read while a date range is selected", async () => {
	vi.mocked(fetchWhatsNew).mockImplementation(async (_githubId, since) => ({ ...snapshot, since }));
	const onReady = vi.fn();
	render(WhatsNewReader, {
		...readerProps,
		selection: "past_7_days",
		onReady,
	});
	await screen.findByText("Human comment");
	const requestedCutoff = vi.mocked(fetchWhatsNew).mock.calls[0][1];
	expect(Date.parse(requestedCutoff)).toBeLessThan(Date.now() - 6 * 24 * 60 * 60 * 1000);
	expect(Date.parse(requestedCutoff)).toBeGreaterThan(Date.now() - 8 * 24 * 60 * 60 * 1000);
	expect(onReady).not.toHaveBeenCalled();
});
