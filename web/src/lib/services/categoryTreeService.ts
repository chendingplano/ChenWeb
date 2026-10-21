// Read wrapper for the category trees (devdoc 2026102003, openspec ontology-categories-page).
//
// The Categories page under home3/knowledge → Ontology reads three endpoints:
//   GET /api/v1/kb/category-trees                        → every tree
//   GET /api/v1/kb/category-trees/:name                  → one tree, flat and depth-ordered
//   GET /api/v1/kb/category-trees/:name/nodes/:id/members → the bindings of one node

export type LangText = Record<string, string>;

export type CategoryTree = {
	id: string;
	canonical_name: string;
	namespace: string;
	pref_labels: LangText;
	definitions: LangText;
	source: string;
	current_seq: number;
	created_by: string;
	created_at: string;
	updated_at: string;
};

export type CategoryTreeNode = {
	id: string;
	tree_id: string;
	parent_id?: string;
	canonical_name: string;
	is_others: boolean;
	pref_labels: LangText;
	definitions: LangText;
	source?: string;
	depth: number;
	iri: string;
	is_leaf: boolean;
};

export type CategoryTreeResponse = {
	tree: CategoryTree;
	seq: number;
	revision: string;
	nodes: CategoryTreeNode[];
	valid: boolean;
	problems?: string[];
};

export type CategoryMember = {
	node_id: string;
	canonical_name: string;
	is_others: boolean;
	path: string[];
	artifact_type: string;
	artifact_id: string;
	record_id: number;
	tree_seq: number;
	stale: boolean;
	created_at: string;
};

export type CategoryMembersResponse = {
	members: CategoryMember[];
	direct: boolean;
};

// The members endpoint returns at most this many rows when no limit is given
// (store_bindings.go Members). A full page means the list may be truncated.
export const CATEGORY_MEMBERS_LIMIT = 500;

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

export async function listCategoryTrees(): Promise<CategoryTree[]> {
	const body = await getJson<{ trees: CategoryTree[] }>(
		'/api/v1/kb/category-trees',
		'failed to list category trees'
	);
	return body.trees ?? [];
}

export async function getCategoryTree(name: string): Promise<CategoryTreeResponse> {
	return getJson<CategoryTreeResponse>(
		`/api/v1/kb/category-trees/${encodeURIComponent(name)}`,
		'failed to load category tree'
	);
}

// Only direct bindings: a leaf holds its own memberships, so direct=true is the full list.
export async function listCategoryMembers(
	treeName: string,
	nodeId: string
): Promise<CategoryMembersResponse> {
	return getJson<CategoryMembersResponse>(
		`/api/v1/kb/category-trees/${encodeURIComponent(treeName)}/nodes/${encodeURIComponent(nodeId)}/members?direct=true`,
		'failed to load category members'
	);
}
