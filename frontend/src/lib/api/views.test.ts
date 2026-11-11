import { describe, expect, it, vi } from "vitest";
import { ApiError } from "./fetch";
import { deleteView, ViewLinkedRulesError } from "./views";

describe("deleteView conflicts", () => {
	it("reports dependent views without offering linked-rule force deletion", async () => {
		const fetchImpl = vi.fn<typeof fetch>().mockResolvedValue(
			new Response(JSON.stringify({ error: 'view is referenced by other views: "Actionable"' }), {
				status: 409,
			})
		);
		const error = await deleteView("noise", false, fetchImpl).catch((error: unknown) => error);
		expect(error).toBeInstanceOf(ApiError);
		expect(error).not.toBeInstanceOf(ViewLinkedRulesError);
		expect(error).toMatchObject({
			status: 409,
			message: 'view is referenced by other views: "Actionable"',
		});
		expect(fetchImpl).toHaveBeenCalledTimes(1);
	});

	it("preserves the linked-rule confirmation", async () => {
		const fetchImpl = vi
			.fn<typeof fetch>()
			.mockResolvedValue(new Response(JSON.stringify({ linkedRuleCount: 2 }), { status: 409 }));
		await expect(deleteView("noise", false, fetchImpl)).rejects.toMatchObject({
			name: "ViewLinkedRulesError",
			linkedRuleCount: 2,
		});
	});
});
