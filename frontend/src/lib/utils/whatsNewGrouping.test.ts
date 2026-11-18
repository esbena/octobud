import { describe, expect, it } from "vitest";
import { buildWhatsNewReaderModel, compareWhatsNewTimestamps } from "./whatsNewGrouping";
import type { WhatsNewEvidence, WhatsNewSnapshot, WhatsNewThread } from "$lib/types/whatsNew";

const cutoff = "2026-11-10T12:00:00Z";
function event(id: string, extra: Partial<WhatsNewEvidence> = {}): WhatsNewEvidence {
	return {
		id,
		kind: "commented",
		change: "created",
		createdAt: "2026-11-11T12:00:00Z",
		at: "2026-11-11T12:00:00Z",
		actor: "reader",
		actorType: "human",
		url: `https://github.com/o/r/issues/1#${id}`,
		body: id,
		...extra,
	};
}
function snapshot(
	newActivity: WhatsNewEvidence[],
	olderContext: WhatsNewEvidence[] = []
): WhatsNewSnapshot {
	return {
		notificationId: "1",
		title: "Topic",
		since: cutoff,
		fetchedAt: "2026-11-12T12:00:00Z",
		totalEntries: new Set([...newActivity, ...olderContext].map((item) => item.id)).size,
		newActivity,
		olderContext,
		warnings: [],
	};
}
function entryIds(thread: WhatsNewThread) {
	return thread.entries.flatMap((item) => (item.kind === "entry" ? [item.entry.id] : []));
}
function old(id: string, hour: number, extra: Partial<WhatsNewEvidence> = {}) {
	return event(id, {
		createdAt: `2026-11-09T${hour}:00:00Z`,
		at: `2026-11-09T${hour}:00:00Z`,
		change: "context",
		...extra,
	});
}

