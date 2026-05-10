import test from 'node:test';
import assert from 'node:assert/strict';

import { createTopicTreeState } from './topic-tree-state.js';
import { selectTopicTreeViewerTarget } from './topic-tree-selection.js';

test('selectTopicTreeViewerTarget updates both the selected topic target and viewer page', () => {
	const initial = createTopicTreeState();

	const next = selectTopicTreeViewerTarget(initial, {
		recordId: 42,
		topicId: 'topic-9',
		inputId: 42,
		page: 7
	});

	assert.equal(next.page, 7);
	assert.equal(next.treeState.selectedRecordId, 42);
	assert.equal(next.treeState.selectedTopicId, 'topic-9');
	assert.deepEqual(next.treeState.selectedPdfTarget, {
		inputId: 42,
		page: 7,
		topicId: 'topic-9'
	});
});

test('selectTopicTreeViewerTarget clamps invalid pages to 1 for the viewer', () => {
	const initial = createTopicTreeState();

	const next = selectTopicTreeViewerTarget(initial, {
		recordId: 8,
		topicId: 'topic-1',
		inputId: 8,
		page: 0
	});

	assert.equal(next.page, 1);
	assert.equal(next.treeState.selectedPdfTarget?.page, 0);
});
