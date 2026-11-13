import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/svelte";
import WhatsNewThread from "./WhatsNewThread.svelte";
import type { WhatsNewEntry } from "$lib/types/whatsNew";

afterEach(cleanup);

const opening: WhatsNewEntry = {
	id: "opening",
	eventKind: "review_comment",
	author: { login: "reviewer", type: "human" },
	body: "Opening question",
	createdAt: "2026-11-11T08:00:00Z",
	htmlUrl: "https://github.com/esbena/octobud/pull/1#discussion_r1",
	change: "created",
	isNew: false,
};

it("renders selected context once, omissions, and the new divider in model order", () => {
	render(WhatsNewThread, {
		thread: {
			id: "opening",
			authorType: "human",
			path: "example.ts",
			htmlUrl: "https://github.com/esbena/octobud/pull/1#discussion_r1",
			entries: [
				{ kind: "entry", entry: opening },
				{ kind: "omitted", count: 3 },
				{ kind: "entry", entry: { ...opening, id: "last-old", body: "Previous reply" } },
				{ kind: "entry", entry: { ...opening, id: "new", body: "**New response**", isNew: true } },
			],
		},
	});
	expect(screen.getAllByText("Opening question")).toHaveLength(1);
	expect(screen.getByText("3 earlier comments omitted")).toBeInTheDocument();
	const cards = screen.getAllByRole("article");
	expect(cards.map((card) => card.textContent)).toEqual([
		expect.stringContaining("Opening question"),
		expect.stringContaining("Previous reply"),
		expect.stringContaining("New response"),
	]);
	expect(
		cards[1].compareDocumentPosition(cards[2]) & Node.DOCUMENT_POSITION_FOLLOWING
	).toBeTruthy();
	expect(cards[2].querySelector("strong")).toHaveTextContent("New response");
	expect(cards[0]).not.toHaveClass("border-indigo-400");
	expect(cards[2]).toHaveClass("border-indigo-400");
	expect(screen.queryByText("New activity")).not.toBeInTheDocument();
	expect(screen.getByRole("link", { name: "View" })).toHaveAttribute("href", opening.htmlUrl);
	expect(screen.getByRole("button", { name: "3 review comments" })).toHaveAttribute(
		"aria-expanded",
		"true"
	);
	expect(screen.queryByRole("heading", { name: "example.ts" })).not.toBeInTheDocument();
});

it("distinguishes edited old comments from context without labelling unresolved authors as bots", () => {
	render(WhatsNewThread, {
		thread: {
			id: "opening",
			authorType: "unknown",
			entries: [
				{
					kind: "entry",
					entry: {
						...opening,
						path: "example.ts",
						isNew: true,
						change: "edited",
						activityAt: "2026-11-12T08:00:00Z",
						author: { login: "", type: "unknown" },
						body: "<script>alert('unsafe')</script>Edited content",
					},
				},
			],
		},
	});
	expect(screen.getByText("Edited")).toBeInTheDocument();
	expect(screen.getByText("Unknown author")).toBeInTheDocument();
	expect(screen.queryByText("Bot")).not.toBeInTheDocument();
	expect(screen.queryByText("Earlier context")).not.toBeInTheDocument();
	expect(screen.getByRole("article").querySelector("script")).toBeNull();
});
