import test from 'node:test';
import assert from 'node:assert/strict';

import {
	KB_METRIC_SEARCH_DEFAULTS,
	buildKbMetricSearchParams,
	createEmptyKbMetricSearchFilters
} from './kb-metric-search-state.js';

test('metric search defaults are LLM-friendly but bounded', () => {
	assert.equal(KB_METRIC_SEARCH_DEFAULTS.page, 1);
	assert.equal(KB_METRIC_SEARCH_DEFAULTS.pageSize, 20);
	assert.equal(KB_METRIC_SEARCH_DEFAULTS.minRank, 0);
});

test('buildKbMetricSearchParams trims query text and omits empty filters', () => {
	const params = buildKbMetricSearchParams({
		query: '  energy intensity  ',
		page: 3,
		pageSize: 40,
		filters: {
			...createEmptyKbMetricSearchFilters(),
			inputRecordId: '7',
			valueClass: 'performance',
			metricUnit: 'kWh/m2'
		}
	});

	assert.deepEqual(params, {
		q: 'energy intensity',
		page: 3,
		pageSize: 40,
		inputRecordId: 7,
		valueClass: 'performance',
		metricUnit: 'kWh/m2'
	});
});
