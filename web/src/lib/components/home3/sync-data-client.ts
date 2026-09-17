export type SyncItemKind = 'table' | 'table_with_files';
export type SyncItemOrigin = 'compiled' | 'local' | 'learned';

export type SyncItem = {
	item_id: string;
	table: string;
	kind: SyncItemKind;
	origin: SyncItemOrigin;
	cursor_col: string;
	natural_key: string[];
	columns: string[];
	json_columns: string[];
	filter?: string;
	file_column?: string;
	file_dir_env?: string;
	file_dir_default_subdir?: string;
	last_synced_at?: string;
	last_row_count: number;
	last_error?: string;
};

export type ListSyncItemsResponse = {
	ok: boolean;
	items: SyncItem[];
};

export type PreviewSyncResponse = {
	ok: boolean;
	item_id: string;
	changed_row_count: number;
};

export type ApplySyncResponse = {
	ok: boolean;
	item_id: string;
	synced_row_count: number;
};

// SyncItemDraft is what the New Data Syncher form submits. Deliberately has
// no columns/json_columns -- the server always derives those from live
// introspection of `table` (see schema_introspect.go / resolveItemShape),
// so the syncher always copies every column, and users pick cursor_col /
// natural_key / file_column from that table's real columns/constraints
// rather than typing names.
export type SyncItemDraft = {
	id: string;
	kind: SyncItemKind;
	table: string;
	cursor_col: string;
	natural_key: string[];
	filter: string;
	file_column: string;
	file_dir_env: string;
	file_dir_default_subdir: string;
};

export type SyncItemResponse = {
	ok: boolean;
	item: SyncItem;
};

export type TableRef = {
	schema: string;
	table: string;
};

export type ListSyncableTablesResponse = {
	ok: boolean;
	tables: TableRef[];
};

export type ColumnInfo = {
	name: string;
	data_type: string;
};

export type NaturalKeyCandidate = {
	columns: string[];
};

export type TableSchemaInfo = {
	columns: ColumnInfo[];
	primary_key: string[];
	natural_key_candidates: NaturalKeyCandidate[];
	json_columns: string[];
};

export type GetTableSchemaResponse = {
	ok: boolean;
	info: TableSchemaInfo;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(path, {
		credentials: 'same-origin',
		...init
	});
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
			parsed && typeof parsed === 'object' && parsed !== null && 'message' in parsed
				? String((parsed as { message: unknown }).message)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export function listSyncItems(): Promise<ListSyncItemsResponse> {
	return req<ListSyncItemsResponse>('/api/v1/data-sync/items');
}

export function previewSync(itemId: string): Promise<PreviewSyncResponse> {
	return req<PreviewSyncResponse>(`/api/v1/data-sync/items/${encodeURIComponent(itemId)}/preview`, {
		method: 'POST'
	});
}

export function applySync(itemId: string): Promise<ApplySyncResponse> {
	return req<ApplySyncResponse>(`/api/v1/data-sync/items/${encodeURIComponent(itemId)}/apply`, {
		method: 'POST'
	});
}

export function createSyncItem(draft: SyncItemDraft): Promise<SyncItemResponse> {
	return req<SyncItemResponse>('/api/v1/data-sync/items', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(draft)
	});
}

export function updateSyncItem(itemId: string, draft: SyncItemDraft): Promise<SyncItemResponse> {
	return req<SyncItemResponse>(`/api/v1/data-sync/items/${encodeURIComponent(itemId)}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(draft)
	});
}

export function deleteSyncItem(itemId: string): Promise<{ ok: boolean }> {
	return req<{ ok: boolean }>(`/api/v1/data-sync/items/${encodeURIComponent(itemId)}`, {
		method: 'DELETE'
	});
}

export function listSyncableTables(): Promise<ListSyncableTablesResponse> {
	return req<ListSyncableTablesResponse>('/api/v1/data-sync/schema/tables');
}

export function getTableSchema(schema: string, table: string): Promise<GetTableSchemaResponse> {
	return req<GetTableSchemaResponse>(
		`/api/v1/data-sync/schema/tables/${encodeURIComponent(schema)}/${encodeURIComponent(table)}`
	);
}
