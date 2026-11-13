<script lang="ts">
	// Copyright (C) 2025 Austin Beattie
	//
	// This program is free software: you can redistribute it and/or modify
	// it under the terms of the GNU Affero General Public License as
	// published by the Free Software Foundation, either version 3 of the
	// License, or (at your option) any later version.
	//
	// This program is distributed in the hope that it will be useful,
	// but WITHOUT ANY WARRANTY; without even the implied warranty of
	// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	// GNU Affero General Public License for more details.
	//
	// You should have received a copy of the GNU Affero General Public License
	// along with this program.  If not, see <https://www.gnu.org/licenses/>.

	import { getThemeStore } from "$lib/stores/themeStore";
	import { getNotificationSettingsStore } from "$lib/stores/notificationSettings";
	import { toastStore } from "$lib/stores/toastStore";
	import { WHATS_NEW_SELECTIONS } from "$lib/utils/whatsNewCutoff";

	const themeStore = getThemeStore();
	const { theme } = themeStore;

	const settingsStore = getNotificationSettingsStore();
	const {
		timelineAutoScroll,
		hoverActionsEnabled,
		showTotalCounts,
		whatsNewDefault,
		whatsNewBaseline,
	} = settingsStore;

	function handleThemeToggle() {
		themeStore.toggle();
	}

	function handleTimelineAutoScrollToggle(enabled: boolean) {
		settingsStore.setTimelineAutoScroll(enabled);
		if (enabled) {
			toastStore.success("Auto-scroll to new timeline activity enabled");
		} else {
			toastStore.success("Auto-scroll to new timeline activity disabled");
		}
	}

	function handleHoverActionsToggle(enabled: boolean) {
		settingsStore.setHoverActionsEnabled(enabled);
		toastStore.success(
			enabled ? "Notification hover actions enabled" : "Notification hover actions disabled"
		);
	}

	$: isDark = $theme === "dark";
</script>

<div class="space-y-4">
	<!-- Theme Toggle -->
	<div class="flex items-center justify-between py-1">
		<div class="flex-1">
			<div class="flex items-center gap-2">
				<span class="text-md font-medium text-gray-900 dark:text-gray-100"> Dark mode </span>
			</div>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Switch between light and dark mode.
			</p>
		</div>
		<label class="relative flex items-center cursor-pointer">
			<input type="checkbox" checked={isDark} on:change={handleThemeToggle} class="sr-only peer" />
			<div
				class="relative w-9 h-5 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-blue-600 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600 peer-disabled:opacity-50"
			></div>
		</label>
	</div>

	<div class="flex items-center justify-between py-3 border-t border-gray-200 dark:border-gray-800">
		<div class="flex-1">
			<span class="text-md font-medium text-gray-900 dark:text-gray-100">Show total counts</span>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Show each view's total notifications alongside its unread count in the sidebar.
			</p>
		</div>
		<label class="relative flex items-center cursor-pointer">
			<input
				type="checkbox"
				aria-label="Show total counts"
				checked={$showTotalCounts}
				on:change={(e) => settingsStore.setShowTotalCounts(e.currentTarget.checked)}
				class="sr-only peer"
			/>
			<div
				class="relative w-9 h-5 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-blue-600 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600 peer-disabled:opacity-50"
			></div>
		</label>
	</div>

	<div class="flex items-center justify-between py-3 border-t border-gray-200 dark:border-gray-800">
		<div class="flex-1">
			<span class="text-md font-medium text-gray-900 dark:text-gray-100"
				>Open What's new by default</span
			>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Open the selected catch-up range first. Since last read falls back to the full view until a
				read checkpoint exists.
			</p>
		</div>
		<label class="relative flex items-center cursor-pointer">
			<input
				type="checkbox"
				aria-label="Open What's new by default"
				checked={$whatsNewDefault}
				on:change={(e) => settingsStore.setWhatsNewDefault(e.currentTarget.checked)}
				class="sr-only peer"
			/>
			<div
				class="relative w-9 h-5 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-blue-600 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600 peer-disabled:opacity-50"
			></div>
		</label>
	</div>

	<div
		class="flex items-center justify-between gap-6 py-3 border-t border-gray-200 dark:border-gray-800"
	>
		<div class="flex-1">
			<label
				for="whats-new-default-baseline"
				class="text-md font-medium text-gray-900 dark:text-gray-100"
			>
				What's new default range
			</label>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Choose the default catch-up range. Use Look back above the list to change the current range
				for all notifications.
			</p>
		</div>
		<select
			id="whats-new-default-baseline"
			aria-label="What's new default baseline"
			value={$whatsNewBaseline}
			on:change={(event) => {
				const value = event.currentTarget.value;
				const selection = WHATS_NEW_SELECTIONS.find((option) => option.value === value);
				if (selection) settingsStore.setWhatsNewBaseline(selection.value);
			}}
			class="h-9 rounded-lg border border-gray-300 bg-white px-3 text-sm text-gray-900 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 dark:border-gray-700 dark:bg-gray-950 dark:text-gray-100"
		>
			{#each WHATS_NEW_SELECTIONS as option (option.value)}
				<option value={option.value}>{option.label}</option>
			{/each}
		</select>
	</div>

	<!-- Timeline Auto-Scroll Toggle -->
	<div class="flex items-center justify-between py-3 border-t border-gray-200 dark:border-gray-800">
		<div class="flex-1">
			<div class="flex items-center gap-2">
				<span class="text-md font-medium text-gray-900 dark:text-gray-100">
					Auto-scroll to new timeline activity
				</span>
			</div>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Automatically scroll to the first unseen activity when opening the detail view.
			</p>
		</div>
		<label class="relative flex items-center cursor-pointer">
			<input
				type="checkbox"
				checked={$timelineAutoScroll}
				on:change={(e) => handleTimelineAutoScrollToggle(e.currentTarget.checked)}
				class="sr-only peer"
			/>
			<div
				class="relative w-9 h-5 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-blue-600 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600 peer-disabled:opacity-50"
			></div>
		</label>
	</div>

	<!-- Notification Hover Actions Toggle -->
	<div class="flex items-center justify-between py-3 border-t border-gray-200 dark:border-gray-800">
		<div class="flex-1">
			<div class="flex items-center gap-2">
				<span class="text-md font-medium text-gray-900 dark:text-gray-100">
					Show notification actions on hover
				</span>
			</div>
			<p class="text-xs text-gray-600 dark:text-gray-400 mt-1">
				Show quick-action buttons when the pointer moves over a notification in a list.
			</p>
		</div>
		<label class="relative flex items-center cursor-pointer">
			<input
				type="checkbox"
				checked={$hoverActionsEnabled}
				on:change={(e) => handleHoverActionsToggle(e.currentTarget.checked)}
				class="sr-only peer"
			/>
			<div
				class="relative w-9 h-5 bg-gray-300 dark:bg-gray-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-blue-600 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-blue-600 peer-disabled:opacity-50"
			></div>
		</label>
	</div>
</div>
