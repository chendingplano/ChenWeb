import type { LLMModelEntry } from './llm-models-client';

export type EmbeddingRecord = {
	id: number;
	model_key: string;
	model_name: string;
	content: string;
	created_at: string;
	updated_at: string;
};

async function req<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, {
		credentials: 'same-origin',
		...init
	});
	const body = await response.text();
	let parsed: any = null;
	try {
		parsed = body ? JSON.parse(body) : null;
	} catch {
		/* keep the HTTP status as the useful error */
	}
	if (!response.ok) throw new Error(parsed?.message ?? `HTTP ${response.status}`);
	return parsed as T;
}

const json = (body: unknown): RequestInit => ({
	method: 'POST',
	headers: { 'Content-Type': 'application/json' },
	body: JSON.stringify(body)
});

export function createEmbedding(model_key: string, content: string, save: boolean) {
	return req<{ dimension: number; embedding: number[]; record_id?: number }>(
		'/api/v1/llm/embeddings',
		json({ model_key, content, save })
	);
}

export function searchEmbeddingSimilarity(model_key: string, content: string, top_n: number) {
	return req<{ matches: Array<EmbeddingRecord & { similarity: number }> }>(
		'/api/v1/llm/embeddings/similarity',
		json({ model_key, content, top_n })
	);
}

export function listEmbeddingRecords(
	dimension: number,
	model_key: string,
	page: number,
	limit = 20
) {
	const params = new URLSearchParams({
		dimension: String(dimension),
		page: String(page),
		limit: String(limit),
		model_key
	});
	return req<{ records: EmbeddingRecord[]; total: number; page: number; limit: number }>(
		`/api/v1/llm/embeddings/records?${params}`
	);
}

export function compareEmbeddings(model_key: string, ids: number[]) {
	return req<{ pairs: Array<{ id_a: number; id_b: number; similarity: number }> }>(
		'/api/v1/llm/embeddings/compare',
		json({ model_key, ids })
	);
}

export function updateEmbeddingRecord(id: number, model_key: string, content: string) {
	return req<{ ok: boolean }>(`/api/v1/llm/embeddings/records/${id}`, {
		...json({ model_key, content }),
		method: 'PUT'
	});
}

export function regenerateEmbeddingRecord(id: number, model_key: string) {
	return req<{ ok: boolean }>(
		`/api/v1/llm/embeddings/records/${id}/regenerate?model_key=${encodeURIComponent(model_key)}`,
		{ method: 'POST' }
	);
}

export function deleteEmbeddingRecord(id: number, model_key: string) {
	return req<{ ok: boolean }>(
		`/api/v1/llm/embeddings/records/${id}?model_key=${encodeURIComponent(model_key)}`,
		{ method: 'DELETE' }
	);
}

export function clearEmbeddingRecords(dimension: number) {
	return req<{ ok: boolean }>(`/api/v1/llm/embeddings/records?dimension=${dimension}`, {
		method: 'DELETE'
	});
}

export type { LLMModelEntry };
