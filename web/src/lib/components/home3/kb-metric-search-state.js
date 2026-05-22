export const KB_METRIC_SEARCH_DEFAULTS = Object.freeze({
	page: 1,
	pageSize: 20,
	minRank: 0
});

export function createEmptyKbMetricSearchFilters() {
	return {
		inputRecordId: '',
		isExplicitMetric: '',
		valueClass: '',
		valueDataType: '',
		metricUnit: ''
	};
}

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

export function buildKbMetricSearchParams({ query, page, pageSize, filters }) {
	const params = {
		q: String(query ?? '').trim(),
		page: Number.isFinite(page) && page > 0 ? page : KB_METRIC_SEARCH_DEFAULTS.page,
		pageSize:
			Number.isFinite(pageSize) && pageSize > 0 ? pageSize : KB_METRIC_SEARCH_DEFAULTS.pageSize
	};

	const inputRecordId = Number.parseInt(String(filters?.inputRecordId ?? '').trim(), 10);
	if (Number.isFinite(inputRecordId) && inputRecordId > 0) {
		params.inputRecordId = inputRecordId;
	}
	if (filters?.isExplicitMetric === 'true') {
		params.isExplicitMetric = true;
	} else if (filters?.isExplicitMetric === 'false') {
		params.isExplicitMetric = false;
	}
	if (String(filters?.valueClass ?? '').trim()) {
		params.valueClass = String(filters.valueClass).trim();
	}
	if (String(filters?.valueDataType ?? '').trim()) {
		params.valueDataType = String(filters.valueDataType).trim();
	}
	if (String(filters?.metricUnit ?? '').trim()) {
		params.metricUnit = String(filters.metricUnit).trim();
	}

	return params;
}
