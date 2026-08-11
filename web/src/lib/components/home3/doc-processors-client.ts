/**
 * API client for the doc processor admin catalog (backend:
 * server/api/kbhandler/doc_processors_handler.go, routes under
 * /api/v1/kb/doc-processors). A doc processor is a row in kb.doc_processors —
 * the capsule §7 roster, manageable (create/update/delete/search) by admins.
 */

export const DOC_PROCESSOR_TYPES = ['mandatory', 'configurable'] as const;
export const DOC_PROCESSOR_STATUSES = ['active', 'disabled', 'suspended'] as const;

export type DocProcessorType = (typeof DOC_PROCESSOR_TYPES)[number];
export type DocProcessorStatus = (typeof DOC_PROCESSOR_STATUSES)[number];

export type DocProcessor = {
	name_as_id: string;
	display_name: string;
	description?: string;
	type: DocProcessorType;
	require_llm: boolean;
	status: DocProcessorStatus;
	notes?: string;
	create_time: string;
	modify_time: string;
};

export type CreateProcessorInput = {
	name_as_id: string;
	display_name: string;
	description?: string;
	type: DocProcessorType;
	require_llm?: boolean;
	status?: DocProcessorStatus;
	notes?: string;
};

export type UpdateProcessorInput = {
	/** null clears the value; omitting keeps the current value. name_as_id is immutable. */
	display_name?: string;
	description?: string | null;
	type?: DocProcessorType;
	require_llm?: boolean;
	status?: DocProcessorStatus;
	notes?: string | null;
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
			parsed && typeof parsed === 'object' && parsed !== null && 'error_msg' in parsed
				? String((parsed as { error_msg: unknown }).error_msg)
				: `HTTP ${res.status}`;
		throw new Error(msg);
	}
	return parsed as T;
}

export async function listDocProcessors(search = ''): Promise<DocProcessor[]> {
	const q = search ? `?search=${encodeURIComponent(search)}` : '';
	const res = await req<{ results: DocProcessor[] }>(`/api/v1/kb/doc-processors${q}`);
	return res.results;
}

export async function createProcessor(input: CreateProcessorInput): Promise<DocProcessor> {
	const res = await req<{ record: DocProcessor }>('/api/v1/kb/doc-processors', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
	return res.record;
}

export async function updateProcessor(name: string, input: UpdateProcessorInput): Promise<DocProcessor> {
	const res = await req<{ record: DocProcessor }>(
		`/api/v1/kb/doc-processors/${encodeURIComponent(name)}`,
		{
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(input)
		}
	);
	return res.record;
}

export async function deleteProcessor(name: string): Promise<number> {
	const res = await req<{ deleted: number }>(
		`/api/v1/kb/doc-processors/${encodeURIComponent(name)}`,
		{
			method: 'DELETE'
		}
	);
	return res.deleted;
}
