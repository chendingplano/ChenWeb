export type StageStatus = 'pending' | 'in-progress' | 'success' | 'failed';

export type StatusEntry = {
	operation?: string;
	time?: string;
	start_time?: string;
	status?: string;
	proc_status?: string;
	'proc-status'?: string;
	error?: string;
	progress?: string;
};

export type PipelineRecord = {
	status?: StatusEntry[];
};

export type StageInfo = {
	id: string;
	label: string;
	status: StageStatus;
	entry?: StatusEntry;
};

// 'operations' lists all status operation names this stage may emit, including legacy aliases.
export const PIPELINE_STAGES = [
	{ id: 'staged', label: 'Staged', operations: [] as string[] },
	{ id: 'parsing', label: 'PDF Parser', operations: ['parsing', 'parsed'] },
	{ id: 'converting', label: 'Result Convert', operations: ['converting', 'converted', 'line-file-generated'] },
	{ id: 'static_analyzer', label: 'Static Analyzer', operations: ['static_analyzer', 'static_analzyer', 'structure_analyzer'] },
	{ id: 'chunking', label: 'Chunking', operations: ['chunking', 'chunked'] },
	{ id: 'extract_doc_metadata', label: 'Extract Metadata', operations: ['extract_doc_metadata', 'extract_metadata'] },
	{ id: 'extract_metrics', label: 'Extract Metrics', operations: ['extract_metrics'] },
	{ id: 'extract_provisions', label: 'Extract Provisions', operations: ['extract_provisions'] },
	{ id: 'generate_summaries', label: 'Generate Summaries', operations: ['generate_summaries'] },
	{ id: 'generate_topics', label: 'Generate Topics', operations: ['generate_topics'] },
	{ id: 'generate_scene_blocks', label: 'Generate Scene Blocks', operations: ['generate_scene_blocks', 'extract_scene_blocks'] },
	{ id: 'extract_products', label: 'Extract Products', operations: ['extract_products'] }
];

// All doc-processor-managed stages must finish for the active pipeline card to disappear.
export const DOC_PROCESSOR_STAGES = PIPELINE_STAGES.slice(3);

export const ALL_PROCESSOR_IDS = [
	'static_analyzer', 'chunking', 'extract_doc_metadata', 'extract_metrics', 'extract_provisions',
	'generate_summaries', 'generate_topics', 'generate_scene_blocks', 'extract_products'
];

const FINAL_STATUSES = new Set(['success', 'fail', 'failed']);

export function resolveEntryStatus(entry: StatusEntry): StageStatus {
	const ps = (entry.proc_status ?? entry['proc-status'] ?? entry.status ?? '').toLowerCase();
	if (ps === 'success') return 'success';
	if (ps === 'fail' || ps === 'failed' || ps === 'error') return 'failed';
	if (entry.progress !== undefined) return 'in-progress';
	if (!ps) return 'in-progress';
	return 'in-progress';
}

export function computeStages(record: PipelineRecord): StageInfo[] {
	const statusMap = new Map<string, StatusEntry>();
	for (const e of record.status ?? []) {
		if (e.operation) statusMap.set(e.operation, e);
	}

	function stageFor(operations: string[]): { status: StageStatus; entry?: StatusEntry } {
		for (const op of operations) {
			const e = statusMap.get(op);
			if (e) return { status: resolveEntryStatus(e), entry: e };
		}
		return { status: 'pending' };
	}

	return PIPELINE_STAGES.map((stage) => {
		if (stage.id === 'staged') return { id: 'staged', label: stage.label, status: 'success' as StageStatus };
		const { status, entry } = stageFor(stage.operations);
		return { id: stage.id, label: stage.label, status, entry };
	});
}

export function isActiveRecord(record: PipelineRecord): boolean {
	const entries = record.status ?? [];
	if (!entries.length) return true;

	const statusMap = new Map<string, string>();
	for (const entry of entries) {
		const op = entry.operation ?? '';
		const ps = (entry.proc_status ?? entry['proc-status'] ?? entry.status ?? '').toLowerCase();
		if (op) statusMap.set(op, ps);
	}

	for (const ps of statusMap.values()) {
		if (!FINAL_STATUSES.has(ps)) return true;
	}

	for (const stage of DOC_PROCESSOR_STAGES) {
		const hasFinal = stage.operations.some((op) => FINAL_STATUSES.has(statusMap.get(op) ?? ''));
		if (!hasFinal) return true;
	}

	return false;
}