describe("What's new reader grouping", () => {
	it("includes opening posts only when they are new within the selected window", () => {
		const opening = event("opening", { kind: "opened", body: "Original description" });
		const fresh = buildWhatsNewReaderModel(snapshot([opening]));
		expect(fresh.humanFeed).toMatchObject([
			{ kind: "entry", entry: { id: "opening", body: "Original description", isNew: true } },
		]);
		const later = buildWhatsNewReaderModel(
			snapshot([event("reply")], [old("opening", 10, { kind: "opened" })])
		);
		expect(later.humanFeed).toMatchObject([{ kind: "entry", entry: { id: "reply" } }]);
		expect(later.humanComments.map((entry) => entry.id)).toEqual(["reply"]);
	});

	it("compares precise timestamps for shared cutoff and seen guards", () => {
		const first = "2026-11-11T13:00:00.123456788Z";
		const second = "2026-11-11T14:00:00.123456789+01:00";
		expect(compareWhatsNewTimestamps(first, second)).toBe(-1);
		expect(compareWhatsNewTimestamps(second, first)).toBe(1);
		expect(compareWhatsNewTimestamps(first, "2026-11-11T14:00:00.123456788+01:00")).toBe(0);
		expect(compareWhatsNewTimestamps("invalid", first)).toBeUndefined();
		expect(compareWhatsNewTimestamps(first, "")).toBeUndefined();
	});

	it("starts after the latest authored comment and keeps its thread context", () => {
		const root = old("root", 10, { kind: "review_comment", actor: "teammate" });
		const previous = old("previous", 11, {
			kind: "review_comment",
			actor: "reader",
			parentId: root.id,
			rootId: root.id,
		});
		const anchor = event("anchor", {
			kind: "review_comment",
			actor: "reader",
			createdAt: "2026-11-10T12:00:00Z",
			at: "2026-11-10T12:00:00Z",
			parentId: root.id,
			rootId: root.id,
		});
		const reply = event("reply", {
			kind: "review_comment",
			actor: "teammate",
			createdAt: "2026-11-11T12:00:00Z",
			at: "2026-11-11T12:00:00Z",
			parentId: anchor.id,
			rootId: root.id,
		});
		const simultaneous = event("simultaneous", {
			createdAt: anchor.createdAt,
			at: anchor.at,
			actor: "teammate",
		});
		const input = {
			...snapshot([root, previous, anchor, reply, simultaneous]),
			since: "1970-01-01T00:00:00.000Z",
		};

		const model = buildWhatsNewReaderModel(input, "my_last_comment", "Reader");

		expect(model.cutoff).toBe(anchor.createdAt);
		expect(model.humanThreads).toHaveLength(1);
		expect(entryIds(model.humanThreads[0])).toEqual(["root", "anchor", "reply"]);
		expect(model.humanThreads[0].entries.map((item) => item.kind)).toEqual([
			"entry",
			"entry",
			"entry",
		]);
		expect(model.humanThreads[0].entries).toMatchObject([
			{ kind: "entry", entry: { id: "root", isNew: false } },
			{ kind: "entry", entry: { id: "anchor", isNew: false } },
			{ kind: "entry", entry: { id: "reply", isNew: true } },
		]);
		expect(model.humanComments.map((entry) => [entry.id, entry.isNew])).toEqual([
			["simultaneous", true],
		]);
		expect(model.baselineMessage).toBeUndefined();
	});

	it("reclassifies an opening-post edit after the last comment and orders it by edit time", () => {
		const openingAt = "2026-11-10T12:00:00Z";
		const commentedAt = "2026-11-11T12:00:00Z";
		const repliedAt = "2026-11-11T13:00:00Z";
		const editedAt = "2026-11-11T14:00:00Z";
		const model = buildWhatsNewReaderModel(
			{
				...snapshot([
					event("opening", {
						kind: "opened",
						actor: "author",
						createdAt: openingAt,
						at: openingAt,
						updatedAt: editedAt,
					}),
					event("anchor", { createdAt: commentedAt, at: commentedAt }),
					event("reply", { actor: "teammate", createdAt: repliedAt, at: repliedAt }),
				]),
				since: "1970-01-01T00:00:00.000Z",
			},
			"my_last_comment",
			"reader"
		);
		expect(model.cutoff).toBe(commentedAt);
		expect(model.humanActivity).toMatchObject([
			{ id: "opening", change: "edited", activityAt: editedAt },
		]);
		expect(
			model.humanFeed.flatMap((item) =>
				item.kind === "entry" && item.entry.isNew ? [item.entry.id] : []
			)
		).toEqual(["reply", "opening"]);
	});

	it("shows all available activity when the reader has no prior comment", () => {
		const input = { ...snapshot([event("comment")]), since: "1970-01-01T00:00:00.000Z" };
		const model = buildWhatsNewReaderModel(input, "my_last_comment", "someone-else");
		expect(model.humanComments.map((entry) => entry.id)).toEqual(["comment"]);
		expect(model.baselineMessage).toBe(
			"No previous comment found; showing all available activity."
		);
	});

	it("keeps top-level comments separate from chronological threads and lifecycle activity", () => {
		const root = old("root", 10);
		const newer = event("reply", { parentId: root.id, rootId: root.id, kind: "discussion_reply" });
		const result = buildWhatsNewReaderModel(
			snapshot([event("closed", { kind: "closed" }), newer, event("standalone")], [root])
		);
		expect(result.humanComments.map((entry) => entry.id)).toEqual(["standalone"]);
		expect(result.humanActivity.map((entry) => entry.id)).toEqual(["closed"]);
		expect(entryIds(result.humanThreads[0])).toEqual(["root", "reply"]);
		expect(result.humanThreads[0].entries.map((item) => item.kind)).toEqual(["entry", "entry"]);
	});

	it("prepends exactly one latest old top-level context comment to each active author stream", () => {
		const model = buildWhatsNewReaderModel(
			snapshot(
				[event("new-human"), event("new-bot", { actorType: "bot" })],
				[
					old("early-human", 10),
					old("latest-human", 12),
					old("early-bot", 10, { actorType: "bot" }),
					old("latest-bot", 11, { actorType: "bot" }),
					old("lifecycle", 13, { kind: "closed" }),
				]
			)
		);
		expect(model.humanComments.map((entry) => [entry.id, entry.isNew])).toEqual([
			["latest-human", false],
			["new-human", true],
		]);
		expect(model.nonHumanComments.map((entry) => [entry.id, entry.isNew])).toEqual([
			["latest-bot", false],
			["new-bot", true],
		]);
	});

	it("does not create context-only streams or expose old replies as top-level comment context", () => {
		const root = old("review-root", 12, { kind: "review_comment" });
		const input = snapshot(
			[event("new-human")],
			[
				old("bot-only", 11, { actorType: "bot" }),
				old("standalone", 10),
				root,
				old("reply", 13, { kind: "review_comment", parentId: root.id, rootId: root.id }),
			]
		);
		const model = buildWhatsNewReaderModel(input);
		expect(model.humanComments.map((entry) => entry.id)).toEqual(["standalone", "new-human"]);
		expect(model.nonHumanComments).toEqual([]);
		expect(buildWhatsNewReaderModel(snapshot([], input.olderContext)).humanComments).toEqual([]);
	});

	it("keeps old stream context before edited comments without duplicating old versions", () => {
		const edited = old("edited", 10, { change: "edited", updatedAt: "2026-11-11T12:00:00Z" });
		const model = buildWhatsNewReaderModel(snapshot([edited], [edited, old("last-old", 12)]));
		expect(model.humanComments.map((entry) => [entry.id, entry.isNew])).toEqual([
			["last-old", false],
			["edited", true],
		]);
	});

	it("orders top-level edits by activity time rather than creation time", () => {
		const edited = old("edited", 10, {
			change: "edited",
			at: "2026-11-11T13:00:00Z",
			updatedAt: "2026-11-11T13:00:00Z",
		});
		const model = buildWhatsNewReaderModel(snapshot([edited, event("created")]));
		expect(model.humanComments.map((entry) => entry.id)).toEqual(["created", "edited"]);
		expect(model.humanFeed.map((item) => item.kind === "entry" && item.entry.id)).toEqual([
			"created",
			"edited",
		]);
	});

	it("positions threads by their latest new activity without moving edited thread openings", () => {
		const root = old("root", 10, {
			kind: "review_comment",
			change: "edited",
			at: "2026-11-11T14:00:00Z",
			updatedAt: "2026-11-11T14:00:00Z",
		});
		const reply = event("reply", {
			kind: "review_comment",
			parentId: root.id,
			rootId: root.id,
		});
		const standalone = event("standalone", {
			createdAt: "2026-11-11T13:00:00Z",
			at: "2026-11-11T13:00:00Z",
		});
		const model = buildWhatsNewReaderModel(snapshot([root, standalone, reply]));
		expect(entryIds(model.humanThreads[0])).toEqual(["root", "reply"]);
		expect(
			model.humanFeed.map((item) => (item.kind === "entry" ? item.entry.id : item.thread.id))
		).toEqual(["standalone", "root"]);
	});

	it("deduplicates opening and last-old context, retains omission counts and all new replies", () => {
		const root = old("root", 10, { kind: "review_comment" });
		const reply = (id: string, hour: number) =>
			old(id, hour, {
				kind: "review_comment",
				parentId: root.id,
				rootId: root.id,
			});
		const fresh = event("fresh", { kind: "review_comment", parentId: root.id, rootId: root.id });
		const model = buildWhatsNewReaderModel(
			snapshot([fresh], [root, reply("middle", 11), reply("last", 12), fresh])
		);
		expect(entryIds(model.humanThreads[0])).toEqual(["root", "last", "fresh"]);
		expect(model.humanThreads[0].entries).toContainEqual({ kind: "omitted", count: 1 });
		const minimal = buildWhatsNewReaderModel(snapshot([fresh], [root]));
		expect(entryIds(minimal.humanThreads[0])).toEqual(["root", "fresh"]);
	});

	it("splits mixed-author threads into their respective activity streams", () => {
		const root = old("root", 10, { actorType: "bot" });
		const human = old("human", 11, { parentId: root.id, rootId: root.id });
		const bot = old("bot", 12, { parentId: root.id, rootId: root.id, actorType: "bot" });
		const freshHuman = event("fresh-human", { parentId: root.id, rootId: root.id });
		const freshBot = event("fresh-bot", {
			parentId: root.id,
			rootId: root.id,
			actorType: "bot",
		});
		const model = buildWhatsNewReaderModel(snapshot([freshHuman, freshBot], [root, human, bot]));
		expect(model.humanThreads).toHaveLength(1);
		expect(model.nonHumanThreads).toHaveLength(1);
		expect(entryIds(model.humanThreads[0])).toContain("fresh-human");
		expect(entryIds(model.humanThreads[0])).not.toContain("fresh-bot");
		expect(entryIds(model.nonHumanThreads[0])).toContain("fresh-bot");
		expect(entryIds(model.nonHumanThreads[0])).not.toContain("fresh-human");
		expect(model.counts).toEqual({ full: 5, whatsNew: 4 });
	});

	it("keeps unknown authors in the human stream without changing their identity", () => {
		const model = buildWhatsNewReaderModel(
			snapshot(
				[
					event("unknown", { actorType: undefined, actor: undefined }),
					event("bot", { actorType: "bot" }),
					event("unknown-reply", {
						parentId: "bot-root",
						rootId: "bot-root",
						actorType: "unknown",
						actor: undefined,
					}),
				],
				[old("bot-root", 10, { actorType: "bot" })]
			)
		);
		expect(model.humanComments[0]).toMatchObject({ id: "unknown", author: { type: "unknown" } });
		expect(model.nonHumanComments[0].id).toBe("bot");
		expect(model.humanThreads[0].authorType).toBe("human");
		const unknownReply = model.humanThreads[0].entries.find(
			(item) => item.kind === "entry" && item.entry.id === "unknown-reply"
		);
		expect(unknownReply).toMatchObject({
			kind: "entry",
			entry: { author: { type: "unknown" } },
		});
		expect(model.unknownThreads).toHaveLength(0);
	});

	it("uses GitHub bot identity and the standard bot suffix without Copilot-specific rules", () => {
		const model = buildWhatsNewReaderModel(
			snapshot([
				event("copilot", { actor: "Copilot", actorType: "bot" }),
				event("app", { actor: "custom-app[bot]", actorType: undefined }),
				event("human", { actor: "participant", actorType: "human" }),
				event("missing", { actor: undefined, actorType: "bot" }),
			])
		);
		expect(model.nonHumanComments.map((entry) => entry.id)).toEqual(["app", "copilot", "missing"]);
		expect(model.humanComments.map((entry) => entry.id)).toEqual(["human"]);
	});

	it("keeps edits new without duplicating or moving their original comment", () => {
		const opening = old("opening", 10, { change: "edited", updatedAt: "2026-11-11T12:00:00Z" });
		const last = old("last", 11, { parentId: opening.id, rootId: opening.id });
		const model = buildWhatsNewReaderModel(snapshot([opening], [opening, last]));
		const thread = model.humanThreads[0];
		expect(entryIds(thread)).toEqual(["opening", "last"]);
		expect(thread.entries[0]).toMatchObject({
			kind: "entry",
			entry: { isNew: true, change: "edited" },
		});
	});

	it("uses reply roots, not the shared review ID, and retains empty-body formal review events", () => {
		const model = buildWhatsNewReaderModel(
			snapshot([
				event("first", { kind: "review_comment", reviewId: "review" }),
				event("second", { kind: "review_comment", reviewId: "review" }),
				event("review", { kind: "reviewed", body: "", state: "APPROVED" }),
			])
		);
		expect(model.humanThreads.map((thread) => thread.id)).toEqual(["first", "second"]);
		expect(model.humanActivity[0]).toMatchObject({ id: "review", state: "APPROVED" });
	});

	it("excludes lifecycle activity without a timestamp while retaining coverage warnings", () => {
		const input = snapshot(
			[],
			[event("undated", { kind: "closed", at: undefined, createdAt: undefined })]
		);
		input.warnings.push("Lifecycle time unavailable");
		const model = buildWhatsNewReaderModel(input);
		expect(model.humanActivity).toEqual([]);
		expect(model.humanFeed).toEqual([]);
		expect(model.warnings).toEqual(input.warnings);
	});

	it("fails explicitly for missing parents and cycles rather than dropping replies", () => {
		expect(() =>
			buildWhatsNewReaderModel(snapshot([event("orphan", { parentId: "absent" })]))
		).toThrow("missing reply parent");
		expect(() =>
			buildWhatsNewReaderModel(
				snapshot([event("a", { parentId: "b" }), event("b", { parentId: "a" })])
			)
		).toThrow("cyclic reply");
	});

	it("exposes the latest evidenced activity, including edits, not fetch or commit dates", () => {
		const input = snapshot(
			[event("created-and-edited", { updatedAt: "2026-11-11T13:00:00Z" })],
			[
				event("commit", { kind: "committed", at: "2099-01-01T00:00:00Z" }),
				event("invalid", { kind: "closed", at: "bad", createdAt: undefined }),
			]
		);
		expect(buildWhatsNewReaderModel(input).latestActivityAt).toBe("2026-11-11T13:00:00Z");
		expect(buildWhatsNewReaderModel(snapshot([])).latestActivityAt).toBeUndefined();
	});

	it("preserves fractional-second chronology and latest activity across timezones", () => {
		const earlier = "2026-11-11T13:00:00.123456788Z";
		const later = "2026-11-11T14:00:00.123456789+01:00";
		const model = buildWhatsNewReaderModel(
			snapshot([
				event("a-later", { at: later, createdAt: later }),
				event("z-earlier", { at: earlier, createdAt: earlier }),
			])
		);
		expect(model.humanComments.map((entry) => entry.id)).toEqual(["z-earlier", "a-later"]);
		expect(model.latestActivityAt).toBe(later);
	});

	it("filters unrelated activity without mutating source evidence", () => {
		const fresh = event("future-kind", { kind: "new_github_kind", actorType: "bot" });
		const input = snapshot([fresh]);
		const original = structuredClone(input);
		const model = buildWhatsNewReaderModel(input);
		expect(model.nonHumanActivity).toEqual([]);
		expect(model.botFeed).toEqual([]);
		expect(input).toEqual(original);
	});

	it("keeps comments, review decisions, and issue milestones while filtering metadata and git events", () => {
		const input = snapshot([
			event("comment", { createdAt: "2026-11-11T12:00:00Z", at: "2026-11-11T12:00:00Z" }),
			event("approval", {
				kind: "reviewed",
				state: "APPROVED",
				body: "",
				createdAt: "2026-11-11T13:00:00Z",
				at: "2026-11-11T13:00:00Z",
			}),
			event("requested-changes", {
				kind: "reviewed",
				state: "CHANGES_REQUESTED",
				actor: "review-helper[bot]",
				actorType: undefined,
				body: "",
				createdAt: "2026-11-11T14:00:00Z",
				at: "2026-11-11T14:00:00Z",
			}),
			event("merged", {
				kind: "merged",
				createdAt: "2026-11-11T15:00:00Z",
				at: "2026-11-11T15:00:00Z",
			}),
			event("closed", {
				kind: "closed",
				createdAt: "2026-11-11T16:00:00Z",
				at: "2026-11-11T16:00:00Z",
			}),
			event("reopened", {
				kind: "reopened",
				createdAt: "2026-11-11T17:00:00Z",
				at: "2026-11-11T17:00:00Z",
			}),
			event("subscribed", {
				kind: "subscribed",
				createdAt: "2026-11-11T18:00:00Z",
				at: "2026-11-11T18:00:00Z",
			}),
			event("commit", {
				kind: "committed",
				createdAt: "2026-11-11T19:00:00Z",
				at: "2026-11-11T19:00:00Z",
			}),
		]);

		const model = buildWhatsNewReaderModel(input);

		expect(
			model.humanFeed.map((item) => (item.kind === "entry" ? item.entry.id : item.thread.id))
		).toEqual(["comment", "approval", "merged", "closed", "reopened"]);
		expect(
			model.botFeed.map((item) => (item.kind === "entry" ? item.entry.id : item.thread.id))
		).toEqual(["requested-changes"]);
	});
});
