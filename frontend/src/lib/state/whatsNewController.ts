import { writable } from "svelte/store";
import { fetchWhatsNew } from "$lib/api/whatsNew";
import { buildWhatsNewReaderModel, compareWhatsNewTimestamps } from "$lib/utils/whatsNewGrouping";
import type { WhatsNewReaderModel } from "$lib/types/whatsNew";
import type { WhatsNewSelection } from "$lib/utils/whatsNewCutoff";

export interface WhatsNewOpenInput {
	githubId: string;
	cutoff: string | null;
	selection: WhatsNewSelection;
	readerUsername: string | null;
}

export function createWhatsNewController() {
	const status = writable<"no_baseline" | "loading" | "ready" | "error">("no_baseline");
	const model = writable<WhatsNewReaderModel | null>(null);
	const error = writable<string | null>(null);
	let current: WhatsNewOpenInput | null = null;
	let controller: AbortController | null = null;
	let generation = 0;

	async function load() {
		controller?.abort();
		controller = new AbortController();
		const signal = controller.signal;
		const request = ++generation;
		const input = current;
		model.set(null);
		error.set(null);
		if (!input || input.cutoff === null) {
			status.set("no_baseline");
			return;
		}
		status.set("loading");
		try {
			if (!input.githubId || !Number.isFinite(Date.parse(input.cutoff))) {
				throw new Error("A valid recorded timeline cutoff is required");
			}
			const snapshot = await fetchWhatsNew(
				input.githubId,
				input.cutoff,
				signal,
				undefined,
				input.selection
			);
			if (request !== generation || signal.aborted) return;
			if (
				snapshot.notificationId !== input.githubId ||
				(snapshot.selection === "my_last_comment"
					? input.selection !== "my_last_comment"
					: compareWhatsNewTimestamps(snapshot.since, input.cutoff) !== 0)
			) {
				throw new Error("Activity response does not match the selected notification and cutoff");
			}
			model.set(buildWhatsNewReaderModel(snapshot, input.selection, input.readerUsername));
			status.set("ready");
		} catch (err) {
			if (request !== generation || signal.aborted) return;
			console.error("[WhatsNewController] Failed to load activity:", err);
			error.set(err instanceof Error ? err.message : "Unable to load what's new");
			status.set("error");
		}
	}

	return {
		stores: { status, model, error },
		actions: {
			open(input: WhatsNewOpenInput) {
				current = { ...input };
				return load();
			},
			retry: load,
			reset() {
				controller?.abort();
				controller = null;
				current = null;
				generation++;
				model.set(null);
				error.set(null);
				status.set("no_baseline");
			},
		},
	};
}

export type WhatsNewController = ReturnType<typeof createWhatsNewController>;
