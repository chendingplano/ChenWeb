export const TOPIC_TREE_PAGE_SIZE = 50;

/**
 * @typedef {{
 *   searchRecordId: string;
 *   searchTitle: string;
 *   searchDocNo: string;
 *   searchFileName: string;
 *   searchDocType: string;
 *   searchParserName: string;
 *   searchOperation: string;
 *   searchProcStatus: string;
 *   searchCreateStart: string;
 *   searchCreateEnd: string;
 *   searchModifyStart: string;
 *   searchModifyEnd: string;
 * }} RecordBrowserFilters
 */

export function createDefaultRecordBrowserFilters() {
	/** @type {RecordBrowserFilters} */
	return {
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
	};
}

/**
 * @param {Partial<RecordBrowserFilters> | undefined} filters
 * @returns {boolean}
 */
export function hasActiveRecordBrowserFilters(filters = createDefaultRecordBrowserFilters()) {
	/** @type {RecordBrowserFilters} */
	const defaults = createDefaultRecordBrowserFilters();
	return /** @type {(keyof RecordBrowserFilters)[]} */ (Object.keys(defaults)).some((key) => {
		const defaultValue = defaults[key];
		const value = filters?.[key];
		return (value ?? defaultValue) !== defaultValue;
	});
}

/**
 * @param {{
 *   recordId?: string;
 *   page?: number;
 *   pageSize?: number;
 *   activeStoreId?: number | null;
 *   scopeToActiveStore?: boolean;
 *   filters?: Partial<RecordBrowserFilters>;
 * }} [options]
 */
export function buildTopicTreeListParams({
	recordId = '',
	page = 1,
	pageSize = TOPIC_TREE_PAGE_SIZE,
	activeStoreId = null,
	scopeToActiveStore = false,
	filters = createDefaultRecordBrowserFilters()
} = {}) {
	const normalizedFilters = { ...createDefaultRecordBrowserFilters(), ...filters };
	/** @type {import('$lib/services/kbService').ListKbInputsParams} */
	const params = {
		docType: normalizedFilters.searchDocType?.trim() || 'all',
		excludeDocType: 'zip',
		parseState: 'all',
		fileName: normalizedFilters.searchFileName?.trim() || '',
		startTime: normalizedFilters.searchCreateStart?.trim() || '',
		endTime: normalizedFilters.searchCreateEnd?.trim() || '',
		page: Math.max(1, Math.trunc(page) || 1),
		pageSize: Math.max(1, Math.trunc(pageSize) || TOPIC_TREE_PAGE_SIZE),
		recordId: recordId.trim() || normalizedFilters.searchRecordId?.trim() || '',
		title: normalizedFilters.searchTitle?.trim() || '',
		docNo: normalizedFilters.searchDocNo?.trim() || '',
		parserName: normalizedFilters.searchParserName?.trim() || '',
		operation: normalizedFilters.searchOperation?.trim() || '',
		procStatus:
			normalizedFilters.searchProcStatus?.trim() && normalizedFilters.searchProcStatus !== 'all'
				? normalizedFilters.searchProcStatus.trim()
				: '',
		modifyStartTime: normalizedFilters.searchModifyStart?.trim() || '',
		modifyEndTime: normalizedFilters.searchModifyEnd?.trim() || ''
	};

	if (scopeToActiveStore && activeStoreId != null) {
		params.ksStoreId = activeStoreId;
	}

	return params;
}

/**
 * @param {Array<{ id: number }>} records
 * @returns {number | null}
 */
export function selectFirstRecordId(records) {
	return Array.isArray(records) && records[0] ? records[0].id : null;
}

/**
 * @param {{
 *   records?: Array<{ id: number }>;
 *   selectedRecordId?: number | null;
 *   autoSelectFirstRecord?: boolean;
 * }} [options]
 * @returns {number | null}
 */
export function resolveSelectedRecordId({
	records = [],
	selectedRecordId = null,
	autoSelectFirstRecord = true
} = {}) {
	if (!Array.isArray(records) || records.length === 0) return null;
	if (selectedRecordId != null && records.some((record) => record.id === selectedRecordId)) {
		return selectedRecordId;
	}
	return autoSelectFirstRecord ? selectFirstRecordId(records) : null;
}
