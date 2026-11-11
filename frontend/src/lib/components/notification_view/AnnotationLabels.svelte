<script lang="ts">
	import { onDestroy } from "svelte";
	import { browser } from "$app/environment";
	import type { Annotation } from "$lib/api/annotations";
	import type { Notification } from "$lib/api/types";
	import {
		annotationErrorMessage,
		compileAnnotation,
		runAnnotation,
		labelTextColor,
		type AnnotationLabel,
	} from "$lib/utils/annotations";

	export let annotations: Annotation[] = [];
	export let notification: Notification;
	let results: {
		name: string;
		labels: AnnotationLabel[];
		error: string | null;
		pending: boolean;
	}[] = [];
	let generation = 0;

	async function evaluate(definitions: Annotation[], snapshot: Notification) {
		const current = ++generation;
		results = definitions.map((item) => ({
			name: item.name,
			labels: [],
			error: null,
			pending: true,
		}));
		await Promise.all(
			definitions.map(async (definition, index) => {
				try {
					const labels = await runAnnotation(compileAnnotation(definition.code), snapshot);
					if (current !== generation) return;
					results[index] = { name: definition.name, labels, error: null, pending: false };
				} catch (cause) {
					if (current !== generation) return;
					results[index] = {
						name: definition.name,
						labels: [],
						error: annotationErrorMessage(cause),
						pending: false,
					};
				}
			})
		);
	}

	$: if (browser) void evaluate(annotations, notification);
	onDestroy(() => {
		generation++;
	});
</script>

{#each results as result, resultIndex (resultIndex)}
	{#if result.pending}
		<span
			role="status"
			title={`Running ${result.name}`}
			class="inline-flex items-center gap-1 text-[10px] text-gray-500"
		>
			<span
				class="h-3 w-3 animate-spin rounded-full border-2 border-current border-t-transparent"
				aria-hidden="true"
			></span>
			<span class="sr-only">Running {result.name}</span>
		</span>
	{:else if result.error}
		<span
			class="text-[10px] text-red-600 dark:text-red-400"
			title={`${result.name}: ${result.error}`}>Annotation error: {result.name}</span
		>
	{:else}
		{#each result.labels as label, labelIndex (labelIndex)}
			<span
				class="rounded-full border border-black/10 px-2 py-0.5 text-[10px] font-semibold"
				style:background-color={label.color}
				style:color={labelTextColor(label.color)}
				title={label.hoverText ?? label.text}
			>
				{label.text}
			</span>
		{/each}
	{/if}
{/each}
