// Read wrapper for the MeSH browser (server/api/meshbrowser). All endpoints are read-only.
//
//   GET /api/v1/mesh/overview            → release and record counts per kind
//   GET /api/v1/mesh/entities?kind=…     → one page of records of a kind
//   GET /api/v1/mesh/tree?parent=…       → tree numbers under a parent (none = top level)
//   GET /api/v1/mesh/entity?id=…         → one record with its properties and links
//   GET /api/v1/mesh/neighborhood?id=…   → a tree number with its children and grandchildren

export type MeshKindCount = { kind: string; count: number };

export type MeshOverview = {
	release: string;
	source_triples: number;
	kinds: MeshKindCount[];
};

export type MeshEntityRow = { id: string; label: string };

export type MeshValue = {
	text?: string;
	lang?: string;
	id?: string;
	label?: string;
};

export type MeshProperty = { predicate: string; values: MeshValue[] };

export type MeshEntity = {
	id: string;
	kind: string;
	label: string;
	properties: MeshProperty[];
	filed?: MeshEntityRow[];
};

// Page size for record lists (the server caps it at 200).
export const MESH_PAGE_SIZE = 50;

async function getJson<T>(url: string, fallback: string): Promise<T> {
	const res = await fetch(url, { method: 'GET', credentials: 'same-origin' });
	if (!res.ok) {
		const payload = await res.json().catch(() => null);
		const msg =
			payload && typeof payload.message === 'string'
				? payload.message
				: `${fallback} (${res.status})`;
		throw new Error(msg);
	}
	return res.json() as Promise<T>;
}

export function getMeshOverview(): Promise<MeshOverview> {
	return getJson<MeshOverview>('/api/v1/mesh/overview', 'failed to load MeSH overview');
}

export async function listMeshEntities(
	kind: string,
	query: string,
	offset: number
): Promise<{ items: MeshEntityRow[]; has_more: boolean }> {
	const params = new URLSearchParams({
		kind,
		q: query,
		limit: String(MESH_PAGE_SIZE),
		offset: String(offset)
	});
	const body = await getJson<{ items: MeshEntityRow[] | null; has_more: boolean }>(
		`/api/v1/mesh/entities?${params}`,
		'failed to list MeSH records'
	);
	return { items: body.items ?? [], has_more: body.has_more };
}

export async function listMeshTree(parent: string): Promise<MeshEntityRow[]> {
	const params = parent ? `?parent=${encodeURIComponent(parent)}` : '';
	const body = await getJson<{ items: MeshEntityRow[] | null }>(
		`/api/v1/mesh/tree${params}`,
		'failed to load MeSH tree'
	);
	return body.items ?? [];
}

// A tree number with its children (level 1) and their children (level 2), for the tree chart.
// Hidden counts the children cut off by the server's per-level cap.
export type MeshNode = {
	id: string;
	label: string;
	child_count?: number;
	hidden?: number;
	children?: MeshNode[];
};

export function getMeshNeighborhood(id: string): Promise<MeshNode> {
	return getJson<MeshNode>(
		`/api/v1/mesh/neighborhood?id=${encodeURIComponent(id)}`,
		'failed to load MeSH tree'
	);
}

export function getMeshEntity(id: string): Promise<MeshEntity> {
	return getJson<MeshEntity>(
		`/api/v1/mesh/entity?id=${encodeURIComponent(id)}`,
		'failed to load MeSH record'
	);
}
