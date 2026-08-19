export type OrphanedLabel = {
	id: number;
	term_id: string;
	label: string;
	lang: string;
	label_role: string;
	status: string;
	create_time?: string;
	create_by?: string;
	modify_time?: string;
	modify_by?: string;
};

export type OrphanedLabelsFilters = {
	q?: string;
	lang?: string;
	label_role?: string;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, { credentials: 'same-origin', ...init });
	const text = await response.text();
	let parsed: unknown = null;
	if (text) {
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
	}
	if (!response.ok) {
		const message =
			parsed && typeof parsed === 'object' && parsed !== null && 'error' in parsed
				? String((parsed as { error: unknown }).error)
				: `HTTP ${response.status}`;
		throw new Error(message);
	}
	return parsed as T;
}

export function buildOrphanedLabelsQuery(filters: OrphanedLabelsFilters = {}): string {
	const params = new URLSearchParams();
	if (filters.q?.trim()) params.set('q', filters.q.trim());
	if (filters.lang?.trim()) params.set('lang', filters.lang.trim());
	if (filters.label_role?.trim()) params.set('label_role', filters.label_role.trim());
	return params.toString();
}

export function listOrphanedLabels(
	filters: OrphanedLabelsFilters = {}
): Promise<{ status: boolean; results: OrphanedLabel[]; total: number }> {
	const query = buildOrphanedLabelsQuery(filters);
	return req(`/api/v1/admin/db/ontology-term-labels/orphans${query ? `?${query}` : ''}`);
}

export function resolveOrphanedLabels(
	ids: number[],
	filters: OrphanedLabelsFilters = {}
): Promise<{ status: boolean; deleted_count: number }> {
	return req('/api/v1/admin/db/ontology-term-labels/orphans/resolve', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			ids,
			q: filters.q?.trim() ?? '',
			lang: filters.lang?.trim() ?? '',
			label_role: filters.label_role?.trim() ?? ''
		})
	});
}
