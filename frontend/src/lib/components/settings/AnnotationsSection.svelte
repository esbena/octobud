<script lang="ts">
	import { onMount } from "svelte";
	import { SvelteSet } from "svelte/reactivity";
	import {
		fetchAnnotations,
		fetchAnnotationViews,
		saveAnnotation,
		deleteAnnotation,
		type Annotation,
	} from "$lib/api/annotations";
	import type { NotificationView } from "$lib/api/types";
	import { BUILT_IN_VIEWS, normalizeViewId } from "$lib/state/types";
	import { annotationErrorMessage, compileAnnotation } from "$lib/utils/annotations";
	import Modal from "$lib/components/shared/Modal.svelte";
	import ConfirmDialog from "$lib/components/dialogs/ConfirmDialog.svelte";
	import { toastStore } from "$lib/stores/toastStore";
	import AnnotationTestPanel from "./AnnotationTestPanel.svelte";

	const example = `({ notification, subject }) => {
  if (!subject?.body?.includes("BREAKING CHANGE")) return [];
  return [{
    text: "Breaking change",
    color: "#d93f0b",
    hoverText: "The stored description mentions a breaking change."
  }];
}`;
	let annotations: Annotation[] = [];
	let views: { id: string; name: string }[] = [];
	let loading = true;
	let loadError = "";
	let open = false;
	let editing: Annotation | null = null;
	let name = "";
	let code = example;
	let viewIds: string[] = [];
	let error = "";
	let saving = false;
	let confirmDelete = false;
	let enabled = true;
	let expandedIds = new SvelteSet<string>();
	let togglingIds = new SvelteSet<string>();

	function viewNames(annotation: Annotation): string {
		return annotation.viewIds.length === 0
			? "All views"
			: annotation.viewIds
					.map((id) => views.find((view) => view.id === id)?.name ?? "Unavailable view")
					.join(", ");
	}

	async function toggleEnabled(annotation: Annotation) {
		togglingIds.add(annotation.id);
		try {
			const { id, ...input } = annotation;
			const result = await saveAnnotation({ ...input, enabled: !annotation.enabled }, id);
			annotations = annotations.map((item) => (item.id === id ? result : item));
			toastStore.show(`Annotation ${result.enabled ? "enabled" : "disabled"}`, "success");
		} catch (cause) {
			toastStore.show(`Failed to update annotation: ${cause}`, "error");
		} finally {
			togglingIds.delete(annotation.id);
		}
	}

	async function load() {
		loading = true;
		loadError = "";
		try {
			const [items, availableViews] = await Promise.all([
				fetchAnnotations(),
				fetchAnnotationViews(),
			]);
			annotations = items;
			const builtIn = Object.values(BUILT_IN_VIEWS);
			views = [
				...builtIn,
				...availableViews.filter(
					(view: NotificationView) =>
						!view.systemView && !builtIn.some((builtin) => builtin.id === normalizeViewId(view.id))
				),
			];
		} catch (cause) {
			loadError = String(cause);
		} finally {
			loading = false;
		}
	}
	onMount(() => {
		void load();
	});

	function edit(annotation: Annotation | null) {
		editing = annotation;
		name = annotation?.name ?? "";
		code = annotation?.code ?? example;
		viewIds = [...(annotation?.viewIds ?? [])];
		enabled = annotation?.enabled ?? true;
		error = "";
		open = true;
	}

	async function save() {
		error = "";
		saving = true;
		try {
			if (!name.trim()) throw new Error("A name is required.");
			compileAnnotation(code);
			const result = await saveAnnotation(
				{ name: name.trim(), code, viewIds, enabled },
				editing?.id
			);
			annotations = editing
				? annotations.map((item) => (item.id === result.id ? result : item))
				: [...annotations, result];
			open = false;
			toastStore.show("Annotation saved", "success");
		} catch (cause) {
			error = annotationErrorMessage(cause);
		} finally {
			saving = false;
		}
	}

	async function remove() {
		if (!editing) return;
		saving = true;
		try {
			await deleteAnnotation(editing.id);
			annotations = annotations.filter((item) => item.id !== editing?.id);
			confirmDelete = false;
			open = false;
			toastStore.show("Annotation deleted", "success");
		} catch (cause) {
			confirmDelete = false;
			error = String(cause);
		} finally {
			saving = false;
		}
	}
</script>

