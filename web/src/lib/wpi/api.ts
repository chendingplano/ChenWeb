// Client for the WPI endpoints (server/api/wpi).
import type { ActionInstance, ActionRef, Bundle, Issue, Json, JsonObject, Text } from './types';

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

export async function fetchPagePreview(pageId: string): Promise<Bundle> {
 const res = await fetch(`/api/v1/wpi/pages/${encodeURIComponent(pageId)}/preview`, { credentials: 'same-origin' });
 const body = await parse(res);
 if (!res.ok) throw new WpiError(String(body.error ?? res.statusText), res.status, (body.errors as Issue[]) ?? []);
 return body as unknown as Bundle;
}

export type SaveResult = { key_attribute: string; key_value: Json };

// Runs a load or save instance named by its place in a stored page. Throws
// WpiError; a save rejected by its field rules carries fieldErrors.
export async function runAction(
	ref: ActionRef,
	args: JsonObject = {},
	inputDoc?: JsonObject,
	fetchFn: typeof fetch = fetch,
	signal?: AbortSignal
): Promise<Record<string, Json>> {
	const res = await fetchFn('/api/v1/wpi/actions/run', {
		method: 'POST',
		credentials: 'same-origin',
		signal,
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

// The editor previews a validated, read-only list source without storing a page.
export async function previewList(source: ActionInstance, args: JsonObject, signal?: AbortSignal): Promise<JsonObject[]> {
 const res = await fetch('/api/v1/wpi/actions/preview-list', {
  method: 'POST', credentials: 'same-origin', signal,
  headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ source, args })
 });
 const body = await parse(res);
 if (!res.ok) throw new WpiError(String(body.error ?? res.statusText), res.status);
 return (body.result ?? []) as JsonObject[];
}

// An instance attribute of an element class row (format 0.4). default_value
// null: every instance must set it; left out: optional, no default.
// instance_modifiable false: the class fixes default_value for every instance.
export type ElementAttribute = { help_tip?: Text; default_value?: Json; instance_modifiable?: boolean };

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

export async function listPageLayouters(fetchFn: typeof fetch = fetch): Promise<JsonObject[]> {
 const res = await fetchFn('/api/v1/wpi/classes/page-layouter', { credentials: 'same-origin' });
 const body = await parse(res);
 if (!res.ok) throw new WpiError(String(body.error ?? res.statusText), res.status);
 return (body.classes as JsonObject[]) ?? [];
}

// A row of kb.code_modules: the code that draws elements, performs actions or
// lays out pages. An element class may name only an active element module.
export type CodeModule = {
	code_module_id: string;
	module_name: string;
	description: string;
	module_type: 'element' | 'action' | 'page' | 'page-layouter' | 'element-layouter';
	module_doc: string;
	notes: string;
	status: 'active' | 'suspended';
};

export async function listCodeModules(fetchFn: typeof fetch = fetch): Promise<CodeModule[]> {
	const res = await fetchFn('/api/v1/wpi/code-modules', { credentials: 'same-origin' });
	const body = await parse(res);
	if (!res.ok) {
		throw new WpiError(String(body.error ?? res.statusText), res.status);
	}
	return (body.code_modules as CodeModule[]) ?? [];
}

// Stores a new element class as a draft and returns its warnings: attributes
// its code module, when it names a registered one, does not support. Throws
// WpiError: 409 when the class ID already exists, 400 with issues when the
// draft breaks a rule.
export async function createElementClass(classRow: JsonObject, fetchFn: typeof fetch = fetch): Promise<Issue[]> {
	const res = await fetchFn('/api/v1/wpi/classes/element', {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(classRow)
	});
	const body = await parse(res);
	if (!res.ok) {
		throw new WpiError(String(body.error ?? res.statusText), res.status, (body.errors as Issue[]) ?? []);
	}
	return (body.warnings as Issue[] | undefined) ?? [];
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
// draft stays a draft, an active class must still name a registered code
// module. The warnings are attributes that module does not support. Throws
// WpiError: 404 when the class has no draft or active row, 400 with issues
// when the row breaks a rule.
export async function updateElementClass(
	classId: string,
	classRow: JsonObject,
	fetchFn: typeof fetch = fetch
): Promise<{ version: number; status: 'draft' | 'active'; warnings: Issue[] }> {
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
	const saved = body as { version: number; status: 'draft' | 'active'; warnings?: Issue[] };
	return { version: saved.version, status: saved.status, warnings: saved.warnings ?? [] };
}

export type PageSummary = {
	descriptor_id: string;
	version: number;
	page_name: Text | null;
	description: Text | null;
	created_at: string;
	updated_at: string;
};

export async function searchPages(
	params: URLSearchParams,
	signal?: AbortSignal
): Promise<{ pages: PageSummary[]; has_more: boolean }> {
	const res = await fetch(`/api/v1/wpi/pages?${params}`, { credentials: 'same-origin', signal });
	const body = await parse(res);
	if (!res.ok) throw new WpiError(String(body.error ?? res.statusText), res.status);
	return body as unknown as { pages: PageSummary[]; has_more: boolean };
}

export async function getPage(
	id: string,
	signal?: AbortSignal
): Promise<{ page: JsonObject; version: number }> {
	const res = await fetch(`/api/v1/wpi/pages/${encodeURIComponent(id)}`, {
		credentials: 'same-origin',
		signal
	});
	const body = await parse(res);
	if (!res.ok) throw new WpiError(String(body.error ?? res.statusText), res.status);
	return body as unknown as { page: JsonObject; version: number };
}

// Save a page as a new active descriptor version. Validation errors are returned to the author.
// Saves a page as a new version. The server copies the active classes into
// instances and layouter copies that have none, and returns the stored page
// (OpenSpec wpi-pages-own-panes, design D4).
export async function savePage(id: string, page: JsonObject, fetchFn: typeof fetch = fetch): Promise<{ version: number; page: JsonObject }> {
	const res = await fetchFn(`/api/v1/wpi/descriptors/page/${encodeURIComponent(id)}`, {
		method: 'PUT', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(page)
	});
	const body = await parse(res);
	if (!res.ok) throw new WpiError(String(body.error ?? res.statusText), res.status, (body.errors as Issue[]) ?? []);
	return { version: body.version as number, page: body.page as JsonObject };
}
