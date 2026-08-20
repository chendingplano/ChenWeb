export type StageStatus = 'pending' | 'in-progress' | 'success' | 'failed';

export type StatusEntry = {
	operation?: string;
	doc_processor_name?: string;
	'doc-processor-name'?: string;
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

export type StageDef = {
	id: string;
	label: string;
	operations: string[];
};

// Pipeline pre-stages (not real processors) plus the processors whose status
// is currently written under an operation name that differs from their id.
// This is the only hard-coded metadata left — it mirrors the backend's own
// alias mapping (control.go: processorStatusAliases / primaryProcessorStatusOperation)
// and exists purely to interpret status entries correctly. It never gates
// which processors are selectable — see buildStageDefs, which adds a plain
// entry for any other configured id automatically.
const KNOWN_STAGE_DEFS: StageDef[] = [
	{ id: 'staged', label: 'Staged', operations: [] },
	{ id: 'parsing', label: 'PDF Parser', operations: ['parsing', 'parsed'] },
	{ id: 'converting', label: 'Result Convert', operations: ['converting', 'converted', 'line-file-generated'] },
	// Mandatory — always run, never configurable (see MANDATORY_PROCESSOR_IDS).
	{ id: 'static_analyzer', label: 'Doc Structure Analyzer', operations: ['static_analyzer', 'static_analzyer', 'structure_analyzer'] },
	{
		id: 'chunking',
		label: 'Chunking',
		operations: ['chunking', 'chunked', 'topic_chunking', 'fix_size_chunking']
	},
	{ id: 'extract_doc_metadata', label: 'Extract Doc Metadata', operations: ['extract_doc_metadata', 'extract_metadata'] },
	// Configurable, not mandatory — but generate-scene-blocks-processor.go writes its
	// success status under "extract_scene_blocks" rather than its own id, so it needs
	// its alias hard-coded to display correctly.
	{ id: 'generate_scene_blocks', label: 'Generate Scene Blocks', operations: ['generate_scene_blocks', 'extract_scene_blocks'] }
];

// Stage IDs shown regardless of [doc-processing].required_processors —
// the pre-processor pipeline steps plus the always-on mandatory processors.
export const ALWAYS_VISIBLE_PIPELINE_STAGE_IDS = [
	'staged',
	'parsing',
	'converting',
	'static_analyzer',
	'chunking',
	'extract_doc_metadata'
];

function humanizeLabel(id: string): string {
	return id.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());
}

// buildStageDefs: KNOWN_STAGE_DEFS plus one generic entry (label derived from
// the id, operations matching only the id itself) for every configured
// processor id not already covered above. There is no catalog of "known"
// processors that gates the UI — anything listed in config.toml's
// [doc-processing].required_processors becomes a real, displayable,
// selectable stage without any frontend code change.
export function buildStageDefs(configuredProcessorIds: string[]): StageDef[] {
	const knownIds = new Set(KNOWN_STAGE_DEFS.map((s) => s.id));
	const extra = configuredProcessorIds
		.filter((id) => !knownIds.has(id))
		.map((id) => ({ id, label: humanizeLabel(id), operations: [id] }));
	return [...KNOWN_STAGE_DEFS, ...extra];
}

// Phase A processors that always run. The blocking processor always runs
// before these and never appears in operation lists.
export const MANDATORY_PROCESSOR_IDS: string[] = ['chunking', 'extract_doc_metadata', 'static_analyzer'];

// Ordered list for the "Processors to run" locked section.
// Does not include parse_file / convert_parse_result — those are pre-processor optionals.
export const MANDATORY_DISPLAY_STAGES: { id: string; label: string }[] = [
	{ id: 'blocking', label: 'Blocking' }
];

export const ENTITY_PROCESSOR_ID = 'extract_entity';
export const RELATION_PROCESSOR_ID = 'extract_relation';

// entityExtractionSucceeded reports whether extract_entity has already completed
// successfully for a record (so its entities exist for relation linking).
export function entityExtractionSucceeded(record: PipelineRecord): boolean {
	const stage = computeStages(record, [ENTITY_PROCESSOR_ID]).find((s) => s.id === ENTITY_PROCESSOR_ID);
	return stage?.status === 'success';
}

// enforceEntityBeforeRelation applies the extract_entity -> extract_relation
// dependency (ADR 2026061702): relation extraction is free-form, but its endpoints
// are linked (in Phase C) against entities from extract_entity. If extract_entity
// has not already succeeded for the record, selecting extract_relation forces
// extract_entity to be included too. Order-free — both run in parallel; this only
// guarantees the entities exist for the link step. Returns the resolved id list.
export function enforceEntityBeforeRelation(chosen: string[], entityAlreadySucceeded: boolean): string[] {
	if (!chosen.includes(RELATION_PROCESSOR_ID)) return chosen;
	if (entityAlreadySucceeded) return chosen;
	if (chosen.includes(ENTITY_PROCESSOR_ID)) return chosen;
	return [...chosen, ENTITY_PROCESSOR_ID];
}

export function buildManualLaunchOperations(
	selectableProcessorIds: string[],
	processors: Record<string, boolean>,
	entityAlreadySucceeded: boolean
): string[] {
	const chosen = selectableProcessorIds.filter((p) => processors[p]);
	return enforceEntityBeforeRelation(chosen, entityAlreadySucceeded);
}

