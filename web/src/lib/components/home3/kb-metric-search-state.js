export const KB_METRIC_SEARCH_DEFAULTS = Object.freeze({
	page: 1,
	pageSize: 20,
	minRank: 0
});

/**
 * @typedef {{
 *   inputRecordId: string;
 *   isExplicitMetric: string;
 *   valueClass: string;
 *   valueDataType: string;
 *   metricUnit: string;
 * }} KbMetricSearchUiFilters
 */

/**
 * @returns {KbMetricSearchUiFilters}
 */
export function createEmptyKbMetricSearchFilters() {
	return {
		inputRecordId: '',
		isExplicitMetric: '',
		valueClass: '',
		valueDataType: '',
		metricUnit: ''
	};
}

/**
 * @param {KbMetricSearchUiFilters | null | undefined} filters
 * @returns {boolean}
 */
export function hasKbMetricSearchFilters(filters) {
	if (!filters || typeof filters !== 'object') return false;
	return Boolean(
		String(filters.inputRecordId ?? '').trim() ||
			String(filters.isExplicitMetric ?? '').trim() ||
			String(filters.valueClass ?? '').trim() ||
			String(filters.valueDataType ?? '').trim() ||
			String(filters.metricUnit ?? '').trim()
		);
}

/**
 * @param {{
 *   query: string;
 *   page?: number;
 *   pageSize?: number;
 *   filters?: KbMetricSearchUiFilters | null | undefined;
 * }} input
 * @returns {import('$lib/services/kbMetricSearch').KbMetricSearchParams}
 */
export function buildKbMetricSearchParams({ query, page, pageSize, filters }) {
	const safePage = Number.isFinite(page) ? Number(page) : NaN;
	const safePageSize = Number.isFinite(pageSize) ? Number(pageSize) : NaN;
	const safeFilters = filters ?? createEmptyKbMetricSearchFilters();

	/** @type {import('$lib/services/kbMetricSearch').KbMetricSearchParams} */
	const params = {
		q: String(query ?? '').trim(),
		page: safePage > 0 ? safePage : KB_METRIC_SEARCH_DEFAULTS.page,
		pageSize: safePageSize > 0 ? safePageSize : KB_METRIC_SEARCH_DEFAULTS.pageSize
	};

	const inputRecordId = Number.parseInt(String(safeFilters.inputRecordId ?? '').trim(), 10);
	if (Number.isFinite(inputRecordId) && inputRecordId > 0) {
		params.inputRecordId = inputRecordId;
	}
	if (safeFilters.isExplicitMetric === 'true') {
		params.isExplicitMetric = true;
	} else if (safeFilters.isExplicitMetric === 'false') {
		params.isExplicitMetric = false;
	}
	if (String(safeFilters.valueClass ?? '').trim()) {
		params.valueClass = String(safeFilters.valueClass).trim();
	}
	if (String(safeFilters.valueDataType ?? '').trim()) {
		params.valueDataType = String(safeFilters.valueDataType).trim();
	}
	if (String(safeFilters.metricUnit ?? '').trim()) {
		params.metricUnit = String(safeFilters.metricUnit).trim();
	}

	return params;
}
