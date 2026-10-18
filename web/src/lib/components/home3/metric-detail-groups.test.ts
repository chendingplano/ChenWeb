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

test('metric group shows the frame: uses with spec, method and procedure', () => {
	const groups = buildMetricGroupAttrs(
		{
			id: 3,
			input_record_id: 364,
			metric_name: '葡萄糖静推剂量',
			frame: {
				uses: [
					{ role: 'agent', name: '葡萄糖', spec: [{ property: '浓度', value: '50', unit: '%' }] }
				],
				method: [{ label: '给药途径', value: '静脉推注' }],
				procedure: { step: 'A.2.4' }
			}
		},
		[],
		new Map()
	);
	const byKey = new Map(groups.metric.map((a) => [a.key, a]));
	assert.equal(byKey.get('frame_uses')?.items.length, 1);
	assert.match(byKey.get('frame_uses')?.items[0] ?? '', /葡萄糖 \(浓度 50%\)/);
	assert.deepEqual(byKey.get('frame_method')?.items, ['给药途径：静脉推注']);
	assert.equal(byKey.get('frame_procedure')?.value, 'A.2.4');
	const empty = buildMetricGroupAttrs({ id: 4, input_record_id: 364, frame: null }, [], new Map());
	assert.equal(empty.metric.find((a) => a.key === 'frame_uses')?.hasValue, false);
	assert.equal(empty.metric.find((a) => a.key === 'frame_procedure')?.hasValue, false);
});
