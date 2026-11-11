import { describe, expect, it } from "vitest";
import { tokenizeForSyntax } from "./querySyntaxTokenizer";

describe("tokenizeForSyntax", () => {
	it.each([
		'"Keep visible"',
		'"Keep (AND OR NOT) visible"',
		String.raw`"Keep \"visible\""`,
		String.raw`"Path C:\\"`,
		'"\u{1f4ac} Team \u00e9"',
		'inbox,"Keep visible","Another view"',
	])("keeps the quoted field value intact: %s", (value) => {
		const query = `-in:${value} (is:filtered OR reason:team_mention)`;
		const tokens = tokenizeForSyntax(query);

		expect(tokens.slice(0, 4)).toEqual([
			{ type: "negation", text: "-" },
			{ type: "key", text: "in" },
			{ type: "colon", text: ":" },
			{ type: "value", text: value },
		]);
		expect(tokens.slice(4, 8)).toEqual([
			{ type: "whitespace", text: " " },
			{ type: "paren", text: "(" },
			{ type: "key", text: "is" },
			{ type: "colon", text: ":" },
		]);
		expect(tokens.map((token) => token.text).join("")).toBe(query);
	});

	it.each(['"Keep visible (OR', '"Keep visible\\'])(
		"highlights an unfinished quoted value while typing: %s",
		(value) => {
			expect(tokenizeForSyntax(`in:${value}`)).toEqual([
				{ type: "key", text: "in" },
				{ type: "colon", text: ":" },
				{ type: "value", text: value },
			]);
		}
	);

	it("preserves unquoted values and surrounding operators", () => {
		expect(tokenizeForSyntax("in:inbox,filtered OR (is:unread)")).toEqual([
			{ type: "key", text: "in" },
			{ type: "colon", text: ":" },
			{ type: "value", text: "inbox,filtered" },
			{ type: "whitespace", text: " " },
			{ type: "operator", text: "OR" },
			{ type: "whitespace", text: " " },
			{ type: "paren", text: "(" },
			{ type: "key", text: "is" },
			{ type: "colon", text: ":" },
			{ type: "value", text: "unread" },
			{ type: "paren", text: ")" },
		]);
	});
});
