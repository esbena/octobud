import type {
	WhatsNewAuthorType,
	WhatsNewEntry,
	WhatsNewEvidence,
	WhatsNewFeedItem,
	WhatsNewReaderModel,
	WhatsNewSnapshot,
	WhatsNewThread,
	WhatsNewThreadItem,
} from "$lib/types/whatsNew";
import type { WhatsNewSelection } from "$lib/utils/whatsNewCutoff";
import { classifyWhatsNewAuthor } from "./whatsNewAuthors";

function timestampValue(timestamp: string | undefined): bigint | undefined {
	if (!timestamp) return undefined;
	const milliseconds = Date.parse(timestamp);
	if (!Number.isFinite(milliseconds)) return undefined;
	const fraction = timestamp.match(/\.(\d+)(?:Z|[+-]\d{2}:\d{2})$/)?.[1] ?? "";
	return BigInt(milliseconds) * 1_000_000n + BigInt(fraction.slice(3, 9).padEnd(6, "0"));
}

export function compareWhatsNewTimestamps(first: string, second: string): number | undefined {
	const a = timestampValue(first);
	const b = timestampValue(second);
	if (a === undefined || b === undefined) return undefined;
	return a === b ? 0 : a < b ? -1 : 1;
}

function compareEntries(a: WhatsNewEntry, b: WhatsNewEntry, byActivity = false): number {
	const first = byActivity
		? (timestampValue(a.activityAt) ?? timestampValue(a.createdAt))
		: (timestampValue(a.createdAt) ?? timestampValue(a.activityAt));
	const second = byActivity
		? (timestampValue(b.activityAt) ?? timestampValue(b.createdAt))
		: (timestampValue(b.createdAt) ?? timestampValue(b.activityAt));
	if (first === undefined && second !== undefined) return -1;
	if (first !== undefined && second === undefined) return 1;
	if (first !== undefined && second !== undefined && first !== second)
		return first < second ? -1 : 1;
	return a.id.localeCompare(b.id);
}

function toEntry(evidence: WhatsNewEvidence, isNew: boolean): WhatsNewEntry {
	return {
		id: evidence.id,
		eventKind: evidence.kind,
		author: {
			login: evidence.actor ?? "",
			type: classifyWhatsNewAuthor(evidence.actor, evidence.actorType),
		},
		body: evidence.body ?? "",
		createdAt: evidence.createdAt,
		updatedAt: evidence.updatedAt,
		activityAt: evidence.at,
		htmlUrl: evidence.url,
		change: evidence.change,
		isNew,
		state: evidence.state,
		detail: evidence.detail,
		path: evidence.path,
		diffHunk: evidence.diffHunk,
		outdated: evidence.outdated,
		message: evidence.message,
	};
}

function latestEvidenceTimestamp(evidence: WhatsNewEvidence): bigint | undefined {
	let latest: bigint | undefined;
	for (const timestamp of [evidence.at, evidence.updatedAt, evidence.createdAt]) {
		const value = timestampValue(timestamp);
		if (value !== undefined && (latest === undefined || value > latest)) latest = value;
	}
	return latest;
}

export function isWhatsNewComment(kind: string, body?: string): boolean {
	if (["commented", "comment", "review_comment", "discussion_reply"].includes(kind)) {
		return true;
	}
	return kind === "reviewed" && Boolean(body?.trim());
}

function snapshotSinceLastComment(
	snapshot: WhatsNewSnapshot,
	readerUsername: string | null
): { snapshot: WhatsNewSnapshot; baselineMessage?: string } {
	const username = readerUsername?.trim().toLowerCase();
	if (!username) {
		return {
			snapshot,
			baselineMessage: "Couldn't identify your GitHub account; showing all available activity.",
		};
	}

	const evidence = [...snapshot.olderContext, ...snapshot.newActivity];
	const anchor = evidence
		.filter(
			(item) =>
				isWhatsNewComment(item.kind, item.body) &&
				item.actor?.toLowerCase() === username &&
				(item.createdAt || item.at)
		)
		.reduce<WhatsNewEvidence | undefined>((latest, item) => {
			if (!latest) return item;
			const itemTime = timestampValue(item.createdAt ?? item.at);
			const latestTime = timestampValue(latest.createdAt ?? latest.at);
			return itemTime !== undefined && (latestTime === undefined || itemTime > latestTime)
				? item
				: latest;
		}, undefined);

	if (!anchor) {
		return {
			snapshot,
			baselineMessage: "No previous comment found; showing all available activity.",
		};
	}
	const anchorTimestamp = anchor.createdAt ?? anchor.at;
	if (!anchorTimestamp) {
		return {
			snapshot,
			baselineMessage: "Couldn't determine your last comment time; showing all available activity.",
		};
	}

	const anchorTime = timestampValue(anchorTimestamp);
	if (anchorTime === undefined) {
		return {
			snapshot,
			baselineMessage: "Couldn't determine your last comment time; showing all available activity.",
		};
	}
	const allById = new Map(evidence.map((item) => [item.id, item]));
	const newActivity = evidence
		.filter((item) => {
			if (item.id === anchor.id) return false;
			const activityTime = latestEvidenceTimestamp(item);
			// GitHub timestamps can tie, so keep same-time activity rather than risk hiding it.
			return activityTime === undefined || activityTime >= anchorTime;
		})
		.map((item) => {
			const createdTime = timestampValue(item.createdAt);
			if (createdTime !== undefined && createdTime >= anchorTime) {
				return { ...item, change: "created", at: item.createdAt };
			}
			const updatedTime = timestampValue(item.updatedAt);
			if (updatedTime !== undefined && updatedTime >= anchorTime) {
				return { ...item, change: "edited", at: item.updatedAt };
			}
			return item;
		});
	const newIDs = new Set(newActivity.map((item) => item.id));
	const contextIDs = new Set([anchor.id]);
	for (const item of newActivity) {
		let parentID = item.parentId;
		const visitedParents = new Set([item.id]);
		while (parentID && !visitedParents.has(parentID)) {
			visitedParents.add(parentID);
			contextIDs.add(parentID);
			parentID = allById.get(parentID)?.parentId;
		}
	}

	return {
		snapshot: {
			...snapshot,
			since: anchorTimestamp,
			newActivity,
			olderContext: evidence.filter((item) => contextIDs.has(item.id) && !newIDs.has(item.id)),
		},
	};
}

