import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/svelte";
import { writable, type Writable } from "svelte/store";
import type { Notification } from "$lib/api/types";
import type { WhatsNewEntryCounts } from "$lib/types/whatsNew";
import { createTimelineController } from "$lib/state/timelineController";
import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";
import { updateTimelineLastSeen } from "$lib/api/notifications";
import InlineDetailView from "./InlineDetailView.svelte";

const captures = vi.hoisted(() => ({
	readers: [] as {
		githubId: string;
		lastReadAt: string | null;
		lastArchivedAt: string | null;
		readerUsername: string | null;
		selection: string;
		active: boolean;
		onEntryCounts?: (counts: WhatsNewEntryCounts | null) => void;
		richSummaryTrigger?: number;
		onReady?: (timestamp: string) => void;
		onShowFullView?: () => void;
	}[],
	actionBar: null as {
		onShowActivitySummary?: () => void;
		onGenerateDetailedSummary?: () => void;
	} | null,
	seen: null as ((timestamp: string) => Promise<void>) | null,
}));
vi.mock("./DetailActionBar.svelte", () => ({
	default: (_anchor: unknown, props: typeof captures.actionBar) => {
		captures.actionBar = props;
	},
}));
vi.mock("$lib/components/timeline/TimelineThread.svelte", () => ({
	default: (
		_anchor: unknown,
		props: { onUpdateLastSeen: (timestamp: string) => Promise<void> }
	) => {
		captures.seen = props.onUpdateLastSeen;
	},
}));
vi.mock("$lib/components/timeline/WhatsNewReader.svelte", () => ({
	default: (
		_anchor: unknown,
		props: {
			githubId: string;
			lastReadAt: string | null;
			lastArchivedAt: string | null;
			readerUsername: string | null;
			selection: string;
			active: boolean;
			onEntryCounts?: (counts: WhatsNewEntryCounts | null) => void;
			richSummaryTrigger?: number;
			onReady?: (timestamp: string) => void;
			onShowFullView?: () => void;
		}
	) => {
		captures.readers.push(props);
	},
}));
vi.mock("$lib/api/notifications", () => ({
	updateTimelineLastSeen: vi.fn().mockResolvedValue(undefined),
}));

const cutoff = "2026-11-11T08:00:00Z";
const newer = "2026-11-12T08:00:00Z";
const notification: Notification = {
	id: "one",
	githubId: "github-one",
	repoFullName: "esbena/octobud",
	subjectTitle: "Example issue",
	subjectType: "issue",
	reason: "subscribed",
	archived: false,
	isRead: true,
	muted: false,
	starred: false,
	updatedAt: newer,
	labels: [],
	viewIds: [],
};

function openDetail(current: Writable<Notification | null>) {
	const updateNotification = vi.fn();
	const component = render(InlineDetailView, {
		props: { timelineController: createTimelineController() },
		context: new Map([
			[
				"notificationPageController",
				{
					stores: { currentDetailNotification: current },
					actions: { updateNotification, softMarkRead: vi.fn() },
				},
			],
		]),
	});
	return { ...component, updateNotification };
}

beforeEach(() => {
	vi.clearAllMocks();
	captures.readers = [];
	captures.actionBar = null;
	captures.seen = null;
	getNotificationSettingsStore().setWhatsNewDefault(false);
	getNotificationSettingsStore().setWhatsNewBaseline("last_read");
	HTMLElement.prototype.scrollTo = vi.fn();
});
afterEach(cleanup);

it("keeps the first-visit read baseline frozen while using the global catch-up range", async () => {
	getNotificationSettingsStore().setWhatsNewDefault(true);
	const current = writable<Notification | null>(notification);
	const first = openDetail(current);
	expect(screen.getByRole("tab", { name: "What's new" })).toBeInTheDocument();
	expect(screen.getByRole("tabpanel", { name: "What's new" })).toBeInTheDocument();
	expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true");
	await fireEvent.click(screen.getByRole("tab", { name: "What's new" }));
	expect(captures.readers.at(-1)).toMatchObject({
		githubId: notification.githubId,
		lastReadAt: null,
		selection: "last_read",
	});
	getNotificationSettingsStore().setCatchUpSelection("past_7_days");
	await fireEvent.click(screen.getByRole("tab", { name: "Full view" }));
	await fireEvent.click(screen.getByRole("tab", { name: "What's new" }));
	expect(captures.readers.at(-1)?.selection).toBe("past_7_days");
	current.set({ ...notification, timelineLastSeenAt: newer });
	expect(captures.readers.at(-1)?.lastReadAt).toBeNull();
	getNotificationSettingsStore().setCatchUpSelection("last_read");
	first.unmount();
	openDetail(current);
	expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true");
	expect(captures.readers.at(-1)).toMatchObject({
		githubId: notification.githubId,
		lastReadAt: newer,
		selection: "last_read",
	});
});

it("allows first-read guidance to switch to the full view", async () => {
	getNotificationSettingsStore().setWhatsNewDefault(true);
	openDetail(writable<Notification | null>(notification));
	captures.readers.at(-1)?.onShowFullView?.();
	await waitFor(() =>
		expect(screen.getByRole("tab", { name: "Full view" })).toHaveAttribute("aria-selected", "true")
	);
});

it("loads reader counts while Full view is selected and clears them on navigation", async () => {
	const current = writable<Notification | null>(notification);
	openDetail(current);
	expect(captures.readers.at(-1)?.active).toBe(false);
	captures.readers.at(-1)?.onEntryCounts?.({ full: 8, whatsNew: 2 });
	await screen.findByRole("tab", { name: "Full view (8)" });
	const whatsNew = screen.getByRole("tab", { name: "What's new (2)" });
	await fireEvent.click(whatsNew);
	expect(captures.readers.at(-1)?.active).toBe(true);
	expect(captures.readers).toHaveLength(1);
	current.set({ ...notification, id: "two", githubId: "github-two" });
	await screen.findByRole("tab", { name: "Full view" });
	expect(screen.getByRole("tab", { name: "What's new" })).toBeInTheDocument();
});

