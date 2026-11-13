import { expect, it } from "vitest";
import { classifyWhatsNewAuthor } from "./whatsNewAuthors";

it("uses GitHub's actor type and the standard bot suffix without Copilot-specific rules", () => {
	expect(classifyWhatsNewAuthor("Copilot", "bot")).toBe("bot");
	expect(classifyWhatsNewAuthor("copilot[bot]", "unknown")).toBe("bot");
	expect(classifyWhatsNewAuthor("Copilot", "unknown")).toBe("human");
	expect(classifyWhatsNewAuthor("participant", "human")).toBe("human");
	expect(classifyWhatsNewAuthor(undefined, "unknown")).toBe("unknown");
});
