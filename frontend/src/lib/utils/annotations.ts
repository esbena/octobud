import type { Notification } from "$lib/api/types";

export interface AnnotationLabel {
	text: string;
	color: string;
	hoverText?: string;
}

export interface AnnotationContext {
	notification: Notification;
	subject: unknown;
}

type AnnotationFunction = (input: AnnotationContext) => unknown;

export function annotationErrorMessage(cause: unknown): string {
	try {
		const message = String(cause);
		if (message.trim()) return message;
	} catch {
		// User code can throw values that cannot be converted to text.
	}
	return "Annotation failed without an error message.";
}

export function createAnnotationContext(notification: Notification): AnnotationContext {
	const snapshot = structuredClone(notification);
	return { notification: snapshot, subject: snapshot.subjectRaw ?? null };
}

export function compileAnnotation(code: string): AnnotationFunction {
	const result: unknown = new Function(`"use strict"; return (\n${code}\n);`)();
	if (typeof result !== "function") {
		throw new Error("JavaScript must be a function expression.");
	}
	return result as AnnotationFunction;
}

export async function runAnnotation(
	fn: AnnotationFunction,
	notification: Notification
): Promise<AnnotationLabel[]> {
	const result: unknown = await fn(createAnnotationContext(notification));
	if (!Array.isArray(result)) throw new Error("Return an array of labels (or [] for no labels).");
	return Array.from(result, (item: unknown) => {
		if (!item || typeof item !== "object") throw new Error("Each label must be an object.");
		const label = item as Record<string, unknown>;
		if (typeof label.text !== "string" || !label.text.trim()) {
			throw new Error("Each label needs non-empty text.");
		}
		if (typeof label.color !== "string" || !/^#[0-9a-f]{6}$/i.test(label.color)) {
			throw new Error("Each label needs a color in #RRGGBB format.");
		}
		const { hoverText } = label;
		const result = { text: label.text, color: label.color };
		if (hoverText === undefined) return result;
		if (typeof hoverText !== "string") {
			throw new Error("Label hoverText must be plain text.");
		}
		return { ...result, hoverText };
	});
}

export function labelTextColor(color: string): string {
	const channels = [1, 3, 5].map((offset) => {
		const value = parseInt(color.slice(offset, offset + 2), 16) / 255;
		return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
	});
	const luminance = channels[0] * 0.2126 + channels[1] * 0.7152 + channels[2] * 0.0722;
	return luminance > 0.179 ? "#000000" : "#ffffff";
}
