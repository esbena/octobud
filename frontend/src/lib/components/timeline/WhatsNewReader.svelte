<script lang="ts">
	import { onDestroy, tick } from "svelte";
	import { fetchWhatsNew } from "$lib/api/whatsNew";
	import {
		fetchRichActivitySummary,
		type RichActivitySummaryResponse,
	} from "$lib/api/activityBriefs";
	import { createWhatsNewController } from "$lib/state/whatsNewController";
	import { currentTime } from "$lib/stores/timeStore";
	import type {
		WhatsNewEntryCounts,
		WhatsNewReaderModel,
		WhatsNewSnapshot,
	} from "$lib/types/whatsNew";
	import { formatAbsolute } from "$lib/utils/notificationHelpers";
	import { formatRelativeShort, parseTimestamp } from "$lib/utils/time";
	import {
		buildWhatsNewReaderModel,
		compareWhatsNewTimestamps,
		isWhatsNewComment,
	} from "$lib/utils/whatsNewGrouping";
	import {
		ALL_AVAILABLE_ACTIVITY_CUTOFF,
		getWhatsNewCutoff,
		WHATS_NEW_SELECTIONS,
		whatsNewRangeLabel,
		type WhatsNewSelection,
	} from "$lib/utils/whatsNewCutoff";
	import WhatsNewEntry from "./WhatsNewEntry.svelte";
	import WhatsNewThread from "./WhatsNewThread.svelte";

	export let githubId: string;
	export let lastReadAt: string | null;
	export let lastArchivedAt: string | null;
	export let readerUsername: string | null = null;
	export let selection: WhatsNewSelection = "last_read";
	export let active = true;
	export let richSummaryTrigger = 0;
	export let onRichSummaryRequestHandled: (() => void) | undefined = undefined;
	export let onReady: ((timestamp: string) => void) | undefined = undefined;
	export let onShowFullView: (() => void) | undefined = undefined;
	export let onEntryCounts: ((counts: WhatsNewEntryCounts | null) => void) | undefined = undefined;

	const controller = createWhatsNewController();
	const { status, model, error } = controller.stores;
	let lastCommentAvailability: { key: string; available: boolean } | null = null;
	let availabilityController: AbortController | null = null;
	let availabilityRequestKey = "";
	let availabilityErrorKey = "";
	let destroyed = false;
	const reportedModels = new WeakSet<WhatsNewReaderModel>();
	let previousRequest = "";
	let cutoff: string | null = null;
	let richSummary: RichActivitySummaryResponse | null = null;
	let richSummaryKey = "";
	let richSummaryLoading = false;
	let richSummaryError = "";
	let richSummaryRequest = 0;
	let handledRichSummaryTrigger = 0;
	$: selectionLabel =
		WHATS_NEW_SELECTIONS.find((option) => option.value === selection)?.label ?? "";
	$: currentSummaryKey = `${githubId}\u0000${selection}\u0000${actualCutoff ?? "all"}`;
	$: availabilityKey = `${githubId}\u0000${readerUsername ?? ""}`;
	$: cutoff = getWhatsNewCutoff(selection, lastReadAt, lastArchivedAt);
	$: allHistoryFallback =
		(selection === "last_read" && !lastReadAt) ||
		(selection === "last_archive" && !lastArchivedAt) ||
		(selection === "my_last_comment" && $status === "ready" && Boolean($model?.baselineMessage));
	$: actualCutoff =
		selection === "my_last_comment"
			? $status === "ready" && !allHistoryFallback
				? ($model?.cutoff ?? null)
				: null
			: cutoff;
	$: parsedActualCutoff = actualCutoff ? parseTimestamp(actualCutoff) : null;
	$: cutoffLabel =
		parsedActualCutoff && $currentTime ? formatRelativeShort(parsedActualCutoff) : "";
	$: updateCommentAvailability(githubId, readerUsername, selection, active);
	$: if (githubId) {
		const request = `${githubId}\u0000${selection}\u0000${cutoff ?? ""}\u0000${readerUsername ?? ""}`;
		if (request !== previousRequest) {
			previousRequest = request;
			void controller.actions.open({ githubId, cutoff, selection, readerUsername });
		}
	}
	$: onEntryCounts?.($status === "ready" && $model ? $model.counts : null);
	$: if (richSummaryTrigger === 0) handledRichSummaryTrigger = 0;
	$: if (richSummaryTrigger > handledRichSummaryTrigger && $status === "ready") {
		handledRichSummaryTrigger = richSummaryTrigger;
		onRichSummaryRequestHandled?.();
		void generateRichSummary();
	}
	onDestroy(() => {
		destroyed = true;
		availabilityController?.abort();
		controller.actions.reset();
	});

	function updateCommentAvailability(
		notificationId: string,
		username: string | null,
		currentSelection: WhatsNewSelection,
		isActive: boolean
	) {
		const key = `${notificationId}\u0000${username ?? ""}`;
		if (!isActive || !notificationId || !username || currentSelection === "my_last_comment") {
			availabilityController?.abort();
			availabilityController = null;
			availabilityRequestKey = "";
			return;
		}
		if (
			lastCommentAvailability?.key === key ||
			availabilityErrorKey === key ||
			availabilityRequestKey === key
		) {
			return;
		}

		availabilityController?.abort();
		const requestController = new AbortController();
		availabilityController = requestController;
		availabilityRequestKey = key;
		availabilityErrorKey = "";
		void checkCommentAvailability(notificationId, username, key, requestController);
	}

	async function checkCommentAvailability(
		notificationId: string,
		username: string,
		key: string,
		requestController: AbortController
	) {
		try {
			const snapshot: WhatsNewSnapshot = await fetchWhatsNew(
				notificationId,
				ALL_AVAILABLE_ACTIVITY_CUTOFF,
				requestController.signal
			);
			if (requestController.signal.aborted || selection === "my_last_comment") return;
			if (
				snapshot.notificationId !== notificationId ||
				compareWhatsNewTimestamps(snapshot.since, ALL_AVAILABLE_ACTIVITY_CUTOFF) !== 0
			) {
				throw new Error("Activity response does not match the selected notification and cutoff");
			}
			const availabilityModel = buildWhatsNewReaderModel(snapshot, "my_last_comment", username);
			lastCommentAvailability = {
				key,
				available: !availabilityModel.baselineMessage,
			};
		} catch (err) {
			if (requestController.signal.aborted) return;
			console.error("[WhatsNewReader] Failed to check comment availability:", err);
			availabilityErrorKey = key;
		} finally {
			if (availabilityController === requestController) {
				availabilityController = null;
				availabilityRequestKey = "";
			}
		}
	}

	async function reportReady(readyModel: WhatsNewReaderModel) {
		await tick();
		if (
			destroyed ||
			!active ||
			selection !== "last_read" ||
			$status !== "ready" ||
			$model !== readyModel ||
			reportedModels.has(readyModel)
		)
			return;
		reportedModels.add(readyModel);
		const timestamp = readyModel.latestActivityAt;
		if (timestamp && cutoff && compareWhatsNewTimestamps(timestamp, cutoff) === 1) {
			onReady?.(timestamp);
		}
	}

	async function generateRichSummary() {
		const summaryCutoff =
			actualCutoff ?? (allHistoryFallback ? ALL_AVAILABLE_ACTIVITY_CUTOFF : null);
		if (!summaryCutoff || richSummaryLoading) return;
		const request = ++richSummaryRequest;
		const key = currentSummaryKey;
		richSummaryKey = key;
		richSummaryLoading = true;
		richSummaryError = "";
		try {
			const result = await fetchRichActivitySummary(githubId, summaryCutoff);
			if (destroyed || request !== richSummaryRequest || key !== currentSummaryKey) return;
			richSummary = result;
			richSummaryKey = key;
		} catch (err) {
			if (destroyed || request !== richSummaryRequest || key !== currentSummaryKey) return;
			richSummaryError =
				err instanceof Error ? err.message : "Unable to generate a detailed activity summary.";
		} finally {
			if (request === richSummaryRequest) richSummaryLoading = false;
		}
	}

	$: if (active && $status === "ready" && $model && !reportedModels.has($model)) {
		void reportReady($model);
	}
	$: if (richSummaryKey && richSummaryKey !== currentSummaryKey) {
		richSummary = null;
		richSummaryKey = "";
		richSummaryError = "";
		richSummaryRequest++;
		richSummaryLoading = false;
	}
	$: if (selection === "my_last_comment" && $status === "ready" && $model) {
		lastCommentAvailability = {
			key: `${githubId}\u0000${readerUsername ?? ""}`,
			available: !$model.baselineMessage,
		};
		availabilityErrorKey = "";
	}

	$: sections = $model
		? [
				{ title: "Human activity", items: $model.humanFeed },
				{ title: "Bot activity", items: $model.botFeed },
			]
		: [];
	$: hasActivity = sections.some((section) => section.items.length > 0);
	$: coversFullHistory =
		allHistoryFallback ||
		sections.some((section) =>
			section.items.some(
				(item) =>
					item.kind === "entry" &&
					item.entry.eventKind === "opened" &&
					item.entry.isNew &&
					item.entry.change === "created"
			)
		);
