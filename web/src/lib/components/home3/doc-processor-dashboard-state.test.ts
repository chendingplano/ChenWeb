import test from 'node:test';
import assert from 'node:assert/strict';

import {
	computeStages,
	isActiveRecord,
	MANDATORY_PROCESSOR_IDS,
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

test('record is still active when a mandatory processor is missing', () => {
	const record = makeRecord([
		{ operation: 'static_analyzer', proc_status: 'success' },
		// chunking missing
		{ operation: 'extract_doc_metadata', proc_status: 'success' }
	]);

	assert.equal(isActiveRecord(record, MANDATORY_PROCESSOR_IDS), true, 'missing chunking keeps record active');
});
