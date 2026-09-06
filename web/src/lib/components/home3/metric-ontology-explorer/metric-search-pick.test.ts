import test from 'node:test';
import assert from 'node:assert/strict';

import { pickableMetricId } from './metric-search-pick.js';

test('canonical "<record>_mtc_<seq>" id is pickable', () => {
	assert.equal(pickableMetricId({ metric_id: '412_mtc_7' }), '412_mtc_7');
	assert.equal(pickableMetricId({ metric_id: '  412_mtc_7  ' }), '412_mtc_7');
});

test('legacy "<record>_<seq>" id is pickable', () => {
	assert.equal(pickableMetricId({ metric_id: '412_7' }), '412_7');
});

test('blank, null, missing, or non-canonical id is not pickable', () => {
	assert.equal(pickableMetricId({ metric_id: '' }), null);
	assert.equal(pickableMetricId({ metric_id: '   ' }), null);
	assert.equal(pickableMetricId({ metric_id: null }), null); // kb.metrics row shape
	assert.equal(pickableMetricId({}), null);
	assert.equal(pickableMetricId(null), null);
	assert.equal(pickableMetricId({ metric_id: 'tensile strength' }), null);
	assert.equal(pickableMetricId({ metric_id: '412_mtc_' }), null);
	assert.equal(pickableMetricId({ metric_id: 'mtc_7' }), null);
});
