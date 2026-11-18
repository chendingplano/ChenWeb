export type ProductRelationType = {
	code: string;
	value: string;
	status: 'approved' | 'proposed';
	description_en: string;
	description_zh: string;
	weight: number;
	first_record_id: number | null;
	first_product_ref: string;
	seen_count: number;
	product_count: number;
	extra_info: Record<string, unknown>;
	created_at: string;
	approved_at: string | null;
	approved_by: string;
};

export type ProductRelationTypeInput = {
	description_en: string;
	description_zh: string;
	weight: number;
};

export type ProductRelationTypeFilter = 'proposed' | 'approved' | 'all';

export const DEFAULT_RELATION_WEIGHT = 0.5;

export function filterProductRelationTypes(
	rows: ProductRelationType[],
	filter: ProductRelationTypeFilter,
	search: string
): ProductRelationType[] {
	const query = search.trim().toLocaleLowerCase();
	return rows.filter(
		(row) =>
			(filter === 'all' || row.status === filter) &&
			(!query ||
				[row.code, row.value, row.description_en, row.description_zh].some((v) =>
					v.toLocaleLowerCase().includes(query)
				))
	);
}

/** Mirrors the server: both descriptions required, weight within 0..1. */
export function validateRelationTypeInput(
	input: ProductRelationTypeInput
): 'descriptions' | 'weight' | null {
	if (!input.description_en.trim() || !input.description_zh.trim()) return 'descriptions';
	if (!Number.isFinite(input.weight) || input.weight < 0 || input.weight > 1) return 'weight';
	return null;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, { credentials: 'same-origin', ...init });
	const payload = await response.json().catch(() => null);
	if (!response.ok || !payload || payload.status !== true) {
		throw new Error(payload?.error_msg || `HTTP ${response.status}`);
	}
	return payload as T;
}

const base = '/api/v1/kb/product-relation-types';
const json = (method: string, body: ProductRelationTypeInput): RequestInit => ({
	method,
	headers: { 'Content-Type': 'application/json' },
	body: JSON.stringify(body)
});

export function listProductRelationTypes(): Promise<{ status: true; rows: ProductRelationType[] }> {
	return request(base);
}

export function approveProductRelationType(
	code: string,
	input: ProductRelationTypeInput
): Promise<{ status: true; row: ProductRelationType }> {
	return request(`${base}/${encodeURIComponent(code)}/approve`, json('POST', input));
}

export function updateProductRelationType(
	code: string,
	input: ProductRelationTypeInput
): Promise<{ status: true; row: ProductRelationType }> {
	return request(`${base}/${encodeURIComponent(code)}`, json('PUT', input));
}

export function deleteProductRelationType(code: string): Promise<{ status: true }> {
	return request(`${base}/${encodeURIComponent(code)}`, { method: 'DELETE' });
}
