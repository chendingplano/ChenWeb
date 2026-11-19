// Client for the Releases page (System Admin > System > Releases) and release notes.
const BASE = '/api/v1/releases';

export type ItemType = 'new feature' | 'improvement' | 'bug fix';
export type Component = 'ChenWeb' | 'shared';

export type ReleaseItem = {
	id: number;
	item_type: ItemType;
	component: Component;
	description: string;
	issue: string;
	pull_request: string;
	notes: string;
	created_by: string;
	created_at: string;
};

export type ItemInput = Pick<ReleaseItem, 'item_type' | 'component' | 'description' | 'issue' | 'pull_request' | 'notes'>;

export type Release = {
	id: number;
	project: string;
	version: string;
	kind: 'major' | 'minor' | 'build';
	status: 'building' | 'released';
	description: string;
	build_number: string;
	release_date: string;
	git_rev: string;
	git_dirty: boolean;
	shared_git_rev: string;
	shared_git_dirty: boolean;
	created_by: string;
	created_at: string;
	items: ReleaseItem[];
};

export type ReleasesState = {
	project: string;
	role: string;
	editable: boolean;
	current: string;
	current_build_number: string;
	next_build: string;
	revisions_left: number;
	building: Release | null;
	pending: ReleaseItem[];
	releases: Release[];
};

/** An API error; `code` is set for errors the page explains (read_only). */
export class ReleasesError extends Error {
	constructor(
		message: string,
		public code: string,
		public status: number
	) {
		super(message);
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(BASE + path, { credentials: 'same-origin', ...init });
	let body: Record<string, unknown> = {};
	try {
		body = await response.json();
	} catch {
		// not JSON: use the HTTP status below
	}
	if (!response.ok || body.status === false) {
		throw new ReleasesError(String(body.error_msg ?? `HTTP ${response.status}`), String(body.code ?? ''), response.status);
	}
	return body as T;
}

const send = (method: string, body: unknown): RequestInit => ({
	method,
	headers: { 'Content-Type': 'application/json' },
	body: JSON.stringify(body)
});

export const getReleases = (project = 'ChenWeb') => request<ReleasesState>(`?project=${encodeURIComponent(project)}`);

export const createLine = (kind: 'major' | 'minor', description: string, project = 'ChenWeb') =>
	request<{ release: Release }>(kind === 'major' ? '/majors' : '/minors', send('POST', { project, description }));

export const addItem = (item: ItemInput, project = 'ChenWeb') =>
	request<{ item: ReleaseItem }>('/items', send('POST', { project, ...item }));

export const updateItem = (id: number, item: ItemInput) => request<{ item: ReleaseItem }>(`/items/${id}`, send('PUT', item));

export const deleteItem = (id: number) => request<{ status: boolean }>(`/items/${id}`, { method: 'DELETE' });

function notesQuery(from: string, to: string, lang: string, format: 'text' | 'pdf', project: string) {
	const q = new URLSearchParams({ project, to, lang, format });
	if (from) q.set('from', from);
	return `${BASE}/notes?${q}`;
}

/** Plain-text notes for the releases after `from` (empty = from the first) up to `to`. */
export async function getNotesText(from: string, to: string, lang: string, project = 'ChenWeb'): Promise<string> {
	const response = await fetch(notesQuery(from, to, lang, 'text', project), { credentials: 'same-origin' });
	if (!response.ok) {
		let message = `HTTP ${response.status}`;
		try {
			message = String((await response.json()).error_msg ?? message);
		} catch {
			// keep the status
		}
		throw new ReleasesError(message, '', response.status);
	}
	return response.text();
}

/** URL of the same notes as PDF, for opening in a new tab. */
export const notesPdfUrl = (from: string, to: string, lang: string, project = 'ChenWeb') =>
	notesQuery(from, to, lang, 'pdf', project);

/** The newest build release older than `version` in a newest-first list, or ''. */
export function previousBuild(releases: Release[], version: string): string {
	for (const r of releases) {
		if (r.kind === 'build' && r.status === 'released' && r.version < version) return r.version;
	}
	return '';
}