// ADR 2026081801 task 5.8: extract_metrics still legitimately fails a record
// when a value_range_type string is unreviewed (DR12 keeps this failure --
// it is not a semantic finding, it is a real "vocabulary needs triage"
// signal). But it is routine, self-resolving backlog, not breakage, and
// restarting the pipeline will not fix it -- only approving/correcting the
// mapping does. The Failed Pipelines panel must not show this identically to
// a genuine crash. The substring below is copied from the one place that
// error text is generated (extract-metrics.go's checkValueRangeTypeMappings,
// MID_26081403 area): "extract_metrics: %d metric(s) blocked on ungoverned
// value_range_type vocabulary awaiting review".
const MAPPING_TRIAGE_ERROR_SUBSTRING = 'blocked on ungoverned value_range_type vocabulary awaiting review';

// isMappingTriageFailure reports whether a failed status entry is this known,
// self-explanatory extract_metrics case rather than a genuine failure.
export function isMappingTriageFailure(entry: StatusEntry): boolean {
	if (normalizeOperationName(entry.operation) !== 'extract_metrics') return false;
	return (entry.error ?? '').includes(MAPPING_TRIAGE_ERROR_SUBSTRING);
}

const FINAL_STATUSES = new Set(['success', 'fail', 'failed', 'stopped']);

function normalizeOperationName(value?: string): string {
	return (value ?? '').trim().toLowerCase().replace(/-/g, '_');
}

export function resolveEntryStatus(entry: StatusEntry): StageStatus {
	const ps = (entry.proc_status ?? entry['proc-status'] ?? entry.status ?? '').toLowerCase();
	if (ps === 'success') return 'success';
	if (ps === 'fail' || ps === 'failed' || ps === 'error' || ps === 'stopped') return 'failed';
	if (entry.progress !== undefined) return 'in-progress';
	if (!ps) return 'in-progress';
	return 'in-progress';
}

// computeStages: stage info for KNOWN_STAGE_DEFS plus every id in
// configuredProcessorIds (mandatory + config.toml's required_processors).
export function computeStages(record: PipelineRecord, configuredProcessorIds: string[] = []): StageInfo[] {
	const entries = record.status ?? [];

	function stageFor(stageId: string, operations: string[]): { status: StageStatus; entry?: StatusEntry } {
		const expectedNames = new Set([stageId, ...operations].map(normalizeOperationName));
		let matchedStageEntry: StatusEntry | undefined;
		let docProcessingEntry: StatusEntry | undefined;

		for (const entry of entries) {
			const operationName = normalizeOperationName(entry.operation);
			if (operationName === 'doc_processing') {
				const processorName = normalizeOperationName(entry.doc_processor_name ?? entry['doc-processor-name']);
				if (expectedNames.has(processorName)) {
					docProcessingEntry = entry;
				}
				continue;
			}
			if (expectedNames.has(operationName)) {
				matchedStageEntry = entry; // always update → takes last match, consistent with kb.input_proc_status trigger
			}
		}

		const docProcessingStatus = docProcessingEntry ? resolveEntryStatus(docProcessingEntry) : undefined;

		// A live doc_processing entry for this stage must override any stale direct
		// success/failure row from an earlier run, otherwise restarted/rerun stages
		// never show the blue in-progress state.
		if (docProcessingEntry && docProcessingStatus === 'in-progress') {
			return { status: docProcessingStatus, entry: docProcessingEntry };
		}

		// Direct processor entry takes precedence once the coordinator is no longer
		// actively naming this stage.
		if (matchedStageEntry) {
			return { status: resolveEntryStatus(matchedStageEntry), entry: matchedStageEntry };
		}
		if (docProcessingEntry) {
			return { status: docProcessingStatus ?? resolveEntryStatus(docProcessingEntry), entry: docProcessingEntry };
		}
		return { status: 'pending' };
	}

	return buildStageDefs(configuredProcessorIds).map((stage) => {
		if (stage.id === 'staged') return { id: 'staged', label: stage.label, status: 'success' as StageStatus };
		const { status, entry } = stageFor(stage.id, stage.operations);
		return { id: stage.id, label: stage.label, status, entry };
	});
}

export function visibleStages(stages: StageInfo[], requiredProcessorIds?: string[]): StageInfo[] {
	if (!requiredProcessorIds?.length) return stages;
	const visibleIds = new Set([...ALWAYS_VISIBLE_PIPELINE_STAGE_IDS, ...requiredProcessorIds]);
	return stages.filter((stage) => visibleIds.has(stage.id));
}

// isActiveRecord: expectedProcessorIds (mandatory + config.toml's
// required_processors) is the exact set of stages that must reach a final
// status for the record to be considered finished — there is no wider
// "all processors" universe to fall back to.
export function isActiveRecord(record: PipelineRecord, expectedProcessorIds: string[]): boolean {
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

	const stagesToCheck = buildStageDefs(expectedProcessorIds).filter((s) => expectedProcessorIds.includes(s.id));

	for (const stage of stagesToCheck) {
		const hasFinal = stage.operations.some((op) => FINAL_STATUSES.has(statusMap.get(op) ?? ''));
		if (!hasFinal) return true;
	}

	return false;
}
