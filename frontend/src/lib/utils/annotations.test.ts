import { describe, expect, it } from "vitest";
import type { Notification } from "$lib/api/types";
import { compileAnnotation, runAnnotation, labelTextColor } from "./annotations";

const notification: Notification = {
	id: "1",
	repoFullName: "owner/repo",
	subjectTitle: "Update",
	subjectType: "pull_request",
	reason: "subscribed",
	archived: false,
	isRead: false,
	muted: false,
	starred: false,
	updatedAt: "2026-11-10T00:00:00Z",
	labels: [],
	viewIds: [],
	subjectRaw: { body: "BREAKING CHANGE: update configuration" },
	payload: { customFact: "available" },
};

describe("annotation functions", () => {
	it("runs a function expression against the stored body and original payload", async () => {
		const fn =
			compileAnnotation(`({ notification, subject }) => subject.body.includes("BREAKING CHANGE")
			? [{ text: "⚠️ Breaking", color: "#d93f0b", hoverText: notification.payload.customFact }]
			: []`);
		await expect(runAnnotation(fn, notification)).resolves.toEqual([
			{ text: "⚠️ Breaking", color: "#d93f0b", hoverText: "available" },
		]);
	});

	it("awaits async functions", async () => {
		const fn = compileAnnotation(`async function ({ subject }) {
			await Promise.resolve();
			return [{ text: subject.body, color: "#ABCDEF" }];
		}`);
		await expect(runAnnotation(fn, notification)).resolves.toEqual([
			{
				text: notification.subjectRaw && (notification.subjectRaw as { body: string }).body,
				color: "#ABCDEF",
			},
		]);
	});

	it("provides null for an unavailable subject", async () => {
		await expect(
			runAnnotation(compileAnnotation("({ subject }) => subject === null ? [] : null"), {
				...notification,
				subjectRaw: undefined,
			})
		).resolves.toEqual([]);
	});

	it("does not mutate the notification or subject", async () => {
		await runAnnotation(
			compileAnnotation(`({ notification, subject }) => {
			notification.subjectTitle = "changed";
			subject.body = "changed";
			return [];
		}`),
			notification
		);
		expect(notification.subjectTitle).toBe("Update");
		expect(notification.subjectRaw).toEqual({ body: "BREAKING CHANGE: update configuration" });
	});

	it.each(["", "async (", "const f = () => []", "42", "({})"])(
		"rejects invalid expressions: %s",
		(code) => {
			expect(() => compileAnnotation(code)).toThrow();
		}
	);

	it.each([
		"null",
		"undefined",
		"{}",
		"'label'",
		"[null]",
		"Array(1)",
		`[{text: 'x', color: '#ffffff'}, ,]`,
		"[{}]",
		`[{text: '', color: '#ffffff'}]`,
		`[{text: 'x', color: 'red'}]`,
		`[{text: 'x', color: '#fff'}]`,
		`[{text: 'x', color: '#ffffff; display: none'}]`,
		`[{text: 'x', color: '#ffffff', hoverText: { html: 'x' }}]`,
	])("rejects invalid label results: %s", async (output) => {
		await expect(
			runAnnotation(compileAnnotation(`() => (${output})`), notification)
		).rejects.toThrow();
	});

	it("propagates synchronous exceptions and async rejections", async () => {
		for (const code of [
			`() => { throw new Error("failed"); }`,
			`async () => { throw new Error("failed"); }`,
		]) {
			await expect(runAnnotation(compileAnnotation(code), notification)).rejects.toThrow("failed");
		}
	});

	it("keeps HTML and emoji shortcodes as literal text", async () => {
		await expect(
			runAnnotation(
				compileAnnotation(`() => [{
			text: "<b>:warning:</b>", color: "#ffffff", hoverText: "<img src=x>"
		}]`),
				notification
			)
		).resolves.toEqual([{ text: "<b>:warning:</b>", color: "#ffffff", hoverText: "<img src=x>" }]);
	});

	it("chooses contrasting text colors", () => {
		expect(labelTextColor("#ffffff")).toBe("#000000");
		expect(labelTextColor("#000000")).toBe("#ffffff");
	});
});
