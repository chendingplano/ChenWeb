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