it("moves detailed summaries to the action bar and makes generated summaries viewable", async () => {
	const current = writable<Notification | null>({ ...notification, timelineLastSeenAt: cutoff });
	openDetail(current);
	expect(captures.actionBar?.onGenerateDetailedSummary).toBeTypeOf("function");
	captures.actionBar?.onGenerateDetailedSummary?.();
	await waitFor(() =>
		expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true")
	);
	expect(captures.readers.at(-1)).toMatchObject({
		selection: "last_read",
		richSummaryTrigger: 1,
	});
	captures.actionBar?.onShowActivitySummary?.();
	await waitFor(() =>
		expect(screen.getByRole("tab", { name: "Full view" })).toHaveAttribute("aria-selected", "true")
	);
});

it.each(["Release", "CheckSuite", "Commit"])(
	"does not offer a detailed summary for unsupported %s notifications",
	(subjectType) => {
		openDetail(writable<Notification | null>({ ...notification, subjectType }));
		expect(captures.actionBar?.onGenerateDetailedSummary).toBeUndefined();
		expect(screen.queryByRole("tab", { name: "What's new" })).not.toBeInTheDocument();
	}
);

it("keeps the opening cutoff across seen updates and tab switches", async () => {
	const current = writable<Notification | null>({ ...notification, timelineLastSeenAt: cutoff });
	openDetail(current);
	current.set({ ...notification, timelineLastSeenAt: newer });
	await fireEvent.click(screen.getByRole("tab", { name: "What's new" }));
	await fireEvent.click(screen.getByRole("tab", { name: "Full view" }));
	await fireEvent.click(screen.getByRole("tab", { name: "What's new" }));
	expect(captures.readers.map((reader) => reader.lastReadAt)).toEqual([cutoff]);
});

it("opens What's new on the archive default and does not advance the read cursor", async () => {
	getNotificationSettingsStore().setWhatsNewDefault(true);
	getNotificationSettingsStore().setWhatsNewBaseline("last_archive");
	openDetail(writable<Notification | null>(notification));
	expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true");
	expect(captures.readers.at(-1)).toMatchObject({
		lastArchivedAt: null,
		selection: "last_archive",
		onReady: undefined,
	});
});

it("opens What's new on the last-comment default without a read checkpoint", async () => {
	getNotificationSettingsStore().setWhatsNewDefault(true);
	getNotificationSettingsStore().setWhatsNewBaseline("my_last_comment");
	openDetail(writable<Notification | null>(notification));
	expect(screen.getByRole("tab", { name: "What's new" })).toHaveAttribute("aria-selected", "true");
	expect(captures.readers.at(-1)).toMatchObject({
		selection: "my_last_comment",
		onReady: undefined,
	});
});

it("ignores a seen-write response after navigating to another notification", async () => {
	let complete!: () => void;
	vi.mocked(updateTimelineLastSeen).mockImplementationOnce(
		() =>
			new Promise<void>((resolve) => {
				complete = resolve;
			})
	);
	const current = writable<Notification | null>({ ...notification, timelineLastSeenAt: cutoff });
	const { updateNotification } = openDetail(current);
	const pending = captures.seen!(newer);
	current.set({ ...notification, id: "two", githubId: "github-two", timelineLastSeenAt: cutoff });
	await fireEvent.click(screen.getByRole("tab", { name: "Full view" }));
	complete();
	await pending;
	expect(updateNotification).not.toHaveBeenCalled();
});

it("ignores an old reader callback even after reopening the same notification", async () => {
	const current = writable<Notification | null>({ ...notification, timelineLastSeenAt: cutoff });
	openDetail(current);
	const oldCallback = captures.seen!;
	current.set({ ...notification, id: "two", githubId: "github-two", timelineLastSeenAt: cutoff });
	await fireEvent.click(screen.getByRole("tab", { name: "Full view" }));
	current.set({ ...notification, timelineLastSeenAt: cutoff });
	await fireEvent.click(screen.getByRole("tab", { name: "Full view" }));
	await oldCallback(newer);
	expect(updateTimelineLastSeen).not.toHaveBeenCalled();
});

it("does not overwrite a newer seen timestamp when an earlier write finishes", async () => {
	let complete!: () => void;
	vi.mocked(updateTimelineLastSeen).mockImplementationOnce(
		() =>
			new Promise<void>((resolve) => {
				complete = resolve;
			})
	);
	const current = writable<Notification | null>({ ...notification, timelineLastSeenAt: cutoff });
	const { updateNotification } = openDetail(current);
	const pending = captures.seen!("2026-11-11T12:00:00Z");
	current.set({ ...notification, timelineLastSeenAt: newer });
	complete();
	await pending;
	expect(updateNotification).not.toHaveBeenCalled();
});

it("advances seen within the same millisecond without losing source precision", async () => {
	const previous = "2026-11-12T08:00:00.000000001Z";
	const candidate = "2026-11-12T08:00:00.000000002Z";
	const current = writable<Notification | null>({ ...notification, timelineLastSeenAt: previous });
	const { updateNotification } = openDetail(current);
	await captures.seen!(candidate);
	expect(updateTimelineLastSeen).toHaveBeenCalledWith(notification.githubId, candidate);
	expect(updateNotification).toHaveBeenCalledWith({
		...notification,
		timelineLastSeenAt: candidate,
	});
});
