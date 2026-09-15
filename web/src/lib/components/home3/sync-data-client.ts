export type SyncItem = {
	item_id: string;
	table: string;
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
