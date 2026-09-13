// Client for GET /api/v1/kb/product-names: the shared kb.product_names
// catalog used by the Product Name typeahead (product-name-field.svelte).
//
// Every call returns the decoded JSON body and throws Error(error_msg) on a
// non-2xx response or a `{ status: false }` envelope — the same contract the
// rest of the ChenWeb services use.

const BASE = '/api/v1/kb';

export type ProductNameEntry = {
	id: number;
	product_name: string;
	product_name_en: string;
	aliases: string[];
	category_l1: string;
	category_l2: string;
	status: string;
};

async function call<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`${BASE}${path}`, { credentials: 'same-origin', ...init });
	const data = await res.json().catch(() => null);
	if (!res.ok || !data || data.status === false) {
		const msg =
			(data && (data.error_msg as string)) ||
			(data && (data.error_code as string)) ||
			`request to ${path} failed (${res.status})`;
		throw new Error(msg);
	}
	return data as T;
}

export function listProductNames(): Promise<{ status: true; product_names: ProductNameEntry[] }> {
	return call('/product-names');
}
