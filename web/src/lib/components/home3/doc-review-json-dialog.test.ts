import test from 'node:test';
import assert from 'node:assert/strict';

import {
	buildJsonSections,
	buildMatchedUnitsSections,
	buildMetadataSections,
	formatCompactContent,
	lineNumbersFromSpans,
	matchedUnitFocusTarget,
	relatedArtifactsFromMetadata
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

test('relatedArtifactsFromMetadata reads a plain array field', () => {
	const result = relatedArtifactsFromMetadata({
		related_artifacts: [
			{
				summary: '同一指标，同一条件：候选指标来自嘉兴市第一医院...',
				relationship: 'same_consistent',
				related_record_id: 430,
				related_artifact_id: '430_mtc_1'
			},
			{
				summary: '不同指标：候选指标为"两臂收缩压差>5 mmHg"...',
				relationship: 'related_distinct',
				related_record_id: 415,
				related_artifact_id: '415_mtc_1'
			}
		]
	});

	assert.deepEqual(result, [
		{
			relationship: 'same_consistent',
			related_record_id: '430',
			related_artifact_id: '430_mtc_1',
			summary: '同一指标，同一条件：候选指标来自嘉兴市第一医院...',
			fields: []
		},
		{
			relationship: 'related_distinct',
			related_record_id: '415',
			related_artifact_id: '415_mtc_1',
			summary: '不同指标：候选指标为"两臂收缩压差>5 mmHg"...',
			fields: []
		}
	]);
});

test('relatedArtifactsFromMetadata parses a JSON-encoded string field', () => {
	const result = relatedArtifactsFromMetadata({
		related_artifacts: JSON.stringify([
			{ summary: 'same metric', relationship: 'same_consistent', related_record_id: 430, related_artifact_id: '430_mtc_1' }
		])
	});

	assert.deepEqual(result, [
		{ relationship: 'same_consistent', related_record_id: '430', related_artifact_id: '430_mtc_1', summary: 'same metric', fields: [] }
	]);
});

test('relatedArtifactsFromMetadata reads the singular shape (ordinary "issue" findings) and flattens related_artifact_fields', () => {
	const result = relatedArtifactsFromMetadata({
		related_artifact_id: '430_mtc_19',
		related_record_id: 430,
		analysis_relationship: 'same_conflict',
		related_artifact_fields: { metric_name: '有效记录时长', metric_value: '≥24 h' }
	});

	assert.deepEqual(result, [
		{
			relationship: 'same_conflict',
			related_record_id: '430',
			related_artifact_id: '430_mtc_19',
			summary: '',
			fields: [
				{ label: 'metric_name', value: '有效记录时长' },
				{ label: 'metric_value', value: '≥24 h' }
			]
		}
	]);
});

test('relatedArtifactsFromMetadata parses a JSON-encoded related_artifact_fields string', () => {
	const result = relatedArtifactsFromMetadata({
		related_artifact_id: '430_mtc_19',
		related_record_id: 430,
		related_artifact_fields: JSON.stringify({ metric_name: '有效记录时长' })
	});

	assert.deepEqual(result, [
		{
			relationship: 'null',
			related_record_id: '430',
			related_artifact_id: '430_mtc_19',
			summary: '',
			fields: [{ label: 'metric_name', value: '有效记录时长' }]
		}
	]);
});

test('relatedArtifactsFromMetadata returns [] when metadata is missing, non-object, or has no related_artifacts', () => {
	assert.deepEqual(relatedArtifactsFromMetadata(null), []);
	assert.deepEqual(relatedArtifactsFromMetadata('not an object'), []);
	assert.deepEqual(relatedArtifactsFromMetadata({ schema_version: 1 }), []);
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

test('lineNumbersFromSpans expands, de-duplicates, and sorts single/ranged/colon spans', () => {
	assert.deepEqual(lineNumbersFromSpans(['27']), [27]);
	assert.deepEqual(lineNumbersFromSpans(['53-56']), [53, 54, 55, 56]);
	assert.deepEqual(lineNumbersFromSpans(['10', '12:13', '10']), [10, 12, 13]);
	assert.deepEqual(lineNumbersFromSpans(['14', '5']), [5, 14]);
});

test('lineNumbersFromSpans returns [] for non-arrays or unparseable entries', () => {
	assert.deepEqual(lineNumbersFromSpans(undefined), []);
	assert.deepEqual(lineNumbersFromSpans('27'), []);
	assert.deepEqual(lineNumbersFromSpans([42, 'not-a-span']), []);
});

test('matchedUnitFocusTarget expands single and ranged source_line_spans across metric/provision/item keys', () => {
	assert.deepEqual(
		matchedUnitFocusTarget({
			metric: { metric_id: '415_mtc_72', source_line_spans: ['27'] },
			source_record_id: 415
		}),
		{ recordId: 415, lineNumbers: [27] }
	);
	assert.deepEqual(
		matchedUnitFocusTarget({
			provision: { prv_id: '415_prv_3', source_line_spans: ['53-56'] },
			source_record_id: 415
		}),
		{ recordId: 415, lineNumbers: [53, 54, 55, 56] }
	);
	assert.deepEqual(
		matchedUnitFocusTarget({
			item: { inv_id: '415_inv_1', source_line_spans: ['10', '12:13'] },
			source_record_id: 415
		}),
		{ recordId: 415, lineNumbers: [10, 12, 13] }
	);
});

test('matchedUnitFocusTarget falls back to source_context line numbers when spans are absent', () => {
	assert.deepEqual(
		matchedUnitFocusTarget({
			entity: { entity_id: '415_ent_1' },
			source_record_id: 415,
			source_context: [
				{ line_number: 12, content: 'a' },
				{ line_number: 14, content: 'b' }
			]
		}),
		{ recordId: 415, lineNumbers: [12, 14] }
	);
});

test('matchedUnitFocusTarget returns empty lineNumbers (but a valid recordId) when no location data exists', () => {
	assert.deepEqual(
		matchedUnitFocusTarget({ entity: { entity_id: '415_ent_1' }, source_record_id: 415 }),
		{ recordId: 415, lineNumbers: [] }
	);
});

test('matchedUnitFocusTarget returns null when source_record_id is missing or invalid', () => {
	assert.equal(matchedUnitFocusTarget({ metric: { metric_id: '415_mtc_72' } }), null);
	assert.equal(
		matchedUnitFocusTarget({ metric: { metric_id: '415_mtc_72' }, source_record_id: 0 }),
		null
	);
	assert.equal(matchedUnitFocusTarget('not an object'), null);
});
