import test from 'node:test';
import assert from 'node:assert/strict';

import {
	computeStages,
	isActiveRecord,
	type StatusEntry
} from './doc-processor-dashboard-state.ts';

function makeRecord(status: StatusEntry[]) {
	return {
		status,
	};
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
	assert.equal(isActiveRecord(record), false);
});
