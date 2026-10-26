// Client for the WPI endpoints (server/api/wpi).
import type { ActionRef, Bundle, Issue, Json, JsonObject } from './types';

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

// Runs a stored load or save action. Throws WpiError; a save rejected by its
// field rules carries fieldErrors.
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
