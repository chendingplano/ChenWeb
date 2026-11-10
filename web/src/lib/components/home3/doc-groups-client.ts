// Client for the document group administration API (server/api/docbuilder).
export type DocGroupMember = {
	member_id: number;
	member_type: 'dir' | 'file';
	path: string;
	exists: boolean;
};
export type DocGroup = {
	group_id: number;
	group_key: string;
	label: Record<string, string>;
	parent_menu_id: string;
	sort_order: number;
	access_role: string[];
	enabled: boolean;
	members: DocGroupMember[];
	documents: number;
	uncategorized: number;
};
export type DocGroupDoc = {
	doc_id: number;
	path: string;
	kind: 'markdown' | 'pdf';
	title: string;
	category: string;
};

const base = '/api/v1/doc-groups/admin/groups';

export class ApiError extends Error {
	constructor(
		message: string,
		public status: number,
		public groups: string[] = []
	) {
		super(message);
	}
}

async function call<T>(url: string, method = 'GET', body?: unknown): Promise<T> {
	const r = await fetch(url, {
		method,
		credentials: 'same-origin',
		headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
	if (r.status === 204) return undefined as T;
	const data = await r.json().catch(() => ({}));
	if (!r.ok) throw new ApiError(data.error ?? String(r.status), r.status, data.groups ?? []);
	return data as T;
}

const k = (key: string) => `${base}/${encodeURIComponent(key)}`;

export const listGroups = () => call<DocGroup[]>(base);
export const saveGroup = (g: Partial<DocGroup> & { group_key: string }, isNew: boolean) =>
	isNew ? call<DocGroup>(base, 'POST', g) : call<DocGroup>(k(g.group_key), 'PUT', g);
export const deleteGroup = (key: string) => call<void>(k(key), 'DELETE');
export const addMember = (key: string, member_type: string, path: string) =>
	call<DocGroupMember>(`${k(key)}/members`, 'POST', { member_type, path });
export const removeMember = (key: string, id: number) =>
	call<void>(`${k(key)}/members/${id}`, 'DELETE');
export const listDocs = (key: string) => call<DocGroupDoc[]>(`${k(key)}/documents`);
export const removeDoc = (key: string, docId: number, del: boolean) =>
	call<void>(`${k(key)}/documents/${docId}/remove`, 'POST', { delete: del });
export const rescan = (key: string) => call<void>(`${k(key)}/rescan`, 'POST');
export const classify = (key: string, force: boolean) =>
	call<{ classified: number }>(`${k(key)}/classify`, 'POST', { force });

// Sync = what `mise build-docs <group>` does: rescan, then classify the documents without a category.
// A server without a classification model answers 503; the rescan is kept and classified is null.
export async function syncGroup(key: string): Promise<{ classified: number | null }> {
	await rescan(key);
	try {
		return await classify(key, false);
	} catch (e) {
		if (e instanceof ApiError && e.status === 503) return { classified: null };
		throw e;
	}
}

// A group's page is its entry at its menu location, which the menu shows exactly while the group is
// enabled; building the page enables the group.
export const hasPage = (g: DocGroup) => g.enabled;
export const buildPage = (g: DocGroup) =>
	saveGroup(
		{
			group_key: g.group_key,
			label: g.label,
			parent_menu_id: g.parent_menu_id,
			sort_order: g.sort_order,
			access_role: g.access_role,
			enabled: true
		},
		false
	);

export class MemberRefusedError extends Error {
	constructor(
		public path: string,
		public cause: unknown
	) {
		super(`member refused: ${path}`);
	}
}

export type NewDocGroup = {
	group_key: string;
	label: Record<string, string>;
	sort_order: number;
	access_role: string[];
	members: { member_type: 'dir' | 'file'; path: string }[];
};

// Creates a group under Documents without a page, then adds its members. If the server refuses a
// member, the new group is deleted again, so a failed create leaves nothing behind.
export async function createGroup(input: NewDocGroup): Promise<void> {
	const { members, ...fields } = input;
	const saved = await saveGroup({ ...fields, parent_menu_id: 'documents', enabled: false }, true);
	const key = saved?.group_key || fields.group_key;
	for (const mem of members) {
		try {
			await addMember(key, mem.member_type, mem.path);
		} catch (e) {
			await deleteGroup(key).catch(() => {});
			throw e instanceof ApiError && e.status === 400 ? new MemberRefusedError(mem.path, e) : e;
		}
	}
}

// Position for a new group: after the last group under the parent.
export function nextPosition(groups: DocGroup[], parent = 'documents'): number {
	const orders = groups.filter((g) => g.parent_menu_id === parent).map((g) => g.sort_order);
	return orders.length ? Math.max(...orders) + 10 : 10;
}

export const groupLabel = (g: Pick<DocGroup, 'label' | 'group_key'>, locale: string) =>
	g.label?.[locale] || g.label?.en || g.group_key;

// "a, B ,a" → ["a","b"]: role tokens are lower-case and unique (the server validates them).
export function parseRoles(text: string): string[] {
	return [
		...new Set(
			text
				.split(',')
				.map((r) => r.trim().toLowerCase())
				.filter(Boolean)
		)
	];
}
