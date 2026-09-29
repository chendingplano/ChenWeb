import test from 'node:test';
import assert from 'node:assert/strict';

import { buildInputSearchQuery, groupNonMetrics, sortBySeverity, type NonMetricEntry } from './metric-review-client.js';

test('numeric query searches by record_id', () => {
	const p = new URLSearchParams(buildInputSearchQuery(' 416 '));
	assert.equal(p.get('record_id'), '416');
	assert.equal(p.get('title'), null);
});

test('text query searches by title', () => {
	const p = new URLSearchParams(buildInputSearchQuery('垃圾分类'));
	assert.equal(p.get('title'), '垃圾分类');
	assert.equal(p.get('record_id'), null);
});

test('blank query sends neither filter', () => {
	const p = new URLSearchParams(buildInputSearchQuery('  '));
	assert.equal(p.get('title'), null);
	assert.equal(p.get('record_id'), null);
});

test('sortBySeverity puts high first and keeps order within a severity', () => {
	const out = sortBySeverity([
		{ id: 1, severity: 'low' },
		{ id: 2, severity: 'high' },
		{ id: 3, severity: 'medium' },
		{ id: 4, severity: 'high' }
	]);
	assert.deepEqual(
		out.map((x) => x.id),
		[2, 4, 3, 1]
	);
});

test('groupNonMetrics orders categories and omits empty groups', () => {
	const entries: NonMetricEntry[] = [
		{ metric_ids: ['a'], category: 'formula_input', reason: '' },
		{ metric_ids: ['b'], category: 'not_metric', reason: '' }
	];
	const groups = groupNonMetrics(entries);
	assert.deepEqual(
		groups.map((g) => g.category),
		['not_metric', 'formula_input']
	);
});
