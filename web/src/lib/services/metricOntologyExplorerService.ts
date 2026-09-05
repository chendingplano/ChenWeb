// Read wrappers for the Metric Ontology Explorer record tabs.
//
// Only the five chain nodes whose kb.* table already has a read endpoint are
// wired here. The rest render a schema panel (see model.ts — no `loaderKey`)
// until a follow-up change adds their endpoints.

import { searchObjects, type ArtifactObjectSummary, type ObjectNodeSummary } from './objectManagerService';
import type { LoaderKey } from '$lib/components/home3/metric-ontology-explorer/model';

export type Cell = string | number | null;
export type RecordRows = { rows: Cell[][] };

async function getJson<T>(url: string, fallback: string): Promise<T> {
	const res = await fetch(url, { method: 'GET', credentials: 'same-origin' });
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `${fallback} (${res.status})`;
		throw new Error(msg);
	}
	return res.json() as Promise<T>;
}

const PAGE = 25;
const dash = (v: unknown): Cell => (v === null || v === undefined || v === '' ? '—' : (v as Cell));
const clip = (v: unknown, n = 90): Cell => {
	const s = v == null ? '' : String(v);
	return s.length > n ? s.slice(0, n - 1) + '…' : dash(s);
};

async function loadArtifactObjects(): Promise<RecordRows> {
	const { rows } = await searchObjects({ table: 'artifact_objects', page_size: PAGE });
	return {
		rows: (rows as ArtifactObjectSummary[]).map((r) => [
			r.id,
			dash(r.object_name_en || r.object_name),
			`${r.artifact_type} · ${r.artifact_id}`,
			dash(r.object_id),
			dash(r.reconcile_status)
		])
	};
}

async function loadObjectNodes(): Promise<RecordRows> {
	const { rows } = await searchObjects({ table: 'object_nodes', page_size: PAGE });
	return {
		rows: (rows as ObjectNodeSummary[]).map((r) => [
			r.id,
			dash(r.canonical_name_en || r.canonical_name),
			dash(r.object_type),
			dash(r.object_id),
			dash(r.reconcile_status)
		])
	};
}

type KeywordConcept = {
	concept_id: string;
	pref_label: string;
	status: string;
	scope: string;
	gloss: string | null;
};
async function loadKeywordConcepts(): Promise<RecordRows> {
	const data = await getJson<{ results: KeywordConcept[] }>(
		'/api/v1/kb/keyword-concepts',
		'failed to list keyword concepts'
	);
	return {
		rows: (data.results ?? []).slice(0, PAGE).map((c) => [
			c.concept_id,
			dash(c.pref_label),
			dash(c.status),
			dash(c.scope),
			clip(c.gloss)
		])
	};
}

type OntologyTerm = {
	term_id: string;
	term_kind: string;
	module_id: string;
	status: string;
	definition: string;
};
async function loadOntologyTerms(): Promise<RecordRows> {
	const data = await getJson<{ results: OntologyTerm[] }>(
		'/api/v1/kb/ontology/terms',
		'failed to list ontology terms'
	);
	return {
		rows: (data.results ?? []).slice(0, PAGE).map((t) => [
			t.term_id,
			dash(t.term_kind),
			dash(t.module_id),
			dash(t.status),
			clip(t.definition)
		])
	};
}

type SemanticAssertion = {
	id: number;
	subject_ref_id?: string;
	subject_object_id?: string;
	predicate_term_id?: string;
	object_ref_id?: string;
	object_object_id?: string;
	assertion_kind_term_id?: string;
	confidence?: number | null;
};
async function loadSemanticAssertions(): Promise<RecordRows> {
	const data = await getJson<{ results: SemanticAssertion[] }>(
		`/api/v1/kb/semantic-assertions?page_size=${PAGE}&latest_only=true`,
		'failed to list semantic assertions'
	);
	return {
		rows: (data.results ?? []).map((a) => [
			a.id,
			dash(a.subject_ref_id || a.subject_object_id),
			dash(a.predicate_term_id),
			dash(a.object_ref_id || a.object_object_id),
			dash(a.assertion_kind_term_id),
			a.confidence == null ? '—' : a.confidence.toFixed(2)
		])
	};
}

export const LOADERS: Record<LoaderKey, () => Promise<RecordRows>> = {
	artifact_objects: loadArtifactObjects,
	object_nodes: loadObjectNodes,
	keyword_concepts: loadKeywordConcepts,
	ontology_terms: loadOntologyTerms,
	semantic_assertions: loadSemanticAssertions
};
