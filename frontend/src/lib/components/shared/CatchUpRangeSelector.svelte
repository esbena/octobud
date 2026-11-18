<script lang="ts">
	import { createEventDispatcher } from "svelte";
	import { WHATS_NEW_SELECTIONS, type WhatsNewSelection } from "$lib/utils/whatsNewCutoff";

	export let selection: WhatsNewSelection;
	const dispatch = createEventDispatcher<{ selection: WhatsNewSelection }>();
</script>

<div role="group" aria-label="Activity window" class="flex flex-wrap gap-2">
	{#each WHATS_NEW_SELECTIONS as option (option.value)}
		<button
			type="button"
			aria-pressed={selection === option.value}
			class={selection === option.value
				? "rounded-full border border-indigo-500 bg-indigo-50 px-3 py-1.5 text-xs font-medium text-indigo-800 dark:bg-indigo-950 dark:text-indigo-200"
				: "rounded-full border border-gray-200 bg-white px-3 py-1.5 text-xs text-gray-700 hover:bg-gray-50 dark:border-gray-800 dark:bg-gray-900 dark:text-gray-300 dark:hover:bg-gray-800"}
			on:click={() => {
				selection = option.value;
				dispatch("selection", option.value);
			}}
		>
			{option.label}
		</button>
	{/each}
</div>
