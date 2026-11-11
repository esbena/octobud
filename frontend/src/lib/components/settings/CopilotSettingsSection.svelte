<script lang="ts">
	import { onMount } from "svelte";
	import { copilotSettingsRequest, type CopilotSettings } from "$lib/api/copilotSettings";

	let settings: CopilotSettings | null = null;
	let busy = true;
	let error = "";
	let saved = false;

	onMount(() => {
		void load();
	});

	async function load() {
		busy = true;
		error = "";
		try {
			settings = await copilotSettingsRequest();
		} catch (err) {
			error = err instanceof Error ? err.message : "Unable to load Copilot settings";
		} finally {
			busy = false;
		}
	}

	async function save() {
		if (!settings) return;
		busy = true;
		error = "";
		saved = false;
		try {
			settings = await copilotSettingsRequest(settings);
			saved = true;
		} catch (err) {
			error = err instanceof Error ? err.message : "Unable to save Copilot settings";
		} finally {
			busy = false;
		}
	}
</script>

<section class="max-w-3xl space-y-6 text-sm text-gray-700 dark:text-gray-300">
	{#if settings}
		<form on:submit|preventDefault={save} class="space-y-5">
			<fieldset disabled={busy} class="space-y-5">
				<div class="flex items-center justify-between gap-6 py-1">
					<div>
						<label
							for="copilot-enabled"
							class="text-base font-medium text-gray-900 dark:text-gray-100">Enable Copilot</label
						>
						<p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
							Off by default. Choose which Copilot features can run in Octobud.
						</p>
					</div>
					<label class="relative flex shrink-0 cursor-pointer items-center">
						<input
							id="copilot-enabled"
							type="checkbox"
							bind:checked={settings.enabled}
							class="peer sr-only"
						/>
						<div
							class="relative h-5 w-9 rounded-full bg-gray-300 after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-all peer-checked:bg-blue-600 peer-checked:after:translate-x-full peer-focus-visible:ring-2 peer-focus-visible:ring-blue-600 peer-focus-visible:ring-offset-2 peer-disabled:opacity-50 dark:bg-gray-700 dark:peer-focus-visible:ring-offset-gray-950"
						></div>
					</label>
				</div>
				<fieldset
					disabled={!settings.enabled}
					class="space-y-5 rounded-xl border border-gray-200 bg-gray-50/50 p-5 transition-opacity disabled:opacity-50 dark:border-gray-800 dark:bg-gray-900/30"
				>
					<legend class="px-2 font-medium text-gray-900 dark:text-gray-100"
						>Activity highlights</legend
					>
					<div class="flex items-center justify-between gap-6">
						<div>
							<label
								for="activity-highlights-enabled"
								class="font-medium text-gray-900 dark:text-gray-100"
								>Enable activity highlights</label
							>
							<p class="mt-1 text-xs text-gray-600 dark:text-gray-400">
								Brief updates for quick triage, with requests for you shown first. Only runs when
								you click <span class="font-medium">Summarize 10 more</span> or a notification's
								<span class="font-medium">Summarize activity</span> action.
							</p>
						</div>
						<label class="relative flex shrink-0 cursor-pointer items-center">
							<input
								id="activity-highlights-enabled"
								type="checkbox"
								bind:checked={settings.activityHighlights.enabled}
								class="peer sr-only"
							/>
							<div
								class="relative h-5 w-9 rounded-full bg-gray-300 after:absolute after:left-[2px] after:top-[2px] after:h-4 after:w-4 after:rounded-full after:bg-white after:transition-all peer-checked:bg-blue-600 peer-checked:after:translate-x-full peer-focus-visible:ring-2 peer-focus-visible:ring-blue-600 peer-focus-visible:ring-offset-2 peer-disabled:opacity-50 dark:bg-gray-700 dark:peer-focus-visible:ring-offset-gray-950"
							></div>
						</label>
					</div>
					<div class="grid gap-5 border-t border-gray-200 pt-5 sm:grid-cols-2 dark:border-gray-800">
						<div class="space-y-2">
							<label
								for="activity-default-days"
								class="block text-xs font-medium text-gray-900 dark:text-gray-100"
								>Default lookback</label
							>
							<select
								id="activity-default-days"
								bind:value={settings.activityHighlights.defaultDays}
								class="h-9 w-full rounded-lg border border-gray-300 bg-white pl-3 pr-8 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-700 dark:bg-gray-950"
							>
								{#each [1, 3, 7, 14, 30] as days}
									<option value={days}>{days} {days === 1 ? "day" : "days"}</option>
								{/each}
							</select>
							<p class="text-xs text-gray-500 dark:text-gray-400">
								Adjustable in each view; not an archive baseline.
							</p>
						</div>
						<div class="space-y-2">
							<label
								for="activity-model"
								class="block text-xs font-medium text-gray-900 dark:text-gray-100">Model</label
							>
							<input
								id="activity-model"
								bind:value={settings.activityHighlights.model}
								required
								maxlength="128"
								class="h-9 w-full rounded-lg border border-gray-300 bg-white px-3 text-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-700 dark:bg-gray-950"
							/>
							<p class="text-xs text-gray-500 dark:text-gray-400">
								A model ID available to your Copilot account.
							</p>
						</div>
					</div>
				</fieldset>
				<div class="flex flex-wrap items-center gap-3">
					<button
						type="submit"
						class="rounded-lg bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-600 focus-visible:ring-offset-2 disabled:opacity-50 dark:focus-visible:ring-offset-gray-950"
					>
						{busy ? "Saving..." : "Save"}
					</button>
					{#if saved}<p role="status" class="text-xs text-green-700 dark:text-green-400">
							Copilot settings saved.
						</p>{/if}
				</div>
				<p class="text-xs text-gray-500 dark:text-gray-400">
					Saving never generates summaries. Disabling either switch cancels pending and running
					summaries.
				</p>
			</fieldset>
		</form>
	{:else if busy}
		<p role="status">Loading Copilot settings...</p>
	{:else}
		<button type="button" class="underline" on:click={load}>Retry loading settings</button>
	{/if}
	{#if error}<p role="alert" class="text-red-600 dark:text-red-400">{error}</p>{/if}
	<div class="space-y-3 border-t border-gray-200 pt-6 dark:border-gray-800">
		<h3 class="font-medium text-gray-900 dark:text-gray-100">Authentication</h3>
		<p>
			Copilot uses authentication available to the Octobud backend's local Copilot runtime, which
			may differ from your Octobud GitHub account. Sign in with <code>copilot login</code>, or use
			an authenticated GitHub CLI (<code>gh</code>). To supply a different token, set
			<code>COPILOT_GITHUB_TOKEN</code> in the environment that launches Octobud. Octobud does not store
			this token.
		</p>
		<details class="text-xs text-gray-500 dark:text-gray-400">
			<summary class="cursor-pointer font-medium text-gray-700 dark:text-gray-300"
				>Token precedence and requirements</summary
			>
			<p class="mt-2 leading-relaxed">
				Credential precedence: <code>COPILOT_GITHUB_TOKEN</code>, <code>GH_TOKEN</code>,
				<code>GITHUB_TOKEN</code>, saved Copilot login, then <code>gh auth token</code>. A personal
				access token must be fine-grained with the Copilot Requests account permission; Octobud's
				classic notifications token cannot be reused.
			</p>
		</details>
		<a
			class="underline"
			href="https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/authenticate-copilot-cli"
			target="_blank"
			rel="noreferrer">Copilot CLI authentication documentation</a
		>
	</div>
</section>
