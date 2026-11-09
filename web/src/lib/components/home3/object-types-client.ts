export type ObjectType = {
	id: number;
	name: string;
	status: 'approved' | 'proposed';
	raw_value: string;
	source_record_id: number;
	source_run_id: number;
	source_processor: string;
	source_model: string;
	source_prompt: string;
	source_spans: string[];
	created_at: string;
	approved_at: string | null;
	approved_by: string;
};

export type ObjectTypeFilter = 'proposed' | 'approved' | 'all';
export type ObjectTypesPageState = {
	filter: ObjectTypeFilter;
	search: string;
	selectedId: number | null;
	scrollTop: number;
	listScrollTop: number;
};

export const OBJECT_TYPES_STATE_KEY = 'chenweb:object-types:v1';

export function restoreObjectTypesState(raw: string | null): ObjectTypesPageState {
	const initial: ObjectTypesPageState = {
		filter: 'proposed', search: '', selectedId: null, scrollTop: 0, listScrollTop: 0
	};
	try {
		const saved = JSON.parse(raw ?? 'null');
		if (!saved || typeof saved !== 'object') return initial;
		return {
			filter: ['proposed', 'approved', 'all'].includes(saved.filter) ? saved.filter : initial.filter,
			search: typeof saved.search === 'string' ? saved.search : '',
			selectedId: Number.isSafeInteger(saved.selectedId) && saved.selectedId > 0 ? saved.selectedId : null,
			scrollTop: Number.isFinite(saved.scrollTop) && saved.scrollTop >= 0 ? saved.scrollTop : 0,
			listScrollTop: Number.isFinite(saved.listScrollTop) && saved.listScrollTop >= 0 ? saved.listScrollTop : 0
		};
	} catch {
		return initial;
	}
}

export function filterObjectTypes(rows: ObjectType[], filter: ObjectTypeFilter, search: string): ObjectType[] {
	const query = search.trim().toLocaleLowerCase();
	return rows.filter((row) =>
		(filter === 'all' || row.status === filter) &&
		(!query || [row.name, row.raw_value, row.source_processor, row.source_model,
			String(row.source_record_id || '')].some((value) => value.toLocaleLowerCase().includes(query)))
	);
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, { credentials: 'same-origin', ...init });
	const payload = await response.json().catch(() => null);
	if (!response.ok || !payload || payload.status !== true) {
		throw new Error(payload?.error_msg || `HTTP ${response.status}`);
	}
	return payload as T;
}

export function listObjectTypes(): Promise<{ status: true; rows: ObjectType[] }> {
	return request('/api/v1/kb/object-types');
}

export function approveObjectType(id: number): Promise<{ status: true; row: ObjectType }> {
	return request(`/api/v1/kb/object-types/${id}/approve`, { method: 'POST' });
}
