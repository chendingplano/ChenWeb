import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildJsonSections,
	buildMatchedUnitsSections,
	buildMetadataSections,
	formatCompactContent
} from './doc-review-json-dialog';

test('buildMatchedUnitsSections flattens metric wrappers and preserves string arrays', () => {
	const sections = buildMatchedUnitsSections([
		{
			metric: {
				metric_id: '415_mtc_72',
				metric_name: 'Morning minimum diastolic blood pressure time',
				metric_categories: ['blood-pressure', 'circadian-pattern']
			},
			match_via: 'hybrid_search',
			match_rank: 1
		}
	]);

	assert.equal(sections.length, 1);
	assert.deepEqual(sections[0]?.rows, [
		{ label: 'metric_id', value: '415_mtc_72' },
		{ label: 'metric_name', value: 'Morning minimum diastolic blood pressure time' },
		{ label: 'metric_categories', value: "['blood-pressure', 'circadian-pattern']" }
	]);
});

test('buildMatchedUnitsSections creates one section per matched unit', () => {
	const sections = buildMatchedUnitsSections([
		{ metric: { metric_id: '1_mtc_1' } },
		{ metric: { metric_id: '1_mtc_2' } }
	]);

	assert.equal(sections.length, 2);
	assert.deepEqual(sections[0]?.rows, [{ label: 'metric_id', value: '1_mtc_1' }]);
	assert.deepEqual(sections[1]?.rows, [{ label: 'metric_id', value: '1_mtc_2' }]);
});

test('buildJsonSections keeps findings as simple name-value pairs', () => {
	const sections = buildJsonSections([
		{ severity: 'warning', reason: 'missing source span' },
		{ severity: 'error', reason: 'unit mismatch' }
	]);

	assert.equal(sections.length, 2);
	assert.deepEqual(sections[0]?.rows, [
		{ label: 'severity', value: 'warning' },
		{ label: 'reason', value: 'missing source span' }
	]);
});

test('buildJsonSections handles scalar values and nulls', () => {
	assert.deepEqual(buildJsonSections('ok'), [{ rows: [{ label: 'value', value: 'ok' }] }]);
	assert.deepEqual(buildJsonSections(null), [{ rows: [{ label: 'value', value: 'null' }] }]);
});

test('buildMetadataSections expands a JSON-string object field into indented rows', () => {
	const sections = buildMetadataSections({
		en: JSON.stringify({ title: 'Metric comparison', provenance: 'canonical' }),
		run_id: 78
	});

	assert.equal(sections.length, 1);
	assert.deepEqual(sections[0]?.rows, [
		{ label: 'en', value: '' },
		{ label: 'title', value: 'Metric comparison', indent: true },
		{ label: 'provenance', value: 'canonical', indent: true },
		{ label: 'run_id', value: '78' }
	]);
});

test('buildMetadataSections expands a JSON-string array of objects with index-prefixed rows', () => {
	const sections = buildMetadataSections({
		related_artifacts: JSON.stringify([
			{ summary: 'same metric', relationship: 'same_consistent' },
			{ summary: 'different metric', relationship: 'related_distinct' }
		])
	});

	assert.deepEqual(sections[0]?.rows, [
		{ label: 'related_artifacts', value: '2 items' },
		{ label: '[0] summary', value: 'same metric', indent: true },
		{ label: '[0] relationship', value: 'same_consistent', indent: true },
		{ label: '[1] summary', value: 'different metric', indent: true },
		{ label: '[1] relationship', value: 'related_distinct', indent: true }
	]);
});

test('buildMetadataSections leaves plain scalar fields untouched', () => {
	const sections = buildMetadataSections({ source_language: 'en', schema_version: 1 });
	assert.deepEqual(sections[0]?.rows, [
		{ label: 'source_language', value: 'en' },
		{ label: 'schema_version', value: '1' }
	]);
});

test('formatCompactContent unwraps simple unit location wrappers', () => {
	assert.equal(formatCompactContent({ line_spans: ['91'] }), '[91]');
	assert.equal(formatCompactContent({ source_line_spans: [58, 91] }), '[58, 91]');
});

test('formatCompactContent keeps a JSON fallback for more complex objects', () => {
	assert.equal(
		formatCompactContent({ line_spans: ['91'], page: 3 }),
		'{"line_spans":["91"],"page":3}'
	);
});
