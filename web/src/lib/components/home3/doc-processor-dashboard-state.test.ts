import test from 'node:test';
import assert from 'node:assert/strict';

import {
	computeStages,
	isActiveRecord,
	ALL_PROCESSOR_IDS,
	MANDATORY_PROCESSOR_IDS,
	MANDATORY_DISPLAY_STAGES,
	visibleStages,
	enforceEntityBeforeRelation,
	entityExtractionSucceeded,
	type StatusEntry
} from './doc-processor-dashboard-state';

function makeRecord(status: StatusEntry[]) {
	return { status };
}

test('only blocking is locked in the processor selection UI', () => {
	assert.deepEqual(MANDATORY_DISPLAY_STAGES.map((stage) => stage.id), ['blocking']);
	for (const processorID of ['static_analyzer', 'chunking', 'extract_doc_metadata']) {
		assert.equal(ALL_PROCESSOR_IDS.includes(processorID), true);
	}
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

	const stages = computeStages(record);
	const sceneBlocks = stages.find((stage) => stage.id === 'generate_scene_blocks');

	assert.ok(sceneBlocks);
	assert.equal(sceneBlocks.status, 'success');

	const allExpected = [...MANDATORY_PROCESSOR_IDS, 'extract_metrics', 'extract_provisions',
		'generate_summaries', 'generate_topics', 'generate_scene_blocks', 'extract_products'];
	assert.equal(isActiveRecord(record, allExpected), false);
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

	const stages = computeStages(record);
	const semanticProjection = stages.find((stage) => stage.id === 'extract_semantic_projections');

	assert.ok(semanticProjection);
	assert.equal(semanticProjection.status, 'in-progress');
	assert.equal(semanticProjection.entry?.progress, '42% (pass 1: 8/19)');
});

test('visible stages hide processors that are not enabled in config', () => {
	const stages = visibleStages(computeStages(makeRecord([])), [
		'extract_metrics',
		'extract_provisions',
		'generate_summaries',
		'generate_topics',
		'generate_scene_blocks',
		'extract_semantic_projections',
		'extract_entity',
		'extract_relation'
	]);

	const stageIds = stages.map((stage) => stage.id);

	assert.equal(stageIds.includes('extract_products'), false);
	assert.equal(stageIds.includes('extract_structured_knowledge'), false);
	assert.equal(stageIds.includes('extract_semantic_projections'), true);
	assert.equal(stageIds.includes('generate_summaries'), true);
	assert.equal(stageIds.includes('extract_doc_metadata'), true);
	assert.equal(stageIds.includes('extract_entity'), true);
	assert.equal(stageIds.includes('extract_relation'), true);
});

test('split entity/relation stages recognize the legacy combined op (back-compat)', () => {
	const record = makeRecord([
		{ operation: 'extract-entity-relation', proc_status: 'running', progress: '67% (8/12)' }
	]);

	const stages = computeStages(record);
	for (const id of ['extract_entity', 'extract_relation']) {
		const stage = stages.find((s) => s.id === id);
		assert.ok(stage, `${id} stage missing`);
		assert.equal(stage.status, 'in-progress');
		assert.equal(stage.entry?.progress, '67% (8/12)');
	}
});

test('extract_entity stage recognizes its own doc_processing entry', () => {
	const record = makeRecord([
		{ operation: 'doc_processing', proc_status: 'running', doc_processor_name: 'extract_entity' }
	]);

	const stages = computeStages(record);
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

	const stages = computeStages(record);
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
