import { describe, expect, it, vi } from "vitest";
import { fetchAnnotations, saveAnnotation, deleteAnnotation } from "./annotations";
import { fetchNotifications } from "./notifications";
import { fetchWithAuth } from "./fetch";

vi.mock("./fetch", async (importOriginal) => ({
	...(await importOriginal<typeof import("./fetch")>()),
	fetchWithAuth: vi.fn(),
}));

describe("annotation API", () => {
	it("loads, creates, updates and deletes server definitions", async () => {
		const input = { name: "Body", code: "() => []", viewIds: ["__inbox__"], enabled: true };
		const annotation = { id: "a1", ...input };
		vi.mocked(fetchWithAuth).mockResolvedValueOnce(Response.json({ annotations: [annotation] }));
		await expect(fetchAnnotations()).resolves.toEqual([annotation]);
		vi.mocked(fetchWithAuth).mockResolvedValueOnce(Response.json({ annotation }));
		await expect(saveAnnotation(input)).resolves.toEqual(annotation);
		expect(fetchWithAuth).toHaveBeenLastCalledWith(
			"/api/annotations",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify(input),
			})
		);
		vi.mocked(fetchWithAuth).mockResolvedValueOnce(Response.json({ annotation }));
		await saveAnnotation(input, "a1");
		expect(fetchWithAuth).toHaveBeenLastCalledWith(
			"/api/annotations/a1",
			expect.objectContaining({
				method: "PUT",
			})
		);
		vi.mocked(fetchWithAuth).mockResolvedValueOnce(new Response(null, { status: 204 }));
		await deleteAnnotation("a1");
		expect(fetchWithAuth).toHaveBeenLastCalledWith("/api/annotations/a1", { method: "DELETE" });
	});

	it("propagates server errors rather than returning an empty list", async () => {
		vi.mocked(fetchWithAuth).mockResolvedValueOnce(
			Response.json({ error: "Database unavailable" }, { status: 500 })
		);
		await expect(fetchAnnotations()).rejects.toThrow("Database unavailable");
	});

	it("requests stored subjects and preserves full notification JSON for annotations", async () => {
		const backendNotification = {
			id: 1,
			githubId: "g1",
			repositoryId: 2,
			subjectTitle: "PR",
			subjectType: "PullRequest",
			subjectRaw: { body: "Stored description" },
			payload: { reason: "mention", arbitrary: true },
			subjectFetchedAt: "2026-11-10T00:00:00Z",
			githubLastReadAt: "2026-11-09T00:00:00Z",
			importedAt: "2026-11-10T00:00:00Z",
		};
		vi.mocked(fetchWithAuth).mockResolvedValueOnce(
			Response.json({
				notifications: [backendNotification],
				total: 1,
				page: 1,
				pageSize: 30,
			})
		);
		const page = await fetchNotifications({ filters: { query: "is:unread" } });
		expect(fetchWithAuth).toHaveBeenLastCalledWith(
			expect.stringContaining("includeSubject=true"),
			{},
			undefined
		);
		expect(page.items[0].raw).toEqual(backendNotification);
		expect(page.items[0].payload).toEqual(backendNotification.payload);
		expect(page.items[0].subjectRaw).toEqual({ body: "Stored description" });
		expect(page.items[0].subjectFetchedAt).toBe("2026-11-10T00:00:00Z");
	});
});
