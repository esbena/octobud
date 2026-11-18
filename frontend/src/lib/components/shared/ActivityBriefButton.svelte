<script lang="ts">
	import octicons from "@primer/octicons";
	import type { Notification } from "$lib/api/types";
	import { activityBriefRequest } from "$lib/api/activityBriefs";
	import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";
	import {
		activityBriefAutoSummarize,
		activityBriefEnabled,
		activityBriefEntries,
		matchingActivityBrief,
	} from "$lib/stores/activityBriefs";
	import { getWhatsNewCutoff } from "$lib/utils/whatsNewCutoff";

	const { catchUpSelection } = getNotificationSettingsStore();
	export let notification: Pick<
		Notification,
		"githubId" | "effectiveSortDate" | "subjectType" | "timelineLastSeenAt" | "lastArchivedAt"
	>;
	export let sinceOverride: string | null | undefined = undefined;
	export let onShowReadySummary: (() => void) | undefined = undefined;
	export let autoSummarizeOnVisible = false;
	export let autoVisible = false;
	let submitting = false;
	let error = "";
	let autoAttemptedKey = "";
	const timeZone =
		typeof Intl !== "undefined" ? Intl.DateTimeFormat().resolvedOptions().timeZone : "UTC";
	$: since =
		sinceOverride !== undefined
			? sinceOverride
			: getWhatsNewCutoff(
					$catchUpSelection,
					notification.timelineLastSeenAt ?? null,
					notification.lastArchivedAt ?? null
				);
	$: supported = ["issue", "pullrequest", "pull_request", "pr", "discussion"].includes(
		notification.subjectType.toLowerCase()
	);
	$: brief = matchingActivityBrief(
		$activityBriefEntries,
		notification.githubId,
		notification.effectiveSortDate,
		$catchUpSelection,
		since
	);
	$: baselineUnavailable = $catchUpSelection === "last_read" && !since;
	$: autoKey = `${notification.githubId}\u0000${notification.effectiveSortDate ?? ""}\u0000${$catchUpSelection}\u0000${since ?? ""}`;
	$: if (
		autoVisible &&
		autoSummarizeOnVisible &&
		$activityBriefAutoSummarize &&
		$activityBriefEnabled &&
		supported &&
		!baselineUnavailable &&
		notification.githubId &&
		brief?.status !== "ready" &&
		brief?.status !== "queued" &&
		brief?.status !== "running" &&
		autoAttemptedKey !== autoKey
	) {
		autoAttemptedKey = autoKey;
		void summarize();
	}
	$: pending = submitting || brief?.status === "queued" || brief?.status === "running";
	$: ready = brief?.status === "ready";
	$: label = baselineUnavailable
		? "Selected catch-up range is unavailable"
		: pending
			? "Summarizing activity"
			: ready
				? onShowReadySummary
					? "View activity summary"
					: "Activity already summarized"
				: "Summarize activity";

	async function summarize() {
		if (!notification.githubId || pending || baselineUnavailable || ready) return;
		submitting = true;
		error = "";
		try {
			const response = await activityBriefRequest("batch", {
				notificationId: notification.githubId,
				selection: $catchUpSelection,
				timeZone,
				...(since ? { since } : {}),
			});
			activityBriefEntries.set(response.entries);
			activityBriefEnabled.set(response.enabled);
			activityBriefAutoSummarize.set(response.autoSummarize);
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
		disabled={pending || baselineUnavailable || (ready && !onShowReadySummary)}
		aria-label={label}
		aria-busy={pending}
		title={label}
		on:click|stopPropagation={() => {
			if (ready) onShowReadySummary?.();
			else void summarize();
		}}
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