function buildThread(
	id: string,
	entries: WhatsNewEntry[],
	authorType: Exclude<WhatsNewAuthorType, "unknown">
): WhatsNewThread {
	entries.sort(compareEntries);
	const opening = entries.find((entry) => entry.id === id);
	if (!opening) throw new Error(`Incomplete activity: missing thread opening ${id}`);
	const oldEntries = entries.filter((entry) => !entry.isNew);
	const lastOld = oldEntries[oldEntries.length - 1];
	const selected = new Set(
		[opening, ...(lastOld ? [lastOld] : []), ...entries.filter((entry) => entry.isNew)].map(
			(entry) => entry.id
		)
	);
	const items: WhatsNewThreadItem[] = [];
	let omitted = 0;
	for (const entry of entries) {
		if (!selected.has(entry.id)) {
			omitted++;
			continue;
		}
		if (omitted > 0) {
			items.push({ kind: "omitted", count: omitted });
			omitted = 0;
		}
		items.push({ kind: "entry", entry });
	}
	if (omitted > 0) items.push({ kind: "omitted", count: omitted });
	return {
		id,
		path: opening.path,
		htmlUrl: opening.htmlUrl,
		authorType,
		entries: items,
	};
}

function isRelevantActivity(entry: WhatsNewEntry): boolean {
	if (["opened", "merged", "closed", "reopened"].includes(entry.eventKind)) return true;
	return (
		entry.eventKind === "reviewed" &&
		["APPROVED", "CHANGES_REQUESTED", "DISMISSED"].includes(entry.state?.toUpperCase() ?? "")
	);
}

function latestNewThreadEntry(thread: WhatsNewThread): WhatsNewEntry {
	const entry = thread.entries.reduce<WhatsNewEntry | undefined>((latest, item) => {
		if (item.kind !== "entry" || !item.entry.isNew) return latest;
		return !latest || compareEntries(item.entry, latest, true) > 0 ? item.entry : latest;
	}, undefined);
	if (!entry) throw new Error(`Incomplete activity: thread ${thread.id} has no new entries`);
	return entry;
}

function compareFeedItems(a: WhatsNewFeedItem, b: WhatsNewFeedItem): number {
	const first = a.kind === "entry" ? a.entry : latestNewThreadEntry(a.thread);
	const second = b.kind === "entry" ? b.entry : latestNewThreadEntry(b.thread);
	return compareEntries(first, second, true);
}

