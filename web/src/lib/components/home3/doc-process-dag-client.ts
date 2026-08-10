/**
 * API client for the Doc Process DAG composite API (backend:
 * server/api/kbhandler/doc_process_dag_handler.go, routes under
 * /api/v1/kb/doc-process-dags). A DAG is a named doc process pipeline (its
 * current version), its processor gates + depends_on_processors edges
 * (kb.pipeline_rules), and its read-only knowledge-store bindings.
 */

export type DagRule = {
	id: number;
	name: string;
	priority: number;
	target_processor?: string;
	effect?: string;
	predicate?: Record<string, unknown>;
	predicate_checksum?: string;
	required_facets?: string[];
	depends_on_processors?: string[];
	active: boolean;
	create_time: string;
	modify_time: string;
};

export type DagBinding = {
	id: number;
	name?: string;
	priority: number;
	ks_store_id?: number;
	pipeline_id: number;
	binding_kind: string;
	predicate?: Record<string, unknown>;
	active: boolean;
	tenant_id?: string;
	user_id?: string;
	input_record_id?: number;
	create_time: string;
	modify_time: string;
};

export type DocProcessDag = {
	id: number;
	name: string;
	display_name?: string;
	description?: string;
	processors: string[];
	legacy_equivalent: boolean;
	is_system_default: boolean;
	version: number;
	status: string;
	rule_count: number;
	create_time: string;
	modify_time: string;
};

export type DocProcessDagDetail = DocProcessDag & {
	rules: DagRule[];
	bindings: DagBinding[];
};

export type ProcessorSpec = {
	name: string;
	phase: string;
	class?: string;
	cost?: string;
	on_undetermined?: string;
	idempotent: boolean;
	requires?: string[];
	produces?: string[];
};

export type RuleInput = {
	name: string;
	priority?: number;
	target_processor: string;
	effect: string;
	predicate?: Record<string, unknown>;
	depends_on_processors?: string[];
};

export type CreateDagInput = {
	name: string;
	display_name?: string;
	description?: string;
	processors: string[];
	legacy_equivalent?: boolean;
	is_system_default?: boolean;
	rules: RuleInput[];
};

export type UpdateDagInput = {
	/** null clears the value; omitting keeps the current value. */
	display_name?: string | null;
	description?: string | null;
	processors?: string[];
	legacy_equivalent?: boolean;
	is_system_default?: boolean;
	rules?: RuleInput[];
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

export async function listDags(search = ''): Promise<DocProcessDag[]> {
	const q = search ? `?search=${encodeURIComponent(search)}` : '';
	const res = await req<{ results: DocProcessDag[] }>(`/api/v1/kb/doc-process-dags${q}`);
	return res.results;
}

export async function getDag(name: string): Promise<DocProcessDagDetail> {
	const res = await req<{ record: DocProcessDagDetail }>(
		`/api/v1/kb/doc-process-dags/${encodeURIComponent(name)}`
	);
	return res.record;
}

export async function createDag(input: CreateDagInput): Promise<DocProcessDagDetail> {
	const res = await req<{ record: DocProcessDagDetail }>('/api/v1/kb/doc-process-dags', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input)
	});
	return res.record;
}

export async function updateDag(name: string, input: UpdateDagInput): Promise<DocProcessDagDetail> {
	const res = await req<{ record: DocProcessDagDetail }>(
		`/api/v1/kb/doc-process-dags/${encodeURIComponent(name)}`,
		{
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(input)
		}
	);
	return res.record;
}

export async function deleteDag(name: string): Promise<number> {
	const res = await req<{ deleted: number }>(
		`/api/v1/kb/doc-process-dags/${encodeURIComponent(name)}`,
		{
			method: 'DELETE'
		}
	);
	return res.deleted;
}

export async function listProcessors(): Promise<ProcessorSpec[]> {
	const res = await req<{ results: ProcessorSpec[] }>('/api/v1/kb/doc-process-processors');
	return res.results;
}

/** Rule-less processor set — used by the create form's default gate rows. */
export function defaultRuleFor(processor: string): RuleInput {
	return {
		name: `gate-${processor}`,
		target_processor: processor,
		effect: 'require',
		depends_on_processors: []
	};
}