</script>

{#if active}
	<div class="space-y-6">
		<div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
			<span class="text-xs font-medium text-gray-600 dark:text-gray-400"
				>Showing {selectionLabel}</span
			>
			{#if allHistoryFallback}
				<span
					aria-label="Cutoff: all available history"
					class="ml-auto whitespace-nowrap text-xs text-gray-500 dark:text-gray-400"
					>All available history</span
				>
			{:else if actualCutoff && parsedActualCutoff}
				<time
					datetime={actualCutoff}
					title={formatAbsolute(actualCutoff)}
					aria-label={`Cutoff: ${formatAbsolute(actualCutoff)}`}
					class="ml-auto whitespace-nowrap text-xs text-gray-500 dark:text-gray-400"
					>{cutoffLabel}</time
				>
			{:else if selection === "my_last_comment" && $status === "loading"}
				<span class="ml-auto whitespace-nowrap text-xs text-gray-500 dark:text-gray-400"
					>Finding cutoff...</span
				>
			{/if}
		</div>
		{#if selection !== "my_last_comment" && availabilityRequestKey === availabilityKey}
			<p role="status" class="text-xs text-gray-500 dark:text-gray-400">
				Checking whether you have commented...
			</p>
		{:else if selection !== "my_last_comment" && availabilityErrorKey === availabilityKey}
			<p role="status" class="text-xs text-gray-500 dark:text-gray-400">
				Couldn't check your comment history; selecting this range will retry.
			</p>
		{:else if selection !== "my_last_comment" && lastCommentAvailability?.key === availabilityKey && !lastCommentAvailability.available}
			<p role="note" class="text-xs text-gray-500 dark:text-gray-400">
				No comment by you was found in the available activity; "Since my last comment" is
				unavailable.
			</p>
		{/if}
		{#if $status === "loading"}
			<p role="status" class="text-sm text-gray-600 dark:text-gray-400 py-6">
				Loading What's new...
			</p>
		{:else if $status === "error"}
			<div
				role="alert"
				class="rounded-lg border border-red-300 dark:border-red-900 bg-red-50 dark:bg-red-950/30 p-4"
			>
				<p class="text-sm text-red-800 dark:text-red-200">Could not load What's new. {$error}</p>
				<button
					type="button"
					class="mt-2 text-sm text-red-700 dark:text-red-300 underline"
					on:click={() => controller.actions.retry()}>Try again</button
				>
			</div>
		{:else if $status === "no_baseline"}
			<p class="text-sm text-gray-600 dark:text-gray-400">
				No previous read is recorded for this notification. Choose another range, or use the full
				view.
			</p>
		{:else if $status === "ready" && $model}
			{#if coversFullHistory}
				<div
					role="note"
					class="space-y-2 rounded-lg bg-gray-50 p-3 text-sm text-gray-600 dark:bg-gray-800/50 dark:text-gray-400"
				>
					<p>
						Showing activity from the start of this conversation. You may prefer Full view for
						reading the whole conversation.
					</p>
					{#if onShowFullView}
						<button type="button" class="font-medium underline" on:click={onShowFullView}
							>Switch to Full view</button
						>
					{/if}
				</div>
			{/if}
			<div class="space-y-3">
				{#if richSummaryError}<p role="alert" class="text-xs text-red-600 dark:text-red-400">
						{richSummaryError}
					</p>{/if}
				{#if richSummary && richSummaryKey === currentSummaryKey}
					<div
						class="space-y-2 rounded-lg border border-indigo-200 bg-indigo-50/60 p-4 dark:border-indigo-900 dark:bg-indigo-950/30"
					>
						<p class="text-sm leading-relaxed text-gray-900 dark:text-gray-100">
							{richSummary.richSummary.text || "No substantive new developments to summarize."}
						</p>
						{#if richSummary.richSummary.evidenceIds.length}
							<div class="flex flex-wrap gap-x-3 gap-y-1">
								{#each richSummary.richSummary.evidenceIds as evidenceId (evidenceId)}
									{@const evidence = [...richSummary.newActivity, ...richSummary.olderContext].find(
										(item) => item.id === evidenceId
									)}
									{#if evidence?.url}
										<a
											href={evidence.url}
											target="_blank"
											rel="noreferrer"
											class="text-xs text-indigo-700 underline decoration-indigo-300 underline-offset-2 dark:text-indigo-300"
										>
											Source: {evidence.actor || evidence.kind}
										</a>
									{/if}
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			</div>
			{#if !hasActivity}
				<p class="py-6 text-sm text-gray-600 dark:text-gray-400">
					No activity {whatsNewRangeLabel(selection)}.
				</p>
			{/if}
			{#each sections as section (section.title)}
				{#if section.items.length}
					<section aria-label={section.title} class="space-y-3">
						<h3 class="text-sm font-semibold text-gray-900 dark:text-gray-200">{section.title}</h3>
						{#each section.items as item (item.kind === "entry" ? `entry:${item.entry.id}` : `thread:${item.thread.id}`)}
							{#if item.kind === "entry"}
								<WhatsNewEntry
									entry={item.entry}
									compact={!isWhatsNewComment(item.entry.eventKind, item.entry.body)}
								/>
							{:else}
								<WhatsNewThread thread={item.thread} />
							{/if}
						{/each}
					</section>
				{/if}
			{/each}
		{/if}
	</div>
{/if}
