<script lang="ts">
	import octicons from "@primer/octicons";
	import { getContext, onMount } from "svelte";
	import { activityBriefRequest } from "$lib/api/activityBriefs";
	import { activityBriefDays, activityBriefEntries } from "$lib/stores/activityBriefs";
	import type { NotificationPageController } from "$lib/state/types";

	const controller = getContext<NotificationPageController>("notificationPageController");
	const { pageData, isLoading } = controller.stores;
	let enabled = false;
	let submitting = false;
	let error = "";
	let statusError = "";
	let message = "";
	let alive = true;
	let requestSequence = 0;
	let appliedSequence = 0;
	let defaultsLoaded = false;
	let loaded = false;
	$: pending = $activityBriefEntries.filter(
		(entry) => entry.status === "queued" || entry.status === "running"
	).length;
	$: failures = $activityBriefEntries.filter((entry) => entry.status === "failed").length;

	async function update(action: "status" | "batch" | "stop") {
		const sequence = ++requestSequence;
		try {
			const scope = $pageData.queryScope;
			if (action === "batch" && !scope) throw new Error("Wait for the current view to load.");
			const response = await activityBriefRequest(
				action,
				action === "batch" && scope
					? { query: scope.query, repositoryIds: scope.repositoryIds, days: $activityBriefDays }
					: undefined
			);
			if (!alive || sequence < appliedSequence) return;
			appliedSequence = sequence;
			enabled = response.enabled;
			loaded = true;
			if (!defaultsLoaded) {
				activityBriefDays.set(response.defaultDays);
				defaultsLoaded = true;
			}
			activityBriefEntries.set(response.entries);
			if (action !== "status") error = "";
			if (action === "status") statusError = "";
			if (action === "batch") {
				message = response.queued
					? `Queued ${response.queued} more.`
					: "All supported items in this view already have current summaries.";
			}
			if (action === "stop") message = "Stopped pending summaries.";
			if (response.notice) error = response.notice;
		} catch (err) {
			if (alive && sequence >= appliedSequence) {
				const message = err instanceof Error ? err.message : "Activity summary request failed.";
				if (action === "status") statusError = message;
				else error = message;
			}
		}
	}

	async function submit(action: "batch" | "stop") {
		submitting = true;
		message = "";
		await update(action);
		submitting = false;
	}

	onMount(() => {
		alive = true;
		let timer: ReturnType<typeof setTimeout>;
		async function poll() {
			if (!submitting) await update("status");
			if (alive) timer = setTimeout(poll, 2500);
		}
		void poll();
		return () => {
			alive = false;
			clearTimeout(timer);
		};
	});
</script>

{#if enabled}
	<div
		class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-2 pr-3 text-xs"
		aria-label="Activity highlights controls"
	>
		<span class="flex items-center gap-1.5 font-medium text-gray-600 dark:text-gray-400">
			<svg
				aria-hidden="true"
				class="h-3.5 w-3.5 text-indigo-500 dark:text-indigo-400"
				viewBox="0 0 16 16"
				fill="currentColor">{@html octicons.copilot.heights["16"].path}</svg
			>
			Activity highlights
		</span>
		<label class="flex items-center gap-2 text-gray-500 dark:text-gray-400">
			Max lookback
			<select
				class="h-7 rounded-md border border-gray-200 bg-white py-0 pl-2 pr-7 text-xs text-gray-700 focus:border-indigo-400 focus:outline-none focus:ring-1 focus:ring-indigo-400 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300"
				bind:value={$activityBriefDays}
			>
				{#each [1, 3, 7, 14, 30] as days (days)}
					<option value={days}>{days} {days === 1 ? "day" : "days"}</option>
				{/each}
			</select>
		</label>
		<button
			type="button"
			class="h-7 rounded-md bg-indigo-50 px-3 font-medium text-indigo-700 transition-colors hover:bg-indigo-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-indigo-500/10 dark:text-indigo-300 dark:hover:bg-indigo-500/20"
			disabled={!enabled || submitting || $isLoading || !$pageData.queryScope}
			on:click={() => submit("batch")}
			title="Queue the first ten missing summaries from the top of this view, across pages. Cached and queued items are skipped."
			>Summarize 10 more</button
		>
		{#if pending}
			<span role="status" class="flex items-center gap-1.5 text-gray-500 dark:text-gray-400"
				><span aria-hidden="true" class="h-1.5 w-1.5 rounded-full bg-indigo-500"></span>{pending} pending</span
			>
			<button
				type="button"
				class="rounded px-1 py-0.5 text-gray-600 underline decoration-gray-300 underline-offset-2 hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400 disabled:opacity-50 dark:text-gray-400 dark:hover:text-gray-100"
				disabled={submitting}
				on:click={() => submit("stop")}>Stop</button
			>
		{/if}
		{#if failures}<span class="text-red-600 dark:text-red-400"
				>{failures} failed · click summarize to retry missing items</span
			>{/if}
		<a
			href="/settings#copilot"
			class="ml-auto rounded p-1 text-gray-400 transition-colors hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400 dark:text-gray-500 dark:hover:text-gray-300"
			aria-label="Activity highlights settings"
			title="Activity highlights settings"
			><svg
				aria-hidden="true"
				class="h-3.5 w-3.5"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="1.5"
				stroke-linecap="round"><path d="M4 7h16M4 17h16M9 4v6M15 14v6" /></svg
			></a
		>
		{#if message}<span role="status" class="text-gray-500">{message}</span>{/if}
	</div>
{:else if loaded && !statusError}
	<div class="mt-2 text-xs">
		<a
			class="text-gray-400 transition-colors hover:text-indigo-600 dark:text-gray-500 dark:hover:text-indigo-400"
			href="/settings#copilot">Set up activity highlights</a
		>
	</div>
{/if}
{#if error || statusError}
	<p role="alert" class="mt-2 text-xs text-red-600 dark:text-red-400">{error || statusError}</p>
{/if}
