export type ProductNamesImportResult = {
	status: boolean;
	error?: string;
	row_count?: number;
	inserted?: number;
	skipped?: number;
};

async function upload(path: string, file: File): Promise<ProductNamesImportResult> {
	const body = new FormData();
	body.append('file', file);
	const res = await fetch(path, { method: 'POST', credentials: 'same-origin', body });
	const data = (await res.json()) as ProductNamesImportResult;
	if (!res.ok || !data.status) throw new Error(data.error || `Request failed (${res.status})`);
	return data;
}

export function previewMechanicalProductNames(file: File): Promise<ProductNamesImportResult> {
	return upload('/api/v1/product-names/china-mechanical/preview', file);
}

export function importMechanicalProductNames(file: File): Promise<ProductNamesImportResult> {
	return upload('/api/v1/product-names/china-mechanical/import', file);
}
