<script lang="ts">
	import octicons from "@primer/octicons";
	import type { Notification } from "$lib/api/types";
	import { activityBriefRequest } from "$lib/api/activityBriefs";
	import {
		activityBriefDays,
		activityBriefEnabled,
		activityBriefEntries,
		matchingActivityBrief,
	} from "$lib/stores/activityBriefs";

	export let notification: Pick<Notification, "githubId" | "effectiveSortDate" | "subjectType">;
	let submitting = false;
	let error = "";
	$: supported = ["issue", "pullrequest", "pull_request", "pr", "discussion"].includes(
		notification.subjectType.toLowerCase()
	);
	$: brief = matchingActivityBrief(
		$activityBriefEntries,
		notification.githubId,
		notification.effectiveSortDate,
		$activityBriefDays
	);
	$: pending = submitting || brief?.status === "queued" || brief?.status === "running";
	$: label = pending
		? "Summarizing activity"
		: brief?.status === "ready"
			? "Activity already summarized"
			: "Summarize activity";

	async function summarize() {
		if (!notification.githubId || pending || brief?.status === "ready") return;
		submitting = true;
		error = "";
		try {
			const response = await activityBriefRequest("batch", {
				notificationId: notification.githubId,
				days: $activityBriefDays,
			});
			activityBriefEntries.set(response.entries);
			activityBriefEnabled.set(response.enabled);
			if (response.notice) error = response.notice;
		} catch (err) {
			error = err instanceof Error ? err.message : "Unable to summarize activity";
		} finally {
			submitting = false;
		}
	}
</script>

{#if $activityBriefEnabled && supported && notification.githubId}
	<button
		type="button"
		class="flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-indigo-600 transition hover:bg-gray-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-400 disabled:opacity-50 dark:text-indigo-300 dark:hover:bg-gray-700/50"
		disabled={pending || brief?.status === "ready"}
		aria-label={label}
		aria-busy={pending}
		title={label}
		on:click|stopPropagation={summarize}
	>
		<svg
			aria-hidden="true"
			class:animate-pulse={pending}
			class="h-5 w-5"
			fill="currentColor"
			viewBox="0 0 16 16">{@html octicons.copilot.heights["16"].path}</svg
		>
	</button>
	{#if error}<span role="alert" class="max-w-48 text-xs text-red-600 dark:text-red-400"
			>{error}</span
		>{/if}
{/if}
