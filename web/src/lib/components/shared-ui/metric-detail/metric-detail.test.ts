import test from 'node:test';
import assert from 'node:assert/strict';
import { buildMetricDetailGroups, metricTitle } from './metric-detail';

test('groups come in panel order with filled/total counts and empty fields kept', () => {
	const groups = buildMetricDetailGroups(
		{ id: 47657, input_record_id: 416, metric_id: '416_mtc_1', metric_name: '易腐垃圾收运频次' },
		[]
	);
	assert.deepEqual(
		groups.map((g) => g.key),
		['metric', 'metadata', 'context', 'grounding', 'reasoning']
	);
	const metric = groups[0];
	const byKey = new Map(metric.fields.map((f) => [f.key, f]));
	assert.deepEqual(byKey.get('name')?.values, [{ text: '易腐垃圾收运频次', mono: false }]);
	assert.deepEqual(byKey.get('metric_artifact_id')?.values, [{ text: '416_mtc_1', mono: true }]);
	assert.deepEqual(byKey.get('unit')?.values, []);
	assert.equal(metric.total, metric.fields.length);
	assert.equal(metric.filled, metric.fields.filter((f) => f.values.length > 0).length);
	assert.equal(groups[3].filled, 0);
});

test('chips, frame JSON and cited lines become one value each', () => {
	const groups = buildMetricDetailGroups(
		{
			id: 1,
			input_record_id: 416,
			metric_keywords: ['易腐垃圾', '收运频次'],
			frame: { procedure: { step: '8.1.2' } },
			source_line_spans: ['90']
		},
		[{ page_number: 6, line_number: 90, content: '8.1.2 易腐垃圾应每日定时收运', line_type: 'text', coords: [] }]
	);
	const field = (group: string, key: string) =>
		groups.find((g) => g.key === group)?.fields.find((f) => f.key === key);
	assert.deepEqual(field('context', 'keywords')?.values, [{ text: '易腐垃圾' }, { text: '收运频次' }]);
	assert.deepEqual(field('metric', 'frame_json')?.values, [
		{ text: 'procedure', mono: true },
		{ text: '  step: 8.1.2', mono: true }
	]);
	assert.deepEqual(field('grounding', 'source_line_spans')?.values, [
		{ text: 'L90 · P6 (text)\n8.1.2 易腐垃圾应每日定时收运' }
	]);
});

test('title falls back from name to subject to the row id', () => {
	assert.deepEqual(metricTitle({ id: 1, input_record_id: 1, metric_name: ' A ', metric_name_en: 'a' }), {
		name: 'A',
		subname: 'a'
	});
	assert.equal(metricTitle({ id: 1, input_record_id: 1, metric_subject: 'S' }).name, 'S');
	assert.match(metricTitle({ id: 7, input_record_id: 1 }).name, /#7/);
});
