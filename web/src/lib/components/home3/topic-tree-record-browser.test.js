import test from 'node:test';
import assert from 'node:assert/strict';

import {
	TOPIC_TREE_PAGE_SIZE,
	buildTopicTreeListParams,
	selectFirstRecordId
} from './topic-tree-record-browser.js';

test('buildTopicTreeListParams defaults to unscoped kb.inputs pagination', () => {
	const params = buildTopicTreeListParams({ page: 3 });

	assert.equal(params.page, 3);
	assert.equal(params.pageSize, TOPIC_TREE_PAGE_SIZE);
	assert.equal(params.recordId, '');
	assert.equal('ksStoreId' in params, false);
});

test('buildTopicTreeListParams includes the active store only when explicitly scoped', () => {
	const params = buildTopicTreeListParams({
		recordId: ' 1042 ',
		page: 2,
		activeStoreId: 88,
		scopeToActiveStore: true
	});

	assert.equal(params.recordId, '1042');
	assert.equal(params.page, 2);
	assert.equal(params.ksStoreId, 88);
});

test('selectFirstRecordId picks the first record for initial auto-selection', () => {
	assert.equal(selectFirstRecordId([]), null);
	assert.equal(selectFirstRecordId([{ id: 42 }, { id: 77 }]), 42);
});
