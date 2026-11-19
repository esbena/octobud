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

import { writable, get, type Writable } from "svelte/store";
import { browser } from "$app/environment";
import type { WhatsNewSelection } from "$lib/utils/whatsNewCutoff";
import { BUILT_IN_VIEWS } from "$lib/state/types";

const NOTIFICATIONS_ENABLED_KEY = "octobud:notifications:enabled";
const FAVICON_BADGE_ENABLED_KEY = "octobud:notifications:faviconBadge";
const TIMELINE_AUTO_SCROLL_KEY = "octobud:timeline:autoScroll";
const HOVER_ACTIONS_ENABLED_KEY = "octobud:notifications:hoverActions";
const SHOW_TOTAL_COUNTS_KEY = "octobud:sidebar:showTotalCounts";
const BUILT_IN_VIEW_VISIBLE_KEY = "octobud:sidebar:builtInViewVisible:";
const WHATS_NEW_DEFAULT_KEY = "octobud:timeline:whatsNewDefault";
const WHATS_NEW_BASELINE_KEY = "octobud:timeline:whatsNewBaseline";

function isWhatsNewSelection(value: string | null): value is WhatsNewSelection {
	return (
		value === "last_read" ||
		value === "last_archive" ||
		value === "my_last_comment" ||
		value === "today" ||
		value === "since_yesterday" ||
		value === "past_7_days"
	);
}

/**
 * Notification Settings Store
 * Manages notification preferences persisted to localStorage
 */
