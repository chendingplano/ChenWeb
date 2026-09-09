import test from 'node:test';
import assert from 'node:assert/strict';

import { SATELLITES, CHAINS, CHAIN_NODE_BY_ID, CHAINED_SATELLITES } from './model.js';
import { PROJECT } from '../../../services/metricOntologyExplorerService.js';

test('nine satellites in canvas order', () => {
	assert.deepEqual(
		SATELLITES.map((s) => s.id),
		['document', 'object', 'keyword', 'evidence', 'mdef', 'processor', 'product', 'analysis', 'misc']
	);
});

test('exactly three satellites have no chain', () => {
	const chainless = SATELLITES.filter((s) => !CHAINED_SATELLITES.has(s.id)).map((s) => s.id);
	assert.deepEqual(chainless.sort(), ['document', 'misc', 'product']);
});

test('analysis satellite unfolds a two-node chain', () => {
	assert.ok(CHAINED_SATELLITES.has('analysis'));
	assert.deepEqual(
		CHAINS.analysis.map((n) => n.label),
		['Metrics of Same Class', 'Metrics of Similar Classes']
	);
	assert.deepEqual(
		CHAINS.analysis.map((n) => n.id),
		['analysis__same_class', 'analysis__similar_class']
	);
	assert.deepEqual(
		CHAINS.analysis.map((n) => n.related),
		['same_class', 'similar_class']
	);
	assert.equal(CHAIN_NODE_BY_ID['analysis__same_class'].parentId, 'analysis');
	assert.equal(CHAIN_NODE_BY_ID['analysis__similar_class'].parentId, 'analysis');
});

test('chains match the spec exactly (labels, order)', () => {
	const shape = (id: string) => CHAINS[id].map((n) => n.label);
	assert.deepEqual(shape('object'), ['Object Mention', 'Object Node']);
	assert.deepEqual(shape('keyword'), ['Keyword Concept']);
	assert.deepEqual(shape('mdef'), ['Ontology Term', 'Class Contract Revision']);
	assert.deepEqual(shape('processor'), [
		'Extract Metrics',
		'Normalize Assertion',
		'Associate Semantics',
		'Project Semantics'
	]);
	assert.deepEqual(shape('evidence'), [
		'Assertion Evidence',
		'Decision Candidate',
		'Semantic Assertion'
	]);
});

test('every chain node has a PROJECT entry that fills its columns', () => {
	for (const n of Object.values(CHAIN_NODE_BY_ID)) {
		const project = PROJECT[n.id];
		assert.ok(project, `${n.id} has a PROJECT entry`);
		// A sparse row (no known fields) still projects to one cell per column.
		assert.equal(project({}).length, n.columns.length, `${n.id} column count`);
	}
});

test('PROJECT has no entry without a matching chain node', () => {
	for (const id of Object.keys(PROJECT)) {
		assert.ok(CHAIN_NODE_BY_ID[id], `PROJECT.${id} matches a chain node`);
	}
});

test('every chain node carries a table name, columns and a description', () => {
	for (const n of Object.values(CHAIN_NODE_BY_ID)) {
		assert.ok(n.table.length > 0, `${n.id} table`);
		assert.ok(n.columns.length > 0, `${n.id} columns`);
		assert.ok(n.description.length > 0, `${n.id} description`);
	}
});

test('Assertion Evidence carries evidence span ids for the source pane', () => {
	assert.deepEqual(CHAIN_NODE_BY_ID['ev__ae'].evidenceSpans, ['e1', 'e2', 'e5']);
});

test('analysis PROJECT entries shape a related-metric row to their columns', () => {
	const sameRow = {
		metric_id: '812_mtc_3',
		metric_name: '收运频次',
		metric_name_en: 'collection frequency',
		input_record_id: 812,
		class_term_id: 'measurement:collection_frequency_x',
		class_label: 'measurement:collection_frequency_x',
		metric_value: '2',
		metric_unit: 'times/day',
		source_filename: 'std_1503937.pdf'
	};
	// A similar-class row's own class is the matched class (backend sets them equal).
	const similarRow = {
		...sameRow,
		class_term_id: 'measurement:pickup_interval_9c2f',
		class_label: 'measurement:pickup_interval_9c2f',
		matched_class_term_id: 'measurement:pickup_interval_9c2f',
		score: 0.031746
	};

	const same = PROJECT['analysis__same_class'](sameRow);
	assert.equal(same.length, CHAIN_NODE_BY_ID['analysis__same_class'].columns.length);
	assert.equal(same[0], '812_mtc_3');
	assert.equal(same[4], 'std_1503937.pdf');

	const similar = PROJECT['analysis__similar_class'](similarRow);
	assert.equal(similar.length, CHAIN_NODE_BY_ID['analysis__similar_class'].columns.length);
	assert.equal(similar[2], 'measurement:pickup_interval_9c2f'); // class
	assert.equal(similar[3], 'measurement:pickup_interval_9c2f'); // matched via
	assert.equal(similar[4], 0.0317); // score, 4-dp
});
