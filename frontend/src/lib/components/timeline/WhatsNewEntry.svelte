<script lang="ts">
	import type { NotificationTimelineItem } from "$lib/api/types";
	import type { WhatsNewEntry } from "$lib/types/whatsNew";
	import { renderMarkdown } from "$lib/utils/markdown";
	import { formatRelativeShort } from "$lib/utils/time";
	import { isWhatsNewComment } from "$lib/utils/whatsNewGrouping";
	import Comment from "./Comment.svelte";

	export let entry: WhatsNewEntry;
	export let compact = false;

	$: timestamp =
		entry.change === "edited"
			? (entry.activityAt ?? entry.updatedAt)
			: (entry.createdAt ?? entry.activityAt);
	$: formattedTimestamp =
		timestamp && !Number.isNaN(Date.parse(timestamp))
			? formatRelativeShort(new Date(timestamp))
			: "";
	$: usesCommentCard =
		entry.eventKind === "opened" || isWhatsNewComment(entry.eventKind, entry.body);
	$: timelineItem = toTimelineItem(entry);

	function reviewState(state?: string): NotificationTimelineItem["state"] {
		switch (state) {
			case "APPROVED":
			case "CHANGES_REQUESTED":
			case "COMMENTED":
			case "DISMISSED":
			case "PENDING":
				return state;
			default:
				return undefined;
		}
	}

	function toTimelineItem(item: WhatsNewEntry): NotificationTimelineItem {
		return {
			type:
				item.eventKind === "discussion_reply" || item.eventKind === "review_comment"
					? "commented"
					: item.eventKind,
			id: item.id,
			author: { login: item.author.login || "Unknown author", avatarUrl: "" },
			body: item.body,
			state: reviewState(item.state),
			htmlUrl: item.htmlUrl,
			createdAt: item.createdAt,
			updatedAt: item.change === "edited" ? (item.activityAt ?? item.updatedAt) : item.updatedAt,
			submittedAt: item.createdAt,
		};
	}
</script>

<article
	class:rounded-lg={!compact && !usesCommentCard}
	class:border={!compact && !usesCommentCard}
	class:border-gray-200={!compact && !usesCommentCard}
	class:dark:border-gray-800={!compact && !usesCommentCard}
	class:overflow-hidden={!compact && !usesCommentCard}
	aria-label={`${entry.isNew ? "New activity" : "Earlier context"} by ${entry.author.login || "unknown author"}`}
>
	{#if !entry.isNew}
		<p class="px-4 pt-3 text-xs font-medium text-gray-500 dark:text-gray-400">Earlier context</p>
	{/if}
	{#if usesCommentCard}
		<Comment
			item={timelineItem}
			showThread={false}
			isLastItem={true}
			knownBot={entry.author.type === "bot"}
			forceEdited={entry.change === "edited"}
			activityEmphasis={entry.isNew ? "new" : "context"}
		/>
	{:else if compact}
		<div
			class="flex flex-wrap items-center gap-x-2 gap-y-1 py-1 text-xs text-gray-600 dark:text-gray-400"
		>
			<span class="font-semibold text-gray-800 dark:text-gray-200">
				{entry.author.login || "Unknown author"}
			</span>
			{#if entry.author.type === "bot"}<span>Bot</span>{/if}
			<span
				>{entry.state?.replaceAll("_", " ") ||
					entry.detail ||
					entry.eventKind.replaceAll("_", " ")}</span
			>
			{#if entry.htmlUrl}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a
					href={entry.htmlUrl}
					target="_blank"
					rel="noreferrer"
					class="text-indigo-600 dark:text-indigo-300 hover:underline"
					title="Open activity on GitHub"
				>
					{formattedTimestamp || "View"}
				</a>
			{:else if formattedTimestamp}
				<span>{formattedTimestamp}</span>
			{/if}
		</div>
	{:else}
		<div
			class="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 bg-gray-50 dark:bg-gray-800/50 text-xs text-gray-600 dark:text-gray-400"
		>
			<span class="font-semibold text-gray-900 dark:text-gray-200"
				>{entry.author.login || "Unknown author"}</span
			>
			{#if entry.author.type === "bot"}<span>Bot</span>{/if}
			{#if entry.change === "edited"}<span>Edited</span>{/if}
			{#if entry.state}<span>{entry.state.replaceAll("_", " ")}</span>{/if}
			{#if entry.htmlUrl}
				<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
				<a
					href={entry.htmlUrl}
					target="_blank"
					rel="noreferrer"
					class="ml-auto text-indigo-600 dark:text-indigo-300 hover:underline"
					title="Open activity on GitHub"
				>
					{formattedTimestamp || "View on GitHub"}
				</a>
			{:else if formattedTimestamp}
				<span class="ml-auto">{formattedTimestamp}</span>
			{/if}
		</div>
		<div class="px-4 py-3 space-y-2">
			{#if entry.path}<p class="font-mono text-xs text-gray-600 dark:text-gray-400 break-all">
					{entry.path}
				</p>{/if}
			{#if entry.diffHunk}<pre
					class="overflow-x-auto text-xs bg-gray-100 dark:bg-gray-950 p-3 rounded"><code
						>{entry.diffHunk}</code
					></pre>{/if}
			{#if entry.body || entry.message}
				<div class="overflow-x-auto prose dark:prose-invert prose-sm max-w-none break-words">
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html renderMarkdown(entry.body || entry.message || "")}
				</div>
			{:else}
				<p class="text-sm text-gray-700 dark:text-gray-300">
					{entry.detail || entry.eventKind.replaceAll("_", " ")}
				</p>
			{/if}
		</div>
	{/if}
</article>
