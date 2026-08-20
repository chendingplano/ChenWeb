import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildStageDefs,
	computeStages,
	isActiveRecord,
	isMappingTriageFailure,
	MANDATORY_PROCESSOR_IDS,
	MANDATORY_DISPLAY_STAGES,
	buildManualLaunchOperations,
	visibleStages,
	enforceEntityBeforeRelation,
	entityExtractionSucceeded,
	type StatusEntry
} from './doc-processor-dashboard-state.ts';

function makeRecord(status: StatusEntry[]) {
	return { status };
}

test('only blocking is locked in the processor selection UI', () => {
	assert.deepEqual(MANDATORY_DISPLAY_STAGES.map((stage) => stage.id), ['blocking']);
	for (const processorID of ['static_analyzer', 'chunking', 'extract_doc_metadata']) {
		assert.equal(MANDATORY_PROCESSOR_IDS.includes(processorID), true);
	}
});

test('manual launch operations preserve explicit selection when scene blocks is unchecked', () => {
	const selectable = [
		...MANDATORY_PROCESSOR_IDS,
		'extract_metrics',
		'extract_provisions',
		'extract_semantic_projections',
		'extract_entity',
		'extract_relation',
		'generate_topics',
		'generate_summaries',
		'generate_scene_blocks',
		'extract_inventory_items'
	];
	const checked = Object.fromEntries(selectable.map((id) => [id, id !== 'generate_scene_blocks']));

	const operations = buildManualLaunchOperations(selectable, checked, true);

	assert.equal(operations.includes('generate_scene_blocks'), false);
	assert.deepEqual(operations, selectable.filter((id) => id !== 'generate_scene_blocks'));
});

// buildStageDefs is the sole source of stage metadata: a hard-coded entry for the
// small set of pipeline pre-stages/mandatory processors (plus generate_scene_blocks,
// whose success status the backend currently writes under a different operation
// name — see generate-scene-blocks-processor.go), and a generic entry (derived
// label, operations == [id]) for anything else. This is what fixes the "Processors
// to run" checklist not reflecting new entries in config.toml's required_processors:
// there is no longer a static catalog that can silently drop an unrecognized id.
test('buildStageDefs generates a usable stage def for any processor id not hard-coded', () => {
	const defs = buildStageDefs(['extract_metric_definitions']);
	const def = defs.find((d) => d.id === 'extract_metric_definitions');

	assert.ok(def, 'expected a generated stage def for an id outside the known set');
	assert.equal(def?.label, 'Extract Metric Definitions');
	assert.deepEqual(def?.operations, ['extract_metric_definitions']);
});

test('computeStages reports status for a dynamically configured processor id', () => {
	const record = makeRecord([{ operation: 'extract_metric_definitions', proc_status: 'success' }]);
	const stages = computeStages(record, ['extract_metric_definitions']);
	const stage = stages.find((s) => s.id === 'extract_metric_definitions');

	assert.ok(stage);
	assert.equal(stage?.status, 'success');
});

test('scene blocks stage treats extract_scene_blocks success as finished', () => {
	const record = makeRecord([
		{ operation: 'static_analyzer', proc_status: 'success' },
		{ operation: 'chunking', proc_status: 'success' },
		{ operation: 'extract_doc_metadata', proc_status: 'success' },
		{ operation: 'extract_metrics', proc_status: 'success' },
		{ operation: 'extract_provisions', proc_status: 'success' },
		{ operation: 'generate_summaries', proc_status: 'success' },
		{ operation: 'generate_topics', proc_status: 'success' },
		{ operation: 'extract_scene_blocks', proc_status: 'success' },
		{ operation: 'extract_products', proc_status: 'success' }
	]);

	const configuredIds = [...MANDATORY_PROCESSOR_IDS, 'extract_metrics', 'extract_provisions',
		'generate_summaries', 'generate_topics', 'generate_scene_blocks', 'extract_products'];
	const stages = computeStages(record, configuredIds);
	const sceneBlocks = stages.find((stage) => stage.id === 'generate_scene_blocks');

	assert.ok(sceneBlocks);
	assert.equal(sceneBlocks.status, 'success');
	assert.equal(isActiveRecord(record, configuredIds), false);
});

