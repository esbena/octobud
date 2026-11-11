<script lang="ts">
	import { onDestroy } from "svelte";
	import { fetchNotificationDetail } from "$lib/api/notifications";
	import type { Notification } from "$lib/api/types";
	import {
		annotationErrorMessage,
		compileAnnotation,
		createAnnotationContext,
		runAnnotation,
	} from "$lib/utils/annotations";

	export let code: string;

	let url = "";
	let notification: Notification | null = null;
	let inputJson = "";
	let outputJson = "";
	let loadError = "";
	let runError = "";
	let loading = false;
	let running = false;
	let request: AbortController | null = null;
	let loadGeneration = 0;
	let runGeneration = 0;

	function resetInput(_url: string) {
		request?.abort();
		loadGeneration++;
		resetOutput(code);
		notification = null;
		inputJson = "";
		loadError = "";
		loading = false;
	}

	function resetOutput(_code: string) {
		runGeneration++;
		outputJson = "";
		runError = "";
		running = false;
	}

	$: resetInput(url);
	$: resetOutput(code);

	async function loadInput() {
		resetInput(url);
		const current = loadGeneration;
		const controller = new AbortController();
		request = controller;
		loading = true;
		try {
			const parsed = new URL(url);
			const ids = parsed.searchParams.getAll("id");
			if (
				!["http:", "https:"].includes(parsed.protocol) ||
				!/^\/views\/[^/]+\/?$/.test(parsed.pathname) ||
				ids.length !== 1 ||
				!ids[0].trim()
			) {
				throw new Error("Enter an Octobud view URL with a single notification id parameter.");
			}
			// Only the ID is used; notification data always comes from this Octobud instance.
			const detail = await fetchNotificationDetail(ids[0], { signal: controller.signal });
			if (current !== loadGeneration) return;
			inputJson = JSON.stringify(createAnnotationContext(detail.notification), null, 2);
			notification = detail.notification;
		} catch (cause) {
			if (current === loadGeneration) loadError = String(cause);
		} finally {
			if (current === loadGeneration) loading = false;
		}
	}

	async function run() {
		if (!notification) return;
		const current = ++runGeneration;
		running = true;
		outputJson = "";
		runError = "";
		try {
			const output = await runAnnotation(compileAnnotation(code), notification);
			if (current === runGeneration) outputJson = JSON.stringify(output, null, 2);
		} catch (cause) {
			if (current === runGeneration) runError = annotationErrorMessage(cause);
		} finally {
			if (current === runGeneration) running = false;
		}
	}

	onDestroy(() => {
		request?.abort();
		loadGeneration++;
		runGeneration++;
	});
</script>

<section
	class="space-y-3 rounded-lg border border-gray-200 dark:border-gray-800 p-4"
	aria-label="Test annotation"
>
	<h3 class="text-sm font-medium">Test annotation</h3>
	<label class="block text-sm">
		Notification URL
		<input
			type="text"
			class="mt-1 block w-full rounded border-gray-300 dark:bg-gray-900 text-sm"
			bind:value={url}
			placeholder="http://localhost:8808/views/inbox?id=27088099001"
			on:keydown={(event) => {
				if (event.key === "Enter") {
					event.preventDefault();
					if (url.trim() && !loading) void loadInput();
				}
			}}
		/>
	</label>
	<p class="text-xs text-gray-500">
		Load a stored notification from this Octobud instance, then run the current unsaved code. No
		read/archive state is changed. Console logs appear in your browser's developer console.
	</p>
	<button
		type="button"
		class="rounded-full border border-gray-300 dark:border-gray-700 px-3 py-1.5 text-sm"
		disabled={!url.trim() || loading}
		on:click={loadInput}
	>
		{loading ? "Loading input..." : "Load input"}
	</button>
	{#if loading}<p role="status" class="text-xs text-gray-500">Loading notification...</p>{/if}
	{#if loadError}<p role="alert" class="text-sm text-red-600">{loadError}</p>{/if}
	<label class="block text-sm">
		Function argument (JSON)
		<textarea
			class="mt-1 block w-full rounded border-gray-300 dark:bg-gray-900 font-mono text-xs"
			rows="10"
			readonly
			value={inputJson}
			spellcheck="false"
		></textarea>
	</label>
	<button
		type="button"
		class="rounded-full border border-gray-300 dark:border-gray-700 px-3 py-1.5 text-sm"
		disabled={!notification || loading || running}
		on:click={run}
	>
		{running ? "Running..." : "Run annotation"}
	</button>
	{#if running}<p role="status" class="text-xs text-gray-500">Running annotation...</p>{/if}
	{#if runError}<p role="alert" class="text-sm text-red-600">{runError}</p>{/if}
	<label class="block text-sm">
		Function output (JSON)
		<textarea
			class="mt-1 block w-full rounded border-gray-300 dark:bg-gray-900 font-mono text-xs"
			rows="6"
			readonly
			value={outputJson}
			spellcheck="false"
		></textarea>
	</label>
</section>