export function createNotificationSettingsStore() {
	const storage = typeof localStorage !== "undefined" ? localStorage : null;
	const readBuiltInViewVisibility = (): Record<string, boolean> =>
		Object.fromEntries(
			Object.values(BUILT_IN_VIEWS).map(({ slug }) => [
				slug,
				storage?.getItem(BUILT_IN_VIEW_VISIBLE_KEY + slug) !== "false",
			])
		);
	let persistedVisibility = readBuiltInViewVisibility();
	const builtInViewVisibility = writable(persistedVisibility);
	// Load from localStorage, default to true (enabled)
	const storedEnabled =
		storage
			? storage.getItem(NOTIFICATIONS_ENABLED_KEY) !== "false" // Default to true
			: true;

	const storedFaviconBadgeEnabled =
		storage
			? storage.getItem(FAVICON_BADGE_ENABLED_KEY) !== "false" // Default to true
			: true;

	// Auto-scroll defaults to false (opt-in feature)
	const storedTimelineAutoScroll =
		storage
			? storage.getItem(TIMELINE_AUTO_SCROLL_KEY) === "true"
			: false;

	const storedHoverActionsEnabled =
		storage
			? storage.getItem(HOVER_ACTIONS_ENABLED_KEY) !== "false"
			: true;

	const notificationsEnabled = writable<boolean>(storedEnabled);
	const faviconBadgeEnabled = writable<boolean>(storedFaviconBadgeEnabled);
	const timelineAutoScroll = writable<boolean>(storedTimelineAutoScroll);
	const hoverActionsEnabled = writable<boolean>(storedHoverActionsEnabled);
	const showTotalCounts = writable(
		!!storage && storage.getItem(SHOW_TOTAL_COUNTS_KEY) === "true"
	);
	const whatsNewDefault = writable(
		!!storage && storage.getItem(WHATS_NEW_DEFAULT_KEY) === "true"
	);
	const storedWhatsNewBaseline =
		storage ? storage.getItem(WHATS_NEW_BASELINE_KEY) : null;
	const defaultWhatsNewSelection: WhatsNewSelection = isWhatsNewSelection(storedWhatsNewBaseline)
		? storedWhatsNewBaseline
		: "last_read";
	const whatsNewBaseline = writable<WhatsNewSelection>(defaultWhatsNewSelection);
	const catchUpSelection = writable<WhatsNewSelection>(defaultWhatsNewSelection);
	whatsNewBaseline.subscribe((value) => {
		catchUpSelection.set(value);
		storage?.setItem(WHATS_NEW_BASELINE_KEY, value);
	});

	// Persist to localStorage and notify service worker
	if (browser && storage) {
		builtInViewVisibility.subscribe((visibility) => {
			for (const { slug } of Object.values(BUILT_IN_VIEWS)) {
				if (visibility[slug] !== persistedVisibility[slug]) {
					storage.setItem(BUILT_IN_VIEW_VISIBLE_KEY + slug, visibility[slug].toString());
				}
			}
			persistedVisibility = visibility;
		});
		notificationsEnabled.subscribe((value) => {
			storage.setItem(NOTIFICATIONS_ENABLED_KEY, value.toString());

			// Notify service worker of setting change
			if ("serviceWorker" in navigator && navigator.serviceWorker.controller) {
				navigator.serviceWorker.controller.postMessage({
					type: "NOTIFICATION_SETTING_CHANGED",
					enabled: value,
				});
			}
		});

		faviconBadgeEnabled.subscribe((value) => {
			storage.setItem(FAVICON_BADGE_ENABLED_KEY, value.toString());
		});

		timelineAutoScroll.subscribe((value) => {
			storage.setItem(TIMELINE_AUTO_SCROLL_KEY, value.toString());
		});
		hoverActionsEnabled.subscribe((value) => {
			storage.setItem(HOVER_ACTIONS_ENABLED_KEY, value.toString());
		});
		showTotalCounts.subscribe((value) => {
			storage.setItem(SHOW_TOTAL_COUNTS_KEY, value.toString());
		});
		whatsNewDefault.subscribe((value) => {
			storage.setItem(WHATS_NEW_DEFAULT_KEY, value.toString());
		});
	}

	return {
		notificationsEnabled: notificationsEnabled as Writable<boolean>,
		faviconBadgeEnabled: faviconBadgeEnabled as Writable<boolean>,
		timelineAutoScroll: timelineAutoScroll as Writable<boolean>,
		hoverActionsEnabled: hoverActionsEnabled as Writable<boolean>,
		showTotalCounts,
		builtInViewVisibility,
		whatsNewDefault,
		whatsNewBaseline,
		catchUpSelection,
		syncFromStorage: () => {
			if (!storage) return;
			builtInViewVisibility.set(readBuiltInViewVisibility());
			const newEnabled = storage.getItem(NOTIFICATIONS_ENABLED_KEY) !== "false";
			const newFaviconBadge = storage.getItem(FAVICON_BADGE_ENABLED_KEY) !== "false";
			const newAutoScroll = storage.getItem(TIMELINE_AUTO_SCROLL_KEY) === "true";
			const newHoverActions = storage.getItem(HOVER_ACTIONS_ENABLED_KEY) !== "false";
			const newShowTotalCounts = storage.getItem(SHOW_TOTAL_COUNTS_KEY) === "true";
			const newWhatsNewDefault = storage.getItem(WHATS_NEW_DEFAULT_KEY) === "true";
			const storedBaseline = storage.getItem(WHATS_NEW_BASELINE_KEY);
			const newWhatsNewBaseline = isWhatsNewSelection(storedBaseline)
				? storedBaseline
				: "last_read";
			if (newEnabled !== get(notificationsEnabled)) notificationsEnabled.set(newEnabled);
			if (newFaviconBadge !== get(faviconBadgeEnabled)) faviconBadgeEnabled.set(newFaviconBadge);
			if (newAutoScroll !== get(timelineAutoScroll)) timelineAutoScroll.set(newAutoScroll);
			if (newHoverActions !== get(hoverActionsEnabled)) {
				hoverActionsEnabled.set(newHoverActions);
			}
			if (newShowTotalCounts !== get(showTotalCounts)) showTotalCounts.set(newShowTotalCounts);
			if (newWhatsNewDefault !== get(whatsNewDefault)) whatsNewDefault.set(newWhatsNewDefault);
			if (newWhatsNewBaseline !== get(whatsNewBaseline)) {
				whatsNewBaseline.set(newWhatsNewBaseline);
			}
		},
		setEnabled: (enabled: boolean) => {
			notificationsEnabled.set(enabled);
		},
		setFaviconBadgeEnabled: (enabled: boolean) => {
			faviconBadgeEnabled.set(enabled);
		},
		setTimelineAutoScroll: (enabled: boolean) => {
			timelineAutoScroll.set(enabled);
		},
		setHoverActionsEnabled: (enabled: boolean) => {
			hoverActionsEnabled.set(enabled);
		},
		setShowTotalCounts: (enabled: boolean) => {
			showTotalCounts.set(enabled);
		},
		setBuiltInViewVisible: (slug: keyof typeof BUILT_IN_VIEWS, visible: boolean) => {
			builtInViewVisibility.update((visibility) => ({ ...visibility, [slug]: visible }));
		},
		setWhatsNewDefault: (enabled: boolean) => {
			whatsNewDefault.set(enabled);
		},
		setWhatsNewBaseline: (baseline: WhatsNewSelection) => {
			whatsNewBaseline.set(baseline);
		},
		setCatchUpSelection: (selection: WhatsNewSelection) => {
			catchUpSelection.set(selection);
		},
		toggle: () => {
			notificationsEnabled.update((enabled) => !enabled);
		},
		toggleFaviconBadge: () => {
			faviconBadgeEnabled.update((enabled) => !enabled);
		},
		toggleTimelineAutoScroll: () => {
			timelineAutoScroll.update((enabled) => !enabled);
		},
		toggleHoverActions: () => {
			hoverActionsEnabled.update((enabled) => !enabled);
		},
	};
}

// Singleton instance
let notificationSettingsStoreInstance: ReturnType<typeof createNotificationSettingsStore> | null =
	null;

export function getNotificationSettingsStore() {
	if (!notificationSettingsStoreInstance) {
		notificationSettingsStoreInstance = createNotificationSettingsStore();
	}
	return notificationSettingsStoreInstance;
}

// Helper to check if notifications are enabled (for service worker)
export function isNotificationEnabled(): boolean {
	if (!browser || typeof localStorage === "undefined") return true; // Default to enabled on server
	const stored = localStorage.getItem(NOTIFICATIONS_ENABLED_KEY);
	return stored !== "false"; // Default to true
}

// Helper to check if favicon badge is enabled
export function isFaviconBadgeEnabled(): boolean {
	if (!browser || typeof localStorage === "undefined") return true; // Default to enabled on server
	const stored = localStorage.getItem(FAVICON_BADGE_ENABLED_KEY);
	return stored !== "false"; // Default to true
}

// Helper to check if timeline auto-scroll is enabled
export function isTimelineAutoScrollEnabled(): boolean {
	if (!browser || typeof localStorage === "undefined") return false; // Default to disabled on server
	const stored = localStorage.getItem(TIMELINE_AUTO_SCROLL_KEY);
	return stored === "true"; // Default to false
}
