import test from 'node:test';
import assert from 'node:assert/strict';

import { SATELLITES, CHAINS, CHAIN_NODE_BY_ID, CHAINED_SATELLITES } from './model.js';

test('nine satellites in canvas order', () => {
	assert.deepEqual(
		SATELLITES.map((s) => s.id),
		['document', 'object', 'keyword', 'evidence', 'mdef', 'processor', 'product', 'analysis', 'misc']
	);
});

test('exactly five satellites have no chain', () => {
	const chainless = SATELLITES.filter((s) => !CHAINED_SATELLITES.has(s.id)).map((s) => s.id);
	assert.deepEqual(chainless.sort(), ['analysis', 'document', 'misc', 'product']);
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

test('exactly five chain nodes are live (have a loaderKey)', () => {
	const live = Object.values(CHAIN_NODE_BY_ID)
		.filter((n) => n.loaderKey)
		.map((n) => n.id)
		.sort();
	assert.deepEqual(live, [
		'ev__sa',
		'keyword__concept',
		'mdef__term',
		'object__mention',
		'object__node'
	]);
});

test('the seven remaining chain nodes render a schema panel (no loaderKey)', () => {
	const schema = Object.values(CHAIN_NODE_BY_ID)
		.filter((n) => !n.loaderKey)
		.map((n) => n.id)
		.sort();
	assert.deepEqual(schema, [
		'ev__ae',
		'ev__dc',
		'mdef__contract',
		'proc__associate',
		'proc__extract',
		'proc__normalize',
		'proc__project'
	]);
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
