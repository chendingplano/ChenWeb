export const TOPIC_TREE_PAGE_SIZE = 50;

export function buildTopicTreeListParams({
	recordId = '',
	page = 1,
	pageSize = TOPIC_TREE_PAGE_SIZE,
	activeStoreId = null,
	scopeToActiveStore = false
} = {}) {
	const params = {
		docType: 'all',
		parseState: 'all',
		fileName: '',
		startTime: '',
		endTime: '',
		page: Math.max(1, Math.trunc(page) || 1),
		pageSize: Math.max(1, Math.trunc(pageSize) || TOPIC_TREE_PAGE_SIZE),
		recordId: recordId.trim(),
		title: '',
		docNo: '',
		parserName: '',
		operation: '',
		procStatus: '',
		modifyStartTime: '',
		modifyEndTime: ''
	};

	if (scopeToActiveStore && activeStoreId != null) {
		params.ksStoreId = activeStoreId;
	}

	return params;
}

export function selectFirstRecordId(records) {
	return Array.isArray(records) && records[0] ? records[0].id : null;
}
