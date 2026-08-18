export type RetryQueueJob = {
	id: number; outcome_id: number; finding_id?: number; target_dependency_fingerprint: string;
	source_input_fingerprint?: string; state: string; attempts: number; lease_token?: string;
	lease_expires_at?: string; last_error?: string; create_time: string; modify_time: string;
	outcome_input_record_id?: number; outcome_artifact_type?: string; outcome_artifact_id?: string;
	outcome_stage_term_id?: string;
};
export type RetryQueueFilters = { state: string; outcome_id: string };
async function req<T>(path: string): Promise<T> {
	const res = await fetch(path, { credentials: 'same-origin' });
	const text = await res.text();
	let body: any = null;
	try {
		body = text ? JSON.parse(text) : null;
	} catch {}
	if (!res.ok) throw new Error(body?.error_msg || `HTTP ${res.status}`);
	return body as T;
}
export async function listRetryQueue(filters: RetryQueueFilters, page = 1, pageSize = 50) {
	const q = new URLSearchParams({ page: String(page), page_size: String(pageSize) });
	for (const [k, v] of Object.entries(filters)) if (v.trim()) q.set(k, v.trim());
	return req<{ results: RetryQueueJob[]; total: number; page: number; page_size: number }>(
		`/api/v1/kb/semantic-retry-queue?${q}`
	);
}
