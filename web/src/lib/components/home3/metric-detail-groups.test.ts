import test from 'node:test';
import assert from 'node:assert/strict';

import { buildMetricGroupAttrs } from './metric-detail-groups.js';

test('metric group shows condition and condition_en', () => {
	const groups = buildMetricGroupAttrs(
		{
			id: 1,
			input_record_id: 371,
			metric_name: '有效量程',
			metric_subject: '身高测试仪(计)',
			condition: '适用人群：幼儿',
			condition_en: 'Population: preschool children'
		},
		[],
		new Map()
	);
	const byKey = new Map(groups.metric.map((a) => [a.key, a]));
	assert.equal(byKey.get('condition')?.value, '适用人群：幼儿');
	assert.equal(byKey.get('condition')?.hasValue, true);
	assert.equal(byKey.get('condition_en')?.value, 'Population: preschool children');
});

test('metric group marks a missing condition as empty', () => {
	const groups = buildMetricGroupAttrs({ id: 2, input_record_id: 371, condition: null }, [], new Map());
	const condition = groups.metric.find((a) => a.key === 'condition');
	assert.equal(condition?.hasValue, false);
	assert.equal(groups.metric.find((a) => a.key === 'condition_en')?.hasValue, false);
});
