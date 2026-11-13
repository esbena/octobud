import type { WhatsNewAuthorType } from "$lib/types/whatsNew";

export function classifyWhatsNewAuthor(
	login: string | undefined,
	sourceType: WhatsNewAuthorType | undefined
): WhatsNewAuthorType {
	if (sourceType === "bot") return "bot";
	const handle = login?.trim().toLowerCase();
	if (!handle) return "unknown";
	if (handle.includes("[bot]")) return "bot";
	return "human";
}