test('record is finished when extract_products is not in expected set and all others are done', () => {
	const record = makeRecord([
		{ operation: 'static_analyzer', proc_status: 'success' },
		{ operation: 'chunking', proc_status: 'success' },
		{ operation: 'extract_doc_metadata', proc_status: 'success' },
		{ operation: 'extract_metrics', proc_status: 'success' },
		{ operation: 'extract_provisions', proc_status: 'success' },
		{ operation: 'generate_summaries', proc_status: 'success' },
		{ operation: 'generate_topics', proc_status: 'success' },
		{ operation: 'extract_scene_blocks', proc_status: 'success' }
		// extract_products intentionally absent — not configured
	]);

	const expected = [...MANDATORY_PROCESSOR_IDS, 'extract_metrics', 'extract_provisions',
		'generate_summaries', 'generate_topics', 'generate_scene_blocks'];
	assert.equal(isActiveRecord(record, expected), false, 'should be finished without extract_products');
});

test('record is still active when a processor is missing', () => {
	const record = makeRecord([
		{ operation: 'static_analyzer', proc_status: 'success' },
		// chunking missing
		{ operation: 'extract_doc_metadata', proc_status: 'success' }
	]);

	assert.equal(isActiveRecord(record, ['static_analyzer', 'chunking', 'extract_doc_metadata']), true, 'missing chunking keeps record active');
});

test('semantic projections stage recognizes legacy hyphenated in-progress status and progress', () => {
	const record = makeRecord([
		{ operation: 'extract-semantic-projections', proc_status: 'running', progress: '42% (pass 1: 8/19)' }
	]);

	const stages = computeStages(record, ['extract_semantic_projections']);
	const semanticProjection = stages.find((stage) => stage.id === 'extract_semantic_projections');

	assert.ok(semanticProjection);
	assert.equal(semanticProjection.status, 'in-progress');
	assert.equal(semanticProjection.entry?.progress, '42% (pass 1: 8/19)');
});

test('chunking stage recognizes topic chunking and fix-size chunking as in-progress', () => {
	for (const docProcessorName of ['topic_chunking', 'fix_size_chunking']) {
		const record = makeRecord([
			{ operation: 'doc_processing', proc_status: 'running', doc_processor_name: docProcessorName }
		]);

		const stages = computeStages(record);
		const chunking = stages.find((stage) => stage.id === 'chunking');

		assert.ok(chunking, 'chunking stage missing');
		assert.equal(chunking.status, 'in-progress');
		assert.equal(chunking.entry?.doc_processor_name, docProcessorName);
	}
});

test('visible stages hide processors that are not enabled in config', () => {
	// extract_products / extract_structured_knowledge are computed (e.g. because an
	// older record's status still references them) but not in the current required
	// list, so visibleStages must still hide them.
	const configuredIds = [
		'extract_metrics', 'extract_provisions', 'generate_summaries', 'generate_topics',
		'extract_semantic_projections', 'extract_entity', 'extract_relation',
		'extract_products', 'extract_structured_knowledge'
	];
	const requiredIds = [
		'extract_metrics', 'extract_provisions', 'generate_summaries', 'generate_topics',
		'extract_semantic_projections', 'extract_entity', 'extract_relation'
	];
	const stages = visibleStages(computeStages(makeRecord([]), configuredIds), requiredIds);

	const stageIds = stages.map((stage) => stage.id);

	assert.equal(stageIds.includes('extract_products'), false);
	assert.equal(stageIds.includes('extract_structured_knowledge'), false);
	assert.equal(stageIds.includes('extract_semantic_projections'), true);
	assert.equal(stageIds.includes('generate_summaries'), true);
	assert.equal(stageIds.includes('extract_doc_metadata'), true);
	assert.equal(stageIds.includes('extract_entity'), true);
	assert.equal(stageIds.includes('extract_relation'), true);
});

