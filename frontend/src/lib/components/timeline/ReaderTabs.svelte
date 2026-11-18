<script lang="ts">
	import type { WhatsNewEntryCounts } from "$lib/types/whatsNew";

	export let activeView: "full" | "whats_new" = "full";
	export let id: string;
	export let counts: WhatsNewEntryCounts | null = null;
	export let onViewChange: ((view: "full" | "whats_new") => void) | undefined = undefined;

	const tabs = [
		{ value: "full", label: "Full view" },
		{ value: "whats_new", label: "What's new" },
	];

	function selectView(view: "full" | "whats_new") {
		activeView = view;
		onViewChange?.(view);
	}

	function handleKeydown(event: KeyboardEvent) {
		if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
		event.preventDefault();
		selectView(
			event.key === "Home"
				? "full"
				: event.key === "End"
					? "whats_new"
					: activeView === "full"
						? "whats_new"
						: "full"
		);
		document.getElementById(`${id}-${activeView}-tab`)?.focus();
	}
</script>

<div class="mb-5">
	<div
		role="tablist"
		aria-label="Conversation view"
		class="flex gap-4 border-b border-gray-200 dark:border-gray-800"
	>
		{#each tabs as tab (tab.value)}
			<button
				type="button"
				role="tab"
				id={`${id}-${tab.value}-tab`}
				aria-controls={`${id}-${tab.value}-panel`}
				aria-selected={activeView === tab.value}
				tabindex={activeView === tab.value ? 0 : -1}
				class="py-2 text-sm border-b-2 border-transparent text-gray-600 dark:text-gray-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500"
				class:!border-indigo-500={activeView === tab.value}
				class:!text-indigo-600={activeView === tab.value}
				on:click={() => selectView(tab.value === "full" ? "full" : "whats_new")}
				on:keydown={handleKeydown}
				>{tab.label}{counts
					? ` (${tab.value === "full" ? counts.full : counts.whatsNew})`
					: ""}</button
			>
		{/each}
	</div>
</div>
