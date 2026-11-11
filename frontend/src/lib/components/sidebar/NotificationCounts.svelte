<script lang="ts">
	import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";

	export let unread: number | undefined = undefined;
	export let total: number | undefined = undefined;
	export let selected = false;

	const { showTotalCounts } = getNotificationSettingsStore();
	$: showTotal = $showTotalCounts && total !== undefined;
	$: label = showTotal
		? `${unread ?? 0} unread, ${total} total`
		: `${unread} new notification${unread === 1 ? "" : "s"}`;
</script>

{#if (showTotal && total > 0) || (unread !== undefined && unread > 0)}
	<span
		class="flex h-6 flex-shrink-0 items-center justify-center gap-1 whitespace-nowrap rounded-full px-2 text-[11px] tabular-nums {selected
			? 'bg-indigo-200 text-gray-700 dark:bg-gray-700 dark:text-gray-100'
			: 'bg-gray-200 text-gray-700 dark:bg-gray-800/60 dark:text-gray-100'}"
		title={label}
		aria-label={label}
	>
		<span class="font-semibold">{unread ?? 0}</span>
		{#if showTotal}
			<span class="font-normal text-gray-600 dark:text-gray-400">
				<span aria-hidden="true">/</span>
				{total}
			</span>
		{/if}
	</span>
{/if}