// extract_entity / extract_relation are fully config-driven (no hard-coded alias
// table) since EntityProcessor/RelationProcessor always write status under their
// own canonical op name today (entity-relation-split.go). The legacy combined
// "extract_entity_relation" op was only ever written by the pre-split processor,
// so records created before that split display as pending for these two stages
// rather than in-progress/success — an accepted, intentional gap.
test('legacy combined entity/relation op is no longer recognized (pre-split records only)', () => {
	const record = makeRecord([
		{ operation: 'extract-entity-relation', proc_status: 'running', progress: '67% (8/12)' }
	]);

	const stages = computeStages(record, ['extract_entity', 'extract_relation']);
	for (const id of ['extract_entity', 'extract_relation']) {
		const stage = stages.find((s) => s.id === id);
		assert.ok(stage, `${id} stage missing`);
		assert.equal(stage.status, 'pending');
	}
});

test('extract_entity stage recognizes its own doc_processing entry', () => {
	const record = makeRecord([
		{ operation: 'doc_processing', proc_status: 'running', doc_processor_name: 'extract_entity' }
	]);

	const stages = computeStages(record, ['extract_entity']);
	const entity = stages.find((stage) => stage.id === 'extract_entity');

	assert.ok(entity);
	assert.equal(entity.status, 'in-progress');
	assert.equal(entity.entry?.doc_processor_name, 'extract_entity');
});

test('live doc_processing entry overrides stale stage success for the running processor', () => {
	const record = makeRecord([
		{ operation: 'extract_inventory_items', proc_status: 'success' },
		{ operation: 'doc_processing', proc_status: 'running', doc_processor_name: 'extract_inventory_items' }
	]);

	const stages = computeStages(record, ['extract_inventory_items']);
	const inventoryItems = stages.find((stage) => stage.id === 'extract_inventory_items');

	assert.ok(inventoryItems);
	assert.equal(inventoryItems.status, 'in-progress');
	assert.equal(inventoryItems.entry?.doc_processor_name, 'extract_inventory_items');
});

test('enforceEntityBeforeRelation forces extract_entity when relation chosen and entity not done', () => {
	const out = enforceEntityBeforeRelation(['extract_relation'], false);
	assert.deepEqual(out, ['extract_relation', 'extract_entity']);
});

test('enforceEntityBeforeRelation is a no-op when extract_entity already succeeded', () => {
	const out = enforceEntityBeforeRelation(['extract_relation'], true);
	assert.deepEqual(out, ['extract_relation']);
});

test('enforceEntityBeforeRelation is a no-op when extract_entity already chosen', () => {
	const out = enforceEntityBeforeRelation(['extract_relation', 'extract_entity'], false);
	assert.deepEqual(out, ['extract_relation', 'extract_entity']);
});

test('enforceEntityBeforeRelation is a no-op when relation not chosen', () => {
	const out = enforceEntityBeforeRelation(['extract_metrics'], false);
	assert.deepEqual(out, ['extract_metrics']);
});

test('entityExtractionSucceeded reads the record status', () => {
	assert.equal(
		entityExtractionSucceeded(makeRecord([{ operation: 'extract_entity', proc_status: 'success' }])),
		true
	);
	assert.equal(
		entityExtractionSucceeded(makeRecord([{ operation: 'extract_entity', proc_status: 'running' }])),
		false
	);
	assert.equal(entityExtractionSucceeded(makeRecord([])), false);
});

test('isMappingTriageFailure recognizes the known extract_metrics mapping-backlog error', () => {
	assert.equal(
		isMappingTriageFailure({
			operation: 'extract_metrics',
			proc_status: 'failed',
			error: 'extract_metrics: 3 metric(s) blocked on ungoverned value_range_type vocabulary awaiting review'
		}),
		true
	);
});

test('isMappingTriageFailure is false for a different extract_metrics failure', () => {
	assert.equal(
		isMappingTriageFailure({
			operation: 'extract_metrics',
			proc_status: 'failed',
			error: 'LLM service unavailable'
		}),
		false
	);
});

test('isMappingTriageFailure is false for a different processor with the same-shaped error', () => {
	assert.equal(
		isMappingTriageFailure({
			operation: 'associate_semantics',
			proc_status: 'failed',
			error: 'blocked on ungoverned value_range_type vocabulary awaiting review'
		}),
		false
	);
});

test('isMappingTriageFailure is false when there is no error text', () => {
	assert.equal(isMappingTriageFailure({ operation: 'extract_metrics', proc_status: 'failed' }), false);
});
