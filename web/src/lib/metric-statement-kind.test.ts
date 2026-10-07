import test from 'node:test';
import assert from 'node:assert/strict';

import { classifyMetricStatement, type StatementKind } from './metric-statement-kind.js';

test('qualitative requirement is an inspection requirement, not a metric', () => {
	assert.deepEqual(classifyMetricStatement({ value_class: 'requirement', value_range_type: 'qualitative' }), {
		kind: 'inspection_requirement',
		group: 'requirement'
	});
});

test('numeric requirement keeps its criterion', () => {
	assert.deepEqual(classifyMetricStatement({ value_class: 'requirement', value_range_type: 'upper_bound' }), {
		kind: 'requirement_with_criterion',
		group: 'requirement'
	});
});

test('target counts as a requirement', () => {
	assert.equal(classifyMetricStatement({ value_class: 'target', value_range_type: 'exact' }).kind, 'requirement_with_criterion');
});

test('limit_absent requirement leaves the value open', () => {
	assert.equal(classifyMetricStatement({ value_class: 'requirement', value_range_type: 'limit_absent' }).kind, 'requirement_value_open');
});

test('gold metric-with-no-value rows classify like requirement + limit_absent', () => {
	assert.equal(
		classifyMetricStatement({ value_class: 'metric-with-no-value', value_range_type: 'limit_absent' }).kind,
		'requirement_value_open'
	);
});

test('delegated requirement', () => {
	assert.equal(
		classifyMetricStatement({ value_class: 'reference', value_range_type: 'qualitative', reasoning_tags: ['cited_doc:CJJ 52'] }).kind,
		'delegated_requirement'
	);
});

test('citation tag alone marks a delegated requirement', () => {
	assert.equal(
		classifyMetricStatement({ value_class: 'requirement', value_range_type: 'qualitative', reasoning_tags: ['external_reference'] }).kind,
		'delegated_requirement'
	);
});

test('test setting wins over requirement class', () => {
	assert.deepEqual(
		classifyMetricStatement({ value_class: 'requirement', value_range_type: 'exact', reasoning_tags: ['test_condition'] }),
		{ kind: 'test_parameter', group: 'test' }
	);
});

test('formula definition vs term definition', () => {
	assert.equal(classifyMetricStatement({ value_class: 'definition', formula_or_definition: 'GI = a / b' }).kind, 'metric_definition');
	assert.equal(classifyMetricStatement({ value_class: 'definition', formula_or_definition: '  ' }).kind, 'definition');
});

test('observations and design capabilities are metric values', () => {
	assert.deepEqual(classifyMetricStatement({ value_class: 'observation', value_range_type: 'exact' }), {
		kind: 'metric_value',
		group: 'metric'
	});
	assert.equal(classifyMetricStatement({ value_class: 'design_capability', value_range_type: 'qualitative' }).kind, 'observation');
});

test('unknown class is never shown as a metric', () => {
	assert.deepEqual(classifyMetricStatement({}), { kind: 'unclassified', group: 'unclassified' });
	assert.equal(classifyMetricStatement({ value_class: 'whatever', value_range_type: 'exact' }).group, 'unclassified');
});

test('tags arrive as a JSON string, or malformed', () => {
	assert.equal(
		classifyMetricStatement({ value_class: 'requirement', value_range_type: 'exact', reasoning_tags: '["test_condition"]' }).kind,
		'test_parameter'
	);
	assert.equal(
		classifyMetricStatement({ value_class: 'requirement', value_range_type: 'exact', reasoning_tags: 'not json' }).kind,
		'requirement_with_criterion'
	);
});

test('values are trimmed and case-insensitive', () => {
	assert.equal(classifyMetricStatement({ value_class: ' Requirement ', value_range_type: 'QUALITATIVE' }).kind, 'inspection_requirement');
});

// Record 416 gold benchmark (skill 2.0.0, gpt-6.1-sol, run 20261006_121911):
// [value_class, value_range_type, tags, has formula] for gold rows 1..69.
const RECORD_416: [string, string, string[], boolean][] = [
	["requirement", "qualitative", [], false],
	["reference", "qualitative", ["external_reference", "cited_doc:CJJ 27"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:CJJ 27", "cited_doc:GB 16889"], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "lower_bound", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "exact", [], false],
	["requirement", "limit_absent", [], false],
	["requirement", "limit_absent", [], false],
	["reference", "qualitative", ["external_reference", "cited_doc:CJJ 184"], false],
	["requirement", "exact", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "limit_absent", [], false],
	["requirement", "limit_absent", [], false],
	["requirement", "limit_absent", [], false],
	["reference", "qualitative", ["external_reference", "cited_doc:CJJ 52"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:CJJ 52"], false],
	["requirement", "limit_absent", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["reference", "qualitative", ["external_reference", "cited_doc:NY/T 90"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:NY/T 2371"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:GB 50869"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:GB 16889"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:CJJ 90"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:GB 18485"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:GB/T 31962"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:GB 16889"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:GB 14554"], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "qualitative", [], false],
	["requirement", "limit_absent", [], false],
	["reference", "qualitative", ["external_reference", "cited_doc:NY 1109"], false],
	["requirement", "lower_bound", [], false],
	["reference", "qualitative", ["external_reference", "cited_doc:NY884"], false],
	["reference", "qualitative", ["external_reference", "cited_doc:NY884"], false],
	["requirement", "lower_bound", [], false],
	["requirement", "upper_bound", [], false],
	["requirement", "range", [], false],
	["requirement", "upper_bound", [], false],
	["requirement", "upper_bound", [], false],
	["requirement", "upper_bound", [], false],
	["requirement", "upper_bound", [], false],
	["requirement", "upper_bound", [], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "lower_bound", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "lower_bound", ["test_condition"], false],
	["requirement", "lower_bound", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "range", ["test_condition"], false],
	["requirement", "upper_bound", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["requirement", "exact", ["test_condition"], false],
	["definition", "qualitative", [], true],
	["reference", "upper_bound", ["strict_bound"], false],
	["reference", "lower_bound", ["strict_bound"], false],
];

test('record 416 benchmark rows sort into the expected kinds', () => {
	const counts: Partial<Record<StatementKind, number>> = {};
	for (const [value_class, value_range_type, reasoning_tags, hasFormula] of RECORD_416) {
		const { kind } = classifyMetricStatement({
			value_class,
			value_range_type,
			reasoning_tags,
			formula_or_definition: hasFormula ? 'formula' : ''
		});
		counts[kind] = (counts[kind] ?? 0) + 1;
	}
	assert.deepEqual(counts, {
		inspection_requirement: 15,
		delegated_requirement: 17,
		requirement_with_criterion: 12,
		requirement_value_open: 7,
		test_parameter: 15,
		metric_definition: 1,
		metric_value: 2 // GI < 100% / > 100% interpretation bounds
	});
});
