// Client for the WPI endpoints (server/api/wpi).
import type { ActionRef, Bundle, Issue, Json, JsonObject, Text } from './types';

export class WpiError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly issues: Issue[] = [],
		readonly fieldErrors: Record<string, string[]> | null = null
	) {
		super(message);
	}
}

async function parse(res: Response): Promise<Record<string, unknown>> {
	try {
		return (await res.json()) as Record<string, unknown>;
	} catch {
		return {};
	}
}

export async function fetchApp(appId: string, fetchFn: typeof fetch = fetch): Promise<Bundle> {
	const res = await fetchFn(`/api/v1/wpi/apps/${encodeURIComponent(appId)}`, { credentials: 'same-origin' });
	const body = await parse(res);
	if (!res.ok) {
		throw new WpiError(String(body.error ?? res.statusText), res.status, (body.errors as Issue[]) ?? []);
	}
	return body as unknown as Bundle;
}

export type SaveResult = { key_attribute: string; key_value: Json };

// Runs a load or save instance named by its place in a stored page. Throws
// WpiError; a save rejected by its field rules carries fieldErrors.
export async function runAction(
	ref: ActionRef,
	args: JsonObject = {},
	inputDoc?: JsonObject,
	fetchFn: typeof fetch = fetch
): Promise<Record<string, Json>> {
	const res = await fetchFn('/api/v1/wpi/actions/run', {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ ref, args, input_doc: inputDoc })
	});
	const body = await parse(res);
	if (!res.ok) {
		const fieldErrors = (body.field_errors as Record<string, string[]> | undefined) ?? null;
		throw new WpiError(String(body.error ?? res.statusText), res.status, [], fieldErrors);
	}
	return body as Record<string, Json>;
}

// An instance attribute of an element class row (format 0.3). default_value
// null: every instance must set it; left out: optional, no default.
export type ElementAttribute = { help_tip?: Text; default_value?: Json };

// The latest draft or active version of an element class, for the editor's
// palette and properties panel. A draft is defined but has no code yet.
export type ElementClassSummary = {
	descriptor_id: string;
	version: number;
	status: 'draft' | 'active';
	description: Text | null;
	attributes: Record<string, ElementAttribute>;
};

export async function listElementClasses(fetchFn: typeof fetch = fetch): Promise<ElementClassSummary[]> {
	const res = await fetchFn('/api/v1/wpi/classes/element', { credentials: 'same-origin' });
	const body = await parse(res);
	if (!res.ok) {
		throw new WpiError(String(body.error ?? res.statusText), res.status);
	}
	return (body.classes as ElementClassSummary[]) ?? [];
}

// Stores a new element class as a draft. Throws WpiError: 409 when the class
// ID already exists, 400 with issues when the draft breaks a rule.
export async function createElementClass(classRow: JsonObject, fetchFn: typeof fetch = fetch): Promise<void> {
	const res = await fetchFn('/api/v1/wpi/classes/element', {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(classRow)
	});
	if (!res.ok) {
		const body = await parse(res);
		throw new WpiError(String(body.error ?? res.statusText), res.status, (body.errors as Issue[]) ?? []);
	}
}

// The latest draft or active version of an element class, whole, for the
// editor's edit form.
export type ElementClassRow = { version: number; status: 'draft' | 'active'; class: JsonObject };

export async function getElementClass(classId: string, fetchFn: typeof fetch = fetch): Promise<ElementClassRow> {
	const res = await fetchFn(`/api/v1/wpi/classes/element/${encodeURIComponent(classId)}`, { credentials: 'same-origin' });
	const body = await parse(res);
	if (!res.ok) {
		throw new WpiError(String(body.error ?? res.statusText), res.status);
	}
	return body as unknown as ElementClassRow;
}

// Updates an existing element class in place, keeping its version and status: a
// draft stays a draft, an active class must still match its code. Throws
// WpiError: 404 when the class has no draft or active row, 400 with issues
// when the row breaks a rule.
export async function updateElementClass(
	classId: string,
	classRow: JsonObject,
	fetchFn: typeof fetch = fetch
): Promise<{ version: number; status: 'draft' | 'active' }> {
	const res = await fetchFn(`/api/v1/wpi/classes/element/${encodeURIComponent(classId)}`, {
		method: 'PUT',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(classRow)
	});
	const body = await parse(res);
	if (!res.ok) {
		throw new WpiError(String(body.error ?? res.statusText), res.status, (body.errors as Issue[]) ?? []);
	}
	return body as { version: number; status: 'draft' | 'active' };
}
