import test from 'node:test';
import assert from 'node:assert/strict';

import {
	createSummaryTreeState,
	toggleSummaryTreeListMode,
	selectSummaryTreeRecord,
	selectRecordSummaryTarget
} from './summary-tree-state.js';

test('summary tree defaults to compact list mode', () => {
	const state = createSummaryTreeState();

	assert.equal(state.listMode, 'compact');
	assert.equal(state.selectedRecordId, null);
	assert.equal(state.selectedSummaryId, null);
});

test('toggleSummaryTreeListMode switches between compact and cards', () => {
	const initial = createSummaryTreeState();
	const toggled = toggleSummaryTreeListMode(initial);

	assert.equal(toggled.listMode, 'cards');
	assert.equal(toggleSummaryTreeListMode(toggled).listMode, 'compact');
});

test('selectSummaryTreeRecord updates the active record and clears summary selection', () => {
	const state = createSummaryTreeState();
	state.selectedRecordId = 14;
	state.selectedSummaryId = 'summary-1';
	state.selectedPdfTarget = { inputId: 14, page: 2, summaryId: 'summary-1' };

	const next = selectSummaryTreeRecord(state, 20);

	assert.equal(next.selectedRecordId, 20);
	assert.equal(next.selectedSummaryId, null);
	assert.equal(next.selectedPdfTarget, null);
});

test('selectRecordSummaryTarget stores the summary id and target page', () => {
	const initial = createSummaryTreeState();

	const next = selectRecordSummaryTarget(initial, {
		recordId: 42,
		summaryId: 'summary-9',
		inputId: 42,
		page: 7
	});

	assert.equal(next.selectedRecordId, 42);
	assert.equal(next.selectedSummaryId, 'summary-9');
	assert.deepEqual(next.selectedPdfTarget, {
		inputId: 42,
		page: 7,
		summaryId: 'summary-9'
	});
});
