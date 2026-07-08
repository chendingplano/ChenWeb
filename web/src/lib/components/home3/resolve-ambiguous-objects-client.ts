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

export type CandidateFieldMatchKey =
	| 'canonical_name'
	| 'canonical_name_en'
	| 'canonical_name_zh'
	| 'aliases'
	| 'acronyms';

export type CandidateMatchDetails = {
	matchedFields: Partial<Record<CandidateFieldMatchKey, string[]>>;
	matchedTerms: string[];
	hasLexicalMatch: boolean;
	hasMatch: boolean;
	objectTypeMatched: boolean;
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

export type CreateObjectNodeFields = {
	object_name: string;
	object_name_en: string;
	object_name_zh: string;
	language: string;
	object_type: string;
	aliases: string[];
	acronyms: string[];
	description: string;
};

export type CreateObjectNodeResult =
	| { status: boolean; created: true; node: ObjectNodeCandidate }
	| { status: boolean; created: false; nodes: ObjectNodeCandidate[] };

/**
 * Calls the "Create New" action for an ambiguous artifact object: if a
 * kb.object_nodes row already has this canonical_name, it comes back as
 * `nodes` instead of being duplicated; otherwise a new node is created and
 * returned as `node`.
 */
export function createObjectNode(
	id: number,
	fields: CreateObjectNodeFields
): Promise<CreateObjectNodeResult> {
	return req(`/api/v1/kb/objects/ambiguous/${id}/create-node`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(fields)
	});
}

/**
 * Merges one object node into another via POST /api/v1/kb/objects/merge: the
 * loser node's kb.artifact_objects mentions are repointed to the survivor and
 * the loser is marked reconcile_status='merged' (so it is no longer considered
 * when resolving artifact objects). Each call writes one merge_nodes audit row
 * to kb.object_audit_log.
 */
export function mergeObjectNodes(
	survivorObjectId: string,
	loserObjectId: string
): Promise<{ status: boolean; survivor_object_id: string; repointed_mentions: number }> {
	return req('/api/v1/kb/objects/merge', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ survivor_object_id: survivorObjectId, loser_object_id: loserObjectId })
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
 * Index to select, in the id list with `resolvedId` already removed, right
 * after resolving it: the row that took its place (the one that used to
 * follow it), or the new last row if `resolvedId` was last. Returns null
 * when no rows remain.
 */
export function nextIndexAfterResolve(idsBeforeResolve: number[], resolvedId: number): number | null {
	const resolvedIndex = idsBeforeResolve.indexOf(resolvedId);
	const remaining = idsBeforeResolve.length - 1;
	if (resolvedIndex === -1 || remaining <= 0) return null;
	return Math.min(resolvedIndex, remaining - 1);
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

function normalizeObjectName(raw: string): string {
	const trimmed = raw.trim();
	if (!trimmed) return '';
	return trimmed
		.toLowerCase()
		.replace(/[._\-\/]+/g, ' ')
		.replace(/\s+/g, ' ')
		.trim();
}

function normalizeObjectToken(raw: string): string {
	return normalizeObjectName(raw).replaceAll(' ', '_');
}

function appendNormalizedTerms(target: string[], values: string | string[]): void {
	const items = Array.isArray(values) ? values : [values];
	for (const value of items) {
		const normalized = normalizeObjectName(value);
		if (normalized && !target.includes(normalized)) target.push(normalized);
	}
}

function artifactNormalizedTerms(obj: ArtifactObjectDetail): string[] {
	const out: string[] = [];
	appendNormalizedTerms(out, obj.object_name);
	appendNormalizedTerms(out, obj.object_name_en);
	appendNormalizedTerms(out, obj.object_name_zh);
	appendNormalizedTerms(out, obj.aliases);
	appendNormalizedTerms(out, obj.acronyms);
	return out;
}

function matchedTermsForField(
	artifactTerms: Set<string>,
	values: string | string[]
): string[] {
	const fieldTerms: string[] = [];
	appendNormalizedTerms(fieldTerms, values);
	return fieldTerms.filter((term) => artifactTerms.has(term));
}

function objectTypesCompatible(a: string, b: string): boolean {
	const left = normalizeObjectToken(a);
	const right = normalizeObjectToken(b);
	return !left || !right || left === 'other' || right === 'other' || left === right;
}

export function describeCandidateMatches(
	artifact: ArtifactObjectDetail,
	candidate: ObjectNodeCandidate
): CandidateMatchDetails {
	const artifactTerms = new Set(artifactNormalizedTerms(artifact));
	const matchedFields: Partial<Record<CandidateFieldMatchKey, string[]>> = {};
	for (const [field, values] of [
		['canonical_name', candidate.canonical_name],
		['canonical_name_en', candidate.canonical_name_en],
		['canonical_name_zh', candidate.canonical_name_zh],
		['aliases', candidate.aliases],
		['acronyms', candidate.acronyms]
	] as const) {
		const matched = matchedTermsForField(artifactTerms, values);
		if (matched.length > 0) matchedFields[field] = matched;
	}
	const matchedTerms = [...new Set(Object.values(matchedFields).flat())];
	const hasLexicalMatch = matchedTerms.length > 0;
	const objectTypeMatched = objectTypesCompatible(artifact.object_type, candidate.object_type);
	return {
		matchedFields,
		matchedTerms,
		hasLexicalMatch,
		hasMatch: hasLexicalMatch,
		objectTypeMatched
	};
}
