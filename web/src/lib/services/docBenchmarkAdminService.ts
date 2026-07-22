const BASE = '/api/v1/admin/benchmark';

export type BenchmarkConfig = {
	scope: string;
	experiment_path: string;
	dataset_root: string;
	artifact_root: string;
	work_root: string;
	evidence_root: string;
	store_id: number;
	owner: string;
	tenant_id: string;
	metrics_model_name: string;
	allow_dirty: boolean;
	report_format: string;
	report_output_path: string;
	metrics_baseline: string;
	metrics_candidate: string;
	chunk_baseline: string;
	chunk_candidate: string;
	created_at?: string;
	updated_at?: string;
};

export type BenchmarkJob = {
	id: number;
	scope: string;
	step_id: string;
	job_type: string;
	status: string;
	message: string;
	request?: Record<string, unknown>;
	result?: Record<string, unknown>;
	error_text?: string;
	created_by?: string;
	created_at?: string;
	started_at?: string;
	finished_at?: string;
	updated_at?: string;
};

export type BenchmarkStepState = {
	id: string;
	title: string;
	description: string;
	status: string;
	message: string;
	detected?: Record<string, unknown>;
	completed_at?: string;
	failed_at?: string;
	running_job_id?: number;
};

export type BenchmarkSetupState = {
	config: BenchmarkConfig;
	steps: BenchmarkStepState[];
	active_jobs: BenchmarkJob[];
	recent_jobs: BenchmarkJob[];
	last_experiment_id?: string;
};

async function parseOrThrow(res: Response) {
	const body = await res.json().catch(() => ({}));
	if (!res.ok) {
		throw new Error((body as { message?: string }).message ?? `Request failed: ${res.status}`);
	}
	return body;
}

export async function getBenchmarkConfig(fetchFn: typeof fetch = fetch): Promise<BenchmarkConfig> {
	const res = await fetchFn(`${BASE}/config`, { credentials: 'same-origin' });
	return parseOrThrow(res);
}

export async function saveBenchmarkConfig(
	payload: BenchmarkConfig,
	fetchFn: typeof fetch = fetch
): Promise<BenchmarkConfig> {
	const res = await fetchFn(`${BASE}/config`, {
		method: 'PUT',
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(payload)
	});
	return parseOrThrow(res);
}

export async function getBenchmarkSetupState(fetchFn: typeof fetch = fetch): Promise<BenchmarkSetupState> {
	const res = await fetchFn(`${BASE}/setup-state`, { credentials: 'same-origin' });
	return parseOrThrow(res);
}

export async function runBenchmarkStep(
	stepId: string,
	fetchFn: typeof fetch = fetch
): Promise<BenchmarkJob> {
	const res = await fetchFn(`${BASE}/steps/${encodeURIComponent(stepId)}/run`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	return parseOrThrow(res);
}

export async function runNextBenchmarkStep(fetchFn: typeof fetch = fetch): Promise<BenchmarkJob> {
	const res = await fetchFn(`${BASE}/run-next`, {
		method: 'POST',
		credentials: 'same-origin'
	});
	return parseOrThrow(res);
}

export async function listBenchmarkJobs(fetchFn: typeof fetch = fetch): Promise<BenchmarkJob[]> {
	const res = await fetchFn(`${BASE}/jobs`, { credentials: 'same-origin' });
	const data = (await parseOrThrow(res)) as { jobs?: BenchmarkJob[] };
	return data.jobs ?? [];
}

export async function getBenchmarkJob(
	jobId: number,
	fetchFn: typeof fetch = fetch
): Promise<BenchmarkJob> {
	const res = await fetchFn(`${BASE}/jobs/${jobId}`, { credentials: 'same-origin' });
	return parseOrThrow(res);
}
