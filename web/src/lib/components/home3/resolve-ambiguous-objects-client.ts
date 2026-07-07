export type AmbiguousObjectSummary = {
	id: number;
	artifact_type: string;
	artifact_id: string;
	object_name: string;
	object_name_en: string;
	confidence: number;
};

export type ArtifactObjectDetail = {
	id: number;
	source_record_id: number;
	artifact_type: string;
	artifact_id: string;
	object_name: string;
	object_name_en: string;
	object_name_zh: string;
	language: string;
	object_type: string;
	object_role: string;
	aliases: string[];
	acronyms: string[];
	description: string;
	evidence_quote: string;
	object_id: string;
	reconcile_status: string;
	reconcile_confidence: number;
};

export type ObjectNodeCandidate = {
	object_id: string;
	canonical_name: string;
	canonical_name_en: string;
	canonical_name_zh: string;
	primary_language: string;
	object_type: string;
	aliases: string[];
	acronyms: string[];
	description: string;
	score: number;
	method: string;
	recommended: boolean;
};

export type AmbiguousObjectDetailResponse = {
	status: boolean;
	artifact_object: ArtifactObjectDetail;
	candidates: ObjectNodeCandidate[];
};

export const ARTIFACT_OBJECT_EDITABLE_FIELDS = [
	'object_name',
	'object_name_en',
	'object_name_zh',
	'language',
	'object_type',
	'object_role',
	'aliases',
	'acronyms',
	'description',
	'evidence_quote',
	'object_id',
	'reconcile_status',
	'reconcile_confidence'
] as const;

export const OBJECT_NODE_EDITABLE_FIELDS = [
	'canonical_name',
	'canonical_name_en',
	'canonical_name_zh',
	'primary_language',
	'object_type',
	'aliases',
	'acronyms',
	'description'
] as const;

export const RECONCILE_STATUS_OPTIONS = [
	'pending',
	'matched',
	'new',
	'ambiguous',
	'ambiguous_resolved',
	'rejected'
] as const;

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await res.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!res.ok) {
		const msg =
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export function listAmbiguousObjects(): Promise<{ status: boolean; rows: AmbiguousObjectSummary[] }> {
	return req('/api/v1/kb/objects/ambiguous');
}

export function getAmbiguousObjectDetail(id: number): Promise<AmbiguousObjectDetailResponse> {
	return req(`/api/v1/kb/objects/ambiguous/${id}`);
}

export function updateArtifactObject(
	id: number,
	patch: Record<string, unknown>
): Promise<{ status: boolean }> {
	return req(`/api/v1/kb/objects/artifact-objects/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(patch)
	});
}

export function updateObjectNode(
	objectId: string,
	patch: Record<string, unknown>
): Promise<{ status: boolean }> {
	return req(`/api/v1/kb/object-nodes/${encodeURIComponent(objectId)}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(patch)
	});
}

function diffFields<T extends Record<string, unknown>>(
	original: T,
	edited: T,
	fields: readonly string[]
): Record<string, unknown> {
	const patch: Record<string, unknown> = {};
	for (const field of fields) {
		const before = original[field];
		const after = edited[field];
		if (Array.isArray(before) && Array.isArray(after)) {
			if (JSON.stringify(before) !== JSON.stringify(after)) patch[field] = after;
		} else if (before !== after) {
			patch[field] = after;
		}
	}
	return patch;
}

/** Returns only the ARTIFACT_OBJECT_EDITABLE_FIELDS that differ between original and edited. */
export function buildArtifactObjectPatch(
	original: ArtifactObjectDetail,
	edited: ArtifactObjectDetail
): Record<string, unknown> {
	return diffFields(original, edited, ARTIFACT_OBJECT_EDITABLE_FIELDS);
}

/** Returns only the OBJECT_NODE_EDITABLE_FIELDS that differ between original and edited. */
export function buildObjectNodePatch(
	original: ObjectNodeCandidate,
	edited: ObjectNodeCandidate
): Record<string, unknown> {
	return diffFields(original, edited, OBJECT_NODE_EDITABLE_FIELDS);
}

/** Index math for Prev/Next within the left-panel id list. Returns null past either end. */
export function neighborAmbiguousId(ids: number[], currentId: number, direction: 1 | -1): number | null {
	const index = ids.indexOf(currentId);
	if (index === -1) return null;
	const nextIndex = index + direction;
	if (nextIndex < 0 || nextIndex >= ids.length) return null;
	return ids[nextIndex];
}

/**
 * True when `current` differs from `snapshot`. Array-aware and order-sensitive
 * (same comparison semantics as diffFields above), used to highlight
 * unsaved-edit state per field in the admin view.
 */
export function fieldDirty(current: unknown, snapshot: unknown): boolean {
	if (Array.isArray(current) && Array.isArray(snapshot)) {
		return JSON.stringify(current) !== JSON.stringify(snapshot);
	}
	return current !== snapshot;
}