/** Select context without changing the collector's strict cutoff partition. */
export function buildWhatsNewReaderModel(
	sourceSnapshot: WhatsNewSnapshot,
	selection: WhatsNewSelection = "last_read",
	readerUsername: string | null = null
): WhatsNewReaderModel {
	const serverRebased = selection === "my_last_comment" && sourceSnapshot.selection === selection;
	const { snapshot, baselineMessage } =
		selection === "my_last_comment" && !serverRebased
			? snapshotSinceLastComment(sourceSnapshot, readerUsername)
			: { snapshot: sourceSnapshot, baselineMessage: sourceSnapshot.baselineMessage };
	const model: WhatsNewReaderModel = {
		counts: { full: sourceSnapshot.totalEntries, whatsNew: 0 },
		cutoff: snapshot.since,
		baselineMessage,
		warnings: [...snapshot.warnings],
		humanFeed: [],
		botFeed: [],
		humanComments: [],
		humanThreads: [],
		humanActivity: [],
		nonHumanComments: [],
		nonHumanThreads: [],
		nonHumanActivity: [],
		unknownComments: [],
		unknownThreads: [],
		unknownActivity: [],
	};
	const evidence = new Map<string, WhatsNewEvidence>();
	for (const item of [...snapshot.olderContext, ...snapshot.newActivity])
		evidence.set(item.id, item);
	let latestTime: bigint | undefined;
	for (const item of evidence.values()) {
		if (item.kind === "committed") continue;
		for (const timestamp of [item.at, item.updatedAt, item.createdAt]) {
			if (!timestamp) continue;
			const time = timestampValue(timestamp);
			if (time !== undefined && (latestTime === undefined || time > latestTime)) {
				latestTime = time;
				model.latestActivityAt = timestamp;
			}
		}
	}
	const newIds = new Set(snapshot.newActivity.map((item) => item.id));
	const groups = new Map<string, WhatsNewEntry[]>();
	const oldHumanComments: WhatsNewEntry[] = [];
	const oldBotComments: WhatsNewEntry[] = [];
	for (const item of evidence.values()) {
		let root = item;
		const visited = new Set<string>();
		while (root.parentId) {
			if (visited.has(root.id)) throw new Error("Incomplete activity: cyclic reply relationship");
			visited.add(root.id);
			const parent = evidence.get(root.parentId);
			if (!parent) throw new Error(`Incomplete activity: missing reply parent ${root.parentId}`);
			root = parent;
		}
		const rootId = item.rootId ?? root.id;
		if (!evidence.has(rootId)) throw new Error(`Incomplete activity: missing thread ${rootId}`);
		const entries = groups.get(rootId) ?? [];
		entries.push(toEntry(item, newIds.has(item.id)));
		groups.set(rootId, entries);
	}
	for (const [id, entries] of groups) {
		if (entries.length > 1 || entries.some((entry) => entry.eventKind === "review_comment")) {
			const newAuthorTypes = new Set<Exclude<WhatsNewAuthorType, "unknown">>(
				entries
					.filter((entry) => entry.isNew)
					.map((entry) => (entry.author.type === "bot" ? "bot" : "human"))
			);
			for (const authorType of newAuthorTypes) {
				const threadEntries = entries.filter(
					(entry) => !entry.isNew || (entry.author.type === "bot" ? "bot" : "human") === authorType
				);
				const opening = entries.find((entry) => entry.id === id);
				if (opening && !threadEntries.some((entry) => entry.id === id)) {
					threadEntries.push({ ...opening, isNew: false });
				}
				const thread = buildThread(id, threadEntries, authorType);
				(authorType === "bot" ? model.nonHumanThreads : model.humanThreads).push(thread);
			}
			continue;
		}
		const entry = entries[0];
		const isComment = isWhatsNewComment(entry.eventKind, entry.body);
		if (!entry.isNew && isComment) {
			(entry.author.type === "bot" ? oldBotComments : oldHumanComments).push(entry);
			continue;
		}
		if (!isComment && !isRelevantActivity(entry)) continue;
		if (!entry.isNew && isRelevantActivity(entry)) continue;
		const target =
			entry.author.type === "bot"
				? isComment
					? model.nonHumanComments
					: model.nonHumanActivity
				: isComment
					? model.humanComments
					: model.humanActivity;
		target.push(entry);
	}
	for (const entries of [
		model.humanComments,
		model.humanActivity,
		model.nonHumanComments,
		model.nonHumanActivity,
		model.unknownComments,
		model.unknownActivity,
	])
		entries.sort((a, b) => compareEntries(a, b, true));
	for (const [comments, context] of [
		[model.humanComments, oldHumanComments],
		[model.nonHumanComments, oldBotComments],
	]) {
		if (comments.length === 0 || context.length === 0) continue;
		context.sort(compareEntries);
		comments.unshift(context[context.length - 1]);
	}
	const threadOrder = (a: WhatsNewThread, b: WhatsNewThread) =>
		compareEntries(latestNewThreadEntry(a), latestNewThreadEntry(b), true);
	for (const threads of [model.humanThreads, model.nonHumanThreads, model.unknownThreads]) {
		threads.sort(threadOrder);
	}
	model.humanFeed = [
		...model.humanComments.map((entry) => ({ kind: "entry" as const, entry })),
		...model.humanActivity.map((entry) => ({ kind: "entry" as const, entry })),
		...model.humanThreads.map((thread) => ({ kind: "thread" as const, thread })),
	].sort(compareFeedItems);
	model.botFeed = [
		...model.nonHumanComments.map((entry) => ({ kind: "entry" as const, entry })),
		...model.nonHumanActivity.map((entry) => ({ kind: "entry" as const, entry })),
		...model.nonHumanThreads.map((thread) => ({ kind: "thread" as const, thread })),
	].sort(compareFeedItems);
	model.counts.whatsNew = new Set(
		[...model.humanFeed, ...model.botFeed].flatMap((item) =>
			item.kind === "entry"
				? [item.entry.id]
				: item.thread.entries.flatMap((entry) => (entry.kind === "entry" ? [entry.entry.id] : []))
		)
	).size;
	return model;
}
