import test from 'node:test';
import assert from 'node:assert/strict';

import {
	computeStages,
	isActiveRecord,
	MANDATORY_PROCESSOR_IDS,
	visibleStages,
	type StatusEntry
} from './doc-processor-dashboard-state.ts';

function makeRecord(status: StatusEntry[]) {
	return { status };
}

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
		'extract_entity_relation'
	]);

	const stageIds = stages.map((stage) => stage.id);

	assert.equal(stageIds.includes('extract_products'), false);
	assert.equal(stageIds.includes('extract_structured_knowledge'), false);
	assert.equal(stageIds.includes('extract_semantic_projections'), true);
	assert.equal(stageIds.includes('generate_summaries'), true);
	assert.equal(stageIds.includes('extract_doc_metadata'), true);
});

test('entity relation stage recognizes legacy hyphenated in-progress status and progress', () => {
	const record = makeRecord([
		{ operation: 'extract-entity-relation', proc_status: 'running', progress: '67% (8/12)' }
	]);

	const stages = computeStages(record);
	const entityRelation = stages.find((stage) => stage.id === 'extract_entity_relation');

	assert.ok(entityRelation);
	assert.equal(entityRelation.status, 'in-progress');
	assert.equal(entityRelation.entry?.progress, '67% (8/12)');
});
