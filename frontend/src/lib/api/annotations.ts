import { fetchWithAuth, apiErrorFromResponse } from "./fetch";
import type { NotificationView } from "./types";

export interface AnnotationInput {
	name: string;
	code: string;
	viewIds: string[];
	enabled: boolean;
}

export interface Annotation extends AnnotationInput {
	id: string;
}

export async function fetchAnnotations(): Promise<Annotation[]> {
	const response = await fetchWithAuth("/api/annotations");
	if (!response.ok) throw await apiErrorFromResponse(response, "Failed to load annotations");
	const data: { annotations: Annotation[] } = await response.json();
	return data.annotations;
}

export async function saveAnnotation(input: AnnotationInput, id?: string): Promise<Annotation> {
	const response = await fetchWithAuth(
		id ? `/api/annotations/${encodeURIComponent(id)}` : "/api/annotations",
		{
			method: id ? "PUT" : "POST",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify(input),
		}
	);
	if (!response.ok) throw await apiErrorFromResponse(response, "Failed to save annotation");
	const data: { annotation: Annotation } = await response.json();
	return data.annotation;
}

export async function deleteAnnotation(id: string): Promise<void> {
	const response = await fetchWithAuth(`/api/annotations/${encodeURIComponent(id)}`, {
		method: "DELETE",
	});
	if (!response.ok) throw await apiErrorFromResponse(response, "Failed to delete annotation");
}

export async function fetchAnnotationViews(): Promise<NotificationView[]> {
	const response = await fetchWithAuth("/api/views");
	if (!response.ok) throw await apiErrorFromResponse(response, "Failed to load annotation views");
	const data: { views: NotificationView[] } = await response.json();
	return data.views;
}
