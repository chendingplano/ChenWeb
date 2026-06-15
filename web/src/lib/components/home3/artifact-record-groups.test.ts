import test from 'node:test';
import assert from 'node:assert/strict';

import { buildArtifactRecordGroups } from './artifact-record-groups.js';

test('buildArtifactRecordGroups exposes all metric fields in curated sections', () => {
	const groups = buildArtifactRecordGroups('metric', {
		metric_id: '5_mtc_3',
		input_record_id: 5,
		metric_name: 'Switching Frequency',
		metric_name_en: 'Switching Frequency',
		metric_value: '50',
		metric_unit: 'Hz',
		metric_keywords: ['power', 'switching'],
		reasoning_tags: ['grounded'],
		source_line_spans: ['10:11']
	});

	assert.equal(groups.length > 0, true);
	assert.equal(groups.some((group) => group.title === 'Core Fields'), true);
	assert.equal(groups.some((group) => group.title === 'JSON Fields'), true);

	const core = groups.find((group) => group.title === 'Core Fields');
	assert.equal(core?.rows.some((row) => row.label === 'metric_name' && row.value === 'Switching Frequency'), true);

	const jsonFields = groups.find((group) => group.title === 'JSON Fields');
	assert.equal(
		jsonFields?.rows.some((row) => row.label === 'metric_keywords' && row.value.includes('power')),
		true
	);
	assert.equal(
		jsonFields?.rows.some((row) => row.label === 'reasoning_tags' && row.value.includes('grounded')),
		true
	);
});

test('buildArtifactRecordGroups localizes group titles for zh-cn', () => {
	const groups = buildArtifactRecordGroups(
		'metric',
		{
			metric_id: '5_mtc_3',
			metric_name: 'Switching Frequency'
		},
		'zh-cn'
	);

	assert.equal(groups.some((group) => group.title === '核心字段'), true);
});
