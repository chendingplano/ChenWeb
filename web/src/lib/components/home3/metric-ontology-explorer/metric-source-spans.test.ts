import test from 'node:test';
import assert from 'node:assert/strict';

import {
	lineNumbersFromSpans,
	resolveMetricSpans,
	recordIdFromMetricId
} from './metric-source-spans.js';

test('lineNumbersFromSpans handles numbers, strings, ranges, objects — deduped + sorted', () => {
	assert.deepEqual(lineNumbersFromSpans([90]), [90]);
	assert.deepEqual(lineNumbersFromSpans(['90']), [90]);
	assert.deepEqual(lineNumbersFromSpans(['98:99']), [98, 99]);
	assert.deepEqual(lineNumbersFromSpans(['10-12', 11]), [10, 11, 12]);
	assert.deepEqual(lineNumbersFromSpans([{ line_number: 5 }, { line: 3 }]), [3, 5]);
	assert.deepEqual(lineNumbersFromSpans(['7,7', 7]), [7]);
});

test('lineNumbersFromSpans is defensive about junk', () => {
	assert.deepEqual(lineNumbersFromSpans(undefined), []);
	assert.deepEqual(lineNumbersFromSpans(null), []);
	assert.deepEqual(lineNumbersFromSpans('90'), []); // not an array
	assert.deepEqual(lineNumbersFromSpans([0, -3, 'x', {}]), []);
	assert.equal(lineNumbersFromSpans(['1:9999']).length, 201); // range cap = start + 200
});

test('resolveMetricSpans maps line numbers to pages via raw lines, dropping unmapped', () => {
	const rawLines = [
		{ line_number: 90, page_number: 3, line_type: '', content: '', coords: [1, 2, 3, 4] },
		{ line_number: 91, page_number: 3, line_type: '', content: '', coords: [1, 2, 3, 4] },
		{ line_number: 200, page_number: 7, line_type: '', content: '', coords: [1, 2, 3, 4] }
	];
	assert.deepEqual(resolveMetricSpans(['90:91'], rawLines), [
		{ page_number: 3, line_number: 90 },
		{ page_number: 3, line_number: 91 }
	]);
	// line 999 has no raw line -> dropped
	assert.deepEqual(resolveMetricSpans([200, 999], rawLines), [
		{ page_number: 7, line_number: 200 }
	]);
	assert.deepEqual(resolveMetricSpans(['90'], []), []);
});

test('recordIdFromMetricId reads the canonical / legacy prefix', () => {
	assert.equal(recordIdFromMetricId('416_mtc_2'), 416);
	assert.equal(recordIdFromMetricId('  416_mtc_2 '), 416);
	assert.equal(recordIdFromMetricId('416_2'), 416); // legacy
	assert.equal(recordIdFromMetricId(''), null);
	assert.equal(recordIdFromMetricId('mtc_2'), null);
	assert.equal(recordIdFromMetricId('abc_mtc_2'), null);
});
