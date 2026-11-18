<script lang="ts">
	import type { TimelineReviewComment } from "$lib/api/types";
	import type { WhatsNewThread } from "$lib/types/whatsNew";
	import ReviewComments from "./ReviewComments.svelte";
	import WhatsNewEntry from "./WhatsNewEntry.svelte";

	export let thread: WhatsNewThread;

	$: entries = thread.entries.flatMap((item) => (item.kind === "entry" ? [item.entry] : []));
	$: opening = entries.find((entry) => entry.id === thread.id);
	$: isReviewThread =
		!!opening &&
		entries.length > 0 &&
		entries.every((entry) => entry.eventKind === "review_comment");
	$: reviewComments =
		isReviewThread && opening
			? [
					{
						id: opening.id,
						body: opening.body,
						path: opening.path ?? thread.path ?? "",
						diffHunk: opening.diffHunk,
						outdated: opening.outdated,
						edited: opening.change === "edited",
						author: { login: opening.author.login || "Unknown author", avatarUrl: "" },
						createdAt: opening.createdAt,
						htmlUrl: opening.htmlUrl,
						replies: entries
							.filter((entry) => entry.id !== opening.id)
							.map(
								(entry): TimelineReviewComment => ({
									id: entry.id,
									body: entry.body,
									path: entry.path ?? thread.path ?? "",
									diffHunk: entry.diffHunk,
									outdated: entry.outdated,
									edited: entry.change === "edited",
									author: { login: entry.author.login || "Unknown author", avatarUrl: "" },
									createdAt: entry.createdAt,
									htmlUrl: entry.htmlUrl,
								})
							),
					},
				]
			: [];
	$: omittedBeforeCommentId = markerTargetId(thread.entries, "omitted");
	$: omittedCount = thread.entries.reduce(
		(count, item) => count + (item.kind === "omitted" ? item.count : 0),
		0
	);
	$: contextCommentIds = entries.filter((entry) => !entry.isNew).map((entry) => entry.id);

	function markerTargetId(items: WhatsNewThread["entries"], kind: "omitted"): string | undefined {
		const index = items.findIndex((item) => item.kind === kind);
		if (index < 0) return undefined;
		for (const item of items.slice(index + 1)) {
			if (item.kind === "entry") return item.entry.id;
		}
		return undefined;
	}
</script>

{#if isReviewThread && opening}
	<ReviewComments
		comments={reviewComments}
		expandedByDefault={true}
		showAllReplies={true}
		{contextCommentIds}
		{omittedCount}
		{omittedBeforeCommentId}
		highlightNewActivity={true}
	/>
{:else}
	<section
		class="rounded-lg border border-gray-200 dark:border-gray-800 overflow-hidden"
		aria-label={thread.path ? `Thread in ${thread.path}` : "Conversation thread"}
	>
		<header
			class="flex items-center justify-between gap-3 px-4 py-3 bg-gray-50 dark:bg-gray-800/50 border-b border-gray-200 dark:border-gray-800 text-sm"
		>
			<h4 class="font-semibold text-gray-900 dark:text-gray-200 break-all">
				{thread.path || "Conversation thread"}
			</h4>
			{#if thread.htmlUrl}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a
					href={thread.htmlUrl}
					target="_blank"
					rel="noreferrer"
					class="shrink-0 text-xs text-indigo-600 dark:text-indigo-300 hover:underline"
				>
					Open thread on GitHub
				</a>
			{/if}
		</header>
		<div class="p-4 space-y-3">
			{#each thread.entries as item, index (item.kind === "entry" ? item.entry.id : `${item.kind}-${index}`)}
				{#if item.kind === "entry"}
					<WhatsNewEntry entry={item.entry} />
				{:else if item.kind === "omitted"}
					<p class="text-center text-xs text-gray-500 dark:text-gray-400 py-1">
						{item.count} earlier {item.count === 1 ? "comment" : "comments"} omitted
					</p>
				{/if}
			{/each}
		</div>
	</section>
{/if}
