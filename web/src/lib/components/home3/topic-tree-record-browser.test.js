import test from 'node:test';
import assert from 'node:assert/strict';

import {
	TOPIC_TREE_PAGE_SIZE,
	buildTopicTreeListParams,
	createDefaultRecordBrowserFilters,
	hasActiveRecordBrowserFilters,
	resolveSelectedRecordId,
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

test('createDefaultRecordBrowserFilters returns the default empty filter state', () => {
	const filters = createDefaultRecordBrowserFilters();

	assert.deepEqual(filters, {
		searchRecordId: '',
		searchTitle: '',
		searchDocNo: '',
		searchFileName: '',
		searchDocType: 'all',
		searchParserName: '',
		searchOperation: '',
		searchProcStatus: 'all',
		searchCreateStart: '',
		searchCreateEnd: '',
		searchModifyStart: '',
		searchModifyEnd: ''
	});
});

test('buildTopicTreeListParams applies search filters to the kb.inputs query', () => {
	const params = buildTopicTreeListParams({
		page: 4,
		filters: {
			searchRecordId: '84',
			searchTitle: 'standard',
			searchDocNo: 'GB/T',
			searchFileName: 'report',
			searchDocType: 'pdf',
			searchParserName: 'mineru',
			searchOperation: 'extract_metadata',
			searchProcStatus: 'success',
			searchCreateStart: '2026-05-01',
			searchCreateEnd: '2026-05-02',
			searchModifyStart: '2026-05-03',
			searchModifyEnd: '2026-05-04'
		}
	});

	assert.equal(params.recordId, '84');
	assert.equal(params.title, 'standard');
	assert.equal(params.docNo, 'GB/T');
	assert.equal(params.fileName, 'report');
	assert.equal(params.docType, 'pdf');
	assert.equal(params.parserName, 'mineru');
	assert.equal(params.operation, 'extract_metadata');
	assert.equal(params.procStatus, 'success');
	assert.equal(params.startTime, '2026-05-01');
	assert.equal(params.endTime, '2026-05-02');
	assert.equal(params.modifyStartTime, '2026-05-03');
	assert.equal(params.modifyEndTime, '2026-05-04');
	assert.equal(params.page, 4);
});

test('hasActiveRecordBrowserFilters reports whether reset should be enabled', () => {
	assert.equal(hasActiveRecordBrowserFilters(createDefaultRecordBrowserFilters()), false);
	assert.equal(
		hasActiveRecordBrowserFilters({
			...createDefaultRecordBrowserFilters(),
			searchTitle: 'compliance'
		}),
		true
	);
});

test('resolveSelectedRecordId preserves a visible selection and falls back to the first record', () => {
	const records = [{ id: 11 }, { id: 12 }, { id: 13 }];

	assert.equal(resolveSelectedRecordId({ records, selectedRecordId: 12 }), 12);
	assert.equal(resolveSelectedRecordId({ records, selectedRecordId: 99 }), 11);
	assert.equal(resolveSelectedRecordId({ records, selectedRecordId: null }), 11);
	assert.equal(resolveSelectedRecordId({ records: [], selectedRecordId: 12 }), null);
	assert.equal(
		resolveSelectedRecordId({
			records,
			selectedRecordId: null,
			autoSelectFirstRecord: false
		}),
		null
	);
});
