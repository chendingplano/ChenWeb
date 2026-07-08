// Service layer for the Object Manager page (ADR 2026070703).
// Wraps the /api/v1/kb/objects/* backend endpoints.

const BASE = '/api/v1/kb';

async function getJson<T>(url: string, fallback: string): Promise<T> {
	const response = await fetch(url, { method: 'GET', credentials: 'same-origin' });
	if (!response.ok) {
		const payload = await response.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `${fallback} (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<T>;
}

async function postJson<T>(url: string, body: unknown, fallback: string): Promise<T> {
	const response = await fetch(url, {
		method: 'POST',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body)
	});
	if (!response.ok) {
		const payload = await response.json().catch(() => null);
		const msg =
			payload && typeof payload.error_msg === 'string'
				? payload.error_msg
				: `${fallback} (${response.status})`;
		throw new Error(msg);
	}
	return response.json() as Promise<T>;
}

// ---- Search (Left Panel) ----

export type ObjectSearchTable = 'object_nodes' | 'artifact_objects';

export interface ObjectNodeSummary {
	id: number;
	object_id: string;
	canonical_name: string;
	canonical_name_en: string;
	object_type: string;
	reconcile_status: string;
}

export interface ArtifactObjectSummary {
	id: number;
	artifact_type: string;
	artifact_id: string;
	object_name: string;
	object_name_en: string;
	object_id: string;
	reconcile_status: string;
}

export interface ObjectSearchRequest {
	table?: ObjectSearchTable;
	query?: string;
	record_id?: number;
	page?: number;
	page_size?: number;
}

export interface ObjectSearchResponse {
	status: boolean;
	table: ObjectSearchTable;
	rows: ObjectNodeSummary[] | ArtifactObjectSummary[];
}

export function searchObjects(req: ObjectSearchRequest): Promise<ObjectSearchResponse> {
	return postJson<ObjectSearchResponse>(`${BASE}/objects/search`, req, 'Failed to search objects');
}

// ---- Relation graph (Object Relation Chart) ----

export type ObjectGraphNodeType = 'object_node' | 'artifact_object' | 'artifact';

export interface ObjectGraphNode {
	key: string;
	type: ObjectGraphNodeType;
	label: string;
	object_id?: string;
	artifact_type?: string;
	artifact_id?: string;
	reconcile_status?: string;
}

export interface ObjectGraphEdge {
	from: string;
	to: string;
	type: 'mentions' | 'about' | 'similar' | 'same_object';
}

export interface ObjectGraph {
	seed_object_id: string;
	truncated: boolean;
	nodes: ObjectGraphNode[];
	edges: ObjectGraphEdge[];
}

export interface ObjectGraphRequest {
	object_id?: string;
	artifact_object_id?: number;
	top_n?: number;
	level?: number;
	max_nodes?: number;
}

export function buildObjectGraph(
	req: ObjectGraphRequest
): Promise<{ status: boolean; graph: ObjectGraph }> {
	return postJson(`${BASE}/objects/graph`, req, 'Failed to build object graph');
}

// ---- Statistics ----

export interface ArtifactObjectStats {
	total: number;
	provisions: number;
	metrics: number;
	inventory_items: number;
	other: number;
	unresolved: number;
}

export function getArtifactObjectStats(): Promise<{ status: boolean; stats: ArtifactObjectStats }> {
	return getJson(`${BASE}/objects/stats/artifact-objects`, 'Failed to load artifact object stats');
}

export interface ObjectNodeStats {
	total: number;
	provisions: number;
	metrics: number;
	inventory_items: number;
}

export function getObjectNodeStats(): Promise<{ status: boolean; stats: ObjectNodeStats }> {
	return getJson(`${BASE}/objects/stats/object-nodes`, 'Failed to load object node stats');
}

export interface ConnectivityRow {
	object_id: string;
	canonical_name: string;
	connections: number;
}

export function getObjectConnectivity(
	topN: number
): Promise<{ status: boolean; top_n: number; rows: ConnectivityRow[] }> {
	return getJson(
		`${BASE}/objects/connectivity?top_n=${encodeURIComponent(String(topN))}`,
		'Failed to load object connectivity'
	);
}

// ---- PDF locator (Right Panel) ----

export interface PdfLocator {
	artifact_object_id: number;
	input_record_id: number;
	document: string;
	source_line_spans: string[];
}

export function getObjectPdfLocator(params: {
	artifactObjectId?: number;
	objectId?: string;
}): Promise<{ status: boolean; locator: PdfLocator }> {
	const q = new URLSearchParams();
	if (params.artifactObjectId != null) q.set('artifact_object_id', String(params.artifactObjectId));
	if (params.objectId) q.set('object_id', params.objectId);
	return getJson(`${BASE}/objects/pdf-locator?${q.toString()}`, 'Failed to resolve PDF locator');
}

// ---- Merge ----

export function mergeObjectNodes(
	loserObjectId: string,
	survivorObjectId: string
): Promise<{ status: boolean; survivor_object_id: string; repointed_mentions: number }> {
	return postJson(
		`${BASE}/objects/merge`,
		{ loser_object_id: loserObjectId, survivor_object_id: survivorObjectId },
		'Failed to merge object nodes'
	);
}