<div>
	<div class="flex items-center justify-between mb-6 gap-4">
		<div>
			<h2 class="text-md font-medium text-gray-900 dark:text-gray-100">Annotations</h2>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Add display-only labels by inspecting stored notification data in your browser.
			</p>
		</div>
		<button
			class="rounded-full bg-indigo-600 px-4 py-2 text-xs font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50 cursor-pointer shrink-0"
			on:click={() => edit(null)}
			disabled={loading || !!loadError}
		>
			New annotation
		</button>
	</div>
	{#if loading}
		<p role="status">Loading annotations...</p>
	{:else if loadError}
		<p role="alert" class="text-red-600">{loadError}</p>
		<button on:click={load}>Retry</button>
	{:else if annotations.length === 0}
		<div
			class="text-center py-12 bg-gray-50 dark:bg-gray-900 rounded-lg border border-gray-200 dark:border-gray-800"
		>
			<h3 class="text-md font-medium text-gray-700 dark:text-gray-300 mb-2">No annotations yet.</h3>
			<p class="text-xs text-gray-600 dark:text-gray-500 mb-4">
				Create your first annotation to highlight information in your views.
			</p>
			<button
				class="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-medium rounded-full transition-colors cursor-pointer"
				on:click={() => edit(null)}>Create annotation</button
			>
		</div>
	{:else}
		<div class="space-y-2">
			{#each annotations as annotation (annotation.id)}
				{@const expanded = expandedIds.has(annotation.id)}
				<div
					class="bg-gray-50 dark:bg-gray-900/60 border border-gray-200 dark:border-gray-800 rounded-lg overflow-hidden"
				>
					<button
						type="button"
						class="flex items-center justify-between gap-3 px-3 py-3 w-full text-left cursor-pointer hover:bg-gray-100 dark:hover:bg-gray-800/30 transition-colors"
						aria-expanded={expanded}
						aria-controls={`annotation-overview-${annotation.id}`}
						on:click={() =>
							expanded ? expandedIds.delete(annotation.id) : expandedIds.add(annotation.id)}
					>
						<span class="flex items-center gap-3 flex-1 min-w-0">
							<span
								class="w-2 h-2 rounded-full shrink-0 {annotation.enabled
									? 'bg-green-500'
									: 'bg-gray-500'}"
								title={annotation.enabled ? "Enabled" : "Disabled"}
							></span>
							<span class="text-sm font-semibold text-gray-900 dark:text-gray-100 truncate"
								>{annotation.name}</span
							>
							<span class="hidden sm:inline text-xs text-gray-500 truncate"
								>{viewNames(annotation)}</span
							>
							<span class="sr-only">{annotation.enabled ? "Enabled" : "Disabled"}</span>
						</span>
						<svg
							class="w-4 h-4 text-gray-600 dark:text-gray-400 shrink-0 transition-transform {expanded
								? 'rotate-180'
								: ''}"
							fill="none"
							stroke="currentColor"
							viewBox="0 0 24 24"
							aria-hidden="true"
						>
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								stroke-width="2"
								d="M19 9l-7 7-7-7"
							/>
						</svg>
					</button>
					{#if expanded}
						<div
							id={`annotation-overview-${annotation.id}`}
							class="border-t border-gray-200 dark:border-gray-800 pt-5 px-4 pb-4 space-y-3"
						>
							<div class="bg-gray-100/40 dark:bg-gray-800/40 rounded-lg p-4 space-y-5">
								<div class="space-y-2">
									<h4 class="text-sm font-medium text-gray-900 dark:text-gray-200">
										Apply to views
									</h4>
									<p class="text-xs text-gray-600 dark:text-gray-400">{viewNames(annotation)}</p>
								</div>
								<div class="space-y-2">
									<h4 class="text-sm font-medium text-gray-900 dark:text-gray-200">
										JavaScript function
									</h4>
									<pre
										class="max-h-48 overflow-auto rounded-lg border border-gray-300 dark:border-gray-800 bg-white dark:bg-gray-950 p-3 text-xs text-gray-800 dark:text-gray-300"><code
											>{annotation.code}</code
										></pre>
								</div>
							</div>
							<div class="flex items-center justify-between gap-3">
								<label
									class="flex items-center gap-3 cursor-pointer text-sm text-gray-700 dark:text-gray-300"
								>
									<span>Enable annotation</span>
									<input
										type="checkbox"
										class="sr-only peer"
										aria-label={`Enable ${annotation.name}`}
										checked={annotation.enabled}
										disabled={togglingIds.has(annotation.id)}
										on:change={(event) => {
											event.currentTarget.checked = annotation.enabled;
											void toggleEnabled(annotation);
										}}
									/>
									<span
										class="relative w-11 h-6 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-violet-600 rounded-full peer peer-disabled:opacity-50 peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-violet-600"
									></span>
								</label>
								<button
									type="button"
									class="rounded-full border border-gray-300 dark:border-gray-800 px-4 py-2 text-xs font-medium text-gray-700 dark:text-gray-300 transition hover:bg-gray-100 dark:hover:bg-gray-800 cursor-pointer disabled:opacity-50"
									disabled={togglingIds.has(annotation.id)}
									on:click={() => edit(annotation)}>Edit annotation</button
								>
							</div>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</div>

<Modal
	{open}
	title={editing ? "Edit annotation" : "New annotation"}
	size="lg"
	closeOnOverlay={false}
	maxHeight="calc(100vh - 12rem)"
	onClose={() => {
		if (!saving) open = false;
	}}
>
	<form class="space-y-5" on:submit|preventDefault={save}>
		<div class="rounded-xl bg-gray-50 dark:bg-gray-900/80 p-5 space-y-5">
			<div class="space-y-2">
				<label for="annotation-name" class="text-sm font-medium text-gray-900 dark:text-gray-200">
					Name <span class="text-red-400" aria-hidden="true">*</span>
				</label>
				<p class="text-xs text-gray-600 dark:text-gray-500">
					Choose a descriptive name for this annotation
				</p>
				<input
					id="annotation-name"
					class="w-full rounded-lg border border-gray-300 dark:border-gray-800 bg-white dark:bg-gray-950 px-3 py-2 text-sm text-gray-900 dark:text-gray-200 placeholder-gray-500 outline-none transition focus:border-blue-600 focus:ring-2 focus:ring-blue-600/30"
					placeholder="e.g., Repository mentions"
					bind:value={name}
					required
					disabled={saving}
				/>
			</div>
			<fieldset disabled={saving} class="space-y-2">
				<legend class="text-sm font-medium text-gray-900 dark:text-gray-200 mb-2"
					>Apply to views</legend
				>
				<div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-x-4 gap-y-2">
					{#each views as view (view.id)}
						<label class="flex gap-2 items-start text-sm min-w-0">
							<input class="mt-0.5 shrink-0" type="checkbox" value={view.id} bind:group={viewIds} />
							<span class="break-words min-w-0">{view.name}</span>
						</label>
					{/each}
				</div>
				<p class="text-xs text-gray-600 dark:text-gray-500">
					Select views to limit where this annotation runs. With no views selected, it runs
					in all views. Disable the annotation to stop it everywhere.
				</p>
			</fieldset>
			<div class="space-y-2">
				<label for="annotation-code" class="text-sm font-medium text-gray-900 dark:text-gray-200">
					JavaScript function expression <span class="text-red-400" aria-hidden="true">*</span>
				</label>
				<p class="text-xs text-gray-600 dark:text-gray-500">
					Inspect stored notification data and return display-only labels
				</p>
				<textarea
					id="annotation-code"
					class="block w-full rounded-lg border border-gray-300 dark:border-gray-800 bg-white dark:bg-gray-950 px-3 py-2 text-gray-900 dark:text-gray-200 font-mono text-xs outline-none transition focus:border-blue-600 focus:ring-2 focus:ring-blue-600/30"
					rows="12"
					bind:value={code}
					spellcheck="false"
					required
					disabled={saving}
				></textarea>
			</div>
			<p class="text-xs text-gray-500">
				Receives {"{ notification, subject }"}. subject is the stored raw GitHub subject, or null;
				notification.raw contains the complete API snapshot; notification.payload and
				subjectFetchedAt are also available when stored. Return an array of {'{ text, color: "#RRGGBB", hoverText? }'}
				labels, or []. Text and hover text are plain text; Unicode emojis work, HTML and Markdown are
				not rendered. Ordinary and async functions are supported. Code runs in your browser without a
				worker; synchronous loops can freeze the UI. Only use code you trust.
			</p>
			{#if open}
				<AnnotationTestPanel {code} />
			{/if}
			<div
				class="flex items-center justify-between gap-3 py-3 border-t border-gray-200 dark:border-gray-800"
			>
				<div>
					<div class="text-sm font-medium text-gray-700 dark:text-gray-300">Enable annotation</div>
					<p class="text-xs text-gray-600 dark:text-gray-500">
						Run in the selected views, or all views if none are selected
					</p>
				</div>
				<label class="flex items-center cursor-pointer">
					<input
						type="checkbox"
						bind:checked={enabled}
						disabled={saving}
						class="sr-only peer"
						aria-label="Enable annotation"
					/>
					<span
						class="relative w-11 h-6 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-violet-600 rounded-full peer peer-checked:after:translate-x-full after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-violet-600"
					></span>
				</label>
			</div>
			{#if error}<p role="alert" class="text-sm text-red-600">{error}</p>{/if}
		</div>
		<div class="flex items-center justify-between gap-3 pt-2">
			<div>
				{#if editing}<button
						type="button"
						class="rounded-full border border-gray-300 dark:border-gray-800 px-4 py-2 text-sm font-medium text-rose-600 dark:text-rose-300 transition hover:border-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40 cursor-pointer"
						disabled={saving}
						on:click={() => (confirmDelete = true)}>Delete annotation</button
					>{/if}
			</div>
			<div class="flex items-center gap-3">
				<button
					type="button"
					class="rounded-full border border-transparent px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-300 transition hover:bg-gray-100 dark:hover:bg-gray-800 cursor-pointer"
					disabled={saving}
					on:click={() => (open = false)}>Cancel</button
				>
				<button
					type="submit"
					class="rounded-full bg-indigo-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-indigo-700 disabled:opacity-50 disabled:hover:bg-indigo-600 cursor-pointer"
					disabled={saving || !name.trim() || !code.trim()}
					>{saving ? "Saving..." : editing ? "Save changes" : "Create annotation"}</button
				>
			</div>
		</div>
	</form>
</Modal>

<ConfirmDialog
	open={confirmDelete}
	title="Delete annotation"
	body="Delete this annotation from all its views?"
	confirmLabel="Delete"
	confirming={saving}
	onConfirm={remove}
	onCancel={() => {
		if (!saving) confirmDelete = false;
	}}
/>
